package contextepoch

import (
	"strings"
	"testing"
	"time"
)

func TestContextEpochStateStrictIntegrityAndCanonicalRegistry(t *testing.T) {
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	entryA := SourceEntry{
		Version: ContractVersion, SourceID: "source-a", Kind: "memory", Digest: SHA256Hex([]byte("alpha")), Sequence: 2,
		TrustState: TrustUntrusted, PromptBoundary: BoundaryDynamicContext, TokenBudget: 32, ActivationState: ActivationActive,
		ActivationReason: "explicit-turn-selection",
	}
	entryB := SourceEntry{
		Version: ContractVersion, SourceID: "source-b", Kind: "diagnostics", Digest: SHA256Hex([]byte("beta")), Sequence: 1,
		TrustState: TrustTrusted, PromptBoundary: BoundaryDiagnosticsOnly, ActivationState: ActivationInactive,
	}
	snapshot := SealSnapshot(Snapshot{
		ThreadID: "thread-1", Epoch: 3, BaselineSequence: 2, RegistryDigest: RegistryDigest([]SourceEntry{entryB, entryA}),
		Sources: []SourceSnapshot{SourceSnapshotFromEntry(entryB), SourceSnapshotFromEntry(entryA)}, AcceptedAt: now,
		ChangeReasons: []ChangeReason{ReasonSourceAdded, ReasonActivationChanged}, Impact: ChangeImpact{DynamicContext: true},
	})
	state := SealState(State{ThreadID: "thread-1", Registry: []SourceEntry{entryB, entryA}, AcceptedSnapshot: snapshot})
	if err := ValidateState(state); err != nil {
		t.Fatal(err)
	}
	if state.Registry[0].SourceID != "source-a" || state.AcceptedSnapshot.Sources[0].SourceID != "source-a" {
		t.Fatalf("registry and snapshot must be canonical: %#v", state)
	}
	parsed, err := ParseState(state)
	if err != nil || parsed.StateDigest != state.StateDigest {
		t.Fatalf("strict round trip failed: parsed=%#v err=%v", parsed, err)
	}

	tampered := state
	tampered.Registry = append([]SourceEntry(nil), state.Registry...)
	tampered.Registry[0].TokenBudget++
	if err := ValidateState(tampered); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("tampered state must fail closed: %v", err)
	}

	unknown := map[string]any{
		"version": state.Version, "threadId": state.ThreadID, "registry": state.Registry,
		"acceptedSnapshot": state.AcceptedSnapshot, "stateDigest": state.StateDigest, "unexpected": true,
	}
	if _, err := ParseState(unknown); err == nil {
		t.Fatal("unknown context epoch properties must fail closed")
	}
}

func TestContextEpochSourceEntryRejectsRawReasonAndUnboundedActiveSource(t *testing.T) {
	base := SourceEntry{
		Version: ContractVersion, SourceID: "source-a", Kind: "memory", Digest: SHA256Hex([]byte("alpha")), Sequence: 1,
		TrustState: TrustUntrusted, PromptBoundary: BoundaryDynamicContext, TokenBudget: 16, ActivationState: ActivationActive,
		ActivationReason: "explicit-turn-selection",
	}
	if err := ValidateSourceEntry(base); err != nil {
		t.Fatal(err)
	}
	rawReason := base
	rawReason.ActivationReason = "/Users/alice/secret case.txt"
	if err := ValidateSourceEntry(rawReason); err == nil {
		t.Fatal("raw local paths must not be accepted as reason codes")
	}
	unbounded := base
	unbounded.TokenBudget = 0
	if err := ValidateSourceEntry(unbounded); err == nil {
		t.Fatal("active provider-visible sources must be bounded")
	}
}
