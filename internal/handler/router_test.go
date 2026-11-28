package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"kebabseller6/rwe/internal/manifest"
	"kebabseller6/rwe/internal/service"
)

const unknownID = "00000000-0000-7000-8000-000000000000"

type noopRunner struct{}

func (noopRunner) Run(context.Context, service.RunSpec, io.Writer) (int, error) { return 0, nil }

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	data := t.TempDir()
	ws, err := service.NewWorkspaceService(filepath.Join(data, "workspaces"), service.DefaultWorkspaceLimits.Retention)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := service.NewJobService(filepath.Join(data, "jobs"), filepath.Join(data, "caches"), ws, noopRunner{}, service.DefaultJobLimits)
	if err != nil {
		t.Fatal(err)
	}
	go jobs.Start(t.Context())
	return serve(t, ws, jobs)
}

func serve(t *testing.T, ws WorkspaceService, jobs JobService) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(NewRouter(ws, jobs, logger))
	t.Cleanup(srv.Close)
	return srv
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(tw.Close(), gz.Close()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func do(t *testing.T, method, url, contentType string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func createWorkspace(t *testing.T, srv *httptest.Server, files map[string]string) service.Workspace {
	t.Helper()
	status, raw := do(t, http.MethodPost, srv.URL+"/workspaces", "application/gzip", tarGz(t, files))
	if status != http.StatusCreated {
		t.Fatalf("create workspace = %d %s", status, raw)
	}
	var ws service.Workspace
	if err := json.Unmarshal(raw, &ws); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestWorkspaceRequestsRejected(t *testing.T) {
	srv := newTestServer(t)
	ws := createWorkspace(t, srv, map[string]string{"a.txt": "a"})
	wsPath := "/workspaces/" + ws.ID.String()

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		want        int
	}{
		{"create with wrong content-type", "POST", "/workspaces", "text/plain", "x", 400},
		{"create with non-gzip body", "POST", "/workspaces", "application/gzip", "x", 400},
		{"sync with bad id", "POST", "/workspaces/nope/sync", "application/json", `{"files":{}}`, 400},
		{"sync unknown workspace", "POST", "/workspaces/" + unknownID + "/sync", "application/json", `{"files":{}}`, 404},
		{"sync with wrong content-type", "POST", wsPath + "/sync", "text/plain", `{"files":{}}`, 400},
		{"sync with typo'd key", "POST", wsPath + "/sync", "application/json", `{"file":{}}`, 400},
		{"sync without files", "POST", wsPath + "/sync", "application/json", `{}`, 400},
		{"sync with escaping path", "POST", wsPath + "/sync", "application/json", `{"files":{"../x":"` + sha("x") + `"}}`, 400},
		{"upload with wrong content-type", "PUT", wsPath + "/blobs/" + sha("b"), "text/plain", "b", 400},
		{"upload with invalid sha256", "PUT", wsPath + "/blobs/nope", "application/octet-stream", "b", 400},
		{"upload without pending sync", "PUT", "/workspaces/" + unknownID + "/blobs/" + sha("b"), "application/octet-stream", "b", 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, raw := do(t, tt.method, srv.URL+tt.path, tt.contentType, []byte(tt.body))
			if status != tt.want {
				t.Errorf("status = %d, want %d; body %s", status, tt.want, raw)
			}
			var res ErrorResponse
			if err := json.Unmarshal(raw, &res); err != nil || res.Message == "" {
				t.Errorf("body %s is not an error response: %v", raw, err)
			}
		})
	}
}

func TestUploadAcceptsOnlyRequestedBlobs(t *testing.T) {
	srv := newTestServer(t)
	ws := createWorkspace(t, srv, map[string]string{"a.txt": "a"})
	wsPath := "/workspaces/" + ws.ID.String()

	body, _ := json.Marshal(SyncRequest{Files: manifest.Manifest{"a.txt": sha("a"), "b.txt": sha("b")}, Sizes: map[string]int64{"b.txt": 1}})
	if status, raw := do(t, "POST", srv.URL+wsPath+"/sync", "application/json", body); status != 200 {
		t.Fatalf("sync = %d %s", status, raw)
	}
	if status, raw := do(t, "PUT", srv.URL+wsPath+"/blobs/"+sha("never requested"), "application/octet-stream", []byte("never requested")); status != 400 {
		t.Errorf("upload of a blob nobody asked for = %d %s, want 400", status, raw)
	}

	status, raw := do(t, "PUT", srv.URL+wsPath+"/blobs/"+sha("b"), "application/octet-stream", []byte("b"))
	if status != 200 || !strings.Contains(string(raw), `"remaining":0`) {
		t.Errorf("upload of the requested blob = %d %s, want 200 with nothing remaining", status, raw)
	}
}

type failingWorkspaces struct{ WorkspaceService }

func (failingWorkspaces) Create(context.Context, io.Reader) (service.Workspace, error) {
	return service.Workspace{}, errors.New("disk on fire at /var/lib/rwe")
}

func TestInternalErrorHidesDetails(t *testing.T) {
	srv := serve(t, failingWorkspaces{}, nil)

	status, raw := do(t, "POST", srv.URL+"/workspaces", "application/gzip", tarGz(t, nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if strings.Contains(string(raw), "/var/lib/rwe") || !strings.Contains(string(raw), `"internal error"`) {
		t.Errorf("body %s leaks details or lacks the generic message", raw)
	}
}
