package evidence

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRawArtifactContentV1ChunkedExactRoundTrip(t *testing.T) {
	bindingDigest := sourceRowTestBindingV1(t, "case-raw-content").BindingKeyDigest
	locatorDigest := domainsecurity.SHA256Hex([]byte("host-source-locator"))
	firstBody := make([]byte, RawArtifactContentChunkBytesV1)
	for index := range firstBody {
		firstBody[index] = byte(index % 251)
	}
	secondBody := []byte("opaque-tail-with-001234567890-leading-zero-bytes")
	first, err := newRawArtifactContentChunkDescriptorV1(bindingDigest, locatorDigest, 1, 0, firstBody)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRawArtifactContentChunkDescriptorV1(
		bindingDigest, locatorDigest, 2, uint64(len(firstBody)), secondBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := newRawArtifactContentIndexPageV1(
		bindingDigest, locatorDigest, 1, []RawArtifactContentChunkDescriptorV1{first, second},
	)
	if err != nil {
		t.Fatal(err)
	}
	pageDescriptor, err := newRawArtifactContentIndexPageDescriptorV1(page)
	if err != nil {
		t.Fatal(err)
	}
	fullBody := append(append([]byte(nil), firstBody...), secondBody...)
	root, err := newRawArtifactContentRootV1(
		bindingDigest, locatorDigest, domainsecurity.SHA256Hex(fullBody), []RawArtifactContentIndexPageDescriptorV1{pageDescriptor},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactContentChunkBytesV1(first, firstBody); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactContentIndexPageMembershipV1(root, page); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactContentFullSHA256V1(root, bytes.NewReader(fullBody)); err != nil {
		t.Fatal(err)
	}
	if ValidateRawArtifactContentFullSHA256V1(root, bytes.NewReader(fullBody[:len(fullBody)-1])) == nil {
		t.Fatal("short raw artifact content matched its full SHA")
	}
	if ValidateRawArtifactContentFullSHA256V1(root, bytes.NewReader(append(append([]byte(nil), fullBody...), 0))) == nil {
		t.Fatal("extra raw artifact content matched its full SHA")
	}
	wrongFullSHA := root
	wrongFullSHA.FullSHA256 = domainsecurity.SHA256Hex([]byte("other-full-artifact"))
	wrongFullSHA.RootDigest = rawArtifactContentRootDigestV1(wrongFullSHA)
	if ValidateRawArtifactContentFullSHA256V1(wrongFullSHA, bytes.NewReader(fullBody)) == nil {
		t.Fatal("wrong full-file SHA matched exact raw artifact bytes")
	}
	tampered := append([]byte(nil), secondBody...)
	tampered[0] ^= 0xff
	if ValidateRawArtifactContentChunkBytesV1(second, tampered) == nil {
		t.Fatal("modified raw chunk bytes matched the immutable descriptor")
	}
	pageBody, err := RawArtifactContentIndexPageV1Bytes(page)
	if err != nil {
		t.Fatal(err)
	}
	rootBody, err := RawArtifactContentRootV1Bytes(root)
	if err != nil {
		t.Fatal(err)
	}
	parsedPage, pageErr := ParseRawArtifactContentIndexPageV1(pageBody)
	parsedRoot, rootErr := ParseRawArtifactContentRootV1(rootBody)
	if pageErr != nil || rootErr != nil || parsedPage.IndexPageDigest != page.IndexPageDigest || parsedRoot.RootDigest != root.RootDigest ||
		root.ArtifactByteLength != uint64(len(fullBody)) || root.ChunkCount != 2 {
		t.Fatalf("raw chunk hierarchy did not round-trip exactly: page=%v root=%v", pageErr, rootErr)
	}
}

func TestRawArtifactContentV1RejectsPartialReorderedAndUnknownMaterial(t *testing.T) {
	bindingDigest := sourceRowTestBindingV1(t, "case-raw-invalid").BindingKeyDigest
	locatorDigest := domainsecurity.SHA256Hex([]byte("host-source-locator-invalid"))
	first, err := newRawArtifactContentChunkDescriptorV1(bindingDigest, locatorDigest, 1, 0, []byte("partial-first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRawArtifactContentChunkDescriptorV1(
		bindingDigest, locatorDigest, 2, first.ChunkByteLength, []byte("tail"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newRawArtifactContentIndexPageV1(
		bindingDigest, locatorDigest, 1, []RawArtifactContentChunkDescriptorV1{first, second},
	)
	requireSourceRowErrorContainsV1(t, err, "partial non-terminal chunk")
	fullFirst := first
	fullFirst.ChunkByteLength = RawArtifactContentChunkBytesV1
	fullFirst.ChunkSHA256 = domainsecurity.SHA256Hex([]byte("synthetic-full-first"))
	fullFirst.DescriptorDigest = rawArtifactContentChunkDescriptorDigestV1(fullFirst)
	discontinuous := second
	discontinuous.ChunkNumber = 3
	discontinuous.ByteOffset = RawArtifactContentChunkBytesV1
	discontinuous.DescriptorDigest = rawArtifactContentChunkDescriptorDigestV1(discontinuous)
	_, err = newRawArtifactContentIndexPageV1(
		bindingDigest, locatorDigest, 1, []RawArtifactContentChunkDescriptorV1{fullFirst, discontinuous},
	)
	requireSourceRowErrorContainsV1(t, err, "not contiguous")

	lastOnly, err := newRawArtifactContentIndexPageV1(
		bindingDigest, locatorDigest, 1, []RawArtifactContentChunkDescriptorV1{first},
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := RawArtifactContentIndexPageV1Bytes(lastOnly)
	if err != nil {
		t.Fatal(err)
	}
	var external map[string]any
	if err := json.Unmarshal(body, &external); err != nil {
		t.Fatal(err)
	}
	external["providerAuthority"] = true
	unknown, _ := json.Marshal(external)
	if _, err := ParseRawArtifactContentIndexPageV1(unknown); err == nil {
		t.Fatal("unknown provider field entered raw content index")
	}
}

func TestRawArtifactContentV1BoundedMerkleCapacityCoversOneTiB(t *testing.T) {
	bindingDigest := sourceRowTestBindingV1(t, "case-raw-capacity").BindingKeyDigest
	locatorDigest := domainsecurity.SHA256Hex([]byte("host-source-locator-capacity"))
	chunks := make([]RawArtifactContentChunkDescriptorV1, maxRawArtifactChunksPerIndexPageV1)
	for index := range chunks {
		descriptor := RawArtifactContentChunkDescriptorV1{
			SchemaVersion:    RawArtifactContentChunkDescriptorSchemaVersionV1,
			Purpose:          RawArtifactContentChunkDescriptorPurposeV1,
			BindingKeyDigest: bindingDigest, SourceLocatorDigest: locatorDigest,
			ChunkNumber: uint64(index + 1), ByteOffset: uint64(index) * RawArtifactContentChunkBytesV1,
			ChunkSHA256:     domainsecurity.SHA256Hex([]byte("chunk:" + strconv.Itoa(index))),
			ChunkByteLength: RawArtifactContentChunkBytesV1,
		}
		descriptor.DescriptorDigest = rawArtifactContentChunkDescriptorDigestV1(descriptor)
		chunks[index] = descriptor
	}
	page, err := newRawArtifactContentIndexPageV1(bindingDigest, locatorDigest, 1, chunks)
	if err != nil {
		t.Fatal(err)
	}
	pageBody, err := RawArtifactContentIndexPageV1Bytes(page)
	if err != nil || len(pageBody) > maxRawArtifactContentIndexPageBytesV1 {
		t.Fatalf("full raw content index page exceeds its canonical bound: bytes=%d err=%v", len(pageBody), err)
	}
	base, err := newRawArtifactContentIndexPageDescriptorV1(page)
	if err != nil {
		t.Fatal(err)
	}
	pages := make([]RawArtifactContentIndexPageDescriptorV1, maxRawArtifactContentRootIndexPagesV1)
	for index := range pages {
		descriptor := base
		descriptor.IndexPageNumber = uint64(index + 1)
		descriptor.IndexPageDigest = domainsecurity.SHA256Hex([]byte("content-index-digest:" + strconv.Itoa(index)))
		descriptor.IndexPageSHA256 = domainsecurity.SHA256Hex([]byte("content-index-bytes:" + strconv.Itoa(index)))
		descriptor.FirstChunkNumber = uint64(index*maxRawArtifactChunksPerIndexPageV1 + 1)
		descriptor.LastChunkNumber = descriptor.FirstChunkNumber + maxRawArtifactChunksPerIndexPageV1 - 1
		descriptor.FirstByteOffset = uint64(index*maxRawArtifactChunksPerIndexPageV1) * RawArtifactContentChunkBytesV1
		descriptor.LastByteExclusive = descriptor.FirstByteOffset +
			uint64(maxRawArtifactChunksPerIndexPageV1)*RawArtifactContentChunkBytesV1
		descriptor.AggregateChunkBytes = descriptor.LastByteExclusive - descriptor.FirstByteOffset
		descriptor.DescriptorDigest = rawArtifactContentIndexDescriptorDigestV1(descriptor)
		pages[index] = descriptor
	}
	root, err := newRawArtifactContentRootV1(
		bindingDigest, locatorDigest, domainsecurity.SHA256Hex([]byte("one-tib-full-sha")), pages,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootBody, err := RawArtifactContentRootV1Bytes(root)
	if err != nil || root.ArtifactByteLength != maxRawArtifactContentBytesV1 ||
		len(rootBody) > maxRawArtifactContentRootBytesV1 {
		t.Fatalf("bounded raw Merkle root cannot represent one TiB: bytes=%d length=%d err=%v", len(rootBody), root.ArtifactByteLength, err)
	}
	if _, err := ParseRawArtifactContentRootV1(rootBody); err != nil {
		t.Fatalf("maximum raw content root did not parse: %v", err)
	}
}

func TestRawArtifactContentV1RejectsCrossBindingAndLocatorIndexSubstitution(t *testing.T) {
	hierarchy := rawArtifactHierarchyForTestV1(t, 1, "content-substitution")
	root := hierarchy.contentRoots[0]
	locator := hierarchy.locators[0]
	content := []byte("opaque-source:content-substitution:0")
	chunk, err := newRawArtifactContentChunkDescriptorV1(
		root.BindingKeyDigest, locator.LocatorDigest, 1, 0, content,
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := newRawArtifactContentIndexPageV1(
		root.BindingKeyDigest, locator.LocatorDigest, 1, []RawArtifactContentChunkDescriptorV1{chunk},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactContentIndexPageMembershipV1(root, page); err != nil {
		t.Fatal(err)
	}

	otherBinding := sourceRowTestBindingV1(t, "case-content-substitution-other").BindingKeyDigest
	otherChunk, err := newRawArtifactContentChunkDescriptorV1(otherBinding, locator.LocatorDigest, 1, 0, content)
	if err != nil {
		t.Fatal(err)
	}
	otherPage, err := newRawArtifactContentIndexPageV1(
		otherBinding, locator.LocatorDigest, 1, []RawArtifactContentChunkDescriptorV1{otherChunk},
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateRawArtifactContentIndexPageMembershipV1(root, otherPage) == nil {
		t.Fatal("index page from another binding matched raw content root")
	}

	otherLocator := domainsecurity.SHA256Hex([]byte("other-source-locator"))
	locatorChunk, err := newRawArtifactContentChunkDescriptorV1(root.BindingKeyDigest, otherLocator, 1, 0, content)
	if err != nil {
		t.Fatal(err)
	}
	locatorPage, err := newRawArtifactContentIndexPageV1(
		root.BindingKeyDigest, otherLocator, 1, []RawArtifactContentChunkDescriptorV1{locatorChunk},
	)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateRawArtifactContentIndexPageMembershipV1(root, locatorPage) == nil {
		t.Fatal("index page from another source locator matched raw content root")
	}
}
