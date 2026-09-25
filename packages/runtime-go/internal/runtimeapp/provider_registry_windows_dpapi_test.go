//go:build windows

package runtimeapp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	provider "analytix.local/runtime-go/internal/provider"
)

func TestWindowsProviderRegistryDPAPISaveRestartAndMissingKeyPreservation(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = connectProviderRegistryTestWinnerV1(t, ctx, authority.Manager(), "provider-windows-dpapi", "synthetic-windows-key")
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dataDir, "private", "provider-secrets", "credentials.v1.json")
	before, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := newProviderRegistryExecutionResolverV1(restarted.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.Config.APIKey != "synthetic-windows-key" {
		t.Fatalf("Windows DPAPI Provider credential did not survive restart: %v", err)
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(secretPath), "master-key", "master-key.dpapi")); err != nil {
		t.Fatal(err)
	}
	if reopened, err := openProviderRegistryAuthorityV1(ctx, dataDir); err == nil {
		_ = reopened.Close()
		t.Fatal("missing DPAPI master key was regenerated")
	}
	if after, err := os.ReadFile(secretPath); err != nil || !bytes.Equal(before, after) {
		t.Fatal("missing DPAPI master key changed committed ciphertext")
	}
}
