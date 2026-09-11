package nativecomponentrunner

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	nativeProtocolVersion                   = "analytix-native-v1"
	readinessSchemaVersion                  = 3
	readinessFrameLimit                     = 4 * 1024
	requestFrameLimit                       = 4 * 1024
	responseFrameLimit                      = 64 * 1024
	accountFlowResponseFrameLimit           = domainnative.AccountFlowMaximumResultBytesV1 + 16*1024
	accountIngressResponseFrameLimit        = domainnative.AccountIngressResolutionMaximumResultBytesV1 + 16*1024
	directSourcePreviewResponseFrameLimit   = domainnative.DirectSourcePreviewMaximumResultBytesV1 + 16*1024
	deterministicCleaningResponseFrameLimit = domainnative.DeterministicCleaningMaximumResultBytesV1 + 16*1024
	transactionSourceRowResponseFrameLimit  = domainnative.TransactionSourceRowMaximumResultBytesV1 + 16*1024
	fundsCanonicalCSVResponseFrameLimit     = domainnative.FundsCanonicalCSVMaximumResultBytesV1 + 16*1024
	maxDiagnosticRunMilliseconds            = uint64(10_000)
	transactionSourceRowMaxRunMilliseconds  = uint64(domainnative.TransactionSourceRowMaximumDurationV1 / time.Millisecond)
)

const deterministicCleaningResponseEnvelopeTokens = 26
const deterministicCleaningResponseMaximumTokens = domainnative.DeterministicCleaningMaximumResultTokensV1 +
	deterministicCleaningResponseEnvelopeTokens

type readinessEnvelope struct {
	Kind            string `json:"kind"`
	SchemaVersion   *int   `json:"schema_version"`
	ComponentID     string `json:"component_id"`
	LaunchNonce     string `json:"launch_nonce"`
	ProtocolVersion string `json:"protocol_version"`
	ProcessID       *int   `json:"process_id"`
}

func encodePingRequestFrame(requestID string) ([]byte, error) {
	if !domainsecurity.IsSHA256Hex(requestID) {
		return nil, ErrProtocol
	}
	const prefix = `{"request_id":"`
	const suffix = `","command":"ping"}` + "\n"
	total := len(prefix) + len(requestID) + len(suffix)
	if total <= 0 || total > requestFrameLimit {
		return nil, ErrProtocol
	}
	output := make([]byte, total)
	offset := copy(output, prefix)
	offset += copy(output[offset:], requestID)
	offset += copy(output[offset:], suffix)
	if offset != total || output[total-1] != '\n' {
		return nil, ErrProtocol
	}
	frame := output[:total-1]
	if err := domainjsonstrict.Validate(frame, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: requestFrameLimit - 1, MaxDepth: 4, MaxTokens: 16,
		MaxStringBytes: 128, MaxNumberBytes: 16, MaxAbsExponent: 1,
	}); err != nil {
		return nil, ErrProtocol
	}
	return output, nil
}

func EncodeProbePingRequest(requestID string) ([]byte, error) {
	return encodePingRequestFrame(requestID)
}

type pingResponse struct {
	RequestID   string           `json:"request_id"`
	OK          *bool            `json:"ok"`
	Data        *pingData        `json:"data"`
	Diagnostics *pingDiagnostics `json:"diagnostics"`
}

type pingData struct {
	Pong *bool `json:"pong"`
	PID  *int  `json:"pid"`
}

type pingDiagnostics struct {
	Engine      string  `json:"engine"`
	Command     string  `json:"command"`
	CaseBound   *bool   `json:"case_bound"`
	DBBound     *bool   `json:"db_bound"`
	PID         *int    `json:"pid"`
	QueueWaitMS *uint64 `json:"queue_wait_ms"`
	RunMS       *uint64 `json:"run_ms"`
	OwnerEpoch  *string `json:"owner_epoch"`
}

type accountFlowResponse struct {
	RequestID   string           `json:"request_id"`
	OK          *bool            `json:"ok"`
	Data        json.RawMessage  `json:"data"`
	Diagnostics *pingDiagnostics `json:"diagnostics"`
}

type exactJSONShape map[string]exactJSONShape

var readinessJSONShape = exactJSONShape{
	"kind": nil, "schema_version": nil, "component_id": nil, "launch_nonce": nil,
	"protocol_version": nil, "process_id": nil,
}

var pingResponseJSONShape = exactJSONShape{
	"request_id": nil, "ok": nil,
	"data": {"pong": nil, "pid": nil},
	"diagnostics": {
		"engine": nil, "command": nil, "case_bound": nil, "db_bound": nil,
		"pid": nil, "queue_wait_ms": nil, "run_ms": nil, "owner_epoch": nil,
	},
}

var accountFlowResponseJSONShape = exactJSONShape{
	"request_id": nil, "ok": nil, "data": nil,
	"diagnostics": {
		"engine": nil, "command": nil, "case_bound": nil, "db_bound": nil,
		"pid": nil, "queue_wait_ms": nil, "run_ms": nil, "owner_epoch": nil,
	},
}

func decodeStrictFrame(frame []byte, limit int, shape exactJSONShape, target any) error {
	return decodeStrictFrameWithLimits(frame, limit, 4096, shape, target)
}

func decodeStrictFrameWithLimits(frame []byte, limit, maxTokens int, shape exactJSONShape, target any) error {
	if len(frame) == 0 || limit <= 1 || maxTokens <= 0 || len(frame) >= limit || len(shape) == 0 || target == nil {
		return ErrProtocol
	}
	if err := domainjsonstrict.Validate(frame, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: limit - 1, MaxDepth: 16, MaxTokens: maxTokens,
		MaxStringBytes: 4096, MaxNumberBytes: 64, MaxAbsExponent: 64,
	}); err != nil {
		return ErrProtocol
	}
	if !matchesExactJSONShape(frame, shape) {
		return ErrProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrProtocol
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ErrProtocol
	}
	return nil
}

func parseAnalyzeAccountFlowsResponse(
	frame []byte,
	requestID string,
	pid int,
	arguments domainnative.AnalyzeAccountFlowsArgumentsV1,
) (domainnative.AnalyzeAccountFlowsResultV1, error) {
	zero := domainnative.AnalyzeAccountFlowsResultV1{}
	if !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 {
		return zero, ErrProtocol
	}
	var response accountFlowResponse
	if decodeStrictFrameWithLimits(
		frame,
		accountFlowResponseFrameLimit,
		64_128,
		accountFlowResponseJSONShape,
		&response,
	) != nil || response.RequestID != requestID || !trueValue(response.OK) ||
		len(bytes.TrimSpace(response.Data)) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) ||
		len(response.Data) > domainnative.AccountFlowMaximumResultBytesV1 || response.Diagnostics == nil ||
		response.Diagnostics.Engine != "analytix-data-engine" ||
		response.Diagnostics.Command != domainnative.OperationFundsAnalyzeAccountFlows ||
		!trueValue(response.Diagnostics.CaseBound) || !trueValue(response.Diagnostics.DBBound) ||
		response.Diagnostics.PID == nil || *response.Diagnostics.PID != pid ||
		response.Diagnostics.QueueWaitMS == nil || *response.Diagnostics.QueueWaitMS != 0 ||
		response.Diagnostics.RunMS == nil || *response.Diagnostics.RunMS > maxDiagnosticRunMilliseconds ||
		response.Diagnostics.OwnerEpoch == nil || *response.Diagnostics.OwnerEpoch != "" {
		return zero, ErrProtocol
	}
	result, err := domainnative.ParseAnalyzeAccountFlowsResultV1(response.Data, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	return result, nil
}

func parseResolveAccountIngressResponse(
	frame []byte,
	requestID string,
	pid int,
	arguments domainnative.ResolveAccountIngressArgumentsV1,
) (domainnative.ResolveAccountIngressResultV1, error) {
	zero := domainnative.ResolveAccountIngressResultV1{}
	if !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 {
		return zero, ErrProtocol
	}
	var response accountFlowResponse
	if decodeStrictFrameWithLimits(
		frame,
		accountIngressResponseFrameLimit,
		16_384,
		accountFlowResponseJSONShape,
		&response,
	) != nil || response.RequestID != requestID || !trueValue(response.OK) ||
		len(bytes.TrimSpace(response.Data)) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) ||
		len(response.Data) > domainnative.AccountIngressResolutionMaximumResultBytesV1 ||
		response.Diagnostics == nil ||
		response.Diagnostics.Engine != "analytix-data-engine" ||
		response.Diagnostics.Command != domainnative.OperationFundsResolveAccountIngress ||
		!trueValue(response.Diagnostics.CaseBound) || !trueValue(response.Diagnostics.DBBound) ||
		response.Diagnostics.PID == nil || *response.Diagnostics.PID != pid ||
		response.Diagnostics.QueueWaitMS == nil || *response.Diagnostics.QueueWaitMS != 0 ||
		response.Diagnostics.RunMS == nil || *response.Diagnostics.RunMS > maxDiagnosticRunMilliseconds ||
		response.Diagnostics.OwnerEpoch == nil || *response.Diagnostics.OwnerEpoch != "" {
		return zero, ErrProtocol
	}
	result, err := domainnative.ParseResolveAccountIngressResultV1(response.Data, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	return result, nil
}

func parseDirectSourcePreviewResponse(
	frame []byte,
	requestID string,
	pid int,
	arguments domainnative.DirectSourcePreviewArgumentsV1,
) (domainnative.DirectSourcePreviewResultV1, error) {
	zero := domainnative.DirectSourcePreviewResultV1{}
	if !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 {
		return zero, ErrProtocol
	}
	var response accountFlowResponse
	if decodeStrictFrameWithLimits(
		frame,
		directSourcePreviewResponseFrameLimit,
		32_768,
		accountFlowResponseJSONShape,
		&response,
	) != nil || response.RequestID != requestID || !trueValue(response.OK) ||
		len(bytes.TrimSpace(response.Data)) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) ||
		len(response.Data) > domainnative.DirectSourcePreviewMaximumResultBytesV1 ||
		response.Diagnostics == nil ||
		response.Diagnostics.Engine != "analytix-data-engine" ||
		response.Diagnostics.Command != domainnative.OperationFundsDirectSourcePreview ||
		!trueValue(response.Diagnostics.CaseBound) || !trueValue(response.Diagnostics.DBBound) ||
		response.Diagnostics.PID == nil || *response.Diagnostics.PID != pid ||
		response.Diagnostics.QueueWaitMS == nil || *response.Diagnostics.QueueWaitMS != 0 ||
		response.Diagnostics.RunMS == nil || *response.Diagnostics.RunMS > maxDiagnosticRunMilliseconds ||
		response.Diagnostics.OwnerEpoch == nil || *response.Diagnostics.OwnerEpoch != "" {
		return zero, ErrProtocol
	}
	result, err := domainnative.ParseDirectSourcePreviewResultV1(response.Data, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	return result, nil
}

func parseDeterministicCleaningResponse(
	frame []byte,
	requestID string,
	pid int,
	arguments domainnative.DeterministicCleaningArgumentsV1,
) (domainnative.DeterministicCleaningResultV1, error) {
	zero := domainnative.DeterministicCleaningResultV1{}
	if !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 {
		return zero, ErrProtocol
	}
	var response accountFlowResponse
	if decodeStrictFrameWithLimits(
		frame,
		deterministicCleaningResponseFrameLimit,
		deterministicCleaningResponseMaximumTokens,
		accountFlowResponseJSONShape,
		&response,
	) != nil || response.RequestID != requestID || !trueValue(response.OK) ||
		len(bytes.TrimSpace(response.Data)) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) ||
		len(response.Data) > domainnative.DeterministicCleaningMaximumResultBytesV1 ||
		response.Diagnostics == nil || response.Diagnostics.Engine != "analytix-data-engine" ||
		response.Diagnostics.Command != domainnative.OperationFundsDeterministicCleaningV1 ||
		!trueValue(response.Diagnostics.CaseBound) || !trueValue(response.Diagnostics.DBBound) ||
		response.Diagnostics.PID == nil || *response.Diagnostics.PID != pid ||
		response.Diagnostics.QueueWaitMS == nil || *response.Diagnostics.QueueWaitMS != 0 ||
		response.Diagnostics.RunMS == nil || *response.Diagnostics.RunMS > maxDiagnosticRunMilliseconds ||
		response.Diagnostics.OwnerEpoch == nil || *response.Diagnostics.OwnerEpoch != "" {
		return zero, ErrProtocol
	}
	result, err := domainnative.ParseDeterministicCleaningResultV1(response.Data, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	return result, nil
}

func parseTransactionSourceRowPageResponse(
	frame []byte,
	requestID string,
	pid int,
	arguments domainnative.TransactionSourceRowPageArgumentsV1,
) (domainnative.TransactionSourceRowPageV1, error) {
	zero := domainnative.TransactionSourceRowPageV1{}
	if !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 || len(frame) == 0 ||
		len(frame) >= transactionSourceRowResponseFrameLimit ||
		domainjsonstrict.Validate(frame, domainjsonstrict.Options{
			RequireObject:  true,
			MaxBytes:       transactionSourceRowResponseFrameLimit - 1,
			MaxDepth:       20,
			MaxTokens:      180_000,
			MaxStringBytes: 32 * 1024,
			MaxNumberBytes: 64,
			MaxAbsExponent: 1,
		}) != nil || !matchesExactJSONShape(frame, accountFlowResponseJSONShape) {
		return zero, ErrProtocol
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	var response accountFlowResponse
	if decoder.Decode(&response) != nil {
		return zero, ErrProtocol
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return zero, ErrProtocol
	}
	if response.RequestID != requestID || !trueValue(response.OK) ||
		len(bytes.TrimSpace(response.Data)) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) ||
		len(response.Data) > domainnative.TransactionSourceRowMaximumResultBytesV1 ||
		response.Diagnostics == nil || response.Diagnostics.Engine != "analytix-data-engine" ||
		response.Diagnostics.Command != domainnative.OperationFundsTransactionSourceRowPageV1 ||
		!trueValue(response.Diagnostics.CaseBound) || !trueValue(response.Diagnostics.DBBound) ||
		response.Diagnostics.PID == nil || *response.Diagnostics.PID != pid ||
		response.Diagnostics.QueueWaitMS == nil || *response.Diagnostics.QueueWaitMS != 0 ||
		response.Diagnostics.RunMS == nil || *response.Diagnostics.RunMS > transactionSourceRowMaxRunMilliseconds ||
		response.Diagnostics.OwnerEpoch == nil || *response.Diagnostics.OwnerEpoch != "" {
		return zero, ErrProtocol
	}
	result, err := domainnative.ParseTransactionSourceRowPageResultV1(response.Data, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	return result, nil
}

func parseFundsCanonicalCSVSnapshotBuildResponse(
	frame []byte,
	requestID string,
	pid int,
	arguments domainnative.FundsCanonicalCSVSnapshotBuildArgumentsV1,
) (domainnative.FundsCanonicalCSVSnapshotBuildResultV1, error) {
	zero := domainnative.FundsCanonicalCSVSnapshotBuildResultV1{}
	if !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 {
		return zero, ErrProtocol
	}
	var response accountFlowResponse
	if decodeStrictFrameWithLimits(
		frame,
		fundsCanonicalCSVResponseFrameLimit,
		4_096,
		accountFlowResponseJSONShape,
		&response,
	) != nil || response.RequestID != requestID || !trueValue(response.OK) ||
		len(bytes.TrimSpace(response.Data)) == 0 || bytes.Equal(bytes.TrimSpace(response.Data), []byte("null")) ||
		len(response.Data) > domainnative.FundsCanonicalCSVMaximumResultBytesV1 ||
		response.Diagnostics == nil || response.Diagnostics.Engine != "analytix-data-engine" ||
		response.Diagnostics.Command != domainnative.OperationFundsBuildCanonicalCSVSnapshotV1 ||
		!trueValue(response.Diagnostics.CaseBound) || !trueValue(response.Diagnostics.DBBound) ||
		response.Diagnostics.PID == nil || *response.Diagnostics.PID != pid ||
		response.Diagnostics.QueueWaitMS == nil || *response.Diagnostics.QueueWaitMS != 0 ||
		response.Diagnostics.RunMS == nil || *response.Diagnostics.RunMS > maxDiagnosticRunMilliseconds ||
		response.Diagnostics.OwnerEpoch == nil || *response.Diagnostics.OwnerEpoch != "" {
		return zero, ErrProtocol
	}
	result, err := domainnative.ParseFundsCanonicalCSVSnapshotBuildResultV1(response.Data, arguments)
	if err != nil {
		return zero, ErrProtocol
	}
	return result, nil
}

func matchesExactJSONShape(raw json.RawMessage, shape exactJSONShape) bool {
	if len(shape) == 0 {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != len(shape) {
		return false
	}
	for name, nested := range shape {
		value, ok := object[name]
		if !ok || nested != nil && !matchesExactJSONShape(value, nested) {
			return false
		}
	}
	return true
}

func trueValue(value *bool) bool {
	return value != nil && *value
}

func falseValue(value *bool) bool {
	return value != nil && !*value
}

func ValidateProbeReadiness(frame []byte, componentID string, nonce string, pid int) bool {
	if !validProbeComponent(componentID) || !domainsecurity.IsSHA256Hex(nonce) || pid <= 0 {
		return false
	}
	var readiness readinessEnvelope
	return decodeStrictFrame(frame, readinessFrameLimit, readinessJSONShape, &readiness) == nil &&
		readiness.SchemaVersion != nil && *readiness.SchemaVersion == readinessSchemaVersion &&
		readiness.Kind == "analytix_native_ready" && readiness.ComponentID == componentID &&
		readiness.LaunchNonce == nonce && readiness.ProtocolVersion == nativeProtocolVersion &&
		readiness.ProcessID != nil && *readiness.ProcessID == pid
}

func ValidateProbePingResponse(frame []byte, componentID string, requestID string, pid int) bool {
	engine, ok := probeEngine(componentID)
	if !ok || !domainsecurity.IsSHA256Hex(requestID) || pid <= 0 {
		return false
	}
	var response pingResponse
	return decodeStrictFrame(frame, responseFrameLimit, pingResponseJSONShape, &response) == nil &&
		response.RequestID == requestID && trueValue(response.OK) && response.Data != nil &&
		trueValue(response.Data.Pong) && response.Data.PID != nil && *response.Data.PID == pid &&
		response.Diagnostics != nil && response.Diagnostics.Engine == engine &&
		response.Diagnostics.Command == "ping" && falseValue(response.Diagnostics.CaseBound) &&
		falseValue(response.Diagnostics.DBBound) && response.Diagnostics.PID != nil &&
		*response.Diagnostics.PID == pid && response.Diagnostics.QueueWaitMS != nil &&
		*response.Diagnostics.QueueWaitMS == 0 && response.Diagnostics.RunMS != nil &&
		*response.Diagnostics.RunMS <= maxDiagnosticRunMilliseconds && response.Diagnostics.OwnerEpoch != nil &&
		*response.Diagnostics.OwnerEpoch == ""
}

func validProbeComponent(componentID string) bool {
	_, ok := probeEngine(componentID)
	return ok
}

func probeEngine(componentID string) (string, bool) {
	switch componentID {
	case domainnative.ComponentImportAccelerator:
		return "analytix-import-accelerator", true
	case domainnative.ComponentCleaningOps:
		return "analytix-cleaning-ops", true
	case domainnative.ComponentAnalysisCompute:
		return "analytix-analysis-compute", true
	case domainnative.ComponentDataEngine:
		return "analytix-data-engine", true
	default:
		return "", false
	}
}
