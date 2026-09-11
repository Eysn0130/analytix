package evidence

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	appturnterminal "analytix.local/runtime-go/internal/app/turnterminal"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type acceptedFinalPublicReaderStub struct {
	threads map[string]map[string]any
}

type caseTerminalDynamicReader struct {
	securityContext domainsecurity.TurnSecurityContext
	store           *caseTerminalStoreStub
}

type visitOnlyPrivateFinalStore struct {
	*memoryPrivateFinalStore
	listCalls             int
	listDispositionCalls  int
	visitCalls            int
	visitDispositionCalls int
}

func (store *visitOnlyPrivateFinalStore) List(context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	store.listCalls++
	return nil, errors.New("eager private record listing is forbidden during preflight")
}

func (store *visitOnlyPrivateFinalStore) ListDispositions(context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error) {
	store.listDispositionCalls++
	return nil, errors.New("eager disposition listing is forbidden during preflight")
}

func (store *visitOnlyPrivateFinalStore) VisitAcceptedFinals(ctx context.Context, visit func(domainevidence.PrivateAcceptedFinalRecord) error) error {
	store.visitCalls++
	return store.memoryPrivateFinalStore.VisitAcceptedFinals(ctx, visit)
}

func (store *visitOnlyPrivateFinalStore) VisitDispositions(ctx context.Context, visit func(domainevidence.AcceptedFinalDispositionRecord) error) error {
	store.visitDispositionCalls++
	return store.memoryPrivateFinalStore.VisitDispositions(ctx, visit)
}

func (reader caseTerminalDynamicReader) AllThreadIDs() ([]string, error) {
	return []string{reader.securityContext.ThreadID}, nil
}

func (reader caseTerminalDynamicReader) GetThread(threadID string) (map[string]any, error) {
	if threadID != reader.securityContext.ThreadID || reader.store == nil {
		return nil, errors.New("test thread is missing")
	}
	return acceptedFinalReaderForStore(reader.securityContext, reader.store).threads[threadID], nil
}

func (reader caseTerminalDynamicReader) ReadAcceptedFinalCASObservation(ctx context.Context, threadID string, turnID string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	thread, err := reader.GetThread(threadID)
	if err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, err
	}
	return acceptedFinalCASObservationForTest(ctx, thread, threadID, turnID)
}

func (reader acceptedFinalPublicReaderStub) AllThreadIDs() ([]string, error) {
	ids := make([]string, 0, len(reader.threads))
	for id := range reader.threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (reader acceptedFinalPublicReaderStub) GetThread(threadID string) (map[string]any, error) {
	thread, ok := reader.threads[threadID]
	if !ok {
		return nil, errors.New("test thread is missing")
	}
	return thread, nil
}

func (reader acceptedFinalPublicReaderStub) ReadAcceptedFinalCASObservation(ctx context.Context, threadID string, turnID string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	thread, err := reader.GetThread(threadID)
	if err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, err
	}
	return acceptedFinalCASObservationForTest(ctx, thread, threadID, turnID)
}

type countingAcceptedFinalPublicReader struct {
	delegate       AcceptedFinalPublicReader
	allThreadCalls int
	getThreadCalls int
}

func (reader *countingAcceptedFinalPublicReader) AllThreadIDs() ([]string, error) {
	reader.allThreadCalls++
	return reader.delegate.AllThreadIDs()
}

func (reader *countingAcceptedFinalPublicReader) GetThread(threadID string) (map[string]any, error) {
	reader.getThreadCalls++
	return reader.delegate.GetThread(threadID)
}

func acceptedFinalCASObservationForTest(ctx context.Context, thread map[string]any, threadID string, turnID string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return domainevidence.AcceptedFinalCASObservationV1{}, err
		}
	}
	var matched map[string]any
	for _, turn := range authorityTurns(thread) {
		if authorityString(turn, "id") == turnID {
			matched = turn
			break
		}
	}
	if matched == nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("test primary CAS turn is missing")
	}
	frozen, err := domainsecurity.ParseTurnSecurityContext(matched["securityContext"])
	if err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, err
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, err
	}
	observation := domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: threadID, TurnID: turnID, Status: authorityString(matched, "status"), FrozenContext: frozen, CurrentContext: current,
		ThreadFileSHA256: canonicalAuthorityDigest(thread), TurnProjectionSHA256: canonicalAuthorityDigest(matched),
	}
	if matched["acceptedFinal"] != nil {
		winner, err := domainevidence.ParseAcceptedFinalRecord(matched["acceptedFinal"])
		if err != nil {
			return domainevidence.AcceptedFinalCASObservationV1{}, err
		}
		observation.HasWinner = true
		observation.Winner = winner
	}
	return domainevidence.NewAcceptedFinalCASObservationV1(observation)
}

func TestFinalAuthorityPreflightCaseCompactionMarkerIsNarrowAndFailClosed(t *testing.T) {
	fixture := newCaseCompactionPreflightFixtureV1(t)
	newReader := func(thread map[string]any) *caseCompactionPreflightReaderV1 {
		return &caseCompactionPreflightReaderV1{
			acceptedFinalPublicReaderStub: acceptedFinalPublicReaderStub{
				threads: map[string]map[string]any{fixture.threadID: thread},
			},
			authority: fixture.authority,
		}
	}
	run := func(t *testing.T, thread map[string]any) error {
		t.Helper()
		reader := newReader(thread)
		_, registry, authority, privateStore := newTestCasePublicationFinalizer()
		return PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore)
	}
	if err := run(t, fixture.thread); err != nil {
		t.Fatalf("valid case compaction authority marker failed preflight: %v", err)
	}

	cases := map[string]func(map[string]any){
		"ordinary completed case turn without accepted-final": func(thread map[string]any) {
			turn := caseCompactionPreflightTurnV1(thread, fixture.targetTurnID)
			delete(turn, "caseHistoryProjection")
			turn["items"] = []any{}
		},
		"forged compaction projection": func(thread map[string]any) {
			turn := caseCompactionPreflightTurnV1(thread, fixture.sourceTurnID)
			turn["caseHistoryProjection"] = "compaction_authority_v1"
			turn["items"] = []any{}
		},
		"binding tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			binding := item["caseCompactionBinding"].(map[string]any)
			binding["operationStamp"] = "1"
		},
		"source digest tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			item["sourceDigest"] = strings.Repeat("f", 64)
		},
		"security context tamper": func(thread map[string]any) {
			turn := caseCompactionPreflightTurnV1(thread, fixture.targetTurnID)
			context := turn["securityContext"].(map[string]any)
			context["turnId"] = "turn-forged"
		},
		"identity tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			item["id"] = "compaction-forged"
		},
		"signed target epoch tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			item["auto"] = true
			item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			thread := cloneCaseCompactionPreflightMapV1(t, fixture.thread)
			mutate(thread)
			if err := run(t, thread); err == nil {
				t.Fatalf("tampered case compaction marker passed final-authority preflight")
			}
		})
	}
	if err := ValidateAcceptedFinalEventReplay(fixture.thread, nil); err == nil {
		t.Fatal("case compaction marker passed public replay without committed authority")
	}
	reader := newReader(fixture.thread)
	replayAuthority := acceptedFinalReplayAuthorityV1{
		validateCaseCompactionTurn: reader.ValidateCaseCompactionAuthorityTurnV1,
	}
	if err := validateAcceptedFinalEventReplayWithAuthority(fixture.thread, nil, replayAuthority); err != nil {
		t.Fatalf("valid case compaction authority marker failed authority-backed event replay: %v", err)
	}
	eventReplayCases := map[string]func(map[string]any){
		"ordinary completed case turn without accepted-final": func(thread map[string]any) {
			turn := caseCompactionPreflightTurnV1(thread, fixture.targetTurnID)
			delete(turn, "caseHistoryProjection")
		},
		"forged compaction projection": func(thread map[string]any) {
			turn := caseCompactionPreflightTurnV1(thread, fixture.targetTurnID)
			turn["items"] = []any{}
		},
		"binding tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			binding := item["caseCompactionBinding"].(map[string]any)
			binding["operationStamp"] = "1"
		},
		"source digest tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			item["sourceDigest"] = strings.Repeat("f", 64)
		},
		"security context tamper": func(thread map[string]any) {
			turn := caseCompactionPreflightTurnV1(thread, fixture.targetTurnID)
			context := turn["securityContext"].(map[string]any)
			context["turnId"] = "turn-forged"
		},
		"identity tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			item["id"] = "compaction-forged"
		},
		"signed target epoch tamper": func(thread map[string]any) {
			item := caseCompactionPreflightItemV1(thread, fixture.targetTurnID)
			item["auto"] = true
			item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
		},
	}
	for name, mutate := range eventReplayCases {
		t.Run("event-replay/"+name, func(t *testing.T) {
			thread := cloneCaseCompactionPreflightMapV1(t, fixture.thread)
			mutate(thread)
			if err := validateAcceptedFinalEventReplayWithAuthority(thread, nil, replayAuthority); err == nil {
				t.Fatalf("tampered case compaction marker passed accepted-final event replay")
			}
		})
	}
}

type caseCompactionPreflightFixtureV1 struct {
	thread       map[string]any
	authority    caseCompactionPreflightAuthorityV1
	threadID     string
	sourceTurnID string
	targetTurnID string
}

type caseCompactionPreflightReaderV1 struct {
	acceptedFinalPublicReaderStub
	authority caseCompactionPreflightAuthorityV1
}

func (reader *caseCompactionPreflightReaderV1) ValidateCaseCompactionAuthorityTurnV1(
	threadID string,
	thread map[string]any,
	turn map[string]any,
) error {
	target, found := reader.authority.CommittedContext(threadID, authorityString(turn, "id"))
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !found || target.SecurityContext != frozen || !reader.authority.ContainsContext(frozen) {
		return errors.New("test case compaction authority is unavailable")
	}
	return appturn.ValidateCaseCompactionAuthorityTurnV1(threadID, thread, turn, target.EpochState)
}

type caseCompactionPreflightAuthorityV1 struct {
	threadID  string
	committed map[string]casethreadapp.CommittedContext
}

func (authority caseCompactionPreflightAuthorityV1) IsCaseThread(threadID string) bool {
	return strings.TrimSpace(threadID) == authority.threadID
}

func (authority caseCompactionPreflightAuthorityV1) ContainsContext(context domainsecurity.TurnSecurityContext) bool {
	committed, found := authority.committed[context.TurnID]
	return found && committed.SecurityContext == context
}

func (authority caseCompactionPreflightAuthorityV1) CommittedContext(
	threadID string,
	turnID string,
) (casethreadapp.CommittedContext, bool) {
	if strings.TrimSpace(threadID) != authority.threadID {
		return casethreadapp.CommittedContext{}, false
	}
	committed, found := authority.committed[strings.TrimSpace(turnID)]
	return committed, found
}

func newCaseCompactionPreflightFixtureV1(t *testing.T) caseCompactionPreflightFixtureV1 {
	t.Helper()
	threadID := "thread-final-authority-compaction"
	stamp := time.Date(2026, 8, 1, 1, 2, 3, 4, time.UTC).UnixNano()
	sourceTurnID := "turn-source-authority"
	sourceContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: sourceTurnID, WorkspaceRealPath: "/cases/final-authority-compaction",
		CaseID: "case-final-authority-compaction", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("final-authority-compaction"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1,
		IssuedAt: time.Unix(0, stamp-1_000_000).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	continuation, err := threaddomain.SealTaskContinuationSnapshotV1(threaddomain.TaskContinuationSnapshotV1{
		Todos: []threaddomain.TaskContinuationTodoV1{}, LatestUserConstraints: []string{},
		EvidenceReferences: []threaddomain.TaskContinuationEvidenceReferenceV1{},
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceTurn := map[string]any{
		"id": sourceTurnID, "threadId": threadID, "status": "running",
		"securityContext": turnsecurityapp.PublicRecord(sourceContext), "items": []any{},
	}
	binding, err := appturn.NewCaseCompactionOperationBindingV1(
		threadID, []any{sourceTurn}, []any{sourceTurn}, sourceContext, stamp, continuation, []string{sourceTurnID},
	)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := appturn.CaseCompactionOperationDigestV1(binding)
	if err != nil {
		t.Fatal(err)
	}
	targetTurnID := fmt.Sprintf("turn_%s_compaction_%s", contracts.SafeRecordID(threadID), binding.OperationStamp)
	targetIssuedAt := time.Unix(0, stamp).UTC()
	targetContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: targetTurnID, WorkspaceRealPath: sourceContext.WorkspaceRealPath,
		CaseID: sourceContext.CaseID, CaseBindingHash: sourceContext.CaseBindingHash,
		DatasetSnapshotID: sourceContext.DatasetSnapshotID, SourceManifestHash: sourceContext.SourceManifestHash,
		ContextEpoch: 2, IssuedAt: targetIssuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := contextepochapp.SecurityBindingEntry(targetContext)
	modeEntry := domaincontextepoch.SourceEntry{
		Version: domaincontextepoch.ContractVersion, SourceID: contextepochapp.CompactionModeSourceID,
		Kind: "compaction-operation", Reference: "manual",
		Digest: domaincontextepoch.SHA256Hex([]byte("analytix-compaction-operation-mode:manual")), Sequence: 2,
		TrustState: domaincontextepoch.TrustTrusted, PromptBoundary: domaincontextepoch.BoundaryStoreOnly,
		ActivationState: domaincontextepoch.ActivationInactive, ActivationReason: "manual",
	}
	registry := []domaincontextepoch.SourceEntry{entry, modeEntry}
	snapshot := domaincontextepoch.Snapshot{
		Version: domaincontextepoch.ContractVersion, ThreadID: threadID, Epoch: targetContext.ContextEpoch,
		BaselineSequence: 1, RegistryDigest: domaincontextepoch.RegistryDigest(registry),
		Sources: []domaincontextepoch.SourceSnapshot{
			domaincontextepoch.SourceSnapshotFromEntry(entry), domaincontextepoch.SourceSnapshotFromEntry(modeEntry),
		},
		RecoveryDigest: digest, AcceptedAt: targetContext.IssuedAt,
		ChangeReasons: []domaincontextepoch.ChangeReason{domaincontextepoch.ReasonCompactionRecovery},
		Impact:        domaincontextepoch.ChangeImpact{DiagnosticsOnly: true},
	}
	snapshot = domaincontextepoch.SealSnapshot(snapshot)
	state := domaincontextepoch.SealState(domaincontextepoch.State{
		Version: domaincontextepoch.ContractVersion, ThreadID: threadID, Registry: registry,
		AcceptedSnapshot: snapshot,
	})
	itemID := fmt.Sprintf("compaction_%s_%s", contracts.SafeRecordID(threadID), binding.OperationStamp)
	item := map[string]any{
		"id": itemID, "turnId": targetTurnID, "threadId": threadID, "role": "system", "status": "completed",
		"createdAt": targetContext.IssuedAt, "finishedAt": targetContext.IssuedAt, "kind": "compaction",
		"summary": appturn.CaseCompactionSummaryTextV1, "replacedTokens": float64(1), "auto": false,
		"pinnedConstraints": []any{"user: preserve recent turns"}, "sourceDigest": digest,
		"digestMarker": "sha256:" + digest[:12], "sourceItemIds": []any{}, "schemaVersion": float64(3),
		"reasoningExcluded": true, "caseFactsExcluded": true, "caseHistoryProjectionVersion": float64(2),
		"taskContinuation":      threaddomain.TaskContinuationSnapshotMapV1(continuation),
		"sourceContextDigest":   sourceContext.ContextDigest,
		"caseCompactionBinding": appturn.CaseCompactionOperationBindingMapV1(binding),
	}
	item["reasoningExclusionProof"] = domainevent.ReasoningExclusionProof(item)
	target := map[string]any{
		"id": targetTurnID, "threadId": threadID, "status": "completed", "prompt": "/compact", "model": "test",
		"createdAt": targetContext.IssuedAt, "startedAt": targetContext.IssuedAt, "finishedAt": targetContext.IssuedAt,
		"items": []any{item}, "caseHistoryProjection": "compaction_authority_v1",
		"securityContext":      turnsecurityapp.PublicRecord(targetContext),
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(snapshot),
	}
	thread := map[string]any{
		"id": threadID, "turns": []any{sourceTurn, target},
		"securityState":     turnsecurityapp.PublicRecord(targetContext),
		"contextEpochState": contextepochapp.PublicState(state),
	}
	sourceState, err := contextepochapp.BootstrapState(
		threadID,
		sourceContext.ContextEpoch,
		[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(sourceContext)},
		targetIssuedAt.Add(-time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := caseCompactionPreflightAuthorityV1{
		threadID: threadID,
		committed: map[string]casethreadapp.CommittedContext{
			sourceTurnID: {SecurityContext: sourceContext, EpochState: sourceState},
			targetTurnID: {SecurityContext: targetContext, EpochState: state},
		},
	}
	return caseCompactionPreflightFixtureV1{
		thread: thread, authority: authority, threadID: threadID,
		sourceTurnID: sourceTurnID, targetTurnID: targetTurnID,
	}
}

func caseCompactionPreflightTurnV1(thread map[string]any, turnID string) map[string]any {
	for _, raw := range thread["turns"].([]any) {
		turn := raw.(map[string]any)
		if turn["id"] == turnID {
			return turn
		}
	}
	return nil
}

func caseCompactionPreflightItemV1(thread map[string]any, turnID string) map[string]any {
	turn := caseCompactionPreflightTurnV1(thread, turnID)
	return turn["items"].([]any)[0].(map[string]any)
}

func cloneCaseCompactionPreflightMapV1(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestFinalAuthorityPreflightBindsPublicPrivateTrustedKeyAndHistoricalRegistryHead(t *testing.T) {
	issuer, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil || !result.Persistence.Changed {
		t.Fatalf("accepted final setup failed: result=%#v err=%v", result, err)
	}
	reader := acceptedFinalReaderForStore(input.Context, store)
	if hasAuthority, err := HasPublicAcceptedFinalAuthority(reader); err != nil || !hasAuthority {
		t.Fatalf("public authority state was not detected: has=%t err=%v", hasAuthority, err)
	}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err != nil {
		t.Fatalf("valid accepted final failed startup preflight: %v", err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 || records[0].Envelope.EnvelopeDigest != result.Boundary.Envelope.EnvelopeDigest ||
		records[0].RenderedText != result.Boundary.Text || records[0].SecurityContext.ContextDigest != input.Context.ContextDigest {
		t.Fatalf("complete private final was not persisted: records=%#v err=%v", records, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		dispositions[0].DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		dispositions[0].WinnerDigest != result.Persistence.AcceptedFinal.RecordDigest || dispositions[0].TurnCASDigest == "" {
		t.Fatalf("normal publication disposition is not bound to its exact CAS winner: dispositions=%#v err=%v", dispositions, err)
	}

	issuer.Registry = registry
	before, err := registry.Replay(context.Background(), input.Context)
	if err != nil {
		t.Fatal(err)
	}
	if prepared, err := issuer.Prepare(context.Background(), input); err == nil || prepared.Record.ReceiptID != "" {
		t.Fatalf("legacy snapshot appended after accepted registry head: prepared=%#v err=%v", prepared, err)
	}
	after, err := registry.Replay(context.Background(), input.Context)
	if err != nil || after.Sequence != before.Sequence || after.StateDigest != before.StateDigest {
		t.Fatalf("rejected legacy append changed registry head: before=%#v after=%#v err=%v", before, after, err)
	}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err != nil {
		t.Fatalf("rejected legacy registry append invalidated historical accepted final: %v", err)
	}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, newMemoryFinalAuthority(2), privateStore); err == nil {
		t.Fatal("untrusted installation key accepted a public/private final")
	}
	emptyPrivate := &memoryPrivateFinalStore{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, emptyPrivate); err == nil {
		t.Fatal("public accepted final without private record passed preflight")
	}
}

func TestUnavailableRegistryPersistsFixedCaseBoundaryWithoutRegistryOrWitnessUse(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	baseRegistry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	blocked := &unavailableLockedRegistryV1{lockedMemoryEvidenceRegistry: baseRegistry}
	privateStore := &memoryPrivateFinalStore{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{}}
	authority := newMemoryFinalAuthority(1)
	finalizer := NewCasePublicationFinalizerWithHostEvidenceAuthority(
		blocked, blocked, authority, privateStore, newTestFinalPublicationEventIO(),
		newTestTurnTerminalCoordinator(authority, privateStore), nil, nil,
	)
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil || result.Boundary.Envelope.Blocker != caseEvidenceAuthorityUnavailableBlockerV1 ||
		result.Boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer || !result.Persistence.Changed ||
		blocked.lockedCalls != 0 || blocked.commitCalls != 0 {
		t.Fatalf("blocked registry did not produce the fixed typed boundary without authority use: result=%#v locked=%d commits=%d err=%v", result, blocked.lockedCalls, blocked.commitCalls, err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 || records[0].RegistryHead.Sequence != 0 ||
		records[0].AcceptedFinal.FactFinalWitnessAdmission != nil || records[0].PublicationSnapshotProof != nil {
		t.Fatalf("blocked case boundary acquired evidence or witness authority: records=%#v err=%v", records, err)
	}
	if err := domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(records[0]); err != nil {
		t.Fatalf("blocked case boundary lost audit authority: %v", err)
	}
}

type unavailableLockedRegistryV1 struct {
	*lockedMemoryEvidenceRegistry
	lockedCalls int
	commitCalls int
}

func (*unavailableLockedRegistryV1) CaseEvidenceAuthorityUnavailableV1() bool { return true }

func (registry *unavailableLockedRegistryV1) WithLockedSnapshot(context.Context, domainsecurity.TurnSecurityContext, func(domainevidence.EvidenceReceiptRegistry) error) error {
	registry.lockedCalls++
	return errors.New("blocked locked snapshot reached")
}

func (registry *unavailableLockedRegistryV1) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	registry.commitCalls++
	return domainevidence.EvidenceReceipt{}, errors.New("blocked registry commit reached")
}

func TestFinalAuthorityPreflightUsesEachStreamingInventoryExactlyOnce(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	reader := acceptedFinalReaderForStore(input.Context, store)
	streaming := &visitOnlyPrivateFinalStore{memoryPrivateFinalStore: privateStore}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, streaming); err != nil {
		t.Fatalf("streaming-only preflight failed: %v", err)
	}
	if streaming.visitCalls != 1 || streaming.visitDispositionCalls != 1 ||
		streaming.listCalls != 0 || streaming.listDispositionCalls != 0 {
		t.Fatalf("preflight inventory calls = visit:%d dispositions:%d list:%d listDispositions:%d",
			streaming.visitCalls, streaming.visitDispositionCalls, streaming.listCalls, streaming.listDispositionCalls)
	}
}

func TestFinalAuthorityPreflightRejectsPublicTamper(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	reader := acceptedFinalReaderForStore(input.Context, store)
	turn := authorityTurns(reader.threads[input.Context.ThreadID])[0]
	accepted := turn["acceptedFinal"].(map[string]any)
	accepted["acceptedAt"] = "2026-07-11T00:00:00Z"
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
		t.Fatal("tampered public accepted final passed startup preflight")
	}
}

func TestFinalAuthorityPreflightRejectsTextTamperSiblingAndAuthorityStripping(t *testing.T) {
	for _, mutate := range []struct {
		name  string
		apply func(map[string]any)
	}{
		{name: "text tamper", apply: func(turn map[string]any) {
			items := turn["items"].([]any)
			items[0].(map[string]any)["text"] = "伪造金额 420 万元"
		}},
		{name: "raw sibling", apply: func(turn map[string]any) {
			items := turn["items"].([]any)
			turn["items"] = append(items, map[string]any{
				"kind": "assistant_text", "role": "assistant", "threadId": "thread-a", "turnId": "turn-a", "text": "伪造案件事实",
			})
		}},
		{name: "authority stripping", apply: func(turn map[string]any) {
			delete(turn, "acceptedFinal")
			items := turn["items"].([]any)
			delete(items[0].(map[string]any), "acceptedFinal")
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			_, input := evidenceIssuerFixture(t)
			finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
			store := &caseTerminalStoreStub{}
			if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
				Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
				ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
			}); err != nil {
				t.Fatal(err)
			}
			reader := acceptedFinalReaderForStore(input.Context, store)
			mutate.apply(authorityTurns(reader.threads[input.Context.ThreadID])[0])
			if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
				t.Fatalf("%s passed startup preflight", mutate.name)
			}
		})
	}
}

func TestFinalAuthorityPreflightRejectsTerminalPublicationItemTamper(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalProviderFailure,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	if len(store.items) != 2 {
		t.Fatalf("expected terminal error item fixture, got %#v", store.items)
	}
	store.items[1]["message"] = "账户 6222020000000000 金额 4200000 元"
	reader := acceptedFinalReaderForStore(input.Context, store)
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
		t.Fatal("tampered terminal publication item passed preflight")
	}
}

func TestFinalAuthorityPreflightRejectsFractionalLegacyVersion(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		input.Context.ThreadID: {
			"id": input.Context.ThreadID,
			"turns": []any{map[string]any{
				"id": input.Context.TurnID, "status": "completed", "acceptedFinal": map[string]any{"schemaVersion": 1.5}, "items": []any{},
			}},
		},
	}}
	_, registry, authority, privateStore := newTestCasePublicationFinalizer()
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
		t.Fatal("fractional accepted-final version was downgraded to legacy v1")
	}
	reader.threads[input.Context.ThreadID]["turns"] = []any{map[string]any{
		"id": input.Context.TurnID, "status": "completed", "acceptedFinal": "forged", "items": []any{},
	}}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
		t.Fatal("non-object accepted-final value was treated as absent")
	}
	reader.threads[input.Context.ThreadID]["turns"] = []any{map[string]any{
		"id": input.Context.TurnID, "status": "completed", "items": []any{map[string]any{
			"id": "item-forged", "kind": "assistant_text", "acceptedFinal": map[string]any{"schemaVersion": float64(1)},
		}},
	}}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
		t.Fatal("item-only legacy accepted final bypassed turn authority")
	}
}

func TestFinalAuthorityPreflightRejectsLegacyV1WithoutSignedCutoverInventory(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: input.Context, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	legacy := domainevidence.LegacyAcceptedFinalRecord{
		SchemaVersion: 1, EnvelopeDigest: envelope.EnvelopeDigest, ContextDigest: input.Context.ContextDigest,
		ContextEpoch: input.Context.ContextEpoch, DatasetSnapshotID: input.Context.DatasetSnapshotID,
		Variant: envelope.Variant, TerminalReason: envelope.TerminalReason,
		RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(rendered)), AcceptedAt: evidenceIssuerTime().Format(time.RFC3339Nano),
	}
	body, _ := json.Marshal(legacy)
	legacy.RecordDigest = domainsecurity.SHA256Hex(body)
	legacyMap := domainevidence.LegacyAcceptedFinalRecordMap(legacy)
	item := map[string]any{
		"id": "legacy-final", "kind": "assistant_text", "role": "assistant", "threadId": input.Context.ThreadID,
		"turnId": input.Context.TurnID, "text": rendered, "acceptedFinal": legacyMap,
	}
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		input.Context.ThreadID: {"id": input.Context.ThreadID, "turns": []any{map[string]any{
			"id": input.Context.TurnID, "status": "completed", "securityContext": turnSecurityContextRecord(input.Context),
			"acceptedFinal": legacyMap, "items": []any{item},
		}}},
	}}
	_, registry, authority, privateStore := newTestCasePublicationFinalizer()
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err == nil {
		t.Fatal("self-hashed legacy v1 history passed without a trusted signed cutover inventory")
	}
	thread := reader.threads[input.Context.ThreadID]
	validEvent := map[string]any{"kind": "item_completed", "threadId": input.Context.ThreadID, "turnId": input.Context.TurnID, "item": item}
	if err := ValidateAcceptedFinalEventReplay(thread, []map[string]any{validEvent}); err == nil {
		t.Fatal("legacy v1 event replay passed without a trusted signed cutover inventory")
	}
	forgedEvent := map[string]any{
		"kind": "item_completed", "threadId": input.Context.ThreadID, "turnId": input.Context.TurnID,
		"item": map[string]any{"kind": "assistant_text", "text": "账户 6222020000000000 金额 4200000 元"},
	}
	if err := ValidateAcceptedFinalEventReplay(thread, []map[string]any{forgedEvent}); err == nil {
		t.Fatal("forged legacy assistant event passed startup replay validation")
	}
}

func TestFinalAuthorityPreflightPreservesTrustedV2BoundaryOnly(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil || result.Boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer {
		t.Fatalf("v2 migration fixture setup failed: result=%#v err=%v", result, err)
	}
	currentPrivate, err := privateStore.Resolve(context.Background(), result.Persistence.AcceptedFinal.RecordDigest)
	if err != nil {
		t.Fatal(err)
	}
	previousPrivateDigest := previousAcceptedFinalPrivateDigest(t, currentPrivate)
	previousPublic := currentPrivate.AcceptedFinal
	previousPublic.SchemaVersion = domainevidence.PreviousAcceptedFinalRecordVersion
	previousPublic.FinalGateVersion = domainevidence.LegacyFinalEvidenceGateVersion
	previousPublic.PublicationSnapshotProofDigest = ""
	previousPublic.PublicView = nil
	previousPublic.PublicViewDigest = ""
	previousPublic.PrivateRecordDigest = previousPrivateDigest
	previousPublic.AuthoritySignature = ""
	previousPublic.RecordDigest = ""
	signature, err := authority.Sign(context.Background(), domainevidence.AcceptedFinalSigningBytes(previousPublic))
	if err != nil {
		t.Fatal(err)
	}
	previousPublic.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	previousPublic.RecordDigest = acceptedFinalPublicDigest(t, previousPublic)
	previousPrivate := currentPrivate
	previousPrivate.SchemaVersion = domainevidence.PreviousAcceptedFinalRecordVersion
	previousPrivate.PublicationSnapshotProof = nil
	previousPrivate.AcceptedFinal = previousPublic
	previousPrivate.PrivateRecordDigest = previousPrivateDigest
	previousPrivate.StoreDigest = ""
	previousPrivate.StoreDigest = acceptedFinalPrivateStoreDigest(t, previousPrivate)
	if err := domainevidence.ValidatePrivateAcceptedFinalRecord(previousPrivate); err != nil {
		t.Fatalf("trusted v2 boundary fixture is invalid: %v", err)
	}
	plan, err := appturn.BuildAcceptedFinalAuditPublicationPlan(previousPublic, previousPrivate.RenderedText, previousPrivate.PublicationIntent)
	if err != nil {
		t.Fatal(err)
	}
	store.status = previousPrivate.PublicationIntent.TerminalStatus
	store.items = plan.TurnItems
	store.fields = plan.TurnFields
	privateStore.records = map[string]domainevidence.PrivateAcceptedFinalRecord{previousPublic.RecordDigest: previousPrivate}
	decidedAt, _ := time.Parse(time.RFC3339Nano, previousPublic.AcceptedAt)
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: previousPublic, State: domainevidence.AcceptedFinalCommitted, EventManifestDigest: plan.EventManifestDigest,
		DecidedAt: decidedAt, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	privateStore.dispositions = map[string]domainevidence.AcceptedFinalDispositionRecord{previousPublic.RecordDigest: disposition}
	reader := acceptedFinalReaderForStore(input.Context, store)
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err != nil {
		t.Fatalf("trusted v2 boundary failed restart preflight: %v", err)
	}
	previousFact := previousPublic
	previousFact.Variant = domainevidence.EvidenceBackedAnswer
	previousFact.AuthoritySignature = ""
	previousFact.RecordDigest = ""
	signature, _ = authority.Sign(context.Background(), domainevidence.AcceptedFinalSigningBytes(previousFact))
	previousFact.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	previousFact.RecordDigest = acceptedFinalPublicDigest(t, previousFact)
	if err := domainevidence.ValidateAcceptedFinalRecord(previousFact); err == nil {
		t.Fatal("trusted-key v2 fact remained accepted without a publication snapshot proof")
	}
}

func previousAcceptedFinalPrivateDigest(t *testing.T, record domainevidence.PrivateAcceptedFinalRecord) string {
	t.Helper()
	body, err := json.Marshal(struct {
		SchemaVersion   int                                      `json:"schemaVersion"`
		SecurityContext domainsecurity.TurnSecurityContext       `json:"securityContext"`
		Envelope        domainevidence.FinalAnswerEnvelope       `json:"envelope"`
		RenderedText    string                                   `json:"renderedText"`
		Intent          domainevidence.TerminalPublicationIntent `json:"publicationIntent"`
	}{
		SchemaVersion: domainevidence.PreviousAcceptedFinalRecordVersion, SecurityContext: record.SecurityContext,
		Envelope: record.Envelope, RenderedText: record.RenderedText, Intent: record.PublicationIntent,
	})
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.SHA256Hex(body)
}

func acceptedFinalPublicDigest(t *testing.T, record domainevidence.AcceptedFinalRecord) string {
	t.Helper()
	record.RecordDigest = ""
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.SHA256Hex(body)
}

func acceptedFinalPrivateStoreDigest(t *testing.T, record domainevidence.PrivateAcceptedFinalRecord) string {
	t.Helper()
	record.StoreDigest = ""
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.SHA256Hex(body)
}

func TestPrivateFinalCommitPrecedesPublicCASAndMissingPrimaryCASFailsPreflight(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{unchanged: true}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err == nil || result.Persistence.Changed || len(store.events) != 0 {
		t.Fatalf("unclassified public CAS did not fail closed: result=%#v events=%#v err=%v", result, store.events, err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("private commit was not retained as a safe orphan: records=%#v err=%v", records, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 0 {
		t.Fatalf("CAS without an exact winner received a guessed disposition: dispositions=%#v err=%v", dispositions, err)
	}
	emptyReader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
	if err := PreflightFinalAuthority(context.Background(), emptyReader, emptyReader, registry, authority, privateStore); err == nil {
		t.Fatal("signed uncommitted disposition without an exact surviving primary CAS was accepted")
	}

	failingFinalizer, _, _, failingPrivate := newTestCasePublicationFinalizer()
	failingPrivate.putErr = errors.New("private write failed")
	failingStore := &caseTerminalStoreStub{}
	if _, err := failingFinalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: failingStore, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err == nil || len(failingStore.items) != 0 || len(failingStore.events) != 0 || failingStore.fields != nil {
		t.Fatalf("private write failure reached public CAS: items=%#v fields=%#v events=%#v err=%v", failingStore.items, failingStore.fields, failingStore.events, err)
	}
}

func TestFinalAuthorityPreflightDoesNotSignDetachedDispositionAndRejectsDeletedCommittedPublication(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	reader := acceptedFinalReaderForStore(input.Context, store)
	privateStore.mu.Lock()
	privateStore.dispositions = map[string]domainevidence.AcceptedFinalDispositionRecord{}
	privateStore.mu.Unlock()
	inventory, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, registry, authority, privateStore)
	if err != nil || len(inventory.Committed) != 1 || len(inventory.CommittedDispositions) != 0 {
		t.Fatalf("preflight did not classify the unsigned committed final without minting a disposition: inventory=%#v err=%v", inventory, err)
	}
	if _, err := recoverPreflightCandidatesV1(context.Background(), finalizer, store, reader, privateStore, inventory.Committed); err == nil {
		t.Fatal("detached terminal disposition was repaired by backfilling a missing accepted-final disposition")
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 0 {
		t.Fatalf("preflight or rejected recovery wrote a disposition: dispositions=%#v err=%v", dispositions, err)
	}

	finalizer, registry, authority, privateStore = newTestCasePublicationFinalizer()
	store = &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	emptyReader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
	if err := PreflightFinalAuthority(context.Background(), emptyReader, emptyReader, registry, authority, privateStore); err == nil {
		t.Fatal("deleted committed public thread was misclassified as a private orphan")
	}
}

func TestDifferentCASContenderIsRejectedBeforeTerminalMutation(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	winner, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime().Add(time.Second),
	})
	if err == nil || second.Persistence.Changed || second.Persistence.AcceptedFinal.RecordDigest != "" {
		t.Fatalf("different terminal contender was not rejected: winner=%#v second=%#v err=%v", winner.Persistence, second.Persistence, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].AcceptedFinalDigest != winner.Persistence.AcceptedFinal.RecordDigest ||
		dispositions[0].State != domainevidence.AcceptedFinalCommitted {
		t.Fatalf("rejected contender changed terminal disposition authority: dispositions=%#v err=%v", dispositions, err)
	}
	reader := acceptedFinalReaderForStore(input.Context, store)
	observation, err := reader.ReadAcceptedFinalCASObservation(context.Background(), input.Context.ThreadID, input.Context.TurnID)
	if err != nil || !observation.HasWinner || observation.Winner.RecordDigest != winner.Persistence.AcceptedFinal.RecordDigest {
		t.Fatalf("rejected contender changed the public winner: observation=%#v err=%v", observation, err)
	}
}

func TestUnresolvedPrivatePreparedIntentRepairsPublicCommit(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{unchanged: true}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err == nil {
		t.Fatal("public CAS failpoint did not leave an I+C recovery prefix")
	}
	originals, err := privateStore.List(context.Background())
	if err != nil || len(originals) != 1 {
		t.Fatal("failed first CAS did not retain one private preparation")
	}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalProviderFailure,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime().Add(time.Second),
	}); err == nil {
		t.Fatal("fallback replaced an unresolved private preparation")
	}
	afterFallback, err := privateStore.List(context.Background())
	if err != nil || len(afterFallback) != 1 || afterFallback[0].StoreDigest != originals[0].StoreDigest {
		t.Fatal("fallback added a conflicting private preparation")
	}
	store.unchanged = false
	store.status = "running"
	reader := caseTerminalDynamicReader{securityContext: input.Context, store: store}
	inventory, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, registry, authority, privateStore)
	if err != nil || len(inventory.PublicCommitRepairs) != 1 || len(inventory.Committed) != 0 {
		t.Fatalf("prepared private intent did not produce one public CAS repair: inventory=%#v err=%v", inventory, err)
	}
	if _, err := recoverPreflightCandidatesV1(context.Background(), finalizer, store, reader, privateStore, publicRepairCandidatesV1(inventory)); err != nil {
		t.Fatalf("prepared public CAS repair failed: %v", err)
	}
	verified, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, registry, authority, privateStore)
	if err != nil || len(verified.Committed) != 1 || len(verified.CommittedDispositions) != 1 || len(verified.PublicCommitRepairs) != 0 {
		t.Fatalf("repaired public commit did not become committed authority: inventory=%#v err=%v", verified, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].State != domainevidence.AcceptedFinalCommitted ||
		dispositions[0].SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		dispositions[0].TurnCASDigest == "" || dispositions[0].WinnerDigest != dispositions[0].AcceptedFinalDigest {
		t.Fatalf("repaired public commit lacks committed disposition: dispositions=%#v err=%v", dispositions, err)
	}
}

func TestUnresolvedPrivateRejectsStaleFrozenAndTerminalWinnerlessCAS(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	privateRecord, err := privateStore.Resolve(context.Background(), result.Persistence.AcceptedFinal.RecordDigest)
	if err != nil {
		t.Fatal(err)
	}
	store.status, store.items, store.fields = "running", nil, nil
	reader := caseTerminalDynamicReader{securityContext: input.Context, store: store}
	baseline, err := reader.ReadAcceptedFinalCASObservation(context.Background(), input.Context.ThreadID, input.Context.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if state, repairable, err := classifyPreparedAcceptedFinalCAS(baseline, privateRecord); err != nil || state != "" || !repairable {
		t.Fatalf("valid prepared CAS was not repairable: state=%q repairable=%v err=%v", state, repairable, err)
	}
	stale, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, WorkspaceRealPath: input.Context.WorkspaceRealPath,
		TenantID: input.Context.TenantID, UserID: input.Context.UserID, CaseID: "case-stale",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("stale-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("stale"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("stale-manifest")), ContextEpoch: input.Context.ContextEpoch + 1,
		IssuedAt: evidenceIssuerTime().Add(time.Minute), PublicationPolicy: input.Context.PublicationPolicy,
		RiskAuthorityBinding: input.Context.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	staleCurrent := baseline
	staleCurrent.CurrentContext = stale
	staleCurrent, err = domainevidence.NewAcceptedFinalCASObservationV1(staleCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := classifyPreparedAcceptedFinalCAS(staleCurrent, privateRecord); err == nil {
		t.Fatal("prepared private final accepted a stale current thread context")
	}
	staleFrozen := baseline
	staleFrozen.FrozenContext, staleFrozen.CurrentContext = stale, stale
	staleFrozen, err = domainevidence.NewAcceptedFinalCASObservationV1(staleFrozen)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := classifyPreparedAcceptedFinalCAS(staleFrozen, privateRecord); err == nil {
		t.Fatal("prepared private final accepted a different frozen turn context")
	}
	staleLosing := staleFrozen
	staleLosing.Status = "completed"
	staleLosing.HasWinner = true
	staleFinalizer, _, _, _ := newTestCasePublicationFinalizer()
	staleResult, err := staleFinalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: &caseTerminalStoreStub{}, Context: stale, TerminalReason: TerminalSuccess,
		ThreadID: stale.ThreadID, TurnID: stale.TurnID, AcceptedAt: evidenceIssuerTime().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	staleLosing.Winner = staleResult.Persistence.AcceptedFinal
	staleLosing, err = domainevidence.NewAcceptedFinalCASObservationV1(staleLosing)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := classifyPreparedAcceptedFinalCAS(staleLosing, privateRecord); err == nil {
		t.Fatal("private final from a different frozen context was classified through the winner/loser shortcut")
	}
	terminal := baseline
	terminal.Status = "completed"
	terminal, err = domainevidence.NewAcceptedFinalCASObservationV1(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := classifyPreparedAcceptedFinalCAS(terminal, privateRecord); err == nil {
		t.Fatal("terminal turn without a CAS winner was treated as repairable")
	}
}

func TestPublicCommitRepairRejectsNewContenderAndRestoresOriginalIntent(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{unchanged: true}
	_, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err == nil {
		t.Fatal("public CAS failpoint did not leave the original terminal prefix")
	}
	store.unchanged = false
	store.status = "running"
	reader := caseTerminalDynamicReader{securityContext: input.Context, store: store}
	inventory, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, registry, authority, privateStore)
	if err != nil || len(inventory.PublicCommitRepairs) != 1 {
		t.Fatalf("prepared repair fixture mismatch: inventory=%#v err=%v", inventory, err)
	}
	original := inventory.PublicCommitRepairs[0].PrivateRecord.AcceptedFinal.RecordDigest
	second, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime().Add(time.Second),
	})
	if err == nil || second.Persistence.Changed || second.Persistence.AcceptedFinal.RecordDigest != "" {
		t.Fatalf("new contender bypassed the existing terminal intent: second=%#v err=%v", second.Persistence, err)
	}
	privateRecords, err := privateStore.List(context.Background())
	if err != nil || len(privateRecords) != 1 || privateRecords[0].AcceptedFinal.RecordDigest != original {
		t.Fatal("rejected contender created another private preparation")
	}
	if _, err := recoverPreflightCandidatesV1(context.Background(), finalizer, store, reader, privateStore, publicRepairCandidatesV1(inventory)); err != nil {
		t.Fatalf("original terminal intent did not resume: %v", err)
	}
	current, err := reader.ReadAcceptedFinalCASObservation(context.Background(), input.Context.ThreadID, input.Context.TurnID)
	if err != nil || !current.HasWinner || current.Winner.RecordDigest != original {
		t.Fatalf("original terminal intent did not regain its exact winner: observation=%#v err=%v", current, err)
	}
	classified, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, registry, authority, privateStore)
	if err != nil || len(classified.Committed) != 1 || len(classified.CommittedDispositions) != 1 ||
		len(classified.NotCommitted) != 0 || len(classified.PublicCommitRepairs) != 0 {
		t.Fatalf("resumed terminal intent did not preserve the sole original preparation: inventory=%#v err=%v", classified, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].AcceptedFinalDigest != original {
		t.Fatalf("rejected contender acquired a durable disposition: dispositions=%#v err=%v", dispositions, err)
	}
}

func TestCommittedFinalRepairsEveryPartialPublicationBundleCut(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalProviderFailure,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	privateRecord, err := privateStore.Resolve(context.Background(), result.Persistence.AcceptedFinal.RecordDigest)
	if err != nil {
		t.Fatal(err)
	}
	thread := acceptedFinalReaderForStore(input.Context, store).threads[input.Context.ThreadID]
	for cut := 0; cut <= len(result.Persistence.Publication.Events); cut++ {
		t.Run(fmt.Sprintf("cut-%d", cut), func(t *testing.T) {
			persisted := []map[string]any{}
			for index := 0; index < cut; index++ {
				event := cloneEventMaps([]map[string]any{result.Persistence.Publication.Events[index].Draft})[0]
				event["seq"] = float64(index + 1)
				persisted = append(persisted, event)
			}
			io := finalPublicationEventIOStub(thread, &persisted)
			if err := ReconcileAcceptedFinalEvents(context.Background(), io, store, privateRecord); err != nil {
				t.Fatalf("partial bundle was not repaired: %v", err)
			}
			if len(persisted) != len(result.Persistence.Publication.Events) {
				t.Fatalf("repair count mismatch: got=%d want=%d", len(persisted), len(result.Persistence.Publication.Events))
			}
			seen := map[string]bool{}
			for _, event := range persisted {
				id := authorityString(event, "publicationEventId")
				if id == "" || seen[id] {
					t.Fatalf("repair produced a missing or duplicate event id: %#v", persisted)
				}
				seen[id] = true
			}
		})
	}

	conflicting := cloneEventMaps([]map[string]any{result.Persistence.Publication.Events[0].Draft})
	conflicting[0]["seq"] = float64(1)
	conflicting[0]["publicationSlot"] = "tampered-slot"
	io := finalPublicationEventIOStub(thread, &conflicting)
	if err := ReconcileAcceptedFinalEvents(context.Background(), io, store, privateRecord); err == nil {
		t.Fatal("conflicting stable publication event id was repaired over instead of failing closed")
	}
	duplicateSeq := []map[string]any{}
	for _, event := range result.Persistence.Publication.Events {
		cloned := cloneEventMaps([]map[string]any{event.Draft})[0]
		cloned["seq"] = float64(1)
		duplicateSeq = append(duplicateSeq, cloned)
	}
	io = finalPublicationEventIOStub(thread, &duplicateSeq)
	if err := ReconcileAcceptedFinalEvents(context.Background(), io, store, privateRecord); err == nil {
		t.Fatal("duplicate publication event sequence passed reconciliation")
	}
}

func TestAcceptedFinalInventoryPreflightRejectsBeforeFirstWrite(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("private authority fixture mismatch: records=%d err=%v", len(records), err)
	}
	thread := acceptedFinalReaderForStore(input.Context, store).threads[input.Context.ThreadID]
	inventory := FinalAuthorityInventory{Committed: records}

	tests := []struct {
		name   string
		events []map[string]any
	}{
		{
			name: "unknown publication digest",
			events: []map[string]any{{
				"kind": "usage", "threadId": input.Context.ThreadID, "turnId": input.Context.TurnID, "seq": float64(1),
				"publicationCommitId": strings.Repeat("a", 64), "acceptedFinalDigest": strings.Repeat("a", 64),
				"publicationEventId": strings.Repeat("b", 64), "publicationSlot": "usage", "publicationPayloadDigest": strings.Repeat("c", 64),
			}},
		},
		{
			name: "unmanifested accepted final item",
			events: func() []map[string]any {
				event := cloneEventMaps(store.events[:1])[0]
				for _, field := range []string{"publicationCommitId", "acceptedFinalDigest", "publicationEventId", "publicationSlot", "publicationPayloadDigest"} {
					delete(event, field)
				}
				event["seq"] = float64(1)
				return []map[string]any{event}
			}(),
		},
		{
			name: "non-prefix manifest slot",
			events: func() []map[string]any {
				event := cloneEventMaps(store.events[1:2])[0]
				event["seq"] = float64(1)
				return []map[string]any{event}
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := cloneEventMaps(test.events)
			appendCalls := 0
			io := finalPublicationInventoryEventIO(thread, &events, &appendCalls)
			reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{input.Context.ThreadID: thread}}
			if err := ReconcileAcceptedFinalEventInventory(context.Background(), io, store, reader, inventory); err == nil {
				t.Fatal("corrupt inventory passed startup reconciliation")
			}
			if appendCalls != 0 || len(events) != len(test.events) {
				t.Fatalf("preflight failure wrote events: calls=%d events=%#v", appendCalls, events)
			}
		})
	}
}

func TestAcceptedFinalInventoryRepairIsIdempotent(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("private authority fixture mismatch: records=%d err=%v", len(records), err)
	}
	thread := acceptedFinalReaderForStore(input.Context, store).threads[input.Context.ThreadID]
	events := cloneEventMaps(store.events[:1])
	events[0]["seq"] = float64(1)
	appendCalls := 0
	io := finalPublicationInventoryEventIO(thread, &events, &appendCalls)
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{input.Context.ThreadID: thread}}
	inventory := FinalAuthorityInventory{Committed: records}
	if err := ReconcileAcceptedFinalEventInventory(context.Background(), io, store, reader, inventory); err != nil {
		t.Fatalf("repair partial inventory: %v", err)
	}
	firstAppendCount := appendCalls
	if firstAppendCount != 1 {
		t.Fatalf("repair must append the missing suffix as one atomic bundle: calls=%d", firstAppendCount)
	}
	if err := ReconcileAcceptedFinalEventInventory(context.Background(), io, store, reader, inventory); err != nil {
		t.Fatalf("idempotent inventory readback: %v", err)
	}
	if appendCalls != firstAppendCount {
		t.Fatalf("idempotent repair appended duplicates: before=%d after=%d", firstAppendCount, appendCalls)
	}
}

func TestAcceptedFinalInventoryPreflightReadsEachThreadOncePerAttempt(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("private authority fixture mismatch: records=%d err=%v", len(records), err)
	}
	thread := acceptedFinalReaderForStore(input.Context, store).threads[input.Context.ThreadID]
	events := cloneEventMaps(store.events)
	appendCalls := 0
	readThreadCalls := 0
	loadEventsCalls := 0
	io := finalPublicationInventoryEventIO(thread, &events, &appendCalls)
	loadEvents := io.LoadEvents
	io.ReadThread = func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
		readThreadCalls++
		return thread, nil
	}
	io.LoadEvents = func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
		loadEventsCalls++
		return loadEvents(ctx, store, threadID)
	}
	reader := &countingAcceptedFinalPublicReader{delegate: acceptedFinalPublicReaderStub{
		threads: map[string]map[string]any{input.Context.ThreadID: thread},
	}}
	for attempt := 1; attempt <= 2; attempt++ {
		plans, err := PreflightAcceptedFinalEventReconciliations(
			context.Background(), io, store, reader, records,
		)
		if err != nil || len(plans) != 1 {
			t.Fatalf("attempt %d preflight mismatch: plans=%d err=%v", attempt, len(plans), err)
		}
		if reader.allThreadCalls != attempt || reader.getThreadCalls != attempt || loadEventsCalls != attempt {
			t.Fatalf(
				"attempt %d did not capture each thread exactly once: all=%d get=%d events=%d",
				attempt, reader.allThreadCalls, reader.getThreadCalls, loadEventsCalls,
			)
		}
	}
	if readThreadCalls != 0 {
		t.Fatalf("inventory preflight bypassed its captured public thread: calls=%d", readThreadCalls)
	}
	if appendCalls != 0 {
		t.Fatalf("inventory preflight wrote events: calls=%d", appendCalls)
	}
}

func TestAcceptedFinalInventoryPreflightBoundsParallelThreadReplay(t *testing.T) {
	threads := map[string]map[string]any{}
	for index := 0; index < 9; index++ {
		threadID := fmt.Sprintf("thr_parallel_%02d", index)
		threads[threadID] = map[string]any{"id": threadID, "turns": []any{}}
	}
	reader := acceptedFinalPublicReaderStub{threads: threads}
	started := make(chan struct{}, 9)
	release := make(chan struct{})
	var mu sync.Mutex
	active := 0
	maximum := 0
	loadCalls := map[string]int{}
	io := FinalPublicationEventIO{
		InventoryReadConcurrency: 3,
		ReadThread: func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return nil, errors.New("per-record thread read must not run during inventory capture")
		},
		LoadEvents: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
			mu.Lock()
			active++
			if active > maximum {
				maximum = active
			}
			loadCalls[threadID]++
			mu.Unlock()
			started <- struct{}{}
			<-release
			mu.Lock()
			active--
			mu.Unlock()
			return []map[string]any{}, nil
		},
	}
	done := make(chan error, 1)
	go func() {
		_, err := PreflightAcceptedFinalEventReconciliations(
			context.Background(), io, &caseTerminalStoreStub{}, reader, nil,
		)
		done <- err
	}()
	for index := 0; index < 3; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("bounded replay workers did not start")
		}
	}
	mu.Lock()
	observedMaximum := maximum
	mu.Unlock()
	if observedMaximum != 3 {
		close(release)
		t.Fatalf("bounded replay concurrency=%d want=3", observedMaximum)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("bounded replay preflight failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(loadCalls) != len(threads) {
		t.Fatalf("bounded replay loaded %d threads want=%d", len(loadCalls), len(threads))
	}
	for threadID, calls := range loadCalls {
		if calls != 1 {
			t.Fatalf("thread %s replay calls=%d want=1", threadID, calls)
		}
	}
}

func TestAcceptedFinalInventoryParallelReplayReturnsFirstSortedError(t *testing.T) {
	firstErr := errors.New("first sorted thread failed")
	secondErr := errors.New("second sorted thread failed")
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		"thr_a": {"id": "thr_a", "turns": []any{}},
		"thr_b": {"id": "thr_b", "turns": []any{}},
	}}
	io := FinalPublicationEventIO{
		InventoryReadConcurrency: 8,
		ReadThread: func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return nil, errors.New("per-record thread read must not run during inventory capture")
		},
		LoadEvents: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
			if threadID == "thr_a" {
				time.Sleep(20 * time.Millisecond)
				return nil, firstErr
			}
			return nil, secondErr
		},
	}
	if _, err := PreflightAcceptedFinalEventReconciliations(
		context.Background(), io, &caseTerminalStoreStub{}, reader, nil,
	); !errors.Is(err, firstErr) {
		t.Fatalf("parallel replay error=%v want first sorted error", err)
	}
}

func TestAuditOnlyFactWinnerNeverRepairsPublicationEvents(t *testing.T) {
	privateRecord, thread, publication, registry, authority := historicalFactAuditFixture(t)
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		privateRecord.SecurityContext.ThreadID: thread,
	}}
	privateStore := &memoryPrivateFinalStore{records: map[string]domainevidence.PrivateAcceptedFinalRecord{
		privateRecord.AcceptedFinal.RecordDigest: privateRecord,
	}}
	historical := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: registry}
	inventory, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, historical, authority, privateStore)
	if err != nil || len(inventory.AuditOnlyPublicWinners) != 1 || len(inventory.AuditOnlyNotCommitted) != 0 ||
		len(inventory.Committed) != 0 || len(inventory.NotCommitted) != 0 || len(inventory.PublicCommitRepairs) != 0 {
		t.Fatalf("historical fact winner was not classified audit-only: inventory=%#v err=%v", inventory, err)
	}
	if err := appturn.ValidateAcceptedFinalTerminalUpdate(
		thread, privateRecord.SecurityContext.TurnID, "completed", publication.TurnItems, publication.TurnFields,
	); err == nil {
		t.Fatal("historical V3 fact bypassed the durable current-write boundary")
	}
	withoutWinner := cloneEventMaps([]map[string]any{thread})[0]
	turns := authorityTurns(withoutWinner)
	delete(turns[0], "acceptedFinal")
	turns[0]["items"] = []any{}
	turns[0]["status"] = "running"
	withoutWinnerReader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		privateRecord.SecurityContext.ThreadID: withoutWinner,
	}}
	notCommitted, err := PreflightFinalAuthorityInventory(
		context.Background(), withoutWinnerReader, withoutWinnerReader, historical, authority, privateStore,
	)
	if err != nil || len(notCommitted.AuditOnlyNotCommitted) != 1 || len(notCommitted.AuditOnlyPublicWinners) != 0 ||
		len(notCommitted.PublicCommitRepairs) != 0 {
		t.Fatalf("historical fact loser became repairable: inventory=%#v err=%v", notCommitted, err)
	}
	store := &caseTerminalStoreStub{}
	for _, test := range []struct {
		name   string
		events []map[string]any
	}{
		{name: "missing", events: nil},
		{name: "partial", events: acceptedFinalPublicationEventsForTest(publication, 1)},
		{name: "complete", events: acceptedFinalPublicationEventsForTest(publication, len(publication.Events))},
	} {
		t.Run(test.name, func(t *testing.T) {
			events := cloneEventMaps(test.events)
			appendCalls := 0
			io := finalPublicationInventoryEventIO(thread, &events, &appendCalls)
			plans, err := PreflightAcceptedFinalEventReconciliationsWithQuarantine(
				context.Background(), io, store, reader, nil, nil,
				[]domainevidence.PrivateAcceptedFinalRecord{privateRecord},
			)
			if err != nil || len(plans) != 0 {
				t.Fatalf("audit-only fact inspection produced a repair plan: plans=%d err=%v", len(plans), err)
			}
			if err := ApplyAcceptedFinalEventReconciliationInventoryWithQuarantine(
				context.Background(), io, store, reader, plans, nil, nil,
				[]domainevidence.PrivateAcceptedFinalRecord{privateRecord},
			); err != nil {
				t.Fatalf("audit-only fact readback failed: %v", err)
			}
			if appendCalls != 0 || !reflect.DeepEqual(events, test.events) {
				t.Fatalf("audit-only fact mutated event history: calls=%d events=%#v", appendCalls, events)
			}
		})
	}

	complete := acceptedFinalPublicationEventsForTest(publication, len(publication.Events))
	appendCalls := 0
	io := finalPublicationInventoryEventIO(thread, &complete, &appendCalls)
	if _, err := PreflightAcceptedFinalEventReconciliationsWithQuarantine(
		context.Background(), io, store, reader, nil, nil, nil,
	); err == nil {
		t.Fatal("audit-only fact marker was accepted without an explicit public-winner inventory")
	}
	if appendCalls != 0 {
		t.Fatal("unknown audit-only marker triggered a repair")
	}
}

func TestUnavailableRegistryKeepsTrustedPrivateFinalAuditOnlyWithoutHistoricalReplay(t *testing.T) {
	privateRecord, thread, _, _, authority := historicalFactAuditFixture(t)
	reader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		privateRecord.SecurityContext.ThreadID: thread,
	}}
	privateStore := &memoryPrivateFinalStore{records: map[string]domainevidence.PrivateAcceptedFinalRecord{
		privateRecord.AcceptedFinal.RecordDigest: privateRecord,
	}}
	historical := &unavailableHistoricalReplayV1{}
	inventory, err := PreflightFinalAuthorityInventory(
		context.Background(), reader, reader, historical, authority, privateStore,
	)
	if err != nil || historical.replayCalls != 0 || len(inventory.AuditOnlyPublicWinners) != 1 ||
		len(inventory.Committed) != 0 || len(inventory.NotCommitted) != 0 || len(inventory.PublicCommitRepairs) != 0 {
		t.Fatalf("unavailable registry did not retain exact audit-only final: inventory=%#v replay=%d err=%v", inventory, historical.replayCalls, err)
	}
}

func TestRegistryAvailabilityKeepsCurrentZeroFactFinalCommittedAndRejectsAuditDowngrade(t *testing.T) {
	ordinary, err := domainordinaryresult.NewResultSlotV1("The ordinary comparison completed.")
	if err != nil {
		t.Fatal(err)
	}
	for _, registryCase := range []struct {
		name        string
		unavailable bool
	}{
		{name: "marker-false-registry-available"},
		{name: "marker-true-registry-unavailable", unavailable: true},
	} {
		for _, headCase := range []struct {
			name     string
			nonempty bool
		}{
			{name: "empty-registry-head"},
			{name: "nonempty-historical-registry-head", nonempty: true},
		} {
			for _, testCase := range []struct {
				name            string
				input           func(domainsecurity.TurnSecurityContext, *caseTerminalStoreStub) PersistCaseBoundaryInput
				expectedVariant domainevidence.FinalAnswerVariant
				ordinary        bool
			}{
				{
					name: "source-unavailable-without-ordinary", expectedVariant: domainevidence.SourceUnavailableAnswer,
					input: func(context domainsecurity.TurnSecurityContext, store *caseTerminalStoreStub) PersistCaseBoundaryInput {
						return PersistCaseBoundaryInput{Store: store, Context: context, TerminalReason: TerminalSourceUnavailable, SourceUnavailable: true}
					},
				},
				{
					name: "needs-evidence-with-typed-ordinary", expectedVariant: domainevidence.NeedsEvidenceAnswer, ordinary: true,
					input: func(context domainsecurity.TurnSecurityContext, store *caseTerminalStoreStub) PersistCaseBoundaryInput {
						return PersistCaseBoundaryInput{Store: store, Context: context, TerminalReason: TerminalSuccess, OrdinaryResult: &ordinary, CaseSlotIntent: CaseSlotRequestedV1}
					},
				},
				{
					name: "general-guidance-with-typed-ordinary", expectedVariant: domainevidence.GeneralGuidanceAnswer, ordinary: true,
					input: func(context domainsecurity.TurnSecurityContext, store *caseTerminalStoreStub) PersistCaseBoundaryInput {
						return PersistCaseBoundaryInput{Store: store, Context: context, TerminalReason: TerminalSuccess, OrdinaryResult: &ordinary, CaseSlotIntent: CaseSlotNotRequestedV1}
					},
				},
			} {
				t.Run(registryCase.name+"/"+headCase.name+"/"+testCase.name, func(t *testing.T) {
					issuer, input := evidenceIssuerFixture(t)
					finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
					issuer.Registry = registry.memoryEvidenceRegistry
					if headCase.nonempty {
						seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
					}

					store := &caseTerminalStoreStub{}
					persistInput := testCase.input(input.Context, store)
					persistInput.ThreadID = input.Context.ThreadID
					persistInput.TurnID = input.Context.TurnID
					persistInput.AcceptedAt = evidenceIssuerTime()
					result, err := finalizer.PersistBoundary(context.Background(), persistInput)
					if err != nil || !result.Persistence.Changed || result.Boundary.Envelope.Variant != testCase.expectedVariant {
						t.Fatalf("current zero-fact accepted final setup failed: result=%#v err=%v", result, err)
					}
					records, err := privateStore.List(context.Background())
					if err != nil || len(records) != 1 || (records[0].RegistryHead.Sequence > 0) != headCase.nonempty ||
						len(records[0].Envelope.Claims) != 0 || len(records[0].Envelope.EvidenceReceiptIDs) != 0 ||
						records[0].PublicationSnapshotProof != nil || records[0].AcceptedFinal.PublicationSnapshotProofDigest != "" ||
						records[0].AcceptedFinal.FactFinalWitnessAdmission != nil {
						t.Fatalf("current zero-fact accepted final has the wrong registry binding: records=%#v err=%v", records, err)
					}
					if testCase.ordinary && (records[0].Envelope.OrdinaryResult == nil || records[0].Envelope.OrdinaryResult.Text != ordinary.Text) {
						t.Fatalf("typed ordinary result was not retained in the private final: %#v", records[0].Envelope.OrdinaryResult)
					}

					var historical registryport.HistoricalReplay = registry
					var unavailable *unavailableHistoricalReplayV1
					if registryCase.unavailable {
						unavailable = &unavailableHistoricalReplayV1{}
						historical = unavailable
					}
					reader := acceptedFinalReaderForStore(input.Context, store)
					inventory, err := PreflightFinalAuthorityInventory(
						context.Background(), reader, reader, historical, authority, privateStore,
					)
					if err != nil || unavailable != nil && unavailable.replayCalls != 0 || len(inventory.Committed) != 1 ||
						inventory.Committed[0].AcceptedFinal.RecordDigest != records[0].AcceptedFinal.RecordDigest ||
						len(inventory.AuditOnlyPublicWinners) != 0 || len(inventory.AuditOnlyNotCommitted) != 0 ||
						len(inventory.NotCommitted) != 0 || len(inventory.PublicCommitRepairs) != 0 {
						replayCalls := 0
						if unavailable != nil {
							replayCalls = unavailable.replayCalls
						}
						t.Fatalf("registry availability degraded a current zero-fact accepted final: inventory=%#v replay=%d err=%v", inventory, replayCalls, err)
					}

					coordinator := newTestTurnTerminalCoordinator(authority, privateStore)
					finishCallsBefore := store.finishCalls
					eventsBefore := cloneEventMaps(store.events)
					privatePutsBefore := privateStore.putCalls
					if _, recoverErr := coordinator.RecoverV1(context.Background(), appturnterminal.RestartRecoveryInputV1{
						CompletionStore: store, CASReader: reader,
						AuditOnlyPrivateInventory: inventory.Committed,
					}); recoverErr == nil || !strings.Contains(recoverErr.Error(), "invalid or executable") {
						t.Fatalf("current zero-fact committed final was accepted after hostile audit-only reclassification: %v", recoverErr)
					}
					if store.finishCalls != finishCallsBefore || !reflect.DeepEqual(store.events, eventsBefore) ||
						privateStore.putCalls != privatePutsBefore {
						t.Fatalf("rejected audit-only reclassification mutated authority: finish=%d/%d private_put=%d/%d events=%#v",
							store.finishCalls, finishCallsBefore, privateStore.putCalls, privatePutsBefore, store.events)
					}
				})
			}
		}
	}
}

func TestUnavailableRegistryKeepsCurrentFactFinalAuditOnlyWithoutHistoricalReplay(t *testing.T) {
	fixture := newConcreteRegistryFinalizerFixture(t)
	result, err := fixture.persist(context.Background())
	if err != nil || !result.Persistence.Changed || result.Persistence.AcceptedFinal.SchemaVersion != domainevidence.AcceptedFinalRecordVersion ||
		result.Persistence.AcceptedFinal.FactFinalWitnessAdmission == nil || result.Persistence.AcceptedFinal.PublicationSnapshotProofDigest == "" {
		t.Fatalf("current witnessed fact-final setup failed: result=%#v err=%v", result, err)
	}

	historical := &unavailableHistoricalReplayV1{}
	reader := acceptedFinalReaderForStore(fixture.securityContext, fixture.store)
	inventory, err := PreflightFinalAuthorityInventory(
		context.Background(), reader, reader, historical, fixture.coordinator.authority, fixture.privateStore,
	)
	if err != nil || historical.replayCalls != 0 || len(inventory.AuditOnlyPublicWinners) != 1 ||
		inventory.AuditOnlyPublicWinners[0].AcceptedFinal.RecordDigest != result.Persistence.AcceptedFinal.RecordDigest ||
		len(inventory.Committed) != 0 || len(inventory.AuditOnlyNotCommitted) != 0 ||
		len(inventory.NotCommitted) != 0 || len(inventory.PublicCommitRepairs) != 0 {
		t.Fatalf("unavailable registry did not isolate the current witnessed fact final: inventory=%#v replay=%d err=%v", inventory, historical.replayCalls, err)
	}
	storeBefore, err := json.Marshal(fixture.store)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := newTestTurnTerminalCoordinator(fixture.coordinator.authority, fixture.privateStore)
	recovery, recoverErr := coordinator.RecoverV1(context.Background(), appturnterminal.RestartRecoveryInputV1{
		CompletionStore: fixture.store, CASReader: reader,
		AuditOnlyPrivateInventory: inventory.AuditOnlyPublicWinners,
	})
	if recoverErr != nil || len(recovery.NonExecutableAuditOnly) != 1 || len(recovery.Complete) != 0 ||
		recovery.NonExecutableAuditOnly[0].AcceptedFinal.RecordDigest != result.Persistence.AcceptedFinal.RecordDigest {
		t.Fatalf("current witnessed fact final escaped non-executable audit retention: recovery=%#v err=%v", recovery, recoverErr)
	}
	storeAfter, err := json.Marshal(fixture.store)
	if err != nil || !bytes.Equal(storeBefore, storeAfter) {
		t.Fatalf("current witnessed fact audit recovery mutated the public CAS: before=%s after=%s err=%v", storeBefore, storeAfter, err)
	}
}

type unavailableHistoricalReplayV1 struct {
	replayCalls int
}

func (*unavailableHistoricalReplayV1) CaseEvidenceAuthorityUnavailableV1() bool { return true }

func (historical *unavailableHistoricalReplayV1) ReplayAt(context.Context, domainsecurity.TurnSecurityContext, uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	historical.replayCalls++
	return domainevidence.EvidenceReceiptRegistry{}, errors.New("blocked historical replay reached")
}

func historicalFactAuditFixture(t *testing.T) (
	domainevidence.PrivateAcceptedFinalRecord,
	map[string]any,
	appturn.AcceptedFinalPublicationPlan,
	*memoryEvidenceRegistry,
	*memoryFinalAuthority,
) {
	t.Helper()
	issuer, input, proposal := claimEvidenceFixture(
		t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete,
	)
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	proposal.EvidenceIDs = []string{receipt.ReceiptID}
	claim, err := claimVerifierForTest(issuer).Verify(context.Background(), input.Context, proposal)
	if err != nil || claim.SupportState != domainevidence.ClaimVerified {
		t.Fatalf("historical fact claim fixture is invalid: claim=%#v err=%v", claim, err)
	}
	envelope, err := (FinalEvidenceGate{Registry: issuer.Registry}).Finalize(context.Background(), FinalGateInput{
		Context: input.Context, TerminalReason: TerminalSuccess, Claims: []domainevidence.ClaimRecord{claim},
		CheckedScope: &input.Material.QueryRange, RequestedScope: &input.Material.QueryRange, IssuedAt: evidenceIssuerTime(),
	})
	if err != nil || !domainevidence.FinalAnswerRequiresPublicationSnapshotProof(envelope) {
		t.Fatalf("historical fact envelope fixture is invalid: envelope=%#v err=%v", envelope, err)
	}
	registry, err := issuer.Registry.Replay(context.Background(), input.Context)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := domainevidence.NewPublicationSnapshotProof(domainevidence.PublicationSnapshotProofInput{
		Context: input.Context, RegistryHead: head, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs,
		Sources: []domainevidence.PublicationSourceSnapshot{{
			ReceiptID: receipt.ReceiptID, ServerID: input.SourceProbe.ServerID, ServerIdentity: input.SourceProbe.ServerIdentity,
			ServerVersion: input.Material.ServerVersion, ConnectionEpoch: input.SourceProbe.ConnectionEpoch,
			ToolName: input.Grant.ToolName, DatasetSnapshotID: input.Context.DatasetSnapshotID,
			CatalogFingerprint: input.SourceProbe.CatalogFingerprint, SpecFingerprint: input.SourceProbe.SpecFingerprint,
			ProbeDigest: input.SourceProbe.ProbeDigest, CheckedAt: input.SourceProbe.CheckedAt,
		}},
		CheckedAt: evidenceIssuerTime().Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: evidenceIssuerTime().Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigestBody := struct {
		SchemaVersion            int                                      `json:"schemaVersion"`
		SecurityContext          domainsecurity.TurnSecurityContext       `json:"securityContext"`
		Envelope                 domainevidence.FinalAnswerEnvelope       `json:"envelope"`
		RenderedText             string                                   `json:"renderedText"`
		Intent                   domainevidence.TerminalPublicationIntent `json:"publicationIntent"`
		PublicationSnapshotProof *domainevidence.PublicationSnapshotProof `json:"publicationSnapshotProof,omitempty"`
	}{domainevidence.BoundaryAcceptedFinalRecordVersion, input.Context, envelope, rendered, intent, &proof}
	privateDigestBytes, _ := json.Marshal(privateDigestBody)
	privateDigest := domainsecurity.SHA256Hex(privateDigestBytes)
	authority := newMemoryFinalAuthority(211)
	acceptedAt := evidenceIssuerTime().Add(2 * time.Second).Format(time.RFC3339Nano)
	accepted := domainevidence.AcceptedFinalRecord{
		SchemaVersion:    domainevidence.BoundaryAcceptedFinalRecordVersion,
		AuthorityPurpose: domainevidence.AcceptedFinalAuthorityPurpose, AuthorityAlgorithm: domainevidence.AcceptedFinalAuthorityAlgorithm,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(authority.PublicKey()),
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, EnvelopeDigest: envelope.EnvelopeDigest,
		ContextDigest: input.Context.ContextDigest, ContextEpoch: input.Context.ContextEpoch, DatasetSnapshotID: input.Context.DatasetSnapshotID,
		Variant: envelope.Variant, TerminalReason: envelope.TerminalReason, RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(rendered)),
		RegistrySequence: head.Sequence, RegistryStateDigest: head.StateDigest, RendererVersion: domainevidence.FinalAnswerRendererVersion,
		FinalGateVersion: domainevidence.HistoricalBoundaryFinalGateVersion, VerifierVersion: domainevidence.ClaimVerifierPolicyVersion,
		PublicationSnapshotProofDigest: proof.ProofDigest, PrivateRecordDigest: privateDigest, AcceptedAt: acceptedAt,
	}
	signature, err := authority.Sign(context.Background(), domainevidence.AcceptedFinalSigningBytes(accepted))
	if err != nil {
		t.Fatal(err)
	}
	accepted.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	acceptedDigestBody, _ := json.Marshal(accepted)
	accepted.RecordDigest = domainsecurity.SHA256Hex(acceptedDigestBody)
	privateRecord := domainevidence.PrivateAcceptedFinalRecord{
		SchemaVersion: domainevidence.BoundaryAcceptedFinalRecordVersion, SecurityContext: input.Context, Envelope: envelope,
		RenderedText: rendered, RegistryHead: head, PublicationIntent: intent, PublicationSnapshotProof: &proof,
		AcceptedFinal: accepted, PrivateRecordDigest: privateDigest,
	}
	storeDigestBody, _ := json.Marshal(privateRecord)
	privateRecord.StoreDigest = domainsecurity.SHA256Hex(storeDigestBody)
	if err := domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(privateRecord); err != nil {
		t.Fatalf("historical fact private fixture is invalid: %v", err)
	}
	publication, err := appturn.BuildAcceptedFinalAuditPublicationPlan(accepted, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	terminalStore := &caseTerminalStoreStub{status: "completed", items: publication.TurnItems, fields: publication.TurnFields}
	thread := acceptedFinalReaderForStore(input.Context, terminalStore).threads[input.Context.ThreadID]
	registryStore, ok := issuer.Registry.(*memoryEvidenceRegistry)
	if !ok {
		t.Fatal("historical fact fixture registry type is invalid")
	}
	return privateRecord, thread, publication, registryStore, authority
}

func acceptedFinalPublicationEventsForTest(publication appturn.AcceptedFinalPublicationPlan, count int) []map[string]any {
	if count > len(publication.Events) {
		count = len(publication.Events)
	}
	events := make([]map[string]any, 0, count)
	for index := 0; index < count; index++ {
		event := cloneEventMaps([]map[string]any{publication.Events[index].Draft})[0]
		event["seq"] = float64(index + 1)
		events = append(events, event)
	}
	return events
}

func finalPublicationInventoryEventIO(thread map[string]any, events *[]map[string]any, appendCalls *int) FinalPublicationEventIO {
	return FinalPublicationEventIO{
		ReadThread: func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return thread, nil
		},
		LoadEvents: func(context.Context, appturn.AcceptedFinalCompletionStore, string) ([]map[string]any, error) {
			return cloneEventMaps(*events), nil
		},
		AppendEvents: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, bundle []map[string]any) ([]map[string]any, error) {
			written := cloneEventMaps(bundle)
			for index := range written {
				written[index]["seq"] = float64(len(*events) + index + 1)
			}
			*events = append(*events, written...)
			(*appendCalls)++
			return cloneEventMaps(written), nil
		},
	}
}

func TestCASSidecarFailureReconcilesObservableCommittedTurn(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{finishErr: errors.New("metadata sidecar sync failed")}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil || !result.Persistence.Changed || len(store.events) != len(result.Persistence.Publication.Events) {
		t.Fatalf("observable committed CAS was not reconciled after sidecar error: result=%#v events=%#v err=%v", result, store.events, err)
	}
	dispositions, err := privateStore.ListDispositions(context.Background())
	if err != nil || len(dispositions) != 1 || dispositions[0].State != domainevidence.AcceptedFinalCommitted {
		t.Fatalf("observable committed CAS lacks committed disposition: %#v err=%v", dispositions, err)
	}
	reader := acceptedFinalReaderForStore(input.Context, store)
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err != nil {
		t.Fatalf("reconciled sidecar failure did not survive preflight: %v", err)
	}
}

func finalPublicationEventIOStub(thread map[string]any, events *[]map[string]any) FinalPublicationEventIO {
	return FinalPublicationEventIO{
		ReadThread: func(context.Context, appturn.AcceptedFinalCompletionStore, domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return thread, nil
		},
		LoadEvents: func(context.Context, appturn.AcceptedFinalCompletionStore, string) ([]map[string]any, error) {
			return cloneEventMaps(*events), nil
		},
		AppendEvents: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, bundle []map[string]any) ([]map[string]any, error) {
			written := cloneEventMaps(bundle)
			for index := range written {
				written[index]["seq"] = float64(len(*events) + index + 1)
			}
			*events = append(*events, written...)
			return cloneEventMaps(written), nil
		},
	}
}

func TestFinalAuthorityPreflightResumesPriorTerminalIntentWithoutNewAttempt(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, registry, authority, privateStore := newTestCasePublicationFinalizer()
	firstStore := &caseTerminalStoreStub{unchanged: true}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: firstStore, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err == nil {
		t.Fatal("winnerless first attempt did not remain unresolved")
	}
	secondStore := &caseTerminalStoreStub{status: "running"}
	reader := caseTerminalDynamicReader{securityContext: input.Context, store: secondStore}
	inventory, err := PreflightFinalAuthorityInventory(context.Background(), reader, reader, registry, authority, privateStore)
	if err != nil || len(inventory.PublicCommitRepairs) != 1 {
		t.Fatalf("prepared terminal intent was not recoverable: inventory=%#v err=%v", inventory, err)
	}
	if _, err := recoverPreflightCandidatesV1(context.Background(), finalizer, secondStore, reader, privateStore, publicRepairCandidatesV1(inventory)); err != nil {
		t.Fatal(err)
	}
	records, err := privateStore.List(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("terminal recovery created another private attempt: records=%d err=%v", len(records), err)
	}
	if err := PreflightFinalAuthority(context.Background(), reader, reader, registry, authority, privateStore); err != nil {
		t.Fatalf("recovered prior terminal intent failed preflight: %v", err)
	}
}

func publicRepairCandidatesV1(inventory FinalAuthorityInventory) []domainevidence.PrivateAcceptedFinalRecord {
	candidates := make([]domainevidence.PrivateAcceptedFinalRecord, 0, len(inventory.PublicCommitRepairs))
	for _, repair := range inventory.PublicCommitRepairs {
		candidates = append(candidates, repair.PrivateRecord)
	}
	return candidates
}

func recoverPreflightCandidatesV1(
	ctx context.Context,
	finalizer CasePublicationFinalizer,
	store appturn.AcceptedFinalCompletionStore,
	reader authorityport.AcceptedFinalCASReader,
	privateStore *memoryPrivateFinalStore,
	candidates []domainevidence.PrivateAcceptedFinalRecord,
) (appturnterminal.RestartRecoveryResultV1, error) {
	records, err := privateStore.List(ctx)
	if err != nil {
		return appturnterminal.RestartRecoveryResultV1{}, err
	}
	return finalizer.(*casePublicationFinalizer).terminalCoordinator.RecoverV1(ctx, appturnterminal.RestartRecoveryInputV1{
		CompletionStore: store, CASReader: reader, PrivateInventory: records, Candidates: candidates,
	})
}

func TestUnconfiguredFinalizerWithoutAuthorityFailsClosed(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	store := &caseTerminalStoreStub{}
	if _, err := (&casePublicationFinalizer{}).PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err == nil || store.fields != nil || len(store.events) != 0 {
		t.Fatalf("compatibility finalizer published without authority: fields=%#v events=%#v err=%v", store.fields, store.events, err)
	}
}

func TestAcceptedFinalEventReplayRejectsTamperMissingAndUnknownCaseText(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, _ := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	thread := acceptedFinalReaderForStore(input.Context, store).threads[input.Context.ThreadID]
	if err := ValidateAcceptedFinalEventReplay(thread, store.events); err != nil {
		t.Fatalf("valid accepted-final event replay failed: %v", err)
	}
	if err := ValidateAcceptedFinalEventReplay(thread, store.events[:len(store.events)-1]); err == nil {
		t.Fatal("missing accepted-final terminal event passed replay")
	}
	for _, privateField := range []string{
		"publicationSnapshotProof", "securityContext", "envelope", "registryHead", "publicationIntent", "storeDigest",
	} {
		privateAuthorityLeak := cloneEventMaps(store.events)
		privateItem := privateAuthorityLeak[0]["item"].(map[string]any)
		privateItem[privateField] = map[string]any{"privateSentinel": true}
		if err := ValidateAcceptedFinalEventReplay(thread, privateAuthorityLeak); err == nil {
			t.Fatalf("host-private accepted-final field %q entered generic event replay", privateField)
		}
	}
	tampered := cloneEventMaps(store.events)
	item := tampered[0]["item"].(map[string]any)
	item["text"] = "伪造金额 420 万元"
	if err := ValidateAcceptedFinalEventReplay(thread, tampered); err == nil {
		t.Fatal("tampered accepted-final item event passed replay")
	}
	unknown := cloneEventMaps(store.events)
	unknown = append(unknown, map[string]any{
		"kind": "item_completed", "threadId": input.Context.ThreadID, "turnId": "turn-forged",
		"item": map[string]any{"kind": "assistant_text", "role": "assistant", "threadId": input.Context.ThreadID, "turnId": "turn-forged", "text": "伪造案件事实"},
	})
	if err := ValidateAcceptedFinalEventReplay(thread, unknown); err == nil {
		t.Fatal("unknown-turn case assistant event passed replay")
	}
	unknownDelta := cloneEventMaps(store.events)
	unknownDelta = append(unknownDelta, map[string]any{
		"kind": "assistant_text_delta", "threadId": input.Context.ThreadID, "turnId": "turn-forged", "text": "金额 420 万元",
	})
	if err := ValidateAcceptedFinalEventReplay(thread, unknownDelta); err == nil {
		t.Fatal("unknown-turn case assistant delta passed replay")
	}
}

func TestAcceptedFinalEventReplayAllowsKnownUnboundTurnBeforeCaseBinding(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, _ := newTestCasePublicationFinalizer()
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	thread := acceptedFinalReaderForStore(input.Context, store).threads[input.Context.ThreadID]
	unbound := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.Context.ThreadID, TurnID: "turn-before-case", WorkspaceRealPath: input.Context.WorkspaceRealPath,
		ContextEpoch: 1, IssuedAt: evidenceIssuerTime().Add(-time.Minute),
	})
	turns := authorityTurns(thread)
	thread["turns"] = []any{
		map[string]any{
			"id": unbound.TurnID, "status": "completed", "securityContext": turnSecurityContextRecord(unbound),
			"items": []any{map[string]any{"kind": "assistant_text", "text": "ordinary pre-case answer"}},
		},
		turns[0],
	}
	events := []map[string]any{
		{"kind": "assistant_text_delta", "threadId": unbound.ThreadID, "turnId": unbound.TurnID, "delta": "ordinary pre-case answer"},
		{"kind": "item_completed", "threadId": unbound.ThreadID, "turnId": unbound.TurnID, "item": map[string]any{
			"kind": "assistant_text", "threadId": unbound.ThreadID, "turnId": unbound.TurnID, "text": "ordinary pre-case answer",
		}},
	}
	events = append(events, store.events...)
	if err := ValidateAcceptedFinalEventReplay(thread, events); err != nil {
		t.Fatalf("known unbound history was rejected after the thread became case-bound: %v", err)
	}
}

func cloneEventMaps(events []map[string]any) []map[string]any {
	body, _ := json.Marshal(events)
	cloned := []map[string]any{}
	_ = json.Unmarshal(body, &cloned)
	return cloned
}

func acceptedFinalReaderForStore(securityContext domainsecurity.TurnSecurityContext, store *caseTerminalStoreStub) acceptedFinalPublicReaderStub {
	thread, _ := store.FinalPublicationThread(securityContext)
	return acceptedFinalPublicReaderStub{threads: map[string]map[string]any{
		securityContext.ThreadID: thread,
	}}
}
