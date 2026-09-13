package server

import (
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestThreadSearchHTTPUsesClosedCaseLifecycleProjectionAndKeepsOrdinaryTextAdditive(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	t.Cleanup(func() { handler.runtimeSubagentState().CancelBackgroundJobsAndWait(2 * time.Second) })
	server := httptest.NewServer(handler)
	defer server.Close()

	caseWorkspace := workspacetest.New(t)
	caseThread, err := handler.store.CreateThread(map[string]any{
		"title":     "SEARCH_CASE_TITLE_SENTINEL",
		"workspace": caseWorkspace,
	}, caseWorkspace)
	if err != nil {
		t.Fatalf("create case thread: %v", err)
	}
	caseThreadID := stringField(caseThread, "id")
	caseTurnID := "turn_search_case"
	if err := handler.store.AppendTurnToThread(caseThreadID, map[string]any{
		"id":                    caseTurnID,
		"threadId":              caseThreadID,
		"status":                "completed",
		"caseHistoryProjection": "case_boundary_only_v1",
		"sourceExactProse":      "SEARCH_SOURCE_EXACT_SENTINEL",
		"items": []any{
			map[string]any{
				"id":       "item_search_user",
				"kind":     "user_message",
				"threadId": caseThreadID,
				"turnId":   caseTurnID,
				"text":     "SEARCH_USER_SENTINEL 赵敏与钱志强系夫妻",
			},
			map[string]any{
				"id":       "item_search_call",
				"kind":     "tool_call",
				"threadId": caseThreadID,
				"turnId":   caseTurnID,
				"arguments": map[string]any{
					"query": "SEARCH_TOOL_ARGUMENT_SENTINEL",
				},
			},
			map[string]any{
				"id":       "item_search_result",
				"kind":     "tool_result",
				"threadId": caseThreadID,
				"turnId":   caseTurnID,
				"output": map[string]any{
					"rawBody": "SEARCH_TOOL_RESULT_SENTINEL",
				},
			},
			map[string]any{
				"id":                       "item_search_final",
				"kind":                     "assistant_text",
				"threadId":                 caseThreadID,
				"turnId":                   caseTurnID,
				"acceptedFinalDigest":      "SEARCH_ACCEPTED_FINAL_SENTINEL",
				"publicationCommitId":      "SEARCH_PUBLICATION_COMMIT_SENTINEL",
				"publicationPayloadDigest": "SEARCH_PUBLICATION_PAYLOAD_SENTINEL",
			},
		},
		"privatePerson": "赵敏",
		"relationship":  "夫妻",
		"privatePath":   "/Users/private/cases/search.csv",
		"authorityRef":  "SEARCH_AUTHORITY_REF_SENTINEL",
	}, "", nil); err != nil {
		t.Fatalf("append case turn: %v", err)
	}

	ordinaryWorkspace := workspacetest.New(t)
	ordinaryThread, err := handler.store.CreateThread(map[string]any{
		"title":     "Ordinary additive title",
		"workspace": ordinaryWorkspace,
	}, ordinaryWorkspace)
	if err != nil {
		t.Fatalf("create ordinary thread: %v", err)
	}
	ordinaryThreadID := stringField(ordinaryThread, "id")
	if err := handler.store.AppendTurnToThread(ordinaryThreadID, map[string]any{
		"id":       "turn_search_ordinary",
		"threadId": ordinaryThreadID,
		"status":   "completed",
		"items": []any{
			map[string]any{
				"id":       "item_search_ordinary_user",
				"kind":     "user_message",
				"threadId": ordinaryThreadID,
				"turnId":   "turn_search_ordinary",
				"text":     "ordinary additive body",
			},
		},
	}, "", nil); err != nil {
		t.Fatalf("append ordinary turn: %v", err)
	}

	for _, hostile := range []string{
		"SEARCH_CASE_TITLE_SENTINEL",
		caseWorkspace,
		"SEARCH_USER_SENTINEL",
		"SEARCH_TOOL_ARGUMENT_SENTINEL",
		"SEARCH_TOOL_RESULT_SENTINEL",
		"SEARCH_ACCEPTED_FINAL_SENTINEL",
		"SEARCH_SOURCE_EXACT_SENTINEL",
		"赵敏",
		"夫妻",
		"/Users/private/cases/search.csv",
		"SEARCH_AUTHORITY_REF_SENTINEL",
	} {
		response := requestThreadSummaryJSON(
			t, server.URL, http.MethodGet, "/v1/threads?search="+url.QueryEscape(hostile), nil, http.StatusOK,
		)
		if threads := listAny(response["threads"]); len(threads) != 0 {
			t.Fatalf("case-private search value %q became a membership oracle: %#v", hostile, threads)
		}
	}

	lifecycle := requestThreadSummaryJSON(
		t, server.URL, http.MethodGet, "/v1/threads?search=running", nil, http.StatusOK,
	)
	if threads := listAny(lifecycle["threads"]); len(threads) != 2 {
		t.Fatalf("closed lifecycle search must retain both running threads: %#v", threads)
	}
	for _, ordinary := range []string{"Ordinary additive title", "ordinary additive body"} {
		response := requestThreadSummaryJSON(
			t, server.URL, http.MethodGet, "/v1/threads?search="+url.QueryEscape(ordinary), nil, http.StatusOK,
		)
		threads := listAny(response["threads"])
		if len(threads) != 1 || stringField(threads[0].(map[string]any), "id") != ordinaryThreadID {
			t.Fatalf("ordinary search %q was not additive: %#v", ordinary, threads)
		}
	}
}
