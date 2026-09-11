package threadsummary

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

var threadSummaryPublicIDV1 = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

var threadSummaryTaskJobKindsV1 = map[string]bool{
	"task": true, "parallel_task": true, "background-shell": true, "bash": true,
	"subagent": true, "child-run": true, "parallel-child-run": true, "unknown": true,
}

var threadSummaryTaskJobStatusesV1 = map[string]bool{
	"queued": true, "running": true, "pause_requested": true, "paused": true,
	"resume_requested": true, "resuming": true, "completed": true, "failed": true,
	"aborted": true, "interrupted": true, "killed": true, "canceled": true,
	"timeout": true, "unknown": true,
}

var threadSummaryCommandStatusesV1 = map[string]bool{
	"running": true, "pending": true, "stopped": true, "failed": true,
	"aborted": true, "killed": true, "canceled": true, "timeout": true,
	"error": true, "completed": true, "done": true, "success": true, "unknown": true,
}

var threadSummaryPublicStatusesV1 = map[string]bool{
	"queued": true, "running": true, "pause_requested": true, "paused": true,
	"resume_requested": true, "resuming": true, "completed": true, "failed": true,
	"aborted": true, "interrupted": true, "killed": true, "canceled": true,
	"timeout": true, "unknown": true, "starting": true, "done": true,
	"missing": true, "stopped": true, "archived": true, "idle": true,
	"pending": true, "error": true, "success": true,
}

var threadSummaryModelSourcesV1 = map[string]bool{
	"thread": true, "subagent-profile": true, "explicit-input": true,
	"session": true, "runtime-default": true,
}

// ValidateSummaryResponseV1 is the host-side public boundary for the summary
// read model. TypeScript validation is defense in depth, not publication
// authority: every successful Go response must satisfy this exact contract.
func ValidateSummaryResponseV1(value map[string]any, expectedThreadID string) error {
	if value == nil || domainevent.ValidatePublicRecord(value) != nil {
		return errors.New("thread summary contains non-public content")
	}
	if !closedObjectV1(value,
		[]string{"threadId", "generatedAt", "latestSeq", "subagents", "tasks", "outputs", "sources", "sideChats", "backgroundProcesses"},
		[]string{"historyAuthority"},
	) {
		return errors.New("thread summary root shape is invalid")
	}
	threadID, ok := requiredPublicIDV1(value["threadId"])
	if !ok || threadID != strings.TrimSpace(expectedThreadID) {
		return errors.New("thread summary identity is invalid")
	}
	if !rfc3339TimeV1(value["generatedAt"]) || !nonNegativeIntegerV1(value["latestSeq"]) {
		return errors.New("thread summary generation metadata is invalid")
	}
	if authority, exists := value["historyAuthority"]; exists && authority != "case_boundary_only_v1" {
		return errors.New("thread summary history authority is invalid")
	}

	subagents, ok := mapSliceV1(value["subagents"])
	if !ok {
		return errors.New("thread summary subagents are invalid")
	}
	for _, subagent := range subagents {
		if err := validateSummarySubagentV1(subagent, threadID); err != nil {
			return err
		}
	}
	tasks, ok := mapSliceV1(value["tasks"])
	if !ok {
		return errors.New("thread summary tasks are invalid")
	}
	for _, task := range tasks {
		if err := ValidateTaskProjectionV1(task); err != nil {
			return err
		}
	}
	for _, key := range []string{"outputs", "sources", "backgroundProcesses"} {
		entries, valid := anySliceV1(value[key])
		if !valid || len(entries) != 0 {
			return errors.New("thread summary fact-bearing collections must be empty")
		}
	}
	sideChats, ok := mapSliceV1(value["sideChats"])
	if !ok {
		return errors.New("thread summary side chats are invalid")
	}
	for _, sideChat := range sideChats {
		if err := validateSummarySideChatV1(sideChat, threadID); err != nil {
			return err
		}
	}
	if _, restricted := value["historyAuthority"]; restricted &&
		(len(tasks) != 0 || len(sideChats) != 0) {
		return errors.New("case-bound thread summary contains non-subagent metadata")
	}
	return nil
}

// ValidateTaskMutationResponseV1 protects kill/restart responses with the
// same closed projection used by the summary list.
func ValidateTaskMutationResponseV1(value map[string]any, expectedTaskID string) error {
	if value == nil || domainevent.ValidatePublicRecord(value) != nil ||
		!closedObjectV1(value, []string{"task"}, nil) {
		return errors.New("thread summary task mutation response is invalid")
	}
	task, ok := value["task"].(map[string]any)
	if !ok || ValidateTaskProjectionV1(task) != nil {
		return errors.New("thread summary task mutation projection is invalid")
	}
	actualID, _ := task["id"].(string)
	if !sameSummaryTaskIdentityV1(actualID, expectedTaskID) {
		return errors.New("thread summary task mutation identity is invalid")
	}
	return nil
}

// ValidateTaskProjectionV1 accepts only the common task-job projection or the
// metadata-only command projection. No output, path, command, or diagnostics
// field is legal in either variant.
func ValidateTaskProjectionV1(value map[string]any) error {
	if value == nil || domainevent.ValidatePublicRecord(value) != nil {
		return errors.New("thread summary task contains non-public content")
	}
	kind, _ := value["kind"].(string)
	if kind == "command" {
		return validateCommandTaskProjectionV1(value)
	}
	return validateTaskJobProjectionV1(value)
}

// ValidateTaskOutputResponseV1 keeps command-private and child-untrusted
// output variants distinct. A transport success can never upgrade either
// variant into readable or evidentiary output.
func ValidateTaskOutputResponseV1(value map[string]any, expectedTaskID string) error {
	if value == nil || domainevent.ValidatePublicRecord(value) != nil || !closedObjectV1(value, []string{
		"schemaVersion", "availability", "taskId", "status", "reasonCode", "outputWithheld",
		"outputTrustStatus", "factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent",
	}, nil) || !numericLiteralV1(value["schemaVersion"], 1) || value["availability"] != "withheld" ||
		value["outputWithheld"] != true || value["factAnswerAllowed"] != false ||
		value["evidenceAuthority"] != false || value["canReadOutput"] != false ||
		value["canContinueParent"] != false {
		return errors.New("thread summary task output response shape is invalid")
	}
	taskID, ok := requiredPublicIDV1(value["taskId"])
	if !ok || !sameSummaryTaskIdentityV1(taskID, expectedTaskID) {
		return errors.New("thread summary task output identity is invalid")
	}
	status, _ := value["status"].(string)
	reason, _ := value["reasonCode"].(string)
	trust, _ := value["outputTrustStatus"].(string)
	switch reason {
	case "security_bound_child_output":
		if trust != "untrusted_child_output" || !threadSummaryTaskJobStatusesV1[status] ||
			!strings.HasPrefix(canonicalSummaryTaskIdentityV1(taskID), "taskjob:") {
			return errors.New("thread summary child output variant is invalid")
		}
	case "tool_output_private":
		if trust != "private_tool_output" || !threadSummaryCommandStatusesV1[status] ||
			!strings.HasPrefix(taskID, "command:") {
			return errors.New("thread summary command output variant is invalid")
		}
	default:
		return errors.New("thread summary task output reason is invalid")
	}
	return nil
}

func validateTaskJobProjectionV1(value map[string]any) error {
	if !closedObjectV1(value, []string{
		"schemaVersion", "id", "kind", "status", "background", "terminal", "outputWithheld",
		"outputTrustStatus", "factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent",
	}, []string{"active"}) || !numericLiteralV1(value["schemaVersion"], 1) {
		return errors.New("thread summary task-job shape is invalid")
	}
	id, ok := requiredPublicIDV1(value["id"])
	if !ok || !strings.HasPrefix(id, "taskjob:") {
		return errors.New("thread summary task-job identity is invalid")
	}
	kind, kindOK := value["kind"].(string)
	status, statusOK := value["status"].(string)
	background, backgroundOK := value["background"].(bool)
	terminal, terminalOK := value["terminal"].(bool)
	_ = background
	if !kindOK || !threadSummaryTaskJobKindsV1[kind] || !statusOK || !threadSummaryTaskJobStatusesV1[status] ||
		!backgroundOK || !terminalOK || terminal != domainjob.TerminalStatusV1(status) ||
		value["outputWithheld"] != true || value["outputTrustStatus"] != "untrusted_child_output" ||
		value["factAnswerAllowed"] != false || value["evidenceAuthority"] != false ||
		value["canReadOutput"] != false || value["canContinueParent"] != false {
		return errors.New("thread summary task-job semantics are invalid")
	}
	if active, exists := value["active"]; exists {
		activeValue, valid := active.(bool)
		if !valid || activeValue != taskJobActiveStatusV1(status) {
			return errors.New("thread summary task-job active flag is invalid")
		}
	}
	return nil
}

func validateCommandTaskProjectionV1(value map[string]any) error {
	if !closedObjectV1(value, []string{
		"schemaVersion", "id", "kind", "status", "background", "active", "terminal", "outputWithheld",
		"outputTrustStatus", "factAnswerAllowed", "evidenceAuthority", "canReadOutput", "canContinueParent",
	}, nil) || !numericLiteralV1(value["schemaVersion"], 1) || value["kind"] != "command" ||
		value["background"] != false || value["outputWithheld"] != true ||
		value["outputTrustStatus"] != "private_tool_output" || value["factAnswerAllowed"] != false ||
		value["evidenceAuthority"] != false || value["canReadOutput"] != false ||
		value["canContinueParent"] != false {
		return errors.New("thread summary command task shape is invalid")
	}
	id, ok := requiredPublicIDV1(value["id"])
	status, statusOK := value["status"].(string)
	active, activeOK := value["active"].(bool)
	terminal, terminalOK := value["terminal"].(bool)
	if !ok || !strings.HasPrefix(id, "command:") || !statusOK || !threadSummaryCommandStatusesV1[status] ||
		!activeOK || active != commandActiveStatusV1(status) || !terminalOK || terminal != commandTerminalStatusV1(status) {
		return errors.New("thread summary command task semantics are invalid")
	}
	return nil
}

func validateSummarySubagentV1(value map[string]any, expectedParentThreadID string) error {
	if !closedObjectV1(value, []string{
		"schemaVersion", "id", "key", "parentThreadId", "status", "rawStatus", "outputTrustStatus",
		"canOpenThread", "canKill", "canRestart", "updatedAt", "outputWithheld", "factAnswerAllowed",
		"evidenceAuthority", "canContinueParent", "canReadOutput",
	}, []string{
		"parentTurnId", "parentToolCallId", "childId", "childRunId", "taskJobId", "taskKind",
		"childThreadId", "childTurnId", "displayName", "agentNickname", "title", "label", "model",
		"providerId", "endpointFormat", "variant", "modelSource", "effort", "profile", "toolPolicy",
		"maxModelSteps", "timeBudgetMs",
		"background", "diagnostics", "parallelGroupId", "parallelIndex", "toolInvocations",
		"evidenceLedgered", "durationMs", "queuedMs", "totalTokens", "cacheHitRate", "costUsd",
		"costCny", "createdAt",
	}) || !numericLiteralV1(value["schemaVersion"], 1) || value["outputWithheld"] != true ||
		value["outputTrustStatus"] != "untrusted_child_output" || value["factAnswerAllowed"] != false ||
		value["evidenceAuthority"] != false || value["canContinueParent"] != false ||
		value["canReadOutput"] != false || value["canKill"] != false || value["canRestart"] != false {
		return errors.New("thread summary subagent shape is invalid")
	}
	id, idOK := requiredPublicIDV1(value["id"])
	key, keyOK := requiredPublicIDV1(value["key"])
	parent, parentOK := requiredPublicIDV1(value["parentThreadId"])
	status, statusOK := value["status"].(string)
	rawStatus, rawStatusOK := value["rawStatus"].(string)
	canOpen, canOpenOK := value["canOpenThread"].(bool)
	if !idOK || !keyOK || id != key || !parentOK || parent != expectedParentThreadID ||
		!statusOK || !rawStatusOK || !threadSummaryPublicStatusesV1[rawStatus] ||
		status != NormalizeActiveStatus(rawStatus) || !canOpenOK || !rfc3339TimeV1(value["updatedAt"]) {
		return errors.New("thread summary subagent identity or lifecycle is invalid")
	}
	childThreadID := ""
	if raw, exists := value["childThreadId"]; exists {
		var ok bool
		childThreadID, ok = requiredPublicIDV1(raw)
		if !ok {
			return errors.New("thread summary subagent child thread identity is invalid")
		}
	}
	if canOpen != (childThreadID != "") {
		return errors.New("thread summary subagent open-thread flag is invalid")
	}
	for _, key := range []string{"parentTurnId", "parentToolCallId", "childId", "childRunId", "taskJobId", "childTurnId", "parallelGroupId"} {
		if raw, exists := value[key]; exists {
			if _, ok := requiredPublicIDV1(raw); !ok {
				return errors.New("thread summary subagent lineage identity is invalid")
			}
		}
	}
	if raw, exists := value["taskKind"]; exists {
		kind, ok := raw.(string)
		if !ok || !threadSummaryTaskJobKindsV1[kind] {
			return errors.New("thread summary subagent task kind is invalid")
		}
	}
	if raw, exists := value["modelSource"]; exists {
		source, ok := raw.(string)
		if !ok || !threadSummaryModelSourcesV1[source] {
			return errors.New("thread summary subagent model source is invalid")
		}
	}
	for _, key := range []string{"displayName", "agentNickname", "title", "label", "model", "providerId", "endpointFormat", "variant", "profile", "toolPolicy"} {
		if raw, exists := value[key]; exists {
			if _, ok := raw.(string); !ok {
				return errors.New("thread summary subagent display metadata is invalid")
			}
		}
	}
	if raw, exists := value["effort"]; exists {
		effort, ok := raw.(string)
		projected, valid := domainmodel.ProjectReasoningEffortV1(effort)
		if !ok || !valid || projected == "" {
			return errors.New("thread summary subagent reasoning effort is invalid")
		}
	}
	if raw, exists := value["background"]; exists {
		if _, ok := raw.(bool); !ok {
			return errors.New("thread summary subagent background flag is invalid")
		}
	}
	if raw, exists := value["evidenceLedgered"]; exists {
		if _, ok := raw.(bool); !ok {
			return errors.New("thread summary subagent ledger flag is invalid")
		}
	}
	for _, key := range []string{"parallelIndex", "toolInvocations", "durationMs", "queuedMs", "totalTokens", "maxModelSteps", "timeBudgetMs"} {
		if raw, exists := value[key]; exists && !nonNegativeIntegerV1(raw) {
			return errors.New("thread summary subagent integer metric is invalid")
		}
	}
	if raw, exists := value["cacheHitRate"]; exists && raw != nil {
		number, ok := finiteNumberV1(raw)
		if !ok || number < 0 || number > 1 {
			return errors.New("thread summary subagent cache metric is invalid")
		}
	}
	for _, key := range []string{"costUsd", "costCny"} {
		if raw, exists := value[key]; exists {
			number, ok := finiteNumberV1(raw)
			if !ok || number < 0 {
				return errors.New("thread summary subagent cost metric is invalid")
			}
		}
	}
	for _, key := range []string{"createdAt"} {
		if raw, exists := value[key]; exists && !rfc3339TimeV1(raw) {
			return errors.New("thread summary subagent timestamp is invalid")
		}
	}
	if raw, exists := value["diagnostics"]; exists {
		diagnostics, ok := raw.(map[string]any)
		if !ok || validateSummaryJobDiagnosticsV1(diagnostics, rawStatus, value["background"] == true) != nil {
			return errors.New("thread summary subagent diagnostics are invalid")
		}
	}
	return nil
}

func validateSummaryJobDiagnosticsV1(value map[string]any, expectedStatus string, expectedBackground bool) error {
	if !closedObjectV1(value, []string{"status", "terminal", "background", "paused"}, []string{"startedAt", "updatedAt", "finishedAt"}) {
		return errors.New("thread summary diagnostics shape is invalid")
	}
	status, ok := value["status"].(string)
	terminal, terminalOK := value["terminal"].(bool)
	background, backgroundOK := value["background"].(bool)
	paused, pausedOK := value["paused"].(bool)
	if !ok || !threadSummaryPublicStatusesV1[status] || status != expectedStatus || !terminalOK ||
		terminal != summaryTerminalStatusV1(status) || !backgroundOK || background != expectedBackground ||
		!pausedOK || paused != (status == "paused" || status == "pause_requested") {
		return errors.New("thread summary diagnostics semantics are invalid")
	}
	for _, key := range []string{"startedAt", "updatedAt", "finishedAt"} {
		if raw, exists := value[key]; exists && !rfc3339TimeV1(raw) {
			return errors.New("thread summary diagnostics timestamp is invalid")
		}
	}
	return nil
}

func validateSummarySideChatV1(value map[string]any, expectedParentThreadID string) error {
	if !closedObjectV1(value,
		[]string{"threadId", "title", "status", "relation", "parentThreadId", "createdAt", "updatedAt"},
		[]string{"messageCount", "turnCount"},
	) {
		return errors.New("thread summary side-chat shape is invalid")
	}
	threadID, threadOK := requiredPublicIDV1(value["threadId"])
	parentID, parentOK := requiredPublicIDV1(value["parentThreadId"])
	title, titleOK := value["title"].(string)
	status, statusOK := value["status"].(string)
	if !threadOK || threadID == expectedParentThreadID || !parentOK || parentID != expectedParentThreadID ||
		!titleOK || value["relation"] != "side" || !statusOK || !map[string]bool{
		"idle": true, "running": true, "archived": true, "deleted": true,
	}[status] || !rfc3339TimeV1(value["createdAt"]) || !rfc3339TimeV1(value["updatedAt"]) {
		return errors.New("thread summary side-chat semantics are invalid")
	}
	_ = title
	for _, key := range []string{"messageCount", "turnCount"} {
		if raw, exists := value[key]; exists && !nonNegativeIntegerV1(raw) {
			return errors.New("thread summary side-chat count is invalid")
		}
	}
	return nil
}

func closedObjectV1(value map[string]any, required []string, optional []string) bool {
	if value == nil {
		return false
	}
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = true
		if _, ok := value[key]; !ok {
			return false
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range value {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func requiredPublicIDV1(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && threadSummaryPublicIDV1.MatchString(text)
}

func rfc3339TimeV1(value any) bool {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) != text || text == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, text)
	return err == nil
}

func finiteNumberV1(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case int:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	case float32:
		number = float64(typed)
	case float64:
		number = typed
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func nonNegativeIntegerV1(value any) bool {
	number, ok := finiteNumberV1(value)
	return ok && number >= 0 && math.Trunc(number) == number
}

func numericLiteralV1(value any, expected float64) bool {
	number, ok := finiteNumberV1(value)
	return ok && number == expected
}

func mapSliceV1(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, entry := range typed {
			record, ok := entry.(map[string]any)
			if !ok || record == nil {
				return nil, false
			}
			out = append(out, record)
		}
		return out, true
	default:
		return nil, false
	}
}

func anySliceV1(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, entry := range typed {
			out = append(out, entry)
		}
		return out, true
	default:
		return nil, false
	}
}

func taskJobActiveStatusV1(status string) bool {
	switch status {
	case "queued", "running", "pause_requested", "paused", "resume_requested", "resuming":
		return true
	default:
		return false
	}
}

func commandActiveStatusV1(status string) bool {
	return status == "running" || status == "pending"
}

func commandTerminalStatusV1(status string) bool {
	switch status {
	case "failed", "aborted", "killed", "canceled", "timeout", "error", "completed", "done", "success":
		return true
	default:
		return false
	}
}

func summaryTerminalStatusV1(status string) bool {
	state := NormalizeActiveStatus(status)
	return state == "done" || state == "terminal"
}

func canonicalSummaryTaskIdentityV1(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "run:") {
		return "taskjob:" + strings.TrimPrefix(value, "run:")
	}
	if strings.HasPrefix(value, "job-") {
		return "taskjob:" + value
	}
	return value
}

func sameSummaryTaskIdentityV1(left string, right string) bool {
	left = canonicalSummaryTaskIdentityV1(left)
	right = canonicalSummaryTaskIdentityV1(right)
	return left != "" && left == right
}
