//go:build darwin && analytix_dev_credentials

package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
