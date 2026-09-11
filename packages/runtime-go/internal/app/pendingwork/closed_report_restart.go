package pendingwork

import (
	"context"
	"errors"
	"reflect"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
)

// BindClosedReportRestartV1 accepts only the exact WorkIDs whose complete
// Original report history the root has audited. It creates no execution hold.
func (service *Service) BindClosedReportRestartV1(ctx context.Context, inventory TrustedInventoryV1, workIDs []string) error {
	if ctx == nil || service == nil {
		return ErrAuthorityUnavailable
	}
	bound := map[string]executiongrantapp.RestartGrantOutcomeAuthorityV1{}
	for _, id := range workIDs {
		if _, duplicate := bound[id]; duplicate {
			return errors.New("closed report restart WorkID is duplicated")
		}
		found := false
		for _, receipt := range inventory.Receipts {
			if receipt.WorkID != id {
				continue
			}
			disposition, exists := inventory.Dispositions[id]
			_, valid := executiongrantapp.ClosedReportRestartProjectionV1(receipt, disposition)
			if found || !exists || !valid {
				return errors.New("closed report restart authority is invalid")
			}
			if err := errors.Join(service.verifyReceipt(ctx, receipt), service.verifyDisposition(ctx, disposition, receipt)); err != nil {
				return err
			}
			bound[id] = executiongrantapp.RestartGrantOutcomeAuthorityV1{Receipt: clonePendingWorkReceipt(receipt), Disposition: disposition}
			found = true
		}
		if !found {
			return errors.New("closed report restart receipt is absent")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	service.restartMu.Lock()
	defer service.restartMu.Unlock()
	if service.closedReportRestart != nil {
		return errors.New("closed report restart authority is already bound")
	}
	service.closedReportRestart = bound
	return nil
}

func (service *Service) AddClosedReportRestartOutcomesV1(ctx context.Context, inventory TrustedInventoryV1, outcomes map[string]executiongrantapp.RestartGrantOutcomesV1) error {
	if ctx == nil || service == nil || outcomes == nil {
		return ErrAuthorityUnavailable
	}
	service.restartMu.RLock()
	defer service.restartMu.RUnlock()
	seen := map[string]bool{}
	// Verify the entire selected denominator before modifying the output map.
	for id, authority := range service.closedReportRestart {
		found := false
		for _, receipt := range inventory.Receipts {
			if receipt.WorkID == id {
				if found || !reflect.DeepEqual(receipt, authority.Receipt) {
					return errors.New("closed report restart receipt changed")
				}
				found = true
			}
		}
		if disposition, exists := inventory.Dispositions[id]; !found || !exists || !reflect.DeepEqual(disposition, authority.Disposition) {
			return errors.New("closed report restart disposition changed")
		}
		if err := errors.Join(service.verifyReceipt(ctx, authority.Receipt), service.verifyDisposition(ctx, authority.Disposition, authority.Receipt)); err != nil {
			return err
		}
		turnKey := authority.Receipt.Context.ThreadID + "\x00" + authority.Receipt.Context.TurnID
		grantID := authority.Receipt.GrantMembers[0].GrantID
		if _, exists := outcomes[turnKey][grantID]; exists || seen[turnKey+"\x00"+grantID] {
			return errors.New("closed report restart grant authority conflicts")
		}
		seen[turnKey+"\x00"+grantID] = true
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, authority := range service.closedReportRestart {
		turnKey := authority.Receipt.Context.ThreadID + "\x00" + authority.Receipt.Context.TurnID
		if outcomes[turnKey] == nil {
			outcomes[turnKey] = executiongrantapp.RestartGrantOutcomesV1{}
		}
		authority.Receipt = clonePendingWorkReceipt(authority.Receipt)
		outcomes[turnKey][authority.Receipt.GrantMembers[0].GrantID] = authority
	}
	return nil
}
