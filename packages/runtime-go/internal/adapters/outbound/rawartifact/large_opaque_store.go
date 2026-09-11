package rawartifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	"analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	largeopaqueport "analytix.local/runtime-go/internal/ports/largeopaque"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
)

// LargeOpaqueChunkStore maps validated raw-artifact descriptors to the generic
// non-enumerable filesystem substrate. It never derives an address from bytes
// alone, so identical bytes from different case bindings remain independently
// parent-addressed.
type LargeOpaqueChunkStore struct {
	store largeopaqueport.Store
}

const LargeOpaqueChunkStoreRootNameV1 = "raw-artifact-chunks-v1"

const (
	largeOpaqueChunkStoreCommittedObservationBoundV1 = uint64(1 << 24)
	largeOpaqueChunkStoreTemporaryObservationBoundV1 = uint64(1 << 16)
	largeOpaqueChunkStoreCreateResidueBoundV1        = uint32(257)
)

var _ rawartifactport.LargeOpaqueChunkStore = (*LargeOpaqueChunkStore)(nil)

func NewLargeOpaqueChunkStore(
	lease *persistencefs.CompositeLease,
) (*LargeOpaqueChunkStore, error) {
	scope, err := lease.IssueDataDirPrivateCASScope(LargeOpaqueChunkStoreRootNameV1)
	if err != nil {
		return nil, errors.Join(errors.New("large opaque chunk owner authority is unavailable"), err)
	}
	root, err := scope.Root()
	if err != nil {
		return nil, err
	}
	store, err := finalauthorityadapter.NewLargeOpaqueStore(
		root,
		domainevidence.RawArtifactContentChunkBytesV1,
		scope,
	)
	if err != nil {
		return nil, err
	}
	return &LargeOpaqueChunkStore{store: store}, nil
}

// PrepareLargeOpaqueChunkStoreRecoveryV1 fixes the owner root and object bound
// to the raw-artifact contract. Callers cannot expand recovery authority by
// supplying a different directory or inventory limit.
func PrepareLargeOpaqueChunkStoreRecoveryV1(
	ctx context.Context,
	lease *persistencefs.CompositeLease,
) (*finalauthorityadapter.PreparedLargeOpaqueRecoveryV1, error) {
	scope, err := lease.IssueDataDirPrivateCASScope(LargeOpaqueChunkStoreRootNameV1)
	if err != nil {
		return nil, errors.Join(errors.New("large opaque chunk recovery authority is unavailable"), err)
	}
	root, err := scope.Root()
	if err != nil {
		return nil, err
	}
	return finalauthorityadapter.PrepareLargeOpaqueStoreRecoveryV1(
		ctx,
		root,
		domainevidence.RawArtifactContentChunkBytesV1,
		finalauthorityadapter.LargeOpaqueRecoveryLimitsV1{
			MaxCommittedObjects: largeOpaqueChunkStoreCommittedObservationBoundV1,
			MaxTemporaryObjects: largeOpaqueChunkStoreTemporaryObservationBoundV1,
			MaxCreateResidues:   largeOpaqueChunkStoreCreateResidueBoundV1,
		},
		scope,
	)
}

func (store *LargeOpaqueChunkStore) PutExact(
	ctx context.Context,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
	reader io.Reader,
) (rawartifactport.LargeOpaqueChunkWriteOutcome, error) {
	if store == nil || store.store == nil || largeOpaqueNilInterface(reader) ||
		domainevidence.ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil {
		return rawartifactport.LargeOpaqueChunkWriteOutcome{},
			errors.Join(rawartifactport.ErrMismatch, errors.New("large opaque raw chunk write input is invalid"))
	}
	outcome, err := store.store.PutExact(ctx, largeOpaqueRef(descriptor), reader)
	if err != nil {
		return rawartifactport.LargeOpaqueChunkWriteOutcome{}, mapLargeOpaqueError(err)
	}
	disposition := rawartifactport.LargeOpaqueChunkWriteDisposition("")
	switch outcome {
	case largeopaqueport.PutOutcomeCreated:
		disposition = rawartifactport.LargeOpaqueChunkCreated
	case largeopaqueport.PutOutcomeExistingEqual:
		disposition = rawartifactport.LargeOpaqueChunkExistingEqual
	default:
		return rawartifactport.LargeOpaqueChunkWriteOutcome{},
			errors.Join(rawartifactport.ErrCorrupt, errors.New("large opaque raw chunk write outcome is invalid"))
	}
	return rawartifactport.LargeOpaqueChunkWriteOutcome{
		Disposition:      disposition,
		DescriptorDigest: descriptor.DescriptorDigest,
		ChunkSHA256:      descriptor.ChunkSHA256,
		ChunkByteLength:  descriptor.ChunkByteLength,
	}, nil
}

func (store *LargeOpaqueChunkStore) ReadExact(
	ctx context.Context,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
	writer io.Writer,
) error {
	if store == nil || store.store == nil || largeOpaqueNilInterface(writer) ||
		domainevidence.ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil {
		return errors.Join(rawartifactport.ErrMismatch, errors.New("large opaque raw chunk read input is invalid"))
	}
	// A raw-artifact chunk is bounded to 8 MiB by its domain contract. Keep the
	// generic store's verification output private until the complete read,
	// digest, EOF, metadata, and topology witness succeeds. This mechanically
	// prevents a failed read from leaking unverified bytes through an arbitrary
	// caller-supplied writer.
	verified := bytes.NewBuffer(make([]byte, 0, int(descriptor.ChunkByteLength)))
	if err := store.store.ReadExact(ctx, largeOpaqueRef(descriptor), verified); err != nil {
		return mapLargeOpaqueError(err)
	}
	if uint64(verified.Len()) != descriptor.ChunkByteLength {
		return errors.Join(rawartifactport.ErrCorrupt, largeopaqueport.ErrIntegrity,
			errors.New("verified large opaque chunk length changed before delivery"))
	}
	written, err := writer.Write(verified.Bytes())
	if err != nil {
		return errors.Join(largeopaqueport.ErrDelivery, errors.New("verified large opaque chunk delivery failed"))
	}
	if written != verified.Len() {
		return errors.Join(largeopaqueport.ErrDelivery, io.ErrShortWrite)
	}
	return nil
}

func mapLargeOpaqueError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, largeopaqueport.ErrDelivery):
		return errors.Join(largeopaqueport.ErrDelivery, err)
	case errors.Is(err, largeopaqueport.ErrIndeterminate):
		return errors.Join(rawartifactport.ErrUnavailable, largeopaqueport.ErrIndeterminate, err)
	case errors.Is(err, largeopaqueport.ErrSourceMismatch),
		errors.Is(err, largeopaqueport.ErrIntegrity), errors.Is(err, os.ErrNotExist):
		return errors.Join(rawartifactport.ErrCorrupt, err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, largeopaqueport.ErrSourceRead):
		return errors.Join(rawartifactport.ErrUnavailable, largeopaqueport.ErrSourceRead, err)
	case errors.Is(err, largeopaqueport.ErrUnsupportedPlatform):
		return errors.Join(rawartifactport.ErrUnavailable, err)
	default:
		return errors.Join(rawartifactport.ErrUnavailable, err)
	}
}

func largeOpaqueNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func largeOpaqueRef(descriptor domainevidence.RawArtifactContentChunkDescriptorV1) largeopaqueport.ExactRef {
	return largeopaqueport.ExactRef{
		Address:    descriptor.DescriptorDigest,
		SHA256:     descriptor.ChunkSHA256,
		ByteLength: descriptor.ChunkByteLength,
	}
}
