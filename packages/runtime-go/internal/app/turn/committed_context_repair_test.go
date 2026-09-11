package turn

import (
	"reflect"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCommittedTurnContextRepairsMissingAndCorruptPublicCopies(t *testing.T) {
	securityContext := newTurnCaseExecutionContextV2(t, "thread-repair", "turn-repair", "/cases/repair", 3, time.Unix(3, 0))
	state, err := contextepochapp.BootstrapState(securityContext.ThreadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": map[string]any{"corrupt": true}, "contextEpochState": map[string]any{"corrupt": true},
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "running", "securityContext": map[string]any{"corrupt": true},
		}},
	}
	repaired, err := RepairCommittedTurnContext(CommittedContextRepairInput{Thread: thread, SecurityContext: securityContext, EpochState: state})
	if err != nil {
		t.Fatal(err)
	}
	current, err := domainsecurity.ParseTurnSecurityContext(repaired["securityState"])
	if err != nil || !reflect.DeepEqual(current, securityContext) {
		t.Fatalf("thread security state was not repaired: current=%#v err=%v", current, err)
	}
	parsedState, err := domaincontextepoch.ParseState(repaired["contextEpochState"])
	if err != nil || !reflect.DeepEqual(parsedState, state) {
		t.Fatalf("context epoch state was not repaired: state=%#v err=%v", parsedState, err)
	}
	turn := repaired["turns"].([]any)[0].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	snapshot, snapshotErr := parseEpochSnapshot(turn["contextEpochSnapshot"])
	if err != nil || snapshotErr != nil || !reflect.DeepEqual(frozen, securityContext) || !reflect.DeepEqual(snapshot, state.AcceptedSnapshot) {
		t.Fatalf("turn authority was not repaired: context=%#v snapshot=%#v err=%v snapshotErr=%v", frozen, snapshot, err, snapshotErr)
	}

	conflicting := cloneCommittedContextValue(repaired)
	other := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
		CaseID: "other-case", CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")), DatasetSnapshotID: "other-snapshot",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("other-manifest")), ContextEpoch: securityContext.ContextEpoch, IssuedAt: time.Unix(3, 0),
	})
	conflicting["securityState"] = committedContextRecord(other)
	if _, err := RepairCommittedTurnContext(CommittedContextRepairInput{Thread: conflicting, SecurityContext: securityContext, EpochState: state}); err == nil {
		t.Fatal("valid conflicting public context was overwritten")
	}
}

func TestBoundaryOnlyCommittedContextRepairsOnRestart(t *testing.T) {
	securityContext := newTurnBoundaryOnlyContextV2(t, "thread-boundary-repair", "turn-boundary-repair", "/cases/repair", 4, time.Unix(4, 0))
	state, err := contextepochapp.BootstrapState(securityContext.ThreadID, securityContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, time.Unix(4, 0))
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": securityContext.ThreadID,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "running", "securityContext": map[string]any{"corrupt": true},
		}},
	}
	repaired, err := RepairCommittedTurnContext(CommittedContextRepairInput{Thread: thread, SecurityContext: securityContext, EpochState: state})
	if err != nil {
		t.Fatal(err)
	}
	turn := repaired["turns"].([]any)[0].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || frozen != securityContext {
		t.Fatalf("boundary-only committed authority was not repaired: context=%#v err=%v", frozen, err)
	}
}
