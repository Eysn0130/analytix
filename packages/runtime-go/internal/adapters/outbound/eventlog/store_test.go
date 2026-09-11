package eventlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestStoreAppendReadHighestAndTypedEvents(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent("thr_1", map[string]any{"seq": float64(1), "kind": "tool_call_ready", "threadId": "thr_1", "turnId": "turn_1"}); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := store.Append(domainevent.RuntimeEvent{Seq: 2, Kind: "usage", ThreadID: "thr_1"}); err != nil {
		t.Fatalf("append second: %v", err)
	}
	result, err := store.LoadSince("thr_1", 0)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(result.Events) != 2 || result.Events[0]["kind"] != "tool_call_ready" || result.Events[1]["kind"] != "usage" {
		t.Fatalf("events should preserve physical sequence order: %#v", result.Events)
	}
	if highest, err := store.HighestSeq("thr_1"); err != nil || highest != 2 {
		t.Fatalf("highest seq mismatch: highest=%d err=%v", highest, err)
	}
	typed, diagnostics, err := store.ReadFromSeq("thr_1", domainevent.Seq(1))
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("typed read diagnostics mismatch: diagnostics=%#v err=%v", diagnostics, err)
	}
	if len(typed) != 1 || typed[0].Seq != 2 || typed[0].Kind != "usage" || typed[0].ThreadID != "thr_1" {
		t.Fatalf("typed events mismatch: %#v", typed)
	}
	if !store.NewlineTerminated("thr_1") {
		t.Fatal("event log should stay newline terminated")
	}
}

func TestStoreRejectsRestrictedEvidenceBeforeAnyDurableWrite(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	threadID := "thr_restricted_evidence"
	if err := store.AppendEvent(threadID, map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	restricted := map[string]any{
		"seq": float64(2), "kind": "usage", "threadId": threadID,
		"review": map[string]any{"output": map[string]any{
			"schemaVersion": 2, "purpose": "analytix.source-field-binding/v2",
			"sourceExactValue": "0012-3456789012345678",
		}},
	}
	if err := store.AppendEvent(threadID, restricted); err == nil {
		t.Fatal("restricted evidence was appended as one event")
	}
	if err := store.AppendEventsAtomic(threadID, []map[string]any{restricted}); err == nil {
		t.Fatal("restricted evidence was appended as an atomic bundle")
	}
	if _, _, err := store.AppendRawLine(threadID,
		`purpose: analytix.source-row-lineage/v1, lineageDigest: `+strings.Repeat("a", 64)); err == nil {
		t.Fatal("restricted evidence was appended as a malformed raw line")
	}
	after, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("rejected restricted evidence changed the event log: after=%q err=%v", after, err)
	}
}

func TestStoreRejectsCredentialsAndPIIBeforeEveryDurableWrite(t *testing.T) {
	for _, entrypoint := range []string{"single", "atomic", "raw"} {
		t.Run(entrypoint, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)
			threadID := "thr_public_content_guard"
			event := map[string]any{
				"seq": float64(1), "kind": "usage", "threadId": threadID,
				"message": "Authorization: Bearer opaque-event-secret-123",
				"details": map[string]any{"account": "6222020202020202020"},
			}
			var err error
			switch entrypoint {
			case "single":
				err = store.AppendEvent(threadID, event)
			case "atomic":
				err = store.AppendEventsAtomic(threadID, []map[string]any{event})
			case "raw":
				body, marshalErr := json.Marshal(event)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				_, _, err = store.AppendRawLine(threadID, string(body))
			}
			if err == nil {
				t.Fatal("credential/PII event was accepted")
			}
			if _, statErr := os.Stat(store.ThreadDir(threadID)); !os.IsNotExist(statErr) {
				t.Fatalf("rejected event created durable state: %v", statErr)
			}
		})
	}
}

func TestStoreLegacyReplayReplacesUnsafeContentAndPreservesSequence(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	threadID := "thr_legacy_public_content"
	if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	const secret = "opaque-event-secret-123"
	const account = "6222020202020202020"
	lines := strings.Join([]string{
		`{"seq":1,"kind":"usage","threadId":"thr_legacy_public_content"}`,
		`{"seq":2,"kind":"usage","threadId":"thr_legacy_public_content","message":"Authorization: Bearer ` + secret + `"}`,
		`{"seq":3,"kind":"usage","threadId":"thr_legacy_public_content","message":"account number: ` + account + `"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(store.EventsPath(threadID), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.LoadSince(threadID, 0)
	if err != nil || len(result.Events) != 3 || len(result.Diagnostics) != 2 {
		t.Fatalf("legacy unsafe replay containment mismatch: result=%#v err=%v", result, err)
	}
	for index, seq := range []float64{1, 2, 3} {
		if result.Events[index]["seq"] != seq {
			t.Fatalf("sequence frontier changed: %#v", result.Events)
		}
	}
	for _, event := range result.Events[1:] {
		if event["kind"] != "content_redacted" || event["code"] != "private_content_removed" {
			t.Fatalf("unsafe legacy record was not tombstoned: %#v", event)
		}
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte(secret)) || bytes.Contains(encoded, []byte(account)) {
		t.Fatalf("legacy bytes crossed replay or diagnostics: %s", encoded)
	}
	if highest, highestErr := store.HighestSeq(threadID); highestErr != nil || highest != 3 {
		t.Fatalf("redaction regressed event frontier: highest=%d err=%v", highest, highestErr)
	}
}

func TestStoreRejectsInvalidNewLifecycleAtEveryWriteEntrypointBeforeSideEffects(t *testing.T) {
	for _, entrypoint := range []string{"single", "non_atomic", "atomic", "atomic_at_frontier", "raw"} {
		t.Run(entrypoint, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)
			replaceCalls := 0
			syncCalls := 0
			cutCalls := 0
			store.replaceFile = func(string, string) error {
				replaceCalls++
				return nil
			}
			store.syncParentFolder = func(string) error {
				syncCalls++
				return nil
			}
			store.atomicAppendCut = func(string) error {
				cutCalls++
				return nil
			}
			threadID := "thr_invalid_lifecycle"
			event := map[string]any{
				"seq": float64(1), "threadId": threadID,
				"kind": "pipeline_stage", "stage": "subagent_ completed",
			}
			var err error
			switch entrypoint {
			case "single":
				err = store.AppendEvent(threadID, event)
			case "non_atomic":
				err = store.AppendEvents(threadID, []map[string]any{event}, false)
			case "atomic":
				err = store.AppendEventsAtomic(threadID, []map[string]any{event})
			case "atomic_at_frontier":
				err = store.AppendEventsAtomicAtFrontier(threadID, []map[string]any{event}, EventLogFrontierV1{})
			case "raw":
				body, marshalErr := json.Marshal(event)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				_, _, err = store.AppendRawLine(threadID, string(body))
			}
			if !errors.Is(err, ErrInvalidEventLifecycleV1) {
				t.Fatalf("append error=%v, want closed lifecycle rejection", err)
			}
			if replaceCalls != 0 || syncCalls != 0 || cutCalls != 0 {
				t.Fatalf("rejection reached durable hooks: replace=%d sync=%d cut=%d", replaceCalls, syncCalls, cutCalls)
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("rejection created durable state: entries=%#v err=%v", entries, readErr)
			}
		})
	}
}

func TestStoreLegacyReplayDoesNotGrandfatherUnknownLifecycle(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	threadID := "thr_legacy_lifecycle"
	if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"seq":1,"kind":"legacy_open_lifecycle","threadId":"thr_legacy_lifecycle","status":"provider_chosen"}` + "\n")
	if err := os.WriteFile(store.EventsPath(threadID), legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.LoadSince(threadID, 0)
	if err != nil || len(result.Events) != 1 || result.Events[0]["kind"] != "legacy_open_lifecycle" {
		t.Fatalf("legacy replay was not preserved for projection/diagnosis: result=%#v err=%v", result, err)
	}
	candidate := map[string]any{
		"seq": float64(2), "kind": "legacy_open_lifecycle", "threadId": threadID, "status": "provider_chosen",
	}
	if err := store.AppendEvent(threadID, candidate); !errors.Is(err, ErrInvalidEventLifecycleV1) {
		t.Fatalf("legacy lifecycle was grandfathered into a new append: %v", err)
	}
	after, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil || !bytes.Equal(after, legacy) {
		t.Fatalf("rejected legacy re-append changed event log: after=%q err=%v", after, err)
	}
}

func TestStoreAppendEventsAtomicCommitsWholeBundleOrNothing(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent("thr_bundle", map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": "thr_bundle", "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	bundle := []map[string]any{
		{"seq": float64(2), "kind": "item_completed", "threadId": "thr_bundle", "turnId": "turn_1"},
		{"seq": float64(3), "kind": "turn_completed", "threadId": "thr_bundle", "turnId": "turn_1", "status": "completed"},
	}
	if err := store.AppendEventsAtomic("thr_bundle", bundle); err != nil {
		t.Fatalf("append atomic bundle: %v", err)
	}
	result, err := store.LoadSince("thr_bundle", 0)
	if err != nil || len(result.Events) != 3 || result.Events[1]["kind"] != "item_completed" || result.Events[2]["kind"] != "turn_completed" {
		t.Fatalf("whole bundle was not committed: events=%#v err=%v", result.Events, err)
	}

	invalid := []map[string]any{
		{"seq": float64(4), "kind": "usage", "threadId": "thr_bundle", "turnId": "turn_2"},
		{"seq": float64(5), "kind": "turn_completed", "threadId": "thr_other", "turnId": "turn_2", "status": "completed"},
	}
	if err := store.AppendEventsAtomic("thr_bundle", invalid); err == nil {
		t.Fatal("mixed-thread bundle should fail closed")
	}
	result, err = store.LoadSince("thr_bundle", 0)
	if err != nil || len(result.Events) != 3 {
		t.Fatalf("invalid bundle changed the committed log: events=%#v err=%v", result.Events, err)
	}
}

func TestStoreAppendSingleEventAvoidsWholeLogReplacement(t *testing.T) {
	for _, atomic := range []bool{false, true} {
		t.Run(fmt.Sprintf("atomic_%t", atomic), func(t *testing.T) {
			const threadID = "thr_single_append"
			store := NewStore(t.TempDir())
			if err := store.AppendEvent(threadID, map[string]any{
				"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
			}); err != nil {
				t.Fatal(err)
			}
			store.replaceFile = func(string, string) error {
				return errors.New("whole event log replacement was used")
			}
			if err := store.AppendEvents(threadID, []map[string]any{{
				"seq": float64(2), "kind": "item_created", "threadId": threadID, "turnId": "turn_1",
				"item": map[string]any{"id": "item_1", "type": "user_message", "text": "steer now"},
			}}, atomic); err != nil {
				t.Fatalf("append a single durable event: %v", err)
			}
			result, err := store.LoadSince(threadID, 0)
			if err != nil || len(result.Events) != 2 || stringValue(result.Events[1]["kind"]) != "item_created" {
				t.Fatalf("single-event replay mismatch: events=%#v err=%v", result.Events, err)
			}
		})
	}
}

func TestStoreRecoversEveryAtomicSingleEventJournalCrashCut(t *testing.T) {
	tests := []struct {
		cut       string
		committed bool
	}{
		{cut: atomicSingleCutJournalFileSynced},
		{cut: atomicSingleCutJournalCommitted},
		{cut: atomicSingleCutTargetWritten, committed: true},
		{cut: atomicSingleCutTargetSynced, committed: true},
		{cut: atomicSingleCutJournalRemoved, committed: true},
	}
	for _, test := range tests {
		t.Run(test.cut, func(t *testing.T) {
			const threadID = "thr_single_crash"
			root := t.TempDir()
			store := NewStore(root)
			if err := store.AppendEvent(threadID, map[string]any{
				"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
			}); err != nil {
				t.Fatal(err)
			}
			store.atomicAppendCut = func(cut string) error {
				if cut == test.cut {
					return errors.New("simulated process crash")
				}
				return nil
			}
			err := store.AppendEvents(threadID, []map[string]any{{
				"seq": float64(2), "kind": "pipeline_stage", "threadId": threadID,
				"turnId": "turn_1", "stage": "pre_send",
			}}, false)
			if err == nil || AtomicAppendCommitted(err) != test.committed {
				t.Fatalf("crash cut commit classification mismatch: committed=%v err=%v", test.committed, err)
			}

			reopened := NewStore(root)
			result, err := reopened.LoadSince(threadID, 0)
			if err != nil {
				t.Fatalf("restart recovery failed: %v", err)
			}
			want := 1
			if test.committed {
				want = 2
			}
			if len(result.Events) != want {
				t.Fatalf("recovered event frontier mismatch: got=%d want=%d events=%#v", len(result.Events), want, result.Events)
			}
			if names, err := atomicSingleJournalResidueNames(reopened.ThreadDir(threadID)); err != nil || len(names) != 0 {
				t.Fatalf("single-event crash residue survived recovery: names=%v err=%v", names, err)
			}
		})
	}
}

func TestStoreRollsBackTornAtomicSingleEventTail(t *testing.T) {
	const threadID = "thr_single_torn"
	root := t.TempDir()
	store := NewStore(root)
	if err := store.AppendEvent(threadID, map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.EventsPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	store.atomicAppendCut = func(cut string) error {
		if cut == atomicSingleCutJournalCommitted {
			return errors.New("simulated process crash")
		}
		return nil
	}
	if err := store.AppendEvents(threadID, []map[string]any{{
		"seq": float64(2), "kind": "pipeline_stage", "threadId": threadID,
		"turnId": "turn_1", "stage": "pre_send",
	}}, false); err == nil {
		t.Fatal("journal fixture did not stop before target append")
	}
	journalNames, err := atomicSingleJournalResidueNames(store.ThreadDir(threadID))
	if err != nil || len(journalNames) != 1 {
		t.Fatalf("read journal residue name: names=%v err=%v", journalNames, err)
	}
	journalBody, err := os.ReadFile(filepath.Join(store.ThreadDir(threadID), journalNames[0]))
	if err != nil {
		t.Fatal(err)
	}
	var journal atomicSingleJournalV1
	if err := json.Unmarshal(journalBody, &journal); err != nil {
		t.Fatal(err)
	}
	line, err := validateAtomicSingleJournal(journal)
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.OpenFile(store.EventsPath(threadID), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Write(line[:len(line)/2]); err != nil {
		_ = target.Close()
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	result, err := reopened.LoadSince(threadID, 0)
	if err != nil || len(result.Events) != 1 {
		t.Fatalf("torn single-event recovery mismatch: events=%#v err=%v", result.Events, err)
	}
	after, err := os.ReadFile(reopened.EventsPath(threadID))
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("torn single-event tail was not rolled back exactly: err=%v", err)
	}
}

func TestStoreKeepsAtomicSingleJournalUntilRecoveredCommittedTailSyncs(t *testing.T) {
	const threadID = "thr_single_recovery_sync"
	root := t.TempDir()
	store := NewStore(root)
	if err := store.AppendEvent(threadID, map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	store.atomicAppendCut = func(cut string) error {
		if cut == atomicSingleCutTargetWritten {
			return errors.New("simulated process crash before target sync")
		}
		return nil
	}
	err := store.AppendEvents(threadID, []map[string]any{{
		"seq": float64(2), "kind": "pipeline_stage", "threadId": threadID,
		"turnId": "turn_1", "stage": "pre_send",
	}}, false)
	if err == nil || !AtomicAppendCommitted(err) {
		t.Fatalf("target-written crash was not classified as committed: %v", err)
	}

	reopened := NewStore(root)
	syncCalls := 0
	reopened.syncRecoveredFile = func(*os.File) error {
		syncCalls++
		return errors.New("simulated recovery target sync failure")
	}
	if _, err := reopened.LoadSince(threadID, 0); err == nil || !strings.Contains(err.Error(), "recovery target sync failure") {
		t.Fatalf("recovery discarded a journal without syncing its committed target: %v", err)
	}
	if syncCalls != 1 {
		t.Fatalf("recovery target sync calls = %d, want 1", syncCalls)
	}
	if names, err := atomicSingleJournalResidueNames(reopened.ThreadDir(threadID)); err != nil || len(names) != 1 {
		t.Fatalf("failed target sync removed the recovery journal: names=%v err=%v", names, err)
	}

	result, err := NewStore(root).LoadSince(threadID, 0)
	if err != nil || len(result.Events) != 2 {
		t.Fatalf("durable retry did not recover the committed tail: events=%#v err=%v", result.Events, err)
	}
}

func TestStoreRecoversIncompleteContentAddressedSingleEventJournalOnlyAtBoundFrontier(t *testing.T) {
	for _, driftTarget := range []bool{false, true} {
		t.Run(fmt.Sprintf("target_drift_%t", driftTarget), func(t *testing.T) {
			const threadID = "thr_single_incomplete"
			root := t.TempDir()
			store := NewStore(root)
			if err := store.AppendEvent(threadID, map[string]any{
				"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
			}); err != nil {
				t.Fatal(err)
			}
			_, frontier, err := store.LoadSinceWithFrontier(threadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			line, err := json.Marshal(map[string]any{
				"seq": float64(2), "kind": "pipeline_stage", "threadId": threadID,
				"turnId": "turn_1", "stage": "pre_send",
			})
			if err != nil {
				t.Fatal(err)
			}
			journal, err := newAtomicSingleJournal(threadID, frontier, append(line, '\n'))
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(journal)
			if err != nil {
				t.Fatal(err)
			}
			name := atomicSingleJournalFileName(frontier.Size, frontier.SHA256, digestBytes(body))
			path := filepath.Join(store.ThreadDir(threadID), name)
			if err := os.WriteFile(path, body[:len(body)/2], 0o600); err != nil {
				t.Fatal(err)
			}
			if driftTarget {
				target, err := os.OpenFile(store.EventsPath(threadID), os.O_WRONLY|os.O_APPEND, 0)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := target.Write([]byte("x")); err != nil {
					_ = target.Close()
					t.Fatal(err)
				}
				if err := target.Close(); err != nil {
					t.Fatal(err)
				}
			}

			result, err := NewStore(root).LoadSince(threadID, 0)
			if driftTarget {
				if err == nil || !strings.Contains(err.Error(), "does not match its before frontier") {
					t.Fatalf("drifted target accepted incomplete journal: events=%#v err=%v", result.Events, err)
				}
				if _, statErr := os.Stat(path); statErr != nil {
					t.Fatalf("rejected incomplete journal was mutated: %v", statErr)
				}
				return
			}
			if err != nil || len(result.Events) != 1 {
				t.Fatalf("incomplete pre-target journal recovery mismatch: events=%#v err=%v", result.Events, err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovered incomplete journal survived: %v", err)
			}
		})
	}
}

func TestStoreAppendEventsAtomicFailsBeforeReplaceWithoutChangingLog(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent("thr_replace", map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": "thr_replace", "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.EventsPath("thr_replace"))
	if err != nil {
		t.Fatal(err)
	}
	store.replaceFile = func(string, string) error { return errors.New("replace failed") }
	err = store.AppendEventsAtomic("thr_replace", []map[string]any{{
		"seq": float64(2), "kind": "turn_completed", "threadId": "thr_replace", "turnId": "turn_1", "status": "completed",
	}})
	if err == nil || AtomicAppendCommitted(err) {
		t.Fatalf("pre-replace failure was misclassified: %v", err)
	}
	after, readErr := os.ReadFile(store.EventsPath("thr_replace"))
	if readErr != nil || string(after) != string(before) {
		t.Fatalf("pre-replace failure changed the log: after=%q err=%v", after, readErr)
	}
}

func TestStoreAppendEventsAtomicReportsCommittedWhenPostReplaceSyncFails(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent("thr_sync", map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": "thr_sync", "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	syncCalls := 0
	store.syncParentFolder = func(string) error {
		syncCalls++
		if syncCalls == 2 {
			return errors.New("directory sync failed")
		}
		return nil
	}
	err := store.AppendEventsAtomic("thr_sync", []map[string]any{{
		"seq": float64(2), "kind": "turn_completed", "threadId": "thr_sync", "turnId": "turn_1", "status": "completed",
	}})
	if err == nil || !AtomicAppendCommitted(err) || !strings.Contains(err.Error(), "directory sync failed") {
		t.Fatalf("post-replace failure lost its commit state: %v", err)
	}
	result, loadErr := store.LoadSince("thr_sync", 0)
	if loadErr != nil || len(result.Events) != 2 || result.Events[1]["seq"] != float64(2) {
		t.Fatalf("committed replacement was not readable: events=%#v err=%v", result.Events, loadErr)
	}
	if highest, readbackErr := store.HighestSeqAfterCommittedTail("thr_sync", []map[string]any{result.Events[1]}); readbackErr != nil || highest != 2 {
		t.Fatalf("committed replacement tail was not verified: highest=%d err=%v", highest, readbackErr)
	}
}

func TestStoreAppendEventsAtomicDoesNotBlockOnCleanupOnlyDirectorySync(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent("thr_cleanup_sync", map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": "thr_cleanup_sync", "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	syncCalls := 0
	store.syncParentFolder = func(string) error {
		syncCalls++
		return nil
	}
	if err := store.AppendEvent("thr_cleanup_sync", map[string]any{
		"seq": float64(2), "kind": "pipeline_stage", "threadId": "thr_cleanup_sync", "turnId": "turn_1", "stage": "pre_send",
	}); err != nil {
		t.Fatal(err)
	}
	// One sync commits the journal entry and one commits the target rename.
	// Journal deletion is cleanup-only: if its directory entry reappears after
	// a crash, recovery accepts the already-synced exact target and removes it.
	if syncCalls != 2 {
		t.Fatalf("atomic append directory sync calls = %d, want 2", syncCalls)
	}
}

func TestStoreAppendEventsAtomicRejectsTornExistingLog(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.ThreadDir("thr_torn"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := store.EventsPath("thr_torn")
	before := []byte(`{"seq":1,"kind":"thread_created","threadId":"thr_torn"}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEventsAtomic("thr_torn", []map[string]any{{
		"seq": float64(2), "kind": "turn_completed", "threadId": "thr_torn", "turnId": "turn_1", "status": "completed",
	}}); err == nil || !strings.Contains(err.Error(), "newline terminated") {
		t.Fatalf("torn event log should reject atomic append: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("rejected append changed torn log: after=%q err=%v", after, err)
	}
}

func TestStoreAppendEventsAtomicRejectsStaleObservedFrontier(t *testing.T) {
	const threadID = "thr_stale_frontier"
	root := t.TempDir()
	writer := NewStore(root)
	interloper := NewStore(root)
	if err := writer.AppendEvent(threadID, map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	loaded, frontier, err := writer.LoadSinceWithFrontier(threadID, 0)
	if err != nil || len(loaded.Events) != 1 {
		t.Fatalf("observe event frontier: loaded=%#v frontier=%#v err=%v", loaded, frontier, err)
	}
	if err := interloper.AppendEvent(threadID, map[string]any{
		"seq": float64(2), "kind": "pipeline_stage", "threadId": threadID, "turnId": "turn_interloper", "stage": "response_received",
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendEventsAtomicAtFrontier(threadID, []map[string]any{{
		"seq": float64(2), "kind": "turn_completed", "threadId": threadID, "turnId": "turn_writer", "status": "completed",
	}}, frontier); !errors.Is(err, ErrEventLogFrontierChanged) {
		t.Fatalf("stale observed frontier was not rejected: %v", err)
	}
	replay, err := writer.LoadSince(threadID, 0)
	if err != nil || len(replay.Events) != 2 || stringValue(replay.Events[1]["turnId"]) != "turn_interloper" {
		t.Fatalf("stale append changed the winning event log: replay=%#v err=%v", replay, err)
	}
	for _, name := range []string{atomicBundleCandidateName, atomicBundleJournalTemp, atomicBundleJournalName} {
		if _, err := os.Stat(filepath.Join(writer.ThreadDir(threadID), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale frontier rejection left residue %s: %v", name, err)
		}
	}
}

func TestStoreRejectsOrphanedAtomicBundleTemporaryFile(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.ThreadDir("thr_orphan"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.ThreadDir("thr_orphan"), ".events-bundle-crash.tmp"), []byte("candidate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSince("thr_orphan", 0); err == nil || !strings.Contains(err.Error(), "fail-closed recovery") {
		t.Fatalf("orphaned atomic bundle did not block replay: %v", err)
	}
	if err := store.AppendEventsAtomic("thr_orphan", []map[string]any{{
		"seq": float64(1), "kind": "thread_created", "threadId": "thr_orphan", "status": "idle",
	}}); err == nil || !strings.Contains(err.Error(), "fail-closed recovery") {
		t.Fatalf("orphaned atomic bundle did not block append: %v", err)
	}
}

func TestStoreRecoversEveryAtomicBundleJournalCrashCut(t *testing.T) {
	tests := []struct {
		cut       string
		committed bool
	}{
		{cut: atomicBundleCutCandidateSynced},
		{cut: atomicBundleCutJournalTempSynced},
		{cut: atomicBundleCutJournalCommitted},
		{cut: atomicBundleCutTargetReplaced, committed: true},
		{cut: atomicBundleCutDirectorySynced, committed: true},
		{cut: atomicBundleCutJournalRemoved, committed: true},
	}
	for _, test := range tests {
		t.Run(test.cut, func(t *testing.T) {
			root := t.TempDir()
			store := NewStore(root)
			if err := store.AppendEvent("thr_crash", map[string]any{
				"seq": float64(1), "kind": "thread_created", "threadId": "thr_crash", "status": "idle",
			}); err != nil {
				t.Fatal(err)
			}
			bundle := []map[string]any{
				{"seq": float64(2), "kind": "item_completed", "threadId": "thr_crash", "turnId": "turn_1"},
				{"seq": float64(3), "kind": "turn_completed", "threadId": "thr_crash", "turnId": "turn_1", "status": "completed"},
			}
			store.atomicAppendCut = func(cut string) error {
				if cut == test.cut {
					return errors.New("simulated process crash")
				}
				return nil
			}
			err := store.AppendEventsAtomic("thr_crash", bundle)
			if err == nil || AtomicAppendCommitted(err) != test.committed {
				t.Fatalf("crash cut commit classification mismatch: committed=%v err=%v", test.committed, err)
			}

			reopened := NewStore(root)
			result, err := reopened.LoadSince("thr_crash", 0)
			if err != nil {
				t.Fatalf("restart recovery failed: %v", err)
			}
			want := 1
			if test.committed {
				want = 3
			}
			if len(result.Events) != want {
				t.Fatalf("recovered event frontier mismatch: got=%d want=%d events=%#v", len(result.Events), want, result.Events)
			}
			if !test.committed {
				if err := reopened.AppendEventsAtomic("thr_crash", bundle); err != nil {
					t.Fatalf("rolled-back bundle could not be replayed: %v", err)
				}
				result, err = reopened.LoadSince("thr_crash", 0)
				if err != nil || len(result.Events) != 3 {
					t.Fatalf("replayed event bundle mismatch: events=%#v err=%v", result.Events, err)
				}
			}
			for _, name := range []string{atomicBundleCandidateName, atomicBundleJournalTemp, atomicBundleJournalName} {
				if _, err := os.Stat(filepath.Join(reopened.ThreadDir("thr_crash"), name)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("crash residue %s survived recovery: %v", name, err)
				}
			}
		})
	}
}

func TestStoreLoadCannotRecoverLiveAtomicBundleCandidate(t *testing.T) {
	const threadID = "thr_live_candidate"
	root := t.TempDir()
	writer := NewStore(root)
	reader := NewStore(root)
	if err := writer.AppendEvent(threadID, map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}

	candidateSynced := make(chan struct{})
	releaseWriter := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseWriter) }) })
	writer.atomicAppendCut = func(cut string) error {
		if cut == atomicBundleCutCandidateSynced {
			close(candidateSynced)
			<-releaseWriter
		}
		return nil
	}
	bundle := []map[string]any{
		{"seq": float64(2), "kind": "item_completed", "threadId": threadID, "turnId": "turn_1"},
		{"seq": float64(3), "kind": "turn_completed", "threadId": threadID, "turnId": "turn_1", "status": "completed"},
	}
	writerDone := make(chan error, 1)
	go func() { writerDone <- writer.AppendEventsAtomic(threadID, bundle) }()
	select {
	case <-candidateSynced:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not reach the candidate-synced barrier")
	}

	transactionLock := eventlogTransactionMutex(reader.transactionRoot, threadID)
	if transactionLock.TryLock() {
		transactionLock.Unlock()
		t.Fatal("writer did not retain the thread transaction lock at the crash cut")
	}
	candidatePath := filepath.Join(writer.ThreadDir(threadID), atomicBundleCandidateName)
	if _, err := os.Stat(candidatePath); err != nil {
		t.Fatalf("live candidate is missing at the barrier: %v", err)
	}

	type loadResponse struct {
		result LoadResult
		err    error
	}
	readerStarted := make(chan struct{})
	readerDone := make(chan loadResponse, 1)
	go func() {
		close(readerStarted)
		result, err := reader.LoadSince(threadID, 0)
		readerDone <- loadResponse{result: result, err: err}
	}()
	<-readerStarted
	select {
	case response := <-readerDone:
		t.Fatalf("reader crossed a live atomic append transaction: result=%#v err=%v", response.result, response.err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := os.Stat(candidatePath); err != nil {
		t.Fatalf("reader removed the writer's live candidate: %v", err)
	}

	releaseOnce.Do(func() { close(releaseWriter) })
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("writer failed after the reader waited: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish after release")
	}
	select {
	case response := <-readerDone:
		if response.err != nil || len(response.result.Events) != 3 {
			t.Fatalf("reader did not observe the committed whole bundle: events=%#v err=%v", response.result.Events, response.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not resume after the atomic append committed")
	}
	for _, name := range []string{atomicBundleCandidateName, atomicBundleJournalTemp, atomicBundleJournalName} {
		if _, err := os.Stat(filepath.Join(writer.ThreadDir(threadID), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("atomic residue %s survived the completed transaction: %v", name, err)
		}
	}
}

func TestStoreRejectsAtomicBundleJournalWithUnknownTargetFrontier(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if err := store.AppendEvent("thr_frontier", map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": "thr_frontier", "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	store.atomicAppendCut = func(cut string) error {
		if cut == atomicBundleCutJournalCommitted {
			return errors.New("simulated process crash")
		}
		return nil
	}
	if err := store.AppendEventsAtomic("thr_frontier", []map[string]any{{
		"seq": float64(2), "kind": "turn_completed", "threadId": "thr_frontier", "turnId": "turn_1", "status": "completed",
	}}); err == nil {
		t.Fatal("journal fixture did not stop at the crash cut")
	}
	if err := os.WriteFile(store.EventsPath("thr_frontier"), []byte("unrelated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root).LoadSince("thr_frontier", 0); err == nil || !strings.Contains(err.Error(), "neither journal frontier") {
		t.Fatalf("unknown event target frontier was not rejected: %v", err)
	}
}

func TestStoreRejectsDuplicateAtomicBundleJournalKey(t *testing.T) {
	const threadID = "thr_duplicate_journal"
	root := t.TempDir()
	store := NewStore(root)
	if err := store.AppendEvent(threadID, map[string]any{
		"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle",
	}); err != nil {
		t.Fatal(err)
	}
	store.atomicAppendCut = func(cut string) error {
		if cut == atomicBundleCutJournalCommitted {
			return errors.New("simulated process crash")
		}
		return nil
	}
	if err := store.AppendEventsAtomic(threadID, []map[string]any{{
		"seq": float64(2), "kind": "turn_completed", "threadId": threadID, "turnId": "turn_1", "status": "completed",
	}}); err == nil {
		t.Fatal("journal fixture did not stop at the crash cut")
	}
	journalPath := filepath.Join(store.ThreadDir(threadID), atomicBundleJournalName)
	canonical, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte(`"threadId":"` + threadID + `"`)
	if bytes.Count(canonical, needle) != 1 {
		t.Fatalf("canonical journal thread field count is not one: %d", bytes.Count(canonical, needle))
	}
	ambiguous := bytes.Replace(canonical, needle, []byte(
		`"threadId":"forged","threadId":"`+threadID+`"`,
	), 1)
	if err := os.WriteFile(journalPath, ambiguous, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root).LoadSince(threadID, 0); err == nil || !strings.Contains(err.Error(), "journal is corrupt") {
		t.Fatalf("duplicate-key journal was not rejected: %v", err)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil || !bytes.Equal(ambiguous, after) {
		t.Fatalf("rejected duplicate-key journal was mutated: err=%v", err)
	}
}

func TestStoreTypedForkArchiveAndCompact(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Append(domainevent.RuntimeEvent{Seq: 1, Kind: "thread_created", ThreadID: "source", Payload: map[string]any{"status": "idle"}}); err != nil {
		t.Fatalf("append source create: %v", err)
	}
	if err := store.Append(domainevent.RuntimeEvent{Seq: 2, Kind: "turn_completed", ThreadID: "source", TurnID: "turn_1", Terminal: true, Payload: map[string]any{"status": "completed"}}); err != nil {
		t.Fatalf("append source turn: %v", err)
	}
	if err := store.Append(domainevent.RuntimeEvent{Seq: 3, Kind: "usage", ThreadID: "source"}); err != nil {
		t.Fatalf("append source tail: %v", err)
	}
	copied, err := store.Fork("source", "forked", 2)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if copied != 2 {
		t.Fatalf("expected two forked events, got %d", copied)
	}
	if err := store.Archive("forked", 3, "user request"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := store.Compact("forked", 4, "summary"); err != nil {
		t.Fatalf("compact: %v", err)
	}
	events, diagnostics, err := store.ReadAggregate("forked")
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("read forked aggregate diagnostics=%#v err=%v", diagnostics, err)
	}
	if len(events) != 4 || events[0].ThreadID != "forked" || events[0].Payload["sourceThreadId"] != "source" ||
		events[1].Payload["sourceSeq"] != float64(2) || events[2].Kind != "thread_archived" || events[3].Kind != "eventlog_compacted" {
		t.Fatalf("fork/archive/compact events mismatch: %#v", events)
	}
	if highest, err := store.HighestSeq("forked"); err != nil || highest != 4 {
		t.Fatalf("forked highest seq mismatch: highest=%d err=%v", highest, err)
	}
}

func TestStoreSearchProjectsThreadsAndArchiveState(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Append(domainevent.RuntimeEvent{
		Seq:      1,
		Kind:     "thread_created",
		ThreadID: "thr_alpha",
		Payload:  map[string]any{"title": "Alpha research", "status": "idle"},
	}); err != nil {
		t.Fatalf("append alpha create: %v", err)
	}
	if err := store.Append(domainevent.RuntimeEvent{
		Seq:      2,
		Kind:     "item_created",
		ThreadID: "thr_alpha",
		Payload:  map[string]any{"text": "contains revenue waterfall"},
	}); err != nil {
		t.Fatalf("append alpha item: %v", err)
	}
	if err := store.Append(domainevent.RuntimeEvent{
		Seq:      1,
		Kind:     "thread_created",
		ThreadID: "thr_beta",
		Payload:  map[string]any{"title": "Beta archive", "status": "idle"},
	}); err != nil {
		t.Fatalf("append beta create: %v", err)
	}
	if err := store.Archive("thr_beta", 2, "done"); err != nil {
		t.Fatalf("archive beta: %v", err)
	}
	active, diagnostics, err := store.Search("revenue", false)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("active search diagnostics=%#v err=%v", diagnostics, err)
	}
	if len(active) != 1 || active[0].ThreadID != "thr_alpha" || active[0].HighestSeq != 2 || active[0].Match != "item_created" {
		t.Fatalf("active search mismatch: %#v", active)
	}
	archived, diagnostics, err := store.Search("archive", true)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("archived search diagnostics=%#v err=%v", diagnostics, err)
	}
	if len(archived) != 1 || archived[0].ThreadID != "thr_beta" || !archived[0].Archived || archived[0].Status != "archived" {
		t.Fatalf("archived search mismatch: %#v", archived)
	}
	hidden, _, err := store.Search("archive", false)
	if err != nil {
		t.Fatalf("hidden archived search: %v", err)
	}
	if len(hidden) != 0 {
		t.Fatalf("archived thread should be hidden by default: %#v", hidden)
	}
}

func TestStoreSearchFailsClosedForCorruptThreads(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Append(domainevent.RuntimeEvent{
		Seq:      1,
		Kind:     "thread_created",
		ThreadID: "thr_good",
		Payload:  map[string]any{"title": "Good thread", "status": "idle"},
	}); err != nil {
		t.Fatalf("append good: %v", err)
	}
	if err := os.MkdirAll(store.ThreadDir("thr_bad"), 0o700); err != nil {
		t.Fatal(err)
	}
	const sentinel = "PRIVATE_MALFORMED_SEARCH_SENTINEL"
	body := `{"seq":1,"kind":"thread_created","threadId":"thr_bad","title":"Bad thread","status":"idle"}` + "\n" +
		`{"private":"` + sentinel + `"` + "\n"
	if err := os.WriteFile(store.EventsPath("thr_bad"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	results, diagnostics, err := store.Search("", true)
	if err == nil || !strings.Contains(err.Error(), "event log search contains invalid records") ||
		strings.Contains(err.Error(), sentinel) || results != nil || diagnostics != nil {
		t.Fatalf("corrupt inventory returned a partial search result: results=%#v diagnostics=%#v err=%v", results, diagnostics, err)
	}
}

func TestStoreTypedAppendValidation(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Append(domainevent.RuntimeEvent{Seq: 1, Kind: "missing_thread"}); err == nil || !strings.Contains(err.Error(), "threadId") {
		t.Fatalf("expected missing thread validation, got %v", err)
	}
	if err := store.Append(domainevent.RuntimeEvent{Kind: "missing_seq", ThreadID: "thr_1"}); err == nil || !strings.Contains(err.Error(), "seq") {
		t.Fatalf("expected missing seq validation, got %v", err)
	}
}

func TestStoreAppendRawLineUpdatesHighestSeqWhenValid(t *testing.T) {
	store := NewStore(t.TempDir())
	seq, ok, err := store.AppendRawLine("thr_raw", `{"seq":7,"kind":"usage","threadId":"thr_raw"}`)
	if err != nil {
		t.Fatalf("append raw: %v", err)
	}
	if !ok || seq != 7 {
		t.Fatalf("raw line seq mismatch: seq=%d ok=%v", seq, ok)
	}
	before, err := os.ReadFile(store.EventsPath("thr_raw"))
	if err != nil {
		t.Fatal(err)
	}
	seq, ok, err = store.AppendRawLine("thr_raw", `{not-json}`)
	if err == nil || ok || seq != 0 {
		t.Fatalf("corrupt raw line must fail before append: seq=%d ok=%v err=%v", seq, ok, err)
	}
	after, readErr := os.ReadFile(store.EventsPath("thr_raw"))
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("rejected corrupt raw changed the event log: err=%v after=%q", readErr, after)
	}
	result, err := store.LoadSince("thr_raw", 0)
	if err != nil {
		t.Fatalf("load raw: %v", err)
	}
	if len(result.Events) != 1 || len(result.Diagnostics) != 0 {
		t.Fatalf("rejected corrupt raw must not create replay diagnostics: %#v", result)
	}
}

func TestStoreAppendRawLineRejectsReasoningMetadataSmugglingBeforeFilesystemEffects(t *testing.T) {
	store := NewStore(t.TempDir())
	threadID := "thr_reasoning_smuggle"
	line := `{"seq":1,"kind":"usage","threadId":"thr_reasoning_smuggle","usage":{"reasoningTokens":"PRIVATE_REASONING_SENTINEL"}}`

	seq, ok, err := store.AppendRawLine(threadID, line)
	if err == nil || ok || seq != 0 {
		t.Fatalf("reasoning metadata smuggling must fail closed: seq=%d ok=%v err=%v", seq, ok, err)
	}
	if _, statErr := os.Stat(store.ThreadDir(threadID)); !os.IsNotExist(statErr) {
		t.Fatalf("rejected raw line created durable state: %v", statErr)
	}
}

func TestStoreRejectsMalformedAndMissingSequenceWithoutPartialReplay(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.ThreadDir("thr_bad"), 0o700); err != nil {
		t.Fatal(err)
	}
	lines := strings.Join([]string{
		`{"seq":3,"kind":"ok","threadId":"thr_bad"}`,
		`{"kind":"missing_seq","threadId":"thr_bad"}`,
		`{bad-json}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(store.ThreadDir("thr_bad"), "events.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := store.LoadSince("thr_bad", 0)
	if err == nil || result.Events != nil || result.Diagnostics != nil {
		t.Fatalf("structurally invalid log returned a partial replay: result=%#v err=%v", result, err)
	}
}

func TestStoreReplayPreservesExactPayloadNumberAndRejectsInexactSequence(t *testing.T) {
	store := NewStore(t.TempDir())
	threadID := "thr_exact_replay_number"
	if err := os.MkdirAll(store.ThreadDir(threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	line := `{"seq":1,"kind":"usage","threadId":"thr_exact_replay_number","amount":9007199254740993}` + "\n"
	if err := os.WriteFile(store.EventsPath(threadID), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := store.LoadSince(threadID, 0)
	if err != nil {
		t.Fatalf("load exact replay fixture: %v", err)
	}
	if len(result.Events) != 1 || result.Events[0]["seq"] != float64(1) ||
		result.Events[0]["amount"] != json.Number("9007199254740993") {
		t.Fatalf("exact payload number was lost: %#v", result.Events)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("exact replay produced diagnostics: %#v", result.Diagnostics)
	}

	invalidThreadID := "thr_inexact_replay_sequence"
	if err := os.MkdirAll(store.ThreadDir(invalidThreadID), 0o700); err != nil {
		t.Fatal(err)
	}
	invalid := `{"seq":9007199254740993,"kind":"usage","threadId":"thr_inexact_replay_sequence"}` + "\n"
	if err := os.WriteFile(store.EventsPath(invalidThreadID), []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidResult, invalidErr := store.LoadSince(invalidThreadID, 0)
	if invalidErr == nil || invalidResult.Events != nil || invalidResult.Diagnostics != nil {
		t.Fatalf("inexact sequence returned a partial replay: result=%#v err=%v", invalidResult, invalidErr)
	}
}
