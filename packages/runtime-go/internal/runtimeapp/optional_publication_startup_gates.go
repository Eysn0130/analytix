package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	reportpublicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	reportpublicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// Complete Original attempt/Core linkage and the typed denial scope precede
// the first recovery or semantic writer. Unavailable owners stay frozen; an
// attempt associated with held unresolved Core work grants no recovery authority.
func prepareRuntimeReportPreservationBeforeRecoveryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority, advance *runtimeAuthorityAdvanceStartupV2) (runtimeReportRestartPreservationV1, error) {
	publication, err := prepareRuntimePublicationSemanticPreservationV1(ctx, core, installation)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	preserved, err := prepareRuntimeReportScopeBeforeRecoveryV1(ctx, core, access, installation, publication, advance)
	if err != nil {
		return runtimeReportRestartPreservationV1{}, err
	}
	preserved.publication = publication
	return preserved, nil
}

func prepareRuntimeReportScopeBeforeRecoveryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, access finalauthority.SecurePrivateCASRecoveryAccessAuthority, installation *finalauthority.AnchoredFileAuthority, publication *runtimePublicationSemanticPreservationV1, advance *runtimeAuthorityAdvanceStartupV2) (runtimeReportRestartPreservationV1, error) {
	empty := runtimeReportRestartPreservationV1{}
	if core == nil {
		return empty, errRuntimeReportRestartReconciliationRequired
	}
	unresolved := false
	for _, receipt := range core.pendingInventory.Receipts {
		if receipt.Kind != domainpendingwork.KindReportStage {
			continue
		}
		if disposition, found := core.pendingInventory.Dispositions[receipt.WorkID]; !found || disposition.Status == domainpendingwork.StatusOutcomeUnknown {
			unresolved = true
		}
	}
	hasAttempts, err := reportpublicationstore.HasPreparedAttemptsV1(ctx, filepath.Join(core.roots.DataDir, "private", "report-publication"), access, publication.creationProofV1("report-publication"))
	if err != nil {
		return empty, err
	}
	if !unresolved && !hasAttempts {
		return prepareRuntimeReportRestartPreservationV1(ctx, core)
	}
	if err := core.revalidate(ctx); err != nil {
		return empty, err
	}
	if !hasAttempts && !publication.unavailable {
		// The complete original endpoint already proved this healthy closure;
		// ordinary create/orphan cleanup has not run yet.
		return prepareRuntimeReportRestartPreservationV1(ctx, core)
	}
	// This gate owns the complete four-owner attempt/publication closure.
	// Associated dataset/evidence physical prefixes and signed Original/Final
	// are observed by preservation below, before the first semantic writer.
	domains, err := prepareRuntimeOptionalDomainCapabilitiesForOwnersV1(ctx, core.roots.DataDir, access, installation, runtimePublicationDomainOwner, publication)
	if err != nil {
		return empty, err
	}
	if err := core.revalidate(ctx); err != nil {
		return empty, err
	}
	for _, name := range []string{"pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2"} {
		capability, exists := domains[name]
		if !exists || !capability.available {
			if hasAttempts && !unresolved {
				return empty, errRuntimeReportRestartReconciliationRequired
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				if unresolved {
					return empty, errors.Join(errRuntimeReportRestartReconciliationRequired, err)
				}
				return empty, err
			}
			if unresolved {
				preserved.publication = publication
				if err := validateRuntimeUnavailableReportPreservationV1(ctx, preserved, core.pendingInventory, domains); err != nil {
					return empty, err
				}
			}
			return preserved, nil
		}
	}
	// Only a fully available owner closure can supply typed attempt facts.
	// The original Core denominator includes every stage, regardless of status.
	if hasAttempts {
		revalidate := func() error {
			if err := core.revalidate(ctx); err != nil {
				return err
			}
			for _, name := range []string{"pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2"} {
				if err := domains[name].prepared.Revalidate(ctx); err != nil {
					return err
				}
			}
			return nil
		}
		if err := revalidate(); err != nil {
			return empty, err
		}
		attempts, err := reportpublicationstore.PrepareAttemptInventoryV1(ctx, filepath.Join(core.roots.DataDir, "private", "report-publication"), access)
		if err != nil {
			return empty, err
		}
		linkageErr := reportpublicationapp.VerifyAttemptCoreInventoryV1(ctx, core.pendingInventory, attempts, core.verification)
		if err := attempts.Revalidate(ctx); err != nil {
			return empty, err
		}
		if err := revalidate(); err != nil {
			return empty, err
		}
		if linkageErr != nil {
			return empty, linkageErr
		}
		// Classify the complete Original/Final history before choosing completed
		// history or exact pre-witness preservation. Neither grants live effects.
		if advance == nil {
			return empty, errRuntimeReportRestartReconciliationRequired
		}
		preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
		if err != nil {
			return empty, err
		}
		history, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, publication, advance, preserved.report)
		if err != nil {
			return empty, err
		}
		preserved.core, preserved.publication = core, publication
		if history.deferred != nil {
			publication.deferred = history.deferred
		}
		if (history.closedFailed || history.closedCompletedPrefix || history.mixedClosed != nil) && history.semantic != nil {
			publication.closed, preserved.history = history, history
		} else if history.deferred == nil {
			if !history.matchesCompletedPlanV1(history.plan) || history.semantic == nil {
				return empty, errRuntimeReportRestartReconciliationRequired
			}
			preserved.history = history
		}
		if err := preserved.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
			return empty, err
		}
		return preserved, nil
	}
	return prepareRuntimeReportRestartPreservationV1(ctx, core)
}

func validateRuntimeUnavailableReportPreservationV1(ctx context.Context, preserved runtimeReportRestartPreservationV1, inventory pendingworkapp.TrustedInventoryV1, domains map[string]runtimeOptionalDomainCapability) (resultErr error) {
	if ctx == nil || preserved.core == nil || preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return errRuntimeReportRestartReconciliationRequired
	}
	core := preserved.core
	revalidate := func() error {
		if err := core.originalRegistryTrust.Revalidate(ctx); err != nil {
			return err
		}
		if err := core.revalidateKey(ctx); err != nil {
			return err
		}
		for _, name := range []string{"pii-authorization", "report-publication", "controlled-artifact-access", "controlled-artifact-access-v2"} {
			capability, found := domains[name]
			if !found || capability.prepared == nil {
				return errRuntimeReportRestartReconciliationRequired
			}
			if err := capability.prepared.Revalidate(ctx); err != nil {
				return err
			}
		}
		return ctx.Err()
	}
	if err := revalidate(); err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, revalidate(), preserved.report.RevalidatePrimary(ctx)) }()
	if domains["controlled-artifact-access-v2"].hasRecords {
		return errRuntimeReportRestartReconciliationRequired
	}
	if err := validateRuntimeOriginalHeldReportAttemptsV1(ctx, preserved); err != nil {
		return err
	}
	if err := preserved.report.ValidateTrustedPendingInventoryV1(ctx, inventory); err != nil {
		return err
	}
	return preserved.ValidateSemanticOperationsV1(ctx, nil, nil, "")
}

// This observer only associates a complete canonical Original attempt set
// with an already trusted held scope. It cannot create a hold, open a store,
// complete a grant, or supply publication/delivery authority.
func validateRuntimeOriginalHeldReportAttemptsV1(ctx context.Context, preserved runtimeReportRestartPreservationV1) (resultErr error) {
	publication, core, scope := preserved.publication, preserved.core, preserved.report
	if publication == nil || !publication.unavailable || core == nil || scope == nil {
		return errRuntimeReportRestartReconciliationRequired
	}
	if err := scope.RevalidatePrimary(ctx); err != nil {
		return err
	}
	original, final, observation, err := publication.readV1(ctx)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx), scope.RevalidatePrimary(ctx)) }()
	if err := publication.validateEndpointV1(ctx, original); err != nil {
		return err
	}
	if err := publication.validateEndpointV1(ctx, final); err != nil {
		return err
	}
	files, found := original["report-publication"]
	if !found {
		return errRuntimeReportRestartReconciliationRequired
	}
	names := []string{}
	for name, entry := range files {
		if entry.Directory || !strings.HasPrefix(name, "attempts/") {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) == 3 && domainprivatecas.ValidShardV1(parts[1]) {
			if _, residue := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); residue {
				// Native physical validation already proved this exact opaque
				// residue. Full4 freezing retains it without parsing or replay.
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	attempts := runtimeOriginalReportAttemptsV1{}
	for _, name := range names {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		entry := files[name]
		attempt, err := domainpublication.ParsePublicationAttemptV1(entry.Body)
		if err != nil {
			return err
		}
		canonical, err := domainpublication.PublicationAttemptV1Bytes(attempt)
		if err != nil || !domainsecurity.IsSHA256Hex(attempt.AttemptID) ||
			name != "attempts/"+attempt.AttemptID[:2]+"/"+attempt.AttemptID+".json" || !bytes.Equal(canonical, entry.Body) {
			return errors.Join(errors.New("original report attempt is not canonical"), err)
		}
		attempts = append(attempts, attempt)
	}
	// Association always uses the signed Original Core denominator. The caller's
	// fresh pending inventory is separately checked for full scope equality.
	if err := reportpublicationapp.VerifyAttemptCoreInventoryV1(ctx, core.pendingInventory, attempts, core.verification); err != nil {
		return err
	}
	stages := map[string]domainpendingwork.PendingWorkReceiptV1{}
	for _, receipt := range core.pendingInventory.Receipts {
		stages[receipt.WorkID] = receipt
	}
	for _, attempt := range attempts {
		stage := stages[attempt.ReportStageWorkID]
		disposition, disposed := core.pendingInventory.Dispositions[stage.WorkID]
		if stage.Kind != domainpendingwork.KindReportStage || !scope.OwnsThread(stage.Context.ThreadID) ||
			(disposed && disposition.Status != domainpendingwork.StatusOutcomeUnknown) || len(stage.GrantMembers) != 1 {
			return errRuntimeReportRestartReconciliationRequired
		}
		primary, err := scope.ReadPrimaryThreadSnapshotV1(ctx, stage.Context.ThreadID)
		if err != nil {
			return err
		}
		registry, err := executiongrantapp.RegistrySnapshotFromThread(stage.Context.ThreadID, primary.Thread, stage.Context.TurnID, stage.GrantRegistrySequence, stage.GrantRegistryDigest)
		if err != nil {
			return err
		}
		member := stage.GrantMembers[0]
		entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
		if !found || entry.Sequence != member.RegistrySequence || entry.EntryDigest != member.RegistryEntryDigest ||
			entry.Status != domainsecurity.GrantRegistryActive || entry.Grant.ToolCallID != attempt.ToolCallID || entry.Grant.ToolName != attempt.ToolName {
			return reportpublicationapp.ErrAttemptCoreLinkageV1
		}
	}
	return context.Cause(ctx)
}

type runtimeOriginalReportAttemptsV1 []domainpublication.PublicationAttemptV1

func (attempts runtimeOriginalReportAttemptsV1) VisitAttempts(ctx context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	if ctx == nil || visit == nil {
		return errRuntimeReportRestartReconciliationRequired
	}
	for _, attempt := range attempts {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		if err := visit(attempt); err != nil {
			return err
		}
	}
	return context.Cause(ctx)
}
