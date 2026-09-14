package sideeffectidentity

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	generationapp "analytix.local/runtime-go/internal/app/documentgeneration"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	goalapp "analytix.local/runtime-go/internal/app/goal"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	terminalapp "analytix.local/runtime-go/internal/app/terminal"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
)

const SchemaVersionV1 = domainsideeffectidentity.SchemaVersionV1

type IdentityV1 = domainsideeffectidentity.IdentityV1

// PathResolver is the host path-identity port. Production supplies the same
// lexical resolver used before filesystem mutation; it deliberately does not
// equate a symlink with its target because mutation uses no-follow semantics.
type PathResolver func(workspaceRealPath string, requestedPath string) (string, bool)

type SkillResolver func(name string) (map[string]any, bool)

type SkillBodyResolver func(skill map[string]any) (string, error)

type PlanProjectionResolver func(arguments map[string]any) (any, error)

type OwnerProjectionResolver func(arguments map[string]any) (any, error)

type MCPProjectionResolver func(toolName string, arguments json.RawMessage) (any, error)

type TaskProjectionResolver func(request subagentapp.TaskRequest) (any, error)

type Input struct {
	ToolName            string
	Arguments           json.RawMessage
	WorkspaceRealPath   string
	ResolvePath         PathResolver
	ResolveSkill        SkillResolver
	ResolveSkillBody    SkillBodyResolver
	ResolvePlan         PlanProjectionResolver
	ResolveCompleteStep OwnerProjectionResolver
	ResolveUpdateGoal   OwnerProjectionResolver
	ResolveTodoOps      OwnerProjectionResolver
	ResolveNotebook     OwnerProjectionResolver
	ResolveDeleteSymbol OwnerProjectionResolver
	ResolveMCP          MCPProjectionResolver
	ResolveTask         TaskProjectionResolver
	// HostBinding is supplied by trusted composition for installed plugin tools.
	// It is never read from model arguments or a filesystem skill manifest.
	HostBinding any
}

// ResolveV1 derives a versioned semantic identity from the owner parser used
// by each host side-effect tool. Unknown host tools fail closed. Remote MCP
// tools retain exact strict canonical-JSON semantics because only the remote
// owner can define additional aliases or defaults.
func ResolveV1(input Input) (IdentityV1, error) {
	toolName := canonicalToolName(input.ToolName)
	if toolName == "" {
		return IdentityV1{}, errors.New("side-effect tool name is required")
	}
	var (
		projection any
		err        error
	)
	if toolcatalogapp.MCPToolServerID(toolName) != "" {
		if input.ResolveMCP == nil {
			return IdentityV1{}, errors.New("mutating MCP tool lacks a host-authoritative semantic identity contract")
		}
		projection, err = input.ResolveMCP(toolName, input.Arguments)
		if err == nil {
			if _, ok := projection.(map[string]any); !ok {
				err = errors.New("MCP side-effect projection must be an object")
			}
		}
	} else {
		var record map[string]any
		record, err = domainsecurity.DecodeCanonicalJSONObject(input.Arguments)
		if err != nil {
			return IdentityV1{}, err
		}
		projection, err = hostProjection(toolName, record, input)
	}
	if err != nil {
		return IdentityV1{}, err
	}
	envelope := struct {
		SchemaVersion int    `json:"schemaVersion"`
		ToolName      string `json:"toolName"`
		Arguments     any    `json:"arguments"`
		HostBinding   any    `json:"hostBinding,omitempty"`
	}{SchemaVersionV1, toolName, projection, input.HostBinding}
	body, err := json.Marshal(envelope)
	if err != nil {
		return IdentityV1{}, err
	}
	argsHash := domainsecurity.CanonicalJSONHash(body)
	if argsHash == "" {
		return IdentityV1{}, errors.New("side-effect semantic projection is invalid")
	}
	return IdentityV1{SchemaVersion: SchemaVersionV1, ToolName: toolName, ArgsHash: argsHash}, nil
}

func SupportsHostToolV1(toolName string) bool {
	switch canonicalToolName(toolName) {
	case "bash", "write_file", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol", "generate_office_document",
		"task", "parallel_tasks", "run_skill", "kill_shell", "restart_job", "create_goal", "complete_step", "update_goal",
		"todo_write", "todo_ops", toolcatalogapp.ToolCreatePlanName:
		return true
	default:
		return false
	}
}

func hostProjection(toolName string, record map[string]any, input Input) (any, error) {
	switch toolName {
	case "bash":
		workspace, err := resolvePath(input, ".")
		if err != nil {
			return nil, err
		}
		request, failure, failed := terminalapp.BashToolRequestFromArgs(
			record,
			workspace,
			true,
			"danger-full-access",
			terminalapp.DefaultBashTimeoutSeconds,
			terminalapp.MaxBashTimeoutSeconds,
		)
		if failed {
			return nil, fmt.Errorf("bash semantic request is invalid: %v", failure["error"])
		}
		return map[string]any{
			"command": request.Command, "timeoutSeconds": request.TimeoutSeconds,
			"runInBackground": boolAny(record["run_in_background"], record["runInBackground"]),
		}, nil
	case "generate_office_document":
		request, err := generationapp.ParseRequest(record)
		if err != nil {
			return nil, err
		}
		path, err := resolvePath(input, request.Path)
		if err != nil {
			return nil, err
		}
		images := make([]map[string]any, 0, len(request.Images))
		for _, image := range request.Images {
			images = append(images, map[string]any{"id": image.ID, "type": image.Type, "encodedContentHash": domainsecurity.SHA256Hex([]byte(image.DataBase64))})
		}
		projection := map[string]any{"path": path, "kind": request.Kind, "title": request.Title}
		switch request.Kind {
		case "docx":
			projection["markdown"], projection["images"] = request.Markdown, images
		case "xlsx":
			projection["workbook"] = request.Workbook
		case "pptx":
			projection["presentation"], projection["images"] = request.Presentation, images
		}
		return projection, nil
	case "write_file":
		request, err := filetoolsapp.ParseWriteToolRequest(record)
		if err != nil {
			return nil, err
		}
		path, err := resolvePath(input, request.Path)
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": path, "content": request.Content}, nil
	case "edit_file", "multi_edit":
		path, err := selectedResolvedPath(input, record, "path", "filePath", "FilePath")
		if err != nil {
			return nil, err
		}
		edits, err := filetoolsapp.ParseEditInstructions(record)
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": path, "edits": edits}, nil
	case "move_file":
		source, err := selectedResolvedPath(input, record, "source_path", "sourcePath", "source", "from", "path")
		if err != nil {
			return nil, err
		}
		destination, err := selectedResolvedPath(input, record, "destination_path", "destinationPath", "destination", "to")
		if err != nil {
			return nil, err
		}
		return map[string]any{"sourcePath": source, "destinationPath": destination}, nil
	case "notebook_edit":
		if input.ResolveNotebook != nil {
			return input.ResolveNotebook(record)
		}
		return nil, errors.New("notebook_edit semantic owner is unavailable")
	case "delete_range":
		path, err := selectedResolvedPath(input, record, "path", "filePath", "FilePath")
		if err != nil {
			return nil, err
		}
		start := firstNonEmptyExactString(record["start_anchor"], record["startAnchor"])
		end := firstNonEmptyExactString(record["end_anchor"], record["endAnchor"])
		if strings.TrimSpace(start) == "" || strings.TrimSpace(end) == "" {
			return nil, errors.New("delete_range requires start_anchor and end_anchor")
		}
		inclusive := true
		if value, ok := record["inclusive"].(bool); ok {
			inclusive = value
		}
		return map[string]any{"path": path, "startAnchor": start, "endAnchor": end, "inclusive": inclusive}, nil
	case "delete_symbol":
		if input.ResolveDeleteSymbol != nil {
			return input.ResolveDeleteSymbol(record)
		}
		return nil, errors.New("delete_symbol semantic owner is unavailable")
	case "task":
		request, err := subagentapp.TaskRequestFromArgs("task", record)
		if err != nil {
			return nil, err
		}
		return taskProjection(request, input)
	case "parallel_tasks":
		requests, err := subagentapp.ParallelTaskRequestsFromArgs(record)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(requests))
		for _, request := range requests {
			projection, err := taskProjection(request.Request, input)
			if err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"index": request.Index, "id": request.ID, "request": projection})
		}
		return map[string]any{"tasks": out}, nil
	case "run_skill":
		name := subagentapp.SkillNameFromArgs(record)
		if strings.TrimSpace(name) == "" || input.ResolveSkill == nil || input.ResolveSkillBody == nil {
			return nil, errors.New("run_skill requires name")
		}
		skill, ok := input.ResolveSkill(name)
		if !ok {
			return nil, errors.New("run_skill target is unavailable")
		}
		body, err := input.ResolveSkillBody(skill)
		if err != nil {
			return nil, err
		}
		runAs := subagentapp.SkillRunAs(skill)
		if runAs == "subagent" {
			prompt := toolcatalogapp.SkillSubagentPrompt(skill, body, subagentapp.SkillArguments(record))
			request, err := subagentapp.SkillTaskRequestFromArgs(subagentapp.SkillTaskRequestInput{
				Skill: skill, Args: record, Prompt: prompt,
			})
			if err != nil {
				return nil, err
			}
			projection, err := taskProjection(request, input)
			if err != nil {
				return nil, err
			}
			return map[string]any{"runAs": "subagent", "request": projection}, nil
		}
		if subagentapp.SkillContinueOrForkRequested(record) {
			return nil, errors.New("continue_from/fork_from require a subagent skill")
		}
		if runAs == "" {
			runAs = "inline"
		}
		return map[string]any{
			"runAs": runAs, "skillId": firstTrimmedString(skill["id"]),
			"packageDigest": firstTrimmedString(skill["packageDigest"]), "entry": firstTrimmedString(skill["entry"]),
			"arguments": strings.TrimSpace(subagentapp.SkillArguments(record)),
		}, nil
	case "kill_shell":
		request, failure, failed := subagentapp.TaskJobKillRequestFromArgs(record)
		if failed {
			return nil, fmt.Errorf("kill_shell semantic request is invalid: %v", failure["error"])
		}
		return request, nil
	case "restart_job":
		request, failure, failed := subagentapp.TaskJobRestartRequestFromArgs(record)
		if failed {
			return nil, fmt.Errorf("restart_job semantic request is invalid: %v", failure["error"])
		}
		return request, nil
	case "create_goal":
		return createGoalProjection(record)
	case "complete_step":
		if input.ResolveCompleteStep == nil {
			return nil, errors.New("complete_step semantic owner is unavailable")
		}
		return input.ResolveCompleteStep(record)
	case "update_goal":
		if input.ResolveUpdateGoal == nil {
			return nil, errors.New("update_goal semantic owner is unavailable")
		}
		return input.ResolveUpdateGoal(record)
	case "todo_write":
		items, ok := record["todos"].([]any)
		if !ok {
			return nil, errors.New("todo_write requires todos")
		}
		normalized, err := threadapp.NormalizeTodos("semantic", items, "semantic")
		if err != nil {
			return nil, err
		}
		return map[string]any{"items": normalized["items"]}, nil
	case "todo_ops":
		if input.ResolveTodoOps == nil {
			return nil, errors.New("todo_ops semantic owner is unavailable")
		}
		return input.ResolveTodoOps(record)
	case toolcatalogapp.ToolCreatePlanName:
		if input.ResolvePlan == nil {
			return nil, errors.New("create_plan semantic owner is unavailable")
		}
		return input.ResolvePlan(record)
	default:
		return nil, fmt.Errorf("host side-effect tool %q has no semantic canonicalizer", toolName)
	}
}

func taskProjection(request subagentapp.TaskRequest, input Input) (map[string]any, error) {
	if input.ResolveTask != nil {
		projection, err := input.ResolveTask(request)
		if err != nil {
			return nil, err
		}
		resolved, ok := projection.(map[string]any)
		if !ok || resolved == nil {
			return nil, errors.New("subagent side-effect projection must be an object")
		}
		return resolved, nil
	}
	workspace := strings.TrimSpace(request.Workspace)
	if workspace == "" {
		workspace = strings.TrimSpace(input.WorkspaceRealPath)
	} else {
		resolved, err := resolvePath(input, workspace)
		if err != nil {
			return nil, err
		}
		workspace = resolved
	}
	return map[string]any{
		"id": request.ID, "dependsOn": request.DependsOn, "name": request.Name, "prompt": request.Prompt, "label": request.Label,
		"nameExplicit": request.NameExplicit, "labelExplicit": request.LabelExplicit, "workspace": workspace,
		"providerId": request.ProviderID, "model": request.Model, "endpointFormat": request.EndpointFormat, "variant": request.Variant,
		"effort": request.Effort, "profileName": request.ProfileName, "modelExplicit": request.ModelExplicit,
		"providerExplicit": request.ProviderExplicit, "endpointExplicit": request.EndpointExplicit, "variantExplicit": request.VariantExplicit,
		"toolPolicy": request.ToolPolicy, "toolPolicySet": request.ToolPolicySet, "tools": request.Tools,
		"blockedTools": request.BlockedTools, "blockedMcpServers": request.BlockedMCPServers, "blockedSkills": request.BlockedSkills,
		"maxSteps": request.MaxSteps, "maxStepsSet": request.MaxStepsSet, "tokenBudget": request.TokenBudget,
		"tokenBudgetSet": request.TokenBudgetSet, "timeBudgetMs": request.TimeBudgetMS, "timeBudgetMsSet": request.TimeBudgetMSSet,
		"returnFormat": request.ReturnFormat, "runInBackground": request.RunInBackground,
		"autoContinueParent": request.AutoContinueParent, "isolationMode": request.IsolationMode,
		"continueFrom": request.ContinueFrom, "forkFrom": request.ForkFrom,
	}, nil
}

func createGoalProjection(record map[string]any) (any, error) {
	objective := firstTrimmedString(record["objective"])
	if objective == "" {
		return nil, errors.New("create_goal requires objective")
	}
	projection := map[string]any{
		"objective": objective, "strictCompletion": boolAny(record["strict_completion"], record["strictCompletion"]),
	}
	budget, present, err := goalapp.NormalizeTokenBudget(firstNonNil(record["token_budget"], record["tokenBudget"]))
	if err != nil {
		return nil, err
	}
	if present {
		projection["tokenBudget"] = budget
	}
	return projection, nil
}

func completeStepProjection(record map[string]any) (any, error) {
	step := firstTrimmedString(record["step"])
	stepIndex, _ := integerValue(firstNonNil(record["step_index"], record["stepIndex"]))
	if step == "" && stepIndex > 0 {
		step = strconv.Itoa(stepIndex)
	}
	if step == "" {
		return nil, errors.New("complete_step requires step or step_index")
	}
	evidence, err := goalapp.NormalizeEvidenceItemsV1(record["evidence"])
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"step": step, "stepIndex": stepIndex, "evidence": evidence,
		"summary":       firstTrimmedString(record["summary"], record["result"], record["notes"]),
		"requirementId": firstTrimmedString(record["requirement_id"], record["requirementId"]),
		"selfCheck":     boolAny(record["self_check"], record["selfCheck"]),
	}, nil
}

func updateGoalProjection(record map[string]any) (any, error) {
	status := firstTrimmedString(record["status"])
	if status != "complete" && status != "blocked" {
		return nil, errors.New("update_goal status is invalid")
	}
	projection := map[string]any{"status": status, "phase": "test-owner"}
	if status == "blocked" {
		_, reasonKey, err := goalapp.CanonicalBlockedReasonV1(firstTrimmedString(record["reason"]))
		if err != nil {
			return nil, err
		}
		projection["reason"] = reasonKey
	}
	return projection, nil
}

func notebookProjection(input Input, record map[string]any) (any, error) {
	request, err := filetoolsapp.ParseNotebookEditRequest(record)
	if err != nil {
		return nil, err
	}
	path, err := resolvePath(input, request.Path)
	if err != nil {
		return nil, err
	}
	projection := map[string]any{"path": path, "editMode": request.EditMode}
	if request.CellID != "" {
		projection["cellId"] = request.CellID
	} else {
		projection["cellIndex"] = optionalInt(request.CellIndex)
	}
	if request.EditMode != "delete" {
		projection["newSource"] = request.NewSource
		projection["cellType"] = request.CellType
	}
	return projection, nil
}

func deleteSymbolProjection(input Input, record map[string]any) (any, error) {
	path, err := selectedResolvedPath(input, record, "path", "filePath", "FilePath")
	if err != nil {
		return nil, err
	}
	name := firstTrimmedString(record["name"], record["symbol"], record["symbol_name"], record["symbolName"])
	if name == "" {
		return nil, errors.New("delete_symbol requires name")
	}
	return map[string]any{
		"path": path, "name": name, "kind": firstTrimmedString(record["kind"]), "parent": firstTrimmedString(record["parent"]),
	}, nil
}

func todoOpsProjection(record map[string]any) (any, error) {
	raw, ok := firstNonNil(record["ops"], record["operations"]).([]any)
	if !ok || len(raw) == 0 {
		return nil, errors.New("todo_ops requires ops")
	}
	normalized, err := threadapp.NormalizeTodoOpsForSemanticIdentityV1(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ops": normalized}, nil
}

func canonicalToolName(toolName string) string {
	return domainsideeffectidentity.CanonicalToolNameV1(toolName)
}

func selectedResolvedPath(input Input, record map[string]any, keys ...string) (string, error) {
	path := firstTrimmedValues(record, keys...)
	if path == "" {
		return "", errors.New("side-effect path is required")
	}
	return resolvePath(input, path)
}

func resolvePath(input Input, requestedPath string) (string, error) {
	if input.ResolvePath == nil {
		return "", errors.New("side-effect path resolver is unavailable")
	}
	resolved, ok := input.ResolvePath(strings.TrimSpace(input.WorkspaceRealPath), requestedPath)
	if !ok || strings.TrimSpace(resolved) == "" {
		return "", errors.New("side-effect path identity is invalid")
	}
	return resolved, nil
}

func firstTrimmedValues(record map[string]any, keys ...string) string {
	values := make([]any, 0, len(keys))
	for _, key := range keys {
		values = append(values, record[key])
	}
	return firstTrimmedString(values...)
}

func firstTrimmedString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok {
			if text = strings.TrimSpace(text); text != "" {
				return text
			}
		}
	}
	return ""
}

func firstNonEmptyExactString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func boolAny(values ...any) bool {
	for _, value := range values {
		if typed, ok := value.(bool); ok && typed {
			return true
		}
	}
	return false
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func integerValue(value any) (int, bool) {
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

func optionalInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func trimmedStringList(value any) []string {
	raw, _ := value.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text := firstTrimmedString(item); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}
