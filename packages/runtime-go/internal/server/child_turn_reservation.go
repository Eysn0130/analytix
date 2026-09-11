package server

import (
	"context"
	"errors"
	"fmt"

	turnstartapp "analytix.local/runtime-go/internal/app/turnstart"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type childTurnReservationV1 struct {
	owner    *runtimeServerHandler
	threadID string
	turnID   string
	sequence int
	claim    *childTurnReservationClaimV1
}

type childTurnReservationClaimV1 struct{ consumed bool }

// consumeRuntimeChildTurnV1 accepts only the process slot retained from the
// signed producer and the exact running start claim. Durable inventory alone
// cannot reconstruct it. Consumption precedes every turn-start effect.
func (h *runtimeServerHandler) consumeRuntimeChildTurnV1(ctx context.Context, authority *turnstartapp.ChildTransitionAuthority) (string, int, error) {
	if h == nil || ctx == nil || ctx.Err() != nil || h.jobs == nil || authority == nil {
		return "", 0, errors.New("child turn producer authority is unavailable")
	}
	slot, ok := ctx.Value(runtimeChildProducerSlotKeyV1{}).(*runtimeChildProducerSlotV1)
	record := authority.ExpectedRecord
	if !ok || slot == nil || slot.owner != h || slot.turn == nil || slot.target != slot.job.TargetV1() ||
		record.ID != slot.target.JobID || record.ChildThreadID != slot.target.ChildThreadID || record.ChildTurnID != slot.target.ChildTurnID ||
		authority.ChildRunID != record.ID || authority.ChildThreadID != record.ChildThreadID || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil ||
		record.SecurityBinding.BindingDigest != slot.parentBindingDigest || slot.turn.threadID != record.ChildThreadID || slot.turn.turnID != record.ChildTurnID {
		return "", 0, errors.New("child turn differs from signed process allocation")
	}
	if _, err := h.jobs.ValidateChildRunStart(record); err != nil {
		return "", 0, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	if err := h.validateRuntimeChildTurnNoLock(slot.turn); err != nil {
		return "", 0, err
	}
	slot.turn.claim.consumed = true
	return slot.turn.turnID, slot.turn.sequence, nil
}

func (h *runtimeServerHandler) reserveRuntimeChildTurnV1(ctx context.Context, threadID string) (*childTurnReservationV1, error) {
	if h == nil || ctx == nil || !domainthread.IsCanonicalRecordID(threadID) {
		return nil, errors.New("child turn reservation is invalid")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	turnID, sequence, err := h.nextRuntimeTurnIdentityNoLock()
	if err != nil {
		return nil, err
	}
	return &childTurnReservationV1{owner: h, threadID: threadID, turnID: turnID, sequence: sequence, claim: &childTurnReservationClaimV1{}}, nil
}

func (h *runtimeServerHandler) revalidateRuntimeChildTurnV1(ctx context.Context, reservation *childTurnReservationV1) error {
	if h == nil || ctx == nil {
		return errors.New("child turn reservation context is unavailable")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.validateRuntimeChildTurnNoLock(reservation)
}

func (h *runtimeServerHandler) validateRuntimeChildTurnNoLock(reservation *childTurnReservationV1) error {
	if reservation == nil || reservation.owner != h || reservation.claim == nil || reservation.claim.consumed ||
		reservation.sequence <= 0 || reservation.sequence > h.turnSeq || !domainthread.IsCanonicalRecordID(reservation.threadID) ||
		reservation.turnID != fmt.Sprintf("turn_%d", reservation.sequence) {
		return errors.New("child turn reservation is stale or foreign")
	}
	return nil
}

func (h *runtimeServerHandler) nextRuntimeTurnIdentityNoLock() (string, int, error) {
	if h.turnSeq < 0 || h.turnSeq == int(^uint(0)>>1) {
		return "", 0, errors.New("runtime turn identity counter is invalid or exhausted")
	}
	h.turnSeq++
	return fmt.Sprintf("turn_%d", h.turnSeq), h.turnSeq, nil
}
