package client

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRunRecord(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"a.txt": "a", "sub/b.txt": "b"})
	spec := JobSpec{Image: "img", Command: []string{"ls"}}

	job, first, err := c.Run(t.Context(), dir, spec, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if first.JobID != job.ID || first.Hash == "" || first.Files != 2 || !first.Created {
		t.Errorf("first record = %+v, want job %s, a hash, 2 files in a new workspace", first, job.ID)
	}
	if first.Status != JobFailed || first.ExitCode == nil || *first.ExitCode != 3 {
		t.Errorf("first record status = %s, exit %v; want failed, 3", first.Status, first.ExitCode)
	}
	if _, ok := first.PhasesMS["run"]; !ok {
		t.Errorf("first record phases = %v, want a run phase", first.PhasesMS)
	}

	_, second, err := c.Run(t.Context(), dir, spec, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if second.Hash != first.Hash || second.Uploaded != 0 || second.Created {
		t.Errorf("second record = %+v, want hash %s, nothing uploaded, same workspace", second, first.Hash)
	}
}

func TestSaveRunKeepsLatest(t *testing.T) {
	dir := t.TempDir()
	for i := range keepRuns + 5 {
		if err := SaveRun(dir, RunRecord{JobID: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(runsFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != keepRuns {
		t.Fatalf("%d records, want %d", len(lines), keepRuns)
	}
	for i, want := range map[int]string{0: "5", keepRuns - 1: fmt.Sprint(keepRuns + 4)} {
		var rec RunRecord
		if err := json.Unmarshal([]byte(lines[i]), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.JobID != want {
			t.Errorf("record %d is job %s, want %s", i, rec.JobID, want)
		}
	}
}
