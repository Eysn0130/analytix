package server

import (
	packagehostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	"context"
	"encoding/json"
	"reflect"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

// This adapter authorizes a protected-local scope against current Core thread
// authority. Public thread projection and caller-supplied owner fields are not
// authorization. Raw text and private file paths never enter ordinary events.
type runtimeObjectProjector struct{ handler *runtimeServerHandler }

func (p runtimeObjectProjector) ValidateCurrent(ctx context.Context, scope editingapp.ScopeAuthority) error {
	h := p.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.turnSecurity.Identity == nil ||
		(scope.Purpose != "discuss" && scope.Purpose != "edit") ||
		h.turnSecurity.Identity.ValidateCurrent(ctx, scope.Principal) != nil {
		return editingapp.ErrProjection
	}
	workspace, err := (filestore.CaseBindingReader{}).WorkspaceRealPath(scope.Workspace)
	if err != nil || workspace != scope.Workspace {
		return editingapp.ErrProjection
	}
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, scope.ThreadID, workspace, scope.Principal.TenantID, scope.Principal.UserID)
	if err != nil {
		return editingapp.ErrProjection
	}
	defer release()
	thread, err := h.store.GetThread(scope.ThreadID)
	if err != nil || stringField(thread, "id") != scope.ThreadID {
		return editingapp.ErrProjection
	}
	threadWorkspace, err := (filestore.CaseBindingReader{}).WorkspaceRealPath(stringField(thread, "workspace"))
	if err != nil || threadWorkspace != workspace {
		return editingapp.ErrProjection
	}
	securityContext, err := p.selectionSecurityContext(ctx, thread, scope)
	if err != nil || securityContext.ThreadID != scope.ThreadID || securityContext.WorkspaceRealPath != workspace ||
		securityContext.TenantID != scope.Principal.TenantID || securityContext.UserID != scope.Principal.UserID {
		return editingapp.ErrProjection
	}
	input := turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity,
		Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority,
		SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2,
		Context: securityContext, Workspace: workspace}
	if turnsecurityapp.ValidateCurrentOrdinaryEffect(input) != nil {
		return editingapp.ErrProjection
	}
	if !domainsecurity.TurnSecurityContextIsGeneral(securityContext) {
		committed, ok := h.caseThreads.(casethreadapp.CommittedAuthority)
		if !ok || !committed.CanExecute(scope.ThreadID) {
			return editingapp.ErrProjection
		}
		current, found := committed.CommittedContext(scope.ThreadID, securityContext.TurnID)
		epoch, epochErr := domaincontextepoch.ParseState(thread["contextEpochState"])
		if !found || epochErr != nil || current.SecurityContext != securityContext || !reflect.DeepEqual(current.EpochState, epoch) {
			return editingapp.ErrProjection
		}
	}
	if h.turnSecurity.Identity.ValidateCurrent(ctx, scope.Principal) != nil {
		return editingapp.ErrProjection
	}
	return nil
}

// A newly created primary thread has no turn security record until its first
// send. Freeze its selection authority through the same host V2 authority used
// by turn start, while holding the caller's transition read gate. This does not
// create a turn or persist securityState; normal turn start owns those records.
// Missing authority on an existing, forked, or case thread is never repaired here.
func (p runtimeObjectProjector) selectionSecurityContext(ctx context.Context, thread map[string]any, scope editingapp.ScopeAuthority) (domainsecurity.TurnSecurityContext, error) {
	if record, exists := thread["securityState"]; exists {
		return domainsecurity.ParseTurnSecurityContext(record)
	}
	turns, validTurns := thread["turns"].([]any)
	if !validTurns || len(turns) != 0 || stringField(thread, "relation") != "primary" || stringField(thread, "status") != "idle" ||
		(p.handler.caseThreads != nil && p.handler.caseThreads.IsCaseThread(scope.ThreadID)) {
		return domainsecurity.TurnSecurityContext{}, editingapp.ErrProjection
	}
	// BuildThread emits no kind or ancestry/security metadata for a fresh main
	// thread. Presence (even null) distinguishes an unsupported/recovered record.
	for _, key := range []string{"kind", "parentThreadId", "forkedFromThreadId", "historyAuthority", "contextEpochState", "caseId", "caseProjectId", "caseBindingHash"} {
		if _, exists := thread[key]; exists {
			return domainsecurity.TurnSecurityContext{}, editingapp.ErrProjection
		}
	}
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: p.handler.turnSecurity, Thread: thread,
		ThreadID: scope.ThreadID, TurnID: "selection_" + scope.ThreadID,
		Workspace: scope.Workspace, Principal: scope.Principal, IssuedAt: time.Now().UTC(),
	})
	if err != nil || !domainsecurity.TurnSecurityContextIsGeneral(frozen) {
		return domainsecurity.TurnSecurityContext{}, editingapp.ErrProjection
	}
	return frozen, nil
}

func (p runtimeObjectProjector) AuthorizeAndProject(ctx context.Context, input editingapp.ProjectionInput) ([]editingapp.ProtectedRange, error) {
	if err := p.ValidateCurrent(ctx, input.ScopeAuthority); err != nil {
		return nil, err
	}
	ranges, err := editingapp.ProtectedSelectionRanges(input.Text)
	if err != nil {
		return nil, err
	}
	if err := p.ValidateCurrent(ctx, input.ScopeAuthority); err != nil {
		return nil, err
	}
	return ranges, nil
}

func (h *runtimeServerHandler) checkManagedMutation(paths ...string) error {
	if h.managedEditing == nil {
		return nil
	} // Legacy isolated component fixtures.
	return h.managedEditing.CheckMutation(paths...)
}

func (h *runtimeServerHandler) beginOpaqueEditingExecution(ctx context.Context) error {
	if h.managedEditing == nil {
		return nil
	}
	return h.managedEditing.BeginOpaque(ctx)
}

// The same runner serves foreground, background and resumed shell jobs. The
// observation precedes actual execution and deliberately survives RunShell's
// return: waiting for a direct child is not a proof that all descendants exited.
type managedEditingShellRunner struct {
	next     ports.ShellRunner
	registry *managededitingapp.Registry
}

func (r managedEditingShellRunner) RunShell(ctx context.Context, request ports.ShellRequest) ports.ShellResult {
	if r.next == nil || r.registry.BeginOpaque(ctx) != nil {
		return ports.ShellResult{ExitCode: -1, StartFailed: true, Error: "Shell execution is unavailable while a controlled editing session is active."}
	}
	return r.next.RunShell(ctx, request)
}

// Native selection tools reuse the running turn's provider and thread. Current
// plugin generation and activation are checked by the same Host as Main calls.
// Raw selected bytes and approved replacements never enter this tool route.
func (h *runtimeServerHandler) executeNativeSelectionTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	failure := func() (any, bool) {
		return map[string]any{"code": "tool_failed", "error": "The native selection is unavailable or stale."}, true
	}
	if h.officePackageHost == nil {
		return failure()
	}
	scopeID, ok := args["scopeId"].(string)
	if !ok {
		return failure()
	}
	operation := "model-selection-read"
	input := map[string]any{"scopeId": scopeID, "threadId": pending.ThreadID}
	if pending.Call.Name == "native_selection_propose" {
		operation = "model-selection-propose"
		input["operationId"] = args["operationId"]
		input["parts"] = args["parts"]
	}
	body, err := json.Marshal(input)
	if err != nil {
		return failure()
	}
	packages, err := h.officePackageHost.List(ctx)
	if err != nil {
		return failure()
	}
	for _, pkg := range packages {
		if !pkg.Available {
			continue
		}
		result, err := h.officePackageHost.Invoke(ctx, packagehostapp.InvokeRequest{PackageID: pkg.PackageID, GenerationID: pkg.GenerationID, ExpectedRevision: pkg.ActivationRevision, ContributionID: "workspace-editor", Operation: operation, Input: body})
		if err != nil {
			continue
		}
		var response struct {
			OK     bool           `json:"ok"`
			Result map[string]any `json:"result"`
			Code   string         `json:"code"`
		}
		if json.Unmarshal(result.Output, &response) != nil {
			continue
		}
		if response.OK && response.Result != nil {
			return response.Result, false
		}
		if response.Code != "scope_invalid" {
			return failure()
		}
	}
	return failure()
}
