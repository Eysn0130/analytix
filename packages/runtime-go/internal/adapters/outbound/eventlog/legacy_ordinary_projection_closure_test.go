package eventlog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateLegacyOrdinaryProjectionClosureRetiresUntrustedToolsAndTombstonesIdempotently(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-legacy-closure"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	user := map[string]any{
		"id": "item-user", "threadId": threadID, "turnId": "turn-1", "kind": "user_message",
		"status": "completed", "text": "keep user content", "createdAt": "2026-07-01T00:00:00Z",
	}
	call := map[string]any{
		"id": "item-call", "threadId": threadID, "turnId": "turn-1", "kind": "tool_call",
		"status": "completed", "toolName": "read_file", "callId": "provider-call",
		"arguments": map[string]any{"path": "PRIVATE_TOOL_ARGUMENT"},
	}
	result := map[string]any{
		"id": "item-result", "threadId": threadID, "turnId": "turn-1", "kind": "tool_result",
		"status": "completed", "toolName": "read_file", "callId": "provider-call", "isError": false,
		"output": map[string]any{"amount": float64(2645472)},
	}
	thread := map[string]any{
		"id": threadID, "title": "legacy closure", "status": "idle", "workspace": "/workspace",
		"turns": []any{map[string]any{
			"id": "turn-1", "threadId": threadID, "status": "completed", "items": []any{user, call, result},
		}},
	}
	writeMigrationFixtureJSON(t, filepath.Join(threadDir, "thread.json"), thread)
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "messages.jsonl"), []map[string]any{
		user, call, result,
		{"id": "redacted", "threadId": threadID, "turnId": "turn-1", "kind": "content_redacted", "code": "private_message_content_removed"},
		{"id": "reasoning", "threadId": threadID, "turnId": "turn-1", "kind": "assistant_reasoning", "text": "PRIVATE_REASONING"},
		{"id": "draft", "threadId": threadID, "turnId": "turn-1", "kind": "assistant_text", "text": "UNACCEPTED_DRAFT"},
	})

	input := LegacyOrdinaryProjectionClosureInput{Root: root}
	if err := MigrateLegacyOrdinaryProjectionClosure(input); err != nil {
		t.Fatal(err)
	}
	migrated := readMigrationFixtureObject(t, filepath.Join(threadDir, "thread.json"))
	items := migrated["turns"].([]any)[0].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != "item-user" {
		t.Fatalf("legacy tool history survived closed primary projection: %#v", items)
	}
	messages := readMigrationFixtureLines(t, filepath.Join(threadDir, "messages.jsonl"))
	if len(messages) != 1 || messages[0]["id"] != "item-user" {
		t.Fatalf("non-hydratable message history survived closure: %#v", messages)
	}
	for _, path := range []string{filepath.Join(threadDir, "thread.json"), filepath.Join(threadDir, "messages.jsonl")} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range [][]byte{
			[]byte("PRIVATE_TOOL_ARGUMENT"), []byte("2645472"), []byte("PRIVATE_REASONING"), []byte("UNACCEPTED_DRAFT"),
		} {
			if bytes.Contains(body, forbidden) {
				t.Fatalf("retired content %q survived in %s: %s", forbidden, path, body)
			}
		}
	}
	beforeThread, _ := os.ReadFile(filepath.Join(threadDir, "thread.json"))
	beforeMessages, _ := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if err := MigrateLegacyOrdinaryProjectionClosure(input); err != nil {
		t.Fatal(err)
	}
	afterThread, _ := os.ReadFile(filepath.Join(threadDir, "thread.json"))
	afterMessages, _ := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if !bytes.Equal(beforeThread, afterThread) || !bytes.Equal(beforeMessages, afterMessages) {
		t.Fatal("legacy ordinary projection closure is not byte-idempotent")
	}
}

func TestMigrateLegacyOrdinaryProjectionClosureSupportsSidecarOnlyRecoveryWithoutMintingPrimary(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-sidecar-closure"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": threadID, "title": "sidecar only", "status": "idle", "turns": []any{map[string]any{
			"id": "turn-1", "threadId": threadID, "status": "completed", "items": []any{},
		}},
	}
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), []map[string]any{{
		"kind": "thread_metadata", "timestamp": "2026-07-01T00:00:00Z", "thread": thread,
	}})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "messages.jsonl"), []map[string]any{
		{"id": "item-user", "threadId": threadID, "turnId": "turn-1", "kind": "user_message", "status": "completed", "text": "recover me"},
		{"id": "item-call", "threadId": threadID, "turnId": "turn-1", "kind": "tool_call", "status": "completed", "toolName": "delegate_task", "callId": "provider-call", "arguments": map[string]any{"private": true}},
	})
	if err := MigrateLegacyOrdinaryProjectionClosure(LegacyOrdinaryProjectionClosureInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(threadDir, "thread.json")); !os.IsNotExist(err) {
		t.Fatalf("sidecar-only closure minted primary authority: %v", err)
	}
	messages := readMigrationFixtureLines(t, filepath.Join(threadDir, "messages.jsonl"))
	if len(messages) != 1 || messages[0]["id"] != "item-user" {
		t.Fatalf("sidecar-only closure did not retain only safe user history: %#v", messages)
	}
}

func TestMigrateLegacyOrdinaryProjectionClosureAbortsUnsignedInFlightTurnWithoutMintingAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-legacy-in-flight"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	user := map[string]any{
		"id": "item-user", "threadId": threadID, "turnId": "turn-1", "kind": "user_message",
		"status": "completed", "text": "keep user content",
	}
	call := map[string]any{
		"id": "item-call", "threadId": threadID, "turnId": "turn-1", "kind": "tool_call",
		"status": "pending", "toolName": "read_file", "callId": "provider-call",
		"arguments": map[string]any{"path": "UNTRUSTED_PENDING_ARGUMENT"},
	}
	approval := map[string]any{
		"id": "item-approval", "threadId": threadID, "turnId": "turn-1", "kind": "approval",
		"status": "pending", "approvalId": "legacy-approval", "toolName": "read_file",
	}
	thread := map[string]any{
		"id": threadID, "title": "legacy in flight", "status": "running", "workspace": "/workspace",
		"turns": []any{map[string]any{
			"id": "turn-1", "threadId": threadID, "status": "running", "items": []any{user, call, approval},
		}},
	}
	writeMigrationFixtureJSON(t, filepath.Join(threadDir, "thread.json"), thread)
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "messages.jsonl"), []map[string]any{user, call, approval})

	input := LegacyOrdinaryProjectionClosureInput{Root: root}
	if err := MigrateLegacyOrdinaryProjectionClosure(input); err != nil {
		t.Fatal(err)
	}
	migrated := readMigrationFixtureObject(t, filepath.Join(threadDir, "thread.json"))
	if migrated["status"] != "idle" {
		t.Fatalf("unsigned legacy thread remained executable: %#v", migrated)
	}
	turn := migrated["turns"].([]any)[0].(map[string]any)
	if turn["status"] != "aborted" {
		t.Fatalf("unsigned legacy turn remained resumable: %#v", turn)
	}
	items := turn["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["kind"] != "user_message" ||
		items[1].(map[string]any)["kind"] != "approval" || items[1].(map[string]any)["status"] != "expired" {
		t.Fatalf("legacy pending execution was not closed deterministically: %#v", items)
	}
	messages := readMigrationFixtureLines(t, filepath.Join(threadDir, "messages.jsonl"))
	if len(messages) != 2 || messages[1]["kind"] != "approval" || messages[1]["status"] != "expired" {
		t.Fatalf("legacy sidecar retained a resumable gate: %#v", messages)
	}
	for _, record := range []map[string]any{migrated, turn, items[1].(map[string]any)} {
		for _, forbidden := range []string{"securityState", "securityContext", "executionGrant", "continuationReceiptId"} {
			if _, present := record[forbidden]; present {
				t.Fatalf("legacy closure minted %s: %#v", forbidden, record)
			}
		}
	}
	body, err := os.ReadFile(filepath.Join(threadDir, "thread.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("UNTRUSTED_PENDING_ARGUMENT")) || bytes.Contains(body, []byte("provider-call")) {
		t.Fatalf("legacy provider execution identity survived closure: %s", body)
	}
	beforeThread := append([]byte(nil), body...)
	beforeMessages, _ := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if err := MigrateLegacyOrdinaryProjectionClosure(input); err != nil {
		t.Fatal(err)
	}
	afterThread, _ := os.ReadFile(filepath.Join(threadDir, "thread.json"))
	afterMessages, _ := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if !bytes.Equal(beforeThread, afterThread) || !bytes.Equal(beforeMessages, afterMessages) {
		t.Fatal("legacy in-flight closure is not byte-idempotent")
	}
}

func TestMigrateLegacyOrdinaryProjectionClosureRejectsMalformedCurrentAuthorityWithoutWrite(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-current-corrupt"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	threadPath := filepath.Join(threadDir, "thread.json")
	messagesPath := filepath.Join(threadDir, "messages.jsonl")
	threadBody := []byte(`{"id":"thread-current-corrupt","securityState":null,"turns":[{"id":"turn-1","items":[]}]}`)
	messagesBody := []byte(`{"id":"item-user","threadId":"thread-current-corrupt","turnId":"turn-1","kind":"user_message","text":"unchanged"}` + "\n")
	if err := os.WriteFile(threadPath, threadBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(messagesPath, messagesBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyOrdinaryProjectionClosure(LegacyOrdinaryProjectionClosureInput{Root: root}); err == nil {
		t.Fatal("malformed current authority was reinterpreted as legacy")
	}
	afterThread, _ := os.ReadFile(threadPath)
	afterMessages, _ := os.ReadFile(messagesPath)
	if !bytes.Equal(threadBody, afterThread) || !bytes.Equal(messagesBody, afterMessages) {
		t.Fatal("rejected current authority closure mutated staged bytes")
	}
}

func TestSemanticStartupAuthorityPreflightRejectsLateCorruptionBeforeAnyMigrationWrite(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "threads", "a-legacy")
	corruptDir := filepath.Join(root, "threads", "z-current-corrupt")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(corruptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, "thread.json")
	legacyBody := []byte(`{"id":"a-legacy","turns":[{"id":"turn-1","items":[{"id":"result-1","kind":"tool_result","status":"completed","toolName":"read_file","callId":"provider-call","output":{"secret":"MUST_REMAIN_BEFORE_PREFLIGHT"}}]}]}`)
	corruptPath := filepath.Join(corruptDir, "thread.json")
	corruptBody := []byte(`{"id":"z-current-corrupt","securityState":null,"turns":[{"id":"turn-1","items":[]}]}`)
	if err := os.WriteFile(legacyPath, legacyBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corruptPath, corruptBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root}); err == nil {
		t.Fatal("semantic startup accepted malformed current authority")
	}
	afterLegacy, _ := os.ReadFile(legacyPath)
	afterCorrupt, _ := os.ReadFile(corruptPath)
	if !bytes.Equal(legacyBody, afterLegacy) || !bytes.Equal(corruptBody, afterCorrupt) {
		t.Fatal("semantic authority preflight mutated an earlier legacy thread before rejecting late corruption")
	}
}

func TestSemanticStartupAuthorityPreflightRejectsCurrentExecutionEventMismatch(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-current-event"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	securityContext := migrationExecutionContext(t, threadID, "turn-1", now)
	grant := migrationGrant(securityContext, migrationHostToolCallID("event"), "SAFE_ARGUMENT", now)
	call := migrationToolCallItem(threadID, securityContext, grant, "SAFE_ARGUMENT")
	thread := map[string]any{
		"id": threadID, "securityState": securityContext, "turns": []any{map[string]any{
			"id": "turn-1", "threadId": threadID, "status": "running", "securityContext": securityContext,
			"items": []any{call},
		}},
	}
	event := migrationReadyEvent(threadID, securityContext, grant, float64(1))
	event["executionGrantId"] = strings.Repeat("f", 64)
	threadPath := filepath.Join(threadDir, "thread.json")
	eventsPath := filepath.Join(threadDir, "events.jsonl")
	writeMigrationFixtureJSON(t, threadPath, thread)
	writeMigrationFixtureJSONL(t, eventsPath, []map[string]any{event})
	beforeThread, _ := os.ReadFile(threadPath)
	beforeEvents, _ := os.ReadFile(eventsPath)
	if err := MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root}); err == nil {
		t.Fatal("mismatched current execution event was tombstoned instead of rejected")
	}
	afterThread, _ := os.ReadFile(threadPath)
	afterEvents, _ := os.ReadFile(eventsPath)
	if !bytes.Equal(beforeThread, afterThread) || !bytes.Equal(beforeEvents, afterEvents) {
		t.Fatal("current execution event preflight mutated staged authority")
	}
}
