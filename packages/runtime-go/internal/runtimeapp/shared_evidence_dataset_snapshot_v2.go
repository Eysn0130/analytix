package runtimeapp

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"

	authorityanchorenv "analytix.local/runtime-go/internal/adapters/outbound/authorityanchorenv"
	authoritycredentialsfs "analytix.local/runtime-go/internal/adapters/outbound/authoritycredentialsfs"
	authoritymanifestfs "analytix.local/runtime-go/internal/adapters/outbound/authoritymanifestfs"
	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceauthoritystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	monotonicheadprojection "analytix.local/runtime-go/internal/adapters/outbound/monotonicheadprojection"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	evidenceregistryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authoritycredentialsport "analytix.local/runtime-go/internal/ports/authoritycredentials"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceregistryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const runtimeAuthorityManifestFileNameV2 = "manifest.json"

type runtimeSharedEvidenceStoresV2 struct {
	bundles      *evidenceauthoritystore.BundleStore
	observations *evidenceauthoritystore.ObservationStore
}

func openRuntimeSharedEvidenceStoresV2(
	dataDir string,
	access finalauthority.SecurePrivateCASAccessAuthority,
) (runtimeSharedEvidenceStoresV2, error) {
	root := filepath.Join(dataDir, "private", "evidence-authority")
	bundles, err := evidenceauthoritystore.NewBundleStore(filepath.Join(root, "bundles"), access)
	if err != nil {
		return runtimeSharedEvidenceStoresV2{}, err
	}
	observations, err := evidenceauthoritystore.NewObservationStore(filepath.Join(root, "observations"), access)
	if err != nil {
		return runtimeSharedEvidenceStoresV2{}, errors.Join(err, bundles.Close())
	}
	return runtimeSharedEvidenceStoresV2{bundles: bundles, observations: observations}, nil
}

func (stores runtimeSharedEvidenceStoresV2) Close() error {
	return errors.Join(stores.observations.Close(), stores.bundles.Close())
}

func (stores runtimeSharedEvidenceStoresV2) hasRecords(ctx context.Context) (bool, error) {
	bundles, err := stores.bundles.HasRecords(ctx)
	if err != nil {
		return false, err
	}
	observations, err := stores.observations.HasRecords(ctx)
	return bundles || observations, err
}

type runtimeSharedEvidenceEnrollmentV2 struct {
	projection  domainenrollment.ManifestAnchorProjectionV2
	credentials authoritycredentialsport.EnrolledWitnessesV1
	witnessKey  []byte
}

type runtimeSharedEvidenceDatasetSnapshotV2 struct {
	evidence      *evidenceauthorityapp.Authority
	snapshot      *datasetsnapshotapp.SealedServiceV2
	registry      *evidenceregistryapp.Service
	registryOwner *runtimeImportActivatedRegistryV1
}

var _ runtimeEvidenceRegistryAuthority = (*evidenceregistryapp.Service)(nil)
var _ evidenceregistryport.FactFinalWitnessIssuer = (*evidenceregistryapp.Service)(nil)

var errRuntimeCaseEvidenceAuthorityUnavailableV1 = errors.New("case evidence authority is unavailable")

// Turn admission cannot use a snapshot when its evidence registry is locally
// unavailable. Reject before the shared witness challenge. Import admission
// retains the underlying snapshot owner so a confirmed import can activate the
// registry; this turn-facing view observes that activation on each request.
type runtimeCaseDatasetSnapshotAuthorityV2 struct {
	snapshot datasetsnapshotport.AuthorityV2
	registry *runtimeImportActivatedRegistryV1
}

func (authority runtimeCaseDatasetSnapshotAuthorityV2) ResolveWitnessedV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if authority.snapshot == nil || authority.registry == nil || authority.registry.CaseEvidenceAuthorityUnavailableV1() {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	return authority.snapshot.ResolveWitnessedV2(ctx, input)
}

// runtimeUnavailableEvidenceRegistryV1 is a stateless host-private capability
// marker. It owns no files and every registry operation fails closed; startup
// preflights and the case finalizer use only its marker method to quarantine
// existing case state and persist a typed boundary without interpreting the
// blocked registry as empty.
type runtimeUnavailableEvidenceRegistryV1 struct{}

var _ runtimeEvidenceRegistryAuthority = runtimeUnavailableEvidenceRegistryV1{}

func (runtimeUnavailableEvidenceRegistryV1) CaseEvidenceAuthorityUnavailableV1() bool { return true }

func (runtimeUnavailableEvidenceRegistryV1) CommitPrepared(context.Context, evidenceregistryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	return domainevidence.EvidenceReceipt{}, errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) Resolve(context.Context, evidenceregistryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	return domainevidence.RegisteredEvidence{}, errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) Revoke(context.Context, evidenceregistryport.RevokeInput) error {
	return errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) Replay(context.Context, domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	return domainevidence.EvidenceReceiptRegistry{}, errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) WithLockedSnapshot(context.Context, domainsecurity.TurnSecurityContext, func(domainevidence.EvidenceReceiptRegistry) error) error {
	return errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) ReplayAt(context.Context, domainsecurity.TurnSecurityContext, uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	return domainevidence.EvidenceReceiptRegistry{}, errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) ListRegistries(context.Context, []domainsecurity.TurnSecurityContext) ([]evidenceregistryport.InventoryRecord, error) {
	return nil, errRuntimeCaseEvidenceAuthorityUnavailableV1
}

func (runtimeUnavailableEvidenceRegistryV1) HasRecords(context.Context) (bool, error) {
	return false, errRuntimeCaseEvidenceAuthorityUnavailableV1
}

// newRuntimeSharedEvidenceDatasetSnapshotV2 composes only local durable and
// credential dependencies. It intentionally performs no Current or Initialize
// call: ordinary process activation never depends on witness availability.
// Every ResolveWitnessedV2, AdmitExactV2 and callback-scoped exact use reaches
// the same EvidenceAuthority, which performs a fresh challenge for that
// protected operation and can recover after a transient outage in-process.
func newRuntimeSharedEvidenceDatasetSnapshotV2(
	ctx context.Context,
	config Config,
	installationAuthority finalauthorityport.Authority,
	snapshotStores *datasetsnapshotstore.StoresV2,
	evidenceStores runtimeSharedEvidenceStoresV2,
	registryAccess finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	bindingObserver casecontextport.Observer,
	registryPreservation *runtimeRegistrySemanticPreservationV1,
) (runtimeSharedEvidenceDatasetSnapshotV2, bool, error) {
	enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(ctx, config, installationAuthority)
	if err != nil || !configured {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, configured, err
	}
	if snapshotStores == nil || evidenceStores.bundles == nil || evidenceStores.observations == nil ||
		registryAccess == nil || bindingObserver == nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, errors.New("runtime shared evidence durable stores are unavailable")
	}
	privateRoot := filepath.Join(config.DataDir, "private")
	floor, err := monotonicheadprojection.New(monotonicheadprojection.Config{
		Root:              filepath.Join(privateRoot, "evidence-checkpoint-floor"),
		InstallationID:    enrolled.projection.InstallationID,
		EnrollmentID:      enrolled.projection.Enrollment.EnrollmentID,
		Namespace:         enrolled.projection.Enrollment.Namespace,
		WitnessKeyID:      enrolled.projection.Enrollment.WitnessKeyID,
		WitnessPublicKey:  enrolled.witnessKey,
		InitialCheckpoint: enrolled.projection.Enrollment.InitialCheckpoint,
	})
	if err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	projection, err := evidenceauthoritystore.NewProjection(
		filepath.Join(privateRoot, "evidence-authority-projection"), evidenceStores.bundles,
	)
	if err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	evidenceAuthority, err := evidenceauthorityapp.New(evidenceauthorityapp.Config{
		InstallationID:   enrolled.projection.InstallationID,
		EnrollmentID:     enrolled.projection.Enrollment.EnrollmentID,
		Authority:        installationAuthority,
		WitnessKeyID:     enrolled.projection.Enrollment.WitnessKeyID,
		WitnessPublicKey: enrolled.witnessKey,
		Random:           cryptorand.Reader,
		Witness:          enrolled.credentials.SharedEvidence,
		CheckpointFloor:  floor,
		Bundles:          evidenceStores.bundles,
		Observations:     evidenceStores.observations,
		Projection:       projection,
		Genesis: evidenceauthorityapp.Genesis{
			DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		},
	})
	if err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	sealed, err := datasetsnapshotapp.NewSealedV2(datasetsnapshotapp.SealedConfigV2{
		InstallationID: enrolled.projection.InstallationID,
		EnrollmentID:   enrolled.projection.Enrollment.EnrollmentID,
		Authority:      installationAuthority,
		Coordinator:    evidenceAuthority,
		LegacyRecords:  snapshotStores.LegacyRecords,
		Bundles:        snapshotStores.AuthorityBundles,
		Indexes:        snapshotStores.Indexes,
		Materials:      snapshotStores.Materials,
		Random:         cryptorand.Reader,
	})
	if err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	registryRoot := filepath.Join(privateRoot, "evidence-registry")
	preparedRegistry, err := evidenceregistrystore.PrepareRecoveryV2(ctx, registryRoot, registryAccess)
	if err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	if registryPreservation != nil && registryPreservation.unavailable {
		files, err := preparedRegistry.SnapshotOriginalFilesV1(ctx)
		if err != nil {
			return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
		}
		if err := registryPreservation.validateHeldV1(files, nil, nil); err != nil {
			return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
		}
		return runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidenceAuthority, snapshot: sealed}, true, nil
	}
	semanticErr := preparedRegistry.ValidateSemantics(ctx)
	if semanticErr != nil {
		if _, domainOnly := semanticErr.(*finalauthority.DomainRecordUnavailableError); !domainOnly {
			return runtimeSharedEvidenceDatasetSnapshotV2{}, true, semanticErr
		}
	}
	if err := preparedRegistry.RevalidatePhysicalV2(ctx); err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	openRegistry := func() (*evidenceregistryapp.Service, func() error, error) {
		indexes, err := evidenceregistrystore.NewAuthorityIndexStoreV2(
			filepath.Join(registryRoot, "indexes"), registryAccess,
		)
		if err != nil {
			return nil, nil, err
		}
		capsules, err := evidenceregistrystore.NewAuthorityCapsuleStoreV2(
			filepath.Join(registryRoot, "capsules"), registryAccess,
		)
		if err != nil {
			return nil, nil, errors.Join(err, indexes.Close())
		}
		closeStores := func() error { return errors.Join(indexes.Close(), capsules.Close()) }
		registry, err := evidenceregistryapp.New(evidenceregistryapp.Config{
			InstallationID:   enrolled.projection.InstallationID,
			EnrollmentID:     enrolled.projection.Enrollment.EnrollmentID,
			Authority:        installationAuthority,
			WitnessKeyID:     enrolled.projection.Enrollment.WitnessKeyID,
			WitnessKey:       enrolled.witnessKey,
			Coordinator:      evidenceAuthority,
			WitnessChain:     evidenceAuthority,
			Indexes:          indexes,
			Capsules:         capsules,
			DatasetAuthority: sealed,
			BindingObserver:  bindingObserver,
			Random:           cryptorand.Reader,
		})
		if err != nil {
			return nil, nil, errors.Join(err, closeStores())
		}
		return registry, closeStores, nil
	}
	if semanticErr != nil || !preparedRegistry.WitnessedV2ActivationAllowed() {
		result := runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidenceAuthority, snapshot: sealed}
		if semanticErr == nil && preparedRegistry.FreshImportInventoryV2() {
			result.registryOwner = newRuntimeFreshImportRegistryV1(registryRoot, registryAccess, evidenceAuthority, sealed, preparedRegistry, openRegistry)
		}
		return result, true, nil
	}
	registry, closeStores, err := openRegistry()
	if err != nil {
		return runtimeSharedEvidenceDatasetSnapshotV2{}, true, err
	}
	return runtimeSharedEvidenceDatasetSnapshotV2{
		evidence: evidenceAuthority, snapshot: sealed, registry: registry,
		registryOwner: &runtimeImportActivatedRegistryV1{registry: registry, closeStores: closeStores},
	}, true, nil
}

func loadRuntimeSharedEvidenceEnrollmentV2(
	ctx context.Context,
	config Config,
	installationAuthority finalauthorityport.Authority,
) (runtimeSharedEvidenceEnrollmentV2, bool, error) {
	enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentConfigV2(ctx, config)
	if err != nil || !configured {
		return enrolled, configured, err
	}
	if installationAuthority == nil || enrolled.projection.InstallationAuthorityKeyID != installationAuthority.KeyID() ||
		!bytes.Equal(enrolled.projection.InstallationAuthorityPublicKey, installationAuthority.PublicKey()) {
		return runtimeSharedEvidenceEnrollmentV2{}, true, errors.New("runtime installation authority does not match protected enrollment")
	}
	return enrolled, true, nil
}

func loadRuntimeSharedEvidenceEnrollmentConfigV2(ctx context.Context, config Config) (runtimeSharedEvidenceEnrollmentV2, bool, error) {
	anchored, projection, configured, err := loadRuntimeSharedEvidenceManifestV2(ctx, config)
	if err != nil || !configured {
		return runtimeSharedEvidenceEnrollmentV2{}, configured, err
	}
	source := authorityanchorenv.Source{Lookup: func(name string) (string, bool) {
		if name != authorityanchorenv.AnchorEnvelopeV1Variable {
			return "", false
		}
		return config.AuthorityAnchorV1, true
	}}
	credentials, err := (authoritycredentialsfs.Loader{
		ProfileRoot: strings.TrimSpace(config.AuthorityCredentialProfileRoot), BundleRoot: strings.TrimSpace(config.AuthorityCredentialBundleRoot), Anchor: source,
	}).LoadCurrent(ctx, anchored)
	if err != nil {
		return runtimeSharedEvidenceEnrollmentV2{}, true, err
	}
	if credentials.ManifestDigest != projection.ManifestDigest ||
		credentials.ProfileDigest != projection.CredentialProfileDigest ||
		credentials.ProfileGeneration != projection.CredentialProfileGeneration {
		return runtimeSharedEvidenceEnrollmentV2{}, true, errors.New("runtime authority credentials do not match anchored profile")
	}
	witnessKey, err := base64.RawURLEncoding.DecodeString(projection.Enrollment.WitnessPublicKey)
	if err != nil || base64.RawURLEncoding.EncodeToString(witnessKey) != projection.Enrollment.WitnessPublicKey ||
		projection.Enrollment.WitnessKeyID != domainsecurity.SHA256Hex(witnessKey) {
		return runtimeSharedEvidenceEnrollmentV2{}, true, errors.New("runtime shared evidence witness key is invalid")
	}
	return runtimeSharedEvidenceEnrollmentV2{
		projection: projection, credentials: credentials, witnessKey: witnessKey,
	}, true, nil
}

func loadRuntimeSharedEvidenceManifestV2(ctx context.Context, config Config) (domainenrollment.AnchoredManifestV2, domainenrollment.ManifestAnchorProjectionV2, bool, error) {
	anchorBody := config.AuthorityAnchorV1
	roots := []string{
		strings.TrimSpace(config.AuthorityManifestRoot),
		strings.TrimSpace(config.AuthorityCredentialProfileRoot),
		strings.TrimSpace(config.AuthorityCredentialBundleRoot),
	}
	if anchorBody == "" {
		for _, root := range roots {
			if root != "" {
				return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, true, errors.New("runtime authority roots require a protected anchor")
			}
		}
		return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, false, nil
	}
	if ctx == nil || anchorBody != strings.TrimSpace(anchorBody) {
		return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, true, errors.New("runtime protected authority configuration is invalid")
	}
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, true, errors.New("runtime authority root is invalid")
		}
		if _, duplicate := seen[root]; duplicate {
			return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, true, errors.New("runtime authority roots must be distinct")
		}
		seen[root] = struct{}{}
	}
	source := authorityanchorenv.Source{Lookup: func(name string) (string, bool) {
		if name != authorityanchorenv.AnchorEnvelopeV1Variable {
			return "", false
		}
		return anchorBody, true
	}}
	anchored, err := (authoritymanifestfs.Reader{
		Root: roots[0], Name: runtimeAuthorityManifestFileNameV2, Anchor: source,
	}).LoadAnchoredV2(ctx)
	if err != nil {
		return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, true, err
	}
	projection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(
		anchored, domainenrollment.SharedEvidenceNamespaceV1,
	)
	if err != nil {
		return domainenrollment.AnchoredManifestV2{}, domainenrollment.ManifestAnchorProjectionV2{}, true, err
	}
	return anchored, projection, true, nil
}
