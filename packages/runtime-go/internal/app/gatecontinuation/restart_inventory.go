package gatecontinuation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

type RestartInventoryDependencies struct {
	Continuations              *continuationapp.Service
	GetThread                  func(string) (map[string]any, error)
	EnsureGateRequestItemExact func(string, string, map[string]any) error
	RecordRequest              func(map[string]any) error
	RecordResolution           func(map[string]any) error
	PatchItemStatus            func(map[string]any, controlapp.GateRecord, string) error
	LoadEvents                 func(string) ([]map[string]any, error)
	Now                        func() time.Time
}

type RestartInventory struct {
	deps RestartInventoryDependencies
}

type RestartTurnOwnershipV1 interface {
	OwnsRestartTurnV1(threadID, turnID string) bool
}

func NewRestartInventory(deps RestartInventoryDependencies) (*RestartInventory, error) {
	if deps.Continuations == nil || deps.GetThread == nil || deps.EnsureGateRequestItemExact == nil ||
		deps.RecordRequest == nil || deps.RecordResolution == nil || deps.PatchItemStatus == nil || deps.LoadEvents == nil {
		return nil, errors.New("restart gate inventory dependencies are incomplete")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &RestartInventory{deps: deps}, nil
}

func TrustedGateReceiptsByThreadV1(inventory continuationapp.TrustedInventoryV1) map[string][]domaincontinuation.Receipt {
	grouped := map[string][]domaincontinuation.Receipt{}
	for _, receipt := range inventory.Receipts {
		threadID := strings.TrimSpace(receipt.Payload.ThreadID)
		grouped[threadID] = append(grouped[threadID], receipt)
	}
	return grouped
}

func (s *RestartInventory) CloseOpenWithoutOwningThreadV1(
	threadIDs []string,
	inventory continuationapp.TrustedInventoryV1,
) error {
	return s.CloseOpenWithoutOwningThreadExceptOwnedV1(threadIDs, inventory, nil)
}

func (s *RestartInventory) CloseOpenWithoutOwningThreadExceptOwnedV1(
	threadIDs []string,
	inventory continuationapp.TrustedInventoryV1,
	ownership RestartTurnOwnershipV1,
) error {
	known := make(map[string]bool, len(threadIDs))
	for _, threadID := range threadIDs {
		known[strings.TrimSpace(threadID)] = true
	}
	for _, receipt := range inventory.Receipts {
		if ownsRestartGateTurnV1(ownership, receipt) {
			continue
		}
		if known[receipt.Payload.ThreadID] {
			continue
		}
		if _, disposed := inventory.Dispositions[receipt.Payload.GateID]; disposed {
			continue
		}
		if err := s.disposeRestartInvalid(receipt.Payload.GateID, restartInvalidReasonV1(receipt, "restart_thread_unavailable")); err != nil {
			return fmt.Errorf("close continuation without owning thread %s: %w", receipt.Payload.GateID, err)
		}
	}
	return nil
}

func (s *RestartInventory) CloseOpenForIsolatedThreadV1(
	receipts []domaincontinuation.Receipt,
	dispositions map[string]domaincontinuation.Disposition,
) error {
	return s.CloseOpenForIsolatedThreadExceptOwnedV1(receipts, dispositions, nil)
}

func (s *RestartInventory) CloseOpenForIsolatedThreadExceptOwnedV1(
	receipts []domaincontinuation.Receipt,
	dispositions map[string]domaincontinuation.Disposition,
	ownership RestartTurnOwnershipV1,
) error {
	for _, receipt := range receipts {
		if ownsRestartGateTurnV1(ownership, receipt) {
			continue
		}
		if _, disposed := dispositions[receipt.Payload.GateID]; disposed {
			continue
		}
		if err := s.disposeRestartInvalid(receipt.Payload.GateID, restartInvalidReasonV1(receipt, "restart_thread_quarantined")); err != nil {
			return err
		}
	}
	return nil
}

func (s *RestartInventory) ReconcileThreadV1(
	threadID string,
	thread map[string]any,
	receipts []domaincontinuation.Receipt,
	dispositions map[string]domaincontinuation.Disposition,
) error {
	return s.ReconcileThreadExceptOwnedV1(threadID, thread, receipts, dispositions, nil)
}

func (s *RestartInventory) ReconcileThreadExceptOwnedV1(
	threadID string,
	thread map[string]any,
	receipts []domaincontinuation.Receipt,
	dispositions map[string]domaincontinuation.Disposition,
	ownership RestartTurnOwnershipV1,
) error {
	for _, receipt := range receipts {
		if receipt.Payload.ThreadID != threadID {
			return errors.New("trusted continuation inventory crossed thread ownership")
		}
		if ownsRestartGateTurnV1(ownership, receipt) {
			continue
		}
		if err := s.reconcileReceiptV1(thread, receipt, dispositions[receipt.Payload.GateID]); err != nil {
			return fmt.Errorf("reconcile trusted gate %s: %w", receipt.Payload.GateID, err)
		}
		var err error
		thread, err = s.deps.GetThread(threadID)
		if err != nil {
			return err
		}
	}
	return nil
}

func ownsRestartGateTurnV1(ownership RestartTurnOwnershipV1, receipt domaincontinuation.Receipt) bool {
	return ownership != nil && ownership.OwnsRestartTurnV1(receipt.Payload.ThreadID, receipt.Payload.TurnID)
}

func (s *RestartInventory) reconcileReceiptV1(
	thread map[string]any,
	receipt domaincontinuation.Receipt,
	disposition domaincontinuation.Disposition,
) error {
	projection, err := ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		return err
	}
	turn, status, found := RestartInventoryTurnV1(thread, receipt.Payload.TurnID)
	if !found {
		if disposition.GateID == "" {
			if err := s.disposeRestartInvalid(receipt.Payload.GateID, "restart_turn_unavailable"); err != nil {
				return err
			}
		}
		return errors.New("continuation receipt has no owning turn")
	}
	if appturn.IsTerminalStatus(status) {
		if disposition.GateID == "" {
			if err := s.disposeRestartInvalid(receipt.Payload.GateID, "restart_terminal_without_resolution"); err != nil {
				return err
			}
			return errors.New("terminal turn retained an open continuation receipt")
		}
		return s.validateTerminalV1(thread, turn, projection, disposition)
	}
	if status != "running" && status != "queued" && status != "waiting" {
		return errors.New("continuation receipt owner has an invalid turn status")
	}
	publicStatus := "pending"
	cancelledBy := ""
	if disposition.GateID != "" {
		publicStatus, cancelledBy, err = RestartDispositionProjectionV1(disposition)
		if err != nil {
			return err
		}
	}
	itemFound, itemStatus, err := InspectRestartGateRequestItemV1(turn, projection.Item)
	if err != nil {
		return err
	}
	if !itemFound {
		if err := s.deps.EnsureGateRequestItemExact(receipt.Payload.ThreadID, receipt.Payload.TurnID, projection.Item); err != nil {
			return err
		}
	} else if itemStatus != "pending" && itemStatus != publicStatus {
		return errors.New("gate request item status conflicts with signed authority")
	}
	if err := s.deps.RecordRequest(projection.Event); err != nil {
		return err
	}
	if disposition.GateID == "" {
		if err := s.disposeRestartInvalid(receipt.Payload.GateID, restartInvalidReasonV1(receipt, "restart_nonresumable")); err != nil {
			return err
		}
		_, disposition, err = s.deps.Continuations.ResolveTrustedDispositionForAudit(context.Background(), receipt.Payload.GateID)
		if err != nil {
			return err
		}
		publicStatus, cancelledBy, err = RestartDispositionProjectionV1(disposition)
		if err != nil {
			return err
		}
	}
	resolution := RestartResolutionEventV1(projection, publicStatus, cancelledBy)
	if err := s.deps.RecordResolution(resolution); err != nil {
		return err
	}
	current, err := s.deps.GetThread(receipt.Payload.ThreadID)
	if err != nil {
		return err
	}
	return s.deps.PatchItemStatus(current, projection.Record, publicStatus)
}

func restartInvalidReasonV1(receipt domaincontinuation.Receipt, fallback string) string {
	if receipt.Payload.Version == domaincontinuation.ContractVersionV2 {
		return "restart_legacy_continuation_contract"
	}
	if !domaincontinuation.HasExactProviderStepBinding(receipt.Payload) {
		return "restart_provider_step_binding_missing"
	}
	return strings.TrimSpace(fallback)
}

func (s *RestartInventory) validateTerminalV1(
	thread map[string]any,
	turn map[string]any,
	projection GateRequestProjectionV1,
	disposition domaincontinuation.Disposition,
) error {
	publicStatus, cancelledBy, err := RestartDispositionProjectionV1(disposition)
	if err != nil {
		return err
	}
	found, status, err := InspectRestartGateRequestItemV1(turn, projection.Item)
	if err != nil || !found || status != publicStatus {
		return errors.Join(errors.New("terminal gate item lacks exact signed projection"), err)
	}
	if err := s.requireOneExactEventV1(thread, projection.Event, PendingGateRequestBaseKeyV1); err != nil {
		return err
	}
	resolution := RestartResolutionEventV1(projection, publicStatus, cancelledBy)
	return s.requireOneExactEventV1(thread, resolution, PendingGateResolutionBaseKeyV1)
}

func (s *RestartInventory) requireOneExactEventV1(
	thread map[string]any,
	expected map[string]any,
	baseKey func(map[string]any) string,
) error {
	threadID := strings.TrimSpace(contracts.StringField(expected, "threadId"))
	key := baseKey(expected)
	if threadID == "" || key == "" {
		return errors.New("gate event projection identity is invalid")
	}
	projected, err := appturn.SanitizeCaseEventPublication(thread, expected)
	if err != nil {
		return err
	}
	events, err := s.deps.LoadEvents(threadID)
	if err != nil {
		return err
	}
	matches := 0
	for _, event := range events {
		if baseKey(event) != key {
			continue
		}
		if !ExactGateEventProjectionV1(event, projected) {
			return errors.New("gate event conflicts with signed projection")
		}
		matches++
	}
	if matches != 1 {
		return fmt.Errorf("gate event exact projection count is %d", matches)
	}
	return nil
}

func (s *RestartInventory) disposeRestartInvalid(gateID, reason string) error {
	err := s.deps.Continuations.DisposeHost(
		gateID, domaincontinuation.StatusRestartInvalid, reason, s.deps.Now().UTC(),
	)
	if errors.Is(err, continuationapp.ErrReceiptConsumed) {
		return nil
	}
	return err
}

func RestartDispositionProjectionV1(disposition domaincontinuation.Disposition) (string, string, error) {
	if projection, ok := domaincontinuation.ResolveProjection(disposition.Kind, disposition.Status, disposition.ReasonCode); ok {
		return projection.PublicStatus, "", nil
	}
	if strings.TrimSpace(disposition.ReasonCode) == "" {
		return "", "", errors.New("terminal continuation disposition has no reason")
	}
	switch disposition.Status {
	case domaincontinuation.StatusRestartInvalid, domaincontinuation.StatusRejected, domaincontinuation.StatusInterrupted:
		if disposition.Kind == domaincontinuation.KindApproval {
			return "expired", disposition.ReasonCode, nil
		}
		if disposition.Kind == domaincontinuation.KindUserInput {
			return "cancelled", disposition.ReasonCode, nil
		}
	}
	return "", "", errors.New("continuation disposition cannot be publicly projected")
}

func RestartResolutionEventV1(projection GateRequestProjectionV1, status, cancelledBy string) map[string]any {
	cancellation := controlapp.ApprovalGateCancellation(projection.GateID, projection.Record)
	if projection.Kind == domaincontinuation.KindUserInput {
		cancellation = controlapp.UserInputGateCancellation(projection.GateID, projection.Record)
	}
	var event map[string]any
	if projection.Kind == domaincontinuation.KindApproval {
		event = appturn.ApprovalResolvedEvent(cancellation.Record, status)
	} else {
		event = appturn.UserInputResolvedEvent(cancellation.Record, status)
	}
	if strings.TrimSpace(cancelledBy) != "" {
		event["cancelledBy"] = strings.TrimSpace(cancelledBy)
	}
	return event
}

func RestartInventoryTurnV1(thread map[string]any, turnID string) (map[string]any, string, bool) {
	for _, raw := range listRestartAnyV1(thread["turns"]) {
		turn, _ := raw.(map[string]any)
		if contracts.StringField(turn, "id") == strings.TrimSpace(turnID) {
			return turn, contracts.StringField(turn, "status"), true
		}
	}
	return nil, "", false
}

func InspectRestartGateRequestItemV1(turn map[string]any, expected map[string]any) (bool, string, error) {
	expectedID := strings.TrimSpace(contracts.StringField(expected, "id"))
	matches := 0
	status := ""
	for _, raw := range listRestartAnyV1(turn["items"]) {
		item, _ := raw.(map[string]any)
		if strings.TrimSpace(contracts.StringField(item, "id")) != expectedID {
			continue
		}
		if !sameRestartGateItemAuthorityV1(item, expected) {
			return false, "", errors.New("gate request item identity conflicts with signed projection")
		}
		matches++
		status = strings.TrimSpace(contracts.StringField(item, "status"))
	}
	if matches > 1 {
		return false, "", errors.New("gate request item is duplicated")
	}
	return matches == 1, status, nil
}

func sameRestartGateItemAuthorityV1(existing, expected map[string]any) bool {
	left := contracts.CloneMap(existing)
	right := contracts.CloneMap(expected)
	delete(left, "status")
	delete(left, "finishedAt")
	delete(right, "status")
	delete(right, "finishedAt")
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func listRestartAnyV1(value any) []any {
	items, _ := value.([]any)
	return items
}
