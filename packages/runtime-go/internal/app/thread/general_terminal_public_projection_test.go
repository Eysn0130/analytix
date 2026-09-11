package thread

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	appusage "analytix.local/runtime-go/internal/app/usage"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

var generalTerminalPublicMarkerFieldsV1 = []string{
	"generalTerminalCommitId", "generalTerminalEventId", "generalTerminalSlot",
	"generalTerminalPayloadDigest", "generalTerminalAuthorityKind", "generalTerminalAuthorityDigest",
}

func TestOrdinaryTerminalPublicProjectionRequiresCanonicalOutbox(t *testing.T) {
	for _, test := range []struct {
		name   string
		status string
		reason string
	}{
		{name: "completed", status: "completed", reason: "success"},
		{name: "failed", status: "failed", reason: "provider_failure"},
		{name: "aborted", status: "aborted", reason: "cancel"},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread, events, sentinel := canonicalGeneralTerminalPublicFixture(t, test.status, test.reason)
			projected, err := ProjectPublicThread(thread)
			body, _ := json.Marshal(projected)
			if err != nil || !strings.Contains(string(body), sentinel) || strings.Contains(string(body), "generalTerminal") {
				t.Fatalf("canonical terminal snapshot mismatch: body=%s err=%v", body, err)
			}
			for _, event := range events {
				publicEvent, visible, eventErr := ProjectPublicThreadEvent("thread-public-terminal", thread, event)
				eventBody, _ := json.Marshal(publicEvent)
				if eventErr != nil || !visible || publicEvent == nil || strings.Contains(string(eventBody), "generalTerminal") {
					t.Fatalf("canonical terminal event was not projected: event=%#v public=%s visible=%t err=%v", event, eventBody, visible, eventErr)
				}
			}

			unmarked := contracts.CloneMap(events[0])
			for _, field := range generalTerminalPublicMarkerFieldsV1 {
				delete(unmarked, field)
			}
			if publicEvent, visible, eventErr := ProjectPublicThreadEvent("thread-public-terminal", thread, unmarked); eventErr != nil || visible || publicEvent != nil {
				t.Fatalf("marker-stripped terminal item became public: public=%#v visible=%t err=%v", publicEvent, visible, eventErr)
			}

			tampered := contracts.CloneMap(events[0])
			item := tampered["item"].(map[string]any)
			if test.status == "completed" {
				item["text"] = sentinel + "_TAMPERED"
			} else {
				item["message"] = sentinel + "_TAMPERED"
			}
			if publicEvent, visible, eventErr := ProjectPublicThreadEvent("thread-public-terminal", thread, tampered); eventErr != nil || visible || publicEvent != nil {
				t.Fatalf("digest-mismatched terminal item became public: public=%#v visible=%t err=%v", publicEvent, visible, eventErr)
			}
		})
	}
}

func TestContextBoundLegacyTerminalIsBoundaryOnlyAndPartialAuthorityFailsClosed(t *testing.T) {
	thread, events, sentinel := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	legacy := contracts.CloneMap(thread)
	delete(legacy, appturn.GeneralTerminalPublicationArchiveFieldV1)
	turn := legacy["turns"].([]any)[0].(map[string]any)
	delete(turn, "generalTerminalPublication")
	delete(turn, "generalTerminalCASBinding")
	projected, err := ProjectPublicThread(legacy)
	body, _ := json.Marshal(projected)
	if err != nil || strings.Contains(string(body), sentinel) {
		t.Fatalf("authority-free context-bound terminal was not boundary-only: %s err=%v", body, err)
	}
	for _, event := range events {
		unmarked := contracts.CloneMap(event)
		for _, field := range generalTerminalPublicMarkerFieldsV1 {
			delete(unmarked, field)
		}
		if publicEvent, visible, eventErr := ProjectPublicThreadEvent("thread-public-terminal", legacy, unmarked); eventErr != nil || visible || publicEvent != nil {
			t.Fatalf("legacy terminal event became public: %#v visible=%t err=%v", publicEvent, visible, eventErr)
		}
	}

	partial := contracts.CloneMap(thread)
	partialTurn := partial["turns"].([]any)[0].(map[string]any)
	delete(partialTurn, "generalTerminalPublication")
	if projected, projectionErr := ProjectPublicThread(partial); projectionErr == nil || projected != nil {
		t.Fatalf("partial terminal authority did not fail closed: projected=%#v err=%v", projected, projectionErr)
	}

	corrupt := contracts.CloneMap(thread)
	corruptTurn := corrupt["turns"].([]any)[0].(map[string]any)
	corruptTurn["generalTerminalPublication"].(map[string]any)["terminalReason"] = "recovery"
	if projected, projectionErr := ProjectPublicThread(corrupt); projectionErr == nil || projected != nil {
		t.Fatalf("corrupt terminal authority did not fail closed: projected=%#v err=%v", projected, projectionErr)
	}
}

func TestCancelledGeneralTerminalProjectsCompleteAtomicInterruptMetadata(t *testing.T) {
	thread, rawEvents, _ := canonicalGeneralTerminalPublicFixture(t, "aborted", "cancel")
	projectedEvents := make([]map[string]any, 0, len(rawEvents))
	for _, raw := range rawEvents {
		projected, visible, err := ProjectPublicThreadEvent("thread-public-terminal", thread, raw)
		if err != nil || !visible || projected == nil {
			t.Fatalf("cancel event projection failed: projected=%#v visible=%t err=%v", projected, visible, err)
		}
		projectedEvents = append(projectedEvents, projected)
	}
	terminal := projectedEvents[len(projectedEvents)-1]
	if terminal["discard"] != true || terminal["cancelled"] != true || terminal["cancelledPendingGates"] != 2 {
		t.Fatalf("public cancel metadata is incomplete: %#v", terminal)
	}
	if _, present := terminal["error"]; present {
		t.Fatalf("durable-only error crossed the public cancel projection: %#v", terminal)
	}
	if _, err := domainevent.NewGeneralTerminalDeliveryBatchV1(rawEvents, projectedEvents); err != nil {
		t.Fatalf("canonical cancel terminal did not form one atomic public batch: raw=%#v projected=%#v err=%v", rawEvents, projectedEvents, err)
	}

	tampered := contracts.CloneMap(thread)
	turn := tampered["turns"].([]any)[0].(map[string]any)
	turn["cancelledPendingGates"] = 1
	if projected, visible, err := ProjectPublicThreadEvent(
		"thread-public-terminal", tampered, rawEvents[len(rawEvents)-1],
	); err == nil || visible || projected != nil {
		t.Fatalf("detached cancel metadata became public: projected=%#v visible=%t err=%v", projected, visible, err)
	}
}

func TestAllDispositionsUseExactRawPublicAndAtomicBatchProfiles(t *testing.T) {
	dispositions := domainterminal.AllDispositionsV1()
	if len(dispositions) != 18 {
		t.Fatalf("terminal disposition inventory drifted: %#v", dispositions)
	}
	for _, disposition := range dispositions {
		t.Run(disposition.Reason, func(t *testing.T) {
			thread, rawEvents, _ := canonicalGeneralTerminalPublicFixture(t, disposition.Status, disposition.Reason)
			if len(rawEvents) != 3 {
				t.Fatalf("production terminal did not use three slots: %#v", rawEvents)
			}
			projectedEvents := make([]map[string]any, 0, len(rawEvents))
			for _, raw := range rawEvents {
				projected, visible, err := ProjectPublicThreadEvent("thread-public-terminal", thread, raw)
				if err != nil || !visible || projected == nil {
					t.Fatalf("raw terminal event did not project: raw=%#v projected=%#v visible=%t err=%v", raw, projected, visible, err)
				}
				projectedEvents = append(projectedEvents, projected)
			}
			batch, err := domainevent.NewGeneralTerminalDeliveryBatchV1(rawEvents, projectedEvents)
			if err != nil || len(batch.Events) != 3 || batch.FactAnswerAllowed || batch.EvidenceAuthority || batch.CitationAuthority {
				t.Fatalf("terminal profile did not form one closed batch: batch=%#v err=%v", batch, err)
			}

			rawItem := rawEvents[0]["item"].(map[string]any)
			rawTerminal := rawEvents[2]
			publicTerminal := projectedEvents[2]
			if rawTerminal["itemId"] != rawEvents[0]["itemId"] || publicTerminal["itemId"] != projectedEvents[0]["itemId"] {
				t.Fatalf("terminal item identity is not bound: raw=%#v public=%#v", rawTerminal, publicTerminal)
			}
			if disposition.Status == "completed" {
				if rawItem["kind"] != "assistant_text" || rawItem["text"] != appturn.GeneralProviderFinalQuarantinedText {
					t.Fatalf("completed terminal did not use the fixed boundary item: %#v", rawItem)
				}
			} else {
				projection, ok := domainterminal.GeneralFailureProjectionV1(disposition.Reason)
				if !ok || rawItem["kind"] != "error" || rawItem["status"] != disposition.Status ||
					rawItem["code"] != projection.Code || rawItem["message"] != projection.Message || rawItem["severity"] != projection.Severity {
					t.Fatalf("failed terminal item is not the exact host projection: item=%#v projection=%#v", rawItem, projection)
				}
			}
			if disposition.CandidateAllowed {
				for _, key := range []string{"code", "message", "error", "severity"} {
					if _, present := rawTerminal[key]; present {
						t.Fatalf("candidate terminal carried failure field %q: %#v", key, rawTerminal)
					}
				}
			} else {
				projection, ok := domainterminal.GeneralFailureProjectionV1(disposition.Reason)
				if !ok || rawTerminal["code"] != projection.Code || rawTerminal["message"] != projection.Message ||
					rawTerminal["error"] != projection.Message || rawTerminal["severity"] != projection.Severity ||
					publicTerminal["code"] != projection.Code || publicTerminal["message"] != projection.Message ||
					publicTerminal["severity"] != projection.Severity {
					t.Fatalf("terminal lifecycle projection mismatch: raw=%#v public=%#v projection=%#v", rawTerminal, publicTerminal, projection)
				}
				if _, present := publicTerminal["error"]; present {
					t.Fatalf("durable-only error crossed public projection: %#v", publicTerminal)
				}
			}
			for _, record := range []map[string]any{rawTerminal, publicTerminal} {
				_, hasDiscard := record["discard"]
				_, hasCancelled := record["cancelled"]
				_, hasPending := record["cancelledPendingGates"]
				if disposition.Reason == "cancel" {
					if !hasDiscard || !hasCancelled || !hasPending {
						t.Fatalf("cancel terminal lost atomic interrupt metadata: %#v", record)
					}
				} else if hasDiscard || hasCancelled || hasPending {
					t.Fatalf("non-cancel terminal gained interrupt metadata: %#v", record)
				}
			}
		})
	}
}

func canonicalGeneralTerminalPublicFixture(
	t *testing.T,
	status string,
	reason string,
) (map[string]any, []map[string]any, string) {
	t.Helper()
	context, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-public-terminal", TurnID: "turn-public-terminal", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-15T00:00:00Z"
	sentinel := "CANONICAL_GENERAL_TERMINAL_" + strings.ToUpper(status)
	item := map[string]any{
		"id": "item-turn-public-terminal", "threadId": context.ThreadID, "turnId": context.TurnID,
		"status": status, "createdAt": committedAt, "finishedAt": committedAt,
	}
	if status == "completed" {
		sentinel = appturn.GeneralProviderFinalQuarantinedText
		item["kind"] = "assistant_text"
		item["role"] = "assistant"
		item["text"] = sentinel
	} else {
		projection, ok := domainterminal.GeneralFailureProjectionV1(reason)
		if !ok || projection.Status != status {
			t.Fatalf("terminal reason %q has no exact %q failure projection", reason, status)
		}
		sentinel = projection.Message
		item["kind"] = "error"
		item["role"] = "system"
		item["message"] = sentinel
		item["code"] = projection.Code
		item["severity"] = projection.Severity
	}
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(context, reason, status, sentinel)
	if err != nil {
		t.Fatal(err)
	}
	item["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	itemEvent := appturn.AssistantItemCompletedEvent(item)
	itemEvent["timestamp"] = committedAt
	telemetry := appusage.NewTerminalTelemetryV1(domainmodel.Usage{TotalTokens: 3}, nil)
	usageEvent := map[string]any{
		"kind": "usage", "threadId": context.ThreadID, "turnId": context.TurnID, "model": "gpt-5",
		"usage": telemetry.PublicUsageMap(), "cacheDiagnostics": telemetry.PublicCacheDiagnosticsMap(),
		"timestamp": committedAt, "usageFinalStatus": status,
	}
	terminalKind, ok := domainturnterminal.GeneralTerminalLifecycleEventKindV1(status)
	if !ok {
		t.Fatalf("unsupported terminal status %q", status)
	}
	terminalEvent := map[string]any{
		"kind": terminalKind, "threadId": context.ThreadID, "turnId": context.TurnID, "status": status,
		"itemId": item["id"], "timestamp": committedAt, "terminalReason": reason,
		"generalTerminalCASBindingDigest": binding.BindingDigest,
	}
	if projection, fixed := domainterminal.GeneralFailureProjectionV1(reason); fixed {
		terminalEvent["code"] = projection.Code
		terminalEvent["message"] = projection.Message
		terminalEvent["error"] = projection.Message
		terminalEvent["severity"] = projection.Severity
	}
	if reason == "cancel" {
		terminalEvent["discard"] = true
		terminalEvent["cancelled"] = true
		terminalEvent["cancelledPendingGates"] = 2
	}
	commit, err := appturn.BuildGeneralTerminalPublicationCommitForEventsV1(
		context, binding, itemEvent, usageEvent, terminalEvent, committedAt, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1(
		[]domainturnterminal.GeneralTerminalPublicationCommitV1{commit},
	)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := publicProjectionSecurityRecord(context)
	turn := map[string]any{
		"id": context.TurnID, "threadId": context.ThreadID, "status": status, "finishedAt": committedAt,
		"securityContext": contextRecord, "items": []any{item},
		"generalTerminalCASBinding":  domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
		"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	}
	if reason == "cancel" {
		turn["discard"] = true
		turn["cancelled"] = true
		turn["cancelledPendingGates"] = 2
	}
	thread := map[string]any{
		"id": context.ThreadID, "workspace": context.WorkspaceRealPath, "status": "idle",
		"securityState": contextRecord,
		appturn.GeneralTerminalPublicationArchiveFieldV1: domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
		"turns": []any{turn},
	}
	events, err := appturn.PrepareGeneralTerminalEventBundleV1(thread, context.TurnID, commit, 1)
	if err != nil {
		t.Fatal(err)
	}
	return thread, events, sentinel
}
