package model

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
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
		messages = append(messages, domainmodel.Message{
			Role:    "user",
			Content: SteeringProviderContent(text, entry),
		})
	}
	return PromotedSteeringOutput{ItemCreatedEvents: events, Messages: messages}
}

func steeringStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}
