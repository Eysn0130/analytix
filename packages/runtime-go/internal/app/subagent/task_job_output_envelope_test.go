package subagent

import (
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestTaskJobOutputResponseV1OnlyAcceptsClosedWithheldVariant(t *testing.T) {
	withheld := TaskJobOutputWithheldResponseV1(domainjob.Record{ID: "job-2", Status: "completed"})
	if err := ValidateTaskJobOutputResponseV1(withheld); err != nil {
		t.Fatal(err)
	}
	available := map[string]any{
		"schemaVersion": 1, "availability": "available", "jobId": "job-1", "status": "completed",
		"output": "private job output", "offset": 0, "nextOffset": 18, "outputBytes": 18, "truncated": false,
	}
	if err := ValidateTaskJobOutputResponseV1(available); err == nil {
		t.Fatal("fact-bearing task-job output variant was accepted")
	}

	for name, mutate := range map[string]func(map[string]any){
		"output":             func(value map[string]any) { value["output"] = "private child final" },
		"unknown":            func(value map[string]any) { value["unknownAuthority"] = true },
		"unsafe flag":        func(value map[string]any) { value["factAnswerAllowed"] = true },
		"readable flag":      func(value map[string]any) { value["canReadOutput"] = true },
		"unknown variant":    func(value map[string]any) { value["availability"] = "model_claimed_safe" },
		"unprojected status": func(value map[string]any) { value["status"] = "completed-private" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cloneMap(withheld)
			mutate(candidate)
			if err := ValidateTaskJobOutputResponseV1(candidate); err == nil {
				t.Fatalf("invalid task-job output response was accepted: %#v", candidate)
			}
		})
	}
}

func TestTaskJobOutputWithheldResponseDoesNotReflectPrivateRecordFields(t *testing.T) {
	const sentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	response := TaskJobOutputWithheldResponseV1(domainjob.Record{
		ID: "job-1", Status: "completed-" + sentinel, Output: sentinel, Error: sentinel,
	})
	if response["status"] != "unknown" || strings.Contains(firstNonEmptyAnyString(response["status"]), sentinel) {
		t.Fatalf("withheld response reflected private status: %#v", response)
	}
	if err := ValidateTaskJobOutputResponseV1(response); err != nil {
		t.Fatalf("fixed withheld projection should remain schema-valid: %v", err)
	}
	for _, forbidden := range []string{"output", "offset", "nextOffset", "outputBytes", "truncated", "error"} {
		if _, ok := response[forbidden]; ok {
			t.Fatalf("withheld response exposed %q: %#v", forbidden, response)
		}
	}
	malicious := cloneMap(response)
	malicious["status"] = "completed-" + sentinel
	if err := ValidateTaskJobOutputResponseV1(malicious); err == nil {
		t.Fatal("validator accepted an open response status")
	}
}
