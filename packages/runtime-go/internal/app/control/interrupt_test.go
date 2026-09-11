package control

import "testing"

func TestTerminalItemCompletedEvents(t *testing.T) {
	source := map[string]any{"id": " item_1 ", "kind": "assistant_text", "text": "done"}
	events := TerminalItemCompletedEvents(TerminalItemEventsInput{
		ThreadID: " thread_1 ",
		TurnID:   " turn_1 ",
		Items:    []map[string]any{source, nil},
	})
	if len(events) != 1 {
		t.Fatalf("expected one completed item event: %#v", events)
	}
	event := events[0]
	if event["kind"] != "item_completed" || event["threadId"] != "thread_1" || event["turnId"] != "turn_1" || event["itemId"] != "item_1" {
		t.Fatalf("unexpected item event: %#v", event)
	}
	item, _ := event["item"].(map[string]any)
	item["text"] = "mutated"
	if source["text"] != "done" {
		t.Fatalf("event item should not alias source item: %#v", source)
	}
}

func TestBuildTurnAbortedEventAndResponse(t *testing.T) {
	event := BuildTurnAbortedEvent(TurnAbortedEventInput{
		ThreadID:              " thread_1 ",
		TurnID:                " turn_1 ",
		Discard:               true,
		Cancelled:             true,
		CancelledPendingGates: 2,
	})
	if event["kind"] != "turn_aborted" || event["threadId"] != "thread_1" || event["turnId"] != "turn_1" || event["status"] != "aborted" {
		t.Fatalf("unexpected abort event identity: %#v", event)
	}
	if event["discard"] != true || event["cancelled"] != true || event["cancelledPendingGates"] != 2 {
		t.Fatalf("unexpected abort event flags: %#v", event)
	}
	response := InterruptAcceptedResponse(InterruptAcceptedResponseInput{
		ThreadID:  " thread_1 ",
		TurnID:    " turn_1 ",
		Status:    " aborted ",
		Discard:   true,
		Cancelled: true,
	})
	if response["threadId"] != "thread_1" || response["turnId"] != "turn_1" || response["status"] != "aborted" || response["discard"] != true || response["cancelled"] != true {
		t.Fatalf("unexpected interrupt response: %#v", response)
	}
	if fields := AbortedTurnFields(true); len(fields) != 1 || fields["discard"] != true {
		t.Fatalf("unexpected aborted turn fields: %#v", fields)
	}
}
