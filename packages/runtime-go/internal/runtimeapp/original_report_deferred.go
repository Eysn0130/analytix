package runtimeapp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// This is a verified unfinished cut, distinct from domain unavailability.
// It supplies only preservation and denial, never a publication recovery plan.
type runtimeDeferredReportHistoryV1 struct {
	plan    publicationapp.RestartPlanV1
	advance *runtimeAuthorityAdvanceStartupV2
}

// A single post-witness cut has a signed Advance receipt and may have an exact
// stored selection, matching commit and decision. A settled Core result must
// independently match that decision. A present grant settlement must already
// pass the complete exact durable graph; no missing suffix is inferred.
// It supplies preservation only.
// Mixed post-witness histories require their own complete linkage proof.
func runtimeReportPlanIsPostWitnessDeferredV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) != 1 {
		return false
	}
	entry := plan.Attempts[0]
	switch entry.State {
	case publicationapp.RestartAttemptCommittedSettlementV1, publicationapp.RestartAttemptCommitSelectionV1, publicationapp.RestartAttemptCommitReceiptV1, publicationapp.RestartAttemptDeliveryDecisionV1, publicationapp.RestartAttemptGrantSettlementV1:
	default:
		return false
	}
	return entry.Intent != nil && entry.Settlement != nil &&
		entry.Settlement.Kind == domainauthority.MonotonicAdvanceSettlementCommittedV2 && entry.Settlement.Committed != nil &&
		entry.Candidate != nil && entry.Index != nil && entry.Disposition == nil &&
		(entry.Selection != nil) == (entry.State != publicationapp.RestartAttemptCommittedSettlementV1) &&
		(entry.Commit != nil) == (entry.State == publicationapp.RestartAttemptCommitReceiptV1 || entry.State == publicationapp.RestartAttemptDeliveryDecisionV1 || entry.State == publicationapp.RestartAttemptGrantSettlementV1) &&
		(entry.Decision != nil) == (entry.State == publicationapp.RestartAttemptDeliveryDecisionV1 || entry.State == publicationapp.RestartAttemptGrantSettlementV1) &&
		(entry.GrantSettlement != nil) == (entry.State == publicationapp.RestartAttemptGrantSettlementV1) && entry.StageCompletion == nil && entry.DeliveryOutcome == nil
}

func runtimeReportPlanIsDeferredV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) == 0 {
		return false
	}
	for _, entry := range plan.Attempts {
		single := publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}
		if !runtimeReportPlanIsPreWitnessDeferredV1(single) && !runtimeReportPlanIsPostWitnessDeferredV1(single) && !runtimeReportPlanIsUnsettledIntentV1(single) && !runtimeReportPlanIsUnknownV1(single) {
			return false
		}
	}
	return true
}

func runtimeReportPlanIsPreWitnessDeferredV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) == 0 {
		return false
	}
	for _, entry := range plan.Attempts {
		if entry.Disposition != nil || entry.Intent != nil || entry.Settlement != nil || entry.Selection != nil || entry.Commit != nil || entry.Decision != nil || entry.GrantSettlement != nil || entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return false
		}
		switch entry.State {
		case publicationapp.RestartAttemptReservedV1:
			if entry.Candidate != nil || entry.Index != nil {
				return false
			}
		case publicationapp.RestartAttemptCandidateDurableV1:
			if entry.Candidate == nil || entry.Index != nil {
				return false
			}
		case publicationapp.RestartAttemptMaterialsDurableV1:
			if entry.Candidate == nil || entry.Index == nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func prepareRuntimeDeferredReportHistoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, scope *pendingapp.ReportRestartScopeV1, before, after publicationapp.RestartPlanV1, original, final map[string]runtimeOriginalSemanticFilesV1) (*runtimeDeferredReportHistoryV1, error) {
	if !runtimeReportPlanIsDeferredV1(before) || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(original, final) {
		return nil, errors.New("deferred report Original/Final prefix changed")
	}
	// Controlled-access history needs its own complete historical authority;
	// these unfinished cuts cannot infer it from a partial publication plan.
	for _, owner := range []string{"controlled-artifact-access", "controlled-artifact-access-v2"} {
		if len(runtimeReportCommittedFilesV1(original[owner])) != 0 {
			return nil, errRuntimeReportRestartReconciliationRequired
		}
	}
	deferred := &runtimeDeferredReportHistoryV1{plan: before, advance: advance}
	if err := deferred.validateV1(ctx, core, publication, scope, core.pendingInventory); err != nil {
		return nil, err
	}
	return deferred, nil
}

func (deferred *runtimeDeferredReportHistoryV1) validateV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, scope *pendingapp.ReportRestartScopeV1, inventory pendingapp.TrustedInventoryV1) (resultErr error) {
	if ctx == nil || deferred == nil || core == nil || publication == nil || publication.unavailable || publication.installation == nil || scope == nil || deferred.advance == nil || !runtimeReportPlanIsDeferredV1(deferred.plan) {
		return errRuntimeReportRestartReconciliationRequired
	}
	revalidate := func() error {
		if err := deferred.advance.revalidate(ctx); err != nil {
			return fmt.Errorf("revalidate deferred report authority advance: %w", err)
		}
		return errors.Join(core.revalidateKey(ctx), core.originalRegistryTrust.Revalidate(ctx), publication.installation.ValidateCurrentInstallation(ctx), scope.RevalidatePrimary(ctx), context.Cause(ctx))
	}
	if err := revalidate(); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, revalidate()) }()
	if err := scope.ValidateTrustedPendingInventoryV1(ctx, inventory); err != nil {
		return err
	}
	verifier := pendingapp.NewService(publication.installation, nil, nil)
	for _, entry := range deferred.plan.Attempts {
		projection := core.originalRegistryTrust.projection
		if err := domainpublication.ValidatePublicationAttemptForInstallationV1(entry.Attempt, projection.InstallationID, projection.Enrollment.EnrollmentID, core.verification.KeyID(), core.verification.PublicKey()); err != nil {
			return err
		}
		// Every intent-bearing prefix requires independent authority checks,
		// whether or not a witness settlement has been stored.
		if entry.Intent != nil {
			if entry.Settlement == nil {
				if err := validateRuntimeOriginalUnsettledReportIntentV1(core, entry); err != nil {
					return err
				}
			} else if err := validateRuntimeOriginalCommittedReportAdvanceV1(core, entry); err != nil {
				return err
			}
		}
		if !scope.OwnsThread(entry.Stage.Context.ThreadID) {
			return errRuntimeReportRestartReconciliationRequired
		}
		disposition, disposed := inventory.Dispositions[entry.Stage.WorkID]
		if entry.Disposition != nil {
			if !runtimeReportPlanIsUnknownV1(publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}) || !disposed || !reflect.DeepEqual(disposition, *entry.Disposition) {
				return errors.New("deferred report original UNKNOWN disposition changed")
			}
		} else if disposed {
			return errors.New("deferred report acquired a pending disposition")
		}
		primary, err := scope.ReadPrimaryThreadSnapshotV1(ctx, entry.Stage.Context.ThreadID)
		if err != nil {
			return err
		}
		if err := verifier.VerifyHistoricalReportStageInputV1(ctx, entry.Stage, primary.Thread, entry.Attempt.ToolCallID, entry.Attempt.StageInputHash); err != nil {
			return err
		}
		if entry.Intent != nil || entry.Disposition != nil {
			// Restart state alone cannot distinguish before-result from a later
			// Core result whose grant-settlement suffix is still absent.
			registry, err := executiongrantapp.RegistryFromThread(entry.Stage.Context.ThreadID, primary.Thread, entry.Stage.Context.TurnID)
			if err != nil {
				return err
			}
			member, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, entry.Attempt.GrantID)
			if !found || member.ContextDigest != entry.Stage.Context.ContextDigest {
				return errors.New("deferred report requires an unsettled Original execution grant")
			}
			if entry.Disposition != nil && member.Status != domainsecurity.GrantRegistryActive {
				return errors.New("original UNKNOWN requires its still Active Core grant")
			}
			if entry.GrantSettlement != nil && member.Status != domainsecurity.GrantRegistrySettled {
				return errors.New("original report grant settlement requires its settled Core result")
			}
			if member.Status != domainsecurity.GrantRegistryActive {
				if member.Status != domainsecurity.GrantRegistrySettled || entry.Decision == nil {
					return errors.New("deferred report requires an unsettled Original execution grant")
				}
				if err := validateRuntimeDeferredOriginalReportResultV1(ctx, core, verifier, entry, primary.Thread); err != nil {
					return errors.Join(errors.New("original report Core result does not match its delivery decision"), err)
				}
			}
		}
	}
	return nil
}

func validateRuntimeDeferredOriginalReportResultV1(ctx context.Context, core *runtimeChildIdentityStartupV1, verifier *pendingapp.Service, entry publicationapp.RestartAttemptV1, thread map[string]any) error {
	decision := entry.Decision
	if decision == nil || entry.Disposition != nil {
		return errRuntimeReportRestartReconciliationRequired
	}
	settled, err := verifier.InspectOriginalSettledReportStageForDecisionV1(ctx, entry.Stage, thread, decision.DecisionID, decision.RecordDigest)
	if err != nil {
		return err
	}
	return publicationapp.ValidateStoredSettledResultV1(ctx, entry, settled, core.verification.KeyID(), core.verification.PublicKey())
}

func (deferred *runtimeDeferredReportHistoryV1) validateSemanticV1(ctx context.Context, preserved runtimeReportRestartPreservationV1, operations []domainstartup.SemanticStartupOperationV1) error {
	for _, operation := range operations {
		if err := validateRuntimeOriginalSemanticAncestorV1("data/private/authority-advance", operation); err != nil {
			return err
		}
		if _, owned := runtimeAssociatedRelativeV1("authority-advance", operation); owned {
			return errors.New("deferred report authority advance cannot be changed by semantic startup")
		}
	}
	return deferred.validateV1(ctx, preserved.core, preserved.publication, preserved.report, preserved.core.pendingInventory)
}

func validateRuntimeOriginalCommittedReportAdvanceV1(core *runtimeChildIdentityStartupV1, entry publicationapp.RestartAttemptV1) error {
	projection := core.originalRegistryTrust.projection
	intent, settlement := entry.Intent, entry.Settlement
	if intent == nil || settlement == nil || settlement.Committed == nil || intent.Namespace != projection.Enrollment.Namespace ||
		domainauthority.ValidateMonotonicAdvanceSettlementForIntentV2(*settlement, *intent, nil) != nil {
		return errors.New("committed report settlement lost its exact Original intent")
	}
	if err := domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(intent.PreviousCheckpoint, intent.AdvanceRequest, settlement.Committed.Receipt,
		projection.InstallationID, core.verification.KeyID(), core.verification.PublicKey(), projection.Enrollment.EnrollmentID, projection.Enrollment.WitnessKeyID, core.originalRegistryTrust.witnessKey); err != nil {
		return errors.Join(errors.New("committed report settlement lacks independent witness authority"), err)
	}
	return nil
}

// An unsettled intent proves only its local write-ahead boundary. It neither
// proves nor retries a witness commit, and cannot acquire a terminal suffix.
func runtimeReportPlanIsUnsettledIntentV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) != 1 {
		return false
	}
	entry := plan.Attempts[0]
	return entry.State == publicationapp.RestartAttemptIntentDurableV1 && entry.Intent != nil && entry.Candidate != nil && entry.Index != nil &&
		entry.Settlement == nil && entry.Selection == nil && entry.Commit == nil && entry.Decision == nil && entry.GrantSettlement == nil && entry.Disposition == nil && entry.StageCompletion == nil && entry.DeliveryOutcome == nil
}

func validateRuntimeOriginalUnsettledReportIntentV1(core *runtimeChildIdentityStartupV1, entry publicationapp.RestartAttemptV1) error {
	intent := entry.Intent
	projection := core.originalRegistryTrust.projection
	if intent == nil || entry.Settlement != nil ||
		domainauthority.ValidateMonotonicAdvanceIntentV2(*intent) != nil || intent.Root != domainauthority.AdvanceRootPublicationV2 ||
		intent.InstallationID != projection.InstallationID || intent.EnrollmentID != projection.Enrollment.EnrollmentID || intent.Namespace != projection.Enrollment.Namespace ||
		intent.AuthorityKeyID != core.verification.KeyID() || intent.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(core.verification.PublicKey()) {
		return errors.New("unsettled report intent lost its independent installation binding")
	}
	if err := domainsecurity.ValidateMonotonicHeadCheckpointForWitnessV1(intent.PreviousCheckpoint, projection.InstallationID, projection.Enrollment.EnrollmentID, projection.Enrollment.WitnessKeyID, core.originalRegistryTrust.witnessKey); err != nil {
		return errors.Join(errors.New("unsettled report intent lacks independent witness authority"), err)
	}
	return domainsecurity.ValidateMonotonicHeadAdvanceRequestForInstallationV1(intent.AdvanceRequest, projection.InstallationID, core.verification.KeyID(), core.verification.PublicKey())
}

// Preflight's disposition label overlays the stored publication prefix. This
// copy is used only to classify its proof obligations, never as an inventory,
// recovery input, inferred state change, or replacement for the sealed master.
func runtimeReportUndisposedPrefixV1(entry publicationapp.RestartAttemptV1) (publicationapp.RestartAttemptV1, bool) {
	if entry.Disposition == nil || entry.GrantSettlement != nil || entry.StageCompletion != nil || entry.DeliveryOutcome != nil ||
		(entry.Intent == nil && entry.State != publicationapp.RestartAttemptAbortedV1) ||
		(entry.Intent != nil && entry.State != publicationapp.RestartAttemptDeliveryBlockedV1) {
		return entry, false
	}
	entry.Disposition = nil
	switch {
	case entry.Decision != nil:
		entry.State = publicationapp.RestartAttemptDeliveryDecisionV1
	case entry.Commit != nil:
		entry.State = publicationapp.RestartAttemptCommitReceiptV1
	case entry.Selection != nil:
		entry.State = publicationapp.RestartAttemptCommitSelectionV1
	case entry.Settlement != nil:
		entry.State = publicationapp.RestartAttemptCommittedSettlementV1
	case entry.Intent != nil:
		entry.State = publicationapp.RestartAttemptIntentDurableV1
	case entry.Index != nil:
		entry.State = publicationapp.RestartAttemptMaterialsDurableV1
	case entry.Candidate != nil:
		entry.State = publicationapp.RestartAttemptCandidateDurableV1
	default:
		entry.State = publicationapp.RestartAttemptReservedV1
	}
	single := publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}
	return entry, runtimeReportPlanIsPreWitnessDeferredV1(single) || runtimeReportPlanIsPostWitnessDeferredV1(single) || runtimeReportPlanIsUnsettledIntentV1(single)
}

// A durable UNKNOWN retains its exact original disposition and Active Core
// hold at any supported pre-result prefix. No witness effect is inferred.
func runtimeReportPlanIsUnknownV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) != 1 {
		return false
	}
	entry := plan.Attempts[0]
	_, valid := runtimeReportUndisposedPrefixV1(entry)
	return valid && entry.Disposition.Status == domainpending.StatusOutcomeUnknown && entry.Disposition.ReasonCode == "report_stage_outcome_unknown_after_restart"
}
