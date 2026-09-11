package turn

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type acceptedFinalStoreStub struct {
	items   []map[string]any
	fields  map[string]any
	events  []map[string]any
	changed bool
	status  string
	finish  string
}

func (stub *acceptedFinalStoreStub) FinishTurnIfActiveWithItemsAndFields(_, _ string, status string, items []map[string]any, fields map[string]any) (bool, string, error) {
	stub.items = items
	stub.fields = fields
	stub.finish = status
	return stub.changed, stub.status, nil
}

func (stub *acceptedFinalStoreStub) FinishTurnIfActiveWithAcceptedFinalAuthority(
	_, _ string,
	status string,
	items []map[string]any,
	fields map[string]any,
	_ domainevidence.PrivateAcceptedFinalRecord,
	_ FactFinalMutationAuthority,
) (bool, string, error) {
	return stub.FinishTurnIfActiveWithItemsAndFields("", "", status, items, fields)
}

func TestEveryAcceptedFinalTerminalStatusPublishesOnlyAfterAtomicCommit(t *testing.T) {
	context := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	for _, test := range []struct {
		status string
		reason string
		kind   string
		items  int
		events int
	}{
		{status: "completed", reason: "success", kind: "turn_completed", items: 1, events: 3},
		{status: "failed", reason: "provider_failure", kind: "turn_failed", items: 2, events: 4},
		{status: "aborted", reason: "cancel", kind: "turn_aborted", items: 2, events: 4},
	} {
		t.Run(test.status, func(t *testing.T) {
			envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
				Variant: domainevidence.NeedsEvidenceAnswer, Context: context, TerminalReason: test.reason,
				MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: time.Unix(2, 0),
			})
			if err != nil {
				t.Fatal(err)
			}
			store := &acceptedFinalStoreStub{changed: true, status: test.status}
			privateFinal := privateAcceptedFinalForTest(t, context, envelope, time.Unix(2, 0))
			acceptedFinal := privateFinal.AcceptedFinal
			intent := acceptedFinalIntentForTest(t, envelope.TerminalReason, time.Unix(2, 0))
			result, err := PersistAcceptedFinalTerminal(PersistAcceptedFinalInput{
				Store: store, ThreadID: context.ThreadID, TurnID: context.TurnID, RenderedText: domainevidence.CaseUnverifiedText,
				AcceptedFinal: acceptedFinal, PublicationIntent: intent, PrivateFinal: privateFinal,
			})
			if err != nil || store.finish != test.status || len(store.items) != test.items || len(store.events) != 0 || len(result.Publication.Events) != test.events {
				t.Fatalf("terminal persistence mismatch: finish=%q items=%#v events=%#v publication=%#v err=%v", store.finish, store.items, store.events, result.Publication.Events, err)
			}
			terminal := result.Publication.Events[len(result.Publication.Events)-1].Draft
			if terminal["kind"] != test.kind || terminal["acceptedFinalDigest"] == "" {
				t.Fatalf("accepted terminal event mismatch: %#v", terminal)
			}
		})
	}
}

func (stub *acceptedFinalStoreStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	stub.events = append(stub.events, event)
	return event, nil, nil
}

func TestPersistAcceptedFinalCompletionCommitsBeforePublishingEvents(t *testing.T) {
	context := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: context, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &acceptedFinalStoreStub{changed: true, status: "completed"}
	privateFinal := privateAcceptedFinalForTest(t, context, envelope, time.Unix(2, 0))
	acceptedFinal := privateFinal.AcceptedFinal
	intent := acceptedFinalIntentForTest(t, envelope.TerminalReason, time.Unix(2, 0))
	result, err := PersistAcceptedFinalTerminal(PersistAcceptedFinalInput{
		Store: store, ThreadID: context.ThreadID, TurnID: context.TurnID, RenderedText: domainevidence.CaseUnverifiedText,
		AcceptedFinal: acceptedFinal, PublicationIntent: intent, PrivateFinal: privateFinal,
	})
	if err != nil || !result.Changed || len(store.items) != 1 || store.fields["acceptedFinal"] == nil {
		t.Fatalf("accepted final persistence mismatch: result=%#v items=%#v fields=%#v err=%v", result, store.items, store.fields, err)
	}
	publicationEvents := []map[string]any{}
	for _, event := range result.Publication.Events {
		publicationEvents = append(publicationEvents, event.Draft)
	}
	if kinds := eventKinds(publicationEvents); len(kinds) != 3 || kinds[0] != "item_completed" || kinds[1] != "usage" || kinds[2] != "turn_completed" || len(store.events) != 0 {
		t.Fatalf("accepted final event order mismatch: %#v", kinds)
	}
}

func TestPersistAcceptedFinalCompletionRejectsFabricatedRenderedText(t *testing.T) {
	context := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: context, TerminalReason: "success",
		MissingScope: []string{"facts"}, AcquisitionSteps: []string{"collect"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &acceptedFinalStoreStub{changed: true, status: "completed"}
	privateFinal := privateAcceptedFinalForTest(t, context, envelope, time.Unix(2, 0))
	acceptedFinal := privateFinal.AcceptedFinal
	intent := acceptedFinalIntentForTest(t, envelope.TerminalReason, time.Unix(2, 0))
	_, err = PersistAcceptedFinalTerminal(PersistAcceptedFinalInput{
		Store: store, ThreadID: context.ThreadID, TurnID: context.TurnID, AcceptedFinal: acceptedFinal,
		RenderedText: "账户 6222020000000000 金额 4200000 元", PublicationIntent: intent, PrivateFinal: privateFinal,
	})
	if err == nil || len(store.items) != 0 || len(store.events) != 0 {
		t.Fatalf("fabricated text was stamped as accepted final: items=%#v events=%#v err=%v", store.items, store.events, err)
	}
}

func TestTerminalCASReplayRequiresExactAuthorizedWinner(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-replay", "turn-replay", 2)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success",
		MissingScope: []string{"facts"}, AcquisitionSteps: []string{"collect"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	proposed := privateAcceptedFinalForTest(t, securityContext, envelope, time.Unix(3, 0))
	existing := privateAcceptedFinalForTest(t, securityContext, envelope, time.Unix(4, 0))
	plan, err := BuildAcceptedFinalPublicationPlan(
		proposed.AcceptedFinal, proposed.RenderedText, proposed.PublicationIntent,
	)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := turnSecurityContextRecord(securityContext)
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": contextRecord,
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "completed", "securityContext": contextRecord,
			"acceptedFinal": domainevidence.AcceptedFinalRecordMap(existing.AcceptedFinal),
		}},
	}
	if prepared, err := PrepareTerminalCASMutation(PrepareTerminalCASInput{
		Thread: thread, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Status: "completed",
		AppendItems: plan.TurnItems, Fields: plan.TurnFields, PrivateFinal: proposed, Now: time.Unix(5, 0),
	}); err == nil || prepared.Applied {
		t.Fatalf("different exact contender received idempotent terminal success: prepared=%#v err=%v", prepared, err)
	}
	if prepared, err := PrepareTerminalCASMutation(PrepareTerminalCASInput{
		Thread: thread, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Status: "completed",
		Now: time.Unix(5, 0),
	}); err == nil || prepared.Applied {
		t.Fatalf("generic replay claimed an existing accepted-final winner: prepared=%#v err=%v", prepared, err)
	}
}

func TestSourceEpochMismatchBlocksPublish(t *testing.T) {
	oldContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	newContext := acceptedFinalTestContext(t, "thread-a", "turn-b", 3)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: oldContext, TerminalReason: "success",
		MissingScope: []string{"facts"}, AcquisitionSteps: []string{"collect"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := signedAcceptedFinalForTest(t, oldContext, envelope, time.Unix(2, 0))
	item := BuildAssistantTextItem(AssistantTextItemInput{ThreadID: oldContext.ThreadID, TurnID: oldContext.TurnID, Text: domainevidence.CaseUnverifiedText})
	item["acceptedFinal"] = domainevidence.AcceptedFinalRecordMap(record)
	thread := map[string]any{
		"id": oldContext.ThreadID, "securityState": turnSecurityContextRecord(newContext),
		"turns": []any{map[string]any{"id": oldContext.TurnID, "securityContext": turnSecurityContextRecord(oldContext)}},
	}
	if err := ValidateAcceptedFinalTerminalUpdate(thread, oldContext.TurnID, "completed", []map[string]any{item}, map[string]any{"acceptedFinal": domainevidence.AcceptedFinalRecordMap(record)}); err == nil {
		t.Fatal("an old-epoch accepted final passed the atomic store check")
	}
}

func TestCaseBoundEveryTerminalStatusRequiresAcceptedFinal(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": turnSecurityContextRecord(securityContext),
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "status": "running", "securityContext": turnSecurityContextRecord(securityContext),
		}},
	}
	for _, status := range []string{"completed", "failed", "aborted"} {
		if err := ValidateAcceptedFinalTerminalUpdate(thread, securityContext.TurnID, status, nil, nil); err == nil {
			t.Fatalf("case-bound %s terminal update bypassed accepted final", status)
		}
	}
}

func TestAuditOnlyV1CaseContextCannotEnterAcceptedFinalAtomicUpdate(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1", TurnID: "turn-v1", WorkspaceRealPath: "/workspace", CaseID: "case-v1",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-v1-binding")), DatasetSnapshotID: "legacy-snapshot-v1",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("legacy-source")), ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	record := turnSecurityContextRecord(legacy)
	thread := map[string]any{
		"id": legacy.ThreadID, "securityState": record,
		"turns": []any{map[string]any{"id": legacy.TurnID, "status": "running", "securityContext": record}},
	}
	if err := ValidateAcceptedFinalTerminalUpdate(thread, legacy.TurnID, "completed", nil, nil); err == nil {
		t.Fatal("audit-only V1 case context entered the accepted-final atomic publication path")
	}
}

func TestBoundaryOnlyV2CannotUseOrdinaryTerminalUpdate(t *testing.T) {
	boundary := newTurnBoundaryOnlyContextV2(t, "thread-boundary", "turn-boundary", "/workspace", 2, time.Now().UTC())
	record := turnSecurityContextRecord(boundary)
	thread := map[string]any{
		"id": boundary.ThreadID, "securityState": record,
		"turns": []any{map[string]any{"id": boundary.TurnID, "status": "running", "securityContext": record}},
	}
	for _, status := range []string{"completed", "failed", "aborted"} {
		if err := ValidateAcceptedFinalTerminalUpdate(thread, boundary.TurnID, status, nil, nil); err == nil {
			t.Fatalf("boundary-only V2 used ordinary %s terminal persistence", status)
		}
	}
}

func TestCaseBoundAcceptedFinalRejectsRawSiblingAssistant(t *testing.T) {
	securityContext := acceptedFinalTestContext(t, "thread-a", "turn-a", 2)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	record := signedAcceptedFinalForTest(t, securityContext, envelope, time.Unix(2, 0))
	accepted := BuildAssistantTextItem(AssistantTextItemInput{ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Text: domainevidence.CaseUnverifiedText})
	accepted["acceptedFinal"] = domainevidence.AcceptedFinalRecordMap(record)
	raw := BuildAssistantTextItem(AssistantTextItemInput{ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ItemID: "item-raw", Text: "金额 420 万元"})
	thread := map[string]any{
		"id": securityContext.ThreadID, "securityState": turnSecurityContextRecord(securityContext),
		"turns": []any{map[string]any{"id": securityContext.TurnID, "securityContext": turnSecurityContextRecord(securityContext)}},
	}
	if err := ValidateAcceptedFinalTerminalUpdate(thread, securityContext.TurnID, "completed", []map[string]any{accepted, raw}, map[string]any{"acceptedFinal": domainevidence.AcceptedFinalRecordMap(record)}); err == nil {
		t.Fatal("raw assistant text was committed beside an accepted case final")
	}
}

func TestGeneralTerminalCASBindingRejectsStaleCurrentContext(t *testing.T) {
	frozen := generalTerminalTestContext(t, "thread-general", "turn-general", 1)
	current := generalTerminalTestContext(t, "thread-general", "turn-general", 2)
	binding, err := domainturnterminal.NewGeneralTerminalCASBindingV1(frozen, "done")
	if err != nil {
		t.Fatal(err)
	}
	item := BuildAssistantTextItem(AssistantTextItemInput{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Text: "done"})
	item["generalTerminalCASBinding"] = domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	thread := map[string]any{
		"id": frozen.ThreadID, "securityState": turnSecurityContextRecord(current),
		"turns": []any{map[string]any{"id": frozen.TurnID, "securityContext": turnSecurityContextRecord(frozen)}},
	}
	if err := ValidateAcceptedFinalTerminalUpdate(thread, frozen.TurnID, "completed", []map[string]any{item}, map[string]any{
		"generalTerminalCASBinding": domainturnterminal.GeneralTerminalCASBindingV1Map(binding),
	}); err == nil {
		t.Fatal("stale general provider text passed the atomic context check")
	}
}

func TestGeneralAssistantTerminalRequiresExactCASBinding(t *testing.T) {
	thread, commit, item, binding := pendingGeneralTerminalPublicationFixture(t, GeneralProviderFinalQuarantinedText)
	if err := ValidateAcceptedFinalTerminalUpdate(thread, commit.TurnID, "completed", []map[string]any{item}, nil); err == nil {
		t.Fatal("general assistant text without a CAS binding was accepted")
	}
	record := domainturnterminal.GeneralTerminalCASBindingV1Map(binding)
	if err := ValidateAcceptedFinalTerminalUpdate(thread, commit.TurnID, "completed", []map[string]any{item}, map[string]any{
		"generalTerminalCASBinding": record,
	}); err == nil {
		t.Fatal("general terminal CAS binding without its outbox was accepted")
	}
	fields := map[string]any{
		"generalTerminalCASBinding":  record,
		"generalTerminalPublication": domainturnterminal.GeneralTerminalPublicationCommitV1Map(commit),
	}
	if err := ValidateAcceptedFinalTerminalUpdate(thread, commit.TurnID, "completed", []map[string]any{item}, fields); err != nil {
		t.Fatalf("exact general terminal CAS binding and outbox were rejected: %v", err)
	}
	item["text"] = "different"
	if err := ValidateAcceptedFinalTerminalUpdate(thread, commit.TurnID, "completed", []map[string]any{item}, fields); err == nil {
		t.Fatal("general terminal CAS binding accepted different text")
	}
}

func acceptedFinalTestContext(t *testing.T, threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	return newTurnCaseExecutionContextV2(t, threadID, turnID, "/workspace", epoch, time.Unix(1, 0))
}

func generalTerminalTestContext(t *testing.T, threadID, turnID string, epoch uint64) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace", ContextEpoch: epoch, IssuedAt: time.Unix(int64(epoch), 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func signedAcceptedFinalForTest(t *testing.T, context domainsecurity.TurnSecurityContext, envelope domainevidence.FinalAnswerEnvelope, acceptedAt time.Time) domainevidence.AcceptedFinalRecord {
	t.Helper()
	registry, err := domainevidence.NewEvidenceReceiptRegistry(context)
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
	intent := acceptedFinalIntentForTest(t, envelope.TerminalReason, acceptedAt)
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(context, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: context, Envelope: envelope, RenderedText: rendered, RegistryHead: head, PrivateRecordDigest: privateDigest,
		AcceptedAt: acceptedAt, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func privateAcceptedFinalForTest(
	t *testing.T,
	context domainsecurity.TurnSecurityContext,
	envelope domainevidence.FinalAnswerEnvelope,
	acceptedAt time.Time,
) domainevidence.PrivateAcceptedFinalRecord {
	t.Helper()
	registry, err := domainevidence.NewEvidenceReceiptRegistry(context)
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
	intent := acceptedFinalIntentForTest(t, envelope.TerminalReason, acceptedAt)
	acceptedFinal := signedAcceptedFinalForTest(t, context, envelope, acceptedAt)
	privateFinal, err := domainevidence.NewPrivateAcceptedFinalRecord(
		context, envelope, rendered, head, intent, acceptedFinal,
	)
	if err != nil {
		t.Fatal(err)
	}
	return privateFinal
}

func acceptedFinalIntentForTest(t *testing.T, terminalReason string, acceptedAt time.Time) domainevidence.TerminalPublicationIntent {
	t.Helper()
	status, ok := domainevidence.FinalAnswerTerminalStatus(terminalReason)
	if !ok {
		t.Fatalf("unknown terminal reason %q", terminalReason)
	}
	projection, ok := domainterminal.FailureProjectionV1(terminalReason)
	if !ok || projection.Status != status {
		t.Fatalf("terminal reason %q has no closed public projection", terminalReason)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: acceptedAt.UTC().Format(time.RFC3339Nano), TerminalStatus: status,
		TerminalCode: projection.Code, TerminalMessage: projection.Message,
		TerminalSeverity: projection.Severity, Discard: true, Cancelled: true, CancelledPendingGates: 2,
	}, terminalReason)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func turnSecurityContextRecord(context domainsecurity.TurnSecurityContext) map[string]any {
	body, _ := json.Marshal(context)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
