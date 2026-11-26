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

const logPollInterval = 200 * time.Millisecond

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
			if errors.Is(readErr, io.EOF) || n == 0 {
				break
			}
			if readErr != nil {
				return readErr
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
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return data, nil
		}
		if err := emitLine(data[:i]); err != nil {
			return nil, err
		}
		data = data[i+1:]
	}
}
