package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
	"analytix.local/runtime-go/internal/server"
	caseterminaltest "analytix.local/runtime-go/internal/testsupport/caseturnterminal"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

var (
	errInjectedTerminalDisposition = errors.New("injected terminal disposition crash")
	errInjectedProviderClosure     = errors.New("injected provider closure crash")
	errInjectedAcceptedDisposition = errors.New("injected accepted-final disposition crash")
)

type runtimeTerminalStartupFixtureV1 struct {
	config            Config
	store             *server.DurableEventSessionStore
	securityContext   domainsecurity.TurnSecurityContext
	authority         *finalauthority.FileAuthority
	registry          *evidenceregistry.Store
	privateStore      *finalauthority.PrivateStore
	casReader         *finalauthority.AcceptedFinalCASReader
	eventIO           evidenceapp.FinalPublicationEventIO
	providerTelemetry *cachetelemetryapp.DurableService
	providerStore     *cachetelemetrystore.Store
	terminalStore     *turnterminalstore.Store
	prePublication    []map[string]any
	acceptedEventPath string
	privateAccess     *privatecastest.AccessAuthority
	privateRoot       string
	threadID          string
	turnID            string
}

func newRuntimeTerminalStartupFixtureV1(t *testing.T, suffix string) *runtimeTerminalStartupFixtureV1 {
	t.Helper()
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	workspace := workspacetest.New(t)
	store, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "turn terminal startup " + suffix}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	turnID := "turn_terminal_startup_" + suffix
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-terminal-startup-" + suffix,
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-" + suffix)),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("snapshot-terminal-startup-" + suffix),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-" + suffix)), ContextEpoch: 2, IssuedAt: time.Unix(10, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{
		Thread: map[string]any{}, SecurityContext: securityContext, At: time.Unix(10, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{},
	}, "deepseek", map[string]any{
		"securityState": contextRecord, "contextEpochState": contextepochapp.PublicState(epoch.State),
	}); err != nil {
		t.Fatal(err)
	}
	replay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(replay.Diagnostics) != 0 {
		t.Fatalf("capture pre-publication events: diagnostics=%#v err=%v", replay.Diagnostics, err)
	}
	privateRoot := filepath.Join(dataDir, "private")
	authority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistry.NewStore(filepath.Join(privateRoot, "evidence-registry"), authority)
	if err != nil {
		t.Fatal(err)
	}
	privateAccess, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	privateStore, err := finalauthority.NewPrivateStore(filepath.Join(privateRoot, "accepted-finals"), privateAccess)
	if err != nil {
		t.Fatal(err)
	}
	providerStore, err := cachetelemetrystore.NewStore(filepath.Join(privateRoot, "provider-cache-telemetry"), privateAccess)
	if err != nil {
		t.Fatal(err)
	}
	providerTelemetry, err := cachetelemetryapp.NewDurableService(authority, providerStore)
	if err != nil {
		t.Fatal(err)
	}
	terminalStore, err := turnterminalstore.NewStore(filepath.Join(privateRoot, "turn-terminal-authority"), privateAccess)
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	return &runtimeTerminalStartupFixtureV1{
		config: Config{RuntimeToken: DefaultRuntimeToken, DataDir: dataDir, DurableTempDir: durableRoot},
		store:  store, securityContext: securityContext, authority: authority, registry: registry,
		privateStore: privateStore, casReader: casReader, eventIO: startupFinalEventIOForTest(store, casReader),
		providerTelemetry: providerTelemetry, providerStore: providerStore, terminalStore: terminalStore,
		prePublication: replay.Events, acceptedEventPath: filepath.Join(durableRoot, "threads", threadID, "events.jsonl"),
		privateAccess: privateAccess, privateRoot: privateRoot, threadID: threadID, turnID: turnID,
	}
}

func (fixture *runtimeTerminalStartupFixtureV1) persistBoundaryV1(
	t *testing.T,
	coordinator *turnterminalapp.Coordinator,
) (evidenceapp.PersistCaseBoundaryResult, error) {
	t.Helper()
	finalizer := evidenceapp.NewCasePublicationFinalizerWithAuthority(
		fixture.registry, fixture.registry, fixture.authority, fixture.privateStore, fixture.eventIO, coordinator,
	)
	return finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Store: fixture.store, Context: fixture.securityContext, TerminalReason: evidenceapp.TerminalProviderFailure,
		ThreadID: fixture.threadID, TurnID: fixture.turnID, AcceptedAt: time.Unix(11, 0),
	})
}

type failTerminalDispositionStoreV1 struct {
	turnterminalstoreport.Store
}

func (*failTerminalDispositionStoreV1) PutDispositionIfAbsent(context.Context, domainturnterminal.TurnTerminalDispositionV1) error {
	return errInjectedTerminalDisposition
}

type failProviderClosureV1 struct {
	turnterminalapp.ProviderTurnAuthority
}

type failAcceptedDispositionStoreV1 struct {
	finalauthorityport.PrivateFinalStore
}

func (*failAcceptedDispositionStoreV1) PutDispositionIfAbsent(
	context.Context,
	domainevidence.AcceptedFinalDispositionRecord,
) error {
	return errInjectedAcceptedDisposition
}

func (*failProviderClosureV1) CloseTurn(
	context.Context,
	domainsecurity.TurnSecurityContext,
	domaincachetelemetry.ProviderTurnTerminalReasonV1,
	time.Time,
) (domaincachetelemetry.ProviderTurnClosureV1, error) {
	return domaincachetelemetry.ProviderTurnClosureV1{}, errInjectedProviderClosure
}

func TestRuntimeStartupCompletesTerminalPrefixAndSecondRestartIsStable(t *testing.T) {
	fixture := newRuntimeTerminalStartupFixtureV1(t, "complete_prefix")
	coordinator, err := turnterminalapp.NewCoordinator(
		fixture.authority,
		fixture.privateStore,
		&failTerminalDispositionStoreV1{Store: fixture.terminalStore},
		fixture.providerTelemetry,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.persistBoundaryV1(t, coordinator); !errors.Is(err, errInjectedTerminalDisposition) {
		t.Fatalf("expected crash after accepted-final disposition, got %v", err)
	}
	privateRecords, err := fixture.privateStore.List(context.Background())
	if err != nil || len(privateRecords) != 1 {
		t.Fatalf("read private final prefix: count=%d err=%v", len(privateRecords), err)
	}
	privateRecord := privateRecords[0]
	if _, err := fixture.terminalStore.ReadIntent(context.Background(), fixture.securityContext.ContextDigest); err != nil {
		t.Fatalf("terminal prefix lacks intent: %v", err)
	}
	if _, found, err := fixture.providerTelemetry.ObserveTurnClosureV1(context.Background(), fixture.securityContext); err != nil || !found {
		t.Fatalf("terminal prefix lacks provider closure: found=%v err=%v", found, err)
	}
	observation, err := fixture.casReader.ReadAcceptedFinalCASObservation(context.Background(), fixture.threadID, fixture.turnID)
	if err != nil || !observation.HasWinner || observation.Winner.RecordDigest != privateRecord.AcceptedFinal.RecordDigest {
		t.Fatalf("terminal prefix lacks public winner: observation=%#v err=%v", observation, err)
	}
	if _, err := fixture.privateStore.ResolveDisposition(context.Background(), privateRecord.AcceptedFinal.RecordDigest); err != nil {
		t.Fatalf("terminal prefix lacks accepted-final disposition: %v", err)
	}
	if _, err := fixture.terminalStore.ReadDisposition(context.Background(), fixture.securityContext.ContextDigest); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
		t.Fatalf("terminal disposition existed before restart recovery: %v", err)
	}

	first, err := NewRuntimeServerHandlerE(fixture.config)
	if err != nil {
		t.Fatalf("recover valid terminal prefix: %v", err)
	}
	detail := runtimeStartupJSON(t, first, http.MethodGet, "/v1/threads/"+fixture.threadID, nil, http.StatusOK)
	if _, present := detail["acceptedFinalDelivery"]; present {
		t.Fatalf("all-invisible accepted-final manifest produced a public hydration batch: %#v", detail["acceptedFinalDelivery"])
	}
	if _, present := detail["acceptedFinalDeliveries"]; present {
		t.Fatalf("all-invisible accepted-final manifest produced public per-turn delivery authority: %#v", detail["acceptedFinalDeliveries"])
	}
	detailBody, _ := json.Marshal(detail)
	if bytes.Contains(detailBody, []byte(privateRecord.RenderedText)) || bytes.Contains(detailBody, []byte(privateRecord.AcceptedFinal.RecordDigest)) {
		t.Fatalf("synthetic case authority unexpectedly entered the public projection: %s", detailBody)
	}
	shutdownOwnedRuntimeHandler(t, first)
	terminalDispositionRoot := filepath.Join(fixture.privateRoot, "turn-terminal-authority", "dispositions")
	if files := runtimeStartupRegularFileCount(t, terminalDispositionRoot); files != 1 {
		t.Fatalf("restart committed %d terminal disposition files, want exactly 1", files)
	}
	firstTerminalDispositionDigest := startupWholeTreeDigest(t, terminalDispositionRoot)
	rawThread, err := fixture.store.GetThread(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := fixture.store.LoadEventsSince(fixture.threadID, 0)
	if err != nil || len(replay.Diagnostics) != 0 {
		t.Fatalf("load repaired accepted-final events: diagnostics=%#v err=%v", replay.Diagnostics, err)
	}
	if err := evidenceapp.ValidateAcceptedFinalEventReplay(rawThread, replay.Events); err != nil {
		t.Fatalf("restart did not repair the durable accepted-final event authority: %v", err)
	}
	publicationEvents := 0
	for _, event := range replay.Events {
		if event["publicationCommitId"] == privateRecord.AcceptedFinal.RecordDigest {
			publicationEvents++
		}
	}
	expectedPublication, err := appturn.BuildAcceptedFinalPublicationPlan(
		privateRecord.AcceptedFinal, privateRecord.RenderedText, privateRecord.PublicationIntent,
	)
	if err != nil {
		t.Fatal(err)
	}
	if publicationEvents != len(expectedPublication.Events) {
		t.Fatalf("restart repaired %d accepted-final events, want exactly %d", publicationEvents, len(expectedPublication.Events))
	}
	firstEvents, err := os.ReadFile(fixture.acceptedEventPath)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewRuntimeServerHandlerE(fixture.config)
	if err != nil {
		t.Fatalf("second terminal-complete restart failed: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, second)
	if secondTerminalDispositionDigest := startupWholeTreeDigest(t, terminalDispositionRoot); secondTerminalDispositionDigest != firstTerminalDispositionDigest {
		t.Fatalf("second restart changed terminal disposition: before=%s after=%s", firstTerminalDispositionDigest, secondTerminalDispositionDigest)
	}
	secondEvents, err := os.ReadFile(fixture.acceptedEventPath)
	if err != nil || !bytes.Equal(secondEvents, firstEvents) {
		t.Fatalf("second restart duplicated or changed accepted-final events: err=%v", err)
	}
}

func TestAcceptedFinalRestartRejectsCorruptEventSequence(t *testing.T) {
	tests := []struct {
		name    string
		corrupt func([]map[string]any)
	}{
		{
			name: "duplicate",
			corrupt: func(events []map[string]any) {
				events[1]["seq"] = events[0]["seq"]
			},
		},
		{
			name: "gap",
			corrupt: func(events []map[string]any) {
				first, _ := contracts.NumericSeq(events[0]["seq"])
				events[1]["seq"] = float64(first + 2)
			},
		},
		{
			name: "backward",
			corrupt: func(events []map[string]any) {
				events[0], events[1] = events[1], events[0]
			},
		},
		{
			name: "wrong_thread",
			corrupt: func(events []map[string]any) {
				events[0]["threadId"] = "thr_foreign_restart_sequence"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRuntimeTerminalStartupFixtureV1(t, "corrupt_sequence_"+test.name)
			coordinator, err := turnterminalapp.NewCoordinator(
				fixture.authority,
				fixture.privateStore,
				&failTerminalDispositionStoreV1{Store: fixture.terminalStore},
				fixture.providerTelemetry,
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.persistBoundaryV1(t, coordinator); !errors.Is(err, errInjectedTerminalDisposition) {
				t.Fatalf("expected terminal-prefix crash before restart repair, got %v", err)
			}

			events := []map[string]any{
				{"seq": float64(1), "kind": "heartbeat", "threadId": fixture.threadID},
				{"seq": float64(2), "kind": "heartbeat", "threadId": fixture.threadID},
			}
			test.corrupt(events)
			writeStartupEvents(t, fixture.acceptedEventPath, events)
			beforeEvents, err := os.ReadFile(fixture.acceptedEventPath)
			if err != nil {
				t.Fatal(err)
			}
			beforeTree := startupWholeTreeDigest(t, fixture.config.DataDir, fixture.config.DurableTempDir)

			var providerCalls atomic.Int64
			endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				providerCalls.Add(1)
			}))
			defer endpoint.Close()
			providers, err := json.Marshal(map[string]any{
				"defaultProviderId": "provider-restart-sequence",
				"providers": []map[string]any{{
					"id": "provider-restart-sequence", "apiKey": "test-key", "baseUrl": endpoint.URL + "/v1",
					"endpointFormat": "chat_completions", "models": []string{"model-test"},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			fixture.config.ModelProvidersJSON = string(providers)

			handler, startupErr := NewRuntimeServerHandlerE(fixture.config)
			if startupErr == nil {
				if handler != nil {
					shutdownOwnedRuntimeHandler(t, handler)
				}
				t.Fatal("corrupt accepted-final event sequence passed production restart")
			}
			if handler != nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatalf("corrupt restart exposed a public handler: %T", handler)
			}
			if !strings.Contains(startupErr.Error(), "thread_record_identity") {
				t.Fatalf("corrupt restart failed outside the raw event authority gate: %v", startupErr)
			}
			if providerCalls.Load() != 0 {
				t.Fatalf("corrupt restart activated provider calls: %d", providerCalls.Load())
			}
			afterEvents, readErr := os.ReadFile(fixture.acceptedEventPath)
			if readErr != nil || !bytes.Equal(afterEvents, beforeEvents) {
				t.Fatalf("corrupt restart repaired or changed public replay: err=%v", readErr)
			}
			if afterTree := startupWholeTreeDigest(t, fixture.config.DataDir, fixture.config.DurableTempDir); afterTree != beforeTree {
				t.Fatalf("corrupt restart mutated live persistence: before=%s after=%s", beforeTree, afterTree)
			}
			if _, err := fixture.terminalStore.ReadDisposition(context.Background(), fixture.securityContext.ContextDigest); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
				t.Fatalf("corrupt restart wrote a terminal repair disposition: %v", err)
			}
		})
	}
}
func runtimeStartupRegularFileCount(t *testing.T, root string) int {
	t.Helper()
	count := 0
	if err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestRuntimeStartupRejectsPublicCommitBeforeProviderClosureWithoutMutation(t *testing.T) {
	fixture := newRuntimeTerminalStartupFixtureV1(t, "public_before_closure")
	coordinator, err := turnterminalapp.NewCoordinator(
		fixture.authority,
		fixture.privateStore,
		fixture.terminalStore,
		&failProviderClosureV1{ProviderTurnAuthority: fixture.providerTelemetry},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.persistBoundaryV1(t, coordinator); !errors.Is(err, errInjectedProviderClosure) {
		t.Fatalf("expected crash after terminal intent, got %v", err)
	}
	privateRecords, err := fixture.privateStore.List(context.Background())
	if err != nil || len(privateRecords) != 1 {
		t.Fatalf("read private final after intent crash: count=%d err=%v", len(privateRecords), err)
	}
	privateRecord := privateRecords[0]
	if _, err := appturn.PersistAcceptedFinalTerminal(appturn.PersistAcceptedFinalInput{
		Store: fixture.store, ThreadID: fixture.threadID, TurnID: fixture.turnID,
		RenderedText: privateRecord.RenderedText, AcceptedFinal: privateRecord.AcceptedFinal,
		PublicationIntent: privateRecord.PublicationIntent, PrivateFinal: privateRecord,
	}); err != nil {
		t.Fatalf("seed invalid public-before-closure order: %v", err)
	}
	before := startupWholeTreeDigest(t, fixture.config.DataDir, fixture.config.DurableTempDir)
	if handler, err := NewRuntimeServerHandlerE(fixture.config); err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("public commit before provider closure passed production startup")
	} else if !strings.Contains(err.Error(), "public winner lacks its prior provider closure") {
		t.Fatalf("unexpected invalid-order startup error: %v", err)
	}
	if after := startupWholeTreeDigest(t, fixture.config.DataDir, fixture.config.DurableTempDir); after != before {
		t.Fatalf("invalid terminal order mutated live persistence: before=%s after=%s", before, after)
	}
	if _, found, err := fixture.providerTelemetry.ObserveTurnClosureV1(context.Background(), fixture.securityContext); err != nil || found {
		t.Fatalf("startup backfilled provider closure: found=%v err=%v", found, err)
	}
	if _, err := fixture.terminalStore.ReadDisposition(context.Background(), fixture.securityContext.ContextDigest); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
		t.Fatalf("startup backfilled terminal disposition after invalid order: %v", err)
	}
}

func TestRuntimeStartupQuarantinesLegacyPublicWinnerWithoutBackfillOrProjection(t *testing.T) {
	fixture := newRuntimeTerminalStartupFixtureV1(t, "legacy_public_winner")
	failingPrivateStore := &failAcceptedDispositionStoreV1{PrivateFinalStore: fixture.privateStore}
	coordinator, err := caseterminaltest.NewInMemoryCoordinatorV1(fixture.authority, failingPrivateStore)
	if err != nil {
		t.Fatal(err)
	}
	finalizer := evidenceapp.NewCasePublicationFinalizerWithAuthority(
		fixture.registry, fixture.registry, fixture.authority, failingPrivateStore, fixture.eventIO, coordinator,
	)
	if _, err := finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Store: fixture.store, Context: fixture.securityContext, TerminalReason: evidenceapp.TerminalProviderFailure,
		ThreadID: fixture.threadID, TurnID: fixture.turnID, AcceptedAt: time.Unix(11, 0),
	}); !errors.Is(err, errInjectedAcceptedDisposition) {
		t.Fatalf("expected legacy crash after public commit, got %v", err)
	}
	privateRecords, err := fixture.privateStore.List(context.Background())
	if err != nil || len(privateRecords) != 1 {
		t.Fatalf("read legacy private final: count=%d err=%v", len(privateRecords), err)
	}
	privateRecord := privateRecords[0]
	writeStartupEvents(t, fixture.acceptedEventPath, fixture.prePublication)
	if _, err := fixture.terminalStore.ReadIntent(context.Background(), fixture.securityContext.ContextDigest); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
		t.Fatalf("legacy fixture unexpectedly persisted a terminal intent: %v", err)
	}

	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err != nil {
		t.Fatalf("legacy public winner should remain audit-only rather than block startup: %v", err)
	}
	for _, path := range []string{
		"/v1/threads/" + fixture.threadID,
		"/v1/threads/" + fixture.threadID + "/events?since_seq=0",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		body := response.Body.String()
		if strings.Contains(body, privateRecord.RenderedText) || strings.Contains(body, privateRecord.AcceptedFinal.RecordDigest) {
			t.Fatalf("legacy public winner entered public projection for %s: status=%d body=%s", path, response.Code, body)
		}
	}
	shutdownOwnedRuntimeHandler(t, handler)
	if _, err := fixture.terminalStore.ReadIntent(context.Background(), fixture.securityContext.ContextDigest); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
		t.Fatalf("startup backfilled a legacy terminal intent: %v", err)
	}
	if _, err := fixture.terminalStore.ReadDisposition(context.Background(), fixture.securityContext.ContextDigest); !errors.Is(err, turnterminalstoreport.ErrNotFound) {
		t.Fatalf("startup backfilled a legacy terminal disposition: %v", err)
	}
	if _, found, err := fixture.providerTelemetry.ObserveTurnClosureV1(context.Background(), fixture.securityContext); err != nil || found {
		t.Fatalf("startup backfilled a legacy provider closure: found=%v err=%v", found, err)
	}
}
