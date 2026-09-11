package thread

import (
	"encoding/json"
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrThreadMutationBaselineConflict = errors.New("durable thread changed before mutation commit")
	ErrThreadMutationTransition       = errors.New("thread mutation security transition is unavailable")
	ErrCaseWorkspaceSignedRebind      = errors.New("case workspace change requires signed host rebind authority")
	ErrCaseRewindSignedArchive        = errors.New("case rewind requires a signed authority archive")
	ErrCaseDeleteSignedTombstone      = errors.New("case delete requires a signed authority tombstone")
)

func MutationBaselineDigest(thread map[string]any) (string, error) {
	body, err := json.Marshal(thread)
	if err != nil || len(body) == 0 {
		return "", errors.New("durable thread mutation baseline is not canonical")
	}
	return domainsecurity.CanonicalJSONHash(body), nil
}

func ValidateMutationBaseline(thread map[string]any, threadID, expectedDigest string) error {
	currentDigest, err := MutationBaselineDigest(thread)
	if err != nil || !domainsecurity.IsSHA256Hex(expectedDigest) || currentDigest != expectedDigest ||
		strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
		return ErrThreadMutationBaselineConflict
	}
	return nil
}

func ThreadIsCaseBound(authority CaseThreadAuthority, threadID string, thread map[string]any) bool {
	if authority != nil && authority.IsCaseThread(strings.TrimSpace(threadID)) {
		return true
	}
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	return err != nil || caseSensitive
}
