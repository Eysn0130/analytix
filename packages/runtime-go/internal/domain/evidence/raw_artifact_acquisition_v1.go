package evidence

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	RawArtifactAcquisitionIntentSchemaVersionV1 = 1
	RawArtifactAcquisitionIntentPurposeV1       = "analytix.raw-artifact-acquisition-intent/v1"
	RawArtifactSourceLocatorSchemaVersionV1     = 1
	RawArtifactSourceLocatorPurposeV1           = "analytix.raw-artifact-source-locator/v1"
	RawArtifactAcquisitionMethodV1              = "host_case_import/v1"

	maxRawArtifactAcquisitionIntentBytesV1 = 64 * 1024
	maxRawArtifactSourceLocatorBytesV1     = 64 * 1024
)

var (
	rawArtifactAcquisitionIntentDigestDomainV1 = []byte("analytix.raw-artifact-acquisition-intent/digest/v1\x00")
	rawArtifactSourceLocatorDigestDomainV1     = []byte("analytix.raw-artifact-source-locator/digest/v1\x00")
)

// RawArtifactAcquisitionIntentV1 is pre-content intent material. It cannot
// reference a chunk, content root, entry, manifest, snapshot, or commit
// receipt. That one-way dependency keeps the acquisition DAG acyclic:
// intent -> locator -> content -> manifest -> commit receipt -> DSV2.
//
// This restricted value is structural only. Production code must derive it
// from an authenticated host request and must keep it out of ordinary public
// projections.
type RawArtifactAcquisitionIntentV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Purpose                string `json:"purpose"`
	BindingKeyDigest       string `json:"bindingKeyDigest"`
	AcquisitionMethod      string `json:"acquisitionMethod"`
	AcquiredAt             string `json:"acquiredAt"`
	AcquisitionActorDigest string `json:"acquisitionActorDigest"`
	ExpectedArtifactCount  uint64 `json:"expectedArtifactCount"`
	IntentNonceDigest      string `json:"intentNonceDigest"`
	IntentDigest           string `json:"intentDigest"`
}

// RawArtifactSourceLocatorV1 is the host-derived identity of one source
// occurrence within an acquisition intent. SourceFileIDDigest deliberately
// uses the same derivation as SourceRowLocatorV1 so an exact raw artifact and
// its parsed rows cannot silently disagree about file identity.
type RawArtifactSourceLocatorV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	BindingKeyDigest        string `json:"bindingKeyDigest"`
	AcquisitionIntentDigest string `json:"acquisitionIntentDigest"`
	SourceOccurrenceOrdinal uint64 `json:"sourceOccurrenceOrdinal"`
	SourceFileIDDigest      string `json:"sourceFileIdDigest"`
	LocatorDigest           string `json:"locatorDigest"`
}

// newRawArtifactAcquisitionIntentV1 remains package-private until the app
// admission service owns authenticated request derivation. It creates no
// snapshot or factual authority.
func newRawArtifactAcquisitionIntentV1(
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	acquiredAt time.Time,
	acquisitionActorDigest string,
	expectedArtifactCount uint64,
	intentNonceDigest string,
) (RawArtifactAcquisitionIntentV1, error) {
	if domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil || acquiredAt.IsZero() {
		return RawArtifactAcquisitionIntentV1{}, errors.New("raw artifact acquisition intent input is invalid")
	}
	intent := RawArtifactAcquisitionIntentV1{
		SchemaVersion:          RawArtifactAcquisitionIntentSchemaVersionV1,
		Purpose:                RawArtifactAcquisitionIntentPurposeV1,
		BindingKeyDigest:       binding.BindingKeyDigest,
		AcquisitionMethod:      RawArtifactAcquisitionMethodV1,
		AcquiredAt:             acquiredAt.UTC().Format(time.RFC3339Nano),
		AcquisitionActorDigest: strings.TrimSpace(acquisitionActorDigest),
		ExpectedArtifactCount:  expectedArtifactCount,
		IntentNonceDigest:      strings.TrimSpace(intentNonceDigest),
	}
	intent.IntentDigest = rawArtifactAcquisitionIntentDigestV1(intent)
	if err := ValidateRawArtifactAcquisitionIntentV1(intent); err != nil {
		return RawArtifactAcquisitionIntentV1{}, err
	}
	return intent, nil
}

func ValidateRawArtifactAcquisitionIntentV1(intent RawArtifactAcquisitionIntentV1) error {
	acquiredAt, acquiredAtErr := time.Parse(time.RFC3339Nano, intent.AcquiredAt)
	if intent.SchemaVersion != RawArtifactAcquisitionIntentSchemaVersionV1 ||
		intent.Purpose != RawArtifactAcquisitionIntentPurposeV1 ||
		!validSourceRowSHA256V1(intent.BindingKeyDigest) ||
		intent.AcquisitionMethod != RawArtifactAcquisitionMethodV1 ||
		acquiredAtErr != nil || acquiredAt.IsZero() || acquiredAt.UTC().Format(time.RFC3339Nano) != intent.AcquiredAt ||
		!validSourceRowSHA256V1(intent.AcquisitionActorDigest) ||
		intent.ExpectedArtifactCount == 0 || intent.ExpectedArtifactCount > maxRawArtifactCountV1 ||
		!validSourceRowSHA256V1(intent.IntentNonceDigest) ||
		!validSourceRowSHA256V1(intent.IntentDigest) ||
		intent.IntentDigest != rawArtifactAcquisitionIntentDigestV1(intent) {
		return errors.New("raw artifact acquisition intent is invalid")
	}
	return nil
}

func ParseRawArtifactAcquisitionIntentV1(raw []byte) (RawArtifactAcquisitionIntentV1, error) {
	var intent RawArtifactAcquisitionIntentV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &intent, maxRawArtifactAcquisitionIntentBytesV1, 2_048, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactAcquisitionIntentV1{}, err
	}
	return intent, ValidateRawArtifactAcquisitionIntentV1(intent)
}

func RawArtifactAcquisitionIntentV1Bytes(intent RawArtifactAcquisitionIntentV1) ([]byte, error) {
	if err := ValidateRawArtifactAcquisitionIntentV1(intent); err != nil {
		return nil, err
	}
	body, err := json.Marshal(intent)
	if err != nil || len(body) > maxRawArtifactAcquisitionIntentBytesV1 {
		return nil, errors.New("raw artifact acquisition intent exceeds its canonical byte limit")
	}
	return body, nil
}

// newRawArtifactSourceLocatorV1 remains package-private until a host-owned
// source occurrence registry is composed. sourceFileID is validated as the
// current funds producer's opaque host id and is never stored in the locator.
func newRawArtifactSourceLocatorV1(
	intent RawArtifactAcquisitionIntentV1,
	sourceOccurrenceOrdinal uint64,
	sourceFileID string,
) (RawArtifactSourceLocatorV1, error) {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil ||
		sourceOccurrenceOrdinal == 0 || sourceOccurrenceOrdinal > intent.ExpectedArtifactCount ||
		!validRawArtifactHostSourceFileIDV1(sourceFileID) {
		return RawArtifactSourceLocatorV1{}, errors.New("raw artifact source locator input is invalid")
	}
	locator := RawArtifactSourceLocatorV1{
		SchemaVersion:           RawArtifactSourceLocatorSchemaVersionV1,
		Purpose:                 RawArtifactSourceLocatorPurposeV1,
		BindingKeyDigest:        intent.BindingKeyDigest,
		AcquisitionIntentDigest: intent.IntentDigest,
		SourceOccurrenceOrdinal: sourceOccurrenceOrdinal,
		SourceFileIDDigest:      sourceRowSourceFileIDDigestV1(sourceFileID),
	}
	locator.LocatorDigest = rawArtifactSourceLocatorDigestV1(locator)
	if err := ValidateRawArtifactSourceLocatorForIntentV1(intent, locator); err != nil {
		return RawArtifactSourceLocatorV1{}, err
	}
	return locator, nil
}

func ValidateRawArtifactSourceLocatorV1(locator RawArtifactSourceLocatorV1) error {
	if locator.SchemaVersion != RawArtifactSourceLocatorSchemaVersionV1 ||
		locator.Purpose != RawArtifactSourceLocatorPurposeV1 ||
		!validSourceRowSHA256V1(locator.BindingKeyDigest) ||
		!validSourceRowSHA256V1(locator.AcquisitionIntentDigest) ||
		locator.SourceOccurrenceOrdinal == 0 || locator.SourceOccurrenceOrdinal > maxRawArtifactCountV1 ||
		!validSourceRowSHA256V1(locator.SourceFileIDDigest) ||
		!validSourceRowSHA256V1(locator.LocatorDigest) ||
		locator.LocatorDigest != rawArtifactSourceLocatorDigestV1(locator) {
		return errors.New("raw artifact source locator is invalid")
	}
	return nil
}

func ValidateRawArtifactSourceLocatorForIntentV1(
	intent RawArtifactAcquisitionIntentV1,
	locator RawArtifactSourceLocatorV1,
) error {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil || ValidateRawArtifactSourceLocatorV1(locator) != nil ||
		locator.BindingKeyDigest != intent.BindingKeyDigest ||
		locator.AcquisitionIntentDigest != intent.IntentDigest ||
		locator.SourceOccurrenceOrdinal > intent.ExpectedArtifactCount {
		return errors.New("raw artifact source locator does not match its acquisition intent")
	}
	return nil
}

func ParseRawArtifactSourceLocatorV1(raw []byte) (RawArtifactSourceLocatorV1, error) {
	var locator RawArtifactSourceLocatorV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &locator, maxRawArtifactSourceLocatorBytesV1, 2_048, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactSourceLocatorV1{}, err
	}
	return locator, ValidateRawArtifactSourceLocatorV1(locator)
}

func RawArtifactSourceLocatorV1Bytes(locator RawArtifactSourceLocatorV1) ([]byte, error) {
	if err := ValidateRawArtifactSourceLocatorV1(locator); err != nil {
		return nil, err
	}
	body, err := json.Marshal(locator)
	if err != nil || len(body) > maxRawArtifactSourceLocatorBytesV1 {
		return nil, errors.New("raw artifact source locator exceeds its canonical byte limit")
	}
	return body, nil
}

func rawArtifactAcquisitionIntentDigestV1(intent RawArtifactAcquisitionIntentV1) string {
	intent.IntentDigest = ""
	body, _ := json.Marshal(intent)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactAcquisitionIntentDigestDomainV1...), body...))
}

func rawArtifactSourceLocatorDigestV1(locator RawArtifactSourceLocatorV1) string {
	locator.LocatorDigest = ""
	body, _ := json.Marshal(locator)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactSourceLocatorDigestDomainV1...), body...))
}

func validRawArtifactHostSourceFileIDV1(value string) bool {
	if len(value) != 20 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') {
			return false
		}
	}
	return true
}
