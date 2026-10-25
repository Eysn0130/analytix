package thread

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func feedbackEventFixtureV1(kind string) map[string]any {
	event := map[string]any{"kind": kind, "threadId": "thread.1", "threadCreatedAt": "2026-10-10T00:00:00Z", "timestamp": "2026-10-10T00:00:01Z", "seq": 1}
	if kind == MessageFeedbackPutKindV1 {
		event["feedback"] = map[string]any{"messageId": "message.1", "rating": "positive", "version": "11111111-1111-4111-8111-111111111111", "createdAt": 1, "updatedAt": 1}
	} else {
		event["messageId"] = "message.1"
	}
	return event
}

func TestMessageFeedbackProjectionLifecycleV1(t *testing.T) {
	put := feedbackEventFixtureV1(MessageFeedbackPutKindV1)
	remove := feedbackEventFixtureV1(MessageFeedbackDeleteKindV1)
	remove["seq"] = 2
	for _, events := range [][]map[string]any{{put}, {put, remove}} {
		items, err := MessageFeedbackCurrentV1(events, "thread.1", "2026-10-10T00:00:00Z")
		if err != nil || len(items) != 2-len(events) {
			t.Fatalf("projection mismatch: items=%#v err=%v", items, err)
		}
	}
	if _, err := MessageFeedbackCurrentV1([]map[string]any{put}, "thread.1", "2026-10-11T00:00:00Z"); err == nil {
		t.Fatal("feedback crossed a recreated lifecycle")
	}
	for _, key := range []string{"seq", "timestamp", "threadCreatedAt", "feedback"} {
		bad := feedbackEventFixtureV1(MessageFeedbackPutKindV1)
		delete(bad, key)
		if ValidateMessageFeedbackEventV1(bad) == nil {
			t.Fatalf("missing %s was accepted", key)
		}
	}
	bad := feedbackEventFixtureV1(MessageFeedbackPutKindV1)
	bad["replyText"] = "must not store a copy"
	if ValidateMessageFeedbackEventV1(bad) == nil {
		t.Fatal("unknown event field accepted")
	}
}

func TestMessageFeedbackClosedBodyAndUTF8BytesV1(t *testing.T) {
	valid := strings.Repeat("界", 2730) + "ab"
	if err := ValidateMessageFeedbackPutV1(MessageFeedbackPutRequestV1{Rating: "positive", Note: &valid}); err != nil {
		t.Fatal(err)
	}
	tooLong := valid + "界"
	if err := ValidateMessageFeedbackPutV1(MessageFeedbackPutRequestV1{Rating: "positive", Note: &tooLong}); !errors.Is(err, ErrMessageFeedbackNoteTooLargeV1) {
		t.Fatal("UTF-8 byte limit not enforced")
	}
	for _, body := range []string{`{"rating":"positive","rating":"negative","ifVersion":null}`, `{"rating":"positive","unknown":true}`, `{"rating":"positive"} {}`, `{"note":"\ud800"}`} {
		if DecodeMessageFeedbackV1([]byte(body), new(MessageFeedbackPutRequestV1)) == nil {
			t.Fatal("ambiguous body accepted")
		}
	}
	item := feedbackEventFixtureV1(MessageFeedbackPutKindV1)["feedback"].(map[string]any)
	delete(item, "createdAt")
	if _, err := ParseMessageFeedbackItemV1(item); err == nil {
		t.Fatal("missing timestamp accepted")
	}
	body, _ := json.Marshal(feedbackEventFixtureV1(MessageFeedbackPutKindV1))
	if !json.Valid(body) {
		t.Fatal("fixture is invalid")
	}
}
