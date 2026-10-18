package event

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
)

func TestStageClosedLogicalStepIDPreservesRootCorrelationV1(t *testing.T) {
	for _, stage := range []string{"logical_step_started", "logical_step_assembled", "logical_step_closed"} {
		t.Run(stage, func(t *testing.T) {
			input := logicalStepPrivacyFixtureV1(stage)
			before, _ := json.Marshal(input)
			staged, restore := StageClosedLogicalStepIDV1(input)
			projected := domainordinary.ProjectValueV1(staged)
			restored, ok := restore(projected)
			if !ok || !reflect.DeepEqual(restored, input) {
				t.Fatalf("closed root correlation changed: restored=%#v ok=%t", restored, ok)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("staging or restoration mutated producer input")
			}
		})
	}
}

func TestStageClosedLogicalStepIDDoesNotAdmitLookalikesV1(t *testing.T) {
	wrongLabel := logicalStepPrivacyFixtureV1("logical_step_started")
	wrongLabel["label"] = "Wrong label"
	unknownDetails := logicalStepPrivacyFixtureV1("logical_step_started")
	unknownDetails["details"].(map[string]any)["extra"] = "ordinary prose"
	extraProse := logicalStepPrivacyFixtureV1("logical_step_started")
	extraProse["text"] = "account 6222021234567890"
	for name, input := range map[string]any{
		"wrong-label":       wrongLabel,
		"unknown-details":   unknownDetails,
		"extra-prose":       extraProse,
		"bare-details":      map[string]any{"details": logicalStepPrivacyFixtureV1("logical_step_started")["details"]},
		"nested-event":      map[string]any{"payload": logicalStepPrivacyFixtureV1("logical_step_started")},
		"nested-array":      map[string]any{"payload": []any{logicalStepPrivacyFixtureV1("logical_step_started")}},
		"root-array":        []any{logicalStepPrivacyFixtureV1("logical_step_started")},
		"tool-result-prose": map[string]any{"kind": "tool_result", "output": map[string]any{"summary": logicalStepPrivacyIDV1}},
	} {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(input)
			staged, restore := StageClosedLogicalStepIDV1(input)
			if !reflect.DeepEqual(staged, input) {
				t.Fatal("a non-root or malformed event acquired correlation staging")
			}
			projected := domainordinary.ProjectValueV1(staged)
			restored, ok := restore(projected)
			body, err := json.Marshal(restored)
			if err != nil || !ok || restored == nil || strings.Contains(string(body), "123456789012") ||
				strings.Contains(string(body), "6222021234567890") {
				t.Fatalf("lookalike prose bypassed ordinary projection: %s ok=%t err=%v", body, ok, err)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("lookalike projection mutated its input")
			}
		})
	}
}

func TestStageClosedLogicalStepIDRestorationIsPathBoundV1(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"changed-placeholder": func(staged map[string]any) {
			staged["details"].(map[string]any)["logicalStepId"].(map[string]any)["ordinal"] = float64(9)
		},
		"replaced-placeholder": func(staged map[string]any) {
			staged["details"].(map[string]any)["logicalStepId"] = logicalStepPrivacyIDV1
		},
		"removed-placeholder": func(staged map[string]any) {
			delete(staged["details"].(map[string]any), "logicalStepId")
		},
		"moved-placeholder": func(staged map[string]any) {
			details := staged["details"].(map[string]any)
			details["relocated"] = details["logicalStepId"]
			delete(details, "logicalStepId")
		},
		"moved-container": func(staged map[string]any) {
			staged["relocated"] = staged["details"]
			delete(staged, "details")
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := logicalStepPrivacyFixtureV1("logical_step_started")
			before, _ := json.Marshal(input)
			staged, restore := StageClosedLogicalStepIDV1(input)
			mutate(staged.(map[string]any))
			if restored, ok := restore(staged); ok || restored != nil {
				t.Fatal("changed or relocated placeholder restored a correlation ID")
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("tampering with staged data mutated producer input")
			}
		})
	}
}
