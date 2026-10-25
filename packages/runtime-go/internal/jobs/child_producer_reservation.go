package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

const maxChildRunSequence = int(^uint(0) >> 1)

// ChildRunReservationV1 owns only a process-local identity allocation. The
// host must separately bind and verify the complete signed child intent before
// consuming it. It is neither durable execution authority nor a restart token.
type ChildRunReservationV1 struct{ state *childRunReservationV1 }

type childRunReservationV1 struct {
	owner              *Manager
	binding            *domainjob.SecurityBinding
	hostInput          *domainjob.HostChildInputIdentityV1
	hostParentThreadID string
	target             domainpendingwork.ChildProducerTargetV1
	consumed           bool
}

func (reservation ChildRunReservationV1) TargetV1() domainpendingwork.ChildProducerTargetV1 {
	if reservation.state == nil {
		return domainpendingwork.ChildProducerTargetV1{}
	}
	return reservation.state.target
}

// ReserveChildRunV1 shares the ordinary manager counter but never creates a
// directory, job, log, or lease. Failed/unused reservations are not recycled.
func (m *Manager) ReserveChildRunV1(ctx context.Context, binding *domainjob.SecurityBinding, ordinal uint32, childThreadID, childTurnID string) (ChildRunReservationV1, error) {
	if m == nil || ctx == nil || ctx.Err() != nil || domainjob.ValidateSecurityBinding(binding) != nil || ordinal == 0 {
		return ChildRunReservationV1{}, errors.New("child-run reservation authority is invalid")
	}
	target := domainpendingwork.ChildProducerTargetV1{Ordinal: 1, JobID: "job-1", ChildThreadID: childThreadID, ChildTurnID: childTurnID}
	if domainpendingwork.ValidateChildProducerV1(&domainpendingwork.ChildProducerV1{ParentBindingDigest: binding.BindingDigest, Children: []domainpendingwork.ChildProducerTargetV1{target}}) != nil ||
		childThreadID == binding.ParentThreadID || childTurnID == binding.ParentTurnID {
		return ChildRunReservationV1{}, errors.New("child-run reservation target is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ChildRunReservationV1{}, err
	}
	m.restartEffectsStarted = true
	if m.restartPreservesRecordNoLock(Record{ParentThreadID: binding.ParentThreadID, ChildThreadID: childThreadID}) {
		return ChildRunReservationV1{}, ErrRestartPreserved
	}
	if _, err := m.readCurrentRestartRecordsNoLock(); err != nil {
		return ChildRunReservationV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return ChildRunReservationV1{}, err
	}
	id, err := m.nextChildRunIDNoLock()
	if err != nil {
		return ChildRunReservationV1{}, err
	}
	target.Ordinal, target.JobID = ordinal, id
	return ChildRunReservationV1{state: &childRunReservationV1{owner: m, binding: domainjob.CloneSecurityBinding(binding), target: target}}, nil
}

func (m *Manager) RevalidateChildRunReservationV1(ctx context.Context, reservation ChildRunReservationV1) error {
	if m == nil || ctx == nil || ctx.Err() != nil {
		return errors.New("child-run reservation context is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := m.validateChildRunReservationNoLock(reservation); err != nil {
		return err
	}
	return ctx.Err()
}

func (m *Manager) validateChildRunReservationNoLock(reservation ChildRunReservationV1) error {
	state := reservation.state
	if state == nil || state.owner != m || state.consumed || !validPersistedJobID(state.target.JobID) || sequence(state.target.JobID) > m.seq {
		return errors.New("child-run reservation is stale or foreign")
	}
	parentThreadID := state.hostParentThreadID
	if state.binding != nil {
		parentThreadID = state.binding.ParentThreadID
	}
	if m.restartPreservesRecordNoLock(Record{ParentThreadID: parentThreadID, ChildThreadID: state.target.ChildThreadID}) {
		return ErrRestartPreserved
	}
	records, err := m.readCurrentRestartRecordsNoLock()
	if err != nil {
		return err
	}
	if _, exists := records[state.target.JobID]; exists {
		return errors.New("reserved child-run identity is occupied")
	}
	return nil
}

// StartReservedChildRunV1 consumes the exact host allocation once. Only the
// producer's already-verified intent path supplies this process-local token;
// public StartRequest fields cannot select an arbitrary job identity.
func (m *Manager) StartReservedChildRunV1(ctx context.Context, request StartRequest, reservation ChildRunReservationV1) (Record, error) {
	if request.Kind != "subagent" || request.Status != "queued" {
		return Record{}, errors.New("reserved child must start as a queued subagent")
	}
	state := reservation.state
	if state == nil || state.owner != m || domainjob.ValidateSecurityBinding(request.SecurityBinding) != nil ||
		request.SecurityBinding.BindingDigest != state.binding.BindingDigest ||
		(request.ChildThreadID != "" && request.ChildThreadID != state.target.ChildThreadID) ||
		(request.ChildTurnID != "" && request.ChildTurnID != state.target.ChildTurnID) {
		return Record{}, errors.New("child-run request differs from host reservation")
	}
	ordinal := request.ParallelIndex
	if ordinal == 0 {
		ordinal = 1
	}
	if ordinal < 1 || uint64(ordinal) != uint64(state.target.Ordinal) {
		return Record{}, errors.New("child-run reservation ordinal differs from request")
	}
	request.ChildThreadID, request.ChildTurnID = state.target.ChildThreadID, state.target.ChildTurnID
	return m.startChildRun(ctx, request, &reservation)
}

func (m *Manager) nextChildRunIDNoLock() (string, error) {
	if m.seq < 0 || m.seq == maxChildRunSequence {
		return "", errors.New("child-run identity counter is exhausted")
	}
	m.seq++
	return fmt.Sprintf("job-%d", m.seq), nil
}

func (m *Manager) consumeChildRunIDNoLock(reservation *ChildRunReservationV1) (string, error) {
	if reservation == nil {
		return m.nextChildRunIDNoLock()
	}
	if err := m.validateChildRunReservationNoLock(*reservation); err != nil {
		return "", err
	}
	reservation.state.consumed = true
	return reservation.state.target.JobID, nil
}

// ReserveHostChildInputRunV1 allocates identities only. It shares the existing
// manager counter and reservation; the private Host turn path owns authority.
func (m *Manager) ReserveHostChildInputRunV1(ctx context.Context, parentThreadID, childThreadID, childTurnID string, identity *domainjob.HostChildInputIdentityV1) (ChildRunReservationV1, error) {
	if m == nil || ctx == nil || ctx.Err() != nil || domainjob.ValidateHostChildInputIdentityV1(identity) != nil || identity.ClaimedAt != "" ||
		strings.TrimSpace(parentThreadID) == "" || strings.TrimSpace(childThreadID) == "" || strings.TrimSpace(childTurnID) == "" || parentThreadID == childThreadID {
		return ChildRunReservationV1{}, errors.New("Host child input reservation is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ChildRunReservationV1{}, err
	}
	if m.restartPreservesRecordNoLock(Record{ParentThreadID: parentThreadID, ChildThreadID: childThreadID}) {
		return ChildRunReservationV1{}, ErrRestartPreserved
	}
	if _, err := m.readCurrentRestartRecordsNoLock(); err != nil {
		return ChildRunReservationV1{}, err
	}
	id, err := m.nextChildRunIDNoLock()
	if err != nil {
		return ChildRunReservationV1{}, err
	}
	m.restartEffectsStarted = true
	return ChildRunReservationV1{state: &childRunReservationV1{owner: m, hostInput: domainjob.CloneHostChildInputIdentityV1(identity), hostParentThreadID: parentThreadID,
		target: domainpendingwork.ChildProducerTargetV1{Ordinal: 1, JobID: id, ChildThreadID: childThreadID, ChildTurnID: childTurnID}}}, nil
}

func (m *Manager) StartReservedHostChildInputRunV1(ctx context.Context, request StartRequest, reservation ChildRunReservationV1) (Record, error) {
	state := reservation.state
	if m == nil || state == nil || state.owner != m || state.binding != nil || state.hostInput == nil || request.HostChildInput == nil ||
		*request.HostChildInput != *state.hostInput || request.ParentThreadID != state.hostParentThreadID || request.SecurityBinding != nil ||
		request.Kind != "subagent" || request.Status != "queued" || request.ChildThreadID != state.target.ChildThreadID || request.ChildTurnID != state.target.ChildTurnID {
		return Record{}, errors.New("Host child input differs from its process reservation")
	}
	return m.startChildRun(ctx, request, &reservation)
}
