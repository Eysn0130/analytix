package steeringauthority

import (
	"context"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

type PublicReader interface {
	AllThreadIDs() ([]string, error)
	GetThread(string) (map[string]any, error)
}

func HasPublicAuthorityStateV1(reader PublicReader) (bool, error) {
	found := false
	err := visitPublicSteeringV1(reader, func(record publicSteeringRecordV1) error {
		found = true
		return nil
	})
	return found, err
}

func VerifyTrustedInventoryV1(ctx context.Context, reader PublicReader, service *Service) error {
	if ctx == nil || service == nil {
		return errors.New("steering authority inventory verifier is unavailable")
	}
	seenCommits := map[string]struct{}{}
	return visitPublicSteeringV1(reader, func(record publicSteeringRecordV1) error {
		entry := record.Entry
		switch status, _ := entry["status"].(string); status {
		case "pending", "promoted":
			if service.Verify(ctx, entry, record.ContextDigest) != nil {
				return errors.New("steering authority inventory is not trusted by this installation")
			}
			if status == "promoted" {
				commit, err := domainsteering.NewPromotionCommitV1(
					record.ThreadID, record.TurnID, record.ContextDigest, entry, record.Item,
					func(candidate map[string]any, digest string) error { return service.Verify(ctx, candidate, digest) },
				)
				if err != nil {
					return errors.New("steering promotion commit inventory is invalid")
				}
				if _, duplicate := seenCommits[commit.CommitID]; duplicate {
					return errors.New("steering promotion commit inventory contains a duplicate")
				}
				seenCommits[commit.CommitID] = struct{}{}
			}
		case "cancelled":
			pending, err := cancelledAdmissionProjectionV1(entry)
			if err != nil || service.Verify(ctx, pending, record.ContextDigest) != nil {
				return errors.New("cancelled steering authority inventory is invalid")
			}
		default:
			return errors.New("steering authority inventory status is invalid")
		}
		return nil
	})
}

type publicSteeringRecordV1 struct {
	ThreadID      string
	TurnID        string
	ContextDigest string
	Entry         map[string]any
	Item          map[string]any
}

func visitPublicSteeringV1(reader PublicReader, visit func(publicSteeringRecordV1) error) error {
	if reader == nil || visit == nil {
		return errors.New("steering authority public reader is unavailable")
	}
	threadIDs, err := reader.AllThreadIDs()
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(threadIDs))
	seenEntryIDs := map[string]struct{}{}
	seenClientIDs := map[string]struct{}{}
	seenItemIDs := map[string]struct{}{}
	for _, threadID := range threadIDs {
		threadID = strings.TrimSpace(threadID)
		if threadID == "" {
			return errors.New("steering authority inventory thread id is invalid")
		}
		if _, duplicate := seen[threadID]; duplicate {
			return errors.New("steering authority inventory contains a duplicate thread")
		}
		seen[threadID] = struct{}{}
		thread, err := reader.GetThread(threadID)
		if err != nil || thread == nil {
			return errors.Join(err, errors.New("steering authority inventory cannot read canonical thread"))
		}
		if persistedID, present := thread["id"]; present && persistedID != threadID {
			return errors.New("steering authority inventory thread identity is invalid")
		}
		turns, ok := thread["turns"].([]any)
		if !ok {
			return errors.New("steering authority inventory turns are invalid")
		}
		for _, rawTurn := range turns {
			turn, ok := rawTurn.(map[string]any)
			if !ok {
				return errors.New("steering authority inventory turn is invalid")
			}
			rawSteering, present := turn["steering"]
			if !present {
				continue
			}
			entries, ok := rawSteering.([]any)
			if !ok {
				return errors.New("steering authority inventory entries are invalid")
			}
			hasAuthorityInTurn := false
			for _, rawEntry := range entries {
				entry, ok := rawEntry.(map[string]any)
				if !ok || entry == nil {
					return errors.New("steering authority inventory entry is invalid")
				}
				hasAuthority, err := hasCompleteAuthorityMaterialV1(entry)
				if err != nil {
					return err
				}
				hasAuthorityInTurn = hasAuthorityInTurn || hasAuthority
			}
			if !hasAuthorityInTurn {
				continue
			}
			itemByID := map[string]map[string]any{}
			if rawItems, present := turn["items"]; present {
				items, ok := rawItems.([]any)
				if !ok {
					return errors.New("steering authority inventory items are invalid")
				}
				for _, rawItem := range items {
					item, ok := rawItem.(map[string]any)
					itemID, idOK := item["id"].(string)
					if !ok || !idOK || strings.TrimSpace(itemID) == "" || strings.TrimSpace(itemID) != itemID {
						return errors.New("steering authority inventory item is invalid")
					}
					if _, duplicate := seenItemIDs[itemID]; duplicate {
						return errors.New("steering authority inventory contains a duplicate item")
					}
					seenItemIDs[itemID] = struct{}{}
					itemByID[itemID] = item
				}
			}
			for _, rawEntry := range entries {
				entry, ok := rawEntry.(map[string]any)
				if !ok || entry == nil {
					return errors.New("steering authority inventory entry is invalid")
				}
				hasAuthority, err := hasCompleteAuthorityMaterialV1(entry)
				if err != nil {
					return err
				}
				if !hasAuthority {
					continue
				}
				turnID, turnIDOK := turn["id"].(string)
				contextDigest, digestOK := entry["contextDigest"].(string)
				frozen, contextErr := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
				if !turnIDOK || strings.TrimSpace(turnID) == "" || strings.TrimSpace(turnID) != turnID ||
					!digestOK || strings.TrimSpace(contextDigest) != contextDigest ||
					contextErr != nil || domainsecurity.ValidateTurnSecurityContextForExecution(frozen) != nil ||
					frozen.ThreadID != threadID || frozen.TurnID != turnID || frozen.ContextDigest != contextDigest {
					return errors.New("steering authority inventory context is invalid")
				}
				entryID, entryIDOK := entry["id"].(string)
				clientID, clientIDOK := entry["clientUserMessageId"].(string)
				if !entryIDOK || !clientIDOK || entryID != domainsteering.EntryIDV1(turnID, clientID) {
					return errors.New("steering authority inventory identity is invalid")
				}
				if _, duplicate := seenEntryIDs[entryID]; duplicate {
					return errors.New("steering authority inventory contains a duplicate entry")
				}
				if _, duplicate := seenClientIDs[clientID]; duplicate {
					return errors.New("steering authority inventory contains a duplicate client identity")
				}
				seenEntryIDs[entryID] = struct{}{}
				seenClientIDs[clientID] = struct{}{}
				status, _ := entry["status"].(string)
				item := itemByID[entryID]
				switch status {
				case "pending", "cancelled":
					if item != nil {
						return errors.New("unpromoted steering authority owns a promoted item")
					}
				case "promoted":
					if item == nil || domainsteering.ValidatePromotedItemForContextV1(entry, item, threadID, turnID, contextDigest) != nil {
						return errors.New("steering promotion inventory item is invalid")
					}
				default:
					return errors.New("steering authority inventory status is invalid")
				}
				if err := visit(publicSteeringRecordV1{
					ThreadID: threadID, TurnID: turnID, ContextDigest: contextDigest, Entry: entry, Item: item,
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func hasCompleteAuthorityMaterialV1(entry map[string]any) (bool, error) {
	admission, err := completeStringTripletV1(entry, "authorityKeyId", "authorityPublicKey", "authoritySignature")
	if err != nil {
		return false, err
	}
	promotion, err := completeStringTripletV1(entry, "promotionAuthorityKeyId", "promotionAuthorityPublicKey", "promotionAuthoritySignature")
	if err != nil || promotion && !admission {
		return false, errors.Join(err, errors.New("steering promotion authority is detached from admission"))
	}
	return admission || promotion, nil
}

func completeStringTripletV1(entry map[string]any, keys ...string) (bool, error) {
	present := 0
	for _, key := range keys {
		value, found := entry[key]
		if !found {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" || strings.TrimSpace(text) != text {
			return false, errors.New("steering authority inventory material is invalid")
		}
		present++
	}
	if present != 0 && present != len(keys) {
		return false, errors.New("steering authority inventory material is incomplete")
	}
	return present == len(keys), nil
}

func cancelledAdmissionProjectionV1(entry map[string]any) (map[string]any, error) {
	cancelledAt, cancelledAtOK := entry["cancelledAt"].(string)
	reason, reasonOK := entry["cancelReason"].(string)
	parsed, timeErr := time.Parse(time.RFC3339Nano, cancelledAt)
	if !cancelledAtOK || !reasonOK || strings.TrimSpace(reason) == "" || strings.TrimSpace(reason) != reason ||
		timeErr != nil || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != cancelledAt {
		return nil, errors.New("cancelled steering authority metadata is invalid")
	}
	pending := make(map[string]any, len(entry)-2)
	for key, value := range entry {
		switch key {
		case "cancelledAt", "cancelReason":
		default:
			pending[key] = value
		}
	}
	pending["status"] = "pending"
	return pending, nil
}
