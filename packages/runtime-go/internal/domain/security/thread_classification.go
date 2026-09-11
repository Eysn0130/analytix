package security

import (
	"errors"
	"strings"
)

// ClassifyCaseSensitiveThread treats malformed authority markers as
// case-sensitive and returns an error so callers that can block do so. Read
// projections that cannot return an error must still choose the restrictive
// case path whenever the boolean is true.
func ClassifyCaseSensitiveThread(thread map[string]any) (bool, error) {
	if thread == nil {
		return false, nil
	}
	threadID := strings.TrimSpace(threadString(thread, "id"))
	if value, present := thread["historyAuthority"]; present && value != nil {
		authority, ok := value.(string)
		if !ok || strings.TrimSpace(authority) == "" {
			return true, errors.New("thread history authority marker is invalid")
		}
		if strings.TrimSpace(authority) == "case_boundary_only_v1" {
			return true, nil
		}
		return true, errors.New("thread history authority marker is unknown")
	}
	for _, key := range []string{"caseId", "caseProjectId", "caseBindingHash"} {
		if value, present := thread[key]; present && value != nil {
			text, ok := value.(string)
			if !ok {
				return true, errors.New("thread case marker is invalid")
			}
			text = strings.TrimSpace(text)
			if text != "" && !(key == "caseId" && text == UnboundCaseID) {
				return true, nil
			}
		}
	}
	if value := thread["securityState"]; value != nil {
		securityContext, err := ParseTurnSecurityContext(value)
		if err != nil || (threadID != "" && securityContext.ThreadID != threadID) {
			return true, errors.New("thread security state is invalid")
		}
		if TurnSecurityContextIsCaseSensitive(securityContext) {
			return true, nil
		}
	}
	turns, ok := thread["turns"].([]any)
	if thread["turns"] != nil && !ok {
		return true, errors.New("thread turns are invalid")
	}
	caseSensitive := false
	for _, turnValue := range turns {
		turn, ok := turnValue.(map[string]any)
		if !ok {
			return true, errors.New("thread contains an invalid turn")
		}
		turnID := strings.TrimSpace(threadString(turn, "id"))
		if recordHasCaseAuthorityMarker(turn) || strings.TrimSpace(threadString(turn, "caseHistoryProjection")) != "" {
			caseSensitive = true
		}
		if value := turn["securityContext"]; value != nil {
			securityContext, err := ParseTurnSecurityContext(value)
			if err != nil || (threadID != "" && securityContext.ThreadID != threadID) ||
				(turnID != "" && securityContext.TurnID != turnID) {
				return true, errors.New("thread contains an invalid frozen security context")
			}
			if TurnSecurityContextIsCaseSensitive(securityContext) {
				caseSensitive = true
			}
		}
		items, ok := turn["items"].([]any)
		if turn["items"] != nil && !ok {
			return true, errors.New("thread turn items are invalid")
		}
		for _, itemValue := range items {
			item, ok := itemValue.(map[string]any)
			if !ok {
				return true, errors.New("thread contains an invalid item")
			}
			if recordHasCaseAuthorityMarker(item) {
				caseSensitive = true
			}
		}
	}
	return caseSensitive, nil
}

func recordHasCaseAuthorityMarker(record map[string]any) bool {
	for _, key := range []string{
		"acceptedFinal", "acceptedFinalView", "acceptedFinalDigest", "publicationCommitId",
		"publicationEventId", "publicationPayloadDigest",
	} {
		if value, present := record[key]; present && value != nil {
			return true
		}
	}
	return false
}

func threadString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
