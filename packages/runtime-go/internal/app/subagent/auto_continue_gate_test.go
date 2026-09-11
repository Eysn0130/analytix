package subagent

import (
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestBackgroundAutoContinueSiblingGateBindsParentThreadAndTurn(t *testing.T) {
	record := domainjob.Record{
		ID: "job-current", ParentThreadID: "thread-a", ParentTurnID: "turn_1", AutoContinueParent: true,
	}
	crossThread := domainjob.Record{
		ID: "job-cross", ParentThreadID: "thread-b", ParentTurnID: "turn_1",
		AutoContinueParent: true, AutoContinueStatus: "starting",
	}
	if reason := BackgroundAutoContinueSiblingGate(record, []domainjob.Record{record, crossThread}); reason != "" {
		t.Fatalf("cross-thread same-name turn was treated as a sibling: %q", reason)
	}
	sameParent := crossThread
	sameParent.ID = "job-sibling"
	sameParent.ParentThreadID = record.ParentThreadID
	if reason := BackgroundAutoContinueSiblingGate(record, []domainjob.Record{record, sameParent}); reason != "auto_continue_already_starting" {
		t.Fatalf("same parent sibling was not blocked: %q", reason)
	}
}
