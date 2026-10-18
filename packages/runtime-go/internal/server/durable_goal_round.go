package server

import (
	"errors"
	"time"

	goalapp "analytix.local/runtime-go/internal/app/goal"
	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
)

func (s *DurableEventSessionStore) AppendGoalTurnIfBaselineV1(threadID string, turn map[string]any, providerID string, patch map[string]any, workspace, digest string, attempt domaingoal.RoundAdmissionV1, prompt string, authorizePrimary func(func() error) error) (turnstartapp.GoalAppendOutcomeV1, error) {
	var result turnstartapp.GoalAppendOutcomeV1
	if digest == "" || attempt == nil || attempt.ThreadID() != threadID || authorizePrimary == nil {
		return result, turnstartapp.ErrBaselineConflict
	}
	err := attempt.WithAdmission(func() (bool, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.settleGeneralTerminalPublicationsBeforeHistoryMutationNoLock(threadID); err != nil {
			return false, err
		}
		thread, err := s.goalPrimaryReadV1(threadID)
		if err != nil {
			return false, err
		}
		// The stable turn/source witness is checked before running/baseline
		// predicates. An exact duplicate cannot append or consume another round.
		if turnstartapp.GoalPrimaryWitnessV1(thread, turn, attempt.Source(), prompt) {
			result.Committed = true
			return true, nil
		}
		next, err := turnstartapp.ApplyAppendMutation(turnstartapp.AppendMutationInput{Thread: thread, Turn: turn, ThreadID: threadID, ProviderID: providerID, ThreadPatch: patch, ExpectedWorkspace: workspace, ExpectedDigest: digest, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
		if err != nil {
			return false, err
		}
		next, err = turnstartapp.ApplyGoalAdmissionV1(next, turn, attempt.Source(), prompt)
		if err != nil {
			return false, err
		}
		previousGoal, _ := thread["goal"].(map[string]any)
		nextGoal, _ := next["goal"].(map[string]any)
		goalapp.AppendHistoryV1(next, previousGoal, nextGoal, "round_started", stringField(turn, "createdAt"))
		writeErr := s.upsertThreadNoLockWithAtomicWriteAuthority(next, false, authorizePrimary)
		primary, readErr := s.goalPrimaryReadV1(threadID)
		result.Committed = readErr == nil && turnstartapp.GoalPrimaryWitnessV1(primary, turn, attempt.Source(), prompt)
		if result.Committed && writeErr != nil {
			result.RecoveryRequired = true
			return true, errors.Join(errors.New("Goal primary committed; sidecar recovery required"), writeErr)
		}
		if !result.Committed {
			return false, errors.Join(errors.New("Goal primary admission was not confirmed; automatic retry is disarmed"), writeErr, readErr)
		}
		return true, nil
	})
	return result, err
}
