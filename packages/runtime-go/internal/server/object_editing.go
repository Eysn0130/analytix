package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	packagehostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

// This adapter authorizes a protected-local scope against current Core thread
// authority. Public thread projection and caller-supplied owner fields are not
// authorization. Raw text and private file paths never enter ordinary events.
type runtimeObjectProjector struct{ handler *runtimeServerHandler }

func (p runtimeObjectProjector) ValidateCurrent(ctx context.Context, scope editingapp.ScopeAuthority) error {
	return p.validateSelectionAuthority(ctx, scope, nil)
}

func (p runtimeObjectProjector) FreezeSelectionAuthority(ctx context.Context, scope editingapp.ScopeAuthority) (string, error) {
	// The caller cannot choose a fingerprint to be made current.
	scope.SecurityBinding = ""
	var frozen string
	err := p.validateSelectionAuthority(ctx, scope, &frozen)
	return frozen, err
}

func (p runtimeObjectProjector) validateSelectionAuthority(ctx context.Context, scope editingapp.ScopeAuthority, frozen *string) error {
	h := p.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.turnSecurity.Identity == nil || h.turnSecurity.Observer == nil ||
		(scope.Purpose != "discuss" && scope.Purpose != "edit") ||
		h.turnSecurity.Identity.ValidateCurrent(ctx, scope.Principal) != nil {
		return editingapp.ErrProjection
	}
	// Object sessions retain filesystem-semantic identities; thread security
	// records retain their original canonical spelling. Compare through the real
	// filesystem without changing either identity or persisted security context.
	sameWorkspace := func(left, right string) bool {
		leftID, leftOK := filestore.ResolveMutationIdentityPath(left, left)
		rightID, rightOK := filestore.ResolveMutationIdentityPath(right, right)
		return leftOK && rightOK && leftID == rightID
	}
	observation, err := h.turnSecurity.Observer.Observe(scope.Workspace)
	if err != nil || !sameWorkspace(scope.Workspace, observation.WorkspaceRealPath) {
		return editingapp.ErrProjection
	}
	thread, err := h.store.GetThread(scope.ThreadID)
	if err != nil || stringField(thread, "id") != scope.ThreadID {
		return editingapp.ErrProjection
	}
	threadObservation, err := h.turnSecurity.Observer.Observe(stringField(thread, "workspace"))
	if err != nil || !sameWorkspace(stringField(thread, "workspace"), threadObservation.WorkspaceRealPath) ||
		!sameWorkspace(observation.WorkspaceRealPath, threadObservation.WorkspaceRealPath) {
		return editingapp.ErrProjection
	}
	workspace := threadObservation.WorkspaceRealPath
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, scope.ThreadID, workspace, scope.Principal.TenantID, scope.Principal.UserID)
	if err != nil {
		return editingapp.ErrProjection
	}
	defer release()
	// A transition may have completed before the gate was acquired. Re-read both
	// observations and identities under it; never switch to another workspace.
	thread, err = h.store.GetThread(scope.ThreadID)
	if err != nil || stringField(thread, "id") != scope.ThreadID {
		return editingapp.ErrProjection
	}
	threadObservation, err = h.turnSecurity.Observer.Observe(stringField(thread, "workspace"))
	if err != nil || threadObservation.WorkspaceRealPath != workspace ||
		!sameWorkspace(stringField(thread, "workspace"), threadObservation.WorkspaceRealPath) {
		return editingapp.ErrProjection
	}
	observation, err = h.turnSecurity.Observer.Observe(scope.Workspace)
	if err != nil || !sameWorkspace(scope.Workspace, observation.WorkspaceRealPath) ||
		!sameWorkspace(observation.WorkspaceRealPath, threadObservation.WorkspaceRealPath) {
		return editingapp.ErrProjection
	}
	scope.Workspace = workspace
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
	if frozen != nil || scope.SecurityBinding != "" {
		binding, err := selectionAuthorityFingerprint(scope, securityContext)
		if err != nil || scope.SecurityBinding != "" && scope.SecurityBinding != binding {
			return editingapp.ErrProjection
		}
		if frozen != nil {
			*frozen = binding
		}
	}
	if h.turnSecurity.Identity.ValidateCurrent(ctx, scope.Principal) != nil || ctx.Err() != nil {
		return editingapp.ErrProjection
	}
	return nil
}

// A fingerprint invalidates an old selection; it never validates authority.
// The caller first revalidates identity, case/epoch, publication and the actual
// current risk witness under the existing transition read gate. Fresh witness
// challenge replies may change ObservationDigest without changing the enrolled
// index/checkpoint. Bind that stable head, not per-read challenge entropy.
func selectionAuthorityFingerprint(scope editingapp.ScopeAuthority, current domainsecurity.TurnSecurityContext) (string, error) {
	risk := current.RiskAuthorityBinding
	observation := risk.ObservationDigest
	if risk.State == domainsecurity.RiskAuthorityBindingStateWitnessed {
		observation = ""
	}
	stable, err := json.Marshal(struct {
		Version                          int
		Thread, Workspace, Object, Path  string
		Principal                        domainidentity.PrincipalV1
		Case, Binding, Dataset, Manifest string
		Epoch                            uint64
		Publication                      domainsecurity.TurnPublicationPolicyV1
		Risk                             any
	}{current.Version, scope.ThreadID, scope.Workspace, scope.ObjectID, scope.Path,
		scope.Principal, current.CaseID,
		current.CaseBindingHash, current.DatasetSnapshotID, current.SourceManifestHash,
		current.ContextEpoch, current.PublicationPolicy, struct {
			Version                                                                                                     int
			Purpose, State, Index, Checkpoint, Observation, ThreadPolicy, GeneralPolicy, HostPolicy, BindingObservation string
			Generation                                                                                                  uint64
		}{risk.SchemaVersion, risk.Purpose, risk.State, risk.IndexDigest, risk.CheckpointDigest,
			observation, risk.ThreadPolicyDigest, risk.GeneralPolicyDigest, risk.HostPolicyDigest,
			risk.BindingObservationDigest, risk.Generation}})
	if err != nil {
		return "", editingapp.ErrProjection
	}
	sum := sha256.Sum256(stable)
	return hex.EncodeToString(sum[:]), nil
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
	for key := range args {
		if key != "scopeId" && (pending.Call.Name != "native_selection_propose" || key != "operationId" && key != "parts" && key != "workbook" && key != "canvas" && key != "presentation") {
			return failure()
		}
	}
	if pending.Call.Name == "native_selection_propose" {
		payloads := 0
		for _, key := range []string{"parts", "workbook", "canvas", "presentation"} {
			if _, ok := args[key]; ok {
				payloads++
			}
		}
		if payloads != 1 {
			return failure()
		}
	}
	operation := "model-selection-read"
	input := map[string]any{"scopeId": scopeID, "threadId": pending.ThreadID}
	if pending.Call.Name == "native_selection_propose" {
		operation = "model-selection-propose"
		input["operationId"] = args["operationId"]
		if value, ok := args["canvas"]; ok {
			input["canvas"] = value
		} else if value, ok := args["workbook"]; ok {
			input["workbook"] = value
		} else if value, ok := args["presentation"]; ok {
			input["presentation"] = value
		} else {
			input["parts"] = args["parts"]
		}
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
		if !pkg.Available || !slices.Contains(pkg.Operations, operation) {
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
