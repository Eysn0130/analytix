package eventlog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appcontextepoch "analytix.local/runtime-go/internal/app/contextepoch"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestProjectLegacyOrdinaryThreadPrivacyRevalidatesPIIProjectionAgainstReasoningLimits(t *testing.T) {
	thread := map[string]any{
		"id":        "thr_post_privacy_reasoning_limit",
		"workspace": "/tmp/authorized-workspace",
		"turns": []any{map[string]any{
			"id": "turn-1",
			"items": []any{map[string]any{
				"id":   "assistant-1",
				"kind": "assistant_text",
				"text": strings.Repeat("账号: 6222020202020202020\n", 33),
			}},
		}},
	}

	projected, err := projectLegacyOrdinaryThreadPrivacyV1(thread)
	if err != nil {
		t.Fatal(err)
	}
	turns, _ := projected["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("post-privacy reasoning-limit assistant item survived: %#v", items)
	}
	if turn["providerPrivateContentRemoved"] != true {
		t.Fatalf("post-privacy removal was not audit-marked: %#v", turn)
	}
	if err := threadapp.ValidatePublicHistory(projected); err != nil {
		t.Fatalf("final privacy projection is not publicly valid: %v", err)
	}
}

func TestProjectLegacyOrdinaryThreadPrivacyKeepsCurrentAuthorityExactOrFailsClosed(t *testing.T) {
	state, err := appcontextepoch.DefaultState("thread-current-authority", 7, time.Unix(7, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	authority := appcontextepoch.PublicState(state)
	authority["version"] = json.Number("1")
	input := map[string]any{
		"id": "thread-current-authority", "status": "idle", "turns": []any{},
		"contextEpochState": authority, "title": "public title",
	}
	projected, err := projectLegacyOrdinaryThreadPrivacyV1(input)
	if err != nil {
		t.Fatal(err)
	}
	beforeAuthority, _ := json.Marshal(authority)
	afterAuthority, _ := json.Marshal(projected["contextEpochState"])
	if !bytes.Equal(afterAuthority, beforeAuthority) {
		t.Fatalf("frozen execution authority changed:\nwant=%s\ngot=%s", beforeAuthority, afterAuthority)
	}
	projectedAuthority, _ := projected["contextEpochState"].(map[string]any)
	if _, exactNumberType := projectedAuthority["version"].(json.Number); !exactNumberType {
		t.Fatalf("frozen execution authority scalar type changed: %#v", projectedAuthority)
	}

	unsafe := cloneLegacyAuthorityValueV1(input).(map[string]any)
	unsafe["title"] = "Authorization: Bearer ordinary-secret"
	if projected, err := projectLegacyOrdinaryThreadPrivacyV1(unsafe); err == nil || projected != nil {
		t.Fatalf("unsafe content beside current authority was rewritten instead of failing closed: %#v err=%v", projected, err)
	}
}

func TestMigrateReasoningThreadJSONPreflightsCurrentAuthorityBeforeSanitizing(t *testing.T) {
	state, err := appcontextepoch.DefaultState("thread-current-preflight", 7, time.Unix(7, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "thread.json")
	thread := map[string]any{
		"id":                "thread-current-preflight",
		"status":            "idle",
		"turns":             []any{},
		"contextEpochState": appcontextepoch.PublicState(state),
		"title":             "Authorization: Bearer current-authority-secret",
	}
	encoded, err := json.MarshalIndent(thread, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	before := append(encoded, '\n')
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateReasoningThreadJSON(path); err == nil {
		t.Fatal("unsafe current authority thread was sanitized instead of rejected")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected current authority thread changed bytes:\nwant=%s\ngot=%s", before, after)
	}
}

func TestCurrentHistoryLocatorProjectionPreservesRestartAuthority(t *testing.T) {
	state, err := appcontextepoch.DefaultState("thread-private-prose-restart", 7, time.Unix(7, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	const locator = "/Users/private-owner/SYNTHETIC-PII.csv"
	thread := map[string]any{
		"id": "thread-private-prose-restart", "status": "idle", "turns": []any{},
		"contextEpochState": appcontextepoch.PublicState(state), "title": "Review " + locator,
	}
	before, err := json.MarshalIndent(thread, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "thread.json")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateReasoningThreadJSON(path); err != nil {
			t.Fatalf("new prose rule blocked valid current history: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, before) {
			t.Fatal("restart rewrote current authority bytes")
		}
		public, err := threadapp.ProjectPublicThread(thread)
		if err != nil || public["title"] != "Review [PRIVATE_PATH]" {
			t.Fatalf("current publication leaked old prose: %v", err)
		}
	}
}

func TestMigrateReasoningThreadJSONRejectsDuplicateKeysBesideCurrentAuthority(t *testing.T) {
	state, err := appcontextepoch.DefaultState("thread-current-duplicate", 7, time.Unix(7, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	authority, err := json.Marshal(appcontextepoch.PublicState(state))
	if err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"id":"thread-current-duplicate","status":"idle","turns":[],"contextEpochState":` + string(authority) + `,"title":"first","title":"second"}`)
	path := filepath.Join(t.TempDir(), "thread.json")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateReasoningThreadJSON(path); err == nil {
		t.Fatal("duplicate-key current authority thread passed strict preflight")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("rejected duplicate-key current authority thread changed bytes")
	}
}

func TestProjectLegacyOrdinaryThreadPrivacyDoesNotPromoteNestedContentKeysToAuthority(t *testing.T) {
	const secret = "sk-nested-ordinary-content"
	input := map[string]any{
		"id": "thread-nested-content", "status": "running",
		"turns": []any{map[string]any{
			"id": "turn-nested-content", "status": "running",
			"items": []any{map[string]any{
				"id": "item-nested-content", "kind": "user_message",
				"data": map[string]any{"caseId": secret, "workspaceRealPath": "/tmp/" + secret, "approvalId": secret},
			}},
		}},
	}
	projected, err := projectLegacyOrdinaryThreadPrivacyV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := json.Marshal(projected); strings.Contains(string(body), secret) {
		t.Fatalf("nested same-name content bypassed ordinary projection: %s", body)
	}
}

func TestProjectLegacyOrdinaryThreadPrivacyRejectsMalformedNullAuthorityFields(t *testing.T) {
	thread := map[string]any{
		"id":        "thr_explicit_null_authority",
		"workspace": "/tmp/authorized-workspace",
		"contextEpochState": map[string]any{
			"registry": nil,
			"acceptedSnapshot": map[string]any{
				"sources": nil,
			},
		},
		"turns": []any{nil},
	}
	before, _ := json.Marshal(thread)
	projected, err := projectLegacyOrdinaryThreadPrivacyV1(thread)
	if err == nil || projected != nil {
		t.Fatalf("malformed current authority was rewritten instead of failing closed: %#v err=%v", projected, err)
	}
	after, _ := json.Marshal(thread)
	if !bytes.Equal(after, before) {
		t.Fatalf("rejected malformed authority mutated input:\nwant=%s\ngot=%s", before, after)
	}
}

func TestProjectLegacyPrivateContentPreservesNullWhileRemovingPrivateSiblings(t *testing.T) {
	value := map[string]any{
		"safeNull": nil,
		"private": map[string]any{
			"kind": "assistant_reasoning",
			"text": "PRIVATE_REASONING_SENTINEL",
		},
		"values": []any{
			nil,
			map[string]any{"kind": "assistant_reasoning", "text": "PRIVATE_REASONING_SENTINEL"},
			nil,
		},
	}

	projected, removed := projectLegacyPrivateContentValueV1(value)
	public, _ := projected.(map[string]any)
	values, _ := public["values"].([]any)
	if _, present := public["safeNull"]; !present || public["safeNull"] != nil || public["private"] != nil {
		t.Fatalf("safe null or private sibling projection changed: %#v", public)
	}
	if len(values) != 2 || values[0] != nil || values[1] != nil {
		t.Fatalf("array null cardinality changed: %#v", values)
	}
	if removed&legacyReasoningContentRemovedV1 == 0 || public["providerPrivateContentRemoved"] != true {
		t.Fatalf("private sibling removal was not recorded: projected=%#v removed=%d", public, removed)
	}
}

func TestProjectLegacyPrivateContentDoesNotTrustGenericClosedFailureShape(t *testing.T) {
	failure := domainfailure.New(domainfailure.CodeTurnFailed, nil)
	record := map[string]any{
		"kind":     "turn_failed",
		"code":     failure.Code(),
		"message":  failure.Message(),
		"error":    failure.Message(),
		"severity": failure.Severity(),
	}

	projected, removed := projectLegacyPrivateContentValueV1(record)
	closed, _ := projected.(map[string]any)
	if removed&legacyFailureContentRemovedV1 == 0 || closed["error"] != nil || closed["failureContentRedacted"] != true {
		t.Fatalf("generic failure shape was trusted as host authority: projected=%#v removed=%d", projected, removed)
	}
}

func TestProjectLegacyPrivateContentPreservesDigestBoundGeneralTerminalRecords(t *testing.T) {
	context, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-migration-terminal", TurnID: "turn-migration-terminal",
		WorkspaceRealPath: "/workspace/migration-terminal", ContextEpoch: 2,
		IssuedAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	failure := domainfailure.New(domainfailure.CodeProviderTimeout, nil)
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingForOutcomeV1(context, "timeout", "failed", "")
	if err != nil {
		t.Fatal(err)
	}
	committedAt := "2026-07-15T00:00:00Z"
	drafts := []domainturnterminal.GeneralTerminalPublicationDraftV1{
		{Slot: "usage", Draft: map[string]any{
			"kind": "usage", "threadId": context.ThreadID, "turnId": context.TurnID,
			"model": "gpt-5", "usage": map[string]any{
				"totalTokens": float64(0), "cacheHitRate": nil,
				"cacheableTokenHitRate": nil, "totalInputTokenHitRate": nil,
			},
			"usageFinalStatus": "failed", "timestamp": committedAt,
		}},
		{Slot: "terminal", Draft: map[string]any{
			"kind": "turn_failed", "threadId": context.ThreadID, "turnId": context.TurnID,
			"status": "failed", "timestamp": committedAt, "terminalReason": "timeout",
			"code": failure.Code(), "message": failure.Message(), "error": failure.Message(),
			"severity": failure.Severity(), "generalTerminalCASBindingDigest": binding.BindingDigest,
		}},
	}
	commit, err := domainturnterminal.NewGeneralTerminalPublicationCommitV1(context, binding, committedAt, drafts)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := domainturnterminal.NewGeneralTerminalPublicationArchiveV1([]domainturnterminal.GeneralTerminalPublicationCommitV1{commit})
	if err != nil {
		t.Fatal(err)
	}

	for name, input := range map[string]map[string]any{
		"commit":  domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
		"archive": domainturnterminal.GeneralTerminalPublicationArchiveV1Map(archive),
	} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			projected, removed := projectLegacyPrivateContentValueV1(input)
			record, _ := projected.(map[string]any)
			after, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if removed != 0 || !bytes.Equal(after, before) {
				t.Fatalf("digest-bound terminal bytes changed: removed=%d before=%s after=%s", removed, before, after)
			}
			if name == "commit" {
				if _, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(record); err != nil {
					t.Fatalf("preserved commit no longer validates: %v", err)
				}
			} else if _, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(record); err != nil {
				t.Fatalf("preserved archive no longer validates: %v", err)
			}
			again, againRemoved := projectLegacyPrivateContentValueV1(record)
			againBody, _ := json.Marshal(again)
			if againRemoved != 0 || !bytes.Equal(againBody, after) {
				t.Fatalf("repeat projection changed terminal bytes: removed=%d first=%s second=%s", againRemoved, after, againBody)
			}
		})
	}

	tampered := domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit)
	tampered["terminalEvent"].(map[string]any)["error"] = domainfailure.New(domainfailure.CodeTurnFailed, nil).Message()
	if _, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(tampered); err == nil {
		t.Fatal("tampered terminal error retained digest-bound authority")
	}
	projected, removed := projectLegacyPrivateContentValueV1(tampered)
	projectedMap, _ := projected.(map[string]any)
	terminalEvent, _ := projectedMap["terminalEvent"].(map[string]any)
	if removed&legacyFailureContentRemovedV1 == 0 || terminalEvent["error"] != nil {
		t.Fatalf("tampered terminal did not fall back to legacy redaction: projected=%#v removed=%d", projected, removed)
	}
}

func TestMigratePrivateReasoningProjectsLegacyOrdinaryPIIWithoutChangingWorkspaceAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_legacy_ordinary_pii"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "thread.json")
	workspace := "/tmp/authorized-workspace-6222020202020202020"
	legacy := []byte(`{"id":"thr_legacy_ordinary_pii","workspace":"/tmp/authorized-workspace-6222020202020202020","title":"银行卡号: 6222020202020202020","turns":[{"id":"turn-1","items":[{"id":"user-1","kind":"user_message","text":"账号: 6222020202020202020"}]}]}`)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	input := PrivateReasoningMigrationInput{Root: root}
	if err := MigratePrivateReasoning(input); err != nil {
		t.Fatal(err)
	}
	thread := readMigrationFixtureObject(t, path)
	if thread["workspace"] != workspace {
		t.Fatalf("workspace authority changed: %#v", thread["workspace"])
	}
	if thread["title"] != "银行卡号: [ACCOUNT]" {
		t.Fatalf("legacy title PII was not projected: %#v", thread["title"])
	}
	turns, _ := thread["turns"].([]any)
	if len(turns) != 1 {
		t.Fatalf("migrated turns = %d, want 1", len(turns))
	}
	turn, _ := turns[0].(map[string]any)
	items, _ := turn["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("migrated items = %d, want 1", len(items))
	}
	item, _ := items[0].(map[string]any)
	if item["text"] != "账号: [ACCOUNT]" {
		t.Fatalf("legacy user-message PII was not projected: %#v", item["text"])
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(body, []byte("6222020202020202020")) != 1 {
		t.Fatal("legacy ordinary PII survived outside the exact workspace authority")
	}
	if !bytes.Contains(body, []byte("[ACCOUNT]")) {
		t.Fatalf("legacy ordinary PII was not deterministically projected: %s", body)
	}
	before := append([]byte(nil), body...)
	if err := MigratePrivateReasoning(input); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, before) {
		t.Fatal("repeat private-reasoning migration was not byte-stable")
	}
}

func TestMigratePrivateReasoningRedactsNestedLegacyFailureWithoutContentHash(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_legacy_nested_failure"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "thread.json")
	const sentinel = "PRIVATE_RAW_FAILURE_SENTINEL"
	legacy := []byte(`{"id":"thr_legacy_nested_failure","workspace":"/tmp/workspace","turns":[{"id":"turn-1","threadId":"thr_legacy_nested_failure","status":"failed","error":"PRIVATE_RAW_FAILURE_SENTINEL","items":[{"id":"error-1","threadId":"thr_legacy_nested_failure","turnId":"turn-1","kind":"error","message":"PRIVATE_RAW_FAILURE_SENTINEL","stack":"PRIVATE_RAW_FAILURE_SENTINEL"},{"id":"draft-1","threadId":"thr_legacy_nested_failure","turnId":"turn-1","kind":"assistant_text","status":"running","text":"PRIVATE_ASSISTANT_DRAFT_SENTINEL"},{"id":"user-1","kind":"user_message","text":"safe user text"}]}]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigratePrivateReasoning(PrivateReasoningMigrationInput{Root: root}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(sentinel)) || bytes.Contains(body, []byte("PRIVATE_ASSISTANT_DRAFT_SENTINEL")) || bytes.Contains(body, []byte("sourceHash")) {
		t.Fatalf("nested raw failure survived or gained a content-derived hash: %s", body)
	}
	thread := readMigrationFixtureObject(t, path)
	turns, _ := thread["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	if turn["failureContentRedacted"] != true || turn["assistantDraftRedacted"] != true || turn["error"] != nil {
		t.Fatalf("turn failure content was not closed: %#v", turn)
	}
	items, _ := turn["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("migrated items = %d, want unsafe failure removed and one safe sibling", len(items))
	}
	user, _ := items[0].(map[string]any)
	if user["text"] != "safe user text" {
		t.Fatalf("safe sibling item changed: %#v", user)
	}
	if _, err := threadapp.ProjectPublicThread(thread); err != nil {
		t.Fatalf("migrated thread is not publicly projectable: %v", err)
	}
}

func TestMigratePrivateReasoningNeverTombstonesCurrentTerminalAuthority(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_current_authority_private_content"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	threadBefore := []byte(`{"id":"thr_current_authority_private_content","turns":[]}`)
	threadPath := filepath.Join(threadDir, "thread.json")
	if err := os.WriteFile(threadPath, threadBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	eventsBefore := []byte("{\"kind\":\"tool_progress\",\"threadId\":\"thr_current_authority_private_content\",\"seq\":1}\n" +
		"{\"kind\":\"tool_progress\",\"threadId\":\"thr_current_authority_private_content\",\"seq\":3,\"acceptedFinal\":null,\"message\":\"Authorization: Bearer opaque-current-secret\"}\n" +
		"{\"kind\":\"tool_progress\",\"threadId\":\"thr_current_authority_private_content\",\"seq\":2}\n")
	eventsPath := filepath.Join(threadDir, "events.jsonl")
	if err := os.WriteFile(eventsPath, eventsBefore, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigratePrivateReasoning(PrivateReasoningMigrationInput{Root: root}); err == nil {
		t.Fatal("private-content migration tombstoned malformed current terminal authority")
	}
	threadAfter, threadErr := os.ReadFile(threadPath)
	eventsAfter, eventsErr := os.ReadFile(eventsPath)
	if threadErr != nil || eventsErr != nil || !bytes.Equal(threadAfter, threadBefore) || !bytes.Equal(eventsAfter, eventsBefore) {
		t.Fatalf("rejected current authority mutated source bytes: threadErr=%v eventsErr=%v", threadErr, eventsErr)
	}
}

func TestPreflightReasoningEventAuthorityRejectsNonStrictCurrentRecord(t *testing.T) {
	for name, line := range map[string]string{
		"duplicate key": `{"kind":"tool_progress","threadId":"thr_non_strict","seq":1,"approvalItemId":"first","approvalItemId":"second"}`,
		"trailing data": `{"kind":"tool_progress","threadId":"thr_non_strict","seq":1,"datasetSnapshotId":"snapshot"} trailing`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "events.jsonl")
			before := []byte(line + "\n")
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := preflightReasoningEventAuthorityV1(path); err == nil {
				t.Fatal("non-strict current authority record passed preflight")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("rejected non-strict authority mutated source: err=%v", err)
			}
		})
	}
}

func TestMigratePrivateReasoningPreservesMaskedMessageCardinalityAndAuditMarkers(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_legacy_message_privacy"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadDir, "messages.jsonl")
	const privateReasoning = "PRIVATE_REASONING_MESSAGE_SENTINEL"
	legacy := []byte(
		"{\"id\":\"user-1\",\"threadId\":\"thr_legacy_message_privacy\",\"turnId\":\"turn-1\",\"kind\":\"user_message\",\"text\":\"银行卡号: 6222020202020202020\"}\n" +
			"{\"id\":\"reasoning-1\",\"threadId\":\"thr_legacy_message_privacy\",\"turnId\":\"turn-1\",\"kind\":\"assistant_reasoning\",\"text\":\"" + privateReasoning + "\"}\n" +
			"{\"id\":\"assistant-1\",\"threadId\":\"thr_legacy_message_privacy\",\"turnId\":\"turn-1\",\"kind\":\"assistant_text\",\"text\":\"账号: 6222020202020202020\"}\n",
	)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	input := PrivateReasoningMigrationInput{Root: root}
	if err := MigratePrivateReasoning(input); err != nil {
		t.Fatal(err)
	}
	records := readMigrationFixtureLines(t, path)
	if len(records) != 3 {
		t.Fatalf("migrated message records = %d, want 3", len(records))
	}
	if records[0]["kind"] != "user_message" || records[0]["text"] != "银行卡号: [ACCOUNT]" {
		t.Fatalf("legacy user message was not retained with masking: %#v", records[0])
	}
	if records[1]["kind"] != "content_redacted" || records[1]["code"] != "private_message_content_removed" || records[1]["sourceHash"] != nil {
		t.Fatalf("private reasoning audit marker mismatch: %#v", records[1])
	}
	if records[2]["kind"] != "assistant_text" || records[2]["text"] != "账号: [ACCOUNT]" {
		t.Fatalf("legacy assistant final was not retained with masking: %#v", records[2])
	}
	body, _ := os.ReadFile(path)
	for _, forbidden := range []string{"6222020202020202020", privateReasoning, "sourceHash"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("private message bytes survived migration (%s): %s", forbidden, body)
		}
	}
	before := append([]byte(nil), body...)
	if err := MigratePrivateReasoning(input); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, before) {
		t.Fatal("repeat private message migration was not byte-stable")
	}
}
