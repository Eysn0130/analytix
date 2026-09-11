package turn

import (
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

func TestResolveCommittedGeneralInterruptMetadataV1UsesCanonicalOutbox(t *testing.T) {
	securityContext := failureGeneralContext(t)
	store := &failureStoreStub{changed: true}
	_, err := CommitGeneralFailureTerminal(PersistFailureInput{
		Store: store, SecurityContext: securityContext, TerminalReason: "cancel",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil), FinishedAt: "2026-01-02T03:04:05Z",
		Interrupt: &GeneralTerminalInterruptMetadata{Discard: true, Cancelled: true, CancelledPendingGates: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := committedGeneralInterruptThread(t, store, securityContext.ThreadID, securityContext.TurnID)
	metadata, err := ResolveCommittedGeneralInterruptMetadataV1(thread, securityContext.TurnID)
	if err != nil || !metadata.Discard || !metadata.Cancelled || metadata.CancelledPendingGates != 2 {
		t.Fatalf("canonical interrupt metadata mismatch: %#v err=%v", metadata, err)
	}

	tampered := contracts.CloneMap(thread)
	turn := tampered["turns"].([]any)[0].(map[string]any)
	turn["discard"] = false
	if _, err := ResolveCommittedGeneralInterruptMetadataV1(tampered, securityContext.TurnID); err == nil {
		t.Fatal("top-level interrupt projection overrode its canonical outbox")
	}
}

func committedGeneralInterruptThread(t *testing.T, store *failureStoreStub, threadID, turnID string) map[string]any {
	t.Helper()
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.terminalFields["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]any, 0, len(store.failedItems))
	for _, item := range store.failedItems {
		items = append(items, contracts.CloneMap(item))
	}
	turn := map[string]any{
		"id": turnID, "status": "aborted", "finishedAt": commit.CommittedAt,
		"securityContext": turnSecurityContextRecord(failureGeneralContext(t)), "items": items,
	}
	for key, value := range store.terminalFields {
		turn[key] = value
	}
	return map[string]any{
		"id": threadID, "securityState": turnSecurityContextRecord(failureGeneralContext(t)), "turns": []any{turn},
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
	}
}
