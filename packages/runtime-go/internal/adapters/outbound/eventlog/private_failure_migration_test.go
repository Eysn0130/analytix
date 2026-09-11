package eventlog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestEventLogRejectsRawFailureBeforeFilesystemMutation(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	err := store.AppendEvent("thread-private-error", map[string]any{
		"seq": float64(1), "kind": "tool_progress", "threadId": "thread-private-error", "status": "error",
		"error": "PRIVATE_PROVIDER_FAILURE",
	})
	if err == nil {
		t.Fatal("event log accepted a raw failure string")
	}
	if _, statErr := os.Stat(store.EventsPath("thread-private-error")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected append created durable state: %v", statErr)
	}
}

func TestSemanticMigrationPreservesOnlyNumericUsageCounts(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	threadID := "thread-legacy-usage"
	path := store.EventsPath(threadID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	const sentinel = "PRIVATE_PROVIDER_FAILURE_6E1B"
	legacy := `{"seq":1,"kind":"usage","threadId":"thread-legacy-usage","turnId":"turn-1","timestamp":"2026-07-19T00:00:00Z","usage":{"totalTokens":17,"reasoningTokens":3,"cacheDiagnostics":{"message":"` + sentinel + `"}},"error":"` + sentinel + `"}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSince(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 1 || len(loaded.Events) != 1 {
		t.Fatalf("legacy load projection mismatch: result=%#v err=%v", loaded, err)
	}
	assertNumericUsageProjectionV1(t, loaded.Events[0], sentinel)

	if err := MigratePrivateReasoning(PrivateReasoningMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(first), sentinel) {
		t.Fatalf("semantic migration retained private failure bytes: err=%v body=%s", err, first)
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(first), &event); err != nil {
		t.Fatal(err)
	}
	assertNumericUsageProjectionV1(t, event, sentinel)
	if err := MigratePrivateReasoning(PrivateReasoningMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("semantic migration was not idempotent: err=%v first=%s second=%s", err, first, second)
	}
}

func TestSemanticMigrationTombstonesLegacyFailureWithoutContentDerivedHash(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	threadID := "thread-legacy-failure"
	path := store.EventsPath(threadID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	const sentinel = "PRIVATE_PROVIDER_FAILURE account 6222020202020202020"
	legacy := `{"seq":7,"kind":"tool_progress","threadId":"thread-legacy-failure","turnId":"turn-1","status":"error","error":"` + sentinel + `"}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSince(threadID, 0)
	if err != nil || len(loaded.Events) != 1 {
		t.Fatalf("legacy failure load mismatch: result=%#v err=%v", loaded, err)
	}
	assertPrivateFailureTombstoneV1(t, loaded.Events[0], sentinel)

	if err := MigratePrivateReasoning(PrivateReasoningMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(body), sentinel) {
		t.Fatalf("legacy failure bytes survived semantic migration: err=%v body=%s", err, body)
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(body), &event); err != nil {
		t.Fatal(err)
	}
	assertPrivateFailureTombstoneV1(t, event, sentinel)
}

func TestPrivateContentMigrationProjectionCannotRetainReasoningInTimestamp(t *testing.T) {
	for _, timestamp := range []string{
		"<think>private</think>2026-07-19T00:00:00Z",
		"<analysis>private</analysis>2026-07-19T00:00:00Z",
		"<reasoning>private</reasoning>2026-07-19T00:00:00Z",
		"not-a-timestamp",
	} {
		record := map[string]any{
			"kind": "legacy_private", "timestamp": timestamp, "error": "private failure",
		}
		projected := validatedPrivateContentMigrationProjection(record, 1, "legacy raw line")
		if projected["timestamp"] != nil {
			t.Fatalf("migration retained reasoning timestamp %q: %#v", timestamp, projected)
		}
		if err := domainevent.ValidatePublicRecord(projected); err != nil {
			t.Fatalf("migration emitted a non-public record for %q: %#v err=%v", timestamp, projected, err)
		}
	}
}

func assertNumericUsageProjectionV1(t *testing.T, event map[string]any, sentinel string) {
	t.Helper()
	usage, _ := event["usage"].(map[string]any)
	if event["kind"] != "usage" || usage["totalTokens"] != float64(17) || usage["reasoningTokens"] != float64(3) || len(usage) != 2 {
		t.Fatalf("numeric usage projection mismatch: %#v", event)
	}
	encoded, _ := json.Marshal(event)
	if strings.Contains(string(encoded), sentinel) || event["error"] != nil || event["sourceHash"] != nil {
		t.Fatalf("numeric usage projection retained private failure semantics: %s", encoded)
	}
}

func assertPrivateFailureTombstoneV1(t *testing.T, event map[string]any, sentinel string) {
	t.Helper()
	encoded, _ := json.Marshal(event)
	if event["kind"] != "content_redacted" || event["code"] != "private_failure_removed" ||
		event["seq"] != float64(7) || event["sourceHash"] != nil || event["error"] != nil ||
		strings.Contains(string(encoded), sentinel) {
		t.Fatalf("legacy failure was not reduced to a fixed tombstone: %s", encoded)
	}
}
