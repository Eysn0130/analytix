package server

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestChildIdentityFloorsReachAllThreadAllocatorsBeforeFirstWrite(t *testing.T) {
	root, workspace := t.TempDir(), workspacetest.New(t)
	floors := domainpendingwork.ChildIdentityFloorsV1{ThreadSequence: 700, ForkSequence: 900, ResumeSequence: 1100}
	store, err := NewTempDurableEventSessionStore(root, floors)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "synthetic"}, workspace)
	if err != nil || stringField(thread, "id") != "thr_durable_701" {
		t.Fatalf("first thread ignored floor: %v", err)
	}
	fork, err := store.ForkThread("thr_durable_701", map[string]any{})
	if err != nil || stringField(fork, "id") != "thr_durable_fork_901" {
		t.Fatalf("first fork ignored floor: %v", err)
	}
	resumed, err := store.ResumeSession("thr_durable_701", map[string]any{})
	if err != nil || stringField(resumed, "thread_id") != "thr_durable_resume_1101" {
		t.Fatalf("first resume ignored floor: %v", err)
	}
	// A missing meta file cannot lower the already consumed process floor.
	if err := os.Remove(store.metaPath()); err != nil {
		t.Fatal(err)
	}
	resumed, err = store.ResumeSession("thr_durable_701", map[string]any{})
	if err != nil || stringField(resumed, "thread_id") != "thr_durable_resume_1102" {
		t.Fatalf("resume reused a process identity: %v", err)
	}
	store.childResumeCounterFloor = int(^uint(0) >> 1)
	before := childThreadReservationTree(t, root)
	if _, err := store.ResumeSession("thr_durable_701", map[string]any{}); err == nil {
		t.Fatal("exhausted resume counter wrapped")
	}
	if !reflect.DeepEqual(before, childThreadReservationTree(t, root)) {
		t.Fatal("exhausted resume changed durable state")
	}
}

func TestInvalidChildIdentityFloorsRejectBeforeDurableConstructorWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	if _, err := NewTempDurableEventSessionStore(root, domainpendingwork.ChildIdentityFloorsV1{TurnSequence: -1}); err == nil {
		t.Fatal("invalid floor was accepted")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("invalid floor created durable root")
	}
}

func TestChildTurnFloorSurvivesRuntimeRestoreAndReservation(t *testing.T) {
	h := newCompatibilityRuntimeServerHandler(RuntimeServerConfig{DurableTempDir: t.TempDir(), DataDir: t.TempDir()}).(*runtimeServerHandler)
	t.Cleanup(func() {
		if err := h.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	h.turnSeq = 900
	if err := h.restoreRuntimeState(); err != nil {
		t.Fatal(err)
	}
	reserved, err := h.reserveRuntimeChildTurnV1(context.Background(), "thr_durable_1")
	if err != nil || reserved.turnID != "turn_901" {
		t.Fatalf("restored floor did not reach first child reservation: %v", err)
	}
}
