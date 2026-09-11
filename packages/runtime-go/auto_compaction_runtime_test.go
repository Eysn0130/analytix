//go:build !analytix_prod

package runtimego

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeServerAutomaticCompactionRunsBeforeAdmissionAndRestoresContinuation(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	providerFrames := make([][]string, 0, 5)
	for index := 1; index <= 3; index++ {
		historyAnswer := fmt.Sprintf("history-source-%02d-%s", index, strings.Repeat("é", 4_000))
		providerFrames = append(providerFrames, []string{
			"data: " + string(mustJSONNoTest(map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{"content": historyAnswer}}},
			})),
			`data: [DONE]`,
		})
	}
	for index := 0; index < 2; index++ {
		providerFrames = append(providerFrames, []string{
			`data: {"choices":[{"delta":{"content":"continuation accepted"}}]}`,
			`data: [DONE]`,
		})
	}
	provider := newCompleteProviderServer(t, providerFrames)
	_, _ = prepareRuntimeServerExplicitProviderRegistryFixture(
		t, dataDir, provider.URL(), "auto-compact-provider", "large-context",
	)
	modelProviders := string(mustJSONNoTest(map[string]any{
		"defaultProviderId": "auto-compact-provider",
		"providers": []map[string]any{{
			"id":             "auto-compact-provider",
			"baseUrl":        provider.URL() + "/v1",
			"endpointFormat": "chat_completions",
			"models":         []string{"large-context", "small-context"},
			"modelProfiles": map[string]any{
				"large-context": map[string]any{"contextWindowTokens": 50_000},
				"small-context": map[string]any{"contextWindowTokens": 18_000},
			},
		}},
	}))
	connectProviderRegistry := func(serverURL string) {
		t.Helper()
		const registryPath = "/v1/provider-registry"
		initialRegistry := assertLiveJSON(t, serverURL, http.MethodGet, registryPath, DefaultRuntimeToken, nil, http.StatusOK)
		initialProviders, ok := initialRegistry["providers"].([]any)
		if !ok || len(initialProviders) != 0 || stringField(initialRegistry, "selectedProviderId") != "" {
			t.Fatal("automatic compaction Registry fixture did not start empty and unselected")
		}
		registryRevision := stringField(initialRegistry, "registryRevision")
		registryIncarnation := stringField(initialRegistry, "registryIncarnation")
		if registryRevision == "" || registryIncarnation == "" {
			t.Fatal("automatic compaction Registry fixture did not expose an exact initial CAS")
		}

		credential := []byte("synthetic-auto-compaction-provider-credential")
		encodedCredential := base64.StdEncoding.EncodeToString(credential)
		clear(credential)
		connectedRegistry := assertLiveJSON(t, serverURL, http.MethodPost, registryPath, DefaultRuntimeToken, mustJSON(t, map[string]any{
			"schemaVersion": 1,
			"expected": map[string]any{
				"registryRevision":          registryRevision,
				"registryIncarnation":       registryIncarnation,
				"providerRevision":          "0",
				"providerGeneration":        "0",
				"providerIncarnation":       "",
				"providerCredentialPurpose": "",
			},
			"provider": map[string]any{
				"id":                 "auto-compact-provider",
				"kind":               "openai-compatible",
				"endpoint":           provider.URL() + "/v1",
				"proxy":              "",
				"models":             []string{"large-context", "small-context"},
				"mediaModels":        []string{},
				"selectedModel":      "large-context",
				"selectedMediaModel": "",
				"selectedRoutes":     []string{"primary"},
			},
			"credential": map[string]any{
				"kind":        "set",
				"purpose":     "provider-api-key",
				"valueBase64": encodedCredential,
			},
		}), http.StatusOK)
		assertRegistry := func(label string, registry map[string]any) {
			t.Helper()
			projection, err := json.Marshal(registry)
			if err != nil {
				t.Fatalf("automatic compaction Registry %s projection was not valid JSON", label)
			}
			for _, forbidden := range []string{
				"synthetic-auto-compaction-provider-credential", encodedCredential,
				"credentialRef", "ciphertext", "nonce", "masterKey", "master-key", "valueBase64", "rawBody", "raw_body",
			} {
				if strings.Contains(string(projection), forbidden) {
					t.Fatalf("automatic compaction Registry %s projection exposed a forbidden private field or value", label)
				}
			}
			providers, providersOK := registry["providers"].([]any)
			if label == "connect" {
				providers = []any{registry["provider"]}
				providersOK = true
			}
			if !providersOK || len(providers) != 1 || label != "connect" && stringField(registry, "selectedProviderId") != "auto-compact-provider" {
				t.Fatalf("automatic compaction Registry %s did not expose exactly one selected Provider", label)
			}
			selected, ok := providers[0].(map[string]any)
			if !ok {
				t.Fatalf("automatic compaction Registry %s winner was not a Provider object", label)
			}
			models, modelsOK := selected["models"].([]any)
			routes, routesOK := selected["selectedRoutes"].([]any)
			if stringField(selected, "id") != "auto-compact-provider" ||
				stringField(selected, "kind") != "openai-compatible" ||
				stringField(selected, "endpoint") != provider.URL()+"/v1" ||
				stringField(selected, "selectedModel") != "large-context" ||
				!modelsOK || len(models) != 2 || models[0] != "large-context" || models[1] != "small-context" ||
				!routesOK || len(routes) != 1 || routes[0] != "primary" ||
				!boolField(selected, "credentialConfigured") {
				t.Fatalf("automatic compaction Registry %s winner did not match the bounded two-model fixture", label)
			}
		}
		assertRegistry("connect", connectedRegistry)
		readback := assertLiveJSON(t, serverURL, http.MethodGet, registryPath, DefaultRuntimeToken, nil, http.StatusOK)
		assertRegistry("readback", readback)
	}
	config := RuntimeServerContractConfig{
		RuntimeToken:       DefaultRuntimeToken,
		DurableTempDir:     durableRoot,
		Host:               "127.0.0.1",
		Port:               0,
		DataDir:            dataDir,
		ModelProvidersJSON: modelProviders,
	}

	firstHandler := newRuntimeServerTestHandler(t, config)
	first := httptest.NewServer(firstHandler)
	connectProviderRegistry(first.URL)
	thread := assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"title":      "Automatic compaction restart",
		"workspace":  dataDir,
		"providerId": "auto-compact-provider",
		"model":      "large-context",
	}), http.StatusCreated)
	threadID := stringField(thread, "id")
	assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads/"+threadID+"/goal", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"objective": "Preserve the active objective across automatic compaction",
	}), http.StatusOK)
	assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads/"+threadID+"/todos", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"todos": []map[string]any{{"content": "Preserve this unfinished todo", "status": "pending"}},
	}), http.StatusOK)

	for index := 1; index <= 3; index++ {
		prompt := fmt.Sprintf("简单总结一下 history-%02d", index)
		assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
			"prompt":     prompt,
			"providerId": "auto-compact-provider",
			"model":      "large-context",
		}), http.StatusAccepted)
	}
	if provider.RequestCount() != 3 {
		t.Fatalf("history setup provider requests = %d, want 3", provider.RequestCount())
	}

	triggerPrompt := "简单总结一下 threshold"
	trigger := assertLiveJSON(t, first.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     triggerPrompt,
		"providerId": "auto-compact-provider",
		"model":      "small-context",
	}), http.StatusAccepted)
	triggerTurnID := stringField(trigger, "turnId")
	if provider.RequestCount() != 4 {
		t.Fatalf("automatic compaction should admit exactly one provider request after rewrite, got %d", provider.RequestCount())
	}
	triggerBody := provider.Body(3)
	for _, required := range []string{
		"Analytix task continuation snapshot",
		"Preserve the active objective across automatic compaction",
		"Preserve this unfinished todo",
		"unverified_for_case_facts",
		triggerPrompt,
	} {
		if !strings.Contains(triggerBody, required) {
			t.Fatalf("provider request after automatic compaction omitted %q:\n%s", required, triggerBody)
		}
	}
	if strings.Contains(triggerBody, "history-source-01-") {
		t.Fatalf("provider request replayed compacted source prose:\n%s", triggerBody)
	}

	replay := liveSSE(t, first.URL, "/v1/threads/"+threadID+"/events?since_seq=0", DefaultRuntimeToken, http.StatusOK)
	events := parseRuntimeServerSSEEvents(t, replay)
	compactionIndex := -1
	turnStartedIndex := -1
	for index, event := range events {
		switch stringField(event, "kind") {
		case "compaction_completed":
			if event["auto"] == true {
				compactionIndex = index
			}
		case "turn_started":
			if stringField(event, "turnId") == triggerTurnID {
				turnStartedIndex = index
			}
		}
	}
	if compactionIndex < 0 || turnStartedIndex < 0 || compactionIndex >= turnStartedIndex {
		t.Fatalf("automatic compaction must durably complete before turn admission: compaction=%d started=%d events=%#v", compactionIndex, turnStartedIndex, events)
	}

	beforeRestart := assertLiveJSON(t, first.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	continuationBefore := automaticCompactionContinuationForTest(t, beforeRestart)
	continuationBeforeJSON := string(mustJSON(t, continuationBefore))
	if !strings.Contains(continuationBeforeJSON, `"stateDigest"`) {
		t.Fatalf("automatic compaction continuation is not sealed: %s", continuationBeforeJSON)
	}

	first.Close()
	shutdownRuntimeTestHandler(t, firstHandler)

	restarted := httptest.NewServer(newRuntimeServerTestHandler(t, config))
	defer restarted.Close()
	afterRestart := assertLiveJSON(t, restarted.URL, http.MethodGet, "/v1/threads/"+threadID, DefaultRuntimeToken, nil, http.StatusOK)
	continuationAfterJSON := string(mustJSON(t, automaticCompactionContinuationForTest(t, afterRestart)))
	if continuationAfterJSON != continuationBeforeJSON {
		t.Fatalf("restart changed the durable task continuation:\nbefore=%s\nafter=%s", continuationBeforeJSON, continuationAfterJSON)
	}

	restartPrompt := "简单总结一下 restart"
	assertLiveJSON(t, restarted.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", DefaultRuntimeToken, mustJSON(t, map[string]any{
		"prompt":     restartPrompt,
		"providerId": "auto-compact-provider",
		"model":      "small-context",
	}), http.StatusAccepted)
	if provider.RequestCount() != 5 {
		t.Fatalf("restart continuation should make one provider request, got %d", provider.RequestCount())
	}
	restartBody := provider.Body(4)
	for _, required := range []string{
		"Analytix task continuation snapshot",
		"Preserve the active objective across automatic compaction",
		"Preserve this unfinished todo",
		"unverified_for_case_facts",
		restartPrompt,
	} {
		if !strings.Contains(restartBody, required) {
			t.Fatalf("provider request after restart omitted %q:\n%s", required, restartBody)
		}
	}
}

func automaticCompactionContinuationForTest(t *testing.T, thread map[string]any) map[string]any {
	t.Helper()
	for _, rawTurn := range anyList(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range anyList(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") != "compaction" || item["auto"] != true {
				continue
			}
			continuation, ok := item["taskContinuation"].(map[string]any)
			if !ok || continuation == nil {
				t.Fatalf("automatic compaction item has no task continuation: %#v", item)
			}
			return continuation
		}
	}
	t.Fatalf("thread has no automatic compaction continuation: %#v", thread)
	return nil
}
