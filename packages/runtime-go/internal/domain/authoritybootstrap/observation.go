package authoritybootstrap

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func NewUnpreparedObservationV1(
	anchored AnchoredBootstrapBindingV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	sign WitnessSignFuncV1,
) (BootstrapObservationV1, error) {
	return newBootstrapObservationV1(anchored, PhaseUnprepared, request, observation, "", "", "", sign)
}

func NewPreparedObservationV1(
	anchored AnchoredBootstrapBindingV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	prepare BootstrapPrepareReceiptV1,
	sign WitnessSignFuncV1,
) (BootstrapObservationV1, error) {
	if err := ValidateBootstrapPrepareReceiptForBindingV1(prepare, anchored); err != nil ||
		!equalCheckpointV1(observation.Checkpoint, prepare.AdvanceReceipt.Checkpoint) {
		return BootstrapObservationV1{}, errors.Join(ErrTransitionConflict, err)
	}
	return newBootstrapObservationV1(
		anchored, PhasePrepared, request, observation, prepare.AdvanceRequest.MutationID, prepare.ReceiptDigest, "", sign,
	)
}

func NewCommittedObservationV1(
	anchored AnchoredBootstrapBindingV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	observation domainsecurity.MonotonicHeadObservationV1,
	prepare BootstrapPrepareReceiptV1,
	commit BootstrapCommitReceiptV1,
	sign WitnessSignFuncV1,
) (BootstrapObservationV1, error) {
	if err := ValidateBootstrapCommitReceiptForPrepareV1(commit, prepare, anchored); err != nil ||
		!equalCheckpointV1(observation.Checkpoint, commit.AdvanceReceipt.Checkpoint) {
		return BootstrapObservationV1{}, errors.Join(ErrTransitionConflict, err)
	}
	return newBootstrapObservationV1(
		anchored, PhaseCommitted, request, observation, prepare.AdvanceRequest.MutationID,
		prepare.ReceiptDigest, commit.ReceiptDigest, sign,
	)
}

func newBootstrapObservationV1(
	anchored AnchoredBootstrapBindingV1,
	phase BootstrapPhaseV1,
	request domainsecurity.MonotonicHeadObserveRequestV1,
	monotonicObservation domainsecurity.MonotonicHeadObservationV1,
	prepareMutationID, prepareReceiptDigest, commitReceiptDigest string,
	sign WitnessSignFuncV1,
) (BootstrapObservationV1, error) {
	binding, err := anchored.BindingV1()
	if err != nil {
		return BootstrapObservationV1{}, err
	}
	observation := BootstrapObservationV1{
		SchemaVersion:        SchemaVersionV1,
		Purpose:              ObservationPurposeV1,
		BindingDigest:        binding.BindingDigest,
		Phase:                phase,
		PrepareMutationID:    prepareMutationID,
		PrepareReceiptDigest: prepareReceiptDigest,
		CommitReceiptDigest:  commitReceiptDigest,
		ObserveRequest:       request,
		MonotonicObservation: monotonicObservation,
		WitnessAlgorithm:     AlgorithmV1,
		WitnessKeyID:         binding.WitnessKeyID,
		WitnessPublicKey:     binding.WitnessPublicKey,
	}
	if sign == nil || validateObservationForAnchoredBindingV1(observation, anchored) != nil {
		return BootstrapObservationV1{}, ErrInvalidContract
	}
	signature, err := sign(BootstrapObservationSigningBytesV1(observation))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return BootstrapObservationV1{}, errors.Join(ErrInvalidContract, errors.New("authority bootstrap observation signing failed"))
	}
	observation.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	observation.ObservationDigest = observationDigestV1(observation)
	if err := ValidateBootstrapObservationV1(observation); err != nil {
		return BootstrapObservationV1{}, err
	}
	return observation, nil
}

// ValidateBootstrapObservationV1 verifies self-contained authenticity. It
// cannot establish manifest anchoring or caller freshness by itself.
func ValidateBootstrapObservationV1(observation BootstrapObservationV1) error {
	if observation.SchemaVersion != SchemaVersionV1 || observation.Purpose != ObservationPurposeV1 ||
		!canonicalDigest(observation.BindingDigest) || !validBootstrapPhaseV1(observation.Phase) ||
		domainsecurity.ValidateMonotonicHeadObserveRequestV1(observation.ObserveRequest) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationV1(observation.MonotonicObservation) != nil ||
		observation.MonotonicObservation.RequestDigest != observation.ObserveRequest.RequestDigest ||
		observation.MonotonicObservation.ChallengeNonce != observation.ObserveRequest.ChallengeNonce ||
		observation.WitnessAlgorithm != AlgorithmV1 || !canonicalDigest(observation.WitnessKeyID) {
		return ErrInvalidContract
	}
	switch observation.Phase {
	case PhaseUnprepared:
		if observation.PrepareMutationID != "" || observation.PrepareReceiptDigest != "" || observation.CommitReceiptDigest != "" ||
			observation.MonotonicObservation.Checkpoint.Generation != 0 {
			return ErrInvalidContract
		}
	case PhasePrepared:
		if !canonicalDigest(observation.PrepareMutationID) || !canonicalDigest(observation.PrepareReceiptDigest) || observation.CommitReceiptDigest != "" ||
			observation.MonotonicObservation.Checkpoint.Generation != 1 {
			return ErrInvalidContract
		}
	case PhaseCommitted:
		if !canonicalDigest(observation.PrepareMutationID) || !canonicalDigest(observation.PrepareReceiptDigest) || !canonicalDigest(observation.CommitReceiptDigest) ||
			observation.MonotonicObservation.Checkpoint.Generation != 2 {
			return ErrInvalidContract
		}
	default:
		return ErrInvalidContract
	}
	if err := verifySignedV1(
		observation.WitnessAlgorithm,
		observation.WitnessKeyID,
		observation.WitnessPublicKey,
		observation.WitnessSignature,
		BootstrapObservationSigningBytesV1(observation),
	); err != nil || !canonicalDigest(observation.ObservationDigest) ||
		observation.ObservationDigest != observationDigestV1(observation) {
		return errors.Join(ErrInvalidContract, err)
	}
	return nil
}

func validateObservationForAnchoredBindingV1(observation BootstrapObservationV1, anchored AnchoredBootstrapBindingV1) error {
	binding, err := anchored.BindingV1()
	if err != nil || validateBootstrapObservationUnsignedOuterV1(observation) != nil {
		return errors.Join(ErrAnchorMismatch, err)
	}
	installationPublicKey, installErr := decodeCanonicalPublicKeyV1(
		binding.InstallationAuthorityPublicKey, binding.InstallationAuthorityKeyID,
	)
	witnessPublicKey, witnessErr := decodeCanonicalPublicKeyV1(binding.WitnessPublicKey, binding.WitnessKeyID)
	if installErr != nil || witnessErr != nil || observation.BindingDigest != binding.BindingDigest ||
		observation.WitnessKeyID != binding.WitnessKeyID || observation.WitnessPublicKey != binding.WitnessPublicKey ||
		observation.ObserveRequest.EnrollmentID != binding.BootstrapEnrollmentID ||
		observation.ObserveRequest.Namespace != binding.Namespace ||
		domainsecurity.ValidateMonotonicHeadObservationForRequestV1(
			observation.MonotonicObservation,
			observation.ObserveRequest,
			binding.InstallationID,
			binding.InstallationAuthorityKeyID,
			installationPublicKey,
			binding.BootstrapEnrollmentID,
			binding.WitnessKeyID,
			witnessPublicKey,
		) != nil {
		return ErrAnchorMismatch
	}
	checkpoint := observation.MonotonicObservation.Checkpoint
	switch observation.Phase {
	case PhaseUnprepared:
		if !equalCheckpointV1(checkpoint, binding.BootstrapInitialCheckpoint) ||
			observation.PrepareMutationID != "" || observation.PrepareReceiptDigest != "" || observation.CommitReceiptDigest != "" {
			return ErrTransitionConflict
		}
	case PhasePrepared:
		expectedState, stateErr := PreparedStateDigestV1(anchored, checkpoint.MutationID)
		if stateErr != nil || checkpoint.Generation != 1 || checkpoint.CurrentStateDigest != expectedState ||
			observation.PrepareMutationID != checkpoint.MutationID || !canonicalDigest(observation.PrepareReceiptDigest) || observation.CommitReceiptDigest != "" {
			return ErrTransitionConflict
		}
	case PhaseCommitted:
		expectedState := bootstrapStateDigestV1(
			binding.BindingDigest, binding.BootstrapEnrollmentID, binding.ProjectionSlotID, binding.FloorProjectionDigest,
			PhaseCommitted, observation.PrepareMutationID, observation.PrepareReceiptDigest, checkpoint.MutationID,
		)
		// The exact committed state is fully checked against prepare/commit
		// receipts by ValidateCommittedObservationV1. Generic observation
		// anchoring still rejects generation, receipt, and identity confusion.
		if checkpoint.Generation != 2 || !canonicalDigest(observation.PrepareMutationID) || !canonicalDigest(observation.PrepareReceiptDigest) ||
			!canonicalDigest(observation.CommitReceiptDigest) || checkpoint.CurrentStateDigest != expectedState {
			return ErrTransitionConflict
		}
	default:
		return ErrTransitionConflict
	}
	return nil
}

// validateBootstrapObservationUnsignedOuterV1 validates every embedded
// signed monotonic contract and phase field before the outer signature exists.
func validateBootstrapObservationUnsignedOuterV1(observation BootstrapObservationV1) error {
	copy := observation
	copy.WitnessSignature = ""
	copy.ObservationDigest = ""
	if copy.SchemaVersion != SchemaVersionV1 || copy.Purpose != ObservationPurposeV1 ||
		!canonicalDigest(copy.BindingDigest) || !validBootstrapPhaseV1(copy.Phase) ||
		domainsecurity.ValidateMonotonicHeadObserveRequestV1(copy.ObserveRequest) != nil ||
		domainsecurity.ValidateMonotonicHeadObservationV1(copy.MonotonicObservation) != nil ||
		copy.MonotonicObservation.RequestDigest != copy.ObserveRequest.RequestDigest ||
		copy.MonotonicObservation.ChallengeNonce != copy.ObserveRequest.ChallengeNonce ||
		copy.WitnessAlgorithm != AlgorithmV1 || !canonicalDigest(copy.WitnessKeyID) || copy.WitnessPublicKey == "" {
		return ErrInvalidContract
	}
	return nil
}

func ValidateObservationForBindingV1(
	observation BootstrapObservationV1,
	anchored AnchoredBootstrapBindingV1,
	expectedRequest domainsecurity.MonotonicHeadObserveRequestV1,
) error {
	if err := ValidateBootstrapObservationV1(observation); err != nil {
		return err
	}
	if err := validateObservationForAnchoredBindingV1(observation, anchored); err != nil {
		return err
	}
	if !equalObserveRequestV1(observation.ObserveRequest, expectedRequest) {
		return ErrLiveRevalidation
	}
	return nil
}

// ValidateSameHeadObservationV1 proves that a separately challenged signed
// observation names the exact same accepted head. It detects reset, rollback,
// ABA, and same-generation forks, but it deliberately makes no wall-clock or
// write-cut freshness claim. A production caller must obtain the challenge
// and observation inside its live conditional witness operation.
func ValidateSameHeadObservationV1(
	anchored AnchoredBootstrapBindingV1,
	accepted BootstrapObservationV1,
	current BootstrapObservationV1,
	expectedRequest domainsecurity.MonotonicHeadObserveRequestV1,
) error {
	if ValidateBootstrapObservationV1(accepted) != nil ||
		validateObservationForAnchoredBindingV1(accepted, anchored) != nil ||
		expectedRequest.RequestDigest == accepted.ObserveRequest.RequestDigest ||
		ValidateObservationForBindingV1(current, anchored, expectedRequest) != nil {
		return ErrLiveRevalidation
	}
	if current.Phase != accepted.Phase || current.PrepareMutationID != accepted.PrepareMutationID ||
		current.PrepareReceiptDigest != accepted.PrepareReceiptDigest ||
		current.CommitReceiptDigest != accepted.CommitReceiptDigest ||
		!equalCheckpointV1(current.MonotonicObservation.Checkpoint, accepted.MonotonicObservation.Checkpoint) {
		return ErrTransitionConflict
	}
	return nil
}
