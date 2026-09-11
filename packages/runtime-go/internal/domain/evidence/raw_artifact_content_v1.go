package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	RawArtifactContentChunkDescriptorSchemaVersionV1 = 1
	RawArtifactContentChunkDescriptorPurposeV1       = "analytix.raw-artifact-content-chunk-descriptor/v1"
	RawArtifactContentIndexPageSchemaVersionV1       = 1
	RawArtifactContentIndexPagePurposeV1             = "analytix.raw-artifact-content-index-page/v1"
	RawArtifactContentIndexDescriptorSchemaVersionV1 = 1
	RawArtifactContentIndexDescriptorPurposeV1       = "analytix.raw-artifact-content-index-page-descriptor/v1"
	RawArtifactContentRootSchemaVersionV1            = 1
	RawArtifactContentRootPurposeV1                  = "analytix.raw-artifact-content-root/v1"

	RawArtifactContentChunkBytesV1           = uint64(8 * 1024 * 1024)
	RawArtifactMaximumChunkCountV1           = uint64(256 * 512)
	maxRawArtifactChunksPerIndexPageV1       = 256
	maxRawArtifactContentIndexPageBytesV1    = 512 * 1024
	maxRawArtifactContentRootIndexPagesV1    = 512
	maxRawArtifactContentRootBytesV1         = 512 * 1024
	maxRawArtifactContentBytesV1             = uint64(1024 * 1024 * 1024 * 1024)
	maxRawArtifactChunkCountV1               = RawArtifactMaximumChunkCountV1
	maxRawArtifactContentIndexParserTokensV1 = 100_000
)

var (
	rawArtifactContentChunkDescriptorDigestDomainV1 = []byte("analytix.raw-artifact-content-chunk-descriptor/digest/v1\x00")
	rawArtifactContentIndexPageDigestDomainV1       = []byte("analytix.raw-artifact-content-index-page/digest/v1\x00")
	rawArtifactContentIndexDescriptorDigestDomainV1 = []byte("analytix.raw-artifact-content-index-page-descriptor/digest/v1\x00")
	rawArtifactContentRootDigestDomainV1            = []byte("analytix.raw-artifact-content-root/digest/v1\x00")
)

// RawArtifactContentChunkDescriptorV1 identifies one bounded opaque raw-byte
// chunk. Raw bytes never enter this JSON contract or ordinary projections.
type RawArtifactContentChunkDescriptorV1 struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Purpose             string `json:"purpose"`
	BindingKeyDigest    string `json:"bindingKeyDigest"`
	SourceLocatorDigest string `json:"sourceLocatorDigest"`
	ChunkNumber         uint64 `json:"chunkNumber"`
	ByteOffset          uint64 `json:"byteOffset"`
	ChunkSHA256         string `json:"chunkSha256"`
	ChunkByteLength     uint64 `json:"chunkByteLength"`
	DescriptorDigest    string `json:"descriptorDigest"`
}

type RawArtifactContentIndexPageV1 struct {
	SchemaVersion       int                                   `json:"schemaVersion"`
	Purpose             string                                `json:"purpose"`
	BindingKeyDigest    string                                `json:"bindingKeyDigest"`
	SourceLocatorDigest string                                `json:"sourceLocatorDigest"`
	IndexPageNumber     uint64                                `json:"indexPageNumber"`
	ChunkDescriptors    []RawArtifactContentChunkDescriptorV1 `json:"chunkDescriptors"`
	ChunkCount          uint32                                `json:"chunkCount"`
	AggregateChunkBytes uint64                                `json:"aggregateChunkBytes"`
	IndexPageDigest     string                                `json:"indexPageDigest"`
}

type RawArtifactContentIndexPageDescriptorV1 struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Purpose             string `json:"purpose"`
	BindingKeyDigest    string `json:"bindingKeyDigest"`
	SourceLocatorDigest string `json:"sourceLocatorDigest"`
	IndexPageNumber     uint64 `json:"indexPageNumber"`
	IndexPageDigest     string `json:"indexPageDigest"`
	IndexPageSHA256     string `json:"indexPageSha256"`
	IndexPageByteLength uint64 `json:"indexPageByteLength"`
	FirstChunkNumber    uint64 `json:"firstChunkNumber"`
	LastChunkNumber     uint64 `json:"lastChunkNumber"`
	ChunkCount          uint32 `json:"chunkCount"`
	FirstByteOffset     uint64 `json:"firstByteOffset"`
	LastByteExclusive   uint64 `json:"lastByteExclusive"`
	AggregateChunkBytes uint64 `json:"aggregateChunkBytes"`
	DescriptorDigest    string `json:"descriptorDigest"`
}

// RawArtifactContentRootV1 is restricted, pseudonymous private-CAS metadata
// over opaque raw chunks. Stable hashes remain correlatable. FullSHA256 must
// be recomputed while exact-reading chunks; this structural value alone is
// never acquisition or snapshot authority.
type RawArtifactContentRootV1 struct {
	SchemaVersion        int                                       `json:"schemaVersion"`
	Purpose              string                                    `json:"purpose"`
	BindingKeyDigest     string                                    `json:"bindingKeyDigest"`
	SourceLocatorDigest  string                                    `json:"sourceLocatorDigest"`
	IndexPageDescriptors []RawArtifactContentIndexPageDescriptorV1 `json:"indexPageDescriptors"`
	IndexPageCount       uint32                                    `json:"indexPageCount"`
	ChunkCount           uint64                                    `json:"chunkCount"`
	ArtifactByteLength   uint64                                    `json:"artifactByteLength"`
	FullSHA256           string                                    `json:"fullSha256"`
	RootDigest           string                                    `json:"rootDigest"`
}

func newRawArtifactContentChunkDescriptorV1(
	bindingKeyDigest string,
	sourceLocatorDigest string,
	chunkNumber uint64,
	byteOffset uint64,
	body []byte,
) (RawArtifactContentChunkDescriptorV1, error) {
	if len(body) == 0 || uint64(len(body)) > RawArtifactContentChunkBytesV1 {
		return RawArtifactContentChunkDescriptorV1{}, errors.New("raw artifact content chunk exceeds its fixed bound")
	}
	descriptor := RawArtifactContentChunkDescriptorV1{
		SchemaVersion:    RawArtifactContentChunkDescriptorSchemaVersionV1,
		Purpose:          RawArtifactContentChunkDescriptorPurposeV1,
		BindingKeyDigest: bindingKeyDigest, SourceLocatorDigest: sourceLocatorDigest,
		ChunkNumber: chunkNumber, ByteOffset: byteOffset,
		ChunkSHA256: domainsecurity.SHA256Hex(body), ChunkByteLength: uint64(len(body)),
	}
	descriptor.DescriptorDigest = rawArtifactContentChunkDescriptorDigestV1(descriptor)
	if err := ValidateRawArtifactContentChunkDescriptorV1(descriptor); err != nil {
		return RawArtifactContentChunkDescriptorV1{}, err
	}
	return descriptor, nil
}

func ValidateRawArtifactContentChunkDescriptorV1(descriptor RawArtifactContentChunkDescriptorV1) error {
	if descriptor.SchemaVersion != RawArtifactContentChunkDescriptorSchemaVersionV1 ||
		descriptor.Purpose != RawArtifactContentChunkDescriptorPurposeV1 ||
		!validSourceRowSHA256V1(descriptor.BindingKeyDigest) || !validSourceRowSHA256V1(descriptor.SourceLocatorDigest) ||
		descriptor.ChunkNumber == 0 || descriptor.ChunkNumber > maxSourceRowJSONIntegerV1 ||
		descriptor.ByteOffset > maxRawArtifactContentBytesV1 || !validSourceRowSHA256V1(descriptor.ChunkSHA256) ||
		descriptor.ChunkByteLength == 0 || descriptor.ChunkByteLength > RawArtifactContentChunkBytesV1 ||
		descriptor.ByteOffset > maxRawArtifactContentBytesV1-descriptor.ChunkByteLength ||
		!validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != rawArtifactContentChunkDescriptorDigestV1(descriptor) {
		return errors.New("raw artifact content chunk descriptor is invalid")
	}
	return nil
}

func ValidateRawArtifactContentChunkBytesV1(descriptor RawArtifactContentChunkDescriptorV1, body []byte) error {
	if ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil || uint64(len(body)) != descriptor.ChunkByteLength ||
		domainsecurity.SHA256Hex(body) != descriptor.ChunkSHA256 {
		return errors.New("raw artifact content chunk exact bytes mismatch")
	}
	return nil
}

func newRawArtifactContentIndexPageV1(
	bindingKeyDigest string,
	sourceLocatorDigest string,
	indexPageNumber uint64,
	descriptors []RawArtifactContentChunkDescriptorV1,
) (RawArtifactContentIndexPageV1, error) {
	if len(descriptors) == 0 || len(descriptors) > maxRawArtifactChunksPerIndexPageV1 {
		return RawArtifactContentIndexPageV1{}, errors.New("raw artifact content index page input is invalid")
	}
	page := RawArtifactContentIndexPageV1{
		SchemaVersion: RawArtifactContentIndexPageSchemaVersionV1, Purpose: RawArtifactContentIndexPagePurposeV1,
		BindingKeyDigest: bindingKeyDigest, SourceLocatorDigest: sourceLocatorDigest, IndexPageNumber: indexPageNumber,
		ChunkDescriptors: append([]RawArtifactContentChunkDescriptorV1(nil), descriptors...), ChunkCount: uint32(len(descriptors)),
	}
	for _, descriptor := range page.ChunkDescriptors {
		if ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil ||
			page.AggregateChunkBytes > maxRawArtifactContentBytesV1-descriptor.ChunkByteLength {
			return RawArtifactContentIndexPageV1{}, errors.New("raw artifact content index page descriptor is invalid")
		}
		page.AggregateChunkBytes += descriptor.ChunkByteLength
	}
	page.IndexPageDigest = rawArtifactContentIndexPageDigestV1(page)
	if err := ValidateRawArtifactContentIndexPageV1(page); err != nil {
		return RawArtifactContentIndexPageV1{}, err
	}
	return page, nil
}

func ValidateRawArtifactContentIndexPageV1(page RawArtifactContentIndexPageV1) error {
	if page.SchemaVersion != RawArtifactContentIndexPageSchemaVersionV1 || page.Purpose != RawArtifactContentIndexPagePurposeV1 ||
		!validSourceRowSHA256V1(page.BindingKeyDigest) || !validSourceRowSHA256V1(page.SourceLocatorDigest) ||
		page.IndexPageNumber == 0 || page.IndexPageNumber > maxRawArtifactContentRootIndexPagesV1 ||
		len(page.ChunkDescriptors) == 0 || len(page.ChunkDescriptors) > maxRawArtifactChunksPerIndexPageV1 ||
		page.ChunkCount != uint32(len(page.ChunkDescriptors)) || page.AggregateChunkBytes == 0 ||
		page.AggregateChunkBytes > maxRawArtifactContentBytesV1 || !validSourceRowSHA256V1(page.IndexPageDigest) {
		return errors.New("raw artifact content index page is invalid")
	}
	var aggregate uint64
	for index, descriptor := range page.ChunkDescriptors {
		if ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil ||
			descriptor.BindingKeyDigest != page.BindingKeyDigest || descriptor.SourceLocatorDigest != page.SourceLocatorDigest ||
			aggregate > maxRawArtifactContentBytesV1-descriptor.ChunkByteLength {
			return errors.New("raw artifact content index page descriptors are invalid")
		}
		if index > 0 {
			previous := page.ChunkDescriptors[index-1]
			if descriptor.ChunkNumber != previous.ChunkNumber+1 ||
				descriptor.ByteOffset != previous.ByteOffset+previous.ChunkByteLength {
				return errors.New("raw artifact content chunks are not contiguous")
			}
		}
		if index < len(page.ChunkDescriptors)-1 && descriptor.ChunkByteLength != RawArtifactContentChunkBytesV1 {
			return errors.New("raw artifact content index contains a partial non-terminal chunk")
		}
		aggregate += descriptor.ChunkByteLength
	}
	body, err := json.Marshal(page)
	if err != nil || len(body) > maxRawArtifactContentIndexPageBytesV1 || aggregate != page.AggregateChunkBytes ||
		page.IndexPageDigest != rawArtifactContentIndexPageDigestV1(page) {
		return errors.New("raw artifact content index page content is invalid")
	}
	return nil
}

func newRawArtifactContentIndexPageDescriptorV1(
	page RawArtifactContentIndexPageV1,
) (RawArtifactContentIndexPageDescriptorV1, error) {
	if err := ValidateRawArtifactContentIndexPageV1(page); err != nil {
		return RawArtifactContentIndexPageDescriptorV1{}, err
	}
	body, _ := json.Marshal(page)
	first := page.ChunkDescriptors[0]
	last := page.ChunkDescriptors[len(page.ChunkDescriptors)-1]
	descriptor := RawArtifactContentIndexPageDescriptorV1{
		SchemaVersion:    RawArtifactContentIndexDescriptorSchemaVersionV1,
		Purpose:          RawArtifactContentIndexDescriptorPurposeV1,
		BindingKeyDigest: page.BindingKeyDigest, SourceLocatorDigest: page.SourceLocatorDigest,
		IndexPageNumber: page.IndexPageNumber, IndexPageDigest: page.IndexPageDigest,
		IndexPageSHA256: domainsecurity.SHA256Hex(body), IndexPageByteLength: uint64(len(body)),
		FirstChunkNumber: first.ChunkNumber, LastChunkNumber: last.ChunkNumber, ChunkCount: page.ChunkCount,
		FirstByteOffset: first.ByteOffset, LastByteExclusive: last.ByteOffset + last.ChunkByteLength,
		AggregateChunkBytes: page.AggregateChunkBytes,
	}
	descriptor.DescriptorDigest = rawArtifactContentIndexDescriptorDigestV1(descriptor)
	if err := ValidateRawArtifactContentIndexPageDescriptorV1(descriptor); err != nil {
		return RawArtifactContentIndexPageDescriptorV1{}, err
	}
	return descriptor, nil
}

func ValidateRawArtifactContentIndexPageDescriptorV1(descriptor RawArtifactContentIndexPageDescriptorV1) error {
	if descriptor.SchemaVersion != RawArtifactContentIndexDescriptorSchemaVersionV1 ||
		descriptor.Purpose != RawArtifactContentIndexDescriptorPurposeV1 ||
		!validSourceRowSHA256V1(descriptor.BindingKeyDigest) || !validSourceRowSHA256V1(descriptor.SourceLocatorDigest) ||
		descriptor.IndexPageNumber == 0 ||
		descriptor.IndexPageNumber > maxRawArtifactContentRootIndexPagesV1 || !validSourceRowSHA256V1(descriptor.IndexPageDigest) ||
		!validSourceRowSHA256V1(descriptor.IndexPageSHA256) || descriptor.IndexPageByteLength == 0 ||
		descriptor.IndexPageByteLength > maxRawArtifactContentIndexPageBytesV1 || descriptor.FirstChunkNumber == 0 ||
		descriptor.LastChunkNumber < descriptor.FirstChunkNumber || descriptor.ChunkCount == 0 ||
		descriptor.ChunkCount > maxRawArtifactChunksPerIndexPageV1 ||
		descriptor.LastChunkNumber-descriptor.FirstChunkNumber+1 != uint64(descriptor.ChunkCount) ||
		descriptor.FirstByteOffset > maxRawArtifactContentBytesV1 || descriptor.LastByteExclusive <= descriptor.FirstByteOffset ||
		descriptor.LastByteExclusive > maxRawArtifactContentBytesV1 || descriptor.AggregateChunkBytes == 0 ||
		descriptor.AggregateChunkBytes != descriptor.LastByteExclusive-descriptor.FirstByteOffset ||
		!validSourceRowSHA256V1(descriptor.DescriptorDigest) ||
		descriptor.DescriptorDigest != rawArtifactContentIndexDescriptorDigestV1(descriptor) {
		return errors.New("raw artifact content index page descriptor is invalid")
	}
	return nil
}

// ValidateRawArtifactContentIndexPageAgainstDescriptorV1 compares one exact
// canonical index page with its parent descriptor.
func ValidateRawArtifactContentIndexPageAgainstDescriptorV1(
	descriptor RawArtifactContentIndexPageDescriptorV1,
	page RawArtifactContentIndexPageV1,
) error {
	if ValidateRawArtifactContentIndexPageDescriptorV1(descriptor) != nil ||
		ValidateRawArtifactContentIndexPageV1(page) != nil {
		return errors.New("raw artifact content index descriptor material is invalid")
	}
	derived, err := newRawArtifactContentIndexPageDescriptorV1(page)
	if err != nil || derived != descriptor {
		return errors.New("raw artifact content index page does not match its descriptor")
	}
	return nil
}

func newRawArtifactContentRootV1(
	bindingKeyDigest string,
	sourceLocatorDigest string,
	fullSHA256 string,
	descriptors []RawArtifactContentIndexPageDescriptorV1,
) (RawArtifactContentRootV1, error) {
	if len(descriptors) == 0 || len(descriptors) > maxRawArtifactContentRootIndexPagesV1 {
		return RawArtifactContentRootV1{}, errors.New("raw artifact content root input is invalid")
	}
	root := RawArtifactContentRootV1{
		SchemaVersion: RawArtifactContentRootSchemaVersionV1, Purpose: RawArtifactContentRootPurposeV1,
		BindingKeyDigest: bindingKeyDigest, SourceLocatorDigest: sourceLocatorDigest,
		IndexPageDescriptors: append([]RawArtifactContentIndexPageDescriptorV1(nil), descriptors...),
		IndexPageCount:       uint32(len(descriptors)), FullSHA256: fullSHA256,
	}
	for _, descriptor := range root.IndexPageDescriptors {
		if ValidateRawArtifactContentIndexPageDescriptorV1(descriptor) != nil ||
			root.ChunkCount > maxSourceRowJSONIntegerV1-uint64(descriptor.ChunkCount) ||
			root.ArtifactByteLength > maxRawArtifactContentBytesV1-descriptor.AggregateChunkBytes {
			return RawArtifactContentRootV1{}, errors.New("raw artifact content root descriptor is invalid")
		}
		root.ChunkCount += uint64(descriptor.ChunkCount)
		root.ArtifactByteLength += descriptor.AggregateChunkBytes
	}
	root.RootDigest = rawArtifactContentRootDigestV1(root)
	if err := ValidateRawArtifactContentRootV1(root); err != nil {
		return RawArtifactContentRootV1{}, err
	}
	return root, nil
}

func ValidateRawArtifactContentRootV1(root RawArtifactContentRootV1) error {
	if root.SchemaVersion != RawArtifactContentRootSchemaVersionV1 || root.Purpose != RawArtifactContentRootPurposeV1 ||
		!validSourceRowSHA256V1(root.BindingKeyDigest) || !validSourceRowSHA256V1(root.SourceLocatorDigest) ||
		len(root.IndexPageDescriptors) == 0 || len(root.IndexPageDescriptors) > maxRawArtifactContentRootIndexPagesV1 ||
		root.IndexPageCount != uint32(len(root.IndexPageDescriptors)) || root.ChunkCount == 0 ||
		root.ChunkCount > uint64(maxRawArtifactChunksPerIndexPageV1)*uint64(root.IndexPageCount) ||
		root.ArtifactByteLength == 0 || root.ArtifactByteLength > maxRawArtifactContentBytesV1 ||
		!validSourceRowSHA256V1(root.FullSHA256) || !validSourceRowSHA256V1(root.RootDigest) {
		return errors.New("raw artifact content root is invalid")
	}
	var chunks uint64
	var totalBytes uint64
	seenPageDigests := make(map[string]struct{}, len(root.IndexPageDescriptors))
	seenPageSHA256 := make(map[string]struct{}, len(root.IndexPageDescriptors))
	for index, descriptor := range root.IndexPageDescriptors {
		if ValidateRawArtifactContentIndexPageDescriptorV1(descriptor) != nil || descriptor.IndexPageNumber != uint64(index+1) ||
			descriptor.BindingKeyDigest != root.BindingKeyDigest || descriptor.SourceLocatorDigest != root.SourceLocatorDigest ||
			chunks > maxSourceRowJSONIntegerV1-uint64(descriptor.ChunkCount) ||
			totalBytes > maxRawArtifactContentBytesV1-descriptor.AggregateChunkBytes {
			return errors.New("raw artifact content root descriptors are not canonical")
		}
		if index < len(root.IndexPageDescriptors)-1 &&
			(descriptor.ChunkCount != maxRawArtifactChunksPerIndexPageV1 ||
				descriptor.AggregateChunkBytes != uint64(descriptor.ChunkCount)*RawArtifactContentChunkBytesV1) {
			return errors.New("raw artifact content root contains a partial non-terminal index page")
		}
		if _, exists := seenPageDigests[descriptor.IndexPageDigest]; exists {
			return errors.New("raw artifact content root reuses an index page digest")
		}
		if _, exists := seenPageSHA256[descriptor.IndexPageSHA256]; exists {
			return errors.New("raw artifact content root reuses an index page content address")
		}
		if index == 0 {
			if descriptor.FirstChunkNumber != 1 || descriptor.FirstByteOffset != 0 {
				return errors.New("raw artifact content root does not begin at the first byte")
			}
		} else {
			previous := root.IndexPageDescriptors[index-1]
			if descriptor.FirstChunkNumber != previous.LastChunkNumber+1 ||
				descriptor.FirstByteOffset != previous.LastByteExclusive {
				return errors.New("raw artifact content root index pages are not contiguous")
			}
		}
		seenPageDigests[descriptor.IndexPageDigest] = struct{}{}
		seenPageSHA256[descriptor.IndexPageSHA256] = struct{}{}
		chunks += uint64(descriptor.ChunkCount)
		totalBytes += descriptor.AggregateChunkBytes
	}
	body, err := json.Marshal(root)
	if err != nil || len(body) > maxRawArtifactContentRootBytesV1 || chunks != root.ChunkCount ||
		totalBytes != root.ArtifactByteLength || root.RootDigest != rawArtifactContentRootDigestV1(root) {
		return errors.New("raw artifact content root aggregate is invalid")
	}
	return nil
}

// ValidateRawArtifactContentIndexPageMembershipV1 proves exact structural
// root -> index-page membership. Production admission must still exact-read
// every referenced chunk and validate its bytes.
func ValidateRawArtifactContentIndexPageMembershipV1(
	root RawArtifactContentRootV1,
	page RawArtifactContentIndexPageV1,
) error {
	if ValidateRawArtifactContentRootV1(root) != nil || ValidateRawArtifactContentIndexPageV1(page) != nil ||
		page.BindingKeyDigest != root.BindingKeyDigest || page.SourceLocatorDigest != root.SourceLocatorDigest ||
		page.IndexPageNumber > uint64(len(root.IndexPageDescriptors)) {
		return errors.New("raw artifact content index membership material is invalid")
	}
	descriptor := root.IndexPageDescriptors[page.IndexPageNumber-1]
	if ValidateRawArtifactContentIndexPageAgainstDescriptorV1(descriptor, page) != nil {
		return errors.New("raw artifact content index page descriptor mismatch")
	}
	return nil
}

// ValidateRawArtifactContentFullSHA256V1 consumes exactly the artifact byte
// length and recomputes the full-file SHA-256. It rejects both short and extra
// reads. A production caller must construct reader only from exact, ordered,
// descriptor-validated private-CAS chunks.
func ValidateRawArtifactContentFullSHA256V1(root RawArtifactContentRootV1, reader io.Reader) error {
	if ValidateRawArtifactContentRootV1(root) != nil || reader == nil {
		return errors.New("raw artifact full content material is invalid")
	}
	hasher := sha256.New()
	written, err := io.CopyN(hasher, reader, int64(root.ArtifactByteLength))
	if err != nil || written != int64(root.ArtifactByteLength) {
		return errors.New("raw artifact full content is shorter than its root")
	}
	var extra [1]byte
	n, extraErr := reader.Read(extra[:])
	if n != 0 || extraErr == nil || extraErr != io.EOF {
		return errors.New("raw artifact full content is longer than its root")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != root.FullSHA256 {
		return errors.New("raw artifact full content SHA-256 mismatch")
	}
	return nil
}

func ParseRawArtifactContentIndexPageV1(raw []byte) (RawArtifactContentIndexPageV1, error) {
	var page RawArtifactContentIndexPageV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &page, maxRawArtifactContentIndexPageBytesV1, maxRawArtifactContentIndexParserTokensV1, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactContentIndexPageV1{}, err
	}
	return page, ValidateRawArtifactContentIndexPageV1(page)
}

func RawArtifactContentIndexPageV1Bytes(page RawArtifactContentIndexPageV1) ([]byte, error) {
	if err := ValidateRawArtifactContentIndexPageV1(page); err != nil {
		return nil, err
	}
	return json.Marshal(page)
}

func ParseRawArtifactContentRootV1(raw []byte) (RawArtifactContentRootV1, error) {
	var root RawArtifactContentRootV1
	if err := parseCanonicalSourceRowContractV1(
		raw, &root, maxRawArtifactContentRootBytesV1, maxRawArtifactContentIndexParserTokensV1, maxSourceRowPolicyTextBytesV1,
	); err != nil {
		return RawArtifactContentRootV1{}, err
	}
	return root, ValidateRawArtifactContentRootV1(root)
}

func RawArtifactContentRootV1Bytes(root RawArtifactContentRootV1) ([]byte, error) {
	if err := ValidateRawArtifactContentRootV1(root); err != nil {
		return nil, err
	}
	return json.Marshal(root)
}

func rawArtifactContentChunkDescriptorDigestV1(descriptor RawArtifactContentChunkDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactContentChunkDescriptorDigestDomainV1...), body...))
}

func rawArtifactContentIndexPageDigestV1(page RawArtifactContentIndexPageV1) string {
	page.IndexPageDigest = ""
	body, _ := json.Marshal(page)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactContentIndexPageDigestDomainV1...), body...))
}

func rawArtifactContentIndexDescriptorDigestV1(descriptor RawArtifactContentIndexPageDescriptorV1) string {
	descriptor.DescriptorDigest = ""
	body, _ := json.Marshal(descriptor)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactContentIndexDescriptorDigestDomainV1...), body...))
}

func rawArtifactContentRootDigestV1(root RawArtifactContentRootV1) string {
	root.RootDigest = ""
	body, _ := json.Marshal(root)
	return domainsecurity.SHA256Hex(append(append([]byte(nil), rawArtifactContentRootDigestDomainV1...), body...))
}
