//go:build !analytix_prod

package livelocal

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

const LiveLocalDurablePrefix = "/v1/conformance/durable"

type LiveLocalDurableHandler struct {
	runtimeToken string
	store        *DurableEventSessionStore
}

func NewLiveLocalDurableHandler(runtimeToken string, tempDir string, routes []G2RouteReplayCase) (*LiveLocalDurableHandler, error) {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = DefaultRuntimeToken
	}
	store, err := NewTempDurableEventSessionStore(tempDir)
	if err != nil {
		return nil, err
	}
	if err := store.SeedFromG2Routes(routes); err != nil {
		return nil, err
	}
	return &LiveLocalDurableHandler{
		runtimeToken: runtimeToken,
		store:        store,
	}, nil
}

func (h *LiveLocalDurableHandler) Handles(path string) bool {
	return h != nil && strings.HasPrefix(path, LiveLocalDurablePrefix)
}

func (h *LiveLocalDurableHandler) Store() *DurableEventSessionStore {
	if h == nil {
		return nil
	}
	return h.store
}

func (h *LiveLocalDurableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !Authorized(r, h.runtimeToken) {
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}

	switch {
	case r.URL.Path == LiveLocalDurablePrefix+"/boundary":
		h.handleBoundary(w, r)
	case r.URL.Path == LiveLocalDurablePrefix+"/snapshot":
		h.handleSnapshot(w, r)
	case r.URL.Path == LiveLocalDurablePrefix+"/event":
		h.handleRecordEvent(w, r)
	case r.URL.Path == LiveLocalDurablePrefix+"/events/malformed":
		h.handleAppendMalformedEvent(w, r)
	case r.URL.Path == LiveLocalDurablePrefix+"/events/replay":
		h.handleReplayJSON(w, r)
	case r.URL.Path == LiveLocalDurablePrefix+"/recovered-state":
		h.handleRecoveredState(w, r)
	case r.URL.Path == LiveLocalDurablePrefix+"/threads":
		h.handleThreads(w, r)
	case strings.HasPrefix(r.URL.Path, LiveLocalDurablePrefix+"/threads/"):
		h.handleThreadPath(w, r)
	case strings.HasPrefix(r.URL.Path, LiveLocalDurablePrefix+"/sessions/"):
		h.handleSessionPath(w, r)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h *LiveLocalDurableHandler) handleBoundary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                         true,
		"testConformanceOnly":                 true,
		"tempDurableEventSessionStoreEnabled": true,
		"tempDirOnly":                         true,
		"eventsJsonlAppendNewlineTerminated":  true,
		"persistentSeqHighWater":              true,
		"malformedJsonlDiagnostics":           true,
		"externalNetworkAllowed":              false,
		"providerCredentialsAllowed":          false,
		"apiKeyReadAllowed":                   false,
		"providerLiveCallsAllowed":            false,
		"toolExecutionAllowed":                false,
		"approvalExecutionAllowed":            false,
		"mcpConnectionAllowed":                false,
		"mcpCredentialsAllowed":               false,
		"credentialReadAllowed":               false,
		"realWorkspaceMutationAllowed":        false,
		"electronMainConnected":               false,
		"defaultGoBackendEnabled":             false,
		"rendererVisibleGoRoutesAllowed":      false,
		"reasonixPublicProtocolAllowed":       false,
		"productBoundary":                     contracts.LiveLocalSidecarDurableProductBoundary(),
	})
}

func (h *LiveLocalDurableHandler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	WriteJSON(w, http.StatusOK, h.store.Snapshot())
}

func (h *LiveLocalDurableHandler) handleRecordEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	var draft map[string]any
	if err := json.Unmarshal(body, &draft); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid event body"})
		return
	}
	if err := ensureFixtureThread(h.store, stringField(draft, "threadId")); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	event, order, err := h.store.RecordEvent(draft)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	threadID := stringField(event, "threadId")
	highestSeq, _ := h.store.HighestSeq(threadID)
	WriteJSON(w, http.StatusOK, map[string]any{
		"event":               event,
		"publishPersistOrder": order,
		"newlineTerminated":   h.store.NewlineTerminated(threadID),
		"highestSeq":          highestSeq,
	})
}

func (h *LiveLocalDurableHandler) handleAppendMalformedEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	var request struct {
		ThreadID string `json:"threadId"`
		Line     string `json:"line"`
	}
	if err := json.Unmarshal(body, &request); err != nil || strings.TrimSpace(request.ThreadID) == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "threadId and line are required"})
		return
	}
	if err := h.store.AppendRawEventLine(request.ThreadID, request.Line); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"threadId":          request.ThreadID,
		"newlineTerminated": h.store.NewlineTerminated(request.ThreadID),
	})
}

func (h *LiveLocalDurableHandler) handleReplayJSON(w http.ResponseWriter, r *http.Request) {
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
		"diagnostics": result.Diagnostics,
	})
}

func (h *LiveLocalDurableHandler) handleRecoveredState(w http.ResponseWriter, r *http.Request) {
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

func (h *LiveLocalDurableHandler) handleThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	threads, err := h.store.ListThreads(
		r.URL.Query().Get("archived_only") == "true",
		r.URL.Query().Get("include_archived") == "true",
		threadListIncludesSide(r.URL.Query().Get("include")),
		r.URL.Query().Get("search"),
	)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

func (h *LiveLocalDurableHandler) handleThreadPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, LiveLocalDurablePrefix+"/threads/")
	if strings.HasSuffix(rest, "/events") {
		threadID := strings.TrimSuffix(rest, "/events")
		h.handleThreadEventsSSE(w, r, threadID)
		return
	}
	if strings.HasSuffix(rest, "/fork") {
		threadID := strings.TrimSuffix(rest, "/fork")
		h.handleThreadFork(w, r, threadID)
		return
	}
	if strings.HasSuffix(rest, "/highest-seq") {
		threadID := strings.TrimSuffix(rest, "/highest-seq")
		highestSeq, err := h.store.HighestSeq(threadID)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"threadId": threadID, "highestSeq": highestSeq})
		return
	}
	threadID := rest
	switch r.Method {
	case http.MethodGet:
		thread, err := h.store.GetThread(threadID)
		if err != nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
			return
		}
		if thread == nil {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		highestSeq, _ := h.store.HighestSeq(threadID)
		thread["latestSeq"] = float64(highestSeq)
		WriteJSON(w, http.StatusOK, thread)
	case http.MethodPatch:
		body, ok := RequestBody(w, r)
		if !ok {
			return
		}
		var patch map[string]any
		if err := json.Unmarshal(body, &patch); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid thread patch"})
			return
		}
		thread, err := h.store.PatchThread(threadID, patch)
		if errors.Is(err, os.ErrNotExist) {
			WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
			return
		}
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		WriteJSON(w, http.StatusOK, thread)
	default:
		MethodNotAllowed(w)
	}
}

func (h *LiveLocalDurableHandler) handleThreadFork(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	request := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &request); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid fork body"})
			return
		}
	}
	thread, err := h.store.ForkThread(threadID, request)
	if errors.Is(err, os.ErrNotExist) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "thread not found"})
		return
	}
	if errors.Is(err, errDurableTurnNotFound) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": err.Error()})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, thread)
}

func (h *LiveLocalDurableHandler) handleThreadEventsSSE(w http.ResponseWriter, r *http.Request, threadID string) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	sinceSeqFromQuery := IntQuery(r.URL.Query(), "since_seq")
	sinceSeqFromHeader, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
	sinceSeq := sinceSeqFromQuery
	if sinceSeq == 0 {
		sinceSeq = sinceSeqFromHeader
	}
	highestSeq, _ := h.store.HighestSeq(threadID)
	result := DurableLoadEventsResult{Events: []map[string]any{}, Diagnostics: []DurableJSONLDiagnostic{}}
	if sinceSeq < highestSeq {
		loaded, err := h.store.LoadEventsSince(threadID, sinceSeq)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
			return
		}
		result = loaded
	}
	WriteDurableSSE(w, http.StatusOK, result.Events)
}

func (h *LiveLocalDurableHandler) handleSessionPath(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, LiveLocalDurablePrefix+"/sessions/")
	if !strings.HasSuffix(rest, "/resume") {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	sessionID := strings.TrimSuffix(rest, "/resume")
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}
	request := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &request); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid resume body"})
			return
		}
	}
	response, err := h.store.ResumeSession(sessionID, request)
	if errors.Is(err, os.ErrNotExist) {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "session not found"})
		return
	}
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, response)
}
