package client

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const keepRuns = 50

type RunRecord struct {
	At          time.Time        `json:"at"`
	JobID       string           `json:"job_id,omitempty"`
	Image       string           `json:"image"`
	Command     []string         `json:"command"`
	Hash        string           `json:"hash,omitempty"`
	Status      JobStatus        `json:"status,omitempty"`
	ExitCode    *int             `json:"exit_code,omitempty"`
	Error       string           `json:"error,omitempty"`
	Files       int              `json:"files"`
	Uploaded    int              `json:"uploaded"`
	Created     bool             `json:"created_workspace,omitempty"`
	SyncMS      int64            `json:"sync_ms"`
	QueueMS     int64            `json:"queue_ms"`
	PhasesMS    map[string]int64 `json:"phases_ms,omitempty"`
	ArtifactsMS int64            `json:"artifacts_ms"`
	TotalMS     int64            `json:"total_ms"`
}

func (r *RunRecord) setJob(job Job) {
	r.Status, r.ExitCode, r.Error, r.PhasesMS = job.Status, job.ExitCode, job.Error, job.PhasesMS
	if !job.StartedAt.IsZero() {
		r.QueueMS = job.StartedAt.Sub(job.CreatedAt).Milliseconds()
	}
}

func runsFile(dir string) string {
	return filepath.Join(dir, stateDir, "runs.jsonl")
}

func SaveRun(dir string, rec RunRecord) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	path := runsFile(dir)
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	lines := slices.DeleteFunc(strings.Split(string(old), "\n"), func(l string) bool { return l == "" })
	lines = append(lines, string(line))
	lines = lines[max(0, len(lines)-keepRuns):]

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadRuns(dir string, n int) ([]RunRecord, error) {
	raw, err := os.ReadFile(runsFile(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := slices.DeleteFunc(strings.Split(string(raw), "\n"), func(l string) bool { return l == "" })
	lines = lines[max(0, len(lines)-n):]

	runs := make([]RunRecord, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal([]byte(line), &runs[i]); err != nil {
			return nil, fmt.Errorf("%s: %w", runsFile(dir), err)
		}
	}
	return runs, nil
}
