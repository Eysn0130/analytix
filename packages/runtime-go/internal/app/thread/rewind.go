package thread

import (
	"fmt"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type RewindInput struct {
	Thread                          map[string]any
	ThreadID                        string
	TurnID                          string
	Now                             string
	AllowCurrentSecurityReplacement bool
}

type RewindPlan struct {
	Thread   map[string]any
	Response map[string]any
}

// ValidateRewindAcceptedFinalProtection performs an authority-free rejection
// check before a history mutation transition starts. It grants no permission:
// BuildRewind repeats the same check against the post-transition baseline.
func ValidateRewindAcceptedFinalProtection(thread map[string]any, turnID string) error {
	turnID = strings.TrimSpace(turnID)
	turns, _ := thread["turns"].([]any)
	targetIndex := -1
	for index, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") == turnID {
			targetIndex = index
			break
		}
	}
	if targetIndex < 0 {
		return fmt.Errorf("%w: %s", ErrTurnNotFound, turnID)
	}
	for _, value := range turns[targetIndex:] {
		turn, _ := value.(map[string]any)
		if turnContainsAcceptedFinal(turn) {
			return ErrAcceptedFinalRewind
		}
	}
	return nil
}

func BuildRewind(input RewindInput) (RewindPlan, error) {
	if input.Thread == nil {
		return RewindPlan{}, ErrThreadNotFound
	}
	thread := contracts.CloneMap(input.Thread)
	if strings.EqualFold(stringField(thread, "status"), "running") {
		return RewindPlan{}, ErrThreadRunning
	}
	turnID := strings.TrimSpace(input.TurnID)
	turns, _ := thread["turns"].([]any)
	targetIndex := -1
	for index, value := range turns {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") == turnID {
			targetIndex = index
			break
		}
	}
	if targetIndex < 0 {
		return RewindPlan{}, fmt.Errorf("%w: %s", ErrTurnNotFound, turnID)
	}
	removedTurns := turns[targetIndex:]
	currentTurnID := ""
	if input.Thread["securityState"] != nil {
		current, err := domainsecurity.ParseTurnSecurityContext(input.Thread["securityState"])
		if err != nil || current.ThreadID != strings.TrimSpace(input.ThreadID) {
			return RewindPlan{}, ErrCurrentSecurityContextRewind
		}
		currentTurnID = current.TurnID
	}
	for _, value := range removedTurns {
		turn, _ := value.(map[string]any)
		if currentTurnID != "" && stringField(turn, "id") == currentTurnID && !input.AllowCurrentSecurityReplacement {
			return RewindPlan{}, ErrCurrentSecurityContextRewind
		}
		if turnContainsAcceptedFinal(turn) {
			return RewindPlan{}, ErrAcceptedFinalRewind
		}
	}
	removedTurnIDs := make([]any, 0, len(removedTurns))
	for _, value := range removedTurns {
		turn, _ := value.(map[string]any)
		if id := stringField(turn, "id"); id != "" {
			removedTurnIDs = append(removedTurnIDs, id)
		}
	}
	remainingTurns := make([]any, targetIndex)
	copy(remainingTurns, turns[:targetIndex])
	now := strings.TrimSpace(input.Now)
	thread["turns"] = remainingTurns
	thread["status"] = "idle"
	thread["updatedAt"] = now
	return RewindPlan{
		Thread: thread,
		Response: map[string]any{
			"threadId":       strings.TrimSpace(input.ThreadID),
			"turnId":         turnID,
			"removedTurns":   float64(len(removedTurns)),
			"remainingTurns": float64(len(remainingTurns)),
			"removedTurnIds": removedTurnIDs,
		},
	}, nil
}

func turnContainsAcceptedFinal(turn map[string]any) bool {
	if turn == nil {
		return false
	}
	for _, key := range []string{"acceptedFinal", "acceptedFinalDigest", "publicationCommitId"} {
		if value, exists := turn[key]; exists && value != nil {
			return true
		}
	}
	items, _ := turn["items"].([]any)
	for _, value := range items {
		item, _ := value.(map[string]any)
		for _, key := range []string{"acceptedFinal", "acceptedFinalDigest", "publicationCommitId"} {
			if authority, exists := item[key]; exists && authority != nil {
				return true
			}
		}
	}
	return false
}
