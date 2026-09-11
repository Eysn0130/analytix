package security

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

type datasetSnapshotIndexTestFixture struct {
	keys         datasetSnapshotAuthorityTestKeys
	enrollmentID string
	observation  CaseBindingObservationV1
	record       DatasetSnapshotAuthorityRecordV1
	index        DatasetSnapshotIndexV1
}

func newDatasetSnapshotIndexTestFixture(t *testing.T) datasetSnapshotIndexTestFixture {
	t.Helper()
	keys := newDatasetSnapshotAuthorityTestKeys(t, "dataset-index-installation")
	enrollmentID := SHA256Hex([]byte("dataset-index-enrollment"))
	observation := datasetSnapshotAuthorityTestObservation(t)
	record := newDatasetSnapshotAuthorityTestRecord(
		t, keys, observation, "dataset-index-snapshot-a", "", time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC),
	)
	binding, err := DatasetSnapshotBindingKeyFromRecordV1(record)
	if err != nil {
		t.Fatal(err)
	}
	index, err := NewDatasetSnapshotIndexV1(DatasetSnapshotIndexInputV1{
		InstallationID: keys.installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          SHA256Hex([]byte("dataset-index-mutation-1")),
		Binding:             binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	return datasetSnapshotIndexTestFixture{
		keys: keys, enrollmentID: enrollmentID, observation: observation, record: record, index: index,
	}
}

func TestDatasetSnapshotIndexV1CanonicalRoundTripAndExactAnchors(t *testing.T) {
	fixture := newDatasetSnapshotIndexTestFixture(t)
	body, err := DatasetSnapshotIndexV1Bytes(fixture.index)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDatasetSnapshotIndexV1(body)
	if err != nil || parsed != fixture.index {
		t.Fatalf("dataset snapshot index canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	if err := ValidateDatasetSnapshotIndexForInstallationV1(
		fixture.index, fixture.keys.installationID, fixture.enrollmentID, fixture.keys.keyID, fixture.keys.publicKey,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotIndexRecordForInstallationV1(
		fixture.index, fixture.record, fixture.keys.installationID, fixture.enrollmentID,
		fixture.keys.keyID, fixture.keys.publicKey,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotBindingKeyForObservationV1(
		fixture.index.Binding, fixture.record.TenantID, fixture.record.UserID, fixture.observation,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotSelectionV1(
		fixture.index, fixture.record, fixture.record.TenantID, fixture.record.UserID, fixture.observation,
		fixture.keys.installationID, fixture.enrollmentID, fixture.keys.keyID, fixture.keys.publicKey,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotIndexWitnessRootV1(fixture.index, fixture.index.IndexDigest, 1); err != nil {
		t.Fatal(err)
	}
	if fixture.index.IndexDigest == fixture.index.SnapshotRecordDigest ||
		fixture.index.IndexDigest == fixture.index.PreviousIndexDigest {
		t.Fatalf("index content address aliases another authority digest: %#v", fixture.index)
	}

	attacker := newDatasetSnapshotAuthorityTestKeys(t, "dataset-index-installation")
	attacker.installationID = fixture.keys.installationID
	attackerRecord := newDatasetSnapshotAuthorityTestRecord(
		t, attacker, fixture.observation, "dataset-index-snapshot-a", "", time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC),
	)
	attackerBinding, err := DatasetSnapshotBindingKeyFromRecordV1(attackerRecord)
	if err != nil {
		t.Fatal(err)
	}
	attackerIndex, err := NewDatasetSnapshotIndexV1(DatasetSnapshotIndexInputV1{
		InstallationID: fixture.keys.installationID, EnrollmentID: fixture.enrollmentID, Generation: 1,
		PreviousIndexDigest: DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          SHA256Hex([]byte("dataset-index-attacker-mutation")),
		Binding:             attackerBinding, SnapshotRecordDigest: attackerRecord.RecordDigest,
		AuthorityKeyID: attacker.keyID, AuthorityPublicKey: attacker.publicKey,
	}, attacker.sign)
	if err != nil || ValidateDatasetSnapshotIndexV1(attackerIndex) != nil {
		t.Fatalf("self-signed attacker control is not structurally valid: index=%#v err=%v", attackerIndex, err)
	}
	if ValidateDatasetSnapshotIndexForInstallationV1(
		attackerIndex, fixture.keys.installationID, fixture.enrollmentID, fixture.keys.keyID, fixture.keys.publicKey,
	) == nil {
		t.Fatal("self-signed attacker index was accepted as installation authority")
	}
	if ValidateDatasetSnapshotIndexForInstallationV1(
		fixture.index, fixture.keys.installationID, SHA256Hex([]byte("other-enrollment")),
		fixture.keys.keyID, fixture.keys.publicKey,
	) == nil {
		t.Fatal("dataset snapshot index was accepted for another witness enrollment")
	}
}

func TestDatasetSnapshotIndexV1IsGlobalAppendLineageNotASelectionClaim(t *testing.T) {
	fixture := newDatasetSnapshotIndexTestFixture(t)
	otherObservation, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/case-b", State: CaseBindingStateValid, CaseID: "case-b",
		BindingSHA256: SHA256Hex([]byte("binding-file-b")), CaseBindingHash: SHA256Hex([]byte("binding-canonical-b")),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRecord := newDatasetSnapshotAuthorityTestRecord(
		t, fixture.keys, otherObservation, "dataset-index-snapshot-b", "", time.Date(2026, 7, 13, 1, 1, 0, 0, time.UTC),
	)
	otherBinding, err := DatasetSnapshotBindingKeyFromRecordV1(otherRecord)
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewDatasetSnapshotIndexV1(DatasetSnapshotIndexInputV1{
		InstallationID: fixture.keys.installationID, EnrollmentID: fixture.enrollmentID, Generation: 2,
		PreviousIndexDigest: fixture.index.IndexDigest,
		MutationID:          SHA256Hex([]byte("dataset-index-mutation-2")),
		Binding:             otherBinding, SnapshotRecordDigest: otherRecord.RecordDigest,
		AuthorityKeyID: fixture.keys.keyID, AuthorityPublicKey: fixture.keys.publicKey,
	}, fixture.keys.sign)
	if err != nil || ValidateDatasetSnapshotIndexTransitionV1(fixture.index, next) != nil {
		t.Fatalf("global append across exact binding scopes failed: next=%#v err=%v", next, err)
	}
	if err := ValidateDatasetSnapshotIndexRecordV1(next, otherRecord); err != nil {
		t.Fatal(err)
	}

	wrongPredecessor := next
	wrongPredecessor.PreviousIndexDigest = SHA256Hex([]byte("dataset-index-other-predecessor"))
	wrongPredecessor = resignDatasetSnapshotIndexForTest(t, fixture.keys, wrongPredecessor)
	if ValidateDatasetSnapshotIndexV1(wrongPredecessor) != nil {
		t.Fatal("wrong-predecessor control is not a structurally valid signed index")
	}
	if ValidateDatasetSnapshotIndexTransitionV1(fixture.index, wrongPredecessor) == nil {
		t.Fatal("index generation skipped its exact predecessor")
	}
	wrongGeneration := next
	wrongGeneration.Generation++
	wrongGeneration = resignDatasetSnapshotIndexForTest(t, fixture.keys, wrongGeneration)
	if ValidateDatasetSnapshotIndexTransitionV1(fixture.index, wrongGeneration) == nil {
		t.Fatal("index generation skip was accepted")
	}
	replayedRecord, err := NewDatasetSnapshotIndexV1(DatasetSnapshotIndexInputV1{
		InstallationID: fixture.keys.installationID, EnrollmentID: fixture.enrollmentID, Generation: 2,
		PreviousIndexDigest: fixture.index.IndexDigest,
		MutationID:          SHA256Hex([]byte("dataset-index-replayed-record")),
		Binding:             fixture.index.Binding, SnapshotRecordDigest: fixture.index.SnapshotRecordDigest,
		AuthorityKeyID: fixture.keys.keyID, AuthorityPublicKey: fixture.keys.publicKey,
	}, fixture.keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateDatasetSnapshotIndexTransitionV1(fixture.index, replayedRecord) == nil {
		t.Fatal("the same snapshot record was appended twice")
	}
}

func TestDatasetSnapshotIndexV1RejectsAmbiguousTamperedAndMismatchedInput(t *testing.T) {
	fixture := newDatasetSnapshotIndexTestFixture(t)
	body, err := DatasetSnapshotIndexV1Bytes(fixture.index)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":2`), 1)
	if _, err := ParseDatasetSnapshotIndexV1(duplicate); err == nil {
		t.Fatal("duplicate index property was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["current"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseDatasetSnapshotIndexV1(unknown); err == nil {
		t.Fatal("unknown current-selection property was accepted")
	}
	if _, err := ParseDatasetSnapshotIndexV1(append(append([]byte(nil), body...), []byte(` {}`)...)); err == nil {
		t.Fatal("trailing index JSON was accepted")
	}
	if _, err := ParseDatasetSnapshotIndexV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("non-canonical index whitespace was accepted")
	}

	tampered := fixture.index
	tampered.SnapshotRecordDigest = SHA256Hex([]byte("other-record"))
	if ValidateDatasetSnapshotIndexV1(tampered) == nil {
		t.Fatal("tampered snapshot record digest retained index authority")
	}
	otherObservation, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: fixture.observation.WorkspaceRealPath, State: CaseBindingStateValid, CaseID: fixture.observation.CaseID,
		BindingSHA256: SHA256Hex([]byte("other-binding-file")), CaseBindingHash: fixture.observation.CaseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ValidateDatasetSnapshotBindingKeyForObservationV1(
		fixture.index.Binding, fixture.record.TenantID, fixture.record.UserID, otherObservation,
	) == nil {
		t.Fatal("dataset snapshot binding key accepted another observation digest")
	}
	if ValidateDatasetSnapshotBindingKeyForObservationV1(
		fixture.index.Binding, fixture.record.TenantID, "other-user", fixture.observation,
	) == nil {
		t.Fatal("dataset snapshot binding key crossed user authority")
	}
	otherRecord := newDatasetSnapshotAuthorityTestRecord(
		t, fixture.keys, fixture.observation, "other-record", "", time.Date(2026, 7, 13, 1, 2, 0, 0, time.UTC),
	)
	if ValidateDatasetSnapshotIndexRecordV1(fixture.index, otherRecord) == nil {
		t.Fatal("index was rebound to an unreferenced authority record")
	}
}

func resignDatasetSnapshotIndexForTest(t *testing.T, keys datasetSnapshotAuthorityTestKeys, index DatasetSnapshotIndexV1) DatasetSnapshotIndexV1 {
	t.Helper()
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	signature, err := keys.sign(DatasetSnapshotIndexSigningBytesV1(index))
	if err != nil {
		t.Fatal(err)
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = datasetSnapshotIndexDigestV1(index)
	return index
}
