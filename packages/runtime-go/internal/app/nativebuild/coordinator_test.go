//go:build analytix_native_build_probe && !analytix_prod

package nativebuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	domainnativebuild "analytix.local/runtime-go/internal/domain/nativebuild"
	portnativebuild "analytix.local/runtime-go/internal/ports/nativebuild"
)

func TestCoordinatorBlocksBeforeEveryEffectWhenPreflightIsNotReady(t *testing.T) {
	host := &fakeHostV1{
		assessment:           portnativebuild.EnvironmentAssessmentV1{Blocker: "toolchain_user_writable"},
		preflightDisposition: portnativebuild.EffectNoneV1,
	}
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if err != nil {
		t.Fatalf("blocked preflight: %v", err)
	}
	if result.Status != StatusBlocked || result.Blocker != "toolchain_user_writable" {
		t.Fatalf("blocked result = %#v", result)
	}
	if !reflect.DeepEqual(result.CargoExecutionReceipt, domainnativebuild.CargoExecutionReceiptV1{}) {
		t.Fatalf("blocked preflight invented a Cargo execution receipt: %#v", result.CargoExecutionReceipt)
	}
	if !reflect.DeepEqual(host.calls, []string{"preflight"}) {
		t.Fatalf("blocked preflight effects = %v", host.calls)
	}
}

func TestCoordinatorMintsPermitOnlyAfterVerifiedSnapshotCleanup(t *testing.T) {
	host := successfulFakeHostV1()
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if err != nil {
		t.Fatalf("run coordinator: %v", err)
	}
	if result.Status != StatusPublished || result.Blocker != "" || !result.CargoExecutionReceipt.ReleaseEligible ||
		!result.CargoExecutionReceipt.Containment.TemporaryStateDestroyed {
		t.Fatalf("published result = %#v", result)
	}
	want := []string{"preflight", "begin_snapshot", "snapshot_binding", "execute", "verify", "discard", "close_session", "publish"}
	if !reflect.DeepEqual(host.calls, want) {
		t.Fatalf("call order = %v, want %v", host.calls, want)
	}
	if host.publishCount != 1 || host.abortCount != 0 {
		t.Fatalf("publish=%d abort=%d", host.publishCount, host.abortCount)
	}
}

func TestCoordinatorUsesReconcileWithoutUpgradingSnapshotEvidence(t *testing.T) {
	host := successfulFakeHostV1()
	host.discardErr = errors.New("discard response lost")
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if err != nil || result.Status != StatusPublished {
		t.Fatalf("reconciled run result=%#v err=%v", result, err)
	}
	want := []string{"preflight", "begin_snapshot", "snapshot_binding", "execute", "verify", "discard", "reconcile", "close_session", "publish"}
	if !reflect.DeepEqual(host.calls, want) {
		t.Fatalf("reconcile call order = %v, want %v", host.calls, want)
	}
}

func TestCoordinatorNeverPublishesWhenCleanupIsIndeterminate(t *testing.T) {
	host := successfulFakeHostV1()
	host.discardErr = errors.New("discard failed")
	host.reconcileErr = errors.New("reconcile failed")
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, ErrCleanupIndeterminate) || result.Status != StatusFailed || result.Blocker != "build_cleanup_indeterminate" {
		t.Fatalf("cleanup result=%#v err=%v", result, err)
	}
	if host.publishCount != 0 || host.abortCount != 1 {
		t.Fatalf("cleanup failure publish=%d abort=%d", host.publishCount, host.abortCount)
	}
}

func TestCoordinatorNeverPublishesFailureCancelOrSnapshotDrift(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*fakeHostV1)
		wantBlocker string
	}{
		{
			name: "cargo failure",
			mutate: func(host *fakeHostV1) {
				host.executeErr = errors.New("cargo failed")
			},
			wantBlocker: "cargo_execution_failed",
		},
		{
			name: "snapshot drift",
			mutate: func(host *fakeHostV1) {
				host.verifyErr = errors.New("snapshot changed")
			},
			wantBlocker: "source_snapshot_unbound",
		},
		{
			name: "mismatched snapshot binding",
			mutate: func(host *fakeHostV1) {
				host.receipt.SourceSnapshot.GenerationID = digestV1("other-generation")
			},
			wantBlocker: "cargo_execution_failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := successfulFakeHostV1()
			test.mutate(host)
			coordinator := mustCoordinatorV1(t, host)
			result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
			if err == nil || result.Status != StatusFailed || result.Blocker != test.wantBlocker {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if host.publishCount != 0 || host.abortCount != 1 {
				t.Fatalf("publish=%d abort=%d", host.publishCount, host.abortCount)
			}
		})
	}
}

func TestCoordinatorDoesNotRetryPublicationOrRemintPermit(t *testing.T) {
	host := successfulFakeHostV1()
	host.publishErr = errors.New("commit response indeterminate")
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, ErrPublication) || result.Status != StatusFailed || result.Blocker != "publication_failed" {
		t.Fatalf("publication result=%#v err=%v", result, err)
	}
	if host.publishCount != 1 || host.abortCount != 1 {
		t.Fatalf("publication attempts=%d abort=%d", host.publishCount, host.abortCount)
	}
	if host.observePublicationCount != 1 {
		t.Fatalf("publication observations=%d, want 1", host.observePublicationCount)
	}
}

func TestCoordinatorResponseLossObservesExactCommitWithoutRetryOrAbort(t *testing.T) {
	host := successfulFakeHostV1()
	host.publishErr = errors.New("commit response lost")
	host.observePublicationOutcome = portnativebuild.PublicationOutcomeV1{State: portnativebuild.PublicationCommittedV1}
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if err != nil || result.Status != StatusPublished || result.Publication.RequestNonce != digestV1("request") {
		t.Fatalf("observed commit result=%#v err=%v", result, err)
	}
	if host.publishCount != 1 || host.observePublicationCount != 1 || host.abortCount != 0 {
		t.Fatalf("publish=%d observe=%d abort=%d", host.publishCount, host.observePublicationCount, host.abortCount)
	}
}

func TestPublicationRequiresComponentReceiptDigest(t *testing.T) {
	requestNonce := digestV1("request")
	bindingSHA256 := digestV1("binding")
	result := portnativebuild.PublicationResultV1{
		RequestNonce: requestNonce, PublicationBindingSHA256: bindingSHA256,
		GenerationID: digestV1("generation"), InventorySHA256: digestV1("inventory"),
		GenerationReceiptSHA256: digestV1("generation-receipt"),
		ComponentReceiptSHA256:  digestV1("component-receipt"),
	}
	if !validPublicationV1(result, requestNonce, bindingSHA256) {
		t.Fatal("complete publication receipt was rejected")
	}
	result.ComponentReceiptSHA256 = ""
	if validPublicationV1(result, requestNonce, bindingSHA256) {
		t.Fatal("publication without component receipt digest was accepted")
	}
}

func TestCoordinatorUnknownOrMismatchedCommitPerformsNoMutation(t *testing.T) {
	tests := []struct {
		name    string
		outcome portnativebuild.PublicationOutcomeV1
		err     error
	}{
		{
			name:    "indeterminate",
			outcome: portnativebuild.PublicationOutcomeV1{State: portnativebuild.PublicationIndeterminateV1},
		},
		{
			name: "mismatched committed",
			outcome: portnativebuild.PublicationOutcomeV1{
				State: portnativebuild.PublicationCommittedV1,
				Result: portnativebuild.PublicationResultV1{
					RequestNonce: digestV1("other-request"), PublicationBindingSHA256: digestV1("other-binding"),
					GenerationID: digestV1("generation"), InventorySHA256: digestV1("inventory"),
					GenerationReceiptSHA256: digestV1("receipt"), ComponentReceiptSHA256: digestV1("component-receipt"),
				},
			},
		},
		{
			name: "observation failure",
			err:  errors.New("read-only observation failed"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := successfulFakeHostV1()
			host.publishErr = errors.New("publish response unavailable")
			host.observePublicationOutcome = test.outcome
			host.observePublicationErr = test.err
			coordinator := mustCoordinatorV1(t, host)
			result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
			if !errors.Is(err, ErrPublication) || result.Status != StatusFailed || result.Blocker != "publication_commit_indeterminate" {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if host.publishCount != 1 || host.observePublicationCount != 1 || host.abortCount != 0 {
				t.Fatalf("publish=%d observe=%d abort=%d", host.publishCount, host.observePublicationCount, host.abortCount)
			}
		})
	}
}

func TestCoordinatorDefiniteNoCommitAbortFailureIsCleanupIndeterminate(t *testing.T) {
	host := successfulFakeHostV1()
	host.publishErr = errors.New("publish failed")
	host.abortErr = errors.New("abort response lost")
	host.reconcileArtifactAbortErr = errors.New("artifact cleanup unprovable")
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, ErrCleanupIndeterminate) || result.Status != StatusFailed || result.Blocker != "build_cleanup_indeterminate" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if host.abortCount != 1 || host.observePublicationCount != 1 {
		t.Fatalf("abort=%d observe=%d", host.abortCount, host.observePublicationCount)
	}
}

func TestCoordinatorFinalizeFailurePlusAbortFailureIsCleanupIndeterminate(t *testing.T) {
	host := successfulFakeHostV1()
	host.receipt.Outputs[0].PayloadSHA256 = "invalid"
	host.abortErr = errors.New("abort failed")
	host.reconcileArtifactAbortErr = errors.New("abort reconcile failed")
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(context.Background(), RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, ErrCleanupIndeterminate) || result.Blocker != "build_cleanup_indeterminate" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if host.publishCount != 0 || host.abortCount != 1 {
		t.Fatalf("publish=%d abort=%d", host.publishCount, host.abortCount)
	}
}

func TestCoordinatorCancellationAfterCleanupBeforeMintNeverPublishes(t *testing.T) {
	host := successfulFakeHostV1()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host.closeSessionHook = cancel
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(ctx, RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, ErrExecution) || result.Blocker != "cargo_execution_cancelled" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if host.publishCount != 0 || host.abortCount != 1 {
		t.Fatalf("publish=%d abort=%d", host.publishCount, host.abortCount)
	}
}

func TestCoordinatorTimeoutAfterSessionCloseNeverPublishes(t *testing.T) {
	host := successfulFakeHostV1()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	host.closeSessionHook = func() { <-ctx.Done() }
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(ctx, RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, context.DeadlineExceeded) || result.Blocker != "cargo_execution_timeout" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if host.publishCount != 0 || host.abortCount != 1 {
		t.Fatalf("publish=%d abort=%d", host.publishCount, host.abortCount)
	}
}

func TestCoordinatorCancellationDuringVerifyKeepsCancellationBlocker(t *testing.T) {
	host := successfulFakeHostV1()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host.verifyHook = cancel
	host.verifyErr = context.Canceled
	coordinator := mustCoordinatorV1(t, host)
	result, err := coordinator.RunV1(ctx, RequestV1{RequestNonce: digestV1("request")})
	if !errors.Is(err, context.Canceled) || result.Blocker != "cargo_execution_cancelled" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if host.publishCount != 0 || host.abortCount != 1 {
		t.Fatalf("publish=%d abort=%d", host.publishCount, host.abortCount)
	}
}

type fakeSnapshotLeaseV1 struct{}

func (*fakeSnapshotLeaseV1) NativeBuildSnapshotLeaseV1() {}

type fakeArtifactLeaseV1 struct{}

func (*fakeArtifactLeaseV1) NativeBuildArtifactLeaseV1() {}

type fakeBuildSessionV1 struct{}

func (*fakeBuildSessionV1) NativeBuildSessionV1() {}

type fakeHostV1 struct {
	assessment                 portnativebuild.EnvironmentAssessmentV1
	preflightDisposition       portnativebuild.EffectDispositionV1
	preflightErr               error
	reconcileSessionStartErr   error
	closeSessionErr            error
	reconcileSessionCloseErr   error
	closeSessionHook           func()
	beginDisposition           portnativebuild.EffectDispositionV1
	beginErr                   error
	reconcileSnapshotStartErr  error
	binding                    domainnativebuild.SourceSnapshotBindingV1
	bindingKnown               bool
	receipt                    domainnativebuild.CargoExecutionReceiptV1
	executeDisposition         portnativebuild.EffectDispositionV1
	executeErr                 error
	reconcileCargoExecutionErr error
	verifyErr                  error
	verifyHook                 func()
	discardErr                 error
	reconcileErr               error
	abortErr                   error
	reconcileArtifactAbortErr  error
	publishErr                 error
	observePublicationOutcome  portnativebuild.PublicationOutcomeV1
	observePublicationErr      error
	calls                      []string
	publishCount               int
	abortCount                 int
	observePublicationCount    int
}

func successfulFakeHostV1() *fakeHostV1 {
	binding := domainnativebuild.SourceSnapshotBindingV1{
		GenerationID: digestV1("snapshot-generation"), InventorySHA256: digestV1("snapshot-inventory"),
		GenerationReceiptSHA256: digestV1("snapshot-receipt"),
	}
	return &fakeHostV1{
		assessment:           portnativebuild.EnvironmentAssessmentV1{Ready: true},
		preflightDisposition: portnativebuild.EffectOwnedV1,
		beginDisposition:     portnativebuild.EffectOwnedV1,
		binding:              binding,
		bindingKnown:         true,
		executeDisposition:   portnativebuild.EffectOwnedV1,
		receipt:              controlledReceiptFixtureV1(binding, digestV1("request")),
		observePublicationOutcome: portnativebuild.PublicationOutcomeV1{
			State: portnativebuild.PublicationNotCommittedV1,
		},
	}
}

func (host *fakeHostV1) Preflight(context.Context, string) (portnativebuild.PreflightResultV1, error) {
	host.calls = append(host.calls, "preflight")
	var session portnativebuild.BuildSession
	if host.preflightDisposition == portnativebuild.EffectOwnedV1 {
		session = &fakeBuildSessionV1{}
	}
	return portnativebuild.PreflightResultV1{
		Disposition: host.preflightDisposition,
		Assessment:  host.assessment,
		Session:     session,
	}, host.preflightErr
}

func (host *fakeHostV1) ReconcileSessionStart(context.Context, string) error {
	host.calls = append(host.calls, "reconcile_session_start")
	return host.reconcileSessionStartErr
}

func (host *fakeHostV1) CloseSession(context.Context, portnativebuild.BuildSession) error {
	host.calls = append(host.calls, "close_session")
	if host.closeSessionHook != nil {
		host.closeSessionHook()
	}
	return host.closeSessionErr
}

func (host *fakeHostV1) ReconcileSessionClose(context.Context, portnativebuild.BuildSession, string) error {
	host.calls = append(host.calls, "reconcile_session_close")
	return host.reconcileSessionCloseErr
}

func (host *fakeHostV1) BeginSnapshot(context.Context, portnativebuild.BuildSession) (portnativebuild.SnapshotStartV1, error) {
	host.calls = append(host.calls, "begin_snapshot")
	var lease portnativebuild.SnapshotLease
	if host.beginDisposition == portnativebuild.EffectOwnedV1 {
		lease = &fakeSnapshotLeaseV1{}
	}
	return portnativebuild.SnapshotStartV1{Disposition: host.beginDisposition, Lease: lease}, host.beginErr
}

func (host *fakeHostV1) ReconcileSnapshotStart(context.Context, portnativebuild.BuildSession, string) error {
	host.calls = append(host.calls, "reconcile_snapshot_start")
	return host.reconcileSnapshotStartErr
}

func (host *fakeHostV1) SnapshotBinding(portnativebuild.SnapshotLease) (domainnativebuild.SourceSnapshotBindingV1, bool) {
	host.calls = append(host.calls, "snapshot_binding")
	return host.binding, host.bindingKnown
}

func (host *fakeHostV1) ExecuteCargoPlan(
	context.Context,
	portnativebuild.BuildSession,
	portnativebuild.SnapshotLease,
) (portnativebuild.CargoPlanResultV1, error) {
	host.calls = append(host.calls, "execute")
	var artifacts portnativebuild.ArtifactLease
	if host.executeDisposition == portnativebuild.EffectOwnedV1 {
		artifacts = &fakeArtifactLeaseV1{}
	}
	return portnativebuild.CargoPlanResultV1{
		Disposition: host.executeDisposition,
		Receipt:     host.receipt,
		Artifacts:   artifacts,
	}, host.executeErr
}

func (host *fakeHostV1) ReconcileCargoExecution(
	context.Context,
	portnativebuild.BuildSession,
	portnativebuild.SnapshotLease,
	string,
) error {
	host.calls = append(host.calls, "reconcile_cargo_execution")
	return host.reconcileCargoExecutionErr
}

func (host *fakeHostV1) VerifySnapshot(context.Context, portnativebuild.SnapshotLease) error {
	host.calls = append(host.calls, "verify")
	if host.verifyHook != nil {
		host.verifyHook()
	}
	return host.verifyErr
}

func (host *fakeHostV1) DiscardSnapshot(context.Context, portnativebuild.SnapshotLease) error {
	host.calls = append(host.calls, "discard")
	return host.discardErr
}

func (host *fakeHostV1) ReconcileDiscard(context.Context, portnativebuild.SnapshotLease) error {
	host.calls = append(host.calls, "reconcile")
	return host.reconcileErr
}

func (host *fakeHostV1) AbortArtifacts(context.Context, portnativebuild.ArtifactLease) error {
	host.calls = append(host.calls, "abort_artifacts")
	host.abortCount++
	return host.abortErr
}

func (host *fakeHostV1) ReconcileArtifactAbort(context.Context, portnativebuild.ArtifactLease, string) error {
	host.calls = append(host.calls, "reconcile_artifact_abort")
	return host.reconcileArtifactAbortErr
}

func (host *fakeHostV1) Publish(
	_ context.Context,
	_ portnativebuild.ArtifactLease,
	permit domainnativebuild.PublicationPermitV1,
	binding domainnativebuild.PublicationBindingV1,
) (portnativebuild.PublicationOutcomeV1, error) {
	host.calls = append(host.calls, "publish")
	host.publishCount++
	if err := domainnativebuild.ConsumeExactPublicationPermitV1(permit, binding, binding.Outputs); err != nil {
		return portnativebuild.PublicationOutcomeV1{}, err
	}
	if host.publishErr != nil {
		return portnativebuild.PublicationOutcomeV1{}, host.publishErr
	}
	bindingSHA256, err := domainnativebuild.PublicationBindingSHA256V1(binding)
	if err != nil {
		return portnativebuild.PublicationOutcomeV1{}, err
	}
	return portnativebuild.PublicationOutcomeV1{
		State: portnativebuild.PublicationCommittedV1,
		Result: portnativebuild.PublicationResultV1{
			RequestNonce: binding.RequestNonce, PublicationBindingSHA256: bindingSHA256,
			GenerationID: digestV1("published-generation"), InventorySHA256: digestV1("published-inventory"),
			GenerationReceiptSHA256: digestV1("published-receipt"), ComponentReceiptSHA256: digestV1("published-component-receipt"),
		},
	}, nil
}

func (host *fakeHostV1) ObservePublication(
	_ context.Context,
	_ string,
	binding domainnativebuild.PublicationBindingV1,
) (portnativebuild.PublicationOutcomeV1, error) {
	host.calls = append(host.calls, "observe_publication")
	host.observePublicationCount++
	outcome := host.observePublicationOutcome
	if outcome.State == portnativebuild.PublicationCommittedV1 &&
		reflect.DeepEqual(outcome.Result, portnativebuild.PublicationResultV1{}) {
		bindingSHA256, err := domainnativebuild.PublicationBindingSHA256V1(binding)
		if err != nil {
			return portnativebuild.PublicationOutcomeV1{}, err
		}
		outcome.Result = portnativebuild.PublicationResultV1{
			RequestNonce: binding.RequestNonce, PublicationBindingSHA256: bindingSHA256,
			GenerationID: digestV1("observed-generation"), InventorySHA256: digestV1("observed-inventory"),
			GenerationReceiptSHA256: digestV1("observed-receipt"), ComponentReceiptSHA256: digestV1("observed-component-receipt"),
		}
	}
	return outcome, host.observePublicationErr
}

func controlledReceiptFixtureV1(snapshot domainnativebuild.SourceSnapshotBindingV1, requestNonce string) domainnativebuild.CargoExecutionReceiptV1 {
	outputs := make([]domainnativebuild.CargoOutputV1, 0, 4)
	for _, id := range []string{"import-accelerator", "cleaning-ops", "analysis-compute", "data-engine"} {
		outputs = append(outputs, domainnativebuild.CargoOutputV1{
			SchemaVersion: 1, ID: id, SourceDigest: digestV1(id + "-source"), CargoLockSHA256: digestV1(id + "-lock"),
			BuildEnvironmentSHA256: digestV1(id + "-environment"),
			RawBuildSHA256:         digestV1(id + "-binary"), RawBuildSize: 1024,
			PayloadSHA256: digestV1(id + "-payload"), PayloadSize: 768, Format: "mach-o", Arch: "arm64",
		})
	}
	return domainnativebuild.CargoExecutionReceiptV1{
		SchemaVersion: domainnativebuild.CargoExecutionSchemaVersionV1,
		Purpose:       domainnativebuild.CargoExecutionPurposeV1,
		RequestNonce:  requestNonce,
		Status:        domainnativebuild.CargoStatusSucceeded,
		TrustClass:    domainnativebuild.CargoTrustControlledRelease,
		TargetKey:     "darwin-arm64", TargetTriple: "aarch64-apple-darwin",
		ManifestSHA256: digestV1("manifest"), SourceSetSHA256: digestV1("source-set"), SourceSnapshot: snapshot,
		BuildPlanSHA256: digestV1("build-plan"), BuildEnvironmentSHA256: digestV1("build-environment"),
		Toolchain: domainnativebuild.CargoToolchainIdentityV1{
			CargoExecutableSHA256: digestV1("cargo"), CargoVersion: "cargo 1.94.1",
			RustcExecutableSHA256: digestV1("rustc"), RustcVersion: "rustc 1.94.1",
			TreeSHA256: digestV1("tree"), TreeFileCount: 42, CompilerSHA256: digestV1("compiler"),
			ArchiverSHA256: digestV1("archiver"), LinkerSHA256: digestV1("linker"), PlatformSignerSHA256: digestV1("signer"),
			SDKIdentitySHA256: digestV1("sdk"), OwnershipClass: domainnativebuild.ToolchainOwnershipImmutableRelease,
		},
		Containment: domainnativebuild.CargoContainmentV1{
			SourceDescriptorsHeld: true, ToolchainDescriptorsHeld: true, LoadedImagesBound: true,
			WorkingDirectoriesBound: true, TargetWriteScopeBound: true, NetworkDenied: true, DeadlineBound: true,
			ProcessTreeEmpty: true, OutputsDescriptorBound: true, RawBuildDescriptorsHeld: true, PayloadIdentityBound: true,
			TemporaryStateDestroyed: false,
			SameUIDMutationDenied:   true, ProcessContainmentBound: true, DependencyClosureBound: true, SDKClosureBound: true,
		},
		Outputs: outputs, OutputInventorySHA256: domainnativebuild.CargoOutputInventorySHA256V1(outputs),
		Authority: domainnativebuild.CargoExecutionAuthorityV1{
			Protocol: domainnativebuild.CargoExecutionAuthorityProtocol, TargetKey: "darwin-arm64",
			BinarySHA256: digestV1("authority-binary"), SourceSetSHA256: digestV1("authority-source"),
			BuildEnvironmentSHA256: digestV1("authority-environment"), GoExecutableSHA256: digestV1("go"),
		},
	}
}

func mustCoordinatorV1(t *testing.T, host portnativebuild.Host) *Coordinator {
	t.Helper()
	coordinator, err := NewCoordinator(host)
	if err != nil {
		t.Fatalf("new coordinator: %v", err)
	}
	return coordinator
}

func digestV1(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
