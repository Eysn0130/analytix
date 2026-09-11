package subagent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
)

type foregroundCleanupIssuerStub struct {
	deleted      []string
	prepareErr   error
	prepareCalls int
}

func (stub *foregroundCleanupIssuerStub) Prepare(context.Context, domainjob.Record, string) (PreparedForegroundHandoff, error) {
	stub.prepareCalls++
	if stub.prepareErr != nil {
		return PreparedForegroundHandoff{}, stub.prepareErr
	}
	return PreparedForegroundHandoff{}, errors.New("unused")
}

func (stub *foregroundCleanupIssuerStub) Consume(context.Context, domainjob.Record, PreparedForegroundHandoff) (VerifiedForegroundHandoff, error) {
	return VerifiedForegroundHandoff{}, errors.New("unused")
}

func (stub *foregroundCleanupIssuerStub) Delete(childRunID string) {
	stub.deleted = append(stub.deleted, childRunID)
}

func TestCompleteTaskDestroysForegroundCapabilityOnEveryEarlyFailure(t *testing.T) {
	authority := &foregroundCleanupIssuerStub{}
	result := CompleteTask(context.Background(), CompleteTaskInput{
		Record: domainjob.Record{ID: "foreground-run", ToolScope: []string{toolcatalogForegroundSubmitToolName}},
		Effort: "invalid", ForegroundAuthority: authority,
	})
	if !result.IsError || len(authority.deleted) != 1 || authority.deleted[0] != "foreground-run" {
		t.Fatalf("early foreground failure retained a live capability: result=%#v deleted=%#v", result, authority.deleted)
	}
}

func TestCompleteTaskDestroysForegroundCapabilityOnCanceledAndTimedOutStart(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	timedOut, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for name, runContext := range map[string]context.Context{"canceled": canceled, "timed_out": timedOut} {
		t.Run(name, func(t *testing.T) {
			record := domainjob.Record{
				ID: "foreground-" + name, ChildThreadID: "child-" + name,
				Status: string(domainjob.StatusRunning), Effort: "medium",
				ToolScope: []string{toolcatalogForegroundSubmitToolName},
			}
			driver := newCompletionDriverStub(record)
			authority := &foregroundCleanupIssuerStub{}
			result := CompleteTask(runContext, CompleteTaskInput{
				Request: TaskRequest{Prompt: "bounded"}, Record: record, Effort: "medium",
				Driver: driver, StartAuthority: NewBackgroundJobStartBarrier(), ForegroundAuthority: authority,
			})
			if !result.IsError || len(authority.deleted) != 1 || authority.deleted[0] != record.ID || len(driver.startRequests) != 0 {
				t.Fatalf("%s foreground start retained a capability: result=%#v deleted=%#v starts=%#v", name, result, authority.deleted, driver.startRequests)
			}
		})
	}
}

func TestCompleteTaskCarriesForegroundOutputTokenBudgetToChildTurn(t *testing.T) {
	record := domainjob.Record{
		ID: "foreground-budget", ChildThreadID: "foreground-budget-thread",
		Status: string(domainjob.StatusRunning), Effort: "medium",
		ToolScope: []string{toolcatalogForegroundSubmitToolName},
	}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "foreground-budget-turn", "status": "completed"}
	driver.summary = "Foreground child result submitted."
	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request: TaskRequest{
			Prompt: "bounded", MaxSteps: 2, MaxStepsSet: true,
			TokenBudget: 512, TokenBudgetSet: true, TimeBudgetMS: 30000, TimeBudgetMSSet: true,
		},
		Record: record, ProviderID: "provider", Model: "model", Effort: "medium",
		Driver: driver, StartAuthority: NewBackgroundJobStartBarrier(),
		ForegroundAuthority: &foregroundCleanupIssuerStub{},
	})
	if !result.IsError || len(driver.startRequests) != 1 || driver.startRequests[0].InternalOutputTokenBudget != 512 {
		t.Fatalf("foreground token budget did not reach child turn: result=%#v requests=%#v", result, driver.startRequests)
	}
}

func TestCompleteTaskStartsChildTurnAndPersistsCompletion(t *testing.T) {
	maxSteps := 8
	record := domainjob.Record{
		ID:             "child-1",
		ChildThreadID:  "thread-child",
		Status:         string(domainjob.StatusRunning),
		EndpointFormat: "openai-chat-completions",
		Effort:         "medium",
	}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{
		"turnId":           "turn-child",
		"status":           "completed",
		"usage":            map[string]any{"totalTokens": float64(42)},
		"cacheDiagnostics": map[string]any{"prefixReused": true},
	}
	driver.summary = "child summary"
	driver.toolInvocations = 3
	driver.usage = map[string]any{"totalTokens": float64(999)}
	driver.cacheDiagnostics = map[string]any{"prefixReused": false}

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:              TaskRequest{Prompt: "inspect this", ToolPolicy: "readOnly"},
		Record:               record,
		ProviderID:           "deepseek",
		Model:                "deepseek-chat",
		Effort:               "medium",
		ParentMaxModelSteps:  &maxSteps,
		ParentApprovalPolicy: "on-request",
		ParentSandboxMode:    "workspace-write",
		ParentSubagentDepth:  1,
		ToolScope:            []string{"read_file"},
		Driver:               driver,
		StartAuthority:       NewBackgroundJobStartBarrier(),
	})

	if result.IsError {
		t.Fatalf("CompleteTask returned error output: %#v", result.Output)
	}
	if len(driver.startRequests) != 1 {
		t.Fatalf("expected one child turn start, got %d", len(driver.startRequests))
	}
	start := driver.startRequests[0]
	if start.ThreadID != "thread-child" ||
		start.ProviderID != "deepseek" ||
		start.Model != "deepseek-chat" ||
		start.EndpointFormat != "" ||
		start.ApprovalPolicy != "never" ||
		start.SandboxMode != "workspace-write" ||
		!start.DisableUserInput ||
		!start.DisableUserInputSet ||
		start.InternalUsageSource != usageapp.SourceSubagent ||
		start.InternalSystemPrompt != SystemPromptForRequest(TaskRequest{Prompt: "inspect this", ToolPolicy: "readOnly"}) ||
		start.InternalChildRunID != "child-1" ||
		start.InternalSubagentDepth != 2 {
		t.Fatalf("child turn request mismatch: %#v", start)
	}
	if start.MaxModelSteps == nil || *start.MaxModelSteps != DefaultMaxSteps(&maxSteps) {
		t.Fatalf("max model steps mismatch: %#v", start.MaxModelSteps)
	}
	if len(start.InternalToolScope) != 1 || start.InternalToolScope[0] != "read_file" {
		t.Fatalf("tool scope mismatch: %#v", start.InternalToolScope)
	}
	if result.Record.Status != string(domainjob.StatusCompleted) ||
		result.Record.ChildTurnID != "turn-child" ||
		result.Record.Output != "child summary" ||
		result.Record.ToolInvocations != 3 {
		t.Fatalf("completed record mismatch: %#v", result.Record)
	}
	if result.Output["usage"] != nil || result.Output["outputWithheld"] != true || result.Output["canReadOutput"] != false {
		t.Fatalf("completion output escaped the fixed metadata projection: %#v", result.Output)
	}
	if driver.usageSnapshots != 0 || driver.cacheDiagnosticsCalls != 0 {
		t.Fatalf("completion should use start response usage/cache without index fallback: usage=%d cache=%d", driver.usageSnapshots, driver.cacheDiagnosticsCalls)
	}
	if len(driver.progress) != 1 || driver.progress[0].status != string(domainjob.StatusCompleted) {
		t.Fatalf("progress mismatch: %#v", driver.progress)
	}
}

func TestCompleteTaskUsesDurableTypedPromptForCaseBoundChild(t *testing.T) {
	_, binding := runtimeStateSecurityFixture(t, "thread-parent-prompt", "turn-parent-prompt", "case-prompt", "snapshot-prompt", 1)
	record := domainjob.Record{
		ID: "child-case-prompt", ParentThreadID: binding.ParentThreadID, ParentTurnID: binding.ParentTurnID,
		ParentToolCallID: binding.ParentToolCallID, ChildThreadID: "thread-child-prompt",
		Kind: "subagent", Status: string(domainjob.StatusRunning), SecurityBinding: binding, Effort: "auto",
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "turn-child-prompt", "status": "waiting"}
	const rawProviderBody = "RAW_PROVIDER_CHILD_BODY_93AF 13800138000 /Users/private/case.json"
	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request: TaskRequest{Prompt: rawProviderBody}, Record: record, Effort: "auto", Driver: driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})
	if !result.IsError || len(driver.startRequests) != 1 {
		t.Fatalf("case-bound child did not reach the expected bounded start seam: result=%#v requests=%#v", result, driver.startRequests)
	}
	started := driver.startRequests[0]
	if started.Prompt != record.Prompt || strings.Contains(started.Prompt, rawProviderBody) ||
		strings.Contains(started.Prompt, "13800138000") || strings.Contains(started.Prompt, "/Users/private") {
		t.Fatalf("case-bound StartTurnRequest used ephemeral/raw prompt: %#v", started)
	}
}

func TestCompleteTaskRejectsReasoningEffortPoisonBeforeDriverEffects(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for name, input := range map[string]CompleteTaskInput{
		"input poison": {
			Record: domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning), Effort: "high"},
			Effort: sentinel,
		},
		"record poison": {
			Record: domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning), Effort: sentinel},
			Effort: "high",
		},
		"authority mismatch": {
			Record: domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning), Effort: "low"},
			Effort: "high",
		},
	} {
		t.Run(name, func(t *testing.T) {
			driver := newCompletionDriverStub(input.Record)
			input.Driver = driver
			input.StartAuthority = NewBackgroundJobStartBarrier()
			result := CompleteTask(context.Background(), input)
			if !result.IsError || len(driver.startRequests) != 0 || len(driver.progress) != 0 ||
				strings.Contains(fmt.Sprint(result.Output), sentinel) {
				t.Fatalf("invalid effort reached or was reflected by completion driver: result=%#v starts=%#v progress=%#v", result, driver.startRequests, driver.progress)
			}
		})
	}
}

func TestCaseChildCompletionNeverPersistsRawOutput(t *testing.T) {
	const sensitive = "bank account 6222020000000000000 and private child reasoning"
	record := domainjob.Record{
		ID: "job-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning),
		SecurityBinding: &domainjob.SecurityBinding{},
	}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "turn-child", "status": "completed"}
	driver.summary = sensitive

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request: TaskRequest{Prompt: "case child"}, Record: record, Driver: driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})
	if !result.IsError || result.Record.Status != string(domainjob.StatusFailed) || result.Record.Output != "" || driver.records[record.ID].Output != "" {
		t.Fatalf("security-bound child persisted a raw completion: %#v", result.Record)
	}
	if strings.Contains(fmt.Sprint(result.Output), sensitive) || result.Output["outputWithheld"] != true || result.Output["canReadOutput"] != false {
		t.Fatalf("security-bound completion escaped its metadata-only projection: %#v", result.Output)
	}
}

func TestCompleteTaskRevalidatesAuthorityBeforeChildProviderStart(t *testing.T) {
	record := domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:        TaskRequest{Prompt: "inspect this"},
		Record:         record,
		Driver:         driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
		AuthorizeStart: func(current domainjob.Record) (domainjob.Record, error) {
			return current, errors.New("task job authority rejected before execution: parent_security_context_mismatch")
		},
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusInterrupted) {
		t.Fatalf("stale child authority should interrupt before provider execution: %#v", result)
	}
	if len(driver.startRequests) != 0 {
		t.Fatalf("provider was started despite stale child authority: %#v", driver.startRequests)
	}
	if len(driver.progress) != 1 || driver.progress[0].status != string(domainjob.StatusInterrupted) {
		t.Fatalf("authority rejection progress mismatch: %#v", driver.progress)
	}
}

func TestCompleteTaskFailsClosedWithoutChildTurnStartAuthority(t *testing.T) {
	record := domainjob.Record{ID: "child-without-start-authority", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request: TaskRequest{Prompt: "must not start"}, Record: record, Driver: driver,
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusInterrupted) ||
		result.Output["failureCode"] != nil || result.Output["error"] != nil || result.Output["outputWithheld"] != true {
		t.Fatalf("missing child start authority did not fail closed: %#v", result)
	}
	if len(driver.startRequests) != 0 {
		t.Fatalf("child turn started without host authority: %#v", driver.startRequests)
	}
}

func TestCompleteTaskFailsWaitingChildTurn(t *testing.T) {
	record := domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "turn-child", "status": "waiting"}

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:        TaskRequest{Prompt: "inspect this"},
		Record:         record,
		Driver:         driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusFailed) || result.Record.ChildTurnID != "turn-child" {
		t.Fatalf("waiting child should fail with child turn id, got result=%#v record=%#v", result.Output, result.Record)
	}
	if result.Output["failureCode"] != nil || result.Output["error"] != nil || result.Output["outputWithheld"] != true {
		t.Fatalf("waiting error mismatch: %#v", result.Output)
	}
	if len(driver.progress) != 1 || driver.progress[0].status != string(domainjob.StatusFailed) {
		t.Fatalf("progress mismatch: %#v", driver.progress)
	}
}

func TestCompleteTaskFailsEmptyFinalResponse(t *testing.T) {
	record := domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "turn-child", "status": "completed"}
	driver.summary = "   "
	driver.toolInvocations = 2

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:        TaskRequest{Prompt: "inspect this"},
		Record:         record,
		Driver:         driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusFailed) || result.Record.ChildTurnID != "turn-child" {
		t.Fatalf("empty child final should fail with child turn id, got result=%#v record=%#v", result.Output, result.Record)
	}
	if result.Record.ToolInvocations != 2 {
		t.Fatalf("tool invocation count should still be persisted, got %#v", result.Record)
	}
	if result.Output["failureCode"] != nil || result.Output["error"] != nil || result.Output["outputWithheld"] != true {
		t.Fatalf("empty final error mismatch: %#v", result.Output)
	}
	if len(driver.progress) != 1 || driver.progress[0].status != string(domainjob.StatusFailed) {
		t.Fatalf("progress mismatch: %#v", driver.progress)
	}
}

func TestCompleteTaskMapsCanceledStartToKilled(t *testing.T) {
	record := domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)
	driver.startErr = context.Canceled

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:        TaskRequest{Prompt: "inspect this"},
		Record:         record,
		Driver:         driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusKilled) {
		t.Fatalf("canceled start should kill child run, got result=%#v record=%#v", result.Output, result.Record)
	}
	if len(driver.progress) != 1 || driver.progress[0].status != string(domainjob.StatusKilled) {
		t.Fatalf("progress mismatch: %#v", driver.progress)
	}
}

func TestCompleteTaskDoesNotReflectProviderFailureIntoProgressOrDurableRecord(t *testing.T) {
	const sentinel = "PRIVATE_PROVIDER_FAILURE account 6222020202020202020"
	record := domainjob.Record{ID: "child-private-error", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)
	driver.startErr = errors.New(sentinel)

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:        TaskRequest{Prompt: "inspect this"},
		Record:         record,
		Driver:         driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusFailed) || result.Record.Error != "" ||
		result.Record.FailureCode != domainjob.FailureChildExecutionFailed {
		t.Fatalf("provider failure was not reduced to a closed durable code: %#v", result.Record)
	}
	if len(driver.progress) != 1 || driver.progress[0].message != domainjob.FailureChildExecutionFailed {
		t.Fatalf("provider failure escaped through progress: %#v", driver.progress)
	}
	if strings.Contains(fmt.Sprint(result.Output), sentinel) || result.Output["error"] != nil ||
		result.Output["failureCode"] != nil || result.Output["outputWithheld"] != true {
		t.Fatalf("provider failure escaped through public output: %#v", result.Output)
	}
}

func TestCompleteTaskReturnsKilledRecordAfterChildTurn(t *testing.T) {
	record := domainjob.Record{ID: "child-1", ChildThreadID: "thread-child", Status: string(domainjob.StatusRunning)}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "turn-child", "status": "completed"}
	driver.records[record.ID] = domainjob.Record{
		ID:            record.ID,
		ChildThreadID: record.ChildThreadID,
		Status:        string(domainjob.StatusKilled),
		Output:        "partial",
		Error:         "stop requested",
	}

	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request:        TaskRequest{Prompt: "inspect this"},
		Record:         record,
		Driver:         driver,
		StartAuthority: NewBackgroundJobStartBarrier(),
	})

	if !result.IsError || result.Record.Status != string(domainjob.StatusKilled) ||
		result.Output["failureCode"] != nil || result.Output["error"] != nil || result.Output["outputWithheld"] != true {
		t.Fatalf("killed child should be surfaced, got result=%#v record=%#v", result.Output, result.Record)
	}
	if len(driver.progress) != 1 || driver.progress[0].status != string(domainjob.StatusKilled) {
		t.Fatalf("progress mismatch: %#v", driver.progress)
	}
}

type completionDriverStub struct {
	startResponse         map[string]any
	startErr              error
	loadErr               error
	completionErr         error
	usageErr              error
	cacheErr              error
	loadResult            *domainjob.Record
	updates               int
	startRequests         []controlapp.StartTurnRequest
	records               map[string]domainjob.Record
	summary               string
	toolInvocations       int
	usage                 map[string]any
	cacheDiagnostics      map[string]any
	usageSnapshots        int
	cacheDiagnosticsCalls int
	progress              []completionProgress
}

type completionProgress struct {
	record  domainjob.Record
	status  string
	message string
}

func newCompletionDriverStub(records ...domainjob.Record) *completionDriverStub {
	driver := &completionDriverStub{records: map[string]domainjob.Record{}}
	for _, record := range records {
		driver.records[record.ID] = record
	}
	return driver
}

func (d *completionDriverStub) StartTurn(_ context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
	d.startRequests = append(d.startRequests, request)
	if d.startErr != nil {
		return nil, d.startErr
	}
	if d.startResponse == nil {
		return map[string]any{"turnId": "turn-child", "status": "completed"}, nil
	}
	return d.startResponse, nil
}

func (d *completionDriverStub) UpdateChildRun(id string, request domainjob.UpdateRequest) (domainjob.Record, error) {
	d.updates++
	record, ok := d.records[id]
	if !ok {
		return domainjob.Record{}, errors.New("not found")
	}
	if request.Status != "" {
		record.Status = request.Status
	}
	if request.ChildThreadID != "" {
		record.ChildThreadID = request.ChildThreadID
	}
	if request.ChildTurnID != "" {
		record.ChildTurnID = request.ChildTurnID
	}
	if request.ChildCompletionReceipt != nil {
		record.ChildCompletionReceipt = domainjob.CloneChildCompletionReceiptV1(request.ChildCompletionReceipt)
	}
	if request.Output != "" {
		record.Output = request.Output
	}
	record.Error = ""
	if request.FailureCode != "" {
		record.FailureCode = request.FailureCode
	}
	if request.Usage != nil {
		record.Usage = request.Usage
	}
	if request.ToolInvocations != nil {
		record.ToolInvocations = *request.ToolInvocations
	}
	d.records[id] = record
	return record, nil
}

func (d *completionDriverStub) LoadChildRun(id string) (domainjob.Record, error) {
	if d.loadErr != nil {
		return domainjob.Record{}, d.loadErr
	}
	if d.loadResult != nil {
		return *d.loadResult, nil
	}
	record, ok := d.records[id]
	if !ok {
		return domainjob.Record{}, errors.New("not found")
	}
	return record, nil
}

func TestCompleteTaskReadFailurePreservesRunningRecord(t *testing.T) {
	for _, source := range []string{"job", "thread", "usage", "cache"} {
		t.Run(source, func(t *testing.T) {
			record := domainjob.Record{ID: "held-read-run", ChildThreadID: "held-read-thread", Status: string(domainjob.StatusRunning), IsolationMode: string(domainjob.IsolationWorktree)}
			driver := newCompletionDriverStub(record)
			cause := errors.New("synthetic original child read unavailable")
			wantUsageReads := 0
			wantCacheReads := 0
			switch source {
			case "job":
				driver.loadErr = cause
			case "thread":
				driver.completionErr = cause
			case "usage":
				driver.usageErr, wantUsageReads = cause, 1
			case "cache":
				driver.cacheErr, wantUsageReads, wantCacheReads = cause, 1, 1
			}
			driver.summary = "Synthetic result"
			result := CompleteTask(context.Background(), CompleteTaskInput{Record: record, Driver: driver, StartAuthority: NewBackgroundJobStartBarrier()})
			if !result.IsError || !errors.Is(result.Err, cause) || driver.updates != 0 || len(driver.progress) != 0 || driver.usageSnapshots != wantUsageReads || driver.cacheDiagnosticsCalls != wantCacheReads || !reflect.DeepEqual(record, driver.records[record.ID]) {
				t.Fatalf("unavailable child observation became a terminal settlement: error=%v updates=%d progress=%d usage=%d cache=%d", result.IsError, driver.updates, len(driver.progress), driver.usageSnapshots, driver.cacheDiagnosticsCalls)
			}
		})
	}
}

func TestCompleteTaskForegroundAuthorityFailurePreservesRunningRecord(t *testing.T) {
	record := domainjob.Record{ID: "held-foreground", ChildThreadID: "held-foreground-thread", Status: string(domainjob.StatusRunning), ToolScope: []string{toolcatalogForegroundSubmitToolName}}
	driver := newCompletionDriverStub(record)
	driver.summary = "Synthetic foreground result"
	driver.startResponse = map[string]any{"turnId": "child-turn", "status": "completed", "usage": map[string]any{}, "cacheDiagnostics": map[string]any{}}
	cause := errors.New("synthetic current foreground authority unavailable")
	authority := &foregroundCleanupIssuerStub{prepareErr: cause}
	result := CompleteTask(context.Background(), CompleteTaskInput{Record: record, Driver: driver, StartAuthority: NewBackgroundJobStartBarrier(), ForegroundAuthority: authority})
	if !result.IsError || !errors.Is(result.Err, cause) || driver.updates != 0 || len(driver.progress) != 0 || !reflect.DeepEqual(record, driver.records[record.ID]) || authority.prepareCalls != 1 || len(authority.deleted) != 1 {
		t.Fatalf("unavailable foreground authority settled job or retained capability: error=%v updates=%d progress=%d prepares=%d deletes=%d", result.IsError, driver.updates, len(driver.progress), authority.prepareCalls, len(authority.deleted))
	}
}

func TestCompleteTaskRejectsReloadedForeignJobBinding(t *testing.T) {
	for _, field := range []string{"job", "child", "parent"} {
		t.Run(field, func(t *testing.T) {
			record := domainjob.Record{ID: "child-run", ChildThreadID: "child-thread", ParentThreadID: "parent-thread", Status: string(domainjob.StatusRunning)}
			foreign := record
			switch field {
			case "job":
				foreign.ID = "foreign-run"
			case "child":
				foreign.ChildThreadID = "foreign-child"
			case "parent":
				foreign.ParentThreadID = "foreign-parent"
			}
			driver := newCompletionDriverStub(record)
			driver.loadResult = &foreign
			driver.summary = "Synthetic result"
			result := CompleteTask(context.Background(), CompleteTaskInput{Record: record, Driver: driver, StartAuthority: NewBackgroundJobStartBarrier()})
			if !result.IsError || result.Err == nil || driver.updates != 0 || len(driver.progress) != 0 || driver.usageSnapshots != 0 || !reflect.DeepEqual(record, result.Record) {
				t.Fatal("reloaded foreign job binding entered completion effects")
			}
		})
	}
}

func TestTurnCompletionReadPreservesCauseAndRequiresPrimary(t *testing.T) {
	for _, source := range []string{"read_error", "missing", "foreign", "missing_turn", "blank_turn", "duplicate_turn", "malformed_turn", "malformed_items", "foreign_turn_thread"} {
		t.Run(source, func(t *testing.T) {
			cause := errors.New("synthetic child primary I/O failure")
			reads := 0
			load := func(string) (map[string]any, error) {
				reads++
				switch source {
				case "read_error":
					return nil, cause
				case "foreign":
					return map[string]any{"id": "foreign-thread"}, nil
				case "missing_turn", "blank_turn":
					return map[string]any{"id": "child-thread", "turns": []any{map[string]any{"id": "another-turn"}}}, nil
				case "duplicate_turn":
					return map[string]any{"id": "child-thread", "turns": []any{map[string]any{"id": "child-turn"}, map[string]any{"id": "child-turn"}}}, nil
				case "malformed_turn":
					return map[string]any{"id": "child-thread", "turns": []any{"not-a-turn"}}, nil
				case "malformed_items":
					return map[string]any{"id": "child-thread", "turns": []any{map[string]any{"id": "child-turn", "items": "not-items"}}}, nil
				case "foreign_turn_thread":
					return map[string]any{"id": "child-thread", "turns": []any{map[string]any{"id": "child-turn", "threadId": "foreign-thread"}}}, nil
				default:
					return nil, nil
				}
			}
			turnID, wantReads := "child-turn", 1
			if source == "blank_turn" {
				turnID, wantReads = "", 0
			}
			summary, count, err := TurnCompletionFromThreadStoreV1(load, "child-thread", turnID)
			if err == nil || summary != "" || count != 0 || reads != wantReads || (source == "read_error" && !errors.Is(err, cause)) {
				t.Fatalf("unavailable primary became completion data or lost cause: reads=%d err=%v", reads, err)
			}
		})
	}
}

func TestTurnCompletionReadUsesOneExactTurnSnapshot(t *testing.T) {
	for _, final := range []string{"", "Synthetic completed response"} {
		reads := 0
		load := func(string) (map[string]any, error) {
			reads++
			return map[string]any{"id": "child-thread", "turns": []any{
				map[string]any{"id": "older-turn", "items": []any{map[string]any{"kind": "tool_call", "callId": "older-call"}}},
				map[string]any{"id": "child-turn", "threadId": "child-thread", "items": []any{
					map[string]any{"kind": "tool_call", "callId": "child-call"},
					map[string]any{"kind": "assistant_text", "text": final},
				}},
			}}, nil
		}
		summary, count, err := TurnCompletionFromThreadStoreV1(load, "child-thread", "child-turn")
		if err != nil || summary != final || count != 1 || reads != 1 {
			t.Fatalf("completion summary/count did not share the exact successful read: summary=%q count=%d reads=%d err=%v", summary, count, reads, err)
		}
	}
}

func (d *completionDriverStub) TurnCompletion(string, string) (string, int, error) {
	return d.summary, d.toolInvocations, d.completionErr
}

func (d *completionDriverStub) UsageSnapshot(string) (map[string]any, error) {
	d.usageSnapshots++
	return d.usage, d.usageErr
}

func (d *completionDriverStub) CacheDiagnostics(string, string) (map[string]any, error) {
	d.cacheDiagnosticsCalls++
	return d.cacheDiagnostics, d.cacheErr
}

func (d *completionDriverStub) RecordProgress(record domainjob.Record, status string, message string) {
	d.progress = append(d.progress, completionProgress{record: record, status: status, message: message})
}

func TestCompleteTaskClaimsInsidePendingSteerStartBarrier(t *testing.T) {
	record := domainjob.Record{ID: "child-pending", ChildThreadID: "thread-child", Status: "running"}
	driver := newCompletionDriverStub(record)
	driver.startResponse = map[string]any{"turnId": "turn-child", "status": "completed"}
	driver.summary = "synthetic result"
	barrier := NewBackgroundJobStartBarrier()
	claimedInside := false
	result := CompleteTask(context.Background(), CompleteTaskInput{Record: record, Driver: driver, StartAuthority: barrier,
		AuthorizeStart: func(current domainjob.Record) (domainjob.Record, error) {
			if barrier.mu.TryLock() {
				barrier.mu.Unlock()
				return current, errors.New("claim ran outside pending steer barrier")
			}
			claimedInside = barrier.consumed
			return current, nil
		},
	})
	if result.IsError || !claimedInside || len(driver.startRequests) != 1 {
		t.Fatal("first-turn claim was not protected by the pending steer start boundary")
	}
}

func TestPendingSteerReservationSerializesClaimAndCancellation(t *testing.T) {
	for _, cancelBeforeRelease := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelBeforeRelease), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			barrier := NewBackgroundJobStartBarrier()
			release, err := barrier.reservePendingSteer(ctx)
			if err != nil {
				t.Fatal(err)
			}
			record := domainjob.Record{ID: "child-serialized", ChildThreadID: "thread-serialized", Status: "running"}
			driver := newCompletionDriverStub(record)
			driver.startResponse = map[string]any{"turnId": "turn-child", "status": "completed"}
			driver.summary = "synthetic result"
			var claims atomic.Int32
			done := make(chan CompleteTaskResult, 1)
			go func() {
				done <- CompleteTask(ctx, CompleteTaskInput{Record: record, Driver: driver, StartAuthority: barrier, AuthorizeStart: func(current domainjob.Record) (domainjob.Record, error) { claims.Add(1); return current, nil }})
			}()
			select {
			case <-done:
				t.Fatal("child escaped held pending queue boundary")
			case <-time.After(20 * time.Millisecond):
			}
			if claims.Load() != 0 {
				t.Fatal("claim crossed held pending queue boundary")
			}
			if cancelBeforeRelease {
				barrier.Cancel()
				cancel()
			}
			release()
			release()
			select {
			case result := <-done:
				if cancelBeforeRelease {
					if !result.IsError || claims.Load() != 0 || len(driver.startRequests) != 0 {
						t.Fatal("cancelled pending child claimed or started")
					}
				} else if result.IsError || claims.Load() != 1 || len(driver.startRequests) != 1 {
					t.Fatal("released pending child did not claim and start once")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("pending/start boundary did not release")
			}
			if nextRelease, err := barrier.reservePendingSteer(context.Background()); err == nil {
				nextRelease()
				t.Fatal("consumed or cancelled start admitted a later pending queue")
			}
		})
	}
}
