package toolresult

import (
	"strings"
	"testing"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
)

func TestProtectedProducerJobOutputEnvelopeKeepsOriginalIdentityV1(t *testing.T) {
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	body, truncated := "actual producer bytes\n", false
	base := ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal,
		Workspace: "/synthetic/workspace", ThreadID: "thread-a", TurnID: "turn-a", CallID: callID,
		ResultItemID: ToolResultItemIDV1("turn-a", callID), ContextDigest: strings.Repeat("c", 64),
		ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64),
		Inspect: &ProtectedInspectV1{InputJSON: `{}`},
		Capture: ProtectedCaptureV1{Kind: "job", Status: "completed", Job: &ProtectedJobCaptureV1{
			Operation: "output", Jobs: []ProtectedJobRecordV1{{JobID: "job-a", Kind: "subagent", Status: "completed", Output: ProtectedJobOutputV1{Availability: "available", Body: &body, Truncated: &truncated}}}, Total: 1,
		}},
	}
	clone := func() ProtectedSnapshotV1 {
		s := base
		s.Capture.Job = cloneProtectedJobCaptureV1(base.Capture.Job)
		i := *base.Inspect
		s.Inspect = &i
		return s
	}
	for _, tool := range []string{"task", "delegate_task", "parallel_tasks", "run_skill", "bash"} {
		t.Run(tool, func(t *testing.T) {
			s := clone()
			s.ToolName = tool
			if tool == "bash" {
				s.Capture.Job.Jobs[0].Kind = "background-shell"
			}
			encoded, err := ProtectedSnapshotBytesV1(s)
			if err != nil {
				t.Fatal("actual original producer observation rejected", err)
			}
			if ValidateProtectedDisplaySnapshotV1(s) == nil {
				t.Fatal("producer source became its original call's display result")
			}
			decoded, err := ParseProtectedSnapshotV1(encoded)
			if err != nil || decoded.CallID != s.CallID || decoded.ResultItemID != s.ResultItemID || decoded.ToolName != tool || decoded.ExecutionGrantID != s.ExecutionGrantID {
				t.Fatal("source acquired another call/result/grant", err)
			}
			for _, mutate := range []func(*ProtectedSnapshotV1){
				func(s *ProtectedSnapshotV1) { s.Inspect = nil },
				func(s *ProtectedSnapshotV1) { s.Capture.Status = "failed" },
				func(s *ProtectedSnapshotV1) {
					s.ResultItemID = ToolResultItemIDV1(s.TurnID, "call_host_"+strings.Repeat("f", 64))
				},
				func(s *ProtectedSnapshotV1) { s.Capture.Job.Jobs[0].Status = "running" },
				func(s *ProtectedSnapshotV1) {
					s.Capture.Job.Jobs[0].Output = ProtectedJobOutputV1{Availability: "unavailable"}
				},
				func(s *ProtectedSnapshotV1) { s.Capture.Job.Jobs[0].Kind = "child-run" },
				func(s *ProtectedSnapshotV1) {
					s.Capture.Job.Receipt = &ProtectedJobReceiptV1{Operation: "cancel", Status: "cancellation-requested"}
				},
				func(s *ProtectedSnapshotV1) { s.Capture.Job.Total = 2; s.Capture.Job.Truncated = true },
			} {
				bad := s
				bad.Capture.Job = cloneProtectedJobCaptureV1(s.Capture.Job)
				mutate(&bad)
				if ValidateProtectedSnapshotV1(bad) == nil {
					t.Fatal("open producer source variant accepted")
				}
			}
		})
	}
	for _, tool := range []string{"read", "web_fetch", "get_goal", "list_jobs", "wait", "kill_shell", "restart_job"} {
		s := clone()
		s.ToolName = tool
		if ValidateProtectedSnapshotV1(s) == nil {
			t.Fatal("unrelated tool acquired producer output source")
		}
	}
	// A query seals an independently admitted bash_output identity, never the
	// original parent producer's call/result/grant tuple.
	query := clone()
	query.ToolName = "bash_output"
	query.CallID = "call_host_" + strings.Repeat("f", 64)
	query.ResultItemID = ToolResultItemIDV1(query.TurnID, query.CallID)
	query.ExecutionGrantID = strings.Repeat("f", 64)
	if ValidateProtectedDisplaySnapshotV1(query) != nil {
		t.Fatal("existing query variant changed")
	}
	for _, status := range []string{"completed", "failed", "timeout", "canceled", "killed", "interrupted", "aborted"} {
		s := clone()
		s.ToolName = "bash"
		s.Capture.Job.Jobs[0].Kind = "background-shell"
		s.Capture.Job.Jobs[0].Status = status
		if ValidateProtectedSnapshotV1(s) != nil {
			t.Fatal("real terminal shell status rejected", status)
		}
	}
	for tool, operation := range map[string]string{"task": "spawn", "delegate_task": "spawn", "parallel_tasks": "spawn", "list_jobs": "list", "wait": "wait", "bash_output": "output", "kill_shell": "cancel", "restart_job": "restart"} {
		s := clone()
		s.ToolName = tool
		s.Capture.Job.Operation = operation
		if operation != "output" {
			s.Capture.Job.Jobs[0].Output = ProtectedJobOutputV1{Availability: "unavailable"}
		}
		if ValidateProtectedDisplaySnapshotV1(s) != nil {
			t.Fatal("existing display operation pair changed", tool)
		}
	}
}
