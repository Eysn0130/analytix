package thread

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type publicWorkspaceTransitionFixture struct {
	thread      map[string]any
	prepared    PreparedWorkspaceMutation
	reader      mutationWorkspaceObserver
	authority   turnsecurityapp.WorkspaceSecurityAuthority
	publicTurns any
}

func preparePublicWorkspaceTransitionFixture(t *testing.T, target string) publicWorkspaceTransitionFixture {
	t.Helper()
	at := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
	reader := mutationWorkspaceObserver{observations: map[string]domainsecurity.CaseBindingObservationV1{
		"/workspace/a": mutationObservation(t, "/workspace/a", domainsecurity.CaseBindingStateMissing, "", ""),
		"/workspace/b": mutationObservation(t, "/workspace/b", domainsecurity.CaseBindingStateMissing, "", ""),
	}}
	reader.observations["/workspace/b-alias"] = reader.observations["/workspace/b"]
	authority := turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(), Observer: reader, RiskAuthority: newMutationRiskAuthority()}
	thread := BuildThread(CreateInput{Request: map[string]any{"workspace": "/workspace/a", "model": "fixture-model"}, ThreadID: "thread-workspace-public", Now: at.Add(-time.Hour).Format(time.RFC3339Nano)})
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: contracts.StringField(thread, "id"), TurnID: "turn-retained", Prompt: "retained request", Model: "fixture-model", CreatedAt: at.Add(-time.Hour).Format(time.RFC3339Nano)})
	start.Turn["status"] = "completed"
	start.Turn["finishedAt"] = start.Turn["createdAt"]
	thread["turns"] = []any{start.Turn}
	publicBefore, err := ProjectPublicThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := FreezeWorkspaceRebindScopes(thread, contracts.StringField(thread, "id"), target, reader, testIdentityPrincipal(), at, authority)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareWorkspaceMutation(thread, scopes)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := MutationBaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := ApplyWorkspaceMutationCommit(thread, WorkspaceMutationCommitRequest{ExpectedBaselineDigest: digest, Prepared: prepared, Patch: map[string]any{"workspace": target}})
	if err != nil {
		t.Fatal(err)
	}
	return publicWorkspaceTransitionFixture{thread: committed, prepared: prepared, reader: reader, authority: authority, publicTurns: publicBefore["turns"]}
}

func TestOrdinaryPublicHistoryOmitsWorkspaceTransitionForPatchAndGet(t *testing.T) {
	for _, target := range []string{"/workspace/b", "/workspace/b-alias"} {
		t.Run(target, func(t *testing.T) {
			fixture := preparePublicWorkspaceTransitionFixture(t, target)
			before := contracts.CloneMap(fixture.thread)
			// Patch returns this same ordinary public projection after commit.
			patch, err := ProjectPublicThread(fixture.thread)
			if err != nil {
				t.Fatal(err)
			}
			if !samePublicJSON(patch["turns"], fixture.publicTurns) {
				t.Fatal("workspace patch exposed an internal transition as a conversation turn")
			}
			body, err := json.Marshal(fixture.thread)
			if err != nil {
				t.Fatal(err)
			}
			reloaded := map[string]any{}
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			service := NewService(Dependencies{Repository: &repositoryStub{thread: reloaded}})
			detail, err := service.Get(fixture.prepared.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			if !samePublicJSON(detail["turns"], fixture.publicTurns) || detail["pendingApprovalIds"] == nil || detail["pendingUserInputIds"] == nil {
				t.Fatal("reloaded GET did not expose the retained public conversation")
			}
			if !samePublicJSON(fixture.thread, before) || !samePublicJSON(reloaded, before) {
				t.Fatal("projection changed durable workspace authority")
			}
		})
	}
}

func TestOrdinaryPublicHistoryRejectsInvalidWorkspaceTransition(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, map[string]any, map[string]any)
	}{
		{"ordinary prompt disguised as transition", func(_ *testing.T, _ map[string]any, marker map[string]any) { marker["prompt"] = "must remain visible" }},
		{"nonempty items", func(_ *testing.T, _ map[string]any, marker map[string]any) {
			marker["items"] = []any{map[string]any{"kind": "user_message", "text": "must remain visible"}}
		}},
		{"missing context", func(_ *testing.T, _ map[string]any, marker map[string]any) { delete(marker, "securityContext") }},
		{"corrupt context", func(_ *testing.T, _ map[string]any, marker map[string]any) {
			marker["securityContext"].(map[string]any)["contextDigest"] = "invalid"
		}},
		{"foreign thread", func(_ *testing.T, _ map[string]any, marker map[string]any) { marker["threadId"] = "foreign-thread" }},
		{"corrupt snapshot", func(_ *testing.T, _ map[string]any, marker map[string]any) {
			marker["contextEpochSnapshot"].(map[string]any)["contextDigest"] = "invalid"
		}},
		{"missing current authority", func(_ *testing.T, thread map[string]any, _ map[string]any) { delete(thread, "securityState") }},
		{"resealed wrong binding witness", func(t *testing.T, thread map[string]any, marker map[string]any) {
			state, found, err := contextepochapp.StateFromThread(thread)
			if err != nil || !found {
				t.Fatalf("fixture state unavailable: %v", err)
			}
			for index := range state.Registry {
				if state.Registry[index].SourceID == contextepochapp.SecurityBindingSourceID {
					state.Registry[index].Digest = state.AcceptedSnapshot.ContextDigest
				}
			}
			for index := range state.AcceptedSnapshot.Sources {
				if state.AcceptedSnapshot.Sources[index].SourceID == contextepochapp.SecurityBindingSourceID {
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
			fixture := preparePublicWorkspaceTransitionFixture(t, "/workspace/b")
			marker := fixture.thread["turns"].([]any)[1].(map[string]any)
			test.mutate(t, fixture.thread, marker)
			before := contracts.CloneMap(fixture.thread)
			if _, err := ProjectPublicThread(fixture.thread); err == nil {
				t.Fatal("invalid workspace transition was accepted")
			}
			if !samePublicJSON(fixture.thread, before) {
				t.Fatal("rejected projection mutated durable state")
			}
		})
	}
}

func TestOrdinaryPublicHistoryRetainsLaterTurnAndRewindAfterWorkspaceTransition(t *testing.T) {
	fixture := preparePublicWorkspaceTransitionFixture(t, "/workspace/b")
	at := time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: fixture.authority, Reader: fixture.reader, Thread: fixture.thread,
		ThreadID: fixture.prepared.ThreadID, TurnID: "turn-later", Workspace: "/workspace/b", Principal: testIdentityPrincipal(), IssuedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: fixture.prepared.ThreadID, TurnID: frozen.TurnID, Prompt: "later ordinary request", Model: "fixture-model", CreatedAt: at.Format(time.RFC3339Nano)})
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{Thread: fixture.thread, SecurityContext: frozen, At: at})
	if err != nil {
		t.Fatal(err)
	}
	patch := map[string]any{}
	turnsecurityapp.AttachStartRecords(start.Turn, start.TurnStartedEvent, patch, epoch.SecurityContext)
	contextepochapp.AttachStartRecords(start.Turn, start.TurnStartedEvent, patch, epoch.State)
	for key, value := range patch {
		fixture.thread[key] = value
	}
	start.Turn["status"] = "completed"
	start.Turn["finishedAt"] = at.Format(time.RFC3339Nano)
	fixture.thread["turns"] = append(fixture.thread["turns"].([]any), start.Turn)
	projected, err := ProjectPublicThread(fixture.thread)
	if err != nil {
		t.Fatal(err)
	}
	turns := projected["turns"].([]any)
	if len(turns) != 2 || contracts.StringField(turns[1].(map[string]any), "prompt") != "later ordinary request" {
		t.Fatal("historical workspace transition concealed the later ordinary turn")
	}
	rewound, err := PrepareRewindMutation(fixture.thread, fixture.prepared.ThreadID, frozen.TurnID, epoch.SecurityContext, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	before := contracts.CloneMap(rewound.Thread)
	projected, err = ProjectPublicThread(rewound.Thread)
	if err != nil {
		t.Fatal(err)
	}
	if !samePublicJSON(projected["turns"], fixture.publicTurns) || !samePublicJSON(rewound.Thread, before) {
		t.Fatal("workspace then rewind changed the retained conversation or durable authority")
	}
}
