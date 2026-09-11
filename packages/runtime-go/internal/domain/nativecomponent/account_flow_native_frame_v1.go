package nativecomponent

import (
	"bytes"
	"encoding/json"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const AccountFlowMaximumNativeRequestFrameBytesV1 = 20 * 1024

type analyzeAccountFlowsNativeRequestFrameWireV1 struct {
	RequestID string          `json:"request_id"`
	Command   string          `json:"command"`
	CaseID    string          `json:"case_id"`
	Payload   json.RawMessage `json:"payload"`
}

// EncodeAnalyzeAccountFlowsNativeRequestFrameV1 is the only serialization
// exit for the opaque host-private account-flow arguments. It emits one fixed
// data-engine command and deliberately has no database-path or SQL field.
// The returned bytes are private process-channel material and must never be
// reused as an MCP, provider, SSE, history, log, or renderer projection.
func EncodeAnalyzeAccountFlowsNativeRequestFrameV1(
	requestID string,
	arguments AnalyzeAccountFlowsArgumentsV1,
) ([]byte, error) {
	if !domainsecurity.IsSHA256Hex(requestID) || requestID != strings.ToLower(requestID) {
		return nil, ErrRequestInvalid
	}
	payload, err := analyzeAccountFlowsPrivatePayloadV1(arguments)
	if err != nil {
		return nil, ErrRequestInvalid
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(analyzeAccountFlowsNativeRequestFrameWireV1{
		RequestID: requestID,
		Command:   OperationFundsAnalyzeAccountFlows,
		CaseID:    arguments.caseID,
		Payload:   json.RawMessage(payload),
	}); err != nil || output.Len() == 0 || output.Len() > AccountFlowMaximumNativeRequestFrameBytesV1 {
		return nil, ErrRequestInvalid
	}
	frame := output.Bytes()
	if frame[len(frame)-1] != '\n' || bytes.IndexByte(frame[:len(frame)-1], '\n') >= 0 ||
		bytes.IndexByte(frame, '\r') >= 0 || bytes.IndexByte(frame, 0) >= 0 {
		return nil, ErrRequestInvalid
	}
	return append([]byte(nil), frame...), nil
}
