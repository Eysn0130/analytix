package control

import (
	"context"
	"time"
)

// TurnExecutionOwnerV1 identifies one in-process execution registration. It is
// a completion observation, never execution or durable continuation authority.
// Only an owned registration can place this identity in a worker context.
type TurnExecutionOwnerV1 struct {
	controller *Controller
	key        turnKey
	done       chan struct{}
	owned      bool
}

type turnExecutionOwnerContextKeyV1 struct{}

// RegisterTurnCancelOwnedV1 preserves the ordinary admission guards and binds
// the successful registration's identity to the returned worker context.
func (c *Controller) RegisterTurnCancelOwnedV1(ctx context.Context, threadID, turnID string, cancel context.CancelFunc) (context.Context, *TurnExecutionOwnerV1, error) {
	owner, err := c.registerTurnExecutionOwnerV1(ctx, threadID, turnID, cancel, true)
	if err != nil {
		return nil, nil, err
	}
	return context.WithValue(ctx, turnExecutionOwnerContextKeyV1{}, owner), owner, nil
}

// CaptureTurnExecutionOwnerV1 requires the producer's real worker context. A
// matching public key or a legacy registration cannot manufacture this proof.
func (c *Controller) CaptureTurnExecutionOwnerV1(ctx context.Context, threadID, turnID string) (*TurnExecutionOwnerV1, error) {
	if c == nil || ctx == nil {
		return nil, ErrTurnExecutionConflict
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	owner, _ := ctx.Value(turnExecutionOwnerContextKeyV1{}).(*TurnExecutionOwnerV1)
	if owner == nil || owner.controller != c || !owner.MatchesTurn(threadID, turnID) {
		return nil, ErrTurnExecutionConflict
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.turnOwners[owner.key] != owner || c.turnCancels[owner.key] == nil {
		return nil, ErrTurnExecutionConflict
	}
	return owner, nil
}

// MatchesTurn checks only the immutable registration identity. A completed
// owner's identity stays valid for the gate that captured it before release.
func (owner *TurnExecutionOwnerV1) MatchesTurn(threadID, turnID string) bool {
	key, ok := normalizedTurnKey(threadID, turnID)
	return ok && owner != nil && owner.owned && owner.controller != nil && owner.done != nil && owner.key == key
}

// Done closes only when the matching execution registration is released.
// Calling its cancel function alone does not signal completion.
func (owner *TurnExecutionOwnerV1) Done() <-chan struct{} {
	if owner == nil || !owner.owned || owner.controller == nil {
		return nil
	}
	return owner.done
}

// WaitBounded observes the exact producer's completion without cancelling it
// or acquiring an effect/gate claim. It preserves the caller's context and
// limits the handoff independently of its deadline; self-wait is rejected.
func (owner *TurnExecutionOwnerV1) WaitBounded(ctx context.Context, maximum time.Duration) error {
	if maximum <= 0 {
		return ErrTurnExecutionConflict
	}
	if owner == nil || !owner.owned || owner.controller == nil || owner.done == nil || ctx == nil {
		return ErrTurnExecutionConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if current, _ := ctx.Value(turnExecutionOwnerContextKeyV1{}).(*TurnExecutionOwnerV1); current == owner {
		return ErrTurnExecutionConflict
	}
	expires := time.Now().Add(maximum)
	timer := time.NewTimer(maximum)
	defer timer.Stop()
	select {
	case <-owner.done:
		if err := ctx.Err(); err != nil {
			return err
		}
		if !time.Now().Before(expires) {
			return context.DeadlineExceeded
		}
		return nil
	case <-timer.C:
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.DeadlineExceeded
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseTurnExecutionOwnerV1 cannot remove a replacement execution or its
// cancellation reservation, even when it uses the same thread/turn key.
func (c *Controller) ReleaseTurnExecutionOwnerV1(owner *TurnExecutionOwnerV1) bool {
	if c == nil || owner == nil || owner.controller != c || !owner.owned {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.releaseTurnExecutionOwnerLockedV1(owner)
}

func (c *Controller) installTurnExecutionOwnerLockedV1(key turnKey, cancel context.CancelFunc, owned bool) *TurnExecutionOwnerV1 {
	owner := &TurnExecutionOwnerV1{controller: c, key: key, done: make(chan struct{}), owned: owned}
	c.turnCancels[key] = cancel
	c.turnOwners[key] = owner
	c.notifyTurnStateChangedLocked()
	return owner
}

func (c *Controller) releaseTurnExecutionOwnerLockedV1(owner *TurnExecutionOwnerV1) bool {
	if owner == nil || owner.controller != c || c.turnOwners[owner.key] != owner || c.turnCancels[owner.key] == nil {
		return false
	}
	delete(c.turnCancels, owner.key)
	delete(c.turnOwners, owner.key)
	delete(c.cancelReservations, owner.key)
	close(owner.done)
	c.notifyTurnStateChangedLocked()
	return true
}
