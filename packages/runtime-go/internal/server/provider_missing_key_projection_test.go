package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	provider "analytix.local/runtime-go/internal/provider"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// This fixture supplies ordinary key-free intent, then exercises the actual
// missing-key configuration owner at the physical-effect preparation boundary.
// It has no Secret Store, network transport, or production Registry credentials.
type missingKeyProjectionResolver struct {
	handlerProviderExecutionResolverForTest
	missing     provider.RuntimeProviderConfigSet
	sourceError error
}

func (resolver *missingKeyProjectionResolver) ResolveTurnExecution(_ context.Context, _ provider.TurnExecutionInput) (provider.TurnExecutionResult, error) {
	result, err := resolver.missing.ResolveTurnExecution(provider.TurnExecutionInput{})
	resolver.sourceError = err
	return result, err
}

func TestMissingProviderKeySourceIsClosedAcrossDurableSSERecoveryAndModel(t *testing.T) {
	const identityCanary = "d5-private-provider-canary"
	const prompt = "D5 ordinary user request"
	const closedFailure = "The turn failed before a verified response was available."
	config := RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "d5-public-provider", BaseURL: "https://provider.invalid", APIKey: "synthetic-test-placeholder",
		Model: "d5-public-model", EndpointFormat: "chat_completions",
	}
	handler := NewRuntimeServerHandler(config).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	resolver := &missingKeyProjectionResolver{
		handlerProviderExecutionResolverForTest: handlerProviderExecutionResolverForTest{handler: handler},
		missing: provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
			DefaultProviderID: identityCanary, DefaultBaseURL: "https://provider.invalid", DefaultModel: config.Model,
		}),
	}
	handler.providerExecution = resolver
	transport := &workspaceReadCanaryProvider{}
	handler.provider = transport
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{"title": "Missing key projection", "workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	stderr := captureRuntimeStderrForTest(t, func() {
		_, _ = handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: prompt})
	})
	if resolver.sourceError == nil || resolver.sourceError.Error() != "provider configuration error: apiKey is required for provider "+identityCanary {
		t.Fatal("real missing-key source was not reached")
	}
	if transport.calls != 0 {
		t.Fatal("missing-key execution reached the provider transport")
	}
	assertSafe := func(label, body string) {
		t.Helper()
		for _, forbidden := range []string{identityCanary, resolver.sourceError.Error(), config.APIKey,
			fmt.Sprintf("%x", sha256.Sum256([]byte(identityCanary))),
			fmt.Sprintf("%x", sha256.Sum256([]byte(resolver.sourceError.Error())))} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s retained private missing-key or credential bytes", label)
			}
		}
	}
	assertSafe("stderr", stderr)
	assertProjection := func(label string) {
		t.Helper()
		raw, err := handler.store.GetThread(threadID)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		assertSafe(label+" durable history", string(body))
		turns := listAny(raw["turns"])
		if len(turns) != 1 || stringField(turns[0].(map[string]any), "status") != "failed" {
			t.Fatal("missing-key failure did not commit a failed turn")
		}
		items := listAny(turns[0].(map[string]any)["items"])
		foundClosedFailure := false
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "error" {
				if stringField(item, "message") != closedFailure {
					t.Fatal("durable failure lost its fixed host diagnostic")
				}
				foundClosedFailure = true
			}
		}
		if !foundClosedFailure {
			t.Fatal("durable failure omitted its closed diagnostic")
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		sse := requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0")
		assertSafe(label+" public SSE", sse)
		if !strings.Contains(sse, "general_terminal_batch") || !strings.Contains(sse, "turn_failed") || !strings.Contains(sse, closedFailure) {
			t.Fatal("public SSE lost the closed failure terminal")
		}
		assertSafe(label+" durable files", readCaseIngressPublicTreeV1(t, config.DurableTempDir))
		assertSafe(label+" runtime data", readCaseIngressPublicTreeV1(t, config.DataDir))
	}
	assertProjection("before store reopen")
	reopened, err := NewTempDurableEventSessionStore(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := reopened.PreflightGeneralTerminalPublicationRecoveryV1()
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.ApplyGeneralTerminalPublicationRecoveryV1(plans); err != nil {
		t.Fatal(err)
	}
	handler.store = reopened
	handler.store.SetCaseThreadAuthority(handler.caseThreads)
	handler.threads = nil
	assertProjection("after store reopen")
	installHandlerProviderExecutionResolverForTest(handler)
	_, err = handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{Prompt: "Continue the ordinary request"})
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 || len(transport.requests) != 1 {
		t.Fatal("healthy next turn did not reach the synthetic provider")
	}
	assertSafe("next model request", string(transport.requests[0]))
	if !strings.Contains(string(transport.requests[0]), prompt) {
		t.Fatal("recovery model projection lost ordinary user history")
	}
}
