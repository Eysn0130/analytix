package filestore

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appplan "analytix.local/runtime-go/internal/app/plan"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type CreatePlanToolInput struct {
	Workspace   string
	Mode        string
	GUIPlan     map[string]any
	SandboxMode string
	Args        map[string]any
	Now         func() time.Time
}

type resolvedCreatePlanTool struct {
	args         map[string]any
	markdown     string
	target       appplan.Target
	absolutePath string
	autoWrite    *preparedAtomicFileWrite
}

// PreparedCreatePlanTool is a process-local one-shot owner value. It binds the
// exact resolved path, content, and observed before-state used by admission;
// none of these private fields is serializable or provider-controlled.
type PreparedCreatePlanTool struct {
	resolved   resolvedCreatePlanTool
	write      preparedAtomicFileWrite
	identity   map[string]any
	savedAtNow func() time.Time
}

func ExecuteCreatePlanTool(input CreatePlanToolInput) (any, bool) {
	prepared, output, failed := PrepareCreatePlanTool(input)
	if failed {
		return output, true
	}
	return ExecutePreparedCreatePlanTool(prepared)
}

func ExecutePreparedCreatePlanTool(prepared PreparedCreatePlanTool) (any, bool) {
	if prepared.savedAtNow == nil || strings.TrimSpace(prepared.resolved.absolutePath) == "" {
		return map[string]any{"code": "write_failed", "error": "prepared create_plan authority is invalid"}, true
	}
	if err := executePreparedAtomicFileWrite(prepared.write); err != nil {
		return map[string]any{"code": "write_failed", "error": err.Error()}, true
	}
	savedAt := prepared.savedAtNow().UTC().Format(time.RFC3339Nano)
	return appplan.ToolResponse(prepared.resolved.target, prepared.resolved.args, prepared.resolved.markdown, prepared.resolved.absolutePath, savedAt), false
}

// PrepareCreatePlanTool performs the one owner resolution used by both
// semantic admission and physical execution.
func PrepareCreatePlanTool(input CreatePlanToolInput) (PreparedCreatePlanTool, map[string]any, bool) {
	resolved, output, failed := resolveCreatePlanTool(input)
	if failed {
		return PreparedCreatePlanTool{}, output, true
	}
	identityPath, ok := ResolveMutationIdentityPath(input.Workspace, resolved.absolutePath)
	if !ok {
		return PreparedCreatePlanTool{}, map[string]any{"code": "write_failed", "error": "create_plan target identity is unavailable"}, true
	}
	write := preparedAtomicFileWrite{}
	if resolved.autoWrite != nil {
		write = *resolved.autoWrite
	} else {
		var err error
		write, err = prepareAtomicFileWrite(resolved.absolutePath, []byte(resolved.markdown))
		if err != nil {
			return PreparedCreatePlanTool{}, map[string]any{"code": "write_failed", "error": err.Error()}, true
		}
	}
	now := input.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return PreparedCreatePlanTool{
		resolved: resolved, write: write, savedAtNow: now,
		identity: createPlanSemanticEffectProjection(resolved, identityPath),
	}, nil, false
}

// CreatePlanSemanticEffectProjection resolves the exact target and bytes used
// by ExecuteCreatePlanTool. The runtime binds the same issued-at value across
// admission and execution; durable intent CAS arbitrates concurrent identity.
func CreatePlanSemanticEffectProjection(input CreatePlanToolInput) (map[string]any, error) {
	prepared, output, failed := PrepareCreatePlanTool(input)
	if failed {
		return nil, errors.New(firstNonEmptyAnyString(output["error"], output["code"], "create_plan semantic request is invalid"))
	}
	return PreparedCreatePlanSemanticEffectProjection(prepared)
}

func PreparedCreatePlanSemanticEffectProjection(prepared PreparedCreatePlanTool) (map[string]any, error) {
	_, contentOK := prepared.identity["content"].(string)
	target, targetOK := prepared.identity["target"].(string)
	if !contentOK || !targetOK || strings.TrimSpace(target) == "" || strings.TrimSpace(prepared.resolved.absolutePath) == "" {
		return nil, errors.New("prepared create_plan semantic authority is invalid")
	}
	projection := make(map[string]any, len(prepared.identity))
	for key, value := range prepared.identity {
		projection[key] = value
	}
	return projection, nil
}

// createPlanSemanticEffectProjection binds the stable logical target selected
// by the owner, rather than the next currently available filename. Otherwise
// an identical retry after the first write would resolve "feature-2.md" and
// evade the durable semantic side-effect receipt.
func createPlanSemanticEffectProjection(resolved resolvedCreatePlanTool, identityPath string) map[string]any {
	projection := map[string]any{
		"content": resolved.markdown,
		"target":  identityPath,
	}
	// Auto, explicit, and GUI-reserved request shapes that resolve to the same
	// canonical target are the same physical effect. Their route kind is audit
	// metadata, not durable side-effect identity.
	return projection
}

func resolveCreatePlanTool(input CreatePlanToolInput) (resolvedCreatePlanTool, map[string]any, bool) {
	args := input.Args
	if args == nil {
		args = map[string]any{}
	}
	effectivePlan := appplan.EffectiveGUIPlan(input.GUIPlan, input.Workspace)
	if !appplan.ToolActiveForMode(input.Mode) && len(effectivePlan) == 0 {
		return resolvedCreatePlanTool{}, map[string]any{"error": "create_plan requires Plan mode or an active GUI plan context"}, true
	}
	markdown := firstNonEmptyAnyString(args["markdown"])
	if strings.TrimSpace(markdown) == "" {
		return resolvedCreatePlanTool{}, map[string]any{"error": "markdown is required and must be non-empty"}, true
	}
	operation := strings.TrimSpace(firstNonEmptyAnyString(args["operation"]))
	if operation != "draft" && operation != "refine" {
		return resolvedCreatePlanTool{}, map[string]any{"error": `operation must be "draft" or "refine"`}, true
	}

	ownerArgs := args
	var autoWrite *preparedAtomicFileWrite
	if len(effectivePlan) == 0 && appplan.NormalizeRelativePath(firstNonEmptyAnyString(args["plan_relative_path"])) == "" {
		relativePath, preparedWrite, selectErr := stableAutoPlanRelativePath(
			input.Workspace,
			appplan.FeatureName(firstNonEmptyAnyString(args["title"], args["source_request"])),
			markdown,
		)
		if selectErr != nil {
			return resolvedCreatePlanTool{}, map[string]any{"code": "write_failed", "error": selectErr.Error()}, true
		}
		ownerArgs = make(map[string]any, len(args)+1)
		for key, value := range args {
			ownerArgs[key] = value
		}
		ownerArgs["plan_relative_path"] = relativePath
		autoWrite = &preparedWrite
	}
	target, err := appplan.ResolveTarget(ownerArgs, effectivePlan, input.Workspace, operation, ExistingMarkdownRelativePaths(input.Workspace, appplan.CurrentRelativeDir), input.now().UTC())
	if err != nil {
		return resolvedCreatePlanTool{}, map[string]any{"error": err.Error()}, true
	}
	if err := domaintoolresult.ValidatePlanTargetV1(target.PlanID, target.RelativePath); err != nil {
		return resolvedCreatePlanTool{}, map[string]any{"code": "validation_error", "error": err.Error()}, true
	}
	absolutePath, ok := ResolveWorkspaceRelativePath(target.WorkspaceRoot, target.RelativePath)
	if !ok {
		return resolvedCreatePlanTool{}, map[string]any{"code": "sandbox_write_blocked", "error": "plan write escaped the configured workspace root"}, true
	}
	if block := appplan.SandboxBlock(controlapp.NormalizeSandboxMode(input.SandboxMode), absolutePath); block != nil {
		return resolvedCreatePlanTool{}, block, true
	}
	if autoWrite != nil && autoWrite.path != absolutePath {
		return resolvedCreatePlanTool{}, map[string]any{"code": "write_failed", "error": "create_plan automatic target changed during owner resolution"}, true
	}
	return resolvedCreatePlanTool{args: args, markdown: markdown, target: target, absolutePath: absolutePath, autoWrite: autoWrite}, nil, false
}

// stableAutoPlanRelativePath preserves the established base/-2/-3 naming but
// makes a repeated request resolve to the file that already contains its exact
// bytes. Existence is checked through the same no-follow filesystem owner used
// for mutation, so case and Unicode aliases follow the host filesystem rather
// than a case-sensitive directory-entry map.
func stableAutoPlanRelativePath(workspace string, feature string, markdown string) (string, preparedAtomicFileWrite, error) {
	content := []byte(markdown)
	for attempt := 1; attempt <= 50; attempt++ {
		suffix := ""
		if attempt > 1 {
			suffix = fmt.Sprintf("-%d", attempt)
		}
		candidate := fmt.Sprintf("%s/%s%s.md", appplan.CurrentRelativeDir, feature, suffix)
		available, same, prepared, err := inspectAutoPlanCandidate(workspace, candidate, content)
		if err != nil {
			return "", preparedAtomicFileWrite{}, err
		}
		if available || same {
			return candidate, prepared, nil
		}
	}
	sum := sha256.Sum256(content)
	candidate := fmt.Sprintf("%s/%s-%x.md", appplan.CurrentRelativeDir, feature, sum[:16])
	available, same, prepared, err := inspectAutoPlanCandidate(workspace, candidate, content)
	if err != nil {
		return "", preparedAtomicFileWrite{}, err
	}
	if !available && !same {
		return "", preparedAtomicFileWrite{}, errors.New("create_plan deterministic fallback path is occupied by different content")
	}
	return candidate, prepared, nil
}

func inspectAutoPlanCandidate(workspace string, relativePath string, content []byte) (available bool, same bool, prepared preparedAtomicFileWrite, err error) {
	absolutePath, ok := ResolveWorkspaceRelativePath(workspace, relativePath)
	if !ok {
		return false, false, preparedAtomicFileWrite{}, errors.New("create_plan automatic target is unavailable")
	}
	state, err := inspectAtomicTextTarget(absolutePath, true)
	if err != nil {
		return false, false, preparedAtomicFileWrite{}, err
	}
	expectedHash := ""
	if state.Exists {
		expectedHash = digestAtomicText(state.Content)
	}
	prepared = preparedAtomicFileWrite{
		path: absolutePath, content: append([]byte(nil), content...),
		expectedExists: state.Exists, expectedHash: expectedHash,
	}
	if !state.Exists {
		return true, false, prepared, nil
	}
	return false, equalAtomicTextBytes(state.Content, content), prepared, nil
}

func (input CreatePlanToolInput) now() time.Time {
	if input.Now != nil {
		return input.Now()
	}
	return time.Now().UTC()
}
