package evidence

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestFundsCanonicalCSVAdmissionComposesCompleteExactGraph(t *testing.T) {
	source := []byte("header-a,header-b\r\nvalue-a,value-b\r\n")
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath:        "/cases/funds-canonical-admission-complete",
			CaseID:                   "case_funds_canonical_admission_complete",
			CaseBindingHash:          strings.Repeat("6", 64),
			BindingObservationDigest: strings.Repeat("7", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	materials := make(map[FundsCanonicalCSVAdmissionMaterialKindV1]int)
	summary, err := ComposeFundsCanonicalCSVAdmissionV1(
		context.Background(),
		FundsCanonicalCSVAdmissionInputV1{
			Binding: binding, AcquiredAt: time.Date(2026, 8, 17, 4, 5, 6, 0, time.UTC),
			AcquisitionActorDigest: strings.Repeat("8", 64), IntentNonceDigest: strings.Repeat("9", 64),
			SourceArtifactSHA256:     domainsecurity.SHA256Hex(source),
			SourceArtifactByteLength: uint64(len(source)), SourceRowCount: 1,
		},
		bytes.NewReader(source),
		func(_ context.Context, build FundsCanonicalCSVNativeBuildContextV1) (FundsCanonicalCSVNativeBuildResultV1, error) {
			materialization := fundsCanonicalCSVAdmissionMaterializationV1(t, build)
			parsedFields := []ParsedTypedFieldV1{
				{Name: "norm.clean_amount", Scalar: ParsedTypedScalarV1{Kind: "decimal", Value: "12.5"}},
				{Name: "raw.acct_no", Scalar: ParsedTypedScalarV1{Kind: "text", Value: "6222020000000000001"}},
			}
			row, rowErr := NewFundsCanonicalCSVHostRowV1(
				mustFundsCanonicalCSVSourceFileIDV1(t, build), 1, build.SourceArtifactSHA256,
				parsedCanonicalRowSHA256V1(parsedFields),
				[]FundsCanonicalCSVHostTypedFieldV1{
					{Name: "norm.clean_amount", Kind: "decimal", Value: "12.5"},
					{Name: "raw.acct_no", Kind: "text", Value: "6222020000000000001"},
				},
			)
			if rowErr != nil {
				t.Fatal(rowErr)
			}
			return NewFundsCanonicalCSVNativeBuildResultV1(
				materialization, strings.Repeat("a", 64), 4096,
				[]FundsCanonicalCSVHostRowV1{row},
			)
		},
		func(material FundsCanonicalCSVAdmissionMaterialV1) error {
			return material.UseExactV1(func(kind FundsCanonicalCSVAdmissionMaterialKindV1, _ string, body []byte) error {
				materials[kind]++
				if kind == FundsCanonicalCSVMaterialSnapshotManifestV1 {
					manifest, err := domainsecurity.ParseDatasetSnapshotManifestV2(body)
					if err != nil {
						return err
					}
					analytical, err := manifest.AnalyticalDuckDB.Values()
					if err != nil || !domainfundsquerysource.QueryProfileSupportsAccountFlowV1(analytical.QueryProfileDigest) ||
						!domainfundsquerysource.QueryProfileSupportsAccountIngressResolutionV1(analytical.QueryProfileDigest) ||
						!domainfundsquerysource.QueryProfileSupportsDirectSourcePreviewV1(analytical.QueryProfileDigest) {
						t.Fatal("new canonical import did not bind the existing fixed analysis and local-display profile")
					}
				}
				return nil
			})
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !domainsecurity.IsSHA256Hex(summary.ManifestSHA256) || summary.ManifestByteLength == 0 ||
		!domainsecurity.IsSHA256Hex(summary.ProducerSHA256) || summary.ProducerByteLength == 0 ||
		summary.SourceArtifactSHA256 != domainsecurity.SHA256Hex(source) || summary.SourceRowCount != 1 ||
		materials[FundsCanonicalCSVMaterialSnapshotManifestV1] != 1 ||
		materials[FundsCanonicalCSVMaterialProducerContentV1] != 1 ||
		materials[FundsCanonicalCSVMaterialSourceRowRecordV1] != 1 ||
		materials[FundsCanonicalCSVMaterialSourceRowLineageV1] != 1 {
		t.Fatalf("complete graph summary/materials changed: summary=%#v materials=%#v", summary, materials)
	}
}

func TestFundsCanonicalCSVAdmissionDerivesRawAndParsedIdentityBeforeNativeBuild(t *testing.T) {
	source := []byte("header-a,header-b\r\nvalue-a,value-b\r\n")
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath:        "/cases/funds-canonical-admission",
			CaseID:                   "case_funds_canonical_admission",
			CaseBindingHash:          strings.Repeat("a", 64),
			BindingObservationDigest: strings.Repeat("b", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := FundsCanonicalCSVAdmissionInputV1{
		Binding: binding, AcquiredAt: time.Date(2026, 8, 17, 1, 2, 3, 0, time.UTC),
		AcquisitionActorDigest: strings.Repeat("c", 64), IntentNonceDigest: strings.Repeat("d", 64),
		SourceArtifactSHA256:     domainsecurity.SHA256Hex(source),
		SourceArtifactByteLength: uint64(len(source)), SourceRowCount: 1,
	}
	stop := errors.New("stop after native build authority observation")
	var observed FundsCanonicalCSVNativeBuildContextV1
	materialCount := 0
	_, err = ComposeFundsCanonicalCSVAdmissionV1(
		context.Background(),
		input,
		bytes.NewReader(source),
		func(_ context.Context, build FundsCanonicalCSVNativeBuildContextV1) (FundsCanonicalCSVNativeBuildResultV1, error) {
			observed = build
			return FundsCanonicalCSVNativeBuildResultV1{}, stop
		},
		func(material FundsCanonicalCSVAdmissionMaterialV1) error {
			materialCount++
			return material.UseExactV1(func(kind FundsCanonicalCSVAdmissionMaterialKindV1, address string, body []byte) error {
				if !validFundsCanonicalCSVAdmissionMaterialKindV1(kind) ||
					domainsecurity.SHA256Hex(body) != address {
					t.Fatal("emitted material identity changed")
				}
				return nil
			})
		},
	)
	if err == nil || !strings.Contains(err.Error(), "native build failed") {
		t.Fatalf("native stop was not contained: %v", err)
	}
	if observed.Binding != binding || observed.SourceArtifactSHA256 != input.SourceArtifactSHA256 ||
		observed.SourceArtifactByteLength != input.SourceArtifactByteLength ||
		observed.SourceRowCount != input.SourceRowCount || len(observed.PrivateImportFileID) != 20 ||
		!domainsecurity.IsSHA256Hex(observed.RawArtifactManifestSHA256) ||
		!domainsecurity.IsSHA256Hex(observed.ParsedGenerationIdentitySHA256) || materialCount < 9 {
		t.Fatalf("derived build context/materials changed: context=%#v materials=%d", observed, materialCount)
	}
}

func TestFundsCanonicalCSVAdmissionRejectsChangedSourceBeforeAnyEffect(t *testing.T) {
	source := []byte("header\r\nvalue\r\n")
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(
		domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath:        "/cases/funds-canonical-admission-reject",
			CaseID:                   "case_funds_canonical_admission_reject",
			CaseBindingHash:          strings.Repeat("1", 64),
			BindingObservationDigest: strings.Repeat("2", 64),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	buildCalls, emitCalls := 0, 0
	_, err = ComposeFundsCanonicalCSVAdmissionV1(
		context.Background(),
		FundsCanonicalCSVAdmissionInputV1{
			Binding: binding, AcquiredAt: time.Now().UTC(),
			AcquisitionActorDigest: strings.Repeat("3", 64), IntentNonceDigest: strings.Repeat("4", 64),
			SourceArtifactSHA256:     strings.Repeat("5", 64),
			SourceArtifactByteLength: uint64(len(source)), SourceRowCount: 1,
		},
		bytes.NewReader(source),
		func(context.Context, FundsCanonicalCSVNativeBuildContextV1) (FundsCanonicalCSVNativeBuildResultV1, error) {
			buildCalls++
			return FundsCanonicalCSVNativeBuildResultV1{}, nil
		},
		func(FundsCanonicalCSVAdmissionMaterialV1) error {
			emitCalls++
			return nil
		},
	)
	if err == nil || buildCalls != 0 || emitCalls != 0 {
		t.Fatalf("changed source crossed admission: err=%v build=%d emit=%d", err, buildCalls, emitCalls)
	}
}

func TestFundsCanonicalCSVParsedConfigurationUsesSharedImportBudget(t *testing.T) {
	configuration := NewFundsTransactionParsedConfigurationV1()
	if configuration.MaxRows != FundsTransactionCSVMaxRowsV1 ||
		ValidateFundsTransactionParsedConfigurationV1(configuration) != nil {
		t.Fatalf("funds parsed row budget drifted: %#v", configuration)
	}
	if FundsTransactionCSVMaxRowsV1 != 100_000 {
		t.Fatalf("funds parsed row budget = %d, want 100000", FundsTransactionCSVMaxRowsV1)
	}
	columns := FundsTransactionCSVColumnsV1()
	if len(columns) != 34 {
		t.Fatalf("funds canonical mapping width = %d, want 34", len(columns))
	}
	columns[0].Header = "mutated"
	if FundsTransactionCSVColumnsV1()[0].Header == "mutated" {
		t.Fatal("exported canonical mapping was not defensive")
	}
}

func mustFundsCanonicalCSVSourceFileIDV1(
	t *testing.T,
	build FundsCanonicalCSVNativeBuildContextV1,
) string {
	t.Helper()
	value, err := DeriveFundsCanonicalCSVSourceFileIDV1(build.Binding.CaseID, build.PrivateImportFileID)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func fundsCanonicalCSVAdmissionMaterializationV1(
	t *testing.T,
	build FundsCanonicalCSVNativeBuildContextV1,
) domainsecurity.FundsMaterializationResultV1 {
	t.Helper()
	producer, err := domainsecurity.NewFundsProducerContentManifestV1(
		domainsecurity.FundsProducerContentManifestInputV1{
			CaseID: build.Binding.CaseID, SourceRevision: 1,
			RawManifestSHA256:       build.RawArtifactManifestSHA256,
			NormalizedContentSHA256: strings.Repeat("b", 64),
			DetailContentSHA256:     strings.Repeat("c", 64),
			AggregateContentSHA256:  strings.Repeat("d", 64),
			KeywordContentSHA256:    strings.Repeat("e", 64),
			AccountContentSHA256:    strings.Repeat("f", 64),
			NormalizedRowCount:      1, AcceptedRowCount: 1, DetailRowCount: 1,
			AggregateRowCount: 1, KeywordRowCount: 1, AccountRowCount: 1,
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
		CleanedStatus: "done", FileID: build.PrivateImportFileID, RowsImportedNorm: 1,
		SHA256: build.SourceArtifactSHA256, Status: "已完成",
	}}
	rawSourceBody, err := json.Marshal(rawSources)
	if err != nil {
		t.Fatal(err)
	}
	materialization := domainsecurity.FundsMaterializationResultV1{
		CaseID: build.Binding.CaseID, RowCount: 1,
		AggregateName:                        domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		AggregateVersion:                     domainsecurity.FundsMaterializationAggregateVersionV1,
		MaterializationIdentity:              domainsecurity.FundsMaterializationIdentityPrefixV1 + strings.Repeat("0", 64),
		MaterializationIdentitySchemaVersion: domainsecurity.FundsMaterializationIdentitySchemaVersionV1,
		ProducerContentID:                    domainsecurity.DeriveFundsProducerContentIDV1(producer),
		ProducerContentManifestBase64:        base64.StdEncoding.EncodeToString(producerBody),
		ProducerContentManifestSHA256:        domainsecurity.SHA256Hex(producerBody),
		ProducerContentManifestByteLength:    uint64(len(producerBody)),
		RawArtifactManifestSHA256:            build.RawArtifactManifestSHA256,
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
