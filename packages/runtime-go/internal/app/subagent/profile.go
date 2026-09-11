package subagent

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

var (
	ErrSourceToolScopeMismatch = errors.New("subagent reference uses a different tool scope")
	ErrSourceNotReplayable     = errors.New("subagent reference cannot be continued or forked")
	ErrReasoningEffortInvalid  = errors.New("subagent reasoning effort is invalid")
)

type ProfileConfig struct {
	Name              string
	DisplayName       string
	Mode              string
	Hidden            bool
	Description       string
	ProviderID        string
	Model             string
	EndpointFormat    string
	Variant           string
	Effort            string
	EffortInvalid     bool
	PromptPreamble    string
	SystemPrompt      string
	ToolPolicy        string
	Tools             []string
	BlockedTools      []string
	BlockedMCPServers []string
	BlockedSkills     []string
	MaxSteps          int
	MaxStepsSet       bool
	TokenBudget       int
	TokenBudgetSet    bool
	TimeBudgetMS      int
	TimeBudgetMSSet   bool
	Color             string
	Icon              string
	Source            string
}

type ProfileSettings struct {
	Enabled           bool
	DefaultToolPolicy string
	DefaultProfile    string
	MaxParallel       int
	MaxChildRuns      int
	Profiles          map[string]ProfileConfig
}

type SourceValidation struct {
	ParentThreadID      string
	AllowAncestorSource bool
	Workspace           string
	ProviderID          string
	Model               string
	EndpointFormat      string
	Variant             string
	Effort              string
	ToolScope           []string
	ToolSchemaHash      string
	SystemPromptHash    string
	SkillPackageDigest  string
}

func ApplyProfile(request TaskRequest, settings ProfileSettings) (TaskRequest, error) {
	if request.ForegroundHandoff {
		if err := ValidateForegroundHandoffRequest(request); err != nil {
			return request, err
		}
		request.ToolPolicy = "readOnly"
		request.ToolPolicySet = true
		request.ToolPolicySource = "host-" + request.ForegroundHandoffKind
		request.Tools = []string{toolcatalogForegroundSubmitToolName}
		request.ReturnFormat = "summary"
		return request, nil
	}
	if err := validateSubagentReasoningEffort(request.Effort); err != nil {
		return request, err
	}
	if strings.TrimSpace(request.ProfileName) == "" && strings.TrimSpace(settings.DefaultProfile) != "" {
		request.ProfileName = strings.TrimSpace(settings.DefaultProfile)
	}
	if strings.TrimSpace(request.ProfileName) != "" && !strings.HasPrefix(request.ProfileName, "skill:") {
		if profile, ok := settings.Profiles[request.ProfileName]; ok {
			if profile.EffortInvalid || validateSubagentReasoningEffort(profile.Effort) != nil {
				return request, ErrReasoningEffortInvalid
			}
			profileMode := NormalizeProfileMode(firstNonEmptyAnyString(profile.Mode, "subagent"))
			if profileMode == "" {
				return request, fmt.Errorf("subagent profile %q has invalid mode %q", request.ProfileName, profile.Mode)
			}
			if profileMode == "primary" {
				return request, fmt.Errorf("subagent profile %q is primary-only and cannot be delegated", request.ProfileName)
			}
			if err := validateBuiltinProfilePrompt(request.ProfileName, request.Prompt); err != nil {
				return request, err
			}
			request.ProfileDisplayName = firstNonEmptyAnyString(profile.DisplayName, profile.Name, request.ProfileName)
			request.ProfileDescription = profile.Description
			request.ProfileMode = profileMode
			request.ProfileHidden = profile.Hidden
			request.ProfileColor = profile.Color
			request.ProfileIcon = profile.Icon
			if strings.TrimSpace(request.ProviderID) == "" && strings.TrimSpace(profile.ProviderID) != "" {
				request.ProviderID = profile.ProviderID
				request.ProfileExecutionConfigured = true
			}
			if strings.TrimSpace(request.Model) == "" && strings.TrimSpace(profile.Model) != "" {
				request.Model = profile.Model
				request.ProfileExecutionConfigured = true
			}
			if strings.TrimSpace(request.EndpointFormat) == "" && strings.TrimSpace(profile.EndpointFormat) != "" {
				request.EndpointFormat = profile.EndpointFormat
				request.ProfileExecutionConfigured = true
			}
			if strings.TrimSpace(request.Variant) == "" && strings.TrimSpace(profile.Variant) != "" {
				request.Variant = profile.Variant
				request.ProfileExecutionConfigured = true
			}
			if strings.TrimSpace(request.Effort) == "" {
				request.Effort = profile.Effort
			}
			if !request.ToolPolicySet && strings.TrimSpace(profile.ToolPolicy) != "" {
				request.ToolPolicy = profile.ToolPolicy
				request.ToolPolicySet = true
				request.ToolPolicySource = "profile"
			}
			if len(request.Tools) == 0 && len(profile.Tools) > 0 {
				request.Tools = append([]string(nil), profile.Tools...)
			}
			request.BlockedTools = UniqueStringList(append(request.BlockedTools, profile.BlockedTools...))
			request.BlockedMCPServers = UniqueStringList(append(request.BlockedMCPServers, profile.BlockedMCPServers...))
			request.BlockedSkills = UniqueStringList(append(request.BlockedSkills, profile.BlockedSkills...))
			if !request.MaxStepsSet && profile.MaxStepsSet {
				request.MaxSteps = profile.MaxSteps
				request.MaxStepsSet = true
			}
			if !request.TokenBudgetSet && profile.TokenBudgetSet {
				request.TokenBudget = profile.TokenBudget
				request.TokenBudgetSet = true
			}
			if !request.TimeBudgetMSSet && profile.TimeBudgetMSSet {
				request.TimeBudgetMS = profile.TimeBudgetMS
				request.TimeBudgetMSSet = true
			}
			if strings.TrimSpace(profile.SystemPrompt) != "" {
				request.SystemPrompt = profile.SystemPrompt
			}
			if strings.TrimSpace(profile.PromptPreamble) != "" {
				request.Prompt = strings.TrimSpace(profile.PromptPreamble) + "\n\n" + request.Prompt
			}
		} else if request.ProfileName != "inherit" {
			return request, fmt.Errorf("unknown subagent profile: %s", request.ProfileName)
		}
	}
	if !request.ToolPolicySet && strings.TrimSpace(settings.DefaultToolPolicy) != "" {
		request.ToolPolicy = settings.DefaultToolPolicy
		request.ToolPolicySet = true
		request.ToolPolicySource = "default"
	}
	if strings.TrimSpace(request.ToolPolicy) == "" {
		request.ToolPolicy = "readOnly"
		request.ToolPolicySource = "runtime-default"
	}
	if request.ToolPolicy == "readOnly" {
		if err := validateReadOnlyPrompt(request.Prompt); err != nil {
			return request, err
		}
	}
	request.Prompt = PromptWithReturnFormat(request.Prompt, request.ReturnFormat)
	return request, nil
}

func SystemPromptForRequest(request TaskRequest) string {
	if request.ForegroundHandoff {
		if request.ForegroundHandoffKind == ForegroundHandoffCaseTypedV1 {
			return ForegroundCaseTypedHandoffSystemPrompt
		}
		return ForegroundHandoffSystemPrompt
	}
	base := SystemPrompt
	extension := strings.TrimSpace(request.SystemPrompt)
	if extension == "" {
		return base
	}
	return base + "\n\nProfile system extension:\n" + extension
}

func PromptWithReturnFormat(prompt string, returnFormat string) string {
	switch NormalizeReturnFormat(returnFormat) {
	case "evidence":
		return strings.TrimSpace(prompt) + "\n\nReturn your normal concise answer, then include a fenced JSON object with an evidence array. Each evidence item should use kind verification, diff, files, or manual, include a summary, and include command or paths when applicable. Do not claim the parent goal is complete."
	case "transcriptRef":
		return strings.TrimSpace(prompt) + "\n\nReturn a concise answer and reference the child transcript id in the final answer if it is relevant. Do not include the full transcript."
	default:
		return prompt
	}
}

func ValidateSource(source domainjob.Record, request TaskRequest, spec SourceValidation) error {
	if domainmodel.ValidateReasoningEffortV1(source.Effort) != nil || domainmodel.ValidateReasoningEffortV1(spec.Effort) != nil {
		return errors.New("subagent reasoning effort authority is invalid")
	}
	if strings.TrimSpace(source.Kind) != "subagent" {
		return fmt.Errorf("subagent reference %q is %q, not a subagent transcript", source.ID, source.Kind)
	}
	if strings.TrimSpace(source.ParentThreadID) != "" && strings.TrimSpace(spec.ParentThreadID) != "" && source.ParentThreadID != spec.ParentThreadID && !spec.AllowAncestorSource {
		return fmt.Errorf("subagent reference %q belongs to parent thread %q, current parent thread is %q", source.ID, source.ParentThreadID, spec.ParentThreadID)
	}
	switch source.Status {
	case "running", "queued":
		return fmt.Errorf("subagent reference %q is still in progress", source.ID)
	case "failed", "interrupted", "aborted", "killed":
		return fmt.Errorf("%w: %q is %s", ErrSourceNotReplayable, source.ID, source.Status)
	}
	if strings.TrimSpace(source.ChildThreadID) == "" {
		return fmt.Errorf("subagent reference %q has no durable child transcript", source.ID)
	}
	if source.Workspace != "" && spec.Workspace != "" && source.Workspace != spec.Workspace {
		return fmt.Errorf("subagent reference %q belongs to workspace %q, current workspace is %q", source.ID, source.Workspace, spec.Workspace)
	}
	if source.ProviderID != "" && spec.ProviderID != "" && source.ProviderID != spec.ProviderID {
		return fmt.Errorf("subagent reference %q uses provider %q, current run would use %q", source.ID, source.ProviderID, spec.ProviderID)
	}
	if source.Model != "" && spec.Model != "" && source.Model != spec.Model {
		return fmt.Errorf("subagent reference %q uses model %q, current run would use %q", source.ID, source.Model, spec.Model)
	}
	if source.EndpointFormat != "" && spec.EndpointFormat != "" && source.EndpointFormat != spec.EndpointFormat {
		return fmt.Errorf("subagent reference %q uses endpoint format %q, current run would use %q", source.ID, source.EndpointFormat, spec.EndpointFormat)
	}
	if source.Variant != "" && spec.Variant != "" && source.Variant != spec.Variant {
		return fmt.Errorf("subagent reference %q uses variant %q, current run would use %q", source.ID, source.Variant, spec.Variant)
	}
	if source.Effort != "" && spec.Effort != "" && source.Effort != spec.Effort {
		return errors.New("subagent reasoning effort mismatch")
	}
	if source.ToolPolicy != "" && request.ToolPolicy != "" && source.ToolPolicy != request.ToolPolicy {
		return fmt.Errorf("subagent reference %q uses tool policy %q, current run would use %q", source.ID, source.ToolPolicy, request.ToolPolicy)
	}
	if !domainmodel.SameStringSlice(source.ToolScope, spec.ToolScope) {
		return fmt.Errorf("%w: %q", ErrSourceToolScopeMismatch, source.ID)
	}
	if source.ToolSchemaHash != "" && spec.ToolSchemaHash != "" && source.ToolSchemaHash != spec.ToolSchemaHash {
		return fmt.Errorf("subagent reference %q uses different tool schemas", source.ID)
	}
	if source.SystemPromptHash != "" && spec.SystemPromptHash != "" && source.SystemPromptHash != spec.SystemPromptHash {
		return fmt.Errorf("subagent reference %q uses a different subagent persona", source.ID)
	}
	if source.SkillPackageDigest != "" || spec.SkillPackageDigest != "" {
		if source.SkillPackageDigest != spec.SkillPackageDigest {
			return fmt.Errorf("subagent reference %q uses a different skill package snapshot", source.ID)
		}
	}
	return nil
}

func validateSubagentReasoningEffort(value string) error {
	canonical, valid := domainmodel.ProjectReasoningEffortV1(value)
	if !valid || canonical != value || canonical == "auto" {
		return ErrReasoningEffortInvalid
	}
	return nil
}

func ToolScope(requested []string, allowed []string) []string {
	if len(requested) == 0 {
		out := append([]string(nil), allowed...)
		sort.Strings(out)
		return out
	}
	allowedSet := map[string]bool{}
	for _, name := range allowed {
		allowedSet[name] = true
	}
	out := []string{}
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if allowedSet[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func ApprovalPolicy(toolPolicy string, parentPolicy string) string {
	if toolPolicy == "inherit" {
		if strings.TrimSpace(parentPolicy) != "" {
			return parentPolicy
		}
		return "on-request"
	}
	return "never"
}

func ProfileSource(request TaskRequest) string {
	if strings.TrimSpace(request.ProfileName) != "" {
		return "profile:" + strings.TrimSpace(request.ProfileName)
	}
	return "parent-default"
}

func ModelSource(explicitExecutionOverride, profileExecutionOverride, sourceExecutionInherited bool) string {
	switch {
	case explicitExecutionOverride:
		return "explicit-input"
	case profileExecutionOverride:
		return "subagent-profile"
	case sourceExecutionInherited:
		return "session"
	default:
		return "thread"
	}
}

func DefaultMaxSteps(parentMaxSteps *int) int {
	if parentMaxSteps == nil {
		return defaultSubagentMaxSteps
	}
	if *parentMaxSteps <= 0 {
		return defaultSubagentMaxSteps
	}
	maxSteps := *parentMaxSteps / 2
	if maxSteps < minimumInheritedSubagentMaxSteps {
		maxSteps = minimumInheritedSubagentMaxSteps
	}
	if maxSteps > *parentMaxSteps {
		maxSteps = *parentMaxSteps
	}
	return maxSteps
}

func ParentMaxModelSteps(maxModelSteps *int, effectiveMaxModelSteps *int) *int {
	if maxModelSteps != nil {
		return maxModelSteps
	}
	return effectiveMaxModelSteps
}

func ResolvedMaxSteps(request TaskRequest, parentMaxSteps *int) int {
	if request.MaxStepsSet {
		return request.MaxSteps
	}
	baseline := defaultMaxStepsForPrompt(request.Prompt)
	if parentMaxSteps == nil {
		return baseline
	}
	if *parentMaxSteps <= 0 {
		return baseline
	}
	maxSteps := *parentMaxSteps / 2
	if maxSteps < baseline {
		maxSteps = baseline
	}
	if maxSteps > *parentMaxSteps {
		maxSteps = *parentMaxSteps
	}
	if maxSteps < 1 {
		maxSteps = 1
	}
	return maxSteps
}

const (
	defaultSubagentMaxSteps          = 12
	complexSubagentMaxSteps          = 20
	minimumInheritedSubagentMaxSteps = 8
)

func defaultMaxStepsForPrompt(prompt string) int {
	normalized := normalizedPrompt(prompt)
	for _, marker := range []string{
		"scan", "audit", "investigate", "metadata", "report", "desktop", "directory", "workspace",
		"扫描", "审计", "排查", "调查", "元数据", "报告", "桌面", "目录", "工作区", "取证",
	} {
		if strings.Contains(normalized, marker) {
			return complexSubagentMaxSteps
		}
	}
	return defaultSubagentMaxSteps
}

func validateBuiltinProfilePrompt(profileName string, prompt string) error {
	switch strings.TrimSpace(profileName) {
	case "design-reviewer":
		if promptHasAny(prompt,
			"frontend", "front-end", "ui", "ux", "visual", "prototype", "accessibility", "spacing", "motion", "layout", "css", "react", "component", "screen",
			"前端", "界面", "视觉", "交互", "原型", "可访问", "布局", "间距", "动效", "组件", "页面",
		) {
			return nil
		}
		return fmt.Errorf("subagent profile %q is only for frontend/design review tasks; omit profile for general file analysis", profileName)
	case "over-engineering-reviewer":
		if promptHasAny(prompt,
			"code", "implementation", "refactor", "complexity", "abstraction", "dependency", "architecture",
			"代码", "实现", "重构", "复杂", "抽象", "依赖", "架构",
		) {
			return nil
		}
		return fmt.Errorf("subagent profile %q is only for code complexity review tasks; omit profile for general file analysis", profileName)
	default:
		return nil
	}
}

func validateReadOnlyPrompt(prompt string) error {
	if promptHasAny(prompt,
		"write file", "write a file", "save file", "save to", "create file", "edit file", "modify file",
		"写文件", "写入文件", "保存到", "创建文件", "修改文件", "编辑文件", "生成文件",
	) && !promptHasAny(prompt,
		"do not write file", "don't write file", "do not create file", "do not edit file", "do not modify file",
		"never edit file", "never edit files", "never modify file", "no file writes",
		"不要写文件", "不要写入文件", "不需要写文件", "无需写文件", "不要创建文件", "不要编辑文件", "不要修改文件", "不要改文件", "不修改文件", "只读取不修改",
	) {
		return fmt.Errorf("subagent task requires file write/edit capability but toolPolicy is readOnly; set toolPolicy to inherit, or keep the child read-only and have the parent perform writes")
	}
	if promptHasAny(prompt,
		"run python", "execute python", "bash", "shell command", "run command", "execute command", "file command", "mime type",
		"执行 python", "运行 python", "执行python", "运行python", "执行命令", "运行命令", "bash", "shell", "真实 mime", "真实MIME",
	) && !promptHasAny(prompt,
		"do not run python", "don't run python", "do not execute python", "do not use bash", "do not run command", "do not execute command",
		"不要运行 python", "不要执行 python", "不要运行python", "不要执行python", "不要使用 bash", "不要使用bash", "不要执行命令", "不要运行命令",
	) {
		return fmt.Errorf("subagent task requires shell/command capability but toolPolicy is readOnly; set toolPolicy to inherit, or remove the execution requirement")
	}
	return nil
}

func promptHasAny(prompt string, markers ...string) bool {
	normalized := normalizedPrompt(prompt)
	for _, marker := range markers {
		if strings.Contains(normalized, strings.ToLower(strings.TrimSpace(marker))) {
			return true
		}
	}
	return false
}

func normalizedPrompt(prompt string) string {
	return strings.ToLower(strings.Join(strings.Fields(prompt), " "))
}
