package service

import (
	"slices"
	"strings"
	"testing"
)

func TestEmitLines(t *testing.T) {
	long := strings.Repeat("x", maxLogLine)
	tests := []struct {
		name        string
		data        string
		wantLines   []string
		wantPending string
	}{
		{"complete lines", "a\nb\n", []string{"a", "b"}, ""},
		{"partial line stays pending", "a\nb", []string{"a"}, "b"},
		{"line at the limit", long + "\n", []string{long}, ""},
		{"long line is split", long + "yz\n", []string{long, "yz"}, ""},
		{"long partial line is split", long + "y", []string{long}, "y"},
		{"partial line at the limit waits", long, nil, long},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines []string
			pending, err := emitLines([]byte(tt.data), func(line []byte) error {
				lines = append(lines, string(line))
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(lines, tt.wantLines) {
				t.Errorf("lines = %q, want %q", lines, tt.wantLines)
			}
			if string(pending) != tt.wantPending {
				t.Errorf("pending = %q, want %q", pending, tt.wantPending)
			}
		})
	}
}
