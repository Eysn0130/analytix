package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	controlapp "analytix.local/runtime-go/internal/app/control"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func requestArchiveIntentJSON(t *testing.T, baseURL, method, path string, body io.Reader, status int) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("bounded archive request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("archive HTTP status = %d, want %d", response.StatusCode, status)
	}
	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func newArchiveIntentHTTPFixture(t *testing.T) (*runtimeServerHandler, *httptest.Server, string) {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
	}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	workspace := workspacetest.New(t)
	body, err := json.Marshal(map[string]any{"title": "archive fixture", "workspace": workspace})
	if err != nil {
		t.Fatal(err)
	}
	created := requestArchiveIntentJSON(t, server.URL, http.MethodPost, "/v1/threads", bytes.NewReader(body), http.StatusCreated)
	threadID := stringField(created, "id")
	if threadID == "" || stringField(created, "status") != "idle" {
		t.Fatal("ordinary HTTP thread fixture was not created idle")
	}
	return handler, server, threadID
}

func TestArchiveIntentHTTPPersistsListsAndRestores(t *testing.T) {
	handler, server, threadID := newArchiveIntentHTTPFixture(t)
	path := "/v1/threads/" + threadID
	archived := requestArchiveIntentJSON(t, server.URL, http.MethodPatch, path, strings.NewReader(`{"archived":true}`), http.StatusOK)
	if stringField(archived, "status") != "archived" {
		t.Fatal("archive intent did not reach the durable runtime status")
	}
	listed := requestArchiveIntentJSON(t, server.URL, http.MethodGet, "/v1/threads?archived_only=true&limit=0", nil, http.StatusOK)
	threads := listAny(listed["threads"])
	if len(threads) != 1 {
		t.Fatalf("archive list count = %d, want 1", len(threads))
	}
	summary, _ := threads[0].(map[string]any)
	if stringField(summary, "id") != threadID || summary["archived"] != true {
		t.Fatal("archive list did not project the archived thread")
	}
	ordinary := requestArchiveIntentJSON(t, server.URL, http.MethodGet, "/v1/threads?limit=0", nil, http.StatusOK)
	if len(listAny(ordinary["threads"])) != 0 {
		t.Fatal("archived thread remained in the ordinary thread list")
	}
	reopened, err := NewTempDurableEventSessionStore(handler.store.root)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reopened.GetThread(threadID)
	if err != nil || stringField(persisted, "status") != "archived" {
		t.Fatalf("archive did not survive a fresh durable reader: %v", err)
	}
	if _, present := persisted["archived"]; present {
		t.Fatal("archive intent introduced a second persisted status authority")
	}
	restored := requestArchiveIntentJSON(t, server.URL, http.MethodPatch, path, strings.NewReader(`{"archived":false}`), http.StatusOK)
	if stringField(restored, "status") != "idle" || stringField(restored, "title") != "archive fixture" {
		t.Fatal("restore did not preserve the ordinary thread")
	}
	listed = requestArchiveIntentJSON(t, server.URL, http.MethodGet, "/v1/threads?archived_only=true&limit=0", nil, http.StatusOK)
	if len(listAny(listed["threads"])) != 0 {
		t.Fatal("restored thread remained in the archive list")
	}
	ordinary = requestArchiveIntentJSON(t, server.URL, http.MethodGet, "/v1/threads?limit=0", nil, http.StatusOK)
	if len(listAny(ordinary["threads"])) != 1 {
		t.Fatal("restored thread did not return to the ordinary list")
	}
	persisted, err = reopened.GetThread(threadID)
	if err != nil || stringField(persisted, "status") != "idle" {
		t.Fatalf("restore was not persisted: %v", err)
	}
}

func TestArchiveIntentHTTPRejectsMalformedAndPublicStatusWrites(t *testing.T) {
	handler, server, threadID := newArchiveIntentHTTPFixture(t)
	_, before, err := handler.store.ReadThreadMutationBaseline(threadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"archived":null}`, `{"archived":"true"}`, `{"archived":1}`,
		`{"archived":true,"title":"changed"}`, `{"archived":false,"workspace":"/other"}`,
		`{"archived":true,"unknown":true}`, `{"archived":true,"status":"archived"}`,
		`{"status":"idle"}`, `{"status":"archived"}`, `{"status":"running"}`, `{"status":"deleted"}`,
	} {
		result := requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+threadID, strings.NewReader(body), http.StatusBadRequest)
		if result["code"] != "validation_error" {
			t.Fatal("invalid archive patch did not fail at the public validation boundary")
		}
	}
	_, after, err := handler.store.ReadThreadMutationBaseline(threadID)
	if err != nil || after != before {
		t.Fatalf("rejected public patches changed the durable thread: %v", err)
	}
}

func TestArchiveIntentHTTPRejectsBusyOwnersWithoutCancellation(t *testing.T) {
	for _, owner := range []string{"preparing", "registered-turn", "steer"} {
		t.Run(owner, func(t *testing.T) {
			handler, server, threadID := newArchiveIntentHTTPFixture(t)
			control := handler.runtimeControl()
			var cancelled atomic.Bool
			var release func()
			var err error
			switch owner {
			case "preparing":
				_, release, err = control.PrepareForegroundWithWitness(context.Background(), threadID)
			case "registered-turn":
				err = control.RegisterTurnCancelWithError(threadID, "turn_archive_busy", func() { cancelled.Store(true) })
				release = func() { control.UnregisterTurnCancel(threadID, "turn_archive_busy") }
			case "steer":
				release, err = control.ReserveSteerAdmission(threadID, "turn_archive_busy")
			}
			if err != nil || release == nil {
				t.Fatalf("register existing owner: %v", err)
			}
			defer release()
			_, before, err := handler.store.ReadThreadMutationBaseline(threadID)
			if err != nil {
				t.Fatal(err)
			}
			requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+threadID, strings.NewReader(`{"archived":true}`), http.StatusConflict)
			_, after, err := handler.store.ReadThreadMutationBaseline(threadID)
			if err != nil || before != after || cancelled.Load() {
				t.Fatalf("archive mutated or cancelled existing work: changed=%t cancelled=%t err=%v", before != after, cancelled.Load(), err)
			}
			release()
			result := requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+threadID, strings.NewReader(`{"archived":true}`), http.StatusOK)
			if stringField(result, "status") != "archived" {
				t.Fatal("rejected archive leaked its admission reservation")
			}
		})
	}
}

func TestArchiveIntentHTTPRejectsDurableActiveQueuedAndDeletedState(t *testing.T) {
	for _, state := range []string{"running-thread", "running-turn", "queued-turn", "deleted"} {
		t.Run(state, func(t *testing.T) {
			handler, server, threadID := newArchiveIntentHTTPFixture(t)
			status := "idle"
			switch state {
			case "running-thread":
				status = "running"
			case "deleted":
				status = "deleted"
			default:
				turnStatus := strings.TrimSuffix(state, "-turn")
				if err := handler.store.AppendTurnToThread(threadID, map[string]any{
					"id": "turn_archive_durable", "threadId": threadID, "status": turnStatus, "items": []any{},
				}, "", nil); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := handler.store.PatchThread(threadID, map[string]any{"status": status}); err != nil {
				t.Fatal(err)
			}
			_, before, err := handler.store.ReadThreadMutationBaseline(threadID)
			if err != nil {
				t.Fatal(err)
			}
			for _, body := range []string{`{"archived":true}`, `{"archived":false}`} {
				requestArchiveIntentJSON(t, server.URL, http.MethodPatch, "/v1/threads/"+threadID, strings.NewReader(body), http.StatusConflict)
			}
			_, after, err := handler.store.ReadThreadMutationBaseline(threadID)
			if err != nil || before != after {
				t.Fatalf("archive changed non-idle durable state: %v", err)
			}
		})
	}
}

type archiveIntentReadBarrier struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (barrier *archiveIntentReadBarrier) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	barrier.once.Do(func() {
		close(barrier.entered)
		<-barrier.release
	})
	return (filestore.CaseBindingReader{}).Observe(workspace)
}

func TestArchiveIntentHTTPFencesAdmissionAndRechecksEveryCAS(t *testing.T) {
	for _, change := range []string{"metadata", "queued-turn"} {
		t.Run(change, func(t *testing.T) {
			handler, _, threadID := newArchiveIntentHTTPFixture(t)
			barrier := &archiveIntentReadBarrier{entered: make(chan struct{}), release: make(chan struct{})}
			handler.turnSecurity.Observer = barrier
			handler.threads = nil
			request := httptest.NewRequest(http.MethodPatch, "/v1/threads/"+threadID, strings.NewReader(`{"archived":true}`))
			request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
			request.Header.Set("Content-Type", "application/json")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request = request.WithContext(ctx)
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				handler.ServeHTTP(response, request)
				close(done)
			}()
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
			t.Cleanup(func() {
				release()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("archive request did not release its metadata owner")
				}
			})
			select {
			case <-barrier.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("archive request did not reach its post-read validation boundary")
			}
			var cancelled atomic.Bool
			err := handler.runtimeControl().RegisterTurnCancelWithError(threadID, "turn_archive_race", func() { cancelled.Store(true) })
			if err == nil {
				handler.runtimeControl().UnregisterTurnCancel(threadID, "turn_archive_race")
				t.Fatal("new turn registered between archive read and durable CAS")
			}
			if !errors.Is(err, controlapp.ErrTurnExecutionConflict) || cancelled.Load() {
				t.Fatalf("archive admission fence cancelled a competing turn: %v", err)
			}
			if change == "queued-turn" {
				if err := handler.store.AppendTurnToThread(threadID, map[string]any{
					"id": "turn_archive_race", "threadId": threadID, "status": "queued", "items": []any{},
				}, "", nil); err != nil {
					t.Fatal(err)
				}
				if _, err := handler.store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
					t.Fatal(err)
				}
			} else if _, err := handler.store.PatchThread(threadID, map[string]any{"title": "concurrent metadata"}); err != nil {
				t.Fatal(err)
			}
			_, changedDigest, err := handler.store.ReadThreadMutationBaseline(threadID)
			if err != nil {
				t.Fatal(err)
			}
			release()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("archive CAS did not finish")
			}
			current, currentDigest, err := handler.store.ReadThreadMutationBaseline(threadID)
			if err != nil {
				t.Fatal(err)
			}
			if change == "queued-turn" {
				if response.Code != http.StatusConflict || currentDigest != changedDigest {
					t.Fatalf("archive retry bypassed the fresh queued-turn guard: status=%d", response.Code)
				}
			} else if response.Code != http.StatusOK || stringField(current, "status") != "archived" || stringField(current, "title") != "concurrent metadata" {
				t.Fatalf("archive did not retry its stale metadata CAS: status=%d", response.Code)
			}
			if !handler.runtimeControl().GoalIdleV1(threadID) {
				t.Fatal("completed archive request retained its Controller reservation")
			}
			if _, err := handler.store.PatchThreadIfBaseline(threadID, map[string]any{"archived": true}, "stale-baseline"); !errors.Is(err, threadapp.ErrThreadMutationBaselineConflict) {
				t.Fatalf("archive durable writer accepted a stale baseline: %v", err)
			}
		})
	}
}
