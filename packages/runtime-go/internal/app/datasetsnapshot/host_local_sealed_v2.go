package datasetsnapshot

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"errors"
	"io"
	"strings"

	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// HostLocalSealedConfigV2 reuses the DSV2 immutable record/material stores,
// while selecting the child root only through the signed host-local head.
type HostLocalSealedConfigV2 struct {
	InstallationID string
	Authority      finalauthorityport.Authority
	Heads          evidenceauthorityport.HostLocalHeadCoordinator
	LegacyRecords  datasetsnapshotport.RecordStore
	Bundles        datasetsnapshotport.AuthorityBundleStoreV2
	Indexes        datasetsnapshotport.IndexStore
	Materials      datasetsnapshotport.AdmissionMaterialReaderV2
	Random         io.Reader
}

type HostLocalSealedServiceV2 struct {
	*SealedServiceV2
	heads                evidenceauthorityport.HostLocalHeadCoordinator
	modeCommitmentDigest string
	rootBindingDigest    string
}

var _ datasetsnapshotport.AuthorityV2 = (*HostLocalSealedServiceV2)(nil)

// The embedded material helpers carry legacy V1 methods. A host-local
// service never presents a synthetic witnessed V1 authority.
func (*HostLocalSealedServiceV2) ResolveWitnessed(context.Context,
	datasetsnapshotport.ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrUnavailable
}

func (*HostLocalSealedServiceV2) Accept(context.Context,
	AcceptInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrUnavailable
}

func NewHostLocalSealedV2(ctx context.Context, config HostLocalSealedConfigV2) (*HostLocalSealedServiceV2, error) {
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	if ctx == nil || config.Authority == nil || config.Heads == nil || config.LegacyRecords == nil ||
		config.Bundles == nil || config.Indexes == nil || config.Materials == nil ||
		!domainsecurity.IsSHA256Hex(config.InstallationID) || len(config.Authority.PublicKey()) != ed25519.PublicKeySize ||
		config.Authority.KeyID() != domainsecurity.SHA256Hex(config.Authority.PublicKey()) {
		return nil, errors.New("host-local sealed dataset snapshot configuration is invalid")
	}
	genesis, err := config.Heads.CurrentModeCommitment(ctx)
	if err != nil || domainhost.ValidateHeadV1(genesis) != nil || genesis.Generation != 0 ||
		genesis.InstallationID != config.InstallationID || genesis.AuthorityKeyID != config.Authority.KeyID() {
		return nil, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	base := &SealedServiceV2{
		Service: &Service{
			installationID: config.InstallationID, authority: config.Authority,
			authorityKeyID: config.Authority.KeyID(), authorityKey: append([]byte(nil), config.Authority.PublicKey()...),
			records: config.LegacyRecords, indexes: config.Indexes, random: config.Random,
		},
		bundles: config.Bundles, materials: config.Materials,
	}
	return &HostLocalSealedServiceV2{SealedServiceV2: base, heads: config.Heads,
		modeCommitmentDigest: genesis.RecordDigest, rootBindingDigest: genesis.RootBindingDigest}, nil
}

// AdmitExactV2 is the host-only immutable DSV2 admission for this profile.
// It never routes through the witnessed SealedServiceV2 coordinator.
func (service *HostLocalSealedServiceV2) AdmitExactV2(ctx context.Context, input AdmitInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	return service.admitHostLocalV2(ctx, input, "")
}

func (service *HostLocalSealedServiceV2) admitHostLocalV2(ctx context.Context, input AdmitInputV2,
	expectedCurrentDatasetSnapshotID string) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if service == nil || ctx == nil || input.AcceptedAt.IsZero() {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	var resolved datasetsnapshotport.ResolvedSnapshotV2
	err := service.heads.WithProtectedMutation(ctx, func(admissionContext context.Context, mutation evidenceauthorityport.HostLocalMutation) error {
		var admitErr error
		resolved, admitErr = service.admitHostLocalExactV2(admissionContext, input, expectedCurrentDatasetSnapshotID, mutation)
		return admitErr
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	return resolved, nil
}

func (service *HostLocalSealedServiceV2) admitHostLocalExactV2(ctx context.Context, input AdmitInputV2,
	expectedCurrentDatasetSnapshotID string, mutation evidenceauthorityport.HostLocalMutation) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if ctx == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	resolve := datasetsnapshotport.ResolveInputV2{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID), Observation: input.Observation,
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{
		TenantID: resolve.TenantID, UserID: resolve.UserID, Observation: resolve.Observation,
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	head, err := service.currentHostHead(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	state, err := service.loadVersionedChainFromRootV2(ctx, head.DatasetSnapshotIndexDigest,
		head.DatasetSnapshotCount, true, service.modeCommitmentDigest, &binding, resolve)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	latest, hasLatest := state.latestByBinding[binding.BindingKeyDigest]
	if expectedCurrentDatasetSnapshotID != "" && (!hasLatest || latest.record.V2 == nil ||
		latest.record.V2.DatasetSnapshotID != expectedCurrentDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale,
			errors.New("host-local dataset snapshot expected predecessor is not current"))
	}
	material, err := service.verifyExactMaterialV2(ctx, input.ManifestReference, input.FundsProducerReference)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if material.manifest.Binding != binding {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	derivedID := domainsecurity.DeriveDatasetSnapshotIDV2(material.manifest)
	if hasLatest && latest.record.V2 != nil && latest.record.V2.DatasetSnapshotID == derivedID {
		resolved, err := service.resolveHostNodeMaterialV2(ctx, latest, resolve)
		if err != nil || service.confirmHostDatasetHead(ctx, head) != nil {
			return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale, err)
		}
		return resolved, nil
	}
	if state.snapshotIDs[derivedID] || state.producerIDs[material.manifest.ProducerContentID] {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	predecessor := ""
	if hasLatest {
		predecessor = versionedRecordDigestV2(latest.record)
	}
	var record domainsecurity.DatasetSnapshotAuthorityRecordV2
	err = material.manifest.WithExactFundsProducerAuthorityAdmissionV2(material.producer, func(
		issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2,
	) error {
		issued, issueErr := issuer.Issue(domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
			InstallationID: service.installationID, AcceptedAt: input.AcceptedAt.UTC(),
			PredecessorRecordDigest: predecessor, AuthorityKeyID: service.authorityKeyID,
			AuthorityPublicKey: service.authorityKey,
		}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
		record = issued
		return issueErr
	})
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	nextVersioned := domainsecurity.VersionedDatasetSnapshotAuthorityRecord{
		SchemaVersion: domainsecurity.DatasetSnapshotAuthorityRecordSchemaVersionV2, V2: &record,
	}
	if hasLatest && domainsecurity.ValidateVersionedDatasetSnapshotAuthorityTransition(latest.record, nextVersioned) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	mutationID, err := service.freshMutationID(ctx)
	if err != nil || state.mutationIDs[mutationID] {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	index, err := domainsecurity.NewDatasetSnapshotIndexHostLocalV2(domainsecurity.DatasetSnapshotIndexHostLocalInputV2{
		InstallationID: service.installationID, ModeCommitmentDigest: service.modeCommitmentDigest,
		Generation: head.DatasetSnapshotCount + 1, PreviousIndexDigest: head.DatasetSnapshotIndexDigest,
		MutationID: mutationID, Binding: binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: service.authorityKeyID, AuthorityPublicKey: service.authorityKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil || state.headIndex != nil &&
		domainsecurity.ValidateDatasetSnapshotIndexHostLocalTransitionV2(*state.headIndex, index) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexRecordForHostLocalV2(index, record,
			service.installationID, service.modeCommitmentDigest, service.authorityKeyID, service.authorityKey) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	if err := service.persistCandidateV2(ctx, datasetsnapshotport.AuthorityBundleV2{
		Record: record, Manifest: material.manifest,
	}, index); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	nextHead, err := service.nextDatasetHead(ctx, head, index.IndexDigest, mutationID)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if err := mutation.AdvanceExact(ctx, head, nextHead); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	committed, err := service.loadVersionedChainFromRootV2(ctx, nextHead.DatasetSnapshotIndexDigest,
		nextHead.DatasetSnapshotCount, true, service.modeCommitmentDigest, &binding, resolve)
	if err != nil || committed.selected == nil || committed.selected.record.V2 == nil ||
		committed.selected.record.V2.RecordDigest != record.RecordDigest {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	resolved, err := service.resolveHostNodeMaterialV2(ctx, *committed.selected, resolve)
	if err != nil || service.confirmHostDatasetHead(ctx, nextHead) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale, err)
	}
	return resolved, nil
}

// Cleaning compares its exact input snapshot inside the same protected
// mutation that verifies material and advances the signed dataset child.
func (service *HostLocalSealedServiceV2) AdmitAfterExactV2(ctx context.Context, input AdmitAfterInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if !domainsecurity.IsDatasetSnapshotIDV2Syntax(input.ExpectedCurrentDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch,
			errors.New("host-local dataset snapshot expected predecessor is invalid"))
	}
	return service.admitHostLocalV2(ctx, input.AdmitInputV2, input.ExpectedCurrentDatasetSnapshotID)
}

// ResolveWitnessedV2 is the existing AuthorityV2 compatibility method name.
// This concrete service uses only the explicitly signed host-local profile.
func (service *HostLocalSealedServiceV2) ResolveWitnessedV2(ctx context.Context, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if service == nil || ctx == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID), Observation: input.Observation,
	})
	if err != nil || input.ExpectedDatasetSnapshotID != "" && !domainsecurity.IsDatasetSnapshotIDV2Syntax(input.ExpectedDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	head, err := service.currentHostHead(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	state, err := service.loadVersionedChainFromRootV2(ctx, head.DatasetSnapshotIndexDigest,
		head.DatasetSnapshotCount, true, service.modeCommitmentDigest, &binding, input)
	if err != nil || state.selected == nil || state.selected.record.V2 == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	if input.ExpectedDatasetSnapshotID != "" && state.selected.record.V2.DatasetSnapshotID != input.ExpectedDatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	resolved, err := service.resolveHostNodeMaterialV2(ctx, *state.selected, input)
	if err != nil || service.confirmHostDatasetHead(ctx, head) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale, err)
	}
	return resolved, nil
}

func (service *HostLocalSealedServiceV2) currentHostHead(ctx context.Context) (domainhost.HeadV1, error) {
	head, found, err := service.heads.Current(ctx)
	if err != nil || !found || domainhost.ValidateHeadV1(head) != nil ||
		head.InstallationID != service.installationID ||
		head.RootBindingDigest != service.rootBindingDigest ||
		head.AuthorityKeyID != service.authorityKeyID {
		return domainhost.HeadV1{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	return head, nil
}

func (service *HostLocalSealedServiceV2) confirmHostDatasetHead(ctx context.Context, expected domainhost.HeadV1) error {
	current, err := service.currentHostHead(ctx)
	if err != nil || current != expected {
		return errors.Join(datasetsnapshotport.ErrStale, err)
	}
	return nil
}

func (service *HostLocalSealedServiceV2) nextDatasetHead(ctx context.Context, previous domainhost.HeadV1,
	indexDigest, mutationID string) (domainhost.HeadV1, error) {
	if previous.Generation == ^uint64(0) || previous.DatasetSnapshotCount == ^uint64(0) {
		return domainhost.HeadV1{}, datasetsnapshotport.ErrCorrupt
	}
	return domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: previous.InstallationID, RootBindingDigest: previous.RootBindingDigest,
		Generation: previous.Generation + 1, PreviousHeadDigest: previous.RecordDigest,
		MutationID: mutationID, DatasetSnapshotIndexDigest: indexDigest,
		DatasetSnapshotCount:        previous.DatasetSnapshotCount + 1,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       previous.EvidenceRegistryCount,
		PublicationIndexDigest:      previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: service.authorityKeyID, AuthorityPublicKey: service.authorityKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
}

func (service *HostLocalSealedServiceV2) resolveHostNodeMaterialV2(ctx context.Context,
	node versionedSnapshotNodeV2, input datasetsnapshotport.ResolveInputV2) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if node.bundle == nil || node.record.V2 == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	manifestBody, err := domainsecurity.DatasetSnapshotManifestV2Bytes(node.bundle.Manifest)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	material, err := service.verifyExactMaterialV2(ctx,
		exactReferenceV2(domainsecurity.SHA256Hex(manifestBody), uint64(len(manifestBody))),
		exactReferenceV2(node.bundle.Manifest.ProducerContentManifestSHA256,
			node.bundle.Manifest.ProducerContentManifestByteLength))
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if material.manifest != node.bundle.Manifest ||
		domainsecurity.ValidateDatasetSnapshotIndexNodeForHostLocalManifestV2(
			node.index, *node.record.V2, material.manifest, material.producer,
			strings.TrimSpace(input.TenantID), strings.TrimSpace(input.UserID), input.Observation,
			service.installationID, service.modeCommitmentDigest, service.authorityKeyID, service.authorityKey) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrMismatch
	}
	return datasetsnapshotport.ResolvedSnapshotV2{
		Record: *node.record.V2, Manifest: material.manifest, FundsProducerContent: material.producer,
	}, nil
}
