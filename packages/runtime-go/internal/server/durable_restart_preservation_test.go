package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
)

func durableRestartPreservationFixtureV1(t *testing.T, legacy bool) (string, string, string, map[string]any, *eventlog.SemanticRestartPreservationV1) {
	t.Helper()
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	held, err := store.CreateThread(map[string]any{"title": "Held original"}, "")
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.CreateThread(map[string]any{"title": "Independent"}, "")
	if err != nil {
		t.Fatal(err)
	}
	id := stringField(held, "id")
	if legacy {
		destination := filepath.Join(root, "runtime-go", "threads", id)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(store.threadDir(id), destination); err != nil {
			t.Fatal(err)
		}
	}
	preserved, err := eventlog.PrepareSemanticRestartPreservationV1(context.Background(), root, store.threadSummaryIndex.Path(), []string{id})
	if err != nil {
		t.Fatal(err)
	}
	return root, id, stringField(active, "id"), held, preserved
}

func TestDurableRestartPreservationDeniesBeforePrimarySidecarsAndAuthorizer(t *testing.T) {
	for _, operation := range []string{"upsert", "if_absent", "authorized", "metadata", "messages", "patch", "delete", "fork", "resume", "record", "raw"} {
		t.Run(operation, func(t *testing.T) {
			root, id, _, original, preserved := durableRestartPreservationFixtureV1(t, false)
			store, err := newDurableEventSessionStoreWithPreservationV1(root, durableStoreModeTemp, false, preserved)
			if err != nil {
				t.Fatal(err)
			}
			before := runtimeRestoreFileDigestsV1(t, root)
			changed := cloneMap(original)
			changed["title"] = "REPLACEMENT"
			changed["turns"] = []any{map[string]any{"id": "turn-1", "threadId": id, "status": "completed", "items": []any{map[string]any{"id": "message-1", "threadId": id, "turnId": "turn-1", "kind": "user_message", "role": "user", "text": "SYNTHETIC"}}}}
			calls := 0
			store.beforeRecordEventHook = func(map[string]any) error { calls++; return nil }
			store.beforePersistEventHook = func() { calls++ }
			switch operation {
			case "upsert":
				store.mu.Lock()
				err = store.upsertThreadNoLock(changed, true)
				store.mu.Unlock()
			case "if_absent":
				store.mu.Lock()
				err = store.upsertThreadIfAbsentNoLock(changed, true)
				store.mu.Unlock()
			case "authorized":
				store.mu.Lock()
				err = store.upsertThreadNoLockWithAtomicWriteAuthority(changed, true, func(write func() error) error { calls++; return write() })
				store.mu.Unlock()
			case "metadata":
				store.mu.Lock()
				err = store.appendThreadMetadataSidecarNoLock(changed)
				store.mu.Unlock()
			case "messages":
				store.mu.Lock()
				err = store.appendMissingThreadMessageSidecarsNoLock(changed)
				store.mu.Unlock()
			case "patch":
				_, err = store.PatchThread(id, map[string]any{"title": "PATCHED"})
			case "delete":
				_, err = store.DeleteThread(id)
			case "fork":
				_, err = store.ForkThread(id, nil)
			case "resume":
				_, err = store.ResumeSession(id, nil)
			case "record":
				_, _, err = store.RecordEvent(map[string]any{"kind": "tool_call_ready", "threadId": id, "turnId": "turn-1"})
			case "raw":
				err = store.AppendRawEventLine(id, `{"kind":"tool_call_ready","seq":1,"threadId":"`+id+`","turnId":"turn-1"}`)
			}
			if !errors.Is(err, casethreadapp.ErrRestartPreserved) {
				t.Errorf("durable owner did not deny at its boundary: %v", err)
			}
			if calls != 0 || !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, root)) {
				t.Fatal("held request reached authorizer/hook or changed durable state")
			}
		})
	}
}

func TestDurableRestartPreservationProtectsLegacyBeforeConstructorImport(t *testing.T) {
	root, id, active, _, preserved := durableRestartPreservationFixtureV1(t, true)
	legacyHeld := filepath.Join(root, "runtime-go", "threads", id)
	before := runtimeRestoreFileDigestsV1(t, legacyHeld)
	// An independent legacy directory must still migrate in this same startup.
	legacyActive := filepath.Join(root, "runtime-go", "threads", active)
	if err := os.Rename(filepath.Join(root, "threads", active), legacyActive); err != nil {
		t.Fatal(err)
	}
	store, err := newDurableEventSessionStoreWithPreservationV1(root, durableStoreModeSemanticStage, true, preserved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, legacyHeld)) {
		t.Fatal("constructor changed held legacy source")
	}
	if _, err := os.Lstat(filepath.Join(root, "threads", id)); !os.IsNotExist(err) {
		t.Fatal("constructor promoted held legacy primary")
	}
	if _, err := os.Stat(filepath.Join(root, "threads", active, "thread.json")); err != nil {
		t.Fatal("independent legacy primary was not imported")
	}
	if err := store.ApplySemanticStartupMigrationsAfterAuthorityRepair(); err != nil {
		t.Fatal(err)
	}
	if err := preserved.Revalidate(context.Background(), root); err != nil {
		t.Fatal(err)
	}
}

func TestDurableRestartPreservationKeepsIndependentMutationAndRejectsStaleConstructor(t *testing.T) {
	root, _, active, _, preserved := durableRestartPreservationFixtureV1(t, false)
	store, err := newDurableEventSessionStoreWithPreservationV1(root, durableStoreModeTemp, false, preserved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PatchThread(active, map[string]any{"title": "UPDATED"}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureThreadSummaryIndex(); err != nil {
		t.Fatal(err)
	}
	if err := preserved.Revalidate(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "absent")
	if _, err := newDurableEventSessionStoreWithPreservationV1(foreign, durableStoreModeTemp, false, preserved); err == nil {
		t.Fatal("foreign constructor scope accepted")
	}
	if _, err := os.Lstat(foreign); !os.IsNotExist(err) {
		t.Fatal("foreign constructor created root")
	}
}
