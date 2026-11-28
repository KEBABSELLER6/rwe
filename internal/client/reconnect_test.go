package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func scriptedLogs(t *testing.T, responses ...func(w http.ResponseWriter)) (*Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		responses[min(n, len(responses)-1)](w)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL), &calls
}

func logLines(lines ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for _, l := range lines {
			fmt.Fprintf(w, "data: %s\n\n", l)
		}
	}
}

func logDone(lines ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		logLines(lines...)(w)
		fmt.Fprint(w, "event: done\ndata: {\"id\":\"j1\",\"status\":\"passed\"}\n\n")
	}
}

func status(code int) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		http.Error(w, `{"message":"nope"}`, code)
	}
}

func shortReconnect(t *testing.T, timeout time.Duration) {
	delay, limit := reconnectDelay, reconnectTimeout
	reconnectDelay, reconnectTimeout = time.Millisecond, timeout
	t.Cleanup(func() { reconnectDelay, reconnectTimeout = delay, limit })
}

func TestFollowLogsReconnects(t *testing.T) {
	shortReconnect(t, time.Minute)
	c, calls := scriptedLogs(t,
		logLines("a", "b"),
		status(http.StatusServiceUnavailable),
		logDone("a", "b", "c"),
	)

	var got []string
	job, err := c.FollowLogs(t.Context(), "j1", func(line string) { got = append(got, line) })
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobPassed {
		t.Errorf("status = %s, want passed", job.Status)
	}
	if want := []string{"a", "b", reconnectNotice, "c"}; !slices.Equal(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if calls.Load() != 3 {
		t.Errorf("requests = %d, want 3", calls.Load())
	}
}

func TestFollowLogsStopsOnClientError(t *testing.T) {
	shortReconnect(t, time.Minute)
	c, calls := scriptedLogs(t, status(http.StatusNotFound))

	_, err := c.FollowLogs(t.Context(), "j1", func(string) {})
	if !isStatus(err, http.StatusNotFound) {
		t.Errorf("err = %v, want 404", err)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1", calls.Load())
	}
}

func TestFollowLogsGivesUp(t *testing.T) {
	shortReconnect(t, 20*time.Millisecond)
	c, _ := scriptedLogs(t, status(http.StatusBadGateway))

	_, err := c.FollowLogs(t.Context(), "j1", func(string) {})
	if !isStatus(err, http.StatusBadGateway) {
		t.Errorf("err = %v, want the last 502", err)
	}
}

func TestFollowLogsBadFinalJob(t *testing.T) {
	shortReconnect(t, time.Minute)
	c, calls := scriptedLogs(t, func(w http.ResponseWriter) {
		fmt.Fprint(w, "event: done\ndata: {\n\n")
	})

	_, err := c.FollowLogs(t.Context(), "j1", func(string) {})
	if !errors.Is(err, errBadFinalJob) || calls.Load() != 1 {
		t.Errorf("err = %v after %d requests, want errBadFinalJob after 1", err, calls.Load())
	}
}
