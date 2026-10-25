package toolresultsnapshot

import (
	"context"
	"strings"
	"testing"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
)

func TestProducerJobSourceCannotBeReadAsOriginalSettledResultV1(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	body, truncated := "actual source bytes", false
	for _, tool := range []string{"task", "bash"} {
		snapshot := domaintoolresult.ProtectedSnapshotV1{Version: 1, Purpose: domaintoolresult.ProtectedSnapshotPurposeV1, Principal: principal,
			Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID, ResultItemID: domaintoolresult.ToolResultItemIDV1("turn-a", callID), ToolName: tool,
			ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Inspect: &domaintoolresult.ProtectedInspectV1{InputJSON: `{}`},
			Capture: domaintoolresult.ProtectedCaptureV1{Kind: "job", Status: "completed", Job: &domaintoolresult.ProtectedJobCaptureV1{Operation: "output", Total: 1,
				Jobs: []domaintoolresult.ProtectedJobRecordV1{{JobID: "job-a", Kind: "subagent", Status: "completed", Output: domaintoolresult.ProtectedJobOutputV1{Availability: "available", Body: &body, Truncated: &truncated}}}}},
		}
		if tool == "bash" {
			snapshot.Capture.Job.Jobs[0].Kind = "background-shell"
		}
		binding, err := domaintoolresult.ProtectedSnapshotBindingForV1(snapshot)
		if err != nil {
			t.Fatal("source was not CAS-valid", err)
		}
		selector := snapshotport.SelectorV1{ThreadID: snapshot.ThreadID, TurnID: snapshot.TurnID, CallID: snapshot.CallID, ResultItemID: snapshot.ResultItemID}
		order := []string{}
		// Even a source binding incorrectly attached to its original settled
		// result cannot acquire that operation's local-display shape.
		access := &localSnapshotFixtureAccess{order: &order, authorized: snapshotport.AuthorizedV1{Selector: selector, Principal: principal, Workspace: snapshot.Workspace, ToolName: tool, ContextDigest: snapshot.ContextDigest, ContextEpoch: 1, ExecutionGrantID: snapshot.ExecutionGrantID, CaseBindingHash: snapshot.CaseBindingHash, Binding: binding}}
		service := &Service{Store: localSnapshotFixtureStore{snapshot, &order}, Authority: access}
		if display, err := service.Read(context.Background(), selector); err == nil || display.Capture.Job != nil {
			t.Fatal("source leaked into original operation's display")
		}
	}
}
