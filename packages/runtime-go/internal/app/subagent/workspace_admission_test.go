package subagent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/ports"
)

func TestValidateChildWorkspaceAdmissionAllowsSameWorkspaceForeground(t *testing.T) {
	err := ValidateChildWorkspaceAdmission(ChildWorkspaceAdmissionInput{
		Request:                 TaskRequest{Prompt: "inspect"},
		ParentWorkspaceRealPath: "/workspace/case",
		ChildWorkspaceRealPath:  "/workspace/case",
	})
	if err != nil {
		t.Fatalf("same-workspace foreground child was rejected: %v", err)
	}
}

func TestValidateChildWorkspaceAdmissionRejectsCrossWorkspaceForeground(t *testing.T) {
	err := ValidateChildWorkspaceAdmission(ChildWorkspaceAdmissionInput{
		Request:                 TaskRequest{Prompt: "inspect"},
		ParentWorkspaceRealPath: "/workspace/case-a",
		ChildWorkspaceRealPath:  "/workspace/case-b",
	})
	if err == nil || !strings.Contains(err.Error(), "run_in_background=true") {
		t.Fatalf("cross-workspace foreground child should be rejected with the host path: %v", err)
	}
}

func TestValidateChildWorkspaceAdmissionAllowsHostBackgroundPath(t *testing.T) {
	request := TaskRequest{Prompt: "inspect", RunInBackground: true}
	if err := ValidateChildWorkspaceAdmission(ChildWorkspaceAdmissionInput{
		Request: request, ParentWorkspaceRealPath: "/workspace/case-a", ChildWorkspaceRealPath: "/workspace/case-b",
	}); err != nil {
		t.Fatalf("host-supported background path was rejected: %v", err)
	}
}

func TestValidateChildWorkspaceAdmissionFailsClosedWithoutFrozenAuthority(t *testing.T) {
	for _, input := range []ChildWorkspaceAdmissionInput{
		{Request: TaskRequest{RunInBackground: true}, ChildWorkspaceRealPath: "/workspace/case"},
		{Request: TaskRequest{RunInBackground: true}, ParentWorkspaceRealPath: "/workspace/case"},
	} {
		if err := ValidateChildWorkspaceAdmission(input); err == nil {
			t.Fatal("missing host workspace authority should fail closed")
		}
	}
}

func TestPrepareWorktreeIsolationRequiresAuthorityBeforeSideEffects(t *testing.T) {
	manager := &worktreeIsolationManagerSpy{}
	store := &worktreeIsolationStoreSpy{}
	prepareThreadCalls := 0
	record := domainjob.Record{
		ID:             "job-1",
		ParentThreadID: "thread-1",
		Status:         string(domainjob.StatusQueued),
		IsolationMode:  string(domainjob.IsolationWorktree),
	}

	gotRecord, gotWorkspace, err := PrepareWorktreeIsolation(context.Background(), WorktreeIsolationPrepareInput{
		Manager:         manager,
		Store:           store,
		Record:          record,
		ParentWorkspace: "/workspace/case",
		ParentThreadID:  "thread-1",
		PrepareThread: func(string) (string, error) {
			prepareThreadCalls++
			return "child-thread", nil
		},
	})

	if !errors.Is(err, ErrWorktreeIsolationAuthorityRequired) {
		t.Fatalf("worktree preparation should require durable mutation authority: %v", err)
	}
	if err.Error() != ErrWorktreeIsolationAuthorityRequired.Error() {
		t.Fatalf("worktree preparation returned a non-deterministic boundary: %q", err)
	}
	if manager.createCalls != 0 || store.updateCalls != 0 || prepareThreadCalls != 0 {
		t.Fatalf("worktree preparation crossed a side-effect boundary: manager=%d store=%d prepareThread=%d", manager.createCalls, store.updateCalls, prepareThreadCalls)
	}
	if !reflect.DeepEqual(gotRecord, record) {
		t.Fatalf("blocked preparation mutated the record: got=%#v want=%#v", gotRecord, record)
	}
	if gotWorkspace != "/workspace/case" {
		t.Fatalf("blocked preparation changed the workspace: %q", gotWorkspace)
	}
}

type worktreeIsolationManagerSpy struct {
	createCalls int
}

func (s *worktreeIsolationManagerSpy) CreateSubagentWorktree(context.Context, ports.WorktreeCreateRequest) (domainjob.WorktreeIsolation, error) {
	s.createCalls++
	return domainjob.WorktreeIsolation{WorktreePath: "/workspace/case-worktree"}, nil
}

func (*worktreeIsolationManagerSpy) SummarizeSubagentWorktree(context.Context, domainjob.WorktreeIsolation) (domainjob.WorktreeIsolation, error) {
	return domainjob.WorktreeIsolation{}, nil
}

func (*worktreeIsolationManagerSpy) CleanupSubagentWorktree(context.Context, domainjob.WorktreeIsolation) (domainjob.CleanupReceipt, error) {
	return domainjob.CleanupReceipt{}, nil
}

func (*worktreeIsolationManagerSpy) AcceptSubagentWorktree(context.Context, ports.WorktreeAcceptRequest) (domainjob.WorktreeIsolation, domainjob.AcceptDecision, error) {
	return domainjob.WorktreeIsolation{}, domainjob.AcceptDecision{}, nil
}

func (*worktreeIsolationManagerSpy) ReportSubagentWorktreeConflict(context.Context, ports.WorktreeConflictReportRequest) (domainjob.WorktreeIsolation, domainjob.ConflictReport, error) {
	return domainjob.WorktreeIsolation{}, domainjob.ConflictReport{}, nil
}

func (*worktreeIsolationManagerSpy) CheckSubagentRepairPatch(context.Context, ports.WorktreeRepairCheckRequest) (domainjob.WorktreeIsolation, domainjob.RepairPatchReview, error) {
	return domainjob.WorktreeIsolation{}, domainjob.RepairPatchReview{}, nil
}

func (*worktreeIsolationManagerSpy) AcceptSubagentRepairPatch(context.Context, ports.WorktreeRepairAcceptRequest) (domainjob.WorktreeIsolation, domainjob.RepairDecision, error) {
	return domainjob.WorktreeIsolation{}, domainjob.RepairDecision{}, nil
}

type worktreeIsolationStoreSpy struct {
	updateCalls int
}

func (s *worktreeIsolationStoreSpy) UpdateChildRun(string, domainjob.UpdateRequest) (domainjob.Record, error) {
	s.updateCalls++
	return domainjob.Record{}, nil
}
