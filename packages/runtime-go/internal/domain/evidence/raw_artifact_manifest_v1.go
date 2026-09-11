package evidence

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	RawArtifactEntrySchemaVersionV1                  = 1
	RawArtifactEntryPurposeV1                        = "analytix.raw-artifact-entry/v1"
	RawArtifactManifestPageSchemaVersionV1           = 1
	RawArtifactManifestPagePurposeV1                 = "analytix.raw-artifact-manifest-page/v1"
	RawArtifactManifestPageDescriptorSchemaVersionV1 = 1
	RawArtifactManifestPageDescriptorPurposeV1       = "analytix.raw-artifact-manifest-page-descriptor/v1"
	RawArtifactManifestSchemaVersionV1               = 1
	RawArtifactManifestPurposeV1                     = "analytix.raw-artifact-manifest/v1"

	RawArtifactIDPrefixV1             = "rart1_"
	RawArtifactSourceKindCaseImportV1 = "case_import_file"
	RawArtifactMediaTypeOpaqueV1      = "application/octet-stream"
	RawArtifactOriginDirectImportV1   = "direct_case_import"

	maxRawArtifactsPerManifestPageV1  = 256
	maxRawArtifactEntryBytesV1        = 64 * 1024
	maxRawArtifactManifestPageBytesV1 = 1024 * 1024
	maxRawArtifactManifestPagesV1     = 512
	maxRawArtifactManifestBytesV1     = 512 * 1024
	maxRawArtifactCountV1             = maxRawArtifactsPerManifestPageV1 * maxRawArtifactManifestPagesV1
)

var (
	rawArtifactIDDomainV1                       = []byte("analytix.raw-artifact/id/v1\x00")
	rawArtifactEntryDigestDomainV1              = []byte("analytix.raw-artifact-entry/digest/v1\x00")
	rawArtifactManifestPageDigestDomainV1       = []byte("analytix.raw-artifact-manifest-page/digest/v1\x00")
	rawArtifactManifestDescriptorDigestDomainV1 = []byte("analytix.raw-artifact-manifest-page-descriptor/digest/v1\x00")
	rawArtifactSetDigestDomainV1                = []byte("analytix.raw-artifact-manifest/set-digest/v1\x00")
	rawArtifactManifestDigestDomainV1           = []byte("analytix.raw-artifact-manifest/digest/v1\x00")
)

// RawArtifactEntryV1 contains no source filename, path, raw bytes, or account
// value. It binds a host-derived source occurrence to an exact chunked content
// root and full-file SHA-256.
type RawArtifactEntryV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	ArtifactOrdinal         uint64 `json:"artifactOrdinal"`
	ArtifactID              string `json:"artifactId"`
	BindingKeyDigest        string `json:"bindingKeyDigest"`
	AcquisitionIntentDigest string `json:"acquisitionIntentDigest"`
	SourceFileIDDigest      string `json:"sourceFileIdDigest"`
	SourceKind              string `json:"sourceKind"`
	MediaType               string `json:"mediaType"`
	SourceLocatorDigest     string `json:"sourceLocatorDigest"`
	SourceLocatorSHA256     string `json:"sourceLocatorSha256"`
	SourceLocatorByteLength uint64 `json:"sourceLocatorByteLength"`
	OriginKind              string `json:"originKind"`
	ContentRootDigest       string `json:"contentRootDigest"`
	ContentRootSHA256       string `json:"contentRootSha256"`
	ContentRootByteLength   uint64 `json:"contentRootByteLength"`
	FullSHA256              string `json:"fullSha256"`
	ArtifactByteLength      uint64 `json:"artifactByteLength"`
	ContentChunkCount       uint64 `json:"contentChunkCount"`
	EntryDigest             string `json:"entryDigest"`
}

type RawArtifactManifestPageV1 struct {
	SchemaVersion           int                  `json:"schemaVersion"`
	Purpose                 string               `json:"purpose"`
	BindingKeyDigest        string               `json:"bindingKeyDigest"`
	AcquisitionIntentDigest string               `json:"acquisitionIntentDigest"`
	ManifestPageNumber      uint64               `json:"manifestPageNumber"`
	Entries                 []RawArtifactEntryV1 `json:"entries"`
	ArtifactCount           uint32               `json:"artifactCount"`
	AggregateChunkCount     uint64               `json:"aggregateChunkCount"`
	AggregateRawBytes       uint64               `json:"aggregateRawBytes"`
	PageDigest              string               `json:"pageDigest"`
}

type RawArtifactManifestPageDescriptorV1 struct {
	SchemaVersion           int    `json:"schemaVersion"`
	Purpose                 string `json:"purpose"`
	BindingKeyDigest        string `json:"bindingKeyDigest"`
	AcquisitionIntentDigest string `json:"acquisitionIntentDigest"`
	ManifestPageNumber      uint64 `json:"manifestPageNumber"`
	PageDigest              string `json:"pageDigest"`
	PageSHA256              string `json:"pageSha256"`
	PageByteLength          uint64 `json:"pageByteLength"`
	FirstArtifactOrdinal    uint64 `json:"firstArtifactOrdinal"`
	LastArtifactOrdinal     uint64 `json:"lastArtifactOrdinal"`
	ArtifactCount           uint32 `json:"artifactCount"`
	FirstArtifactID         string `json:"firstArtifactId"`
	LastArtifactID          string `json:"lastArtifactId"`
	AggregateChunkCount     uint64 `json:"aggregateChunkCount"`
	AggregateRawBytes       uint64 `json:"aggregateRawBytes"`
	DescriptorDigest        string `json:"descriptorDigest"`
}

// RawArtifactManifestV1 is a bounded, restricted pseudonymous root. Stable
// hashes remain correlatable and stay private. Acquisition authority and exact
// chunk membership remain app-layer private-CAS admission obligations.
type RawArtifactManifestV1 struct {
	SchemaVersion               int                                   `json:"schemaVersion"`
	Purpose                     string                                `json:"purpose"`
	BindingKeyDigest            string                                `json:"bindingKeyDigest"`
	AcquisitionMethod           string                                `json:"acquisitionMethod"`
	AcquiredAt                  string                                `json:"acquiredAt"`
	AcquisitionActorDigest      string                                `json:"acquisitionActorDigest"`
	AcquisitionIntentDigest     string                                `json:"acquisitionIntentDigest"`
	AcquisitionIntentSHA256     string                                `json:"acquisitionIntentSha256"`
	AcquisitionIntentByteLength uint64                                `json:"acquisitionIntentByteLength"`
	PageDescriptors             []RawArtifactManifestPageDescriptorV1 `json:"pageDescriptors"`
	PageCount                   uint32                                `json:"pageCount"`
	ArtifactCount               uint64                                `json:"artifactCount"`
	AggregateChunkCount         uint64                                `json:"aggregateChunkCount"`
	AggregateRawBytes           uint64                                `json:"aggregateRawBytes"`
	ArtifactSetDigest           string                                `json:"artifactSetDigest"`
	ManifestDigest              string                                `json:"manifestDigest"`
}

func newRawArtifactEntryV1(
	intent RawArtifactAcquisitionIntentV1,
	locator RawArtifactSourceLocatorV1,
	contentRoot RawArtifactContentRootV1,
) (RawArtifactEntryV1, error) {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil ||
		ValidateRawArtifactSourceLocatorForIntentV1(intent, locator) != nil ||
		ValidateRawArtifactContentRootV1(contentRoot) != nil {
		return RawArtifactEntryV1{}, errors.New("raw artifact entry content root is invalid")
	}
	locatorBody, err := RawArtifactSourceLocatorV1Bytes(locator)
	if err != nil {
		return RawArtifactEntryV1{}, err
	}
	rootBody, err := RawArtifactContentRootV1Bytes(contentRoot)
	if err != nil {
		return RawArtifactEntryV1{}, err
	}
	entry := RawArtifactEntryV1{
		SchemaVersion: RawArtifactEntrySchemaVersionV1, Purpose: RawArtifactEntryPurposeV1,
		ArtifactOrdinal: locator.SourceOccurrenceOrdinal, BindingKeyDigest: locator.BindingKeyDigest,
		AcquisitionIntentDigest: locator.AcquisitionIntentDigest, SourceFileIDDigest: locator.SourceFileIDDigest,
		SourceKind: RawArtifactSourceKindCaseImportV1, MediaType: RawArtifactMediaTypeOpaqueV1,
		SourceLocatorDigest: locator.LocatorDigest, SourceLocatorSHA256: domainsecurity.SHA256Hex(locatorBody),
		SourceLocatorByteLength: uint64(len(locatorBody)), OriginKind: RawArtifactOriginDirectImportV1,
		ContentRootDigest: contentRoot.RootDigest,
		ContentRootSHA256: domainsecurity.SHA256Hex(rootBody), ContentRootByteLength: uint64(len(rootBody)),
		FullSHA256: contentRoot.FullSHA256, ArtifactByteLength: contentRoot.ArtifactByteLength,
		ContentChunkCount: contentRoot.ChunkCount,
	}
	entry.ArtifactID = deriveRawArtifactIDV1(entry)
	entry.EntryDigest = rawArtifactEntryDigestV1(entry)
	if err := ValidateRawArtifactEntryAgainstSourceLocatorV1(intent, entry, locator); err != nil {
		return RawArtifactEntryV1{}, err
	}
	if err := ValidateRawArtifactEntryWithContentRootV1(entry, contentRoot); err != nil {
		return RawArtifactEntryV1{}, err
	}
	return entry, nil
}

func ValidateRawArtifactEntryV1(entry RawArtifactEntryV1) error {
	if entry.SchemaVersion != RawArtifactEntrySchemaVersionV1 || entry.Purpose != RawArtifactEntryPurposeV1 ||
		entry.ArtifactOrdinal == 0 || entry.ArtifactOrdinal > maxRawArtifactCountV1 || !validRawArtifactIDV1(entry.ArtifactID) ||
		!validSourceRowSHA256V1(entry.BindingKeyDigest) || !validSourceRowSHA256V1(entry.SourceFileIDDigest) ||
		!validSourceRowSHA256V1(entry.AcquisitionIntentDigest) ||
		entry.SourceKind != RawArtifactSourceKindCaseImportV1 || entry.MediaType != RawArtifactMediaTypeOpaqueV1 ||
		!validSourceRowSHA256V1(entry.SourceLocatorDigest) || entry.OriginKind != RawArtifactOriginDirectImportV1 ||
		!validSourceRowSHA256V1(entry.SourceLocatorSHA256) || entry.SourceLocatorByteLength == 0 ||
		entry.SourceLocatorByteLength > maxRawArtifactSourceLocatorBytesV1 ||
		!validSourceRowSHA256V1(entry.ContentRootDigest) || !validSourceRowSHA256V1(entry.ContentRootSHA256) ||
		entry.ContentRootByteLength == 0 || entry.ContentRootByteLength > maxRawArtifactContentRootBytesV1 ||
		!validSourceRowSHA256V1(entry.FullSHA256) || entry.ArtifactByteLength == 0 ||
		entry.ArtifactByteLength > maxRawArtifactContentBytesV1 || entry.ContentChunkCount == 0 ||
		entry.ContentChunkCount > maxRawArtifactChunkCountV1 || !validSourceRowSHA256V1(entry.EntryDigest) ||
		entry.ArtifactID != deriveRawArtifactIDV1(entry) || entry.EntryDigest != rawArtifactEntryDigestV1(entry) {
		return errors.New("raw artifact entry is invalid")
	}
	return nil
}

func ValidateRawArtifactEntryAgainstSourceLocatorV1(
	intent RawArtifactAcquisitionIntentV1,
	entry RawArtifactEntryV1,
	locator RawArtifactSourceLocatorV1,
) error {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil || ValidateRawArtifactEntryV1(entry) != nil ||
		ValidateRawArtifactSourceLocatorForIntentV1(intent, locator) != nil {
		return errors.New("raw artifact entry source locator material is invalid")
	}
	body, err := RawArtifactSourceLocatorV1Bytes(locator)
	if err != nil || entry.ArtifactOrdinal != locator.SourceOccurrenceOrdinal ||
		entry.BindingKeyDigest != locator.BindingKeyDigest ||
		entry.AcquisitionIntentDigest != locator.AcquisitionIntentDigest ||
		entry.SourceFileIDDigest != locator.SourceFileIDDigest || entry.SourceLocatorDigest != locator.LocatorDigest ||
		entry.SourceLocatorSHA256 != domainsecurity.SHA256Hex(body) || entry.SourceLocatorByteLength != uint64(len(body)) {
		return errors.New("raw artifact entry does not match its exact source locator")
	}
	return nil
}

func ValidateRawArtifactEntryWithContentRootV1(entry RawArtifactEntryV1, root RawArtifactContentRootV1) error {
	if err := ValidateRawArtifactEntryV1(entry); err != nil {
		return err
	}
	if err := ValidateRawArtifactContentRootV1(root); err != nil {
		return err
	}
	body, err := RawArtifactContentRootV1Bytes(root)
	if err != nil || entry.BindingKeyDigest != root.BindingKeyDigest || entry.SourceLocatorDigest != root.SourceLocatorDigest ||
		entry.ContentRootDigest != root.RootDigest || entry.ContentRootSHA256 != domainsecurity.SHA256Hex(body) ||
		entry.ContentRootByteLength != uint64(len(body)) || entry.FullSHA256 != root.FullSHA256 ||
		entry.ArtifactByteLength != root.ArtifactByteLength || entry.ContentChunkCount != root.ChunkCount {
		return errors.New("raw artifact entry does not match its exact content root")
	}
	return nil
}

func ParseRawArtifactEntryV1(raw []byte) (RawArtifactEntryV1, error) {
	var entry RawArtifactEntryV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &entry, maxRawArtifactEntryBytesV1, 2_048, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactEntryV1{}, err
	}
	return entry, ValidateRawArtifactEntryV1(entry)
}

func RawArtifactEntryV1Bytes(entry RawArtifactEntryV1) ([]byte, error) {
	if err := ValidateRawArtifactEntryV1(entry); err != nil {
		return nil, err
	}
	body, err := json.Marshal(entry)
	if err != nil || len(body) > maxRawArtifactEntryBytesV1 {
		return nil, errors.New("raw artifact entry exceeds its canonical byte limit")
	}
	return body, nil
}

func newRawArtifactManifestPageV1(
	intent RawArtifactAcquisitionIntentV1,
	pageNumber uint64,
	entries []RawArtifactEntryV1,
) (RawArtifactManifestPageV1, error) {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil || len(entries) == 0 ||
		len(entries) > maxRawArtifactsPerManifestPageV1 {
		return RawArtifactManifestPageV1{}, errors.New("raw artifact manifest page input is invalid")
	}
	page := RawArtifactManifestPageV1{
		SchemaVersion: RawArtifactManifestPageSchemaVersionV1, Purpose: RawArtifactManifestPagePurposeV1,
		BindingKeyDigest: intent.BindingKeyDigest, AcquisitionIntentDigest: intent.IntentDigest,
		ManifestPageNumber: pageNumber,
		Entries:            append([]RawArtifactEntryV1(nil), entries...), ArtifactCount: uint32(len(entries)),
	}
	for _, entry := range page.Entries {
		if ValidateRawArtifactEntryV1(entry) != nil ||
			page.AggregateChunkCount > maxRawArtifactChunkCountV1-entry.ContentChunkCount ||
			page.AggregateRawBytes > maxRawArtifactContentBytesV1-entry.ArtifactByteLength {
			return RawArtifactManifestPageV1{}, errors.New("raw artifact manifest page entry is invalid")
		}
		page.AggregateChunkCount += entry.ContentChunkCount
		page.AggregateRawBytes += entry.ArtifactByteLength
	}
	page.PageDigest = rawArtifactManifestPageDigestV1(page)
	if err := ValidateRawArtifactManifestPageV1(page); err != nil {
		return RawArtifactManifestPageV1{}, err
	}
	return page, nil
}

func ValidateRawArtifactManifestPageV1(page RawArtifactManifestPageV1) error {
	if page.SchemaVersion != RawArtifactManifestPageSchemaVersionV1 || page.Purpose != RawArtifactManifestPagePurposeV1 ||
		!validSourceRowSHA256V1(page.BindingKeyDigest) || !validSourceRowSHA256V1(page.AcquisitionIntentDigest) ||
		page.ManifestPageNumber == 0 ||
		page.ManifestPageNumber > maxRawArtifactManifestPagesV1 || len(page.Entries) == 0 ||
		len(page.Entries) > maxRawArtifactsPerManifestPageV1 || page.ArtifactCount != uint32(len(page.Entries)) ||
		page.AggregateChunkCount == 0 || page.AggregateChunkCount > maxRawArtifactChunkCountV1 ||
		page.AggregateRawBytes == 0 || page.AggregateRawBytes > maxRawArtifactContentBytesV1 ||
		!validSourceRowSHA256V1(page.PageDigest) {
		return errors.New("raw artifact manifest page is invalid")
	}
	var chunks uint64
	var rawBytes uint64
	seenArtifactIDs := make(map[string]struct{}, len(page.Entries))
	seenLocators := make(map[string]struct{}, len(page.Entries))
	for index, entry := range page.Entries {
		if ValidateRawArtifactEntryV1(entry) != nil || entry.BindingKeyDigest != page.BindingKeyDigest ||
			entry.AcquisitionIntentDigest != page.AcquisitionIntentDigest ||
			entry.ArtifactOrdinal != page.Entries[0].ArtifactOrdinal+uint64(index) ||
			chunks > maxRawArtifactChunkCountV1-entry.ContentChunkCount ||
			rawBytes > maxRawArtifactContentBytesV1-entry.ArtifactByteLength {
			return errors.New("raw artifact manifest page entries are not canonical")
		}
		if _, exists := seenArtifactIDs[entry.ArtifactID]; exists {
			return errors.New("raw artifact manifest page contains a duplicate artifact id")
		}
		if _, exists := seenLocators[entry.SourceLocatorDigest]; exists {
			return errors.New("raw artifact manifest page contains a duplicate source locator")
		}
		seenArtifactIDs[entry.ArtifactID] = struct{}{}
		seenLocators[entry.SourceLocatorDigest] = struct{}{}
		chunks += entry.ContentChunkCount
		rawBytes += entry.ArtifactByteLength
	}
	body, err := json.Marshal(page)
	if err != nil || len(body) > maxRawArtifactManifestPageBytesV1 || chunks != page.AggregateChunkCount ||
		rawBytes != page.AggregateRawBytes || page.PageDigest != rawArtifactManifestPageDigestV1(page) {
		return errors.New("raw artifact manifest page content is invalid")
	}
	return nil
}

func newRawArtifactManifestPageDescriptorV1(
	page RawArtifactManifestPageV1,
) (RawArtifactManifestPageDescriptorV1, error) {
	if err := ValidateRawArtifactManifestPageV1(page); err != nil {
		return RawArtifactManifestPageDescriptorV1{}, err
	}
	body, _ := json.Marshal(page)
	first := page.Entries[0]
	last := page.Entries[len(page.Entries)-1]
	descriptor := RawArtifactManifestPageDescriptorV1{
		SchemaVersion:           RawArtifactManifestPageDescriptorSchemaVersionV1,
		Purpose:                 RawArtifactManifestPageDescriptorPurposeV1,
		BindingKeyDigest:        page.BindingKeyDigest,
		AcquisitionIntentDigest: page.AcquisitionIntentDigest,
		ManifestPageNumber:      page.ManifestPageNumber, PageDigest: page.PageDigest,
		PageSHA256: domainsecurity.SHA256Hex(body), PageByteLength: uint64(len(body)),
		FirstArtifactOrdinal: first.ArtifactOrdinal, LastArtifactOrdinal: last.ArtifactOrdinal,
		ArtifactCount: page.ArtifactCount, FirstArtifactID: first.ArtifactID, LastArtifactID: last.ArtifactID,
		AggregateChunkCount: page.AggregateChunkCount, AggregateRawBytes: page.AggregateRawBytes,
	}
	descriptor.DescriptorDigest = rawArtifactManifestPageDescriptorDigestV1(descriptor)
	if err := ValidateRawArtifactManifestPageDescriptorV1(descriptor); err != nil {
		return RawArtifactManifestPageDescriptorV1{}, err
	}
	return descriptor, nil
}

func ValidateRawArtifactManifestPageDescriptorV1(descriptor RawArtifactManifestPageDescriptorV1) error {
	if descriptor.SchemaVersion != RawArtifactManifestPageDescriptorSchemaVersionV1 ||
		descriptor.Purpose != RawArtifactManifestPageDescriptorPurposeV1 ||
		!validSourceRowSHA256V1(descriptor.BindingKeyDigest) ||
		!validSourceRowSHA256V1(descriptor.AcquisitionIntentDigest) || descriptor.ManifestPageNumber == 0 ||
		descriptor.ManifestPageNumber > maxRawArtifactManifestPagesV1 || !validSourceRowSHA256V1(descriptor.PageDigest) ||
		!validSourceRowSHA256V1(descriptor.PageSHA256) || descriptor.PageByteLength == 0 ||
		descriptor.PageByteLength > maxRawArtifactManifestPageBytesV1 || descriptor.FirstArtifactOrdinal == 0 ||
		descriptor.LastArtifactOrdinal < descriptor.FirstArtifactOrdinal || descriptor.ArtifactCount == 0 ||
		descriptor.ArtifactCount > maxRawArtifactsPerManifestPageV1 ||
		descriptor.LastArtifactOrdinal-descriptor.FirstArtifactOrdinal+1 != uint64(descriptor.ArtifactCount) ||
		!validRawArtifactIDV1(descriptor.FirstArtifactID) || !validRawArtifactIDV1(descriptor.LastArtifactID) ||
		descriptor.AggregateChunkCount == 0 || descriptor.AggregateChunkCount > maxRawArtifactChunkCountV1 ||
		descriptor.AggregateRawBytes == 0 || descriptor.AggregateRawBytes > maxRawArtifactContentBytesV1 ||
		!validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != rawArtifactManifestPageDescriptorDigestV1(descriptor) {
		return errors.New("raw artifact manifest page descriptor is invalid")
	}
	if descriptor.ArtifactCount == 1 && descriptor.FirstArtifactID != descriptor.LastArtifactID {
		return errors.New("single raw artifact manifest descriptor has inconsistent ids")
	}
	return nil
}

// ValidateRawArtifactManifestPageAgainstDescriptorV1 compares one exact
// canonical manifest page with its parent descriptor.
func ValidateRawArtifactManifestPageAgainstDescriptorV1(
	descriptor RawArtifactManifestPageDescriptorV1,
	page RawArtifactManifestPageV1,
) error {
	if ValidateRawArtifactManifestPageDescriptorV1(descriptor) != nil || ValidateRawArtifactManifestPageV1(page) != nil {
		return errors.New("raw artifact manifest page descriptor material is invalid")
	}
	derived, err := newRawArtifactManifestPageDescriptorV1(page)
	if err != nil || derived != descriptor {
		return errors.New("raw artifact manifest page does not match its descriptor")
	}
	return nil
}

func newRawArtifactManifestV1(
	intent RawArtifactAcquisitionIntentV1,
	descriptors []RawArtifactManifestPageDescriptorV1,
) (RawArtifactManifestV1, error) {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil || len(descriptors) == 0 ||
		len(descriptors) > maxRawArtifactManifestPagesV1 {
		return RawArtifactManifestV1{}, errors.New("raw artifact manifest input is invalid")
	}
	intentBody, err := RawArtifactAcquisitionIntentV1Bytes(intent)
	if err != nil {
		return RawArtifactManifestV1{}, err
	}
	manifest := RawArtifactManifestV1{
		SchemaVersion: RawArtifactManifestSchemaVersionV1, Purpose: RawArtifactManifestPurposeV1,
		BindingKeyDigest: intent.BindingKeyDigest, AcquisitionMethod: intent.AcquisitionMethod,
		AcquiredAt: intent.AcquiredAt, AcquisitionActorDigest: intent.AcquisitionActorDigest,
		AcquisitionIntentDigest: intent.IntentDigest, AcquisitionIntentSHA256: domainsecurity.SHA256Hex(intentBody),
		AcquisitionIntentByteLength: uint64(len(intentBody)),
		PageDescriptors:             append([]RawArtifactManifestPageDescriptorV1(nil), descriptors...), PageCount: uint32(len(descriptors)),
	}
	for _, descriptor := range manifest.PageDescriptors {
		if ValidateRawArtifactManifestPageDescriptorV1(descriptor) != nil ||
			descriptor.BindingKeyDigest != manifest.BindingKeyDigest ||
			descriptor.AcquisitionIntentDigest != manifest.AcquisitionIntentDigest ||
			manifest.ArtifactCount > maxRawArtifactCountV1-uint64(descriptor.ArtifactCount) ||
			manifest.AggregateChunkCount > maxRawArtifactChunkCountV1-descriptor.AggregateChunkCount ||
			manifest.AggregateRawBytes > maxRawArtifactContentBytesV1-descriptor.AggregateRawBytes {
			return RawArtifactManifestV1{}, errors.New("raw artifact manifest descriptor is invalid")
		}
		manifest.ArtifactCount += uint64(descriptor.ArtifactCount)
		manifest.AggregateChunkCount += descriptor.AggregateChunkCount
		manifest.AggregateRawBytes += descriptor.AggregateRawBytes
	}
	if manifest.ArtifactCount != intent.ExpectedArtifactCount {
		return RawArtifactManifestV1{}, errors.New("raw artifact manifest does not consume its acquisition intent")
	}
	manifest.ArtifactSetDigest = rawArtifactSetDigestV1(manifest)
	manifest.ManifestDigest = rawArtifactManifestDigestV1(manifest)
	if err := ValidateRawArtifactManifestForIntentV1(intent, manifest); err != nil {
		return RawArtifactManifestV1{}, err
	}
	return manifest, nil
}

// ValidateRawArtifactManifestForIntentV1 proves that the manifest consumes
// the exact pre-content intent. A later commit receipt may reference both
// objects; neither object may reference that later receipt.
func ValidateRawArtifactManifestForIntentV1(
	intent RawArtifactAcquisitionIntentV1,
	manifest RawArtifactManifestV1,
) error {
	if ValidateRawArtifactAcquisitionIntentV1(intent) != nil || ValidateRawArtifactManifestV1(manifest) != nil {
		return errors.New("raw artifact manifest intent material is invalid")
	}
	body, err := RawArtifactAcquisitionIntentV1Bytes(intent)
	if err != nil || manifest.BindingKeyDigest != intent.BindingKeyDigest ||
		manifest.AcquisitionMethod != intent.AcquisitionMethod || manifest.AcquiredAt != intent.AcquiredAt ||
		manifest.AcquisitionActorDigest != intent.AcquisitionActorDigest ||
		manifest.AcquisitionIntentDigest != intent.IntentDigest ||
		manifest.AcquisitionIntentSHA256 != domainsecurity.SHA256Hex(body) ||
		manifest.AcquisitionIntentByteLength != uint64(len(body)) ||
		manifest.ArtifactCount != intent.ExpectedArtifactCount {
		return errors.New("raw artifact manifest does not match its exact acquisition intent")
	}
	return nil
}

func ValidateRawArtifactManifestV1(manifest RawArtifactManifestV1) error {
	acquiredAt, acquiredAtErr := time.Parse(time.RFC3339Nano, manifest.AcquiredAt)
	if manifest.SchemaVersion != RawArtifactManifestSchemaVersionV1 || manifest.Purpose != RawArtifactManifestPurposeV1 ||
		!validSourceRowSHA256V1(manifest.BindingKeyDigest) || manifest.AcquisitionMethod != RawArtifactAcquisitionMethodV1 ||
		acquiredAtErr != nil || acquiredAt.IsZero() || acquiredAt.UTC().Format(time.RFC3339Nano) != manifest.AcquiredAt ||
		!validSourceRowSHA256V1(manifest.AcquisitionActorDigest) ||
		!validSourceRowSHA256V1(manifest.AcquisitionIntentDigest) ||
		!validSourceRowSHA256V1(manifest.AcquisitionIntentSHA256) || manifest.AcquisitionIntentByteLength == 0 ||
		manifest.AcquisitionIntentByteLength > maxRawArtifactAcquisitionIntentBytesV1 ||
		len(manifest.PageDescriptors) == 0 || len(manifest.PageDescriptors) > maxRawArtifactManifestPagesV1 ||
		manifest.PageCount != uint32(len(manifest.PageDescriptors)) || manifest.ArtifactCount == 0 ||
		manifest.ArtifactCount > maxRawArtifactCountV1 || manifest.AggregateChunkCount == 0 ||
		manifest.AggregateChunkCount > maxRawArtifactChunkCountV1 || manifest.AggregateRawBytes == 0 ||
		manifest.AggregateRawBytes > maxRawArtifactContentBytesV1 || !validSourceRowSHA256V1(manifest.ArtifactSetDigest) ||
		!validSourceRowSHA256V1(manifest.ManifestDigest) {
		return errors.New("raw artifact manifest is invalid")
	}
	var artifacts uint64
	var chunks uint64
	var rawBytes uint64
	seenPageDigests := make(map[string]struct{}, len(manifest.PageDescriptors))
	seenPageSHA256 := make(map[string]struct{}, len(manifest.PageDescriptors))
	for index, descriptor := range manifest.PageDescriptors {
		if ValidateRawArtifactManifestPageDescriptorV1(descriptor) != nil || descriptor.ManifestPageNumber != uint64(index+1) ||
			descriptor.BindingKeyDigest != manifest.BindingKeyDigest ||
			descriptor.AcquisitionIntentDigest != manifest.AcquisitionIntentDigest ||
			artifacts > maxRawArtifactCountV1-uint64(descriptor.ArtifactCount) ||
			chunks > maxRawArtifactChunkCountV1-descriptor.AggregateChunkCount ||
			rawBytes > maxRawArtifactContentBytesV1-descriptor.AggregateRawBytes {
			return errors.New("raw artifact manifest page descriptors are not canonical")
		}
		if index < len(manifest.PageDescriptors)-1 && descriptor.ArtifactCount != maxRawArtifactsPerManifestPageV1 {
			return errors.New("raw artifact manifest contains a partial non-terminal page")
		}
		if _, exists := seenPageDigests[descriptor.PageDigest]; exists {
			return errors.New("raw artifact manifest reuses a page digest")
		}
		if _, exists := seenPageSHA256[descriptor.PageSHA256]; exists {
			return errors.New("raw artifact manifest reuses a page content address")
		}
		if index == 0 {
			if descriptor.FirstArtifactOrdinal != 1 {
				return errors.New("raw artifact manifest does not begin at the first artifact")
			}
		} else {
			previous := manifest.PageDescriptors[index-1]
			if descriptor.FirstArtifactOrdinal != previous.LastArtifactOrdinal+1 {
				return errors.New("raw artifact manifest pages are not contiguous")
			}
		}
		seenPageDigests[descriptor.PageDigest] = struct{}{}
		seenPageSHA256[descriptor.PageSHA256] = struct{}{}
		artifacts += uint64(descriptor.ArtifactCount)
		chunks += descriptor.AggregateChunkCount
		rawBytes += descriptor.AggregateRawBytes
	}
	body, err := json.Marshal(manifest)
	if err != nil || len(body) > maxRawArtifactManifestBytesV1 || artifacts != manifest.ArtifactCount ||
		chunks != manifest.AggregateChunkCount || rawBytes != manifest.AggregateRawBytes ||
		manifest.ArtifactSetDigest != rawArtifactSetDigestV1(manifest) ||
		manifest.ManifestDigest != rawArtifactManifestDigestV1(manifest) {
		return errors.New("raw artifact manifest aggregate is invalid")
	}
	return nil
}

// ValidateRawArtifactManifestEntryMembershipV1 proves exact structural
// membership through manifest -> page descriptor -> page -> entry. It does not
// prove that any object was persisted or selected by current snapshot
// authority; app admission must exact-read these canonical bytes from private
// CAS in one callback.
func ValidateRawArtifactManifestEntryMembershipV1(
	manifest RawArtifactManifestV1,
	page RawArtifactManifestPageV1,
	entry RawArtifactEntryV1,
) error {
	if ValidateRawArtifactManifestPageMembershipV1(manifest, page) != nil || ValidateRawArtifactEntryV1(entry) != nil ||
		entry.BindingKeyDigest != manifest.BindingKeyDigest || entry.AcquisitionIntentDigest != manifest.AcquisitionIntentDigest {
		return errors.New("raw artifact manifest membership material is invalid")
	}
	descriptor := manifest.PageDescriptors[page.ManifestPageNumber-1]
	if entry.ArtifactOrdinal < descriptor.FirstArtifactOrdinal || entry.ArtifactOrdinal > descriptor.LastArtifactOrdinal {
		return errors.New("raw artifact entry ordinal is outside its manifest page")
	}
	offset := entry.ArtifactOrdinal - descriptor.FirstArtifactOrdinal
	if offset >= uint64(len(page.Entries)) || page.Entries[offset] != entry {
		return errors.New("raw artifact entry is absent from its exact manifest page")
	}
	return nil
}

// ValidateRawArtifactManifestPageMembershipV1 proves exact structural
// manifest -> page membership without selecting a page from storage.
func ValidateRawArtifactManifestPageMembershipV1(
	manifest RawArtifactManifestV1,
	page RawArtifactManifestPageV1,
) error {
	if ValidateRawArtifactManifestV1(manifest) != nil || ValidateRawArtifactManifestPageV1(page) != nil ||
		page.BindingKeyDigest != manifest.BindingKeyDigest ||
		page.AcquisitionIntentDigest != manifest.AcquisitionIntentDigest ||
		page.ManifestPageNumber > uint64(len(manifest.PageDescriptors)) {
		return errors.New("raw artifact manifest page membership material is invalid")
	}
	descriptor := manifest.PageDescriptors[page.ManifestPageNumber-1]
	if ValidateRawArtifactManifestPageAgainstDescriptorV1(descriptor, page) != nil {
		return errors.New("raw artifact manifest page descriptor mismatch")
	}
	return nil
}

// ValidateRawArtifactManifestHierarchyV1 exact-compares every manifest page
// and enforces global source uniqueness. Structural root validation alone
// deliberately cannot prove cross-page uniqueness because locators are kept
// in private pages rather than duplicated into the bounded root.
func ValidateRawArtifactManifestHierarchyV1(
	intent RawArtifactAcquisitionIntentV1,
	manifest RawArtifactManifestV1,
	pages []RawArtifactManifestPageV1,
) error {
	if ValidateRawArtifactManifestForIntentV1(intent, manifest) != nil ||
		len(pages) != len(manifest.PageDescriptors) {
		return errors.New("raw artifact manifest hierarchy material is invalid")
	}
	seenArtifactIDs := make(map[string]struct{}, manifest.ArtifactCount)
	seenLocators := make(map[string]struct{}, manifest.ArtifactCount)
	seenSourceFiles := make(map[string]struct{}, manifest.ArtifactCount)
	var artifactCount uint64
	for index, page := range pages {
		if ValidateRawArtifactManifestPageMembershipV1(manifest, page) != nil || page.ManifestPageNumber != uint64(index+1) {
			return errors.New("raw artifact manifest hierarchy page is invalid")
		}
		for _, entry := range page.Entries {
			artifactCount++
			if entry.ArtifactOrdinal != artifactCount {
				return errors.New("raw artifact manifest hierarchy artifact order is invalid")
			}
			if _, exists := seenArtifactIDs[entry.ArtifactID]; exists {
				return errors.New("raw artifact manifest hierarchy contains a duplicate artifact id")
			}
			if _, exists := seenLocators[entry.SourceLocatorDigest]; exists {
				return errors.New("raw artifact manifest hierarchy contains a duplicate source locator")
			}
			if _, exists := seenSourceFiles[entry.SourceFileIDDigest]; exists {
				return errors.New("raw artifact manifest hierarchy contains a duplicate source file id")
			}
			seenArtifactIDs[entry.ArtifactID] = struct{}{}
			seenLocators[entry.SourceLocatorDigest] = struct{}{}
			seenSourceFiles[entry.SourceFileIDDigest] = struct{}{}
		}
	}
	if artifactCount != manifest.ArtifactCount || artifactCount != intent.ExpectedArtifactCount {
		return errors.New("raw artifact manifest hierarchy is incomplete")
	}
	return nil
}

func ParseRawArtifactManifestPageV1(raw []byte) (RawArtifactManifestPageV1, error) {
	var page RawArtifactManifestPageV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &page, maxRawArtifactManifestPageBytesV1, 250_000, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactManifestPageV1{}, err
	}
	return page, ValidateRawArtifactManifestPageV1(page)
}

func RawArtifactManifestPageV1Bytes(page RawArtifactManifestPageV1) ([]byte, error) {
	if err := ValidateRawArtifactManifestPageV1(page); err != nil {
		return nil, err
	}
	return json.Marshal(page)
}

func ParseRawArtifactManifestV1(raw []byte) (RawArtifactManifestV1, error) {
	var manifest RawArtifactManifestV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &manifest, maxRawArtifactManifestBytesV1, 100_000, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactManifestV1{}, err
	}
	return manifest, ValidateRawArtifactManifestV1(manifest)
}

func RawArtifactManifestV1Bytes(manifest RawArtifactManifestV1) ([]byte, error) {
	if err := ValidateRawArtifactManifestV1(manifest); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func deriveRawArtifactIDV1(entry RawArtifactEntryV1) string {
	identity := struct {
		BindingKeyDigest        string `json:"bindingKeyDigest"`
		AcquisitionIntentDigest string `json:"acquisitionIntentDigest"`
		SourceFileIDDigest      string `json:"sourceFileIdDigest"`
		SourceKind              string `json:"sourceKind"`
		SourceLocatorDigest     string `json:"sourceLocatorDigest"`
		FullSHA256              string `json:"fullSha256"`
		ArtifactByteLength      uint64 `json:"artifactByteLength"`
	}{entry.BindingKeyDigest, entry.AcquisitionIntentDigest, entry.SourceFileIDDigest, entry.SourceKind, entry.SourceLocatorDigest,
		entry.FullSHA256, entry.ArtifactByteLength}
	body, _ := json.Marshal(identity)
	return RawArtifactIDPrefixV1 + domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactIDDomainV1...), body...))
}

func validRawArtifactIDV1(value string) bool {
	return value == strings.TrimSpace(value) && strings.HasPrefix(value, RawArtifactIDPrefixV1) &&
		domainsecurity.IsSHA256Hex(strings.TrimPrefix(value, RawArtifactIDPrefixV1))
}

func rawArtifactEntryDigestV1(entry RawArtifactEntryV1) string {
	entry.EntryDigest = ""
	body, _ := json.Marshal(entry)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactEntryDigestDomainV1...), body...))
}

func rawArtifactManifestPageDigestV1(page RawArtifactManifestPageV1) string {
	page.PageDigest = ""
	body, _ := json.Marshal(page)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactManifestPageDigestDomainV1...), body...))
}

func rawArtifactManifestPageDescriptorDigestV1(descriptor RawArtifactManifestPageDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactManifestDescriptorDigestDomainV1...), body...))
}

func rawArtifactSetDigestV1(manifest RawArtifactManifestV1) string {
	identity := struct {
		BindingKeyDigest    string                                `json:"bindingKeyDigest"`
		PageDescriptors     []RawArtifactManifestPageDescriptorV1 `json:"pageDescriptors"`
		ArtifactCount       uint64                                `json:"artifactCount"`
		AggregateChunkCount uint64                                `json:"aggregateChunkCount"`
		AggregateRawBytes   uint64                                `json:"aggregateRawBytes"`
	}{manifest.BindingKeyDigest, manifest.PageDescriptors, manifest.ArtifactCount,
		manifest.AggregateChunkCount, manifest.AggregateRawBytes}
	body, _ := json.Marshal(identity)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactSetDigestDomainV1...), body...))
}

func rawArtifactManifestDigestV1(manifest RawArtifactManifestV1) string {
	manifest.ManifestDigest = ""
	body, _ := json.Marshal(manifest)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactManifestDigestDomainV1...), body...))
}
