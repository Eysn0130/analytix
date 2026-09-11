package thread

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type AppendTurnInput struct {
	Thread      map[string]any
	Turn        map[string]any
	ProviderID  string
	ThreadPatch map[string]any
	UpdatedAt   string
}

func AppendTurn(input AppendTurnInput) map[string]any {
	thread := contracts.CloneMap(input.Thread)
	turn := contracts.CloneMap(input.Turn)
	turns, _ := thread["turns"].([]any)
	if len(turns) == 0 && ShouldAutoTitleThread(thread) {
		if title := TitleFromTurn(turn); title != "" {
			thread["title"] = title
			thread["autoTitle"] = false
		}
	}
	thread["turns"] = append(turns, turn)
	thread["status"] = "running"
	if providerID := strings.TrimSpace(input.ProviderID); providerID != "" {
		thread["providerId"] = providerID
	}
	for _, key := range []string{"executionPolicyVersion", "approvalPolicy", "sandboxMode", "model", "reasoningEffort", "endpointFormat", "securityState", "contextEpochState"} {
		if value, ok := input.ThreadPatch[key]; ok {
			thread[key] = contracts.CloneValue(value)
		}
	}
	thread["updatedAt"] = strings.TrimSpace(input.UpdatedAt)
	return thread
}
