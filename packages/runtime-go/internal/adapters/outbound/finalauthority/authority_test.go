package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func newPrivateStoreForTest(t *testing.T, root string) (*PrivateStore, error) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewPrivateStore(root, access)
}

func recoverPrivateStoreForTest(t *testing.T, root string) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	plans := make([]*PreparedSecurePrivateCASRecoveryV1, 0, 2)
	for _, leaf := range []string{"records", "dispositions"} {
		prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(
			context.Background(), filepath.Join(root, leaf), maxPrivateAcceptedFinalBytes, access,
		)
		if err != nil {
			t.Fatalf("prepare private store %s recovery: %v", leaf, err)
		}
		plans = append(plans, prepared)
	}
	for _, prepared := range plans {
		if err := prepared.Revalidate(context.Background()); err != nil {
			t.Fatalf("revalidate private store recovery: %v", err)
		}
	}
	for _, prepared := range plans {
		if err := prepared.Apply(context.Background()); err != nil {
			t.Fatalf("apply private store recovery: %v", err)
		}
	}
}

func TestFileAuthorityIdentityIsStableAcrossRestart(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "authority", "final-answer-ed25519-v1.json")
	first, err := OpenOrCreateFileAuthority(path, false)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("sealed-final")
	signature, err := first.Sign(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenOrCreateFileAuthority(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if first.KeyID() != second.KeyID() || string(first.PublicKey()) != string(second.PublicKey()) {
		t.Fatal("file authority identity changed across restart")
	}
	if err := second.VerifyTrusted(context.Background(), first.KeyID(), first.PublicKey(), message, signature); err != nil {
		t.Fatalf("restarted authority rejected prior signature: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("authority key permissions mismatch: info=%#v err=%v", info, err)
	}
}

func TestFileAuthorityFailsClosedForMissingCorruptBroadOrSymlinkKey(t *testing.T) {
	t.Run("missing with records", func(t *testing.T) {
		if _, err := OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority", "missing.json"), true); err == nil {
			t.Fatal("missing authority key was regenerated while records exist")
		}
	})
	t.Run("corrupt", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "authority", "key.json")
		if _, err := OpenOrCreateFileAuthority(path, false); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenOrCreateFileAuthority(path, true); err == nil {
			t.Fatal("corrupt authority key was accepted")
		}
	})
	if runtime.GOOS != "windows" {
		t.Run("broad permissions", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authority", "key.json")
			if _, err := OpenOrCreateFileAuthority(path, false); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenOrCreateFileAuthority(path, true); err == nil {
				t.Fatal("over-permissive authority key was accepted")
			}
		})
		t.Run("symlink", func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "authority", "target.json")
			if _, err := OpenOrCreateFileAuthority(target, false); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(root, "authority", "linked.json")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenOrCreateFileAuthority(link, true); err == nil {
				t.Fatal("symlink authority key was accepted")
			}
		})
	}
}

func TestPrivateAcceptedFinalStoreIsImmutableIdempotentAndStrict(t *testing.T) {
	root := t.TempDir()
	authority, err := OpenOrCreateFileAuthority(filepath.Join(root, "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	record := finalAuthorityRecordFixture(t, authority)
	store, err := newPrivateStoreForTest(t, filepath.Join(root, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatalf("idempotent private write failed: %v", err)
	}
	resolved, err := store.Resolve(context.Background(), record.AcceptedFinal.RecordDigest)
	if err != nil || resolved.StoreDigest != record.StoreDigest {
		t.Fatalf("private final readback mismatch: record=%#v err=%v", resolved, err)
	}
	records, err := store.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("private final listing mismatch: len=%d err=%v", len(records), err)
	}
	visitedRecords := 0
	if err := store.VisitAcceptedFinals(context.Background(), func(current domainevidence.PrivateAcceptedFinalRecord) error {
		visitedRecords++
		if current.StoreDigest != record.StoreDigest {
			return errors.New("visited private final changed")
		}
		return nil
	}); err != nil || visitedRecords != 1 {
		t.Fatalf("private final visit mismatch: count=%d err=%v", visitedRecords, err)
	}
	if hasRecords, err := store.HasRecords(context.Background()); err != nil || !hasRecords {
		t.Fatalf("private final state was not detected: has=%t err=%v", hasRecords, err)
	}
	manifestDigest := domainsecurity.SHA256Hex([]byte("publication-manifest"))
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: record.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: manifestDigest, DecidedAt: time.Unix(3, 0),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatalf("idempotent disposition write failed: %v", err)
	}
	resolvedDisposition, err := store.ResolveDisposition(context.Background(), record.AcceptedFinal.RecordDigest)
	if err != nil || resolvedDisposition.RecordDigest != disposition.RecordDigest {
		t.Fatalf("disposition readback mismatch: record=%#v err=%v", resolvedDisposition, err)
	}
	dispositions, err := store.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 {
		t.Fatalf("disposition listing mismatch: len=%d err=%v", len(dispositions), err)
	}
	visitedDispositions := 0
	if err := store.VisitDispositions(context.Background(), func(current domainevidence.AcceptedFinalDispositionRecord) error {
		visitedDispositions++
		if current.RecordDigest != disposition.RecordDigest {
			return errors.New("visited disposition changed")
		}
		return nil
	}); err != nil || visitedDispositions != 1 {
		t.Fatalf("disposition visit mismatch: count=%d err=%v", visitedDispositions, err)
	}
	if err := store.VisitAcceptedFinals(context.Background(), func(current domainevidence.PrivateAcceptedFinalRecord) error {
		resolved, err := store.Resolve(context.Background(), current.AcceptedFinal.RecordDigest)
		if err != nil || resolved.StoreDigest != current.StoreDigest {
			return errors.New("accepted-final visitor could not reenter the store after snapshot capture")
		}
		return nil
	}); err != nil {
		t.Fatalf("accepted-final visitor retained the store mutex: %v", err)
	}
	if err := store.VisitDispositions(context.Background(), func(current domainevidence.AcceptedFinalDispositionRecord) error {
		resolved, err := store.ResolveDisposition(context.Background(), current.AcceptedFinalDigest)
		if err != nil || resolved.RecordDigest != current.RecordDigest {
			return errors.New("disposition visitor could not reenter the store after snapshot capture")
		}
		return nil
	}); err != nil {
		t.Fatalf("disposition visitor retained the store mutex: %v", err)
	}
	conflict, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: record.AcceptedFinal, State: domainevidence.AcceptedFinalExplicitlyNotCommitted,
		EventManifestDigest: manifestDigest, DecidedAt: time.Unix(3, 0),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), conflict); err == nil {
		t.Fatal("conflicting disposition overwrote committed authority")
	}
	path := store.recordPath(record.AcceptedFinal.RecordDigest)
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), record); err == nil {
		t.Fatal("corrupt existing private final was overwritten")
	}
	if hasRecords, err := store.HasRecords(context.Background()); err == nil || hasRecords {
		t.Fatalf("corrupt later inventory was hidden by an earlier valid record: has=%t err=%v", hasRecords, err)
	}
}

func TestPrivateAcceptedFinalVisitHonorsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "accepted-finals")
	store, err := newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.VisitAcceptedFinals(ctx, func(domainevidence.PrivateAcceptedFinalRecord) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled private final visit = %v", err)
	}
}

func TestPrivateAcceptedFinalStoreRecoversEveryExclusiveWriteResidue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "accepted-finals")
	authority, err := OpenOrCreateFileAuthority(filepath.Join(filepath.Dir(root), "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	record := finalAuthorityRecordFixture(t, authority)
	store, err := newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatal(err)
	}
	recordShard := filepath.Join(store.records, record.AcceptedFinal.RecordDigest[:2])
	if err := os.Mkdir(recordShard, 0o700); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(recordShard, "."+record.AcceptedFinal.RecordDigest+".json-partial.tmp")
	if err := os.WriteFile(partial, []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	recoverPrivateStoreForTest(t, root)
	store, err = newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatalf("recover pre-link private record: %v", err)
	}
	if _, err := os.Stat(partial); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-link private temp survived: %v", err)
	}
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	recordPath := store.recordPath(record.AcceptedFinal.RecordDigest)
	recordTemp := filepath.Join(filepath.Dir(recordPath), "."+filepath.Base(recordPath)+"-linked.tmp")
	if err := os.Link(recordPath, recordTemp); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	recoverPrivateStoreForTest(t, root)
	store, err = newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatalf("recover post-link private record: %v", err)
	}
	if _, err := os.Stat(recordTemp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-link private temp survived: %v", err)
	}
	if resolved, err := store.Resolve(context.Background(), record.AcceptedFinal.RecordDigest); err != nil || resolved.StoreDigest != record.StoreDigest {
		t.Fatalf("committed private record was not preserved: record=%#v err=%v", resolved, err)
	}

	manifestDigest := domainsecurity.SHA256Hex([]byte("publication-manifest"))
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: record.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: manifestDigest, DecidedAt: time.Unix(3, 0),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	dispositionShard := filepath.Join(store.dispositions, disposition.AcceptedFinalDigest[:2])
	if err := os.Mkdir(dispositionShard, 0o700); err != nil {
		t.Fatal(err)
	}
	partialDisposition := filepath.Join(dispositionShard, "."+disposition.AcceptedFinalDigest+".json-partial.tmp")
	if err := os.WriteFile(partialDisposition, []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	recoverPrivateStoreForTest(t, root)
	store, err = newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatalf("recover pre-link disposition: %v", err)
	}
	if _, err := os.Stat(partialDisposition); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-link disposition temp survived: %v", err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	dispositionPath := store.dispositionPath(disposition.AcceptedFinalDigest)
	dispositionTemp := filepath.Join(filepath.Dir(dispositionPath), "."+filepath.Base(dispositionPath)+"-linked.tmp")
	if err := os.Link(dispositionPath, dispositionTemp); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	recoverPrivateStoreForTest(t, root)
	store, err = newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatalf("recover post-link disposition: %v", err)
	}
	if _, err := os.Stat(dispositionTemp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-link disposition temp survived: %v", err)
	}
	if resolved, err := store.ResolveDisposition(context.Background(), disposition.AcceptedFinalDigest); err != nil || resolved.RecordDigest != disposition.RecordDigest {
		t.Fatalf("committed disposition was not preserved: disposition=%#v err=%v", resolved, err)
	}
}

func TestPrivateAcceptedFinalStoreRecoversNoReplaceRecordWriteCrashCuts(t *testing.T) {
	for _, cut := range []string{"pre-rename", "post-rename", "lost-no-replace-race"} {
		t.Run(cut, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "accepted-finals")
			authority, err := OpenOrCreateFileAuthority(filepath.Join(filepath.Dir(root), "authority", "key.json"), false)
			if err != nil {
				t.Fatal(err)
			}
			record := finalAuthorityRecordFixture(t, authority)
			body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
			if err != nil {
				t.Fatal(err)
			}
			store, err := newPrivateStoreForTest(t, root)
			if err != nil {
				t.Fatal(err)
			}
			digest := record.AcceptedFinal.RecordDigest
			shard := filepath.Join(store.records, digest[:2])
			tempPath := filepath.Join(shard, "."+digest+".json-0123456789abcdef01234567.tmp")
			finalPath := store.recordPath(digest)

			switch cut {
			case "pre-rename":
				if err := os.MkdirAll(shard, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tempPath, body, 0o600); err != nil {
					t.Fatal(err)
				}
			case "post-rename":
				if err := store.PutIfAbsent(context.Background(), record); err != nil {
					t.Fatal(err)
				}
			case "lost-no-replace-race":
				if err := store.PutIfAbsent(context.Background(), record); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(tempPath, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			recoverPrivateStoreForTest(t, root)
			reopened, err := newPrivateStoreForTest(t, root)
			if err != nil {
				t.Fatalf("recover %s record write cut: %v", cut, err)
			}
			if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s record temp survived recovery: %v", cut, err)
			}
			if cut == "pre-rename" {
				if _, err := os.Stat(finalPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("pre-rename record was promoted without a no-replace commit: %v", err)
				}
				if err := reopened.PutIfAbsent(context.Background(), record); err != nil {
					t.Fatalf("pre-rename record could not be deterministically replayed: %v", err)
				}
			}
			resolved, err := reopened.Resolve(context.Background(), digest)
			if err != nil {
				t.Fatal(err)
			}
			resolvedBody, err := domainevidence.PrivateAcceptedFinalRecordBytes(resolved)
			if err != nil || !bytes.Equal(resolvedBody, body) {
				t.Fatalf("%s record bytes changed across recovery: err=%v", cut, err)
			}
		})
	}
}

func TestPrivateAcceptedFinalStoreRejectsUnknownResidueAndUntrackedHardlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "accepted-finals")
	store, err := newPrivateStoreForTest(t, root)
	if err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(store.records, "aa")
	if err := os.Mkdir(shard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shard, ".unknown.tmp"), []byte("untrusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newPrivateStoreForTest(t, root); err == nil {
		t.Fatal("unknown private authority residue passed restart recovery")
	}
	if runtime.GOOS == "windows" {
		return
	}

	cleanRoot := filepath.Join(t.TempDir(), "accepted-finals")
	authority, err := OpenOrCreateFileAuthority(filepath.Join(filepath.Dir(cleanRoot), "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	record := finalAuthorityRecordFixture(t, authority)
	store, err = newPrivateStoreForTest(t, cleanRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	outsideLink := filepath.Join(t.TempDir(), "untracked-link.json")
	if err := os.Link(store.recordPath(record.AcceptedFinal.RecordDigest), outsideLink); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	if _, err := store.Resolve(context.Background(), record.AcceptedFinal.RecordDigest); err == nil {
		t.Fatal("untracked private authority hardlink passed read validation")
	}
}

func TestPrivateAcceptedFinalStoreRejectsNonCanonicalAuthorityPaths(t *testing.T) {
	root := t.TempDir()
	authority, err := OpenOrCreateFileAuthority(filepath.Join(root, "authority", "key.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	record := finalAuthorityRecordFixture(t, authority)
	storeRoot := filepath.Join(root, "accepted-finals")
	store, err := newPrivateStoreForTest(t, storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	// The signed record digest depends on a newly generated authority key.
	// A fixed "ff" shard can therefore be its valid shard, not a bad fixture.
	wrongShardName := "ff"
	if record.AcceptedFinal.RecordDigest[:2] == wrongShardName {
		wrongShardName = "00"
	}
	wrongShard := filepath.Join(store.records, wrongShardName)
	if err := os.Mkdir(wrongShard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrongShard, record.AcceptedFinal.RecordDigest+".json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newPrivateStoreForTest(t, storeRoot); err == nil {
		t.Fatal("valid authority content at a non-canonical shard passed restart inventory")
	}

	rootLevel := filepath.Join(t.TempDir(), "accepted-finals")
	store, err = newPrivateStoreForTest(t, rootLevel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.dispositions, record.AcceptedFinal.RecordDigest+".json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newPrivateStoreForTest(t, rootLevel); err == nil {
		t.Fatal("root-level private authority JSON passed restart inventory")
	}
}

func TestPrivateAcceptedFinalStoreNeverFollowsSwappedAuthorityRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix ancestor-swap coverage")
	}
	for _, replacement := range []string{"symlink", "directory"} {
		t.Run(replacement, func(t *testing.T) {
			root := t.TempDir()
			authority, err := OpenOrCreateFileAuthority(filepath.Join(root, "authority", "key.json"), false)
			if err != nil {
				t.Fatal(err)
			}
			record := finalAuthorityRecordFixture(t, authority)
			storeRoot := filepath.Join(root, "accepted-finals")
			store, err := newPrivateStoreForTest(t, storeRoot)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PutIfAbsent(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(storeRoot, "records")
			moved := filepath.Join(storeRoot, "records-original")
			if err := os.Rename(original, moved); err != nil {
				t.Fatal(err)
			}
			escape := filepath.Join(root, "escape")
			if err := os.Mkdir(escape, 0o700); err != nil {
				t.Fatal(err)
			}
			if replacement == "symlink" {
				if err := os.Symlink(escape, original); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(original, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := store.PutIfAbsent(context.Background(), record); err == nil {
				t.Fatal("swapped private authority root accepted a write")
			}
			if _, err := store.Resolve(context.Background(), record.AcceptedFinal.RecordDigest); err == nil {
				t.Fatal("swapped private authority root accepted a read")
			}
			entries, err := os.ReadDir(escape)
			if err != nil || len(entries) != 0 {
				t.Fatalf("private authority operation escaped the captured root: entries=%#v err=%v", entries, err)
			}
		})
	}
}

func finalAuthorityRecordFixture(t *testing.T, authority *FileAuthority) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: securityContext, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: time.Unix(2, 0).UTC().Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(securityContext, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: time.Unix(2, 0), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainevidence.NewPrivateAcceptedFinalRecord(securityContext, envelope, rendered, head, intent, acceptedFinal)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
