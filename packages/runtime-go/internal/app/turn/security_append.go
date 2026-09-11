package turn

import (
	"errors"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrFrozenSecurityContextMissing = errors.New("turn security context is missing")

type FrozenSecurityContextThreadReader interface {
	GetThread(string) (map[string]any, error)
}

// ValidateSecurityBoundAppend is the store-side, atomic last check for
// provider-originated tool records. It runs while the durable thread lock is
// held so a terminal turn or newer context cannot accept a late result.
func ValidateSecurityBoundAppend(thread map[string]any, turnID string, item map[string]any) error {
	turn, ok := securityTurnByID(thread, strings.TrimSpace(turnID))
	if !ok {
		return errors.New("tool record turn does not exist")
	}
	status, _ := turn["status"].(string)
	status = strings.TrimSpace(status)
	turnContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if turn["securityContext"] != nil && contextErr != nil {
		return errors.New("turn has an invalid frozen security context")
	}
	caseBound := contextErr == nil && domainsecurity.TurnSecurityContextIsCaseSensitive(turnContext)
	authorityPresent := securityBoundAuthorityPresent(item)
	digest, _ := item["contextDigest"].(string)
	digest = strings.TrimSpace(digest)
	if stringField(item, "kind") == "assistant_text" {
		return errors.New("assistant text requires atomic terminal persistence")
	}
	if IsTerminalStatus(status) && (caseBound || authorityPresent) {
		return errors.New("turn is terminal and cannot accept late security-bound items")
	}
	if !authorityPresent {
		return nil
	}
	grantID, _ := item["executionGrantId"].(string)
	epoch, epochOK := securityBoundEpoch(item["contextEpoch"])
	if digest == "" || strings.TrimSpace(grantID) == "" || !epochOK ||
		stringField(item, "threadId") != strings.TrimSpace(currentThreadID(thread)) || stringField(item, "turnId") != strings.TrimSpace(turnID) {
		return errors.New("security-bound tool record has an incomplete authority binding")
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(current) != nil || current.ContextDigest != digest || current.ContextEpoch != epoch {
		return errors.New("security-bound tool record does not match the current thread context")
	}
	if status != "running" && status != "waiting" {
		return errors.New("security-bound tool record turn is no longer active")
	}
	if contextErr != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(turnContext) != nil ||
		turnContext.ContextDigest != digest || turnContext.ContextEpoch != current.ContextEpoch {
		return errors.New("security-bound tool record does not match the active turn context")
	}
	if err := executiongrantapp.ValidateRegistryAppend(current.ThreadID, thread, turnID, item); err != nil {
		return err
	}
	return nil
}

func securityBoundAuthorityPresent(item map[string]any) bool {
	for _, key := range []string{
		"contextDigest", "contextEpoch", "executionGrantId", "executionGrant", "parentGrantId", "hostEvidenceSettlement",
		"approvalTransition", "approvalTransitionId", "continuationDispositionId",
	} {
		if _, ok := item[key]; ok {
			return true
		}
	}
	return false
}

func securityBoundEpoch(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, typed > 0
	case uint:
		return uint64(typed), typed > 0
	case int:
		return uint64(typed), typed > 0
	case int64:
		return uint64(typed), typed > 0
	case float64:
		if typed > 0 && typed <= 9007199254740991 && typed == float64(uint64(typed)) {
			return uint64(typed), true
		}
	}
	return 0, false
}

func currentThreadID(thread map[string]any) string {
	value, _ := thread["id"].(string)
	return value
}

func securityTurnByID(thread map[string]any, turnID string) (map[string]any, bool) {
	turns, _ := thread["turns"].([]any)
	for _, raw := range turns {
		turn, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := turn["id"].(string)
		if strings.TrimSpace(id) == turnID {
			return turn, true
		}
	}
	return nil, false
}

func FrozenSecurityContextForTurn(thread map[string]any, turnID string) (domainsecurity.TurnSecurityContext, error) {
	turn, ok := securityTurnByID(thread, strings.TrimSpace(turnID))
	if !ok {
		return domainsecurity.TurnSecurityContext{}, errors.New("turn security context does not exist")
	}
	if turn["securityContext"] == nil {
		return domainsecurity.TurnSecurityContext{}, ErrFrozenSecurityContextMissing
	}
	return domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
}

func LoadFrozenSecurityContext(reader FrozenSecurityContextThreadReader, threadID, turnID string) (domainsecurity.TurnSecurityContext, error) {
	thread, err := reader.GetThread(strings.TrimSpace(threadID))
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	return FrozenSecurityContextForTurn(thread, turnID)
}
