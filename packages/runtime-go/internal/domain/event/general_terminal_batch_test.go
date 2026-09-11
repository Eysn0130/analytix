package event

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

func TestGeneralTerminalDeliveryBatchV1BindsExactOrdinaryTransportWithoutFactAuthority(t *testing.T) {
	raw, projected := generalTerminalDeliveryBatchFixture(t, true)
	batch, err := NewGeneralTerminalDeliveryBatchV1(raw, projected)
	if err != nil {
		t.Fatal(err)
	}
	if batch.Kind != GeneralTerminalDeliveryBatchKind || batch.FirstSeq != 11 || batch.LastSeq != 13 || batch.Seq != 13 ||
		batch.GeneralTerminalCommitID != contracts.StringField(raw[0], "generalTerminalCommitId") ||
		batch.TransportAuthority != GeneralTerminalTransportAuthority || batch.EvidenceAuthority ||
		batch.CitationAuthority || batch.FactAnswerAllowed || len(batch.EventManifest) != 3 {
		t.Fatalf("unexpected general terminal delivery batch: %#v", batch)
	}
	value := GeneralTerminalDeliveryBatchV1Map(batch)
	parsed, err := ParseGeneralTerminalDeliveryBatchV1(value)
	if err != nil || !reflect.DeepEqual(GeneralTerminalDeliveryBatchV1Map(parsed), value) {
		t.Fatalf("canonical batch did not round trip: parsed=%#v err=%v", parsed, err)
	}
	if ContainsAcceptedFinalPublicationAuthority(value) {
		t.Fatalf("ordinary transport was promoted to accepted-final authority: %#v", value)
	}

	value["factAnswerAllowed"] = true
	if _, err := ParseGeneralTerminalDeliveryBatchV1(value); err == nil {
		t.Fatal("fact authorization tampering bypassed the ordinary transport boundary")
	}
}

func TestGeneralTerminalDeliveryBatchV1SupportsExactTwoSlotFailureTransport(t *testing.T) {
	raw, projected := generalTerminalDeliveryBatchFixture(t, false)
	batch, err := NewGeneralTerminalDeliveryBatchV1(raw, projected)
	if err != nil {
		t.Fatal(err)
	}
	if batch.FirstSeq != 11 || batch.LastSeq != 12 || len(batch.Events) != 2 ||
		batch.EventManifest[0].Slot != "usage" || batch.EventManifest[1].Slot != "terminal" {
		t.Fatalf("unexpected two-slot transport: %#v", batch)
	}
}

func TestGeneralTerminalDeliveryBatchV1AcceptsExactCompletedHostFailures(t *testing.T) {
	for _, reason := range []string{"source_unavailable", "recovery", "approval_denied", "input_cancelled"} {
		t.Run(reason, func(t *testing.T) {
			raw, projected := generalTerminalCompletedFailureFixture(t, reason)
			batch, err := NewGeneralTerminalDeliveryBatchV1(raw, projected)
			if err != nil {
				t.Fatal(err)
			}
			if batch.FactAnswerAllowed || batch.EvidenceAuthority || batch.CitationAuthority ||
				contracts.StringField(batch.Events[2], "terminalReason") != reason {
				t.Fatalf("completed host failure gained fact authority: %#v", batch)
			}
		})
	}
}

func TestGeneralTerminalDeliveryBatchV1RejectsReasonFailureProjectionMismatch(t *testing.T) {
	raw, projected := generalTerminalCompletedFailureFixture(t, "recovery")
	wrong, ok := domainterminal.GeneralFailureProjectionV1("approval_denied")
	if !ok {
		t.Fatal("approval denial projection is unavailable")
	}
	for _, terminal := range []map[string]any{raw[2], projected[2]} {
		terminal["code"] = wrong.Code
		terminal["message"] = wrong.Message
		terminal["severity"] = wrong.Severity
	}
	raw[2]["error"] = wrong.Message
	raw[2]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[2])
	if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
		t.Fatal("self-consistent failure projection for another terminal reason was accepted")
	}
}

func TestGeneralTerminalDeliveryBatchV1AcceptsExactAndLegacyToolFailureCodes(t *testing.T) {
	for _, code := range []string{"tool_invalid_arguments_storm", "tool_not_advertised", "turn_failed", "tool_failure_storm"} {
		t.Run(code, func(t *testing.T) {
			raw, projected := generalTerminalDeliveryBatchFixture(t, false)
			projection, ok := domainterminal.GeneralFailureProjectionForCodeV1("tool_failure", code)
			if !ok {
				t.Fatalf("tool failure projection unavailable for %q", code)
			}
			for _, terminal := range []map[string]any{raw[1], projected[1]} {
				terminal["terminalReason"] = "tool_failure"
				terminal["code"] = projection.Code
				terminal["message"] = projection.Message
				terminal["severity"] = projection.Severity
			}
			raw[1]["error"] = projection.Message
			delete(projected[1], "error")
			raw[1]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[1])
			if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err != nil {
				t.Fatalf("compatible tool failure was rejected: %v", err)
			}
		})
	}

	raw, projected := generalTerminalDeliveryBatchFixture(t, false)
	wrong := domainfailure.New(domainfailure.CodeProviderError, nil)
	for _, terminal := range []map[string]any{raw[1], projected[1]} {
		terminal["terminalReason"] = "tool_failure"
		terminal["code"] = wrong.Code()
		terminal["message"] = wrong.Message()
		terminal["severity"] = wrong.Severity()
	}
	raw[1]["error"] = wrong.Message()
	delete(projected[1], "error")
	raw[1]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[1])
	if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
		t.Fatal("incompatible provider failure projection was admitted for a tool terminal")
	}
}

func TestGeneralTerminalDeliveryBatchV1AcceptsExactAndLegacyProviderFailureCodes(t *testing.T) {
	for _, code := range []string{
		domainfailure.CodeProviderError,
		domainfailure.CodeProviderAuthenticationFailed,
		domainfailure.CodeProviderReasoningMarkupInvalid,
		domainfailure.CodeProviderEmptyFinal,
	} {
		t.Run(code, func(t *testing.T) {
			raw, projected := generalTerminalDeliveryBatchFixture(t, false)
			projection, ok := domainterminal.GeneralFailureProjectionForCodeV1("provider_failure", code)
			if !ok {
				t.Fatalf("provider failure projection unavailable for %q", code)
			}
			for _, terminal := range []map[string]any{raw[1], projected[1]} {
				terminal["terminalReason"] = "provider_failure"
				terminal["code"] = projection.Code
				terminal["message"] = projection.Message
				terminal["severity"] = projection.Severity
			}
			raw[1]["error"] = projection.Message
			delete(projected[1], "error")
			raw[1]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[1])
			if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err != nil {
				t.Fatalf("compatible provider failure was rejected: %v", err)
			}
		})
	}

	raw, projected := generalTerminalDeliveryBatchFixture(t, false)
	wrong := domainfailure.New(domainfailure.CodeHostCandidateAuthorityFailed, nil)
	for _, terminal := range []map[string]any{raw[1], projected[1]} {
		terminal["terminalReason"] = "provider_failure"
		terminal["code"] = wrong.Code()
		terminal["message"] = wrong.Message()
		terminal["severity"] = wrong.Severity()
	}
	raw[1]["error"] = wrong.Message()
	delete(projected[1], "error")
	raw[1]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[1])
	if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
		t.Fatal("incompatible host failure projection was admitted for a provider terminal")
	}
}

func TestGeneralTerminalDeliveryBatchV1AcceptsContextAdmissionAsSemanticFailure(t *testing.T) {
	raw, projected := generalTerminalDeliveryBatchFixture(t, false)
	projection, ok := domainterminal.GeneralFailureProjectionForCodeV1(
		"semantic_failure", "context_window_hard_limit",
	)
	if !ok {
		t.Fatal("context admission semantic projection is unavailable")
	}
	for _, terminal := range []map[string]any{raw[1], projected[1]} {
		terminal["terminalReason"] = "semantic_failure"
		terminal["code"] = projection.Code
		terminal["message"] = projection.Message
		terminal["severity"] = projection.Severity
	}
	raw[1]["error"] = projection.Message
	delete(projected[1], "error")
	raw[1]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[1])
	if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err != nil {
		t.Fatalf("context admission semantic failure was rejected: %v", err)
	}
}

func TestGeneralTerminalDeliveryBatchV1RetainsClosedToolNotAdvertisedDiagnostics(t *testing.T) {
	details := map[string]any{
		"rejectedToolNormalizedNameSha256":       strings.Repeat("a", 64),
		"rejectedToolCategory":                   "known_builtin_not_advertised",
		"promptRoute":                            "tool_agent",
		"loopStep":                               float64(1),
		"advertisedToolCount":                    float64(7),
		"advertisedToolManifestHash":             strings.Repeat("b", 64),
		"advertisedNameSetSortedHash":            strings.Repeat("c", 64),
		"providerRequestToolManifestHash":        strings.Repeat("d", 64),
		"runToolStepManifestHash":                strings.Repeat("d", 64),
		"providerRequestRunToolStepManifestSame": true,
	}
	raw, projected := generalTerminalDeliveryBatchFixture(t, true)
	failure := domainfailure.New("tool_not_advertised", details)
	for _, events := range [][]map[string]any{raw, projected} {
		item := events[0]["item"].(map[string]any)
		delete(item, "text")
		item["role"], item["kind"], item["status"] = "system", "error", "failed"
		item["code"], item["message"], item["severity"] = failure.Code(), failure.Message(), failure.Severity()
		item["details"] = failure.Details()
		events[1]["usageFinalStatus"] = "failed"
		terminal := events[2]
		terminal["kind"], terminal["status"], terminal["terminalReason"] = "turn_failed", "failed", "tool_failure"
		terminal["itemId"] = events[0]["itemId"]
		terminal["code"], terminal["message"], terminal["severity"] = failure.Code(), failure.Message(), failure.Severity()
		terminal["details"] = failure.Details()
	}
	raw[2]["error"] = failure.Message()
	delete(projected[2], "error")
	for _, event := range raw {
		event["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(event)
	}
	batch, err := NewGeneralTerminalDeliveryBatchV1(raw, projected)
	if err != nil {
		t.Fatalf("closed unadvertised-tool diagnostic batch was rejected: %v", err)
	}
	terminal := batch.Events[len(batch.Events)-1]
	terminalBody, marshalErr := json.Marshal(terminal)
	expectedDetailsBody, expectedMarshalErr := json.Marshal(details)
	actualDetailsBody, actualMarshalErr := json.Marshal(terminal["details"])
	if marshalErr != nil || expectedMarshalErr != nil || actualMarshalErr != nil ||
		string(actualDetailsBody) != string(expectedDetailsBody) || strings.Contains(string(terminalBody), `"toolName"`) {
		t.Fatalf("tool rejection diagnostics were altered or opened: %#v", terminal)
	}

	projected[2]["details"].(map[string]any)["toolName"] = "read_file"
	if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
		t.Fatal("raw rejected tool name entered the public terminal batch")
	}
}

func TestGeneralTerminalDeliveryBatchV1KeepsDurableFailureErrorPrivate(t *testing.T) {
	t.Run("durable error is required", func(t *testing.T) {
		raw, projected := generalTerminalCompletedFailureFixture(t, "approval_denied")
		delete(raw[2], "error")
		raw[2]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[2])
		if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
			t.Fatal("durable host failure without its closed error record was accepted")
		}
	})

	t.Run("public error is rejected", func(t *testing.T) {
		raw, projected := generalTerminalCompletedFailureFixture(t, "approval_denied")
		projected[2]["error"] = projected[2]["message"]
		if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
			t.Fatal("durable-only error bytes crossed the ordinary public projection")
		}
	})
}

func TestGeneralTerminalDeliveryBatchV1RejectsRehashedOpenProjectedRecords(t *testing.T) {
	tests := map[string]func([]map[string]any, []map[string]any){
		"arbitrary assistant text": func(raw, projected []map[string]any) {
			raw[0]["item"].(map[string]any)["text"] = "arbitrary provider prose"
			projected[0]["item"].(map[string]any)["text"] = "arbitrary provider prose"
		},
		"unknown event field": func(raw, projected []map[string]any) {
			raw[0]["goAcceptedExtra"] = "extra"
			projected[0]["goAcceptedExtra"] = "extra"
		},
		"unknown item field": func(raw, projected []map[string]any) {
			raw[0]["item"].(map[string]any)["goAcceptedExtra"] = "extra"
			projected[0]["item"].(map[string]any)["goAcceptedExtra"] = "extra"
		},
		"unsafe nested usage integer": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				usage := events[1]["usage"].(map[string]any)
				usage["promptTokens"] = float64(9_007_199_254_740_992)
				usage["totalTokens"] = float64(9_007_199_254_740_992)
			}
		},
		"unknown nested usage field": func(raw, projected []map[string]any) {
			raw[1]["usage"].(map[string]any)["untrusted"] = true
			projected[1]["usage"].(map[string]any)["untrusted"] = true
		},
		"inconsistent usage total": func(raw, projected []map[string]any) {
			raw[1]["usage"].(map[string]any)["totalTokens"] = 99
			projected[1]["usage"].(map[string]any)["totalTokens"] = 99
		},
		"unknown cache diagnostic": func(raw, projected []map[string]any) {
			raw[1]["cacheDiagnostics"].(map[string]any)["untrusted"] = true
			projected[1]["cacheDiagnostics"].(map[string]any)["untrusted"] = true
		},
		"negative zero usage token": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[1]["usage"].(map[string]any)["promptTokens"] = math.Copysign(0, -1)
			}
		},
		"negative zero cache diagnostic": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[1]["cacheDiagnostics"].(map[string]any)["firstTokenLatencyMs"] = math.Copysign(0, -1)
			}
		},
		"terminal optional wrong type": func(raw, projected []map[string]any) {
			raw[2]["message"] = 7
			projected[2]["message"] = 7
		},
		"non canonical item timestamp": func(raw, projected []map[string]any) {
			raw[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T01:59:59,1Z"
			projected[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T01:59:59,1Z"
		},
		"item timestamp above nanosecond precision": func(raw, projected []map[string]any) {
			raw[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T01:59:59.1234567890Z"
			projected[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T01:59:59.1234567890Z"
		},
		"item timestamp with invalid offset": func(raw, projected []map[string]any) {
			raw[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T01:59:59+24:00"
			projected[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T01:59:59+24:00"
		},
		"item created after finished": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[0]["item"].(map[string]any)["createdAt"] = "2026-07-20T02:00:00.000000001Z"
			}
		},
		"non canonical item identity": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[0]["itemId"] = " item-general "
				events[0]["item"].(map[string]any)["id"] = " item-general "
			}
		},
		"NEL item identity boundary": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[0]["itemId"] = "\u0085item-general"
				events[0]["item"].(map[string]any)["id"] = "\u0085item-general"
			}
		},
		"BOM item identity boundary": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[0]["itemId"] = "\ufeffitem-general"
				events[0]["item"].(map[string]any)["id"] = "\ufeffitem-general"
			}
		},
		"slash item identity": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[0]["itemId"] = "item/general"
				events[0]["item"].(map[string]any)["id"] = "item/general"
			}
		},
		"overlong child run identity": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[1]["childRunId"] = strings.Repeat("r", 257)
			}
		},
		"non canonical usage source": func(raw, projected []map[string]any) {
			raw[1]["usageSource"] = " provider_usage "
			projected[1]["usageSource"] = " provider_usage "
		},
		"non canonical failure code": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				events[2]["code"] = "turn_failed "
				events[2]["message"] = "The turn failed before a verified response was available."
			}
		},
		"non canonical error item code": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				item := events[0]["item"].(map[string]any)
				delete(item, "text")
				item["role"] = "system"
				item["kind"] = "error"
				item["status"] = "failed"
				item["code"] = " turn_failed "
				item["message"] = "The turn failed before a verified response was available."
				item["severity"] = "error"
				events[1]["usageFinalStatus"] = "failed"
				events[2]["kind"] = "turn_failed"
				events[2]["status"] = "failed"
				events[2]["terminalReason"] = "provider_failure"
			}
		},
		"successful lifecycle carries failure projection": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				failure := domainfailure.New("provider_timeout", nil)
				events[2]["code"] = failure.Code()
				events[2]["message"] = failure.Message()
				events[2]["error"] = failure.Message()
				events[2]["severity"] = failure.Severity()
			}
		},
		"error item and lifecycle use different canonical failures": func(raw, projected []map[string]any) {
			for _, events := range [][]map[string]any{raw, projected} {
				itemFailure := domainfailure.New("provider_timeout", nil)
				lifecycleFailure := domainfailure.New("provider_rate_limited", nil)
				item := events[0]["item"].(map[string]any)
				delete(item, "text")
				item["role"], item["kind"], item["status"] = "system", "error", "failed"
				item["code"], item["message"], item["severity"] = itemFailure.Code(), itemFailure.Message(), itemFailure.Severity()
				events[1]["usageFinalStatus"] = "failed"
				events[2]["kind"], events[2]["status"], events[2]["terminalReason"] = "turn_failed", "failed", "provider_failure"
				events[2]["itemId"] = events[0]["itemId"]
				events[2]["code"], events[2]["message"], events[2]["error"] = lifecycleFailure.Code(), lifecycleFailure.Message(), lifecycleFailure.Message()
				events[2]["severity"] = lifecycleFailure.Severity()
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			raw, projected := generalTerminalDeliveryBatchFixture(t, true)
			mutate(raw, projected)
			for _, event := range raw {
				event["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(event)
			}
			if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
				t.Fatal("rehashed open projected record was accepted")
			}
		})
	}
}

func TestGeneralTerminalDeliveryBatchV1RejectsInconsistentCancelDisposition(t *testing.T) {
	raw, projected := generalTerminalDeliveryBatchFixture(t, false)
	for _, events := range [][]map[string]any{raw, projected} {
		events[0]["usageFinalStatus"] = "aborted"
		terminal := events[1]
		terminal["kind"] = "turn_aborted"
		terminal["status"] = "aborted"
		terminal["terminalReason"] = "cancel"
		terminal["discard"] = true
		terminal["cancelled"] = false
		terminal["cancelledPendingGates"] = 1
	}
	for _, event := range raw {
		event["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(event)
	}
	if _, err := NewGeneralTerminalDeliveryBatchV1(raw, projected); err == nil {
		t.Fatal("pending gate cancellation was accepted while cancelled=false")
	}
}

func TestGeneralTerminalDeliveryBatchV1RejectsSequencesAboveJSSafeInteger(t *testing.T) {
	if int64(int(^uint(0)>>1)) <= generalTerminalMaxSafeIntegerV1 {
		return
	}
	raw, projected := generalTerminalDeliveryBatchFixture(t, false)
	batch, err := NewGeneralTerminalDeliveryBatchV1(raw, projected)
	if err != nil {
		t.Fatal(err)
	}
	batch.FirstSeq = int(generalTerminalMaxSafeIntegerV1 + 1)
	batch.LastSeq = int(generalTerminalMaxSafeIntegerV1 + 2)
	batch.Seq = batch.LastSeq
	batch.Events[0]["seq"] = int64(batch.FirstSeq)
	batch.Events[1]["seq"] = int64(batch.LastSeq)
	batch.ProjectedEventsDigest = generalTerminalProjectedEventsDigestV1(batch.Events)
	batch.BatchDigest = generalTerminalDeliveryBatchDigestV1(batch)
	if err := ValidateGeneralTerminalDeliveryBatchV1(batch); err == nil {
		t.Fatal("general terminal batch accepted a sequence that the desktop cannot represent exactly")
	}
}

func TestGeneralTerminalDeliveryEventsV1RejectsMalformedDuplicateOutOfOrderAndCrossTurnGroups(t *testing.T) {
	raw, _ := generalTerminalDeliveryBatchFixture(t, true)
	tests := map[string]func([]map[string]any) []map[string]any{
		"missing": func(events []map[string]any) []map[string]any { return events[:2] },
		"duplicate": func(events []map[string]any) []map[string]any {
			events[1]["generalTerminalEventId"] = events[0]["generalTerminalEventId"]
			return events
		},
		"out_of_order": func(events []map[string]any) []map[string]any {
			events[1], events[2] = events[2], events[1]
			return events
		},
		"cross_turn": func(events []map[string]any) []map[string]any {
			events[1]["turnId"] = "turn-other"
			return events
		},
		"partial_markers": func(events []map[string]any) []map[string]any {
			delete(events[1], "generalTerminalAuthorityDigest")
			return events
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := cloneAcceptedFinalDeliveryEvents(raw)
			if err := ValidateGeneralTerminalDeliveryEventsV1(mutate(candidate)); err == nil {
				t.Fatal("malformed general terminal group was accepted")
			}
		})
	}
}

func TestGeneralTerminalReplayEventsAfterRewindsInsideWholeGroupAndRejectsSplitCommit(t *testing.T) {
	raw, _ := generalTerminalDeliveryBatchFixture(t, true)
	events := append([]map[string]any{{
		"kind": "pipeline_stage", "threadId": "thread-general", "turnId": "turn-general",
		"seq": float64(10), "timestamp": "2026-07-20T02:00:00Z", "stage": "response_received",
	}}, raw...)
	replayed, err := GeneralTerminalReplayEventsAfter(events, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 3 || contracts.StringField(replayed[0], "generalTerminalSlot") != "terminal-item" ||
		contracts.StringField(replayed[2], "generalTerminalSlot") != "terminal" {
		t.Fatalf("cursor returned a terminal suffix instead of the whole group: %#v", replayed)
	}
	replayed, err = GeneralTerminalReplayEventsAfter(events, 13)
	if err != nil || len(replayed) != 0 {
		t.Fatalf("cursor after group replayed terminal records: events=%#v err=%v", replayed, err)
	}

	split := append(cloneAcceptedFinalDeliveryEvents(raw[:1]), map[string]any{
		"kind": "pipeline_stage", "threadId": "thread-general", "turnId": "turn-general",
		"seq": float64(12), "timestamp": "2026-07-20T02:00:00Z", "stage": "response_received",
	})
	split = append(split, cloneAcceptedFinalDeliveryEvents(raw[1:])...)
	if _, err := GeneralTerminalReplayEventsAfter(split, 0); err == nil {
		t.Fatal("split general terminal commit did not fail closed")
	}
}

func TestAtomicTerminalReplayEventsAfterPreservesAcceptedFinalRewind(t *testing.T) {
	accepted := acceptedFinalSemanticEventsForTest(t, "success", "completed")
	replayed, err := AtomicTerminalReplayEventsAfter(accepted, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 3 || contracts.StringField(replayed[0], "publicationSlot") != "assistant-final" ||
		contracts.StringField(replayed[2], "publicationSlot") != "terminal" {
		t.Fatalf("combined atomic cursor returned an accepted-final suffix: %#v", replayed)
	}
	accepted[0]["generalTerminalCommitId"] = domainsecurity.SHA256Hex([]byte("mixed-authority"))
	if _, err := AtomicTerminalReplayEventsAfter(accepted, 0); err == nil {
		t.Fatal("accepted-final and general-terminal authorities were mixed in one delivery")
	}
}

func generalTerminalDeliveryBatchFixture(t *testing.T, withItem bool) ([]map[string]any, []map[string]any) {
	t.Helper()
	commitID := domainsecurity.SHA256Hex([]byte("general-terminal-commit"))
	authorityDigest := domainsecurity.SHA256Hex([]byte("general-terminal-cas-binding"))
	timestamp := "2026-07-20T02:00:00Z"
	raw := []map[string]any{}
	if withItem {
		raw = append(raw, map[string]any{
			"kind": "item_completed", "threadId": "thread-general", "turnId": "turn-general", "itemId": "item-general",
			"timestamp": timestamp, "item": map[string]any{
				"id": "item-general", "threadId": "thread-general", "turnId": "turn-general", "role": "assistant",
				"kind": "assistant_text", "status": "completed", "createdAt": timestamp, "finishedAt": timestamp,
				"text": GeneralTerminalCompletedBoundaryTextV1,
			},
		})
	}
	raw = append(raw,
		map[string]any{
			"kind": "usage", "threadId": "thread-general", "turnId": "turn-general", "timestamp": timestamp,
			"model": "", "usage": domainterminaltelemetry.ProviderUsageMap(domainmodel.Usage{}),
			"cacheDiagnostics": map[string]any{},
			"usageFinalStatus": map[bool]string{true: "completed", false: "failed"}[withItem],
		},
		map[string]any{
			"kind":     map[bool]string{true: "turn_completed", false: "turn_failed"}[withItem],
			"threadId": "thread-general", "turnId": "turn-general", "timestamp": timestamp,
			"status":         map[bool]string{true: "completed", false: "failed"}[withItem],
			"terminalReason": map[bool]string{true: "success", false: "provider_failure"}[withItem],
		},
	)
	if !withItem {
		projection, ok := domainterminal.GeneralFailureProjectionV1("provider_failure")
		if !ok {
			t.Fatal("provider failure projection is unavailable")
		}
		terminal := raw[len(raw)-1]
		terminal["code"] = projection.Code
		terminal["message"] = projection.Message
		terminal["error"] = projection.Message
		terminal["severity"] = projection.Severity
	}
	slots := generalTerminalDeliverySlots
	if withItem {
		slots = generalTerminalDeliveryWithItemSlots
	}
	for index, event := range raw {
		event["seq"] = float64(11 + index)
		event["generalTerminalCommitId"] = commitID
		event["generalTerminalEventId"] = GeneralTerminalDeliveryEventIDV1(commitID, slots[index])
		event["generalTerminalSlot"] = slots[index]
		event["generalTerminalAuthorityKind"] = GeneralTerminalCASAuthorityKind
		event["generalTerminalAuthorityDigest"] = authorityDigest
		event["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(event)
	}
	projected := cloneAcceptedFinalDeliveryEvents(raw)
	for _, event := range projected {
		for _, marker := range generalTerminalDeliveryPublicationMarkerNames {
			delete(event, marker)
		}
	}
	if !withItem {
		delete(projected[len(projected)-1], "error")
	}
	// Force a stable JSON representation like the production projection path.
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &projected); err != nil {
		t.Fatal(err)
	}
	return raw, projected
}

func generalTerminalCompletedFailureFixture(t *testing.T, reason string) ([]map[string]any, []map[string]any) {
	t.Helper()
	raw, projected := generalTerminalDeliveryBatchFixture(t, true)
	projection, ok := domainterminal.GeneralFailureProjectionV1(reason)
	if !ok || projection.Status != "completed" {
		t.Fatalf("completed host failure projection is unavailable for %q", reason)
	}
	for _, terminal := range []map[string]any{raw[2], projected[2]} {
		terminal["terminalReason"] = reason
		terminal["code"] = projection.Code
		terminal["message"] = projection.Message
		terminal["severity"] = projection.Severity
	}
	raw[2]["error"] = projection.Message
	delete(projected[2], "error")
	raw[2]["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(raw[2])
	return raw, projected
}
