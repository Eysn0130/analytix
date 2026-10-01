package gateprojection

import (
	"context"
	"errors"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type factRecoveryFailure struct {
	phase string
	cause error
}

func (failure *factRecoveryFailure) Error() string {
	return "fact recovery " + failure.phase + " unavailable"
}
func (failure *factRecoveryFailure) Unwrap() error { return failure.cause }

// FactRecoveryFailurePhase exposes only static stage labels, never the cause.
func FactRecoveryFailurePhase(err error) string {
	var failure *factRecoveryFailure
	if errors.As(err, &failure) {
		return failure.phase
	}
	return "batch"
}

// RestoreFactTerminalBatch admits one thread's complete original terminals.
// It has no durable writes, event delivery or provider execution. The staging
// index is private until every original in the thread has passed fresh witness,
// exact I/C/P/Daf/Dt and durable manifest checks. A failure publishes nothing.
// The runtime calls this before serving requests or starting live turn writers.
func (index *TrustedFinalProjectionIndex) RestoreFactTerminalBatch(
	ctx context.Context,
	authorities []TerminalCompleteFinalAuthorityV1,
	witness registryport.RecoveredFactFinalWitnessIssuer,
	validateCurrent func(context.Context, domainevidence.PrivateAcceptedFinalRecord) error,
	loadEvents func(context.Context, domainevidence.PrivateAcceptedFinalRecord, appturn.FactFinalMutationAuthority) ([]map[string]any, error),
) error {
	if index == nil || index.authority == nil || index.readback == nil || witness == nil || validateCurrent == nil || loadEvents == nil || len(authorities) == 0 {
		return errors.New("fact startup batch dependencies are unavailable")
	}
	staged := NewTrustedFinalProjectionIndexWithReadback(index.authority, index.readback)
	threadID := authorities[0].PrivateFinal.SecurityContext.ThreadID
	seen := make(map[string]bool, len(authorities))
	for _, original := range authorities {
		record := original.PrivateFinal
		key := trustedFinalProjectionKey(record.SecurityContext.ThreadID, record.SecurityContext.TurnID)
		if seen[key] || record.SecurityContext.ThreadID != threadID || !domainevidence.FinalAnswerRequiresPublicationSnapshotProof(record.Envelope) || original.FactAuthority != nil {
			return errors.New("fact startup batch scope is invalid")
		}
		seen[key] = true
		phase := "witness"
		err := witness.WithRecoveredFactFinalWitness(ctx, record, func(cap registryport.FactFinalWitnessCapability) error {
			authority := original
			authority.FactAuthority = &currentRecoveryAuthority{ctx: ctx, witness: cap, validate: validateCurrent}
			phase = "currentness"
			if err := authority.FactAuthority.UseExact(record, func() error { return nil }); err != nil {
				return err
			}
			phase = "events"
			events, err := loadEvents(ctx, record, authority.FactAuthority)
			if err != nil {
				return err
			}
			phase = "manifest"
			if err := index.readback.VerifyCommittedManifest(ctx, events); err != nil {
				return err
			}
			phase = "stage"
			lease, err := staged.StageTerminalComplete(ctx, authority)
			if err != nil {
				return err
			}
			defer lease.Discard()
			phase = "activation"
			return lease.Activate()
		})
		if err != nil {
			return &factRecoveryFailure{phase: phase, cause: err}
		}
	}
	// Hold visibility for the whole batch. Admission is re-challenged inside
	// each insertion, not copied from an expired staging capability. On failure
	// remove only this batch's new entries before any reader can observe them.
	index.mu.Lock()
	defer index.mu.Unlock()
	for key, entry := range staged.records {
		if _, present := index.records[key]; present || !entry.published {
			return errors.New("fact startup batch conflicts with admitted history")
		}
	}
	inserted := make([]string, 0, len(staged.records))
	for _, original := range authorities {
		record := original.PrivateFinal
		key := trustedFinalProjectionKey(record.SecurityContext.ThreadID, record.SecurityContext.TurnID)
		err := witness.WithRecoveredFactFinalWitness(ctx, record, func(cap registryport.FactFinalWitnessCapability) error {
			current := &currentRecoveryAuthority{ctx: ctx, witness: cap, validate: validateCurrent}
			return current.UseExact(record, func() error {
				entry := staged.records[key]
				index.nextLease++
				entry.leaseGeneration = index.nextLease
				index.records[key] = entry
				inserted = append(inserted, key)
				return nil
			})
		})
		if err != nil {
			for _, key := range inserted {
				delete(index.records, key)
			}
			return &factRecoveryFailure{phase: "publication", cause: err}
		}
	}
	return nil
}

type currentRecoveryAuthority struct {
	ctx      context.Context
	witness  registryport.FactFinalWitnessCapability
	validate func(context.Context, domainevidence.PrivateAcceptedFinalRecord) error
}

func (authority *currentRecoveryAuthority) UseExact(record domainevidence.PrivateAcceptedFinalRecord, use func() error) error {
	return authority.witness.UseExact(record, func() error {
		if err := authority.validate(authority.ctx, record); err != nil {
			return err
		}
		return use()
	})
}
