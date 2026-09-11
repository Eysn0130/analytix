package pendingwork

import (
	"context"
	"errors"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

// TrustedCompletedReportStageV1 is one immutable, installation-verified
// completed report effect reconstructed from a single durable thread read.
// ResultItem is a caller-owned deep copy; it is private host state and must
// never be projected to SSE, history, logs, or providers.
type TrustedCompletedReportStageV1 struct {
	Receipt          domainpendingwork.PendingWorkReceiptV1
	Disposition      domainpendingwork.PendingWorkDispositionV1
	Grant            domainsecurity.ExecutionGrant
	ActiveRegistry   domainsecurity.ExecutionGrantRegistry
	SettledRegistry  domainsecurity.ExecutionGrantRegistry
	ResultItemID     string
	ResultItemDigest string
	ResultItem       map[string]any
	SettledAt        time.Time
}

// TrustedSettledReportStageV1 is the exact successful durable result used to
// create a report grant settlement before any completed stage disposition is
// written. It deliberately has no disposition field: a generic restart close
// cannot be reinterpreted as the missing decision-bound settlement writer.
type TrustedSettledReportStageV1 struct {
	Receipt          domainpendingwork.PendingWorkReceiptV1
	Grant            domainsecurity.ExecutionGrant
	ActiveRegistry   domainsecurity.ExecutionGrantRegistry
	SettledRegistry  domainsecurity.ExecutionGrantRegistry
	ResultItemID     string
	ResultItemDigest string
	ResultItem       map[string]any
	SettledAt        time.Time
}

// ResolveTrustedSettledReportStageForDecisionV1 performs one durable thread
// read and proves the report result's active->settled grant transition plus
// its exact private host admission. Any existing stage disposition blocks this
// pre-disposition authority so restart code cannot repair an invalid ordering
// by signing around a generic closure.
func (service *Service) ResolveTrustedSettledReportStageForDecisionV1(
	ctx context.Context,
	workID string,
	decisionID string,
	decisionRecordDigest string,
) (TrustedSettledReportStageV1, error) {
	if service == nil {
		return TrustedSettledReportStageV1{}, ErrAuthorityUnavailable
	}
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	return service.resolveTrustedSettledReportStageForDecisionV1(ctx, workID, decisionID, decisionRecordDigest)
}

// WithTrustedSettledReportStageForDecisionV1 holds the same process-owned
// terminal reservation used by every pending-work close while callback
// installs the decision-bound settlement CAS. The production data-root lease
// excludes a second process, so a disposition is mechanically ordered either
// before the final open check or after the settlement write.
func (service *Service) WithTrustedSettledReportStageForDecisionV1(
	ctx context.Context,
	workID string,
	decisionID string,
	decisionRecordDigest string,
	callback func(TrustedSettledReportStageV1) error,
) error {
	if service == nil || callback == nil {
		return ErrAuthorityUnavailable
	}
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	settled, err := service.resolveTrustedSettledReportStageForDecisionV1(ctx, workID, decisionID, decisionRecordDigest)
	if err != nil {
		return err
	}
	return callback(settled)
}

func (service *Service) resolveTrustedSettledReportStageForDecisionV1(
	ctx context.Context,
	workID string,
	decisionID string,
	decisionRecordDigest string,
) (TrustedSettledReportStageV1, error) {
	if !service.Available() || ctx == nil {
		return TrustedSettledReportStageV1{}, ErrAuthorityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	workID = strings.TrimSpace(workID)
	decisionID = strings.TrimSpace(decisionID)
	decisionRecordDigest = strings.TrimSpace(decisionRecordDigest)
	if !domainsecurity.IsSHA256Hex(workID) || !domainsecurity.IsSHA256Hex(decisionID) ||
		!domainsecurity.IsSHA256Hex(decisionRecordDigest) {
		return TrustedSettledReportStageV1{}, ErrOperationMismatch
	}
	receipt, err := service.store.ReadReceipt(ctx, workID)
	if err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	if service.verifyReceipt(ctx, receipt) != nil || receipt.WorkID != workID ||
		receipt.Kind != domainpendingwork.KindReportStage || len(receipt.GrantMembers) != 1 {
		return TrustedSettledReportStageV1{}, ErrOperationMismatch
	}
	if service.restartOwnsThread(receipt.Context.ThreadID) {
		return TrustedSettledReportStageV1{}, ErrRestartPreserved
	}
	if _, err := service.store.ReadDisposition(ctx, workID); err == nil {
		return TrustedSettledReportStageV1{}, ErrWorkClosed
	} else if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return TrustedSettledReportStageV1{}, err
	}
	thread, err := service.grants.GetThread(receipt.Context.ThreadID)
	if err != nil || thread == nil || mapString(thread, "id") != receipt.Context.ThreadID {
		return TrustedSettledReportStageV1{}, errors.Join(ErrGrantAuthority, err)
	}
	settled, err := inspectSettledReportStageFromThreadForDecisionV1(ctx, receipt, thread, decisionID, decisionRecordDigest)
	if err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	if _, err := service.store.ReadDisposition(ctx, workID); err == nil {
		return TrustedSettledReportStageV1{}, ErrWorkClosed
	} else if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return TrustedSettledReportStageV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	return settled, nil
}

// InspectOriginalSettledReportStageForDecisionV1 audits one supplied Original
// primary and current-installation receipt without reading a live store,
// acquiring an effect lease or authorizing a settlement write. The caller owns
// complete inventory, Original observation and absence-of-disposition proof.
func (service *Service) InspectOriginalSettledReportStageForDecisionV1(
	ctx context.Context,
	receipt domainpendingwork.PendingWorkReceiptV1,
	thread map[string]any,
	decisionID, decisionRecordDigest string,
) (TrustedSettledReportStageV1, error) {
	if service == nil || service.authority == nil || ctx == nil {
		return TrustedSettledReportStageV1{}, ErrAuthorityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	if err := service.verifyReceipt(ctx, receipt); err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	if receipt.Kind != domainpendingwork.KindReportStage || len(receipt.GrantMembers) != 1 ||
		!domainsecurity.IsSHA256Hex(decisionID) || !domainsecurity.IsSHA256Hex(decisionRecordDigest) {
		return TrustedSettledReportStageV1{}, ErrOperationMismatch
	}
	return inspectSettledReportStageFromThreadForDecisionV1(ctx, receipt, thread, decisionID, decisionRecordDigest)
}

func inspectSettledReportStageFromThreadForDecisionV1(
	ctx context.Context,
	receipt domainpendingwork.PendingWorkReceiptV1,
	thread map[string]any,
	decisionID, decisionRecordDigest string,
) (TrustedSettledReportStageV1, error) {
	if ctx == nil || thread == nil || mapString(thread, "id") != receipt.Context.ThreadID {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	turn, found := appmodel.TurnByID(thread, receipt.Context.TurnID)
	if !found {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !receiptMatchesContext(receipt, securityContext) {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	registry, err := executiongrantapp.RegistryFromThread(receipt.Context.ThreadID, thread, receipt.Context.TurnID)
	if err != nil {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	member := receipt.GrantMembers[0]
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
	if !found || entry.Status != domainsecurity.GrantRegistrySettled ||
		!writeEffectReceiptMatchesRegistry(receipt, thread, entry.Grant) {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	resultItemID, resultItem, found := exactDurableResultItem(thread, securityContext, entry.Grant)
	if !found {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	durable, err := executiongrantapp.DurableSettlementFromThread(
		securityContext.ThreadID, thread, securityContext.TurnID, resultItemID, entry.Grant,
	)
	if err != nil || verifyCompletedGrantAuthority(receipt, securityContext, thread, registry, durable.SettledAt.UTC()) != nil ||
		domaintoolresult.ValidatePrivateAdmittedReportResultItemV1(
			resultItem, decisionID, decisionRecordDigest,
		) != nil {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	resultItem = clonePendingWorkRecordV1(resultItem)
	resultDigest := canonicalRecordHash(resultItem)
	if resultDigest == "" {
		return TrustedSettledReportStageV1{}, ErrGrantAuthority
	}
	if err := ctx.Err(); err != nil {
		return TrustedSettledReportStageV1{}, err
	}
	return TrustedSettledReportStageV1{
		Receipt: clonePendingWorkReceipt(receipt), Grant: entry.Grant,
		ActiveRegistry:  clonePendingWorkRegistryV1(durable.ActiveRegistry),
		SettledRegistry: clonePendingWorkRegistryV1(durable.SettledRegistry),
		ResultItemID:    resultItemID, ResultItemDigest: resultDigest, ResultItem: resultItem,
		SettledAt: durable.SettledAt.UTC(),
	}, nil
}

// ResolveTrustedCompletedReportStageV1 resolves one exact report-stage
// terminal without exposing the raw persistence store as publication
// authority. Both records must be signed by the current installation and the
// completed disposition must still bind the unique durable grant result.
func (service *Service) ResolveTrustedCompletedReportStageV1(
	ctx context.Context,
	workID string,
) (TrustedCompletedReportStageV1, error) {
	if !service.Available() || ctx == nil {
		return TrustedCompletedReportStageV1{}, ErrAuthorityUnavailable
	}
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	if err := ctx.Err(); err != nil {
		return TrustedCompletedReportStageV1{}, err
	}
	workID = strings.TrimSpace(workID)
	if !domainsecurity.IsSHA256Hex(workID) {
		return TrustedCompletedReportStageV1{}, ErrOperationMismatch
	}
	receipt, err := service.store.ReadReceipt(ctx, workID)
	if err != nil {
		return TrustedCompletedReportStageV1{}, err
	}
	if service.verifyReceipt(ctx, receipt) != nil || receipt.WorkID != workID ||
		receipt.Kind != domainpendingwork.KindReportStage || len(receipt.GrantMembers) != 1 {
		return TrustedCompletedReportStageV1{}, ErrOperationMismatch
	}
	if service.restartOwnsThread(receipt.Context.ThreadID) {
		return TrustedCompletedReportStageV1{}, ErrRestartPreserved
	}
	disposition, err := service.store.ReadDisposition(ctx, workID)
	if errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return TrustedCompletedReportStageV1{}, ErrWorkAlreadyOpen
	}
	if err != nil {
		return TrustedCompletedReportStageV1{}, err
	}
	if disposition.Status != domainpendingwork.StatusCompleted ||
		service.verifyDisposition(ctx, disposition, receipt) != nil {
		return TrustedCompletedReportStageV1{}, ErrGrantAuthority
	}
	terminal, err := service.resolveCompletedInventoryDisposition(receipt, disposition)
	if err != nil || terminal == nil {
		return TrustedCompletedReportStageV1{}, errors.Join(ErrGrantAuthority, err)
	}
	if err := ctx.Err(); err != nil {
		return TrustedCompletedReportStageV1{}, err
	}
	return *terminal, nil
}
