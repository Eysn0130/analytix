package server

import (
	"net/http"

	"analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	usageapp "analytix.local/runtime-go/internal/app/usage"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type usageSnapshot = usageapp.Snapshot
type usageRecord = usageapp.Record
type usageIndexRecord = usageapp.IndexRecord

func (s *DurableEventSessionStore) EnsureUsageIndex() error {
	return s.usageIndex.Ensure()
}

func (s *DurableEventSessionStore) LoadUsageIndexRecords(threadID string) ([]usageRecord, error) {
	return s.usageIndex.LoadRecords(threadID)
}

func (h *runtimeServerHandler) handleUsage(w http.ResponseWriter, r *http.Request) {
	h.usageHandlers().Handle(w, r)
}

func (h *runtimeServerHandler) usageHandlers() httpapi.UsageHandlers {
	service := h.runtimeUsageService()
	return httpapi.UsageHandlers{
		Records:         service.Records,
		RuntimeResponse: service.RuntimeResponse,
	}
}

func (h *runtimeServerHandler) threadUsageSnapshot(threadID string) (map[string]any, error) {
	snapshot, err := h.runtimeUsageService().ThreadSnapshot(threadID)
	if err != nil {
		return nil, err
	}
	return snapshot.Map(), nil
}

func (h *runtimeServerHandler) runtimeUsageService() usageapp.RuntimeService {
	return usageapp.RuntimeService{Repository: runtimeUsageRepository{store: h.store}}
}

type runtimeUsageRepository struct {
	store *DurableEventSessionStore
}

func (r runtimeUsageRepository) ThreadExists(threadID string) (bool, error) {
	if !domainthread.IsCanonicalRecordID(threadID) {
		return false, nil
	}
	thread, err := r.store.GetThread(threadID)
	if err != nil {
		return false, err
	}
	return thread != nil, nil
}

func (r runtimeUsageRepository) LoadRecords(threadID string) ([]usageRecord, error) {
	return r.store.LoadUsageIndexRecords(threadID)
}

func (r runtimeUsageRepository) ListRuntimeThreadIDs() ([]string, error) {
	threads, err := r.store.ListThreads(false, true, false, "")
	if err != nil {
		return nil, err
	}
	threadIDs := make([]string, 0, len(threads))
	for _, thread := range threads {
		if threadID := stringField(thread, "id"); threadID != "" {
			threadIDs = append(threadIDs, threadID)
		}
	}
	return threadIDs, nil
}
