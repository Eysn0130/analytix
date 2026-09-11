package datasetsnapshot

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

// SealedConfigV2 deliberately reuses the V1 index, signer and shared witness.
// The legacy record store is needed to validate a possible V1 -> V2 ancestry;
// no second current/head selector is introduced.
type SealedConfigV2 struct {
	InstallationID string
	EnrollmentID   string
	Authority      finalauthorityport.Authority
	Coordinator    evidenceauthorityport.DatasetCoordinator
	LegacyRecords  datasetsnapshotport.RecordStore
	Bundles        datasetsnapshotport.AuthorityBundleStoreV2
	Indexes        datasetsnapshotport.IndexStore
	Materials      datasetsnapshotport.AdmissionMaterialReaderV2
	Random         io.Reader
}

type SealedServiceV2 struct {
	*Service
	bundles   datasetsnapshotport.AuthorityBundleStoreV2
	materials datasetsnapshotport.AdmissionMaterialReaderV2
}

type freshHeadChallengeSourceV2 interface {
	WithFreshHeadChallenge(
		context.Context,
		func(evidenceauthorityport.FreshHead, func(context.Context) error) error,
	) error
}

var _ datasetsnapshotport.AuthorityV2 = (*SealedServiceV2)(nil)
var _ datasetsnapshotport.CurrentAuthorityV2 = (*SealedServiceV2)(nil)
var _ datasetsnapshotport.RetainedSelectionReaderV2 = (*SealedServiceV2)(nil)

// AdmitInputV2 contains only exact content addresses and host request scope.
// It cannot carry a manifest, producer payload, record id, predecessor,
// generation, mutation id, or current-head claim.
type AdmitInputV2 struct {
	TenantID    string
	UserID      string
	Observation domainsecurity.CaseBindingObservationV1

	ManifestReference      datasetsnapshotport.ExactMaterialReferenceV2
	FundsProducerReference datasetsnapshotport.ExactMaterialReferenceV2
	AcceptedAt             time.Time
}

type versionedSnapshotNodeV2 struct {
	index   domainsecurity.DatasetSnapshotIndexV1
	record  domainsecurity.VersionedDatasetSnapshotAuthorityRecord
	bundle  *datasetsnapshotport.AuthorityBundleV2
	binding domainsecurity.DatasetSnapshotBindingKeyV1
}

type versionedChainStateV2 struct {
	headIndex       *domainsecurity.DatasetSnapshotIndexV1
	selected        *versionedSnapshotNodeV2
	indexPath       []domainsecurity.DatasetSnapshotIndexV1
	indexDigests    map[string]bool
	recordDigests   map[string]bool
	snapshotIDs     map[string]bool
	producerIDs     map[string]bool
	mutationIDs     map[string]bool
	latestByBinding map[string]versionedSnapshotNodeV2
}

type verifiedSnapshotMaterialV2 struct {
	manifest domainsecurity.DatasetSnapshotManifestV2
	producer domainsecurity.FundsProducerContentManifestV1
}

func NewSealedV2(config SealedConfigV2) (*SealedServiceV2, error) {
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	base, err := New(Config{
		InstallationID: config.InstallationID,
		EnrollmentID:   config.EnrollmentID,
		Authority:      config.Authority,
		Coordinator:    config.Coordinator,
		Records:        config.LegacyRecords,
		Indexes:        config.Indexes,
		Random:         config.Random,
	})
	if err != nil || config.Bundles == nil || config.Materials == nil {
		return nil, errors.New("sealed dataset snapshot v2 service configuration is invalid")
	}
	return &SealedServiceV2{Service: base, bundles: config.Bundles, materials: config.Materials}, nil
}

// AdmitExactV2 verifies the complete parent-addressed private-CAS graph before
// a callback-scoped one-use domain issuer can sign. Persisted candidates stay
// inert until the existing shared witness CAS selects their index node.
func (service *SealedServiceV2) AdmitExactV2(
	ctx context.Context,
	input AdmitInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	return service.admitExactV2(ctx, input, "")
}

// AdmitAfterExactV2 is the cleaning producer's expected-input variant of the
// same sealed admission and witness CAS. It introduces no second head: the
// expected DSV2 must be the binding's selected node at the fresh CAS base.
type AdmitAfterInputV2 struct {
	AdmitInputV2
	ExpectedCurrentDatasetSnapshotID string
}

func (service *SealedServiceV2) AdmitAfterExactV2(
	ctx context.Context,
	input AdmitAfterInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if !domainsecurity.IsDatasetSnapshotIDV2Syntax(input.ExpectedCurrentDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(
			datasetsnapshotport.ErrMismatch,
			errors.New("sealed dataset snapshot v2 expected predecessor is invalid"),
		)
	}
	return service.admitExactV2(ctx, input.AdmitInputV2, input.ExpectedCurrentDatasetSnapshotID)
}

func (service *SealedServiceV2) admitExactV2(
	ctx context.Context,
	input AdmitInputV2,
	expectedCurrentDatasetSnapshotID string,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID), Observation: input.Observation,
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{
		TenantID: resolveInput.TenantID, UserID: resolveInput.UserID, Observation: resolveInput.Observation,
	})
	if err != nil || input.AcceptedAt.IsZero() {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("sealed dataset snapshot v2 admission input is invalid"))
	}
	head, err := service.observeFresh(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	state, err := service.loadVersionedChainV2(ctx, head, &binding, resolveInput)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	latest, hasLatest := state.latestByBinding[binding.BindingKeyDigest]
	if expectedCurrentDatasetSnapshotID != "" && (!hasLatest || latest.record.V2 == nil ||
		latest.record.V2.DatasetSnapshotID != expectedCurrentDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(
			datasetsnapshotport.ErrStale,
			errors.New("dataset snapshot v2 expected predecessor is not current"),
		)
	}
	material, err := service.verifyExactMaterialV2(ctx, input.ManifestReference, input.FundsProducerReference)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if material.manifest.Binding != binding {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot v2 manifest binding mismatch"))
	}
	derivedID := domainsecurity.DeriveDatasetSnapshotIDV2(material.manifest)
	if hasLatest && latest.record.V2 != nil && latest.record.V2.DatasetSnapshotID == derivedID {
		resolved, err := service.resolveNodeMaterialV2(ctx, latest, resolveInput)
		if err != nil {
			return datasetsnapshotport.ResolvedSnapshotV2{}, err
		}
		if err := service.confirmDatasetHead(ctx, head); err != nil {
			return datasetsnapshotport.ResolvedSnapshotV2{}, err
		}
		return resolved, nil
	}
	if state.snapshotIDs[derivedID] || state.producerIDs[material.manifest.ProducerContentID] {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale, errors.New("dataset snapshot v2 content would be reminted"))
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
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot v2 record transition is invalid"))
	}

	mutationID, err := service.freshMutationID(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if state.mutationIDs[mutationID] {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot mutation nonce was reused"))
	}
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		Generation: head.Bundle.DatasetSnapshotCount + 1, PreviousIndexDigest: head.Bundle.DatasetSnapshotIndexDigest,
		MutationID: mutationID, Binding: binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: service.authorityKeyID, AuthorityPublicKey: service.authorityKey,
	}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil || state.headIndex != nil && domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(*state.headIndex, index) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexRecordForInstallationV2(
			index, record, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index transition is invalid"))
	}
	bundle := datasetsnapshotport.AuthorityBundleV2{Record: record, Manifest: material.manifest}
	if err := service.persistCandidateV2(ctx, bundle, index); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	advanced, err := advanceDatasetSnapshot(ctx, service.coordinator, evidenceauthorityport.DatasetAdvanceInput{
		ExpectedBundleDigest: head.Bundle.RecordDigest, NextIndexDigest: index.IndexDigest,
	})
	if err != nil {
		if errors.Is(err, monotonicheadport.ErrCASConflict) || errors.Is(err, monotonicheadport.ErrMutationConflict) {
			return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale, err)
		}
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	committed, err := service.loadVersionedChainV2(ctx, advanced, &binding, resolveInput)
	if err != nil || committed.selected == nil || committed.selected.record.V2 == nil ||
		committed.selected.record.V2.RecordDigest != record.RecordDigest {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("witnessed dataset snapshot v2 candidate is absent after commit"), err)
	}
	resolved, err := service.resolveNodeMaterialV2(ctx, *committed.selected, resolveInput)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if err := service.confirmDatasetHead(ctx, advanced); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	return resolved, nil
}

func (service *SealedServiceV2) ResolveWitnessedV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID), Observation: input.Observation,
	})
	if err != nil || input.ExpectedDatasetSnapshotID != "" && !domainsecurity.IsDatasetSnapshotIDV2Syntax(input.ExpectedDatasetSnapshotID) {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot v2 resolve input is invalid"))
	}
	head, err := service.observeFresh(ctx)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	state, err := service.loadVersionedChainV2(ctx, head, &binding, input)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if state.selected == nil || state.selected.record.V2 == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	if input.ExpectedDatasetSnapshotID != "" && state.selected.record.V2.DatasetSnapshotID != input.ExpectedDatasetSnapshotID {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrStale, errors.New("dataset snapshot v2 expected id is not current"))
	}
	resolved, err := service.resolveNodeMaterialV2(ctx, *state.selected, input)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if err := service.confirmDatasetHead(ctx, head); err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	return resolved, nil
}

type currentSelectionCapabilityV2 struct {
	mu                 sync.Mutex
	condition          *sync.Cond
	active             bool
	activeUses         uint64
	registryEffectUsed bool
	ctx                context.Context
	service            *SealedServiceV2
	input              datasetsnapshotport.ResolveInputV2
	securityContext    domainsecurity.TurnSecurityContext
	selection          datasetsnapshotport.CurrentSelectionV2
	freshHead          func(context.Context) error
}

// WithCurrentSelectionV2 resolves the complete DSV2 graph from one fresh
// shared EvidenceAuthority witness head and keeps currentness usable only
// inside callback. The returned selection is a clone; retaining it does not
// retain the opaque capability.
func (service *SealedServiceV2) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if service == nil || ctx == nil || callback == nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, errors.New("current dataset snapshot capability input is invalid"))
	}
	if err := validateCurrentResolveInputV2(input, securityContext); err != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	binding, err := resolveBinding(datasetsnapshotport.ResolveInput{
		TenantID: strings.TrimSpace(input.TenantID), UserID: strings.TrimSpace(input.UserID), Observation: input.Observation,
	})
	if err != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	if source, ok := service.coordinator.(freshHeadChallengeSourceV2); ok {
		callbackEntered := false
		err := source.WithFreshHeadChallenge(
			ctx,
			func(
				head evidenceauthorityport.FreshHead,
				challenge func(context.Context) error,
			) error {
				callbackEntered = true
				return service.withCurrentSelectionV2(
					ctx, input, securityContext, callback, binding, head, challenge,
				)
			},
		)
		if err != nil && !callbackEntered {
			return errors.Join(datasetsnapshotport.ErrUnavailable, err)
		}
		return err
	}
	head, err := service.observeFresh(ctx)
	if err != nil {
		return err
	}
	return service.withCurrentSelectionV2(
		ctx, input, securityContext, callback, binding, head, nil,
	)
}

func (service *SealedServiceV2) withCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
	binding domainsecurity.DatasetSnapshotBindingKeyV1,
	head evidenceauthorityport.FreshHead,
	freshHeadChallenge func(context.Context) error,
) error {
	if !head.HasBundle {
		return datasetsnapshotport.ErrUnavailable
	}
	if err := service.validateHead(head); err != nil {
		return errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	state, err := service.loadVersionedChainV2(ctx, head, &binding, input)
	if err != nil {
		return err
	}
	if state.selected == nil || state.selected.record.V2 == nil || state.headIndex == nil {
		return datasetsnapshotport.ErrUnavailable
	}
	if state.selected.record.V2.DatasetSnapshotID != input.ExpectedDatasetSnapshotID {
		return errors.Join(datasetsnapshotport.ErrStale, errors.New("current dataset snapshot id is not selected"))
	}
	resolved, err := service.resolveNodeMaterialV2(ctx, *state.selected, input)
	if err != nil {
		return err
	}
	confirmed, err := service.confirmSharedEvidenceHeadV2(ctx, head)
	if err != nil {
		return err
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		Head:             confirmed,
		DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), state.indexPath...),
		SelectedIndex:    state.selected.index,
		Snapshot:         resolved,
	}
	selection.SelectionDigest, err = currentSelectionDigestV2(selection)
	if err != nil {
		return errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	if err := service.validateCurrentSelectionV2(selection, input, securityContext); err != nil {
		return err
	}
	frozen := cloneCurrentSelectionV2(selection)
	capability := &currentSelectionCapabilityV2{
		active: true, ctx: ctx, service: service, input: input,
		securityContext: securityContext, selection: frozen,
		freshHead: freshHeadChallenge,
	}
	capability.condition = sync.NewCond(&capability.mu)
	defer capability.close()
	if err := callback(cloneCurrentSelectionV2(selection), capability); err != nil {
		return err
	}
	return ctx.Err()
}

// WithRetainedSelectionV2 keeps the existing current DSV2 capability live
// while locating one caller-specified historical dsv2_ record in the exact
// witnessed index path. It never scans storage or selects a latest record.
// Both the retained immutable bundle/material and the current witness are
// challenged before and after the callback.
func (service *SealedServiceV2) WithRetainedSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.RetainedSelectionInputV2,
	callback func(context.Context, datasetsnapshotport.RetainedSelectionV2) error,
) error {
	if service == nil || ctx == nil || callback == nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, errors.New("retained dataset snapshot capability input is invalid"))
	}
	if err := validateRetainedSelectionInputV2(input); err != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, err)
	}
	return service.WithCurrentSelectionV2(
		ctx,
		input.CurrentResolveInput,
		input.CurrentSecurityContext,
		func(
			current datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			return capability.UseExact(
				current,
				input.CurrentSecurityContext,
				func(leaseContext context.Context) error {
					if err := service.verifyCurrentSelectionMaterialV2(
						leaseContext, current, input.CurrentResolveInput,
					); err != nil {
						return err
					}
					retained, err := service.resolveRetainedSelectionV2(
						leaseContext, current, input,
					)
					if err != nil {
						return err
					}
					callbackErr := callback(leaseContext, cloneRetainedSelectionV2(retained))
					post, postErr := service.resolveRetainedSelectionV2(
						leaseContext, current, input,
					)
					if postErr == nil && !retainedSelectionEqualV2(retained, post) {
						postErr = errors.Join(
							datasetsnapshotport.ErrCorrupt,
							errors.New("retained dataset snapshot material changed during use"),
						)
					}
					currentPostErr := service.verifyCurrentSelectionMaterialV2(
						leaseContext, current, input.CurrentResolveInput,
					)
					if callbackErr != nil || postErr != nil || currentPostErr != nil {
						return errors.Join(
							callbackErr,
							postErr,
							currentPostErr,
							errors.New("retained dataset snapshot use did not remain exact"),
						)
					}
					return nil
				},
			)
		},
	)
}

func (service *SealedServiceV2) verifyCurrentSelectionMaterialV2(
	ctx context.Context,
	selection datasetsnapshotport.CurrentSelectionV2,
	input datasetsnapshotport.ResolveInputV2,
) error {
	node, err := service.resolveVersionedNodeV2(ctx, selection.SelectedIndex)
	if err != nil {
		return err
	}
	resolved, err := service.resolveNodeMaterialV2(ctx, node, input)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(resolved, selection.Snapshot) {
		return errors.Join(
			datasetsnapshotport.ErrCorrupt,
			errors.New("current dataset snapshot material changed during use"),
		)
	}
	return nil
}

func validateRetainedSelectionInputV2(
	input datasetsnapshotport.RetainedSelectionInputV2,
) error {
	if validateCurrentResolveInputV2(
		input.CurrentResolveInput,
		input.CurrentSecurityContext,
	) != nil ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(input.RetainedDatasetSnapshotID) ||
		input.RetainedDatasetSnapshotID != strings.TrimSpace(input.RetainedDatasetSnapshotID) ||
		!domainsecurity.IsSHA256Hex(input.RetainedSourceManifestHash) ||
		input.RetainedSourceManifestHash != strings.TrimSpace(input.RetainedSourceManifestHash) {
		return errors.New("retained dataset snapshot selection input is invalid")
	}
	return nil
}

func (service *SealedServiceV2) resolveRetainedSelectionV2(
	ctx context.Context,
	current datasetsnapshotport.CurrentSelectionV2,
	input datasetsnapshotport.RetainedSelectionInputV2,
) (datasetsnapshotport.RetainedSelectionV2, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx == nil {
			return datasetsnapshotport.RetainedSelectionV2{}, datasetsnapshotport.ErrUnavailable
		}
		return datasetsnapshotport.RetainedSelectionV2{}, ctx.Err()
	}
	for _, index := range current.DatasetIndexPath {
		node, err := service.resolveVersionedNodeV2(ctx, index)
		if err != nil {
			return datasetsnapshotport.RetainedSelectionV2{}, err
		}
		if node.record.V2 == nil ||
			node.record.V2.DatasetSnapshotID != input.RetainedDatasetSnapshotID {
			continue
		}
		if node.bundle == nil ||
			node.record.V2.SourceManifestHash != input.RetainedSourceManifestHash ||
			node.bundle.Manifest.SourceManifestHash != input.RetainedSourceManifestHash {
			return datasetsnapshotport.RetainedSelectionV2{}, errors.Join(
				datasetsnapshotport.ErrMismatch,
				errors.New("retained dataset snapshot source manifest is not exact"),
			)
		}
		resolved, err := service.resolveNodeMaterialV2(
			ctx, node, input.CurrentResolveInput,
		)
		if err != nil {
			return datasetsnapshotport.RetainedSelectionV2{}, err
		}
		if resolved.Record.DatasetSnapshotID != input.RetainedDatasetSnapshotID ||
			resolved.Record.SourceManifestHash != input.RetainedSourceManifestHash ||
			resolved.Manifest.SourceManifestHash != input.RetainedSourceManifestHash {
			return datasetsnapshotport.RetainedSelectionV2{}, errors.Join(
				datasetsnapshotport.ErrMismatch,
				errors.New("retained dataset snapshot graph is not exact"),
			)
		}
		return datasetsnapshotport.RetainedSelectionV2{
			Current:       cloneCurrentSelectionV2(current),
			SelectedIndex: node.index,
			Snapshot:      resolved,
		}, nil
	}
	return datasetsnapshotport.RetainedSelectionV2{}, errors.Join(
		datasetsnapshotport.ErrNotFound,
		errors.New("retained dataset snapshot is not a witnessed current-path member"),
	)
}

func cloneRetainedSelectionV2(
	selection datasetsnapshotport.RetainedSelectionV2,
) datasetsnapshotport.RetainedSelectionV2 {
	selection.Current = cloneCurrentSelectionV2(selection.Current)
	return selection
}

func retainedSelectionEqualV2(
	left datasetsnapshotport.RetainedSelectionV2,
	right datasetsnapshotport.RetainedSelectionV2,
) bool {
	return left.SelectedIndex == right.SelectedIndex &&
		reflect.DeepEqual(left.Snapshot, right.Snapshot)
}

func (capability *currentSelectionCapabilityV2) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	return capability.useExact(selection, securityContext, use, false)
}

// UsePostNativeExact is intentionally reachable only through a structural
// interface held by the Funds callback. It requires one already-live outer
// UseExact and substitutes only this nested use's pre/post full observation
// with the exact-bound EvidenceAuthority challenge issued for the selection.
func (capability *currentSelectionCapabilityV2) UsePostNativeExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	return capability.useExact(selection, securityContext, use, true)
}

func (capability *currentSelectionCapabilityV2) useExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
	postNative bool,
) error {
	began := capability != nil && use != nil && ((!postNative && capability.beginUse()) ||
		(postNative && capability.beginPostNativeUse()))
	if !began {
		return errors.Join(datasetsnapshotport.ErrUnavailable, errors.New("current dataset snapshot capability is inactive"))
	}
	defer capability.endUse()

	if securityContext != capability.securityContext ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		capability.service == nil ||
		capability.service.validateCurrentSelectionV2(selection, capability.input, securityContext) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("current dataset snapshot capability does not authorize this selection"))
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	confirmCurrent := func() error {
		if postNative && capability.freshHead != nil {
			return capability.freshHead(leaseContext)
		}
		_, err := capability.service.confirmSharedEvidenceHeadV2(leaseContext, capability.selection.Head)
		return err
	}
	if err := confirmCurrent(); err != nil {
		return err
	}

	useErr := use(leaseContext)
	currentErr := confirmCurrent()
	afterErr := capability.service.validateCurrentSelectionV2(selection, capability.input, securityContext)
	switch {
	case useErr != nil || currentErr != nil || afterErr != nil || leaseContext.Err() != nil:
		return errors.Join(
			useErr,
			currentErr,
			afterErr,
			leaseContext.Err(),
			errors.New("current dataset snapshot capability use did not remain exact"),
		)
	default:
		return nil
	}
}

func (*currentSelectionCapabilityV2) MarshalJSON() ([]byte, error) {
	return nil, errors.New("current dataset snapshot capability is not serializable")
}

func (capability *currentSelectionCapabilityV2) beginUse() bool {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil {
		return false
	}
	capability.activeUses++
	return true
}

func (capability *currentSelectionCapabilityV2) beginPostNativeUse() bool {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || capability.activeUses != 1 {
		return false
	}
	capability.activeUses++
	return true
}

func (capability *currentSelectionCapabilityV2) endUse() {
	capability.mu.Lock()
	if capability.activeUses > 0 {
		capability.activeUses--
	}
	if capability.activeUses == 0 && capability.condition != nil {
		capability.condition.Broadcast()
	}
	capability.mu.Unlock()
}

func (capability *currentSelectionCapabilityV2) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	for capability.activeUses > 0 {
		capability.condition.Wait()
	}
	capability.freshHead = nil
	capability.mu.Unlock()
}

func validateCurrentResolveInputV2(
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainsecurity.ValidateCaseBindingObservationV1(input.Observation) != nil ||
		input.Observation.State != domainsecurity.CaseBindingStateValid ||
		input.TenantID != strings.TrimSpace(input.TenantID) || input.UserID != strings.TrimSpace(input.UserID) ||
		input.ExpectedDatasetSnapshotID != strings.TrimSpace(input.ExpectedDatasetSnapshotID) ||
		input.TenantID != securityContext.TenantID || input.UserID != securityContext.UserID ||
		input.Observation.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		input.Observation.CaseID != securityContext.CaseID ||
		input.Observation.CaseBindingHash != securityContext.CaseBindingHash ||
		input.Observation.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest ||
		input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID {
		return errors.New("current dataset snapshot resolve input does not match turn security context")
	}
	return nil
}

func (service *SealedServiceV2) validateCurrentSelectionV2(
	selection datasetsnapshotport.CurrentSelectionV2,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if service == nil || validateCurrentResolveInputV2(input, securityContext) != nil ||
		service.validateHead(selection.Head) != nil ||
		len(selection.DatasetIndexPath) == 0 ||
		len(selection.DatasetIndexPath) > maxDatasetSnapshotIndexDepth ||
		uint64(len(selection.DatasetIndexPath)) != selection.Head.Bundle.DatasetSnapshotCount ||
		selection.Head.Bundle.DatasetSnapshotIndexDigest != selection.DatasetIndexPath[0].IndexDigest ||
		selection.Snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		selection.Snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash ||
		selection.Snapshot.Manifest.SourceManifestHash != securityContext.SourceManifestHash ||
		selection.Snapshot.Record.Binding != selection.Snapshot.Manifest.Binding ||
		selection.SelectedIndex.Binding != selection.Snapshot.Record.Binding ||
		selection.SelectedIndex.SnapshotRecordDigest != selection.Snapshot.Record.RecordDigest ||
		domainsecurity.ValidateDatasetSnapshotAuthorityRecordForFundsProducerContentV2(
			selection.Snapshot.Record, selection.Snapshot.Manifest, selection.Snapshot.FundsProducerContent,
		) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexNodeForManifestV2(
			selection.SelectedIndex, selection.Snapshot.Record, selection.Snapshot.Manifest,
			selection.Snapshot.FundsProducerContent, input.TenantID, input.UserID, input.Observation,
			service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("current dataset snapshot selection binding is invalid"))
	}
	seenIndex := make(map[string]bool, len(selection.DatasetIndexPath))
	selectedCount := 0
	for offset, index := range selection.DatasetIndexPath {
		expectedGeneration := selection.Head.Bundle.DatasetSnapshotCount - uint64(offset)
		if seenIndex[index.IndexDigest] ||
			index.Generation != expectedGeneration ||
			domainsecurity.ValidateDatasetSnapshotIndexForInstallationV1(
				index, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
			) != nil {
			return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("current dataset snapshot index path is invalid"))
		}
		seenIndex[index.IndexDigest] = true
		if index == selection.SelectedIndex {
			selectedCount++
		}
		if offset == 0 {
			if domainsecurity.ValidateDatasetSnapshotIndexWitnessRootV1(
				index,
				selection.Head.Bundle.DatasetSnapshotIndexDigest,
				selection.Head.Bundle.DatasetSnapshotCount,
			) != nil {
				return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("current dataset snapshot root is invalid"))
			}
			continue
		}
		if domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(index, selection.DatasetIndexPath[offset-1]) != nil {
			return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("current dataset snapshot index ancestry is invalid"))
		}
	}
	if selectedCount != 1 ||
		selection.DatasetIndexPath[len(selection.DatasetIndexPath)-1].PreviousIndexDigest !=
			domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("current dataset snapshot selection is not an exact path member"))
	}
	digest, err := currentSelectionDigestV2(selection)
	if err != nil || selection.SelectionDigest != digest {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("current dataset snapshot selection digest is invalid"), err)
	}
	return nil
}

func (service *SealedServiceV2) confirmSharedEvidenceHeadV2(
	ctx context.Context,
	expected evidenceauthorityport.FreshHead,
) (evidenceauthorityport.FreshHead, error) {
	current, err := service.observeFresh(ctx)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	if !expected.HasBundle || !current.HasBundle || current.Bundle != expected.Bundle {
		return evidenceauthorityport.FreshHead{}, errors.Join(
			datasetsnapshotport.ErrStale,
			errors.New("shared evidence authority head changed during dataset snapshot use"),
		)
	}
	return current, nil
}

func currentSelectionDigestV2(selection datasetsnapshotport.CurrentSelectionV2) (string, error) {
	selection.SelectionDigest = ""
	body, err := json.Marshal(selection)
	if err != nil {
		return "", errors.New("current dataset snapshot selection cannot be frozen")
	}
	return domainsecurity.SHA256Hex(append(
		[]byte("analytix.current-dataset-snapshot-selection/v2\x00"),
		body...,
	)), nil
}

func cloneCurrentSelectionV2(selection datasetsnapshotport.CurrentSelectionV2) datasetsnapshotport.CurrentSelectionV2 {
	selection.DatasetIndexPath = append([]domainsecurity.DatasetSnapshotIndexV1(nil), selection.DatasetIndexPath...)
	return selection
}

func (service *SealedServiceV2) resolveNodeMaterialV2(
	ctx context.Context,
	node versionedSnapshotNodeV2,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	if node.bundle == nil || node.record.V2 == nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrUnavailable
	}
	manifestBody, err := domainsecurity.DatasetSnapshotManifestV2Bytes(node.bundle.Manifest)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	manifestReference := exactReferenceV2(domainsecurity.SHA256Hex(manifestBody), uint64(len(manifestBody)))
	producerReference := exactReferenceV2(
		node.bundle.Manifest.ProducerContentManifestSHA256,
		node.bundle.Manifest.ProducerContentManifestByteLength,
	)
	material, err := service.verifyExactMaterialV2(ctx, manifestReference, producerReference)
	if err != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, err
	}
	if material.manifest != node.bundle.Manifest ||
		domainsecurity.ValidateDatasetSnapshotIndexNodeForManifestV2(
			node.index, *node.record.V2, material.manifest, material.producer,
			strings.TrimSpace(input.TenantID), strings.TrimSpace(input.UserID), input.Observation,
			service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil {
		return datasetsnapshotport.ResolvedSnapshotV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot v2 selected node material is mismatched"))
	}
	return datasetsnapshotport.ResolvedSnapshotV2{
		Record: *node.record.V2, Manifest: material.manifest, FundsProducerContent: material.producer,
	}, nil
}

func (service *SealedServiceV2) persistCandidateV2(
	ctx context.Context,
	bundle datasetsnapshotport.AuthorityBundleV2,
	index domainsecurity.DatasetSnapshotIndexV1,
) error {
	if err := service.bundles.PutIfAbsent(ctx, bundle); err != nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	writtenBundle, err := service.bundles.Resolve(ctx, bundle.Record.RecordDigest)
	if err != nil || writtenBundle.Record != bundle.Record || writtenBundle.Manifest != bundle.Manifest {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 authority bundle readback failed"), err)
	}
	if err := service.indexes.PutIfAbsent(ctx, index); err != nil {
		return errors.Join(datasetsnapshotport.ErrUnavailable, err)
	}
	writtenIndex, err := service.indexes.Resolve(ctx, index.IndexDigest)
	if err != nil || writtenIndex != index {
		return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index readback failed"), err)
	}
	return nil
}

func (service *SealedServiceV2) loadVersionedChainV2(
	ctx context.Context,
	head evidenceauthorityport.FreshHead,
	desired *domainsecurity.DatasetSnapshotBindingKeyV1,
	input datasetsnapshotport.ResolveInputV2,
) (versionedChainStateV2, error) {
	state := versionedChainStateV2{
		indexDigests: map[string]bool{}, recordDigests: map[string]bool{}, snapshotIDs: map[string]bool{},
		producerIDs: map[string]bool{}, mutationIDs: map[string]bool{}, latestByBinding: map[string]versionedSnapshotNodeV2{},
	}
	if err := service.validateHead(head); err != nil {
		return state, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	count := head.Bundle.DatasetSnapshotCount
	currentDigest := head.Bundle.DatasetSnapshotIndexDigest
	if count == 0 {
		if currentDigest != domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
			return state, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("empty dataset snapshot v2 root is invalid"))
		}
		return state, nil
	}
	if count > maxDatasetSnapshotIndexDepth || currentDigest == domainsecurity.DatasetSnapshotIndexGenesisDigestV1() {
		return state, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 witness count is invalid"))
	}
	var newerIndex *domainsecurity.DatasetSnapshotIndexV1
	newerByBinding := map[string]domainsecurity.VersionedDatasetSnapshotAuthorityRecord{}
	for expectedGeneration := count; expectedGeneration > 0; expectedGeneration-- {
		if err := ctx.Err(); err != nil {
			return versionedChainStateV2{}, err
		}
		if state.indexDigests[currentDigest] {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index cycle detected"))
		}
		index, err := service.indexes.Resolve(ctx, currentDigest)
		if err != nil || domainsecurity.ValidateDatasetSnapshotIndexForInstallationV1(
			index, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil || index.Generation != expectedGeneration || index.IndexDigest != currentDigest {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index ancestry is unavailable"), err)
		}
		if expectedGeneration == count && domainsecurity.ValidateDatasetSnapshotIndexWitnessRootV1(index, currentDigest, count) != nil {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index does not match witness root"))
		}
		if newerIndex != nil && domainsecurity.ValidateDatasetSnapshotIndexTransitionV1(index, *newerIndex) != nil {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index lineage is invalid"))
		}
		if state.mutationIDs[index.MutationID] || state.recordDigests[index.SnapshotRecordDigest] {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index replay detected"))
		}
		state.indexDigests[currentDigest] = true
		state.mutationIDs[index.MutationID] = true
		state.recordDigests[index.SnapshotRecordDigest] = true
		state.indexPath = append(state.indexPath, index)
		node, err := service.resolveVersionedNodeV2(ctx, index)
		if err != nil {
			return versionedChainStateV2{}, err
		}
		snapshotID := versionedSnapshotIDV2(node.record)
		if state.snapshotIDs[snapshotID] {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 id replay detected"))
		}
		state.snapshotIDs[snapshotID] = true
		if node.bundle != nil {
			producerID := node.bundle.Manifest.ProducerContentID
			if state.producerIDs[producerID] {
				return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 producer content replay detected"))
			}
			state.producerIDs[producerID] = true
		}
		bindingDigest := node.binding.BindingKeyDigest
		if newer, found := newerByBinding[bindingDigest]; found {
			if domainsecurity.ValidateVersionedDatasetSnapshotAuthorityTransition(node.record, newer) != nil {
				return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 per-binding lineage is invalid"))
			}
		} else {
			state.latestByBinding[bindingDigest] = node
		}
		newerByBinding[bindingDigest] = node.record
		if desired != nil && state.selected == nil && index.Binding == *desired {
			if domainsecurity.ValidateDatasetSnapshotBindingKeyForObservationV1(
				index.Binding, strings.TrimSpace(input.TenantID), strings.TrimSpace(input.UserID), input.Observation,
			) != nil {
				return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot v2 selection is mismatched"))
			}
			selected := node
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
		return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 index does not reach genesis"))
	}
	for _, oldest := range newerByBinding {
		if versionedPredecessorDigestV2(oldest) != "" {
			return versionedChainStateV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 binding lineage does not reach genesis"))
		}
	}
	return state, nil
}

func (service *SealedServiceV2) resolveVersionedNodeV2(
	ctx context.Context,
	index domainsecurity.DatasetSnapshotIndexV1,
) (versionedSnapshotNodeV2, error) {
	bundle, bundleErr := service.bundles.Resolve(ctx, index.SnapshotRecordDigest)
	legacy, legacyErr := service.records.Resolve(ctx, index.SnapshotRecordDigest)
	bundleFound := bundleErr == nil
	legacyFound := legacyErr == nil
	if bundleFound == legacyFound {
		return versionedSnapshotNodeV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot record version is absent or ambiguous"), bundleErr, legacyErr)
	}
	if !bundleFound && !errors.Is(bundleErr, datasetsnapshotport.ErrNotFound) ||
		!legacyFound && !errors.Is(legacyErr, datasetsnapshotport.ErrNotFound) {
		return versionedSnapshotNodeV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot record store failed closed"), bundleErr, legacyErr)
	}
	if bundleFound {
		if domainsecurity.ValidateDatasetSnapshotIndexRecordForInstallationV2(
			index, bundle.Record, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
		) != nil || domainsecurity.ValidateDatasetSnapshotAuthorityRecordForManifestV2(bundle.Record, bundle.Manifest) != nil {
			return versionedSnapshotNodeV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot v2 record is invalid"))
		}
		record := bundle.Record
		versioned := domainsecurity.VersionedDatasetSnapshotAuthorityRecord{
			SchemaVersion: domainsecurity.DatasetSnapshotAuthorityRecordSchemaVersionV2, V2: &record,
		}
		return versionedSnapshotNodeV2{index: index, record: versioned, bundle: &bundle, binding: bundle.Record.Binding}, nil
	}
	if domainsecurity.ValidateDatasetSnapshotIndexRecordForInstallationV1(
		index, legacy, service.installationID, service.enrollmentID, service.authorityKeyID, service.authorityKey,
	) != nil {
		return versionedSnapshotNodeV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("legacy dataset snapshot record is invalid"))
	}
	binding, err := domainsecurity.DatasetSnapshotBindingKeyFromRecordV1(legacy)
	if err != nil {
		return versionedSnapshotNodeV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	versioned := domainsecurity.VersionedDatasetSnapshotAuthorityRecord{
		SchemaVersion: domainsecurity.DatasetSnapshotAuthorityRecordSchemaVersion, V1: &legacy,
	}
	return versionedSnapshotNodeV2{index: index, record: versioned, binding: binding}, nil
}

func versionedRecordDigestV2(record domainsecurity.VersionedDatasetSnapshotAuthorityRecord) string {
	if record.V2 != nil {
		return record.V2.RecordDigest
	}
	if record.V1 != nil {
		return record.V1.RecordDigest
	}
	return ""
}

func versionedSnapshotIDV2(record domainsecurity.VersionedDatasetSnapshotAuthorityRecord) string {
	if record.V2 != nil {
		return record.V2.DatasetSnapshotID
	}
	if record.V1 != nil {
		return record.V1.DatasetSnapshotID
	}
	return ""
}

func versionedPredecessorDigestV2(record domainsecurity.VersionedDatasetSnapshotAuthorityRecord) string {
	if record.V2 != nil {
		return record.V2.PredecessorRecordDigest
	}
	if record.V1 != nil {
		return record.V1.PredecessorRecordDigest
	}
	return ""
}

func exactReferenceV2(sha256 string, byteLength uint64) datasetsnapshotport.ExactMaterialReferenceV2 {
	return datasetsnapshotport.ExactMaterialReferenceV2{Address: sha256, SHA256: sha256, ByteLength: byteLength}
}

func (service *SealedServiceV2) readExactTwiceV2(
	ctx context.Context,
	kind datasetsnapshotport.MaterialKindV2,
	reference datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	if reference.Address != reference.SHA256 || !domainsecurity.IsSHA256Hex(reference.SHA256) || reference.ByteLength == 0 {
		return nil, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot exact material reference is invalid"))
	}
	first, err := service.materials.ResolveExact(ctx, kind, reference)
	if err != nil {
		return nil, err
	}
	second, err := service.materials.ResolveExact(ctx, kind, reference)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) || uint64(len(first)) != reference.ByteLength ||
		domainsecurity.SHA256Hex(first) != reference.SHA256 {
		return nil, errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot private CAS double read is inconsistent"))
	}
	return first, nil
}

func (service *SealedServiceV2) verifyExactMaterialV2(
	ctx context.Context,
	manifestReference datasetsnapshotport.ExactMaterialReferenceV2,
	producerReference datasetsnapshotport.ExactMaterialReferenceV2,
) (verifiedSnapshotMaterialV2, error) {
	manifestBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialSnapshotManifestV2, manifestReference)
	if err != nil {
		return verifiedSnapshotMaterialV2{}, err
	}
	manifest, err := domainsecurity.ParseDatasetSnapshotManifestV2(manifestBody)
	if err != nil {
		return verifiedSnapshotMaterialV2{}, errors.Join(datasetsnapshotport.ErrCorrupt, err)
	}
	producerBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialFundsProducerContentV1, producerReference)
	if err != nil {
		return verifiedSnapshotMaterialV2{}, err
	}
	producer, err := domainsecurity.ParseFundsProducerContentManifestV1(producerBody)
	if err != nil || manifest.ProducerContentManifestSHA256 != producerReference.SHA256 ||
		manifest.ProducerContentManifestByteLength != producerReference.ByteLength ||
		domainsecurity.ValidateDatasetSnapshotManifestV2FundsProducerContentV1(manifest, producer) != nil ||
		producer.RawManifestSHA256 != manifest.RawArtifactManifestSHA256 {
		return verifiedSnapshotMaterialV2{}, errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot v2 funds producer material is invalid"), err)
	}
	if err := service.verifyEvidenceGraphV2(ctx, manifest, producer); err != nil {
		return verifiedSnapshotMaterialV2{}, err
	}
	return verifiedSnapshotMaterialV2{manifest: manifest, producer: producer}, nil
}

type verifiedRawArtifactV2 struct {
	entry   domainevidence.RawArtifactEntryV1
	locator domainevidence.RawArtifactSourceLocatorV1
	root    domainevidence.RawArtifactContentRootV1
}

type verifiedParsedPageV2 struct {
	index domainevidence.ParsedPageIndexV1
	page  domainevidence.ParsedPageV1
}

func (service *SealedServiceV2) verifyEvidenceGraphV2(
	ctx context.Context,
	manifest domainsecurity.DatasetSnapshotManifestV2,
	producer domainsecurity.FundsProducerContentManifestV1,
) error {
	rawManifestBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawManifestV1,
		exactReferenceV2(manifest.RawArtifactManifestSHA256, manifest.RawArtifactManifestByteLength))
	if err != nil {
		return err
	}
	rawManifest, err := domainevidence.ParseRawArtifactManifestV1(rawManifestBody)
	if err != nil || rawManifest.ManifestDigest != manifest.RawArtifactManifestDigest {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw manifest is invalid"), err)
	}
	intentBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawAcquisitionIntentV1,
		exactReferenceV2(rawManifest.AcquisitionIntentSHA256, rawManifest.AcquisitionIntentByteLength))
	if err != nil {
		return err
	}
	intent, err := domainevidence.ParseRawArtifactAcquisitionIntentV1(intentBody)
	if err != nil || intent.IntentDigest != rawManifest.AcquisitionIntentDigest {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot acquisition intent is invalid"), err)
	}
	rawPages := make([]domainevidence.RawArtifactManifestPageV1, 0, len(rawManifest.PageDescriptors))
	rawByEntry := make(map[string]verifiedRawArtifactV2, rawManifest.ArtifactCount)
	for _, descriptor := range rawManifest.PageDescriptors {
		pageBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawManifestPageV1,
			exactReferenceV2(descriptor.PageSHA256, descriptor.PageByteLength))
		if readErr != nil {
			return readErr
		}
		page, parseErr := domainevidence.ParseRawArtifactManifestPageV1(pageBody)
		if parseErr != nil || domainevidence.ValidateRawArtifactManifestPageAgainstDescriptorV1(descriptor, page) != nil {
			return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw manifest page is invalid"), parseErr)
		}
		rawPages = append(rawPages, page)
		for _, embedded := range page.Entries {
			entryCanonical, canonicalErr := domainevidence.RawArtifactEntryV1Bytes(embedded)
			if canonicalErr != nil {
				return errors.Join(datasetsnapshotport.ErrCorrupt, canonicalErr)
			}
			entryBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawEntryV1,
				exactReferenceV2(domainsecurity.SHA256Hex(entryCanonical), uint64(len(entryCanonical))))
			if readErr != nil {
				return readErr
			}
			entry, parseErr := domainevidence.ParseRawArtifactEntryV1(entryBody)
			if parseErr != nil || entry != embedded {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw entry is invalid"), parseErr)
			}
			locatorBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawSourceLocatorV1,
				exactReferenceV2(entry.SourceLocatorSHA256, entry.SourceLocatorByteLength))
			if readErr != nil {
				return readErr
			}
			locator, parseErr := domainevidence.ParseRawArtifactSourceLocatorV1(locatorBody)
			if parseErr != nil || domainevidence.ValidateRawArtifactEntryAgainstSourceLocatorV1(intent, entry, locator) != nil {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw source locator is invalid"), parseErr)
			}
			rootBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawContentRootV1,
				exactReferenceV2(entry.ContentRootSHA256, entry.ContentRootByteLength))
			if readErr != nil {
				return readErr
			}
			root, parseErr := domainevidence.ParseRawArtifactContentRootV1(rootBody)
			if parseErr != nil || domainevidence.ValidateRawArtifactEntryWithContentRootV1(entry, root) != nil {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw content root is invalid"), parseErr)
			}
			chunkReaders := make([]io.Reader, 0, root.ChunkCount)
			for _, indexDescriptor := range root.IndexPageDescriptors {
				indexBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawContentIndexPageV1,
					exactReferenceV2(indexDescriptor.IndexPageSHA256, indexDescriptor.IndexPageByteLength))
				if readErr != nil {
					return readErr
				}
				contentIndex, parseErr := domainevidence.ParseRawArtifactContentIndexPageV1(indexBody)
				if parseErr != nil || domainevidence.ValidateRawArtifactContentIndexPageMembershipV1(root, contentIndex) != nil {
					return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw content index is invalid"), parseErr)
				}
				for _, chunkDescriptor := range contentIndex.ChunkDescriptors {
					chunk, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialRawContentChunkV1,
						exactReferenceV2(chunkDescriptor.ChunkSHA256, chunkDescriptor.ChunkByteLength))
					if readErr != nil {
						return readErr
					}
					if domainevidence.ValidateRawArtifactContentChunkBytesV1(chunkDescriptor, chunk) != nil {
						return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw chunk is invalid"))
					}
					chunkReaders = append(chunkReaders, bytes.NewReader(chunk))
				}
			}
			if domainevidence.ValidateRawArtifactContentFullSHA256V1(root, io.MultiReader(chunkReaders...)) != nil {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw full content is invalid"))
			}
			if _, exists := rawByEntry[entry.EntryDigest]; exists {
				return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot raw entry is duplicated"))
			}
			rawByEntry[entry.EntryDigest] = verifiedRawArtifactV2{entry: entry, locator: locator, root: root}
		}
	}
	if domainevidence.ValidateDatasetSnapshotManifestV2RawArtifactHierarchyV1(manifest, intent, rawManifest, rawPages) != nil ||
		producer.RawManifestSHA256 != domainsecurity.SHA256Hex(rawManifestBody) {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot raw hierarchy is incomplete"))
	}

	receiptBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialParsedReceiptV1,
		exactReferenceV2(manifest.ParsedGenerationReceiptSHA256, manifest.ParsedGenerationReceiptByteLength))
	if err != nil {
		return err
	}
	classificationBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialClassificationLedgerV1,
		exactReferenceV2(manifest.ClassificationLedgerSHA256, manifest.ClassificationLedgerByteLength))
	if err != nil {
		return err
	}
	if !bytes.Equal(receiptBody, classificationBody) {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot classification ledger is not the exact receipt alias"))
	}
	receipt, err := domainevidence.ParseParsedGenerationReceiptV1(receiptBody)
	if err != nil || receipt.ReceiptDigest != manifest.ParsedGenerationReceiptDigest ||
		domainevidence.ValidateDatasetSnapshotManifestV2ParsedGenerationV1(manifest, receipt) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot parsed receipt is invalid"), err)
	}
	configuration, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialParsedConfigurationV1,
		exactReferenceV2(receipt.Identity.ConfigurationSHA256, receipt.Identity.ConfigurationByteLength))
	if err != nil || len(configuration) == 0 {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot parser configuration is unavailable"), err)
	}
	parsedIndexes := make([]domainevidence.ParsedPageIndexV1, 0, len(receipt.IndexPageDescriptors))
	parsedPages := make([]domainevidence.ParsedPageV1, 0, receipt.PageCount)
	parsedByDigest := make(map[string]verifiedParsedPageV2, receipt.PageCount)
	accepted := make(map[string]domainevidence.ParsedOutcomeV1, receipt.AcceptedCount)
	for _, indexDescriptor := range receipt.IndexPageDescriptors {
		indexBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialParsedIndexPageV1,
			exactReferenceV2(indexDescriptor.IndexPageSHA256, indexDescriptor.IndexPageByteLength))
		if readErr != nil {
			return readErr
		}
		index, parseErr := domainevidence.ParseParsedPageIndexV1(indexBody)
		if parseErr != nil || domainevidence.ValidateParsedPageIndexAgainstDescriptorV1(indexDescriptor, index) != nil {
			return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot parsed index is invalid"), parseErr)
		}
		parsedIndexes = append(parsedIndexes, index)
		for _, pageDescriptor := range index.PageDescriptors {
			pageBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialParsedPageV1,
				exactReferenceV2(pageDescriptor.PageSHA256, pageDescriptor.PageByteLength))
			if readErr != nil {
				return readErr
			}
			page, parseErr := domainevidence.ParseParsedPageV1(pageBody)
			if parseErr != nil || domainevidence.ValidateParsedPageAgainstDescriptorV1(pageDescriptor, page) != nil {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot parsed page is invalid"), parseErr)
			}
			parsedPages = append(parsedPages, page)
			parsedByDigest[page.PageDigest] = verifiedParsedPageV2{index: index, page: page}
			for _, outcome := range page.Outcomes {
				if outcome.Disposition == domainevidence.ParsedOutcomeAcceptedV1 {
					if _, exists := accepted[outcome.SourceRecordID]; exists {
						return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot accepted source record is duplicated"))
					}
					accepted[outcome.SourceRecordID] = outcome
				}
			}
		}
	}
	if domainevidence.ValidateParsedGenerationHierarchyV1(receipt, parsedIndexes, parsedPages) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot parsed hierarchy is incomplete"))
	}

	rootBody, err := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialSourceRowLedgerRootV1,
		exactReferenceV2(manifest.SourceRowLedgerRootSHA256, manifest.SourceRowLedgerRootByteLength))
	if err != nil {
		return err
	}
	root, err := domainevidence.ParseSourceRowLedgerRootV1(rootBody)
	if err != nil || domainevidence.ValidateDatasetSnapshotManifestV2RowRootStructureV1(manifest, root) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row root is invalid"), err)
	}
	rowIndexes := make([]domainevidence.SourceRowLedgerIndexPageV1, 0, len(root.IndexPageDescriptors))
	rowPages := make([]domainevidence.SourceRowLedgerPageV1, 0, root.PageCount)
	for _, indexDescriptor := range root.IndexPageDescriptors {
		indexBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialSourceRowIndexPageV1,
			exactReferenceV2(indexDescriptor.IndexPageSHA256, indexDescriptor.IndexPageByteLength))
		if readErr != nil {
			return readErr
		}
		index, parseErr := domainevidence.ParseSourceRowLedgerIndexPageV1(indexBody)
		if parseErr != nil {
			return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row index is invalid"), parseErr)
		}
		rowIndexes = append(rowIndexes, index)
		for _, pageDescriptor := range index.PageDescriptors {
			pageBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialSourceRowPageV1,
				exactReferenceV2(pageDescriptor.PageSHA256, pageDescriptor.PageByteLength))
			if readErr != nil {
				return readErr
			}
			page, parseErr := domainevidence.ParseSourceRowLedgerPageV1(pageBody)
			if parseErr != nil {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row page is invalid"), parseErr)
			}
			rowPages = append(rowPages, page)
		}
	}
	if root.ValidateExactHierarchyV1(rowIndexes, rowPages) != nil {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row hierarchy is incomplete"))
	}
	policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(root.PolicyID)
	if !ok {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row policy is unavailable"))
	}
	seenRows := make(map[string]bool, root.RecordCount)
	for _, page := range rowPages {
		for _, rowEntry := range page.Entries {
			record := rowEntry.Record
			if seenRows[record.SourceRecordID] {
				return errors.Join(datasetsnapshotport.ErrCorrupt, errors.New("dataset snapshot source row is duplicated"))
			}
			seenRows[record.SourceRecordID] = true
			lineageBody, readErr := service.readExactTwiceV2(ctx, datasetsnapshotport.MaterialSourceRowLineageV1,
				exactReferenceV2(record.LineageSHA256, record.LineageByteLength))
			if readErr != nil {
				return readErr
			}
			lineage, parseErr := domainevidence.ParseSourceRowLineageV1(lineageBody)
			parsedMaterial, hasParsed := parsedByDigest[lineage.ParsedPageDigest]
			rawMaterial, hasRaw := rawByEntry[lineage.RawArtifactEntryDigest]
			_, hasAccepted := accepted[record.SourceRecordID]
			if parseErr != nil || !hasParsed || !hasRaw || !hasAccepted ||
				domainevidence.ValidateSourceRowRecordAgainstLineageV1(policy, root.Binding, record, lineage) != nil ||
				domainevidence.ValidateSourceRowLineageAgainstParsedGenerationV1(
					lineage, receipt, parsedMaterial.index, parsedMaterial.page,
				) != nil ||
				domainevidence.ValidateSourceRowLineageAgainstRawArtifactV1(
					lineage, intent, rawManifest, rawPages, rawMaterial.entry, rawMaterial.locator, rawMaterial.root,
				) != nil {
				return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row lineage is invalid"), parseErr)
			}
			delete(accepted, record.SourceRecordID)
		}
	}
	if len(accepted) != 0 || uint64(len(seenRows)) != root.RecordCount ||
		producer.AcceptedRowCount != root.RecordCount || producer.NormalizedRowCount != receipt.OutcomeCount {
		return errors.Join(datasetsnapshotport.ErrMismatch, errors.New("dataset snapshot source row coverage is incomplete"))
	}
	return nil
}
