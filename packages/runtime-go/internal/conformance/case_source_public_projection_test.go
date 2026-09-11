package conformance_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func TestGoCaseSourcePublicProjectionMatchesSharedStrictFixture(t *testing.T) {
	fixtureBody, err := os.ReadFile("../../../runtime/src/conformance/fixtures/go-case-source-public-projection-v1.json")
	if err != nil {
		t.Fatalf("read shared case-source projection fixture: %v", err)
	}
	var fixture any
	if err := json.Unmarshal(fixtureBody, &fixture); err != nil {
		t.Fatalf("decode shared case-source projection fixture: %v", err)
	}
	expected, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"contract":      "analytix.go-case-source-public-projection/v1",
		"projections": map[string]any{
			"completed": domaintoolresult.PublicToolResultProjectionRecordV1(
				toolcatalogapp.BuildPublicToolResultProjectionV1("mcp__analytix-fund-analysis__query_transactions", map[string]any{
					"code": "case_source_result_private", "executed": true, "isError": false,
				}, false),
			),
			"failed": domaintoolresult.PublicToolResultProjectionRecordV1(
				toolcatalogapp.BuildPublicToolResultProjectionV1("mcp__analytix-fund-analysis__query_transactions", map[string]any{
					"code": "case_source_result_private", "executed": false, "isError": true,
				}, true),
			),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("Go projection drifted from shared strict fixture:\nactual:   %s\nexpected: %s", actual, expected)
	}
	for _, forbidden := range []string{
		"caseOutcome", "caseSourceBindingProof", domaintoolresult.CaseSourceBindingProofPurposeV1,
		"toolName", "toolCallId", "contextDigest", "executionGrantId", "contextEpoch",
		"datasetSnapshotId", "transportStatus", "semanticStatus", "safeToAnswer", "evidenceReceipt",
		"AuthorityRef", "6222020202020202020", "/private/case.sqlite", "SELECT * FROM private_case",
		"PRIVATE_REVERSE_MAP", "PRIVATE_PROVIDER_BODY",
	} {
		if strings.Contains(string(actual), forbidden) {
			t.Fatalf("shared Go public projection retained forbidden bytes %q: %s", forbidden, actual)
		}
	}
}
