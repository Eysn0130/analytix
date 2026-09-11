package rawartifact

import (
	"context"
	"io"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

// LargeOpaqueChunkWriteDisposition distinguishes a no-replace commit made by
// the current call from an exact object that was already present. Neither
// disposition is an EvidenceReceipt, acquisition receipt, or snapshot
// authority.
type LargeOpaqueChunkWriteDisposition string

const (
	LargeOpaqueChunkCreated       LargeOpaqueChunkWriteDisposition = "created"
	LargeOpaqueChunkExistingEqual LargeOpaqueChunkWriteDisposition = "existing_equal"
)

// LargeOpaqueChunkWriteOutcome is an inert storage result. The descriptor
// remains the semantic authority for its expected digest and byte length.
type LargeOpaqueChunkWriteOutcome struct {
	Disposition      LargeOpaqueChunkWriteDisposition
	DescriptorDigest string
	ChunkSHA256      string
	ChunkByteLength  uint64
}

// LargeOpaqueChunkStore stores raw evidence bytes without exposing List,
// Latest, Current, Active, paths, or filesystem handles. PutExact must stream
// exactly descriptor.ChunkByteLength bytes, prove EOF, verify ChunkSHA256,
// commit without replacement, fsync the commit, and exact-read the committed
// object before returning success.
//
// ReadExact must retain the complete bounded chunk in private memory until its
// identity, topology, metadata, exact byte count, EOF, and digest have all
// passed. It must not call the supplied writer at all on a validation failure.
// Delivery begins only after that immutable verification boundary; a delivery
// error is distinct from evidence corruption.
type LargeOpaqueChunkStore interface {
	PutExact(
		context.Context,
		domainevidence.RawArtifactContentChunkDescriptorV1,
		io.Reader,
	) (LargeOpaqueChunkWriteOutcome, error)
	ReadExact(
		context.Context,
		domainevidence.RawArtifactContentChunkDescriptorV1,
		io.Writer,
	) error
}
