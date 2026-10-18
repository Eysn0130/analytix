package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestTaskJobCaptureUsesReturnedProducerFactsAndWithholdsPublicBytesV1(t *testing.T) {
	record := domainjob.Record{ID: "job-a", Kind: "subagent", Status: "running", Name: "ACTUAL_NAME", Label: "ACTUAL_LABEL", Output: "PRIVATE_OUTPUT"}
	result := TaskJobServiceResult{Record: record, Response: TaskJobOutputWithheldResponseV1(record)}
	capture, ok := TaskJobResultCaptureV1("output", result)
	if !ok || capture.Job == nil || len(capture.Job.Jobs) != 1 {
		t.Fatal("authorized record was not captured")
	}
	job := capture.Job.Jobs[0]
	if job.Name == nil || *job.Name != record.Name || job.Label == nil || *job.Label != record.Label || job.Output.Body == nil || *job.Output.Body != record.Output || job.Status != "running" {
		t.Fatal("capture replaced producer facts")
	}
	public, _ := json.Marshal(result.Response)
	for _, secret := range []string{record.Name, record.Label, record.Output} {
		if strings.Contains(string(public), secret) {
			t.Fatal("private job text entered public response")
		}
	}
	result.Record.Name, result.Record.Label, result.Record.Output, result.Record.Status = "new name", "new label", "new output", "completed"
	if *capture.Job.Jobs[0].Label != "ACTUAL_LABEL" || *capture.Job.Jobs[0].Output.Body != "PRIVATE_OUTPUT" || capture.Job.Jobs[0].Status != "running" {
		t.Fatal("later job mutation replaced historical capture")
	}
	unknown, ok := TaskJobResultCaptureV1("output", TaskJobServiceResult{Record: domainjob.Record{ID: "old", Kind: "subagent", Status: "completed"}})
	if !ok || unknown.Job.Jobs[0].Name != nil || unknown.Job.Jobs[0].Label != nil || unknown.Job.Jobs[0].Output.Availability != "unavailable" {
		t.Fatal("legacy missing facts were fabricated")
	}
}

func TestTaskJobWaitTimeoutCaptureKeepsJobRunningV1(t *testing.T) {
	repo := newTaskJobRepoStub([]domainjob.Record{{ID: "job-live", Kind: "subagent", ParentThreadID: "parent", Status: "running"}})
	service := NewService(Dependencies{Jobs: repo, State: NewRuntimeState()})
	result := service.WaitTaskJobs("parent", TaskJobWaitRequest{JobIDs: []string{"job-live"}, TimeoutMS: 1}, true)
	if result.IsError || result.Receipt == nil || result.Receipt.Operation != "wait" || result.Receipt.Status != "timeout" {
		t.Fatal("wait lost its independent timeout receipt")
	}
	capture, ok := TaskJobResultCaptureV1("wait", result)
	if !ok || capture.Job.Jobs[0].Status != "running" {
		t.Fatal("wait timeout became job terminal status")
	}
	current, _ := repo.LoadChildRun("job-live")
	if current.Status != "running" {
		t.Fatal("wait timeout changed job state")
	}
}

func TestTaskJobWaitTerminalRecordWithUnsettledWorkerReturnsTimeoutV1(t *testing.T) {
	repo := newTaskJobRepoStub([]domainjob.Record{{ID: "job-tail", Kind: "subagent", ParentThreadID: "parent", Status: "completed"}})
	state := NewRuntimeState()
	state.RegisterBackgroundJob("job-tail", func() {})
	defer state.UnregisterBackgroundJob("job-tail")
	service := NewService(Dependencies{Jobs: repo, State: state})
	result := service.WaitTaskJobs("parent", TaskJobWaitRequest{JobIDs: []string{"job-tail"}, TimeoutMS: 10}, true)
	if result.IsError || result.Receipt == nil || result.Receipt.Status != "timeout" || len(result.Records) != 1 || result.Records[0].Status != "completed" {
		t.Fatal("terminal durable status hid the actual worker-settlement timeout")
	}
	capture, ok := TaskJobResultCaptureV1("wait", result)
	if !ok || capture.Job.Receipt.Status != "timeout" || capture.Job.Jobs[0].Status != "completed" {
		t.Fatal("wait timeout replaced the independent terminal Job status")
	}
	state.UnregisterBackgroundJob("job-tail")
	settled := service.WaitTaskJobs("parent", TaskJobWaitRequest{JobIDs: []string{"job-tail"}, TimeoutMS: 10}, true)
	if settled.Receipt == nil || settled.Receipt.Status != "completed" {
		t.Fatal("acknowledged worker settlement did not complete the next wait")
	}
}

func TestTaskJobCancelReceiptDistinguishesRequestFromTerminalV1(t *testing.T) {
	repo := newTaskJobRepoStub([]domainjob.Record{{ID: "job-live", Kind: "subagent", ParentThreadID: "parent", Status: "running"}, {ID: "job-done", Kind: "subagent", ParentThreadID: "parent", Status: "completed"}})
	state := NewRuntimeState()
	jobContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	state.RegisterBackgroundJob("job-live", cancel)
	defer state.UnregisterBackgroundJob("job-live")
	service := NewService(Dependencies{Jobs: repo, State: state})
	requested := service.killTaskJobWithWait("parent", TaskJobKillRequest{JobID: "job-live"}, time.Millisecond)
	if jobContext.Err() == nil || !requested.IsError || requested.Record.Status != "running" || requested.Receipt == nil || requested.Receipt.Status != "cancellation-requested" {
		t.Fatal("cancel request falsely became a killed job")
	}
	capture, ok := TaskJobResultCaptureV1("cancel", requested)
	if !ok || capture.Status != "failed" || capture.Job.Jobs[0].Status != "running" || capture.Job.Receipt.Status != "cancellation-requested" {
		t.Fatal("unacknowledged cancellation lost actual operation facts")
	}
	already := service.KillTaskJob("parent", TaskJobKillRequest{JobID: "job-done"})
	if already.IsError || already.Record.Status != "completed" || already.Receipt == nil || already.Receipt.Status != "already-finished" {
		t.Fatal("finished job received a fabricated cancellation request")
	}
}

func TestTaskJobCaptureCollectorOwnsReceiptAndOutputV1(t *testing.T) {
	capture, ok := TaskJobResultCaptureV1("output", TaskJobServiceResult{Record: domainjob.Record{ID: "job-a", Kind: "subagent", Status: "running", Label: "actual", Output: "retained"}})
	if !ok {
		t.Fatal("capture unavailable")
	}
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	domaintoolresult.CaptureProtectedToolResultV1(ctx, capture)
	*capture.Job.Jobs[0].Label, *capture.Job.Jobs[0].Output.Body = "later", "changed"
	retained, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !ok || *retained.Job.Jobs[0].Label != "actual" || *retained.Job.Jobs[0].Output.Body != "retained" {
		t.Fatal("collector retained mutable producer pointers")
	}
}

func TestTaskJobCaptureHonestExistingBodyAndNewListDisplayLimitsV1(t *testing.T) {
	output := strings.Repeat("界", domaintoolresult.ProtectedSnapshotBodyLimitV1)
	capture, ok := TaskJobResultCaptureV1("output", TaskJobServiceResult{Record: domainjob.Record{ID: "job-a", Kind: "subagent", Status: "completed", Output: output}})
	if !ok || capture.Job.Jobs[0].Output.Body == nil || capture.Job.Jobs[0].Output.Truncated == nil || !*capture.Job.Jobs[0].Output.Truncated || len(*capture.Job.Jobs[0].Output.Body) > domaintoolresult.ProtectedSnapshotBodyLimitV1 || !strings.HasPrefix(output, *capture.Job.Jobs[0].Output.Body) {
		t.Fatal("bounded Job body hid truncation or changed retained bytes")
	}
	records := make([]domainjob.Record, domaintoolresult.ProtectedJobListLimitV1+1)
	for i := range records {
		records[i] = domainjob.Record{ID: fmt.Sprintf("job-%d", i), Kind: "subagent", Status: "running"}
	}
	list, ok := TaskJobResultCaptureV1("list", TaskJobServiceResult{Records: records})
	if !ok || len(list.Job.Jobs) != domaintoolresult.ProtectedJobListLimitV1 || list.Job.Total != len(records) || !list.Job.Truncated {
		t.Fatal("omitted Job records were represented as a complete list")
	}
}

func TestTaskJobCaptureKeepsProtectedEmptyDistinctFromMissingAndIgnoresBoundRecordTextV1(t *testing.T) {
	empty, truncated := "", false
	record := domainjob.Record{ID: "job-a", Kind: "subagent", Status: "completed", SecurityBinding: &domainjob.SecurityBinding{}, Output: "untrusted record bytes"}
	missing, ok := TaskJobResultCaptureV1("output", TaskJobServiceResult{Record: record})
	if !ok || missing.Job.Jobs[0].Output.Availability != "unavailable" {
		t.Fatal("bound ordinary Record.Output became a protected source")
	}
	result := TaskJobServiceResult{Record: record, OutputCapture: &domaintoolresult.ProtectedJobOutputV1{Availability: "available", Body: &empty, Truncated: &truncated}}
	capture, ok := TaskJobResultCaptureV1("output", result)
	if !ok || capture.Job.Jobs[0].Output.Availability != "available" || capture.Job.Jobs[0].Output.Body == nil || *capture.Job.Jobs[0].Output.Body != "" {
		t.Fatal("actual empty producer body became missing")
	}
	empty, truncated = "changed", true
	if *capture.Job.Jobs[0].Output.Body != "" || *capture.Job.Jobs[0].Output.Truncated {
		t.Fatal("capture retained mutable observation pointers")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "OutputCapture") || strings.Contains(string(encoded), "availability") {
		t.Fatal("protected process-local observation is JSON serializable")
	}
}
