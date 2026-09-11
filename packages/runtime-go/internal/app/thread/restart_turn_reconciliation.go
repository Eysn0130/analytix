package thread

import (
	"errors"

	controlapp "analytix.local/runtime-go/internal/app/control"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	"analytix.local/runtime-go/internal/contracts"
)

type RestartTurnReconciliationDependenciesV1 struct {
	GetThread         func(string) (map[string]any, error)
	OwnsRestartTurn   func(threadID, turnID string) bool
	ReconcileTerminal func(threadID, turnID string, outcomes executiongrantapp.RestartGrantOutcomesV1) error
	AbortActive       func(threadID, turnID string, outcomes executiongrantapp.RestartGrantOutcomesV1) error
}

func AbortStaleRuntimeTurnsAfterRestartV1(
	threadIDs []string,
	pendingTurnKeys map[string]bool,
	restartGrantOutcomes map[string]executiongrantapp.RestartGrantOutcomesV1,
	deps RestartTurnReconciliationDependenciesV1,
) error {
	if deps.GetThread == nil || deps.OwnsRestartTurn == nil ||
		deps.ReconcileTerminal == nil || deps.AbortActive == nil {
		return errors.New("restart turn reconciliation authority is unavailable")
	}
	for _, threadID := range threadIDs {
		thread, err := deps.GetThread(threadID)
		if err != nil {
			return err
		}
		turns, _ := thread["turns"].([]any)
		for _, rawTurn := range turns {
			turn, _ := rawTurn.(map[string]any)
			turnID := contracts.StringField(turn, "id")
			if turnID == "" || deps.OwnsRestartTurn(threadID, turnID) {
				continue
			}
			turnKey := controlapp.TurnKey(threadID, turnID)
			outcomes := restartGrantOutcomes[turnKey]
			if pendingTurnKeys[turnKey] {
				if len(outcomes) > 0 {
					return errors.New("outcome-unknown write dispatch conflicts with a resumable gate")
				}
				continue
			}
			status := contracts.StringField(turn, "status")
			if status != "running" && status != "queued" && status != "waiting" {
				if len(outcomes) > 0 {
					if err := deps.ReconcileTerminal(threadID, turnID, outcomes); err != nil {
						return err
					}
					delete(restartGrantOutcomes, turnKey)
				}
				continue
			}
			if err := deps.AbortActive(threadID, turnID, outcomes); err != nil {
				return err
			}
			delete(restartGrantOutcomes, turnKey)
		}
	}
	if len(restartGrantOutcomes) != 0 {
		return errors.New("outcome-unknown write dispatch has no exact restorable turn")
	}
	return nil
}
