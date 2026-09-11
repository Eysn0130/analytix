package rawartifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	rawartifactport "analytix.local/runtime-go/internal/ports/rawartifact"
	rawartifactfixture "analytix.local/runtime-go/internal/testsupport/rawartifactfixture"
)

func TestVerifyAndStageConsumesEveryExactRawArtifactAndReturnsNoAuthority(t *testing.T) {
	hierarchy := rawArtifactAdmissionHierarchyV1(t, "complete", 2)
	source := &rawArtifactCandidateForTestV1{hierarchy: hierarchy}
	store := newRawArtifactStagingStoreForTestV1()
	result, err := VerifyAndStage(context.Background(), hierarchy.Binding, source, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.schemaVersion != StagedHierarchySchemaVersionV1 || result.purpose != StagedHierarchyPurposeV1 ||
		result.authorityClass != StagedHierarchyAuthorityNoneV1 || result.factAnswerAllowed || result.CanAuthorizeFacts() ||
		result.bindingKeyDigest != hierarchy.Binding.BindingKeyDigest ||
		result.acquisitionIntentDigest != hierarchy.Intent.IntentDigest ||
		result.rawArtifactManifestDigest != hierarchy.Manifest.ManifestDigest ||
		result.validatedArtifactCount != 2 || result.validatedSourceLocatorCount != 2 ||
		result.validatedContentRootCount != 2 || result.validatedIndexPageCount != 2 ||
		result.validatedChunkCount != 2 || result.validatedRawBytes != hierarchy.Manifest.AggregateRawBytes {
		t.Fatalf("unexpected raw artifact staged completion: %#v", result)
	}
	if got := store.lastStage(); got != "manifest:"+hierarchy.Manifest.ManifestDigest {
		t.Fatalf("manifest was not the final inert staged object: %s", got)
	}
	if source.chunkReads != hierarchy.Manifest.AggregateChunkCount {
		t.Fatalf("not every exact chunk was consumed: got=%d want=%d", source.chunkReads, hierarchy.Manifest.AggregateChunkCount)
	}
}

func TestVerifyAndStageRejectsCrossBindingCorruptChunkAndMissingMaterial(t *testing.T) {
	hierarchy := rawArtifactAdmissionHierarchyV1(t, "hostile", 2)

	t.Run("cross binding", func(t *testing.T) {
		other, err := domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			WorkspaceRealPath: "/workspace", CaseID: "case-other",
			CaseBindingHash:          domainsecurity.SHA256Hex([]byte("other-binding")),
			BindingObservationDigest: domainsecurity.SHA256Hex([]byte("other-observation")),
		})
		if err != nil {
			t.Fatal(err)
		}
		store := newRawArtifactStagingStoreForTestV1()
		if _, err := VerifyAndStage(
			context.Background(), other, &rawArtifactCandidateForTestV1{hierarchy: hierarchy}, store,
		); !errors.Is(err, rawartifactport.ErrMismatch) || len(store.sequence) != 0 {
			t.Fatalf("cross-binding intent was not rejected before staging: sequence=%v err=%v", store.sequence, err)
		}
	})

	t.Run("corrupt chunk", func(t *testing.T) {
		source := &rawArtifactCandidateForTestV1{hierarchy: hierarchy, corruptChunk: true}
		store := newRawArtifactStagingStoreForTestV1()
		if _, err := VerifyAndStage(context.Background(), hierarchy.Binding, source, store); !errors.Is(err, rawartifactport.ErrCorrupt) {
			t.Fatalf("corrupt chunk was not rejected deterministically: %v", err)
		}
		if store.hasStagePrefix("manifest:") {
			t.Fatal("corrupt raw hierarchy staged its manifest completion object")
		}
	})

	for _, test := range []struct {
		name string
		mode string
	}{
		{name: "short chunk", mode: "short"},
		{name: "extra chunk", mode: "extra"},
		{name: "zero progress", mode: "zero-progress"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &rawArtifactCandidateForTestV1{hierarchy: hierarchy, chunkReadMode: test.mode}
			store := newRawArtifactStagingStoreForTestV1()
			if _, err := VerifyAndStage(context.Background(), hierarchy.Binding, source, store); !errors.Is(err, rawartifactport.ErrCorrupt) {
				t.Fatalf("%s was not rejected deterministically: %v", test.name, err)
			}
			if store.hasStagePrefix("manifest:") {
				t.Fatalf("%s staged its manifest completion object", test.name)
			}
		})
	}

	t.Run("missing page", func(t *testing.T) {
		source := &rawArtifactCandidateForTestV1{hierarchy: hierarchy, missingManifestPage: true}
		store := newRawArtifactStagingStoreForTestV1()
		if _, err := VerifyAndStage(context.Background(), hierarchy.Binding, source, store); !errors.Is(err, rawartifactport.ErrUnavailable) {
			t.Fatalf("missing exact page was not unavailable: %v", err)
		}
		if store.hasStagePrefix("manifest:") {
			t.Fatal("partial raw hierarchy staged its manifest completion object")
		}
	})
}

func TestVerifyAndStageACKLossLeavesOnlyInertObjectsAndRetryCompletes(t *testing.T) {
	hierarchy := rawArtifactAdmissionHierarchyV1(t, "ack-loss", 1)
	source := &rawArtifactCandidateForTestV1{hierarchy: hierarchy}
	store := newRawArtifactStagingStoreForTestV1()
	store.failOnceAt = "entry:"
	if _, err := VerifyAndStage(context.Background(), hierarchy.Binding, source, store); !errors.Is(err, rawartifactport.ErrUnavailable) {
		t.Fatalf("staging ACK loss was not surfaced: %v", err)
	}
	if store.hasStagePrefix("manifest:") {
		t.Fatal("failed admission exposed a manifest completion object")
	}
	result, err := VerifyAndStage(context.Background(), hierarchy.Binding, source, store)
	if err != nil || result.factAnswerAllowed || result.CanAuthorizeFacts() || result.validatedArtifactCount != 1 {
		t.Fatalf("idempotent exact retry did not complete safely: result=%#v err=%v", result, err)
	}
}

func TestVerifyAndStageCancellationNeverStagesManifest(t *testing.T) {
	hierarchy := rawArtifactAdmissionHierarchyV1(t, "cancel", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newRawArtifactStagingStoreForTestV1()
	if _, err := VerifyAndStage(ctx, hierarchy.Binding, &rawArtifactCandidateForTestV1{hierarchy: hierarchy}, store); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was hidden: %v", err)
	}
	if len(store.sequence) != 0 {
		t.Fatalf("canceled admission staged objects: %v", store.sequence)
	}
}

type rawArtifactCandidateForTestV1 struct {
	hierarchy           rawartifactfixture.HierarchyV1
	corruptChunk        bool
	chunkReadMode       string
	missingManifestPage bool
	chunkReads          uint64
}

func (source *rawArtifactCandidateForTestV1) AcquisitionIntent(context.Context) (domainevidence.RawArtifactAcquisitionIntentV1, error) {
	return source.hierarchy.Intent, nil
}

func (source *rawArtifactCandidateForTestV1) Manifest(
	_ context.Context,
	intent domainevidence.RawArtifactAcquisitionIntentV1,
) (domainevidence.RawArtifactManifestV1, error) {
	if intent != source.hierarchy.Intent {
		return domainevidence.RawArtifactManifestV1{}, errors.New("intent mismatch")
	}
	return source.hierarchy.Manifest, nil
}

func (source *rawArtifactCandidateForTestV1) ManifestPage(
	_ context.Context,
	descriptor domainevidence.RawArtifactManifestPageDescriptorV1,
) (domainevidence.RawArtifactManifestPageV1, error) {
	if source.missingManifestPage {
		return domainevidence.RawArtifactManifestPageV1{}, errors.New("manifest page unavailable")
	}
	for index, candidate := range source.hierarchy.Manifest.PageDescriptors {
		if candidate == descriptor {
			return source.hierarchy.ManifestPages[index], nil
		}
	}
	return domainevidence.RawArtifactManifestPageV1{}, errors.New("manifest page reference mismatch")
}

func (source *rawArtifactCandidateForTestV1) SourceLocator(
	_ context.Context,
	entry domainevidence.RawArtifactEntryV1,
) (domainevidence.RawArtifactSourceLocatorV1, error) {
	for index, candidate := range source.hierarchy.Entries {
		if candidate == entry {
			return source.hierarchy.Locators[index], nil
		}
	}
	return domainevidence.RawArtifactSourceLocatorV1{}, errors.New("source locator reference mismatch")
}

func (source *rawArtifactCandidateForTestV1) ContentRoot(
	_ context.Context,
	entry domainevidence.RawArtifactEntryV1,
) (domainevidence.RawArtifactContentRootV1, error) {
	for index, candidate := range source.hierarchy.Entries {
		if candidate == entry {
			return source.hierarchy.ContentRoots[index], nil
		}
	}
	return domainevidence.RawArtifactContentRootV1{}, errors.New("content root reference mismatch")
}

func (source *rawArtifactCandidateForTestV1) ContentIndexPage(
	_ context.Context,
	descriptor domainevidence.RawArtifactContentIndexPageDescriptorV1,
) (domainevidence.RawArtifactContentIndexPageV1, error) {
	for index, root := range source.hierarchy.ContentRoots {
		if len(root.IndexPageDescriptors) == 1 && root.IndexPageDescriptors[0] == descriptor {
			return source.hierarchy.ContentPages[index], nil
		}
	}
	return domainevidence.RawArtifactContentIndexPageV1{}, errors.New("content page reference mismatch")
}

func (source *rawArtifactCandidateForTestV1) OpenChunk(
	_ context.Context,
	descriptor domainevidence.RawArtifactContentChunkDescriptorV1,
) (io.ReadCloser, error) {
	for index, page := range source.hierarchy.ContentPages {
		if len(page.ChunkDescriptors) == 1 && page.ChunkDescriptors[0] == descriptor {
			source.chunkReads++
			body := append([]byte(nil), source.hierarchy.ChunkBodies[index]...)
			if source.corruptChunk {
				body[0] ^= 0xff
			}
			switch source.chunkReadMode {
			case "short":
				body = body[:len(body)-1]
			case "extra":
				body = append(body, 0)
			case "zero-progress":
				return io.NopCloser(zeroProgressRawArtifactReaderV1{}), nil
			}
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return nil, errors.New("chunk reference mismatch")
}

type zeroProgressRawArtifactReaderV1 struct{}

func (zeroProgressRawArtifactReaderV1) Read([]byte) (int, error) { return 0, nil }

type rawArtifactStagingStoreForTestV1 struct {
	mu         sync.Mutex
	sequence   []string
	staged     map[string][]byte
	failOnceAt string
	failed     bool
}

func newRawArtifactStagingStoreForTestV1() *rawArtifactStagingStoreForTestV1 {
	return &rawArtifactStagingStoreForTestV1{staged: map[string][]byte{}}
}

func (store *rawArtifactStagingStoreForTestV1) stage(key string, body []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, ok := store.staged[key]; ok && string(current) != string(body) {
		return errors.New("staging conflict")
	}
	store.staged[key] = append([]byte(nil), body...)
	store.sequence = append(store.sequence, key)
	if !store.failed && store.failOnceAt != "" && len(key) >= len(store.failOnceAt) && key[:len(store.failOnceAt)] == store.failOnceAt {
		store.failed = true
		return errors.New("simulated ACK loss")
	}
	return nil
}

func (store *rawArtifactStagingStoreForTestV1) StageAcquisitionIntentExact(
	_ context.Context, value domainevidence.RawArtifactAcquisitionIntentV1,
) error {
	body, err := domainevidence.RawArtifactAcquisitionIntentV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("intent:"+value.IntentDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageSourceLocatorExact(
	_ context.Context, value domainevidence.RawArtifactSourceLocatorV1,
) error {
	body, err := domainevidence.RawArtifactSourceLocatorV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("locator:"+value.LocatorDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageChunkExact(
	_ context.Context, descriptor domainevidence.RawArtifactContentChunkDescriptorV1, body []byte,
) error {
	if err := domainevidence.ValidateRawArtifactContentChunkBytesV1(descriptor, body); err != nil {
		return err
	}
	return store.stage("chunk:"+descriptor.DescriptorDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageContentIndexPageExact(
	_ context.Context, value domainevidence.RawArtifactContentIndexPageV1,
) error {
	body, err := domainevidence.RawArtifactContentIndexPageV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("content-page:"+value.IndexPageDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageContentRootExact(
	_ context.Context, value domainevidence.RawArtifactContentRootV1,
) error {
	body, err := domainevidence.RawArtifactContentRootV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("content-root:"+value.RootDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageEntryExact(
	_ context.Context, value domainevidence.RawArtifactEntryV1,
) error {
	body, err := domainevidence.RawArtifactEntryV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("entry:"+value.EntryDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageManifestPageExact(
	_ context.Context, value domainevidence.RawArtifactManifestPageV1,
) error {
	body, err := domainevidence.RawArtifactManifestPageV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("manifest-page:"+value.PageDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) StageManifestExact(
	_ context.Context, value domainevidence.RawArtifactManifestV1,
) error {
	body, err := domainevidence.RawArtifactManifestV1Bytes(value)
	if err != nil {
		return err
	}
	return store.stage("manifest:"+value.ManifestDigest, body)
}

func (store *rawArtifactStagingStoreForTestV1) lastStage() string {
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.sequence) == 0 {
		return ""
	}
	return store.sequence[len(store.sequence)-1]
}

func (store *rawArtifactStagingStoreForTestV1) hasStagePrefix(prefix string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, key := range store.sequence {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func rawArtifactAdmissionHierarchyV1(t *testing.T, seed string, count int) rawartifactfixture.HierarchyV1 {
	t.Helper()
	hierarchy, err := rawartifactfixture.BuildV1(seed, count)
	if err != nil {
		t.Fatal(err)
	}
	return hierarchy
}
