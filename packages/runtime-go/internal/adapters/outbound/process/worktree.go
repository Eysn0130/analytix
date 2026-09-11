package process

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

const maxSubagentAcceptPatchBytes = 20 * 1024 * 1024
const maxSubagentRepairPatchBytes = 2 * 1024 * 1024
const maxSubagentRepairPatchFiles = 200

type WorktreeManager struct {
	Root              string
	CommandProbe      ports.CommandProbe
	protectedReadDirs []string
	Timeout           time.Duration
}

func NewWorktreeManager(root string, protectedReadDirs ...string) WorktreeManager {
	return WorktreeManager{
		Root:              root,
		CommandProbe:      NewCommandProbe(protectedReadDirs...),
		protectedReadDirs: append([]string(nil), protectedReadDirs...),
		Timeout:           10 * time.Second,
	}
}

func (m WorktreeManager) CreateSubagentWorktree(ctx context.Context, request ports.WorktreeCreateRequest) (domainjob.WorktreeIsolation, error) {
	parentWorkspace, err := filepath.Abs(strings.TrimSpace(request.ParentWorkspace))
	if err != nil || strings.TrimSpace(request.ParentWorkspace) == "" {
		return domainjob.WorktreeIsolation{}, errors.New("parent workspace is required for worktree isolation")
	}
	if ok, err := m.gitBool(ctx, parentWorkspace, "rev-parse", "--is-inside-work-tree"); err != nil || !ok {
		return domainjob.WorktreeIsolation{}, errors.New("parent workspace must be a git worktree")
	}
	status, err := m.gitOutput(ctx, parentWorkspace, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return domainjob.WorktreeIsolation{}, fmt.Errorf("inspect parent git status: %w", err)
	}
	conflicts, err := m.gitOutput(ctx, parentWorkspace, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return domainjob.WorktreeIsolation{}, fmt.Errorf("inspect parent git conflicts: %w", err)
	}
	if strings.TrimSpace(status) != "" || strings.TrimSpace(conflicts) != "" {
		return domainjob.WorktreeIsolation{}, errors.New("parent workspace has tracked, untracked, or conflicted changes; worktree isolation requires a clean parent")
	}
	baseCommit, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(baseCommit) == "" {
		return domainjob.WorktreeIsolation{}, errors.New("parent workspace must have a valid HEAD commit")
	}
	branch := subagentWorktreeBranch(request.ParentThreadID, request.JobID)
	if branch == "codex/subagent//" {
		return domainjob.WorktreeIsolation{}, errors.New("parent thread id and job id are required for worktree isolation")
	}
	if exists, _ := m.gitCommandSucceeds(ctx, parentWorkspace, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); exists {
		return domainjob.WorktreeIsolation{}, fmt.Errorf("subagent worktree branch already exists: %s", branch)
	}
	root, worktreePath, err := m.worktreePath(request.ParentThreadID, request.JobID)
	if err != nil {
		return domainjob.WorktreeIsolation{}, err
	}
	if err := ensurePathInside(root, worktreePath); err != nil {
		return domainjob.WorktreeIsolation{}, err
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return domainjob.WorktreeIsolation{}, fmt.Errorf("subagent worktree path already exists: %s", worktreePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainjob.WorktreeIsolation{}, err
	}
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o700); err != nil {
		return domainjob.WorktreeIsolation{}, err
	}
	if _, err := m.gitOutput(ctx, parentWorkspace, "worktree", "add", "-b", branch, worktreePath, strings.TrimSpace(baseCommit)); err != nil {
		return domainjob.WorktreeIsolation{}, fmt.Errorf("create subagent worktree: %w", err)
	}
	currentCommit, err := m.gitOutput(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil {
		currentCommit = strings.TrimSpace(baseCommit)
	}
	return domainjob.WorktreeIsolation{
		IsolationMode:  string(domainjob.IsolationWorktree),
		WorktreePath:   worktreePath,
		WorktreeBranch: branch,
		BaseCommit:     strings.TrimSpace(baseCommit),
		CurrentCommit:  strings.TrimSpace(currentCommit),
		MergeStatus:    "not_requested",
	}, nil
}

func (m WorktreeManager) SummarizeSubagentWorktree(ctx context.Context, isolation domainjob.WorktreeIsolation) (domainjob.WorktreeIsolation, error) {
	if strings.TrimSpace(isolation.IsolationMode) != string(domainjob.IsolationWorktree) {
		return isolation, nil
	}
	worktreePath := strings.TrimSpace(isolation.WorktreePath)
	if worktreePath == "" {
		return isolation, errors.New("worktree path is required")
	}
	root, _, err := m.worktreePath("placeholder", "placeholder")
	if err != nil {
		return isolation, err
	}
	if err := ensurePathInside(root, worktreePath); err != nil {
		return isolation, err
	}
	currentCommit, err := m.gitOutput(ctx, worktreePath, "rev-parse", "HEAD")
	if err == nil && strings.TrimSpace(currentCommit) != "" {
		isolation.CurrentCommit = strings.TrimSpace(currentCommit)
	}
	status, err := m.gitOutput(ctx, worktreePath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return isolation, fmt.Errorf("summarize worktree status: %w", err)
	}
	isolation.ChangedFiles = parseGitPorcelain(status)
	isolation.DiffSummary = boundedWorktreeSummary(m.diffStat(ctx, worktreePath), isolation.ChangedFiles)
	if strings.TrimSpace(isolation.MergeStatus) == "" {
		isolation.MergeStatus = "not_requested"
	}
	return isolation, nil
}

func (m WorktreeManager) CleanupSubagentWorktree(ctx context.Context, isolation domainjob.WorktreeIsolation) (domainjob.CleanupReceipt, error) {
	now := time.Now().UTC()
	receipt := domainjob.CleanupReceipt{
		ID:           "cleanup_" + safePathSegment(isolation.WorktreeBranch) + "_" + fmt.Sprintf("%d", now.UnixNano()),
		WorktreePath: strings.TrimSpace(isolation.WorktreePath),
		Branch:       strings.TrimSpace(isolation.WorktreeBranch),
		CreatedAt:    now.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(isolation.IsolationMode) != string(domainjob.IsolationWorktree) {
		return receipt, errors.New("cleanup requires worktree isolation")
	}
	worktreePath := strings.TrimSpace(isolation.WorktreePath)
	if worktreePath == "" {
		return receipt, errors.New("worktree path is required")
	}
	branch := strings.TrimSpace(isolation.WorktreeBranch)
	if !strings.HasPrefix(branch, "codex/subagent/") {
		return receipt, errors.New("cleanup requires an analytix-owned subagent branch")
	}
	root, _, err := m.worktreePath("placeholder", "placeholder")
	if err != nil {
		return receipt, err
	}
	if err := ensurePathInside(root, worktreePath); err != nil {
		return receipt, err
	}
	if _, err := os.Stat(worktreePath); errors.Is(err, os.ErrNotExist) {
		receipt.Removed = true
		return receipt, nil
	} else if err != nil {
		return receipt, err
	}
	commonDir, err := m.gitOutput(ctx, worktreePath, "rev-parse", "--git-common-dir")
	if err != nil || strings.TrimSpace(commonDir) == "" {
		return receipt, errors.New("inspect worktree git common dir")
	}
	commonDir = strings.TrimSpace(commonDir)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(worktreePath, commonDir)
	}
	if _, err := m.gitRawOutput(ctx, "--git-dir="+commonDir, "worktree", "remove", "--force", worktreePath); err != nil {
		return receipt, fmt.Errorf("remove subagent worktree: %w", err)
	}
	if _, err := m.gitRawOutput(ctx, "--git-dir="+commonDir, "branch", "-D", branch); err != nil {
		return receipt, fmt.Errorf("remove subagent worktree branch: %w", err)
	}
	receipt.Removed = true
	return receipt, nil
}

func (m WorktreeManager) AcceptSubagentWorktree(ctx context.Context, request ports.WorktreeAcceptRequest) (domainjob.WorktreeIsolation, domainjob.AcceptDecision, error) {
	isolation := request.Isolation
	now := time.Now().UTC()
	decision := domainjob.AcceptDecision{
		ID:             "accept_" + safePathSegment(request.JobID) + "_" + fmt.Sprintf("%d", now.UnixNano()),
		ParentThreadID: strings.TrimSpace(request.ParentThreadID),
		ChildRunID:     strings.TrimSpace(request.JobID),
		JobID:          strings.TrimSpace(request.JobID),
		MergeRequestID: strings.TrimSpace(request.MergeRequestID),
		ApprovalID:     strings.TrimSpace(request.ApprovalID),
		BaseCommit:     strings.TrimSpace(isolation.BaseCommit),
		ChangedFiles:   cloneProcessChangedFiles(isolation.ChangedFiles),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(isolation.IsolationMode) != string(domainjob.IsolationWorktree) {
		return isolation, decision, errors.New("accept requires worktree isolation")
	}
	if strings.TrimSpace(request.ApprovalID) == "" {
		return isolation, decision, errors.New("approval_id is required for isolation accept")
	}
	parentWorkspace, err := filepath.Abs(strings.TrimSpace(request.ParentWorkspace))
	if err != nil || strings.TrimSpace(request.ParentWorkspace) == "" {
		return isolation, decision, errors.New("parent workspace is required for isolation accept")
	}
	if ok, err := m.gitBool(ctx, parentWorkspace, "rev-parse", "--is-inside-work-tree"); err != nil || !ok {
		return isolation, decision, errors.New("parent workspace must be a git worktree")
	}
	if err := m.requireCleanParentWorkspace(ctx, parentWorkspace); err != nil {
		return isolation, decision, err
	}
	worktreePath := strings.TrimSpace(isolation.WorktreePath)
	if worktreePath == "" {
		return isolation, decision, errors.New("worktree path is required")
	}
	branch := strings.TrimSpace(isolation.WorktreeBranch)
	if !strings.HasPrefix(branch, "codex/subagent/") {
		return isolation, decision, errors.New("accept requires an analytix-owned subagent branch")
	}
	root, _, err := m.worktreePath("placeholder", "placeholder")
	if err != nil {
		return isolation, decision, err
	}
	if err := ensurePathInside(root, worktreePath); err != nil {
		return isolation, decision, err
	}
	if _, err := os.Stat(worktreePath); err != nil {
		return isolation, decision, err
	}
	refreshed, err := m.SummarizeSubagentWorktree(ctx, isolation)
	if err != nil {
		return isolation, decision, fmt.Errorf("summarize isolation before accept: %w", err)
	}
	isolation = refreshed
	decision.BaseCommit = strings.TrimSpace(isolation.BaseCommit)
	decision.ChangedFiles = cloneProcessChangedFiles(isolation.ChangedFiles)
	parentHead, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(parentHead) == "" {
		return isolation, decision, errors.New("parent workspace must have a valid HEAD commit")
	}
	parentHead = strings.TrimSpace(parentHead)
	baseCommit := strings.TrimSpace(isolation.BaseCommit)
	if baseCommit == "" {
		return isolation, decision, errors.New("base commit is required for isolation accept")
	}
	if parentHead != baseCommit {
		return isolation, decision, fmt.Errorf("parent HEAD %s does not match isolation base commit %s", parentHead, baseCommit)
	}
	decision.ParentHeadBefore = parentHead
	if hasConflictedChangedFile(isolation.ChangedFiles) {
		isolation.MergeStatus = "conflicted"
		return isolation, decision, errors.New("isolation changed files contain conflicts")
	}
	if err := m.markUntrackedForDiff(ctx, worktreePath, isolation.ChangedFiles); err != nil {
		return isolation, decision, err
	}
	patch, err := m.gitStdout(ctx, append([]string{"-C", worktreePath}, "diff", "--binary", "--no-ext-diff", baseCommit, "--")...)
	if err != nil {
		return isolation, decision, fmt.Errorf("generate isolation patch: %w", err)
	}
	if strings.TrimSpace(patch) == "" {
		return isolation, decision, errors.New("isolation patch is empty")
	}
	if len(patch) > maxSubagentAcceptPatchBytes {
		return isolation, decision, fmt.Errorf("isolation patch exceeds %d bytes", maxSubagentAcceptPatchBytes)
	}
	digest := sha256.Sum256([]byte(patch))
	decision.AppliedPatchDigest = "sha256:" + hex.EncodeToString(digest[:])
	patchFile, err := os.CreateTemp("", "analytix-subagent-accept-*.patch")
	if err != nil {
		return isolation, decision, err
	}
	patchPath := patchFile.Name()
	defer func() { _ = os.Remove(patchPath) }()
	if _, err := patchFile.WriteString(patch); err != nil {
		_ = patchFile.Close()
		return isolation, decision, err
	}
	if !strings.HasSuffix(patch, "\n") {
		if _, err := patchFile.WriteString("\n"); err != nil {
			_ = patchFile.Close()
			return isolation, decision, err
		}
	}
	if err := patchFile.Close(); err != nil {
		return isolation, decision, err
	}
	if _, err := m.gitOutput(ctx, parentWorkspace, "apply", "--check", "--whitespace=nowarn", patchPath); err != nil {
		isolation.MergeStatus = "conflicted"
		return isolation, decision, fmt.Errorf("isolation patch does not apply cleanly: %w", err)
	}
	if _, err := m.gitOutput(ctx, parentWorkspace, "apply", "--whitespace=nowarn", patchPath); err != nil {
		isolation.MergeStatus = "conflicted"
		return isolation, decision, fmt.Errorf("apply isolation patch: %w", err)
	}
	parentHeadAfter, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(parentHeadAfter) == "" {
		parentHeadAfter = parentHead
	}
	decision.ParentHeadAfter = strings.TrimSpace(parentHeadAfter)
	isolation.MergeStatus = "accepted"
	return isolation, decision, nil
}

func (m WorktreeManager) ReportSubagentWorktreeConflict(ctx context.Context, request ports.WorktreeConflictReportRequest) (domainjob.WorktreeIsolation, domainjob.ConflictReport, error) {
	isolation := request.Isolation
	now := time.Now().UTC()
	report := domainjob.ConflictReport{
		ID:                     "conflict_" + safePathSegment(request.JobID) + "_" + fmt.Sprintf("%d", now.UnixNano()),
		ParentThreadID:         strings.TrimSpace(request.ParentThreadID),
		ChildRunID:             strings.TrimSpace(request.JobID),
		JobID:                  strings.TrimSpace(request.JobID),
		BaseCommit:             strings.TrimSpace(isolation.BaseCommit),
		CreatedAt:              now.Format(time.RFC3339Nano),
		TouchedParentWorkspace: false,
	}
	if strings.TrimSpace(isolation.IsolationMode) != string(domainjob.IsolationWorktree) {
		return isolation, report, errors.New("conflict report requires worktree isolation")
	}
	parentWorkspace, err := filepath.Abs(strings.TrimSpace(request.ParentWorkspace))
	if err != nil || strings.TrimSpace(request.ParentWorkspace) == "" {
		return isolation, report, errors.New("parent workspace is required for conflict report")
	}
	if ok, err := m.gitBool(ctx, parentWorkspace, "rev-parse", "--is-inside-work-tree"); err != nil || !ok {
		return isolation, report, errors.New("parent workspace must be a git worktree")
	}
	parentHead, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(parentHead) == "" {
		return isolation, report, errors.New("parent workspace must have a valid HEAD commit")
	}
	report.ParentHeadAtConflict = strings.TrimSpace(parentHead)
	refreshed, err := m.SummarizeSubagentWorktree(ctx, isolation)
	if err != nil {
		return isolation, report, fmt.Errorf("summarize isolation before conflict report: %w", err)
	}
	isolation = refreshed
	isolation.MergeStatus = "conflicted"
	report.BaseCommit = strings.TrimSpace(isolation.BaseCommit)
	report.ChildHeadAtConflict = strings.TrimSpace(isolation.CurrentCommit)
	report.ConflictFiles = conflictFilesForReport(isolation.ChangedFiles)
	report.ConflictSummary = boundedWorktreeSummary(isolation.DiffSummary, report.ConflictFiles)
	if digest, err := m.sourcePatchDigest(ctx, isolation); err == nil {
		report.SourcePatchDigest = digest
	}
	return isolation, report, nil
}

func (m WorktreeManager) CheckSubagentRepairPatch(ctx context.Context, request ports.WorktreeRepairCheckRequest) (domainjob.WorktreeIsolation, domainjob.RepairPatchReview, error) {
	isolation := request.Isolation
	now := time.Now().UTC()
	review := domainjob.RepairPatchReview{
		ID:                 "repair_review_" + safePathSegment(request.JobID) + "_" + fmt.Sprintf("%d", now.UnixNano()),
		ConflictReportID:   strings.TrimSpace(request.ConflictReportID),
		ParentThreadID:     strings.TrimSpace(request.ParentThreadID),
		ChildRunID:         strings.TrimSpace(request.JobID),
		JobID:              strings.TrimSpace(request.JobID),
		ExpectedParentHead: strings.TrimSpace(request.ExpectedParentHead),
		CreatedAt:          now.Format(time.RFC3339Nano),
	}
	parentWorkspace, err := filepath.Abs(strings.TrimSpace(request.ParentWorkspace))
	if err != nil || strings.TrimSpace(request.ParentWorkspace) == "" {
		return isolation, review, errors.New("parent workspace is required for repair check")
	}
	if ok, err := m.gitBool(ctx, parentWorkspace, "rev-parse", "--is-inside-work-tree"); err != nil || !ok {
		return isolation, review, errors.New("parent workspace must be a git worktree")
	}
	parentHead, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(parentHead) == "" {
		return isolation, review, errors.New("parent workspace must have a valid HEAD commit")
	}
	parentHead = strings.TrimSpace(parentHead)
	if strings.TrimSpace(review.ExpectedParentHead) == "" {
		review.ExpectedParentHead = parentHead
	}
	if parentHead != review.ExpectedParentHead {
		return isolation, review, fmt.Errorf("parent HEAD %s does not match expected repair head %s", parentHead, review.ExpectedParentHead)
	}
	patch := strings.TrimSpace(request.RepairPatch)
	changedFiles, digest, err := validateRepairPatch(patch)
	if err != nil {
		return isolation, review, err
	}
	review.ChangedFiles = changedFiles
	review.RepairPatchDigest = digest
	review.RepairPatch = patch
	patchPath, cleanup, err := writeTempPatch(patch, "analytix-subagent-repair-check-*.patch")
	if err != nil {
		return isolation, review, err
	}
	defer cleanup()
	if _, err := m.gitOutput(ctx, parentWorkspace, "apply", "--check", "--whitespace=nowarn", patchPath); err != nil {
		review.DryRunStatus = "conflicted"
		review.RejectedReason = "repair patch does not apply cleanly: " + err.Error()
		isolation.MergeStatus = "repair_rejected"
		return isolation, review, nil
	}
	review.DryRunStatus = "clean"
	isolation.MergeStatus = "repair_checked"
	return isolation, review, nil
}

func (m WorktreeManager) AcceptSubagentRepairPatch(ctx context.Context, request ports.WorktreeRepairAcceptRequest) (domainjob.WorktreeIsolation, domainjob.RepairDecision, error) {
	isolation := request.Isolation
	now := time.Now().UTC()
	decision := domainjob.RepairDecision{
		ID:             "repair_accept_" + safePathSegment(request.JobID) + "_" + fmt.Sprintf("%d", now.UnixNano()),
		RepairReviewID: strings.TrimSpace(request.RepairReviewID),
		ApprovalID:     strings.TrimSpace(request.ApprovalID),
		CreatedAt:      now.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(request.ApprovalID) == "" {
		return isolation, decision, errors.New("approval_id is required for repair accept")
	}
	parentWorkspace, err := filepath.Abs(strings.TrimSpace(request.ParentWorkspace))
	if err != nil || strings.TrimSpace(request.ParentWorkspace) == "" {
		return isolation, decision, errors.New("parent workspace is required for repair accept")
	}
	if ok, err := m.gitBool(ctx, parentWorkspace, "rev-parse", "--is-inside-work-tree"); err != nil || !ok {
		return isolation, decision, errors.New("parent workspace must be a git worktree")
	}
	if err := m.requireCleanParentWorkspace(ctx, parentWorkspace); err != nil {
		return isolation, decision, err
	}
	parentHead, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(parentHead) == "" {
		return isolation, decision, errors.New("parent workspace must have a valid HEAD commit")
	}
	parentHead = strings.TrimSpace(parentHead)
	expected := strings.TrimSpace(request.ExpectedParentHead)
	if expected == "" {
		return isolation, decision, errors.New("expected parent head is required for repair accept")
	}
	if parentHead != expected {
		return isolation, decision, fmt.Errorf("parent HEAD %s does not match expected repair head %s", parentHead, expected)
	}
	patch := strings.TrimSpace(request.RepairPatch)
	_, digest, err := validateRepairPatch(patch)
	if err != nil {
		return isolation, decision, err
	}
	decision.ParentHeadBefore = parentHead
	decision.AppliedPatchDigest = digest
	patchPath, cleanup, err := writeTempPatch(patch, "analytix-subagent-repair-accept-*.patch")
	if err != nil {
		return isolation, decision, err
	}
	defer cleanup()
	if _, err := m.gitOutput(ctx, parentWorkspace, "apply", "--check", "--whitespace=nowarn", patchPath); err != nil {
		isolation.MergeStatus = "repair_rejected"
		return isolation, decision, fmt.Errorf("repair patch does not apply cleanly: %w", err)
	}
	if _, err := m.gitOutput(ctx, parentWorkspace, "apply", "--whitespace=nowarn", patchPath); err != nil {
		isolation.MergeStatus = "repair_rejected"
		return isolation, decision, fmt.Errorf("apply repair patch: %w", err)
	}
	parentHeadAfter, err := m.gitOutput(ctx, parentWorkspace, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(parentHeadAfter) == "" {
		parentHeadAfter = parentHead
	}
	decision.ParentHeadAfter = strings.TrimSpace(parentHeadAfter)
	isolation.MergeStatus = "repair_accepted"
	return isolation, decision, nil
}

func (m WorktreeManager) diffStat(ctx context.Context, worktreePath string) string {
	parts := []string{}
	if value, err := m.gitOutput(ctx, worktreePath, "diff", "--stat"); err == nil && strings.TrimSpace(value) != "" {
		parts = append(parts, strings.TrimSpace(value))
	}
	if value, err := m.gitOutput(ctx, worktreePath, "diff", "--cached", "--stat"); err == nil && strings.TrimSpace(value) != "" {
		parts = append(parts, strings.TrimSpace(value))
	}
	return strings.Join(parts, "\n")
}

func (m WorktreeManager) requireCleanParentWorkspace(ctx context.Context, parentWorkspace string) error {
	status, err := m.gitOutput(ctx, parentWorkspace, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect parent git status: %w", err)
	}
	conflicts, err := m.gitOutput(ctx, parentWorkspace, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return fmt.Errorf("inspect parent git conflicts: %w", err)
	}
	if strings.TrimSpace(status) != "" || strings.TrimSpace(conflicts) != "" {
		return errors.New("parent workspace has tracked, untracked, or conflicted changes; isolation accept requires a clean parent")
	}
	return nil
}

func (m WorktreeManager) markUntrackedForDiff(ctx context.Context, worktreePath string, files []domainjob.ChangedFile) error {
	paths := []string{}
	for _, file := range files {
		if strings.TrimSpace(file.Status) != "??" || strings.TrimSpace(file.Path) == "" {
			continue
		}
		paths = append(paths, strings.TrimSpace(file.Path))
	}
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "-N", "--"}, paths...)
	if _, err := m.gitOutput(ctx, worktreePath, args...); err != nil {
		return fmt.Errorf("prepare untracked files for isolation patch: %w", err)
	}
	return nil
}

func (m WorktreeManager) sourcePatchDigest(ctx context.Context, isolation domainjob.WorktreeIsolation) (string, error) {
	worktreePath := strings.TrimSpace(isolation.WorktreePath)
	baseCommit := strings.TrimSpace(isolation.BaseCommit)
	if worktreePath == "" || baseCommit == "" {
		return "", errors.New("worktree path and base commit are required for source patch digest")
	}
	patch, err := m.gitStdout(ctx, append([]string{"-C", worktreePath}, "diff", "--binary", "--no-ext-diff", baseCommit, "--")...)
	if err != nil {
		return "", err
	}
	if len(patch) > maxSubagentAcceptPatchBytes {
		return "", fmt.Errorf("source patch exceeds %d bytes", maxSubagentAcceptPatchBytes)
	}
	digest := sha256.Sum256([]byte(patch))
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func conflictFilesForReport(files []domainjob.ChangedFile) []domainjob.ChangedFile {
	conflicted := []domainjob.ChangedFile{}
	for _, file := range files {
		if hasConflictedChangedFile([]domainjob.ChangedFile{file}) {
			conflicted = append(conflicted, domainjob.ChangedFile{Path: strings.TrimSpace(file.Path), Status: strings.TrimSpace(file.Status)})
		}
	}
	if len(conflicted) > 0 {
		return conflicted
	}
	return cloneProcessChangedFiles(files)
}

func validateRepairPatch(patch string) ([]domainjob.ChangedFile, string, error) {
	if strings.TrimSpace(patch) == "" {
		return nil, "", errors.New("repair patch is required")
	}
	if len(patch) > maxSubagentRepairPatchBytes {
		return nil, "", fmt.Errorf("repair patch exceeds %d bytes", maxSubagentRepairPatchBytes)
	}
	files, err := repairPatchChangedFiles(patch)
	if err != nil {
		return nil, "", err
	}
	if len(files) == 0 {
		return nil, "", errors.New("repair patch has no file changes")
	}
	if len(files) > maxSubagentRepairPatchFiles {
		return nil, "", fmt.Errorf("repair patch changes too many files: %d", len(files))
	}
	digest := sha256.Sum256([]byte(patch))
	return files, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func writeTempPatch(patch string, pattern string) (string, func(), error) {
	patchFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", func() {}, err
	}
	patchPath := patchFile.Name()
	cleanup := func() { _ = os.Remove(patchPath) }
	if _, err := patchFile.WriteString(patch); err != nil {
		_ = patchFile.Close()
		cleanup()
		return "", func() {}, err
	}
	if !strings.HasSuffix(patch, "\n") {
		if _, err := patchFile.WriteString("\n"); err != nil {
			_ = patchFile.Close()
			cleanup()
			return "", func() {}, err
		}
	}
	if err := patchFile.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return patchPath, cleanup, nil
}

func repairPatchChangedFiles(patch string) ([]domainjob.ChangedFile, error) {
	seen := map[string]bool{}
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			fields := strings.Fields(strings.TrimPrefix(line, "diff --git "))
			for _, field := range fields {
				addRepairPatchPath(seen, field)
			}
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				addRepairPatchPath(seen, fields[1])
			}
		case strings.HasPrefix(line, "rename from "):
			addRepairPatchPath(seen, strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			addRepairPatchPath(seen, strings.TrimPrefix(line, "rename to "))
		}
	}
	out := make([]domainjob.ChangedFile, 0, len(seen))
	for path := range seen {
		if err := validateRepairPatchPath(path); err != nil {
			return nil, err
		}
		out = append(out, domainjob.ChangedFile{Path: path, Status: "M"})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func addRepairPatchPath(seen map[string]bool, value string) {
	path := strings.TrimSpace(value)
	path = strings.Trim(path, "\"")
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	if path == "" || path == "/dev/null" {
		return
	}
	seen[path] = true
}

func validateRepairPatchPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsRune(path, 0) || filepath.IsAbs(path) || strings.Contains(path, "\\") {
		return fmt.Errorf("repair patch contains unsafe path: %s", path)
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("repair patch contains unsafe path: %s", path)
	}
	return nil
}

func hasConflictedChangedFile(files []domainjob.ChangedFile) bool {
	for _, file := range files {
		status := strings.TrimSpace(file.Status)
		if strings.Contains(status, "U") || status == "AA" || status == "DD" {
			return true
		}
	}
	return false
}

func cloneProcessChangedFiles(values []domainjob.ChangedFile) []domainjob.ChangedFile {
	if len(values) == 0 {
		return nil
	}
	out := make([]domainjob.ChangedFile, 0, len(values))
	for _, value := range values {
		path := strings.TrimSpace(value.Path)
		status := strings.TrimSpace(value.Status)
		if path == "" && status == "" {
			continue
		}
		out = append(out, domainjob.ChangedFile{Path: path, Status: status})
	}
	return out
}

func (m WorktreeManager) worktreePath(parentThreadID, jobID string) (string, string, error) {
	root := strings.TrimSpace(m.Root)
	if root == "" {
		return "", "", errors.New("subagent worktree root is not configured")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	threadSegment := safePathSegment(parentThreadID)
	jobSegment := safePathSegment(jobID)
	if threadSegment == "" || jobSegment == "" {
		return "", "", errors.New("parent thread id and job id are required for worktree isolation")
	}
	return absRoot, filepath.Join(absRoot, threadSegment, jobSegment), nil
}

func (m WorktreeManager) gitBool(ctx context.Context, workspace string, args ...string) (bool, error) {
	value, err := m.gitOutput(ctx, workspace, args...)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(value) == "true", nil
}

func (m WorktreeManager) gitCommandSucceeds(ctx context.Context, workspace string, args ...string) (bool, error) {
	_, err := m.gitOutput(ctx, workspace, args...)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m WorktreeManager) gitOutput(ctx context.Context, workspace string, args ...string) (string, error) {
	return m.gitOutputArgs(ctx, append([]string{"-C", workspace}, args...)...)
}

func (m WorktreeManager) gitRawOutput(ctx context.Context, args ...string) (string, error) {
	return m.gitOutputArgs(ctx, args...)
}

func (m WorktreeManager) gitOutputArgs(ctx context.Context, args ...string) (string, error) {
	stdout, err := m.gitStdout(ctx, args...)
	return strings.TrimSpace(stdout), err
}

func (m WorktreeManager) gitStdout(ctx context.Context, args ...string) (string, error) {
	probe := m.CommandProbe
	if probe == nil {
		probe = NewCommandProbe(m.protectedReadDirs...)
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result := probe.ProbeCommand(probeCtx, ports.CommandProbeRequest{
		Binary: "git",
		Args:   append([]string(nil), args...),
	})
	if !result.Found {
		return "", errors.New("git not found")
	}
	if result.TimedOut {
		return "", errors.New("git command timed out")
	}
	if strings.TrimSpace(result.Error) != "" {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Error)
		}
		return strings.TrimSpace(result.Stdout), errors.New(message)
	}
	return result.Stdout, nil
}

func subagentWorktreeBranch(parentThreadID, jobID string) string {
	return "codex/subagent/" + safePathSegment(parentThreadID) + "/" + safePathSegment(jobID)
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastDash := false
	for _, ch := range value {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
			builder.WriteRune(ch)
			lastDash = false
		case ch == '-' || ch == '_' || ch == '.':
			builder.WriteRune(ch)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-.")
}

func ensurePathInside(root, path string) error {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("worktree path must stay under analytix-owned root: %s", root)
	}
	return nil
}

func parseGitPorcelain(status string) []domainjob.ChangedFile {
	files := []domainjob.ChangedFile{}
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		statusCode := strings.TrimSpace(line[:minInt(len(line), 2)])
		path := ""
		if len(line) > 3 {
			path = strings.TrimSpace(line[3:])
		}
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = strings.TrimSpace(parts[len(parts)-1])
		}
		if path == "" {
			continue
		}
		files = append(files, domainjob.ChangedFile{Path: path, Status: statusCode})
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Path == files[j].Path {
			return files[i].Status < files[j].Status
		}
		return files[i].Path < files[j].Path
	})
	return files
}

func boundedWorktreeSummary(diffStat string, files []domainjob.ChangedFile) string {
	lines := []string{}
	if strings.TrimSpace(diffStat) != "" {
		lines = append(lines, strings.TrimSpace(diffStat))
	}
	untracked := 0
	for _, file := range files {
		if file.Status == "??" {
			untracked++
		}
	}
	if untracked > 0 {
		lines = append(lines, fmt.Sprintf("untracked files: %d", untracked))
	}
	if len(files) > 0 {
		lines = append(lines, fmt.Sprintf("changed files: %d", len(files)))
	}
	value := strings.TrimSpace(strings.Join(lines, "\n"))
	runes := []rune(value)
	if len(runes) <= 2000 {
		return value
	}
	return string(runes[:2000])
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
