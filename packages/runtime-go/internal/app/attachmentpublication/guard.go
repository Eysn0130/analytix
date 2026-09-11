package attachmentpublication

import (
	"context"
	"errors"

	attachmentuseapp "analytix.local/runtime-go/internal/app/attachmentuse"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

type guardedFinalizer struct {
	next evidenceapp.CasePublicationFinalizer
	uses *attachmentuseapp.Service
}

var _ evidenceapp.ToolEvidenceAuthority = guardedFinalizer{}

// WithUseGuard makes open or corrupt attachment effects a mechanical blocker
// for every case publication path, including success, failure, cancellation,
// recovery, resume, restart, and host boundary output.
func WithUseGuard(next evidenceapp.CasePublicationFinalizer, uses *attachmentuseapp.Service) evidenceapp.CasePublicationFinalizer {
	return guardedFinalizer{next: next, uses: uses}
}

func (guard guardedFinalizer) PersistBoundary(ctx context.Context, input evidenceapp.PersistCaseBoundaryInput) (evidenceapp.PersistCaseBoundaryResult, error) {
	if guard.next == nil || guard.uses == nil || !guard.uses.Available() {
		return evidenceapp.PersistCaseBoundaryResult{}, errors.New("attachment publication guard is unavailable")
	}
	if err := guard.uses.RequireNoOpenForFrozenPublicationContext(ctx, input.Context); err != nil {
		return evidenceapp.PersistCaseBoundaryResult{}, err
	}
	return guard.next.PersistBoundary(ctx, input)
}

// Preserve the optional tool-evidence authority through the attachment
// publication guard. Dropping this interface would silently prevent a valid
// tool outcome from reaching the host evidence reader and final gate.
func (guard guardedFinalizer) PrepareCurrentToolEvidence(
	ctx context.Context,
	input evidenceapp.PrepareToolEvidenceInput,
) (evidenceapp.PreparedToolEvidence, bool, error) {
	authority, ok := guard.next.(evidenceapp.ToolEvidenceAuthority)
	if !ok || authority == nil || guard.uses == nil || !guard.uses.Available() {
		return evidenceapp.PreparedToolEvidence{}, false, nil
	}
	return authority.PrepareCurrentToolEvidence(ctx, input)
}

func (guard guardedFinalizer) CommitCurrentToolEvidence(
	ctx context.Context,
	input evidenceapp.CommitToolEvidenceInput,
) (domainevidence.EvidenceReceipt, error) {
	authority, ok := guard.next.(evidenceapp.ToolEvidenceAuthority)
	if !ok || authority == nil || guard.uses == nil || !guard.uses.Available() {
		return domainevidence.EvidenceReceipt{}, errors.New("attachment-guarded tool evidence authority is unavailable")
	}
	return authority.CommitCurrentToolEvidence(ctx, input)
}
