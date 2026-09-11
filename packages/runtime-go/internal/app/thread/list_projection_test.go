package thread

import "testing"

func TestIncludeThreadInListAppliesRelationArchiveAndSearch(t *testing.T) {
	thread := map[string]any{
		"id":       "thr_1",
		"title":    "Plain",
		"status":   "idle",
		"relation": "primary",
		"turns": []any{
			map[string]any{"prompt": "needle prompt"},
		},
	}
	summary := map[string]any{"id": "thr_1", "title": "Plain", "updatedAt": "2026-01-01T00:00:00Z"}
	if !IncludeThreadInList(thread, summary, ListProjectionFilter{Search: "needle"}) {
		t.Fatal("expected prompt search to include thread")
	}
	if IncludeThreadInList(thread, summary, ListProjectionFilter{Search: "missing"}) {
		t.Fatal("expected missing search to exclude thread")
	}
	if !IncludeThreadInList(thread, summary, ListProjectionFilter{Search: "missing", EventLogMatched: true}) {
		t.Fatal("expected eventlog match to include thread")
	}
	thread["relation"] = "side"
	if IncludeThreadInList(thread, summary, ListProjectionFilter{IncludeSide: false}) {
		t.Fatal("expected side thread to be hidden by default")
	}
	thread["relation"] = "primary"
	thread["parentThreadId"] = "thr_parent"
	if IncludeThreadInList(thread, summary, ListProjectionFilter{IncludeSide: false}) {
		t.Fatal("expected legacy child thread to be hidden by default")
	}
	thread["relation"] = "fork"
	if !IncludeThreadInList(thread, summary, ListProjectionFilter{IncludeSide: false}) {
		t.Fatal("expected response fork to remain visible by default")
	}
	delete(thread, "parentThreadId")
	thread["relation"] = "primary"
	thread["status"] = "archived"
	if IncludeThreadInList(thread, summary, ListProjectionFilter{}) {
		t.Fatal("expected archived thread to be hidden by default")
	}
	if !IncludeThreadInList(thread, summary, ListProjectionFilter{IncludeArchived: true}) {
		t.Fatal("expected includeArchived to include archived thread")
	}
	if !IncludeThreadInList(thread, summary, ListProjectionFilter{ArchivedOnly: true}) {
		t.Fatal("expected archivedOnly to include archived thread")
	}
}

func TestListIncludesSideParsesCommaSeparatedInclude(t *testing.T) {
	if !ListIncludesSide("usage, side ,history") {
		t.Fatal("expected include list to enable side threads")
	}
	if ListIncludesSide("usage,history") {
		t.Fatal("did not expect side threads without side include")
	}
}

func TestCaseThreadSearchUsesOnlyClosedLifecycleMetadata(t *testing.T) {
	canaries := []string{
		"CASE_SOURCE_EXACT_CANARY",
		"/private/case/source.duckdb",
		"SELECT * FROM source_rows WHERE account = ?",
		`{"tool":"read","arguments":{"path":"/private/evidence.csv"}}`,
		"cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"acct:17",
		"6222021234567890123",
	}
	thread := map[string]any{
		"id":               "thread-case-search",
		"title":            canaries[0],
		"workspace":        canaries[1],
		"status":           "idle",
		"historyAuthority": "case_boundary_only_v1",
		"turns": []any{map[string]any{
			"id": "turn-case-search",
			"items": []any{
				map[string]any{"kind": "user_message", "text": canaries[2]},
				map[string]any{"kind": "tool_call", "arguments": canaries[3]},
				map[string]any{"kind": "assistant_text", "text": canaries[4] + " " + canaries[5] + " " + canaries[6]},
			},
		}},
	}
	summary := map[string]any{
		"id": thread["id"], "title": thread["title"], "workspace": thread["workspace"], "status": "idle",
	}
	for _, canary := range canaries {
		if ThreadMatchesSearch(thread, summary, canary) {
			t.Fatalf("case private canary became searchable: %q", canary)
		}
	}
	if !ThreadMatchesSearch(thread, summary, "idle") {
		t.Fatal("closed case lifecycle metadata was not searchable")
	}
}

func TestSortThreadSummariesOrdersByUpdatedAtThenID(t *testing.T) {
	threads := []map[string]any{
		{"id": "thr_a", "updatedAt": "2026-01-01T00:00:00Z"},
		{"id": "thr_b", "updatedAt": "2026-01-02T00:00:00Z"},
		{"id": "thr_c", "updatedAt": "2026-01-02T00:00:00Z"},
	}
	SortThreadSummaries(threads)
	if stringField(threads[0], "id") != "thr_c" || stringField(threads[1], "id") != "thr_b" || stringField(threads[2], "id") != "thr_a" {
		t.Fatalf("unexpected order: %#v", threads)
	}
}
