package event

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

func TestValidatePublicRecordRejectsRawFailureContentAtEveryNestingShape(t *testing.T) {
	for name, value := range map[string]any{
		"top level": map[string]any{"kind": "tool_progress", "error": "PRIVATE_PROVIDER_FAILURE"},
		"nested": map[string]any{"kind": "tool_progress", "child": map[string]any{
			"lastError": "PRIVATE_JOB_FAILURE",
		}},
		"stderr": map[string]any{"kind": "tool_progress", "details": map[string]any{
			"stderr": "PRIVATE_PROCESS_FAILURE",
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicRecord(value); !errors.Is(err, ErrRawFailurePersistence) {
				t.Fatalf("raw failure validation error=%v", err)
			}
		})
	}
}

func TestValidatePublicRecordAllowsClosedHostFailure(t *testing.T) {
	failure := domainfailure.New(domainfailure.CodeProviderTimeout, nil)
	record := map[string]any{
		"kind": "turn_failed", "status": "failed", "code": failure.Code(),
		"message": failure.Message(), "error": failure.Message(), "severity": failure.Severity(),
	}
	if err := ValidatePublicRecord(record); err != nil {
		t.Fatalf("closed host failure was rejected: %v", err)
	}
	record["message"] = "provider timeout at account 6222020202020202020"
	record["error"] = record["message"]
	if err := ValidatePublicRecord(record); err == nil {
		t.Fatal("detached failure message was accepted")
	}
}

func TestValidatePublicRecordAllowsOnlyClosedTypedProviderDiagnostic(t *testing.T) {
	record := map[string]any{
		"kind": "pipeline_stage", "stage": "provider_retrying",
		"details": map[string]any{
			"providerError": map[string]any{
				"status": float64(503), "kind": "server", "retryable": true,
				"failureStage": "transport_after_observed_send", "dispatchState": "sent", "attempt": float64(1),
			},
		},
	}
	if err := ValidatePublicRecord(record); err != nil {
		t.Fatalf("closed provider diagnostic was rejected: %v", err)
	}
	strictDecoded := map[string]any{
		"kind": "pipeline_stage", "stage": "provider_retrying",
		"details": map[string]any{
			"providerError": map[string]any{
				"status": json.Number("503"), "kind": "server", "retryable": true,
				"failureStage": "transport_after_observed_send", "dispatchState": "sent", "attempt": json.Number("1"),
			},
		},
	}
	if err := ValidatePublicRecord(strictDecoded); err != nil {
		t.Fatalf("strict-decoded closed provider diagnostic was rejected: %v", err)
	}
	for name, value := range map[string]any{
		"raw string":    "PRIVATE_PROVIDER_FAILURE",
		"raw message":   map[string]any{"status": float64(503), "message": "PRIVATE_PROVIDER_FAILURE"},
		"unknown field": map[string]any{"status": float64(503), "responseBody": "PRIVATE_PROVIDER_FAILURE"},
		"invalid type":  map[string]any{"status": "503"},
		"unknown stage": map[string]any{"failureStage": "guessed", "dispatchState": "sent"},
		"unknown state": map[string]any{"failureStage": "unclassified", "dispatchState": "maybe_sent"},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := map[string]any{
				"kind": "pipeline_stage", "stage": "provider_retrying",
				"details": map[string]any{"providerError": value},
			}
			if err := ValidatePublicRecord(candidate); !errors.Is(err, ErrRawFailurePersistence) {
				t.Fatalf("unsafe provider diagnostic validation error=%v", err)
			}
		})
	}
}

func TestValidatePublicRecordAllowsOnlyExactStagedHostTerminalFailure(t *testing.T) {
	projection, ok := domainterminal.FailureProjectionV1("provider_failure")
	if !ok {
		t.Fatal("closed terminal projection unavailable")
	}
	record := map[string]any{
		"id": "item_turn-1_case_terminal", "turnId": "turn-1", "threadId": "thread-1", "role": "system",
		"status": projection.Status, "kind": "error", "createdAt": "2026-07-20T01:02:03Z", "finishedAt": "2026-07-20T01:02:03Z",
		"code": projection.Code, "message": projection.Message, "severity": projection.Severity,
		"acceptedFinalDigest": strings.Repeat("a", 64),
	}
	if err := ValidatePublicRecord(record); err != nil {
		t.Fatalf("exact staged host terminal failure was rejected before its public CAS: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"raw message": func(value map[string]any) { value["message"] = "PRIVATE_PROVIDER_FAILURE" },
		"raw stack":   func(value map[string]any) { value["stack"] = "PRIVATE_PROVIDER_STACK" },
		"wrong code":  func(value map[string]any) { value["code"] = "case_terminal_unknown" },
		"wrong id":    func(value map[string]any) { value["id"] = "item_other_case_terminal" },
		"bad digest":  func(value map[string]any) { value["acceptedFinalDigest"] = "forged" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := make(map[string]any, len(record)+1)
			for key, value := range record {
				candidate[key] = value
			}
			mutate(candidate)
			if err := ValidatePublicRecord(candidate); !errors.Is(err, ErrRawFailurePersistence) {
				t.Fatalf("non-canonical staged failure validation error=%v candidate=%#v", err, candidate)
			}
		})
	}
}

func TestValidatePublicRecordAllowsClosedAttemptTelemetryOnlyAtUsageRoot(t *testing.T) {
	newUsage := func() map[string]any {
		telemetry := domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{
			PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7,
		}, map[string]any{
			"providerAttemptTelemetrySchema":   domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1,
			"providerAttemptTelemetryValid":    false,
			"providerAttemptTelemetryError":    "invalid_or_incomplete",
			"providerAttemptInventoryComplete": false,
		})
		return map[string]any{
			"kind": "usage", "threadId": "thread-usage", "turnId": "turn-usage", "model": "test-model",
			"timestamp": "2026-10-18T01:02:03Z", "usage": telemetry.PublicUsageMap(),
			"cacheDiagnostics": telemetry.PublicCacheDiagnosticsMap(),
		}
	}
	for _, stage := range []string{"draft", "durable", "public"} {
		t.Run(stage, func(t *testing.T) {
			record := newUsage()
			if stage == "durable" {
				commitID := strings.Repeat("a", 64)
				record["generalTerminalCommitId"] = commitID
				record["generalTerminalEventId"] = GeneralTerminalDeliveryEventIDV1(commitID, "usage")
				record["generalTerminalSlot"] = "usage"
				record["generalTerminalAuthorityKind"] = GeneralTerminalCASAuthorityKind
				record["generalTerminalAuthorityDigest"] = strings.Repeat("b", 64)
			}
			if stage != "draft" {
				record["seq"] = float64(3)
				record["usageFinalStatus"] = "completed"
				record["usageSource"] = "fixture"
				record["childRunId"] = "child-usage"
			}
			if stage == "durable" {
				record["generalTerminalPayloadDigest"] = GeneralTerminalDeliveryPayloadDigestV1(record)
			}
			before, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidatePublicRecord(record); err != nil {
				t.Fatalf("closed %s usage telemetry was rejected: %v", stage, err)
			}
			after, err := json.Marshal(record)
			if err != nil || string(before) != string(after) {
				t.Fatal("usage validation mutated the original record")
			}
		})
	}
	for name, mutate := range map[string]func(map[string]any) any{
		"nested map": func(record map[string]any) any {
			return map[string]any{"kind": "tool_progress", "payload": record}
		},
		"nested array":     func(record map[string]any) any { return []any{record} },
		"bare diagnostics": func(record map[string]any) any { return record["cacheDiagnostics"] },
		"wrong root":       func(record map[string]any) any { record["kind"] = "tool_progress"; return record },
		"missing usage":    func(record map[string]any) any { delete(record, "usage"); return record },
		"noncanonical pair": func(record map[string]any) any {
			record["usage"].(map[string]any)["totalTokens"] = float64(1)
			return record
		},
		"raw enum": func(record map[string]any) any {
			record["cacheDiagnostics"].(map[string]any)["providerAttemptTelemetryError"] = "PRIVATE_PROVIDER_FAILURE"
			return record
		},
		"object enum": func(record map[string]any) any {
			record["cacheDiagnostics"].(map[string]any)["providerAttemptTelemetryError"] = map[string]any{"message": "PRIVATE_PROVIDER_FAILURE"}
			return record
		},
		"telemetry lookalike": func(record map[string]any) any {
			record["cacheDiagnostics"].(map[string]any)["providerAttemptTelemetrySchema"] = "other"
			return record
		},
		"aliased error key": func(record map[string]any) any {
			diagnostics := record["cacheDiagnostics"].(map[string]any)
			delete(diagnostics, "providerAttemptTelemetryError")
			diagnostics["provider_attempt_telemetry_error"] = "invalid_or_incomplete"
			return record
		},
		"raw sibling": func(record map[string]any) any {
			record["lastError"] = "PRIVATE_PROVIDER_FAILURE"
			return record
		},
		"raw diagnostic sibling": func(record map[string]any) any {
			record["cacheDiagnostics"].(map[string]any)["lastError"] = "PRIVATE_PROVIDER_FAILURE"
			return record
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePublicRecord(mutate(contracts.CloneMap(newUsage()))); !errors.Is(err, ErrRawFailurePersistence) {
				t.Fatalf("unsafe usage telemetry validation error=%v", err)
			}
		})
	}
	for name, mutate := range map[string]func(map[string]any){
		"credential":        func(record map[string]any) { record["model"] = "api_key=sk-test-private-value" },
		"private reasoning": func(record map[string]any) { record["model"] = "<think>PRIVATE_REASONING</think>" },
	} {
		t.Run(name, func(t *testing.T) {
			record := newUsage()
			mutate(record)
			if err := ValidatePublicRecord(record); err == nil {
				t.Fatal("closed telemetry bypassed another public-record safety check")
			}
		})
	}
}

func TestClosedTerminalTelemetryFailureValidationViewV1RequiresExactPairWithoutMutation(t *testing.T) {
	newPair := func() (map[string]any, map[string]any) {
		telemetry := domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, map[string]any{
			"providerAttemptTelemetrySchema": domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1,
			"providerAttemptTelemetryValid":  false, "providerAttemptTelemetryError": "invalid_or_incomplete", "providerAttemptInventoryComplete": false,
		})
		return telemetry.PublicUsageMap(), telemetry.PublicCacheDiagnosticsMap()
	}
	for _, fault := range []string{"healthy", "legacy pair", "wrong usage", "unknown usage", "unknown diagnostic", "raw enum", "object enum", "aliased enum", "wrong schema", "raw sibling", "credential", "reasoning", "nested diagnostics"} {
		t.Run(fault, func(t *testing.T) {
			usage, diagnostics := newPair()
			switch fault {
			case "legacy pair":
				usage, diagnostics = map[string]any{"inputTokens": 7}, map[string]any{"hits": 17}
			case "wrong usage":
				usage["totalTokens"] = 1
			case "unknown usage":
				usage["unrecognized"] = true
			case "unknown diagnostic":
				diagnostics["unrecognized"] = true
			case "raw enum":
				diagnostics["providerAttemptTelemetryError"] = "PRIVATE_PROVIDER_FAILURE"
			case "object enum":
				diagnostics["providerAttemptTelemetryError"] = map[string]any{"message": "PRIVATE_PROVIDER_FAILURE"}
			case "aliased enum":
				delete(diagnostics, "providerAttemptTelemetryError")
				diagnostics["provider_attempt_telemetry_error"] = "invalid_or_incomplete"
			case "wrong schema":
				diagnostics["providerAttemptTelemetrySchema"] = "other"
			case "raw sibling":
				diagnostics["lastError"] = "PRIVATE_PROVIDER_FAILURE"
			case "credential":
				diagnostics["apiKey"] = "sk-test-private-value"
			case "reasoning":
				diagnostics["details"] = "<think>PRIVATE_REASONING</think>"
			case "nested diagnostics":
				diagnostics = map[string]any{"nested": diagnostics}
			}
			before, err := json.Marshal([]any{usage, diagnostics})
			if err != nil {
				t.Fatal(err)
			}
			view := ClosedTerminalTelemetryFailureValidationViewV1(usage, diagnostics)
			if fault == "healthy" {
				if _, present := view["providerAttemptTelemetryError"]; present || len(view) != len(diagnostics)-1 {
					t.Fatal("exact pair did not classify only its fixed error enum")
				}
				if ValidatePublicRecord(usage) != nil || ValidatePublicRecord(view) != nil || ValidatePublicRecord(diagnostics) == nil {
					t.Fatal("pair view changed the strict bare-diagnostic validation contract")
				}
				view["viewOnly"] = true
				if _, present := diagnostics["viewOnly"]; present {
					t.Fatal("pair view aliases the original diagnostic map")
				}
			} else if !reflect.DeepEqual(view, diagnostics) {
				t.Fatal("noncanonical or legacy pair acquired a telemetry exception")
			}
			after, err := json.Marshal([]any{usage, diagnostics})
			if err != nil || string(before) != string(after) {
				t.Fatal("pair validation changed the original payload")
			}
		})
	}
}

func TestValidatePublicRecordClosedAttemptTelemetryKeepsOriginalResourceBudget(t *testing.T) {
	for _, fault := range []string{"model string", "usage string", "diagnostic string", "diagnostic nodes", "diagnostic depth"} {
		t.Run(fault, func(t *testing.T) {
			telemetry := domainterminaltelemetry.NewTerminalTelemetryV1(domainmodel.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}, map[string]any{
				"providerAttemptTelemetrySchema": domainterminaltelemetry.ProviderAttemptTelemetrySchemaV1,
				"providerAttemptTelemetryValid":  false, "providerAttemptTelemetryError": "invalid_or_incomplete", "providerAttemptInventoryComplete": false,
			})
			usage, diagnostics := telemetry.PublicUsageMap(), telemetry.PublicCacheDiagnosticsMap()
			record := map[string]any{"kind": "usage", "threadId": "thread-budget", "turnId": "turn-budget", "model": "test-model", "timestamp": "2026-10-18T01:02:03Z", "usage": usage, "cacheDiagnostics": diagnostics}
			switch fault {
			case "model string":
				record["model"] = strings.Repeat("x", (1<<20)+1)
			case "usage string":
				usage["oversized"] = strings.Repeat("x", (1<<20)+1)
			case "diagnostic string":
				diagnostics["oversized"] = strings.Repeat("x", (1<<20)+1)
			case "diagnostic nodes":
				diagnostics["oversized"] = make([]any, 10_001)
			case "diagnostic depth":
				var nested any = "leaf"
				for depth := 0; depth < 65; depth++ {
					nested = map[string]any{"nested": nested}
				}
				diagnostics["oversized"] = nested
			}
			if err := ValidatePublicRecord(record); !errors.Is(err, domainsecret.ErrProjectionLimitV1) {
				t.Fatalf("original telemetry resource budget error = %v", err)
			}
		})
	}
}
