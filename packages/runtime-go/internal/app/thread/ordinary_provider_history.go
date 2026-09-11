package thread

import (
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
)

// TypedOrdinaryProviderHistoryBeforeTurnV1 keeps a failed case accepted-final
// projection scoped to case slots. Independently verified general history and
// signed compaction continuation remain available to the ordinary Agent lane.
func TypedOrdinaryProviderHistoryBeforeTurnV1(
	projector PublicProjector,
	thread map[string]any,
	activeTurnID string,
	trustedCaseCompactionTurnIDs map[string]bool,
) []domainmodel.Message {
	caseSlots := map[string]domainordinaryresult.ResultSlotV1{}
	if resolver, ok := projector.(interface {
		CommittedCaseOrdinaryResultsV1(map[string]any) (map[string]domainordinaryresult.ResultSlotV1, error)
	}); ok {
		if resolved, err := resolver.CommittedCaseOrdinaryResultsV1(thread); err == nil {
			caseSlots = resolved
		}
	}
	history, err := appmodel.TypedOrdinaryProviderHistoryBeforeTurnWithCaseCompactionsV1(
		thread, activeTurnID, caseSlots, trustedCaseCompactionTurnIDs,
	)
	if err != nil {
		return nil
	}
	return history
}
