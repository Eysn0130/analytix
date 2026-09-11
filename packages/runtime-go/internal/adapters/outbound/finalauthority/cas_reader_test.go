package finalauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrimaryThreadSnapshotRequiresOriginalStrictBytes(t *testing.T) {
	for _, fault := range []string{"healthy", "duplicate JSON key", "null turns", "non-object turn", "duplicate turn", "thread alias", "turn alias", "foreign turn", "sidecar-only", "root replacement", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "durable")
			threadID, turnID := "thread-primary", "turn-primary"
			writeAcceptedFinalCASTestThread(t, root, threadID, turnID, "completed")
			path := filepath.Join(root, "threads", threadID, "thread.json")
			reader, err := NewAcceptedFinalCASReader(root)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(body, &record); err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "null turns":
				record["turns"] = nil
			case "non-object turn":
				record["turns"] = append(record["turns"].([]any), true)
			case "duplicate turn":
				record["turns"] = append(record["turns"].([]any), record["turns"].([]any)[0])
			case "thread alias":
				record["id"] = " " + threadID
			case "turn alias":
				record["turns"].([]any)[0].(map[string]any)["id"] = " " + turnID
			case "foreign turn":
				record["turns"].([]any)[0].(map[string]any)["threadId"] = "thread-foreign"
			}
			body, err = json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if fault == "duplicate JSON key" {
				body = append([]byte(`{"id":"thread-shadow",`), body[1:]...)
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if fault == "sidecar-only" {
				if err := os.Rename(path, filepath.Join(filepath.Dir(path), "metadata.json")); err != nil {
					t.Fatal(err)
				}
			}
			if fault == "root replacement" {
				if err := os.Rename(root, root+"-retained"); err != nil {
					t.Fatal(err)
				}
				writeAcceptedFinalCASTestThread(t, root, threadID, turnID, "completed")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if fault == "cancelled" {
				cancel()
			}
			snapshot, err := reader.ReadPrimaryThreadSnapshotV1(ctx, threadID)
			if fault == "healthy" {
				observedBody, marshalErr := json.Marshal(snapshot.Thread)
				if err != nil || marshalErr != nil || snapshot.ThreadID != threadID || snapshot.ThreadFileSHA256 != domainsecurity.SHA256Hex(body) || !bytes.Equal(observedBody, body) {
					t.Fatalf("exact primary observation failed: %v", err)
				}
			} else if err == nil || snapshot.Thread != nil {
				t.Fatal("untrusted primary produced a snapshot")
			}
			if fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation identity lost: %v", err)
			}
		})
	}
}

func TestCoreEventSnapshotUsesAnchoredSingleLinkAuthority(t *testing.T) {
	for _, fault := range []string{"healthy", "symlink", "hardlink", "root replacement"} {
		t.Run(fault, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "durable")
			threadID := "thread-event-snapshot"
			writeAcceptedFinalCASTestThread(t, root, threadID, "turn-event-snapshot", "completed")
			reader, err := NewAcceptedFinalCASReader(root)
			if err != nil {
				t.Fatal(err)
			}
			body := []byte("SYNTHETIC_EVENT_BYTES\n")
			path := filepath.Join(root, "threads", threadID, "events.jsonl")
			if fault == "symlink" || fault == "hardlink" {
				target := filepath.Join(filepath.Dir(root), "retained-event-bytes")
				if err := os.WriteFile(target, body, 0o600); err != nil {
					t.Fatal(err)
				}
				link := os.Link
				if fault == "symlink" {
					link = os.Symlink
				}
				if err := link(target, path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if fault == "root replacement" {
				if err := os.Rename(root, root+"-retained"); err != nil {
					t.Fatal(err)
				}
				writeAcceptedFinalCASTestThread(t, root, threadID, "turn-event-snapshot", "completed")
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			digest, err := reader.ReadCommittedEventLogSHA256V1(context.Background(), threadID)
			if fault == "healthy" {
				if err != nil || digest != domainsecurity.SHA256Hex(body) {
					t.Fatalf("exact event digest: %v", err)
				}
			} else if err == nil || digest != "" {
				t.Fatal("unanchored event bytes supplied proof")
			}
		})
	}
}

func TestAcceptedFinalCASReaderPinsDurableRootAndThreadsAuthority(t *testing.T) {
	for _, target := range []string{"durable-root", "threads-root"} {
		t.Run(target, func(t *testing.T) {
			base := t.TempDir()
			durableRoot := filepath.Join(base, "durable")
			threadID := "thr_cas_authority"
			turnID := "turn_cas_authority"
			writeAcceptedFinalCASTestThread(t, durableRoot, threadID, turnID, "running")
			reader, err := NewAcceptedFinalCASReader(durableRoot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadAcceptedFinalCASObservation(context.Background(), threadID, turnID); err != nil {
				t.Fatalf("initial CAS read failed: %v", err)
			}

			switch target {
			case "durable-root":
				if err := os.Rename(durableRoot, durableRoot+"-original"); err != nil {
					t.Fatal(err)
				}
				writeAcceptedFinalCASTestThread(t, durableRoot, threadID, turnID, "completed")
			case "threads-root":
				threads := filepath.Join(durableRoot, "threads")
				if err := os.Rename(threads, threads+"-original"); err != nil {
					t.Fatal(err)
				}
				writeAcceptedFinalCASTestThread(t, durableRoot, threadID, turnID, "completed")
			}
			if _, err := reader.ReadAcceptedFinalCASObservation(context.Background(), threadID, turnID); err == nil {
				t.Fatalf("CAS reader followed a replaced %s", target)
			}
		})
	}
}

func TestAcceptedFinalCASBatchReaderUsesOneExactPrimarySnapshot(t *testing.T) {
	base := t.TempDir()
	durableRoot := filepath.Join(base, "durable")
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	threadID := "thr_cas_batch"
	contextFor := func(turnID string, issuedAt int64) domainsecurity.TurnSecurityContext {
		return domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-cas-batch",
			CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-cas-batch",
			SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: time.Unix(issuedAt, 0),
		})
	}
	first := contextFor("turn_cas_batch_a", 1)
	second := contextFor("turn_cas_batch_b", 2)
	turns := []any{
		map[string]any{"id": first.TurnID, "status": "completed", "securityContext": first, "items": []any{}},
		map[string]any{"id": second.TurnID, "status": "running", "securityContext": second, "items": []any{}},
	}
	write := func(turns []any) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"id": threadID, "securityState": second, "turns": turns})
		if err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(durableRoot, "threads", threadID)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "thread.json"), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(turns)
	reader, err := NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := reader.ReadAcceptedFinalCASObservations(context.Background(), threadID, []string{first.TurnID, second.TurnID})
	if err != nil || len(observations) != 2 ||
		observations[first.TurnID].ThreadFileSHA256 != observations[second.TurnID].ThreadFileSHA256 ||
		!reflect.DeepEqual(observations[first.TurnID].CurrentContext, second) ||
		!reflect.DeepEqual(observations[second.TurnID].CurrentContext, second) {
		t.Fatalf("batch CAS snapshot mismatch: observations=%#v err=%v", observations, err)
	}
	if _, err := reader.ReadAcceptedFinalCASObservations(context.Background(), threadID, []string{first.TurnID, first.TurnID}); err == nil {
		t.Fatal("duplicate requested CAS turn was accepted")
	}
	if _, err := reader.ReadAcceptedFinalCASObservations(context.Background(), threadID, []string{"turn_missing"}); err == nil {
		t.Fatal("missing requested CAS turn was accepted")
	}
	write(append(turns, map[string]any{"id": first.TurnID, "status": "completed", "securityContext": first, "items": []any{}}))
	if _, err := reader.ReadAcceptedFinalCASObservations(context.Background(), threadID, []string{second.TurnID}); err == nil {
		t.Fatal("duplicate primary turn outside the requested set was ignored")
	}
}

func writeAcceptedFinalCASTestThread(t *testing.T, durableRoot, threadID, turnID, status string) {
	t.Helper()
	workspace := filepath.Join(filepath.Dir(durableRoot), "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-cas-authority",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: "snapshot-cas-authority",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: time.Unix(1, 0),
	})
	thread := map[string]any{
		"id": threadID, "securityState": securityContext,
		"turns": []any{map[string]any{
			"id": turnID, "status": status, "securityContext": securityContext, "items": []any{},
		}},
	}
	body, err := json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(durableRoot, "threads", threadID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "thread.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}
