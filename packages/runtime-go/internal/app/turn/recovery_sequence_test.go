package turn

import (
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestRestoredRuntimeSequenceIncludesDurableAutoContinueReservations(t *testing.T) {
	got := RestoredRuntimeSequenceV1(
		2,
		[]map[string]any{{"turnId": "turn_4"}, {"turnId": "invalid"}},
		[]domainjob.Record{{AutoContinueTurnID: "turn_7"}, {AutoContinueTurnID: "turn_invalid"}},
	)
	if got != 7 {
		t.Fatalf("restored runtime sequence=%d want=7", got)
	}
}
