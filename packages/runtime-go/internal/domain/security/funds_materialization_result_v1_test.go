package security

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestFundsMaterializationResultV1RequiresExactProducerAndRawSourceBytes(t *testing.T) {
	rawSources := FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: "file-1", RowsImportedNorm: 2,
		SHA256: strings.Repeat("a", 64), Status: "已完成",
	}}
	rawBody, err := fundsRawSourceManifestV1Bytes(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := NewFundsProducerContentManifestV1(FundsProducerContentManifestInputV1{
		CaseID: "case-a", SourceRevision: 7, RawManifestSHA256: strings.Repeat("9", 64),
		NormalizedContentSHA256: strings.Repeat("b", 64), DetailContentSHA256: strings.Repeat("c", 64),
		AggregateContentSHA256: strings.Repeat("d", 64), KeywordContentSHA256: strings.Repeat("e", 64),
		AccountContentSHA256: strings.Repeat("f", 64), NormalizedRowCount: 2, AcceptedRowCount: 2,
		DetailRowCount: 2, AggregateRowCount: 1, KeywordRowCount: 2, AccountRowCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	producerBody, err := FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	result := FundsMaterializationResultV1{
		CaseID: "case-a", RowCount: 1,
		AggregateName:                        FundsMaterializationIdentityPrefixV1 + strings.Repeat("1", 64),
		AggregateVersion:                     FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              FundsMaterializationIdentityPrefixV1 + strings.Repeat("1", 64),
		MaterializationIdentitySchemaVersion: FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            producer.RawManifestSHA256,
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawBody),
		RawSourceManifestSHA256:              SHA256Hex(rawBody), RawSourceManifestByteLength: uint64(len(rawBody)),
		DuckDBContentSnapshotDigest:  strings.Repeat("7", 64),
		DuckDBSnapshotManifestSHA256: strings.Repeat("8", 64),
		SchemaDigest:                 FixedFundsAnalyticalSchemaDigestV1(),
	}
	body, err := fundsMaterializationResultV1Bytes(result)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFundsMaterializationResultV1(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ProducerContentManifest != producer || len(parsed.RawSourceManifest) != 1 {
		t.Fatalf("parsed materialization lost exact producer evidence: %#v", parsed)
	}
	if parsed.RawArtifactManifestSHA256 == parsed.RawSourceManifestSHA256 ||
		parsed.RawArtifactManifestSHA256 != parsed.ProducerContentManifest.RawManifestSHA256 {
		t.Fatal("typed raw manifest and simplified raw-source inventory were conflated")
	}

	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{
		"producerContentId":                    "fpc1_" + strings.Repeat("2", 64),
		"producerContentManifestByteLength":    float64(len(producerBody) + 1),
		"rawSourceManifestSha256":              strings.Repeat("3", 64),
		"rawArtifactManifestSha256":            strings.Repeat("4", 64),
		"duckdbContentSnapshotDigest":          strings.Repeat("A", 64),
		"duckdbSnapshotManifestSha256":         strings.Repeat("6", 63),
		"schemaDigest":                         strings.Repeat("a", 64),
		"materializationIdentitySchemaVersion": float64(1),
	} {
		mutated := make(map[string]any, len(wire))
		for key, item := range wire {
			mutated[key] = item
		}
		mutated[name] = value
		candidate, err := canonicalJSONWithoutHTMLEscapeV1(mutated)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseFundsMaterializationResultV1(candidate); err == nil {
			t.Fatalf("materialization accepted mutated %s", name)
		}
	}

}

func TestFixedFundsAnalyticalSchemaDigestV1Golden(t *testing.T) {
	const want = "81a474553b16799c12d8d85c28d99f25ba6c9b40ae2073701443d2898f91e11e"
	if got := FixedFundsAnalyticalSchemaDigestV1(); got != want {
		t.Fatalf("fixed funds analytical schema drifted: got=%s", got)
	}
}

func TestFundsRawSourceManifestV1RejectsOrderingCoverageAndUnknownFields(t *testing.T) {
	valid := `[{"cleanedStatus":"done","fileId":"a","rowsImportedNorm":1,"sha256":"` + strings.Repeat("a", 64) + `","status":"已完成"},{"cleanedStatus":"done","fileId":"b","rowsImportedNorm":1,"sha256":"` + strings.Repeat("b", 64) + `","status":"已完成"}]`
	if _, err := ParseFundsRawSourceManifestV1([]byte(valid), 2); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{
		strings.Replace(valid, `"fileId":"a"`, `"fileId":"b"`, 1),
		strings.Replace(valid, `"rowsImportedNorm":1`, `"rowsImportedNorm":0`, 1),
		strings.Replace(valid, `"status":"已完成"`, `"status":"pending"`, 1),
		strings.Replace(valid, `"fileId":"a"`, `"extra":true,"fileId":"a"`, 1),
	} {
		if _, err := ParseFundsRawSourceManifestV1([]byte(candidate), 2); err == nil {
			t.Fatalf("raw-source manifest accepted invalid body: %s", candidate)
		}
	}
}

func TestFundsMaterializationResultV1RejectsLegacyConflatedRawManifest(t *testing.T) {
	rawSources := FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: "file-1", RowsImportedNorm: 1,
		SHA256: strings.Repeat("a", 64), Status: "已完成",
	}}
	rawBody, err := fundsRawSourceManifestV1Bytes(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	rawSourceSHA := SHA256Hex(rawBody)
	producer, err := NewFundsProducerContentManifestV1(FundsProducerContentManifestInputV1{
		CaseID: "case-a", SourceRevision: 1, RawManifestSHA256: rawSourceSHA,
		NormalizedContentSHA256: strings.Repeat("b", 64), DetailContentSHA256: strings.Repeat("c", 64),
		AggregateContentSHA256: strings.Repeat("d", 64), KeywordContentSHA256: strings.Repeat("e", 64),
		AccountContentSHA256: strings.Repeat("f", 64), NormalizedRowCount: 1, AcceptedRowCount: 1,
		DetailRowCount: 1, AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	producerBody, err := FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	legacy := FundsMaterializationResultV1{
		CaseID: "case-a", RowCount: 1,
		AggregateName:                        FundsMaterializationIdentityPrefixV1 + strings.Repeat("1", 64),
		AggregateVersion:                     FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              FundsMaterializationIdentityPrefixV1 + strings.Repeat("1", 64),
		MaterializationIdentitySchemaVersion: FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            rawSourceSHA,
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawBody),
		RawSourceManifestSHA256:              rawSourceSHA,
		RawSourceManifestByteLength:          uint64(len(rawBody)),
		DuckDBContentSnapshotDigest:          strings.Repeat("7", 64),
		DuckDBSnapshotManifestSHA256:         strings.Repeat("8", 64),
		SchemaDigest:                         FixedFundsAnalyticalSchemaDigestV1(),
	}
	body, err := fundsMaterializationResultV1Bytes(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFundsMaterializationResultV1(body); err == nil {
		t.Fatal("legacy simplified raw-source digest acquired typed raw-manifest authority")
	}
}
