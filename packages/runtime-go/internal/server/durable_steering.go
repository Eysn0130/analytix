package server

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
)

func cancelPendingDurableSteeringEntries(value any, cancelledAt string, reason string) []any {
	return turnapp.CancelPendingSteeringEntries(value, cancelledAt, reason)
}

func (s *DurableEventSessionStore) AdmitSteeringEntryForContext(
	threadID, turnID, expectedTurnID, expectedContextDigest string,
	entry map[string]any,
) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(expectedTurnID) != "" && strings.TrimSpace(expectedTurnID) != turnID {
		return nil, errDurableExpectedTurnMismatch
	}
	thread, turns, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, err
	}
	if !turnapp.TurnAcceptsSteering(turn) {
		return nil, errDurableTurnNotSteerable
	}
	bound, err := domainsteering.BindPendingEntryV1(entry, expectedContextDigest)
	if err != nil {
		return nil, errDurableSteeringProjectionInvalid
	}
	sealed, err := s.sealSteeringEntryAuthorityV1(bound, expectedContextDigest)
	if err != nil {
		return nil, err
	}
	updated, admitted, changed, err := domainsteering.AdmitEntryToTurnV1(
		threadID, turnID, turn, sealed, expectedContextDigest, s.verifySteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, err
	}
	if !changed {
		return admitted, nil
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["status"] = "running"
	thread["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, err
	}
	return admitted, nil
}

func (s *DurableEventSessionStore) PromotePendingSteeringEntriesForContext(
	threadID, turnID, expectedContextDigest string,
) ([]map[string]any, []map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, turns, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, nil, err
	}
	updated, entries, items, err := domainsteering.PromoteTurnEntriesV1(
		threadID, turnID, turn, expectedContextDigest, time.Now().UTC().Format(time.RFC3339Nano),
		s.verifySteeringEntryAuthorityV1, s.promoteSteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, nil, err
	}
	return entries, items, nil
}

// PromotePendingSteeringEntryPrefixForContext performs the pending-prefix CAS
// and its durable turn update under the same store mutex. New tail admissions
// may therefore coexist with an earlier exact prefix, while a stale, skipped,
// or reordered expectation cannot promote a different batch.
func (s *DurableEventSessionStore) PromotePendingSteeringEntryPrefixForContext(
	threadID, turnID, expectedContextDigest string,
	expected []domainsteering.PendingEntryExpectationV1,
) ([]map[string]any, []map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, turns, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, nil, err
	}
	updated, entries, items, err := domainsteering.PromoteTurnEntryPrefixV1(
		threadID, turnID, turn, expected, expectedContextDigest, time.Now().UTC().Format(time.RFC3339Nano),
		s.verifySteeringEntryAuthorityV1, s.promoteSteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	turns[len(turns)-1] = updated
	thread["turns"] = turns
	thread["updatedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.upsertThreadNoLock(thread, false); err != nil {
		return nil, nil, err
	}
	return entries, items, nil
}

func (s *DurableEventSessionStore) PendingSteeringEntriesForContext(
	threadID, turnID, expectedContextDigest string,
) ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _, turn, err := s.currentSteeringTurnNoLock(threadID, turnID, expectedContextDigest)
	if err != nil {
		return nil, err
	}
	entries, err := domainsteering.PendingEntriesFromTurnV1(
		threadID, turnID, turn, expectedContextDigest, s.verifySteeringEntryAuthorityV1,
	)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *DurableEventSessionStore) currentSteeringTurnNoLock(
	threadID, turnID, expectedContextDigest string,
) (map[string]any, []any, map[string]any, error) {
	thread, err := s.readThreadNoLock(threadID)
	if err != nil {
		return nil, nil, nil, err
	}
	if thread == nil {
		return nil, nil, nil, os.ErrNotExist
	}
	turns, turn, err := domainsteering.CurrentExecutableTurnV1(thread, threadID, turnID, expectedContextDigest)
	if errors.Is(err, domainsteering.ErrTurnNotFound) {
		err = errDurableTurnNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return thread, turns, turn, nil
}

func (s *DurableEventSessionStore) sealSteeringEntryAuthorityV1(
	entry map[string]any,
	contextDigest string,
) (map[string]any, error) {
	if s.steeringAuthority == nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	sealed, err := s.steeringAuthority.Seal(context.Background(), entry, contextDigest)
	if err != nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	return sealed, nil
}

func (s *DurableEventSessionStore) verifySteeringEntryAuthorityV1(entry map[string]any, contextDigest string) error {
	if s.steeringAuthority == nil {
		return errDurableSteeringAuthorityUnavailable
	}
	if s.steeringAuthority.Verify(context.Background(), entry, contextDigest) != nil {
		return errDurableSteeringAuthorityUnavailable
	}
	return nil
}

func (s *DurableEventSessionStore) promoteSteeringEntryAuthorityV1(entry map[string]any, contextDigest string) (map[string]any, error) {
	if s.steeringAuthority == nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	promoted, err := s.steeringAuthority.Promote(context.Background(), entry, contextDigest)
	if err != nil {
		return nil, errDurableSteeringAuthorityUnavailable
	}
	return promoted, nil
}
