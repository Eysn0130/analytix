package turn

import (
	"errors"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

// InspectTerminalAuthorityV1 identifies an existing terminal CAS winner and
// validates its durable authority before a competing terminal operation may
// be treated as an idempotent no-op.
func InspectTerminalAuthorityV1(thread map[string]any, turnID string) (status string, found bool, terminal bool, err error) {
	turn, found := securityTurnByID(thread, strings.TrimSpace(turnID))
	if !found {
		return "", false, false, nil
	}
	status = strings.TrimSpace(stringField(turn, "status"))
	if !IsTerminalStatus(status) {
		return status, true, false, nil
	}
	if turn["acceptedFinal"] != nil {
		record, parseErr := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
		expectedStatus, statusOK := domainevidence.FinalAnswerTerminalStatus(record.TerminalReason)
		if parseErr != nil || !statusOK || expectedStatus != status || validateAcceptedFinalTurnContext(turn, record) != nil ||
			strings.TrimSpace(stringField(turn, "finishedAt")) != record.AcceptedAt {
			return status, true, true, errors.New("existing accepted-final terminal authority is invalid")
		}
		assistantCount := 0
		items, _ := turn["items"].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if stringField(item, "kind") != "assistant_text" {
				continue
			}
			assistantCount++
			if validateAcceptedFinalPrivateItem(turn, item) != nil {
				return status, true, true, errors.New("existing accepted-final item authority is invalid")
			}
		}
		if assistantCount != 1 {
			return status, true, true, errors.New("existing accepted-final terminal item is missing or ambiguous")
		}
		return status, true, true, nil
	}
	commit, parseErr := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	if parseErr != nil || ValidateGeneralTerminalPublicationCommitForThreadV1(thread, turnID, commit) != nil {
		return status, true, true, errors.New("existing general terminal authority is invalid")
	}
	return status, true, true, nil
}
