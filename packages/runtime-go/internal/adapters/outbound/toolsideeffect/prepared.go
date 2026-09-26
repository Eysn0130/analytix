package toolsideeffect

import (
	"context"
	"errors"
	"strings"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appgoal "analytix.local/runtime-go/internal/app/goal"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	sideeffectidentityapp "analytix.local/runtime-go/internal/app/sideeffectidentity"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
)

type Dependencies struct {
	GoalService      appgoal.ToolService
	ResolveSkill     sideeffectidentityapp.SkillResolver
	ResolveSkillBody sideeffectidentityapp.SkillBodyResolver
	ResolveTask      sideeffectidentityapp.TaskProjectionResolver
	MutationInput    func(context.Context, appmodel.PendingToolCall, map[string]any) filestore.MutationToolInput
	ResolveMCP       sideeffectidentityapp.MCPProjectionResolver
	HostBinding      any
}

type preparedContextKey struct{}

type preparedValues struct {
	issuedAt     time.Time
	plan         *filestore.PreparedCreatePlanTool
	completeStep *appgoal.PreparedCompleteStep
	updateGoal   *appgoal.PreparedUpdateGoal
	todoOps      *appgoal.PreparedTodoOps
	notebook     *filestore.PreparedNotebookEditTool
	deleteSymbol *filestore.PreparedDeleteSymbolTool
}

func Prepare(ctx context.Context, pending appmodel.PendingToolCall, issuedAt time.Time, dependencies Dependencies) (apploop.PreparedSideEffect, error) {
	values := preparedValues{issuedAt: issuedAt.UTC()}
	var rejection *apploop.ToolDispatchOverride
	reject := func(output any) error {
		rejection = &apploop.ToolDispatchOverride{Output: output, IsError: true}
		return errors.New("host side-effect owner rejected the request")
	}
	if toolcatalogapp.MCPToolServerID(pending.ExecutionGrant.ToolName) != "" && dependencies.ResolveMCP == nil {
		return apploop.PreparedSideEffect{Rejection: &apploop.ToolDispatchOverride{
			Output: toolFailure("side_effect_identity_unavailable", "mutating MCP tool lacks a host-authoritative semantic identity contract"), IsError: true,
		}}, nil
	}
	resolvePath := func(workspaceRealPath string, requestedPath string) (string, bool) {
		resolved, ok := filestore.ResolveMutationIdentityPath(workspaceRealPath, requestedPath)
		if !ok && rejection == nil {
			rejection = &apploop.ToolDispatchOverride{Output: toolFailure("workspace_escape", "side-effect path is outside host-authorized workspace identity"), IsError: true}
		}
		return resolved, ok
	}
	goalPending := appgoal.PendingToolContext{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ToolCallID: pending.Call.ID}
	identity, err := sideeffectidentityapp.ResolveV1(sideeffectidentityapp.Input{
		ToolName: pending.ExecutionGrant.ToolName, Arguments: pending.Call.Arguments,
		WorkspaceRealPath: pending.SecurityContext.WorkspaceRealPath, ResolvePath: resolvePath,
		ResolveSkill: dependencies.ResolveSkill, ResolveSkillBody: dependencies.ResolveSkillBody, ResolveMCP: dependencies.ResolveMCP,
		HostBinding: dependencies.HostBinding,
		ResolveTask: func(request subagentapp.TaskRequest) (any, error) {
			if dependencies.ResolveTask == nil {
				return nil, errors.New("subagent semantic owner is unavailable")
			}
			projection, err := dependencies.ResolveTask(request)
			if err != nil {
				return nil, reject(toolFailure("validation_error", err.Error()))
			}
			return projection, nil
		},
		ResolveCompleteStep: func(arguments map[string]any) (any, error) {
			prepared, err := dependencies.GoalService.PrepareCompleteStep(goalPending, arguments)
			if err != nil {
				return nil, reject(appgoal.PreparationFailure(err).Output)
			}
			values.completeStep = &prepared
			return prepared.SemanticProjectionV1(), nil
		},
		ResolveUpdateGoal: func(arguments map[string]any) (any, error) {
			prepared, err := dependencies.GoalService.PrepareUpdateGoal(goalPending, arguments)
			if err != nil {
				return nil, reject(appgoal.PreparationFailure(err).Output)
			}
			values.updateGoal = &prepared
			return prepared.SemanticProjectionV1(), nil
		},
		ResolveTodoOps: func(arguments map[string]any) (any, error) {
			prepared, err := dependencies.GoalService.PrepareTodoOps(pending.ThreadID, arguments)
			if err != nil {
				return nil, reject(appgoal.PreparationFailure(err).Output)
			}
			values.todoOps = &prepared
			return prepared.SemanticProjectionV1(), nil
		},
		ResolveNotebook: func(arguments map[string]any) (any, error) {
			if dependencies.MutationInput == nil {
				return nil, errors.New("notebook_edit semantic owner is unavailable")
			}
			prepared, output, failed := filestore.PrepareNotebookEditTool(dependencies.MutationInput(ctx, pending, arguments))
			if failed {
				return nil, reject(output)
			}
			projection, err := filestore.PreparedNotebookEditSemanticEffectProjection(prepared)
			if err == nil {
				values.notebook = &prepared
			}
			return projection, err
		},
		ResolveDeleteSymbol: func(arguments map[string]any) (any, error) {
			if dependencies.MutationInput == nil {
				return nil, errors.New("delete_symbol semantic owner is unavailable")
			}
			prepared, output, failed := filestore.PrepareDeleteSymbolTool(dependencies.MutationInput(ctx, pending, arguments))
			if failed {
				return nil, reject(output)
			}
			projection, err := filestore.PreparedDeleteSymbolSemanticEffectProjection(prepared)
			if err == nil {
				values.deleteSymbol = &prepared
			}
			return projection, err
		},
		ResolvePlan: func(arguments map[string]any) (any, error) {
			prepared, output, err := preparePlan(pending, arguments, issuedAt)
			if err != nil {
				return nil, reject(output)
			}
			values.plan = &prepared
			return filestore.PreparedCreatePlanSemanticEffectProjection(prepared)
		},
	})
	if err != nil {
		if rejection != nil {
			return apploop.PreparedSideEffect{Rejection: rejection}, nil
		}
		if strings.TrimSpace(pending.ExecutionGrant.ToolName) == "todo_write" {
			return apploop.PreparedSideEffect{Rejection: &apploop.ToolDispatchOverride{
				Output: toolFailure("todo_write_failed", err.Error()), IsError: true,
			}}, nil
		}
		return apploop.PreparedSideEffect{}, executiongrantapp.ValidationError{Code: "side_effect_identity_invalid"}
	}
	return apploop.PreparedSideEffect{
		SemanticIdentity: identity,
		Bind: func(bindCtx context.Context) context.Context {
			return context.WithValue(bindCtx, preparedContextKey{}, values)
		},
	}, nil
}

func IssuedAt(ctx context.Context) time.Time {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return values.issuedAt
}

func Plan(ctx context.Context) (filestore.PreparedCreatePlanTool, bool) {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return pointerValue(values.plan)
}

func CompleteStep(ctx context.Context) (appgoal.PreparedCompleteStep, bool) {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return pointerValue(values.completeStep)
}

func UpdateGoal(ctx context.Context) (appgoal.PreparedUpdateGoal, bool) {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return pointerValue(values.updateGoal)
}

func TodoOps(ctx context.Context) (appgoal.PreparedTodoOps, bool) {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return pointerValue(values.todoOps)
}

func Notebook(ctx context.Context) (filestore.PreparedNotebookEditTool, bool) {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return pointerValue(values.notebook)
}

func DeleteSymbol(ctx context.Context) (filestore.PreparedDeleteSymbolTool, bool) {
	values, _ := ctx.Value(preparedContextKey{}).(preparedValues)
	return pointerValue(values.deleteSymbol)
}

func preparePlan(pending appmodel.PendingToolCall, arguments map[string]any, issuedAt time.Time) (filestore.PreparedCreatePlanTool, any, error) {
	markdown, _ := arguments["markdown"].(string)
	ordinaryResult, blocked, err := appturn.CompileOrdinaryResultSlot(
		pending.SecurityContext,
		strings.TrimSpace(markdown),
	)
	if err != nil || blocked {
		output := toolFailure("plan_content_requires_case_evidence", "plan content requires host publication authority")
		return filestore.PreparedCreatePlanTool{}, output, errors.New("plan content requires host publication authority")
	}
	guarded := cloneMap(arguments)
	guarded["markdown"] = ordinaryResult.Text
	prepared, output, failed := filestore.PrepareCreatePlanTool(filestore.CreatePlanToolInput{
		Workspace: pending.Workspace, Mode: pending.Mode, GUIPlan: pending.GUIPlan,
		SandboxMode: pending.SandboxMode, Args: guarded, Now: func() time.Time { return issuedAt.UTC() },
	})
	if failed {
		return filestore.PreparedCreatePlanTool{}, output, errors.New("create_plan owner rejected the request")
	}
	return prepared, nil, nil
}

func toolFailure(code string, message string) map[string]any {
	return map[string]any{"code": strings.TrimSpace(code), "error": strings.TrimSpace(message), "executed": false}
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func pointerValue[T any](value *T) (T, bool) {
	if value == nil {
		var zero T
		return zero, false
	}
	return *value, true
}
