//go:build !analytix_prod

package livelocal

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	agent "analytix.local/runtime-go/internal/agent"
	contracts "analytix.local/runtime-go/internal/contracts"
)

const LiveLocalLoopPrefix = "/v1/conformance/loop"

type LiveLocalLoopHandler struct {
	runtimeToken string
	contract     agent.GoMinimalAgentLoopContract
	store        *DurableEventSessionStore

	mu             sync.Mutex
	runRecorded    bool
	cancelRecorded bool
	resumeRecorded bool
}

func NewLiveLocalLoopHandler(
	runtimeToken string,
	contract agent.GoMinimalAgentLoopContract,
	store *DurableEventSessionStore,
) *LiveLocalLoopHandler {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = DefaultRuntimeToken
	}
	return &LiveLocalLoopHandler{
		runtimeToken: runtimeToken,
		contract:     contract,
		store:        store,
	}
}

func (h *LiveLocalLoopHandler) Handles(path string) bool {
	return h != nil && h.store != nil && strings.HasPrefix(path, LiveLocalLoopPrefix)
}

func (h *LiveLocalLoopHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !Authorized(r, h.runtimeToken) {
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}

	switch {
	case r.URL.Path == LiveLocalLoopPrefix+"/boundary":
		h.handleBoundary(w, r)
	case r.URL.Path == LiveLocalLoopPrefix+"/run":
		h.handleRun(w, r)
	case r.URL.Path == LiveLocalLoopPrefix+"/cancel":
		h.handleCancel(w, r)
	case r.URL.Path == LiveLocalLoopPrefix+"/resume":
		h.handleResume(w, r)
	case r.URL.Path == LiveLocalLoopPrefix+"/replay":
		h.handleReplayJSON(w, r)
	case r.URL.Path == LiveLocalLoopPrefix+"/recovered-state":
		h.handleRecoveredState(w, r)
	case strings.HasPrefix(r.URL.Path, LiveLocalLoopPrefix+"/threads/"):
		h.handleThreadPath(w, r)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h *LiveLocalLoopHandler) handleBoundary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                         true,
		"testConformanceOnly":                 true,
		"routePrefix":                         LiveLocalLoopPrefix,
		"minimalAgentLoopPrototype":           true,
		"fixtureBackedLoopOnly":               true,
		"usesFixtureModelScript":              true,
		"tempDurableEventSessionStoreEnabled": true,
		"publishBeforePersist":                false,
		"persistBeforePublish":                true,
		"externalNetworkAllowed":              false,
		"providerCredentialsAllowed":          false,
		"apiKeyReadAllowed":                   false,
		"providerLiveCallsAllowed":            false,
		"toolExecutionAllowed":                false,
		"approvalExecutionAllowed":            false,
		"mcpConnectionAllowed":                false,
		"mcpCredentialsAllowed":               false,
		"credentialReadAllowed":               false,
		"fileMutationAllowed":                 false,
		"realWorkspaceMutationAllowed":        false,
		"electronMainConnected":               false,
		"defaultGoBackendEnabled":             false,
		"rendererVisibleGoRoutesAllowed":      false,
		"reasonixPublicProtocolAllowed":       false,
		"productBoundary":                     contracts.LiveLocalSidecarLoopProductBoundary(),
	})
}

func (h *LiveLocalLoopHandler) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if _, ok := RequestBody(w, r); !ok {
		return
	}
	events, orders, err := h.recordDrafts(h.contract.EventDrafts)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	h.mu.Lock()
	h.runRecorded = true
	h.mu.Unlock()

	recovered, err := h.store.RecoveredState(h.contract.ThreadID)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	highestSeq, _ := h.store.HighestSeq(h.contract.ThreadID)
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":             true,
		"status":                  "completed",
		"threadId":                h.contract.ThreadID,
		"turnId":                  h.contract.TurnID,
		"eventKinds":              contracts.EventKinds(events),
		"itemKinds":               itemKindsFromEvents(events),
		"highestSeq":              highestSeq,
		"publishPersistOrders":    orders,
		"allPersistBeforePublish": contracts.AllPersistBeforePublish(orders),
		"stablePrefix":            h.contract.StablePrefix,
		"modelRequestShape":       h.contract.ModelRequestShape,
		"providerCacheTelemetry":  h.contract.ProviderCacheTelemetry,
		"approvalDenied":          h.contract.ApprovalDenied,
		"userInputGates":          h.contract.UserInputGates,
		"mcpToolCatalog":          h.contract.MCPToolCatalog,
		"stepLimit":               h.contract.Control.StepLimit,
		"sideEffects":             h.sideEffects(),
		"recoveredState":          recovered,
	})
}

func (h *LiveLocalLoopHandler) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if _, ok := RequestBody(w, r); !ok {
		return
	}
	events, orders, err := h.recordDrafts(h.contract.Control.Cancel.EventDrafts)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	h.mu.Lock()
	h.cancelRecorded = true
	h.mu.Unlock()
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":             true,
		"threadId":                h.contract.CancelThreadID,
		"status":                  h.contract.Control.Cancel.Status,
		"resultCode":              h.contract.Control.Cancel.ResultCode,
		"eventKinds":              contracts.EventKinds(events),
		"publishPersistOrders":    orders,
		"allPersistBeforePublish": contracts.AllPersistBeforePublish(orders),
		"sideEffects":             h.sideEffects(),
	})
}

func (h *LiveLocalLoopHandler) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if _, ok := RequestBody(w, r); !ok {
		return
	}
	sourceState, err := h.store.RecoveredState(h.contract.ThreadID)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	events, orders, err := h.recordDrafts(h.contract.Control.Resume.EventDrafts)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	h.mu.Lock()
	h.resumeRecorded = true
	h.mu.Unlock()
	sourceKinds, _ := sourceState["eventKinds"].([]string)
	if len(sourceKinds) == 0 {
		if rawKinds, ok := sourceState["eventKinds"].([]any); ok {
			sourceKinds = make([]string, 0, len(rawKinds))
			for _, raw := range rawKinds {
				if value, ok := raw.(string); ok {
					sourceKinds = append(sourceKinds, value)
				}
			}
		}
	}
	highestSeq, _ := contracts.NumericSeq(sourceState["highestSeq"])
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":             true,
		"sourceThreadId":          h.contract.ThreadID,
		"resumedThreadId":         h.contract.ResumeThreadID,
		"sourceRecoveredState":    sourceState,
		"sourceMatchesExpected":   sameLoopStringSlice(sourceKinds, h.contract.Expected.EventKinds) && highestSeq == h.contract.Expected.HighestSeq,
		"resumeEventKinds":        contracts.EventKinds(events),
		"publishPersistOrders":    orders,
		"allPersistBeforePublish": contracts.AllPersistBeforePublish(orders),
		"sideEffects":             h.sideEffects(),
	})
}

func (h *LiveLocalLoopHandler) handleReplayJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	threadID := r.URL.Query().Get("thread_id")
	afterSeq := IntQuery(r.URL.Query(), "after_seq")
	result, err := h.store.LoadEventsSince(threadID, afterSeq)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	highestSeq, _ := h.store.HighestSeq(threadID)
	WriteJSON(w, http.StatusOK, map[string]any{
		"threadId":    threadID,
		"afterSeq":    afterSeq,
		"highestSeq":  highestSeq,
		"events":      result.Events,
		"eventKinds":  contracts.EventKinds(result.Events),
		"diagnostics": result.Diagnostics,
	})
}

func (h *LiveLocalLoopHandler) handleRecoveredState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	threadID := r.URL.Query().Get("thread_id")
	state, err := h.store.RecoveredState(threadID)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, state)
}

func (h *LiveLocalLoopHandler) handleThreadPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, LiveLocalLoopPrefix+"/threads/")
	if !strings.HasSuffix(rest, "/events") {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	threadID := strings.TrimSuffix(rest, "/events")
	sinceSeqFromQuery := IntQuery(r.URL.Query(), "since_seq")
	sinceSeqFromHeader, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
	sinceSeq := sinceSeqFromQuery
	if sinceSeq == 0 {
		sinceSeq = sinceSeqFromHeader
	}
	result, err := h.store.LoadEventsSince(threadID, sinceSeq)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteDurableSSE(w, http.StatusOK, result.Events)
}

func (h *LiveLocalLoopHandler) recordDrafts(drafts []map[string]any) ([]map[string]any, [][]string, error) {
	events := make([]map[string]any, 0, len(drafts))
	orders := make([][]string, 0, len(drafts))
	for _, draft := range drafts {
		if err := ensureFixtureThread(h.store, stringField(draft, "threadId")); err != nil {
			return nil, nil, err
		}
		event, order, err := h.store.RecordEvent(draft)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
		orders = append(orders, order)
	}
	return events, orders, nil
}

func (h *LiveLocalLoopHandler) sideEffects() map[string]int {
	return map[string]int{
		"providerCallAttempts":       0,
		"toolExecutionAttempts":      0,
		"approvalExecutionAttempts":  0,
		"mcpConnectionAttempts":      0,
		"credentialReadAttempts":     0,
		"fileMutationAttempts":       0,
		"realWorkspaceWriteAttempts": 0,
	}
}

func itemKindsFromEvents(events []map[string]any) []string {
	seen := map[string]bool{}
	kinds := []string{}
	for _, event := range events {
		item, ok := event["item"].(map[string]any)
		if !ok {
			continue
		}
		kind := contracts.StringField(item, "kind")
		if kind == "" || seen[kind] {
			continue
		}
		seen[kind] = true
		kinds = append(kinds, kind)
	}
	return kinds
}

func sameLoopStringSlice(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
