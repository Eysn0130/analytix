package reportpublication

import (
	"context"
	"crypto/ed25519"
	"errors"
	"reflect"
	"strings"
	"time"

	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authoritystoreport "analytix.local/runtime-go/internal/ports/authorityadvance"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

var ErrPublicationRestartUnresolved = errors.New("report publication restart effect remains unresolved")

type RestartApplyConfigV1 struct {
	InstallationID       string
	EnrollmentID         string
	Authority            finalauthorityport.Authority
	WitnessKeyID         string
	WitnessKey           []byte
	Evidence             registryport.WitnessedSnapshotReader
	HeadReader           evidenceauthorityport.FreshHeadReader
	Advances             ExactPublicationCoordinator
	Pending              RestartPendingStoreV1
	Attempts             publicationport.AttemptStore
	Receipts             publicationport.ReceiptStore
	Indexes              publicationport.IndexStore
	Intents              authoritystoreport.IntentStore
	Settlements          authoritystoreport.SettlementStore
	Bundles              evidenceauthorityport.BundleStore
	Observations         evidenceauthorityport.ObservationStore
	Contexts             RestartContextAuthorityV1
	Ledgers              publicationport.ClaimLedgerStore
	PIIProjections       publicationport.PIIProjectionStore
	Inspections          publicationport.RenderInspectionStore
	Artifacts            publicationport.ArtifactStore
	PIIAuthority         publicationport.PIIAuthorizationAuthority
	Selections           publicationport.CommitSelectionStore
	Commits              publicationport.CommitReceiptStore
	Decisions            publicationport.DeliveryDecisionStore
	GrantSettlements     restartGrantSettlementStoreV1
	StageCompletions     restartStageCompletionStoreV1
	Threads              restartThreadReaderV1
	DeliveryOutcomes     publicationport.DeliveryOutcomeStore
	AcquireContextEffect RestartContextEffectAcquirerV1
}

type RestartContextEffectAcquirerV1 func(
	context.Context,
	domainsecurity.TurnSecurityContext,
) (context.Context, func(), error)

type restartGrantSettlementStoreV1 interface {
	publicationport.ReportGrantSettlementStore
	publicationport.ReportGrantSettlementInventoryStore
}

type restartStageCompletionStoreV1 interface {
	publicationport.ReportStageCompletionStore
	publicationport.ReportStageCompletionInventoryStore
}

type RestartPendingStoreV1 interface {
	ReadReceipt(context.Context, string) (domainpendingwork.PendingWorkReceiptV1, error)
	PutDispositionIfAbsent(context.Context, domainpendingwork.PendingWorkDispositionV1) error
	ReadDisposition(context.Context, string) (domainpendingwork.PendingWorkDispositionV1, error)
}

// RestartContextAuthorityV1 returns the exact committed context only while it
// is still the current case/epoch authority. Implementations must reject a
// stale case switch rather than merely finding historical context bytes.
type RestartContextAuthorityV1 interface {
	ResolveCurrent(context.Context, string, string, string) (domainsecurity.TurnSecurityContext, error)
}

type RestartApplyResultV1 struct {
	DeferredBeforeWitness uint64
	AbortedBeforeWitness  uint64
	Superseded            uint64
	RecoveredSettlements  uint64
	IssuedCommits         uint64
	Delivered             uint64
	DeliveryRejected      uint64
	DeliveryBlocked       uint64
}

// ApplyRestartV1 resumes only effects whose exact immutable attempt graph was
// accepted by PreflightRestartV1. Pre-witness cuts are deliberately not
// replayed. An unsettled durable intent is resolved by mutation identity; an
// indeterminate witness answer blocks activation. Delivery is idempotent only
// after exact projection readback of the same signed commit receipt.
func ApplyRestartV1(ctx context.Context, plan RestartPlanV1, config RestartApplyConfigV1) (RestartApplyResultV1, error) {
	if err := validateRestartApplyConfigV1(config); err != nil || ctx == nil {
		return RestartApplyResultV1{}, ErrPublicationUnavailable
	}
	snapshot, err := snapshotRestartPlanV1(plan)
	if err != nil {
		return RestartApplyResultV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return RestartApplyResultV1{}, err
	}
	result := RestartApplyResultV1{}
	for _, sealedEntry := range snapshot.Attempts {
		entry := sealedEntry
		applyCtx := ctx
		release := func() {}
		effectContext, needsEffect, effectContextErr := restartEntryEffectContextV1(ctx, entry, config)
		if effectContextErr != nil {
			return result, effectContextErr
		}
		if needsEffect {
			var acquireErr error
			applyCtx, release, acquireErr = config.AcquireContextEffect(ctx, effectContext)
			if acquireErr != nil || applyCtx == nil || release == nil {
				if acquireErr == nil {
					acquireErr = errors.New("report publication restart context effect lease is invalid")
				}
				return result, errors.Join(ErrPublicationRestartUnresolved, acquireErr)
			}
		}
		entryResult, applyErr := func() (RestartApplyResultV1, error) {
			defer release()
			if membershipErr := validateRestartEntryMembershipV1(applyCtx, entry, config); membershipErr != nil {
				return RestartApplyResultV1{}, membershipErr
			}
			refreshed, refreshErr := refreshRestartEntrySuffixV1(applyCtx, entry, config)
			if refreshErr != nil {
				return RestartApplyResultV1{}, refreshErr
			}
			return applyRestartEntryV1(applyCtx, refreshed, config)
		}()
		mergeRestartApplyResultV1(&result, entryResult)
		if applyErr != nil {
			return result, applyErr
		}
	}
	return result, nil
}

func applyRestartEntryV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) (RestartApplyResultV1, error) {
	result := RestartApplyResultV1{}
	if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
		return result, err
	}
	switch entry.State {
	case RestartAttemptReservedV1, RestartAttemptCandidateDurableV1, RestartAttemptMaterialsDurableV1:
		result.DeferredBeforeWitness++
		return result, nil
	case RestartAttemptAbortedV1:
		if entry.Disposition == nil || entry.Intent != nil || entry.Settlement != nil || entry.Selection != nil || entry.Commit != nil || entry.Decision != nil || entry.GrantSettlement != nil || entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return result, ErrPublicationIntegrity
		}
		result.AbortedBeforeWitness++
		return result, nil
	case RestartAttemptSupersededSettlementV1:
		result.Superseded++
		return result, nil
	case RestartAttemptIntentDurableV1:
		if entry.Intent == nil {
			return result, ErrPublicationIntegrity
		}
		recovered, err := config.Advances.RecoverExact(ctx, entry.Attempt.AuthorityAdvanceMutationID)
		if err != nil {
			return result, errors.Join(ErrPublicationRestartUnresolved, err)
		}
		if !reflect.DeepEqual(recovered.Intent, *entry.Intent) || recovered.Settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 ||
			domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(recovered.Settlement, recovered.Intent, nil) != nil {
			return result, ErrPublicationIntegrity
		}
		entry.Settlement = &recovered.Settlement
		entry.State = RestartAttemptCommittedSettlementV1
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		result.RecoveredSettlements++
		fallthrough
	case RestartAttemptCommittedSettlementV1, RestartAttemptCommitSelectionV1:
		if err := validateRestartCandidateAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		commit, issued, err := recoverPublicationCommitV1(ctx, entry, config)
		if err != nil {
			return result, err
		}
		if issued {
			result.IssuedCommits++
		}
		selection, resolveErr := config.Selections.Resolve(
			ctx,
			domainpublication.PublicationCommitSelectionIDV1(
				config.InstallationID, config.EnrollmentID, entry.Attempt.AttemptID,
			),
		)
		if resolveErr != nil {
			return result, restartMembershipReadErrorV1(resolveErr, publicationport.ErrNotFound)
		}
		entry.Selection = &selection
		entry.Commit = &commit
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateRestartCandidateAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateCommitStillWitnessedV1(ctx, commit, config); err != nil {
			return result, err
		}
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		return result, errors.Join(
			ErrPublicationRestartUnresolved,
			errors.New("report commit lacks a decision-bound settled stage and cannot be delivered"),
		)
	case RestartAttemptCommitReceiptV1:
		if entry.Commit == nil || entry.Selection == nil || entry.Settlement == nil || entry.Candidate == nil || entry.Index == nil ||
			domainpublication.ValidatePublicationCommitReceiptMaterialsV1(*entry.Commit, *entry.Candidate, *entry.Index) != nil {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartCandidateAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateCommitAuthorityV1(ctx, *entry.Commit, config); err != nil {
			return result, err
		}
		if err := validateCommitExactWitnessV1(ctx, *entry.Commit, *entry.Candidate, *entry.Index, config); err != nil {
			return result, err
		}
		if err := validateCommitSelectionExactV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateCommitStillWitnessedV1(ctx, *entry.Commit, config); err != nil {
			return result, err
		}
		if err := validateRestartCandidateAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		return result, errors.Join(
			ErrPublicationRestartUnresolved,
			errors.New("report commit lacks a decision-bound settled stage and cannot be delivered"),
		)
	case RestartAttemptDeliveryDecisionV1:
		if entry.Decision == nil || entry.Commit == nil || entry.Selection == nil || entry.Settlement == nil ||
			entry.Candidate == nil || entry.Index == nil {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartCandidateAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateCommitAuthorityV1(ctx, *entry.Commit, config); err != nil {
			return result, err
		}
		if err := validateCommitExactWitnessV1(ctx, *entry.Commit, *entry.Candidate, *entry.Index, config); err != nil {
			return result, err
		}
		if err := validateCommitSelectionExactV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateCommitStillWitnessedV1(ctx, *entry.Commit, config); err != nil {
			return result, err
		}
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		return result, errors.Join(
			ErrPublicationRestartUnresolved,
			errors.New("report decision lacks a decision-bound settled grant and completed stage"),
		)
	case RestartAttemptGrantSettlementV1:
		if entry.GrantSettlement == nil || entry.Decision == nil || entry.Commit == nil || entry.Selection == nil ||
			entry.Settlement == nil || entry.Candidate == nil || entry.Index == nil || entry.Disposition != nil ||
			entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartDeliveryAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		settledAt, err := time.Parse(time.RFC3339Nano, entry.GrantSettlement.SettledAt)
		if err != nil || settledAt.IsZero() || settledAt.UTC().Format(time.RFC3339Nano) != entry.GrantSettlement.SettledAt {
			return result, ErrPublicationIntegrity
		}
		disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
			entry.Stage, domainpendingwork.StatusCompleted, "report_stage_completed", settledAt,
			config.Authority.KeyID(), config.Authority.PublicKey(),
			func(message []byte) ([]byte, error) { return config.Authority.Sign(ctx, message) },
		)
		if err != nil {
			return result, errors.Join(ErrPublicationRestartUnresolved, err)
		}
		if err := reconcileExactStageDispositionV1(ctx, config.Pending, entry.Stage, disposition); err != nil {
			return result, err
		}
		entry.Disposition = &disposition
		entry.State = RestartAttemptStageDispositionV1
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		fallthrough
	case RestartAttemptStageDispositionV1:
		if entry.Disposition == nil || entry.GrantSettlement == nil || entry.Decision == nil || entry.Commit == nil ||
			entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartDeliveryAuthorityV1(ctx, entry, config); err != nil {
			return result, err
		}
		completion, err := domainpublication.NewReportStageCompletionV1(
			domainpublication.ReportStageCompletionInputV1{
				Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
				StageReceipt: entry.Stage, StageDisposition: *entry.Disposition,
				AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
			},
			func(message []byte) ([]byte, error) { return config.Authority.Sign(ctx, message) },
		)
		if err != nil {
			return result, errors.Join(ErrPublicationRestartUnresolved, err)
		}
		if err := reconcileExactStageCompletionV1(ctx, config.StageCompletions, completion); err != nil {
			return result, err
		}
		entry.StageCompletion = &completion
		entry.State = RestartAttemptStageCompletionV1
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		fallthrough
	case RestartAttemptStageCompletionV1:
		if entry.Disposition == nil || entry.GrantSettlement == nil || entry.Decision == nil || entry.Commit == nil ||
			entry.StageCompletion == nil || entry.DeliveryOutcome != nil {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartHistoricalDeliveryGraphV1(ctx, entry, config); err != nil {
			return result, err
		}
		if err := validateRestartCurrentDeliveryContextV1(ctx, entry, config); err != nil {
			return result, err
		}
		outcome, err := decideRestartDeliveryWithinWitnessedSnapshotV1(ctx, entry, config)
		if err != nil {
			return result, err
		}
		entry.DeliveryOutcome = &outcome
		if outcome.Kind == domainpublication.ReportDeliveryOutcomeProjectedV1 {
			entry.State = RestartAttemptDeliveryProjectionV1
			result.Delivered++
		} else {
			entry.State = RestartAttemptDeliveryRejectionV1
			result.DeliveryRejected++
		}
		if err := validateRestartEntryMembershipV1(ctx, entry, config); err != nil {
			return result, err
		}
		return result, nil
	case RestartAttemptDeliveryProjectionV1:
		if entry.Disposition == nil || entry.GrantSettlement == nil || entry.Decision == nil || entry.Commit == nil ||
			entry.StageCompletion == nil || entry.DeliveryOutcome == nil ||
			entry.DeliveryOutcome.Kind != domainpublication.ReportDeliveryOutcomeProjectedV1 {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartHistoricalDeliveryGraphV1(ctx, entry, config); err != nil {
			return result, err
		}
		result.Delivered++
		return result, nil
	case RestartAttemptDeliveryRejectionV1:
		if entry.Disposition == nil || entry.GrantSettlement == nil || entry.Decision == nil || entry.Commit == nil ||
			entry.StageCompletion == nil || entry.DeliveryOutcome == nil ||
			entry.DeliveryOutcome.Kind != domainpublication.ReportDeliveryOutcomeRejectedV1 {
			return result, ErrPublicationIntegrity
		}
		if err := validateRestartHistoricalDeliveryGraphV1(ctx, entry, config); err != nil {
			return result, err
		}
		result.DeliveryRejected++
		return result, nil
	case RestartAttemptDeliveryBlockedV1:
		if entry.Disposition == nil || entry.Disposition.Status == domainpendingwork.StatusCompleted || entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return result, ErrPublicationIntegrity
		}
		result.DeliveryBlocked++
		return result, nil
	default:
		return result, ErrPublicationIntegrity
	}
}

// decideRestartDeliveryWithinWitnessedSnapshotV1 gives projection or
// evidence-based rejection one shared linearization point with evidence
// issuance/revocation. The caller already holds the case-context effect lease;
// the witnessed snapshot remains locked through the single outcome CAS and
// exact readback. An opposite outcome that already won the stable key is
// returned as the durable authority rather than creating a second terminal.
func decideRestartDeliveryWithinWitnessedSnapshotV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	if entry.Candidate == nil || entry.Commit == nil || entry.Decision == nil || entry.GrantSettlement == nil || entry.Disposition == nil ||
		entry.StageCompletion == nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, ErrPublicationIntegrity
	}
	securityContext := entry.Decision.Context
	settled := domainpublication.ReportDeliveryOutcomeV1{}
	err := withWitnessedSnapshotAuthorityV2(ctx, config.Evidence, securityContext, func(
		snapshot registryport.WitnessedSnapshot,
		capability registryport.WitnessedSnapshotCapability,
	) error {
		if snapshot.Context != securityContext || snapshot.Registry.ContextDigest != securityContext.ContextDigest ||
			domainevidence.ValidateEvidenceRegistryAuthorityIndexWitnessRootV2(
				snapshot.RootIndex, snapshot.Head.Bundle.EvidenceRegistryIndexDigest, snapshot.Head.Bundle.EvidenceRegistryCount,
			) != nil || validateFreshPublicationHeadV1(snapshot.Head, config) != nil {
			return ErrPublicationIntegrity
		}
		committed, resolveErr := config.Bundles.Resolve(ctx, entry.Commit.CommittedEvidenceBundleDigest)
		if resolveErr != nil || committed.RecordDigest != entry.Commit.CommittedEvidenceBundleDigest {
			return ErrPublicationIntegrity
		}
		ancestor, ancestorErr := publicationBundleIsAncestorV1(ctx, snapshot.Head.Bundle, committed, config)
		if ancestorErr != nil || !ancestor {
			return ErrPublicationIntegrity
		}
		ledger, ledgerErr := config.Ledgers.Resolve(ctx, entry.Candidate.ClaimLedgerDigest)
		if ledgerErr != nil {
			return restartMembershipReadErrorV1(ledgerErr, publicationport.ErrNotFound)
		}
		piiProjection, projectionErr := config.PIIProjections.Resolve(ctx, entry.Candidate.PIIProjectionDigest)
		if projectionErr != nil {
			return restartMembershipReadErrorV1(projectionErr, publicationport.ErrNotFound)
		}
		artifact, artifactErr := config.Artifacts.ResolveExact(ctx, entry.Candidate.TargetIdentityDigest)
		if artifactErr != nil {
			return restartMembershipReadErrorV1(artifactErr, publicationport.ErrNotFound)
		}
		allowControlled := piiProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull
		candidate := domainpublication.ReportDeliveryOutcomeV1{}
		if validateLedgerSnapshotV1(ctx, snapshot, entry.Candidate.ReportVariant, ledger, allowControlled) != nil {
			rejection, createErr := domainpublication.NewReportDeliveryRejectionV1(
				domainpublication.ReportDeliveryRejectionInputV1{
					Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
					StageReceipt: entry.Stage, StageDisposition: *entry.Disposition,
					StageCompletion:               *entry.StageCompletion,
					ReasonCode:                    domainpublication.ReportDeliveryRejectionEvidenceChangedV1,
					WitnessObservationDigest:      snapshot.Head.Observation.ObservationDigest,
					EvidenceAuthorityBundleDigest: snapshot.Head.Bundle.RecordDigest,
					EvidenceRegistryIndexDigest:   snapshot.RootIndex.IndexDigest,
					EvidenceRegistrySequence:      snapshot.Registry.Sequence,
					EvidenceRegistryStateDigest:   snapshot.Registry.StateDigest,
					AuthorityKeyID:                config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
				},
				func(message []byte) ([]byte, error) { return config.Authority.Sign(ctx, message) },
			)
			if createErr != nil {
				return createErr
			}
			candidate, createErr = domainpublication.RejectedReportDeliveryOutcomeV1(rejection)
			if createErr != nil {
				return createErr
			}
		} else {
			if allowControlled && validateRestartControlledArtifactPIIV1(
				ctx, securityContext, *entry.Candidate, ledger, piiProjection, artifact, config, &snapshot, capability,
			) != nil {
				// The current PII/approval contract conflates permanent denial
				// with not-found, I/O, corruption, and unavailable authority.
				// It therefore cannot authorize a durable rejection yet.
				return ErrPublicationRestartUnresolved
			}
			projection, createErr := domainpublication.NewReportDeliveryProjectionV1(
				domainpublication.ReportDeliveryProjectionInputV1{
					Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
					StageReceipt: entry.Stage, StageDisposition: *entry.Disposition,
					StageCompletion: *entry.StageCompletion,
					AuthorityKeyID:  config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
				},
				func(message []byte) ([]byte, error) { return config.Authority.Sign(ctx, message) },
			)
			if createErr != nil {
				return createErr
			}
			candidate, createErr = domainpublication.ProjectedReportDeliveryOutcomeV1(projection)
			if createErr != nil {
				return createErr
			}
		}
		winner, reconcileErr := reconcileExactDeliveryOutcomeV1(
			ctx, config.DeliveryOutcomes, *entry.StageCompletion, candidate,
		)
		if reconcileErr != nil {
			return reconcileErr
		}
		settled = winner
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrPublicationIntegrity) || errors.Is(err, ErrPublicationChanged) ||
			errors.Is(err, ErrPublicationRestartUnresolved) {
			return domainpublication.ReportDeliveryOutcomeV1{}, err
		}
		return domainpublication.ReportDeliveryOutcomeV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	if domainpublication.ValidateReportDeliveryOutcomeV1(settled) != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, ErrPublicationIntegrity
	}
	return settled, nil
}

func restartEntryEffectContextV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) (domainsecurity.TurnSecurityContext, bool, error) {
	switch entry.State {
	case RestartAttemptDeliveryProjectionV1, RestartAttemptDeliveryRejectionV1,
		RestartAttemptDeliveryBlockedV1, RestartAttemptAbortedV1:
		// Durable historical terminals are private audit state. Startup may
		// validate their immutable graph without requiring an old case/epoch
		// to remain current or reacquiring authority to expose the artifact.
		return domainsecurity.TurnSecurityContext{}, false, nil
	}
	if entry.Decision != nil {
		return entry.Decision.Context, true, nil
	}
	switch entry.State {
	case RestartAttemptIntentDurableV1, RestartAttemptCommittedSettlementV1,
		RestartAttemptCommitSelectionV1, RestartAttemptCommitReceiptV1:
		if entry.Candidate == nil {
			return domainsecurity.TurnSecurityContext{}, false, ErrPublicationIntegrity
		}
		candidate := entry.Candidate
		securityContext, err := config.Contexts.ResolveCurrent(
			ctx, candidate.ThreadID, candidate.TurnID, candidate.ContextDigest,
		)
		if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
			securityContext.ThreadID != candidate.ThreadID || securityContext.TurnID != candidate.TurnID ||
			securityContext.ContextDigest != candidate.ContextDigest || securityContext.CaseID != candidate.CaseID ||
			securityContext.CaseBindingHash != candidate.CaseBindingHash || securityContext.ContextEpoch != candidate.ContextEpoch ||
			securityContext.DatasetSnapshotID != candidate.DatasetSnapshotID || securityContext.SourceManifestHash != candidate.SourceManifestHash {
			return domainsecurity.TurnSecurityContext{}, false, errors.Join(ErrPublicationRestartUnresolved, err)
		}
		return securityContext, true, nil
	default:
		return domainsecurity.TurnSecurityContext{}, false, nil
	}
}

func mergeRestartApplyResultV1(target *RestartApplyResultV1, add RestartApplyResultV1) {
	if target == nil {
		return
	}
	target.DeferredBeforeWitness += add.DeferredBeforeWitness
	target.AbortedBeforeWitness += add.AbortedBeforeWitness
	target.Superseded += add.Superseded
	target.RecoveredSettlements += add.RecoveredSettlements
	target.IssuedCommits += add.IssuedCommits
	target.Delivered += add.Delivered
	target.DeliveryRejected += add.DeliveryRejected
	target.DeliveryBlocked += add.DeliveryBlocked
}

// refreshRestartEntrySuffixV1 preserves the sealed preflight prefix while
// admitting only exact durable successors created by an earlier or concurrent
// application of the same recovery graph. Missing suffixes remain valid crash
// cuts; the exact shared delivery-outcome winner is authoritative even when
// it is the opposite kind proposed by a concurrent recovery.
func refreshRestartEntrySuffixV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) (RestartAttemptV1, error) {
	if entry.Decision == nil || entry.GrantSettlement == nil {
		return entry, nil
	}
	disposition, err := config.Pending.ReadDisposition(ctx, entry.Stage.WorkID)
	if err != nil {
		if errors.Is(err, pendingworkstoreport.ErrNotFound) {
			if entry.Disposition != nil {
				return RestartAttemptV1{}, ErrPublicationIntegrity
			}
			return entry, nil
		}
		return RestartAttemptV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	if entry.Disposition != nil && !reflect.DeepEqual(disposition, *entry.Disposition) ||
		domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, entry.Stage) != nil {
		return RestartAttemptV1{}, ErrPublicationIntegrity
	}
	entry.Disposition = &disposition
	switch disposition.Status {
	case domainpendingwork.StatusCompleted:
		if validateRestartSuccessfulReportResultV1(ctx, *entry.GrantSettlement, *entry.Decision, config.Threads) != nil {
			return RestartAttemptV1{}, ErrPublicationIntegrity
		}
		entry.State = RestartAttemptStageDispositionV1
	case domainpendingwork.StatusFailed, domainpendingwork.StatusCancelled, domainpendingwork.StatusExpired,
		domainpendingwork.StatusRejected, domainpendingwork.StatusRestartInvalid, domainpendingwork.StatusStaleContext,
		domainpendingwork.StatusOutcomeUnknown:
		if entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return RestartAttemptV1{}, ErrPublicationIntegrity
		}
		entry.State = RestartAttemptDeliveryBlockedV1
		return entry, nil
	default:
		return RestartAttemptV1{}, ErrPublicationIntegrity
	}

	completionID := domainpublication.ReportStageCompletionIDV1(
		config.InstallationID, config.EnrollmentID, entry.Decision.DecisionID,
	)
	completion, err := config.StageCompletions.Resolve(ctx, completionID)
	if err != nil {
		if errors.Is(err, publicationport.ErrNotFound) {
			if entry.StageCompletion != nil {
				return RestartAttemptV1{}, ErrPublicationIntegrity
			}
			return entry, nil
		}
		return RestartAttemptV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	completionInput := domainpublication.ReportStageCompletionInputV1{
		Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
		StageReceipt: entry.Stage, StageDisposition: disposition,
		AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
	}
	if entry.StageCompletion != nil && !reflect.DeepEqual(completion, *entry.StageCompletion) ||
		domainpublication.ValidateReportStageCompletionGraphV1(completion, completionInput) != nil {
		return RestartAttemptV1{}, ErrPublicationIntegrity
	}
	entry.StageCompletion = &completion
	entry.State = RestartAttemptStageCompletionV1

	deliveryID := domainpublication.ReportDeliveryOutcomeIDV1(
		config.InstallationID, config.EnrollmentID, completion.CompletionID,
	)
	outcome, err := config.DeliveryOutcomes.ResolveOutcome(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, publicationport.ErrNotFound) {
			if entry.DeliveryOutcome != nil {
				return RestartAttemptV1{}, ErrPublicationIntegrity
			}
			return entry, nil
		}
		return RestartAttemptV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	if entry.DeliveryOutcome != nil && !reflect.DeepEqual(outcome, *entry.DeliveryOutcome) ||
		validateRestartDeliveryOutcomeGraphV1(
			outcome, *entry.Decision, *entry.GrantSettlement, entry.Stage, disposition, completion,
			config.Authority.KeyID(), config.Authority.PublicKey(),
		) != nil {
		return RestartAttemptV1{}, ErrPublicationIntegrity
	}
	entry.DeliveryOutcome = &outcome
	if outcome.Kind == domainpublication.ReportDeliveryOutcomeProjectedV1 {
		entry.State = RestartAttemptDeliveryProjectionV1
	} else {
		entry.State = RestartAttemptDeliveryRejectionV1
	}
	return entry, nil
}

func validateRestartDeliveryAuthorityV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) error {
	if entry.Candidate == nil || entry.Index == nil || entry.Settlement == nil || entry.Selection == nil ||
		entry.Commit == nil || entry.Decision == nil || entry.GrantSettlement == nil {
		return ErrPublicationIntegrity
	}
	if err := validateRestartCandidateAuthorityV1(ctx, entry, config); err != nil {
		return err
	}
	if err := validateCommitAuthorityV1(ctx, *entry.Commit, config); err != nil {
		return err
	}
	if err := validateCommitExactWitnessV1(ctx, *entry.Commit, *entry.Candidate, *entry.Index, config); err != nil {
		return err
	}
	if err := validateCommitSelectionExactV1(ctx, entry, config); err != nil {
		return err
	}
	if err := validateCommitStillWitnessedV1(ctx, *entry.Commit, config); err != nil {
		return err
	}
	if err := validateRestartSuccessfulReportResultV1(ctx, *entry.GrantSettlement, *entry.Decision, config.Threads); err != nil {
		return err
	}
	return validateRestartEntryMembershipV1(ctx, entry, config)
}

// validateRestartHistoricalDeliveryGraphV1 validates a durable terminal as
// private audit history. It deliberately does not require the old case/epoch,
// evidence support, or controlled-PII grant to remain current and therefore
// cannot be used as authority to expose an artifact now.
func validateRestartHistoricalDeliveryGraphV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) error {
	if entry.Candidate == nil || entry.Index == nil || entry.Settlement == nil || entry.Selection == nil ||
		entry.Commit == nil || entry.Decision == nil || entry.GrantSettlement == nil || entry.StageCompletion == nil {
		return ErrPublicationIntegrity
	}
	if err := validateRestartCandidateDurableMaterialsV1(ctx, entry, config); err != nil {
		return err
	}
	if err := validateCommitAuthorityV1(ctx, *entry.Commit, config); err != nil {
		return err
	}
	if err := validateCommitExactWitnessV1(ctx, *entry.Commit, *entry.Candidate, *entry.Index, config); err != nil {
		return err
	}
	if err := validateCommitSelectionExactV1(ctx, entry, config); err != nil {
		return err
	}
	if err := validateRestartSuccessfulReportResultV1(ctx, *entry.GrantSettlement, *entry.Decision, config.Threads); err != nil {
		return err
	}
	return validateRestartEntryMembershipV1(ctx, entry, config)
}

func validateRestartCurrentDeliveryContextV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) error {
	if entry.Candidate == nil || entry.Decision == nil || config.Contexts == nil {
		return ErrPublicationIntegrity
	}
	candidate := *entry.Candidate
	securityContext, err := config.Contexts.ResolveCurrent(ctx, candidate.ThreadID, candidate.TurnID, candidate.ContextDigest)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		securityContext != entry.Decision.Context || securityContext.ThreadID != candidate.ThreadID ||
		securityContext.TurnID != candidate.TurnID || securityContext.ContextDigest != candidate.ContextDigest ||
		securityContext.CaseID != candidate.CaseID || securityContext.CaseBindingHash != candidate.CaseBindingHash ||
		securityContext.ContextEpoch != candidate.ContextEpoch || securityContext.DatasetSnapshotID != candidate.DatasetSnapshotID ||
		securityContext.SourceManifestHash != candidate.SourceManifestHash {
		return errors.Join(ErrPublicationRestartUnresolved, err)
	}
	return nil
}

func reconcileExactStageDispositionV1(
	ctx context.Context,
	pending RestartPendingStoreV1,
	receipt domainpendingwork.PendingWorkReceiptV1,
	disposition domainpendingwork.PendingWorkDispositionV1,
) error {
	existing, err := pending.ReadDisposition(ctx, receipt.WorkID)
	if err == nil {
		if !reflect.DeepEqual(existing, disposition) ||
			domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(existing, receipt) != nil {
			return ErrPublicationIntegrity
		}
		return nil
	}
	if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return errors.Join(ErrPublicationRestartUnresolved, err)
	}
	putErr := pending.PutDispositionIfAbsent(ctx, disposition)
	readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
	defer cancel()
	existing, readErr := pending.ReadDisposition(readbackCtx, receipt.WorkID)
	if readErr != nil {
		return errors.Join(ErrPublicationRestartUnresolved, putErr, readErr)
	}
	if !reflect.DeepEqual(existing, disposition) ||
		domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(existing, receipt) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func reconcileExactStageCompletionV1(
	ctx context.Context,
	store restartStageCompletionStoreV1,
	completion domainpublication.ReportStageCompletionV1,
) error {
	existing, err := store.Resolve(ctx, completion.CompletionID)
	if err == nil {
		if !reflect.DeepEqual(existing, completion) || domainpublication.ValidateReportStageCompletionV1(existing) != nil {
			return ErrPublicationIntegrity
		}
		return nil
	}
	if !errors.Is(err, publicationport.ErrNotFound) {
		return errors.Join(ErrPublicationRestartUnresolved, err)
	}
	_, createErr := store.CreateExclusive(ctx, completion)
	readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
	defer cancel()
	existing, readErr := store.Resolve(readbackCtx, completion.CompletionID)
	if readErr != nil {
		return errors.Join(ErrPublicationRestartUnresolved, createErr, readErr)
	}
	if !reflect.DeepEqual(existing, completion) || domainpublication.ValidateReportStageCompletionV1(existing) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

// validateRestartEntryMembershipV1 proves that the sealed restart plan still
// names exact members of every durable host registry. The plan seal prevents
// caller-owned memory mutation; it is not a substitute for registry
// membership after preflight or across a crash/restart boundary.
func validateRestartEntryMembershipV1(ctx context.Context, entry RestartAttemptV1, config RestartApplyConfigV1) error {
	attempt, err := config.Attempts.Resolve(ctx, entry.Attempt.AttemptID)
	if err != nil {
		return restartMembershipReadErrorV1(err, publicationport.ErrNotFound)
	}
	if !reflect.DeepEqual(attempt, entry.Attempt) {
		return ErrPublicationIntegrity
	}
	stage, err := config.Pending.ReadReceipt(ctx, entry.Stage.WorkID)
	if err != nil {
		return restartMembershipReadErrorV1(err, pendingworkstoreport.ErrNotFound)
	}
	if !reflect.DeepEqual(stage, entry.Stage) {
		return ErrPublicationIntegrity
	}
	if entry.Disposition != nil {
		disposition, dispositionErr := config.Pending.ReadDisposition(ctx, entry.Stage.WorkID)
		if dispositionErr != nil {
			return restartMembershipReadErrorV1(dispositionErr, pendingworkstoreport.ErrNotFound)
		}
		if !reflect.DeepEqual(disposition, *entry.Disposition) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Candidate != nil {
		candidate, candidateErr := config.Receipts.Resolve(ctx, entry.Candidate.RecordDigest)
		if candidateErr != nil {
			return restartMembershipReadErrorV1(candidateErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(candidate, *entry.Candidate) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Index != nil {
		index, indexErr := config.Indexes.Resolve(ctx, entry.Index.IndexDigest)
		if indexErr != nil {
			return restartMembershipReadErrorV1(indexErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(index, *entry.Index) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Intent != nil {
		intent, intentErr := config.Intents.ResolveIntent(ctx, entry.Intent.MutationID)
		if intentErr != nil {
			return restartMembershipReadErrorV1(intentErr, authoritystoreport.ErrNotFound)
		}
		if !reflect.DeepEqual(intent, *entry.Intent) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Settlement != nil {
		settlement, settlementErr := config.Settlements.ResolveSettlement(ctx, entry.Settlement.MutationID)
		if settlementErr != nil {
			return restartMembershipReadErrorV1(settlementErr, authoritystoreport.ErrNotFound)
		}
		if !reflect.DeepEqual(settlement, *entry.Settlement) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Selection != nil {
		selection, selectionErr := config.Selections.Resolve(ctx, entry.Selection.SelectionID)
		if selectionErr != nil {
			return restartMembershipReadErrorV1(selectionErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(selection, *entry.Selection) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Commit != nil {
		commit, commitErr := config.Commits.Resolve(ctx, entry.Commit.RecordDigest)
		if commitErr != nil {
			return restartMembershipReadErrorV1(commitErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(commit, *entry.Commit) {
			return ErrPublicationIntegrity
		}
	}
	if entry.Decision != nil {
		decision, decisionErr := config.Decisions.Resolve(ctx, entry.Decision.DecisionID)
		if decisionErr != nil {
			return restartMembershipReadErrorV1(decisionErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(decision, *entry.Decision) || validateRestartDecisionDurableGraphV1(ctx, entry, config) != nil {
			return ErrPublicationIntegrity
		}
	} else {
		decisionID := domainpublication.ReportDeliveryDecisionIDV1(
			config.InstallationID, config.EnrollmentID, entry.Attempt.AttemptID,
		)
		if _, decisionErr := config.Decisions.Resolve(ctx, decisionID); decisionErr == nil {
			return ErrPublicationRestartUnresolved
		} else if !errors.Is(decisionErr, publicationport.ErrNotFound) {
			return errors.Join(ErrPublicationRestartUnresolved, decisionErr)
		}
	}
	if entry.GrantSettlement != nil {
		settlement, settlementErr := config.GrantSettlements.Resolve(ctx, entry.GrantSettlement.SettlementID)
		if settlementErr != nil {
			return restartMembershipReadErrorV1(settlementErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(settlement, *entry.GrantSettlement) || entry.Decision == nil ||
			validateRestartGrantSettlementFromThreadV1(ctx, settlement, *entry.Decision, config.Threads) != nil {
			return ErrPublicationIntegrity
		}
	} else if entry.Decision != nil {
		found := false
		if err := config.GrantSettlements.VisitGrantSettlements(ctx, func(settlement domainpublication.ReportGrantSettlementV1) error {
			if settlement.DecisionID == entry.Decision.DecisionID {
				found = true
			}
			return nil
		}); err != nil {
			return errors.Join(ErrPublicationRestartUnresolved, err)
		}
		if found {
			return ErrPublicationRestartUnresolved
		}
	}
	if entry.StageCompletion != nil {
		completion, completionErr := config.StageCompletions.Resolve(ctx, entry.StageCompletion.CompletionID)
		if completionErr != nil {
			return restartMembershipReadErrorV1(completionErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(completion, *entry.StageCompletion) || entry.Decision == nil || entry.GrantSettlement == nil || entry.Disposition == nil ||
			domainpublication.ValidateReportStageCompletionGraphV1(
				completion,
				domainpublication.ReportStageCompletionInputV1{
					Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
					StageReceipt: entry.Stage, StageDisposition: *entry.Disposition,
					AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
				},
			) != nil || validateRestartSuccessfulReportResultV1(ctx, *entry.GrantSettlement, *entry.Decision, config.Threads) != nil {
			return ErrPublicationIntegrity
		}
	}
	if entry.DeliveryOutcome != nil {
		if entry.StageCompletion == nil || entry.Decision == nil || entry.GrantSettlement == nil || entry.Disposition == nil {
			return ErrPublicationIntegrity
		}
		deliveryID := domainpublication.ReportDeliveryOutcomeID(*entry.DeliveryOutcome)
		outcome, outcomeErr := config.DeliveryOutcomes.ResolveOutcome(ctx, deliveryID)
		if outcomeErr != nil {
			return restartMembershipReadErrorV1(outcomeErr, publicationport.ErrNotFound)
		}
		if !reflect.DeepEqual(outcome, *entry.DeliveryOutcome) ||
			validateRestartDeliveryOutcomeGraphV1(
				outcome, *entry.Decision, *entry.GrantSettlement, entry.Stage, *entry.Disposition,
				*entry.StageCompletion, config.Authority.KeyID(), config.Authority.PublicKey(),
			) != nil {
			return ErrPublicationIntegrity
		}
	}
	return nil
}

func validateRestartDecisionDurableGraphV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) error {
	if entry.Decision == nil || entry.Candidate == nil || entry.Index == nil || entry.Selection == nil || entry.Commit == nil {
		return ErrPublicationIntegrity
	}
	ledger, ledgerErr := config.Ledgers.Resolve(ctx, entry.Decision.ClaimLedgerDigest)
	projection, projectionErr := config.PIIProjections.Resolve(ctx, entry.Decision.PIIProjectionDigest)
	inspection, inspectionErr := config.Inspections.Resolve(ctx, entry.Decision.RenderInspectionDigest)
	if ledgerErr != nil || projectionErr != nil || inspectionErr != nil ||
		domainpublication.ValidateReportDeliveryDecisionDurableGraphV1(
			*entry.Decision, entry.Attempt, *entry.Candidate, *entry.Index, *entry.Selection, *entry.Commit,
			ledger, projection, inspection,
		) != nil {
		return ErrPublicationIntegrity
	}
	keyID, publicKey, signature, err := domainpublication.ReportDeliveryDecisionAuthorityMaterialV1(*entry.Decision)
	if err != nil || config.Authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpublication.ReportDeliveryDecisionSigningBytesV1(*entry.Decision), signature,
	) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func restartMembershipReadErrorV1(err error, notFound error) error {
	if err == nil || errors.Is(err, notFound) {
		return ErrPublicationIntegrity
	}
	return errors.Join(ErrPublicationRestartUnresolved, err)
}

func recoverPublicationCommitV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) (domainpublication.PublicationCommitReceiptV1, bool, error) {
	if entry.Intent == nil || entry.Settlement == nil || entry.Candidate == nil || entry.Index == nil ||
		domainpublication.ValidatePublicationAttemptGraphV1(entry.Attempt, entry.Stage, *entry.Candidate, *entry.Index) != nil ||
		domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(*entry.Settlement, *entry.Intent, nil) != nil ||
		entry.Settlement.Kind != domainauthority.MonotonicAdvanceSettlementCommittedV2 || entry.Intent.Transition.EvidenceBundle == nil {
		return domainpublication.PublicationCommitReceiptV1{}, false, ErrPublicationIntegrity
	}
	return selectAndSignPublicationCommitV1(
		ctx, entry.Attempt, *entry.Candidate, *entry.Index, entry.Settlement.RecordDigest, nil,
		commitSelectorConfigFromRestartV1(config),
	)
}

func validateCommitSelectionExactV1(ctx context.Context, entry RestartAttemptV1, config RestartApplyConfigV1) error {
	if entry.Selection == nil || entry.Commit == nil || entry.Candidate == nil || entry.Index == nil || entry.Settlement == nil {
		return ErrPublicationIntegrity
	}
	input, err := resolvePublicationCommitSelectionInputV1(
		ctx, *entry.Selection, entry.Attempt, *entry.Candidate, *entry.Index, entry.Settlement.RecordDigest,
		commitSelectorConfigFromRestartV1(config),
	)
	if err != nil || domainpublication.ValidatePublicationCommitReceiptExactV1(*entry.Commit, input) != nil ||
		domainpublication.ValidatePublicationCommitSelectionCommitV1(*entry.Selection, *entry.Commit) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func commitSelectorConfigFromRestartV1(config RestartApplyConfigV1) CommitSelectorConfigV1 {
	return CommitSelectorConfigV1{
		InstallationID: config.InstallationID, EnrollmentID: config.EnrollmentID, Authority: config.Authority,
		WitnessKeyID: config.WitnessKeyID, WitnessKey: config.WitnessKey, HeadReader: config.HeadReader,
		Bundles: config.Bundles, Observations: config.Observations, Selections: config.Selections, Commits: config.Commits,
	}
}

func validateRestartCandidateAuthorityV1(ctx context.Context, entry RestartAttemptV1, config RestartApplyConfigV1) error {
	if entry.Candidate == nil || entry.Index == nil || config.Contexts == nil || config.Ledgers == nil ||
		config.PIIProjections == nil || config.Inspections == nil || config.Artifacts == nil {
		return ErrPublicationIntegrity
	}
	candidate := *entry.Candidate
	securityContext, err := config.Contexts.ResolveCurrent(ctx, candidate.ThreadID, candidate.TurnID, candidate.ContextDigest)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		securityContext.ThreadID != candidate.ThreadID || securityContext.TurnID != candidate.TurnID ||
		securityContext.ContextDigest != candidate.ContextDigest || securityContext.CaseID != candidate.CaseID ||
		securityContext.CaseBindingHash != candidate.CaseBindingHash || securityContext.ContextEpoch != candidate.ContextEpoch ||
		securityContext.DatasetSnapshotID != candidate.DatasetSnapshotID || securityContext.SourceManifestHash != candidate.SourceManifestHash {
		return errors.Join(ErrPublicationRestartUnresolved, err)
	}
	ledger, projection, artifact, err := resolveRestartCandidateDurableMaterialsV1(ctx, entry, config)
	if err != nil {
		return err
	}
	if projection.ProjectionClass == domainpublication.PIIProjectionControlledFull {
		if validateRestartControlledArtifactPIIV1(
			ctx, securityContext, candidate, ledger, projection, artifact, config, nil, nil,
		) != nil {
			return errors.New("controlled report PII authorization is not current during restart")
		}
	}
	return nil
}

func validateRestartCandidateDurableMaterialsV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) error {
	_, _, _, err := resolveRestartCandidateDurableMaterialsV1(ctx, entry, config)
	return err
}

func resolveRestartCandidateDurableMaterialsV1(
	ctx context.Context,
	entry RestartAttemptV1,
	config RestartApplyConfigV1,
) (domainpublication.ClaimLedgerV1, domainpublication.PIIProjectionV1, []byte, error) {
	if entry.Candidate == nil || entry.Index == nil || config.Ledgers == nil || config.PIIProjections == nil ||
		config.Inspections == nil || config.Artifacts == nil {
		return domainpublication.ClaimLedgerV1{}, domainpublication.PIIProjectionV1{}, nil, ErrPublicationIntegrity
	}
	candidate := *entry.Candidate
	projection, err := config.PIIProjections.Resolve(ctx, candidate.PIIProjectionDigest)
	if err != nil || domainpublication.ValidatePIIProjectionV1(projection) != nil ||
		projection.ProjectionDigest != candidate.PIIProjectionDigest || projection.ProjectionClass != candidate.PIIProjectionClass ||
		projection.AuthorizationAuditDigest != candidate.AuthorizationAuditDigest ||
		entry.Attempt.PIIProjectionDigest != projection.ProjectionDigest || entry.Attempt.PIIProjectionClass != projection.ProjectionClass ||
		entry.Attempt.AuthorizationAuditDigest != projection.AuthorizationAuditDigest ||
		entry.Attempt.TargetIdentityDigest != candidate.TargetIdentityDigest {
		return domainpublication.ClaimLedgerV1{}, domainpublication.PIIProjectionV1{}, nil, ErrPublicationIntegrity
	}
	ledger, ledgerErr := config.Ledgers.Resolve(ctx, candidate.ClaimLedgerDigest)
	inspection, inspectionErr := config.Inspections.Resolve(ctx, candidate.RenderInspectionDigest)
	artifact, artifactErr := config.Artifacts.ResolveExact(ctx, candidate.TargetIdentityDigest)
	if ledgerErr != nil || inspectionErr != nil || artifactErr != nil ||
		domainpublication.ValidatePublicationReceiptMaterialsV1(candidate, ledger, projection, inspection) != nil ||
		uint64(len(artifact)) != candidate.ReportByteLength || domainsecurity.SHA256Hex(artifact) != candidate.ReportSHA256 {
		return domainpublication.ClaimLedgerV1{}, domainpublication.PIIProjectionV1{}, nil, ErrPublicationIntegrity
	}
	return ledger, projection, artifact, nil
}

func validateRestartControlledArtifactPIIV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	candidate domainpublication.PublicationReceiptV1,
	ledger domainpublication.ClaimLedgerV1,
	projection domainpublication.PIIProjectionV1,
	artifact []byte,
	config RestartApplyConfigV1,
	snapshot *registryport.WitnessedSnapshot,
	capability registryport.WitnessedSnapshotCapability,
) error {
	metadata, parseErr := domainpii.ControlledPIIArtifactMetadataFromBytesV1(artifact)
	metadataStore, metadataStoreOK := config.Artifacts.(publicationport.ControlledArtifactMetadataStore)
	storedMetadata := domainpii.ControlledPIIArtifactMetadataV1{}
	var metadataErr error
	if metadataStoreOK {
		storedMetadata, metadataErr = metadataStore.ResolveControlledMetadata(ctx, candidate.TargetIdentityDigest)
	}
	authority, authorityOK := config.PIIAuthority.(publicationport.ControlledArtifactPIIAuthorizationAuthority)
	if parseErr != nil || metadataErr != nil || !metadataStoreOK || !reflect.DeepEqual(metadata, storedMetadata) ||
		domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
			metadata, securityContext, ledger.LedgerDigest, projection.RulesetHash,
			candidate.TargetIdentityDigest, candidate.ReportSHA256, candidate.ReportByteLength,
			projection.PreservedControlledFieldCount,
		) != nil || projection.RestrictedFieldCount != projection.PreservedControlledFieldCount || !authorityOK {
		return ErrPublicationIntegrity
	}
	if snapshot != nil {
		return authority.ValidateControlledArtifactCurrentWithinSnapshot(
			ctx, securityContext, projection, metadata, *snapshot, capability,
		)
	}
	return authority.ValidateControlledArtifactCurrent(ctx, securityContext, projection, metadata)
}

func validateCommitExactWitnessV1(
	ctx context.Context,
	commit domainpublication.PublicationCommitReceiptV1,
	candidate domainpublication.PublicationReceiptV1,
	index domainpublication.PublicationIndexV1,
	config RestartApplyConfigV1,
) error {
	committed, err := config.Bundles.Resolve(ctx, commit.CommittedEvidenceBundleDigest)
	if err != nil || committed.RecordDigest != commit.CommittedEvidenceBundleDigest {
		return ErrPublicationIntegrity
	}
	previous, err := config.Bundles.Resolve(ctx, commit.PreviousEvidenceBundleDigest)
	if err != nil || previous.RecordDigest != commit.PreviousEvidenceBundleDigest ||
		domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, committed) != nil {
		return ErrPublicationIntegrity
	}
	observation, err := config.Observations.Resolve(ctx, commit.WitnessBinding.ObservationDigest)
	if err != nil || !reflect.DeepEqual(observation.Bundle, committed) {
		return ErrPublicationIntegrity
	}
	input := domainpublication.PublicationCommitReceiptInputV1{
		PreviousBundle: previous, CommittedBundle: committed,
		ObserveRequest: observation.Request, Observation: observation.Observation,
		Candidate: candidate, Index: index,
		InstallationID: config.InstallationID, EnrollmentID: config.EnrollmentID,
		AuthorityKeyID: config.Authority.KeyID(), AuthorityPublicKey: config.Authority.PublicKey(),
		WitnessKeyID: config.WitnessKeyID, WitnessPublicKey: config.WitnessKey,
	}
	if domainpublication.ValidatePublicationCommitReceiptExactV1(commit, input) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func validateCommitStillWitnessedV1(ctx context.Context, commit domainpublication.PublicationCommitReceiptV1, config RestartApplyConfigV1) error {
	committed, err := config.Bundles.Resolve(ctx, commit.CommittedEvidenceBundleDigest)
	if err != nil || committed.RecordDigest != commit.CommittedEvidenceBundleDigest ||
		domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
			committed, config.InstallationID, config.EnrollmentID, config.Authority.KeyID(), config.Authority.PublicKey(),
		) != nil {
		return ErrPublicationIntegrity
	}
	head, err := config.HeadReader.ObserveFresh(ctx)
	if err != nil {
		return errors.Join(ErrPublicationRestartUnresolved, err)
	}
	if err := validateFreshPublicationHeadV1(head, config); err != nil {
		return err
	}
	ancestor, err := publicationBundleIsAncestorV1(ctx, head.Bundle, committed, config)
	if err != nil || !ancestor {
		return ErrPublicationIntegrity
	}
	return nil
}

func reconcileExactDeliveryOutcomeV1(
	ctx context.Context,
	delivery publicationport.DeliveryOutcomeStore,
	completion domainpublication.ReportStageCompletionV1,
	candidate domainpublication.ReportDeliveryOutcomeV1,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	deliveryID := domainpublication.ReportDeliveryOutcomeID(candidate)
	winner, err := delivery.ResolveOutcome(ctx, deliveryID)
	if err == nil {
		if domainpublication.ValidateReportDeliveryOutcomeCompletionV1(winner, completion) != nil {
			return domainpublication.ReportDeliveryOutcomeV1{}, ErrPublicationIntegrity
		}
		return winner, nil
	}
	if !errors.Is(err, publicationport.ErrNotFound) {
		return domainpublication.ReportDeliveryOutcomeV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	createdWinner, _, createErr := delivery.CreateOutcomeExclusive(ctx, completion, candidate)
	readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
	defer cancel()
	winner, resolveErr := delivery.ResolveOutcome(readbackCtx, deliveryID)
	if resolveErr != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, errors.Join(ErrPublicationRestartUnresolved, createErr, resolveErr)
	}
	if createErr == nil {
		if domainpublication.ValidateReportDeliveryOutcomeV1(createdWinner) != nil ||
			!reflect.DeepEqual(createdWinner, winner) {
			return domainpublication.ReportDeliveryOutcomeV1{}, ErrPublicationIntegrity
		}
	}
	if domainpublication.ValidateReportDeliveryOutcomeCompletionV1(winner, completion) != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, ErrPublicationIntegrity
	}
	return winner, nil
}

func validateFreshPublicationHeadV1(head evidenceauthorityport.FreshHead, config RestartApplyConfigV1) error {
	if !head.HasBundle || domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
		head.Bundle, config.InstallationID, config.EnrollmentID, config.Authority.KeyID(), config.Authority.PublicKey(),
	) != nil || domainevidence.ValidateEvidenceAuthorityBundleCheckpointV1(head.Bundle, head.Observation.Checkpoint) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			head.Observation, head.Request, config.InstallationID, config.Authority.KeyID(), config.Authority.PublicKey(),
			config.EnrollmentID, config.WitnessKeyID, config.WitnessKey,
		) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func publicationBundleIsAncestorV1(
	ctx context.Context,
	current domainevidence.EvidenceAuthorityBundleV1,
	ancestor domainevidence.EvidenceAuthorityBundleV1,
	config RestartApplyConfigV1,
) (bool, error) {
	if current.Generation < ancestor.Generation {
		return false, nil
	}
	cursor := current
	for depth := 0; cursor.Generation > ancestor.Generation; depth++ {
		if depth >= maxPublicationIndexDepthV1 {
			return false, ErrPublicationIntegrity
		}
		previous, err := config.Bundles.Resolve(ctx, cursor.PreviousBundleDigest)
		if err != nil || previous.RecordDigest != cursor.PreviousBundleDigest ||
			domainevidence.ValidateEvidenceAuthorityBundleForInstallationV1(
				previous, config.InstallationID, config.EnrollmentID, config.Authority.KeyID(), config.Authority.PublicKey(),
			) != nil || domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previous, cursor) != nil {
			return false, ErrPublicationIntegrity
		}
		cursor = previous
	}
	return reflect.DeepEqual(cursor, ancestor), nil
}

func validateCommitAuthorityV1(ctx context.Context, commit domainpublication.PublicationCommitReceiptV1, config RestartApplyConfigV1) error {
	keyID, publicKey, signature, err := domainpublication.PublicationCommitReceiptAuthorityMaterialV1(commit)
	if err != nil || config.Authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpublication.PublicationCommitReceiptSigningBytesV1(commit), signature,
	) != nil {
		return ErrPublicationIntegrity
	}
	return nil
}

func validateRestartApplyConfigV1(config RestartApplyConfigV1) error {
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
		config.Evidence == nil || config.HeadReader == nil || config.Advances == nil || config.Pending == nil || config.Attempts == nil ||
		config.Receipts == nil || config.Indexes == nil || config.Intents == nil || config.Settlements == nil ||
		config.Bundles == nil || config.Observations == nil ||
		config.Contexts == nil || config.Ledgers == nil || config.PIIProjections == nil || config.Inspections == nil ||
		config.Artifacts == nil || config.Selections == nil || config.Commits == nil || config.Decisions == nil ||
		config.GrantSettlements == nil || config.StageCompletions == nil || config.Threads == nil || config.DeliveryOutcomes == nil ||
		config.AcquireContextEffect == nil {
		return ErrPublicationUnavailable
	}
	return nil
}
