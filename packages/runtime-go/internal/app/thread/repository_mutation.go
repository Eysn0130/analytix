package thread

import (
	"errors"
	"strings"
)

// ReadMutationBaseline validates the route/body identity before returning the
// canonical digest used by every durable metadata CAS.
func ReadMutationBaseline(thread map[string]any, threadID string) (string, error) {
	if thread == nil {
		return "", ErrThreadNotFound
	}
	if strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
		return "", errors.New("durable thread route and body identity mismatch")
	}
	return MutationBaselineDigest(thread)
}

func ApplyPatchMutation(thread map[string]any, threadID string, patch map[string]any, expectedDigest, now string) (map[string]any, error) {
	if thread == nil {
		return nil, ErrThreadNotFound
	}
	if expectedDigest != "" {
		if err := ValidateMutationBaseline(thread, threadID, expectedDigest); err != nil {
			return nil, err
		}
	}
	return ApplyPatch(thread, patch, now), nil
}

func ApplyDeleteMutation(thread map[string]any, threadID, expectedDigest, now string, listedThreadIDs []string) (map[string]any, []string, error) {
	if thread == nil {
		return nil, nil, ErrThreadNotFound
	}
	if expectedDigest != "" {
		if err := ValidateMutationBaseline(thread, threadID, expectedDigest); err != nil {
			return nil, nil, err
		}
	}
	listed := make([]string, 0, len(listedThreadIDs))
	for _, id := range listedThreadIDs {
		if id != threadID {
			listed = append(listed, id)
		}
	}
	return MarkDeleted(thread, now), listed, nil
}

func ApplyRewindMutation(thread map[string]any, threadID, turnID, expectedDigest, now string) (RewindPlan, error) {
	if thread == nil {
		return RewindPlan{}, ErrThreadNotFound
	}
	if expectedDigest != "" {
		if err := ValidateMutationBaseline(thread, threadID, expectedDigest); err != nil {
			return RewindPlan{}, err
		}
	}
	return BuildRewind(RewindInput{Thread: thread, ThreadID: threadID, TurnID: turnID, Now: now})
}
