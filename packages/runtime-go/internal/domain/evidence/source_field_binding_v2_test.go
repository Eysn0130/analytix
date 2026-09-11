package evidence

import (
	"encoding/json"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestSourceFieldBindingV2PreservesLeadingZeroByteExact(t *testing.T) {
	binding := sourceFieldBindingTestV2(t, "fact-account", "row-a", "/rows/0", "/rows/0/account/cardNo", "0012-3456 7890", "001234567890")
	if binding.SourceExactValue != "0012-3456 7890" || binding.CanonicalAccountID != "001234567890" ||
		binding.SourceExactValueSHA256 != domainsecurity.SHA256Hex([]byte("0012-3456 7890")) {
		t.Fatalf("source-exact account bytes were not preserved: %#v", binding)
	}
	if err := ValidateSourceFieldBindingV2(binding); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFieldBindingV2RejectsNumericScientificAndLossyAccountSources(t *testing.T) {
	tests := []SourceFieldBindingInputV2{
		invalidSourceFieldInputV2("6222020000000000000", "number", "6222020000000000000"),
		invalidSourceFieldInputV2("6222020000000000000", SourceFieldBindingScalarTextV2, "6.22202e18"),
		invalidSourceFieldInputV2("001", SourceFieldBindingScalarTextV2, "001.0"),
		invalidSourceFieldInputV2("001", SourceFieldBindingScalarTextV2, " 001"),
		invalidSourceFieldInputV2("001", SourceFieldBindingScalarTextV2, "００１"),
	}
	for _, input := range tests {
		if binding, err := NewSourceFieldBindingV2(input); err == nil || binding.BindingDigest != "" {
			t.Fatalf("lossy account source minted authority: input=%#v binding=%#v err=%v", input, binding, err)
		}
	}
}

func TestSourceFieldBindingV2RejectsUnboundedOrControlBearingAuthorityKeys(t *testing.T) {
	tests := map[string]func(*SourceFieldBindingInputV2){
		"fact ID NUL":       func(input *SourceFieldBindingInputV2) { input.FactID = "fact\x00other" },
		"entity newline":    func(input *SourceFieldBindingInputV2) { input.CanonicalEntityID = "entity\nother" },
		"record C1 control": func(input *SourceFieldBindingInputV2) { input.SourceRecordID = "row\u0085other" },
		"record ID unbounded": func(input *SourceFieldBindingInputV2) {
			input.SourceRecordID = strings.Repeat("r", maxSourceFieldIdentifierBytesV2+1)
		},
		"path control": func(input *SourceFieldBindingInputV2) { input.SourceRecordPath = "/rows/\u0085" },
		"path unbounded": func(input *SourceFieldBindingInputV2) {
			input.SourceFieldPath = "/" + strings.Repeat("a", maxSourceFieldPathBytesV2)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := invalidSourceFieldInputV2("00123456", SourceFieldBindingScalarTextV2, "0012-3456")
			mutate(&input)
			if binding, err := NewSourceFieldBindingV2(input); err == nil || binding.BindingDigest != "" {
				t.Fatalf("unsafe authority key minted a source binding: binding=%#v err=%v", binding, err)
			}
		})
	}
}

func TestCanonicalEvidenceV2BindsSourceFieldToExactFactAndReceiptLineage(t *testing.T) {
	material := canonicalAccountEvidenceMaterialV2(t, "0012-3456789012345678", "00123456789012345678")
	body, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalEvidenceBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCanonicalEvidenceMaterial(canonical)
	if err != nil || parsed.SchemaVersion != CanonicalEvidenceVersionV2 || len(parsed.SourceFieldBindings) != 1 ||
		parsed.SourceFieldBindings[0].SourceExactValue != "0012-3456789012345678" {
		t.Fatalf("canonical V2 evidence did not preserve private exact value: parsed=%#v err=%v", parsed, err)
	}

	securityContext := evidenceReceiptTestContext(t, "thread-v2", "turn-v2", "case-v2", "snapshot-v2", 9)
	draft := evidenceReceiptTestDraft(t, securityContext, canonical, "receipt-v2")
	draft.PIIClassification = PIIControlled
	draft.RawSHA256 = material.SourceFieldBindings[0].RawArtifactSHA256
	draft.ReceiptDigest = evidenceReceiptDigest(draft)
	if err := ValidateCanonicalEvidenceAgainstReceipt(draft, canonical); err != nil {
		t.Fatalf("valid V2 source binding did not match receipt lineage: %v", err)
	}

	wrongRecord := draft
	wrongRecord.SourceRecordIDs = []string{"row-other"}
	wrongRecord.ReceiptDigest = evidenceReceiptDigest(wrongRecord)
	if err := ValidateCanonicalEvidenceAgainstReceipt(wrongRecord, canonical); err == nil {
		t.Fatal("source field outside receipt row lineage was accepted")
	}
	masked := draft
	masked.PIIClassification = PIIMasked
	masked.ReceiptDigest = evidenceReceiptDigest(masked)
	if err := ValidateCanonicalEvidenceAgainstReceipt(masked, canonical); err == nil {
		t.Fatal("masked receipt authorized private source-exact account material")
	}
	wrongRaw := draft
	wrongRaw.RawSHA256 = domainsecurity.SHA256Hex([]byte("different-raw-result"))
	wrongRaw.ReceiptDigest = evidenceReceiptDigest(wrongRaw)
	if err := ValidateCanonicalEvidenceAgainstReceipt(wrongRaw, canonical); err == nil {
		t.Fatal("source field hash outside the receipt raw result became authority")
	}
}

func TestSourceFieldBindingV2RequiresExactTextAtRawJSONPointer(t *testing.T) {
	raw := json.RawMessage(`{"structuredContent":{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":"0012-3456789012345678"}]}}`)
	rawHash := domainsecurity.SHA256Hex(raw)
	binding, err := NewSourceFieldBindingV2(SourceFieldBindingInputV2{
		FactID: "fact-account", ClaimType: ClaimAccount, CanonicalEntityID: "entity-a", CanonicalAccountID: "00123456789012345678",
		SourceRecordID: "row-a", RawArtifactSHA256: rawHash,
		SourceRecordSHA256: sourceRecordSHAFromRawV2(t, raw, "/structuredContent/rows/0"),
		SourceRecordPath:   "/structuredContent/rows/0", SourceRecordIDPath: "/structuredContent/rows/0/sourceRecordId",
		SourceEntityIDPath: "/structuredContent/rows/0/entityId",
		SourceFieldPath:    "/structuredContent/rows/0/accountId", SourceScalarKind: SourceFieldBindingScalarTextV2,
		SourceExactValue: "0012-3456789012345678",
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{binding})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	material := CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-account", ClaimType: ClaimAccount,
			NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-a", AccountID: "00123456789012345678"},
		}},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
	if err := ValidateSourceFieldBindingsAgainstRawResultV2(raw, rawHash, material); err != nil {
		t.Fatalf("byte-exact raw source field was rejected: %v", err)
	}

	for name, candidate := range map[string]json.RawMessage{
		"changed text":   json.RawMessage(`{"structuredContent":{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":"00123456789012345678"}]}}`),
		"numeric scalar": json.RawMessage(`{"structuredContent":{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":123456789012345678}]}}`),
		"missing path":   json.RawMessage(`{"structuredContent":{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a"}]}}`),
	} {
		t.Run(name, func(t *testing.T) {
			candidateHash := domainsecurity.SHA256Hex(candidate)
			candidateBinding, bindingErr := NewSourceFieldBindingV2(SourceFieldBindingInputV2{
				FactID: binding.FactID, ClaimType: binding.ClaimType, CanonicalEntityID: binding.CanonicalEntityID,
				CanonicalAccountID: binding.CanonicalAccountID,
				SourceRecordID:     binding.SourceRecordID, RawArtifactSHA256: candidateHash,
				SourceRecordSHA256: sourceRecordSHAFromRawV2(t, candidate, binding.SourceRecordPath),
				SourceRecordPath:   binding.SourceRecordPath, SourceRecordIDPath: binding.SourceRecordIDPath,
				SourceEntityIDPath: binding.SourceEntityIDPath,
				SourceFieldPath:    binding.SourceFieldPath, SourceScalarKind: binding.SourceScalarKind,
				SourceExactValue: binding.SourceExactValue,
			})
			if bindingErr != nil {
				t.Fatal(bindingErr)
			}
			candidateBindings, digestErr := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{candidateBinding})
			if digestErr != nil {
				t.Fatal(digestErr)
			}
			candidateMaterial := material
			candidateMaterial.SourceFieldBindings = candidateBindings
			candidateMaterial.SourceFieldBindingSetDigest, digestErr = SourceFieldBindingSetDigestV2(candidateBindings)
			if digestErr != nil {
				t.Fatal(digestErr)
			}
			if ValidateSourceFieldBindingsAgainstRawResultV2(candidate, candidateHash, candidateMaterial) == nil {
				t.Fatal("raw source mismatch became source-field authority")
			}
		})
	}
}

func TestPointerToRowBCannotClaimSourceRecordIDA(t *testing.T) {
	raw := json.RawMessage(`{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":"0012-3456"},{"sourceRecordId":"row-b","entityId":"entity-b","accountId":"0099-8877"}]}`)
	binding, err := NewSourceFieldBindingV2(SourceFieldBindingInputV2{
		FactID: "fact-account", ClaimType: ClaimAccount, CanonicalEntityID: "entity-b", CanonicalAccountID: "00998877",
		SourceRecordID: "row-a", RawArtifactSHA256: domainsecurity.SHA256Hex(raw),
		SourceRecordSHA256: sourceRecordSHAFromRawV2(t, raw, "/rows/1"),
		SourceRecordPath:   "/rows/1", SourceRecordIDPath: "/rows/1/sourceRecordId", SourceEntityIDPath: "/rows/1/entityId",
		SourceFieldPath: "/rows/1/accountId", SourceScalarKind: SourceFieldBindingScalarTextV2, SourceExactValue: "0099-8877",
	})
	if err != nil {
		t.Fatal(err)
	}
	material := sourceFieldRawValidationMaterialV2(t, binding, "entity-b")
	if ValidateSourceFieldBindingsAgainstRawResultV2(raw, domainsecurity.SHA256Hex(raw), material) == nil {
		t.Fatal("row B field borrowed row A sourceRecordId")
	}
}

func TestExactAccountRowCannotBeReboundToDifferentSubject(t *testing.T) {
	raw := json.RawMessage(`{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":"0012-3456"}]}`)
	rawHash := domainsecurity.SHA256Hex(raw)
	binding := sourceFieldBindingForRawRowV2(t, raw, rawHash, "fact-account", "entity-a", "row-a", "/rows/0", "0012-3456", "00123456")
	material := sourceFieldRawValidationMaterialV2(t, binding, "entity-b")
	body, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseCanonicalEvidenceMaterial(body); err == nil || parsed.SchemaVersion != 0 {
		t.Fatalf("account row for entity A supported a claim about entity B: parsed=%#v err=%v", parsed, err)
	}
}

func TestSourceAccountCannotBindDifferentRenderedSubject(t *testing.T) {
	raw := json.RawMessage(`{"rows":[{"sourceRecordId":"row-b","entityId":"entity-b","accountId":"0012-3456"}]}`)
	rawHash := domainsecurity.SHA256Hex(raw)
	binding := sourceFieldBindingForRawRowV2(t, raw, rawHash, "fact-account", "entity-b", "row-b", "/rows/0", "0012-3456", "00123456")
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{binding})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	material := CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-account", ClaimType: ClaimAccount,
			NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-a", EntityID: "entity-b", AccountID: "00123456"},
		}},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
	body, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseCanonicalEvidenceMaterial(body); err == nil || parsed.SchemaVersion != 0 {
		t.Fatalf("row B account was rebound to rendered subject A: parsed=%#v err=%v", parsed, err)
	}
}

func TestTwoRowsOneEnvelopeHaveDistinctRowHashes(t *testing.T) {
	raw := json.RawMessage(`{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":"0012-3456"},{"sourceRecordId":"row-b","entityId":"entity-b","accountId":"0099-8877"}]}`)
	rawHash := domainsecurity.SHA256Hex(raw)
	first := sourceFieldBindingForRawRowV2(t, raw, rawHash, "fact-a", "entity-a", "row-a", "/rows/0", "0012-3456", "00123456")
	second := sourceFieldBindingForRawRowV2(t, raw, rawHash, "fact-b", "entity-b", "row-b", "/rows/1", "0099-8877", "00998877")
	if first.SourceRecordSHA256 == second.SourceRecordSHA256 {
		t.Fatal("distinct source rows shared a record hash")
	}
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{first, second})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	material := CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{
			{FactID: "fact-a", ClaimType: ClaimAccount, NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-a", AccountID: "00123456"}},
			{FactID: "fact-b", ClaimType: ClaimAccount, NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-b", AccountID: "00998877"}},
		},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
	if err := ValidateSourceFieldBindingsAgainstRawResultV2(raw, rawHash, material); err != nil {
		t.Fatalf("independently bound rows were rejected: %v", err)
	}
}

func TestRowHashMustMatchImmutableRawArtifact(t *testing.T) {
	raw := json.RawMessage(`{"rows":[{"sourceRecordId":"row-a","entityId":"entity-a","accountId":"0012-3456"}]}`)
	rawHash := domainsecurity.SHA256Hex(raw)
	binding := sourceFieldBindingForRawRowV2(t, raw, rawHash, "fact-a", "entity-a", "row-a", "/rows/0", "0012-3456", "00123456")
	binding.SourceRecordSHA256 = domainsecurity.SHA256Hex([]byte("forged-row"))
	binding.BindingDigest = sourceFieldBindingDigestV2(binding)
	material := sourceFieldRawValidationMaterialV2(t, binding, "entity-a")
	if ValidateSourceFieldBindingsAgainstRawResultV2(raw, rawHash, material) == nil {
		t.Fatal("forged row hash became immutable source authority")
	}
}

func TestCanonicalEvidenceV2RejectsConflictingExactValuesAndV1Upgrade(t *testing.T) {
	first := sourceFieldBindingTestV2(t, "fact-account", "row-a", "/rows/0", "/rows/0/account", "0012-3456", "00123456")
	second := sourceFieldBindingTestV2(t, "fact-account", "row-b", "/rows/1", "/rows/1/account", "00123456", "00123456")
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{first, second})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-account", ClaimType: ClaimAccount,
			NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-a", AccountID: "00123456"},
		}},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
	body, _ := json.Marshal(conflicting)
	if parsed, err := ParseCanonicalEvidenceMaterial(body); err == nil || parsed.SchemaVersion != 0 {
		t.Fatalf("conflicting exact source values became controlled authority: parsed=%#v err=%v", parsed, err)
	}

	legacy := conflicting
	legacy.SchemaVersion = CanonicalEvidenceVersion
	legacy.Purpose = ""
	body, _ = json.Marshal(legacy)
	if parsed, err := ParseCanonicalEvidenceMaterial(body); err == nil || parsed.SchemaVersion != 0 {
		t.Fatalf("V1 material upgraded itself with V2 bindings: parsed=%#v err=%v", parsed, err)
	}
}

func TestCanonicalEvidenceV2RejectsEquivalentFactsWithDifferentSourceRepresentations(t *testing.T) {
	first := sourceFieldBindingTestV2(t, "fact-account-a", "row-a", "/rows/0", "/rows/0/account", "0012-3456", "00123456")
	second := sourceFieldBindingTestV2(t, "fact-account-b", "row-b", "/rows/1", "/rows/1/account", "00123456", "00123456")
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{first, second})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	payload := NormalizedClaimPayload{SubjectID: "entity-a", AccountID: "00123456"}
	material := CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{
			{FactID: "fact-account-a", ClaimType: ClaimAccount, NormalizedPayload: payload},
			{FactID: "fact-account-b", ClaimType: ClaimAccount, NormalizedPayload: payload},
		},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
	body, _ := json.Marshal(material)
	if parsed, err := ParseCanonicalEvidenceMaterial(body); err == nil || parsed.SchemaVersion != 0 {
		t.Fatalf("duplicate semantic facts offered caller-selectable exact accounts: parsed=%#v err=%v", parsed, err)
	}
}

func canonicalAccountEvidenceMaterialV2(t *testing.T, exactValue, canonicalAccountID string) CanonicalEvidenceMaterial {
	t.Helper()
	binding := sourceFieldBindingTestV2(t, "fact-account", "row-a", "/rows/0", "/rows/0/account/cardNo", exactValue, canonicalAccountID)
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{binding})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	return CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{{
			FactID: "fact-account", ClaimType: ClaimAccount,
			NormalizedPayload: NormalizedClaimPayload{SubjectID: "entity-a", AccountID: canonicalAccountID},
		}},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
}

func sourceFieldBindingTestV2(
	t *testing.T,
	factID string,
	recordID string,
	recordPath string,
	fieldPath string,
	exactValue string,
	canonicalAccountID string,
) SourceFieldBindingV2 {
	t.Helper()
	binding, err := NewSourceFieldBindingV2(SourceFieldBindingInputV2{
		FactID: factID, ClaimType: ClaimAccount, CanonicalEntityID: "entity-a", CanonicalAccountID: canonicalAccountID,
		SourceRecordID: recordID, RawArtifactSHA256: sourceFieldRawHashV2(recordID), SourceRecordSHA256: sourceFieldRecordHashV2(recordID),
		SourceRecordPath: recordPath, SourceRecordIDPath: recordPath + "/sourceRecordId",
		SourceEntityIDPath: recordPath + "/entityId",
		SourceFieldPath:    fieldPath, SourceScalarKind: SourceFieldBindingScalarTextV2, SourceExactValue: exactValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func invalidSourceFieldInputV2(canonicalAccountID, scalarKind, exactValue string) SourceFieldBindingInputV2 {
	return SourceFieldBindingInputV2{
		FactID: "fact-a", ClaimType: ClaimAccount, CanonicalEntityID: "entity-a", CanonicalAccountID: canonicalAccountID,
		SourceRecordID: "row-a", RawArtifactSHA256: sourceFieldRawHashV2("row-a"), SourceRecordSHA256: sourceFieldRecordHashV2("row-a"),
		SourceRecordPath: "/rows/0", SourceRecordIDPath: "/rows/0/sourceRecordId", SourceEntityIDPath: "/rows/0/entityId",
		SourceFieldPath:  "/rows/0/account",
		SourceScalarKind: scalarKind, SourceExactValue: exactValue,
	}
}

func sourceFieldBindingForRawRowV2(
	t *testing.T,
	raw json.RawMessage,
	rawHash string,
	factID string,
	entityID string,
	recordID string,
	recordPath string,
	exactValue string,
	canonicalAccountID string,
) SourceFieldBindingV2 {
	t.Helper()
	binding, err := NewSourceFieldBindingV2(SourceFieldBindingInputV2{
		FactID: factID, ClaimType: ClaimAccount, CanonicalEntityID: entityID, CanonicalAccountID: canonicalAccountID,
		SourceRecordID: recordID, RawArtifactSHA256: rawHash, SourceRecordSHA256: sourceRecordSHAFromRawV2(t, raw, recordPath),
		SourceRecordPath: recordPath, SourceRecordIDPath: recordPath + "/sourceRecordId",
		SourceEntityIDPath: recordPath + "/entityId", SourceFieldPath: recordPath + "/accountId",
		SourceScalarKind: SourceFieldBindingScalarTextV2, SourceExactValue: exactValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func sourceFieldRawValidationMaterialV2(t *testing.T, binding SourceFieldBindingV2, entityID string) CanonicalEvidenceMaterial {
	t.Helper()
	bindings, err := CanonicalSourceFieldBindingsV2([]SourceFieldBindingV2{binding})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := SourceFieldBindingSetDigestV2(bindings)
	if err != nil {
		t.Fatal(err)
	}
	return CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV2, Purpose: CanonicalEvidencePurposeV2,
		Facts: []CanonicalEvidenceFact{{
			FactID: binding.FactID, ClaimType: ClaimAccount,
			NormalizedPayload: NormalizedClaimPayload{SubjectID: entityID, AccountID: binding.CanonicalAccountID},
		}},
		SourceFieldBindings: bindings, SourceFieldBindingSetDigest: digest,
	}
}

func sourceRecordSHAFromRawV2(t *testing.T, raw json.RawMessage, recordPath string) string {
	t.Helper()
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	record, ok := resolveSourceJSONPointerV2(root, recordPath)
	if !ok {
		t.Fatalf("missing source record path %s", recordPath)
	}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.SHA256Hex(body)
}

func sourceFieldRawHashV2(recordID string) string {
	return domainsecurity.SHA256Hex([]byte("raw-artifact:" + recordID))
}

func sourceFieldRecordHashV2(recordID string) string {
	return domainsecurity.SHA256Hex([]byte("raw-record:" + recordID))
}
