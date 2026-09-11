package event

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
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
