package evidenceregistry

import (
	"context"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const maxRegistryAuthorityIndexDepthV2 = 100_000

var (
	ErrAuthorityUnavailable    = errors.New("witnessed evidence registry authority is unavailable")
	ErrAuthorityNotInitialized = errors.New("witnessed evidence registry authority is not initialized")
	ErrAuthorityIntegrity      = errors.New("witnessed evidence registry authority integrity failure")
)

type Config struct {
	InstallationID      string
	EnrollmentID        string
	Authority           finalauthorityport.Authority
	WitnessKeyID        string
	WitnessKey          []byte
	Coordinator         evidenceauthorityport.RegistryCoordinator
	WitnessChain        evidenceauthorityport.WitnessBindingChainResolver
	Indexes             registryport.AuthorityIndexStore
	Capsules            registryport.AuthorityCapsuleStore
	DatasetAuthority    datasetsnapshotport.CurrentAuthorityV2
	BindingObserver     casecontextport.Observer
	AuthorityKnownEmpty bool
	Random              io.Reader
	Now                 func() time.Time
}

// Service implements current registry membership exclusively from a fresh
// shared-witness head. Immutable candidates and local projections are never
// scanned or promoted to current authority.
type Service struct {
	installationID      string
	enrollmentID        string
	authority           finalauthorityport.Authority
	keyID               string
	publicKey           []byte
	witnessKeyID        string
	witnessKey          []byte
	coordinator         evidenceauthorityport.RegistryCoordinator
	witnessChain        evidenceauthorityport.WitnessBindingChainResolver
	indexes             registryport.AuthorityIndexStore
	capsules            registryport.AuthorityCapsuleStore
	datasetAuthority    datasetsnapshotport.CurrentAuthorityV2
	bindingObserver     casecontextport.Observer
	random              io.Reader
	now                 func() time.Time
	authorityKnownEmpty bool

	mu              sync.Mutex
	mutationCounter uint64
}

var _ registryport.Registry = (*Service)(nil)
var _ registryport.LockedSnapshot = (*Service)(nil)
var _ registryport.HistoricalReplay = (*Service)(nil)
var _ registryport.Inventory = (*Service)(nil)
var _ registryport.WitnessedSnapshotReader = (*Service)(nil)
var _ registryport.WitnessedSnapshotAuthority = (*Service)(nil)
var _ registryport.FactFinalWitnessVerifier = (*Service)(nil)
var _ registryport.FactFinalWitnessIssuer = (*Service)(nil)

func New(config Config) (*Service, error) {
	if config.Random == nil {
		config.Random = cryptorand.Reader
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	publicKey := []byte(nil)
	keyID := ""
	if config.Authority != nil {
		publicKey = append([]byte(nil), config.Authority.PublicKey()...)
		keyID = strings.TrimSpace(config.Authority.KeyID())
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(config.InstallationID)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.EnrollmentID)) || config.Authority == nil ||
		len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		len(config.WitnessKey) != ed25519.PublicKeySize || strings.TrimSpace(config.WitnessKeyID) != domainsecurity.SHA256Hex(config.WitnessKey) ||
		config.Coordinator == nil || config.Indexes == nil || config.Capsules == nil || config.Random == nil || config.Now == nil {
		return nil, ErrAuthorityUnavailable
	}
	return &Service{
		installationID: strings.TrimSpace(config.InstallationID), enrollmentID: strings.TrimSpace(config.EnrollmentID),
		authority: config.Authority, keyID: keyID, publicKey: publicKey, coordinator: config.Coordinator,
		witnessKeyID: strings.TrimSpace(config.WitnessKeyID), witnessKey: append([]byte(nil), config.WitnessKey...),
		witnessChain: config.WitnessChain, indexes: config.Indexes, capsules: config.Capsules,
		datasetAuthority: config.DatasetAuthority, bindingObserver: config.BindingObserver,
		authorityKnownEmpty: config.AuthorityKnownEmpty,
		random:              config.Random, now: config.Now,
	}, nil
}

func (service *Service) CommitPrepared(ctx context.Context, input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		!domainsecurity.IsHostToolCallIDV1(input.Draft.ToolCallID) {
		return domainevidence.EvidenceReceipt{}, errors.New("evidence registration requires current V2 case fact authority")
	}
	if err := ctx.Err(); err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	effect, releaseEffect, err := service.beginPreparedCommitEffectV1(ctx, input)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	defer releaseEffect()
	head, registry, err := service.currentRegistryLocked(ctx, input.Context)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	if effect != nil && head.Bundle != effect.before.Bundle {
		return domainevidence.EvidenceReceipt{}, errors.New("prepared registry effect pre-head changed")
	}
	if receipt, matched, err := domainevidence.MatchEvidenceReceiptRegistration(registry, input.Draft, input.CanonicalEvidence, input.SettlementProof); err != nil {
		return domainevidence.EvidenceReceipt{}, err
	} else if matched {
		return receipt, nil
	}
	next, receipt, err := domainevidence.RegisterEvidenceReceipt(registry, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	committed, readback, err := service.commitRegistryLocked(ctx, head, input.Context, next)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	service.authorityKnownEmpty = false
	registered, err := domainevidence.VerifyEvidenceReceiptMembership(committed, input.Context, receipt.ReceiptID)
	if err != nil || registered.Revoked || !reflect.DeepEqual(registered.Receipt, receipt) {
		return domainevidence.EvidenceReceipt{}, ErrAuthorityIntegrity
	}
	if effect != nil {
		if err := effect.finish(readback, receipt); err != nil {
			return domainevidence.EvidenceReceipt{}, err
		}
	}
	return receipt, nil
}

func (service *Service) Resolve(ctx context.Context, query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(query.Context) != nil {
		return domainevidence.RegisteredEvidence{}, errors.New("evidence membership requires current V2 case fact authority")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, registry, err := service.currentRegistryLocked(ctx, query.Context)
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	return domainevidence.VerifyEvidenceReceiptMembership(registry, query.Context, query.ReceiptID)
}

func (service *Service) Revoke(ctx context.Context, input registryport.RevokeInput) error {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
		return errors.New("evidence revocation requires current V2 case fact authority")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	head, registry, err := service.currentRegistryLocked(ctx, input.Context)
	if err != nil {
		return err
	}
	if matched, err := domainevidence.MatchEvidenceReceiptRevocation(registry, input.ReceiptID, input.ReasonCode); err != nil {
		return err
	} else if matched {
		return nil
	}
	next, err := domainevidence.RevokeEvidenceReceipt(registry, input.ReceiptID, input.ReasonCode, input.RevokedAt)
	if err != nil {
		return err
	}
	committed, _, err := service.commitRegistryLocked(ctx, head, input.Context, next)
	if err != nil {
		return err
	}
	service.authorityKnownEmpty = false
	if _, err := domainevidence.VerifyEvidenceReceiptMembership(committed, input.Context, input.ReceiptID); err == nil {
		return ErrAuthorityIntegrity
	}
	return nil
}

func (service *Service) Replay(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence replay requires current V2 case publication authority")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, registry, err := service.currentRegistryLocked(ctx, securityContext)
	return registry, err
}

func (service *Service) ReplayAt(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	if service == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("historical evidence replay requires V2 case publication authority")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, current, err := service.currentRegistryLocked(ctx, securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	if sequence > current.Sequence {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry historical sequence is unavailable")
	}
	prefix, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	for _, entry := range current.Entries[:sequence] {
		prefix, err = domainevidence.ApplyEvidenceRegistryEntry(prefix, entry)
		if err != nil {
			return domainevidence.EvidenceReceiptRegistry{}, err
		}
	}
	return prefix, nil
}

func (service *Service) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, callback func(domainevidence.EvidenceReceiptRegistry) error) error {
	if service == nil || ctx == nil || callback == nil || domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return errors.New("witnessed evidence registry locked snapshot input is invalid")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	_, registry, err := service.currentRegistryLocked(ctx, securityContext)
	if err != nil {
		return err
	}
	copy, err := domainevidence.ParseEvidenceReceiptRegistry(registry)
	if err != nil {
		return err
	}
	return callback(copy)
}

func (service *Service) WithWitnessedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, callback func(registryport.WitnessedSnapshot) error) error {
	if service == nil || ctx == nil || callback == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return errors.New("witnessed evidence registry snapshot input is invalid")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	head, err := service.coordinator.ObserveFresh(ctx)
	if err != nil {
		return err
	}
	if head.Bundle.EvidenceRegistryCount == 0 {
		return errors.New("witnessed evidence registry snapshot is empty")
	}
	selection, err := service.registrySelectionFromHeadLocked(ctx, head, securityContext)
	if err != nil {
		return err
	}
	registryCopy, err := domainevidence.ParseEvidenceReceiptRegistry(selection.registry)
	if err != nil {
		return err
	}
	return callback(registryport.WitnessedSnapshot{
		Head: head, RootIndex: selection.rootIndex, HasSelection: selection.hasSelection,
		SelectedIndex: selection.selectedIndex, SelectedCapsule: selection.selectedCapsule,
		RegistryIndexPath: append([]domainevidence.EvidenceRegistryAuthorityIndexV2(nil), selection.indexPath...),
		Context:           securityContext, Registry: registryCopy,
	})
}

type witnessedSnapshotCapabilityV2 struct {
	mu             sync.RWMutex
	active         bool
	ctx            context.Context
	snapshotDigest string
}

func (service *Service) WithWitnessedSnapshotAuthority(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(registryport.WitnessedSnapshot, registryport.WitnessedSnapshotCapability) error,
) error {
	if service == nil || ctx == nil || callback == nil {
		return errors.New("witnessed evidence registry capability input is invalid")
	}
	return service.WithWitnessedSnapshot(ctx, securityContext, func(snapshot registryport.WitnessedSnapshot) error {
		digest, err := witnessedSnapshotDigestV2(snapshot)
		if err != nil {
			return err
		}
		capability := &witnessedSnapshotCapabilityV2{active: true, ctx: ctx, snapshotDigest: digest}
		defer capability.close()
		return callback(snapshot, capability)
	})
}

func (capability *witnessedSnapshotCapabilityV2) UseExact(
	snapshot registryport.WitnessedSnapshot,
	mutation func() error,
) error {
	if capability == nil {
		return errors.New("witnessed evidence registry capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	digest, err := witnessedSnapshotDigestV2(snapshot)
	if err != nil || !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || mutation == nil ||
		digest != capability.snapshotDigest {
		return errors.New("witnessed evidence registry capability does not authorize this snapshot")
	}
	if err := mutation(); err != nil {
		return err
	}
	afterDigest, err := witnessedSnapshotDigestV2(snapshot)
	if err != nil || afterDigest != capability.snapshotDigest || capability.ctx.Err() != nil {
		return errors.New("witnessed evidence registry capability changed or was cancelled")
	}
	return nil
}

func witnessedSnapshotDigestV2(snapshot registryport.WitnessedSnapshot) (string, error) {
	body, err := json.Marshal(snapshot)
	if err != nil {
		return "", errors.New("witnessed evidence registry snapshot cannot be frozen")
	}
	return domainsecurity.SHA256Hex(body), nil
}

func (capability *witnessedSnapshotCapabilityV2) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

type factFinalWitnessCapabilityV1 struct {
	mu           sync.RWMutex
	active       bool
	privateFinal domainevidence.PrivateAcceptedFinalRecord
	input        domainevidence.FactFinalWitnessAdmissionInputV1
}

func (service *Service) WithFactFinalWitnessAuthority(
	ctx context.Context,
	request registryport.FactFinalWitnessRequest,
	callback func(registryport.FactFinalWitnessCapability) error,
) error {
	if service == nil || ctx == nil || callback == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(request.Context) != nil ||
		domainevidence.ValidateFinalAnswerEnvelope(request.Envelope) != nil ||
		domainevidence.ValidateTerminalPublicationIntent(request.PublicationIntent, request.Envelope.TerminalReason) != nil ||
		!domainevidence.FinalAnswerRequiresPublicationSnapshotProof(request.Envelope) ||
		request.PublicationProof == nil || strings.TrimSpace(request.RenderedText) == "" ||
		validateFactFinalHostEvidenceRequestV2(request) != nil {
		return errors.New("fact final witness issuance input is invalid")
	}
	probe := request.SourceProbes[0]
	var issued *factFinalWitnessCapabilityV1
	err := request.HostEvidenceCapability.UseExact(
		request.Context,
		probe,
		request.DatasetSelection,
		func(leaseContext context.Context) error {
			return service.WithWitnessedSnapshot(leaseContext, request.Context, func(snapshot registryport.WitnessedSnapshot) error {
				selection := request.DatasetSelection
				if snapshot.Head.Bundle.DatasetSnapshotIndexDigest != selection.Head.Bundle.DatasetSnapshotIndexDigest ||
					snapshot.Head.Bundle.DatasetSnapshotCount != selection.Head.Bundle.DatasetSnapshotCount {
					return errors.New("fact final dataset authority changed before admission")
				}
				input := domainevidence.FactFinalWitnessAdmissionInputV1{
					Context: request.Context, Envelope: request.Envelope, RenderedText: request.RenderedText,
					PublicationProof: request.PublicationProof, Registry: snapshot.Registry,
					Bundle: snapshot.Head.Bundle, ObserveRequest: snapshot.Head.Request, Observation: snapshot.Head.Observation,
					RootIndex: snapshot.RootIndex, SelectedIndex: snapshot.SelectedIndex, SelectedCapsule: snapshot.SelectedCapsule,
					RegistryIndexPath: append([]domainevidence.EvidenceRegistryAuthorityIndexV2(nil), snapshot.RegistryIndexPath...),
					DatasetRootIndex:  selection.DatasetIndexPath[0], SelectedDatasetIndex: selection.SelectedIndex,
					DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), selection.DatasetIndexPath...),
					DatasetRecord:    selection.Snapshot.Record, DatasetManifest: selection.Snapshot.Manifest,
					FundsProducerContent: selection.Snapshot.FundsProducerContent,
					BindingObservation:   request.BindingObservation,
					InstallationID:       service.installationID, EnrollmentID: service.enrollmentID,
					AuthorityKeyID: service.keyID, AuthorityPublicKey: append([]byte(nil), service.publicKey...),
					WitnessKeyID: service.witnessKeyID, WitnessPublicKey: append([]byte(nil), service.witnessKey...),
					AdmittedAt: service.now().UTC(),
				}
				admission, err := domainevidence.NewFactFinalWitnessAdmissionV1(input)
				if err != nil {
					return err
				}
				frozenInput, err := cloneFactFinalWitnessInputV1(input)
				if err != nil {
					return err
				}
				registryHead, err := domainevidence.NewEvidenceRegistryHead(snapshot.Registry)
				if err != nil {
					return err
				}
				privateDigest, err := domainevidence.PrivateAcceptedFinalDigestWithFactWitnessV1(
					request.Context,
					request.Envelope,
					request.RenderedText,
					request.PublicationIntent,
					request.PublicationProof,
					&admission,
				)
				if err != nil {
					return err
				}
				acceptedAt, err := time.Parse(time.RFC3339Nano, admission.AdmittedAt)
				if err != nil {
					return err
				}
				acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
					Context: request.Context, Envelope: request.Envelope, RenderedText: request.RenderedText,
					RegistryHead: registryHead, PublicationSnapshotProof: request.PublicationProof,
					FactFinalWitnessAdmission: &admission, FactFinalWitnessAuthority: &frozenInput,
					PrivateRecordDigest: privateDigest, AcceptedAt: acceptedAt,
					AuthorityKeyID: service.keyID, AuthorityPublicKey: append([]byte(nil), service.publicKey...),
				}, func(message []byte) ([]byte, error) {
					return service.authority.Sign(leaseContext, message)
				})
				if err != nil {
					return err
				}
				privateFinal, err := domainevidence.NewPrivateAcceptedFinalRecord(
					request.Context,
					request.Envelope,
					request.RenderedText,
					registryHead,
					request.PublicationIntent,
					acceptedFinal,
					request.PublicationProof,
				)
				if err != nil {
					return err
				}
				issued = &factFinalWitnessCapabilityV1{active: true, privateFinal: privateFinal, input: frozenInput}
				return nil
			})
		},
	)
	if err != nil || issued == nil {
		return errors.Join(errors.New("fact final witness issuance did not remain current"), err)
	}
	defer issued.close()
	return callback(issued)
}

func validateFactFinalHostEvidenceRequestV2(request registryport.FactFinalWitnessRequest) error {
	if request.HostEvidenceCapability == nil || len(request.SourceProbes) != 1 ||
		domainsecurity.ValidateCaseBindingObservationV1(request.BindingObservation) != nil ||
		request.BindingObservation.State != domainsecurity.CaseBindingStateValid ||
		request.BindingObservation.WorkspaceRealPath != request.Context.WorkspaceRealPath ||
		request.BindingObservation.CaseID != request.Context.CaseID ||
		request.BindingObservation.CaseBindingHash != request.Context.CaseBindingHash ||
		request.BindingObservation.ObservationDigest != request.Context.PublicationPolicy.BindingObservationDigest ||
		!domainsecurity.SourceProbeEligibleForHostAuthorityV2(request.SourceProbes[0]) ||
		request.SourceProbes[0].ThreadID != request.Context.ThreadID ||
		request.SourceProbes[0].TurnID != request.Context.TurnID ||
		request.SourceProbes[0].ContextEpoch != request.Context.ContextEpoch ||
		request.SourceProbes[0].ProbeContextDigest != request.Context.ContextDigest ||
		request.SourceProbes[0].CaseID != request.Context.CaseID ||
		request.SourceProbes[0].CaseBindingHash != request.Context.CaseBindingHash ||
		request.SourceProbes[0].DatasetSnapshotID != request.Context.DatasetSnapshotID ||
		len(request.DatasetSelection.DatasetIndexPath) == 0 ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(request.DatasetSelection) != nil ||
		!factFinalPublicationProofMatchesHostProbeV2(request.PublicationProof, request.SourceProbes[0]) {
		return errors.New("fact final host evidence authority is invalid")
	}
	return nil
}

func factFinalPublicationProofMatchesHostProbeV2(
	proof *domainevidence.PublicationSnapshotProof,
	probe domainsecurity.VerifiedSourceProbe,
) bool {
	if proof == nil || domainevidence.ValidatePublicationSnapshotProof(*proof) != nil || len(proof.Sources) == 0 {
		return false
	}
	identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(probe.ServerIdentity)
	if err != nil || identity.ServerID != probe.ServerID || identity.ConnectionEpoch != probe.ConnectionEpoch {
		return false
	}
	for _, source := range proof.Sources {
		if source.ServerID != probe.ServerID || source.ServerIdentity != probe.ServerIdentity ||
			source.ServerVersion != identity.ObservedVersion || source.ConnectionEpoch != probe.ConnectionEpoch ||
			source.DatasetSnapshotID != probe.DatasetSnapshotID || source.CatalogFingerprint != probe.CatalogFingerprint ||
			source.SpecFingerprint != probe.SpecFingerprint || source.ProbeDigest != probe.ProbeDigest ||
			source.CheckedAt != probe.CheckedAt {
			return false
		}
	}
	return true
}

func (capability *factFinalWitnessCapabilityV1) PrivateFinal() (domainevidence.PrivateAcceptedFinalRecord, error) {
	if capability == nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("fact final witness capability is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("fact final witness capability is inactive")
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(capability.privateFinal)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	return domainevidence.ParsePrivateAcceptedFinalRecord(body)
}

func (capability *factFinalWitnessCapabilityV1) UseExact(
	record domainevidence.PrivateAcceptedFinalRecord,
	mutation func() error,
) error {
	if capability == nil {
		return errors.New("fact final witness capability does not authorize this private final")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || mutation == nil || domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil ||
		!reflect.DeepEqual(record, capability.privateFinal) || record.StoreDigest != capability.privateFinal.StoreDigest ||
		record.AcceptedFinal.FactFinalWitnessAdmission == nil {
		return errors.New("fact final witness capability does not authorize this private final")
	}
	if domainevidence.ValidateFactFinalWitnessAdmissionExactV1(
		*record.AcceptedFinal.FactFinalWitnessAdmission,
		capability.input,
	) != nil {
		return errors.New("fact final witness capability authority is invalid")
	}
	return mutation()
}

func (capability *factFinalWitnessCapabilityV1) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

func cloneFactFinalWitnessInputV1(input domainevidence.FactFinalWitnessAdmissionInputV1) (domainevidence.FactFinalWitnessAdmissionInputV1, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return domainevidence.FactFinalWitnessAdmissionInputV1{}, err
	}
	var clone domainevidence.FactFinalWitnessAdmissionInputV1
	if err := json.Unmarshal(body, &clone); err != nil {
		return domainevidence.FactFinalWitnessAdmissionInputV1{}, err
	}
	return clone, nil
}

// VerifyFactFinalWitnessCurrent proves that the compact admission embedded in
// one V5 fact final names an exact historical witness exchange that remains
// on a newly challenged shared authority chain. The immutable registry index
// and capsule are then replayed at that historical bundle; the current local
// registry projection is never used to manufacture freshness.
func (service *Service) VerifyFactFinalWitnessCurrent(
	ctx context.Context,
	record domainevidence.PrivateAcceptedFinalRecord,
) error {
	return service.withVerifiedFactFinalWitness(ctx, record, func() error { return nil })
}

func (service *Service) withVerifiedFactFinalWitness(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, use func() error) error {
	if service == nil || ctx == nil || service.witnessChain == nil ||
		domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil ||
		!domainevidence.FinalAnswerRequiresPublicationSnapshotProof(record.Envelope) ||
		record.AcceptedFinal.FactFinalWitnessAdmission == nil {
		return errors.New("fact final witness replay input is invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	chain, err := service.witnessChain.ResolveWitnessBindingOnFreshChain(
		ctx, record.AcceptedFinal.FactFinalWitnessAdmission.WitnessBinding,
	)
	if err != nil {
		return errors.Join(errors.New("fact final witness chain is unavailable"), err)
	}
	historical := chain.Historical
	historicalHead := evidenceauthorityport.FreshHead{
		HasBundle: true, Bundle: historical.Bundle, Request: historical.Request, Observation: historical.Observation,
	}
	historicalSelection, err := service.registrySelectionFromHeadLocked(ctx, historicalHead, record.SecurityContext)
	if err != nil || !historicalSelection.hasSelection {
		return errors.Join(errors.New("fact final historical registry selection is unavailable"), err)
	}
	currentSelection, err := service.registrySelectionFromHeadLocked(ctx, chain.Current, record.SecurityContext)
	currentHead, headErr := domainevidence.NewEvidenceRegistryHead(currentSelection.registry)
	if err != nil || headErr != nil || !currentSelection.hasSelection ||
		!reflect.DeepEqual(currentSelection.selectedIndex, historicalSelection.selectedIndex) ||
		!reflect.DeepEqual(currentSelection.selectedCapsule, historicalSelection.selectedCapsule) ||
		!reflect.DeepEqual(currentHead, record.RegistryHead) ||
		domainevidence.ValidatePublicationSnapshotProofAgainstRegistry(
			record.PublicationSnapshotProof, record.SecurityContext, record.Envelope, currentSelection.registry,
		) != nil {
		return errors.Join(errors.New("fact final current registry authority changed after admission"), err, headErr)
	}
	admittedAt, err := time.Parse(time.RFC3339Nano, record.AcceptedFinal.FactFinalWitnessAdmission.AdmittedAt)
	if err != nil {
		return errors.New("fact final witness admission time is invalid")
	}
	input := domainevidence.FactFinalWitnessAdmissionInputV1{
		Context: record.SecurityContext, Envelope: record.Envelope, RenderedText: record.RenderedText,
		PublicationProof: record.PublicationSnapshotProof, Registry: historicalSelection.registry,
		Bundle: historical.Bundle, ObserveRequest: historical.Request, Observation: historical.Observation,
		RootIndex: historicalSelection.rootIndex, SelectedIndex: historicalSelection.selectedIndex, SelectedCapsule: historicalSelection.selectedCapsule,
		RegistryIndexPath: append([]domainevidence.EvidenceRegistryAuthorityIndexV2(nil), historicalSelection.indexPath...),
		InstallationID:    service.installationID, EnrollmentID: service.enrollmentID,
		AuthorityKeyID: service.keyID, AuthorityPublicKey: append([]byte(nil), service.publicKey...),
		WitnessKeyID: service.witnessKeyID, WitnessPublicKey: append([]byte(nil), service.witnessKey...),
		AdmittedAt: admittedAt,
	}
	if record.AcceptedFinal.FactFinalWitnessAdmission.SchemaVersion ==
		domainevidence.FactFinalWitnessAdmissionSchemaVersionV2 {
		return service.verifyFactFinalDatasetAuthorityCurrentV2(ctx, record, historical.Bundle, input, use)
	}
	if err := domainevidence.ValidateFactFinalWitnessAdmissionExactV1(
		*record.AcceptedFinal.FactFinalWitnessAdmission, input,
	); err != nil {
		return errors.Join(errors.New("fact final witness admission does not match fresh-chain authority"), err)
	}
	return use()
}

func (service *Service) verifyFactFinalDatasetAuthorityCurrentV2(
	ctx context.Context,
	record domainevidence.PrivateAcceptedFinalRecord,
	historicalBundle domainevidence.EvidenceAuthorityBundleV1,
	input domainevidence.FactFinalWitnessAdmissionInputV1,
	use func() error,
) error {
	if service.datasetAuthority == nil || service.bindingObserver == nil {
		return errors.New("fact final current dataset replay authority is unavailable")
	}
	binding, err := service.bindingObserver.Observe(record.SecurityContext.WorkspaceRealPath)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(binding) != nil ||
		binding.State != domainsecurity.CaseBindingStateValid ||
		binding.WorkspaceRealPath != record.SecurityContext.WorkspaceRealPath ||
		binding.CaseID != record.SecurityContext.CaseID ||
		binding.CaseBindingHash != record.SecurityContext.CaseBindingHash ||
		binding.ObservationDigest != record.SecurityContext.PublicationPolicy.BindingObservationDigest {
		return errors.New("fact final current dataset binding changed after admission")
	}
	resolveInput := datasetsnapshotport.ResolveInputV2{
		TenantID:                  record.SecurityContext.TenantID,
		UserID:                    record.SecurityContext.UserID,
		Observation:               binding,
		ExpectedDatasetSnapshotID: record.SecurityContext.DatasetSnapshotID,
	}
	return service.datasetAuthority.WithCurrentSelectionV2(
		ctx,
		resolveInput,
		record.SecurityContext,
		func(
			selection datasetsnapshotport.CurrentSelectionV2,
			capability datasetsnapshotport.CurrentSelectionCapabilityV2,
		) error {
			if capability == nil ||
				selection.Head.Bundle.DatasetSnapshotIndexDigest != historicalBundle.DatasetSnapshotIndexDigest ||
				selection.Head.Bundle.DatasetSnapshotCount != historicalBundle.DatasetSnapshotCount ||
				len(selection.DatasetIndexPath) == 0 ||
				datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil {
				return errors.New("fact final current dataset selection changed after admission")
			}
			return capability.UseExact(
				selection,
				record.SecurityContext,
				func(context.Context) error {
					input.DatasetRootIndex = selection.DatasetIndexPath[0]
					input.SelectedDatasetIndex = selection.SelectedIndex
					input.DatasetIndexPath = append([]domainsecurity.DatasetSnapshotIndexV1(nil), selection.DatasetIndexPath...)
					input.DatasetRecord = selection.Snapshot.Record
					input.DatasetManifest = selection.Snapshot.Manifest
					input.FundsProducerContent = selection.Snapshot.FundsProducerContent
					input.BindingObservation = binding
					if err := domainevidence.ValidateFactFinalWitnessAdmissionExactV1(
						*record.AcceptedFinal.FactFinalWitnessAdmission,
						input,
					); err != nil {
						return errors.Join(
							errors.New("fact final witness admission does not match current dataset authority"),
							err,
						)
					}
					return use()
				},
			)
		},
	)
}

func (service *Service) ListRegistries(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	if service == nil || ctx == nil {
		return nil, ErrAuthorityUnavailable
	}
	contextByIdentity := make(map[string]domainsecurity.TurnSecurityContext, len(contexts))
	for _, securityContext := range contexts {
		if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil {
			return nil, errors.New("evidence registry inventory context is invalid")
		}
		identity := securityContext.ThreadID + "\x00" + securityContext.TurnID
		if _, duplicate := contextByIdentity[identity]; duplicate {
			return nil, errors.New("evidence registry inventory contains a duplicate turn identity")
		}
		contextByIdentity[identity] = securityContext
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.authorityKnownEmpty {
		return []registryport.InventoryRecord{}, nil
	}
	head, err := service.coordinator.ObserveFresh(ctx)
	if err != nil {
		return nil, err
	}
	if err := service.validateHead(head); err != nil {
		return nil, err
	}
	root := head.Bundle.EvidenceRegistryIndexDigest
	count := head.Bundle.EvidenceRegistryCount
	if count == 0 {
		if root != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
			return nil, ErrAuthorityIntegrity
		}
		return []registryport.InventoryRecord{}, nil
	}
	if count > maxRegistryAuthorityIndexDepthV2 || root == domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
		return nil, ErrAuthorityIntegrity
	}
	latest := make(map[string]domainevidence.EvidenceReceiptRegistry, len(contexts))
	type inventorySelectionV2 struct {
		entry   domainevidence.EvidenceRegistryAuthorityIndexEntry
		capsule domainevidence.EvidenceRegistryAuthorityCapsule
	}
	lineages := make(map[string][]inventorySelectionV2, len(contexts))
	currentDigest := root
	var newer *domainevidence.EvidenceRegistryAuthorityIndexV2
	for generation := count; generation > 0; generation-- {
		index, err := service.indexes.Resolve(ctx, currentDigest)
		if err != nil || domainevidence.ValidateEvidenceRegistryAuthorityIndexForInstallationV2(
			index, service.installationID, service.enrollmentID, service.keyID, service.publicKey,
		) != nil || index.Generation != generation || index.IndexDigest != currentDigest {
			return nil, ErrAuthorityIntegrity
		}
		if generation == count && domainevidence.ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(index, root, count) != nil {
			return nil, ErrAuthorityIntegrity
		}
		if newer != nil && domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(index, *newer) != nil {
			return nil, ErrAuthorityIntegrity
		}
		identity := index.Entry.ThreadID + "\x00" + index.Entry.TurnID
		securityContext, found := contextByIdentity[identity]
		if !found || index.Entry.ContextDigest != securityContext.ContextDigest {
			return nil, errors.New("evidence registry is detached from durable turn authority")
		}
		if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
			return nil, errors.New("non-factual or audit-only turn has evidence registry authority")
		}
		capsule, err := service.capsules.Resolve(ctx, index.Entry.CapsuleRecordDigest)
		if err != nil || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(index, capsule) ||
			!reflect.DeepEqual(capsule.SecurityContext, securityContext) {
			return nil, ErrAuthorityIntegrity
		}
		if _, selected := latest[identity]; !selected {
			latest[identity] = capsule.Registry
		}
		lineages[identity] = append(lineages[identity], inventorySelectionV2{entry: index.Entry, capsule: capsule})
		if generation == 1 {
			if index.PreviousIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
				return nil, ErrAuthorityIntegrity
			}
			break
		}
		newer = &index
		currentDigest = index.PreviousIndexDigest
	}
	for _, lineage := range lineages {
		oldest := lineage[len(lineage)-1]
		if oldest.entry.RegistrySequence != 1 {
			return nil, ErrAuthorityIntegrity
		}
		previous := oldest
		for position := len(lineage) - 2; position >= 0; position-- {
			next := lineage[position]
			if domainevidence.ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(
				previous.entry, next.entry, next.capsule,
			) != nil {
				return nil, ErrAuthorityIntegrity
			}
			previous = next
		}
	}
	records := make([]registryport.InventoryRecord, 0, len(contexts))
	for _, securityContext := range contexts {
		identity := securityContext.ThreadID + "\x00" + securityContext.TurnID
		if registry, found := latest[identity]; found {
			records = append(records, registryport.InventoryRecord{Context: securityContext, Registry: registry})
		}
	}
	return records, nil
}

func (service *Service) HasRecords(ctx context.Context) (bool, error) {
	if service == nil || ctx == nil {
		return false, ErrAuthorityUnavailable
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.authorityKnownEmpty {
		return false, nil
	}
	head, err := service.coordinator.ObserveFresh(ctx)
	if err != nil {
		return false, err
	}
	if !head.HasBundle {
		return false, ErrAuthorityNotInitialized
	}
	if err := service.validateHead(head); err != nil {
		return false, err
	}
	return head.Bundle.EvidenceRegistryCount > 0, nil
}

func (service *Service) currentRegistryLocked(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (evidenceauthorityport.FreshHead, domainevidence.EvidenceReceiptRegistry, error) {
	head, err := service.coordinator.ObserveFresh(ctx)
	if err != nil {
		return evidenceauthorityport.FreshHead{}, domainevidence.EvidenceReceiptRegistry{}, err
	}
	registry, err := service.registryFromHeadLocked(ctx, head, securityContext)
	return head, registry, err
}

func (service *Service) registryFromHeadLocked(ctx context.Context, head evidenceauthorityport.FreshHead, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	selection, err := service.registrySelectionFromHeadLocked(ctx, head, securityContext)
	return selection.registry, err
}

type registrySelectionV2 struct {
	registry        domainevidence.EvidenceReceiptRegistry
	rootIndex       domainevidence.EvidenceRegistryAuthorityIndexV2
	hasSelection    bool
	selectedIndex   domainevidence.EvidenceRegistryAuthorityIndexV2
	selectedCapsule domainevidence.EvidenceRegistryAuthorityCapsule
	indexPath       []domainevidence.EvidenceRegistryAuthorityIndexV2
}

func (service *Service) registrySelectionFromHeadLocked(ctx context.Context, head evidenceauthorityport.FreshHead, securityContext domainsecurity.TurnSecurityContext) (registrySelectionV2, error) {
	if err := service.validateHead(head); err != nil {
		return registrySelectionV2{}, err
	}
	empty, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return registrySelectionV2{}, err
	}
	root := head.Bundle.EvidenceRegistryIndexDigest
	count := head.Bundle.EvidenceRegistryCount
	if count == 0 {
		if root != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
			return registrySelectionV2{}, ErrAuthorityIntegrity
		}
		return registrySelectionV2{registry: empty}, nil
	}
	if count > maxRegistryAuthorityIndexDepthV2 || root == domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
		return registrySelectionV2{}, ErrAuthorityIntegrity
	}
	currentDigest := root
	var newer *domainevidence.EvidenceRegistryAuthorityIndexV2
	var selected *domainevidence.EvidenceRegistryAuthorityCapsule
	var selectedIndex domainevidence.EvidenceRegistryAuthorityIndexV2
	var rootIndex domainevidence.EvidenceRegistryAuthorityIndexV2
	path := make([]domainevidence.EvidenceRegistryAuthorityIndexV2, 0, count)
	var selectedPath []domainevidence.EvidenceRegistryAuthorityIndexV2
	for generation := count; generation > 0; generation-- {
		index, err := service.indexes.Resolve(ctx, currentDigest)
		if err != nil || domainevidence.ValidateEvidenceRegistryAuthorityIndexForInstallationV2(
			index, service.installationID, service.enrollmentID, service.keyID, service.publicKey,
		) != nil || index.Generation != generation || index.IndexDigest != currentDigest {
			return registrySelectionV2{}, ErrAuthorityIntegrity
		}
		path = append(path, index)
		if generation == count {
			rootIndex = index
		}
		if generation == count && domainevidence.ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(index, root, count) != nil {
			return registrySelectionV2{}, ErrAuthorityIntegrity
		}
		if newer != nil && domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(index, *newer) != nil {
			return registrySelectionV2{}, ErrAuthorityIntegrity
		}
		if selected == nil && index.Entry.ThreadID == securityContext.ThreadID && index.Entry.TurnID == securityContext.TurnID &&
			index.Entry.ContextDigest == securityContext.ContextDigest {
			capsule, err := service.capsules.Resolve(ctx, index.Entry.CapsuleRecordDigest)
			if err != nil || !domainevidence.EvidenceRegistryAuthorityIndexEntryMatchesCapsuleV2(index, capsule) ||
				!reflect.DeepEqual(capsule.SecurityContext, securityContext) {
				return registrySelectionV2{}, ErrAuthorityIntegrity
			}
			copy := capsule
			selected = &copy
			selectedIndex = index
			selectedPath = append([]domainevidence.EvidenceRegistryAuthorityIndexV2(nil), path...)
		}
		if generation == 1 {
			if index.PreviousIndexDigest != domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2() {
				return registrySelectionV2{}, ErrAuthorityIntegrity
			}
			break
		}
		newer = &index
		currentDigest = index.PreviousIndexDigest
	}
	if selected == nil {
		return registrySelectionV2{registry: empty, rootIndex: rootIndex}, nil
	}
	return registrySelectionV2{
		registry: selected.Registry, rootIndex: rootIndex, hasSelection: true,
		selectedIndex: selectedIndex, selectedCapsule: *selected, indexPath: selectedPath,
	}, nil
}

func (service *Service) commitRegistryLocked(
	ctx context.Context,
	head evidenceauthorityport.FreshHead,
	securityContext domainsecurity.TurnSecurityContext,
	nextRegistry domainevidence.EvidenceReceiptRegistry,
) (domainevidence.EvidenceReceiptRegistry, *registryCommitReadbackV1, error) {
	if err := service.validateHead(head); err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		domainevidence.ValidateEvidenceReceiptRegistry(nextRegistry) != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, ErrAuthorityIntegrity
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		securityContext, nextRegistry, service.keyID, service.publicKey,
		func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) },
	)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	mutationID, err := service.nextMutationIDLocked(ctx, head.Bundle.RecordDigest, securityContext.ContextDigest)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	index, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: service.installationID, EnrollmentID: service.enrollmentID,
		Generation: head.Bundle.EvidenceRegistryCount + 1, PreviousIndexDigest: head.Bundle.EvidenceRegistryIndexDigest, MutationID: mutationID,
	}, capsule, service.keyID, service.publicKey, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	if err := service.validateEntryAppendLocked(ctx, head, index, capsule); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	if err := service.capsules.PutIfAbsent(ctx, capsule); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	storedCapsule, err := service.capsules.Resolve(ctx, capsule.RecordDigest)
	if err != nil || !reflect.DeepEqual(storedCapsule, capsule) {
		return domainevidence.EvidenceReceiptRegistry{}, nil, ErrAuthorityIntegrity
	}
	if err := service.indexes.PutIfAbsent(ctx, index); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	storedIndex, err := service.indexes.Resolve(ctx, index.IndexDigest)
	if err != nil || !reflect.DeepEqual(storedIndex, index) {
		return domainevidence.EvidenceReceiptRegistry{}, nil, ErrAuthorityIntegrity
	}
	advanced, err := service.coordinator.AdvanceEvidenceRegistry(ctx, evidenceauthorityport.RegistryAdvanceInput{
		ExpectedBundleDigest: head.Bundle.RecordDigest, NextIndexDigest: index.IndexDigest,
	})
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, nil, err
	}
	if err := service.validateHead(advanced); err != nil ||
		domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(head.Bundle, advanced.Bundle) != nil ||
		advanced.Bundle.EvidenceRegistryIndexDigest != index.IndexDigest ||
		advanced.Bundle.EvidenceRegistryCount != head.Bundle.EvidenceRegistryCount+1 ||
		advanced.Bundle.DatasetSnapshotIndexDigest != head.Bundle.DatasetSnapshotIndexDigest ||
		advanced.Bundle.DatasetSnapshotCount != head.Bundle.DatasetSnapshotCount ||
		advanced.Bundle.PublicationIndexDigest != head.Bundle.PublicationIndexDigest ||
		advanced.Bundle.PublicationCount != head.Bundle.PublicationCount {
		return domainevidence.EvidenceReceiptRegistry{}, nil, ErrAuthorityIntegrity
	}
	committed, err := service.registryFromHeadLocked(ctx, advanced, securityContext)
	if err != nil || committed.StateDigest != nextRegistry.StateDigest || committed.Sequence != nextRegistry.Sequence {
		return domainevidence.EvidenceReceiptRegistry{}, nil, ErrAuthorityIntegrity
	}
	return committed, &registryCommitReadbackV1{before: head, after: advanced, index: storedIndex, capsule: storedCapsule}, nil
}

func (service *Service) validateEntryAppendLocked(
	ctx context.Context,
	head evidenceauthorityport.FreshHead,
	index domainevidence.EvidenceRegistryAuthorityIndexV2,
	capsule domainevidence.EvidenceRegistryAuthorityCapsule,
) error {
	if head.Bundle.EvidenceRegistryCount == 0 {
		if index.Entry.RegistrySequence != 1 {
			return errors.New("first evidence registry authority entry must start at sequence one")
		}
		return nil
	}
	currentDigest := head.Bundle.EvidenceRegistryIndexDigest
	for generation := head.Bundle.EvidenceRegistryCount; generation > 0; generation-- {
		previous, err := service.indexes.Resolve(ctx, currentDigest)
		if err != nil || previous.Generation != generation {
			return ErrAuthorityIntegrity
		}
		entry := previous.Entry
		if entry.ThreadID == index.Entry.ThreadID && entry.TurnID == index.Entry.TurnID {
			return domainevidence.ValidateEvidenceRegistryAuthorityIndexEntryExtensionV2(entry, index.Entry, capsule)
		}
		if generation == 1 {
			break
		}
		currentDigest = previous.PreviousIndexDigest
	}
	if index.Entry.RegistrySequence != 1 {
		return errors.New("new evidence registry authority entry must start at sequence one")
	}
	return nil
}

func (service *Service) validateHead(head evidenceauthorityport.FreshHead) error {
	if !head.HasBundle {
		return ErrAuthorityNotInitialized
	}
	if _, err := domainevidence.NewEvidenceAuthorityWitnessBindingV1(
		head.Bundle, head.Request, head.Observation,
		service.installationID, service.enrollmentID, service.keyID, service.publicKey,
		service.witnessKeyID, service.witnessKey,
	); err != nil {
		return ErrAuthorityIntegrity
	}
	return nil
}

func (service *Service) nextMutationIDLocked(ctx context.Context, bundleDigest, contextDigest string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	random := make([]byte, 32)
	if _, err := io.ReadFull(service.random, random); err != nil {
		return "", err
	}
	service.mutationCounter++
	counter := make([]byte, 8)
	binary.BigEndian.PutUint64(counter, service.mutationCounter)
	material := append([]byte("analytix.evidence-registry-index/mutation/v2\x00"), random...)
	material = append(material, counter...)
	material = append(material, bundleDigest...)
	material = append(material, contextDigest...)
	return domainsecurity.SHA256Hex(material), nil
}
