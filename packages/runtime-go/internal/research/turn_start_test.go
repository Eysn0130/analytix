package research

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspacePreparationBeforeFreezeV1ReturnsOnlyForResearch(t *testing.T) {
	if got := WorkspacePreparationBeforeFreezeV1("ordinary turn"); got != nil {
		t.Fatal("ordinary prompt acquired a pre-freeze workspace effect")
	}
	if got := WorkspacePreparationBeforeFreezeV1("/goal --research ordinary topic"); got == nil {
		t.Fatal("research prompt lost its pre-freeze workspace preparation")
	}
}

func TestPrepareWorkspaceBeforeFreezeMaterializesOnlyResearchRoot(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(parent, "missing-research-workspace")
	if err := PrepareWorkspaceBeforeFreeze(context.Background(), "/goal --research inspect authority", workspace); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		t.Fatalf("research workspace root was not created: info=%#v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".analytix")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-freeze preparation created research or case state: %v", err)
	}
}

func TestPrepareWorkspaceBeforeFreezeDoesNotCreateForOrdinaryTurn(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(parent, "ordinary-workspace")
	if err := PrepareWorkspaceBeforeFreeze(context.Background(), "ordinary turn", workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary turn unexpectedly created a workspace: %v", err)
	}
}

func TestPrepareWorkspaceBeforeFreezeHonorsCancellation(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(parent, "cancelled-workspace")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := PrepareWorkspaceBeforeFreeze(ctx, "/goal --research cancelled", workspace); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation did not fail closed: %v", err)
	}
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled preparation created a workspace: %v", err)
	}
}
