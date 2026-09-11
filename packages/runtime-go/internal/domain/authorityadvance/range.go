package authorityadvance

import (
	"encoding/base64"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// MonotonicAdvanceRangeReferenceV1 is only an immutable pointer into a
// committed range. It never proves supersession without the referenced exact
// intent, settlement, request, receipt, and contiguous checkpoint chain.
type MonotonicAdvanceRangeReferenceV1 struct {
	Root                     AdvanceRootV1 `json:"root"`
	MutationID               string        `json:"mutationId"`
	IntentDigest             string        `json:"intentDigest"`
	RequestDigest            string        `json:"requestDigest"`
	ReceiptDigest            string        `json:"receiptDigest"`
	SettlementDigest         string        `json:"settlementDigest"`
	PreviousCheckpointDigest string        `json:"previousCheckpointDigest"`
	NextCheckpointDigest     string        `json:"nextCheckpointDigest"`
}

type MonotonicAdvanceCommittedStepV1 struct {
	Intent     MonotonicAdvanceIntentV1     `json:"intent"`
	Settlement MonotonicAdvanceSettlementV1 `json:"settlement"`
}

func ValidateMonotonicAdvanceRangeReferenceV1(reference MonotonicAdvanceRangeReferenceV1) error {
	if ValidateAdvanceRootV1(reference.Root) != nil || !canonicalSHA256V1(reference.MutationID) ||
		!canonicalSHA256V1(reference.IntentDigest) || !canonicalSHA256V1(reference.RequestDigest) ||
		!canonicalSHA256V1(reference.ReceiptDigest) || !canonicalSHA256V1(reference.SettlementDigest) ||
		!canonicalSHA256V1(reference.PreviousCheckpointDigest) || !canonicalSHA256V1(reference.NextCheckpointDigest) {
		return errors.New("monotonic advance range reference is invalid")
	}
	return nil
}

// ValidateMonotonicAdvanceCommittedRangeV1 proves every checkpoint/request/
// receipt step, exact previous-checkpoint continuity, and exact final tail.
// Empty, truncated, reordered, duplicate, and cross-namespace ranges fail.
func ValidateMonotonicAdvanceCommittedRangeV1(
	anchor domainsecurity.MonotonicHeadCheckpointV1,
	steps []MonotonicAdvanceCommittedStepV1,
	tail domainsecurity.MonotonicHeadCheckpointV1,
) error {
	if domainsecurity.ValidateMonotonicHeadCheckpointV1(anchor) != nil ||
		domainsecurity.ValidateMonotonicHeadCheckpointV1(tail) != nil {
		return errors.New("monotonic advance committed range checkpoint is invalid")
	}
	if len(steps) == 0 || len(steps) > maxMonotonicAdvanceRangeStepsV1 {
		return errors.New("monotonic advance committed range length is invalid")
	}
	current := anchor
	seenMutations := make(map[string]struct{}, len(steps))
	seenIntents := make(map[string]struct{}, len(steps))
	seenSettlements := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		if err := ValidateMonotonicAdvanceIntentForPreviousCheckpointV1(step.Intent, current); err != nil {
			return errors.New("monotonic advance committed range intent at step is invalid")
		}
		if err := ValidateMonotonicAdvanceSettlementForIntentV1(step.Settlement, step.Intent, nil); err != nil {
			return errors.New("monotonic advance committed range settlement at step is invalid")
		}
		if step.Settlement.Kind != MonotonicAdvanceSettlementCommittedV1 {
			return errors.New("monotonic advance committed range contains a non-committed settlement")
		}
		next := step.Settlement.Committed.Receipt.Checkpoint
		if err := domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(current, next); err != nil {
			return errors.New("monotonic advance committed range checkpoint continuity is invalid")
		}
		if _, exists := seenMutations[step.Intent.MutationID]; exists {
			return errors.New("monotonic advance committed range mutation is duplicated")
		}
		if _, exists := seenIntents[step.Intent.RecordDigest]; exists {
			return errors.New("monotonic advance committed range intent is duplicated")
		}
		if _, exists := seenSettlements[step.Settlement.RecordDigest]; exists {
			return errors.New("monotonic advance committed range settlement is duplicated")
		}
		seenMutations[step.Intent.MutationID] = struct{}{}
		seenIntents[step.Intent.RecordDigest] = struct{}{}
		seenSettlements[step.Settlement.RecordDigest] = struct{}{}
		current = next
	}
	if current != tail {
		return errors.New("monotonic advance committed range tail is not exact")
	}
	return nil
}

func NewMonotonicAdvanceRangeReferencesV1(
	anchor domainsecurity.MonotonicHeadCheckpointV1,
	steps []MonotonicAdvanceCommittedStepV1,
	tail domainsecurity.MonotonicHeadCheckpointV1,
) ([]MonotonicAdvanceRangeReferenceV1, error) {
	if err := ValidateMonotonicAdvanceCommittedRangeV1(anchor, steps, tail); err != nil {
		return nil, err
	}
	references := make([]MonotonicAdvanceRangeReferenceV1, len(steps))
	for index, step := range steps {
		receipt := step.Settlement.Committed.Receipt
		references[index] = MonotonicAdvanceRangeReferenceV1{
			Root:                     step.Intent.Root,
			MutationID:               step.Intent.MutationID,
			IntentDigest:             step.Intent.RecordDigest,
			RequestDigest:            step.Intent.RequestDigest,
			ReceiptDigest:            receipt.ReceiptDigest,
			SettlementDigest:         step.Settlement.RecordDigest,
			PreviousCheckpointDigest: step.Intent.PreviousCheckpoint.CheckpointDigest,
			NextCheckpointDigest:     receipt.Checkpoint.CheckpointDigest,
		}
	}
	return references, nil
}

// ValidateMonotonicAdvanceSupersededSettlementV1 requires a newly challenged
// witness observation plus the complete committed range from the losing
// intent's exact previous checkpoint to that observation. It rejects a range
// whose first committed candidate equals the losing next state.
func ValidateMonotonicAdvanceSupersededSettlementV1(
	settlement MonotonicAdvanceSettlementV1,
	intent MonotonicAdvanceIntentV1,
	committedRange []MonotonicAdvanceCommittedStepV1,
) error {
	if err := ValidateMonotonicAdvanceSettlementV1(settlement); err != nil {
		return err
	}
	if err := ValidateMonotonicAdvanceIntentV1(intent); err != nil {
		return err
	}
	if settlement.Kind != MonotonicAdvanceSettlementSupersededV1 || settlement.Superseded == nil || settlement.Committed != nil {
		return errors.New("monotonic advance settlement is not superseded")
	}
	if settlement.InstallationID != intent.InstallationID || settlement.EnrollmentID != intent.EnrollmentID ||
		settlement.Namespace != intent.Namespace || settlement.Root != intent.Root ||
		settlement.MutationID != intent.MutationID || settlement.IntentDigest != intent.RecordDigest ||
		settlement.RequestDigest != intent.RequestDigest || settlement.AuthorityAlgorithm != intent.AuthorityAlgorithm ||
		settlement.AuthorityKeyID != intent.AuthorityKeyID || settlement.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("superseded monotonic advance settlement intent binding mismatch")
	}
	superseded := settlement.Superseded
	authorityPublicKey, authorityErr := decodeCanonicalPublicKeyV1(intent.AuthorityPublicKey)
	witnessPublicKey, witnessErr := base64.RawURLEncoding.DecodeString(intent.PreviousCheckpoint.WitnessPublicKey)
	if authorityErr != nil || witnessErr != nil ||
		base64.RawURLEncoding.EncodeToString(witnessPublicKey) != intent.PreviousCheckpoint.WitnessPublicKey {
		return errors.New("superseded monotonic advance authority key is invalid")
	}
	if err := domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
		superseded.Observation,
		superseded.ObserveRequest,
		intent.InstallationID,
		intent.AuthorityKeyID,
		authorityPublicKey,
		intent.EnrollmentID,
		intent.PreviousCheckpoint.WitnessKeyID,
		witnessPublicKey,
	); err != nil {
		return errors.New("superseded monotonic advance observation is not freshly bound")
	}
	if superseded.Observation.Checkpoint.Generation < intent.AdvanceRequest.NextGeneration {
		return errors.New("superseded monotonic advance observation does not pass candidate generation")
	}
	if err := ValidateMonotonicAdvanceCommittedRangeV1(
		intent.PreviousCheckpoint, committedRange, superseded.Observation.Checkpoint,
	); err != nil {
		return err
	}
	first := committedRange[0].Intent
	if first.AdvanceRequest.NextGeneration != intent.AdvanceRequest.NextGeneration ||
		first.AdvanceRequest.NextStateDigest == intent.AdvanceRequest.NextStateDigest {
		return errors.New("superseded monotonic advance range does not exclude the candidate state")
	}
	for _, step := range committedRange {
		if step.Intent.MutationID == intent.MutationID || step.Intent.RequestDigest == intent.RequestDigest {
			return errors.New("superseded monotonic advance range contains the candidate request")
		}
	}
	expectedReferences, err := NewMonotonicAdvanceRangeReferencesV1(
		intent.PreviousCheckpoint, committedRange, superseded.Observation.Checkpoint,
	)
	if err != nil {
		return err
	}
	if len(expectedReferences) != len(superseded.RangeReferences) {
		return errors.New("superseded monotonic advance range references are incomplete")
	}
	for index := range expectedReferences {
		if expectedReferences[index] != superseded.RangeReferences[index] {
			return errors.New("superseded monotonic advance range reference is not exact")
		}
	}
	return nil
}
