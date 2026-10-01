package evidenceregistry

import (
	"context"
	"errors"
	"reflect"
	"sync"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

// A recovery lease is callback-scoped. Every use challenges current authority;
// the original signed record remains byte-identical and is never reissued.
func (service *Service) WithRecoveredFactFinalWitness(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, use func(registryport.FactFinalWitnessCapability) error) error {
	if use == nil || record.AcceptedFinal.FactFinalWitnessAdmission == nil || record.AcceptedFinal.FactFinalWitnessAdmission.SchemaVersion != domainevidence.FactFinalWitnessAdmissionSchemaVersionV2 {
		return errors.New("recovered fact callback is unavailable")
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(record)
	if err != nil {
		return err
	}
	original, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
	if err != nil {
		return err
	}
	if err := service.VerifyFactFinalWitnessCurrent(ctx, original); err != nil {
		return err
	}
	lease := &recoveredFactCapability{active: true, ctx: ctx, service: service, record: original}
	defer func() { lease.mu.Lock(); lease.active = false; lease.mu.Unlock() }()
	return use(lease)
}

type recoveredFactCapability struct {
	mu      sync.RWMutex
	active  bool
	ctx     context.Context
	service *Service
	record  domainevidence.PrivateAcceptedFinalRecord
}

func (lease *recoveredFactCapability) PrivateFinal() (domainevidence.PrivateAcceptedFinalRecord, error) {
	lease.mu.RLock()
	defer lease.mu.RUnlock()
	if !lease.active || lease.ctx.Err() != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, errors.New("recovered fact authority is inactive")
	}
	body, err := domainevidence.PrivateAcceptedFinalRecordBytes(lease.record)
	if err != nil {
		return domainevidence.PrivateAcceptedFinalRecord{}, err
	}
	return domainevidence.ParsePrivateAcceptedFinalRecord(body)
}
func (lease *recoveredFactCapability) UseExact(record domainevidence.PrivateAcceptedFinalRecord, use func() error) error {
	lease.mu.RLock()
	defer lease.mu.RUnlock()
	if !lease.active || use == nil || !reflect.DeepEqual(record, lease.record) {
		return errors.New("recovered fact authority is not exact")
	}
	return lease.service.withVerifiedFactFinalWitness(lease.ctx, lease.record, use)
}
func (*recoveredFactCapability) MarshalJSON() ([]byte, error) {
	return nil, errors.New("recovered fact authority is not serializable")
}
