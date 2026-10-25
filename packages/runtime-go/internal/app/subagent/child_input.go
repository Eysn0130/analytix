package subagent

import (
	"encoding/json"
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ChildInputRequestV1 is an intent, never execution authority. Only the Host
// control entry may dispatch it; the model task_steer entry remains unchanged.
type ChildInputRequestV1 struct {
	Mode                string
	ParentThreadID      string
	JobID               string
	ExpectedRunID       string
	ExpectedChildTurnID string
	Message             string
	ClientMessageID     string
}

const (
	ChildInputInspectV1      = "inspect"
	ChildInputStartOrSteerV1 = "start_or_steer"
)

// ChildInputRequestFromArgsV1 retains the existing task-job aliases, while
// rejecting duplicate aliases and caller-supplied source/authority fields.
func ChildInputRequestFromArgsV1(args map[string]any) (ChildInputRequestV1, error) {
	var request ChildInputRequestV1
	allowed := map[string]bool{
		"mode": true, "parentThreadId": true, "threadId": true,
		"job_id": true, "jobId": true, "id": true,
		"expectedRunId": true, "expectedChildTurnId": true,
		"message": true, "text": true, "client_message_id": true, "clientMessageId": true,
	}
	for key := range args {
		if !allowed[key] {
			return request, errors.New("child input contains an unsupported field")
		}
	}
	fields := []struct {
		target *string
		keys   []string
	}{
		{&request.Mode, []string{"mode"}},
		{&request.ParentThreadID, []string{"parentThreadId", "threadId"}},
		{&request.JobID, []string{"job_id", "jobId", "id"}},
		{&request.ExpectedRunID, []string{"expectedRunId"}},
		{&request.ExpectedChildTurnID, []string{"expectedChildTurnId"}},
		{&request.Message, []string{"message", "text"}},
		{&request.ClientMessageID, []string{"client_message_id", "clientMessageId"}},
	}
	for _, field := range fields {
		value, invalid := taskJobSteerStringArg(args, field.keys...)
		if invalid {
			return ChildInputRequestV1{}, errors.New("child input has conflicting or invalid aliases")
		}
		*field.target = value
	}
	if err := ValidateChildInputRequestV1(request); err != nil {
		return ChildInputRequestV1{}, err
	}
	return request, nil
}

func ValidateChildInputRequestV1(request ChildInputRequestV1) error {
	for _, identity := range []string{request.ParentThreadID, request.JobID} {
		if !childInputIdentityV1(identity) {
			return errors.New("child input parent and job identities are required")
		}
	}
	switch request.Mode {
	case ChildInputInspectV1:
		if request.Message != "" || request.ClientMessageID != "" || request.ExpectedRunID != "" || request.ExpectedChildTurnID != "" {
			return errors.New("child input inspection cannot contain an input")
		}
	case ChildInputStartOrSteerV1:
		if !childInputIdentityV1(request.ExpectedRunID) || !childInputIdentityV1(request.ExpectedChildTurnID) ||
			!contracts.IsCanonicalOpaqueUUIDV4(request.ClientMessageID) || request.Message == "" ||
			request.Message != strings.TrimSpace(request.Message) || len([]rune(request.Message)) > DefaultTaskJobSteerMaxRunes {
			return errors.New("child input requires an exact run, turn, message and opaque client identity")
		}
	default:
		return errors.New("child input mode is invalid")
	}
	return nil
}

func childInputIdentityV1(value string) bool {
	if value == "" || len(value) > 128 || value != strings.TrimSpace(value) {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}

// ChildInputDigestV1 binds the caller's input and lineage, independent of the
// later delivery choice. Retrying across active/idle may never dispatch twice.
func ChildInputDigestV1(request ChildInputRequestV1) string {
	body, _ := json.Marshal(map[string]any{
		"schemaVersion": 1, "parentThreadId": request.ParentThreadID, "sourceJobId": request.JobID,
		"expectedRunId": request.ExpectedRunID, "expectedChildTurnId": request.ExpectedChildTurnID,
		"clientMessageId": request.ClientMessageID, "message": request.Message,
	})
	return domainsecurity.CanonicalJSONHash(body)
}

// ChildInputCapabilitiesV1 reports an observed control seam. It is advisory;
// every submission must repeat current Host authority and admission checks.
type ChildInputCapabilitiesV1 struct {
	Send          string `json:"send"`
	Stop          bool   `json:"stop"`
	RunID         string `json:"runId,omitempty"`
	ChildThreadID string `json:"childThreadId,omitempty"`
	ChildTurnID   string `json:"childTurnId,omitempty"`
	ReasonCode    string `json:"reasonCode,omitempty"`
}

type ChildInputReceiptV1 struct {
	Action          string `json:"action"`
	Admission       string `json:"admission"`
	ClientMessageID string `json:"clientMessageId"`
	JobID           string `json:"jobId"`
	ChildThreadID   string `json:"childThreadId"`
	ChildTurnID     string `json:"childTurnId"`
	MessageID       string `json:"messageId"`
}

type ChildInputResponseV1 struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Capabilities  ChildInputCapabilitiesV1 `json:"capabilities"`
	Receipt       *ChildInputReceiptV1     `json:"receipt,omitempty"`
	Code          string                   `json:"code,omitempty"`
}

// DecodeChildInputResponseV1 is the closed private HTTP boundary. Neither an
// underlying error body nor an authority structure can be serialized here.
func DecodeChildInputResponseV1(value any) (ChildInputResponseV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ChildInputResponseV1{}, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || string(fields["schemaVersion"]) != "1" {
		return ChildInputResponseV1{}, errors.New("child input response version is required")
	}
	var capsFields map[string]json.RawMessage
	if json.Unmarshal(fields["capabilities"], &capsFields) != nil || capsFields["send"] == nil || (string(capsFields["stop"]) != "true" && string(capsFields["stop"]) != "false") {
		return ChildInputResponseV1{}, errors.New("child input capabilities require send and stop")
	}
	if raw, exists := fields["receipt"]; exists && string(raw) == "null" {
		return ChildInputResponseV1{}, errors.New("child input receipt cannot be null")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var response ChildInputResponseV1
	if err := decoder.Decode(&response); err != nil || response.SchemaVersion != 1 {
		return ChildInputResponseV1{}, errors.New("child input response is invalid")
	}
	c := response.Capabilities
	if c.Send != "steer" && c.Send != "start" && c.Send != "disabled" {
		return ChildInputResponseV1{}, errors.New("child input capabilities are invalid")
	}
	for _, id := range []string{c.RunID, c.ChildThreadID, c.ChildTurnID} {
		if id != "" && !childInputIdentityV1(id) {
			return ChildInputResponseV1{}, errors.New("child control identity is invalid")
		}
	}
	if (c.RunID != "" || c.ChildThreadID != "" || c.ChildTurnID != "" || c.Send != "disabled" || c.Stop) && (c.RunID == "" || c.ChildThreadID == "" || c.ChildTurnID == "") {
		return ChildInputResponseV1{}, errors.New("child control identities are incomplete")
	}
	if c.Send == "steer" && !c.Stop || c.Send == "start" && c.Stop {
		return ChildInputResponseV1{}, errors.New("child input control state is inconsistent")
	}
	if !childInputCodeV1(response.Code) || !childInputReasonV1(c.ReasonCode) {
		return ChildInputResponseV1{}, errors.New("child control reason is invalid")
	}
	if r := response.Receipt; r != nil {
		if response.Code != "" || r.Action == "start" && r.Admission != "admitted" || c.ChildThreadID != "" && r.ChildThreadID != c.ChildThreadID {
			return ChildInputResponseV1{}, errors.New("child input receipt is inconsistent with its response")
		}
		if (r.Action != "start" && r.Action != "steer") || (r.Admission != "queued" && r.Admission != "admitted") || !contracts.IsCanonicalOpaqueUUIDV4(r.ClientMessageID) {
			return ChildInputResponseV1{}, errors.New("child input receipt state is invalid")
		}
		for _, id := range []string{r.JobID, r.ChildThreadID, r.ChildTurnID, r.MessageID} {
			if !childInputIdentityV1(id) {
				return ChildInputResponseV1{}, errors.New("child input receipt identity is invalid")
			}
		}
	}
	return response, nil
}

func childInputCodeV1(code string) bool {
	switch code {
	case "", "conflict", "new_turn_required", "forbidden", "not_found", "validation_error", "child_input_unknown", "runtime_shutting_down", "input_cancelled":
		return true
	default:
		return false
	}
}

func childInputReasonV1(code string) bool {
	switch code {
	case "", "authority_unavailable", "run_conflict", "pending_start", "paused", "turn_terminalizing", "thread_transition", "new_turn_required", "case_risk_raise", "context_changed", "child_input_unknown", "run_stopped", "run_unavailable", "runtime_shutting_down", "input_cancelled":
		return true
	default:
		return false
	}
}
