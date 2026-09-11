package thread

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	appusage "analytix.local/runtime-go/internal/app/usage"
	"analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestProviderErrorPipelineStagePublicProjectionIsClosedForOrdinaryAndCaseThreads(t *testing.T) {
	const (
		threadID  = "ordinary-provider-error"
		turnID    = "turn-provider-error"
		timestamp = "2026-08-12T00:00:00Z"
	)
	ordinary := map[string]any{"id": threadID, "turns": []any{map[string]any{"id": turnID, "threadId": threadID, "items": []any{}}}}
	ordinaryEvent := map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID, "seq": float64(7), "timestamp": timestamp,
		"stage": "provider_error", "label": "ATTACKER_LABEL_SENTINEL",
		"details": map[string]any{
			"reasonCode": domainfailure.CodeProviderTimeout,
			"providerError": map[string]any{
				"kind": "network", "retryable": true, "authStatus": "none",
				"failureStage": "transport_before_observed_send", "dispatchState": "indeterminate", "attempt": float64(1),
			},
			"message":       "PRIVATE_PROVIDER_MESSAGE_SENTINEL",
			"providerBody":  "PRIVATE_PROVIDER_BODY_SENTINEL",
			"endpoint":      "https://provider.invalid/private-endpoint",
			"apiKey":        "PRIVATE_API_KEY_SENTINEL",
			"modelRequest":  "PRIVATE_MODEL_REQUEST_SENTINEL",
			"modelResponse": "PRIVATE_MODEL_RESPONSE_SENTINEL",
		},
	}
	projected, visible, err := ProjectPublicThreadEvent(threadID, ordinary, ordinaryEvent)
	if err != nil || !visible {
		t.Fatalf("ordinary provider error event was not visible: projected=%#v visible=%t err=%v", projected, visible, err)
	}
	expected := map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID, "seq": float64(7), "timestamp": timestamp,
		"stage": "provider_error", "label": "Provider stream failed",
		"details": map[string]any{
			"reasonCode": domainfailure.CodeProviderTimeout,
			"providerError": map[string]any{
				"kind": "network", "retryable": true, "authStatus": "none",
				"failureStage": "transport_before_observed_send", "dispatchState": "indeterminate", "attempt": float64(1),
			},
		},
	}
	if !reflect.DeepEqual(projected, expected) {
		t.Fatalf("ordinary provider error projection was not strict: %#v", projected)
	}
	body, _ := json.Marshal(projected)
	for _, sentinel := range []string{
		"ATTACKER_LABEL_SENTINEL", "PRIVATE_PROVIDER_MESSAGE_SENTINEL", "PRIVATE_PROVIDER_BODY_SENTINEL",
		"PRIVATE_API_KEY_SENTINEL", "PRIVATE_MODEL_REQUEST_SENTINEL", "PRIVATE_MODEL_RESPONSE_SENTINEL",
	} {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("ordinary provider error projection leaked %s: %s", sentinel, body)
		}
	}

	caseContext := newTrustedProjectionContext("case-provider-error", "turn-case-provider-error", "case-provider-error", "snapshot-provider-error", 2)
	caseThread := map[string]any{
		"id": caseContext.ThreadID, "securityState": publicProjectionSecurityRecord(caseContext),
		"turns": []any{map[string]any{
			"id": caseContext.TurnID, "threadId": caseContext.ThreadID,
			"securityContext": publicProjectionSecurityRecord(caseContext), "items": []any{},
		}},
	}
	caseEvent := contracts.CloneMap(ordinaryEvent)
	caseEvent["threadId"] = caseContext.ThreadID
	caseEvent["turnId"] = caseContext.TurnID
	projected, visible, err = ProjectPublicThreadEvent(caseContext.ThreadID, caseThread, caseEvent)
	if err != nil || !visible {
		t.Fatalf("case provider error event was not visible: projected=%#v visible=%t err=%v", projected, visible, err)
	}
	expected = map[string]any{
		"kind": "pipeline_stage", "threadId": caseContext.ThreadID, "turnId": caseContext.TurnID,
		"seq": float64(7), "timestamp": timestamp, "stage": "provider_error", "label": "Provider stream failed",
		"details": map[string]any{
			"reasonCode": domainfailure.CodeProviderTimeout,
			"providerError": map[string]any{
				"kind": "network", "retryable": true, "authStatus": "none",
				"failureStage": "transport_before_observed_send", "dispatchState": "indeterminate", "attempt": float64(1),
			},
		},
	}
	if !reflect.DeepEqual(projected, expected) {
		t.Fatalf("case provider error projection was not strict: %#v", projected)
	}

	settledCaseThread, _, _ := canonicalGeneralTerminalPublicFixture(t, "failed", "provider_failure")
	settledCaseThread["caseId"] = "case-provider-error-history"
	settledTurn := settledCaseThread["turns"].([]any)[0].(map[string]any)
	settledEvent := contracts.CloneMap(ordinaryEvent)
	settledEvent["threadId"] = contracts.StringField(settledCaseThread, "id")
	settledEvent["turnId"] = contracts.StringField(settledTurn, "id")
	projected, visible, err = ProjectPublicThreadEvent(
		contracts.StringField(settledCaseThread, "id"), settledCaseThread, settledEvent,
	)
	if err != nil || !visible {
		t.Fatalf("settled case-history provider error was not visible: projected=%#v visible=%t err=%v", projected, visible, err)
	}
	settledDetails, _ := projected["details"].(map[string]any)
	if settledDetails["reasonCode"] != domainfailure.CodeProviderTimeout || len(settledDetails) != 2 {
		t.Fatalf("settled case-history provider error was not closed: %#v", projected)
	}

	for _, reasonCode := range []any{"provider_reason_was_invented", "Provider_Timeout", " provider_timeout", float64(1)} {
		candidate := contracts.CloneMap(ordinaryEvent)
		candidate["details"] = map[string]any{"reasonCode": reasonCode}
		if candidateProjection, candidateVisible, candidateErr := ProjectPublicThreadEvent(threadID, ordinary, candidate); candidateErr != nil || candidateVisible || candidateProjection != nil {
			t.Fatalf("ordinary non-canonical provider reason code was published: reason=%#v projection=%#v visible=%t err=%v", reasonCode, candidateProjection, candidateVisible, candidateErr)
		}

		caseCandidate := contracts.CloneMap(caseEvent)
		caseCandidate["details"] = map[string]any{"reasonCode": reasonCode}
		if candidateProjection, candidateVisible, candidateErr := ProjectPublicThreadEvent(caseContext.ThreadID, caseThread, caseCandidate); candidateErr != nil || candidateVisible || candidateProjection != nil {
			t.Fatalf("case non-canonical provider reason code was published: reason=%#v projection=%#v visible=%t err=%v", reasonCode, candidateProjection, candidateVisible, candidateErr)
		}
	}

	for name, candidate := range map[string]map[string]any{
		"missing reason": {
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
			"seq": float64(8), "timestamp": timestamp, "stage": "provider_error", "details": map[string]any{},
		},
		"non-canonical kind": {
			"kind": "pipeline_stage ", "threadId": threadID, "turnId": turnID,
			"seq": float64(8), "timestamp": timestamp, "stage": "provider_error",
			"details": map[string]any{"reasonCode": domainfailure.CodeProviderTimeout},
		},
		"non-canonical stage": {
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
			"seq": float64(8), "timestamp": timestamp, "stage": " provider_error",
			"details": map[string]any{"reasonCode": domainfailure.CodeProviderTimeout},
		},
		"half transport identity": {
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
			"seq": float64(8), "stage": "provider_error",
			"details": map[string]any{"reasonCode": domainfailure.CodeProviderTimeout},
		},
		"open provider diagnostic": {
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
			"seq": float64(8), "timestamp": timestamp, "stage": "provider_error",
			"details": map[string]any{
				"reasonCode": domainfailure.CodeProviderTimeout,
				"providerError": map[string]any{
					"kind": "network", "endpoint": "https://provider.invalid/private",
				},
			},
		},
	} {
		if candidateProjection, candidateVisible, candidateErr := ProjectPublicThreadEvent(threadID, ordinary, candidate); candidateErr != nil || candidateVisible || candidateProjection != nil {
			t.Fatalf("ordinary %s provider error schema was published: projection=%#v visible=%t err=%v", name, candidateProjection, candidateVisible, candidateErr)
		}
	}
}

func TestCasePublicEventProjectionRejectsPriorAssistantAndToolFacts(t *testing.T) {
	current := newTrustedProjectionContext("thread-a", "turn-case", "case-a", "snapshot-a", 2)
	unbound := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-before-case", WorkspaceRealPath: "/cases/a", ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	userItem := map[string]any{
		"id": "user-case", "threadId": current.ThreadID, "turnId": current.TurnID, "kind": "user_message",
		"role": "user", "status": "completed", "text": "USER_CASE_REQUEST",
	}
	thread := map[string]any{
		"id": current.ThreadID, "securityState": publicProjectionSecurityRecord(current),
		"turns": []any{
			map[string]any{"id": unbound.TurnID, "securityContext": publicProjectionSecurityRecord(unbound), "items": []any{
				map[string]any{"id": "old-assistant", "kind": "assistant_text", "text": "OLD_AMOUNT_SENTINEL_4200000"},
			}},
			map[string]any{"id": current.TurnID, "securityContext": publicProjectionSecurityRecord(current), "items": []any{userItem}},
		},
	}
	for _, event := range []map[string]any{
		{"kind": "assistant_text_delta", "threadId": current.ThreadID, "turnId": unbound.TurnID, "text": "OLD_AMOUNT_SENTINEL_4200000"},
		{"kind": "item_completed", "threadId": current.ThreadID, "turnId": unbound.TurnID,
			"item": map[string]any{"id": "old-assistant", "kind": "assistant_text", "text": "OLD_ACCOUNT_SENTINEL_622202"}},
		{"kind": "tool_call_finished", "threadId": current.ThreadID, "turnId": current.TurnID,
			"item": map[string]any{"id": "tool", "kind": "tool_result", "output": "MAC_SENTINEL_00:11:22:33:44:55"}},
	} {
		projected, visible, err := ProjectPublicThreadEvent(current.ThreadID, thread, event)
		if err != nil || visible || projected != nil {
			t.Fatalf("case fact event remained public: event=%#v projected=%#v visible=%t err=%v", event, projected, visible, err)
		}
	}
	userEvent := map[string]any{
		"kind": "item_created", "threadId": current.ThreadID, "turnId": current.TurnID, "itemId": "user-case", "item": userItem,
	}
	projected, visible, err := ProjectPublicThreadEvent(current.ThreadID, thread, userEvent)
	body, _ := json.Marshal(projected)
	if err != nil || visible || projected != nil || strings.Contains(string(body), "USER_CASE_REQUEST") {
		t.Fatalf("case user event without an admission receipt was projected: %s visible=%t err=%v", body, visible, err)
	}

	ordinary := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	raw := map[string]any{
		"kind": "assistant_text_delta", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "assistant-delta",
		"item": map[string]any{
			"id": "assistant-delta", "threadId": "ordinary", "turnId": "turn-ordinary", "role": "assistant",
			"status": "running", "createdAt": "2026-07-14T00:00:00Z", "kind": "assistant_text", "text": "ordinary answer",
		},
	}
	projected, visible, err = ProjectPublicThreadEvent("ordinary", ordinary, raw)
	if err != nil || visible || projected != nil {
		t.Fatalf("ordinary provider draft became public: %#v visible=%t err=%v", projected, visible, err)
	}
	if projected, visible, err := ProjectPublicThreadEvent("thread-b", thread, userEvent); err != nil || visible || projected != nil {
		t.Fatalf("cross-route thread event identity was accepted: projected=%#v visible=%t err=%v", projected, visible, err)
	}
}

func TestOrdinaryEventProjectionUsesClosedKindsAndDropsPrivateAuthority(t *testing.T) {
	const sentinel = "PRIVATE_EVENT_AUTHORITY_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	for _, event := range []map[string]any{
		{
			"kind": "tool_call_ready", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "item-call",
			"toolName": "bash", "callId": "call-1", "readyCount": float64(1), "seq": float64(3),
			"executionGrant": map[string]any{"argsHash": sentinel}, "securityContext": map[string]any{"workspaceRealPath": sentinel},
			"contextDigest": sentinel, "contextEpochSnapshot": map[string]any{"private": sentinel}, "unexpected": sentinel,
		},
		{
			"kind": "turn_started", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "running", "seq": float64(4),
			"securityContext": map[string]any{"workspaceRealPath": sentinel}, "contextEpochSnapshot": map[string]any{"private": sentinel},
			"executionGrant": map[string]any{"grantId": sentinel}, "unexpected": sentinel,
		},
		{
			"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary", "stage": "provider_retrying",
			"label": "Retrying", "details": map[string]any{
				"visibleRecovery": true, "recoveryKind": "interrupted_stream", "message": sentinel, "raw": sentinel,
			},
		},
	} {
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || projected == nil || strings.Contains(string(body), sentinel) ||
			strings.Contains(string(body), "executionGrant") || strings.Contains(string(body), "securityContext") ||
			strings.Contains(string(body), "contextEpochSnapshot") || strings.Contains(string(body), "unexpected") {
			t.Fatalf("ordinary event was not closed: event=%#v projected=%s visible=%t err=%v", event, body, visible, err)
		}
	}
	unknown := map[string]any{
		"kind": "future_private_event", "threadId": "ordinary", "turnId": "turn-ordinary", "secret": sentinel,
		"item": map[string]any{"kind": "tool_call", "executionGrant": map[string]any{"grantId": sentinel}},
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, unknown); err != nil || visible || projected != nil {
		t.Fatalf("unknown ordinary event did not fail closed: projected=%#v visible=%t err=%v", projected, visible, err)
	}
}

func TestOrdinaryErrorProjectionOnlyRetainsClosedUnadvertisedToolDetails(t *testing.T) {
	diagnostic := map[string]any{
		"rejectedToolNormalizedNameSha256":       strings.Repeat("1", 64),
		"rejectedToolCategory":                   "known_builtin_not_advertised",
		"promptRoute":                            "tool_agent",
		"loopStep":                               float64(1),
		"advertisedToolCount":                    float64(7),
		"advertisedToolManifestHash":             strings.Repeat("2", 64),
		"advertisedNameSetSortedHash":            strings.Repeat("3", 64),
		"providerRequestToolManifestHash":        strings.Repeat("4", 64),
		"runToolStepManifestHash":                strings.Repeat("4", 64),
		"providerRequestRunToolStepManifestSame": true,
	}
	if !domainfailure.ValidateToolNotAdvertisedDetails(diagnostic) {
		t.Fatalf("test diagnostic is not canonical: %#v", diagnostic)
	}

	for name, event := range map[string]map[string]any{
		"closed tool diagnostic": {
			"kind": "error", "threadId": "ordinary", "turnId": "turn-ordinary",
			"code": "tool_not_advertised", "message": "closed", "details": diagnostic,
		},
		"generic diagnostic": {
			"kind": "error", "threadId": "ordinary", "turnId": "turn-ordinary",
			"code": "provider_unavailable", "message": "closed", "details": map[string]any{"status": float64(500)},
		},
		"malformed tool diagnostic": {
			"kind": "error", "threadId": "ordinary", "turnId": "turn-ordinary",
			"code": "tool_not_advertised", "message": "closed", "details": map[string]any{"status": float64(500)},
		},
	} {
		projected, visible := projectOrdinaryPublicThreadEvent(event)
		if !visible || projected == nil {
			t.Fatalf("%s error event was not projected: projected=%#v visible=%t", name, projected, visible)
		}
		_, hasDetails := projected["details"]
		if name == "closed tool diagnostic" {
			if !hasDetails || !domainfailure.ValidateToolNotAdvertisedDetails(projected["details"].(map[string]any)) {
				t.Fatalf("closed tool diagnostic was lost: %#v", projected)
			}
		} else if hasDetails {
			t.Fatalf("%s crossed the public event boundary: %#v", name, projected)
		}
	}
}

func TestOrdinarySkillLifecycleUsesClosedPublicToolKind(t *testing.T) {
	const threadID = "ordinary"
	const turnID = "turn-ordinary"
	const timestamp = "2026-08-03T00:00:00Z"
	callID := "call_host_" + strings.Repeat("a", 64)
	thread := map[string]any{"id": threadID, "turns": []any{map[string]any{"id": turnID, "items": []any{}}}}

	callItemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	callEvent := map[string]any{
		"kind": "tool_call_started", "seq": float64(1), "timestamp": timestamp,
		"threadId": threadID, "turnId": turnID, "itemId": callItemID,
		"item": map[string]any{
			"id": callItemID, "threadId": threadID, "turnId": turnID,
			"createdAt": timestamp, "kind": "tool_call", "role": "tool", "status": "running",
			"toolName": "run_skill", "callId": callID, "toolKind": "skill",
			"arguments": map[string]any{"name": "private-skill-package"},
		},
	}
	projectedCall, visible, err := ProjectPublicThreadEvent(threadID, thread, callEvent)
	projectedCallItem, _ := projectedCall["item"].(map[string]any)
	if err != nil || !visible || projectedCallItem["toolKind"] != "tool_call" ||
		projectedCallItem["arguments"] == nil {
		t.Fatalf("skill call did not cross the closed public event seam: projected=%#v visible=%t err=%v", projectedCall, visible, err)
	}

	resultItemID := domaintoolresult.ToolResultItemIDV1(turnID, callID)
	resultEvent := map[string]any{
		"kind": "tool_call_finished", "seq": float64(2), "timestamp": timestamp,
		"threadId": threadID, "turnId": turnID, "itemId": resultItemID,
		"item": map[string]any{
			"id": resultItemID, "threadId": threadID, "turnId": turnID,
			"createdAt": timestamp, "finishedAt": timestamp, "kind": "tool_result", "role": "tool", "status": "completed",
			"toolName": "run_skill", "callId": callID, "toolKind": "skill", "isError": false,
			"output": domaintoolresult.PublicToolResultProjectionRecordV1(
				domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
			),
		},
	}
	projectedResult, visible, err := ProjectPublicThreadEvent(threadID, thread, resultEvent)
	projectedResultItem, _ := projectedResult["item"].(map[string]any)
	if err != nil || !visible || projectedResultItem["toolKind"] != "tool_call" ||
		projectedResultItem["status"] != "completed" {
		t.Fatalf("skill result did not cross the closed public event seam: projected=%#v visible=%t err=%v", projectedResult, visible, err)
	}
}

func TestOrdinaryEventAndToolPayloadGenericAcceptedFinalNamesUseClosedProjection(t *testing.T) {
	const (
		threadID  = "ordinary-generic-authority-names"
		turnID    = "turn-ordinary-generic-authority-names"
		timestamp = "2026-08-25T00:00:00Z"
	)
	callID := "call_host_" + strings.Repeat("b", 64)
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "items": []any{},
		}},
	}
	lifecycleEvent := map[string]any{
		"kind": "turn_started", "seq": float64(1), "timestamp": timestamp,
		"threadId": threadID, "turnId": turnID, "status": "running",
		"envelope":          map[string]any{"subject": "ORDINARY_EVENT_MAIL_SENTINEL"},
		"registryHead":      map[string]any{"branch": "ORDINARY_EVENT_REGISTRY_SENTINEL"},
		"publicationIntent": "ORDINARY_EVENT_INTENT_SENTINEL",
		"storeDigest":       "ORDINARY_EVENT_STORE_SENTINEL",
	}
	if domainevent.ContainsPrivateAcceptedFinalAuthority(lifecycleEvent) {
		t.Fatal("ordinary lifecycle generic field names were classified as private accepted-final authority")
	}
	projectedLifecycle, visible, err := ProjectPublicThreadEvent(threadID, thread, lifecycleEvent)
	if err != nil || !visible || projectedLifecycle == nil {
		t.Fatalf("ordinary lifecycle event was killed before its closed projection: projected=%#v visible=%t err=%v", projectedLifecycle, visible, err)
	}
	lifecycleBody, _ := json.Marshal(projectedLifecycle)
	for _, forbidden := range []string{
		`"envelope"`, `"registryHead"`, `"publicationIntent"`, `"storeDigest"`,
		"ORDINARY_EVENT_MAIL_SENTINEL", "ORDINARY_EVENT_REGISTRY_SENTINEL",
		"ORDINARY_EVENT_INTENT_SENTINEL", "ORDINARY_EVENT_STORE_SENTINEL",
	} {
		if strings.Contains(string(lifecycleBody), forbidden) {
			t.Fatalf("ordinary lifecycle extension crossed its closed projection via %q: %s", forbidden, lifecycleBody)
		}
	}

	event := map[string]any{
		"kind": "tool_call_started", "seq": float64(2), "timestamp": timestamp,
		"threadId": threadID, "turnId": turnID, "itemId": itemID,
		"item": map[string]any{
			"id": itemID, "threadId": threadID, "turnId": turnID,
			"createdAt": timestamp, "kind": "tool_call", "role": "tool", "status": "running",
			"toolName": "ordinary_generic_names", "callId": callID, "toolKind": "tool_call",
			"arguments": map[string]any{
				"envelope":          map[string]any{"subject": "ORDINARY_MAIL_ENVELOPE_SENTINEL"},
				"registryHead":      map[string]any{"branch": "ORDINARY_PACKAGE_INDEX_SENTINEL"},
				"publicationIntent": "ORDINARY_DOCUMENTATION_INTENT_SENTINEL",
				"storeDigest":       "ORDINARY_CACHE_KEY_SENTINEL",
			},
		},
	}
	if domainevent.ContainsPrivateAcceptedFinalAuthority(event) {
		t.Fatal("ordinary generic field names were classified as private accepted-final authority")
	}
	projected, visible, err := ProjectPublicThreadEvent(threadID, thread, event)
	if err != nil || !visible || projected == nil {
		t.Fatalf("ordinary tool event was killed before its closed projection: projected=%#v visible=%t err=%v", projected, visible, err)
	}
	projectedItem, _ := projected["item"].(map[string]any)
	if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(projectedItem["arguments"]); err != nil {
		t.Fatalf("ordinary tool arguments were not replaced by the closed metadata-only projection: %#v err=%v", projectedItem, err)
	}
	body, _ := json.Marshal(projected)
	for _, forbidden := range []string{
		`"envelope"`, `"registryHead"`, `"publicationIntent"`, `"storeDigest"`,
		"ORDINARY_MAIL_ENVELOPE_SENTINEL", "ORDINARY_PACKAGE_INDEX_SENTINEL",
		"ORDINARY_DOCUMENTATION_INTENT_SENTINEL", "ORDINARY_CACHE_KEY_SENTINEL",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("raw ordinary tool arguments crossed the closed projection via %q: %s", forbidden, body)
		}
	}

	for _, strongField := range []string{
		"acceptedFinal", "factFinalWitnessAdmission", "publicationSnapshotProof", "publicationSnapshotProofDigest",
	} {
		candidate := contracts.CloneMap(event)
		candidateItem := candidate["item"].(map[string]any)
		candidateItem["arguments"] = map[string]any{strongField: "DETACHED_PRIVATE_AUTHORITY_SENTINEL"}
		if leaked, allowed, projectionErr := ProjectPublicThreadEvent(threadID, thread, candidate); projectionErr != nil || allowed || leaked != nil {
			t.Fatalf("detached private authority %q did not fail closed at the ordinary projector: projected=%#v visible=%t err=%v", strongField, leaked, allowed, projectionErr)
		}
	}
}

func TestOrdinaryUsageEventUsesClosedReasoningEffort(t *testing.T) {
	telemetry := appusage.NewTerminalTelemetryV1(domainmodel.Usage{}, nil)
	base := map[string]any{
		"kind": "usage", "threadId": "ordinary", "turnId": "turn-ordinary",
		"usage": telemetry.PublicUsageMap(), "cacheDiagnostics": telemetry.PublicCacheDiagnosticsMap(),
	}
	for _, effort := range []string{"auto", "off", "low", "medium", "high", "max"} {
		event := contracts.CloneMap(base)
		event["effort"] = effort
		projected, visible := projectOrdinaryPublicThreadEvent(event)
		if !visible || contracts.StringField(projected, "effort") != effort {
			t.Fatalf("valid usage effort %q failed: projected=%#v visible=%t", effort, projected, visible)
		}
	}
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []any{" high ", "HIGH", sentinel, map[string]any{"reasoning": sentinel}} {
		event := contracts.CloneMap(base)
		event["effort"] = effort
		projected, visible := projectOrdinaryPublicThreadEvent(event)
		body, _ := json.Marshal(projected)
		if visible || projected != nil || strings.Contains(string(body), sentinel) {
			t.Fatalf("invalid usage effort entered public replay: effort=%#v projected=%s visible=%t", effort, body, visible)
		}
	}
}

func TestOrdinaryCheckpointEventsExposeMetadataOnly(t *testing.T) {
	const sentinel = "PRIVATE_CHECKPOINT_CONTENT_SENTINEL_6222020202020202020"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	events := []struct {
		kind       string
		payload    string
		event      map[string]any
		fieldCount int
	}{
		{
			kind: "checkpoint_captured", payload: "checkpoint", fieldCount: 10,
			event: map[string]any{
				"kind": "checkpoint_captured", "threadId": "ordinary", "turnId": "turn-ordinary", "seq": float64(7), "timestamp": "2026-07-14T00:00:00Z",
				"checkpoint": map[string]any{
					"schemaVersion": float64(1), "checkpointId": "axcp_cp-1", "sourceWorkspaceCheckpointId": sentinel,
					"threadId": "ordinary", "turnId": "turn-ordinary", "workspace": "/private/" + sentinel,
					"createdAt": "2026-07-14T00:00:00Z", "status": "captured", "snapshotStorage": "runtime_private_cas",
					"changedFiles": []any{map[string]any{"relativePath": sentinel, "changeKind": "modified", "beforeHash": sentinel, "afterHash": sentinel}},
				},
			},
		},
		{
			kind: "checkpoint_rewind_rescue_created", payload: "rescue", fieldCount: 9,
			event: map[string]any{
				"kind": "checkpoint_rewind_rescue_created", "threadId": "ordinary", "seq": float64(8), "timestamp": "2026-07-14T00:00:00Z",
				"rescue": map[string]any{
					"schemaVersion": float64(1), "rescueId": "axrr_axcp_cp-1", "planId": "axrp_axcp_cp-1", "checkpointId": "axcp_cp-1",
					"threadId": "ordinary", "workspace": "/private/" + sentinel, "createdAt": "2026-07-14T00:00:00Z",
					"files": []any{map[string]any{"relativePath": sentinel, "existed": true, "hash": sentinel, "content": sentinel, "encoding": "utf8"}},
				},
			},
		},
		{
			kind: "checkpoint_rewind_applied", payload: "apply", fieldCount: 13,
			event: map[string]any{
				"kind": "checkpoint_rewind_applied", "threadId": "ordinary", "seq": float64(9), "timestamp": "2026-07-14T00:00:00Z",
				"apply": map[string]any{
					"schemaVersion": float64(1), "applyId": "axra_axcp_cp-1", "planId": "axrp_axcp_cp-1", "checkpointId": "axcp_cp-1",
					"threadId": "ordinary", "workspace": "/private/" + sentinel, "createdAt": "2026-07-14T00:00:00Z",
					"scope": "combined", "status": "applied", "destructive": true,
					"rescue":       map[string]any{"rescueId": "axrr_cp-1", "content": sentinel},
					"files":        []any{map[string]any{"relativePath": sentinel, "status": "applied", "reason": sentinel, "beforeHash": sentinel}},
					"conversation": map[string]any{"status": "audit_recorded", "removedTurnIds": []any{sentinel}, "reason": sentinel},
					"summary": map[string]any{
						"fileAppliedCount": float64(1), "fileNoopCount": float64(0), "fileManualReviewCount": float64(0),
						"fileBlockedCount": float64(0), "fileFailedCount": float64(0),
					},
				},
			},
		},
	}
	for _, test := range events {
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, test.event)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || strings.Contains(string(body), sentinel) || strings.Contains(string(body), "workspace") ||
			strings.Contains(string(body), "relativePath") || strings.Contains(string(body), "checkpointId") || strings.Contains(string(body), "rescueId") {
			t.Fatalf("%s leaked private checkpoint payload: %s visible=%t err=%v", test.kind, body, visible, err)
		}
		metadata, _ := projected[test.payload].(map[string]any)
		if len(metadata) != test.fieldCount || metadata["projectionKind"] != "checkpoint_status" || metadata["disclosure"] != "metadata_only" ||
			metadata["privatePayloadWithheld"] != true || metadata["factAnswerAllowed"] != false || metadata["evidenceAuthority"] != false ||
			len(strings.TrimSpace(contracts.StringField(metadata, "checkpointRefDigest"))) != 64 {
			t.Fatalf("%s public metadata shape mismatch: %#v", test.kind, metadata)
		}
		outerFieldCount := 5
		if test.kind == "checkpoint_captured" {
			outerFieldCount = 6
		}
		if len(projected) != outerFieldCount {
			t.Fatalf("%s outer public event is not exact: %#v", test.kind, projected)
		}
	}
}

func TestOrdinaryCheckpointEventRejectsMismatchedNestedAuthority(t *testing.T) {
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	event := map[string]any{
		"kind": "checkpoint_captured", "threadId": "ordinary", "turnId": "turn-ordinary", "seq": float64(1), "timestamp": "2026-07-14T00:00:00Z",
		"checkpoint": map[string]any{
			"schemaVersion": float64(1), "checkpointId": "axcp_cp-1", "threadId": "other-thread", "turnId": "turn-ordinary",
			"status": "captured", "changedFiles": []any{},
		},
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || visible || projected != nil {
		t.Fatalf("mismatched checkpoint authority was published: %#v visible=%t err=%v", projected, visible, err)
	}
}

func TestOrdinaryCheckpointCaptureMarkerIsValidatedAndRemovedFromPublicProjection(t *testing.T) {
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	payload := map[string]any{
		"schemaVersion": float64(1), "checkpointId": domaincheckpointref.RuntimeID("marker-bound-checkpoint"),
		"threadId": "ordinary", "turnId": "turn-ordinary", "createdAt": "2026-07-14T00:00:00Z",
		"status": "captured", "changedFileCount": float64(1), "snapshotStorage": "runtime_private_cas",
		"captureEventId": domaincheckpointref.CaptureEventID(strings.Repeat("a", 64)),
	}
	payload["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(payload)
	event := map[string]any{
		"kind": "checkpoint_captured", "threadId": "ordinary", "turnId": "turn-ordinary",
		"seq": float64(1), "timestamp": "2026-07-14T00:00:01Z", "checkpoint": payload,
	}
	projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	if err != nil || !visible || projected == nil {
		t.Fatalf("valid marker-bound checkpoint event was rejected: %#v visible=%t err=%v", projected, visible, err)
	}
	publicPayload := projected["checkpoint"].(map[string]any)
	if _, present := publicPayload["captureEventId"]; present {
		t.Fatalf("ordinary public checkpoint event leaked capture id: %#v", publicPayload)
	}
	if _, present := publicPayload["capturePayloadDigest"]; present {
		t.Fatalf("ordinary public checkpoint event leaked capture payload digest: %#v", publicPayload)
	}
	tampered := contracts.CloneMap(event)
	tampered["checkpoint"].(map[string]any)["changedFileCount"] = float64(2)
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, tampered); err != nil || visible || projected != nil {
		t.Fatalf("checkpoint event with stale capture digest was published: %#v visible=%t err=%v", projected, visible, err)
	}
}

func TestOrdinaryCheckpointSummaryBuildersProjectMetadataOnly(t *testing.T) {
	const sentinel = "PRIVATE_RESCUE_BUILDER_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{}}
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("builder-1")
	rescue := checkpointapp.BuildRescueRecord(checkpointapp.RescueRecordInput{
		ThreadID: "ordinary", CheckpointID: checkpointID, PlanID: checkpointapp.PlanID(checkpointID),
		Workspace: "/private/" + sentinel, CreatedAt: "2026-07-14T00:00:00Z",
		Files: []checkpointapp.RescueFile{{RelativePath: sentinel, Existed: true, Hash: sentinel, Content: sentinel}},
	})
	rescueEvent := checkpointapp.BuildRescueCreatedEvent("ordinary", rescue)
	rescueEvent["seq"] = float64(1)
	rescueEvent["timestamp"] = "2026-07-14T00:00:01Z"
	apply := checkpointapp.BuildApplyResponse(checkpointapp.ApplyResponseInput{
		ThreadID: "ordinary", CheckpointID: checkpointID, Workspace: "/private/" + sentinel,
		PlanID: checkpointapp.PlanID(checkpointID), Scope: "code", CreatedAt: "2026-07-14T00:00:02Z", Status: "applied",
		Files: []map[string]any{{"relativePath": sentinel, "status": "applied", "reason": sentinel}}, Rescue: rescue,
	})
	applyEvent := checkpointapp.BuildRewindAppliedEvent("ordinary", apply)
	applyEvent["seq"] = float64(2)
	applyEvent["timestamp"] = "2026-07-14T00:00:03Z"
	for _, event := range []map[string]any{rescueEvent, applyEvent} {
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || projected == nil || strings.Contains(string(body), sentinel) || strings.Contains(string(body), "relativePath") || strings.Contains(string(body), "workspace") {
			t.Fatalf("builder summary event was not safely projected: %s visible=%t err=%v", body, visible, err)
		}
	}
}

func TestOrdinaryCheckpointV2EventsExposeMetadataOnly(t *testing.T) {
	const sentinel = "PRIVATE_CHECKPOINT_V2_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{}}
	checkpointID := checkpointapp.RuntimeCheckpointIDFromWorkspaceCheckpointID("workspace/checkpoint:v2")
	rescue := checkpointapp.BuildRescueRecord(checkpointapp.RescueRecordInput{
		ThreadID: "ordinary", CheckpointID: checkpointID, PlanID: checkpointapp.PlanID(checkpointID),
		Workspace: "/private/" + sentinel, CreatedAt: "2026-07-14T00:00:00Z",
		Files: []checkpointapp.RescueFile{{RelativePath: sentinel, Existed: true, Hash: sentinel, Content: sentinel}},
	})
	apply := checkpointapp.BuildApplyResponse(checkpointapp.ApplyResponseInput{
		ThreadID: "ordinary", CheckpointID: checkpointID, Workspace: "/private/" + sentinel,
		PlanID: checkpointapp.PlanID(checkpointID), Scope: "code", CreatedAt: "2026-07-14T00:00:01Z", Status: "applied",
		Files: []map[string]any{{"relativePath": sentinel, "status": "applied", "reason": sentinel}}, Rescue: rescue,
	})
	events := []map[string]any{
		checkpointapp.BuildRescueCreatedEvent("ordinary", rescue),
		checkpointapp.BuildRewindAppliedEvent("ordinary", apply),
	}
	for index, event := range events {
		event["seq"] = float64(index + 1)
		event["timestamp"] = "2026-07-14T00:00:02Z"
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || projected == nil || strings.Contains(string(body), sentinel) ||
			strings.Contains(string(body), checkpointID) || strings.Contains(string(body), "workspace") || strings.Contains(string(body), "relativePath") {
			t.Fatalf("V2 checkpoint event was not safely projected: %s visible=%t err=%v", body, visible, err)
		}
	}
}

func TestOrdinaryCheckpointEventsRejectNonCanonicalFields(t *testing.T) {
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	base := map[string]any{
		"kind": "checkpoint_captured", "threadId": "ordinary", "turnId": "turn-ordinary", "seq": float64(1), "timestamp": "2026-07-14T00:00:00Z",
		"checkpoint": map[string]any{
			"schemaVersion": float64(1), "checkpointId": "axcp_strict_1", "threadId": "ordinary", "turnId": "turn-ordinary",
			"status": "captured", "changedFiles": []any{map[string]any{"relativePath": "a.txt", "changeKind": "modified"}},
		},
	}
	for name, mutate := range map[string]func(map[string]any){
		"empty id suffix": func(event map[string]any) { event["checkpoint"].(map[string]any)["checkpointId"] = "axcp_" },
		"whitespace id":   func(event map[string]any) { event["checkpoint"].(map[string]any)["checkpointId"] = " axcp_strict_1 " },
		"unicode id":      func(event map[string]any) { event["checkpoint"].(map[string]any)["checkpointId"] = "axcp_案件" },
		"nested thread":   func(event map[string]any) { event["checkpoint"].(map[string]any)["threadId"] = " ordinary " },
		"trimmed status":  func(event map[string]any) { event["checkpoint"].(map[string]any)["status"] = " captured " },
		"scalar file":     func(event map[string]any) { event["checkpoint"].(map[string]any)["changedFiles"] = []any{"a.txt"} },
		"timestamp type":  func(event map[string]any) { event["timestamp"] = map[string]any{"value": "2026-07-14T00:00:00Z"} },
	} {
		event := contracts.CloneMap(base)
		mutate(event)
		if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || visible || projected != nil {
			t.Fatalf("%s checkpoint event was published: %#v visible=%t err=%v", name, projected, visible, err)
		}
	}
}

func TestLegacyChildLifecycleReplayUsesClosedMetadataProjection(t *testing.T) {
	const sentinel = "PRIVATE_LEGACY_CHILD_REASONING_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	events := []map[string]any{
		{
			"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary", "stage": "background_job_completed",
			"label": sentinel, "message": sentinel,
			"child":   map[string]any{"childRunId": "job-1", "childStatus": "completed", "output": sentinel, "error": sentinel},
			"details": map[string]any{"jobId": "job-1", "status": "completed", "outputPreview": sentinel, "deliveryError": sentinel},
		},
		{
			"kind": "item_created", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "item-1",
			"item": map[string]any{
				"id": "item-1", "threadId": "ordinary", "turnId": "turn-ordinary", "kind": "tool_progress",
				"toolName": "background_auto_continue", "summary": sentinel, "message": sentinel,
				"arguments": map[string]any{
					"runtimeStatus": "tool_progress", "stage": "background_job_completed", "message": sentinel,
					"diagnostics": map[string]any{"childRunId": "job-1", "status": "completed", "error": sentinel},
				},
			},
		},
	}
	for _, event := range events {
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || projected == nil || strings.Contains(string(body), sentinel) {
			t.Fatalf("legacy child lifecycle event was not safely projected: %s visible=%t err=%v", body, visible, err)
		}
		if !strings.Contains(string(body), "untrusted_child_output") || !strings.Contains(string(body), "job-1") {
			t.Fatalf("legacy child lifecycle metadata was lost: %s", body)
		}
	}
}

func TestTamperedChildLifecycleReplayUsesClosedStatuses(t *testing.T) {
	const invented = "invented_private_status"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}

	progress := map[string]any{
		"kind": "tool_progress", "threadId": "ordinary", "turnId": "turn-ordinary",
		"toolName": "read", "status": invented, "message": "ordinary progress",
	}
	projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, progress)
	body, _ := json.Marshal(projected)
	if err != nil || !visible || contracts.StringField(projected, "status") != "unknown" || strings.Contains(string(body), invented) {
		t.Fatalf("tampered tool progress status was reflected: %s visible=%t err=%v", body, visible, err)
	}

	childStage := map[string]any{
		"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary", "stage": "background_job_" + invented, "status": invented,
		"child":   map[string]any{"childRunId": "job-1", "childStatus": invented, "terminal": true},
		"details": map[string]any{"jobId": "job-1", "status": invented, "notificationKind": invented},
	}
	projected, visible, err = ProjectPublicThreadEvent("ordinary", thread, childStage)
	body, _ = json.Marshal(projected)
	child, _ := projected["child"].(map[string]any)
	terminal, _ := child["terminal"].(bool)
	if err != nil || !visible || contracts.StringField(projected, "stage") != "background_job_unknown" ||
		contracts.StringField(projected, "status") != "unknown" || contracts.StringField(child, "childStatus") != "unknown" ||
		terminal || strings.Contains(string(body), invented) {
		t.Fatalf("tampered child stage/status was reflected: %s visible=%t err=%v", body, visible, err)
	}

	malformedItem := map[string]any{
		"kind": "item_created", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "item-malformed",
		"item": map[string]any{
			"id": "item-malformed", "threadId": "ordinary", "turnId": "turn-ordinary", "kind": "tool_progress",
			"status": "completed", "toolName": "background_delivery",
			"arguments": map[string]any{
				"runtimeStatus": invented, "stage": "background_job_delivery_", "status": "retry",
				"diagnostics": map[string]any{"jobId": "job-1", "status": "completed"},
			},
		},
	}
	projected, visible, err = ProjectPublicThreadEvent("ordinary", thread, malformedItem)
	body, _ = json.Marshal(projected)
	item, _ := projected["item"].(map[string]any)
	arguments, _ := item["arguments"].(map[string]any)
	if err != nil || !visible || contracts.StringField(arguments, "runtimeStatus") != "unknown" ||
		contracts.StringField(arguments, "stage") != "background_job_delivery_unknown" ||
		contracts.StringField(arguments, "status") != "unknown" || strings.Contains(string(body), invented) {
		t.Fatalf("malformed child lifecycle item was not downgraded: %s visible=%t err=%v", body, visible, err)
	}

	for _, event := range []map[string]any{
		{"kind": "child_steer_queued", "threadId": "ordinary", "turnId": "turn-ordinary", "status": invented, "jobId": "job-1"},
		{"kind": "child_paused", "threadId": "ordinary", "turnId": "turn-ordinary", "status": invented, "jobId": "job-1"},
	} {
		if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || visible || projected != nil {
			t.Fatalf("mismatched lifecycle event kind/status was published: %#v visible=%t err=%v", projected, visible, err)
		}
	}

	expired := map[string]any{
		"kind": "child_pause_rejected", "threadId": "ordinary", "turnId": "turn-ordinary",
		"status": "expired", "jobId": "job-1", "pauseRequestId": "pause-1",
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, expired); err != nil || !visible || contracts.StringField(projected, "status") != "expired" {
		t.Fatalf("valid expired pause event was lost: %#v visible=%t err=%v", projected, visible, err)
	}
}

func TestGoalChildLineagePreservesClosedBasePipelineStage(t *testing.T) {
	const sentinel = "PRIVATE_GOAL_CHILD_LINEAGE_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	event := map[string]any{
		"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary",
		"stage": "response_received", "label": "internal goal child-run lineage",
		"child": map[string]any{
			"childRunId": "job-1", "childThreadId": "child-1", "childStatus": "completed",
			"parentGoalObjective": sentinel, "output": sentinel, "error": sentinel,
		},
	}
	projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	body, _ := json.Marshal(projected)
	if err != nil || !visible || contracts.StringField(projected, "stage") != "response_received" || strings.Contains(string(body), sentinel) {
		t.Fatalf("goal child lineage base stage was lost or leaked private data: %s visible=%t err=%v", body, visible, err)
	}
}

func TestOrdinaryToolResultRootAndOutputAreClosedAcrossSnapshotAndEvent(t *testing.T) {
	const sentinel = "PRIVATE_ORDINARY_TOOL_RESULT_SENTINEL"
	callID := threadTestHostToolCallID("ordinary-hostile-query")
	itemID := domaintoolresult.ToolResultItemIDV1("turn-ordinary", callID)
	item := map[string]any{
		"id": itemID, "threadId": "ordinary", "turnId": "turn-ordinary", "role": "tool",
		"status": "completed", "createdAt": "2026-07-14T00:00:00Z", "finishedAt": "2026-07-14T00:00:01Z",
		"kind": "tool_result", "toolName": "mcp__hostile__query", "callId": callID, "toolKind": "tool_call", "isError": false,
		"summary": sentinel, "details": map[string]any{"account": sentinel}, "dataUrl": "data:image/png;base64," + sentinel,
		"output": map[string]any{"code": "6222020202020202020", "content": sentinel, "previewUrl": "http://127.0.0.1:4173/" + sentinel},
	}
	thread := map[string]any{
		"id": "ordinary", "turns": []any{map[string]any{
			"id": "turn-ordinary", "threadId": "ordinary", "items": []any{item},
		}},
	}
	projectedThread, err := ProjectPublicThread(thread)
	if err == nil || projectedThread != nil {
		t.Fatalf("unverifiable tool-result lifecycle entered ordinary snapshot: %#v err=%v", projectedThread, err)
	}
	threadBody, _ := json.Marshal(projectedThread)
	if strings.Contains(string(threadBody), sentinel) || strings.Contains(string(threadBody), "6222020202020202020") {
		t.Fatalf("ordinary snapshot error reflected raw tool result: %s", threadBody)
	}

	event := map[string]any{
		"kind": "tool_call_finished", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": itemID, "item": item,
	}
	projectedEvent, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	eventBody, _ := json.Marshal(projectedEvent)
	if err != nil || visible || projectedEvent != nil || strings.Contains(string(eventBody), sentinel) || strings.Contains(string(eventBody), "6222020202020202020") {
		t.Fatalf("unverifiable tool-result completion event was not dropped: %s visible=%t err=%v", eventBody, visible, err)
	}
}

func TestCaseSourceStatusRemainsExactAcrossSnapshotAndPublicEvent(t *testing.T) {
	callID := threadTestHostToolCallID("case-source-closed-public-chain")
	contextDigest := domainsecurity.SHA256Hex([]byte("case-source-private-context"))
	grantID := domainsecurity.SHA256Hex([]byte("case-source-private-grant"))
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionCaseSourceStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "case_source_private", Status: "completed",
		Code: "case_source_result_private", PrivatePayloadWithheld: true,
	}
	item := map[string]any{
		"id": domaintoolresult.ToolResultItemIDV1("turn-case-source", callID), "threadId": "ordinary-case-source",
		"turnId": "turn-case-source", "role": "tool", "status": "completed", "createdAt": "2026-08-23T00:00:00Z",
		"finishedAt": "2026-08-23T00:00:01Z", "kind": "tool_result",
		"toolName": "mcp__analytix-fund-analysis__query_transactions", "callId": callID, "toolKind": "tool_call", "isError": false,
		"contextDigest": contextDigest, "contextEpoch": uint64(4), "executionGrantId": grantID,
		"privateProtocolObserved": true,
		"output":                  domaintoolresult.PublicToolResultProjectionRecordV1(projection),
	}
	proof, err := domaintoolresult.NewCaseSourceBindingProofV1(domaintoolresult.CaseSourceBindingInputV1{
		ToolName: item["toolName"].(string), ToolCallID: callID, ContextDigest: contextDigest,
		ContextEpoch: 4, ExecutionGrantID: grantID, Projection: projection, IsError: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	item[domaintoolresult.CaseSourceBindingProofFieldV1] = domaintoolresult.CaseSourceBindingProofRecordV1(proof)
	direct := domaintoolresult.PublicToolResultItemRecordV1(item)
	directProjection, directErr := domaintoolresult.ParsePublicToolResultProjectionV1(direct["output"])
	if directErr != nil || directProjection.ProjectionKind != domaintoolresult.ProjectionCaseSourceStatus {
		t.Fatalf("canonical private fixture failed item-level admission: %#v err=%v", direct, directErr)
	}
	thread := map[string]any{
		"id": "ordinary-case-source", "turns": []any{map[string]any{
			"id": "turn-case-source", "threadId": "ordinary-case-source", "status": "completed", "items": []any{item},
		}},
	}
	projectedThread, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	projectedEvent, visible, err := ProjectPublicThreadEvent("ordinary-case-source", thread, map[string]any{
		"kind": "tool_call_finished", "threadId": "ordinary-case-source", "turnId": "turn-case-source",
		"itemId": item["id"], "item": item,
	})
	if err != nil || !visible {
		t.Fatalf("closed case-source event was not public: event=%#v visible=%t err=%v", projectedEvent, visible, err)
	}
	for name, value := range map[string]any{"snapshot": projectedThread, "event": projectedEvent} {
		body, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, forbidden := range []string{
			contextDigest, grantID, proof.Digest, domaintoolresult.CaseSourceBindingProofFieldV1,
			"contextEpoch", "executionGrantId", "caseOutcome", "privateProtocolObserved",
		} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s retained private case settlement bytes %q: %s", name, forbidden, body)
			}
		}
		if !strings.Contains(string(body), `"projectionKind":"case_source_status"`) ||
			!strings.Contains(string(body), `"messageKey":"case_source_private"`) ||
			strings.Contains(string(body), "legacy_output_withheld") {
			t.Fatalf("%s did not preserve exact closed case-source status: %s", name, body)
		}
	}

	mutations := map[string]func(map[string]any){
		"missing proof": func(value map[string]any) { delete(value, domaintoolresult.CaseSourceBindingProofFieldV1) },
		"tool rebound":  func(value map[string]any) { value["toolName"] = "mcp__analytix-fund-analysis__other" },
		"call rebound":  func(value map[string]any) { value["callId"] = threadTestHostToolCallID("case-source-rebound") },
		"digest rebound": func(value map[string]any) {
			value["contextDigest"] = domainsecurity.SHA256Hex([]byte("rebound-context"))
		},
		"epoch rebound": func(value map[string]any) { value["contextEpoch"] = uint64(5) },
		"grant rebound": func(value map[string]any) {
			value["executionGrantId"] = domainsecurity.SHA256Hex([]byte("rebound-grant"))
		},
		"status rebound": func(value map[string]any) { value["status"] = "failed" },
		"error rebound":  func(value map[string]any) { value["isError"] = true },
		"proof rebound": func(value map[string]any) {
			value[domaintoolresult.CaseSourceBindingProofFieldV1].(map[string]any)["digest"] = domainsecurity.SHA256Hex([]byte("rebound-proof"))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			reboundItem := contracts.CloneMap(item)
			mutate(reboundItem)
			directRebound := projectOrdinaryPublicToolResultItem(reboundItem)
			if !ordinaryToolResultLifecycleConsistentV1(directRebound) || directRebound["lifecycleStatusWithheld"] != nil {
				t.Fatalf("item-level rebound did not form a closed legacy lifecycle: %#v", directRebound)
			}
			reboundThread := map[string]any{
				"id": "ordinary-case-source", "turns": []any{map[string]any{
					"id": "turn-case-source", "threadId": "ordinary-case-source", "status": "completed", "items": []any{reboundItem},
				}},
			}
			snapshot, snapshotErr := ProjectPublicThread(reboundThread)
			snapshotBody, _ := json.Marshal(snapshot)
			if snapshotErr != nil || !strings.Contains(string(snapshotBody), "legacy_output_withheld") ||
				strings.Contains(string(snapshotBody), "case_source_status") || strings.Contains(string(snapshotBody), proof.Digest) ||
				strings.Contains(string(snapshotBody), domaintoolresult.CaseSourceBindingProofFieldV1) {
				t.Fatalf("rebound snapshot was not withheld: %s err=%v", snapshotBody, snapshotErr)
			}
			event, visible, eventErr := ProjectPublicThreadEvent("ordinary-case-source", reboundThread, map[string]any{
				"kind": "tool_call_finished", "threadId": "ordinary-case-source", "turnId": "turn-case-source",
				"itemId": item["id"], "item": reboundItem,
			})
			eventBody, _ := json.Marshal(event)
			if eventErr != nil || strings.Contains(string(eventBody), "case_source_status") ||
				strings.Contains(string(eventBody), proof.Digest) || strings.Contains(string(eventBody), domaintoolresult.CaseSourceBindingProofFieldV1) ||
				(visible && !strings.Contains(string(eventBody), "legacy_output_withheld")) {
				t.Fatalf("rebound event was not withheld or dropped: %s visible=%t err=%v", eventBody, visible, eventErr)
			}
		})
	}
}

func TestOrdinaryToolResultAllowsOnlyCanonicalRestartOutcomeUnknownMismatch(t *testing.T) {
	callID := threadTestHostToolCallID("restart-outcome-unknown")
	itemID := domaintoolresult.ToolResultItemIDV1("turn-ordinary", callID)
	item := map[string]any{
		"id": itemID, "threadId": "ordinary", "turnId": "turn-ordinary", "role": "tool",
		"status": "failed", "createdAt": "2026-07-16T00:00:00Z", "finishedAt": "2026-07-16T00:00:01Z",
		"kind": "tool_result", "toolName": "write_file", "callId": callID, "toolKind": "file_change", "isError": true,
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.OutcomeUnknownAfterRestartProjectionV1()),
	}
	thread := map[string]any{
		"id": "ordinary", "turns": []any{map[string]any{
			"id": "turn-ordinary", "threadId": "ordinary", "items": []any{item},
		}},
	}
	projectedThread, err := ProjectPublicThread(thread)
	if err != nil || projectedThread == nil {
		t.Fatalf("canonical restart outcome-unknown result was rejected: %#v err=%v", projectedThread, err)
	}
	event := map[string]any{
		"kind": "tool_call_finished", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": itemID, "item": item,
	}
	projectedEvent, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	if err != nil || !visible || projectedEvent == nil {
		t.Fatalf("canonical restart outcome-unknown event was rejected: %#v visible=%t err=%v", projectedEvent, visible, err)
	}

	forged := contracts.CloneMap(thread)
	forgedOutput := forged["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["output"].(map[string]any)
	forgedOutput["messageKey"] = "tool_output_withheld"
	if projected, projectionErr := ProjectPublicThread(forged); projectionErr == nil || projected != nil {
		t.Fatalf("forged unknown-status projection entered ordinary history: %#v err=%v", projected, projectionErr)
	}
}

func TestOrdinaryToolResultSeparatesRootLifecycleFromClosedBlockedAndCancelledStatus(t *testing.T) {
	tests := []struct {
		name       string
		messageKey string
		status     string
		code       string
	}{
		{name: "blocked", messageKey: "tool_blocked", status: "blocked", code: "tool_not_advertised"},
		{name: "cancelled", messageKey: "tool_cancelled", status: "cancelled", code: "tool_cancelled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			callID := threadTestHostToolCallID("closed-" + test.name)
			itemID := domaintoolresult.ToolResultItemIDV1("turn-ordinary", callID)
			projection := domaintoolresult.PublicToolResultProjectionV1{
				SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionHostStatus,
				Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: test.messageKey, Status: test.status, Code: test.code,
				PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
			}
			item := map[string]any{
				"id": itemID, "threadId": "ordinary", "turnId": "turn-ordinary", "role": "tool",
				"status": "failed", "kind": "tool_result", "toolName": "read", "callId": callID, "toolKind": "tool_call", "isError": true,
				"output": domaintoolresult.PublicToolResultProjectionRecordV1(projection),
			}
			thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{
				"id": "turn-ordinary", "threadId": "ordinary", "items": []any{item},
			}}}
			projected, err := ProjectPublicThread(thread)
			if err != nil || projected == nil {
				t.Fatalf("closed %s result was rejected: %#v err=%v", test.status, projected, err)
			}
		})
	}
}

func TestOrdinaryLifecycleProjectionEnforcesKindStatusPairs(t *testing.T) {
	const hostile = "completed_6222020202020202020_PRIVATE_REASONING_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	tests := []struct {
		name  string
		event map[string]any
	}{
		{"thread created", map[string]any{"kind": "thread_created", "threadId": "ordinary", "status": hostile}},
		{"thread updated", map[string]any{"kind": "thread_updated", "threadId": "ordinary", "status": hostile}},
		{"turn started", map[string]any{"kind": "turn_started", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"turn completed", map[string]any{"kind": "turn_completed", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"turn steered with status", map[string]any{"kind": "turn_steered", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"approval requested", map[string]any{"kind": "approval_requested", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"approval resolved", map[string]any{"kind": "approval_resolved", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"input requested", map[string]any{"kind": "user_input_requested", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"input resolved", map[string]any{"kind": "user_input_resolved", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"upload wait", map[string]any{"kind": "tool_result_upload_wait", "threadId": "ordinary", "turnId": "turn-ordinary", "status": hostile}},
		{"item status", map[string]any{
			"kind": "item_created", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "item-user",
			"item": map[string]any{"id": "item-user", "threadId": "ordinary", "turnId": "turn-ordinary", "kind": "user_message", "status": hostile},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, test.event)
			body, _ := json.Marshal(projected)
			if err != nil || visible || projected != nil || strings.Contains(string(body), hostile) {
				t.Fatalf("mismatched lifecycle was published: %s visible=%t err=%v", body, visible, err)
			}
		})
	}

	valid := []map[string]any{
		{"kind": "thread_created", "threadId": "ordinary", "status": "idle", "title": hostile},
		{"kind": "thread_updated", "threadId": "ordinary", "status": "running", "title": hostile},
		{"kind": "turn_started", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "running"},
		{"kind": "turn_steered", "threadId": "ordinary", "turnId": "turn-ordinary", "text": "continue"},
		{"kind": "approval_requested", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "pending"},
		{"kind": "approval_resolved", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "allowed"},
		{"kind": "user_input_requested", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "pending"},
		{"kind": "user_input_resolved", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "submitted"},
		{"kind": "tool_result_upload_wait", "threadId": "ordinary", "turnId": "turn-ordinary", "status": "waiting"},
	}
	for _, event := range valid {
		if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || !visible || projected == nil {
			t.Fatalf("valid lifecycle was lost: event=%#v projected=%#v visible=%t err=%v", event, projected, visible, err)
		} else if projected["title"] != nil {
			t.Fatalf("thread lifecycle projection exposed title: %#v", projected)
		}
	}
}

func TestAutoResearchAuditProjectionIsClosedAndCountConsistent(t *testing.T) {
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	event := map[string]any{
		"kind": "autoresearch_state_audit", "threadId": "ordinary", "turnId": "turn-ordinary",
		"seq": float64(1), "timestamp": "2026-07-20T00:00:00Z", "schemaVersion": float64(1),
		"changeId": "autoresearch-state-audit", "runtimeContract": "analytix-go-runtime",
		"upstreamSource": "reasonix-absorbed", "goalMode": "research", "fileCount": float64(5),
		"requirementCount": float64(2), "completedRequirementCount": float64(1),
		"staleRequirementCount": float64(1), "staleDirectionCount": float64(1),
		"complete": false, "pivotRequired": true, "result": "pivot_required",
		"unknownRequirementAccepted": false, "findingsWrittenForUnknownRequirement": false,
		"writesReasonixFile": false, "writesAgentsFile": false, "stablePrefixContainsState": false,
		"toolSchemaContainsState": false, "topLevelAutoResearchRouteExposed": false,
		"usesReasonixPublicProtocol": false, "usesReasonixConfigRoot": false,
		"changesRendererContract": false, "changesProductIdentity": false,
	}
	projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	if err != nil || !visible || projected == nil {
		t.Fatalf("valid closed audit was rejected: projected=%#v visible=%t err=%v", projected, visible, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"operational path": func(candidate map[string]any) { candidate["progressPath"] = ".analytix/private" },
		"count mismatch":   func(candidate map[string]any) { candidate["staleRequirementCount"] = float64(0) },
		"result mismatch":  func(candidate map[string]any) { candidate["complete"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := contracts.CloneMap(event)
			mutate(candidate)
			projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, candidate)
			if err != nil || visible || projected != nil {
				t.Fatalf("invalid audit escaped: projected=%#v visible=%t err=%v", projected, visible, err)
			}
		})
	}
}

func TestOrdinaryPipelineStageProjectionUsesClosedStageV1(t *testing.T) {
	const hostile = "response_received_6222020202020202020_PRIVATE_REASONING_SENTINEL"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	for _, stage := range []string{
		"setup", "pre_start", "post_start", "input_received", "input_cached", "input_routed", "input_compressed",
		"input_remembered", "pre_send", "post_send", "response_received", "provider_retrying", "step_limit_finalizing",
		"provider_admission_rejected", "provider_error", "empty_final_recovered", "loop_guard", "subagent_running",
		"background_job_completed", "background_job_delivery_retry", "background_job_auto_continue_started",
	} {
		event := map[string]any{"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary", "stage": stage, "label": hostile}
		if stage == "provider_error" {
			event["details"] = map[string]any{"reasonCode": domainfailure.CodeProviderError}
		}
		if stage == "provider_admission_rejected" {
			event["details"] = map[string]any{
				"reasonCode": "context_window_hard_limit", "projectedRequestTokens": float64(900),
				"hardThresholdTokens": float64(850), "providerAttemptCount": float64(0),
			}
		}
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || contracts.StringField(projected, "stage") != stage || strings.Contains(string(body), hostile) {
			t.Fatalf("closed pipeline stage projection mismatch: stage=%s body=%s visible=%t err=%v", stage, body, visible, err)
		}
	}
	for _, stage := range []string{hostile, "case_fund_final_recovered"} {
		unknown := map[string]any{"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary", "stage": stage, "label": hostile}
		projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, unknown)
		body, _ := json.Marshal(projected)
		if err != nil || !visible || contracts.StringField(projected, "stage") != "unknown" ||
			contracts.StringField(projected, "label") != "Pipeline stage unavailable" || strings.Contains(string(body), hostile) || strings.Contains(string(body), "case_fund_final_recovered") {
			t.Fatalf("unknown pipeline stage was reflected: %s visible=%t err=%v", body, visible, err)
		}
	}
	invalidAdmission := map[string]any{
		"kind": "pipeline_stage", "threadId": "ordinary", "turnId": "turn-ordinary",
		"stage": "provider_admission_rejected", "label": "Provider admission rejected",
		"details": map[string]any{
			"reasonCode": "context_window_hard_limit", "projectedRequestTokens": float64(900),
			"hardThresholdTokens": float64(850), "providerAttemptCount": float64(0),
			"rawRequest": hostile,
		},
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, invalidAdmission); err != nil || visible || projected != nil {
		t.Fatalf("open context admission diagnostic survived public projection: projected=%#v visible=%t err=%v", projected, visible, err)
	}
}

func TestOrdinaryHistoryRejectsUnknownLifecycleControl(t *testing.T) {
	const hostile = "completed_6222020202020202020_PRIVATE_REASONING_SENTINEL"
	base := map[string]any{
		"id": "ordinary-history", "status": "idle",
		"turns": []any{map[string]any{
			"id": "turn-history", "threadId": "ordinary-history", "status": "running",
			"items": []any{map[string]any{
				"id": "item-user", "threadId": "ordinary-history", "turnId": "turn-history",
				"kind": "user_message", "role": "user", "status": "completed", "text": "hello",
			}},
		}},
	}
	tests := map[string]func(map[string]any){
		"thread status": func(thread map[string]any) { thread["status"] = hostile },
		"turn status": func(thread map[string]any) {
			thread["turns"].([]any)[0].(map[string]any)["status"] = hostile
		},
		"item status": func(thread map[string]any) {
			thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["status"] = hostile
		},
		"item kind": func(thread map[string]any) {
			thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["kind"] = hostile
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			thread := contracts.CloneMap(base)
			mutate(thread)
			projected, err := ProjectPublicThread(thread)
			body, _ := json.Marshal(projected)
			if err == nil || projected != nil || strings.Contains(string(body), hostile) {
				t.Fatalf("unknown snapshot lifecycle did not fail closed: %s err=%v", body, err)
			}
		})
	}

	roleTamper := contracts.CloneMap(base)
	roleTamper["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["role"] = hostile
	projected, err := ProjectPublicThread(roleTamper)
	body, _ := json.Marshal(projected)
	if err != nil || strings.Contains(string(body), hostile) || !strings.Contains(string(body), `"role":"user"`) {
		t.Fatalf("ordinary item role was not host-projected: %s err=%v", body, err)
	}
}

func TestOrdinaryHistoryWithholdsPrivateExecutionGrantTransition(t *testing.T) {
	const sentinel = "PRIVATE_APPROVAL_GRANT_SENTINEL"
	thread := map[string]any{
		"id": "ordinary-grant-transition", "status": "running",
		"turns": []any{map[string]any{
			"id": "turn-grant-transition", "threadId": "ordinary-grant-transition", "status": "running",
			"items": []any{
				map[string]any{
					"id": "item-user", "threadId": "ordinary-grant-transition", "turnId": "turn-grant-transition",
					"kind": "user_message", "role": "user", "status": "completed", "text": "continue",
				},
				map[string]any{
					"id": "item-grant-transition", "threadId": "ordinary-grant-transition", "turnId": "turn-grant-transition",
					"kind": "execution_grant_transition", "role": "tool", "status": "completed",
					"executionGrantId": sentinel, "approvalTransition": map[string]any{"private": sentinel},
				},
			},
		}},
	}
	projected, err := ProjectPublicThread(thread)
	body, _ := json.Marshal(projected)
	if err != nil || projected == nil || strings.Contains(string(body), sentinel) ||
		strings.Contains(string(body), "execution_grant_transition") {
		t.Fatalf("private grant transition entered ordinary public history: %s err=%v", body, err)
	}
	turn := projected["turns"].([]any)[0].(map[string]any)
	items := turn["items"].([]any)
	if len(items) != 1 || contracts.StringField(items[0].(map[string]any), "kind") != "user_message" {
		t.Fatalf("grant transition filtering changed public user history: %#v", items)
	}

	unknown := contracts.CloneMap(thread)
	unknownItems := unknown["turns"].([]any)[0].(map[string]any)["items"].([]any)
	unknownItems[1].(map[string]any)["kind"] = "execution_grant_transition_v2_unknown"
	if value, projectionErr := ProjectPublicThread(unknown); projectionErr == nil || value != nil {
		t.Fatalf("unknown transition lifecycle did not fail closed: %#v err=%v", value, projectionErr)
	}
}

func TestOrdinaryHistoryCanonicalizesLegacyTerminalStatuses(t *testing.T) {
	for _, status := range []string{"interrupted", "killed", "canceled", "cancelled", "timeout"} {
		t.Run(status, func(t *testing.T) {
			thread := map[string]any{
				"id": "ordinary-legacy-terminal", "status": "idle",
				"turns": []any{map[string]any{
					"id": "turn-legacy-terminal", "threadId": "ordinary-legacy-terminal", "status": status,
					"items": []any{map[string]any{
						"id": "item-user", "threadId": "ordinary-legacy-terminal", "turnId": "turn-legacy-terminal",
						"kind": "user_message", "role": "user", "status": "completed", "text": "continue",
					}},
				}},
			}
			projected, err := ProjectPublicThread(thread)
			if err != nil || projected == nil {
				t.Fatalf("closed legacy terminal status was rejected: %#v err=%v", projected, err)
			}
			turn := projected["turns"].([]any)[0].(map[string]any)
			if contracts.StringField(turn, "status") != "aborted" {
				t.Fatalf("legacy terminal status was not mapped to public aborted: %#v", turn)
			}
		})
	}
}

func TestOrdinaryInterruptedHistoryOmitsUntrustedProviderToolIdentity(t *testing.T) {
	thread := map[string]any{
		"id": "ordinary-interrupted-tool", "status": "idle",
		"turns": []any{map[string]any{
			"id": "turn-interrupted-tool", "threadId": "ordinary-interrupted-tool", "status": "interrupted",
			"items": []any{
				map[string]any{
					"id": "item-user", "threadId": "ordinary-interrupted-tool", "turnId": "turn-interrupted-tool",
					"kind": "user_message", "role": "user", "status": "completed", "text": "continue",
				},
				map[string]any{
					"id": "item-provider-call", "threadId": "ordinary-interrupted-tool", "turnId": "turn-interrupted-tool",
					"kind": "tool_call", "role": "assistant", "status": "running", "toolName": "read_file",
					"callId": "provider_call_untrusted", "arguments": map[string]any{"path": "private.txt"},
				},
			},
		}},
	}
	projected, err := ProjectPublicThread(thread)
	body, _ := json.Marshal(projected)
	if err != nil || projected == nil || strings.Contains(string(body), "provider_call_untrusted") ||
		strings.Contains(string(body), "private.txt") {
		t.Fatalf("untrusted interrupted tool identity entered public history: %s err=%v", body, err)
	}
	turn := projected["turns"].([]any)[0].(map[string]any)
	items := turn["items"].([]any)
	if contracts.StringField(turn, "status") != "aborted" || len(items) != 1 ||
		contracts.StringField(items[0].(map[string]any), "kind") != "user_message" {
		t.Fatalf("interrupted history repair changed the closed public projection: %#v", turn)
	}
}

func TestRestrictedEvidenceIsWithheldFromOrdinarySnapshotAndSSE(t *testing.T) {
	restricted := map[string]any{
		"schemaVersion":  2,
		"purpose":        "analytix.dataset-snapshot-manifest/v2",
		"manifestDigest": strings.Repeat("a", 64),
	}
	thread := map[string]any{
		"id": "ordinary-restricted",
		"turns": []any{map[string]any{
			"id": "turn-restricted", "threadId": "ordinary-restricted",
			"items": []any{map[string]any{
				"id": "review-restricted", "kind": "review", "output": map[string]any{"neutral": restricted},
			}},
		}},
	}
	if projected, err := ProjectPublicThread(thread); err == nil || projected != nil {
		t.Fatalf("restricted evidence reached ordinary snapshot: projected=%#v err=%v", projected, err)
	}
	event := map[string]any{
		"kind": "review_completed", "threadId": "ordinary-restricted", "turnId": "turn-restricted",
		"review": map[string]any{"output": map[string]any{"neutral": restricted}},
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary-restricted", thread, event); err != nil || visible || projected != nil {
		t.Fatalf("restricted evidence reached ordinary SSE: projected=%#v visible=%t err=%v", projected, visible, err)
	}
}

func TestLegacyPIIToolCallIdentityNeverReplaysToSnapshotOrSSE(t *testing.T) {
	const account = "6222020202020202020"
	item := map[string]any{
		"id":       "item_result_turn-legacy_provider_call_" + account,
		"threadId": "ordinary-legacy", "turnId": "turn-legacy", "kind": "tool_result", "role": "tool",
		"status": "failed", "toolName": "read", "callId": "provider_call_" + account, "isError": true,
		"output": map[string]any{"account": account},
	}
	thread := map[string]any{"id": "ordinary-legacy", "turns": []any{map[string]any{
		"id": "turn-legacy", "threadId": "ordinary-legacy", "items": []any{item},
	}}}
	projected, err := ProjectPublicThread(thread)
	body, _ := json.Marshal(projected)
	if err == nil || projected != nil || strings.Contains(string(body), account) {
		t.Fatalf("legacy provider identity was not rejected from snapshot projection: %s err=%v", body, err)
	}
	event := map[string]any{
		"kind": "tool_call_finished", "threadId": "ordinary-legacy", "turnId": "turn-legacy",
		"itemId": item["id"], "item": item,
	}
	if publicEvent, visible, eventErr := ProjectPublicThreadEvent("ordinary-legacy", thread, event); eventErr != nil || visible || publicEvent != nil {
		t.Fatalf("legacy provider identity event was replayed: event=%#v visible=%t err=%v", publicEvent, visible, eventErr)
	}
	progress := map[string]any{
		"kind": "tool_progress", "threadId": "ordinary-legacy", "turnId": "turn-legacy",
		"itemId":   "item_tool_turn-legacy_provider_call_" + account,
		"toolName": "read", "callId": "provider_call_" + account, "status": "running", "message": "tool running",
	}
	publicProgress, visible, progressErr := ProjectPublicThreadEvent("ordinary-legacy", thread, progress)
	progressBody, _ := json.Marshal(publicProgress)
	if progressErr != nil || !visible || strings.Contains(string(progressBody), account) || publicProgress["callId"] != nil || publicProgress["itemId"] != nil {
		t.Fatalf("legacy provider identity crossed progress SSE: %s visible=%t err=%v", progressBody, visible, progressErr)
	}
	compaction := map[string]any{
		"kind": "compaction_completed", "threadId": "ordinary-legacy", "turnId": "turn-legacy",
		"sourceItemIds": []any{"item-safe", "item_result_turn-legacy_provider_call_" + account},
	}
	publicCompaction, visible, compactionErr := ProjectPublicThreadEvent("ordinary-legacy", thread, compaction)
	compactionBody, _ := json.Marshal(publicCompaction)
	if compactionErr != nil || visible || publicCompaction != nil || strings.Contains(string(compactionBody), account) {
		t.Fatalf("unproved legacy compaction crossed SSE: %s visible=%t err=%v", compactionBody, visible, compactionErr)
	}
}

func TestHostToolItemIdentityIsNotMisclassifiedAsPII(t *testing.T) {
	const callID = "call_host_4a10b59ad049d0cbf8fdf3abee1d7e048294b6a887ce37ae4e161262d146dcf7"
	itemID := domaintoolcall.ToolCallItemIDV1("turn_1", callID)
	if itemID != "item_tool_host_v1_caed5c53688fbd0862820e165a1601868a0ee61d130d2780dff4770390296333" {
		t.Fatalf("fixture no longer exercises the digit-run collision: %q", itemID)
	}
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn_1", "items": []any{}}}}
	event := map[string]any{
		"kind": "tool_progress", "threadId": "ordinary", "turnId": "turn_1", "itemId": itemID,
		"callId": callID, "toolName": "delegate_task", "status": "running",
		"child": map[string]any{"jobId": "job-1", "childRunId": "job-1", "childStatus": "running"},
	}
	projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	if err != nil || !visible || contracts.StringField(projected, "itemId") != itemID {
		t.Fatalf("host-issued tool item identity was not projected deterministically: %#v visible=%t err=%v", projected, visible, err)
	}
}

func TestOrdinaryToolProgressWithholdsUntrustedSummaryAndMessageBeforeSSE(t *testing.T) {
	const secret = "opaque-sse-secret-123"
	const account = "6222020202020202020"
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn_1", "items": []any{}}}}
	event := map[string]any{
		"kind": "tool_progress", "threadId": "ordinary", "turnId": "turn_1",
		"toolName": "read_file", "callId": "call-safe", "status": "running",
		"summary": "read " + account,
		"message": "Authorization: Bearer " + secret + " account number: " + account,
	}

	projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event)
	body, _ := json.Marshal(projected)
	if err != nil || !visible || projected == nil {
		t.Fatalf("safe projected progress was not visible: projected=%#v visible=%t err=%v", projected, visible, err)
	}
	if strings.Contains(string(body), secret) || strings.Contains(string(body), account) {
		t.Fatalf("credential or PII crossed SSE projection: %s", body)
	}
	if projected["summary"] != nil || projected["message"] != nil {
		t.Fatalf("ordinary progress projected untrusted display text: %s", body)
	}
}

func TestOrdinaryAssistantDraftDeltaIsAlwaysPrivate(t *testing.T) {
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	assistant := map[string]any{
		"id": "item-assistant", "threadId": "ordinary", "turnId": "turn-ordinary", "kind": "assistant_text",
		"role": "assistant", "status": "running", "createdAt": "2026-07-14T00:00:00Z", "text": "safe fragment",
	}
	valid := map[string]any{
		"kind": "assistant_text_delta", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "item-assistant", "item": assistant,
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, valid); err != nil || visible || projected != nil {
		t.Fatalf("assistant draft delta became public: %#v visible=%t err=%v", projected, visible, err)
	}

	for name, mutate := range map[string]func(map[string]any){
		"item id":        func(event map[string]any) { event["itemId"] = "other-item" },
		"thread":         func(event map[string]any) { event["item"].(map[string]any)["threadId"] = "other-thread" },
		"turn":           func(event map[string]any) { event["item"].(map[string]any)["turnId"] = "other-turn" },
		"lifecycle kind": func(event map[string]any) { event["item"].(map[string]any)["kind"] = "tool_call" },
	} {
		event := contracts.CloneMap(valid)
		mutate(event)
		if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || visible || projected != nil {
			t.Fatalf("%s mismatch was published: %#v visible=%t err=%v", name, projected, visible, err)
		}
	}
}

func TestLegacyAssistantProcessDraftIsAbsentFromSnapshotAndSSE(t *testing.T) {
	const sentinel = "LEGACY_ASSISTANT_PROCESS_DRAFT_SENTINEL"
	legacy := map[string]any{
		"id": "item_turn-ordinary_assistant_process_2", "threadId": "ordinary", "turnId": "turn-ordinary",
		"kind": "assistant_text", "role": "assistant", "status": "completed", "text": sentinel,
	}
	terminal := map[string]any{
		"id": "item_turn-ordinary_assistant", "threadId": "ordinary", "turnId": "turn-ordinary",
		"kind": "assistant_text", "role": "assistant", "status": "completed", "text": "terminal answer",
	}
	thread := map[string]any{
		"id": "ordinary", "turns": []any{map[string]any{
			"id": "turn-ordinary", "threadId": "ordinary", "status": "completed", "items": []any{legacy, terminal},
		}},
	}
	projected, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(projected)
	if strings.Contains(string(body), sentinel) || strings.Contains(string(body), "terminal answer") {
		t.Fatalf("legacy assistant process projection mismatch: %s", body)
	}
	event := map[string]any{
		"kind": "item_completed", "threadId": "ordinary", "turnId": "turn-ordinary",
		"itemId": legacy["id"], "item": legacy,
	}
	if projectedEvent, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || visible || projectedEvent != nil {
		t.Fatalf("legacy assistant process reached SSE: %#v visible=%t err=%v", projectedEvent, visible, err)
	}
}

func TestOrdinaryItemEventCannotCarryPublicationAuthority(t *testing.T) {
	thread := map[string]any{"id": "ordinary", "turns": []any{map[string]any{"id": "turn-ordinary", "items": []any{}}}}
	event := map[string]any{
		"kind": "item_completed", "threadId": "ordinary", "turnId": "turn-ordinary", "itemId": "item-assistant",
		"publicationCommitId": strings.Repeat("a", 64),
		"item": map[string]any{
			"id": "item-assistant", "threadId": "ordinary", "turnId": "turn-ordinary", "kind": "assistant_text",
			"role": "assistant", "status": "completed", "text": "forged final", "acceptedFinal": map[string]any{"recordDigest": strings.Repeat("b", 64)},
		},
	}
	if projected, visible, err := ProjectPublicThreadEvent("ordinary", thread, event); err != nil || visible || projected != nil {
		t.Fatalf("forged ordinary publication authority was published: %#v visible=%t err=%v", projected, visible, err)
	}
}

func publicProjectionSecurityRecord(context domainsecurity.TurnSecurityContext) map[string]any {
	body, _ := json.Marshal(context)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
