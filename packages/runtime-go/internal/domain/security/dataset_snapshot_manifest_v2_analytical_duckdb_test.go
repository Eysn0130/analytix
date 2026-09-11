package security

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDatasetSnapshotManifestV2AnalyticalDuckDBIsOptionalPathFreeAndIdentityBound(t *testing.T) {
	legacyInput := datasetSnapshotManifestInputV2ForTest(
		t,
		datasetSnapshotAuthorityTestObservation(t),
		"analytical-duckdb-legacy",
	)
	legacy, err := NewDatasetSnapshotManifestV2(legacyInput)
	if err != nil {
		t.Fatal(err)
	}
	legacyBody, err := DatasetSnapshotManifestV2Bytes(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(legacyBody, []byte(`"analyticalDuckdb"`)) {
		t.Fatal("an absent analytical DuckDB binding changed legacy canonical bytes")
	}
	if got := SHA256Hex(legacyBody); got != "5edab9336997731fe553ae3b2182faffeba411b2dd177c847b5ee74fbc2e0c2d" {
		t.Fatalf("legacy canonical bytes changed: got %s", got)
	}
	if legacy.SourceManifestHash != "027179d97a0877e380fea5287e334f3f44651fee0cfe56a84640d2d19551c84f" {
		t.Fatalf("legacy source manifest hash changed: got %s", legacy.SourceManifestHash)
	}
	if legacy.ManifestDigest != "c06924647ad4babaecc6b2f89ad68cd09b7961f74f37ddf0e36f9da49c71e784" {
		t.Fatalf("legacy manifest digest changed: got %s", legacy.ManifestDigest)
	}

	bindingInput := datasetSnapshotAnalyticalDuckDBBindingInputV2ForTest("identity")
	binding, err := NewDatasetSnapshotAnalyticalDuckDBBindingV2(bindingInput)
	if err != nil {
		t.Fatal(err)
	}
	values, err := binding.Values()
	if err != nil || values != bindingInput {
		t.Fatalf("analytical DuckDB binding values changed: values=%#v err=%v", values, err)
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(binding), &object); err != nil {
		t.Fatal(err)
	}
	wantKeys := map[string]bool{
		"duckdbSha256": true, "duckdbByteLength": true,
		"duckdbContentSnapshotDigest": true, "duckdbSnapshotManifestSha256": true,
		"materializationIdentity": true, "schemaDigest": true, "queryProfileDigest": true,
		"datasetUtcOffsetMinutes": true, "expectedCurrency": true, "minorUnitScale": true,
	}
	if len(object) != len(wantKeys) {
		t.Fatalf("analytical DuckDB binding has an unexpected shape: %#v", object)
	}
	for key := range object {
		if !wantKeys[key] {
			t.Fatalf("analytical DuckDB binding exposed unexpected field %q", key)
		}
	}
	for _, forbidden := range []string{
		"databasePath", "duckdbPath", "workspaceRealPath", "sql", "queryText", "opaqueObjectAddress",
	} {
		if strings.Contains(string(binding), forbidden) {
			t.Fatalf("path-free analytical DuckDB binding contains forbidden field %q", forbidden)
		}
	}

	boundInput := legacyInput
	boundInput.AnalyticalDuckDB = binding
	bound, err := NewDatasetSnapshotManifestV2(boundInput)
	if err != nil {
		t.Fatal(err)
	}
	if bound.AnalyticalDuckDB != binding ||
		bound.SourceManifestHash == legacy.SourceManifestHash ||
		bound.ManifestDigest == legacy.ManifestDigest {
		t.Fatal("analytical DuckDB binding was not included in both DSV2 identities")
	}
	boundBody, err := DatasetSnapshotManifestV2Bytes(bound)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDatasetSnapshotManifestV2(boundBody)
	if err != nil || parsed != bound {
		t.Fatalf("bound DSV2 canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}

	changedBindingInput := bindingInput
	changedBindingInput.DuckDBSHA256 = SHA256Hex([]byte("another-immutable-duckdb"))
	changedBinding, err := NewDatasetSnapshotAnalyticalDuckDBBindingV2(changedBindingInput)
	if err != nil {
		t.Fatal(err)
	}
	changedInput := legacyInput
	changedInput.AnalyticalDuckDB = changedBinding
	changed, err := NewDatasetSnapshotManifestV2(changedInput)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SourceManifestHash == bound.SourceManifestHash ||
		changed.ManifestDigest == bound.ManifestDigest {
		t.Fatal("analytical DuckDB content drift reused a DSV2 identity")
	}
}

func TestDatasetSnapshotManifestV2AnalyticalDuckDBRejectsPartialFPC2AndLegacyUse(t *testing.T) {
	bindingInput := datasetSnapshotAnalyticalDuckDBBindingInputV2ForTest("strict")
	for label, mutate := range map[string]func(*DatasetSnapshotAnalyticalDuckDBBindingInputV2){
		"duckdb sha": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.DuckDBSHA256 = ""
		},
		"duckdb length": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.DuckDBByteLength = 0
		},
		"content snapshot": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.DuckDBContentSnapshotDigest = ""
		},
		"snapshot manifest": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.DuckDBSnapshotManifestSHA256 = ""
		},
		"materialization": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.MaterializationIdentity = ""
		},
		"schema": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.SchemaDigest = ""
		},
		"query profile": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.QueryProfileDigest = ""
		},
		"timezone": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.DatasetUTCOffsetMinutes = 841
		},
		"currency": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.ExpectedCurrency = "cny"
		},
		"minor unit scale": func(value *DatasetSnapshotAnalyticalDuckDBBindingInputV2) {
			value.MinorUnitScale = 3
		},
	} {
		t.Run(label, func(t *testing.T) {
			candidate := bindingInput
			mutate(&candidate)
			if _, err := NewDatasetSnapshotAnalyticalDuckDBBindingV2(candidate); err == nil {
				t.Fatal("incomplete analytical DuckDB binding was accepted")
			}
		})
	}

	binding, err := NewDatasetSnapshotAnalyticalDuckDBBindingV2(bindingInput)
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := bytes.Replace(
		[]byte(binding),
		[]byte(`{"duckdbSha256":`),
		[]byte(`{"databasePath":"/private/case.duckdb","duckdbSha256":`),
		1,
	)
	if _, err := ParseDatasetSnapshotAnalyticalDuckDBBindingV2(withUnknown); err == nil {
		t.Fatal("analytical DuckDB binding accepted a database path")
	}
	if _, err := ParseDatasetSnapshotAnalyticalDuckDBBindingV2([]byte(`{}`)); err == nil {
		t.Fatal("partial analytical DuckDB binding was accepted")
	}

	fpc2Input := datasetSnapshotManifestInputV2ForTest(
		t,
		datasetSnapshotAuthorityTestObservation(t),
		"analytical-duckdb-fpc2",
	)
	fpc2Input.FundsProducerContentManifest = FundsProducerContentManifestV1{}
	fpc2Input.AnalyticalDuckDB = binding
	producer, err := NewFundsProducerContentManifestV2(FundsProducerContentManifestInputV2{
		CaseID:                    fpc2Input.Binding.CaseID,
		SourceRevision:            1,
		RawArtifactManifestSHA256: fpc2Input.RawArtifactManifestSHA256,
		NormalizedContentSHA256:   SHA256Hex([]byte("fpc2-normalized")),
		DetailContentSHA256:       SHA256Hex([]byte("fpc2-detail")),
		SourceRowCount:            fpc2Input.SourceRecordCount,
		AcceptedRowCount:          fpc2Input.AcceptedRecordCount,
		RejectedRowCount:          fpc2Input.RejectedRecordCount,
		DuplicateRowCount:         fpc2Input.DuplicateRecordCount,
		DetailRowCount:            fpc2Input.AcceptedRecordCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDatasetSnapshotManifestForFundsProducerContentV2(fpc2Input, producer); err == nil {
		t.Fatal("count-only FPC2 accepted an analytical DuckDB binding")
	}

	fpc1Input := datasetSnapshotManifestInputV2ForTest(
		t,
		datasetSnapshotAuthorityTestObservation(t),
		"analytical-duckdb-json",
	)
	fpc1Input.AnalyticalDuckDB = binding
	manifest, err := NewDatasetSnapshotManifestV2(fpc1Input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := DatasetSnapshotManifestV2Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	partial := bytes.Replace(body, []byte(binding), []byte(`{}`), 1)
	if _, err := ParseDatasetSnapshotManifestV2(partial); err == nil {
		t.Fatal("DSV2 parser accepted a partial analytical DuckDB binding")
	}
	legacySchema := bytes.Replace(body, []byte(`"schemaVersion":2`), []byte(`"schemaVersion":1`), 1)
	if _, err := ParseDatasetSnapshotManifestV2(legacySchema); err == nil {
		t.Fatal("legacy dataset snapshot schema accepted an analytical DuckDB binding")
	}
}

func datasetSnapshotAnalyticalDuckDBBindingInputV2ForTest(
	seed string,
) DatasetSnapshotAnalyticalDuckDBBindingInputV2 {
	return DatasetSnapshotAnalyticalDuckDBBindingInputV2{
		DuckDBSHA256:                 SHA256Hex([]byte("duckdb:" + seed)),
		DuckDBByteLength:             8192,
		DuckDBContentSnapshotDigest:  SHA256Hex([]byte("duckdb-content-snapshot:" + seed)),
		DuckDBSnapshotManifestSHA256: SHA256Hex([]byte("duckdb-snapshot-manifest:" + seed)),
		MaterializationIdentity: FundsMaterializationIdentityPrefixV1 +
			SHA256Hex([]byte("materialization:"+seed)),
		SchemaDigest:            SHA256Hex([]byte("schema:" + seed)),
		QueryProfileDigest:      SHA256Hex([]byte("query-profile:" + seed)),
		DatasetUTCOffsetMinutes: 0,
		ExpectedCurrency:        "CNY",
		MinorUnitScale:          datasetSnapshotAnalyticalMinorUnitScaleV2,
	}
}
