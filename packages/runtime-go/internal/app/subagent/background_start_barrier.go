package subagent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// BackgroundJobStartBarrier is the per-job linearization point between the
// final cancellation check and the first external job effect, including a
// child turn or operating-system process start. Cancellation and start never
// hold RuntimeState.mu or the context effect gate while they wait for this
// lock, avoiding a reverse lock order with context transitions.
type BackgroundJobStartBarrier struct {
	mu       sync.Mutex
	canceled atomic.Bool
	consumed bool
}

func NewBackgroundJobStartBarrier() *BackgroundJobStartBarrier {
	return &BackgroundJobStartBarrier{}
}

// reservePendingSteer holds the same boundary as the first-turn claim/start.
// A start that already won is unavailable here; waiting for the whole running
// turn would deadlock its own steering preparation.
func (barrier *BackgroundJobStartBarrier) reservePendingSteer(ctx context.Context) (func(), error) {
	if barrier == nil || ctx == nil || ctx.Err() != nil || !barrier.mu.TryLock() {
		return nil, errors.New("pending child steer start authority is unavailable")
	}
	if barrier.canceled.Load() || barrier.consumed || ctx.Err() != nil {
		barrier.mu.Unlock()
		return nil, errors.New("pending child steer start authority is closed")
	}
	var once sync.Once
	return func() { once.Do(barrier.mu.Unlock) }, nil
}

func (barrier *BackgroundJobStartBarrier) StartIfActive(ctx context.Context, start func() error) error {
	if barrier == nil {
		return errors.New("job start barrier is unavailable")
	}
	if start == nil {
		return errors.New("job start callback is unavailable")
	}
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if barrier.canceled.Load() {
		return context.Canceled
	}
	if ctx == nil {
		return errors.New("background job process start context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if barrier.consumed {
		return errors.New("job start authority was already consumed")
	}
	barrier.consumed = true
	return start()
}

// Cancel closes start authority without waiting for a start that already won.
// The atomic store is the cancellation linearization point: a later start sees
// it and fails, while a start that already observed the open state may finish
// StartTracked before the owning worker acknowledges termination.
func (barrier *BackgroundJobStartBarrier) Cancel() {
	if barrier == nil {
		return
	}
	barrier.canceled.Store(true)
}

// CancelAndWait closes authority and waits for any already-authorized
// StartTracked call to return. RuntimeState uses this only while unregistering
// the worker, before it closes the job's done acknowledgement.
func (barrier *BackgroundJobStartBarrier) CancelAndWait() {
	if barrier == nil {
		return
	}
	barrier.canceled.Store(true)
	barrier.mu.Lock()
	barrier.mu.Unlock()
}
