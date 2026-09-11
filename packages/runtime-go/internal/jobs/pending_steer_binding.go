package jobs

import (
	"errors"
	"os"
	"reflect"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

// BindPendingSteerMessageForTurn preserves the original queue content and
// authority digests while binding its previously absent execution context.
// The host holds current frozen-turn effect/control authority until admission
// finishes; this CAS cannot derive that authority from a nonempty turn ID.
func (m *Manager) BindPendingSteerMessageForTurn(id string, authority domainjob.SteerQueueAuthorityV1, expected domainjob.SteerMessage) (Record, domainjob.SteerMessage, error) {
	if m == nil || id == "" || authority.PendingUntilChildTurn || expected.Status != "queued" || expected.ContextDigest != "" {
		return Record{}, domainjob.SteerMessage{}, errors.New("pending steer turn binding is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.jobs {
		record := m.jobs[index]
		if record.ID != id {
			continue
		}
		if m.restartPreservesRecordNoLock(record) {
			return cloneRecord(record), domainjob.SteerMessage{}, ErrRestartPreserved
		}
		if record.Status != "running" || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil ||
			domainjob.ValidateSteerQueueAuthorityForRecordV1(authority, record) != nil ||
			expected.AuthorityDigest != record.SecurityBinding.BindingDigest {
			return cloneRecord(record), domainjob.SteerMessage{}, errors.New("pending steer current turn authority changed")
		}
		for steerIndex, message := range record.Steers {
			if message.ID != expected.ID {
				continue
			}
			if !reflect.DeepEqual(message, expected) || message.ContentDigest != domainjob.SteerMessageContentDigestV1(message) {
				return cloneRecord(record), domainjob.SteerMessage{}, errors.New("pending steer original content changed")
			}
			record.Steers = cloneSteerMessages(record.Steers)
			record.Steers[steerIndex].ContextDigest = authority.ContextDigest
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			record.SteerState = childRunState(record)
			if err := m.writeRecordNoLock(&record); err != nil {
				return Record{}, domainjob.SteerMessage{}, err
			}
			m.jobs[index] = record
			return cloneRecord(record), record.Steers[steerIndex], nil
		}
		return cloneRecord(record), domainjob.SteerMessage{}, os.ErrNotExist
	}
	return Record{}, domainjob.SteerMessage{}, os.ErrNotExist
}
