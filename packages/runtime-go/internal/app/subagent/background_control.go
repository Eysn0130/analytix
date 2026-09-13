package subagent

import (
	"context"
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

// BoundChildAdmission owns the cancellation and one-shot start authority for
// a child run. Foreground children inherit the parent context; background
// children use a detached context and may transfer cleanup to their worker.
type BoundChildAdmission struct {
	state  *RuntimeState
	jobID  string
	ctx    context.Context
	cancel context.CancelFunc
	start  *BackgroundJobStartBarrier
	owned  bool
}

// BoundBackgroundAdmission is retained as a source-compatible name for the
// background-only constructor below.
type BoundBackgroundAdmission = BoundChildAdmission

// BeginBoundChildAdmission registers both foreground and background child
// runs before child-thread preparation. The background path deliberately
// detaches from the provider tool context; the foreground path preserves it,
// including the parent effect lease.
func BeginBoundChildAdmission(parent context.Context, background bool, state *RuntimeState, jobID string, binding *domainjob.SecurityBinding) (*BoundChildAdmission, context.Context, error) {
	jobID = strings.TrimSpace(jobID)
	if state == nil || jobID == "" || domainjob.ValidateSecurityBinding(binding) != nil {
		return nil, parent, errors.New("child job context authority is invalid")
	}
	if parent == nil {
		return nil, parent, errors.New("child job parent context is required")
	}
	base := parent
	if background {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	startBarrier, registered := state.registerBackgroundJob(parent, jobID, binding, cancel)
	if !registered || startBarrier == nil {
		return nil, parent, errors.New("child job context authority changed before registration")
	}
	control := &BoundChildAdmission{state: state, jobID: jobID, ctx: ctx, cancel: cancel, start: startBarrier, owned: true}
	return control, ctx, nil
}

func BeginOptionalBoundBackgroundAdmission(parent context.Context, enabled bool, state *RuntimeState, jobID string, binding *domainjob.SecurityBinding) (*BoundBackgroundAdmission, context.Context, error) {
	if !enabled {
		return &BoundBackgroundAdmission{}, parent, nil
	}
	return BeginBoundChildAdmission(parent, true, state, jobID, binding)
}

func (control *BoundChildAdmission) StartIfActive(ctx context.Context, start func() error) error {
	if control == nil || control.start == nil {
		return errors.New("child turn start authority is unavailable")
	}
	return control.start.StartIfActive(ctx, start)
}

func (control *BoundChildAdmission) Close() {
	if control == nil || !control.owned {
		return
	}
	control.owned = false
	control.start.Cancel()
	control.cancel()
	control.state.UnregisterBackgroundJob(control.jobID)
}

func (control *BoundChildAdmission) TransferCleanup() func() {
	if control == nil || !control.owned {
		return func() {}
	}
	control.owned = false
	return func() {
		control.start.Cancel()
		control.cancel()
		control.state.UnregisterBackgroundJob(control.jobID)
	}
}
