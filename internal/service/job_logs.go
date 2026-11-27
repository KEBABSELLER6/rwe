package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"time"
	"uuid"
)

const (
	logPollInterval = 200 * time.Millisecond
	maxLogLine      = 64 << 10
)

func (s *JobService) FollowLogs(ctx context.Context, id uuid.UUID, emitLine func(line []byte) error) error {
	if _, err := s.Get(id); err != nil {
		return err
	}

	f, err := os.Open(s.logPath(id))
	if err != nil {
		return err
	}
	defer f.Close()

	var pending []byte
	buf := make([]byte, 32<<10)
	for {
		job, err := s.Get(id)
		if err != nil {
			return err
		}
		finishedBeforeRead := job.Status.Finished()

		for {
			n, readErr := f.Read(buf)
			pending = append(pending, buf[:n]...)
			if pending, err = emitLines(pending, emitLine); err != nil {
				return err
			}
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return readErr
			}
			if n == 0 {
				break
			}
		}

		if finishedBeforeRead {
			if len(pending) > 0 {
				return emitLine(pending)
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(logPollInterval):
		}
	}
}

func emitLines(data []byte, emitLine func([]byte) error) ([]byte, error) {
	for {
		line, rest := data, data
		switch i := bytes.IndexByte(data, '\n'); {
		case i >= 0 && i <= maxLogLine:
			line, rest = data[:i], data[i+1:]
		case len(data) > maxLogLine:
			line, rest = data[:maxLogLine], data[maxLogLine:]
		default:
			return data, nil
		}
		if err := emitLine(line); err != nil {
			return nil, err
		}
		data = rest
	}
}
