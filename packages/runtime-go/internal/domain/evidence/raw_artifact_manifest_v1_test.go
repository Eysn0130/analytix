package evidence

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type rawArtifactHierarchyTestV1 struct {
	intent       RawArtifactAcquisitionIntentV1
	locators     []RawArtifactSourceLocatorV1
	contentRoots []RawArtifactContentRootV1
	entries      []RawArtifactEntryV1
	pages        []RawArtifactManifestPageV1
	manifest     RawArtifactManifestV1
}

func TestRawArtifactManifestV1ExactHierarchyRoundTrip(t *testing.T) {
	hierarchy := rawArtifactHierarchyForTestV1(t, 2, "exact")
	if err := ValidateRawArtifactManifestHierarchyV1(hierarchy.intent, hierarchy.manifest, hierarchy.pages); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactManifestEntryMembershipV1(
		hierarchy.manifest, hierarchy.pages[0], hierarchy.entries[1],
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactEntryAgainstSourceLocatorV1(
		hierarchy.intent, hierarchy.entries[1], hierarchy.locators[1],
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRawArtifactEntryWithContentRootV1(hierarchy.entries[1], hierarchy.contentRoots[1]); err != nil {
		t.Fatal(err)
	}

	intentBody, _ := RawArtifactAcquisitionIntentV1Bytes(hierarchy.intent)
	locatorBody, _ := RawArtifactSourceLocatorV1Bytes(hierarchy.locators[0])
	entryBody, _ := RawArtifactEntryV1Bytes(hierarchy.entries[0])
	pageBody, _ := RawArtifactManifestPageV1Bytes(hierarchy.pages[0])
	manifestBody, _ := RawArtifactManifestV1Bytes(hierarchy.manifest)
	if _, err := ParseRawArtifactAcquisitionIntentV1(intentBody); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRawArtifactSourceLocatorV1(locatorBody); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRawArtifactEntryV1(entryBody); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRawArtifactManifestPageV1(pageBody); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRawArtifactManifestV1(manifestBody); err != nil {
		t.Fatal(err)
	}
	if hierarchy.manifest.AcquisitionIntentDigest != hierarchy.intent.IntentDigest ||
		hierarchy.manifest.ArtifactCount != hierarchy.intent.ExpectedArtifactCount {
		t.Fatal("manifest detached from its pre-content acquisition intent")
	}
}

func TestRawArtifactManifestV1RejectsCrossPageSourceReuseAndCASSubstitution(t *testing.T) {
	hierarchy := rawArtifactHierarchyForTestV1(t, maxRawArtifactsPerManifestPageV1+1, "cross-page")
	if err := ValidateRawArtifactManifestHierarchyV1(hierarchy.intent, hierarchy.manifest, hierarchy.pages); err != nil {
		t.Fatal(err)
	}

	duplicatePages := append([]RawArtifactManifestPageV1(nil), hierarchy.pages...)
	duplicatePages[1] = cloneRawArtifactManifestPageForTestV1(t, duplicatePages[1])
	duplicate := duplicatePages[1].Entries[0]
	duplicate.SourceFileIDDigest = hierarchy.entries[0].SourceFileIDDigest
	duplicate.SourceLocatorDigest = hierarchy.entries[0].SourceLocatorDigest
	duplicate.SourceLocatorSHA256 = hierarchy.entries[0].SourceLocatorSHA256
	duplicate.SourceLocatorByteLength = hierarchy.entries[0].SourceLocatorByteLength
	duplicate.ArtifactID = deriveRawArtifactIDV1(duplicate)
	duplicate.EntryDigest = rawArtifactEntryDigestV1(duplicate)
	duplicatePages[1].Entries[0] = duplicate
	duplicatePages[1].PageDigest = rawArtifactManifestPageDigestV1(duplicatePages[1])
	duplicateDescriptor, err := newRawArtifactManifestPageDescriptorV1(duplicatePages[1])
	if err != nil {
		t.Fatal(err)
	}
	descriptors := append([]RawArtifactManifestPageDescriptorV1(nil), hierarchy.manifest.PageDescriptors...)
	descriptors[1] = duplicateDescriptor
	duplicateManifest, err := newRawArtifactManifestV1(hierarchy.intent, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateRawArtifactManifestHierarchyV1(hierarchy.intent, duplicateManifest, duplicatePages) == nil {
		t.Fatal("source locator reused across manifest pages")
	}

	tampered := hierarchy.manifest
	tampered.PageDescriptors = append([]RawArtifactManifestPageDescriptorV1(nil), hierarchy.manifest.PageDescriptors...)
	tampered.PageDescriptors[0].PageSHA256 = domainsecurity.SHA256Hex([]byte("other-page-body"))
	tampered.PageDescriptors[0].DescriptorDigest = rawArtifactManifestPageDescriptorDigestV1(tampered.PageDescriptors[0])
	tampered.ArtifactSetDigest = rawArtifactSetDigestV1(tampered)
	tampered.ManifestDigest = rawArtifactManifestDigestV1(tampered)
	if err := ValidateRawArtifactManifestV1(tampered); err != nil {
		t.Fatalf("structural substitution fixture is invalid: %v", err)
	}
	if ValidateRawArtifactManifestEntryMembershipV1(tampered, hierarchy.pages[0], hierarchy.entries[0]) == nil {
		t.Fatal("manifest page physical CAS substitution matched exact membership")
	}

	wrongRoot := hierarchy.contentRoots[0]
	wrongRoot.FullSHA256 = domainsecurity.SHA256Hex([]byte("different-full-artifact"))
	wrongRoot.RootDigest = rawArtifactContentRootDigestV1(wrongRoot)
	if ValidateRawArtifactEntryWithContentRootV1(hierarchy.entries[0], wrongRoot) == nil {
		t.Fatal("entry matched a different content root")
	}
}

func TestRawArtifactManifestV1RejectsUnknownDuplicateAndPartialMaterial(t *testing.T) {
	hierarchy := rawArtifactHierarchyForTestV1(t, 2, "strict")
	body, err := RawArtifactManifestV1Bytes(hierarchy.manifest)
	if err != nil {
		t.Fatal(err)
	}
	var external map[string]any
	if err := json.Unmarshal(body, &external); err != nil {
		t.Fatal(err)
	}
	external["providerAuthority"] = true
	unknown, _ := json.Marshal(external)
	if _, err := ParseRawArtifactManifestV1(unknown); err == nil {
		t.Fatal("unknown provider field entered raw artifact manifest")
	}
	duplicateKey := append(append([]byte(nil), body[:len(body)-1]...),
		[]byte(`,"manifestDigest":"`+hierarchy.manifest.ManifestDigest+`"}`)...)
	if _, err := ParseRawArtifactManifestV1(duplicateKey); err == nil {
		t.Fatal("duplicate manifest key was accepted")
	}

	firstPage, err := newRawArtifactManifestPageV1(hierarchy.intent, 1, hierarchy.entries[:1])
	if err != nil {
		t.Fatal(err)
	}
	secondPage, err := newRawArtifactManifestPageV1(hierarchy.intent, 2, hierarchy.entries[1:])
	if err != nil {
		t.Fatal(err)
	}
	firstDescriptor, _ := newRawArtifactManifestPageDescriptorV1(firstPage)
	secondDescriptor, _ := newRawArtifactManifestPageDescriptorV1(secondPage)
	if _, err := newRawArtifactManifestV1(
		hierarchy.intent, []RawArtifactManifestPageDescriptorV1{firstDescriptor, secondDescriptor},
	); err == nil {
		t.Fatal("partial non-terminal raw manifest page was accepted")
	}

	wrongEntry := hierarchy.entries[0]
	wrongEntry.ArtifactOrdinal = 2
	wrongEntry.ArtifactID = deriveRawArtifactIDV1(wrongEntry)
	wrongEntry.EntryDigest = rawArtifactEntryDigestV1(wrongEntry)
	if ValidateRawArtifactManifestEntryMembershipV1(hierarchy.manifest, hierarchy.pages[0], wrongEntry) == nil {
		t.Fatal("wrong artifact ordinal matched manifest membership")
	}
}

func TestRawArtifactManifestV1BoundedRootCoversMaximumChunkPopulation(t *testing.T) {
	binding := sourceRowTestBindingV1(t, "case-raw-manifest-capacity")
	intent, err := newRawArtifactAcquisitionIntentV1(
		binding,
		time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		domainsecurity.SHA256Hex([]byte("raw-capacity-actor")),
		maxRawArtifactCountV1,
		domainsecurity.SHA256Hex([]byte("raw-capacity-intent-nonce")),
	)
	if err != nil {
		t.Fatal(err)
	}
	descriptors := make([]RawArtifactManifestPageDescriptorV1, maxRawArtifactManifestPagesV1)
	for index := range descriptors {
		firstOrdinal := uint64(index*maxRawArtifactsPerManifestPageV1 + 1)
		descriptor := RawArtifactManifestPageDescriptorV1{
			SchemaVersion:    RawArtifactManifestPageDescriptorSchemaVersionV1,
			Purpose:          RawArtifactManifestPageDescriptorPurposeV1,
			BindingKeyDigest: intent.BindingKeyDigest, AcquisitionIntentDigest: intent.IntentDigest,
			ManifestPageNumber: uint64(index + 1),
			PageDigest:         domainsecurity.SHA256Hex([]byte("page-digest:" + strconv.Itoa(index))),
			PageSHA256:         domainsecurity.SHA256Hex([]byte("page-body:" + strconv.Itoa(index))), PageByteLength: 1024,
			FirstArtifactOrdinal: firstOrdinal, LastArtifactOrdinal: firstOrdinal + maxRawArtifactsPerManifestPageV1 - 1,
			ArtifactCount:       maxRawArtifactsPerManifestPageV1,
			FirstArtifactID:     RawArtifactIDPrefixV1 + domainsecurity.SHA256Hex([]byte("first:"+strconv.Itoa(index))),
			LastArtifactID:      RawArtifactIDPrefixV1 + domainsecurity.SHA256Hex([]byte("last:"+strconv.Itoa(index))),
			AggregateChunkCount: maxRawArtifactsPerManifestPageV1,
			AggregateRawBytes:   maxRawArtifactsPerManifestPageV1,
		}
		descriptor.DescriptorDigest = rawArtifactManifestPageDescriptorDigestV1(descriptor)
		descriptors[index] = descriptor
	}
	manifest, err := newRawArtifactManifestV1(intent, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	body, err := RawArtifactManifestV1Bytes(manifest)
	if err != nil || len(body) > maxRawArtifactManifestBytesV1 || manifest.ArtifactCount != maxRawArtifactCountV1 ||
		manifest.AggregateChunkCount != maxRawArtifactChunkCountV1 {
		t.Fatalf("bounded manifest root cannot cover maximum chunk population: bytes=%d artifacts=%d chunks=%d err=%v",
			len(body), manifest.ArtifactCount, manifest.AggregateChunkCount, err)
	}
	if _, err := ParseRawArtifactManifestV1(body); err != nil {
		t.Fatal(err)
	}
	if _, err := newRawArtifactManifestV1(intent, append(descriptors, descriptors[0])); err == nil {
		t.Fatal("manifest page bound +1 was accepted")
	}
}

func rawArtifactHierarchyForTestV1(t *testing.T, count int, seed string) rawArtifactHierarchyTestV1 {
	t.Helper()
	if count <= 0 || count > maxRawArtifactCountV1 {
		t.Fatalf("invalid raw artifact test count: %d", count)
	}
	binding := sourceRowTestBindingV1(t, "case-raw-"+seed)
	intent, err := newRawArtifactAcquisitionIntentV1(
		binding,
		time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC),
		domainsecurity.SHA256Hex([]byte("raw-actor:"+seed)),
		uint64(count),
		domainsecurity.SHA256Hex([]byte("raw-intent-nonce:"+seed)),
	)
	if err != nil {
		t.Fatal(err)
	}
	locators := make([]RawArtifactSourceLocatorV1, count)
	roots := make([]RawArtifactContentRootV1, count)
	entries := make([]RawArtifactEntryV1, count)
	for index := 0; index < count; index++ {
		sourceFileID := domainsecurity.SHA256Hex([]byte(fmt.Sprintf("raw-file:%s:%d", seed, index)))[:20]
		locator, err := newRawArtifactSourceLocatorV1(intent, uint64(index+1), sourceFileID)
		if err != nil {
			t.Fatal(err)
		}
		content := []byte(fmt.Sprintf("opaque-source:%s:%d", seed, index))
		chunk, err := newRawArtifactContentChunkDescriptorV1(
			intent.BindingKeyDigest, locator.LocatorDigest, 1, 0, content,
		)
		if err != nil {
			t.Fatal(err)
		}
		contentPage, err := newRawArtifactContentIndexPageV1(
			intent.BindingKeyDigest, locator.LocatorDigest, 1, []RawArtifactContentChunkDescriptorV1{chunk},
		)
		if err != nil {
			t.Fatal(err)
		}
		contentDescriptor, err := newRawArtifactContentIndexPageDescriptorV1(contentPage)
		if err != nil {
			t.Fatal(err)
		}
		root, err := newRawArtifactContentRootV1(
			intent.BindingKeyDigest, locator.LocatorDigest, domainsecurity.SHA256Hex(content),
			[]RawArtifactContentIndexPageDescriptorV1{contentDescriptor},
		)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := newRawArtifactEntryV1(intent, locator, root)
		if err != nil {
			t.Fatal(err)
		}
		locators[index] = locator
		roots[index] = root
		entries[index] = entry
	}
	pageCount := (count + maxRawArtifactsPerManifestPageV1 - 1) / maxRawArtifactsPerManifestPageV1
	pages := make([]RawArtifactManifestPageV1, pageCount)
	descriptors := make([]RawArtifactManifestPageDescriptorV1, pageCount)
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		start := pageIndex * maxRawArtifactsPerManifestPageV1
		end := start + maxRawArtifactsPerManifestPageV1
		if end > count {
			end = count
		}
		page, err := newRawArtifactManifestPageV1(intent, uint64(pageIndex+1), entries[start:end])
		if err != nil {
			t.Fatal(err)
		}
		descriptor, err := newRawArtifactManifestPageDescriptorV1(page)
		if err != nil {
			t.Fatal(err)
		}
		pages[pageIndex] = page
		descriptors[pageIndex] = descriptor
	}
	manifest, err := newRawArtifactManifestV1(intent, descriptors)
	if err != nil {
		t.Fatal(err)
	}
	return rawArtifactHierarchyTestV1{
		intent: intent, locators: locators, contentRoots: roots, entries: entries, pages: pages, manifest: manifest,
	}
}

func cloneRawArtifactManifestPageForTestV1(t *testing.T, page RawArtifactManifestPageV1) RawArtifactManifestPageV1 {
	t.Helper()
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var clone RawArtifactManifestPageV1
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
