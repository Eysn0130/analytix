package thread

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type projectionTestAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
	signCalls  atomic.Int64
}

func newProjectionTestAuthority(seed byte) *projectionTestAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &projectionTestAuthority{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (authority *projectionTestAuthority) KeyID() string { return authority.keyID }
func (authority *projectionTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *projectionTestAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	authority.signCalls.Add(1)
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *projectionTestAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("projection test authority mismatch")
	}
	return nil
}

type trustedProjectionFixture struct {
	securityContext   domainsecurity.TurnSecurityContext
	privateRecord     domainevidence.PrivateAcceptedFinalRecord
	disposition       domainevidence.AcceptedFinalDispositionRecord
	terminalAuthority gateprojection.TerminalCompleteFinalAuthorityV1
	plan              appturn.AcceptedFinalPublicationPlan
}

type projectionDeliveryReadbackStub struct {
	expected []map[string]any
	durable  bool
}

func (stub *projectionDeliveryReadbackStub) VerifyReservedTail(_ context.Context, events []map[string]any) error {
	if stub == nil || !stub.durable || !reflect.DeepEqual(stub.expected, events) {
		return errors.New("projection delivery events have no exact durable reserved tail")
	}
	return nil
}

func (stub *projectionDeliveryReadbackStub) VerifyCommittedManifest(_ context.Context, events []map[string]any) error {
	if stub == nil || !reflect.DeepEqual(stub.expected, events) {
		return errors.New("projection delivery events have no exact durable manifest")
	}
	return nil
}

type projectionCASReaderStub struct {
	observations map[string]domainevidence.AcceptedFinalCASObservationV1
	err          error
}

func (stub projectionCASReaderStub) ReadAcceptedFinalCASObservation(_ context.Context, threadID, turnID string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	if stub.err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, stub.err
	}
	observation, found := stub.observations[turnID]
	if !found || observation.ThreadID != threadID {
		return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("projection CAS observation is missing")
	}
	return observation, nil
}

func (stub projectionCASReaderStub) ReadAcceptedFinalCASObservations(_ context.Context, threadID string, turnIDs []string) (map[string]domainevidence.AcceptedFinalCASObservationV1, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	out := make(map[string]domainevidence.AcceptedFinalCASObservationV1, len(turnIDs))
	for _, turnID := range turnIDs {
		observation, found := stub.observations[turnID]
		if !found || observation.ThreadID != threadID {
			return nil, errors.New("projection CAS observation is missing")
		}
		out[turnID] = observation
	}
	return out, nil
}

func newTrustedProjectionFixture(t *testing.T, authority *projectionTestAuthority, securityContext domainsecurity.TurnSecurityContext) trustedProjectionFixture {
	t.Helper()
	now := time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: securityContext, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
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
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(securityContext, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	privateRecord, err := domainevidence.NewPrivateAcceptedFinalRecord(securityContext, envelope, rendered, head, intent, acceptedFinal)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := appturn.BuildAcceptedFinalPublicationPlan(acceptedFinal, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	turn := trustedProjectionRawTurn(securityContext, acceptedFinal, plan)
	turnProjectionSHA256, err := domainevidence.AcceptedFinalTurnProjectionSHA256V1(turn)
	if err != nil {
		t.Fatal(err)
	}
	terminalIntent, err := domainturnterminal.NewTurnTerminalIntentV1(domainturnterminal.TurnTerminalIntentInputV1{
		PrivateFinal: privateRecord, EventManifestDigest: plan.EventManifestDigest,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	providerClosure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC: domainsecurity.SHA256Hex([]byte("projection-provider-turn:" + securityContext.ContextDigest)),
		Intents:         []domaincachetelemetry.ProviderAttemptIntentV1{}, Settlements: []domaincachetelemetry.ProviderAttemptSettlementV1{},
		TerminalReasonCode: terminalIntent.TerminalReasonCode, ClosedAt: now,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainevidence.NewAcceptedFinalCASObservationV1(domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Status: "completed",
		FrozenContext: securityContext, CurrentContext: securityContext, HasWinner: true, Winner: acceptedFinal,
		ThreadFileSHA256:     domainsecurity.SHA256Hex([]byte("projection-thread:" + turnProjectionSHA256)),
		TurnProjectionSHA256: turnProjectionSHA256,
	})
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: acceptedFinal, State: domainevidence.AcceptedFinalCommitted, EventManifestDigest: plan.EventManifestDigest,
		DecidedAt: now, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, observation, domainevidence.AcceptedFinalDecisionSamePublicWinner,
		func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	terminalDisposition, err := domainturnterminal.NewTurnTerminalDispositionV1(domainturnterminal.TurnTerminalDispositionInputV1{
		Intent: terminalIntent, ProviderClosure: providerClosure, AcceptedFinalDisposition: disposition,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	terminalAuthority := gateprojection.TerminalCompleteFinalAuthorityV1{
		PrivateFinal: privateRecord, Intent: terminalIntent, ProviderClosure: providerClosure,
		PublicObservation: observation, AcceptedFinalDisposition: disposition,
		TerminalDisposition: terminalDisposition,
	}
	return trustedProjectionFixture{
		securityContext: securityContext, privateRecord: privateRecord, disposition: disposition,
		terminalAuthority: terminalAuthority, plan: plan,
	}
}

func newTrustedProjectionContext(threadID, turnID, caseID, snapshot string, epoch uint64) domainsecurity.TurnSecurityContext {
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/cases/" + caseID, CaseID: caseID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding:" + caseID)), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID(snapshot),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + snapshot)), ContextEpoch: epoch,
		IssuedAt: time.Date(2026, 7, 11, 7, 59, 0, 0, time.UTC),
	})
	if err != nil {
		panic(err)
	}
	return securityContext
}

func (fixture trustedProjectionFixture) thread() map[string]any {
	return map[string]any{
		"id": fixture.securityContext.ThreadID, "status": "idle", "workspace": fixture.securityContext.WorkspaceRealPath,
		"securityState": publicProjectionSecurityRecord(fixture.securityContext),
		"turns":         []any{trustedProjectionRawTurn(fixture.securityContext, fixture.privateRecord.AcceptedFinal, fixture.plan)},
	}
}

func (fixture trustedProjectionFixture) casReader(t *testing.T, current domainsecurity.TurnSecurityContext) projectionCASReaderStub {
	t.Helper()
	observation := fixture.terminalAuthority.PublicObservation
	observation.CurrentContext = current
	observation.ThreadFileSHA256 = domainsecurity.SHA256Hex([]byte("projection-primary:" + current.ContextDigest))
	observation.ObservationDigest = ""
	observation, err := domainevidence.NewAcceptedFinalCASObservationV1(observation)
	if err != nil {
		t.Fatal(err)
	}
	return projectionCASReaderStub{observations: map[string]domainevidence.AcceptedFinalCASObservationV1{
		fixture.securityContext.TurnID: observation,
	}}
}

func trustedProjectionRawTurn(
	securityContext domainsecurity.TurnSecurityContext,
	acceptedFinal domainevidence.AcceptedFinalRecord,
	plan appturn.AcceptedFinalPublicationPlan,
) map[string]any {
	user := map[string]any{
		"id": "user-" + securityContext.TurnID, "threadId": securityContext.ThreadID,
		"turnId": securityContext.TurnID, "kind": "user_message", "role": "user", "status": "completed", "text": "USER_REQUEST",
	}
	tool := map[string]any{
		"id": "tool-" + securityContext.TurnID, "threadId": securityContext.ThreadID,
		"turnId": securityContext.TurnID, "kind": "tool_call", "status": "completed", "toolName": "read_file",
	}
	return map[string]any{
		"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "completed", "reasoningEffort": "high",
		"securityContext": publicProjectionSecurityRecord(securityContext),
		"acceptedFinal":   domainevidence.AcceptedFinalRecordMap(acceptedFinal),
		"items":           []any{user, tool, contracts.CloneMap(plan.TurnItems[0])},
	}
}

func (fixture trustedProjectionFixture) event(slot string) map[string]any {
	for _, event := range fixture.plan.Events {
		if event.Slot == slot {
			return contracts.CloneMap(event.Draft)
		}
	}
	return nil
}

func TestTrustedCommittedFinalProjectsOnlyExactThreadAndManifestEvents(t *testing.T) {
	authority := newProjectionTestAuthority(1)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-a", "turn-a", "case-a", "snapshot-a", 3))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, fixture.casReader(t, fixture.securityContext))
	projected, err := projector.ProjectThread(fixture.thread())
	if err != nil {
		t.Fatal(err)
	}
	body := string(mustProjectionJSON(t, projected))
	if !strings.Contains(body, fixture.privateRecord.RenderedText) || !strings.Contains(body, fixture.privateRecord.AcceptedFinal.RecordDigest) {
		t.Fatalf("trusted final was not projected: %s", body)
	}
	projectedTurn := projected["turns"].([]any)[0].(map[string]any)
	if got := contracts.StringField(projectedTurn, "reasoningEffort"); got != "high" {
		t.Fatalf("trusted case turn reasoning effort = %q, want high", got)
	}
	if _, present := projectedTurn["acceptedFinal"]; present {
		t.Fatalf("ordinary projection exposed the private accepted-final record: %#v", projectedTurn["acceptedFinal"])
	}
	publicView, err := domainevidence.ParseAcceptedFinalPublicViewV3(projectedTurn["acceptedFinalView"], fixture.privateRecord)
	if err != nil || publicView.BlockerCode != "current_case_source_unavailable" || publicView.ReceiptMetadata.Count != 0 {
		t.Fatalf("trusted V3 public view did not match signed V5 authority: view=%#v err=%v", publicView, err)
	}
	viewRecord, ok := projectedTurn["acceptedFinalView"].(map[string]any)
	if !ok || len(viewRecord) != 14 {
		t.Fatalf("trusted history V3 is not the closed allowlist: %#v", projectedTurn["acceptedFinalView"])
	}
	for _, field := range []string{
		"schemaVersion", "acceptedFinalDigest", "publicationState", "variant", "terminalReason",
		"blockerCode", "coverageStatus", "checkedScopeDigest", "missingScopeCount", "claimCount",
		"claimTypes", "receiptMetadata", "noHitWording", "acceptedAt",
	} {
		if _, present := viewRecord[field]; !present {
			t.Fatalf("trusted history V3 omitted allowlisted field %q: %#v", field, viewRecord)
		}
	}
	viewBody := string(mustProjectionJSON(t, viewRecord))
	for _, forbiddenProperty := range []string{
		"publicViewDigest", "envelopeDigest", "contextDigest", "contextEpoch", "datasetSnapshotId",
		"envelopeIssuedAt", "threadId", "turnId", "renderedTextSha256", "acceptedFinal",
	} {
		if strings.Contains(viewBody, `"`+forbiddenProperty+`"`) {
			t.Fatalf("trusted history V3 exposed forbidden property %q: %s", forbiddenProperty, viewBody)
		}
	}
	for _, forbiddenValue := range []string{
		fixture.privateRecord.AcceptedFinal.ThreadID,
		fixture.privateRecord.AcceptedFinal.TurnID,
		fixture.privateRecord.AcceptedFinal.EnvelopeDigest,
		fixture.privateRecord.AcceptedFinal.RenderedTextSHA256,
		fixture.privateRecord.AcceptedFinal.PublicViewDigest,
		fixture.privateRecord.SecurityContext.ContextDigest,
		fixture.privateRecord.SecurityContext.CaseBindingHash,
		fixture.privateRecord.SecurityContext.DatasetSnapshotID,
		fixture.privateRecord.PrivateRecordDigest,
		fixture.privateRecord.StoreDigest,
	} {
		if forbiddenValue != "" && strings.Contains(viewBody, forbiddenValue) {
			t.Fatalf("trusted history V3 exposed private exact-value canary %q: %s", forbiddenValue, viewBody)
		}
	}
	projectedItem := projectedTurn["items"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(projectedTurn["acceptedFinalView"], projectedItem["acceptedFinalView"]) {
		t.Fatalf("turn/item accepted-final public views diverged: turn=%#v item=%#v", projectedTurn["acceptedFinalView"], projectedItem["acceptedFinalView"])
	}
	itemTampered := contracts.CloneMap(fixture.thread())
	tamperedTurn := itemTampered["turns"].([]any)[0].(map[string]any)
	tamperedTurn["items"].([]any)[2].(map[string]any)["id"] = "item-tampered"
	if projected, err := projector.ProjectThread(itemTampered); err == nil || projected != nil {
		t.Fatalf("accepted item metadata outside the deterministic plan was projected: %#v err=%v", projected, err)
	}
	lateToolPatch := contracts.CloneMap(fixture.thread())
	lateTurn := lateToolPatch["turns"].([]any)[0].(map[string]any)
	lateTurn["items"].([]any)[1].(map[string]any)["status"] = "failed"
	if projected, err := projector.ProjectThread(lateToolPatch); err != nil || projected == nil {
		t.Fatalf("normalized-only filtered tool state overrode strict primary CAS authority: %#v err=%v", projected, err)
	}
	if event, visible, err := projector.ProjectEvent(
		fixture.securityContext.ThreadID, lateToolPatch, fixture.event("terminal"),
	); err != nil || !visible || event == nil {
		t.Fatalf("normalized-only filtered tool state overrode strict primary SSE authority: event=%#v visible=%t err=%v", event, visible, err)
	}
	omitted := contracts.CloneMap(fixture.thread())
	omitted["turns"] = []any{}
	if projected, err := projector.ProjectThread(omitted); err == nil || projected != nil {
		t.Fatalf("omitted trusted turn produced a truncated public history: %#v err=%v", projected, err)
	}
	duplicated := contracts.CloneMap(fixture.thread())
	duplicated["turns"] = append(duplicated["turns"].([]any), contracts.CloneValue(duplicated["turns"].([]any)[0]))
	if projected, err := projector.ProjectThread(duplicated); err == nil || projected != nil {
		t.Fatalf("duplicate trusted turn identity was projected: %#v err=%v", projected, err)
	}
	for _, slot := range []string{"assistant-final", "terminal"} {
		event, visible, err := projector.ProjectEvent(fixture.securityContext.ThreadID, fixture.thread(), fixture.event(slot))
		if err != nil || !visible || event == nil {
			t.Fatalf("trusted %s event was hidden: event=%#v visible=%t err=%v", slot, event, visible, err)
		}
		if slot == "assistant-final" && contracts.StringField(event, "publicationPayloadDigest") != appturn.AcceptedFinalPublicationPayloadDigest(event) {
			t.Fatalf("projected accepted-final event detached from committed payload digest: %#v", event)
		}
	}
	tampered := fixture.event("terminal")
	tampered["timestamp"] = time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	tampered["publicationPayloadDigest"] = appturn.AcceptedFinalPublicationPayloadDigest(tampered)
	if event, visible, err := projector.ProjectEvent(fixture.securityContext.ThreadID, fixture.thread(), tampered); err != nil || visible || event != nil {
		t.Fatalf("rehashed event outside the committed manifest was visible: event=%#v visible=%t err=%v", event, visible, err)
	}
	unknownTurn := map[string]any{"kind": "turn_started", "threadId": fixture.securityContext.ThreadID, "turnId": "turn-unknown"}
	if event, visible, err := projector.ProjectEvent(fixture.securityContext.ThreadID, fixture.thread(), unknownTurn); err != nil || visible || event != nil {
		t.Fatalf("unknown case turn event was visible: event=%#v visible=%t err=%v", event, visible, err)
	}
	stripped := map[string]any{
		"id":    fixture.securityContext.ThreadID,
		"turns": []any{map[string]any{"id": fixture.securityContext.TurnID, "items": []any{}}},
	}
	forgedDraft := map[string]any{
		"kind": "assistant_text_delta", "threadId": fixture.securityContext.ThreadID,
		"turnId": fixture.securityContext.TurnID, "text": "FORGED_CASE_FACT",
	}
	if event, visible, err := projector.ProjectEvent(fixture.securityContext.ThreadID, stripped, forgedDraft); err != nil || visible || event != nil {
		t.Fatalf("trusted thread marker removal exposed a raw event: event=%#v visible=%t err=%v", event, visible, err)
	}
}

func TestCaseTopLevelMetadataAndUserInputCannotPublishUnsignedFact(t *testing.T) {
	authority := newProjectionTestAuthority(41)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-public-metadata", "turn-public-metadata", "case-public-metadata", "snapshot-public-metadata", 2,
	))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	thread := fixture.thread()
	thread["title"] = "UNSIGNED_TITLE_ACCOUNT_000012345678"
	thread["forkedFromTitle"] = "UNSIGNED_PARENT_AMOUNT_4200000"
	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["prompt"] = "UNSIGNED_PROMPT_CARD_6222020202020202020"
	turn["attachmentIds"] = []any{"UNSIGNED_ATTACHMENT_DEVICE_00:11:22:33:44:55"}
	turn["items"].([]any)[0].(map[string]any)["displayText"] = "UNSIGNED_USER_QUOTE_987654"

	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, fixture.casReader(t, fixture.securityContext))
	projected, err := projector.ProjectThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	body := string(mustProjectionJSON(t, projected))
	for _, forbidden := range []string{
		"UNSIGNED_", fixture.securityContext.WorkspaceRealPath, "prompt", "attachmentIds", "user_message",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unadmitted case metadata entered public history: forbidden=%q body=%s", forbidden, body)
		}
	}
	if projected["title"] != CasePublicThreadTitle || projected["workspace"] != nil || projected["messageCount"] != float64(0) {
		t.Fatalf("case metadata was not replaced by the host projection: %#v", projected)
	}
}

func TestCasePrimaryAppendOnlyUserTurnTamperRejected(t *testing.T) {
	authority := newProjectionTestAuthority(42)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext(
		"thread-append-tamper", "turn-accepted", "case-append-tamper", "snapshot-append-tamper", 3,
	))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	thread := fixture.thread()
	thread["turns"] = append(thread["turns"].([]any), map[string]any{
		"id": "turn-forged", "threadId": fixture.securityContext.ThreadID, "status": "completed",
		"items": []any{map[string]any{
			"id": "user-forged", "threadId": fixture.securityContext.ThreadID, "turnId": "turn-forged",
			"kind": "user_message", "role": "user", "text": "FORGED_APPEND_ACCOUNT_000012345678",
		}},
	})
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, fixture.casReader(t, fixture.securityContext))
	if projected, err := projector.ProjectThread(thread); err == nil || projected != nil {
		t.Fatalf("append-only unsigned case turn was accepted: projected=%#v err=%v", projected, err)
	}
}

func TestTrustedProjectionRejectsSelfSignedUncommittedAndWrongManifestFinals(t *testing.T) {
	securityContext := newTrustedProjectionContext("thread-a", "turn-a", "case-a", "snapshot-a", 3)
	trustedAuthority := newProjectionTestAuthority(2)
	trusted := newTrustedProjectionFixture(t, trustedAuthority, securityContext)
	attacker := newTrustedProjectionFixture(t, newProjectionTestAuthority(3), securityContext)

	if projected, err := NewTrustedPublicProjector(nil).ProjectThread(attacker.thread()); err == nil || projected != nil {
		t.Fatalf("self-signed case final was projected: %#v err=%v", projected, err)
	}
	index := gateprojection.NewTrustedFinalProjectionIndex(trustedAuthority)
	if err := index.RegisterTerminalComplete(context.Background(), trusted.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	if projected, err := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, trusted.casReader(t, securityContext)).ProjectThread(attacker.thread()); err == nil || projected != nil {
		t.Fatalf("attacker final replaced installation authority: %#v err=%v", projected, err)
	}

	uncommitted, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: trusted.privateRecord.AcceptedFinal, State: domainevidence.AcceptedFinalExplicitlyNotCommitted,
		EventManifestDigest: trusted.plan.EventManifestDigest, DecidedAt: time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC),
		AuthorityKeyID: trustedAuthority.KeyID(), AuthorityPublicKey: trustedAuthority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return trustedAuthority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	uncommittedAuthority := trusted.terminalAuthority
	uncommittedAuthority.AcceptedFinalDisposition = uncommitted
	if err := gateprojection.NewTrustedFinalProjectionIndex(trustedAuthority).RegisterTerminalComplete(context.Background(), uncommittedAuthority); err == nil {
		t.Fatal("explicitly uncommitted final entered the trusted projection index")
	}

	wrongManifest, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: trusted.privateRecord.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: domainsecurity.SHA256Hex([]byte("wrong-manifest")), DecidedAt: time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC),
		AuthorityKeyID: trustedAuthority.KeyID(), AuthorityPublicKey: trustedAuthority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return trustedAuthority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	wrongIndex := gateprojection.NewTrustedFinalProjectionIndex(trustedAuthority)
	wrongManifestAuthority := trusted.terminalAuthority
	wrongManifestAuthority.AcceptedFinalDisposition = wrongManifest
	if err := wrongIndex.RegisterTerminalComplete(context.Background(), wrongManifestAuthority); err == nil {
		t.Fatal("wrong publication manifest entered terminal-complete projection authority")
	}
}

func TestTrustedProjectionRequiresStrictPrimaryCASReader(t *testing.T) {
	authority := newProjectionTestAuthority(31)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-cas-required", "turn-cas-required", "case-cas-required", "snapshot-cas-required", 1))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	for name, projector := range map[string]*TrustedPublicProjector{
		"missing": NewTrustedPublicProjector(index),
		"failed":  NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, projectionCASReaderStub{err: errors.New("primary unavailable")}),
	} {
		t.Run(name, func(t *testing.T) {
			if projected, err := projector.ProjectThread(fixture.thread()); err == nil || projected != nil {
				t.Fatalf("indexed final bypassed strict primary CAS: projected=%#v err=%v", projected, err)
			}
			if event, visible, err := projector.ProjectEvent(fixture.securityContext.ThreadID, fixture.thread(), fixture.event("terminal")); err == nil || visible || event != nil {
				t.Fatalf("indexed SSE bypassed strict primary CAS: event=%#v visible=%t err=%v", event, visible, err)
			}
		})
	}
}

func TestTrustedProjectionRejectsCrossFileCASObservations(t *testing.T) {
	authority := newProjectionTestAuthority(32)
	first := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-cas-set", "turn-cas-a", "case-cas-set", "snapshot-cas-set", 2))
	second := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-cas-set", "turn-cas-b", "case-cas-set", "snapshot-cas-set", 2))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.SeedTerminalComplete(context.Background(), []gateprojection.TerminalCompleteFinalAuthorityV1{first.terminalAuthority, second.terminalAuthority}); err != nil {
		t.Fatal(err)
	}
	thread := first.thread()
	thread["securityState"] = publicProjectionSecurityRecord(second.securityContext)
	thread["turns"] = append(thread["turns"].([]any), trustedProjectionRawTurn(second.securityContext, second.privateRecord.AcceptedFinal, second.plan))
	firstReader := first.casReader(t, second.securityContext)
	secondReader := second.casReader(t, second.securityContext)
	observations := map[string]domainevidence.AcceptedFinalCASObservationV1{
		first.securityContext.TurnID:  firstReader.observations[first.securityContext.TurnID],
		second.securityContext.TurnID: secondReader.observations[second.securityContext.TurnID],
	}
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, projectionCASReaderStub{observations: observations})
	if projected, err := projector.ProjectThread(thread); err != nil || projected == nil {
		t.Fatalf("same-file primary CAS set was rejected: projected=%#v err=%v", projected, err)
	}
	tampered := observations[second.securityContext.TurnID]
	tampered.ThreadFileSHA256 = domainsecurity.SHA256Hex([]byte("different-primary-file"))
	tampered.ObservationDigest = ""
	var err error
	tampered, err = domainevidence.NewAcceptedFinalCASObservationV1(tampered)
	if err != nil {
		t.Fatal(err)
	}
	observations[second.securityContext.TurnID] = tampered
	projector = NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, projectionCASReaderStub{observations: observations})
	if projected, err := projector.ProjectThread(thread); err == nil || projected != nil {
		t.Fatalf("cross-file CAS observations formed a synthetic snapshot: projected=%#v err=%v", projected, err)
	}
}

func TestTrustedPublicProjectorMasksOrdinaryThreadAndSSEContent(t *testing.T) {
	const account = "6222020000000000000"
	const privatePath = "/Users/private-owner/SYNTHETIC-PII.csv"
	thread := map[string]any{
		"id": "thread-ordinary-privacy", "title": "账号 " + account, "status": "idle", "workspace": "/Users/developer/project",
		"turns": []any{map[string]any{
			"id": "turn-ordinary-privacy", "threadId": "thread-ordinary-privacy", "status": "running",
			"items": []any{map[string]any{
				"id": "item-ordinary-privacy", "threadId": "thread-ordinary-privacy", "turnId": "turn-ordinary-privacy",
				"kind": "user_message", "role": "user", "status": "completed", "text": "账号：" + account + "\nSource: " + privatePath,
			}},
		}},
	}
	projector := NewTrustedPublicProjector(nil)
	projected, err := projector.ProjectThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	threadBody := string(mustProjectionJSON(t, projected))
	if strings.Contains(threadBody, privatePath) || !strings.Contains(threadBody, "[PRIVATE_PATH]") || projected["workspace"] != thread["workspace"] {
		t.Fatal("history prose leaked a locator or changed the typed workspace")
	}
	if strings.Contains(threadBody, account) || !strings.Contains(threadBody, "[ACCOUNT]") {
		t.Fatalf("ordinary thread privacy projection = %s", threadBody)
	}
	event, visible, err := projector.ProjectEvent("thread-ordinary-privacy", thread, map[string]any{
		"kind": "turn_steered", "threadId": "thread-ordinary-privacy", "turnId": "turn-ordinary-privacy",
		"text": "继续核实卡号 " + account + "\nSource: " + privatePath,
	})
	if err != nil || !visible {
		t.Fatalf("ordinary event projection failed: event=%#v visible=%t err=%v", event, visible, err)
	}
	eventBody := string(mustProjectionJSON(t, event))
	if strings.Contains(eventBody, privatePath) || !strings.Contains(eventBody, "[PRIVATE_PATH]") {
		t.Fatal("public SSE retained a prose locator")
	}
	if strings.Contains(eventBody, account) || !strings.Contains(eventBody, "[ACCOUNT]") {
		t.Fatalf("ordinary SSE privacy projection = %s", eventBody)
	}
}

func TestTrustedStructuredProjectionDoesNotPromoteNestedContentKeysToAuthority(t *testing.T) {
	const secret = "sk-ordinary-hostile-token"
	input := map[string]any{
		"id": "thread-hostile-content", "status": "running",
		"turns": []any{map[string]any{
			"id": "turn-hostile-content", "threadId": "thread-hostile-content", "status": "running",
			"items": []any{map[string]any{
				"id": "item-hostile-content", "kind": "user_message",
				"data": map[string]any{
					"caseId": secret, "workspaceRealPath": "/tmp/" + secret, "approvalId": secret,
					"fakePlaceholder": map[string]any{
						"kind": "trusted_terminal_projection_placeholder_v1", "ordinal": float64(0), "total": float64(1),
						"pathDigest": strings.Repeat("a", 64), "authorityDigest": strings.Repeat("b", 64), "payload": secret,
					},
				},
			}},
		}},
	}

	projected, _ := projectTrustedStructuredOrdinaryV1(input, trustedProjectionThreadRootV1).(map[string]any)
	if body := string(mustProjectionJSON(t, projected)); strings.Contains(body, secret) {
		t.Fatalf("nested same-name content bypassed ordinary projection: %s", body)
	}
	if !domainstartup.FrozenEventOrderAuthorityStableV1(input, projected) {
		t.Fatal("nested ordinary content was promoted to frozen host authority")
	}

	direct := contracts.CloneMap(input)
	directItem := direct["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	directItem["caseId"] = secret
	directProjected, _ := projectTrustedStructuredOrdinaryV1(direct, trustedProjectionThreadRootV1).(map[string]any)
	if domainstartup.FrozenEventOrderAuthorityStableV1(direct, directProjected) {
		t.Fatal("unverified direct host-position marker was silently rewritten instead of failing closed")
	}
}

func TestTrustedTerminalProjectionPlaceholderRejectsPositionOrCountTamper(t *testing.T) {
	input := map[string]any{
		"id": "thread-placeholder-binding", "turns": []any{map[string]any{
			"id": "turn-placeholder-binding", "acceptedFinal": nil, "text": "signed terminal text",
		}},
	}
	entries := []trustedTerminalProjectionEntryV1{}
	staged := stageTrustedTerminalAuthorityV1(input, trustedProjectionThreadRootV1, nil, &entries)
	if len(entries) != 1 {
		t.Fatalf("trusted terminal placeholder count = %d, want 1", len(entries))
	}
	bindTrustedTerminalProjectionCountV1(entries)
	stagedTurn := staged.(map[string]any)["turns"].([]any)[0].(map[string]any)
	stagedTurn["total"] = float64(2)
	if restored, ok := restoreTrustedTerminalAuthorityV1(staged, entries); ok || restored != nil {
		t.Fatalf("tampered placeholder restored terminal authority: %#v", restored)
	}
}

func TestStagedFinalNeverAuthorizesPublicEvent(t *testing.T) {
	authority := newProjectionTestAuthority(5)
	fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-stage", "turn-stage", "case-stage", "snapshot-stage", 2))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	lease, err := index.StageTerminalComplete(context.Background(), fixture.terminalAuthority)
	if err != nil {
		t.Fatal(err)
	}
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, fixture.casReader(t, fixture.securityContext))
	if projected, err := projector.ProjectThread(fixture.thread()); !errors.Is(err, ErrPublicProjectionPending) || projected != nil {
		t.Fatalf("staged final became snapshot-visible before event durability: %#v err=%v", projected, err)
	}
	attacker := newTrustedProjectionFixture(t, newProjectionTestAuthority(33), fixture.securityContext)
	if projected, err := projector.ProjectThread(attacker.thread()); err == nil || errors.Is(err, ErrPublicProjectionPending) || projected != nil {
		t.Fatalf("staged lease masked a mismatched normalized final: projected=%#v err=%v", projected, err)
	}
	if event, visible, _ := projector.ProjectEvent(fixture.securityContext.ThreadID, fixture.thread(), fixture.event("assistant-final")); visible || event != nil {
		t.Fatalf("staged authority became public event authority: event=%#v visible=%t", event, visible)
	}
	lease.Discard()
	if event, visible, err := projector.ProjectEvent(fixture.securityContext.ThreadID, fixture.thread(), fixture.event("assistant-final")); visible || event != nil {
		t.Fatalf("discarded failed publication remained event-visible: event=%#v visible=%t err=%v", event, visible, err)
	}
	lease, err = index.StageTerminalComplete(context.Background(), fixture.terminalAuthority)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Activate(); err != nil {
		t.Fatal(err)
	}
	if projected, err := projector.ProjectThread(fixture.thread()); err != nil || projected == nil {
		t.Fatalf("durable final did not become snapshot-visible: %#v err=%v", projected, err)
	}
}

func TestInstallationSignedFinalIsRetryableBeforeProjectionLease(t *testing.T) {
	authority := newProjectionTestAuthority(31)
	securityContext := newTrustedProjectionContext("thread-commit-stage-gap", "turn-commit-stage-gap", "case-commit-stage-gap", "snapshot-commit-stage-gap", 2)
	trusted := newTrustedProjectionFixture(t, authority, securityContext)
	attacker := newTrustedProjectionFixture(t, newProjectionTestAuthority(32), securityContext)
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)

	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, trusted.casReader(t, securityContext))
	if projected, err := projector.ProjectThread(trusted.thread()); !errors.Is(err, ErrPublicProjectionPending) || projected != nil {
		t.Fatalf("installation-signed committed final was not classified as pending before lease staging: projected=%#v err=%v", projected, err)
	}
	if projected, err := projector.ProjectThread(attacker.thread()); err == nil || errors.Is(err, ErrPublicProjectionPending) || projected != nil {
		t.Fatalf("untrusted final became a retryable host transition: projected=%#v err=%v", projected, err)
	}
}

func TestSealCannotIssueBeforeExactDurableReadback(t *testing.T) {
	authority := newProjectionTestAuthority(19)
	fixture := newTrustedProjectionFixture(
		t,
		authority,
		newTrustedProjectionContext("thread-seal-readback", "turn-seal-readback", "case-seal-readback", "snapshot-seal-readback", 2),
	)
	events := make([]map[string]any, 0, len(fixture.plan.Events))
	for index, planned := range fixture.plan.Events {
		event := contracts.CloneMap(planned.Draft)
		event["seq"] = float64(index + 1)
		events = append(events, event)
	}
	if err := domainevent.ValidateAcceptedFinalDeliveryEventsV2(events); err != nil {
		t.Fatal(err)
	}
	readback := &projectionDeliveryReadbackStub{expected: events}
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, readback)
	lease, err := index.StageTerminalComplete(context.Background(), fixture.terminalAuthority)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Discard()

	signCallsBefore := authority.signCalls.Load()
	if seal, err := lease.SealAcceptedFinalDelivery(context.Background(), events); err == nil || seal.AuthoritySignature != "" {
		t.Fatalf("prospective events were sealed without durable readback: seal=%#v err=%v", seal, err)
	}
	if got := authority.signCalls.Load(); got != signCallsBefore {
		t.Fatalf("durable readback rejection happened after signing: before=%d after=%d", signCallsBefore, got)
	}

	readback.durable = true
	seal, err := lease.SealAcceptedFinalDelivery(context.Background(), events)
	if err != nil || seal.AuthoritySignature == "" {
		t.Fatalf("exact durable reserved tail was not sealed: seal=%#v err=%v", seal, err)
	}
	if got := authority.signCalls.Load(); got != signCallsBefore+1 {
		t.Fatalf("accepted delivery sign calls = %d, want %d", got, signCallsBefore+1)
	}
}

func TestTrustedProjectionStagedLeaseCannotBeDiscardedBySecondCaller(t *testing.T) {
	authority := newProjectionTestAuthority(6)
	fixture := newTrustedProjectionFixture(
		t, authority, newTrustedProjectionContext("thread-stage-owner", "turn-stage-owner", "case-stage-owner", "snapshot-stage-owner", 2),
	)
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	owner, err := index.StageTerminalComplete(context.Background(), fixture.terminalAuthority)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Discard()
	if contender, err := index.StageTerminalComplete(context.Background(), fixture.terminalAuthority); err == nil || contender != nil {
		t.Fatalf("second caller acquired an existing staged projection: lease=%#v err=%v", contender, err)
	}
	if err := owner.Activate(); err != nil {
		t.Fatalf("original staged owner could not activate its projection: %v", err)
	}
	if record, found := index.Resolve(fixture.securityContext.ThreadID, fixture.securityContext.TurnID); !found ||
		record.AcceptedFinal.RecordDigest != fixture.privateRecord.AcceptedFinal.RecordDigest {
		t.Fatalf("original owner lost its committed projection: record=%#v found=%v", record, found)
	}
}

func TestTrustedProjectionHidesCommittedFinalFromStaleCaseEpoch(t *testing.T) {
	authority := newProjectionTestAuthority(4)
	old := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-a", "turn-a", "case-a", "snapshot-a", 3))
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.SeedTerminalComplete(context.Background(), []gateprojection.TerminalCompleteFinalAuthorityV1{old.terminalAuthority}); err != nil {
		t.Fatal(err)
	}
	current := newTrustedProjectionContext("thread-a", "turn-b", "case-b", "snapshot-b", 4)
	thread := old.thread()
	thread["securityState"] = publicProjectionSecurityRecord(current)
	thread["turns"] = append(thread["turns"].([]any), map[string]any{
		"id": current.TurnID, "threadId": current.ThreadID, "status": "running",
		"securityContext": publicProjectionSecurityRecord(current), "items": []any{map[string]any{
			"id": "user-b", "threadId": current.ThreadID, "turnId": current.TurnID,
			"kind": "user_message", "role": "user", "text": "USER_CASE_B",
		}},
	})
	projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, old.casReader(t, current))
	projected, err := projector.ProjectThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	body := string(mustProjectionJSON(t, projected))
	if strings.Contains(body, old.privateRecord.RenderedText) || strings.Contains(body, old.privateRecord.AcceptedFinal.RecordDigest) {
		t.Fatalf("stale case epoch final crossed into the current case: %s", body)
	}
	if strings.Contains(body, "USER_CASE_B") || strings.Contains(body, "USER_REQUEST") {
		t.Fatalf("unadmitted user history entered the case projection: %s", body)
	}
	if event, visible, err := projector.ProjectEvent(old.securityContext.ThreadID, thread, old.event("assistant-final")); err != nil || visible || event != nil {
		t.Fatalf("stale case epoch event was replayed: event=%#v visible=%t err=%v", event, visible, err)
	}
	movedWorkspace := old.thread()
	movedWorkspace["workspace"] = "/cases/other"
	oldProjector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, old.casReader(t, old.securityContext))
	moved, err := oldProjector.ProjectThread(movedWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(mustProjectionJSON(t, moved)); strings.Contains(body, old.privateRecord.RenderedText) || strings.Contains(body, old.privateRecord.AcceptedFinal.RecordDigest) {
		t.Fatalf("case final crossed a top-level workspace mutation: %s", body)
	}
	if event, visible, err := oldProjector.ProjectEvent(old.securityContext.ThreadID, movedWorkspace, old.event("terminal")); err != nil || visible || event != nil {
		t.Fatalf("case terminal crossed a top-level workspace mutation: event=%#v visible=%t err=%v", event, visible, err)
	}

	resolved, _, ok := index.ResolveCommitted(old.securityContext.ThreadID, old.securityContext.TurnID)
	if !ok {
		t.Fatal("seeded projection record was not resolvable")
	}
	resolved.RenderedText = "mutated"
	again, ok := index.Resolve(old.securityContext.ThreadID, old.securityContext.TurnID)
	if !ok || again.RenderedText != old.privateRecord.RenderedText {
		t.Fatal("projection index returned mutable private authority")
	}
	invalidTerminalAuthority := old.terminalAuthority
	invalidTerminalAuthority.TerminalDisposition = domainturnterminal.TurnTerminalDispositionV1{}
	if err := index.SeedTerminalComplete(context.Background(), []gateprojection.TerminalCompleteFinalAuthorityV1{invalidTerminalAuthority}); err == nil {
		t.Fatal("seed without terminal-complete authority succeeded")
	}
	if _, ok := index.Resolve(old.securityContext.ThreadID, old.securityContext.TurnID); !ok {
		t.Fatal("failed seed erased the previous trusted index atomically")
	}
}

func TestCasePublicTurnReasoningEffortUsesClosedProjection(t *testing.T) {
	t.Run("accepted-final", func(t *testing.T) {
		authority := newProjectionTestAuthority(41)
		fixture := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-effort", "turn-effort", "case-effort", "snapshot-effort", 3))
		index := gateprojection.NewTrustedFinalProjectionIndex(authority)
		if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
			t.Fatal(err)
		}
		projector := NewTrustedPublicProjectorWithPrimaryCAS(index, nil, nil, fixture.casReader(t, fixture.securityContext))
		project := func(value any) (map[string]any, error) {
			thread := fixture.thread()
			turn := thread["turns"].([]any)[0].(map[string]any)
			switch current := value.(type) {
			case nil:
				delete(turn, "reasoningEffort")
			default:
				turn["reasoningEffort"] = current
			}
			return projector.ProjectThread(thread)
		}
		projected, err := project("high")
		if err != nil {
			t.Fatal(err)
		}
		projectedTurn := projected["turns"].([]any)[0].(map[string]any)
		if projectedTurn["reasoningEffort"] != "high" {
			t.Fatalf("accepted-final reasoning effort = %#v, want high", projectedTurn["reasoningEffort"])
		}
		for _, test := range []struct {
			name       string
			value      any
			wantAbsent bool
			wantError  bool
			sentinel   string
		}{
			{name: "missing", wantAbsent: true},
			{name: "empty", value: "", wantAbsent: true},
			{name: "invalid-sentinel", value: "CASE_REASONING_EFFORT_SENTINEL", wantError: true, sentinel: "CASE_REASONING_EFFORT_SENTINEL"},
			{name: "non-string", value: []any{"CASE_REASONING_EFFORT_NON_STRING"}, wantError: true, sentinel: "CASE_REASONING_EFFORT_NON_STRING"},
		} {
			t.Run(test.name, func(t *testing.T) {
				projected, err := project(test.value)
				if test.wantError {
					if err == nil || projected != nil {
						t.Fatalf("invalid reasoning effort was admitted: projected=%#v err=%v", projected, err)
					}
					if strings.Contains(err.Error(), test.sentinel) {
						t.Fatalf("reasoning effort sentinel was reflected in error: %v", err)
					}
					return
				}
				if err != nil || projected == nil {
					t.Fatalf("valid empty reasoning effort failed: projected=%#v err=%v", projected, err)
				}
				turn := projected["turns"].([]any)[0].(map[string]any)
				if _, present := turn["reasoningEffort"]; present != !test.wantAbsent {
					t.Fatalf("empty/missing reasoning effort presence=%t, want=%t: %#v", present, !test.wantAbsent, turn)
				}
			})
		}
	})

	t.Run("general-terminal", func(t *testing.T) {
		thread, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
		turn := thread["turns"].([]any)[0].(map[string]any)
		turn["reasoningEffort"] = "high"
		authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(thread)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := projectCaseHistoryGeneralTerminalV1(
			thread["id"].(string), turn, authorities[turn["id"].(string)],
		)
		if err != nil {
			t.Fatal(err)
		}
		if projected["reasoningEffort"] != "high" {
			t.Fatalf("general-terminal reasoning effort = %#v, want high", projected["reasoningEffort"])
		}
	})

	t.Run("compaction-retention", func(t *testing.T) {
		securityContext := newTrustedProjectionContext("thread-effort-compaction", "turn-effort-compaction", "case-effort-compaction", "snapshot-effort-compaction", 4)
		turn := map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID, "status": "completed",
			"reasoningEffort": "high", "caseHistoryProjection": "authority_only_v1",
			"securityContext": publicProjectionSecurityRecord(securityContext), "items": []any{},
		}
		caseThreads := caseCompactionProjectionAuthorityStubV1{contexts: map[string]domainsecurity.TurnSecurityContext{
			securityContext.ContextDigest: securityContext,
		}}
		projected, retained, err := projectCaseCompactionRetentionTurnV1(securityContext.ThreadID, turn, caseThreads)
		if err != nil || !retained {
			t.Fatalf("compaction retention projection failed: projected=%#v retained=%t err=%v", projected, retained, err)
		}
		if projected["reasoningEffort"] != "high" {
			t.Fatalf("compaction-retention reasoning effort = %#v, want high", projected["reasoningEffort"])
		}
	})
}

func mustProjectionJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRetainedSnapshotPublicProjectionRequiresExactOperationAdmission(t *testing.T) {
	authority := newProjectionTestAuthority(4)
	old := newTrustedProjectionFixture(t, authority, newTrustedProjectionContext("thread-retained", "turn-old", "case-a", "snapshot-a", 3))
	current := newTrustedProjectionContext("thread-retained", "turn-current", "case-a", "snapshot-b", 4)
	if current.PublicationPolicy == old.securityContext.PublicationPolicy {
		t.Fatal("fixture must preserve distinct original and current publication policies")
	}
	index := gateprojection.NewTrustedFinalProjectionIndex(authority)
	if err := index.SeedTerminalComplete(context.Background(), []gateprojection.TerminalCompleteFinalAuthorityV1{old.terminalAuthority}); err != nil {
		t.Fatal(err)
	}
	thread := old.thread()
	thread["securityState"] = publicProjectionSecurityRecord(current)
	thread["turns"] = append(thread["turns"].([]any), map[string]any{"id": current.TurnID, "threadId": current.ThreadID, "status": "running", "securityContext": publicProjectionSecurityRecord(current), "items": []any{}})
	// An input marker is untrusted and must not admit an older snapshot.
	thread["turns"].([]any)[0].(map[string]any)["factHistoryState"] = "retained_snapshot"
	hidden, err := projectPublicThreadWithAuthority(thread, index, nil, nil, old.casReader(t, current))
	if err != nil {
		t.Fatal(err)
	}
	first := hidden["turns"].([]any)[0].(map[string]any)
	if first["acceptedFinalView"] != nil || first["factHistoryState"] != nil {
		t.Fatal("unadmitted old history was exposed")
	}
	shown, err := projectPublicThreadWithRetainedFactsV1(thread, index, nil, nil, old.casReader(t, current), map[string]bool{old.securityContext.TurnID: true})
	if err != nil {
		t.Fatal(err)
	}
	first = shown["turns"].([]any)[0].(map[string]any)
	view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(first["acceptedFinalView"])
	if err != nil || view.AcceptedFinalDigest != old.privateRecord.AcceptedFinal.RecordDigest || first["factHistoryState"] != "retained_snapshot" {
		t.Fatal("authorized retained view lost its original identity or historical label")
	}
	for _, mutate := range []func(*domainsecurity.TurnSecurityContext){
		func(c *domainsecurity.TurnSecurityContext) { c.CaseID = "case-b" },
		func(c *domainsecurity.TurnSecurityContext) { c.UserID = "other-user" },
		func(c *domainsecurity.TurnSecurityContext) { c.WorkspaceRealPath = "/different" },
		func(c *domainsecurity.TurnSecurityContext) { c.ContextEpoch = 2 },
		func(c *domainsecurity.TurnSecurityContext) { c.CaseBindingHash = strings.Repeat("b", 64) },
	} {
		changed := current
		mutate(&changed)
		if sameRetainedFactScopeV1(changed, old.securityContext) {
			t.Fatal("retained display crossed current authority scope")
		}
	}
}
