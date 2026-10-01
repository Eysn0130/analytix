//go:build !darwin && !linux

package filestore

import (
	"analytix.local/runtime-go/internal/ports/workspaceread"
	"context"
	"time"
)

func (*WorkspaceReadFiles) InspectRoot(context.Context, string) (workspaceread.Root, error) {
	return workspaceread.Root{}, workspaceread.ErrUnavailable
}

func (*WorkspaceReadFiles) Scan(context.Context, workspaceread.Root, bool, time.Duration, func() error) ([]workspaceread.File, error) {
	return nil, workspaceread.ErrUnavailable
}
