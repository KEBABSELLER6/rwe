package main

import (
	"fmt"
	"strings"
	"testing"

	"kebabseller6/rwe/internal/client"
)

func TestStatsTool(t *testing.T) {
	dir := t.TempDir()
	for i := range 3 {
		if err := client.SaveRun(dir, client.RunRecord{JobID: fmt.Sprint("job-", i), Status: client.JobPassed}); err != nil {
			t.Fatal(err)
		}
	}
	tool := statsTool(config{dir: dir})

	for raw, want := range map[string]string{`{"limit": 2}`: "last 2 runs", `{}`: "last 3 runs", `{"limit": 0}`: "last 3 runs"} {
		got, err := tool.Call(t.Context(), []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, want) {
			t.Errorf("stats %s = %q, want it to start with %q", raw, got, want)
		}
	}
}
