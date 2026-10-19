package sse

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
)

type finiteCancellationWriter struct {
	header                   http.Header
	headerWrites, bodyWrites int
	body                     bytes.Buffer
}

func (w *finiteCancellationWriter) Header() http.Header { return w.header }
func (w *finiteCancellationWriter) WriteHeader(int)     { w.headerWrites++ }
func (w *finiteCancellationWriter) Write(data []byte) (int, error) {
	w.bodyWrites++
	return w.body.Write(data)
}

func (w *finiteCancellationWriter) assertUnpublished(t *testing.T) {
	t.Helper()
	if w.headerWrites != 0 || w.bodyWrites != 0 || len(w.header) != 0 || w.body.Len() != 0 {
		t.Fatalf("cancelled finite replay wrote a response: headers=%d writes=%d header=%v body=%s",
			w.headerWrites, w.bodyWrites, w.header, w.body.String())
	}
}

func finiteCancellationStore(store ThreadEventStore) ThreadEventStore {
	store.PreflightPublic = func(string) string { return "" }
	if store.ValidateAcceptedFinalDelivery == nil {
		store.ValidateAcceptedFinalDelivery = func(domainevent.AcceptedFinalDeliveryBatchV2) error { return nil }
	}
	return store
}

func finiteCancellationEvents() []map[string]any {
	return []map[string]any{
		{"kind": "thread_updated", "threadId": "finite-cancel", "seq": float64(1), "timestamp": "2026-10-19T00:00:00Z", "status": "idle"},
		{"kind": "thread_updated", "threadId": "finite-cancel", "seq": float64(2), "timestamp": "2026-10-19T00:00:00Z", "status": "idle"},
		{"kind": "thread_updated", "threadId": "finite-cancel", "seq": float64(3), "timestamp": "2026-10-19T00:00:00Z", "status": "idle"},
	}
}

func TestFiniteReplayCancellationStopsBeforeProjection(t *testing.T) {
	for _, phase := range []string{"before_request", "preflight", "highest_seq", "load"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			preflights, highest, loads, projections := 0, 0, 0, 0
			store := ThreadEventStore{
				PreflightPublic: func(string) string {
					preflights++
					if phase == "preflight" {
						cancel()
					}
					return ""
				},
				HighestSeq: func(string) (int, error) {
					highest++
					if phase == "highest_seq" {
						cancel()
					}
					return 3, nil
				},
				LoadEventsSince: func(string, int) ([]map[string]any, error) {
					loads++
					if phase == "load" {
						cancel()
					}
					return finiteCancellationEvents(), nil
				},
				ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
					projections++
					return event, true, ""
				},
			}
			if phase == "before_request" {
				cancel()
			}
			writer := &finiteCancellationWriter{header: make(http.Header)}
			ThreadEventsHandler{Store: store}.ServeHTTP(writer,
				httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), "finite-cancel")
			writer.assertUnpublished(t)
			if projections != 0 || phase == "before_request" && preflights != 0 ||
				phase == "preflight" && highest != 0 || phase == "highest_seq" && loads != 0 {
				t.Fatalf("work continued after cancellation: preflight=%d highest=%d load=%d project=%d", preflights, highest, loads, projections)
			}
		})
	}
}

func TestFiniteReplayCancellationDiscardsProjectedPrefix(t *testing.T) {
	for _, outcome := range []string{"visible", "hidden_cursor", "revocation", "last_event"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, stopAt := 0, 2
			if outcome == "last_event" {
				stopAt = 3
			}
			store := finiteCancellationStore(ThreadEventStore{
				HighestSeq:      func(string) (int, error) { return 3, nil },
				LoadEventsSince: func(string, int) ([]map[string]any, error) { return finiteCancellationEvents(), nil },
				ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
					calls++
					if calls == stopAt {
						cancel()
					}
					if outcome == "revocation" && calls == stopAt {
						return nil, false, casePublicAuthorityRejectedCode
					}
					return event, outcome != "hidden_cursor", ""
				},
			})
			writer := &finiteCancellationWriter{header: make(http.Header)}
			ThreadEventsHandler{Store: store}.ServeHTTP(writer,
				httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), "finite-cancel")
			writer.assertUnpublished(t)
			if calls != stopAt {
				t.Fatalf("projected %d events after cancellation at %d", calls, stopAt)
			}
		})
	}
}

func TestFiniteReplayCancellationStopsInsideGeneralTerminalGroup(t *testing.T) {
	events := generalTerminalSSEFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := generalTerminalSSEStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 13, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return events, nil },
	})
	project, calls := store.ProjectPublic, 0
	store.ProjectPublic = func(id string, event map[string]any) (map[string]any, bool, string) {
		calls++
		value, visible, code := project(id, event)
		cancel()
		return value, visible, code
	}
	writer := &finiteCancellationWriter{header: make(http.Header)}
	ThreadEventsHandler{Store: store}.ServeHTTP(writer,
		httptest.NewRequest(http.MethodGet, "/events?since_seq=12", nil).WithContext(ctx), "thread-general-sse")
	writer.assertUnpublished(t)
	if calls != 1 {
		t.Fatalf("ordinary terminal group continued after cancellation: %d callbacks", calls)
	}
}

func TestFiniteReplayCancellationStopsInsideAcceptedFinalGroup(t *testing.T) {
	batch := acceptedFinalSSEDeliveryBatchFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls, seals := 0, 0
	store := finiteCancellationStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return batch.LastSeq, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return batch.Events, nil },
		SealAcceptedFinalDelivery: func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			seals++
			return batch.PublicationAuthority, nil
		},
		ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
			calls++
			cancel()
			return event, true, ""
		},
	})
	writer := &finiteCancellationWriter{header: make(http.Header)}
	ThreadEventsHandler{Store: store}.ServeHTTP(writer,
		httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), batch.ThreadID)
	writer.assertUnpublished(t)
	if calls != 1 || seals != 1 {
		t.Fatalf("accepted-final cancellation changed boundary: project=%d seals=%d", calls, seals)
	}
}

func TestFiniteReplayCancellationStopsBeforeNextSeal(t *testing.T) {
	first := acceptedFinalSSEDeliveryBatchFixture(t)
	second := remapAcceptedFinalSSEDeliveryBatchFixture(t, first, "turn-cancel-second", 4, 41)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seals, projections := 0, 0
	store := finiteCancellationStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return second.LastSeq, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) {
			return append(append([]map[string]any{}, first.Events...), second.Events...), nil
		},
		SealAcceptedFinalDelivery: func([]map[string]any) (domainevent.AcceptedFinalDeliverySealV1, error) {
			seals++
			cancel()
			return first.PublicationAuthority, nil
		},
		ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
			projections++
			return event, true, ""
		},
	})
	writer := &finiteCancellationWriter{header: make(http.Header)}
	ThreadEventsHandler{Store: store}.ServeHTTP(writer,
		httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), first.ThreadID)
	writer.assertUnpublished(t)
	if seals != 1 || projections != 0 {
		t.Fatalf("work continued after seal cancelled: seals=%d projects=%d", seals, projections)
	}
}

func TestFiniteReplayCancellationDoesNotPoisonNextRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	store := finiteCancellationStore(ThreadEventStore{
		HighestSeq:      func(string) (int, error) { return 3, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) { return finiteCancellationEvents(), nil },
		ProjectPublic: func(_ string, event map[string]any) (map[string]any, bool, string) {
			calls++
			if calls == 1 {
				cancel()
			}
			return event, true, ""
		},
	})
	handler := ThreadEventsHandler{Store: store}
	first := &finiteCancellationWriter{header: make(http.Header)}
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), "finite-cancel")
	first.assertUnpublished(t)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/events", nil), "finite-cancel")
	if calls != 4 || bytes.Count(second.Body.Bytes(), []byte("event: thread_updated")) != 3 {
		t.Fatalf("cancelled request leaked context to next request: calls=%d response=%s", calls, second.Body.String())
	}
}
