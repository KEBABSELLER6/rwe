package client

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const detachedTimeout = 10 * time.Second

type JobStatus string

const (
	JobPassed    JobStatus = "passed"
	JobFailed    JobStatus = "failed"
	JobTimeout   JobStatus = "timeout"
	JobCancelled JobStatus = "cancelled"
)

type JobSpec struct {
	Image          string   `json:"image"`
	Command        []string `json:"command"`
	TimeoutSeconds int      `json:"timeout_seconds,omitzero"`
	Caches         []string `json:"caches,omitempty"`
	Artifacts      []string `json:"artifacts,omitempty"`
}

type Job struct {
	ID         string           `json:"id"`
	Image      string           `json:"image"`
	Command    []string         `json:"command"`
	Status     JobStatus        `json:"status"`
	ExitCode   *int             `json:"exit_code"`
	Error      string           `json:"error"`
	CreatedAt  time.Time        `json:"created_at"`
	StartedAt  time.Time        `json:"started_at"`
	DurationMS int64            `json:"duration_ms"`
	PhasesMS   map[string]int64 `json:"phases_ms"`
	Artifacts  []Artifact       `json:"artifacts"`
}

func (c *Client) Run(ctx context.Context, dir string, spec JobSpec, emit func(line string)) (Job, RunRecord, error) {
	rec := RunRecord{At: time.Now(), Image: spec.Image, Command: spec.Command}
	synced, err := c.Sync(ctx, dir)
	rec.SyncMS = time.Since(rec.At).Milliseconds()
	if err != nil {
		return Job{}, rec, fmt.Errorf("sync: %w", err)
	}
	rec.Hash, rec.Files, rec.Uploaded, rec.Created = synced.Hash, synced.Files, synced.Uploaded, synced.Created
	submitCtx, cancel := detached(ctx)
	job, err := c.Submit(submitCtx, synced, spec)
	cancel()
	if err != nil {
		return Job{}, rec, err
	}
	rec.JobID = job.ID

	final, err := c.FollowLogs(ctx, job.ID, emit)
	if ctx.Err() != nil {
		cancelCtx, cancel := detached(ctx)
		defer cancel()
		c.Cancel(cancelCtx, job.ID)
	}
	rec.setJob(final)
	return final, rec, err
}

func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), detachedTimeout)
}

func (c *Client) Cancel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/jobs/"+id, "", nil, nil)
}

func (c *Client) Submit(ctx context.Context, synced Synced, spec JobSpec) (Job, error) {
	body := struct {
		ExpectedHash string `json:"expected_hash"`
		JobSpec      `json:",inline"`
	}{synced.Hash, spec}

	var job Job
	err := c.postJSON(ctx, "/workspaces/"+synced.WorkspaceID+"/jobs", body, &job)
	return job, err
}

func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var job Job
	err := c.do(ctx, http.MethodGet, "/jobs/"+id, "", nil, &job)
	return job, err
}

func (c *Client) FollowLogs(ctx context.Context, id string, emit func(line string)) (Job, error) {
	resp, err := c.send(ctx, http.MethodGet, "/jobs/"+id+"/logs", "", nil)
	if err != nil {
		return Job{}, err
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && event == "done":
			var job Job
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &job); err != nil {
				return Job{}, fmt.Errorf("decode final job: %w", err)
			}
			return job, nil
		case strings.HasPrefix(line, "data: "):
			emit(strings.TrimPrefix(line, "data: "))
		case line == "":
			event = ""
		}
	}
	if err := sc.Err(); err != nil {
		return Job{}, fmt.Errorf("log stream: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Job{}, err
	}
	return c.Job(ctx, id)
}
