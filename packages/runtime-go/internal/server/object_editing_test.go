package server

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type selectionObserver func(string) (domainsecurity.CaseBindingObservationV1, error)

func (observe selectionObserver) Observe(path string) (domainsecurity.CaseBindingObservationV1, error) {
	return observe(path)
}

func TestNativeSelectionCannotBypassInjectedWorkspaceObservation(t *testing.T) {
	p, _, scope := newSelectionProjectorFixture(t)
	calls := 0
	p.handler.turnSecurity.Observer = selectionObserver(func(string) (domainsecurity.CaseBindingObservationV1, error) {
		calls++
		return domainsecurity.CaseBindingObservationV1{}, errors.New("unavailable")
	})
	if p.ValidateCurrent(context.Background(), scope) == nil || calls != 1 {
		t.Fatal("selection bypassed the injected workspace observer")
	}
	p.handler.turnSecurity.Observer = nil
	if p.ValidateCurrent(context.Background(), scope) == nil {
		t.Fatal("selection inferred filesystem authority without an observer")
	}
}

func newSelectionProjectorFixture(t *testing.T) (runtimeObjectProjector, map[string]any, editingapp.ScopeAuthority) {
	t.Helper()
	workspace := workspacetest.New(t)
	h := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, h)
	thread, err := h.store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err = h.store.GetThread(stringField(thread, "id"))
	if err != nil {
		t.Fatal(err)
	}
	scope := editingapp.ScopeAuthority{Principal: testIdentityPrincipal(), ObjectID: "synthetic-object", ThreadID: stringField(thread, "id"), Purpose: "edit", Workspace: workspace, Path: "synthetic.docx"}
	return runtimeObjectProjector{handler: h}, thread, scope
}

func TestNativeSelectionFreshThreadUsesHostAuthorityWithoutCreatingHistory(t *testing.T) {
	p, before, scope := newSelectionProjectorFixture(t)
	ctx := context.Background()
	for _, purpose := range []string{"discuss", "edit"} {
		scope.Purpose = purpose
		ranges, err := p.AuthorizeAndProject(ctx, editingapp.ProjectionInput{ScopeAuthority: scope, Text: "Contact: alice@example.com"})
		if err != nil || len(ranges) == 0 {
			t.Fatalf("fresh %s capture did not retain protected projection: ranges=%d err=%v", purpose, len(ranges), err)
		}
	}
	after, err := p.handler.store.GetThread(scope.ThreadID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("selection altered the thread or its history: err=%v", err)
	}
	// The normal turn-start authority can take over without the selection having
	// manufactured a prior turn, persisted context, or provider request.
	frozen, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: ctx, Authority: p.handler.turnSecurity, Thread: after, ThreadID: scope.ThreadID,
		TurnID: "turn-first-real-send", Workspace: scope.Workspace, Principal: scope.Principal, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	securityRecord := turnsecurityapp.PublicRecord(frozen)
	if err := p.handler.store.AppendTurnToThread(scope.ThreadID, map[string]any{
		"id": frozen.TurnID, "threadId": scope.ThreadID, "status": "completed", "items": []any{}, "securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateCurrent(ctx, scope); err != nil {
		t.Fatalf("same-thread selection could not use normal turn authority: %v", err)
	}
}

func TestNativeSelectionFreshThreadRejectsNonfreshOrDamagedAuthority(t *testing.T) {
	p, original, scope := newSelectionProjectorFixture(t)
	cases := map[string]func(map[string]any){
		"past turn":              func(v map[string]any) { v["turns"] = []any{map[string]any{"id": "previous"}} },
		"missing turns":          func(v map[string]any) { delete(v, "turns") },
		"malformed turns":        func(v map[string]any) { v["turns"] = "invalid" },
		"running":                func(v map[string]any) { v["status"] = "running" },
		"side":                   func(v map[string]any) { v["relation"] = "side" },
		"missing relation":       func(v map[string]any) { delete(v, "relation") },
		"kind":                   func(v map[string]any) { v["kind"] = "case" },
		"null kind":              func(v map[string]any) { v["kind"] = nil },
		"case marker":            func(v map[string]any) { v["caseId"] = "synthetic-case" },
		"null case marker":       func(v map[string]any) { v["caseProjectId"] = nil },
		"parent":                 func(v map[string]any) { v["parentThreadId"] = "parent" },
		"fork":                   func(v map[string]any) { v["forkedFromThreadId"] = "source" },
		"boundary history":       func(v map[string]any) { v["historyAuthority"] = "case_boundary_only" },
		"epoch without security": func(v map[string]any) { v["contextEpochState"] = nil },
		"null security":          func(v map[string]any) { v["securityState"] = nil },
		"damaged security":       func(v map[string]any) { v["securityState"] = map[string]any{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			thread := contracts.CloneMap(original)
			mutate(thread)
			if _, err := p.selectionSecurityContext(context.Background(), thread, scope); err == nil {
				t.Fatal("unsupported thread received fresh selection authority")
			}
		})
	}
	p.handler.caseThreads.(*caseThreadAuthorityStub).threads[scope.ThreadID] = true
	if _, err := p.selectionSecurityContext(context.Background(), original, scope); err == nil {
		t.Fatal("registered case thread received fresh general authority")
	}
}

func TestNativeSelectionFreshThreadRejectsIdentityWorkspaceAndAuthorityMismatch(t *testing.T) {
	p, _, scope := newSelectionProjectorFixture(t)
	wrongPrincipal := scope
	wrongPrincipal.Principal.UserID = "other-user"
	wrongWorkspace := scope
	wrongWorkspace.Workspace = workspacetest.New(t)
	wrongThread := scope
	wrongThread.ThreadID = "missing-thread"
	for _, candidate := range []editingapp.ScopeAuthority{wrongPrincipal, wrongWorkspace, wrongThread} {
		if err := p.ValidateCurrent(context.Background(), candidate); err == nil {
			t.Fatal("mismatched selection received authority")
		}
	}
	p.handler.turnSecurity.RiskAuthority = nil
	if err := p.ValidateCurrent(context.Background(), scope); err == nil {
		t.Fatal("missing risk authority inferred a general selection")
	}
}
