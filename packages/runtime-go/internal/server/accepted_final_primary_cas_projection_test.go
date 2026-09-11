package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type durablePrimaryCASProjectionFixture struct {
	store          *DurableEventSessionStore
	casReader      *finalauthority.AcceptedFinalCASReader
	projector      *threadapp.TrustedPublicProjector
	service        *threadapp.Service
	threadID       string
	turnID         string
	threadPath     string
	messagesPath   string
	acceptedDigest string
	turnCASDigest  string
	renderedText   string
	terminalEvent  map[string]any
}

func newDurablePrimaryCASProjectionFixture(t *testing.T, largeProjectionNumber bool) durablePrimaryCASProjectionFixture {
	t.Helper()
	root := t.TempDir()
	durableRoot := filepath.Join(root, "durable")
	privateRoot := filepath.Join(root, "private")
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "strict primary projection"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_strict_primary_projection"
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-strict-primary-projection",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")),
		ContextEpoch: 5, IssuedAt: time.Unix(30, 0),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"items": []any{map[string]any{
			"id": "user_strict_primary", "threadId": threadID, "turnId": turnID,
			"kind": "user_message", "role": "user", "status": "completed", "text": "USER_REQUEST",
		}},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	threadPath := store.threadPath(threadID)
	if largeProjectionNumber {
		rewritePrimaryThreadJSON(t, threadPath, func(primary map[string]any) {
			primaryTurnByID(t, primary, turnID)["projectionNumber"] = json.Number("9007199254740992")
		})
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistry.NewStore(filepath.Join(privateRoot, "evidence-registry"), authority)
	if err != nil {
		t.Fatal(err)
	}
	privateStore, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(store, casReader)
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	finalizer := evidenceapp.NewCasePublicationFinalizerWithPublicationSnapshots(
		registry, registry, authority, privateStore, eventIO,
		newServerTestTurnTerminalCoordinator(t, authority, privateStore), nil, index,
	)
	result, err := finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Store: store, Context: securityContext, TerminalReason: evidenceapp.TerminalProviderFailure,
		ThreadID: threadID, TurnID: turnID, AcceptedAt: time.Unix(31, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, disposition, found := index.ResolveCommitted(threadID, turnID)
	if !found {
		t.Fatal("committed projection authority is missing")
	}
	projector := threadapp.NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, casReader)
	service := threadapp.NewService(threadapp.Dependencies{Repository: store, PublicProjector: projector})
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Diagnostics) != 0 {
		t.Fatalf("load terminal events: diagnostics=%#v err=%v", replay.Diagnostics, err)
	}
	var terminalEvent map[string]any
	for _, event := range replay.Events {
		if stringField(event, "publicationCommitId") == result.Persistence.AcceptedFinal.RecordDigest && stringField(event, "publicationSlot") == "terminal" {
			terminalEvent = cloneMap(event)
			break
		}
	}
	if terminalEvent == nil {
		t.Fatal("terminal publication event is missing")
	}
	fixture := durablePrimaryCASProjectionFixture{
		store: store, casReader: casReader, projector: projector, service: service,
		threadID: threadID, turnID: turnID, threadPath: threadPath, messagesPath: store.messagesPath(threadID),
		acceptedDigest: result.Persistence.AcceptedFinal.RecordDigest, turnCASDigest: disposition.TurnCASDigest,
		renderedText: result.Boundary.Text, terminalEvent: terminalEvent,
	}
	fixture.assertVisible(t)
	return fixture
}

func (fixture durablePrimaryCASProjectionFixture) assertVisible(t *testing.T) {
	t.Helper()
	snapshot, err := fixture.service.Get(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(snapshot)
	if !bytes.Contains(body, []byte(fixture.acceptedDigest)) || !bytes.Contains(body, []byte(fixture.renderedText)) {
		t.Fatalf("baseline strict-primary snapshot is not visible: %s", body)
	}
	projectEvent := threadapp.NewPublicEventProjector(fixture.store, fixture.projector)
	event, visible, revocationCode := projectEvent(fixture.threadID, fixture.terminalEvent)
	if !visible || event == nil || revocationCode != "" || stringField(event, "acceptedFinalDigest") != fixture.acceptedDigest {
		t.Fatalf("baseline strict-primary SSE is not visible: event=%#v visible=%t code=%q", event, visible, revocationCode)
	}
}

func (fixture durablePrimaryCASProjectionFixture) assertRevoked(t *testing.T) {
	t.Helper()
	if observation, err := fixture.casReader.ReadAcceptedFinalCASObservation(context.Background(), fixture.threadID, fixture.turnID); err == nil && observation.TurnProjectionSHA256 == fixture.turnCASDigest {
		t.Fatalf("tampered primary retained committed CAS authority: %#v", observation)
	}
	snapshot, err := fixture.service.Get(fixture.threadID)
	if err == nil && snapshot != nil {
		body, _ := json.Marshal(snapshot)
		if bytes.Contains(body, []byte(fixture.acceptedDigest)) || bytes.Contains(body, []byte(fixture.renderedText)) {
			t.Fatalf("tampered primary remained snapshot-visible: %s", body)
		}
	}
	projectEvent := threadapp.NewPublicEventProjector(fixture.store, fixture.projector)
	if event, visible, revocationCode := projectEvent(fixture.threadID, fixture.terminalEvent); visible || event != nil || revocationCode != threadapp.CasePublicAuthorityRejectedCode {
		t.Fatalf("tampered primary did not revoke SSE: event=%#v visible=%t code=%q", event, visible, revocationCode)
	}
}

func TestAcceptedFinalProjectionRejectsPrimaryTamperingHiddenByRenderingViews(t *testing.T) {
	t.Run("cleared primary items cannot hydrate from a rendering sidecar", func(t *testing.T) {
		fixture := newDurablePrimaryCASProjectionFixture(t, false)
		beforeSidecar, err := os.ReadFile(fixture.messagesPath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read rendering sidecar baseline: %v", err)
		}
		beforeExists := err == nil
		if len(bytes.TrimSpace(beforeSidecar)) != 0 {
			t.Fatalf("case terminal output escaped into a rendering sidecar: %s", beforeSidecar)
		}
		rewritePrimaryThreadJSON(t, fixture.threadPath, func(primary map[string]any) {
			primaryTurnByID(t, primary, fixture.turnID)["items"] = []any{}
		})
		afterSidecar, err := os.ReadFile(fixture.messagesPath)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read rendering sidecar after primary mutation: %v", err)
		}
		if (err == nil) != beforeExists || !bytes.Equal(beforeSidecar, afterSidecar) {
			t.Fatalf("primary mutation changed rendering-sidecar existence or bytes: before=%q after=%q err=%v", beforeSidecar, afterSidecar, err)
		}
		fixture.assertRevoked(t)
	})

	t.Run("reasoning removed by normalizer still changes primary CAS", func(t *testing.T) {
		fixture := newDurablePrimaryCASProjectionFixture(t, false)
		rewritePrimaryThreadJSON(t, fixture.threadPath, func(primary map[string]any) {
			primaryTurnByID(t, primary, fixture.turnID)["reasoningContent"] = "PRIVATE_REASONING_SENTINEL"
		})
		normalized, err := fixture.store.GetThread(fixture.threadID)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(normalized)
		if bytes.Contains(body, []byte("PRIVATE_REASONING_SENTINEL")) || bytes.Contains(body, []byte("reasoningContent")) {
			t.Fatalf("normalizer exposed private reasoning: %s", body)
		}
		fixture.assertRevoked(t)
	})

	t.Run("duplicate JSON key is rejected by both canonical read and strict CAS", func(t *testing.T) {
		fixture := newDurablePrimaryCASProjectionFixture(t, false)
		body, err := os.ReadFile(fixture.threadPath)
		if err != nil {
			t.Fatal(err)
		}
		needle := []byte(`"id":"` + fixture.turnID + `"`)
		if bytes.Count(body, needle) != 1 {
			t.Fatalf("turn identity raw anchor is not unique: %s", body)
		}
		replacement := []byte(`"id":"` + fixture.turnID + `","id":"` + fixture.turnID + `"`)
		if err := os.WriteFile(fixture.threadPath, bytes.Replace(body, needle, replacement, 1), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.store.GetThread(fixture.threadID); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
			t.Fatalf("canonical thread read accepted a duplicate JSON key: %v", err)
		}
		if _, err := fixture.casReader.ReadAcceptedFinalCASObservation(context.Background(), fixture.threadID, fixture.turnID); err == nil {
			t.Fatal("strict primary CAS accepted a duplicate JSON key")
		}
		if snapshot, err := fixture.service.Get(fixture.threadID); err == nil || snapshot != nil {
			t.Fatalf("duplicate-key primary remained snapshot-readable: snapshot=%#v err=%v", snapshot, err)
		}
		projectEvent := threadapp.NewPublicEventProjector(fixture.store, fixture.projector)
		if event, visible, code := projectEvent(fixture.threadID, fixture.terminalEvent); visible || event != nil || code != threadapp.CasePublicAuthorityUnavailableCode {
			t.Fatalf("duplicate-key primary did not fail closed at SSE: event=%#v visible=%t code=%q", event, visible, code)
		}
	})

	t.Run("float64 collision cannot hide exact primary number mutation", func(t *testing.T) {
		fixture := newDurablePrimaryCASProjectionFixture(t, true)
		body, err := os.ReadFile(fixture.threadPath)
		if err != nil {
			t.Fatal(err)
		}
		before := []byte(`"projectionNumber":9007199254740992`)
		after := []byte(`"projectionNumber":9007199254740993`)
		if bytes.Count(body, before) != 1 {
			t.Fatalf("exact-number raw anchor is not unique: %s", body)
		}
		if err := os.WriteFile(fixture.threadPath, bytes.Replace(body, before, after, 1), 0o600); err != nil {
			t.Fatal(err)
		}
		normalized, err := fixture.store.GetThread(fixture.threadID)
		if err != nil {
			t.Fatal(err)
		}
		normalizedBody, _ := json.Marshal(normalized)
		if !bytes.Contains(normalizedBody, before) || bytes.Contains(normalizedBody, after) {
			t.Fatalf("loose rendering view did not demonstrate float64 collision: %s", normalizedBody)
		}
		fixture.assertRevoked(t)
	})
}

func rewritePrimaryThreadJSON(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	primary := map[string]any{}
	if err := decoder.Decode(&primary); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("primary JSON contains trailing data: %v", err)
	}
	mutate(primary)
	body, err = json.Marshal(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func primaryTurnByID(t *testing.T, primary map[string]any, turnID string) map[string]any {
	t.Helper()
	var matched map[string]any
	turns, _ := primary["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		if strings.TrimSpace(stringField(turn, "id")) != turnID {
			continue
		}
		if matched != nil {
			t.Fatal("primary contains duplicate fixture turn")
		}
		matched = turn
	}
	if matched == nil {
		t.Fatal("primary fixture turn is missing")
	}
	return matched
}
