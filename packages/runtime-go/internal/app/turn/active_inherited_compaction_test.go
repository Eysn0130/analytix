package turn

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func activeInheritedCompactionFixtureV1(t *testing.T) CompactionInput {
	t.Helper()
	threadID := "active-compaction-target"
	turns := []any{}
	inherited := []domainsecurity.ActiveInheritedTurnV1{}
	for _, id := range []string{"inherited-first", "inherited-second"} {
		turn := map[string]any{
			"id": id, "threadId": threadID, "status": "completed",
			"createdAt": "2026-09-09T00:00:00Z", "reasoningEffort": "high",
			"items": []any{map[string]any{
				"id": "user-" + id, "kind": "user_message", "role": "user", "text": "private inherited request",
				"attachments": []any{map[string]any{"id": "attachment-" + id, "size": json.Number("9007199254740993")}},
			}},
		}
		body, err := json.Marshal(turn)
		if err != nil {
			t.Fatal(err)
		}
		inherited = append(inherited, domainsecurity.ActiveInheritedTurnV1{TurnID: id, ContentSHA256: domainsecurity.SHA256Hex(body)})
		turns = append(turns, turn)
	}
	current := newTurnCaseExecutionContextV2(t, threadID, "target-current", "/cases/active-compaction", 2, time.Unix(100, 0))
	turns = append(turns,
		map[string]any{"id": "old-unbound", "threadId": threadID, "status": "completed", "items": []any{}},
		map[string]any{"id": current.TurnID, "threadId": threadID, "status": "completed", "securityContext": current,
			"items": []any{map[string]any{"id": "target-user", "kind": "user_message", "role": "user", "text": "target request"}}},
	)
	continuation, err := BuildTaskContinuationSnapshotV1(map[string]any{"id": threadID, "turns": turns})
	if err != nil {
		t.Fatal(err)
	}
	return CompactionInput{
		ThreadID: threadID, Turns: turns, SecurityContext: current, CaseMarker: true,
		Reason: "manual", Stamp: 200, Now: "2026-09-09T00:01:00Z",
		CaseContinuation: &continuation, CaseAuthorityTurnIDs: []string{current.TurnID}, ActiveInheritedTurns: inherited,
	}
}

func TestActiveInheritedCompactionPreservesExactPrivatePrefixV1(t *testing.T) {
	input := activeInheritedCompactionFixtureV1(t)
	before, _ := json.Marshal(input.Turns)
	plan := BuildCompaction(input)
	if plan.Error != nil || !plan.Changed || plan.Result.RemainingTurns != len(plan.NextTurns) {
		t.Fatalf("active compaction failed: %v", plan.Error)
	}
	for index, entry := range input.ActiveInheritedTurns {
		body, err := json.Marshal(plan.NextTurns[index])
		if err != nil || domainsecurity.SHA256Hex(body) != entry.ContentSHA256 {
			t.Fatal("compaction changed private prefix content or metadata outside recent tail")
		}
		turn := plan.NextTurns[index].(map[string]any)
		if _, present := turn["securityContext"]; present {
			t.Fatal("inherited prefix acquired target execution authority")
		}
		if _, present := turn["caseHistoryProjection"]; present {
			t.Fatal("inherited prefix acquired a compaction marker")
		}
	}
	last := plan.NextTurns[len(plan.NextTurns)-1].(map[string]any)
	if last["id"] != plan.Result.TurnID || last["threadId"] != input.ThreadID || last["caseHistoryProjection"] != "compaction_authority_v1" {
		t.Fatal("new target compaction marker is missing or detached")
	}
	item := last["items"].([]any)[0].(map[string]any)
	binding, err := ParseCaseCompactionOperationBindingV1(item["caseCompactionBinding"])
	if err != nil || !reflect.DeepEqual(binding.AuthorityTurnIDs, input.CaseAuthorityTurnIDs) {
		t.Fatal("inherited identities entered target compaction execution inventory")
	}
	// Mutating the result must not mutate the original authenticated input.
	first := plan.NextTurns[0].(map[string]any)
	first["items"].([]any)[0].(map[string]any)["text"] = "caller mutation"
	after, _ := json.Marshal(input.Turns)
	if string(before) != string(after) {
		t.Fatal("compaction aliased the source private prefix")
	}
}

func TestActiveInheritedCompactionRejectsInvalidReplayV1(t *testing.T) {
	for _, fault := range []string{"wrong id", "reordered", "wrong digest", "duplicate inventory", "later duplicate", "overlap", "incomplete prefix", "ordinary", "missing case authorization"} {
		t.Run(fault, func(t *testing.T) {
			input := activeInheritedCompactionFixtureV1(t)
			switch fault {
			case "wrong id":
				input.ActiveInheritedTurns[0].TurnID = "other-turn"
			case "reordered":
				input.ActiveInheritedTurns[0], input.ActiveInheritedTurns[1] = input.ActiveInheritedTurns[1], input.ActiveInheritedTurns[0]
			case "wrong digest":
				input.ActiveInheritedTurns[0].ContentSHA256 = domainsecurity.SHA256Hex([]byte("substituted content"))
			case "duplicate inventory":
				input.ActiveInheritedTurns[1] = input.ActiveInheritedTurns[0]
			case "later duplicate":
				input.Turns = append(input.Turns, input.Turns[0])
			case "overlap":
				current := input.SecurityContext.(domainsecurity.TurnSecurityContext)
				input.Turns[0] = input.Turns[len(input.Turns)-1]
				body, _ := json.Marshal(input.Turns[0])
				input.Turns = input.Turns[:len(input.Turns)-1]
				input.ActiveInheritedTurns[0] = domainsecurity.ActiveInheritedTurnV1{TurnID: current.TurnID, ContentSHA256: domainsecurity.SHA256Hex(body)}
			case "incomplete prefix":
				input.ActiveInheritedTurns = append(input.ActiveInheritedTurns, input.ActiveInheritedTurns...)
				input.ActiveInheritedTurns = append(input.ActiveInheritedTurns, input.ActiveInheritedTurns[0])
			case "ordinary":
				input.SecurityContext, input.CaseContinuation, input.CaseAuthorityTurnIDs = nil, nil, nil
				input.CaseMarker = false
				input.Turns = append(input.Turns[:2], map[string]any{"id": "ordinary-tail", "items": []any{}})
			case "missing case authorization":
				input.CaseContinuation = nil
			}
			plan := BuildCompaction(input)
			if plan.Error == nil || plan.Changed || len(plan.NextTurns) != 0 {
				t.Fatal("invalid active prefix was accepted for compaction")
			}
		})
	}
}

func TestActiveInheritedCompactionOptionalInputKeepsLegacyBehaviorV1(t *testing.T) {
	input := activeInheritedCompactionFixtureV1(t)
	thread := map[string]any{"id": input.ThreadID, "turns": input.Turns, "securityState": input.SecurityContext}
	legacy := BuildThreadCompactionWithCaseAuthority(thread, input.ThreadID, input.Reason, input.Stamp, input.Now, false, *input.CaseContinuation, input.CaseAuthorityTurnIDs)
	explicitNil := BuildThreadCompactionWithCaseAuthority(thread, input.ThreadID, input.Reason, input.Stamp, input.Now, false, *input.CaseContinuation, input.CaseAuthorityTurnIDs, nil)
	if legacy.Error != nil || !legacy.Changed || !reflect.DeepEqual(legacy, explicitNil) {
		t.Fatal("omitted or nil inherited input changed legacy compaction")
	}
	active := BuildThreadCompactionWithCaseAuthority(thread, input.ThreadID, input.Reason, input.Stamp, input.Now, false, *input.CaseContinuation, input.CaseAuthorityTurnIDs, input.ActiveInheritedTurns)
	if active.Error != nil || !active.Changed || len(active.NextTurns) != len(legacy.NextTurns)+len(input.ActiveInheritedTurns) {
		t.Fatal("optional authenticated prefix did not reach the producer")
	}
	ambiguous := BuildThreadCompactionWithCaseAuthority(thread, input.ThreadID, input.Reason, input.Stamp, input.Now, false, *input.CaseContinuation, input.CaseAuthorityTurnIDs, nil, nil)
	if ambiguous.Error == nil {
		t.Fatal("multiple inherited inventories were accepted")
	}
}
