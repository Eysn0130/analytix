package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	controlapp "analytix.local/runtime-go/internal/app/control"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	"analytix.local/runtime-go/internal/jobs"
)

var errHostChildInputConflictV1 = errors.New("Host child input conflicts with its current child scope")
var errHostChildInputExistingV1 = errors.New("Host child input already has a primary admission")
var errHostChildInputCancelledV1 = errors.New("Host child input was cancelled")

type hostChildInputContextKeyV1 struct{}
type hostChildInputCarrierV1 struct {
	host           *runtimeServerHandler
	request        subagentapp.ChildInputRequestV1
	source         domainjob.Record
	current        domainjob.Record
	child          map[string]any
	startRequest   controlapp.StartTurnRequest
	taskRequest    subagentapp.TaskRequest
	turn           *childTurnReservationV1
	job            jobs.ChildRunReservationV1
	created        domainjob.Record
	preparation    subagentapp.TaskRunPreparation
	foreground     *controlapp.ForegroundReservation
	startBarrier   *subagentapp.BackgroundJobStartBarrier
	at             time.Time
	writeAttempted bool
	appendError    error
	cleanup        func()
	transferred    bool
}

func hostChildInputCarrierFromContextV1(ctx context.Context, h *runtimeServerHandler) (*hostChildInputCarrierV1, error) {
	if ctx == nil || ctx.Value(hostChildInputContextKeyV1{}) == nil {
		return nil, nil
	}
	c, ok := ctx.Value(hostChildInputContextKeyV1{}).(*hostChildInputCarrierV1)
	if !ok || c == nil || c.host != h || subagentapp.ValidateChildInputRequestV1(c.request) != nil || c.request.Mode != subagentapp.ChildInputStartOrSteerV1 {
		return nil, errors.New("Host child input provenance is invalid")
	}
	return c, nil
}

func childInputResultV1(caps subagentapp.ChildInputCapabilitiesV1, receipt *subagentapp.ChildInputReceiptV1, code, errorCode string) subagentapp.TaskJobServiceResult {
	response := map[string]any{"schemaVersion": 1, "capabilities": caps}
	if receipt != nil {
		response["receipt"] = receipt
	}
	if code != "" {
		response["code"] = code
	}
	return subagentapp.TaskJobServiceResult{Response: response, IsError: errorCode != "", ErrorCode: errorCode}
}

func disabledChildInputV1(code, reason, errorCode string) subagentapp.TaskJobServiceResult {
	return childInputResultV1(subagentapp.ChildInputCapabilitiesV1{Send: "disabled", ReasonCode: reason}, nil, code, errorCode)
}

// readHostChildScopeV1 verifies durable ownership with the current Host
// principal. A historical parent grant is lineage only and is never reused.
func (h *runtimeServerHandler) readHostChildScopeV1(ctx context.Context, r subagentapp.ChildInputRequestV1) (domainjob.Record, domainjob.Record, map[string]any, error) {
	if h == nil || h.store == nil || h.jobs == nil || ctx == nil || ctx.Err() != nil {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child scope unavailable")
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return domainjob.Record{}, domainjob.Record{}, nil, err
	}
	source, err := h.jobs.LoadChildRun(r.JobID)
	if err != nil || source.Kind != "subagent" || source.ParentThreadID != r.ParentThreadID || source.ChildThreadID == "" || source.ChildTurnID == "" {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child lineage unavailable")
	}
	parent, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, r.ParentThreadID)
	if err != nil || stringField(parent.Thread, "status") == "deleted" {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("parent unavailable")
	}
	child, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, source.ChildThreadID)
	if err != nil || stringField(child.Thread, "parentThreadId") != r.ParentThreadID || stringField(child.Thread, "relation") != "side" || stringField(child.Thread, "status") == "deleted" {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child relationship unavailable")
	}
	frozen, freezeErr := turnapp.FrozenSecurityContextForTurn(child.Thread, source.ChildTurnID)
	if freezeErr != nil {
		if domainjob.ValidateSecurityBinding(source.SecurityBinding) != nil {
			return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child owner unavailable")
		}
		frozen, freezeErr = turnapp.FrozenSecurityContextForTurn(parent.Thread, source.ParentTurnID)
	}
	if freezeErr != nil || frozen.TenantID != principal.TenantID || frozen.UserID != principal.UserID || h.turnSecurity.Observer == nil {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child principal unavailable")
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(child.Thread, "workspace"))
	if err != nil || observation.WorkspaceRealPath != frozen.WorkspaceRealPath || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child workspace unavailable")
	}
	current := source
	turns := listAny(child.Thread["turns"])
	latestID := source.ChildTurnID
	if len(turns) > 0 {
		latest, ok := turns[len(turns)-1].(map[string]any)
		if !ok {
			return domainjob.Record{}, domainjob.Record{}, nil, errHostChildInputConflictV1
		}
		latestID = stringField(latest, "id")
	}
	count := 0
	for _, record := range h.jobs.AllRecords() {
		if record.ChildThreadID == source.ChildThreadID && record.ChildTurnID == latestID {
			if record.ParentThreadID != r.ParentThreadID {
				return domainjob.Record{}, domainjob.Record{}, nil, errHostChildInputConflictV1
			}
			current, count = record, count+1
		}
	}
	if count > 1 || (count == 0 && latestID != source.ChildTurnID) {
		return domainjob.Record{}, domainjob.Record{}, nil, errHostChildInputConflictV1
	}
	if turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, principal) != nil {
		return domainjob.Record{}, domainjob.Record{}, nil, errors.New("child principal changed")
	}
	return source, current, child.Thread, nil
}

func (h *runtimeServerHandler) hostChildInputCapabilitiesV1(ctx context.Context, current domainjob.Record, child map[string]any) subagentapp.ChildInputCapabilitiesV1 {
	caps := subagentapp.ChildInputCapabilitiesV1{Send: "disabled", RunID: current.ID, ChildThreadID: current.ChildThreadID, ChildTurnID: current.ChildTurnID}
	if h.runtimeControl().ShuttingDown() {
		caps.ReasonCode = "runtime_shutting_down"
		return caps
	}
	for _, id := range h.runtimeSubagentState().ActiveBackgroundJobIDs() {
		if id == current.ID {
			caps.Stop = true
		}
	}
	if current.HostChildInput != nil && !h.runtimeSubagentState().BackgroundJobStartedV1(current.ID) {
		caps.Stop = false
	}
	turns := listAny(child["turns"])
	var latest map[string]any
	if len(turns) > 0 {
		latest, _ = turns[len(turns)-1].(map[string]any)
	}
	if !h.subagents.Enabled {
		caps.ReasonCode = "authority_unavailable"
		return caps
	}
	if !subagentapp.TaskJobTerminal(current) {
		if current.Status == "running" && stringField(latest, "status") == "running" && caps.Stop {
			frozen, err := turnapp.FrozenSecurityContextForTurn(child, current.ChildTurnID)
			if err != nil || turnsecurityapp.ValidateCurrentForEffect(turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: frozen, Workspace: frozen.WorkspaceRealPath}, false) != nil {
				caps.ReasonCode = "context_changed"
			} else {
				caps.Send = "steer"
			}
		} else if current.Status == "running" && turnapp.IsTerminalStatus(stringField(latest, "status")) {
			caps.ReasonCode = "turn_terminalizing"
		} else if current.Status == "paused" || current.Status == "pause_requested" || current.Status == "resuming" {
			caps.ReasonCode = "paused"
		} else {
			caps.ReasonCode = "pending_start"
		}
		return caps
	}
	if caps.Stop {
		caps.ReasonCode = "turn_terminalizing"
		return caps
	}
	// Cold interrupted runs may leave a running primary. The ordinary Host
	// start barrier, not this projection, must quiesce that exact old turn.
	if len(turns) == 0 || scheduleIdleV1(child) || stringField(latest, "status") == "running" && !caps.Stop {
		caps.Send = "start"
	} else {
		caps.ReasonCode = "run_unavailable"
	}
	return caps
}

// Only the existing authenticated private Host transport enters this method.
// Model task_steer never obtains this carrier or selects a start operation.
func (h *runtimeServerHandler) admitHostChildInputV1(ctx context.Context, r subagentapp.ChildInputRequestV1) subagentapp.TaskJobServiceResult {
	if h == nil || !httpapi.HostChildInputHTTPAuthorizedV1(ctx, h.runtimeToken) {
		return disabledChildInputV1("forbidden", "authority_unavailable", subagentapp.TaskJobErrorForbidden)
	}
	if subagentapp.ValidateChildInputRequestV1(r) != nil {
		return disabledChildInputV1("validation_error", "authority_unavailable", subagentapp.TaskJobErrorValidation)
	}
	source, current, child, err := h.readHostChildScopeV1(ctx, r)
	if err != nil {
		return disabledChildInputV1("child_input_unknown", "authority_unavailable", "child_input_unknown")
	}
	caps := h.hostChildInputCapabilitiesV1(ctx, current, child)
	if r.Mode == subagentapp.ChildInputInspectV1 {
		return childInputResultV1(caps, nil, "", "")
	}
	receipt, _, err := h.store.hostChildInputReceiptFromPrimaryV1(child, r)
	if errors.Is(err, errHostChildInputCancelledV1) {
		return childInputResultV1(caps, nil, "input_cancelled", subagentapp.TaskJobErrorConflict)
	}
	if errors.Is(err, errHostChildInputConflictV1) {
		return childInputResultV1(caps, nil, "conflict", subagentapp.TaskJobErrorConflict)
	}
	if err != nil {
		return childInputResultV1(caps, nil, "child_input_unknown", "child_input_unknown")
	}
	if receipt != nil {
		return h.confirmHostChildInputReceiptV1(ctx, r, caps, receipt)
	}
	// A retained identity whose primary cannot confirm admission must not be
	// redispatched, even after a crash or after the previous run became idle.
	for _, record := range h.jobs.AllRecords() {
		if record.ChildThreadID != source.ChildThreadID || record.HostChildInput == nil || record.HostChildInput.ClientMessageID != r.ClientMessageID {
			continue
		}
		if record.HostChildInput.InputDigest != subagentapp.ChildInputDigestV1(r) || record.ContinueFrom != r.JobID {
			return childInputResultV1(caps, nil, "conflict", subagentapp.TaskJobErrorConflict)
		}
		return childInputResultV1(caps, nil, "child_input_unknown", "child_input_unknown")
	}
	if current.ID != r.ExpectedRunID || current.ChildTurnID != r.ExpectedChildTurnID {
		return childInputResultV1(caps, nil, "conflict", subagentapp.TaskJobErrorConflict)
	}
	if caps.Send == "disabled" {
		if caps.ReasonCode == "context_changed" {
			return childInputResultV1(caps, nil, "new_turn_required", subagentapp.TaskJobErrorConflict)
		}
		return childInputResultV1(caps, nil, "conflict", subagentapp.TaskJobErrorConflict)
	}
	c := &hostChildInputCarrierV1{host: h, request: r, source: source, current: current, child: child, at: time.Now().UTC()}
	carrierCtx := context.WithValue(ctx, hostChildInputContextKeyV1{}, c)
	defer func() {
		if !c.transferred {
			h.finishHostChildInputRunV1(c)
			if c.created.ID != "" {
				if record, loadErr := h.jobs.LoadChildRun(c.created.ID); loadErr == nil && !subagentapp.TaskJobTerminal(record) {
					_, _ = h.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: "interrupted", FailureCode: domainjob.FailureChildInterrupted})
				}
			}
			if c.cleanup != nil {
				c.cleanup()
			}
		}
	}()
	if caps.Send == "steer" {
		action, steerErr := h.runtimeControl().SteerTurn(carrierCtx, controlapp.SteerTurnRequest{ThreadID: current.ChildThreadID, TurnID: current.ChildTurnID, ExpectedTurnID: current.ChildTurnID, Text: r.Message, ClientUserMessageID: r.ClientMessageID})
		err = steerErr
		if action.StatusCode == http.StatusConflict && stringField(action.Body, "code") == "new_turn_required" {
			return childInputResultV1(caps, nil, "new_turn_required", subagentapp.TaskJobErrorConflict)
		}
	} else {
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = h.runtimeControl().WaitForThreadTurns(waitCtx, current.ChildThreadID)
		cancel()
		if err == nil {
			err = h.prepareHostChildInputStartV1(ctx, c)
		}
		if err == nil {
			_, err = h.runtimeControl().SendTurn(carrierCtx, c.startRequest)
		}
	}
	if !c.transferred {
		h.finishHostChildInputRunV1(c)
	}
	// Read actual primary after every dispatch result, including sidecar/ACK
	// errors. No error path dispatches a second execution.
	_, freshCurrent, freshChild, readErr := h.readHostChildScopeV1(ctx, r)
	if readErr != nil {
		return childInputResultV1(caps, nil, "child_input_unknown", "child_input_unknown")
	}
	caps = h.hostChildInputCapabilitiesV1(ctx, freshCurrent, freshChild)
	receipt, _, readErr = h.store.hostChildInputReceiptFromPrimaryV1(freshChild, r)
	if errors.Is(readErr, errHostChildInputCancelledV1) {
		return childInputResultV1(caps, nil, "input_cancelled", subagentapp.TaskJobErrorConflict)
	}
	if errors.Is(readErr, errHostChildInputConflictV1) {
		return childInputResultV1(caps, nil, "conflict", subagentapp.TaskJobErrorConflict)
	}
	if readErr == nil && receipt != nil {
		result := h.confirmHostChildInputReceiptV1(ctx, r, caps, receipt)
		result.Err = c.appendError
		return result
	}
	if readErr != nil || c.writeAttempted || c.created.ID != "" {
		return childInputResultV1(caps, nil, "child_input_unknown", "child_input_unknown")
	}
	return childInputResultV1(caps, nil, "conflict", subagentapp.TaskJobErrorConflict)
}

func (h *runtimeServerHandler) prepareHostChildInputStartV1(ctx context.Context, c *hostChildInputCarrierV1) error {
	_, current, child, err := h.readHostChildScopeV1(ctx, c.request)
	if err != nil || current.ID != c.request.ExpectedRunID || current.ChildTurnID != c.request.ExpectedChildTurnID || !subagentapp.TaskJobTerminal(current) {
		return errHostChildInputConflictV1
	}
	turns := listAny(child["turns"])
	if len(turns) > 0 {
		latest, ok := turns[len(turns)-1].(map[string]any)
		if !ok {
			return errHostChildInputConflictV1
		}
		if stringField(latest, "status") == "running" {
			// Reuse the normal terminal owner for this exact cold turn. It
			// closes primary/history and pending steering before a fresh start.
			action, interruptErr := h.runtimeControl().InterruptTurn(ctx, controlapp.InterruptTurnRequest{ThreadID: current.ChildThreadID, TurnID: current.ChildTurnID})
			if interruptErr != nil {
				return interruptErr
			}
			if action.StatusCode != http.StatusOK {
				return errHostChildInputConflictV1
			}
			_, current, child, err = h.readHostChildScopeV1(ctx, c.request)
			if err != nil || current.ID != c.request.ExpectedRunID || current.ChildTurnID != c.request.ExpectedChildTurnID || !scheduleIdleV1(child) {
				return errHostChildInputConflictV1
			}
		}
	}
	c.child, c.current = child, current
	request, err := subagentapp.ApplyProfile(subagentapp.TaskRequest{Prompt: c.request.Message, ProfileName: c.source.ProfileName, RunInBackground: true}, h.subagents)
	if err != nil {
		return err
	}
	execution, err := subagentapp.ResolveExecution(subagentapp.ExecutionInput{Request: request, Parent: subagentapp.ParentExecution{ProviderID: stringField(child, "providerId"), Model: stringField(child, "model"), Effort: stringField(child, "reasoningEffort"), Workspace: stringField(child, "workspace")}, Resolver: h.providerConfig})
	if err != nil {
		return err
	}
	c.taskRequest = request
	maxSteps := subagentapp.ResolvedMaxSteps(request, c.source.MaxModelSteps)
	c.startRequest = controlapp.NormalizeStartTurnRequest(controlapp.StartTurnRequest{ThreadID: current.ChildThreadID, Prompt: c.request.Message, Async: true, ProviderID: execution.ProviderID, Model: execution.Model, ReasoningEffort: execution.Effort,
		Mode: stringField(child, "mode"), ApprovalPolicy: stringField(child, "approvalPolicy"), SandboxMode: stringField(child, "sandboxMode"), MaxModelSteps: &maxSteps, DisableUserInput: true, DisableUserInputSet: true})
	// Acquire the existing runtime slot before taking the fresh turn writer.
	releaseSlot, err := h.acquireRuntimeSubagentSlot(ctx)
	if err != nil {
		return err
	}
	c.cleanup = releaseSlot
	c.turn, err = h.reserveRuntimeChildTurnV1(ctx, current.ChildThreadID)
	if err != nil {
		return err
	}
	identity := &domainjob.HostChildInputIdentityV1{SchemaVersion: 1, ClientMessageID: c.request.ClientMessageID, InputDigest: subagentapp.ChildInputDigestV1(c.request)}
	c.job, err = h.jobs.ReserveHostChildInputRunV1(ctx, c.request.ParentThreadID, current.ChildThreadID, c.turn.turnID, identity)
	return err
}

func (c *hostChildInputCarrierV1) validateStartAnchorV1(ctx context.Context, thread map[string]any) error {
	if c.foreground == nil || c.foreground.Validate(ctx, c.current.ChildThreadID) != nil {
		return errHostChildInputConflictV1
	}
	_, current, _, err := c.host.readHostChildScopeV1(ctx, c.request)
	if err != nil || current.ID != c.request.ExpectedRunID || current.ChildTurnID != c.request.ExpectedChildTurnID || !subagentapp.TaskJobTerminal(current) {
		return errHostChildInputConflictV1
	}
	turns := listAny(thread["turns"])
	if len(turns) > 0 {
		latest, ok := turns[len(turns)-1].(map[string]any)
		if !ok || stringField(latest, "id") != c.request.ExpectedChildTurnID {
			return errHostChildInputConflictV1
		}
	}
	if receipt, _, err := c.host.store.hostChildInputReceiptFromPrimaryV1(thread, c.request); err != nil {
		return err
	} else if receipt != nil {
		return errHostChildInputExistingV1
	}
	return nil
}

// Prepare before case authority registration; a failed job admission must not
// strand a signed staged case context that has no primary turn.
func (h *runtimeServerHandler) prepareHostChildInputRunV1(ctx context.Context, c *hostChildInputCarrierV1, frozen domainsecurity.TurnSecurityContext) error {
	if c.created.ID != "" || frozen.ThreadID != c.current.ChildThreadID || frozen.TurnID != c.turn.turnID {
		return errHostChildInputConflictV1
	}
	providerID, model, effort := c.startRequest.ProviderID, c.startRequest.Model, c.startRequest.ReasoningEffort
	preparation, err := h.prepareRuntimeChildRunV1(c.taskRequest, subagentapp.ParentExecution{Workspace: frozen.WorkspaceRealPath, ProviderID: providerID, Model: model, Effort: effort}, c.request.ParentThreadID, frozen)
	if err != nil {
		return err
	}
	defer preparation.ReleaseSourceLock()
	if preparation.HasSource || preparation.Execution.ProviderID != providerID || preparation.Execution.Model != model || preparation.Execution.Effort != effort {
		return errHostChildInputConflictV1
	}
	start, err := subagentapp.BuildChildRunStartRequest(subagentapp.ChildRunStartInput{ParentGoalID: c.source.ParentGoalID, ParentThreadID: c.request.ParentThreadID, ParentTurnID: c.source.ParentTurnID, ChildThreadID: c.current.ChildThreadID,
		FallbackName: c.source.Name, SourceRef: c.source.ID, Request: preparation.Request, Execution: preparation.Execution, ParentMaxModelSteps: c.source.MaxModelSteps, ToolScope: preparation.ToolScope, ToolSchemaHash: preparation.ToolSchemaHash,
		DelegatedToolManifest: preparation.DelegatedToolManifest, SystemPromptHash: preparation.SystemPromptHash, MaxChildRuns: h.subagents.MaxChildRuns})
	if err != nil {
		return err
	}
	start.ContinueFrom, start.SourceRef, start.ChildTurnID = c.source.ID, c.source.ID, c.turn.turnID
	start.HostChildInput = &domainjob.HostChildInputIdentityV1{SchemaVersion: 1, ClientMessageID: c.request.ClientMessageID, InputDigest: subagentapp.ChildInputDigestV1(c.request)}
	// A direct user input reuses a child session; it does not spawn another
	// delegated child. The model spawn quota remains enforced at its owner.
	start.MaxChildRunsSet = false
	c.created, err = h.jobs.StartReservedHostChildInputRunV1(ctx, start, c.job)
	if err != nil {
		return err
	}
	c.preparation = preparation
	return nil
}

func (c *hostChildInputCarrierV1) bindStartRecordsV1(frozen domainsecurity.TurnSecurityContext, plan turnapp.StartPlan) error {
	if c.created.ID == "" || plan.Record.UserItem == nil || frozen.ThreadID != c.current.ChildThreadID || frozen.TurnID != c.turn.turnID {
		return errHostChildInputConflictV1
	}
	binding := c.bindingV1(c.created.ID, frozen.TurnID, plan.UserItemID, "start", stringField(plan.Record.UserItem, "createdAt"), frozen.ContextDigest, stringField(plan.Record.UserItem, "text"))
	plan.Record.Turn[domainjob.HostChildInputMarkerFieldV1] = []any{binding.RecordV1()}
	return domainjob.ValidateHostChildInputBindingV1(binding)
}

func (c *hostChildInputCarrierV1) bindingV1(jobID, turnID, messageID, action, admittedAt, contextDigest, text string) domainjob.HostChildInputBindingV1 {
	return domainjob.HostChildInputBindingV1{SchemaVersion: 1, ClientMessageID: c.request.ClientMessageID, InputDigest: subagentapp.ChildInputDigestV1(c.request), ParentThreadID: c.request.ParentThreadID, SourceJobID: c.request.JobID,
		JobID: jobID, ChildThreadID: c.current.ChildThreadID, ChildTurnID: turnID, MessageID: messageID, Action: action, ContextDigest: contextDigest, ContentDigest: domainjob.HostChildInputTextDigestV1(text), AdmittedAt: admittedAt}
}

func (h *runtimeServerHandler) registerHostChildInputRunV1(c *hostChildInputCarrierV1, cancel context.CancelFunc) error {
	if c.created.ID == "" {
		return errors.New("Host child run unavailable")
	}
	barrier, registered := h.runtimeSubagentState().RegisterBackgroundJobWithStartBarrier(c.created.ID, cancel)
	if !registered || barrier == nil {
		return errors.New("Host child run control unavailable")
	}
	releaseSlot := c.cleanup
	c.cleanup = func() {
		barrier.Cancel()
		h.runtimeSubagentState().UnregisterBackgroundJob(c.created.ID)
		if releaseSlot != nil {
			releaseSlot()
		}
	}
	c.startBarrier = barrier
	return nil
}

// Durable claim and real execution handoff share the existing one-shot start
// barrier, after all fallible turn preparation and event-tail work completed.
func (h *runtimeServerHandler) claimHostChildInputRunV1(ctx context.Context, c *hostChildInputCarrierV1, start func() error) error {
	if c.startBarrier == nil || c.foreground == nil || start == nil {
		return errHostChildInputConflictV1
	}
	return c.startBarrier.StartIfActive(ctx, func() error {
		snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, c.created.ChildThreadID)
		if err != nil {
			return err
		}
		_, frozen, err := h.store.hostChildInputBindingForRunV1(snapshot.Thread, c.created)
		if err != nil {
			return err
		}
		effectCtx, release, err := h.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, frozen, false)
		if err != nil {
			return err
		}
		defer release()
		if err := c.foreground.Validate(effectCtx, c.created.ChildThreadID); err != nil {
			return err
		}
		if !h.hostChildInputRecordCurrentV1(effectCtx, c.created, false) {
			return errors.New("Host child run current authority unavailable")
		}
		record, err := h.jobs.UpdateChildRun(c.created.ID, domainjob.UpdateRequest{Status: "running"})
		if err != nil {
			return err
		}
		c.created, err = h.jobs.ClaimChildRunStart(record)
		if err != nil {
			return err
		}
		// Claim is the exact durable CAS. No fallible preparation remains
		// between that CAS and the actual owner handoff.
		return start()
	})
}

func (h *runtimeServerHandler) finishHostChildInputRunV1(c *hostChildInputCarrierV1) {
	if c == nil || c.created.ID == "" {
		return
	}
	record, err := h.jobs.LoadChildRun(c.created.ID)
	if err != nil || subagentapp.TaskJobTerminal(record) {
		return
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), record.ChildThreadID)
	if err != nil {
		return
	}
	status, found, terminal, terminalErr := turnapp.InspectTerminalAuthorityV1(snapshot.Thread, record.ChildTurnID)
	if terminalErr != nil || !found || !terminal {
		return
	}
	_, _ = h.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: status, FailureCode: domainjob.NormalizeFailureCode("", status, status != "completed")})
}

func (h *runtimeServerHandler) hostChildInputRecordCurrentV1(ctx context.Context, record domainjob.Record, requireClaim bool) bool {
	if domainjob.ValidateHostChildInputRecordV1(record) != nil || record.HostChildInput == nil || requireClaim && record.HostChildInput.ClaimedAt == "" {
		return false
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil {
		return false
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, record.ChildThreadID)
	if err != nil || stringField(snapshot.Thread, "parentThreadId") != record.ParentThreadID {
		return false
	}
	binding, frozen, err := h.store.hostChildInputBindingForRunV1(snapshot.Thread, record)
	if err != nil || binding == nil || frozen.TenantID != principal.TenantID || frozen.UserID != principal.UserID {
		return false
	}
	input := turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: frozen, Workspace: frozen.WorkspaceRealPath}
	if domainsecurity.TurnSecurityContextIsBoundaryOnly(frozen) {
		return turnsecurityapp.ValidateCurrentHostBoundary(input) == nil
	}
	return turnsecurityapp.ValidateCurrentForEffect(input, false) == nil
}

func (h *runtimeServerHandler) confirmHostChildInputReceiptV1(ctx context.Context, request subagentapp.ChildInputRequestV1, caps subagentapp.ChildInputCapabilitiesV1, receipt *subagentapp.ChildInputReceiptV1) subagentapp.TaskJobServiceResult {
	if receipt.Action == "start" {
		record, err := h.jobs.LoadChildRun(receipt.JobID)
		if err != nil || domainjob.ValidateHostChildInputRecordV1(record) != nil || record.LateCompletionSuppressed || record.DeadLetterReason != "" || record.CompletionDeliveryStatus == "dead_letter" || record.HostChildInput == nil || record.HostChildInput.ClaimedAt == "" || record.HostChildInput.ClientMessageID != request.ClientMessageID || record.HostChildInput.InputDigest != subagentapp.ChildInputDigestV1(request) || record.ContinueFrom != request.JobID || record.ParentThreadID != request.ParentThreadID || record.ChildThreadID != receipt.ChildThreadID || record.ChildTurnID != receipt.ChildTurnID {
			return childInputResultV1(caps, nil, "child_input_unknown", "child_input_unknown")
		}
		if !h.runtimeSubagentState().BackgroundJobStartedV1(record.ID) {
			// A retained claim after a crash or failed callback is uncertain.
			// Only a non-orphan terminal run with its exact primary terminal
			// may retain confirmed historical admission after owner cleanup.
			snapshot, readErr := h.store.ReadPrimaryThreadSnapshotV1(ctx, record.ChildThreadID)
			status, found, terminal, terminalErr := turnapp.InspectTerminalAuthorityV1(snapshot.Thread, record.ChildTurnID)
			if readErr != nil || terminalErr != nil || record.Orphaned || !subagentapp.TaskJobTerminal(record) || !found || !terminal || record.Status != status {
				return childInputResultV1(caps, nil, "child_input_unknown", "child_input_unknown")
			}
		}
	}
	return childInputResultV1(caps, receipt, "", "")
}

// primary readback validates actual frozen context and signed steering/item
// membership. The marker is not a standalone capability or a second ledger.
func (s *DurableEventSessionStore) hostChildInputReceiptFromPrimaryV1(thread map[string]any, request subagentapp.ChildInputRequestV1) (*subagentapp.ChildInputReceiptV1, *domainjob.HostChildInputBindingV1, error) {
	seen := map[string]bool{}
	var result *subagentapp.ChildInputReceiptV1
	var matched *domainjob.HostChildInputBindingV1
	for _, raw := range listAny(thread["turns"]) {
		turn, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, errHostChildInputConflictV1
		}
		markers, present := turn[domainjob.HostChildInputMarkerFieldV1]
		if !present {
			continue
		}
		entries, ok := markers.([]any)
		if !ok || len(entries) == 0 {
			return nil, nil, errHostChildInputConflictV1
		}
		for _, value := range entries {
			binding, err := domainjob.DecodeHostChildInputBindingV1(value)
			if err != nil || seen[binding.ClientMessageID] || binding.ChildThreadID != stringField(thread, "id") || binding.ChildTurnID != stringField(turn, "id") || binding.ParentThreadID != stringField(thread, "parentThreadId") {
				return nil, nil, errHostChildInputConflictV1
			}
			seen[binding.ClientMessageID] = true
			frozen, err := turnapp.FrozenSecurityContextForTurn(thread, binding.ChildTurnID)
			if err != nil || frozen.ContextDigest != binding.ContextDigest {
				return nil, nil, errHostChildInputConflictV1
			}
			admission := "admitted"
			cancelled := false
			count := 0
			if binding.Action == "start" {
				for i, value := range listAny(turn["items"]) {
					item, ok := value.(map[string]any)
					if !ok {
						return nil, nil, errHostChildInputConflictV1
					}
					if stringField(item, "id") != binding.MessageID {
						continue
					}
					count++
					if i != 0 || stringField(item, "kind") != "user_message" || stringField(item, "turnId") != binding.ChildTurnID || stringField(item, "threadId") != binding.ChildThreadID || domainjob.HostChildInputTextDigestV1(stringField(item, "text")) != binding.ContentDigest || stringField(item, "createdAt") != binding.AdmittedAt {
						return nil, nil, errHostChildInputConflictV1
					}
				}
			} else {
				admission = "queued"
				for _, value := range listAny(turn["steering"]) {
					entry, ok := value.(map[string]any)
					if !ok || stringField(entry, "id") != binding.MessageID {
						continue
					}
					count++
					if stringField(entry, "clientUserMessageId") != binding.ClientMessageID || stringField(entry, "admittedAt") != binding.AdmittedAt || domainjob.HostChildInputTextDigestV1(stringField(entry, "text")) != binding.ContentDigest {
						return nil, nil, errHostChildInputConflictV1
					}
					entry = cloneMap(entry)
					if n, ok := entry["projectionVersion"].(json.Number); ok {
						if n.String() != "1" {
							return nil, nil, errHostChildInputConflictV1
						}
						entry["projectionVersion"] = 1
					}
					pending := cloneMap(entry)
					switch stringField(entry, "status") {
					case "pending":
					case "cancelled":
						cancelled = true
						pending["status"] = "pending"
						delete(pending, "cancelledAt")
						delete(pending, "cancelReason")
					case "promoted":
						pending["status"] = "pending"
						for _, key := range []string{"promotedAt", "promotedItemId", "promotionAuthorityKeyId", "promotionAuthorityPublicKey", "promotionAuthoritySignature"} {
							delete(pending, key)
						}
						admission = "admitted"
					default:
						return nil, nil, errHostChildInputConflictV1
					}
					if domainsteering.ValidatePendingEntryForContextV1(pending, binding.ContextDigest) != nil || s.verifySteeringEntryAuthorityV1(pending, binding.ContextDigest) != nil {
						return nil, nil, errHostChildInputConflictV1
					}
					if admission == "admitted" {
						matches := 0
						for _, value := range listAny(turn["items"]) {
							item, _ := value.(map[string]any)
							if stringField(item, "id") == stringField(entry, "promotedItemId") {
								matches++
								adapted := cloneMap(item)
								if n, ok := adapted["steeringProjectionVersion"].(json.Number); ok && n.String() == "1" {
									adapted["steeringProjectionVersion"] = 1
								}
								if _, err := domainsteering.NewPromotionCommitV1(binding.ChildThreadID, binding.ChildTurnID, binding.ContextDigest, entry, adapted, s.verifySteeringEntryAuthorityV1); err != nil {
									return nil, nil, errHostChildInputConflictV1
								}
							}
						}
						if matches != 1 {
							return nil, nil, errHostChildInputConflictV1
						}
					}
				}
			}
			if count != 1 {
				return nil, nil, errHostChildInputConflictV1
			}
			if binding.ClientMessageID == request.ClientMessageID {
				if binding.InputDigest != subagentapp.ChildInputDigestV1(request) || binding.SourceJobID != request.JobID || binding.ParentThreadID != request.ParentThreadID {
					return nil, nil, errHostChildInputConflictV1
				}
				if binding.Action == "steer" && (binding.JobID != request.ExpectedRunID || binding.ChildTurnID != request.ExpectedChildTurnID) {
					return nil, nil, errHostChildInputConflictV1
				}
				if cancelled {
					return nil, &binding, errHostChildInputCancelledV1
				}
				copy := binding
				matched = &copy
				result = &subagentapp.ChildInputReceiptV1{Action: binding.Action, Admission: admission, ClientMessageID: binding.ClientMessageID, JobID: binding.JobID, ChildThreadID: binding.ChildThreadID, ChildTurnID: binding.ChildTurnID, MessageID: binding.MessageID}
			}
		}
	}
	return result, matched, nil
}

func (s *DurableEventSessionStore) hostChildInputBindingForRunV1(thread map[string]any, record domainjob.Record) (*domainjob.HostChildInputBindingV1, domainsecurity.TurnSecurityContext, error) {
	if record.HostChildInput == nil {
		return nil, domainsecurity.TurnSecurityContext{}, errHostChildInputConflictV1
	}
	if _, _, err := s.hostChildInputReceiptFromPrimaryV1(thread, subagentapp.ChildInputRequestV1{Mode: subagentapp.ChildInputInspectV1, ParentThreadID: record.ParentThreadID, JobID: record.ContinueFrom}); err != nil {
		return nil, domainsecurity.TurnSecurityContext{}, err
	}
	for _, value := range listAny(thread["turns"]) {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") != record.ChildTurnID {
			continue
		}
		for _, raw := range listAny(turn[domainjob.HostChildInputMarkerFieldV1]) {
			binding, err := domainjob.DecodeHostChildInputBindingV1(raw)
			if err != nil {
				return nil, domainsecurity.TurnSecurityContext{}, err
			}
			if binding.ClientMessageID != record.HostChildInput.ClientMessageID {
				continue
			}
			if binding.Action != "start" || binding.InputDigest != record.HostChildInput.InputDigest || binding.JobID != record.ID || binding.SourceJobID != record.ContinueFrom || binding.ParentThreadID != record.ParentThreadID || binding.ChildThreadID != record.ChildThreadID || binding.ChildTurnID != record.ChildTurnID {
				return nil, domainsecurity.TurnSecurityContext{}, errHostChildInputConflictV1
			}
			frozen, err := turnapp.FrozenSecurityContextForTurn(thread, record.ChildTurnID)
			if err != nil || binding.ContextDigest != frozen.ContextDigest {
				return nil, domainsecurity.TurnSecurityContext{}, errHostChildInputConflictV1
			}
			count := 0
			for position, rawItem := range listAny(turn["items"]) {
				item, _ := rawItem.(map[string]any)
				if stringField(item, "id") == binding.MessageID {
					count++
					if position != 0 || binding.MessageID != "item_"+record.ChildTurnID+"_user" || stringField(item, "kind") != "user_message" || stringField(item, "role") != "user" || stringField(item, "threadId") != record.ChildThreadID || stringField(item, "turnId") != record.ChildTurnID || stringField(item, "createdAt") != binding.AdmittedAt || domainjob.HostChildInputTextDigestV1(stringField(item, "text")) != binding.ContentDigest {
						return nil, domainsecurity.TurnSecurityContext{}, errHostChildInputConflictV1
					}
				}
			}
			if count != 1 {
				return nil, domainsecurity.TurnSecurityContext{}, errHostChildInputConflictV1
			}
			return &binding, frozen, nil
		}
	}
	return nil, domainsecurity.TurnSecurityContext{}, errors.New("Host child primary admission unavailable")
}

type hostChildInputTurnStoreV1 struct {
	*DurableEventSessionStore
	carrier *hostChildInputCarrierV1
}

func (s hostChildInputTurnStoreV1) AppendTurnToThreadIfBaseline(threadID string, turn map[string]any, providerID string, patch map[string]any, workspace, digest string) error {
	c := s.carrier
	if c.foreground == nil || threadID != c.current.ChildThreadID {
		return errHostChildInputConflictV1
	}
	err := s.DurableEventSessionStore.appendHostChildInputTurnV1(c, nil, threadID, turn, providerID, patch, workspace, digest)
	if err != nil {
		// A primary commit followed by sidecar failure is still a committed
		// admission. Preserve the diagnostic while finishing existing owners.
		snapshot, readErr := s.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
		if readErr == nil {
			receipt, _, readErr := s.hostChildInputReceiptFromPrimaryV1(snapshot.Thread, c.request)
			if readErr == nil && receipt != nil && receipt.ChildTurnID == stringField(turn, "id") {
				c.appendError = err
				return nil
			}
		}
	}
	return err
}

func (s hostChildInputTurnStoreV1) GetThread(threadID string) (map[string]any, error) {
	snapshot, err := s.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
	return snapshot.Thread, err
}

type hostChildInputSteeringStoreV1 struct {
	*DurableEventSessionStore
	carrier *hostChildInputCarrierV1
}

func (s hostChildInputSteeringStoreV1) AdmitSteeringEntryForContext(threadID, turnID, expectedTurnID, digest string, entry map[string]any) (map[string]any, error) {
	return s.DurableEventSessionStore.admitHostChildInputSteeringV1(s.carrier, threadID, turnID, expectedTurnID, digest, entry)
}

func (c *hostChildInputCarrierV1) configureLoopRequestV1(request *startRuntimeTurnRequest, preparation subagentapp.TaskRunPreparation) {
	request.InternalChildRunID, request.InternalSubagentDepth, request.InternalUsageSource = c.created.ID, 1, usageapp.SourceSubagent
	request.InternalToolScope = append([]string(nil), preparation.ToolScope...)
	request.InternalSystemPrompt = subagentapp.SystemPromptForRequest(preparation.Request)
	// Host follow-up input is ordinary background work; profile budgets stay
	// soft reporting metadata rather than foreground output-token authority.
	request.InternalOutputTokenBudget = 0
}

// retain reflect-based request equality at the private start/steer boundary.
func (c *hostChildInputCarrierV1) matchesStartV1(threadID string, request startRuntimeTurnRequest) bool {
	return c.turn != nil && threadID == c.current.ChildThreadID && reflect.DeepEqual(request, c.startRequest)
}

var _ turnstartapp.DurableStore = hostChildInputTurnStoreV1{}
