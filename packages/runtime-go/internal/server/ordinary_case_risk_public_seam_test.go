package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type mixedCaseLongContextProviderV1 struct {
	calls                        int
	continuationSnapshotObserved bool
	oldOrdinaryHistoryObserved   bool
	protectedCaseHistoryObserved bool
}

func (*mixedCaseLongContextProviderV1) RequiresDurablePipelineStagesV1() {}

func (providerClient *mixedCaseLongContextProviderV1) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	providerClient.calls++
	if providerClient.calls == 3 {
		for _, message := range request.Messages {
			providerClient.continuationSnapshotObserved = providerClient.continuationSnapshotObserved ||
				strings.Contains(message.Content, "Analytix task continuation snapshot")
			providerClient.oldOrdinaryHistoryObserved = providerClient.oldOrdinaryHistoryObserved ||
				strings.Contains(message.Content, "ordinary history alpha")
			providerClient.protectedCaseHistoryObserved = providerClient.protectedCaseHistoryObserved ||
				strings.Contains(message.Content, strings.Repeat("案", 16))
		}
	}
	text := "MIXED_CONTEXT_CONTINUATION_OK"
	if providerClient.calls <= 2 {
		text = strings.Repeat("ordinary history alpha ", 2_000)
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: text}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true}, nil
}

type boundaryOrdinaryReadProvider struct {
	calls           int
	readPaths       []string
	readResultCount int
}

type boundaryOrdinaryWriteProvider struct {
	calls     int
	sawResult bool
}

type boundaryOrdinaryPlanProvider struct {
	calls               int
	markdown            string
	sawSuccessfulResult bool
	sawToolResult       bool
	sawProtectedTool    bool
	sawReflectedResult  bool
	forbiddenSentinels  []string
}

func (*boundaryOrdinaryPlanProvider) RequiresDurablePipelineStagesV1() {}

func (providerClient *boundaryOrdinaryPlanProvider) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	providerClient.calls++
	for _, tool := range request.Tools {
		providerClient.sawProtectedTool = providerClient.sawProtectedTool ||
			strings.Contains(tool.Name, "analytix_funds") || strings.Contains(tool.Name, "fund-analysis")
	}
	if providerClient.calls == 1 {
		arguments, err := json.Marshal(map[string]any{
			"operation":          "draft",
			"markdown":           providerClient.markdown,
			"plan_id":            "boundary-ordinary-plan",
			"plan_relative_path": ".analytixsdd/plan/boundary-ordinary.md",
			"source_request":     "Plan the ordinary code change",
			"title":              "Boundary ordinary plan",
		})
		if err != nil {
			return domainmodel.Result{}, err
		}
		call := domainmodel.ToolCall{ID: "raw-boundary-plan", Name: "create_plan", Arguments: arguments}
		if request.OnChunk != nil {
			if err := request.OnChunk(domainmodel.Chunk{
				Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: call.ID, Name: call.Name},
			}); err != nil {
				return domainmodel.Result{}, err
			}
		}
		return domainmodel.Result{
			ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat,
			Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: call}}, StreamCompleted: true,
		}, nil
	}
	for _, message := range request.Messages {
		if message.Role != "tool" || message.Name != "create_plan" {
			continue
		}
		providerClient.sawToolResult = true
		if strings.Contains(message.Content, `"relative_path":".analytixsdd/plan/boundary-ordinary.md"`) {
			providerClient.sawSuccessfulResult = true
		}
		for _, sentinel := range providerClient.forbiddenSentinels {
			providerClient.sawReflectedResult = providerClient.sawReflectedResult ||
				strings.Contains(message.Content, sentinel)
		}
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "BOUNDARY_ORDINARY_PLAN_FINAL_OK"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{
		ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat,
		Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true,
	}, nil
}

func (*boundaryOrdinaryWriteProvider) RequiresDurablePipelineStagesV1() {}

func (providerClient *boundaryOrdinaryWriteProvider) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	providerClient.calls++
	if providerClient.calls == 1 {
		call := domainmodel.ToolCall{
			ID: "raw-boundary-write", Name: "write_file",
			Arguments: json.RawMessage(`{"path":"generated.txt","content":"BOUNDARY_ORDINARY_WRITE_CONTENT\n"}`),
		}
		if request.OnChunk != nil {
			if err := request.OnChunk(domainmodel.Chunk{
				Kind:     domainmodel.ChunkToolCallStart,
				ToolCall: domainmodel.ToolCall{ID: call.ID, Name: call.Name},
			}); err != nil {
				return domainmodel.Result{}, err
			}
		}
		return domainmodel.Result{
			ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat,
			Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: call}}, StreamCompleted: true,
		}, nil
	}
	for _, message := range request.Messages {
		if message.Role == "tool" && message.Name == "write_file" {
			providerClient.sawResult = true
		}
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "BOUNDARY_ORDINARY_WRITE_FINAL_OK"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{
		ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat,
		Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true,
	}, nil
}

func (*boundaryOrdinaryReadProvider) RequiresDurablePipelineStagesV1() {}

func (providerClient *boundaryOrdinaryReadProvider) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	providerClient.calls++
	if providerClient.calls == 1 {
		result := domainmodel.Result{
			ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat, StreamCompleted: true,
			Chunks: make([]domainmodel.Chunk, 0, len(providerClient.readPaths)),
		}
		for index, path := range providerClient.readPaths {
			call := domainmodel.ToolCall{ID: "raw-boundary-read-" + string(rune('a'+index)), Name: "read", Arguments: json.RawMessage(`{"path":"` + path + `"}`)}
			if request.OnChunk != nil {
				if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: call.ID, Name: call.Name}}); err != nil {
					return domainmodel.Result{}, err
				}
			}
			result.Chunks = append(result.Chunks, domainmodel.Chunk{Kind: domainmodel.ChunkToolCall, ToolCall: call})
		}
		return result, nil
	}
	for _, message := range request.Messages {
		if message.Role == "tool" && strings.Contains(message.Content, "BOUNDARY_ORDINARY_READ_CONTENT") {
			providerClient.readResultCount++
		}
	}
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "BOUNDARY_ORDINARY_TOOL_FINAL_OK"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{
		ProviderID: request.ProviderID, EndpointFormat: request.EndpointFormat,
		Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true,
	}, nil
}

func TestOrdinaryPlanningHTTPPublicSeamDoesNotAcquireCaseRisk(t *testing.T) {
	workspace := workspacetest.New(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		ProviderID:     "ordinary-public-seam-provider",
		BaseURL:        "https://provider.invalid",
		APIKey:         "test-key",
		Model:          "ordinary-public-seam-model",
		EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	recordingProvider := &providerStepRecordingProvider{}
	handler.provider = recordingProvider
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "ordinary planning public seam", "workspace": workspace,
			"providerId": "ordinary-public-seam-provider", "model": "ordinary-public-seam-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	if threadID == "" {
		t.Fatalf("HTTP thread creation returned no thread id: %#v", thread)
	}

	prompt := strings.Join([]string{
		"Work only in this pre-existing isolated non-case code repository and inspect the task through tools.",
		"The user request is: Update Express content-type normalization so the reserved HTTP quality parameter name is handled case-insensitively. Add a focused regression test covering both lowercase and uppercase quality parameter names while preserving ordinary parameters, then run the focused real project test.",
		`Call read exactly once for each contract-required task inspection path, with only its relative path and no optional offset or limit: "lib/utils.js", "test/utils.js", "package.json".`,
		"Do not read the contract-bound long-context file in this planning turn; the acceptance harness validates it in a dedicated later continuation.",
		"Use no tools other than those exact read calls and one create_plan call.",
		"Do not call list, search, code index, web fetch, bash, Todo, subagent/delegation, skill, MCP, or read any other path.",
		"Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call.",
		"Complete those exact read calls before recording the plan.",
		"Call create_plan exactly once after those reads complete.",
		"Record a concrete implementation and verification plan at the GUI-reserved plan path.",
		"After create_plan returns successfully, end the turn immediately without another tool call.",
		"If every required read succeeds, do not send a final response or the completion marker until create_plan has returned successfully.",
		"In the final response include the exact marker MILESTONE_A_PLAN_OK.",
		"Do not edit files, run the test, delegate, or run /compact in this planning turn.",
	}, " ")
	started := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{"prompt": prompt})),
		http.StatusAccepted,
	)
	turnID := stringField(started, "turnId")
	if turnID == "" {
		t.Fatalf("HTTP turn start returned no turn id: %#v", started)
	}

	requests := recordingProvider.Requests()
	if len(requests) != 1 {
		t.Fatalf("ordinary planning provider calls=%d want=1", len(requests))
	}
	if requests[0].PrivateProviderTelemetry == nil || !requests[0].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("ordinary planning request acquired a protected effect: %#v", requests[0].PrivateProviderTelemetry)
	}
	if got := lastProviderStepUserPrompt(requests[0].Messages); got != prompt {
		t.Fatalf("ordinary planning provider prompt changed: %q", got)
	}
	if providerStepRequestHasTool(requests[0], providerStepFundsTool) {
		t.Fatalf("unbound ordinary planning advertised funds authority: %#v", requests[0].Tools)
	}

	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, found := appmodel.TurnByID(rawThread, turnID)
	if !found || stringField(turn, "status") != "completed" {
		t.Fatalf("ordinary planning turn did not complete: %#v", turn)
	}
	publicThread := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK,
	)
	publicBody, err := json.Marshal(publicThread)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(publicBody, []byte("provider-step-candidate-1")) {
		t.Fatalf("ordinary planning public projection lost the provider result: %s", publicBody)
	}
}

func TestUnboundCaseBoundaryHTTPPublicSeamAllowsLaterOrdinaryContinuation(t *testing.T) {
	workspace := workspacetest.New(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "unbound-boundary-ordinary-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "unbound-boundary-ordinary-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	providerClient := &providerStepRecordingProvider{}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "unbound boundary ordinary continuation", "workspace": workspace,
			"providerId": "unbound-boundary-ordinary-provider", "model": "unbound-boundary-ordinary-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	blocked := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt": "请分析当前案件账户的资金流入、流出和净额。",
		})),
		http.StatusAccepted,
	)
	if len(providerClient.Requests()) != 0 {
		t.Fatal("unbound protected turn reached the provider")
	}

	continued := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt": "Read the current ordinary repository branch and summarize it concisely.",
		})),
		http.StatusAccepted,
	)
	if requests := providerClient.Requests(); len(requests) != 1 ||
		requests[0].PrivateProviderTelemetry == nil || !requests[0].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("later ordinary continuation did not use exactly one ordinary provider attempt: requests=%d", len(requests))
	}

	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := map[string]map[string]any{}
	for _, raw := range listAny(rawThread["turns"]) {
		turn, _ := raw.(map[string]any)
		turns[stringField(turn, "id")] = turn
	}
	blockedTurn := turns[stringField(blocked, "turnId")]
	blockedFinal, _ := blockedTurn["acceptedFinal"].(map[string]any)
	if stringField(blockedTurn, "status") != "completed" ||
		stringField(blockedFinal, "terminalReason") != "source_unavailable" {
		t.Fatalf("unbound protected turn lost its fixed boundary: status=%q reason=%q",
			stringField(blockedTurn, "status"), stringField(blockedFinal, "terminalReason"))
	}
	continuedTurn := turns[stringField(continued, "turnId")]
	continuedFinal, _ := continuedTurn["acceptedFinal"].(map[string]any)
	continuedText := acceptedFinalAssistantText(continuedTurn)
	if stringField(continuedTurn, "status") != "completed" ||
		stringField(continuedFinal, "terminalReason") != "success" ||
		continuedText != "provider-step-candidate-1" ||
		strings.Contains(continuedText, apploop.CaseFundUnverifiedFinalAnswer()) ||
		strings.Contains(continuedText, "资金") || strings.Contains(continuedText, "案件") {
		t.Fatalf("later ordinary continuation was not published as an exact ordinary result: status=%q reason=%q final=%q",
			stringField(continuedTurn, "status"), stringField(continuedFinal, "terminalReason"),
			continuedText)
	}
}

func TestMixedCaseLongContextHTTPPublicSeamCompactsBeforeOrdinaryContinuation(t *testing.T) {
	workspace := workspacetest.New(t)
	providerID := "mixed-case-long-context-provider"
	largeModel := "mixed-case-large-context"
	smallModel := "mixed-case-small-context"
	modelProviders, err := json.Marshal(map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id": providerID, "apiKey": "test-key", "baseUrl": "https://provider.invalid",
			"endpointFormat": "chat_completions", "models": []string{largeModel, smallModel},
			"modelProfiles": map[string]any{
				largeModel: map[string]any{"contextWindowTokens": 200_000},
				smallModel: map[string]any{"contextWindowTokens": 20_000},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: providerID, BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: largeModel, EndpointFormat: "chat_completions", ModelProvidersJSON: string(modelProviders),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	providerClient := &mixedCaseLongContextProviderV1{}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "mixed case long context", "workspace": workspace,
			"providerId": providerID, "model": largeModel,
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	for _, prompt := range []string{
		"Summarize the ordinary repository status.",
		"List one safe next step for the ordinary repository.",
	} {
		requestThreadSummaryJSON(
			t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{"prompt": prompt, "model": largeModel})),
			http.StatusAccepted,
		)
	}
	if providerClient.calls != 2 {
		t.Fatalf("ordinary setup provider calls=%d want=2", providerClient.calls)
	}

	protectedPrompt := "请分析当前案件账户的资金流入、流出和净额。" + strings.Repeat("案", 1_000)
	var protected map[string]any
	for range 16 {
		protected = requestThreadSummaryJSON(
			t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
			bytes.NewReader(caseIngressJSONV1(t, map[string]any{
				"prompt": protectedPrompt, "model": largeModel,
			})),
			http.StatusAccepted,
		)
	}
	if providerClient.calls != 2 {
		t.Fatalf("protected source-unavailable turn reached provider: requests=%d", providerClient.calls)
	}

	continued := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt": "Read the ordinary repository status and answer briefly.", "model": smallModel,
		})),
		http.StatusAccepted,
	)
	if providerClient.calls != 3 {
		t.Fatalf("small ordinary continuation provider calls=%d want=3", providerClient.calls)
	}
	if !providerClient.continuationSnapshotObserved || providerClient.oldOrdinaryHistoryObserved ||
		providerClient.protectedCaseHistoryObserved {
		t.Fatalf("compacted ordinary lane projection is invalid: continuation=%t oldOrdinary=%t protectedCase=%t",
			providerClient.continuationSnapshotObserved, providerClient.oldOrdinaryHistoryObserved,
			providerClient.protectedCaseHistoryObserved)
	}

	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := map[string]map[string]any{}
	for _, raw := range listAny(rawThread["turns"]) {
		turn, _ := raw.(map[string]any)
		turns[stringField(turn, "id")] = turn
	}
	protectedTurn := turns[stringField(protected, "turnId")]
	protectedFinal, _ := protectedTurn["acceptedFinal"].(map[string]any)
	if stringField(protectedTurn, "status") != "completed" ||
		stringField(protectedFinal, "terminalReason") != "source_unavailable" {
		t.Fatalf("protected source-unavailable boundary changed: status=%q reason=%q",
			stringField(protectedTurn, "status"), stringField(protectedFinal, "terminalReason"))
	}
	continuedTurn := turns[stringField(continued, "turnId")]
	continuedFinal, _ := continuedTurn["acceptedFinal"].(map[string]any)
	if stringField(continuedTurn, "status") != "completed" ||
		stringField(continuedFinal, "terminalReason") != "success" ||
		!strings.Contains(acceptedFinalAssistantText(continuedTurn), "MIXED_CONTEXT_CONTINUATION_OK") {
		t.Fatalf("small ordinary continuation did not publish its accepted final: status=%q reason=%q final=%q",
			stringField(continuedTurn, "status"), stringField(continuedFinal, "terminalReason"),
			acceptedFinalAssistantText(continuedTurn))
	}

	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	compactionIndex := -1
	automaticCompactionCount := 0
	continuationStartedIndex := -1
	for index, event := range replay.Events {
		switch stringField(event, "kind") {
		case "compaction_completed":
			if event["auto"] == true {
				automaticCompactionCount++
				compactionIndex = index
			}
		case "turn_started":
			if stringField(event, "turnId") == stringField(continued, "turnId") {
				continuationStartedIndex = index
			}
		}
	}
	if automaticCompactionCount != 1 || compactionIndex < 0 || continuationStartedIndex < 0 || compactionIndex >= continuationStartedIndex {
		t.Fatalf("exactly one automatic compaction did not durably precede continuation admission: count=%d compaction=%d started=%d",
			automaticCompactionCount, compactionIndex, continuationStartedIndex)
	}
}

func TestContextHardLimitHTTPPublicSeamEmitsHostAdmissionDiagnostic(t *testing.T) {
	workspace := workspacetest.New(t)
	providerID := "context-admission-provider"
	modelID := "context-admission-small-model"
	modelProviders, err := json.Marshal(map[string]any{
		"defaultProviderId": providerID,
		"providers": []map[string]any{{
			"id": providerID, "apiKey": "test-key", "baseUrl": "https://provider.invalid",
			"endpointFormat": "chat_completions", "models": []string{modelID},
			"modelProfiles": map[string]any{
				modelID: map[string]any{"contextWindowTokens": 1_000},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: providerID, BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: modelID, EndpointFormat: "chat_completions", ModelProvidersJSON: string(modelProviders),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	providerClient := &mixedCaseLongContextProviderV1{}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "context admission diagnostic", "workspace": workspace,
			"providerId": providerID, "model": modelID,
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	prompt := strings.Repeat("Continue the bounded ordinary repository task. ", 250)
	failureResponse := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{"prompt": prompt, "model": modelID})),
		http.StatusInternalServerError,
	)
	if providerClient.calls != 0 {
		t.Fatalf("hard-limit request reached provider: calls=%d", providerClient.calls)
	}

	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(rawThread["turns"])
	if len(turns) != 1 {
		t.Fatalf("hard-limit admission durable turn count=%d want=1", len(turns))
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "status") != "failed" {
		t.Fatalf("hard-limit turn status=%q want=failed", stringField(turn, "status"))
	}
	replay, err := handler.store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	admissionEvents := 0
	terminalObserved := false
	var admissionEvent map[string]any
	for _, event := range replay.Events {
		if stringField(event, "stage") == "provider_error" {
			t.Fatal("host context admission was misreported as provider transport failure")
		}
		if stringField(event, "stage") == "provider_admission_rejected" {
			admissionEvents++
			admissionEvent = event
			details, _ := event["details"].(map[string]any)
			attemptCount, attemptCountOK := contracts.NumericSeq(details["providerAttemptCount"])
			if stringField(details, "reasonCode") != "context_window_hard_limit" ||
				!attemptCountOK || attemptCount != 0 || len(details) != 4 {
				t.Fatalf("hard-limit diagnostic is not closed: %#v", event)
			}
		}
		if stringField(event, "kind") == "turn_failed" &&
			stringField(event, "terminalReason") == "semantic_failure" &&
			stringField(event, "code") == "context_window_hard_limit" {
			terminalObserved = true
		}
	}
	if admissionEvents == 0 {
		t.Fatalf("host context admission diagnostic missing: response=%#v", failureResponse)
	}
	if admissionEvents != 1 || !terminalObserved {
		t.Fatalf("host admission or semantic terminal missing: admissions=%d terminal=%t responseCode=%q responseReason=%q",
			admissionEvents, terminalObserved, stringField(failureResponse, "code"), stringField(failureResponse, "reasonCode"))
	}
	diagnosticBody, err := json.Marshal(admissionEvent)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(diagnosticBody, []byte("/Users/")) ||
		bytes.Contains(diagnosticBody, []byte("projectedRequest"+"Body")) {
		t.Fatalf("open request or local path entered admission diagnostics: %s", diagnosticBody)
	}
}

func TestBoundaryCaseHTTPPublicSeamRunsOrdinarySingleAndBatchReads(t *testing.T) {
	for _, readPaths := range [][]string{{"README.md"}, {"README.md", "notes.txt"}} {
		name := "single"
		if len(readPaths) == 2 {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			workspace := writeThreadMutationCaseBinding(t)
			for _, path := range readPaths {
				if err := os.WriteFile(filepath.Join(workspace, path), []byte("BOUNDARY_ORDINARY_READ_CONTENT "+path+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			durableRoot := t.TempDir()
			handler := NewRuntimeServerHandler(RuntimeServerConfig{
				RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
				ProviderID: "boundary-ordinary-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
				Model: "boundary-ordinary-model", EndpointFormat: "chat_completions",
			}).(*runtimeServerHandler)
			configureServerGeneralExecution(t, handler)
			configureSteerTestCaseAuthorities(t, handler, durableRoot)
			providerClient := &boundaryOrdinaryReadProvider{readPaths: readPaths}
			handler.provider = providerClient
			server := httptest.NewServer(handler)
			defer server.Close()

			thread := requestThreadSummaryJSON(
				t, server.URL, http.MethodPost, "/v1/threads",
				bytes.NewReader(caseIngressJSONV1(t, map[string]any{
					"title": "boundary ordinary tool public seam", "workspace": workspace,
					"providerId": "boundary-ordinary-provider", "model": "boundary-ordinary-model",
				})),
				http.StatusCreated,
			)
			threadID := stringField(thread, "id")
			prompt := "Read the requested ordinary project files and summarize them; also analyze the current case account flows, which must fail closed if its snapshot is unavailable."
			started := requestThreadSummaryJSON(
				t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
				bytes.NewReader(caseIngressJSONV1(t, map[string]any{
					"prompt": prompt, "riskIntent": "case", "approvalPolicy": "never", "sandboxMode": "workspace-write",
				})),
				http.StatusAccepted,
			)
			turnID := stringField(started, "turnId")
			if providerClient.calls != 2 || providerClient.readResultCount != len(readPaths) {
				t.Fatalf("ordinary boundary tool flow did not reach provider continuation: calls=%d results=%d want=%d", providerClient.calls, providerClient.readResultCount, len(readPaths))
			}
			rawThread, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			turn, found := appmodel.TurnByID(rawThread, turnID)
			final := acceptedFinalAssistantText(turn)
			if !found || stringField(turn, "status") != "completed" ||
				!strings.Contains(final, "BOUNDARY_ORDINARY_TOOL_FINAL_OK") ||
				!strings.Contains(final, apploop.CaseFundSourceUnavailableAnswer()) {
				t.Fatalf("ordinary boundary tool turn did not publish its additive final: found=%t status=%q", found, stringField(turn, "status"))
			}
		})
	}
}

func TestBoundaryCaseHTTPPublicSeamRunsOrdinaryWrite(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "boundary-write-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "boundary-write-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	providerClient := &boundaryOrdinaryWriteProvider{}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "boundary ordinary write public seam", "workspace": workspace,
			"providerId": "boundary-write-provider", "model": "boundary-write-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	started := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "Write generated.txt in the ordinary project workspace; also analyze the current case account flows, which must fail closed if its snapshot is unavailable.",
			"riskIntent": "case", "approvalPolicy": "auto", "sandboxMode": "workspace-write",
		})),
		http.StatusAccepted,
	)
	if providerClient.calls != 2 || !providerClient.sawResult {
		t.Fatalf("ordinary boundary write did not reach provider continuation: calls=%d result=%t", providerClient.calls, providerClient.sawResult)
	}
	body, err := os.ReadFile(filepath.Join(workspace, "generated.txt"))
	if err != nil || string(body) != "BOUNDARY_ORDINARY_WRITE_CONTENT\n" {
		t.Fatalf("ordinary boundary write effect was not durable: bytes=%d err=%v", len(body), err)
	}
	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, found := appmodel.TurnByID(rawThread, stringField(started, "turnId"))
	final := acceptedFinalAssistantText(turn)
	if !found || stringField(turn, "status") != "completed" ||
		!strings.Contains(final, "BOUNDARY_ORDINARY_WRITE_FINAL_OK") ||
		!strings.Contains(final, apploop.CaseFundSourceUnavailableAnswer()) {
		t.Fatalf("ordinary boundary write did not publish its additive final: found=%t status=%q provider=%t boundary=%t", found, stringField(turn, "status"), strings.Contains(final, "BOUNDARY_ORDINARY_WRITE_FINAL_OK"), strings.Contains(final, apploop.CaseFundSourceUnavailableAnswer()))
	}
}

func TestBoundaryCaseHTTPPublicSeamRunsOrdinaryCreatePlan(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "boundary-plan-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "boundary-plan-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	const planMarkdown = "# Ordinary implementation plan\n\n1. Update the parser.\n2. Run the focused Go test."
	providerClient := &boundaryOrdinaryPlanProvider{markdown: planMarkdown}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "boundary ordinary plan public seam", "workspace": workspace,
			"providerId": "boundary-plan-provider", "model": "boundary-plan-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	started := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "update files a, b, and c; query current-case funds",
			"riskIntent": "case", "mode": "plan", "approvalPolicy": "never", "sandboxMode": "workspace-write",
			"guiPlan": map[string]any{
				"operation": "draft", "workspaceRoot": workspace,
				"relativePath": ".analytixsdd/plan/boundary-ordinary.md",
				"planId":       "boundary-ordinary-plan", "sourceRequest": "Plan the ordinary code change",
				"title": "Boundary ordinary plan",
			},
		})),
		http.StatusAccepted,
	)
	if providerClient.calls != 2 || !providerClient.sawSuccessfulResult {
		t.Fatalf("ordinary boundary create_plan did not reach a successful provider continuation: calls=%d result=%t",
			providerClient.calls, providerClient.sawSuccessfulResult)
	}
	planPath := filepath.Join(workspace, ".analytixsdd", "plan", "boundary-ordinary.md")
	if body, err := os.ReadFile(planPath); err != nil || string(body) != planMarkdown {
		t.Fatalf("ordinary boundary create_plan effect was not exact: body=%q err=%v", body, err)
	}
	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, found := appmodel.TurnByID(rawThread, stringField(started, "turnId"))
	securityContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if contextErr != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
		domainsecurity.TurnSecurityContextIsGeneral(securityContext) ||
		domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(securityContext) != nil {
		t.Fatalf("ordinary create_plan changed boundary authority: context=%#v err=%v", securityContext, contextErr)
	}
	final := acceptedFinalAssistantText(turn)
	if !found || stringField(turn, "status") != "completed" ||
		!strings.Contains(final, "BOUNDARY_ORDINARY_PLAN_FINAL_OK") ||
		!strings.Contains(final, apploop.CaseFundSourceUnavailableAnswer()) {
		t.Fatalf("ordinary boundary create_plan did not publish its additive final: found=%t status=%q provider=%t boundary=%t",
			found, stringField(turn, "status"), strings.Contains(final, "BOUNDARY_ORDINARY_PLAN_FINAL_OK"),
			strings.Contains(final, apploop.CaseFundSourceUnavailableAnswer()))
	}
	if providerClient.sawProtectedTool || !providerClient.sawToolResult {
		t.Fatalf("ordinary boundary create_plan acquired a protected tool or lost its typed result: protected=%t result=%t",
			providerClient.sawProtectedTool, providerClient.sawToolResult)
	}
}

func TestBoundaryCaseHTTPPublicSeamRejectsProtectedCreatePlanWithoutReflection(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "boundary-unsafe-plan-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "boundary-unsafe-plan-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	forbidden := []string{
		"张某实际控制甲公司",
	}
	providerClient := &boundaryOrdinaryPlanProvider{
		markdown: strings.Join([]string{
			"# Unsafe plan candidate",
			forbidden[0],
		}, "\n"),
		forbiddenSentinels: forbidden,
	}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "boundary protected plan public seam", "workspace": workspace,
			"providerId": "boundary-unsafe-plan-provider", "model": "boundary-unsafe-plan-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	started := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "update files a, b, and c; query current-case funds",
			"riskIntent": "case", "mode": "plan", "approvalPolicy": "never", "sandboxMode": "workspace-write",
			"guiPlan": map[string]any{
				"operation": "draft", "workspaceRoot": workspace,
				"relativePath": ".analytixsdd/plan/boundary-ordinary.md",
				"planId":       "boundary-ordinary-plan", "sourceRequest": "Plan the ordinary code change",
				"title": "Boundary ordinary plan",
			},
		})),
		http.StatusAccepted,
	)
	if providerClient.calls != 2 || !providerClient.sawToolResult || providerClient.sawSuccessfulResult ||
		providerClient.sawProtectedTool || providerClient.sawReflectedResult {
		t.Fatalf("protected boundary create_plan did not fail closed: calls=%d result=%t success=%t protected=%t reflected=%t",
			providerClient.calls, providerClient.sawToolResult, providerClient.sawSuccessfulResult,
			providerClient.sawProtectedTool, providerClient.sawReflectedResult)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "boundary-ordinary.md")); !os.IsNotExist(err) {
		t.Fatalf("protected boundary create_plan produced a file side effect: err=%v", err)
	}
	rawThread, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, found := appmodel.TurnByID(rawThread, stringField(started, "turnId"))
	securityContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if !found || contextErr != nil || !domainsecurity.TurnSecurityContextIsBoundaryOnly(securityContext) ||
		domainsecurity.TurnSecurityContextIsGeneral(securityContext) ||
		domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("protected create_plan changed boundary authority: found=%t context=%#v err=%v", found, securityContext, contextErr)
	}
	publicFailure := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusInternalServerError,
	)
	publicBody, err := json.Marshal(publicFailure)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range forbidden {
		if bytes.Contains(publicBody, []byte(sentinel)) {
			t.Fatalf("protected create_plan candidate reflected through the public seam: sentinel=%q body=%s", sentinel, publicBody)
		}
	}
	final := acceptedFinalAssistantText(turn)
	if stringField(turn, "status") != "completed" ||
		!strings.Contains(final, "BOUNDARY_ORDINARY_PLAN_FINAL_OK") ||
		!strings.Contains(final, apploop.CaseFundSourceUnavailableAnswer()) {
		t.Fatalf("blocked plan lost the additive ordinary/case boundary final: status=%q final=%q",
			stringField(turn, "status"), final)
	}
}

func TestBoundaryCaseHTTPPublicSeamRejectsPrivatePlanArguments(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "boundary-private-plan-provider", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "boundary-private-plan-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	forbidden := []string{
		"6222021234567890123",
		"/Users/private/case-source.json",
		"cer1_" + strings.Repeat("a", 64),
		"RAW_PROVIDER_BODY_BOUNDARY_PLAN",
		"RAW_TOOL_BODY_BOUNDARY_PLAN",
	}
	providerClient := &boundaryOrdinaryPlanProvider{
		markdown: strings.Join([]string{
			"# Unsafe private body plan candidate",
			`providerBody: {"account":"` + forbidden[0] + `","sourceExactValue":"` + forbidden[1] + `","raw":"` + forbidden[3] + `"}`,
			`toolBody: {"purpose":"analytix.source-row-record/v1","authorityEntityRef":"` + forbidden[2] + `","raw":"` + forbidden[4] + `"}`,
		}, "\n"),
		forbiddenSentinels: forbidden,
	}
	handler.provider = providerClient
	server := httptest.NewServer(handler)
	defer server.Close()

	thread := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"title": "boundary private plan public seam", "workspace": workspace,
			"providerId": "boundary-private-plan-provider", "model": "boundary-private-plan-model",
		})),
		http.StatusCreated,
	)
	threadID := stringField(thread, "id")
	failure := requestThreadSummaryJSON(
		t, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		bytes.NewReader(caseIngressJSONV1(t, map[string]any{
			"prompt":     "update files a, b, and c; query current-case funds",
			"riskIntent": "case", "mode": "plan", "approvalPolicy": "never", "sandboxMode": "workspace-write",
			"guiPlan": map[string]any{
				"operation": "draft", "workspaceRoot": workspace,
				"relativePath": ".analytixsdd/plan/boundary-ordinary.md",
				"planId":       "boundary-ordinary-plan", "sourceRequest": "Plan the ordinary code change",
				"title": "Boundary ordinary plan",
			},
		})),
		http.StatusInternalServerError,
	)
	if stringField(failure, "reasonCode") != "tool_private_arguments" || providerClient.calls != 1 ||
		providerClient.sawToolResult || providerClient.sawSuccessfulResult || providerClient.sawProtectedTool ||
		providerClient.sawReflectedResult {
		t.Fatalf("raw provider/tool plan body did not fail at the private-argument boundary: response=%#v calls=%d result=%t success=%t protected=%t reflected=%t",
			failure, providerClient.calls, providerClient.sawToolResult, providerClient.sawSuccessfulResult,
			providerClient.sawProtectedTool, providerClient.sawReflectedResult)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytixsdd", "plan", "boundary-ordinary.md")); !os.IsNotExist(err) {
		t.Fatalf("raw provider/tool plan body produced a file side effect: err=%v", err)
	}
	publicFailure := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusInternalServerError,
	)
	publicBody, err := json.Marshal(publicFailure)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range forbidden {
		if bytes.Contains(publicBody, []byte(sentinel)) {
			t.Fatalf("raw provider/tool plan body reflected through the public seam: sentinel=%q body=%s", sentinel, publicBody)
		}
	}
}
