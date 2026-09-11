package security

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSettledToolReferences = 512

// SettledToolReference identifies one exact host-persisted tool_result without
// carrying its private output. Registry replay and the durable result item must
// both validate before the reference can authorize provider continuation.
type SettledToolReference struct {
	GrantID      string `json:"grantId"`
	ResultItemID string `json:"resultItemId"`
}

func ValidateSettledToolReferences(references []SettledToolReference) error {
	if len(references) > maxSettledToolReferences {
		return errors.New("settled tool reference count is invalid")
	}
	seenGrants := map[string]bool{}
	seenResults := map[string]bool{}
	for _, reference := range references {
		grantID := strings.TrimSpace(reference.GrantID)
		resultItemID := strings.TrimSpace(reference.ResultItemID)
		if !IsSHA256Hex(grantID) || grantID != reference.GrantID || !strings.HasPrefix(resultItemID, "item_result_") || resultItemID != reference.ResultItemID ||
			!utf8.ValidString(resultItemID) || len(resultItemID) > 4096 || strings.IndexFunc(resultItemID, unicode.IsControl) >= 0 ||
			seenGrants[grantID] || seenResults[resultItemID] {
			return errors.New("settled tool reference is invalid")
		}
		seenGrants[grantID] = true
		seenResults[resultItemID] = true
	}
	return nil
}
