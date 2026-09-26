package control

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"

	appusage "analytix.local/runtime-go/internal/app/usage"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const InternalAutoContinuePromptV1 = "Continue the parent task after a host-verified background completion. Use only durable typed lifecycle state."

var (
	ErrNoDriver                = errors.New("runtime control driver is required")
	ErrMissingThreadID         = errors.New("thread id is required")
	ErrMissingTurnID           = errors.New("turn id is required")
	ErrMissingPrompt           = errors.New("prompt is required")
	ErrMissingApprovalID       = errors.New("approval id is required")
	ErrInvalidDecision         = errors.New("approval decision must be allow or deny")
	ErrMissingInputID          = errors.New("user input id is required")
	ErrMissingSteerText        = errors.New("steer text is required")
	ErrInvalidSteerClientID    = errors.New("client user message id must be an opaque UUID")
	ErrThreadNotFound          = errors.New("thread not found")
	ErrAttachmentNotAuthorized = errors.New("attachment is not authorized for this turn")
	ErrTerminalArbitration     = errors.New("turn terminal arbitration is unavailable")
	ErrThreadTransition        = errors.New("thread security transition is active")
	ErrTurnExecutionConflict   = errors.New("turn execution authority is already active")
	ErrRuntimeShuttingDown     = errors.New("runtime is shutting down")
	ErrTurnTerminalizing       = errors.New("turn terminal publication is already active")
)

type Controller struct {
	driver Driver

	mu                    sync.Mutex
	auxiliary             map[string]*auxiliaryOperation
	foregroundPreparing   map[string]int
	turnCancels           map[turnKey]context.CancelFunc
	interruptReservations map[turnKey]struct{}
	cancelReservations    map[turnKey]struct{}
	candidateTerminals    map[turnKey]string
	candidateTokens       map[turnKey]*candidateTerminalToken
	steerAdmissions       map[turnKey]int
	threadTransitions     map[string]struct{}
	turnStateChanged      chan struct{}
	shuttingDown          bool
	activeTurnOperations  int
	idle                  chan struct{}
}

type Driver interface {
	SendTurn(context.Context, StartTurnRequest) (map[string]any, error)
	SteerTurn(context.Context, SteerTurnRequest) (ActionResult, error)
	InterruptTurn(context.Context, InterruptTurnRequest) (ActionResult, error)
	ApproveTool(context.Context, ApprovalDecision) (ActionResult, error)
	RespondUserInput(context.Context, UserInputResponse) (ActionResult, error)
}

type ActionResult struct {
	StatusCode int
	Body       map[string]any
}

type StartTurnRequest struct {
	ThreadID              string
	Prompt                string
	DisplayText           string
	RiskIntent            string
	Async                 bool
	Model                 string
	ProviderID            string
	EndpointFormat        string
	ReasoningEffort       string
	Mode                  string
	ApprovalPolicy        string
	SandboxMode           string
	AttachmentIDs         []string
	FileReferences        []any
	GUIPlan               map[string]any
	DisableUserInput      bool
	DisableUserInputSet   bool
	WorkspaceCheckpointID string
	MaxModelSteps         *int
	InternalToolScope     []string
	InternalSubagentDepth int
	InternalUsageSource   string
	InternalSystemPrompt  string
	InternalChildRunID    string
	// InternalTurnID and its auto-continue lineage are host-only reservation
	// inputs. Public HTTP decoding never populates them.
	InternalTurnID                   string
	InternalAutoContinueJobID        string
	InternalAutoContinueParentTurnID string
	// InternalOutputTokenBudget is issued only by the host for a bounded child
	// turn. It is not decoded from the public HTTP start-turn contract.
	InternalOutputTokenBudget int
}

// ValidateInternalReservedTurnRequestV1 rejects every carrier except the
// exact host-built reserved-turn shape. Comparing the complete struct keeps
// future provider, security, and content fields closed by default.
func ValidateInternalReservedTurnRequestV1(request StartTurnRequest, fixedPrompt, usageSource string) error {
	expected := StartTurnRequest{
		ThreadID: request.ThreadID, Prompt: fixedPrompt, Async: true, InternalUsageSource: usageSource,
		InternalTurnID: request.InternalTurnID, InternalAutoContinueJobID: request.InternalAutoContinueJobID,
		InternalAutoContinueParentTurnID: request.InternalAutoContinueParentTurnID,
	}
	if !reflect.DeepEqual(request, expected) && !reflect.DeepEqual(request, NormalizeStartTurnRequest(expected)) {
		return errors.New("internal reserved turn carrier is invalid")
	}
	return nil
}

type SteerTurnRequest struct {
	ThreadID            string
	TurnID              string
	Text                string
	DisplayText         string
	RiskIntent          string
	ClientUserMessageID string
	ExpectedTurnID      string
	AttachmentIDs       []string
	FileReferences      []any
}

type InterruptTurnRequest struct {
	ThreadID string
	TurnID   string
	Discard  bool
}

type ApprovalDecision struct {
	ApprovalID string
	Decision   string
	Reason     string
}

type UserInputResponse struct {
	InputID   string
	Answers   []map[string]string
	Cancelled bool
}

type turnKey struct {
	threadID string
	turnID   string
}

// candidateTerminalToken is an in-process, unforgeable handoff from the
// runtime loop's final steering boundary to the existing terminal publication
// path. It never crosses persistence, HTTP, or provider boundaries.
type candidateTerminalToken struct {
	controller    *Controller
	key           turnKey
	contextDigest string
	consumed      bool
}

type candidateTerminalContextKey struct{}

func NewController(driver Driver) *Controller {
	idle := make(chan struct{})
	close(idle)
	return &Controller{
		driver:                driver,
		auxiliary:             map[string]*auxiliaryOperation{},
		foregroundPreparing:   map[string]int{},
		turnCancels:           map[turnKey]context.CancelFunc{},
		interruptReservations: map[turnKey]struct{}{},
		cancelReservations:    map[turnKey]struct{}{},
		candidateTerminals:    map[turnKey]string{},
		candidateTokens:       map[turnKey]*candidateTerminalToken{},
		steerAdmissions:       map[turnKey]int{},
		threadTransitions:     map[string]struct{}{},
		turnStateChanged:      make(chan struct{}),
		idle:                  idle,
	}
}

func (c *Controller) SendTurn(ctx context.Context, request StartTurnRequest) (map[string]any, error) {
	if err := c.requireDriver(); err != nil {
		return nil, err
	}
	if err := domainmodel.ValidateReasoningEffortV1(request.ReasoningEffort); err != nil {
		return nil, err
	}
	if request.InternalTurnID != "" || request.InternalAutoContinueJobID != "" || request.InternalAutoContinueParentTurnID != "" {
		if err := ValidateInternalReservedTurnRequestV1(request, InternalAutoContinuePromptV1, appusage.SourceTurn); err != nil {
			return nil, err
		}
	}
	request = NormalizeStartTurnRequest(request)
	if request.ThreadID == "" {
		return nil, ErrMissingThreadID
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, ErrMissingPrompt
	}
	return c.driver.SendTurn(ctx, request)
}

func (c *Controller) SteerTurn(ctx context.Context, request SteerTurnRequest) (ActionResult, error) {
	if err := c.requireDriver(); err != nil {
		return ActionResult{}, err
	}
	request = NormalizeSteerTurnRequest(request)
	if request.ThreadID == "" {
		return ActionResult{}, ErrMissingThreadID
	}
	if request.TurnID == "" {
		return ActionResult{}, ErrMissingTurnID
	}
	if request.Text == "" {
		return ActionResult{}, ErrMissingSteerText
	}
	return c.runTurnOperation(func() (ActionResult, error) { return c.driver.SteerTurn(ctx, request) })
}

func (c *Controller) InterruptTurn(ctx context.Context, request InterruptTurnRequest) (ActionResult, error) {
	if err := c.requireDriver(); err != nil {
		return ActionResult{}, err
	}
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.TurnID = strings.TrimSpace(request.TurnID)
	if request.ThreadID == "" {
		return ActionResult{}, ErrMissingThreadID
	}
	if request.TurnID == "" {
		return ActionResult{}, ErrMissingTurnID
	}
	return c.runTurnOperation(func() (ActionResult, error) { return c.driver.InterruptTurn(ctx, request) })
}

func (c *Controller) ApproveTool(ctx context.Context, request ApprovalDecision) (ActionResult, error) {
	if err := c.requireDriver(); err != nil {
		return ActionResult{}, err
	}
	request.ApprovalID = strings.TrimSpace(request.ApprovalID)
	request.Decision = strings.TrimSpace(request.Decision)
	if request.ApprovalID == "" {
		return ActionResult{}, ErrMissingApprovalID
	}
	if request.Decision != "allow" && request.Decision != "deny" {
		return ActionResult{}, ErrInvalidDecision
	}
	return c.runTurnOperation(func() (ActionResult, error) { return c.driver.ApproveTool(ctx, request) })
}

func (c *Controller) RespondUserInput(ctx context.Context, request UserInputResponse) (ActionResult, error) {
	if err := c.requireDriver(); err != nil {
		return ActionResult{}, err
	}
	request.InputID = strings.TrimSpace(request.InputID)
	if request.InputID == "" {
		return ActionResult{}, ErrMissingInputID
	}
	return c.runTurnOperation(func() (ActionResult, error) { return c.driver.RespondUserInput(ctx, request) })
}

func (c *Controller) runTurnOperation(run func() (ActionResult, error)) (ActionResult, error) {
	finish, admitted := c.BeginTurnOperation()
	if !admitted {
		return runtimeShuttingDownAction(), nil
	}
	defer finish()
	return run()
}

func runtimeShuttingDownAction() ActionResult {
	return ActionResult{StatusCode: 503, Body: map[string]any{
		"code": "runtime_shutting_down", "message": "runtime is shutting down",
	}}
}

// ReserveSteerAdmission linearizes one exact durable steering mutation with
// terminal publication. Callers must acquire and freshly validate the turn's
// context-effect lease first, preserving the repository-wide effect ->
// Controller lock order, and must hold the reservation through admission and
// its event tail.
func (c *Controller) ReserveSteerAdmission(threadID, turnID string) (func(), error) {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok || c == nil {
		return nil, ErrTerminalArbitration
	}
	c.mu.Lock()
	if c.shuttingDown {
		c.mu.Unlock()
		return nil, ErrRuntimeShuttingDown
	}
	if _, terminalActive := c.candidateTerminals[key]; terminalActive || c.threadTerminalActiveOtherThanLocked(key.threadID, key) {
		c.mu.Unlock()
		return nil, ErrTurnTerminalizing
	}
	if _, reserved := c.interruptReservations[key]; reserved {
		c.mu.Unlock()
		return nil, ErrTurnTerminalizing
	}
	if _, cancelled := c.cancelReservations[key]; cancelled {
		c.mu.Unlock()
		return nil, ErrTurnTerminalizing
	}
	if _, transitioning := c.threadTransitions[key.threadID]; transitioning {
		c.mu.Unlock()
		return nil, ErrThreadTransition
	}
	c.steerAdmissions[key]++
	c.notifyTurnStateChangedLocked()
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			if count := c.steerAdmissions[key]; count <= 1 {
				delete(c.steerAdmissions, key)
			} else {
				c.steerAdmissions[key] = count - 1
			}
			c.notifyTurnStateChangedLocked()
			c.mu.Unlock()
		})
	}, nil
}

func (c *Controller) RegisterTurnCancel(threadID, turnID string, cancel context.CancelFunc) bool {
	return c.RegisterTurnCancelWithError(threadID, turnID, cancel) == nil
}

// RegisterTurnCancelAfterThreadTail serializes a new execution behind a
// same-thread terminal or security-transition tail. The request context bounds
// the wait, and registration happens under the same mutex that observes the
// tail release so a completed public turn cannot leave a transient admission
// gap. Active non-terminal owners remain subject to the later transition
// quiescence barrier.
func (c *Controller) RegisterTurnCancelAfterThreadTail(
	ctx context.Context,
	threadID, turnID string,
	cancel context.CancelFunc,
) error {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok || cancel == nil || c == nil || ctx == nil {
		return ErrTurnExecutionConflict
	}
	releasePreparation, err := c.PrepareForeground(ctx, threadID)
	if err != nil {
		return err
	}
	defer releasePreparation()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.mu.Lock()
		if c.shuttingDown {
			c.mu.Unlock()
			return ErrRuntimeShuttingDown
		}
		if _, exists := c.turnCancels[key]; exists {
			c.mu.Unlock()
			return ErrTurnExecutionConflict
		}
		if _, reserved := c.interruptReservations[key]; reserved {
			c.mu.Unlock()
			return ErrTurnExecutionConflict
		}
		if _, cancelled := c.cancelReservations[key]; cancelled {
			c.mu.Unlock()
			return ErrTurnExecutionConflict
		}
		if _, terminalActive := c.candidateTerminals[key]; terminalActive {
			c.mu.Unlock()
			return ErrTurnExecutionConflict
		}
		_, transitioning := c.threadTransitions[key.threadID]
		if transitioning || c.threadTerminalActiveLocked(key.threadID) {
			changed := c.turnStateChanged
			c.mu.Unlock()
			select {
			case <-changed:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		c.turnCancels[key] = cancel
		c.notifyTurnStateChangedLocked()
		c.mu.Unlock()
		return nil
	}
}

func (c *Controller) RegisterTurnCancelWithError(threadID, turnID string, cancel context.CancelFunc) error {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok || cancel == nil {
		return ErrTurnExecutionConflict
	}
	c.mu.Lock()
	if c.shuttingDown {
		c.mu.Unlock()
		return ErrRuntimeShuttingDown
	}
	if auxiliary := c.auxiliary[key.threadID]; auxiliary != nil {
		c.mu.Unlock()
		auxiliary.cancel()
		return ErrTurnExecutionConflict
	}
	if _, exists := c.turnCancels[key]; exists {
		c.mu.Unlock()
		return ErrTurnExecutionConflict
	}
	if _, reserved := c.interruptReservations[key]; reserved {
		c.mu.Unlock()
		return ErrTurnExecutionConflict
	}
	if _, cancelled := c.cancelReservations[key]; cancelled {
		c.mu.Unlock()
		return ErrTurnExecutionConflict
	}
	if _, terminalActive := c.candidateTerminals[key]; terminalActive {
		c.mu.Unlock()
		return ErrTurnExecutionConflict
	}
	if _, transitioning := c.threadTransitions[key.threadID]; transitioning {
		c.mu.Unlock()
		return ErrTurnExecutionConflict
	}
	// A durable terminal transition owns the whole thread until its
	// post-CAS event/projection tail has completed. Admitting another turn for
	// the same thread here could let a new context epoch cross that tail even
	// though the previous thread record already reads idle.
	if c.threadTerminalActiveLocked(key.threadID) {
		c.mu.Unlock()
		return ErrTurnExecutionConflict
	}
	c.turnCancels[key] = cancel
	c.notifyTurnStateChangedLocked()
	c.mu.Unlock()
	return nil
}

// BeginTurnOperation admits one host-owned turn operation. The returned done
// function is idempotent and must run only after terminal persistence has
// completed or ownership has been transferred to an asynchronous worker.
func (c *Controller) BeginTurnOperation() (done func(), admitted bool) {
	if c == nil {
		return func() {}, false
	}
	c.mu.Lock()
	if c.shuttingDown {
		c.mu.Unlock()
		return func() {}, false
	}
	if c.activeTurnOperations == 0 {
		c.idle = make(chan struct{})
	}
	c.activeTurnOperations++
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(c.finishTurnOperation)
	}, true
}

func (c *Controller) finishTurnOperation() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeTurnOperations <= 0 {
		return
	}
	c.activeTurnOperations--
	if c.activeTurnOperations == 0 {
		close(c.idle)
	}
}

// BeginShutdown atomically closes turn admission before cancelling all
// registered executions. WaitForTurnOperations then observes the same closed
// admission epoch, so a late operation cannot escape the shutdown barrier.
func (c *Controller) BeginShutdown() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	c.shuttingDown = true
	for key := range c.turnCancels {
		c.cancelReservations[key] = struct{}{}
	}
	if len(c.turnCancels) > 0 {
		c.notifyTurnStateChangedLocked()
	}
	count := len(c.turnCancels) + len(c.auxiliary)
	cancels := make([]context.CancelFunc, 0, len(c.auxiliary))
	for _, operation := range c.auxiliary {
		cancels = append(cancels, operation.cancel)
	}
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return count
}

func (c *Controller) WaitForTurnOperations(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	idle := c.idle
	c.mu.Unlock()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Controller) ShuttingDown() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shuttingDown
}

func (c *Controller) UnregisterTurnCancel(threadID, turnID string) bool {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok {
		return false
	}
	c.mu.Lock()
	_, existed := c.turnCancels[key]
	delete(c.turnCancels, key)
	delete(c.cancelReservations, key)
	if existed {
		c.notifyTurnStateChangedLocked()
	}
	c.mu.Unlock()
	return existed
}

// CancelThreadTurnsAndWait cancels every registered execution for one thread
// and does not return until those executions have released their terminal
// persistence ownership. Callers hold the shared thread transition writer,
// preventing a new turn from crossing RegisterRequired -> durable Append while
// this barrier is active.
func (c *Controller) CancelThreadTurnsAndWait(ctx context.Context, threadID string) error {
	return c.waitForThreadTurns(ctx, threadID, "", true)
}

func (c *Controller) WaitForThreadTurns(ctx context.Context, threadID string) error {
	return c.waitForThreadTurns(ctx, threadID, "", false)
}

func (c *Controller) CancelThreadTurnsAndWaitExcept(ctx context.Context, threadID, excludedTurnID string) error {
	excludedTurnID = strings.TrimSpace(excludedTurnID)
	if excludedTurnID == "" {
		return ErrMissingTurnID
	}
	return c.waitForThreadTurns(ctx, threadID, excludedTurnID, true)
}

// ReserveThreadTransition blocks every new foreground owner for one thread
// while the caller drains old owners, paused gates, and commits an epoch or
// workspace transition. Terminal owners that were already admitted remain
// able to finish, avoiding a writer/terminal lock inversion.
func (c *Controller) ReserveThreadTransition(threadID string) (func(), error) {
	if c == nil {
		return nil, ErrThreadTransition
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil, ErrMissingThreadID
	}
	c.mu.Lock()
	if c.shuttingDown {
		c.mu.Unlock()
		return nil, context.Canceled
	}
	if _, exists := c.threadTransitions[threadID]; exists {
		c.mu.Unlock()
		return nil, ErrThreadTransition
	}
	c.threadTransitions[threadID] = struct{}{}
	c.notifyTurnStateChangedLocked()
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			if _, exists := c.threadTransitions[threadID]; exists {
				delete(c.threadTransitions, threadID)
				c.notifyTurnStateChangedLocked()
			}
			c.mu.Unlock()
		})
	}, nil
}

func (c *Controller) waitForThreadTurns(ctx context.Context, threadID, excludedTurnID string, cancelActive bool) error {
	if c == nil {
		return nil
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ErrMissingThreadID
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		c.mu.Lock()
		cancels := make([]context.CancelFunc, 0)
		excluded := turnKey{threadID: threadID, turnID: excludedTurnID}
		active := c.threadTerminalActiveLocked(threadID)
		if excludedTurnID != "" {
			active = c.threadTerminalActiveOtherThanLocked(threadID, excluded)
		}
		if c.threadSteerAdmissionActiveOtherThanLocked(threadID, excluded) {
			active = true
		}
		reservationChanged := false
		for key, cancel := range c.turnCancels {
			if key.threadID == threadID && key.turnID != excludedTurnID && cancel != nil {
				active = true
				if cancelActive {
					if _, terminalActive := c.candidateTerminals[key]; terminalActive {
						continue
					}
					if _, alreadyReserved := c.cancelReservations[key]; alreadyReserved {
						continue
					}
					c.cancelReservations[key] = struct{}{}
					reservationChanged = true
				}
				cancels = append(cancels, cancel)
			}
		}
		if reservationChanged {
			c.notifyTurnStateChangedLocked()
		}
		if !active {
			c.mu.Unlock()
			return nil
		}
		changed := c.turnStateChanged
		c.mu.Unlock()
		if cancelActive {
			for _, cancel := range cancels {
				cancel()
			}
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Controller) CancelRegisteredTurn(threadID, turnID string) bool {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok {
		return false
	}
	c.mu.Lock()
	cancel := c.turnCancels[key]
	if cancel != nil {
		if _, terminalActive := c.candidateTerminals[key]; terminalActive {
			c.mu.Unlock()
			return false
		}
		c.cancelReservations[key] = struct{}{}
		c.notifyTurnStateChangedLocked()
	}
	c.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// CancelRegisteredTurnAndWait cancels one exact execution owner and waits
// until that owner has finished its provider/tool settlement and terminal
// persistence path. An interrupt caller must not independently publish a
// competing terminal while the registered owner is still active.
func (c *Controller) CancelRegisteredTurnAndWait(ctx context.Context, threadID, turnID string) (bool, error) {
	cancelled, release, err := c.ReserveInterruptTerminalAndWait(ctx, threadID, turnID)
	if release != nil {
		release()
	}
	return cancelled, err
}

// ReserveInterruptTerminalAndWait linearizes one interrupt terminal against
// provider candidate publication. The reservation is installed before an
// execution owner is inspected and remains held by the caller through its
// durable interrupt CAS. If a candidate permit won first, that candidate is
// allowed to finish; otherwise the registered owner is cancelled and no later
// candidate can acquire terminal publication authority.
func (c *Controller) ReserveInterruptTerminalAndWait(ctx context.Context, threadID, turnID string) (bool, func(), error) {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok {
		return false, nil, errors.Join(ErrMissingThreadID, ErrMissingTurnID)
	}
	if c == nil {
		return false, nil, ErrTerminalArbitration
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		c.mu.Lock()
		_, active := c.turnCancels[key]
		c.mu.Unlock()
		return active, nil, err
	}
	c.mu.Lock()
	if _, reserved := c.interruptReservations[key]; reserved {
		c.mu.Unlock()
		return false, nil, ErrTerminalArbitration
	}
	if c.threadTerminalActiveOtherThanLocked(key.threadID, key) {
		c.mu.Unlock()
		return false, nil, ErrTerminalArbitration
	}
	c.interruptReservations[key] = struct{}{}
	c.notifyTurnStateChangedLocked()
	cancel, exists := c.turnCancels[key]
	_, candidateWon := c.candidateTerminals[key]
	cancelled := exists && cancel != nil && !candidateWon
	changed := c.turnStateChanged
	c.mu.Unlock()
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { c.releaseInterruptReservation(key) })
	}
	// The interrupt intent blocks every later steer. Let an already-admitted
	// steering mutation finish its durable entry and event tail before the
	// execution is cancelled and before the interrupt terminal CAS can begin.
	// A candidate that won first remains the terminal owner and is allowed to
	// finish under the pre-existing candidate-first rule.
	if cancelled {
		for {
			c.mu.Lock()
			steerActive := c.threadSteerAdmissionActiveLocked(key.threadID)
			changed = c.turnStateChanged
			c.mu.Unlock()
			if !steerActive {
				break
			}
			select {
			case <-changed:
			case <-ctx.Done():
				release()
				return cancelled, nil, ctx.Err()
			}
		}
		cancel()
	}
	for {
		c.mu.Lock()
		_, ownerActive := c.turnCancels[key]
		_, candidateActive := c.candidateTerminals[key]
		steerActive := c.threadSteerAdmissionActiveLocked(key.threadID)
		changed = c.turnStateChanged
		c.mu.Unlock()
		if !ownerActive && !candidateActive && !steerActive {
			return cancelled, release, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			release()
			return cancelled, nil, ctx.Err()
		}
	}
}

// AcquireCandidateTerminal grants the exact registered execution permission
// to run one provider-originated terminal CAS. It shares Controller.mu with
// interrupt reservation, making their ordering a host-owned linearization
// decision rather than a check-then-CAS race.
func (c *Controller) AcquireCandidateTerminal(ctx context.Context, threadID, turnID, contextDigest string) (func(), error) {
	return c.acquireTerminal(ctx, threadID, turnID, contextDigest, false)
}

// AcquireCandidateTerminalForLoop places the candidate terminal claim before
// the loop's final durable steering promotion. A steer admission that entered
// first is allowed to finish, after which this method acquires the terminal
// claim and the loop promotes that already-durable input while no later steer
// can enter. The returned context carries an unforgeable in-process handoff so
// the existing publication layer can consume the same claim without a second
// arbitration gap.
func (c *Controller) AcquireCandidateTerminalForLoop(
	ctx context.Context,
	threadID, turnID, contextDigest string,
) (context.Context, func(), error) {
	key, ok := normalizedTurnKey(threadID, turnID)
	contextDigest = strings.TrimSpace(contextDigest)
	if !ok || contextDigest == "" || c == nil || ctx == nil {
		return nil, nil, ErrTerminalArbitration
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	if c.shuttingDown {
		c.mu.Unlock()
		return nil, nil, context.Canceled
	}
	if _, ownerActive := c.turnCancels[key]; !ownerActive {
		c.mu.Unlock()
		return nil, nil, ErrTerminalArbitration
	}
	if _, reserved := c.interruptReservations[key]; reserved {
		c.mu.Unlock()
		return nil, nil, context.Canceled
	}
	if _, cancelled := c.cancelReservations[key]; cancelled {
		c.mu.Unlock()
		return nil, nil, context.Canceled
	}
	if _, transitioning := c.threadTransitions[key.threadID]; transitioning {
		c.mu.Unlock()
		return nil, nil, ErrThreadTransition
	}
	if c.threadTerminalActiveLocked(key.threadID) {
		c.mu.Unlock()
		return nil, nil, ErrTerminalArbitration
	}
	// Install the intent before waiting so a continuous stream of later steer
	// requests cannot starve terminal publication. Admissions that already won
	// remain counted and are allowed to finish below.
	token := &candidateTerminalToken{controller: c, key: key, contextDigest: contextDigest}
	c.candidateTerminals[key] = contextDigest
	c.candidateTokens[key] = token
	c.notifyTurnStateChangedLocked()
	c.mu.Unlock()
	release := c.releaseTerminalClaim(key, contextDigest, token)
	if err := c.waitForSteerAdmissions(ctx, key, contextDigest, token); err != nil {
		release()
		return nil, nil, err
	}
	return context.WithValue(ctx, candidateTerminalContextKey{}, token), release, nil
}

// HasLiveCandidateTerminalForLoop observes the existing unconsumed handoff.
// It never acquires or consumes a claim, including for stale token contexts.
func (c *Controller) HasLiveCandidateTerminalForLoop(ctx context.Context, threadID, turnID, contextDigest string) bool {
	key, ok := normalizedTurnKey(threadID, turnID)
	contextDigest = strings.TrimSpace(contextDigest)
	if !ok || contextDigest == "" || c == nil || ctx == nil {
		return false
	}
	token, _ := ctx.Value(candidateTerminalContextKey{}).(*candidateTerminalToken)
	if token == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return token.controller == c && token.key == key && token.contextDigest == contextDigest &&
		c.candidateTerminals[key] == contextDigest && c.candidateTokens[key] == token && !token.consumed
}

// AcquireHostTerminal grants a deterministic host-coded fixed terminal the
// same CAS exclusion as a provider candidate. Unlike a candidate it may
// settle an earlier non-interrupt cancellation request; an interrupt
// reservation still wins and remains the sole terminal owner.
func (c *Controller) AcquireHostTerminal(ctx context.Context, threadID, turnID, contextDigest string) (func(), error) {
	return c.acquireTerminal(ctx, threadID, turnID, contextDigest, true)
}

func (c *Controller) acquireTerminal(ctx context.Context, threadID, turnID, contextDigest string, hostFixed bool) (func(), error) {
	key, ok := normalizedTurnKey(threadID, turnID)
	contextDigest = strings.TrimSpace(contextDigest)
	if !ok || contextDigest == "" || c == nil {
		return nil, ErrTerminalArbitration
	}
	if ctx == nil {
		return nil, ErrTerminalArbitration
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if token, _ := ctx.Value(candidateTerminalContextKey{}).(*candidateTerminalToken); token != nil {
		if token.controller == c && token.key == key && token.contextDigest == contextDigest &&
			c.candidateTerminals[key] == contextDigest && c.candidateTokens[key] == token && !token.consumed {
			token.consumed = true
			c.mu.Unlock()
			return func() {}, nil
		}
		// A context that carries one of this package's terminal tokens must never
		// fall back to a fresh acquisition. That would let a released/stale token
		// cross an ABA boundary and reclaim the same turn later.
		c.mu.Unlock()
		return nil, ErrTerminalArbitration
	}
	if c.shuttingDown && !hostFixed {
		c.mu.Unlock()
		return nil, context.Canceled
	}
	if _, ownerActive := c.turnCancels[key]; !ownerActive && !hostFixed {
		c.mu.Unlock()
		return nil, ErrTerminalArbitration
	}
	if _, reserved := c.interruptReservations[key]; reserved {
		c.mu.Unlock()
		return nil, context.Canceled
	}
	if _, cancelled := c.cancelReservations[key]; cancelled && !hostFixed {
		c.mu.Unlock()
		return nil, context.Canceled
	}
	if _, transitioning := c.threadTransitions[key.threadID]; transitioning && !hostFixed {
		c.mu.Unlock()
		return nil, ErrThreadTransition
	}
	if c.threadTerminalActiveLocked(key.threadID) {
		c.mu.Unlock()
		return nil, ErrTerminalArbitration
	}
	c.candidateTerminals[key] = contextDigest
	delete(c.candidateTokens, key)
	c.notifyTurnStateChangedLocked()
	c.mu.Unlock()
	release := c.releaseTerminalClaim(key, contextDigest, nil)
	if err := c.waitForSteerAdmissions(ctx, key, contextDigest, nil); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (c *Controller) waitForSteerAdmissions(
	ctx context.Context,
	key turnKey,
	contextDigest string,
	token *candidateTerminalToken,
) error {
	for {
		c.mu.Lock()
		if c.candidateTerminals[key] != contextDigest || c.candidateTokens[key] != token {
			c.mu.Unlock()
			return ErrTerminalArbitration
		}
		if !c.threadSteerAdmissionActiveLocked(key.threadID) {
			c.mu.Unlock()
			return nil
		}
		changed := c.turnStateChanged
		c.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Controller) releaseTerminalClaim(
	key turnKey,
	contextDigest string,
	token *candidateTerminalToken,
) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			if c.candidateTerminals[key] == contextDigest && c.candidateTokens[key] == token {
				delete(c.candidateTerminals, key)
				delete(c.candidateTokens, key)
				c.notifyTurnStateChangedLocked()
			}
			c.mu.Unlock()
		})
	}
}

// TurnInterruptReserved reports whether the interrupt endpoint owns the
// public terminal transition for this exact execution. The execution owner
// still settles provider/tool effects, but must not race that endpoint by
// persisting a generic cancellation failure.
func (c *Controller) TurnInterruptReserved(threadID, turnID string) bool {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok {
		return false
	}
	c.mu.Lock()
	_, reserved := c.interruptReservations[key]
	c.mu.Unlock()
	return reserved
}

// HostTerminalTakeoverReserved reports whether shutdown, an exact interrupt,
// or a thread security transition has already committed to closing this
// execution after its registered owner releases. A cancelled continuation
// owner transfers its terminal claim instead of racing that host path.
func (c *Controller) HostTerminalTakeoverReserved(threadID, turnID string) bool {
	key, ok := normalizedTurnKey(threadID, turnID)
	if !ok || c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shuttingDown {
		return true
	}
	if _, reserved := c.interruptReservations[key]; reserved {
		return true
	}
	_, transitioning := c.threadTransitions[key.threadID]
	return transitioning
}

func (c *Controller) releaseInterruptReservation(key turnKey) {
	c.mu.Lock()
	if _, exists := c.interruptReservations[key]; exists {
		delete(c.interruptReservations, key)
		c.notifyTurnStateChangedLocked()
	}
	c.mu.Unlock()
}

func (c *Controller) CancelAllRegisteredTurns() int {
	c.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(c.turnCancels))
	for key, cancel := range c.turnCancels {
		if cancel != nil {
			if _, terminalActive := c.candidateTerminals[key]; terminalActive {
				continue
			}
			c.cancelReservations[key] = struct{}{}
			cancels = append(cancels, cancel)
		}
	}
	if len(cancels) > 0 {
		c.notifyTurnStateChangedLocked()
	}
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

func (c *Controller) ActiveTurnCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.turnCancels)
}

func (c *Controller) notifyTurnStateChangedLocked() {
	close(c.turnStateChanged)
	c.turnStateChanged = make(chan struct{})
}

func (c *Controller) threadTerminalActiveLocked(threadID string) bool {
	for key := range c.candidateTerminals {
		if key.threadID == threadID {
			return true
		}
	}
	for key := range c.interruptReservations {
		if key.threadID == threadID {
			return true
		}
	}
	return false
}

func (c *Controller) threadTerminalActiveOtherThanLocked(threadID string, excluded turnKey) bool {
	for key := range c.candidateTerminals {
		if key.threadID == threadID && key != excluded {
			return true
		}
	}
	for key := range c.interruptReservations {
		if key.threadID == threadID && key != excluded {
			return true
		}
	}
	return false
}

func (c *Controller) threadSteerAdmissionActiveOtherThanLocked(threadID string, excluded turnKey) bool {
	for key, count := range c.steerAdmissions {
		if key.threadID == threadID && key != excluded && count > 0 {
			return true
		}
	}
	return false
}

func (c *Controller) threadSteerAdmissionActiveLocked(threadID string) bool {
	for key, count := range c.steerAdmissions {
		if key.threadID == threadID && count > 0 {
			return true
		}
	}
	return false
}

func (c *Controller) requireDriver() error {
	if c == nil || c.driver == nil {
		return ErrNoDriver
	}
	return nil
}

func normalizedTurnKey(threadID, turnID string) (turnKey, bool) {
	threadID = strings.TrimSpace(threadID)
	turnID = strings.TrimSpace(turnID)
	if threadID == "" || turnID == "" {
		return turnKey{}, false
	}
	return turnKey{threadID: threadID, turnID: turnID}, true
}
