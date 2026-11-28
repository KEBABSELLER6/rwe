package service

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestWorkspaceRetention_DeletesOnlyUnusedWorkspaces(t *testing.T) {
	for _, tc := range []struct {
		name      string
		retention time.Duration
		age       time.Duration
		busy      bool
		wantKept  bool
	}{
		{"expired", time.Hour, 2 * time.Hour, false, false},
		{"recent", time.Hour, time.Minute, false, true},
		{"busy", time.Hour, 2 * time.Hour, true, true},
		{"disabled", 0, 1000 * time.Hour, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, wsDir, id := newSyncedWorkspace(t)
			s.limits.Retention = tc.retention
			if tc.busy {
				s.busy[id] = true
			}

			s.prune(time.Now().Add(tc.age))

			_, err := os.Stat(wsDir)
			if kept := err == nil; kept != tc.wantKept {
				t.Errorf("workspace kept = %v, want %v (stat err %v)", kept, tc.wantKept, err)
			}
			entries, _ := os.ReadDir(s.dir)
			if want := map[bool]int{true: 1, false: 0}[tc.wantKept]; len(entries) != want {
				t.Errorf("%d entries left in %s, want %d", len(entries), s.dir, want)
			}
		})
	}
}

func TestWorkspaceRetention_UseKeepsWorkspace(t *testing.T) {
	s, wsDir, id := newSyncedWorkspace(t)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(wsDir, old, old); err != nil {
		t.Fatal(err)
	}
	m, err := s.loadManifest(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Acquire(id, m.Hash()); err != nil {
		t.Fatal(err)
	}
	s.Release(id)
	s.limits.Retention = time.Hour

	s.prune(time.Now())

	if _, err := os.Stat(wsDir); err != nil {
		t.Errorf("used workspace deleted: %v", err)
	}
}

func TestWorkspaceRetention_DeletedWorkspaceIsNotFound(t *testing.T) {
	s, _, id := newSyncedWorkspace(t)
	s.limits.Retention = time.Hour

	s.prune(time.Now().Add(2 * time.Hour))

	if _, err := s.Acquire(id, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("acquire deleted workspace = %v, want ErrNotFound", err)
	}
}
