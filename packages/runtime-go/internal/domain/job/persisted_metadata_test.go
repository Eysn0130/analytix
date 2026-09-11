package job

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func validPersistedModelExecutionFixtureV1() map[string]any {
	return map[string]any{
		"providerId":            "deepseek",
		"modelId":               "deepseek-chat",
		"source":                "runtime-default",
		"resolvedAt":            "2026-07-18T12:34:56.123456789Z",
		"endpointFormat":        "chat_completions",
		"baseUrlFingerprint":    strings.Repeat("a", 64),
		"capabilityFingerprint": strings.Repeat("b", 64),
	}
}

func validBackgroundUsageFixtureV1() map[string]any {
	return map[string]any{
		"exitCode": float64(0), "durationMs": float64(12), "outputBytes": float64(7),
		"maxOutputBytes": float64(1024), "truncated": false, "timedOut": false,
	}
}

func TestPersistedModelExecutionV1UsesExactClosedProjection(t *testing.T) {
	valid := validPersistedModelExecutionFixtureV1()
	if err := ValidatePersistedModelExecutionV1(valid); err != nil ||
		!reflect.DeepEqual(ProjectPersistedModelExecutionV1(valid), valid) {
		t.Fatalf("valid model execution projection failed: projected=%#v err=%v", ProjectPersistedModelExecutionV1(valid), err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"unknown key":       func(value map[string]any) { value["reasoningContent"] = "PRIVATE_REASONING_SENTINEL" },
		"restricted pii":    func(value map[string]any) { value["providerId"] = "6222020202020202020" },
		"wrong fingerprint": func(value map[string]any) { value["capabilityFingerprint"] = strings.Repeat("A", 64) },
		"wrong type":        func(value map[string]any) { value["modelId"] = map[string]any{"raw": "provider body"} },
		"two endpoints": func(value map[string]any) {
			value["customFullEndpointFingerprint"] = strings.Repeat("c", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := make(map[string]any, len(valid)+1)
			for key, value := range valid {
				candidate[key] = value
			}
			mutate(candidate)
			if !errors.Is(ValidatePersistedModelExecutionV1(candidate), ErrPersistedModelExecutionInvalid) {
				t.Fatalf("invalid model execution was accepted: %#v", candidate)
			}
		})
	}
}

func TestPersistedUsageV1IsBackgroundProcessDiagnosticsOnly(t *testing.T) {
	valid := validBackgroundUsageFixtureV1()
	if err := ValidatePersistedUsageV1("background-shell", valid); err != nil ||
		!reflect.DeepEqual(ProjectPersistedUsageV1("background-shell", valid), valid) {
		t.Fatalf("valid background usage failed: projected=%#v err=%v", ProjectPersistedUsageV1("background-shell", valid), err)
	}
	poisoned := validBackgroundUsageFixtureV1()
	poisoned["reasoningTokens"] = "PRIVATE_REASONING_SENTINEL"
	if !errors.Is(ValidatePersistedUsageV1("background-shell", poisoned), ErrPersistedUsageInvalid) {
		t.Fatalf("unknown background usage key was accepted: %#v", poisoned)
	}
	if ProjectPersistedUsageV1("subagent", map[string]any{"totalTokens": float64(7)}) != nil ||
		!errors.Is(ValidatePersistedUsageV1("subagent", map[string]any{"totalTokens": float64(7)}), ErrPersistedUsageInvalid) {
		t.Fatal("subagent token usage retained a second mutable authority in job storage")
	}
}

func TestNormalizePersistedRecordV1ClearsLegacyJobMetadataPoison(t *testing.T) {
	modelExecution := validPersistedModelExecutionFixtureV1()
	modelExecution["rawProviderResponse"] = "PRIVATE_PROVIDER_BODY_SENTINEL"
	usage := validBackgroundUsageFixtureV1()
	usage["reasoning"] = "PRIVATE_REASONING_SENTINEL"
	record := Record{Kind: "background-shell", ModelExecution: modelExecution, Usage: usage}
	projected := NormalizePersistedRecordV1(record)
	if projected.ModelExecution["rawProviderResponse"] != nil || projected.Usage["reasoning"] != nil ||
		projected.ModelExecution["providerId"] != "deepseek" || projected.Usage["exitCode"] != float64(0) {
		t.Fatalf("legacy job metadata projection mismatch: %#v", projected)
	}
	if ValidatePersistedProjectionV1(record) == nil || ValidatePersistedProjectionV1(projected) != nil {
		t.Fatal("raw and projected job metadata were not distinguished")
	}
}
