package evidenceregistry

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

var _ registryport.FactFinalWitnessIssuer = (*HostLocalServiceV3)(nil)
var _ registryport.FactFinalWitnessVerifier = (*HostLocalServiceV3)(nil)
var _ registryport.RecoveredFactFinalWitnessIssuer = (*HostLocalServiceV3)(nil)

// The shared interfaces retain their historical names; this implementation
// issues only the distinct host-local V6 grammar and never a witness binding.
func (service *HostLocalServiceV3) WithFactFinalWitnessAuthority(ctx context.Context, request registryport.FactFinalWitnessRequest,
	use func(registryport.FactFinalWitnessCapability) error) error {
	if service == nil || ctx == nil || use == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(request.Context) != nil ||
		domainevidence.ValidateFinalAnswerEnvelope(request.Envelope) != nil ||
		domainevidence.ValidateTerminalPublicationIntent(request.PublicationIntent, request.Envelope.TerminalReason) != nil ||
		!domainevidence.FinalAnswerRequiresPublicationSnapshotProof(request.Envelope) || strings.TrimSpace(request.RenderedText) == "" ||
		validateFactFinalHostEvidenceRequestV2(request) != nil || request.DatasetSelection.HostLocalHead == nil {
		return ErrAuthorityUnavailable
	}
	// Serialize the complete callback, including nested UseExact calls in the
	// terminal coordinator. Reacquiring the writer inside UseExact would deadlock.
	return service.withFinalReadGuard(ctx, func(lease context.Context) error {
		var record domainevidence.PrivateAcceptedFinalRecord
		err := request.HostEvidenceCapability.UseExact(request.Context, request.SourceProbes[0], request.DatasetSelection, func(sourceLease context.Context) error {
			service.mu.Lock()
			defer service.mu.Unlock()
			head, selection, err := service.currentLocked(sourceLease, request.Context)
			if err != nil || !selection.hasSelection || head.DatasetSnapshotIndexDigest != request.DatasetSelection.HostLocalHead.DatasetSnapshotIndexDigest ||
				head.DatasetSnapshotCount != request.DatasetSelection.HostLocalHead.DatasetSnapshotCount {
				return errors.Join(ErrAuthorityIntegrity, err)
			}
			mode, err := service.heads.CurrentModeCommitment(sourceLease)
			if err != nil || mode.RecordDigest != service.modeCommitmentDigest {
				return errors.Join(ErrAuthorityIntegrity, err)
			}
			input := service.hostLocalFinalInput(request.Context, request.Envelope, request.RenderedText, request.PublicationProof,
				head, mode, selection, request.DatasetSelection, request.BindingObservation, service.base.now().UTC())
			admission, err := domainevidence.NewFactFinalHostLocalAdmissionV1(input)
			if err != nil {
				return err
			}
			registryHead, err := domainevidence.NewEvidenceRegistryHead(selection.registry)
			if err != nil {
				return err
			}
			privateDigest, err := domainevidence.PrivateAcceptedFinalDigestWithHostLocalV1(request.Context, request.Envelope, request.RenderedText, request.PublicationIntent, request.PublicationProof, &admission)
			if err != nil {
				return err
			}
			accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
				Context: request.Context, Envelope: request.Envelope, RenderedText: request.RenderedText, RegistryHead: registryHead,
				PublicationSnapshotProof: request.PublicationProof, FactFinalHostLocalAdmission: &admission, FactFinalHostLocalAuthority: &input,
				PrivateRecordDigest: privateDigest, AcceptedAt: input.AdmittedAt, AuthorityKeyID: service.base.keyID, AuthorityPublicKey: service.base.publicKey,
			}, func(message []byte) ([]byte, error) { return service.base.authority.Sign(sourceLease, message) })
			if err != nil {
				return err
			}
			record, err = domainevidence.NewPrivateAcceptedFinalRecord(request.Context, request.Envelope, request.RenderedText, registryHead, request.PublicationIntent, accepted, request.PublicationProof)
			return errors.Join(err, service.confirmHead(sourceLease, head))
		})
		if err != nil {
			return err
		}
		return service.withHostLocalFinalCapability(lease, record, use)
	})
}

func (service *HostLocalServiceV3) hostLocalFinalInput(securityContext domainsecurity.TurnSecurityContext, envelope domainevidence.FinalAnswerEnvelope,
	rendered string, proof *domainevidence.PublicationSnapshotProof, head, mode domainhost.HeadV1, registry registrySelectionV2,
	dataset datasetsnapshotport.CurrentSelectionV2, binding domainsecurity.CaseBindingObservationV1, admitted time.Time) domainevidence.FactFinalHostLocalAdmissionInputV1 {
	return domainevidence.FactFinalHostLocalAdmissionInputV1{
		Context: securityContext, Envelope: envelope, RenderedText: rendered, PublicationProof: proof, Registry: registry.registry,
		Head: head, ModeCommitment: mode, RegistryIndexPath: append([]domainevidence.EvidenceRegistryAuthorityIndexV2(nil), registry.indexPath...), SelectedCapsule: registry.selectedCapsule,
		DatasetIndexPath: append([]domainsecurity.DatasetSnapshotIndexV1(nil), dataset.DatasetIndexPath...), SelectedDatasetIndex: dataset.SelectedIndex,
		DatasetRecord: dataset.Snapshot.Record, DatasetManifest: dataset.Snapshot.Manifest, FundsProducerContent: dataset.Snapshot.FundsProducerContent,
		BindingObservation: binding, InstallationID: service.base.installationID, RootBindingDigest: service.rootBindingDigest,
		AuthorityKeyID: service.base.keyID, AuthorityPublicKey: append([]byte(nil), service.base.publicKey...), AdmittedAt: admitted,
	}
}

func (service *HostLocalServiceV3) VerifyFactFinalWitnessCurrent(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
	if service == nil || ctx == nil {
		return ErrAuthorityUnavailable
	}
	return service.withFinalReadGuard(ctx, func(lease context.Context) error {
		return service.withVerifiedHostLocalFinal(lease, record, func() error { return nil })
	})
}

func (service *HostLocalServiceV3) WithRecoveredFactFinalWitness(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord,
	use func(registryport.FactFinalWitnessCapability) error) error {
	if service == nil || ctx == nil || use == nil {
		return ErrAuthorityUnavailable
	}
	return service.withFinalReadGuard(ctx, func(lease context.Context) error {
		return service.withHostLocalFinalCapability(lease, record, use)
	})
}

func (service *HostLocalServiceV3) withFinalReadGuard(ctx context.Context, use func(context.Context) error) error {
	guard, ok := service.heads.(evidenceauthorityport.HostLocalFinalReadGuardV1)
	if !ok {
		return ErrAuthorityUnavailable
	}
	return guard.WithProtectedFinalReadV1(ctx, use)
}

func (service *HostLocalServiceV3) withHostLocalFinalCapability(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, use func(registryport.FactFinalWitnessCapability) error) error {
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
	if err != nil {
		return err
	}
	original, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
	if err != nil {
		return err
	}
	if err := service.withVerifiedHostLocalFinal(ctx, original, func() error { return nil }); err != nil {
		return err
	}
	capability := &hostLocalFinalCapabilityV1{active: true, ctx: ctx, service: service, record: original}
	capability.idle = sync.NewCond(&capability.mu)
	defer capability.close()
	return use(capability)
}

// Called only within the enclosing selected-head serializer. The registry
// mutex is released before use: Final persistence has nested authority checks.
func (service *HostLocalServiceV3) withVerifiedHostLocalFinal(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, use func() error) error {
	if ctx == nil || ctx.Err() != nil || use == nil || service.base.bindingObserver == nil ||
		domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil || record.SchemaVersion != domainevidence.HostLocalAcceptedFinalRecordVersionV6 ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(record.SecurityContext) != nil || record.AcceptedFinal.FactFinalHostLocalAdmission == nil {
		return ErrAuthorityUnavailable
	}
	history, ok := service.heads.(evidenceauthorityport.HostLocalHeadHistoryV1)
	datasetHistory, datasetOK := service.base.datasetAuthority.(datasetsnapshotport.HostLocalHistoricalFactAuthorityV1)
	if !ok || !datasetOK {
		return ErrAuthorityUnavailable
	}
	admission := *record.AcceptedFinal.FactFinalHostLocalAdmission
	historical, err := history.ResolveRetainedHeadV1(ctx, admission.HostLocalHead.RecordDigest)
	if err != nil || historical != admission.HostLocalHead {
		return errors.Join(ErrAuthorityIntegrity, err)
	}
	mode, err := service.heads.CurrentModeCommitment(ctx)
	if err != nil || mode.RecordDigest != service.modeCommitmentDigest || mode.RecordDigest != admission.ModeCommitmentDigest {
		return errors.Join(ErrAuthorityIntegrity, err)
	}
	service.mu.Lock()
	current, selected, err := service.currentLocked(ctx, record.SecurityContext)
	original, originalErr := service.base.registrySelectionFromRootLocked(ctx, historical.EvidenceRegistryIndexDigest, historical.EvidenceRegistryCount, true, service.modeCommitmentDigest, record.SecurityContext)
	registryHead, headErr := domainevidence.NewEvidenceRegistryHead(selected.registry)
	service.mu.Unlock()
	if err != nil || originalErr != nil || headErr != nil || !original.hasSelection || !selected.hasSelection ||
		!reflect.DeepEqual(selected.selectedCapsule, original.selectedCapsule) || selected.selectedIndex != original.selectedIndex || registryHead != record.RegistryHead ||
		domainevidence.ValidatePublicationSnapshotProofAgainstRegistry(record.PublicationSnapshotProof, record.SecurityContext, record.Envelope, selected.registry) != nil {
		return errors.Join(ErrAuthorityIntegrity, err, originalErr, headErr)
	}
	binding, err := service.base.bindingObserver.Observe(record.SecurityContext.WorkspaceRealPath)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(binding) != nil || binding.State != domainsecurity.CaseBindingStateValid ||
		binding.WorkspaceRealPath != record.SecurityContext.WorkspaceRealPath || binding.CaseID != record.SecurityContext.CaseID || binding.CaseBindingHash != record.SecurityContext.CaseBindingHash ||
		binding.ObservationDigest != record.SecurityContext.PublicationPolicy.BindingObservationDigest {
		return errors.Join(ErrAuthorityIntegrity, err)
	}
	admittedAt, err := time.Parse(time.RFC3339Nano, admission.AdmittedAt)
	if err != nil {
		return err
	}
	return datasetHistory.WithHostLocalHistoricalFactSelectionV1(ctx, datasetsnapshotport.ResolveInputV2{
		TenantID: record.SecurityContext.TenantID, UserID: record.SecurityContext.UserID, Observation: binding, ExpectedDatasetSnapshotID: record.SecurityContext.DatasetSnapshotID,
	}, record.SecurityContext, historical, func(selection datasetsnapshotport.CurrentSelectionV2) error {
		input := service.hostLocalFinalInput(record.SecurityContext, record.Envelope, record.RenderedText, record.PublicationSnapshotProof, historical, mode, original, selection, binding, admittedAt)
		if err := domainevidence.ValidateFactFinalHostLocalAdmissionExactV1(admission, input); err != nil {
			return err
		}
		if err := service.confirmHead(ctx, current); err != nil {
			return err
		}
		useErr := use()
		afterBinding, bindingErr := service.base.bindingObserver.Observe(record.SecurityContext.WorkspaceRealPath)
		if bindingErr == nil && !reflect.DeepEqual(afterBinding, binding) {
			bindingErr = ErrAuthorityIntegrity
		}
		return errors.Join(useErr, bindingErr, service.confirmHead(ctx, current), ctx.Err())
	})
}

type hostLocalFinalCapabilityV1 struct {
	mu      sync.Mutex
	idle    *sync.Cond
	active  bool
	uses    uint64
	ctx     context.Context
	service *HostLocalServiceV3
	record  domainevidence.PrivateAcceptedFinalRecord
}

func (capability *hostLocalFinalCapabilityV1) begin() bool {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if !capability.active || capability.ctx.Err() != nil {
		return false
	}
	capability.uses++
	return true
}
func (capability *hostLocalFinalCapabilityV1) end() {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	capability.uses--
	if capability.uses == 0 {
		capability.idle.Broadcast()
	}
}
func (capability *hostLocalFinalCapabilityV1) close() {
	capability.mu.Lock()
	defer capability.mu.Unlock()
	capability.active = false
	for capability.uses > 0 {
		capability.idle.Wait()
	}
}
func (capability *hostLocalFinalCapabilityV1) PrivateFinal() (domainevidence.PrivateAcceptedFinalRecord, error) {
	if !capability.begin() {
		return domainevidence.PrivateAcceptedFinalRecord{}, ErrAuthorityUnavailable
	}
	defer capability.end()
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(capability.record)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	return domainevidence.ParsePrivateAcceptedFinalRecord(body)
}
func (capability *hostLocalFinalCapabilityV1) UseExact(record domainevidence.PrivateAcceptedFinalRecord, use func() error) error {
	if !capability.begin() {
		return ErrAuthorityUnavailable
	}
	defer capability.end()
	if use == nil || !reflect.DeepEqual(record, capability.record) {
		return ErrAuthorityIntegrity
	}
	return capability.service.withVerifiedHostLocalFinal(capability.ctx, capability.record, use)
}
func (*hostLocalFinalCapabilityV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("host-local Final capability is not serializable")
}
