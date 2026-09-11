package threadsummary

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func threadSummaryTestToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.thread-summary-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func TestServiceBuildsSummaryReadModel(t *testing.T) {
	now := time.Date(2026, 7, 2, 7, 0, 0, 0, time.UTC)
	const turnID = "turn-1"
	toolCallID := threadSummaryTestToolCallID("build-summary")
	repo := &summaryRepositoryStub{
		threads: map[string]map[string]any{
			"thread-1": {
				"id":             "thread-1",
				"workspace":      "/workspace",
				"approvalPolicy": "on-request",
				"sandboxMode":    "workspace-write",
				"turns": []any{
					map[string]any{"id": turnID, "threadId": "thread-1", "items": []any{
						map[string]any{"id": "user-1", "kind": "user_message", "displayText": "build it", "createdAt": "2026-07-02T06:59:00Z"},
						map[string]any{"id": domaintoolcall.ToolCallItemIDV1(turnID, toolCallID), "kind": "tool_call", "role": "assistant", "status": "completed", "turnId": turnID, "toolName": "bash", "callId": toolCallID, "createdAt": "2026-07-02T07:00:00Z", "arguments": map[string]any{"command": "npm test", "cwd": "/workspace"}},
						map[string]any{
							"id": domaintoolresult.ToolResultItemIDV1(turnID, toolCallID), "kind": "tool_result", "role": "tool",
							"status": "completed", "isError": false, "turnId": turnID, "toolKind": "command_execution",
							"toolName": "bash", "callId": toolCallID, "createdAt": "2026-07-02T07:00:01Z",
							"output": domaintoolresult.PublicToolResultProjectionRecordV1(
								domaintoolresult.WithheldProjectionV1("completed", ""),
							),
						},
					}},
				},
			},
			"child-1": {
				"id":    "child-1",
				"title": "Investigator",
				"turns": []any{map[string]any{"items": []any{map[string]any{"kind": "user_message", "displayText": "child prompt"}}}},
			},
			"side-1": {
				"id":             "side-1",
				"parentThreadId": "thread-1",
				"relation":       "side",
				"title":          "Side chat",
			},
		},
		highestSeq: 4,
	}
	jobs := &summaryJobsStub{records: []domainjob.Record{
		{ID: "run-1", Kind: "subagent", ParentThreadID: "thread-1", ChildThreadID: "child-1", Status: "completed", StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), FinishedAt: now.Format(time.RFC3339Nano)},
		{ID: "job-1", Kind: "background-shell", ParentThreadID: "thread-1", Status: "running", Label: "npm test", Workspace: "/workspace", StartedAt: now.Format(time.RFC3339Nano)},
	}}
	service := NewService(Dependencies{
		Repository: repo,
		Jobs:       jobs,
		Now:        func() time.Time { return now },
	})

	summary, err := service.Summary("thread-1")
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	if summary["threadId"] != "thread-1" || summary["latestSeq"] != float64(4) {
		t.Fatalf("summary identity mismatch: %#v", summary)
	}
	subagents := summary["subagents"].([]any)
	if len(subagents) != 1 || subagents[0].(map[string]any)["displayName"] != "Investigator" {
		t.Fatalf("subagent projection mismatch: %#v", subagents)
	}
	if len(summary["sideChats"].([]map[string]any)) != 1 {
		t.Fatalf("side chat projection mismatch: %#v", summary["sideChats"])
	}
	tasks := summary["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("expected task job and command task, got %#v", tasks)
	}
	if len(summary["outputs"].([]any)) != 0 || len(summary["sources"].([]any)) != 0 {
		t.Fatalf("legacy raw outputs or source-like metadata were not withheld: %#v", summary)
	}
}

func TestServiceKeepsDurableSubagentStatusOverStaleReplayEvent(t *testing.T) {
	now := time.Date(2026, 7, 2, 7, 0, 3, 0, time.UTC)
	repo := &summaryRepositoryStub{
		threads: map[string]map[string]any{
			"thread-1": {"id": "thread-1"},
			"child-1":  {"id": "child-1", "title": "Child"},
		},
		events: []map[string]any{
			{
				"timestamp": "2026-07-02T07:00:01Z",
				"child": map[string]any{
					"parentThreadId": "thread-1",
					"childRunId":     "job-1",
					"childThreadId":  "child-1",
					"childStatus":    "queued",
				},
			},
		},
	}
	jobs := &summaryJobsStub{records: []domainjob.Record{
		{
			ID:             "job-1",
			Kind:           "subagent",
			ParentThreadID: "thread-1",
			ChildThreadID:  "child-1",
			Status:         "completed",
			StartedAt:      "2026-07-02T07:00:00Z",
			FinishedAt:     "2026-07-02T07:00:02Z",
			UpdatedAt:      "2026-07-02T07:00:02Z",
			Output:         "done",
		},
	}}
	service := NewService(Dependencies{
		Repository: repo,
		Jobs:       jobs,
		Now:        func() time.Time { return now },
	})

	summary, err := service.Summary("thread-1")
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	subagents := summary["subagents"].([]any)
	if len(subagents) != 1 {
		t.Fatalf("expected one subagent, got %#v", subagents)
	}
	subagent := subagents[0].(map[string]any)
	if subagent["rawStatus"] != "completed" || subagent["status"] != "done" {
		t.Fatalf("stale replay event should not override durable job status: %#v", subagent)
	}
}

func TestServiceProjectsSecretsAndPIIFromEventsJobsAndChildThreads(t *testing.T) {
	now := time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC)
	const secret = "opaque-summary-secret-123"
	const account = "6222020202020202020"
	repo := &summaryRepositoryStub{
		threads: map[string]map[string]any{
			"thread-1": {"id": "thread-1", "title": "Parent", "turns": []any{map[string]any{"id": "turn-1", "threadId": "thread-1", "items": []any{}}}},
			"child-1": {
				"id": "child-1", "parentThreadId": "thread-1", "title": "Bearer " + secret,
				"turns": []any{map[string]any{"id": "child-turn", "threadId": "child-1", "items": []any{
					map[string]any{"id": "child-user", "kind": "user_message", "displayText": "account number: " + account},
				}}},
			},
			"side-1": {"id": "side-1", "parentThreadId": "thread-1", "relation": "side", "title": "api_key=" + secret, "turns": []any{}},
		},
		events: []map[string]any{{
			"kind": "tool_progress", "threadId": "thread-1", "turnId": "turn-1",
			"toolName": "read_file", "callId": "call-safe", "status": "running",
			"message": "Authorization: Bearer " + secret + " account number: " + account,
		}},
		highestSeq: 7,
	}
	jobs := &summaryJobsStub{records: []domainjob.Record{{
		ID: "run-1", Kind: "subagent", ParentThreadID: "thread-1", ChildThreadID: "child-1",
		Status: "completed", Label: "password=" + secret, UpdatedAt: now.Format(time.RFC3339Nano),
	}}}
	service := NewService(Dependencies{Repository: repo, Jobs: jobs, Now: func() time.Time { return now }})

	summary, err := service.Summary("thread-1")
	body, _ := json.Marshal(summary)
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	if strings.Contains(string(body), secret) || strings.Contains(string(body), account) {
		t.Fatalf("credential or PII crossed thread summary: %s", body)
	}
	if !strings.Contains(string(body), "redacted") && !strings.Contains(string(body), "[ACCOUNT]") {
		t.Fatalf("thread summary lost deterministic public projection: %s", body)
	}
}

func TestServiceCommandOutputIsMetadataOnly(t *testing.T) {
	thread := map[string]any{
		"id":             "thread-1",
		"approvalPolicy": "on-request",
		"sandboxMode":    "workspace-write",
		"turns": []any{map[string]any{"items": []any{
			map[string]any{"id": "call", "kind": "tool_call", "toolName": "bash", "callId": "call-1", "turnId": "turn-1", "createdAt": "2026-07-02T07:00:00Z", "arguments": map[string]any{"command": "npm test", "cwd": "/workspace/pkg"}},
			map[string]any{"id": "result", "kind": "tool_result", "toolKind": "command_execution", "callId": "call-1", "turnId": "turn-1", "createdAt": "2026-07-02T07:00:01Z", "output": map[string]any{"status": "completed", "output": "abcdef"}},
		}}},
	}
	service := NewService(Dependencies{
		Now: func() time.Time { return time.Date(2026, 7, 2, 7, 0, 2, 0, time.UTC) },
	})
	context := Context{Thread: thread}

	output, ok := service.CommandOutput(context, "command:call-1", 2, 3)
	if !ok || output["schemaVersion"] != 1 || output["availability"] != "withheld" || output["reasonCode"] != "tool_output_private" ||
		output["outputWithheld"] != true || output["canReadOutput"] != false || output["factAnswerAllowed"] != false || output["evidenceAuthority"] != false {
		t.Fatalf("command output mismatch: ok=%v output=%#v", ok, output)
	}
}

func TestServiceCaseSummaryWithholdsThreadEventJobAndCommandFacts(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case", TurnID: "turn-case", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	securityRecord := map[string]any{
		"version": securityContext.Version, "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"workspaceRealPath": securityContext.WorkspaceRealPath, "tenantId": securityContext.TenantID, "userId": securityContext.UserID,
		"caseId": securityContext.CaseID, "caseBindingHash": securityContext.CaseBindingHash,
		"datasetSnapshotId": securityContext.DatasetSnapshotID, "sourceManifestHash": securityContext.SourceManifestHash,
		"contextEpoch": securityContext.ContextEpoch, "issuedAt": securityContext.IssuedAt, "contextDigest": securityContext.ContextDigest,
	}
	repo := &summaryRepositoryStub{
		threads: map[string]map[string]any{"thread-case": {
			"id": "thread-case", "title": "Case", "workspace": "/cases/a", "model": "model", "mode": "agent", "status": "idle",
			"createdAt": "2026-07-11T00:00:00Z", "updatedAt": "2026-07-11T00:00:00Z", "securityState": securityRecord,
			"turns": []any{map[string]any{
				"id": "turn-case", "threadId": "thread-case", "status": "completed", "securityContext": securityRecord,
				"items": []any{
					map[string]any{"id": "user", "kind": "user_message", "role": "user", "text": "USER_REQUEST"},
					map[string]any{"id": "call", "kind": "tool_call", "toolName": "bash", "callId": "call-case", "arguments": map[string]any{"command": "echo COMMAND_FACT_SENTINEL"}},
					map[string]any{"id": "result", "kind": "tool_result", "callId": "call-case", "output": "COMMAND_OUTPUT_FACT_SENTINEL"},
				},
			}},
		}},
		events: []map[string]any{{"kind": "item_completed", "message": "EVENT_FACT_SENTINEL"}}, highestSeq: 9,
	}
	jobs := &summaryJobsStub{records: []domainjob.Record{
		{
			ID: "job-case", Kind: "background-shell", ParentThreadID: "thread-case", Status: "completed", Output: "JOB_FACT_SENTINEL",
		},
		{
			ID: "subagent-case", Kind: "subagent", ParentThreadID: "thread-case",
			ParentTurnID: "turn-case", ParentToolCallID: "call-subagent", ChildThreadID: "child-case",
			ChildTurnID: "child-turn-case", Status: "completed", Label: "password=CASE_SUBAGENT_SECRET",
			Prompt: "account number: 6222020202020202020", Output: "CASE_SUBAGENT_OUTPUT_SENTINEL",
			ProfileName: "read-only-reviewer", ToolPolicy: "readOnly", Model: "deepseek-v4-flash",
			StartedAt: "2026-07-11T00:00:01Z", FinishedAt: "2026-07-11T00:00:02Z",
			UpdatedAt: "2026-07-11T00:00:02Z",
		},
	}}
	service := NewService(Dependencies{Repository: repo, Jobs: jobs, Now: func() time.Time { return time.Unix(2, 0) }})
	context, err := service.LoadContext("thread-case")
	if err != nil || !context.CaseRestricted {
		t.Fatalf("case summary context was not restricted: context=%#v err=%v", context, err)
	}
	if len(context.Jobs) != 1 || context.Jobs[0].ID != "subagent-case" ||
		context.Jobs[0].Name != "" || context.Jobs[0].Label != "" ||
		context.Jobs[0].Prompt != "" || context.Jobs[0].Output != "" || context.Jobs[0].Error != "" {
		t.Fatalf("case summary context retained non-lifecycle job fields: %#v", context.Jobs)
	}
	summary := service.SummaryFromContext(context)
	body, _ := json.Marshal(summary)
	for _, sentinel := range []string{
		"COMMAND_FACT_SENTINEL", "COMMAND_OUTPUT_FACT_SENTINEL", "EVENT_FACT_SENTINEL", "JOB_FACT_SENTINEL",
		"CASE_SUBAGENT_SECRET", "6222020202020202020", "CASE_SUBAGENT_OUTPUT_SENTINEL",
	} {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("case summary exposed untrusted content: sentinel=%s summary=%s", sentinel, body)
		}
	}
	if summary["historyAuthority"] != "case_boundary_only_v1" || summary["latestSeq"] != float64(9) {
		t.Fatalf("case summary restriction metadata is incomplete: %#v", summary)
	}
	subagents, _ := summary["subagents"].([]any)
	if len(subagents) != 1 {
		t.Fatalf("case summary did not preserve one closed subagent lifecycle: %#v", summary)
	}
	subagent, _ := subagents[0].(map[string]any)
	if subagent["id"] != "run:subagent-case" || subagent["status"] != "done" ||
		subagent["rawStatus"] != "completed" || subagent["childThreadId"] != "child-case" ||
		subagent["model"] != "deepseek-v4-flash" || subagent["profile"] != "read-only-reviewer" ||
		subagent["toolPolicy"] != "readOnly" ||
		subagent["outputWithheld"] != true || subagent["canReadOutput"] != false ||
		subagent["factAnswerAllowed"] != false || subagent["evidenceAuthority"] != false ||
		subagent["canContinueParent"] != false {
		t.Fatalf("case subagent lifecycle projection is not closed: %#v", subagent)
	}
	if tasks, _ := summary["tasks"].([]any); len(tasks) != 0 {
		t.Fatalf("case summary exposed task or command metadata: %#v", tasks)
	}
	if output, ok := service.CommandOutput(context, "command:call-case", 0, 100); ok || output != nil {
		t.Fatalf("case command output remained available: %#v", output)
	}
}

func TestServiceLoadContextReturnsThreadNotFound(t *testing.T) {
	service := NewService(Dependencies{
		Repository: &summaryRepositoryStub{threads: map[string]map[string]any{}},
		Jobs:       &summaryJobsStub{},
	})
	_, err := service.LoadContext("missing")
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("expected ErrThreadNotFound, got %v", err)
	}
}

type summaryRepositoryStub struct {
	threads    map[string]map[string]any
	events     []map[string]any
	highestSeq int
}

func (r *summaryRepositoryStub) GetThread(threadID string) (map[string]any, error) {
	return r.threads[threadID], nil
}

func (r *summaryRepositoryStub) LoadEventsSince(string, int) ([]map[string]any, error) {
	return r.events, nil
}

func (r *summaryRepositoryStub) HighestSeq(string) (int, error) {
	return r.highestSeq, nil
}

func (r *summaryRepositoryStub) ListThreads(bool, bool, bool, string) ([]map[string]any, error) {
	threads := []map[string]any{}
	for _, thread := range r.threads {
		threads = append(threads, thread)
	}
	return threads, nil
}

type summaryJobsStub struct {
	records []domainjob.Record
}

func (j *summaryJobsStub) ListTaskJobs(string) ([]domainjob.Record, error) {
	return j.records, nil
}
