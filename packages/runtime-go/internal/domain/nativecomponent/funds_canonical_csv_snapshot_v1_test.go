package nativecomponent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestFundsCanonicalCSVSnapshotArgumentsAreOpaqueAndFrameIsClosed(t *testing.T) {
	arguments := fundsCanonicalCSVSnapshotTestArgumentsV1(t)
	if encoded, err := json.Marshal(arguments); err == nil || encoded != nil {
		t.Fatalf("opaque builder arguments serialized: %q err=%v", encoded, err)
	}
	for _, rendered := range []string{fmt.Sprint(arguments), fmt.Sprintf("%#v", arguments)} {
		for _, private := range []string{"0123456789abcdef0123", "case-canonical-csv", strings.Repeat("a", 64)} {
			if strings.Contains(rendered, private) {
				t.Fatalf("opaque builder argument rendering disclosed %q: %s", private, rendered)
			}
		}
	}

	requestID := strings.Repeat("9", 64)
	frame, err := EncodeFundsBuildCanonicalCSVSnapshotNativeRequestFrameV1(
		requestID,
		arguments,
	)
	if err != nil || len(frame) == 0 || frame[len(frame)-1] != '\n' {
		t.Fatalf("encode fixed builder frame: size=%d err=%v", len(frame), err)
	}
	var wire fundsCanonicalCSVSnapshotBuildRequestWireV1
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil || wire.RequestID != requestID ||
		wire.Command != OperationFundsBuildCanonicalCSVSnapshotV1 ||
		wire.CaseID != "case-canonical-csv" ||
		wire.Payload.Profile != FundsCanonicalDirectCSVProfileV1 ||
		wire.Payload.PrivateImportFileID != "0123456789abcdef0123" {
		t.Fatalf("fixed builder frame mismatch: wire=%#v err=%v", wire, err)
	}
	for _, forbidden := range []string{
		"db_path", "sourcePath", "source_path", "sql", "/private/", "6222021234567890123",
	} {
		if bytes.Contains(frame, []byte(forbidden)) {
			t.Fatalf("fixed builder frame disclosed forbidden field/value %q", forbidden)
		}
	}
}

func TestFundsCanonicalCSVSnapshotFrameRejectsInvalidRequestIdentity(t *testing.T) {
	arguments := fundsCanonicalCSVSnapshotTestArgumentsV1(t)
	for _, requestID := range []string{
		"", strings.Repeat("9", 63), strings.Repeat("A", 64), strings.Repeat("g", 64),
	} {
		t.Run(fmt.Sprintf("%q", requestID), func(t *testing.T) {
			if _, err := EncodeFundsBuildCanonicalCSVSnapshotNativeRequestFrameV1(
				requestID,
				arguments,
			); err == nil {
				t.Fatal("invalid request identity was accepted")
			}
		})
	}
}

func TestFundsCanonicalCSVSnapshotResultBindsRawGraphAndMaterialization(t *testing.T) {
	arguments := fundsCanonicalCSVSnapshotTestArgumentsV1(t)
	materialization := fundsCanonicalCSVSnapshotTestMaterializationV1(t)
	wire := fundsCanonicalCSVSnapshotBuildResultWireV1{
		SchemaVersion:                1,
		Operation:                    OperationFundsBuildCanonicalCSVSnapshotV1,
		Profile:                      FundsCanonicalDirectCSVProfileV1,
		PrivateImportFileID:          "0123456789abcdef0123",
		SourceArtifactSHA256:         strings.Repeat("7", 64),
		SourceArtifactByteLength:     4096,
		SourceRowCount:               2,
		SourceMaxTxnTS:               "2026-01-02 03:04:05",
		SourceMaxID:                  2,
		RowHashContract:              FundsCanonicalCSVRowHashContractV1,
		FundsMaterializationResultV1: materialization,
	}
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseFundsCanonicalCSVSnapshotBuildResultV1(body, arguments)
	if err != nil {
		t.Fatalf("parse exact builder result: %v", err)
	}
	if encoded, err := json.Marshal(result); err == nil || encoded != nil {
		t.Fatalf("host-private builder result serialized: %q err=%v", encoded, err)
	}
	parsedMaterialization, err := result.MaterializationV1()
	if err != nil || parsedMaterialization.ProducerContentID != materialization.ProducerContentID {
		t.Fatalf("builder result lost materialization: %#v err=%v", parsedMaterialization, err)
	}
	sha256, byteLength, rows, maxTxnTS, maxID := result.SourceStatsV1()
	if sha256 != strings.Repeat("7", 64) || byteLength != 4096 || rows != 2 ||
		maxTxnTS != "2026-01-02 03:04:05" || maxID != 2 {
		t.Fatalf("builder result source stats drifted: %q %d %d %q %d", sha256, byteLength, rows, maxTxnTS, maxID)
	}

	mutations := map[string]func(*fundsCanonicalCSVSnapshotBuildResultWireV1){
		"short operation": func(value *fundsCanonicalCSVSnapshotBuildResultWireV1) {
			value.Operation = "build_canonical_csv_snapshot_v1"
		},
		"source digest": func(value *fundsCanonicalCSVSnapshotBuildResultWireV1) {
			value.SourceArtifactSHA256 = strings.Repeat("0", 64)
		},
		"source rows": func(value *fundsCanonicalCSVSnapshotBuildResultWireV1) {
			value.SourceRowCount++
		},
		"private import": func(value *fundsCanonicalCSVSnapshotBuildResultWireV1) {
			value.PrivateImportFileID = "abcdef0123456789abcd"
		},
		"materialization manifest": func(value *fundsCanonicalCSVSnapshotBuildResultWireV1) {
			value.RawArtifactManifestSHA256 = strings.Repeat("0", 64)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := wire
			mutate(&candidate)
			body, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseFundsCanonicalCSVSnapshotBuildResultV1(body, arguments); err == nil {
				t.Fatal("mutated builder result remained valid")
			}
		})
	}

	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseFundsCanonicalCSVSnapshotBuildResultV1(unknown, arguments); err == nil {
		t.Fatal("unknown builder result field was accepted")
	}
}

func TestFundsCanonicalCSVSnapshotArgumentsRejectUnboundOrMismatchedInputs(t *testing.T) {
	binding := fundsCanonicalCSVSnapshotTestBindingV1(t)
	valid := FundsCanonicalCSVSnapshotBuildInputV1{
		Binding: binding, PrivateImportFileID: "0123456789abcdef0123", SourceRevision: 7,
		RawArtifactManifestSHA256: strings.Repeat("a", 64),
		SourceArtifactSHA256:      strings.Repeat("7", 64),
		SourceArtifactByteLength:  4096,
		SourceRowCount:            2,
	}
	mutations := []func(*FundsCanonicalCSVSnapshotBuildInputV1){
		func(value *FundsCanonicalCSVSnapshotBuildInputV1) {
			value.Binding.BindingKeyDigest = strings.Repeat("0", 64)
		},
		func(value *FundsCanonicalCSVSnapshotBuildInputV1) { value.PrivateImportFileID = "IMPORT-PRIVATE" },
		func(value *FundsCanonicalCSVSnapshotBuildInputV1) { value.SourceRevision = 0 },
		func(value *FundsCanonicalCSVSnapshotBuildInputV1) { value.SourceArtifactByteLength = 0 },
		func(value *FundsCanonicalCSVSnapshotBuildInputV1) {
			value.SourceRowCount = FundsCanonicalCSVMaximumSourceRowsV1 + 1
		},
	}
	for index, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		if _, err := NewFundsCanonicalCSVSnapshotBuildArgumentsV1(candidate); err == nil {
			t.Fatalf("invalid builder input %d was accepted", index)
		}
	}
	maximum := valid
	maximum.SourceRowCount = FundsCanonicalCSVMaximumSourceRowsV1
	if _, err := NewFundsCanonicalCSVSnapshotBuildArgumentsV1(maximum); err != nil {
		t.Fatalf("shared 100k-row import budget was rejected: %v", err)
	}
}

func fundsCanonicalCSVSnapshotTestArgumentsV1(
	t *testing.T,
) FundsCanonicalCSVSnapshotBuildArgumentsV1 {
	t.Helper()
	arguments, err := NewFundsCanonicalCSVSnapshotBuildArgumentsV1(
		FundsCanonicalCSVSnapshotBuildInputV1{
			Binding:                   fundsCanonicalCSVSnapshotTestBindingV1(t),
			PrivateImportFileID:       "0123456789abcdef0123",
			SourceRevision:            7,
			RawArtifactManifestSHA256: strings.Repeat("a", 64),
			SourceArtifactSHA256:      strings.Repeat("7", 64),
			SourceArtifactByteLength:  4096,
			SourceRowCount:            2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return arguments
}

func fundsCanonicalCSVSnapshotTestBindingV1(
	t *testing.T,
) domainsecurity.DatasetSnapshotBindingKeyV1 {
	t.Helper()
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID:                 domainsecurity.LocalTenantID,
			UserID:                   domainsecurity.LocalUserID,
			WorkspaceRealPath:        "/cases/case-canonical-csv",
			CaseID:                   "case-canonical-csv",
			CaseBindingHash:          strings.Repeat("5", 64),
			BindingObservationDigest: strings.Repeat("6", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func fundsCanonicalCSVSnapshotTestMaterializationV1(
	t *testing.T,
) domainsecurity.FundsMaterializationResultV1 {
	t.Helper()
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(
		domainsecurity.FundsProducerContentManifestInputV1{
			CaseID:                  "case-canonical-csv",
			SourceRevision:          7,
			RawManifestSHA256:       strings.Repeat("a", 64),
			NormalizedContentSHA256: strings.Repeat("b", 64),
			DetailContentSHA256:     strings.Repeat("c", 64),
			AggregateContentSHA256:  strings.Repeat("d", 64),
			KeywordContentSHA256:    strings.Repeat("e", 64),
			AccountContentSHA256:    strings.Repeat("f", 64),
			NormalizedRowCount:      2,
			AcceptedRowCount:        2,
			DetailRowCount:          2,
			AggregateRowCount:       1,
			KeywordRowCount:         1,
			AccountRowCount:         1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	producerBody, err := domainsecurity.FundsProducerContentManifestV1Bytes(producer)
	if err != nil {
		t.Fatal(err)
	}
	rawSources := domainsecurity.FundsRawSourceManifestV1{{
		CleanedStatus: "done", FileID: "0123456789abcdef0123", RowsImportedNorm: 2,
		SHA256: strings.Repeat("7", 64), Status: "已完成",
	}}
	rawSourceBody, err := json.Marshal(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	materialization := domainsecurity.FundsMaterializationResultV1{
		CaseID:                               "case-canonical-csv",
		RowCount:                             producer.AggregateRowCount,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            strings.Repeat("a", 64),
		RawSourceManifestBase64:              base64.StdEncoding.EncodeToString(rawSourceBody),
		RawSourceManifestSHA256:              domainsecurity.SHA256Hex(rawSourceBody),
		RawSourceManifestByteLength:          uint64(len(rawSourceBody)),
		DuckDBContentSnapshotDigest:          strings.Repeat("1", 64),
		DuckDBSnapshotManifestSHA256:         strings.Repeat("2", 64),
		SchemaDigest:                         domainsecurity.FixedFundsAnalyticalSchemaDigestV1(),
	}
	body, err := json.Marshal(materialization)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domainsecurity.ParseFundsMaterializationResultV1(body)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
