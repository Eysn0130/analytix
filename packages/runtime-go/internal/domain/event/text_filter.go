package event

import (
	"errors"

	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

// FilterPublicText is the single event/history/compaction reasoning-markup
// projection. It preserves public bytes exactly and returns an error for an
// incomplete, malformed, or over-limit stream. User-authored text must bypass
// this function: raw provider markers are reserved protocol syntax, while user
// input remains untrusted data that may discuss the syntax literally.
func FilterPublicText(value string) (string, error) {
	outcome, err := domainreasoningmarkup.Filter(value)
	if err != nil {
		return "", errors.Join(ErrPrivateReasoningPersistence, err)
	}
	return outcome.PublicText, nil
}
