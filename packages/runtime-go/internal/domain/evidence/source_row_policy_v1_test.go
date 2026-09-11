package evidence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceRowProducerPolicyV1IsClosedAndFactIncapable(t *testing.T) {
	legacy, legacyOK := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1)
	if !legacyOK || ValidateSourceRowProducerPolicyV1(legacy) != nil ||
		legacy.Operation != SourceRowProducerOperationFundsLegacyPageV1 ||
		legacy.OperationSchemaHash != "0e7e537d10dcc73e904224272dd6dec0c90aabda0bd553e3fb576c904ed9f261" ||
		legacy.PolicyDigest != "670db878875c706a035f723c4a1c28a6cb7b6c3c48ad9ab0ad65b836a6a16a96" {
		t.Fatalf("immutable legacy source-row policy drifted: %#v", legacy)
	}
	policy, ok := ResolveSourceRowProducerPolicyV1(FundsCanonicalTransactionSourceRowPolicyIDV1)
	if !ok || ValidateSourceRowProducerPolicyV1(policy) != nil {
		t.Fatal("registered canonical source row policy was unavailable")
	}
	if SourceRowProducerOperationFundsPageV1 != "funds.transaction_source_row_page_v1" {
		t.Fatal("funds source-row operation constant is not the exact native command")
	}
	if policy.Relation != "fc_transaction_raw" || policy.ProducerComponentID != "data-engine" || !policy.ReadOnly ||
		policy.Operation != SourceRowProducerOperationFundsPageV1 {
		t.Fatalf("source row policy selected the wrong producer boundary: %#v", policy)
	}
	if policy.ActivationState != SourceRowProducerActivationTargetInactiveV1 || policy.CanMintClaims ||
		len(policy.AllowedClaimTypes) != 0 || SourceRowProducerPolicyAllowsClaimV1(policy, ClaimAccount) {
		t.Fatal("row-existence policy incorrectly authorized an account or other semantic claim")
	}
	if policy.MaxRowsPerPage != 100 || policy.MaxPageBytes != 1024*1024 {
		t.Fatal("inactive producer target exceeds the closed native response budget")
	}
	var operationSchema map[string]any
	if err := json.Unmarshal(fundsTransactionSourceRowOperationSchemaBytesV1(), &operationSchema); err != nil {
		t.Fatal(err)
	}
	inputSchema, inputOK := operationSchema["inputSchema"].(map[string]any)
	properties, propertiesOK := inputSchema["properties"].(map[string]any)
	maxRows, maxRowsOK := properties["maxRows"].(map[string]any)
	_, cursorOK := properties["cursor"].(map[string]any)
	_, identityOK := properties["parsedGenerationIdentitySha256"].(map[string]any)
	if !inputOK || !propertiesOK || len(properties) != 6 || !maxRowsOK || !cursorOK || !identityOK ||
		maxRows["minimum"] != float64(1) || maxRows["maximum"] != float64(policy.MaxRowsPerPage) ||
		maxRows["const"] != nil || properties["pageNumber"] != nil ||
		strings.Contains(string(fundsTransactionSourceRowOperationSchemaBytesV1()), `"lineage"`) ||
		strings.Contains(string(fundsTransactionSourceRowOperationSchemaBytesV1()), `"parsedGenerationSha256"`) {
		t.Fatalf("producer request did not freeze the canonical page width: %#v", maxRows)
	}
	outputSchema := operationSchema["outputSchema"].(map[string]any)
	outputProperties := outputSchema["properties"].(map[string]any)
	operation := outputProperties["operation"].(map[string]any)
	if len(outputProperties) != 26 || operation["const"] != policy.Operation || outputProperties["nextCursor"] == nil ||
		outputProperties["inventory"] == nil || outputProperties["sourceSnapshot"] == nil ||
		outputProperties["hostRawReplayRequired"] == nil {
		t.Fatal("producer output schema did not match the exact paged native result")
	}
	if policy.LocatorOrdering != SourceRowProducerLocatorOrderingV1 {
		t.Fatal("producer policy does not require the host-verifiable strict locator order")
	}
	if len(policy.RequiredSourceLocatorFields) != 3 || strings.Join(policy.RequiredSourceLocatorFields, ",") != "case_id,file_id,row_no" ||
		len(policy.LegacyLineageHintFields) != 1 || policy.LegacyLineageHintFields[0] != "row_hash" {
		t.Fatalf("source locator and legacy hints were conflated: %#v", policy)
	}
	body, err := SourceRowProducerPolicyV1Bytes(policy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSourceRowProducerPolicyV1(body)
	if err != nil || parsed.PolicyDigest != policy.PolicyDigest || parsed.AllowedClaimTypes == nil {
		t.Fatalf("canonical policy round-trip failed: %v", err)
	}
	for _, forbidden := range []string{"entityIdPathTemplate", "accountIdPathTemplate", "normalized_transactions_v11", "fc_transaction_norm"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("policy invented an unavailable source field or relation: %s", forbidden)
		}
	}
}

func TestGoCountSourceRowPolicyV1IsExactAndCountOnly(t *testing.T) {
	policy, ok := ResolveSourceRowProducerPolicyV1(FundsGoCountSourceRowPolicyIDV1)
	validationErr := ValidateSourceRowProducerPolicyV1(policy)
	if !ok || validationErr != nil {
		t.Fatalf("registered Go count source row policy was unavailable: ok=%v err=%v policy=%#v", ok, validationErr, policy)
	}
	legacy, _ := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1)
	if policy.PolicyDigest == legacy.PolicyDigest ||
		policy.ProducerComponentID != SourceRowProducerComponentRuntimeGoV1 ||
		policy.ProducerComponentVersion != "1.0.0" ||
		policy.Operation != SourceRowProducerOperationGoCountPageV1 ||
		policy.Relation != SourceRowProducerRelationGoCountV1 ||
		policy.ParserID != "analytix.strict-utf8-csv" ||
		policy.ParserVersion != "1" {
		t.Fatalf("Go count policy identity drifted or impersonated the legacy producer: %#v", policy)
	}
	if policy.ActivationState != SourceRowProducerActivationActiveV1 || !policy.CanMintClaims ||
		len(policy.AllowedClaimTypes) != 1 || policy.AllowedClaimTypes[0] != ClaimCount ||
		!SourceRowProducerPolicyAllowsClaimV1(policy, ClaimCount) ||
		SourceRowProducerPolicyAllowsClaimV1(policy, ClaimAmount) ||
		SourceRowProducerPolicyAllowsClaimV1(policy, ClaimAccount) {
		t.Fatalf("Go source row policy exceeded its count-only capability: %#v", policy)
	}
	if policy.LegacyLineageHintFields == nil || len(policy.LegacyLineageHintFields) != 0 {
		t.Fatal("Go producer inherited a legacy lineage hint")
	}
	body, err := SourceRowProducerPolicyV1Bytes(policy)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseSourceRowProducerPolicyV1(body)
	if err != nil || !equalSourceRowProducerPolicyV1(parsed, policy) {
		t.Fatalf("Go count policy canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	var operationSchema map[string]any
	if err := json.Unmarshal(goCountSourceRowOperationSchemaBytesV1(
		SourceRowProducerRelationGoCountV1,
		policy.ParserID,
		policy.ParserVersion,
	), &operationSchema); err != nil {
		t.Fatal(err)
	}
	if policy.OperationSchemaHash != fundsGoCountSourceRowOperationSchemaHashV1() {
		t.Fatal("Go count operation schema hash drifted")
	}
	input := operationSchema["inputSchema"].(map[string]any)
	inputProperties := input["properties"].(map[string]any)
	output := operationSchema["outputSchema"].(map[string]any)
	outputProperties := output["properties"].(map[string]any)
	if inputProperties["relation"].(map[string]any)["const"] != policy.Relation ||
		outputProperties["relation"].(map[string]any)["const"] != policy.Relation ||
		outputProperties["parserId"].(map[string]any)["const"] != policy.ParserID ||
		outputProperties["parserVersion"].(map[string]any)["const"] != policy.ParserVersion {
		t.Fatal("Go count operation schema did not bind its exact producer profile")
	}

	tampered := cloneSourceRowProducerPolicyV1(policy)
	tampered.AllowedClaimTypes = []ClaimType{ClaimAccount}
	tampered.PolicyDigest = sourceRowProducerPolicyDigestV1(tampered)
	if ValidateSourceRowProducerPolicyV1(tampered) == nil ||
		SourceRowProducerPolicyAllowsClaimV1(tampered, ClaimAccount) {
		t.Fatal("caller-mutated Go policy expanded claim authority")
	}
	fresh, ok := ResolveSourceRowProducerPolicyV1(FundsGoCountSourceRowPolicyIDV1)
	if !ok || !SourceRowProducerPolicyAllowsClaimV1(fresh, ClaimCount) ||
		SourceRowProducerPolicyAllowsClaimV1(fresh, ClaimAccount) {
		t.Fatal("caller mutation changed the package-owned Go policy")
	}
}

func TestSourceRowProducerPolicyV1RejectsDynamicExtensionAndMutation(t *testing.T) {
	if _, ok := ResolveSourceRowProducerPolicyV1("mcp.reported-policy/v1"); ok {
		t.Fatal("unknown MCP-reported policy entered the closed registry")
	}
	if _, ok := ResolveSourceRowProducerPolicyV1(" " + FundsTransactionSourceRowPolicyIDV1); ok {
		t.Fatal("noncanonical policy id entered the closed registry")
	}
	policy, _ := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1)
	tampered := policy
	tampered.AllowedClaimTypes = []ClaimType{ClaimAccount}
	tampered.PolicyDigest = sourceRowProducerPolicyDigestV1(tampered)
	if ValidateSourceRowProducerPolicyV1(tampered) == nil {
		t.Fatal("caller-mutated policy acquired a semantic capability")
	}
	tampered = policy
	tampered.RecordPathTemplate = "/_meta/rows/{ordinal}"
	tampered.PolicyDigest = sourceRowProducerPolicyDigestV1(tampered)
	if ValidateSourceRowProducerPolicyV1(tampered) == nil {
		t.Fatal("caller-mutated metadata path entered the registered policy")
	}
	tampered = policy
	tampered.ActivationState = "active"
	tampered.CanMintClaims = true
	tampered.PolicyDigest = sourceRowProducerPolicyDigestV1(tampered)
	if ValidateSourceRowProducerPolicyV1(tampered) == nil || SourceRowProducerPolicyAllowsClaimV1(tampered, ClaimAccount) {
		t.Fatal("caller mutation activated an uncomposed target policy")
	}
	tampered = cloneSourceRowProducerPolicyV1(policy)
	tampered.RequiredSourceLocatorFields[0] = "attacker_field"
	if fresh, ok := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1); !ok ||
		fresh.RequiredSourceLocatorFields[0] != "case_id" {
		t.Fatal("resolved policy slices shared mutable state")
	}
	body, err := SourceRowProducerPolicyV1Bytes(policy)
	if err != nil {
		t.Fatalf("serialize policy: %v", err)
	}
	var external map[string]any
	if err := json.Unmarshal(body, &external); err != nil {
		t.Fatal(err)
	}
	external["providerActivated"] = true
	unknown, _ := json.Marshal(external)
	if _, err := ParseSourceRowProducerPolicyV1(unknown); err == nil {
		t.Fatal("unknown policy field bypassed the closed external schema")
	}
}

func TestSourceRowPolicyTextAndPathsAreCanonicalAndBounded(t *testing.T) {
	policy, _ := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1)
	binding := sourceRowTestBindingV1(t, "case-paths")
	record := sourceRowTestRecordV1(t, policy, binding, "file-001", 13, "artifact-path", "row-path", "lineage-path")
	entry, err := newSourceRowLedgerPageEntryV1(policy, binding, 12, record)
	if err != nil {
		t.Fatal(err)
	}
	if entry.RecordPath != "/rows/12" || entry.RecordIDPath != "/rows/12/sourceRecordId" ||
		entry.SourceFileIDPath != "/rows/12/sourceFileId" || entry.SourceRowNumberPath != "/rows/12/sourceRowNumber" {
		t.Fatalf("host paths were not derived from the registered template: %#v", entry)
	}
	for _, sourceFileID := range []string{" file", "file\x00id", strings.Repeat("x", maxSourceRowPolicyTextBytesV1+1)} {
		if _, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{
			SourceFileID: sourceFileID, SourceRowNumber: 1,
		}); err == nil {
			t.Fatalf("invalid source file id was accepted: %q", sourceFileID)
		}
	}
}
