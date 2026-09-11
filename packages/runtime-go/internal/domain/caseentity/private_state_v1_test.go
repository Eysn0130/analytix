package caseentity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestCaseEntityBindingRecordSeparatesStableIdentityFromSnapshot(t *testing.T) {
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	firstContext := privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 7)
	secondContext := privateStateTestContextV1(t, "thread-b", "turn-b", "case-a", "binding-a", "snapshot-b", 8)

	const canonicalValue = "6222021234567890"
	first, err := NewCaseEntityBindingRecordV1(WithCaseEntityBindingStableOrdinalV1(NewCaseEntityBindingRecordInputV1(
		firstContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		"6222-0212 3456-7890",
	), 1))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCaseEntityBindingRecordV1(WithCaseEntityBindingStableOrdinalV1(NewCaseEntityBindingRecordInputV1(
		secondContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		canonicalValue,
	), 1))
	if err != nil {
		t.Fatal(err)
	}
	firstBody, firstBodyErr := CaseEntityBindingRecordV1Bytes(first)
	secondBody, secondBodyErr := CaseEntityBindingRecordV1Bytes(second)
	if firstBodyErr != nil || secondBodyErr != nil || string(firstBody) != string(secondBody) || first.StableOrdinal != 1 {
		t.Fatal("snapshot, epoch, turn, or thread changed the case-scoped identity binding")
	}
	body, err := CaseEntityBindingRecordV1Bytes(first)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseEntityBindingRecordV1(body)
	parsedBody, parsedBodyErr := CaseEntityBindingRecordV1Bytes(parsed)
	if err != nil || parsedBodyErr != nil || string(parsedBody) != string(body) {
		t.Fatalf("binding canonical round trip failed: err=%v", err)
	}
	if !strings.Contains(string(body), canonicalValue) {
		t.Fatal("explicit private binding bytes omitted the private canonical value")
	}
	ordinary, ordinaryErr := json.Marshal(first)
	if ordinaryErr == nil || strings.Contains(string(ordinary), canonicalValue) ||
		strings.Contains(ordinaryErr.Error(), canonicalValue) {
		t.Fatalf("ordinary JSON exposed a private binding: body=%q err=%v", ordinary, ordinaryErr)
	}
	var ordinaryParsed CaseEntityBindingRecord
	if err := json.Unmarshal(body, &ordinaryParsed); err == nil || ordinaryParsed.RecordDigest != "" ||
		ordinaryParsed.canonicalValueUse != nil ||
		strings.Contains(err.Error(), canonicalValue) {
		t.Fatalf("ordinary JSON deserialized private binding bytes: record=%v err=%v", ordinaryParsed, err)
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		formatted := fmt.Sprintf(format, first)
		if strings.Contains(formatted, canonicalValue) || !strings.Contains(formatted, "[REDACTED]") {
			t.Fatalf("ordinary format %q exposed a private binding: %q", format, formatted)
		}
	}
	type bindingRecordDefinedV1 CaseEntityBindingRecord
	type bindingInputDefinedV1 CaseEntityBindingRecordInputV1
	privateInput := NewCaseEntityBindingRecordInputV1(
		firstContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		canonicalValue,
	)
	for name, value := range map[string]any{
		"record": bindingRecordDefinedV1(first),
		"input":  bindingInputDefinedV1(privateInput),
	} {
		definedBody, definedErr := json.Marshal(value)
		definedFormat := fmt.Sprintf("%#v", value)
		if definedErr != nil || strings.Contains(string(definedBody), canonicalValue) ||
			strings.Contains(definedFormat, canonicalValue) {
			t.Fatalf("defined-type %s exposed private binding text: body=%q format=%q err=%v", name, definedBody, definedFormat, definedErr)
		}
	}
	for _, excluded := range []string{
		firstContext.ThreadID,
		firstContext.TurnID,
		firstContext.DatasetSnapshotID,
		firstContext.ContextDigest,
	} {
		if strings.Contains(string(body), excluded) {
			t.Fatalf("binding mixed identity with evidence version field %q", excluded)
		}
	}
}

func TestCaseEntityBindingRecordRequiresStoreAllocatedStableOrdinal(t *testing.T) {
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	input := NewCaseEntityBindingRecordInputV1(
		privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 1),
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		"6222021234567890",
	)
	if record, err := NewCaseEntityBindingRecordV1(input); err == nil || record.RecordDigest != "" {
		t.Fatalf("binding accepted a caller without a store ordinal: record=%#v err=%v", record, err)
	}
}

func TestLegacyCaseEntityBindingGoldenRecoversOrdinalWithoutChangingImmutableV1Bytes(t *testing.T) {
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 1)
	bindingKey := caseEntityBindingKeyV1(
		securityContext.TenantID,
		securityContext.UserID,
		securityContext.CaseID,
		securityContext.CaseBindingHash,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
	)
	legacy := legacyCaseEntityBindingRecordWireV1{
		SchemaVersion:   PrivateStateSchemaVersionV1,
		Purpose:         CaseEntityBindingPurposeV1,
		TenantID:        securityContext.TenantID,
		UserID:          securityContext.UserID,
		CaseID:          securityContext.CaseID,
		CaseBindingHash: securityContext.CaseBindingHash,
		EntityType:      domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		Reference:       reference,
		Canonicalizer:   domaincontrolledaccount.ControlledAccountFinancialCanonicalizationV1,
		CanonicalValue:  "6222021234567890",
		BindingKey:      bindingKey,
	}
	unsigned, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacy.RecordDigest = domainsecurity.SHA256Hex(unsigned)
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	const goldenSHA256 = "7050e69709771a5ec69d2e7915ea6062557df160fafa17029e3f3d875c65c490"
	if digest := domainsecurity.SHA256Hex(body); digest != goldenSHA256 {
		t.Fatalf("legacy V1 golden digest = %s", digest)
	}

	parsed, err := ParseCaseEntityBindingRecordV1(body)
	if err != nil || !CaseEntityBindingNeedsStableOrdinalRecoveryV1(parsed) || parsed.StableOrdinal != 0 {
		t.Fatalf("legacy V1 record was not admitted for bounded ordinal recovery: record=%#v err=%v", parsed, err)
	}
	recovered, err := WithRecoveredCaseEntityBindingStableOrdinalV1(parsed, 7)
	if err != nil || recovered.StableOrdinal != 7 || recovered.RecordDigest != legacy.RecordDigest {
		t.Fatalf("legacy V1 ordinal recovery changed immutable identity: record=%#v err=%v", recovered, err)
	}
	recoveredBody, err := CaseEntityBindingRecordV1Bytes(recovered)
	if err != nil || string(recoveredBody) != string(body) {
		t.Fatalf("legacy V1 ordinal recovery changed canonical bytes: err=%v", err)
	}
	if _, err := WithRecoveredCaseEntityBindingStableOrdinalV1(recovered, 8); err == nil {
		t.Fatal("legacy V1 ordinal recovery accepted a second assignment")
	}

	var forged map[string]any
	if err := json.Unmarshal(body, &forged); err != nil {
		t.Fatal(err)
	}
	forged["stableOrdinal"] = float64(7)
	forgedBody, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCaseEntityBindingRecordV1(forgedBody); err == nil {
		t.Fatal("legacy V1 record accepted a forged ordinal without a bound digest")
	}
}

func TestCaseIngressRecordBindsTypedPrivateSpanWithoutEchoingFailures(t *testing.T) {
	securityContext := privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 1)
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	rawValue := "6222-0212 3456-7890"
	raw := "请分析账号 " + rawValue + " 的流入与流出"
	projected := "请分析账号 " + string(reference) + " 的流入与流出"
	rawStart := strings.Index(raw, rawValue)
	projectedStart := strings.Index(projected, string(reference))
	record, err := NewCaseIngressRecordV1(NewCaseIngressRecordInputV1(
		securityContext,
		CaseIngressKindTurnV1,
		0,
		raw,
		projected,
		[]CaseIngressSpanV1{{
			EntityType:         domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			Reference:          reference,
			RawStartByte:       rawStart,
			RawEndByte:         rawStart + len(rawValue),
			ProjectedStartByte: projectedStart,
			ProjectedEndByte:   projectedStart + len(reference),
		}},
	))
	if err != nil {
		t.Fatal(err)
	}
	lookupID, err := CaseIngressLookupIDV1(
		securityContext,
		CaseIngressKindTurnV1,
		0,
	)
	if err != nil || lookupID != record.IngressID {
		t.Fatalf("public ingress lookup drifted from private record identity: lookup=%q record=%q err=%v", lookupID, record.IngressID, err)
	}
	steerLookupID, err := CaseIngressLookupIDV1(
		securityContext,
		CaseIngressKindSteerV1,
		0,
	)
	if err != nil || steerLookupID == record.IngressID {
		t.Fatalf("ingress kind did not separate deterministic lookup identity: lookup=%q err=%v", steerLookupID, err)
	}
	ordinalLookupID, err := CaseIngressLookupIDV1(
		securityContext,
		CaseIngressKindTurnV1,
		1,
	)
	if err != nil || ordinalLookupID == record.IngressID {
		t.Fatalf("ingress ordinal did not separate deterministic lookup identity: lookup=%q err=%v", ordinalLookupID, err)
	}
	if _, err := CaseIngressLookupIDV1(securityContext, "unsupported", 0); err == nil {
		t.Fatal("unsupported ingress kind received a deterministic lookup identity")
	}
	body, err := CaseIngressRecordV1Bytes(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseIngressRecordV1(body)
	var parsedRaw string
	parsedRawErr := parsed.UseRawTextV1(func(value string) error {
		parsedRaw = value
		return nil
	})
	if err != nil || parsedRawErr != nil || parsed.RecordDigest != record.RecordDigest ||
		parsedRaw != raw || parsed.ProjectedText != projected {
		t.Fatalf("private ingress canonical round trip failed: err=%v", err)
	}
	ordinary, ordinaryErr := json.Marshal(record)
	if ordinaryErr == nil || strings.Contains(string(ordinary), rawValue) ||
		strings.Contains(ordinaryErr.Error(), rawValue) {
		t.Fatalf("ordinary JSON exposed private ingress: body=%q err=%v", ordinary, ordinaryErr)
	}
	var ordinaryParsed CaseIngressRecord
	if err := json.Unmarshal(body, &ordinaryParsed); err == nil ||
		ordinaryParsed.rawTextUse != nil ||
		strings.Contains(err.Error(), rawValue) {
		t.Fatalf("ordinary JSON deserialized private ingress bytes: record=%v err=%v", ordinaryParsed, err)
	}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		formatted := fmt.Sprintf(format, record)
		if strings.Contains(formatted, rawValue) || !strings.Contains(formatted, "[REDACTED]") {
			t.Fatalf("ordinary format %q exposed private ingress: %q", format, formatted)
		}
	}
	type ingressRecordDefinedV1 CaseIngressRecord
	type ingressInputDefinedV1 CaseIngressRecordInputV1
	privateInput := NewCaseIngressRecordInputV1(
		securityContext,
		CaseIngressKindTurnV1,
		0,
		raw,
		projected,
		record.Spans,
	)
	for name, value := range map[string]any{
		"record": ingressRecordDefinedV1(record),
		"input":  ingressInputDefinedV1(privateInput),
	} {
		definedBody, definedErr := json.Marshal(value)
		definedFormat := fmt.Sprintf("%#v", value)
		if definedErr != nil || strings.Contains(string(definedBody), rawValue) ||
			strings.Contains(definedFormat, rawValue) {
			t.Fatalf("defined-type %s exposed private ingress text: body=%q format=%q err=%v", name, definedBody, definedFormat, definedErr)
		}
	}

	tampered := record
	tampered.ProjectedText = strings.Replace(projected, string(reference), rawValue, 1)
	tampered.RecordDigest = caseIngressRecordDigestV1(tampered)
	if err := ValidateCaseIngressRecordV1(tampered); err == nil ||
		strings.Contains(err.Error(), rawValue) {
		t.Fatalf("source-exact projection failure was not safely rejected: %v", err)
	}
	forgedReference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewCaseIngressRecordV1(NewCaseIngressRecordInputV1(
		securityContext,
		CaseIngressKindTurnV1,
		0,
		raw+" "+string(forgedReference),
		projected+" "+string(forgedReference),
		record.Spans,
	))
	if err == nil || strings.Contains(err.Error(), string(forgedReference)) {
		t.Fatalf("unbound private-gap reference was not rejected safely: %v", err)
	}
	for _, candidate := range []string{
		"cer1_" + strings.Repeat("aaaa\u200b", 16),
		"c.e.r.1_" + strings.Repeat("dead-beef-", 8),
		"ＣＥＲ１＿" + strings.Repeat("Ａ", 64),
		"c\u034fe\u200br1_" + strings.Repeat("aaaa-aaaa-", 8),
	} {
		_, candidateErr := NewCaseIngressRecordV1(NewCaseIngressRecordInputV1(
			securityContext,
			CaseIngressKindTurnV1,
			0,
			raw+" "+candidate,
			projected+" "+candidate,
			record.Spans,
		))
		if candidateErr == nil || strings.Contains(candidateErr.Error(), candidate) {
			t.Fatalf("obfuscated private-gap reference was not rejected safely: candidate=%q err=%v", candidate, candidateErr)
		}
	}

	oversized := strings.Repeat("x", maxCaseIngressTextBytesV1+1)
	_, err = NewCaseIngressRecordV1(NewCaseIngressRecordInputV1(
		securityContext,
		CaseIngressKindTurnV1,
		0,
		oversized,
		projected,
		[]CaseIngressSpanV1{{
			EntityType:         domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			Reference:          reference,
			RawStartByte:       0,
			RawEndByte:         16,
			ProjectedStartByte: projectedStart,
			ProjectedEndByte:   projectedStart + len(reference),
		}},
	))
	if err == nil || strings.Contains(err.Error(), oversized) {
		t.Fatalf("oversized ingress did not fail with a bounded non-reflective error: %v", err)
	}

	for name, unsafeTail := range map[string]string{
		"unspanned eight digits":       "12345678",
		"unspanned ten digits":         "1234567890",
		"unicode separated account":    "6217–0098–7654–3210",
		"full width separated account": "６２１７：００９８：７６５４：３２１０",
	} {
		t.Run(name, func(t *testing.T) {
			unsafeRaw := raw + "，对手 " + unsafeTail
			unsafeProjected := projected + "，对手 " + unsafeTail
			_, err := NewCaseIngressRecordV1(NewCaseIngressRecordInputV1(
				securityContext,
				CaseIngressKindTurnV1,
				0,
				unsafeRaw,
				unsafeProjected,
				[]CaseIngressSpanV1{{
					EntityType:         domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
					Reference:          reference,
					RawStartByte:       rawStart,
					RawEndByte:         rawStart + len(rawValue),
					ProjectedStartByte: projectedStart,
					ProjectedEndByte:   projectedStart + len(reference),
				}},
			))
			if err == nil || strings.Contains(err.Error(), unsafeTail) {
				t.Fatalf("unspanned complete identifier was not safely rejected: %v", err)
			}
		})
	}

	for name, unsafeProjection := range map[string]string{
		"safe text mutation":         projected + "（已修改）",
		"canonical account appended": projected + " 6222021234567890",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewCaseIngressRecordV1(NewCaseIngressRecordInputV1(
				securityContext,
				CaseIngressKindTurnV1,
				0,
				raw,
				unsafeProjection,
				[]CaseIngressSpanV1{{
					EntityType:         domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
					Reference:          reference,
					RawStartByte:       rawStart,
					RawEndByte:         rawStart + len(rawValue),
					ProjectedStartByte: projectedStart,
					ProjectedEndByte:   projectedStart + len(reference),
				}},
			))
			if err == nil {
				t.Fatal("non-exact ingress projection was accepted")
			}
		})
	}
}

func TestCompleteIdentifierShapeV1RejectsShortAndUnicodeAccountsWithoutClassifyingISODate(t *testing.T) {
	for _, value := range []string{
		"12345678",
		"1234 5678 90",
		"6217–0098–7654–3210",
		"６２１７：００９８：７６５４：３２１０",
		strings.Repeat("⁶", 8),
		"①②③④⑤⑥⑦⑧",
		"⑩⑩⑩⑩",
		"账号 2026-07-28",
		"2026-02-30",
		"金额 328000000",
		"交易笔数 12600000",
		"notcny 6217009876543210",
		"6217009876543210 cny_account",
		"非人民币 6217009876543210",
		"6217009876543210 元件",
	} {
		if !containsCompleteIdentifierShapeV1(value) {
			t.Fatalf("complete identifier shape bypassed detection: %q", value)
		}
	}
	for _, value := range []string{
		"2026-07-28",
		"范围 2026/07/28 至 2026/08/01",
	} {
		if containsCompleteIdentifierShapeV1(value) {
			t.Fatalf("canonical ISO date was misclassified as an account: %q", value)
		}
	}
	for _, value := range []string{
		"金额 328000000 CNY",
		"金额 ¥328,000,000.25",
		"金额 328000000 元",
	} {
		if !containsCompleteIdentifierShapeV1(value) {
			t.Fatalf("general private reference accepted an ingress-only scalar: %q", value)
		}
		projected, err := ProjectCaseIngressPrivateGapV1(value)
		if err != nil || projected == value || strings.Contains(projected, "328000000") {
			t.Fatalf("private ingress gap retained a complete scalar: value=%q projected=%q err=%v", value, projected, err)
		}
	}
	for _, value := range []string{
		"6222021\u034f2345678\u034f90123",
		"6222021\ufe0f2345678\ufe0f90123",
		"6222021\x002345678\x0090123",
		strings.Repeat("⁶", 8),
		"①②③④⑤⑥⑦⑧",
		"⑩⑩⑩⑩",
		"6222021" + strings.Repeat("\u034f", 300) + "2345678" +
			strings.Repeat("\ufe0f", 300) + "90",
	} {
		projected, err := ProjectCaseIngressPrivateGapV1(value)
		if err != nil || projected != "[NUMBER]" {
			t.Fatalf("obfuscated private ingress scalar was not closed: raw=%q projected=%q err=%v", value, projected, err)
		}
	}
}

func TestThreadCaseContextEvolutionMarksOldFactsHistorical(t *testing.T) {
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	firstContext := privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 3)
	secondContext := privateStateTestContextV1(t, "thread-a", "turn-b", "case-a", "binding-a", "snapshot-b", 4)

	first, err := NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
		SecurityContext:  firstContext,
		Generation:       1,
		EntityReferences: []ReferenceV1{reference},
		Snapshots: []CaseSnapshotStateV1{{
			DatasetSnapshotID: firstContext.DatasetSnapshotID,
			ContextEpoch:      firstContext.ContextEpoch,
			Currentness:       SnapshotCurrentV1,
		}},
		Evidence: []CaseEvidenceStateV1{{
			EvidenceReference: "evidence_snapshot_a",
			DatasetSnapshotID: firstContext.DatasetSnapshotID,
			Currentness:       SnapshotCurrentV1,
		}},
		Claims: []CaseClaimStateV1{{
			ClaimReference:            "claim_snapshot_a",
			DatasetSnapshotID:         firstContext.DatasetSnapshotID,
			Currentness:               SnapshotCurrentV1,
			InvestigationState:        InvestigationConfirmedV1,
			EvidenceReferences:        []string{"evidence_snapshot_a"},
			CounterEvidenceReferences: []string{},
		}},
		OpenQuestionReferences: []string{"question_counterparty"},
		DataGapReferences:      []string{"gap_prior_period"},
	})
	if err != nil {
		t.Fatal(err)
	}
	obfuscatedContextReference := "c.e.r.1_" + strings.Repeat("dead-beef-", 8)
	for name, mutate := range map[string]func(ThreadCaseContextRecord) ThreadCaseContextRecord{
		"evidence reference": func(value ThreadCaseContextRecord) ThreadCaseContextRecord {
			value.Evidence = append([]CaseEvidenceStateV1(nil), value.Evidence...)
			value.Evidence[0].EvidenceReference = obfuscatedContextReference
			return value
		},
		"claim reference": func(value ThreadCaseContextRecord) ThreadCaseContextRecord {
			value.Claims = cloneCaseClaimStatesV1(value.Claims)
			value.Claims[0].ClaimReference = obfuscatedContextReference
			return value
		},
		"claim evidence reference": func(value ThreadCaseContextRecord) ThreadCaseContextRecord {
			value.Claims = cloneCaseClaimStatesV1(value.Claims)
			value.Claims[0].EvidenceReferences[0] = obfuscatedContextReference
			return value
		},
		"open question reference": func(value ThreadCaseContextRecord) ThreadCaseContextRecord {
			value.OpenQuestionReferences = append([]string(nil), value.OpenQuestionReferences...)
			value.OpenQuestionReferences[0] = obfuscatedContextReference
			return value
		},
		"data gap reference": func(value ThreadCaseContextRecord) ThreadCaseContextRecord {
			value.DataGapReferences = append([]string(nil), value.DataGapReferences...)
			value.DataGapReferences[0] = obfuscatedContextReference
			return value
		},
	} {
		t.Run("rejects obfuscated "+name, func(t *testing.T) {
			mutated := mutate(first)
			mutated.RecordDigest = threadCaseContextRecordDigestV1(mutated)
			validationErr := ValidateThreadCaseContextRecordV1(mutated)
			if validationErr == nil || strings.Contains(validationErr.Error(), obfuscatedContextReference) {
				t.Fatalf("obfuscated context reference was retained: err=%v", validationErr)
			}
		})
	}
	second, err := NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
		SecurityContext:      secondContext,
		Generation:           2,
		PreviousRecordDigest: first.RecordDigest,
		EntityReferences:     []ReferenceV1{reference},
		Snapshots: []CaseSnapshotStateV1{
			{
				DatasetSnapshotID: firstContext.DatasetSnapshotID,
				ContextEpoch:      firstContext.ContextEpoch,
				Currentness:       SnapshotHistoricalV1,
			},
			{
				DatasetSnapshotID: secondContext.DatasetSnapshotID,
				ContextEpoch:      secondContext.ContextEpoch,
				Currentness:       SnapshotCurrentV1,
			},
		},
		Evidence: []CaseEvidenceStateV1{
			{
				EvidenceReference: "evidence_snapshot_a",
				DatasetSnapshotID: firstContext.DatasetSnapshotID,
				Currentness:       SnapshotHistoricalV1,
			},
			{
				EvidenceReference: "evidence_snapshot_b",
				DatasetSnapshotID: secondContext.DatasetSnapshotID,
				Currentness:       SnapshotCurrentV1,
			},
		},
		Claims: []CaseClaimStateV1{
			{
				ClaimReference:            "claim_snapshot_a",
				DatasetSnapshotID:         firstContext.DatasetSnapshotID,
				Currentness:               SnapshotHistoricalV1,
				InvestigationState:        InvestigationConfirmedV1,
				EvidenceReferences:        []string{"evidence_snapshot_a"},
				CounterEvidenceReferences: []string{},
			},
			{
				ClaimReference:            "claim_snapshot_b",
				DatasetSnapshotID:         secondContext.DatasetSnapshotID,
				Currentness:               SnapshotCurrentV1,
				InvestigationState:        InvestigationOpenV1,
				EvidenceReferences:        []string{"evidence_snapshot_b"},
				CounterEvidenceReferences: []string{},
			},
		},
		OpenQuestionReferences: []string{"question_counterparty"},
		DataGapReferences:      []string{"gap_prior_period"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateThreadCaseContextEvolutionV1(first, second); err != nil {
		t.Fatalf("valid snapshot evolution failed: %v", err)
	}
	if first.EntityReferences[0] != second.EntityReferences[0] {
		t.Fatal("stable entity identity changed with snapshot evidence")
	}

	stalePromoted := second
	stalePromoted.Claims = cloneCaseClaimStatesV1(second.Claims)
	stalePromoted.Claims[0].Currentness = SnapshotCurrentV1
	stalePromoted.RecordDigest = threadCaseContextRecordDigestV1(stalePromoted)
	if err := ValidateThreadCaseContextRecordV1(stalePromoted); err == nil {
		t.Fatal("historical snapshot claim was promoted to current")
	}

	droppedRef := second
	droppedRef.EntityReferences = []ReferenceV1{}
	droppedRef.RecordDigest = threadCaseContextRecordDigestV1(droppedRef)
	if err := ValidateThreadCaseContextRecordV1(droppedRef); err != nil {
		t.Fatal(err)
	}
	if err := ValidateThreadCaseContextEvolutionV1(first, droppedRef); err == nil {
		t.Fatal("context evolution dropped a stable entity reference")
	}
}

func TestLegacyThreadCaseClaimV1RoundTripsWithoutTypedStateRewrite(t *testing.T) {
	firstContext := privateStateTestContextV1(t, "thread-legacy-claim", "turn-a", "case-legacy-claim", "binding-legacy-claim", "snapshot-legacy-claim", 1)
	legacy, err := NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
		SecurityContext: firstContext, Generation: 1,
		Snapshots: []CaseSnapshotStateV1{{
			DatasetSnapshotID: firstContext.DatasetSnapshotID,
			ContextEpoch:      firstContext.ContextEpoch,
			Currentness:       SnapshotCurrentV1,
		}},
		Claims: []CaseClaimStateV1{{
			ClaimReference:     "claim_legacy_v1",
			DatasetSnapshotID:  firstContext.DatasetSnapshotID,
			Currentness:        SnapshotCurrentV1,
			InvestigationState: InvestigationOpenV1,
			EvidenceReferences: []string{}, CounterEvidenceReferences: []string{},
		}},
		EntityReferences: []ReferenceV1{}, Evidence: []CaseEvidenceStateV1{},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := ThreadCaseContextRecordV1Bytes(legacy)
	if err != nil || strings.Contains(string(body), "typedState") {
		t.Fatalf("legacy V1 claim wire was widened: body=%s err=%v", body, err)
	}
	const legacyGoldenSHA256 = "94a3c49985f02a5c35f66921a41b465dd41155eb4865d528c1450b8b4958c4a9"
	if digest := domainsecurity.SHA256Hex(body); digest != legacyGoldenSHA256 {
		t.Fatalf("legacy claim V1 golden digest = %s", digest)
	}
	parsed, err := ParseThreadCaseContextRecordV1(body)
	parsedBody, parsedBodyErr := ThreadCaseContextRecordV1Bytes(parsed)
	if err != nil || parsedBodyErr != nil || parsed.RecordDigest != legacy.RecordDigest ||
		len(parsed.Claims) != 1 || parsed.Claims[0].TypedState != nil || string(parsedBody) != string(body) {
		t.Fatalf("legacy V1 claim readback changed canonical state: parsed=%#v err=%v bytesErr=%v", parsed, err, parsedBodyErr)
	}

	secondContext := privateStateTestContextV1(t, "thread-legacy-claim", "turn-b", "case-legacy-claim", "binding-legacy-claim", "snapshot-legacy-claim", 2)
	evolved, err := NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
		SecurityContext: secondContext, Generation: 2, PreviousRecordDigest: legacy.RecordDigest,
		Snapshots: []CaseSnapshotStateV1{{
			DatasetSnapshotID: secondContext.DatasetSnapshotID,
			ContextEpoch:      secondContext.ContextEpoch,
			Currentness:       SnapshotCurrentV1,
		}},
		Claims:           cloneCaseClaimStatesV1(legacy.Claims),
		EntityReferences: []ReferenceV1{}, Evidence: []CaseEvidenceStateV1{},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	})
	if err != nil || ValidateThreadCaseContextEvolutionV1(legacy, evolved) != nil ||
		evolved.Claims[0].TypedState != nil || evolved.DisplayBindings != nil {
		t.Fatalf("legacy V1 evolution rewrote typed claim state: evolved=%#v err=%v", evolved, err)
	}
	typedState, err := NewCaseClaimTypedStateV1("relationship")
	if err != nil {
		t.Fatal(err)
	}
	backfilledClaims := cloneCaseClaimStatesV1(evolved.Claims)
	backfilledClaims[0].TypedState = typedState
	backfilled, backfillErr := NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
		SecurityContext: secondContext, Generation: 2, PreviousRecordDigest: legacy.RecordDigest,
		Snapshots: evolved.Snapshots, Claims: backfilledClaims,
		EntityReferences: []ReferenceV1{}, Evidence: []CaseEvidenceStateV1{},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	})
	recordErr := ValidateThreadCaseContextRecordV1(backfilled)
	evolutionErr := ValidateThreadCaseContextEvolutionV1(legacy, backfilled)
	if backfillErr != nil || recordErr != nil || evolutionErr == nil {
		t.Fatalf("legacy claim identity accepted an implicit typed-state backfill: constructErr=%v recordErr=%v evolutionErr=%v", backfillErr, recordErr, evolutionErr)
	}
	for name, invalid := range map[string]*CaseClaimTypedStateV1{
		"unknown version": {SchemaVersion: 2, ClaimType: "relationship"},
		"unknown type":    {SchemaVersion: 1, ClaimType: "model_summary"},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := legacy
			tampered.Claims = cloneCaseClaimStatesV1(legacy.Claims)
			tampered.Claims[0].TypedState = invalid
			tampered.RecordDigest = threadCaseContextRecordDigestV1(tampered)
			if ValidateThreadCaseContextRecordV1(tampered) == nil {
				t.Fatal("tampered typed claim state was accepted")
			}
		})
	}
}

func TestCaseAcceptedDisplayBindingV1IsPrivateCanonicalAndFailsClosedOnHostileEvolution(t *testing.T) {
	securityContext := privateStateTestContextV1(
		t, "thread-display-origin", "turn-display-origin", "case-display", "binding-display", "snapshot-display", 1,
	)
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	evidenceReference := "evr_" + domainsecurity.SHA256Hex([]byte("display-evidence-reference"))
	claim := CaseClaimStateV1{
		ClaimReference:    "clm_9487be9220a30235565b689d2378dbeed51718db44762897484cf44c5906a67f",
		ClaimDigest:       domainsecurity.SHA256Hex([]byte("display-claim")),
		DatasetSnapshotID: securityContext.DatasetSnapshotID, Currentness: SnapshotCurrentV1,
		InvestigationState: InvestigationConfirmedV1,
		EvidenceReferences: []string{evidenceReference}, CounterEvidenceReferences: []string{},
	}
	evidence := CaseEvidenceStateV1{
		EvidenceReference: evidenceReference, EvidenceDigest: domainsecurity.SHA256Hex([]byte("display-evidence")),
		DatasetSnapshotID: securityContext.DatasetSnapshotID, Currentness: SnapshotCurrentV1,
	}
	binding, err := NewCaseAcceptedDisplayBindingV1(CaseAcceptedDisplayBindingInputV1{
		CaseBindingHash:  securityContext.CaseBindingHash,
		OriginalThreadID: securityContext.ThreadID, OriginalTurnID: securityContext.TurnID,
		AcceptedFinalDigest: domainsecurity.SHA256Hex([]byte("display-final")),
		DispositionDigest:   domainsecurity.SHA256Hex([]byte("display-disposition")),
		FinalGateVersion:    caseAcceptedDisplayFinalGateVersionV1, ContextDigest: securityContext.ContextDigest,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ContextEpoch: securityContext.ContextEpoch,
		EntityReference: reference, EntityBindingDigest: domainsecurity.SHA256Hex([]byte("display-entity-binding")),
		SlotID: "account-slot-1",
		ClaimBindings: []CaseAcceptedDisplayClaimBindingV1{{
			ClaimReference: claim.ClaimReference, ClaimDigest: claim.ClaimDigest,
		}},
		EvidenceReceiptBindings: []CaseAcceptedDisplayEvidenceBindingV1{{
			EvidenceReference: evidence.EvidenceReference, EvidenceDigest: evidence.EvidenceDigest,
		}},
		Currentness: SnapshotCurrentV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: 1,
		EntityReferences: []ReferenceV1{reference},
		EntityIdentities: []CaseEntityIdentityStateV1{{
			Reference:     reference,
			EntityType:    domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
			StableOrdinal: 1,
		}},
		Snapshots: []CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			ContextEpoch:      securityContext.ContextEpoch, Currentness: SnapshotCurrentV1,
		}},
		Claims: []CaseClaimStateV1{claim}, Evidence: []CaseEvidenceStateV1{evidence},
		DisplayBindings:        []CaseAcceptedDisplayBindingV1{binding},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	}
	index, err := NewCaseLongitudinalIndexRecordV1(base)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ThreadCaseContextRecordV1Bytes(index)
	parsed, parseErr := ParseThreadCaseContextRecordV1(body)
	if err != nil || parseErr != nil || len(parsed.DisplayBindings) != 1 ||
		parsed.DisplayBindings[0].BindingDigest != binding.BindingDigest || parsed.RecordDigest != index.RecordDigest {
		t.Fatalf("display binding restart readback failed: parsed=%#v encodeErr=%v parseErr=%v", parsed, err, parseErr)
	}
	for _, forbidden := range []string{
		"sourceFileId", "sourceRowNumber", "sourceRecordId", "acceptedSlotSourceBindings",
		"rawProvider", "rawTool", "6222021234567890",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("private value-free display binding admitted forbidden material %q", forbidden)
		}
	}

	for name, mutate := range map[string]func(*CaseAcceptedDisplayBindingV1){
		"unknown child version": func(candidate *CaseAcceptedDisplayBindingV1) { candidate.SchemaVersion++ },
		"case rebound":          func(candidate *CaseAcceptedDisplayBindingV1) { candidate.CaseBindingHash = strings.Repeat("8", 64) },
		"claim digest tamper": func(candidate *CaseAcceptedDisplayBindingV1) {
			candidate.ClaimBindings[0].ClaimDigest = strings.Repeat("9", 64)
		},
		"receipt digest tamper": func(candidate *CaseAcceptedDisplayBindingV1) {
			candidate.EvidenceReceiptBindings[0].EvidenceDigest = strings.Repeat("a", 64)
		},
		"entity rebound": func(candidate *CaseAcceptedDisplayBindingV1) {
			candidate.EntityReference = ReferenceV1(ReferencePrefixV1 + strings.Repeat("b", 64))
		},
	} {
		t.Run(name, func(t *testing.T) {
			hostile := index
			hostile.DisplayBindings = cloneCaseAcceptedDisplayBindingsV1(index.DisplayBindings)
			mutate(&hostile.DisplayBindings[0])
			hostile.DisplayBindings[0].BindingDigest = caseAcceptedDisplayBindingDigestV1(hostile.DisplayBindings[0])
			hostile.RecordDigest = threadCaseContextRecordDigestV1(hostile)
			if ValidateThreadCaseContextRecordV1(hostile) == nil {
				t.Fatal("hostile display binding was accepted")
			}
		})
	}

	t.Run("unknown final gate version survives self digest but not CAS reopen", func(t *testing.T) {
		hostile := index
		hostile.DisplayBindings = cloneCaseAcceptedDisplayBindingsV1(index.DisplayBindings)
		hostile.DisplayBindings[0].FinalGateVersion = "analytix.final-evidence-gate/v999"
		hostile.DisplayBindings[0].BindingDigest = caseAcceptedDisplayBindingDigestV1(hostile.DisplayBindings[0])
		hostile.RecordDigest = threadCaseContextRecordDigestV1(hostile)
		body, err := json.Marshal(hostile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseThreadCaseContextRecordV1(body); err == nil {
			t.Fatal("CAS reopen accepted an unknown final-gate version with internally consistent digests")
		}
	})

	duplicate := base
	duplicate.DisplayBindings = []CaseAcceptedDisplayBindingV1{binding, binding}
	if _, err := NewCaseLongitudinalIndexRecordV1(duplicate); err == nil {
		t.Fatal("duplicate display binding was accepted")
	}
	missingInput := base
	missingInput.Generation = 2
	missingInput.PreviousRecordDigest = index.RecordDigest
	missingInput.DisplayBindings = nil
	missing, err := NewCaseLongitudinalIndexRecordV1(missingInput)
	if err != nil || ValidateThreadCaseContextEvolutionV1(index, missing) == nil {
		t.Fatal("display binding evolution admitted a missing replacement")
	}

	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	displayBindings := object["displayBindings"].([]any)
	displayBindings[0].(map[string]any)["unknown"] = true
	unknown, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseThreadCaseContextRecordV1(unknown); err == nil {
		t.Fatal("unknown display-binding child field was accepted")
	}
	delete(displayBindings[0].(map[string]any), "unknown")
	delete(displayBindings[0].(map[string]any), "claimBindings")
	partial, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseThreadCaseContextRecordV1(partial); err == nil {
		t.Fatal("partial display-binding child was accepted")
	}
}

func TestCaseLongitudinalIndexBindsStableOrdinalsWithoutChangingThreadWireShape(t *testing.T) {
	accountReference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	cardReference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("2", 64))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := privateStateTestContextV1(t, "thread-index", "turn-index", "case-index", "binding-index", "snapshot-index", 1)
	base := ThreadCaseContextRecordInputV1{
		SecurityContext: securityContext, Generation: 1,
		EntityReferences: []ReferenceV1{accountReference, cardReference},
		Snapshots: []CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			ContextEpoch:      securityContext.ContextEpoch,
			Currentness:       SnapshotCurrentV1,
		}},
		Claims: []CaseClaimStateV1{}, Evidence: []CaseEvidenceStateV1{},
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	}
	threadRecord, err := NewThreadCaseContextRecordV1(base)
	if err != nil || threadRecord.EntityIdentities != nil {
		t.Fatalf("ordinary thread record gained identity-index state: record=%#v err=%v", threadRecord, err)
	}
	base.EntityIdentities = []CaseEntityIdentityStateV1{
		{Reference: accountReference, EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, StableOrdinal: 1},
		{Reference: cardReference, EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1, StableOrdinal: 2},
	}
	index, err := NewCaseLongitudinalIndexRecordV1(base)
	if err != nil || !IsCaseLongitudinalIndexRecordV1(index) || len(index.EntityIdentities) != 2 {
		t.Fatalf("case index identity state mismatch: index=%#v err=%v", index, err)
	}

	collision := base
	collision.EntityIdentities = append([]CaseEntityIdentityStateV1(nil), base.EntityIdentities...)
	collision.EntityIdentities[1].EntityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
	collision.EntityIdentities[1].StableOrdinal = 1
	if _, err := NewCaseLongitudinalIndexRecordV1(collision); err == nil {
		t.Fatal("case index accepted one alias for two authority references")
	}
	if _, err := NewThreadCaseContextRecordV1(base); err == nil {
		t.Fatal("ordinary thread record accepted case-level identity-index state")
	}
}

func TestPrivateStateParsingRejectsUnknownAndNonCanonicalFields(t *testing.T) {
	reference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	securityContext := privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 1)
	record, err := NewCaseEntityBindingRecordV1(WithCaseEntityBindingStableOrdinalV1(NewCaseEntityBindingRecordInputV1(
		securityContext,
		domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1,
		reference,
		"6222021234567890",
	), 1))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := CaseEntityBindingRecordV1Bytes(record)
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := ParseCaseEntityBindingRecordV1(unknown); err == nil {
		t.Fatal("unknown private-state property was accepted")
	}
	nonCanonical := append([]byte(" \n"), body...)
	if _, err := ParseCaseEntityBindingRecordV1(nonCanonical); err == nil {
		t.Fatal("non-canonical private-state bytes were accepted")
	}
}

func TestThreadCaseContextBoundsFailWithoutReflectingPrivateValues(t *testing.T) {
	securityContext := privateStateTestContextV1(t, "thread-a", "turn-a", "case-a", "binding-a", "snapshot-a", 1)
	protectedReference, err := NewReferenceV1FromKeyedDigest(strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	references := make([]ReferenceV1, 0, maxThreadCaseEntitiesV1+1)
	for index := 0; index <= maxThreadCaseEntitiesV1; index++ {
		reference, err := NewReferenceV1FromKeyedDigest(fmt.Sprintf("%064x", index))
		if err != nil {
			t.Fatal(err)
		}
		references = append(references, reference)
	}
	_, err = NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
		SecurityContext:  securityContext,
		Generation:       1,
		EntityReferences: references,
		Snapshots: []CaseSnapshotStateV1{{
			DatasetSnapshotID: securityContext.DatasetSnapshotID,
			ContextEpoch:      securityContext.ContextEpoch,
			Currentness:       SnapshotCurrentV1,
		}},
		Claims:                 []CaseClaimStateV1{},
		Evidence:               []CaseEvidenceStateV1{},
		OpenQuestionReferences: []string{},
		DataGapReferences:      []string{},
	})
	if err == nil || strings.Contains(err.Error(), string(references[len(references)-1])) {
		t.Fatalf("entity bound did not fail with a non-reflective error: %v", err)
	}

	for _, sensitiveReference := range []string{
		"question_6222021234567890",
		"question_12345678",
		"question_6217–0098–7654–3210",
		"question_６２１７：００９８：７６５４：３２１０",
		"question_" + string(protectedReference),
	} {
		_, err = NewThreadCaseContextRecordV1(ThreadCaseContextRecordInputV1{
			SecurityContext: securityContext,
			Generation:      1,
			Snapshots: []CaseSnapshotStateV1{{
				DatasetSnapshotID: securityContext.DatasetSnapshotID,
				ContextEpoch:      securityContext.ContextEpoch,
				Currentness:       SnapshotCurrentV1,
			}},
			EntityReferences:       []ReferenceV1{},
			Claims:                 []CaseClaimStateV1{},
			Evidence:               []CaseEvidenceStateV1{},
			OpenQuestionReferences: []string{sensitiveReference},
			DataGapReferences:      []string{},
		})
		if err == nil || strings.Contains(err.Error(), sensitiveReference) {
			t.Fatalf("thread context retained a protected reference: %v", err)
		}
	}
}

func privateStateTestContextV1(
	t *testing.T,
	threadID string,
	turnID string,
	caseID string,
	bindingSeed string,
	snapshotSeed string,
	epoch uint64,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID:           threadID,
		TurnID:             turnID,
		WorkspaceRealPath:  "/cases/private-state",
		TenantID:           "tenant-a",
		UserID:             "user-a",
		CaseID:             caseID,
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte(bindingSeed)),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID(snapshotSeed),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:" + snapshotSeed)),
		ContextEpoch:       epoch,
		IssuedAt:           time.Unix(1_800_000_000+int64(epoch), 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
