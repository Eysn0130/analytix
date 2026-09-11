package eventlog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
)

func TestMigrateOrdinaryToolPayloadsClosesEveryDurableProjectionIdempotently(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-tool-payload"
	contextDigest := strings.Repeat("a", 64)
	grantID := strings.Repeat("b", 64)
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	call := map[string]any{
		"id": "call-item", "threadId": threadID, "turnId": "turn-1", "kind": "tool_call", "role": "assistant",
		"status": "completed", "toolName": "mcp__funds__query", "callId": "call-1",
		"arguments": map[string]any{"account": "RAW_TOOL_ARGUMENT_SENTINEL"},
	}
	result := map[string]any{
		"id": "result-item", "threadId": threadID, "turnId": "turn-1", "kind": "tool_result", "role": "tool",
		"status": "completed", "toolName": "mcp__funds__query", "callId": "call-1", "isError": false,
		"output":        map[string]any{"account": "RAW_TOOL_RESULT_SENTINEL", "amount": float64(2645472)},
		"media":         []any{map[string]any{"path": "/private/RAW_TOOL_MEDIA_SENTINEL.png"}},
		"contextDigest": contextDigest, "contextEpoch": float64(7), "executionGrantId": grantID,
		"hostEvidenceSettlement": map[string]any{"settlementId": "host-only-settlement"},
	}
	thread := map[string]any{
		"id": threadID, "title": "tool migration", "turns": []any{map[string]any{
			"id": "turn-1", "threadId": threadID, "status": "completed", "items": []any{call, result},
		}},
	}
	writeMigrationFixtureJSON(t, filepath.Join(threadDir, "thread.json"), thread)
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "messages.jsonl"), []map[string]any{call, result})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), []map[string]any{{
		"kind": "thread_metadata", "timestamp": "2026-07-16T00:00:00Z", "thread": thread,
	}})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "events.jsonl"), []map[string]any{
		{"kind": "item_started", "threadId": threadID, "turnId": "turn-1", "seq": float64(1), "item": call},
		{"kind": "item_completed", "threadId": threadID, "turnId": "turn-1", "seq": float64(2), "item": result},
		{"kind": "tool_result", "threadId": threadID, "turnId": "turn-1", "seq": float64(3), "output": "RAW_TOP_LEVEL_TOOL_SENTINEL"},
	})
	indexPath := filepath.Join(root, "thread_summaries.jsonl")
	writeMigrationFixtureJSONL(t, indexPath, []map[string]any{{
		"schemaVersion": float64(1), "threadId": threadID, "summary": map[string]any{"lastItem": result},
	}})

	input := OrdinaryToolPayloadMigrationInput{Root: root, ThreadSummaryIndexPath: indexPath}
	if err := MigrateOrdinaryToolPayloads(input); err != nil {
		t.Fatal(err)
	}
	paths := []string{
		filepath.Join(threadDir, "thread.json"), filepath.Join(threadDir, "messages.jsonl"),
		filepath.Join(threadDir, "metadata.jsonl"), filepath.Join(threadDir, "events.jsonl"), indexPath,
	}
	before := map[string][]byte{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = body
		for _, forbidden := range []string{
			"RAW_TOOL_ARGUMENT_SENTINEL", "RAW_TOOL_RESULT_SENTINEL", "RAW_TOOL_MEDIA_SENTINEL", "RAW_TOP_LEVEL_TOOL_SENTINEL", "\"amount\":2645472",
		} {
			if bytes.Contains(body, []byte(forbidden)) {
				t.Fatalf("legacy tool bytes survived in %s (%s): %s", path, forbidden, body)
			}
		}
	}
	threadBody := readMigrationFixtureObject(t, filepath.Join(threadDir, "thread.json"))
	items := threadBody["turns"].([]any)[0].(map[string]any)["items"].([]any)
	callProjection := items[0].(map[string]any)["arguments"].(map[string]any)
	resultProjection := items[1].(map[string]any)["output"].(map[string]any)
	if callProjection["messageKey"] != "tool_arguments_withheld" || resultProjection["messageKey"] != "legacy_output_withheld" {
		t.Fatalf("closed projections missing: call=%#v result=%#v", callProjection, resultProjection)
	}
	durableResult := items[1].(map[string]any)
	if durableResult["contextDigest"] != contextDigest || durableResult["contextEpoch"] != float64(7) ||
		durableResult["executionGrantId"] != grantID || durableResult["hostEvidenceSettlement"] == nil {
		t.Fatalf("durable migration lost host settlement authority: %#v", durableResult)
	}
	for _, path := range []string{filepath.Join(threadDir, "messages.jsonl"), filepath.Join(threadDir, "metadata.jsonl"), indexPath} {
		body := before[path]
		if bytes.Contains(body, []byte(contextDigest)) || bytes.Contains(body, []byte(grantID)) || bytes.Contains(body, []byte("host-only-settlement")) {
			t.Fatalf("private settlement authority escaped ordinary migration projection %s: %s", path, body)
		}
	}
	events := readMigrationFixtureLines(t, filepath.Join(threadDir, "events.jsonl"))
	if len(events) != 3 || events[0]["seq"] != float64(1) || events[1]["seq"] != float64(2) || events[2]["seq"] != float64(3) ||
		events[2]["kind"] != "tool_payload_redacted" || events[2]["sourceHash"] != nil {
		t.Fatalf("event cursor/audit tombstone changed: %#v", events)
	}

	if err := MigrateOrdinaryToolPayloads(input); err != nil {
		t.Fatal(err)
	}
	for path, want := range before {
		got, _ := os.ReadFile(path)
		if !bytes.Equal(got, want) {
			t.Fatalf("ordinary tool-payload migration is not idempotent for %s\nwant=%s\ngot=%s", path, strings.TrimSpace(string(want)), strings.TrimSpace(string(got)))
		}
	}
}

func TestMigrateOrdinaryToolPayloadsRejectsAmbiguousThreadBeforeWrite(t *testing.T) {
	root := t.TempDir()
	threadDir := filepath.Join(root, "threads", "thread-duplicate")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "thread.json")
	body := []byte(`{"id":"thread-duplicate","id":"attacker","turns":[{"items":[{"kind":"tool_result","output":"RAW"}]}]}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateOrdinaryToolPayloads(OrdinaryToolPayloadMigrationInput{Root: root}); err == nil {
		t.Fatal("duplicate-key legacy thread was accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, body) {
		t.Fatalf("rejected migration mutated the source: %s", after)
	}
}

func TestSemanticStartupContentMigrationIncludesToolPayloadClosure(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-semantic-content"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{"id": threadID, "turns": []any{map[string]any{
		"id": "turn-1", "items": []any{map[string]any{
			"id": "result-1", "threadId": threadID, "turnId": "turn-1", "kind": "tool_result", "role": "tool",
			"status": "completed", "toolName": "read_file", "callId": "call-1", "isError": false,
			"output": map[string]any{"secret": "SEMANTIC_RAW_TOOL_SENTINEL"},
		}},
	}}}
	writeMigrationFixtureJSON(t, filepath.Join(threadDir, "thread.json"), thread)
	if err := MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(threadDir, "thread.json"))
	if bytes.Contains(body, []byte("SEMANTIC_RAW_TOOL_SENTINEL")) || bytes.Contains(body, []byte("legacy_output_withheld")) {
		t.Fatalf("semantic startup retained untrusted legacy tool history: %s", body)
	}
	var decoded map[string]any
	if json.Unmarshal(body, &decoded) != nil {
		t.Fatalf("migrated thread is not JSON: %s", body)
	}
	items := decoded["turns"].([]any)[0].(map[string]any)["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("authority-free legacy tool result was not retired: %#v", items)
	}
	if _, err := threadapp.ProjectPublicThread(decoded); err != nil {
		t.Fatalf("semantic startup did not produce a closed public thread: %v", err)
	}
}
