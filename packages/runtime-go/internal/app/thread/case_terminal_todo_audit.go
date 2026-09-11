package thread

import (
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintodo "analytix.local/runtime-go/internal/domain/todo"
)

const (
	caseFailedTodoAuditContentV1   = "Case Todo content withheld; failed state retained for audit."
	caseCanceledTodoAuditContentV1 = "Case Todo content withheld; canceled state retained for audit."
)

var caseTerminalTodoAuditIDDomainV1 = []byte(
	"analytix.case-terminal-todo-audit/id/v1\x00",
)

// CaseTerminalTodoAuditProjectionV1 retains only the host-enumerated terminal
// lifecycle state required for Todo audit. Case task text, notes, evidence
// identifiers, source metadata, and caller-controlled ids never cross the
// boundary-only public projection.
func CaseTerminalTodoAuditProjectionV1(
	todos map[string]any,
	threadID string,
	projectedAt string,
) (map[string]any, error) {
	threadID = strings.TrimSpace(threadID)
	projectedAt = strings.TrimSpace(projectedAt)
	if todos == nil {
		return nil, nil
	}
	if threadID == "" {
		return nil, errors.New("case terminal Todo audit thread identity is required")
	}
	items := listAny(todos["items"])
	if len(items) > MaxTodoItems {
		return nil, errors.New("case terminal Todo audit exceeds the Todo limit")
	}

	projected := make([]any, 0, len(items))
	seenSourceIDs := map[string]bool{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || item == nil {
			continue
		}
		rawStatus, statusOK := item["status"].(string)
		if !statusOK || rawStatus != strings.TrimSpace(rawStatus) {
			continue
		}
		status, statusErr := domaintodo.ParseStatus(rawStatus)
		if statusErr != nil || !domaintodo.IsRetainedTerminal(status) {
			continue
		}
		if err := validateAllowedTodoKeys(item); err != nil {
			return nil, errors.Join(errors.New("case terminal Todo audit item is invalid"), err)
		}
		sourceID, idOK := item["id"].(string)
		sourceID = strings.TrimSpace(sourceID)
		content, contentOK := item["content"].(string)
		if !idOK || sourceID == "" || !contentOK || strings.TrimSpace(content) == "" ||
			seenSourceIDs[sourceID] {
			return nil, errors.New("case terminal Todo audit identity is invalid")
		}
		seenSourceIDs[sourceID] = true
		reason, reasonExists, reasonErr := todoStatusReason(item)
		if reasonErr != nil || !reasonExists || domaintodo.ValidateStatusReason(status, reason) != nil {
			return nil, errors.New("case terminal Todo audit reason is invalid")
		}
		at := projectedAt
		if at == "" {
			at = strings.TrimSpace(stringField(item, "updatedAt"))
		}
		if at == "" {
			at = strings.TrimSpace(stringField(item, "createdAt"))
		}
		if at == "" {
			return nil, errors.New("case terminal Todo audit time is unavailable")
		}
		safeContent := caseFailedTodoAuditContentV1
		if status == domaintodo.StatusCanceled {
			safeContent = caseCanceledTodoAuditContentV1
		}
		projected = append(projected, map[string]any{
			"id":               caseTerminalTodoAuditIDV1(threadID, sourceID),
			"content":          safeContent,
			"status":           string(status),
			"statusReasonCode": reason,
			"createdAt":        at,
			"updatedAt":        at,
		})
	}
	if len(projected) == 0 {
		return nil, nil
	}
	if projectedAt == "" {
		projectedAt = stringField(projected[0].(map[string]any), "updatedAt")
	}
	return map[string]any{
		"threadId":  threadID,
		"items":     projected,
		"updatedAt": projectedAt,
	}, nil
}

func caseTerminalTodoAuditIDV1(threadID string, sourceID string) string {
	body := append([]byte(nil), caseTerminalTodoAuditIDDomainV1...)
	body = append(body, threadID...)
	body = append(body, 0)
	body = append(body, sourceID...)
	return "todo_case_audit_" + domainsecurity.SHA256Hex(body)
}
