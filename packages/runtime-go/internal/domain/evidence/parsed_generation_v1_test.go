package evidence

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestParsedPageV1ClosesGenerationReceiptCycleAndValidatesUnion(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "closed-dag")
	fileIDs := parsedOrderedSourceFileIDsV1(t, policy)
	accepted := parsedOutcomeFixtureV1(t, identity, policy, binding, 1, fileIDs[0], ParsedOutcomeAcceptedV1)
	rejected := parsedOutcomeFixtureV1(t, identity, policy, binding, 2, fileIDs[1], ParsedOutcomeRejectedV1)
	duplicate := parsedOutcomeFixtureV1(t, identity, policy, binding, 3, fileIDs[2], ParsedOutcomeDuplicateV1)
	duplicate.DuplicateKeyDigest = accepted.DuplicateKeyDigest
	duplicate.DuplicateOfSourceRecordID = accepted.SourceRecordID
	duplicate.CanonicalRowSHA256 = accepted.CanonicalRowSHA256

	page, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{accepted, rejected, duplicate})
	if err != nil {
		t.Fatal(err)
	}
	if page.AcceptedCount != 1 || page.RejectedCount != 1 || page.DuplicateCount != 1 ||
		ValidateParsedPageSequenceV1([]ParsedPageV1{page}) != nil {
		t.Fatalf("unexpected parsed page classification: %#v", page)
	}
	body, err := ParsedPageV1Bytes(page)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseParsedPageV1(body)
	if err != nil || parsed.PageDigest != page.PageDigest {
		t.Fatalf("canonical parsed page did not round trip: parsed=%#v err=%v", parsed, err)
	}
	for _, forbidden := range []string{"generationReceiptDigest", "lineageDigest", "datasetSnapshotId", "contextDigest"} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatalf("parsed page reintroduced a downstream hash-cycle field %q", forbidden)
		}
	}

	wrongUnion := page
	wrongUnion.Outcomes = append([]ParsedOutcomeV1(nil), page.Outcomes...)
	wrongUnion.Outcomes[1].CanonicalRowSHA256 = accepted.CanonicalRowSHA256
	wrongUnion.PageDigest = parsedPageDigestV1(wrongUnion)
	if ValidateParsedPageV1(wrongUnion) == nil {
		t.Fatal("rejected outcome carried accepted-row material")
	}

	relabelled := page
	relabelled.AcceptedCount++
	relabelled.RejectedCount--
	relabelled.PageDigest = parsedPageDigestV1(relabelled)
	if ValidateParsedPageV1(relabelled) == nil {
		t.Fatal("parsed page accepted relabelled classification counts")
	}
}

func TestParsedPageV1RejectsCrossBindingOrderingAndFalseDuplicateTargets(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "hostile")
	fileIDs := parsedOrderedSourceFileIDsV1(t, policy)
	accepted := parsedOutcomeFixtureV1(t, identity, policy, binding, 1, fileIDs[0], ParsedOutcomeAcceptedV1)
	duplicate := parsedOutcomeFixtureV1(t, identity, policy, binding, 2, fileIDs[1], ParsedOutcomeDuplicateV1)
	duplicate.DuplicateKeyDigest = accepted.DuplicateKeyDigest
	duplicate.DuplicateOfSourceRecordID = SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("invented-target"))
	duplicate.CanonicalRowSHA256 = accepted.CanonicalRowSHA256
	page, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{accepted, duplicate})
	if err != nil {
		t.Fatal(err)
	}
	if ValidateParsedPageSequenceV1([]ParsedPageV1{page}) == nil {
		t.Fatal("duplicate pointer to a non-accepted source record was accepted")
	}

	reordered := page
	reordered.Outcomes = []ParsedOutcomeV1{duplicate, accepted}
	reordered.Outcomes[0].OccurrenceOrdinal = 1
	reordered.Outcomes[1].OccurrenceOrdinal = 2
	reordered.PageDigest = parsedPageDigestV1(reordered)
	if ValidateParsedPageV1(reordered) == nil {
		t.Fatal("parsed page accepted descending source locators")
	}

	otherBinding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		WorkspaceRealPath: "/workspace", CaseID: "case-parsed-other",
		CaseBindingHash:          domainsecurity.SHA256Hex([]byte("other-case-binding")),
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("other-binding-observation")),
	})
	if err != nil {
		t.Fatal(err)
	}
	crossBinding, err := newParsedGenerationIdentityV1(parsedGenerationIdentityInputV1{
		Policy: policy, Binding: otherBinding,
		AcquisitionIntentDigest: identity.AcquisitionIntentDigest, AcquisitionIntentSHA256: identity.AcquisitionIntentSHA256,
		AcquisitionIntentByteLength: identity.AcquisitionIntentByteLength,
		RawArtifactManifestDigest:   identity.RawArtifactManifestDigest, RawArtifactManifestSHA256: identity.RawArtifactManifestSHA256,
		RawArtifactManifestLength: identity.RawArtifactManifestLength, MappingDigest: identity.MappingDigest,
		ProjectionSchemaDigest: identity.ProjectionSchemaDigest, ReaderModeDigest: identity.ReaderModeDigest,
		ConfigurationDigest: identity.ConfigurationDigest, ConfigurationSHA256: identity.ConfigurationSHA256,
		ConfigurationByteLength: identity.ConfigurationByteLength,
	})
	if err != nil || crossBinding.GenerationIntentDigest == identity.GenerationIntentDigest {
		t.Fatalf("cross-binding generation identity was not separated: other=%#v err=%v", crossBinding, err)
	}
}

func TestParsedPageV1StrictCanonicalParsingAndScalarValidation(t *testing.T) {
	identity, policy, binding := parsedGenerationIdentityFixtureV1(t, "strict")
	accepted := parsedOutcomeFixtureV1(t, identity, policy, binding, 1, parsedOrderedSourceFileIDsV1(t, policy)[0], ParsedOutcomeAcceptedV1)
	page, err := newParsedPageV1(identity, 1, []ParsedOutcomeV1{accepted})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ParsedPageV1Bytes(page)
	withUnknown := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"unknown":true}`)...)
	if _, err := ParseParsedPageV1(withUnknown); err == nil {
		t.Fatal("parsed page accepted an unknown field")
	}
	duplicateKey := bytes.Replace(body, []byte(`"pageNumber":1`), []byte(`"pageNumber":1,"pageNumber":1`), 1)
	if _, err := ParseParsedPageV1(duplicateKey); err == nil {
		t.Fatal("parsed page accepted a duplicate JSON key")
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "  "); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseParsedPageV1(indented.Bytes()); err == nil {
		t.Fatal("parsed page accepted non-canonical JSON bytes")
	}

	for _, scalar := range []ParsedTypedScalarV1{
		{Kind: "integer", Value: "01"},
		{Kind: "integer", Value: "-0"},
		{Kind: "decimal", Value: "1.0"},
		{Kind: "timestamp_utc", Value: "2026-07-18T12:00:00+08:00"},
		{Kind: "bytes_hex", Value: "ABC0"},
		{Kind: "unknown", Value: "x"},
	} {
		if validateParsedTypedScalarV1(scalar) == nil {
			t.Fatalf("non-canonical scalar was accepted: %#v", scalar)
		}
	}
	for _, scalar := range []ParsedTypedScalarV1{
		{Kind: "null", Value: ""}, {Kind: "bool", Value: "false"}, {Kind: "integer", Value: "-10"},
		{Kind: "decimal", Value: "10.25"}, {Kind: "float_hex", Value: "0x1.8p+00"},
		{Kind: "timestamp_utc", Value: "2026-07-18T04:00:00Z"}, {Kind: "text", Value: "untrusted prompt text"},
		{Kind: "bytes_hex", Value: "abc0"},
	} {
		if err := validateParsedTypedScalarV1(scalar); err != nil {
			t.Fatalf("canonical scalar was rejected: scalar=%#v err=%v", scalar, err)
		}
	}
}

func parsedGenerationIdentityFixtureV1(
	t *testing.T,
	seed string,
) (ParsedGenerationIdentityV1, SourceRowProducerPolicyV1, domainsecurity.DatasetSnapshotBindingKeyV1) {
	t.Helper()
	policy, ok := ResolveSourceRowProducerPolicyV1(FundsTransactionSourceRowPolicyIDV1)
	if !ok {
		t.Fatal("funds source row policy is unavailable")
	}
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		WorkspaceRealPath: "/workspace", CaseID: "case-parsed-" + seed,
		CaseBindingHash:          domainsecurity.SHA256Hex([]byte("case-binding:" + seed)),
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("binding-observation:" + seed)),
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := newParsedGenerationIdentityV1(parsedGenerationIdentityInputV1{
		Policy: policy, Binding: binding,
		AcquisitionIntentDigest: domainsecurity.SHA256Hex([]byte("intent-digest:" + seed)),
		AcquisitionIntentSHA256: domainsecurity.SHA256Hex([]byte("intent-body:" + seed)), AcquisitionIntentByteLength: 1024,
		RawArtifactManifestDigest: domainsecurity.SHA256Hex([]byte("manifest-digest:" + seed)),
		RawArtifactManifestSHA256: domainsecurity.SHA256Hex([]byte("manifest-body:" + seed)), RawArtifactManifestLength: 2048,
		MappingDigest:          domainsecurity.SHA256Hex([]byte("mapping:" + seed)),
		ProjectionSchemaDigest: domainsecurity.SHA256Hex([]byte("projection:" + seed)),
		ReaderModeDigest:       domainsecurity.SHA256Hex([]byte("reader:" + seed)),
		ConfigurationDigest:    domainsecurity.SHA256Hex([]byte("configuration-digest:" + seed)),
		ConfigurationSHA256:    domainsecurity.SHA256Hex([]byte("configuration-body:" + seed)), ConfigurationByteLength: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	return identity, policy, binding
}

func parsedOutcomeFixtureV1(
	t *testing.T,
	identity ParsedGenerationIdentityV1,
	policy SourceRowProducerPolicyV1,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	ordinal uint64,
	sourceFileID string,
	disposition string,
) ParsedOutcomeV1 {
	t.Helper()
	locator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{SourceFileID: sourceFileID, SourceRowNumber: ordinal})
	if err != nil {
		t.Fatal(err)
	}
	artifactSHA := domainsecurity.SHA256Hex([]byte("artifact:" + sourceFileID))
	recordID, err := DeriveSourceRowRecordIDV1(policy, binding, artifactSHA, locator)
	if err != nil {
		t.Fatal(err)
	}
	fields := []ParsedTypedFieldV1{
		{Name: "account", Scalar: ParsedTypedScalarV1{Kind: "text", Value: strings.Repeat("1", 20)}},
		{Name: "amount", Scalar: ParsedTypedScalarV1{Kind: "decimal", Value: "10.25"}},
	}
	outcome := ParsedOutcomeV1{
		SchemaVersion: ParsedOutcomeSchemaVersionV1, Purpose: ParsedOutcomePurposeV1, Disposition: disposition,
		OccurrenceOrdinal: ordinal, RawArtifactOrdinal: ordinal,
		RawArtifactEntryDigest: domainsecurity.SHA256Hex([]byte("entry:" + sourceFileID)),
		SourceRecordID:         recordID, Locator: locator, SourceArtifactSHA256: artifactSHA,
		ReaderRecordSHA256: domainsecurity.SHA256Hex([]byte("reader-record:" + sourceFileID)),
	}
	switch disposition {
	case ParsedOutcomeAcceptedV1:
		outcome.CanonicalTypedRow = fields
		outcome.CanonicalRowSHA256 = parsedCanonicalRowSHA256V1(fields)
		outcome.DuplicateKeyDigest = domainsecurity.SHA256Hex([]byte("dedupe:" + sourceFileID))
	case ParsedOutcomeRejectedV1:
		outcome.RejectionStage = ParsedRejectionStageSchemaV1
		outcome.RejectionCode = ParsedRejectionMissingRequiredFieldV1
	case ParsedOutcomeDuplicateV1:
		outcome.CanonicalRowSHA256 = parsedCanonicalRowSHA256V1(fields)
		outcome.DuplicateKeyDigest = domainsecurity.SHA256Hex([]byte("dedupe:" + sourceFileID))
		outcome.DuplicateOfSourceRecordID = SourceRowRecordIDPrefixV1 + domainsecurity.SHA256Hex([]byte("placeholder-earlier"))
	}
	if err := ValidateParsedOutcomeV1(identity, outcome); err != nil {
		t.Fatalf("parsed outcome fixture is invalid: %#v err=%v", outcome, err)
	}
	return outcome
}

func parsedOrderedSourceFileIDsV1(t *testing.T, policy SourceRowProducerPolicyV1) []string {
	t.Helper()
	type candidateV1 struct {
		fileID string
		digest string
	}
	candidates := make([]candidateV1, 3)
	for index := range candidates {
		fileID := "0000000000000000000" + string(rune('1'+index))
		locator, err := NewSourceRowLocatorV1(policy, SourceRowLocatorInputV1{SourceFileID: fileID, SourceRowNumber: uint64(index + 1)})
		if err != nil {
			t.Fatal(err)
		}
		candidates[index] = candidateV1{fileID: fileID, digest: locator.SourceFileIDDigest}
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].digest < candidates[right].digest })
	ordered := make([]string, len(candidates))
	for index := range candidates {
		ordered[index] = candidates[index].fileID
	}
	return ordered
}
