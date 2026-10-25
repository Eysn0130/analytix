package model

import (
	"encoding/json"
	"errors"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

type PromotedSteeringInput struct {
	ThreadID string
	TurnID   string
	Entries  []map[string]any
	Items    []map[string]any
}

type PromotedSteeringOutput struct {
	ItemCreatedEvents []map[string]any
	Messages          []domainmodel.Message
}

func BuildPromotedSteering(input PromotedSteeringInput) PromotedSteeringOutput {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	if threadID == "" || turnID == "" || len(input.Items) == 0 {
		return PromotedSteeringOutput{}
	}
	events := make([]map[string]any, 0, len(input.Items))
	messages := make([]domainmodel.Message, 0, len(input.Items))
	for index, item := range input.Items {
		if item == nil {
			continue
		}
		itemID := steeringStringField(item, "id")
		events = append(events, map[string]any{
			"kind":     "item_created",
			"threadId": threadID,
			"turnId":   turnID,
			"itemId":   itemID,
			"item":     contracts.CloneMap(item),
		})
		text, _ := item["text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		var entry map[string]any
		if index < len(input.Entries) {
			entry = input.Entries[index]
		}
		message := domainmodel.Message{Role: "user", Content: SteeringProviderContent(text, entry)}
		if _, present, err := domainsteering.UserInputReplyIdentityFromEntryV1(entry); present || err != nil {
			message = domainmodel.Message{Role: "user", Content: text, PrivateUserInputReplyRequired: true}
			contextDigest := steeringStringField(entry, "contextDigest")
			if err == nil && domainsteering.ValidatePromotedItemForContextV1(entry, item, threadID, turnID, contextDigest) == nil {
				message, _ = bindUserInputReplyProviderMessageV1(message, entry, threadID, contextDigest)
			}
		}
		messages = append(messages, message)
	}
	return PromotedSteeringOutput{ItemCreatedEvents: events, Messages: messages}
}

func steeringStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

// Immutable process-local proof. No JSON or external map can construct this
// concrete type. The caller must already verify installation authority and the
// exact promoted item; replay binds the verified source to the current scope.
type userInputReplyProviderBindingV1 struct {
	threadID, contextDigest, content, identityJSON string
}

func bindUserInputReplyProviderMessageV1(message domainmodel.Message, entry map[string]any, threadID, contextDigest string) (domainmodel.Message, error) {
	identity, present, err := domainsteering.UserInputReplyIdentityFromEntryV1(entry)
	if err != nil || !present || !domainsecurity.IsSHA256Hex(contextDigest) || message.Role != "user" || message.Content != steeringStringField(entry, "text") {
		return message, errors.New("late Question provider binding is invalid")
	}
	if _, err := domainsteering.ParseUserInputReplyContentV1(message.Content, identity); err != nil {
		return message, err
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return message, err
	}
	message.PrivateUserInputReplyRequired = true
	message.PrivateUserInputReplyBinding = userInputReplyProviderBindingV1{threadID: threadID, contextDigest: contextDigest, content: message.Content, identityJSON: string(encoded)}
	return message, nil
}

// UserInputReplyProviderPayloadV1 opens only a verified exact in-process message.
// It returns defensive decoded values, never the immutable carrier itself.
func UserInputReplyProviderPayloadV1(message domainmodel.Message, context domainsecurity.TurnSecurityContext) (domainsteering.UserInputReplyIdentityV1, domainsteering.UserInputReplyContentV1, error) {
	var identity domainsteering.UserInputReplyIdentityV1
	var content domainsteering.UserInputReplyContentV1
	binding, ok := message.PrivateUserInputReplyBinding.(userInputReplyProviderBindingV1)
	if !ok || !message.PrivateUserInputReplyRequired || binding.threadID != context.ThreadID || binding.contextDigest != context.ContextDigest || message.Role != "user" || message.Name != "" || message.ToolCallID != "" || len(message.Parts) != 0 || len(message.ToolCalls) != 0 || message.PrivateProviderSemanticBinding != nil || message.PrivateProviderReferenceBinding != nil || message.Content != binding.content {
		return identity, content, errors.New("late Question provider authority is unavailable")
	}
	if json.Unmarshal([]byte(binding.identityJSON), &identity) != nil {
		return identity, content, errors.New("late Question provider identity is invalid")
	}
	content, err := domainsteering.ParseUserInputReplyContentV1(message.Content, identity)
	return identity, content, err
}

// The reserved content DTO cannot silently become an ordinary message after a
// serialization boundary drops the process-local proof.
func UserInputReplyProviderContentRequiredV1(text string) bool {
	var marker struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal([]byte(text), &marker) == nil && marker.Kind == domainsteering.UserInputReplyContentKindV1
}
