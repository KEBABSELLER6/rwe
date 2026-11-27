package handler

import (
	"context"
	"io"
	"kebabseller6/rwe/internal/manifest"
	"kebabseller6/rwe/internal/service"
	"uuid"
)

type WorkspaceService interface {
	Create(ctx context.Context, body io.Reader) (service.Workspace, error)
	Sync(ctx context.Context, id uuid.UUID, files manifest.Manifest, sizes map[string]int64) ([]string, error)
	PutBlob(ctx context.Context, id uuid.UUID, sum string, body io.Reader) (int, error)
}

type JobService interface {
	Submit(req service.JobRequest) (service.Job, error)
	FollowLogs(ctx context.Context, id uuid.UUID, emitLine func(line []byte) error) error
	Get(id uuid.UUID) (service.Job, error)
	Cancel(id uuid.UUID) (service.Job, error)
}
