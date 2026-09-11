package eventlog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEventObservationPreservesEveryPendingAtomicResidue(t *testing.T) {
	for _, residue := range []string{"", atomicBundleCandidateName, atomicBundleJournalName, atomicBundleJournalTemp, atomicSingleJournalPrefix + "unresolved.json", ".events-bundle-unknown"} {
		t.Run(residue, func(t *testing.T) {
			store := NewStore(t.TempDir())
			threadID := "thread-observation"
			if err := store.AppendEvent(threadID, map[string]any{"seq": float64(1), "kind": "thread_created", "threadId": threadID, "status": "idle"}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.EventsPath(threadID))
			if err != nil {
				t.Fatal(err)
			}
			retained := []byte("SYNTHETIC_PENDING_EVENT_TRANSACTION")
			if residue != "" {
				if err := os.WriteFile(filepath.Join(store.ThreadDir(threadID), residue), retained, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			result, frontier, err := store.ObserveSinceWithFrontier(context.Background(), threadID, 0)
			if residue == "" {
				if err != nil || len(result.Events) != 1 || len(result.Diagnostics) != 0 || frontier.SHA256 != digestBytes(before) {
					t.Fatalf("healthy observation: %v", err)
				}
			} else {
				if err == nil || result.Events != nil {
					t.Fatal("unsettled transaction supplied publication proof")
				}
				after, err := os.ReadFile(filepath.Join(store.ThreadDir(threadID), residue))
				if err != nil || !bytes.Equal(after, retained) {
					t.Fatal("observation modified pending transaction")
				}
			}
			after, err := os.ReadFile(store.EventsPath(threadID))
			if err != nil || !bytes.Equal(after, before) {
				t.Fatal("observation modified committed log")
			}
		})
	}
}
