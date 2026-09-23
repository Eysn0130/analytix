package runtimeapp

import (
	provider "analytix.local/runtime-go/internal/provider"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentProviderAuthorityIsolationAndRestart(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "provider-credentials")
	if err = os.Mkdir(root, 0700); err != nil {
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
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := openDevelopmentProviderAuthority(ctx, cfg, false); err == nil {
		t.Fatal("unsafe authority permissions accepted")
	}
}
