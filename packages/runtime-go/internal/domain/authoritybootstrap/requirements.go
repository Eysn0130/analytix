package authoritybootstrap

import (
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// PreparedRecoveryRequirementV1 is an opaque accepted-state constraint, not
// a write capability. In particular it has no method that accepts a
// caller-supplied observation and returns an authorization-like target. A
// future write port must own the live witness/conditional-lease cut.
type PreparedRecoveryRequirementV1 struct {
	anchored                     AnchoredBootstrapBindingV1
	acceptedObserveRequestDigest string
	prepareReceiptDigest         string
	checkpointDigest             string
	checkpointStateDigest        string
	checkpointFenceNonce         string
}

func NewPreparedRecoveryRequirementV1(
	anchored AnchoredBootstrapBindingV1,
	prepared BootstrapObservationV1,
	expectedObserveRequest domainsecurity.MonotonicHeadObserveRequestV1,
	prepare BootstrapPrepareReceiptV1,
) (PreparedRecoveryRequirementV1, error) {
	if err := ValidatePreparedObservationV1(prepared, anchored, expectedObserveRequest, prepare); err != nil {
		return PreparedRecoveryRequirementV1{}, err
	}
	checkpoint := prepared.MonotonicObservation.Checkpoint
	return PreparedRecoveryRequirementV1{
		anchored:                     anchored,
		acceptedObserveRequestDigest: expectedObserveRequest.RequestDigest,
		prepareReceiptDigest:         prepare.ReceiptDigest,
		checkpointDigest:             checkpoint.CheckpointDigest,
		checkpointStateDigest:        checkpoint.CurrentStateDigest,
		checkpointFenceNonce:         checkpoint.FenceNonce,
	}, nil
}

// ProjectionConditionV1 returns non-authoritative comparison data. It does
// not prove that the prepared head is current, reserve the head, consume the
// requirement, or permit a write.
func (requirement PreparedRecoveryRequirementV1) ProjectionConditionV1() (ProjectionConditionV1, error) {
	binding, err := requirement.anchored.BindingV1()
	if err != nil || !canonicalDigest(requirement.acceptedObserveRequestDigest) ||
		!canonicalDigest(requirement.prepareReceiptDigest) || !canonicalDigest(requirement.checkpointDigest) ||
		!canonicalDigest(requirement.checkpointStateDigest) || !canonicalDigest(requirement.checkpointFenceNonce) {
		return ProjectionConditionV1{}, errors.Join(ErrInvalidContract, err)
	}
	return projectionConditionV1(
		binding,
		requirement.acceptedObserveRequestDigest,
		PhasePrepared,
		1,
		requirement.checkpointDigest,
		requirement.checkpointStateDigest,
		requirement.checkpointFenceNonce,
		requirement.prepareReceiptDigest,
		"",
	), nil
}

type CommittedFloorRequirementV1 struct {
	anchored                     AnchoredBootstrapBindingV1
	acceptedObserveRequestDigest string
	prepareReceiptDigest         string
	commitReceiptDigest          string
	checkpointDigest             string
	checkpointStateDigest        string
	checkpointFenceNonce         string
}

func NewCommittedFloorRequirementV1(
	anchored AnchoredBootstrapBindingV1,
	committed BootstrapObservationV1,
	expectedObserveRequest domainsecurity.MonotonicHeadObserveRequestV1,
	prepare BootstrapPrepareReceiptV1,
	commit BootstrapCommitReceiptV1,
) (CommittedFloorRequirementV1, error) {
	if err := ValidateCommittedObservationV1(committed, anchored, expectedObserveRequest, prepare, commit); err != nil {
		return CommittedFloorRequirementV1{}, err
	}
	checkpoint := committed.MonotonicObservation.Checkpoint
	return CommittedFloorRequirementV1{
		anchored:                     anchored,
		acceptedObserveRequestDigest: expectedObserveRequest.RequestDigest,
		prepareReceiptDigest:         prepare.ReceiptDigest,
		commitReceiptDigest:          commit.ReceiptDigest,
		checkpointDigest:             checkpoint.CheckpointDigest,
		checkpointStateDigest:        checkpoint.CurrentStateDigest,
		checkpointFenceNonce:         checkpoint.FenceNonce,
	}, nil
}

// ProjectionConditionV1 exposes the exact committed comparison data needed
// to classify a locally missing floor after a future write port has proved a
// live conditional witness cut. It does not itself prove liveness or that the
// local floor is missing.
func (requirement CommittedFloorRequirementV1) ProjectionConditionV1() (ProjectionConditionV1, error) {
	binding, err := requirement.anchored.BindingV1()
	if err != nil || !canonicalDigest(requirement.acceptedObserveRequestDigest) ||
		!canonicalDigest(requirement.prepareReceiptDigest) || !canonicalDigest(requirement.commitReceiptDigest) ||
		!canonicalDigest(requirement.checkpointDigest) || !canonicalDigest(requirement.checkpointStateDigest) ||
		!canonicalDigest(requirement.checkpointFenceNonce) {
		return ProjectionConditionV1{}, errors.Join(ErrInvalidContract, err)
	}
	return projectionConditionV1(
		binding,
		requirement.acceptedObserveRequestDigest,
		PhaseCommitted,
		2,
		requirement.checkpointDigest,
		requirement.checkpointStateDigest,
		requirement.checkpointFenceNonce,
		requirement.prepareReceiptDigest,
		requirement.commitReceiptDigest,
	), nil
}

func projectionConditionV1(
	binding BootstrapBindingV1,
	acceptedBootstrapObserveRequestDigest string,
	phase BootstrapPhaseV1,
	generation uint64,
	checkpointDigest, stateDigest, fenceNonce, prepareReceiptDigest, commitReceiptDigest string,
) ProjectionConditionV1 {
	return ProjectionConditionV1{
		InstallationID:                        binding.InstallationID,
		CurrentManifestDigest:                 binding.CurrentManifestDigest,
		ManifestEnrollmentDigest:              binding.ManifestEnrollmentDigest,
		Namespace:                             binding.Namespace,
		SourceEnrollmentID:                    binding.EnrollmentID,
		BootstrapEnrollmentID:                 binding.BootstrapEnrollmentID,
		ProjectionSlotID:                      binding.ProjectionSlotID,
		BindingDigest:                         binding.BindingDigest,
		AcceptedFloorObserveRequestDigest:     binding.FloorObserveRequest.RequestDigest,
		AcceptedFloorObservationDigest:        binding.FloorObservation.ObservationDigest,
		AcceptedBootstrapObserveRequestDigest: acceptedBootstrapObserveRequestDigest,
		FloorCheckpoint:                       binding.FloorCheckpoint,
		FloorProjectionDigest:                 binding.FloorProjectionDigest,
		ExpectedPhase:                         phase,
		ExpectedGeneration:                    generation,
		ExpectedCheckpointDigest:              checkpointDigest,
		ExpectedStateDigest:                   stateDigest,
		ExpectedFenceNonce:                    fenceNonce,
		PrepareReceiptDigest:                  prepareReceiptDigest,
		CommitReceiptDigest:                   commitReceiptDigest,
	}
}
