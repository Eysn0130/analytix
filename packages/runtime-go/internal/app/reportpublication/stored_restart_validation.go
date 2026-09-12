package reportpublication

import (
	"context"
	"errors"

	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// ValidateStoredSettledResultV1 verifies the exact original decision, Core
// grant transition and optional durable grant settlement. It issues no record
// or current publication capability; private settlement values stay here.
func ValidateStoredSettledResultV1(ctx context.Context, entry RestartAttemptV1, settled pendingapp.TrustedSettledReportStageV1, authorityKeyID string, authorityPublicKey []byte) error {
	decision := entry.Decision
	if decision == nil || entry.Disposition != nil {
		return errors.New("stored settled report decision is unavailable")
	}
	active, found := domainsecurity.ExecutionGrantRegistryEntryByID(settled.ActiveRegistry, decision.GrantID)
	if !found || settled.ActiveRegistry.Sequence != decision.GrantRegistrySequence || settled.ActiveRegistry.StateDigest != decision.GrantRegistryDigest ||
		active.EntryDigest != decision.GrantRegistryEntryDigest || settled.ActiveRegistry.Sequence != settled.SettledRegistry.Sequence ||
		settled.ActiveRegistry.StateDigest == settled.SettledRegistry.StateDigest || settled.Grant.GrantID != decision.GrantID ||
		settled.Grant.ToolCallID != decision.ToolCallID || settled.Grant.ToolName != decision.ToolName ||
		settled.ResultItemID != domaintoolresult.ToolResultItemIDV1(decision.Context.TurnID, decision.ToolCallID) ||
		!domainsecurity.IsSHA256Hex(settled.ResultItemDigest) || settled.SettledAt.IsZero() {
		return errors.New("original report result lost its exact decision grant transition")
	}
	if entry.GrantSettlement != nil {
		if err := domainpublication.ValidateReportGrantSettlementGraphV1(*entry.GrantSettlement, domainpublication.ReportGrantSettlementInputV1{
			Decision: *decision, Grant: settled.Grant, ActiveRegistry: settled.ActiveRegistry, SettledRegistry: settled.SettledRegistry,
			ResultItemID: settled.ResultItemID, ResultItemDigest: settled.ResultItemDigest, SettledAt: settled.SettledAt,
			AuthorityKeyID: authorityKeyID, AuthorityPublicKey: authorityPublicKey,
		}); err != nil {
			return err
		}
	}
	return errors.Join(
		domainsecurity.VerifyExecutionGrantMembership(settled.ActiveRegistry, decision.Context.ThreadID, decision.Context.TurnID, settled.Grant, domainsecurity.GrantRegistryActive),
		domainsecurity.VerifyExecutionGrantMembership(settled.SettledRegistry, decision.Context.ThreadID, decision.Context.TurnID, settled.Grant, domainsecurity.GrantRegistrySettled),
		context.Cause(ctx),
	)
}

// VerifyStoredAttemptHistoryV1 interprets an existing outcome only through the
// complete historical authority. Missing outcomes remain an unfinished prefix;
// no outcome or grant-settlement value crosses this error-only boundary.
func VerifyStoredAttemptHistoryV1(ctx context.Context, historical *ProjectedDeliveryAuthorityV1, entry RestartAttemptV1) error {
	if entry.DeliveryOutcome == nil {
		return nil
	}
	if entry.Commit == nil {
		return errors.New("original report outcome lost its commit")
	}
	if historical == nil {
		return errors.New("original report historical authority is unavailable")
	}
	frozen := entry.Stage.Context
	selector := publicationport.HistoricalProjectedDeliverySelectorV1{
		DeliveryID: domainpublication.ReportDeliveryOutcomeID(*entry.DeliveryOutcome), OutcomeRecordDigest: domainpublication.ReportDeliveryOutcomeRecordDigest(*entry.DeliveryOutcome),
		PublicationCommitDigest: entry.Commit.RecordDigest, ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, ContextDigest: frozen.ContextDigest,
		CaseBindingHash: frozen.CaseBindingHash, ContextEpoch: frozen.ContextEpoch, DatasetSnapshotID: frozen.DatasetSnapshotID, SourceManifestHash: frozen.SourceManifestHash,
	}
	return historical.VerifyTrustedHistoricalDeliveryOutcomeV1(ctx, selector)
}
