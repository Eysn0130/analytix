package documentgeneration

import (
	"context"

	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

type ResolvedArtifact struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
	ByteSize int64  `json:"byteSize"`
	Changed  bool   `json:"changed"`
}

type ArtifactFiles interface {
	InspectGenerated(context.Context, string, checkpointfileport.PathAuthority, string, string) (ResolvedArtifact, error)
}
