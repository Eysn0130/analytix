package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	loopapp "analytix.local/runtime-go/internal/app/loop"
	planapp "analytix.local/runtime-go/internal/app/plan"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const (
	maxStartTurnRequestBytes  = 4 << 20
	maxStartTurnJSONDepth     = 32
	maxStartTurnJSONTokens    = 50_000
	maxStartTurnJSONString    = 2 << 20
	maxStartTurnModelSteps    = 10_000
	maxStartTurnListItemCount = 4_096
)

var startTurnRequestFields = map[string]struct{}{
	"prompt": {}, "displayText": {}, "riskIntent": {}, "async": {}, "model": {},
	"providerId": {}, "endpointFormat": {}, "reasoningEffort": {}, "mode": {},
	"approvalPolicy": {}, "sandboxMode": {}, "attachmentIds": {}, "fileReferences": {},
	"guiPlan": {}, "workspaceCheckpointId": {}, "disableUserInput": {}, "maxModelSteps": {},
}

type StartTurnControl interface {
	SendTurn(context.Context, controlapp.StartTurnRequest) (map[string]any, error)
}

type StartTurnHandlers struct {
	Control StartTurnControl
}

func (h StartTurnHandlers) HandleThreadTurns(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Control == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_control_missing", "message": "runtime control missing"})
		return
	}
	body, err := readStartTurnBody(r)
	if err != nil {
		writeInvalidStartTurnBody(w)
		return
	}
	request, err := DecodeStartTurnRequest(threadID, body)
	if err != nil {
		writeInvalidStartTurnBody(w)
		return
	}
	response, err := h.Control.SendTurn(r.Context(), request)
	WriteStartTurnResult(w, response, err)
}

func readStartTurnBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, errors.New("start turn body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxStartTurnRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxStartTurnRequestBytes {
		return nil, errors.New("start turn body exceeds size limit")
	}
	return body, nil
}

func writeInvalidStartTurnBody(w http.ResponseWriter) {
	WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid start turn body"})
}

// DecodeStartTurnRequest is the sole public HTTP decoder for a foreground
// turn. It rejects ambiguous JSON and closed-schema violations before a typed
// request can reach control or any host-owned security policy.
func DecodeStartTurnRequest(threadID string, body []byte) (controlapp.StartTurnRequest, error) {
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		MaxBytes:       maxStartTurnRequestBytes,
		MaxDepth:       maxStartTurnJSONDepth,
		MaxTokens:      maxStartTurnJSONTokens,
		MaxStringBytes: maxStartTurnJSONString,
		MaxNumberBytes: 64,
		MaxAbsExponent: 10_000,
	})
	if err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	if err := rejectUnknownStartTurnFields(fields, startTurnRequestFields); err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	request := controlapp.StartTurnRequest{ThreadID: threadID}
	for key, target := range map[string]*string{
		"prompt": &request.Prompt, "displayText": &request.DisplayText,
		"model": &request.Model, "providerId": &request.ProviderID,
		"endpointFormat": &request.EndpointFormat, "reasoningEffort": &request.ReasoningEffort,
		"mode": &request.Mode, "approvalPolicy": &request.ApprovalPolicy,
		"sandboxMode": &request.SandboxMode, "workspaceCheckpointId": &request.WorkspaceCheckpointID,
	} {
		if err := decodeStartTurnString(fields, key, target); err != nil {
			return controlapp.StartTurnRequest{}, err
		}
	}
	if _, ok := fields["prompt"]; !ok || strings.TrimSpace(request.Prompt) == "" {
		return controlapp.StartTurnRequest{}, errors.New("prompt must be a non-empty string")
	}
	if _, present := fields["reasoningEffort"]; present {
		if err := domainmodel.ValidateReasoningEffortV1(request.ReasoningEffort); err != nil {
			return controlapp.StartTurnRequest{}, errors.New("reasoningEffort contains an unsupported value")
		}
	}
	if err := validateStartTurnEnum(fields, "mode", request.Mode, "agent", "plan"); err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	if err := validateStartTurnEnum(fields, "approvalPolicy", request.ApprovalPolicy, "always", "auto", "on-request", "untrusted", "suggest", "never"); err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	if err := validateStartTurnEnum(fields, "sandboxMode", request.SandboxMode, "read-only", "workspace-write", "danger-full-access", "external-sandbox"); err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	if raw, ok := fields["riskIntent"]; ok {
		if err := decodeRequiredJSON(raw, &request.RiskIntent); err != nil || request.RiskIntent != "case" {
			return controlapp.StartTurnRequest{}, errors.New("riskIntent must be the literal case")
		}
	}
	if err := decodeStartTurnBool(fields, "async", &request.Async, nil); err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	if err := decodeStartTurnBool(fields, "disableUserInput", &request.DisableUserInput, &request.DisableUserInputSet); err != nil {
		return controlapp.StartTurnRequest{}, err
	}
	if raw, ok := fields["attachmentIds"]; ok {
		request.AttachmentIDs, err = decodeStartTurnStringList(raw, "attachmentIds")
		if err != nil {
			return controlapp.StartTurnRequest{}, err
		}
	}
	if raw, ok := fields["fileReferences"]; ok {
		request.FileReferences, err = decodeStartTurnFileReferences(raw)
		if err != nil {
			return controlapp.StartTurnRequest{}, err
		}
	}
	if raw, ok := fields["guiPlan"]; ok {
		request.GUIPlan, err = decodeStartTurnGUIPlan(raw)
		if err != nil {
			return controlapp.StartTurnRequest{}, err
		}
	}
	if raw, ok := fields["maxModelSteps"]; ok {
		steps, parseErr := decodeStartTurnModelSteps(raw)
		if parseErr != nil {
			return controlapp.StartTurnRequest{}, parseErr
		}
		request.MaxModelSteps = &steps
	}
	return controlapp.NormalizeStartTurnRequest(request), nil
}

func rejectUnknownStartTurnFields(fields map[string]json.RawMessage, allowed map[string]struct{}) error {
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown start turn field %q", key)
		}
	}
	return nil
}

func decodeStartTurnString(fields map[string]json.RawMessage, key string, target *string) error {
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	if err := decodeRequiredJSON(raw, target); err != nil {
		return fmt.Errorf("%s must be a string", key)
	}
	return nil
}

func validateStartTurnEnum(fields map[string]json.RawMessage, key, value string, allowed ...string) error {
	if _, ok := fields[key]; !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s contains an unsupported value", key)
}

func decodeStartTurnBool(fields map[string]json.RawMessage, key string, target *bool, present *bool) error {
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	if err := decodeRequiredJSON(raw, target); err != nil {
		return fmt.Errorf("%s must be a boolean", key)
	}
	if present != nil {
		*present = true
	}
	return nil
}

func decodeRequiredJSON(raw json.RawMessage, target any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("null is not allowed")
	}
	return json.Unmarshal(raw, target)
}

func decodeStartTurnStringList(raw json.RawMessage, name string) ([]string, error) {
	var items []json.RawMessage
	if err := decodeRequiredJSON(raw, &items); err != nil || len(items) > maxStartTurnListItemCount {
		return nil, fmt.Errorf("%s must be a bounded string array", name)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		var value string
		if decodeRequiredJSON(item, &value) != nil {
			return nil, fmt.Errorf("%s must contain only strings", name)
		}
		result = append(result, value)
	}
	return result, nil
}

func decodeStartTurnModelSteps(raw json.RawMessage) (int, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, errors.New("maxModelSteps must be an integer")
	}
	rational, ok := new(big.Rat).SetString(string(bytes.TrimSpace(raw)))
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, errors.New("maxModelSteps must be an integer")
	}
	steps := rational.Num().Int64()
	if steps < 0 || steps > maxStartTurnModelSteps {
		return 0, errors.New("maxModelSteps is outside the supported range")
	}
	return int(steps), nil
}

func decodeStartTurnFileReferences(raw json.RawMessage) ([]any, error) {
	var items []json.RawMessage
	if err := decodeRequiredJSON(raw, &items); err != nil || len(items) > maxStartTurnListItemCount {
		return nil, errors.New("fileReferences must be a bounded array")
	}
	result := make([]any, 0, len(items))
	allowed := map[string]struct{}{"path": {}, "relativePath": {}, "name": {}, "kind": {}}
	for _, item := range items {
		fields, err := domainjsonstrict.DecodeRawObject(item, domainjsonstrict.Options{MaxBytes: maxStartTurnRequestBytes, MaxDepth: 4, MaxTokens: 16, MaxStringBytes: maxStartTurnJSONString})
		if err != nil || rejectUnknownStartTurnFields(fields, allowed) != nil {
			return nil, errors.New("fileReferences contains an invalid object")
		}
		record := map[string]any{}
		for _, key := range []string{"path", "relativePath", "name"} {
			var value string
			if rawValue, exists := fields[key]; !exists || decodeRequiredJSON(rawValue, &value) != nil || strings.TrimSpace(value) == "" {
				return nil, errors.New("fileReferences contains an invalid required field")
			}
			record[key] = value
		}
		if rawKind, exists := fields["kind"]; exists {
			var kind string
			if decodeRequiredJSON(rawKind, &kind) != nil {
				return nil, errors.New("fileReferences kind must be a string")
			}
			kind = strings.TrimSpace(kind)
			if kind != "file" && kind != "directory" {
				return nil, errors.New("fileReferences kind is invalid")
			}
			record["kind"] = kind
		}
		result = append(result, record)
	}
	return result, nil
}

func decodeStartTurnGUIPlan(raw json.RawMessage) (map[string]any, error) {
	fields, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{MaxBytes: maxStartTurnRequestBytes, MaxDepth: 4, MaxTokens: 24, MaxStringBytes: maxStartTurnJSONString})
	allowed := map[string]struct{}{
		"operation": {}, "workspaceRoot": {}, "relativePath": {}, "planId": {}, "sourceRequest": {}, "title": {},
	}
	if err != nil || rejectUnknownStartTurnFields(fields, allowed) != nil {
		return nil, errors.New("guiPlan must be a closed object")
	}
	result := map[string]any{}
	for _, key := range []string{"operation", "workspaceRoot", "relativePath", "planId"} {
		var value string
		if rawValue, exists := fields[key]; !exists || decodeRequiredJSON(rawValue, &value) != nil || strings.TrimSpace(value) == "" {
			return nil, errors.New("guiPlan contains an invalid required field")
		}
		result[key] = value
	}
	operation := result["operation"].(string)
	if operation != "draft" && operation != "refine" {
		return nil, errors.New("guiPlan operation is invalid")
	}
	if !planapp.IsGUIPlanRelativePath(result["relativePath"].(string)) {
		return nil, errors.New("guiPlan relativePath is invalid")
	}
	for _, key := range []string{"sourceRequest", "title"} {
		if rawValue, exists := fields[key]; exists {
			var value string
			if decodeRequiredJSON(rawValue, &value) != nil {
				return nil, fmt.Errorf("guiPlan %s must be a string", key)
			}
			result[key] = value
		}
	}
	return result, nil
}

func WriteStartTurnResult(w http.ResponseWriter, response map[string]any, err error) {
	if err != nil {
		if errors.Is(err, controlapp.ErrMissingThreadID) ||
			errors.Is(err, controlapp.ErrMissingPrompt) {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		if errors.Is(err, controlapp.ErrThreadNotFound) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if errors.Is(err, controlapp.ErrAttachmentNotAuthorized) {
			WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": err.Error()})
			return
		}
		if errors.Is(err, controlapp.ErrTurnExecutionConflict) || errors.Is(err, controlapp.ErrThreadTransition) || errors.Is(err, controlapp.ErrTerminalArbitration) {
			WriteJSON(w, http.StatusConflict, map[string]any{"code": "turn_execution_conflict", "message": "turn execution authority is already active"})
			return
		}
		if errors.Is(err, controlapp.ErrRuntimeShuttingDown) {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"code": "runtime_shutting_down", "message": "runtime is shutting down"})
			return
		}
		publicFailure := loopapp.PublicFailureForError(err)
		response := map[string]any{
			"code":       "turn_failed",
			"reasonCode": publicFailure.Code(),
			"message":    publicFailure.Message(),
		}
		if details := publicFailure.Details(); publicFailure.Code() == "tool_not_advertised" &&
			domainfailure.ValidateToolNotAdvertisedDetails(details) {
			response["details"] = details
		}
		if diagnostic := loopapp.ProviderErrorDiagnostic(err); diagnostic != nil {
			response["providerError"] = diagnostic
		}
		WriteJSON(w, http.StatusInternalServerError, response)
		return
	}
	WriteJSON(w, http.StatusAccepted, response)
}
