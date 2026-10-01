package security

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestDatasetSnapshotIndexHostLocalV2ClosedModeAndLegacyBytes(t *testing.T) {
	fixture := newDatasetSnapshotIndexTestFixture(t)
	legacy, err := DatasetSnapshotIndexV1Bytes(fixture.index)
	if err != nil {
		t.Fatal(err)
	}
	// Frozen V1 byte shape from the c2cd3e2 signed-index contract, including
	// its original signature and digest. The new union encoder must preserve it.
	historical, err := os.ReadFile("testdata/dataset-snapshot-index-v1-historical.json")
	if err != nil {
		t.Fatal(err)
	}
	historical = bytes.TrimSuffix(historical, []byte("\n"))
	parsedHistorical, err := ParseVersionedDatasetSnapshotIndex(historical)
	if err != nil || parsedHistorical.SchemaVersion != DatasetSnapshotIndexSchemaVersion ||
		parsedHistorical.EnrollmentID == "" || parsedHistorical.Mode != "" {
		t.Fatalf("historical witnessed index cannot be read by the versioned parser: %v", err)
	}
	if reencoded, err := VersionedDatasetSnapshotIndexBytes(parsedHistorical); err != nil || !bytes.Equal(reencoded, historical) {
		t.Fatalf("historical witnessed signature and digest bytes changed: %v", err)
	}
	legacyThroughUnion, err := VersionedDatasetSnapshotIndexBytes(fixture.index)
	if err != nil || !bytes.Equal(legacy, legacyThroughUnion) {
		t.Fatalf("versioned parser changed witnessed signed bytes: %v", err)
	}
	modeCommitment := SHA256Hex([]byte("host-local-mode-genesis"))
	index, err := NewDatasetSnapshotIndexHostLocalV2(DatasetSnapshotIndexHostLocalInputV2{
		InstallationID:       fixture.keys.installationID,
		ModeCommitmentDigest: modeCommitment,
		Generation:           1,
		PreviousIndexDigest:  DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:           SHA256Hex([]byte("host-local-dataset-mutation")),
		Binding:              fixture.index.Binding,
		SnapshotRecordDigest: fixture.record.RecordDigest,
		AuthorityKeyID:       fixture.keys.keyID,
		AuthorityPublicKey:   fixture.keys.publicKey,
	}, fixture.keys.sign)
	if err != nil {
		t.Fatal(err)
	}
	body, err := VersionedDatasetSnapshotIndexBytes(index)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseVersionedDatasetSnapshotIndex(body)
	if err != nil || parsed != index {
		t.Fatalf("host-local index did not round trip: %v", err)
	}
	if _, err := ParseDatasetSnapshotIndexV1(body); err == nil || ValidateDatasetSnapshotIndexV1(index) == nil {
		t.Fatal("old witnessed index parser accepted host-local mode")
	}
	if ValidateDatasetSnapshotIndexForHostLocalV2(index, fixture.keys.installationID,
		modeCommitment, fixture.keys.keyID, fixture.keys.publicKey) != nil ||
		ValidateDatasetSnapshotIndexForHostLocalV2(index, fixture.keys.installationID,
			SHA256Hex([]byte("other mode")), fixture.keys.keyID, fixture.keys.publicKey) == nil {
		t.Fatal("host-local mode commitment was not anchored exactly")
	}
	mutated := index
	mutated.Mode = "witnessed"
	if ValidateVersionedDatasetSnapshotIndex(mutated) == nil {
		t.Fatal("cross-labelled host-local index was accepted")
	}
	mutated = index
	mutated.EnrollmentID = fixture.enrollmentID
	if ValidateVersionedDatasetSnapshotIndex(mutated) == nil {
		t.Fatal("host-local index accepted a witness enrollment")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseVersionedDatasetSnapshotIndex(unknown); err == nil {
		t.Fatal("host-local index accepted unknown signed fields")
	}
}
