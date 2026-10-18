package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func TestLegacyTypeScriptLineageSourceReadsOnlyThreeSnapshotSources(t *testing.T) {
	authority, root := legacyLineageSourceRootForTest(t)
	for _, sourceName := range []string{"thread.json", "metadata.jsonl", "messages.jsonl"} {
		t.Run(sourceName, func(t *testing.T) {
			body := []byte("{\"source\":\"" + sourceName + "\"}\n")
			expected := writeLegacyLineageSourceForTest(t, root, sourceName, body)
			read, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", sourceName, expected)
			if err != nil || !present || string(read) != string(body) {
				t.Fatalf("source read: present=%t body=%q err=%v", present, read, err)
			}
		})
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{root, filepath.Join(root, "threads"), filepath.Join(root, "threads", "parent")} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Fatalf("ordinary directory mode changed: path=%s err=%v", path, err)
			}
		}
		info, err := os.Stat(filepath.Join(root, "threads", "parent", "thread.json"))
		if err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("ordinary file mode changed: err=%v", err)
		}
	}
}

func TestLegacyTypeScriptLineageSourceRejectsInvalidAuthority(t *testing.T) {
	authority, root := legacyLineageSourceRootForTest(t)
	valid := writeLegacyLineageSourceForTest(t, root, "thread.json", []byte("{}\n"))
	for _, test := range []struct {
		name       string
		threadID   string
		sourceName string
		expected   EntryRecord
	}{
		{name: "invalid thread", threadID: "../parent", sourceName: "thread.json", expected: valid},
		{name: "empty thread", sourceName: "thread.json", expected: valid},
		{name: "unknown source", threadID: "parent", sourceName: "other.json", expected: valid},
		{name: "source traversal", threadID: "parent", sourceName: "../thread.json", expected: valid},
		{name: "zero expected", threadID: "parent", sourceName: "thread.json"},
		{name: "wrong path", threadID: "parent", sourceName: "thread.json", expected: EntryRecord{Path: "data/threads/parent/thread.json", Type: domainstartup.ManagedEntryTypeAbsent}},
		{name: "implicit absence", threadID: "parent", sourceName: "thread.json", expected: EntryRecord{Path: valid.Path}},
		{name: "nonzero absence", threadID: "parent", sourceName: "thread.json", expected: EntryRecord{Path: valid.Path, Type: domainstartup.ManagedEntryTypeAbsent, Mode: 0o644}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), test.threadID, test.sourceName, test.expected)
			if err == nil || present || body != nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid source authority: present=%t body=%q err=%v", present, body, err)
			}
		})
	}
}

func TestLegacyTypeScriptLineageSourceRejectsSnapshotDrift(t *testing.T) {
	for _, mutation := range []string{"mode", "size", "sha", "content", "deleted"} {
		t.Run(mutation, func(t *testing.T) {
			authority, root := legacyLineageSourceRootForTest(t)
			expected := writeLegacyLineageSourceForTest(t, root, "thread.json", []byte("{}\n"))
			path := filepath.Join(root, "threads", "parent", "thread.json")
			switch mutation {
			case "mode":
				expected.Mode ^= 0o200
			case "size":
				expected.Size++
			case "sha":
				expected.SHA256 = hex.EncodeToString(make([]byte, sha256.Size))
			case "content":
				if err := os.WriteFile(path, []byte("[]\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			body, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", "thread.json", expected)
			if err == nil || present || body != nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("snapshot drift: present=%t body=%q err=%v", present, body, err)
			}
		})
	}
}

func TestLegacyTypeScriptLineageSourceVerifiesExplicitAbsence(t *testing.T) {
	for _, appeared := range []string{"file", "directory", "symlink"} {
		t.Run(appeared, func(t *testing.T) {
			authority, root := legacyLineageSourceRootForTest(t)
			expected := EntryRecord{Path: "durable/threads/parent/messages.jsonl", Type: domainstartup.ManagedEntryTypeAbsent}
			body, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", "messages.jsonl", expected)
			if err != nil || present || body != nil {
				t.Fatalf("explicit source absence: present=%t body=%q err=%v", present, body, err)
			}
			path := filepath.Join(root, "threads", "parent", "messages.jsonl")
			switch appeared {
			case "file":
				if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(t.TempDir(), "missing-target")
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			body, present, err = authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", "messages.jsonl", expected)
			if err == nil || present || body != nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("absent source appeared: present=%t body=%q err=%v", present, body, err)
			}
		})
	}
}

func TestLegacyTypeScriptLineageSourceRejectsAncestorSymlink(t *testing.T) {
	for _, ancestor := range []string{"threads", "parent"} {
		t.Run(ancestor, func(t *testing.T) {
			authority, root := legacyLineageSourceRootForTest(t)
			expected := writeLegacyLineageSourceForTest(t, root, "thread.json", []byte("{}\n"))
			path := filepath.Join(root, "threads")
			if ancestor == "parent" {
				path = filepath.Join(path, "parent")
			}
			retired := path + "-retired"
			if err := os.Rename(path, retired); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(retired, path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			for _, entry := range []EntryRecord{expected, {Path: "durable/threads/parent/messages.jsonl", Type: domainstartup.ManagedEntryTypeAbsent}} {
				sourceName := filepath.Base(entry.Path)
				body, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", sourceName, entry)
				if err == nil || present || body != nil || errors.Is(err, os.ErrNotExist) {
					t.Fatalf("ancestor symlink: present=%t body=%q err=%v", present, body, err)
				}
			}
		})
	}
}

func TestLegacyTypeScriptLineageSourceHonors64MiBFileBound(t *testing.T) {
	authority, root := legacyLineageSourceRootForTest(t)
	path := filepath.Join(root, "threads", "parent", "messages.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(file.Truncate(domainstartup.MaxSemanticManagedFileBytesV1), file.Close()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(hasher, file)
	if err := errors.Join(copyErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	expected := EntryRecord{
		Path: "durable/threads/parent/messages.jsonl", Type: domainstartup.ManagedEntryTypeFile,
		Mode: uint32(info.Mode()), Size: info.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}
	body, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", "messages.jsonl", expected)
	if err != nil || !present || int64(len(body)) != domainstartup.MaxSemanticManagedFileBytesV1 {
		t.Fatalf("64 MiB source: present=%t bytes=%d err=%v", present, len(body), err)
	}
	clear(body)
	expected.Size++
	if body, present, err := authority.ReadLegacyTypeScriptLineageSourceV1(context.Background(), "parent", "messages.jsonl", expected); err == nil || present || body != nil {
		t.Fatalf("oversized source authority: present=%t bytes=%d err=%v", present, len(body), err)
	}
}

func legacyLineageSourceRootForTest(t *testing.T) (*RootAuthority, string) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("secure managed reader is unsupported")
	}
	root := filepath.Join(t.TempDir(), "durable")
	threadDir := filepath.Join(root, "threads", "parent")
	if err := os.MkdirAll(threadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, filepath.Dir(threadDir), threadDir} {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	authority, err := FreezeRootAuthority(RootSet{DataDir: root, DurableDir: root})
	if err != nil {
		t.Fatal(err)
	}
	roots, ok := authority.Roots()
	if !ok {
		t.Fatal("fixture root authority is unavailable")
	}
	return authority, roots.DurableDir
}

func writeLegacyLineageSourceForTest(t *testing.T, root, sourceName string, body []byte) EntryRecord {
	t.Helper()
	path := filepath.Join(root, "threads", "parent", sourceName)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return EntryRecord{
		Path: "durable/threads/parent/" + sourceName, Type: domainstartup.ManagedEntryTypeFile,
		Mode: uint32(info.Mode()), Size: info.Size(), SHA256: hex.EncodeToString(digest[:]),
	}
}
