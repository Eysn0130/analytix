package subagent

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func RunOutput(record domainjob.Record, summary string, usage map[string]any, err error) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		if err != nil && strings.TrimSpace(record.Status) == "" {
			record.Status = string(domainjob.StatusFailed)
		}
		return SecurityBoundChildOutputProjection(record)
	}
	status := strings.TrimSpace(record.Status)
	if err != nil && status == "" {
		status = string(domainjob.StatusFailed)
	}
	status = domainjob.PublicStatusV1(status)
	out := map[string]any{
		"kind":                         "subagent_task",
		"childId":                      record.ID,
		"childRunId":                   record.ID,
		"jobId":                        record.ID,
		"name":                         record.Name,
		"childThreadId":                record.ChildThreadID,
		"childTurnId":                  record.ChildTurnID,
		"status":                       status,
		"label":                        record.Label,
		"summary":                      summary,
		"model":                        record.Model,
		"providerId":                   record.ProviderID,
		"endpointFormat":               record.EndpointFormat,
		"variant":                      record.Variant,
		"modelSource":                  record.ModelSource,
		"profile":                      record.ProfileName,
		"profileMode":                  record.ProfileMode,
		"profileDescription":           record.ProfileDescription,
		"profileColor":                 record.ProfileColor,
		"profileIcon":                  record.ProfileIcon,
		"toolPolicy":                   record.ToolPolicy,
		"toolScope":                    stringListAny(record.ToolScope),
		"returnFormat":                 firstNonEmptyAnyString(record.ReturnFormat, "summary"),
		"continueFrom":                 record.ContinueFrom,
		"forkFrom":                     record.ForkFrom,
		"sourceRef":                    record.SourceRef,
		"background":                   record.Background,
		"artifactPath":                 record.ArtifactPath,
		"prefixReused":                 true,
		"inheritedHistoryItems":        float64(0),
		"childSeq":                     float64(record.ChildSeq),
		"queuedMs":                     float64(record.QueuedMs),
		"durationMs":                   DurationMs(record),
		"toolInvocations":              float64(record.ToolInvocations),
		"topLevelSubagentRouteExposed": false,
	}
	if effort, valid := domainmodel.ProjectReasoningEffortV1(record.Effort); valid && effort != "" {
		out["effort"] = effort
	}
	if record.TokenBudget > 0 {
		out["tokenBudget"] = float64(record.TokenBudget)
	}
	if record.TimeBudgetMs > 0 {
		out["timeBudgetMs"] = float64(record.TimeBudgetMs)
	}
	AddSteerStateMetadata(out, record)
	AddPauseStateMetadata(out, record)
	AddJobHealthMetadata(out, record)
	AddIsolationMetadata(out, record)
	AddChildTodoMetadata(out, record)
	AddParallelMetadata(out, record)
	if record.SecurityBinding != nil {
		out["outputTrustStatus"] = record.SecurityBinding.OutputTrustStatus
		out["evidence"] = []any{}
		out["evidenceBundleStatus"] = "unsupported"
		out["evidenceCount"] = float64(0)
	} else {
		AddEvidenceMetadata(out, summary, record.ReturnFormat)
	}
	if usage != nil {
		projectedUsage := projectPublicUsageNumbersV1(usage)
		if len(projectedUsage) > 0 {
			out["usage"] = projectedUsage
		}
		if record.TokenBudget > 0 {
			totalTokens := int(floatFromAny(projectedUsage["totalTokens"]))
			out["budgetExceeded"] = totalTokens > record.TokenBudget
		}
	}
	if projected := domainjob.ProjectPersistedModelExecutionV1(record.ModelExecution); len(projected) > 0 {
		out["modelExecution"] = projected
	}
	if failureCode := domainjob.NormalizeFailureCode(record.FailureCode, status, err != nil); failureCode != "" {
		out["failureCode"] = failureCode
	}
	return ProjectOrdinaryJobMap(out)
}

func EventChild(record domainjob.Record, status string, _ string) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		record.Status = firstNonEmptyAnyString(status, record.Status)
		return SecurityBoundChildOutputProjection(record)
	}
	status = domainjob.PublicStatusV1(status)
	child := map[string]any{
		"parentGoalId":                 record.ParentGoalID,
		"parentGoalObjective":          record.ParentGoalObjective,
		"parentThreadId":               record.ParentThreadID,
		"parentTurnId":                 record.ParentTurnID,
		"parentToolItemId":             record.ParentToolItemID,
		"parentToolCallId":             record.ParentToolCallID,
		"childName":                    record.Name,
		"childId":                      record.ID,
		"childRunId":                   record.ID,
		"jobId":                        record.ID,
		"childThreadId":                record.ChildThreadID,
		"childTurnId":                  record.ChildTurnID,
		"childLabel":                   record.Label,
		"childStatus":                  status,
		"childModel":                   record.Model,
		"childProviderId":              record.ProviderID,
		"childEndpointFormat":          record.EndpointFormat,
		"childModelVariant":            record.Variant,
		"childModelSource":             record.ModelSource,
		"childProfile":                 record.ProfileName,
		"childProfileMode":             record.ProfileMode,
		"childProfileDescription":      record.ProfileDescription,
		"childProfileColor":            record.ProfileColor,
		"childProfileIcon":             record.ProfileIcon,
		"childToolPolicy":              record.ToolPolicy,
		"toolScope":                    stringListAny(record.ToolScope),
		"lineageKey":                   record.LineageKey,
		"model":                        record.Model,
		"providerId":                   record.ProviderID,
		"endpointFormat":               record.EndpointFormat,
		"variant":                      record.Variant,
		"modelSource":                  record.ModelSource,
		"profileSource":                record.ProfileSource,
		"defaultModelInherited":        record.DefaultModelInherited,
		"continueFrom":                 record.ContinueFrom,
		"forkFrom":                     record.ForkFrom,
		"sourceRef":                    record.SourceRef,
		"background":                   record.Background,
		"artifactPath":                 record.ArtifactPath,
		"prefixReused":                 true,
		"inheritedHistoryItems":        float64(0),
		"childSeq":                     float64(record.ChildSeq),
		"queuedMs":                     float64(record.QueuedMs),
		"durationMs":                   DurationMs(record),
		"toolInvocations":              float64(record.ToolInvocations),
		"returnFormat":                 firstNonEmptyAnyString(record.ReturnFormat, "summary"),
		"contract":                     "analytix-subagent-child-run",
		"topLevelSubagentRouteExposed": false,
	}
	if effort, valid := domainmodel.ProjectReasoningEffortV1(record.Effort); valid && effort != "" {
		child["effort"] = effort
	}
	if record.TokenBudget > 0 {
		child["tokenBudget"] = float64(record.TokenBudget)
	}
	if record.TimeBudgetMs > 0 {
		child["timeBudgetMs"] = float64(record.TimeBudgetMs)
	}
	AddSteerStateMetadata(child, record)
	AddPauseStateMetadata(child, record)
	AddJobHealthMetadata(child, record)
	AddIsolationMetadata(child, record)
	AddChildTodoMetadata(child, record)
	if record.SecurityBinding == nil && (strings.TrimSpace(record.Output) != "" || taskJobTerminalStatus(status)) {
		AddEvidenceSummaryMetadata(child, record.Output, record.ReturnFormat)
	} else if record.SecurityBinding != nil {
		child["outputTrustStatus"] = record.SecurityBinding.OutputTrustStatus
		child["evidenceBundleStatus"] = "unsupported"
		child["evidenceCount"] = float64(0)
	}
	if record.MaxModelSteps != nil {
		child["maxModelSteps"] = float64(*record.MaxModelSteps)
	}
	if projected := domainjob.ProjectPersistedModelExecutionV1(record.ModelExecution); len(projected) > 0 {
		child["childModelExecution"] = projected
		child["modelExecution"] = projected
	}
	AddParallelMetadata(child, record)
	if record.Usage != nil {
		AddUsageMetadata(child, record.Usage)
	}
	if failureCode := domainjob.NormalizeFailureCode(record.FailureCode, status, false); failureCode != "" {
		child["failureCode"] = failureCode
	}
	return ProjectOrdinaryJobMap(child)
}

func TaskJobProgressChild(record domainjob.Record, status string) map[string]any {
	if ChildOutputRequiresWithholding(record) {
		record.Status = firstNonEmptyAnyString(status, record.Status)
		return SecurityBoundChildOutputProjection(record)
	}
	status = domainjob.PublicStatusV1(status)
	child := map[string]any{
		"parentGoalId":        record.ParentGoalID,
		"parentGoalObjective": record.ParentGoalObjective,
		"parentThreadId":      record.ParentThreadID,
		"parentTurnId":        record.ParentTurnID,
		"parentToolItemId":    record.ParentToolItemID,
		"parentToolCallId":    record.ParentToolCallID,
		"childId":             record.ID,
		"childRunId":          record.ID,
		"jobId":               record.ID,
		"childLabel":          record.Label,
		"childStatus":         status,
		"childToolPolicy":     record.ToolPolicy,
		"lineageKey":          record.LineageKey,
		"background":          record.Background,
		"artifactPath":        record.ArtifactPath,
		"kind":                record.Kind,
		"name":                record.Name,
		"workspace":           record.Workspace,
		"outputBytes":         float64(len([]byte(record.Output))),
	}
	if record.Usage != nil {
		for _, key := range []string{"exitCode", "durationMs", "timedOut", "maxOutputBytes", "truncated"} {
			if value, ok := record.Usage[key]; ok {
				child[key] = value
			}
		}
	}
	AddSteerStateMetadata(child, record)
	AddPauseStateMetadata(child, record)
	AddJobHealthMetadata(child, record)
	AddIsolationMetadata(child, record)
	AddChildTodoMetadata(child, record)
	if failureCode := domainjob.NormalizeFailureCode(record.FailureCode, status, false); failureCode != "" {
		child["failureCode"] = failureCode
	}
	return ProjectOrdinaryJobMap(child)
}

func AddJobHealthMetadata(out map[string]any, record domainjob.Record) {
	if out == nil {
		return
	}
	diagnostics := TaskJobDiagnostics(record, time.Now().UTC(), DefaultTaskJobStalledAfter)
	for _, key := range []string{
		"heartbeatStatus",
		"lastHeartbeatAt",
		"heartbeatAt",
		"heartbeatAgeMs",
		"leaseOwner",
		"leaseExpiresAt",
		"leaseExpired",
		"staleAfterMs",
		"stalled",
		"orphaned",
		"recoveryStatus",
		"recoveryAttempt",
		"recoveryReason",
		"recoveryUpdatedAt",
		"deadLetterReason",
		"completionDeliveryAt",
		"completionDeadLetterAt",
		"completionDeliveryAttempt",
	} {
		if value, ok := diagnostics[key]; ok {
			out[key] = value
		}
	}
}

func taskJobTerminalStatus(status string) bool {
	return TaskJobTerminal(domainjob.Record{Status: status})
}

func SummaryFromThread(thread map[string]any, turnID string) string {
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if mapString(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for i := len(items) - 1; i >= 0; i-- {
			item, _ := items[i].(map[string]any)
			if mapString(item, "kind") == "assistant_text" {
				return strings.TrimSpace(mapString(item, "text"))
			}
		}
	}
	return ""
}

func ToolInvocationCountFromThread(thread map[string]any, turnID string) int {
	turns, _ := thread["turns"].([]any)
	count := 0
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if strings.TrimSpace(turnID) != "" && mapString(turn, "id") != turnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if mapString(item, "kind") == "tool_call" && strings.TrimSpace(mapString(item, "callId")) != "" {
				count++
			}
		}
	}
	return count
}

func AddUsageMetadata(child map[string]any, usage map[string]any) {
	for key, value := range projectPublicUsageNumbersV1(usage) {
		child[key] = value
	}
}

func projectPublicUsageNumbersV1(usage map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{
		"inputTokens",
		"promptTokens",
		"outputTokens",
		"completionTokens",
		"reasoningTokens",
		"totalTokens",
		"cachedTokens",
		"cacheHitTokens",
		"cacheMissTokens",
		"cacheHitRate",
		"cacheableTokenHitRate",
		"totalInputTokenHitRate",
		"costUsd",
		"costCny",
		"cacheSavingsUsd",
		"cacheSavingsCny",
		"tokenEconomySavingsTokens",
		"tokenEconomySavingsUsd",
		"tokenEconomySavingsCny",
	} {
		value, found := usage[key]
		if !found {
			continue
		}
		number, ok := numericValueV1(value)
		if !ok || number < 0 || number > 1<<53-1 {
			continue
		}
		out[key] = number
	}
	return out
}

func AddEvidenceMetadata(out map[string]any, summary string, returnFormat string) {
	if NormalizeReturnFormat(returnFormat) != "evidence" {
		return
	}
	evidence, ok := EvidenceBundleFromSummary(summary)
	if ok {
		out["evidence"] = evidence
		out["evidenceBundleStatus"] = "parsed"
		out["evidenceCount"] = float64(len(evidence))
		return
	}
	out["evidence"] = []any{}
	out["evidenceBundleStatus"] = "not_found"
	out["evidenceCount"] = float64(0)
}

func AddEvidenceSummaryMetadata(out map[string]any, summary string, returnFormat string) {
	if NormalizeReturnFormat(returnFormat) != "evidence" {
		return
	}
	evidence, ok := EvidenceBundleFromSummary(summary)
	if ok {
		out["evidenceBundleStatus"] = "parsed"
		out["evidenceCount"] = float64(len(evidence))
		return
	}
	out["evidenceBundleStatus"] = "not_found"
	out["evidenceCount"] = float64(0)
}

func DurationMs(record domainjob.Record) float64 {
	started := strings.TrimSpace(record.StartedAt)
	if started == "" {
		return 0
	}
	ended := firstNonEmptyAnyString(record.FinishedAt, record.UpdatedAt)
	if strings.TrimSpace(ended) == "" {
		return 0
	}
	startTime, err := time.Parse(time.RFC3339Nano, started)
	if err != nil {
		return 0
	}
	endTime, err := time.Parse(time.RFC3339Nano, ended)
	if err != nil || endTime.Before(startTime) {
		return 0
	}
	return float64(endTime.Sub(startTime).Milliseconds())
}

func AddParallelMetadata(out map[string]any, record domainjob.Record) {
	groupID := strings.TrimSpace(record.ParallelGroupID)
	if groupID == "" {
		return
	}
	out["parallelGroupId"] = groupID
	if record.ParallelIndex > 0 {
		out["parallelIndex"] = float64(record.ParallelIndex)
	}
}

func FailedOutput(request TaskRequest, message string) map[string]any {
	out := map[string]any{
		"code":        "subagent_failed",
		"failureCode": domainjob.FailureChildExecutionFailed,
		"status":      string(domainjob.StatusFailed),
		"label":       request.Label,
		"profile":     request.ProfileName,
		"toolPolicy":  request.ToolPolicy,
	}
	if maxChildRuns, ok := publicMaxChildRunsLimit(message); ok {
		out["reasonCode"] = "subagent_child_run_limit_reached"
		out["message"] = "subagent child run limit reached: maxChildRuns=" + strconv.Itoa(maxChildRuns)
		out["maxChildRuns"] = float64(maxChildRuns)
	}
	return out
}

func publicMaxChildRunsLimit(message string) (int, bool) {
	const prefix = "subagent child run limit reached: maxChildRuns="
	value, ok := strings.CutPrefix(strings.TrimSpace(message), prefix)
	if !ok || value == "" {
		return 0, false
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 0 || strconv.Itoa(limit) != value {
		return 0, false
	}
	return limit, true
}

func SourceToolScopeMismatchOutput(request TaskRequest) map[string]any {
	out := FailedOutput(request, "")
	out["reasonCode"] = "subagent_reference_tool_scope_mismatch"
	out["message"] = ErrSourceToolScopeMismatch.Error()
	return out
}

func SourceNotReplayableOutput(request TaskRequest, sourceID, status string) map[string]any {
	out := FailedOutput(request, "")
	status = domainjob.PublicStatusV1(status)
	if status == "unknown" {
		status = "failed"
	}
	sourceID = publicTaskJobReferenceIDV1(sourceID)
	out["reasonCode"] = "subagent_reference_not_replayable"
	out["sourceRef"] = sourceID
	out["sourceStatus"] = status
	out["message"] = "subagent reference " + strconv.Quote(sourceID) + " is " + status + " and cannot be continued or forked"
	return out
}

func ErrorForTerminalRecord(record domainjob.Record, _ string) error {
	code := domainjob.NormalizeFailureCode(record.FailureCode, record.Status, true)
	if code == "" {
		code = domainjob.FailureChildUnknown
	}
	return errors.New(code)
}

func ParallelStatusFromErrors(errors []bool) string {
	for _, isError := range errors {
		if isError {
			return string(domainjob.StatusFailed)
		}
	}
	return string(domainjob.StatusCompleted)
}

func cloneMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func floatFromAny(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func mapString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
