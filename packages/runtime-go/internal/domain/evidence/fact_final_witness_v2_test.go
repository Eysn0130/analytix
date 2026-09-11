package evidence

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type factFinalDatasetAuthorityFixture struct {
	context          domainsecurity.TurnSecurityContext
	observation      domainsecurity.CaseBindingObservationV1
	rootIndex        domainsecurity.DatasetSnapshotIndexV1
	selectedIndex    domainsecurity.DatasetSnapshotIndexV1
	indexPath        []domainsecurity.DatasetSnapshotIndexV1
	record           domainsecurity.DatasetSnapshotAuthorityRecordV2
	manifest         domainsecurity.DatasetSnapshotManifestV2
	producer         domainsecurity.FundsProducerContentManifestV1
	previousIndex    domainsecurity.DatasetSnapshotIndexV1
	previousRecord   domainsecurity.DatasetSnapshotAuthorityRecordV2
	previousManifest domainsecurity.DatasetSnapshotManifestV2
	previousProducer domainsecurity.FundsProducerContentManifestV1
}

type factFinalWitnessAdmissionLegacyWireV1 struct {
	SchemaVersion                     int                               `json:"schemaVersion"`
	Purpose                           string                            `json:"purpose"`
	ContextDigest                     string                            `json:"contextDigest"`
	DatasetSnapshotID                 string                            `json:"datasetSnapshotId"`
	SourceManifestHash                string                            `json:"sourceManifestHash"`
	EnvelopeDigest                    string                            `json:"envelopeDigest"`
	RenderedTextSHA256                string                            `json:"renderedTextSha256"`
	PublicationSnapshotProofDigest    string                            `json:"publicationSnapshotProofDigest"`
	RegistrySequence                  uint64                            `json:"registrySequence"`
	RegistryStateDigest               string                            `json:"registryStateDigest"`
	EvidenceReceiptIDsDigest          string                            `json:"evidenceReceiptIdsDigest"`
	EvidenceReceiptCount              uint64                            `json:"evidenceReceiptCount"`
	EvidenceAuthorityBundleDigest     string                            `json:"evidenceAuthorityBundleDigest"`
	EvidenceAuthorityBundleGeneration uint64                            `json:"evidenceAuthorityBundleGeneration"`
	EvidenceRegistryIndexDigest       string                            `json:"evidenceRegistryIndexDigest"`
	EvidenceRegistryCount             uint64                            `json:"evidenceRegistryCount"`
	SelectedRegistryIndexDigest       string                            `json:"selectedRegistryIndexDigest"`
	SelectedRegistryIndexGeneration   uint64                            `json:"selectedRegistryIndexGeneration"`
	SelectedRegistryCapsuleDigest     string                            `json:"selectedRegistryCapsuleDigest"`
	WitnessBinding                    EvidenceAuthorityWitnessBindingV1 `json:"witnessBinding"`
	AdmittedAt                        string                            `json:"admittedAt"`
	AdmissionDigest                   string                            `json:"admissionDigest"`
}

func factFinalWitnessAdmissionLegacyWireForTest(
	admission FactFinalWitnessAdmissionV1,
) factFinalWitnessAdmissionLegacyWireV1 {
	return factFinalWitnessAdmissionLegacyWireV1{
		SchemaVersion:                     admission.SchemaVersion,
		Purpose:                           admission.Purpose,
		ContextDigest:                     admission.ContextDigest,
		DatasetSnapshotID:                 admission.DatasetSnapshotID,
		SourceManifestHash:                admission.SourceManifestHash,
		EnvelopeDigest:                    admission.EnvelopeDigest,
		RenderedTextSHA256:                admission.RenderedTextSHA256,
		PublicationSnapshotProofDigest:    admission.PublicationSnapshotProofDigest,
		RegistrySequence:                  admission.RegistrySequence,
		RegistryStateDigest:               admission.RegistryStateDigest,
		EvidenceReceiptIDsDigest:          admission.EvidenceReceiptIDsDigest,
		EvidenceReceiptCount:              admission.EvidenceReceiptCount,
		EvidenceAuthorityBundleDigest:     admission.EvidenceAuthorityBundleDigest,
		EvidenceAuthorityBundleGeneration: admission.EvidenceAuthorityBundleGeneration,
		EvidenceRegistryIndexDigest:       admission.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:             admission.EvidenceRegistryCount,
		SelectedRegistryIndexDigest:       admission.SelectedRegistryIndexDigest,
		SelectedRegistryIndexGeneration:   admission.SelectedRegistryIndexGeneration,
		SelectedRegistryCapsuleDigest:     admission.SelectedRegistryCapsuleDigest,
		WitnessBinding:                    admission.WitnessBinding,
		AdmittedAt:                        admission.AdmittedAt,
		AdmissionDigest:                   admission.AdmissionDigest,
	}
}

func factFinalWitnessAdmissionLegacyDigestForTest(admission FactFinalWitnessAdmissionV1) string {
	admission.AdmissionDigest = ""
	body, _ := json.Marshal(factFinalWitnessAdmissionLegacyWireForTest(admission))
	return domainsecurity.SHA256Hex(body)
}

func newFactFinalDatasetAuthorityFixture(
	t *testing.T,
	threadID, turnID, caseID string,
	epoch uint64,
	installationID, enrollmentID, authorityKeyID string,
	authorityPublicKey ed25519.PublicKey,
	sign func([]byte) ([]byte, error),
) factFinalDatasetAuthorityFixture {
	t.Helper()
	const workspace = "/workspace"
	now := evidenceReceiptTestTime()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            caseID,
		BindingSHA256:     domainsecurity.SHA256Hex([]byte("fact-final-binding-file:\x00" + caseID)),
		CaseBindingHash:   domainsecurity.SHA256Hex([]byte("binding-" + caseID)),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID:                 domainsecurity.LocalTenantID,
		UserID:                   domainsecurity.LocalUserID,
		WorkspaceRealPath:        workspace,
		CaseID:                   caseID,
		CaseBindingHash:          observation.CaseBindingHash,
		BindingObservationDigest: observation.ObservationDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	newProducer := func(seed string, revision uint64) domainsecurity.FundsProducerContentManifestV1 {
		producer, err := domainsecurity.NewFundsProducerContentManifestV1(domainsecurity.FundsProducerContentManifestInputV1{
			CaseID:                  caseID,
			SourceRevision:          revision,
			RawManifestSHA256:       domainsecurity.SHA256Hex([]byte(seed + ":raw")),
			NormalizedContentSHA256: domainsecurity.SHA256Hex([]byte(seed + ":normalized")),
			DetailContentSHA256:     domainsecurity.SHA256Hex([]byte(seed + ":detail")),
			AggregateContentSHA256:  domainsecurity.SHA256Hex([]byte(seed + ":aggregate")),
			KeywordContentSHA256:    domainsecurity.SHA256Hex([]byte(seed + ":keyword")),
			AccountContentSHA256:    domainsecurity.SHA256Hex([]byte(seed + ":account")),
			NormalizedRowCount:      4,
			AcceptedRowCount:        2,
			RejectedRowCount:        1,
			DuplicateRowCount:       1,
			DetailRowCount:          2,
			AggregateRowCount:       1,
			KeywordRowCount:         1,
			AccountRowCount:         2,
		})
		if err != nil {
			t.Fatal(err)
		}
		return producer
	}
	newManifest := func(seed string, acquiredAt time.Time, producer domainsecurity.FundsProducerContentManifestV1) domainsecurity.DatasetSnapshotManifestV2 {
		manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(domainsecurity.DatasetSnapshotManifestInputV2{
			Binding:                           binding,
			AcquisitionMethod:                 "test_import",
			AcquiredAt:                        acquiredAt,
			AcquisitionActorDigest:            domainsecurity.SHA256Hex([]byte(seed + ":actor")),
			RawArtifactManifestDigest:         domainsecurity.SHA256Hex([]byte(seed + ":raw-manifest-digest")),
			RawArtifactManifestSHA256:         producer.RawManifestSHA256,
			RawArtifactManifestByteLength:     128,
			RawArtifactCount:                  1,
			FundsProducerContentManifest:      producer,
			SourceType:                        "funds_transactions",
			ProducerPolicyID:                  "funds-materialization-v1",
			ProducerPolicyDigest:              domainsecurity.SHA256Hex([]byte(seed + ":producer-policy")),
			ProducerComponentID:               "analysis-compute",
			ProducerComponentVersion:          "0.1.0",
			ProducerOperation:                 "materialize-txn-daily",
			ProducerOperationSchemaHash:       domainsecurity.FundsProducerOperationSchemaHashV1,
			ParserID:                          "analytix-funds-parser",
			ParserVersion:                     "1.0.0",
			ParsedGenerationReceiptDigest:     domainsecurity.SHA256Hex([]byte(seed + ":parsed-receipt")),
			ParsedGenerationReceiptSHA256:     domainsecurity.SHA256Hex([]byte(seed + ":parsed-receipt-sha")),
			ParsedGenerationReceiptByteLength: 256,
			ClassificationLedgerDigest:        domainsecurity.SHA256Hex([]byte(seed + ":classification")),
			ClassificationLedgerSHA256:        domainsecurity.SHA256Hex([]byte(seed + ":classification-sha")),
			ClassificationLedgerByteLength:    192,
			TimezoneSemantics:                 "UTC",
			CurrencySemantics:                 "CNY_decimal_string",
			SourceRowLedgerRootDigest:         domainsecurity.SHA256Hex([]byte(seed + ":row-ledger")),
			SourceRowLedgerRootSHA256:         domainsecurity.SHA256Hex([]byte(seed + ":row-ledger-sha")),
			SourceRowLedgerRootByteLength:     512,
			SourceRowLedgerPageCount:          1,
			SourceRecordCount:                 4,
			AcceptedRecordCount:               2,
			RejectedRecordCount:               1,
			DuplicateRecordCount:              1,
		})
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	issueRecord := func(
		manifest domainsecurity.DatasetSnapshotManifestV2,
		producer domainsecurity.FundsProducerContentManifestV1,
		acceptedAt time.Time,
		predecessor string,
	) domainsecurity.DatasetSnapshotAuthorityRecordV2 {
		var record domainsecurity.DatasetSnapshotAuthorityRecordV2
		err := manifest.WithExactFundsProducerAuthorityAdmissionV2(
			producer,
			func(capability domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2) error {
				var issueErr error
				record, issueErr = capability.Issue(domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
					InstallationID:          installationID,
					AcceptedAt:              acceptedAt,
					PredecessorRecordDigest: predecessor,
					AuthorityKeyID:          authorityKeyID,
					AuthorityPublicKey:      authorityPublicKey,
				}, sign)
				return issueErr
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	previousProducer := newProducer("fact-final-dataset-previous", 1)
	previousManifest := newManifest("fact-final-dataset-previous", now.Add(-2*time.Minute), previousProducer)
	previousRecord := issueRecord(previousManifest, previousProducer, now.Add(-time.Minute), "")
	previousIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID:       installationID,
		EnrollmentID:         enrollmentID,
		Generation:           1,
		PreviousIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:           domainsecurity.SHA256Hex([]byte("fact-final-dataset-index-1")),
		Binding:              binding,
		SnapshotRecordDigest: previousRecord.RecordDigest,
		AuthorityKeyID:       authorityKeyID,
		AuthorityPublicKey:   authorityPublicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	producer := newProducer("fact-final-dataset-selected", 2)
	manifest := newManifest("fact-final-dataset-selected", now.Add(-30*time.Second), producer)
	record := issueRecord(manifest, producer, now.Add(-15*time.Second), previousRecord.RecordDigest)
	selectedIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID:       installationID,
		EnrollmentID:         enrollmentID,
		Generation:           2,
		PreviousIndexDigest:  previousIndex.IndexDigest,
		MutationID:           domainsecurity.SHA256Hex([]byte("fact-final-dataset-index-2")),
		Binding:              binding,
		SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID:       authorityKeyID,
		AuthorityPublicKey:   authorityPublicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	policyDigest := domainsecurity.SHA256Hex([]byte("fact-final-risk-policy:\x00" + threadID))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   policyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding(threadID, workspace, domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID:             threadID,
		TurnID:               turnID,
		WorkspaceRealPath:    workspace,
		TenantID:             domainsecurity.LocalTenantID,
		UserID:               domainsecurity.LocalUserID,
		CaseID:               caseID,
		CaseBindingHash:      observation.CaseBindingHash,
		DatasetSnapshotID:    record.DatasetSnapshotID,
		SourceManifestHash:   manifest.SourceManifestHash,
		ContextEpoch:         epoch,
		IssuedAt:             now,
		PublicationPolicy:    policy,
		RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return factFinalDatasetAuthorityFixture{
		context:          securityContext,
		observation:      observation,
		rootIndex:        selectedIndex,
		selectedIndex:    selectedIndex,
		indexPath:        []domainsecurity.DatasetSnapshotIndexV1{selectedIndex, previousIndex},
		record:           record,
		manifest:         manifest,
		producer:         producer,
		previousIndex:    previousIndex,
		previousRecord:   previousRecord,
		previousManifest: previousManifest,
		previousProducer: previousProducer,
	}
}
