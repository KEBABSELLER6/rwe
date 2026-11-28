package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/KEBABSELLER6/rwe/internal/manifest"
)

const jobLogFile = "log"

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobPassed    JobStatus = "passed"
	JobFailed    JobStatus = "failed"
	JobTimeout   JobStatus = "timeout"
	JobError     JobStatus = "error"
	JobCancelled JobStatus = "cancelled"
)

func (s JobStatus) Finished() bool {
	return s != JobQueued && s != JobRunning
}

type JobRequest struct {
	WorkspaceID  uuid.UUID
	ExpectedHash string
	Image        string
	Command      []string
	Timeout      time.Duration
	Memory       int64
	CPUs         float64
	Caches       []string
	Artifacts    []string
}

type Job struct {
	ID          uuid.UUID        `json:"id"`
	WorkspaceID uuid.UUID        `json:"workspace_id"`
	Image       string           `json:"image"`
	Command     []string         `json:"command"`
	Caches      []string         `json:"caches,omitempty"`
	Status      JobStatus        `json:"status"`
	ExitCode    *int             `json:"exit_code,omitempty"`
	Error       string           `json:"error,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	StartedAt   time.Time        `json:"started_at,omitzero"`
	FinishedAt  time.Time        `json:"finished_at,omitzero"`
	DurationMS  int64            `json:"duration_ms,omitzero"`
	PhasesMS    map[string]int64 `json:"phases_ms,omitempty"`
	Artifacts   []Artifact       `json:"artifacts,omitempty"`

	expectedHash  string
	spec          RunSpec
	artifactPaths []string
	cancel        context.CancelCauseFunc
}

type JobLimits struct {
	DefaultTimeout    time.Duration
	MaxTimeout        time.Duration
	QueueSize         int
	MaxMemory         int64
	LogRetention      time.Duration
	CacheRetention    time.Duration
	ArtifactRetention time.Duration
	ArtifactMaxSize   int64
}

var DefaultJobLimits = JobLimits{
	DefaultTimeout:    10 * time.Minute,
	MaxTimeout:        time.Hour,
	QueueSize:         16,
	LogRetention:      7 * 24 * time.Hour,
	CacheRetention:    30 * 24 * time.Hour,
	ArtifactRetention: 24 * time.Hour,
	ArtifactMaxSize:   256 << 20,
}

type WorkspaceLocker interface {
	Acquire(id uuid.UUID, expectedHash string) (filesDir string, err error)
	Release(id uuid.UUID)
	Exists(id uuid.UUID) bool
}

type JobService struct {
	dir        string
	cachesDir  string
	workspaces WorkspaceLocker
	runner     Runner
	limits     JobLimits
	queue      chan uuid.UUID

	mu   sync.Mutex
	jobs map[uuid.UUID]*Job
}

func NewJobService(dir, cachesDir string, workspaces WorkspaceLocker, runner Runner, limits JobLimits) (*JobService, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create jobs dir: %w", err)
	}
	s := &JobService{
		dir:        dir,
		cachesDir:  cachesDir,
		workspaces: workspaces,
		runner:     runner,
		limits:     limits,
		queue:      make(chan uuid.UUID, limits.QueueSize),
		jobs:       map[uuid.UUID]*Job{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *JobService) Get(id uuid.UUID) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: job %s", ErrNotFound, id)
	}
	return *job, nil
}

func (s *JobService) logPath(id uuid.UUID) string {
	return filepath.Join(s.dir, id.String(), jobLogFile)
}

func (s *JobService) jobFilesDir(id uuid.UUID) string {
	return filepath.Join(s.dir, id.String(), filesDirName)
}

func (s *JobService) Submit(req JobRequest) (Job, error) {
	if err := s.validate(&req); err != nil {
		return Job{}, err
	}

	filesDir, err := s.workspaces.Acquire(req.WorkspaceID, req.ExpectedHash)
	if err != nil {
		return Job{}, err
	}

	job, err := s.newJob(req, filesDir)
	if err != nil {
		s.workspaces.Release(req.WorkspaceID)
		return Job{}, err
	}

	select {
	case s.queue <- job.ID:
		return job, nil
	default:
		s.forget(job.ID)
		s.workspaces.Release(req.WorkspaceID)
		return Job{}, fmt.Errorf("%w: job queue is full", ErrConflict)
	}
}

var errCancelled = errors.New("cancelled by client")

func (s *JobService) Cancel(id uuid.UUID) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("%w: job %s", ErrNotFound, id)
	}
	switch job.Status {
	case JobQueued:
		job.Status = JobCancelled
		job.FinishedAt = time.Now()
		s.saveOrWarn(job)
		s.workspaces.Release(job.WorkspaceID)
	case JobRunning:
		job.cancel(errCancelled)
	}
	return *job, nil
}

func (s *JobService) validate(req *JobRequest) error {
	switch {
	case !manifest.Sha256Hex.MatchString(req.ExpectedHash):
		return fmt.Errorf("%w: expected_hash must be a sha256", ErrInvalidInput)
	case req.Image == "":
		return fmt.Errorf("%w: image is required", ErrInvalidInput)
	case len(req.Command) == 0:
		return fmt.Errorf("%w: command is required", ErrInvalidInput)
	case req.Timeout < 0 || req.Timeout > s.limits.MaxTimeout:
		return fmt.Errorf("%w: timeout must be between 0 and %s", ErrInvalidInput, s.limits.MaxTimeout)
	case req.Memory < 0 || req.CPUs < 0:
		return fmt.Errorf("%w: limits must not be negative", ErrInvalidInput)
	case s.limits.MaxMemory > 0 && req.Memory > s.limits.MaxMemory:
		return fmt.Errorf("%w: memory must not exceed %d bytes", ErrInvalidInput, s.limits.MaxMemory)
	}
	if err := validateCaches(req.Caches); err != nil {
		return err
	}
	artifacts, err := validateArtifacts(req.Artifacts)
	if err != nil {
		return err
	}
	req.Artifacts = artifacts
	if req.Timeout == 0 {
		req.Timeout = s.limits.DefaultTimeout
	}
	if req.Memory == 0 {
		req.Memory = s.limits.MaxMemory
	}
	return nil
}

func (s *JobService) newJob(req JobRequest, filesDir string) (Job, error) {
	id := uuid.NewV7()
	if err := os.MkdirAll(filepath.Dir(s.logPath(id)), 0o755); err != nil {
		return Job{}, err
	}
	if err := os.WriteFile(s.logPath(id), nil, 0o644); err != nil {
		return Job{}, err
	}

	job := &Job{
		ID:          id,
		WorkspaceID: req.WorkspaceID,
		Image:       req.Image,
		Command:     slices.Clone(req.Command),
		Caches:      slices.Clone(req.Caches),
		Status:      JobQueued,
		CreatedAt:   time.Now(),
	}
	s.prepare(job, req, filesDir)

	if err := s.save(job); err != nil {
		forceRemoveAll(filepath.Dir(s.logPath(id)))
		return Job{}, err
	}

	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	return *job, nil
}

func (s *JobService) prepare(job *Job, req JobRequest, filesDir string) {
	job.expectedHash = req.ExpectedHash
	job.artifactPaths = req.Artifacts
	job.spec = RunSpec{
		Name:     "rwe-" + job.ID.String(),
		Image:    req.Image,
		Command:  slices.Clone(req.Command),
		FilesDir: filesDir,
		Timeout:  req.Timeout,
		Memory:   req.Memory,
		CPUs:     req.CPUs,
		Caches:   s.cacheMounts(req.WorkspaceID, req.Caches),
	}
}

func (s *JobService) forget(id uuid.UUID) {
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
	forceRemoveAll(filepath.Dir(s.logPath(id)))
}
