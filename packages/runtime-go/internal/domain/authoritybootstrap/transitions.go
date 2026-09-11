package authoritybootstrap

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func NewPrepareReceiptV1(
	anchored AnchoredBootstrapBindingV1,
	unprepared BootstrapObservationV1,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
	monotonicReceipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
	sign WitnessSignFuncV1,
) (BootstrapPrepareReceiptV1, error) {
	binding, err := anchored.BindingV1()
	if err != nil || unprepared.Phase != PhaseUnprepared ||
		ValidateObservationForBindingV1(unprepared, anchored, unprepared.ObserveRequest) != nil {
		return BootstrapPrepareReceiptV1{}, errors.Join(ErrTransitionConflict, err)
	}
	receipt := BootstrapPrepareReceiptV1{
		SchemaVersion:             SchemaVersionV1,
		Purpose:                   PrepareReceiptPurposeV1,
		BindingDigest:             binding.BindingDigest,
		ExpectedPhase:             PhaseUnprepared,
		NextPhase:                 PhasePrepared,
		ExpectedObservationDigest: unprepared.ObservationDigest,
		AdvanceRequest:            request,
		AdvanceReceipt:            monotonicReceipt,
		WitnessAlgorithm:          AlgorithmV1,
		WitnessKeyID:              binding.WitnessKeyID,
		WitnessPublicKey:          binding.WitnessPublicKey,
	}
	if sign == nil || validatePrepareReceiptForAnchoredBindingV1(receipt, anchored) != nil {
		return BootstrapPrepareReceiptV1{}, ErrInvalidContract
	}
	signature, err := sign(BootstrapPrepareReceiptSigningBytesV1(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return BootstrapPrepareReceiptV1{}, errors.Join(ErrInvalidContract, errors.New("authority bootstrap prepare receipt signing failed"))
	}
	receipt.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.ReceiptDigest = prepareReceiptDigestV1(receipt)
	if err := ValidateBootstrapPrepareReceiptV1(receipt); err != nil {
		return BootstrapPrepareReceiptV1{}, err
	}
	return receipt, nil
}

func ValidateBootstrapPrepareReceiptV1(receipt BootstrapPrepareReceiptV1) error {
	if receipt.SchemaVersion != SchemaVersionV1 || receipt.Purpose != PrepareReceiptPurposeV1 ||
		!canonicalDigest(receipt.BindingDigest) || receipt.ExpectedPhase != PhaseUnprepared || receipt.NextPhase != PhasePrepared ||
		!canonicalDigest(receipt.ExpectedObservationDigest) ||
		domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(receipt.AdvanceRequest) != nil ||
		domainsecurity.ValidateMonotonicHeadAdvanceReceiptV1(receipt.AdvanceReceipt) != nil ||
		receipt.AdvanceRequest.ExpectedGeneration != 0 || receipt.AdvanceRequest.NextGeneration != 1 ||
		receipt.AdvanceReceipt.RequestDigest != receipt.AdvanceRequest.RequestDigest ||
		receipt.AdvanceReceipt.MutationID != receipt.AdvanceRequest.MutationID ||
		receipt.WitnessAlgorithm != AlgorithmV1 || !canonicalDigest(receipt.WitnessKeyID) {
		return ErrInvalidContract
	}
	if err := verifySignedV1(
		receipt.WitnessAlgorithm,
		receipt.WitnessKeyID,
		receipt.WitnessPublicKey,
		receipt.WitnessSignature,
		BootstrapPrepareReceiptSigningBytesV1(receipt),
	); err != nil || !canonicalDigest(receipt.ReceiptDigest) || receipt.ReceiptDigest != prepareReceiptDigestV1(receipt) {
		return errors.Join(ErrInvalidContract, err)
	}
	return nil
}

func ValidateBootstrapPrepareReceiptForBindingV1(
	receipt BootstrapPrepareReceiptV1,
	anchored AnchoredBootstrapBindingV1,
) error {
	if err := ValidateBootstrapPrepareReceiptV1(receipt); err != nil {
		return err
	}
	return validatePrepareReceiptForAnchoredBindingV1(receipt, anchored)
}

func validatePrepareReceiptForAnchoredBindingV1(
	receipt BootstrapPrepareReceiptV1,
	anchored AnchoredBootstrapBindingV1,
) error {
	binding, err := anchored.BindingV1()
	if err != nil {
		return err
	}
	installationPublicKey, installErr := decodeCanonicalPublicKeyV1(
		binding.InstallationAuthorityPublicKey, binding.InstallationAuthorityKeyID,
	)
	witnessPublicKey, witnessErr := decodeCanonicalPublicKeyV1(binding.WitnessPublicKey, binding.WitnessKeyID)
	expectedState, stateErr := PreparedStateDigestV1(anchored, receipt.AdvanceRequest.MutationID)
	if installErr != nil || witnessErr != nil || stateErr != nil || receipt.BindingDigest != binding.BindingDigest ||
		receipt.WitnessKeyID != binding.WitnessKeyID || receipt.WitnessPublicKey != binding.WitnessPublicKey ||
		receipt.AdvanceRequest.EnrollmentID != binding.BootstrapEnrollmentID || receipt.AdvanceRequest.Namespace != binding.Namespace ||
		receipt.AdvanceRequest.NextStateDigest != expectedState ||
		domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
			binding.BootstrapInitialCheckpoint,
			receipt.AdvanceRequest,
			receipt.AdvanceReceipt,
			binding.InstallationID,
			binding.InstallationAuthorityKeyID,
			installationPublicKey,
			binding.BootstrapEnrollmentID,
			binding.WitnessKeyID,
			witnessPublicKey,
		) != nil {
		return ErrTransitionConflict
	}
	return nil
}

func ValidatePrepareTransitionV1(
	anchored AnchoredBootstrapBindingV1,
	unprepared BootstrapObservationV1,
	expectedObserveRequest domainsecurity.MonotonicHeadObserveRequestV1,
	receipt BootstrapPrepareReceiptV1,
) error {
	if err := ValidateObservationForBindingV1(unprepared, anchored, expectedObserveRequest); err != nil {
		return err
	}
	if err := ValidateBootstrapPrepareReceiptForBindingV1(receipt, anchored); err != nil {
		return err
	}
	if unprepared.Phase != PhaseUnprepared || receipt.ExpectedObservationDigest != unprepared.ObservationDigest ||
		!equalCheckpointV1(unprepared.MonotonicObservation.Checkpoint, bindingCheckpointV1(anchored)) ||
		receipt.AdvanceRequest.ExpectedCheckpointDigest != unprepared.MonotonicObservation.Checkpoint.CheckpointDigest ||
		receipt.AdvanceRequest.ExpectedFenceNonce != unprepared.MonotonicObservation.Checkpoint.FenceNonce {
		return ErrTransitionConflict
	}
	return nil
}

func NewCommitReceiptV1(
	anchored AnchoredBootstrapBindingV1,
	prepared BootstrapObservationV1,
	prepare BootstrapPrepareReceiptV1,
	request domainsecurity.MonotonicHeadAdvanceRequestV1,
	monotonicReceipt domainsecurity.MonotonicHeadAdvanceReceiptV1,
	sign WitnessSignFuncV1,
) (BootstrapCommitReceiptV1, error) {
	binding, err := anchored.BindingV1()
	if err != nil || prepared.Phase != PhasePrepared ||
		ValidatePreparedObservationV1(prepared, anchored, prepared.ObserveRequest, prepare) != nil {
		return BootstrapCommitReceiptV1{}, errors.Join(ErrTransitionConflict, err)
	}
	receipt := BootstrapCommitReceiptV1{
		SchemaVersion:             SchemaVersionV1,
		Purpose:                   CommitReceiptPurposeV1,
		BindingDigest:             binding.BindingDigest,
		PrepareReceiptDigest:      prepare.ReceiptDigest,
		ExpectedPhase:             PhasePrepared,
		NextPhase:                 PhaseCommitted,
		ExpectedObservationDigest: prepared.ObservationDigest,
		AdvanceRequest:            request,
		AdvanceReceipt:            monotonicReceipt,
		WitnessAlgorithm:          AlgorithmV1,
		WitnessKeyID:              binding.WitnessKeyID,
		WitnessPublicKey:          binding.WitnessPublicKey,
	}
	if sign == nil || validateCommitReceiptForPrepareV1(receipt, prepare, anchored) != nil {
		return BootstrapCommitReceiptV1{}, ErrInvalidContract
	}
	signature, err := sign(BootstrapCommitReceiptSigningBytesV1(receipt))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return BootstrapCommitReceiptV1{}, errors.Join(ErrInvalidContract, errors.New("authority bootstrap commit receipt signing failed"))
	}
	receipt.WitnessSignature = base64.RawURLEncoding.EncodeToString(signature)
	receipt.ReceiptDigest = commitReceiptDigestV1(receipt)
	if err := ValidateBootstrapCommitReceiptV1(receipt); err != nil {
		return BootstrapCommitReceiptV1{}, err
	}
	return receipt, nil
}

func ValidateBootstrapCommitReceiptV1(receipt BootstrapCommitReceiptV1) error {
	if receipt.SchemaVersion != SchemaVersionV1 || receipt.Purpose != CommitReceiptPurposeV1 ||
		!canonicalDigest(receipt.BindingDigest) || !canonicalDigest(receipt.PrepareReceiptDigest) ||
		receipt.ExpectedPhase != PhasePrepared || receipt.NextPhase != PhaseCommitted ||
		!canonicalDigest(receipt.ExpectedObservationDigest) ||
		domainsecurity.ValidateMonotonicHeadAdvanceRequestV1(receipt.AdvanceRequest) != nil ||
		domainsecurity.ValidateMonotonicHeadAdvanceReceiptV1(receipt.AdvanceReceipt) != nil ||
		receipt.AdvanceRequest.ExpectedGeneration != 1 || receipt.AdvanceRequest.NextGeneration != 2 ||
		receipt.AdvanceReceipt.RequestDigest != receipt.AdvanceRequest.RequestDigest ||
		receipt.AdvanceReceipt.MutationID != receipt.AdvanceRequest.MutationID ||
		receipt.WitnessAlgorithm != AlgorithmV1 || !canonicalDigest(receipt.WitnessKeyID) {
		return ErrInvalidContract
	}
	if err := verifySignedV1(
		receipt.WitnessAlgorithm,
		receipt.WitnessKeyID,
		receipt.WitnessPublicKey,
		receipt.WitnessSignature,
		BootstrapCommitReceiptSigningBytesV1(receipt),
	); err != nil || !canonicalDigest(receipt.ReceiptDigest) || receipt.ReceiptDigest != commitReceiptDigestV1(receipt) {
		return errors.Join(ErrInvalidContract, err)
	}
	return nil
}

func ValidateBootstrapCommitReceiptForPrepareV1(
	receipt BootstrapCommitReceiptV1,
	prepare BootstrapPrepareReceiptV1,
	anchored AnchoredBootstrapBindingV1,
) error {
	if err := ValidateBootstrapCommitReceiptV1(receipt); err != nil {
		return err
	}
	return validateCommitReceiptForPrepareV1(receipt, prepare, anchored)
}

func validateCommitReceiptForPrepareV1(
	receipt BootstrapCommitReceiptV1,
	prepare BootstrapPrepareReceiptV1,
	anchored AnchoredBootstrapBindingV1,
) error {
	binding, err := anchored.BindingV1()
	if err != nil || ValidateBootstrapPrepareReceiptForBindingV1(prepare, anchored) != nil {
		return errors.Join(ErrTransitionConflict, err)
	}
	installationPublicKey, installErr := decodeCanonicalPublicKeyV1(
		binding.InstallationAuthorityPublicKey, binding.InstallationAuthorityKeyID,
	)
	witnessPublicKey, witnessErr := decodeCanonicalPublicKeyV1(binding.WitnessPublicKey, binding.WitnessKeyID)
	expectedState, stateErr := CommittedStateDigestV1(anchored, prepare, receipt.AdvanceRequest.MutationID)
	if installErr != nil || witnessErr != nil || stateErr != nil || receipt.BindingDigest != binding.BindingDigest ||
		receipt.PrepareReceiptDigest != prepare.ReceiptDigest || receipt.WitnessKeyID != binding.WitnessKeyID ||
		receipt.WitnessPublicKey != binding.WitnessPublicKey || receipt.AdvanceRequest.EnrollmentID != binding.BootstrapEnrollmentID ||
		receipt.AdvanceRequest.Namespace != binding.Namespace || receipt.AdvanceRequest.NextStateDigest != expectedState ||
		domainsecurity.ValidateMonotonicHeadAdvanceForAuthoritiesV1(
			prepare.AdvanceReceipt.Checkpoint,
			receipt.AdvanceRequest,
			receipt.AdvanceReceipt,
			binding.InstallationID,
			binding.InstallationAuthorityKeyID,
			installationPublicKey,
			binding.BootstrapEnrollmentID,
			binding.WitnessKeyID,
			witnessPublicKey,
		) != nil {
		return ErrTransitionConflict
	}
	return nil
}

func ValidateCommitTransitionV1(
	anchored AnchoredBootstrapBindingV1,
	prepared BootstrapObservationV1,
	expectedObserveRequest domainsecurity.MonotonicHeadObserveRequestV1,
	prepare BootstrapPrepareReceiptV1,
	commit BootstrapCommitReceiptV1,
) error {
	if err := ValidatePreparedObservationV1(prepared, anchored, expectedObserveRequest, prepare); err != nil {
		return err
	}
	if err := ValidateBootstrapCommitReceiptForPrepareV1(commit, prepare, anchored); err != nil {
		return err
	}
	checkpoint := prepared.MonotonicObservation.Checkpoint
	if commit.ExpectedObservationDigest != prepared.ObservationDigest ||
		!equalCheckpointV1(checkpoint, prepare.AdvanceReceipt.Checkpoint) ||
		commit.AdvanceRequest.ExpectedCheckpointDigest != checkpoint.CheckpointDigest ||
		commit.AdvanceRequest.ExpectedFenceNonce != checkpoint.FenceNonce {
		return ErrTransitionConflict
	}
	return nil
}

func ValidatePreparedObservationV1(
	prepared BootstrapObservationV1,
	anchored AnchoredBootstrapBindingV1,
	expectedObserveRequest domainsecurity.MonotonicHeadObserveRequestV1,
	prepare BootstrapPrepareReceiptV1,
) error {
	if err := ValidateObservationForBindingV1(prepared, anchored, expectedObserveRequest); err != nil {
		return err
	}
	if err := ValidateBootstrapPrepareReceiptForBindingV1(prepare, anchored); err != nil {
		return err
	}
	if prepared.Phase != PhasePrepared || prepared.PrepareMutationID != prepare.AdvanceRequest.MutationID ||
		prepared.PrepareReceiptDigest != prepare.ReceiptDigest || prepared.CommitReceiptDigest != "" ||
		!equalCheckpointV1(prepared.MonotonicObservation.Checkpoint, prepare.AdvanceReceipt.Checkpoint) {
		return ErrTransitionConflict
	}
	return nil
}

func ValidateCommittedObservationV1(
	committed BootstrapObservationV1,
	anchored AnchoredBootstrapBindingV1,
	expectedObserveRequest domainsecurity.MonotonicHeadObserveRequestV1,
	prepare BootstrapPrepareReceiptV1,
	commit BootstrapCommitReceiptV1,
) error {
	if err := ValidateObservationForBindingV1(committed, anchored, expectedObserveRequest); err != nil {
		return err
	}
	if err := ValidateBootstrapCommitReceiptForPrepareV1(commit, prepare, anchored); err != nil {
		return err
	}
	if committed.Phase != PhaseCommitted || committed.PrepareMutationID != prepare.AdvanceRequest.MutationID ||
		committed.PrepareReceiptDigest != prepare.ReceiptDigest || committed.CommitReceiptDigest != commit.ReceiptDigest ||
		!equalCheckpointV1(committed.MonotonicObservation.Checkpoint, commit.AdvanceReceipt.Checkpoint) {
		return ErrTransitionConflict
	}
	return nil
}

func ValidatePrepareReceiptExactReplayV1(
	anchored AnchoredBootstrapBindingV1,
	committed, replay BootstrapPrepareReceiptV1,
) error {
	if ValidateBootstrapPrepareReceiptForBindingV1(committed, anchored) != nil ||
		ValidateBootstrapPrepareReceiptForBindingV1(replay, anchored) != nil ||
		domainsecurity.ValidateMonotonicHeadIdempotentReplayV1(
			committed.AdvanceRequest, committed.AdvanceReceipt, replay.AdvanceRequest, replay.AdvanceReceipt,
		) != nil {
		return ErrReplayConflict
	}
	left, _ := BootstrapPrepareReceiptV1Bytes(committed)
	right, _ := BootstrapPrepareReceiptV1Bytes(replay)
	if !bytes.Equal(left, right) {
		return ErrReplayConflict
	}
	return nil
}

func ValidateCommitReceiptExactReplayV1(
	anchored AnchoredBootstrapBindingV1,
	prepare BootstrapPrepareReceiptV1,
	committed, replay BootstrapCommitReceiptV1,
) error {
	if ValidateBootstrapCommitReceiptForPrepareV1(committed, prepare, anchored) != nil ||
		ValidateBootstrapCommitReceiptForPrepareV1(replay, prepare, anchored) != nil ||
		domainsecurity.ValidateMonotonicHeadIdempotentReplayV1(
			committed.AdvanceRequest, committed.AdvanceReceipt, replay.AdvanceRequest, replay.AdvanceReceipt,
		) != nil {
		return ErrReplayConflict
	}
	left, _ := BootstrapCommitReceiptV1Bytes(committed)
	right, _ := BootstrapCommitReceiptV1Bytes(replay)
	if !bytes.Equal(left, right) {
		return ErrReplayConflict
	}
	return nil
}

func bindingCheckpointV1(anchored AnchoredBootstrapBindingV1) domainsecurity.MonotonicHeadCheckpointV1 {
	binding, _ := anchored.BindingV1()
	return binding.BootstrapInitialCheckpoint
}
