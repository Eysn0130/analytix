package turn

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

func TestValidateAssistantPublicationEventAcceptsExactCommittedGeneralTerminal(t *testing.T) {
	thread, event := committedGeneralAssistantPublicationFixture(t)
	if err := ValidateAssistantPublicationEvent(thread, event); err != nil {
		t.Fatalf("exact committed general assistant event was rejected: %v", err)
	}
}

func TestValidateAssistantPublicationEventRejectsTamperingAndStaleContext(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, map[string]any, map[string]any)
	}{
		{
			name: "text",
			mutate: func(_ *testing.T, _ map[string]any, event map[string]any) {
				event["item"].(map[string]any)["text"] = "different"
			},
		},
		{
			name: "item identity",
			mutate: func(_ *testing.T, _ map[string]any, event map[string]any) {
				event["itemId"] = "item_other"
			},
		},
		{
			name: "item timestamp",
			mutate: func(_ *testing.T, _ map[string]any, event map[string]any) {
				event["item"].(map[string]any)["finishedAt"] = "2026-07-15T00:00:01Z"
			},
		},
		{
			name: "event timestamp",
			mutate: func(_ *testing.T, _ map[string]any, event map[string]any) {
				event["timestamp"] = "2026-07-15T00:00:01Z"
			},
		},
		{
			name: "missing turn binding",
			mutate: func(_ *testing.T, thread map[string]any, _ map[string]any) {
				turn := thread["turns"].([]any)[0].(map[string]any)
				delete(turn, "generalTerminalCASBinding")
			},
		},
		{
			name: "stale current context",
			mutate: func(t *testing.T, thread map[string]any, _ map[string]any) {
				frozen, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
				if err != nil {
					t.Fatal(err)
				}
				current := generalTerminalTestContext(t, frozen.ThreadID, frozen.TurnID, frozen.ContextEpoch+1)
				thread["securityState"] = turnSecurityContextRecord(current)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			thread, event := committedGeneralAssistantPublicationFixture(t)
			test.mutate(t, thread, event)
			if err := ValidateAssistantPublicationEvent(thread, event); err == nil {
				t.Fatal("tampered or stale assistant publication was accepted")
			}
		})
	}
}

func TestValidateAssistantPublicationEventRejectsNestedAndCaseAssistantRecords(t *testing.T) {
	thread, event := committedGeneralAssistantPublicationFixture(t)
	forged := map[string]any{
		"kind": "usage", "threadId": stringField(thread, "id"),
		"details": map[string]any{"kind": "assistant_text", "text": "forged"},
	}
	if err := ValidateAssistantPublicationEvent(thread, forged); err == nil {
		t.Fatal("nested assistant text bypassed the publication sink")
	}

	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["acceptedFinal"] = map[string]any{"recordDigest": domainsecurity.SHA256Hex([]byte("accepted"))}
	if err := ValidateAssistantPublicationEvent(thread, event); err == nil {
		t.Fatal("case accepted-final assistant event bypassed its atomic manifest path")
	}
}

func committedGeneralAssistantPublicationFixture(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	securityContext := generalTerminalTestContext(t, "thread-general-event", "turn-general-event", 1)
	text := "general guidance"
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingV1(securityContext, text)
	if err != nil {
		t.Fatal(err)
	}
	bindingRecord := domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	item := BuildAssistantTextItem(AssistantTextItemInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Text: text,
		CreatedAt: "2026-07-15T00:00:00Z", FinishedAt: "2026-07-15T00:00:00Z",
	})
	item["generalTerminalCASBinding"] = bindingRecord
	turn := map[string]any{
		"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "completed",
		"securityContext":           turnSecurityContextRecord(securityContext),
		"generalTerminalCASBinding": bindingRecord,
		"items":                     []any{item},
	}
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": turnSecurityContextRecord(securityContext), "turns": []any{turn},
	}
	event := AssistantItemCompletedEvent(item)
	event["timestamp"] = item["finishedAt"]
	return thread, event
}
