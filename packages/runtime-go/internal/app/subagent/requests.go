package subagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type TaskRequest struct {
	ID                         string
	DependsOn                  []string
	Name                       string
	Prompt                     string
	Label                      string
	NameExplicit               bool
	LabelExplicit              bool
	Workspace                  string
	ProviderID                 string
	Model                      string
	EndpointFormat             string
	Variant                    string
	Effort                     string
	ProfileName                string
	ModelExplicit              bool
	ProviderExplicit           bool
	EndpointExplicit           bool
	VariantExplicit            bool
	ProfileExecutionConfigured bool
	ProfileDisplayName         string
	ProfileDescription         string
	ProfileMode                string
	ProfileHidden              bool
	ProfileColor               string
	ProfileIcon                string
	ToolPolicy                 string
	ToolPolicySet              bool
	ToolPolicySource           string
	Tools                      []string
	SkillPackageDigest         string
	BlockedTools               []string
	BlockedMCPServers          []string
	BlockedSkills              []string
	MaxSteps                   int
	MaxStepsSet                bool
	TokenBudget                int
	TokenBudgetSet             bool
	TimeBudgetMS               int
	TimeBudgetMSSet            bool
	ReturnFormat               string
	SystemPrompt               string
	RunInBackground            bool
	AutoContinueParent         bool
	IsolationMode              string
	ContinueFrom               string
	ForkFrom                   string
	ParallelGroupID            string
	ParallelIndex              int
	AcquireQueued              chan<- struct{}
	parallelDependencyPrompt   *parallelDependencyPromptV1
	// CaseDelegation is host-issued process-private input. Provider arguments
	// cannot populate it and ordinary unbound tasks leave it nil.
	CaseDelegation            *domainjob.CaseDelegationContextV1
	caseDelegationPreparation *preparedCaseDelegationV1
	caseAnswerSlotBinding     *domainjob.CaseDelegatedAnswerSlotBindingV1
	// ForegroundHandoff is host-owned state. It is never parsed from provider
	// arguments and may only be enabled after validating a host-general-only
	// parent call against the closed foreground task contract.
	ForegroundHandoff bool
	// ForegroundHandoffKind is host-owned and distinguishes the unchanged raw
	// general lane from the closed case-typed lane. It is never decoded from
	// provider arguments or persisted as child output.
	ForegroundHandoffKind string
}

type parallelDependencyPromptV1 struct{ original, expanded string }

// OriginalParallelRequestV1 removes only the immutable prompt expansion made
// by the host parallel runner. Tool arguments cannot populate this witness.
func (request TaskRequest) OriginalParallelRequestV1() (TaskRequest, bool) {
	witness := request.parallelDependencyPrompt
	if witness == nil || request.Prompt != witness.expanded || len(request.DependsOn) == 0 {
		return request, false
	}
	request.Prompt = witness.original
	request.parallelDependencyPrompt = nil
	return request, true
}

func (request TaskRequest) UseCaseAnswerSlotBindingV1(
	use func(domainjob.CaseDelegatedAnswerSlotBindingV1) error,
) error {
	if request.caseAnswerSlotBinding == nil || use == nil {
		return errors.New("case answer-slot binding is unavailable")
	}
	binding := *request.caseAnswerSlotBinding
	binding.AnswerSlot.Gaps = append([]string{}, binding.AnswerSlot.Gaps...)
	binding.Claims = append([]domainjob.CaseDelegatedClaimReferenceV1{}, binding.Claims...)
	binding.Evidence = append([]domainjob.CaseDelegatedEvidenceReferenceV1{}, binding.Evidence...)
	return use(binding)
}

type ParallelTaskRequest struct {
	Index   int
	ID      string
	Request TaskRequest
}

func WithSystemPromptHint(request TaskRequest, hint string) TaskRequest {
	if hint = strings.TrimSpace(hint); hint != "" {
		request.SystemPrompt = strings.TrimSpace(request.SystemPrompt + "\n" + hint)
	}
	return request
}

func ParallelTaskRequestsFromArgs(args map[string]any) ([]ParallelTaskRequest, error) {
	rawTasks, _ := args["tasks"].([]any)
	if len(rawTasks) == 0 {
		return nil, errors.New("tasks is required")
	}
	if len(rawTasks) == 1 {
		return nil, errors.New("parallel_tasks with a single task is equivalent to task; use task instead")
	}
	tasks := make([]ParallelTaskRequest, 0, len(rawTasks))
	ids := map[string]bool{}
	for index, raw := range rawTasks {
		taskMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tasks[%d] must be an object", index)
		}
		request, err := TaskRequestFromArgs("task", taskMap)
		if err != nil {
			return nil, fmt.Errorf("tasks[%d]: %s", index, err.Error())
		}
		id := strings.TrimSpace(firstNonEmptyAnyString(taskMap["id"], taskMap["task_id"], taskMap["taskId"]))
		if id == "" {
			id = fmt.Sprintf("task_%d", index+1)
		}
		request.ID = id
		request.DependsOn = UniqueStringList(append(stringList(taskMap["depends_on"]), stringList(taskMap["dependsOn"])...))
		if ids[id] {
			return nil, fmt.Errorf("tasks[%d] has duplicate id %q", index, id)
		}
		ids[id] = true
		tasks = append(tasks, ParallelTaskRequest{Index: index, ID: id, Request: request})
	}
	for _, task := range tasks {
		for _, dependency := range task.Request.DependsOn {
			if dependency == task.ID {
				return nil, fmt.Errorf("task %q cannot depend on itself", task.ID)
			}
			if !ids[dependency] {
				return nil, fmt.Errorf("task %q depends on unknown task %q", task.ID, dependency)
			}
		}
	}
	if ParallelHasDependencyCycle(tasks) {
		return nil, errors.New("parallel_tasks dependency cycle detected")
	}
	return tasks, nil
}

func ParallelDependenciesComplete(dependsOn []string, completed map[string]bool) bool {
	for _, dependency := range dependsOn {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func ParallelDependencyContext(dependsOn []string, completedSummaries map[string]string) string {
	lines := []string{}
	for _, dependency := range dependsOn {
		summary := strings.TrimSpace(completedSummaries[dependency])
		if summary == "" {
			continue
		}
		lines = append(lines, "- "+dependency+": "+truncateText(summary, 2000))
	}
	return truncateText(strings.Join(lines, "\n"), 6000)
}

func DependencySummary(output map[string]any) string {
	if output == nil || SecurityBoundOutputMap(output) {
		return ""
	}
	summary := firstNonEmptyAnyString(output["summary"], output["output"], output["message"])
	if strings.TrimSpace(summary) == "" && output["error"] != nil {
		summary = "error: " + firstNonEmptyAnyString(output["error"])
	}
	return strings.TrimSpace(summary)
}

func ParallelHasDependencyCycle(tasks []ParallelTaskRequest) bool {
	remaining := map[string]ParallelTaskRequest{}
	completed := map[string]bool{}
	for _, task := range tasks {
		remaining[task.ID] = task
	}
	for len(remaining) > 0 {
		progress := false
		for id, task := range remaining {
			if ParallelDependenciesComplete(task.Request.DependsOn, completed) {
				completed[id] = true
				delete(remaining, id)
				progress = true
			}
		}
		if !progress {
			return true
		}
	}
	return false
}

func UniqueStringList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func TaskRequestFromArgs(toolName string, args map[string]any) (TaskRequest, error) {
	_ = toolName
	prompt := firstNonEmptyAnyString(args["prompt"], args["task"])
	if strings.TrimSpace(prompt) == "" {
		return TaskRequest{}, errors.New("prompt is required")
	}
	explicitName := firstNonEmptyAnyString(args["name"], args["subagentName"])
	explicitLabel := firstNonEmptyAnyString(args["label"], args["description"])
	label := firstNonEmptyAnyString(explicitLabel, explicitName, PromptLabel(prompt))
	maxSteps := 0
	maxStepsSet := false
	if value, ok := numericAny(args["max_steps"]); ok {
		maxSteps = value
		maxStepsSet = true
	} else if value, ok := numericAny(args["maxSteps"]); ok {
		maxSteps = value
		maxStepsSet = true
	}
	tokenBudget := 0
	tokenBudgetSet := false
	if value, ok := numericAny(firstNonNilValue(args["token_budget"], args["tokenBudget"])); ok && value > 0 {
		tokenBudget = value
		tokenBudgetSet = true
	}
	timeBudgetMS := 0
	timeBudgetMSSet := false
	if value, ok := numericAny(firstNonNilValue(args["time_budget_ms"], args["timeBudgetMs"])); ok && value > 0 {
		timeBudgetMS = value
		timeBudgetMSSet = true
	} else if value, ok := numericAny(firstNonNilValue(args["time_budget_seconds"], args["timeBudgetSeconds"])); ok && value > 0 {
		timeBudgetMS = value * 1000
		timeBudgetMSSet = true
	}
	profile := strings.TrimSpace(firstNonEmptyAnyString(args["profile"]))
	toolPolicy := NormalizeToolPolicy(firstNonEmptyAnyString(args["toolPolicy"], args["tool_policy"]))
	toolPolicySet := toolPolicy != ""
	toolPolicySource := ""
	if toolPolicySet {
		toolPolicySource = "request"
	}
	if profile == "inherit" && !toolPolicySet {
		toolPolicy = "inherit"
		toolPolicySet = true
		toolPolicySource = "request"
	}
	isolationMode := NormalizeIsolationMode(firstNonEmptyAnyString(args["isolationMode"], args["isolation_mode"]))
	if isolationMode == "" && strings.TrimSpace(firstNonEmptyAnyString(args["isolationMode"], args["isolation_mode"])) != "" {
		return TaskRequest{}, fmt.Errorf("unsupported isolationMode %q", firstNonEmptyAnyString(args["isolationMode"], args["isolation_mode"]))
	}
	model := strings.TrimSpace(firstNonEmptyAnyString(args["model"]))
	providerID := strings.TrimSpace(firstNonEmptyAnyString(args["providerId"], args["provider_id"]))
	endpointFormat := domainmodel.OptionalEndpointFormat(firstNonEmptyAnyString(args["endpointFormat"], args["endpoint_format"]))
	variant := strings.TrimSpace(firstNonEmptyAnyString(args["variant"], args["modelVariant"], args["model_variant"]))
	effort, effortErr := subagentReasoningEffortFromValues(args["effort"], args["reasoningEffort"])
	if effortErr != nil {
		return TaskRequest{}, effortErr
	}
	continueFrom := firstNonEmptyAnyString(args["continue_from"], args["continueFrom"])
	if continueFrom == "" {
		continueFrom = firstNonEmptyAnyString(args["task_id"], args["taskId"])
	}
	return TaskRequest{
		Name:               strings.TrimSpace(explicitName),
		Prompt:             prompt,
		Label:              label,
		NameExplicit:       strings.TrimSpace(explicitName) != "",
		LabelExplicit:      strings.TrimSpace(firstNonEmptyAnyString(explicitLabel, explicitName)) != "",
		Workspace:          firstNonEmptyAnyString(args["workspace"]),
		ProviderID:         providerID,
		Model:              model,
		EndpointFormat:     endpointFormat,
		Variant:            variant,
		Effort:             effort,
		ProfileName:        profile,
		ModelExplicit:      model != "",
		ProviderExplicit:   providerID != "",
		EndpointExplicit:   endpointFormat != "",
		VariantExplicit:    variant != "",
		ToolPolicy:         toolPolicy,
		ToolPolicySet:      toolPolicySet,
		ToolPolicySource:   toolPolicySource,
		Tools:              stringList(args["tools"]),
		BlockedTools:       stringList(firstNonNilValue(args["blockedTools"], args["blocked_tools"])),
		BlockedMCPServers:  stringList(firstNonNilValue(args["blockedMcpServers"], args["blocked_mcp_servers"])),
		BlockedSkills:      stringList(firstNonNilValue(args["blockedSkills"], args["blocked_skills"])),
		MaxSteps:           maxSteps,
		MaxStepsSet:        maxStepsSet,
		TokenBudget:        tokenBudget,
		TokenBudgetSet:     tokenBudgetSet,
		TimeBudgetMS:       timeBudgetMS,
		TimeBudgetMSSet:    timeBudgetMSSet,
		ReturnFormat:       NormalizeReturnFormat(firstNonEmptyAnyString(args["returnFormat"], args["return_format"])),
		RunInBackground:    boolField(args, "run_in_background") || boolField(args, "runInBackground"),
		AutoContinueParent: boolField(args, "auto_continue_parent") || boolField(args, "autoContinueParent"),
		IsolationMode:      isolationMode,
		ContinueFrom:       continueFrom,
		ForkFrom:           firstNonEmptyAnyString(args["fork_from"], args["forkFrom"]),
	}, nil
}

func PromptLabel(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "Subagent"
	}
	lines := strings.Split(prompt, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.Trim(line, "`#*-0123456789. \t"))
		if line == "" {
			continue
		}
		return ShortLabel(line)
	}
	return ShortLabel(prompt)
}

func ShortLabel(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return "Subagent"
	}
	runes := []rune(value)
	if len(runes) <= 48 {
		return value
	}
	return string(runes[:48]) + "..."
}

func DisplayNameCandidate(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "child agent:") {
		value = strings.TrimSpace(value[len("Child agent:"):])
	}
	value = strings.TrimSpace(strings.TrimSuffix(value, " fork"))
	if value == "" || GenericDisplayName(value) || PromptLikeDisplayName(value) {
		return ""
	}
	return ShortLabel(value)
}

func PromptLikeDisplayName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	runes := []rune(value)
	if len(runes) > 32 {
		return true
	}
	if strings.ContainsAny(value, "\n\r") {
		return true
	}
	if len(strings.Fields(value)) > 4 {
		return true
	}
	if strings.ContainsAny(value, "，。！？；：、,.!?;:") && len(runes) > 12 {
		return true
	}
	lower := strings.ToLower(value)
	for _, prefix := range []string{"please inspect", "please review", "please analyze", "please analyse", "inspect ", "review ", "analyze ", "analyse ", "generate ", "summarize ", "summarise ", "search ", "read ", "create ", "build "} {
		if strings.HasPrefix(lower, prefix) && (strings.Contains(value, " ") || len(runes) > 16) {
			return true
		}
	}
	for _, prefix := range []string{"请", "基于", "根据", "分析", "整合", "生成", "查看", "读取", "审查", "调研", "搜索", "检查"} {
		if strings.HasPrefix(value, prefix) && len(runes) > 4 {
			return true
		}
	}
	return false
}

func GeneratedNickname(candidates ...string) string {
	seed := ""
	for _, candidate := range candidates {
		if value := strings.TrimSpace(candidate); value != "" {
			seed = value
			break
		}
	}
	if seed == "" {
		return "Subagent"
	}
	ordinal := trailingInteger(seed)
	if ordinal > 0 {
		return fallbackNames[(ordinal-1)%len(fallbackNames)]
	}
	return fallbackNames[hashSeed(seed)%uint32(len(fallbackNames))]
}

func ParallelIndexSeed(value int) string {
	if value > 0 {
		return strconv.Itoa(value)
	}
	return ""
}

func GenericDisplayName(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "child agent:")
	value = strings.TrimSpace(value)
	switch value {
	case "", "task", "delegate_task", "subagent", "child-run", "child run", "parallel-child-run":
		return true
	default:
		return false
	}
}

func NormalizeToolPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case "inherit":
		return "inherit"
	case "readOnly", "read-only", "readonly", "read_only":
		return "readOnly"
	default:
		return ""
	}
}

func NormalizeIsolationMode(value string) string {
	switch strings.TrimSpace(value) {
	case "", "none":
		return ""
	case "worktree":
		return "worktree"
	default:
		return ""
	}
}

func NormalizeProfileMode(value string) string {
	switch strings.TrimSpace(value) {
	case "primary", "subagent", "all":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func NormalizeReturnFormat(value string) string {
	switch strings.TrimSpace(value) {
	case "summary", "evidence", "transcriptRef":
		return strings.TrimSpace(value)
	case "transcript_ref", "transcript-ref":
		return "transcriptRef"
	default:
		return "summary"
	}
}

func subagentReasoningEffortFromValues(values ...any) (string, error) {
	for _, raw := range values {
		if raw == nil {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return "", ErrReasoningEffortInvalid
		}
		if value == "" {
			continue
		}
		if err := validateSubagentReasoningEffort(value); err != nil {
			return "", err
		}
		return value, nil
	}
	return "", nil
}

var fallbackNames = []string{"Lagrange", "Nash", "Noether", "Turing", "Euler", "Ada", "Kepler", "Curie", "Fermi", "Gauss", "Hopper", "Bohr"}

func trailingInteger(value string) int {
	runes := []rune(value)
	end := len(runes)
	start := end
	for start > 0 && runes[start-1] >= '0' && runes[start-1] <= '9' {
		start--
	}
	if start == end {
		return 0
	}
	n := 0
	for _, ch := range runes[start:end] {
		n = n*10 + int(ch-'0')
	}
	return n
}

func hashSeed(value string) uint32 {
	hash := uint32(2166136261)
	for _, ch := range value {
		hash ^= uint32(ch)
		hash *= 16777619
	}
	return hash
}

func stringList(value any) []string {
	raw := []any{}
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		raw = make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return ""
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func truncateText(value string, limit int) string {
	if len([]byte(value)) <= limit {
		return value
	}
	if limit <= 0 {
		return ""
	}
	data := []byte(value)
	if len(data) <= limit {
		return value
	}
	return string(data[:limit]) + "\n[truncated]"
}
