package server

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// runtimeThreadTransition retains the Controller admission reservation for the
// complete durable security transition, not merely for effect-writer acquire.
// This prevents a late approval, input response, or new turn from registering
// between quiescence and epoch commit.
type runtimeThreadTransition struct {
	inner   threadapp.CompactionTransition
	release func()
	once    sync.Once
}

func (transition *runtimeThreadTransition) Prepare(ctx context.Context, current domainsecurity.TurnSecurityContext, timeoutDuration time.Duration) error {
	if transition == nil || transition.inner == nil {
		return errors.New("runtime thread transition is unavailable")
	}
	return transition.inner.Prepare(ctx, current, timeoutDuration)
}

func (transition *runtimeThreadTransition) Commit() error {
	if transition == nil || transition.inner == nil {
		return errors.New("runtime thread transition is unavailable")
	}
	if err := transition.inner.Commit(); err != nil {
		return err
	}
	transition.finish()
	return nil
}

func (transition *runtimeThreadTransition) Abort() {
	if transition == nil {
		return
	}
	if transition.inner != nil {
		transition.inner.Abort()
	}
	transition.finish()
}

func (transition *runtimeThreadTransition) finish() {
	transition.once.Do(func() {
		if transition.release != nil {
			transition.release()
		}
	})
}

func (h *runtimeServerHandler) beginRuntimeThreadContextTransition(
	ctx context.Context,
	target domainsecurity.TurnSecurityContext,
) (threadapp.CompactionTransition, error) {
	release, err := h.reserveRuntimeThreadTransition(target.ThreadID)
	if err != nil {
		return nil, err
	}
	inner, err := h.runtimeSubagentState().BeginSecurityContextTransitionWithBarrier(ctx, target, func() error {
		return h.quiesceRuntimeThreadForMutation(ctx, target.ThreadID)
	})
	if err != nil {
		release()
		return nil, err
	}
	return &runtimeThreadTransition{inner: inner, release: release}, nil
}

func (h *runtimeServerHandler) beginRuntimeThreadScopeTransition(
	ctx context.Context,
	threadID, workspaceRealPath, tenantID, userID string,
) (threadapp.CompactionTransition, error) {
	release, err := h.reserveRuntimeThreadTransition(threadID)
	if err != nil {
		return nil, err
	}
	inner, err := h.runtimeSubagentState().BeginSecurityScopeTransitionIdentityWithBarrier(
		ctx, threadID, workspaceRealPath, tenantID, userID,
		func() error { return h.quiesceRuntimeThreadForMutation(ctx, threadID) },
	)
	if err != nil {
		release()
		return nil, err
	}
	return &runtimeThreadTransition{inner: inner, release: release}, nil
}

func (h *runtimeServerHandler) beginRuntimeWorkspaceScopeTransition(
	ctx context.Context,
	threadID, previousWorkspaceRealPath, targetWorkspaceRealPath, tenantID, userID string,
	barrier func() error,
) (threadapp.CompactionTransition, error) {
	release, err := h.reserveRuntimeThreadTransition(threadID)
	if err != nil {
		return nil, err
	}
	inner, err := h.runtimeSubagentState().BeginWorkspaceSecurityScopeTransition(
		ctx, threadID, previousWorkspaceRealPath, targetWorkspaceRealPath, tenantID, userID,
		func() error {
			if err := h.quiesceRuntimeThreadForMutation(ctx, threadID); err != nil {
				return err
			}
			if barrier != nil {
				return barrier()
			}
			return nil
		},
	)
	if err != nil {
		release()
		return nil, err
	}
	return &runtimeThreadTransition{inner: inner, release: release}, nil
}

func (h *runtimeServerHandler) reserveRuntimeThreadTransition(threadID string) (func(), error) {
	if h == nil || strings.TrimSpace(threadID) == "" {
		return nil, errors.New("runtime thread transition authority is unavailable")
	}
	return h.runtimeControl().ReserveThreadTransition(threadID)
}

func (h *runtimeServerHandler) quiesceRuntimeThreadForMutation(ctx context.Context, threadID string) error {
	return h.quiesceRuntimeThread(ctx, threadID, "", "security_context_transition")
}

func (h *runtimeServerHandler) quiesceRuntimeThreadForTurnStart(ctx context.Context, threadID, currentTurnID string) error {
	return h.quiesceRuntimeThread(ctx, threadID, currentTurnID, "turn_start_security_transition")
}

func (h *runtimeServerHandler) quiesceRuntimeThread(ctx context.Context, threadID, excludedTurnID, reason string) error {
	if h == nil || strings.TrimSpace(threadID) == "" {
		return errors.New("runtime thread quiescence authority is unavailable")
	}
	var quiesceErr error
	if strings.TrimSpace(excludedTurnID) == "" {
		quiesceErr = h.runtimeControl().CancelThreadTurnsAndWait(ctx, threadID)
	} else {
		quiesceErr = h.runtimeControl().CancelThreadTurnsAndWaitExcept(ctx, threadID, excludedTurnID)
	}
	if quiesceErr != nil {
		return quiesceErr
	}
	drained := h.gates.DrainForThread(threadID)
	if len(drained) == 0 {
		return nil
	}
	hostContext, cancel := newHostAuthorityContextFrom(ctx)
	err := gatecontinuationapp.CloseDrainedForTerminal(
		hostContext, drained, reason, context.Canceled, h.runtimeGateContinuationDependencies(),
	)
	cancel()
	if err != nil && !h.gates.RestoreDrained(drained) {
		err = errors.Join(err, errors.New("security transition gate settlement claim could not be retained"))
	}
	if err == nil && !h.gates.CommitDrained(drained) {
		err = errors.New("security transition gate settlement claim could not be committed")
	}
	return err
}
