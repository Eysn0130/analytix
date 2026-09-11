package rawartifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
)

const (
	StagedHierarchySchemaVersionV1 = 1
	StagedHierarchyPurposeV1       = "analytix.raw-artifact-staged-hierarchy/v1"
	StagedHierarchyAuthorityNoneV1 = "staged_structural_only"
)

// StagedHierarchyV1 is an internal completion summary, not a receipt. It is
// intentionally incapable of authorizing claims, snapshot currentness, or
// publication. A later admission commit receipt must exact-read the staged
// hierarchy and then be incorporated into witnessed DSV2 authority.
type StagedHierarchyV1 struct {
	schemaVersion                 int
	purpose                       string
	authorityClass                string
	factAnswerAllowed             bool
	bindingKeyDigest              string
	acquisitionIntentDigest       string
	rawArtifactManifestDigest     string
	rawArtifactManifestSHA256     string
	rawArtifactManifestByteLength uint64
	validatedArtifactCount        uint64
	validatedSourceLocatorCount   uint64
	validatedContentRootCount     uint64
	validatedIndexPageCount       uint64
	validatedChunkCount           uint64
	validatedRawBytes             uint64
}

func (StagedHierarchyV1) CanAuthorizeFacts() bool { return false }

// VerifyAndStage consumes every object in one parent-addressed untrusted
// candidate, recomputes every claimed artifact's full SHA-256, enforces global
// source uniqueness, and stages exact read-back-verified objects. It is an
// import verifier, not the production host-acquisition authority: a candidate
// chooses its own claimed inventory, so this function cannot prove that an
// authenticated host selection omitted no file. Failure can leave inert CAS
// objects but can never return a completion summary or factual authority.
func VerifyAndStage(
	ctx context.Context,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	source rawartifactport.CandidateSource,
	store rawartifactport.StagingStore,
) (StagedHierarchyV1, error) {
	if ctx == nil || domainsecurity.ValidateDatasetSnapshotBindingKeyV1(binding) != nil || source == nil || store == nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact admission input is invalid"))
	}
	if err := ctx.Err(); err != nil {
		return StagedHierarchyV1{}, err
	}
	intent, err := source.AcquisitionIntent(ctx)
	if err != nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
	}
	if domainevidence.ValidateRawArtifactAcquisitionIntentV1(intent) != nil ||
		intent.BindingKeyDigest != binding.BindingKeyDigest {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact intent binding is invalid"))
	}
	if err := store.StageAcquisitionIntentExact(ctx, intent); err != nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
	}

	manifest, err := source.Manifest(ctx, intent)
	if err != nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
	}
	if domainevidence.ValidateRawArtifactManifestForIntentV1(intent, manifest) != nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact manifest does not match its intent"))
	}
	seenArtifactIDs := make(map[string]struct{}, int(manifest.ArtifactCount))
	seenLocators := make(map[string]struct{}, int(manifest.ArtifactCount))
	seenSourceFiles := make(map[string]struct{}, int(manifest.ArtifactCount))

	var artifactCount uint64
	var locatorCount uint64
	var contentRootCount uint64
	var indexPageCount uint64
	var chunkCount uint64
	var rawBytes uint64
	for pageIndex, pageDescriptor := range manifest.PageDescriptors {
		if err := ctx.Err(); err != nil {
			return StagedHierarchyV1{}, err
		}
		page, err := source.ManifestPage(ctx, pageDescriptor)
		if err != nil {
			return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
		}
		if page.ManifestPageNumber != uint64(pageIndex+1) ||
			domainevidence.ValidateRawArtifactManifestPageMembershipV1(manifest, page) != nil {
			return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact manifest page is mismatched"))
		}
		for _, entry := range page.Entries {
			if err := ctx.Err(); err != nil {
				return StagedHierarchyV1{}, err
			}
			artifactCount++
			if entry.ArtifactOrdinal != artifactCount {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact global order is invalid"))
			}
			if _, exists := seenArtifactIDs[entry.ArtifactID]; exists {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact id is duplicated"))
			}
			if _, exists := seenLocators[entry.SourceLocatorDigest]; exists {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact source locator is duplicated"))
			}
			if _, exists := seenSourceFiles[entry.SourceFileIDDigest]; exists {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact source file id is duplicated"))
			}
			seenArtifactIDs[entry.ArtifactID] = struct{}{}
			seenLocators[entry.SourceLocatorDigest] = struct{}{}
			seenSourceFiles[entry.SourceFileIDDigest] = struct{}{}

			locator, err := source.SourceLocator(ctx, entry)
			if err != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
			}
			if domainevidence.ValidateRawArtifactEntryAgainstSourceLocatorV1(intent, entry, locator) != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact source locator is mismatched"))
			}
			if err := store.StageSourceLocatorExact(ctx, locator); err != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
			}
			locatorCount++

			contentRoot, err := source.ContentRoot(ctx, entry)
			if err != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
			}
			if domainevidence.ValidateRawArtifactEntryWithContentRootV1(entry, contentRoot) != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact content root is mismatched"))
			}
			fullHasher := sha256.New()
			var artifactChunks uint64
			var artifactBytes uint64
			for _, indexDescriptor := range contentRoot.IndexPageDescriptors {
				if err := ctx.Err(); err != nil {
					return StagedHierarchyV1{}, err
				}
				indexPage, err := source.ContentIndexPage(ctx, indexDescriptor)
				if err != nil {
					return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
				}
				if domainevidence.ValidateRawArtifactContentIndexPageMembershipV1(contentRoot, indexPage) != nil {
					return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrMismatch, errors.New("raw artifact content index page is mismatched"))
				}
				for _, chunkDescriptor := range indexPage.ChunkDescriptors {
					body, err := readCandidateChunkExactV1(ctx, source, chunkDescriptor)
					if err != nil {
						if errors.Is(err, rawartifactport.ErrCorrupt) {
							return StagedHierarchyV1{}, err
						}
						return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
					}
					if domainevidence.ValidateRawArtifactContentChunkBytesV1(chunkDescriptor, body) != nil {
						return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk bytes are mismatched"))
					}
					_, _ = fullHasher.Write(body)
					artifactChunks++
					artifactBytes += uint64(len(body))
					if err := store.StageChunkExact(ctx, chunkDescriptor, body); err != nil {
						return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
					}
				}
				if err := store.StageContentIndexPageExact(ctx, indexPage); err != nil {
					return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
				}
				indexPageCount++
			}
			if artifactChunks != contentRoot.ChunkCount || artifactBytes != contentRoot.ArtifactByteLength ||
				hex.EncodeToString(fullHasher.Sum(nil)) != contentRoot.FullSHA256 {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact full content is incomplete"))
			}
			if err := store.StageContentRootExact(ctx, contentRoot); err != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
			}
			if err := store.StageEntryExact(ctx, entry); err != nil {
				return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
			}
			contentRootCount++
			chunkCount += artifactChunks
			rawBytes += artifactBytes
		}
		if err := store.StageManifestPageExact(ctx, page); err != nil {
			return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
		}
	}
	if artifactCount != manifest.ArtifactCount || artifactCount != intent.ExpectedArtifactCount ||
		locatorCount != artifactCount || contentRootCount != artifactCount ||
		chunkCount != manifest.AggregateChunkCount || rawBytes != manifest.AggregateRawBytes {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact hierarchy is incomplete"))
	}
	if err := store.StageManifestExact(ctx, manifest); err != nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrUnavailable, err)
	}
	manifestBody, err := domainevidence.RawArtifactManifestV1Bytes(manifest)
	if err != nil {
		return StagedHierarchyV1{}, errors.Join(rawartifactport.ErrCorrupt, err)
	}
	return StagedHierarchyV1{
		schemaVersion: StagedHierarchySchemaVersionV1, purpose: StagedHierarchyPurposeV1,
		authorityClass: StagedHierarchyAuthorityNoneV1, factAnswerAllowed: false,
		bindingKeyDigest: binding.BindingKeyDigest, acquisitionIntentDigest: intent.IntentDigest,
		rawArtifactManifestDigest:     manifest.ManifestDigest,
		rawArtifactManifestSHA256:     domainsecurity.SHA256Hex(manifestBody),
		rawArtifactManifestByteLength: uint64(len(manifestBody)),
		validatedArtifactCount:        artifactCount, validatedSourceLocatorCount: locatorCount,
		validatedContentRootCount: contentRootCount, validatedIndexPageCount: indexPageCount,
		validatedChunkCount: chunkCount, validatedRawBytes: rawBytes,
	}, nil
}

func readCandidateChunkExactV1(
	ctx context.Context,
	source rawartifactport.CandidateSource,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
) ([]byte, error) {
	if ctx == nil || source == nil ||
		domainevidence.ValidateRawArtifactContentChunkDescriptorV1(descriptor) != nil ||
		descriptor.ChunkByteLength > domainevidence.RawArtifactContentChunkBytesV1 {
		return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk descriptor is invalid"))
	}
	reader, err := source.OpenChunk(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk reader is unavailable"))
	}
	defer func() { _ = reader.Close() }()

	body := make([]byte, int(descriptor.ChunkByteLength))
	read := 0
	for read < len(body) {
		if err := ctx.Err(); err != nil {
			clear(body)
			return nil, err
		}
		count, readErr := reader.Read(body[read:])
		if count < 0 || read+count > len(body) {
			clear(body)
			return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk reader returned an invalid count"))
		}
		read += count
		if errors.Is(readErr, io.EOF) {
			if read != len(body) {
				clear(body)
				return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk ended before its declared length"))
			}
			break
		}
		if readErr != nil {
			clear(body)
			return nil, readErr
		}
		if count == 0 {
			clear(body)
			return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk reader made no progress"))
		}
	}
	var extra [1]byte
	extraCount, extraErr := reader.Read(extra[:])
	if extraCount != 0 || !errors.Is(extraErr, io.EOF) {
		clear(body)
		return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk exceeds its declared length"))
	}
	if domainevidence.ValidateRawArtifactContentChunkBytesV1(descriptor, body) != nil {
		clear(body)
		return nil, errors.Join(rawartifactport.ErrCorrupt, errors.New("raw artifact chunk bytes are mismatched"))
	}
	return body, nil
}
