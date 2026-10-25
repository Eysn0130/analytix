package server

import (
	"context"
	"errors"
	"time"

	liveevents "analytix.local/runtime-go/internal/adapters/outbound/liveevents"
	controlapp "analytix.local/runtime-go/internal/app/control"
	goalapp "analytix.local/runtime-go/internal/app/goal"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func (h *runtimeServerHandler) newGoalRoundDriverV1() *goalapp.RoundDriverV1 {
	return goalapp.NewRoundDriverV1(goalapp.DriverDependenciesV1{
		ReadGoal: h.store.GetGoal,
		Ready: func(threadID string) bool {
			if !h.runtimeControl().GoalIdleV1(threadID) {
				return false
			}
			thread, err := h.store.GetThread(threadID)
			if err != nil || thread == nil || stringField(thread, "status") != "idle" {
				return false
			}
			if sensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread); err != nil || sensitive {
				return false
			}
			approvals, inputs, gateErr := h.pendingGateIDsFromReplay(threadID)
			return gateErr == nil && len(approvals) == 0 && len(inputs) == 0
		},
		Send: func(ctx context.Context, threadID string, attempt *goalapp.RoundAttemptV1) error {
			_, err := h.runtimeControl().SendTurn(ctx, controlapp.StartTurnRequest{ThreadID: threadID, Prompt: attempt.Prompt(), Async: true, InternalGoalRound: attempt})
			return err
		},
		Deferred:    func(err error) bool { return errors.Is(err, controlapp.ErrTurnExecutionConflict) },
		BlockBudget: h.store.blockGoalRoundBudgetV1,
		Publish: func(threadID string, activation *domaingoal.ActivationV1) {
			h.store.publishGoalActivationV1(threadID, activation)
		},
	})
}

func (s *DurableEventSessionStore) blockGoalRoundBudgetV1(threadID string, expected domaingoal.RefV1) (map[string]any, error) {
	thread, err := s.mutateGoalLifecycleThreadV1(threadID, func(thread map[string]any, now string) error {
		goal, _ := thread["goal"].(map[string]any)
		ref, maximum, started, known := domaingoal.CountedRecordV1(goal)
		if !known || ref != expected || started < maximum || stringField(goal, "status") != "active" {
			return goalapp.ErrGoalRefConflictV1
		}
		next, err := goalapp.ApplyStatePatch(goalapp.StatePatchInput{ThreadID: threadID, Existing: goal, Patch: map[string]any{"status": "blocked", "blockedReason": "Goal round budget exhausted"}, Now: now})
		if err != nil {
			return err
		}
		thread["goal"] = next
		goalapp.AppendHistoryV1(thread, goal, next, "round_limit", now)
		return nil
	})
	return clonedGoalTodoField(thread, "goal"), err
}

func (s *DurableEventSessionStore) publishGoalActivationV1(threadID string, activation *domaingoal.ActivationV1) {
	event := goalActivationEventV1(threadID, activation)
	s.mu.Lock()
	defer s.mu.Unlock()
	// No durable seq, event log write, replay or publication-order claim.
	// A full queue must reconnect and receive the attach snapshot; silently
	// losing a stop signal would leave a previously armed display stale.
	liveevents.BroadcastBundleNoPrefix(s.subscribers[threadID], []map[string]any{event}, cloneMap)
	if len(s.subscribers[threadID]) == 0 {
		delete(s.subscribers, threadID)
	}
}

func goalActivationEventV1(threadID string, activation *domaingoal.ActivationV1) map[string]any {
	event := map[string]any{"kind": "goal_activation_changed", "threadId": threadID, "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "activation": nil}
	if activation != nil {
		event["activation"] = map[string]any{"id": activation.ID, "revision": activation.Revision, "activation": activation.Activation}
	}
	return event
}

func (h *runtimeServerHandler) initialGoalActivationV1(threadID string) map[string]any {
	state, err := h.runtimeThreadService().GetGoalStateV1(threadID)
	if err != nil {
		return nil
	}
	activation, _ := state["activation"].(*domaingoal.ActivationV1)
	return goalActivationEventV1(threadID, activation)
}

func (h *runtimeServerHandler) goalTurnStoppedV1(threadID, turnID string) bool {
	thread, err := h.store.GetThread(threadID)
	if err != nil {
		return true
	}
	for _, raw := range listAny(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") != turnID {
			continue
		}
		reason, _ := turn["endReason"].(map[string]any)
		return stringField(turn, "status") != "completed" || stringField(reason, "kind") == "max-tokens"
	}
	return true
}
