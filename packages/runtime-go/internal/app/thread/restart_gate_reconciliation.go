package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
)

type RestartGateContinuationAuthorityV1 interface {
	ValidateRestartRecordHost(gateID, receiptID, kind, threadID, turnID, itemID string) error
	ResolveTrustedDispositionForAudit(context.Context, string) (domaincontinuation.Receipt, domaincontinuation.Disposition, error)
	RestartDispositionReason(
		gateID, receiptID, kind, threadID, turnID, itemID string,
		thread map[string]any,
		resolver appmodel.StrictPendingToolProviderResolver,
		authorize func(appmodel.PendingToolCall) error,
	) string
	DisposeHost(gateID, status, reasonCode string, at time.Time) error
}

type RestartGateReconciliationDependenciesV1 struct {
	Continuations       RestartGateContinuationAuthorityV1
	ProviderResolver    appmodel.StrictPendingToolProviderResolver
	AuthorizePending    func(context.Context, appmodel.PendingToolCall, string, time.Time) error
	PatchTurnItemStatus func(threadID, turnID, itemID, status string) error
	RecordResolution    func(map[string]any) error
	RecordCancellations func([]controlapp.PendingGateCancellation, string) error
	CurrentTime         func() time.Time
}

func RevalidateAndCloseRestartedGateV1(
	kind, gateID string,
	thread map[string]any,
	state controlapp.PendingGateState[appmodel.PendingToolCall],
	deps RestartGateReconciliationDependenciesV1,
) error {
	if err := validateRestartGateReconciliationDependenciesV1(deps); err != nil {
		return err
	}
	record := state.Record
	if err := deps.Continuations.ValidateRestartRecordHost(
		gateID, record.ContinuationReceiptID, kind,
		record.ThreadID, record.TurnID, record.ItemID,
	); err != nil {
		return err
	}
	reason := ""
	_, disposition, dispositionErr := deps.Continuations.ResolveTrustedDispositionForAudit(context.Background(), gateID)
	if dispositionErr == nil {
		if projection, ok := domaincontinuation.ResolveProjection(kind, disposition.Status, disposition.ReasonCode); ok {
			if err := PatchRestartedGateItemStatusV1(thread, record, projection.PublicStatus, deps.PatchTurnItemStatus); err != nil {
				return err
			}
			return deps.RecordResolution(restartedGateResolutionEventV1(kind, gateID, record, projection.PublicStatus))
		}
		switch disposition.Status {
		case domaincontinuation.StatusRestartInvalid:
			if disposition.ReasonCode == "" || !strings.HasPrefix(disposition.ReasonCode, "restart_") {
				return errors.New("restarted continuation has an invalid restart disposition")
			}
		case domaincontinuation.StatusRejected, domaincontinuation.StatusInterrupted:
			if disposition.ReasonCode == "" {
				return errors.New("restarted continuation terminal disposition has no reason")
			}
		default:
			return errors.New("restarted continuation has an unsupported signed disposition")
		}
		reason = disposition.ReasonCode
	} else if !errors.Is(dispositionErr, continuationstoreport.ErrNotFound) {
		return dispositionErr
	} else {
		expectedApprovalState := "not_required"
		if kind == domaincontinuation.KindApproval {
			expectedApprovalState = "pending"
		}
		reason = deps.Continuations.RestartDispositionReason(
			gateID, record.ContinuationReceiptID, kind,
			record.ThreadID, record.TurnID, record.ItemID,
			thread, deps.ProviderResolver,
			func(pending appmodel.PendingToolCall) error {
				return deps.AuthorizePending(context.Background(), pending, expectedApprovalState, currentRestartGateTimeV1(deps))
			},
		)
		if err := deps.Continuations.DisposeHost(
			gateID, domaincontinuation.StatusRestartInvalid, reason, currentRestartGateTimeV1(deps),
		); err != nil {
			return err
		}
	}
	status := "expired"
	if kind == domaincontinuation.KindUserInput {
		status = "cancelled"
	}
	if err := PatchRestartedGateItemStatusV1(thread, record, status, deps.PatchTurnItemStatus); err != nil {
		return err
	}
	cancellation := controlapp.ApprovalGateCancellation(gateID, record)
	if kind == domaincontinuation.KindUserInput {
		cancellation = controlapp.UserInputGateCancellation(gateID, record)
	}
	return deps.RecordCancellations([]controlapp.PendingGateCancellation{cancellation}, reason)
}

func ValidateClosedRestartedGateResolutionV1(
	kind, gateID string,
	thread map[string]any,
	resolution controlapp.RestartGateResolution,
	deps RestartGateReconciliationDependenciesV1,
) error {
	if err := validateRestartGateReconciliationDependenciesV1(deps); err != nil {
		return err
	}
	record := resolution.Record
	if err := deps.Continuations.ValidateRestartRecordHost(
		gateID, record.ContinuationReceiptID, kind,
		record.ThreadID, record.TurnID, record.ItemID,
	); err != nil {
		return err
	}
	_, disposition, err := deps.Continuations.ResolveTrustedDispositionForAudit(context.Background(), gateID)
	if err != nil {
		return err
	}
	if projection, ok := domaincontinuation.ResolveProjection(kind, disposition.Status, disposition.ReasonCode); ok {
		if resolution.CancelledBy != "" || resolution.Status != projection.PublicStatus {
			return errors.New("closed gate resolution does not match its signed disposition")
		}
		return PatchRestartedGateItemStatusV1(thread, record, projection.PublicStatus, deps.PatchTurnItemStatus)
	}
	expectedStatus := "expired"
	if kind == domaincontinuation.KindUserInput {
		expectedStatus = "cancelled"
	}
	if resolution.Status != expectedStatus || resolution.CancelledBy == "" ||
		resolution.CancelledBy != disposition.ReasonCode {
		return errors.New("closed terminal gate projection does not match its signed disposition")
	}
	switch disposition.Status {
	case domaincontinuation.StatusRestartInvalid, domaincontinuation.StatusRejected, domaincontinuation.StatusInterrupted:
		return PatchRestartedGateItemStatusV1(thread, record, expectedStatus, deps.PatchTurnItemStatus)
	default:
		return errors.New("closed terminal gate has an unsupported signed disposition")
	}
}

func validateRestartGateReconciliationDependenciesV1(deps RestartGateReconciliationDependenciesV1) error {
	if deps.Continuations == nil || deps.ProviderResolver == nil || deps.AuthorizePending == nil ||
		deps.PatchTurnItemStatus == nil || deps.RecordResolution == nil || deps.RecordCancellations == nil {
		return errors.New("restart gate reconciliation authority is unavailable")
	}
	return nil
}

func currentRestartGateTimeV1(deps RestartGateReconciliationDependenciesV1) time.Time {
	if deps.CurrentTime != nil {
		return deps.CurrentTime().UTC()
	}
	return time.Now().UTC()
}

func PatchRestartedGateItemStatusV1(
	thread map[string]any,
	record controlapp.GateRecord,
	status string,
	patch func(threadID, turnID, itemID, status string) error,
) error {
	turns, _ := thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if contracts.StringField(turn, "id") != record.TurnID {
			continue
		}
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if contracts.StringField(item, "id") == record.ItemID &&
				contracts.StringField(item, "status") == status {
				return nil
			}
		}
		break
	}
	return patch(record.ThreadID, record.TurnID, record.ItemID, status)
}

func restartedGateResolutionEventV1(
	kind, gateID string,
	record controlapp.GateRecord,
	status string,
) map[string]any {
	if kind == domaincontinuation.KindApproval {
		return appturn.ApprovalResolvedEvent(controlapp.ApprovalGateCancellation(gateID, record).Record, status)
	}
	return appturn.UserInputResolvedEvent(controlapp.UserInputGateCancellation(gateID, record).Record, status)
}
