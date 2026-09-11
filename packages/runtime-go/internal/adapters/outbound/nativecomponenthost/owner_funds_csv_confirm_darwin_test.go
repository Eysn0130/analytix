//go:build darwin

package nativecomponenthost

import (
	"bytes"
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	authorityanchorenv "analytix.local/runtime-go/internal/adapters/outbound/authorityanchorenv"
	authoritycredentialsfs "analytix.local/runtime-go/internal/adapters/outbound/authoritycredentialsfs"
	authoritymanifestfs "analytix.local/runtime-go/internal/adapters/outbound/authoritymanifestfs"
	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceauthoritystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	fundscsvsourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundscsvsource"
	fundsquerysource "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	monotonicheadprojection "analytix.local/runtime-go/internal/adapters/outbound/monotonicheadprojection"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	appidentity "analytix.local/runtime-go/internal/app/identity"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	formalauthority "analytix.local/runtime-go/internal/formalauthority"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

const (
	abR2ConfirmPackagePhaseV1    = "package_owner_and_local_build_trust"
	abR2ConfirmAuthorityPhaseV1  = "production_authority_enrollment"
	abR2ConfirmStagePhaseV1      = "stage_and_frozen_source_revalidation"
	abR2ConfirmEvidencePhaseV1   = "evidence_initialize_and_initial_current"
	abR2ConfirmNativePhaseV1     = "packaged_native_build_install_source_row"
	abR2ConfirmMaterialPhaseV1   = "production_21_material_writer"
	abR2ConfirmDSV2PhaseV1       = "sealed_dsv2_admit_cas_current_witness"
	abR2ConfirmResultPhaseV1     = "post_admit_case_binding_and_result"
	abR2ConfirmCaseIDV1          = "case_ab_r2_local_nonpublishable"
	abR2ConfirmManifestNameV1    = "manifest.json"
	abR2ConfirmPrivateSentinelV1 = "AB_R2_PRIVATE_SOURCE_SENTINEL_9f07f00d_GENERATION_ONE"
)

type abR2ConfirmCountersV1 struct {
	authorityRootSeparated bool
	profileRootCacheBacked bool
	builds                 int
	buildSuccesses         int
	nativeBuildErrorClass  string
	installs               int
	installSuccesses       int
	installedUses          int
	installedCallbacks     int
	rowPages               int
	rowPageSuccesses       int
	materialsAccepted      int
	resolveAttempts        int
	resolveSuccesses       int
	initialResolveMissing  bool
	admitAttempts          int
	admitSuccesses         int
	admitAfterAttempts     int
	stageCalls             int
	confirmCalls           int
	postAdmitResolveCalls  int
	materialKinds          map[domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1]int
	admitted               datasetsnapshotport.ResolvedSnapshotV2
}

type abR2ConfirmNativeOwnerV1 struct {
	delegate *Owner
	counters *abR2ConfirmCountersV1
}

type abR2ConfirmImmutableSourceV1 struct {
	delegate *fundsquerysource.Source
	counters *abR2ConfirmCountersV1
}

type abR2ConfirmMaterialWriterV1 struct {
	delegate *datasetsnapshotstore.StoresV2
	counters *abR2ConfirmCountersV1
}

type abR2ConfirmSnapshotAuthorityV1 struct {
	delegate *datasetsnapshotapp.SealedServiceV2
	counters *abR2ConfirmCountersV1
}

func TestLocalNonpublishablePackagedOwnerConfirmsFundsCSVThroughProductionEvidenceDSV2(t *testing.T) {
	executable := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_LOCAL_NONPUBLISHABLE_RUNTIME_SERVER"))
	if executable == "" {
		t.Skip("local non-publishable packaged confirm seam requires an explicit isolated runtime-server")
	}
	if !filepath.IsAbs(executable) || !strings.HasPrefix(filepath.Clean(executable), "/Volumes/AnalytixCache/") {
		t.Fatal(abR2ConfirmPackagePhaseV1)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(abR2ConfirmPackagePhaseV1)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	repositoryRoot, err := filepath.EvalSymlinks(filepath.Join(
		workingDirectory, "..", "..", "..", "..", "..", "..",
	))
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	if _, err := os.Stat(filepath.Join(repositoryRoot, "packages", "runtime-go", "go.mod")); err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	profileRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil || os.Chmod(profileRoot, 0o700) != nil ||
		!strings.HasPrefix(profileRoot, "/Volumes/AnalytixCache/") ||
		!abR2ConfirmExactPrivateDirectoryV1(profileRoot) {
		t.Fatal(abR2ConfirmPackagePhaseV1)
	}
	authorityRoot, err := os.MkdirTemp("/private/tmp", "analytix-ab-r2-confirm-authority-")
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	authorityRootRemoved := false
	var formal *formalauthority.Service
	formalClosed := false
	defer func() {
		if !formalClosed && formal != nil {
			formal.Close()
		}
		if !authorityRootRemoved {
			if cleanupErr := abR2ConfirmRemoveAuthorityRootV1(authorityRoot); cleanupErr != nil {
				t.Error(abR2ConfirmAuthorityPhaseV1)
			}
		}
	}()
	canonicalAuthorityRoot, err := filepath.EvalSymlinks(authorityRoot)
	if err != nil || os.Chmod(authorityRoot, 0o700) != nil || canonicalAuthorityRoot != authorityRoot ||
		!abR2ConfirmExactAuthorityRootV1(authorityRoot) ||
		abR2ConfirmPathsOverlapV1(authorityRoot, repositoryRoot) ||
		abR2ConfirmPathsOverlapV1(authorityRoot, "/Volumes/AnalytixCache") ||
		abR2ConfirmPathsOverlapV1(authorityRoot, profileRoot) {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	formal, err = formalauthority.New(authorityRoot)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	if formalauthority.SecureConfigurationFilesystemBlocker(formal.ManifestRoot) != "" ||
		formal.TotalAttempts() != 0 || !abR2ConfirmPathContainedV1(authorityRoot, formal.DataDir) ||
		abR2ConfirmPathsOverlapV1(formal.DataDir, profileRoot) {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}

	immutableSource, err := fundsquerysource.NewHostExactSource(profileRoot)
	if err != nil {
		t.Fatal(abR2ConfirmNativePhaseV1)
	}
	owner, err := openDarwinHostCandidateForExecutable(profileRoot, resolvedExecutable)
	if err != nil {
		t.Fatal(abR2ConfirmPackagePhaseV1)
	}
	ownerClosed := false
	defer func() {
		if !ownerClosed {
			_ = owner.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	privateRoot := filepath.Join(formal.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	stores, err := datasetsnapshotstore.OpenStoresV2(
		filepath.Join(privateRoot, "dataset-snapshot-authority"), access,
	)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	bundles, err := evidenceauthoritystore.NewBundleStore(
		filepath.Join(privateRoot, "evidence-authority", "bundles"), access,
	)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	observations, err := evidenceauthoritystore.NewObservationStore(
		filepath.Join(privateRoot, "evidence-authority", "observations"), access,
	)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	if hasEvidence, checkErr := abR2ConfirmEvidenceRecordsV1(ctx, bundles, observations); checkErr != nil || hasEvidence {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	if hasSnapshots, checkErr := stores.HasRecords(ctx); checkErr != nil || hasSnapshots || formal.TotalAttempts() != 0 {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}

	anchor := authorityanchorenv.Source{Lookup: func(name string) (string, bool) {
		if name != authorityanchorenv.AnchorEnvelopeV1Variable {
			return "", false
		}
		return formal.AnchorEnvelope, true
	}}
	anchored, err := (authoritymanifestfs.Reader{
		Root: formal.ManifestRoot, Name: abR2ConfirmManifestNameV1, Anchor: anchor,
	}).LoadAnchoredV2(ctx)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	enrollment, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(
		anchored, domainenrollment.SharedEvidenceNamespaceV1,
	)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	authorityPublicKey := formal.Authority.PublicKey()
	if enrollment.InstallationID != formal.InstallationID ||
		enrollment.InstallationAuthorityKeyID != formal.Authority.KeyID() ||
		!bytes.Equal(enrollment.InstallationAuthorityPublicKey, authorityPublicKey) ||
		enrollment.Enrollment.Namespace != domainenrollment.SharedEvidenceNamespaceV1 ||
		enrollment.Enrollment.EnrollmentID == "" || enrollment.Enrollment.WitnessKeyID == "" {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	credentials, err := (authoritycredentialsfs.Loader{
		ProfileRoot: formal.CredentialProfileRoot,
		BundleRoot:  formal.CredentialBundleRoot,
		Anchor:      anchor,
	}).LoadCurrent(ctx, anchored)
	if err != nil || credentials.ManifestDigest != enrollment.ManifestDigest ||
		credentials.ProfileDigest != enrollment.CredentialProfileDigest ||
		credentials.ProfileGeneration != enrollment.CredentialProfileGeneration {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	witnessKey, err := base64.RawURLEncoding.DecodeString(enrollment.Enrollment.WitnessPublicKey)
	if err != nil || len(witnessKey) != ed25519.PublicKeySize ||
		base64.RawURLEncoding.EncodeToString(witnessKey) != enrollment.Enrollment.WitnessPublicKey ||
		domainsecurity.SHA256Hex(witnessKey) != enrollment.Enrollment.WitnessKeyID {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	floor, err := monotonicheadprojection.New(monotonicheadprojection.Config{
		Root:              filepath.Join(privateRoot, "evidence-checkpoint-floor"),
		InstallationID:    enrollment.InstallationID,
		EnrollmentID:      enrollment.Enrollment.EnrollmentID,
		Namespace:         enrollment.Enrollment.Namespace,
		WitnessKeyID:      enrollment.Enrollment.WitnessKeyID,
		WitnessPublicKey:  witnessKey,
		InitialCheckpoint: enrollment.Enrollment.InitialCheckpoint,
	})
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	projection, err := evidenceauthoritystore.NewProjection(
		filepath.Join(privateRoot, "evidence-authority-projection"), bundles,
	)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	evidence, err := evidenceauthorityapp.New(evidenceauthorityapp.Config{
		InstallationID:   enrollment.InstallationID,
		EnrollmentID:     enrollment.Enrollment.EnrollmentID,
		Authority:        formal.Authority,
		WitnessKeyID:     enrollment.Enrollment.WitnessKeyID,
		WitnessPublicKey: witnessKey,
		Random:           cryptorand.Reader,
		Witness:          credentials.SharedEvidence,
		CheckpointFloor:  floor,
		Bundles:          bundles,
		Observations:     observations,
		Projection:       projection,
		Genesis: evidenceauthorityapp.Genesis{
			DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		},
	})
	clear(witnessKey)
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	sealed, err := datasetsnapshotapp.NewSealedV2(datasetsnapshotapp.SealedConfigV2{
		InstallationID: enrollment.InstallationID,
		EnrollmentID:   enrollment.Enrollment.EnrollmentID,
		Authority:      formal.Authority,
		Coordinator:    evidence,
		LegacyRecords:  stores.LegacyRecords,
		Bundles:        stores.AuthorityBundles,
		Indexes:        stores.Indexes,
		Materials:      stores.Materials,
		Random:         cryptorand.Reader,
	})
	if err != nil || formal.TotalAttempts() != 0 {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}
	identity, err := appidentity.NewInstallationLocalAuthority(formal.Authority.KeyID())
	if err != nil {
		t.Fatal(abR2ConfirmAuthorityPhaseV1)
	}

	workspace, sourcePath, sourceBody := abR2ConfirmWriteSmokeWorkspaceV1(t, profileRoot)
	defer clear(sourceBody)
	observer := filestore.CaseBindingReader{}
	beforeBinding, err := observer.Observe(workspace)
	if err != nil || beforeBinding.CaseID != abR2ConfirmCaseIDV1 ||
		beforeBinding.State != domainsecurity.CaseBindingStateValid {
		t.Fatal(abR2ConfirmStagePhaseV1)
	}
	counters := &abR2ConfirmCountersV1{
		authorityRootSeparated: true,
		profileRootCacheBacked: true,
		materialKinds:          make(map[domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1]int),
	}
	native := &abR2ConfirmNativeOwnerV1{delegate: owner, counters: counters}
	source := &abR2ConfirmImmutableSourceV1{delegate: immutableSource, counters: counters}
	materials := &abR2ConfirmMaterialWriterV1{delegate: stores, counters: counters}
	snapshots := &abR2ConfirmSnapshotAuthorityV1{delegate: sealed, counters: counters}
	service, err := fundscsvadmissionapp.NewServiceV1(fundscsvadmissionapp.ConfigV1{
		Observer:         observer,
		Identity:         identity,
		Evidence:         evidence,
		Snapshots:        snapshots,
		Materials:        materials,
		Native:           native,
		Source:           source,
		ReadImportSource: fundscsvsourceadapter.ReadImportExactV1,
		Now: func() time.Time {
			return time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
		},
		Random: bytes.NewReader(bytes.Repeat([]byte{0x5a}, 1024)),
	})
	if err != nil {
		abR2ConfirmFailV1(t, abR2ConfirmAuthorityPhaseV1, counters, formal.TotalAttempts())
	}

	counters.stageCalls++
	staged, err := service.StageMainSelectedImportV1(ctx, fundscsvadmissionapp.StageInputV1{
		WorkspaceRoot: workspace,
		SourcePath:    sourcePath,
	})
	if err != nil || staged.Status != "ready" || staged.TotalRowCount != 1 || len(staged.Items) != 1 ||
		staged.Items[0].Selector == "" || staged.Items[0].RowCount != 1 || formal.TotalAttempts() != 0 {
		abR2ConfirmFailV1(t, abR2ConfirmStagePhaseV1, counters, formal.TotalAttempts())
	}
	counters.confirmCalls++
	confirmed, err := service.ConfirmImportV1(ctx, staged.Items[0].Selector)
	if err != nil {
		abR2ConfirmFailV1(t, abR2ConfirmFailurePhaseV1(ctx, bundles, observations, counters), counters, formal.TotalAttempts())
	}
	if confirmed.SourceArtifactSHA256 != domainsecurity.SHA256Hex(sourceBody) ||
		confirmed.SourceArtifactByteLength != uint64(len(sourceBody)) || confirmed.SourceRowCount != 1 {
		abR2ConfirmFailV1(t, abR2ConfirmResultPhaseV1, counters, formal.TotalAttempts())
	}
	if !abR2ConfirmNativeExactV1(counters) {
		abR2ConfirmFailV1(t, abR2ConfirmNativePhaseV1, counters, formal.TotalAttempts())
	}
	if !abR2ConfirmMaterialGraphExactV1(counters) {
		abR2ConfirmFailV1(t, abR2ConfirmMaterialPhaseV1, counters, formal.TotalAttempts())
	}
	if counters.resolveAttempts != 1 || counters.resolveSuccesses != 0 || !counters.initialResolveMissing ||
		counters.admitAttempts != 1 || counters.admitSuccesses != 1 || counters.admitAfterAttempts != 0 ||
		datasetsnapshotport.ValidateResolvedSnapshotV2(counters.admitted) != nil {
		abR2ConfirmFailV1(t, abR2ConfirmDSV2PhaseV1, counters, formal.TotalAttempts())
	}
	afterBinding, err := observer.Observe(workspace)
	if err != nil || afterBinding != beforeBinding {
		abR2ConfirmFailV1(t, abR2ConfirmResultPhaseV1, counters, formal.TotalAttempts())
	}
	counters.postAdmitResolveCalls++
	selected, err := snapshots.ResolveWitnessedV2(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID:                  domainsecurity.LocalTenantID,
		UserID:                    domainsecurity.LocalUserID,
		Observation:               afterBinding,
		ExpectedDatasetSnapshotID: counters.admitted.Record.DatasetSnapshotID,
	})
	if err != nil || datasetsnapshotport.ValidateResolvedSnapshotV2(selected) != nil ||
		selected.Record != counters.admitted.Record || selected.Manifest != counters.admitted.Manifest ||
		selected.FundsProducerContent != counters.admitted.FundsProducerContent {
		abR2ConfirmFailV1(t, abR2ConfirmDSV2PhaseV1, counters, formal.TotalAttempts())
	}
	head, err := evidence.Current(ctx)
	if err != nil || !head.HasBundle || head.Bundle.DatasetSnapshotCount != 1 ||
		head.Bundle.DatasetSnapshotIndexDigest == domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		abR2ConfirmFailV1(t, abR2ConfirmDSV2PhaseV1, counters, formal.TotalAttempts())
	}
	hasEvidence, evidenceErr := abR2ConfirmEvidenceRecordsV1(ctx, bundles, observations)
	hasSnapshots, snapshotErr := stores.HasRecords(ctx)
	if evidenceErr != nil || snapshotErr != nil || !hasEvidence || !hasSnapshots || formal.TotalAttempts() == 0 ||
		!counters.authorityRootSeparated || !counters.profileRootCacheBacked ||
		counters.stageCalls != 1 || counters.confirmCalls != 1 || counters.resolveAttempts != 2 ||
		counters.resolveSuccesses != 1 || counters.postAdmitResolveCalls != 1 {
		abR2ConfirmFailV1(t, abR2ConfirmResultPhaseV1, counters, formal.TotalAttempts())
	}
	if err := owner.Close(); err != nil {
		abR2ConfirmFailV1(t, abR2ConfirmPackagePhaseV1, counters, formal.TotalAttempts())
	}
	ownerClosed = true
	formal.Close()
	formalClosed = true
	if err := abR2ConfirmRemoveAuthorityRootV1(authorityRoot); err != nil {
		abR2ConfirmFailV1(t, abR2ConfirmAuthorityPhaseV1, counters, formal.TotalAttempts())
	}
	authorityRootRemoved = true
}

func (owner *abR2ConfirmNativeOwnerV1) BuildFundsCanonicalCSVSnapshot(
	ctx context.Context,
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
	source io.Reader,
	installer fundsquerysourceport.ImmutableSnapshotInstaller,
) (
	domainnative.FundsCanonicalCSVSnapshotBuildResultV1,
	domainfundsquerysource.ImmutableSnapshotObjectV1,
	fundsquerysourceport.ImmutableSnapshotInstallDispositionV1,
	error,
) {
	owner.counters.builds++
	result, object, disposition, err := owner.delegate.BuildFundsCanonicalCSVSnapshot(ctx, arguments, source, installer)
	if err == nil {
		owner.counters.buildSuccesses++
	} else {
		owner.counters.nativeBuildErrorClass = abR2ConfirmNativeBuildErrorClassV1(err)
	}
	return result, object, disposition, err
}

func (owner *abR2ConfirmNativeOwnerV1) TransactionSourceRowPage(
	ctx context.Context,
	arguments domainnative.TransactionSourceRowPageArgumentsV1,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	lease fundsquerysourceport.ExactReadLease,
) (domainnative.TransactionSourceRowPageV1, error) {
	owner.counters.rowPages++
	page, err := owner.delegate.TransactionSourceRowPage(ctx, arguments, object, lease)
	if err == nil {
		owner.counters.rowPageSuccesses++
	}
	return page, err
}

func (source *abR2ConfirmImmutableSourceV1) InstallExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	file *os.File,
) (fundsquerysourceport.ImmutableSnapshotInstallDispositionV1, error) {
	source.counters.installs++
	disposition, err := source.delegate.InstallExact(ctx, object, file)
	if err == nil && disposition == fundsquerysourceport.ImmutableSnapshotInstallCreatedV1 {
		source.counters.installSuccesses++
	}
	return disposition, err
}

func (source *abR2ConfirmImmutableSourceV1) WithInstalledExact(
	ctx context.Context,
	object domainfundsquerysource.ImmutableSnapshotObjectV1,
	use func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	source.counters.installedUses++
	return source.delegate.WithInstalledExact(ctx, object, func(
		leaseCtx context.Context,
		lease fundsquerysourceport.ExactReadLease,
	) error {
		source.counters.installedCallbacks++
		return use(leaseCtx, lease)
	})
}

func (writer *abR2ConfirmMaterialWriterV1) PutFundsCanonicalCSVAdmissionMaterialV1(
	ctx context.Context,
	material domainevidence.FundsCanonicalCSVAdmissionMaterialV1,
) error {
	var kind domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1
	if err := material.UseExactV1(func(
		observed domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1,
		_ string,
		_ []byte,
	) error {
		kind = observed
		return nil
	}); err != nil {
		return err
	}
	if err := writer.delegate.PutFundsCanonicalCSVAdmissionMaterialV1(ctx, material); err != nil {
		return err
	}
	writer.counters.materialsAccepted++
	writer.counters.materialKinds[kind]++
	return nil
}

func (snapshots *abR2ConfirmSnapshotAuthorityV1) AdmitExactV2(
	ctx context.Context,
	input datasetsnapshotapp.AdmitInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshots.counters.admitAttempts++
	resolved, err := snapshots.delegate.AdmitExactV2(ctx, input)
	if err == nil {
		snapshots.counters.admitSuccesses++
		snapshots.counters.admitted = resolved
	}
	return resolved, err
}

func (snapshots *abR2ConfirmSnapshotAuthorityV1) AdmitAfterExactV2(
	ctx context.Context,
	input datasetsnapshotapp.AdmitAfterInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshots.counters.admitAfterAttempts++
	return snapshots.delegate.AdmitAfterExactV2(ctx, input)
}

func (snapshots *abR2ConfirmSnapshotAuthorityV1) ResolveWitnessedV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	snapshots.counters.resolveAttempts++
	resolved, err := snapshots.delegate.ResolveWitnessedV2(ctx, input)
	if err == nil {
		snapshots.counters.resolveSuccesses++
	} else if snapshots.counters.resolveAttempts == 1 && errors.Is(err, datasetsnapshotport.ErrUnavailable) {
		snapshots.counters.initialResolveMissing = true
	}
	return resolved, err
}

func abR2ConfirmWriteSmokeWorkspaceV1(t *testing.T, profileRoot string) (string, string, []byte) {
	t.Helper()
	workspace := filepath.Join(profileRoot, "confirm-first-workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(abR2ConfirmStagePhaseV1)
	}
	workspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(abR2ConfirmStagePhaseV1)
	}
	metadata := filepath.Join(workspace, ".analytix")
	if err := os.Mkdir(metadata, 0o700); err != nil {
		t.Fatal(abR2ConfirmStagePhaseV1)
	}
	caseProject, err := json.Marshal(struct {
		Version       int    `json:"version"`
		WorkspaceRoot string `json:"workspaceRoot"`
		CaseID        string `json:"caseId"`
		Source        string `json:"source"`
		UpdatedAt     string `json:"updatedAt"`
	}{
		Version: 1, WorkspaceRoot: workspace, CaseID: abR2ConfirmCaseIDV1,
		Source: "analytix-data-analysis", UpdatedAt: "2026-08-27T08:00:00Z",
	})
	if err != nil || os.WriteFile(filepath.Join(metadata, "case-project.json"), caseProject, 0o600) != nil {
		t.Fatal(abR2ConfirmStagePhaseV1)
	}
	columns := []string{
		"交易卡号", "交易账号", "账户开户名称", "开户人证件号码", "交易时间", "交易金额", "交易余额", "收付标志",
		"交易对手账卡号", "现金标志", "对手户名", "对手身份证号", "对手开户银行", "摘要说明", "交易币种", "交易网点名称",
		"交易网点代码", "交易发生地", "交易是否成功", "传票号", "终端号", "IP地址", "MAC地址", "对手交易余额",
		"交易流水号", "日志号", "凭证种类", "凭证号", "交易柜员号", "商户名称", "商户号", "备注", "交易类型", "查询反馈结果原因",
	}
	row := make([]string, len(columns))
	row[0], row[1], row[2], row[3] = "6222021234567890001", "1000001", "合成账户甲", "SYNTHID0001"
	row[4], row[5], row[6], row[7] = "2026-08-27 10:00:00", "12.5", "1000", "进"
	row[8], row[9], row[10], row[11] = "CP001", "否", "合成对手甲", "SYNTHCPID001"
	row[12], row[13], row[14], row[31] = "合成银行", "合成交易", "CNY", abR2ConfirmPrivateSentinelV1
	body := []byte(strings.Join(columns, ",") + "\r\n" + strings.Join(row, ",") + "\r\n")
	for index := range row {
		row[index] = ""
	}
	sourcePath := filepath.Join(workspace, "synthetic-generation_one.csv")
	if err := os.WriteFile(sourcePath, body, 0o600); err != nil {
		clear(body)
		t.Fatal(abR2ConfirmStagePhaseV1)
	}
	return workspace, sourcePath, body
}

func abR2ConfirmEvidenceRecordsV1(
	ctx context.Context,
	bundles *evidenceauthoritystore.BundleStore,
	observations *evidenceauthoritystore.ObservationStore,
) (bool, error) {
	hasBundles, err := bundles.HasRecords(ctx)
	if err != nil {
		return false, err
	}
	hasObservations, err := observations.HasRecords(ctx)
	return hasBundles || hasObservations, err
}

func abR2ConfirmExactPrivateDirectoryV1(root string) bool {
	info, err := os.Lstat(root)
	stat, ok := infoSyscallStatV1(info)
	return err == nil && ok && info.IsDir() && info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().Perm() == 0o700 && stat.Uid == uint32(os.Geteuid())
}

func abR2ConfirmExactAuthorityRootV1(root string) bool {
	return filepath.Dir(root) == "/private/tmp" &&
		strings.HasPrefix(filepath.Base(root), "analytix-ab-r2-confirm-authority-") &&
		abR2ConfirmExactPrivateDirectoryV1(root)
}

func abR2ConfirmPathContainedV1(root string, candidate string) bool {
	relativePath, err := filepath.Rel(root, candidate)
	return err == nil && (relativePath == "." ||
		(relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) &&
			!filepath.IsAbs(relativePath)))
}

func abR2ConfirmPathsOverlapV1(left string, right string) bool {
	return abR2ConfirmPathContainedV1(left, right) || abR2ConfirmPathContainedV1(right, left)
}

func abR2ConfirmRemoveAuthorityRootV1(root string) error {
	if !abR2ConfirmExactAuthorityRootV1(root) {
		return errors.New(abR2ConfirmAuthorityPhaseV1)
	}
	return os.RemoveAll(root)
}

func infoSyscallStatV1(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return stat, ok
}

func abR2ConfirmNativeExactV1(counters *abR2ConfirmCountersV1) bool {
	return counters != nil && counters.builds == 1 && counters.buildSuccesses == 1 &&
		counters.installs == 1 && counters.installSuccesses == 1 && counters.installedUses == 1 &&
		counters.installedCallbacks == 1 && counters.rowPages == 1 && counters.rowPageSuccesses == 1
}

func abR2ConfirmMaterialGraphExactV1(counters *abR2ConfirmCountersV1) bool {
	if counters == nil || counters.materialsAccepted != 21 || len(counters.materialKinds) != 21 {
		return false
	}
	want := []domainevidence.FundsCanonicalCSVAdmissionMaterialKindV1{
		domainevidence.FundsCanonicalCSVMaterialSnapshotManifestV1,
		domainevidence.FundsCanonicalCSVMaterialProducerContentV1,
		domainevidence.FundsCanonicalCSVMaterialRawIntentV1,
		domainevidence.FundsCanonicalCSVMaterialRawManifestV1,
		domainevidence.FundsCanonicalCSVMaterialRawManifestPageV1,
		domainevidence.FundsCanonicalCSVMaterialRawEntryV1,
		domainevidence.FundsCanonicalCSVMaterialRawSourceLocatorV1,
		domainevidence.FundsCanonicalCSVMaterialRawContentRootV1,
		domainevidence.FundsCanonicalCSVMaterialRawContentIndexPageV1,
		domainevidence.FundsCanonicalCSVMaterialRawContentChunkV1,
		domainevidence.FundsCanonicalCSVMaterialParsedConfigurationV1,
		domainevidence.FundsCanonicalCSVMaterialParsedIdentityV1,
		domainevidence.FundsCanonicalCSVMaterialParsedReceiptV1,
		domainevidence.FundsCanonicalCSVMaterialClassificationLedgerV1,
		domainevidence.FundsCanonicalCSVMaterialParsedIndexPageV1,
		domainevidence.FundsCanonicalCSVMaterialParsedPageV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowRootV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowIndexPageV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowPageV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowRecordV1,
		domainevidence.FundsCanonicalCSVMaterialSourceRowLineageV1,
	}
	for _, kind := range want {
		if counters.materialKinds[kind] != 1 {
			return false
		}
	}
	return true
}

func abR2ConfirmFailurePhaseV1(
	ctx context.Context,
	bundles *evidenceauthoritystore.BundleStore,
	observations *evidenceauthoritystore.ObservationStore,
	counters *abR2ConfirmCountersV1,
) string {
	hasEvidence, _ := abR2ConfirmEvidenceRecordsV1(ctx, bundles, observations)
	if !hasEvidence || counters.resolveAttempts == 0 {
		return abR2ConfirmEvidencePhaseV1
	}
	if !abR2ConfirmNativeExactV1(counters) {
		return abR2ConfirmNativePhaseV1
	}
	if !abR2ConfirmMaterialGraphExactV1(counters) {
		return abR2ConfirmMaterialPhaseV1
	}
	if counters.admitAttempts != 1 || counters.admitSuccesses != 1 {
		return abR2ConfirmDSV2PhaseV1
	}
	return abR2ConfirmResultPhaseV1
}

func abR2ConfirmNativeBuildErrorClassV1(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, nativecomponentport.ErrRequestInvalid):
		return "request_invalid"
	case errors.Is(err, nativecomponentport.ErrRegistryInvalid):
		return "registry_invalid"
	case errors.Is(err, nativecomponentport.ErrProtocolInvalid):
		return "protocol_invalid"
	case errors.Is(err, nativecomponentport.ErrTerminationUnconfirmed):
		return "termination_unconfirmed"
	case errors.Is(err, nativecomponentport.ErrUnavailable), errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "unclassified"
	}
}

func abR2ConfirmFailV1(
	t *testing.T,
	phase string,
	counters *abR2ConfirmCountersV1,
	witnessAttempts int,
) {
	t.Helper()
	if counters == nil {
		t.Fatalf("%s counts=0/0/0/0/0/0/0/0", phase)
	}
	if counters.nativeBuildErrorClass != "" {
		t.Fatalf(
			"%s native_build_error=%s counts=%d/%d/%d/%d/%d/%d/%d/%d",
			phase,
			counters.nativeBuildErrorClass,
			counters.builds,
			counters.installs,
			counters.installedUses,
			counters.rowPages,
			counters.materialsAccepted,
			counters.resolveAttempts,
			counters.admitAttempts,
			witnessAttempts,
		)
	}
	t.Fatalf(
		"%s counts=%d/%d/%d/%d/%d/%d/%d/%d",
		phase,
		counters.builds,
		counters.installs,
		counters.installedUses,
		counters.rowPages,
		counters.materialsAccepted,
		counters.resolveAttempts,
		counters.admitAttempts,
		witnessAttempts,
	)
}

var _ fundscsvadmissionapp.NativeOwnerV1 = (*abR2ConfirmNativeOwnerV1)(nil)
var _ fundscsvadmissionapp.ImmutableSourceV1 = (*abR2ConfirmImmutableSourceV1)(nil)
var _ fundscsvadmissionapp.MaterialWriterV1 = (*abR2ConfirmMaterialWriterV1)(nil)
var _ fundscsvadmissionapp.SnapshotAuthorityV2 = (*abR2ConfirmSnapshotAuthorityV1)(nil)
