package security

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const fundsProducerContentManifestV2Golden = `{"acceptedRowCount":3,"canonicalEncoder":"analytix.go-funds-content-manifest/v2","capabilities":["count_case_rows"],"caseId":"case-资金-002","contract":"analytix.funds-producer-content-manifest/v2","detailContentSha256":"3333333333333333333333333333333333333333333333333333333333333333","detailRowCount":3,"duplicateRowCount":1,"normalizedContentSha256":"2222222222222222222222222222222222222222222222222222222222222222","parserId":"analytix.strict-utf8-csv","parserVersion":"1","producerComponentId":"runtime-go","producerComponentVersion":"1.0.0","producerEngine":"analytix.runtime-go.strict-csv","producerOperation":"build-count-case-rows-snapshot","producerOperationSchemaHash":"361bb839d98ba47106a0d8cab020de70552cf1290557718f5827d7ab4e328988","rawArtifactManifestSha256":"1111111111111111111111111111111111111111111111111111111111111111","rejectedRowCount":1,"schemaVersion":2,"sourceRevision":8,"sourceRowCount":5,"transformationId":"analytix.count-case-rows","transformationVersion":"1"}`

func TestFundsProducerContentManifestV2CanonicalIdentity(t *testing.T) {
	manifest := fundsProducerContentManifestV2GoldenFixture(t)
	body, err := FundsProducerContentManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != fundsProducerContentManifestV2Golden {
		t.Fatalf("funds producer v2 canonical bytes drifted: len=%d body=%s", len(body), body)
	}
	if len(body) != 997 {
		t.Fatalf("funds producer v2 canonical byte length drifted: %d", len(body))
	}
	parsed, err := ParseFundsProducerContentManifestV2(body)
	if err != nil || parsed != manifest {
		t.Fatalf("funds producer v2 canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	manifestSHA256, err := FundsProducerContentManifestV2SHA256(manifest)
	if err != nil || manifestSHA256 != "11231100244875061880b00ce6ab9fc55c1143322db3e7b6c23a4856b76afe69" {
		t.Fatalf("funds producer v2 SHA-256 failed: sha=%q err=%v", manifestSHA256, err)
	}
	id := DeriveFundsProducerContentIDV2(manifest)
	if id != "fpc2_9dbd9c03ce7cd276ebc7341ed6afe8dd2386372a6c8388b1182032898a85f376" ||
		!IsFundsProducerContentIDV2Syntax(id) ||
		IsFundsProducerContentIDV1Syntax(id) ||
		IsDatasetSnapshotIDV2Syntax(id) {
		t.Fatalf("funds producer v2 identity was not isolated: %q", id)
	}
}

func TestFundsProducerContentManifestV2RejectsOpenRelabelledOrInconsistentPayload(t *testing.T) {
	manifest := fundsProducerContentManifestV2GoldenFixture(t)
	body, _ := FundsProducerContentManifestV2Bytes(manifest)
	duplicate := bytes.Replace(body, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":2,"schemaVersion":2`), 1)
	if _, err := ParseFundsProducerContentManifestV2(duplicate); err == nil {
		t.Fatal("duplicate producer v2 property was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["datasetSnapshotId"] = DatasetSnapshotIDPrefixV2 + strings.Repeat("a", 64)
	unknown, _ := json.Marshal(object)
	if _, err := ParseFundsProducerContentManifestV2(unknown); err == nil {
		t.Fatal("unknown host-authority property entered the producer v2 contract")
	}
	if _, err := ParseFundsProducerContentManifestV2(append(body, ' ')); err == nil {
		t.Fatal("noncanonical producer v2 bytes were accepted")
	}

	for label, mutate := range map[string]func(*FundsProducerContentManifestV2){
		"contract": func(value *FundsProducerContentManifestV2) {
			value.Contract = FundsProducerContentManifestContractV1
		},
		"capability": func(value *FundsProducerContentManifestV2) {
			value.Capabilities[0] = "query_transactions"
		},
		"component": func(value *FundsProducerContentManifestV2) {
			value.ProducerComponentID = FundsProducerComponentIDV1
		},
		"component version": func(value *FundsProducerContentManifestV2) {
			value.ProducerComponentVersion = "0.1.0"
		},
		"engine": func(value *FundsProducerContentManifestV2) {
			value.ProducerEngine = FundsProducerDuckDBVersionV1
		},
		"operation": func(value *FundsProducerContentManifestV2) {
			value.ProducerOperation = FundsProducerOperationV1
		},
		"operation schema": func(value *FundsProducerContentManifestV2) {
			value.ProducerOperationSchemaHash = strings.Repeat("a", 64)
		},
		"parser": func(value *FundsProducerContentManifestV2) {
			value.ParserID = "caller-selected"
		},
		"transformation": func(value *FundsProducerContentManifestV2) {
			value.TransformationVersion = "2"
		},
		"encoder": func(value *FundsProducerContentManifestV2) {
			value.CanonicalEncoder = FundsProducerCanonicalEncoderV1
		},
		"raw graph syntax": func(value *FundsProducerContentManifestV2) {
			value.RawArtifactManifestSHA256 = strings.Repeat("A", 64)
		},
		"source coverage": func(value *FundsProducerContentManifestV2) {
			value.SourceRowCount++
		},
		"detail coverage": func(value *FundsProducerContentManifestV2) {
			value.DetailRowCount++
		},
	} {
		changed := manifest
		mutate(&changed)
		if ValidateFundsProducerContentManifestV2(changed) == nil {
			t.Fatalf("%s relabelling or inconsistency was accepted", label)
		}
	}
}

func TestFundsProducerContentManifestV2ZeroRowsRemainACompleteCountSnapshot(t *testing.T) {
	manifest, err := NewFundsProducerContentManifestV2(FundsProducerContentManifestInputV2{
		CaseID:                    "case-empty",
		SourceRevision:            1,
		RawArtifactManifestSHA256: strings.Repeat("1", 64),
		NormalizedContentSHA256:   strings.Repeat("2", 64),
		DetailContentSHA256:       strings.Repeat("3", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SourceRowCount != 0 || manifest.AcceptedRowCount != 0 ||
		manifest.RejectedRowCount != 0 || manifest.DuplicateRowCount != 0 ||
		manifest.DetailRowCount != 0 {
		t.Fatalf("zero-row snapshot changed coverage: %#v", manifest)
	}
}

func fundsProducerContentManifestV2GoldenFixture(t *testing.T) FundsProducerContentManifestV2 {
	t.Helper()
	manifest, err := NewFundsProducerContentManifestV2(FundsProducerContentManifestInputV2{
		CaseID:                    "case-资金-002",
		SourceRevision:            8,
		RawArtifactManifestSHA256: strings.Repeat("1", 64),
		NormalizedContentSHA256:   strings.Repeat("2", 64),
		DetailContentSHA256:       strings.Repeat("3", 64),
		SourceRowCount:            5,
		AcceptedRowCount:          3,
		RejectedRowCount:          1,
		DuplicateRowCount:         1,
		DetailRowCount:            3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
