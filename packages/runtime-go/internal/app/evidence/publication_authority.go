package evidence

import (
	"context"
	"errors"

	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// CurrentPublicationAuthority is the host-owned freshness check performed at
// every server publication entry. It is intentionally independent from the
// model, tool output, local registry index, and accepted-final payload.
type CurrentPublicationAuthority interface {
	ValidateCurrentPublication(context.Context, domainsecurity.TurnSecurityContext) error
}

func (finalizer *currentPublicationAuthorityFinalizer) ReconcileCommittedInterrupt(
	ctx context.Context,
	store appturn.AcceptedFinalCompletionStore,
	threadID, turnID string,
) (domainevidence.TerminalPublicationIntent, bool, error) {
	if finalizer == nil || finalizer.CasePublicationFinalizer == nil {
		return domainevidence.TerminalPublicationIntent{}, false, errors.New("case interrupt recovery authority is unavailable")
	}
	authority, ok := finalizer.CasePublicationFinalizer.(CaseInterruptPublicationAuthority)
	if !ok || authority == nil {
		return domainevidence.TerminalPublicationIntent{}, false, errors.New("case interrupt recovery authority is unavailable")
	}
	return authority.ReconcileCommittedInterrupt(ctx, store, threadID, turnID)
}

type CurrentPublicationAuthorityFunc func(context.Context, domainsecurity.TurnSecurityContext) error

func (fn CurrentPublicationAuthorityFunc) ValidateCurrentPublication(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if fn == nil {
		return errors.New("current publication authority is unavailable")
	}
	return fn(ctx, securityContext)
}

type currentPublicationAuthorityFinalizer struct {
	CasePublicationFinalizer
	authority CurrentPublicationAuthority
}

// WithCurrentPublicationAuthority makes stale risk, binding, or snapshot
// authority a deterministic failure boundary. It never returns the authority
// error to a model for repair and never lets an already-populated registry
// project claims when the frozen executable context is no longer current.
func WithCurrentPublicationAuthority(finalizer CasePublicationFinalizer, authority CurrentPublicationAuthority) CasePublicationFinalizer {
	if finalizer == nil {
		return finalizer
	}
	return &currentPublicationAuthorityFinalizer{CasePublicationFinalizer: finalizer, authority: authority}
}

func (finalizer *currentPublicationAuthorityFinalizer) PersistBoundary(ctx context.Context, input PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error) {
	if finalizer == nil || finalizer.CasePublicationFinalizer == nil {
		return PersistCaseBoundaryResult{}, errors.New("current publication authority is unavailable")
	}
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(input.Context) != nil {
		input.TerminalReason = TerminalProviderFailure
		input.SourceUnavailable = false
		input.TerminalStatus = ""
		input.Telemetry = appusage.TerminalTelemetryV1{}
	} else if domainsecurity.TurnSecurityContextAllowsCaseEvidence(input.Context) {
		if finalizer.authority == nil || finalizer.authority.ValidateCurrentPublication(ctx, input.Context) != nil {
			input.TerminalReason = TerminalProviderFailure
			input.SourceUnavailable = false
			input.TerminalStatus = ""
			input.Telemetry = appusage.TerminalTelemetryV1{}
		}
	}
	return finalizer.CasePublicationFinalizer.PersistBoundary(ctx, input)
}

// Preserve the optional tool-evidence surface when this wrapper is applied at
// server composition. A missing inner authority remains fail-closed.
func (finalizer *currentPublicationAuthorityFinalizer) PrepareCurrentToolEvidence(ctx context.Context, input PrepareToolEvidenceInput) (PreparedToolEvidence, bool, error) {
	authority, ok := finalizer.CasePublicationFinalizer.(ToolEvidenceAuthority)
	if !ok || authority == nil {
		return PreparedToolEvidence{}, false, nil
	}
	return authority.PrepareCurrentToolEvidence(ctx, input)
}

func (finalizer *currentPublicationAuthorityFinalizer) CommitCurrentToolEvidence(ctx context.Context, input CommitToolEvidenceInput) (domainevidence.EvidenceReceipt, error) {
	authority, ok := finalizer.CasePublicationFinalizer.(ToolEvidenceAuthority)
	if !ok || authority == nil {
		return domainevidence.EvidenceReceipt{}, errors.New("tool evidence authority is unavailable")
	}
	return authority.CommitCurrentToolEvidence(ctx, input)
}
