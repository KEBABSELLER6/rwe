package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"uuid"
)

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
}

type Job struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Image       string    `json:"image"`
	Command     []string  `json:"command"`
	Status      JobStatus `json:"status"`
	ExitCode    *int      `json:"exit_code,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at,omitzero"`
	FinishedAt  time.Time `json:"finished_at,omitzero"`
	DurationMS  int64     `json:"duration_ms,omitzero"`

	spec   RunSpec
	cancel context.CancelCauseFunc
}

type JobLimits struct {
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
	QueueSize      int
	MaxMemory      int64
}

var DefaultJobLimits = JobLimits{
	DefaultTimeout: 10 * time.Minute,
	MaxTimeout:     time.Hour,
	QueueSize:      16,
}

type WorkspaceLocker interface {
	Acquire(id uuid.UUID, expectedHash string) (filesDir string, err error)
	Release(id uuid.UUID)
}

type JobService struct {
	dir        string
	workspaces WorkspaceLocker
	runner     Runner
	limits     JobLimits
	queue      chan uuid.UUID

	mu   sync.Mutex
	jobs map[uuid.UUID]*Job
}

func NewJobService(dir string, workspaces WorkspaceLocker, runner Runner, limits JobLimits) (*JobService, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create jobs dir: %w", err)
	}
	return &JobService{
		dir:        dir,
		workspaces: workspaces,
		runner:     runner,
		limits:     limits,
		queue:      make(chan uuid.UUID, limits.QueueSize),
		jobs:       map[uuid.UUID]*Job{},
	}, nil
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
	return filepath.Join(s.dir, id.String(), "log")
}

func (s *JobService) jobFilesDir(id uuid.UUID) string {
	return filepath.Join(s.dir, id.String(), "files")
}
