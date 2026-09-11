package piiauthorization

import (
	"context"
	"io"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// AccessStore is the private, append-only audit authority for controlled
// artifact access. Receipts are keyed by AccessID and dispositions are also
// keyed by AccessID so one access cannot acquire conflicting terminal states.
type AccessStore interface {
	ReserveAccessReceipt(context.Context, domainpii.ControlledArtifactAccessReceiptV1) error
	ResolveAccessReceipt(context.Context, string) (domainpii.ControlledArtifactAccessReceiptV1, error)
	PutAccessDispositionIfAbsent(context.Context, domainpii.ControlledArtifactAccessDispositionV1) error
	ResolveAccessDisposition(context.Context, string) (domainpii.ControlledArtifactAccessDispositionV1, error)
}

// RestartAccessStore exposes whether recovery writes are confined to the
// disposable semantic-startup stage. A live store may verify an already
// closed inventory, but it cannot terminalize open access reservations.
type RestartAccessStore interface {
	AccessStore
	IsSemanticStageControlledAccessStore() bool
}

type AccessInventoryStore interface {
	VisitAccessReceipts(context.Context, func(domainpii.ControlledArtifactAccessReceiptV1) error) error
	VisitAccessDispositions(context.Context, func(domainpii.ControlledArtifactAccessDispositionV1) error) error
}

// AccessStoreV2 is physically and contractually isolated from the legacy V1
// journal. A V2 reservation binds the use slot to one exact projected delivery
// outcome; the same slot cannot be rebound even when the outcome later changes.
type AccessStoreV2 interface {
	ReserveAccessReceiptV2(context.Context, domainpii.ControlledArtifactAccessReceiptV2) error
	ResolveAccessReceiptV2(context.Context, string) (domainpii.ControlledArtifactAccessReceiptV2, error)
	PutAccessDispositionIfAbsentV2(context.Context, domainpii.ControlledArtifactAccessDispositionV2) error
	ResolveAccessDispositionV2(context.Context, string) (domainpii.ControlledArtifactAccessDispositionV2, error)
}

// RestartAccessStoreV2 exposes the disposable semantic-stage boundary for V2
// recovery. Production stores must never acquire restart terminalization
// authority after the runtime becomes live.
type RestartAccessStoreV2 interface {
	AccessStoreV2
	IsSemanticStageControlledAccessStoreV2() bool
}

type AccessInventoryStoreV2 interface {
	VisitAccessReceiptsV2(context.Context, func(domainpii.ControlledArtifactAccessReceiptV2) error) error
	VisitAccessDispositionsV2(context.Context, func(domainpii.ControlledArtifactAccessDispositionV2) error) error
}

// ControlledAccessAdmissionRequestV1 carries only ephemeral host tokens and
// the already-frozen turn context. The tokens must never be persisted or
// returned by the app service.
type ControlledAccessAdmissionRequestV1 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	AccessAction       string
	ControlledHandle   string
	UseSlot            string
	RendererPrincipal  string
	RendererGeneration uint64
	BackendGeneration  uint64
}

// ControlledAccessAdmissionV1 is reconstructed by the desktop authority from
// its current opaque-handle and renderer-principal registries. Hashes supplied
// by an IPC caller are not admission authority.
type ControlledAccessAdmissionV1 struct {
	SecurityContext         domainsecurity.TurnSecurityContext
	AccessAction            string
	ControlledHandleDigest  string
	UseSlotDigest           string
	RendererPrincipalDigest string
	RendererGeneration      uint64
	BackendGeneration       uint64
	PublicationCommitDigest string
	AuthorizedUntil         time.Time
}

// ControlledAccessAdmissionAuthority validates the current main-frame
// principal, backend generation, one-use slot, action, and opaque publication
// handle. ResolveCurrent must be idempotent so the app can revalidate at the
// final release boundary without consuming the slot itself.
type ControlledAccessAdmissionAuthority interface {
	ResolveCurrent(context.Context, ControlledAccessAdmissionRequestV1) (ControlledAccessAdmissionV1, error)
}

// ControlledReleaseRequestV1 is delivered only to a host-owned trusted sink.
// Release must consume Body synchronously and must not retain the reader after
// returning. The app never returns Body, a filesystem path, or raw PII.
type ControlledReleaseRequestV1 struct {
	Receipt            domainpii.ControlledArtifactAccessReceiptV1
	Body               io.Reader
	ArtifactByteLength uint64
}

// ControlledReleaseResultV1 is intentionally small. A release is accepted as
// committed only when the sink returns nil, Committed=true, and the exact byte
// length. Every other post-call outcome is conservatively indeterminate.
type ControlledReleaseResultV1 struct {
	Committed          bool
	ReleasedByteLength uint64
}

type ControlledReleaseSink interface {
	Release(context.Context, ControlledReleaseRequestV1) (ControlledReleaseResultV1, error)
}

// ControlledAccessAdmissionRequestV2 is deliberately a distinct type from
// V1 so a legacy admission authority cannot satisfy the projected-delivery
// release contract by structural accident.
type ControlledAccessAdmissionRequestV2 struct {
	SecurityContext    domainsecurity.TurnSecurityContext
	AccessAction       string
	ControlledHandle   string
	UseSlot            string
	RendererPrincipal  string
	RendererGeneration uint64
	BackendGeneration  uint64
}

// ControlledAccessAdmissionV2 binds the host's opaque handle to the exact
// no-replace delivery winner, not merely to a publication commit.
type ControlledAccessAdmissionV2 struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	AccessAction                string
	ControlledHandleDigest      string
	UseSlotDigest               string
	RendererPrincipalDigest     string
	RendererGeneration          uint64
	BackendGeneration           uint64
	DeliveryID                  string
	DeliveryOutcomeRecordDigest string
	PublicationCommitDigest     string
	ReleaseTargetIdentityDigest string
	AuthorizedUntil             time.Time
}

type ControlledAccessAdmissionAuthorityV2 interface {
	ResolveCurrentV2(context.Context, ControlledAccessAdmissionRequestV2) (ControlledAccessAdmissionV2, error)
}

// ControlledReleaseRequestV2 carries only the signed V2 audit receipt and an
// in-memory reader. The trusted host must consume Body synchronously.
type ControlledReleaseRequestV2 struct {
	Receipt            domainpii.ControlledArtifactAccessReceiptV2
	Body               io.Reader
	ArtifactByteLength uint64
}

type ControlledReleaseResultV2 struct {
	Committed          bool
	ReleasedByteLength uint64
}

type ControlledReleaseSinkV2 interface {
	ReleaseV2(context.Context, ControlledReleaseRequestV2) (ControlledReleaseResultV2, error)
}
