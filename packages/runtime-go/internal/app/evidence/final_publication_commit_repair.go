package evidence

import (
	"errors"
	"reflect"

	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

// AcceptedFinalPublicCommitRepair is a preflighted replay of the atomic public
// CAS for a signed private accepted-final prepared intent. It is intentionally
// separate from event reconciliation, which starts only after this CAS wins.
type AcceptedFinalPublicCommitRepair struct {
	PrivateRecord domainevidence.PrivateAcceptedFinalRecord
}

func classifyUnresolvedPrivateFinal(privateRecord domainevidence.PrivateAcceptedFinalRecord, observation domainevidence.AcceptedFinalCASObservationV1) (domainevidence.AcceptedFinalDispositionState, *AcceptedFinalPublicCommitRepair, error) {
	state, repairable, err := classifyPreparedAcceptedFinalCAS(observation, privateRecord)
	if err != nil {
		return "", nil, err
	}
	if state != "" {
		return state, nil, nil
	}
	if !repairable {
		return "", nil, errors.New("unresolved private accepted final cannot be repaired")
	}
	return "", &AcceptedFinalPublicCommitRepair{PrivateRecord: privateRecord}, nil
}

func classifyPreparedAcceptedFinalCAS(observation domainevidence.AcceptedFinalCASObservationV1, privateRecord domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalDispositionState, bool, error) {
	if domainevidence.ValidatePrivateAcceptedFinalPublicationAuthority(privateRecord) != nil ||
		domainevidence.ValidateAcceptedFinalCASObservationV1(observation) != nil ||
		observation.ThreadID != privateRecord.SecurityContext.ThreadID || observation.TurnID != privateRecord.SecurityContext.TurnID {
		return "", false, errors.New("prepared accepted final primary CAS observation is unavailable")
	}
	if !reflect.DeepEqual(observation.FrozenContext, privateRecord.SecurityContext) {
		return "", false, errors.New("prepared accepted final does not match the frozen turn context")
	}
	if observation.HasWinner {
		if observation.Winner.RecordDigest == privateRecord.AcceptedFinal.RecordDigest {
			return domainevidence.AcceptedFinalCommitted, false, nil
		}
		return domainevidence.AcceptedFinalExplicitlyNotCommitted, false, nil
	}
	status := observation.Status
	if status != "running" && status != "queued" && status != "waiting" {
		return "", false, errors.New("prepared accepted final turn is no longer active and has no authority winner")
	}
	if !reflect.DeepEqual(observation.CurrentContext, privateRecord.SecurityContext) {
		return "", false, errors.New("prepared accepted final does not match the current thread context")
	}
	if _, err := appturn.BuildAcceptedFinalPublicationPlan(privateRecord.AcceptedFinal, privateRecord.RenderedText, privateRecord.PublicationIntent); err != nil {
		return "", false, err
	}
	return "", true, nil
}
