package pendingwork

import "testing"

func TestRestartGrantOutcomesByTurnV1RejectsInvalidOrDuplicateAuthority(t *testing.T) {
	valid := RestartOutcomeUnknownGrantV1{ThreadID: "thread-1", TurnID: "turn-1", GrantID: "grant-1"}
	grouped, err := RestartGrantOutcomesByTurnV1([]RestartOutcomeUnknownGrantV1{valid})
	if err != nil || len(grouped) != 1 || len(grouped["thread-1\x00turn-1"]) != 1 {
		t.Fatalf("valid restart grant was not grouped by canonical turn: grouped=%#v err=%v", grouped, err)
	}
	for name, records := range map[string][]RestartOutcomeUnknownGrantV1{
		"missing turn":    {{ThreadID: "thread-1", GrantID: "grant-1"}},
		"missing grant":   {{ThreadID: "thread-1", TurnID: "turn-1"}},
		"duplicate grant": {valid, valid},
	} {
		t.Run(name, func(t *testing.T) {
			if grouped, err := RestartGrantOutcomesByTurnV1(records); err == nil || grouped != nil {
				t.Fatalf("invalid restart grant authority was accepted: grouped=%#v err=%v", grouped, err)
			}
		})
	}
}
