package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"strings"

	"kebabseller6/rwe/internal/client"
	"kebabseller6/rwe/internal/mcp"
)

const (
	tailLines        = 100
	defaultLogsLimit = 200
)

func serveMCP(ctx context.Context, cfg config) error {
	c := client.New(cfg.Server)

	run := mcp.Tool{
		Name: "run",
		Description: "Sync the current project to the remote rwe server and run a shell command there in a fresh container " +
			"(default image " + cfg.Image + "). Use it for builds and tests instead of running them locally. " +
			"Waits for the job and returns its status, exit code, duration and the last " + fmt.Sprint(tailLines) + " log lines; " +
			"use the logs tool with the job id for the rest.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":         map[string]any{"type": "string", "description": "Shell command, run with sh -c in the project root, e.g. \"make test\""},
				"image":           map[string]any{"type": "string", "description": "Container image, defaults to " + cfg.Image},
				"timeout_seconds": map[string]any{"type": "integer", "description": "Job timeout; omit for the server default"},
				"artifacts": map[string]any{
					"type": "array", "items": map[string]any{"type": "string"},
					"description": "Files or directories (relative to the project root) to copy back after the job, e.g. [\"coverage.out\", \"dist\", \"reports/*.xml\"]; globs use * ? [a-z] within one path segment; replaces the artifacts list in .rwe.json",
				},
			},
			"required": []string{"command"},
		},
		Call: func(ctx context.Context, raw jsontext.Value) (string, error) {
			var args struct {
				Command        string   `json:"command"`
				Image          string   `json:"image"`
				TimeoutSeconds int      `json:"timeout_seconds"`
				Artifacts      []string `json:"artifacts"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			if strings.TrimSpace(args.Command) == "" {
				return "", errors.New("command is required")
			}
			if args.Image == "" {
				args.Image = cfg.Image
			}
			if args.Artifacts == nil {
				args.Artifacts = cfg.Artifacts
			}

			var tail lineTail
			spec := client.JobSpec{Image: args.Image, Command: []string{"sh", "-c", args.Command}, TimeoutSeconds: args.TimeoutSeconds, Caches: cfg.Caches, Artifacts: args.Artifacts}
			job, err := c.Run(ctx, cfg.dir, spec, tail.add)
			if err != nil {
				return "", err
			}
			dst, err := c.FetchArtifacts(ctx, cfg.dir, job)
			if err != nil {
				dst = "not downloaded: " + err.Error()
			}
			return formatRun(job, tail, dst), nil
		},
	}

	logs := mcp.Tool{
		Name:        "logs",
		Description: "Read the log of an rwe job by line range. Jobs are kept only until the rwe server restarts.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string"},
				"offset": map[string]any{"type": "integer", "description": "First line to return, 0-based. Default 0"},
				"limit":  map[string]any{"type": "integer", "description": fmt.Sprintf("Max lines to return. Default %d", defaultLogsLimit)},
			},
			"required": []string{"job_id"},
		},
		Call: func(ctx context.Context, raw jsontext.Value) (string, error) {
			args := struct {
				JobID  string `json:"job_id"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}{Limit: defaultLogsLimit}
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}

			var lines []string
			job, err := c.FollowLogs(ctx, args.JobID, func(line string) { lines = append(lines, line) })
			if err != nil {
				return "", err
			}
			return formatLogs(job, lines, args.Offset, args.Limit), nil
		},
	}

	return mcp.NewServer("rwe", version, run, logs).Serve(ctx, os.Stdin, os.Stdout)
}

type lineTail struct {
	lines []string
	total int
}

func (t *lineTail) add(line string) {
	t.total++
	t.lines = append(t.lines, line)
	if len(t.lines) > tailLines {
		t.lines = t.lines[1:]
	}
}

func formatRun(job client.Job, tail lineTail, artifactsDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\njob_id: %s\n", summary(job), job.ID)
	if artifactsDir != "" {
		fmt.Fprintf(&b, "artifacts: %s\n", artifactsDir)
		for _, a := range job.Artifacts {
			fmt.Fprintf(&b, "  %s (%d bytes)\n", a.Path, a.Size)
		}
	}
	if tail.total > len(tail.lines) {
		fmt.Fprintf(&b, "--- last %d of %d log lines (logs tool for more)\n", len(tail.lines), tail.total)
	} else {
		fmt.Fprintf(&b, "--- log (%d lines)\n", tail.total)
	}
	b.WriteString(strings.Join(tail.lines, "\n"))
	return b.String()
}

func formatLogs(job client.Job, lines []string, offset, limit int) string {
	start := min(max(offset, 0), len(lines))
	end := min(start+max(limit, 1), len(lines))

	var b strings.Builder
	fmt.Fprintf(&b, "%s\nlines %d-%d of %d\n", summary(job), start, end, len(lines))
	b.WriteString(strings.Join(lines[start:end], "\n"))
	return b.String()
}
