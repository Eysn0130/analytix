package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

func TestFiniteReplayFinalRejectionDiscardsBufferedEvents(t *testing.T) {
	store := generalTerminalSSEStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 1, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) {
			return []map[string]any{{"kind": "turn_started", "threadId": "thread-finite", "turnId": "turn-finite", "seq": float64(1)}}, nil
		},
		FinishPublicReplay: func(string) string { return casePublicAuthorityRejectedCode },
	})
	response := httptest.NewRecorder()
	ThreadEventsHandler{Store: store}.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/events", nil), "thread-finite")
	body := response.Body.String()
	if !strings.Contains(body, "public_projection_revoked") || strings.Contains(body, "event: turn_started") || strings.Contains(body, "event: cursor_advanced") {
		t.Fatalf("final rejection published a buffered prefix: %s", body)
	}
}

func TestFiniteReplayCancellationStopsNestedMembersAndWritesNothing(t *testing.T) {
	for _, kind := range []string{"general", "accepted_final"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			threadID := "thread-general-sse"
			events := generalTerminalSSEFixture(t)
			store := generalTerminalSSEStore(ThreadEventStore{})
			if kind == "accepted_final" {
				batch := acceptedFinalSSEDeliveryBatchFixture(t)
				threadID, events = batch.ThreadID, batch.Events
				store.SealAcceptedFinalDelivery = func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
					return batch.PublicationAuthority, nil
				}
			}
			calls := 0
			originalProject := store.ProjectPublic
			store.ProjectPublic = func(id string, event map[string]any) (map[string]any, bool, string) {
				calls++
				cancel()
				return originalProject(id, event)
			}
			store.HighestSeq = func(string) (int, error) { return 100, nil }
			store.LoadEventsSince = func(string, int) ([]map[string]any, error) { return events, nil }
			store.FinishPublicReplay = func(string) string { t.Fatal("cancelled batch reached final gate"); return "" }
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
			ThreadEventsHandler{Store: store}.ServeHTTP(response, request, threadID)
			if calls != 1 || response.Body.Len() != 0 || len(response.Header()) != 0 {
				t.Fatalf("cancelled batch continued or wrote a prefix: calls=%d headers=%v body=%s", calls, response.Header(), response.Body.String())
			}
		})
	}
}

func TestFiniteReplayCancellationAtFinalGateWritesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := generalTerminalSSEStore(ThreadEventStore{
		HighestSeq:         func(string) (int, error) { return 0, nil },
		FinishPublicReplay: func(string) string { cancel(); return "" },
	})
	response := httptest.NewRecorder()
	ThreadEventsHandler{Store: store}.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), "thread-cancel-final")
	if response.Body.Len() != 0 || len(response.Header()) != 0 {
		t.Fatalf("cancelled final gate wrote response: headers=%v body=%s", response.Header(), response.Body.String())
	}
}

func TestLiveReplayDoesNotUseFiniteFinalGate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := generalTerminalSSEStore(ThreadEventStore{
		HighestSeq:         func(string) (int, error) { return 0, nil },
		SubscribeEvents:    func(string) (<-chan map[string]any, func()) { return make(chan map[string]any), func() {} },
		FinishPublicReplay: func(string) string { t.Fatal("live replay used finite final gate"); return "" },
	})
	ThreadEventsHandler{Store: store}.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/events?live=1", nil).WithContext(ctx), "thread-live")
}
