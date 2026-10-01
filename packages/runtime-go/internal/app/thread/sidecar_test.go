package thread

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeForReadAppliesDurableThreadDefaults(t *testing.T) {
	thread, err := NormalizeForRead("thr_1", map[string]any{
		"title": "New thread",
		"turns": []any{
			map[string]any{"id": "turn_1", "prompt": "Hello from prompt", "reasoningEffort": ""},
		},
		"model": " ", "reasoningEffort": "",
	}, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if thread["id"] != "thr_1" || thread["title"] != "Hello from prompt" {
		t.Fatalf("identity/title mismatch: %#v", thread)
	}
	if _, ok := thread["model"]; ok {
		t.Fatalf("empty model should be removed: %#v", thread)
	}
	if _, ok := thread["reasoningEffort"]; ok {
		t.Fatalf("legacy empty reasoning effort should be removed: %#v", thread)
	}
	turn, _ := listAny(thread["turns"])[0].(map[string]any)
	if _, ok := turn["reasoningEffort"]; ok {
		t.Fatalf("nested legacy empty reasoning effort should be removed: %#v", turn)
	}
	if thread["mode"] != "agent" || thread["status"] != "idle" || thread["relation"] != "primary" {
		t.Fatalf("default fields mismatch: %#v", thread)
	}
	if thread["createdAt"] != "2026-07-03T00:00:00Z" || thread["updatedAt"] != "2026-07-03T00:00:00Z" {
		t.Fatalf("timestamps mismatch: %#v", thread)
	}
}

func TestValidateCaseHistoryPreservesWorkspaceAuthorityButRejectsOrdinaryPII(t *testing.T) {
	const account = "6222020202020202020"
	thread := map[string]any{
		"id": "thr_case_workspace", "caseId": "case-workspace", "title": "案件分析",
		"workspace": "/controlled/cases/" + account, "turns": []any{},
	}
	if err := ValidatePublicHistory(thread); err != nil {
		t.Fatalf("exact workspace authority was rejected as ordinary PII: %v", err)
	}
	projected, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(projected)
	if _, present := projected["workspace"]; present || strings.Contains(string(body), account) {
		t.Fatalf("workspace authority crossed the case public projection: %s", body)
	}

	thread["title"] = "账号: " + account
	if err := ValidatePublicHistory(thread); err == nil {
		t.Fatal("ordinary case history accepted a restricted account number")
	}
}

func TestValidateOrdinaryHistoryPreservesWorkspaceAuthorityAndProjectsItsPublicView(t *testing.T) {
	const accountLikePathSegment = "1234567890123456789"
	thread := map[string]any{
		"id": "thr_ordinary_workspace", "title": "Ordinary workspace", "status": "idle",
		"workspace": "/tmp/analytix-test-" + accountLikePathSegment, "turns": []any{},
	}
	if err := ValidatePublicHistory(thread); err != nil {
		t.Fatalf("exact ordinary workspace authority was rejected as public content: %v", err)
	}
	projected, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := projected["workspace"].(string)
	if strings.Contains(workspace, accountLikePathSegment) || !strings.Contains(workspace, "[ACCOUNT]") {
		t.Fatalf("ordinary public workspace was not privacy projected: %#v", projected)
	}
	if thread["workspace"] != "/tmp/analytix-test-"+accountLikePathSegment {
		t.Fatalf("workspace authority was mutated during validation/projection: %#v", thread)
	}

	thread["title"] = "账号: " + accountLikePathSegment
	if err := ValidatePublicHistory(thread); err == nil {
		t.Fatal("ordinary history accepted restricted PII outside workspace authority")
	}
}

func TestLongOrdinaryHistoryKeepsPerRecordProjectionBounds(t *testing.T) {
	const threadID = "thr_long_ordinary"
	turns := make([]any, 0, 120)
	for turnNumber := 1; turnNumber <= 120; turnNumber++ {
		turnID := fmt.Sprintf("turn_%03d", turnNumber)
		items := make([]any, 0, 24)
		for itemNumber := 1; itemNumber <= 24; itemNumber++ {
			items = append(items, map[string]any{
				"id":       fmt.Sprintf("item_%03d_%02d", turnNumber, itemNumber),
				"threadId": threadID, "turnId": turnID, "kind": "user_message",
				"text": "A synthetic public history item", "status": "completed",
			})
		}
		turns = append(turns, map[string]any{
			"id": turnID, "threadId": threadID, "status": "completed",
			"prompt": "Continue synthetic history", "items": items,
		})
	}
	thread := map[string]any{
		"id": threadID, "title": "Synthetic long history", "workspace": "/tmp/ordinary-history",
		"status": "idle", "turns": turns,
	}
	if err := ValidatePublicHistory(thread); err != nil {
		t.Fatalf("valid 120-turn history was rejected: %v", err)
	}
	projected, err := ProjectPublicThread(thread)
	publicTurns, _ := projected["turns"].([]any)
	if err != nil || len(publicTurns) != len(turns) {
		t.Fatalf("120-turn public history was truncated: turns=%d err=%v", len(publicTurns), err)
	}
	item := turns[59].(map[string]any)["items"].([]any)[12].(map[string]any)
	item["text"] = "sk-synthetic-private-token-123456"
	if err := ValidatePublicHistory(thread); err == nil {
		t.Fatal("credential in a later turn passed the per-record boundary")
	}
	item["text"] = strings.Repeat("x", (1<<20)+1)
	if err := ValidatePublicHistory(thread); err == nil {
		t.Fatal("oversized individual item passed the fixed credential boundary")
	}
}

func TestMessageSidecarCannotPersistOrRehydrateTerminalOutput(t *testing.T) {
	thread, _, sentinel := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	missing, _, err := MissingMessageSidecarItems(thread, nil)
	body, _ := json.Marshal(missing)
	if err != nil || strings.Contains(string(body), sentinel) || strings.Contains(string(body), "assistant_text") {
		t.Fatalf("canonical terminal output entered messages sidecar: %s err=%v", body, err)
	}
	metadata := MetadataSidecarEntry(thread, "2026-07-15T00:00:01Z")
	metadataBody, _ := json.Marshal(metadata)
	if strings.Contains(string(metadataBody), sentinel) {
		t.Fatalf("terminal output entered metadata sidecar preview: %s", metadataBody)
	}

	assistant := map[string]any{
		"id": "sidecar-assistant", "threadId": "thread-sidecar", "turnId": "turn-sidecar",
		"kind": "assistant_text", "role": "assistant", "status": "completed", "text": sentinel,
	}
	if items := LatestMessageSidecarItems([]map[string]any{assistant}); len(items) != 0 {
		t.Fatalf("unbound assistant survived sidecar inventory: %#v", items)
	}
	hydrated := HydrateSidecarItems(HydrateSidecarInput{
		ThreadID: "thread-sidecar",
		Thread: map[string]any{"id": "thread-sidecar", "turns": []any{map[string]any{
			"id": "turn-sidecar", "threadId": "thread-sidecar", "status": "completed", "items": []any{},
		}}},
		Items: []map[string]any{assistant}, FallbackTime: "2026-07-15T00:00:00Z",
	})
	hydratedBody, _ := json.Marshal(hydrated)
	if strings.Contains(string(hydratedBody), sentinel) || strings.Contains(string(hydratedBody), "sidecar-assistant") {
		t.Fatalf("unbound assistant was rehydrated from sidecar: %s", hydratedBody)
	}
}

func TestNormalizeForReadDropsPersistedReasoning(t *testing.T) {
	thread, err := NormalizeForRead("thr_1", map[string]any{
		"turns": []any{map[string]any{
			"id": "turn_1",
			"items": []any{
				map[string]any{"id": "reasoning", "kind": "assistant_reasoning", "text": "private scratchpad"},
				map[string]any{"id": "tool", "kind": "tool_call", "reasoningContent": "private tool reasoning"},
				map[string]any{"id": "answer", "kind": "assistant_text", "text": "<think>private inline reasoning</think>public answer"},
			},
		}},
	}, "2026-07-03T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(thread["turns"])
	turn, _ := turns[0].(map[string]any)
	items := listAny(turn["items"])
	if len(items) != 2 {
		t.Fatalf("reasoning item must be removed from public history: %#v", thread)
	}
	tool, _ := items[0].(map[string]any)
	if _, ok := tool["reasoningContent"]; ok {
		t.Fatalf("tool reasoning must be removed from public history: %#v", tool)
	}
	if answer, _ := items[1].(map[string]any); answer["text"] != "public answer" {
		t.Fatalf("public answer must be preserved: %#v", items)
	}
}

func TestNormalizeForReadRejectsInvalidReasoningMetadataWithoutSynthesizingAnEmptyThread(t *testing.T) {
	thread, err := NormalizeForRead("thr_1", map[string]any{
		"id": "thr_1",
		"turns": []any{map[string]any{
			"id": "turn_1", "reasoningEffort": "PRIVATE_REASONING_SENTINEL",
		}},
	}, "2026-07-03T00:00:00Z")
	if !errors.Is(err, ErrDurableHistorySanitization) || thread != nil {
		t.Fatalf("invalid durable history must fail closed: thread=%#v err=%v", thread, err)
	}
}

func TestHydrateSidecarItemsOnlyFillsKnownEmptyTurn(t *testing.T) {
	source := map[string]any{
		"id":        "thr_1",
		"updatedAt": "fallback-time",
		"turns": []any{
			map[string]any{"id": "turn_nonempty", "createdAt": "2026-07-03T00:00:00Z", "items": []any{
				map[string]any{"id": "primary", "turnId": "turn_nonempty", "threadId": "thr_1", "kind": "user_message", "text": "Primary"},
			}},
			map[string]any{"id": "turn_2", "createdAt": "2026-07-03T00:02:00Z", "items": []any{}},
		},
	}
	items := []map[string]any{
		{"id": "item_1_user", "turnId": "turn_1", "threadId": "thr_1", "kind": "user_message", "text": "First", "status": "completed", "createdAt": "2026-07-03T00:01:00Z", "finishedAt": "2026-07-03T00:01:01Z"},
		{"id": "sidecar_nonempty", "turnId": "turn_nonempty", "threadId": "thr_1", "kind": "user_message", "text": "Overwrite", "status": "completed", "createdAt": "2026-07-03T00:00:00Z"},
		{"id": "item_2_user", "turnId": "turn_2", "threadId": "thr_1", "kind": "user_message", "text": "Second", "status": "completed", "attachmentIds": []any{"att_1"}, "createdAt": "2026-07-03T00:02:00Z"},
	}
	hydrated := HydrateSidecarItems(HydrateSidecarInput{
		ThreadID:     "thr_1",
		Thread:       source,
		Items:        items,
		FallbackTime: "fallback-time",
	})
	turns, _ := hydrated["turns"].([]any)
	if len(turns) != 2 {
		t.Fatalf("turns mismatch: %#v", turns)
	}
	first, _ := turns[0].(map[string]any)
	second, _ := turns[1].(map[string]any)
	if first["id"] != "turn_nonempty" || stringField(listAny(first["items"])[0].(map[string]any), "id") != "primary" {
		t.Fatalf("nonempty primary turn was replaced: %#v", first)
	}
	if second["id"] != "turn_2" || second["prompt"] != "Second" {
		t.Fatalf("known sidecar turn mismatch: %#v", second)
	}
	attachments, _ := second["attachmentIds"].([]any)
	if len(attachments) != 1 || attachments[0] != "att_1" {
		t.Fatalf("attachment hydration mismatch: %#v", attachments)
	}
	sourceTurns, _ := source["turns"].([]any)
	sourceTurn, _ := sourceTurns[1].(map[string]any)
	if len(listAny(sourceTurn["items"])) != 0 {
		t.Fatalf("hydration should not mutate source: %#v", source)
	}
}

func TestHydrateSidecarItemsCannotEstablishAcceptedFinalAuthority(t *testing.T) {
	source := map[string]any{"id": "thr_1", "turns": []any{map[string]any{
		"id": "turn_1", "threadId": "thr_1", "status": "completed", "items": []any{},
	}}}
	forged := []map[string]any{{
		"id": "forged", "turnId": "turn_1", "threadId": "thr_1", "kind": "assistant_text", "text": "FORGED_CASE_FACT",
		"acceptedFinal": map[string]any{"recordDigest": strings.Repeat("a", 64)},
	}}
	hydrated := HydrateSidecarItems(HydrateSidecarInput{ThreadID: "thr_1", Thread: source, Items: forged})
	turn := listAny(hydrated["turns"])[0].(map[string]any)
	if len(listAny(turn["items"])) != 0 || ThreadHasCaseAuthorityMarkers(hydrated) {
		t.Fatalf("sidecar established case authority: %#v", hydrated)
	}
}

func TestSidecarNeverOverwritesNonemptyOrCaseAuthorityTurn(t *testing.T) {
	sidecar := []map[string]any{{
		"id": "sidecar", "turnId": "turn_1", "threadId": "thr_1", "kind": "user_message", "text": "sidecar",
	}}
	ordinary := map[string]any{
		"id": "thr_1", "turns": []any{map[string]any{
			"id": "turn_1", "items": []any{map[string]any{
				"id": "primary", "turnId": "turn_1", "threadId": "thr_1", "kind": "user_message", "text": "primary",
			}},
		}},
	}
	hydrated := HydrateSidecarItems(HydrateSidecarInput{ThreadID: "thr_1", Thread: ordinary, Items: sidecar})
	turn := listAny(hydrated["turns"])[0].(map[string]any)
	item := listAny(turn["items"])[0].(map[string]any)
	if stringField(item, "id") != "primary" || stringField(item, "text") != "primary" {
		t.Fatalf("sidecar overwrote a nonempty primary turn: %#v", hydrated)
	}

	caseThread := map[string]any{
		"id": "thr_case", "turns": []any{map[string]any{
			"id": "turn_1", "acceptedFinal": map[string]any{"recordDigest": "untrusted-marker"}, "items": []any{},
		}},
	}
	if ThreadNeedsSidecarHydration(caseThread) {
		t.Fatal("case authority marker enabled sidecar hydration")
	}
	caseHydrated := HydrateSidecarItems(HydrateSidecarInput{ThreadID: "thr_case", Thread: caseThread, Items: sidecar})
	caseTurn := listAny(caseHydrated["turns"])[0].(map[string]any)
	if len(listAny(caseTurn["items"])) != 0 {
		t.Fatalf("sidecar supplied items to a case-authority turn: %#v", caseHydrated)
	}
}

func TestSidecarMetadataAndMessageProjection(t *testing.T) {
	metadata := []map[string]any{
		{"kind": "ignored", "thread": map[string]any{"id": "thr_1", "title": "Ignored"}},
		{"kind": "thread_metadata", "thread": map[string]any{"id": "other", "title": "Other"}},
		{"kind": "thread_metadata", "thread": map[string]any{"id": "thr_1", "title": "Old"}},
		{"kind": "thread_metadata", "thread": map[string]any{"id": "thr_1", "title": "New"}},
	}
	latest := LatestMetadataSidecarThread("thr_1", metadata)
	if latest["title"] != "New" {
		t.Fatalf("latest metadata mismatch: %#v", latest)
	}
	latest["title"] = "mutated"
	if metadata[3]["thread"].(map[string]any)["title"] != "New" {
		t.Fatalf("latest metadata should be cloned: %#v", metadata[3])
	}

	items := LatestMessageSidecarItems([]map[string]any{
		{"id": "item_1", "kind": "user_message", "status": "completed", "text": "old"},
		{"id": " ", "kind": "user_message", "status": "completed", "text": "skip"},
		{"id": "item_2", "kind": "user_message", "status": "completed", "text": "second"},
		{"id": "item_1", "kind": "user_message", "status": "completed", "text": "new"},
		{"id": "redacted", "kind": "content_redacted", "status": "completed", "text": "audit tombstone"},
		{"id": "missing-kind", "status": "completed", "text": "unknown"},
		{"id": "unknown-kind", "kind": "future_untrusted_kind", "status": "completed", "text": "unknown"},
	})
	if len(items) != 2 || items[0]["text"] != "second" || items[1]["text"] != "new" {
		t.Fatalf("latest closed items mismatch: %#v", items)
	}
}

func TestMetadataSidecarWithholdsGeneralTerminalRecoveryAuthority(t *testing.T) {
	thread := map[string]any{
		"id": "thr_1", "generalTerminalPublicationArchive": map[string]any{"archiveDigest": "GENERAL_ARCHIVE_SIDECAR_SENTINEL"},
		"turns": []any{map[string]any{
			"id": "turn_1", "status": "completed",
			"generalTerminalPublication": map[string]any{"commitId": "GENERAL_OUTBOX_SIDECAR_SENTINEL"},
			"generalTerminalCASBinding":  map[string]any{"bindingDigest": "GENERAL_BINDING_SIDECAR_SENTINEL"},
			"items": []any{map[string]any{
				"id": "assistant", "kind": "assistant_text", "text": "public answer",
				"generalTerminalCASBinding": map[string]any{"bindingDigest": "GENERAL_ITEM_BINDING_SENTINEL"},
			}},
		}},
	}
	entry := MetadataSidecarEntry(thread, "2026-07-15T00:00:00Z")
	body, _ := json.Marshal(entry)
	for _, forbidden := range []string{
		"generalTerminal", "GENERAL_OUTBOX_SIDECAR_SENTINEL", "GENERAL_ARCHIVE_SIDECAR_SENTINEL",
		"GENERAL_BINDING_SIDECAR_SENTINEL", "GENERAL_ITEM_BINDING_SENTINEL",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("metadata sidecar retained private terminal authority %q: %s", forbidden, body)
		}
	}
}

func TestSidecarPrivateTerminalAuthorityDetectionIsRecursiveAndKeyNormalized(t *testing.T) {
	for _, value := range []any{
		map[string]any{"generalTerminalPublicationArchive": map[string]any{}},
		map[string]any{"nested": []any{map[string]any{"general_terminal_cas_binding": map[string]any{}}}},
		map[string]any{"item": map[string]any{"GENERAL-TERMINAL-EVENT-ID": "forged"}},
	} {
		if !SidecarValueHasPrivateTerminalAuthority(value) {
			t.Fatalf("private terminal authority shape was not detected: %#v", value)
		}
	}
	if SidecarValueHasPrivateTerminalAuthority(map[string]any{"text": "generalTerminalPublication is user text"}) {
		t.Fatal("user text value was mistaken for a private terminal authority key")
	}
}

func TestShouldReplacePlaceholderThread(t *testing.T) {
	if !ShouldReplacePlaceholderThread("thread.json", map[string]any{"id": "thr_1", "title": "Recovered"}, map[string]any{"id": "thr_1", "title": "New thread", "turns": []any{}}) {
		t.Fatal("recoverable source should replace placeholder thread")
	}
	if ShouldReplacePlaceholderThread("events.jsonl", map[string]any{"title": "Recovered"}, map[string]any{"title": "New thread"}) {
		t.Fatal("only thread.json should be replaceable")
	}
	if ShouldReplacePlaceholderThread("thread.json", map[string]any{"id": "left", "title": "Recovered"}, map[string]any{"id": "right", "title": "New thread"}) {
		t.Fatal("mismatched ids should not replace")
	}
	if ShouldReplacePlaceholderThread("thread.json", map[string]any{"title": "New thread"}, map[string]any{"title": "Custom"}) {
		t.Fatal("non-recoverable source or non-placeholder target should not replace")
	}
}

func TestTurnFromSidecarItemsDetectsRunningStatus(t *testing.T) {
	turn := TurnFromSidecarItems("thr_1", "turn_1", []map[string]any{
		{"id": "item_1_user", "turnId": "turn_1", "kind": "user_message", "text": "Prompt", "status": "completed", "createdAt": "created"},
		{"id": "item_1_tool", "turnId": "turn_1", "kind": "tool_call", "status": "running"},
	}, "fallback")
	if turn["status"] != "running" {
		t.Fatalf("turn should be running: %#v", turn)
	}
	if _, ok := turn["finishedAt"]; ok {
		t.Fatalf("running sidecar turn should not be finished: %#v", turn)
	}
}

func TestSidecarStripSummaryAndThreadItems(t *testing.T) {
	thread := map[string]any{
		"id":    "thr_1",
		"title": "Title",
		"turns": []any{
			map[string]any{"id": "turn_1", "prompt": "Prompt", "items": []any{
				map[string]any{"id": "item_1", "kind": "user_message", "text": "Hello"},
			}},
		},
	}
	stripped := StripItemsForSidecar(thread)
	turns, _ := stripped["turns"].([]any)
	turn, _ := turns[0].(map[string]any)
	if turn["prompt"] != "" || len(listAny(turn["items"])) != 0 {
		t.Fatalf("strip mismatch: %#v", stripped)
	}
	items := ThreadItemsInOrder(thread)
	if len(items) != 1 || items[0]["id"] != "item_1" {
		t.Fatalf("items in order mismatch: %#v", items)
	}
	summary := SidecarSummary(thread)
	if summary["schemaVersion"] != float64(1) || summary["messageCount"] != float64(1) || summary["turnCount"] != float64(1) {
		t.Fatalf("summary mismatch: %#v", summary)
	}
	entry := MetadataSidecarEntry(thread, "now")
	if entry["kind"] != "thread_metadata" || entry["timestamp"] != "now" {
		t.Fatalf("metadata entry mismatch: %#v", entry)
	}
	existing, err := MessageSidecarJSONByID(items)
	if err != nil {
		t.Fatalf("message json by id: %v", err)
	}
	missing, updated, err := MissingMessageSidecarItems(thread, existing)
	if err != nil {
		t.Fatalf("missing message sidecars: %v", err)
	}
	if len(missing) != 0 || len(updated) != 1 {
		t.Fatalf("matching item should not append: missing=%#v updated=%#v", missing, updated)
	}
	thread["turns"] = []any{
		map[string]any{"id": "turn_1", "items": []any{
			map[string]any{"id": "item_1", "kind": "user_message", "text": "Changed"},
		}},
	}
	oldJSON := existing["item_1"]
	missing, updated, err = MissingMessageSidecarItems(thread, existing)
	if err != nil {
		t.Fatalf("changed sidecar item: %v", err)
	}
	if len(missing) != 1 || updated["item_1"] == oldJSON {
		t.Fatalf("changed item should append and update json: missing=%#v updated=%#v", missing, updated)
	}
}

func TestThreadTopLevelPrivateReasoningNeverLeavesRuntime(t *testing.T) {
	thread := map[string]any{
		"id":               "thr_private",
		"reasoningContent": "PRIVATE_TOP_LEVEL",
		"turns": []any{map[string]any{
			"id":               "turn_1",
			"thinking_content": "PRIVATE_TURN_LEVEL",
			"items": []any{
				map[string]any{"id": "user_1", "kind": "user_message", "text": "literal <think>user example</think>"},
				map[string]any{"id": "assistant_1", "kind": "assistant_text", "text": "<think>PRIVATE_ITEM</think>public answer"},
			},
		}},
	}
	public := SanitizePublicHistory(thread)
	entry := MetadataSidecarEntry(public, "now")
	data, err := json.Marshal(map[string]any{"thread": public, "metadata": entry})
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, forbidden := range []string{"PRIVATE_TOP_LEVEL", "PRIVATE_TURN_LEVEL", "PRIVATE_ITEM", "reasoningContent", "thinking_content"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public thread/metadata retained %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, "literal") || !strings.Contains(serialized, "user example") || strings.Contains(serialized, "public answer") {
		t.Fatalf("unbound assistant text survived or user-authored text was lost: %s", serialized)
	}
}
