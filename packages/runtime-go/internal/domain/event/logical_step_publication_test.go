package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
)

const logicalStepPrivacyIDV1 = "bbbbbbbb-bbbb-4bbb-abbb-123456789012"

func logicalStepPrivacyFixtureV1(stage string) map[string]any {
	labels := map[string]string{"logical_step_started": "Logical step started", "logical_step_assembled": "Logical step assembled", "logical_step_closed": "Logical step closed"}
	details := map[string]any{"schemaVersion": float64(1), "logicalStepId": logicalStepPrivacyIDV1}
	if stage == "logical_step_closed" {
		details["assistantMessageAssembled"], details["llmMs"] = true, float64(80)
	}
	return map[string]any{"kind": "pipeline_stage", "threadId": "thread-logical", "turnId": "turn-logical", "stage": stage, "label": labels[stage], "details": details, "seq": float64(1), "timestamp": "2026-10-18T00:00:00Z"}
}

func TestLogicalStepPrivacyPreservesOnlyClosedRootCorrelationV1(t *testing.T) {
	for _, stage := range []string{"logical_step_started", "logical_step_assembled", "logical_step_closed"} {
		t.Run(stage, func(t *testing.T) {
			event := logicalStepPrivacyFixtureV1(stage)
			before, _ := json.Marshal(event)
			if err := ValidatePublicRecord(event); err != nil {
				t.Fatalf("validated correlation was classified as prose PII: %v", err)
			}
			projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(event)
			after, _ := json.Marshal(projected)
			if changed || !bytes.Equal(before, after) {
				t.Fatalf("closed correlation changed: %s", after)
			}
			original, _ := json.Marshal(event)
			if !bytes.Equal(before, original) {
				t.Fatal("projection or validation mutated producer input")
			}
		})
	}
}

func TestLogicalStepPrivacyScansUnknownDetailsProseAndNestedLookalikesV1(t *testing.T) {
	closed := logicalStepPrivacyFixtureV1("logical_step_started")
	encoded, _ := json.Marshal(closed)
	unknownDetails := contracts.CloneMap(closed)
	unknownDetails["details"].(map[string]any)["account"] = "6222021234567890"
	extraField := contracts.CloneMap(closed)
	extraField["text"] = "account 6222021234567890"
	wrongLabel := contracts.CloneMap(closed)
	wrongLabel["label"] = "account 6222021234567890"
	wrongStage := contracts.CloneMap(closed)
	wrongStage["stage"] = "response_received"
	for name, value := range map[string]any{
		"unknown-details":      unknownDetails,
		"extra-envelope-field": extraField,
		"wrong-label":          wrongLabel,
		"unrelated-stage":      wrongStage,
		"bare-details":         map[string]any{"details": closed["details"]},
		"nested-map":           map[string]any{"kind": "usage", "payload": closed},
		"nested-array":         map[string]any{"kind": "usage", "payload": []any{closed}},
		"root-array":           []any{closed},
		"encoded-prose":        map[string]any{"kind": "usage", "text": string(encoded)},
		"user-prose":           map[string]any{"kind": "usage", "text": logicalStepPrivacyIDV1 + " account 6222021234567890"},
		"same-field-name":      map[string]any{"kind": "usage", "details": map[string]any{"logicalStepId": logicalStepPrivacyIDV1}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicRecord(value); !errors.Is(err, ErrPrivacyProjection) {
				t.Fatalf("PII lookalike was not rejected: %v", err)
			}
			projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(value)
			body, err := json.Marshal(projected)
			if err != nil || !changed || projected == nil || strings.Contains(string(body), "123456789012") || strings.Contains(string(body), "6222021234567890") {
				t.Fatalf("PII survived projection: %s changed=%v err=%v", body, changed, err)
			}
			if err := ValidatePublicRecord(projected); err != nil {
				t.Fatalf("ordinary projected value is invalid: %v", err)
			}
		})
	}
}
