package pendingwork

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestObserveSuccessfulToolExecutionV1BindsExactForegroundBash(t *testing.T) {
	fixture := newGeneralToolExecutionObservationFixture(t)
	arguments := map[string]any{"command": "npm test -- acceptance.test.ts", "timeout": float64(120)}
	pending := addObservationGrantV1(t, fixture, "bash", arguments, false, fixture.now.Add(time.Second))
	identity := bashObservationIdentityV1(t, arguments)
	request := SideEffectIntentRequest{Pending: pending, IssuedAt: fixture.now.Add(2 * time.Second), SemanticIdentity: identity}
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	addObservationResultV1(fixture, pending.ExecutionGrant, false, fixture.now.Add(4*time.Second))
	if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(context.Background(), lease, request, fixture.now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}

	input := SuccessfulToolExecutionObservationInputV1{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		ToolName: "bash", ExpectedWorkspace: fixture.securityContext.WorkspaceRealPath,
		Arguments: pending.Call.Arguments, SemanticIdentity: identity,
	}
	observed, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), input)
	if err != nil || domainpendingwork.ValidateSuccessfulToolExecutionObservationV1(observed) != nil ||
		observed.ExecutionGrantID != pending.ExecutionGrant.GrantID || observed.ToolName != "bash" {
		t.Fatalf("exact bash execution was not observed: observation=%#v err=%v", observed, err)
	}
	body, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"npm test", fixture.securityContext.WorkspaceRealPath, "command", "arguments", "output"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("observation leaked private execution content %q: %s", forbidden, body)
		}
	}

	wrongArguments := map[string]any{"command": "npm test -- other.test.ts", "timeout": float64(120)}
	wrongArgumentBytes, _ := json.Marshal(wrongArguments)
	for name, mutate := range map[string]func(*SuccessfulToolExecutionObservationInputV1){
		"thread":    func(value *SuccessfulToolExecutionObservationInputV1) { value.ThreadID = "thread-other" },
		"turn":      func(value *SuccessfulToolExecutionObservationInputV1) { value.TurnID = "turn-other" },
		"workspace": func(value *SuccessfulToolExecutionObservationInputV1) { value.ExpectedWorkspace = "/workspace/other" },
		"arguments": func(value *SuccessfulToolExecutionObservationInputV1) {
			value.Arguments = wrongArgumentBytes
			value.SemanticIdentity = bashObservationIdentityV1(t, wrongArguments)
		},
	} {
		t.Run("rejects_wrong_"+name, func(t *testing.T) {
			changed := input
			mutate(&changed)
			if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), changed); !errors.Is(err, ErrToolExecutionNotObserved) {
				t.Fatalf("wrong %s retained observation authority: %v", name, err)
			}
		})
	}
}

func TestObserveSuccessfulToolExecutionV1BindsExactReadBatchMember(t *testing.T) {
	fixture := newGeneralToolExecutionObservationFixture(t)
	arguments := map[string]any{"path": "src/deeplaw/knowledge_store.py", "offset": float64(1), "limit": float64(200)}
	pending := addObservationGrantV1(t, fixture, "read", arguments, true, fixture.now.Add(time.Second))
	receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	addObservationResultV1(fixture, pending.ExecutionGrant, false, fixture.now.Add(3*time.Second))
	if _, err := fixture.service.CompleteToolBatch(
		context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending},
		domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(4*time.Second),
	); err != nil {
		t.Fatal(err)
	}

	input := SuccessfulToolExecutionObservationInputV1{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		ToolName: "read", ExpectedWorkspace: fixture.securityContext.WorkspaceRealPath,
		Arguments: pending.Call.Arguments, ReadPathResolved: true,
	}
	observed, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), input)
	if err != nil || observed.ToolName != "read" || observed.WorkID != receipt.WorkID ||
		observed.ExecutionGrantID != pending.ExecutionGrant.GrantID {
		t.Fatalf("exact read execution was not observed: observation=%#v err=%v", observed, err)
	}
	changed := input
	changed.ReadPathResolved = false
	if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), changed); !errors.Is(err, ErrToolExecutionObservationInvalid) {
		t.Fatalf("unresolved read path retained observation authority: %v", err)
	}
	changed = input
	changed.Arguments = json.RawMessage(`{"path":"src/deeplaw/knowledge_store.py","offset":2,"limit":200}`)
	if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), changed); !errors.Is(err, ErrToolExecutionNotObserved) {
		t.Fatalf("changed read window retained exact execution authority: %v", err)
	}
	changed = input
	changed.Arguments = json.RawMessage(`{"path":"README.md","offset":1,"limit":200}`)
	if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), changed); !errors.Is(err, ErrToolExecutionNotObserved) {
		t.Fatalf("a different valid read inherited the observed grant: %v", err)
	}
	for _, unsafe := range []string{"../outside.txt", "/absolute.txt", `dir\\file.txt`, "."} {
		changed = input
		body, _ := json.Marshal(map[string]any{"path": unsafe})
		changed.Arguments = body
		if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), changed); !errors.Is(err, ErrToolExecutionObservationInvalid) {
			t.Fatalf("unsafe read path %q entered release evidence: %v", unsafe, err)
		}
	}
}

func TestObserveSuccessfulToolExecutionV1AdmitsOrdinaryReadInHostPolicyBoundary(t *testing.T) {
	fixture := newHostPolicyBoundaryToolExecutionObservationFixture(t)
	if fixture.securityContext.RiskAuthorityBinding.State != domainsecurity.RiskAuthorityBindingStateHostPolicy ||
		!domainsecurity.TurnSecurityContextIsBoundaryOnly(fixture.securityContext) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(fixture.securityContext) == nil ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(fixture.securityContext) != nil ||
		domainsecurity.TurnSecurityContextAllowsCaseEvidence(fixture.securityContext) {
		t.Fatalf("host-policy boundary test context widened authority: %#v", fixture.securityContext)
	}
	arguments := map[string]any{"path": "docs/acceptance-context.txt", "limit": float64(3912)}
	pending := addObservationGrantV1(t, fixture, "read", arguments, true, fixture.now.Add(time.Second))
	receipt, err := fixture.service.IssueToolBatch(context.Background(), []appmodel.PendingToolCall{pending}, fixture.now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	addObservationResultV1(fixture, pending.ExecutionGrant, false, fixture.now.Add(3*time.Second))
	if _, err := fixture.service.CompleteToolBatch(
		context.Background(), receipt.WorkID, fixture.securityContext, []appmodel.PendingToolCall{pending},
		domainpendingwork.StatusCompleted, "batch_completed", fixture.now.Add(4*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	input := SuccessfulToolExecutionObservationInputV1{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID, ToolName: "read",
		ExpectedWorkspace: fixture.securityContext.WorkspaceRealPath, Arguments: pending.Call.Arguments, ReadPathResolved: true,
	}
	observed, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), input)
	if err != nil || domainpendingwork.ValidateSuccessfulToolExecutionObservationV1(observed) != nil ||
		observed.ExecutionGrantID != pending.ExecutionGrant.GrantID || observed.ToolName != "read" {
		t.Fatalf("ordinary host-policy boundary read was not observed: observation=%#v err=%v", observed, err)
	}
	body, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"acceptance-context.txt", fixture.securityContext.WorkspaceRealPath, "path", "arguments", "output"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("case-thread observation leaked private execution content %q: %s", forbidden, body)
		}
	}

	quarantined, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: fixture.securityContext.WorkspaceRealPath,
		TenantID:          fixture.securityContext.TenantID, UserID: fixture.securityContext.UserID,
		CaseID: fixture.securityContext.CaseID, CaseBindingHash: fixture.securityContext.CaseBindingHash,
		DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID, SourceManifestHash: fixture.securityContext.SourceManifestHash,
		ContextEpoch: fixture.securityContext.ContextEpoch, IssuedAt: fixture.now,
		PublicationPolicy:    fixture.securityContext.PublicationPolicy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	thread := fixture.threads.threads[fixture.securityContext.ThreadID]
	turn, found := appmodel.TurnByID(thread, fixture.securityContext.TurnID)
	if !found {
		t.Fatal("host-policy boundary turn disappeared")
	}
	thread["securityState"] = mapRecord(quarantined)
	turn["securityContext"] = mapRecord(quarantined)
	if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), input); !errors.Is(err, ErrToolExecutionNotObserved) {
		t.Fatalf("quarantined boundary retained ordinary execution observation authority: %v", err)
	}
}

func TestObserveSuccessfulToolExecutionV1FailsClosedForErrorForgeryAndDuplicate(t *testing.T) {
	t.Run("settled error", func(t *testing.T) {
		fixture := newGeneralToolExecutionObservationFixture(t)
		arguments := map[string]any{"command": "exit 7"}
		pending := addObservationGrantV1(t, fixture, "bash", arguments, false, fixture.now.Add(time.Second))
		identity := bashObservationIdentityV1(t, arguments)
		completeObservationSideEffectV1(t, fixture, pending, identity, true)
		_, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), SuccessfulToolExecutionObservationInputV1{
			ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID, ToolName: "bash",
			ExpectedWorkspace: fixture.securityContext.WorkspaceRealPath, Arguments: pending.Call.Arguments, SemanticIdentity: identity,
		})
		if !errors.Is(err, ErrToolExecutionNotObserved) {
			t.Fatalf("settled error became successful execution evidence: %v", err)
		}
	})

	t.Run("forged receipt", func(t *testing.T) {
		fixture, input := successfulObservationFixtureV1(t)
		fixture.store.mu.Lock()
		for workID, receipt := range fixture.store.receipts {
			receipt.PayloadHash = domainsecurity.SHA256Hex([]byte("forged payload"))
			fixture.store.receipts[workID] = receipt
			break
		}
		fixture.store.mu.Unlock()
		if _, err := fixture.service.ObserveSuccessfulToolExecutionV1(context.Background(), input); err == nil || errors.Is(err, ErrToolExecutionNotObserved) {
			t.Fatalf("forged receipt did not fail the trusted inventory closed: %v", err)
		}
	})

	t.Run("duplicate inventory", func(t *testing.T) {
		fixture, input := successfulObservationFixtureV1(t)
		duplicated := &duplicateObservationInventoryStoreV1{Store: fixture.store}
		service := NewService(fixture.authority, duplicated, fixture.threads)
		if _, err := service.ObserveSuccessfulToolExecutionV1(context.Background(), input); err == nil || errors.Is(err, ErrToolExecutionNotObserved) {
			t.Fatalf("duplicate trusted receipt became an observation match: %v", err)
		}
	})
}

func newGeneralToolExecutionObservationFixture(t *testing.T) *serviceFixture {
	fixture := newServiceFixture(t)
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: "/workspace/general-observation", ContextEpoch: fixture.securityContext.ContextEpoch,
		IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.installContext(securityContext)
	return fixture
}

func newHostPolicyBoundaryToolExecutionObservationFixture(t *testing.T) *serviceFixture {
	t.Helper()
	fixture := newServiceFixture(t)
	workspace := "/workspace/case-boundary-observation"
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace,
		State:             domainsecurity.CaseBindingStateMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{67}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	policy, err := domainsecurity.NewThreadRiskPolicyV1(domainsecurity.ThreadRiskPolicyInputV1{
		ThreadID: fixture.securityContext.ThreadID, WorkspaceRealPath: workspace,
		RiskClass: domainsecurity.RiskClassCase, Origin: domainsecurity.RiskPolicyOriginProtectedDataGuard,
		SignalsDigest: domainsecurity.SHA256Hex([]byte("host-policy-boundary-observation")),
		IssuedAt:      fixture.now, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainsecurity.NewHostPolicyRiskAuthorityBindingV1(policy, observation)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policy.PolicyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		WorkspaceRealPath: workspace,
		TenantID:          domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: fixture.securityContext.ContextEpoch, IssuedAt: fixture.now,
		PublicationPolicy: publication, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.installContext(securityContext)
	return fixture
}

func addObservationGrantV1(
	t *testing.T,
	fixture *serviceFixture,
	toolName string,
	arguments map[string]any,
	readOnly bool,
	issuedAt time.Time,
) appmodel.PendingToolCall {
	t.Helper()
	argumentBytes, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	callID := pendingWorkTestToolCallID("observe-" + toolName + "-" + domainsecurity.SHA256Hex(argumentBytes)[:12])
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.securityContext, Provider: "provider-test", ServerIdentity: "host:builtin",
		ToolName: toolName, ToolCallID: callID, ArgsHash: domainsecurity.CanonicalJSONHash(argumentBytes),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema:" + toolName)),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("scope:" + toolName)),
		ReadOnly:   readOnly, ApprovalState: "not_required", IssuedAt: issuedAt, ExpiresAt: fixture.now.Add(20 * time.Minute),
	})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		t.Fatal(err)
	}
	fixture.appendItem(map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(fixture.securityContext.TurnID, callID), "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": fixture.securityContext.ThreadID, "turnId": fixture.securityContext.TurnID,
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": arguments, "createdAt": grant.IssuedAt,
		"contextDigest": fixture.securityContext.ContextDigest, "contextEpoch": float64(fixture.securityContext.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": mapRecord(grant),
	})
	fixture.grantArguments[grant.GrantID] = arguments
	return appmodel.PendingToolCall{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID,
		ProviderID: grant.Provider, Call: domainmodel.ToolCall{ID: callID, Name: toolName, Arguments: argumentBytes},
		SecurityContext: fixture.securityContext, ExecutionGrant: grant,
	}
}

func bashObservationIdentityV1(t *testing.T, arguments map[string]any) domainsideeffectidentity.IdentityV1 {
	t.Helper()
	command, _ := arguments["command"].(string)
	timeout := 120
	if value, ok := arguments["timeout"].(float64); ok && value > 0 {
		timeout = int(value)
		if timeout > 120 {
			timeout = 120
		}
	}
	projection := struct {
		SchemaVersion int            `json:"schemaVersion"`
		ToolName      string         `json:"toolName"`
		Arguments     map[string]any `json:"arguments"`
	}{
		SchemaVersion: domainsideeffectidentity.SchemaVersionV1,
		ToolName:      "bash",
		Arguments: map[string]any{
			"command": command, "timeoutSeconds": timeout, "runInBackground": false,
		},
	}
	body, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	identity := domainsideeffectidentity.IdentityV1{
		SchemaVersion: domainsideeffectidentity.SchemaVersionV1,
		ToolName:      "bash",
		ArgsHash:      domainsecurity.CanonicalJSONHash(body),
	}
	if err := domainsideeffectidentity.ValidateV1(identity); err != nil {
		t.Fatal(err)
	}
	return identity
}

func addObservationResultV1(fixture *serviceFixture, grant domainsecurity.ExecutionGrant, isError bool, finishedAt time.Time) {
	status, messageKey, code := "completed", "tool_completed", "tool_completed"
	if isError {
		status, messageKey, code = "failed", "tool_failed", "tool_failed"
	}
	item := fixture.addResult(grant, isError, "raw output must be replaced", finishedAt)
	item["status"] = status
	item["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:  domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind: domaintoolresult.ProjectionHostStatus,
		Disclosure:     domaintoolresult.MetadataOnlyDisclosure, MessageKey: messageKey, Status: status, Code: code,
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
	})
}

func completeObservationSideEffectV1(
	t *testing.T,
	fixture *serviceFixture,
	pending appmodel.PendingToolCall,
	identity domainsideeffectidentity.IdentityV1,
	isError bool,
) {
	t.Helper()
	request := SideEffectIntentRequest{Pending: pending, IssuedAt: fixture.now.Add(2 * time.Second), SemanticIdentity: identity}
	lease, err := fixture.service.BeginSideEffectIntent(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifySideEffectIntentAtSend(context.Background(), lease, request, fixture.now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	addObservationResultV1(fixture, pending.ExecutionGrant, isError, fixture.now.Add(4*time.Second))
	if _, err := fixture.service.CloseSideEffectIntentAfterSettlement(context.Background(), lease, request, fixture.now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
}

func successfulObservationFixtureV1(t *testing.T) (*serviceFixture, SuccessfulToolExecutionObservationInputV1) {
	fixture := newGeneralToolExecutionObservationFixture(t)
	arguments := map[string]any{"command": "printf observed"}
	pending := addObservationGrantV1(t, fixture, "bash", arguments, false, fixture.now.Add(time.Second))
	identity := bashObservationIdentityV1(t, arguments)
	completeObservationSideEffectV1(t, fixture, pending, identity, false)
	return fixture, SuccessfulToolExecutionObservationInputV1{
		ThreadID: fixture.securityContext.ThreadID, TurnID: fixture.securityContext.TurnID, ToolName: "bash",
		ExpectedWorkspace: fixture.securityContext.WorkspaceRealPath, Arguments: pending.Call.Arguments, SemanticIdentity: identity,
	}
}

type duplicateObservationInventoryStoreV1 struct {
	pendingworkstoreport.Store
}

func (store *duplicateObservationInventoryStoreV1) SnapshotInventory(
	ctx context.Context,
) ([]domainpendingwork.PendingWorkReceiptV1, []domainpendingwork.PendingWorkDispositionV1, error) {
	receipts, dispositions, err := store.Store.SnapshotInventory(ctx)
	if err == nil && len(receipts) != 0 {
		receipts = append(receipts, receipts[0])
	}
	return receipts, dispositions, err
}
