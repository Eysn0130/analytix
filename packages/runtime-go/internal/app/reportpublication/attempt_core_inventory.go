package reportpublication

import (
	"context"
	"errors"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

var ErrAttemptCoreLinkageV1 = errors.New("report publication attempt lost its trusted report stage")

// VerifyAttemptCoreInventoryV1 checks every trusted attempt against the complete
// current-key pending inventory, including terminal and expired work. It accepts
// reserved crash cuts without inventing missing publication suffix records.
// The caller must first validate the full optional owner closure and sandwich
// this observation with revalidation of its original Core snapshot.
func VerifyAttemptCoreInventoryV1(ctx context.Context, pending pendingworkapp.TrustedInventoryV1, attempts publicationport.AttemptInventoryStore, authority finalauthorityport.Verifier) error {
	if ctx == nil || attempts == nil {
		return errors.New("report publication Core attempt inventory is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	validator, err := newAttemptCoreValidatorV1(pending, authority)
	if err != nil {
		return err
	}
	if err := attempts.VisitAttempts(ctx, func(attempt domainpublication.PublicationAttemptV1) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := validator.validate(attempt)
		return err
	}); err != nil {
		return err
	}
	return ctx.Err()
}

type attemptCoreValidatorV1 struct {
	stages    map[string]domainpendingwork.PendingWorkReceiptV1
	seen      map[string]bool
	keyID     string
	publicKey []byte
}

func newAttemptCoreValidatorV1(pending pendingworkapp.TrustedInventoryV1, authority finalauthorityport.Verifier) (*attemptCoreValidatorV1, error) {
	validator := &attemptCoreValidatorV1{
		stages: make(map[string]domainpendingwork.PendingWorkReceiptV1, len(pending.Receipts)),
		seen:   map[string]bool{},
	}
	for _, receipt := range pending.Receipts {
		if _, duplicate := validator.stages[receipt.WorkID]; duplicate {
			return nil, errors.New("report publication restart pending inventory repeats work")
		}
		validator.stages[receipt.WorkID] = receipt
	}
	if authority != nil {
		validator.keyID = authority.KeyID()
		validator.publicKey = append([]byte(nil), authority.PublicKey()...)
	}
	return validator, nil
}

func (validator *attemptCoreValidatorV1) validate(attempt domainpublication.PublicationAttemptV1) (domainpendingwork.PendingWorkReceiptV1, error) {
	if validator.seen[attempt.AttemptID] {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("report publication restart inventory repeats an attempt")
	}
	validator.seen[attempt.AttemptID] = true
	stage, found := validator.stages[attempt.ReportStageWorkID]
	if !found || domainpublication.ValidatePublicationAttemptForInstallationV1(
		attempt, attempt.InstallationID, attempt.EnrollmentID, validator.keyID, validator.publicKey,
	) != nil || domainpublication.ValidatePublicationAttemptStageV1(attempt, stage) != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrAttemptCoreLinkageV1
	}
	return stage, nil
}
