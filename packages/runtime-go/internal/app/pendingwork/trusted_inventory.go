package pendingwork

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

// TrustedInventoryV1 is an all-or-nothing private authority snapshot. Receipt
// and disposition values are copies; callers never receive store-owned slices.
type TrustedInventoryV1 struct {
	Receipts     []domainpendingwork.PendingWorkReceiptV1
	Dispositions map[string]domainpendingwork.PendingWorkDispositionV1
}

// RestartOutcomeUnknownGrantV1 identifies one approved writable grant whose
// external effect may have happened before the runtime process stopped.
// It carries no tool payload, case ID, PII, or provider output.
type RestartOutcomeUnknownGrantV1 struct {
	ThreadID    string
	TurnID      string
	GrantID     string
	Receipt     domainpendingwork.PendingWorkReceiptV1
	Disposition domainpendingwork.PendingWorkDispositionV1
}

// TrustedInventoryV1 verifies the complete store before returning any member.
// Duplicate receipts, orphan/duplicate dispositions, untrusted signatures,
// and multiple approved-dispatch receipts for one context/grant all fail closed.
func (service *Service) TrustedInventoryV1(ctx context.Context) (TrustedInventoryV1, error) {
	if service == nil || service.authority == nil || service.store == nil ||
		strings.TrimSpace(service.authority.KeyID()) == "" || len(service.authority.PublicKey()) == 0 {
		return TrustedInventoryV1{}, ErrAuthorityUnavailable
	}
	receipts, dispositions, err := service.store.SnapshotInventory(ctx)
	if err != nil {
		return TrustedInventoryV1{}, err
	}
	return service.verifyTrustedSnapshotV1(ctx, receipts, dispositions)
}

// VerifyTrustedSnapshotV1 verifies a complete prepared read-only inventory
// before a live store or any allocator is opened. It uses the same signature,
// graph, and immutable-copy checks as the live store snapshot.
func VerifyTrustedSnapshotV1(ctx context.Context, receipts []domainpendingwork.PendingWorkReceiptV1, dispositions []domainpendingwork.PendingWorkDispositionV1, authority authorityport.Authority) (TrustedInventoryV1, error) {
	if ctx == nil || authority == nil || strings.TrimSpace(authority.KeyID()) == "" || len(authority.PublicKey()) == 0 {
		return TrustedInventoryV1{}, ErrAuthorityUnavailable
	}
	if err := ctx.Err(); err != nil {
		return TrustedInventoryV1{}, err
	}
	return (&Service{authority: authority}).verifyTrustedSnapshotV1(ctx, receipts, dispositions)
}

func (service *Service) verifyTrustedSnapshotV1(ctx context.Context, receipts []domainpendingwork.PendingWorkReceiptV1, dispositions []domainpendingwork.PendingWorkDispositionV1) (TrustedInventoryV1, error) {
	receiptByWork := make(map[string]domainpendingwork.PendingWorkReceiptV1, len(receipts))
	receiptIDs := make(map[string]bool, len(receipts))
	approvedEffectIDs := make(map[string]bool)
	trustedReceipts := make([]domainpendingwork.PendingWorkReceiptV1, 0, len(receipts))
	for _, receipt := range receipts {
		if err := service.verifyReceipt(ctx, receipt); err != nil {
			return TrustedInventoryV1{}, errors.Join(errors.New("pending work receipt lacks current installation authority"), err)
		}
		if receiptByWork[receipt.WorkID].WorkID != "" || receiptIDs[receipt.ReceiptID] {
			return TrustedInventoryV1{}, errors.New("pending work receipt inventory is invalid")
		}
		receiptIDs[receipt.ReceiptID] = true
		if receipt.Kind == domainpendingwork.KindApprovedToolDispatch || receipt.Kind == domainpendingwork.KindSideEffectIntent || receipt.Kind == domainpendingwork.KindReportStage {
			if len(receipt.GrantMembers) != 1 {
				return TrustedInventoryV1{}, errors.New("approved dispatch receipt membership is invalid")
			}
			effectID := receipt.WorkID
			if receipt.Kind != domainpendingwork.KindSideEffectIntent {
				effectID = strings.Join([]string{
					receipt.Context.ContextDigest,
					receipt.Context.ThreadID,
					receipt.Context.TurnID,
					receipt.GrantMembers[0].GrantID,
				}, "\x00")
			}
			if approvedEffectIDs[effectID] {
				return TrustedInventoryV1{}, errors.New("approved dispatch receipt effect identity is duplicated")
			}
			approvedEffectIDs[effectID] = true
		}
		cloned := clonePendingWorkReceipt(receipt)
		receiptByWork[receipt.WorkID] = cloned
		trustedReceipts = append(trustedReceipts, cloned)
	}
	sort.Slice(trustedReceipts, func(i, j int) bool { return trustedReceipts[i].WorkID < trustedReceipts[j].WorkID })
	trustedDispositions := make(map[string]domainpendingwork.PendingWorkDispositionV1, len(dispositions))
	dispositionIDs := make(map[string]bool, len(dispositions))
	for _, disposition := range dispositions {
		receipt, found := receiptByWork[disposition.WorkID]
		if !found || trustedDispositions[disposition.WorkID].WorkID != "" || dispositionIDs[disposition.DispositionID] {
			return TrustedInventoryV1{}, errors.New("pending work disposition inventory is invalid")
		}
		if err := service.verifyDisposition(ctx, disposition, receipt); err != nil {
			return TrustedInventoryV1{}, errors.Join(errors.New("pending work disposition lacks current installation authority"), err)
		}
		unknownReason := disposition.ReasonCode == "tool_outcome_unknown_after_restart" ||
			disposition.ReasonCode == "report_stage_outcome_unknown_after_restart"
		if (disposition.Status == domainpendingwork.StatusOutcomeUnknown) != unknownReason ||
			(disposition.Status == domainpendingwork.StatusOutcomeUnknown && !canonicalOutcomeUnknownDisposition(receipt, disposition)) {
			return TrustedInventoryV1{}, errors.New("pending work outcome-unknown disposition is invalid")
		}
		// Persistence preflight may run before the grant reader is assembled.
		// Once runtime authority is available, every completed disposition is
		// re-bound to its unique durable tool_result and exact active->settled
		// registry transition on every inventory read.
		if disposition.Status == domainpendingwork.StatusCompleted && service.grants != nil {
			if err := service.verifyCompletedInventoryDisposition(receipt, disposition); err != nil {
				return TrustedInventoryV1{}, errors.Join(errors.New("completed pending work lost its durable settlement authority"), err)
			}
		}
		dispositionIDs[disposition.DispositionID] = true
		trustedDispositions[disposition.WorkID] = disposition
	}
	return TrustedInventoryV1{Receipts: trustedReceipts, Dispositions: trustedDispositions}, nil
}

func (service *Service) verifyCompletedInventoryDisposition(
	receipt domainpendingwork.PendingWorkReceiptV1,
	disposition domainpendingwork.PendingWorkDispositionV1,
) error {
	_, err := service.resolveCompletedInventoryDisposition(receipt, disposition)
	return err
}

func (service *Service) resolveCompletedInventoryDisposition(
	receipt domainpendingwork.PendingWorkReceiptV1,
	disposition domainpendingwork.PendingWorkDispositionV1,
) (*TrustedCompletedReportStageV1, error) {
	thread, err := service.grants.GetThread(receipt.Context.ThreadID)
	if err != nil {
		return nil, errors.Join(ErrGrantAuthority, err)
	}
	return resolveCompletedInventoryDispositionFromThreadV1(receipt, disposition, thread)
}

func resolveCompletedInventoryDispositionFromThreadV1(
	receipt domainpendingwork.PendingWorkReceiptV1,
	disposition domainpendingwork.PendingWorkDispositionV1,
	thread map[string]any,
) (*TrustedCompletedReportStageV1, error) {
	if thread == nil || mapString(thread, "id") != receipt.Context.ThreadID {
		return nil, ErrGrantAuthority
	}
	turn, found := appmodel.TurnByID(thread, receipt.Context.TurnID)
	if !found {
		return nil, ErrGrantAuthority
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || !receiptMatchesContext(receipt, securityContext) {
		return nil, ErrGrantAuthority
	}
	registry, err := executiongrantapp.RegistryFromThread(receipt.Context.ThreadID, thread, receipt.Context.TurnID)
	if err != nil {
		return nil, ErrGrantAuthority
	}
	disposedAt, err := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if err != nil {
		return nil, ErrGrantAuthority
	}
	if err := verifyCompletedGrantAuthority(receipt, securityContext, thread, registry, disposedAt.UTC()); err != nil {
		return nil, err
	}
	if receipt.Kind != domainpendingwork.KindReportStage {
		return nil, nil
	}
	if len(receipt.GrantMembers) != 1 {
		return nil, ErrGrantAuthority
	}
	member := receipt.GrantMembers[0]
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, member.GrantID)
	if !found {
		return nil, ErrGrantAuthority
	}
	resultItemID, resultItem, found := exactDurableResultItem(thread, securityContext, entry.Grant)
	if !found {
		return nil, ErrGrantAuthority
	}
	durable, err := executiongrantapp.DurableSettlementFromThread(
		securityContext.ThreadID, thread, securityContext.TurnID, resultItemID, entry.Grant,
	)
	if err != nil {
		return nil, ErrGrantAuthority
	}
	resultItem = clonePendingWorkRecordV1(resultItem)
	resultDigest := canonicalRecordHash(resultItem)
	if resultDigest == "" {
		return nil, ErrGrantAuthority
	}
	return &TrustedCompletedReportStageV1{
		Receipt: clonePendingWorkReceipt(receipt), Disposition: disposition, Grant: entry.Grant,
		ActiveRegistry:  clonePendingWorkRegistryV1(durable.ActiveRegistry),
		SettledRegistry: clonePendingWorkRegistryV1(durable.SettledRegistry),
		ResultItemID:    resultItemID, ResultItemDigest: resultDigest, ResultItem: resultItem,
		SettledAt: durable.SettledAt,
	}, nil
}

// VerifyTrustedInventoryV1 is the persistence-preflight form used before the
// runtime has assembled live grant readers. It validates the same complete
// graph and installation key without granting execution authority.
func VerifyTrustedInventoryV1(
	ctx context.Context,
	store pendingworkstoreport.Store,
	authority authorityport.Authority,
) error {
	service := &Service{authority: authority, store: store}
	_, err := service.TrustedInventoryV1(ctx)
	return err
}

// RestartOutcomeUnknownGrantsV1 derives the only restart classification that
// may replace generic cancellation. A prior exact outcome-unknown disposition
// remains authoritative across every later restart.
func RestartOutcomeUnknownGrantsV1(inventory TrustedInventoryV1) ([]RestartOutcomeUnknownGrantV1, error) {
	result := make([]RestartOutcomeUnknownGrantV1, 0)
	seen := map[string]bool{}
	for _, receipt := range inventory.Receipts {
		disposition, closed := inventory.Dispositions[receipt.WorkID]
		if !closed || disposition.Status != domainpendingwork.StatusOutcomeUnknown {
			continue
		}
		if !canonicalOutcomeUnknownDisposition(receipt, disposition) || len(receipt.GrantMembers) != 1 {
			return nil, errors.New("pending work outcome-unknown authority is invalid")
		}
		grantID := receipt.GrantMembers[0].GrantID
		identity := strings.Join([]string{receipt.Context.ThreadID, receipt.Context.TurnID, grantID}, "\x00")
		if seen[identity] {
			return nil, errors.New("pending work outcome-unknown grant is duplicated")
		}
		seen[identity] = true
		result = append(result, RestartOutcomeUnknownGrantV1{
			ThreadID: receipt.Context.ThreadID, TurnID: receipt.Context.TurnID, GrantID: grantID,
			Receipt: clonePendingWorkReceipt(receipt), Disposition: disposition,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ThreadID != result[j].ThreadID {
			return result[i].ThreadID < result[j].ThreadID
		}
		if result[i].TurnID != result[j].TurnID {
			return result[i].TurnID < result[j].TurnID
		}
		return result[i].GrantID < result[j].GrantID
	})
	return result, nil
}

func canonicalOutcomeUnknownDisposition(
	receipt domainpendingwork.PendingWorkReceiptV1,
	disposition domainpendingwork.PendingWorkDispositionV1,
) bool {
	if disposition.Status != domainpendingwork.StatusOutcomeUnknown {
		return false
	}
	return ((receipt.Kind == domainpendingwork.KindApprovedToolDispatch || receipt.Kind == domainpendingwork.KindSideEffectIntent) && disposition.ReasonCode == "tool_outcome_unknown_after_restart") ||
		(receipt.Kind == domainpendingwork.KindReportStage && disposition.ReasonCode == "report_stage_outcome_unknown_after_restart")
}

func clonePendingWorkReceipt(receipt domainpendingwork.PendingWorkReceiptV1) domainpendingwork.PendingWorkReceiptV1 {
	receipt.GrantMembers = append([]domainpendingwork.GrantMemberV1(nil), receipt.GrantMembers...)
	receipt.ChildProducer = domainpendingwork.CloneChildProducerV1(receipt.ChildProducer)
	return receipt
}

func clonePendingWorkRegistryV1(registry domainsecurity.ExecutionGrantRegistry) domainsecurity.ExecutionGrantRegistry {
	registry.Entries = append([]domainsecurity.ExecutionGrantRegistryEntry(nil), registry.Entries...)
	return registry
}

func clonePendingWorkRecordV1(record map[string]any) map[string]any {
	if record == nil {
		return nil
	}
	cloned := make(map[string]any, len(record))
	for key, value := range record {
		cloned[key] = clonePendingWorkValueV1(value)
	}
	return cloned
}

func clonePendingWorkValueV1(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return clonePendingWorkRecordV1(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index := range typed {
			cloned[index] = clonePendingWorkValueV1(typed[index])
		}
		return cloned
	default:
		return typed
	}
}
