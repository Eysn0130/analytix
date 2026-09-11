package evidence

import (
	"bytes"
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

//go:embed testdata/accepted-final-v5-fact-witness-admission-v2.json
var acceptedFinalV5FactWitnessAdmissionV2Fixture []byte

func TestPublicationSnapshotProofBindsFactAcceptedFinalAndRejectsTampering(t *testing.T) {
	now := evidenceReceiptTestTime()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x57}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	installationID := domainsecurity.SHA256Hex([]byte("fact-final-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("fact-final-enrollment"))
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	dataset := newFactFinalDatasetAuthorityFixture(
		t,
		"thread-proof",
		"turn-proof",
		"case-proof",
		8,
		installationID,
		enrollmentID,
		keyID,
		publicKey,
		sign,
	)
	securityContext := dataset.context
	registry, err := NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	material := evidenceReceiptTestMaterial(t, "1800000")
	settlement := evidenceReceiptTestSettlementProof("publication-proof")
	registry, receipt, err := RegisterEvidenceReceipt(
		registry,
		evidenceReceiptTestDraft(t, securityContext, material, EvidenceSettlementReceiptID(settlement.SettlementID)),
		material, settlement, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	payload := NormalizedClaimPayload{
		SubjectID: "entity-a", EntityID: "entity-a", AttributeName: "document_author",
		AttributeValue: "controlled-evidence-only", Granularity: "record",
	}
	proposal := ClaimProposal{
		SchemaVersion: ClaimProposalVersion, ProposalID: "proposal-proof", ClaimType: ClaimBidEditMetadata,
		NormalizedPayload: payload, EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	scope := EvidenceQueryRange{
		EntityIDs: []string{"entity-a"}, AccountIDs: []string{}, Directions: []string{}, SourceIDs: []string{"bid-a"},
		FiltersHash: domainsecurity.SHA256Hex([]byte("filters-proof")),
	}
	claim, err := NewClaimRecord(ClaimRecordInput{
		ClaimID: "claim-proof", Proposal: proposal, SupportState: ClaimVerified, EvidenceIDs: []string{receipt.ReceiptID},
		CounterEvidenceIDs: []string{}, SupportedScope: &scope, AllowedWording: []string{"exact_verified_fact"},
		ProhibitedUpgrades: []string{"legal_characterization_without_review"},
		VerifierReceiptID:  VerifierReceiptDigest("claim-proof", proposal.ClaimType, payload, []string{receipt.ReceiptID}, nil, ClaimVerified),
		VerificationReason: "test", VerifiedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: EvidenceBackedAnswer, Context: securityContext, TerminalReason: "success", Claims: []ClaimRecord{claim},
		EvidenceReceiptIDs: []string{receipt.ReceiptID}, CheckedScope: &scope, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := NewPublicationSnapshotProof(PublicationSnapshotProofInput{
		Context: securityContext, RegistryHead: head, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs, CheckedAt: now.Add(time.Second),
		Sources: []PublicationSourceSnapshot{{
			ReceiptID: receipt.ReceiptID, ServerID: "analytix_funds",
			ServerIdentity: receipt.ServerIdentity, ServerVersion: receipt.ServerVersion, ConnectionEpoch: receipt.ConnectionEpoch,
			ToolName: receipt.ToolName, DatasetSnapshotID: securityContext.DatasetSnapshotID,
			CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog-proof")),
			SpecFingerprint:    domainsecurity.SHA256Hex([]byte("spec-proof")), ProbeDigest: domainsecurity.SHA256Hex([]byte("probe-proof")),
			CheckedAt: now.Format(time.RFC3339Nano),
		}},
	})
	if err != nil || ValidatePublicationSnapshotProofValue(&proof, securityContext, envelope, &head) != nil {
		t.Fatalf("publication snapshot proof did not validate: %#v err=%v", proof, err)
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
	if _, err := PrivateAcceptedFinalDigest(securityContext, envelope, rendered, intent); err == nil {
		t.Fatal("fact-bearing private accepted final omitted the publication snapshot proof")
	}
	if _, err := PrivateAcceptedFinalDigest(securityContext, envelope, rendered, intent, &proof); err == nil {
		t.Fatal("fact-bearing private accepted final accepted a source proof without witnessed evidence authority")
	}
	capsule, err := NewEvidenceRegistryAuthorityCapsule(securityContext, registry, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	rootIndex, err := NewEvidenceRegistryAuthorityIndexV2(EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		MutationID:          domainsecurity.SHA256Hex([]byte("fact-final-registry-mutation")),
	}, capsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewEvidenceAuthorityBundleV1(EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("fact-final-bundle-mutation")),
		DatasetSnapshotIndexDigest:  dataset.rootIndex.IndexDigest,
		DatasetSnapshotCount:        uint64(len(dataset.indexPath)),
		EvidenceRegistryIndexDigest: rootIndex.IndexDigest, EvidenceRegistryCount: 1,
		PublicationIndexDigest: domainsecurity.SHA256Hex([]byte("fact-final-publication-index")),
		AuthorityKeyID:         keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	witnessKeyID := domainsecurity.SHA256Hex(witnessPublic)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: EvidenceAuthorityBundleWitnessNamespaceV1,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("fact-final-previous-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("fact-final-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("fact-final-fence")), MutationID: bundle.MutationID,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	observeRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: EvidenceAuthorityBundleWitnessNamespaceV1,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("fact-final-fresh-challenge")), AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(observeRequest, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witnessPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	admissionInput := FactFinalWitnessAdmissionInputV1{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, PublicationProof: &proof, Registry: registry,
		Bundle: bundle, ObserveRequest: observeRequest, Observation: observation, RootIndex: rootIndex,
		SelectedIndex: rootIndex, SelectedCapsule: capsule, RegistryIndexPath: []EvidenceRegistryAuthorityIndexV2{rootIndex},
		DatasetRootIndex: dataset.rootIndex, SelectedDatasetIndex: dataset.selectedIndex,
		DatasetIndexPath: dataset.indexPath, DatasetRecord: dataset.record, DatasetManifest: dataset.manifest,
		FundsProducerContent: dataset.producer, BindingObservation: dataset.observation,
		InstallationID: installationID, EnrollmentID: enrollmentID, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic, AdmittedAt: now.Add(2 * time.Second),
	}
	admission, err := NewFactFinalWitnessAdmissionV1(admissionInput)
	if err != nil {
		t.Fatal(err)
	}
	if admission.SchemaVersion != FactFinalWitnessAdmissionSchemaVersionV2 ||
		admission.Purpose != FactFinalWitnessAdmissionPurposeV2 ||
		ValidateFactFinalWitnessAdmissionExactV1(admission, admissionInput) != nil {
		t.Fatalf("new fact-final admission was not exact V2: %#v", admission)
	}
	for name, mutate := range map[string]func(*FactFinalWitnessAdmissionInputV1){
		"path": func(candidate *FactFinalWitnessAdmissionInputV1) {
			candidate.DatasetIndexPath = []domainsecurity.DatasetSnapshotIndexV1{dataset.rootIndex}
		},
		"root": func(candidate *FactFinalWitnessAdmissionInputV1) {
			candidate.DatasetRootIndex = dataset.previousIndex
		},
		"selected": func(candidate *FactFinalWitnessAdmissionInputV1) {
			candidate.SelectedDatasetIndex = dataset.previousIndex
		},
		"record": func(candidate *FactFinalWitnessAdmissionInputV1) {
			candidate.DatasetRecord = dataset.previousRecord
		},
		"manifest": func(candidate *FactFinalWitnessAdmissionInputV1) {
			candidate.DatasetManifest = dataset.previousManifest
		},
		"producer": func(candidate *FactFinalWitnessAdmissionInputV1) {
			candidate.FundsProducerContent = dataset.previousProducer
		},
	} {
		t.Run("dataset_"+name+"_tamper_rejected", func(t *testing.T) {
			candidate := admissionInput
			candidate.DatasetIndexPath = append(
				[]domainsecurity.DatasetSnapshotIndexV1(nil),
				admissionInput.DatasetIndexPath...,
			)
			mutate(&candidate)
			if _, err := NewFactFinalWitnessAdmissionV1(candidate); err == nil {
				t.Fatalf("fact-final admission accepted tampered dataset %s", name)
			}
		})
	}
	legacy := admission
	legacy.SchemaVersion = FactFinalWitnessAdmissionSchemaVersionV1
	legacy.Purpose = FactFinalWitnessAdmissionPurposeV1
	legacy.DatasetSnapshotIndexDigest = ""
	legacy.DatasetSnapshotCount = 0
	legacy.SelectedDatasetSnapshotIndexDigest = ""
	legacy.SelectedDatasetSnapshotIndexGeneration = 0
	legacy.SelectedDatasetSnapshotRecordDigest = ""
	legacy.DatasetSnapshotManifestDigest = ""
	legacy.FundsProducerContentID = ""
	legacy.FundsProducerContentManifestSHA256 = ""
	legacy.AdmissionDigest = factFinalWitnessAdmissionLegacyDigestForTest(legacy)
	legacyBody, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyWireBody, err := json.Marshal(factFinalWitnessAdmissionLegacyWireForTest(legacy))
	if err != nil || !bytes.Equal(legacyBody, legacyWireBody) ||
		legacy.AdmissionDigest != factFinalWitnessAdmissionDigestV1(legacy) ||
		ValidateFactFinalWitnessAdmissionV1(legacy) != nil {
		t.Fatalf(
			"historical V1 admission bytes or digest drifted: body=%s wire=%s err=%v",
			legacyBody,
			legacyWireBody,
			err,
		)
	}
	legacyInput := admissionInput
	legacyInput.DatasetRootIndex = domainsecurity.DatasetSnapshotIndexV1{}
	legacyInput.SelectedDatasetIndex = domainsecurity.DatasetSnapshotIndexV1{}
	legacyInput.DatasetIndexPath = nil
	legacyInput.DatasetRecord = domainsecurity.DatasetSnapshotAuthorityRecordV2{}
	legacyInput.DatasetManifest = domainsecurity.DatasetSnapshotManifestV2{}
	legacyInput.FundsProducerContent = domainsecurity.FundsProducerContentManifestV1{}
	legacyInput.BindingObservation = domainsecurity.CaseBindingObservationV1{}
	if err := ValidateFactFinalWitnessAdmissionExactV1(legacy, legacyInput); err != nil {
		t.Fatalf("historical V1 exact audit validation drifted: %v", err)
	}
	if ValidateFactFinalWitnessAdmissionExactV1(legacy, admissionInput) == nil {
		t.Fatal("historical V1 exact validation accepted newly supplied dataset authority")
	}
	legacyWithDatasetFields := legacy
	legacyWithDatasetFields.DatasetSnapshotIndexDigest = admission.DatasetSnapshotIndexDigest
	legacyWithDatasetFields.AdmissionDigest = factFinalWitnessAdmissionDigestV1(legacyWithDatasetFields)
	if ValidateFactFinalWitnessAdmissionV1(legacyWithDatasetFields) == nil {
		t.Fatal("historical V1 admission accepted V2 dataset fields")
	}
	if issued, err := NewFactFinalWitnessAdmissionV1(legacyInput); err == nil ||
		issued.SchemaVersion == FactFinalWitnessAdmissionSchemaVersionV1 {
		t.Fatalf("new constructor re-issued legacy V1 authority: admission=%#v err=%v", issued, err)
	}
	privateDigest, err := PrivateAcceptedFinalDigestWithFactWitnessV1(securityContext, envelope, rendered, intent, &proof, &admission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign); err == nil {
		t.Fatal("fact-bearing accepted final omitted the publication snapshot proof")
	}
	record, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head, PublicationSnapshotProof: &proof,
		FactFinalWitnessAdmission: &admission, FactFinalWitnessAuthority: &admissionInput,
		PrivateRecordDigest: privateDigest, AcceptedAt: now.Add(2 * time.Second),
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil || record.SchemaVersion != AcceptedFinalRecordVersion || record.PublicationSnapshotProofDigest != proof.ProofDigest {
		t.Fatalf("accepted final did not bind the publication snapshot proof: %#v err=%v", record, err)
	}
	recordBody, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recordBody, bytes.TrimSpace(acceptedFinalV5FactWitnessAdmissionV2Fixture)) {
		t.Fatalf("Go V2 accepted-final fixture drifted: %s", recordBody)
	}
	resignAcceptedFinal := func(candidate *AcceptedFinalRecord) {
		candidate.AuthoritySignature = ""
		candidate.RecordDigest = ""
		candidate.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, AcceptedFinalSigningBytes(*candidate)))
		candidate.RecordDigest = acceptedFinalRecordDigest(*candidate)
	}
	missingExactAuthority := AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head, PublicationSnapshotProof: &proof,
		FactFinalWitnessAdmission: &admission, PrivateRecordDigest: privateDigest, AcceptedAt: now.Add(2 * time.Second),
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	if _, err := NewAcceptedFinalRecord(missingExactAuthority, sign); err == nil {
		t.Fatal("accepted final signer trusted a compact witness admission without exact authority input")
	}
	noncanonicalTime := record
	noncanonicalTime.AcceptedAt = now.Add(2 * time.Second).Format("2006-01-02T15:04:05.999999999+00:00")
	resignAcceptedFinal(&noncanonicalTime)
	if ValidateAcceptedFinalRecord(noncanonicalTime) == nil {
		t.Fatal("witnessed accepted final admitted a non-canonical UTC timestamp")
	}
	reversedTime := record
	reversedTime.AcceptedAt = now.Add(time.Second).UTC().Format(time.RFC3339Nano)
	resignAcceptedFinal(&reversedTime)
	if ValidateAcceptedFinalRecord(reversedTime) == nil {
		t.Fatal("witnessed accepted final predated its admission")
	}
	detachedAuthority := record
	detachedAdmission := *record.FactFinalWitnessAdmission
	detachedAdmission.WitnessBinding.AuthorityKeyID = domainsecurity.SHA256Hex([]byte("other-final-authority"))
	detachedAdmission.WitnessBinding.BindingDigest = evidenceAuthorityWitnessBindingDigestV1(detachedAdmission.WitnessBinding)
	detachedAdmission.AdmissionDigest = factFinalWitnessAdmissionDigestV1(detachedAdmission)
	detachedAuthority.FactFinalWitnessAdmission = &detachedAdmission
	resignAcceptedFinal(&detachedAuthority)
	if ValidateAcceptedFinalRecord(detachedAuthority) == nil {
		t.Fatal("witness admission authority detached from accepted-final signer")
	}
	privateRecord, err := NewPrivateAcceptedFinalRecord(securityContext, envelope, rendered, head, intent, record, &proof)
	if err != nil || ValidatePrivateAcceptedFinalRecord(privateRecord) != nil {
		t.Fatalf("private accepted final did not preserve the publication snapshot proof: %#v err=%v", privateRecord, err)
	}
	if err := ValidateAcceptedFinalForCurrentWriteV1(record); err != nil {
		t.Fatalf("witnessed V5 fact final was rejected at its current-write boundary: %v", err)
	}
	v4WithV2Admission := record
	historicalRendered, err := RenderFinalAnswerAtVersion(envelope, HistoricalFinalAnswerRendererVersion)
	if err != nil {
		t.Fatal(err)
	}
	v2AdmissionForV4 := *record.FactFinalWitnessAdmission
	v2AdmissionForV4.RenderedTextSHA256 = domainsecurity.SHA256Hex([]byte(historicalRendered))
	v2AdmissionForV4.AdmissionDigest = factFinalWitnessAdmissionDigestV1(v2AdmissionForV4)
	v4WithV2Admission.SchemaVersion = WitnessedFactAcceptedFinalRecordVersion
	v4WithV2Admission.RendererVersion = HistoricalFinalAnswerRendererVersion
	v4WithV2Admission.FinalGateVersion = WitnessedFinalEvidenceGateVersion
	v4WithV2Admission.RenderedTextSHA256 = domainsecurity.SHA256Hex([]byte(historicalRendered))
	v4WithV2Admission.PublicView = nil
	v4WithV2Admission.PublicViewDigest = ""
	v4WithV2Admission.FactFinalWitnessAdmission = &v2AdmissionForV4
	resignAcceptedFinal(&v4WithV2Admission)
	if ValidateAcceptedFinalRecord(v4WithV2Admission) == nil {
		t.Fatal("historical V4 accepted a current V2 fact-final witness admission")
	}
	if _, err := privateAcceptedFinalDigestForVersionAndRenderer(
		WitnessedFactPrivateFinalRecordVersion,
		v4WithV2Admission.RendererVersion,
		securityContext,
		envelope,
		historicalRendered,
		intent,
		&proof,
		v4WithV2Admission.FactFinalWitnessAdmission,
	); err == nil {
		t.Fatal("historical V4 private digest accepted a current V2 fact-final witness admission")
	}
	if err := ValidatePrivateAcceptedFinalPublicationAuthority(privateRecord); err == nil {
		t.Fatal("historical witness admission became live authority without a fresh witness replay")
	}
	if err := ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(privateRecord, func(candidate PrivateAcceptedFinalRecord) error {
		if candidate.StoreDigest != privateRecord.StoreDigest {
			t.Fatal("fresh witness verifier received a different private final")
		}
		return nil
	}); err != nil {
		t.Fatalf("fresh witness replay authority did not admit the exact V5 fact final: %v", err)
	}
	v3PrivateDigest, err := privateAcceptedFinalDigestForVersion(
		BoundaryAcceptedFinalRecordVersion, securityContext, envelope, rendered, intent, &proof, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	v3Fact := record
	v3Fact.SchemaVersion = BoundaryAcceptedFinalRecordVersion
	v3Fact.FinalGateVersion = HistoricalBoundaryFinalGateVersion
	v3Fact.FactFinalWitnessAdmission = nil
	v3Fact.PublicView = nil
	v3Fact.PublicViewDigest = ""
	v3Fact.PrivateRecordDigest = v3PrivateDigest
	v3Fact.AuthoritySignature = ""
	v3Fact.RecordDigest = ""
	v3Fact.AuthoritySignature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, AcceptedFinalSigningBytes(v3Fact)))
	v3Fact.RecordDigest = acceptedFinalRecordDigest(v3Fact)
	v3Private := privateRecord
	v3Private.SchemaVersion = BoundaryAcceptedFinalRecordVersion
	v3Private.AcceptedFinal = v3Fact
	v3Private.PrivateRecordDigest = v3PrivateDigest
	v3Private.StoreDigest = privateAcceptedFinalStoreDigest(v3Private)
	if ValidateAcceptedFinalRecord(v3Fact) != nil || ValidatePrivateAcceptedFinalRecord(v3Private) != nil {
		t.Fatal("historical V3 fact final stopped being structurally auditable")
	}
	if ValidateAcceptedFinalForCurrentWriteV1(v3Fact) == nil ||
		ValidatePrivateAcceptedFinalPublicationAuthority(v3Private) == nil ||
		ValidatePrivateAcceptedFinalPublicationAuthorityWithWitnessV1(v3Private, func(PrivateAcceptedFinalRecord) error { return nil }) == nil {
		t.Fatal("historical V3 fact final regained live publication authority")
	}
	mismatchedAdmissionInput := admissionInput
	mismatchedAdmissionInput.RootIndex.Generation++
	if _, err := NewFactFinalWitnessAdmissionV1(mismatchedAdmissionInput); err == nil {
		t.Fatal("fact final witness admission accepted a root outside the witnessed registry frontier")
	}
	detachedSelection := admissionInput
	detachedSelection.RegistryIndexPath = []EvidenceRegistryAuthorityIndexV2{rootIndex}
	detachedSelection.SelectedIndex = rootIndex
	detachedSelection.SelectedIndex.Entry.ContextDigest = domainsecurity.SHA256Hex([]byte("other-context"))
	if _, err := NewFactFinalWitnessAdmissionV1(detachedSelection); err == nil {
		t.Fatal("fact final witness admission accepted a detached selected registry index")
	}
	fabricated := admission
	fabricated.SourceManifestHash = domainsecurity.SHA256Hex([]byte("fabricated-manifest"))
	fabricated.AdmissionDigest = factFinalWitnessAdmissionDigestV1(fabricated)
	if _, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, RegistryHead: head, PublicationSnapshotProof: &proof,
		FactFinalWitnessAdmission: &fabricated, FactFinalWitnessAuthority: &admissionInput,
		PrivateRecordDigest: privateDigest, AcceptedAt: now.Add(2 * time.Second),
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign); err == nil {
		t.Fatal("accepted final signer trusted a self-consistent compact witness admission without exact authority")
	}
	tamperedProof := proof
	tamperedProof.Sources = clonePublicationSources(proof.Sources)
	tamperedProof.Sources[0].CatalogFingerprint = domainsecurity.SHA256Hex([]byte("different-catalog"))
	if ValidatePublicationSnapshotProof(tamperedProof) == nil {
		t.Fatal("publication snapshot proof accepted source tampering")
	}
	tamperedProof.ProofDigest = publicationSnapshotProofDigest(tamperedProof)
	tamperedPrivate := privateRecord
	tamperedPrivate.PublicationSnapshotProof = &tamperedProof
	tamperedPrivate.StoreDigest = privateAcceptedFinalStoreDigest(tamperedPrivate)
	if ValidatePrivateAcceptedFinalRecord(tamperedPrivate) == nil {
		t.Fatal("private accepted final accepted a rehashed proof detached from its signed public digest")
	}
}

func TestPublicationSnapshotRejectsAmbiguousReceiptToolIdentity(t *testing.T) {
	if got := publicationReceiptServerID("mcp__docs__lookup"); got != "docs" {
		t.Fatalf("valid publication tool identity failed: %q", got)
	}
	if got := publicationReceiptServerID("mcp__foo__bar__lookup"); got != "foo" {
		t.Fatalf("valid repeated-underscore publication tool identity failed: %q", got)
	}
	for _, name := range []string{"mcp__Docs__lookup", "mcp____lookup", "mcp__docs__"} {
		if got := publicationReceiptServerID(name); got != "" {
			t.Fatalf("ambiguous publication tool identity was parsed: %q => %q", name, got)
		}
	}
}
