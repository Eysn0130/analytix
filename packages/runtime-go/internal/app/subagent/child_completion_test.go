package subagent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type childCompletionTestAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newChildCompletionTestAuthority(seed byte) *childCompletionTestAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return &childCompletionTestAuthority{privateKey: privateKey, publicKey: publicKey, keyID: domainsecurity.SHA256Hex(publicKey)}
}

func (authority *childCompletionTestAuthority) KeyID() string { return authority.keyID }
func (authority *childCompletionTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *childCompletionTestAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *childCompletionTestAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("child completion test authority mismatch")
	}
	return nil
}

type childCompletionFinalStub struct {
	private     domainevidence.PrivateAcceptedFinalRecord
	disposition domainevidence.AcceptedFinalDispositionRecord
	ok          bool
}

func (stub childCompletionFinalStub) ResolveCommitted(threadID string, turnID string) (domainevidence.PrivateAcceptedFinalRecord, domainevidence.AcceptedFinalDispositionRecord, bool) {
	if !stub.ok || stub.private.SecurityContext.ThreadID != threadID || stub.private.SecurityContext.TurnID != turnID {
		return domainevidence.PrivateAcceptedFinalRecord{}, domainevidence.AcceptedFinalDispositionRecord{}, false
	}
	return stub.private, stub.disposition, true
}

type childCompletionFixture struct {
	authority    *ChildCompletionAuthority
	host         *childCompletionTestAuthority
	parent       domainsecurity.TurnSecurityContext
	child        domainsecurity.TurnSecurityContext
	grant        domainsecurity.ExecutionGrant
	binding      *domainjob.SecurityBinding
	record       domainjob.Record
	privateFinal domainevidence.PrivateAcceptedFinalRecord
	thread       map[string]any
	issuedAt     time.Time
}

type childCompletionVerificationFailureV1 struct {
	*childCompletionTestAuthority
	err                     error
	call, failAt, signCalls int
}

func (authority *childCompletionVerificationFailureV1) VerifyTrusted(ctx context.Context, keyID string, publicKey, message, signature []byte) error {
	authority.call++
	if authority.call == authority.failAt {
		return authority.err
	}
	return authority.childCompletionTestAuthority.VerifyTrusted(ctx, keyID, publicKey, message, signature)
}

func (authority *childCompletionVerificationFailureV1) Sign(context.Context, []byte) ([]byte, error) {
	authority.signCalls++
	return nil, errors.New("historical observation cannot sign")
}

type childCompletionOriginalReadFailureV1 struct{ err error }

func (reader childCompletionOriginalReadFailureV1) GetThread(string) (map[string]any, error) {
	return nil, reader.err
}

func TestStoredChildCompletionPreservesOriginalObservationErrors(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	verified, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := verified.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record
	record.Status, record.ChildTurnID, record.ChildCompletionReceipt = "completed", fixture.child.TurnID, receipt
	for _, sentinel := range []error{errors.New("synthetic historical observation I/O failure"), context.Canceled} {
		for _, failAt := range []int{0, 1, 2, 3} {
			owner := *fixture.authority
			key := &childCompletionVerificationFailureV1{childCompletionTestAuthority: fixture.host, err: sentinel, failAt: failAt}
			owner.authority = key
			if failAt == 0 {
				owner.threads = childCompletionOriginalReadFailureV1{err: sentinel}
			}
			if err := owner.VerifyStoredChildCompletion(context.Background(), record); !errors.Is(err, sentinel) {
				t.Errorf("historical owner lost original failure at %d: %v", failAt, err)
			}
			if key.signCalls != 0 {
				t.Fatal("historical observation attempted to sign")
			}
		}
	}
}

func TestStoredChildCompletionVerifierExposesOnlyHistoricalObservation(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	verified, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := verified.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record
	record.Status, record.ChildTurnID, record.ChildCompletionReceipt = "completed", fixture.child.TurnID, receipt
	key := &childCompletionVerificationFailureV1{childCompletionTestAuthority: fixture.host}
	reader := NewStoredChildCompletionVerifierV1(fixture.authority.contexts, fixture.authority.finals, key, fixture.authority.threads)
	// No dummy current validator is installed. The returned method set cannot
	// issue, rehydrate or produce parent continuation capabilities.
	if reflect.TypeOf(reader).NumMethod() != 1 {
		t.Fatal("historical verifier exposes a live capability")
	}
	if err := reader.VerifyStoredChildCompletion(context.Background(), record); err != nil || key.signCalls != 0 || key.call != 3 {
		t.Fatalf("historical observation failed or signed: calls=%d signs=%d err=%v", key.call, key.signCalls, err)
	}
	fixture.authority.validateCurrent = nil
	if _, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID); err == nil {
		t.Fatal("live issue bypassed missing current validation")
	}
	if _, err := fixture.authority.Rehydrate(context.Background(), record); err == nil {
		t.Fatal("live rehydration bypassed missing current validation")
	}
	for _, fault := range []string{"missing_context", "missing_final", "foreign_key", "changed_run", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			candidate := record
			contexts, finals := fixture.authority.contexts, fixture.authority.finals
			var authority = fixture.authority.authority
			ctx := context.Background()
			switch fault {
			case "missing_context":
				contexts = func(string, string) (domainsecurity.TurnSecurityContext, bool) {
					return domainsecurity.TurnSecurityContext{}, false
				}
			case "missing_final":
				finals = childCompletionFinalStub{}
			case "foreign_key":
				authority = newChildCompletionTestAuthority(72)
			case "changed_run":
				candidate.ID = "job-2"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			reader := NewStoredChildCompletionVerifierV1(contexts, finals, authority, fixture.authority.threads)
			if err := reader.VerifyStoredChildCompletion(ctx, candidate); err == nil || fault == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("invalid historical observation was accepted or masked: %v", err)
			}
		})
	}
}

func TestChildCompletionReceiptVerifiesAndRehydratesOnlyThroughTrustedHost(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	verified, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := verified.ReceiptForPersistence()
	if err != nil {
		t.Fatal(err)
	}
	completed := fixture.record
	completed.Status = string(domainjob.StatusCompleted)
	completed.ChildTurnID = fixture.child.TurnID
	completed.ChildCompletionReceipt = receipt
	if err := fixture.authority.VerifyStoredChildCompletion(context.Background(), completed); err != nil {
		t.Fatal(err)
	}
	if completed.Output != "" || completed.ChildCompletionReceipt == nil {
		t.Fatalf("trusted completion retained unsafe output: %#v", completed)
	}
	rehydrated, err := fixture.authority.Rehydrate(context.Background(), completed)
	if err != nil {
		t.Fatal(err)
	}
	if digest, ok := rehydrated.VerifyParentContinuation(fixture.parent, fixture.grant, fixture.grant.ToolCallID); !ok || digest != receipt.ReceiptDigest {
		t.Fatalf("rehydrated completion lost exact parent authority: digest=%q ok=%v", digest, ok)
	}
}

func TestForgedSelfSignedChildReceiptIsRejectedByHostTrustAnchor(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	attacker := newChildCompletionTestAuthority(92)
	forged, err := domainjob.NewChildCompletionReceiptV1(domainjob.ChildCompletionReceiptInputV1{
		ChildRunID: fixture.record.ID, SecurityBinding: fixture.binding, ParentContext: fixture.parent, ChildContext: fixture.child,
		AcceptedFinal: fixture.privateFinal.AcceptedFinal, CanContinueParent: true, IssuedAt: fixture.issuedAt,
		AuthorityKeyID: attacker.KeyID(), AuthorityPublicKey: attacker.PublicKey(),
	}, func(message []byte) ([]byte, error) { return attacker.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	record := fixture.record
	record.Status = string(domainjob.StatusCompleted)
	record.ChildTurnID = fixture.child.TurnID
	record.ChildCompletionReceipt = &forged
	if err := fixture.authority.VerifyStoredChildCompletion(context.Background(), record); err == nil {
		t.Fatal("arbitrary self-signed child receipt passed the host trust anchor")
	}
	if _, err := fixture.authority.Rehydrate(context.Background(), record); err == nil {
		t.Fatal("arbitrary self-signed child receipt created an in-process continuation capability")
	}
}

func TestExpiredOrStaleParentAuthorityCannotIssueChildCompletionReceipt(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	expiresAt, err := time.Parse(time.RFC3339Nano, fixture.grant.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority.now = func() time.Time { return expiresAt }
	if _, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID); err == nil {
		t.Fatal("expired parent grant issued a child completion receipt")
	}

	fixture = newChildCompletionFixture(t)
	fixture.authority.validateCurrent = func(_ context.Context, context domainsecurity.TurnSecurityContext) error {
		if context == fixture.parent {
			return errors.New("stale parent")
		}
		return nil
	}
	if _, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID); err == nil {
		t.Fatal("stale parent context issued a child completion receipt")
	}
}

func TestChildCompletionReceiptCannotPrecedeCommittedFinalDisposition(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	futureDisposition, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: fixture.privateFinal.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: domainsecurity.SHA256Hex([]byte("future-child-event-manifest")),
		DecidedAt:           fixture.issuedAt.Add(time.Minute), AuthorityKeyID: fixture.host.KeyID(), AuthorityPublicKey: fixture.host.PublicKey(),
	}, func(message []byte) ([]byte, error) { return fixture.host.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority.finals = childCompletionFinalStub{private: fixture.privateFinal, disposition: futureDisposition, ok: true}
	if _, err := fixture.authority.Issue(context.Background(), fixture.record, fixture.child.TurnID); err == nil {
		t.Fatal("child completion receipt was issued before the committed final disposition existed")
	}
}

func TestCompleteTaskReturnsOnlyRehydratedMetadataAndPersistsNoRawCaseOutput(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	const sensitive = "bank account 6222020000000000000 and private child reasoning"
	driver := newCompletionDriverStub(fixture.record)
	driver.startResponse = map[string]any{
		"turnId": fixture.child.TurnID, "status": "completed",
		"usage":            map[string]any{"totalTokens": float64(10), "reasoning": sensitive},
		"cacheDiagnostics": map[string]any{"privatePayload": sensitive},
	}
	driver.summary = sensitive
	result := CompleteTask(context.Background(), CompleteTaskInput{
		Request: TaskRequest{Prompt: "case child"}, Record: fixture.record, Driver: driver,
		StartAuthority: NewBackgroundJobStartBarrier(), CompletionAuthority: fixture.authority,
	})
	if result.IsError || result.Record.Status != string(domainjob.StatusCompleted) || result.Record.ChildCompletionReceipt == nil {
		t.Fatalf("trusted case child did not complete through receipt authority: %#v", result)
	}
	persisted := driver.records[fixture.record.ID]
	if persisted.Output != "" || persisted.Usage != nil || strings.Contains(fmt.Sprint(result.Output), sensitive) {
		t.Fatalf("raw case child output or private usage escaped: record=%#v output=%#v", persisted, result.Output)
	}
	if result.Output["childCompletionReceiptDigest"] == "" || result.Output["outputWithheld"] != true || result.Output["canReadOutput"] != false {
		t.Fatalf("trusted child completion did not return metadata-only projection: %#v", result.Output)
	}
}

func TestCompleteTaskCurrentAuthorityReadFailurePreservesRunningRecord(t *testing.T) {
	fixture := newChildCompletionFixture(t)
	cause := errors.New("synthetic current completion authority I/O failure")
	fixture.authority.validateCurrent = func(context.Context, domainsecurity.TurnSecurityContext) error { return cause }
	driver := newCompletionDriverStub(fixture.record)
	driver.startResponse = map[string]any{"turnId": fixture.child.TurnID, "status": "completed", "usage": map[string]any{}, "cacheDiagnostics": map[string]any{}}
	driver.summary = "Synthetic successful child response"
	result := CompleteTask(context.Background(), CompleteTaskInput{Record: fixture.record, Driver: driver, StartAuthority: NewBackgroundJobStartBarrier(), CompletionAuthority: fixture.authority})
	if !result.IsError || !errors.Is(result.Err, cause) || driver.updates != 0 || len(driver.progress) != 0 || driver.records[fixture.record.ID].Status != fixture.record.Status || driver.records[fixture.record.ID].ChildCompletionReceipt != nil {
		t.Fatalf("unavailable completion authority became terminal: error=%v updates=%d progress=%d", result.IsError, driver.updates, len(driver.progress))
	}
}

func newChildCompletionFixture(t *testing.T) childCompletionFixture {
	t.Helper()
	parent, grant, thread := jobSecurityAuthorityFixture(t, "task", false)
	child, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child", TurnID: "turn-child", WorkspaceRealPath: parent.WorkspaceRealPath, CaseID: parent.CaseID,
		CaseBindingHash: parent.CaseBindingHash, DatasetSnapshotID: parent.DatasetSnapshotID,
		SourceManifestHash: parent.SourceManifestHash, ContextEpoch: 1, IssuedAt: mustJobSecurityTime(t, grant.IssuedAt).Add(30 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainjob.NewSecurityBinding(parent, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	host := newChildCompletionTestAuthority(71)
	acceptedAt := mustJobSecurityTime(t, grant.IssuedAt).Add(time.Minute)
	privateFinal, disposition := newChildCompletionFinal(t, host, child, acceptedAt)
	contexts := map[string]domainsecurity.TurnSecurityContext{
		parent.ThreadID + "\x00" + parent.TurnID: parent,
		child.ThreadID + "\x00" + child.TurnID:   child,
	}
	authority := NewChildCompletionAuthority(
		func(threadID string, turnID string) (domainsecurity.TurnSecurityContext, bool) {
			context, ok := contexts[threadID+"\x00"+turnID]
			return context, ok
		},
		childCompletionFinalStub{private: privateFinal, disposition: disposition, ok: true}, host,
		jobSecurityThreadStub{thread: thread},
		func(_ context.Context, context domainsecurity.TurnSecurityContext) error {
			if current, ok := contexts[context.ThreadID+"\x00"+context.TurnID]; !ok || current != context {
				return errors.New("context is stale")
			}
			return nil
		},
	)
	issuedAt := acceptedAt.Add(time.Minute)
	authority.now = func() time.Time { return issuedAt }
	record := domainjob.Record{
		ID: "job-1", ParentGoalID: "goal-1", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: grant.ToolCallID, SecurityBinding: binding, ChildThreadID: child.ThreadID,
		Kind: "subagent", Status: string(domainjob.StatusRunning),
	}
	jobsecuritytest.BindDelegatedToolManifest(t, &record)
	return childCompletionFixture{
		authority: authority, host: host, parent: parent, child: child, grant: grant, binding: binding,
		record: record, privateFinal: privateFinal, thread: thread, issuedAt: issuedAt,
	}
}

func newChildCompletionFinal(t *testing.T, authority *childCompletionTestAuthority, contextValue domainsecurity.TurnSecurityContext, acceptedAt time.Time) (domainevidence.PrivateAcceptedFinalRecord, domainevidence.AcceptedFinalDispositionRecord) {
	t.Helper()
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: contextValue, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: acceptedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(contextValue)
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
		CreatedAt: acceptedAt.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(contextValue, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: contextValue, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: acceptedAt, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	private, err := domainevidence.NewPrivateAcceptedFinalRecord(contextValue, envelope, rendered, head, intent, accepted)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecord(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: accepted, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: domainsecurity.SHA256Hex([]byte("child-event-manifest")), DecidedAt: acceptedAt.Add(time.Second),
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	return private, disposition
}
