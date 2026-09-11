package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	providerschema "analytix.local/runtime-go/internal/adapters/outbound/provider/schema"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func providerTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte(seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func providerPublicFailureCode(err error) string {
	var publicFailure interface{ PublicFailureRecord() domainfailure.Record }
	if !errors.As(err, &publicFailure) {
		return ""
	}
	return publicFailure.PublicFailureRecord().Code()
}

func TestDefaultHTTPClientWithProxyUsesConfiguredProxyAndRedactsDiagnostics(t *testing.T) {
	client := NewDefaultHTTPClientWithProxy("socks5://proxy-user:proxy-secret@127.0.0.1:7890")
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatalf("configured provider proxy should install an http.Transport proxy: %#v", client.Transport)
	}
	target, err := url.Parse("https://api.deepseek.com/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(&http.Request{URL: target})
	if err != nil {
		t.Fatalf("proxy lookup failed: %v", err)
	}
	if proxyURL == nil || proxyURL.Scheme != "socks5" || proxyURL.Host != "127.0.0.1:7890" {
		t.Fatalf("configured provider proxy not used: %v", proxyURL)
	}
	if password, ok := proxyURL.User.Password(); !ok || password != "proxy-secret" {
		t.Fatalf("transport proxy should preserve the password for dialing")
	}

	diagnostics := NetworkProxyDiagnostics("socks5://proxy-user:proxy-secret@127.0.0.1:7890")
	if diagnostics["configured"] != true ||
		diagnostics["valid"] != true ||
		diagnostics["source"] != "settings.provider.proxy" ||
		diagnostics["summary"] != "custom (socks5://proxy-user@127.0.0.1:7890)" {
		t.Fatalf("proxy diagnostics mismatch: %#v", diagnostics)
	}
	raw := string(mustProviderJSON(t, diagnostics))
	if strings.Contains(raw, "proxy-secret") {
		t.Fatalf("proxy diagnostics leaked password: %s", raw)
	}

	aliasDiagnostics := NetworkProxyDiagnostics("socks://proxy-user:proxy-secret@127.0.0.1:7890")
	if aliasDiagnostics["valid"] != true ||
		aliasDiagnostics["summary"] != "custom (socks5://proxy-user@127.0.0.1:7890)" {
		t.Fatalf("socks proxy alias diagnostics mismatch: %#v", aliasDiagnostics)
	}
	if strings.Contains(string(mustProviderJSON(t, aliasDiagnostics)), "proxy-secret") {
		t.Fatalf("socks alias diagnostics leaked password: %#v", aliasDiagnostics)
	}

	invalid := NetworkProxyDiagnostics("http://proxy-user:proxy-secret@%")
	invalidJSON := string(mustProviderJSON(t, invalid))
	if invalid["valid"] != false || strings.Contains(invalidJSON, "proxy-secret") {
		t.Fatalf("invalid proxy diagnostics should be false and redacted: %#v", invalid)
	}
}

func TestHTTPProviderClientReturnsSanitizedAuthError(t *testing.T) {
	const secret = "sk-test-provider-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Authentication Fails, Your api key: ` + secret + ` is invalid"}}`))
	}))
	defer server.Close()

	client := NewHTTPProviderClient(server.Client())
	_, err := client.Stream(context.Background(), Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         secret,
		Model:          "deepseek-chat",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider auth error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if providerErr.Kind != "auth" || providerErr.Status != http.StatusUnauthorized || !providerErr.AuthRequired() {
		t.Fatalf("unexpected provider auth error: %#v", providerErr)
	}
	diagnostics := providerErr.Diagnostics()
	if diagnostics["authStatus"] != "required" || diagnostics["hasApiKey"] != true || diagnostics["providerId"] != "deepseek" {
		t.Fatalf("provider diagnostics mismatch: %#v", diagnostics)
	}
	raw, _ := json.Marshal(diagnostics)
	if strings.Contains(err.Error(), secret) || strings.Contains(string(raw), secret) {
		t.Fatalf("provider auth diagnostics leaked API key: err=%q diagnostics=%s", err.Error(), string(raw))
	}
}

func TestRuntimeProviderConfigSetValidateExecutionModelRejectsBoundedProviderMismatch(t *testing.T) {
	set := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: string(mustProviderJSON(t, ModelProvidersConfig{
			DefaultProviderID: "deepseek",
			Providers: []ModelProviderConfig{{
				ID:             "deepseek",
				APIKey:         "sk-test",
				BaseURL:        "https://api.deepseek.com/v1",
				EndpointFormat: "chat_completions",
				Models:         []string{"deepseek-v4-pro"},
			}},
		})),
	})

	if err := set.ValidateExecutionModel("deepseek", "deepseek-v4-pro"); err != nil {
		t.Fatalf("configured model should validate: %v", err)
	}
	err := set.ValidateExecutionModel("deepseek", "mimo-v2-pro")
	if err == nil {
		t.Fatal("expected invalid model error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
	if providerErr.Kind != "invalid_model" || providerErr.ProviderID != "deepseek" || providerErr.Family != "deepseek" {
		t.Fatalf("unexpected provider error: %#v", providerErr)
	}
	diagnostics := providerErr.Diagnostics()
	if diagnostics["kind"] != "invalid_model" || diagnostics["providerId"] != "deepseek" {
		t.Fatalf("invalid model diagnostics mismatch: %#v", diagnostics)
	}
}

func TestHTTPProviderClientRedactsCustomEndpointURLAndBody(t *testing.T) {
	const querySecret = "query-secret-token"
	const bodySecret = "body-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`Authorization: Bearer ` + bodySecret + ` rejected`))
	}))
	defer server.Close()

	client := NewHTTPProviderClient(server.Client())
	_, err := client.Stream(context.Background(), Request{
		ProviderID:     "custom",
		Family:         "custom_endpoint",
		EndpointFormat: "custom_endpoint",
		BaseURL:        server.URL + "/messages?key=" + querySecret,
		APIKey:         "custom-api-key",
		Model:          "custom-model",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider request error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if providerErr.Kind != "request" || providerErr.Status != http.StatusBadRequest {
		t.Fatalf("unexpected provider request error: %#v", providerErr)
	}
	diagnosticsJSON := string(mustProviderJSON(t, providerErr.Diagnostics()))
	if strings.Contains(err.Error(), querySecret) ||
		strings.Contains(err.Error(), bodySecret) ||
		strings.Contains(diagnosticsJSON, querySecret) ||
		strings.Contains(diagnosticsJSON, bodySecret) {
		t.Fatalf("provider diagnostics leaked secrets: err=%q diagnostics=%s", err.Error(), diagnosticsJSON)
	}
	if !strings.Contains(providerErr.RequestURL, "key=%3Credacted%3E") {
		t.Fatalf("provider request URL should redact auth query: %q", providerErr.RequestURL)
	}
}

func TestHTTPProviderClientCapturesRetryAfterDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer server.Close()

	client := NewHTTPProviderClient(server.Client())
	_, err := client.Stream(context.Background(), Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-retry-diagnostic",
		Model:          "deepseek-chat",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider retry-after error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if providerErr.Kind != "rate_limit" || !providerErr.Retryable() || providerErr.RetryAfterMs != 3000 {
		t.Fatalf("unexpected retry-after provider error: %#v", providerErr)
	}
	diagnostics := providerErr.Diagnostics()
	if diagnostics["retryAfterMs"] != float64(3000) {
		t.Fatalf("retry-after diagnostics missing: %#v", diagnostics)
	}
}

func TestHTTPProviderClientClassifiesInsufficientBalance(t *testing.T) {
	const secret = "sk-balance-diagnostic"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient balance for api_key=` + secret + `"}}`))
	}))
	defer server.Close()

	client := NewHTTPProviderClient(server.Client())
	_, err := client.Stream(context.Background(), Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         secret,
		Model:          "deepseek-chat",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider balance error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if providerErr.Kind != "insufficient_balance" || providerErr.Status != http.StatusPaymentRequired || providerErr.Retryable() {
		t.Fatalf("unexpected provider balance error: %#v", providerErr)
	}
	diagnostics := providerErr.Diagnostics()
	if diagnostics["kind"] != "insufficient_balance" || diagnostics["hasApiKey"] != true || diagnostics["authStatus"] != "none" {
		t.Fatalf("provider balance diagnostics mismatch: %#v", diagnostics)
	}
	raw, _ := json.Marshal(diagnostics)
	if strings.Contains(err.Error(), secret) || strings.Contains(string(raw), secret) {
		t.Fatalf("provider balance diagnostics leaked API key: err=%q diagnostics=%s", err.Error(), string(raw))
	}
}

func TestHTTPProviderClientReplaysPreOutputStreamCut(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		attempt := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if attempt == 1 {
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"retry ok"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	seen := []Chunk{}
	client := NewHTTPProviderClient(server.Client())
	result, err := client.Stream(context.Background(), Request{
		ProviderID:     "openai-live",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-stream-replay",
		Model:          "gpt-live",
		Messages:       []Message{{Role: "user", Content: "hello"}},
		OnChunk: func(chunk Chunk) error {
			seen = append(seen, chunk)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	totalRequests := requests
	mu.Unlock()
	if totalRequests != 2 {
		t.Fatalf("pre-output stream cut should replay once, got %d requests", totalRequests)
	}
	if got := collectChunkText(result.Chunks, ChunkText); got != "retry ok" {
		t.Fatalf("replayed stream text mismatch: %q chunks=%#v", got, result.Chunks)
	}
	if got := collectChunkText(seen, ChunkText); got != "retry ok" {
		t.Fatalf("callback should only see replayed output, got %q from %#v", got, seen)
	}
}

func TestHTTPProviderClientDoesNotReconnectAfterUncommittedText(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
	defer server.Close()

	seen := []Chunk{}
	client := NewHTTPProviderClient(server.Client())
	_, err := client.Stream(context.Background(), Request{
		ProviderID:     "openai-live",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-stream-visible-cut",
		Model:          "gpt-live",
		Messages:       []Message{{Role: "user", Content: "hello"}},
		OnChunk: func(chunk Chunk) error {
			seen = append(seen, chunk)
			return nil
		},
	})
	if err == nil || providerPublicFailureCode(err) != domainfailure.CodeProviderStreamInterrupted {
		t.Fatalf("expected visible stream-cut error, got %v", err)
	}
	var outputStarted interface{ ProviderOutputStarted() bool }
	if !errors.As(err, &outputStarted) || !outputStarted.ProviderOutputStarted() {
		t.Fatalf("post-text stream cut lost provider-output observation: %T %v", err, err)
	}
	mu.Lock()
	totalRequests := requests
	mu.Unlock()
	if totalRequests != 1 {
		t.Fatalf("observed provider text must make reconnect unsafe, got %d requests", totalRequests)
	}
	if got := collectChunkText(seen, ChunkText); got != "" {
		t.Fatalf("uncommitted partial output crossed the callback boundary: %q", got)
	}
}

func TestHTTPProviderClientIdleWatchdogStopsStalledStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewHTTPProviderClient(server.Client())
	client.StreamIdleTimeout = 20 * time.Millisecond
	client.MaxStreamReconnects = 0
	start := time.Now()
	_, err := client.Stream(context.Background(), Request{
		ProviderID:     "openai-live",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-stream-stall",
		Model:          "gpt-live",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil || !strings.Contains(err.Error(), "stream stalled") {
		t.Fatalf("expected stream stalled watchdog error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("idle watchdog should fail quickly, took %s", elapsed)
	}
}

func TestDefaultHTTPProviderClientDoesNotApplyTotalTimeoutToActiveSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"slow "}}]}` + "\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(60 * time.Millisecond)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"done"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewHTTPProviderClient(NewDefaultHTTPClient(20 * time.Millisecond))
	client.StreamIdleTimeout = 500 * time.Millisecond
	result, err := client.Stream(context.Background(), Request{
		ProviderID:     "openai-live",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-stream-long",
		Model:          "gpt-live",
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("active SSE must be governed by idle watchdog, not total http.Client timeout: %v", err)
	}
	if got := collectChunkText(result.Chunks, ChunkText); got != "slow done" {
		t.Fatalf("unexpected active SSE text: %q chunks=%#v", got, result.Chunks)
	}
}

func TestParseOpenAIChatToolCallStreamSynthesizesIDAndArguments(t *testing.T) {
	chunks, usage, err := parseSSE("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
		`data: [DONE]`,
	}, "\n\n")))
	if err != nil {
		t.Fatal(err)
	}
	start := firstToolCallStartChunk(t, chunks)
	if start.ID != "call_0" || start.Name != "read_file" {
		t.Fatalf("unexpected OpenAI chat tool call start: %#v", start)
	}
	call := firstToolCallChunk(t, chunks)
	if call.ID != "call_0" || call.Name != "read_file" || string(call.Arguments) != `{"path":"a.txt"}` {
		t.Fatalf("unexpected OpenAI chat tool call: %#v", call)
	}
	if usage.PromptTokens != 7 || usage.CompletionTokens != 3 || usage.TotalTokens != 10 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestParseOpenAIChatMultipleMissingIDsStayDistinctByIndex(t *testing.T) {
	chunks, _, err := parseSSE("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"read_file","arguments":"{\"path\""}},{"index":1,"type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a.txt\"}"}},{"index":1,"function":{"arguments":":\"b.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n")))
	if err != nil {
		t.Fatal(err)
	}
	calls := toolCallChunks(chunks)
	if len(calls) != 2 {
		t.Fatalf("expected two distinct tool_call chunks, got %#v", chunks)
	}
	if calls[0].ID != "call_0" || calls[1].ID != "call_1" ||
		string(calls[0].Arguments) != `{"path":"a.txt"}` ||
		string(calls[1].Arguments) != `{"path":"b.txt"}` {
		t.Fatalf("missing-id tool calls should stay distinct by stream index: %#v", calls)
	}
}

func TestParseOpenAIChatSplitsLeadingThinkTagsAcrossDeltas(t *testing.T) {
	chunks, _, err := parseSSE("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"\n\n  <th"}}]}`,
		`data: {"choices":[{"delta":{"content":"ink>private "}}]}`,
		`data: {"choices":[{"delta":{"content":"chain</thi"}}]}`,
		`data: {"choices":[{"delta":{"content":"nk>\n\nvisible"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n")))
	if err != nil {
		t.Fatal(err)
	}
	if got := collectChunkText(chunks, ChunkReasoning); got != "" {
		t.Fatalf("inline private reasoning must be discarded, got %q from %#v", got, chunks)
	}
	if got := collectChunkText(chunks, ChunkText); got != "\n\n  \n\nvisible" {
		t.Fatalf("public bytes around inline reasoning must remain exact, got %q from %#v", got, chunks)
	}
}

func TestParseOpenAIChatRejectsUnclosedRawThinkMarker(t *testing.T) {
	chunks, _, err := parseSSE("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"the model emits "}}]}`,
		`data: {"choices":[{"delta":{"content":"<think> tags around its reasoning"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}, "\n\n")))
	var protocolErr domainreasoningmarkup.ProtocolError
	if !errors.As(err, &protocolErr) || !errors.Is(err, domainreasoningmarkup.ErrIncomplete) || len(chunks) != 0 {
		t.Fatalf("unclosed raw marker was not blocked: chunks=%#v err=%v", chunks, err)
	}
}

func TestParseOpenAIResponsesToolCallStream(t *testing.T) {
	chunks, usage, err := parseSSE("responses", strings.NewReader(strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_resp","name":"lookup","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"query\""}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":":\"weather\"}"}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_resp","name":"lookup","arguments":""}}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":9,"output_tokens":4,"total_tokens":13,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1}}}}`,
	}, "\n\n")))
	if err != nil {
		t.Fatal(err)
	}
	start := firstToolCallStartChunk(t, chunks)
	if start.ID != "call_resp" || start.Name != "lookup" {
		t.Fatalf("unexpected OpenAI responses tool call start: %#v", start)
	}
	call := firstToolCallChunk(t, chunks)
	if call.ID != "call_resp" || call.Name != "lookup" || string(call.Arguments) != `{"query":"weather"}` {
		t.Fatalf("unexpected OpenAI responses tool call: %#v", call)
	}
	if usage.PromptTokens != 9 || usage.CompletionTokens != 4 || usage.TotalTokens != 13 || usage.CacheHitTokens != 2 || usage.ReasoningTokens != 1 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestParseOpenAIResponsesDoneFlushesToolCallThroughCallback(t *testing.T) {
	seen := []string{}
	chunks, _, err := parseSSEWithCallback("responses", strings.NewReader(strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_resp","name":"lookup","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"query\":\"weather\"}"}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
		seen = append(seen, string(chunk.Kind))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := chunkKinds(chunks); !sameChunkKindSlice(got, []string{"tool_call_start", "tool_call", "done"}) {
		t.Fatalf("unexpected response chunks: %#v", got)
	}
	if !sameChunkKindSlice(seen, []string{"tool_call_start", "tool_call", "done"}) {
		t.Fatalf("Responses [DONE] must flush tool_call and done through OnChunk, got %#v", seen)
	}
}

func TestParseOpenAIResponsesCompletedThenDoneDoesNotDuplicateDoneCallback(t *testing.T) {
	seen := []string{}
	chunks, _, err := parseSSEWithCallback("responses", strings.NewReader(strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
		seen = append(seen, string(chunk.Kind))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := chunkKinds(chunks); !sameChunkKindSlice(got, []string{"text", "usage", "done"}) {
		t.Fatalf("unexpected response chunks: %#v", got)
	}
	if !sameChunkKindSlice(seen, []string{"text", "usage", "done"}) {
		t.Fatalf("Responses completed + [DONE] must not duplicate done through OnChunk, got %#v", seen)
	}
}

func TestParseOpenAIChatCleanEOFFinishesWhenFinishReasonArrived(t *testing.T) {
	chunks, _, err := parseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}, "\n\n")), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := chunkKinds(chunks); !sameChunkKindSlice(got, []string{"text", "done"}) {
		t.Fatalf("terminal finish_reason should complete clean EOF chat streams, got %#v", got)
	}
}

func TestParseOpenAIChatCleanEOFRejectsIncompleteToolCall(t *testing.T) {
	seen := []string{}
	_, _, err := parseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
	}, "\n\n")), func(chunk Chunk) error {
		seen = append(seen, string(chunk.Kind))
		return nil
	})
	if err == nil || providerPublicFailureCode(err) != domainfailure.CodeProviderStreamInterrupted {
		t.Fatalf("incomplete chat tool call should fail on clean EOF, err=%v", err)
	}
	if !sameChunkKindSlice(seen, []string{"tool_call_start"}) {
		t.Fatalf("incomplete chat tool call must not flush final tool_call/done chunks, got %#v", seen)
	}
}

func TestParseOpenAIResponsesCleanEOFRejectsIncompleteToolCall(t *testing.T) {
	seen := []string{}
	_, _, err := parseSSEWithCallback("responses", strings.NewReader(strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_resp","name":"lookup","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"query\""}`,
	}, "\n\n")), func(chunk Chunk) error {
		seen = append(seen, string(chunk.Kind))
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "before completing tool calls") {
		t.Fatalf("incomplete Responses tool call should fail on clean EOF, err=%v", err)
	}
	if !sameChunkKindSlice(seen, []string{"tool_call_start"}) {
		t.Fatalf("incomplete Responses tool call must not flush final tool_call/done chunks, got %#v", seen)
	}
}

func TestParseAnthropicCleanEOFRejectsIncompleteToolUse(t *testing.T) {
	seen := []string{}
	_, _, err := parseSSEWithCallback("messages", strings.NewReader(strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\""}}`,
	}, "\n\n")), func(chunk Chunk) error {
		seen = append(seen, string(chunk.Kind))
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "before completing tool calls") {
		t.Fatalf("incomplete Anthropic tool_use should fail on clean EOF, err=%v", err)
	}
	if !sameChunkKindSlice(seen, []string{"tool_call_start"}) {
		t.Fatalf("incomplete Anthropic tool_use must not flush final tool_call/done chunks, got %#v", seen)
	}
}

func TestParseAnthropicToolUseStream(t *testing.T) {
	chunks, usage, err := parseSSE("messages", strings.NewReader(strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"lookup","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":":\"Shanghai\"}"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","usage":{"output_tokens":2}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n")))
	if err != nil {
		t.Fatal(err)
	}
	start := firstToolCallStartChunk(t, chunks)
	if start.ID != "toolu_1" || start.Name != "lookup" {
		t.Fatalf("unexpected Anthropic tool_use start: %#v", start)
	}
	call := firstToolCallChunk(t, chunks)
	if call.ID != "toolu_1" || call.Name != "lookup" || string(call.Arguments) != `{"city":"Shanghai"}` {
		t.Fatalf("unexpected Anthropic tool_use: %#v", call)
	}
	if usage.PromptTokens != 5 || usage.CompletionTokens != 2 || usage.TotalTokens != 7 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

func TestAnthropicRequestRoundTripsSignedThinking(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "use a tool"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:        "toolu_1",
				Name:      "lookup",
				Arguments: json.RawMessage(`{"q":"x"}`),
			}},
		},
		{Role: "tool", ToolCallID: "toolu_1", Content: "ok"},
	}
	session, err := domainmodel.NewPrivateProtocolSession()
	if err != nil {
		t.Fatal(err)
	}
	contextDigest := domainsecurity.SHA256Hex([]byte("anthropic-context"))
	providerRouteHash := domainsecurity.SHA256Hex([]byte("anthropic-route"))
	toolManifestHash := domainsecurity.SHA256Hex([]byte("anthropic-tools"))
	capsule, err := session.IssueAnthropicThinking(domainmodel.AnthropicThinkingCapsuleInput{
		ContextDigest: contextDigest, ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 2, AssistantMessageIndex: 1, AssistantMessage: messages[1],
		PrefixMessages: messages[:1], Thinking: "signed chain", Signature: "sig_123",
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := session.ConsumeAnthropicThinking(capsule, domainmodel.AnthropicThinkingConsumeInput{
		ContextDigest: contextDigest, ProviderRouteHash: providerRouteHash, PromptRoute: "general",
		ToolManifestHash: toolManifestHash, Sequence: 2, Messages: messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, body, _, err := buildHTTPRequest(Request{
		ProviderID:              "anthropic",
		Family:                  "anthropic-compatible",
		EndpointFormat:          "messages",
		BaseURL:                 "https://api.anthropic.com",
		APIKey:                  "test-key",
		Model:                   "claude-test",
		Messages:                messages,
		AnthropicThinkingReplay: replay,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	text := string(raw)
	if !strings.Contains(text, `"type":"thinking"`) ||
		!strings.Contains(text, `"thinking":"signed chain"`) ||
		!strings.Contains(text, `"signature":"sig_123"`) {
		t.Fatalf("Anthropic request did not preserve signed thinking: %s", text)
	}
	messagesBody, _ := body["messages"].([]map[string]any)
	assistantContent, _ := messagesBody[1]["content"].([]map[string]any)
	if len(assistantContent) < 2 || assistantContent[0]["type"] != "thinking" || assistantContent[1]["type"] != "tool_use" {
		t.Fatalf("signed thinking must retain its exact position before assistant tool_use: %#v", assistantContent)
	}
}

func TestAnthropicRequestReplaysEveryThinkingBlockIncludingSignatureOnly(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "use tools"},
		{Role: "assistant", ToolCalls: []ToolCall{{
			ID: "toolu_1", Name: "lookup", Arguments: json.RawMessage(`{"q":"a"}`),
		}}},
		{Role: "tool", ToolCallID: "toolu_1", Content: "a"},
		{Role: "assistant", ToolCalls: []ToolCall{{
			ID: "toolu_2", Name: "lookup", Arguments: json.RawMessage(`{"q":"b"}`),
		}}},
		{Role: "tool", ToolCallID: "toolu_2", Content: "b"},
	}
	session, err := domainmodel.NewPrivateProtocolSession()
	if err != nil {
		t.Fatal(err)
	}
	contextDigest := domainsecurity.SHA256Hex([]byte("anthropic-multi-context"))
	providerRouteHash := domainsecurity.SHA256Hex([]byte("anthropic-multi-route"))
	toolManifestHash := domainsecurity.SHA256Hex([]byte("anthropic-multi-tools"))
	issue := func(index int, sequence uint64, thinking, signature string) *domainmodel.AnthropicThinkingCapsule {
		t.Helper()
		capsule, issueErr := session.IssueAnthropicThinking(domainmodel.AnthropicThinkingCapsuleInput{
			Protocol: "anthropic-thinking", ContextDigest: contextDigest,
			ProviderRouteHash: providerRouteHash, PromptRoute: "general",
			ToolManifestHash: toolManifestHash, Sequence: sequence, AssistantMessageIndex: index,
			PrefixMessages: messages[:index], AssistantMessage: messages[index],
			Thinking: thinking, Signature: signature,
		})
		if issueErr != nil {
			t.Fatalf("issue replay %d: %v", index, issueErr)
		}
		return capsule
	}
	consume := func(capsule *domainmodel.AnthropicThinkingCapsule) *domainmodel.AnthropicThinkingReplay {
		t.Helper()
		replay, consumeErr := session.ConsumeAnthropicThinking(capsule, domainmodel.AnthropicThinkingConsumeInput{
			Protocol: "anthropic-thinking", ContextDigest: contextDigest,
			ProviderRouteHash: providerRouteHash, PromptRoute: "general",
			ToolManifestHash: toolManifestHash, Sequence: 3, Messages: messages,
		})
		if consumeErr != nil {
			t.Fatalf("consume replay: %v", consumeErr)
		}
		return replay
	}
	replays := []*domainmodel.AnthropicThinkingReplay{
		consume(issue(1, 2, "", "sig_omitted")),
		consume(issue(3, 3, "second private block", "sig_second")),
	}
	_, body, _, err := buildHTTPRequest(Request{
		ProviderID: "anthropic", Family: "anthropic-compatible", EndpointFormat: "messages",
		BaseURL: "https://api.anthropic.com", APIKey: "test-key", Model: "claude-test",
		Messages: messages, DeepSeekReasoningReplays: replays,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	text := string(raw)
	for _, expected := range []string{
		`"thinking":""`, `"signature":"sig_omitted"`,
		`"thinking":"second private block"`, `"signature":"sig_second"`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("Anthropic multi-block request lost %s: %s", expected, text)
		}
	}
}

func TestAnthropicRequestUsesNonEmptyToolResultFallback(t *testing.T) {
	_, body, _, err := buildHTTPRequest(Request{
		ProviderID:     "anthropic",
		Family:         "anthropic-compatible",
		EndpointFormat: "messages",
		BaseURL:        "https://api.anthropic.com",
		APIKey:         "test-key",
		Model:          "claude-test",
		Messages: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID:        "toolu_empty",
				Name:      "lookup",
				Arguments: json.RawMessage(`{"q":"x"}`),
			}}},
			{Role: "tool", ToolCallID: "toolu_empty", Content: ""},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"content":"(no output)"`) {
		t.Fatalf("Anthropic empty tool_result must use a non-empty fallback: %s", string(raw))
	}
}

func TestReasoningProtocolControlsProviderRequestFields(t *testing.T) {
	_, openAIBody, _, err := buildHTTPRequest(Request{
		ProviderID:      "openai-compatible",
		Family:          "openai-compatible",
		EndpointFormat:  "chat_completions",
		BaseURL:         "https://openai.example/v1",
		APIKey:          "test-key",
		Model:           "gpt-compatible",
		ReasoningEffort: "high",
		Messages:        []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := openAIBody["reasoning_effort"]; ok {
		t.Fatalf("ordinary OpenAI-compatible chat must not receive DeepSeek reasoning_effort: %#v", openAIBody)
	}
	if _, ok := openAIBody["thinking"]; ok {
		t.Fatalf("ordinary OpenAI-compatible chat must not receive thinking field: %#v", openAIBody)
	}

	_, xiaomiWithoutProtocolBody, _, err := buildHTTPRequest(Request{
		ProviderID:      "xiaomi",
		Family:          "openai-compatible",
		EndpointFormat:  "chat_completions",
		BaseURL:         "https://api.xiaomimimo.com/v1",
		APIKey:          "test-key",
		Model:           "mimo-v2.5-pro",
		ReasoningEffort: "high",
		Messages:        []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := xiaomiWithoutProtocolBody["reasoning_effort"]; ok {
		t.Fatalf("Xiaomi/MiMo must opt into scoped reasoning via model profile protocol: %#v", xiaomiWithoutProtocolBody)
	}
	if _, ok := xiaomiWithoutProtocolBody["thinking"]; ok {
		t.Fatalf("Xiaomi/MiMo must not receive thinking field without model profile protocol: %#v", xiaomiWithoutProtocolBody)
	}

	_, mimoBody, _, err := buildHTTPRequest(Request{
		ProviderID:        "xiaomi-mimo",
		Family:            "openai-compatible",
		EndpointFormat:    "chat_completions",
		BaseURL:           "https://api.xiaomimimo.com/v1",
		APIKey:            "test-key",
		Model:             "mimo-v2.5",
		ReasoningEffort:   "medium",
		ReasoningProtocol: "mimo-chat-completions",
		Messages:          []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mimoBody["reasoning_effort"] != "medium" || mapStringField(mimoBody["thinking"], "type") != "enabled" {
		t.Fatalf("MiMo chat reasoning protocol should use MiMo fields: %#v", mimoBody)
	}

	_, glmBody, _, err := buildHTTPRequest(Request{
		ProviderID:        "zai",
		Family:            "openai-compatible",
		EndpointFormat:    "chat_completions",
		BaseURL:           "https://zai.example/v1",
		APIKey:            "test-key",
		Model:             "glm-5",
		ReasoningEffort:   "off",
		ReasoningProtocol: "glm-chat-completions",
		Messages:          []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mapStringField(glmBody["thinking"], "type") != "disabled" || mapBoolField(glmBody["thinking"], "clear_thinking") != true {
		t.Fatalf("GLM chat reasoning protocol should use clear_thinking toggle: %#v", glmBody)
	}
	if _, ok := glmBody["reasoning_effort"]; ok {
		t.Fatalf("GLM chat reasoning protocol should not use reasoning_effort: %#v", glmBody)
	}

	_, responsesBody, _, err := buildHTTPRequest(Request{
		ProviderID:        "openai-responses",
		Family:            "openai-compatible",
		EndpointFormat:    "responses",
		BaseURL:           "https://openai.example/v1",
		APIKey:            "test-key",
		Model:             "gpt-5",
		ReasoningEffort:   "max",
		ReasoningProtocol: "openai-responses",
		Messages:          []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mapStringField(responsesBody["reasoning"], "effort") != "high" {
		t.Fatalf("OpenAI responses reasoning protocol should normalize max to high: %#v", responsesBody)
	}

	_, anthropicBody, _, err := buildHTTPRequest(Request{
		ProviderID:        "minimax",
		Family:            "anthropic-compatible",
		EndpointFormat:    "messages",
		BaseURL:           "https://api.minimaxi.com/anthropic",
		APIKey:            "test-key",
		Model:             "MiniMax-M3",
		ReasoningEffort:   "high",
		ReasoningProtocol: "anthropic-thinking",
		Messages:          []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mapStringField(anthropicBody["thinking"], "type") != "adaptive" ||
		mapStringField(anthropicBody["output_config"], "effort") != "high" {
		t.Fatalf("Anthropic thinking protocol should use thinking/output_config: %#v", anthropicBody)
	}
}

func TestRuntimeProviderConfigReadsModelReasoningProtocol(t *testing.T) {
	config := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "xiaomi-mimo",
			"providers": [{
				"id": "xiaomi-mimo",
				"apiKey": "test-provider-key",
				"baseUrl": "https://api.xiaomimimo.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["mimo-v2.5"],
				"modelProfiles": {
					"mimo-v2.5": {
						"inputModalities": ["text"],
						"messageParts": ["text"],
						"reasoning": {
							"supportedEfforts": ["off", "low", "medium", "high"],
							"defaultEffort": "high",
							"requestProtocol": "mimo-chat-completions"
						}
					}
				}
			}]
		}`,
	}).TurnConfig("xiaomi-mimo", "mimo-v2.5")
	if config.Family != "openai-compatible" ||
		config.ReasoningProtocol != "mimo-chat-completions" ||
		!reflect.DeepEqual(config.ReasoningSupportedEfforts, []string{"off", "low", "medium", "high"}) ||
		config.ReasoningDefaultEffort != "high" ||
		config.ReasoningEffort != "high" {
		t.Fatalf("provider config should preserve profile reasoning protocol without inheriting DeepSeek effort: %#v", config)
	}
}

func TestRuntimeProviderConfigResolvesModelProfileAliases(t *testing.T) {
	config := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "xiaomi",
			"providers": [{
				"id": "xiaomi",
				"apiKey": "test-provider-key",
				"baseUrl": "https://api.xiaomimimo.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["mimo-v2.5-pro", "mimo-v2.5"],
				"modelProfiles": {
					"mimo-v2.5-pro": {
						"aliases": ["mimo-v2.5-pro-ultraspeed"],
						"inputModalities": ["text"],
						"messageParts": ["text"],
						"reasoning": {
							"supportedEfforts": ["off", "low", "medium", "high"],
							"defaultEffort": "high",
							"requestProtocol": "mimo-chat-completions"
						},
						"price": {
							"input": 0.15,
							"output": 0.6,
							"currency": "CNY"
						}
					}
				}
			}]
		}`,
	}).TurnConfig("xiaomi", "mimo-v2.5-pro-ultraspeed")

	if config.ProviderID != "xiaomi" ||
		config.Model != "mimo-v2.5-pro" ||
		config.Family != "openai-compatible" ||
		config.EndpointFormat != "chat_completions" ||
		config.ReasoningProtocol != "mimo-chat-completions" ||
		!reflect.DeepEqual(config.ReasoningSupportedEfforts, []string{"off", "low", "medium", "high"}) ||
		config.ReasoningDefaultEffort != "high" ||
		config.ReasoningEffort != "high" {
		t.Fatalf("provider config should canonicalize MiMo alias through modelProfiles: %#v", config)
	}
	if config.DeepSeekPrefixEnhancement || config.CacheTelemetrySupported {
		t.Fatalf("MiMo alias should not inherit DeepSeek-specific behavior: %#v", config)
	}
	if config.Pricing == nil || config.Pricing.Output != 0.6 || config.Pricing.Currency != "CNY" {
		t.Fatalf("MiMo alias should use canonical model profile pricing: %#v", config.Pricing)
	}
}

func TestRuntimeProviderConfigRejectsCrossProviderModelDrift(t *testing.T) {
	configs := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "deepseek",
			"providers": [{
				"id": "deepseek",
				"apiKey": "test-deepseek-key",
				"baseUrl": "https://api.deepseek.com",
				"endpointFormat": "chat_completions",
				"models": ["deepseek-v4-flash", "deepseek-v4-pro"],
				"modelProfiles": {
					"deepseek-v4-pro": {"inputModalities": ["text"], "messageParts": ["text"]},
					"deepseek-v4-flash": {"inputModalities": ["text"], "messageParts": ["text"]}
				}
			}, {
				"id": "xiaomi",
				"apiKey": "test-xiaomi-key",
				"baseUrl": "https://api.xiaomimimo.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["mimo-v2.5-pro"]
			}, {
				"id": "open-catalog",
				"apiKey": "test-open-key",
				"baseUrl": "https://gateway.example/v1",
				"endpointFormat": "chat_completions"
			}]
		}`,
	})

	deepseek := configs.TurnConfig("deepseek", "mimo-v2-pro")
	if deepseek.ProviderID != "deepseek" || deepseek.Model != "deepseek-v4-flash" {
		t.Fatalf("DeepSeek must not pass a MiMo model through to the provider: %#v", deepseek)
	}

	xiaomi := configs.TurnConfig("xiaomi", "not-listed")
	if xiaomi.ProviderID != "xiaomi" || xiaomi.Model != "mimo-v2.5-pro" {
		t.Fatalf("bounded MiMo provider should fall back to its configured model: %#v", xiaomi)
	}

	openCatalog := configs.TurnConfig("open-catalog", "custom-live-model")
	if openCatalog.ProviderID != "open-catalog" || openCatalog.Model != "custom-live-model" {
		t.Fatalf("open provider catalogs must still allow custom live model ids: %#v", openCatalog)
	}
}

func TestRuntimeProviderExecutionConfigRequiresValidationForBoundedCatalogs(t *testing.T) {
	configs := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "xiaomi",
			"providers": [{
				"id": "xiaomi",
				"apiKey": "test-xiaomi-key",
				"baseUrl": "https://api.xiaomimimo.com/v1",
				"endpointFormat": "chat_completions",
				"models": ["mimo-v2.5-pro"]
			}]
		}`,
	})

	diagnosticConfig := configs.TurnConfig("xiaomi", "child-special")
	if diagnosticConfig.Model != "mimo-v2.5-pro" {
		t.Fatalf("ordinary provider config should retain bounded-catalog fallback: %#v", diagnosticConfig)
	}

	if err := configs.ValidateExecutionModel("xiaomi", "child-special"); err == nil {
		t.Fatalf("bounded execution model should be rejected before TurnConfigForExecution can preserve it")
	}

	openConfigs := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "custom-open",
			"providers": [{
				"id": "custom-open",
				"apiKey": "test-open-key",
				"baseUrl": "https://models.example/v1",
				"endpointFormat": "chat_completions"
			}]
		}`,
	})
	if err := openConfigs.ValidateExecutionModel("custom-open", "child-special"); err != nil {
		t.Fatalf("open provider catalogs must still allow custom live model ids: %v", err)
	}
	executionConfig := openConfigs.TurnConfigForExecution("custom-open", "child-special")
	if executionConfig.ProviderID != "custom-open" || executionConfig.Model != "child-special" {
		t.Fatalf("open runtime execution config must preserve explicit live model overrides: %#v", executionConfig)
	}
}

func TestRuntimeProviderConfigUsesModelProfileEndpointFormatOverride(t *testing.T) {
	configs := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: `{
			"defaultProviderId": "mixed-gateway",
			"providers": [{
				"id": "mixed-gateway",
				"apiKey": "test-provider-key",
				"baseUrl": "https://gateway.example/v1",
				"endpointFormat": "chat_completions",
				"models": ["gpt-chat", "gpt-responses", "claude-via-gateway"],
				"modelProfiles": {
					"gpt-responses": {
						"endpointFormat": "responses",
						"inputModalities": ["text"],
						"messageParts": ["text"]
					},
					"claude-via-gateway": {
						"endpointFormat": "messages",
						"inputModalities": ["text", "image"],
						"messageParts": ["text", "input_image"]
					}
				}
			}]
		}`,
	})

	chat := configs.TurnConfig("mixed-gateway", "gpt-chat")
	if chat.EndpointFormat != "chat_completions" || chat.Family != "openai-compatible" {
		t.Fatalf("provider default chat-completions format should remain intact: %#v", chat)
	}
	responses := configs.TurnConfig("mixed-gateway", "gpt-responses")
	if responses.EndpointFormat != "responses" || responses.Family != "openai-compatible" {
		t.Fatalf("model profile should override to OpenAI responses: %#v", responses)
	}
	responsesShape, err := BuildRequestShapeForTest(Request{
		ProviderID:     responses.ProviderID,
		Family:         responses.Family,
		EndpointFormat: responses.EndpointFormat,
		BaseURL:        responses.BaseURL,
		APIKey:         responses.APIKey,
		Model:          responses.Model,
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if responsesShape.RequestURL != "https://gateway.example/v1/responses" ||
		!sameStringSet(responsesShape.RequestBodyFields, []string{"input", "model", "stream"}) {
		t.Fatalf("responses profile should use responses request shape: %#v", responsesShape)
	}

	messages := configs.TurnConfig("mixed-gateway", "claude-via-gateway")
	if messages.EndpointFormat != "messages" ||
		messages.Family != "anthropic-compatible" ||
		!messages.SupportsImageInput ||
		!hasString(messages.MessageParts, "input_image") {
		t.Fatalf("model profile should override to Anthropic messages with profile capabilities: %#v", messages)
	}
	messagesShape, err := BuildRequestShapeForTest(Request{
		ProviderID:     messages.ProviderID,
		Family:         messages.Family,
		EndpointFormat: messages.EndpointFormat,
		BaseURL:        messages.BaseURL,
		APIKey:         messages.APIKey,
		Model:          messages.Model,
		Messages:       []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if messagesShape.RequestURL != "https://gateway.example/v1/messages" ||
		!sameStringSet(messagesShape.RequestBodyFields, []string{"max_tokens", "messages", "model", "stream"}) {
		t.Fatalf("messages profile should use Anthropic request shape: %#v", messagesShape)
	}
}

func mapStringField(value any, key string) string {
	if item, ok := value.(map[string]any); ok {
		if text, ok := item[key].(string); ok {
			return text
		}
	}
	return ""
}

func mapBoolField(value any, key string) bool {
	if item, ok := value.(map[string]any); ok {
		if flag, ok := item[key].(bool); ok {
			return flag
		}
	}
	return false
}

func TestDeepSeekChatRequestNeverReuploadsHistoricalPrivateToolReasoning(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "inspect file"},
		{
			Role: "assistant",
			Parts: []MessagePart{{
				Type: "reasoning",
				Text: "I need to read the file before answering.",
			}},
			ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
		},
		{Role: "tool", ToolCallID: "call_read", Content: "file contents"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:        "call_list",
				Name:      "list",
				Arguments: json.RawMessage(`{}`),
			}},
		},
		{Role: "tool", ToolCallID: "call_list", Content: "listed files"},
		{
			Role: "assistant",
			Parts: []MessagePart{{
				Type: "reasoning",
				Text: "ordinary assistant reasoning should not upload without a tool call",
			}},
			Content: "plain answer",
		},
	}
	_, deepseekBody, _, err := buildHTTPRequest(Request{
		ProviderID:        "deepseek",
		Family:            "deepseek",
		EndpointFormat:    "chat_completions",
		ReasoningProtocol: "deepseek-chat-completions",
		ReasoningEffort:   "off",
		BaseURL:           "https://api.deepseek.com",
		APIKey:            "test-key",
		Model:             "deepseek-chat",
		Messages:          messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(deepseekBody)
	text := string(raw)
	if strings.Contains(text, `"reasoning_content"`) {
		t.Fatalf("DeepSeek historical tool-call messages must not synthesize reasoning_content: %s", text)
	}
	for _, privateReasoning := range []string{"I need to read the file before answering.", "ordinary assistant reasoning should not upload"} {
		if strings.Contains(text, privateReasoning) {
			t.Fatalf("private reasoning must not be uploaded in DeepSeek history: %s", text)
		}
	}

	_, openaiBody, _, err := buildHTTPRequest(Request{
		ProviderID:     "openai-compatible",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        "https://openai.example/v1",
		APIKey:         "test-key",
		Model:          "gpt-compatible",
		Messages:       messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(openaiBody)
	if strings.Contains(string(raw), "reasoning_content") {
		t.Fatalf("non-DeepSeek OpenAI-compatible provider must not receive DeepSeek reasoning_content: %s", string(raw))
	}
}

func TestDeepSeekCustomEndpointNeverReuploadsPrivateToolReasoning(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "inspect file"},
		{
			Role: "assistant",
			Parts: []MessagePart{{
				Type: "reasoning",
				Text: "Need to inspect before calling the tool.",
			}},
			ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}},
		},
		{Role: "tool", ToolCallID: "call_read", Content: "note contents"},
	}
	_, deepseekBody, _, err := buildHTTPRequest(Request{
		ProviderID:        "deepseek-custom",
		Family:            "custom_endpoint",
		EndpointFormat:    "custom_endpoint",
		ReasoningProtocol: "deepseek-chat-completions",
		ReasoningEffort:   "off",
		BaseURL:           "https://api.deepseek.com/custom/chat/completions",
		APIKey:            "test-key",
		Model:             "deepseek-chat",
		Messages:          messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(deepseekBody)
	if strings.Contains(string(raw), `"reasoning_content"`) || strings.Contains(string(raw), "Need to inspect before calling the tool.") {
		t.Fatalf("DeepSeek custom endpoint must not synthesize or copy historical reasoning: %s", string(raw))
	}

	_, customBody, _, err := buildHTTPRequest(Request{
		ProviderID:     "custom-openai",
		Family:         "custom_endpoint",
		EndpointFormat: "custom_endpoint",
		BaseURL:        "https://provider.example/custom/chat/completions",
		APIKey:         "test-key",
		Model:          "custom-chat",
		Messages:       messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(customBody)
	if strings.Contains(string(raw), "reasoning_content") {
		t.Fatalf("non-DeepSeek custom endpoint must not receive DeepSeek reasoning_content: %s", string(raw))
	}
}

func TestCustomEndpointUsesFullPathRequestShape(t *testing.T) {
	tool := ToolSchema{
		Name:        "search_issues",
		Description: "Search issues",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
	}
	cases := []struct {
		name       string
		baseURL    string
		fields     []string
		headers    []string
		bodyAssert func(t *testing.T, body map[string]any)
	}{
		{
			name:    "responses",
			baseURL: "https://gateway.example/custom-path/responses",
			fields:  []string{"input", "model", "stream", "tools"},
			headers: []string{"Accept", "Authorization", "Content-Type"},
			bodyAssert: func(t *testing.T, body map[string]any) {
				t.Helper()
				if _, ok := body["messages"]; ok {
					t.Fatalf("custom /responses must not send chat messages body: %#v", body)
				}
				if _, ok := body["input"]; !ok {
					t.Fatalf("custom /responses must send Responses input body: %#v", body)
				}
			},
		},
		{
			name:    "messages",
			baseURL: "https://gateway.example/custom-path/messages",
			fields:  []string{"max_tokens", "messages", "model", "stream", "system", "tools"},
			headers: []string{"Accept", "Authorization", "Content-Type", "anthropic-version", "x-api-key"},
			bodyAssert: func(t *testing.T, body map[string]any) {
				t.Helper()
				if _, ok := body["input"]; ok {
					t.Fatalf("custom /messages must not send Responses input body: %#v", body)
				}
				if _, ok := body["system"]; !ok {
					t.Fatalf("custom /messages should preserve Anthropic system body: %#v", body)
				}
			},
		},
		{
			name:    "chat completions",
			baseURL: "https://gateway.example/custom-path/chat/completions",
			fields:  []string{"messages", "model", "stream", "stream_options", "tools"},
			headers: []string{"Accept", "Authorization", "Content-Type"},
			bodyAssert: func(t *testing.T, body map[string]any) {
				t.Helper()
				if _, ok := body["input"]; ok {
					t.Fatalf("custom /chat/completions must not send Responses input body: %#v", body)
				}
				if _, ok := body["stream_options"]; !ok {
					t.Fatalf("custom /chat/completions should include stream usage options: %#v", body)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, body, headers, err := buildHTTPRequest(Request{
				ProviderID:     "custom-provider",
				Family:         "custom_endpoint",
				EndpointFormat: "custom_endpoint",
				BaseURL:        tc.baseURL,
				APIKey:         "test-key",
				Model:          "custom-model",
				Messages: []Message{
					{Role: "system", Content: "You are analytix."},
					{Role: "user", Content: "hello"},
				},
				Tools: []ToolSchema{tool},
			})
			if err != nil {
				t.Fatal(err)
			}
			if endpoint != tc.baseURL {
				t.Fatalf("custom endpoint must keep the exact full path, got %q", endpoint)
			}
			if !sameStringSet(sortedMapKeys(body), tc.fields) {
				t.Fatalf("custom %s body fields mismatch: %#v", tc.name, body)
			}
			if !sameStringSet(sortedMapKeysString(headers), tc.headers) {
				t.Fatalf("custom %s headers mismatch: %#v", tc.name, headers)
			}
			tc.bodyAssert(t, body)
		})
	}
}

func TestHTTPProviderClientParsesCustomFullEndpointStreamsByPath(t *testing.T) {
	t.Run("responses", func(t *testing.T) {
		var body string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			body = string(data)
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			_, _ = w.Write([]byte(`data: {"type":"response.output_text.delta","delta":"custom responses ok"}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}` + "\n\n"))
		}))
		defer server.Close()

		client := NewHTTPProviderClient(server.Client())
		result, err := client.Stream(context.Background(), Request{
			ProviderID:     "custom-responses",
			Family:         "custom_endpoint",
			EndpointFormat: "custom_endpoint",
			BaseURL:        server.URL + "/custom/responses",
			APIKey:         "test-key",
			Model:          "custom-responses-model",
			Messages:       []Message{{Role: "user", Content: "hello"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := collectChunkText(result.Chunks, ChunkText); got != "custom responses ok" {
			t.Fatalf("custom /responses stream parsed with wrong protocol: %q", got)
		}
		if !strings.Contains(body, `"input"`) || strings.Contains(body, `"messages"`) {
			t.Fatalf("custom /responses sent wrong request body: %s", body)
		}
	})

	t.Run("messages", func(t *testing.T) {
		var body string
		var xAPIKey string
		var auth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			body = string(data)
			xAPIKey = r.Header.Get("x-api-key")
			auth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			_, _ = w.Write([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":4}}}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"custom messages ok"}}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"type":"message_delta","usage":{"output_tokens":2}}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"type":"message_stop"}` + "\n\n"))
		}))
		defer server.Close()

		client := NewHTTPProviderClient(server.Client())
		result, err := client.Stream(context.Background(), Request{
			ProviderID:     "custom-messages",
			Family:         "custom_endpoint",
			EndpointFormat: "custom_endpoint",
			BaseURL:        server.URL + "/custom/messages",
			APIKey:         "test-key",
			Model:          "custom-messages-model",
			Messages: []Message{
				{Role: "system", Content: "You are analytix."},
				{Role: "user", Content: "hello"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := collectChunkText(result.Chunks, ChunkText); got != "custom messages ok" {
			t.Fatalf("custom /messages stream parsed with wrong protocol: %q", got)
		}
		if !strings.Contains(body, `"messages"`) || strings.Contains(body, `"input"`) {
			t.Fatalf("custom /messages sent wrong request body: %s", body)
		}
		if xAPIKey != "test-key" || auth != "Bearer test-key" {
			t.Fatalf("custom /messages should support both Anthropic and gateway auth headers: x-api-key=%q auth=%q", xAPIKey, auth)
		}
	})
}

func TestOpenAIChatToolCallOnlyAssistantUsesNullContent(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "inspect file"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}},
		},
		{Role: "tool", ToolCallID: "call_read", Content: "note contents"},
	}
	_, body, _, err := buildHTTPRequest(Request{
		ProviderID:     "openai-compatible",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        "https://openai.example/v1",
		APIKey:         "test-key",
		Model:          "gpt-compatible",
		Messages:       messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	wireMessages, ok := body["messages"].([]map[string]any)
	if !ok || len(wireMessages) != 3 {
		t.Fatalf("unexpected OpenAI chat messages: %#v", body["messages"])
	}
	assistant := wireMessages[1]
	if value, ok := assistant["content"]; !ok || value != nil {
		t.Fatalf("tool-call-only assistant must carry explicit content:null, got %#v", assistant)
	}
	if _, ok := assistant["tool_calls"]; !ok {
		t.Fatalf("assistant message lost tool_calls: %#v", assistant)
	}
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"content":null`) {
		t.Fatalf("serialized OpenAI-compatible chat body must include content:null: %s", string(raw))
	}
}

func TestDeepSeekHistoricalToolPairingPreservesNullContentWithoutPrivateReasoning(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "inspect file"},
		{
			Role: "assistant",
			Parts: []MessagePart{{
				Type: "reasoning",
				Text: "Need the file.",
			}},
			ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}},
		},
		{Role: "tool", ToolCallID: "call_read", Content: "note contents"},
	}
	_, body, _, err := buildHTTPRequest(Request{
		ProviderID:        "deepseek",
		Family:            "deepseek",
		EndpointFormat:    "chat_completions",
		ReasoningProtocol: "deepseek-chat-completions",
		ReasoningEffort:   "off",
		BaseURL:           "https://api.deepseek.com",
		APIKey:            "test-key",
		Model:             "deepseek-chat",
		Messages:          messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(body)
	text := string(raw)
	if !strings.Contains(text, `"content":null`) ||
		strings.Contains(text, `"reasoning_content"`) ||
		!strings.Contains(text, `"tool_calls"`) || strings.Contains(text, "Need the file.") {
		t.Fatalf("DeepSeek historical tool pairing must omit private reasoning: %s", text)
	}
}

func TestNormalizeToolPairingFastPathKeepsHealthyHistoryUnchanged(t *testing.T) {
	callID := providerTestHostToolCallID("fast-path-read")
	messages := []Message{
		{Role: "user", Content: "inspect file"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:        callID,
				Name:      "read",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}},
		},
		{Role: "tool", ToolCallID: callID, Content: "note contents"},
		{Role: "assistant", Content: "done"},
	}

	sanitized := SanitizeToolPairing(messages)
	if len(sanitized) != len(messages) || &sanitized[0] != &messages[0] {
		t.Fatalf("healthy provider history should use the fast path without rewriting: %#v", sanitized)
	}
	session := NormalizeSessionMessages(messages)
	if len(session) != len(messages) || &session[0] != &messages[0] {
		t.Fatalf("healthy session history should use the fast path without rewriting: %#v", session)
	}
	if got := string(sanitized[1].ToolCalls[0].Arguments); got != `{"path":"note.txt"}` {
		t.Fatalf("healthy history arguments should not be reserialized, got %s", got)
	}
}

func TestCapturePrefixShapeToolSchemaReorderKeepsStableHash(t *testing.T) {
	left := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools: []ToolSchema{
			{Name: "b_tool", Description: "B", Parameters: json.RawMessage(`{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"number"}}}`)},
			{Name: "a_tool", Description: "A", Parameters: json.RawMessage(`{"properties":{"q":{"type":"string"}},"type":"object"}`)},
		},
	})
	right := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools: []ToolSchema{
			{Name: "a_tool", Description: "A", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)},
			{Name: "b_tool", Description: "B", Parameters: json.RawMessage(`{"properties":{"a":{"type":"number"},"z":{"type":"string"}},"type":"object"}`)},
		},
	})

	if left.ToolsHash != right.ToolsHash || left.PrefixHash != right.PrefixHash {
		t.Fatalf("tool order/schema key reorder should keep prefix hashes stable: %#v != %#v", left, right)
	}
}

func TestCanonicalJSONSchemaCleansProviderToolSchemas(t *testing.T) {
	got := CanonicalJSONSchema(json.RawMessage(`{
		"type":"object",
		"properties":{
			"mode":{
				"type":["string","null"],
				"enum":["beta","alpha","beta"],
				"required":true
			}
		},
		"required":["beta","alpha","beta"],
		"dependentRequired":{"mode":["path","mode","path"],"skip":true},
		"additionalProperties":false
	}`))
	want := `{"additionalProperties":false,"dependentRequired":{"mode":["mode","path"]},"properties":{"mode":{"enum":["alpha","beta","beta"],"type":["null","string"]}},"required":["alpha","beta"],"type":"object"}`
	if got != want {
		t.Fatalf("provider schema canonicalization mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestProviderToolBodiesUseSchemaCanonicalization(t *testing.T) {
	tool := ToolSchema{
		Name:        "read_file",
		Description: "Preserve exact user arguments",
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{"path":{"type":"string","required":true},"workspace":{"type":"string","description":"Omit unless explicitly requested"}},
			"required":["path","path"],
			"dependentRequired":{"path":["workspace","path"]},
			"additionalProperties":false
		}`),
	}
	bodies := map[string]string{
		"openai":    string(mustProviderJSON(t, providerschema.OpenAITools([]ToolSchema{tool}))),
		"anthropic": string(mustProviderJSON(t, providerschema.AnthropicTools([]ToolSchema{tool}, false))),
	}
	for providerName, body := range bodies {
		if strings.Contains(body, `"required":true`) {
			t.Fatalf("%s provider-visible tool schema leaked invalid required=true: %s", providerName, body)
		}
		if !strings.Contains(body, `"required":["path"]`) {
			t.Fatalf("%s provider-visible tool schema should dedupe required fields: %s", providerName, body)
		}
		if !strings.Contains(body, `"dependentRequired":{"path":["path","workspace"]}`) {
			t.Fatalf("%s provider-visible tool schema should sort dependentRequired fields: %s", providerName, body)
		}
		if !strings.Contains(body, `"additionalProperties":false`) {
			t.Fatalf("%s provider-visible tool schema became open: %s", providerName, body)
		}
		if !strings.Contains(body, `"description":"Preserve exact user arguments"`) ||
			!strings.Contains(body, `"description":"Omit unless explicitly requested"`) {
			t.Fatalf("%s provider-visible tool descriptions were stripped: %s", providerName, body)
		}
	}
}

func TestProviderToolBodiesPreserveBuiltinBashArgumentGuidance(t *testing.T) {
	var bash ToolSchema
	for _, tool := range toolcatalogapp.BuiltinToolSchemas(toolcatalogapp.BuiltinToolSchemaInput{
		AllowBackgroundBash: true,
	}) {
		if tool.Name == "bash" {
			bash = tool
			break
		}
	}
	if bash.Name == "" {
		t.Fatal("builtin bash schema is unavailable")
	}
	for providerName, body := range map[string]string{
		"openai":    string(mustProviderJSON(t, providerschema.OpenAITools([]ToolSchema{bash}))),
		"anthropic": string(mustProviderJSON(t, providerschema.AnthropicTools([]ToolSchema{bash}, false))),
	} {
		for _, guidance := range []string{
			"copy it unchanged",
			"runtime default and maximum are 120 seconds",
			"Omit unless the user explicitly requests background execution",
		} {
			if !strings.Contains(body, guidance) {
				t.Fatalf("%s provider-visible builtin bash guidance lost %q: %s", providerName, guidance, body)
			}
		}
	}
}

func TestAnthropicToolsUseBoundedCacheBreakpoints(t *testing.T) {
	tools := []ToolSchema{
		{Name: "first", Description: "First tool", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "second", Description: "Second tool", Parameters: json.RawMessage(`{"type":"object"}`)},
		{Name: "third", Description: "Third tool", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	body := string(mustProviderJSON(t, providerschema.AnthropicTools(tools, true)))
	if count := strings.Count(body, `"cache_control"`); count != 1 {
		t.Fatalf("Anthropic tool cache breakpoints should stay bounded to one tool breakpoint, got %d in %s", count, body)
	}
	if !strings.Contains(body, `"name":"third"`) {
		t.Fatalf("Anthropic tool schema lost the final tool: %s", body)
	}
}

func TestCapturePrefixShapeUsesProviderSchemaCanonicalization(t *testing.T) {
	left := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools: []ToolSchema{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: json.RawMessage(`{
				"type":"object",
				"properties":{"path":{"type":["string","null"],"enum":["workspace","local"],"required":true}},
				"required":["path","path"],
				"dependentRequired":{"path":["workspace","path","workspace"]}
			}`),
		}},
	})
	right := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools: []ToolSchema{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: json.RawMessage(`{
				"dependentRequired":{"path":["path","workspace"]},
				"properties":{"path":{"enum":["local","workspace"],"type":["null","string"]}},
				"required":["path"],
				"type":"object"
			}`),
		}},
	})

	if left.ToolsHash != right.ToolsHash || left.PrefixHash != right.PrefixHash {
		t.Fatalf("provider-visible schema canonicalization should stabilize prefix hashes: %#v != %#v", left, right)
	}
}

func TestCapturePrefixShapeSeparatesToolSchemaAndSourceDiagnostics(t *testing.T) {
	baseTools := []ToolSchema{{
		Name:        "read",
		Description: "Read files",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Source:      "builtin",
	}, {
		Name:        "mcp_example",
		Description: "Tool from MCP",
		Parameters:  json.RawMessage(`{"type":"object","additionalProperties":true}`),
		Source:      "mcp",
	}}
	left := CapturePrefixShape(Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Model:          "deepseek-chat",
		SystemPrompt:   "stable system",
		Tools:          baseTools,
	})
	sourceOnlyChanged := append([]ToolSchema(nil), baseTools...)
	sourceOnlyChanged[1].Source = "skill"
	right := CapturePrefixShape(Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Model:          "deepseek-chat",
		SystemPrompt:   "stable system",
		Tools:          sourceOnlyChanged,
	})

	if left.ToolsHash != right.ToolsHash || left.PrefixHash != right.PrefixHash {
		t.Fatalf("tool source metadata must not perturb provider-visible prefix: %#v != %#v", left, right)
	}
	if left.ToolSourcesHash == right.ToolSourcesHash {
		t.Fatalf("tool source diagnostics should change when source ids change: %#v == %#v", left, right)
	}
	if !sameStringSet(left.ToolSourceIDs, []string{"builtin", "mcp"}) ||
		!sameStringSet(right.ToolSourceIDs, []string{"builtin", "skill"}) {
		t.Fatalf("unexpected tool source ids: left=%#v right=%#v", left.ToolSourceIDs, right.ToolSourceIDs)
	}
	diagnostics := RuntimeCacheDiagnosticsWithPrevious(left, Result{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		PrefixShape:    right,
	})
	if diagnostics["toolSourceChanged"] != true {
		t.Fatalf("source-only changes should be visible in diagnostics: %#v", diagnostics)
	}
	reasons, ok := diagnostics["toolSourceChangeReasons"].([]string)
	if !ok || !runtimeCacheDiagnosticsReasonContains(reasons, "tool_sources") {
		t.Fatalf("source-only changes should include tool_sources reason: %#v", diagnostics)
	}
	if !sameStringSet(diagnostics["toolSourceIds"].([]string), []string{"builtin", "skill"}) {
		t.Fatalf("diagnostics should expose current tool source ids: %#v", diagnostics)
	}
}

func TestCapturePrefixShapeIgnoresDynamicTurnTail(t *testing.T) {
	stableTools := []ToolSchema{
		{Name: "read_file", Description: "Read a workspace file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
	}
	left := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools:        stableTools,
		Messages: []Message{
			{Role: "user", Content: "selected text at 2026-06-26T01:00:00Z"},
			{Role: "user", Content: "clipboard image OCR v1"},
		},
	})
	right := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools:        stableTools,
		Messages: []Message{
			{Role: "user", Content: "selected text at 2026-06-26T01:00:30Z"},
			{Role: "user", Content: "clipboard image OCR v2"},
		},
	})

	if left.PrefixHash != right.PrefixHash || left.SystemHash != right.SystemHash || left.ToolsHash != right.ToolsHash {
		t.Fatalf("dynamic turn tail must not perturb stable cache prefix: %#v != %#v", left, right)
	}
	if left.PrefixItemsHash != right.PrefixItemsHash {
		t.Fatalf("dynamic turn tail must not perturb completed history prefix hash: %#v != %#v", left, right)
	}
	if left.DynamicStateLeaked || right.DynamicStateLeaked {
		t.Fatalf("message tail changes should not be marked as stable-prefix leakage: %#v %#v", left, right)
	}
}

func TestCapturePrefixShapeTracksCompletedHistoryPrefixItems(t *testing.T) {
	stableTools := []ToolSchema{
		{Name: "read_file", Description: "Read a workspace file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
	}
	left := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools:        stableTools,
		Messages: []Message{
			{Role: "system", Content: "stable system"},
			{Role: "user", Content: "What is in note.txt?"},
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}}},
			{Role: "tool", ToolCallID: "call_read", Content: "alpha"},
			{Role: "assistant", Content: "note.txt says alpha"},
			{Role: "user", Content: "selected text at 2026-06-26T01:00:00Z"},
		},
	})
	rightSameCompletedHistory := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools:        stableTools,
		Messages: []Message{
			{Role: "system", Content: "stable system"},
			{Role: "user", Content: "What is in note.txt?"},
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}}},
			{Role: "tool", ToolCallID: "call_read", Content: "alpha"},
			{Role: "assistant", Content: "note.txt says alpha"},
			{Role: "user", Content: "clipboard OCR changed at 2026-06-26T01:01:00Z"},
		},
	})
	rightChangedCompletedHistory := CapturePrefixShape(Request{
		Family:       "deepseek",
		SystemPrompt: "stable system",
		Tools:        stableTools,
		Messages: []Message{
			{Role: "system", Content: "stable system"},
			{Role: "user", Content: "What is in note.txt?"},
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"note.txt"}`),
			}}},
			{Role: "tool", ToolCallID: "call_read", Content: "beta"},
			{Role: "assistant", Content: "note.txt says beta"},
			{Role: "user", Content: "selected text at 2026-06-26T01:00:00Z"},
		},
	})

	if left.PrefixItemsHash == "" {
		t.Fatalf("completed history prefix hash should be populated: %#v", left)
	}
	if left.PrefixItemsHash != rightSameCompletedHistory.PrefixItemsHash {
		t.Fatalf("trailing user tail should not perturb completed history prefix hash: %#v != %#v", left, rightSameCompletedHistory)
	}
	if left.PrefixItemsHash == rightChangedCompletedHistory.PrefixItemsHash {
		t.Fatalf("completed history changes should perturb prefixItemsHash: %#v == %#v", left, rightChangedCompletedHistory)
	}
	if left.PrefixHash != rightChangedCompletedHistory.PrefixHash {
		t.Fatalf("provider stable system/tools prefix hash should remain separate from history diagnostics: %#v != %#v", left, rightChangedCompletedHistory)
	}
	diagnostics := RuntimeCacheDiagnosticsWithPrevious(left, Result{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		PrefixShape:    rightChangedCompletedHistory,
	})
	if diagnostics["prefixItemsHash"] != rightChangedCompletedHistory.PrefixItemsHash {
		t.Fatalf("diagnostics should expose the current prefixItemsHash: %#v", diagnostics)
	}
	reasons, ok := diagnostics["prefixChangeReasons"].([]string)
	if !ok || !runtimeCacheDiagnosticsReasonContains(reasons, "prefix") {
		t.Fatalf("completed history churn should be attributed to prefix: %#v", diagnostics)
	}
}

func TestRuntimeCacheDiagnosticsComparesPreviousPrefixShape(t *testing.T) {
	base := CapturePrefixShape(Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Model:          "deepseek-chat",
		Route:          "agent",
		SystemPrompt:   "stable system",
		Tools: []ToolSchema{{
			Name:        "read",
			Description: "Read files",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	first := RuntimeCacheDiagnosticsWithPrevious(PrefixShape{}, Result{
		ProviderID:           "deepseek",
		Family:               "deepseek",
		EndpointFormat:       "chat_completions",
		PrefixShape:          base,
		FirstTokenLatencyMs:  12,
		HasFirstTokenLatency: true,
		DurationMs:           34,
		HasDuration:          true,
	})
	if first["prefixChanged"] != false {
		t.Fatalf("first prefix diagnostics should not report churn: %#v", first)
	}
	if first["route"] != "agent" || first["toolCount"] != 1 {
		t.Fatalf("first prefix diagnostics should expose prompt route and tool count: %#v", first)
	}
	if first["firstTokenLatencyMs"] != int64(12) || first["durationMs"] != int64(34) {
		t.Fatalf("first prefix diagnostics should expose latency timing: %#v", first)
	}

	changed := CapturePrefixShape(Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		Model:          "deepseek-chat",
		Route:          "agent",
		SystemPrompt:   "stable system",
		Tools: []ToolSchema{{
			Name:        "write",
			Description: "Write files",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	second := RuntimeCacheDiagnosticsWithPrevious(base, Result{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		PrefixShape:    changed,
		Usage:          Usage{CacheHitTokens: 4, CacheMissTokens: 6},
	})
	if second["prefixChanged"] != true || second["toolSourceChanged"] != true {
		t.Fatalf("changed tool schema should report prefix/tool-source churn: %#v", second)
	}
	reasons, ok := second["prefixChangeReasons"].([]string)
	if !ok || !runtimeCacheDiagnosticsReasonContains(reasons, "tools") {
		t.Fatalf("changed tool schema should include tools reason: %#v", second)
	}
	if second["cacheHitTokens"] != 4 || second["cacheMissTokens"] != 6 {
		t.Fatalf("DeepSeek cache token diagnostics missing: %#v", second)
	}
}

func TestRuntimeCacheDiagnosticsIncludesOpenAICompatibleCachedTokens(t *testing.T) {
	shape := CapturePrefixShape(Request{
		ProviderID:     "openai-compatible",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		Model:          "gpt-compatible",
		SystemPrompt:   "stable system",
		Tools: []ToolSchema{{
			Name:        "read",
			Description: "Read files",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	diagnostics := RuntimeCacheDiagnosticsWithPrevious(PrefixShape{}, Result{
		ProviderID:     "openai-compatible",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		PrefixShape:    shape,
		Usage:          Usage{CacheHitTokens: 64, CacheMissTokens: 36},
	})
	if diagnostics["cacheTelemetrySupported"] != true ||
		diagnostics["cacheTelemetryPresent"] != true ||
		diagnostics["providerNativeCacheTelemetry"] != true ||
		diagnostics["cacheTelemetrySource"] != "provider_usage" {
		t.Fatalf("OpenAI-compatible cache diagnostics should expose parsed provider usage: %#v", diagnostics)
	}
	if diagnostics["cacheHitTokens"] != 64 {
		t.Fatalf("OpenAI-compatible diagnostics should expose native cacheHitTokens: %#v", diagnostics)
	}
	if diagnostics["cacheMissTokens"] != 36 {
		t.Fatalf("OpenAI-compatible diagnostics should expose native cacheMissTokens: %#v", diagnostics)
	}
}

func TestParseDeepSeekPromptCacheUsageRejectsNativeNestedConflict(t *testing.T) {
	payload := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":93,"prompt_tokens_details":{"cached_tokens":99},"completion_tokens_details":{"reasoning_tokens":2}}}`
	if _, _, _, _, err := parseOpenAIChatSSEPayload(payload, &toolCallAccumulator{}, &thinkSplitter{}); err == nil {
		t.Fatal("conflicting native and nested cache usage was accepted")
	}
}

func TestParseDeepSeekPromptCacheUsageAcceptsConsistentNativeFields(t *testing.T) {
	payload := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":93,"prompt_tokens_details":{"cached_tokens":7},"completion_tokens_details":{"reasoning_tokens":2}}}`
	chunks, usage, done, finishReason, err := parseOpenAIChatSSEPayload(payload, &toolCallAccumulator{}, &thinkSplitter{})
	if err != nil {
		t.Fatal(err)
	}
	if !done || finishReason != "" {
		t.Fatalf("usage payload should be terminal without a finish reason: done=%v finish=%q", done, finishReason)
	}
	if usage.CacheHitTokens != 7 || usage.CacheMissTokens != 93 || usage.ReasoningTokens != 2 {
		t.Fatalf("consistent native DeepSeek cache fields were not preserved: %#v", usage)
	}
	if len(chunks) != 1 || chunks[0].Kind != ChunkUsage || chunks[0].Usage.CacheHitTokens != 7 || chunks[0].Usage.CacheMissTokens != 93 {
		t.Fatalf("usage chunk missing native cache tokens: %#v", chunks)
	}
}

func TestParseOpenAICompatiblePromptCacheUsageFallsBackToNestedCachedTokens(t *testing.T) {
	payload := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_tokens_details":{"cached_tokens":64}}}`
	_, usage, done, _, err := parseOpenAIChatSSEPayload(payload, &toolCallAccumulator{}, &thinkSplitter{})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("usage payload should be terminal")
	}
	if usage.CacheHitTokens != 64 || usage.CacheMissTokens != 36 {
		t.Fatalf("OpenAI-compatible nested cached_tokens fallback mismatch: %#v", usage)
	}
	if !usage.HasCacheTelemetry() {
		t.Fatalf("nested cached_tokens must mark cache telemetry present: %#v", usage)
	}
}

func TestParseOpenAICompatibleUsageKeepsAbsentCacheTelemetryUnknown(t *testing.T) {
	payload := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105}}`
	_, usage, done, _, err := parseOpenAIChatSSEPayload(payload, &toolCallAccumulator{}, &thinkSplitter{})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("usage payload should be terminal")
	}
	if usage.HasCacheTelemetry() || usage.CacheHitTokens != 0 || usage.CacheMissTokens != 0 || usage.CacheHitRate != 0 {
		t.Fatalf("absent cache telemetry must stay unknown rather than becoming zero telemetry: %#v", usage)
	}
}

func TestParseOpenAICompatibleZeroCachedTokensIsKnownCacheTelemetry(t *testing.T) {
	payload := `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_tokens_details":{"cached_tokens":0}}}`
	_, usage, done, _, err := parseOpenAIChatSSEPayload(payload, &toolCallAccumulator{}, &thinkSplitter{})
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("usage payload should be terminal")
	}
	if !usage.HasCacheTelemetry() || usage.CacheHitTokens != 0 || usage.CacheMissTokens != 100 {
		t.Fatalf("cached_tokens=0 is known telemetry and should count prompt tokens as misses: %#v", usage)
	}
}

func runtimeCacheDiagnosticsReasonContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestApplyPricingMarksConfiguredAndLeavesMissingPriceUnknown(t *testing.T) {
	usage := Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110}
	unpriced := ApplyPricing(usage, nil)
	if unpriced.PriceConfigured || unpriced.CostUSD != 0 || unpriced.CostCNY != 0 {
		t.Fatalf("missing provider price must remain unconfigured without synthetic zero-cost pricing: %#v", unpriced)
	}

	priced := ApplyPricing(usage, &Pricing{Input: 0, Output: 0, Currency: "USD"})
	if !priced.PriceConfigured || priced.CostUSD != 0 || priced.Currency != "USD" {
		t.Fatalf("explicit zero provider price should stay configured with zero cost: %#v", priced)
	}
}

func TestRuntimeProviderConfigSetSourceDoesNotCarryFakeProviderRouting(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(cwd, "provider.go"))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(source)
	for _, forbidden := range []string{"FakeProviderBaseURL", "RuntimeTurnConfig", "live-local", "test-local"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("production provider config set must not carry fake/local routing marker %q", forbidden)
		}
	}
}

func TestSanitizeToolPairingBackfillsDanglingToolCall(t *testing.T) {
	callID := providerTestHostToolCallID("dangling-ls")
	out := SanitizeToolPairing([]Message{
		{Role: "user", Content: "list files"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: callID, Name: "ls", Arguments: json.RawMessage(`{"path":"."`)}}},
		{Role: "user", Content: "never mind"},
	})
	if len(out) != 4 {
		t.Fatalf("unexpected repaired history length: %#v", out)
	}
	if out[1].Role != "assistant" || string(out[1].ToolCalls[0].Arguments) != `{"path":"."}` {
		t.Fatalf("assistant tool call should be preserved with repaired args: %#v", out[1])
	}
	if out[2].Role != "tool" || out[2].ToolCallID != callID || out[2].Content != interruptedToolResult {
		t.Fatalf("dangling tool call should be answered by placeholder result: %#v", out)
	}
	if out[3].Role != "user" || out[3].Content != "never mind" {
		t.Fatalf("ordinary history after dangling call should be preserved: %#v", out)
	}
}

func TestSanitizeToolPairingOrdersMultipleToolResultsAndDropsOrphans(t *testing.T) {
	callA := providerTestHostToolCallID("ordered-a")
	callB := providerTestHostToolCallID("ordered-b")
	callC := providerTestHostToolCallID("ordered-c")
	out := SanitizeToolPairing([]Message{
		{Role: "tool", ToolCallID: "orphan", Content: "drop me"},
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: callA, Name: "read", Arguments: json.RawMessage(`{"path":"a"}`)},
			{ID: callB, Name: "read", Arguments: json.RawMessage(`{"path":"b"}`)},
			{ID: callC, Name: "read", Arguments: json.RawMessage(`{"path":"c"}`)},
		}},
		{Role: "tool", ToolCallID: callB, Content: "B"},
		{Role: "tool", ToolCallID: callA, Content: "A"},
	})
	if len(out) != 4 {
		t.Fatalf("unexpected repaired history length: %#v", out)
	}
	gotOrder := []string{out[1].ToolCallID, out[2].ToolCallID, out[3].ToolCallID}
	if !sameChunkKindSlice(gotOrder, []string{callA, callB, callC}) {
		t.Fatalf("tool results should be paired in call order: %#v", out)
	}
	if out[3].Content != interruptedToolResult {
		t.Fatalf("missing third result should be backfilled: %#v", out[3])
	}
}

func TestSanitizeToolPairingLeavesWellFormedHistoryOnFastPath(t *testing.T) {
	callID := providerTestHostToolCallID("well-formed-ls")
	in := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "list"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: callID, Name: "ls", Arguments: json.RawMessage(`{"path":"."}`)}}},
		{Role: "tool", ToolCallID: callID, Content: "main.go"},
		{Role: "assistant", Content: "done"},
	}
	out := SanitizeToolPairing(in)
	if len(out) != len(in) {
		t.Fatalf("well-formed history should not change length: %#v", out)
	}
	if &out[0] != &in[0] {
		t.Fatalf("well-formed history should return the original backing slice")
	}
}

func TestSanitizeToolPairingBackfillsEmptyToolCallNameFromResult(t *testing.T) {
	callA := providerTestHostToolCallID("backfill-a")
	callB := providerTestHostToolCallID("backfill-b")
	in := []Message{
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: callA, Arguments: json.RawMessage(`{"path":"a.txt"}`)},
			{ID: callB, Name: "grep", Arguments: json.RawMessage(`{"pattern":"x"}`)},
		}},
		{Role: "tool", Name: "read_file", ToolCallID: callA, Content: "A"},
		{Role: "tool", Name: "grep", ToolCallID: callB, Content: "B"},
	}
	out := SanitizeToolPairing(in)
	if out[0].ToolCalls[0].Name != "read_file" || out[0].ToolCalls[1].Name != "grep" {
		t.Fatalf("empty tool-call names should backfill from paired tool results: %#v", out[0].ToolCalls)
	}
	if in[0].ToolCalls[0].Name != "" {
		t.Fatalf("stored history mutated: %#v", in[0].ToolCalls)
	}
}

func TestSanitizeToolPairingRejectsMissingHostIDs(t *testing.T) {
	in := []Message{
		{Role: "assistant", ToolCalls: []ToolCall{
			{Arguments: json.RawMessage(`{"path":"a.txt"}`)},
			{Arguments: json.RawMessage(`{"pattern":"x"}`)},
		}},
		{Role: "tool", Name: "read_file", Content: "A"},
		{Role: "tool", Name: "grep", Content: "B"},
	}
	if wire := SanitizeToolPairing(in); len(wire) != 0 {
		t.Fatalf("provider projection must reject tool calls without host-issued identities: %#v", wire)
	}
}

func TestNormalizeSessionMessagesPreservesStandaloneToolMessage(t *testing.T) {
	in := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "run it"},
		{Role: "tool", ToolCallID: "orphan", Content: "saved output"},
	}
	out := NormalizeSessionMessages(in)
	if len(out) != len(in) {
		t.Fatalf("session-safe normalize must preserve standalone tool messages: %#v", out)
	}
	if &out[0] != &in[0] {
		t.Fatalf("session-safe healthy history should keep the input slice unchanged")
	}
	if out[2].Role != "tool" || out[2].ToolCallID != "orphan" || out[2].Content != "saved output" {
		t.Fatalf("standalone tool message was not preserved: %#v", out)
	}
}

func TestNormalizeSessionMessagesPreservesExtraToolResult(t *testing.T) {
	callID := providerTestHostToolCallID("session-extra")
	in := []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: callID, Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}}},
		{Role: "tool", ToolCallID: callID, Content: "A"},
		{Role: "tool", ToolCallID: "orphan_extra", Content: "saved extra output"},
	}
	session := NormalizeSessionMessages(in)
	if len(session) != len(in) {
		t.Fatalf("session-safe normalize must preserve extra stored tool results: %#v", session)
	}
	if session[2].ToolCallID != "orphan_extra" || session[2].Content != "saved extra output" {
		t.Fatalf("extra stored tool result was not preserved: %#v", session)
	}

	wire := SanitizeToolPairing(in)
	if len(wire) != 2 {
		t.Fatalf("wire-safe sanitize should drop extra orphan results before provider send: %#v", wire)
	}
}

func TestRuntimeProviderDiagnosticsReflectConfiguredAvailabilityWithoutSecrets(t *testing.T) {
	empty := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		DefaultProviderID:     "deepseek",
		DefaultEndpointFormat: "chat_completions",
		DefaultModel:          "deepseek-chat",
	}).Diagnostics()
	if len(empty) != 1 ||
		empty[0]["id"] != "deepseek" ||
		empty[0]["available"] != false ||
		empty[0]["hasApiKey"] != false {
		t.Fatalf("empty provider config should be unavailable without a credential: %#v", empty)
	}

	configured := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: string(mustProviderJSON(t, map[string]any{
			"defaultProviderId": "deepseek",
			"providers": []map[string]any{
				{"id": "deepseek", "apiKey": "sk-secret-provider-key", "baseUrl": "https://api.deepseek.com", "endpointFormat": "chat_completions", "models": []string{"deepseek-chat"}},
				{"id": "openai-compatible", "apiKey": "", "baseUrl": "https://openai.example/v1", "endpointFormat": "chat_completions", "models": []string{"gpt-compatible"}},
			},
		})),
	}).Diagnostics()
	raw, _ := json.Marshal(configured)
	if strings.Contains(string(raw), "sk-secret-provider-key") {
		t.Fatalf("provider diagnostics leaked API key: %s", string(raw))
	}
	deepseek := providerDiagnosticByID(t, configured, "deepseek")
	if deepseek["available"] != true || deepseek["hasApiKey"] != true || deepseek["model"] != "deepseek-chat" {
		t.Fatalf("configured DeepSeek provider should be available: %#v", configured)
	}
	openai := providerDiagnosticByID(t, configured, "openai-compatible")
	if openai["available"] != false || openai["hasApiKey"] != false {
		t.Fatalf("keyless provider profile should not be marked available: %#v", configured)
	}
}

func TestRuntimeProviderConfigNormalizesAnthropicMessagesAlias(t *testing.T) {
	config := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: string(mustProviderJSON(t, map[string]any{
			"defaultProviderId": "anthropic-main",
			"providers": []map[string]any{
				{
					"id":             "anthropic-main",
					"apiKey":         "sk-secret-provider-key",
					"baseUrl":        "https://api.anthropic.example",
					"endpointFormat": "anthropic_messages",
					"models":         []string{"claude-3-5-sonnet-latest"},
					"modelProfiles": map[string]any{
						"claude-3-5-sonnet-latest": map[string]any{
							"inputModalities": []string{"text", "image"},
						},
					},
				},
			},
		})),
	}).TurnConfig("", "")
	if config.ProviderID != "anthropic-main" ||
		config.Model != "claude-3-5-sonnet-latest" ||
		config.EndpointFormat != "messages" ||
		config.Family != "anthropic-compatible" {
		t.Fatalf("anthropic_messages should normalize to messages: %#v", config)
	}
}

func TestRuntimeProviderConfigSetHasProviderOnlyAllowsConfiguredOrDefault(t *testing.T) {
	config := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		ModelProvidersJSON: string(mustProviderJSON(t, map[string]any{
			"defaultProviderId": "configured-default",
			"providers": []map[string]any{{
				"id":             "configured-default",
				"apiKey":         "sk-default",
				"baseUrl":        "https://default.example/v1",
				"endpointFormat": "chat_completions",
				"models":         []string{"default-model"},
			}, {
				"id":             "configured-alt",
				"apiKey":         "sk-alt",
				"baseUrl":        "https://alt.example/v1",
				"endpointFormat": "messages",
				"models":         []string{"alt-model"},
			}},
		})),
	})

	for _, providerID := range []string{"", "configured-default", "configured-alt"} {
		if !config.HasProvider(providerID) {
			t.Fatalf("expected provider %q to be allowed", providerID)
		}
	}
	if config.HasProvider("missing-provider") {
		t.Fatalf("missing provider must not be accepted as a configured provider")
	}

	legacyDefault := NewRuntimeProviderConfigSet(RuntimeProviderConfigInput{
		DefaultProviderID: "deepseek",
		DefaultBaseURL:    "https://api.deepseek.example",
		DefaultModel:      "deepseek-chat",
	})
	if !legacyDefault.HasProvider("openai-configured") {
		t.Fatalf("without a provider catalog, contract/default runtime mode should defer explicit ids to TurnConfig")
	}
}

func TestParseOpenAIChatToolCallStreamCallbackSeesStartBeforeFinalCall(t *testing.T) {
	seen := []Chunk{}
	_, _, err := parseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
		seen = append(seen, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) < 2 || seen[0].Kind != "tool_call_start" || seen[1].Kind != "tool_call" {
		t.Fatalf("tool call start must stream before final call: %#v", seen)
	}
	if seen[0].ToolCall.ID != "call_read" || seen[0].ToolCall.Name != "read_file" {
		t.Fatalf("unexpected start chunk: %#v", seen[0])
	}
}

func firstToolCallChunk(t *testing.T, chunks []Chunk) ToolCall {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Kind == "tool_call" {
			return chunk.ToolCall
		}
	}
	t.Fatalf("missing tool_call chunk: %#v", chunks)
	return ToolCall{}
}

func firstToolCallStartChunk(t *testing.T, chunks []Chunk) ToolCall {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Kind == "tool_call_start" {
			return chunk.ToolCall
		}
	}
	t.Fatalf("missing tool_call_start chunk: %#v", chunks)
	return ToolCall{}
}

func toolCallChunks(chunks []Chunk) []ToolCall {
	calls := []ToolCall{}
	for _, chunk := range chunks {
		if chunk.Kind == "tool_call" {
			calls = append(calls, chunk.ToolCall)
		}
	}
	return calls
}

func chunkKinds(chunks []Chunk) []string {
	kinds := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		kinds = append(kinds, string(chunk.Kind))
	}
	return kinds
}

func collectChunkText(chunks []Chunk, kind ChunkKind) string {
	var out strings.Builder
	for _, chunk := range chunks {
		if chunk.Kind == kind {
			out.WriteString(chunk.Text)
		}
	}
	return out.String()
}

func sameChunkKindSlice(left []string, right []string) bool {
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

func mustProviderJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func providerDiagnosticByID(t *testing.T, diagnostics []map[string]any, id string) map[string]any {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic["id"] == id {
			return diagnostic
		}
	}
	t.Fatalf("missing provider diagnostic %s in %#v", id, diagnostics)
	return nil
}
