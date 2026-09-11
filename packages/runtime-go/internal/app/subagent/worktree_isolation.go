package subagent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

var ErrWorktreeIsolationAuthorityRequired = errors.New("worktree isolation control requires host-issued durable mutation authority")

func ValidateWorktreeIsolationRequest(request TaskRequest) error {
	if strings.TrimSpace(request.IsolationMode) == "" {
		return nil
	}
	if request.IsolationMode != string(domainjob.IsolationWorktree) {
		return fmt.Errorf("unsupported isolationMode %q", request.IsolationMode)
	}
	return ErrWorktreeIsolationAuthorityRequired
}

func WorktreeIsolationFromRecord(record domainjob.Record) domainjob.WorktreeIsolation {
	return domainjob.WorktreeIsolation{
		IsolationMode:  strings.TrimSpace(record.IsolationMode),
		WorktreePath:   strings.TrimSpace(record.WorktreePath),
		WorktreeBranch: strings.TrimSpace(record.WorktreeBranch),
		BaseCommit:     strings.TrimSpace(record.BaseCommit),
		CurrentCommit:  strings.TrimSpace(record.CurrentCommit),
		ChangedFiles:   append([]domainjob.ChangedFile(nil), record.ChangedFiles...),
		DiffSummary:    strings.TrimSpace(record.DiffSummary),
		MergeStatus:    strings.TrimSpace(record.MergeStatus),
	}
}

type WorktreeIsolationStore interface {
	UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error)
}

type WorktreeThreadPreparer func(workspace string) (string, error)

type WorktreeIsolationPrepareInput struct {
	Manager         ports.WorktreeManager
	Store           WorktreeIsolationStore
	Record          domainjob.Record
	ParentWorkspace string
	ParentThreadID  string
	PrepareThread   WorktreeThreadPreparer
}

func PrepareWorktreeIsolation(ctx context.Context, input WorktreeIsolationPrepareInput) (domainjob.Record, string, error) {
	record := input.Record
	return record, input.ParentWorkspace, ErrWorktreeIsolationAuthorityRequired
}

func MarkWorktreeIsolationFailed(store WorktreeIsolationStore, record domainjob.Record, err error) domainjob.Record {
	if store == nil || strings.TrimSpace(record.ID) == "" {
		return record
	}
	isolation := WorktreeIsolationFromRecord(record)
	if strings.TrimSpace(isolation.IsolationMode) == "" {
		isolation.IsolationMode = string(domainjob.IsolationWorktree)
	}
	if strings.TrimSpace(isolation.MergeStatus) == "" {
		isolation.MergeStatus = "not_requested"
	}
	updated, updateErr := store.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		Status:      string(domainjob.StatusFailed),
		FailureCode: domainjob.FailureChildExecutionFailed,
		Isolation:   &isolation,
	})
	if updateErr == nil {
		return updated
	}
	return record
}

type WorktreeIsolationSummaryInput struct {
	Manager ports.WorktreeManager
	Store   WorktreeIsolationStore
	Record  domainjob.Record
}

func SummarizeWorktreeIsolation(ctx context.Context, input WorktreeIsolationSummaryInput) (domainjob.Record, bool) {
	record := input.Record
	if strings.TrimSpace(record.IsolationMode) != string(domainjob.IsolationWorktree) {
		return record, false
	}
	isolation := WorktreeIsolationFromRecord(record)
	if input.Manager == nil {
		isolation.DiffSummary = "worktree summary unavailable"
	} else if summarized, err := input.Manager.SummarizeSubagentWorktree(ctx, isolation); err == nil {
		isolation = summarized
	} else {
		isolation.DiffSummary = "worktree summary unavailable"
	}
	if input.Store == nil {
		return record, false
	}
	if updated, err := input.Store.UpdateChildRun(record.ID, domainjob.UpdateRequest{Isolation: &isolation}); err == nil {
		return updated, true
	}
	return record, false
}

func AddIsolationMetadata(out map[string]any, record domainjob.Record) {
	if ChildOutputRequiresWithholding(record) {
		return
	}
	if strings.TrimSpace(record.IsolationMode) == "" {
		return
	}
	out["isolationMode"] = strings.TrimSpace(record.IsolationMode)
	if strings.TrimSpace(record.WorktreePath) != "" {
		out["worktreePath"] = strings.TrimSpace(record.WorktreePath)
	}
	if strings.TrimSpace(record.WorktreeBranch) != "" {
		out["worktreeBranch"] = strings.TrimSpace(record.WorktreeBranch)
	}
	if strings.TrimSpace(record.BaseCommit) != "" {
		out["baseCommit"] = strings.TrimSpace(record.BaseCommit)
	}
	if strings.TrimSpace(record.CurrentCommit) != "" {
		out["currentCommit"] = strings.TrimSpace(record.CurrentCommit)
	}
	if strings.TrimSpace(record.MergeStatus) != "" {
		out["mergeStatus"] = strings.TrimSpace(record.MergeStatus)
	}
	if strings.TrimSpace(record.DiffSummary) != "" {
		out["diffSummary"] = strings.TrimSpace(record.DiffSummary)
	}
	if len(record.ChangedFiles) > 0 {
		files := make([]any, 0, len(record.ChangedFiles))
		for _, file := range record.ChangedFiles {
			path := strings.TrimSpace(file.Path)
			status := strings.TrimSpace(file.Status)
			if path == "" {
				continue
			}
			files = append(files, map[string]any{"path": path, "status": status})
		}
		if len(files) > 0 {
			out["changedFiles"] = files
			out["changedFileCount"] = float64(len(files))
		}
	}
	if len(record.MergeDecisions) > 0 {
		decision := record.MergeDecisions[len(record.MergeDecisions)-1]
		if strings.TrimSpace(decision.ID) != "" {
			out["mergeDecisionId"] = strings.TrimSpace(decision.ID)
		}
		if strings.TrimSpace(decision.Decision) != "" {
			out["mergeDecision"] = strings.TrimSpace(decision.Decision)
		}
		if strings.TrimSpace(decision.CreatedAt) != "" {
			out["mergeDecisionCreatedAt"] = strings.TrimSpace(decision.CreatedAt)
		}
	}
	if len(record.CleanupReceipts) > 0 {
		receipt := record.CleanupReceipts[len(record.CleanupReceipts)-1]
		if strings.TrimSpace(receipt.ID) != "" {
			out["cleanupReceiptId"] = strings.TrimSpace(receipt.ID)
		}
		if strings.TrimSpace(receipt.AcceptDecisionID) != "" {
			out["cleanupAcceptDecisionId"] = strings.TrimSpace(receipt.AcceptDecisionID)
		}
		out["cleanupRemoved"] = receipt.Removed
		if strings.TrimSpace(receipt.RetainedReason) != "" {
			out["cleanupRetainedReason"] = strings.TrimSpace(receipt.RetainedReason)
		}
	}
	if len(record.AcceptDecisions) > 0 {
		decision := record.AcceptDecisions[len(record.AcceptDecisions)-1]
		if strings.TrimSpace(decision.ID) != "" {
			out["acceptDecisionId"] = strings.TrimSpace(decision.ID)
		}
		if strings.TrimSpace(decision.ApprovalID) != "" {
			out["acceptApprovalId"] = strings.TrimSpace(decision.ApprovalID)
		}
		if strings.TrimSpace(decision.AppliedPatchDigest) != "" {
			out["appliedPatchDigest"] = strings.TrimSpace(decision.AppliedPatchDigest)
		}
	}
	if len(record.ConflictReports) > 0 {
		report := record.ConflictReports[len(record.ConflictReports)-1]
		if strings.TrimSpace(report.ID) != "" {
			out["conflictReportId"] = strings.TrimSpace(report.ID)
		}
		if strings.TrimSpace(report.ParentHeadAtConflict) != "" {
			out["parentHeadAtConflict"] = strings.TrimSpace(report.ParentHeadAtConflict)
		}
		if strings.TrimSpace(report.ChildHeadAtConflict) != "" {
			out["childHeadAtConflict"] = strings.TrimSpace(report.ChildHeadAtConflict)
		}
		if strings.TrimSpace(report.SourcePatchDigest) != "" {
			out["sourcePatchDigest"] = strings.TrimSpace(report.SourcePatchDigest)
		}
		if strings.TrimSpace(report.ConflictSummary) != "" {
			out["conflictSummary"] = strings.TrimSpace(report.ConflictSummary)
		}
		if len(report.ConflictFiles) > 0 {
			out["conflictFiles"] = changedFilesAny(report.ConflictFiles)
			out["conflictFileCount"] = float64(len(report.ConflictFiles))
		}
		out["conflictTouchedParentWorkspace"] = report.TouchedParentWorkspace
	}
	if len(record.RepairPatchReviews) > 0 {
		review := record.RepairPatchReviews[len(record.RepairPatchReviews)-1]
		if strings.TrimSpace(review.ID) != "" {
			out["repairReviewId"] = strings.TrimSpace(review.ID)
		}
		if strings.TrimSpace(review.ConflictReportID) != "" {
			out["repairConflictReportId"] = strings.TrimSpace(review.ConflictReportID)
		}
		if strings.TrimSpace(review.RepairPatchDigest) != "" {
			out["repairPatchDigest"] = strings.TrimSpace(review.RepairPatchDigest)
		}
		if strings.TrimSpace(review.ExpectedParentHead) != "" {
			out["repairExpectedParentHead"] = strings.TrimSpace(review.ExpectedParentHead)
		}
		if strings.TrimSpace(review.DryRunStatus) != "" {
			out["repairDryRunStatus"] = strings.TrimSpace(review.DryRunStatus)
		}
		if strings.TrimSpace(review.RejectedReason) != "" {
			out["repairRejectedReason"] = strings.TrimSpace(review.RejectedReason)
		}
		if len(review.ChangedFiles) > 0 {
			out["repairChangedFiles"] = changedFilesAny(review.ChangedFiles)
			out["repairChangedFileCount"] = float64(len(review.ChangedFiles))
		}
	}
	if len(record.RepairDecisions) > 0 {
		decision := record.RepairDecisions[len(record.RepairDecisions)-1]
		if strings.TrimSpace(decision.ID) != "" {
			out["repairDecisionId"] = strings.TrimSpace(decision.ID)
		}
		if strings.TrimSpace(decision.RepairReviewID) != "" {
			out["repairDecisionReviewId"] = strings.TrimSpace(decision.RepairReviewID)
		}
		if strings.TrimSpace(decision.ApprovalID) != "" {
			out["repairApprovalId"] = strings.TrimSpace(decision.ApprovalID)
		}
		if strings.TrimSpace(decision.AppliedPatchDigest) != "" {
			out["repairAppliedPatchDigest"] = strings.TrimSpace(decision.AppliedPatchDigest)
		}
	}
}
