package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

func TestWorktreeManagerCreatesAndSummarizesSubagentWorktree(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread/parent",
		JobID:           "job-1",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if isolation.IsolationMode != "worktree" ||
		!strings.HasPrefix(isolation.WorktreeBranch, "codex/subagent/thread-parent/job-1") ||
		isolation.WorktreePath == "" ||
		isolation.WorktreePath == parent ||
		isolation.BaseCommit == "" ||
		isolation.MergeStatus != "not_requested" {
		t.Fatalf("isolation metadata mismatch: %#v", isolation)
	}
	if _, err := os.Stat(filepath.Join(isolation.WorktreePath, "README.md")); err != nil {
		t.Fatalf("worktree should contain repo files: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	summarized, err := manager.SummarizeSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("summarize worktree: %v", err)
	}
	if len(summarized.ChangedFiles) != 2 || !strings.Contains(summarized.DiffSummary, "changed files: 2") {
		t.Fatalf("changed files summary mismatch: %#v", summarized)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "initial\n" {
		t.Fatalf("child worktree write should not affect parent workspace: %q", string(parentReadme))
	}
}

func TestWorktreeManagerContainsPostCheckoutHookAndCreatesOrdinaryWorktree(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("packaged macOS process containment seam")
	}
	requireGit(t)
	parent := initGitRepo(t)
	protectedRoot := t.TempDir()
	protectedFile := filepath.Join(protectedRoot, "sentinel.txt")
	if err := os.WriteFile(protectedFile, []byte("must-not-escape"), 0o600); err != nil {
		t.Fatal(err)
	}
	observerRoot := t.TempDir()
	attemptPath := filepath.Join(observerRoot, "post-checkout-invoked")
	leakPath := filepath.Join(observerRoot, "post-checkout-leak")
	hookPath := filepath.Join(parent, ".git", "hooks", "post-checkout")
	hook := "#!/bin/sh\n" +
		"printf invoked > " + shellQuoteForTest(attemptPath) + "\n" +
		"if secret=$(/bin/cat " + shellQuoteForTest(protectedFile) + " 2>/dev/null); then\n" +
		"  printf '%s' \"$secret\" > " + shellQuoteForTest(leakPath) + "\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(hookPath, []byte(hook), 0o700); err != nil {
		t.Fatal(err)
	}

	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"), protectedRoot)
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-protected-hook",
	})
	if err != nil {
		t.Fatalf("ordinary contained worktree creation failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(isolation.WorktreePath, "README.md")); err != nil {
		t.Fatalf("contained worktree lost ordinary repository content: %v", err)
	}
	attempt, err := os.ReadFile(attemptPath)
	if err != nil || string(attempt) != "invoked" {
		t.Fatalf("post-checkout hook did not execute inside containment: body=%q err=%v", attempt, err)
	}
	if leak, err := os.ReadFile(leakPath); err == nil {
		t.Fatalf("post-checkout hook read protected data: %q", leak)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect post-checkout leak: %v", err)
	}
}

func TestWorktreeManagerRejectsDirtyParent(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(parent, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	_, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-1",
	})
	if err == nil || !strings.Contains(err.Error(), "clean parent") {
		t.Fatalf("dirty parent should be rejected, got %v", err)
	}
}

func TestWorktreeManagerAcceptAppliesCleanPatchWithoutCleanup(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-accept",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("child change\n"), 0o600); err != nil {
		t.Fatalf("write child README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "new.txt"), []byte("new file\n"), 0o600); err != nil {
		t.Fatalf("write child new file: %v", err)
	}
	isolation, err = manager.SummarizeSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("summarize worktree: %v", err)
	}
	isolation.MergeStatus = "review_requested"

	accepted, decision, err := manager.AcceptSubagentWorktree(context.Background(), ports.WorktreeAcceptRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-accept",
		MergeRequestID:  "merge-1",
		ApprovalID:      "approval-1",
		Isolation:       isolation,
	})
	if err != nil {
		t.Fatalf("accept worktree: %v", err)
	}
	if accepted.MergeStatus != "accepted" || decision.ApprovalID != "approval-1" || decision.AppliedPatchDigest == "" {
		t.Fatalf("accept metadata mismatch: isolation=%#v decision=%#v", accepted, decision)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "child change\n" {
		t.Fatalf("accept should apply tracked change to parent: %q", string(parentReadme))
	}
	parentNew, err := os.ReadFile(filepath.Join(parent, "new.txt"))
	if err != nil {
		t.Fatalf("read parent new file: %v", err)
	}
	if string(parentNew) != "new file\n" {
		t.Fatalf("accept should apply new file to parent: %q", string(parentNew))
	}
	if _, err := os.Stat(isolation.WorktreePath); err != nil {
		t.Fatalf("accept should not cleanup child worktree: %v", err)
	}
}

func TestWorktreeManagerAcceptRefreshesIsolationSummary(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-refresh-accept",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("reviewed change\n"), 0o600); err != nil {
		t.Fatalf("write reviewed child README: %v", err)
	}
	isolation, err = manager.SummarizeSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("summarize worktree: %v", err)
	}
	isolation.MergeStatus = "review_requested"
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("latest child change\n"), 0o600); err != nil {
		t.Fatalf("write latest child README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "late.txt"), []byte("late\n"), 0o600); err != nil {
		t.Fatalf("write late child file: %v", err)
	}

	accepted, decision, err := manager.AcceptSubagentWorktree(context.Background(), ports.WorktreeAcceptRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-refresh-accept",
		ApprovalID:      "approval-1",
		Isolation:       isolation,
	})
	if err != nil {
		t.Fatalf("accept worktree: %v", err)
	}
	if len(accepted.ChangedFiles) != 2 || len(decision.ChangedFiles) != 2 {
		t.Fatalf("accept should refresh changed files before applying: isolation=%#v decision=%#v", accepted.ChangedFiles, decision.ChangedFiles)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "latest child change\n" {
		t.Fatalf("accept should apply current child content: %q", string(parentReadme))
	}
	if _, err := os.Stat(filepath.Join(parent, "late.txt")); err != nil {
		t.Fatalf("accept should apply late child file: %v", err)
	}
}

func TestWorktreeManagerCleanupAcceptedWorktreeDoesNotTouchParentWorkspace(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-accepted-cleanup",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("accepted change\n"), 0o600); err != nil {
		t.Fatalf("write child README: %v", err)
	}
	isolation, err = manager.SummarizeSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("summarize worktree: %v", err)
	}
	isolation.MergeStatus = "review_requested"
	accepted, _, err := manager.AcceptSubagentWorktree(context.Background(), ports.WorktreeAcceptRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-accepted-cleanup",
		ApprovalID:      "approval-1",
		Isolation:       isolation,
	})
	if err != nil {
		t.Fatalf("accept worktree: %v", err)
	}
	receipt, err := manager.CleanupSubagentWorktree(context.Background(), accepted)
	if err != nil {
		t.Fatalf("cleanup accepted worktree: %v", err)
	}
	if !receipt.Removed {
		t.Fatalf("cleanup accepted worktree should remove owned worktree: %#v", receipt)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "accepted change\n" {
		t.Fatalf("cleanup should preserve accepted parent patch: %q", string(parentReadme))
	}
	if _, err := os.Stat(isolation.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("accepted worktree should be removed, stat err=%v", err)
	}
}

func TestWorktreeManagerAcceptRejectsDirtyParent(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-dirty-accept",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("child change\n"), 0o600); err != nil {
		t.Fatalf("write child README: %v", err)
	}
	isolation, err = manager.SummarizeSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("summarize worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(parent, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("dirty parent workspace: %v", err)
	}
	_, _, err = manager.AcceptSubagentWorktree(context.Background(), ports.WorktreeAcceptRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-dirty-accept",
		ApprovalID:      "approval-1",
		Isolation:       isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "clean parent") {
		t.Fatalf("dirty parent accept should reject: %v", err)
	}
}

func TestWorktreeManagerCleanupRemovesOnlySubagentWorktree(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-clean",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("child change\n"), 0o600); err != nil {
		t.Fatalf("write child worktree: %v", err)
	}
	receipt, err := manager.CleanupSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("cleanup worktree: %v", err)
	}
	if !receipt.Removed || receipt.WorktreePath != isolation.WorktreePath || receipt.Branch != isolation.WorktreeBranch {
		t.Fatalf("cleanup receipt mismatch: %#v", receipt)
	}
	if _, err := os.Stat(isolation.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree should be removed, stat err=%v", err)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "initial\n" {
		t.Fatalf("cleanup should not touch parent workspace: %q", string(parentReadme))
	}
}

func TestWorktreeManagerCleanupRejectsNonOwnedPathAndBranch(t *testing.T) {
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	if _, err := manager.CleanupSubagentWorktree(context.Background(), domainjob.WorktreeIsolation{
		IsolationMode:  "worktree",
		WorktreePath:   t.TempDir(),
		WorktreeBranch: "codex/subagent/thread/job",
	}); err == nil || !strings.Contains(err.Error(), "analytix-owned root") {
		t.Fatalf("cleanup should reject non-owned path: %v", err)
	}
	if _, err := manager.CleanupSubagentWorktree(context.Background(), domainjob.WorktreeIsolation{
		IsolationMode:  "worktree",
		WorktreePath:   filepath.Join(manager.Root, "thread", "job"),
		WorktreeBranch: "main",
	}); err == nil || !strings.Contains(err.Error(), "analytix-owned subagent branch") {
		t.Fatalf("cleanup should reject non-owned branch: %v", err)
	}
}

func TestWorktreeManagerConflictReportDoesNotTouchParentWorkspace(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-conflict-report",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(isolation.WorktreePath, "README.md"), []byte("child conflict\n"), 0o600); err != nil {
		t.Fatalf("write child README: %v", err)
	}
	isolation, err = manager.SummarizeSubagentWorktree(context.Background(), isolation)
	if err != nil {
		t.Fatalf("summarize worktree: %v", err)
	}
	isolation.MergeStatus = "conflicted"
	parentHead := gitOutput(t, parent, "rev-parse", "HEAD")

	reported, report, err := manager.ReportSubagentWorktreeConflict(context.Background(), ports.WorktreeConflictReportRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-conflict-report",
		Isolation:       isolation,
	})
	if err != nil {
		t.Fatalf("report conflict: %v", err)
	}
	if reported.MergeStatus != "conflicted" || report.ParentHeadAtConflict != parentHead || report.TouchedParentWorkspace {
		t.Fatalf("conflict report metadata mismatch: isolation=%#v report=%#v", reported, report)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "initial\n" || strings.TrimSpace(gitOutput(t, parent, "status", "--porcelain=v1", "--untracked-files=all")) != "" {
		t.Fatalf("conflict report should not touch parent workspace: readme=%q status=%q", string(parentReadme), gitOutput(t, parent, "status", "--porcelain=v1", "--untracked-files=all"))
	}
}

func TestWorktreeManagerRepairCheckRejectsUnsafeInputs(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	head := gitOutput(t, parent, "rev-parse", "HEAD")
	isolation := domainjob.WorktreeIsolation{IsolationMode: "worktree", WorktreePath: filepath.Join(manager.Root, "thread", "job"), WorktreeBranch: "codex/subagent/thread/job", BaseCommit: head, MergeStatus: "conflicted"}

	_, _, err := manager.CheckSubagentRepairPatch(context.Background(), ports.WorktreeRepairCheckRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-oversized",
		ConflictReportID:   "conflict-1",
		ExpectedParentHead: head,
		RepairPatch:        strings.Repeat("x", maxSubagentRepairPatchBytes+1),
		Isolation:          isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "repair patch exceeds") {
		t.Fatalf("oversized patch should reject: %v", err)
	}

	_, _, err = manager.CheckSubagentRepairPatch(context.Background(), ports.WorktreeRepairCheckRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-unsafe",
		ConflictReportID:   "conflict-1",
		ExpectedParentHead: head,
		RepairPatch:        "diff --git a/../evil b/../evil\n--- a/../evil\n+++ b/../evil\n@@ -1 +1 @@\n-a\n+b\n",
		Isolation:          isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("unsafe patch path should reject: %v", err)
	}

	_, _, err = manager.CheckSubagentRepairPatch(context.Background(), ports.WorktreeRepairCheckRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-stale",
		ConflictReportID:   "conflict-1",
		ExpectedParentHead: "stale-head",
		RepairPatch:        readmePatch("initial", "repair"),
		Isolation:          isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "parent HEAD") {
		t.Fatalf("stale parent head should reject: %v", err)
	}
}

func TestWorktreeManagerRepairCheckDryRunDoesNotTouchParentWorkspace(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	head := gitOutput(t, parent, "rev-parse", "HEAD")
	isolation := domainjob.WorktreeIsolation{IsolationMode: "worktree", WorktreePath: filepath.Join(manager.Root, "thread", "job"), WorktreeBranch: "codex/subagent/thread/job", BaseCommit: head, MergeStatus: "conflicted"}

	checked, review, err := manager.CheckSubagentRepairPatch(context.Background(), ports.WorktreeRepairCheckRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-repair-check",
		ConflictReportID:   "conflict-1",
		ExpectedParentHead: head,
		RepairPatch:        readmePatch("initial", "repair"),
		Isolation:          isolation,
	})
	if err != nil {
		t.Fatalf("repair check: %v", err)
	}
	if checked.MergeStatus != "repair_checked" || review.DryRunStatus != "clean" || review.RepairPatchDigest == "" || len(review.ChangedFiles) != 1 {
		t.Fatalf("clean repair check metadata mismatch: isolation=%#v review=%#v", checked, review)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "initial\n" {
		t.Fatalf("repair check should only dry-run: %q", string(parentReadme))
	}

	rejected, review, err := manager.CheckSubagentRepairPatch(context.Background(), ports.WorktreeRepairCheckRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-repair-check-conflict",
		ConflictReportID:   "conflict-1",
		ExpectedParentHead: head,
		RepairPatch:        readmePatch("not-the-current-line", "repair"),
		Isolation:          isolation,
	})
	if err != nil {
		t.Fatalf("conflicting repair check should persist rejected review without returning error: %v", err)
	}
	if rejected.MergeStatus != "repair_rejected" || review.DryRunStatus != "conflicted" || review.RejectedReason == "" {
		t.Fatalf("conflicting repair check metadata mismatch: isolation=%#v review=%#v", rejected, review)
	}
	parentReadme, err = os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "initial\n" {
		t.Fatalf("conflicting repair check should not apply patch: %q", string(parentReadme))
	}
}

func TestWorktreeManagerRepairAcceptAppliesPatchWithoutCleanupOrCommit(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	isolation, err := manager.CreateSubagentWorktree(context.Background(), ports.WorktreeCreateRequest{
		ParentWorkspace: parent,
		ParentThreadID:  "thread-parent",
		JobID:           "job-repair-accept",
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	head := gitOutput(t, parent, "rev-parse", "HEAD")
	patch := readmePatch("initial", "repair accepted")
	accepted, decision, err := manager.AcceptSubagentRepairPatch(context.Background(), ports.WorktreeRepairAcceptRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-repair-accept",
		RepairReviewID:     "repair-review-1",
		ApprovalID:         "approval-1",
		ExpectedParentHead: head,
		RepairPatch:        patch,
		Isolation:          isolation,
	})
	if err != nil {
		t.Fatalf("accept repair patch: %v", err)
	}
	if accepted.MergeStatus != "repair_accepted" || decision.ApprovalID != "approval-1" || decision.AppliedPatchDigest == "" {
		t.Fatalf("repair accept metadata mismatch: isolation=%#v decision=%#v", accepted, decision)
	}
	parentReadme, err := os.ReadFile(filepath.Join(parent, "README.md"))
	if err != nil {
		t.Fatalf("read parent README: %v", err)
	}
	if string(parentReadme) != "repair accepted\n" {
		t.Fatalf("repair accept should apply patch to parent: %q", string(parentReadme))
	}
	if got := gitOutput(t, parent, "rev-parse", "HEAD"); got != head || decision.ParentHeadBefore != head || decision.ParentHeadAfter != head {
		t.Fatalf("repair accept should not commit: head=%s before=%s after=%s", got, decision.ParentHeadBefore, decision.ParentHeadAfter)
	}
	if _, err := os.Stat(isolation.WorktreePath); err != nil {
		t.Fatalf("repair accept should not cleanup child worktree: %v", err)
	}
}

func TestWorktreeManagerRepairAcceptRejectsUnsafeState(t *testing.T) {
	requireGit(t)
	parent := initGitRepo(t)
	manager := NewWorktreeManager(filepath.Join(t.TempDir(), "subagent-worktrees"))
	head := gitOutput(t, parent, "rev-parse", "HEAD")
	isolation := domainjob.WorktreeIsolation{IsolationMode: "worktree", WorktreePath: filepath.Join(manager.Root, "thread", "job"), WorktreeBranch: "codex/subagent/thread/job", BaseCommit: head, MergeStatus: "repair_checked"}
	patch := readmePatch("initial", "repair")

	_, _, err := manager.AcceptSubagentRepairPatch(context.Background(), ports.WorktreeRepairAcceptRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-no-approval",
		RepairReviewID:     "repair-review-1",
		ExpectedParentHead: head,
		RepairPatch:        patch,
		Isolation:          isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "approval_id") {
		t.Fatalf("repair accept without approval should reject: %v", err)
	}

	_, _, err = manager.AcceptSubagentRepairPatch(context.Background(), ports.WorktreeRepairAcceptRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-stale",
		RepairReviewID:     "repair-review-1",
		ApprovalID:         "approval-1",
		ExpectedParentHead: "stale-head",
		RepairPatch:        patch,
		Isolation:          isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "parent HEAD") {
		t.Fatalf("repair accept with stale parent head should reject: %v", err)
	}

	if err := os.WriteFile(filepath.Join(parent, "dirty.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatalf("dirty parent workspace: %v", err)
	}
	_, _, err = manager.AcceptSubagentRepairPatch(context.Background(), ports.WorktreeRepairAcceptRequest{
		ParentWorkspace:    parent,
		ParentThreadID:     "thread-parent",
		JobID:              "job-dirty",
		RepairReviewID:     "repair-review-1",
		ApprovalID:         "approval-1",
		ExpectedParentHead: head,
		RepairPatch:        patch,
		Isolation:          isolation,
	})
	if err == nil || !strings.Contains(err.Error(), "clean parent") {
		t.Fatalf("repair accept with dirty parent should reject: %v", err)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func readmePatch(from string, to string) string {
	return "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-" + from + "\n+" + to + "\n"
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(output))
	}
	return strings.TrimSpace(string(output))
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("initial\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}
