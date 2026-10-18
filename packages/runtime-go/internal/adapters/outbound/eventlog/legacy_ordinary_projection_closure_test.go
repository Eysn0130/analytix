package eventlog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	"analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
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

func legacyForkToolHistoryFixtureV1(t *testing.T, boundary bool) (map[string]any, []map[string]any) {
	t.Helper()
	const sourceID, turnID, now = "thread-fork-source", "turn-inherited", "2026-10-10T00:00:00Z"
	callID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	call := domaintoolcall.PublicToolCallItemRecordV1(map[string]any{
		"threadId": sourceID, "turnId": turnID, "kind": "tool_call", "status": "completed",
		"toolName": "read", "toolKind": "tool_call", "callId": callID, "createdAt": now, "finishedAt": now,
	})
	result := domaintoolresult.PublicToolResultItemRecordV1(map[string]any{
		"id": domaintoolresult.ToolResultItemIDV1(turnID, callID), "threadId": sourceID, "turnId": turnID,
		"kind": "tool_result", "status": "completed", "toolName": "read", "toolKind": "tool_call",
		"callId": callID, "isError": false, "createdAt": now, "finishedAt": now,
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private")),
	})
	source := map[string]any{"id": sourceID, "status": "idle", "turns": []any{map[string]any{
		"id": turnID, "threadId": sourceID, "status": "completed", "createdAt": now, "finishedAt": now,
		"approvalPolicy": "on-request", "sandboxMode": "workspace-write",
		"steering": []any{}, "activeSkillIds": []any{}, "injectedMemoryIds": []any{}, "items": []any{map[string]any{"id": "item-original-user", "kind": "user_message",
			"role": "user", "threadId": sourceID, "turnId": turnID, "status": "completed", "text": "Synthetic ordinary input"}, call, result},
	}}}
	fork, err := threadapp.BuildFork(threadapp.ForkInput{Source: source, ForkID: "thread-fork-child", ParentThreadID: sourceID, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	sourceTurn, end := "", 0
	if boundary {
		sourceTurn, end = turnID, 9
	}
	events, err := threadapp.BuildForkCreatedEventsV1(fork, sourceID, sourceTurn, end)
	if err != nil {
		t.Fatal(err)
	}
	return fork, events
}

func TestSemanticStartupPreservesAndRecoversOnlyClosedForkToolHistoryV1(t *testing.T) {
	for _, boundary := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("boundary=%t/missing=%t", boundary, missing), func(t *testing.T) {
				root := t.TempDir()
				fork, events := legacyForkToolHistoryFixtureV1(t, boundary)
				id := contracts.StringField(fork, "id")
				dir := filepath.Join(root, "threads", id)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				seed, _, err := threadapp.ReadForkCreatedSeedV1(fork, events)
				if err != nil {
					t.Fatal(err)
				}
				if missing {
					fork["turns"].([]any)[0].(map[string]any)["items"] = fork["turns"].([]any)[0].(map[string]any)["items"].([]any)[:1]
				}
				// A later unproved legacy turn must not inherit the seed exemption.
				fork["turns"] = append(fork["turns"].([]any), map[string]any{"id": "turn-later", "threadId": id, "status": "completed", "items": []any{map[string]any{
					"id": "unproved-tool", "kind": "tool_call", "status": "completed", "toolName": "read", "callId": "provider-only", "arguments": map[string]any{"private": "UNPROVED_ARGUMENT"},
				}}})
				primaryPath, eventPath := filepath.Join(dir, "thread.json"), filepath.Join(dir, "events.jsonl")
				writeMigrationFixtureJSON(t, primaryPath, fork)
				writeMigrationFixtureJSONL(t, eventPath, events)
				beforeEvents, _ := os.ReadFile(eventPath)
				input := SemanticStartupContentMigrationInput{Root: root}
				if err := MigrateSemanticStartupContent(input); err != nil {
					t.Fatal(err)
				}
				actual := readMigrationFixtureObject(t, primaryPath)
				turn := actual["turns"].([]any)[0].(map[string]any)
				if !sameMigrationJSON(turn, seed["turns"].([]any)[0].(map[string]any)) {
					t.Fatal("closed inherited history changed")
				}
				if len(actual["turns"].([]any)[1].(map[string]any)["items"].([]any)) != 0 {
					t.Fatal("later turn acquired inherited history exemption")
				}
				for _, raw := range turn["items"].([]any) {
					item := raw.(map[string]any)
					for _, key := range []string{"executionGrant", "executionGrantId", "securityContext", "continuationReceiptId"} {
						if _, present := item[key]; present {
							t.Fatalf("inherited item acquired %s", key)
						}
					}
				}
				beforePrimary, _ := os.ReadFile(primaryPath)
				if err := MigrateSemanticStartupContent(input); err != nil {
					t.Fatal(err)
				}
				afterPrimary, _ := os.ReadFile(primaryPath)
				afterEvents, _ := os.ReadFile(eventPath)
				if !bytes.Equal(beforePrimary, afterPrimary) || !bytes.Equal(beforeEvents, afterEvents) {
					t.Fatal("closed fork recovery is not byte-idempotent")
				}
			})
		}
	}
}

func TestSemanticStartupRejectsForkSeedDisagreementBeforeLegacyWritesV1(t *testing.T) {
	for _, scenario := range []string{"changed-arguments", "changed-tool", "reordered", "duplicate", "missing-user", "changed-turn", "invalid-policy", "padded-policy", "invalid-sandbox", "policy-object", "extra-turn-authority", "missing-close", "bad-digest"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			fork, events := legacyForkToolHistoryFixtureV1(t, false)
			dir := filepath.Join(root, "threads", contracts.StringField(fork, "id"))
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			turn := fork["turns"].([]any)[0].(map[string]any)
			items := turn["items"].([]any)
			switch scenario {
			case "changed-arguments":
				items[1].(map[string]any)["arguments"] = map[string]any{"private": "MUST_NOT_SANITIZE_INTO_PROOF"}
			case "changed-tool":
				items[1].(map[string]any)["toolName"] = "other"
			case "reordered":
				items[1], items[2] = items[2], items[1]
			case "duplicate":
				turn["items"] = append(items, contracts.CloneMap(items[1].(map[string]any)))
			case "missing-user":
				turn["items"] = items[1:]
			case "changed-turn":
				turn["prompt"] = "changed"
			case "invalid-policy":
				turn["approvalPolicy"] = "invalid"
			case "padded-policy":
				turn["approvalPolicy"] = " on-request"
			case "invalid-sandbox":
				turn["sandboxMode"] = "invalid"
			case "policy-object":
				turn["approvalPolicy"] = map[string]any{"value": "on-request"}
			case "extra-turn-authority":
				turn["unknownAuthority"] = "MUST_NOT_NORMALIZE_INTO_PROOF"
			case "missing-close":
				events = events[:len(events)-1]
			case "bad-digest":
				events[0]["inheritedHistorySeed"].(map[string]any)["historySHA256"] = strings.Repeat("0", 64)
			}
			primaryPath, eventPath := filepath.Join(dir, "thread.json"), filepath.Join(dir, "events.jsonl")
			writeMigrationFixtureJSON(t, primaryPath, fork)
			writeMigrationFixtureJSONL(t, eventPath, events)
			legacyDir := filepath.Join(root, "threads", "a-unproved-legacy")
			if err := os.MkdirAll(legacyDir, 0o700); err != nil {
				t.Fatal(err)
			}
			legacyPath := filepath.Join(legacyDir, "thread.json")
			writeMigrationFixtureJSON(t, legacyPath, map[string]any{"id": "a-unproved-legacy", "turns": []any{map[string]any{"id": "legacy-turn", "items": []any{map[string]any{"id": "legacy-tool", "kind": "tool_result", "output": "MUST_REMAIN_BEFORE_PREFLIGHT"}}}}})
			before := map[string][]byte{}
			for _, path := range []string{primaryPath, eventPath, legacyPath} {
				before[path], _ = os.ReadFile(path)
			}
			want := "fork history inherited item differs from seed"
			switch scenario {
			case "reordered", "duplicate":
				want = "fork history inherited item order differs from seed"
			case "missing-user":
				want = "fork history missing item is not a completed inert tool"
			case "changed-turn", "invalid-policy", "padded-policy", "invalid-sandbox", "policy-object", "extra-turn-authority":
				want = "fork history inherited turn metadata differs from seed"
			case "missing-close", "bad-digest":
				want = "fork history creation seed is invalid"
			}
			if err := MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root}); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("seed disagreement failure mismatch scenario=%s err=%v", scenario, err)
			}
			for path, expected := range before {
				actual, _ := os.ReadFile(path)
				if !bytes.Equal(expected, actual) {
					t.Fatal("failed fork preflight changed staged bytes")
				}
			}
		})
	}
}

func TestSemanticStartupRetiresUnprovedClosedForkToolLabelsV1(t *testing.T) {
	root := t.TempDir()
	fork, _ := legacyForkToolHistoryFixtureV1(t, false)
	id := contracts.StringField(fork, "id")
	dir := filepath.Join(root, "threads", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	primaryPath := filepath.Join(dir, "thread.json")
	writeMigrationFixtureJSON(t, primaryPath, fork)
	writeMigrationFixtureJSONL(t, filepath.Join(dir, "events.jsonl"), []map[string]any{{
		"kind": "thread_created", "threadId": id, "timestamp": fork["createdAt"], "status": "idle", "seq": float64(1),
	}})
	if err := MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	items := readMigrationFixtureObject(t, primaryPath)["turns"].([]any)[0].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != "item-original-user" {
		t.Fatal("unproved fork labels retained tool history")
	}
}

func TestSemanticStartupNormalizesOnlyKnownForkPromotionMarkersV1(t *testing.T) {
	for _, scenario := range []string{"complete", "missing-tools", "partial", "version", "task-job", "invalid-digest", "padded-digest", "changed-content", "extra-authority", "tool-marker"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			fork, _ := legacyForkToolHistoryFixtureV1(t, false)
			threadID := contracts.StringField(fork, "id")
			turn := fork["turns"].([]any)[0].(map[string]any)
			turnID := contracts.StringField(turn, "id")
			const clientID, now = "ordinary-history-client", "2026-10-10T00:00:00Z"
			id := domainsteering.EntryIDV1(turnID, clientID)
			item := map[string]any{"id": id, "threadId": threadID, "turnId": turnID, "kind": "user_message", "role": "user", "status": "completed",
				"text": "Synthetic accepted ordinary guidance", "delivery": "steer", "clientUserMessageId": clientID, "createdAt": now, "finishedAt": now}
			turn["items"] = append(turn["items"].([]any), item)
			turn["steering"] = []any{map[string]any{"id": id, "clientUserMessageId": clientID, "text": item["text"], "admittedAt": now,
				"status": "promoted", "promotedAt": now, "promotedItemId": id, "attachmentIds": []any{}, "fileReferences": []any{}}}
			// Build a genuine complete seed first. Only this fresh unit primary
			// then receives the old producer's three-field shape; no seed edits.
			events, err := threadapp.BuildForkCreatedEventsV1(fork, "thread-fork-source", "", 0)
			if err != nil {
				t.Fatal(err)
			}
			seed, _, err := threadapp.ReadForkCreatedSeedV1(fork, events)
			if err != nil {
				t.Fatal(err)
			}
			item["steeringProjectionVersion"], item["steeringContentDigest"], item["steeringOrigin"] = 1, strings.Repeat("a", 64), "ordinary"
			switch scenario {
			case "missing-tools":
				turn["items"] = []any{turn["items"].([]any)[0], item}
			case "partial":
				delete(item, "steeringOrigin")
			case "version":
				item["steeringProjectionVersion"] = 2
			case "task-job":
				item["steeringOrigin"] = "task_job"
			case "invalid-digest":
				item["steeringContentDigest"] = "invalid"
			case "padded-digest":
				item["steeringContentDigest"] = " " + strings.Repeat("a", 64)
			case "changed-content":
				item["text"] = "Changed guidance"
			case "extra-authority":
				item["contextDigest"] = strings.Repeat("b", 64)
			case "tool-marker":
				turn["items"].([]any)[1].(map[string]any)["steeringOrigin"] = "ordinary"
			}
			dir := filepath.Join(root, "threads", threadID)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			primaryPath, eventsPath := filepath.Join(dir, "thread.json"), filepath.Join(dir, "events.jsonl")
			writeMigrationFixtureJSON(t, primaryPath, fork)
			writeMigrationFixtureJSONL(t, eventsPath, events)
			beforePrimary, _ := os.ReadFile(primaryPath)
			beforeEvents, _ := os.ReadFile(eventsPath)
			err = MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root})
			valid := scenario == "complete" || scenario == "missing-tools"
			if !valid {
				if err == nil || !strings.Contains(err.Error(), "fork history inherited item differs from seed") {
					t.Fatal("mismatched fork material was normalized into proof", err)
				}
				afterPrimary, _ := os.ReadFile(primaryPath)
				afterEvents, _ := os.ReadFile(eventsPath)
				if !bytes.Equal(beforePrimary, afterPrimary) || !bytes.Equal(beforeEvents, afterEvents) {
					t.Fatal("failed preflight rewrote history")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			actual := readMigrationFixtureObject(t, primaryPath)
			actualTurns, seedTurns := actual["turns"].([]any), seed["turns"].([]any)
			if len(actualTurns) != 1 || len(seedTurns) != 1 || !sameMigrationJSON(actualTurns[0].(map[string]any), seedTurns[0].(map[string]any)) {
				t.Fatal("compatible primary did not recover the complete closed seed")
			}
			afterPrimary, _ := os.ReadFile(primaryPath)
			afterEvents, _ := os.ReadFile(eventsPath)
			if !bytes.Equal(beforeEvents, afterEvents) {
				t.Fatal("compatible recovery rewrote immutable creation events")
			}
			if err := MigrateSemanticStartupContent(SemanticStartupContentMigrationInput{Root: root}); err != nil {
				t.Fatal(err)
			}
			repeated, _ := os.ReadFile(primaryPath)
			if !bytes.Equal(afterPrimary, repeated) {
				t.Fatal("compatible recovery is not byte-idempotent")
			}
		})
	}
}
