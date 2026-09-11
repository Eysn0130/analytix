package model

import (
	"errors"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

type SteeringPromotionStore interface {
	PromotePendingSteeringEntriesForContext(threadID, turnID, expectedContextDigest string) ([]map[string]any, []map[string]any, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

type SteeringPrefixPromotionStore interface {
	PromotePendingSteeringEntryPrefixForContext(
		threadID, turnID, expectedContextDigest string,
		expected []domainsteering.PendingEntryExpectationV1,
	) ([]map[string]any, []map[string]any, error)
	RecordEvent(map[string]any) (map[string]any, []string, error)
}

func PromoteSteeringForProvider(store SteeringPromotionStore, threadID, turnID, expectedContextDigest string) ([]domainmodel.Message, error) {
	messages, _, _, err := PromoteSteeringForProviderWithEntries(store, threadID, turnID, expectedContextDigest)
	return messages, err
}

func PromoteSteeringForProviderWithEntries(store SteeringPromotionStore, threadID, turnID, expectedContextDigest string) ([]domainmodel.Message, []map[string]any, []map[string]any, error) {
	if store == nil {
		return nil, nil, nil, errors.New("steering promotion store is required")
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(expectedContextDigest)) {
		return nil, nil, nil, errors.New("steering promotion context digest is invalid")
	}
	entries, items, err := store.PromotePendingSteeringEntriesForContext(threadID, turnID, expectedContextDigest)
	return projectPromotedSteeringForProvider(store, threadID, turnID, entries, items, err)
}

func PromoteSteeringPrefixForProviderWithEntries(
	store SteeringPrefixPromotionStore,
	threadID, turnID, expectedContextDigest string,
	expected []domainsteering.PendingEntryExpectationV1,
) ([]domainmodel.Message, []map[string]any, []map[string]any, error) {
	if store == nil {
		return nil, nil, nil, errors.New("steering promotion store is required")
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(expectedContextDigest)) {
		return nil, nil, nil, errors.New("steering promotion context digest is invalid")
	}
	if len(expected) == 0 {
		return nil, nil, nil, errors.New("steering promotion prefix is required")
	}
	for _, candidate := range expected {
		if strings.TrimSpace(candidate.ID) == "" || !domainsecurity.IsSHA256Hex(candidate.ContentDigest) {
			return nil, nil, nil, errors.New("steering promotion prefix is invalid")
		}
	}
	entries, items, err := store.PromotePendingSteeringEntryPrefixForContext(
		threadID, turnID, expectedContextDigest, expected,
	)
	return projectPromotedSteeringForProvider(store, threadID, turnID, entries, items, err)
}

func projectPromotedSteeringForProvider(
	store interface {
		RecordEvent(map[string]any) (map[string]any, []string, error)
	},
	threadID, turnID string,
	entries, items []map[string]any,
	err error,
) ([]domainmodel.Message, []map[string]any, []map[string]any, error) {
	if err != nil || len(items) == 0 {
		return nil, entries, items, err
	}
	promoted := BuildPromotedSteering(PromotedSteeringInput{
		ThreadID: threadID,
		TurnID:   turnID,
		Entries:  entries,
		Items:    items,
	})
	for _, event := range promoted.ItemCreatedEvents {
		if _, _, err := store.RecordEvent(event); err != nil {
			return nil, entries, items, err
		}
	}
	return promoted.Messages, entries, items, nil
}
