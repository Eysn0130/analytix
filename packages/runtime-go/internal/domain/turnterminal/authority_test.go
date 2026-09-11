package turnterminal

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type terminalFixtureV1 struct {
	privateKey          ed25519.PrivateKey
	publicKey           ed25519.PublicKey
	privateFinal        domainevidence.PrivateAcceptedFinalRecord
	eventManifestDigest string
	intent              TurnTerminalIntentV1
	closure             domaincachetelemetry.ProviderTurnClosureV1
	acceptedDisposition domainevidence.AcceptedFinalDispositionRecord
	disposition         TurnTerminalDispositionV1
}

func TestTurnTerminalAuthorityBindsGateClosureAndPublicWinner(t *testing.T) {
	fixture := newTerminalFixtureV1(t)
	if err := ValidateTurnTerminalIntentForPrivateFinalV1(fixture.intent, fixture.privateFinal); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTurnTerminalDispositionForAuthoritiesV1(
		fixture.disposition, fixture.intent, fixture.closure, fixture.acceptedDisposition,
	); err != nil {
		t.Fatal(err)
	}
	intentBody, err := TurnTerminalIntentV1Bytes(fixture.intent)
	if err != nil {
		t.Fatal(err)
	}
	parsedIntent, err := ParseTurnTerminalIntentV1(intentBody)
	if err != nil || parsedIntent != fixture.intent {
		t.Fatalf("turn terminal intent round trip failed: parsed=%#v err=%v", parsedIntent, err)
	}
	dispositionBody, err := TurnTerminalDispositionV1Bytes(fixture.disposition)
	if err != nil {
		t.Fatal(err)
	}
	parsedDisposition, err := ParseTurnTerminalDispositionV1(dispositionBody)
	if err != nil || parsedDisposition != fixture.disposition {
		t.Fatalf("turn terminal disposition round trip failed: parsed=%#v err=%v", parsedDisposition, err)
	}
	for _, forbidden := range []string{
		fixture.privateFinal.RenderedText, "claims", "reasoning", "provider body", "6222021234567890123",
	} {
		if strings.Contains(string(intentBody), forbidden) || strings.Contains(string(dispositionBody), forbidden) {
			t.Fatalf("terminal authority leaked forbidden material %q", forbidden)
		}
	}
}

func TestTurnTerminalAuthorityRejectsAmbiguousOrTamperedRecords(t *testing.T) {
	fixture := newTerminalFixtureV1(t)
	intentBody, _ := TurnTerminalIntentV1Bytes(fixture.intent)
	var object map[string]any
	if err := json.Unmarshal(intentBody, &object); err != nil {
		t.Fatal(err)
	}
	object["providerSaysSafe"] = true
	unknownBody, _ := json.Marshal(object)
	if _, err := ParseTurnTerminalIntentV1(unknownBody); err == nil {
		t.Fatal("turn terminal intent accepted an unknown property")
	}
	if _, err := ParseTurnTerminalIntentV1(append([]byte(" "), intentBody...)); err == nil {
		t.Fatal("turn terminal intent accepted non-canonical JSON")
	}
	tampered := fixture.intent
	tampered.EventManifestDigest = domainsecurity.SHA256Hex([]byte("other-manifest"))
	if err := ValidateTurnTerminalIntentV1(tampered); err == nil {
		t.Fatal("turn terminal intent accepted a tampered event manifest")
	}
	tamperedDisposition := fixture.disposition
	tamperedDisposition.ProviderClosureID = domainsecurity.SHA256Hex([]byte("other-closure"))
	if err := ValidateTurnTerminalDispositionV1(tamperedDisposition); err == nil {
		t.Fatal("turn terminal disposition accepted a tampered provider closure")
	}
}

func TestTurnTerminalDispositionRejectsReasonAuthorityAndOrderMismatch(t *testing.T) {
	fixture := newTerminalFixtureV1(t)
	wrongReason := newProviderClosureV1(t, fixture, domaincachetelemetry.ProviderTurnTerminalProviderFailureV1,
		terminalFixtureTimeV1().Add(time.Second))
	if _, err := NewTurnTerminalDispositionV1(TurnTerminalDispositionInputV1{
		Intent: fixture.intent, ProviderClosure: wrongReason, AcceptedFinalDisposition: fixture.acceptedDisposition,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.publicKey), AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign); err == nil {
		t.Fatal("turn terminal disposition accepted a conflicting terminal reason")
	}
	otherPrivate := ed25519.NewKeyFromSeed(bytesOfV1(7, ed25519.SeedSize))
	otherPublic := otherPrivate.Public().(ed25519.PublicKey)
	if _, err := NewTurnTerminalDispositionV1(TurnTerminalDispositionInputV1{
		Intent: fixture.intent, ProviderClosure: fixture.closure, AcceptedFinalDisposition: fixture.acceptedDisposition,
		AuthorityKeyID: domainsecurity.SHA256Hex(otherPublic), AuthorityPublicKey: otherPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(otherPrivate, message), nil }); err == nil {
		t.Fatal("turn terminal disposition accepted another installation authority")
	}
	earlyClosure := newProviderClosureV1(t, fixture, fixture.intent.TerminalReasonCode,
		terminalFixtureTimeV1().Add(-time.Second))
	if _, err := NewTurnTerminalDispositionV1(TurnTerminalDispositionInputV1{
		Intent: fixture.intent, ProviderClosure: earlyClosure, AcceptedFinalDisposition: fixture.acceptedDisposition,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.publicKey), AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign); err == nil {
		t.Fatal("turn terminal disposition accepted provider closure before terminal intent")
	}
}

func newTerminalFixtureV1(t *testing.T) terminalFixtureV1 {
	t.Helper()
	now := terminalFixtureTimeV1()
	privateKey := ed25519.NewKeyFromSeed(bytesOfV1(3, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	securityContext := terminalSecurityContextV1(t, now)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: securityContext,
		TerminalReason: string(domaincachetelemetry.ProviderTurnTerminalSourceUnavailableV1),
		Blocker:        "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: now,
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
	publicationIntent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(securityContext, envelope, rendered, publicationIntent)
	if err != nil {
		t.Fatal(err)
	}
	acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	privateFinal, err := domainevidence.NewPrivateAcceptedFinalRecord(
		securityContext, envelope, rendered, head, publicationIntent, acceptedFinal,
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest := domainsecurity.SHA256Hex([]byte("accepted-final-event-manifest"))
	intent, err := NewTurnTerminalIntentV1(TurnTerminalIntentInputV1{
		PrivateFinal: privateFinal, EventManifestDigest: manifest,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	fixture := terminalFixtureV1{
		privateKey: privateKey, publicKey: publicKey, privateFinal: privateFinal,
		eventManifestDigest: manifest, intent: intent,
	}
	fixture.closure = newProviderClosureV1(t, fixture, intent.TerminalReasonCode, now.Add(time.Second))
	observation, err := domainevidence.NewAcceptedFinalCASObservationV1(domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Status: "completed",
		FrozenContext: securityContext, CurrentContext: securityContext, HasWinner: true, Winner: acceptedFinal,
		ThreadFileSHA256:     domainsecurity.SHA256Hex([]byte("thread-file")),
		TurnProjectionSHA256: domainsecurity.SHA256Hex([]byte("turn-projection")),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.acceptedDisposition, err = domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: acceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: manifest, DecidedAt: now.Add(2 * time.Second),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, observation, domainevidence.AcceptedFinalDecisionSamePublicWinner, sign)
	if err != nil {
		t.Fatal(err)
	}
	fixture.disposition, err = NewTurnTerminalDispositionV1(TurnTerminalDispositionInputV1{
		Intent: intent, ProviderClosure: fixture.closure, AcceptedFinalDisposition: fixture.acceptedDisposition,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func newProviderClosureV1(t *testing.T, fixture terminalFixtureV1,
	reason domaincachetelemetry.ProviderTurnTerminalReasonV1, closedAt time.Time) domaincachetelemetry.ProviderTurnClosureV1 {
	t.Helper()
	closure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC:    domainsecurity.SHA256Hex([]byte("provider-turn-binding")),
		Intents:            []domaincachetelemetry.ProviderAttemptIntentV1{},
		Settlements:        []domaincachetelemetry.ProviderAttemptSettlementV1{},
		TerminalReasonCode: reason, ClosedAt: closedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(fixture.publicKey), AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

func (fixture terminalFixtureV1) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.privateKey, message), nil
}

func terminalSecurityContextV1(t *testing.T, now time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	workspace := "/workspace/terminal-authority"
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("thread-risk-policy")),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateInvalid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingInvalid,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-terminal", TurnID: "turn-terminal", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 4, IssuedAt: now, PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func terminalFixtureTimeV1() time.Time {
	return time.Date(2026, 7, 15, 5, 0, 0, 0, time.UTC)
}

func bytesOfV1(value byte, count int) []byte {
	body := make([]byte, count)
	for index := range body {
		body[index] = value
	}
	return body
}
