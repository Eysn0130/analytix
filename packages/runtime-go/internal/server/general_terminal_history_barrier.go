package server

import (
	"errors"
	"os"
	"strings"

	turnapp "analytix.local/runtime-go/internal/app/turn"
)

var errUsageIndexRebuildHistoryMutation = errors.New("history rewrite is unavailable during usage index rebuild")

// Terminal appends can be merged into a reader's deferred event inventory, but
// a rewind or compaction can remove the canonical input that reader retained.
// The caller holds s.mu (also the usage index Owner) until the rewrite ends,
// so a rebuild cannot start between this check and the durable mutation.
func (s *DurableEventSessionStore) settleGeneralTerminalPublicationsBeforeHistoryRewriteNoLock(threadID string) error {
	if s == nil || s.usageIndex == nil {
		return errors.New("history rewrite usage index is unavailable")
	}
	if s.usageIndex.StatsOwnerLocked().RebuildActive {
		return errUsageIndexRebuildHistoryMutation
	}
	return s.settleGeneralTerminalPublicationsBeforeHistoryMutationNoLock(threadID)
}

// settleGeneralTerminalPublicationsBeforeHistoryMutationNoLock prevents
// compaction or another destructive history mutation from deleting the only
// canonical assistant item needed to reconstruct a crash-gap terminal bundle.
// The caller owns s.mu for the complete preflight, repair, and readback.
func (s *DurableEventSessionStore) settleGeneralTerminalPublicationsBeforeHistoryMutationNoLock(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if s == nil || threadID == "" || safeDurableID(threadID) != threadID {
		return errors.New("general terminal history barrier identity is invalid")
	}
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return os.ErrNotExist
	}
	loaded, err := s.loadEventsSinceFileNoLock(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return errors.Join(err, errors.New("general terminal history barrier event inventory is not replayable"))
	}
	entries, err := turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
	if err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, entry := range entries {
		if entry.State == turnapp.GeneralTerminalPublicationMissingV1 {
			if entry.ArchivedOnly {
				return errors.New("compacted general terminal publication bundle cannot be repaired")
			}
			missing = append(missing, entry.TurnID)
			continue
		}
		if err := s.settleGeneralTerminalDerivedStateNoLock(entry); err != nil {
			return err
		}
	}
	for _, turnID := range missing {
		if _, err := s.recordGeneralTerminalEventBundleNoLock(threadID, turnID); err != nil {
			return err
		}
	}

	thread, err = s.readThreadNoLock(threadID)
	if err != nil || thread == nil {
		return errors.Join(err, errors.New("general terminal history barrier lost its canonical thread"))
	}
	loaded, err = s.loadEventsSinceFileNoLock(threadID, 0)
	if err != nil || len(loaded.Diagnostics) != 0 {
		return errors.Join(err, errors.New("general terminal history barrier readback is not replayable"))
	}
	entries, err = turnapp.PreflightGeneralTerminalPublicationInventoryV1(thread, loaded.Events)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.State != turnapp.GeneralTerminalPublicationCompleteV1 {
			return errors.New("general terminal history barrier left an unsettled publication bundle")
		}
		if err := s.settleGeneralTerminalDerivedStateNoLock(entry); err != nil {
			return err
		}
	}
	return nil
}
