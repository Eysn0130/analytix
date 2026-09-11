package turnterminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	turnterminalstoreport "analytix.local/runtime-go/internal/ports/turnterminalstore"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
)

func historicalBoundaryPrivateFinalV3(t *testing.T, fixture terminaltest.FixtureV1) domainevidence.PrivateAcceptedFinalRecord {
	return historicalBoundaryPrivateFinalAtVersion(t, fixture, domainevidence.BoundaryAcceptedFinalRecordVersion)
}

func historicalBoundaryPrivateFinalAtVersion(
	t *testing.T,
	fixture terminaltest.FixtureV1,
	version int,
) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	current := fixture.PrivateFinal
	privateDigestBody, err := json.Marshal(struct {
		SchemaVersion             int                                         `json:"schemaVersion"`
		SecurityContext           domainsecurity.TurnSecurityContext          `json:"securityContext"`
		Envelope                  domainevidence.FinalAnswerEnvelope          `json:"envelope"`
		RenderedText              string                                      `json:"renderedText"`
		PublicationIntent         domainevidence.TerminalPublicationIntent    `json:"publicationIntent"`
		PublicationSnapshotProof  *domainevidence.PublicationSnapshotProof    `json:"publicationSnapshotProof,omitempty"`
		FactFinalWitnessAdmission *domainevidence.FactFinalWitnessAdmissionV1 `json:"factFinalWitnessAdmission,omitempty"`
	}{
		SchemaVersion: version, SecurityContext: current.SecurityContext,
		Envelope: current.Envelope, RenderedText: current.RenderedText, PublicationIntent: current.PublicationIntent,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateDigest := domainsecurity.SHA256Hex(privateDigestBody)
	accepted := current.AcceptedFinal
	accepted.SchemaVersion = version
	switch version {
	case domainevidence.PreviousAcceptedFinalRecordVersion:
		accepted.FinalGateVersion = domainevidence.LegacyFinalEvidenceGateVersion
	case domainevidence.BoundaryAcceptedFinalRecordVersion:
		accepted.FinalGateVersion = domainevidence.HistoricalBoundaryFinalGateVersion
	default:
		t.Fatalf("unsupported historical boundary version: %d", version)
	}
	accepted.PublicView = nil
	accepted.PublicViewDigest = ""
	accepted.PublicationSnapshotProofDigest = ""
	accepted.FactFinalWitnessAdmission = nil
	accepted.PrivateRecordDigest = privateDigest
	accepted.AuthoritySignature = ""
	accepted.RecordDigest = ""
	signature, err := fixture.Sign(domainevidence.AcceptedFinalSigningBytes(accepted))
	if err != nil {
		t.Fatal(err)
	}
	accepted.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	acceptedDigestBody, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	accepted.RecordDigest = domainsecurity.SHA256Hex(acceptedDigestBody)
	historical := current
	historical.SchemaVersion = version
	historical.PublicationSnapshotProof = nil
	historical.AcceptedFinal = accepted
	historical.PrivateRecordDigest = privateDigest
	historical.StoreDigest = ""
	storeDigestBody, err := json.Marshal(historical)
	if err != nil {
		t.Fatal(err)
	}
	historical.StoreDigest = domainsecurity.SHA256Hex(storeDigestBody)
	if err := domainevidence.ValidatePrivateAcceptedFinalAuditAuthority(historical); err != nil {
		t.Fatalf("historical V%d boundary fixture is invalid: %v", version, err)
	}
	if err := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(historical); err == nil {
		t.Fatalf("historical V%d boundary fixture retained executable publication authority", version)
	}
	return historical
}

func currentZeroFactPrivateFinalV5(
	t *testing.T,
	fixture terminaltest.FixtureV1,
	input domainevidence.FinalAnswerEnvelopeInput,
	heads ...domainevidence.EvidenceRegistryHead,
) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	now := terminaltest.FixtureTimeV1()
	input.Context = fixture.Context
	input.IssuedAt = now
	envelope, err := domainevidence.NewFinalAnswerEnvelope(input)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) > 1 {
		t.Fatal("zero-fact private-final fixture accepts at most one registry head")
	}
	if len(heads) == 1 {
		head = heads[0]
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(fixture.Context, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: fixture.Context, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	privateFinal, err := domainevidence.NewPrivateAcceptedFinalRecord(
		fixture.Context, envelope, rendered, head, intent, accepted,
	)
	if err != nil {
		t.Fatal(err)
	}
	return privateFinal
}

func historicalTurnTerminalIntentV1(
	t *testing.T,
	fixture terminaltest.FixtureV1,
	historical domainevidence.PrivateAcceptedFinalRecord,
	manifestDigest string,
) domainturnterminal.TurnTerminalIntentV1 {
	t.Helper()
	intent := fixture.Intent
	intent.AcceptedFinalDigest = historical.AcceptedFinal.RecordDigest
	intent.PrivateFinalStoreDigest = historical.StoreDigest
	intent.EventManifestDigest = manifestDigest
	materialBody, err := json.Marshal(struct {
		ContextDigest           string                                            `json:"contextDigest"`
		TerminalReasonCode      domaincachetelemetry.ProviderTurnTerminalReasonV1 `json:"terminalReasonCode"`
		AcceptedFinalDigest     string                                            `json:"acceptedFinalDigest"`
		PrivateFinalStoreDigest string                                            `json:"privateFinalStoreDigest"`
		EventManifestDigest     string                                            `json:"eventManifestDigest"`
	}{
		ContextDigest: intent.SecurityContext.ContextDigest, TerminalReasonCode: intent.TerminalReasonCode,
		AcceptedFinalDigest: intent.AcceptedFinalDigest, PrivateFinalStoreDigest: intent.PrivateFinalStoreDigest,
		EventManifestDigest: intent.EventManifestDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent.PublicationMaterialDigest = domainsecurity.SHA256Hex(append(
		[]byte("analytix/turn-terminal-publication-material/v1\x00"), materialBody...,
	))
	intent.IntentID = ""
	intent.AuthoritySignature = ""
	intentIDBody, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	intent.IntentID = domainsecurity.SHA256Hex(append(
		[]byte("analytix/turn-terminal-intent-id/v1\x00"), intentIDBody...,
	))
	signature, err := fixture.Sign(domainturnterminal.TurnTerminalIntentV1SigningBytes(intent))
	if err != nil {
		t.Fatal(err)
	}
	intent.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := domainturnterminal.ValidateTurnTerminalIntentForPrivateFinalAuditV1(intent, historical); err != nil {
		t.Fatalf("historical terminal intent fixture is invalid: %v", err)
	}
	return intent
}

type recoveryInventoryPrivateStoreV1 struct {
	records      []domainevidence.PrivateAcceptedFinalRecord
	dispositions []domainevidence.AcceptedFinalDispositionRecord
}

func (store *recoveryInventoryPrivateStoreV1) PutIfAbsent(context.Context, domainevidence.PrivateAcceptedFinalRecord) error {
	return errors.New("recovery inventory test store is read-only")
}
func (store *recoveryInventoryPrivateStoreV1) Resolve(context.Context, string) (domainevidence.PrivateAcceptedFinalRecord, error) {
	return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("recovery inventory private final is missing")
}
func (store *recoveryInventoryPrivateStoreV1) List(context.Context) ([]domainevidence.PrivateAcceptedFinalRecord, error) {
	return append([]domainevidence.PrivateAcceptedFinalRecord(nil), store.records...), nil
}
func (store *recoveryInventoryPrivateStoreV1) HasRecords(context.Context) (bool, error) {
	return len(store.records) != 0 || len(store.dispositions) != 0, nil
}
func (store *recoveryInventoryPrivateStoreV1) PutDispositionIfAbsent(context.Context, domainevidence.AcceptedFinalDispositionRecord) error {
	return errors.New("recovery inventory test store is read-only")
}
func (store *recoveryInventoryPrivateStoreV1) ResolveDisposition(context.Context, string) (domainevidence.AcceptedFinalDispositionRecord, error) {
	return domainevidence.AcceptedFinalDispositionRecord{}, errors.New("recovery inventory disposition is missing")
}
func (store *recoveryInventoryPrivateStoreV1) ListDispositions(context.Context) ([]domainevidence.AcceptedFinalDispositionRecord, error) {
	return append([]domainevidence.AcceptedFinalDispositionRecord(nil), store.dispositions...), nil
}

type revalidatingTerminalStoreV1 struct {
	*memoryTerminalStoreV1
	lateIntent domainturnterminal.TurnTerminalIntentV1
	visits     int
}

func (store *revalidatingTerminalStoreV1) ReadIntent(context.Context, string) (domainturnterminal.TurnTerminalIntentV1, error) {
	if store.visits < 2 {
		return domainturnterminal.TurnTerminalIntentV1{}, turnterminalstoreport.ErrNotFound
	}
	return store.lateIntent, nil
}

func (store *revalidatingTerminalStoreV1) VisitIntents(_ context.Context, visit func(domainturnterminal.TurnTerminalIntentV1) error) error {
	store.visits++
	if store.visits < 2 {
		return nil
	}
	return visit(store.lateIntent)
}

func TestRestartRecoveryCompletesValidIntentPrefixInOrder(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Complete) != 1 || len(result.LegacyQuarantined) != 0 || completion.publicCalls != 1 || closer.calls != 1 ||
		terminals.intent == nil || terminals.disposition == nil || privateFinals.disposition == nil {
		t.Fatalf("restart terminal recovery did not complete its exact chain: result=%#v public=%d closure=%d", result, completion.publicCalls, closer.calls)
	}
	assertTraceOrderV1(t, trace.snapshot(), "intent_put", "provider_close", "public_cas", "accepted_disposition_put", "terminal_disposition_put")
}

func TestRestartRecoveryQuarantinesPublicWinnerWithoutPriorIntent(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context, winner: fixture.PrivateFinal.AcceptedFinal}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Complete) != 0 || len(result.LegacyQuarantined) != 1 || terminals.intent != nil || terminals.disposition != nil ||
		closer.calls != 0 || completion.publicCalls != 0 || privateFinals.disposition != nil {
		t.Fatalf("legacy public winner was mutated or trusted: result=%#v", result)
	}
}

func TestRestartRecoveryClosesHistoricalV3AuditInventoryWithoutWrites(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	historical := historicalBoundaryPrivateFinalV3(t, fixture)
	trace := &coordinatorTraceV1{}
	completion := &memoryCompletionCASV1{trace: trace, context: historical.SecurityContext, winner: historical.AcceptedFinal}
	observation, err := completion.ReadAcceptedFinalCASObservation(
		context.Background(), historical.SecurityContext.ThreadID, historical.SecurityContext.TurnID,
	)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := appturn.BuildAcceptedFinalAuditPublicationPlan(
		historical.AcceptedFinal, historical.RenderedText, historical.PublicationIntent,
	)
	if err != nil {
		t.Fatal(err)
	}
	decidedAt, err := time.Parse(time.RFC3339Nano, historical.AcceptedFinal.AcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: historical.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: publication.EventManifestDigest, DecidedAt: decidedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, observation, domainevidence.AcceptedFinalDecisionSamePublicWinner, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	privateFinals := &recoveryInventoryPrivateStoreV1{
		records:      []domainevidence.PrivateAcceptedFinalRecord{historical},
		dispositions: []domainevidence.AcceptedFinalDispositionRecord{disposition},
	}
	terminals := &memoryTerminalStoreV1{trace: trace}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{historical},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.NonExecutableAuditOnly) != 1 ||
		result.NonExecutableAuditOnly[0].AcceptedFinal.RecordDigest != historical.AcceptedFinal.RecordDigest ||
		len(result.Complete) != 0 || len(result.LegacyQuarantined) != 0 || len(result.AuditOnly) != 0 {
		t.Fatalf("historical V3 final escaped non-executable audit classification: %#v", result)
	}
	if completion.publicCalls != 0 || closer.calls != 0 || terminals.intent != nil || terminals.disposition != nil ||
		terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("historical V3 audit recovery caused a write: trace=%v", trace.snapshot())
	}
}

func TestRestartRecoveryClosesHistoricalV2AuditInventoryWithoutWrites(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	historical := historicalBoundaryPrivateFinalAtVersion(t, fixture, domainevidence.PreviousAcceptedFinalRecordVersion)
	trace := &coordinatorTraceV1{}
	completion := &memoryCompletionCASV1{
		trace: trace, context: historical.SecurityContext, winner: fixture.PrivateFinal.AcceptedFinal,
	}
	privateFinals := &recoveryInventoryPrivateStoreV1{
		records: []domainevidence.PrivateAcceptedFinalRecord{historical},
	}
	coordinator, err := NewCoordinator(
		newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals,
		&memoryTerminalStoreV1{trace: trace}, &memoryProviderCloserV1{trace: trace, closure: fixture.Closure},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{historical},
	})
	if err != nil || len(result.NonExecutableAuditOnly) != 1 || len(result.Complete) != 0 ||
		result.NonExecutableAuditOnly[0].AcceptedFinal.RecordDigest != historical.AcceptedFinal.RecordDigest {
		t.Fatalf("historical V2 final escaped non-executable audit-only retention: result=%#v err=%v", result, err)
	}
	if completion.publicCalls != 0 || terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("historical V2 audit recovery caused a write: trace=%v", trace.snapshot())
	}
}

func TestHistoricalCompleteChainNeverEntersExecutableRecovery(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	historical := historicalBoundaryPrivateFinalV3(t, fixture)
	trace := &coordinatorTraceV1{}
	completion := &memoryCompletionCASV1{trace: trace, context: historical.SecurityContext, winner: historical.AcceptedFinal}
	observation, err := completion.ReadAcceptedFinalCASObservation(
		context.Background(), historical.SecurityContext.ThreadID, historical.SecurityContext.TurnID,
	)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := appturn.BuildAcceptedFinalAuditPublicationPlan(
		historical.AcceptedFinal, historical.RenderedText, historical.PublicationIntent,
	)
	if err != nil {
		t.Fatal(err)
	}
	intent := historicalTurnTerminalIntentV1(t, fixture, historical, publication.EventManifestDigest)
	closedAt, err := time.Parse(time.RFC3339Nano, fixture.Closure.ClosedAt)
	if err != nil {
		t.Fatal(err)
	}
	acceptedDisposition, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: historical.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: publication.EventManifestDigest, DecidedAt: closedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
	}, observation, domainevidence.AcceptedFinalDecisionSamePublicWinner, fixture.Sign)
	if err != nil {
		t.Fatal(err)
	}
	terminalDisposition, err := domainturnterminal.NewTurnTerminalDispositionV1(
		domainturnterminal.TurnTerminalDispositionInputV1{
			Intent: intent, ProviderClosure: fixture.Closure, AcceptedFinalDisposition: acceptedDisposition,
			AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
		}, fixture.Sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	privateFinals := &recoveryInventoryPrivateStoreV1{
		records:      []domainevidence.PrivateAcceptedFinalRecord{historical},
		dispositions: []domainevidence.AcceptedFinalDispositionRecord{acceptedDisposition},
	}
	terminals := &memoryTerminalStoreV1{trace: trace, intent: &intent, disposition: &terminalDisposition}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure, calls: 1}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	publicCallsBefore := completion.publicCalls
	closureCallsBefore := closer.calls
	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{historical},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.NonExecutableAuditOnly) != 1 || len(result.Complete) != 0 ||
		result.NonExecutableAuditOnly[0].AcceptedFinal.RecordDigest != historical.AcceptedFinal.RecordDigest {
		t.Fatalf("historical complete chain entered executable recovery: %#v", result)
	}
	if completion.publicCalls != publicCallsBefore || closer.calls != closureCallsBefore ||
		terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("historical complete chain replayed writes: trace=%v", trace.snapshot())
	}
}

func TestHistoricalPartialPrefixNeverResumesOrBackfills(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	historical := historicalBoundaryPrivateFinalV3(t, fixture)
	publication, err := appturn.BuildAcceptedFinalAuditPublicationPlan(
		historical.AcceptedFinal, historical.RenderedText, historical.PublicationIntent,
	)
	if err != nil {
		t.Fatal(err)
	}
	intent := historicalTurnTerminalIntentV1(t, fixture, historical, publication.EventManifestDigest)
	for _, test := range []struct {
		name         string
		closureCalls int
	}{
		{name: "intent-only", closureCalls: 0},
		{name: "intent-and-provider-closure", closureCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			trace := &coordinatorTraceV1{}
			completion := &memoryCompletionCASV1{trace: trace, context: historical.SecurityContext}
			privateFinals := &recoveryInventoryPrivateStoreV1{
				records: []domainevidence.PrivateAcceptedFinalRecord{historical},
			}
			terminals := &memoryTerminalStoreV1{trace: trace, intent: &intent}
			closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure, calls: test.closureCalls}
			coordinator, err := NewCoordinator(
				newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer,
			)
			if err != nil {
				t.Fatal(err)
			}
			publicCallsBefore := completion.publicCalls
			closureCallsBefore := closer.calls
			result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
				CompletionStore: completion, CASReader: completion,
				AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{historical},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.NonExecutableAuditOnly) != 1 || len(result.Complete) != 0 {
				t.Fatalf("historical partial prefix became executable: %#v", result)
			}
			if completion.publicCalls != publicCallsBefore || closer.calls != closureCallsBefore ||
				terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
				t.Fatalf("historical partial prefix was resumed or backfilled: trace=%v", trace.snapshot())
			}
		})
	}
}

func TestRestartRecoveryRejectsCurrentZeroFactV5InAuditOnlyInventoryRegardlessOfShape(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := domainordinaryresult.NewResultSlotV1("The ordinary comparison completed.")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	emptyHead, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	historicalHead := emptyHead
	historicalHead.Sequence = 1
	historicalHead.StateDigest = domainsecurity.SHA256Hex([]byte("historical-registry-head"))
	for _, headCase := range []struct {
		name string
		head domainevidence.EvidenceRegistryHead
	}{
		{name: "empty-registry-head", head: emptyHead},
		{name: "nonempty-historical-registry-head", head: historicalHead},
	} {
		for _, testCase := range []struct {
			name  string
			input domainevidence.FinalAnswerEnvelopeInput
		}{
			{name: "current-case-source-unavailable", input: domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.SourceUnavailableAnswer, TerminalReason: string(domaincachetelemetry.ProviderTurnTerminalSourceUnavailableV1),
				Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"restore_current_case_source"},
			}},
			{name: "case-public-authority-unavailable", input: domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.SourceUnavailableAnswer, TerminalReason: string(domaincachetelemetry.ProviderTurnTerminalSourceUnavailableV1),
				Blocker: domainevidence.CaseEvidenceAuthorityUnavailableBlockerV1, AcquisitionSteps: []string{"restore_current_case_source"},
			}},
			{name: "case-slot-requested-with-valid-ordinary", input: domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.NeedsEvidenceAnswer, TerminalReason: "success", OrdinaryResult: &ordinary,
				MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_same_context_evidence"},
			}},
			{name: "general-guidance-ordinary-only", input: domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.GeneralGuidanceAnswer, TerminalReason: "success", OrdinaryResult: &ordinary,
				Guidance: []string{domainevidence.OrdinaryResultOnlyGuidanceCodeV1},
			}},
		} {
			t.Run(headCase.name+"/"+testCase.name, func(t *testing.T) {
				record := currentZeroFactPrivateFinalV5(t, fixture, testCase.input, headCase.head)
				trace := &coordinatorTraceV1{}
				privateFinals := &recoveryInventoryPrivateStoreV1{records: []domainevidence.PrivateAcceptedFinalRecord{record}}
				completion := &memoryCompletionCASV1{trace: trace, context: record.SecurityContext}
				coordinator, err := NewCoordinator(
					newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals,
					&memoryTerminalStoreV1{trace: trace}, &memoryProviderCloserV1{trace: trace, closure: fixture.Closure},
				)
				if err != nil {
					t.Fatal(err)
				}
				result, recoverErr := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
					CompletionStore: completion, CASReader: completion,
					AuditOnlyPrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{record},
				})
				if recoverErr == nil || !strings.Contains(recoverErr.Error(), "invalid or executable") {
					t.Fatalf("current zero-fact final entered audit-only inventory: result=%#v err=%v", result, recoverErr)
				}
				if terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
					t.Fatalf("rejected zero-fact audit downgrade wrote terminal state: %v", trace.snapshot())
				}
			})
		}
	}
}

func TestRestartRecoveryKeepsCurrentZeroFactV5TerminalCompleteWithHistoricalRegistryHead(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := domainordinaryresult.NewResultSlotV1("The ordinary comparison completed.")
	if err != nil {
		t.Fatalf("typed ordinary result fixture is invalid: %v", err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	head.Sequence = 1
	head.StateDigest = domainsecurity.SHA256Hex([]byte("nonempty-unavailable-registry-head"))
	for _, testCase := range []struct {
		name  string
		input domainevidence.FinalAnswerEnvelopeInput
	}{
		{
			name: "zero-fact-without-ordinary",
			input: domainevidence.FinalAnswerEnvelopeInput{
				Variant:          domainevidence.SourceUnavailableAnswer,
				TerminalReason:   string(domaincachetelemetry.ProviderTurnTerminalSourceUnavailableV1),
				Blocker:          "current_case_source_unavailable",
				AcquisitionSteps: []string{"restore_case_evidence_authority"},
			},
		},
		{
			name: "case-slot-requested-with-valid-ordinary",
			input: domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.NeedsEvidenceAnswer, TerminalReason: "success", OrdinaryResult: &ordinary,
				MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_same_context_evidence"},
			},
		},
		{
			name: "general-guidance-ordinary-only",
			input: domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.GeneralGuidanceAnswer, TerminalReason: "success", OrdinaryResult: &ordinary,
				Guidance: []string{domainevidence.OrdinaryResultOnlyGuidanceCodeV1},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			record := currentZeroFactPrivateFinalV5(t, fixture, testCase.input, head)
			if err := domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(record); err != nil {
				t.Fatalf("current zero-fact fixture is not executable: %v", err)
			}
			trace := &coordinatorTraceV1{}
			privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: record}
			terminals := &memoryTerminalStoreV1{trace: trace}
			completion := &memoryCompletionCASV1{trace: trace, context: record.SecurityContext}
			publication, err := appturn.BuildAcceptedFinalPublicationPlan(
				record.AcceptedFinal, record.RenderedText, record.PublicationIntent,
			)
			if err != nil {
				t.Fatal(err)
			}
			intent, err := domainturnterminal.NewTurnTerminalIntentV1(domainturnterminal.TurnTerminalIntentInputV1{
				PrivateFinal: record, EventManifestDigest: publication.EventManifestDigest,
				AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
			}, fixture.Sign)
			if err != nil {
				t.Fatal(err)
			}
			closure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
				TurnBindingHMAC: domainsecurity.SHA256Hex([]byte("zero-fact-restart:" + testCase.name)),
				Intents:         []domaincachetelemetry.ProviderAttemptIntentV1{}, Settlements: []domaincachetelemetry.ProviderAttemptSettlementV1{},
				TerminalReasonCode: intent.TerminalReasonCode, ClosedAt: terminaltest.FixtureTimeV1().Add(time.Second),
				AuthorityKeyID: domainsecurity.SHA256Hex(fixture.PublicKey), AuthorityPublicKey: fixture.PublicKey,
			}, fixture.Sign)
			if err != nil {
				t.Fatal(err)
			}
			closer := &memoryProviderCloserV1{trace: trace, closure: closure}
			coordinator, err := NewCoordinator(
				newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals,
				terminals, closer,
			)
			if err != nil {
				t.Fatal(err)
			}
			committed, err := coordinator.CommitV1(context.Background(), CommitInputV1{
				CompletionStore: completion, CASReader: completion, PrivateFinal: record,
			})
			if err != nil || committed.Persistence.AcceptedFinal.RecordDigest != record.AcceptedFinal.RecordDigest {
				t.Fatalf("current zero-fact terminal setup failed: result=%#v err=%v", committed, err)
			}
			writesBefore := terminalRecoveryWriteCountV1(trace.snapshot())
			publicCallsBefore, closureCallsBefore := completion.publicCalls, closer.calls
			restarted, err := NewCoordinator(
				newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer,
			)
			if err != nil {
				t.Fatal(err)
			}
			result, recoverErr := restarted.RecoverV1(context.Background(), RestartRecoveryInputV1{
				CompletionStore: completion, CASReader: completion,
				PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{record},
				Candidates:       []domainevidence.PrivateAcceptedFinalRecord{record},
			})
			if recoverErr != nil || len(result.Complete) != 1 || len(result.NonExecutableAuditOnly) != 0 ||
				result.Complete[0].Persistence.AcceptedFinal.RecordDigest != record.AcceptedFinal.RecordDigest {
				t.Fatalf("current zero-fact final did not remain terminal-complete after restart: result=%#v err=%v", result, recoverErr)
			}
			if completion.publicCalls != publicCallsBefore || closer.calls != closureCallsBefore ||
				terminalRecoveryWriteCountV1(trace.snapshot()) != writesBefore {
				t.Fatalf("terminal-complete restart mutated the existing chain: trace=%v", trace.snapshot())
			}
			if record.Envelope.OrdinaryResult != nil &&
				(privateFinals.private.Envelope.OrdinaryResult == nil ||
					privateFinals.private.Envelope.OrdinaryResult.Text != record.Envelope.OrdinaryResult.Text) {
				t.Fatalf("typed ordinary result changed during restart: stored=%#v want=%#v", privateFinals.private.Envelope.OrdinaryResult, record.Envelope.OrdinaryResult)
			}
		})
	}
}

func TestRestartRecoveryRejectsPublicCommitBeforeProviderClosure(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	intent := fixture.Intent
	terminals := &memoryTerminalStoreV1{trace: trace, intent: &intent}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context, winner: fixture.PrivateFinal.AcceptedFinal}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}); err == nil {
		t.Fatal("public commit before provider closure was recovered by backfilling authority")
	}
	if closer.calls != 0 || completion.publicCalls != 0 || terminals.disposition != nil || privateFinals.disposition != nil {
		t.Fatalf("invalid physical order was mutated: closure=%d public=%d", closer.calls, completion.publicCalls)
	}
}

func TestHistoricalCompleteTerminalChainSurvivesNewerCurrentTurnWithoutWrites(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := coordinator.CommitV1(context.Background(), CommitInputV1{
		CompletionStore: completion, CASReader: completion, PrivateFinal: fixture.PrivateFinal,
	})
	if err != nil {
		t.Fatal(err)
	}
	completion.current = newerTerminalContextV1(t, fixture.Context)
	publicCallsBefore := completion.publicCalls
	closureCallsBefore := closer.calls
	writesBefore := terminalRecoveryWriteCountV1(trace.snapshot())

	result, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Complete) != 1 || result.Complete[0].TerminalDisposition.DispositionID != committed.TerminalDisposition.DispositionID {
		t.Fatalf("historical complete chain was not preserved: %#v", result)
	}
	if completion.publicCalls != publicCallsBefore || closer.calls != closureCallsBefore ||
		terminalRecoveryWriteCountV1(trace.snapshot()) != writesBefore {
		t.Fatalf("historical complete chain was replayed with writes: public=%d closure=%d trace=%v",
			completion.publicCalls-publicCallsBefore, closer.calls-closureCallsBefore, trace.snapshot())
	}
}

func TestIncompleteHistoricalPrefixCannotResumeAfterContextAdvance(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	intent := fixture.Intent
	terminals := &memoryTerminalStoreV1{trace: trace, intent: &intent}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	completion := &memoryCompletionCASV1{
		trace: trace, context: fixture.Context, current: newerTerminalContextV1(t, fixture.Context),
	}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	})
	if err == nil || !strings.Contains(err.Error(), "incomplete historical prefix") {
		t.Fatalf("incomplete historical prefix was not rejected: %v", err)
	}
	if completion.publicCalls != 0 || closer.calls != 0 || terminals.disposition != nil || privateFinals.disposition != nil ||
		terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("incomplete historical prefix caused recovery writes: trace=%v", trace.snapshot())
	}
}

func TestRestartRecoveryRejectsLegacyWinnerWithDetachedAcceptedDisposition(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	disposition := fixture.AcceptedDisposition
	terminals := &memoryTerminalStoreV1{trace: trace}
	privateFinals := &memoryPrivateFinalStoreV1{
		trace: trace, private: fixture.PrivateFinal, disposition: &disposition,
	}
	completion := &memoryCompletionCASV1{
		trace: trace, context: fixture.Context, winner: fixture.PrivateFinal.AcceptedFinal,
	}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure}
	coordinator, err := NewCoordinator(newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals, closer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		Candidates: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}); err == nil || !strings.Contains(err.Error(), "detached later authority") {
		t.Fatalf("legacy winner with detached disposition was not rejected: %v", err)
	}
	if completion.publicCalls != 0 || closer.calls != 0 || terminals.intent != nil || terminals.disposition != nil {
		t.Fatalf("detached legacy authority caused recovery writes: trace=%v", trace.snapshot())
	}
}

func TestRestartRecoveryPrivateInventoryMustEqualDurableStore(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	coordinator, err := NewCoordinator(
		newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals,
		&memoryTerminalStoreV1{trace: trace}, &memoryProviderCloserV1{trace: trace, closure: fixture.Closure},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	if _, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion, PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{},
	}); err == nil || !strings.Contains(err.Error(), "omits or invents") {
		t.Fatalf("caller omitted a durable private final without rejection: %v", err)
	}
	if terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("private inventory mismatch caused recovery writes: %v", trace.snapshot())
	}
}

func TestRestartRecoveryRejectsOrphanIntentAndTerminalDisposition(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		terminals *memoryTerminalStoreV1
		want      string
	}{
		{name: "intent", terminals: &memoryTerminalStoreV1{trace: &coordinatorTraceV1{}, intent: &fixture.Intent}, want: "intent is orphaned"},
		{name: "terminal-disposition", terminals: &memoryTerminalStoreV1{trace: &coordinatorTraceV1{}, disposition: &fixture.Disposition}, want: "disposition is orphaned"},
	} {
		t.Run(test.name, func(t *testing.T) {
			privateFinals := &recoveryInventoryPrivateStoreV1{}
			coordinator, err := NewCoordinator(
				newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, test.terminals,
				&memoryProviderCloserV1{trace: test.terminals.trace, closure: fixture.Closure},
			)
			if err != nil {
				t.Fatal(err)
			}
			completion := &memoryCompletionCASV1{trace: test.terminals.trace, context: fixture.Context}
			_, err = coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
				CompletionStore: completion, CASReader: completion, PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("orphan %s was not rejected: %v", test.name, err)
			}
			if terminalRecoveryWriteCountV1(test.terminals.trace.snapshot()) != 0 {
				t.Fatalf("orphan %s caused recovery writes: %v", test.name, test.terminals.trace.snapshot())
			}
		})
	}
}

func TestRestartRecoveryRejectsKnownClosureWithoutTerminalIntent(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	closer := &memoryProviderCloserV1{trace: trace, closure: fixture.Closure, calls: 1}
	coordinator, err := NewCoordinator(
		newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals,
		&memoryTerminalStoreV1{trace: trace}, closer,
	)
	if err != nil {
		t.Fatal(err)
	}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context, winner: fixture.PrivateFinal.AcceptedFinal}
	if _, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
		Candidates:       []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}); err == nil || !strings.Contains(err.Error(), "detached later authority") {
		t.Fatalf("known closure without terminal intent was not rejected: %v", err)
	}
	if completion.publicCalls != 0 || terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("detached provider closure caused recovery writes: %v", trace.snapshot())
	}
}

func TestRestartRecoveryRevalidatesWholeInventoryBeforeFirstWrite(t *testing.T) {
	fixture, err := terminaltest.NewFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	trace := &coordinatorTraceV1{}
	terminals := &revalidatingTerminalStoreV1{
		memoryTerminalStoreV1: &memoryTerminalStoreV1{trace: trace}, lateIntent: fixture.Intent,
	}
	privateFinals := &memoryPrivateFinalStoreV1{trace: trace, private: fixture.PrivateFinal}
	coordinator, err := NewCoordinator(
		newTrustedInventoryAuthorityV1(fixture.PrivateKey), privateFinals, terminals,
		&memoryProviderCloserV1{trace: trace, closure: fixture.Closure},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion := &memoryCompletionCASV1{trace: trace, context: fixture.Context}
	if _, err := coordinator.RecoverV1(context.Background(), RestartRecoveryInputV1{
		CompletionStore: completion, CASReader: completion,
		PrivateInventory: []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
		Candidates:       []domainevidence.PrivateAcceptedFinalRecord{fixture.PrivateFinal},
	}); err == nil || !strings.Contains(err.Error(), "changed after planning") {
		t.Fatalf("late authority inventory mutation was not rejected: %v", err)
	}
	if completion.publicCalls != 0 || terminalRecoveryWriteCountV1(trace.snapshot()) != 0 {
		t.Fatalf("late authority inventory mutation caused recovery writes: %v", trace.snapshot())
	}
}

func newerTerminalContextV1(t *testing.T, current domainsecurity.TurnSecurityContext) domainsecurity.TurnSecurityContext {
	t.Helper()
	issuedAt, err := time.Parse(time.RFC3339Nano, current.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	next, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: current.TurnID + "-next", WorkspaceRealPath: current.WorkspaceRealPath,
		TenantID: current.TenantID, UserID: current.UserID, CaseID: current.CaseID,
		CaseBindingHash: current.CaseBindingHash, DatasetSnapshotID: current.DatasetSnapshotID,
		SourceManifestHash: current.SourceManifestHash, ContextEpoch: current.ContextEpoch + 1,
		IssuedAt: issuedAt.Add(time.Minute), PublicationPolicy: current.PublicationPolicy,
		RiskAuthorityBinding: current.RiskAuthorityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func terminalRecoveryWriteCountV1(trace []string) int {
	writes := 0
	for _, entry := range trace {
		switch entry {
		case "intent_put", "provider_close", "public_cas", "accepted_disposition_put", "terminal_disposition_put":
			writes++
		}
	}
	return writes
}
