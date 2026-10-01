package thread

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
)

func publicHistoryRewindFixture(t *testing.T) (PreparedRewindMutation, map[string]any) {
	t.Helper()
	thread, _, _ := canonicalGeneralTerminalPublicFixture(t, "failed", "provider_failure")
	current, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil || !found {
		t.Fatalf("fixture current context unavailable: %v", err)
	}
	retained := map[string]any{
		"id": "turn-retained", "threadId": current.ThreadID, "status": "completed",
		"prompt": "earlier request", "createdAt": "2026-07-14T00:00:00Z", "items": []any{},
		"steering": []any{}, "attachmentIds": []any{}, "activeSkillIds": []any{}, "injectedMemoryIds": []any{},
	}
	thread["turns"] = append([]any{retained}, thread["turns"].([]any)...)
	prepared, err := PrepareRewindMutation(thread, current.ThreadID, current.TurnID, current, time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return prepared, retained
}

func TestOrdinaryPublicHistoryOmitsValidatedRewindTransitionAfterReload(t *testing.T) {
	prepared, retained := publicHistoryRewindFixture(t)
	before := contracts.CloneMap(prepared.Thread)
	for _, name := range []string{"committed", "reloaded"} {
		t.Run(name, func(t *testing.T) {
			thread := prepared.Thread
			if name == "reloaded" {
				body, err := json.Marshal(thread)
				if err != nil {
					t.Fatal(err)
				}
				thread = map[string]any{}
				if err := json.Unmarshal(body, &thread); err != nil {
					t.Fatal(err)
				}
			}
			projected, err := ProjectPublicThread(thread)
			if err != nil {
				t.Fatal(err)
			}
			if !samePublicJSON(projected["turns"], []any{retained}) {
				t.Fatal("public history must retain the ordinary turn and omit the internal rewind transition")
			}
			if !samePublicJSON(thread, before) {
				t.Fatal("public projection changed durable rewind history or authority")
			}
		})
	}
}

func TestOrdinaryPublicHistoryRejectsInvalidRewindTransition(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any, map[string]any)
	}{
		{"ordinary content disguised as rewind", func(_ map[string]any, marker map[string]any) { marker["prompt"] = "must not be hidden" }},
		{"nonempty items", func(_ map[string]any, marker map[string]any) {
			marker["items"] = []any{map[string]any{"kind": "user_message", "text": "must not be hidden"}}
		}},
		{"missing security authority", func(_ map[string]any, marker map[string]any) { delete(marker, "securityContext") }},
		{"corrupt security authority", func(_ map[string]any, marker map[string]any) {
			marker["securityContext"].(map[string]any)["contextDigest"] = "invalid"
		}},
		{"foreign thread", func(_ map[string]any, marker map[string]any) { marker["threadId"] = "foreign-thread" }},
		{"timestamp mismatch", func(_ map[string]any, marker map[string]any) { marker["completedAt"] = "2026-07-17T00:00:00Z" }},
		{"wrong target identity", func(_ map[string]any, marker map[string]any) { marker["removedTurnIds"] = []any{"other-removed-turn"} }},
		{"duplicate removed identity", func(_ map[string]any, marker map[string]any) {
			marker["removedTurnIds"] = []any{"turn-public-terminal", "turn-public-terminal"}
		}},
		{"removed identity still retained", func(_ map[string]any, marker map[string]any) {
			marker["removedTurnIds"] = append(marker["removedTurnIds"].([]any), "turn-retained")
		}},
		{"corrupt snapshot", func(_ map[string]any, marker map[string]any) {
			marker["contextEpochSnapshot"].(map[string]any)["contextDigest"] = "invalid"
		}},
		{"unknown snapshot field", func(_ map[string]any, marker map[string]any) {
			marker["contextEpochSnapshot"].(map[string]any)["unknown"] = true
		}},
		{"missing current epoch authority", func(thread map[string]any, _ map[string]any) { delete(thread, "contextEpochState") }},
		{"missing current security authority", func(thread map[string]any, _ map[string]any) { delete(thread, "securityState") }},
		{"different current security authority", func(thread map[string]any, _ map[string]any) {
			current, _, _ := turnsecurityapp.LatestContext(thread)
			at, _ := time.Parse(time.RFC3339Nano, current.IssuedAt)
			thread["securityState"] = turnsecurityapp.PublicRecord(rewindMutationTarget(current, "other-target", at))
		}},
		{"resealed wrong history witness", func(thread map[string]any, marker map[string]any) {
			state, found, err := contextepochapp.StateFromThread(thread)
			if err != nil || !found {
				t.Fatalf("fixture epoch state unavailable: %v", err)
			}
			for index := range state.Registry {
				if state.Registry[index].SourceID == "host-history-mutation" {
					state.Registry[index].Digest = state.AcceptedSnapshot.ContextDigest
				}
			}
			for index := range state.AcceptedSnapshot.Sources {
				if state.AcceptedSnapshot.Sources[index].SourceID == "host-history-mutation" {
					state.AcceptedSnapshot.Sources[index].Digest = state.AcceptedSnapshot.ContextDigest
				}
			}
			state.AcceptedSnapshot.RegistryDigest = domaincontextepoch.RegistryDigest(state.Registry)
			state.AcceptedSnapshot = domaincontextepoch.SealSnapshot(state.AcceptedSnapshot)
			state = domaincontextepoch.SealState(state)
			thread["contextEpochState"] = contextepochapp.PublicState(state)
			marker["contextEpochSnapshot"] = contextepochapp.PublicSnapshot(state.AcceptedSnapshot)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prepared, _ := publicHistoryRewindFixture(t)
			thread := prepared.Thread
			marker := thread["turns"].([]any)[1].(map[string]any)
			test.mutate(thread, marker)
			before := contracts.CloneMap(thread)
			if _, err := ProjectPublicThread(thread); err == nil {
				t.Fatal("invalid rewind transition was accepted by the public projection")
			}
			if !reflect.DeepEqual(thread, before) {
				t.Fatal("rejected projection mutated durable state")
			}
		})
	}
}

func TestOrdinaryPublicHistoryDoesNotHideOrdinaryTurnByUnknownKind(t *testing.T) {
	prepared, retained := publicHistoryRewindFixture(t)
	retained["kind"] = "unknown_transition"
	prepared.Thread["turns"].([]any)[0] = retained
	projected, err := ProjectPublicThread(prepared.Thread)
	if err != nil {
		t.Fatal(err)
	}
	turns := projected["turns"].([]any)
	if len(turns) != 1 || contracts.StringField(turns[0].(map[string]any), "prompt") != "earlier request" {
		t.Fatal("unknown kind concealed an ordinary conversation turn")
	}
}

func TestOrdinaryPublicHistoryOmitsHistoricalRewindAfterAnotherRewind(t *testing.T) {
	first, retained := publicHistoryRewindFixture(t)
	first.Thread["turns"] = append(first.Thread["turns"].([]any), map[string]any{
		"id": "turn-later", "threadId": first.ThreadID, "status": "completed",
		"prompt": "later request", "createdAt": "2026-07-17T00:00:00Z", "items": []any{},
	})
	second, err := PrepareRewindMutation(first.Thread, first.ThreadID, "turn-later", first.SecurityContext, time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	before := contracts.CloneMap(second.Thread)
	projected, err := ProjectPublicThread(second.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if !samePublicJSON(projected["turns"], []any{retained}) {
		t.Fatal("historical or current rewind transition entered public history")
	}
	if !samePublicJSON(second.Thread, before) || len(second.Thread["turns"].([]any)) != 3 {
		t.Fatal("projection lost durable rewind authority")
	}
}
