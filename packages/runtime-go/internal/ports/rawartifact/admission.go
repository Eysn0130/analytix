package rawartifact

import (
	"context"
	"errors"
	"io"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

var (
	ErrUnavailable = errors.New("raw artifact source is unavailable")
	ErrMismatch    = errors.New("raw artifact material is mismatched")
	ErrCorrupt     = errors.New("raw artifact material is corrupt")
)

// CandidateSource exposes only parent-addressed reads for one untrusted
// imported hierarchy. It deliberately has no List, Latest, Current, Active,
// or cached-catalog selector. Verifying this interface proves only that the
// supplied hierarchy is internally complete; it does not prove that it is the
// complete byte inventory selected by the host. Production acquisition must
// instead freeze authenticated host handles and stream each selected object to
// EOF while constructing the hierarchy itself.
type CandidateSource interface {
	AcquisitionIntent(context.Context) (domainevidence.RawArtifactAcquisitionIntentV1, error)
	Manifest(context.Context, domainevidence.RawArtifactAcquisitionIntentV1) (domainevidence.RawArtifactManifestV1, error)
	ManifestPage(context.Context, domainevidence.RawArtifactManifestPageDescriptorV1) (domainevidence.RawArtifactManifestPageV1, error)
	SourceLocator(context.Context, domainevidence.RawArtifactEntryV1) (domainevidence.RawArtifactSourceLocatorV1, error)
	ContentRoot(context.Context, domainevidence.RawArtifactEntryV1) (domainevidence.RawArtifactContentRootV1, error)
	ContentIndexPage(context.Context, domainevidence.RawArtifactContentIndexPageDescriptorV1) (domainevidence.RawArtifactContentIndexPageV1, error)
	OpenChunk(context.Context, domainevidence.RawArtifactContentChunkDescriptorV1) (io.ReadCloser, error)
}

// StagingStore persists exact canonical objects as inert private-CAS staging.
// Implementations must perform no-replace write plus exact readback before a
// method returns nil. Staged objects never establish currentness or factual
// authority, including when an admission attempt later fails.
type StagingStore interface {
	StageAcquisitionIntentExact(context.Context, domainevidence.RawArtifactAcquisitionIntentV1) error
	StageSourceLocatorExact(context.Context, domainevidence.RawArtifactSourceLocatorV1) error
	StageChunkExact(context.Context, domainevidence.RawArtifactContentChunkDescriptorV1, []byte) error
	StageContentIndexPageExact(context.Context, domainevidence.RawArtifactContentIndexPageV1) error
	StageContentRootExact(context.Context, domainevidence.RawArtifactContentRootV1) error
	StageEntryExact(context.Context, domainevidence.RawArtifactEntryV1) error
	StageManifestPageExact(context.Context, domainevidence.RawArtifactManifestPageV1) error
	StageManifestExact(context.Context, domainevidence.RawArtifactManifestV1) error
}

// ExactObjectReferenceV1 carries both a logical contract digest and the
// physical canonical byte address. Neither field alone is sufficient.
type ExactObjectReferenceV1 struct {
	Digest     string
	SHA256     string
	ByteLength uint64
}

// ExactReader resolves only caller-supplied parent references. It exposes no
// enumeration or current-state selector; the app layer must compare every
// returned value to its exact parent and current witnessed binding.
type ExactReader interface {
	ResolveAcquisitionIntentExact(context.Context, domainevidence.RawArtifactManifestV1) (domainevidence.RawArtifactAcquisitionIntentV1, error)
	ResolveManifestExact(context.Context, ExactObjectReferenceV1) (domainevidence.RawArtifactManifestV1, error)
	ResolveManifestPageExact(context.Context, domainevidence.RawArtifactManifestPageDescriptorV1) (domainevidence.RawArtifactManifestPageV1, error)
	ResolveEntryExact(context.Context, ExactObjectReferenceV1) (domainevidence.RawArtifactEntryV1, error)
	ResolveSourceLocatorExact(context.Context, domainevidence.RawArtifactEntryV1) (domainevidence.RawArtifactSourceLocatorV1, error)
	ResolveContentRootExact(context.Context, domainevidence.RawArtifactEntryV1) (domainevidence.RawArtifactContentRootV1, error)
	ResolveContentIndexPageExact(context.Context, domainevidence.RawArtifactContentIndexPageDescriptorV1) (domainevidence.RawArtifactContentIndexPageV1, error)
	ResolveChunkExact(context.Context, domainevidence.RawArtifactContentChunkDescriptorV1) ([]byte, error)
}
