//go:build analytix_native_build_probe && !analytix_prod

package nativebuild

import (
	"context"

	domainnativebuild "analytix.local/runtime-go/internal/domain/nativebuild"
)

// EnvironmentAssessmentV1 is host-derived preflight state. A blocked result
// is terminal and must be returned before a snapshot, Cargo process, staging
// directory, or publication effect is created.
type EnvironmentAssessmentV1 struct {
	Ready   bool
	Blocker string
}

type EffectDispositionV1 string

const (
	EffectNoneV1          EffectDispositionV1 = "none"
	EffectOwnedV1         EffectDispositionV1 = "owned"
	EffectIndeterminateV1 EffectDispositionV1 = "indeterminate"
)

// SnapshotLease and ArtifactLease are opaque, process-local capabilities. A
// host adapter must keep their descriptor identities private and must not use
// a caller path as authority.
type SnapshotLease interface {
	NativeBuildSnapshotLeaseV1()
}

type ArtifactLease interface {
	NativeBuildArtifactLeaseV1()
}

type BuildSession interface {
	NativeBuildSessionV1()
}

type PublicationResultV1 struct {
	RequestNonce             string
	PublicationBindingSHA256 string
	GenerationID             string
	InventorySHA256          string
	GenerationReceiptSHA256  string
	ComponentReceiptSHA256   string
}

type PublicationStateV1 string

const (
	PublicationNotCommittedV1  PublicationStateV1 = "not_committed"
	PublicationCommittedV1     PublicationStateV1 = "committed"
	PublicationIndeterminateV1 PublicationStateV1 = "indeterminate"
)

type PublicationOutcomeV1 struct {
	State  PublicationStateV1
	Result PublicationResultV1
}

type SnapshotStartV1 struct {
	Disposition EffectDispositionV1
	Lease       SnapshotLease
}

type PreflightResultV1 struct {
	Disposition EffectDispositionV1
	Assessment  EnvironmentAssessmentV1
	Session     BuildSession
}

type CargoPlanResultV1 struct {
	Disposition EffectDispositionV1
	Receipt     domainnativebuild.CargoExecutionReceiptV1
	Artifacts   ArtifactLease
}

// Host is the only side-effect boundary used by the build coordinator. Its
// production implementation fixes target, component inventory, Cargo plan,
// environment, toolchain, and publication destination from descriptor-bound
// authority rather than caller JSON.
type Host interface {
	Preflight(context.Context, string) (PreflightResultV1, error)
	ReconcileSessionStart(context.Context, string) error
	CloseSession(context.Context, BuildSession) error
	ReconcileSessionClose(context.Context, BuildSession, string) error
	BeginSnapshot(context.Context, BuildSession) (SnapshotStartV1, error)
	ReconcileSnapshotStart(context.Context, BuildSession, string) error
	SnapshotBinding(SnapshotLease) (domainnativebuild.SourceSnapshotBindingV1, bool)
	ExecuteCargoPlan(
		context.Context,
		BuildSession,
		SnapshotLease,
	) (CargoPlanResultV1, error)
	ReconcileCargoExecution(context.Context, BuildSession, SnapshotLease, string) error
	VerifySnapshot(context.Context, SnapshotLease) error
	DiscardSnapshot(context.Context, SnapshotLease) error
	ReconcileDiscard(context.Context, SnapshotLease) error
	AbortArtifacts(context.Context, ArtifactLease) error
	ReconcileArtifactAbort(context.Context, ArtifactLease, string) error
	Publish(
		context.Context,
		ArtifactLease,
		domainnativebuild.PublicationPermitV1,
		domainnativebuild.PublicationBindingV1,
	) (PublicationOutcomeV1, error)
	// ObservePublication is strictly read-only. It must not recover, roll back,
	// delete, create, rename, or fsync publication state.
	ObservePublication(
		context.Context,
		string,
		domainnativebuild.PublicationBindingV1,
	) (PublicationOutcomeV1, error)
}
