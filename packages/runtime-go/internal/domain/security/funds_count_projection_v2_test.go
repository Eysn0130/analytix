package security

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFundsCountProjectionV2IsStrictPathFreeAndPIIFree(t *testing.T) {
	manifestDigest := SHA256Hex([]byte("funds-count-manifest"))
	projection, err := NewFundsCountProjectionV2(FundsCountProjectionInputV2{
		TurnSecurityContextDigest:          SHA256Hex([]byte("funds-count-context")),
		DatasetSnapshotID:                  DatasetSnapshotIDPrefixV2 + manifestDigest,
		DatasetSelectionDigest:             SHA256Hex([]byte("funds-count-selection")),
		DatasetRecordDigest:                SHA256Hex([]byte("funds-count-record")),
		DatasetManifestDigest:              manifestDigest,
		FundsProducerContentID:             FundsProducerContentIDPrefixV1 + SHA256Hex([]byte("funds-count-producer-id")),
		FundsProducerContentManifestSHA256: SHA256Hex([]byte("funds-count-producer-manifest")),
		DetailContentSHA256:                SHA256Hex([]byte("funds-count-detail")),
		RowCount:                           2645472,
	})
	if err != nil || ValidateFundsCountProjectionV2(projection) != nil ||
		projection.RowCount != "2645472" || projection.TableName != FundsCountProjectionTableV2 {
		t.Fatalf("valid funds projection failed: projection=%#v err=%v", projection, err)
	}
	record, err := FundsCountProjectionV2Record(projection)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFundsCountProjectionV2(record)
	if err != nil || parsed != projection {
		t.Fatalf("funds projection did not round trip: parsed=%#v err=%v", parsed, err)
	}
	body := string(mustJSONForFundsCountProjectionTest(t, record))
	for _, forbidden := range []string{
		"/workspace", "workspaceRealPath", "caseId", "tenantId", "userId",
		"account", "card", "database", "duckdb", "pluginRoot", "path",
	} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
			t.Fatalf("funds projection leaked forbidden material %q: %s", forbidden, body)
		}
	}
}

func TestFundsCountProjectionV2AcceptsOnlyClosedProducerContentVersions(t *testing.T) {
	manifestDigest := SHA256Hex([]byte("funds-count-versioned-manifest"))
	input := FundsCountProjectionInputV2{
		TurnSecurityContextDigest:          SHA256Hex([]byte("funds-count-versioned-context")),
		DatasetSnapshotID:                  DatasetSnapshotIDPrefixV2 + manifestDigest,
		DatasetSelectionDigest:             SHA256Hex([]byte("funds-count-versioned-selection")),
		DatasetRecordDigest:                SHA256Hex([]byte("funds-count-versioned-record")),
		DatasetManifestDigest:              manifestDigest,
		FundsProducerContentManifestSHA256: SHA256Hex([]byte("funds-count-versioned-producer-manifest")),
		DetailContentSHA256:                SHA256Hex([]byte("funds-count-versioned-detail")),
		RowCount:                           1,
	}
	for _, prefix := range []string{FundsProducerContentIDPrefixV1, FundsProducerContentIDPrefixV2} {
		input.FundsProducerContentID = prefix + SHA256Hex([]byte(prefix))
		if _, err := NewFundsCountProjectionV2(input); err != nil {
			t.Fatalf("closed producer content version %q was rejected: %v", prefix, err)
		}
	}
	input.FundsProducerContentID = "fpc3_" + SHA256Hex([]byte("future"))
	if _, err := NewFundsCountProjectionV2(input); err == nil {
		t.Fatal("projection accepted an unrecognized producer content version")
	}
}

func TestFundsCountProjectionV2RejectsDriftUnknownAndNonCanonicalCount(t *testing.T) {
	manifestDigest := SHA256Hex([]byte("funds-count-drift-manifest"))
	base, err := NewFundsCountProjectionV2(FundsCountProjectionInputV2{
		TurnSecurityContextDigest:          SHA256Hex([]byte("funds-count-drift-context")),
		DatasetSnapshotID:                  DatasetSnapshotIDPrefixV2 + manifestDigest,
		DatasetSelectionDigest:             SHA256Hex([]byte("funds-count-drift-selection")),
		DatasetRecordDigest:                SHA256Hex([]byte("funds-count-drift-record")),
		DatasetManifestDigest:              manifestDigest,
		FundsProducerContentID:             FundsProducerContentIDPrefixV1 + SHA256Hex([]byte("funds-count-drift-producer-id")),
		FundsProducerContentManifestSHA256: SHA256Hex([]byte("funds-count-drift-producer-manifest")),
		DetailContentSHA256:                SHA256Hex([]byte("funds-count-drift-detail")),
		RowCount:                           2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*FundsCountProjectionV2){
		"context": func(value *FundsCountProjectionV2) {
			value.TurnSecurityContextDigest = SHA256Hex([]byte("other-context"))
		},
		"snapshot": func(value *FundsCountProjectionV2) {
			value.DatasetSnapshotID = DatasetSnapshotIDPrefixV2 + SHA256Hex([]byte("other-manifest"))
		},
		"selection": func(value *FundsCountProjectionV2) {
			value.DatasetSelectionDigest = SHA256Hex([]byte("other-selection"))
		},
		"record": func(value *FundsCountProjectionV2) {
			value.DatasetRecordDigest = SHA256Hex([]byte("other-record"))
		},
		"manifest": func(value *FundsCountProjectionV2) {
			value.DatasetManifestDigest = SHA256Hex([]byte("other-manifest"))
		},
		"producer": func(value *FundsCountProjectionV2) {
			value.FundsProducerContentID = FundsProducerContentIDPrefixV1 + SHA256Hex([]byte("other-producer"))
		},
		"producer manifest": func(value *FundsCountProjectionV2) {
			value.FundsProducerContentManifestSHA256 = SHA256Hex([]byte("other-producer-manifest"))
		},
		"detail": func(value *FundsCountProjectionV2) {
			value.DetailContentSHA256 = SHA256Hex([]byte("other-detail"))
		},
		"table": func(value *FundsCountProjectionV2) {
			value.TableName = "other_table"
		},
		"count": func(value *FundsCountProjectionV2) {
			value.RowCount = "02"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if ValidateFundsCountProjectionV2(changed) == nil {
				t.Fatalf("projection accepted %s drift: %#v", name, changed)
			}
		})
	}
	record, _ := FundsCountProjectionV2Record(base)
	record["safeToAnswer"] = true
	if _, err := ParseFundsCountProjectionV2(record); err == nil {
		t.Fatal("funds projection accepted an authority-like extra field")
	}
}

func mustJSONForFundsCountProjectionTest(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
