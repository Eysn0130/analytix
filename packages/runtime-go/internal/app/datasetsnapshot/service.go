package datasetsnapshot

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"errors"
	"io"
	"strings"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

const maxDatasetSnapshotIndexDepth = 100_000

type Config struct {
	InstallationID string
	EnrollmentID   string
	Authority      finalauthorityport.Authority
	Coordinator    evidenceauthorityport.DatasetCoordinator
	Records        datasetsnapshotport.RecordStore
	Indexes        datasetsnapshotport.IndexStore
	Random         io.Reader
}

type Service struct {
	installationID string
	enrollmentID   string
	authority      finalauthorityport.Authority
	authorityKeyID string
	authorityKey   []byte
	coordinator    evidenceauthorityport.DatasetCoordinator
	records        datasetsnapshotport.RecordStore
	indexes        datasetsnapshotport.IndexStore
	random         io.Reader
}

type AcceptInput struct {
	TenantID           string
	UserID             string
	Observation        domainsecurity.CaseBindingObservationV1
	SourceManifestHash string
	RawManifestSHA256  string
	ParserVersion      string
	AcceptedAt         time.Time
}

type chainState struct {
	headIndex       *domainsecurity.DatasetSnapshotIndexV1
	selected        *domainsecurity.DatasetSnapshotAuthorityRecordV1
	indexDigests    map[string]bool
	recordDigests   map[string]bool
	snapshotIDs     map[string]bool
	mutationIDs     map[string]bool
	latestByBinding map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1
}

var _ datasetsnapshotport.Authority = (*Service)(nil)

func New(config Config) (*Service, error) {
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	publicKey := []byte(nil)
	keyID := ""
	if config.Authority != nil {
		publicKey = append([]byte(nil), config.Authority.PublicKey()...)
		keyID = strings.TrimSpace(config.Authority.KeyID())
	}
	if !domainsecurity.IsSHA256Hex(config.InstallationID) || !domainsecurity.IsSHA256Hex(config.EnrollmentID) ||
		config.Authority == nil || len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		config.Coordinator == nil || config.Records == nil || config.Indexes == nil || config.Random == nil {
		return nil, errors.New("dataset snapshot service configuration is invalid")
	}
	return &Service{
		installationID: config.InstallationID, enrollmentID: config.EnrollmentID,
		authority: config.Authority, authorityKeyID: keyID, authorityKey: publicKey,
		coordinator: config.Coordinator, records: config.Records, indexes: config.Indexes, random: config.Random,
	}, nil
}

// ResolveWitnessed selects a snapshot only from a fresh shared-authority
// observation and rechecks the exact dataset child after full ancestry
// validation. Cached catalogs, local projections, directory order, and record
// signatures alone never establish currentness.
func (service *Service) ResolveWitnessed(ctx context.Context, input datasetsnapshotport.ResolveInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	binding, err := resolveBinding(input)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	head, err := service.observeFresh(ctx)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	state, err := service.loadChain(ctx, head, &binding, input)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	if state.selected == nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, datasetsnapshotport.ErrUnavailable
	}
	if err := service.confirmDatasetHead(ctx, head); err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	return *state.selected, nil
}

// Accept is a host-only workflow. Snapshot identity, predecessor, index
// generation, mutation id, and currentness are all derived by the service;
// none can be supplied by MCP, provider output, or an ordinary turn caller.
func (service *Service) Accept(ctx context.Context, input AcceptInput) (domainsecurity.DatasetSnapshotAuthorityRecordV1, error) {
	resolveInput := datasetsnapshotport.ResolveInput{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID), Observation: input.Observation,
	}
	binding, err := resolveBinding(resolveInput)
	if err != nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(input.SourceManifestHash)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(input.RawManifestSHA256)) ||
		strings.TrimSpace(input.ParserVersion) == "" || strings.TrimSpace(input.ParserVersion) != input.ParserVersion ||
		input.AcceptedAt.IsZero() {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot acceptance input is invalid"))
	}
	head, err := service.observeFresh(ctx)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	state, err := service.loadChain(ctx, head, &binding, resolveInput)
	if err != nil && !errors.Is(err, datasetsnapshotport.ErrUnavailable) {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}

	recordInput := domainsecurity.DatasetSnapshotAuthorityRecordInputV1{
		InstallationID: service.installationID, TenantID: resolveInput.TenantID, UserID: resolveInput.UserID,
		WorkspaceRealPath: input.Observation.WorkspaceRealPath, CaseID: input.Observation.CaseID,
		CaseBindingHash: input.Observation.CaseBindingHash, BindingObservationDigest: input.Observation.ObservationDigest,
		SourceManifestHash: strings.TrimSpace(input.SourceManifestHash), RawManifestSHA256: strings.TrimSpace(input.RawManifestSHA256),
		ParserVersion: input.ParserVersion, AcceptedAt: input.AcceptedAt.UTC(),
		AuthorityKeyID: service.authorityKeyID, AuthorityPublicKey: service.authorityKey,
	}
	derivedID, err := domainsecurity.DeriveDatasetSnapshotIDV1(recordInput)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	latest, hasLatest := state.latestByBinding[binding.BindingKeyDigest]
	if hasLatest && latest.DatasetSnapshotID == derivedID {
		if err := service.confirmDatasetHead(ctx, head); err != nil {
			return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
		}
		return latest, nil
	}
	if state.snapshotIDs[derivedID] {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrStale, errors.New("dataset snapshot acceptance would replay historical content"))
	}
	if hasLatest {
		recordInput.PredecessorRecordDigest = latest.RecordDigest
	}
	record, err := domainsecurity.NewDatasetSnapshotAuthorityRecordV1(recordInput, func(message []byte) ([]byte, error) {
		return service.authority.Sign(ctx, message)
	})
	if err != nil || hasLatest && domainsecurity.ValidateDatasetSnapshotAuthorityTransitionV1(latest, record) != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot record transition is invalid"))
	}

	mutationID, err := service.freshMutationID(ctx)
	if err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	if state.mutationIDs[mutationID] {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot mutation nonce was reused"))
	}
	generation := head.Bundle.DatasetSnapshotCount + 1
	previousIndexDigest := head.Bundle.DatasetSnapshotIndexDigest
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		Generation: generation, PreviousIndexDigest: previousIndexDigest, MutationID: mutationID,
		Binding: binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: service.authorityKeyID, AuthorityPublicKey: service.authorityKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil || state.headIndex != nil && domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(*state.headIndex, index) != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index transition is invalid"))
	}
	if err := service.persistCandidate(ctx, record, index); err != nil {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, err
	}
	advanced, err := advanceDatasetSnapshot(ctx, service.coordinator, evidenceauthorityport.DatasetAdvanceInput{
		ExpectedBundleDigest: head.Bundle.RecordDigest, NextIndexDigest: index.IndexDigest,
	})
	if err != nil {
		if errors.Is(err, monotonicheadport.ErrCASConflict) || errors.Is(err, monotonicheadport.ErrMutationConflict) {
			return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrStale, err)
		}
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	committed, err := service.loadChain(ctx, advanced, &binding, resolveInput)
	if err != nil || !committed.recordDigests[record.RecordDigest] {
		return domainsecurity.DatasetSnapshotAuthorityRecordV1{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("witnessed dataset snapshot candidate is absent after commit"), err)
	}
	return record, nil
}

// advanceDatasetSnapshot remains the sole production invocation point for the
// shared evidence-authority child CAS. V1 and V2 admission both use this exact
// helper so there is one independently witnessed current-head mechanism.
func advanceDatasetSnapshot(
	ctx context.Context,
	coordinator evidenceauthorityport.DatasetCoordinator,
	input evidenceauthorityport.DatasetAdvanceInput,
) (evidenceauthorityport.FreshHead, error) {
	return coordinator.AdvanceDatasetSnapshot(ctx, input)
}

func (service *Service) observeFresh(ctx context.Context) (evidenceauthorityport.FreshHead, error) {
	if service == nil || ctx == nil {
		return evidenceauthorityport.FreshHead{}, datasetsnapshotport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	head, err := service.coordinator.ObserveFresh(ctx)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	if !head.HasBundle {
		return evidenceauthorityport.FreshHead{}, datasetsnapshotport.ErrUnavailable
	}
	if err := service.validateHead(head); err != nil {
		return evidenceauthorityport.FreshHead{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	return head, nil
}

func (service *Service) validateHead(head evidenceauthorityport.FreshHead) error {
	if !head.HasBundle ||
		domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
			head.Bundle, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil ||
		domainsecurity.ValidateMonotonicHeadObserveRequestForInstallationV1(
			head.Request, service.installationID, service.authorityKeyID, service.authorityKey,
		) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationV1(head.Observation) != nil ||
		head.Observation.RequestDigest != head.Request.RequestDigest ||
		head.Observation.ChallengeNonce != head.Request.ChallengeNonce ||
		head.Request.EnrollmentID != service.enrollmentID ||
		head.Request.Namespace != domainevidence.EvidenceAuthorityBundleWitnessNamespaceV1 ||
		domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(head.Bundle, head.Observation.Checkpoint) != nil {
		return errors.New("dataset snapshot fresh witness head is invalid")
	}
	return nil
}

func (service *Service) loadChain(
	ctx context.Context,
	head evidenceauthorityport.FreshHead,
	desired *domainsecurity.DatasetSnapshotBindingKeyV1,
	resolveInput datasetsnapshotport.ResolveInput,
) (chainState, error) {
	state := chainState{
		indexDigests: map[string]bool{}, recordDigests: map[string]bool{}, snapshotIDs: map[string]bool{}, mutationIDs: map[string]bool{},
		latestByBinding: map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1{},
	}
	if err := service.validateHead(head); err != nil {
		return state, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	count := head.Bundle.DatasetSnapshotCount
	root := head.Bundle.DatasetSnapshotIndexDigest
	if count == 0 {
		if root != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
			return state, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("empty dataset snapshot root is invalid"))
		}
		return state, datasetsnapshotport.ErrUnavailable
	}
	if count > maxDatasetSnapshotIndexDepth || root == domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		return state, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot witness count is invalid"))
	}

	currentDigest := root
	var newerIndex *domainsecurity.DatasetSnapshotIndexV1
	newerByBinding := map[string]domainsecurity.DatasetSnapshotAuthorityRecordV1{}
	for expectedGeneration := count; expectedGeneration > 0; expectedGeneration-- {
		if err := ctx.Err(); err != nil {
			return chainState{}, err
		}
		if state.indexDigests[currentDigest] {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index cycle detected"))
		}
		index, err := service.indexes.Resolve(ctx, currentDigest)
		if err != nil || domainsecurity.ValidateDatasetSnapshotIndexForInstallationV1(
			index, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil || index.Generation != expectedGeneration || index.IndexDigest != currentDigest {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index ancestry is unavailable"), err)
		}
		if expectedGeneration == count && domainsecurity.ValidateDatasetSnapshotIndexWitnessRootV1(index, root, count) != nil {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index does not match witness root"))
		}
		state.indexDigests[currentDigest] = true
		if newerIndex != nil && domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(index, *newerIndex) != nil {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index lineage is invalid"))
		}
		if state.mutationIDs[index.MutationID] {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot mutation id is duplicated"))
		}
		state.mutationIDs[index.MutationID] = true
		if state.recordDigests[index.SnapshotRecordDigest] {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot record was replayed in the index"))
		}
		record, err := service.records.Resolve(ctx, index.SnapshotRecordDigest)
		if err != nil || domainsecurity.ValidateDatasetSnapshotIndexRecordForInstallationV1(
			index, record, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot authority record is unavailable"), err)
		}
		state.recordDigests[index.SnapshotRecordDigest] = true
		if state.snapshotIDs[record.DatasetSnapshotID] {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot id was replayed"))
		}
		state.snapshotIDs[record.DatasetSnapshotID] = true
		bindingDigest := index.Binding.BindingKeyDigest
		if newer, found := newerByBinding[bindingDigest]; found {
			if domainsecurity.ValidateDatasetSnapshotAuthorityTransitionV1(record, newer) != nil {
				return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot per-binding lineage is invalid"))
			}
		} else {
			state.latestByBinding[bindingDigest] = record
		}
		newerByBinding[bindingDigest] = record
		if desired != nil && state.selected == nil && index.Binding == *desired {
			if domainsecurity.ValidateDatasetSnapshotSelectionV1(
				index, record, resolveInput.TenantID, resolveInput.UserID, resolveInput.Observation,
				service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
			) != nil {
				return chainState{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot selection is mismatched"))
			}
			selected := record
			state.selected = &selected
		}
		if expectedGeneration == count {
			headIndex := index
			state.headIndex = &headIndex
		}
		newerIndex = &index
		currentDigest = index.PreviousIndexDigest
	}
	if currentDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index does not reach genesis"))
	}
	for _, oldest := range newerByBinding {
		if oldest.PredecessorRecordDigest != "" {
			return chainState{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot binding lineage does not reach genesis"))
		}
	}
	return state, nil
}

func (service *Service) confirmDatasetHead(ctx context.Context, previous evidenceauthorityport.FreshHead) error {
	current, err := service.observeFresh(ctx)
	if err != nil {
		return err
	}
	if current.Bundle.DatasetSnapshotIndexDigest != previous.Bundle.DatasetSnapshotIndexDigest ||
		current.Bundle.DatasetSnapshotCount != previous.Bundle.DatasetSnapshotCount {
		return datasetsnapshotport.ErrStale
	}
	return nil
}

func (service *Service) persistCandidate(
	ctx context.Context,
	record domainsecurity.DatasetSnapshotAuthorityRecordV1,
	index domainsecurity.DatasetSnapshotIndexV1,
) error {
	if err := service.records.PutIfAbsent(ctx, record); err != nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	writtenRecord, err := service.records.Resolve(ctx, record.RecordDigest)
	if err != nil || writtenRecord != record {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot record readback failed"), err)
	}
	if err := service.indexes.PutIfAbsent(ctx, index); err != nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	writtenIndex, err := service.indexes.Resolve(ctx, index.IndexDigest)
	if err != nil || writtenIndex != index {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot index readback failed"), err)
	}
	return nil
}

func (service *Service) freshMutationID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var random [32]byte
	if _, err := io.ReadFull(service.random, random[:]); err != nil {
		return "", errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	return domainsecurity.SHA256Hex(append([]byte("analytix.dataset-snapshot-mutation/v1\x00"), random[:]...)), nil
}

func resolveBinding(input datasetsnapshotport.ResolveInput) (domainsecurity.DatasetSnapshotBindingKeyV1, error) {
	if domainsecurity.ValidateCaseBindingObservationV1(input.Observation) != nil || input.Observation.State != domainsecurity.CaseBindingStateValid {
		return domainsecurity.DatasetSnapshotBindingKeyV1{}, errors.New("dataset snapshot binding observation is invalid")
	}
	return domainsecurity.NewDatasetSnapshotBindingKeyV1(domainsecurity.DatasetSnapshotBindingKeyInputV1{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID),
		WorkspaceRealPath: input.Observation.WorkspaceRealPath, CaseID: input.Observation.CaseID,
		CaseBindingHash: input.Observation.CaseBindingHash, BindingObservationDigest: input.Observation.ObservationDigest,
	})
}
