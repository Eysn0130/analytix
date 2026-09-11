package casethreadauthority

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestStorePersistsStrictCaseThreadAuthorityInventory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "case-thread-authority")
	store, err := newTestCaseThreadStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	record := testCaseThreadAuthorityRecord(t)
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), record); err != nil {
		t.Fatalf("idempotent authority write failed: %v", err)
	}
	reopened, err := newTestCaseThreadStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	records, err := reopened.List(context.Background())
	if err != nil || len(records) != 1 || records[0].RecordDigest != record.RecordDigest {
		t.Fatalf("case thread authority restart inventory mismatch: records=%#v err=%v", records, err)
	}
	if err := os.WriteFile(filepath.Join(root, "unknown.txt"), []byte("untrusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.List(context.Background()); err == nil {
		t.Fatal("case thread authority store ignored an unknown inventory file")
	}
}

func TestStoreRejectsTempCrashAndUnknownInventoryResidue(t *testing.T) {
	tests := []struct {
		name   string
		poison func(*testing.T, string, domainsecurity.CaseThreadAuthorityRecord)
	}{
		{
			name: "root temp",
			poison: func(t *testing.T, root string, _ domainsecurity.CaseThreadAuthorityRecord) {
				writeTestFile(t, filepath.Join(root, ".authority.json-crash.tmp"), []byte("partial"))
			},
		},
		{
			name: "shard temp",
			poison: func(t *testing.T, root string, record domainsecurity.CaseThreadAuthorityRecord) {
				writeTestFile(t, filepath.Join(root, record.RecordDigest[:2], "."+record.RecordDigest+".json-crash.tmp"), []byte("partial"))
			},
		},
		{
			name: "partial json record",
			poison: func(t *testing.T, root string, record domainsecurity.CaseThreadAuthorityRecord) {
				digest := strings.Repeat("a", 64)
				if digest == record.RecordDigest {
					digest = strings.Repeat("b", 64)
				}
				writeTestFile(t, filepath.Join(root, digest[:2], digest+".json"), []byte("{"))
			},
		},
		{
			name: "unknown file",
			poison: func(t *testing.T, root string, _ domainsecurity.CaseThreadAuthorityRecord) {
				writeTestFile(t, filepath.Join(root, ".DS_Store"), []byte("untrusted"))
			},
		},
		{
			name: "empty shard",
			poison: func(t *testing.T, root string, record domainsecurity.CaseThreadAuthorityRecord) {
				shard := "ff"
				if shard == record.RecordDigest[:2] {
					shard = "ee"
				}
				if err := os.Mkdir(filepath.Join(root, shard), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unknown directory",
			poison: func(t *testing.T, root string, _ domainsecurity.CaseThreadAuthorityRecord) {
				if err := os.Mkdir(filepath.Join(root, "staging"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "wrong digest shard",
			poison: func(t *testing.T, root string, record domainsecurity.CaseThreadAuthorityRecord) {
				shard := "ff"
				if shard == record.RecordDigest[:2] {
					shard = "ee"
				}
				body, err := domainsecurity.CaseThreadAuthorityRecordBytes(record)
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(root, shard, record.RecordDigest+".json"), body)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "case-thread-authority")
			store, err := newTestCaseThreadStore(t, root)
			if err != nil {
				t.Fatal(err)
			}
			record := testCaseThreadAuthorityRecord(t)
			if err := store.PutIfAbsent(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			test.poison(t, root, record)
			if _, err := store.List(context.Background()); err == nil {
				t.Fatal("List accepted poisoned case thread authority inventory")
			}
			if hasRecords, err := store.HasRecords(context.Background()); err == nil || hasRecords {
				t.Fatalf("HasRecords did not fail closed: hasRecords=%v err=%v", hasRecords, err)
			}
		})
	}
}

func TestStorePreservesPreexistingIncompleteTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "case-thread-authority")
	store, err := newTestCaseThreadStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	record := testCaseThreadAuthorityRecord(t)
	path := filepath.Join(root, record.RecordDigest[:2], record.RecordDigest+".json")
	residue := []byte("{\"incomplete\":")
	writeTestFile(t, path, residue)
	if err := store.PutIfAbsent(context.Background(), record); err == nil {
		t.Fatal("PutIfAbsent replaced an incomplete preexisting authority target")
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, residue) {
		t.Fatalf("preexisting authority residue was replaced: got %q", stored)
	}
}

func testCaseThreadAuthorityRecord(t *testing.T) domainsecurity.CaseThreadAuthorityRecord {
	t.Helper()
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "authority", "ed25519.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 2, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainsecurity.NewCaseThreadAuthorityRecord(
		securityContext, authority.KeyID(), authority.PublicKey(),
		func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func writeTestFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestCaseThreadStore(t *testing.T, root string) (*Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewStore(root, mutation)
}
