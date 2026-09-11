package contextsource

import (
	"context"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
)

type Content struct {
	Text   string
	Digest string
}

type Reader interface {
	ReadContextSource(context.Context, string, domaincontextepoch.SourceEntry) (Content, error)
}
