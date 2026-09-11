package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const (
	CurrentRelativeDir = ".analytixsdd/plan"
	LegacyRelativeDir  = ".analytix/plan"
)

type FallbackInput struct {
	Prompt    string
	GUIPlan   map[string]any
	Workspace string
}

type Target struct {
	WorkspaceRoot string
	RelativePath  string
	PlanID        string
	Operation     string
	SourceRequest string
	Title         string
}

func FallbackArgs(input FallbackInput, planText string) map[string]any {
	args := map[string]any{
		"markdown": strings.TrimSpace(planText),
	}
	if plan := EffectiveGUIPlan(input.GUIPlan, input.Workspace); len(plan) > 0 {
		args["operation"] = firstNonEmptyAnyString(plan["operation"], "draft")
		if planID := stringField(plan, "planId"); planID != "" {
			args["plan_id"] = planID
		}
		if relativePath := stringField(plan, "relativePath"); relativePath != "" {
			args["plan_relative_path"] = relativePath
		}
		if sourceRequest := firstNonEmptyAnyString(plan["sourceRequest"], input.Prompt); sourceRequest != "" {
			args["source_request"] = sourceRequest
		}
		if title := stringField(plan, "title"); title != "" {
			args["title"] = title
		}
		return args
	}
	args["operation"] = "draft"
	if sourceRequest := strings.TrimSpace(input.Prompt); sourceRequest != "" {
		args["source_request"] = sourceRequest
	}
	return args
}

func MaterializedCallID(contextDigest string) string {
	entropy := sha256.Sum256([]byte("analytix.materialized-plan-tool-call/v1\x00" + strings.TrimSpace(contextDigest)))
	identity, _ := domainmodel.NewHostToolCallIDV1(entropy[:])
	return identity
}

func ToolActive(mode string, guiPlan map[string]any, workspace string) bool {
	return ToolActiveForMode(mode) || len(EffectiveGUIPlan(guiPlan, workspace)) > 0
}

func ToolActiveForMode(mode string) bool {
	return strings.TrimSpace(mode) == "plan"
}

func EffectiveGUIPlan(guiPlan map[string]any, workspace string) map[string]any {
	if len(guiPlan) == 0 {
		return nil
	}
	expected := strings.TrimSpace(stringField(guiPlan, "workspaceRoot"))
	if expected == "" || WorkspaceMatches(workspace, expected) {
		return guiPlan
	}
	return nil
}

func ResolveTarget(args map[string]any, guiPlan map[string]any, workspace string, operation string, existingPlanPaths map[string]bool, now time.Time) (Target, error) {
	if len(guiPlan) > 0 {
		return ResolveReservedTarget(args, guiPlan, workspace, operation)
	}
	return ResolveFreeFormTarget(args, workspace, operation, existingPlanPaths, now)
}

func ResolveReservedTarget(args map[string]any, guiPlan map[string]any, workspace string, operation string) (Target, error) {
	if operation != strings.TrimSpace(stringField(guiPlan, "operation")) {
		return Target{}, errors.New("operation does not match the active GUI plan operation")
	}
	expectedWorkspace := strings.TrimSpace(stringField(guiPlan, "workspaceRoot"))
	if expectedWorkspace != "" && !WorkspaceMatches(workspace, expectedWorkspace) {
		return Target{}, errors.New("tool workspace does not match the active GUI plan workspace")
	}
	relativePath := NormalizeRelativePath(stringField(guiPlan, "relativePath"))
	if !IsGUIPlanRelativePath(relativePath) {
		return Target{}, errors.New("plan_relative_path must be a direct Markdown file under .analytixsdd/plan")
	}
	if operation == "draft" && !IsCurrentRelativePath(relativePath) {
		return Target{}, errors.New("legacy .analytix/plan paths can only be refined")
	}
	if suppliedPath := NormalizeRelativePath(firstNonEmptyAnyString(args["plan_relative_path"])); suppliedPath != "" && suppliedPath != relativePath {
		return Target{}, errors.New("plan_relative_path does not match the reserved GUI plan path")
	}
	planID := firstNonEmptyAnyString(args["plan_id"])
	contextPlanID := strings.TrimSpace(stringField(guiPlan, "planId"))
	if planID != "" && contextPlanID != "" && planID != contextPlanID {
		return Target{}, errors.New("plan_id does not match the reserved GUI plan id")
	}
	workspaceRoot := firstNonEmptyAnyString(expectedWorkspace, workspace)
	if workspaceRoot == "" {
		return Target{}, errors.New("workspace root is required")
	}
	if contextPlanID == "" {
		contextPlanID = BuildGUIPlanID(workspaceRoot, relativePath)
	}
	return Target{
		WorkspaceRoot: workspaceRoot,
		RelativePath:  relativePath,
		PlanID:        contextPlanID,
		Operation:     operation,
		SourceRequest: stringField(guiPlan, "sourceRequest"),
		Title:         stringField(guiPlan, "title"),
	}, nil
}

func ResolveFreeFormTarget(args map[string]any, workspace string, operation string, existingPlanPaths map[string]bool, now time.Time) (Target, error) {
	workspaceRoot := strings.TrimSpace(workspace)
	if workspaceRoot == "" {
		return Target{}, errors.New("workspace root is required")
	}
	relativePath := ""
	if suppliedPath := NormalizeRelativePath(firstNonEmptyAnyString(args["plan_relative_path"])); suppliedPath != "" {
		if !IsCurrentRelativePath(suppliedPath) {
			return Target{}, errors.New("plan_relative_path must be a direct Markdown file under .analytixsdd/plan")
		}
		relativePath = suppliedPath
	} else {
		feature := FeatureName(firstNonEmptyAnyString(args["title"], args["source_request"]))
		relativePath = NextAvailableRelativePath(feature, existingPlanPaths, now)
	}
	return Target{
		WorkspaceRoot: workspaceRoot,
		RelativePath:  relativePath,
		PlanID:        BuildGUIPlanID(workspaceRoot, relativePath),
		Operation:     operation,
		SourceRequest: firstNonEmptyAnyString(args["source_request"]),
		Title:         firstNonEmptyAnyString(args["title"]),
	}, nil
}

func NormalizeRelativePath(raw string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimRight(normalized, "/")
	for strings.Contains(normalized, "//") {
		normalized = strings.ReplaceAll(normalized, "//", "/")
	}
	return normalized
}

func IsGUIPlanRelativePath(value string) bool {
	normalized := strings.ToLower(NormalizeRelativePath(value))
	if !strings.HasSuffix(normalized, ".md") {
		return false
	}
	for _, dir := range []string{CurrentRelativeDir, LegacyRelativeDir} {
		prefix := strings.ToLower(dir) + "/"
		if !strings.HasPrefix(normalized, prefix) {
			continue
		}
		rest := strings.TrimPrefix(normalized, prefix)
		return rest != "" && !strings.Contains(rest, "/") && rest != "." && rest != ".." && !strings.Contains(rest, "..")
	}
	return false
}

func IsCurrentRelativePath(value string) bool {
	normalized := strings.ToLower(NormalizeRelativePath(value))
	if !strings.HasSuffix(normalized, ".md") || !strings.HasPrefix(normalized, CurrentRelativeDir+"/") {
		return false
	}
	rest := strings.TrimPrefix(normalized, CurrentRelativeDir+"/")
	return rest != "" && !strings.Contains(rest, "/") && rest != "." && rest != ".." && !strings.Contains(rest, "..")
}

func WorkspaceMatches(actual string, expected string) bool {
	normalize := func(value string) string {
		value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
		value = strings.TrimRight(value, "/")
		return strings.ToLower(value)
	}
	return normalize(actual) == normalize(expected)
}

func BuildGUIPlanID(workspaceRoot string, relativePath string) string {
	workspaceRoot = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(workspaceRoot), "\\", "/"), "/")
	relativePath = strings.ToLower(NormalizeRelativePath(relativePath))
	return workspaceRoot + ":" + relativePath
}

func FeatureName(seed string) string {
	seed = strings.ToLower(strings.TrimSpace(seed))
	if seed == "" {
		return "plan"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range seed {
		switch {
		case r < 32 || strings.ContainsRune(`<>:"|?*\/`, r):
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		case r == '_' || r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		default:
			builder.WriteRune(r)
			lastDash = false
		}
		if builder.Len() >= 96 {
			break
		}
	}
	value := strings.Trim(builder.String(), ".- ")
	if value == "" {
		return "plan"
	}
	return value
}

func NextAvailableRelativePath(feature string, existingPlanPaths map[string]bool, now time.Time) string {
	for attempt := 1; attempt <= 50; attempt++ {
		suffix := ""
		if attempt > 1 {
			suffix = fmt.Sprintf("-%d", attempt)
		}
		candidate := fmt.Sprintf("%s/%s%s.md", CurrentRelativeDir, feature, suffix)
		if !existingPlanPaths[candidate] {
			return candidate
		}
	}
	return fmt.Sprintf("%s/%s-%d.md", CurrentRelativeDir, feature, now.UTC().UnixMilli())
}

func SandboxBlock(sandboxMode string, absolutePath string) map[string]any {
	switch strings.TrimSpace(sandboxMode) {
	case "danger-full-access", "workspace-write":
		return nil
	case "read-only":
		return map[string]any{
			"code":  "sandbox_read_only",
			"error": "writing is blocked by the read-only sandbox: " + absolutePath,
		}
	case "external-sandbox":
		return map[string]any{
			"code":  "sandbox_write_blocked",
			"error": "writing is blocked because external-sandbox is not enforced by in-process file tools: " + absolutePath,
		}
	default:
		return map[string]any{
			"code":  "sandbox_write_blocked",
			"error": "writing is limited to the workspace sandbox: " + absolutePath,
		}
	}
}

func CanMaterializeInSandbox(sandboxMode string) bool {
	switch strings.TrimSpace(sandboxMode) {
	case "danger-full-access", "workspace-write":
		return true
	default:
		return false
	}
}

func ToolResponse(target Target, args map[string]any, markdown string, absolutePath string, savedAt string) map[string]any {
	sum := sha256.Sum256([]byte(markdown))
	action := "Created"
	if target.Operation == "refine" {
		action = "Refined"
	}
	return map[string]any{
		"summary":        fmt.Sprintf("%s GUI plan at %s.", action, target.RelativePath),
		"plan_id":        target.PlanID,
		"workspace_root": target.WorkspaceRoot,
		"relative_path":  target.RelativePath,
		"absolute_path":  absolutePath,
		"source_request": firstNonEmptyAnyString(args["source_request"], target.SourceRequest),
		"title":          firstNonEmptyAnyString(args["title"], target.Title),
		"operation":      target.Operation,
		"saved_at":       savedAt,
		"content_hash":   hex.EncodeToString(sum[:]),
		"byte_size":      float64(len([]byte(markdown))),
	}
}

func PlanModeToolAllowed(toolName string, _ int, createPlanToolName string) bool {
	if toolName == createPlanToolName {
		return true
	}
	switch toolName {
	case "read", "ls", "find", "glob", "code_index", "grep", "web_fetch", "user_input", "request_user_input", "get_goal", "todo_list":
		return true
	default:
		return false
	}
}

func FilterModeToolSchemas(tools []domainmodel.ToolSchema, planActive bool, _ bool, stepIndex int, createPlanToolName string) []domainmodel.ToolSchema {
	if !planActive {
		return tools
	}
	filtered := make([]domainmodel.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		if PlanModeToolAllowed(tool.Name, stepIndex, createPlanToolName) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case fmt.Stringer:
			if strings.TrimSpace(typed.String()) != "" {
				return strings.TrimSpace(typed.String())
			}
		}
	}
	return ""
}
