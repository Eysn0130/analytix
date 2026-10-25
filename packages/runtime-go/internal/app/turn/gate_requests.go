package turn

import (
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	"strings"
	"time"

	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
)

type ApprovalRequestInput struct {
	ThreadID              string
	TurnID                string
	ItemID                string
	ApprovalID            string
	CreatedAt             string
	ToolName              string
	ApprovalPolicy        string
	SandboxMode           string
	ContinuationReceiptID string
}

// TimedWaitViewV1 is closed metadata derived from the signed request.
// DeadlineAt is an actual JSON null for an unlimited call-bound wait.
type TimedWaitViewV1 struct {
	TimeoutSeconds int     `json:"timeoutSeconds"`
	DeadlineAt     *string `json:"deadlineAt"`
}

func UserInputTimedWaitViewV1(wait *domaincontinuation.UserInputTimedWaitV1, issuedAt string) *TimedWaitViewV1 {
	if wait == nil {
		return nil
	}
	issued, err := time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil || !domaincontinuation.ValidUserInputTimeoutSeconds(wait.TimeoutSeconds) {
		return nil
	}
	view := &TimedWaitViewV1{TimeoutSeconds: wait.TimeoutSeconds}
	if wait.TimeoutSeconds != -1 {
		deadline := issued.Add(time.Duration(wait.TimeoutSeconds) * time.Second).UTC().Format(time.RFC3339Nano)
		view.DeadlineAt = &deadline
	}
	return view
}

type UserInputRequestInput struct {
	TimedWait             *TimedWaitViewV1
	ThreadID              string
	TurnID                string
	ItemID                string
	InputID               string
	CreatedAt             string
	Prompt                string
	Questions             []map[string]any
	ContinuationReceiptID string
}

func ApprovalRequestRecords(input ApprovalRequestInput) (map[string]any, map[string]any) {
	toolName := pendingGateToolName(input.ToolName)
	receiptID := strings.TrimSpace(input.ContinuationReceiptID)
	item := map[string]any{
		"id":                    strings.TrimSpace(input.ItemID),
		"turnId":                strings.TrimSpace(input.TurnID),
		"threadId":              strings.TrimSpace(input.ThreadID),
		"role":                  "tool",
		"status":                "pending",
		"createdAt":             strings.TrimSpace(input.CreatedAt),
		"kind":                  "approval",
		"approvalId":            strings.TrimSpace(input.ApprovalID),
		"toolName":              toolName,
		"summary":               "Approve " + toolName,
		"continuationReceiptId": receiptID,
	}
	event := map[string]any{
		"kind":                  "approval_requested",
		"threadId":              strings.TrimSpace(input.ThreadID),
		"turnId":                strings.TrimSpace(input.TurnID),
		"itemId":                strings.TrimSpace(input.ItemID),
		"approvalId":            strings.TrimSpace(input.ApprovalID),
		"toolName":              toolName,
		"status":                "pending",
		"approvalPolicy":        strings.TrimSpace(input.ApprovalPolicy),
		"sandboxMode":           strings.TrimSpace(input.SandboxMode),
		"summary":               "Approve " + toolName,
		"continuationReceiptId": receiptID,
	}
	return item, event
}

func UserInputRequestRecords(input UserInputRequestInput) (map[string]any, map[string]any) {
	prompt := privacyprojectionapp.ProjectOrdinaryText(pendingGatePrompt(input.Prompt))
	questions := privacyprojectionapp.ProjectHostUserInputQuestions(input.InputID, input.Questions)
	receiptID := strings.TrimSpace(input.ContinuationReceiptID)
	item := map[string]any{
		"id":                    strings.TrimSpace(input.ItemID),
		"turnId":                strings.TrimSpace(input.TurnID),
		"threadId":              strings.TrimSpace(input.ThreadID),
		"role":                  "system",
		"status":                "pending",
		"createdAt":             strings.TrimSpace(input.CreatedAt),
		"kind":                  "user_input",
		"inputId":               strings.TrimSpace(input.InputID),
		"prompt":                prompt,
		"questions":             questions,
		"continuationReceiptId": receiptID,
	}
	event := map[string]any{
		"kind":                  "user_input_requested",
		"threadId":              strings.TrimSpace(input.ThreadID),
		"turnId":                strings.TrimSpace(input.TurnID),
		"itemId":                strings.TrimSpace(input.ItemID),
		"inputId":               strings.TrimSpace(input.InputID),
		"status":                "pending",
		"prompt":                prompt,
		"questions":             questions,
		"continuationReceiptId": receiptID,
	}
	if input.TimedWait != nil {
		var deadline any
		if input.TimedWait.DeadlineAt != nil {
			deadline = *input.TimedWait.DeadlineAt
		}
		record := map[string]any{"timeoutSeconds": float64(input.TimedWait.TimeoutSeconds), "deadlineAt": deadline}
		item["timedWait"] = record
		event["timedWait"] = record
	}
	return item, event
}

func ApprovalResolvedEvent(cancellation PendingGateCancellation, status string) map[string]any {
	toolName := pendingGateToolName(cancellation.ToolName)
	return map[string]any{
		"kind":       "approval_resolved",
		"threadId":   strings.TrimSpace(cancellation.ThreadID),
		"turnId":     strings.TrimSpace(cancellation.TurnID),
		"itemId":     strings.TrimSpace(cancellation.ItemID),
		"approvalId": strings.TrimSpace(cancellation.ID),
		"toolName":   toolName,
		"status":     strings.TrimSpace(status),
		"summary":    "Approve " + toolName,
	}
}

func UserInputResolvedEvent(cancellation PendingGateCancellation, status string) map[string]any {
	return map[string]any{
		"kind":     "user_input_resolved",
		"threadId": strings.TrimSpace(cancellation.ThreadID),
		"turnId":   strings.TrimSpace(cancellation.TurnID),
		"itemId":   strings.TrimSpace(cancellation.ItemID),
		"inputId":  strings.TrimSpace(cancellation.ID),
		"status":   strings.TrimSpace(status),
		"prompt":   privacyprojectionapp.ProjectOrdinaryText(pendingGatePrompt(cancellation.Prompt)),
	}
}

func UserInputResolvedOutput(status string, answers []map[string]string, cancelled bool) map[string]any {
	output := map[string]any{"status": strings.TrimSpace(status)}
	if !cancelled {
		output["answers"] = privacyprojectionapp.ProjectStringRecords(answers)
		output["answerCount"] = float64(len(answers))
	}
	return output
}

// UserInputResolvedOutputForQuestions retains only host-bound question IDs.
// Answer text remains under the existing ordinary/model privacy projection;
// public tool persistence removes the complete answers field separately.
func UserInputResolvedOutputForQuestions(status string, answers []map[string]string, cancelled bool, inputID string, questions []map[string]any) map[string]any {
	output := map[string]any{"status": strings.TrimSpace(status)}
	if !cancelled {
		output["answers"] = privacyprojectionapp.ProjectHostValidatedUserInputAnswers(inputID, questions, answers)
		output["answerCount"] = float64(len(answers))
	}
	return output
}
