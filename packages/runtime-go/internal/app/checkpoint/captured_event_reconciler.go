package checkpoint

import (
	"errors"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	checkpointcaptureport "analytix.local/runtime-go/internal/ports/checkpointcapture"
)

type CapturedEventReconciler struct {
	store checkpointcaptureport.Store
}

func NewCapturedEventReconciler(store checkpointcaptureport.Store) *CapturedEventReconciler {
	return &CapturedEventReconciler{store: store}
}

func (r *CapturedEventReconciler) ReconcileCheckpointCapturedEventForStartup(draft map[string]any, allowWrite bool) error {
	return r.reconcileCheckpointCapturedEvent(draft, allowWrite, false)
}

func (r *CapturedEventReconciler) ReconcileCheckpointCapturedEvent(draft map[string]any) error {
	return r.reconcileCheckpointCapturedEvent(draft, true, true)
}

func (r *CapturedEventReconciler) reconcileCheckpointCapturedEvent(draft map[string]any, allowWrite, publish bool) error {
	if r == nil || r.store == nil {
		return errors.New("checkpoint captured event store is unavailable")
	}
	if _, err := domaincheckpointref.ParseCapturedEventIdentity(draft); err != nil {
		return err
	}
	return r.store.ReconcileCheckpointCapturedEvent(checkpointcaptureport.ReconcileRequest{
		Draft: draft, AllowWrite: allowWrite, Publish: publish,
	})
}
