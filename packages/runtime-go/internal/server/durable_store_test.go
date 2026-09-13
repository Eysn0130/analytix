package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	subagentstartupport "analytix.local/runtime-go/internal/ports/subagentstartup"
	provider "analytix.local/runtime-go/internal/provider"
	casepublication "analytix.local/runtime-go/internal/testsupport/casepublication"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

const serverPositiveTestTimeout = 15 * time.Second

func TestDurableStoreRecordEventsAtomicPersistsAndPublishesContiguousBundle(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "Atomic progress bundle"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_atomic_progress"
	before, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	persistCalls := 0
	store.beforePersistEventHook = func() {
		persistCalls++
	}
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()

	recorded, order, err := store.RecordEventsAtomic([]map[string]any{
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "setup",
			Details: map[string]any{"workspaceBound": true, "caseBound": false},
		}),
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "pre_start",
			Details: map[string]any{"approvalPolicy": "on-request", "sandboxMode": "workspace-write"},
		}),
	})
	if err != nil {
		t.Fatalf("record atomic event bundle: %v", err)
	}
	if persistCalls != 1 || len(recorded) != 2 || len(order) != 2 || order[0] != "persist" || order[1] != "publish" {
		t.Fatalf("atomic event bundle result mismatch: persistCalls=%d recorded=%#v order=%#v", persistCalls, recorded, order)
	}
	firstSeq, firstOK := numericSeq(recorded[0]["seq"])
	secondSeq, secondOK := numericSeq(recorded[1]["seq"])
	if !firstOK || !secondOK || secondSeq != firstSeq+1 {
		t.Fatalf("atomic event bundle sequence mismatch: %#v", recorded)
	}
	for index, stage := range []string{"setup", "pre_start"} {
		select {
		case event := <-live:
			if stringField(event, "stage") != stage || event["seq"] != recorded[index]["seq"] {
				t.Fatalf("live atomic event %d mismatch: event=%#v recorded=%#v", index, event, recorded[index])
			}
		case <-time.After(time.Second):
			t.Fatalf("live atomic event %d was not published", index)
		}
	}
	after, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Events) != len(before.Events)+2 ||
		stringField(after.Events[len(after.Events)-2], "stage") != "setup" ||
		stringField(after.Events[len(after.Events)-1], "stage") != "pre_start" {
		t.Fatalf("atomic event bundle replay mismatch: before=%#v after=%#v", before.Events, after.Events)
	}

	beforeInvalidCount := len(after.Events)
	if _, _, err := store.RecordEventsAtomic([]map[string]any{
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "input_received",
			Details: map[string]any{"stepIndex": float64(0), "promptBytes": float64(4)},
		}),
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: "thr_different", TurnID: turnID, Stage: "input_cached",
			Details: map[string]any{"messageCount": float64(1)},
		}),
	}); err == nil {
		t.Fatal("mixed-thread event bundle should fail closed")
	}
	afterInvalid, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterInvalid.Events) != beforeInvalidCount {
		t.Fatalf("rejected event bundle changed replay: before=%d after=%d", beforeInvalidCount, len(afterInvalid.Events))
	}
	select {
	case event := <-live:
		t.Fatalf("rejected event bundle published a live prefix: %#v", event)
	default:
	}
	if _, _, err := store.RecordEventsAtomic([]map[string]any{
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "input_received",
			Details: map[string]any{"stepIndex": float64(0), "promptBytes": float64(4)},
		}),
		{
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
			"stage": "provider_invented", "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		},
	}); err == nil {
		t.Fatal("bundle with a late invalid public event should fail closed")
	}
	afterMalformed, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterMalformed.Events) != beforeInvalidCount {
		t.Fatalf("late invalid bundle changed replay: before=%d after=%d", beforeInvalidCount, len(afterMalformed.Events))
	}
	select {
	case event := <-live:
		t.Fatalf("late invalid event bundle published a live prefix: %#v", event)
	default:
	}

	continued, _, err := store.RecordEventsAtomic([]map[string]any{
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "input_received",
			Details: map[string]any{"stepIndex": float64(0), "promptBytes": float64(4)},
		}),
	})
	if err != nil {
		t.Fatalf("continue atomic event sequence after rejected bundle: %v", err)
	}
	continuedSeq, continuedOK := numericSeq(continued[0]["seq"])
	if !continuedOK || continuedSeq != secondSeq+1 {
		t.Fatalf("rejected bundle consumed a sequence: previous=%d continued=%#v", secondSeq, continued)
	}
	select {
	case event := <-live:
		if event["seq"] != continued[0]["seq"] {
			t.Fatalf("continued live event sequence mismatch: event=%#v continued=%#v", event, continued[0])
		}
	case <-time.After(time.Second):
		t.Fatal("continued atomic event was not published")
	}
}

func TestDurableStoreRecordEventsAtomicDoesNotPublishOnPersistenceFailure(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "Atomic persistence failure"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_atomic_persist_failure"
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()

	store.beforePersistEventHook = func() {
		if err := os.MkdirAll(store.eventsPath(threadID), 0o700); err != nil {
			t.Errorf("create event-log obstruction: %v", err)
		}
	}
	_, _, err = store.RecordEventsAtomic([]map[string]any{
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "setup",
			Details: map[string]any{"workspaceBound": true, "caseBound": false},
		}),
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "pre_start",
			Details: map[string]any{"approvalPolicy": "on-request", "sandboxMode": "workspace-write"},
		}),
	})
	if err == nil {
		t.Fatal("atomic event bundle should fail when its event log cannot be persisted")
	}
	select {
	case event := <-live:
		t.Fatalf("persistence failure published a live event: %#v", event)
	default:
	}

	store.beforePersistEventHook = nil
	if removeErr := os.Remove(store.eventsPath(threadID)); removeErr != nil {
		t.Fatalf("remove test-owned event-log obstruction: %v", removeErr)
	}
	recorded, _, err := store.RecordEventsAtomic([]map[string]any{
		apploop.BuildPipelineStageEvent(apploop.PipelineStageEventInput{
			ThreadID: threadID, TurnID: turnID, Stage: "setup",
			Details: map[string]any{"workspaceBound": true, "caseBound": false},
		}),
	})
	if err != nil {
		t.Fatalf("record after persistence failure: %v", err)
	}
	if seq, ok := numericSeq(recorded[0]["seq"]); !ok || seq != 1 {
		t.Fatalf("persistence failure advanced the sequence frontier: %#v", recorded)
	}
}

func TestDurableStoreRejectsSafeRecordAliasesBeforeForkOrResume(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateThread(map[string]any{"title": "private ordinary history"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(source, "id")
	alias := strings.Replace(threadID, "_", ":", 1)
	if alias == threadID {
		t.Fatalf("test did not construct a colliding alias for %q", threadID)
	}

	if fork, forkErr := store.ForkThread(alias, map[string]any{"title": "must not exist"}); !errors.Is(forkErr, os.ErrNotExist) || fork != nil {
		t.Fatalf("fork accepted storage alias %q: fork=%#v err=%v", alias, fork, forkErr)
	}
	if resumed, resumeErr := store.ResumeSession(alias, map[string]any{}); !errors.Is(resumeErr, os.ErrNotExist) || resumed != nil {
		t.Fatalf("resume accepted storage alias %q: response=%#v err=%v", alias, resumed, resumeErr)
	}
	threads, err := store.ListThreads(false, false, false, "")
	if err != nil || len(threads) != 1 || stringField(threads[0], "id") != threadID {
		t.Fatalf("alias attempts changed durable thread inventory: threads=%#v err=%v", threads, err)
	}
	store.mu.Lock()
	meta, metaErr := store.readMetaNoLock()
	store.mu.Unlock()
	if metaErr != nil || meta.ForkCounter != 0 || meta.ResumeCounter != 0 {
		t.Fatalf("alias attempts advanced durable counters: meta=%#v err=%v", meta, metaErr)
	}
}

func TestDurableStoreRejectsThreadBodyIdentityMismatchBeforeForkOrResume(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateThread(map[string]any{"title": "identity-bound history"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(source, "id")
	tampered := cloneMap(source)
	tampered["id"] = "thr_foreign"
	body, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.threadPath(threadID), body, 0o600); err != nil {
		t.Fatal(err)
	}

	if fork, forkErr := store.ForkThread(threadID, map[string]any{}); forkErr == nil || fork != nil {
		t.Fatalf("fork accepted mismatched durable body: fork=%#v err=%v", fork, forkErr)
	}
	if resumed, resumeErr := store.ResumeSession(threadID, map[string]any{}); resumeErr == nil || resumed != nil {
		t.Fatalf("resume accepted mismatched durable body: response=%#v err=%v", resumed, resumeErr)
	}
	store.mu.Lock()
	meta, metaErr := store.readMetaNoLock()
	store.mu.Unlock()
	if metaErr != nil || meta.ForkCounter != 0 || meta.ResumeCounter != 0 {
		t.Fatalf("identity mismatch advanced durable counters: meta=%#v err=%v", meta, metaErr)
	}
}

func TestDurableUpsertRejectsUnknownSidecarLifecycleBeforePrimaryWrite(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "closed sidecar projection"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	before, err := os.ReadFile(store.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	invalid := cloneMap(thread)
	invalid["turns"] = []any{map[string]any{
		"id": "turn_unknown_sidecar", "threadId": threadID, "status": "completed",
		"items": []any{map[string]any{
			"id": "item_unknown_sidecar", "threadId": threadID, "turnId": "turn_unknown_sidecar",
			"kind": "future_untrusted_kind", "status": "completed", "text": "must not persist",
		}},
	}}
	store.mu.Lock()
	err = store.upsertThreadNoLock(invalid, false)
	store.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "item_kind_or_status_unsupported") {
		t.Fatalf("unknown sidecar lifecycle was not rejected: %v", err)
	}
	after, err := os.ReadFile(store.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("failed closed projection changed primary thread: before=%s after=%s", before, after)
	}
}

func TestDurableEventStoreRecoversAnalytixSidecarThreadHistory(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_legacy_sidecar"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatalf("create thread dir: %v", err)
	}
	now := "2026-06-20T00:00:00.000Z"
	writeDurableTestJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), map[string]any{
		"kind":      "thread_metadata",
		"version":   1,
		"timestamp": now,
		"thread": map[string]any{
			"id":             threadID,
			"title":          "New thread",
			"workspace":      "/workspace/analytix",
			"model":          "deepseek-chat",
			"mode":           "agent",
			"status":         "idle",
			"approvalPolicy": "on-request",
			"sandboxMode":    "workspace-write",
			"relation":       "primary",
			"turns": []any{map[string]any{
				"id":                "turn_legacy",
				"threadId":          threadID,
				"status":            "completed",
				"prompt":            "",
				"steering":          []any{},
				"attachmentIds":     []any{},
				"activeSkillIds":    []any{},
				"injectedMemoryIds": []any{},
				"createdAt":         now,
				"finishedAt":        now,
				"items":             []any{},
			}},
			"createdAt": now,
			"updatedAt": now,
		},
		"summary": map[string]any{"schemaVersion": 1, "preview": "Legacy preview", "messageCount": 2, "turnCount": 1},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_legacy_user",
		"turnId":     "turn_legacy",
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"kind":       "user_message",
		"text":       "Restore my previous Analytix session history",
		"createdAt":  now,
		"finishedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_legacy_assistant",
		"turnId":     "turn_legacy",
		"threadId":   threadID,
		"role":       "assistant",
		"status":     "completed",
		"kind":       "assistant_text",
		"text":       "Recovered answer",
		"createdAt":  now,
		"finishedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_legacy_reasoning_tombstone",
		"turnId":     "turn_legacy",
		"threadId":   threadID,
		"status":     "completed",
		"kind":       "content_redacted",
		"text":       "REDACTED_TOMBSTONE_MUST_NOT_HYDRATE",
		"createdAt":  now,
		"finishedAt": now,
	})

	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	threads, err := store.ListThreads(false, false, false, "")
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected recovered sidecar thread, got %#v", threads)
	}
	state, err := contextepochapp.DefaultState(threadID, 1, time.Date(2026, 6, 20, 0, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("create context epoch state: %v", err)
	}
	if _, err := store.PatchThread(threadID, map[string]any{"contextEpochState": contextepochapp.PublicState(state)}); err != nil {
		t.Fatalf("persist context epoch state after closed sidecar hydration: %v", err)
	}
	primary, err := os.ReadFile(store.threadPath(threadID))
	if err != nil {
		t.Fatalf("read migrated primary thread: %v", err)
	}
	if bytes.Contains(primary, []byte("content_redacted")) || bytes.Contains(primary, []byte("REDACTED_TOMBSTONE_MUST_NOT_HYDRATE")) {
		t.Fatalf("reasoning audit tombstone entered primary thread: %s", primary)
	}
	if !bytes.Contains(primary, []byte("item_turn_legacy_user")) {
		t.Fatalf("valid user sidecar item was not retained: %s", primary)
	}
	if got := stringField(threads[0], "title"); got != "Restore my previous Analytix session history" {
		t.Fatalf("sidecar thread should derive title from first user message, got %q", got)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	turns := listAny(thread["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected hydrated turn, got %#v", thread)
	}
	turn, _ := turns[0].(map[string]any)
	items := listAny(turn["items"])
	if len(items) != 1 || !strings.Contains(stringField(items[0].(map[string]any), "text"), "Restore my previous") {
		t.Fatalf("expected only user-authored sidecar history, got %#v", items)
	}
	if strings.Contains(fmt.Sprint(items), "Recovered answer") {
		t.Fatalf("unbound assistant sidecar text was rehydrated: %#v", items)
	}
}

func TestDurableStoreRejectsPrivateReasoningWrites(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	thread, err := store.CreateThread(map[string]any{"id": "thr_reasoning_guard", "title": "Reasoning guard"}, workspacetest.New(t))
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_reasoning_guard"
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":       turnID,
		"threadId": threadID,
		"status":   "running",
		"items": []any{map[string]any{
			"id":     "legacy_reasoning",
			"kind":   "assistant_reasoning",
			"text":   "PRIVATE_REASONING_SENTINEL",
			"turnId": turnID,
		}},
	}, "provider", nil); err != nil {
		t.Fatalf("append sanitized turn: %v", err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, map[string]any{
		"id":     "reasoning_item",
		"kind":   "assistant_reasoning",
		"text":   "PRIVATE_REASONING_SENTINEL",
		"turnId": turnID,
	}); !errors.Is(err, domainevent.ErrPrivateReasoningPersistence) {
		t.Fatalf("reasoning item should be rejected: %v", err)
	}
	if _, _, err := store.RecordEvent(map[string]any{
		"kind":     "assistant_reasoning_delta",
		"threadId": threadID,
		"turnId":   turnID,
		"text":     "PRIVATE_REASONING_SENTINEL",
	}); !errors.Is(err, domainevent.ErrPrivateReasoningPersistence) {
		t.Fatalf("reasoning event should be rejected: %v", err)
	}
	raw := `{"kind":"assistant_reasoning_delta","threadId":"` + threadID + `","turnId":"` + turnID + `","seq":99,"text":"PRIVATE_REASONING_SENTINEL"}`
	if err := store.AppendRawEventLine(threadID, raw); !errors.Is(err, domainevent.ErrPrivateReasoningPersistence) {
		t.Fatalf("raw reasoning event should be rejected: %v", err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	encoded, _ := json.Marshal(reloaded)
	if strings.Contains(string(encoded), "PRIVATE_REASONING_SENTINEL") || strings.Contains(string(encoded), "assistant_reasoning") {
		t.Fatalf("private reasoning reached durable thread: %s", encoded)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	replayJSON, _ := json.Marshal(replay.Events)
	if strings.Contains(string(replayJSON), "PRIVATE_REASONING_SENTINEL") || strings.Contains(string(replayJSON), "assistant_reasoning") {
		t.Fatalf("private reasoning reached durable events: %s", replayJSON)
	}
}

func TestDurableStoreRejectsTypeConfusedReasoningMetadataWithoutOverwritingThread(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_reasoning_metadata_guard", "title": "Reasoning metadata guard",
	}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	before, err := os.ReadFile(store.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	err = store.AppendTurnToThread(threadID, map[string]any{
		"id": "turn_reasoning_metadata_guard", "threadId": threadID, "status": "running",
		"reasoningEffort": "PRIVATE_REASONING_SENTINEL", "items": []any{},
	}, "provider", nil)
	if !errors.Is(err, threadapp.ErrDurableHistorySanitization) {
		t.Fatalf("type-confused reasoning metadata did not fail closed: %v", err)
	}
	after, readErr := os.ReadFile(store.threadPath(threadID))
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected durable history overwrote thread state: err=%v before=%s after=%s", readErr, before, after)
	}
}

func TestCaseThreadDurableWriteRejectsRestrictedPII(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "case privacy guard", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_case_privacy_guard"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-privacy-guard",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("privacy-binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("privacy-manifest")), ContextEpoch: 1, IssuedAt: time.Unix(10, 0).UTC(),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	const account = "6222020000000000000"
	err = store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"prompt": "核实账号 " + account,
		"items": []any{map[string]any{
			"id": "item_case_privacy_guard", "threadId": threadID, "turnId": turnID,
			"kind": "user_message", "role": "user", "status": "completed", "text": "账号：" + account,
		}},
	}, "provider", map[string]any{"securityState": contextRecord})
	if err == nil {
		t.Fatal("case thread raw PII bypassed the durable write gate")
	}
	reloaded, readErr := store.GetThread(threadID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	encoded, _ := json.Marshal(reloaded)
	if strings.Contains(string(encoded), account) || len(listAny(reloaded["turns"])) != 0 {
		t.Fatalf("rejected case PII mutated durable state: %s", encoded)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"prompt": "核实账号 [ACCOUNT]",
		"items": []any{map[string]any{
			"id": "item_case_privacy_guard", "threadId": threadID, "turnId": turnID,
			"kind": "user_message", "role": "user", "status": "completed", "text": "账号：[ACCOUNT]",
		}},
	}, "provider", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatalf("masked case turn was rejected: %v", err)
	}
	raw := `{"kind":"turn_steered","threadId":"` + threadID + `","turnId":"` + turnID + `","seq":1,"text":"账号：` + account + `"}`
	if err := store.AppendRawEventLine(threadID, raw); err == nil {
		t.Fatal("raw case event bypassed the exact ordinary projection gate")
	}
	replay, replayErr := store.LoadEventsSince(threadID, 0)
	if replayErr != nil {
		t.Fatal(replayErr)
	}
	replayBody, _ := json.Marshal(replay.Events)
	if strings.Contains(string(replayBody), account) {
		t.Fatalf("rejected raw case event entered replay: %s", replayBody)
	}
}

func TestPublicToolResultNeverPersistsOrReplaysRawPayload(t *testing.T) {
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Closed tool result", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_closed_tool_result"
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "items": []any{},
	}, "provider", nil); err != nil {
		t.Fatal(err)
	}
	const sentinel = "RAW_TOOL_MEDIA_AND_ACCOUNT_SENTINEL_6222020202020202020"
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call-read"), Name: "read"}
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: threadID, TurnID: turnID, Call: call,
		Projection: toolcatalogapp.BuildPublicToolResultProjectionV1(call.Name, map[string]any{
			"content": sentinel,
			"source":  map[string]any{"data": sentinel},
			"bytes":   []byte(sentinel),
		}, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, records.ResultItem); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RecordEvent(records.Event); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	values := []any{reloaded, replay.Events}
	for _, path := range []string{store.threadPath(threadID), store.messagesPath(threadID), store.eventsPath(threadID)} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		values = append(values, string(data))
	}
	for _, value := range values {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if bytes.Contains(encoded, []byte(sentinel)) {
			t.Fatalf("raw tool payload reached durable or replay surface: %s", encoded)
		}
	}
	output, _ := records.ResultItem["output"].(map[string]any)
	if stringField(output, "projectionKind") != "host_status" || !boolField(output, "privatePayloadWithheld") {
		t.Fatalf("closed public projection was not persisted: %#v", output)
	}
}

func TestRecoveredParentToolSettlementExactIsConcurrentAndConflictClosed(t *testing.T) {
	store, input := recoveredParentSettlementFixture(t)
	const workers = 32
	results := make(chan subagentstartupport.RecoveredParentToolSettlementResult, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := store.EnsureRecoveredParentToolSettlementExact(input)
			results <- result
			errs <- err
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent recovered settlement: %v", err)
		}
	}
	inserted, existing := 0, 0
	for result := range results {
		switch result {
		case subagentstartupport.RecoveredParentToolSettlementInserted:
			inserted++
		case subagentstartupport.RecoveredParentToolSettlementExistingExact:
			existing++
		default:
			t.Fatalf("unexpected concurrent settlement result: %q", result)
		}
	}
	if inserted != 1 || existing != workers-1 {
		t.Fatalf("concurrent settlement inserted=%d existing=%d", inserted, existing)
	}
	settled, err := store.GetThread(input.Record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := subagentapp.FindTurn(settled, input.Record.ParentTurnID)
	toolResults := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == input.CallID {
			toolResults++
		}
		if stringField(item, "id") == input.ToolCallItemID &&
			(stringField(item, "status") != input.Status || stringField(item, "finishedAt") != input.Timestamp) {
			t.Fatalf("tool call status did not settle exactly: %#v", item)
		}
	}
	if toolResults != 1 {
		t.Fatalf("tool result count=%d thread=%#v", toolResults, settled)
	}
	before, _ := json.Marshal(settled)
	conflict := input
	conflict.ResultItem = cloneMap(input.ResultItem)
	conflict.ResultItem["output"] = map[string]any{"status": "forged"}
	result, err := store.EnsureRecoveredParentToolSettlementExact(conflict)
	if err != nil || result != subagentstartupport.RecoveredParentToolSettlementConflict {
		t.Fatalf("same-id conflict result=%q err=%v", result, err)
	}
	after, err := store.GetThread(input.Record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := json.Marshal(after)
	if !bytes.Equal(before, afterBody) {
		t.Fatal("same-id conflict overwrote recovered settlement")
	}
}

func TestRecoveredParentToolSettlementExactRejectsArchivedParentWithoutMutation(t *testing.T) {
	store, input := recoveredParentSettlementFixture(t)
	if _, err := store.PatchThread(input.Record.ParentThreadID, map[string]any{"status": "archived"}); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetThread(input.Record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeBody, _ := json.Marshal(before)
	eventsBefore, err := store.LoadEventsSince(input.Record.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventsBeforeBody, _ := json.Marshal(eventsBefore.Events)

	result, err := store.EnsureRecoveredParentToolSettlementExact(input)
	if err != nil || result != subagentstartupport.RecoveredParentToolSettlementConflict {
		t.Fatalf("archived recovered settlement result=%q err=%v", result, err)
	}
	after, err := store.GetThread(input.Record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := json.Marshal(after)
	eventsAfter, err := store.LoadEventsSince(input.Record.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterBody, _ := json.Marshal(eventsAfter.Events)
	if !bytes.Equal(beforeBody, afterBody) || !bytes.Equal(eventsBeforeBody, eventsAfterBody) {
		t.Fatalf("archived recovered settlement changed parent or events: parent=%t events=%t",
			bytes.Equal(beforeBody, afterBody), bytes.Equal(eventsBeforeBody, eventsAfterBody))
	}
}

func TestRecoveredParentToolSettlementExactConvergesAfterAmbiguousSidecarWrite(t *testing.T) {
	store, input := recoveredParentSettlementFixture(t)
	metadataPath := store.metadataPath(input.Record.ParentThreadID)
	backupPath := metadataPath + ".before-recovered-settlement"
	if err := os.Rename(metadataPath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(metadataPath, 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := store.EnsureRecoveredParentToolSettlementExact(input)
	if err != nil || result != subagentstartupport.RecoveredParentToolSettlementInserted {
		t.Fatalf("ambiguous write did not use exact primary readback: result=%q err=%v", result, err)
	}
	if err := os.Remove(metadataPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, metadataPath); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewTempDurableEventSessionStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	result, err = restarted.EnsureRecoveredParentToolSettlementExact(input)
	if err != nil || result != subagentstartupport.RecoveredParentToolSettlementExistingExact {
		t.Fatalf("restart exact convergence result=%q err=%v", result, err)
	}
}

func recoveredParentSettlementFixture(t *testing.T) (*DurableEventSessionStore, subagentstartupport.RecoveredParentToolSettlementInput) {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"id": "thr_recovered_exact", "title": "Recovered exact", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_recovered_exact"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	record := domainjob.Record{
		ID: "job-recovered-exact", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID, SecurityBinding: parent.Binding,
		Kind: "subagent", Status: string(domainjob.StatusInterrupted), Background: true,
		RecoveryUpdatedAt: timestamp, UpdatedAt: timestamp, FinishedAt: timestamp,
	}
	records, err := subagentapp.RecoveredJobToolResult(subagentapp.RecoveredJobToolResultInput{
		ThreadID: threadID, TurnID: turnID, Record: record,
		Message: "runtime restarted", CallID: parent.CallID, ToolName: "task",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, subagentstartupport.RecoveredParentToolSettlementInput{
		Record: record, ToolCallItemID: parent.ItemID, CallID: parent.CallID, ToolName: "task",
		Status: records.Status, Timestamp: timestamp, ResultItem: records.ResultItem,
	}
}

func TestEnsurePrivateReportToolResultExactSettlesOnceAndNeverProjectsAdmission(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Exact private report result", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_exact_private_report_result"
	now := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("exact-report-manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running",
		"createdAt": now.Format(time.RFC3339Nano), "startedAt": now.Format(time.RFC3339Nano),
		"items": []any{}, "securityContext": securityRecord,
	}, "host", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"report":"controlled"}`)
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("call_exact_private_report_result"), Name: pendingworkapp.ReportStageToolName, Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin",
		ToolName: call.Name, ToolCallID: call.ID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("exact-report-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("exact-report-scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	callItem, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{
		ThreadID: threadID, TurnID: turnID, ItemID: domaintoolcall.ToolCallItemIDV1(turnID, call.ID),
		CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "report", Context: securityContext, Grant: grant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, callItem); err != nil {
		t.Fatal(err)
	}
	decisionID := domainsecurity.SHA256Hex([]byte("exact-report-decision"))
	decisionDigest := domainsecurity.SHA256Hex([]byte("exact-report-decision-record"))
	records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID: threadID, TurnID: turnID, CreatedAt: now.Add(time.Second).Format(time.RFC3339Nano),
		FinishedAt: now.Add(2 * time.Second).Format(time.RFC3339Nano), Call: call,
		Projection:       domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
		ContextDigest:    securityContext.ContextDigest,
		ContextEpoch:     securityContext.ContextEpoch,
		ExecutionGrantID: grant.GrantID,
	})
	if err != nil {
		t.Fatal(err)
	}
	records.ResultItem["hostReportAdmission"] = domaintoolresult.HostReportAdmissionRecordV1(
		domaintoolresult.NewHostReportAdmissionV1(decisionID, decisionDigest),
	)
	if err := store.EnsurePrivateToolResultItemExact(threadID, turnID, records.ResultItem); err != nil {
		t.Fatalf("persist exact private report result: %v", err)
	}
	if err := store.EnsurePrivateToolResultItemExact(threadID, turnID, records.ResultItem); err != nil {
		t.Fatalf("exact retry after grant settlement: %v", err)
	}

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(threadID, reloaded, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if err := domainsecurity.VerifyExecutionGrantMembership(registry, threadID, turnID, grant, domainsecurity.GrantRegistrySettled); err != nil {
		t.Fatalf("exact result did not settle its grant: %v", err)
	}
	turn, ok := turnappTurnByIDForRestoreTest(reloaded, turnID)
	if !ok {
		t.Fatal("exact result turn is unavailable")
	}
	var persisted map[string]any
	resultCount := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "id") == records.ResultItemID {
			persisted = item
			resultCount++
		}
	}
	if resultCount != 1 || persisted == nil {
		t.Fatalf("exact retry duplicated or lost result: %#v", turn["items"])
	}
	admission, err := domaintoolresult.ParseHostReportAdmissionV1(persisted["hostReportAdmission"])
	if err != nil || admission.DecisionID != decisionID || admission.DecisionRecordDigest != decisionDigest {
		t.Fatalf("private admission binding changed: %#v err=%v", admission, err)
	}

	publicThread, err := threadapp.ProjectPublicThread(reloaded)
	if err != nil {
		t.Fatalf("project public thread: %v", err)
	}
	publicEvent, visible, err := threadapp.ProjectPublicThreadEvent(threadID, reloaded, map[string]any{
		"kind": "tool_call_finished", "threadId": threadID, "turnId": turnID,
		"itemId": records.ResultItemID, "item": persisted,
	})
	if err != nil || visible || publicEvent != nil {
		t.Fatalf("project public tool event: visible=%v err=%v", visible, err)
	}
	publicBytes, _ := json.Marshal([]any{publicThread, publicEvent})
	for _, forbidden := range []string{"hostReportAdmission", decisionID, decisionDigest} {
		if bytes.Contains(publicBytes, []byte(forbidden)) {
			t.Fatalf("private report admission reached public history or SSE: %s", publicBytes)
		}
	}

	conflict := cloneMap(records.ResultItem)
	conflict["hostReportAdmission"] = domaintoolresult.HostReportAdmissionRecordV1(
		domaintoolresult.NewHostReportAdmissionV1(domainsecurity.SHA256Hex([]byte("other-decision")), decisionDigest),
	)
	if err := store.EnsurePrivateToolResultItemExact(threadID, turnID, conflict); err == nil {
		t.Fatal("same result id with a different report admission was accepted")
	}
	reloaded, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ = turnappTurnByIDForRestoreTest(reloaded, turnID)
	resultCount = 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "id") == records.ResultItemID {
			resultCount++
			admission, parseErr := domaintoolresult.ParseHostReportAdmissionV1(item["hostReportAdmission"])
			if parseErr != nil || admission.DecisionID != decisionID {
				t.Fatalf("collision changed the original report admission: %#v err=%v", admission, parseErr)
			}
		}
	}
	if resultCount != 1 {
		t.Fatalf("collision changed exact result cardinality: %#v", turn["items"])
	}
}

func TestDurableStoreRejectsLateSecurityBoundToolResultAfterAbort(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Late result", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_late_result"
	context := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	record := turnsecurityapp.PublicRecord(context)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": record, "items": []any{},
	}, "provider", map[string]any{"securityState": record}); err != nil {
		t.Fatal(err)
	}
	if err := turnapp.PersistFailure(turnapp.PersistFailureInput{
		Store: store, SecurityContext: context, TerminalReason: "cancel", ThreadID: threadID, TurnID: turnID,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Failure: domainfailure.New(domainfailure.CodeTurnCancelled, nil),
		Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true},
	}); err != nil {
		t.Fatalf("abort turn through terminal outbox: %v", err)
	}
	late := map[string]any{
		"id": "item_late", "threadId": threadID, "turnId": turnID, "kind": "tool_result", "status": "completed",
		"contextDigest": context.ContextDigest, "contextEpoch": float64(context.ContextEpoch), "executionGrantId": "grant_late",
	}
	if err := store.AppendItemToTurn(threadID, turnID, late); err == nil {
		t.Fatal("aborted turn accepted a late security-bound tool result")
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	items := listAny(turns[0].(map[string]any)["items"])
	if len(items) != 1 || stringField(items[0].(map[string]any), "kind") != "error" {
		t.Fatalf("late result changed the sealed terminal item set: %#v", items)
	}
	if strings.Contains(fmt.Sprint(items), "item_late") {
		t.Fatalf("late result reached durable history: %#v", items)
	}
}

func TestDurableStorePublishesOnlyExactGeneralTerminalAssistantEvent(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "General terminal publication", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_general_terminal_publication"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1,
		IssuedAt: time.Unix(10, 0).UTC(),
	})
	contextRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord, "items": []any{},
	}, "provider", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "provider", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	})
	if err != nil || !result.Changed {
		t.Fatalf("commit general terminal: changed=%v err=%v", result.Changed, err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	items := listAny(turns[0].(map[string]any)["items"])
	if len(items) != 1 {
		t.Fatalf("canonical assistant item mismatch: %#v", items)
	}
	forgedItem := cloneMap(items[0].(map[string]any))
	forgedItem["text"] = "forged post-terminal text"
	if _, _, err := store.RecordEvent(map[string]any{
		"kind": "item_completed", "threadId": threadID, "turnId": turnID,
		"itemId": forgedItem["id"], "item": forgedItem, "timestamp": forgedItem["finishedAt"],
	}); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("forged assistant event should fail closed: %v", err)
	}
	if err := store.AppendItemToTurn(threadID, turnID, map[string]any{
		"id": "item_late_assistant", "threadId": threadID, "turnId": turnID,
		"role": "assistant", "status": "completed", "kind": "assistant_text", "text": "late assistant",
	}); err == nil {
		t.Fatalf("late assistant append should fail closed: %v", err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	assistantFinals := 0
	for _, event := range replay.Events {
		item, _ := event["item"].(map[string]any)
		if stringField(event, "kind") == "item_completed" && stringField(item, "kind") == "assistant_text" {
			assistantFinals++
			if stringField(item, "text") != turnapp.GeneralProviderFinalQuarantinedText {
				t.Fatalf("replay exposed non-canonical assistant text: %#v", event)
			}
		}
	}
	if assistantFinals != 1 {
		t.Fatalf("expected one canonical assistant final, got %d: %#v", assistantFinals, replay.Events)
	}
}

func TestAcceptedFinalCASObservationNeverUsesSidecars(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "primary CAS only", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_primary_cas_only"
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-primary-cas",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Unix(10, 0).UTC(),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	_ = json.Unmarshal(contextBody, &contextRecord)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSONL(t, store.metadataPath(threadID), map[string]any{
		"kind": "thread_metadata", "threadId": threadID, "timestamp": "2026-07-11T00:00:00Z",
		"thread": map[string]any{
			"id": threadID, "securityState": contextRecord,
			"turns": []any{map[string]any{
				"id": turnID, "threadId": threadID, "status": "completed", "securityContext": contextRecord,
				"acceptedFinal": map[string]any{"recordDigest": domainsecurity.SHA256Hex([]byte("sidecar-only"))},
			}},
		},
	})
	casReader, err := finalauthorityadapter.NewAcceptedFinalCASReader(store.root)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := casReader.ReadAcceptedFinalCASObservation(context.Background(), threadID, turnID)
	if err != nil || observation.HasWinner || domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil {
		t.Fatalf("sidecar value affected primary CAS observation: observation=%#v err=%v", observation, err)
	}
	if err := os.Remove(store.threadPath(threadID)); err != nil {
		t.Fatal(err)
	}
	if _, err := casReader.ReadAcceptedFinalCASObservation(context.Background(), threadID, turnID); err == nil {
		t.Fatal("metadata/messages sidecars reconstructed a missing primary CAS record")
	}
}

func TestDurableStoreMigratesPrivateReasoningFilesIdempotently(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_reasoning_migration"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": "turn_1",
			"items": []any{
				map[string]any{"id": "reasoning", "turnId": "turn_1", "kind": "assistant_reasoning", "text": "PRIVATE_REASONING_SENTINEL"},
				map[string]any{"id": "tool", "turnId": "turn_1", "kind": "tool_call", "reasoningContent": "PRIVATE_REASONING_SENTINEL"},
				map[string]any{"id": "answer", "turnId": "turn_1", "kind": "assistant_text", "text": "<think>PRIVATE_REASONING_SENTINEL</think>public answer"},
			},
		}},
	}
	threadJSON, _ := json.Marshal(thread)
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), threadJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	messages := strings.Join([]string{
		`{"id":"reasoning","turnId":"turn_1","kind":"assistant_reasoning","text":"PRIVATE_REASONING_SENTINEL"}`,
		`{"id":"tool","turnId":"turn_1","kind":"tool_call","reasoningContent":"PRIVATE_REASONING_SENTINEL"}`,
		`{"id":"answer","turnId":"turn_1","kind":"assistant_text","text":"<think>PRIVATE_REASONING_SENTINEL</think>public answer"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(threadDir, "messages.jsonl"), []byte(messages), 0o600); err != nil {
		t.Fatal(err)
	}
	events := strings.Join([]string{
		`{"kind":"assistant_reasoning_delta","threadId":"thr_reasoning_migration","turnId":"turn_1","seq":1,"text":"PRIVATE_REASONING_SENTINEL"}`,
		`{"kind":"item_completed","threadId":"thr_reasoning_migration","turnId":"turn_1","seq":2,"item":{"kind":"assistant_reasoning","text":"PRIVATE_REASONING_SENTINEL"}}`,
		`{"kind":"assistant_text_delta","threadId":"thr_reasoning_migration","turnId":"turn_1","seq":3,"text":"public answer"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(threadDir, "events.jsonl"), []byte(events), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := `{"kind":"thread_metadata","timestamp":"2026-07-10T00:00:00Z","thread":` + string(threadJSON) + `,"summary":{"preview":"<think class=\"private\">PRIVATE_REASONING_SENTINEL</think>public answer"}}` + "\n"
	if err := os.WriteFile(filepath.Join(threadDir, "metadata.jsonl"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	threadSummaries := `{"schemaVersion":1,"threadId":"thr_reasoning_migration","summary":{"id":"thr_reasoning_migration","preview":"{\"reasoning_content\":\"PRIVATE_REASONING_SENTINEL\"}"},"updatedAt":"2026-07-10T00:00:00Z","writtenAt":"2026-07-10T00:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "thread_summaries.jsonl"), []byte(threadSummaries), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := newSemanticStartupDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("migrate durable store: %v", err)
	}
	if err := store.ApplySemanticStartupMigrationsAfterAuthorityRepair(); err != nil {
		t.Fatalf("apply semantic migrations: %v", err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("read migrated thread: %v", err)
	}
	encoded, _ := json.Marshal(reloaded)
	if strings.Contains(string(encoded), "PRIVATE_REASONING_SENTINEL") || strings.Contains(string(encoded), "assistant_reasoning") || !strings.Contains(string(encoded), "public answer") {
		t.Fatalf("migrated thread content mismatch: %s", encoded)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("read migrated events: %v", err)
	}
	lastSeq := 0
	lastSeqOK := false
	if len(replay.Events) == 3 {
		lastSeq, lastSeqOK = numericSeq(replay.Events[2]["seq"])
	}
	if len(replay.Events) != 3 || !lastSeqOK || lastSeq != 3 {
		t.Fatalf("migration must preserve event cursor cardinality: %#v", replay.Events)
	}
	files := []string{
		filepath.Join(threadDir, "thread.json"),
		filepath.Join(threadDir, "messages.jsonl"),
		filepath.Join(threadDir, "events.jsonl"),
		filepath.Join(threadDir, "metadata.jsonl"),
		filepath.Join(root, "thread_summaries.jsonl"),
	}
	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("PRIVATE_REASONING_SENTINEL")) || bytes.Contains(data, []byte("assistant_reasoning")) || bytes.Contains(data, []byte("reasoningContent")) {
			t.Fatalf("private reasoning remains in %s: %s", filePath, data)
		}
	}
	before := map[string][]byte{}
	for _, filePath := range files {
		before[filePath], _ = os.ReadFile(filePath)
	}
	repeatStore, err := newSemanticStartupDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	if err := repeatStore.ApplySemanticStartupMigrationsAfterAuthorityRepair(); err != nil {
		t.Fatalf("repeat semantic migration: %v", err)
	}
	for filePath, want := range before {
		got, _ := os.ReadFile(filePath)
		if !bytes.Equal(got, want) {
			t.Fatalf("reasoning migration is not idempotent for %s", filePath)
		}
	}
}

func TestDurableEventStoreHydratesSidecarMessagesWhenThreadJSONLostItems(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_sidecar_thread_json_empty"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatalf("create thread dir: %v", err)
	}
	now := "2026-06-22T00:00:00.000Z"
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "New thread",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns": []any{map[string]any{
			"id": "turn_recovered", "threadId": threadID, "status": "completed", "items": []any{}, "createdAt": now, "finishedAt": now,
		}},
		"createdAt": now,
		"updatedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_recovered_user",
		"turnId":     "turn_recovered",
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"kind":       "user_message",
		"text":       "Recover from a partially written thread.json",
		"createdAt":  now,
		"finishedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_recovered_assistant",
		"turnId":     "turn_recovered",
		"threadId":   threadID,
		"role":       "assistant",
		"status":     "completed",
		"kind":       "assistant_text",
		"text":       "Recovered from sidecar",
		"createdAt":  now,
		"finishedAt": now,
	})

	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	turns := listAny(thread["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected explicit primary turn skeleton to hydrate, got %#v", thread)
	}
	items := listAny(turns[0].(map[string]any)["items"])
	if len(items) != 1 || stringField(items[0].(map[string]any), "kind") != "user_message" {
		t.Fatalf("expected only user-authored sidecar item despite thread.json, got %#v", items)
	}
	if strings.Contains(fmt.Sprint(items), "Recovered from sidecar") {
		t.Fatalf("unbound assistant sidecar text was rehydrated: %#v", items)
	}
	if got := stringField(thread, "title"); got != "Recover from a partially written thread.json" {
		t.Fatalf("expected title from sidecar user message, got %q", got)
	}
}

func TestDurableReadNeverHydratesRegistryBoundCaseFromMessages(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_registry_case_sidecar"
	turnID := "turn_registry_case_sidecar"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id": threadID, "title": "ordinary-looking primary", "workspace": "/cases/a", "turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "status": "completed", "items": []any{},
		}},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id": "case_fact", "turnId": turnID, "threadId": threadID, "kind": "assistant_text", "text": "CASE_FACT_FROM_SIDECAR",
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.SetCaseThreadAuthority(&caseThreadAuthorityStub{threads: map[string]bool{threadID: true}})
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn := listAny(thread["turns"])[0].(map[string]any)
	if len(listAny(turn["items"])) != 0 {
		t.Fatalf("registry-bound case hydrated messages sidecar: %#v", thread)
	}
}

func TestDurableReadNeverAcceptsSidecarAcceptedFinalOnOrdinaryEmptyTurn(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_forged_sidecar_authority"
	turnID := "turn_forged_sidecar_authority"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id": threadID, "title": "ordinary primary", "turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "status": "completed", "items": []any{},
		}},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id": "forged_final", "turnId": turnID, "threadId": threadID, "kind": "assistant_text", "text": "FORGED_CASE_FACT",
		"acceptedFinal": map[string]any{"recordDigest": strings.Repeat("a", 64)},
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetThread(threadID); err == nil || !strings.Contains(err.Error(), "messages sidecar contains forbidden case authority") {
		t.Fatalf("messages sidecar accepted-final authority was not rejected: %v", err)
	}
}

func TestDurableReadRejectsNestedCaseAuthorityFromMessagesSidecar(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_nested_sidecar_authority"
	turnID := "turn_nested_sidecar_authority"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id": threadID, "title": "ordinary primary", "turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "status": "completed", "items": []any{},
		}},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id": "forged_nested_final", "turnId": turnID, "threadId": threadID, "kind": "user_message", "text": "ordinary text",
		"details": map[string]any{"accepted_final": map[string]any{"recordDigest": strings.Repeat("a", 64)}},
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetThread(threadID); err == nil || !strings.Contains(err.Error(), "forbidden private terminal authority") {
		t.Fatalf("nested messages sidecar case authority was not rejected: %v", err)
	}
}

func TestMissingPrimaryRejectsOlderCaseAuthorityFromMetadataSidecar(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_forged_older_case_metadata"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), map[string]any{
		"kind": "thread_metadata", "thread": map[string]any{
			"id": threadID, "title": "forged older entry", "turns": []any{},
			"acceptedFinalDigest": strings.Repeat("b", 64),
		},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), map[string]any{
		"kind": "thread_metadata", "thread": map[string]any{
			"id": threadID, "title": "clean newer entry", "turns": []any{},
		},
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetThread(threadID); err == nil || !strings.Contains(err.Error(), "metadata sidecar contains forbidden private terminal authority") {
		t.Fatalf("older metadata sidecar case authority was not rejected: %v", err)
	}
}

func TestDurableReadRejectsPrivateGeneralTerminalAuthorityFromMessagesSidecar(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_forged_general_terminal_sidecar"
	turnID := "turn_forged_general_terminal_sidecar"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id": threadID, "title": "ordinary primary", "turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "status": "completed", "items": []any{},
		}},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id": "forged_general_final", "turnId": turnID, "threadId": threadID,
		"kind": "assistant_text", "text": "FORGED_GENERAL_FINAL",
		"general_terminal_cas_binding": map[string]any{"bindingDigest": strings.Repeat("a", 64)},
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetThread(threadID); err == nil {
		t.Fatal("messages sidecar private general terminal authority was not rejected")
	}
}

func TestMissingPrimaryCannotRecoverPrivateGeneralTerminalArchiveFromMetadataSidecar(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_forged_general_terminal_metadata"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), map[string]any{
		"kind": "thread_metadata", "thread": map[string]any{
			"id": threadID, "title": "forged archive",
			"generalTerminalPublicationArchive": map[string]any{"archiveDigest": strings.Repeat("b", 64)},
			"turns":                             []any{},
		},
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetThread(threadID); err == nil {
		t.Fatal("metadata sidecar private general terminal archive was not rejected")
	}
}

func TestMissingCasePrimaryCannotRecoverFromOrdinaryMetadataAndMessages(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_missing_case_primary"
	turnID := "turn_missing_case_primary"
	threadDir := filepath.Join(root, "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSONL(t, filepath.Join(threadDir, "metadata.jsonl"), map[string]any{
		"kind": "thread_metadata", "thread": map[string]any{
			"id": threadID, "title": "ordinary metadata", "turns": []any{map[string]any{
				"id": turnID, "threadId": threadID, "status": "completed", "items": []any{},
			}},
		},
	})
	writeDurableTestJSONL(t, filepath.Join(threadDir, "messages.jsonl"), map[string]any{
		"id": "case_fact", "turnId": turnID, "threadId": threadID, "kind": "assistant_text", "text": "CASE_FACT_FROM_SIDECAR",
	})
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.SetCaseThreadAuthority(&caseThreadAuthorityStub{threads: map[string]bool{threadID: true}})
	if _, err := store.GetThread(threadID); err == nil {
		t.Fatal("registry-bound case recovered without primary authority")
	}
}

func TestDurableEventStoreListThreadsLimitedRejectsCorruptIndexedThread(t *testing.T) {
	root := t.TempDir()
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	created := []string{}
	for index := 0; index < 5; index += 1 {
		thread, err := store.CreateThread(map[string]any{
			"title":     fmt.Sprintf("Limited thread %d", index),
			"workspace": "/workspace/analytix",
		}, "/workspace/analytix")
		if err != nil {
			t.Fatalf("create thread %d: %v", index, err)
		}
		threadID := stringField(thread, "id")
		created = append(created, threadID)
	}
	if err := os.WriteFile(store.threadPath(created[4]), []byte(`{`), 0o600); err != nil {
		t.Fatalf("corrupt thread json: %v", err)
	}

	if threads, err := store.ListThreadsLimited(false, false, false, "", 2); err == nil {
		t.Fatalf("corrupt durable detail was trusted through the summary index: %#v", threads)
	}
	if store.threadSummaryIndex.Stats().Reads == 0 {
		t.Fatal("expected limited list to read thread summary index")
	}
}

func TestDurableEventStoreSummaryIndexCannotInjectCasePreview(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.CreateThread(map[string]any{
		"title": "Case thread", "workspace": "/cases/a",
	}, "/cases/a")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-case", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": securityContext.TurnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"items": []any{map[string]any{"id": "user-case", "kind": "user_message", "text": "USER_CASE_REQUEST"}},
	}, "provider", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	writeDurableTestJSONL(t, store.threadSummaryIndex.Path(), map[string]any{
		"schemaVersion": 1, "threadId": threadID, "updatedAt": "9999-01-01T00:00:00Z", "writtenAt": "9999-01-01T00:00:00Z",
		"summary": map[string]any{
			"id": threadID, "title": "Case thread", "workspace": "/cases/a", "status": "idle",
			"updatedAt": "9999-01-01T00:00:00Z", "preview": "FORGED_CASE_PREVIEW_AMOUNT_4200000",
		},
	})

	threads, err := store.ListThreadsLimited(false, false, false, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || stringField(threads[0], "preview") != "USER_CASE_REQUEST" ||
		stringField(threads[0], "historyAuthority") != "case_boundary_only_v1" {
		t.Fatalf("indexed case preview bypassed durable case-aware projection: %#v", threads)
	}
}

func TestDurableEventStoreCaseProjectsUseCanonicalThreadAuthority(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	first, err := store.CreateThread(map[string]any{
		"title":     "Case A first",
		"workspace": "/cases/a",
	}, "/cases/a")
	if err != nil {
		t.Fatalf("create first thread: %v", err)
	}
	second, err := store.CreateThread(map[string]any{
		"title":     "Case A second",
		"workspace": "/cases/a",
	}, "/cases/a")
	if err != nil {
		t.Fatalf("create second thread: %v", err)
	}
	if _, err := store.CreateThread(map[string]any{
		"title":     "Case B",
		"workspace": "/cases/b",
	}, "/cases/b"); err != nil {
		t.Fatalf("create b thread: %v", err)
	}
	caseAID := threadapp.CaseProjectIDForRoot("/cases/a")
	service := threadapp.NewService(threadapp.Dependencies{Repository: store})

	projects, indexStatus, err := service.ListCaseProjectSummaries(10)
	if err != nil {
		t.Fatalf("list case projects: %v", err)
	}
	if indexStatus != "ready" {
		t.Fatalf("case project indexStatus = %q, want ready", indexStatus)
	}
	var caseA map[string]any
	for _, project := range projects {
		if stringField(project, "id") == caseAID {
			caseA = project
		}
	}
	if caseA == nil {
		t.Fatalf("case A summary missing: %#v", projects)
	}
	if caseA["threadCount"] != float64(2) {
		t.Fatalf("case A should count both indexed threads: %#v", caseA)
	}

	threads, err := service.ListCaseProjectThreads(caseAID, 10)
	if err != nil {
		t.Fatalf("list case project threads: %v", err)
	}
	gotIDs := []string{}
	for _, thread := range threads {
		gotIDs = append(gotIDs, stringField(thread, "id"))
	}
	if !containsString(gotIDs, stringField(first, "id")) || !containsString(gotIDs, stringField(second, "id")) {
		t.Fatalf("case project threads should come from summary index, got %#v", threads)
	}
	if err := os.WriteFile(store.threadPath(stringField(second, "id")), []byte(`{`), 0o600); err != nil {
		t.Fatalf("corrupt second thread detail: %v", err)
	}
	if projects, _, err := service.ListCaseProjectSummaries(10); err == nil {
		t.Fatalf("case project summaries trusted corrupt indexed detail: %#v", projects)
	}
	if threads, err := service.ListCaseProjectThreads(caseAID, 10); err == nil {
		t.Fatalf("case project thread list trusted corrupt indexed detail: %#v", threads)
	}
}

func TestDurableEventStoreCaseProjectsNormalizeWindowsSlashVariants(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	backslashThread, err := store.CreateThread(map[string]any{
		"title":     "Windows backslash",
		"workspace": `C:\Users\40216\.analytix\default_workspace`,
	}, `C:\Users\40216\.analytix\default_workspace`)
	if err != nil {
		t.Fatalf("create backslash thread: %v", err)
	}
	slashThread, err := store.CreateThread(map[string]any{
		"title":     "Windows slash",
		"workspace": "C:/Users/40216/.analytix/default_workspace",
	}, "C:/Users/40216/.analytix/default_workspace")
	if err != nil {
		t.Fatalf("create slash thread: %v", err)
	}
	caseID := threadapp.CaseProjectIDForRoot("C:/Users/40216/.analytix/default_workspace")
	service := threadapp.NewService(threadapp.Dependencies{Repository: store})

	projects, indexStatus, err := service.ListCaseProjectSummaries(10)
	if err != nil {
		t.Fatalf("list case projects: %v", err)
	}
	if indexStatus != "ready" {
		t.Fatalf("case project indexStatus = %q, want ready", indexStatus)
	}
	if len(projects) != 1 {
		t.Fatalf("Windows slash variants should collapse to one project, got %#v", projects)
	}
	if stringField(projects[0], "id") != caseID {
		t.Fatalf("project id = %q, want %q", stringField(projects[0], "id"), caseID)
	}
	if projects[0]["threadCount"] != float64(2) {
		t.Fatalf("project should count both Windows slash variants: %#v", projects[0])
	}

	threads, err := service.ListCaseProjectThreads(caseID, 10)
	if err != nil {
		t.Fatalf("list case project threads: %v", err)
	}
	gotIDs := []string{}
	for _, thread := range threads {
		gotIDs = append(gotIDs, stringField(thread, "id"))
	}
	if !containsString(gotIDs, stringField(backslashThread, "id")) || !containsString(gotIDs, stringField(slashThread, "id")) {
		t.Fatalf("case project threads should include both slash variants, got %#v", threads)
	}
}

func TestDurableEventStoreSteeringAdmissionPromotionAndCancellationReplay(t *testing.T) {
	root := t.TempDir()
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(map[string]any{
		"title":     "Steering durable replay",
		"workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	now := "2026-07-01T00:00:00Z"
	turnID := "turn_steering_replay"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	userItem := map[string]any{
		"id":         "item_turn_steering_replay_user",
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"kind":       "user_message",
		"text":       "start",
		"createdAt":  now,
		"finishedAt": now,
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":              turnID,
		"threadId":        threadID,
		"status":          "running",
		"prompt":          "start",
		"steering":        []any{},
		"createdAt":       now,
		"startedAt":       now,
		"items":           []any{userItem},
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatalf("append turn: %v", err)
	}
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, map[string]any{
		"id":                  domainsteering.EntryIDV1(turnID, "client_store_steer"),
		"clientUserMessageId": "client_store_steer",
		"text":                "promote me",
		"admittedAt":          now,
		"delivery":            "steer",
	}); err != nil {
		t.Fatalf("admit steering: %v", err)
	}
	entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil {
		t.Fatalf("promote steering: %v", err)
	}
	if len(entries) != 1 || stringField(entries[0], "status") != "promoted" ||
		stringField(entries[0], "promotedItemId") != domainsteering.EntryIDV1(turnID, "client_store_steer") {
		t.Fatalf("promoted entry mismatch: %#v", entries)
	}
	if len(items) != 1 || stringField(items[0], "kind") != "user_message" ||
		stringField(items[0], "clientUserMessageId") != "client_store_steer" ||
		stringField(items[0], "delivery") != "steer" {
		t.Fatalf("promoted item mismatch: %#v", items)
	}

	restarted, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("restart production store: %v", err)
	}
	replayed, err := restarted.GetThread(threadID)
	if err != nil {
		t.Fatalf("get replayed thread: %v", err)
	}
	replayedTurn, ok := appmodel.TurnByID(replayed, turnID)
	if !ok {
		t.Fatalf("missing replayed turn: %#v", replayed)
	}
	if len(listAny(replayedTurn["steering"])) != 1 ||
		stringField(listAny(replayedTurn["steering"])[0].(map[string]any), "status") != "promoted" {
		t.Fatalf("promoted steering entry did not replay: %#v", replayedTurn["steering"])
	}
	foundPromotedItem := false
	for _, raw := range listAny(replayedTurn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "clientUserMessageId") == "client_store_steer" {
			if stringField(item, "delivery") != "steer" {
				t.Fatalf("replayed promoted steering item lost delivery marker: %#v", item)
			}
			foundPromotedItem = true
		}
	}
	if !foundPromotedItem {
		t.Fatalf("promoted steering item did not hydrate from sidecar: %#v", replayedTurn["items"])
	}

	cancelThread, err := store.CreateThread(map[string]any{
		"title":     "Steering cancel",
		"workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create cancel thread: %v", err)
	}
	cancelThreadID := stringField(cancelThread, "id")
	cancelTurnID := "turn_steering_cancel"
	cancelContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: cancelThreadID, TurnID: cancelTurnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	cancelSecurityRecord := turnsecurityapp.PublicRecord(cancelContext)
	if err := store.AppendTurnToThread(cancelThreadID, map[string]any{
		"id":              cancelTurnID,
		"threadId":        cancelThreadID,
		"status":          "running",
		"prompt":          "start",
		"steering":        []any{},
		"createdAt":       now,
		"startedAt":       now,
		"items":           []any{},
		"securityContext": cancelSecurityRecord,
	}, "", map[string]any{"securityState": cancelSecurityRecord}); err != nil {
		t.Fatalf("append cancel turn: %v", err)
	}
	if _, err := store.AdmitSteeringEntryForContext(cancelThreadID, cancelTurnID, cancelTurnID, cancelContext.ContextDigest, map[string]any{
		"id":                  domainsteering.EntryIDV1(cancelTurnID, "client_cancel_steer"),
		"clientUserMessageId": "client_cancel_steer",
		"text":                "cancel me",
		"admittedAt":          now,
		"delivery":            "steer",
	}); err != nil {
		t.Fatalf("admit cancel steering: %v", err)
	}
	if err := turnapp.PersistFailure(turnapp.PersistFailureInput{
		Store: store, SecurityContext: cancelContext, TerminalReason: "cancel",
		ThreadID: cancelThreadID, TurnID: cancelTurnID, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Failure:   domainfailure.New(domainfailure.CodeTurnCancelled, nil),
		Interrupt: &turnapp.GeneralTerminalInterruptMetadata{Cancelled: true},
	}); err != nil {
		t.Fatalf("finish cancel turn through terminal outbox: %v", err)
	}
	cancelled, err := store.GetThread(cancelThreadID)
	if err != nil {
		t.Fatalf("get cancelled thread: %v", err)
	}
	cancelledTurn, ok := appmodel.TurnByID(cancelled, cancelTurnID)
	if !ok {
		t.Fatalf("missing cancelled turn: %#v", cancelled)
	}
	steering := listAny(cancelledTurn["steering"])
	if len(steering) != 1 || stringField(steering[0].(map[string]any), "status") != "cancelled" ||
		stringField(steering[0].(map[string]any), "cancelReason") != "aborted" {
		t.Fatalf("pending steering was not marked cancelled: %#v", steering)
	}
}

func TestDurableEventStoreContextBoundSteeringAdmissionAndPromotion(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(map[string]any{
		"title": "Context steering", "workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_context_steering"
	now := "2026-07-18T01:02:03Z"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": now, "startedAt": now,
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{
		"id": domainsteering.EntryIDV1(turnID, "client_context_steer"), "clientUserMessageId": "client_context_steer",
		"text": "promote only under the frozen context", "admittedAt": now, "delivery": "steer",
	}
	admitted, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, entry)
	if err != nil {
		t.Fatalf("context-bound admission: %v", err)
	}
	projectionVersion, projectionVersionOK := numericSeq(admitted["projectionVersion"])
	if stringField(admitted, "contextDigest") != securityContext.ContextDigest ||
		stringField(admitted, "status") != "pending" || len(stringField(admitted, "contentDigest")) != 64 ||
		!projectionVersionOK || projectionVersion != 1 {
		t.Fatalf("host steering binding mismatch: %#v", admitted)
	}
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, entry); err != nil {
		t.Fatalf("exact replay should be idempotent: %v", err)
	}
	tampered := cloneMap(entry)
	tampered["text"] = "different account 6222021234567890123"
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, tampered); !errors.Is(err, errDurableSteeringReplayMismatch) {
		t.Fatalf("same id with different content should fail closed: %v", err)
	}
	entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil {
		t.Fatalf("context-bound promotion: %v", err)
	}
	if len(entries) != 1 || len(items) != 1 || stringField(entries[0], "status") != "promoted" ||
		stringField(items[0], "contextDigest") != securityContext.ContextDigest ||
		stringField(items[0], "text") != "promote only under the frozen context" {
		t.Fatalf("promoted context binding mismatch: entries=%#v items=%#v", entries, items)
	}
}

func TestDurableEventStoreSteeringPromotionIsAllOrNothingAcrossContextMismatch(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(map[string]any{
		"title": "Mixed steering", "workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_mixed_steering"
	now := "2026-07-18T01:02:03Z"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": now, "startedAt": now,
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, map[string]any{
		"id": domainsteering.EntryIDV1(turnID, "valid_context_steer"), "clientUserMessageId": "valid_context_steer",
		"text": "VALID_CONTEXT_STEER", "admittedAt": now, "delivery": "steer",
	}); err != nil {
		t.Fatal(err)
	}

	mutateDurableSteeringFixture(t, store, threadID, func(_ map[string]any, latest map[string]any) {
		steering := append([]any(nil), listAny(latest["steering"])...)
		steering = append(steering, map[string]any{
			"id": "legacy_mixed_steer", "clientUserMessageId": "legacy_mixed_steer",
			"text": "MIXED_LEGACY_PII_6222021234567890123", "admittedAt": now, "delivery": "steer",
		})
		latest["steering"] = steering
	})

	if entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest); !errors.Is(err, errDurableSteeringProjectionInvalid) || len(entries) != 0 || len(items) != 0 {
		t.Fatalf("mixed pending set should fail with zero writes: entries=%#v items=%#v err=%v", entries, items, err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	reloadedTurn, ok := appmodel.TurnByID(reloaded, turnID)
	if !ok {
		t.Fatalf("missing reloaded turn: %#v", reloaded)
	}
	for _, raw := range listAny(reloadedTurn["items"]) {
		item, _ := raw.(map[string]any)
		if strings.Contains(stringField(item, "text"), "VALID_CONTEXT_STEER") ||
			strings.Contains(stringField(item, "text"), "MIXED_LEGACY_PII") {
			t.Fatalf("mixed promotion wrote a user item: %#v", item)
		}
	}
	steering := listAny(reloadedTurn["steering"])
	if len(steering) != 2 || stringField(steering[0].(map[string]any), "status") != "pending" ||
		stringField(steering[0].(map[string]any), "promotedItemId") != "" {
		t.Fatalf("valid entry was partially promoted: %#v", steering)
	}
}

func TestDuplicatePendingSteerIDsFailClosed(t *testing.T) {
	store, threadID, turnID, securityContext, now := newDurableSteeringSecurityFixture(t, "duplicate-pending")
	clientID := "duplicate-client-steer"
	entryID := domainsteering.EntryIDV1(turnID, clientID)
	first, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": "first authorized guidance", "admittedAt": now, "delivery": "steer",
	})
	if err != nil {
		t.Fatal(err)
	}
	mutateDurableSteeringFixture(t, store, threadID, func(_ map[string]any, turn map[string]any) {
		turn["steering"] = []any{first, cloneMap(first)}
	})
	entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if !errors.Is(err, errDurableSteeringIDConflict) || len(entries) != 0 || len(items) != 0 {
		t.Fatalf("duplicate pending identities should fail closed: entries=%#v items=%#v err=%v", entries, items, err)
	}
	assertNoPromotedSteeringItems(t, store, threadID, turnID)
}

func TestSteerItemIDCannotCollideWithExistingTurnItem(t *testing.T) {
	store, threadID, turnID, securityContext, now := newDurableSteeringSecurityFixture(t, "item-collision")
	clientID := "existing_user_item"
	mutateDurableSteeringFixture(t, store, threadID, func(_ map[string]any, turn map[string]any) {
		turn["items"] = []any{map[string]any{
			"id": clientID, "turnId": turnID, "threadId": threadID, "kind": "user_message", "role": "user", "status": "completed", "text": "original",
		}}
	})
	entry := map[string]any{
		"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
		"text": "collision attempt", "admittedAt": now, "delivery": "steer",
	}
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, entry); !errors.Is(err, errDurableSteeringIDConflict) {
		t.Fatalf("client identity collision should be rejected: %v", err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := appmodel.TurnByID(reloaded, turnID)
	if len(listAny(turn["steering"])) != 0 || len(listAny(turn["items"])) != 1 {
		t.Fatalf("identity collision mutated durable turn: %#v", turn)
	}
}

func TestUnknownSteerStatusPoisonsWholePromotion(t *testing.T) {
	store, threadID, turnID, securityContext, now := newDurableSteeringSecurityFixture(t, "unknown-status")
	clientID := "unknown-status-client"
	entry, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
		"text": "must not promote around corrupt state", "admittedAt": now, "delivery": "steer",
	}, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	entry["status"] = "unknown"
	mutateDurableSteeringFixture(t, store, threadID, func(_ map[string]any, turn map[string]any) {
		turn["steering"] = []any{entry}
	})
	if entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest); !errors.Is(err, errDurableSteeringProjectionInvalid) || len(entries) != 0 || len(items) != 0 {
		t.Fatalf("unknown steering status should fail closed: entries=%#v items=%#v err=%v", entries, items, err)
	}
	assertNoPromotedSteeringItems(t, store, threadID, turnID)
}

func TestSelfConsistentDiskRewriteIsNotHostAuthority(t *testing.T) {
	store, threadID, turnID, securityContext, now := newDurableSteeringSecurityFixture(t, "self-consistent-rewrite")
	clientID := "signed-steer-client"
	entryID := domainsteering.EntryIDV1(turnID, clientID)
	admitted, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": "authorized guidance", "admittedAt": now, "delivery": "steer",
	})
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": "fabricated account 6222021234567890123",
		"admittedAt": now, "delivery": "steer",
	}, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	tampered := cloneMap(admitted)
	tampered["text"] = rewritten["text"]
	tampered["contentDigest"] = rewritten["contentDigest"]
	mutateDurableSteeringFixture(t, store, threadID, func(_ map[string]any, turn map[string]any) {
		turn["steering"] = []any{tampered}
	})
	if entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest); !errors.Is(err, errDurableSteeringProjectionInvalid) || len(entries) != 0 || len(items) != 0 {
		t.Fatalf("self-consistent disk rewrite should lack host authority: entries=%#v items=%#v err=%v", entries, items, err)
	}
	assertNoPromotedSteeringItems(t, store, threadID, turnID)
}

func TestSteeringPromotionRequiresLatestTurn(t *testing.T) {
	store, threadID, turnID, securityContext, now := newDurableSteeringSecurityFixture(t, "not-latest")
	clientID := "old-turn-client"
	entry, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
		"text": "old turn guidance", "admittedAt": now, "delivery": "steer",
	}, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	mutateDurableSteeringFixture(t, store, threadID, func(thread, turn map[string]any) {
		turn["steering"] = []any{entry}
		turns := listAny(thread["turns"])
		turns = append(turns, map[string]any{
			"id": "turn_newer", "threadId": threadID, "status": "running", "items": []any{}, "steering": []any{},
		})
		thread["turns"] = turns
	})
	if entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest); !errors.Is(err, errDurableTurnNotLatest) || len(entries) != 0 || len(items) != 0 {
		t.Fatalf("old turn promotion should fail closed: entries=%#v items=%#v err=%v", entries, items, err)
	}
	assertNoPromotedSteeringItems(t, store, threadID, turnID)
}

func TestPendingSteeringPromotesAcrossRestartWithSameInstallationAuthority(t *testing.T) {
	durableRoot, authorityPath, threadID, turnID, securityContext, contentDigest := newDurableSteeringRestartFixture(t, "same-authority")
	restarted, err := NewProductionDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restarted.SetSteeringAuthority(steeringauthorityapp.NewService(authority))
	entries, items, err := restarted.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(entries) != 1 || len(items) != 1 {
		t.Fatalf("same installation restart did not promote pending steer: entries=%#v items=%#v err=%v", entries, items, err)
	}
	if stringField(entries[0], "contentDigest") != contentDigest ||
		stringField(entries[0], "contextDigest") != securityContext.ContextDigest ||
		stringField(entries[0], "authorityKeyId") != authority.KeyID() ||
		stringField(entries[0], "promotionAuthorityKeyId") != authority.KeyID() ||
		stringField(items[0], "id") != stringField(entries[0], "promotedItemId") {
		t.Fatalf("restarted promotion authority mismatch: entries=%#v items=%#v", entries, items)
	}
}

func TestPendingSteeringRejectsForeignInstallationAuthorityAfterRestart(t *testing.T) {
	durableRoot, authorityPath, threadID, turnID, securityContext, _ := newDurableSteeringRestartFixture(t, "foreign-authority")
	restarted, err := NewProductionDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	foreignRoot := filepath.Join(t.TempDir(), "foreign-authority")
	if err := os.MkdirAll(foreignRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(foreignRoot, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	restarted.SetSteeringAuthority(steeringauthorityapp.NewService(foreign))
	entries, items, err := restarted.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if !errors.Is(err, errDurableSteeringProjectionInvalid) || len(entries) != 0 || len(items) != 0 {
		t.Fatalf("foreign authority did not fail closed: entries=%#v items=%#v err=%v", entries, items, err)
	}
	assertNoPromotedSteeringItems(t, restarted, threadID, turnID)

	recovered, err := NewProductionDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	original, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	recovered.SetSteeringAuthority(steeringauthorityapp.NewService(original))
	entries, items, err = recovered.PromotePendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(entries) != 1 || len(items) != 1 {
		t.Fatalf("foreign zero-write rejection was not recoverable: entries=%#v items=%#v err=%v", entries, items, err)
	}
}

func newDurableSteeringRestartFixture(
	t *testing.T,
	name string,
) (string, string, string, string, domainsecurity.TurnSecurityContext, string) {
	t.Helper()
	durableRoot := filepath.Join(t.TempDir(), "durable")
	store, err := NewProductionDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	authorityRoot := filepath.Join(t.TempDir(), "steering-authority")
	if err := os.MkdirAll(authorityRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(authorityRoot, "authority.json")
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	store.SetSteeringAuthority(steeringauthorityapp.NewService(authority))
	thread, err := store.CreateThread(map[string]any{"title": "Restart steering " + name, "workspace": "/workspace/analytix"}, "/workspace/analytix")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_restart_" + safeDurableID(name)
	now := "2026-07-18T01:02:03Z"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": now, "startedAt": now,
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	clientID := "client_restart_" + safeDurableID(name)
	admitted, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, map[string]any{
		"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
		"text": "restart-safe guidance", "admittedAt": now, "delivery": "steer",
	})
	if err != nil {
		t.Fatal(err)
	}
	return durableRoot, authorityPath, threadID, turnID, securityContext, stringField(admitted, "contentDigest")
}

func newDurableSteeringSecurityFixture(
	t *testing.T,
	name string,
) (*DurableEventSessionStore, string, string, domainsecurity.TurnSecurityContext, string) {
	t.Helper()
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(map[string]any{
		"title": "Steering fixture " + name, "workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_steering_" + safeDurableID(name)
	now := "2026-07-18T01:02:03Z"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": now, "startedAt": now,
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	return store, threadID, turnID, securityContext, now
}

func mutateDurableSteeringFixture(
	t *testing.T,
	store *DurableEventSessionStore,
	threadID string,
	mutate func(thread map[string]any, turn map[string]any),
) {
	t.Helper()
	store.mu.Lock()
	thread, readErr := store.readThreadNoLock(threadID)
	var writeErr error
	if readErr == nil {
		turns := listAny(thread["turns"])
		turn, _ := turns[len(turns)-1].(map[string]any)
		mutate(thread, turn)
		// These tests model an attacker or legacy process rewriting thread.json
		// outside the host persistence boundary. Calling upsertThreadNoLock here
		// would exercise (and now be rejected by) the normal PII/public-record
		// gate instead of the restart/promotion tamper checks under test.
		var body []byte
		body, writeErr = json.Marshal(thread)
		if writeErr == nil {
			writeErr = os.WriteFile(store.threadPath(threadID), body, 0o600)
		}
	}
	store.mu.Unlock()
	if readErr != nil || writeErr != nil {
		t.Fatalf("mutate steering fixture: read=%v write=%v", readErr, writeErr)
	}
}

func configureDurableSteeringAuthority(t *testing.T, store *DurableEventSessionStore) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "steering-authority")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(root, "authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	store.SetSteeringAuthority(steeringauthorityapp.NewService(authority))
}

func assertNoPromotedSteeringItems(t *testing.T, store *DurableEventSessionStore, threadID, turnID string) {
	t.Helper()
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, ok := appmodel.TurnByID(reloaded, turnID)
	if !ok {
		t.Fatalf("missing steering turn: %#v", reloaded)
	}
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		if stringField(item, "delivery") == "steer" {
			t.Fatalf("rejected promotion wrote a steer item: %#v", item)
		}
	}
}

func TestDurableEventStoreSteeringAdmissionRejectsUnsafeTurns(t *testing.T) {
	root := t.TempDir()
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.CreateThread(map[string]any{
		"title":     "Steering rejects",
		"workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	now := "2026-07-01T00:00:00Z"
	turnID := "turn_steering_rejects"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":              turnID,
		"threadId":        threadID,
		"status":          "running",
		"prompt":          "start",
		"steering":        []any{},
		"createdAt":       now,
		"startedAt":       now,
		"items":           []any{},
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatalf("append running turn: %v", err)
	}
	entry := map[string]any{
		"id":                  domainsteering.EntryIDV1(turnID, "client_reject_steer"),
		"clientUserMessageId": "client_reject_steer",
		"text":                "reject me",
		"admittedAt":          now,
	}
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, "turn_elsewhere", securityContext.ContextDigest, entry); !errors.Is(err, errDurableExpectedTurnMismatch) {
		t.Fatalf("expected mismatch error, got %v", err)
	}
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		CreatedAt: now, FinishedAt: now,
	}); err != nil || !result.Changed {
		t.Fatalf("finish running turn through terminal outbox result=%#v err=%v", result, err)
	}
	if _, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, securityContext.ContextDigest, entry); !errors.Is(err, errDurableTurnInactive) {
		t.Fatalf("expected inactive error, got %v", err)
	}

	guiThread, err := store.CreateThread(map[string]any{
		"title":     "Steering rejects gui",
		"workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create gui thread: %v", err)
	}
	guiThreadID := stringField(guiThread, "id")
	guiTurnID := "turn_steering_gui"
	guiContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: guiThreadID, TurnID: guiTurnID, WorkspaceRealPath: "/workspace/analytix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	guiSecurityRecord := turnsecurityapp.PublicRecord(guiContext)
	if err := store.AppendTurnToThread(guiThreadID, map[string]any{
		"id":              guiTurnID,
		"threadId":        guiThreadID,
		"status":          "running",
		"prompt":          "plan",
		"steering":        []any{},
		"createdAt":       now,
		"startedAt":       now,
		"items":           []any{},
		"securityContext": guiSecurityRecord,
		"guiPlan": map[string]any{
			"operation":     "draft",
			"workspaceRoot": "/workspace/analytix",
			"relativePath":  ".analytixsdd/plan/one.md",
			"planId":        "plan-1",
		},
	}, "", map[string]any{"securityState": guiSecurityRecord}); err != nil {
		t.Fatalf("append gui turn: %v", err)
	}
	guiEntry := cloneMap(entry)
	guiEntry["id"] = domainsteering.EntryIDV1(guiTurnID, "client_reject_steer")
	if _, err := store.AdmitSteeringEntryForContext(guiThreadID, guiTurnID, guiTurnID, guiContext.ContextDigest, guiEntry); !errors.Is(err, errDurableTurnNotSteerable) {
		t.Fatalf("expected not steerable error, got %v", err)
	}
}

func TestDurableEventStoreMigratesLegacyRuntimeGoThreadRoot(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_wrong_runtime_go_root"
	legacyThreadDir := filepath.Join(root, "runtime-go", "threads", threadID)
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatalf("create legacy runtime-go thread dir: %v", err)
	}
	now := "2026-06-21T00:00:00.000Z"
	writeDurableTestJSON(t, filepath.Join(legacyThreadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "Thread from wrong runtime-go root",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"createdAt":      now,
		"updatedAt":      now,
		"turns":          []any{},
	})
	writeDurableTestJSONL(t, filepath.Join(legacyThreadDir, "events.jsonl"), map[string]any{
		"kind":      "thread_created",
		"threadId":  threadID,
		"seq":       1,
		"timestamp": now,
	})

	store, err := newSemanticStartupDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	threads, err := store.ListThreads(false, false, false, "")
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 1 || stringField(threads[0], "id") != threadID {
		t.Fatalf("expected migrated runtime-go thread, got %#v", threads)
	}
	if _, err := os.Stat(filepath.Join(root, "threads", threadID, "thread.json")); err != nil {
		t.Fatalf("thread.json was not migrated into product root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "threads", threadID, "events.jsonl")); err != nil {
		t.Fatalf("events.jsonl was not migrated into product root: %v", err)
	}
}

func TestDurableInventoryReadersDoNotImportLegacyAfterConstruction(t *testing.T) {
	root := t.TempDir()
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	if err := store.EnsureThreadSummaryIndex(); err != nil {
		t.Fatalf("build product summary index before late legacy input: %v", err)
	}

	threadID := "thr_late_legacy_inventory"
	legacyThreadDir := filepath.Join(root, "runtime-go", "threads", threadID)
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatalf("create late legacy thread dir: %v", err)
	}
	now := "2026-07-12T00:00:00.000Z"
	legacyThreadPath := filepath.Join(legacyThreadDir, "thread.json")
	writeDurableTestJSON(t, legacyThreadPath, map[string]any{
		"id":             threadID,
		"title":          "Late legacy thread must remain quarantined",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"createdAt":      now,
		"updatedAt":      now,
		"turns":          []any{},
	})

	ids, err := store.AllThreadIDs()
	if err != nil {
		t.Fatalf("list all thread ids: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("late legacy thread entered product inventory: %#v", ids)
	}
	threads, err := store.ListThreads(false, false, false, "")
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("late legacy thread entered list projection: %#v", threads)
	}
	limited, err := store.ListThreadsLimited(false, false, false, "", 10)
	if err != nil {
		t.Fatalf("list limited threads: %v", err)
	}
	if len(limited) != 0 {
		t.Fatalf("late legacy thread entered limited projection: %#v", limited)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get late legacy thread: %v", err)
	}
	if thread != nil {
		t.Fatalf("late legacy thread was read through product inventory: %#v", thread)
	}
	if _, err := os.Stat(legacyThreadPath); err != nil {
		t.Fatalf("late legacy source was mutated or removed by inventory read: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "threads", threadID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("late legacy thread was imported by inventory read, err=%v", err)
	}
}

func TestDurableEventStoreListSearchDoesNotUseUntrustedEventPayload(t *testing.T) {
	root := t.TempDir()
	store, err := NewProductionDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.CreateThread(map[string]any{
		"title":     "Plain Thread",
		"workspace": "/workspace/analytix",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	if _, _, err := store.RecordEvent(map[string]any{
		"kind":     "item_created",
		"threadId": threadID,
		"item": map[string]any{
			"text": "event-only-search-needle",
		},
	}); err != nil {
		t.Fatalf("record event: %v", err)
	}

	threads, err := store.ListThreads(false, false, false, "event-only-search-needle")
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("untrusted event payload became a thread-search membership oracle: %#v", threads)
	}
}

func TestDurableEventStoreMergesMissingSidecarsFromLegacyRuntimeGoRoot(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_partial_runtime_go_root"
	threadDir := filepath.Join(root, "threads", threadID)
	legacyThreadDir := filepath.Join(root, "runtime-go", "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatalf("create product thread dir: %v", err)
	}
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatalf("create legacy runtime-go thread dir: %v", err)
	}
	now := "2026-06-23T00:00:00.000Z"
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "New thread",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns": []any{map[string]any{
			"id": "turn_partial", "threadId": threadID, "status": "completed", "items": []any{}, "createdAt": now, "finishedAt": now,
		}},
		"createdAt": now,
		"updatedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(legacyThreadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_partial_user",
		"turnId":     "turn_partial",
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"kind":       "user_message",
		"text":       "Merge missing sidecars from the wrong runtime root",
		"createdAt":  now,
		"finishedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(legacyThreadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_partial_assistant",
		"turnId":     "turn_partial",
		"threadId":   threadID,
		"role":       "assistant",
		"status":     "completed",
		"kind":       "assistant_text",
		"text":       "Merged answer",
		"createdAt":  now,
		"finishedAt": now,
	})

	store, err := newSemanticStartupDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got := stringField(thread, "title"); got != "Merge missing sidecars from the wrong runtime root" {
		t.Fatalf("expected sidecar title from merged legacy messages, got %q", got)
	}
	turns := listAny(thread["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected merged sidecar turn, got %#v", thread)
	}
	items := listAny(turns[0].(map[string]any)["items"])
	if len(items) != 1 || stringField(items[0].(map[string]any), "kind") != "user_message" {
		t.Fatalf("expected only merged user-authored sidecar message, got %#v", items)
	}
	if strings.Contains(fmt.Sprint(items), "Merged answer") {
		t.Fatalf("legacy unbound assistant sidecar text was rehydrated: %#v", items)
	}
	if _, err := os.Stat(filepath.Join(threadDir, "messages.jsonl")); err != nil {
		t.Fatalf("messages sidecar was not merged into product root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime-go", "threads")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy runtime-go threads root should be removed after sidecar merge, err=%v", err)
	}
}

func TestDurableEventStoreFillsEmptySidecarsFromLegacyRuntimeGoRoot(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_empty_sidecar_runtime_go_root"
	threadDir := filepath.Join(root, "threads", threadID)
	legacyThreadDir := filepath.Join(root, "runtime-go", "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatalf("create product thread dir: %v", err)
	}
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatalf("create legacy runtime-go thread dir: %v", err)
	}
	now := "2026-06-24T00:00:00.000Z"
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "New thread",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns": []any{map[string]any{
			"id": "turn_empty", "threadId": threadID, "status": "completed", "items": []any{}, "createdAt": now, "finishedAt": now,
		}},
		"createdAt": now,
		"updatedAt": now,
	})
	if err := os.WriteFile(filepath.Join(threadDir, "messages.jsonl"), []byte{}, 0o600); err != nil {
		t.Fatalf("create empty product messages sidecar: %v", err)
	}
	writeDurableTestJSONL(t, filepath.Join(legacyThreadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_empty_user",
		"turnId":     "turn_empty",
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"kind":       "user_message",
		"text":       "Fill empty sidecars from the wrong runtime root",
		"createdAt":  now,
		"finishedAt": now,
	})
	writeDurableTestJSONL(t, filepath.Join(legacyThreadDir, "messages.jsonl"), map[string]any{
		"id":         "item_turn_empty_assistant",
		"turnId":     "turn_empty",
		"threadId":   threadID,
		"role":       "assistant",
		"status":     "completed",
		"kind":       "assistant_text",
		"text":       "Filled answer",
		"createdAt":  now,
		"finishedAt": now,
	})

	store, err := newSemanticStartupDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got := stringField(thread, "title"); got != "Fill empty sidecars from the wrong runtime root" {
		t.Fatalf("expected sidecar title from filled legacy messages, got %q", got)
	}
	turns := listAny(thread["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected filled sidecar turn, got %#v", thread)
	}
	items := listAny(turns[0].(map[string]any)["items"])
	if len(items) != 1 || stringField(items[0].(map[string]any), "kind") != "user_message" {
		t.Fatalf("expected only filled user-authored sidecar message, got %#v", items)
	}
	if strings.Contains(fmt.Sprint(items), "Filled answer") {
		t.Fatalf("legacy unbound assistant sidecar text was rehydrated: %#v", items)
	}
	data, err := os.ReadFile(filepath.Join(threadDir, "messages.jsonl"))
	if err != nil {
		t.Fatalf("read filled product messages sidecar: %v", err)
	}
	if !strings.Contains(string(data), "Fill empty sidecars from the wrong runtime root") {
		t.Fatalf("product messages sidecar was not filled from legacy root: %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(root, "runtime-go", "threads")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy runtime-go threads root should be removed after import, err=%v", err)
	}
}

func TestDurableEventStoreReplacesPlaceholderThreadJSONFromLegacyRuntimeGoRoot(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_placeholder_thread_json"
	threadDir := filepath.Join(root, "threads", threadID)
	legacyThreadDir := filepath.Join(root, "runtime-go", "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatalf("create product thread dir: %v", err)
	}
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatalf("create legacy runtime-go thread dir: %v", err)
	}
	now := "2026-06-25T00:00:00.000Z"
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "New thread",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns":          []any{},
		"createdAt":      now,
		"updatedAt":      now,
	})
	writeDurableTestJSON(t, filepath.Join(legacyThreadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "Recovered legacy thread title",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "completed",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns": []any{map[string]any{
			"id":                "turn_legacy_json",
			"threadId":          threadID,
			"status":            "completed",
			"prompt":            "Recovered from legacy thread json",
			"steering":          []any{},
			"attachmentIds":     []any{},
			"activeSkillIds":    []any{},
			"injectedMemoryIds": []any{},
			"createdAt":         now,
			"finishedAt":        now,
			"items": []any{map[string]any{
				"id":         "item_turn_legacy_json_user",
				"turnId":     "turn_legacy_json",
				"threadId":   threadID,
				"role":       "user",
				"status":     "completed",
				"kind":       "user_message",
				"text":       "Recovered from legacy thread json",
				"createdAt":  now,
				"finishedAt": now,
			}},
		}},
		"createdAt": now,
		"updatedAt": now,
	})

	store, err := newSemanticStartupDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got := stringField(thread, "title"); got != "Recovered legacy thread title" {
		t.Fatalf("expected placeholder thread.json to be recovered, got title %q", got)
	}
	turns := listAny(thread["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected legacy thread.json turns, got %#v", thread)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime-go", "threads")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy runtime-go threads root should be removed after placeholder recovery, err=%v", err)
	}
}

func TestDurableEventStoreRejectsDivergentNonPlaceholderLegacyThreadWithoutDeletingSource(t *testing.T) {
	root := t.TempDir()
	threadID := "thr_preserve_product_thread_json"
	threadDir := filepath.Join(root, "threads", threadID)
	legacyThreadDir := filepath.Join(root, "runtime-go", "threads", threadID)
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatalf("create product thread dir: %v", err)
	}
	if err := os.MkdirAll(legacyThreadDir, 0o700); err != nil {
		t.Fatalf("create legacy runtime-go thread dir: %v", err)
	}
	now := "2026-06-26T00:00:00.000Z"
	writeDurableTestJSON(t, filepath.Join(threadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "Real product thread",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns":          []any{},
		"createdAt":      now,
		"updatedAt":      now,
	})
	writeDurableTestJSON(t, filepath.Join(legacyThreadDir, "thread.json"), map[string]any{
		"id":             threadID,
		"title":          "Legacy should not overwrite",
		"workspace":      "/workspace/analytix",
		"model":          "deepseek-chat",
		"mode":           "agent",
		"status":         "idle",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"relation":       "primary",
		"turns": []any{map[string]any{
			"id":        "turn_legacy_should_not_copy",
			"threadId":  threadID,
			"status":    "completed",
			"prompt":    "Legacy should not overwrite",
			"createdAt": now,
			"items":     []any{},
		}},
		"createdAt": now,
		"updatedAt": now,
	})

	if _, err := newSemanticStartupDurableEventSessionStore(root); err == nil {
		t.Fatal("divergent legacy and product thread state was silently merged")
	}
	for path, title := range map[string]string{
		filepath.Join(threadDir, "thread.json"):       "Real product thread",
		filepath.Join(legacyThreadDir, "thread.json"): "Legacy should not overwrite",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("rejected migration removed %s: %v", title, err)
		}
		var thread map[string]any
		if err := json.Unmarshal(body, &thread); err != nil || stringField(thread, "title") != title {
			t.Fatalf("rejected migration mutated %s: thread=%#v err=%v", title, thread, err)
		}
	}
}

func TestDurableEventStoreAutoTitlesFirstGoTurnAndWritesSidecars(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	thread, err := store.CreateThread(map[string]any{"autoTitle": true}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userItem := map[string]any{
		"id":         "item_turn_1_user",
		"turnId":     "turn_1",
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"kind":       "user_message",
		"text":       "Explain why the runtime history disappeared",
		"createdAt":  now,
		"finishedAt": now,
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":                "turn_1",
		"threadId":          threadID,
		"status":            "running",
		"prompt":            "Explain why the runtime history disappeared",
		"steering":          []any{},
		"attachmentIds":     []any{},
		"activeSkillIds":    []any{},
		"injectedMemoryIds": []any{},
		"createdAt":         now,
		"items":             []any{userItem},
	}, "deepseek", nil); err != nil {
		t.Fatalf("append turn: %v", err)
	}
	updated, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("get updated thread: %v", err)
	}
	if got := stringField(updated, "title"); got != "Explain why the runtime history disappeared" {
		t.Fatalf("auto title mismatch: %q", got)
	}
	if _, err := os.Stat(filepath.Join(store.threadDir(threadID), "metadata.jsonl")); err != nil {
		t.Fatalf("metadata sidecar missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.threadDir(threadID), "messages.jsonl")); err != nil {
		t.Fatalf("messages sidecar missing: %v", err)
	}
}

func TestDurableEventStoreDoesNotInventDefaultThreadModel(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create production store: %v", err)
	}
	created, err := store.CreateThread(map[string]any{"title": "No model"}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if got := stringField(created, "model"); got != "" {
		t.Fatalf("durable store must not invent a default model, got %q in %#v", got, created)
	}
	reloaded, err := store.GetThread(stringField(created, "id"))
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	if got := stringField(reloaded, "model"); got != "" {
		t.Fatalf("reloaded thread must not invent a default model, got %q in %#v", got, reloaded)
	}
}

func writeDurableTestJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON value: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create JSON parent dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write JSON file: %v", err)
	}
}

func writeDurableTestJSONL(t *testing.T, path string, value map[string]any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSONL value: %v", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open JSONL file: %v", err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatalf("write JSONL file: %v", err)
	}
}

func TestDurableEventStorePersistsBeforePublishingAndReplaysFromDisk(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "Persist order"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	events, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()

	persistStarted := make(chan struct{})
	releasePersist := make(chan struct{})
	resultCh := make(chan struct {
		event map[string]any
		order []string
		err   error
	}, 1)
	var once sync.Once
	store.beforePersistEventHook = func() {
		once.Do(func() { close(persistStarted) })
		<-releasePersist
	}

	go func() {
		event, order, err := store.RecordEvent(map[string]any{
			"kind":     "pipeline_stage",
			"threadId": threadID,
			"turnId":   "turn_1",
			"stage":    "response_received",
			"label":    "Persisted host progress",
		})
		resultCh <- struct {
			event map[string]any
			order []string
			err   error
		}{event: event, order: order, err: err}
	}()

	select {
	case <-persistStarted:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timed out waiting for persistence hook")
	}
	select {
	case live := <-events:
		t.Fatalf("live event published before persistence completed: %#v", live)
	case <-time.After(50 * time.Millisecond):
	}

	close(releasePersist)
	result := <-resultCh
	if result.err != nil {
		t.Fatalf("record event: %v", result.err)
	}
	if got, want := strings.Join(result.order, ","), "persist,publish"; got != want {
		t.Fatalf("event should persist before publish: %#v", result.order)
	}
	event := result.event
	if seq, _ := numericSeq(event["seq"]); seq != 1 {
		t.Fatalf("seq mismatch: %#v", event)
	}

	select {
	case live := <-events:
		if live["kind"] != "pipeline_stage" || live["seq"] != float64(1) {
			t.Fatalf("live event mismatch: %#v", live)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timed out waiting for live event after persistence")
	}

	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("load replay after persistence: %v", err)
	}
	if len(replay.Events) != 1 || replay.Events[0]["kind"] != "pipeline_stage" {
		t.Fatalf("persisted event should be replayable: %#v", replay.Events)
	}
	if highest, err := store.HighestSeq(threadID); err != nil || highest != 1 {
		t.Fatalf("highest seq should include persisted event: highest=%d err=%v", highest, err)
	}
	if !store.NewlineTerminated(threadID) || store.Snapshot()["pendingEventCount"] != float64(0) {
		t.Fatalf("durable persistence state mismatch: %#v", store.Snapshot())
	}
}

func TestDurableEventStoreRefreshesSequenceAfterCommittedAtomicAppendError(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "Committed append recovery"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	store.mu.Lock()
	nextSeq, err := store.nextSeqNoLock(threadID)
	if err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	committed := map[string]any{
		"seq": float64(nextSeq), "kind": "turn_completed", "threadId": threadID, "turnId": "turn_committed", "status": "completed",
	}
	if err := store.eventLog.AppendEventsAtomic(threadID, []map[string]any{committed}); err != nil {
		store.mu.Unlock()
		t.Fatal(err)
	}
	store.highestSeqByThread[threadID] = nextSeq - 1
	highest, err := store.eventLog.HighestSeqAfterCommittedTail(threadID, []map[string]any{committed})
	if err == nil {
		store.highestSeqByThread[threadID] = highest
	}
	refreshed := store.highestSeqByThread[threadID]
	store.mu.Unlock()
	if refreshed != nextSeq {
		t.Fatalf("committed append did not refresh sequence cache: got=%d want=%d", refreshed, nextSeq)
	}
	recorded, _, err := store.RecordEvent(map[string]any{
		"kind": "usage", "threadId": threadID, "turnId": "turn_after_commit",
	})
	if err != nil || recorded["seq"] != float64(nextSeq+1) {
		t.Fatalf("next event reused a committed sequence: event=%#v err=%v", recorded, err)
	}
}

func TestAcceptedFinalBundlePublishesLiveOnlyAfterWholeBundleIsDurable(t *testing.T) {
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(filepath.Join(root, "durable"))
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "Accepted bundle order"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn_bundle_order", WorkspaceRealPath: "/workspace", CaseID: "case-bundle-order",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Unix(10, 0),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	_ = json.Unmarshal(contextBody, &contextRecord)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": securityContext.TurnID, "threadId": threadID, "status": "running",
		"securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	finalizer, err := casepublication.New(filepath.Join(root, "authority"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := len(loaded.Events)
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	persistStarted := make(chan struct{})
	releasePersist := make(chan struct{})
	var once sync.Once
	store.beforePersistEventHook = func() {
		once.Do(func() { close(persistStarted) })
		<-releasePersist
	}
	resultCh := make(chan error, 1)
	go func() {
		_, persistErr := finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
			Store: store, Context: securityContext, TerminalReason: evidenceapp.TerminalSourceUnavailable,
			SourceUnavailable: true, ThreadID: threadID, TurnID: securityContext.TurnID, AcceptedAt: time.Unix(11, 0),
		})
		resultCh <- persistErr
	}()
	select {
	case <-persistStarted:
	case <-time.After(serverPositiveTestTimeout):
		t.Fatal("accepted final bundle did not reach durable persistence")
	}
	select {
	case event := <-live:
		t.Fatalf("accepted final event escaped before bundle durability: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
	close(releasePersist)
	if err := <-resultCh; err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadEventsSince(threadID, 0)
	if err != nil || len(loaded.Events) <= before {
		t.Fatalf("accepted final bundle was not durably appended: before=%d after=%d err=%v", before, len(loaded.Events), err)
	}
	select {
	case event := <-live:
		batch, parseErr := domainevent.ParseAcceptedFinalDeliveryBatchV2(event)
		if parseErr != nil {
			t.Fatalf("live accepted-final delivery was not a closed batch: event=%#v err=%v", event, parseErr)
		}
		if durable := loaded.Events[before:]; !sameJSONEventSequence(t, batch.Events, durable) {
			t.Fatalf("live accepted-final batch diverged from durable order: live=%#v durable=%#v", batch.Events, durable)
		}
	case <-time.After(time.Second):
		t.Fatal("durable accepted-final batch was not published live")
	}
	select {
	case event := <-live:
		t.Fatalf("accepted-final batch published an extra live prefix or suffix: %#v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func sameJSONEventSequence(t *testing.T, left, right []map[string]any) bool {
	t.Helper()
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func TestRuntimeTurnFailureDoesNotCommitWhenEventInventoryIsUnreadable(t *testing.T) {
	durableRootSentinel := "customer-pii-13900000058-terminal-event-inventory-root"
	threadIDSentinel := "thr_durable_13900000058"
	turnIDSentinel := "turn_customer_pii_13900000058_terminal_event_inventory"
	durableRoot := filepath.Join(t.TempDir(), durableRootSentinel)
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	store.mu.Lock()
	meta, err := store.readMetaNoLock()
	if err == nil {
		meta.ThreadCounter = 13900000057
		err = store.writeMetaNoLock(meta)
	}
	store.mu.Unlock()
	if err != nil {
		t.Fatalf("seed hostile thread identity: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"title":     "Failure record",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	if threadID != threadIDSentinel {
		t.Fatalf("hostile thread identity mismatch: got %q want %q", threadID, threadIDSentinel)
	}
	turnID := turnIDSentinel
	now := time.Now().UTC().Format(time.RFC3339Nano)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":              turnID,
		"threadId":        threadID,
		"status":          "running",
		"createdAt":       now,
		"items":           []any{},
		"steering":        []any{},
		"prompt":          "fail visibly",
		"providerId":      "deepseek",
		"securityContext": securityRecord,
	}, "deepseek", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatalf("append turn: %v", err)
	}

	if err := os.MkdirAll(store.eventsPath(threadID), 0o700); err != nil {
		t.Fatalf("replace events log with directory: %v", err)
	}
	handler := &runtimeServerHandler{store: store, caseThreads: &caseThreadAuthorityStub{}}
	if !handler.runtimeControl().RegisterTurnCancel(threadID, turnID, func() {}) {
		t.Fatal("register fixed-terminal execution owner")
	}
	defer handler.runtimeControl().UnregisterTurnCancel(threadID, turnID)
	var recordErr error
	diagnostic := captureRuntimeStderrForTest(t, func() {
		recordErr = handler.recordRuntimeTurnFailureForInput(runtimeAgentLoopInput{ThreadID: threadID, TurnID: turnID, SecurityContext: securityContext}, apploop.TurnFailureError{
			Message:  "provider stream failed after partial output",
			Code:     "provider_stream_failed",
			Severity: "error",
		})
	})
	const expectedDiagnostic = "[analytix] event=ANALYTIX_RUNTIME_TERMINAL_EVENT_INVENTORY_FAILED\n"
	if diagnostic != expectedDiagnostic {
		t.Fatalf("terminal event inventory diagnostic mismatch: got %q want %q", diagnostic, expectedDiagnostic)
	}
	for _, sentinel := range []string{durableRootSentinel, threadIDSentinel, turnIDSentinel} {
		if strings.Contains(diagnostic, sentinel) {
			t.Fatalf("terminal event inventory diagnostic leaked %q: %q", sentinel, diagnostic)
		}
	}
	// The public error text is intentionally closed; the original inventory
	// failure remains available only through the in-process cause chain.
	if recordErr == nil || turnapp.GeneralTerminalDetailClassV1(recordErr) != turnapp.GeneralTerminalDetailFinishV1 ||
		errors.Unwrap(recordErr) == nil || !strings.Contains(errors.Unwrap(recordErr).Error(), "event inventory") {
		t.Fatal("fail-closed terminal diagnostics lost their finish stage or inventory cause")
	}
	if recordErr.Error() != "general terminal failure: terminal_finish" {
		t.Fatal("terminal diagnostic exposed its private inventory cause")
	}

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	if got := stringField(reloaded, "status"); got != "running" {
		t.Fatalf("thread should remain active when no terminal outbox can commit, got %q in %#v", got, reloaded)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if got := stringField(turn, "status"); got != "running" {
		t.Fatalf("turn committed without a recoverable terminal outbox, got %q in %#v", got, turn)
	}
	items := listAny(turn["items"])
	if len(items) != 0 || turn["generalTerminalPublication"] != nil {
		t.Fatalf("failed CAS leaked an item or publication authority: items=%#v turn=%#v", items, turn)
	}
	eventEntries, err := os.ReadDir(store.eventsPath(threadID))
	if err != nil {
		t.Fatalf("read unreadable event inventory sentinel directory: %v", err)
	}
	if len(eventEntries) != 0 {
		t.Fatalf("failed terminal persistence wrote event inventory entries: %#v", eventEntries)
	}
}

func TestRuntimeTurnFailureDiscardsRejectedAssistantDrafts(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":        "thr_failure_delta",
		"title":     "Failure delta",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_failure_delta"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":              turnID,
		"threadId":        threadID,
		"status":          "running",
		"createdAt":       now,
		"items":           []any{},
		"steering":        []any{},
		"prompt":          "fail after partial output",
		"providerId":      "deepseek",
		"securityContext": securityRecord,
	}, "deepseek", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatalf("append turn: %v", err)
	}

	recorder := apploop.NewRuntimeEventRecorder(store)
	if err := recorder.AssistantTextDelta(threadID, turnID, "partial answer"); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("assistant draft should be rejected by the durable boundary: %v", err)
	}
	handler := &runtimeServerHandler{store: store, caseThreads: &caseThreadAuthorityStub{}}
	if !handler.runtimeControl().RegisterTurnCancel(threadID, turnID, func() {}) {
		t.Fatal("register fixed-terminal execution owner")
	}
	defer handler.runtimeControl().UnregisterTurnCancel(threadID, turnID)
	if err := handler.recordRuntimeTurnFailureForInput(runtimeAgentLoopInput{ThreadID: threadID, TurnID: turnID, SecurityContext: securityContext}, apploop.TurnFailureError{
		Message:  "provider stream failed after partial output",
		Code:     "provider_stream_failed",
		Severity: "error",
	}); err != nil {
		t.Fatalf("record failure: %v", err)
	}

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if got := stringField(turn, "status"); got != "failed" {
		t.Fatalf("turn should be failed, got %q in %#v", got, turn)
	}
	items := listAny(turn["items"])
	if len(items) != 1 {
		t.Fatalf("failed turn should persist only the fixed error item, got %#v", items)
	}
	errorItem, _ := items[0].(map[string]any)
	if stringField(errorItem, "kind") != "error" ||
		stringField(errorItem, "code") != domainfailure.CodeProviderStreamInterrupted ||
		stringField(errorItem, "message") != domainfailure.New(domainfailure.CodeProviderStreamInterrupted, nil).Message() {
		t.Fatalf("error item mismatch: %#v", errorItem)
	}

	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	partialPublished := false
	for _, event := range replay.Events {
		if strings.Contains(fmt.Sprint(event), "partial answer") || stringField(event, "kind") == "assistant_text_delta" {
			partialPublished = true
		}
	}
	if partialPublished {
		t.Fatalf("replay exposed an unverified assistant draft: %#v", replay.Events)
	}
}

func TestRuntimeRestoreAbortsStaleRunningMainTurn(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":        "thr_restore_stale_running",
		"title":     "Stale running",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_12"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	userItem := map[string]any{
		"id":        "item_" + turnID + "_user",
		"turnId":    turnID,
		"threadId":  threadID,
		"role":      "user",
		"status":    "completed",
		"createdAt": now,
		"kind":      "user_message",
		"text":      "continue after restart",
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id":              turnID,
		"threadId":        threadID,
		"status":          "running",
		"createdAt":       now,
		"startedAt":       now,
		"items":           []any{userItem},
		"steering":        []any{},
		"prompt":          "continue after restart",
		"providerId":      "deepseek",
		"securityContext": securityRecord,
	}, "deepseek", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatalf("append running turn: %v", err)
	}
	authorityRoot := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatalf("create pending work authority directory: %v", err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatalf("create pending work authority: %v", err)
	}
	pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatalf("create pending work store: %v", err)
	}
	continuations, err := newServerTestContinuationService(t, filepath.Join(t.TempDir(), "continuations"), authority)
	if err != nil {
		t.Fatalf("create continuation service: %v", err)
	}
	caseAuthority := &caseThreadAuthorityStub{threads: map[string]bool{}}
	store.SetCaseThreadAuthority(caseAuthority)
	handler := &runtimeServerHandler{
		store: store, pendingWork: pendingworkapp.NewService(authority, pendingStore, store),
		continuations: continuations, caseThreads: caseAuthority,
	}
	if err := apploop.NewRuntimeEventRecorder(store).AssistantTextDelta(threadID, turnID, "visible before crash"); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("assistant draft should be rejected before restart: %v", err)
	}

	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatalf("restore runtime state: %v", err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	if got := stringField(reloaded, "status"); got != "idle" {
		t.Fatalf("thread should be idle after stale running cleanup, got %q in %#v", got, reloaded)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if got := stringField(turn, "status"); got != "aborted" {
		t.Fatalf("stale running turn should be aborted, got %q in %#v", got, turn)
	}
	items := listAny(turn["items"])
	if len(items) != 2 {
		t.Fatalf("expected user and runtime restart error items, got %#v", items)
	}
	if strings.Contains(fmt.Sprint(items), "visible before crash") {
		t.Fatalf("restart materialized an unverified assistant draft: %#v", items)
	}
	errorItem, _ := items[1].(map[string]any)
	if stringField(errorItem, "kind") != "error" || stringField(errorItem, "code") != "runtime_restarted" {
		t.Fatalf("runtime restart error item mismatch: %#v", errorItem)
	}
	if _, err := store.RewindThread(threadID, turnID); err == nil || !strings.Contains(err.Error(), "current security context") {
		t.Fatalf("rewind removed the current frozen security authority: %v", err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	abortedEvent := false
	for _, event := range replay.Events {
		if stringField(event, "kind") == "turn_aborted" &&
			stringField(event, "turnId") == turnID &&
			stringField(event, "code") == "runtime_restarted" {
			abortedEvent = true
		}
	}
	if !abortedEvent {
		t.Fatalf("replay missing runtime_restarted turn_aborted event: %#v", replay.Events)
	}
}

type quarantinedCaseThreadAuthorityStub struct {
	*caseThreadAuthorityStub
}

func (*quarantinedCaseThreadAuthorityStub) CanExecute(string) bool { return false }

func TestRuntimeRestoreQuarantinedActiveCaseThreadDiagnosticIsValueFree(t *testing.T) {
	const threadIDSentinel = "thr_durable_13900000059"
	const turnIDSentinel = "turn_customer_pii_13900000059_quarantined_restart"
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	store.mu.Lock()
	meta, err := store.readMetaNoLock()
	if err == nil {
		meta.ThreadCounter = 13900000058
		err = store.writeMetaNoLock(meta)
	}
	store.mu.Unlock()
	if err != nil {
		t.Fatalf("seed hostile thread identity: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Quarantined restart", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	if threadID != threadIDSentinel {
		t.Fatalf("hostile thread identity mismatch: got %q want %q", threadID, threadIDSentinel)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnIDSentinel, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnIDSentinel, "threadId": threadID, "status": "running", "createdAt": now, "startedAt": now,
		"items": []any{}, "steering": []any{}, "prompt": "remain quarantined", "providerId": "deepseek",
		"securityContext": securityRecord,
	}, "deepseek", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatalf("append running turn: %v", err)
	}
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatalf("load pre-restore sequence: %v", err)
	}
	authorityRoot := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(authorityRoot, 0o700); err != nil {
		t.Fatalf("create pending work authority directory: %v", err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(authorityRoot, "authority.json"), false)
	if err != nil {
		t.Fatalf("create pending work authority: %v", err)
	}
	pendingStore, err := newServerTestPendingWorkStore(t, filepath.Join(t.TempDir(), "pending-work"))
	if err != nil {
		t.Fatalf("create pending work store: %v", err)
	}
	continuations, err := newServerTestContinuationService(t, filepath.Join(t.TempDir(), "continuations"), authority)
	if err != nil {
		t.Fatalf("create continuation service: %v", err)
	}
	caseAuthority := &quarantinedCaseThreadAuthorityStub{caseThreadAuthorityStub: &caseThreadAuthorityStub{threads: map[string]bool{threadID: true}}}
	store.SetCaseThreadAuthority(caseAuthority)
	handler := &runtimeServerHandler{
		store: store, pendingWork: pendingworkapp.NewService(authority, pendingStore, store),
		continuations: continuations, caseThreads: caseAuthority,
	}
	diagnostic := captureRuntimeStderrForTest(t, func() {
		if err := handler.restoreRuntimeState(); err != nil {
			t.Fatalf("restore runtime state: %v", err)
		}
	})
	const expectedDiagnostic = "[analytix] event=ANALYTIX_RUNTIME_QUARANTINED_ACTIVE_CASE_THREAD\n"
	if diagnostic != expectedDiagnostic {
		t.Fatalf("quarantined active case diagnostic mismatch: got %q want %q", diagnostic, expectedDiagnostic)
	}
	for _, sentinel := range []string{threadIDSentinel, turnIDSentinel} {
		if strings.Contains(diagnostic, sentinel) {
			t.Fatalf("quarantined active case diagnostic leaked %q: %q", sentinel, diagnostic)
		}
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload quarantined thread: %v", err)
	}
	if stringField(reloaded, "status") != "running" {
		t.Fatalf("quarantined thread status changed: %#v", reloaded)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("quarantined thread turn inventory changed: %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	if stringField(turn, "status") != "running" || len(listAny(turn["items"])) != 0 {
		t.Fatalf("quarantined active turn changed during restart: %#v", turns)
	}
	afterSeq, err := store.HighestSeq(threadID)
	if err != nil || afterSeq != beforeSeq {
		t.Fatalf("quarantined restart changed event sequence: before=%d after=%d err=%v", beforeSeq, afterSeq, err)
	}
}

func TestRuntimeRestoreIgnoresUnsignedLegacyCacheDiagnostics(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	runtimeHandler := handler.(*runtimeServerHandler)
	thread, err := runtimeHandler.store.CreateThread(map[string]any{
		"title":      "Cache restore",
		"workspace":  "/workspace/analytix",
		"model":      "deepseek-chat",
		"providerId": "deepseek",
	}, "/workspace/analytix")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	if _, _, err := runtimeHandler.store.RecordEvent(map[string]any{
		"kind":     "usage",
		"threadId": threadID,
		"turnId":   "turn_cache_1",
		"usage": map[string]any{
			"promptTokens":    100,
			"cacheHitTokens":  64,
			"cacheMissTokens": 36,
		},
		"cacheDiagnostics": map[string]any{
			"prefixHash":       "prefix-a",
			"systemHash":       "system-a",
			"toolsHash":        "tools-a",
			"prefixItemsHash":  "items-a",
			"toolSchemaTokens": 123,
			"toolCount":        17,
			"toolSourcesHash":  "sources-a",
			"toolSourceIds":    []any{"builtin"},
			"provider":         "deepseek",
			"providerId":       "deepseek",
			"endpointFormat":   "chat_completions",
			"model":            "deepseek-chat",
		},
	}); err != nil {
		t.Fatalf("record usage event: %v", err)
	}

	runtimeHandler.cachePrefixShapes = map[string]appusage.PrefixBaseline{}
	if err := runtimeHandler.restoreRuntimeState(); err != nil {
		t.Fatalf("restore runtime state: %v", err)
	}
	diagnostics := runtimeHandler.runtimeCacheDiagnostics(threadID, domainsecurity.TurnSecurityContext{}, provider.Result{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		PrefixShape: provider.PrefixShape{
			PrefixHash:       "prefix-b",
			SystemHash:       "system-a",
			ToolsHash:        "tools-a",
			PrefixItemsHash:  "items-b",
			ToolSchemaTokens: 123,
			ToolCount:        17,
			ToolSourcesHash:  "sources-a",
			ToolSourceIDs:    []string{"builtin"},
			Provider:         "deepseek",
			ProviderID:       "deepseek",
			EndpointFormat:   "chat_completions",
			Model:            "deepseek-chat",
		},
		Usage: domainmodel.Usage{
			PromptTokens:    100,
			CacheHitTokens:  70,
			CacheMissTokens: 30,
			HasCacheHit:     true,
			HasCacheMiss:    true,
		},
	})
	if diagnostics["prefixChanged"] == true || diagnostics["cacheBaselineObserved"] != false {
		t.Fatalf("unsigned legacy cache diagnostics became restart comparison authority: %#v", diagnostics)
	}
	if diagnostics["cacheHitTokens"] != 70 || diagnostics["cacheMissTokens"] != 30 {
		t.Fatalf("DeepSeek cache token diagnostics missing after restore: %#v", diagnostics)
	}
}

func TestRuntimeCacheDiagnosticsIncludesSanitizedContextEpochReasons(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	thread, err := handler.store.CreateThread(map[string]any{"title": "Epoch diagnostics", "workspace": "/workspace"}, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	state, err := contextepochapp.DefaultState(threadID, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = contextepochapp.Upsert(state, domaincontextepoch.SourceEntry{
		SourceID: "diagnostics-a", Kind: "diagnostics", Reference: "/private/case/path",
		Digest: domaincontextepoch.SHA256Hex([]byte("diagnostics")), TrustState: domaincontextepoch.TrustTrusted,
		PromptBoundary: domaincontextepoch.BoundaryDiagnosticsOnly, ActivationState: domaincontextepoch.ActivationInactive,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := contextepochapp.Reconcile(contextepochapp.ReconcileInput{State: state, Cause: contextepochapp.CauseRequestBoundary, At: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{"id": "turn_epoch", "threadId": threadID, "status": "running", "items": []any{}}, "", map[string]any{
		"contextEpochState": contextepochapp.PublicState(reconciled.State),
	}); err != nil {
		t.Fatal(err)
	}
	diagnostics := handler.runtimeCacheDiagnostics(threadID, domainsecurity.TurnSecurityContext{}, provider.Result{PrefixShape: provider.PrefixShape{PrefixHash: "prefix", Provider: "deepseek"}})
	if diagnostics["contextEpochStateValid"] != true || diagnostics["contextEpoch"] != uint64(2) {
		t.Fatalf("context epoch diagnostics missing: %#v", diagnostics)
	}
	if diagnostics["cacheTelemetryPresent"] != false {
		t.Fatalf("missing cache telemetry must remain unknown, not zero: %#v", diagnostics)
	}
	if _, exists := diagnostics["cacheHitRate"]; exists {
		t.Fatalf("unknown cache telemetry must not publish a zero hit rate: %#v", diagnostics)
	}
	serialized, _ := json.Marshal(diagnostics)
	if strings.Contains(string(serialized), "/private/case/path") {
		t.Fatalf("raw source paths must remain prompt- and diagnostics-invisible: %s", serialized)
	}
}

func TestDurableCompactionAdvancesContextEpochOnceAndPersistsRecoveryDigest(t *testing.T) {
	handler, threadID, current := newUnboundCompactionFixture(t)
	result, err := handler.runtimeThreadService().Compact(context.Background(), threadID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if stringField(result, "sourceDigest") == "" {
		t.Fatalf("compaction recovery digest missing: %#v", result)
	}
	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	state, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || state.AcceptedSnapshot.Epoch != current.ContextEpoch+1 || state.AcceptedSnapshot.RecoveryDigest != stringField(result, "sourceDigest") {
		t.Fatalf("compaction epoch state mismatch: state=%#v ok=%v err=%v", state, ok, err)
	}
	turns := listAny(reloaded["turns"])
	compactTurn, _ := turns[0].(map[string]any)
	securityContext, contextErr := domainsecurity.ParseTurnSecurityContext(compactTurn["securityContext"])
	if compactTurn["contextEpochSnapshot"] == nil || contextErr != nil || securityContext.ContextEpoch != state.AcceptedSnapshot.Epoch ||
		securityContext.TurnID != stringField(result, "turnId") {
		t.Fatalf("compaction turn must carry the accepted epoch snapshot: %#v", compactTurn)
	}
	if _, err := handler.runtimeThreadService().Compact(context.Background(), threadID, "manual-again"); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = handler.store.GetThread(threadID)
	state, _, err = contextepochapp.StateFromThread(reloaded)
	if err != nil || state.AcceptedSnapshot.Epoch != current.ContextEpoch+1 {
		t.Fatalf("repeated compaction must not create an epoch loop: %#v err=%v", state, err)
	}
}

func TestRuntimeRestoreMarksUnreadableContextSourceUnavailableWithoutLoop(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	thread, err := handler.store.CreateThread(map[string]any{"title": "Restart epoch", "workspace": workspacetest.New(t)}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)
	content := "restart context"
	state, err := contextepochapp.BootstrapState(threadID, 4, []domaincontextepoch.SourceEntry{{
		Version: domaincontextepoch.ContractVersion, SourceID: "memory-a", Kind: "memory",
		Digest: domaincontextepoch.SHA256Hex([]byte(content)), Sequence: 1,
		TrustState: domaincontextepoch.TrustUntrusted, PromptBoundary: domaincontextepoch.BoundaryDynamicContext,
		TokenBudget: 16, ActivationState: domaincontextepoch.ActivationActive, ActivationReason: "explicit-turn-selection",
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.store.PatchThread(threadID, map[string]any{"contextEpochState": contextepochapp.PublicState(state)}); err != nil {
		t.Fatal(err)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := handler.store.GetThread(threadID)
	recovered, ok, err := contextepochapp.StateFromThread(reloaded)
	if err != nil || !ok || recovered.AcceptedSnapshot.Epoch != 5 || recovered.Registry[0].TrustState != domaincontextepoch.TrustUnavailable {
		t.Fatalf("restart recovery did not fail closed: state=%#v ok=%v err=%v", recovered, ok, err)
	}
	if err := handler.restoreRuntimeState(); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = handler.store.GetThread(threadID)
	recovered, _, _ = contextepochapp.StateFromThread(reloaded)
	if recovered.AcceptedSnapshot.Epoch != 5 {
		t.Fatalf("restart recovery must not repeatedly bump an unavailable source: %#v", recovered)
	}
}

func TestRuntimeRestoreRejectsCorruptContextEpochState(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	thread, err := handler.store.CreateThread(map[string]any{"title": "Corrupt epoch", "workspace": workspacetest.New(t)}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": "turn_corrupt_epoch", "threadId": threadID, "status": "completed", "items": []any{},
	}, "", map[string]any{"contextEpochState": map[string]any{"version": float64(1), "threadId": threadID}}); err != nil {
		t.Fatal(err)
	}
	if err := handler.restoreRuntimeState(); err == nil || !strings.Contains(err.Error(), "invalid persisted context epoch state") {
		t.Fatalf("corrupt context epoch state must fail closed: %v", err)
	}
}

func TestRuntimeThreadEventsLiveAndReplayAfterPersistence(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	runtimeHandler, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("runtime handler type mismatch: %T", handler)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	thread, err := runtimeHandler.store.CreateThread(map[string]any{"title": "HTTP persist order"}, workspacetest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	persistStarted := make(chan struct{})
	releasePersist := make(chan struct{})
	recordCh := make(chan struct {
		event map[string]any
		order []string
		err   error
	}, 1)
	var persistOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releasePersist) }) })
	runtimeHandler.store.beforePersistEventHook = func() {
		persistOnce.Do(func() { close(persistStarted) })
		<-releasePersist
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatalf("build live SSE request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open live SSE: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live SSE status mismatch: %d", response.StatusCode)
	}

	frameCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		frame, err := readSSEFrameContaining(response.Body, "event: pipeline_stage")
		if err != nil {
			errCh <- err
			return
		}
		frameCh <- frame
	}()

	go func() {
		event, order, err := runtimeHandler.store.RecordEvent(map[string]any{
			"kind":     "pipeline_stage",
			"threadId": threadID,
			"turnId":   "turn_1",
			"stage":    "response_received",
			"label":    "Persisted host progress",
		})
		recordCh <- struct {
			event map[string]any
			order []string
			err   error
		}{event: event, order: order, err: err}
	}()
	select {
	case <-persistStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("persistence hook did not start")
	}
	select {
	case frame := <-frameCh:
		t.Fatalf("live SSE frame arrived before persistence completed: %s", frame)
	case err := <-errCh:
		t.Fatalf("read live SSE before persistence: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releasePersist) })
	result := <-recordCh
	if result.err != nil {
		t.Fatalf("record event: %v", result.err)
	}
	if got, want := strings.Join(result.order, ","), "persist,publish"; got != want {
		t.Fatalf("event should persist before publish: %#v", result.order)
	}
	event := result.event
	if seq, _ := numericSeq(event["seq"]); seq != 1 {
		t.Fatalf("seq mismatch: %#v", event)
	}

	select {
	case frame := <-frameCh:
		if !strings.Contains(frame, "response_received") {
			t.Fatalf("live frame missing persisted host progress: %s", frame)
		}
	case err := <-errCh:
		t.Fatalf("read live SSE frame: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("live SSE did not receive persisted event")
	}

	replay := requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0")
	if !strings.Contains(replay, "event: pipeline_stage") || !strings.Contains(replay, "response_received") {
		t.Fatalf("HTTP replay should include persisted event:\n%s", replay)
	}
	if highest, err := runtimeHandler.store.HighestSeq(threadID); err != nil || highest != 1 {
		t.Fatalf("highest seq should include persisted event: highest=%d err=%v", highest, err)
	}
	waitForDurableStoreCondition(t, func() bool {
		return runtimeHandler.store.NewlineTerminated(threadID) &&
			runtimeHandler.store.Snapshot()["pendingEventCount"] == float64(0)
	})
}

func TestRuntimeThreadEventsLiveRejectsAssistantDraftBeforeTurnCompleted(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	runtimeHandler, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("runtime handler type mismatch: %T", handler)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	workspace := workspacetest.New(t)
	thread, err := runtimeHandler.store.CreateThread(map[string]any{"title": "HTTP text order", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatalf("build live SSE request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open live SSE: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live SSE status mismatch: %d", response.StatusCode)
	}

	frameCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		frame, err := readSSEFrameContaining(response.Body, "event: general_terminal_batch")
		if err != nil {
			errCh <- err
			return
		}
		frameCh <- frame
	}()

	if _, _, err := runtimeHandler.store.RecordEvent(map[string]any{
		"kind":     "assistant_text_delta",
		"threadId": threadID,
		"turnId":   "turn_1",
		"itemId":   "item_turn_1_text",
		"item": map[string]any{
			"id": "item_turn_1_text", "threadId": threadID, "turnId": "turn_1", "kind": "assistant_text",
			"role": "assistant", "status": "running", "createdAt": time.Now().UTC().Format(time.RFC3339Nano),
			"text": "1\n",
		},
	}); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("assistant draft should be rejected before terminal publication: %v", err)
	}

	select {
	case frame := <-frameCh:
		t.Fatalf("terminal frame arrived before turn_completed was recorded: %s", frame)
	case err := <-errCh:
		t.Fatalf("read live SSE before terminal event: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	turnID := "turn_1"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("live-sse-manifest")), ContextEpoch: 1,
		IssuedAt: time.Unix(10, 0).UTC(),
	})
	contextRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := runtimeHandler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatalf("append live SSE turn: %v", err)
	}
	if result, err := turnapp.CommitCompletedTurn(turnapp.CommitCompletionInput{
		Store: runtimeHandler.store, SecurityContext: securityContext, ThreadID: threadID, TurnID: turnID,
		Model: "deepseek", CreatedAt: "2026-07-15T00:00:00Z",
		FinishedAt: "2026-07-15T00:00:00Z",
	}); err != nil || !result.Changed {
		t.Fatalf("commit canonical live SSE terminal: changed=%v err=%v", result.Changed, err)
	}
	select {
	case frame := <-frameCh:
		if strings.Contains(frame, "1\\n") || !strings.Contains(frame, "event: general_terminal_batch") ||
			!strings.Contains(frame, `"kind":"turn_completed"`) {
			t.Fatalf("live terminal frame contained an assistant draft: %s", frame)
		}
	case err := <-errCh:
		t.Fatalf("read live terminal SSE frame: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("live SSE did not receive the atomic general terminal batch")
	}
}

func TestRuntimeThreadEventsLiveHeartbeatIsStructuredSSE(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	runtimeHandler, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("runtime handler type mismatch: %T", handler)
	}
	runtimeHandler.heartbeatInterval = 10 * time.Millisecond
	thread, err := runtimeHandler.store.CreateThread(map[string]any{
		"title": "Live SSE heartbeat",
	}, workspacetest.New(t))
	if err != nil {
		t.Fatalf("create live SSE thread: %v", err)
	}
	threadID := stringField(thread, "id")
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatalf("build live SSE request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open live SSE: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live SSE status mismatch: %d", response.StatusCode)
	}

	frame, err := readSSEFrameContaining(response.Body, "event: heartbeat")
	if err != nil {
		t.Fatalf("read live SSE heartbeat: %v", err)
	}
	for _, needle := range []string{
		"id: 0",
		"event: heartbeat",
		`"kind":"heartbeat"`,
		fmt.Sprintf(`"threadId":%q`, threadID),
		`"seq":0`,
	} {
		if !strings.Contains(frame, needle) {
			t.Fatalf("heartbeat frame missing %q:\n%s", needle, frame)
		}
	}
}

func TestRuntimeThreadEventsLiveSetupErrorUsesStructuredSSE(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	runtimeHandler, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("runtime handler type mismatch: %T", handler)
	}
	thread, err := runtimeHandler.store.CreateThread(map[string]any{
		"title": "Live SSE setup error",
	}, workspacetest.New(t))
	if err != nil {
		t.Fatalf("create live SSE thread: %v", err)
	}
	threadID := stringField(thread, "id")
	if err := os.MkdirAll(runtimeHandler.store.eventsPath(threadID), 0o700); err != nil {
		t.Fatalf("create unreadable events path: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0&live=1", nil)
	if err != nil {
		t.Fatalf("build live SSE request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open live SSE: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live SSE setup errors should stay on the SSE channel, got status %d", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("live SSE setup error content type mismatch: %q", contentType)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read live SSE setup error: %v", err)
	}
	replay := string(data)
	for _, needle := range []string{
		": connected",
		"event: error",
		`"kind":"error"`,
		fmt.Sprintf(`"threadId":%q`, threadID),
		`"code":"sse_setup_error"`,
		`"severity":"error"`,
		`"terminal":true`,
		`"phase":"highest_seq"`,
	} {
		if !strings.Contains(replay, needle) {
			t.Fatalf("live SSE setup error missing %q:\n%s", needle, replay)
		}
	}
}

func TestRuntimeThreadEventsReplaySetupErrorUsesStructuredSSE(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
	})
	runtimeHandler, ok := handler.(*runtimeServerHandler)
	if !ok {
		t.Fatalf("runtime handler type mismatch: %T", handler)
	}
	thread, err := runtimeHandler.store.CreateThread(map[string]any{
		"title": "Replay SSE setup error",
	}, workspacetest.New(t))
	if err != nil {
		t.Fatalf("create replay SSE thread: %v", err)
	}
	threadID := stringField(thread, "id")
	if err := os.MkdirAll(runtimeHandler.store.eventsPath(threadID), 0o700); err != nil {
		t.Fatalf("create unreadable events path: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0", nil)
	if err != nil {
		t.Fatalf("build replay SSE request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open replay SSE: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("replay SSE setup errors should stay on the SSE channel, got status %d", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("replay SSE setup error content type mismatch: %q", contentType)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read replay SSE setup error: %v", err)
	}
	replay := string(data)
	for _, needle := range []string{
		"event: error",
		`"kind":"error"`,
		fmt.Sprintf(`"threadId":%q`, threadID),
		`"code":"sse_setup_error"`,
		`"severity":"error"`,
		`"terminal":true`,
		`"phase":"highest_seq"`,
	} {
		if !strings.Contains(replay, needle) {
			t.Fatalf("replay SSE setup error missing %q:\n%s", needle, replay)
		}
	}
}

func readSSEFrameContaining(body io.Reader, needle string) (string, error) {
	reader := bufio.NewReader(body)
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) == "" {
			text := frame.String()
			if strings.Contains(text, needle) {
				return text, nil
			}
			frame.Reset()
			continue
		}
		frame.WriteString(line)
	}
}

func requestRuntimeSSEText(t *testing.T, url string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build SSE replay request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("SSE replay request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("SSE replay status mismatch: %d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read SSE replay: %v", err)
	}
	return string(data)
}

func waitForDurableStoreCondition(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("durable store condition was not met")
}
