package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

var (
	steerTurnRequestFields = map[string]struct{}{
		"text": {}, "displayText": {}, "riskIntent": {}, "clientUserMessageId": {},
		"expectedTurnId": {}, "attachmentIds": {}, "fileReferences": {}, "delivery": {},
	}
	interruptTurnRequestFields = map[string]struct{}{"discard": {}}
)

func readTurnActionBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, errors.New("turn action body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxStartTurnRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxStartTurnRequestBytes {
		return nil, errors.New("turn action body exceeds size limit")
	}
	return body, nil
}

func writeInvalidTurnActionBody(w http.ResponseWriter, message string) {
	WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": message})
}

func decodeTurnActionObject(body []byte, allowed map[string]struct{}) (map[string]json.RawMessage, error) {
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		MaxBytes:       maxStartTurnRequestBytes,
		MaxDepth:       maxStartTurnJSONDepth,
		MaxTokens:      maxStartTurnJSONTokens,
		MaxStringBytes: maxStartTurnJSONString,
		MaxNumberBytes: 64,
		MaxAbsExponent: 10_000,
	})
	if err != nil {
		return nil, err
	}
	for key := range fields {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("unknown turn action field %q", key)
		}
	}
	return fields, nil
}

// DecodeSteerTurnRequest is the sole public HTTP decoder for mid-turn user
// steering. It validates a closed, bounded schema before any value reaches the
// controller. riskIntent is raise-only; publication policy remains host-owned.
func DecodeSteerTurnRequest(threadID, turnID string, body []byte) (controlapp.SteerTurnRequest, error) {
	fields, err := decodeTurnActionObject(body, steerTurnRequestFields)
	if err != nil {
		return controlapp.SteerTurnRequest{}, err
	}
	request := controlapp.SteerTurnRequest{ThreadID: threadID, TurnID: turnID}
	for key, target := range map[string]*string{
		"text": &request.Text, "displayText": &request.DisplayText,
		"clientUserMessageId": &request.ClientUserMessageID, "expectedTurnId": &request.ExpectedTurnID,
	} {
		if err := decodeStartTurnString(fields, key, target); err != nil {
			return controlapp.SteerTurnRequest{}, err
		}
	}
	if _, ok := fields["text"]; !ok || strings.TrimSpace(request.Text) == "" {
		return controlapp.SteerTurnRequest{}, errors.New("text must be a non-empty string")
	}
	for _, key := range []string{"clientUserMessageId", "expectedTurnId"} {
		if _, ok := fields[key]; ok {
			value := request.ClientUserMessageID
			if key == "expectedTurnId" {
				value = request.ExpectedTurnID
			}
			if strings.TrimSpace(value) == "" {
				return controlapp.SteerTurnRequest{}, fmt.Errorf("%s must be a non-empty string", key)
			}
		}
	}
	if raw, ok := fields["riskIntent"]; ok {
		if err := decodeRequiredJSON(raw, &request.RiskIntent); err != nil || request.RiskIntent != "case" {
			return controlapp.SteerTurnRequest{}, errors.New("riskIntent must be the literal case")
		}
	}
	if raw, ok := fields["delivery"]; ok {
		var delivery string
		if err := decodeRequiredJSON(raw, &delivery); err != nil || delivery != "steer" {
			return controlapp.SteerTurnRequest{}, errors.New("delivery must be the literal steer")
		}
		// Compatibility input only. The host writes the same fixed literal in
		// BuildSteeringEntry and never trusts caller-provided delivery state.
	}
	if raw, ok := fields["attachmentIds"]; ok {
		request.AttachmentIDs, err = decodeStartTurnStringList(raw, "attachmentIds")
		if err != nil {
			return controlapp.SteerTurnRequest{}, err
		}
		for _, value := range request.AttachmentIDs {
			if strings.TrimSpace(value) == "" {
				return controlapp.SteerTurnRequest{}, errors.New("attachmentIds must contain only non-empty strings")
			}
		}
	}
	if raw, ok := fields["fileReferences"]; ok {
		request.FileReferences, err = decodeStartTurnFileReferences(raw)
		if err != nil {
			return controlapp.SteerTurnRequest{}, err
		}
	}
	return controlapp.NormalizeSteerTurnRequest(request), nil
}

// DecodeInterruptTurnRequest accepts only an optional boolean discard flag.
func DecodeInterruptTurnRequest(threadID, turnID string, body []byte) (controlapp.InterruptTurnRequest, error) {
	fields, err := decodeTurnActionObject(body, interruptTurnRequestFields)
	if err != nil {
		return controlapp.InterruptTurnRequest{}, err
	}
	request := controlapp.InterruptTurnRequest{ThreadID: threadID, TurnID: turnID}
	if raw, ok := fields["discard"]; ok {
		if err := decodeRequiredJSON(raw, &request.Discard); err != nil {
			return controlapp.InterruptTurnRequest{}, errors.New("discard must be a boolean")
		}
	}
	return request, nil
}
