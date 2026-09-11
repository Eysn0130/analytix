//go:build !analytix_prod

package runtimego

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

type durableContractFixture struct {
	RuntimeToken         string           `json:"runtimeToken"`
	ThreadID             string           `json:"threadId"`
	ConcurrentThreadID   string           `json:"concurrentThreadId"`
	MalformedLine        string           `json:"malformedLine"`
	EventDrafts          []map[string]any `json:"eventDrafts"`
	ConcurrentEventCount int              `json:"concurrentEventCount"`
	Expected             struct {
		EventKinds               []string `json:"eventKinds"`
		HighestSeq               int      `json:"highestSeq"`
		ReplayAfterSeq           int      `json:"replayAfterSeq"`
		ReplayAfterSeqCount      int      `json:"replayAfterSeqCount"`
		CaughtUpReplayFrameCount int      `json:"caughtUpReplayFrameCount"`
		DiagnosticCount          int      `json:"diagnosticCount"`
		Providers                []string `json:"providers"`
		TotalCacheHitTokens      int      `json:"totalCacheHitTokens"`
		TotalCacheMissTokens     int      `json:"totalCacheMissTokens"`
		ApprovalStatuses         []string `json:"approvalStatuses"`
		UserInputStatuses        []string `json:"userInputStatuses"`
		MCPToolNames             []string `json:"mcpToolNames"`
		LatestMCPFingerprint     string   `json:"latestMcpFingerprint"`
		ArchiveThreadID          string   `json:"archiveThreadId"`
		ForkThreadID             string   `json:"forkThreadId"`
		ResumeThreadID           string   `json:"resumeThreadId"`
	} `json:"expected"`
}

func TestTempDurableStoreAppendReplayMalformedAndConcurrency(t *testing.T) {
	contract := loadDurableContract(t)
	root := t.TempDir()
	store, err := newTempDurableEventSessionStore(root)
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	seedDurableContractThread(t, store, contract.ThreadID)
	seedDurableContractThread(t, store, contract.ConcurrentThreadID)

	for index, draft := range contract.EventDrafts {
		event, order, err := store.RecordEvent(draft)
		if err != nil {
			t.Fatalf("record durable event %d: %v", index, err)
		}
		if !reflect.DeepEqual(order, []string{"persist", "publish"}) {
			t.Fatalf("persist-before-publish order mismatch: %#v", order)
		}
		if seq, _ := numericSeq(event["seq"]); seq != index+1 {
			t.Fatalf("event seq mismatch: got %d want %d", seq, index+1)
		}
		waitForDurableNewline(t, store, contract.ThreadID)
		if !store.NewlineTerminated(contract.ThreadID) {
			t.Fatalf("events.jsonl is not newline terminated")
		}
	}
	if highestSeq, err := store.HighestSeq(contract.ThreadID); err != nil || highestSeq != contract.Expected.HighestSeq {
		t.Fatalf("highestSeq mismatch: got %d err %v want %d", highestSeq, err, contract.Expected.HighestSeq)
	}
	if err := store.AppendRawEventLine(contract.ThreadID, contract.MalformedLine); err == nil {
		t.Fatal("malformed JSON crossed the durable append boundary")
	}
	eventsPath := filepath.Join(root, "threads", contract.ThreadID, "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open external-corruption fixture: %v", err)
	}
	if _, err := eventsFile.WriteString(contract.MalformedLine + "\n"); err != nil {
		_ = eventsFile.Close()
		t.Fatalf("inject external-corruption fixture: %v", err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatalf("close external-corruption fixture: %v", err)
	}
	result, err := store.LoadEventsSince(contract.ThreadID, 0)
	if err != nil {
		t.Fatalf("load events since: %v", err)
	}
	if len(result.Events) != len(contract.EventDrafts) {
		t.Fatalf("malformed recovery event count mismatch: got %d want %d", len(result.Events), len(contract.EventDrafts))
	}
	if len(result.Diagnostics) != contract.Expected.DiagnosticCount {
		t.Fatalf("diagnostic count mismatch: got %d want %d", len(result.Diagnostics), contract.Expected.DiagnosticCount)
	}

	var wg sync.WaitGroup
	seqs := make(chan int, contract.ConcurrentEventCount)
	for index := 0; index < contract.ConcurrentEventCount; index += 1 {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			event, _, err := store.RecordEvent(map[string]any{
				"kind":     "tool_catalog_changed",
				"threadId": contract.ConcurrentThreadID,
				"turnId":   "turn_concurrent",
				"itemId":   "item_concurrent",
			})
			if err != nil {
				t.Errorf("record concurrent event %d: %v", index, err)
				return
			}
			seq, _ := numericSeq(event["seq"])
			seqs <- seq
		}(index)
	}
	wg.Wait()
	close(seqs)
	seen := map[int]bool{}
	for seq := range seqs {
		if seen[seq] {
			t.Fatalf("duplicate concurrent seq %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != contract.ConcurrentEventCount {
		t.Fatalf("concurrent seq count mismatch: %#v", seen)
	}
	for seq := 1; seq <= contract.ConcurrentEventCount; seq += 1 {
		if !seen[seq] {
			t.Fatalf("missing concurrent seq %d in %#v", seq, seen)
		}
	}
}

func TestLiveLocalSidecarTempDurableRoutesMatchContract(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	contract := loadDurableContract(t)
	harness := NewLiveLocalSidecarHarness(LiveLocalSidecarConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
	})
	server := httptest.NewServer(harness)
	defer server.Close()

	boundary := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/durable/boundary", g1.RuntimeToken, nil, http.StatusOK)
	assertBoolField(t, boundary, "tempDurableEventSessionStoreEnabled", true)
	assertBoolField(t, boundary, "tempDirOnly", true)
	assertBoolField(t, boundary, "externalNetworkAllowed", false)
	assertBoolField(t, boundary, "credentialReadAllowed", false)
	assertBoolField(t, boundary, "realWorkspaceMutationAllowed", false)
	assertBoolField(t, boundary, "defaultGoBackendEnabled", false)

	threads := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/durable/threads", g1.RuntimeToken, nil, http.StatusOK)
	if len(arrayField(t, threads, "threads")) < 2 {
		t.Fatalf("expected seeded durable thread list, got %#v", threads)
	}
	archived := assertLiveJSON(
		t,
		server.URL,
		http.MethodPatch,
		"/v1/conformance/durable/threads/"+contract.Expected.ArchiveThreadID,
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"status": "archived"}),
		http.StatusOK,
	)
	if archived["status"] != "archived" {
		t.Fatalf("archive status mismatch: %#v", archived)
	}
	archiveSearch := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/durable/threads?include_archived=true&search=archive", g1.RuntimeToken, nil, http.StatusOK)
	if !threadListContainsID(arrayField(t, archiveSearch, "threads"), contract.Expected.ArchiveThreadID) {
		t.Fatalf("archive search did not include archived thread: %#v", archiveSearch)
	}
	fork := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/durable/threads/thr_g2_read/fork",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"relation": "side", "title": "Durable side"}),
		http.StatusCreated,
	)
	if fork["id"] != contract.Expected.ForkThreadID || fork["relation"] != "side" {
		t.Fatalf("fork mismatch: %#v", fork)
	}
	resume := assertLiveJSON(
		t,
		server.URL,
		http.MethodPost,
		"/v1/conformance/durable/sessions/thr_g2_read/resume",
		g1.RuntimeToken,
		mustJSON(t, map[string]string{"workspace": "/tmp/durable-resume", "model": "deepseek-chat"}),
		http.StatusCreated,
	)
	if resume["thread_id"] != contract.Expected.ResumeThreadID || resume["session_id"] != "thr_g2_read" {
		t.Fatalf("resume mismatch: %#v", resume)
	}

	for _, draft := range contract.EventDrafts {
		response := assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/durable/event", g1.RuntimeToken, mustJSON(t, draft), http.StatusOK)
		assertBoolField(t, response, "newlineTerminated", true)
		if !reflect.DeepEqual(stringSliceField(t, response, "publishPersistOrder"), []string{"persist", "publish"}) {
			t.Fatalf("persist-before-publish response mismatch: %#v", response)
		}
	}
	if highest := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/durable/threads/"+contract.ThreadID+"/highest-seq", g1.RuntimeToken, nil, http.StatusOK); jsonIntField(t, highest, "highestSeq") != contract.Expected.HighestSeq {
		t.Fatalf("highest seq response mismatch: %#v", highest)
	}
	assertLiveJSON(t, server.URL, http.MethodPost, "/v1/conformance/durable/events/malformed", g1.RuntimeToken, mustJSON(t, map[string]string{
		"threadId": contract.ThreadID,
		"line":     contract.MalformedLine,
	}), http.StatusBadRequest)
	replay := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/durable/events/replay?thread_id="+contract.ThreadID+"&after_seq=0", g1.RuntimeToken, nil, http.StatusOK)
	if len(arrayField(t, replay, "events")) != len(contract.EventDrafts) ||
		len(arrayField(t, replay, "diagnostics")) != 0 {
		t.Fatalf("replay recovery mismatch: %#v", replay)
	}

	queryFrames := durableSSEFrames(t, server.URL, "/v1/conformance/durable/threads/"+contract.ThreadID+"/events?since_seq=2", g1.RuntimeToken, "")
	headerFrames := durableSSEFrames(t, server.URL, "/v1/conformance/durable/threads/"+contract.ThreadID+"/events", g1.RuntimeToken, "2")
	if !reflect.DeepEqual(queryFrames, headerFrames) {
		t.Fatalf("since_seq and Last-Event-ID replay differ\nquery: %#v\nheader: %#v", queryFrames, headerFrames)
	}
	if len(queryFrames) != contract.Expected.ReplayAfterSeqCount {
		t.Fatalf("SSE replay count mismatch: got %d want %d", len(queryFrames), contract.Expected.ReplayAfterSeqCount)
	}
	caughtUpFrames := durableSSEFrames(t, server.URL, "/v1/conformance/durable/threads/"+contract.ThreadID+"/events?since_seq=8", g1.RuntimeToken, "")
	if len(caughtUpFrames) != contract.Expected.CaughtUpReplayFrameCount {
		t.Fatalf("caught-up replay count mismatch: %#v", caughtUpFrames)
	}

	state := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/conformance/durable/recovered-state?thread_id="+contract.ThreadID, g1.RuntimeToken, nil, http.StatusOK)
	if !reflect.DeepEqual(stringSliceField(t, state, "eventKinds"), contract.Expected.EventKinds) {
		t.Fatalf("event kind recovery mismatch: %#v", state)
	}
	usage := mapField(t, state, "usageCacheAccounting")
	if !reflect.DeepEqual(stringSliceField(t, usage, "providers"), contract.Expected.Providers) ||
		jsonIntField(t, usage, "totalCacheHitTokens") != contract.Expected.TotalCacheHitTokens ||
		jsonIntField(t, usage, "totalCacheMissTokens") != contract.Expected.TotalCacheMissTokens {
		t.Fatalf("usage cache recovery mismatch: %#v", usage)
	}
	approvals := mapField(t, state, "approvals")
	if !reflect.DeepEqual(stringSliceField(t, approvals, "statuses"), contract.Expected.ApprovalStatuses) {
		t.Fatalf("approval recovery mismatch: %#v", approvals)
	}
	userInputs := mapField(t, state, "userInputs")
	if !reflect.DeepEqual(stringSliceField(t, userInputs, "statuses"), contract.Expected.UserInputStatuses) {
		t.Fatalf("user-input recovery mismatch: %#v", userInputs)
	}
	mcp := mapField(t, state, "mcp")
	if mcp["latestFingerprint"] != contract.Expected.LatestMCPFingerprint ||
		!reflect.DeepEqual(stringSliceField(t, mcp, "toolNames"), contract.Expected.MCPToolNames) {
		t.Fatalf("MCP recovery mismatch: %#v", mcp)
	}

	snapshot := harness.Snapshot()
	if !snapshot.TempDurableStoreEnabled || snapshot.EventsJSONLWriteAttempts == 0 ||
		snapshot.ProviderCallAttempts != 0 || snapshot.CredentialReadAttempts != 0 ||
		snapshot.RealWorkspaceWriteAttempts != 0 {
		t.Fatalf("durable snapshot boundary mismatch: %#v", snapshot)
	}
}

func waitForDurableNewline(t *testing.T, store interface{ NewlineTerminated(string) bool }, threadID string) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if store.NewlineTerminated(threadID) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func loadDurableContract(t *testing.T) durableContractFixture {
	t.Helper()
	path := filepath.Join("..", "runtime", "src", "conformance", "fixtures", "go-durable-sidecar-contract.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read durable contract fixture: %v", err)
	}
	var fixture durableContractFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode durable contract fixture: %v", err)
	}
	return fixture
}

func seedDurableContractThread(t *testing.T, store *tempDurableEventSessionStore, threadID string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"threads": []any{map[string]any{
			"id":     threadID,
			"title":  "Durable contract fixture",
			"status": "idle",
			"turns":  []any{},
		}},
	})
	if err != nil {
		t.Fatalf("encode durable contract thread: %v", err)
	}
	if err := store.SeedFromG2Routes([]G2RouteReplayCase{{
		ID:           "thread-list-default",
		ResponseKind: "json",
		Response:     G2RouteResponse{Status: http.StatusOK, Body: body},
	}}); err != nil {
		t.Fatalf("seed durable contract thread %s: %v", threadID, err)
	}
}

func durableSSEFrames(t *testing.T, serverURL string, path string, token string, lastEventID string) []string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, serverURL+path, nil)
	if err != nil {
		t.Fatalf("build durable SSE request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("run durable SSE request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("durable SSE status mismatch: got %d want %d", response.StatusCode, http.StatusOK)
	}
	data := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, readErr := response.Body.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	return splitSSEFrames(string(data))
}

func arrayField(t *testing.T, record map[string]any, field string) []any {
	t.Helper()
	values, ok := record[field].([]any)
	if !ok {
		t.Fatalf("field %q is not an array: %#v", field, record[field])
	}
	return values
}

func threadListContainsID(threads []any, id string) bool {
	ids := []string{}
	for _, value := range threads {
		thread, _ := value.(map[string]any)
		if threadID, _ := thread["id"].(string); threadID != "" {
			ids = append(ids, threadID)
		}
	}
	sort.Strings(ids)
	return containsString(ids, id)
}
