package evidence

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAcceptedSlotSourceBindingV1IsValueFreeAndFactExact(t *testing.T) {
	const reference = "cer1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	factID := "fact_aggregate_" + domainsecurity.SHA256Hex([]byte("accepted-slot-fact"))
	binding, err := NewAcceptedSlotSourceBindingV1(AcceptedSlotSourceBindingInputV1{
		FactIDs: []string{factID}, EntityReference: reference,
		SourceRecordID: SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("source-record")),
		SourceFileID:   "0123456789abcdefabcd", SourceRowNumber: 41,
		Field: AcceptedSlotSourceFieldAccountV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := AcceptedSlotSourceBindingSetDigestV1([]AcceptedSlotSourceBindingV1{binding})
	if err != nil {
		t.Fatal(err)
	}
	material := CanonicalEvidenceMaterial{
		SchemaVersion: CanonicalEvidenceVersionV3, Purpose: CanonicalEvidencePurposeV3,
		Facts: []CanonicalEvidenceFact{{
			FactID: factID, ClaimType: ClaimCount,
			NormalizedPayload: NormalizedClaimPayload{
				SubjectID: reference, EntityID: reference, Count: "1",
				StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z",
				Granularity: "aggregate",
			},
		}},
		AcceptedSlotSourceBindings:         []AcceptedSlotSourceBindingV1{binding},
		AcceptedSlotSourceBindingSetDigest: digest,
	}
	body, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCanonicalEvidenceMaterial(body)
	if err != nil || len(parsed.AcceptedSlotSourceBindings) != 1 ||
		parsed.AcceptedSlotSourceBindings[0].FactIDs[0] != factID {
		t.Fatalf("accepted source binding did not survive canonical validation: parsed=%#v err=%v", parsed, err)
	}
	for _, forbidden := range [][]byte{
		[]byte("/private/"), []byte("6222 0212-3456 7890"), []byte("displayValue"),
		[]byte("canonicalValue"), []byte("label"),
	} {
		if bytes.Contains(body, forbidden) {
			t.Fatalf("value-free source binding persisted forbidden material %q: %s", forbidden, body)
		}
	}

	wrongFact := material
	wrongFact.AcceptedSlotSourceBindings = append([]AcceptedSlotSourceBindingV1(nil), material.AcceptedSlotSourceBindings...)
	wrongFact.AcceptedSlotSourceBindings[0].FactIDs = []string{"afagg1_" + strings.Repeat("f", 64)}
	wrongFact.AcceptedSlotSourceBindings[0].BindingDigest = acceptedSlotSourceBindingDigestV1(wrongFact.AcceptedSlotSourceBindings[0])
	wrongFact.AcceptedSlotSourceBindingSetDigest, _ = AcceptedSlotSourceBindingSetDigestV1(wrongFact.AcceptedSlotSourceBindings)
	wrongBody, _ := json.Marshal(wrongFact)
	if _, err := ParseCanonicalEvidenceMaterial(wrongBody); err == nil {
		t.Fatal("source lineage detached from its exact canonical fact")
	}

	extraFact := material
	extraFact.Facts = append([]CanonicalEvidenceFact(nil), material.Facts...)
	extraFact.Facts = append(extraFact.Facts, CanonicalEvidenceFact{
		FactID:    "fact_aggregate_" + domainsecurity.SHA256Hex([]byte("other-scope-fact")),
		ClaimType: ClaimCount,
		NormalizedPayload: NormalizedClaimPayload{
			SubjectID: reference, EntityID: reference, Count: "2",
			StartAt: "2026-02-01T00:00:00Z", EndAt: "2026-02-28T23:59:59Z",
			Granularity: "aggregate",
		},
	})
	extraBody, _ := json.Marshal(extraFact)
	if _, err := ParseCanonicalEvidenceMaterial(extraBody); err == nil {
		t.Fatal("accepted slot lineage was allowed to select a subset from another fact scope")
	}
}

func TestAcceptedSlotSourceBindingV1RejectsCallerPathsOpenFieldsAndDuplicateLocators(t *testing.T) {
	const reference = "cer1_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	base := AcceptedSlotSourceBindingInputV1{
		FactIDs: []string{"fact-a"}, EntityReference: reference,
		SourceRecordID: SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("source-record-a")),
		SourceFileID:   "fedcba9876543210abcd", SourceRowNumber: 7,
		Field: AcceptedSlotSourceFieldAccountV1,
	}
	for name, mutate := range map[string]func(*AcceptedSlotSourceBindingInputV1){
		"path identity": func(input *AcceptedSlotSourceBindingInputV1) { input.SourceFileID = "/private/case.csv" },
		"open field":    func(input *AcceptedSlotSourceBindingInputV1) { input.Field = "phone" },
		"missing row":   func(input *AcceptedSlotSourceBindingInputV1) { input.SourceRowNumber = 0 },
		"missing fact":  func(input *AcceptedSlotSourceBindingInputV1) { input.FactIDs = nil },
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if _, err := NewAcceptedSlotSourceBindingV1(input); err == nil {
				t.Fatal("invalid caller-provided source lineage was accepted")
			}
		})
	}
	first, err := NewAcceptedSlotSourceBindingV1(base)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := base
	secondInput.SourceRecordID = SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("source-record-b"))
	second, err := NewAcceptedSlotSourceBindingV1(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalAcceptedSlotSourceBindingsV1([]AcceptedSlotSourceBindingV1{first, second}); err == nil {
		t.Fatal("one caller locator was allowed to select multiple source records")
	}
}

func TestAcceptedSlotSourceFieldV1FollowsClosedModelAliasType(t *testing.T) {
	for _, test := range []struct {
		alias string
		want  string
	}{
		{alias: "acct:1", want: AcceptedSlotSourceFieldAccountV1},
		{alias: "card:2", want: AcceptedSlotSourceFieldCardV1},
	} {
		field, err := AcceptedSlotSourceFieldForModelEntityAliasV1(test.alias)
		if err != nil || field != test.want || !IsAcceptedSlotSourceFieldV1(field) {
			t.Fatalf("closed source field for %q = %q err=%v", test.alias, field, err)
		}
	}
	for _, invalid := range []string{"account:1", "acct:01", "person:1", "account"} {
		if field, err := AcceptedSlotSourceFieldForModelEntityAliasV1(invalid); err == nil || field != "" {
			t.Fatalf("invalid alias %q produced source field %q", invalid, field)
		}
	}
	if IsAcceptedSlotSourceFieldV1("phone") {
		t.Fatal("open source field entered the accepted-slot closed enum")
	}

	const reference = "cer1_cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	binding, err := NewAcceptedSlotSourceBindingV1(AcceptedSlotSourceBindingInputV1{
		FactIDs: []string{"fact-card"}, EntityReference: reference,
		SourceRecordID: SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("card-record")),
		SourceFileID:   "0123456789abcdefabcd", SourceRowNumber: 9,
		Field: AcceptedSlotSourceFieldCardV1,
	})
	if err != nil || binding.Field != AcceptedSlotSourceFieldCardV1 {
		t.Fatalf("closed card source binding was rejected: binding=%#v err=%v", binding, err)
	}
}
