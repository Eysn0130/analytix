package steering

import (
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var ErrProjectionInvalid = errors.New("steering projection does not match active turn authority")
var ErrIdentityConflict = errors.New("steering identity conflicts with durable turn state")
var ErrReplayMismatch = errors.New("steering replay does not match admitted content")
var ErrTurnNotFound = errors.New("turn not found")
var ErrTurnInactive = errors.New("turn is not running")
var ErrTurnNotLatest = errors.New("turn is not the active latest turn")
var ErrContextMismatch = errors.New("expected context digest does not match active turn")
var ErrPendingPrefixMismatch = errors.New("expected steering prefix does not match current pending entries")

type AuthorityVerifierV1 func(map[string]any, string) error
type AuthorityPromoterV1 func(map[string]any, string) (map[string]any, error)

// PendingEntryExpectationV1 is the immutable CAS input for one pending entry.
// The pair is sufficient because the host-signed content digest binds every
// entry field, including the optional logical-effect metadata.
type PendingEntryExpectationV1 struct {
	ID            string
	ContentDigest string
}

func NewPendingEntryExpectationV1(entry map[string]any) (PendingEntryExpectationV1, error) {
	expected := PendingEntryExpectationV1{
		ID:            stringField(entry, "id"),
		ContentDigest: stringField(entry, "contentDigest"),
	}
	if stringField(entry, "status") != "pending" || expected.ID == "" ||
		!domainsecurity.IsSHA256Hex(expected.ContentDigest) {
		return PendingEntryExpectationV1{}, ErrProjectionInvalid
	}
	return expected, nil
}

func CurrentExecutableTurnV1(
	thread map[string]any,
	threadID, turnID, expectedContextDigest string,
) ([]any, map[string]any, error) {
	if !domainsecurity.IsSHA256Hex(expectedContextDigest) {
		return nil, nil, ErrContextMismatch
	}
	turns := steeringListV1(thread["turns"])
	if len(turns) == 0 {
		return nil, nil, ErrTurnNotFound
	}
	turn, _ := turns[len(turns)-1].(map[string]any)
	if stringField(turn, "id") != turnID {
		return nil, nil, ErrTurnNotLatest
	}
	if stringField(turn, "status") != "running" {
		return nil, nil, ErrTurnInactive
	}
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil || domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(frozen) != nil ||
		frozen.ThreadID != threadID || frozen.TurnID != turnID || frozen.ContextDigest != expectedContextDigest {
		return nil, nil, ErrContextMismatch
	}
	return turns, turn, nil
}

func AdmitEntryToTurnV1(
	threadID, turnID string,
	turn map[string]any,
	entry map[string]any,
	contextDigest string,
	verify AuthorityVerifierV1,
) (map[string]any, map[string]any, bool, error) {
	if _, err := PendingEntriesFromTurnV1(threadID, turnID, turn, contextDigest, verify); err != nil {
		return nil, nil, false, err
	}
	if ValidatePendingEntryForContextV1(entry, contextDigest) != nil || verify == nil || verify(entry, contextDigest) != nil {
		return nil, nil, false, ErrProjectionInvalid
	}
	entryID := stringField(entry, "id")
	clientID := stringField(entry, "clientUserMessageId")
	if entryID == "" || clientID == "" || entryID != EntryIDV1(turnID, clientID) {
		return nil, nil, false, ErrProjectionInvalid
	}
	for _, raw := range steeringListV1(turn["steering"]) {
		existing, _ := raw.(map[string]any)
		if existing == nil {
			return nil, nil, false, ErrProjectionInvalid
		}
		if stringField(existing, "clientUserMessageId") == clientID {
			if verify(existing, contextDigest) != nil || !SamePendingEntryV1(existing, entry) {
				return nil, nil, false, ErrReplayMismatch
			}
			return cloneSteeringMapV1(turn), cloneSteeringMapV1(existing), false, nil
		}
		if stringField(existing, "id") == entryID {
			return nil, nil, false, ErrIdentityConflict
		}
	}
	for _, raw := range steeringListV1(turn["items"]) {
		item, _ := raw.(map[string]any)
		itemID := stringField(item, "id")
		if itemID != "" && (itemID == entryID || itemID == clientID) {
			return nil, nil, false, ErrIdentityConflict
		}
	}
	updated := cloneSteeringMapV1(turn)
	steering := append([]any(nil), steeringListV1(updated["steering"])...)
	steering = append(steering, cloneSteeringMapV1(entry))
	updated["steering"] = steering
	return updated, cloneSteeringMapV1(entry), true, nil
}

func PromoteTurnEntriesV1(
	threadID, turnID string,
	turn map[string]any,
	contextDigest, promotedAt string,
	verify AuthorityVerifierV1,
	promote AuthorityPromoterV1,
) (map[string]any, []map[string]any, []map[string]any, error) {
	pending, err := PendingEntriesFromTurnV1(threadID, turnID, turn, contextDigest, verify)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(pending) == 0 {
		return cloneSteeringMapV1(turn), nil, nil, nil
	}
	expected := make([]PendingEntryExpectationV1, 0, len(pending))
	for _, entry := range pending {
		entryExpectation, expectationErr := NewPendingEntryExpectationV1(entry)
		if expectationErr != nil {
			return nil, nil, nil, expectationErr
		}
		expected = append(expected, entryExpectation)
	}
	return PromoteTurnEntryPrefixV1(
		threadID, turnID, turn, expected, contextDigest, promotedAt, verify, promote,
	)
}

// PromoteTurnEntryPrefixV1 performs one exact compare-and-swap over the
// pending steering queue. It validates the complete durable steering/item set
// before comparing expected, then promotes only that exact non-empty prefix.
// A stale, reordered, skipped, or tail-consuming expectation fails without
// returning a mutated turn.
func PromoteTurnEntryPrefixV1(
	threadID, turnID string,
	turn map[string]any,
	expected []PendingEntryExpectationV1,
	contextDigest, promotedAt string,
	verify AuthorityVerifierV1,
	promote AuthorityPromoterV1,
) (map[string]any, []map[string]any, []map[string]any, error) {
	pending, err := PendingEntriesFromTurnV1(threadID, turnID, turn, contextDigest, verify)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(expected) == 0 {
		if len(pending) == 0 {
			return cloneSteeringMapV1(turn), nil, nil, nil
		}
		return nil, nil, nil, ErrPendingPrefixMismatch
	}
	if len(expected) > len(pending) {
		return nil, nil, nil, ErrPendingPrefixMismatch
	}
	if promote == nil {
		return nil, nil, nil, ErrProjectionInvalid
	}
	seen := make(map[string]struct{}, len(expected))
	for index, candidate := range expected {
		if candidate.ID == "" || !domainsecurity.IsSHA256Hex(candidate.ContentDigest) {
			return nil, nil, nil, ErrPendingPrefixMismatch
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return nil, nil, nil, ErrPendingPrefixMismatch
		}
		seen[candidate.ID] = struct{}{}
		if candidate.ID != stringField(pending[index], "id") ||
			candidate.ContentDigest != stringField(pending[index], "contentDigest") {
			return nil, nil, nil, ErrPendingPrefixMismatch
		}
	}
	updated := cloneSteeringMapV1(turn)
	steering := append([]any(nil), steeringListV1(updated["steering"])...)
	items := append([]any(nil), steeringListV1(updated["items"])...)
	promotedEntries := make([]map[string]any, 0, len(expected))
	promotedItems := make([]map[string]any, 0, len(expected))
	promotedCount := 0
	for index, raw := range steering {
		entry, _ := raw.(map[string]any)
		if stringField(entry, "status") != "pending" {
			continue
		}
		if promotedCount == len(expected) {
			break
		}
		itemID := stringField(entry, "id")
		text := stringField(entry, "text")
		item := map[string]any{
			"id": itemID, "turnId": turnID, "threadId": threadID, "role": "user", "status": "completed",
			"createdAt": stringField(entry, "admittedAt"), "finishedAt": promotedAt,
			"kind": "user_message", "text": text, "delivery": "steer",
			"contextDigest": contextDigest, "steeringProjectionVersion": ProjectionVersionV1,
			"steeringContentDigest": stringField(entry, "contentDigest"),
			"clientUserMessageId":   stringField(entry, "clientUserMessageId"),
		}
		if displayText := stringField(entry, "displayText"); displayText != "" && displayText != text {
			item["displayText"] = displayText
		}
		if jobID := stringField(entry, "jobId"); jobID != "" {
			item["steeringOrigin"] = "task_job"
			item["jobId"] = jobID
			item["childRunId"] = stringField(entry, "childRunId")
			item["steerMessageId"] = stringField(entry, "steerMessageId")
		} else {
			item["steeringOrigin"] = "ordinary"
		}
		promoted := cloneSteeringMapV1(entry)
		promoted["status"] = "promoted"
		promoted["promotedAt"] = promotedAt
		promoted["promotedItemId"] = itemID
		promoted, err = promote(promoted, contextDigest)
		if err != nil || ValidatePromotedItemForContextV1(promoted, item, threadID, turnID, contextDigest) != nil || verify(promoted, contextDigest) != nil {
			return nil, nil, nil, ErrProjectionInvalid
		}
		steering[index] = promoted
		items = append(items, cloneSteeringMapV1(item))
		promotedEntries = append(promotedEntries, promoted)
		promotedItems = append(promotedItems, item)
		promotedCount++
	}
	updated["steering"] = steering
	updated["items"] = items
	return updated, cloneSteeringMapsV1(promotedEntries), cloneSteeringMapsV1(promotedItems), nil
}

func PendingEntriesFromTurnV1(
	threadID, turnID string,
	turn map[string]any,
	contextDigest string,
	verify AuthorityVerifierV1,
) ([]map[string]any, error) {
	if verify == nil {
		return nil, ErrProjectionInvalid
	}
	itemByID := make(map[string]map[string]any, len(steeringListV1(turn["items"])))
	for _, raw := range steeringListV1(turn["items"]) {
		item, _ := raw.(map[string]any)
		itemID := stringField(item, "id")
		if item == nil || itemID == "" {
			return nil, ErrProjectionInvalid
		}
		if _, duplicate := itemByID[itemID]; duplicate {
			return nil, ErrIdentityConflict
		}
		itemByID[itemID] = item
	}
	steering := steeringListV1(turn["steering"])
	steeringIDs := make(map[string]struct{}, len(steering))
	clientIDs := make(map[string]struct{}, len(steering))
	pending := make([]map[string]any, 0, len(steering))
	for _, raw := range steering {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			return nil, ErrProjectionInvalid
		}
		entryID := stringField(entry, "id")
		clientID := stringField(entry, "clientUserMessageId")
		if entryID == "" || clientID == "" || entryID != EntryIDV1(turnID, clientID) {
			return nil, ErrProjectionInvalid
		}
		if _, duplicate := steeringIDs[entryID]; duplicate {
			return nil, ErrIdentityConflict
		}
		if _, duplicate := clientIDs[clientID]; duplicate {
			return nil, ErrIdentityConflict
		}
		steeringIDs[entryID] = struct{}{}
		clientIDs[clientID] = struct{}{}
		switch stringField(entry, "status") {
		case "pending":
			if ValidatePendingEntryForContextV1(entry, contextDigest) != nil || verify(entry, contextDigest) != nil {
				return nil, ErrProjectionInvalid
			}
			if _, collision := itemByID[entryID]; collision {
				return nil, ErrIdentityConflict
			}
			if _, collision := itemByID[clientID]; collision {
				return nil, ErrIdentityConflict
			}
			pending = append(pending, cloneSteeringMapV1(entry))
		case "promoted":
			item, found := itemByID[entryID]
			if !found || verify(entry, contextDigest) != nil ||
				ValidatePromotedItemForContextV1(entry, item, threadID, turnID, contextDigest) != nil {
				return nil, ErrProjectionInvalid
			}
		default:
			return nil, ErrProjectionInvalid
		}
	}
	return cloneSteeringMapsV1(pending), nil
}

func cloneSteeringMapsV1(values []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, cloneSteeringMapV1(value))
	}
	return out
}

func cloneSteeringMapV1(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneSteeringValueV1(value)
	}
	return out
}

func cloneSteeringValueV1(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSteeringMapV1(typed)
	case []any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = cloneSteeringValueV1(typed[index])
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func steeringListV1(value any) []any {
	values, _ := value.([]any)
	return values
}
