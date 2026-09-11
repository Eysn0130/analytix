package thread

import (
	"testing"

	"analytix.local/runtime-go/internal/contracts"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

func TestCaseHistoryGeneralTerminalProjectsOnlyTypedOrdinaryResult(t *testing.T) {
	const (
		threadID = "thread-case-history-ordinary"
		turnID   = "turn-general-before-case"
		itemID   = "item-general-terminal"
		text     = "Updated the parser and the focused tests pass."
	)
	slot, err := domainordinaryresult.NewResultSlotV1(text)
	if err != nil {
		t.Fatal(err)
	}
	baseTurn := map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed",
		"createdAt": "2026-08-02T00:00:00Z", "finishedAt": "2026-08-02T00:00:00Z",
		"items": []any{map[string]any{
			"id": itemID, "turnId": turnID, "threadId": threadID,
			"kind": "assistant_text", "role": "assistant", "status": "completed",
			"createdAt": "2026-08-02T00:00:00Z", "finishedAt": "2026-08-02T00:00:00Z",
			"text": text, "ordinaryResult": domainordinaryresult.ResultSlotV1Map(slot),
		}},
	}
	authority := domainturnterminal.GeneralTerminalProjectionAuthorityV1{
		Governed: true,
		Terminal: true,
		Commit: domainturnterminal.GeneralTerminalPublicationCommitV1{
			ThreadID: threadID, TurnID: turnID, TerminalStatus: "completed", TerminalItemID: itemID,
		},
	}

	projected, err := projectCaseHistoryGeneralTerminalV1(threadID, contracts.CloneMap(baseTurn), authority)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := projected["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("typed ordinary terminal item count=%d want=1", len(items))
	}
	item, _ := items[0].(map[string]any)
	projectedSlot, err := domainordinaryresult.ParseResultSlotV1(item["ordinaryResult"])
	if err != nil || projectedSlot != slot || contracts.StringField(item, "text") != slot.Text {
		t.Fatalf("typed ordinary result projection mismatch: item=%#v slot=%#v err=%v", item, projectedSlot, err)
	}

	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "missing slot",
			mutate: func(item map[string]any) {
				delete(item, "ordinaryResult")
			},
		},
		{
			name: "digest tamper",
			mutate: func(item map[string]any) {
				item["ordinaryResult"].(map[string]any)["resultDigest"] = domainsecurity.SHA256Hex([]byte("tampered"))
			},
		},
		{
			name: "text mismatch",
			mutate: func(item map[string]any) {
				item["text"] = text + " changed"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := contracts.CloneMap(baseTurn)
			item := turn["items"].([]any)[0].(map[string]any)
			test.mutate(item)
			withheld, projectionErr := projectCaseHistoryGeneralTerminalV1(threadID, turn, authority)
			if projectionErr != nil {
				t.Fatal(projectionErr)
			}
			withheldItems, _ := withheld["items"].([]any)
			if len(withheldItems) != 0 {
				t.Fatalf("unverified ordinary result became public: %#v", withheldItems)
			}
		})
	}

	foreign := authority
	foreign.Commit.TurnID = "turn-foreign"
	if projected, projectionErr := projectCaseHistoryGeneralTerminalV1(threadID, contracts.CloneMap(baseTurn), foreign); projectionErr == nil || projected != nil {
		t.Fatalf("cross-turn authority was accepted: projected=%#v err=%v", projected, projectionErr)
	}
}
