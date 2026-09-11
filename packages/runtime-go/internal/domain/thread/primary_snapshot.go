package thread

import "errors"

// ValidatePrimaryIdentityV1 checks the complete primary identity inventory.
// It does not normalize aliases, recover sidecars, or classify case authority.
func ValidatePrimaryIdentityV1(threadID string, primary map[string]any) error {
	id, ok := primary["id"].(string)
	if !IsCanonicalRecordID(threadID) || !ok || id != threadID {
		return errors.New("primary thread identity is invalid")
	}
	turns, ok := primary["turns"].([]any)
	if !ok {
		return errors.New("primary thread turn inventory is invalid")
	}
	seen := make(map[string]bool, len(turns))
	for _, value := range turns {
		turn, ok := value.(map[string]any)
		if !ok || turn == nil {
			return errors.New("primary thread contains a non-object turn")
		}
		turnID, ok := turn["id"].(string)
		if !ok || !IsCanonicalRecordID(turnID) || seen[turnID] {
			return errors.New("primary thread contains an invalid or duplicate turn identity")
		}
		if parent, present := turn["threadId"]; present && parent != threadID {
			return errors.New("primary thread contains a foreign turn")
		}
		seen[turnID] = true
	}
	return nil
}
