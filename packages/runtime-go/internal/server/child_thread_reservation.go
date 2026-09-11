package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// childThreadReservationV1 is private to the runtime producer. It allocates
// identity only; the signed child intent must be verified before consumption.
type childThreadReservationV1 struct {
	owner          *DurableEventSessionStore
	parentThreadID string
	sourceThreadID string
	threadID       string
	sequence       int
	fork           bool
	claim          *childThreadReservationClaimV1
}

type childThreadReservationClaimV1 struct{ consumed bool }

func (s *DurableEventSessionStore) reserveChildThreadV1(ctx context.Context, parentThreadID, sourceThreadID string) (*childThreadReservationV1, error) {
	if s == nil || ctx == nil || !domainthread.IsCanonicalRecordID(parentThreadID) ||
		(sourceThreadID != "" && !domainthread.IsCanonicalRecordID(sourceThreadID)) {
		return nil, errors.New("child thread reservation is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	meta, err := s.readMetaNoLock()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fork := sourceThreadID != ""
	threadID, sequence, err := s.nextChildThreadIdentityNoLock(meta, fork)
	if err != nil {
		return nil, err
	}
	reservation := &childThreadReservationV1{owner: s, parentThreadID: parentThreadID, sourceThreadID: sourceThreadID,
		threadID: threadID, sequence: sequence, fork: fork, claim: &childThreadReservationClaimV1{}}
	if err := s.validateChildThreadReservationNoLock(reservation); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return reservation, nil
}

func (s *DurableEventSessionStore) revalidateChildThreadReservationV1(ctx context.Context, reservation *childThreadReservationV1) error {
	if s == nil || ctx == nil {
		return errors.New("child thread reservation context is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validateChildThreadReservationNoLock(reservation); err != nil {
		return err
	}
	return ctx.Err()
}

func (s *DurableEventSessionStore) validateChildThreadReservationNoLock(reservation *childThreadReservationV1) error {
	if reservation == nil || reservation.owner != s || reservation.claim == nil || reservation.claim.consumed || reservation.sequence <= 0 ||
		!domainthread.IsCanonicalRecordID(reservation.threadID) {
		return errors.New("child thread reservation is stale or foreign")
	}
	prefix, floor := "thr_durable_", s.childThreadCounterFloor
	if reservation.fork {
		prefix, floor = "thr_durable_fork_", s.childForkCounterFloor
	}
	if reservation.sequence > floor || reservation.threadID != prefix+strconv.Itoa(reservation.sequence) {
		return errors.New("child thread reservation identity changed")
	}
	if _, err := os.Lstat(s.threadDir(reservation.threadID)); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("reserved child thread identity is occupied")
	}
	return nil
}

func (s *DurableEventSessionStore) createReservedChildThreadV1(ctx context.Context, reservation *childThreadReservationV1, request map[string]any, workspace string) (map[string]any, error) {
	if reservation == nil || reservation.owner != s || reservation.fork || request["parentThreadId"] != reservation.parentThreadID || request["relation"] != "side" {
		return nil, errors.New("child creation differs from host reservation")
	}
	return s.createThread(ctx, request, workspace, reservation)
}

func (s *DurableEventSessionStore) forkReservedChildThreadV1(ctx context.Context, reservation *childThreadReservationV1, sourceThreadID string, request map[string]any) (map[string]any, error) {
	if reservation == nil || reservation.owner != s || !reservation.fork || sourceThreadID != reservation.sourceThreadID ||
		request["parentThreadId"] != reservation.parentThreadID || request["relation"] != "side" {
		return nil, errors.New("child fork differs from host reservation")
	}
	return s.forkThread(ctx, sourceThreadID, request, reservation)
}

func (s *DurableEventSessionStore) nextChildThreadIdentityNoLock(meta durableMeta, fork bool) (string, int, error) {
	if meta.ThreadCounter < 0 || meta.ForkCounter < 0 || meta.ResumeCounter < 0 {
		return "", 0, errors.New("durable thread identity counter is invalid")
	}
	sequence, floor, prefix := meta.ThreadCounter, &s.childThreadCounterFloor, "thr_durable_"
	if fork {
		sequence, floor, prefix = meta.ForkCounter, &s.childForkCounterFloor, "thr_durable_fork_"
	}
	if *floor > sequence {
		sequence = *floor
	}
	if sequence == int(^uint(0)>>1) {
		return "", 0, errors.New("durable thread identity counter is exhausted")
	}
	sequence++
	*floor = sequence
	return fmt.Sprintf("%s%d", prefix, sequence), sequence, nil
}

func (s *DurableEventSessionStore) consumeChildThreadIdentityNoLock(meta *durableMeta, reservation *childThreadReservationV1, fork bool) (string, error) {
	var threadID string
	var sequence int
	if reservation == nil {
		var err error
		threadID, sequence, err = s.nextChildThreadIdentityNoLock(*meta, fork)
		if err != nil {
			return "", err
		}
	} else {
		if reservation.fork != fork {
			return "", errors.New("child thread reservation mode changed")
		}
		if err := s.validateChildThreadReservationNoLock(reservation); err != nil {
			return "", err
		}
		reservation.claim.consumed = true
		threadID, sequence = reservation.threadID, reservation.sequence
	}
	// Preserve the global high-water even when reservations are consumed in a
	// different order. The signed inventory restores unwritten slots at restart.
	if fork {
		meta.ForkCounter = max(meta.ForkCounter, sequence, s.childForkCounterFloor)
	} else {
		meta.ThreadCounter = max(meta.ThreadCounter, sequence, s.childThreadCounterFloor)
	}
	return threadID, nil
}
