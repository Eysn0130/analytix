package thread

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const trustedLogicalStepPrivacyIDV1 = "bbbbbbbb-bbbb-4bbb-abbb-123456789012"

func trustedLogicalStepPrivacyFixtureV1(stage string) map[string]any {
	labels := map[string]string{
		"logical_step_started": "Logical step started", "logical_step_assembled": "Logical step assembled",
		"logical_step_closed": "Logical step closed",
	}
	details := map[string]any{"schemaVersion": float64(1), "logicalStepId": trustedLogicalStepPrivacyIDV1}
	if stage == "logical_step_closed" {
		details["assistantMessageAssembled"] = true
		details["llmMs"], details["ttftMs"] = float64(80), float64(20)
		details["decodeMs"], details["decodeTokens"] = float64(60), float64(5)
	}
	return map[string]any{
		"kind": "pipeline_stage", "threadId": "thread-logical", "turnId": "turn-logical",
		"seq": float64(1), "timestamp": "2026-10-18T00:00:00.123456789Z",
		"stage": stage, "label": labels[stage], "details": details,
	}
}

func TestTrustedPublicProjectorPreservesClosedLogicalStepCorrelationV1(t *testing.T) {
	for _, stage := range []string{"logical_step_started", "logical_step_assembled", "logical_step_closed"} {
		t.Run(stage, func(t *testing.T) {
			thread := map[string]any{"id": "thread-logical", "turns": []any{
				map[string]any{"id": "turn-logical", "threadId": "thread-logical", "items": []any{}},
			}}
			input := trustedLogicalStepPrivacyFixtureV1(stage)
			beforeEvent, _ := json.Marshal(input)
			beforeThread, _ := json.Marshal(thread)
			projected, visible, err := NewTrustedPublicProjector(nil).ProjectEvent("thread-logical", thread, input)
			if err != nil || !visible || !reflect.DeepEqual(projected, input) {
				t.Fatalf("public logical event changed: projected=%#v visible=%t err=%v", projected, visible, err)
			}
			afterEvent, _ := json.Marshal(input)
			afterThread, _ := json.Marshal(thread)
			if string(beforeEvent) != string(afterEvent) || string(beforeThread) != string(afterThread) {
				t.Fatal("public event projection mutated its source event or thread")
			}
		})
	}
}

func TestTrustedLogicalStepStagingDoesNotExemptOrdinaryScopesV1(t *testing.T) {
	for name, scope := range map[string]trustedProjectionScopeV1{
		"ordinary": trustedProjectionOrdinaryV1, "thread": trustedProjectionThreadRootV1,
		"turn": trustedProjectionTurnV1, "item": trustedProjectionItemV1,
	} {
		t.Run(name, func(t *testing.T) {
			input := trustedLogicalStepPrivacyFixtureV1("logical_step_started")
			before, _ := json.Marshal(input)
			projected := projectTrustedStructuredOrdinaryV1(input, scope)
			body, err := json.Marshal(projected)
			if err != nil || projected == nil || strings.Contains(string(body), "123456789012") {
				t.Fatalf("ordinary scope exempted a correlation lookalike: %s err=%v", body, err)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("ordinary projection mutated its input")
			}
		})
	}
}

func TestTrustedLogicalStepStagingDoesNotExemptMalformedOrNestedEventsV1(t *testing.T) {
	wrongLabel := trustedLogicalStepPrivacyFixtureV1("logical_step_started")
	wrongLabel["label"] = "Wrong label"
	unknownDetails := trustedLogicalStepPrivacyFixtureV1("logical_step_started")
	unknownDetails["details"].(map[string]any)["extra"] = "ordinary prose"
	extraProse := trustedLogicalStepPrivacyFixtureV1("logical_step_started")
	extraProse["text"] = "account 6222021234567890"
	for name, input := range map[string]any{
		"wrong-label":     wrongLabel,
		"unknown-details": unknownDetails,
		"extra-prose":     extraProse,
		"nested-event":    map[string]any{"payload": trustedLogicalStepPrivacyFixtureV1("logical_step_started")},
		"nested-array":    map[string]any{"payload": []any{trustedLogicalStepPrivacyFixtureV1("logical_step_started")}},
		"bare-details":    map[string]any{"details": trustedLogicalStepPrivacyFixtureV1("logical_step_started")["details"]},
	} {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(input)
			projected := projectTrustedStructuredOrdinaryV1(input, trustedProjectionEventRootV1)
			body, err := json.Marshal(projected)
			if err != nil || projected == nil || strings.Contains(string(body), "123456789012") ||
				strings.Contains(string(body), "6222021234567890") {
				t.Fatalf("malformed or nested event bypassed ordinary projection: %s err=%v", body, err)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("malformed or nested projection mutated its input")
			}
		})
	}
}
