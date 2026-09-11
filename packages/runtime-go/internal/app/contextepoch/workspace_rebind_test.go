package contextepoch

import (
	"testing"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrepareWorkspaceRebindAdvancesEpochAndSecurityBinding(t *testing.T) {
	at := time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC)
	current := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-rebind", TurnID: "turn-before", WorkspaceRealPath: "/workspace/a", ContextEpoch: 3, IssuedAt: at,
	})
	state, err := BootstrapState(current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{SecurityBindingEntry(current)}, at)
	if err != nil {
		t.Fatal(err)
	}
	target := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: "turn-workspace-rebind", WorkspaceRealPath: "/workspace/b",
		ContextEpoch: current.ContextEpoch, IssuedAt: at.Add(time.Second),
	})
	result, err := PrepareWorkspaceRebind(map[string]any{"contextEpochState": PublicState(state)}, current, target, at.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.State.AcceptedSnapshot.Epoch != 4 || result.SecurityContext.ContextEpoch != 4 || result.SecurityContext.WorkspaceRealPath != target.WorkspaceRealPath {
		t.Fatalf("workspace rebind authority mismatch: %#v", result)
	}
	if !stateContainsExactSecurityBinding(result.State, result.SecurityContext) {
		t.Fatalf("workspace rebind state did not seal target security binding: %#v", result.State)
	}
}

func TestPrepareWorkspaceRebindRejectsCaseAndStaleEpoch(t *testing.T) {
	at := time.Now().UTC()
	current := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-rebind", TurnID: "turn-before", WorkspaceRealPath: "/workspace/a", ContextEpoch: 2, IssuedAt: at,
	})
	state, err := BootstrapState(current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{SecurityBindingEntry(current)}, at)
	if err != nil {
		t.Fatal(err)
	}
	target := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: "turn-rebind", WorkspaceRealPath: "/workspace/b", ContextEpoch: 2, IssuedAt: at.Add(time.Second),
	})
	stale := current
	stale.ContextEpoch = 1
	if _, err := PrepareWorkspaceRebind(map[string]any{"contextEpochState": PublicState(state)}, stale, target, at); err == nil {
		t.Fatal("stale current epoch was accepted")
	}
	caseTarget := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: "turn-case", WorkspaceRealPath: "/workspace/case", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 2, IssuedAt: at,
	})
	if _, err := PrepareWorkspaceRebind(map[string]any{"contextEpochState": PublicState(state)}, current, caseTarget, at); err == nil {
		t.Fatal("case workspace rebind was accepted without signed authority")
	}
}
