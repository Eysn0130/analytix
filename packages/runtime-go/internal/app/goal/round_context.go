package goal

import (
	"analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
)

func RecordedRoundForTurnV1(thread, goal map[string]any, turnID string) (domaingoal.RoundSourceV1, bool) {
	ref, _, started, counted := domaingoal.CountedRecordV1(goal)
	if !counted {
		return domaingoal.RoundSourceV1{}, false
	}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") != turnID {
			continue
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if contracts.StringField(item, "kind") != "user_message" || contracts.StringField(item, "threadId") != contracts.StringField(goal, "threadId") || contracts.StringField(item, "turnId") != turnID {
				continue
			}
			source, valid := domaingoal.ParseRoundSourceV1(item["source"])
			if valid && source.GoalID == ref.ID && source.Revision == ref.Revision && source.Round <= started {
				return source, true
			}
		}
	}
	return domaingoal.RoundSourceV1{}, false
}
