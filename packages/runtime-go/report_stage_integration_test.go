package runtimego

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func TestReportStageAuthorityPersistsAndFailsClosedAcrossRealStoreRestart(t *testing.T) {
	fixture := newReportStageIntegrationFixture(t)
	root := t.TempDir()
	authorityPath := filepath.Join(root, "authority", "continuation-ed25519-v1.json")
	authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := filepath.Join(root, "pending-work")
	store, err := newRuntimeTestPendingWorkStore(t, storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	service := pendingworkapp.NewService(authority, store, fixture)
	grant, pending := fixture.addGrant(t)
	request := pendingworkapp.ReportStageRequest{
		PendingToolCall: pending,
		StageInputHash:  domainsecurity.SHA256Hex([]byte("claim-ledger+snapshot+pii-projection")),
		IssuedAt:        fixture.now.Add(time.Second),
	}
	lease, err := service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	reopenedAuthority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := newRuntimeTestPendingWorkStore(t, storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	restarted := pendingworkapp.NewService(reopenedAuthority, reopenedStore, fixture)
	receipt, err := reopenedStore.ReadReceipt(context.Background(), lease.WorkID())
	if err != nil || receipt.Context.ContextDigest != fixture.securityContext.ContextDigest ||
		len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != grant.GrantID {
		t.Fatalf("restarted report stage lost persisted context/grant authority: receipt=%#v err=%v", receipt, err)
	}
	dispositions, err := restarted.CloseAllOpenOnRestart(context.Background(), fixture.now.Add(2*time.Minute))
	if err != nil || len(dispositions) != 1 ||
		dispositions[0].Status != domainpendingwork.StatusOutcomeUnknown ||
		dispositions[0].ReasonCode != "report_stage_outcome_unknown_after_restart" {
		t.Fatalf("restarted report stage did not fail closed: dispositions=%#v err=%v", dispositions, err)
	}
	if _, err := reopenedStore.ReadDisposition(context.Background(), lease.WorkID()); err != nil {
		t.Fatalf("outcome-unknown report stage disposition was not durable: %v", err)
	}
}

func TestCompletedReportStageDurableAuthorityContainsNoRawPIIOrReasoning(t *testing.T) {
	fixture := newReportStageIntegrationFixture(t)
	root := t.TempDir()
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(root, "authority", "continuation-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newRuntimeTestPendingWorkStore(t, filepath.Join(root, "pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	service := pendingworkapp.NewService(authority, store, fixture)
	_, pending := fixture.addGrant(t)
	const privateSentinel = "6222020202020202020_private_reasoning_sentinel"
	request := pendingworkapp.ReportStageRequest{
		PendingToolCall: pending,
		StageInputHash:  domainsecurity.SHA256Hex([]byte(privateSentinel)),
		IssuedAt:        fixture.now.Add(time.Second),
	}
	lease, err := service.BeginReportStage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	fixture.addResult(pending.ExecutionGrant, false, "private stage completed", fixture.now.Add(2*time.Second))
	disposition, err := service.CloseReportStageLease(context.Background(), lease, request, domainpendingwork.StatusCompleted, fixture.now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.ReadReceipt(context.Background(), lease.WorkID())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(struct {
		Receipt     domainpendingwork.PendingWorkReceiptV1     `json:"receipt"`
		Disposition domainpendingwork.PendingWorkDispositionV1 `json:"disposition"`
	}{Receipt: receipt, Disposition: disposition})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), privateSentinel) || strings.Contains(string(body), "6222020202020202020") {
		t.Fatalf("durable report-stage authority leaked raw PII/reasoning: %s", body)
	}
}

type reportStageIntegrationFixture struct {
	now             time.Time
	securityContext domainsecurity.TurnSecurityContext
	thread          map[string]any
	arguments       map[string]any
}

func newReportStageIntegrationFixture(t *testing.T) *reportStageIntegrationFixture {
	t.Helper()
	now := time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-report-stage", TurnID: "turn-report-stage", WorkspaceRealPath: t.TempDir(),
		CaseID: "case-report-stage", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")),
		ContextEpoch: 5, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := reportStageRecord(securityContext)
	return &reportStageIntegrationFixture{
		now: now, securityContext: securityContext, arguments: map[string]any{"stageInputHash": domainsecurity.SHA256Hex([]byte("stage"))},
		thread: map[string]any{
			"id": securityContext.ThreadID, "securityState": contextRecord,
			"turns": []any{map[string]any{
				"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "running",
				"securityContext": contextRecord, "items": []any{},
			}},
		},
	}
}

func (fixture *reportStageIntegrationFixture) GetThread(threadID string) (map[string]any, error) {
	if threadID != fixture.securityContext.ThreadID {
		return nil, errors.New("thread not found")
	}
	return fixture.thread, nil
}

func (fixture *reportStageIntegrationFixture) addGrant(t *testing.T) (domainsecurity.ExecutionGrant, appmodel.PendingToolCall) {
	t.Helper()
	arguments, _ := json.Marshal(fixture.arguments)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "host-report-stage", ServerIdentity: "host:builtin",
		ToolName: pendingworkapp.ReportStageToolName, ToolCallID: toolidentitytest.MustHostToolCallIDV1("report-stage"), ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("report-stage-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("report-stage-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: fixture.now, ExpiresAt: fixture.now.Add(20 * time.Minute),
	})
	fixture.appendItem(map[string]any{
		"id": "item-stage-call", "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": fixture.arguments, "createdAt": grant.IssuedAt,
		"contextDigest": fixture.securityContext.ContextDigest, "contextEpoch": float64(fixture.securityContext.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": reportStageRecord(grant),
	})
	return grant, appmodel.PendingToolCall{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID, ProviderID: grant.Provider,
		Call:            domainmodel.ToolCall{ID: grant.ToolCallID, Name: grant.ToolName, Arguments: arguments},
		SecurityContext: fixture.securityContext, ExecutionGrant: grant,
	}
}

func (fixture *reportStageIntegrationFixture) addResult(grant domainsecurity.ExecutionGrant, isError bool, output string, finishedAt time.Time) {
	stamp := finishedAt.UTC().Format(time.RFC3339Nano)
	fixture.appendItem(map[string]any{
		"id": "item-stage-result", "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "output": output, "isError": isError,
		"createdAt": stamp, "finishedAt": stamp, "contextDigest": fixture.securityContext.ContextDigest,
		"contextEpoch": float64(fixture.securityContext.ContextEpoch), "executionGrantId": grant.GrantID,
	})
}

func (fixture *reportStageIntegrationFixture) appendItem(item map[string]any) {
	turn := fixture.thread["turns"].([]any)[0].(map[string]any)
	turn["items"] = append(turn["items"].([]any), item)
}

func reportStageRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
