package turnstart

import (
	"analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	"testing"
)

func goalAdmissionFixtureV1() (map[string]any, map[string]any, domaingoal.RoundSourceV1) {
	source := domaingoal.RoundSourceV1{Kind: "goal", GoalID: "goal-exact", Revision: 4, Round: 2}
	thread := map[string]any{"id": "thread-exact", "goal": map[string]any{"id": source.GoalID, "revision": 4, "maxGoalRounds": 3, "roundsStarted": 1, "status": "active"}}
	item := map[string]any{"id": "item-exact", "kind": "user_message", "threadId": "thread-exact", "turnId": "turn-exact", "text": "exact continuation", "source": domaingoal.RoundSourceRecordV1(source)}
	turn := map[string]any{"id": "turn-exact", "createdAt": "2026-10-11T00:00:00Z", "items": []any{item}}
	return thread, turn, source
}
func TestGoalAdmissionCountsOnlyExactActualUserItem(t *testing.T) {
	thread, turn, source := goalAdmissionFixtureV1()
	next, err := ApplyGoalAdmissionV1(thread, turn, source, "exact continuation")
	if err != nil {
		t.Fatal(err)
	}
	if thread["goal"].(map[string]any)["roundsStarted"] != 1 || next["goal"].(map[string]any)["roundsStarted"] != 2 {
		t.Fatal("counter mutation is not isolated")
	}
	for _, mutation := range []func(map[string]any, map[string]any){
		func(th, tr map[string]any) { th["goal"].(map[string]any)["revision"] = 5 },
		func(th, tr map[string]any) { th["goal"].(map[string]any)["maxGoalRounds"] = 1 },
		func(th, tr map[string]any) { tr["items"].([]any)[0].(map[string]any)["text"] = "different" },
		func(th, tr map[string]any) { delete(tr["items"].([]any)[0].(map[string]any), "source") },
		func(th, tr map[string]any) {
			tr["items"] = append(tr["items"].([]any), contracts.CloneValue(tr["items"].([]any)[0]))
		},
	} {
		th, tr, _ := goalAdmissionFixtureV1()
		mutation(th, tr)
		if _, err := ApplyGoalAdmissionV1(th, tr, source, "exact continuation"); err == nil {
			t.Fatal("nonexact admission accepted", th, tr)
		}
	}
	if _, err := ApplyGoalAdmissionV1(next, turn, source, "exact continuation"); err == nil {
		t.Fatal("same round counted twice")
	}
}
func TestGoalPrimaryWitnessBindsRevisionCounterAndUniqueItem(t *testing.T) {
	thread, turn, source := goalAdmissionFixtureV1()
	primary, err := ApplyGoalAdmissionV1(thread, turn, source, "exact continuation")
	if err != nil {
		t.Fatal(err)
	}
	primary["turns"] = []any{contracts.CloneMap(turn)}
	if !GoalPrimaryWitnessV1(primary, turn, source, "exact continuation") {
		t.Fatal("real primary witness rejected")
	}
	for _, mutation := range []func(map[string]any){
		func(th map[string]any) { th["goal"].(map[string]any)["revision"] = 5 },
		func(th map[string]any) { th["goal"].(map[string]any)["roundsStarted"] = 3 },
		func(th map[string]any) {
			th["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["id"] = "other-item"
		},
		func(th map[string]any) {
			tr := th["turns"].([]any)[0].(map[string]any)
			tr["items"] = append(tr["items"].([]any), contracts.CloneValue(tr["items"].([]any)[0]))
		},
	} {
		th := contracts.CloneMap(primary)
		mutation(th)
		if GoalPrimaryWitnessV1(th, turn, source, "exact continuation") {
			t.Fatal("corrupt witness accepted", th)
		}
	}
}
