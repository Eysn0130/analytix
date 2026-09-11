package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeCacheBaselineNeverCrossesCaseBinding(t *testing.T) {
	handler, threadID, workspace := cacheBaselineHandler(t)
	contextA := cacheBaselineCaseContext(t, threadID, "turn-a", workspace, "case-a", "snapshot-a", 1)
	setCacheBaselineSecurityState(t, handler, threadID, contextA)
	handler.runtimeCacheDiagnostics(threadID, contextA, cacheBaselineResult("prefix-a"))

	contextB := cacheBaselineCaseContext(t, threadID, "turn-b", workspace, "case-b", "snapshot-b", 2)
	setCacheBaselineSecurityState(t, handler, threadID, contextB)
	diagnostics := handler.runtimeCacheDiagnostics(threadID, contextB, cacheBaselineResult("prefix-b"))
	if diagnostics["prefixChanged"] == true || diagnostics["cacheBaselineObserved"] != true {
		t.Fatalf("case switch compared an incompatible cache baseline: %#v", diagnostics)
	}
	serialized, _ := json.Marshal(diagnostics)
	for _, forbidden := range []string{"case-a", "case-b", workspace} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("cache diagnostics exposed continuity input %q: %s", forbidden, serialized)
		}
	}
}

func TestRuntimeCacheBaselineNeverCrossesDatasetSnapshot(t *testing.T) {
	handler, threadID, workspace := cacheBaselineHandler(t)
	first := cacheBaselineCaseContext(t, threadID, "turn-a", workspace, "case-a", "snapshot-a", 3)
	setCacheBaselineSecurityState(t, handler, threadID, first)
	handler.runtimeCacheDiagnostics(threadID, first, cacheBaselineResult("prefix-a"))

	second := cacheBaselineCaseContext(t, threadID, "turn-b", workspace, "case-a", "snapshot-b", 4)
	setCacheBaselineSecurityState(t, handler, threadID, second)
	if diagnostics := handler.runtimeCacheDiagnostics(threadID, second, cacheBaselineResult("prefix-b")); diagnostics["prefixChanged"] == true {
		t.Fatalf("dataset snapshot switch compared an incompatible cache baseline: %#v", diagnostics)
	}

	third := cacheBaselineCaseContext(t, threadID, "turn-c", workspace, "case-a", "snapshot-b", 4)
	setCacheBaselineSecurityState(t, handler, threadID, third)
	if diagnostics := handler.runtimeCacheDiagnostics(threadID, third, cacheBaselineResult("prefix-c")); diagnostics["prefixChanged"] != true {
		t.Fatalf("compatible adjacent turn did not compare its cache baseline: %#v", diagnostics)
	}
}

func TestSourceUnavailableDoesNotAdvanceCacheBaseline(t *testing.T) {
	handler, threadID, workspace := cacheBaselineHandler(t)
	context := cacheBaselineCaseContext(t, threadID, "turn-a", workspace, "case-a", "snapshot-a", 5)
	setCacheBaselineSecurityState(t, handler, threadID, context)
	handler.runtimeCacheDiagnostics(threadID, context, cacheBaselineResult("prefix-a"))

	zero := handler.runtimeCacheDiagnostics(threadID, context, provider.Result{})
	if zero["cacheBaselineObserved"] != false {
		t.Fatalf("zero provider call advanced the cache baseline: %#v", zero)
	}
	next := handler.runtimeCacheDiagnostics(threadID, context, cacheBaselineResult("prefix-b"))
	if next["prefixChanged"] != true {
		t.Fatalf("zero provider call erased the prior compatible baseline: %#v", next)
	}
}

func cacheBaselineHandler(t *testing.T) (*runtimeServerHandler, string, string) {
	t.Helper()
	workspace := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	thread, err := handler.store.CreateThread(map[string]any{"title": "Cache baseline", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	return handler, stringField(thread, "id"), workspace
}

func cacheBaselineCaseContext(t *testing.T, threadID, turnID, workspace, caseID, snapshot string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	context, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: caseID,
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding:" + caseID)),
		DatasetSnapshotID:  securitytest.DatasetSnapshotID(snapshot),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + snapshot)), ContextEpoch: epoch,
		IssuedAt: time.Date(2026, 7, 13, 16, 0, int(epoch), 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("build cache context: %v", err)
	}
	return context
}

func setCacheBaselineSecurityState(t *testing.T, handler *runtimeServerHandler, threadID string, context domainsecurity.TurnSecurityContext) {
	t.Helper()
	if _, err := handler.store.PatchThread(threadID, map[string]any{"securityState": turnsecurityapp.PublicRecord(context)}); err != nil {
		t.Fatal(err)
	}
}

func cacheBaselineResult(prefix string) provider.Result {
	return provider.Result{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		PrefixShape: domainmodel.PrefixShape{
			PrefixHash: prefix, Provider: "deepseek", ProviderID: "deepseek",
			EndpointFormat: "chat_completions", Model: "deepseek-chat", Route: "chat_completions",
		},
	}
}
