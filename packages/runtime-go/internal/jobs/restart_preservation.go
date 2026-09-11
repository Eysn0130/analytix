package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

var ErrRestartPreserved = errors.New("child run is preserved after restart")

type restartPreservationV1 struct {
	threads   map[string]bool
	jobIDs    map[string]bool
	records   map[string]string
	inventory ChildRunInventoryV1
}

// PreserveRestartScopeV1 installs a denial-only scope after the startup owner
// has proved the complete Core parent/grant/child dependency graph. Matching
// parent/child strings here checks propagation completeness, not provenance.
// This method never supplies ordinary admission or repairs missing bindings.
func (m *Manager) PreserveRestartScopeV1(ctx context.Context, threadIDs []string, records []Record) error {
	if m == nil || ctx == nil {
		return errors.New("child-run restart preservation owner is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restartPreserved != nil || m.restartEffectsStarted {
		return errors.New("child-run restart preservation installation is closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	scope := &restartPreservationV1{threads: map[string]bool{}}
	for _, id := range threadIDs {
		if !domainthread.IsCanonicalRecordID(id) || scope.threads[id] {
			return errors.New("child-run restart thread identity is invalid")
		}
		scope.threads[id] = true
	}
	provided, err := restartRecordDigestsV1(records)
	if err != nil {
		return err
	}
	scope.inventory, err = BuildChildRunInventoryV1(m.root)
	if err != nil {
		return err
	}
	current, err := m.readCurrentRestartRecordsNoLock()
	if err != nil {
		return err
	}
	observed, err := restartObservedRecordDigestsV1(m.jobs)
	if err != nil {
		return err
	}
	scope.records = map[string]string{}
	matched := 0
	for _, record := range m.jobs {
		if scope.threads[record.ParentThreadID] || scope.threads[record.ChildThreadID] {
			if provided[record.ID] != observed[record.ID] {
				return errors.New("child-run restart dependency scope is incomplete or changed")
			}
			scope.records[record.ID] = current[record.ID]
			matched++
		} else if _, selected := provided[record.ID]; selected {
			return errors.New("child-run restart record is outside the proved thread scope")
		}
	}
	if matched != len(provided) {
		return errors.New("child-run restart record inventory changed")
	}
	if err := ValidateChildRunInventoryV1(m.root, scope.inventory); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.restartPreserved = scope
	return nil
}

func (m *Manager) restartPreservesRecordNoLock(record Record) bool {
	if m.restartPreserved == nil {
		return false
	}
	return m.restartPreserved.ownsJob(record.ID) || m.restartPreserved.ownsThreads(record)
}

// RestartPreservesChildRunV1 is required by startup recovery consumers. Its
// first query closes scope installation before returning, so a caller that
// observes false cannot race a new hold during its later effect callbacks.
// No manager mutex is held across callbacks that may themselves update jobs. A
// stale record or changed physical/semantic inventory is an error, not an
// unowned job that may be disposed or delivered.
func (m *Manager) RestartPreservesChildRunV1(record Record) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartEffectsStarted = true
	if m.restartPreserved == nil {
		return false, nil
	}
	_, err := m.revalidateRestartPreservationNoLock()
	if err != nil {
		return false, err
	}
	observed, err := restartObservedRecordDigestsV1(m.jobs)
	if err != nil {
		return false, err
	}
	provided, err := restartRecordDigestsV1([]Record{record})
	if err != nil || provided[record.ID] != observed[record.ID] {
		return false, errors.New("child-run restart consumer record changed")
	}
	return m.restartPreservesRecordNoLock(record), nil
}

// LoadChildRun and AllRecords return cloneRecord's UI projection. Compare the
// complete caller observation against that exact projection, while the scope
// retains full original record digests and physical bytes independently.
func restartObservedRecordDigestsV1(records []Record) (map[string]string, error) {
	observed := make([]Record, len(records))
	for i, record := range records {
		observed[i] = cloneRecord(record)
	}
	return restartRecordDigestsV1(observed)
}

func (m *Manager) revalidateRestartPreservationNoLock() (map[string]string, error) {
	current, err := m.readCurrentRestartRecordsNoLock()
	if err != nil {
		return nil, err
	}
	if m.restartPreserved == nil {
		return current, nil
	}
	inventory, err := BuildChildRunInventoryV1(m.root)
	if err != nil {
		return nil, err
	}
	if err := m.restartPreserved.validateInventory(inventory); err != nil {
		return nil, err
	}
	for id, digest := range m.restartPreserved.records {
		if current[id] != digest {
			return nil, errors.New("preserved child-run authority changed")
		}
	}
	return current, nil
}

func (m *Manager) readCurrentRestartRecordsNoLock() (map[string]string, error) {
	observed, _, err := readRecords(context.Background(), m.root, m.artifactAuthorityRoot, m.completionVerifier, nil, false, m.seq, m.restartPreserved)
	if err != nil {
		return nil, err
	}
	current, err := restartRecordDigestsV1(observed)
	if err != nil {
		return nil, err
	}
	loaded, err := restartRecordDigestsV1(m.jobs)
	if err != nil || !reflect.DeepEqual(current, loaded) {
		return nil, errors.New("child-run restart loaded inventory changed")
	}
	return current, nil
}

func restartRecordDigestsV1(records []Record) (map[string]string, error) {
	result := make(map[string]string, len(records))
	for _, record := range records {
		if !validPersistedJobID(record.ID) || result[record.ID] != "" {
			return nil, errors.New("child-run restart record identity is invalid")
		}
		body, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		result[record.ID] = domainsecurity.SHA256Hex(body)
	}
	return result, nil
}
