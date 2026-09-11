package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

// A closed pre-witness prefix is historical evidence, not a held execution
// scope. UNKNOWN remains unresolved even when Preflight labels it aborted.
func runtimeReportPlanIsClosedPreWitnessV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) == 0 {
		return false
	}
	for _, entry := range plan.Attempts {
		if entry.State != publicationapp.RestartAttemptAbortedV1 || entry.Disposition == nil || entry.Disposition.Status == domainpending.StatusOutcomeUnknown || entry.Disposition.Status == domainpending.StatusCompleted ||
			entry.Intent != nil || entry.Settlement != nil || entry.Selection != nil || entry.Commit != nil || entry.Decision != nil || entry.GrantSettlement != nil || entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			return false
		}
		if _, valid := executiongrantapp.ClosedReportRestartProjectionV1(entry.Stage, *entry.Disposition); !valid {
			return false
		}
	}
	return true
}

func runtimeReportPlanIsClosedFailedV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) == 0 {
		return false
	}
	for _, entry := range plan.Attempts {
		single := publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}
		if runtimeReportPlanIsClosedPreWitnessV1(single) {
			continue
		}
		prefix, valid := runtimeReportUndisposedPrefixV1(entry)
		if !valid || prefix.Intent == nil {
			return false
		}
		if _, valid := executiongrantapp.ClosedReportRestartProjectionV1(entry.Stage, *entry.Disposition); !valid {
			return false
		}
	}
	return true
}

func (history *runtimeOriginalReportHistoryV1) matchesClosedFailedPlanV1(plan publicationapp.RestartPlanV1) bool {
	return history != nil && runtimeReportPlanIsClosedFailedV1(plan) && reflect.DeepEqual(history.plan, plan)
}

func validateRuntimeClosedFailedCoreV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, plan publicationapp.RestartPlanV1, pending pendingapp.TrustedInventoryV1, primaries recoveryport.PrimaryThreadReaderV1) (resultErr error) {
	if ctx == nil || core == nil || publication == nil || publication.installation == nil || advance == nil || primaries == nil || !runtimeReportPlanIsClosedFailedV1(plan) {
		return errRuntimeReportRestartReconciliationRequired
	}
	if err := advance.revalidate(ctx); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, advance.revalidate(ctx), context.Cause(ctx)) }()
	verifier := pendingapp.NewService(publication.installation, nil, nil)
	projection := core.originalRegistryTrust.projection
	if err := validateRuntimeClosedReportPendingV1(plan, pending); err != nil {
		return err
	}
	for _, entry := range plan.Attempts {
		if err := domainpublication.ValidatePublicationAttemptForInstallationV1(entry.Attempt, projection.InstallationID, projection.Enrollment.EnrollmentID, core.verification.KeyID(), core.verification.PublicKey()); err != nil {
			return err
		}
		primary, err := primaries.ReadPrimaryThreadSnapshotV1(ctx, entry.Stage.Context.ThreadID)
		if err != nil {
			return err
		}
		if err := verifier.VerifyHistoricalReportStageInputV1(ctx, entry.Stage, primary.Thread, entry.Attempt.ToolCallID, entry.Attempt.StageInputHash); err != nil {
			return err
		}
		if entry.Intent != nil {
			if entry.Settlement == nil {
				if err := validateRuntimeOriginalUnsettledReportIntentV1(core, entry); err != nil {
					return err
				}
			} else if err := validateRuntimeOriginalCommittedReportAdvanceV1(core, entry); err != nil {
				return err
			}
		}
		result, err := runtimeClosedReportResultV1(entry, primary.Thread)
		if err != nil {
			return err
		}
		if entry.Intent != nil && result == nil {
			return errors.New("closed post-intent report requires its actual failed Core result")
		}
		if result != nil && !executiongrantapp.ClosedReportRestartResultMatchesV1(result, entry.Stage, *entry.Disposition, false) {
			return errors.New("closed aborted report has an incompatible Core result")
		}
	}
	return ctx.Err()
}

func runtimeClosedReportResultV1(entry publicationapp.RestartAttemptV1, primary map[string]any) (map[string]any, error) {
	registry, err := executiongrantapp.RegistryFromThread(entry.Stage.Context.ThreadID, primary, entry.Stage.Context.TurnID)
	if err != nil {
		return nil, err
	}
	grant, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, entry.Stage.GrantMembers[0].GrantID)
	if !found {
		return nil, errors.New("closed report lost its Core grant")
	}
	if grant.Status == domainsecurity.GrantRegistryActive {
		return nil, nil
	}
	if grant.Status != domainsecurity.GrantRegistrySettled {
		return nil, errors.New("closed report has an unsupported Core grant state")
	}
	resultID := domaintoolresult.ToolResultItemIDV1(entry.Stage.Context.TurnID, entry.Attempt.ToolCallID)
	durable, err := executiongrantapp.DurableSettlementFromThread(entry.Stage.Context.ThreadID, primary, entry.Stage.Context.TurnID, resultID, grant.Grant)
	return durable.ResultItem, err
}

func (history *runtimeOriginalReportHistoryV1) captureClosedResultsV1(ctx context.Context, primaries recoveryport.PrimaryThreadReaderV1) error {
	history.closedResults = map[string]string{}
	for _, entry := range history.plan.Attempts {
		if !runtimeReportPlanIsClosedFailedV1(publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}) {
			continue
		}
		primary, err := primaries.ReadPrimaryThreadSnapshotV1(ctx, entry.Stage.Context.ThreadID)
		if err != nil {
			return err
		}
		result, err := runtimeClosedReportResultV1(entry, primary.Thread)
		if err != nil {
			return err
		}
		digest := ""
		if result != nil {
			body, err := json.Marshal(result)
			if err != nil {
				return err
			}
			digest = domainsecurity.SHA256Hex(body)
		}
		history.closedResults[entry.Stage.WorkID] = digest
	}
	return nil
}

func (history *runtimeOriginalReportHistoryV1) validateClosedResultsV1(ctx context.Context, primaries recoveryport.PrimaryThreadReaderV1) error {
	for _, entry := range history.plan.Attempts {
		if !runtimeReportPlanIsClosedFailedV1(publicationapp.RestartPlanV1{Attempts: []publicationapp.RestartAttemptV1{entry}}) {
			continue
		}
		original, found := history.closedResults[entry.Stage.WorkID]
		if !found {
			return errors.New("closed report original result observation is unavailable")
		}
		primary, err := primaries.ReadPrimaryThreadSnapshotV1(ctx, entry.Stage.Context.ThreadID)
		if err != nil {
			return err
		}
		result, err := runtimeClosedReportResultV1(entry, primary.Thread)
		if err != nil {
			return err
		}
		if original != "" {
			body, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if result == nil || domainsecurity.SHA256Hex(body) != original {
				return errors.New("closed report original Core result changed")
			}
		} else if result != nil && !executiongrantapp.ClosedReportRestartResultMatchesV1(result, entry.Stage, *entry.Disposition, true) {
			return errors.New("closed report added an unauthorized Core restart result")
		}
	}
	return nil
}

func validateRuntimeClosedReportPendingV1(plan publicationapp.RestartPlanV1, pending pendingapp.TrustedInventoryV1) error {
	for _, entry := range plan.Attempts {
		if !runtimeReportEntryIsClosedV1(entry) || entry.Disposition == nil {
			return errRuntimeReportRestartReconciliationRequired
		}
		found := false
		for _, receipt := range pending.Receipts {
			if receipt.WorkID == entry.Stage.WorkID {
				if found || !reflect.DeepEqual(receipt, entry.Stage) {
					return errors.New("closed report original receipt changed")
				}
				found = true
			}
		}
		if disposition, ok := pending.Dispositions[entry.Stage.WorkID]; !found || !ok || !reflect.DeepEqual(disposition, *entry.Disposition) {
			return errors.New("closed report original disposition changed")
		}
	}
	return nil
}

// The successful Core effect is closed, while publication delivery still has
// an exact missing suffix. This grants neither a thread hold nor delivery.
func runtimeReportPlanIsClosedCompletedPrefixV1(plan publicationapp.RestartPlanV1) bool {
	if len(plan.Attempts) != 1 {
		return false
	}
	entry := plan.Attempts[0]
	if entry.State != publicationapp.RestartAttemptStageDispositionV1 && entry.State != publicationapp.RestartAttemptStageCompletionV1 {
		return false
	}
	return entry.Disposition != nil && entry.Disposition.Status == domainpending.StatusCompleted &&
		entry.Intent != nil && entry.Settlement != nil && entry.Settlement.Kind == domainauthority.MonotonicAdvanceSettlementCommittedV2 && entry.Settlement.Committed != nil &&
		entry.Candidate != nil && entry.Index != nil && entry.Selection != nil && entry.Commit != nil && entry.Decision != nil && entry.GrantSettlement != nil &&
		(entry.StageCompletion != nil) == (entry.State == publicationapp.RestartAttemptStageCompletionV1) && entry.DeliveryOutcome == nil
}

func (history *runtimeOriginalReportHistoryV1) matchesClosedCompletedPrefixPlanV1(plan publicationapp.RestartPlanV1) bool {
	return history != nil && runtimeReportPlanIsClosedCompletedPrefixV1(plan) && reflect.DeepEqual(history.plan, plan)
}

func validateRuntimeClosedCompletedCoreV1(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2, plan publicationapp.RestartPlanV1, pending pendingapp.TrustedInventoryV1, primaries recoveryport.PrimaryThreadReaderV1) (resultErr error) {
	if ctx == nil || core == nil || publication == nil || publication.installation == nil || advance == nil || primaries == nil || !runtimeReportPlanIsClosedCompletedPrefixV1(plan) {
		return errRuntimeReportRestartReconciliationRequired
	}
	if err := advance.revalidate(ctx); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, advance.revalidate(ctx), context.Cause(ctx)) }()
	if err := validateRuntimeClosedReportPendingV1(plan, pending); err != nil {
		return err
	}
	verifier := pendingapp.NewService(publication.installation, nil, nil)
	projection := core.originalRegistryTrust.projection
	for _, entry := range plan.Attempts {
		if err := domainpublication.ValidatePublicationAttemptForInstallationV1(entry.Attempt, projection.InstallationID, projection.Enrollment.EnrollmentID, core.verification.KeyID(), core.verification.PublicKey()); err != nil {
			return err
		}
		if err := validateRuntimeOriginalCommittedReportAdvanceV1(core, entry); err != nil {
			return err
		}
		primary, err := primaries.ReadPrimaryThreadSnapshotV1(ctx, entry.Stage.Context.ThreadID)
		if err != nil {
			return err
		}
		if err := verifier.VerifyHistoricalReportStageInputV1(ctx, entry.Stage, primary.Thread, entry.Attempt.ToolCallID, entry.Attempt.StageInputHash); err != nil {
			return err
		}
		// Complete endpoint Preflight already verifies current-key disposition,
		// admitted result, persisted settlement and optional completion graph.
	}
	return ctx.Err()
}
