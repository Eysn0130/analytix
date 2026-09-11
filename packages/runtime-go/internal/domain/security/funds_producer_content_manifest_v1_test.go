package security

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const fundsProducerContentManifestV1Golden = `{"acceptedRowCount":3,"accountContentSha256":"6666666666666666666666666666666666666666666666666666666666666666","accountRowCount":2,"aggregateContentSha256":"4444444444444444444444444444444444444444444444444444444444444444","aggregateRowCount":2,"canonicalEncoder":"analytix.duckdb-content-manifest/v1","caseId":"case-资金-001","contract":"analytix.funds-producer-content-manifest/v1","detailContentSha256":"3333333333333333333333333333333333333333333333333333333333333333","detailRowCount":3,"duckdbVersion":"v1.5.4","duplicateRowCount":1,"keywordContentSha256":"5555555555555555555555555555555555555555555555555555555555555555","keywordRowCount":4,"normalizedContentSha256":"2222222222222222222222222222222222222222222222222222222222222222","normalizedRowCount":5,"producerComponentId":"analysis-compute","producerComponentVersion":"0.1.0","producerOperation":"materialize-txn-daily","producerOperationSchemaHash":"c8947b87b6962283023215b4b1e30c704b955d1bb7c8473cc1a0e76b7ba47f36","rawManifestSha256":"1111111111111111111111111111111111111111111111111111111111111111","rejectedRowCount":1,"schemaVersion":1,"sourceRevision":7}`

func TestFundsProducerContentManifestV1MatchesCrossLanguageCanonicalGolden(t *testing.T) {
	manifest := fundsProducerContentManifestV1GoldenFixture(t)
	body, err := FundsProducerContentManifestV1Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != fundsProducerContentManifestV1Golden || len(body) != 1130 {
		t.Fatalf("funds producer canonical bytes drifted: len=%d body=%s", len(body), body)
	}
	sha256, err := FundsProducerContentManifestV1SHA256(manifest)
	if err != nil || sha256 != "bb297df82bc8d820806abde1a398de1ace2c7ae86ef2df88880f94af6e3aa5de" {
		t.Fatalf("funds producer manifest SHA-256 drifted: sha=%q err=%v", sha256, err)
	}
	id := DeriveFundsProducerContentIDV1(manifest)
	if id != "fpc1_b72414526c46fa1392bcbddddee4c26f3e41ec7f5cd349e1ba9b3db25c1d2aa0" ||
		!IsFundsProducerContentIDV1Syntax(id) || IsDatasetSnapshotIDV2Syntax(id) {
		t.Fatalf("funds producer identity was not isolated from DSV2: %q", id)
	}
	parsed, err := ParseFundsProducerContentManifestV1(body)
	if err != nil || parsed != manifest {
		t.Fatalf("funds producer canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	if DatasetSnapshotFactAuthorityBlocker(id) == "" {
		t.Fatal("a bare producer content id acquired dataset fact authority")
	}
}

func TestFundsProducerContentManifestV1RejectsOpenAmbiguousOrRelabelledPayload(t *testing.T) {
	manifest := fundsProducerContentManifestV1GoldenFixture(t)
	body, _ := FundsProducerContentManifestV1Bytes(manifest)
	duplicate := bytes.Replace(body, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1)
	if _, err := ParseFundsProducerContentManifestV1(duplicate); err == nil {
		t.Fatal("duplicate producer property was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["datasetSnapshotId"] = DatasetSnapshotIDPrefixV2 + strings.Repeat("a", 64)
	unknown, _ := json.Marshal(object)
	if _, err := ParseFundsProducerContentManifestV1(unknown); err == nil {
		t.Fatal("unknown host-authority property entered the producer contract")
	}
	if _, err := ParseFundsProducerContentManifestV1(append(body, ' ')); err == nil {
		t.Fatal("noncanonical producer bytes were accepted")
	}

	for label, mutate := range map[string]func(*FundsProducerContentManifestV1){
		"contract": func(value *FundsProducerContentManifestV1) {
			value.Contract = "analytix.funds-producer-content-manifest/v2"
		},
		"component": func(value *FundsProducerContentManifestV1) {
			value.ProducerComponentID = "caller-selected"
		},
		"component version": func(value *FundsProducerContentManifestV1) {
			value.ProducerComponentVersion = "0.1.1"
		},
		"operation": func(value *FundsProducerContentManifestV1) {
			value.ProducerOperation = "query"
		},
		"operation schema": func(value *FundsProducerContentManifestV1) {
			value.ProducerOperationSchemaHash = strings.Repeat("a", 64)
		},
		"engine": func(value *FundsProducerContentManifestV1) {
			value.DuckDBVersion = "v1.5.2"
		},
		"encoder": func(value *FundsProducerContentManifestV1) {
			value.CanonicalEncoder = "caller-selected"
		},
		"coverage": func(value *FundsProducerContentManifestV1) {
			value.DetailRowCount++
		},
	} {
		changed := manifest
		mutate(&changed)
		if ValidateFundsProducerContentManifestV1(changed) == nil {
			t.Fatalf("%s relabelling was accepted", label)
		}
	}
}

func fundsProducerContentManifestV1GoldenFixture(t *testing.T) FundsProducerContentManifestV1 {
	t.Helper()
	manifest, err := NewFundsProducerContentManifestV1(FundsProducerContentManifestInputV1{
		CaseID: "case-资金-001", SourceRevision: 7,
		RawManifestSHA256: strings.Repeat("1", 64), NormalizedContentSHA256: strings.Repeat("2", 64),
		DetailContentSHA256: strings.Repeat("3", 64), AggregateContentSHA256: strings.Repeat("4", 64),
		KeywordContentSHA256: strings.Repeat("5", 64), AccountContentSHA256: strings.Repeat("6", 64),
		NormalizedRowCount: 5, AcceptedRowCount: 3, RejectedRowCount: 1, DuplicateRowCount: 1,
		DetailRowCount: 3, AggregateRowCount: 2, KeywordRowCount: 4, AccountRowCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
