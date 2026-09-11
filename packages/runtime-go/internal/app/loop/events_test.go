package loop

import (
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

func TestBuildAssistantDeltaEventCarriesRunningItem(t *testing.T) {
	event := BuildAssistantDeltaEvent(AssistantDeltaEventInput{
		ThreadID:  "thread-1",
		TurnID:    "turn-1",
		EventKind: "assistant_text_delta",
		ItemKind:  "assistant_text",
		Text:      "hello",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if event["kind"] != "assistant_text_delta" || event["itemId"] != "item_turn-1_assistant_text" {
		t.Fatalf("unexpected assistant delta event: %#v", event)
	}
	item, _ := event["item"].(map[string]any)
	if item["status"] != "running" || item["text"] != "hello" || item["createdAt"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected assistant delta item: %#v", item)
	}
}

func TestBuildStreamInterruptedRecoveryEventClonesProviderDiagnostic(t *testing.T) {
	diagnostic := map[string]any{
		"status": float64(503), "kind": "server", "retryable": true,
		"requestUrl": "https://example.test/v1/chat/completions/6222021234567890",
		"message":    "SOL_PRIVATE_TRACE_7C",
	}
	event := BuildStreamInterruptedRecoveryEvent(StreamInterruptedRecoveryEventInput{
		ThreadID:           "thread-1",
		TurnID:             "turn-1",
		Attempt:            2,
		MaxAttempts:        3,
		PartialToolStarted: true,
		Message:            "unexpected eof",
		ProviderDiagnostic: diagnostic,
	})
	diagnostic["status"] = float64(200)

	if event["stage"] != "provider_retrying" || event["label"] != "Recovering interrupted provider stream" {
		t.Fatalf("unexpected interrupted recovery event: %#v", event)
	}
	details, _ := event["details"].(map[string]any)
	if details["message"] != "The provider stream was interrupted before a verified response was available." ||
		details["visibleRecovery"] != true ||
		details["recoveryKind"] != "interrupted_stream" ||
		details["recoveryAttempt"] != float64(2) ||
		details["maxRecoveryAttempt"] != float64(3) ||
		details["partialToolStarted"] != true {
		t.Fatalf("unexpected interrupted recovery details: %#v", details)
	}
	providerError, _ := details["providerError"].(map[string]any)
	if providerError["status"] != float64(503) || providerError["kind"] != "server" || providerError["retryable"] != true {
		t.Fatalf("provider diagnostic should be typed and cloned: %#v", providerError)
	}
	if _, exists := providerError["requestUrl"]; exists {
		t.Fatalf("provider URL escaped typed diagnostics: %#v", providerError)
	}
}

func TestBuildProviderRetryingEventIsAClosedPublicRecord(t *testing.T) {
	event := BuildProviderRetryingEvent(ProviderRetryingEventInput{
		ThreadID: "thread-1", TurnID: "turn-1", Attempt: 2, MaxAttempts: 3,
		ProviderDiagnostic: map[string]any{
			"status": float64(503), "kind": "server", "retryable": true,
			"hasApiKey": false, "authStatus": "none", "endpointFormat": "chat_completions",
		},
	})
	if err := domainevent.ValidatePublicRecord(event); err != nil {
		t.Fatalf("provider retry event is not a closed public record: event=%#v err=%v", event, err)
	}
}

func TestBuildLoopRecoveryEventsExposeVisibleRecoveryContract(t *testing.T) {
	stepLimit := BuildStepLimitFinalAnswerRecoveryEvent("thread-1", "turn-1", 4)
	stepDetails, _ := stepLimit["details"].(map[string]any)
	if stepLimit["stage"] != "step_limit_finalizing" || stepDetails["recoveryKind"] != "step_limit_final_answer" || stepDetails["maxModelSteps"] != float64(4) {
		t.Fatalf("unexpected step-limit event: %#v", stepLimit)
	}

	empty := BuildEmptyFinalRecoveryEvent(EmptyFinalRecoveryEventInput{ThreadID: "thread-1", TurnID: "turn-1", Attempt: 1, MaxAttempts: 1})
	emptyDetails, _ := empty["details"].(map[string]any)
	if empty["stage"] != "empty_final_recovered" || emptyDetails["visibleRecovery"] != true || emptyDetails["recoveryAttempt"] != float64(1) {
		t.Fatalf("unexpected empty-final recovery event: %#v", empty)
	}

	exhausted := BuildEmptyFinalRecoveryExhaustedEvent(EmptyFinalRecoveryEventInput{ThreadID: "thread-1", TurnID: "turn-1", Attempt: 1, MaxAttempts: 1})
	exhaustedDetails, _ := exhausted["details"].(map[string]any)
	if exhausted["stage"] != "provider_error" || exhaustedDetails["recoveryExhausted"] != true ||
		exhaustedDetails["reasonCode"] != domainfailure.CodeProviderEmptyFinal {
		t.Fatalf("unexpected empty-final exhausted event: %#v", exhausted)
	}
}

func TestBuildProviderErrorEventPreservesClosedFailureReason(t *testing.T) {
	t.Parallel()
	event := BuildProviderErrorEvent(
		"thread-1",
		"turn-1",
		domainfailure.New(domainfailure.CodeProviderReasoningMarkupInvalid, nil),
		nil,
	)
	details, _ := event["details"].(map[string]any)
	if details["reasonCode"] != domainfailure.CodeProviderReasoningMarkupInvalid ||
		details["message"] != "The provider returned invalid private-reasoning markup; the response was blocked." {
		t.Fatalf("provider protocol failure was downgraded: %#v", event)
	}
}

func TestBuildLoopGuardEvent(t *testing.T) {
	guard := BuildLoopGuardEvent("thread-1", "turn-1", "read_file", 3, "tool_failure")
	guardDetails, _ := guard["details"].(map[string]any)
	if guard["stage"] != "loop_guard" || guardDetails["toolName"] != "read_file" || guardDetails["stormCount"] != float64(3) {
		t.Fatalf("unexpected loop guard event: %#v", guard)
	}
}
