package main

import (
	"fmt"
	"strconv"
	"strings"
)

var byteUnits = map[byte]int64{'k': 1 << 10, 'm': 1 << 20, 'g': 1 << 30}

func parseBytes(s string) (int64, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, nil
	}
	unit := int64(1)
	if u, ok := byteUnits[s[len(s)-1]]; ok {
		unit, s = u, s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || n > (1<<62)/unit {
		return 0, fmt.Errorf("want a size like 2g, 512m or a byte count")
	}
	return n * unit, nil
}
