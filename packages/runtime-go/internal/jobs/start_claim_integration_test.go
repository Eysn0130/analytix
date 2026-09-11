package jobs_test

import (
	"context"
	"crypto/sha256"
	"sync/atomic"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobs "analytix.local/runtime-go/internal/jobs"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestForegroundChildKillBeforeAuthorizeProducesZeroStartTurn(t *testing.T) {
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := startForegroundChildFixture(t, manager, "child-one")
	release := make(chan struct{})
	barrier := &jobStartClaimBarrier{manager: manager, entered: make(chan string, 1), release: release}
	driver := &jobStartCASCompletionDriver{manager: manager}
	resultCh := make(chan subagentapp.CompleteTaskResult, 1)
	go func() {
		resultCh <- completeForegroundChildWithAuthority(record, driver, barrier)
	}()

	waitForJobStartClaim(t, barrier.entered, record.ID)
	service := subagentapp.NewService(subagentapp.Dependencies{Jobs: manager})
	killed := service.KillTaskJob(record.ParentThreadID, subagentapp.TaskJobKillRequest{JobID: record.ID, Reason: "kill won before start claim"})
	if killed.IsError || killed.Record.Status != string(domainjob.StatusKilled) {
		t.Fatalf("kill did not durably win before authorization: %#v", killed)
	}
	close(release)
	result := waitForCompleteTaskResult(t, resultCh)
	if driver.starts.Load() != 0 {
		t.Fatalf("child StartTurn ran after kill won durable CAS: starts=%d", driver.starts.Load())
	}
	if !result.IsError || result.Record.Status != string(domainjob.StatusKilled) {
		t.Fatalf("killed durable record was not preserved: %#v", result)
	}
}

func TestParallelForegroundChildrenKilledBeforeAuthorizeProduceZeroStartTurns(t *testing.T) {
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	records := []domainjob.Record{
		startForegroundChildFixture(t, manager, "parallel-one"),
		startForegroundChildFixture(t, manager, "parallel-two"),
	}
	release := make(chan struct{})
	barrier := &jobStartClaimBarrier{manager: manager, entered: make(chan string, len(records)), release: release}
	driver := &jobStartCASCompletionDriver{manager: manager}
	resultCh := make(chan subagentapp.CompleteTaskResult, len(records))
	for _, record := range records {
		record := record
		go func() {
			resultCh <- completeForegroundChildWithAuthority(record, driver, barrier)
		}()
	}
	seen := map[string]bool{}
	for range records {
		seen[waitForAnyJobStartClaim(t, barrier.entered)] = true
	}
	for _, record := range records {
		if !seen[record.ID] {
			t.Fatalf("parallel child did not reach authorization barrier: seen=%#v", seen)
		}
	}
	service := subagentapp.NewService(subagentapp.Dependencies{Jobs: manager})
	for _, record := range records {
		killed := service.KillTaskJob(record.ParentThreadID, subagentapp.TaskJobKillRequest{JobID: record.ID, Reason: "parallel kill won before start claim"})
		if killed.IsError || killed.Record.Status != string(domainjob.StatusKilled) {
			t.Fatalf("parallel kill did not durably win for %s: %#v", record.ID, killed)
		}
	}
	close(release)
	for range records {
		result := waitForCompleteTaskResult(t, resultCh)
		if !result.IsError || result.Record.Status != string(domainjob.StatusKilled) {
			t.Fatalf("parallel killed record was not preserved: %#v", result)
		}
	}
	if driver.starts.Load() != 0 {
		t.Fatalf("parallel child StartTurn ran after kills won durable CAS: starts=%d", driver.starts.Load())
	}
}

func TestForegroundChildKillAfterDurableClaimBeforeStartAuthorityProducesZeroStartTurn(t *testing.T) {
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	securityContext, binding := foregroundChildSecurityFixture(t)
	startRequest := domainjob.StartRequest{
		ParentGoalID: "goal_post_claim", ParentThreadID: securityContext.ThreadID, ParentTurnID: securityContext.TurnID,
		ParentToolCallID: binding.ParentToolCallID, ChildThreadID: "thread_post_claim_child", SecurityBinding: binding,
		Kind: "subagent", Name: "post-claim-child", Status: string(domainjob.StatusRunning),
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	state := subagentapp.NewRuntimeState()
	if err := state.ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), securityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	control, jobCtx, err := subagentapp.BeginBoundChildAdmission(context.Background(), false, state, record.ID, binding)
	if err != nil {
		t.Fatal(err)
	}
	claimed := make(chan struct{})
	releaseClaim := make(chan struct{})
	driver := &jobStartCASCompletionDriver{manager: manager}
	resultCh := make(chan subagentapp.CompleteTaskResult, 1)
	go func() {
		defer control.Close()
		resultCh <- subagentapp.CompleteTask(jobCtx, subagentapp.CompleteTaskInput{
			Request: subagentapp.TaskRequest{Prompt: "must not start after post-claim kill"},
			Record:  record, Driver: driver, StartAuthority: control,
			AuthorizeStart: func(current domainjob.Record) (domainjob.Record, error) {
				claimedRecord, claimErr := subagentapp.AuthorizeJobStart(current, true, manager, func(domainjob.Record) string { return "" }, nil)
				if claimErr == nil {
					close(claimed)
					<-releaseClaim
				}
				return claimedRecord, claimErr
			},
		})
	}()
	select {
	case <-claimed:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not durably claim start authority")
	}

	service := subagentapp.NewService(subagentapp.Dependencies{Jobs: manager, State: state})
	killCh := make(chan subagentapp.TaskJobServiceResult, 1)
	go func() {
		killCh <- service.KillTaskJob(record.ParentThreadID, subagentapp.TaskJobKillRequest{JobID: record.ID, Reason: "kill won after durable claim"})
	}()
	select {
	case <-jobCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("kill did not close foreground child start authority")
	}
	close(releaseClaim)
	result := waitForCompleteTaskResult(t, resultCh)
	var killed subagentapp.TaskJobServiceResult
	select {
	case killed = <-killCh:
	case <-time.After(5 * time.Second):
		t.Fatal("kill did not observe foreground worker termination")
	}
	if killed.IsError || killed.Record.Status != string(domainjob.StatusKilled) {
		t.Fatalf("post-claim kill was not durably acknowledged: %#v", killed)
	}
	if driver.starts.Load() != 0 {
		t.Fatalf("child StartTurn/effect ran after post-claim kill: starts=%d", driver.starts.Load())
	}
	if !result.IsError || result.Record.Status != string(domainjob.StatusKilled) {
		t.Fatalf("post-claim cancellation did not preserve killed state: %#v", result)
	}
}

func foregroundChildSecurityFixture(t *testing.T) (domainsecurity.TurnSecurityContext, *domainjob.SecurityBinding) {
	t.Helper()
	now := time.Now().UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread_post_claim_parent", TurnID: "turn_post_claim_parent", WorkspaceRealPath: "/workspace/post-claim",
		CaseID: "case_post_claim", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-post-claim")),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot_post_claim"), SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-post-claim")),
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	entropy := sha256.Sum256([]byte("jobs-start-claim-post-claim"))
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_post_claim", ServerIdentity: "host:builtin", ToolName: "task",
		ToolCallID: toolCallID, ArgsHash: domainsecurity.SHA256Hex([]byte("args-post-claim")),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema-post-claim")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope-post-claim")),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext, binding
}

type jobStartClaimBarrier struct {
	manager *jobs.Manager
	entered chan string
	release <-chan struct{}
}

func (barrier *jobStartClaimBarrier) LoadChildRun(id string) (domainjob.Record, error) {
	return barrier.manager.LoadChildRun(id)
}

func (barrier *jobStartClaimBarrier) ClaimChildRunStart(expected domainjob.Record) (domainjob.Record, error) {
	barrier.entered <- expected.ID
	<-barrier.release
	return barrier.manager.ClaimChildRunStart(expected)
}

type jobStartCASCompletionDriver struct {
	manager *jobs.Manager
	starts  atomic.Int64
}

func (driver *jobStartCASCompletionDriver) StartTurn(context.Context, controlapp.StartTurnRequest) (map[string]any, error) {
	driver.starts.Add(1)
	return map[string]any{"turnId": "unexpected-turn", "status": "completed"}, nil
}

func (driver *jobStartCASCompletionDriver) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	return driver.manager.UpdateChildRun(id, request)
}

func (driver *jobStartCASCompletionDriver) LoadChildRun(id string) (domainjob.Record, error) {
	return driver.manager.LoadChildRun(id)
}

func (*jobStartCASCompletionDriver) TurnCompletion(string, string) (string, int, error) {
	return "unexpected summary", 0, nil
}
func (*jobStartCASCompletionDriver) UsageSnapshot(string) (map[string]any, error) { return nil, nil }
func (*jobStartCASCompletionDriver) CacheDiagnostics(string, string) (map[string]any, error) {
	return nil, nil
}
func (*jobStartCASCompletionDriver) RecordProgress(domainjob.Record, string, string) {}

func startForegroundChildFixture(t *testing.T, manager *jobs.Manager, name string) domainjob.Record {
	t.Helper()
	record, err := manager.StartChildRun(domainjob.StartRequest{
		ParentGoalID:     "goal_job_start_cas",
		ParentThreadID:   "thread_job_start_cas",
		ParentTurnID:     "turn_job_start_cas",
		ParentToolCallID: "call_job_start_cas_" + name,
		ChildThreadID:    "thread_" + name,
		Kind:             "subagent",
		Name:             name,
		Status:           string(domainjob.StatusRunning),
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func completeForegroundChildWithAuthority(record domainjob.Record, driver *jobStartCASCompletionDriver, barrier *jobStartClaimBarrier) subagentapp.CompleteTaskResult {
	return subagentapp.CompleteTask(context.Background(), subagentapp.CompleteTaskInput{
		Request:        subagentapp.TaskRequest{Prompt: "must not start after kill"},
		Record:         record,
		Driver:         driver,
		StartAuthority: subagentapp.NewBackgroundJobStartBarrier(),
		AuthorizeStart: func(current domainjob.Record) (domainjob.Record, error) {
			return subagentapp.AuthorizeJobStart(current, true, barrier, func(domainjob.Record) string { return "" }, nil)
		},
	})
}

func waitForJobStartClaim(t *testing.T, entered <-chan string, expected string) {
	t.Helper()
	if actual := waitForAnyJobStartClaim(t, entered); actual != expected {
		t.Fatalf("unexpected job at start claim barrier: got=%q want=%q", actual, expected)
	}
}

func waitForAnyJobStartClaim(t *testing.T, entered <-chan string) string {
	t.Helper()
	select {
	case jobID := <-entered:
		return jobID
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for job start claim barrier")
		return ""
	}
}

func waitForCompleteTaskResult(t *testing.T, results <-chan subagentapp.CompleteTaskResult) subagentapp.CompleteTaskResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for foreground child completion")
		return subagentapp.CompleteTaskResult{}
	}
}
