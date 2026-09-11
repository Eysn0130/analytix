package turn

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCaseDraftAndUngatedTerminalEventsFailClosed(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	turn := map[string]any{"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext)}
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{turn}}
	for _, event := range []map[string]any{
		{"kind": "assistant_text_delta", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "delta": "金额 420 万元"},
		{"kind": "assistant_reasoning_delta", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "delta": "private"},
		{"kind": "turn_completed", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID},
	} {
		if err := ValidateCaseEventPublication(thread, event); err == nil {
			t.Fatalf("ungated case event was accepted: %#v", event)
		}
	}
}

func TestAuditOnlyV1CaseContextCannotPublishAnyCaseEvent(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", CaseID: "case-v1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-v1-binding")), DatasetSnapshotID: "legacy-snapshot-v1",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("legacy-source")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	thread := map[string]any{"id": legacy.ThreadID, "turns": []any{map[string]any{
		"id": legacy.TurnID, "securityContext": turnSecurityContextRecord(legacy),
	}}}
	if err := ValidateCaseEventPublication(thread, map[string]any{
		"kind": "tool_progress", "threadId": legacy.ThreadID, "turnId": legacy.TurnID,
	}); err == nil {
		t.Fatal("audit-only V1 case context published a live case event")
	}
}

func TestBoundaryOnlyV2CannotPublishDraftOrUngatedTerminalEvent(t *testing.T) {
	boundary := newTurnBoundaryOnlyContextV2(t, "thread-boundary", "turn-boundary", "/workspace", 2, time.Now().UTC())
	turn := map[string]any{"id": boundary.TurnID, "securityContext": turnSecurityContextRecord(boundary)}
	thread := map[string]any{"id": boundary.ThreadID, "turns": []any{turn}}
	for _, event := range []map[string]any{
		{"kind": "assistant_text_delta", "threadId": boundary.ThreadID, "turnId": boundary.TurnID, "delta": "金额 420 万元"},
		{"kind": "turn_failed", "threadId": boundary.ThreadID, "turnId": boundary.TurnID},
	} {
		if err := ValidateCaseEventPublication(thread, event); err == nil {
			t.Fatalf("boundary-only V2 published an ungated event: %#v", event)
		}
	}
}

func TestProviderErrorPipelineStageSanitizerPreservesOnlyClosedReasonCode(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-provider-error", "turn-provider-error", 2)
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{
		map[string]any{"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "securityContext": turnSecurityContextRecord(securityContext)},
	}}
	event := map[string]any{
		"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"stage": "provider_error", "label": "ATTACKER_LABEL_SENTINEL",
		"details": map[string]any{
			"reasonCode": domainfailure.CodeProviderUnavailable,
			"providerError": map[string]any{
				"kind": "server", "status": float64(503), "retryable": true, "authStatus": "none",
				"failureStage": "transport_after_observed_send", "dispatchState": "sent", "attempt": float64(1),
			},
			"message":       "PRIVATE_PROVIDER_MESSAGE_SENTINEL",
			"providerBody":  "PRIVATE_PROVIDER_BODY_SENTINEL",
			"endpoint":      "https://provider.invalid/private-endpoint",
			"apiKey":        "PRIVATE_API_KEY_SENTINEL",
			"modelRequest":  "PRIVATE_MODEL_REQUEST_SENTINEL",
			"modelResponse": "PRIVATE_MODEL_RESPONSE_SENTINEL",
		},
	}
	projected, err := SanitizeCaseEventPublication(thread, event)
	if err != nil {
		t.Fatalf("provider error case event was rejected: %v", err)
	}
	expected := map[string]any{
		"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"stage": "provider_error", "label": "Provider stream failed",
		"details": map[string]any{
			"reasonCode": domainfailure.CodeProviderUnavailable,
			"providerError": map[string]any{
				"kind": "server", "status": float64(503), "retryable": true, "authStatus": "none",
				"failureStage": "transport_after_observed_send", "dispatchState": "sent", "attempt": float64(1),
			},
		},
	}
	if !reflect.DeepEqual(projected, expected) {
		t.Fatalf("provider error case projection was not minimal: %#v", projected)
	}
	body, _ := json.Marshal(projected)
	for _, sentinel := range []string{
		"ATTACKER_LABEL_SENTINEL", "PRIVATE_PROVIDER_MESSAGE_SENTINEL", "PRIVATE_PROVIDER_BODY_SENTINEL",
		"PRIVATE_API_KEY_SENTINEL", "PRIVATE_MODEL_REQUEST_SENTINEL", "PRIVATE_MODEL_RESPONSE_SENTINEL",
	} {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("provider error case projection leaked %s: %s", sentinel, body)
		}
	}

	recoveryEvent := map[string]any{
		"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"stage": "provider_error", "label": "Provider returned an empty final response",
		"details": map[string]any{
			"reasonCode": domainfailure.CodeProviderEmptyFinal, "message": "PRIVATE_EMPTY_FINAL_MESSAGE",
			"visibleRecovery": true, "recoveryKind": "empty_final", "recoveryAttempt": float64(1),
			"maxRecoveryAttempts": float64(1), "recoveryExhausted": true,
		},
	}
	recoveryProjection, recoveryErr := SanitizeCaseEventPublication(thread, recoveryEvent)
	if recoveryErr != nil {
		t.Fatalf("closed empty-final recovery diagnostic was rejected: %v", recoveryErr)
	}
	recoveryDetails, _ := recoveryProjection["details"].(map[string]any)
	if recoveryProjection["label"] != "Provider stream failed" ||
		recoveryDetails["reasonCode"] != domainfailure.CodeProviderEmptyFinal ||
		recoveryDetails["visibleRecovery"] != true || recoveryDetails["recoveryKind"] != "empty_final" ||
		recoveryDetails["recoveryAttempt"] != float64(1) ||
		recoveryDetails["maxRecoveryAttempts"] != float64(1) ||
		recoveryDetails["recoveryExhausted"] != true || len(recoveryDetails) != 6 {
		t.Fatalf("empty-final recovery diagnostic was not closed: %#v", recoveryProjection)
	}
	recoveryBody, _ := json.Marshal(recoveryProjection)
	if strings.Contains(string(recoveryBody), "PRIVATE_EMPTY_FINAL_MESSAGE") {
		t.Fatalf("empty-final recovery diagnostic leaked the private message: %#v", recoveryProjection)
	}

	preSendEvent := map[string]any{
		"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"stage": "provider_error", "details": map[string]any{
			"reasonCode": domainfailure.CodeProviderError,
			"providerError": map[string]any{
				"failureStage": "telemetry_begin", "dispatchState": "not_sent", "attempt": float64(1),
			},
		},
	}
	preSendProjection, preSendErr := SanitizeCaseEventPublication(thread, preSendEvent)
	if preSendErr != nil {
		t.Fatalf("closed pre-send attribution was rejected: %v", preSendErr)
	}
	preSendDetails := preSendProjection["details"].(map[string]any)["providerError"].(map[string]any)
	if preSendDetails["failureStage"] != "telemetry_begin" ||
		preSendDetails["dispatchState"] != "not_sent" || preSendDetails["attempt"] != float64(1) {
		t.Fatalf("closed pre-send attribution changed: %#v", preSendProjection)
	}

	for _, reasonCode := range []any{
		"provider_reason_was_invented", "Provider_Unavailable", " provider_unavailable", float64(1),
	} {
		candidate := map[string]any{
			"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"stage": "provider_error", "details": map[string]any{"reasonCode": reasonCode},
		}
		if candidateProjection, candidateErr := SanitizeCaseEventPublication(thread, candidate); candidateErr == nil || candidateProjection != nil {
			t.Fatalf("non-canonical provider reason code was accepted: reason=%#v projection=%#v err=%v", reasonCode, candidateProjection, candidateErr)
		}
	}

	for name, candidate := range map[string]map[string]any{
		"missing reason": {
			"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"stage": "provider_error", "details": map[string]any{},
		},
		"non-canonical kind": {
			"kind": " pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"stage": "provider_error", "details": map[string]any{"reasonCode": domainfailure.CodeProviderUnavailable},
		},
		"non-canonical stage": {
			"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"stage": "provider_error ", "details": map[string]any{"reasonCode": domainfailure.CodeProviderUnavailable},
		},
		"non-canonical thread": {
			"kind": "pipeline_stage", "threadId": securityContext.ThreadID + " ", "turnId": securityContext.TurnID,
			"stage": "provider_error", "details": map[string]any{"reasonCode": domainfailure.CodeProviderUnavailable},
		},
		"non-canonical turn": {
			"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": " " + securityContext.TurnID,
			"stage": "provider_error", "details": map[string]any{"reasonCode": domainfailure.CodeProviderUnavailable},
		},
		"open provider diagnostic": {
			"kind": "pipeline_stage", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"stage": "provider_error", "details": map[string]any{
				"reasonCode": domainfailure.CodeProviderUnavailable,
				"providerError": map[string]any{
					"kind": "server", "status": float64(503), "endpoint": "https://provider.invalid/private",
				},
			},
		},
	} {
		if candidateProjection, candidateErr := SanitizeCaseEventPublication(thread, candidate); candidateErr == nil || candidateProjection != nil {
			t.Fatalf("%s provider error schema was accepted: projection=%#v err=%v", name, candidateProjection, candidateErr)
		}
	}
}

func TestUnknownTurnCaseDraftFailsClosed(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{map[string]any{
		"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext),
	}}}
	for _, kind := range []string{"assistant_text_delta", "assistant_reasoning_delta"} {
		if err := ValidateCaseEventPublication(thread, map[string]any{
			"kind": kind, "threadId": securityContext.ThreadID, "turnId": "turn-forged", "text": "金额 420 万元",
		}); err == nil {
			t.Fatalf("unknown-turn case draft %s was accepted", kind)
		}
	}
	if err := ValidateCaseEventPublication(thread, map[string]any{
		"kind": "item_completed", "threadId": securityContext.ThreadID, "turnId": "turn-forged",
		"item": map[string]any{"kind": "assistant_text", "text": "金额 420 万元"},
	}); err == nil {
		t.Fatal("unknown-turn case assistant item was accepted")
	}
}

func TestCaseAcceptedFinalEventsMustMatchCommittedAuthority(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := signedAcceptedFinalForTest(t, securityContext, envelope, time.Unix(2, 0))
	intent := acceptedFinalIntentForTest(t, "success", time.Unix(2, 0))
	plan, err := BuildAcceptedFinalPublicationPlan(record, domainevidence.CaseUnverifiedText, intent)
	if err != nil {
		t.Fatal(err)
	}
	recordMap := domainevidence.AcceptedFinalRecordMap(record)
	turn := map[string]any{"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext), "acceptedFinal": recordMap}
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{turn}}
	if err := ValidateCaseEventPublication(thread, plan.Events[0].Draft); err != nil {
		t.Fatalf("valid accepted item event rejected: %v", err)
	}
	terminal := map[string]any{
		"kind": "turn_completed", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"acceptedFinalDigest": record.RecordDigest,
	}
	if err := ValidateCaseEventPublication(thread, terminal); err != nil {
		t.Fatalf("valid accepted terminal event rejected: %v", err)
	}
	terminal["acceptedFinalDigest"] = "provider-forged"
	if err := ValidateCaseEventPublication(thread, terminal); err == nil {
		t.Fatal("mismatched accepted-final terminal event was accepted")
	}
}

func TestEveryCasePublicEventKindUsesPositiveMetadataProjection(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	turn := map[string]any{
		"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext),
		"items": []any{map[string]any{"id": "user-a", "kind": "user_message", "role": "user", "text": "USER_REQUEST"}},
	}
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{turn}}
	for _, event := range []map[string]any{
		{"kind": "tool_call_finished", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"message": "AMOUNT_SENTINEL_4200000", "item": map[string]any{"id": "tool", "kind": "tool_result", "output": "ACCOUNT_SENTINEL_622202"}},
		{"kind": "tool_progress", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "message": "MAC_SENTINEL_00:11:22:33:44:55"},
		{"kind": "user_input_requested", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"prompt": "RELATION_SENTINEL", "questions": []any{map[string]any{"question": "RELATION_SENTINEL"}}},
		{"kind": "goal_updated", "threadId": securityContext.ThreadID, "goal": map[string]any{"objective": "QUOTE_SENTINEL_7654321"}},
	} {
		projected, err := SanitizeCaseEventPublication(thread, event)
		if err != nil {
			t.Fatalf("case event metadata projection failed for %#v: %v", event, err)
		}
		body, _ := json.Marshal(projected)
		for _, sentinel := range []string{"AMOUNT_SENTINEL", "ACCOUNT_SENTINEL", "MAC_SENTINEL", "RELATION_SENTINEL", "QUOTE_SENTINEL", "ERROR_AMOUNT_SENTINEL"} {
			if strings.Contains(string(body), sentinel) {
				t.Fatalf("case event projection leaked %s: %s", sentinel, body)
			}
		}
	}
	rawFailure := map[string]any{
		"kind": "error", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"message": "ERROR_AMOUNT_SENTINEL_998877",
	}
	if projected, err := SanitizeCaseEventPublication(thread, rawFailure); err == nil || projected != nil {
		t.Fatalf("raw case failure content was not rejected: projected=%#v err=%v", projected, err)
	}

	ordinary := map[string]any{"id": "thread-ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	raw := map[string]any{
		"kind": "tool_progress", "threadId": "thread-ordinary", "turnId": "turn-ordinary",
		"message": "ORDINARY_PROGRESS 卡号 6222020000000000000",
		"details": map[string]any{"opaque": "电话 13800138000"},
	}
	projected, err := SanitizeCaseEventPublication(ordinary, raw)
	if err != nil || projected["message"] != "ORDINARY_PROGRESS 卡号 [ACCOUNT]" ||
		projected["details"].(map[string]any)["opaque"] != "电话 [PHONE]" {
		t.Fatalf("ordinary event projection regressed: projected=%#v err=%v", projected, err)
	}
}

func TestOrdinaryEventRejectsRestrictedEvidenceBeforePersistence(t *testing.T) {
	thread := map[string]any{
		"id":    "thread-ordinary-restricted",
		"turns": []any{map[string]any{"id": "turn-ordinary-restricted", "items": []any{}}},
	}
	event := map[string]any{
		"kind": "review_completed", "threadId": "thread-ordinary-restricted", "turnId": "turn-ordinary-restricted",
		"review": map[string]any{"output": map[string]any{
			"schemaVersion": 1, "purpose": "analytix.parsed-generation-receipt/v1",
			"receiptDigest": strings.Repeat("a", 64),
		}},
	}
	if projected, err := SanitizeCaseEventPublication(thread, event); err == nil || projected != nil {
		t.Fatalf("restricted evidence reached ordinary event persistence: projected=%#v err=%v", projected, err)
	}

	serialized := map[string]any{
		"kind": "review_completed", "threadId": "thread-ordinary-restricted", "turnId": "turn-ordinary-restricted",
		"review": map[string]any{"output": `{"lineageDigest":"` + strings.Repeat("a", 64) +
			`","lineageSha256":"` + strings.Repeat("b", 64) + `","lineageByteLength":3}`},
	}
	if projected, err := SanitizeCaseEventPublication(thread, serialized); err == nil || projected != nil {
		t.Fatalf("serialized restricted evidence reached ordinary event persistence: projected=%#v err=%v", projected, err)
	}
}

func TestFakeCaseUserMessageEventIsRejected(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	trustedUser := map[string]any{
		"id": "user-a", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"kind": "user_message", "role": "user", "status": "completed", "text": "trusted request",
	}
	thread := map[string]any{"id": securityContext.ThreadID, "turns": []any{map[string]any{
		"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext), "items": []any{trustedUser},
	}}}
	forged := map[string]any{
		"kind": "item_completed", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"item": map[string]any{
			"id": "forged", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
			"kind": "user_message", "role": "user", "status": "completed", "text": "账户 622202 金额 4200000",
		},
	}
	if projected, err := SanitizeCaseEventPublication(thread, forged); err == nil || projected != nil {
		t.Fatalf("forged case user attribution was accepted: projected=%#v err=%v", projected, err)
	}
}

func TestGenericEventProjectionRejectsAcceptedFinalMarkersBeforePersistence(t *testing.T) {
	thread := map[string]any{"id": "thread-generic", "turns": []any{map[string]any{"id": "turn-generic"}}}
	for _, marker := range []string{
		"acceptedFinal", "acceptedFinalView", "acceptedFinalDigest", "publicationCommitId",
		"publicationEventId", "publicationSlot", "publicationPayloadDigest",
	} {
		event := map[string]any{
			"kind": "checkpoint_captured", "threadId": "thread-generic", "turnId": "turn-generic",
			"checkpoint": map[string]any{"status": "captured", marker: "forged"},
		}
		if projected, err := SanitizeGenericCaseEventPublication(thread, event); err == nil || projected != nil {
			t.Fatalf("generic projection accepted nested marker %s: projected=%#v err=%v", marker, projected, err)
		}
	}
	arrayEvent := map[string]any{
		"kind": "checkpoint_captured", "threadId": "thread-generic", "turnId": "turn-generic",
		"checkpoint": []map[string]any{{"publicationCommitId": "forged"}},
	}
	if projected, err := SanitizeGenericCaseEventPublication(thread, arrayEvent); err == nil || projected != nil {
		t.Fatalf("generic projection accepted marker in []map: projected=%#v err=%v", projected, err)
	}
}
