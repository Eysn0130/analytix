package jobs

import (
	"context"
	"errors"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

// ValidateChildRunSemanticSourceV1 performs the constructor's complete source
// validation on an isolated value. It never writes, returns a normalized
// record, backfills an identity, or recovers a completion capability.
func ValidateChildRunSemanticSourceV1(ctx context.Context, record Record, verifier ChildCompletionReceiptVerifier) error {
	if ctx == nil {
		return errors.New("child-run semantic validation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.ID != strings.TrimSpace(record.ID) || !validPersistedJobID(record.ID) {
		return errors.New("child-run semantic record identity is invalid")
	}
	if err := domainjob.ValidateStatusV1(record.Status); err != nil {
		return err
	}
	if err := domainjob.ValidateSemanticMigrationSourceV1(record); err != nil {
		return err
	}
	record = domainjob.NormalizePersistedRecordV1(record)
	if status := strings.TrimSpace(record.PauseState.Status); status != "" {
		if err := domainjob.ValidatePauseRequestStatusV1(status); err != nil {
			return err
		}
	}
	record.PauseState, record.SteerState = childRunPauseState(record), childRunState(record)
	if err := validateDurableChildRunRecord(ctx, record, verifier); err != nil {
		return err
	}
	return ctx.Err()
}

// ValidateCommittedChildRunRecordV1 validates an already projected record with
// the same operational and completion owners used by the live constructor.
// The caller separately proves its raw bytes, address and physical authority.
func ValidateCommittedChildRunRecordV1(ctx context.Context, record Record, verifier ChildCompletionReceiptVerifier) error {
	if ctx == nil {
		return errors.New("child-run committed validation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.ID != strings.TrimSpace(record.ID) || !validPersistedJobID(record.ID) || record.ChildSeq <= 0 {
		return errors.New("child-run committed identity is invalid")
	}
	if err := validateDurableChildRunRecord(ctx, record, verifier); err != nil {
		return err
	}
	if err := domainjob.ValidatePersistedProjectionV1(record); err != nil {
		return err
	}
	return ctx.Err()
}
