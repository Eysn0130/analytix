package eventlifecycle

import (
	"encoding/json"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func closedLogicalStepFixtureV1(stage string) map[string]any {
	labels := map[string]string{"logical_step_started": "Logical step started", "logical_step_assembled": "Logical step assembled", "logical_step_closed": "Logical step closed"}
	details := map[string]any{"schemaVersion": json.Number("1"), "logicalStepId": "bbbbbbbb-bbbb-4bbb-abbb-123456789012"}
	if stage == "logical_step_closed" {
		details["assistantMessageAssembled"] = true
		details["llmMs"], details["ttftMs"] = json.Number("80"), json.Number("30")
		details["decodeMs"], details["decodeTokens"] = json.Number("50"), json.Number("12")
	}
	return map[string]any{"kind": "pipeline_stage", "threadId": "thread-logical", "turnId": "turn-logical", "stage": stage, "label": labels[stage], "details": details}
}

func TestClosedLogicalStepEventAcceptsDraftAndDurableMetadataV1(t *testing.T) {
	for _, stage := range []string{"logical_step_started", "logical_step_assembled", "logical_step_closed"} {
		for _, durable := range []bool{false, true} {
			event := closedLogicalStepFixtureV1(stage)
			if durable {
				event["seq"], event["timestamp"] = json.Number("1"), "2026-10-18T00:00:00.123456789Z"
			}
			details, ok := domainevent.ProjectClosedLogicalStepEventDetailsV1(event)
			if !ok || details["logicalStepId"] != "bbbbbbbb-bbbb-4bbb-abbb-123456789012" {
				t.Fatalf("closed %s durable=%v rejected: %#v", stage, durable, details)
			}
			if durable && ValidateNewEventLifecycleV1(event) != nil {
				t.Fatalf("durable %s rejected by lifecycle", stage)
			}
			if !durable && stage == "logical_step_assembled" && ValidateNewEventLifecycleV1(event) == nil {
				t.Fatal("draft assembly became persistable without host metadata")
			}
		}
	}
}

func TestClosedLogicalStepEventRejectsOpenEnvelopeOrDetailsV1(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"unknown-envelope":  func(e map[string]any) { e["text"] = "account 6222021234567890" },
		"wrong-label":       func(e map[string]any) { e["label"] = "logical step started" },
		"padded-stage":      func(e map[string]any) { e["stage"] = " logical_step_started" },
		"wrong-kind":        func(e map[string]any) { e["kind"] = "usage" },
		"missing-thread":    func(e map[string]any) { delete(e, "threadId") },
		"invalid-turn":      func(e map[string]any) { e["turnId"] = "turn logical" },
		"zero-seq":          func(e map[string]any) { e["seq"] = json.Number("0") },
		"inexact-seq":       func(e map[string]any) { e["seq"] = json.Number("9007199254740993") },
		"invalid-timestamp": func(e map[string]any) { e["timestamp"] = "2026-02-30T00:00:00Z" },
		"nil-timestamp":     func(e map[string]any) { e["timestamp"] = nil },
		"missing-details":   func(e map[string]any) { delete(e, "details") },
		"unknown-details":   func(e map[string]any) { e["details"].(map[string]any)["account"] = "6222021234567890" },
		"unknown-version":   func(e map[string]any) { e["details"].(map[string]any)["schemaVersion"] = float64(2) },
		"noncanonical-id": func(e map[string]any) {
			e["details"].(map[string]any)["logicalStepId"] = "BBBBBBBB-bbbb-4bbb-abbb-123456789012"
		},
	} {
		t.Run(name, func(t *testing.T) {
			event := contracts.CloneMap(closedLogicalStepFixtureV1("logical_step_started"))
			mutate(event)
			if _, ok := domainevent.ProjectClosedLogicalStepEventDetailsV1(event); ok {
				t.Fatal("open logical-step record received structural classification")
			}
		})
	}
	for name, mutate := range map[string]func(map[string]any){
		"unassembled-metrics":   func(d map[string]any) { d["assistantMessageAssembled"] = false },
		"ttft-exceeds-llm":      func(d map[string]any) { d["ttftMs"] = float64(81) },
		"decode-mismatch":       func(d map[string]any) { d["decodeMs"] = float64(49) },
		"missing-decode-tokens": func(d map[string]any) { delete(d, "decodeTokens") },
		"negative-metric":       func(d map[string]any) { d["decodeTokens"] = float64(-1) },
	} {
		t.Run(name, func(t *testing.T) {
			event := contracts.CloneMap(closedLogicalStepFixtureV1("logical_step_closed"))
			mutate(event["details"].(map[string]any))
			if _, ok := domainevent.ProjectClosedLogicalStepEventDetailsV1(event); ok {
				t.Fatal("invalid metric relation received structural classification")
			}
		})
	}
}
