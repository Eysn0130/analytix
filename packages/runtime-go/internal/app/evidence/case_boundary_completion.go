package evidence

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	appturn "analytix.local/runtime-go/internal/app/turn"
	appturnterminal "analytix.local/runtime-go/internal/app/turnterminal"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

type PersistCaseBoundaryInput struct {
	Store             appturn.AcceptedFinalCompletionStore
	Context           domainsecurity.TurnSecurityContext
	TerminalReason    TerminalReason
	OrdinaryResult    *domainordinaryresult.ResultSlotV1
	CaseSlotIntent    CaseSlotPublicationIntentV1
	SourceUnavailable bool
	ReportRequested   bool
	ThreadID          string
	TurnID            string
	Model             string
	CreatedAt         string
	AcceptedAt        time.Time
	Telemetry         appusage.TerminalTelemetryV1
	UsageSource       string
	ChildRunID        string
	TerminalStatus    string
	Discard           bool
	Cancelled         bool
	CancelledGates    int
}

type CasePublicationFinalizer interface {
	PersistBoundary(context.Context, PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error)
}

const caseEvidenceAuthorityUnavailableBlockerV1 = domainevidence.CaseEvidenceAuthorityUnavailableBlockerV1

type caseEvidenceAuthorityUnavailableV1 interface {
	CaseEvidenceAuthorityUnavailableV1() bool
}

func isCaseEvidenceAuthorityUnavailableV1(authority any) bool {
	marker, ok := authority.(caseEvidenceAuthorityUnavailableV1)
	return ok && marker.CaseEvidenceAuthorityUnavailableV1()
}

// CaseInterruptPublicationAuthority reconciles an already-committed case
// interrupt from the original private final. It must never mint a new final.
type CaseInterruptPublicationAuthority interface {
	ReconcileCommittedInterrupt(context.Context, appturn.AcceptedFinalCompletionStore, string, string) (domainevidence.TerminalPublicationIntent, bool, error)
}

type casePublicationFinalizer struct {
	privateAdmissionMu   sync.Mutex
	registry             registryport.Registry
	lockedSnapshots      registryport.LockedSnapshot
	authority            authorityport.Authority
	privateStore         authorityport.PrivateFinalStore
	eventIO              FinalPublicationEventIO
	trustedProjection    *gateprojection.TrustedFinalProjectionIndex
	terminalCoordinator  *appturnterminal.Coordinator
	publicationSnapshots sourceprobeport.LockedPublicationSnapshot
	hostSnapshots        sourceprobeport.LockedPublicationSnapshotAuthority
	bindingObserver      casecontextport.Observer
	factFinalWitnesses   registryport.FactFinalWitnessIssuer
}

// NewCasePublicationFinalizerWithHostEvidenceAuthority is the production
// fact-publication composition. The compatibility constructor below cannot
// carry the callback-scoped source and DSV2 authority required by a new fact
// final and therefore remains boundary-only for that path.
func NewCasePublicationFinalizerWithHostEvidenceAuthority(
	registry registryport.Registry,
	lockedSnapshots registryport.LockedSnapshot,
	authority authorityport.Authority,
	privateStore authorityport.PrivateFinalStore,
	eventIO FinalPublicationEventIO,
	terminalCoordinator *appturnterminal.Coordinator,
	publicationSnapshots sourceprobeport.LockedPublicationSnapshotAuthority,
	bindingObserver casecontextport.Observer,
	trusted ...*gateprojection.TrustedFinalProjectionIndex,
) CasePublicationFinalizer {
	projection := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	if len(trusted) > 0 && trusted[0] != nil {
		projection = trusted[0]
	}
	factFinalWitnesses, _ := lockedSnapshots.(registryport.FactFinalWitnessIssuer)
	return &casePublicationFinalizer{
		registry: registry, lockedSnapshots: lockedSnapshots, authority: authority, privateStore: privateStore,
		eventIO: eventIO, trustedProjection: projection, terminalCoordinator: terminalCoordinator,
		hostSnapshots: publicationSnapshots, bindingObserver: bindingObserver, factFinalWitnesses: factFinalWitnesses,
	}
}

func NewCasePublicationFinalizerWithAuthority(registry registryport.Registry, lockedSnapshots registryport.LockedSnapshot, authority authorityport.Authority, privateStore authorityport.PrivateFinalStore, eventIO FinalPublicationEventIO, terminalCoordinator *appturnterminal.Coordinator) CasePublicationFinalizer {
	return &casePublicationFinalizer{registry: registry, lockedSnapshots: lockedSnapshots, authority: authority, privateStore: privateStore, eventIO: eventIO, trustedProjection: gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback), terminalCoordinator: terminalCoordinator}
}

func NewCasePublicationFinalizerWithPublicationSnapshots(registry registryport.Registry, lockedSnapshots registryport.LockedSnapshot, authority authorityport.Authority, privateStore authorityport.PrivateFinalStore, eventIO FinalPublicationEventIO, terminalCoordinator *appturnterminal.Coordinator, publicationSnapshots sourceprobeport.LockedPublicationSnapshot, trusted ...*gateprojection.TrustedFinalProjectionIndex) CasePublicationFinalizer {
	projection := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	if len(trusted) > 0 && trusted[0] != nil {
		projection = trusted[0]
	}
	factFinalWitnesses, _ := lockedSnapshots.(registryport.FactFinalWitnessIssuer)
	return &casePublicationFinalizer{
		registry: registry, lockedSnapshots: lockedSnapshots, authority: authority, privateStore: privateStore,
		eventIO: eventIO, trustedProjection: projection, terminalCoordinator: terminalCoordinator,
		publicationSnapshots: publicationSnapshots, factFinalWitnesses: factFinalWitnesses,
	}
}

type PersistCaseBoundaryResult struct {
	Boundary                           CaseBoundaryResult
	Persistence                        appturn.PersistAcceptedFinalResult
	PublicationIntent                  domainevidence.TerminalPublicationIntent
	useCaseLongitudinalAcceptedSlotsV1 func(func(
		domainsecurity.TurnSecurityContext,
		string,
		string,
		string,
		domainevidence.AcceptedEntitySlotBindingV1,
	) error) error
}

func (result *PersistCaseBoundaryResult) CaseLongitudinalClaimsV1() ([]domainevidence.ClaimRecord, bool) {
	if result == nil {
		return nil, false
	}
	return append([]domainevidence.ClaimRecord(nil), result.Boundary.Envelope.Claims...), true
}

// UseCaseLongitudinalAcceptedSlotsV1 exposes only the closed, committed
// production-finalization bridge needed by the private case owner. The
// capability is closure-backed so neither private final authority nor entity
// references become ordinary serializable result fields.
func (result *PersistCaseBoundaryResult) UseCaseLongitudinalAcceptedSlotsV1(use func(
	domainsecurity.TurnSecurityContext,
	string,
	string,
	string,
	domainevidence.AcceptedEntitySlotBindingV1,
) error) (bool, error) {
	if result == nil || result.useCaseLongitudinalAcceptedSlotsV1 == nil {
		return false, nil
	}
	if use == nil {
		return true, errors.New("case longitudinal accepted slot use is invalid")
	}
	return true, result.useCaseLongitudinalAcceptedSlotsV1(use)
}

func (finalizer *casePublicationFinalizer) PersistBoundary(ctx context.Context, input PersistCaseBoundaryInput) (PersistCaseBoundaryResult, error) {
	if ctx == nil || finalizer == nil || finalizer.registry == nil || finalizer.lockedSnapshots == nil || finalizer.authority == nil || finalizer.privateStore == nil ||
		finalizer.terminalCoordinator == nil || !finalizer.eventIO.validForLiveDelivery() || finalizer.eventIO.ReadCASObservation == nil {
		return PersistCaseBoundaryResult{}, errors.New("case publication authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return PersistCaseBoundaryResult{}, err
	}
	if domainsecurity.ValidateTurnSecurityContextForCasePublication(input.Context) != nil || strings.TrimSpace(input.ThreadID) != input.Context.ThreadID ||
		strings.TrimSpace(input.TurnID) != input.Context.TurnID {
		return PersistCaseBoundaryResult{}, errors.New("case publication context is invalid")
	}
	acceptedAt := input.AcceptedAt.UTC()
	if acceptedAt.IsZero() {
		acceptedAt = time.Now().UTC()
	}
	if isCaseEvidenceAuthorityUnavailableV1(finalizer.registry) ||
		isCaseEvidenceAuthorityUnavailableV1(finalizer.lockedSnapshots) {
		return finalizer.persistUnavailableBoundaryV1(ctx, input, acceptedAt)
	}
	var result PersistCaseBoundaryResult
	type preliminaryFactFinalV1 struct {
		snapshot     domainevidence.EvidenceReceiptRegistry
		boundary     CaseBoundaryResult
		requirements []sourceprobeport.PublicationSourceRequirement
	}
	var preliminary *preliminaryFactFinalV1
	callbackEntered := false
	err := finalizer.lockedSnapshots.WithLockedSnapshot(ctx, input.Context, func(snapshot domainevidence.EvidenceReceiptRegistry) error {
		callbackEntered = true
		boundary, err := finalizeCasePublicationSnapshotV1(ctx, snapshot, input, acceptedAt, "")
		if err != nil {
			return err
		}
		requirements, err := publicationSnapshotRequirements(snapshot, input.Context, boundary.Envelope)
		if err != nil {
			return err
		}
		if len(requirements) == 0 {
			privateRecord, err := finalizer.newBoundaryPrivateFinalV1(ctx, input, boundary, snapshot, acceptedAt)
			if err != nil {
				return err
			}
			return finalizer.persistPrivateFinalV1(ctx, input, boundary, privateRecord, nil, &result)
		}
		frozen, err := domainevidence.ParseEvidenceReceiptRegistry(snapshot)
		if err != nil {
			return err
		}
		preliminary = &preliminaryFactFinalV1{
			snapshot: frozen, boundary: boundary,
			requirements: append([]sourceprobeport.PublicationSourceRequirement(nil), requirements...),
		}
		return nil
	})
	if err != nil && !callbackEntered {
		if ctx.Err() != nil {
			return result, errors.Join(err, ctx.Err())
		}
		if evidenceauthorityapp.IsWitnessObserveUnavailableV1(err) {
			return finalizer.persistUnavailableBoundaryV1(ctx, input, acceptedAt)
		}
	}
	if err != nil || preliminary == nil {
		return result, err
	}
	downgrade := func() (PersistCaseBoundaryResult, error) {
		return finalizer.persistPublicationSnapshotBoundaryV1(ctx, input, acceptedAt)
	}
	if (finalizer.publicationSnapshots == nil && finalizer.hostSnapshots == nil) || finalizer.factFinalWitnesses == nil {
		return downgrade()
	}
	intent, err := terminalPublicationIntentV1(input, preliminary.boundary, acceptedAt)
	if err != nil {
		return PersistCaseBoundaryResult{}, err
	}
	if finalizer.hostSnapshots != nil {
		binding, err := observePublicationCaseBindingV2(finalizer.bindingObserver, input.Context)
		if err != nil {
			return downgrade()
		}
		mutationEntered := false
		err = finalizer.hostSnapshots.WithFreshPublicationSnapshotAuthority(
			ctx,
			sourceprobeport.PublicationInput{
				Context: input.Context, Binding: binding, Requirements: preliminary.requirements,
			},
			func(
				probes []domainsecurity.VerifiedSourceProbe,
				hostCapability sourceprobeport.HostEvidenceCapability,
			) error {
				if hostCapability == nil || len(probes) != 1 {
					return errors.New("fact final host evidence authority is incomplete")
				}
				selection, err := hostCapability.DatasetSelection()
				if err != nil {
					return err
				}
				proof, err := newPublicationSnapshotProofForHostAuthorityV2(
					input.Context,
					preliminary.snapshot,
					preliminary.boundary.Envelope,
					preliminary.requirements,
					probes,
				)
				if err != nil {
					return err
				}
				return finalizer.factFinalWitnesses.WithFactFinalWitnessAuthority(
					ctx,
					registryport.FactFinalWitnessRequest{
						Context: input.Context, Envelope: preliminary.boundary.Envelope,
						RenderedText: preliminary.boundary.Text, PublicationProof: &proof,
						PublicationIntent: intent, BindingObservation: binding,
						SourceProbes:     append([]domainsecurity.VerifiedSourceProbe(nil), probes...),
						DatasetSelection: selection, HostEvidenceCapability: hostCapability,
					},
					func(capability registryport.FactFinalWitnessCapability) error {
						privateRecord, err := capability.PrivateFinal()
						if err != nil || validateWitnessedPrivateFinalV1(
							ctx, finalizer.authority, privateRecord, input.Context,
							preliminary.boundary, intent, proof,
						) != nil {
							return errors.Join(errors.New("fact final witness returned mismatched authority"), err)
						}
						return hostCapability.UseExact(
							input.Context,
							probes[0],
							selection,
							func(leaseContext context.Context) error {
								return capability.UseExact(privateRecord, func() error {
									mutationEntered = true
									return finalizer.persistPrivateFinalV1(
										leaseContext, input, preliminary.boundary, privateRecord, capability, &result,
									)
								})
							},
						)
					},
				)
			},
		)
		if err != nil {
			if mutationEntered {
				return result, err
			}
			return downgrade()
		}
	} else {
		mutationEntered := false
		err = finalizer.publicationSnapshots.WithFreshPublicationSnapshot(ctx, sourceprobeport.PublicationInput{
			Context: input.Context, Requirements: preliminary.requirements,
		}, func(probes []domainsecurity.VerifiedSourceProbe) error {
			proof, err := newPublicationSnapshotProof(
				input.Context, preliminary.snapshot, preliminary.boundary.Envelope, preliminary.requirements, probes,
			)
			if err != nil {
				return err
			}
			return finalizer.factFinalWitnesses.WithFactFinalWitnessAuthority(ctx, registryport.FactFinalWitnessRequest{
				Context: input.Context, Envelope: preliminary.boundary.Envelope, RenderedText: preliminary.boundary.Text,
				PublicationProof: &proof, PublicationIntent: intent,
			}, func(capability registryport.FactFinalWitnessCapability) error {
				privateRecord, err := capability.PrivateFinal()
				if err != nil || validateWitnessedPrivateFinalV1(
					ctx, finalizer.authority, privateRecord, input.Context, preliminary.boundary, intent, proof,
				) != nil {
					return errors.Join(errors.New("fact final witness returned mismatched authority"), err)
				}
				return capability.UseExact(privateRecord, func() error {
					mutationEntered = true
					return finalizer.persistPrivateFinalV1(
						ctx, input, preliminary.boundary, privateRecord, capability, &result,
					)
				})
			})
		})
		if err != nil {
			if mutationEntered {
				return result, err
			}
			return downgrade()
		}
	}
	return result, nil
}

// This context-bound zero head belongs only to the new fixed non-fact
// boundary. It neither reconstructs nor replaces any Original registry.
func (finalizer *casePublicationFinalizer) persistUnavailableBoundaryV1(ctx context.Context, input PersistCaseBoundaryInput, acceptedAt time.Time) (PersistCaseBoundaryResult, error) {
	if err := ctx.Err(); err != nil {
		return PersistCaseBoundaryResult{}, err
	}
	snapshot, err := domainevidence.NewEvidenceReceiptRegistry(input.Context)
	if err != nil {
		return PersistCaseBoundaryResult{}, err
	}
	boundary, err := finalizeCasePublicationSnapshotV1(ctx, snapshot, input, acceptedAt, caseEvidenceAuthorityUnavailableBlockerV1)
	if err != nil {
		return PersistCaseBoundaryResult{}, err
	}
	privateRecord, err := finalizer.newBoundaryPrivateFinalV1(ctx, input, boundary, snapshot, acceptedAt)
	if err != nil {
		return PersistCaseBoundaryResult{}, err
	}
	var result PersistCaseBoundaryResult
	err = finalizer.persistPrivateFinalV1(ctx, input, boundary, privateRecord, nil, &result)
	return result, err
}

func finalizeCasePublicationSnapshotV1(
	ctx context.Context,
	snapshot domainevidence.EvidenceReceiptRegistry,
	input PersistCaseBoundaryInput,
	acceptedAt time.Time,
	publicationBlocker string,
) (CaseBoundaryResult, error) {
	if publicationBlocker == "" && domainsecurity.TurnSecurityContextIsBoundaryOnly(input.Context) {
		publicationBlocker = input.Context.PublicationPolicy.BlockerCode
	}
	return FinalizeCaseBoundary(ctx, newSnapshotRegistry(snapshot), CaseBoundaryInput{
		Context: input.Context, TerminalReason: input.TerminalReason, OrdinaryResult: input.OrdinaryResult,
		CaseSlotIntent:    input.CaseSlotIntent,
		SourceUnavailable: input.SourceUnavailable,
		ReportRequested:   input.ReportRequested, PublicationBlocker: publicationBlocker, IssuedAt: acceptedAt,
	})
}

func terminalPublicationIntentV1(
	input PersistCaseBoundaryInput,
	boundary CaseBoundaryResult,
	acceptedAt time.Time,
) (domainevidence.TerminalPublicationIntent, error) {
	disposition, err := DispositionForTerminalReason(input.TerminalReason)
	if err != nil || (strings.TrimSpace(input.TerminalStatus) != "" && strings.TrimSpace(input.TerminalStatus) != disposition.Status) {
		if err == nil {
			err = errors.New("case terminal status contradicts terminal reason")
		}
		return domainevidence.TerminalPublicationIntent{}, err
	}
	createdAt := strings.TrimSpace(input.CreatedAt)
	if createdAt == "" {
		createdAt = acceptedAt.Format(time.RFC3339Nano)
	}
	return domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: createdAt, Model: input.Model, Usage: input.Telemetry.PublicUsageMap(),
		CacheDiagnostics: input.Telemetry.PublicCacheDiagnosticsMap(), UsageSource: input.UsageSource, ChildRunID: input.ChildRunID,
		TerminalStatus: disposition.Status, TerminalCode: disposition.Code, TerminalMessage: disposition.Message,
		TerminalSeverity: disposition.Severity, Discard: input.Discard, Cancelled: input.Cancelled,
		CancelledPendingGates: input.CancelledGates,
	}, boundary.Envelope.TerminalReason)
}

func (finalizer *casePublicationFinalizer) newBoundaryPrivateFinalV1(
	ctx context.Context,
	input PersistCaseBoundaryInput,
	boundary CaseBoundaryResult,
	snapshot domainevidence.EvidenceReceiptRegistry,
	acceptedAt time.Time,
) (domainevidence.PrivateAcceptedFinalRecord, error) {
	intent, err := terminalPublicationIntentV1(input, boundary, acceptedAt)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	head, err := domainevidence.NewEvidenceRegistryHead(snapshot)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(input.Context, boundary.Envelope, boundary.Text, intent)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	acceptedFinal, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: input.Context, Envelope: boundary.Envelope, RenderedText: boundary.Text, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: acceptedAt, AuthorityKeyID: finalizer.authority.KeyID(),
		AuthorityPublicKey: finalizer.authority.PublicKey(),
	}, func(message []byte) ([]byte, error) { return finalizer.authority.Sign(ctx, message) })
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	return domainevidence.NewPrivateAcceptedFinalRecord(
		input.Context, boundary.Envelope, boundary.Text, head, intent, acceptedFinal,
	)
}

func (finalizer *casePublicationFinalizer) persistPublicationSnapshotBoundaryV1(
	ctx context.Context,
	input PersistCaseBoundaryInput,
	acceptedAt time.Time,
) (PersistCaseBoundaryResult, error) {
	var result PersistCaseBoundaryResult
	err := finalizer.lockedSnapshots.WithLockedSnapshot(ctx, input.Context, func(snapshot domainevidence.EvidenceReceiptRegistry) error {
		blocked, err := finalizeCasePublicationSnapshotV1(
			ctx, snapshot, input, acceptedAt, "publication_snapshot_not_fresh",
		)
		if err != nil {
			return err
		}
		privateRecord, err := finalizer.newBoundaryPrivateFinalV1(ctx, input, blocked, snapshot, acceptedAt)
		if err != nil {
			return err
		}
		return finalizer.persistPrivateFinalV1(ctx, input, blocked, privateRecord, nil, &result)
	})
	return result, err
}

func validateWitnessedPrivateFinalV1(
	ctx context.Context,
	authority authorityport.Authority,
	record domainevidence.PrivateAcceptedFinalRecord,
	securityContext domainsecurity.TurnSecurityContext,
	boundary CaseBoundaryResult,
	intent domainevidence.TerminalPublicationIntent,
	proof domainevidence.PublicationSnapshotProof,
) error {
	if authority == nil || domainevidence.ValidatePrivateAcceptedFinalRecord(record) != nil ||
		!reflect.DeepEqual(record.SecurityContext, securityContext) ||
		!reflect.DeepEqual(record.Envelope, boundary.Envelope) || record.RenderedText != boundary.Text ||
		!reflect.DeepEqual(record.PublicationIntent, intent) ||
		record.PublicationSnapshotProof == nil || !reflect.DeepEqual(*record.PublicationSnapshotProof, proof) ||
		record.AcceptedFinal.FactFinalWitnessAdmission == nil {
		return errors.New("fact final witness private record is not exact")
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(record.AcceptedFinal)
	if err != nil || authority.VerifyTrusted(
		ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(record.AcceptedFinal), signature,
	) != nil {
		return errors.New("fact final witness private record is not installation-trusted")
	}
	return nil
}

func (finalizer *casePublicationFinalizer) persistPrivateFinalV1(
	ctx context.Context,
	input PersistCaseBoundaryInput,
	boundary CaseBoundaryResult,
	privateRecord domainevidence.PrivateAcceptedFinalRecord,
	factAuthority appturn.FactFinalMutationAuthority,
	result *PersistCaseBoundaryResult,
) error {
	if result == nil || domainevidence.ValidatePrivateAcceptedFinalRecord(privateRecord) != nil ||
		privateRecord.SecurityContext.ThreadID != input.ThreadID || privateRecord.SecurityContext.TurnID != input.TurnID ||
		!reflect.DeepEqual(privateRecord.Envelope, boundary.Envelope) || privateRecord.RenderedText != boundary.Text {
		return errors.New("private accepted final persistence input is invalid")
	}
	keyID, publicKey, signature, err := domainevidence.AcceptedFinalAuthorityMaterial(privateRecord.AcceptedFinal)
	if err != nil || finalizer.authority.VerifyTrusted(
		ctx, keyID, publicKey, domainevidence.AcceptedFinalSigningBytes(privateRecord.AcceptedFinal), signature,
	) != nil {
		return errors.New("case publication authority verification failed")
	}
	validatedPlan, err := appturn.BuildAcceptedFinalPublicationPlan(
		privateRecord.AcceptedFinal, boundary.Text, privateRecord.PublicationIntent,
	)
	if err != nil {
		return err
	}
	if err := finalizer.admitPrivateFinalV1(ctx, privateRecord); err != nil {
		return err
	}
	stored, err := finalizer.privateStore.Resolve(ctx, privateRecord.AcceptedFinal.RecordDigest)
	if err != nil || stored.StoreDigest != privateRecord.StoreDigest {
		return errors.New("private accepted final readback verification failed")
	}
	terminalResult, err := finalizer.terminalCoordinator.CommitV1(ctx, appturnterminal.CommitInputV1{
		CompletionStore: input.Store,
		CASReader: casePublicationCASReaderV1{
			store: input.Store, privateRecord: privateRecord, eventIO: finalizer.eventIO,
		},
		PrivateFinal: privateRecord, FactAuthority: factAuthority,
	})
	*result = PersistCaseBoundaryResult{
		Boundary: boundary, Persistence: terminalResult.Persistence, PublicationIntent: privateRecord.PublicationIntent,
	}
	if err != nil {
		return err
	}
	if terminalResult.Persistence.Publication.EventManifestDigest != validatedPlan.EventManifestDigest {
		return errors.New("accepted final publication plan changed after private authority commit")
	}
	storedDisposition := terminalResult.AcceptedFinalDisposition
	if storedDisposition.SchemaVersion != domainevidence.AcceptedFinalDispositionRecordV2 ||
		storedDisposition.State != domainevidence.AcceptedFinalCommitted ||
		storedDisposition.DecisionBasis != domainevidence.AcceptedFinalDecisionSamePublicWinner ||
		storedDisposition.AcceptedFinalDigest != privateRecord.AcceptedFinal.RecordDigest {
		return errors.New("terminal coordinator did not return an exact committed disposition")
	}
	acceptedSlots, err := domainevidence.BuildAcceptedEntitySlotBindingsV1(privateRecord.Envelope)
	if err != nil {
		return errors.New("committed accepted final slots are invalid")
	}
	closedSlots := make([]domainevidence.AcceptedEntitySlotBindingV1, len(acceptedSlots))
	for index, slot := range acceptedSlots {
		closedSlots[index] = slot
		closedSlots[index].ClaimIDs = append([]string(nil), slot.ClaimIDs...)
		closedSlots[index].ReceiptIDs = append([]string(nil), slot.ReceiptIDs...)
	}
	securityContext := privateRecord.SecurityContext
	acceptedFinalDigest := privateRecord.AcceptedFinal.RecordDigest
	dispositionDigest := storedDisposition.RecordDigest
	finalGateVersion := privateRecord.AcceptedFinal.FinalGateVersion
	useCaseLongitudinalAcceptedSlotsV1 := func(use func(
		domainsecurity.TurnSecurityContext,
		string,
		string,
		string,
		domainevidence.AcceptedEntitySlotBindingV1,
	) error) error {
		if use == nil {
			return errors.New("case longitudinal accepted slot use is invalid")
		}
		for _, slot := range closedSlots {
			if err := use(securityContext, acceptedFinalDigest, dispositionDigest, finalGateVersion, slot); err != nil {
				return err
			}
		}
		return nil
	}
	projectionLease, err := finalizer.trustedProjection.StageTerminalComplete(ctx, gateprojection.TerminalCompleteFinalAuthorityV1{
		PrivateFinal: privateRecord, Intent: terminalResult.Intent,
		ProviderClosure: terminalResult.ProviderClosure, PublicObservation: terminalResult.PublicObservation,
		AcceptedFinalDisposition: storedDisposition, TerminalDisposition: terminalResult.TerminalDisposition,
		FactAuthority: factAuthority,
	})
	if err != nil {
		return err
	}
	defer projectionLease.Discard()
	if err := FinalizeAcceptedFinalEventDeliveryV1(
		ctx, finalizer.eventIO, input.Store, privateRecord, projectionLease, factAuthority,
	); err != nil {
		return err
	}
	result.useCaseLongitudinalAcceptedSlotsV1 = useCaseLongitudinalAcceptedSlotsV1
	return nil
}

// A failed CAS can leave an authentic private preparation for startup recovery.
// A fallback must not add a second preparation for that turn while its public
// state is unavailable. Serialize the inventory check with the immutable write;
// exact-record retries remain idempotent and preserve the original authority.
func (finalizer *casePublicationFinalizer) admitPrivateFinalV1(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord) error {
	finalizer.privateAdmissionMu.Lock()
	defer finalizer.privateAdmissionMu.Unlock()
	records, err := finalizer.privateStore.List(ctx)
	if err != nil {
		return err
	}
	for _, existing := range records {
		if existing.SecurityContext.ThreadID == record.SecurityContext.ThreadID &&
			existing.SecurityContext.TurnID == record.SecurityContext.TurnID &&
			existing.AcceptedFinal.RecordDigest != record.AcceptedFinal.RecordDigest {
			return errors.New("case terminal already has another private preparation")
		}
	}
	return finalizer.privateStore.PutIfAbsent(ctx, record)
}

func (finalizer *casePublicationFinalizer) ReconcileCommittedInterrupt(
	ctx context.Context,
	store appturn.AcceptedFinalCompletionStore,
	threadID, turnID string,
) (domainevidence.TerminalPublicationIntent, bool, error) {
	if finalizer == nil || finalizer.privateStore == nil || finalizer.terminalCoordinator == nil ||
		finalizer.trustedProjection == nil || !finalizer.eventIO.validForLiveDelivery() || finalizer.eventIO.ReadCASObservation == nil || store == nil {
		return domainevidence.TerminalPublicationIntent{}, false, errors.New("case interrupt recovery authority is unavailable")
	}
	reader, ok := store.(interface {
		GetThread(string) (map[string]any, error)
	})
	if !ok {
		return domainevidence.TerminalPublicationIntent{}, false, errors.New("case interrupt recovery thread reader is unavailable")
	}
	threadID, turnID = strings.TrimSpace(threadID), strings.TrimSpace(turnID)
	thread, err := reader.GetThread(threadID)
	if err != nil {
		return domainevidence.TerminalPublicationIntent{}, false, err
	}
	status, found, terminal, err := appturn.InspectTerminalAuthorityV1(thread, turnID)
	if err != nil || !found || !terminal {
		return domainevidence.TerminalPublicationIntent{}, false, err
	}
	if status != "aborted" {
		return domainevidence.TerminalPublicationIntent{}, false, nil
	}
	var publicFinal domainevidence.AcceptedFinalRecord
	for _, rawTurn := range caseBoundaryRecordList(thread["turns"]) {
		if strings.TrimSpace(caseBoundaryString(rawTurn, "id")) != turnID {
			continue
		}
		publicFinal, err = domainevidence.ParseAcceptedFinalRecord(rawTurn["acceptedFinal"])
		break
	}
	if err != nil || publicFinal.RecordDigest == "" || publicFinal.TerminalReason != string(TerminalCancel) {
		return domainevidence.TerminalPublicationIntent{}, false, errors.New("case interrupt public terminal authority is invalid")
	}
	privateFinal, err := finalizer.privateStore.Resolve(ctx, publicFinal.RecordDigest)
	if err != nil || privateFinal.AcceptedFinal.RecordDigest != publicFinal.RecordDigest ||
		privateFinal.SecurityContext.ThreadID != threadID || privateFinal.SecurityContext.TurnID != turnID {
		return domainevidence.TerminalPublicationIntent{}, false, errors.Join(errors.New("case interrupt private final is unavailable"), err)
	}
	if domainevidence.FinalAnswerRequiresPublicationSnapshotProof(privateFinal.Envelope) {
		return domainevidence.TerminalPublicationIntent{}, false, errors.New("case interrupt cannot recover a fact-bearing final")
	}
	resolved, err := finalizer.terminalCoordinator.ResolveCommittedV1(ctx, appturnterminal.CommitInputV1{
		CompletionStore: store,
		CASReader: casePublicationCASReaderV1{
			store: store, privateRecord: privateFinal, eventIO: finalizer.eventIO,
		},
		PrivateFinal: privateFinal,
	})
	if err != nil {
		return domainevidence.TerminalPublicationIntent{}, false, err
	}
	terminalAuthority := gateprojection.TerminalCompleteFinalAuthorityV1{
		PrivateFinal: privateFinal, Intent: resolved.Intent, ProviderClosure: resolved.ProviderClosure,
		PublicObservation: resolved.PublicObservation, AcceptedFinalDisposition: resolved.AcceptedFinalDisposition,
		TerminalDisposition: resolved.TerminalDisposition,
	}
	projectionLease, err := finalizer.trustedProjection.StageTerminalComplete(ctx, terminalAuthority)
	if err != nil {
		return domainevidence.TerminalPublicationIntent{}, false, err
	}
	defer projectionLease.Discard()
	if err := FinalizeAcceptedFinalEventDeliveryV1(
		ctx, finalizer.eventIO, store, privateFinal, projectionLease, nil,
	); err != nil {
		return domainevidence.TerminalPublicationIntent{}, false, err
	}
	return privateFinal.PublicationIntent, true, nil
}

func caseBoundaryRecordList(value any) []map[string]any {
	switch records := value.(type) {
	case []map[string]any:
		return records
	case []any:
		out := make([]map[string]any, 0, len(records))
		for _, raw := range records {
			if record, ok := raw.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return nil
	}
}

func caseBoundaryString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

type casePublicationCASReaderV1 struct {
	store         appturn.AcceptedFinalCompletionStore
	privateRecord domainevidence.PrivateAcceptedFinalRecord
	eventIO       FinalPublicationEventIO
}

func (reader casePublicationCASReaderV1) ReadAcceptedFinalCASObservation(ctx context.Context, threadID, turnID string) (domainevidence.AcceptedFinalCASObservationV1, error) {
	if reader.store == nil || reader.eventIO.ReadCASObservation == nil ||
		threadID != reader.privateRecord.SecurityContext.ThreadID || turnID != reader.privateRecord.SecurityContext.TurnID {
		return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("case publication CAS reader binding is invalid")
	}
	return reader.eventIO.ReadCASObservation(ctx, reader.store, reader.privateRecord)
}

func observePublicationCaseBindingV2(
	observer casecontextport.Observer,
	securityContext domainsecurity.TurnSecurityContext,
) (domainsecurity.CaseBindingObservationV1, error) {
	if observer == nil {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("publication case binding observer is unavailable")
	}
	observation, err := observer.Observe(securityContext.WorkspaceRealPath)
	if err != nil || domainsecurity.ValidateCaseBindingObservationV1(observation) != nil ||
		observation.State != domainsecurity.CaseBindingStateValid ||
		observation.WorkspaceRealPath != securityContext.WorkspaceRealPath ||
		observation.CaseID != securityContext.CaseID ||
		observation.CaseBindingHash != securityContext.CaseBindingHash ||
		observation.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest {
		return domainsecurity.CaseBindingObservationV1{}, errors.Join(
			errors.New("publication case binding authority is invalid"),
			err,
		)
	}
	return observation, nil
}

func newPublicationSnapshotProof(securityContext domainsecurity.TurnSecurityContext, snapshot domainevidence.EvidenceReceiptRegistry, envelope domainevidence.FinalAnswerEnvelope, requirements []sourceprobeport.PublicationSourceRequirement, probes []domainsecurity.VerifiedSourceProbe) (domainevidence.PublicationSnapshotProof, error) {
	return newPublicationSnapshotProofWithProbeAuthority(
		securityContext,
		snapshot,
		envelope,
		requirements,
		probes,
		domainsecurity.SourceProbeCanAuthorizeFacts,
	)
}

func newPublicationSnapshotProofForHostAuthorityV2(securityContext domainsecurity.TurnSecurityContext, snapshot domainevidence.EvidenceReceiptRegistry, envelope domainevidence.FinalAnswerEnvelope, requirements []sourceprobeport.PublicationSourceRequirement, probes []domainsecurity.VerifiedSourceProbe) (domainevidence.PublicationSnapshotProof, error) {
	return newPublicationSnapshotProofWithProbeAuthority(
		securityContext,
		snapshot,
		envelope,
		requirements,
		probes,
		domainsecurity.SourceProbeEligibleForHostAuthorityV2,
	)
}

func newPublicationSnapshotProofWithProbeAuthority(
	securityContext domainsecurity.TurnSecurityContext,
	snapshot domainevidence.EvidenceReceiptRegistry,
	envelope domainevidence.FinalAnswerEnvelope,
	requirements []sourceprobeport.PublicationSourceRequirement,
	probes []domainsecurity.VerifiedSourceProbe,
	probeCanAuthorize func(domainsecurity.VerifiedSourceProbe) bool,
) (domainevidence.PublicationSnapshotProof, error) {
	if !publicationSnapshotProbesMatchWithAuthority(
		securityContext,
		requirements,
		probes,
		probeCanAuthorize,
	) {
		return domainevidence.PublicationSnapshotProof{}, errors.New("publication snapshot proof is mismatched")
	}
	head, err := domainevidence.NewEvidenceRegistryHead(snapshot)
	if err != nil {
		return domainevidence.PublicationSnapshotProof{}, err
	}
	byServer := make(map[string]domainsecurity.VerifiedSourceProbe, len(probes))
	checkedAt := time.Now().UTC()
	for _, probe := range probes {
		byServer[probe.ServerID] = probe
		if probeTime, parseErr := time.Parse(time.RFC3339Nano, probe.CheckedAt); parseErr == nil && probeTime.After(checkedAt) {
			checkedAt = probeTime
		}
	}
	sources := make([]domainevidence.PublicationSourceSnapshot, 0, len(requirements))
	for _, requirement := range requirements {
		probe := byServer[requirement.ServerID]
		sources = append(sources, domainevidence.PublicationSourceSnapshot{
			ReceiptID: requirement.ReceiptID, ServerID: requirement.ServerID, ServerIdentity: probe.ServerIdentity,
			ServerVersion: requirement.ServerVersion, ConnectionEpoch: probe.ConnectionEpoch,
			ToolName: requirement.ToolName, DatasetSnapshotID: probe.DatasetSnapshotID, CatalogFingerprint: probe.CatalogFingerprint,
			SpecFingerprint: probe.SpecFingerprint, ProbeDigest: probe.ProbeDigest, CheckedAt: probe.CheckedAt,
		})
	}
	return domainevidence.NewPublicationSnapshotProof(domainevidence.PublicationSnapshotProofInput{
		Context: securityContext, RegistryHead: head, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs,
		Sources: sources, CheckedAt: checkedAt,
	})
}

func publicationSnapshotRequirements(snapshot domainevidence.EvidenceReceiptRegistry, securityContext domainsecurity.TurnSecurityContext, envelope domainevidence.FinalAnswerEnvelope) ([]sourceprobeport.PublicationSourceRequirement, error) {
	if len(envelope.EvidenceReceiptIDs) == 0 {
		return nil, nil
	}
	requirements := make([]sourceprobeport.PublicationSourceRequirement, 0, len(envelope.EvidenceReceiptIDs))
	for _, receiptID := range envelope.EvidenceReceiptIDs {
		registered, err := domainevidence.VerifyEvidenceReceiptMembership(snapshot, securityContext, receiptID)
		if err != nil || registered.Revoked {
			return nil, errors.New("publication receipt authority is unavailable")
		}
		receipt := registered.Receipt
		serverID := toolcatalogapp.MCPToolServerID(receipt.ToolName)
		if serverID == "" {
			return nil, errors.New("publication receipt source is unknown")
		}
		requirements = append(requirements, sourceprobeport.PublicationSourceRequirement{
			ReceiptID: receipt.ReceiptID, ServerID: serverID, ServerIdentity: receipt.ServerIdentity,
			ServerVersion: receipt.ServerVersion, ConnectionEpoch: receipt.ConnectionEpoch,
			ToolName: receipt.ToolName, DatasetSnapshotID: receipt.DatasetSnapshotID,
		})
	}
	return requirements, nil
}

func publicationSnapshotProbesMatch(securityContext domainsecurity.TurnSecurityContext, requirements []sourceprobeport.PublicationSourceRequirement, probes []domainsecurity.VerifiedSourceProbe) bool {
	return publicationSnapshotProbesMatchWithAuthority(
		securityContext,
		requirements,
		probes,
		domainsecurity.SourceProbeCanAuthorizeFacts,
	)
}

func publicationSnapshotProbesMatchWithAuthority(
	securityContext domainsecurity.TurnSecurityContext,
	requirements []sourceprobeport.PublicationSourceRequirement,
	probes []domainsecurity.VerifiedSourceProbe,
	probeCanAuthorize func(domainsecurity.VerifiedSourceProbe) bool,
) bool {
	if probeCanAuthorize == nil {
		return false
	}
	expectedServers := map[string]bool{}
	for _, requirement := range requirements {
		expectedServers[requirement.ServerID] = true
	}
	byServer := map[string]domainsecurity.VerifiedSourceProbe{}
	for _, probe := range probes {
		if !probeCanAuthorize(probe) || !expectedServers[probe.ServerID] || byServer[probe.ServerID].ServerID != "" ||
			probe.ThreadID != securityContext.ThreadID || probe.TurnID != securityContext.TurnID || probe.ContextEpoch != securityContext.ContextEpoch ||
			probe.ProbeContextDigest != securityContext.ContextDigest || probe.CaseID != securityContext.CaseID || probe.CaseBindingHash != securityContext.CaseBindingHash ||
			probe.DatasetSnapshotID != securityContext.DatasetSnapshotID {
			return false
		}
		byServer[probe.ServerID] = probe
	}
	if len(byServer) != len(expectedServers) {
		return false
	}
	for _, requirement := range requirements {
		probe := byServer[requirement.ServerID]
		identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(requirement.ServerIdentity)
		if err != nil || identity.ServerID != requirement.ServerID || identity.ObservedVersion != requirement.ServerVersion ||
			identity.ConnectionEpoch != requirement.ConnectionEpoch || probe.ServerIdentity != requirement.ServerIdentity || probe.ConnectionEpoch != requirement.ConnectionEpoch ||
			probe.DatasetSnapshotID != requirement.DatasetSnapshotID {
			return false
		}
	}
	return true
}
