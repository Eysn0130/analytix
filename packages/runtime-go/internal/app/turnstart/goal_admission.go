package turnstart

import (
	"errors"

	"analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
)

type GoalAppendOutcomeV1 struct {
	Committed        bool
	RecoveryRequired bool
}

// ApplyGoalAdmissionV1 is invoked under the existing durable mutation mutex
// after baseline validation. It binds the increment to one real user item.
func ApplyGoalAdmissionV1(thread, turn map[string]any, source domaingoal.RoundSourceV1, prompt string) (map[string]any, error) {
	goal, _ := thread["goal"].(map[string]any)
	ref, maximum, started, counted := domaingoal.CountedRecordV1(goal)
	if !counted || contracts.StringField(goal, "status") != "active" || source.Kind != "goal" || source.GoalID != ref.ID || source.Revision != ref.Revision || source.Round != started+1 || source.Round > maximum {
		return nil, errors.New("Goal round admission is stale or exhausted")
	}
	items, _ := turn["items"].([]any)
	found := 0
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if contracts.StringField(item, "kind") != "user_message" {
			continue
		}
		actual, valid := domaingoal.ParseRoundSourceV1(item["source"])
		if !valid || actual != source || contracts.StringField(item, "threadId") != contracts.StringField(thread, "id") || contracts.StringField(item, "turnId") != contracts.StringField(turn, "id") || contracts.StringField(item, "id") == "" || contracts.StringField(item, "text") != prompt {
			return nil, errors.New("Goal round user item does not match the exact admission")
		}
		found++
	}
	if found != 1 {
		return nil, errors.New("Goal round requires exactly one actual user item")
	}
	next := contracts.CloneMap(thread)
	nextGoal := contracts.CloneMap(goal)
	nextGoal["roundsStarted"] = source.Round
	nextGoal["updatedAt"] = contracts.StringField(turn, "createdAt")
	next["goal"] = nextGoal
	return next, nil
}

func GoalPrimaryWitnessV1(thread, turn map[string]any, source domaingoal.RoundSourceV1, prompt string) bool {
	goal, _ := thread["goal"].(map[string]any)
	ref, _, started, counted := domaingoal.CountedRecordV1(goal)
	if !counted || ref.ID != source.GoalID || ref.Revision != source.Revision || started != source.Round {
		return false
	}
	expectedItems, _ := turn["items"].([]any)
	expectedItemID := ""
	for _, raw := range expectedItems {
		item, _ := raw.(map[string]any)
		if contracts.StringField(item, "kind") == "user_message" {
			if expectedItemID != "" {
				return false
			}
			expectedItemID = contracts.StringField(item, "id")
		}
	}
	if expectedItemID == "" {
		return false
	}
	turns, _ := thread["turns"].([]any)
	found := 0
	for _, raw := range turns {
		candidate, _ := raw.(map[string]any)
		if contracts.StringField(candidate, "id") != contracts.StringField(turn, "id") {
			continue
		}
		items, _ := candidate["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			actual, valid := domaingoal.ParseRoundSourceV1(item["source"])
			if contracts.StringField(item, "kind") != "user_message" {
				continue
			}
			if !valid || actual != source || contracts.StringField(item, "id") != expectedItemID || contracts.StringField(item, "text") != prompt || contracts.StringField(item, "threadId") != contracts.StringField(thread, "id") || contracts.StringField(item, "turnId") != contracts.StringField(turn, "id") {
				return false
			}
			found++
		}
	}
	return found == 1
}
