package main

import (
	"strings"
	"testing"
	"time"

	"github.com/KEBABSELLER6/rwe/internal/client"
)

func TestFormatStats(t *testing.T) {
	exit := 2
	at := time.Date(2026, 11, 28, 14, 0, 0, 0, time.UTC)
	runs := []client.RunRecord{
		{At: at, JobID: "job-1", Command: []string{"sh", "-c", "make test"}, Hash: "aaaaaaaaaaaaaaaa", Status: client.JobPassed,
			Files: 10, Created: true, SyncMS: 1000, QueueMS: 1000, PhasesMS: map[string]int64{"copy": 1000, "run": 5000}, TotalMS: 10000},
		{At: at.Add(time.Minute), JobID: "job-2", Command: []string{"sh", "-c", "make vet"}, Status: client.JobFailed, ExitCode: &exit,
			Files: 10, Uploaded: 1, SyncMS: 500, PhasesMS: map[string]int64{"run": 1500}, TotalMS: 2000},
		{At: at.Add(90 * time.Second), JobID: "job-old", Command: []string{"true"}, Status: client.JobPassed, SyncMS: 0, TotalMS: 0},
		{At: at.Add(2 * time.Minute), Command: []string{"ls"}, Error: "sync: connection refused", SyncMS: 100, TotalMS: 100},
	}

	got := formatStats(runs)
	for _, want := range []string{
		"last 4 runs: 1 error, 1 failed, 2 passed; total p50 100ms, p90 10s",
		"sync 1.6s (13%), queue 1s (8%), copy 1s (8%), run 6.5s (53%), artifacts 0s (0%), cleanup 0s (0%), other 2s (16%)",
		"failed(2)",
		"other 2s (16%), 1 runs without server phase timings, counted as other",
		"aaaaaaaaaaaa  job-1",
		"errors (newest first):",
		"sync: connection refused",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stats output missing %q:\n%s", want, got)
		}
	}
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[4], "ls") || !strings.Contains(lines[7], "make test") || !strings.Contains(lines[7], "new") {
		t.Errorf("want newest run first and a new workspace marked:\n%s", got)
	}
}

func TestFormatStatsEmpty(t *testing.T) {
	if got := formatStats(nil); !strings.Contains(got, "no runs recorded") {
		t.Errorf("empty stats = %q", got)
	}
}
