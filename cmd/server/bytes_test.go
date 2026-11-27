package main

import "testing"

func TestParseBytes(t *testing.T) {
	for in, want := range map[string]int64{
		"":     0,
		"1024": 1024,
		"512m": 512 << 20,
		"2g":   2 << 30,
		" 4G ": 4 << 30,
		"100k": 100 << 10,
	} {
		if got, err := parseBytes(in); err != nil || got != want {
			t.Errorf("parseBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"g", "2gb", "-1", "1.5g", "abc", "9999999999g"} {
		if _, err := parseBytes(in); err == nil {
			t.Errorf("parseBytes(%q) succeeded, want error", in)
		}
	}
}
