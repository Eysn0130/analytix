package startup

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func TestAuthorizedManagedDeltaAcceptsOnlyExactAddedFileAndParents(t *testing.T) {
	base := []domainstartup.ManagedEntryStateV1{
		{Path: "data/private", Type: domainstartup.ManagedEntryTypeDirectory, Mode: 0o700, Size: 128, ModTimeUnixNano: 1},
		{Path: "data/private/checkpoint-authority/operation-group-terminals-v2", Type: domainstartup.ManagedEntryTypeDirectory, Mode: 0o700, Size: 64, ModTimeUnixNano: 1},
		{Path: "durable/threads/thread-a/thread.json", Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: 2, ModTimeUnixNano: 1, SHA256: domainsecurity.SHA256Hex([]byte("{}")), RecordCount: 1},
	}
	before := managedSnapshotForAuthorizedDeltaTest(t, "before", base)
	body := []byte(`{"purpose":"terminal"}`)
	afterEntries := append([]domainstartup.ManagedEntryStateV1(nil), base...)
	afterEntries[1].Size = 96
	afterEntries[1].ModTimeUnixNano = 2
	afterEntries = append(afterEntries,
		domainstartup.ManagedEntryStateV1{Path: "data/private/checkpoint-authority/operation-group-terminals-v2/ab", Type: domainstartup.ManagedEntryTypeDirectory, Mode: 0o700, Size: 64, ModTimeUnixNano: 2},
		domainstartup.ManagedEntryStateV1{Path: "data/private/checkpoint-authority/operation-group-terminals-v2/ab/abcdef.json", Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), ModTimeUnixNano: 2, SHA256: domainsecurity.SHA256Hex(body), RecordCount: 1},
	)
	after := managedSnapshotForAuthorizedDeltaTest(t, "after", afterEntries)
	delta := AuthorizedManagedDeltaV1{
		AddedFiles: []AuthorizedManagedFileAdditionV1{{
			Path: "data/private/checkpoint-authority/operation-group-terminals-v2/ab/abcdef.json",
			Mode: 0o600, Size: int64(len(body)), ModTimeUnixNano: 2,
			SHA256: domainsecurity.SHA256Hex(body), RecordCount: 1,
		}},
		MutableDirectories: []AuthorizedManagedDirectoryV1{
			{Path: "data/private/checkpoint-authority/operation-group-terminals-v2", Mode: 0o700, Size: 96, ModTimeUnixNano: 2},
			{Path: "data/private/checkpoint-authority/operation-group-terminals-v2/ab", Mode: 0o700, Size: 64, ModTimeUnixNano: 2},
		},
	}
	if err := ValidateAuthorizedManagedDeltaV1(before, after, delta); err != nil {
		t.Fatalf("exact checkpoint recovery delta was rejected: %v", err)
	}

	tamperedEntries := append([]domainstartup.ManagedEntryStateV1(nil), afterEntries...)
	tamperedEntries[2].ModTimeUnixNano = 3
	tampered := managedSnapshotForAuthorizedDeltaTest(t, "tampered", tamperedEntries)
	if err := ValidateAuthorizedManagedDeltaV1(before, tampered, delta); err == nil {
		t.Fatal("unrelated managed file mutation was absorbed into the recovery baseline")
	}
}

func TestAuthorizedManagedDeltaRejectsMissingOrMismatchedTerminal(t *testing.T) {
	body := []byte(`{"purpose":"terminal"}`)
	beforeEntries := []domainstartup.ManagedEntryStateV1{{
		Path: "data/private/checkpoint-authority/operation-group-terminals-v2", Type: domainstartup.ManagedEntryTypeDirectory,
		Mode: 0o700, Size: 64, ModTimeUnixNano: 1,
	}}
	before := managedSnapshotForAuthorizedDeltaTest(t, "before", beforeEntries)
	afterEntries := append([]domainstartup.ManagedEntryStateV1(nil), beforeEntries...)
	afterEntries = append(afterEntries, domainstartup.ManagedEntryStateV1{
		Path: "data/private/checkpoint-authority/operation-group-terminals-v2/ab/abcdef.json",
		Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), ModTimeUnixNano: 2,
		SHA256: domainsecurity.SHA256Hex([]byte("different")), RecordCount: 1,
	})
	after := managedSnapshotForAuthorizedDeltaTest(t, "after", afterEntries)
	delta := AuthorizedManagedDeltaV1{
		AddedFiles: []AuthorizedManagedFileAdditionV1{{
			Path: "data/private/checkpoint-authority/operation-group-terminals-v2/ab/abcdef.json",
			Mode: 0o600, Size: int64(len(body)), ModTimeUnixNano: 2,
			SHA256: domainsecurity.SHA256Hex(body), RecordCount: 1,
		}},
		MutableDirectories: []AuthorizedManagedDirectoryV1{{
			Path: "data/private/checkpoint-authority/operation-group-terminals-v2", Mode: 0o700, Size: 64, ModTimeUnixNano: 1,
		}},
	}
	if err := ValidateAuthorizedManagedDeltaV1(before, after, delta); err == nil {
		t.Fatal("mismatched recovery terminal content was accepted")
	}
}

func managedSnapshotForAuthorizedDeltaTest(
	t *testing.T,
	label string,
	entries []domainstartup.ManagedEntryStateV1,
) domainstartup.ManagedSnapshotV1 {
	t.Helper()
	snapshot, err := domainstartup.NewManagedSnapshotV1(
		[]string{"/data", "/durable"}, domainsecurity.SHA256Hex([]byte(label)), entries,
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
