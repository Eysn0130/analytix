//go:build !analytix_prod

package livelocal

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type LiveLocalSidecarSnapshot struct {
	IsolatedInMemoryStoreOnly  bool           `json:"isolatedInMemoryStoreOnly"`
	MutatedRouteIDs            []string       `json:"mutatedRouteIds"`
	ThreadIDs                  []string       `json:"threadIds"`
	ArchivedThreadIDs          []string       `json:"archivedThreadIds"`
	ForkThreadIDs              []string       `json:"forkThreadIds"`
	ResumedSessionIDs          []string       `json:"resumedSessionIds"`
	ProviderCallAttempts       int            `json:"providerCallAttempts"`
	StubProviderUsageReplays   int            `json:"stubProviderUsageReplays"`
	StubProviderShapeReplays   int            `json:"stubProviderShapeReplays"`
	StubProviderStreamReplays  int            `json:"stubProviderStreamReplays"`
	StubProviderCacheReplays   int            `json:"stubProviderCacheReplays"`
	StubG4ApprovalReplays      int            `json:"stubG4ApprovalReplays"`
	StubG4UserInputReplays     int            `json:"stubG4UserInputReplays"`
	StubG4MCPReplays           int            `json:"stubG4MCPReplays"`
	StubG4ValidationReplays    int            `json:"stubG4ValidationReplays"`
	StubKernelScaffoldReplays  int            `json:"stubKernelScaffoldReplays"`
	TempDurableStoreEnabled    bool           `json:"tempDurableStoreEnabled"`
	ApprovalExecutionAttempts  int            `json:"approvalExecutionAttempts"`
	ToolExecutionAttempts      int            `json:"toolExecutionAttempts"`
	MCPConnectionAttempts      int            `json:"mcpConnectionAttempts"`
	MCPCredentialAttempts      int            `json:"mcpCredentialAttempts"`
	CredentialReadAttempts     int            `json:"credentialReadAttempts"`
	FileMutationAttempts       int            `json:"fileMutationAttempts"`
	EventsJSONLWriteAttempts   int            `json:"eventsJsonlWriteAttempts"`
	RealWorkspaceWriteAttempts int            `json:"realWorkspaceWriteAttempts"`
	ProductionCandidate        map[string]any `json:"productionCandidate,omitempty"`
}

type LiveLocalG2MutableHandler struct {
	runtimeToken string
	routes       map[string]G2RouteReplayCase
	store        *LiveLocalG2Store
}

type liveLocalG3ProviderCounters struct {
	usageReplays  int
	shapeReplays  int
	streamReplays int
	cacheReplays  int
}

type liveLocalG4ManagerCounters struct {
	approvalReplays   int
	userInputReplays  int
	mcpReplays        int
	validationReplays int
}

type LiveLocalG2Store struct {
	threads           map[string]map[string]any
	listedThreadIDs   map[string]bool
	mutatedRouteIDs   []string
	forkThreadIDs     []string
	resumedSessionIDs []string
	stubG3            liveLocalG3ProviderCounters
	stubG4            liveLocalG4ManagerCounters
}

func NewLiveLocalG2MutableHandler(runtimeToken string, routes []G2RouteReplayCase) *LiveLocalG2MutableHandler {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = DefaultRuntimeToken
	}
	routeMap := make(map[string]G2RouteReplayCase, len(routes))
	for _, route := range routes {
		routeMap[RouteKey(route.Method, route.Path, route.Body)] = route
	}
	return &LiveLocalG2MutableHandler{
		runtimeToken: runtimeToken,
		routes:       routeMap,
		store:        NewLiveLocalG2Store(routes),
	}
}

func (h *LiveLocalG2MutableHandler) Store() *LiveLocalG2Store {
	if h == nil {
		return nil
	}
	return h.store
}

func (h *LiveLocalG2MutableHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !Authorized(r, h.runtimeToken) {
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	body, ok := RequestBody(w, r)
	if !ok {
		return
	}

	if r.Method == http.MethodGet {
		if data, ok := h.store.readRoute(r.URL.RequestURI()); ok {
			WriteRawJSON(w, http.StatusOK, data)
			return
		}
	}

	route, ok := h.routes[RouteKey(r.Method, r.URL.RequestURI(), body)]
	if !ok {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
		return
	}
	if route.ResponseKind == "sse" {
		WriteSSE(w, route.Response.Status, route.SSEFrames)
		return
	}
	if IsMutatingG2Route(route) {
		h.store.applyMutation(route)
	}
	WriteRawJSON(w, route.Response.Status, route.Response.Body)
}

func NewLiveLocalG2Store(routes []G2RouteReplayCase) *LiveLocalG2Store {
	store := &LiveLocalG2Store{
		threads:         make(map[string]map[string]any),
		listedThreadIDs: make(map[string]bool),
	}
	for _, route := range routes {
		if route.ResponseKind != "json" || len(route.Response.Body) == 0 || IsMutatingG2Route(route) {
			continue
		}
		switch route.ID {
		case "thread-list-default":
			store.seedThreadList(route.Response.Body)
		case "thread-read-detail":
			store.upsertThread(route.Response.Body)
		}
	}
	return store
}

func MutatingG2Routes(routes []G2RouteReplayCase) []G2RouteReplayCase {
	out := make([]G2RouteReplayCase, 0, len(routes))
	for _, route := range routes {
		if IsMutatingG2Route(route) {
			out = append(out, route)
		}
	}
	return out
}

func IsMutatingG2Route(route G2RouteReplayCase) bool {
	return route.Method == http.MethodPatch || route.Method == http.MethodPost
}

func (s *LiveLocalG2Store) seedThreadList(body json.RawMessage) {
	var list struct {
		Threads []map[string]any `json:"threads"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return
	}
	for _, summary := range list.Threads {
		id, _ := summary["id"].(string)
		if id == "" {
			continue
		}
		s.threads[id] = cloneMap(summary)
		s.listedThreadIDs[id] = true
	}
}

func (s *LiveLocalG2Store) upsertThread(body json.RawMessage) {
	var thread map[string]any
	if err := json.Unmarshal(body, &thread); err != nil {
		return
	}
	id, _ := thread["id"].(string)
	if id == "" {
		return
	}
	if existing, ok := s.threads[id]; ok {
		if _, hasLatestSeq := thread["latestSeq"]; !hasLatestSeq {
			if latestSeq, ok := existing["latestSeq"]; ok {
				thread["latestSeq"] = latestSeq
			}
		}
	}
	s.threads[id] = cloneMap(thread)
}

func (s *LiveLocalG2Store) applyMutation(route G2RouteReplayCase) {
	s.mutatedRouteIDs = append(s.mutatedRouteIDs, route.ID)
	switch route.ID {
	case "thread-archive-patch", "thread-update-title-workspace":
		s.upsertThread(route.Response.Body)
	case "thread-fork-side":
		s.upsertThread(route.Response.Body)
		if id := responseID(route.Response.Body); id != "" {
			s.forkThreadIDs = append(s.forkThreadIDs, id)
		}
	case "session-resume-thread":
		var body map[string]any
		if err := json.Unmarshal(route.Response.Body, &body); err == nil {
			if id, _ := body["session_id"].(string); id != "" {
				s.resumedSessionIDs = append(s.resumedSessionIDs, id)
			}
		}
	}
}

func (s *LiveLocalG2Store) readRoute(requestURI string) (json.RawMessage, bool) {
	if requestURI == "/v1/threads" {
		return s.threadList(false, false, ""), true
	}
	if strings.HasPrefix(requestURI, "/v1/threads?") {
		query, err := url.ParseQuery(strings.TrimPrefix(requestURI, "/v1/threads?"))
		if err != nil {
			return nil, false
		}
		return s.threadList(
			query.Get("archived_only") == "true",
			query.Get("include_archived") == "true",
			query.Get("search"),
		), true
	}
	if strings.HasPrefix(requestURI, "/v1/threads/") && !strings.Contains(requestURI, "/events") {
		id := strings.TrimPrefix(requestURI, "/v1/threads/")
		if strings.Contains(id, "/") || strings.Contains(id, "?") {
			return nil, false
		}
		thread, ok := s.threads[id]
		if !ok {
			return nil, false
		}
		data, err := json.Marshal(thread)
		if err != nil {
			return nil, false
		}
		return data, true
	}
	return nil, false
}

func (s *LiveLocalG2Store) threadList(archivedOnly bool, includeArchived bool, search string) json.RawMessage {
	needle := strings.ToLower(strings.TrimSpace(search))
	threads := make([]map[string]any, 0, len(s.threads))
	for _, thread := range s.threads {
		id := stringField(thread, "id")
		if !s.listedThreadIDs[id] {
			continue
		}
		if stringField(thread, "relation") != "primary" {
			continue
		}
		status := stringField(thread, "status")
		if archivedOnly {
			if status != "archived" {
				continue
			}
		} else if !includeArchived && status == "archived" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(stringField(thread, "title")), needle) {
			continue
		}
		threads = append(threads, threadSummary(thread))
	}
	sort.Slice(threads, func(i, j int) bool {
		left := stringField(threads[i], "updatedAt")
		right := stringField(threads[j], "updatedAt")
		if left == right {
			return stringField(threads[i], "id") > stringField(threads[j], "id")
		}
		return left > right
	})
	data, err := json.Marshal(map[string]any{"threads": threads})
	if err != nil {
		return json.RawMessage(`{"threads":[]}`)
	}
	return data
}

func (s *LiveLocalG2Store) Snapshot() LiveLocalSidecarSnapshot {
	threadIDs := make([]string, 0, len(s.threads))
	archivedThreadIDs := []string{}
	for id, thread := range s.threads {
		threadIDs = append(threadIDs, id)
		if stringField(thread, "status") == "archived" {
			archivedThreadIDs = append(archivedThreadIDs, id)
		}
	}
	sort.Strings(threadIDs)
	sort.Strings(archivedThreadIDs)
	forkThreadIDs := append([]string(nil), s.forkThreadIDs...)
	resumedSessionIDs := append([]string(nil), s.resumedSessionIDs...)
	sort.Strings(forkThreadIDs)
	sort.Strings(resumedSessionIDs)
	return LiveLocalSidecarSnapshot{
		IsolatedInMemoryStoreOnly:  true,
		MutatedRouteIDs:            append([]string(nil), s.mutatedRouteIDs...),
		ThreadIDs:                  threadIDs,
		ArchivedThreadIDs:          archivedThreadIDs,
		ForkThreadIDs:              forkThreadIDs,
		ResumedSessionIDs:          resumedSessionIDs,
		ProviderCallAttempts:       0,
		StubProviderUsageReplays:   s.stubG3.usageReplays,
		StubProviderShapeReplays:   s.stubG3.shapeReplays,
		StubProviderStreamReplays:  s.stubG3.streamReplays,
		StubProviderCacheReplays:   s.stubG3.cacheReplays,
		StubG4ApprovalReplays:      s.stubG4.approvalReplays,
		StubG4UserInputReplays:     s.stubG4.userInputReplays,
		StubG4MCPReplays:           s.stubG4.mcpReplays,
		StubG4ValidationReplays:    s.stubG4.validationReplays,
		TempDurableStoreEnabled:    false,
		ApprovalExecutionAttempts:  0,
		ToolExecutionAttempts:      0,
		MCPConnectionAttempts:      0,
		MCPCredentialAttempts:      0,
		CredentialReadAttempts:     0,
		FileMutationAttempts:       0,
		EventsJSONLWriteAttempts:   0,
		RealWorkspaceWriteAttempts: 0,
	}
}

func (s *LiveLocalG2Store) NoteG3UsageReplay() {
	s.stubG3.usageReplays += 1
}

func (s *LiveLocalG2Store) NoteG3ShapeReplay() {
	s.stubG3.shapeReplays += 1
}

func (s *LiveLocalG2Store) NoteG3StreamReplay() {
	s.stubG3.streamReplays += 1
}

func (s *LiveLocalG2Store) NoteG3CacheReplay() {
	s.stubG3.cacheReplays += 1
}

func (s *LiveLocalG2Store) NoteG4ApprovalReplay() {
	s.stubG4.approvalReplays += 1
}

func (s *LiveLocalG2Store) NoteG4UserInputReplay() {
	s.stubG4.userInputReplays += 1
}

func (s *LiveLocalG2Store) NoteG4MCPReplay() {
	s.stubG4.mcpReplays += 1
}

func (s *LiveLocalG2Store) NoteG4ValidationReplay() {
	s.stubG4.validationReplays += 1
}

func responseID(body json.RawMessage) string {
	return contracts.ResponseID(body)
}
