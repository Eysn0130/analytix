package server

import (
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func goalHTTPV1(t *testing.T, h *runtimeServerHandler, threadID, method string, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/v1/threads/"+threadID+"/goal", bytes.NewReader(raw))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	var value map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal("Goal HTTP returned malformed JSON", err)
	}
	return response.Code, value
}

func TestGoalActivationFullSubscriberQueueDisconnectsForFreshAttach(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := store.SubscribeEvents("goal-thread")
	defer unsubscribe()
	store.mu.Lock()
	for channel := range store.subscribers["goal-thread"] {
		for len(channel) < cap(channel) {
			channel <- map[string]any{"kind": "synthetic-buffer-entry"}
		}
	}
	store.mu.Unlock()
	store.publishGoalActivationV1("goal-thread", nil)
	for range events {
	}
	store.mu.Lock()
	remaining := len(store.subscribers["goal-thread"])
	store.mu.Unlock()
	if remaining != 0 {
		t.Fatal("full metadata subscriber stayed attached after losing stop signal")
	}
}

// Exercises the real HTTP lifecycle, normal controller/provider/security/start
// path, primary source+counter admission and terminal wake. The Provider is
// deterministic and local; no credentials, external requests or user state.
func TestGoalHTTPAutomaticRoundsUseRealTurnItemsAndStopAtCumulativeBudget(t *testing.T) {
	handler := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "goal-fixture", BaseURL: "https://provider.invalid", APIKey: "fixture-key", Model: "goal-fixture-model", EndpointFormat: "chat_completions"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, handler)
	fixtureProvider := &countingImmediateWorkspaceProvider{}
	handler.provider = fixtureProvider
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := handler.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{"title": "Goal production fixture", "providerId": "goal-fixture", "model": "goal-fixture-model", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	httpServer := httptest.NewServer(handler)
	streamContext, cancelStream := context.WithTimeout(context.Background(), 12*time.Second)
	defer func() { cancelStream(); httpServer.Close() }()
	streamRequest, err := http.NewRequestWithContext(streamContext, http.MethodGet, httpServer.URL+"/v1/threads/"+threadID+"/events?live=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	stream, err := httpServer.Client().Do(streamRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatal("Goal SSE did not connect", stream.StatusCode)
	}
	scanner := bufio.NewScanner(stream.Body)
	if !scanner.Scan() || scanner.Text() != ": connected" {
		t.Fatal("Goal SSE subscription not established")
	}
	events := make(chan map[string]any, 64)
	streamErrors := make(chan error, 1)
	go func() {
		eventID := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "id:") {
				eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
				continue
			}
			if line == "" {
				eventID = ""
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
				streamErrors <- err
				return
			}
			if stringField(event, "kind") == "goal_activation_changed" && eventID != "" {
				streamErrors <- fmt.Errorf("Goal metadata acquired SSE id")
				return
			}
			select {
			case events <- event:
			case <-streamContext.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && streamContext.Err() == nil {
			streamErrors <- err
		}
	}()
	code, created := goalHTTPV1(t, handler, threadID, http.MethodPost, map[string]any{"operation": "create", "objective": "Verify the local fixture", "maxGoalRounds": 2})
	if code != http.StatusOK {
		t.Fatalf("Goal create failed: %d %#v", code, created)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-events:
			if stringField(event, "kind") != "goal_activation_changed" {
				continue
			}
			if _, present := event["seq"]; present {
				t.Fatal("activation acquired durable seq")
			}
			code, state := goalHTTPV1(t, handler, threadID, http.MethodGet, nil)
			if code != http.StatusOK {
				t.Fatal("Goal read failed", code)
			}
			goal, _ := state["goal"].(map[string]any)
			if stringField(goal, "status") != "blocked" {
				continue
			}
			_, maximum, started, known := domaingoal.CountedRecordV1(goal)
			if !known || maximum != 2 || started != 2 || fixtureProvider.calls.Load() != 2 {
				t.Fatal("budget/calls mismatch", goal, fixtureProvider.calls.Load())
			}
			activation, _ := state["activation"].(map[string]any)
			if stringField(activation, "activation") != "disarmed" || activation["revision"] != goal["revision"] {
				t.Fatal("budget activation not current", state)
			}
			actual, err := handler.store.GetThread(threadID)
			if err != nil {
				t.Fatal(err)
			}
			turns := listAny(actual["turns"])
			if len(turns) != 2 {
				t.Fatal("wrong real turn count", len(turns))
			}
			for index, rawTurn := range turns {
				turn := rawTurn.(map[string]any)
				if stringField(turn, "status") != "completed" {
					t.Fatal("Goal round did not terminally complete", turn)
				}
				found := 0
				for _, rawItem := range listAny(turn["items"]) {
					item := rawItem.(map[string]any)
					if stringField(item, "kind") != "user_message" {
						continue
					}
					source, valid := domaingoal.ParseRoundSourceV1(item["source"])
					if !valid || source.GoalID != goal["id"] || source.Round != index+1 {
						t.Fatal("real source missing or invented", item)
					}
					found++
				}
				if found != 1 {
					t.Fatal("Goal round has no unique actual user item")
				}
			}
			history := listAny(state["history"])
			if len(history) < 4 {
				t.Fatal("Goal immutable history missing", state)
			}
			ref := map[string]any{"id": goal["id"], "revision": goal["revision"]}
			if code, _ := goalHTTPV1(t, handler, threadID, http.MethodPost, map[string]any{"operation": "resume", "ref": ref}); code != http.StatusBadRequest {
				t.Fatal("exhausted Goal resumed", code)
			}
			if fixtureProvider.calls.Load() != 2 {
				t.Fatal("failed resume started another Provider call")
			}
			return
		case err := <-streamErrors:
			t.Fatal("Goal production SSE failed", err)
		case <-deadline.C:
			t.Fatal("Goal did not reach blocked terminal budget through real production path")
		}
	}
}
