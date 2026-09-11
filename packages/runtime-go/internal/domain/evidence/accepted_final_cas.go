package evidence

import (
	"encoding/json"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const AcceptedFinalCASObservationSchemaVersion = 1

// AcceptedFinalCASObservationV1 is derived only from the canonical primary
// thread record. Sidecars and hydrated projections cannot supply a winner.
type AcceptedFinalCASObservationV1 struct {
	SchemaVersion        int                                `json:"schemaVersion"`
	ThreadID             string                             `json:"threadId"`
	TurnID               string                             `json:"turnId"`
	Status               string                             `json:"status"`
	FrozenContext        domainsecurity.TurnSecurityContext `json:"frozenContext"`
	CurrentContext       domainsecurity.TurnSecurityContext `json:"currentContext"`
	HasWinner            bool                               `json:"hasWinner"`
	Winner               AcceptedFinalRecord                `json:"winner"`
	ThreadFileSHA256     string                             `json:"threadFileSha256"`
	TurnProjectionSHA256 string                             `json:"turnProjectionSha256"`
	ObservationDigest    string                             `json:"observationDigest"`
}

func NewAcceptedFinalCASObservationV1(observation AcceptedFinalCASObservationV1) (AcceptedFinalCASObservationV1, error) {
	observation.SchemaVersion = AcceptedFinalCASObservationSchemaVersion
	observation.ThreadID = strings.TrimSpace(observation.ThreadID)
	observation.TurnID = strings.TrimSpace(observation.TurnID)
	observation.Status = strings.TrimSpace(observation.Status)
	observation.ObservationDigest = ""
	if err := validateAcceptedFinalCASObservationFields(observation); err != nil {
		return AcceptedFinalCASObservationV1{}, err
	}
	observation.ObservationDigest = acceptedFinalCASObservationDigest(observation)
	return observation, nil
}

func ValidateAcceptedFinalCASObservationV1(observation AcceptedFinalCASObservationV1) error {
	if err := validateAcceptedFinalCASObservationFields(observation); err != nil ||
		!domainsecurity.IsSHA256Hex(observation.ObservationDigest) || acceptedFinalCASObservationDigest(observation) != observation.ObservationDigest {
		return errors.New("accepted final CAS observation integrity is invalid")
	}
	return nil
}

// AcceptedFinalTurnProjectionSHA256V1 is the single digest algorithm used by
// the public CAS reader, signed V2 dispositions, and every live/replay public
// projection. It hashes the complete raw turn before any field filtering.
func AcceptedFinalTurnProjectionSHA256V1(turn map[string]any) (string, error) {
	if turn == nil {
		return "", errors.New("accepted final turn projection is missing")
	}
	body, err := json.Marshal(turn)
	if err != nil || len(body) == 0 {
		return "", errors.New("accepted final turn projection is invalid")
	}
	return domainsecurity.SHA256Hex(body), nil
}

func validateAcceptedFinalCASObservationFields(observation AcceptedFinalCASObservationV1) error {
	if observation.SchemaVersion != AcceptedFinalCASObservationSchemaVersion || observation.ThreadID == "" || observation.TurnID == "" ||
		observation.ThreadID != strings.TrimSpace(observation.ThreadID) || observation.TurnID != strings.TrimSpace(observation.TurnID) ||
		observation.Status == "" || !domainsecurity.IsSHA256Hex(observation.ThreadFileSHA256) ||
		!domainsecurity.IsSHA256Hex(observation.TurnProjectionSHA256) ||
		domainsecurity.ValidateTurnSecurityContext(observation.FrozenContext) != nil ||
		domainsecurity.ValidateTurnSecurityContext(observation.CurrentContext) != nil ||
		observation.FrozenContext.ThreadID != observation.ThreadID || observation.FrozenContext.TurnID != observation.TurnID ||
		observation.CurrentContext.ThreadID != observation.ThreadID {
		return errors.New("accepted final CAS observation fields are invalid")
	}
	if observation.HasWinner {
		if ValidateAcceptedFinalRecord(observation.Winner) != nil || observation.Winner.ThreadID != observation.ThreadID ||
			observation.Winner.TurnID != observation.TurnID || observation.Winner.ContextDigest != observation.FrozenContext.ContextDigest {
			return errors.New("accepted final CAS observation winner is invalid")
		}
	} else if observation.Winner.RecordDigest != "" {
		return errors.New("accepted final CAS observation contains a detached winner")
	}
	return nil
}

func acceptedFinalCASObservationDigest(observation AcceptedFinalCASObservationV1) string {
	body := struct {
		SchemaVersion        int                                `json:"schemaVersion"`
		ThreadID             string                             `json:"threadId"`
		TurnID               string                             `json:"turnId"`
		Status               string                             `json:"status"`
		FrozenContext        domainsecurity.TurnSecurityContext `json:"frozenContext"`
		CurrentContext       domainsecurity.TurnSecurityContext `json:"currentContext"`
		HasWinner            bool                               `json:"hasWinner"`
		Winner               AcceptedFinalRecord                `json:"winner"`
		ThreadFileSHA256     string                             `json:"threadFileSha256"`
		TurnProjectionSHA256 string                             `json:"turnProjectionSha256"`
	}{
		observation.SchemaVersion, observation.ThreadID, observation.TurnID, observation.Status,
		observation.FrozenContext, observation.CurrentContext, observation.HasWinner, observation.Winner,
		observation.ThreadFileSHA256, observation.TurnProjectionSHA256,
	}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded)
}
