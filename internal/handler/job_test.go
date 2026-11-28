package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KEBABSELLER6/rwe/internal/service"
)

type scriptRunner struct{}

func (scriptRunner) Run(ctx context.Context, spec service.RunSpec, logs io.Writer) (int, error) {
	fmt.Fprint(logs, "hello\nworld\n")
	return 0, os.WriteFile(filepath.Join(spec.FilesDir, "out.txt"), []byte("built"), 0o644)
}

type blockingRunner struct{ started chan struct{} }

func (r blockingRunner) Run(ctx context.Context, spec service.RunSpec, logs io.Writer) (int, error) {
	close(r.started)
	<-ctx.Done()
	return -1, ctx.Err()
}

func submit(t *testing.T, srv *httptest.Server, ws service.Workspace, extra string) service.Job {
	t.Helper()
	body := fmt.Sprintf(`{"expected_hash":%q,"image":"img","command":["build"]%s}`, ws.ManifestHash, extra)
	status, raw := do(t, "POST", srv.URL+"/workspaces/"+ws.ID.String()+"/jobs", "application/json", []byte(body))
	if status != http.StatusCreated {
		t.Fatalf("submit = %d %s", status, raw)
	}
	var job service.Job
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatal(err)
	}
	return job
}

func followLogs(t *testing.T, srv *httptest.Server, id string) (string, http.Header) {
	t.Helper()
	resp, err := http.Get(srv.URL + "/jobs/" + id + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), resp.Header
}

func finalJob(t *testing.T, stream string) service.Job {
	t.Helper()
	_, done, ok := strings.Cut(stream, "event: done\ndata: ")
	if !ok {
		t.Fatalf("stream has no done event:\n%s", stream)
	}
	var job service.Job
	if err := json.Unmarshal([]byte(strings.TrimSuffix(done, "\n\n")), &job); err != nil {
		t.Fatalf("done event: %v\n%s", err, stream)
	}
	return job
}

func TestJobRequestsRejected(t *testing.T) {
	srv := newTestServer(t)
	ws := createWorkspace(t, srv, map[string]string{"a.txt": "a"})
	jobsPath := "/workspaces/" + ws.ID.String() + "/jobs"
	stale := strings.Repeat("0", 64)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"submit with stale expected_hash", "POST", jobsPath, `{"expected_hash":"` + stale + `","image":"img","command":["true"]}`, 409},
		{"submit with typo'd key", "POST", jobsPath, `{"expected_hash":"` + ws.ManifestHash + `","image":"img","command":["true"],"timout_seconds":5}`, 400},
		{"submit without command", "POST", jobsPath, `{"expected_hash":"` + ws.ManifestHash + `","image":"img"}`, 400},
		{"submit with bad workspace id", "POST", "/workspaces/nope/jobs", `{"expected_hash":"` + ws.ManifestHash + `","image":"img","command":["true"]}`, 400},
		{"get with bad id", "GET", "/jobs/nope", "", 400},
		{"get unknown job", "GET", "/jobs/" + unknownID, "", 404},
		{"logs of unknown job", "GET", "/jobs/" + unknownID + "/logs", "", 404},
		{"artifacts of unknown job", "GET", "/jobs/" + unknownID + "/artifacts", "", 404},
		{"cancel unknown job", "DELETE", "/jobs/" + unknownID, "", 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, raw := do(t, tt.method, srv.URL+tt.path, "application/json", []byte(tt.body))
			if status != tt.want {
				t.Errorf("status = %d, want %d; body %s", status, tt.want, raw)
			}
		})
	}
}

func TestLogsStreamLinesThenDone(t *testing.T) {
	srv := newTestServer(t)
	ws := createWorkspace(t, srv, map[string]string{"a.txt": "a"})
	job := submit(t, srv, ws, "")

	stream, header := followLogs(t, srv, job.ID.String())
	if ct := header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	if !strings.HasPrefix(stream, "data: hello\n\ndata: world\n\nevent: done\n") {
		t.Errorf("stream does not start with the log lines and then done:\n%s", stream)
	}
	if final := finalJob(t, stream); final.ID != job.ID || final.Status != service.JobPassed {
		t.Errorf("final job = %s %s, want %s passed", final.ID, final.Status, job.ID)
	}
}

func TestRunningJobBlocksSyncAndArtifactsUntilCancelled(t *testing.T) {
	runner := blockingRunner{started: make(chan struct{})}
	srv, _ := newTestServerWith(t, runner)
	ws := createWorkspace(t, srv, map[string]string{"a.txt": "a"})
	job := submit(t, srv, ws, "")
	<-runner.started

	if status, raw := do(t, "POST", srv.URL+"/workspaces/"+ws.ID.String()+"/sync", "application/json", []byte(`{"files":{}}`)); status != http.StatusConflict {
		t.Errorf("sync while the job runs = %d %s, want 409", status, raw)
	}
	if status, raw := do(t, "GET", srv.URL+"/jobs/"+job.ID.String()+"/artifacts", "", nil); status != http.StatusConflict {
		t.Errorf("artifacts while the job runs = %d %s, want 409", status, raw)
	}
	if status, raw := do(t, "DELETE", srv.URL+"/jobs/"+job.ID.String(), "", nil); status != http.StatusAccepted {
		t.Errorf("cancel = %d %s, want 202", status, raw)
	}

	stream, _ := followLogs(t, srv, job.ID.String())
	if final := finalJob(t, stream); final.Status != service.JobCancelled {
		t.Errorf("final status = %s, want cancelled", final.Status)
	}
}

func TestArtifactsDownloadMissingAndExpired(t *testing.T) {
	srv, data := newTestServerWith(t, scriptRunner{})
	ws := createWorkspace(t, srv, map[string]string{"a.txt": "a"})

	with := submit(t, srv, ws, `,"artifacts":["out.txt"]`)
	followLogs(t, srv, with.ID.String())
	status, raw := do(t, "GET", srv.URL+"/jobs/"+with.ID.String()+"/artifacts", "", nil)
	if status != http.StatusOK {
		t.Fatalf("artifacts = %d %s, want 200", status, raw)
	}
	if names := tarNames(t, raw); len(names) != 1 || names[0] != "out.txt" {
		t.Errorf("archive holds %v, want [out.txt]", names)
	}

	without := submit(t, srv, ws, "")
	followLogs(t, srv, without.ID.String())
	if status, raw := do(t, "GET", srv.URL+"/jobs/"+without.ID.String()+"/artifacts", "", nil); status != http.StatusNotFound {
		t.Errorf("artifacts of a job without any = %d %s, want 404", status, raw)
	}

	if err := os.RemoveAll(filepath.Join(data, "jobs", with.ID.String(), "artifacts")); err != nil {
		t.Fatal(err)
	}
	if status, raw := do(t, "GET", srv.URL+"/jobs/"+with.ID.String()+"/artifacts", "", nil); status != http.StatusGone {
		t.Errorf("expired artifacts = %d %s, want 410", status, raw)
	}
}

func tarNames(t *testing.T, archive []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			names = append(names, hdr.Name)
		}
	}
}
