package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWorkspaceStatusProbeReportsMissingAndNonGitDirectories(t *testing.T) {
	probe := NewWorkspaceStatusProbe()
	missing := filepath.Join(t.TempDir(), "missing")
	status := probe.WorkspaceStatus(context.Background(), missing)
	if status.Path != missing || status.Exists || status.IsGitRepository || status.IsDirty != nil || status.FileChangeCount != nil {
		t.Fatalf("missing workspace status mismatch: %#v", status)
	}

	dir := t.TempDir()
	status = probe.WorkspaceStatus(context.Background(), dir)
	if status.Path != dir || !status.Exists || status.IsGitRepository || status.IsDirty != nil || status.FileChangeCount != nil {
		t.Fatalf("non-git workspace status mismatch: %#v", status)
	}
}

func TestWorkspaceStatusProbeReportsGitDirtyState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	status := NewWorkspaceStatusProbe().WorkspaceStatus(context.Background(), dir)
	if !status.Exists || !status.IsGitRepository {
		t.Fatalf("expected git repository status: %#v", status)
	}
	if status.IsDirty == nil || !*status.IsDirty || status.FileChangeCount == nil || *status.FileChangeCount == 0 {
		t.Fatalf("expected dirty git repository status: %#v", status)
	}
}

func TestWorkspaceStatusProbeContainsMaliciousFSMonitorHook(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repository := initGitRepo(t)
	protectedRoot := t.TempDir()
	protectedFile := filepath.Join(protectedRoot, "sentinel.txt")
	if err := os.WriteFile(protectedFile, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	observerRoot := t.TempDir()
	attemptPath := filepath.Join(observerRoot, "fsmonitor-invoked")
	leakPath := filepath.Join(observerRoot, "fsmonitor-leak")
	hookPath := filepath.Join(observerRoot, "malicious-fsmonitor")
	hook := "#!/bin/sh\n" +
		"printf invoked > " + shellQuoteForTest(attemptPath) + "\n" +
		"if secret=$(/bin/cat " + shellQuoteForTest(protectedFile) + " 2>/dev/null); then\n" +
		"  printf '%s' \"$secret\" > " + shellQuoteForTest(leakPath) + "\n" +
		"fi\n" +
		"printf 'token\\000'\n"
	if err := os.WriteFile(hookPath, []byte(hook), 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "config", "core.fsmonitor", hookPath)
	runGit(t, repository, "config", "core.fsmonitorHookVersion", "2")

	status := NewWorkspaceStatusProbe(protectedRoot).WorkspaceStatus(context.Background(), repository)
	if !status.Exists || !status.IsGitRepository || status.IsDirty == nil {
		t.Fatalf("ordinary contained git status failed: %#v", status)
	}
	attempt, err := os.ReadFile(attemptPath)
	if err != nil || string(attempt) != "invoked" {
		t.Fatalf("malicious fsmonitor hook did not execute inside containment: body=%q err=%v", attempt, err)
	}
	if leak, err := os.ReadFile(leakPath); err == nil {
		t.Fatalf("malicious fsmonitor read protected data: %q", leak)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect fsmonitor leak: %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func TestWorkspaceStatusProbeDoesNotRevealProtectedMetadata(t *testing.T) {
	protected := t.TempDir()
	file := filepath.Join(protected, "synthetic-private.txt")
	if err := os.WriteFile(file, []byte("synthetic only"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(protected, alias); err != nil {
		t.Fatal(err)
	}
	probe := NewWorkspaceStatusProbe(protected)
	for _, path := range []string{protected, file, filepath.Join(protected, "missing"), alias, filepath.Join(alias, "synthetic-private.txt")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			status := probe.WorkspaceStatus(context.Background(), path)
			if status.Exists || status.IsGitRepository || status.HeadSHA != nil || status.Branch != nil || status.IsDirty != nil || status.FileChangeCount != nil {
				t.Fatal("protected existence or repository metadata escaped")
			}
		})
	}
	if status := probe.WorkspaceStatus(context.Background(), t.TempDir()); runtime.GOOS == "darwin" && !status.Exists {
		t.Fatal("ordinary non-Git directory became unavailable")
	}
}
