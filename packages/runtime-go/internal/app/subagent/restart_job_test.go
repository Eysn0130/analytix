package subagent

import (
	"encoding/json"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestRestartTaskJobRejectsInvalidReasoningEffortWithoutReflection(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: "thread-1", ChildThreadID: "child-thread-1",
		Kind: "subagent", Status: string(domainjob.StatusCompleted), Prompt: "continue", Effort: sentinel,
	}
	result, ok := ValidateRestartTaskJobRecord(record, "thread-1", "job-1")
	encoded, err := json.Marshal(result.Response)
	if ok || !result.IsError || err != nil || strings.Contains(string(encoded), sentinel) {
		t.Fatalf("invalid persisted effort was restartable or reflected: ok=%v result=%#v body=%s err=%v", ok, result, encoded, err)
	}
	request := RestartTaskJobRequest(record, "/workspace")
	if request.Effort != "" {
		t.Fatalf("ungated restart request reflected invalid effort: %#v", request)
	}
}

func TestRestartTaskJobPreservesClosedAutoEffort(t *testing.T) {
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: "thread-1", ChildThreadID: "child-thread-1",
		Kind: "subagent", Status: string(domainjob.StatusCompleted), Prompt: "continue", Effort: "auto",
	}
	if result, ok := ValidateRestartTaskJobRecord(record, "thread-1", "job-1"); !ok || result.IsError {
		t.Fatalf("valid auto effort was rejected: %#v", result)
	}
	if request := RestartTaskJobRequest(record, "/workspace"); request.Effort != "auto" {
		t.Fatalf("valid auto effort was not preserved: %#v", request)
	}
}

func TestRestartTaskJobDoesNotReusePersistedRegistryRoute(t *testing.T) {
	record := domainjob.Record{
		ID: "job-1", ParentThreadID: "thread-1", ChildThreadID: "child-thread-1",
		Kind: "subagent", Status: string(domainjob.StatusCompleted), Prompt: "continue", Effort: "medium",
		ProviderID: "provider-intent", Model: "model-intent", EndpointFormat: "messages",
	}
	request := RestartTaskJobRequest(record, "/workspace")
	if request.ProviderID != "provider-intent" || request.Model != "model-intent" || request.EndpointFormat != "" {
		t.Fatalf("restart did not preserve only key-free provider/model intent: %#v", request)
	}
}
