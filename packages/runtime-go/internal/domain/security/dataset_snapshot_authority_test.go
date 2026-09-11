package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"
)

type datasetSnapshotAuthorityTestKeys struct {
	privateKey     ed25519.PrivateKey
	publicKey      ed25519.PublicKey
	keyID          string
	installationID string
}

func newDatasetSnapshotAuthorityTestKeys(t *testing.T, installation string) datasetSnapshotAuthorityTestKeys {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return datasetSnapshotAuthorityTestKeys{
		privateKey: privateKey, publicKey: publicKey, keyID: SHA256Hex(publicKey), installationID: SHA256Hex([]byte(installation)),
	}
}

func (keys datasetSnapshotAuthorityTestKeys) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(keys.privateKey, message), nil
}

func datasetSnapshotAuthorityTestObservation(t *testing.T) CaseBindingObservationV1 {
	t.Helper()
	observation, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/case-a", State: CaseBindingStateValid, CaseID: "case-a",
		BindingSHA256: SHA256Hex([]byte("binding-file")), CaseBindingHash: SHA256Hex([]byte("binding-canonical")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func newDatasetSnapshotAuthorityTestRecord(t *testing.T, keys datasetSnapshotAuthorityTestKeys, observation CaseBindingObservationV1, snapshotMaterial string, predecessor string, acceptedAt time.Time) DatasetSnapshotAuthorityRecordV1 {
	t.Helper()
	input := DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: keys.installationID, TenantID: "tenant-a", UserID: "user-a",
		WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID,
		CaseBindingHash: observation.CaseBindingHash, BindingObservationDigest: observation.ObservationDigest,
		SourceManifestHash: SHA256Hex([]byte("source-" + snapshotMaterial)),
		RawManifestSHA256:  SHA256Hex([]byte("raw-" + snapshotMaterial)), ParserVersion: "fund-parser/1.0.0",
		AcceptedAt: acceptedAt, PredecessorRecordDigest: predecessor,
		AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}
	input.DatasetSnapshotID, _ = DeriveDatasetSnapshotIDV1(input)
	record, err := NewDatasetSnapshotAuthorityRecordV1(input, keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestDatasetSnapshotAuthorityRecordV1CanonicalRoundTripAndInstallationAnchor(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "installation-a")
	observation := datasetSnapshotAuthorityTestObservation(t)
	record := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "snapshot-a", "", time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC))
	body, err := DatasetSnapshotAuthorityRecordV1Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"rawManifestSHA256":"`)) {
		t.Fatalf("raw manifest property name is not canonical: %s", body)
	}
	parsed, err := ParseDatasetSnapshotAuthorityRecordV1(body)
	if err != nil || parsed != record {
		t.Fatalf("canonical round trip failed: parsed=%+v err=%v", parsed, err)
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForInstallationV1(record, keys.installationID, keys.keyID, keys.publicKey); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDatasetSnapshotAuthorityRecordForBindingV1(record, observation); err != nil {
		t.Fatal(err)
	}
	attacker := newDatasetSnapshotAuthorityTestKeys(t, "installation-a")
	attacker.installationID = keys.installationID
	attackerRecord := newDatasetSnapshotAuthorityTestRecord(t, attacker, observation, "snapshot-a", "", time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC))
	if ValidateDatasetSnapshotAuthorityRecordV1(attackerRecord) != nil {
		t.Fatal("attacker record should be authentic under its own key")
	}
	if ValidateDatasetSnapshotAuthorityRecordForInstallationV1(attackerRecord, keys.installationID, keys.keyID, keys.publicKey) == nil {
		t.Fatal("self-signed attacker record was accepted as installation authority")
	}
}

func TestDatasetSnapshotAuthorityRecordV1RejectsAmbiguousOrNonCanonicalJSON(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "installation-canonical")
	record := newDatasetSnapshotAuthorityTestRecord(t, keys, datasetSnapshotAuthorityTestObservation(t), "snapshot-canonical", "", time.Date(2026, 7, 12, 3, 0, 0, 0, time.UTC))
	body, _ := DatasetSnapshotAuthorityRecordV1Bytes(record)

	duplicate := bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":2`), 1)
	if _, err := ParseDatasetSnapshotAuthorityRecordV1(duplicate); err == nil {
		t.Fatal("duplicate record property was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["safeToAnswer"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseDatasetSnapshotAuthorityRecordV1(unknown); err == nil {
		t.Fatal("unknown record property was accepted")
	}
	if _, err := ParseDatasetSnapshotAuthorityRecordV1(append(body, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	if _, err := ParseDatasetSnapshotAuthorityRecordV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("non-canonical whitespace was accepted")
	}
	tampered := record
	tampered.DatasetSnapshotID = DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("other"))
	if ValidateDatasetSnapshotAuthorityRecordV1(tampered) == nil {
		t.Fatal("tampered snapshot authority record validated")
	}
	invalidID := record
	invalidID.DatasetSnapshotID = "dsv1_ABCDEF"
	if ValidateDatasetSnapshotAuthorityRecordV1(invalidID) == nil {
		t.Fatal("truncated or uppercase snapshot id validated")
	}
}

func TestDatasetSnapshotAuthorityRecordRequiresExactBindingObservation(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "installation-binding")
	observation := datasetSnapshotAuthorityTestObservation(t)
	record := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "snapshot-binding", "", time.Date(2026, 7, 12, 4, 0, 0, 0, time.UTC))

	other, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: observation.WorkspaceRealPath, State: CaseBindingStateValid, CaseID: observation.CaseID,
		BindingSHA256: SHA256Hex([]byte("other-binding-file")), CaseBindingHash: observation.CaseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if DatasetSnapshotAuthorityRecordMatchesBindingV1(record, other) {
		t.Fatal("record matched a different binding observation digest")
	}
	missing, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: observation.WorkspaceRealPath, State: CaseBindingStateMissing,
	})
	if err != nil {
		t.Fatal(err)
	}
	if DatasetSnapshotAuthorityRecordMatchesBindingV1(record, missing) {
		t.Fatal("record matched a non-authoritative missing observation")
	}
}

func TestDatasetSnapshotIDIsHostDerivedAndCaseBound(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "installation-derived-id")
	observation := datasetSnapshotAuthorityTestObservation(t)
	at := time.Date(2026, 7, 12, 4, 30, 0, 0, time.UTC)
	record := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "snapshot-derived", "", at)

	forgedInput := DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: keys.installationID, TenantID: record.TenantID, UserID: record.UserID,
		WorkspaceRealPath: record.WorkspaceRealPath, CaseID: record.CaseID, CaseBindingHash: record.CaseBindingHash,
		BindingObservationDigest: record.BindingObservationDigest,
		DatasetSnapshotID:        DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("caller-selected")),
		SourceManifestHash:       record.SourceManifestHash, RawManifestSHA256: record.RawManifestSHA256,
		ParserVersion: record.ParserVersion, AcceptedAt: at, AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}
	if _, err := NewDatasetSnapshotAuthorityRecordV1(forgedInput, keys.sign); err == nil {
		t.Fatal("caller-selected snapshot id was accepted")
	}

	otherObservation, err := NewCaseBindingObservationV1(CaseBindingObservationInputV1{
		WorkspaceRealPath: observation.WorkspaceRealPath, State: CaseBindingStateValid, CaseID: "case-b",
		BindingSHA256: SHA256Hex([]byte("binding-file-b")), CaseBindingHash: SHA256Hex([]byte("binding-canonical-b")),
	})
	if err != nil {
		t.Fatal(err)
	}
	other := newDatasetSnapshotAuthorityTestRecord(t, keys, otherObservation, "snapshot-derived", "", at)
	if other.DatasetSnapshotID == record.DatasetSnapshotID {
		t.Fatal("identical raw material reused a dataset snapshot id across case bindings")
	}
}

func TestDatasetSnapshotAuthorityTransitionRequiresExplicitImmutableLineage(t *testing.T) {
	keys := newDatasetSnapshotAuthorityTestKeys(t, "installation-transition")
	observation := datasetSnapshotAuthorityTestObservation(t)
	at := time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC)
	previous := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "snapshot-v1", "", at)
	if err := ValidateDatasetSnapshotAuthorityTransitionV1(previous, previous); err != nil {
		t.Fatalf("idempotent record replay failed: %v", err)
	}
	next := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "snapshot-v2", previous.RecordDigest, at.Add(time.Second))
	if err := ValidateDatasetSnapshotAuthorityTransitionV1(previous, next); err != nil {
		t.Fatalf("explicit snapshot successor failed: %v", err)
	}
	missingPredecessor := newDatasetSnapshotAuthorityTestRecord(t, keys, observation, "snapshot-v2", "", at.Add(time.Second))
	if ValidateDatasetSnapshotAuthorityTransitionV1(previous, missingPredecessor) == nil {
		t.Fatal("new snapshot without explicit predecessor was accepted")
	}
	overwriteInput := DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: keys.installationID, TenantID: previous.TenantID, UserID: previous.UserID,
		WorkspaceRealPath: previous.WorkspaceRealPath, CaseID: previous.CaseID, CaseBindingHash: previous.CaseBindingHash,
		BindingObservationDigest: previous.BindingObservationDigest,
		SourceManifestHash:       SHA256Hex([]byte("overwritten-source")), RawManifestSHA256: SHA256Hex([]byte("overwritten-raw")),
		ParserVersion: previous.ParserVersion, AcceptedAt: at.Add(time.Second), PredecessorRecordDigest: previous.RecordDigest,
		AuthorityKeyID: keys.keyID, AuthorityPublicKey: keys.publicKey,
	}
	overwriteInput.DatasetSnapshotID = previous.DatasetSnapshotID
	overwrite, err := NewDatasetSnapshotAuthorityRecordV1(overwriteInput, keys.sign)
	if err == nil {
		if ValidateDatasetSnapshotAuthorityTransitionV1(previous, overwrite) == nil {
			t.Fatal("existing snapshot id was rebound to different content")
		}
	} else if err.Error() != "dataset snapshot id does not match immutable host material" {
		t.Fatalf("unexpected immutable snapshot rejection: %v", err)
	}
}
