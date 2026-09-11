package jobs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestChildRunInventoryAcceptsOnlyExactRuntimeObjects(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"job-1.json":                 []byte("{\"id\":\"job-1\"}\n"),
		"job-1.log":                  []byte("opaque reasoning-era artifact\n"),
		".job-2.json.tmp-0":          []byte("legacy partial"),
		".job-3.json.tmp-4294967295": []byte("legacy complete uint32 suffix"),
		".job-4.json.tmp-v1-0123456789abcdef0123456789abcdef": []byte("versioned partial"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}
	canonicalRoot, err := canonicalChildRunInventoryRootV1(root)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.SchemaVersion != ChildRunInventorySchemaVersionV1 || !inventory.RootExists ||
		inventory.Root != canonicalRoot || len(inventory.Entries) != len(files) || !isLowerSHA256V1(inventory.ManifestSHA256) {
		t.Fatalf("unexpected inventory: %#v", inventory)
	}
	for _, entry := range inventory.Entries {
		data, ok := files[entry.Name]
		if !ok {
			t.Fatalf("unexpected entry: %#v", entry)
		}
		digest := sha256.Sum256(data)
		if entry.Path != filepath.Join(canonicalRoot, entry.Name) || entry.SizeBytes != int64(len(data)) ||
			entry.SHA256 != hex.EncodeToString(digest[:]) || entry.Identity.Device == "" ||
			entry.Identity.Inode == "" || entry.Identity.LinkCount != 1 {
			t.Fatalf("entry is not fully bound: %#v", entry)
		}
		kind, jobID, ok := classifyChildRunInventoryNameV1(entry.Name)
		if !ok || entry.Kind != kind || entry.JobID != jobID {
			t.Fatalf("entry classification drifted: %#v", entry)
		}
	}
	if err := ValidateChildRunInventoryV1(root, inventory); err != nil {
		t.Fatalf("validate unchanged inventory: %v", err)
	}
}

func TestChildRunInventoryRecognizesOnlyExactRetiredTypeScriptProducerName(t *testing.T) {
	root := t.TempDir()
	const name = "child_abcdefgh_abc123.json"
	if err := os.WriteFile(filepath.Join(root, name), []byte(`{"id":"child_abcdefgh_abc123"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatalf("build legacy TypeScript inventory: %v", err)
	}
	if len(inventory.Entries) != 1 || inventory.Entries[0].Kind != ChildRunInventoryLegacyTypeScriptRecordV1 ||
		inventory.Entries[0].JobID != "child_abcdefgh_abc123" {
		t.Fatalf("retired producer entry classification mismatch: %#v", inventory.Entries)
	}
	if _, err := readChildRunInventoryEntryV1(root, inventory.Entries[0]); err != nil {
		t.Fatalf("retired producer record could not be read for semantic migration: %v", err)
	}
}

func TestChildRunInventoryRejectsAliasesDirectoriesAndNearTemporaryNames(t *testing.T) {
	invalidNames := []string{
		"JOB-1.json",
		"Job-1.json",
		"job-01.json",
		"job-0.json",
		"job-1.JSON",
		"job-1.LOG",
		"job-1.json.bak",
		".job-1.json.tmp-crash",
		".job-1.json.tmp-000000001",
		".job-1.json.tmp-4294967296",
		".job-1.json.tmp-1.extra",
		".job-1.json.tmp-v1-0123456789abcdef",
		".job-1.json.tmp-v1-0123456789abcdef0123456789abcdeG",
		".job-1.json.tmp-v1-0123456789ABCDEF0123456789ABCDEF",
		".job-1.json.tmp-v2-0123456789abcdef0123456789abcdef",
		".JOB-1.json.tmp-1",
		"child_abcdefg_abc123.json",
		"child_abcdefghi_abc123.json",
		"child_abcdefgh_abc12.json",
		"child_abcdefgh_abc1234.json",
		"child_ABCDEFGH_ABC123.json",
		"child_abcdefgh-abc123.json",
		"child_abcdefgh_abc123.json.bak",
		".DS_Store",
	}
	for _, name := range invalidNames {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, name), []byte("sentinel"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := BuildChildRunInventoryV1(root); err == nil {
				t.Fatalf("accepted invalid child-run object %q", name)
			}
		})
	}
	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "job-1.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := BuildChildRunInventoryV1(root); err == nil {
			t.Fatal("accepted a directory with a record-shaped name")
		}
	})
}

func TestChildRunTemporaryProducerOwnsExactVersionedGrammar(t *testing.T) {
	root := t.TempDir()
	handle, err := createChildRunTemporaryFileV1(root, "job-7.json")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(handle.Name())
	if _, err := handle.Write([]byte("temporary bytes")); err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	kind, jobID, ok := classifyChildRunInventoryNameV1(name)
	if !ok || kind != ChildRunInventoryTemporaryV1 || jobID != "job-7" ||
		!strings.HasPrefix(name, ".job-7.json.tmp-v1-") || len(strings.TrimPrefix(name, ".job-7.json.tmp-v1-")) != 32 {
		t.Fatalf("producer emitted a name outside the versioned grammar: %q kind=%q job=%q", name, kind, jobID)
	}
	if _, err := BuildChildRunInventoryV1(root); err != nil {
		t.Fatalf("inventory rejected producer-owned temporary: %v", err)
	}
	for _, finalBase := range []string{"", "job-0.json", "job-01.json", "job-1.log", "../job-1.json"} {
		if temporary, err := createChildRunTemporaryFileV1(root, finalBase); err == nil {
			_ = temporary.Close()
			t.Fatalf("temporary producer accepted invalid final base %q", finalBase)
		}
	}
}

func TestChildRunInventoryUnknownObjectFailureDoesNotMutateBytes(t *testing.T) {
	root := t.TempDir()
	const privateFilename = "PRIVATE_REASONING_FILENAME_SENTINEL"
	if err := os.WriteFile(filepath.Join(root, "job-1.json"), []byte("record bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, privateFilename), []byte("unknown sentinel\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	before := childRunDirectoryBytesForTest(t, root)
	_, inventoryErr := BuildChildRunInventoryV1(root)
	if inventoryErr == nil {
		t.Fatal("accepted unknown child-run object")
	}
	if strings.Contains(inventoryErr.Error(), privateFilename) {
		t.Fatal("unknown untrusted filename was reflected into the inventory error")
	}
	after := childRunDirectoryBytesForTest(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected inventory mutated live objects:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestChildRunInventoryEnforcesObjectAndSparseFileLimitsWithoutCleanup(t *testing.T) {
	t.Run("object-count", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"job-1.json", "job-2.json"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("record\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		before := childRunDirectoryBytesForTest(t, root)
		if _, err := buildChildRunInventoryV1(root, childRunInventoryHooksV1{maxObjects: 1}); err == nil {
			t.Fatal("accepted an inventory over the configured object limit")
		}
		after := childRunDirectoryBytesForTest(t, root)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("object-limit rejection mutated the tree: before=%#v after=%#v", before, after)
		}
	})
	for _, fixture := range []struct {
		name string
		size int64
	}{
		{name: "job-1.json", size: childRunInventoryRecordMaxBytesV1 + 1},
		{name: "job-1.log", size: childRunInventoryOpaqueMaxBytesV1 + 1},
	} {
		t.Run("sparse-"+strings.ReplaceAll(fixture.name, ".", "-"), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, fixture.name)
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(path, fixture.size); err != nil {
				t.Fatal(err)
			}
			if _, err := BuildChildRunInventoryV1(root); err == nil {
				t.Fatalf("accepted oversized sparse object %q", fixture.name)
			}
			info, err := os.Stat(path)
			if err != nil || info.Size() != fixture.size {
				t.Fatalf("size-limit rejection removed or changed sparse object: info=%#v err=%v", info, err)
			}
		})
	}
	t.Run("growth-after-open", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "job-1.json")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		grew := false
		_, err := buildChildRunInventoryV1(root, childRunInventoryHooksV1{
			afterFileOpen: func(openPath string) error {
				if grew {
					return nil
				}
				grew = true
				return os.Truncate(openPath, childRunInventoryRecordMaxBytesV1+1)
			},
		})
		if err == nil {
			t.Fatal("accepted a file that grew over the cap after its initial stat")
		}
		info, statErr := os.Stat(path)
		if statErr != nil || info.Size() != childRunInventoryRecordMaxBytesV1+1 {
			t.Fatalf("growth-race rejection changed the sparse file: info=%#v err=%v", info, statErr)
		}
	})
}

func TestChildRunInventoryRejectsSymlinkHardlinkAndSpecialFile(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "child-runs")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(base, "outside.json")
		if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "job-1.json")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := BuildChildRunInventoryV1(root); err == nil {
			t.Fatal("accepted symlinked child-run record")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "child-runs")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(root, "job-1.json")
		if err := os.WriteFile(record, []byte("record"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(record, filepath.Join(base, "outside-hardlink.json")); err != nil {
			t.Skipf("hardlink unavailable: %v", err)
		}
		if _, err := BuildChildRunInventoryV1(root); err == nil {
			t.Fatal("accepted hard-linked child-run record")
		}
	})
	t.Run("special-file", func(t *testing.T) {
		// macOS expands t.TempDir() beneath a long per-user path. Unix-domain
		// socket paths have a much smaller platform limit, so the previous
		// fixture could skip the security assertion before inventory code ran.
		// Use the conventional short Unix temp root for this socket-only fixture.
		shortRoot := os.TempDir()
		if info, statErr := os.Stat("/tmp"); statErr == nil && info.IsDir() {
			shortRoot = "/tmp"
		}
		root, err := os.MkdirTemp(shortRoot, "analytix-job-inventory-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
		listener, err := net.Listen("unix", filepath.Join(root, "job-1.log"))
		if err != nil {
			t.Skipf("unix socket unavailable: %v", err)
		}
		defer listener.Close()
		if _, err := BuildChildRunInventoryV1(root); err == nil {
			t.Fatal("accepted special child-run artifact")
		}
	})
}

func TestChildRunInventoryRejectsDeterministicReplacementRaces(t *testing.T) {
	t.Run("between-snapshots", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "child-runs")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "job-1.json")
		if err := os.WriteFile(path, []byte("first-body"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := buildChildRunInventoryV1(root, childRunInventoryHooksV1{
			betweenSnapshots: func() error {
				if err := os.Rename(path, filepath.Join(base, "replaced-record")); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("other-body"), 0o600)
			},
		})
		if err == nil {
			t.Fatal("accepted replacement between stable snapshots")
		}
	})
	t.Run("while-hashing", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "child-runs")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "job-1.json")
		if err := os.WriteFile(path, []byte("first-body"), 0o600); err != nil {
			t.Fatal(err)
		}
		mutated := false
		_, err := buildChildRunInventoryV1(root, childRunInventoryHooksV1{
			afterFileOpen: func(openPath string) error {
				if mutated {
					return nil
				}
				mutated = true
				if openPath != path {
					return errors.New("unexpected race path")
				}
				if err := os.Rename(path, filepath.Join(base, "opened-record")); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("other-body"), 0o600)
			},
		})
		if err == nil {
			t.Fatal("accepted replacement while hashing")
		}
	})
}

func TestChildRunInventoryIsByteStableAndPlanValidationFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "job-9.json"), []byte("stable record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "job-9.log"), []byte("stable artifact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("unchanged scans are not byte-stable:\n%s\n%s", firstJSON, secondJSON)
	}
	tampered := first
	tampered.Entries = append([]ChildRunInventoryEntryV1(nil), first.Entries...)
	tampered.Entries[0].SHA256 = strings.Repeat("0", sha256.Size*2)
	if err := ValidateChildRunInventoryV1(root, tampered); err == nil {
		t.Fatal("accepted tampered plan input")
	}
	if err := os.WriteFile(filepath.Join(root, "job-9.log"), []byte("changed artifact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateChildRunInventoryV1(root, first); err == nil {
		t.Fatal("accepted stale plan after child-run bytes changed")
	}
}

func TestChildRunInventoryRecordReadRevalidatesExactHandleAndRejectsOpaqueBytes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "child-runs")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	recordBody := []byte("{\"id\":\"job-1\",\"output\":\"legacy\"}\n")
	if err := os.WriteFile(filepath.Join(root, "job-1.json"), recordBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "job-1.log"), []byte("opaque log bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	record := childRunInventoryEntryByNameForTest(t, inventory, "job-1.json")
	read, err := readChildRunInventoryEntryV1(root, record)
	if err != nil || !bytes.Equal(read, recordBody) {
		t.Fatalf("exact record read failed: data=%q err=%v", read, err)
	}
	artifact := childRunInventoryEntryByNameForTest(t, inventory, "job-1.log")
	if _, err := readChildRunInventoryEntryV1(root, artifact); err == nil {
		t.Fatal("opaque artifact was exposed as parseable migration input")
	}
	tampered := record
	tampered.SHA256 = strings.Repeat("0", sha256.Size*2)
	if _, err := readChildRunInventoryEntryV1(root, tampered); err == nil {
		t.Fatal("record read accepted a mismatched expected digest")
	}

	mutated := false
	_, err = readChildRunInventoryEntryWithHookV1(root, record, func(path string) error {
		if mutated {
			return nil
		}
		mutated = true
		if err := os.Rename(path, filepath.Join(base, "opened-record")); err != nil {
			return err
		}
		return os.WriteFile(path, bytes.Repeat([]byte("x"), len(recordBody)), 0o600)
	})
	if err == nil {
		t.Fatal("record read accepted a path replacement after opening")
	}
}

func TestChildRunInventoryResidueRemovalRejectsRecordsAndRemovesOnlyExactResidue(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"job-1.json":         []byte("record\n"),
		"job-1.log":          []byte("artifact\n"),
		".job-1.json.tmp-42": []byte("legacy residue\n"),
		".job-1.json.tmp-v1-0123456789abcdef0123456789abcdef": []byte("current residue\n"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	record := childRunInventoryEntryByNameForTest(t, inventory, "job-1.json")
	if err := removeChildRunInventoryResidueV1(root, record); err == nil {
		t.Fatal("committed record was removable as residue")
	}
	if body, err := os.ReadFile(record.Path); err != nil || !bytes.Equal(body, files[record.Name]) {
		t.Fatalf("record-delete rejection mutated record: body=%q err=%v", body, err)
	}
	for _, name := range []string{
		"job-1.log",
		".job-1.json.tmp-42",
		".job-1.json.tmp-v1-0123456789abcdef0123456789abcdef",
	} {
		entry := childRunInventoryEntryByNameForTest(t, inventory, name)
		if err := removeChildRunInventoryResidueV1(root, entry); err != nil {
			t.Fatalf("remove exact residue %q: %v", name, err)
		}
		if _, err := os.Lstat(entry.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("removed residue %q remains: %v", name, err)
		}
	}
	if _, err := os.Stat(record.Path); err != nil {
		t.Fatalf("residue cleanup removed committed record: %v", err)
	}
}

func TestChildRunInventoryResidueRemovalRejectsReplacementRaceWithoutDeletingReplacement(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "child-runs")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "job-1.log")
	original := []byte("original artifact\n")
	replacement := []byte("replacement bytes\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := childRunInventoryEntryByNameForTest(t, inventory, "job-1.log")
	err = removeChildRunInventoryResidueWithHooksV1(root, entry, childRunResidueRemovalHooksV1{
		afterFirstValidation: func(removePath string) error {
			if removePath != entry.Path {
				return errors.New("unexpected removal path")
			}
			if err := os.Rename(entry.Path, filepath.Join(base, "original-artifact")); err != nil {
				return err
			}
			return os.WriteFile(entry.Path, replacement, 0o600)
		},
	})
	if err == nil {
		t.Fatal("residue removal accepted a replacement race")
	}
	body, readErr := os.ReadFile(entry.Path)
	if readErr != nil || !bytes.Equal(body, replacement) {
		t.Fatalf("failed removal deleted or changed the replacement: body=%q err=%v", body, readErr)
	}
}

func TestRetiredTypeScriptSourceRemovalRequiresDedicatedPathAndRejectsReplacementRace(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "child-runs")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "child_abcdefgh_abc123.json")
	original := []byte("original retired source\n")
	replacement := []byte("replacement retired source\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	entry := childRunInventoryEntryByNameForTest(t, inventory, filepath.Base(path))
	if err := removeChildRunInventoryResidueV1(root, entry); err == nil {
		t.Fatal("generic residue remover accepted a retired TypeScript record")
	}
	err = removeChildRunInventoryResidueWithHooksV1(root, entry, childRunResidueRemovalHooksV1{
		allowLegacyTypeScriptRecordV1: true,
		afterFirstValidation: func(removePath string) error {
			if removePath != entry.Path {
				return errors.New("unexpected removal path")
			}
			if err := os.Rename(entry.Path, filepath.Join(base, "original-retired-source")); err != nil {
				return err
			}
			return os.WriteFile(entry.Path, replacement, 0o600)
		},
	})
	if err == nil {
		t.Fatal("retired TypeScript source removal accepted a replacement race")
	}
	body, readErr := os.ReadFile(entry.Path)
	if readErr != nil || !bytes.Equal(body, replacement) {
		t.Fatalf("failed retired-source removal deleted or changed the replacement: body=%q err=%v", body, readErr)
	}
}

func TestChildRunInventoryDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-child-runs")
	inventory, err := BuildChildRunInventoryV1(root)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := canonicalChildRunInventoryRootV1(root)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.RootExists || len(inventory.Entries) != 0 || inventory.Root != canonicalRoot {
		t.Fatalf("unexpected missing-root inventory: %#v", inventory)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only inventory created the missing root: %v", err)
	}
	if err := ValidateChildRunInventoryV1(root, inventory); err != nil {
		t.Fatalf("validate stable missing root: %v", err)
	}
}

type childRunDirectoryObjectForTest struct {
	Name string
	Mode os.FileMode
	Data string
}

func childRunDirectoryBytesForTest(t *testing.T, root string) []childRunDirectoryObjectForTest {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	objects := make([]childRunDirectoryObjectForTest, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		data := ""
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = string(body)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			data = target
		}
		objects = append(objects, childRunDirectoryObjectForTest{Name: entry.Name(), Mode: info.Mode(), Data: data})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Name < objects[j].Name })
	return objects
}

func childRunInventoryEntryByNameForTest(t *testing.T, inventory ChildRunInventoryV1, name string) ChildRunInventoryEntryV1 {
	t.Helper()
	for _, entry := range inventory.Entries {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("inventory entry %q not found: %#v", name, inventory.Entries)
	return ChildRunInventoryEntryV1{}
}
