package toolresult

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	CaseSourceBindingProofFieldV1   = "caseSourceBindingProof"
	CaseSourceBindingProofVersionV1 = 1
	CaseSourceBindingProofPurposeV1 = "analytix.case-source-result-binding/v1"
	maxCaseSourceContextEpochV1     = uint64(9007199254740991)
)

// CaseSourceBindingProofV1 is host-only durable integrity metadata. Its digest
// binds the private outer settlement tuple to the closed public status without
// retaining ToolOutcome, case/source values, PII, paths, raw tool bodies, or
// provider text. Public tool-result projections always omit this value.
type CaseSourceBindingProofV1 struct {
	Version int    `json:"version"`
	Purpose string `json:"purpose"`
	Digest  string `json:"digest"`
}

type CaseSourceBindingInputV1 struct {
	ToolName         string
	ToolCallID       string
	ContextDigest    string
	ContextEpoch     uint64
	ExecutionGrantID string
	Projection       PublicToolResultProjectionV1
	IsError          bool
}

type caseSourceBindingMaterialV1 struct {
	Version           int                  `json:"version"`
	Purpose           string               `json:"purpose"`
	ToolName          string               `json:"toolName"`
	ToolCallID        string               `json:"callId"`
	ContextDigest     string               `json:"contextDigest"`
	ContextEpoch      uint64               `json:"contextEpoch"`
	ExecutionGrantID  string               `json:"executionGrantId"`
	ProjectionKind    PublicProjectionKind `json:"projectionKind"`
	ProjectionStatus  string               `json:"projectionStatus"`
	ProjectionCode    string               `json:"projectionCode"`
	ProjectionMessage string               `json:"projectionMessage"`
	IsError           bool                 `json:"isError"`
}

func NewCaseSourceBindingProofV1(input CaseSourceBindingInputV1) (CaseSourceBindingProofV1, error) {
	material, err := caseSourceBindingMaterial(input)
	if err != nil {
		return CaseSourceBindingProofV1{}, err
	}
	body, err := json.Marshal(material)
	if err != nil {
		return CaseSourceBindingProofV1{}, err
	}
	digest := domainsecurity.CanonicalJSONHash(body)
	if !domainsecurity.IsSHA256Hex(digest) {
		return CaseSourceBindingProofV1{}, errors.New("case source binding proof digest is invalid")
	}
	return CaseSourceBindingProofV1{
		Version: CaseSourceBindingProofVersionV1,
		Purpose: CaseSourceBindingProofPurposeV1,
		Digest:  digest,
	}, nil
}

func ValidateCaseSourceBindingProofV1(input CaseSourceBindingInputV1, proof CaseSourceBindingProofV1) error {
	if proof.Version != CaseSourceBindingProofVersionV1 || proof.Purpose != CaseSourceBindingProofPurposeV1 ||
		strings.TrimSpace(proof.Digest) != proof.Digest || !domainsecurity.IsSHA256Hex(proof.Digest) {
		return errors.New("case source binding proof is invalid")
	}
	expected, err := NewCaseSourceBindingProofV1(input)
	if err != nil || proof != expected {
		return errors.New("case source binding proof does not match the private outer tuple")
	}
	return nil
}

func ParseCaseSourceBindingProofV1(value any) (CaseSourceBindingProofV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return CaseSourceBindingProofV1{}, err
	}
	// The shared canonical decoder rejects duplicate keys before encoding/json
	// could silently keep the last occurrence.
	if _, err := domainsecurity.DecodeCanonicalJSONObject(body); err != nil {
		return CaseSourceBindingProofV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var proof CaseSourceBindingProofV1
	if err := decoder.Decode(&proof); err != nil {
		return CaseSourceBindingProofV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return CaseSourceBindingProofV1{}, errors.New("case source binding proof contains trailing data")
	}
	if proof.Version != CaseSourceBindingProofVersionV1 || proof.Purpose != CaseSourceBindingProofPurposeV1 ||
		strings.TrimSpace(proof.Digest) != proof.Digest || !domainsecurity.IsSHA256Hex(proof.Digest) {
		return CaseSourceBindingProofV1{}, errors.New("case source binding proof is invalid")
	}
	return proof, nil
}

func CaseSourceBindingProofRecordV1(proof CaseSourceBindingProofV1) map[string]any {
	if proof.Version != CaseSourceBindingProofVersionV1 || proof.Purpose != CaseSourceBindingProofPurposeV1 ||
		strings.TrimSpace(proof.Digest) != proof.Digest || !domainsecurity.IsSHA256Hex(proof.Digest) {
		return nil
	}
	body, _ := json.Marshal(proof)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func validateCaseSourceBindingProofForItemV1(projection PublicToolResultProjectionV1, item map[string]any) error {
	isError, ok := item["isError"].(bool)
	if !ok {
		return errors.New("case source private error state is invalid")
	}
	rootStatus, err := SettlementLifecycleStatusV1(projection, isError)
	if err != nil || publicItemString(item, "status") != rootStatus {
		return errors.New("case source private lifecycle is invalid")
	}
	proof, err := ParseCaseSourceBindingProofV1(item[CaseSourceBindingProofFieldV1])
	if err != nil {
		return err
	}
	input, err := caseSourceBindingInputFromItemV1(projection, item)
	if err != nil {
		return err
	}
	return ValidateCaseSourceBindingProofV1(input, proof)
}

func caseSourceBindingInputFromItemV1(projection PublicToolResultProjectionV1, item map[string]any) (CaseSourceBindingInputV1, error) {
	threadID := publicItemString(item, "threadId")
	turnID := publicItemString(item, "turnId")
	toolName := publicItemString(item, "toolName")
	callID := publicItemString(item, "callId")
	contextDigest := publicItemString(item, "contextDigest")
	grantID := publicItemString(item, "executionGrantId")
	contextEpoch := publicItemUint64(item["contextEpoch"])
	if threadID == "" || strings.TrimSpace(threadID) != threadID || turnID == "" || strings.TrimSpace(turnID) != turnID ||
		publicItemString(item, "id") != ToolResultItemIDV1(turnID, callID) ||
		publicItemString(item, "kind") != "tool_result" || publicItemString(item, "role") != "tool" ||
		toolName == "" || strings.TrimSpace(toolName) != toolName || len(toolName) > 256 ||
		!domainmodel.IsHostToolCallIDV1(callID) || strings.TrimSpace(callID) != callID ||
		strings.TrimSpace(contextDigest) != contextDigest || !domainsecurity.IsSHA256Hex(contextDigest) ||
		contextEpoch == 0 || contextEpoch > maxCaseSourceContextEpochV1 ||
		strings.TrimSpace(grantID) != grantID || !domainsecurity.IsSHA256Hex(grantID) {
		return CaseSourceBindingInputV1{}, errors.New("case source private outer tuple is invalid")
	}
	isError, ok := item["isError"].(bool)
	if !ok {
		return CaseSourceBindingInputV1{}, errors.New("case source private error state is invalid")
	}
	return CaseSourceBindingInputV1{
		ToolName: toolName, ToolCallID: callID, ContextDigest: contextDigest,
		ContextEpoch: contextEpoch, ExecutionGrantID: grantID,
		Projection: projection, IsError: isError,
	}, nil
}

func caseSourceBindingMaterial(input CaseSourceBindingInputV1) (caseSourceBindingMaterialV1, error) {
	if input.ToolName == "" || strings.TrimSpace(input.ToolName) != input.ToolName || len(input.ToolName) > 256 ||
		!domainmodel.IsHostToolCallIDV1(input.ToolCallID) || strings.TrimSpace(input.ToolCallID) != input.ToolCallID ||
		strings.TrimSpace(input.ContextDigest) != input.ContextDigest || !domainsecurity.IsSHA256Hex(input.ContextDigest) ||
		input.ContextEpoch == 0 || input.ContextEpoch > maxCaseSourceContextEpochV1 ||
		strings.TrimSpace(input.ExecutionGrantID) != input.ExecutionGrantID || !domainsecurity.IsSHA256Hex(input.ExecutionGrantID) ||
		ValidatePublicToolResultProjectionV1(input.Projection) != nil ||
		input.Projection.ProjectionKind != ProjectionCaseSourceStatus {
		return caseSourceBindingMaterialV1{}, errors.New("case source binding material is invalid")
	}
	if _, err := SettlementLifecycleStatusV1(input.Projection, input.IsError); err != nil {
		return caseSourceBindingMaterialV1{}, err
	}
	return caseSourceBindingMaterialV1{
		Version: CaseSourceBindingProofVersionV1, Purpose: CaseSourceBindingProofPurposeV1,
		ToolName: input.ToolName, ToolCallID: input.ToolCallID,
		ContextDigest: input.ContextDigest, ContextEpoch: input.ContextEpoch, ExecutionGrantID: input.ExecutionGrantID,
		ProjectionKind: input.Projection.ProjectionKind, ProjectionStatus: input.Projection.Status,
		ProjectionCode: input.Projection.Code, ProjectionMessage: input.Projection.MessageKey, IsError: input.IsError,
	}, nil
}
