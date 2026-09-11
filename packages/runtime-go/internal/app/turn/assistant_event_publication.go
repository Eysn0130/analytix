package turn

import (
	"bytes"
	"encoding/json"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var ErrAssistantPublicationUnbound = domainevent.ErrAssistantDraftPersistence

// ValidateAssistantPublicationEvent is the durable event sink check for
// ordinary assistant text. Case AcceptedFinal events use the separate atomic
// accepted-final bundle and are intentionally rejected here. A general event
// is publishable only when it is an exact readback of the item committed by
// the context-bound terminal CAS.
func ValidateAssistantPublicationEvent(thread map[string]any, event map[string]any) error {
	if !containsAssistantTextRecord(event) {
		return nil
	}
	if stringField(event, "kind") != "item_completed" {
		return ErrAssistantPublicationUnbound
	}
	item, ok := event["item"].(map[string]any)
	if !ok || stringField(item, "kind") != "assistant_text" {
		return ErrAssistantPublicationUnbound
	}
	threadID := strings.TrimSpace(stringField(thread, "id"))
	turnID := strings.TrimSpace(stringField(event, "turnId"))
	itemID := strings.TrimSpace(stringField(event, "itemId"))
	if threadID == "" || turnID == "" || itemID == "" ||
		strings.TrimSpace(stringField(event, "threadId")) != threadID ||
		strings.TrimSpace(stringField(item, "threadId")) != threadID ||
		strings.TrimSpace(stringField(item, "turnId")) != turnID || strings.TrimSpace(stringField(item, "id")) != itemID {
		return ErrAssistantPublicationUnbound
	}
	turn, found := securityTurnByID(thread, turnID)
	if !found || !IsTerminalStatus(stringField(turn, "status")) || turn["acceptedFinal"] != nil || item["acceptedFinal"] != nil {
		return ErrAssistantPublicationUnbound
	}
	canonicalItem, found := turnItemByID(turn, itemID)
	if !found || !canonicalJSONEqual(canonicalItem, item) ||
		strings.TrimSpace(stringField(event, "timestamp")) != strings.TrimSpace(stringField(canonicalItem, "finishedAt")) {
		return ErrAssistantPublicationUnbound
	}
	binding, err := domainturnterminal.ParseGeneralTerminalCASBindingV1(turn["generalTerminalCASBinding"])
	itemBinding, itemErr := domainturnterminal.ParseGeneralTerminalCASBindingV1(item["generalTerminalCASBinding"])
	turnContext, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	current, currentErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	text, _ := item["text"].(string)
	if err != nil || itemErr != nil || contextErr != nil || currentErr != nil || current != turnContext ||
		binding.BindingDigest != itemBinding.BindingDigest ||
		domainturnterminal.ValidateGeneralTerminalCASBindingForContextV1(binding, current, text, stringField(turn, "status")) != nil {
		return ErrAssistantPublicationUnbound
	}
	return nil
}

func canonicalJSONEqual(left, right map[string]any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func containsAssistantTextRecord(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if strings.TrimSpace(stringField(typed, "kind")) == "assistant_text" {
			return true
		}
		for _, child := range typed {
			if containsAssistantTextRecord(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsAssistantTextRecord(child) {
				return true
			}
		}
	}
	return false
}

func turnItemByID(turn map[string]any, itemID string) (map[string]any, bool) {
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok && strings.TrimSpace(stringField(item, "id")) == itemID {
			return item, true
		}
	}
	return nil, false
}
