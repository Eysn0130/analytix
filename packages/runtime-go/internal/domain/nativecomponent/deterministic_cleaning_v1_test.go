package nativecomponent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestDeterministicCleaningContractRejectsRuleSelectorCollisionAndBoundsDrift(t *testing.T) {
	descriptor := deterministicCleaningDescriptorForTestV1(t)
	arguments, err := NewDeterministicCleaningArgumentsV1(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := EncodeDeterministicCleaningNativeRequestFrameV1(strings.Repeat("a", 64), arguments)
	if err != nil || strings.Contains(string(frame), "dbPath") || strings.Contains(string(frame), "/tmp/") {
		t.Fatalf("cleaning request was not path-free and fixed: %s err=%v", frame, err)
	}
	wrongRule := arguments
	wrongRule.RuleDigest = strings.Repeat("b", 64)
	if ValidateDeterministicCleaningArgumentsV1(wrongRule) == nil {
		t.Fatal("wrong rule digest was accepted")
	}
	wrongSnapshot := arguments
	wrongSnapshot.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("wrong")
	if ValidateDeterministicCleaningArgumentsAuthorityV1(wrongSnapshot, descriptor) == nil {
		t.Fatal("wrong input snapshot was accepted")
	}

	valid := deterministicCleaningWireForTestV1(arguments)
	for name, mutate := range map[string]func(*deterministicCleaningResultWireV1){
		"noncanonical output digest": func(value *deterministicCleaningResultWireV1) {
			value.OutputArtifactSHA256 = strings.Repeat("C", 64)
		},
		"row overflow": func(value *deterministicCleaningResultWireV1) {
			value.RowCount = DeterministicCleaningMaximumRowsV1 + 1
		},
		"duplicate row": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows = append(value.ChangedRows, value.ChangedRows[0])
			value.ChangedRowCount++
			value.UnchangedRowCount--
		},
		"duplicate field": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows[0].Cells = append(value.ChangedRows[0].Cells, value.ChangedRows[0].Cells[0])
		},
		"noncanonical field order": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows[0].Cells = append(value.ChangedRows[0].Cells, deterministicCleaningChangedCellWireV1{
				Field: DirectSourcePreviewFieldTransactionTimeV1, BeforeValue: "2026/08/22 01:02:03",
				AfterValue: "2026-08-22 01:02:03", BeforeState: "value", AfterState: "value",
			})
		},
		"unchanged row in changed inventory": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows[0].Status = "unchanged"
		},
		"changed row without projected cells": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows[0].Cells = nil
		},
		"missing state with bytes": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows[0].Cells[0].BeforeState = "missing"
		},
		"unchanged cell": func(value *deterministicCleaningResultWireV1) {
			value.ChangedRows[0].Cells[0].AfterValue = value.ChangedRows[0].Cells[0].BeforeValue
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.ChangedRows = append([]deterministicCleaningChangedRowWireV1(nil), valid.ChangedRows...)
			candidate.ChangedRows[0].Cells = append([]deterministicCleaningChangedCellWireV1(nil), valid.ChangedRows[0].Cells...)
			mutate(&candidate)
			candidate.ResultDigest = deterministicCleaningResultDigestV1(deterministicCleaningDigestMaterialForTestV1(candidate))
			body, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, parseErr := ParseDeterministicCleaningResultV1(body, arguments); parseErr == nil {
				t.Fatalf("invalid cleaning result was accepted: %s", body)
			}
		})
	}
}

func TestDeterministicCleaningPrivateResultRejectsJSONAndSafeFormatsExactValues(t *testing.T) {
	const exactCanary = "EXACT_BEFORE_AFTER_CANARY"
	cell := DeterministicCleaningChangedCellV1{
		Field:       DirectSourcePreviewFieldCounterpartyAccountV1,
		beforeValue: exactCanary + "-before", afterValue: exactCanary + "-after",
		beforeState: "value", afterState: "value",
	}
	row := DeterministicCleaningChangedRowV1{RowIndex: 7, Status: "changed", Cells: []DeterministicCleaningChangedCellV1{cell}}
	result := DeterministicCleaningResultV1{
		SchemaVersion: 1, Operation: OperationFundsDeterministicCleaningV1,
		InputDatasetSnapshotID: "dsv2_" + strings.Repeat("1", 64),
		RuleGeneration:         DeterministicCleaningRuleGenerationV1(), RuleDigest: DeterministicCleaningRuleDigestV1(),
		OutputArtifactSHA256: strings.Repeat("2", 64), OutputArtifactByteLength: 512,
		RowCount: 1, ChangedRowCount: 1, ChangedRows: []DeterministicCleaningChangedRowV1{row},
		ResultDigest: strings.Repeat("3", 64),
	}
	for name, value := range map[string]any{"cell": cell, "row": row, "result": result} {
		if body, err := json.Marshal(value); err == nil || strings.Contains(string(body), exactCanary) {
			t.Fatalf("%s private value was JSON-marshaled: body=%q err=%v", name, body, err)
		}
	}
	for name, target := range map[string]any{
		"cell":   &DeterministicCleaningChangedCellV1{},
		"row":    &DeterministicCleaningChangedRowV1{},
		"result": &DeterministicCleaningResultV1{},
	} {
		if err := json.Unmarshal([]byte(`{"beforeValue":"`+exactCanary+`"}`), target); err == nil {
			t.Fatalf("%s private value accepted JSON unmarshal", name)
		}
	}
	for _, value := range []any{cell, &cell, row, &row, result, &result} {
		for _, formatted := range []string{
			fmt.Sprintf("%v", value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value),
		} {
			if strings.Contains(formatted, exactCanary) || !strings.Contains(formatted, "PRIVATE") {
				t.Fatalf("private cleaning fmt leaked or lost marker: %q", formatted)
			}
		}
	}
}

func TestDeterministicCleaningResultBudgetMatchesStrictTokenAndByteCaps(t *testing.T) {
	arguments, err := NewDeterministicCleaningArgumentsV1(deterministicCleaningDescriptorForTestV1(t))
	if err != nil {
		t.Fatal(err)
	}
	maximumOneCellRows := (DeterministicCleaningMaximumResultTokensV1 - deterministicCleaningResultBaseTokensV1) /
		(deterministicCleaningResultRowTokensV1 + deterministicCleaningResultCellTokensV1)
	buildRows := func(count int, cells []deterministicCleaningChangedCellWireV1) deterministicCleaningResultWireV1 {
		wire := deterministicCleaningWireForTestV1(arguments)
		wire.RowCount = uint64(count)
		wire.ChangedRowCount = uint64(count)
		wire.UnchangedRowCount = 0
		wire.ChangedRows = make([]deterministicCleaningChangedRowWireV1, count)
		for index := range wire.ChangedRows {
			wire.ChangedRows[index] = deterministicCleaningChangedRowWireV1{
				RowIndex: uint32(index), Status: "changed",
				Cells: append([]deterministicCleaningChangedCellWireV1(nil), cells...),
			}
		}
		wire.ResultDigest = deterministicCleaningResultDigestV1(deterministicCleaningDigestMaterialForTestV1(wire))
		return wire
	}
	oneCell := []deterministicCleaningChangedCellWireV1{{
		Field: DirectSourcePreviewFieldRemarkV1, BeforeValue: "a", AfterValue: "b",
		BeforeState: "value", AfterState: "value",
	}}
	boundary := buildRows(maximumOneCellRows, oneCell)
	boundaryBody, err := json.Marshal(boundary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDeterministicCleaningResultV1(boundaryBody, arguments); err != nil {
		t.Fatalf("exact cleaning token boundary was rejected: rows=%d bytes=%d err=%v", maximumOneCellRows, len(boundaryBody), err)
	}
	overTokens := buildRows(maximumOneCellRows+1, oneCell)
	overTokenBody, err := json.Marshal(overTokens)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseDeterministicCleaningResultV1(overTokenBody, arguments); err == nil {
		t.Fatal("cleaning result above the shared token cap was accepted")
	}

	largeCells := make([]deterministicCleaningChangedCellWireV1, len(deterministicCleaningFieldsV1))
	for index, field := range deterministicCleaningFieldsV1 {
		largeCells[index] = deterministicCleaningChangedCellWireV1{
			Field: field, BeforeValue: strings.Repeat("<", DirectSourcePreviewMaximumCellBytesV1),
			AfterValue:  strings.Repeat(">", DirectSourcePreviewMaximumCellBytesV1),
			BeforeState: "value", AfterState: "value",
		}
	}
	withinBytes := buildRows(1, largeCells)
	withinBody, err := json.Marshal(withinBytes)
	if err != nil || len(withinBody) >= DeterministicCleaningMaximumResultBytesV1 {
		t.Fatalf("within-limit byte fixture drifted: bytes=%d err=%v", len(withinBody), err)
	}
	if _, err := ParseDeterministicCleaningResultV1(withinBody, arguments); err != nil {
		t.Fatalf("within-limit cleaning result was rejected: %v", err)
	}
	overBytes := buildRows(2, largeCells)
	overByteBody, err := json.Marshal(overBytes)
	if err != nil || len(overByteBody) <= DeterministicCleaningMaximumResultBytesV1 {
		t.Fatalf("over-limit byte fixture drifted: bytes=%d err=%v", len(overByteBody), err)
	}
	if _, err := ParseDeterministicCleaningResultV1(overByteBody, arguments); err == nil {
		t.Fatal("cleaning result above the shared byte cap was accepted")
	}
}

func TestDeterministicCleaningParserPreservesNullMissingInvalidAndExactDecimalText(t *testing.T) {
	arguments, err := NewDeterministicCleaningArgumentsV1(deterministicCleaningDescriptorForTestV1(t))
	if err != nil {
		t.Fatal(err)
	}
	wire := deterministicCleaningWireForTestV1(arguments)
	wire.ChangedRows[0].Status = "invalid"
	wire.ChangedRows[0].Cells = []deterministicCleaningChangedCellWireV1{
		{Field: DirectSourcePreviewFieldAmountTextV1, BeforeValue: "+001.20", AfterValue: "1.2", BeforeState: "value", AfterState: "value"},
		{Field: DirectSourcePreviewFieldCurrencyV1, BeforeValue: "bad", AfterValue: "bad", BeforeState: "invalid", AfterState: "value"},
		{Field: DirectSourcePreviewFieldRemarkV1, BeforeValue: "", AfterValue: "", BeforeState: "null", AfterState: "missing"},
	}
	wire.ResultDigest = deterministicCleaningResultDigestV1(deterministicCleaningDigestMaterialForTestV1(wire))
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseDeterministicCleaningResultV1(body, arguments)
	if err != nil || result.ChangedRowCount != 1 || len(result.ChangedRows[0].Cells) != 3 {
		t.Fatalf("valid stateful exact result was rejected: result=%#v err=%v", result, err)
	}
	var before, after, beforeState, afterState string
	if err := result.ChangedRows[0].Cells[0].UseExactV1(func(_ DirectSourcePreviewFieldV1, left, right, leftState, rightState string) error {
		before, after, beforeState, afterState = left, right, leftState, rightState
		return nil
	}); err != nil || before != "+001.20" || after != "1.2" || beforeState != "value" || afterState != "value" {
		t.Fatalf("exact decimal/state text changed: before=%q after=%q states=%s/%s err=%v", before, after, beforeState, afterState, err)
	}
}

func TestDeterministicCleaningResultDigestMatchesRustStringEscapingFixture(t *testing.T) {
	material := deterministicCleaningResultDigestMaterialV1{
		SchemaVersion: 1, Operation: OperationFundsDeterministicCleaningV1,
		InputDatasetSnapshotID: "dsv2_" + strings.Repeat("1", 64),
		RuleGeneration:         "tlgen1_" + strings.Repeat("2", 64),
		RuleDigest:             strings.Repeat("3", 64),
		OutputArtifactSHA256:   strings.Repeat("4", 64), OutputArtifactByteLength: 512,
		RowCount: 1, ChangedRowCount: 1, UnchangedRowCount: 0,
		ChangedRows: []deterministicCleaningChangedRowWireV1{{
			RowIndex: 0, Status: "changed",
			Cells: []deterministicCleaningChangedCellWireV1{{
				Field:       DirectSourcePreviewFieldRemarkV1,
				BeforeValue: "<&>\u2028\u2029", AfterValue: "&<>\u2029\u2028",
				BeforeState: "value", AfterState: "value",
			}},
		}},
	}
	if digest := deterministicCleaningResultDigestV1(material); digest != "4dffc9bc7135c00defa6aafc6439607244890f8e20ca1b200289dc1bd9f0e91b" {
		t.Fatalf("Go/Rust cleaning result digest fixture drifted: %s", digest)
	}
}

func deterministicCleaningDescriptorForTestV1(t *testing.T) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	digest := func(value string) string { return domainsecurity.SHA256Hex([]byte("cleaning-domain:" + value)) }
	descriptor, err := domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest: digest("record"), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("cleaning-domain"),
		SourceManifestHash: digest("source"), CaseID: "case-cleaning", CaseBindingHash: digest("binding"),
		DatasetBindingDigest: digest("dataset"), BindingObservationDigest: digest("observation"),
		FundsProducerContentID:             domainsecurity.FundsProducerContentIDPrefixV1 + digest("producer"),
		FundsProducerContentManifestSHA256: digest("producer-manifest"), FundsProducerContentManifestByteLength: 2048,
		DuckDBSHA256: digest("duckdb"), DuckDBByteLength: 8192,
		DuckDBContentSnapshotDigest: digest("duckdb-content"), DuckDBSnapshotManifestSHA256: digest("duckdb-manifest"),
		MaterializationIdentity: "txn_daily_snapshot:v12:" + digest("materialization"),
		SchemaDigest:            domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(), ExpectedCurrency: "CNY", MinorUnitScale: 2,
		QueryProfileDigest: domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func deterministicCleaningWireForTestV1(arguments DeterministicCleaningArgumentsV1) deterministicCleaningResultWireV1 {
	wire := deterministicCleaningResultWireV1{
		SchemaVersion: 1, Operation: OperationFundsDeterministicCleaningV1,
		InputDatasetSnapshotID: arguments.DatasetSnapshotID,
		RuleGeneration:         arguments.RuleGeneration, RuleDigest: arguments.RuleDigest,
		OutputArtifactSHA256: strings.Repeat("d", 64), OutputArtifactByteLength: 512,
		RowCount: 2, ChangedRowCount: 1, UnchangedRowCount: 1,
		ChangedRows: []deterministicCleaningChangedRowWireV1{{
			RowIndex: 0, Status: "changed",
			Cells: []deterministicCleaningChangedCellWireV1{{
				Field: DirectSourcePreviewFieldCounterpartyAccountV1, BeforeValue: "CP-0001", AfterValue: "CP0001",
				BeforeState: "value", AfterState: "value",
			}},
		}},
	}
	wire.ResultDigest = deterministicCleaningResultDigestV1(deterministicCleaningDigestMaterialForTestV1(wire))
	return wire
}

func deterministicCleaningDigestMaterialForTestV1(
	wire deterministicCleaningResultWireV1,
) deterministicCleaningResultDigestMaterialV1 {
	return deterministicCleaningResultDigestMaterialV1{
		SchemaVersion: wire.SchemaVersion, Operation: wire.Operation,
		InputDatasetSnapshotID: wire.InputDatasetSnapshotID,
		RuleGeneration:         wire.RuleGeneration, RuleDigest: wire.RuleDigest,
		OutputArtifactSHA256:     wire.OutputArtifactSHA256,
		OutputArtifactByteLength: wire.OutputArtifactByteLength,
		RowCount:                 wire.RowCount, ChangedRowCount: wire.ChangedRowCount,
		UnchangedRowCount: wire.UnchangedRowCount, ChangedRows: wire.ChangedRows,
	}
}
