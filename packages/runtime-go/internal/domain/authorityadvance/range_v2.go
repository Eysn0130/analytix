package authorityadvance

import (
	"encoding/base64"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// MonotonicAdvanceRangeReferenceV2 makes both referenced wire versions
// explicit. This release admits only all-V2 ranges; unresolved V1 intents must
// be reconciled before V2 writer activation rather than silently re-signed.
type MonotonicAdvanceRangeReferenceV2 struct {
	IntentSchemaVersion      int           `json:"intentSchemaVersion"`
	SettlementSchemaVersion  int           `json:"settlementSchemaVersion"`
	Root                     AdvanceRootV2 `json:"root"`
	MutationID               string        `json:"mutationId"`
	IntentDigest             string        `json:"intentDigest"`
	RequestDigest            string        `json:"requestDigest"`
	ReceiptDigest            string        `json:"receiptDigest"`
	SettlementDigest         string        `json:"settlementDigest"`
	PreviousCheckpointDigest string        `json:"previousCheckpointDigest"`
	NextCheckpointDigest     string        `json:"nextCheckpointDigest"`
}

type MonotonicAdvanceCommittedStepV2 struct {
	Intent     MonotonicAdvanceIntentV2     `json:"intent"`
	Settlement MonotonicAdvanceSettlementV2 `json:"settlement"`
}

func ValidateMonotonicAdvanceRangeReferenceV2(reference MonotonicAdvanceRangeReferenceV2) error {
	if reference.IntentSchemaVersion != MonotonicAdvanceIntentSchemaVersionV2 ||
		reference.SettlementSchemaVersion != MonotonicAdvanceSettlementSchemaVersionV2 ||
		ValidateAdvanceRootV2(reference.Root) != nil || !canonicalSHA256V1(reference.MutationID) ||
		!canonicalSHA256V1(reference.IntentDigest) || !canonicalSHA256V1(reference.RequestDigest) ||
		!canonicalSHA256V1(reference.ReceiptDigest) || !canonicalSHA256V1(reference.SettlementDigest) ||
		!canonicalSHA256V1(reference.PreviousCheckpointDigest) ||
		!canonicalSHA256V1(reference.NextCheckpointDigest) {
		return errors.New("monotonic advance V2 range reference is invalid")
	}
	return nil
}

func ValidateMonotonicAdvanceCommittedRangeV2(
	anchor domainsecurity.MonotonicHeadCheckpointV1,
	steps []MonotonicAdvanceCommittedStepV2,
	tail domainsecurity.MonotonicHeadCheckpointV1,
) error {
	if domainsecurity.ValidateMonotonicHeadCheckpointV1(anchor) != nil ||
		domainsecurity.ValidateMonotonicHeadCheckpointV1(tail) != nil {
		return errors.New("monotonic advance V2 committed range checkpoint is invalid")
	}
	if len(steps) == 0 || len(steps) > MaxMonotonicAdvanceRangeStepsV2 {
		return errors.New("monotonic advance V2 committed range length is invalid")
	}
	current := anchor
	seenMutations := make(map[string]struct{}, len(steps))
	seenIntents := make(map[string]struct{}, len(steps))
	seenSettlements := make(map[string]struct{}, len(steps))
	totalCanonicalBytes := 0
	for _, step := range steps {
		if ValidateMonotonicAdvanceIntentForPreviousCheckpointV2(step.Intent, current) != nil ||
			ValidateMonotonicAdvanceSettlementForIntentV2(step.Settlement, step.Intent, nil) != nil ||
			step.Settlement.Kind != MonotonicAdvanceSettlementCommittedV2 {
			return errors.New("monotonic advance V2 committed range step is invalid")
		}
		intentBody, intentErr := MonotonicAdvanceIntentV2Bytes(step.Intent)
		settlementBody, settlementErr := MonotonicAdvanceSettlementV2Bytes(step.Settlement)
		stepBytes := len(intentBody) + len(settlementBody)
		if intentErr != nil || settlementErr != nil || stepBytes < len(intentBody) ||
			totalCanonicalBytes > MaxMonotonicAdvanceCommittedRangeBytesV2-stepBytes {
			return errors.New("monotonic advance V2 committed range exceeds its canonical byte budget")
		}
		totalCanonicalBytes += stepBytes
		next := step.Settlement.Committed.Receipt.Checkpoint
		if domainsecurity.ValidateMonotonicHeadCheckpointDirectSuccessorV1(current, next) != nil {
			return errors.New("monotonic advance V2 committed range continuity is invalid")
		}
		if _, exists := seenMutations[step.Intent.MutationID]; exists {
			return errors.New("monotonic advance V2 committed range mutation is duplicated")
		}
		if _, exists := seenIntents[step.Intent.RecordDigest]; exists {
			return errors.New("monotonic advance V2 committed range intent is duplicated")
		}
		if _, exists := seenSettlements[step.Settlement.RecordDigest]; exists {
			return errors.New("monotonic advance V2 committed range settlement is duplicated")
		}
		seenMutations[step.Intent.MutationID] = struct{}{}
		seenIntents[step.Intent.RecordDigest] = struct{}{}
		seenSettlements[step.Settlement.RecordDigest] = struct{}{}
		current = next
	}
	if current != tail {
		return errors.New("monotonic advance V2 committed range tail is not exact")
	}
	return nil
}

func NewMonotonicAdvanceRangeReferencesV2(
	anchor domainsecurity.MonotonicHeadCheckpointV1,
	steps []MonotonicAdvanceCommittedStepV2,
	tail domainsecurity.MonotonicHeadCheckpointV1,
) ([]MonotonicAdvanceRangeReferenceV2, error) {
	if err := ValidateMonotonicAdvanceCommittedRangeV2(anchor, steps, tail); err != nil {
		return nil, err
	}
	references := make([]MonotonicAdvanceRangeReferenceV2, len(steps))
	for index, step := range steps {
		receipt := step.Settlement.Committed.Receipt
		references[index] = MonotonicAdvanceRangeReferenceV2{
			IntentSchemaVersion:      MonotonicAdvanceIntentSchemaVersionV2,
			SettlementSchemaVersion:  MonotonicAdvanceSettlementSchemaVersionV2,
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

func ValidateMonotonicAdvanceSupersededSettlementV2(
	settlement MonotonicAdvanceSettlementV2,
	intent MonotonicAdvanceIntentV2,
	committedRange []MonotonicAdvanceCommittedStepV2,
) error {
	if err := ValidateMonotonicAdvanceSettlementV2(settlement); err != nil {
		return err
	}
	if err := ValidateMonotonicAdvanceIntentV2(intent); err != nil {
		return err
	}
	if settlement.Kind != MonotonicAdvanceSettlementSupersededV2 || settlement.Superseded == nil ||
		settlement.Committed != nil {
		return errors.New("monotonic advance V2 settlement is not superseded")
	}
	if settlement.InstallationID != intent.InstallationID || settlement.EnrollmentID != intent.EnrollmentID ||
		settlement.Namespace != intent.Namespace || settlement.Root != intent.Root ||
		settlement.MutationID != intent.MutationID || settlement.IntentDigest != intent.RecordDigest ||
		settlement.RequestDigest != intent.RequestDigest || settlement.AuthorityAlgorithm != intent.AuthorityAlgorithm ||
		settlement.AuthorityKeyID != intent.AuthorityKeyID || settlement.AuthorityPublicKey != intent.AuthorityPublicKey {
		return errors.New("superseded monotonic advance V2 settlement intent binding mismatch")
	}
	superseded := settlement.Superseded
	authorityKey, authorityErr := decodeCanonicalPublicKeyV1(intent.AuthorityPublicKey)
	witnessKey, witnessErr := base64.RawURLEncoding.DecodeString(intent.PreviousCheckpoint.WitnessPublicKey)
	if authorityErr != nil || witnessErr != nil ||
		base64.RawURLEncoding.EncodeToString(witnessKey) != intent.PreviousCheckpoint.WitnessPublicKey ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			superseded.Observation,
			superseded.ObserveRequest,
			intent.InstallationID,
			intent.AuthorityKeyID,
			authorityKey,
			intent.EnrollmentID,
			intent.PreviousCheckpoint.WitnessKeyID,
			witnessKey,
		) != nil {
		return errors.New("superseded monotonic advance V2 observation is not challenge-bound")
	}
	if superseded.Observation.Checkpoint.Generation < intent.AdvanceRequest.NextGeneration ||
		ValidateMonotonicAdvanceCommittedRangeV2(
			intent.PreviousCheckpoint, committedRange, superseded.Observation.Checkpoint,
		) != nil {
		return errors.New("superseded monotonic advance V2 range is incomplete")
	}
	first := committedRange[0].Intent
	if first.AdvanceRequest.NextGeneration != intent.AdvanceRequest.NextGeneration ||
		first.AdvanceRequest.NextStateDigest == intent.AdvanceRequest.NextStateDigest {
		return errors.New("superseded monotonic advance V2 range does not exclude the candidate")
	}
	for _, step := range committedRange {
		if step.Intent.MutationID == intent.MutationID || step.Intent.RequestDigest == intent.RequestDigest {
			return errors.New("superseded monotonic advance V2 range contains the candidate")
		}
	}
	expected, err := NewMonotonicAdvanceRangeReferencesV2(
		intent.PreviousCheckpoint, committedRange, superseded.Observation.Checkpoint,
	)
	if err != nil || len(expected) != len(superseded.RangeReferences) {
		return errors.New("superseded monotonic advance V2 references are incomplete")
	}
	for index := range expected {
		if expected[index] != superseded.RangeReferences[index] {
			return errors.New("superseded monotonic advance V2 reference is not exact")
		}
	}
	return nil
}
