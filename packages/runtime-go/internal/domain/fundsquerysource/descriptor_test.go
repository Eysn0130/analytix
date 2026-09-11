package fundsquerysource

import (
	"bytes"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestDescriptorV1IsCanonicalPathFreeAndFixedProfile(t *testing.T) {
	descriptor := testDescriptorV1(t, "canonical")
	body, err := DescriptorV1Bytes(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDescriptorV1(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed != descriptor ||
		descriptor.QueryProfileDigest != FixedFundsQueryProfileDigestV1() ||
		descriptor.SchemaDigest != FixedFundsAnalyticalSchemaDigestV1() {
		t.Fatal("funds query source descriptor did not round-trip exactly")
	}
	const fixedReadOnlyProfile = "b7378259bc3c7646d0606b640c1c53d53491381d78f24d8f964e440d39d0e674"
	if descriptor.QueryProfileDigest != fixedReadOnlyProfile {
		t.Fatalf("fixed funds query profile drifted: %s", descriptor.QueryProfileDigest)
	}
	const accountFlowOnlyProfile = "6947a3f72aed925f9032b7f2ac29afb854d5efe051fa14d48860c9dd894fd0cf"
	const localDisplayProfile = "bb87d6dd873cbdfc5467d0d43077234f2a164ddc2a2307a89b19f1b227927f38"
	if FixedAccountFlowQueryProfileDigestV1() != accountFlowOnlyProfile ||
		FixedFundsLocalDisplayQueryProfileDigestV1() != localDisplayProfile ||
		!QueryProfileSupportsAccountFlowV1(accountFlowOnlyProfile) ||
		QueryProfileSupportsAccountIngressResolutionV1(accountFlowOnlyProfile) ||
		QueryProfileSupportsDirectSourcePreviewV1(accountFlowOnlyProfile) ||
		!QueryProfileSupportsAccountFlowV1(fixedReadOnlyProfile) ||
		!QueryProfileSupportsAccountIngressResolutionV1(fixedReadOnlyProfile) ||
		QueryProfileSupportsDirectSourcePreviewV1(fixedReadOnlyProfile) ||
		!QueryProfileSupportsAccountFlowV1(localDisplayProfile) ||
		!QueryProfileSupportsAccountIngressResolutionV1(localDisplayProfile) ||
		!QueryProfileSupportsDirectSourcePreviewV1(localDisplayProfile) {
		t.Fatal("fixed profile capability compatibility drifted")
	}
	const fixedAnalyticalSchema = "81a474553b16799c12d8d85c28d99f25ba6c9b40ae2073701443d2898f91e11e"
	if FixedFundsAnalyticalSchemaDigestV1() != fixedAnalyticalSchema ||
		FixedFundsAnalyticalSchemaDigestV1() != domainsecurity.FixedFundsAnalyticalSchemaDigestV1() {
		t.Fatalf("fixed analytical schema drifted: %s", FixedFundsAnalyticalSchemaDigestV1())
	}
	for _, forbidden := range []string{
		"workspaceRealPath",
		"databasePath",
		"duckdbPath",
		"opaqueObjectAddress",
		"selectionDigest",
		"contextEpoch",
		"sql",
		"queryText",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("path-free descriptor contains forbidden field %q", forbidden)
		}
	}
}

func TestDescriptorV1PreservesAcceptedFixedManifestProfiles(t *testing.T) {
	for _, profile := range []string{
		FixedAccountFlowQueryProfileDigestV1(),
		FixedFundsQueryProfileDigestV1(),
		FixedFundsLocalDisplayQueryProfileDigestV1(),
	} {
		input := testDescriptorInputV1("profile-round-trip")
		input.QueryProfileDigest = profile
		descriptor, err := NewDescriptorV1(input)
		if err != nil {
			t.Fatalf("construct profile %s: %v", profile, err)
		}
		body, err := DescriptorV1Bytes(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseDescriptorV1(body)
		if err != nil || parsed.QueryProfileDigest != profile {
			t.Fatalf("profile was not preserved: got=%s want=%s err=%v", parsed.QueryProfileDigest, profile, err)
		}
	}

	missing := testDescriptorInputV1("missing-profile")
	missing.QueryProfileDigest = ""
	if _, err := NewDescriptorV1(missing); err == nil {
		t.Fatal("descriptor constructor accepted a missing manifest query profile")
	}
}

func TestDescriptorV1RejectsDriftAndUnknownFields(t *testing.T) {
	descriptor := testDescriptorV1(t, "drift")
	arbitrarySchema := testDescriptorInputV1("arbitrary-schema")
	arbitrarySchema.SchemaDigest = testDigestV1("caller-selected-schema")
	if _, err := NewDescriptorV1(arbitrarySchema); err == nil {
		t.Fatal("descriptor constructor accepted a caller-selected schema digest")
	}
	tests := []struct {
		name   string
		mutate func(*DescriptorV1)
	}{
		{
			name: "snapshot record",
			mutate: func(value *DescriptorV1) {
				value.SnapshotRecordDigest = testDigestV1("another-snapshot-record")
			},
		},
		{
			name: "query profile",
			mutate: func(value *DescriptorV1) {
				value.QueryProfileDigest = testDigestV1("another-profile")
			},
		},
		{
			name: "duckdb length",
			mutate: func(value *DescriptorV1) {
				value.DuckDBByteLength++
			},
		},
		{
			name: "duckdb content snapshot",
			mutate: func(value *DescriptorV1) {
				value.DuckDBContentSnapshotDigest = testDigestV1("another-content-snapshot")
			},
		},
		{
			name: "duckdb snapshot manifest",
			mutate: func(value *DescriptorV1) {
				value.DuckDBSnapshotManifestSHA256 = testDigestV1("another-duckdb-manifest")
			},
		},
		{
			name: "schema",
			mutate: func(value *DescriptorV1) {
				value.SchemaDigest = testDigestV1("another-schema")
			},
		},
		{
			name: "materialization identity",
			mutate: func(value *DescriptorV1) {
				value.MaterializationIdentity = "txn_daily_snapshot:v12:" + testDigestV1("another-materialization")
			},
		},
		{
			name: "fpc contract",
			mutate: func(value *DescriptorV1) {
				value.FundsProducerContentID =
					domainsecurity.FundsProducerContentIDPrefixV2 +
						testDigestV1("count-canary")
			},
		},
		{
			name: "dataset timezone",
			mutate: func(value *DescriptorV1) {
				value.DatasetUTCOffsetMinutes = 841
			},
		},
		{
			name: "currency",
			mutate: func(value *DescriptorV1) {
				value.ExpectedCurrency = "cny"
			},
		},
		{
			name: "minor unit scale",
			mutate: func(value *DescriptorV1) {
				value.MinorUnitScale = 3
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := descriptor
			test.mutate(&candidate)
			if err := ValidateDescriptorV1(candidate); err == nil {
				t.Fatal("mutated descriptor was accepted")
			}
		})
	}

	body, err := DescriptorV1Bytes(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := bytes.Replace(
		body,
		[]byte(`{"schemaVersion":1,`),
		[]byte(`{"schemaVersion":1,"databasePath":"/private/case.duckdb",`),
		1,
	)
	if _, err := ParseDescriptorV1(withUnknown); err == nil {
		t.Fatal("descriptor parser accepted a raw database path field")
	}
}

func testDescriptorV1(t *testing.T, seed string) DescriptorV1 {
	t.Helper()
	descriptor, err := NewDescriptorV1(testDescriptorInputV1(seed))
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func testDescriptorInputV1(seed string) DescriptorInputV1 {
	return DescriptorInputV1{
		SnapshotRecordDigest:     testDigestV1("snapshot-record:" + seed),
		DatasetSnapshotID:        domainsecurity.DatasetSnapshotIDPrefixV2 + testDigestV1("snapshot:"+seed),
		SourceManifestHash:       testDigestV1("source-manifest:" + seed),
		CaseID:                   "case-" + seed,
		CaseBindingHash:          testDigestV1("case-binding:" + seed),
		DatasetBindingDigest:     testDigestV1("dataset-binding:" + seed),
		BindingObservationDigest: testDigestV1("binding-observation:" + seed),
		FundsProducerContentID: domainsecurity.FundsProducerContentIDPrefixV1 +
			testDigestV1("fpc1:"+seed),
		FundsProducerContentManifestSHA256:     testDigestV1("fpc1-manifest:" + seed),
		FundsProducerContentManifestByteLength: 2048,
		DuckDBSHA256:                           testDigestV1("duckdb:" + seed),
		DuckDBByteLength:                       8192,
		DatasetUTCOffsetMinutes:                0,
		ExpectedCurrency:                       "CNY",
		MinorUnitScale:                         AccountFlowMinorUnitScaleV1,
		DuckDBContentSnapshotDigest:            testDigestV1("duckdb-content-snapshot:" + seed),
		DuckDBSnapshotManifestSHA256:           testDigestV1("duckdb-snapshot-manifest:" + seed),
		MaterializationIdentity:                "txn_daily_snapshot:v12:" + testDigestV1("source-signature:"+seed),
		SchemaDigest:                           FixedFundsAnalyticalSchemaDigestV1(),
		QueryProfileDigest:                     FixedFundsQueryProfileDigestV1(),
	}
}

func testDigestV1(material string) string {
	return domainsecurity.SHA256Hex([]byte("funds-query-source-test:\x00" + material))
}
