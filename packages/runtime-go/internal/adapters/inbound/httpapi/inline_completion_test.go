package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
)

type inlineCompletionTestService func(context.Context, apploop.InlineCompletionRequest) (apploop.InlineCompletionResult, error)

func (f inlineCompletionTestService) Complete(ctx context.Context, request apploop.InlineCompletionRequest) (apploop.InlineCompletionResult, error) {
	return f(ctx, request)
}

func inlineCompletionTestBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(apploop.InlineCompletionRequest{ThreadID: "primary-thread", RequestID: "7e2e6074-90d4-4e8b-a5c1-1e3d79d737f9", Document: apploop.InlineCompletionDocument{Path: "draft.md"}, Prompt: "Complete this draft", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestInlineCompletionClosedRequestAndLocalLane(t *testing.T) {
	calls := 0
	service := inlineCompletionTestService(func(_ context.Context, request apploop.InlineCompletionRequest) (apploop.InlineCompletionResult, error) {
		calls++
		return apploop.InlineCompletionResult{ThreadID: request.ThreadID, RequestID: request.RequestID, ObjectID: strings.Repeat("a", 64), BaseRevision: strings.Repeat("b", 64), Text: "suggestion"}, nil
	})
	mux := LocalDisplayMuxV1{RuntimeToken: "synthetic-token", LocalDisplay: LocalDisplayHandlerV1{InlineCompletion: InlineCompletionHandler{Service: service}}}
	body := inlineCompletionTestBody(t)
	run := func(body []byte, authorized, local bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, InlineCompletionPath, bytes.NewReader(body))
		if authorized {
			r.Header.Set("Authorization", "Bearer synthetic-token")
		}
		if local {
			r.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, pair := range [][2]bool{{false, false}, {false, true}, {true, false}} {
		if w := run(body, pair[0], pair[1]); w.Code == http.StatusOK {
			t.Fatal("untrusted local-display request admitted")
		}
	}
	if calls != 0 {
		t.Fatal("untrusted request reached service")
	}
	for index, invalid := range []string{
		strings.Replace(string(body), `"threadId":`, `"ThreadID":`, 1),
		strings.Replace(string(body), `"threadId":`, `"threadId":"duplicate","threadId":`, 1),
		strings.Replace(string(body), `"document":{"path":"draft.md"}`, `"document":{"path":"draft.md","sessionId":""}`, 1),
		strings.Replace(string(body), `"document":{"path":"draft.md"}`, `"document":null`, 1),
		strings.Replace(string(body), `"model":"test-model"`, `"model":""`, 1),
		strings.Replace(string(body), `"prompt":"Complete this draft"`, `"prompt":"`+strings.Repeat("中", 22000)+`"`, 1),
		string(body) + ` {}`,
	} {
		w := run([]byte(invalid), true, true)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) {
			t.Fatalf("invalid request case=%d status=%d", index, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached service")
	}
	w := run(body, true, true)
	if w.Code != http.StatusOK || calls != 1 || !strings.Contains(w.Body.String(), `"text":"suggestion"`) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("valid auxiliary result unavailable")
	}
}

func TestInlineCompletionMapsErrorsWithoutPrivateDetails(t *testing.T) {
	for _, item := range []struct {
		err    error
		code   string
		status int
	}{
		{apploop.ErrInlineCompletionBusy, "busy", http.StatusConflict},
		{apploop.ErrInlineCompletionCanceled, "canceled", http.StatusRequestTimeout},
		{errors.New("PRIVATE_DIAGNOSTIC_CANARY"), "unavailable", http.StatusServiceUnavailable},
	} {
		h := InlineCompletionHandler{Service: inlineCompletionTestService(func(context.Context, apploop.InlineCompletionRequest) (apploop.InlineCompletionResult, error) {
			return apploop.InlineCompletionResult{}, item.err
		})}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, InlineCompletionPath, bytes.NewReader(inlineCompletionTestBody(t))))
		if w.Code != item.status || !strings.Contains(w.Body.String(), `"code":"`+item.code+`"`) || strings.Contains(w.Body.String(), "PRIVATE_DIAGNOSTIC_CANARY") {
			t.Fatal("unsafe auxiliary error envelope")
		}
	}
}

func TestInlineCompletionClientDisconnectCancelsService(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(InlineCompletionHandler{Service: inlineCompletionTestService(func(ctx context.Context, _ apploop.InlineCompletionRequest) (apploop.InlineCompletionResult, error) {
		close(entered)
		<-ctx.Done()
		close(stopped)
		return apploop.InlineCompletionResult{}, apploop.ErrInlineCompletionCanceled
	})})
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(inlineCompletionTestBody(t)))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("service was not reached")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("client cancellation did not reach service")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("client did not observe cancellation")
	}
}
