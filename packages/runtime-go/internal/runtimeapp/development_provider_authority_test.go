package runtimeapp

import (
	provider "analytix.local/runtime-go/internal/provider"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func developmentProviderTestRoot(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		parent, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(parent, 0700); err != nil {
			t.Fatal(err)
		}
		return parent
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(home, "analytix-provider-test-"+hex.EncodeToString(suffix[:]))
	createWindowsDevelopmentTestDirectory(t, parent)
	t.Cleanup(func() {
		if err := os.RemoveAll(parent); err != nil {
			t.Error(err)
		}
	})
	return parent
}

func createWindowsDevelopmentTestDirectory(t *testing.T, directory string) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "scripts", "windows-development-profile.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", script,
		"-Directory", directory, "-Create").CombinedOutput()
	if err != nil {
		t.Fatalf("Windows development fixture ACL creation failed: %v: %s", err, output)
	}
}

func TestDevelopmentProviderAuthorityIsolationAndRestart(t *testing.T) {
	parent := developmentProviderTestRoot(t)
	root := filepath.Join(parent, "provider-credentials")
	if runtime.GOOS == "windows" {
		createWindowsDevelopmentTestDirectory(t, root)
	} else if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DevelopmentProviderAuthorityDir: root, DataDir: filepath.Join(parent, "task-a"), UserDataDir: filepath.Join(parent, "ui-a")}
	ctx := context.Background()
	if !developmentProviderAuthorityEnabled {
		if _, err := openDevelopmentProviderAuthority(ctx, cfg, false); err == nil {
			t.Fatal("production binary accepted development authority")
		}
		return
	}
	planned, err := openDevelopmentProviderAuthority(ctx, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = planned.Close()
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("semantic planning mutated shared authority")
	}
	first, err := openDevelopmentProviderAuthority(ctx, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	winner := connectProviderRegistryTestWinnerV1(t, ctx, first.Manager(), "provider-development", "synthetic-dev-credential")
	first.Close()
	cfg.DataDir = filepath.Join(parent, "task-b")
	cfg.UserDataDir = filepath.Join(parent, "ui-b")
	next, err := openDevelopmentProviderAuthority(ctx, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	resolved, err := newProviderRegistryExecutionResolverV1(next.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.Config.APIKey != "synthetic-dev-credential" || resolved.Authority.ProviderCredentialRef != winner.CredentialRef {
		t.Fatal("new task did not reuse committed credential reference")
	}
	resolved.Config.APIKey = ""
	cfg.DarwinSecretStoreKeychainDBPath = "/synthetic/locked.keychain-db"
	if _, err := openDevelopmentProviderAuthority(ctx, cfg, false); err == nil {
		t.Fatal("QA and shared development authorities mixed")
	}
	cfg.DarwinSecretStoreKeychainDBPath = ""
	if runtime.GOOS == "windows" {
		// os.Chmod does not change a Windows DACL. Native ACL rejection is
		// exercised by the Windows Secret Store tests.
		return
	}
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := openDevelopmentProviderAuthority(ctx, cfg, false); err == nil {
		t.Fatal("unsafe authority permissions accepted")
	}
}

func TestDevelopmentProviderAuthorityMissingMasterKeyPreservesCommittedCredential(t *testing.T) {
	if runtime.GOOS != "darwin" || !developmentProviderAuthorityEnabled {
		t.Skip("requires the macOS source-development file authority")
	}
	for _, markerPresent := range []bool{true, false} {
		name := "marker-retained"
		if !markerPresent {
			name = "marker-missing"
		}
		t.Run(name, func(t *testing.T) {
			parent := developmentProviderTestRoot(t)
			root := filepath.Join(parent, "provider-credentials")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			cfg := Config{DevelopmentProviderAuthorityDir: root, DataDir: filepath.Join(parent, "task"), UserDataDir: filepath.Join(parent, "ui")}
			ctx := context.Background()
			authority, err := openDevelopmentProviderAuthority(ctx, cfg, false)
			if err != nil {
				t.Fatal(err)
			}
			_ = connectProviderRegistryTestWinnerV1(t, ctx, authority.Manager(), "provider-development-missing-key", "synthetic-development-key")
			if err := authority.Close(); err != nil {
				t.Fatal(err)
			}
			secretPath := filepath.Join(root, "private", "provider-secrets", "credentials.v1.json")
			before, err := os.ReadFile(secretPath)
			if err != nil {
				t.Fatal(err)
			}
			masterPath := filepath.Join(filepath.Dir(secretPath), "master-key", "master.key")
			markerPath := filepath.Join(filepath.Dir(secretPath), "master-key", "authority.v1")
			if err := os.Remove(masterPath); err != nil {
				t.Fatal(err)
			}
			if !markerPresent {
				if err := os.Remove(markerPath); err != nil {
					t.Fatal(err)
				}
			}
			if reopened, err := openDevelopmentProviderAuthority(ctx, cfg, false); err == nil {
				_ = reopened.Close()
				t.Fatal("development recovery accepted a missing master key")
			}
			if _, err := os.Lstat(masterPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("development recovery created a replacement master key over committed ciphertext")
			}
			if !markerPresent {
				if _, err := os.Lstat(markerPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed development recovery recreated the missing authority marker")
				}
			}
			if after, err := os.ReadFile(secretPath); err != nil || !bytes.Equal(before, after) {
				t.Fatal("development recovery changed committed ciphertext")
			}
		})
	}
}
