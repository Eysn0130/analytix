//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenExistingFileAuthorityIsReadOnlyAndStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authority", "final-answer-ed25519-v1.json")
	enrolled, err := OpenOrCreateFileAuthority(path, false)
	if err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeBody, beforeInfo, beforeEntries := existingAuthoritySnapshot(t, path)

	opened, err := openExistingFileAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	if opened.KeyID() != enrolled.KeyID() || !bytes.Equal(opened.PublicKey(), enrolled.PublicKey()) {
		t.Fatal("read-only authority open changed the enrolled identity")
	}
	message := []byte("existing-authority")
	signature, err := opened.Sign(context.Background(), message)
	if err != nil {
		t.Fatal(err)
	}
	if err := enrolled.VerifyTrusted(context.Background(), opened.KeyID(), opened.PublicKey(), message, signature); err != nil {
		t.Fatalf("read-only authority did not retain signing identity: %v", err)
	}
	afterBody, afterInfo, afterEntries := existingAuthoritySnapshot(t, path)
	if !bytes.Equal(beforeBody, afterBody) || beforeInfo.Mode() != afterInfo.Mode() ||
		beforeInfo.Size() != afterInfo.Size() || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) ||
		!reflect.DeepEqual(beforeEntries, afterEntries) {
		t.Fatal("read-only authority open mutated enrolled filesystem state")
	}
}

func TestOpenExistingFileAuthorityMissingStateNeverBootstraps(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "not-created", "final-answer-ed25519-v1.json")
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openExistingFileAuthority(path); err == nil {
		t.Fatal("missing authority was opened")
	}
	if _, err := os.Lstat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only open created a missing authority directory: %v", err)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(existingEntryNames(before), existingEntryNames(after)) {
		t.Fatal("read-only open changed the missing authority parent")
	}

	existingRoot := filepath.Join(root, "existing")
	if err := os.Mkdir(existingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := openExistingFileAuthority(filepath.Join(existingRoot, "missing.json")); err == nil {
		t.Fatal("missing authority file was opened")
	}
	entries, err := os.ReadDir(existingRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("read-only open populated an empty authority root: entries=%v err=%v", entries, err)
	}
}

func TestOpenExistingFileAuthorityRejectsUnsafePathTypeModeLinksAndResidueWithoutCleanup(t *testing.T) {
	t.Run("broad file permissions", func(t *testing.T) {
		path := existingEnrolledAuthorityPath(t)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, path)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("read-only open repaired broad file permissions: info=%v err=%v", info, err)
		}
	})

	t.Run("broad root permissions", func(t *testing.T) {
		path := existingEnrolledAuthorityPath(t)
		if err := os.Chmod(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, path)
		info, err := os.Stat(filepath.Dir(path))
		if err != nil || info.Mode().Perm() != 0o755 {
			t.Fatalf("read-only open repaired broad root permissions: info=%v err=%v", info, err)
		}
	})

	t.Run("target symlink", func(t *testing.T) {
		target := existingEnrolledAuthorityPath(t)
		root := filepath.Join(t.TempDir(), "linked-authority")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "key.json")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, path)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("read-only open replaced the target symlink: info=%v err=%v", info, err)
		}
	})

	t.Run("root symlink", func(t *testing.T) {
		target := existingEnrolledAuthorityPath(t)
		link := filepath.Join(t.TempDir(), "authority-link")
		if err := os.Symlink(filepath.Dir(target), link); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, filepath.Join(link, filepath.Base(target)))
	})

	t.Run("directory target", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "authority")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "key.json")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, path)
	})

	t.Run("fifo target", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "authority")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "key.json")
		if err := unix.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, path)
	})

	t.Run("multiple hard links", func(t *testing.T) {
		path := existingEnrolledAuthorityPath(t)
		otherRoot := filepath.Join(t.TempDir(), "other")
		if err := os.Mkdir(otherRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, filepath.Join(otherRoot, "linked.json")); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		assertExistingAuthorityRejected(t, path)
	})

	t.Run("recovery residue", func(t *testing.T) {
		path := existingEnrolledAuthorityPath(t)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		residue := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"-pre-rename.tmp")
		residueBody := []byte("incomplete")
		if err := os.WriteFile(residue, residueBody, 0o600); err != nil {
			t.Fatal(err)
		}
		assertExistingAuthorityRejected(t, path)
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, before) {
			t.Fatalf("read-only open changed enrolled authority: err=%v", err)
		}
		persistedResidue, err := os.ReadFile(residue)
		if err != nil || !bytes.Equal(persistedResidue, residueBody) {
			t.Fatalf("read-only open cleaned or changed recovery residue: body=%q err=%v", persistedResidue, err)
		}
	})
}

func TestOpenExistingFileAuthorityRejectsInvalidOrAmbiguousEncodingWithoutRewrite(t *testing.T) {
	mutations := map[string]func(t *testing.T, body []byte) []byte{
		"empty":               func(_ *testing.T, _ []byte) []byte { return nil },
		"oversized":           func(_ *testing.T, _ []byte) []byte { return bytes.Repeat([]byte("x"), maxAuthorityKeyBytes+1) },
		"invalid utf8":        func(_ *testing.T, _ []byte) []byte { return []byte{0xff} },
		"corrupt":             func(_ *testing.T, _ []byte) []byte { return []byte(`{"schemaVersion":1}`) },
		"trailing whitespace": func(_ *testing.T, body []byte) []byte { return append(body, '\n') },
		"duplicate field": func(_ *testing.T, body []byte) []byte {
			return bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1)
		},
		"padded base64": func(t *testing.T, body []byte) []byte {
			record := existingAuthorityRecord(t, body)
			record.PublicKey += "="
			return existingAuthorityJSON(t, record)
		},
		"public key with ignored newline": func(t *testing.T, body []byte) []byte {
			record := existingAuthorityRecord(t, body)
			record.PublicKey = record.PublicKey[:8] + "\n" + record.PublicKey[8:]
			return existingAuthorityJSON(t, record)
		},
		"private seed with ignored newline": func(t *testing.T, body []byte) []byte {
			record := existingAuthorityRecord(t, body)
			record.PrivateSeed = record.PrivateSeed[:8] + "\n" + record.PrivateSeed[8:]
			return existingAuthorityJSON(t, record)
		},
		"wrong seed length": func(t *testing.T, body []byte) []byte {
			record := existingAuthorityRecord(t, body)
			record.PrivateSeed = base64.RawURLEncoding.EncodeToString(make([]byte, 31))
			return existingAuthorityJSON(t, record)
		},
		"mismatched material": func(t *testing.T, body []byte) []byte {
			record := existingAuthorityRecord(t, body)
			publicKey, err := base64.RawURLEncoding.DecodeString(record.PublicKey)
			if err != nil {
				t.Fatal(err)
			}
			publicKey[0] ^= 0xff
			record.PublicKey = base64.RawURLEncoding.EncodeToString(publicKey)
			return existingAuthorityJSON(t, record)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			path := existingEnrolledAuthorityPath(t)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			invalid := mutate(t, original)
			if err := os.WriteFile(path, invalid, 0o600); err != nil {
				t.Fatal(err)
			}
			assertExistingAuthorityRejected(t, path)
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, invalid) {
				t.Fatalf("read-only open rewrote invalid authority: err=%v", err)
			}
		})
	}
}

func TestExistingPrivateAuthorityOwnershipValidationIsDeterministic(t *testing.T) {
	const hostUID = uint32(1000)
	root := unix.Stat_t{Mode: unix.S_IFDIR | 0o700, Uid: hostUID}
	if !existingPrivateAuthorityRootSafe(root, hostUID) {
		t.Fatal("safe host-owned root was rejected")
	}
	root.Uid++
	if existingPrivateAuthorityRootSafe(root, hostUID) {
		t.Fatal("foreign-owned root was accepted")
	}
	file := unix.Stat_t{Mode: unix.S_IFREG | 0o600, Nlink: 1, Uid: hostUID, Size: 1}
	if !existingPrivateAuthorityFileSafe(file, hostUID, maxAuthorityKeyBytes) {
		t.Fatal("safe host-owned file was rejected")
	}
	file.Uid++
	if existingPrivateAuthorityFileSafe(file, hostUID, maxAuthorityKeyBytes) {
		t.Fatal("foreign-owned file was accepted")
	}
}

func existingEnrolledAuthorityPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authority", "key.json")
	if _, err := OpenOrCreateFileAuthority(path, false); err != nil {
		t.Fatal(err)
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return realPath
}

func assertExistingAuthorityRejected(t *testing.T, path string) {
	t.Helper()
	if _, err := openExistingFileAuthority(path); err == nil {
		t.Fatal("unsafe or invalid existing authority was accepted")
	}
}

func existingAuthoritySnapshot(t *testing.T, path string) ([]byte, os.FileInfo, []string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	return body, info, existingEntryNames(entries)
}

func existingEntryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index := range entries {
		names[index] = entries[index].Name()
	}
	return names
}

func existingAuthorityRecord(t *testing.T, body []byte) fileAuthorityKey {
	t.Helper()
	var record fileAuthorityKey
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func existingAuthorityJSON(t *testing.T, record fileAuthorityKey) []byte {
	t.Helper()
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestOpenExistingFileAuthorityRejectsEmptyOrTraversalLikePath(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"", "   ", "relative/key.json", " " + filepath.Join(root, "key.json"), filepath.Join(root, "key.json") + " ",
		root + string(os.PathSeparator) + "." + string(os.PathSeparator) + "key.json",
		root + string(os.PathSeparator) + "child" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "key.json",
	} {
		if _, err := openExistingFileAuthority(path); err == nil {
			t.Fatalf("invalid authority path %q was accepted", path)
		}
	}
	path := existingEnrolledAuthorityPath(t)
	if _, err := openExistingFileAuthority(path); err != nil {
		t.Fatalf("canonical enrolled path was rejected: %v", err)
	}
}
