package subagent

import (
	"encoding/json"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func ValidateRestartTaskJobRecord(record domainjob.Record, parentThreadID, jobID string) (TaskJobServiceResult, bool) {
	if !TaskJobRecordAllowed(record, parentThreadID) {
		return TaskJobServiceResult{Response: TaskJobNotFoundResponse(jobID), IsError: true, ErrorCode: TaskJobErrorNotFound}, false
	}
	if !JobIsSubagent(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("summary task is not a subagent job: " + jobID), IsError: true, ErrorCode: TaskJobErrorValidation}, false
	}
	if !TaskJobTerminal(record) {
		return TaskJobServiceResult{Response: ValidationErrorResponse("subagent job is still active: " + jobID), IsError: true, ErrorCode: TaskJobErrorValidation}, false
	}
	if strings.TrimSpace(record.Prompt) == "" {
		return TaskJobServiceResult{Response: ValidationErrorResponse("subagent job can not be restarted without a persisted prompt: " + jobID), IsError: true, ErrorCode: TaskJobErrorValidation}, false
	}
	if err := domainmodel.ValidateReasoningEffortV1(record.Effort); err != nil {
		return TaskJobServiceResult{Response: ValidationErrorResponse("subagent job reasoning effort is invalid"), IsError: true, ErrorCode: TaskJobErrorValidation}, false
	}
	return TaskJobServiceResult{}, true
}

func RestartTaskJobArguments(record domainjob.Record) []byte {
	args, _ := json.Marshal(map[string]any{
		"prompt":               record.Prompt,
		"run_in_background":    true,
		"toolPolicy":           firstNonEmptyAnyString(record.ToolPolicy, "readOnly"),
		"auto_continue_parent": record.AutoContinueParent,
	})
	return args
}

func RestartTaskJobRequest(record domainjob.Record, workspace string) TaskRequest {
	effort, _ := domainmodel.ProjectReasoningEffortV1(record.Effort)
	request := TaskRequest{
		Name:               record.Name,
		Prompt:             record.Prompt,
		Label:              firstNonEmptyAnyString(record.Label, "Restart: "+record.ID),
		LabelExplicit:      true,
		Workspace:          firstNonEmptyAnyString(record.Workspace, workspace),
		ProviderID:         record.ProviderID,
		Model:              record.Model,
		EndpointFormat:     "",
		Variant:            record.Variant,
		Effort:             effort,
		ToolPolicy:         firstNonEmptyAnyString(record.ToolPolicy, "readOnly"),
		ToolPolicySet:      true,
		Tools:              append([]string(nil), record.ToolScope...),
		RunInBackground:    true,
		AutoContinueParent: record.AutoContinueParent,
	}
	if record.MaxModelSteps != nil {
		request.MaxSteps = *record.MaxModelSteps
		request.MaxStepsSet = true
	}
	return request
}
