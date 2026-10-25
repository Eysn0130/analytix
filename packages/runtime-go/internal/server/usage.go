package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"

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
	handlers := h.usageHandlers()
	if strings.TrimSpace(r.URL.Query().Get("group_by")) == "thread" && h.providerUsage != nil {
		handlers.Records = func(threadID string) ([]usageRecord, error) {
			return h.conversationUsageRecordsV1(r.Context(), threadID)
		}
	}
	handlers.Handle(w, r)
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

func (h *runtimeServerHandler) conversationUsageRecordsV1(ctx context.Context, threadID string) ([]usageRecord, error) {
	records, err := h.runtimeUsageService().Records(threadID)
	if err != nil {
		return nil, err
	}
	ids := []string{strings.TrimSpace(threadID)}
	if ids[0] == "" {
		ids, err = (runtimeUsageRepository{store: h.store}).ListRuntimeThreadIDs()
		if err != nil {
			return nil, err
		}
	}
	for _, id := range ids {
		thread, err := h.store.GetThread(id)
		if err != nil {
			return nil, err
		}
		if thread == nil {
			continue
		}
		replay, err := h.store.LoadEventsSinceContext(ctx, id, 0)
		if err != nil {
			return nil, err
		}
		if len(replay.Diagnostics) != 0 {
			return nil, errors.New("conversation usage event inventory is unavailable")
		}
		projection, err := usageapp.ObserveConversationUsageV1(ctx, thread, replay.Events, h.conversationUsageCommittedContextsV1(), h.providerUsage)
		if err != nil {
			return nil, err
		}
		records = append(records, usageRecord{ThreadID: id, Conversation: &projection, ProjectionOnly: true})
	}
	return records, nil
}

func conversationUsageContextsV1(thread map[string]any, events []map[string]any) ([]domainsecurity.TurnSecurityContext, error) {
	return usageapp.ConversationUsageContextsV1(thread, events)
}

func (h *runtimeServerHandler) conversationUsageHistoryV1(thread map[string]any, events []map[string]any) (map[string]any, error) {
	return usageapp.ConversationUsageHistoryV1(thread, events, h.conversationUsageCommittedContextsV1())
}

func (h *runtimeServerHandler) conversationUsageCommittedContextsV1() []domainsecurity.TurnSecurityContext {
	var contexts []domainsecurity.TurnSecurityContext
	if registry, ok := h.caseThreads.(interface {
		CommittedContexts() []casethreadapp.CommittedContext
	}); ok {
		for _, committed := range registry.CommittedContexts() {
			contexts = append(contexts, committed.SecurityContext)
		}
	}
	return contexts
}
