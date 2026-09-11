package startup

import (
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSemanticStartupPlanRejectsOperationBudgetBeforeConstruction(t *testing.T) {
	digest := domainsecurity.SHA256Hex([]byte("fixture"))
	operations := make([]SemanticStartupOperationV1, MaxSemanticPlanOperationsV1+1)
	if _, err := NewSemanticStartupPlanV1(digest, digest, digest, operations); err == nil {
		t.Fatal("semantic startup operation budget was not enforced")
	}
}

func TestSemanticStartupPlanRejectsOversizedFileAndPath(t *testing.T) {
	digest := domainsecurity.SHA256Hex([]byte("fixture"))
	for _, operation := range []SemanticStartupOperationV1{
		{
			Kind: SemanticOperationInstallFile, Path: "data/private/oversized.bin",
			Before: SemanticEntryStateV1{Type: ManagedEntryTypeAbsent},
			After:  SemanticEntryStateV1{Type: ManagedEntryTypeFile, Mode: 0o600, Size: MaxSemanticManagedFileBytesV1 + 1, SHA256: digest},
		},
		{
			Kind:   SemanticOperationCreateDirectory,
			Path:   "data/" + strings.Repeat("a", MaxSemanticManagedPathBytesV1),
			Before: SemanticEntryStateV1{Type: ManagedEntryTypeAbsent},
			After:  SemanticEntryStateV1{Type: ManagedEntryTypeDirectory, Mode: 0o700},
		},
	} {
		if _, err := NewSemanticStartupPlanV1(digest, digest, digest, []SemanticStartupOperationV1{operation}); err == nil {
			t.Fatalf("semantic startup accepted an over-budget operation: %#v", operation)
		}
	}
}

func TestManagedSnapshotRejectsEntryBudgetBeforeConstruction(t *testing.T) {
	entries := make([]ManagedEntryStateV1, MaxManagedSnapshotEntriesV1+1)
	if _, err := NewManagedSnapshotV1([]string{"/data"}, domainsecurity.SHA256Hex([]byte("raw")), entries); err == nil {
		t.Fatal("managed snapshot entry budget was not enforced")
	}
}
