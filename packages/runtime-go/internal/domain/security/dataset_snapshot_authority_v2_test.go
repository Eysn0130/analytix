package security

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDatasetSnapshotManifestV2CanonicalRoundTripAndContentIdentity(t *testing.T) {
	input := datasetSnapshotManifestInputV2ForTest(t, datasetSnapshotAuthorityTestObservation(t), "baseline")
	manifest, err := NewDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	if DeriveDatasetSnapshotIDV2(manifest) != DatasetSnapshotIDPrefixV2+manifest.ManifestDigest {
		t.Fatal("dataset snapshot v2 id was not the exact manifest content identity")
	}
	if err := ValidateDatasetSnapshotManifestV2FundsProducerContentV1(manifest, input.FundsProducerContentManifest); err != nil {
		t.Fatal(err)
	}
	producerBody, _ := FundsProducerContentManifestV1Bytes(input.FundsProducerContentManifest)
	if manifest.ProducerContentContract != FundsProducerContentManifestContractV1 ||
		manifest.ProducerContentID != DeriveFundsProducerContentIDV1(input.FundsProducerContentManifest) ||
		manifest.ProducerContentManifestSHA256 != SHA256Hex(producerBody) ||
		manifest.ProducerContentManifestByteLength != uint64(len(producerBody)) ||
		manifest.ProducerContentComponentID != FundsProducerComponentIDV1 ||
		manifest.ProducerContentComponentVersion != FundsProducerComponentVersionV1 ||
		manifest.ProducerContentOperation != FundsProducerOperationV1 ||
		manifest.ProducerContentOperationSchemaHash != FundsProducerOperationSchemaHashV1 ||
		manifest.ProducerContentEngine != FundsProducerDuckDBVersionV1 ||
		manifest.ProducerContentEncoder != FundsProducerCanonicalEncoderV1 {
		t.Fatalf("dataset snapshot omitted exact producer content binding: %#v", manifest)
	}
	body, err := DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDatasetSnapshotManifestV2(body)
	if err != nil || parsed != manifest {
		t.Fatalf("manifest canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	sha256, err := DatasetSnapshotManifestV2SHA256(manifest)
	if err != nil || sha256 != SHA256Hex(body) {
		t.Fatalf("manifest exact bytes were not hashed: sha=%q err=%v", sha256, err)
	}

	rowMutationInput := input
	rowMutationInput.SourceRowLedgerRootDigest = SHA256Hex([]byte("row-root-mutated"))
	rowMutationInput.SourceRowLedgerRootSHA256 = SHA256Hex([]byte("row-root-mutated-bytes"))
	rowMutation, err := NewDatasetSnapshotManifestV2(rowMutationInput)
	if err != nil {
		t.Fatal(err)
	}
	if rowMutation.SourceManifestHash != manifest.SourceManifestHash || rowMutation.ManifestDigest == manifest.ManifestDigest ||
		DeriveDatasetSnapshotIDV2(rowMutation) == DeriveDatasetSnapshotIDV2(manifest) {
		t.Fatal("row content root failed to change the snapshot without changing stable source provenance")
	}
	producerMutationInput := input
	producerMutationInput.FundsProducerContentManifest = fundsProducerContentManifestV1ForSnapshotTest(
		t, input.Binding.CaseID, "producer-mutated",
	)
	producerMutationInput.FundsProducerContentManifest.RawManifestSHA256 =
		producerMutationInput.RawArtifactManifestSHA256
	producerMutation, err := NewDatasetSnapshotManifestV2(producerMutationInput)
	if err != nil {
		t.Fatal(err)
	}
	if producerMutation.ProducerContentID == manifest.ProducerContentID ||
		producerMutation.SourceManifestHash == manifest.SourceManifestHash ||
		producerMutation.ManifestDigest == manifest.ManifestDigest {
		t.Fatal("producer content mutation reused source or snapshot identity")
	}
	if ValidateDatasetSnapshotManifestV2FundsProducerContentV1(producerMutation, input.FundsProducerContentManifest) == nil {
		t.Fatal("a valid fpc1_ identity for different content was accepted")
	}

	for label, mutate := range map[string]func(*DatasetSnapshotManifestInputV2){
		"raw artifact": func(value *DatasetSnapshotManifestInputV2) {
			value.RawArtifactManifestDigest = SHA256Hex([]byte("raw-mutated"))
		},
		"producer policy": func(value *DatasetSnapshotManifestInputV2) {
			value.ProducerPolicyDigest = SHA256Hex([]byte("policy-mutated"))
		},
		"operation schema": func(value *DatasetSnapshotManifestInputV2) {
			value.ProducerOperationSchemaHash = SHA256Hex([]byte("schema-mutated"))
		},
		"parser": func(value *DatasetSnapshotManifestInputV2) {
			value.ParserVersion = "analytix.funds.transaction-row-parser/v2"
		},
		"parsed generation": func(value *DatasetSnapshotManifestInputV2) {
			value.ParsedGenerationReceiptDigest = SHA256Hex([]byte("generation-mutated"))
		},
	} {
		changedInput := input
		mutate(&changedInput)
		changed, err := NewDatasetSnapshotManifestV2(changedInput)
		if err != nil {
			t.Fatalf("%s mutation failed to build: %v", label, err)
		}
		if changed.SourceManifestHash == manifest.SourceManifestHash || changed.ManifestDigest == manifest.ManifestDigest {
			t.Fatalf("%s mutation reused source or snapshot identity", label)
		}
	}

	otherObservation, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: input.Binding.WorkspaceRealPath, State: CaseBindingStateValid, CaseID: "case-b",
		BindingSHA256: SHA256Hex([]byte("binding-file-b")), CaseBindingHash: SHA256Hex([]byte("binding-case-b")),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherInput := datasetSnapshotManifestInputV2ForTest(t, otherObservation, "baseline")
	other, err := NewDatasetSnapshotManifestV2(otherInput)
	if err != nil {
		t.Fatal(err)
	}
	if other.SourceManifestHash == manifest.SourceManifestHash || other.ManifestDigest == manifest.ManifestDigest {
		t.Fatal("identical content reused a snapshot identity across case bindings")
	}
}

func TestDatasetSnapshotManifestV2AcceptsOnlyExactGoCountProducerV2(t *testing.T) {
	input := datasetSnapshotManifestInputV2ForTest(t, datasetSnapshotAuthorityTestObservation(t), "go-count-v2")
	input.FundsProducerContentManifest = FundsProducerContentManifestV1{}
	producer, err := NewFundsProducerContentManifestV2(FundsProducerContentManifestInputV2{
		CaseID:                    input.Binding.CaseID,
		SourceRevision:            1,
		RawArtifactManifestSHA256: input.RawArtifactManifestSHA256,
		NormalizedContentSHA256:   SHA256Hex([]byte("go-count-v2-normalized")),
		DetailContentSHA256:       SHA256Hex([]byte("go-count-v2-detail")),
		SourceRowCount:            input.SourceRecordCount,
		AcceptedRowCount:          input.AcceptedRecordCount,
		RejectedRowCount:          input.RejectedRecordCount,
		DuplicateRowCount:         input.DuplicateRecordCount,
		DetailRowCount:            input.AcceptedRecordCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewDatasetSnapshotManifestForFundsProducerContentV2(input, producer)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, producer); err != nil {
		t.Fatal(err)
	}
	body, _ := FundsProducerContentManifestV2Bytes(producer)
	if manifest.ProducerContentContract != FundsProducerContentManifestContractV2 ||
		manifest.ProducerContentID != DeriveFundsProducerContentIDV2(producer) ||
		manifest.ProducerContentManifestSHA256 != SHA256Hex(body) ||
		manifest.ProducerContentComponentID != FundsProducerComponentIDV2 ||
		manifest.ProducerContentEngine != FundsProducerEngineV2 {
		t.Fatalf("dataset snapshot omitted exact Go producer v2 binding: %#v", manifest)
	}
	if _, err := ParseDatasetSnapshotManifestV2(mustDatasetSnapshotManifestV2Bytes(t, manifest)); err != nil {
		t.Fatal(err)
	}
	keys := newDatasetSnapshotAuthorityTestKeys(t, "go-count-v2")
	var record DatasetSnapshotAuthorityRecordV2
	err = manifest.WithExactFundsProducerContentV2AuthorityAdmissionV2(
		producer,
		func(issuer DatasetSnapshotAuthoritySealedAdmissionV2) error {
			var issueErr error
			record, issueErr = issuer.Issue(DatasetSnapshotAuthoritySealedIssueInputV2{
				InstallationID:     keys.installationID,
				AcceptedAt:         time.Date(2026, 7, 18, 8, 31, 0, 0, time.UTC),
				AuthorityKeyID:     keys.keyID,
				AuthorityPublicKey: keys.publicKey,
			}, keys.sign)
			return issueErr
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentManifestV2(
		record,
		manifest,
		producer,
	); err != nil {
		t.Fatal(err)
	}

	mismatchedRaw := producer
	mismatchedRaw.RawArtifactManifestSHA256 = SHA256Hex([]byte("another-raw-artifact-manifest"))
	if ValidateFundsProducerContentManifestV2(mismatchedRaw) != nil {
		t.Fatal("raw-mismatch producer fixture is not independently valid")
	}
	if ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, mismatchedRaw) == nil {
		t.Fatal("producer v2 for a different raw-artifact graph was accepted")
	}
	ambiguous := input
	ambiguous.FundsProducerContentManifest = fundsProducerContentManifestV1ForSnapshotTest(
		t, input.Binding.CaseID, "ambiguous",
	)
	ambiguous.FundsProducerContentManifest.RawManifestSHA256 = input.RawArtifactManifestSHA256
	if _, err := NewDatasetSnapshotManifestForFundsProducerContentV2(ambiguous, producer); err == nil {
		t.Fatal("two producer manifest variants were accepted")
	}
}

func TestDatasetSnapshotManifestV2RepresentsExactZeroRowGoCountSnapshot(t *testing.T) {
	input := datasetSnapshotManifestInputV2ForTest(t, datasetSnapshotAuthorityTestObservation(t), "go-count-v2-empty")
	input.FundsProducerContentManifest = FundsProducerContentManifestV1{}
	input.SourceRowLedgerPageCount = 0
	input.SourceRecordCount = 0
	input.AcceptedRecordCount = 0
	input.RejectedRecordCount = 0
	input.DuplicateRecordCount = 0
	producer, err := NewFundsProducerContentManifestV2(FundsProducerContentManifestInputV2{
		CaseID:                    input.Binding.CaseID,
		SourceRevision:            1,
		RawArtifactManifestSHA256: input.RawArtifactManifestSHA256,
		NormalizedContentSHA256:   SHA256Hex([]byte("go-count-v2-empty-normalized")),
		DetailContentSHA256:       SHA256Hex([]byte("go-count-v2-empty-detail")),
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewDatasetSnapshotManifestForFundsProducerContentV2(input, producer)
	if err != nil || ValidateDatasetSnapshotManifestV2FundsProducerContentV2(manifest, producer) != nil {
		t.Fatalf("exact zero-row Go snapshot was rejected: manifest=%#v err=%v", manifest, err)
	}
	for label, mutate := range map[string]func(*DatasetSnapshotManifestInputV2){
		"page without row": func(value *DatasetSnapshotManifestInputV2) {
			value.SourceRowLedgerPageCount = 1
		},
		"row without page": func(value *DatasetSnapshotManifestInputV2) {
			value.SourceRecordCount = 1
			value.RejectedRecordCount = 1
		},
	} {
		changed := input
		mutate(&changed)
		if _, err := NewDatasetSnapshotManifestForFundsProducerContentV2(changed, producer); err == nil {
			t.Fatalf("inconsistent empty hierarchy %q was accepted", label)
		}
	}
}

func TestDatasetSnapshotManifestV2RejectsAmbiguousUntrustedAndIncompleteInput(t *testing.T) {
	input := datasetSnapshotManifestInputV2ForTest(t, datasetSnapshotAuthorityTestObservation(t), "strict")
	manifest, err := NewDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := DatasetSnapshotManifestV2Bytes(manifest)
	duplicate := bytes.Replace(body, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":2,"schemaVersion":2`), 1)
	if _, err := ParseDatasetSnapshotManifestV2(duplicate); err == nil {
		t.Fatal("duplicate manifest property was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["serverIdentity"] = "attacker"
	unknown, _ := json.Marshal(object)
	if _, err := ParseDatasetSnapshotManifestV2(unknown); err == nil {
		t.Fatal("unknown transport authority entered the snapshot manifest")
	}
	if _, err := ParseDatasetSnapshotManifestV2(append(body, []byte(" ")...)); err == nil {
		t.Fatal("noncanonical manifest whitespace was accepted")
	}

	badClassifications := input
	badClassifications.AcceptedRecordCount = badClassifications.SourceRecordCount
	badClassifications.RejectedRecordCount = 1
	if _, err := NewDatasetSnapshotManifestV2(badClassifications); err == nil {
		t.Fatal("inconsistent accepted/rejected/duplicate counts were accepted")
	}
	formatControl := input
	formatControl.AcquisitionMethod = "host\u200bimport"
	if _, err := NewDatasetSnapshotManifestV2(formatControl); err == nil {
		t.Fatal("Unicode format control entered a manifest identity")
	}
	oversized := input
	oversized.SourceRowLedgerRootByteLength = maxDatasetSnapshotSourceRowLedgerRootBytesV2 + 1
	if _, err := NewDatasetSnapshotManifestV2(oversized); err == nil {
		t.Fatal("oversized referenced manifest entered a snapshot")
	}
	tampered := manifest
	tampered.SourceManifestHash = SHA256Hex([]byte("caller-override"))
	tampered.ManifestDigest = datasetSnapshotManifestDigestV2(tampered)
	if ValidateDatasetSnapshotManifestV2(tampered) == nil {
		t.Fatal("caller-supplied source manifest hash overrode host derivation")
	}
	otherProducer := fundsProducerContentManifestV1ForSnapshotTest(t, input.Binding.CaseID, "other-producer")
	tamperedReference := manifest
	tamperedReference.ProducerContentID = DeriveFundsProducerContentIDV1(otherProducer)
	tamperedReference.ProducerContentManifestSHA256, _ = FundsProducerContentManifestV1SHA256(otherProducer)
	otherBody, _ := FundsProducerContentManifestV1Bytes(otherProducer)
	tamperedReference.ProducerContentManifestByteLength = uint64(len(otherBody))
	tamperedReference.SourceManifestHash = datasetSnapshotSourceManifestHashV2(tamperedReference)
	tamperedReference.ManifestDigest = datasetSnapshotManifestDigestV2(tamperedReference)
	if ValidateDatasetSnapshotManifestV2(tamperedReference) != nil {
		t.Fatal("closed DSV2 reference fixture was not structurally valid")
	}
	if ValidateDatasetSnapshotManifestV2FundsProducerContentV1(tamperedReference, input.FundsProducerContentManifest) == nil {
		t.Fatal("syntactically valid but content-mismatched fpc1_ reference was accepted")
	}
	wrongCaseInput := input
	wrongCaseInput.FundsProducerContentManifest = fundsProducerContentManifestV1ForSnapshotTest(t, "case-other", "wrong-case")
	if _, err := NewDatasetSnapshotManifestV2(wrongCaseInput); err == nil {
		t.Fatal("cross-case funds producer content entered a dataset snapshot")
	}
	wrongCoverageInput := input
	wrongCoverageInput.FundsProducerContentManifest.NormalizedRowCount++
	wrongCoverageInput.FundsProducerContentManifest.DuplicateRowCount++
	if ValidateFundsProducerContentManifestV1(wrongCoverageInput.FundsProducerContentManifest) != nil {
		t.Fatal("producer coverage mismatch fixture was not independently valid")
	}
	if _, err := NewDatasetSnapshotManifestV2(wrongCoverageInput); err == nil {
		t.Fatal("producer acceptance-state drift entered a dataset snapshot")
	}
	rawMismatchInput := input
	rawMismatchInput.FundsProducerContentManifest.RawManifestSHA256 = SHA256Hex([]byte("different-raw-graph"))
	if ValidateFundsProducerContentManifestV1(rawMismatchInput.FundsProducerContentManifest) != nil {
		t.Fatal("legacy producer raw-mismatch fixture was not independently valid")
	}
	if _, err := NewDatasetSnapshotManifestV2(rawMismatchInput); err == nil {
		t.Fatal("legacy producer for a different raw-artifact graph entered a dataset snapshot")
	}
}

func TestDatasetSnapshotAuthoritySealedAdmissionV2IsExactOneUseAndCallbackScoped(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "snapshot-v2-sealed-admission")
	input := datasetSnapshotManifestInputV2ForTest(t, datasetSnapshotAuthorityTestObservation(t), "sealed")
	manifest, err := NewDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	issueInput := DatasetSnapshotAuthoritySealedIssueInputV2{
		InstallationID:     keys.installationID,
		AcceptedAt:         time.Date(2026, 7, 18, 8, 30, 0, 0, time.UTC),
		AuthorityKeyID:     keys.keyID,
		AuthorityPublicKey: keys.publicKey,
	}
	var escaped DatasetSnapshotAuthoritySealedAdmissionV2
	if err := manifest.WithExactFundsProducerAuthorityAdmissionV2(input.FundsProducerContentManifest, func(
		issuer DatasetSnapshotAuthoritySealedAdmissionV2,
	) error {
		escaped = issuer
		record, issueErr := issuer.Issue(issueInput, keys.sign)
		if issueErr != nil {
			return issueErr
		}
		if record.DatasetSnapshotID != DeriveDatasetSnapshotIDV2(manifest) {
			t.Fatal("sealed issuer did not preserve its exact captured manifest")
		}
		if _, secondErr := issuer.Issue(issueInput, keys.sign); secondErr == nil {
			t.Fatal("sealed issuer minted a second authority record")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := escaped.Issue(issueInput, keys.sign); err == nil {
		t.Fatal("sealed issuer remained active after its callback returned")
	}

	other := fundsProducerContentManifestV1ForSnapshotTest(t, input.Binding.CaseID, "different-sealed-content")
	called := false
	if err := manifest.WithExactFundsProducerAuthorityAdmissionV2(other, func(DatasetSnapshotAuthoritySealedAdmissionV2) error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatal("mismatched producer content received a sealed issuer")
	}
}

func TestDatasetSnapshotAuthorityRecordV2BindsManifestInstallationIndexAndCase(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "snapshot-v2-installation")
	observation := datasetSnapshotAuthorityTestObservation(t)
	manifest, err := NewDatasetSnapshotManifestV2(datasetSnapshotManifestInputV2ForTest(t, observation, "selection"))
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	record, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: keys.installationID, Manifest: manifest, AcceptedAt: acceptedAt,
		FundsProducerContent: datasetSnapshotManifestInputV2ForTest(t, observation, "selection").FundsProducerContentManifest,
		AuthorityKeyID:       keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: keys.installationID, Manifest: manifest,
		FundsProducerContent: fundsProducerContentManifestV1ForSnapshotTest(t, observation.CaseID, "wrong-selection"),
		AcceptedAt:           acceptedAt, AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign); err == nil {
		t.Fatal("authority record was signed from a merely well-formed but mismatched fpc1_ payload")
	}
	body, err := DatasetSnapshotAuthorityRecordV2Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDatasetSnapshotAuthorityRecordV2(body)
	if err != nil || parsed != record {
		t.Fatalf("v2 record canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	versioned, err := ParseVersionedDatasetSnapshotAuthorityRecord(body)
	if err != nil || versioned.V2 == nil || *versioned.V2 != record || versioned.V1 != nil {
		t.Fatalf("versioned record parser lost the V2 variant: %#v err=%v", versioned, err)
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForInstallationV2(record, keys.installationID, keys.keyID, keys.publicKey); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForBindingV2(record, "tenant-a", "user-a", observation); err != nil {
		t.Fatal(err)
	}

	enrollmentID := SHA256Hex([]byte("snapshot-v2-enrollment"))
	index, err := NewDatasetSnapshotIndexV1(DatasetSnapshotIndexInputV1{
		InstallationID: keys.installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: DatasetSnapshotIndexGenesisDigestV1(), MutationID: SHA256Hex([]byte("snapshot-v2-index-mutation")),
		Binding: record.Binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotIndexNodeForManifestV2(
		index, record, manifest, datasetSnapshotManifestInputV2ForTest(t, observation, "selection").FundsProducerContentManifest,
		"tenant-a", "user-a", observation,
		keys.installationID, enrollmentID, keys.keyID, keys.publicKey,
	); err != nil {
		t.Fatal(err)
	}
	if DatasetSnapshotFactAuthorityBlocker(record.DatasetSnapshotID) == "" {
		t.Fatal("record/index structure incorrectly made a bare DSV2 string authoritative")
	}

	changedManifestInput := datasetSnapshotManifestInputV2ForTest(t, observation, "other-manifest")
	changedManifest, _ := NewDatasetSnapshotManifestV2(changedManifestInput)
	if ValidateDatasetSnapshotAuthorityRecordForManifestV2(record, changedManifest) == nil {
		t.Fatal("record accepted another canonical manifest")
	}
	attacker := newDatasetSnapshotAuthorityTestKeys(t, "snapshot-v2-installation")
	attacker.installationID = keys.installationID
	attackerRecord, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: attacker.installationID, Manifest: manifest, AcceptedAt: acceptedAt,
		FundsProducerContent: datasetSnapshotManifestInputV2ForTest(t, observation, "selection").FundsProducerContentManifest,
		AuthorityKeyID:       attacker.keyID, AuthorityPublicKey: attacker.publicKey,
	}, attacker.sign)
	if err != nil || ValidateDatasetSnapshotAuthorityRecordV2(attackerRecord) != nil {
		t.Fatalf("self-signed attacker fixture was not structurally valid: %v", err)
	}
	if ValidateDatasetSnapshotAuthorityRecordForInstallationV2(attackerRecord, keys.installationID, keys.keyID, keys.publicKey) == nil {
		t.Fatal("self-signed attacker record matched the installation anchor")
	}
}

func TestDatasetSnapshotAuthorityV1ToV2MigrationAndDowngradeRules(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "snapshot-v1-v2-transition")
	observation := datasetSnapshotAuthorityTestObservation(t)
	start := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	legacy := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "legacy", "", start)
	manifest, err := NewDatasetSnapshotManifestV2(datasetSnapshotManifestInputV2ForTest(t, observation, "migrated"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: keys.installationID, Manifest: manifest, AcceptedAt: start.Add(time.Second),
		FundsProducerContent:    datasetSnapshotManifestInputV2ForTest(t, observation, "migrated").FundsProducerContentManifest,
		PredecessorRecordDigest: legacy.RecordDigest, AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil || ValidateDatasetSnapshotAuthorityTransitionV1ToV2(legacy, first) != nil {
		t.Fatalf("explicit V1 to V2 migration failed: record=%#v err=%v", first, err)
	}
	missingPredecessor, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: keys.installationID, Manifest: manifest, AcceptedAt: start.Add(time.Second),
		FundsProducerContent: datasetSnapshotManifestInputV2ForTest(t, observation, "migrated").FundsProducerContentManifest,
		AuthorityKeyID:       keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateDatasetSnapshotAuthorityTransitionV1ToV2(legacy, missingPredecessor) == nil {
		t.Fatal("V1 to V2 migration without the exact predecessor was accepted")
	}

	nextInput := datasetSnapshotManifestInputV2ForTest(t, observation, "successor")
	nextManifest, _ := NewDatasetSnapshotManifestV2(nextInput)
	next, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: keys.installationID, Manifest: nextManifest, AcceptedAt: start.Add(2 * time.Second),
		FundsProducerContent:    nextInput.FundsProducerContentManifest,
		PredecessorRecordDigest: first.RecordDigest, AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil || ValidateDatasetSnapshotAuthorityTransitionV2(first, next) != nil {
		t.Fatalf("V2 successor failed: record=%#v err=%v", next, err)
	}
	reissued, err := newDatasetSnapshotAuthorityRecordV2(datasetSnapshotAuthorityRecordInputV2{
		InstallationID: keys.installationID, Manifest: manifest, AcceptedAt: start.Add(2 * time.Second),
		FundsProducerContent:    datasetSnapshotManifestInputV2ForTest(t, observation, "migrated").FundsProducerContentManifest,
		PredecessorRecordDigest: first.RecordDigest, AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateDatasetSnapshotAuthorityTransitionV2(first, reissued) == nil {
		t.Fatal("identical manifest was reissued as a successor record")
	}
	legacyVariant := VersionedDatasetSnapshotAuthorityRecord{SchemaVersion: 1, V1: &legacy}
	firstVariant := VersionedDatasetSnapshotAuthorityRecord{SchemaVersion: 2, V2: &first}
	if err := ValidateVersionedDatasetSnapshotAuthorityTransition(legacyVariant, firstVariant); err != nil {
		t.Fatal(err)
	}
	if ValidateVersionedDatasetSnapshotAuthorityTransition(firstVariant, legacyVariant) == nil {
		t.Fatal("V2 to V1 authority downgrade was accepted")
	}
	invalidUnions := []VersionedDatasetSnapshotAuthorityRecord{
		{SchemaVersion: DatasetSnapshotAuthorityRecordSchemaVersion},
		{SchemaVersion: DatasetSnapshotAuthorityRecordSchemaVersionV2},
		{SchemaVersion: DatasetSnapshotAuthorityRecordSchemaVersion, V2: &first},
		{SchemaVersion: DatasetSnapshotAuthorityRecordSchemaVersionV2, V1: &legacy},
		{SchemaVersion: DatasetSnapshotAuthorityRecordSchemaVersion, V1: &legacy, V2: &first},
		{SchemaVersion: 0, V1: &legacy},
		{SchemaVersion: 3, V2: &first},
	}
	for _, invalid := range invalidUnions {
		if ValidateVersionedDatasetSnapshotAuthorityRecord(invalid) == nil {
			t.Fatalf("invalid versioned authority union was accepted: %#v", invalid)
		}
		if ValidateVersionedDatasetSnapshotAuthorityTransition(invalid, firstVariant) == nil {
			t.Fatalf("transition accepted an invalid previous union: %#v", invalid)
		}
		if ValidateVersionedDatasetSnapshotAuthorityTransition(firstVariant, invalid) == nil {
			t.Fatalf("transition accepted an invalid next union: %#v", invalid)
		}
	}
	unknown := bytes.Replace(mustDatasetSnapshotAuthorityV2Bytes(t, first), []byte(`"schemaVersion":2`), []byte(`"schemaVersion":3`), 1)
	if _, err := ParseVersionedDatasetSnapshotAuthorityRecord(unknown); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown authority version did not fail closed: %v", err)
	}
}

func datasetSnapshotManifestInputV2ForTest(
	t *testing.T,
	observation CaseBindingObservationV1,
	seed string,
) DatasetSnapshotManifestInputV2 {
	t.Helper()
	binding, err := NewDatasetSnapshotBindingKeyV1(DatasetSnapshotBindingKeyInputV1{
		TenantID: "tenant-a", UserID: "user-a", WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash,
		BindingObservationDigest: observation.ObservationDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := DatasetSnapshotManifestInputV2{
		Binding:           binding,
		AcquisitionMethod: "host_case_import/v1", AcquiredAt: time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC),
		AcquisitionActorDigest:        SHA256Hex([]byte("actor:" + seed)),
		RawArtifactManifestDigest:     SHA256Hex([]byte("raw-manifest-digest:" + seed)),
		RawArtifactManifestSHA256:     SHA256Hex([]byte("raw-manifest-bytes:" + seed)),
		RawArtifactManifestByteLength: 512, RawArtifactCount: 2,
		FundsProducerContentManifest: fundsProducerContentManifestV1ForSnapshotTest(t, observation.CaseID, seed),
		SourceType:                   "transaction_source_occurrence_projection",
		ProducerPolicyID:             "funds.transaction-source-row-ledger/v1",
		ProducerPolicyDigest:         SHA256Hex([]byte("producer-policy:" + seed)),
		ProducerComponentID:          "data-engine", ProducerComponentVersion: "analytix.data-engine-source-row/v1",
		ProducerOperation:           "funds.transaction_source_row_page_v1",
		ProducerOperationSchemaHash: SHA256Hex([]byte("producer-schema:" + seed)),
		ParserID:                    "analytix.funds.transaction-row-parser", ParserVersion: "analytix.funds.transaction-row-parser/v1",
		ParsedGenerationReceiptDigest:     SHA256Hex([]byte("generation-digest:" + seed)),
		ParsedGenerationReceiptSHA256:     SHA256Hex([]byte("generation-bytes:" + seed)),
		ParsedGenerationReceiptByteLength: 640,
		ClassificationLedgerDigest:        SHA256Hex([]byte("classification-digest:" + seed)),
		ClassificationLedgerSHA256:        SHA256Hex([]byte("classification-bytes:" + seed)),
		ClassificationLedgerByteLength:    384, TimezoneSemantics: "unresolved", CurrencySemantics: "unresolved",
		SourceRowLedgerRootDigest:     SHA256Hex([]byte("row-root-digest:" + seed)),
		SourceRowLedgerRootSHA256:     SHA256Hex([]byte("row-root-bytes:" + seed)),
		SourceRowLedgerRootByteLength: 1024, SourceRowLedgerPageCount: 1,
		SourceRecordCount: 3, AcceptedRecordCount: 2, RejectedRecordCount: 1, DuplicateRecordCount: 0,
	}
	input.FundsProducerContentManifest.RawManifestSHA256 = input.RawArtifactManifestSHA256
	return input
}

func fundsProducerContentManifestV1ForSnapshotTest(
	t *testing.T,
	caseID string,
	seed string,
) FundsProducerContentManifestV1 {
	t.Helper()
	manifest, err := NewFundsProducerContentManifestV1(FundsProducerContentManifestInputV1{
		CaseID: caseID, SourceRevision: 1,
		RawManifestSHA256:       SHA256Hex([]byte("funds-raw:" + seed)),
		NormalizedContentSHA256: SHA256Hex([]byte("funds-normalized:" + seed)),
		DetailContentSHA256:     SHA256Hex([]byte("funds-detail:" + seed)),
		AggregateContentSHA256:  SHA256Hex([]byte("funds-aggregate:" + seed)),
		KeywordContentSHA256:    SHA256Hex([]byte("funds-keyword:" + seed)),
		AccountContentSHA256:    SHA256Hex([]byte("funds-account:" + seed)),
		NormalizedRowCount:      3, AcceptedRowCount: 2, RejectedRowCount: 1, DuplicateRowCount: 0,
		DetailRowCount: 2, AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func mustDatasetSnapshotAuthorityV2Bytes(t *testing.T, record DatasetSnapshotAuthorityRecordV2) []byte {
	t.Helper()
	body, err := DatasetSnapshotAuthorityRecordV2Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mustDatasetSnapshotManifestV2Bytes(t *testing.T, manifest DatasetSnapshotManifestV2) []byte {
	t.Helper()
	body, err := DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
