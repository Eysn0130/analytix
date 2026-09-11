package rawartifactfixture

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const maxArtifactsPerManifestPageV1 = 256

type HierarchyV1 struct {
	Binding       domainsecurity.DatasetSnapshotBindingKeyV1
	Intent        domainevidence.RawArtifactAcquisitionIntentV1
	Locators      []domainevidence.RawArtifactSourceLocatorV1
	ChunkBodies   [][]byte
	ContentPages  []domainevidence.RawArtifactContentIndexPageV1
	ContentRoots  []domainevidence.RawArtifactContentRootV1
	Entries       []domainevidence.RawArtifactEntryV1
	ManifestPages []domainevidence.RawArtifactManifestPageV1
	Manifest      domainevidence.RawArtifactManifestV1
}

// BuildV1 is an independent test oracle. It intentionally does not use the
// package-private production builders whose call surface architecture tests
// keep closed.
func BuildV1(seed string, count int) (HierarchyV1, error) {
	if seed == "" || count <= 0 || count > 1024 {
		return HierarchyV1{}, errors.New("raw artifact fixture input is invalid")
	}
	binding, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		WorkspaceRealPath: "/workspace", CaseID: "case-fixture-" + seed,
		CaseBindingHash:          domainsecurity.SHA256Hex([]byte("fixture-binding:" + seed)),
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("fixture-observation:" + seed)),
	})
	if err != nil {
		return HierarchyV1{}, err
	}
	intent := domainevidence.RawArtifactAcquisitionIntentV1{
		SchemaVersion:          domainevidence.RawArtifactAcquisitionIntentSchemaVersionV1,
		Purpose:                domainevidence.RawArtifactAcquisitionIntentPurposeV1,
		BindingKeyDigest:       binding.BindingKeyDigest,
		AcquisitionMethod:      domainevidence.RawArtifactAcquisitionMethodV1,
		AcquiredAt:             time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		AcquisitionActorDigest: domainsecurity.SHA256Hex([]byte("fixture-actor:" + seed)),
		ExpectedArtifactCount:  uint64(count), IntentNonceDigest: domainsecurity.SHA256Hex([]byte("fixture-nonce:" + seed)),
	}
	intent.IntentDigest = fixtureDigestV1("analytix.raw-artifact-acquisition-intent/digest/v1\x00", intent)
	if err := domainevidence.ValidateRawArtifactAcquisitionIntentV1(intent); err != nil {
		return HierarchyV1{}, err
	}

	hierarchy := HierarchyV1{Binding: binding, Intent: intent}
	hierarchy.Locators = make([]domainevidence.RawArtifactSourceLocatorV1, count)
	hierarchy.ChunkBodies = make([][]byte, count)
	hierarchy.ContentPages = make([]domainevidence.RawArtifactContentIndexPageV1, count)
	hierarchy.ContentRoots = make([]domainevidence.RawArtifactContentRootV1, count)
	hierarchy.Entries = make([]domainevidence.RawArtifactEntryV1, count)
	for index := 0; index < count; index++ {
		sourceFileID := domainsecurity.SHA256Hex([]byte(fmt.Sprintf("fixture-file:%s:%d", seed, index)))[:20]
		locator := domainevidence.RawArtifactSourceLocatorV1{
			SchemaVersion:    domainevidence.RawArtifactSourceLocatorSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactSourceLocatorPurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, AcquisitionIntentDigest: intent.IntentDigest,
			SourceOccurrenceOrdinal: uint64(index + 1),
			SourceFileIDDigest:      fixtureBytesDigestV1("analytix.source-row-source-file-id/digest/v1\x00", []byte(sourceFileID)),
		}
		locator.LocatorDigest = fixtureDigestV1("analytix.raw-artifact-source-locator/digest/v1\x00", locator)
		content := []byte(fmt.Sprintf("opaque-fixture:%s:%d", seed, index))
		chunk := domainevidence.RawArtifactContentChunkDescriptorV1{
			SchemaVersion:    domainevidence.RawArtifactContentChunkDescriptorSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactContentChunkDescriptorPurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, SourceLocatorDigest: locator.LocatorDigest,
			ChunkNumber: 1, ByteOffset: 0, ChunkSHA256: domainsecurity.SHA256Hex(content), ChunkByteLength: uint64(len(content)),
		}
		chunk.DescriptorDigest = fixtureDigestV1("analytix.raw-artifact-content-chunk-descriptor/digest/v1\x00", chunk)
		contentPage := domainevidence.RawArtifactContentIndexPageV1{
			SchemaVersion:    domainevidence.RawArtifactContentIndexPageSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactContentIndexPagePurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, SourceLocatorDigest: locator.LocatorDigest,
			IndexPageNumber: 1, ChunkDescriptors: []domainevidence.RawArtifactContentChunkDescriptorV1{chunk},
			ChunkCount: 1, AggregateChunkBytes: uint64(len(content)),
		}
		contentPage.IndexPageDigest = fixtureDigestV1("analytix.raw-artifact-content-index-page/digest/v1\x00", contentPage)
		contentPageBody, _ := json.Marshal(contentPage)
		contentDescriptor := domainevidence.RawArtifactContentIndexPageDescriptorV1{
			SchemaVersion:    domainevidence.RawArtifactContentIndexDescriptorSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactContentIndexDescriptorPurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, SourceLocatorDigest: locator.LocatorDigest,
			IndexPageNumber: 1, IndexPageDigest: contentPage.IndexPageDigest,
			IndexPageSHA256: domainsecurity.SHA256Hex(contentPageBody), IndexPageByteLength: uint64(len(contentPageBody)),
			FirstChunkNumber: 1, LastChunkNumber: 1, ChunkCount: 1,
			FirstByteOffset: 0, LastByteExclusive: uint64(len(content)), AggregateChunkBytes: uint64(len(content)),
		}
		contentDescriptor.DescriptorDigest = fixtureDigestV1(
			"analytix.raw-artifact-content-index-page-descriptor/digest/v1\x00", contentDescriptor,
		)
		root := domainevidence.RawArtifactContentRootV1{
			SchemaVersion:    domainevidence.RawArtifactContentRootSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactContentRootPurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, SourceLocatorDigest: locator.LocatorDigest,
			IndexPageDescriptors: []domainevidence.RawArtifactContentIndexPageDescriptorV1{contentDescriptor},
			IndexPageCount:       1, ChunkCount: 1, ArtifactByteLength: uint64(len(content)),
			FullSHA256: domainsecurity.SHA256Hex(content),
		}
		root.RootDigest = fixtureDigestV1("analytix.raw-artifact-content-root/digest/v1\x00", root)
		locatorBody, _ := json.Marshal(locator)
		rootBody, _ := json.Marshal(root)
		entry := domainevidence.RawArtifactEntryV1{
			SchemaVersion:   domainevidence.RawArtifactEntrySchemaVersionV1,
			Purpose:         domainevidence.RawArtifactEntryPurposeV1,
			ArtifactOrdinal: uint64(index + 1), BindingKeyDigest: binding.BindingKeyDigest,
			AcquisitionIntentDigest: intent.IntentDigest, SourceFileIDDigest: locator.SourceFileIDDigest,
			SourceKind:          domainevidence.RawArtifactSourceKindCaseImportV1,
			MediaType:           domainevidence.RawArtifactMediaTypeOpaqueV1,
			SourceLocatorDigest: locator.LocatorDigest, SourceLocatorSHA256: domainsecurity.SHA256Hex(locatorBody),
			SourceLocatorByteLength: uint64(len(locatorBody)), OriginKind: domainevidence.RawArtifactOriginDirectImportV1,
			ContentRootDigest: root.RootDigest, ContentRootSHA256: domainsecurity.SHA256Hex(rootBody),
			ContentRootByteLength: uint64(len(rootBody)), FullSHA256: root.FullSHA256,
			ArtifactByteLength: root.ArtifactByteLength, ContentChunkCount: root.ChunkCount,
		}
		entry.ArtifactID = fixtureRawArtifactIDV1(entry)
		entry.EntryDigest = fixtureDigestV1("analytix.raw-artifact-entry/digest/v1\x00", entry)
		if domainevidence.ValidateRawArtifactEntryAgainstSourceLocatorV1(intent, entry, locator) != nil ||
			domainevidence.ValidateRawArtifactEntryWithContentRootV1(entry, root) != nil {
			return HierarchyV1{}, errors.New("raw artifact fixture entry is invalid")
		}
		hierarchy.Locators[index] = locator
		hierarchy.ChunkBodies[index] = content
		hierarchy.ContentPages[index] = contentPage
		hierarchy.ContentRoots[index] = root
		hierarchy.Entries[index] = entry
	}

	pageCount := (count + maxArtifactsPerManifestPageV1 - 1) / maxArtifactsPerManifestPageV1
	hierarchy.ManifestPages = make([]domainevidence.RawArtifactManifestPageV1, pageCount)
	descriptors := make([]domainevidence.RawArtifactManifestPageDescriptorV1, pageCount)
	var aggregateChunks uint64
	var aggregateBytes uint64
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		start := pageIndex * maxArtifactsPerManifestPageV1
		end := start + maxArtifactsPerManifestPageV1
		if end > count {
			end = count
		}
		entries := append([]domainevidence.RawArtifactEntryV1(nil), hierarchy.Entries[start:end]...)
		page := domainevidence.RawArtifactManifestPageV1{
			SchemaVersion:    domainevidence.RawArtifactManifestPageSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactManifestPagePurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, AcquisitionIntentDigest: intent.IntentDigest,
			ManifestPageNumber: uint64(pageIndex + 1), Entries: entries, ArtifactCount: uint32(len(entries)),
		}
		for _, entry := range entries {
			page.AggregateChunkCount += entry.ContentChunkCount
			page.AggregateRawBytes += entry.ArtifactByteLength
		}
		page.PageDigest = fixtureDigestV1("analytix.raw-artifact-manifest-page/digest/v1\x00", page)
		pageBody, _ := json.Marshal(page)
		first := entries[0]
		last := entries[len(entries)-1]
		descriptor := domainevidence.RawArtifactManifestPageDescriptorV1{
			SchemaVersion:    domainevidence.RawArtifactManifestPageDescriptorSchemaVersionV1,
			Purpose:          domainevidence.RawArtifactManifestPageDescriptorPurposeV1,
			BindingKeyDigest: binding.BindingKeyDigest, AcquisitionIntentDigest: intent.IntentDigest,
			ManifestPageNumber: uint64(pageIndex + 1), PageDigest: page.PageDigest,
			PageSHA256: domainsecurity.SHA256Hex(pageBody), PageByteLength: uint64(len(pageBody)),
			FirstArtifactOrdinal: first.ArtifactOrdinal, LastArtifactOrdinal: last.ArtifactOrdinal,
			ArtifactCount: uint32(len(entries)), FirstArtifactID: first.ArtifactID, LastArtifactID: last.ArtifactID,
			AggregateChunkCount: page.AggregateChunkCount, AggregateRawBytes: page.AggregateRawBytes,
		}
		descriptor.DescriptorDigest = fixtureDigestV1(
			"analytix.raw-artifact-manifest-page-descriptor/digest/v1\x00", descriptor,
		)
		hierarchy.ManifestPages[pageIndex] = page
		descriptors[pageIndex] = descriptor
		aggregateChunks += page.AggregateChunkCount
		aggregateBytes += page.AggregateRawBytes
	}
	intentBody, _ := json.Marshal(intent)
	manifest := domainevidence.RawArtifactManifestV1{
		SchemaVersion:    domainevidence.RawArtifactManifestSchemaVersionV1,
		Purpose:          domainevidence.RawArtifactManifestPurposeV1,
		BindingKeyDigest: binding.BindingKeyDigest, AcquisitionMethod: intent.AcquisitionMethod,
		AcquiredAt: intent.AcquiredAt, AcquisitionActorDigest: intent.AcquisitionActorDigest,
		AcquisitionIntentDigest: intent.IntentDigest, AcquisitionIntentSHA256: domainsecurity.SHA256Hex(intentBody),
		AcquisitionIntentByteLength: uint64(len(intentBody)), PageDescriptors: descriptors,
		PageCount: uint32(len(descriptors)), ArtifactCount: uint64(count),
		AggregateChunkCount: aggregateChunks, AggregateRawBytes: aggregateBytes,
	}
	manifest.ArtifactSetDigest = fixtureRawArtifactSetDigestV1(manifest)
	manifest.ManifestDigest = fixtureDigestV1("analytix.raw-artifact-manifest/digest/v1\x00", manifest)
	if err := domainevidence.ValidateRawArtifactManifestHierarchyV1(intent, manifest, hierarchy.ManifestPages); err != nil {
		return HierarchyV1{}, err
	}
	hierarchy.Manifest = manifest
	return hierarchy, nil
}

// BuildChunkDescriptorV1 is an independent descriptor oracle for arbitrary
// bounded opaque bytes. It reuses only the fixture's valid frozen binding and
// source locator, then recomputes the descriptor contract independently from
// package-private production constructors.
func BuildChunkDescriptorV1(
	seed string,
	body []byte,
) (domainevidence.RawArtifactContentChunkDescriptorV1, error) {
	if len(body) == 0 || uint64(len(body)) > domainevidence.RawArtifactContentChunkBytesV1 {
		return domainevidence.RawArtifactContentChunkDescriptorV1{}, errors.New("raw artifact chunk fixture body is invalid")
	}
	hierarchy, err := BuildV1(seed, 1)
	if err != nil {
		return domainevidence.RawArtifactContentChunkDescriptorV1{}, err
	}
	descriptor := hierarchy.ContentPages[0].ChunkDescriptors[0]
	descriptor.ChunkSHA256 = domainsecurity.SHA256Hex(body)
	descriptor.ChunkByteLength = uint64(len(body))
	descriptor.DescriptorDigest = ""
	descriptor.DescriptorDigest = fixtureDigestV1(
		"analytix.raw-artifact-content-chunk-descriptor/digest/v1\x00",
		descriptor,
	)
	if err := domainevidence.ValidateRawArtifactContentChunkDescriptorV1(descriptor); err != nil {
		return domainevidence.RawArtifactContentChunkDescriptorV1{}, err
	}
	return descriptor, nil
}

func fixtureDigestV1(domain string, value any) string {
	body, _ := json.Marshal(value)
	return fixtureBytesDigestV1(domain, body)
}

func fixtureBytesDigestV1(domain string, body []byte) string {
	return domainsecurity.SHA256Hex(append(append([]byte(nil), []byte(domain)...), body...))
}

func fixtureRawArtifactIDV1(entry domainevidence.RawArtifactEntryV1) string {
	identity := struct {
		BindingKeyDigest        string `json:"bindingKeyDigest"`
		AcquisitionIntentDigest string `json:"acquisitionIntentDigest"`
		SourceFileIDDigest      string `json:"sourceFileIdDigest"`
		SourceKind              string `json:"sourceKind"`
		SourceLocatorDigest     string `json:"sourceLocatorDigest"`
		FullSHA256              string `json:"fullSha256"`
		ArtifactByteLength      uint64 `json:"artifactByteLength"`
	}{entry.BindingKeyDigest, entry.AcquisitionIntentDigest, entry.SourceFileIDDigest, entry.SourceKind,
		entry.SourceLocatorDigest, entry.FullSHA256, entry.ArtifactByteLength}
	return domainevidence.RawArtifactIDPrefixV1 + fixtureDigestV1("analytix.raw-artifact/id/v1\x00", identity)
}

func fixtureRawArtifactSetDigestV1(manifest domainevidence.RawArtifactManifestV1) string {
	identity := struct {
		BindingKeyDigest    string                                               `json:"bindingKeyDigest"`
		PageDescriptors     []domainevidence.RawArtifactManifestPageDescriptorV1 `json:"pageDescriptors"`
		ArtifactCount       uint64                                               `json:"artifactCount"`
		AggregateChunkCount uint64                                               `json:"aggregateChunkCount"`
		AggregateRawBytes   uint64                                               `json:"aggregateRawBytes"`
	}{manifest.BindingKeyDigest, manifest.PageDescriptors, manifest.ArtifactCount,
		manifest.AggregateChunkCount, manifest.AggregateRawBytes}
	return fixtureDigestV1("analytix.raw-artifact-manifest/set-digest/v1\x00", identity)
}
