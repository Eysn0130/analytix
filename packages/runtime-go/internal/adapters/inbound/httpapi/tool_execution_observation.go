package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	sideeffectidentityapp "analytix.local/runtime-go/internal/app/sideeffectidentity"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

const maxToolExecutionObservationRequestBytesV1 = 4 << 20

var toolExecutionObservationRequestFieldsV1 = map[string]struct{}{
	"threadId": {}, "turnId": {}, "toolName": {}, "workspace": {}, "arguments": {},
}

type SuccessfulToolExecutionObserverV1 interface {
	ObserveSuccessfulToolExecutionV1(
		context.Context,
		pendingworkapp.SuccessfulToolExecutionObservationInputV1,
	) (domainpendingwork.SuccessfulToolExecutionObservationV1, error)
}

type ToolExecutionObservationHandlersV1 struct {
	Service             SuccessfulToolExecutionObserverV1
	ResolveMutationPath sideeffectidentityapp.PathResolver
	ResolveReadPath     sideeffectidentityapp.PathResolver
}

func (handlers ToolExecutionObservationHandlersV1) Handle(w http.ResponseWriter, r *http.Request) {
	toolExecutionObservationNoStoreV1(w)
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if handlers.Service == nil || handlers.ResolveMutationPath == nil || handlers.ResolveReadPath == nil {
		writeToolExecutionObservationUnavailableV1(w)
		return
	}
	body, err := readToolExecutionObservationBodyV1(r)
	if err != nil {
		writeToolExecutionObservationInvalidV1(w)
		return
	}
	defer clearToolExecutionObservationBytesV1(body)
	input, err := decodeToolExecutionObservationRequestV1(
		body,
		handlers.ResolveMutationPath,
		handlers.ResolveReadPath,
	)
	if err != nil {
		writeToolExecutionObservationInvalidV1(w)
		return
	}
	observation, err := handlers.Service.ObserveSuccessfulToolExecutionV1(r.Context(), input)
	switch {
	case err == nil:
		if domainpendingwork.ValidateSuccessfulToolExecutionObservationV1(observation) != nil {
			writeToolExecutionObservationUnavailableV1(w)
			return
		}
		WriteJSON(w, http.StatusOK, observation)
	case errors.Is(err, pendingworkapp.ErrToolExecutionObservationInvalid):
		writeToolExecutionObservationInvalidV1(w)
	case errors.Is(err, pendingworkapp.ErrToolExecutionNotObserved):
		WriteJSON(w, http.StatusNotFound, map[string]any{
			"code": "not_found", "message": "successful tool execution was not observed",
		})
	default:
		writeToolExecutionObservationUnavailableV1(w)
	}
}

func readToolExecutionObservationBodyV1(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, errors.New("tool execution observation body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxToolExecutionObservationRequestBytesV1+1))
	if err != nil || len(body) == 0 || len(body) > maxToolExecutionObservationRequestBytesV1 {
		return nil, errors.New("tool execution observation body is invalid")
	}
	return body, nil
}

func decodeToolExecutionObservationRequestV1(
	body []byte,
	resolveMutationPath sideeffectidentityapp.PathResolver,
	resolveReadPath sideeffectidentityapp.PathResolver,
) (pendingworkapp.SuccessfulToolExecutionObservationInputV1, error) {
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		MaxBytes: maxToolExecutionObservationRequestBytesV1, MaxDepth: 32, MaxTokens: 200_000,
		MaxStringBytes: 1 << 20, MaxNumberBytes: 64, MaxAbsExponent: 10_000,
	})
	if err != nil || len(fields) != len(toolExecutionObservationRequestFieldsV1) {
		return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation request is invalid")
	}
	for key := range fields {
		if _, allowed := toolExecutionObservationRequestFieldsV1[key]; !allowed {
			return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation request is invalid")
		}
	}
	input := pendingworkapp.SuccessfulToolExecutionObservationInputV1{}
	for key, target := range map[string]*string{
		"threadId": &input.ThreadID, "turnId": &input.TurnID,
		"toolName": &input.ToolName, "workspace": &input.ExpectedWorkspace,
	} {
		if err := json.Unmarshal(fields[key], target); err != nil || *target == "" || *target != strings.TrimSpace(*target) {
			return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation request is invalid")
		}
	}
	if input.ToolName != "bash" && input.ToolName != "read" {
		return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation tool is not allowed")
	}
	input.Arguments = append(json.RawMessage(nil), fields["arguments"]...)
	arguments, err := domainjsonstrict.DecodeRawObject(input.Arguments, domainjsonstrict.Options{
		MaxBytes: maxToolExecutionObservationRequestBytesV1, MaxDepth: 16, MaxTokens: 50_000,
		MaxStringBytes: 1 << 20, MaxNumberBytes: 64, MaxAbsExponent: 10_000,
	})
	if err != nil {
		return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation arguments are invalid")
	}
	if input.ToolName == "bash" {
		if toolExecutionObservationBackgroundRequestedV1(arguments) {
			return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("background bash cannot satisfy execution observation")
		}
		input.SemanticIdentity, err = sideeffectidentityapp.ResolveV1(sideeffectidentityapp.Input{
			ToolName: input.ToolName, Arguments: input.Arguments,
			WorkspaceRealPath: input.ExpectedWorkspace, ResolvePath: resolveMutationPath,
		})
		if err != nil {
			return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation arguments are invalid")
		}
		return input, nil
	}
	rawPath, present := arguments["path"]
	if !present {
		return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation read path is invalid")
	}
	var path string
	if err := json.Unmarshal(rawPath, &path); err != nil ||
		!pendingworkapp.ValidSuccessfulToolExecutionObservationReadPathV1(path) {
		return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation read path is invalid")
	}
	resolved, ok := resolveReadPath(input.ExpectedWorkspace, path)
	input.ReadPathResolved = ok && strings.TrimSpace(resolved) != ""
	if !input.ReadPathResolved {
		return pendingworkapp.SuccessfulToolExecutionObservationInputV1{}, errors.New("tool execution observation read path is invalid")
	}
	return input, nil
}

func toolExecutionObservationBackgroundRequestedV1(arguments map[string]json.RawMessage) bool {
	for _, key := range []string{"run_in_background", "runInBackground"} {
		raw, present := arguments[key]
		if !present {
			continue
		}
		var enabled bool
		if json.Unmarshal(raw, &enabled) != nil || enabled {
			return true
		}
	}
	return false
}

func writeToolExecutionObservationInvalidV1(w http.ResponseWriter) {
	WriteJSON(w, http.StatusBadRequest, map[string]any{
		"code": "validation_error", "message": "invalid tool execution observation request",
	})
}

func writeToolExecutionObservationUnavailableV1(w http.ResponseWriter) {
	WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
		"code": "internal_error", "message": "tool execution observation authority is unavailable",
	})
}

func toolExecutionObservationNoStoreV1(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func clearToolExecutionObservationBytesV1(body []byte) {
	for index := range body {
		body[index] = 0
	}
}

func toolExecutionObservationAuthorizedV1(r *http.Request, token string) bool {
	if r == nil || token == "" {
		return false
	}
	values := r.Header.Values("Authorization")
	return len(values) == 1 && values[0] == "Bearer "+token
}

func toolExecutionObservationCanonicalRequestTargetV1(r *http.Request) bool {
	return r != nil && r.URL != nil && r.RequestURI == toolExecutionObservationPathV1 &&
		r.URL.Path == toolExecutionObservationPathV1 && r.URL.EscapedPath() == toolExecutionObservationPathV1 &&
		r.URL.RawQuery == "" && !r.URL.ForceQuery && r.URL.Opaque == "" && r.URL.Fragment == "" && r.URL.RawFragment == ""
}
