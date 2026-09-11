package server

import (
	"net/http"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadsummaryapp "analytix.local/runtime-go/internal/app/threadsummary"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	jobs "analytix.local/runtime-go/internal/jobs"
)

func (h *runtimeServerHandler) handleThreadSummaryPath(w http.ResponseWriter, r *http.Request, rest string) {
	httpapi.ThreadSummaryHandlers{Service: runtimeThreadSummaryHTTPService{handler: h}}.HandleThreadSummaryPath(w, r, rest)
}

func (h *runtimeServerHandler) runtimeThreadSummaryService() *threadsummaryapp.Service {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.threadSummaries == nil {
		h.threadSummaries = threadsummaryapp.NewService(threadsummaryapp.Dependencies{
			Repository:      threadSummaryRepositoryAdapter{store: h.store},
			Jobs:            threadSummaryHistoricalJobLister{jobs: h.jobs},
			PublicProjector: h.publicProjector,
		})
	}
	return h.threadSummaries
}

// threadSummaryHistoricalJobLister exposes only parent-scoped records to the
// summary read model. The read model always strips child output and authority;
// live control paths continue to use runtimeSubagentService and therefore
// require a current SecurityBindingV2.
type threadSummaryHistoricalJobLister struct {
	jobs *jobs.Manager
}

func (l threadSummaryHistoricalJobLister) ListTaskJobs(threadID string) ([]domainjob.Record, error) {
	if l.jobs == nil {
		return nil, nil
	}
	records, err := l.jobs.List(threadID)
	if err != nil {
		return nil, err
	}
	filtered := make([]domainjob.Record, 0, len(records))
	for _, record := range records {
		if subagentapp.TaskJobRecordAllowed(record, threadID) {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

type threadSummaryRepositoryAdapter struct {
	store *DurableEventSessionStore
}

func (a threadSummaryRepositoryAdapter) GetThread(threadID string) (map[string]any, error) {
	return a.store.GetThread(threadID)
}

func (a threadSummaryRepositoryAdapter) LoadEventsSince(threadID string, afterSeq int) ([]map[string]any, error) {
	result, err := a.store.LoadEventsSince(threadID, afterSeq)
	if err != nil {
		return nil, err
	}
	return result.Events, nil
}

func (a threadSummaryRepositoryAdapter) HighestSeq(threadID string) (int, error) {
	return a.store.HighestSeq(threadID)
}

func (a threadSummaryRepositoryAdapter) ListThreads(archivedOnly bool, includeArchived bool, includeSide bool, search string) ([]map[string]any, error) {
	return a.store.ListThreads(archivedOnly, includeArchived, includeSide, search)
}

type runtimeThreadSummaryHTTPService struct {
	handler *runtimeServerHandler
}

func (s runtimeThreadSummaryHTTPService) Summary(threadID string) (map[string]any, error) {
	return s.handler.runtimeThreadSummaryService().Summary(threadID)
}

func (s runtimeThreadSummaryHTTPService) LoadContext(threadID string) (threadsummaryapp.Context, error) {
	return s.handler.runtimeThreadSummaryService().LoadContext(threadID)
}

func (s runtimeThreadSummaryHTTPService) CommandOutput(context threadsummaryapp.Context, taskID string, offset int, limit int) (map[string]any, bool) {
	return s.handler.runtimeThreadSummaryService().CommandOutput(context, taskID, offset, limit)
}

func (s runtimeThreadSummaryHTTPService) OutputTaskJob(parentThreadID string, request subagentapp.TaskJobOutputRequest, useCursor bool) subagentapp.TaskJobServiceResult {
	return s.handler.runtimeSubagentService().OutputTaskJob(parentThreadID, request, useCursor)
}

func (s runtimeThreadSummaryHTTPService) KillTaskJob(parentThreadID string, request subagentapp.TaskJobKillRequest) subagentapp.TaskJobServiceResult {
	result := s.handler.runtimeSubagentService().KillTaskJob(parentThreadID, request)
	if !result.IsError && result.Record.ID != "" {
		s.handler.recordRuntimeJobLifecycleEvent(result.Record, result.Record.Status, firstNonEmptyString(result.Record.Error, request.Reason))
	}
	return result
}

func (s runtimeThreadSummaryHTTPService) RestartTaskJob(parentThreadID string, jobID string) subagentapp.TaskJobServiceResult {
	return s.handler.restartRuntimeSubagentTaskJob(parentThreadID, jobID)
}
