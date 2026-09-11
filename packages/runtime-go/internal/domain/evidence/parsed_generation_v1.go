package evidence

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ParsedGenerationIdentitySchemaVersionV1 = 1
	ParsedGenerationIdentityPurposeV1       = "analytix.parsed-generation-identity/v1"
	ParsedPageSchemaVersionV1               = 1
	ParsedPagePurposeV1                     = "analytix.parsed-page/v1"
	ParsedOutcomeSchemaVersionV1            = 1
	ParsedOutcomePurposeV1                  = "analytix.parsed-outcome/v1"

	ParsedOutcomeAcceptedV1  = "accepted"
	ParsedOutcomeRejectedV1  = "rejected"
	ParsedOutcomeDuplicateV1 = "duplicate"

	ParsedRejectionStageDecodeV1    = "decode"
	ParsedRejectionStageSchemaV1    = "schema"
	ParsedRejectionStageNormalizeV1 = "normalize"
	ParsedRejectionStageIdentityV1  = "identity"
	ParsedRejectionStagePolicyV1    = "policy"

	ParsedRejectionMalformedRecordV1        = "malformed_record"
	ParsedRejectionMissingRequiredFieldV1   = "missing_required_field"
	ParsedRejectionInvalidScalarV1          = "invalid_scalar"
	ParsedRejectionUnsupportedEncodingV1    = "unsupported_encoding"
	ParsedRejectionOutOfRangeV1             = "out_of_range"
	ParsedRejectionPolicyViolationV1        = "policy_violation"
	ParsedRejectionAmbiguousSourceRecordV1  = "ambiguous_source_record"
	ParsedGenerationConfigurationMaxBytesV1 = uint64(1024 * 1024)

	maxParsedTypedFieldsV1       = 256
	maxParsedFieldNameBytesV1    = 256
	maxParsedScalarValueBytesV1  = 1024 * 1024
	maxParsedDecimalValueBytesV1 = 256
)

var (
	parsedGenerationIntentDigestDomainV1 = []byte("analytix.parsed-generation-intent/digest/v1\x00")
	parsedPageDigestDomainV1             = []byte("analytix.parsed-page/digest/v1\x00")
	parsedCanonicalRowDigestDomainV1     = []byte("analytix.parsed-canonical-row/digest/v1\x00")
	parsedCanonicalIntegerV1             = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	parsedCanonicalDecimalV1             = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]*[1-9])?$`)
)

// ParsedGenerationIdentityV1 is the acyclic, exact-input identity shared by
// every parsed page and the later generation receipt. It references only raw
// and host-resolved configuration objects. It never references a parsed page,
// receipt, lineage, row ledger, snapshot, or current-state selector.
type ParsedGenerationIdentityV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`

	PolicyID         string `json:"policyId"`
	PolicyDigest     string `json:"policyDigest"`
	BindingKeyDigest string `json:"bindingKeyDigest"`

	AcquisitionIntentDigest     string `json:"acquisitionIntentDigest"`
	AcquisitionIntentSHA256     string `json:"acquisitionIntentSha256"`
	AcquisitionIntentByteLength uint64 `json:"acquisitionIntentByteLength"`
	RawArtifactManifestDigest   string `json:"rawArtifactManifestDigest"`
	RawArtifactManifestSHA256   string `json:"rawArtifactManifestSha256"`
	RawArtifactManifestLength   uint64 `json:"rawArtifactManifestByteLength"`

	ParserID               string `json:"parserId"`
	ParserVersion          string `json:"parserVersion"`
	MappingDigest          string `json:"mappingDigest"`
	ProjectionSchemaDigest string `json:"projectionSchemaDigest"`
	ReaderModeDigest       string `json:"readerModeDigest"`

	ConfigurationDigest     string `json:"configurationDigest"`
	ConfigurationSHA256     string `json:"configurationSha256"`
	ConfigurationByteLength uint64 `json:"configurationByteLength"`
	GenerationIntentDigest  string `json:"generationIntentDigest"`
}

// ParsedTypedScalarV1 is a closed, deterministic scalar. No provider or MCP
// text can add another kind. Text remains private evidence material and is not
// a public rendering contract.
type ParsedTypedScalarV1 struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type ParsedTypedFieldV1 struct {
	Name   string              `json:"name"`
	Scalar ParsedTypedScalarV1 `json:"scalar"`
}

// ParsedOutcomeV1 is a closed union. Accepted rows carry canonical typed
// values; rejected rows carry only fixed stage/code diagnostics; duplicate
// rows bind their canonical row hash to an earlier accepted source record.
type ParsedOutcomeV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	Disposition   string `json:"disposition"`

	OccurrenceOrdinal      uint64             `json:"occurrenceOrdinal"`
	RawArtifactOrdinal     uint64             `json:"rawArtifactOrdinal"`
	RawArtifactEntryDigest string             `json:"rawArtifactEntryDigest"`
	SourceRecordID         string             `json:"sourceRecordId"`
	Locator                SourceRowLocatorV1 `json:"locator"`
	SourceArtifactSHA256   string             `json:"sourceArtifactSha256"`
	ReaderRecordSHA256     string             `json:"readerRecordSha256"`

	CanonicalTypedRow  []ParsedTypedFieldV1 `json:"canonicalTypedRow,omitempty"`
	CanonicalRowSHA256 string               `json:"canonicalRowSha256,omitempty"`

	RejectionStage string `json:"rejectionStage,omitempty"`
	RejectionCode  string `json:"rejectionCode,omitempty"`

	DuplicateKeyDigest        string `json:"duplicateKeyDigest,omitempty"`
	DuplicateOfSourceRecordID string `json:"duplicateOfSourceRecordId,omitempty"`
}

// ParsedPageV1 precedes its receipt in the hash DAG. In particular, it has no
// generationReceiptDigest and no SourceRowLineageV1 value.
type ParsedPageV1 struct {
	SchemaVersion  int                        `json:"schemaVersion"`
	Purpose        string                     `json:"purpose"`
	Identity       ParsedGenerationIdentityV1 `json:"identity"`
	PageNumber     uint64                     `json:"pageNumber"`
	Outcomes       []ParsedOutcomeV1          `json:"outcomes"`
	OutcomeCount   uint32                     `json:"outcomeCount"`
	AcceptedCount  uint32                     `json:"acceptedCount"`
	RejectedCount  uint32                     `json:"rejectedCount"`
	DuplicateCount uint32                     `json:"duplicateCount"`
	PageDigest     string                     `json:"pageDigest"`
}

type parsedGenerationIdentityInputV1 struct {
	Policy  SourceRowProducerPolicyV1
	Binding domainsecurity.DatasetSnapshotBindingKeyV1

	AcquisitionIntentDigest     string
	AcquisitionIntentSHA256     string
	AcquisitionIntentByteLength uint64
	RawArtifactManifestDigest   string
	RawArtifactManifestSHA256   string
	RawArtifactManifestLength   uint64
	MappingDigest               string
	ProjectionSchemaDigest      string
	ReaderModeDigest            string
	ConfigurationDigest         string
	ConfigurationSHA256         string
	ConfigurationByteLength     uint64
}

func newParsedGenerationIdentityV1(input parsedGenerationIdentityInputV1) (ParsedGenerationIdentityV1, error) {
	identity := ParsedGenerationIdentityV1{
		SchemaVersion: ParsedGenerationIdentitySchemaVersionV1, Purpose: ParsedGenerationIdentityPurposeV1,
		PolicyID: input.Policy.PolicyID, PolicyDigest: input.Policy.PolicyDigest,
		BindingKeyDigest:        input.Binding.BindingKeyDigest,
		AcquisitionIntentDigest: input.AcquisitionIntentDigest, AcquisitionIntentSHA256: input.AcquisitionIntentSHA256,
		AcquisitionIntentByteLength: input.AcquisitionIntentByteLength,
		RawArtifactManifestDigest:   input.RawArtifactManifestDigest, RawArtifactManifestSHA256: input.RawArtifactManifestSHA256,
		RawArtifactManifestLength: input.RawArtifactManifestLength,
		ParserID:                  input.Policy.ParserID, ParserVersion: input.Policy.ParserVersion,
		MappingDigest: input.MappingDigest, ProjectionSchemaDigest: input.ProjectionSchemaDigest,
		ReaderModeDigest: input.ReaderModeDigest, ConfigurationDigest: input.ConfigurationDigest,
		ConfigurationSHA256: input.ConfigurationSHA256, ConfigurationByteLength: input.ConfigurationByteLength,
	}
	identity.GenerationIntentDigest = parsedGenerationIntentDigestV1(identity)
	if ValidateSourceRowProducerPolicyV1(input.Policy) != nil ||
		domainsecurity.ValidateDatasetSnapshotBindingKeyV1(input.Binding) != nil ||
		ValidateParsedGenerationIdentityV1(identity) != nil {
		return ParsedGenerationIdentityV1{}, errors.New("parsed generation identity input is invalid")
	}
	return identity, nil
}

func ValidateParsedGenerationIdentityV1(identity ParsedGenerationIdentityV1) error {
	policy, ok := ResolveSourceRowProducerPolicyV1(identity.PolicyID)
	if !ok || identity.SchemaVersion != ParsedGenerationIdentitySchemaVersionV1 ||
		identity.Purpose != ParsedGenerationIdentityPurposeV1 || identity.PolicyDigest != policy.PolicyDigest ||
		!validSourceRowSHA256V1(identity.BindingKeyDigest) ||
		!validSourceRowSHA256V1(identity.AcquisitionIntentDigest) || !validSourceRowSHA256V1(identity.AcquisitionIntentSHA256) ||
		identity.AcquisitionIntentByteLength == 0 || identity.AcquisitionIntentByteLength > maxRawArtifactAcquisitionIntentBytesV1 ||
		!validSourceRowSHA256V1(identity.RawArtifactManifestDigest) || !validSourceRowSHA256V1(identity.RawArtifactManifestSHA256) ||
		identity.RawArtifactManifestLength == 0 || identity.RawArtifactManifestLength > maxRawArtifactManifestBytesV1 ||
		identity.ParserID != policy.ParserID || identity.ParserVersion != policy.ParserVersion ||
		!validSourceRowSHA256V1(identity.MappingDigest) || !validSourceRowSHA256V1(identity.ProjectionSchemaDigest) ||
		!validSourceRowSHA256V1(identity.ReaderModeDigest) || !validSourceRowSHA256V1(identity.ConfigurationDigest) ||
		!validSourceRowSHA256V1(identity.ConfigurationSHA256) || identity.ConfigurationByteLength == 0 ||
		identity.ConfigurationByteLength > ParsedGenerationConfigurationMaxBytesV1 ||
		!validSourceRowSHA256V1(identity.GenerationIntentDigest) ||
		identity.GenerationIntentDigest != parsedGenerationIntentDigestV1(identity) {
		return errors.New("parsed generation identity is invalid")
	}
	return nil
}

func ParseParsedGenerationIdentityV1(raw []byte) (ParsedGenerationIdentityV1, error) {
	var identity ParsedGenerationIdentityV1
	if err := parseCanonicalSourceRowContractV1(raw, &identity, 64*1024, 256, maxSourceRowPolicyTextBytesV1); err != nil {
		return ParsedGenerationIdentityV1{}, err
	}
	return identity, ValidateParsedGenerationIdentityV1(identity)
}

func ParsedGenerationIdentityV1Bytes(identity ParsedGenerationIdentityV1) ([]byte, error) {
	if err := ValidateParsedGenerationIdentityV1(identity); err != nil {
		return nil, err
	}
	return json.Marshal(identity)
}

func newParsedPageV1(identity ParsedGenerationIdentityV1, pageNumber uint64, outcomes []ParsedOutcomeV1) (ParsedPageV1, error) {
	page := ParsedPageV1{
		SchemaVersion: ParsedPageSchemaVersionV1, Purpose: ParsedPagePurposeV1,
		Identity: identity, PageNumber: pageNumber, Outcomes: append([]ParsedOutcomeV1(nil), outcomes...),
		OutcomeCount: uint32(len(outcomes)),
	}
	for _, outcome := range page.Outcomes {
		switch outcome.Disposition {
		case ParsedOutcomeAcceptedV1:
			page.AcceptedCount++
		case ParsedOutcomeRejectedV1:
			page.RejectedCount++
		case ParsedOutcomeDuplicateV1:
			page.DuplicateCount++
		}
	}
	page.PageDigest = parsedPageDigestV1(page)
	if err := ValidateParsedPageV1(page); err != nil {
		return ParsedPageV1{}, err
	}
	return page, nil
}

func ValidateParsedPageV1(page ParsedPageV1) error {
	if page.SchemaVersion != ParsedPageSchemaVersionV1 || page.Purpose != ParsedPagePurposeV1 ||
		ValidateParsedGenerationIdentityV1(page.Identity) != nil || page.PageNumber == 0 ||
		page.PageNumber > maxSourceRowJSONIntegerV1 || len(page.Outcomes) == 0 ||
		uint64(len(page.Outcomes)) > uint64(resolveParsedPageMaxRowsV1(page.Identity.PolicyID)) ||
		page.OutcomeCount != uint32(len(page.Outcomes)) || !validSourceRowSHA256V1(page.PageDigest) {
		return errors.New("parsed page is invalid")
	}
	policy, _ := ResolveSourceRowProducerPolicyV1(page.Identity.PolicyID)
	maxRows := uint64(policy.MaxRowsPerPage)
	if page.PageNumber-1 > (maxSourceRowJSONIntegerV1-1)/maxRows {
		return errors.New("parsed page occurrence range overflows")
	}
	expectedOccurrence := (page.PageNumber-1)*maxRows + 1
	var accepted, rejected, duplicate uint32
	var previous *ParsedOutcomeV1
	for index := range page.Outcomes {
		outcome := page.Outcomes[index]
		if outcome.OccurrenceOrdinal != expectedOccurrence+uint64(index) ||
			ValidateParsedOutcomeV1(page.Identity, outcome) != nil {
			return errors.New("parsed page outcome is invalid")
		}
		if previous != nil && compareParsedOutcomeLocatorV1(*previous, outcome) >= 0 {
			return errors.New("parsed page source locators are not strictly ordered")
		}
		previous = &page.Outcomes[index]
		switch outcome.Disposition {
		case ParsedOutcomeAcceptedV1:
			accepted++
		case ParsedOutcomeRejectedV1:
			rejected++
		case ParsedOutcomeDuplicateV1:
			duplicate++
		}
	}
	if accepted != page.AcceptedCount || rejected != page.RejectedCount || duplicate != page.DuplicateCount ||
		uint64(accepted)+uint64(rejected)+uint64(duplicate) != uint64(page.OutcomeCount) ||
		page.PageDigest != parsedPageDigestV1(page) {
		return errors.New("parsed page classification counts or digest are invalid")
	}
	body, err := json.Marshal(page)
	if err != nil || uint64(len(body)) > policy.MaxPageBytes {
		return errors.New("parsed page exceeds its registered byte limit")
	}
	return nil
}

func ValidateParsedOutcomeV1(identity ParsedGenerationIdentityV1, outcome ParsedOutcomeV1) error {
	policy, ok := ResolveSourceRowProducerPolicyV1(identity.PolicyID)
	if !ok || ValidateParsedGenerationIdentityV1(identity) != nil ||
		outcome.SchemaVersion != ParsedOutcomeSchemaVersionV1 || outcome.Purpose != ParsedOutcomePurposeV1 ||
		outcome.OccurrenceOrdinal == 0 || outcome.OccurrenceOrdinal > maxSourceRowJSONIntegerV1 ||
		outcome.RawArtifactOrdinal == 0 || outcome.RawArtifactOrdinal > maxRawArtifactCountV1 ||
		!validSourceRowSHA256V1(outcome.RawArtifactEntryDigest) ||
		!validSourceRowRecordIDV1(outcome.SourceRecordID) || ValidateSourceRowLocatorV1(policy, outcome.Locator) != nil ||
		!validSourceRowSHA256V1(outcome.SourceArtifactSHA256) || !validSourceRowSHA256V1(outcome.ReaderRecordSHA256) {
		return errors.New("parsed outcome identity is invalid")
	}
	expectedRecordID := deriveSourceRowRecordIDFromDigestsV1(
		identity.PolicyDigest, identity.BindingKeyDigest, outcome.SourceArtifactSHA256, outcome.Locator,
	)
	if outcome.SourceRecordID != expectedRecordID {
		return errors.New("parsed outcome source record id is detached from its source occurrence")
	}
	switch outcome.Disposition {
	case ParsedOutcomeAcceptedV1:
		if len(outcome.CanonicalTypedRow) == 0 || len(outcome.CanonicalTypedRow) > maxParsedTypedFieldsV1 ||
			!validSourceRowSHA256V1(outcome.CanonicalRowSHA256) ||
			outcome.CanonicalRowSHA256 != parsedCanonicalRowSHA256V1(outcome.CanonicalTypedRow) ||
			outcome.RejectionStage != "" || outcome.RejectionCode != "" ||
			!validSourceRowSHA256V1(outcome.DuplicateKeyDigest) ||
			outcome.DuplicateOfSourceRecordID != "" || validateParsedTypedRowV1(outcome.CanonicalTypedRow) != nil {
			return errors.New("accepted parsed outcome union is invalid")
		}
	case ParsedOutcomeRejectedV1:
		if outcome.CanonicalTypedRow != nil || outcome.CanonicalRowSHA256 != "" ||
			!validParsedRejectionStageV1(outcome.RejectionStage) || !validParsedRejectionCodeV1(outcome.RejectionCode) ||
			outcome.DuplicateKeyDigest != "" || outcome.DuplicateOfSourceRecordID != "" {
			return errors.New("rejected parsed outcome union is invalid")
		}
	case ParsedOutcomeDuplicateV1:
		if outcome.CanonicalTypedRow != nil || !validSourceRowSHA256V1(outcome.CanonicalRowSHA256) ||
			outcome.RejectionStage != "" || outcome.RejectionCode != "" ||
			!validSourceRowSHA256V1(outcome.DuplicateKeyDigest) ||
			!validSourceRowRecordIDV1(outcome.DuplicateOfSourceRecordID) ||
			outcome.DuplicateOfSourceRecordID == outcome.SourceRecordID {
			return errors.New("duplicate parsed outcome union is invalid")
		}
	default:
		return errors.New("parsed outcome disposition is unsupported")
	}
	return nil
}

// ValidateParsedPageSequenceV1 proves one complete ordered generation page
// sequence. It rejects gaps, partial non-terminal pages, duplicate occurrence
// identities, and duplicate pointers that do not target the first earlier
// accepted row for the exact same deterministic key and canonical row hash.
// It is structural comparison only and performs no storage or currentness
// lookup.
func ValidateParsedPageSequenceV1(pages []ParsedPageV1) error {
	if len(pages) == 0 {
		return errors.New("parsed page sequence is empty")
	}
	identity := pages[0].Identity
	if ValidateParsedGenerationIdentityV1(identity) != nil {
		return errors.New("parsed page sequence identity is invalid")
	}
	policy, _ := ResolveSourceRowProducerPolicyV1(identity.PolicyID)
	seenRecords := make(map[string]struct{})
	type acceptedDuplicateTargetV1 struct {
		sourceRecordID string
		rowSHA256      string
	}
	acceptedByKey := make(map[string]acceptedDuplicateTargetV1)
	var previous *ParsedOutcomeV1
	for pageIndex := range pages {
		page := pages[pageIndex]
		if page.Identity != identity || page.PageNumber != uint64(pageIndex+1) || ValidateParsedPageV1(page) != nil ||
			pageIndex < len(pages)-1 && page.OutcomeCount != policy.MaxRowsPerPage {
			return errors.New("parsed page sequence has a gap, mismatch, or partial non-terminal page")
		}
		for outcomeIndex := range page.Outcomes {
			outcome := page.Outcomes[outcomeIndex]
			if previous != nil && compareParsedOutcomeLocatorV1(*previous, outcome) >= 0 {
				return errors.New("parsed page sequence locators are not globally ordered")
			}
			if _, duplicate := seenRecords[outcome.SourceRecordID]; duplicate {
				return errors.New("parsed page sequence repeats a source occurrence")
			}
			seenRecords[outcome.SourceRecordID] = struct{}{}
			switch outcome.Disposition {
			case ParsedOutcomeAcceptedV1:
				if _, duplicateKey := acceptedByKey[outcome.DuplicateKeyDigest]; duplicateKey {
					return errors.New("parsed page sequence accepts a repeated deterministic key")
				}
				acceptedByKey[outcome.DuplicateKeyDigest] = acceptedDuplicateTargetV1{
					sourceRecordID: outcome.SourceRecordID, rowSHA256: outcome.CanonicalRowSHA256,
				}
			case ParsedOutcomeDuplicateV1:
				target, exists := acceptedByKey[outcome.DuplicateKeyDigest]
				if !exists || target.sourceRecordID != outcome.DuplicateOfSourceRecordID ||
					target.rowSHA256 != outcome.CanonicalRowSHA256 {
					return errors.New("parsed duplicate does not target the first exact earlier accepted row")
				}
			}
			previous = &pages[pageIndex].Outcomes[outcomeIndex]
		}
	}
	return nil
}

func ParseParsedPageV1(raw []byte) (ParsedPageV1, error) {
	var page ParsedPageV1
	if err := parseCanonicalSourceRowContractV1(raw, &page, 1024*1024, 100_000, maxParsedScalarValueBytesV1); err != nil {
		return ParsedPageV1{}, err
	}
	return page, ValidateParsedPageV1(page)
}

func ParsedPageV1Bytes(page ParsedPageV1) ([]byte, error) {
	if err := ValidateParsedPageV1(page); err != nil {
		return nil, err
	}
	return json.Marshal(page)
}

func validateParsedTypedRowV1(fields []ParsedTypedFieldV1) error {
	if len(fields) == 0 || len(fields) > maxParsedTypedFieldsV1 {
		return errors.New("canonical typed row field count is invalid")
	}
	for index, field := range fields {
		if !validParsedFieldNameV1(field.Name) || index > 0 && fields[index-1].Name >= field.Name ||
			validateParsedTypedScalarV1(field.Scalar) != nil {
			return errors.New("canonical typed row is not ordered and valid")
		}
	}
	return nil
}

func validateParsedTypedScalarV1(scalar ParsedTypedScalarV1) error {
	if len(scalar.Value) > maxParsedScalarValueBytesV1 || !utf8.ValidString(scalar.Value) {
		return errors.New("canonical typed scalar value is invalid")
	}
	switch scalar.Kind {
	case "null":
		if scalar.Value != "" {
			return errors.New("canonical null scalar is invalid")
		}
	case "bool":
		if scalar.Value != "true" && scalar.Value != "false" {
			return errors.New("canonical bool scalar is invalid")
		}
	case "integer":
		if len(scalar.Value) > maxParsedDecimalValueBytesV1 || !parsedCanonicalIntegerV1.MatchString(scalar.Value) || scalar.Value == "-0" {
			return errors.New("canonical integer scalar is invalid")
		}
	case "decimal":
		if len(scalar.Value) > maxParsedDecimalValueBytesV1 || !parsedCanonicalDecimalV1.MatchString(scalar.Value) || scalar.Value == "-0" {
			return errors.New("canonical decimal scalar is invalid")
		}
	case "float_hex":
		value, err := strconv.ParseFloat(scalar.Value, 64)
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) || strconv.FormatFloat(value, 'x', -1, 64) != scalar.Value {
			return errors.New("canonical float scalar is invalid")
		}
	case "timestamp_utc":
		value, err := time.Parse(time.RFC3339Nano, scalar.Value)
		if err != nil || value.UTC().Format(time.RFC3339Nano) != scalar.Value || !strings.HasSuffix(scalar.Value, "Z") {
			return errors.New("canonical timestamp scalar is invalid")
		}
	case "text":
		if strings.IndexByte(scalar.Value, 0) >= 0 {
			return errors.New("canonical text scalar contains NUL")
		}
	case "bytes_hex":
		if len(scalar.Value)%2 != 0 || strings.ToLower(scalar.Value) != scalar.Value {
			return errors.New("canonical bytes scalar is invalid")
		}
		if _, err := hex.DecodeString(scalar.Value); err != nil {
			return errors.New("canonical bytes scalar is invalid")
		}
	default:
		return errors.New("canonical typed scalar kind is unsupported")
	}
	return nil
}

func parsedCanonicalRowSHA256V1(fields []ParsedTypedFieldV1) string {
	body, _ := json.Marshal(fields)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedCanonicalRowDigestDomainV1...), body...))
}

func parsedGenerationIntentDigestV1(identity ParsedGenerationIdentityV1) string {
	identity.GenerationIntentDigest = ""
	body, _ := json.Marshal(identity)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedGenerationIntentDigestDomainV1...), body...))
}

func parsedPageDigestV1(page ParsedPageV1) string {
	page.PageDigest = ""
	body, _ := json.Marshal(page)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), parsedPageDigestDomainV1...), body...))
}

func compareParsedOutcomeLocatorV1(left, right ParsedOutcomeV1) int {
	if left.Locator.SourceFileIDDigest < right.Locator.SourceFileIDDigest {
		return -1
	}
	if left.Locator.SourceFileIDDigest > right.Locator.SourceFileIDDigest {
		return 1
	}
	if left.Locator.SourceRowNumber < right.Locator.SourceRowNumber {
		return -1
	}
	if left.Locator.SourceRowNumber > right.Locator.SourceRowNumber {
		return 1
	}
	return 0
}

func resolveParsedPageMaxRowsV1(policyID string) uint32 {
	policy, ok := ResolveSourceRowProducerPolicyV1(policyID)
	if !ok {
		return 0
	}
	return policy.MaxRowsPerPage
}

func validParsedFieldNameV1(value string) bool {
	if value == "" || len(value) > maxParsedFieldNameBytesV1 || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return false
		}
	}
	return true
}

func validParsedRejectionStageV1(value string) bool {
	switch value {
	case ParsedRejectionStageDecodeV1, ParsedRejectionStageSchemaV1, ParsedRejectionStageNormalizeV1,
		ParsedRejectionStageIdentityV1, ParsedRejectionStagePolicyV1:
		return true
	default:
		return false
	}
}

func validParsedRejectionCodeV1(value string) bool {
	switch value {
	case ParsedRejectionMalformedRecordV1, ParsedRejectionMissingRequiredFieldV1,
		ParsedRejectionInvalidScalarV1, ParsedRejectionUnsupportedEncodingV1,
		ParsedRejectionOutOfRangeV1, ParsedRejectionPolicyViolationV1,
		ParsedRejectionAmbiguousSourceRecordV1:
		return true
	default:
		return false
	}
}
