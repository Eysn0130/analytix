package model

import (
	"strings"
	"testing"
)

func TestBuildPromotedSteeringBuildsEventsAndProviderMessages(t *testing.T) {
	firstItem := map[string]any{"id": "item_1", "kind": "user_message", "text": " keep going "}
	out := BuildPromotedSteering(PromotedSteeringInput{
		ThreadID: " thread_1 ",
		TurnID:   " turn_1 ",
		Entries: []map[string]any{
			{"id": "steer_1", "admittedAt": "2026-07-03T08:00:00Z"},
		},
		Items: []map[string]any{
			firstItem,
			{"id": "item_2", "kind": "user_message", "text": "   "},
		},
	})
	if len(out.ItemCreatedEvents) != 2 {
		t.Fatalf("expected item_created events for all promoted items: %#v", out.ItemCreatedEvents)
	}
	firstEvent := out.ItemCreatedEvents[0]
	if firstEvent["kind"] != "item_created" || firstEvent["threadId"] != "thread_1" || firstEvent["turnId"] != "turn_1" || firstEvent["itemId"] != "item_1" {
		t.Fatalf("unexpected item event: %#v", firstEvent)
	}
	item, _ := firstEvent["item"].(map[string]any)
	item["text"] = "mutated"
	if firstItem["text"] != " keep going " {
		t.Fatalf("event item should not alias source item: %#v", firstItem)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected one non-empty provider message: %#v", out.Messages)
	}
	message := out.Messages[0]
	if message.Role != "user" || message.Content == "keep going" {
		t.Fatalf("expected steering provider content wrapper: %#v", message)
	}
	if strings.Contains(message.Content, "Admitted at:") ||
		strings.Contains(message.Content, "2026-07-03T08:00:00Z") ||
		!strings.Contains(message.Content, midTurnSteeringPrefix) ||
		!strings.Contains(message.Content, "keep going") {
		t.Fatalf("steering provider content missing metadata/text: %q", message.Content)
	}
}

func TestBuildPromotedSteeringRejectsMissingIdentity(t *testing.T) {
	if out := BuildPromotedSteering(PromotedSteeringInput{TurnID: "turn_1", Items: []map[string]any{{"id": "item_1"}}}); len(out.ItemCreatedEvents) != 0 || len(out.Messages) != 0 {
		t.Fatalf("missing thread id should reject promotion: %#v", out)
	}
	if out := BuildPromotedSteering(PromotedSteeringInput{ThreadID: "thread_1", Items: []map[string]any{{"id": "item_1"}}}); len(out.ItemCreatedEvents) != 0 || len(out.Messages) != 0 {
		t.Fatalf("missing turn id should reject promotion: %#v", out)
	}
}
