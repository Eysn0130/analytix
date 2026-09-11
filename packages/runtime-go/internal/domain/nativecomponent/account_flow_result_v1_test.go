package nativecomponent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeAccountFlowsResultParsesExactRustContractAndRecomputesHashes(t *testing.T) {
	securityContext := nativeComponentTestContext(t, "case-flow", time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC))
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	if err := ValidateAnalyzeAccountFlowsResultV1(result, arguments); err != nil {
		t.Fatalf("validate exact account-flow result: %v", err)
	}
	body, err := marshalAccountFlowRustJSONV1(accountFlowResultToWireV1(result))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAccountFlowResultJSONShapeV1(body); err != nil {
		t.Fatalf("exact result shape rejected: %v\n%s", err, body)
	}
	var decoded analyzeAccountFlowsResultWireV1
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("exact result decode rejected: %v", err)
	}
	if err := ValidateAnalyzeAccountFlowsResultV1(accountFlowResultFromWireV1(decoded), arguments); err != nil {
		t.Fatalf("decoded result semantics rejected: %v\n%s", err, body)
	}
	parsed, err := ParseAnalyzeAccountFlowsResultV1(body, arguments)
	if err != nil || !reflect.DeepEqual(accountFlowResultToWireV1(parsed), accountFlowResultToWireV1(result)) {
		t.Fatalf("strict result round trip failed: err=%v\nparsed=%#v\nwant=%#v", err, parsed, result)
	}
	if result.Provenance.QuerySQLHash != accountFlowQuerySQLHashV1 ||
		result.QueryHash != accountFlowQueryHashV1(arguments, result.Provenance.DuckDBContentSnapshotDigest) ||
		result.ResultHash != accountFlowResultHashV1(result) {
		t.Fatal("fixed SQL/query/result hash binding drifted")
	}
	const wantQueryHash = "d45fe021cbb257b88cdc61e39642f9f634fd9725e582f80b5e6f2938356e4260"
	const wantResultHash = "aa26051b66cb80cf3743141dbd8cea10b813ab246814e3459eda91e4ab24b427"
	if result.QueryHash != wantQueryHash || result.ResultHash != wantResultHash {
		t.Fatalf("cross-language framed hash fixture drifted: query=%s result=%s", result.QueryHash, result.ResultHash)
	}

	for name, hostile := range map[string][]byte{
		"unknown top-level field": bytesBeforeFinalBraceV1(body, `,"sql":"SELECT * FROM secret"`),
		"duplicate amount":        []byte(strings.Replace(string(body), `"inflowMinor":"1000"`, `"inflowMinor":"1000","inflowMinor":"9999"`, 1)),
		"null completeness":       []byte(strings.Replace(string(body), `"aggregateComplete":true`, `"aggregateComplete":null`, 1)),
		"path field":              bytesBeforeFinalBraceV1(body, `,"dbPath":"/private/case.duckdb"`),
		"trailing object":         append(append([]byte(nil), body...), []byte(`{}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if parsed, err := ParseAnalyzeAccountFlowsResultV1(hostile, arguments); err == nil || !reflect.DeepEqual(parsed, AnalyzeAccountFlowsResultV1{}) {
				t.Fatalf("hostile result survived: parsed=%#v err=%v", parsed, err)
			}
		})
	}
}

func TestAccountFlowTypedSourceFieldReferenceIsCaseUnlinkableForIdenticalPublicSemantics(t *testing.T) {
	now := time.Date(2026, 7, 27, 3, 30, 0, 0, time.UTC)
	contextA := nativeComponentTestContext(t, "case-flow-a", now)
	contextB := nativeComponentTestContext(t, "case-flow-b", now)
	argumentsA := nativeComponentTestAccountFlowArguments(t, contextA)
	argumentsB := nativeComponentTestAccountFlowArguments(t, contextB)
	resultA := nativeComponentTestAccountFlowResult(argumentsA)
	resultB := nativeComponentTestAccountFlowResult(argumentsB)
	if argumentsA.subjectAlias != argumentsB.subjectAlias ||
		argumentsA.startInclusive != argumentsB.startInclusive || argumentsA.endInclusive != argumentsB.endInclusive ||
		resultA.InflowMinor != resultB.InflowMinor || resultA.OutflowMinor != resultB.OutflowMinor ||
		resultA.NetMinor != resultB.NetMinor || resultA.Currency != resultB.Currency ||
		resultA.MinorUnitScale != resultB.MinorUnitScale || resultA.TransactionCount != resultB.TransactionCount {
		t.Fatal("cross-case fixture did not preserve identical provider-visible semantics")
	}
	if resultA.QueryHash == resultB.QueryHash || resultA.ResultHash == resultB.ResultHash {
		t.Fatal("native query/result hashes lost case-scoped context authority")
	}
	outcomeA, err := NewAccountFlowProviderOutcomeV1(
		argumentsA.subjectAlias, resultA.AggregateComplete, resultA.EvidenceRowsComplete, resultA.QueryHash, resultA.ResultHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcomeB, err := NewAccountFlowProviderOutcomeV1(
		argumentsB.subjectAlias, resultB.AggregateComplete, resultB.EvidenceRowsComplete, resultB.QueryHash, resultB.ResultHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcomeA.SourceFieldReference.Field != outcomeB.SourceFieldReference.Field ||
		outcomeA.SourceFieldReference.BindingRef == outcomeB.SourceFieldReference.BindingRef {
		t.Fatalf("case A/B produced a linkable typed source-field reference: A=%#v B=%#v", outcomeA, outcomeB)
	}
}

func TestAnalyzeAccountFlowsResultRejectsSemanticTamperingEvenWhenResealed(t *testing.T) {
	securityContext := nativeComponentTestContext(t, "case-flow", time.Date(2026, 7, 27, 4, 0, 0, 0, time.UTC))
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	valid := nativeComponentTestAccountFlowResult(arguments)

	for name, mutate := range map[string]func(*AnalyzeAccountFlowsResultV1){
		"incorrect net": func(result *AnalyzeAccountFlowsResultV1) {
			result.NetMinor = "701"
		},
		"wrong snapshot": func(result *AnalyzeAccountFlowsResultV1) {
			result.Provenance.DatasetSnapshotID = "dsv2_" + strings.Repeat("9", 64)
		},
		"same producer wrong DuckDB content snapshot": func(result *AnalyzeAccountFlowsResultV1) {
			result.Provenance.DuckDBContentSnapshotDigest = strings.Repeat("7", 64)
		},
		"same producer wrong DuckDB snapshot manifest": func(result *AnalyzeAccountFlowsResultV1) {
			result.Provenance.DuckDBSnapshotManifestSHA256 = strings.Repeat("8", 64)
		},
		"same producer wrong materialization": func(result *AnalyzeAccountFlowsResultV1) {
			result.Provenance.SourceSignature = strings.Repeat("9", 64)
			result.Provenance.MaterializationIdentity = accountFlowMaterializationIdentityPrefixV1 + result.Provenance.SourceSignature
		},
		"wrong fixed SQL": func(result *AnalyzeAccountFlowsResultV1) {
			result.Provenance.QuerySQLHash = strings.Repeat("9", 64)
		},
		"coverage arithmetic": func(result *AnalyzeAccountFlowsResultV1) {
			result.Coverage.NormalizedSnapshotRows++
		},
		"duplicate source locator": func(result *AnalyzeAccountFlowsResultV1) {
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				rows[1].SourceRowNumber = rows[0].SourceRowNumber
				return rows
			})
		},
		"reordered time": func(result *AnalyzeAccountFlowsResultV1) {
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				rows[0], rows[1] = rows[1], rows[0]
				return rows
			})
		},
		"complete evidence missing row": func(result *AnalyzeAccountFlowsResultV1) {
			mutateAccountFlowEvidenceRowsV1(result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
				return rows[:1]
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := cloneAccountFlowResultV1(valid)
			mutate(&tampered)
			tampered.QueryHash = accountFlowQueryHashV1(arguments, tampered.Provenance.DuckDBContentSnapshotDigest)
			tampered.ResultHash = accountFlowResultHashV1(tampered)
			if err := ValidateAnalyzeAccountFlowsResultV1(tampered, arguments); err == nil {
				t.Fatal("resealed semantic tampering was accepted")
			}
		})
	}
}

func TestAnalyzeAccountFlowsResultKeepsAggregateAndEvidenceCompletenessSeparate(t *testing.T) {
	securityContext := nativeComponentTestContext(t, "case-flow", time.Date(2026, 7, 27, 5, 0, 0, 0, time.UTC))
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	arguments.evidenceRowLimit = 1
	result := nativeComponentTestAccountFlowResult(arguments)
	result.TransactionCount = 2
	result.InflowMinor = "1000"
	result.OutflowMinor = "300"
	result.NetMinor = "700"
	mutateAccountFlowEvidenceRowsV1(&result, func(rows []accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1 {
		return rows[:1]
	})
	result.AggregateComplete = true
	result.EvidenceRowsComplete = false
	result.Coverage.State = AccountFlowCoveragePartialV1
	result.Coverage.Gaps = []string{AccountFlowGapEvidenceRowLimitV1}
	result.Coverage.ObservedMatchingRows = 2
	result.QueryHash = accountFlowQueryHashV1(arguments, result.Provenance.DuckDBContentSnapshotDigest)
	result.ResultHash = accountFlowResultHashV1(result)
	if err := ValidateAnalyzeAccountFlowsResultV1(result, arguments); err != nil {
		t.Fatalf("bounded evidence with complete aggregate was rejected: %v", err)
	}
}

func TestAnalyzeAccountFlowsHostPrivateValuesCannotUseOrdinarySerializationOrFormatting(t *testing.T) {
	securityContext := nativeComponentTestContext(t, "case-flow", time.Date(2026, 7, 27, 6, 0, 0, 0, time.UTC))
	arguments := nativeComponentTestAccountFlowArguments(t, securityContext)
	result := nativeComponentTestAccountFlowResult(arguments)
	if _, err := json.Marshal(result); err == nil {
		t.Fatal("host-private result became generically JSON serializable")
	}
	directFormatted := fmt.Sprintf("%v %+v %#v", result, result, result)
	if strings.Contains(directFormatted, result.SubjectRef) || !strings.Contains(directFormatted, "subjectRef:[REDACTED]") {
		t.Fatalf("host-private subject reference escaped direct formatting: %s", directFormatted)
	}
	wire := accountFlowResultToWireV1(result)
	privateValues := []string{
		wire.EvidenceRows[0].SourceFileID,
		*wire.EvidenceRows[0].CounterpartyKey,
		*wire.EvidenceRows[0].CounterpartyName,
		*wire.EvidenceRows[0].CounterpartyBank,
	}
	type resultWithoutMethods AnalyzeAccountFlowsResultV1
	aliasJSON, err := json.Marshal(resultWithoutMethods(result))
	if err != nil {
		t.Fatalf("defined result type JSON: %v", err)
	}
	formatted := ""
	for _, verb := range []string{"%v", "%+v", "%#v"} {
		formatted += fmt.Sprintf(verb, resultWithoutMethods(result))
	}
	if err := result.evidenceRows.useExact(func(_ int, row AccountFlowEvidenceRowV1) error {
		if _, err := json.Marshal(row); err == nil {
			t.Error("host-private evidence row became generically JSON serializable")
		}
		type rowWithoutMethods AccountFlowEvidenceRowV1
		directRowFormatted := fmt.Sprintf("%v %+v %#v", row, row, row)
		if !strings.Contains(directRowFormatted, "values:[REDACTED]") {
			return fmt.Errorf("host-private row formatting did not use the fixed redaction: %s", directRowFormatted)
		}
		for _, exact := range []string{
			row.SubjectRef, row.OccurredAt, row.Direction, row.AmountMinor, row.Currency,
			fmt.Sprint(row.SourceRowNumber), fmt.Sprint(row.MinorUnitScale),
		} {
			if exact != "" && strings.Contains(directRowFormatted, exact) {
				return fmt.Errorf("host-private row value escaped direct formatting: %q in %s", exact, directRowFormatted)
			}
		}
		for _, exact := range privateValues {
			if strings.Contains(directRowFormatted, exact) {
				return fmt.Errorf("host-private row value escaped direct formatting: %q in %s", exact, directRowFormatted)
			}
		}
		rowJSON, err := json.Marshal(rowWithoutMethods(row))
		if err != nil {
			return err
		}
		aliasJSON = append(aliasJSON, rowJSON...)
		for _, verb := range []string{"%v", "%+v", "%#v"} {
			formatted += fmt.Sprintf(verb, rowWithoutMethods(row))
		}
		return nil
	}); err != nil {
		t.Fatalf("consume opaque evidence rows: %v", err)
	}
	for _, private := range privateValues {
		if strings.Contains(formatted, private) {
			t.Fatalf("host-private result formatting leaked %q: %s", private, formatted)
		}
		if strings.Contains(string(aliasJSON), private) {
			t.Fatalf("host-private result JSON leaked %q: %s", private, aliasJSON)
		}
	}
}

func nativeComponentTestAccountFlowResult(arguments AnalyzeAccountFlowsArgumentsV1) AnalyzeAccountFlowsResultV1 {
	counterpartyKey := "6217009876543210987"
	counterpartyName := "Private A&B Counterparty"
	counterpartyBank := "Analytix Test Bank"
	sourceSignature := strings.TrimPrefix(arguments.expectedMaterializationIdentity, accountFlowMaterializationIdentityPrefixV1)
	result := AnalyzeAccountFlowsResultV1{
		SubjectRef: arguments.subjectRef, StartInclusive: arguments.startInclusive, EndInclusive: arguments.endInclusive,
		Timezone: accountFlowTimezoneV1(arguments.datasetUTCOffsetMinutes), Currency: arguments.expectedCurrency,
		MinorUnitScale: arguments.minorUnitScale, InflowMinor: "1000", OutflowMinor: "300", NetMinor: "700",
		TransactionCount: 2, AggregateComplete: true, EvidenceRowsComplete: true,
		Coverage: AccountFlowCoverageV1{
			State: AccountFlowCoverageCompleteV1, Gaps: []string{},
			NormalizedSnapshotRows: 2, AcceptedSnapshotRows: 2, ObservedMatchingRows: 2,
		},
		Provenance: AccountFlowProvenanceV1{
			DatasetSnapshotID: arguments.datasetSnapshotID, ContextEpoch: arguments.contextEpoch,
			ContextDigest: arguments.contextDigest, CaseBindingHash: arguments.caseBindingHash,
			ExpectedProducerContentID:      arguments.expectedProducerContentID,
			ExpectedProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
			SubjectResolutionDigest:        arguments.subjectResolutionDigest,
			DuckDBContentSnapshotDigest:    arguments.expectedDuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:   arguments.expectedDuckDBSnapshotManifestSHA256,
			MaterializationIdentity:        arguments.expectedMaterializationIdentity,
			SourceSignature:                sourceSignature, ResultSignature: strings.Repeat("d", 64),
			ProducerContentID:      arguments.expectedProducerContentID,
			ProducerManifestSHA256: arguments.expectedProducerManifestSHA256,
			QueryContract:          AccountFlowQueryContractV1, QuerySQLHash: accountFlowQuerySQLHashV1,
		},
		Currentness:             AccountFlowCurrentnessHostRevalidationRequiredV1,
		SemanticProjectionState: AccountFlowSemanticHostResolutionRequiredV1,
		evidenceRows: newAccountFlowEvidenceRowsPrivateV1([]AccountFlowEvidenceRowV1{
			{
				SubjectRef: arguments.subjectRef,
				private: newAccountFlowEvidencePrivateV1(
					"private-file-1",
					&counterpartyKey,
					&counterpartyName,
					&counterpartyBank,
				),
				SourceRowNumber: 1001,
				OccurredAt:      "2026-01-01T00:00:00.000000Z", Direction: AccountFlowDirectionInflowV1,
				AmountMinor: "1000", Currency: arguments.expectedCurrency, MinorUnitScale: arguments.minorUnitScale,
			},
			{
				SubjectRef: arguments.subjectRef,
				private: newAccountFlowEvidencePrivateV1(
					"private-file-1",
					&counterpartyKey,
					&counterpartyName,
					&counterpartyBank,
				),
				SourceRowNumber: 1002,
				OccurredAt:      "2026-01-01T00:01:00.000000Z", Direction: AccountFlowDirectionOutflowV1,
				AmountMinor: "300", Currency: arguments.expectedCurrency, MinorUnitScale: arguments.minorUnitScale,
			},
		}),
	}
	result.QueryHash = accountFlowQueryHashV1(arguments, result.Provenance.DuckDBContentSnapshotDigest)
	result.ResultHash = accountFlowResultHashV1(result)
	return result
}

func cloneAccountFlowResultV1(result AnalyzeAccountFlowsResultV1) AnalyzeAccountFlowsResultV1 {
	return accountFlowResultFromWireV1(accountFlowResultToWireV1(result))
}

func mutateAccountFlowEvidenceRowsV1(
	result *AnalyzeAccountFlowsResultV1,
	mutate func([]accountFlowEvidenceRowWireV1) []accountFlowEvidenceRowWireV1,
) {
	wire := accountFlowResultToWireV1(*result)
	wire.EvidenceRows = mutate(wire.EvidenceRows)
	*result = accountFlowResultFromWireV1(wire)
}

func bytesBeforeFinalBraceV1(body []byte, insertion string) []byte {
	trimmed := strings.TrimSpace(string(body))
	if !strings.HasSuffix(trimmed, "}") {
		return nil
	}
	return []byte(strings.TrimSuffix(trimmed, "}") + insertion + "}")
}
