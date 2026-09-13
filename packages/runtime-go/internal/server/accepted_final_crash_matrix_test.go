package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appturnterminal "analytix.local/runtime-go/internal/app/turnterminal"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type durableAcceptedFinalCrashFixture struct {
	root           string
	durableRoot    string
	privateRoot    string
	threadID       string
	turnID         string
	security       domainsecurity.TurnSecurityContext
	acceptedDigest string
	beforeFiles    map[string][]byte
	beforeEvents   []map[string]any
	finalEvents    []map[string]any
}

type durableAcceptedFinalRestartOutcomeV1 string

const (
	durableRestartCompletesPreparedV1 durableAcceptedFinalRestartOutcomeV1 = "completes_prepared"
	durableRestartQuarantinesLegacyV1 durableAcceptedFinalRestartOutcomeV1 = "quarantines_legacy"
	durableRestartRejectsDetachedV1   durableAcceptedFinalRestartOutcomeV1 = "rejects_detached"
)

type durableAcceptedFinalCASRaceRegistry struct{}

func (durableAcceptedFinalCASRaceRegistry) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	return domainevidence.EvidenceReceipt{}, errors.New("durable CAS race registry is read-only")
}

func (durableAcceptedFinalCASRaceRegistry) Resolve(context.Context, registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	return domainevidence.RegisteredEvidence{}, errors.New("durable CAS race registry has no evidence")
}

func (durableAcceptedFinalCASRaceRegistry) Revoke(context.Context, registryport.RevokeInput) error {
	return errors.New("durable CAS race registry is read-only")
}

func (durableAcceptedFinalCASRaceRegistry) Replay(_ context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	return domainevidence.NewEvidenceReceiptRegistry(securityContext)
}

func (registry durableAcceptedFinalCASRaceRegistry) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, callback func(domainevidence.EvidenceReceiptRegistry) error) error {
	if callback == nil {
		return errors.New("durable CAS race snapshot callback is required")
	}
	snapshot, err := registry.Replay(ctx, securityContext)
	if err != nil {
		return err
	}
	return callback(snapshot)
}

func (registry durableAcceptedFinalCASRaceRegistry) ReplayAt(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	if sequence != 0 {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("durable CAS race historical sequence is unavailable")
	}
	return registry.Replay(ctx, securityContext)
}

func TestAcceptedFinalDurableCrashCutMatrix(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, durableAcceptedFinalCrashFixture)
		outcome durableAcceptedFinalRestartOutcomeV1
	}{
		{
			name: "post-private-pre-cas",
			mutate: func(t *testing.T, fixture durableAcceptedFinalCrashFixture) {
				t.Helper()
				restoreDurableThreadFiles(t, fixture)
				removeDurableDisposition(t, fixture)
			},
			outcome: durableRestartCompletesPreparedV1,
		},
		{
			name: "post-cas-pre-disposition",
			mutate: func(t *testing.T, fixture durableAcceptedFinalCrashFixture) {
				t.Helper()
				removeDurableDisposition(t, fixture)
				writeDurableAcceptedFinalEvents(t, fixture, fixture.beforeEvents)
			},
			outcome: durableRestartQuarantinesLegacyV1,
		},
		{
			name: "post-disposition-pre-events",
			mutate: func(t *testing.T, fixture durableAcceptedFinalCrashFixture) {
				t.Helper()
				writeDurableAcceptedFinalEvents(t, fixture, fixture.beforeEvents)
			},
			outcome: durableRestartRejectsDetachedV1,
		},
		{name: "post-events-pre-projection", mutate: func(*testing.T, durableAcceptedFinalCrashFixture) {}, outcome: durableRestartRejectsDetachedV1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDurableAcceptedFinalCrashFixture(t)
			test.mutate(t, fixture)
			assertDurableAcceptedFinalRestartOutcomeV1(t, fixture, test.outcome)
		})
	}
}

func TestAcceptedFinalDurableEventSlotCrashMatrix(t *testing.T) {
	fixture := newDurableAcceptedFinalCrashFixture(t)
	for cut := 0; cut <= len(fixture.finalEvents); cut++ {
		t.Run(string(rune('0'+cut)), func(t *testing.T) {
			current := newDurableAcceptedFinalCrashFixture(t)
			events := append(cloneDurableEventMaps(current.beforeEvents), cloneDurableEventMaps(current.finalEvents[:cut])...)
			writeDurableAcceptedFinalEvents(t, current, events)
			assertDurableAcceptedFinalRestartOutcomeV1(t, current, durableRestartRejectsDetachedV1)
		})
	}
}

func TestGenericEventSinkRejectsExactAcceptedFinalWithoutAnySideEffect(t *testing.T) {
	fixture := newDurableAcceptedFinalCrashFixture(t)
	store, err := NewTempDurableEventSessionStore(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	beforeFiles := readDurableThreadFiles(t, filepath.Join(fixture.durableRoot, "threads", fixture.threadID))
	beforeHighest, err := store.HighestSeq(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	live, unsubscribe := store.SubscribeEvents(fixture.threadID)
	defer unsubscribe()

	candidate := cloneMap(fixture.finalEvents[0])
	delete(candidate, "seq")
	if _, _, err := store.RecordEvent(candidate); !errors.Is(err, domainevent.ErrAssistantDraftPersistence) {
		t.Fatalf("exact accepted-final manifest event bypassed the generic sink: %v", err)
	}
	if afterHighest, err := store.HighestSeq(fixture.threadID); err != nil || afterHighest != beforeHighest {
		t.Fatalf("rejected accepted-final event consumed a sequence: before=%d after=%d err=%v", beforeHighest, afterHighest, err)
	}
	afterFiles := readDurableThreadFiles(t, filepath.Join(fixture.durableRoot, "threads", fixture.threadID))
	if !reflect.DeepEqual(beforeFiles, afterFiles) {
		t.Fatal("rejected accepted-final event mutated durable thread files")
	}
	select {
	case event := <-live:
		t.Fatalf("rejected accepted-final event reached a live subscriber: %#v", event)
	case <-time.After(20 * time.Millisecond):
	}
}

// Freeze the actual initial inventories before either independent finalizer
// can create its private record. Otherwise a scheduler may serialize admission
// and correctly reject the second contender before it reaches the CAS race.
type durableFinalAdmissionBarrier struct {
	authorityport.PrivateFinalStore
	arrived chan struct{}
	release chan struct{}
}

func (store *durableFinalAdmissionBarrier) List(ctx context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	records, err := store.PrivateFinalStore.List(ctx)
	if err != nil {
		return nil, err
	}
	select {
	case store.arrived <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-store.release:
		return records, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestAcceptedFinalDurableConcurrentTerminalHasOneWinnerAndQuarantinesContender(t *testing.T) {
	root := workspacetest.New(t)
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
	thread, err := store.CreateThread(map[string]any{"title": "accepted-final durable CAS race"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_durable_cas_race"
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-durable-cas-race",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 7, IssuedAt: time.Unix(20, 0),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	_ = json.Unmarshal(contextBody, &contextRecord)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json")
	authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	registry := durableAcceptedFinalCASRaceRegistry{}
	privateStore, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := durableAcceptedFinalEventIO(store, casReader)
	terminalCoordinator := newServerTestTurnTerminalCoordinator(t, authority, privateStore)
	admission := &durableFinalAdmissionBarrier{PrivateFinalStore: privateStore, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	finalizers := []evidenceapp.CasePublicationFinalizer{
		evidenceapp.NewCasePublicationFinalizerWithAuthority(registry, registry, authority, admission, eventIO, terminalCoordinator),
		evidenceapp.NewCasePublicationFinalizerWithAuthority(registry, registry, authority, admission, eventIO, terminalCoordinator),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type contenderResult struct {
		result evidenceapp.PersistCaseBoundaryResult
		err    error
	}
	results := make(chan contenderResult, len(finalizers))
	for index, finalizer := range finalizers {
		index, finalizer := index, finalizer
		go func() {
			result, persistErr := finalizer.PersistBoundary(ctx, evidenceapp.PersistCaseBoundaryInput{
				Store:   store,
				Context: securityContext, TerminalReason: evidenceapp.TerminalProviderFailure,
				ThreadID: threadID, TurnID: turnID, AcceptedAt: time.Unix(21+int64(index), 0),
			})
			results <- contenderResult{result: result, err: persistErr}
		}()
	}
	for range finalizers {
		select {
		case <-admission.arrived:
		case <-ctx.Done():
			t.Fatal("both contenders did not reach private admission before the CAS race")
		}
	}
	close(admission.release)
	contenders := make([]contenderResult, 0, len(finalizers))
	for range finalizers {
		select {
		case result := <-results:
			contenders = append(contenders, result)
		case <-ctx.Done():
			t.Fatal("durable CAS contender did not finish")
		}
	}
	var winnerDigest string
	succeeded, rejected := 0, 0
	for _, contender := range contenders {
		if contender.err == nil {
			succeeded++
			winnerDigest = contender.result.Persistence.AcceptedFinal.RecordDigest
			if !contender.result.Persistence.Changed {
				t.Fatalf("successful terminal contender did not win public CAS: %#v", contender.result)
			}
		} else {
			rejected++
			if contender.result.Persistence.Changed || contender.result.Persistence.AcceptedFinal.RecordDigest != "" {
				t.Fatalf("rejected terminal contender received public authority: %#v err=%v", contender.result, contender.err)
			}
		}
	}
	if succeeded != 1 || rejected != 1 || winnerDigest == "" {
		t.Fatalf("terminal authority did not elect one exact winner: contenders=%#v", contenders)
	}
	observation, err := casReader.ReadAcceptedFinalCASObservation(context.Background(), threadID, turnID)
	if err != nil || !observation.HasWinner || observation.Winner.RecordDigest != winnerDigest {
		t.Fatalf("durable CAS winner readback mismatch: observation=%#v err=%v", observation, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 {
		t.Fatalf("rejected contender acquired a durable disposition: dispositions=%#v err=%v", dispositions, err)
	}
	winnerDisposition := dispositions[0]
	if winnerDisposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		winnerDisposition.State != domainevidence.AcceptedFinalCommitted ||
		winnerDisposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		winnerDisposition.WinnerDigest != winnerDigest || winnerDisposition.TurnCASDigest != observation.TurnProjectionSHA256 {
		t.Fatalf("durable CAS winner disposition mismatch: %#v", winnerDisposition)
	}
	dispositionPath := filepath.Join(privateRoot, "accepted-finals", "dispositions", winnerDigest[:2], winnerDigest+".json")
	beforeDispositionBytes, err := os.ReadFile(dispositionPath)
	if err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopenedAuthority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	reopenedPrivate, err := newServerTestPrivateFinalStore(t, filepath.Join(privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	reopenedCAS, err := finalauthority.NewAcceptedFinalCASReader(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := evidenceapp.PreflightFinalAuthorityInventory(context.Background(), reopenedStore, reopenedCAS, registry, reopenedAuthority, reopenedPrivate)
	if err != nil || len(inventory.Committed) != 1 || len(inventory.NotCommitted) != 1 ||
		len(inventory.CommittedDispositions) != 1 || len(inventory.PublicCommitRepairs) != 0 ||
		inventory.Committed[0].AcceptedFinal.RecordDigest != winnerDigest ||
		inventory.CommittedDispositions[0].AcceptedFinalDigest != winnerDigest {
		t.Fatalf("durable CAS restart inventory mismatch: inventory=%#v err=%v", inventory, err)
	}
	afterDispositionBytes, readErr := os.ReadFile(dispositionPath)
	if readErr != nil || !reflect.DeepEqual(afterDispositionBytes, beforeDispositionBytes) {
		t.Fatalf("winner disposition bytes changed across restart preflight: err=%v", readErr)
	}
}

func newDurableAcceptedFinalCrashFixture(t *testing.T) durableAcceptedFinalCrashFixture {
	t.Helper()
	root := t.TempDir()
	durableRoot := filepath.Join(root, "durable")
	privateRoot := filepath.Join(root, "private")
	store, err := NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	workspaceSuffix := []byte(domainsecurity.SHA256Hex([]byte(root)))
	for index, character := range workspaceSuffix {
		if character >= '0' && character <= '9' {
			workspaceSuffix[index] = 'g' + character - '0'
		}
	}
	workspace := filepath.Join(os.TempDir(), "analytix-accepted-final-crash-workspace-"+string(workspaceSuffix))
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"title": "accepted-final crash matrix"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_crash_matrix"
	securityContext := newServerCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-crash-matrix",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 3, IssuedAt: time.Unix(10, 0),
	})
	contextBody, _ := json.Marshal(securityContext)
	contextRecord := map[string]any{}
	_ = json.Unmarshal(contextBody, &contextRecord)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord, "items": []any{},
	}, "deepseek", map[string]any{"securityState": contextRecord}); err != nil {
		t.Fatal(err)
	}
	beforeReplay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(beforeReplay.Diagnostics) != 0 {
		t.Fatalf("read baseline events: diagnostics=%#v err=%v", beforeReplay.Diagnostics, err)
	}
	beforeFiles := readDurableThreadFiles(t, filepath.Join(durableRoot, "threads", threadID))

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
	finalizer := evidenceapp.NewCasePublicationFinalizerWithAuthority(registry, registry, authority, privateStore, eventIO, newServerTestTurnTerminalCoordinator(t, authority, privateStore))
	result, err := finalizer.PersistBoundary(context.Background(), evidenceapp.PersistCaseBoundaryInput{
		Store: store, Context: securityContext, TerminalReason: evidenceapp.TerminalProviderFailure,
		ThreadID: threadID, TurnID: turnID, AcceptedAt: time.Unix(11, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	afterReplay, err := store.LoadEventsSince(threadID, 0)
	if err != nil || len(afterReplay.Diagnostics) != 0 || len(afterReplay.Events) <= len(beforeReplay.Events) {
		t.Fatalf("read committed events: diagnostics=%#v events=%#v err=%v", afterReplay.Diagnostics, afterReplay.Events, err)
	}
	return durableAcceptedFinalCrashFixture{
		root: root, durableRoot: durableRoot, privateRoot: privateRoot, threadID: threadID, turnID: turnID,
		security: securityContext, acceptedDigest: result.Persistence.AcceptedFinal.RecordDigest, beforeFiles: beforeFiles,
		beforeEvents: cloneDurableEventMaps(beforeReplay.Events),
		finalEvents:  cloneDurableEventMaps(afterReplay.Events[len(beforeReplay.Events):]),
	}
}

func readDurableThreadFiles(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = body
	}
	return files
}

func restoreDurableThreadFiles(t *testing.T, fixture durableAcceptedFinalCrashFixture) {
	t.Helper()
	directory := filepath.Join(fixture.durableRoot, "threads", fixture.threadID)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, keep := fixture.beforeFiles[entry.Name()]; !keep {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, body := range fixture.beforeFiles {
		if err := os.WriteFile(filepath.Join(directory, name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertDurableAcceptedFinalRestartOutcomeV1(
	t *testing.T,
	fixture durableAcceptedFinalCrashFixture,
	want durableAcceptedFinalRestartOutcomeV1,
) {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(fixture.privateRoot, "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := evidenceregistry.NewStore(filepath.Join(fixture.privateRoot, "evidence-registry"), authority)
	if err != nil {
		t.Fatal(err)
	}
	privateStore, err := newRecoveredServerTestPrivateFinalStore(t, filepath.Join(fixture.privateRoot, "accepted-finals"))
	if err != nil {
		t.Fatal(err)
	}
	casReader, err := finalauthority.NewAcceptedFinalCASReader(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	terminalCoordinator := newServerTestTurnTerminalCoordinator(t, authority, privateStore)
	inventory, err := evidenceapp.PreflightFinalAuthorityInventory(context.Background(), store, casReader, registry, authority, privateStore)
	if err != nil {
		t.Fatal(err)
	}
	privateInventory := append([]domainevidence.PrivateAcceptedFinalRecord{}, inventory.Committed...)
	privateInventory = append(privateInventory, inventory.NotCommitted...)
	candidates := append([]domainevidence.PrivateAcceptedFinalRecord{}, inventory.Committed...)
	for _, repair := range inventory.PublicCommitRepairs {
		privateInventory = append(privateInventory, repair.PrivateRecord)
		candidates = append(candidates, repair.PrivateRecord)
	}
	recovery, recoveryErr := terminalCoordinator.RecoverV1(context.Background(), appturnterminal.RestartRecoveryInputV1{
		CompletionStore: store, CASReader: casReader, PrivateInventory: privateInventory, Candidates: candidates,
	})
	switch want {
	case durableRestartCompletesPreparedV1:
		if recoveryErr != nil || len(recovery.Complete) != 1 || len(recovery.LegacyQuarantined) != 0 ||
			recovery.Complete[0].AcceptedFinalDisposition.AcceptedFinalDigest != fixture.acceptedDigest {
			t.Fatalf("valid private prefix did not complete through the sole coordinator: recovery=%#v err=%v", recovery, recoveryErr)
		}
		verified, err := evidenceapp.PreflightFinalAuthorityInventory(context.Background(), store, casReader, registry, authority, privateStore)
		if err != nil || len(verified.Committed) != 1 || len(verified.CommittedDispositions) != 1 || len(verified.PublicCommitRepairs) != 0 {
			t.Fatalf("completed private prefix did not converge: inventory=%#v err=%v", verified, err)
		}
	case durableRestartQuarantinesLegacyV1:
		if recoveryErr != nil || len(recovery.Complete) != 0 || len(recovery.LegacyQuarantined) != 1 ||
			recovery.LegacyQuarantined[0].AcceptedFinal.RecordDigest != fixture.acceptedDigest {
			t.Fatalf("legacy public winner was not quarantined without backfill: recovery=%#v err=%v", recovery, recoveryErr)
		}
	case durableRestartRejectsDetachedV1:
		if recoveryErr == nil {
			t.Fatalf("detached accepted-final authority was recovered: %#v", recovery)
		}
	default:
		t.Fatalf("unknown restart expectation %q", want)
	}
	projection := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if record, disposition, ok := projection.ResolveCommitted(fixture.threadID, fixture.turnID); ok {
		t.Fatalf("restart classification bypassed terminal-complete projection registration: record=%#v disposition=%#v", record, disposition)
	}
}

func durableAcceptedFinalEventIO(store *DurableEventSessionStore, casReader *finalauthority.AcceptedFinalCASReader) evidenceapp.FinalPublicationEventIO {
	delivery := store.AcceptedFinalEventDelivery()
	return evidenceapp.FinalPublicationEventIO{
		ReadThread: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return store.GetThread(privateRecord.SecurityContext.ThreadID)
		},
		ReadCASObservation: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalCASObservationV1, error) {
			return casReader.ReadAcceptedFinalCASObservation(ctx, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
		},
		LoadEvents: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
			result, err := store.LoadEventsSince(threadID, 0)
			if err != nil || len(result.Diagnostics) != 0 {
				return nil, errors.New("accepted-final crash fixture event log is invalid")
			}
			return result.Events, nil
		},
		AppendEvents: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, events []map[string]any) ([]map[string]any, error) {
			return delivery.Stage(ctx, events)
		},
		Readback: delivery,
		WithEventReservation: func(
			ctx context.Context,
			_ appturn.AcceptedFinalCompletionStore,
			threadID, commitID string,
			work evidenceapp.AcceptedFinalEventReservationWorkV1,
		) error {
			return delivery.WithReservation(ctx, threadID, commitID, work)
		},
		ActivateAndPublishEvents: func(
			ctx context.Context,
			_ appturn.AcceptedFinalCompletionStore,
			events []map[string]any,
			seal domainevent.AcceptedFinalDeliverySealV1,
			activate func() error,
		) error {
			return delivery.ActivateAndPublish(ctx, events, seal, activate)
		},
	}
}

func removeDurableDisposition(t *testing.T, fixture durableAcceptedFinalCrashFixture) {
	t.Helper()
	path := filepath.Join(fixture.privateRoot, "accepted-finals", "dispositions", fixture.acceptedDigest[:2], fixture.acceptedDigest+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// The simulated cut is before the CAS addition, so its shard was never
	// committed either. Leaving an unsigned empty shard would model corruption,
	// not a crash-recoverable write frontier, and must remain fail-closed.
	if err := os.Remove(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}

func writeDurableAcceptedFinalEvents(t *testing.T, fixture durableAcceptedFinalCrashFixture, events []map[string]any) {
	t.Helper()
	path := filepath.Join(fixture.durableRoot, "threads", fixture.threadID, "events.jsonl")
	if len(events) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return
	}
	var body bytes.Buffer
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		body.Write(encoded)
		body.WriteByte('\n')
	}
	if err := os.WriteFile(path, body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func cloneDurableEventMaps(events []map[string]any) []map[string]any {
	body, _ := json.Marshal(events)
	cloned := []map[string]any{}
	_ = json.Unmarshal(body, &cloned)
	return cloned
}
