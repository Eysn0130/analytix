package server

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type caseThreadAuthorityStub struct {
	threads map[string]bool
}

func (stub *caseThreadAuthorityStub) Register(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	stub.threads[securityContext.ThreadID] = true
	return nil
}

func (stub *caseThreadAuthorityStub) Derive(_ context.Context, parentThreadID, threadID, _ string) error {
	if !stub.threads[parentThreadID] {
		return context.Canceled
	}
	stub.threads[threadID] = true
	return nil
}

func (stub *caseThreadAuthorityStub) IsCaseThread(threadID string) bool {
	return stub.threads[strings.TrimSpace(threadID)]
}
func (*caseThreadAuthorityStub) ContainsContext(domainsecurity.TurnSecurityContext) bool {
	return false
}
func (*caseThreadAuthorityStub) ContextTurnIDs(string) []string      { return nil }
func (*caseThreadAuthorityStub) CanExecute(string) bool              { return true }
func (*caseThreadAuthorityStub) ReplaceQuarantine(map[string]string) {}

func (*caseThreadAuthorityStub) WithRestartRecoveryV1(_ string, recover func() error) error {
	return recover()
}

func TestFirstCaseTurnMarkerRemovalNeverPublishesOrLaundersDraftFacts(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "case authority", "workspace": "/cases/a"}, "/cases/a")
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-marker-stripped"
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed", "items": []any{
			map[string]any{"id": "user-a", "threadId": threadID, "turnId": turnID, "kind": "user_message", "role": "user", "text": "USER_REQUEST"},
			map[string]any{"id": "assistant-a", "threadId": threadID, "turnId": turnID, "kind": "assistant_text", "role": "assistant", "text": "CASE_DRAFT_SENTINEL_4200000"},
		},
	}, "provider", nil); err != nil {
		t.Fatal(err)
	}
	authority := &caseThreadAuthorityStub{threads: map[string]bool{threadID: true}}
	store.SetCaseThreadAuthority(authority)
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	if _, _, err := store.RecordEvent(map[string]any{
		"kind": "assistant_text_delta", "threadId": threadID, "turnId": turnID, "text": "CASE_EVENT_SENTINEL_9988",
	}); err == nil {
		t.Fatal("marker-stripped case assistant event reached durable publication")
	}
	select {
	case event := <-live:
		t.Fatalf("marker-stripped case assistant event reached live SSE: %#v", event)
	default:
	}
	if err := store.AppendItemToTurn(threadID, turnID, map[string]any{
		"id": "assistant-late", "kind": "assistant_text", "text": "CASE_APPEND_SENTINEL_7766",
	}); err == nil {
		t.Fatal("marker-stripped case assistant item reached durable history")
	}

	projector := threadapp.NewTrustedPublicProjectorWithCaseThreads(nil, authority)
	raw, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projector.ProjectThread(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertNoCaseDraftSentinel(t, projected)

	fork, err := store.ForkThread(threadID, map[string]any{"title": "isolated case fork"})
	if err != nil || fork == nil {
		t.Fatalf("registered case fork was not isolated through host authority: fork=%#v err=%v", fork, err)
	}
	if resume, err := store.ResumeSession(threadID, map[string]any{"workspace": "/cases/other"}); !errors.Is(err, threadapp.ErrCaseDerivationAdmissionAuthorityRequired) || resume != nil {
		t.Fatalf("case resume accepted a different workspace: resume=%#v err=%v", resume, err)
	}
	resume, err := store.ResumeSession(threadID, map[string]any{})
	if err != nil || resume == nil {
		t.Fatalf("registered same-workspace case resume was not isolated through host authority: resume=%#v err=%v", resume, err)
	}
	for name, derived := range map[string]any{"fork": fork, "resume": resume} {
		body, _ := json.Marshal(derived)
		for _, forbidden := range []string{"CASE_DRAFT_SENTINEL", "CASE_APPEND_SENTINEL", "assistant_text", "acceptedFinal", "securityState"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("%s retained case authority field %q: %s", name, forbidden, body)
			}
		}
	}
	if len(authority.threads) != 3 {
		t.Fatalf("case derivation lineage mismatch: %#v", authority.threads)
	}
	if all, err := store.ListThreads(false, true, true, ""); err != nil || len(all) != 3 {
		t.Fatalf("isolated case derivation inventory mismatch: threads=%#v err=%v", all, err)
	}

	service := threadapp.NewService(threadapp.Dependencies{Repository: store, PublicProjector: projector})
	listed, err := service.List(threadapp.ListInput{Search: "CASE_DRAFT_SENTINEL", IncludeArchived: true, IncludeSide: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("case draft remained searchable through public summaries: %#v", listed)
	}
	projects, _, err := service.ListCaseProjectSummaries(0)
	if err != nil {
		t.Fatal(err)
	}
	assertNoCaseDraftSentinel(t, projects)

	if _, err := service.Compact(context.Background(), threadID, "marker downgrade attack"); !errors.Is(err, threadapp.ErrThreadRunning) {
		t.Fatalf("running marker-stripped case compaction did not fail closed: %v", err)
	}
	compacted, readErr := store.GetThread(threadID)
	if readErr != nil {
		t.Fatal(readErr)
	}
	compactedProjection, projectionErr := projector.ProjectThread(compacted)
	if projectionErr != nil {
		t.Fatal(projectionErr)
	}
	assertNoCaseDraftSentinel(t, compactedProjection)
}

func assertNoCaseDraftSentinel(t *testing.T, value any) {
	t.Helper()
	body, _ := json.Marshal(value)
	for _, sentinel := range []string{"CASE_DRAFT_SENTINEL", "CASE_EVENT_SENTINEL", "CASE_APPEND_SENTINEL"} {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("case authority projection leaked %s: %s", sentinel, body)
		}
	}
}

func (*caseThreadAuthorityStub) RestartPreservesThreadV1(string) bool { return false }
