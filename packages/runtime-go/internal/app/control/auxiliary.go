package control

import (
	"context"
	"strings"
	"sync"
)

type auxiliaryOperation struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// BeginAuxiliary owns the idle thread until the caller has durably closed its
// provider audit. It creates neither a foreground turn nor a chat item.
func (c *Controller) BeginAuxiliary(ctx context.Context, threadID string) (context.Context, func(), error) {
	if c == nil || ctx == nil || ctx.Err() != nil || strings.TrimSpace(threadID) == "" {
		return ctx, nil, ErrTurnExecutionConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shuttingDown {
		return ctx, nil, ErrRuntimeShuttingDown
	}
	if c.auxiliary[threadID] != nil || c.foregroundPreparing[threadID] > 0 || c.threadTerminalActiveLocked(threadID) {
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
	if c == nil || ctx == nil || strings.TrimSpace(threadID) == "" {
		return nil, ErrTurnExecutionConflict
	}
	c.mu.Lock()
	if c.shuttingDown {
		c.mu.Unlock()
		return nil, ErrRuntimeShuttingDown
	}
	c.foregroundPreparing[threadID]++
	operation := c.auxiliary[threadID]
	c.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			c.mu.Lock()
			c.foregroundPreparing[threadID]--
			if c.foregroundPreparing[threadID] == 0 {
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
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}
