package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func TestSemanticCopyStreamsExactBytesAndHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.bin")
	body := make([]byte, 2<<20)
	for index := range body {
		body[index] = byte(index % 251)
	}
	if err := os.WriteFile(source, body, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	expected := EntryRecord{
		Path: "data/private/source.bin", Type: domainstartup.ManagedEntryTypeFile,
		Mode: uint32(info.Mode()), Size: info.Size(), ModTimeUnixNano: info.ModTime().UnixNano(),
		SHA256: domainsecurity.SHA256Hex(body),
	}
	target := filepath.Join(root, "stage", "source.bin")
	if err := copyVerifiedFile(context.Background(), source, target, expected); err != nil {
		t.Fatalf("stream exact semantic file: %v", err)
	}
	written, err := os.ReadFile(target)
	if err != nil || domainsecurity.SHA256Hex(written) != expected.SHA256 {
		t.Fatalf("streamed semantic file changed: err=%v", err)
	}
	cancelledTarget := filepath.Join(root, "cancelled", "source.bin")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := copyVerifiedFile(ctx, source, cancelledTarget, expected); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled semantic copy error = %v", err)
	}
	if _, err := os.Lstat(cancelledTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled semantic copy left a staged file: %v", err)
	}
}

func TestSemanticCopyRejectsFileBudgetBeforeCreatingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "stage", "oversized.bin")
	err := copyVerifiedFile(context.Background(), filepath.Join(t.TempDir(), "missing.bin"), target, EntryRecord{
		Size: domainstartup.MaxSemanticManagedFileBytesV1 + 1,
	})
	var limit StartupResourceLimitError
	if !errors.As(err, &limit) || limit.Code != "managed_file_bytes" {
		t.Fatalf("oversized semantic copy error = %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized semantic copy created a target: %v", err)
	}
}

func TestSemanticControlArtifactsRejectByteBudgetsBeforeDecode(t *testing.T) {
	for _, test := range []struct {
		name string
		body []byte
		call func([]byte) error
		code string
	}{
		{
			name: "journal", body: make([]byte, maxSemanticJournalBytes+1), code: "journal_bytes",
			call: func(body []byte) error { _, err := decodeSemanticJournal(body, nil); return err },
		},
		{
			name: "retirement", body: make([]byte, maxSemanticRetirementBytes+1), code: "retirement_bytes",
			call: func(body []byte) error { _, err := decodeSemanticRetirement(body, nil); return err },
		},
		{
			name: "planning", body: make([]byte, maxSemanticPlanningBytes+1), code: "planning_bytes",
			call: func(body []byte) error { _, err := decodeSemanticPlanningMarker(body); return err },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var limit StartupResourceLimitError
			if err := test.call(test.body); !errors.As(err, &limit) || limit.Code != test.code {
				t.Fatalf("control artifact budget error = %v", err)
			}
		})
	}
}

func TestPlanningStageUsesPersistentPrivateAuthorityNamespace(t *testing.T) {
	roots := semanticRootsForTest(t)
	if _, err := FreezeJournalNamespaceAuthorityForRoots(roots); err != nil {
		t.Fatal(err)
	}
	namespace, err := persistentStartupNamespacePath(roots, true)
	if err != nil {
		t.Fatal(err)
	}
	planningRoot, err := semanticPlanningRoot(roots)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(planningRoot) != namespace {
		t.Fatalf("planning root escaped the private authority namespace: planning=%s namespace=%s", planningRoot, namespace)
	}
	if info, err := os.Lstat(namespace); err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("private planning namespace permissions are unsafe: info=%v err=%v", info, err)
	}
}

func TestCaptureStrictRejectsManagedDepthBudget(t *testing.T) {
	roots := testRootSet(t)
	current := filepath.Join(roots.DataDir, "private")
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	for depth := 0; depth < domainstartup.MaxSemanticManagedPathDepthV1; depth++ {
		current = filepath.Join(current, "d")
	}
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	var limit StartupResourceLimitError
	if _, err := CaptureStrict(roots); !errors.As(err, &limit) || limit.Code != "managed_depth" && limit.Code != "managed_path" {
		t.Fatalf("managed depth budget error = %v", err)
	}
}

func TestCaptureStrictContextCancellationStopsBeforeInventory(t *testing.T) {
	roots := testRootSet(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CaptureStrictContext(ctx, roots); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled snapshot error = %v", err)
	}
}

func TestStartupPrivateDirectoryCancellationLeavesRetiredTreeResumable(t *testing.T) {
	roots := semanticRootsForTest(t)
	authority, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	if err != nil {
		t.Fatal(err)
	}
	root := authority.root
	rootPath := root.pathValue()
	name := ".retired-test-0123456789abcdef0123456789abcdef"
	retired, err := secureStartupCreateDirectory(root, name)
	if err != nil {
		t.Fatal(err)
	}
	defer retired.Close()
	if err := retired.WriteExclusive("payload", []byte("resumable"), 32); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := retired.ReadEntriesBoundedContext(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled private inventory error = %v", err)
	}
	if err := secureStartupRemoveRetiredDirectoryContext(ctx, root, name, retired); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retired-tree cleanup error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(rootPath, name, "payload"))
	if err != nil || string(body) != "resumable" {
		t.Fatalf("cancelled cleanup changed resumable tree: body=%q err=%v", body, err)
	}
}

func TestSnapshotScannerRejectsEntryBudgetBeforeAppend(t *testing.T) {
	scanner := snapshotScanner{entries: make([]EntryRecord, domainstartup.MaxManagedSnapshotEntriesV1)}
	var limit StartupResourceLimitError
	err := scanner.appendEntry(EntryRecord{Path: "data/private/late", Type: domainstartup.ManagedEntryTypeAbsent})
	if !errors.As(err, &limit) || limit.Code != "managed_entries" {
		t.Fatalf("managed entry budget error = %v", err)
	}
}
