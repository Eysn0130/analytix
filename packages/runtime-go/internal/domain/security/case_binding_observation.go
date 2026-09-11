package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	CaseBindingObservationSchemaVersion = 1
	CaseBindingObservationPurpose       = "analytix.case-binding-observation/v1"
)

// CaseBindingObservationV1 is the closed, deterministic result of one
// host-side inspection of the workspace case marker. Only a valid observation
// carries binding authority; all other states are explicit negative evidence.
type CaseBindingObservationV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	WorkspaceRealPath string `json:"workspaceRealPath"`
	State             string `json:"state"`
	CaseID            string `json:"caseId"`
	BindingSHA256     string `json:"bindingSHA256"`
	CaseBindingHash   string `json:"caseBindingHash"`
	ObservationDigest string `json:"observationDigest"`
}

type CaseBindingObservationInputV1 struct {
	WorkspaceRealPath string
	State             string
	CaseID            string
	BindingSHA256     string
	CaseBindingHash   string
}

func NewCaseBindingObservationV1(input CaseBindingObservationInputV1) (CaseBindingObservationV1, error) {
	observation := CaseBindingObservationV1{
		SchemaVersion:     CaseBindingObservationSchemaVersion,
		Purpose:           CaseBindingObservationPurpose,
		WorkspaceRealPath: strings.TrimSpace(input.WorkspaceRealPath),
		State:             strings.TrimSpace(input.State),
		CaseID:            strings.TrimSpace(input.CaseID),
		BindingSHA256:     strings.TrimSpace(input.BindingSHA256),
		CaseBindingHash:   strings.TrimSpace(input.CaseBindingHash),
	}
	observation.ObservationDigest = caseBindingObservationDigestV1(observation)
	if err := ValidateCaseBindingObservationV1(observation); err != nil {
		return CaseBindingObservationV1{}, err
	}
	return observation, nil
}

func ValidateCaseBindingObservationV1(observation CaseBindingObservationV1) error {
	if observation.SchemaVersion != CaseBindingObservationSchemaVersion || observation.Purpose != CaseBindingObservationPurpose ||
		observation.WorkspaceRealPath == "" || strings.TrimSpace(observation.WorkspaceRealPath) != observation.WorkspaceRealPath ||
		observation.State == "" || strings.TrimSpace(observation.State) != observation.State ||
		observation.CaseID != strings.TrimSpace(observation.CaseID) ||
		observation.BindingSHA256 != strings.TrimSpace(observation.BindingSHA256) ||
		observation.CaseBindingHash != strings.TrimSpace(observation.CaseBindingHash) ||
		!isCanonicalSHA256Hex(observation.ObservationDigest) || observation.ObservationDigest != caseBindingObservationDigestV1(observation) {
		return errors.New("case binding observation integrity is invalid")
	}
	switch observation.State {
	case CaseBindingStateValid:
		if observation.CaseID == "" || observation.CaseID == UnboundCaseID ||
			!isCanonicalSHA256Hex(observation.BindingSHA256) || !isCanonicalSHA256Hex(observation.CaseBindingHash) {
			return errors.New("valid case binding observation is incomplete")
		}
	case CaseBindingStateInvalid:
		if observation.CaseID != "" || observation.CaseBindingHash != "" ||
			(observation.BindingSHA256 != "" && !isCanonicalSHA256Hex(observation.BindingSHA256)) {
			return errors.New("invalid case binding observation shape is invalid")
		}
	case CaseBindingStateNotApplicable, CaseBindingStateMissing, CaseBindingStateUnreadable,
		CaseBindingStateUnstable, CaseBindingStateWorkspaceMissing:
		if observation.CaseID != "" || observation.BindingSHA256 != "" || observation.CaseBindingHash != "" {
			return errors.New("non-authoritative case binding observation carries binding fields")
		}
	default:
		return errors.New("case binding observation state is invalid")
	}
	return nil
}

func ParseCaseBindingObservationV1(body []byte) (CaseBindingObservationV1, error) {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 * 1024, MaxDepth: 8, MaxTokens: 128, MaxStringBytes: 16 * 1024,
	}); err != nil {
		return CaseBindingObservationV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var observation CaseBindingObservationV1
	if err := decoder.Decode(&observation); err != nil {
		return CaseBindingObservationV1{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CaseBindingObservationV1{}, errors.New("case binding observation contains trailing JSON")
	}
	return observation, ValidateCaseBindingObservationV1(observation)
}

func CaseBindingObservationV1Bytes(observation CaseBindingObservationV1) ([]byte, error) {
	if err := ValidateCaseBindingObservationV1(observation); err != nil {
		return nil, err
	}
	return json.Marshal(observation)
}

func caseBindingObservationDigestV1(observation CaseBindingObservationV1) string {
	observation.ObservationDigest = ""
	body, _ := json.Marshal(observation)
	return SHA256Hex(body)
}
