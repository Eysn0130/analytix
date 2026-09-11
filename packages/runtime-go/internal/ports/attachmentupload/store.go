package attachmentupload

import (
	"context"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
)

// PreparedUpload is attempt-private. Its metadata contains private owner
// material and must never be serialized to ordinary events, SSE, or history.
type PreparedUpload interface {
	Owner() domainattachment.OwnerRecordV1
	Metadata() map[string]any
	MetadataSHA256() string
	Commit(context.Context) (map[string]any, error)
	Abort(context.Context) error
}

type Store interface {
	Prepare(context.Context, map[string]any) (PreparedUpload, error)
}
