package eventlog

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestMigrateDerivedExecutionAuthorityStripsSourceAndPreservesOwnedTurn(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-derived"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 7, 0, 0, 0, time.UTC)
	sourceContext := migrationExecutionContext(t, "thread-source", "turn-source", now)
	sourceGrant := migrationGrant(sourceContext, migrationHostToolCallID("source"), "SOURCE_ARGUMENT_SENTINEL", now)
	ownedContext := migrationExecutionContext(t, threadID, "turn-owned", now.Add(time.Hour))
	ownedGrant := migrationGrant(ownedContext, migrationHostToolCallID("owned"), "OWNED_ARGUMENT_SENTINEL", now.Add(time.Hour))
	sourceCall := migrationToolCallItem(threadID, sourceContext, sourceGrant, "SOURCE_ARGUMENT_SENTINEL")
	ownedCall := migrationToolCallItem(threadID, ownedContext, ownedGrant, "OWNED_ARGUMENT_SENTINEL")
	hostProjection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionHostStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "tool_completed", Status: "completed", Code: "tool_completed",
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
	}
	sourceResult := map[string]any{
		"id": "result-source", "threadId": threadID, "turnId": sourceContext.TurnID, "kind": "tool_result", "role": "tool",
		"status": "completed", "toolName": "read", "callId": sourceGrant.ToolCallID, "isError": false,
		"createdAt": now.Add(time.Second).Format(time.RFC3339Nano), "finishedAt": now.Add(time.Second).Format(time.RFC3339Nano),
		"contextDigest": sourceContext.ContextDigest, "contextEpoch": float64(sourceContext.ContextEpoch),
		"executionGrantId": sourceGrant.GrantID, "hostEvidenceSettlement": map[string]any{"settlementId": "SOURCE_SETTLEMENT_SENTINEL"},
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(hostProjection),
	}
	sourceTransition := map[string]any{
		"id": "transition-source", "threadId": threadID, "turnId": sourceContext.TurnID, "kind": "execution_grant_transition",
		"contextDigest": sourceContext.ContextDigest, "contextEpoch": float64(sourceContext.ContextEpoch),
		"executionGrantId": sourceGrant.GrantID, "parentGrantId": "SOURCE_PARENT_GRANT_SENTINEL", "executionGrant": migrationRecord(sourceGrant),
	}
	thread := map[string]any{
		"id": threadID, "forkedFromThreadId": sourceContext.ThreadID, "forkedAt": now.Add(30 * time.Minute).Format(time.RFC3339Nano),
		"createdAt": now.Format(time.RFC3339Nano), "updatedAt": now.Add(time.Hour).Format(time.RFC3339Nano),
		"securityState": sourceContext, "contextEpochState": map[string]any{"threadId": sourceContext.ThreadID},
		"pendingApprovalIds": []any{"SOURCE_APPROVAL_SENTINEL"}, "pendingUserInputIds": []any{"SOURCE_INPUT_SENTINEL"},
		"turns": []any{
			map[string]any{
				"id": sourceContext.TurnID, "threadId": threadID, "status": "completed", "securityContext": sourceContext,
				"items": []any{
					sourceCall, sourceTransition, sourceResult,
					map[string]any{"id": "approval-source", "kind": "approval", "threadId": threadID, "turnId": sourceContext.TurnID, "status": "pending", "approvalId": "SOURCE_APPROVAL_SENTINEL", "continuationReceiptId": "SOURCE_RECEIPT_SENTINEL"},
					map[string]any{"id": "input-source", "kind": "user_input", "threadId": threadID, "turnId": sourceContext.TurnID, "status": "pending", "inputId": "SOURCE_INPUT_SENTINEL", "continuationReceiptId": "SOURCE_RECEIPT_SENTINEL"},
				},
			},
			map[string]any{
				"id": ownedContext.TurnID, "threadId": threadID, "status": "running", "securityContext": ownedContext,
				"items": []any{ownedCall},
			},
		},
	}
	writeMigrationFixtureJSON(t, filepath.Join(threadDir, "thread.json"), thread)
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "messages.jsonl"), []map[string]any{
		sourceCall, sourceTransition, sourceResult, ownedCall,
	})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), []map[string]any{{
		"kind": "thread_metadata", "version": float64(1), "timestamp": now.Format(time.RFC3339Nano), "thread": thread,
	}})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "events.jsonl"), []map[string]any{
		migrationReadyEvent(threadID, sourceContext, sourceGrant, float64(1)),
		migrationReadyEvent(threadID, ownedContext, ownedGrant, float64(2)),
	})

	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	migrated := readMigrationFixtureObject(t, filepath.Join(threadDir, "thread.json"))
	body, _ := json.Marshal(migrated)
	for _, sentinel := range []string{
		sourceGrant.GrantID, sourceContext.ContextDigest, "SOURCE_ARGUMENT_SENTINEL", "SOURCE_SETTLEMENT_SENTINEL",
		"SOURCE_PARENT_GRANT_SENTINEL", "SOURCE_APPROVAL_SENTINEL", "SOURCE_INPUT_SENTINEL", "SOURCE_RECEIPT_SENTINEL",
	} {
		if bytes.Contains(body, []byte(sentinel)) {
			t.Fatalf("source authority survived migration %q: %s", sentinel, body)
		}
	}
	if !bytes.Contains(body, []byte(ownedGrant.GrantID)) || !bytes.Contains(body, []byte(ownedContext.ContextDigest)) {
		t.Fatalf("owned post-fork authority was stripped: %s", body)
	}
	if _, err := executiongrantapp.RegistryFromThread(threadID, migrated, ownedContext.TurnID); err != nil {
		t.Fatalf("owned post-fork registry no longer replays: %v", err)
	}
	sourceTurn := migrated["turns"].([]any)[0].(map[string]any)
	if sourceTurn["securityContext"] != nil || len(sourceTurn["items"].([]any)) != 4 {
		t.Fatalf("source turn did not lose its grant transition/context: %#v", sourceTurn)
	}

	messages, err := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(messages, []byte("executionGrant")) || bytes.Contains(messages, []byte("SOURCE_ARGUMENT_SENTINEL")) ||
		bytes.Contains(messages, []byte("OWNED_ARGUMENT_SENTINEL")) || bytes.Contains(messages, []byte("execution_grant_transition")) {
		t.Fatalf("public message sidecar retained execution authority: %s", messages)
	}
	events := readMigrationFixtureLines(t, filepath.Join(threadDir, "events.jsonl"))
	if len(events) != 2 || events[0]["kind"] != "execution_authority_redacted" || events[0]["seq"] != float64(1) ||
		events[1]["kind"] != "tool_call_ready" || events[1]["seq"] != float64(2) {
		t.Fatalf("event migration did not preserve cursor and owned authority: %#v", events)
	}

	paths := []string{"thread.json", "messages.jsonl", "metadata.jsonl", "events.jsonl"}
	before := map[string][]byte{}
	for _, name := range paths {
		before[name], _ = os.ReadFile(filepath.Join(threadDir, name))
	}
	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	for _, name := range paths {
		after, _ := os.ReadFile(filepath.Join(threadDir, name))
		if !bytes.Equal(before[name], after) {
			t.Fatalf("execution-authority migration is not idempotent for %s", name)
		}
	}
}

func TestMigrateDerivedExecutionAuthorityRejectsAmbiguousJSONBeforeMutation(t *testing.T) {
	root := t.TempDir()
	threadDir := filepath.Join(root, "threads", "thread-duplicate")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "thread.json")
	body := []byte(`{"id":"thread-duplicate","id":"thread-attacker","turns":[]}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: root}); err == nil {
		t.Fatal("ambiguous duplicate-key thread JSON was accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(body, after) {
		t.Fatalf("failed strict migration mutated live stage input: %s", after)
	}
}

func TestMigrateDerivedExecutionAuthorityPreservesCanonicalLegacyItemIdentity(t *testing.T) {
	root := t.TempDir()
	threadID := "thread-legacy-item-identity"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": "turn-legacy", "threadId": threadID, "status": "completed",
			"contextDigest": strings.Repeat("a", 64),
			"items": []any{map[string]any{
				"id": "legacy-call-item", "threadId": threadID, "turnId": "turn-legacy",
				"kind": "tool_call", "status": "completed", "toolName": "read_file",
				"callId": "legacy-provider-call", "arguments": map[string]any{"path": "PRIVATE_ARGUMENT"},
				"contextDigest": strings.Repeat("a", 64), "executionGrantId": "legacy-untrusted-grant",
			}},
		}},
	}
	writeMigrationFixtureJSON(t, filepath.Join(threadDir, "thread.json"), thread)

	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	migrated := readMigrationFixtureObject(t, filepath.Join(threadDir, "thread.json"))
	items := migrated["turns"].([]any)[0].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != "legacy-call-item" {
		t.Fatalf("authority stripping created an anonymous legacy item: %#v", items)
	}
	if items[0].(map[string]any)["executionGrantId"] != nil || items[0].(map[string]any)["contextDigest"] != nil {
		t.Fatalf("untrusted legacy authority survived item identity preservation: %#v", items)
	}
}

func TestMigrateDerivedExecutionAuthoritySupportsSidecarOnlyThreadWithoutMintingPrimaryAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_sidecar_only_legacy"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": threadID, "title": "Legacy sidecar", "createdAt": "2026-07-01T00:00:00Z", "updatedAt": "2026-07-01T00:00:01Z",
		"turns": []any{map[string]any{"id": "turn_parent", "threadId": threadID, "status": "completed", "items": []any{}}},
	}
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), []map[string]any{{
		"kind": "thread_metadata", "version": float64(1), "timestamp": "2026-07-01T00:00:01Z", "thread": thread,
	}})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "messages.jsonl"), []map[string]any{{
		"id": "item_parent", "kind": "tool_call", "threadId": threadID, "turnId": "turn_parent", "status": "completed",
		"toolName": "delegate_task", "callId": "call_parent", "arguments": map[string]any{"task": "PRIVATE_ARGUMENT_SENTINEL"},
		"contextDigest": strings.Repeat("a", 64), "contextEpoch": float64(1), "executionGrantId": "grant_untrusted",
	}})
	writeMigrationFixtureJSONL(t, filepath.Join(threadDir, "events.jsonl"), []map[string]any{{
		"kind": "tool_call_ready", "threadId": threadID, "turnId": "turn_parent", "seq": float64(1),
		"contextDigest": strings.Repeat("a", 64), "contextEpoch": float64(1), "executionGrantId": "grant_untrusted",
	}})

	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(threadDir, "thread.json")); !os.IsNotExist(err) {
		t.Fatalf("sidecar migration minted a primary thread snapshot: %v", err)
	}
	messages, err := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"executionGrantId", "contextDigest", "PRIVATE_ARGUMENT_SENTINEL"} {
		if bytes.Contains(messages, []byte(forbidden)) {
			t.Fatalf("sidecar-only migration retained untrusted authority %q", forbidden)
		}
	}
	events := readMigrationFixtureLines(t, filepath.Join(threadDir, "events.jsonl"))
	if len(events) != 1 || events[0]["kind"] != "execution_authority_redacted" || events[0]["seq"] != float64(1) {
		t.Fatalf("sidecar-only authority event did not become a cursor-preserving tombstone: %#v", events)
	}
	before := map[string][]byte{}
	for _, name := range []string{"metadata.jsonl", "messages.jsonl", "events.jsonl"} {
		before[name], _ = os.ReadFile(filepath.Join(threadDir, name))
	}
	if err := MigrateDerivedExecutionAuthority(DerivedExecutionAuthorityMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	for name, expected := range before {
		after, _ := os.ReadFile(filepath.Join(threadDir, name))
		if !bytes.Equal(after, expected) {
			t.Fatalf("sidecar-only execution-authority migration is not idempotent for %s", name)
		}
	}
}

func migrationExecutionContext(t *testing.T, threadID, turnID string, at time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	context, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func migrationGrant(context domainsecurity.TurnSecurityContext, callID, argument string, at time.Time) domainsecurity.ExecutionGrant {
	arguments := []byte(`{"path":"` + argument + `"}`)
	return domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: at, ExpiresAt: at.Add(2 * time.Hour),
	})
}

func migrationToolCallItem(threadID string, context domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, argument string) map[string]any {
	return map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(context.TurnID, grant.ToolCallID), "threadId": threadID, "turnId": context.TurnID, "kind": "tool_call", "role": "assistant",
		"status": "completed", "toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": map[string]any{"path": argument},
		"createdAt": grant.IssuedAt, "contextDigest": context.ContextDigest, "contextEpoch": float64(context.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": migrationRecord(grant),
	}
}

func migrationReadyEvent(threadID string, context domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, seq float64) map[string]any {
	return map[string]any{
		"kind": "tool_call_ready", "threadId": threadID, "turnId": context.TurnID, "itemId": domaintoolcall.ToolCallItemIDV1(context.TurnID, grant.ToolCallID),
		"toolName": grant.ToolName, "callId": grant.ToolCallID, "readyCount": float64(1), "seq": seq,
		"contextDigest": context.ContextDigest, "contextEpoch": float64(context.ContextEpoch), "executionGrant": migrationRecord(grant),
	}
}

func migrationHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("eventlog-execution-authority-migration:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func migrationRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func writeMigrationFixtureJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()
	body, _ := json.Marshal(value)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeMigrationFixtureJSONL(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	lines := make([]string, 0, len(records))
	for _, record := range records {
		body, _ := json.Marshal(record)
		lines = append(lines, string(body))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readMigrationFixtureObject(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{}
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func readMigrationFixtureLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		record := map[string]any{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}
