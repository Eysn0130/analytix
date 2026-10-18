package control

import (
	"context"
	"errors"
	"strings"
	"sync"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type auxiliaryOperation struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type foregroundPreparation struct {
	count   int
	current *ForegroundReservation
}

// ForegroundReservation is an in-process witness to the existing admission,
// never a wire credential or a new permission. Every prepare replaces its
// generation; release and cancellation revoke it.
type ForegroundReservation struct {
	controller *Controller
	threadID   string
	context    context.Context
	active     bool // guarded by controller.mu
}

func (*ForegroundReservation) MarshalJSON() ([]byte, error) {
	return nil, errors.New("foreground reservation is process-local")
}

func (witness *ForegroundReservation) Validate(ctx context.Context, threadID string) error {
	if witness == nil || witness.controller == nil {
		return ErrTurnExecutionConflict
	}
	return witness.controller.ValidateForegroundReservation(ctx, threadID, witness)
}

// WithCurrentV1 serializes revocation with the primary write only. Callers
// already own their transition, Goal admission and durable mutation locks;
// the callback must not acquire those owners or call back into Controller.
func (witness *ForegroundReservation) WithCurrentV1(ctx context.Context, threadID string, write func() error) error {
	if witness == nil || witness.controller == nil || write == nil {
		return ErrTurnExecutionConflict
	}
	c := witness.controller
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.reservationCurrentLocked(ctx, threadID, witness) {
		return ErrTurnExecutionConflict
	}
	return write()
}

func (c *Controller) reservationCurrentLocked(ctx context.Context, threadID string, witness *ForegroundReservation) bool {
	preparation := c.foregroundPreparing[threadID]
	return ctx != nil && ctx.Err() == nil && witness != nil && witness.controller == c &&
		witness.threadID == threadID && witness.active && witness.context.Err() == nil &&
		preparation != nil && preparation.count == 1 && preparation.current == witness &&
		!c.shuttingDown && !c.maintenance
}

func (c *Controller) ValidateForegroundReservation(ctx context.Context, threadID string, witness *ForegroundReservation) error {
	if c == nil {
		return ErrTurnExecutionConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.reservationCurrentLocked(ctx, threadID, witness) {
		return ErrTurnExecutionConflict
	}
	return nil
}

// BeginAuxiliary owns the idle thread until the caller has durably closed its
// provider audit. It creates neither a foreground turn nor a chat item.
func (c *Controller) BeginAuxiliary(ctx context.Context, threadID string) (context.Context, func(), error) {
	return c.beginAuxiliary(ctx, threadID, nil)
}

// BeginAuxiliaryForForeground uses only the same live general admission. The
// generic idle-thread admission above remains closed during preparation.
func (c *Controller) BeginAuxiliaryForForeground(ctx context.Context, current domainsecurity.TurnSecurityContext, witness *ForegroundReservation) (context.Context, func(), error) {
	if witness == nil || domainsecurity.ValidateTurnSecurityContextForExecution(current) != nil || !domainsecurity.TurnSecurityContextIsGeneral(current) {
		return ctx, nil, ErrTurnExecutionConflict
	}
	return c.beginAuxiliary(ctx, current.ThreadID, witness)
}

func (c *Controller) beginAuxiliary(ctx context.Context, threadID string, witness *ForegroundReservation) (context.Context, func(), error) {
	if c == nil || ctx == nil || ctx.Err() != nil || strings.TrimSpace(threadID) == "" {
		return ctx, nil, ErrTurnExecutionConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shuttingDown || c.maintenance {
		return ctx, nil, ErrRuntimeShuttingDown
	}
	if witness != nil && !c.reservationCurrentLocked(ctx, threadID, witness) {
		return ctx, nil, ErrTurnExecutionConflict
	}
	if c.auxiliary[threadID] != nil || (witness == nil && c.foregroundPreparing[threadID] != nil) || c.threadTerminalActiveLocked(threadID) {
		return ctx, nil, ErrTurnExecutionConflict
	}
	if _, active := c.threadTransitions[threadID]; active {
		return ctx, nil, ErrTurnExecutionConflict
	}
	for key := range c.turnCancels {
		if key.threadID == threadID {
			return ctx, nil, ErrTurnExecutionConflict
		}
	}
	child, cancel := context.WithCancel(ctx)
	operation := &auxiliaryOperation{cancel: cancel, done: make(chan struct{})}
	c.auxiliary[threadID] = operation
	if c.activeTurnOperations == 0 {
		c.idle = make(chan struct{})
	}
	c.activeTurnOperations++
	c.notifyTurnStateChangedLocked()
	var once sync.Once
	return child, func() {
		once.Do(func() {
			cancel()
			c.mu.Lock()
			delete(c.auxiliary, threadID)
			close(operation.done)
			c.notifyTurnStateChangedLocked()
			c.mu.Unlock()
			c.finishTurnOperation()
		})
	}, nil
}

// PrepareForeground closes auxiliary admission before cancelling/waiting.
// Keep its reservation through foreground registration, avoiding a gap in
// which another auxiliary producer could start during baseline preparation.
func (c *Controller) PrepareForeground(ctx context.Context, threadID string) (func(), error) {
	_, release, err := c.PrepareForegroundWithWitness(ctx, threadID)
	return release, err
}

func (c *Controller) PrepareForegroundWithWitness(ctx context.Context, threadID string) (*ForegroundReservation, func(), error) {
	return c.prepareForegroundWithWitnessV1(ctx, threadID, false)
}

func (c *Controller) PrepareGoalForegroundWithWitnessV1(ctx context.Context, threadID string) (*ForegroundReservation, func(), error) {
	return c.prepareForegroundWithWitnessV1(ctx, threadID, true)
}

func (c *Controller) GoalIdleV1(threadID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.goalIdleLockedV1(threadID)
}

func (c *Controller) goalIdleLockedV1(threadID string) bool {
	if c.shuttingDown || c.maintenance || c.foregroundPreparing[threadID] != nil || c.auxiliary[threadID] != nil || c.threadTerminalActiveLocked(threadID) {
		return false
	}
	if _, present := c.threadTransitions[threadID]; present {
		return false
	}
	for key := range c.turnCancels {
		if key.threadID == threadID {
			return false
		}
	}
	for key := range c.interruptReservations {
		if key.threadID == threadID {
			return false
		}
	}
	for key := range c.cancelReservations {
		if key.threadID == threadID {
			return false
		}
	}
	for key, count := range c.steerAdmissions {
		if key.threadID == threadID && count != 0 {
			return false
		}
	}
	return true
}

func (c *Controller) prepareForegroundWithWitnessV1(ctx context.Context, threadID string, goalOnly bool) (*ForegroundReservation, func(), error) {
	if c == nil || ctx == nil || strings.TrimSpace(threadID) == "" {
		return nil, nil, ErrTurnExecutionConflict
	}
	c.mu.Lock()
	if c.shuttingDown || c.maintenance {
		c.mu.Unlock()
		return nil, nil, ErrRuntimeShuttingDown
	}
	if goalOnly && !c.goalIdleLockedV1(threadID) {
		c.mu.Unlock()
		return nil, nil, ErrTurnExecutionConflict
	}
	preparation := c.foregroundPreparing[threadID]
	if preparation == nil {
		preparation = &foregroundPreparation{}
		c.foregroundPreparing[threadID] = preparation
	}
	if preparation.current != nil {
		preparation.current.active = false
	}
	witness := &ForegroundReservation{controller: c, threadID: threadID, context: ctx, active: true}
	preparation.current = witness
	preparation.count++
	operation := c.auxiliary[threadID]
	c.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			c.mu.Lock()
			witness.active = false
			preparation.count--
			if preparation.count == 0 {
				delete(c.foregroundPreparing, threadID)
			}
			c.notifyTurnStateChangedLocked()
			c.mu.Unlock()
		})
	}
	if operation != nil {
		operation.cancel()
		select {
		case <-operation.done:
		case <-ctx.Done():
			release()
			return nil, nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, nil, err
	}
	return witness, release, nil
}
