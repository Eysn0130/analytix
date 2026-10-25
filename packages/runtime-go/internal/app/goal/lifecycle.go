package goal

import (
	"errors"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
)

var ErrGoalRefConflictV1 = errors.New("goal reference changed; refresh before trying again")
var ErrGoalRoundLimitV1 = errors.New("goal round budget exhausted; explicitly increase maxGoalRounds before resuming")
var ErrGoalCommittedRecoveryV1 = errors.New("Goal primary committed; dependent recovery is required")
var ErrGoalCommitUnknownV1 = errors.New("Goal primary commit is unknown; refresh before any further mutation")

func ExpectedRefV1(patch map[string]any) (domaingoal.RefV1, bool) {
	record, ok := patch["ref"].(map[string]any)
	if !ok || len(record) != 2 {
		return domaingoal.RefV1{}, false
	}
	return domaingoal.RefFromRecordV1(record)
}

// ApplyLifecycleV1 implements only host lifecycle commands. Tool completion
// continues through ApplyStatePatch and the existing evidence authority.
func ApplyLifecycleV1(input StatePatchInput) (map[string]any, error) {
	patch := input.Patch
	operation := strings.TrimSpace(stringField(patch, "operation"))
	existing := input.Existing
	if operation == "" {
		if _, _, _, counted := domaingoal.CountedRecordV1(existing); counted {
			return nil, ErrGoalRefConflictV1
		}
		// Legacy mutations remain manual. Creation must explicitly choose the
		// new lifecycle, so a compatibility patch cannot arm an imported goal.
		return ApplyStatePatch(input)
	}
	allowed := map[string]bool{"operation": true}
	switch operation {
	case "create":
		for _, key := range []string{"objective", "maxGoalRounds", "tokenBudget", "strictCompletion"} {
			allowed[key] = true
		}
	case "edit":
		for _, key := range []string{"ref", "objective", "maxGoalRounds"} {
			allowed[key] = true
		}
	case "pause", "resume", "clear":
		allowed["ref"] = true
	default:
		return nil, errors.New("unknown goal operation")
	}
	for key := range patch {
		if !allowed[key] {
			return nil, errors.New("goal operation contains an unsupported field")
		}
	}
	if operation == "create" {
		if existing != nil && stringField(existing, "status") != "complete" {
			return nil, errors.New("goal already exists; explicitly clear it before creating another")
		}
		objective := strings.TrimSpace(stringField(patch, "objective"))
		if objective == "" || len([]rune(objective)) > 4000 {
			return nil, errors.New("objective must contain 1 to 4000 characters")
		}
		maximum := domaingoal.DefaultMaxRoundsV1
		if raw, present := patch["maxGoalRounds"]; present {
			var valid bool
			maximum, valid = domaingoal.IntegerV1(raw, 1)
			if !valid {
				return nil, errors.New("maxGoalRounds must be a positive safe integer")
			}
		}
		if raw, present := patch["tokenBudget"]; present && raw != nil {
			if _, valid := domaingoal.IntegerV1(raw, 1); !valid {
				return nil, errors.New("tokenBudget must be a positive safe integer or null")
			}
		}
		if raw, present := patch["strictCompletion"]; present {
			if _, valid := raw.(bool); !valid {
				return nil, errors.New("strictCompletion must be a boolean")
			}
		}
		goal, err := ApplyStatePatch(StatePatchInput{ThreadID: input.ThreadID, Patch: map[string]any{"objective": objective, "tokenBudget": patch["tokenBudget"], "strictCompletion": patch["strictCompletion"]}, Todos: input.Todos, Now: input.Now})
		if err != nil {
			return nil, err
		}
		goal["id"] = contracts.NewOpaqueUUIDV4("goal", input.ThreadID, time.Now().UTC())
		goal["revision"], goal["maxGoalRounds"], goal["roundsStarted"] = 1, maximum, 0
		return goal, nil
	}
	ref, maximum, started, counted := domaingoal.CountedRecordV1(existing)
	expected, valid := ExpectedRefV1(patch)
	if !counted || !valid || expected != ref {
		return nil, ErrGoalRefConflictV1
	}
	if ref.Revision >= 9007199254740991 {
		return nil, errors.New("goal revision exhausted")
	}
	if operation == "clear" {
		return nil, nil
	}
	next := contracts.CloneMap(existing)
	switch operation {
	case "edit":
		if len(patch) == 2 {
			return nil, errors.New("edit must change objective or maxGoalRounds")
		}
		if raw, present := patch["objective"]; present {
			objective, ok := raw.(string)
			if !ok || strings.TrimSpace(objective) == "" || len([]rune(objective)) > 4000 {
				return nil, errors.New("invalid objective")
			}
			next["objective"] = projectGoalText(strings.TrimSpace(objective))
		}
		if raw, present := patch["maxGoalRounds"]; present {
			value, ok := domaingoal.IntegerV1(raw, 1)
			if !ok || value < started {
				return nil, errors.New("maxGoalRounds cannot be less than roundsStarted")
			}
			next["maxGoalRounds"] = value
		}
	case "pause":
		if stringField(existing, "status") != "active" {
			return nil, errors.New("only an active goal can be paused")
		}
		next["status"] = "paused"
	case "resume":
		if started >= maximum {
			return nil, ErrGoalRoundLimitV1
		}
		if status := stringField(existing, "status"); status != "active" && status != "paused" && status != "blocked" {
			return nil, errors.New("goal cannot be resumed from its current state")
		}
		next["status"] = "active"
		for _, key := range []string{"blockedReason", "blockedCount", "blockedTurnId"} {
			delete(next, key)
		}
	}
	next["revision"], next["updatedAt"] = ref.Revision+1, input.Now
	return next, nil
}

func AppendHistoryV1(thread map[string]any, previous, next map[string]any, operation, now string) {
	ref, known := domaingoal.RefFromRecordV1(next)
	if next == nil {
		ref, known = domaingoal.RefFromRecordV1(previous)
		ref.Revision++
	}
	entry := map[string]any{"id": contracts.NewOpaqueUUIDV4("goal-change", contracts.StringField(thread, "id"), time.Now().UTC()), "operation": operation, "goal": nil, "createdAt": now}
	if next != nil {
		snapshot := map[string]any{}
		for _, key := range []string{"id", "revision", "maxGoalRounds", "roundsStarted", "threadId", "objective", "status", "tokenBudget", "tokensUsed", "timeUsedSeconds", "blockedReason", "createdAt", "updatedAt"} {
			if value, present := next[key]; present {
				snapshot[key] = contracts.CloneValue(value)
			}
		}
		entry["goal"] = snapshot
	}
	if known {
		entry["ref"] = map[string]any{"id": ref.ID, "revision": ref.Revision}
	}
	history, _ := thread["goalHistory"].([]any)
	history = append(append([]any(nil), history...), entry)
	if len(history) > 500 {
		history = history[len(history)-500:]
	}
	thread["goalHistory"] = history
}
