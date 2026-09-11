package server

import (
	"context"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSteerAdmissionKeepsOrdinaryBaseAfterFrozenCaseDatasetInvalidation(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "steer-additive-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "steer-additive-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerCaseExecution(t, handler, workspace)

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "additive steer authority", "workspace": workspace,
		"providerId": "steer-additive-provider", "model": "steer-additive-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-steer-additive-authority"
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Thread: thread,
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC),
	})
	if err != nil || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("case steer fixture is invalid: context=%#v err=%v", securityContext, err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start case work",
		"steering": []any{}, "items": []any{}, "createdAt": "2026-07-27T10:00:00Z", "startedAt": "2026-07-27T10:00:00Z",
		"securityContext": securityRecord,
	}, "steer-additive-provider", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}

	snapshot, ok := handler.turnSecurity.SnapshotAuthorityV2.(*admissionFailureSnapshotAuthority)
	if !ok {
		t.Fatalf("unexpected snapshot authority fixture: %T", handler.turnSecurity.SnapshotAuthorityV2)
	}
	snapshot.resolved.Record.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("invalidated-steer-snapshot")
	strictCurrent := turnsecurityapp.CurrentValidationInput{
		OperationContext: context.Background(), Identity: handler.turnSecurity.Identity,
		Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
		SnapshotAuthority: handler.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: handler.turnSecurity.SnapshotAuthorityV2,
		Context: securityContext, Workspace: securityContext.WorkspaceRealPath,
	}
	if err := turnsecurityapp.ValidateCurrentForEffect(strictCurrent, true); err == nil {
		t.Fatal("invalidated DSV2 still authorized a protected case-data effect")
	}
	if err := turnsecurityapp.ValidateCurrentForEffect(strictCurrent, false); err != nil {
		t.Fatalf("invalidated DSV2 disabled the ordinary effect base: %v", err)
	}
	snapshotCallsBeforeSteer := snapshot.calls.Load()

	requests := []struct {
		clientID     string
		text         string
		wantEffect   domainsecurity.LogicalEffect
		wantOrdinary bool
	}{
		{
			clientID:   "018f47a0-13d2-4a9c-8f51-4ae4f62e8b10",
			text:       "修改当前源码中的注释并运行普通单元测试。",
			wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true,
		},
		{
			clientID:   "018f47a0-13d2-4b9c-9f51-4ae4f62e8b11",
			text:       "查询当前案件账户在指定期间的流入、流出、净额和交易笔数。",
			wantEffect: domainsecurity.LogicalEffectFundsData,
		},
	}
	for _, request := range requests {
		result, steerErr := handler.steerRuntimeTurn(context.Background(), controlapp.SteerTurnRequest{
			ThreadID: threadID, TurnID: turnID, ExpectedTurnID: turnID,
			ClientUserMessageID: request.clientID, Text: request.text,
		})
		if steerErr != nil || result.StatusCode != 200 || result.Body["ok"] != true ||
			stringField(result.Body, "clientUserMessageId") != request.clientID {
			t.Fatalf("steer admission failed after DSV2 invalidation: result=%#v err=%v", result, steerErr)
		}
	}
	if calls := snapshot.calls.Load(); calls != snapshotCallsBeforeSteer {
		t.Fatalf("ordinary steering persistence unexpectedly required live DSV2: calls=%d want=%d", calls, snapshotCallsBeforeSteer)
	}

	entries, err := handler.store.PendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(entries) != len(requests) {
		t.Fatalf("pending steering entries mismatch: entries=%#v err=%v", entries, err)
	}
	for index, request := range requests {
		entry := entries[index]
		if stringField(entry, "clientUserMessageId") != request.clientID || stringField(entry, "text") != request.text ||
			stringField(entry, "logicalEffect") != string(request.wantEffect) || entry["ordinaryWork"] != request.wantOrdinary {
			t.Fatalf("steering effect binding mismatch at %d: entry=%#v", index, entry)
		}
		if _, err := domainsteering.AdmissionAuthorityMaterialV1(entry, securityContext.ContextDigest); err != nil {
			t.Fatalf("steering entry %d lost signed admission authority: %v", index, err)
		}
	}
}
