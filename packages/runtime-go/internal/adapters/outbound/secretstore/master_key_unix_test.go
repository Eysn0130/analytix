//go:build darwin || linux

package secretstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestFallbackMasterKeyCreatesExclusivePrivateStableKey(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "master-key")
	path := filepath.Join(directory, "master.key")
	provider := newFallbackMasterKeyProvider(path, bytes.NewReader(bytes.Repeat([]byte{0x31}, masterKeySize)))
	first, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("first LoadOrCreate() error = %v", err)
	}
	defer clearBytes(first)
	second, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatalf("second LoadOrCreate() error = %v", err)
	}
	defer clearBytes(second)
	if !bytes.Equal(first, second) {
		t.Fatal("fallback provider did not return its original key")
	}
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		t.Fatalf("Lstat(directory) error = %v", err)
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, want private 0700 directory", directoryInfo.Mode())
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(key) error = %v", err)
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v, want private 0600 regular file", fileInfo.Mode())
	}
	committed := mustReadFile(t, path)
	if !bytes.Equal(committed, first) {
		t.Fatal("committed fallback key differs from readback")
	}
}

func TestFallbackMasterKeyConcurrentInitializationRereadsWinner(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "master-key", "master.key")
	const workers = 12
	results := make(chan []byte, workers)
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)
		go func(value byte) {
			defer waitGroup.Done()
			provider := newFallbackMasterKeyProvider(path, bytes.NewReader(bytes.Repeat([]byte{value}, masterKeySize)))
			key, err := provider.LoadOrCreate(context.Background())
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- key
		}(byte(worker + 1))
	}
	waitGroup.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent LoadOrCreate() error = %v", err)
	}
	var winner []byte
	for key := range results {
		if winner == nil {
			winner = bytes.Clone(key)
		}
		if !bytes.Equal(winner, key) {
			t.Fatal("concurrent initializer did not re-read the winner")
		}
		clearBytes(key)
	}
	defer clearBytes(winner)
	if committed := mustReadFile(t, path); !bytes.Equal(committed, winner) {
		t.Fatal("committed concurrent winner differs from all returned keys")
	}
}

func TestFallbackMasterKeyMalformedPermissionAndSymlinkStateFailWithoutRewrite(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		setup func(t *testing.T, path string) ([]byte, os.FileMode)
	}{
		{
			name: "short existing key",
			setup: func(t *testing.T, path string) ([]byte, os.FileMode) {
				content := bytes.Repeat([]byte{0x21}, masterKeySize-1)
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				return content, 0o600
			},
		},
		{
			name: "world-readable existing key",
			setup: func(t *testing.T, path string) ([]byte, os.FileMode) {
				content := bytes.Repeat([]byte{0x22}, masterKeySize)
				if err := os.WriteFile(path, content, 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatalf("Chmod() error = %v", err)
				}
				return content, 0o644
			},
		},
		{
			name: "symlink key",
			setup: func(t *testing.T, path string) ([]byte, os.FileMode) {
				content := bytes.Repeat([]byte{0x23}, masterKeySize)
				target := path + ".target"
				if err := os.WriteFile(target, content, 0o600); err != nil {
					t.Fatalf("WriteFile(target) error = %v", err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatalf("Symlink() error = %v", err)
				}
				return content, os.ModeSymlink
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "master-key")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatalf("Mkdir() error = %v", err)
			}
			path := filepath.Join(directory, "master.key")
			before, wantMode := testCase.setup(t, path)
			provider := newFallbackMasterKeyProvider(path, bytes.NewReader(bytes.Repeat([]byte{0x55}, masterKeySize)))
			key, err := provider.LoadOrCreate(context.Background())
			clearBytes(key)
			if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatalf("LoadOrCreate() error = %v, want unavailable", err)
			}
			info, statErr := os.Lstat(path)
			if statErr != nil {
				t.Fatalf("Lstat() error = %v", statErr)
			}
			if wantMode == os.ModeSymlink {
				if info.Mode()&os.ModeSymlink == 0 {
					t.Fatal("provider replaced the existing symlink")
				}
				targetContent := mustReadFile(t, path+".target")
				if !bytes.Equal(before, targetContent) {
					t.Fatal("provider changed the symlink target bytes")
				}
				return
			}
			if info.Mode().Perm() != wantMode {
				t.Fatalf("existing mode = %o, want preserved %o", info.Mode().Perm(), wantMode)
			}
			if after := mustReadFile(t, path); !bytes.Equal(before, after) {
				t.Fatal("provider rewrote malformed or permission-mismatched key bytes")
			}
		})
	}
}
