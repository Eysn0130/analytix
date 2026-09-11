package turn

import "strings"

type PendingGateCancellation struct {
	Kind     string
	ID       string
	ThreadID string
	TurnID   string
	ItemID   string
	ToolName string
	Prompt   string
}

func PendingGateCancellationEvents(cancellations []PendingGateCancellation, reason string) []map[string]any {
	events := make([]map[string]any, 0, len(cancellations))
	reason = strings.TrimSpace(reason)
	for _, cancellation := range cancellations {
		switch strings.TrimSpace(cancellation.Kind) {
		case "approval":
			events = append(events, map[string]any{
				"kind":        "approval_resolved",
				"threadId":    strings.TrimSpace(cancellation.ThreadID),
				"turnId":      strings.TrimSpace(cancellation.TurnID),
				"itemId":      strings.TrimSpace(cancellation.ItemID),
				"approvalId":  strings.TrimSpace(cancellation.ID),
				"toolName":    pendingGateToolName(cancellation.ToolName),
				"status":      "expired",
				"cancelledBy": reason,
				"summary":     "Approve " + pendingGateToolName(cancellation.ToolName),
			})
		case "user_input":
			events = append(events, map[string]any{
				"kind":        "user_input_resolved",
				"threadId":    strings.TrimSpace(cancellation.ThreadID),
				"turnId":      strings.TrimSpace(cancellation.TurnID),
				"itemId":      strings.TrimSpace(cancellation.ItemID),
				"inputId":     strings.TrimSpace(cancellation.ID),
				"status":      "cancelled",
				"prompt":      pendingGatePrompt(cancellation.Prompt),
				"cancelledBy": reason,
			})
		}
	}
	return events
}

func pendingGateToolName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "tool"
	}
	return value
}

func pendingGatePrompt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "User input required"
	}
	return value
}
