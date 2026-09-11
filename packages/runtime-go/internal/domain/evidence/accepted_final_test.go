package evidence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAcceptedFinalRecordIsStrictAndBindsRenderedText(t *testing.T) {
	context := evidenceReceiptTestContext(t, "thread-a", "turn-a", "case-a", "snapshot-a", 4)
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: SourceUnavailableAnswer, Context: context, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: evidenceReceiptTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewEvidenceReceiptRegistry(context)
	if err != nil {
		t.Fatal(err)
	}
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, _ := RenderFinalAnswer(envelope)
	intent, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt: evidenceReceiptTestTime().Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, _ := PrivateAcceptedFinalDigest(context, envelope, rendered, intent)
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: context, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: evidenceReceiptTestTime(),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	if record.ContextDigest != context.ContextDigest || record.RenderedTextSHA256 == "" {
		t.Fatalf("accepted final binding mismatch: %#v", record)
	}
	parsed := AcceptedFinalRecordMap(record)
	parsed["unexpected"] = true
	if _, err := ParseAcceptedFinalRecord(parsed); err == nil {
		t.Fatal("accepted final record allowed an unknown property")
	}
	tampered := record
	tampered.AcceptedAt = time.Unix(2, 0).UTC().Format(time.RFC3339Nano)
	if err := ValidateAcceptedFinalRecord(tampered); err == nil {
		t.Fatal("accepted final record allowed integrity tampering")
	}
	privateRecord, err := NewPrivateAcceptedFinalRecord(context, envelope, rendered, head, intent, record)
	if err != nil || ValidatePrivateAcceptedFinalRecord(privateRecord) != nil {
		t.Fatalf("private accepted final did not validate: %#v err=%v", privateRecord, err)
	}
	publicBody, _ := json.Marshal(record)
	for _, forbidden := range []string{"claims", "evidenceReceiptIds", "acquisitionSteps", rendered} {
		if strings.Contains(string(publicBody), forbidden) {
			t.Fatalf("public accepted final leaked private envelope material %q: %s", forbidden, publicBody)
		}
	}
	privateTamper := privateRecord
	privateTamper.RenderedText += "tamper"
	if err := ValidatePrivateAcceptedFinalRecord(privateTamper); err == nil {
		t.Fatal("private accepted final allowed rendered-text tampering")
	}
	intentTamper := privateRecord
	intentTamper.PublicationIntent.Model = "different-model"
	if err := ValidatePrivateAcceptedFinalRecord(intentTamper); err == nil {
		t.Fatal("private accepted final allowed publication-intent tampering")
	}
	for name, mutate := range map[string]func(*PrivateAcceptedFinalRecord){
		"claim": func(candidate *PrivateAcceptedFinalRecord) {
			candidate.Envelope.Claims = []ClaimRecord{{}}
		},
		"receipt": func(candidate *PrivateAcceptedFinalRecord) {
			candidate.Envelope.EvidenceReceiptIDs = []string{"receipt-detached"}
		},
		"publication proof": func(candidate *PrivateAcceptedFinalRecord) {
			candidate.PublicationSnapshotProof = &PublicationSnapshotProof{}
		},
		"fact-final witness": func(candidate *PrivateAcceptedFinalRecord) {
			candidate.AcceptedFinal.FactFinalWitnessAdmission = &FactFinalWitnessAdmissionV1{}
		},
		"registry context": func(candidate *PrivateAcceptedFinalRecord) {
			candidate.RegistryHead.ContextDigest = domainsecurity.SHA256Hex([]byte("detached-registry-context"))
		},
	} {
		t.Run("private "+name+" mismatch", func(t *testing.T) {
			candidate := privateRecord
			mutate(&candidate)
			candidate.StoreDigest = privateAcceptedFinalStoreDigest(candidate)
			if ValidatePrivateAcceptedFinalAuditAuthority(candidate) == nil {
				t.Fatalf("private %s mismatch retained audit authority", name)
			}
		})
	}
	disposition, err := NewAcceptedFinalDispositionRecord(AcceptedFinalDispositionInput{
		AcceptedFinal: record, State: AcceptedFinalCommitted,
		EventManifestDigest: domainsecurity.SHA256Hex([]byte("manifest")), DecidedAt: evidenceReceiptTestTime(),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil || ValidateAcceptedFinalDispositionRecord(disposition) != nil {
		t.Fatalf("accepted final disposition did not validate: %#v err=%v", disposition, err)
	}
	dispositionTamper := disposition
	dispositionTamper.State = AcceptedFinalExplicitlyNotCommitted
	if ValidateAcceptedFinalDispositionRecord(dispositionTamper) == nil {
		t.Fatal("accepted final disposition allowed state tampering")
	}
	observation, err := NewAcceptedFinalCASObservationV1(AcceptedFinalCASObservationV1{
		ThreadID: context.ThreadID, TurnID: context.TurnID, Status: "completed", FrozenContext: context, CurrentContext: context,
		HasWinner: true, Winner: record, ThreadFileSHA256: domainsecurity.SHA256Hex([]byte("thread-file")),
		TurnProjectionSHA256: domainsecurity.SHA256Hex([]byte("turn-cas")),
	})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := NewAcceptedFinalDispositionRecordV2(AcceptedFinalDispositionInput{
		AcceptedFinal: record, State: AcceptedFinalCommitted,
		EventManifestDigest: domainsecurity.SHA256Hex([]byte("manifest")), DecidedAt: evidenceReceiptTestTime(),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, observation, AcceptedFinalDecisionSamePublicWinner, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil || v2.SchemaVersion != AcceptedFinalDispositionRecordV2 || v2.TurnCASDigest != observation.TurnProjectionSHA256 ||
		v2.WinnerDigest != record.RecordDigest || ValidateAcceptedFinalDispositionRecord(v2) != nil {
		t.Fatalf("accepted final disposition V2 did not bind exact CAS basis: record=%#v err=%v", v2, err)
	}
	v2Tamper := v2
	v2Tamper.TurnCASDigest = domainsecurity.SHA256Hex([]byte("other-cas"))
	if ValidateAcceptedFinalDispositionRecord(v2Tamper) == nil {
		t.Fatal("accepted final disposition V2 allowed CAS-basis tampering")
	}
	if _, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt: evidenceReceiptTestTime().Format(time.RFC3339Nano), TerminalStatus: "completed",
		CacheDiagnostics: map[string]any{"reasoningContent": "private"},
	}, envelope.TerminalReason); err == nil {
		t.Fatal("terminal publication intent accepted private reasoning content")
	}
}

func TestHistoricalRendererV1MixedBoundaryRemainsReadable(t *testing.T) {
	now := evidenceReceiptTestTime()
	securityContext := evidenceReceiptTestContext(t, "thread-renderer-v1", "turn-renderer-v1", "case-renderer-v1", "snapshot-renderer-v1", 2)
	ordinary, err := domainordinaryresult.NewResultSlotV1("The ordinary read completed.")
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success", OrdinaryResult: &ordinary,
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyRendered, err := RenderFinalAnswerAtVersion(envelope, HistoricalFinalAnswerRendererVersion)
	if err != nil || legacyRendered != ordinary.Text+"\n\n"+LegacyCaseUnverifiedText {
		t.Fatalf("historical renderer changed its exact text: rendered=%q err=%v", legacyRendered, err)
	}
	currentRendered, err := RenderFinalAnswer(envelope)
	if err != nil || currentRendered != ordinary.Text+"\n\n"+CaseUnverifiedText || currentRendered == legacyRendered {
		t.Fatalf("current renderer did not separate the ordinary result from the case boundary: rendered=%q err=%v", currentRendered, err)
	}
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := privateAcceptedFinalDigestForVersionAndRenderer(
		PrivateAcceptedFinalRecordVersion, HistoricalFinalAnswerRendererVersion,
		securityContext, envelope, legacyRendered, intent, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	accepted := AcceptedFinalRecord{
		SchemaVersion: AcceptedFinalRecordVersion, AuthorityPurpose: AcceptedFinalAuthorityPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, EnvelopeDigest: envelope.EnvelopeDigest, ContextDigest: securityContext.ContextDigest,
		ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID, Variant: envelope.Variant,
		TerminalReason: envelope.TerminalReason, RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(legacyRendered)),
		RegistrySequence: head.Sequence, RegistryStateDigest: head.StateDigest, RendererVersion: HistoricalFinalAnswerRendererVersion,
		FinalGateVersion: FinalEvidenceGateVersion, VerifierVersion: ClaimVerifierPolicyVersion,
		PrivateRecordDigest: privateDigest, AcceptedAt: now.Format(time.RFC3339Nano),
	}
	publicView := buildAcceptedFinalPublicViewCoreV2(envelope, accepted)
	accepted.PublicView = &publicView
	accepted.PublicViewDigest = acceptedFinalPublicViewCoreV2Digest(publicView)
	accepted.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, AcceptedFinalSigningBytes(accepted)))
	accepted.RecordDigest = acceptedFinalRecordDigest(accepted)
	privateRecord := PrivateAcceptedFinalRecord{
		SchemaVersion: PrivateAcceptedFinalRecordVersion, SecurityContext: securityContext, Envelope: envelope,
		RenderedText: legacyRendered, RegistryHead: head, PublicationIntent: intent, AcceptedFinal: accepted,
		PrivateRecordDigest: privateDigest,
	}
	privateRecord.StoreDigest = privateAcceptedFinalStoreDigest(privateRecord)
	body, err := PrivateAcceptedFinalRecordBytes(privateRecord)
	if err != nil {
		t.Fatalf("historical renderer record failed current validation: %v", err)
	}
	parsed, err := ParsePrivateAcceptedFinalRecord(body)
	if err != nil || parsed.RenderedText != legacyRendered || parsed.AcceptedFinal.RendererVersion != HistoricalFinalAnswerRendererVersion {
		t.Fatalf("historical renderer record failed exact replay: parsed=%#v err=%v", parsed, err)
	}
}

func TestPreviousAcceptedFinalV2AllowsOnlySignedBoundaryHistory(t *testing.T) {
	securityContext := evidenceReceiptTestContext(t, "thread-v2", "turn-v2", "case-v2", "snapshot-v2", 3)
	now := evidenceReceiptTestTime()
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: NeedsEvidenceAnswer, Context: securityContext, TerminalReason: "success",
		MissingScope: []string{"current_case_facts"}, AcquisitionSteps: []string{"collect_evidence"}, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := privateAcceptedFinalDigestForVersion(PreviousAcceptedFinalRecordVersion, securityContext, envelope, rendered, intent, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record := AcceptedFinalRecord{
		SchemaVersion: PreviousAcceptedFinalRecordVersion, AuthorityPurpose: AcceptedFinalAuthorityPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, EnvelopeDigest: envelope.EnvelopeDigest, ContextDigest: securityContext.ContextDigest,
		ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID, Variant: envelope.Variant,
		TerminalReason: envelope.TerminalReason, RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(rendered)),
		RegistrySequence: head.Sequence, RegistryStateDigest: head.StateDigest, RendererVersion: FinalAnswerRendererVersion,
		FinalGateVersion: LegacyFinalEvidenceGateVersion, VerifierVersion: ClaimVerifierPolicyVersion,
		PrivateRecordDigest: privateDigest, AcceptedAt: now.Format(time.RFC3339Nano),
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, AcceptedFinalSigningBytes(record)))
	record.RecordDigest = acceptedFinalRecordDigest(record)
	if err := ValidateAcceptedFinalRecord(record); err != nil {
		t.Fatalf("trusted v2 boundary did not remain readable: %v", err)
	}
	privateRecord := PrivateAcceptedFinalRecord{
		SchemaVersion: PreviousAcceptedFinalRecordVersion, SecurityContext: securityContext, Envelope: envelope,
		RenderedText: rendered, RegistryHead: head, PublicationIntent: intent, AcceptedFinal: record, PrivateRecordDigest: privateDigest,
	}
	privateRecord.StoreDigest = privateAcceptedFinalStoreDigest(privateRecord)
	if err := ValidatePrivateAcceptedFinalRecord(privateRecord); err != nil {
		t.Fatalf("trusted private v2 boundary did not remain readable: %v", err)
	}
	factRecord := record
	factRecord.Variant = EvidenceBackedAnswer
	factRecord.AuthoritySignature = ""
	factRecord.RecordDigest = ""
	factRecord.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, AcceptedFinalSigningBytes(factRecord)))
	factRecord.RecordDigest = acceptedFinalRecordDigest(factRecord)
	if err := ValidateAcceptedFinalRecord(factRecord); err == nil {
		t.Fatal("signed v2 fact-bearing history remained publishable without a snapshot proof")
	}
}

func TestStructurallyValidV1PrivateFinalIsAuditOnly(t *testing.T) {
	now := evidenceReceiptTestTime()
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1-final-audit", TurnID: "turn-v1-final-audit", WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: "case-v1-final-audit",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("v1-final-binding")),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("v1-final-snapshot")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("v1-final-manifest")), ContextEpoch: 7, IssuedAt: now,
	})
	if securityContext.Version != domainsecurity.TurnSecurityContextVersionV1 ||
		domainsecurity.ValidateTurnSecurityContext(securityContext) != nil {
		t.Fatalf("V1 audit context is not structurally valid: %#v", securityContext)
	}
	envelope := FinalAnswerEnvelope{
		SchemaVersion: FinalAnswerEnvelopeVersion, Variant: NeedsEvidenceAnswer,
		ContextDigest: securityContext.ContextDigest, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, TerminalReason: "success",
		Claims: []ClaimRecord{}, EvidenceReceiptIDs: []string{}, MissingScope: []string{"current_case_facts"},
		AcquisitionSteps: []string{"collect_evidence"}, Guidance: []string{}, IssuedAt: now.UTC().Format(time.RFC3339Nano),
	}
	envelope.EnvelopeDigest = finalAnswerEnvelopeDigest(envelope)
	if err := ValidateFinalAnswerEnvelope(envelope); err != nil {
		t.Fatal(err)
	}
	var err error
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := privateAcceptedFinalDigestForVersion(
		BoundaryAcceptedFinalRecordVersion, securityContext, envelope, rendered, intent, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	accepted := AcceptedFinalRecord{
		SchemaVersion: BoundaryAcceptedFinalRecordVersion, AuthorityPurpose: AcceptedFinalAuthorityPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, EnvelopeDigest: envelope.EnvelopeDigest, ContextDigest: securityContext.ContextDigest,
		ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID, Variant: envelope.Variant,
		TerminalReason: envelope.TerminalReason, RenderedTextSHA256: domainsecurity.SHA256Hex([]byte(rendered)),
		RegistrySequence: head.Sequence, RegistryStateDigest: head.StateDigest, RendererVersion: FinalAnswerRendererVersion,
		FinalGateVersion: HistoricalBoundaryFinalGateVersion, VerifierVersion: ClaimVerifierPolicyVersion,
		PrivateRecordDigest: privateDigest, AcceptedAt: now.Format(time.RFC3339Nano),
	}
	accepted.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, AcceptedFinalSigningBytes(accepted)))
	accepted.RecordDigest = acceptedFinalRecordDigest(accepted)
	privateRecord := PrivateAcceptedFinalRecord{
		SchemaVersion: BoundaryAcceptedFinalRecordVersion, SecurityContext: securityContext, Envelope: envelope,
		RenderedText: rendered, RegistryHead: head, PublicationIntent: intent, AcceptedFinal: accepted,
		PrivateRecordDigest: privateDigest,
	}
	privateRecord.StoreDigest = privateAcceptedFinalStoreDigest(privateRecord)
	if err := ValidatePrivateAcceptedFinalRecord(privateRecord); err != nil {
		t.Fatalf("V1 audit private final is not structurally valid: %v", err)
	}
	if err := ValidatePrivateAcceptedFinalPublicationAuthority(privateRecord); err == nil {
		t.Fatal("V1 audit private final became current publication authority")
	}
	if _, err := NewAcceptedFinalPublicViewV1(privateRecord); err == nil {
		t.Fatal("V1 audit private final seeded a public projection")
	}
	if _, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }); err == nil {
		t.Fatal("V1 audit context issued a new accepted final")
	}
	if _, err := NewPrivateAcceptedFinalRecord(securityContext, envelope, rendered, head, intent, accepted); err == nil {
		t.Fatal("V1 audit context issued a new private accepted final")
	}
}
