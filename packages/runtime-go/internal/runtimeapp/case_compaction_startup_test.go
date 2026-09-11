package runtimeapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	serverapp "analytix.local/runtime-go/internal/server"
)

const runtimeProductionCaseCompactionPrivateSourceV1 = "PRIVATE_CASE_COMPACTION_SOURCE_MUST_NOT_CROSS_PUBLIC_SEAM"

func TestRuntimeProductionAutomaticCaseCompactionSurvivesFreshStartup(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "automatic-case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o700); err != nil {
		t.Fatal(err)
	}
	binding, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_runtime_auto_compaction_001",
		"source": "analytix-data-analysis", "updatedAt": "2026-08-25T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	const (
		providerID = "runtime-auto-case-compaction-provider"
		largeModel = "runtime-auto-case-large"
		smallModel = "runtime-auto-case-small"
	)
	var (
		providerMu     sync.Mutex
		providerBodies [][]byte
	)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read automatic-compaction provider request: %v", readErr)
			http.Error(w, "read request", http.StatusInternalServerError)
			return
		}
		providerMu.Lock()
		providerBodies = append(providerBodies, append([]byte(nil), body...))
		call := len(providerBodies)
		providerMu.Unlock()
		content := "automatic-case-continuation-ok"
		if call <= 2 {
			content = fmt.Sprintf("MIXED_ORDINARY_HISTORY_%d_%s", call, strings.Repeat("x", 12_000))
		}
		frame, marshalErr := json.Marshal(map[string]any{
			"choices": []map[string]any{{
				"delta": map[string]any{"content": content}, "finish_reason": "stop",
			}},
		})
		if marshalErr != nil {
			t.Errorf("encode automatic-compaction provider response: %v", marshalErr)
			http.Error(w, "encode response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", frame)
	}))
	defer providerServer.Close()
	modelProviders, err := json.Marshal(map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id": providerID, "apiKey": "test-only", "baseUrl": providerServer.URL + "/v1",
			"endpointFormat": "chat_completions", "models": []string{largeModel, smallModel},
			"modelProfiles": map[string]any{
				largeModel: map[string]any{"contextWindowTokens": 100_000},
				smallModel: map[string]any{"contextWindowTokens": 8_000},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: filepath.Join(root, "durable"),
		DataDir: filepath.Join(root, "runtime-data"), UserDataDir: filepath.Join(root, "user-data"),
		ProviderID: providerID, Model: largeModel, EndpointFormat: "chat_completions",
		ModelProvidersJSON: string(modelProviders),
	}
	seedProviderRegistryExecutionAuthorityV1(
		t, config.DataDir, providerID, providerServer.URL+"/v1",
		[]string{largeModel, smallModel}, largeModel, "test-only",
	)
	first, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("initial production runtime startup: %v", err)
	}
	thread := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads", map[string]any{
		"title": "production automatic case compaction", "workspace": workspace,
		"providerId": providerID, "model": largeModel,
	})
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		t.Fatalf("production thread creation returned no id: %#v", thread)
	}
	runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/goal", map[string]any{
		"objective": "Preserve the active automatic case-compaction objective",
	})
	runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/todos", map[string]any{
		"todos": []map[string]any{{"content": "Preserve the unfinished automatic case-compaction todo", "status": "pending"}},
	})
	for index, prompt := range []string{
		"summarize the first ordinary workspace observation",
		"summarize the second ordinary workspace observation",
	} {
		started := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
			"prompt": prompt, "model": largeModel,
		})
		if turnID, _ := started["turnId"].(string); turnID == "" {
			t.Fatalf("ordinary setup turn %d has no turn id: %#v", index+1, started)
		}
	}
	providerMu.Lock()
	setupCalls := len(providerBodies)
	providerMu.Unlock()
	if setupCalls != 2 {
		t.Fatalf("ordinary mixed-history setup provider calls=%d want=2", setupCalls)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".analytix", "case-project.json"), binding, 0o600); err != nil {
		t.Fatal(err)
	}
	caseStarted := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
		"prompt": runtimeProductionCaseCompactionPrivateSourceV1 +
			" LATEST_SAFE_CASE_CONSTRAINT_V1 keep the current bounded evidence scope",
		"riskIntent": "case", "model": largeModel,
	})
	if turnID, _ := caseStarted["turnId"].(string); turnID == "" {
		t.Fatalf("case archive setup turn has no turn id: %#v", caseStarted)
	}
	beforeTrigger := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodGet, "/v1/threads/"+threadID, nil)
	if runtimeProductionCaseCompactionCountV1(beforeTrigger) != 0 {
		t.Fatalf("large-context setup compacted before the automatic trigger: %#v", beforeTrigger)
	}
	triggered := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
		"prompt": "continue the ordinary workspace task after automatic case compaction", "model": smallModel,
	})
	if turnID, _ := triggered["turnId"].(string); turnID == "" {
		t.Fatalf("automatic case-compaction trigger has no turn id: %#v", triggered)
	}
	providerMu.Lock()
	providerCalls := len(providerBodies)
	providerMu.Unlock()
	if providerCalls != 2 {
		t.Fatalf("source-unavailable trigger reached provider after automatic compaction: calls=%d want=2", providerCalls)
	}
	beforeRestart := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodGet, "/v1/threads/"+threadID, nil)
	if runtimeProductionCaseCompactionCountV1(beforeRestart) != 1 {
		t.Fatalf("automatic trigger did not create exactly one public compaction marker: %#v", beforeRestart)
	}
	markerBefore := runtimeProductionCaseCompactionMarkerV1(t, beforeRestart)
	if markerBefore["auto"] != true || strings.TrimSpace(runtimeProductionCaseCompactionStringV1(markerBefore, "sourceDigest")) == "" {
		t.Fatalf("production automatic case-compaction marker is incomplete: %#v", markerBefore)
	}
	shutdownOwnedRuntimeHandler(t, first)

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("fresh production runtime startup rejected committed automatic case compaction: %v", err)
	}
	afterRestart := runtimeProductionCaseCompactionJSONV1(t, restarted, http.MethodGet, "/v1/threads/"+threadID, nil)
	markerAfter := runtimeProductionCaseCompactionMarkerV1(t, afterRestart)
	if runtimeProductionCaseCompactionCountV1(afterRestart) != 1 || markerAfter["auto"] != true ||
		runtimeProductionCaseCompactionStringV1(markerAfter, "sourceDigest") != runtimeProductionCaseCompactionStringV1(markerBefore, "sourceDigest") {
		t.Fatalf("fresh startup changed the committed automatic marker: before=%#v after=%#v", markerBefore, markerAfter)
	}
	shutdownOwnedRuntimeHandler(t, restarted)

	store, err := serverapp.NewProductionDurableEventSessionStore(config.ProductionDurableRoot)
	if err != nil {
		t.Fatalf("open committed production store for exact readback: %v", err)
	}
	rawThread, err := store.GetThreadForAuthorityRepair(threadID)
	if err != nil {
		t.Fatalf("read committed automatic case compaction: %v", err)
	}
	rawTurn, rawItem := runtimeProductionAutomaticCaseCompactionRawV1(t, rawThread)
	continuation, err := threaddomain.ParseTaskContinuationSnapshotV1(rawItem["taskContinuation"])
	if err != nil || continuation.Goal == nil ||
		continuation.Goal.Objective != "Preserve the active automatic case-compaction objective" ||
		len(continuation.Todos) != 1 || continuation.Todos[0].Content != "Preserve the unfinished automatic case-compaction todo" ||
		!containsRuntimeProductionCaseConstraintV1(continuation.LatestUserConstraints, "LATEST_SAFE_CASE_CONSTRAINT_V1") {
		t.Fatalf("automatic case continuation lost mixed task state: continuation=%#v err=%v", continuation, err)
	}
	operation, err := turnapp.ParseCaseCompactionOperationBindingV1(rawItem["caseCompactionBinding"])
	if err != nil || operation.SourceContextDigest != runtimeProductionCaseCompactionStringV1(rawItem, "sourceContextDigest") ||
		operation.ContinuationDigest != continuation.StateDigest || len(operation.AuthorityTurnIDs) == 0 ||
		runtimeProductionCaseCompactionStringV1(rawItem, "sourceDigest") != runtimeProductionCaseCompactionStringV1(markerBefore, "sourceDigest") {
		t.Fatalf("automatic case compaction lost signed source or authority inventory: operation=%#v item=%#v err=%v", operation, rawItem, err)
	}
	compactionContext, err := domainsecurity.ParseTurnSecurityContext(rawTurn["securityContext"])
	if err != nil {
		t.Fatalf("parse automatic compaction security context: %v", err)
	}
	var sourceEpoch uint64
	for _, raw := range rawThread["turns"].([]any) {
		turn, _ := raw.(map[string]any)
		securityContext, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if parseErr == nil && securityContext.ContextDigest == operation.SourceContextDigest {
			sourceEpoch = securityContext.ContextEpoch
		}
	}
	if sourceEpoch == 0 || compactionContext.ContextEpoch != sourceEpoch+1 || rawTurn["contextEpochSnapshot"] == nil {
		t.Fatalf("automatic case compaction did not make exactly one signed epoch transition: source=%d compact=%d turn=%#v", sourceEpoch, compactionContext.ContextEpoch, rawTurn)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("read automatic compaction lifecycle: %v", err)
	}
	startedCount, completedCount := 0, 0
	for _, event := range replay.Events {
		if event["auto"] != true {
			continue
		}
		switch runtimeProductionCaseCompactionStringV1(event, "kind") {
		case "compaction_started":
			startedCount++
		case "compaction_completed":
			completedCount++
		}
	}
	if startedCount != 1 || completedCount != 1 {
		t.Fatalf("automatic case compaction lifecycle is not exactly one pair: started=%d completed=%d", startedCount, completedCount)
	}

	var priorGeneral domainsecurity.TurnSecurityContext
	for _, raw := range rawThread["turns"].([]any) {
		turn, _ := raw.(map[string]any)
		securityContext, parseErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if parseErr == nil && domainsecurity.TurnSecurityContextIsGeneral(securityContext) &&
			securityContext.ContextEpoch < compactionContext.ContextEpoch {
			priorGeneral = securityContext
			break
		}
	}
	priorIssuedAt, err := time.Parse(time.RFC3339Nano, priorGeneral.IssuedAt)
	if err != nil {
		t.Fatalf("automatic case compaction has no retained prior general context: %v", err)
	}
	forgedContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_forged_non_wanted_ordinary", WorkspaceRealPath: priorGeneral.WorkspaceRealPath,
		TenantID: priorGeneral.TenantID, UserID: priorGeneral.UserID, CaseID: priorGeneral.CaseID,
		CaseBindingHash: priorGeneral.CaseBindingHash, DatasetSnapshotID: priorGeneral.DatasetSnapshotID,
		SourceManifestHash: priorGeneral.SourceManifestHash, ContextEpoch: priorGeneral.ContextEpoch,
		IssuedAt: priorIssuedAt.Add(time.Nanosecond), PublicationPolicy: priorGeneral.PublicationPolicy,
		RiskAuthorityBinding: priorGeneral.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatalf("construct forged prior ordinary context: %v", err)
	}
	hostile := contracts.CloneMap(rawThread)
	hostileTurns := hostile["turns"].([]any)
	insertAt := len(hostileTurns)
	for index, raw := range hostileTurns {
		turn, _ := raw.(map[string]any)
		if runtimeProductionCaseCompactionStringV1(turn, "id") == runtimeProductionCaseCompactionStringV1(rawTurn, "id") {
			insertAt = index
			break
		}
	}
	const forgedOrdinaryText = "FORGED_NON_WANTED_ORDINARY_MUST_NOT_REPLAY"
	forgedTurn := map[string]any{
		"id": "turn_forged_non_wanted_ordinary", "threadId": threadID, "status": "completed",
		"createdAt": forgedContext.IssuedAt, "startedAt": forgedContext.IssuedAt, "finishedAt": forgedContext.IssuedAt,
		"securityContext": turnsecurityapp.PublicRecord(forgedContext),
		"items": []any{map[string]any{
			"id": "item_forged_non_wanted_ordinary", "turnId": "turn_forged_non_wanted_ordinary",
			"threadId": threadID, "kind": "assistant_text", "role": "assistant", "status": "completed",
			"text": forgedOrdinaryText, "createdAt": forgedContext.IssuedAt,
			"finishedAt": forgedContext.IssuedAt,
		}},
	}
	hostileTurns = append(hostileTurns, nil)
	copy(hostileTurns[insertAt+1:], hostileTurns[insertAt:])
	hostileTurns[insertAt] = forgedTurn
	hostile["turns"] = hostileTurns
	if err := store.ReplaceThreadForAuthorityRepair(threadID, hostile); err != nil {
		t.Fatalf("inject isolated forged ordinary turn: %v", err)
	}
	if hostileHandler, startupErr := NewRuntimeServerHandlerE(config); startupErr == nil {
		shutdownOwnedRuntimeHandler(t, hostileHandler)
		t.Fatal("forged non-wanted ordinary turn passed production restart authority validation")
	}
	providerMu.Lock()
	finalProviderBodies := append([][]byte(nil), providerBodies...)
	providerMu.Unlock()
	for _, providerBody := range finalProviderBodies {
		if bytes.Contains(providerBody, []byte(forgedOrdinaryText)) {
			t.Fatal("forged non-wanted ordinary text crossed a provider compaction cut")
		}
	}
}

func TestRuntimeProductionCaseCompactionSurvivesFreshStartup(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "case-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".analytix"), 0o700); err != nil {
		t.Fatal(err)
	}
	binding, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_runtime_compaction_startup_001",
		"source": "analytix-data-analysis", "updatedAt": "2026-08-13T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".analytix", "case-project.json"), binding, 0o600); err != nil {
		t.Fatal(err)
	}
	config := Config{
		RuntimeToken:          DefaultRuntimeToken,
		ProductionDurableRoot: filepath.Join(root, "durable"),
		DataDir:               filepath.Join(root, "runtime-data"),
		UserDataDir:           filepath.Join(root, "user-data"),
		ProviderID:            "runtime-case-compaction-provider",
		BaseURL:               "https://provider.invalid/v1",
		APIKey:                "test-only",
		Model:                 "runtime-case-compaction-model",
		EndpointFormat:        "chat_completions",
	}
	first, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("initial production runtime startup: %v", err)
	}
	thread := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads", map[string]any{
		"title": "production boundary-only compaction", "workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model,
	})
	threadID, _ := thread["id"].(string)
	if threadID == "" {
		t.Fatalf("production thread creation returned no id: %#v", thread)
	}
	for index, prompt := range []string{
		runtimeProductionCaseCompactionPrivateSourceV1,
		"continue bounded host-boundary work one",
		"continue bounded host-boundary work two",
		"continue bounded host-boundary work three",
	} {
		started := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
			"prompt": prompt, "riskIntent": "case",
		})
		if turnID, _ := started["turnId"].(string); turnID == "" {
			t.Fatalf("case turn %d has no turn id: %#v", index+1, started)
		}
	}
	_ = runtimeProductionCaseCompactionJSONV1(t, first, http.MethodGet, "/v1/threads/"+threadID, nil)
	compacted := runtimeProductionCaseCompactionJSONV1(t, first, http.MethodPost, "/v1/threads/"+threadID+"/compact", map[string]any{
		"reason": "manual",
	})
	if compacted["ok"] != true || compacted["auto"] == true {
		t.Fatalf("manual production case compaction response is incomplete: %#v", compacted)
	}
	if sourceDigest, _ := compacted["sourceDigest"].(string); sourceDigest == "" {
		t.Fatalf("manual production case compaction has no source digest: %#v", compacted)
	}
	_ = runtimeProductionCaseCompactionJSONV1(t, first, http.MethodGet, "/v1/threads/"+threadID, nil)
	shutdownOwnedRuntimeHandler(t, first)

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("fresh production runtime startup rejected committed case compaction: %v", err)
	}
	defer shutdownOwnedRuntimeHandler(t, restarted)
	list := runtimeProductionCaseCompactionJSONV1(t, restarted, http.MethodGet, "/v1/threads?limit=1", nil)
	threads, _ := list["threads"].([]any)
	listed := false
	for _, raw := range threads {
		summary, _ := raw.(map[string]any)
		if summary["id"] == threadID {
			listed = true
			break
		}
	}
	if !listed {
		t.Fatalf("restarted production case thread was not publicly listed: %#v", list)
	}
	detail := runtimeProductionCaseCompactionJSONV1(t, restarted, http.MethodGet, "/v1/threads/"+threadID, nil)
	if detail["historyAuthority"] != threadapp.CaseBoundaryOnlyHistoryAuthority {
		t.Fatalf("restarted case detail lost boundary-only history authority: %#v", detail)
	}
	marker := runtimeProductionCaseCompactionMarkerV1(t, detail)
	if marker["auto"] != false {
		t.Fatalf("restarted public compaction marker changed manual mode: %#v", marker)
	}
	if got, _ := marker["sourceDigest"].(string); got != compacted["sourceDigest"] {
		t.Fatalf("restarted public compaction marker changed source digest: marker=%#v response=%#v", marker, compacted)
	}
	if sourceItemIDs, ok := marker["sourceItemIds"].([]any); !ok || len(sourceItemIDs) != 0 {
		t.Fatalf("restarted public compaction marker carried source item identities: %#v", marker)
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"replacedTokens", "taskContinuation", "caseCompactionBinding", "sourceContextDigest",
		runtimeProductionCaseCompactionPrivateSourceV1,
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("restarted public case detail exposed forbidden %q: %s", forbidden, body)
		}
	}
}

func runtimeProductionCaseCompactionJSONV1(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body map[string]any,
) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code < http.StatusOK || recorder.Code >= http.StatusMultipleChoices {
		t.Fatalf("%s %s status=%d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
	return decoded
}

func runtimeProductionCaseCompactionMarkerV1(t *testing.T, detail map[string]any) map[string]any {
	t.Helper()
	turns, _ := detail["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item["kind"] == "compaction" {
				if item["schemaVersion"] != float64(3) || item["caseHistoryProjectionVersion"] != float64(2) ||
					item["reasoningExcluded"] != true || item["assistantProseExcluded"] != true ||
					item["toolPayloadsExcluded"] != true || item["caseFactsExcluded"] != true {
					t.Fatalf("restarted public case marker is incomplete: %#v", item)
				}
				if _, present := item["replacedTokens"]; present {
					t.Fatalf("restarted public case marker exposed replacedTokens: %#v", item)
				}
				return item
			}
		}
	}
	t.Fatalf("restarted public case detail has no compaction marker: %#v", detail)
	return nil
}

func runtimeProductionCaseCompactionCountV1(detail map[string]any) int {
	count := 0
	turns, _ := detail["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item["kind"] == "compaction" {
				count++
			}
		}
	}
	return count
}

func runtimeProductionAutomaticCaseCompactionRawV1(
	t *testing.T,
	thread map[string]any,
) (map[string]any, map[string]any) {
	t.Helper()
	var foundTurn map[string]any
	var foundItem map[string]any
	for _, rawTurn := range thread["turns"].([]any) {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if item["kind"] != "compaction" || item["auto"] != true {
				continue
			}
			if foundItem != nil {
				t.Fatal("committed production thread has multiple automatic compactions")
			}
			foundTurn, foundItem = turn, item
		}
	}
	if foundItem == nil {
		t.Fatal("committed production thread has no automatic case compaction")
	}
	return foundTurn, foundItem
}

func containsRuntimeProductionCaseConstraintV1(values []string, sentinel string) bool {
	for _, value := range values {
		if strings.Contains(value, sentinel) {
			return true
		}
	}
	return false
}

func runtimeProductionCaseCompactionStringV1(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}
