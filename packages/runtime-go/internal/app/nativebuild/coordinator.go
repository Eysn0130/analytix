//go:build analytix_native_build_probe && !analytix_prod

package nativebuild

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"time"

	domainnativebuild "analytix.local/runtime-go/internal/domain/nativebuild"
	portnativebuild "analytix.local/runtime-go/internal/ports/nativebuild"
)

const (
	StatusBlocked     StatusV1 = "blocked"
	StatusFailed      StatusV1 = "failed"
	StatusPublished   StatusV1 = "published"
	cleanupDeadlineV1          = 30 * time.Second
)

var (
	ErrRequestInvalid       = errors.New("native_build_request_invalid")
	ErrPreflightInvalid     = errors.New("native_build_preflight_invalid")
	ErrSnapshot             = errors.New("native_build_snapshot_failed")
	ErrExecution            = errors.New("native_build_execution_failed")
	ErrCleanupIndeterminate = errors.New("native_build_cleanup_indeterminate")
	ErrPublication          = errors.New("native_build_publication_failed")
)

type StatusV1 string

type RequestV1 struct {
	RequestNonce string
}

type ResultV1 struct {
	Status                StatusV1
	Blocker               string
	CargoExecutionReceipt domainnativebuild.CargoExecutionReceiptV1
	Publication           portnativebuild.PublicationResultV1
}

type Coordinator struct {
	host portnativebuild.Host
}

func NewCoordinator(host portnativebuild.Host) (*Coordinator, error) {
	if capabilityNilV1(host) {
		return nil, ErrRequestInvalid
	}
	return &Coordinator{host: host}, nil
}

// RunV1 is the only owner of the release-capable sequence. It begins a live
// observation before host execution, proves snapshot cleanup before minting a
// permit, and never retries publication after the permit is consumed.
func (coordinator *Coordinator) RunV1(ctx context.Context, request RequestV1) (result ResultV1, returnErr error) {
	if coordinator == nil || capabilityNilV1(coordinator.host) || ctx == nil || ctx.Err() != nil ||
		!validDigestV1(request.RequestNonce) {
		return ResultV1{}, ErrRequestInvalid
	}

	observer := domainnativebuild.BeginCargoExecutionObservationV1()
	preflight, err := coordinator.host.Preflight(ctx, request.RequestNonce)
	if err != nil || !validPreflightResultV1(preflight) {
		cleanupErr := reconcilePreflightStartV1(ctx, coordinator.host, request.RequestNonce, preflight)
		if cleanupErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, err, cleanupErr)
		}
		return failedResultV1("controlled_environment_unproven"), errors.Join(ErrPreflightInvalid, err)
	}
	assessment := preflight.Assessment
	if !assessment.Ready {
		return ResultV1{Status: StatusBlocked, Blocker: assessment.Blocker}, nil
	}
	session := preflight.Session
	sessionClosed := false
	defer func() {
		if sessionClosed {
			return
		}
		closeErr := closeSessionAfterV1(ctx, coordinator.host, session, request.RequestNonce)
		sessionClosed = true
		if closeErr != nil {
			result = failedResultV1("build_cleanup_indeterminate")
			returnErr = errors.Join(returnErr, ErrCleanupIndeterminate, closeErr)
		}
	}()

	snapshotStart, err := coordinator.host.BeginSnapshot(ctx, session)
	if err != nil || !validSnapshotStartV1(snapshotStart) || snapshotStart.Disposition != portnativebuild.EffectOwnedV1 {
		cleanupErr := reconcileSnapshotStartV1(ctx, coordinator.host, session, request.RequestNonce, snapshotStart)
		if cleanupErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, err, cleanupErr)
		}
		return snapshotFailureV1(ctx, err)
	}
	snapshot := snapshotStart.Lease
	snapshotBinding, bindingKnown := coordinator.host.SnapshotBinding(snapshot)
	if !bindingKnown {
		cleanupErr := cleanupSnapshotAfterV1(ctx, coordinator.host, snapshot)
		if cleanupErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, cleanupErr)
		}
		return snapshotFailureV1(ctx, nil)
	}

	plan, executeErr := coordinator.host.ExecuteCargoPlan(ctx, session, snapshot)
	if executeErr != nil || !validCargoPlanResultV1(plan) || plan.Disposition != portnativebuild.EffectOwnedV1 ||
		!validUnsettledObservationV1(plan.Receipt, snapshotBinding, request.RequestNonce) {
		reconcileErr := reconcileCargoExecutionV1(ctx, coordinator.host, session, snapshot, request.RequestNonce, plan)
		cleanupErr := cleanupSnapshotAfterV1(ctx, coordinator.host, snapshot)
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, plan.Artifacts, request.RequestNonce)
		if reconcileErr != nil || cleanupErr != nil || artifactErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, executeErr, reconcileErr, cleanupErr, artifactErr)
		}
		return failedResultV1(executionBlockerV1(ctx)), errors.Join(ErrExecution, executeErr)
	}
	receipt := plan.Receipt
	artifacts := plan.Artifacts

	if verifyErr := coordinator.host.VerifySnapshot(ctx, snapshot); verifyErr != nil {
		cleanupErr := cleanupSnapshotAfterV1(ctx, coordinator.host, snapshot)
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
		if cleanupErr != nil || artifactErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, verifyErr, cleanupErr, artifactErr)
		}
		return snapshotFailureV1(ctx, verifyErr)
	}
	if cleanupErr := cleanupSnapshotAfterV1(ctx, coordinator.host, snapshot); cleanupErr != nil {
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
		return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, cleanupErr, artifactErr)
	}

	// The adapter must leave this false while the snapshot exists. Only the
	// coordinator may assert it after exact discard/reconcile has completed.
	receipt.Containment.TemporaryStateDestroyed = true
	receipt.ExecutionID = ""
	if closeErr := closeSessionAfterV1(ctx, coordinator.host, session, request.RequestNonce); closeErr != nil {
		sessionClosed = true
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
		return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, closeErr, artifactErr)
	}
	sessionClosed = true
	if terminal, terminalErr, stop := stopForTerminalContextV1(ctx, coordinator.host, artifacts, request.RequestNonce); stop {
		return terminal, terminalErr
	}
	finalized, live, err := domainnativebuild.FinalizeObservedCargoExecutionV1(observer, receipt)
	if err != nil {
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
		if artifactErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, err, artifactErr)
		}
		return failedResultV1("cargo_execution_failed"), errors.Join(ErrExecution, err, artifactErr)
	}
	if terminal, terminalErr, stop := stopForTerminalContextV1(ctx, coordinator.host, artifacts, request.RequestNonce); stop {
		return terminal, terminalErr
	}
	permit, binding, err := domainnativebuild.AuthorizeForPublicationV1(live, finalized)
	if err != nil {
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
		if artifactErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, err, artifactErr)
		}
		return failedResultV1("cargo_execution_failed"), errors.Join(ErrExecution, err, artifactErr)
	}
	bindingSHA256, err := domainnativebuild.PublicationBindingSHA256V1(binding)
	if err != nil {
		artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
		if artifactErr != nil {
			return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, err, artifactErr)
		}
		return failedResultV1("cargo_execution_failed"), errors.Join(ErrExecution, err)
	}
	if terminal, terminalErr, stop := stopForTerminalContextV1(ctx, coordinator.host, artifacts, request.RequestNonce); stop {
		return terminal, terminalErr
	}
	publication, publishErr := coordinator.host.Publish(ctx, artifacts, permit, binding)
	if publishErr == nil && validCommittedPublicationV1(publication, request.RequestNonce, bindingSHA256) {
		return ResultV1{
			Status: StatusPublished, CargoExecutionReceipt: finalized, Publication: publication.Result,
		}, nil
	}
	observed, observeErr := observePublicationV1(ctx, coordinator.host, request.RequestNonce, binding)
	if observeErr == nil {
		switch observed.State {
		case portnativebuild.PublicationCommittedV1:
			if !validPublicationV1(observed.Result, request.RequestNonce, bindingSHA256) {
				return failedResultV1("publication_commit_indeterminate"), errors.Join(ErrPublication, publishErr)
			}
			return ResultV1{
				Status: StatusPublished, CargoExecutionReceipt: finalized, Publication: observed.Result,
			}, nil
		case portnativebuild.PublicationNotCommittedV1:
			if !reflect.DeepEqual(observed.Result, portnativebuild.PublicationResultV1{}) {
				return failedResultV1("publication_commit_indeterminate"), errors.Join(ErrPublication, publishErr)
			}
			artifactErr := abortArtifactsAfterV1(ctx, coordinator.host, artifacts, request.RequestNonce)
			if artifactErr != nil {
				return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, publishErr, artifactErr)
			}
			return failedResultV1("publication_failed"), errors.Join(ErrPublication, publishErr)
		case portnativebuild.PublicationIndeterminateV1:
			return failedResultV1("publication_commit_indeterminate"), errors.Join(ErrPublication, publishErr)
		}
	}
	return failedResultV1("publication_commit_indeterminate"), errors.Join(ErrPublication, publishErr, observeErr)
}

func validSnapshotStartV1(start portnativebuild.SnapshotStartV1) bool {
	switch start.Disposition {
	case portnativebuild.EffectNoneV1:
		return capabilityNilV1(start.Lease)
	case portnativebuild.EffectOwnedV1:
		return !capabilityNilV1(start.Lease)
	case portnativebuild.EffectIndeterminateV1:
		return true
	default:
		return false
	}
}

func validPreflightResultV1(result portnativebuild.PreflightResultV1) bool {
	if !validAssessmentV1(result.Assessment) {
		return false
	}
	if result.Assessment.Ready {
		return result.Disposition == portnativebuild.EffectOwnedV1 && !capabilityNilV1(result.Session)
	}
	return result.Disposition == portnativebuild.EffectNoneV1 && capabilityNilV1(result.Session)
}

func validCargoPlanResultV1(result portnativebuild.CargoPlanResultV1) bool {
	switch result.Disposition {
	case portnativebuild.EffectNoneV1:
		return capabilityNilV1(result.Artifacts)
	case portnativebuild.EffectOwnedV1:
		return !capabilityNilV1(result.Artifacts)
	case portnativebuild.EffectIndeterminateV1:
		return true
	default:
		return false
	}
}

func reconcileSnapshotStartV1(
	parent context.Context,
	host portnativebuild.Host,
	session portnativebuild.BuildSession,
	requestNonce string,
	start portnativebuild.SnapshotStartV1,
) error {
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	var leaseErr error
	if !capabilityNilV1(start.Lease) {
		leaseErr = cleanupSnapshotV1(ctx, host, start.Lease)
	}
	if start.Disposition == portnativebuild.EffectNoneV1 && validSnapshotStartV1(start) {
		return leaseErr
	}
	return errors.Join(leaseErr, host.ReconcileSnapshotStart(ctx, session, requestNonce))
}

func reconcilePreflightStartV1(
	parent context.Context,
	host portnativebuild.Host,
	requestNonce string,
	result portnativebuild.PreflightResultV1,
) error {
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	var closeErr error
	if !capabilityNilV1(result.Session) {
		closeErr = host.CloseSession(ctx, result.Session)
	}
	if result.Disposition == portnativebuild.EffectNoneV1 && capabilityNilV1(result.Session) {
		return closeErr
	}
	return errors.Join(closeErr, host.ReconcileSessionStart(ctx, requestNonce))
}

func reconcileCargoExecutionV1(
	parent context.Context,
	host portnativebuild.Host,
	session portnativebuild.BuildSession,
	snapshot portnativebuild.SnapshotLease,
	requestNonce string,
	result portnativebuild.CargoPlanResultV1,
) error {
	if result.Disposition == portnativebuild.EffectNoneV1 && validCargoPlanResultV1(result) {
		return nil
	}
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	return host.ReconcileCargoExecution(ctx, session, snapshot, requestNonce)
}

func cleanupSnapshotAfterV1(parent context.Context, host portnativebuild.Host, snapshot portnativebuild.SnapshotLease) error {
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	return cleanupSnapshotV1(ctx, host, snapshot)
}

func closeSessionAfterV1(
	parent context.Context,
	host portnativebuild.Host,
	session portnativebuild.BuildSession,
	requestNonce string,
) error {
	if capabilityNilV1(session) {
		return ErrCleanupIndeterminate
	}
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	if err := host.CloseSession(ctx, session); err == nil {
		return nil
	}
	return host.ReconcileSessionClose(ctx, session, requestNonce)
}

func cleanupSnapshotV1(ctx context.Context, host portnativebuild.Host, snapshot portnativebuild.SnapshotLease) error {
	if capabilityNilV1(snapshot) {
		return nil
	}
	if err := host.DiscardSnapshot(ctx, snapshot); err == nil {
		return nil
	}
	if err := host.ReconcileDiscard(ctx, snapshot); err != nil {
		return err
	}
	return nil
}

func abortArtifactsV1(ctx context.Context, host portnativebuild.Host, artifacts portnativebuild.ArtifactLease) error {
	if capabilityNilV1(artifacts) {
		return nil
	}
	return host.AbortArtifacts(ctx, artifacts)
}

func abortArtifactsAfterV1(
	parent context.Context,
	host portnativebuild.Host,
	artifacts portnativebuild.ArtifactLease,
	requestNonce string,
) error {
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	if err := abortArtifactsV1(ctx, host, artifacts); err == nil {
		return nil
	}
	return host.ReconcileArtifactAbort(ctx, artifacts, requestNonce)
}

func observePublicationV1(
	parent context.Context,
	host portnativebuild.Host,
	requestNonce string,
	binding domainnativebuild.PublicationBindingV1,
) (portnativebuild.PublicationOutcomeV1, error) {
	ctx, cancel := cleanupContextV1(parent)
	defer cancel()
	return host.ObservePublication(ctx, requestNonce, binding)
}

func cleanupContextV1(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if parent != nil {
		base = context.WithoutCancel(parent)
	}
	return context.WithTimeout(base, cleanupDeadlineV1)
}

func validAssessmentV1(assessment portnativebuild.EnvironmentAssessmentV1) bool {
	if assessment.Ready {
		return assessment.Blocker == ""
	}
	switch assessment.Blocker {
	case "toolchain_user_writable", "toolchain_identity_unbound", "authority_identity_unbound", "controlled_environment_unproven":
		return true
	default:
		return false
	}
}

func validUnsettledObservationV1(
	receipt domainnativebuild.CargoExecutionReceiptV1,
	snapshot domainnativebuild.SourceSnapshotBindingV1,
	requestNonce string,
) bool {
	return receipt.ExecutionID == "" && receipt.Status == domainnativebuild.CargoStatusSucceeded &&
		receipt.RequestNonce == requestNonce &&
		receipt.TrustClass == domainnativebuild.CargoTrustControlledRelease && receipt.Blocker == "" &&
		receipt.SourceSnapshot == snapshot && !receipt.Containment.TemporaryStateDestroyed
}

func validPublicationV1(value portnativebuild.PublicationResultV1, requestNonce string, bindingSHA256 string) bool {
	return value.RequestNonce == requestNonce && value.PublicationBindingSHA256 == bindingSHA256 &&
		validDigestV1(value.GenerationID) && validDigestV1(value.InventorySHA256) &&
		validDigestV1(value.GenerationReceiptSHA256) && validDigestV1(value.ComponentReceiptSHA256)
}

func validCommittedPublicationV1(
	outcome portnativebuild.PublicationOutcomeV1,
	requestNonce string,
	bindingSHA256 string,
) bool {
	return outcome.State == portnativebuild.PublicationCommittedV1 &&
		validPublicationV1(outcome.Result, requestNonce, bindingSHA256)
}

func stopForTerminalContextV1(
	ctx context.Context,
	host portnativebuild.Host,
	artifacts portnativebuild.ArtifactLease,
	requestNonce string,
) (ResultV1, error, bool) {
	if ctx == nil || ctx.Err() == nil {
		return ResultV1{}, nil, false
	}
	artifactErr := abortArtifactsAfterV1(ctx, host, artifacts, requestNonce)
	if artifactErr != nil {
		return failedResultV1("build_cleanup_indeterminate"), errors.Join(ErrCleanupIndeterminate, ctx.Err(), artifactErr), true
	}
	return failedResultV1(executionBlockerV1(ctx)), errors.Join(ErrExecution, ctx.Err()), true
}

func failedResultV1(blocker string) ResultV1 {
	return ResultV1{Status: StatusFailed, Blocker: blocker}
}

func snapshotFailureV1(ctx context.Context, cause error) (ResultV1, error) {
	if ctx != nil && ctx.Err() != nil {
		return failedResultV1(executionBlockerV1(ctx)), errors.Join(ErrExecution, ctx.Err(), cause)
	}
	return failedResultV1("source_snapshot_unbound"), errors.Join(ErrSnapshot, cause)
}

func executionBlockerV1(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "cargo_execution_cancelled"
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "cargo_execution_timeout"
	}
	return "cargo_execution_failed"
}

func validDigestV1(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func capabilityNilV1(value any) bool {
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
