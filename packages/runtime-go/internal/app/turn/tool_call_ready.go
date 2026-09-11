package turn

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
)

type ToolCallReadyInput struct {
	ThreadID   string
	TurnID     string
	ItemID     string
	CreatedAt  string
	Call       domainmodel.ToolCall
	ReadyCount int
	ToolKind   string
	Context    domainsecurity.TurnSecurityContext
	Grant      domainsecurity.ExecutionGrant
}

type ToolCallReadyStore interface {
	AppendItemToTurn(string, string, map[string]any) error
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

func ToolCallReadyRecords(input ToolCallReadyInput) (map[string]any, map[string]any, error) {
	if err := executiongrantapp.ValidateExecutionGrantForCall(input.Context, input.Grant, input.Call); err != nil {
		return nil, nil, errors.New("tool call execution grant is invalid")
	}
	if !domainsecurity.IsHostToolCallIDV1(input.Call.ID) {
		return nil, nil, errors.New("tool call identity is not host-issued")
	}
	expectedItemID := domaintoolcall.ToolCallItemIDV1(input.Context.TurnID, input.Call.ID)
	if expectedItemID == "" || input.ItemID != expectedItemID {
		return nil, nil, errors.New("tool call item identity is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(input.Call.Arguments))
	decoder.UseNumber()
	args := map[string]any{}
	if err := decoder.Decode(&args); err != nil {
		return nil, nil, errors.New("tool call arguments are invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, nil, errors.New("tool call arguments contain trailing JSON")
	}
	if strings.TrimSpace(input.ThreadID) != input.Context.ThreadID || strings.TrimSpace(input.TurnID) != input.Context.TurnID ||
		input.Grant.TurnID != input.Context.TurnID || input.Grant.ContextDigest != input.Context.ContextDigest ||
		input.Grant.ToolName != strings.TrimSpace(input.Call.Name) || input.Grant.ToolCallID != strings.TrimSpace(input.Call.ID) ||
		input.Grant.ArgsHash != domainsecurity.CanonicalJSONHash(input.Call.Arguments) {
		return nil, nil, errors.New("tool call execution authority does not match the call")
	}
	item := map[string]any{
		"id":               expectedItemID,
		"turnId":           strings.TrimSpace(input.TurnID),
		"threadId":         strings.TrimSpace(input.ThreadID),
		"role":             "tool",
		"status":           "pending",
		"createdAt":        strings.TrimSpace(input.CreatedAt),
		"kind":             "tool_call",
		"toolName":         strings.TrimSpace(input.Call.Name),
		"callId":           strings.TrimSpace(input.Call.ID),
		"toolKind":         strings.TrimSpace(input.ToolKind),
		"arguments":        domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(),
		"contextDigest":    strings.TrimSpace(input.Context.ContextDigest),
		"contextEpoch":     float64(input.Context.ContextEpoch),
		"executionGrantId": strings.TrimSpace(input.Grant.GrantID),
		"executionGrant":   contractRecord(input.Grant),
	}
	event := map[string]any{
		"kind":           "tool_call_ready",
		"threadId":       strings.TrimSpace(input.ThreadID),
		"turnId":         strings.TrimSpace(input.TurnID),
		"itemId":         expectedItemID,
		"callId":         strings.TrimSpace(input.Call.ID),
		"toolName":       strings.TrimSpace(input.Call.Name),
		"readyCount":     float64(input.ReadyCount),
		"contextDigest":  strings.TrimSpace(input.Context.ContextDigest),
		"contextEpoch":   float64(input.Context.ContextEpoch),
		"executionGrant": contractRecord(input.Grant),
	}
	return item, event, nil
}

func PersistToolCallReady(store ToolCallReadyStore, input ToolCallReadyInput) (string, error) {
	item, event, err := ToolCallReadyRecords(input)
	if err != nil {
		return "", err
	}
	if err := store.AppendItemToTurn(input.ThreadID, input.TurnID, item); err != nil {
		return "", err
	}
	if _, _, err := store.RecordEvent(event); err != nil {
		return "", err
	}
	return input.ItemID, nil
}

func contractRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}
