package evidence

import (
	"context"
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type originalEvidenceSettlementAuditV1 struct {
	prepared []domainevidence.PreparedEvidenceSettlement
	history  *registryport.OriginalHistoryV2
}

// ValidateOriginalEvidenceSettlementInventoryV1 binds a complete native Original
// V2 registry graph to every signed prepared record and strict Core primary.
// The caller owns physical completeness, independent enrollment and revalidation
// throughout use. This consumer checks historical grant/result authority even
// when a durable marker precedes its first capsule; it never returns current
// membership, a repair plan, a thread hold or permission to Commit.
func ValidateOriginalEvidenceSettlementInventoryV1(ctx context.Context, reader AcceptedFinalPublicReader, authority finalauthorityport.Authority, prepared []domainevidence.PreparedEvidenceSettlement, history *registryport.OriginalHistoryV2) error {
	if prepared == nil || history == nil || history.Indexes == nil || history.Capsules == nil {
		return errors.New("original settlement audit denominator is unavailable")
	}
	_, err := inspectEvidenceSettlementInventoryV1(ctx, reader, Issuer{Authority: authority}, nil, nil,
		&originalEvidenceSettlementAuditV1{prepared: prepared, history: history})
	return err
}
