package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func childThreadReservationTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf("/%x", sha256.Sum256(body))
		}
		snapshot[relative] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestChildThreadReservationsArePureAndShareCreateForkCounters(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateThread(map[string]any{"title": "synthetic source"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	parent := stringField(source, "id")
	before := childThreadReservationTree(t, root)
	createOne, err := store.reserveChildThreadV1(context.Background(), parent, "")
	if err != nil {
		t.Fatal(err)
	}
	createTwo, err := store.reserveChildThreadV1(context.Background(), parent, "")
	if err != nil {
		t.Fatal(err)
	}
	forkOne, err := store.reserveChildThreadV1(context.Background(), parent, parent)
	if err != nil {
		t.Fatal(err)
	}
	forkTwo, err := store.reserveChildThreadV1(context.Background(), parent, parent)
	if err != nil {
		t.Fatal(err)
	}
	if createOne.threadID != "thr_durable_2" || createTwo.threadID != "thr_durable_3" || forkOne.threadID != "thr_durable_fork_1" || forkTwo.threadID != "thr_durable_fork_2" {
		t.Fatal("child allocation changed existing ID families or reused a counter")
	}
	if !reflect.DeepEqual(before, childThreadReservationTree(t, root)) {
		t.Fatal("thread reservation wrote durable state")
	}
	ordinary, err := store.CreateThread(map[string]any{"title": "independent"}, workspace)
	if err != nil || stringField(ordinary, "id") != "thr_durable_4" {
		t.Fatal("ordinary create reused a reserved identity")
	}
	ordinaryFork, err := store.ForkThread(parent, map[string]any{})
	if err != nil || stringField(ordinaryFork, "id") != "thr_durable_fork_3" {
		t.Fatalf("ordinary fork reused a reserved identity: %v", err)
	}
	patch := map[string]any{"parentThreadId": parent, "relation": "side", "title": "synthetic child"}
	for _, reservation := range []*childThreadReservationV1{createTwo, createOne} {
		created, err := store.createReservedChildThreadV1(context.Background(), reservation, patch, workspace)
		if err != nil || stringField(created, "id") != reservation.threadID {
			t.Fatalf("reserved create identity changed: %v", err)
		}
	}
	for _, reservation := range []*childThreadReservationV1{forkTwo, forkOne} {
		forked, err := store.forkReservedChildThreadV1(context.Background(), reservation, parent, patch)
		if err != nil || stringField(forked, "id") != reservation.threadID {
			t.Fatalf("reserved fork identity changed: %v", err)
		}
	}
	meta, err := store.readMetaNoLock()
	if err != nil || meta.ThreadCounter != 4 || meta.ForkCounter != 3 {
		t.Fatal("reverse consumption lowered the durable counter floor")
	}
	committed := childThreadReservationTree(t, root)
	if _, err := store.createReservedChildThreadV1(context.Background(), createOne, patch, workspace); err == nil {
		t.Fatal("create reservation replay wrote another thread")
	}
	if _, err := store.forkReservedChildThreadV1(context.Background(), forkOne, parent, patch); err == nil {
		t.Fatal("fork reservation replay wrote another thread")
	}
	if !reflect.DeepEqual(committed, childThreadReservationTree(t, root)) {
		t.Fatal("consumed reservation replay changed durable bytes")
	}
}

func TestChildThreadReservationRejectsForeignModeParentSourceAndOccupiedTarget(t *testing.T) {
	root, workspace := t.TempDir(), t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateThread(map[string]any{"title": "synthetic source"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	parent := stringField(source, "id")
	reservation, err := store.reserveChildThreadV1(context.Background(), parent, "")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.reserveChildThreadV1(context.Background(), parent, parent)
	if err != nil {
		t.Fatal(err)
	}
	before := childThreadReservationTree(t, root)
	patch := map[string]any{"parentThreadId": parent, "relation": "side"}
	wrongParent := map[string]any{"parentThreadId": "thr_durable_99", "relation": "side"}
	if _, err := store.createReservedChildThreadV1(context.Background(), reservation, wrongParent, workspace); err == nil {
		t.Fatal("caller parent replaced reservation parent")
	}
	if _, err := store.createReservedChildThreadV1(context.Background(), fork, patch, workspace); err == nil {
		t.Fatal("fork reservation became create authority")
	}
	if _, err := store.forkReservedChildThreadV1(context.Background(), fork, "thr_durable_99", patch); err == nil {
		t.Fatal("caller source replaced frozen fork source")
	}
	foreign, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.createReservedChildThreadV1(context.Background(), reservation, patch, workspace); err == nil {
		t.Fatal("reservation crossed its store owner")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.createReservedChildThreadV1(cancelled, reservation, patch, workspace); err == nil {
		t.Fatal("cancelled child creation reached persistence")
	}
	if !reflect.DeepEqual(before, childThreadReservationTree(t, root)) {
		t.Fatal("invalid reservation changed durable state")
	}
	if err := os.Mkdir(store.threadDir(reservation.threadID), 0o700); err != nil {
		t.Fatal(err)
	}
	occupied := childThreadReservationTree(t, root)
	if err := store.revalidateChildThreadReservationV1(context.Background(), reservation); err == nil {
		t.Fatal("occupied target remained signable")
	}
	if _, err := store.createReservedChildThreadV1(context.Background(), reservation, patch, workspace); err == nil {
		t.Fatal("reserved create overwrote occupied target")
	}
	if !reflect.DeepEqual(occupied, childThreadReservationTree(t, root)) {
		t.Fatal("occupied target rejection changed durable state")
	}
}

func TestChildThreadReservationRejectsCounterOverflowWithoutWrites(t *testing.T) {
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.childThreadCounterFloor, store.childForkCounterFloor = int(^uint(0)>>1), int(^uint(0)>>1)
	before := childThreadReservationTree(t, root)
	for _, source := range []string{"", "thr_durable_1"} {
		if _, err := store.reserveChildThreadV1(context.Background(), "thr_durable_1", source); err == nil {
			t.Fatal("thread reservation wrapped an exhausted counter")
		}
	}
	if _, err := store.CreateThread(map[string]any{}, t.TempDir()); err == nil {
		t.Fatal("ordinary create wrapped a reserved counter")
	}
	if !reflect.DeepEqual(before, childThreadReservationTree(t, root)) {
		t.Fatal("exhausted thread allocator wrote persistence")
	}
}

func TestCopiedChildThreadReservationCannotRetryFailedConsumption(t *testing.T) {
	root := t.TempDir()
	store, err := NewTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateThread(map[string]any{"title": "synthetic source"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := stringField(source, "id")
	reservation, err := store.reserveChildThreadV1(context.Background(), parent, parent)
	if err != nil {
		t.Fatal(err)
	}
	copied := *reservation
	before := childThreadReservationTree(t, root)
	invalid := map[string]any{"parentThreadId": parent, "relation": "side", "turnId": "missing-turn"}
	if _, err := store.forkReservedChildThreadV1(context.Background(), reservation, parent, invalid); err == nil {
		t.Fatal("invalid fork unexpectedly succeeded")
	}
	valid := map[string]any{"parentThreadId": parent, "relation": "side"}
	if _, err := store.forkReservedChildThreadV1(context.Background(), &copied, parent, valid); err == nil {
		t.Error("copied reservation retried a failed consumption")
	}
	if !reflect.DeepEqual(before, childThreadReservationTree(t, root)) {
		t.Error("copied failed reservation wrote durable state")
	}
}
