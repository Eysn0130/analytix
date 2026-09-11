package turn

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type failureStoreStub struct {
	changed        bool
	finishErr      error
	recordErr      error
	status         string
	failedItems    []map[string]any
	terminalFields map[string]any
	finishCalls    int
	publishCalls   int
}

func (stub *failureStoreStub) FinishTurnIfActiveWithItemsAndFields(_, _ string, status string, items []map[string]any, fields map[string]any) (bool, string, error) {
	stub.finishCalls++
	stub.status = status
	if stub.finishErr != nil {
		return false, "", stub.finishErr
	}
	if stub.changed {
		stub.failedItems = items
		stub.terminalFields = fields
	}
	return stub.changed, status, nil
}

func (stub *failureStoreStub) RecordGeneralTerminalEventBundle(_, _ string) ([]map[string]any, error) {
	stub.publishCalls++
	return nil, stub.recordErr
}

func TestPersistFailureCommitsRecoverableHostTerminalOutbox(t *testing.T) {
	securityContext := failureGeneralContext(t)
	store := &failureStoreStub{changed: true}
	err := PersistFailure(PersistFailureInput{
		Store: store, SecurityContext: securityContext, TerminalReason: "provider_failure",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Failure:    domainfailure.New(domainfailure.CodeProviderAuthenticationFailed, nil),
		FinishedAt: "2026-01-02T03:04:05Z", Model: "gpt-5",
		Result: domainmodel.Result{Usage: domainmodel.Usage{
			PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5,
		}},
		CacheDiagnostics: map[string]any{"outputPreview": "RAW_FAILURE_DIAGNOSTIC_SENTINEL"},
		UsageSource:      "subagent", ChildRunID: "job-1",
		Events: []map[string]any{{
			"kind": "assistant_text_delta", "threadId": securityContext.ThreadID,
			"turnId": securityContext.TurnID, "item": map[string]any{"text": "partial"},
		}},
	})
	if err != nil {
		t.Fatalf("persist failure: %v", err)
	}
	encodedItems, _ := json.Marshal(store.failedItems)
	if store.status != "failed" || len(store.failedItems) != 1 || strings.Contains(string(encodedItems), "partial") {
		t.Fatalf("expected only the fixed host error item, got status=%s items=%s", store.status, encodedItems)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(store.terminalFields["generalTerminalPublication"])
	if err != nil || commit.TerminalStatus != "failed" || commit.TerminalReason != "provider_failure" ||
		len(commit.Events) != 3 || commit.Events[0].Slot != "terminal-item" || commit.Events[1].Slot != "usage" ||
		commit.Events[2].Slot != "terminal" || store.publishCalls != 1 {
		t.Fatalf("failure outbox mismatch: commit=%#v calls=%d err=%v", commit, store.publishCalls, err)
	}
	if commit.UsageEvent["usageSource"] != "subagent" || commit.UsageEvent["childRunId"] != "job-1" ||
		commit.UsageEvent["usageFinalStatus"] != "failed" {
		t.Fatalf("usage payload mismatch: %#v", commit.UsageEvent)
	}
	encodedUsage, _ := json.Marshal(commit.UsageEvent)
	if strings.Contains(string(encodedUsage), "RAW_FAILURE_DIAGNOSTIC_SENTINEL") ||
		!strings.Contains(string(encodedUsage), `"terminalCacheDiagnosticsValid":false`) {
		t.Fatalf("failure outbox retained open cache diagnostics: %s", encodedUsage)
	}
	encodedTerminal, _ := json.Marshal(commit.TerminalEvent)
	expected := domainfailure.New(domainfailure.CodeProviderAuthenticationFailed, nil)
	if commit.TerminalEvent["code"] != expected.Code() ||
		commit.TerminalEvent["message"] != expected.Message() ||
		commit.TerminalEvent["severity"] != expected.Severity() ||
		strings.Contains(string(encodedTerminal), `"details"`) ||
		strings.Contains(string(encodedTerminal), "RAW_FAILURE_DIAGNOSTIC_SENTINEL") {
		t.Fatalf("turn_failed payload mismatch: %#v", commit.TerminalEvent)
	}
}

func TestPersistFailureReconcilesCanonicalPublicationWhenTurnAlreadyTerminal(t *testing.T) {
	securityContext := failureGeneralContext(t)
	store := &failureStoreStub{changed: true, recordErr: errors.New("event gap")}
	input := PersistFailureInput{
		Store: store, SecurityContext: securityContext, TerminalReason: "provider_failure",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, FinishedAt: "2026-01-02T03:04:05Z",
	}
	if err := PersistFailure(input); err == nil || store.publishCalls != 1 || store.terminalFields == nil {
		t.Fatalf("first CAS/event gap mismatch: calls=%d fields=%#v err=%v", store.publishCalls, store.terminalFields, err)
	}
	store.changed = false
	store.recordErr = nil
	if err := PersistFailure(input); err != nil {
		t.Fatalf("reconcile already terminal failure: %v", err)
	}
	if store.publishCalls != 2 {
		t.Fatalf("unchanged canonical turn should reconcile its terminal bundle: %d", store.publishCalls)
	}
}

func TestPersistFailurePropagatesAtomicCommitAndOutboxErrors(t *testing.T) {
	securityContext := failureGeneralContext(t)
	base := PersistFailureInput{
		SecurityContext: securityContext, TerminalReason: "provider_failure",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, FinishedAt: "2026-01-02T03:04:05Z",
	}
	finishErr := errors.New("finish failed")
	input := base
	input.Store = &failureStoreStub{finishErr: finishErr}
	if err := PersistFailure(input); !errors.Is(err, finishErr) {
		t.Fatalf("expected finish error, got %v", err)
	}
	recordErr := errors.New("record failed")
	input = base
	input.Store = &failureStoreStub{changed: true, recordErr: recordErr}
	if err := PersistFailure(input); !errors.Is(err, recordErr) {
		t.Fatalf("expected recoverable outbox publish error, got %v", err)
	}
}

func TestPersistFailureUsesClosedReasonStatusMapping(t *testing.T) {
	securityContext := failureGeneralContext(t)
	store := &failureStoreStub{changed: true}
	if err := PersistFailure(PersistFailureInput{
		Store: store, SecurityContext: securityContext, TerminalReason: "source_unavailable",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Failure: domainfailure.New("source_probe_unavailable", nil), FinishedAt: "2026-01-02T03:04:05Z",
	}); err != nil {
		t.Fatal(err)
	}
	if store.status != "completed" || stringField(store.failedItems[0], "kind") != "assistant_text" {
		t.Fatalf("source boundary was not host-completed: status=%s items=%#v", store.status, store.failedItems)
	}
	if got := stringField(store.failedItems[0], "text"); got != GeneralProviderFinalQuarantinedText {
		t.Fatalf("source boundary did not use the closed host text: %q", got)
	}
	invalid := PersistFailureInput{
		Store: &failureStoreStub{}, SecurityContext: securityContext, TerminalReason: "invented",
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, FinishedAt: "2026-01-02T03:04:05Z",
	}
	if err := PersistFailure(invalid); err == nil {
		t.Fatal("unknown terminal reason was accepted")
	}
}

func TestPersistFailurePreservesClosedToolCauseWithoutStormInflation(t *testing.T) {
	securityContext := failureGeneralContext(t)
	tests := []struct {
		name     string
		failure  domainfailure.Record
		wantCode string
	}{
		{
			name:     "provider tool arguments invalid",
			failure:  domainfailure.New(domainfailure.CodeProviderToolArgumentsInvalid, map[string]any{"unsafe": "TOOL_FAILURE_PRIVATE_SENTINEL"}),
			wantCode: domainfailure.CodeProviderToolArgumentsInvalid,
		},
		{
			name: "invalid arguments storm",
			failure: domainfailure.New("tool_invalid_arguments_storm", map[string]any{
				"stormCount": 3, "unsafe": "TOOL_FAILURE_PRIVATE_SENTINEL",
			}),
			wantCode: "tool_invalid_arguments_storm",
		},
		{
			name: "actual failure storm",
			failure: domainfailure.New("tool_failure_storm", map[string]any{
				"stormCount": 6, "unsafe": "TOOL_FAILURE_PRIVATE_SENTINEL",
			}),
			wantCode: "tool_failure_storm",
		},
		{
			name:     "immediate advertised-tool rejection",
			failure:  domainfailure.New("tool_not_advertised", map[string]any{"unsafe": "TOOL_FAILURE_PRIVATE_SENTINEL"}),
			wantCode: "tool_not_advertised",
		},
		{
			name:     "incompatible cause",
			failure:  domainfailure.New(domainfailure.CodeProviderError, map[string]any{"status": 500}),
			wantCode: domainfailure.CodeTurnFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &failureStoreStub{changed: true}
			if err := PersistFailure(PersistFailureInput{
				Store: store, SecurityContext: securityContext, TerminalReason: "tool_failure",
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
				Failure: test.failure, FinishedAt: "2026-01-02T03:04:05Z",
			}); err != nil {
				t.Fatal(err)
			}
			commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(
				store.terminalFields["generalTerminalPublication"],
			)
			if err != nil || len(store.failedItems) != 1 ||
				stringField(store.failedItems[0], "code") != test.wantCode ||
				stringField(commit.TerminalEvent, "code") != test.wantCode ||
				stringField(store.failedItems[0], "message") != domainfailure.New(test.wantCode, nil).Message() {
				t.Fatalf("tool terminal cause mismatch: items=%#v commit=%#v err=%v", store.failedItems, commit, err)
			}
			encoded, _ := json.Marshal(map[string]any{"items": store.failedItems, "commit": commit})
			if strings.Contains(string(encoded), "TOOL_FAILURE_PRIVATE_SENTINEL") ||
				strings.Contains(string(encoded), `"details"`) {
				t.Fatalf("tool terminal retained cause details: %s", encoded)
			}
		})
	}
}

func TestCommitGeneralFailureTerminalRejectsInvalidInterruptMetadataBeforeCAS(t *testing.T) {
	securityContext := failureGeneralContext(t)
	tests := []struct {
		name      string
		reason    string
		interrupt *GeneralTerminalInterruptMetadata
	}{
		{name: "cancel missing metadata", reason: "cancel"},
		{name: "non-cancel carries metadata", reason: "provider_failure", interrupt: &GeneralTerminalInterruptMetadata{Cancelled: true}},
		{name: "negative pending count", reason: "cancel", interrupt: &GeneralTerminalInterruptMetadata{Cancelled: true, CancelledPendingGates: -1}},
		{name: "pending gates without cancellation", reason: "cancel", interrupt: &GeneralTerminalInterruptMetadata{CancelledPendingGates: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &failureStoreStub{changed: true}
			_, err := CommitGeneralFailureTerminal(PersistFailureInput{
				Store: store, SecurityContext: securityContext, TerminalReason: test.reason,
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
				FinishedAt: "2026-01-02T03:04:05Z", Interrupt: test.interrupt,
			})
			if err == nil || store.finishCalls != 0 || store.publishCalls != 0 {
				t.Fatalf("invalid interrupt metadata reached CAS: finish=%d publish=%d err=%v", store.finishCalls, store.publishCalls, err)
			}
		})
	}
}

func failureGeneralContext(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace",
		ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
