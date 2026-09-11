package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestExecuteBashToolRunsForegroundThroughService(t *testing.T) {
	service := NewService(Dependencies{ShellRunner: fakeShellRunner{write: "ok"}})
	workspace := t.TempDir()
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			Workspace:         workspace,
			WorkspaceAbsolute: true,
			SandboxMode:       "danger-full-access",
		},
		Args: map[string]any{"command": "printf ok"},
	}, BashToolCallbacks{})
	if isError {
		t.Fatalf("foreground bash should succeed: %#v", payload)
	}
	body, ok := payload.(map[string]any)
	if !ok || body["status"] != "completed" || body["output"] != "ok" {
		t.Fatalf("unexpected foreground payload: %#v", payload)
	}
}

func TestExecuteBashToolStartsBackgroundAndRegistersControl(t *testing.T) {
	repo := &fakeJobRepository{}
	service := NewService(Dependencies{Jobs: repo, ShellRunner: fakeShellRunner{write: "background ok"}})
	workspace := t.TempDir()
	binding := testBackgroundBashSecurityBinding(t, "thread-1", "turn-1", workspace)
	done := make(chan struct{})
	registered := ""
	unregistered := ""
	progress := []string{}
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID:            "thread-1",
			TurnID:              "turn-1",
			ToolCallID:          binding.ParentToolCallID,
			Workspace:           workspace,
			WorkspaceAbsolute:   true,
			SandboxMode:         "danger-full-access",
			ParentGoalID:        "goal-1",
			ParentGoalObjective: "finish work",
			SecurityBinding:     binding,
		},
		Args: map[string]any{"command": "printf ok", "run_in_background": true},
	}, BashToolCallbacks{
		RegisterBoundBackgroundJob: func(jobID string, got *domainjob.SecurityBinding, cancel context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
			if got == nil || got.BindingDigest != binding.BindingDigest {
				t.Fatalf("registered binding mismatch: %#v", got)
			}
			registered = jobID
			return subagentapp.NewBackgroundJobStartBarrier(), true
		},
		UnregisterBackgroundJob: func(jobID string) {
			unregistered = jobID
			close(done)
		},
		OnBackgroundProgress: func(record JobRecord, status string, diagnostic string) {
			progress = append(progress, status+":"+diagnostic)
		},
	})
	if isError {
		t.Fatalf("background bash should start: %#v", payload)
	}
	body, ok := payload.(map[string]any)
	if !ok || body["kind"] != "subagent_task" || body["jobId"] != "job-1" || body["outputWithheld"] != true {
		t.Fatalf("unexpected background payload: %#v", payload)
	}
	if registered != "job-1" {
		t.Fatalf("background job was not registered: %q", registered)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background bash did not complete")
	}
	if unregistered != "job-1" || repo.record.Status != "completed" {
		t.Fatalf("background job did not settle: unregistered=%q record=%#v", unregistered, repo.record)
	}
	if len(progress) == 0 {
		t.Fatal("expected background progress callbacks")
	}
}

func TestBackgroundBashRevalidatesAuthorityBeforeShellStart(t *testing.T) {
	repo := &fakeJobRepository{}
	shellStarted := make(chan struct{}, 1)
	service := NewService(Dependencies{Jobs: repo, ShellRunner: fakeShellRunner{onRun: func() {
		shellStarted <- struct{}{}
	}}})
	done := make(chan struct{})
	workspace := t.TempDir()
	binding := testBackgroundBashSecurityBinding(t, "thread-1", "turn-1", workspace)
	_, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: binding.ParentToolCallID,
			Workspace: workspace, WorkspaceAbsolute: true, SandboxMode: "danger-full-access", SecurityBinding: binding,
		},
		Args: map[string]any{"command": "printf should-not-run", "run_in_background": true},
	}, BashToolCallbacks{
		RegisterBoundBackgroundJob: func(string, *domainjob.SecurityBinding, context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
			return subagentapp.NewBackgroundJobStartBarrier(), true
		},
		AuthorizeBackgroundStart: func(record JobRecord) (JobRecord, error) {
			return record, errors.New("task job authority rejected before execution: parent_security_context_mismatch")
		},
		UnregisterBackgroundJob: func(string) {
			close(done)
		},
	})
	if isError {
		t.Fatal("background job admission should return before asynchronous authority revalidation")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background authority rejection did not settle")
	}
	select {
	case <-shellStarted:
		t.Fatal("shell runner started despite stale background authority")
	default:
	}
	if repo.record.Status != string(domainjob.StatusInterrupted) {
		t.Fatalf("stale background authority should interrupt the job: %#v", repo.record)
	}
}

func TestBackgroundBashRejectsLateBoundRegistrationBeforeShellStart(t *testing.T) {
	repo := &fakeJobRepository{}
	shellStarted := false
	service := NewService(Dependencies{Jobs: repo, ShellRunner: fakeShellRunner{onRun: func() { shellStarted = true }}})
	workspace := t.TempDir()
	binding := testBackgroundBashSecurityBinding(t, "thread-1", "turn-1", workspace)
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: binding.ParentToolCallID,
			Workspace: workspace, WorkspaceAbsolute: true, SandboxMode: "danger-full-access", SecurityBinding: binding,
		},
		Args: map[string]any{"command": "printf should-not-run", "run_in_background": true},
	}, BashToolCallbacks{
		RegisterBoundBackgroundJob: func(string, *domainjob.SecurityBinding, context.CancelFunc) (*subagentapp.BackgroundJobStartBarrier, bool) {
			return nil, false
		},
	})
	if !isError {
		t.Fatalf("stale bound registration should fail synchronously: %#v", payload)
	}
	if shellStarted {
		t.Fatal("shell runner started after stale bound registration")
	}
	if repo.record.Status != string(domainjob.StatusInterrupted) {
		t.Fatalf("stale registration did not interrupt durable job: %#v", repo.record)
	}
}

func TestBackgroundBashRejectsMissingSecurityBindingBeforeDurableStart(t *testing.T) {
	repo := &fakeJobRepository{}
	service := NewService(Dependencies{Jobs: repo, ShellRunner: fakeShellRunner{write: "must not run"}})
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: "call-1",
			Workspace: t.TempDir(), WorkspaceAbsolute: true, SandboxMode: "danger-full-access",
		},
		Args: map[string]any{"command": "printf must-not-run", "run_in_background": true},
	}, BashToolCallbacks{})
	body, _ := payload.(map[string]any)
	if !isError || body["code"] != "background_shell_stale_context" {
		t.Fatalf("missing binding did not fail closed: payload=%#v isError=%t", payload, isError)
	}
	if repo.record.ID != "" || repo.lastStart.ParentThreadID != "" {
		t.Fatalf("missing binding created a durable job: record=%#v request=%#v", repo.record, repo.lastStart)
	}
}

func TestBackgroundBashInFlightStopsBeforeNewCaseAuthorityReturns(t *testing.T) {
	now := time.Now().UTC()
	toolCallEntropy := sha256.Sum256([]byte("terminal-in-flight-case-authority"))
	toolCallID, err := domainmodel.NewHostToolCallIDV1(toolCallEntropy[:])
	if err != nil {
		t.Fatal(err)
	}
	contextA, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-a", WorkspaceRealPath: "/workspace/shared", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: contextA, Provider: "provider-1", ServerIdentity: "host:builtin", ToolName: "bash", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(contextA, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	state := subagentapp.NewRuntimeState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextA, time.Second); err != nil {
		t.Fatal(err)
	}
	shellStarted := make(chan struct{}, 1)
	repo := &fakeJobRepository{}
	service := NewService(Dependencies{Jobs: repo, ShellRunner: fakeShellRunner{
		write: "old-case-first-write", waitForDone: true, onRun: func() { shellStarted <- struct{}{} },
	}})
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			ThreadID: contextA.ThreadID, TurnID: contextA.TurnID, ToolCallID: grant.ToolCallID,
			Workspace: t.TempDir(), WorkspaceAbsolute: true, SandboxMode: "danger-full-access", SecurityBinding: binding,
		},
		Args: map[string]any{"command": "wait", "run_in_background": true},
	}, BashToolCallbacks{
		RegisterBoundBackgroundJob: state.RegisterBoundBackgroundJobWithStartBarrier,
		UnregisterBackgroundJob:    state.UnregisterBackgroundJob,
		AuthorizeBackgroundStart:   func(record JobRecord) (JobRecord, error) { return record, nil },
	})
	if isError {
		t.Fatalf("background shell admission failed: %#v", payload)
	}
	select {
	case <-shellStarted:
	case <-time.After(time.Second):
		t.Fatal("background shell did not reach the in-flight boundary")
	}
	contextB, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: contextA.ThreadID, TurnID: "turn-b", WorkspaceRealPath: contextA.WorkspaceRealPath, CaseID: "case-b",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-b")), ContextEpoch: 2, IssuedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), contextB, time.Second); err != nil {
		t.Fatalf("new case authority did not wait for the in-flight shell: %v", err)
	}
	if repo.record.Status != "killed" {
		t.Fatalf("in-flight old-case shell was not canceled before authority returned: %#v", repo.record)
	}
}

func TestBackgroundBashToolOutputWithholdsSecurityBoundCommand(t *testing.T) {
	output := BackgroundBashToolOutput(JobRecord{
		ID: "job-security-bound", Status: string(domainjob.StatusRunning), Background: true,
		SecurityBinding: &domainjob.SecurityBinding{},
	}, "secret-command", "/secret/workspace", 99, time.Now().UTC())
	if len(output) != 15 || output["kind"] != "subagent_task" || output["jobId"] != "job-security-bound" ||
		output["status"] != "running" || output["background"] != true || output["outputWithheld"] != true ||
		output["outputTrustStatus"] != "untrusted_child_output" || output["factAnswerAllowed"] != false ||
		output["evidenceAuthority"] != false || output["canReadOutput"] != false || output["canContinueParent"] != false {
		t.Fatalf("security-bound background shell did not use the closed projection: %#v", output)
	}
	for _, forbidden := range []string{"command", "cwd", "timeoutSeconds", "output", "error", "prompt"} {
		if _, found := output[forbidden]; found {
			t.Fatalf("security-bound background shell exposed %q: %#v", forbidden, output)
		}
	}
}

func TestBackgroundBashToolOutputNeverEchoesCommandOrWorkspace(t *testing.T) {
	output := BackgroundBashToolOutput(
		JobRecord{ID: "job-ordinary", Kind: "background-shell", Status: "running", Background: true},
		"curl -H 'Authorization: Bearer PRIVATE_COMMAND_SECRET' https://example.test",
		"/workspace/PRIVATE_PATH_SECRET", 30, time.Now().UTC(),
	)
	for _, forbidden := range []string{"command", "cwd", "timeoutSeconds"} {
		if _, found := output[forbidden]; found {
			t.Fatalf("background tool output exposed %q: %#v", forbidden, output)
		}
	}
	encoded, _ := json.Marshal(output)
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatalf("background tool output exposed command or workspace bytes: %s", encoded)
	}
}

func TestExecuteBashToolRejectsBackgroundInsideSubagent(t *testing.T) {
	service := NewService(Dependencies{ShellRunner: fakeShellRunner{write: "ignored"}})
	workspace := t.TempDir()
	payload, isError := service.ExecuteBashTool(context.Background(), BashToolRunRequest{
		Context: BashToolContext{
			Workspace:         workspace,
			WorkspaceAbsolute: true,
			SandboxMode:       "danger-full-access",
			SubagentDepth:     1,
		},
		Args: map[string]any{"command": "printf ok", "runInBackground": true},
	}, BashToolCallbacks{})
	body, ok := payload.(map[string]any)
	if !isError || !ok || body["code"] != "background_shell_unavailable" {
		t.Fatalf("unexpected nested background result: %#v isError=%t", payload, isError)
	}
}

func testBackgroundBashSecurityBinding(t *testing.T, threadID string, turnID string, workspace string) *domainjob.SecurityBinding {
	t.Helper()
	now := time.Unix(1_700_000_000, 0).UTC()
	entropy := sha256.Sum256([]byte(threadID + "\x00" + turnID + "\x00" + workspace))
	toolCallID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-test", ServerIdentity: "host:builtin", ToolName: "bash", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, toolCallID)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}
