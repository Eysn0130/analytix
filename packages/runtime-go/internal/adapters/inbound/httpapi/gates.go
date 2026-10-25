package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	displayport "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

type GateControl interface {
	ApproveTool(context.Context, controlapp.ApprovalDecision) (controlapp.ActionResult, error)
	RespondUserInput(context.Context, controlapp.UserInputResponse) (controlapp.ActionResult, error)
}

type UserInputWaitControl interface {
	AttachUserInputWait(context.Context, string, func(*int64) error) error
}

type GateHandlers struct {
	Control GateControl
	Wait    UserInputWaitControl
}

func (h GateHandlers) HandleApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	var request struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if len(body) == 0 || json.Unmarshal(body, &request) != nil || (request.Decision != "allow" && request.Decision != "deny") {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid approval body"})
		return
	}
	if h.Control == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_control_missing", "message": "runtime control missing"})
		return
	}
	result, err := h.Control.ApproveTool(r.Context(), controlapp.ApprovalDecision{
		ApprovalID: strings.TrimPrefix(r.URL.Path, "/v1/approvals/"),
		Decision:   request.Decision,
		Reason:     request.Reason,
	})
	WriteControlActionResult(w, result, err)
}

func (h GateHandlers) HandleUserInput(w http.ResponseWriter, r *http.Request, prefix string) {
	if strings.HasSuffix(r.URL.Path, "/wait") {
		h.handleUserInputWait(w, r, prefix)
		return
	}
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	request, valid := decodeUserInputResponse(body)
	if !valid {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid user input body"})
		return
	}
	if h.Control == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_control_missing", "message": "runtime control missing"})
		return
	}
	request.InputID = strings.TrimPrefix(r.URL.Path, prefix)
	if request.Selector != nil && request.Selector.RequestID != request.InputID {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid user input binding"})
		return
	}
	result, err := h.Control.RespondUserInput(r.Context(), request)
	WriteControlActionResult(w, result, err)
}

// Decode both published answer carriers without accepting a mixed batch or
// client-supplied extensions. Duplicate JSON keys are rejected before decoding.
func decodeUserInputResponse(body []byte) (controlapp.UserInputResponse, bool) {
	var response controlapp.UserInputResponse
	if jsonstrict.Validate(body, jsonstrict.Options{RequireObject: true, MaxDepth: 5}) != nil {
		return response, false
	}
	var envelope struct {
		Selector  *displayport.SelectorV1          `json:"selector"`
		LateReply *controlapp.UserInputLateReplyV1 `json:"lateReply"`
		Action    string                           `json:"action"`
		Answers   json.RawMessage                  `json:"answers"`
		Cancelled bool                             `json:"cancelled"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil {
		return response, false
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	for key := range fields {
		if key != "answers" && key != "cancelled" && key != "action" && key != "lateReply" && key != "selector" {
			return response, false
		}
	}
	if raw, exists := fields["selector"]; exists {
		var selectorFields map[string]json.RawMessage
		if json.Unmarshal(raw, &selectorFields) != nil || len(selectorFields) != 4 || envelope.Selector == nil || !displayport.ValidateSelectorV1(*envelope.Selector) {
			return response, false
		}
		for key := range selectorFields {
			if key != "threadId" && key != "turnId" && key != "itemId" && key != "requestId" {
				return response, false
			}
		}
		response.Selector = envelope.Selector
	}
	if raw, exists := fields["cancelled"]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return response, false
	}
	if raw, exists := fields["action"]; exists {
		expectedFields := 1
		if response.Selector != nil {
			expectedFields++
		}
		if len(fields) != expectedFields || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || envelope.Action != "continue" {
			return response, false
		}
		response.Continue = true
		return response, true
	}
	if raw, exists := fields["lateReply"]; exists {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || envelope.LateReply == nil || !controlapp.IsOpaqueClientUserMessageID(envelope.LateReply.ClientUserMessageID) || strings.TrimSpace(envelope.LateReply.ExpectedTurnID) == "" || envelope.LateReply.ExpectedTurnID != strings.TrimSpace(envelope.LateReply.ExpectedTurnID) || envelope.Cancelled {
			return response, false
		}
		response.LateReply = envelope.LateReply
	}
	response.Cancelled = envelope.Cancelled
	if envelope.Cancelled {
		return response, len(envelope.Answers) == 0
	}
	if len(envelope.Answers) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Answers), []byte("null")) {
		return response, false
	}
	var records []map[string]json.RawMessage
	if json.Unmarshal(envelope.Answers, &records) != nil {
		return response, false
	}
	typed := false
	for index, record := range records {
		if record == nil {
			return response, false
		}
		_, hasKind := record["kind"]
		if index == 0 {
			typed = hasKind
		} else if typed != hasKind {
			return response, false
		}
		for key, value := range record {
			if typed {
				if key != "id" && key != "kind" && key != "optionIds" && key != "customText" {
					return response, false
				}
			} else if key != "id" && key != "label" && key != "value" {
				return response, false
			}
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return response, false
			}
		}
	}
	if typed {
		if json.Unmarshal(envelope.Answers, &response.StructuredAnswers) != nil {
			return response, false
		}
		for _, answer := range response.StructuredAnswers {
			if answer.ID == "" || (answer.Kind != "answer" && answer.Kind != "skip") {
				return response, false
			}
			if answer.Kind == "skip" && (answer.OptionIDs != nil || answer.CustomText != nil) {
				return response, false
			}
			if answer.Kind == "answer" && answer.OptionIDs == nil {
				return response, false
			}
		}
	} else if json.Unmarshal(envelope.Answers, &response.Answers) != nil {
		return response, false
	}
	return response, true
}

// One first frame plus a held attachment lifetime, matching DSH attachWait.
// The Client sends the explicit continue command when its local timer expires,
// and retains this stream until the Host claims the foreground completion.
func (h GateHandlers) handleUserInputWait(w http.ResponseWriter, r *http.Request, prefix string) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Wait == nil {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "timed_wait_unavailable"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "wait_stream_unavailable"})
		return
	}
	inputID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/wait")
	started := false
	err := h.Wait.AttachUserInputWait(r.Context(), inputID, func(remaining *int64) error {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		body, err := json.Marshal(map[string]any{"remainingMs": remaining})
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
			return err
		}
		started = true
		flusher.Flush()
		return nil
	})
	if err != nil && !started && r.Context().Err() == nil {
		WriteJSON(w, http.StatusConflict, map[string]any{"code": "timed_wait_unavailable"})
	}
}
