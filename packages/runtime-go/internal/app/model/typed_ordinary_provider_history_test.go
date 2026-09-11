package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestTypedOrdinaryProviderHistoryBeforeTurnV1PreservesMixedDurableOrderAndPrefix(t *testing.T) {
	generalSlot := typedOrdinaryHistoryProviderSlot(t, "general result")
	generalTurn, generalCommit := typedOrdinaryHistoryGeneralTurn(t, "thread-typed-history", "turn-general", generalSlot)
	caseSlot := typedOrdinaryHistoryProviderSlot(t, "case ordinary result")
	afterSlot := typedOrdinaryHistoryProviderSlot(t, "after active")
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{generalCommit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": "thread-typed-history",
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{
			generalTurn,
			map[string]any{"id": "turn-case", "status": "completed"},
			map[string]any{"id": "turn-active", "status": "running"},
			map[string]any{"id": "turn-after", "status": "completed"},
		},
	}
	messages, err := TypedOrdinaryProviderHistoryBeforeTurnV1(thread, "  turn-active  ", map[string]domainordinaryresult.ResultSlotV1{
		"turn-case":  caseSlot,
		"turn-after": afterSlot,
		"extra":      typedOrdinaryHistoryProviderSlot(t, "not in thread"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "assistant" || messages[0].Content != generalSlot.Text ||
		messages[1].Role != "assistant" || messages[1].Content != caseSlot.Text {
		t.Fatalf("typed ordinary prefix order mismatch: %#v", messages)
	}
}

func TestTypedOrdinaryProviderHistoryBeforeTurnV1SkipsHostFixedResults(t *testing.T) {
	hostFixed, err := domainordinaryresult.NewHostFixedResultSlotV1(
		domainordinaryresult.HostFixedProviderResultWithheldTextV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	generalTurn, generalCommit := typedOrdinaryHistoryGeneralTurn(t, "thread-host-fixed", "turn-general", hostFixed)
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{generalCommit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": "thread-host-fixed",
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{
			generalTurn,
			map[string]any{"id": "turn-case", "status": "completed"},
			map[string]any{"id": "turn-active", "status": "running"},
		},
	}
	messages, err := TypedOrdinaryProviderHistoryBeforeTurnV1(thread, "turn-active", map[string]domainordinaryresult.ResultSlotV1{
		"turn-case": hostFixed,
	})
	if err != nil || len(messages) != 0 {
		t.Fatalf("host-fixed results entered provider history: messages=%#v err=%v", messages, err)
	}
}

func TestTypedOrdinaryProviderHistoryBeforeTurnV1SkipsInvalidSlotsWithoutParsingCaseText(t *testing.T) {
	validCaseSlot := typedOrdinaryHistoryProviderSlot(t, "validated case ordinary result")
	invalidCaseSlot := validCaseSlot
	invalidCaseSlot.Text = "tampered slot text"
	thread := map[string]any{
		"id": "thread-invalid-slot",
		"turns": []any{
			map[string]any{
				"id": "turn-case-valid", "status": "completed",
				"items": []any{map[string]any{
					"id": "case-rendered", "kind": "assistant_text", "text": "tampered rendered text must not be parsed",
				}},
			},
			map[string]any{"id": "turn-case-invalid", "status": "completed"},
			map[string]any{"id": "turn-active", "status": "running"},
		},
	}
	messages, err := TypedOrdinaryProviderHistoryBeforeTurnV1(thread, "turn-active", map[string]domainordinaryresult.ResultSlotV1{
		"turn-case-valid":   validCaseSlot,
		"turn-case-invalid": invalidCaseSlot,
	})
	if err != nil || len(messages) != 1 || messages[0].Content != validCaseSlot.Text {
		t.Fatalf("case rendered text was parsed or tampered slot entered typed history: messages=%#v err=%v", messages, err)
	}
}

func TestTypedOrdinaryProviderHistoryBeforeTurnV1SkipsGeneralTerminalWithoutStrictSlot(t *testing.T) {
	boundarySlot := typedOrdinaryHistoryProviderSlot(t, domainevent.GeneralTerminalCompletedBoundaryTextV1)
	generalTurn, generalCommit := typedOrdinaryHistoryGeneralTurn(t, "thread-slot-missing", "turn-general", boundarySlot)
	delete(generalTurn["items"].([]any)[0].(map[string]any), "ordinaryResult")
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{generalCommit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": "thread-slot-missing",
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{generalTurn, map[string]any{"id": "turn-active", "status": "running"}},
	}
	messages, err := TypedOrdinaryProviderHistoryBeforeTurnV1(thread, "turn-active", nil)
	if err != nil || len(messages) != 0 {
		t.Fatalf("terminal without a strict ordinary slot entered typed history: messages=%#v err=%v", messages, err)
	}
}

func TestTypedOrdinaryProviderHistoryRequiresTrustedCaseCompactionCut(t *testing.T) {
	threadID := "thread-typed-case-compaction"
	generalSlot := typedOrdinaryHistoryProviderSlot(t, "ordinary history before signed case compaction")
	generalTurn, generalCommit := typedOrdinaryHistoryGeneralTurn(t, threadID, "turn-general", generalSlot)
	generalItems := generalTurn["items"].([]any)
	generalTurn["items"] = append([]any{map[string]any{
		"id": "item-turn-general-user", "threadId": threadID, "turnId": "turn-general",
		"kind": "user_message", "role": "user", "status": "completed",
		"text": "preserve the latest ordinary constraint",
	}}, generalItems...)
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{generalCommit},
	)
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := threaddomain.SealTaskContinuationSnapshotV1(threaddomain.TaskContinuationSnapshotV1{
		Todos: []threaddomain.TaskContinuationTodoV1{}, LatestUserConstraints: []string{"protected marker constraint"},
		EvidenceReferences: []threaddomain.TaskContinuationEvidenceReferenceV1{},
	})
	if err != nil {
		t.Fatal(err)
	}
	compactionTurnID := "turn-typed-case-compaction"
	sourceDigest := domainsecurity.SHA256Hex([]byte("typed-case-compaction-source"))
	marker := map[string]any{
		"id": "item-typed-case-compaction", "threadId": threadID, "turnId": compactionTurnID,
		"kind": "compaction", "schemaVersion": float64(3), "caseHistoryProjectionVersion": float64(2),
		"reasoningExcluded": true, "caseFactsExcluded": true, "sourceDigest": sourceDigest,
		"sourceContextDigest":   domainsecurity.SHA256Hex([]byte("typed-case-source-context")),
		"caseCompactionBinding": map[string]any{"authority": "test-only trusted-id input"},
		"taskContinuation":      threaddomain.TaskContinuationSnapshotMapV1(continuation),
	}
	thread := map[string]any{
		"id": threadID,
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{
			generalTurn,
			map[string]any{
				"id": compactionTurnID, "threadId": threadID, "status": "completed",
				"caseHistoryProjection": "compaction_authority_v1", "items": []any{marker},
			},
			map[string]any{"id": "turn-active", "threadId": threadID, "status": "running"},
		},
	}

	untrusted, err := TypedOrdinaryProviderHistoryBeforeTurnV1(thread, "turn-active", nil)
	if err != nil || len(untrusted) != 1 || untrusted[0].Content != generalSlot.Text {
		t.Fatalf("raw case compaction marker changed ordinary history: messages=%#v err=%v", untrusted, err)
	}
	trusted, err := TypedOrdinaryProviderHistoryBeforeTurnWithCaseCompactionsV1(
		thread, "turn-active", nil, map[string]bool{compactionTurnID: true},
	)
	if err != nil || len(trusted) != 1 || trusted[0].Role != "user" ||
		!strings.Contains(trusted[0].Content, "preserve the latest ordinary constraint") ||
		strings.Contains(trusted[0].Content, generalSlot.Text) {
		t.Fatalf("trusted case compaction did not replace old ordinary prose with typed continuation: messages=%#v err=%v", trusted, err)
	}
	caseContinuation := CaseTaskContinuationProviderHistoryBeforeTurnV1(
		thread, "turn-active", map[string]bool{compactionTurnID: true},
	)
	if len(caseContinuation) != 1 || caseContinuation[0].Role != "user" ||
		!strings.Contains(caseContinuation[0].Content, "protected marker constraint") ||
		strings.Contains(caseContinuation[0].Content, "preserve the latest ordinary constraint") ||
		strings.Contains(caseContinuation[0].Content, generalSlot.Text) {
		t.Fatalf("case continuation did not preserve only the original sealed case task state: %#v", caseContinuation)
	}
	if untrustedCase := CaseTaskContinuationProviderHistoryBeforeTurnV1(thread, "turn-active", nil); len(untrustedCase) != 0 {
		t.Fatalf("untrusted case compaction became case-request continuation history: %#v", untrustedCase)
	}
	marker["schemaVersion"] = json.Number("3")
	marker["caseHistoryProjectionVersion"] = json.Number("2")
	restarted, err := TypedOrdinaryProviderHistoryBeforeTurnWithCaseCompactionsV1(
		thread, "turn-active", nil, map[string]bool{compactionTurnID: true},
	)
	if err != nil || len(restarted) != 1 || restarted[0].Role != "user" ||
		!strings.Contains(restarted[0].Content, "preserve the latest ordinary constraint") {
		t.Fatalf("restart-shaped numeric fields invalidated trusted compaction continuation: messages=%#v err=%v", restarted, err)
	}

	tampered := contracts.CloneMap(thread)
	tamperedTurns := tampered["turns"].([]any)
	tamperedMarker := tamperedTurns[1].(map[string]any)["items"].([]any)[0].(map[string]any)
	tamperedContinuation := tamperedMarker["taskContinuation"].(map[string]any)
	tamperedContinuation["latestUserConstraints"] = []any{"tampered continuation"}
	withTamper, err := TypedOrdinaryProviderHistoryBeforeTurnWithCaseCompactionsV1(
		tampered, "turn-active", nil, map[string]bool{compactionTurnID: true},
	)
	if err != nil || len(withTamper) != 1 || withTamper[0].Content != generalSlot.Text {
		t.Fatalf("digest-invalid continuation became a provider history cut: messages=%#v err=%v", withTamper, err)
	}
	if tamperedCase := CaseTaskContinuationProviderHistoryBeforeTurnV1(
		tampered, "turn-active", map[string]bool{compactionTurnID: true},
	); len(tamperedCase) != 0 {
		t.Fatalf("digest-invalid continuation became case-request continuation history: %#v", tamperedCase)
	}
}

func TestTypedOrdinaryProviderHistoryBeforeTurnV1FailsClosedOnInvalidTurnInventory(t *testing.T) {
	tests := map[string]struct {
		thread       map[string]any
		activeTurnID string
	}{
		"empty active": {
			thread: map[string]any{"turns": []any{map[string]any{"id": "turn-active"}}},
		},
		"missing array": {
			thread: map[string]any{}, activeTurnID: "turn-active",
		},
		"non-object turn": {
			thread: map[string]any{"turns": []any{"turn-invalid", map[string]any{"id": "turn-active"}}}, activeTurnID: "turn-active",
		},
		"empty turn id": {
			thread: map[string]any{"turns": []any{map[string]any{"id": " "}, map[string]any{"id": "turn-active"}}}, activeTurnID: "turn-active",
		},
		"duplicate turn id": {
			thread: map[string]any{"turns": []any{map[string]any{"id": "turn-active"}, map[string]any{"id": " turn-active "}}}, activeTurnID: "turn-active",
		},
		"active missing": {
			thread: map[string]any{"turns": []any{map[string]any{"id": "turn-other"}}}, activeTurnID: "turn-active",
		},
		"invalid general authority": {
			thread: map[string]any{
				"id": "thread-invalid-authority",
				"turns": []any{
					map[string]any{"id": "turn-general", "generalTerminalPublication": map[string]any{}},
					map[string]any{"id": "turn-active"},
				},
			},
			activeTurnID: "turn-active",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if messages, err := TypedOrdinaryProviderHistoryBeforeTurnV1(test.thread, test.activeTurnID, nil); err == nil || len(messages) != 0 {
				t.Fatalf("invalid inventory did not fail closed: messages=%#v err=%v", messages, err)
			}
		})
	}
}

func TestTypedOrdinaryProviderHistoryBeforeTurnV1PropagatesTamperedGeneralAuthority(t *testing.T) {
	slot := typedOrdinaryHistoryProviderSlot(t, "general result")
	generalTurn, generalCommit := typedOrdinaryHistoryGeneralTurn(t, "thread-tampered-authority", "turn-general", slot)
	generalTurn["items"].([]any)[0].(map[string]any)["text"] = "tampered terminal item text"
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{generalCommit},
	)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": "thread-tampered-authority",
		domainturnterminal.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{generalTurn, map[string]any{"id": "turn-active"}},
	}
	if messages, err := TypedOrdinaryProviderHistoryBeforeTurnV1(thread, "turn-active", nil); err == nil || len(messages) != 0 {
		t.Fatalf("tampered terminal authority did not fail closed: messages=%#v err=%v", messages, err)
	}
}

func typedOrdinaryHistoryProviderSlot(t *testing.T, text string) domainordinaryresult.ResultSlotV1 {
	t.Helper()
	slot, err := domainordinaryresult.NewResultSlotV1(text)
	if err != nil {
		t.Fatal(err)
	}
	return slot
}

func typedOrdinaryHistoryGeneralTurn(
	t *testing.T,
	threadID string,
	turnID string,
	slot domainordinaryresult.ResultSlotV1,
) (map[string]any, domainturnterminal.GeneralTerminalPublicationCommitV1) {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-27T00:00:00Z"
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(
		securityContext, "success", "completed", slot.Text,
	)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": "item-" + turnID, "threadId": threadID, "turnId": turnID,
		"role": "assistant", "status": "completed", "createdAt": committedAt, "finishedAt": committedAt,
		"kind": "assistant_text", "text": slot.Text,
		"ordinaryResult":            domainordinaryresult.ResultSlotV1Map(slot),
		"generalTerminalCASBinding": domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
	}
	commit, err := domainturnterminal.NewGeneralTerminalPublicationCommitV1(
		securityContext, binding, committedAt, []domainturnterminal.GeneralTerminalPublicationDraftV1{
			{Slot: "terminal-item", Draft: map[string]any{
				"kind": "item_completed", "threadId": threadID, "turnId": turnID,
				"itemId": item["id"], "item": contracts.CloneMap(item), "timestamp": committedAt,
			}},
			{Slot: "usage", Draft: map[string]any{
				"kind": "usage", "threadId": threadID, "turnId": turnID, "model": "gpt-5",
				"usage": map[string]any{}, "cacheDiagnostics": map[string]any{}, "timestamp": committedAt,
				"usageFinalStatus": "completed",
			}},
			{Slot: "terminal", Draft: map[string]any{
				"kind": "turn_completed", "threadId": threadID, "turnId": turnID, "status": "completed",
				"timestamp": committedAt, "terminalReason": "success", "generalTerminalCASBindingDigest": binding.BindingDigest,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	turn := map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed", "finishedAt": committedAt,
		"securityContext": contextRecord, "items": []any{item},
		"generalTerminalCASBinding":  domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
		"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	}
	return turn, commit
}
