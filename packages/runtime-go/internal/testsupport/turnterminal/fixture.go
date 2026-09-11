package turnterminal

import (
	"crypto/ed25519"
	"fmt"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

type FixtureV1 struct {
	PrivateKey          ed25519.PrivateKey
	PublicKey           ed25519.PublicKey
	Context             domainsecurity.TurnSecurityContext
	PrivateFinal        domainevidence.PrivateAcceptedFinalRecord
	EventManifestDigest string
	Intent              domainturnterminal.TurnTerminalIntentV1
	Closure             domaincachetelemetry.ProviderTurnClosureV1
	AcceptedDisposition domainevidence.AcceptedFinalDispositionRecord
	Disposition         domainturnterminal.TurnTerminalDispositionV1
}

func NewFixtureV1() (FixtureV1, error) {
	now := FixtureTimeV1()
	privateKey := ed25519.NewKeyFromSeed(repeatedBytes(3, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	securityContext, err := fixtureSecurityContextV1(now)
	if err != nil {
		return FixtureV1{}, err
	}
	fixture, err := NewFixtureWithAuthorityV1(securityContext, now, publicKey, sign)
	fixture.PrivateKey = privateKey
	return fixture, err
}

// NewFixtureWithAuthorityV1 produces the same complete synthetic terminal
// chain against the caller's frozen context and isolated signing authority.
func NewFixtureWithAuthorityV1(securityContext domainsecurity.TurnSecurityContext, now time.Time, publicKey ed25519.PublicKey, sign func([]byte) ([]byte, error)) (FixtureV1, error) {
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: securityContext,
		TerminalReason: string(domaincachetelemetry.ProviderTurnTerminalSourceUnavailableV1),
		Blocker:        "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: now,
	})
	if err != nil {
		return FixtureV1{}, err
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return FixtureV1{}, err
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		return FixtureV1{}, err
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		return FixtureV1{}, err
	}
	publicationIntent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		return FixtureV1{}, err
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(securityContext, envelope, rendered, publicationIntent)
	if err != nil {
		return FixtureV1{}, err
	}
	acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		return FixtureV1{}, err
	}
	privateFinal, err := domainevidence.NewPrivateAcceptedFinalRecord(
		securityContext, envelope, rendered, head, publicationIntent, acceptedFinal,
	)
	if err != nil {
		return FixtureV1{}, err
	}
	publicationPlan, err := appturn.BuildAcceptedFinalPublicationPlan(acceptedFinal, rendered, publicationIntent)
	if err != nil {
		return FixtureV1{}, err
	}
	manifest := publicationPlan.EventManifestDigest
	intent, err := domainturnterminal.NewTurnTerminalIntentV1(domainturnterminal.TurnTerminalIntentInputV1{
		PrivateFinal: privateFinal, EventManifestDigest: manifest,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		return FixtureV1{}, err
	}
	closure, err := domaincachetelemetry.NewProviderTurnClosureV1(domaincachetelemetry.ProviderTurnClosureInputV1{
		TurnBindingHMAC:    domainsecurity.SHA256Hex([]byte("provider-turn-binding")),
		Intents:            []domaincachetelemetry.ProviderAttemptIntentV1{},
		Settlements:        []domaincachetelemetry.ProviderAttemptSettlementV1{},
		TerminalReasonCode: intent.TerminalReasonCode, ClosedAt: now.Add(time.Second),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		return FixtureV1{}, err
	}
	observation, err := domainevidence.NewAcceptedFinalCASObservationV1(domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Status: "completed",
		FrozenContext: securityContext, CurrentContext: securityContext, HasWinner: true, Winner: acceptedFinal,
		ThreadFileSHA256:     domainsecurity.SHA256Hex([]byte("thread-file")),
		TurnProjectionSHA256: domainsecurity.SHA256Hex([]byte("turn-projection")),
	})
	if err != nil {
		return FixtureV1{}, err
	}
	acceptedDisposition, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{
		AcceptedFinal: acceptedFinal, State: domainevidence.AcceptedFinalCommitted,
		EventManifestDigest: manifest, DecidedAt: now.Add(2 * time.Second),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, observation, domainevidence.AcceptedFinalDecisionSamePublicWinner, sign)
	if err != nil {
		return FixtureV1{}, err
	}
	disposition, err := domainturnterminal.NewTurnTerminalDispositionV1(domainturnterminal.TurnTerminalDispositionInputV1{
		Intent: intent, ProviderClosure: closure, AcceptedFinalDisposition: acceptedDisposition,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		return FixtureV1{}, err
	}
	return FixtureV1{
		PublicKey: publicKey, Context: securityContext, PrivateFinal: privateFinal,
		EventManifestDigest: manifest, Intent: intent, Closure: closure,
		AcceptedDisposition: acceptedDisposition, Disposition: disposition,
	}, nil
}

func (fixture FixtureV1) Sign(message []byte) ([]byte, error) {
	if len(fixture.PrivateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("turn terminal fixture private key is invalid")
	}
	return ed25519.Sign(fixture.PrivateKey, message), nil
}

func FixtureTimeV1() time.Time {
	return time.Date(2026, 7, 15, 5, 0, 0, 0, time.UTC)
}

func fixtureSecurityContextV1(now time.Time) (domainsecurity.TurnSecurityContext, error) {
	workspace := "/workspace/terminal-authority"
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("thread-risk-policy")),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateInvalid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerCaseBindingInvalid,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-terminal", TurnID: "turn-terminal", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 4, IssuedAt: now, PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
}

func repeatedBytes(value byte, count int) []byte {
	body := make([]byte, count)
	for index := range body {
		body[index] = value
	}
	return body
}
