package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type caseSourcePublicSeamFixture struct {
	handler  *runtimeServerHandler
	threadID string
	turnID   string
	itemID   string
	proof    domaintoolresult.CaseSourceBindingProofV1
}

func TestCaseSourceStatusRealCaseHTTPAndSSERemainBoundaryOnlyAndPrivate(t *testing.T) {
	fixture := newCaseSourcePublicSeamFixture(t)
	assertCaseSourcePublicSeams(t, fixture, true)

	mutations := map[string]func(map[string]any){
		"missing proof":  func(item map[string]any) { delete(item, domaintoolresult.CaseSourceBindingProofFieldV1) },
		"tool rebound":   func(item map[string]any) { item["toolName"] = "mcp__analytix-fund-analysis__other" },
		"call rebound":   func(item map[string]any) { item["callId"] = serverTestHostToolCallID("case-source-rebound") },
		"digest rebound": func(item map[string]any) { item["contextDigest"] = domainsecurity.SHA256Hex([]byte("rebound-context")) },
		"epoch rebound":  func(item map[string]any) { item["contextEpoch"] = float64(8) },
		"grant rebound": func(item map[string]any) {
			item["executionGrantId"] = domainsecurity.SHA256Hex([]byte("rebound-grant"))
		},
		"status rebound": func(item map[string]any) { item["status"] = "failed" },
		"error rebound":  func(item map[string]any) { item["isError"] = true },
		"proof rebound": func(item map[string]any) {
			item[domaintoolresult.CaseSourceBindingProofFieldV1].(map[string]any)["digest"] = domainsecurity.SHA256Hex([]byte("rebound-proof"))
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := newCaseSourcePublicSeamFixture(t)
			mutateCaseSourceDurableItem(t, fixture, mutate)
			assertCaseSourcePublicSeams(t, fixture, false)
		})
	}
}

func newCaseSourcePublicSeamFixture(t *testing.T) caseSourcePublicSeamFixture {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{"title": "closed case source", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-case-source-public"
	now := time.Now().UTC().Truncate(time.Second)
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		CaseID: "case-source-public", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-source-public-binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("case-source-public-manifest")),
		ContextEpoch:       7, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "items": []any{},
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"securityContext": securityRecord,
	}, "provider", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	toolName := "mcp__analytix-fund-analysis__query_transactions"
	call := domainmodel.ToolCall{
		ID: serverTestHostToolCallID("case-source-http-sse"), Name: toolName,
		Arguments: json.RawMessage(`{"account":"PRIVATE_REVERSE_MAP_SENTINEL"}`),
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix-fund-analysis", "analytix-fund-analysis", "1.0.0",
		domainsecurity.SHA256Hex([]byte("case-source-public-instance")), 3,
	)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: serverIdentity,
		ToolName: call.Name, ToolCallID: call.ID, ConnectionEpoch: 3,
		ArgsHash:   domainsecurity.CanonicalJSONHash(call.Arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("case-source-public-schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("case-source-public-scope")),
		ReadOnly:   true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	})
	toolCallItemID := domaintoolcall.ToolCallItemIDV1(turnID, call.ID)
	callItem, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: toolCallItemID,
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: toolcatalogapp.ToolKind(call.Name),
		Context: securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.store.AppendItemToTurn(threadID, turnID, callItem); err != nil {
		t.Fatal(err)
	}
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: call.Name, ToolCallID: call.ID, ContextDigest: securityContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess,
		Data: map[string]any{
			"account": "6222020202020202020", "AuthorityRef": "PRIVATE_AUTHORITY_REF",
			"path": "/private/case.sqlite", "sql": "SELECT * FROM private_case", "providerBody": "PRIVATE_PROVIDER_BODY",
		},
		IssuedAt: now.Add(time.Second),
	})
	settlementOutput := map[string]any{
		"code": "case_source_result_private", "executed": true, "isError": false,
		"toolOutcome": domainevidence.ToolOutcomeRecord(outcome),
	}
	projection := toolcatalogapp.BuildPublicToolResultProjectionV1(call.Name, settlementOutput, false)
	pending := runtimePendingToolCall{
		ThreadID: threadID, TurnID: turnID, ProviderID: "provider", Workspace: workspace,
		Call: call, ToolCallItemID: toolCallItemID, SecurityContext: securityContext, ExecutionGrant: grant,
	}
	if err := handler.persistRuntimeToolResult(pending, projection, evidenceapp.PreparedToolSettlement{
		Output: settlementOutput, IsError: false,
	}); err != nil {
		t.Fatal(err)
	}
	itemID := toolcatalogapp.ToolResultItemID(turnID, call.ID)
	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	rawItem := caseSourceResultItem(t, rawThread, turnID, itemID)
	proof, err := domaintoolresult.ParseCaseSourceBindingProofV1(rawItem[domaintoolresult.CaseSourceBindingProofFieldV1])
	if err != nil {
		t.Fatalf("actual settled durable item is missing its private proof: %v", err)
	}
	canonical, ok := domaintoolresult.PrivateDurableToolResultItemRecordV1(rawItem)
	if !ok || canonical[domaintoolresult.CaseSourceBindingProofFieldV1] == nil {
		t.Fatalf("actual settled server item is not canonical private durable state: %#v", rawItem)
	}
	rawBody, _ := json.Marshal([]any{rawItem, canonical})
	for _, forbidden := range []string{
		"6222020202020202020", "PRIVATE_AUTHORITY_REF", "/private/case.sqlite", "SELECT * FROM private_case",
		"PRIVATE_PROVIDER_BODY", "PRIVATE_REVERSE_MAP_SENTINEL", "toolOutcome", "datasetSnapshotId", "serverIdentity",
	} {
		if strings.Contains(string(rawBody), forbidden) {
			t.Fatalf("private durable root retained process-local outcome data %q: %s", forbidden, rawBody)
		}
	}
	return caseSourcePublicSeamFixture{handler: handler, threadID: threadID, turnID: turnID, itemID: itemID, proof: proof}
}

func mutateCaseSourceDurableItem(t *testing.T, fixture caseSourcePublicSeamFixture, mutate func(map[string]any)) {
	t.Helper()
	path := fixture.handler.store.threadPath(fixture.threadID)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{}
	if err := json.Unmarshal(body, &thread); err != nil {
		t.Fatal(err)
	}
	item := caseSourceResultItem(t, thread, fixture.turnID, fixture.itemID)
	mutate(item)
	body, err = json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	if err := filestore.WritePrivateFileAtomic(path, body); err != nil {
		t.Fatal(err)
	}
}

func caseSourceResultItem(t *testing.T, thread map[string]any, turnID, itemID string) map[string]any {
	t.Helper()
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == itemID {
				return item
			}
		}
	}
	t.Fatalf("case-source result %s is missing", itemID)
	return nil
}

func assertCaseSourcePublicSeams(t *testing.T, fixture caseSourcePublicSeamFixture, canonical bool) {
	t.Helper()
	server := httptest.NewServer(fixture.handler)
	defer server.Close()
	detail := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+fixture.threadID, nil, http.StatusOK)
	detailBody, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	sseBody := requestRuntimeSSEText(t, server.URL+"/v1/threads/"+fixture.threadID+"/events?since_seq=0")
	for name, body := range map[string]string{"http": string(detailBody), "sse": sseBody} {
		if strings.Contains(body, "case_source_status") {
			if canonical {
				t.Fatalf("%s exposed a case turn excluded by case_boundary_only history: %s", name, body)
			}
			t.Fatalf("%s rebound durable item upgraded into a visible case-source status: %s", name, body)
		}
		// HTTP detail and SSE replay share the accepted case_boundary_only
		// projection. A real case settlement is therefore safely omitted here;
		// exact status preservation is proved at the item/event projector seams.
		for _, forbidden := range []string{
			"caseOutcome", domaintoolresult.CaseSourceBindingProofFieldV1, domaintoolresult.CaseSourceBindingProofPurposeV1,
			fixture.proof.Digest, "case_source_private", "case_source_failed", "case_source_result_private",
			"contextDigest", "executionGrantId", "contextEpoch", "datasetSnapshotId",
			"transportStatus", "semanticStatus", "safeToAnswer", "evidenceReceipt", "AuthorityRef",
			"6222020202020202020", "/private/case.sqlite", "SELECT * FROM private_case", "PRIVATE_PROVIDER_BODY",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s public seam retained private authority/data %q: %s", name, forbidden, body)
			}
		}
	}
}
