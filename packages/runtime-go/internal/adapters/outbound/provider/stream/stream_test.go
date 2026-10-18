package stream

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

func TestThinkSplitterCarriesFirstRawContentTimestampToAtomicText(t *testing.T) {
	t.Parallel()
	firstRaw := time.Unix(100, 25).UTC()
	splitter := &ThinkSplitter{}
	if chunks, _, _, _, err := parseOpenAIChatSSEPayloadAt(
		`{"choices":[{"delta":{"content":"one"}}]}`,
		newToolCallAccumulator(),
		splitter,
		firstRaw,
	); err != nil || len(chunks) != 0 {
		t.Fatalf("first content was not held transactionally: chunks=%#v err=%v", chunks, err)
	}
	if _, _, _, _, err := parseOpenAIChatSSEPayloadAt(
		`{"choices":[{"delta":{"content":"two"}}]}`,
		newToolCallAccumulator(),
		splitter,
		firstRaw.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	chunks, err := splitter.flushChunks()
	if err != nil || len(chunks) != 1 || chunks[0].Text != "onetwo" ||
		!chunks[0].Trace.ProviderRawSSEChunkAt.Equal(firstRaw) || !chunks[0].Trace.ProviderLastRawSSEChunkAt.Equal(firstRaw.Add(time.Second)) {
		t.Fatalf("atomic text lost first raw trace: chunks=%#v err=%v", chunks, err)
	}
}

func TestProviderPayloadObservationClassifiesAllEndpointFamilies(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		payload     string
		required    PayloadObservation
		retryUnsafe bool
	}{
		{
			name: "chat private reasoning", format: "chat_completions",
			payload:  `data: {"choices":[{"delta":{"reasoning_content":"private"}}]}` + "\n\n",
			required: PayloadObservationPrivateReasoning,
		},
		{
			name: "chat withheld public text", format: "chat_completions",
			payload:  `data: {"choices":[{"delta":{"content":"public"}}]}` + "\n\n",
			required: PayloadObservationPublicText, retryUnsafe: true,
		},
		{
			name: "chat partial tool", format: "chat_completions",
			payload:  `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\""}}]}}]}` + "\n\n",
			required: PayloadObservationToolStarted, retryUnsafe: true,
		},
		{
			name: "responses private reasoning", format: "responses",
			payload:  `data: {"type":"response.reasoning_text.delta","delta":"private"}` + "\n\n",
			required: PayloadObservationPrivateReasoning,
		},
		{
			name: "responses public text", format: "responses",
			payload:  `data: {"type":"response.output_text.delta","delta":"public"}` + "\n\n",
			required: PayloadObservationPublicText, retryUnsafe: true,
		},
		{
			name: "responses partial tool", format: "responses",
			payload:  `data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"x\""}` + "\n\n",
			required: PayloadObservationToolStarted, retryUnsafe: true,
		},
		{
			name: "messages private reasoning", format: "messages",
			payload:  `data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"private"}}` + "\n\n",
			required: PayloadObservationPrivateReasoning,
		},
		{
			name: "messages public text", format: "messages",
			payload:  `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"public"}}` + "\n\n",
			required: PayloadObservationPublicText, retryUnsafe: true,
		},
		{
			name: "messages partial tool", format: "messages",
			payload:  `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"x\""}}` + "\n\n",
			required: PayloadObservationToolStarted, retryUnsafe: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := domainmodel.ProviderOutputObservationNoneV1
			_, _, err := ParseSSEWithCallbackAndPayloadObservation(test.format, strings.NewReader(test.payload), nil, func(next PayloadObservation) {
				observation = observation.MergeV1(next)
			})
			if err == nil {
				t.Fatal("incomplete provider stream unexpectedly completed")
			}
			if !observation.IncludesV1(test.required) || observation.RetryUnsafeV1() != test.retryUnsafe {
				t.Fatalf("provider payload classification mismatch: got=%08b required=%08b unsafe=%v", observation, test.required, observation.RetryUnsafeV1())
			}
		})
	}
}

func TestProviderPayloadObservationSurvivesSamePayloadValidationFailure(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "cache usage conflict",
			payload: `data: {"choices":[{"delta":{"content":"public-before-cache-error"}}],` +
				`"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"prompt_cache_hit_tokens":8,"prompt_cache_miss_tokens":8}}` + "\n\n",
		},
		{
			name: "reasoning markup limit",
			payload: `data: {"choices":[{"delta":{"content":"` +
				strings.Repeat("<think>", domainreasoningmarkup.MaxDepth+1) + `"}}]}` + "\n\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := domainmodel.ProviderOutputObservationNoneV1
			_, _, err := ParseSSEWithCallbackAndPayloadObservation("chat_completions", strings.NewReader(test.payload), nil, func(next PayloadObservation) {
				observation = observation.MergeV1(next)
			})
			if err == nil {
				t.Fatal("invalid provider payload unexpectedly completed")
			}
			if !observation.IncludesV1(PayloadObservationPublicText) || !observation.RetryUnsafeV1() {
				t.Fatalf("validation error discarded prior public output observation: %08b", observation)
			}
		})
	}
}

func TestParseOpenAIChatSSEToolCallAndNativeCacheUsage(t *testing.T) {
	callbackUsageReasons := []string{}
	chunks, usage, err := ParseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"prompt_cache_hit_tokens":2,"prompt_cache_miss_tokens":5}}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
		if chunk.Kind == ChunkUsage {
			callbackUsageReasons = append(callbackUsageReasons, chunk.Usage.FinishReason)
		}
		return nil
	})
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
	if usage.PromptTokens != 7 || usage.CompletionTokens != 3 || usage.TotalTokens != 10 ||
		usage.CacheHitTokens != 2 || usage.CacheMissTokens != 5 || !usage.HasCacheTelemetry() ||
		usage.FinishReason != "tool_calls" {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if len(callbackUsageReasons) != 1 || callbackUsageReasons[0] != "tool_calls" {
		t.Fatalf("usage callback lost the closed finish reason: %#v", callbackUsageReasons)
	}
}

func TestParseOpenAIChatRejectsUnsafeTerminalFinishReasons(t *testing.T) {
	tests := []struct {
		finishReason string
		failureCode  string
	}{
		{finishReason: "content_filter", failureCode: domainfailure.CodeProviderRequestRejected},
	}
	for _, test := range tests {
		for _, terminal := range []struct {
			name   string
			suffix []string
		}{
			{name: "done", suffix: []string{`data: [DONE]`}},
			{name: "clean_eof"},
		} {
			t.Run(test.finishReason+"_"+terminal.name, func(t *testing.T) {
				lines := append([]string{
					`data: {"choices":[{"delta":{"content":"incomplete private candidate"},"finish_reason":"` + test.finishReason + `"}]}`,
					`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
				}, terminal.suffix...)
				chunks, usage, err := ParseSSE("chat_completions", strings.NewReader(strings.Join(lines, "\n\n")))
				if err == nil {
					t.Fatal("unsafe terminal finish reason became a successful provider result")
				}
				publicFailure, ok := err.(interface{ PublicFailureRecord() domainfailure.Record })
				if !ok || publicFailure.PublicFailureRecord().Code() != test.failureCode {
					t.Fatalf("unsafe finish reason did not use the closed host failure: err=%T code=%q", err, publicFailureCode(err))
				}
				if usage.FinishReason != test.finishReason {
					t.Fatalf("unsafe finish reason was not retained as closed telemetry: %#v", usage)
				}
				for _, chunk := range chunks {
					if chunk.Kind == ChunkDone {
						t.Fatalf("unsafe finish reason emitted a successful done chunk: %#v", chunks)
					}
				}
			})
		}
	}
}

func TestParseOpenAIChatPropagatesStopAtTerminalBoundaries(t *testing.T) {
	for _, terminal := range []struct {
		name   string
		suffix []string
	}{
		{name: "done", suffix: []string{`data: [DONE]`}},
		{name: "clean_eof"},
	} {
		t.Run(terminal.name, func(t *testing.T) {
			callbackUsageReasons := []string{}
			lines := append([]string{
				`data: {"choices":[{"delta":{"content":"complete public candidate"},"finish_reason":"stop"}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
			}, terminal.suffix...)
			chunks, usage, err := ParseSSEWithCallback("chat_completions", strings.NewReader(strings.Join(lines, "\n\n")), func(chunk Chunk) error {
				if chunk.Kind == ChunkUsage {
					callbackUsageReasons = append(callbackUsageReasons, chunk.Usage.FinishReason)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if usage.FinishReason != "stop" {
				t.Fatalf("safe stop finish reason was not retained: %#v", usage)
			}
			seenDone := false
			seenUsage := false
			for _, chunk := range chunks {
				switch chunk.Kind {
				case ChunkDone:
					seenDone = true
				case ChunkUsage:
					seenUsage = true
					if chunk.Usage.FinishReason != "stop" {
						t.Fatalf("usage chunk lost the safe stop finish reason: %#v", chunk)
					}
				}
			}
			if !seenDone || !seenUsage {
				t.Fatalf("safe stop did not produce terminal chunks: %#v", chunks)
			}
			if len(callbackUsageReasons) != 1 || callbackUsageReasons[0] != "stop" {
				t.Fatalf("usage callback lost the safe stop finish reason: %#v", callbackUsageReasons)
			}
		})
	}
}

func TestParseOpenAIChatRejectsTerminalToolCallContradictions(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		failureCode   string
		forbiddenKind ChunkKind
	}{
		{
			name:          "tool_calls_without_call",
			payload:       `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			failureCode:   domainfailure.CodeProviderRequestRejected,
			forbiddenKind: ChunkDone,
		},
		{
			name: "stop_with_call",
			payload: `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-stop","type":"function",` +
				`"function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"stop"}]}`,
			failureCode:   domainfailure.CodeProviderStreamInterrupted,
			forbiddenKind: ChunkToolCall,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunks, usage, err := ParseSSE("chat_completions", strings.NewReader(test.payload+"\n\ndata: [DONE]\n\n"))
			if err == nil || publicFailureCode(err) != test.failureCode {
				t.Fatalf("terminal tool-call contradiction did not fail closed: err=%T code=%q", err, publicFailureCode(err))
			}
			if usage.FinishReason == "unknown" || usage.FinishReason == "" {
				t.Fatalf("terminal tool-call contradiction lost finish reason: %#v", usage)
			}
			for _, chunk := range chunks {
				if chunk.Kind == test.forbiddenKind || chunk.Kind == ChunkDone {
					t.Fatalf("terminal tool-call contradiction emitted a forbidden chunk: %#v", chunks)
				}
			}
		})
	}
}

func TestParseOpenAIChatRejectsUnknownNonEmptyFinishReason(t *testing.T) {
	chunks, usage, err := ParseSSE("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"untrusted candidate"},"finish_reason":"provider_private_reason"}]}`,
		`data: [DONE]`,
	}, "\n\n")))
	if err == nil || publicFailureCode(err) != domainfailure.CodeProviderRequestRejected {
		t.Fatalf("unknown provider finish reason did not fail closed: err=%T code=%q", err, publicFailureCode(err))
	}
	if usage.FinishReason != "unknown" {
		t.Fatalf("unknown provider finish reason was not normalized: %#v", usage)
	}
	for _, chunk := range chunks {
		if chunk.Kind == ChunkDone {
			t.Fatalf("unknown provider finish reason emitted a successful done chunk: %#v", chunks)
		}
	}
}

func TestParseOpenAIChatRejectsConflictingChoiceFinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		firstReason  string
		secondReason string
	}{
		{name: "length_then_stop", firstReason: "length", secondReason: "stop"},
		{name: "stop_then_content_filter", firstReason: "stop", secondReason: "content_filter"},
		{name: "unknown_then_stop", firstReason: "provider_private_reason", secondReason: "stop"},
		{name: "stop_then_tool_calls", firstReason: "stop", secondReason: "tool_calls"},
	}
	for _, test := range tests {
		for _, terminal := range []struct {
			name   string
			suffix []string
		}{
			{name: "done", suffix: []string{`data: [DONE]`}},
			{name: "clean_eof"},
		} {
			t.Run(test.name+"_"+terminal.name, func(t *testing.T) {
				lines := append([]string{
					`data: {"choices":[{"delta":{"content":"must remain withheld"},"finish_reason":"` + test.firstReason +
						`"},{"delta":{},"finish_reason":"` + test.secondReason + `"}]}`,
				}, terminal.suffix...)
				chunks, _, err := ParseSSE("chat_completions", strings.NewReader(strings.Join(lines, "\n\n")))
				if err == nil || publicFailureCode(err) != domainfailure.CodeProviderRequestRejected {
					t.Fatalf("conflicting choice finish reasons did not fail closed: err=%T code=%q", err, publicFailureCode(err))
				}
				assertNoPublicTerminalChunks(t, chunks)
			})
		}
	}
}

func TestParseOpenAIChatDoneRejectsToolCallWithoutSemanticFinish(t *testing.T) {
	chunks, usage, err := ParseSSE("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-no-finish","type":"function",` +
			`"function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}}]}`,
		`data: [DONE]`,
	}, "\n\n")))
	if err == nil || publicFailureCode(err) != domainfailure.CodeProviderStreamInterrupted {
		t.Fatalf("tool call without semantic finish did not fail closed: err=%T code=%q", err, publicFailureCode(err))
	}
	if usage.FinishReason != "unknown" {
		t.Fatalf("missing semantic finish was not retained as unknown: %#v", usage)
	}
	assertNoExecutableTerminalChunks(t, chunks)
}

func TestParseOpenAIChatRejectsMissingSemanticFinishAtTerminalBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{
			name:  "done",
			lines: []string{`data: {"choices":[{"delta":{"content":"partial public candidate"}}]}`, `data: [DONE]`},
		},
		{
			name:  "clean_eof",
			lines: []string{`data: {"choices":[{"delta":{"content":"partial public candidate"}}]}`},
		},
		{
			name: "blank_finish_reason",
			lines: []string{
				`data: {"choices":[{"delta":{"content":"partial public candidate"},"finish_reason":"   "}]}`,
				`data: [DONE]`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunks, usage, err := ParseSSE("chat_completions", strings.NewReader(strings.Join(test.lines, "\n\n")))
			if err == nil || publicFailureCode(err) != domainfailure.CodeProviderStreamInterrupted {
				t.Fatalf("missing semantic finish did not fail closed: err=%T code=%q", err, publicFailureCode(err))
			}
			if usage.FinishReason != "unknown" {
				t.Fatalf("missing semantic finish was not normalized: %#v", usage)
			}
			assertNoPublicTerminalChunks(t, chunks)
		})
	}
}

func TestParseOpenAIChatValidatesToolArgumentsBeforePublishingText(t *testing.T) {
	callbackChunks := []Chunk{}
	chunks, _, err := ParseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"must remain withheld"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-invalid","type":"function",` +
			`"function":{"name":"read_file","arguments":"{\"path\":\"a.txt\""}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
		callbackChunks = append(callbackChunks, chunk)
		return nil
	})
	if err == nil || publicFailureCode(err) != domainfailure.CodeProviderToolArgumentsInvalid {
		t.Fatalf("invalid tool arguments did not fail closed before publication: err=%T code=%q", err, publicFailureCode(err))
	}
	assertNoPublicTerminalChunks(t, chunks)
	assertNoPublicTerminalChunks(t, callbackChunks)
}

func TestParseOpenAIChatRejectsPayloadAfterSemanticFinish(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
	}{
		{
			name: "tool_after_stop",
			lines: []string{
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-after-stop","type":"function",` +
					`"function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}}]}`,
			},
		},
		{
			name: "second_tool_after_tool_finish",
			lines: []string{
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-before-finish","type":"function",` +
					`"function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call-after-finish","type":"function",` +
					`"function":{"name":"read_file","arguments":"{\"path\":\"b.txt\"}"}}]}}]}`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := append(test.lines, `data: [DONE]`)
			chunks, _, err := ParseSSE("chat_completions", strings.NewReader(strings.Join(lines, "\n\n")))
			if err == nil || publicFailureCode(err) != domainfailure.CodeProviderRequestRejected {
				t.Fatalf("payload after semantic finish did not fail closed: err=%T code=%q", err, publicFailureCode(err))
			}
			assertNoExecutableTerminalChunks(t, chunks)
		})
	}
}

func assertNoExecutableTerminalChunks(t *testing.T, chunks []Chunk) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Kind == ChunkToolCall || chunk.Kind == ChunkDone {
			t.Fatalf("invalid provider stream emitted an executable terminal chunk: %#v", chunks)
		}
	}
}

func assertNoPublicTerminalChunks(t *testing.T, chunks []Chunk) {
	t.Helper()
	for _, chunk := range chunks {
		switch chunk.Kind {
		case ChunkText, ChunkToolCall, ChunkUsage, ChunkDone:
			t.Fatalf("invalid provider stream emitted public or terminal output: %#v", chunks)
		}
	}
}

func publicFailureCode(err error) string {
	publicFailure, ok := err.(interface{ PublicFailureRecord() domainfailure.Record })
	if !ok {
		return ""
	}
	return publicFailure.PublicFailureRecord().Code()
}

func TestTruncatedProviderToolArgumentsNeverExecute(t *testing.T) {
	for _, raw := range []string{
		`{"path":"a.txt"`,
		`{"items":["a","b"`,
		`{"path":"a.txt",`,
		`{"path":`,
		`["not-an-object"]`,
		`null`,
	} {
		accumulator := newToolCallAccumulator()
		_ = accumulator.add(0, "call-truncated", "read_file", raw)
		chunks, err := accumulator.flush()
		if err == nil || len(chunks) != 0 {
			t.Fatalf("truncated/non-object arguments became an executable call: raw=%q chunks=%#v err=%v", raw, chunks, err)
		}
	}

	accumulator := newToolCallAccumulator()
	_ = accumulator.add(0, "call-valid", "read_file", ` {"path":"a.txt","path":"duplicate-must-survive"} `)
	chunks, err := accumulator.flush()
	if err != nil || len(chunks) != 1 || string(chunks[0].ToolCall.Arguments) != `{"path":"a.txt","path":"duplicate-must-survive"}` {
		t.Fatalf("complete arguments were repaired or canonicalized before strict validation: chunks=%#v err=%v", chunks, err)
	}
}

func TestEveryProviderRejectsTruncatedToolArgumentsAtTerminalFrame(t *testing.T) {
	tests := []struct {
		name           string
		endpointFormat string
		body           string
		failureCode    string
	}{
		{
			name: "openai chat finish_reason", endpointFormat: "chat_completions",
			body: strings.Join([]string{
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-chat","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\""}}]},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
			}, "\n\n"),
			failureCode: domainfailure.CodeProviderToolArgumentsInvalid,
		},
		{
			name: "openai chat DONE", endpointFormat: "chat_completions",
			body: strings.Join([]string{
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-chat-done","type":"function","function":{"name":"read_file","arguments":"{\"path\":"}}]}}]}`,
				`data: [DONE]`,
			}, "\n\n"),
			failureCode: domainfailure.CodeProviderStreamInterrupted,
		},
		{
			name: "openai responses item done", endpointFormat: "responses",
			body: strings.Join([]string{
				`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call-responses","name":"read_file","arguments":""}}`,
				`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"path\":\"a.txt\""}`,
				`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call-responses","name":"read_file","arguments":""}}`,
			}, "\n\n"),
			failureCode: domainfailure.CodeProviderToolArgumentsInvalid,
		},
		{
			name: "openai responses whitespace item done", endpointFormat: "responses",
			body: strings.Join([]string{
				`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call-responses","name":"read_file","arguments":""}}`,
				`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"path\":\"a.txt\""}`,
				`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call-responses","name":"read_file","arguments":"  "}}`,
			}, "\n\n"),
			failureCode: domainfailure.CodeProviderToolArgumentsInvalid,
		},
		{
			name: "anthropic content block stop", endpointFormat: "messages",
			body: strings.Join([]string{
				`data: {"type":"message_start","message":{"usage":{"input_tokens":1}}}`,
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call-anthropic","name":"read_file","input":{}}}`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.txt\""}}`,
				`data: {"type":"content_block_stop","index":0}`,
			}, "\n\n"),
			failureCode: domainfailure.CodeProviderToolArgumentsInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunks, _, err := ParseSSE(test.endpointFormat, strings.NewReader(test.body))
			if err == nil || publicFailureCode(err) != test.failureCode {
				t.Fatalf("truncated provider tool arguments were accepted: chunks=%#v err=%v", chunks, err)
			}
			for _, chunk := range chunks {
				if chunk.Kind == ChunkToolCall || chunk.Kind == ChunkDone {
					t.Fatalf("truncated arguments emitted executable/terminal chunk: %#v", chunks)
				}
			}
		})
	}
}

func TestParseOpenAIChatSSEInterleavedReasoningAndTextCallbacks(t *testing.T) {
	seen := []string{}
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`data: {"choices":[{"delta":{"content":"1\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"2\n"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
		if frame := chunk.ProcessFrame; frame != nil {
			frames = append(frames, *frame)
			if frame.Source == "provider_reasoning" && (frame.Kind == "start" || frame.Kind == "end") && chunk.Kind == ChunkReasoning && chunk.Text == "" {
				return nil
			}
		}
		if chunk.Kind == ChunkReasoning || chunk.Kind == ChunkText {
			seen = append(seen, string(chunk.Kind)+":"+chunk.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(seen, "|"); got != "reasoning:think|text:1\n2\n" {
		t.Fatalf("provider chunks must callback in SSE order, got %q", got)
	}
	if got := chunkKinds(chunks[:2]); !sameChunkKindSlice(got, []string{"reasoning", "text"}) {
		t.Fatalf("unexpected interleaved chunks: %#v", got)
	}
	wantFrames := []domainmodel.ProcessFrameV1{
		{Source: "provider_reasoning", Kind: "start", HostSegmentOrdinal: 1},
		{Source: "provider_reasoning", Kind: "delta", HostSegmentOrdinal: 1, Text: "think"},
		{Source: "provider_reasoning", Kind: "end", HostSegmentOrdinal: 1},
	}
	if !reflect.DeepEqual(frames, wantFrames) {
		t.Fatalf("unexpected host reasoning lifecycle: %#v", frames)
	}
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("private display frame entered retained continuation")
		}
	}

}

func TestParseOpenAIResponsesDoneFlushesToolCallThroughCallback(t *testing.T) {
	seen := []string{}
	chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(strings.Join([]string{
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

func TestParseAnthropicMessagesSSEUsageAndToolCall(t *testing.T) {
	chunks, usage, err := ParseSSE("messages", strings.NewReader(strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5,"cache_creation_input_tokens":3,"cache_read_input_tokens":2}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup","input":{"query":"weather"}}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n")))
	if err != nil {
		t.Fatal(err)
	}
	call := firstToolCallChunk(t, chunks)
	if call.ID != "tool_1" || call.Name != "lookup" || string(call.Arguments) != `{"query":"weather"}` {
		t.Fatalf("unexpected Anthropic tool call: %#v", call)
	}
	if usage.PromptTokens != 10 || usage.CompletionTokens != 4 || usage.TotalTokens != 14 ||
		usage.CacheHitTokens != 2 || usage.CacheMissTokens != 8 || !usage.HasCacheTelemetry() {
		t.Fatalf("unexpected Anthropic usage: %#v", usage)
	}
}

func TestMessagesTerminalReasonAndCumulativeUsage(t *testing.T) {
	for _, reason := range []string{"end_turn", "max_tokens", "unknown", ""} {
		t.Run(reason, func(t *testing.T) {
			lines := []string{
				`data: {"type":"message_start","message":{"usage":{"input_tokens":3,"cache_read_input_tokens":0,"cache_creation_input_tokens":2}}}`,
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"bounded synthetic answer"}}`,
				`data: {"type":"message_delta","usage":{"output_tokens":2}}`,
				`data: {"type":"message_delta","delta":{"stop_reason":"` + reason + `"},"usage":{"output_tokens":5}}`,
				`data: {"type":"message_stop"}`,
			}
			chunks, usage, err := ParseSSE("messages", strings.NewReader(strings.Join(lines, "\n\n")))
			if reason != "end_turn" && reason != "max_tokens" {
				if err == nil {
					t.Fatal("incomplete/unknown terminal succeeded")
				}
				for _, chunk := range chunks {
					if chunk.Kind == ChunkDone {
						t.Fatal("failed stream emitted success")
					}
				}
				return
			}
			if err != nil || usage.CompletionTokens != 5 || usage.PromptTokens != 5 || usage.FinishReason != domainmodel.NormalizeFinishReason(reason) || !usage.HasCacheHit || usage.CacheHitTokens != 0 {
				t.Fatalf("cumulative or known-zero usage lost: %+v %v", usage, err)
			}
		})
	}
}

func TestParseOpenAIChatCleanEOFRejectsIncompleteToolCall(t *testing.T) {
	_, _, err := ParseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"read_file","arguments":"{\"path\""}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	}, "\n\n")), nil)
	if err == nil || publicFailureCode(err) != domainfailure.CodeProviderStreamInterrupted {
		t.Fatalf("expected incomplete tool call error, got %v", err)
	}
}

func firstToolCallStartChunk(t *testing.T, chunks []Chunk) ToolCall {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Kind == ChunkToolCallStart {
			return chunk.ToolCall
		}
	}
	t.Fatalf("missing tool_call_start in %#v", chunks)
	return ToolCall{}
}

func firstToolCallChunk(t *testing.T, chunks []Chunk) ToolCall {
	t.Helper()
	for _, chunk := range chunks {
		if chunk.Kind == ChunkToolCall {
			return chunk.ToolCall
		}
	}
	t.Fatalf("missing tool_call in %#v", chunks)
	return ToolCall{}
}

func chunkKinds(chunks []Chunk) []string {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, string(chunk.Kind))
	}
	return out
}

func sameChunkKindSlice(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func TestMessagesUsageSnapshotsKeepUnknownZeroAndInterruptedCountersDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, initial, terminal string
		known, wantError        bool
	}{
		{"missing buckets", `"input_tokens":8`, `data: {"type":"message_stop"}`, false, false},
		{"explicit zero", `"input_tokens":8,"cache_read_input_tokens":0,"cache_creation_input_tokens":0`, `data: {"type":"message_stop"}`, true, false},
		{"interrupted after usage", `"input_tokens":8,"cache_read_input_tokens":0,"cache_creation_input_tokens":0`, `data: {"type":"error","error":{"message":"PRIVATE MUST NOT ECHO"}}`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := strings.Join([]string{`data: {"type":"message_start","message":{"model":"reported-model","usage":{` + tc.initial + `}}}`, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"public"}}`, `data: {"type":"message_delta","usage":{"output_tokens":3}}`, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`, tc.terminal}, "\n\n")
			chunks, u, err := ParseSSEWithCallback("messages", strings.NewReader(payload), nil)
			if (err != nil) != tc.wantError || u.CompletionTokens != 7 || !u.HasCompletionTokens || u.HasPromptTokens != tc.known || !u.MessagesInput.Uncached.Known || u.MessagesInput.Read.Known != tc.known {
				t.Fatalf("snapshot distinction lost: usage=%#v error=%v", u, err)
			}
			if err != nil && strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("provider error body escaped")
			}
			if !tc.wantError && chunks[len(chunks)-1].Trace.ProviderResponseModel != "reported-model" {
				t.Fatal("response model observation missing")
			}
		})
	}
}

func TestMessagesInterleavedToolBlocksFlushOnlyCompletedIndex(t *testing.T) {
	payload := strings.Join([]string{
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"first","name":"read","input":{}}}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"second","name":"read","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"A\"}"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"B\"}"}}`,
		`data: {"type":"content_block_stop","index":1}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		`data: {"type":"message_stop"}`}, "\n\n")
	chunks, _, err := ParseSSEWithCallback("messages", strings.NewReader(payload), nil)
	if err != nil {
		t.Fatal(err)
	}
	var calls []Chunk
	for _, chunk := range chunks {
		if chunk.Kind == ChunkToolCall {
			calls = append(calls, chunk)
		}
	}
	if len(calls) != 2 || calls[0].ToolCall.ID != "first" || calls[1].ToolCall.ID != "second" {
		t.Fatalf("interleaved tool identity/order lost: %#v", calls)
	}
}

func TestNativeThinkingFramesPreserveInitialAndEmptyContinuationChunks(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"content_block_start","index":7,"content_block":{"type":"thinking","id":"native-thinking-7","thinking":"initial","signature":"initial-signature"}}`,
		`data: {"type":"content_block_delta","index":7,"delta":{"type":"thinking_delta","thinking":""}}`,
		`data: {"type":"content_block_delta","index":7,"delta":{"type":"thinking_delta","thinking":"tail"}}`,
		`data: {"type":"content_block_delta","index":7,"delta":{"type":"signature_delta","signature":"final-signature"}}`,
		`data: {"type":"content_block_stop","index":7}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n")
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithCallback("messages", strings.NewReader(body), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, *chunk.ProcessFrame)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 5 {
		t.Fatalf("native boundaries or empty delta were lost: %#v", frames)
	}
	wantKinds := []string{"start", "delta", "delta", "delta", "end"}
	wantTexts := []string{"", "initial", "", "tail", ""}
	for index, frame := range frames {
		if frame.Kind != wantKinds[index] || frame.Text != wantTexts[index] || frame.NativeIndex != 7 || frame.NativeID != "native-thinking-7" || frame.Attempt != 0 {
			t.Fatalf("parser changed source framing: %#v", frames)
		}
	}
	var reasoning []Chunk
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("display sidecar entered retained provider chunks")
		}
		if chunk.Kind == ChunkReasoning {
			reasoning = append(reasoning, chunk)
		}
	}
	if len(reasoning) != 4 || reasoning[0].Text != "initial" || reasoning[0].Signature != "initial-signature" || reasoning[1].Text != "" || reasoning[2].Text != "tail" || reasoning[3].Signature != "final-signature" {
		t.Fatal("display framing changed exact initial, empty, or signed continuation chunks")
	}
}

func TestResponsesNativeFramesRequireMatchingSourceItemIdentity(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":5,"item":{"id":"rs-native-5","type":"reasoning"}}`,
		`data: {"type":"response.reasoning_text.delta","output_index":5,"item_id":"rs-native-5","delta":"native reasoning"}`,
		`data: {"type":"response.output_item.done","output_index":5,"item":{"id":"rs-native-5","type":"reasoning"}}`,
		`data: {"type":"response.completed"}`,
	}, "\n\n")
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(body), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, *chunk.ProcessFrame)
		}
		return nil
	})
	if err != nil || len(frames) != 3 {
		t.Fatalf("actual Responses native identity was lost: frames=%#v err=%v", frames, err)
	}
	for index, frame := range frames {
		if frame.Kind != []string{"start", "delta", "end"}[index] || frame.NativeIndex != 5 || frame.NativeID != "rs-native-5" || strings.Contains(frame.Text, "ENCRYPTED_MUST_NOT_DISPLAY") {
			t.Fatalf("Responses frame fabricated identity or copied encrypted input: %#v", frame)
		}
	}
	if frames[0].Text != "" || frames[1].Text != "native reasoning" || frames[2].Text != "" {
		t.Fatal("metadata-only boundaries copied provider bytes")
	}
	if len(chunks) != 2 || chunks[0].Kind != ChunkReasoning || chunks[0].Text != "native reasoning" || chunks[1].Kind != ChunkDone {
		t.Fatalf("Responses display changed provider continuation: %#v", chunks)
	}
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("Responses display frames entered retained provider chunks")
		}
	}
}

func TestProviderReasoningWithoutNativeIdentityNeverFabricatesDisplayBlocks(t *testing.T) {
	tests := []struct{ name, format, body string }{
		{"chat private reasoning", "chat_completions", strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning_content":"private continuation"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}, "\n\n")},
		{"messages missing index", "messages", strings.Join([]string{
			`data: {"type":"content_block_start","content_block":{"type":"thinking","thinking":"private continuation"}}`,
			`data: {"type":"content_block_stop"}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`data: {"type":"message_stop"}`,
		}, "\n\n")},
		{"responses missing explicit index", "responses", strings.Join([]string{
			`data: {"type":"response.output_item.added","item":{"id":"rs-unindexed","type":"reasoning"}}`,
			`data: {"type":"response.reasoning_text.delta","item_id":"rs-unindexed","delta":"private continuation"}`,
			`data: {"type":"response.output_item.done","item":{"id":"rs-unindexed","type":"reasoning"}}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n")},
		{"responses missing item id", "responses", strings.Join([]string{
			`data: {"type":"response.output_item.added","output_index":8,"item":{"type":"reasoning"}}`,
			`data: {"type":"response.reasoning_text.delta","output_index":8,"delta":"private continuation"}`,
			`data: {"type":"response.output_item.done","output_index":8,"item":{"type":"reasoning"}}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithCallback(test.format, strings.NewReader(test.body), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames = append(frames, *chunk.ProcessFrame)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.format == "chat_completions" {
				wantFrames := []domainmodel.ProcessFrameV1{
					{Source: "provider_reasoning", Kind: "start", HostSegmentOrdinal: 1},
					{Source: "provider_reasoning", Kind: "delta", HostSegmentOrdinal: 1, Text: "private continuation"},
					{Source: "provider_reasoning", Kind: "end", HostSegmentOrdinal: 1},
				}
				if !reflect.DeepEqual(frames, wantFrames) {
					t.Fatalf("chat fabricated native identity or changed host lifecycle: %#v", frames)
				}
			} else if len(frames) != 0 {
				t.Fatalf("unsupported native framing produced a synthetic block: %#v", frames)
			}
			text := ""
			for _, chunk := range chunks {
				if chunk.ProcessFrame != nil {
					t.Fatal("display frame entered retained provider continuation")
				}
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
				}
			}
			if text != "private continuation" {
				t.Fatal("missing native display evidence changed private continuation")
			}
		})
	}
}

func TestConflictingNativeReasoningIdentityCannotCompleteAnEmptyDisplayBlock(t *testing.T) {
	tests := []struct{ name, format, body string }{
		{"responses delta missing item id", "responses", strings.Join([]string{
			`data: {"type":"response.output_item.added","output_index":8,"item":{"id":"rs-8","type":"reasoning"}}`,
			`data: {"type":"response.reasoning_text.delta","output_index":8,"delta":"unbound continuation"}`,
			`data: {"type":"response.output_item.done","output_index":8,"item":{"id":"rs-8","type":"reasoning"}}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n")},
		{"messages delta missing index", "messages", strings.Join([]string{
			`data: {"type":"content_block_start","index":8,"content_block":{"type":"thinking","thinking":""}}`,
			`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"unbound continuation"}}`,
			`data: {"type":"content_block_stop","index":8}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`data: {"type":"message_stop"}`,
		}, "\n\n")},
		{"messages duplicate block start", "messages", strings.Join([]string{
			`data: {"type":"content_block_start","index":8,"content_block":{"type":"thinking","thinking":""}}`,
			`data: {"type":"content_block_start","index":8,"content_block":{"type":"thinking","thinking":"unbound continuation"}}`,
			`data: {"type":"content_block_stop","index":8}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`data: {"type":"message_stop"}`,
		}, "\n\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithCallback(test.format, strings.NewReader(test.body), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames = append(frames, *chunk.ProcessFrame)
				}
				return nil
			})
			if err != nil || len(frames) != 1 || frames[0].Kind != "start" || frames[0].NativeIndex != 8 || frames[0].Text != "" {
				t.Fatalf("unbound bytes became a completed display block: frames=%#v err=%v", frames, err)
			}
			text := ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
				}
			}
			if text != "unbound continuation" {
				t.Fatal("display identity rejection changed private continuation")
			}
		})
	}
}

func TestNativeMetadataObserverPanicCannotChangeProviderParsing(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"content_block_start","index":3,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_stop","index":3}`,
		`data: {"type":"content_block_start","index":7,"content_block":{"type":"text","text":"public answer"}}`,
		`data: {"type":"content_block_stop","index":7}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n")
	metadata := 0
	chunks, _, err := ParseSSEWithCallback("messages", strings.NewReader(body), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			metadata++
			panic("display metadata observer failed")
		}
		return nil
	})
	if err != nil || metadata != 2 {
		t.Fatalf("metadata observer changed parsing: metadata=%d err=%v", metadata, err)
	}
	text := ""
	for _, chunk := range chunks {
		if chunk.Kind == ChunkText {
			text += chunk.Text
		}
	}
	if text != "public answer" || !hasChunkKind(chunks, ChunkDone) {
		t.Fatal("metadata observer changed the provider result")
	}
}

func TestResponsesOpaqueOrRedactedItemsNeverBecomeDisplayBlocks(t *testing.T) {
	tests := []struct{ name, marker, delta string }{
		{"encrypted only", `"encrypted_content":"ENCRYPTED_ONLY_SENTINEL"`, ""},
		{"encrypted with continuation", `"encrypted_content":"ENCRYPTED_WITH_PLAINTEXT_SENTINEL"`, "private continuation"},
		{"redacted with continuation", `"redacted":true`, "private continuation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := []string{
				`data: {"type":"response.output_item.added","output_index":8,"item":{"id":"rs-opaque-8","type":"reasoning",` + test.marker + `}}`,
			}
			if test.delta != "" {
				lines = append(lines, `data: {"type":"response.reasoning_text.delta","output_index":8,"item_id":"rs-opaque-8","delta":"`+test.delta+`"}`)
			}
			lines = append(lines,
				`data: {"type":"response.output_item.done","output_index":8,"item":{"id":"rs-opaque-8","type":"reasoning",`+test.marker+`}}`,
				`data: {"type":"response.completed"}`,
			)
			frames := 0
			chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(strings.Join(lines, "\n\n")), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames++
				}
				return nil
			})
			if err != nil || frames != 0 {
				t.Fatalf("opaque Responses item claimed a display body: frames=%d err=%v", frames, err)
			}
			text := ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
				}
			}
			if text != test.delta {
				t.Fatal("opaque item display refusal changed existing private continuation")
			}
		})
	}
}

func TestResponsesOpaqueDoneCannotCompletePreviouslyPlaintextDisplayItem(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":8,"item":{"id":"rs-8","type":"reasoning"}}`,
		`data: {"type":"response.reasoning_text.delta","output_index":8,"item_id":"rs-8","delta":"private continuation"}`,
		`data: {"type":"response.output_item.done","output_index":8,"item":{"id":"rs-8","type":"reasoning","encrypted_content":"OPAQUE_TERMINAL_SENTINEL"}}`,
		`data: {"type":"response.completed"}`,
	}, "\n\n")
	var kinds []string
	chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(body), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			kinds = append(kinds, chunk.ProcessFrame.Kind)
		}
		return nil
	})
	if err != nil || strings.Join(kinds, "|") != "start|delta" || len(chunks) != 2 || chunks[0].Text != "private continuation" {
		t.Fatalf("opaque terminal item became a completed display block: kinds=%#v err=%v", kinds, err)
	}
}

func TestPoisonedNativeDisplayIdentityCannotReopenWithinPhysicalStream(t *testing.T) {
	messagesDone := []string{
		`data: {"type":"content_block_delta","index":8,"delta":{"type":"signature_delta","signature":"private-signature-unchanged"}}`,
		`data: {"type":"content_block_stop","index":8}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		`data: {"type":"message_stop"}`,
	}
	messageStart := `data: {"type":"content_block_start","index":8,"content_block":{"type":"thinking","thinking":""}}`
	messageA := `data: {"type":"content_block_delta","index":8,"delta":{"type":"thinking_delta","thinking":"A"}}`
	messageC := `data: {"type":"content_block_delta","index":8,"delta":{"type":"thinking_delta","thinking":"C"}}`
	responseStart := `data: {"type":"response.output_item.added","output_index":8,"item":{"id":"rs-8","type":"reasoning"}}`
	responseA := `data: {"type":"response.reasoning_text.delta","output_index":8,"item_id":"rs-8","delta":"A"}`
	responseC := `data: {"type":"response.reasoning_text.delta","output_index":8,"item_id":"rs-8","delta":"C"}`
	responsesDone := []string{
		`data: {"type":"response.output_item.done","output_index":8,"item":{"id":"rs-8","type":"reasoning"}}`,
		`data: {"type":"response.completed"}`,
	}
	tests := []struct {
		name, format string
		lines        []string
	}{
		{"messages duplicate third start", "messages", append([]string{messageStart, messageA, messageStart, messageStart, messageC}, messagesDone...)},
		{"messages missing block boundary", "messages", append([]string{messageStart, messageA,
			`data: {"type":"content_block_start","index":8}`, messageC}, messagesDone...)},
		{"messages opaque type collision", "messages", append([]string{messageStart, messageA,
			`data: {"type":"content_block_start","index":8,"content_block":{"type":"redacted_thinking","data":"opaque-private"}}`, messageC}, messagesDone...)},
		{"messages text delta collision", "messages", append([]string{messageStart, messageA,
			`data: {"type":"content_block_delta","index":8,"delta":{"type":"text_delta","text":"public preserved"}}`, messageC}, messagesDone...)},
		{"messages missing delta boundary", "messages", append([]string{messageStart, messageA,
			`data: {"type":"content_block_delta","index":8}`, messageC}, messagesDone...)},
		{"messages missing index then restart", "messages", append([]string{messageStart,
			`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"A"}}`, messageStart, messageC}, messagesDone...)},
		{"responses duplicate third start", "responses", append([]string{responseStart, responseA, responseStart, responseStart, responseC}, responsesDone...)},
		{"responses text type collision", "responses", append([]string{responseStart, responseA,
			`data: {"type":"response.output_item.added","output_index":8,"item":{"id":"text-8","type":"message"}}`, responseC}, responsesDone...)},
		{"responses text delta collision", "responses", append([]string{responseStart, responseA,
			`data: {"type":"response.output_text.delta","output_index":8,"item_id":"rs-8","delta":"public preserved"}`, responseC}, responsesDone...)},
		{"responses mismatched text done", "responses", append([]string{responseStart, responseA,
			`data: {"type":"response.reasoning_text.done","output_index":8,"item_id":"rs-other","text":"A"}`, responseC}, responsesDone...)},
		{"responses mismatched done", "responses", append([]string{responseStart, responseA,
			`data: {"type":"response.output_item.done","output_index":8,"item":{"id":"rs-other","type":"reasoning"}}`, responseC}, responsesDone...)},
		{"responses opaque then plaintext restart", "responses", append([]string{responseStart, responseA,
			`data: {"type":"response.output_item.added","output_index":8,"item":{"id":"rs-8","type":"reasoning","encrypted_content":"opaque-private"}}`, responseStart, responseC}, responsesDone...)},
		{"responses missing id then restart", "responses", append([]string{responseStart,
			`data: {"type":"response.reasoning_text.delta","output_index":8,"delta":"A"}`, responseStart, responseC}, responsesDone...)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithCallback(test.format, strings.NewReader(strings.Join(test.lines, "\n\n")+"\n\n"), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames = append(frames, *chunk.ProcessFrame)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			starts := 0
			for _, frame := range frames {
				if frame.Kind == "start" {
					starts++
				}
				if frame.Kind == "end" || frame.Kind == "delta" && frame.Text == "C" {
					t.Fatalf("poisoned source regained completed display authority: %#v", frames)
				}
			}
			if starts != 1 {
				t.Fatalf("source index was re-admitted after poisoning: %#v", frames)
			}
			text, signature, publicText := "", "", ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkText {
					publicText += chunk.Text
				}
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
					if chunk.Signature != "" {
						signature = chunk.Signature
					}
				}
				if chunk.ProcessFrame != nil {
					t.Fatal("display tracker entered continuation results")
				}
			}
			if text != "AC" || test.format == "messages" && signature != "private-signature-unchanged" || !hasChunkKind(chunks, ChunkDone) {
				t.Fatal("display poisoning changed text, signature, or semantic provider completion")
			}
			if strings.Contains(test.name, "text delta collision") && publicText != "public preserved" {
				t.Fatal("display poisoning changed original public text continuation")
			}
		})
	}
}

func TestOversizedNativeSourceIDIsNotRetainedForDisplay(t *testing.T) {
	id := strings.Repeat("n", 257)
	for _, format := range []string{"messages", "responses"} {
		t.Run(format, func(t *testing.T) {
			var lines []string
			if format == "messages" {
				lines = []string{
					`data: {"type":"content_block_start","index":8,"content_block":{"type":"thinking","id":"` + id + `","thinking":"private continuation"}}`,
					`data: {"type":"content_block_stop","index":8}`,
					`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
					`data: {"type":"message_stop"}`,
				}
			} else {
				lines = []string{
					`data: {"type":"response.output_item.added","output_index":8,"item":{"id":"` + id + `","type":"reasoning"}}`,
					`data: {"type":"response.reasoning_text.delta","output_index":8,"item_id":"` + id + `","delta":"private continuation"}`,
					`data: {"type":"response.output_item.done","output_index":8,"item":{"id":"` + id + `","type":"reasoning"}}`,
					`data: {"type":"response.completed"}`,
				}
			}
			frames := 0
			chunks, _, err := ParseSSEWithCallback(format, strings.NewReader(strings.Join(lines, "\n\n")+"\n\n"), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames++
				}
				return nil
			})
			text := ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
				}
			}
			if err != nil || frames != 0 || text != "private continuation" {
				t.Fatalf("native ID limit changed continuation or retained display identity: frames=%d err=%v", frames, err)
			}
		})
	}
}

func TestDuplicateNativeIDAtDifferentIndexesPoisonsBothSources(t *testing.T) {
	for _, format := range []string{"messages", "responses"} {
		t.Run(format, func(t *testing.T) {
			var lines []string
			if format == "messages" {
				lines = []string{
					`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","id":"same-native-source","thinking":""}}`,
					`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"A"}}`,
					`data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","id":"same-native-source","thinking":""}}`,
					`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"C"}}`,
					`data: {"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"D"}}`,
					`data: {"type":"content_block_stop","index":0}`,
					`data: {"type":"content_block_stop","index":1}`,
					`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
					`data: {"type":"message_stop"}`,
				}
			} else {
				lines = []string{
					`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"same-native-source","type":"reasoning"}}`,
					`data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"same-native-source","delta":"A"}`,
					`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"same-native-source","type":"reasoning"}}`,
					`data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"same-native-source","delta":"C"}`,
					`data: {"type":"response.reasoning_text.delta","output_index":1,"item_id":"same-native-source","delta":"D"}`,
					`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"same-native-source","type":"reasoning"}}`,
					`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"same-native-source","type":"reasoning"}}`,
					`data: {"type":"response.completed"}`,
				}
			}
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithCallback(format, strings.NewReader(strings.Join(lines, "\n\n")+"\n\n"), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames = append(frames, *chunk.ProcessFrame)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(frames) != 2 || frames[0].Kind != "start" || frames[0].NativeIndex != 0 || frames[1].Kind != "delta" || frames[1].Text != "A" {
				t.Fatalf("duplicate native ID retained authority at one or both indexes: %#v", frames)
			}
			text := ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
				}
				if chunk.ProcessFrame != nil {
					t.Fatal("native ID tracker entered continuation results")
				}
			}
			if text != "ACD" || !hasChunkKind(chunks, ChunkDone) {
				t.Fatal("duplicate native ID changed original continuation or provider completion")
			}
		})
	}
}

func TestOpaqueNativeIDCannotRestartAtDifferentIndex(t *testing.T) {
	lines := []string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"same-native-source","type":"reasoning","encrypted_content":"opaque-private"}}`,
		`data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"same-native-source","delta":"A"}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"same-native-source","type":"reasoning"}}`,
		`data: {"type":"response.reasoning_text.delta","output_index":1,"item_id":"same-native-source","delta":"C"}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"same-native-source","type":"reasoning","encrypted_content":"opaque-private"}}`,
		`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"same-native-source","type":"reasoning"}}`,
		`data: {"type":"response.completed"}`,
	}
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(strings.Join(lines, "\n\n")+"\n\n"), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, *chunk.ProcessFrame)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 0 {
		t.Fatalf("opaque native ID regained display authority at another index: %#v", frames)
	}
	text := ""
	for _, chunk := range chunks {
		if chunk.Kind == ChunkReasoning {
			text += chunk.Text
		}
		if chunk.ProcessFrame != nil {
			t.Fatal("poisoned source identity entered continuation results")
		}
	}
	if text != "AC" || !hasChunkKind(chunks, ChunkDone) {
		t.Fatal("opaque identity refusal changed original continuation or provider completion")
	}
}

func TestConflictingNativeIDCannotRestartAtDifferentIndex(t *testing.T) {
	responseStartA := `data: {"type":"response.output_item.added","output_index":0,"item":{"id":"source-A","type":"reasoning"}}`
	responseA := `data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"source-A","delta":"A"}`
	responseStartB := `data: {"type":"response.output_item.added","output_index":1,"item":{"id":"source-B","type":"reasoning"}}`
	responseC := `data: {"type":"response.reasoning_text.delta","output_index":1,"item_id":"source-B","delta":"C"}`
	responseDoneA := `data: {"type":"response.output_item.done","output_index":0,"item":{"id":"source-A","type":"reasoning"}}`
	responseDoneB := `data: {"type":"response.output_item.done","output_index":1,"item":{"id":"source-B","type":"reasoning"}}`
	responseCompleted := `data: {"type":"response.completed"}`
	tests := []struct {
		name, format, text string
		initialCompleted   bool
		lines              []string
	}{
		{"responses duplicate index with new ID", "responses", "AC", false, []string{responseStartA, responseA,
			`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"source-B","type":"reasoning"}}`,
			responseStartB, responseC, responseDoneA, responseDoneB, responseCompleted}},
		{"responses mismatched delta ID", "responses", "ABC", false, []string{responseStartA, responseA,
			`data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"source-B","delta":"B"}`,
			responseStartB, responseC, responseDoneA, responseDoneB, responseCompleted}},
		{"responses mismatched done ID", "responses", "AC", false, []string{responseStartA, responseA,
			`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"source-B","type":"reasoning"}}`,
			responseStartB, responseC, responseDoneA, responseDoneB, responseCompleted}},
		{"responses missing index start with new ID", "responses", "AC", false, []string{responseStartA, responseA,
			`data: {"type":"response.output_item.added","item":{"id":"source-B","type":"reasoning"}}`,
			responseStartB, responseC, responseDoneA, responseDoneB, responseCompleted}},
		{"responses completed copy precedes conflict", "responses", "AC", true, []string{responseStartA, responseA, responseDoneA,
			`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"source-B","type":"reasoning"}}`,
			responseStartB, responseC, responseDoneB, responseCompleted}},
		{"messages duplicate index with new ID", "messages", "AC", false, []string{
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","id":"source-A","thinking":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"A"}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","id":"source-B","thinking":""}}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","id":"source-B","thinking":""}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"C"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"private-signature-unchanged"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"content_block_stop","index":1}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
			`data: {"type":"message_stop"}`,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithCallback(test.format, strings.NewReader(strings.Join(test.lines, "\n\n")+"\n\n"), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames = append(frames, *chunk.ProcessFrame)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			expectedFrames := 2
			if test.initialCompleted {
				expectedFrames = 3
			}
			if len(frames) != expectedFrames || frames[0].Kind != "start" || frames[0].NativeIndex != 0 || frames[0].NativeID != "source-A" || frames[1].Kind != "delta" || frames[1].Text != "A" || test.initialCompleted && frames[2].Kind != "end" {
				t.Fatalf("conflicting actual ID regained framing authority: %#v", frames)
			}
			text, signature := "", ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
					signature += chunk.Signature
				}
				if chunk.ProcessFrame != nil {
					t.Fatal("seen source IDs entered continuation results")
				}
			}
			if text != test.text || !hasChunkKind(chunks, ChunkDone) || test.format == "messages" && signature != "private-signature-unchanged" {
				t.Fatal("conflicting ID refusal changed original text, signature or provider completion")
			}
		})
	}
}

func TestNativeIDBudgetOverflowStopsFutureFramingWithoutChangingContinuation(t *testing.T) {
	for _, budget := range []string{"distinct IDs", "ID bytes"} {
		t.Run(budget, func(t *testing.T) {
			lines := []string{
				`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"source-A","type":"reasoning"}}`,
				`data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"source-A","delta":"A"}`,
			}
			if budget == "distinct IDs" {
				// All conflicts use one actual index: ID exhaustion must work independently
				// of the retained native-index quota. Each real ID is at most 256 bytes.
				for count := 1; count <= 256; count++ {
					id := strings.Repeat("x", count)
					lines = append(lines, `data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"`+id+`","delta":""}`)
				}
			} else {
				lines = append(lines, `data: {"type":"response.output_item.added","output_index":9,"item":{"id":"`+strings.Repeat("x", 257)+`","type":"reasoning"}}`)
			}
			lines = append(lines,
				`data: {"type":"response.reasoning_text.delta","output_index":0,"item_id":"source-A","delta":"C"}`,
				`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"source-A","type":"reasoning"}}`,
				`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"source-new","type":"reasoning"}}`,
				`data: {"type":"response.reasoning_text.delta","output_index":1,"item_id":"source-new","delta":"D"}`,
				`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"source-new","type":"reasoning"}}`,
				`data: {"type":"response.completed"}`,
			)
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(strings.Join(lines, "\n\n")+"\n\n"), func(chunk Chunk) error {
				if chunk.ProcessFrame != nil {
					frames = append(frames, *chunk.ProcessFrame)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(frames) != 2 || frames[0].Kind != "start" || frames[1].Kind != "delta" || frames[1].Text != "A" {
				t.Fatalf("ID budget exhaustion allowed future private framing: %#v", frames)
			}
			text := ""
			for _, chunk := range chunks {
				if chunk.Kind == ChunkReasoning {
					text += chunk.Text
				}
				if chunk.ProcessFrame != nil {
					t.Fatal("ID budget entered continuation results")
				}
			}
			if text != "ACD" || !hasChunkKind(chunks, ChunkDone) {
				t.Fatal("ID budget changed original continuation or provider completion")
			}
		})
	}
}

func TestResponsesIncompletePreservesOutputCapObservation(t *testing.T) {
	for _, reason := range []string{"max_output_tokens", "unknown_private_reason"} {
		t.Run(reason, func(t *testing.T) {
			payload := `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"` + reason + `"},"usage":{"input_tokens":7,"output_tokens":9,"total_tokens":16}}}` + "\n\n"
			var delivered []domainmodel.Chunk
			chunks, usage, err := ParseSSEWithCallback("responses", strings.NewReader(payload), func(chunk domainmodel.Chunk) error { delivered = append(delivered, chunk); return nil })
			if (reason == "max_output_tokens") != (err == nil) || usage.PromptTokens != 7 || usage.CompletionTokens != 9 {
				t.Fatalf("incomplete observation lost: usage=%#v err=%v", usage, err)
			}
			if reason == "max_output_tokens" && usage.FinishReason != "length" {
				t.Fatalf("output cap not normalized: %#v", usage)
			}
			if reason != "max_output_tokens" && usage.FinishReason == "length" {
				t.Fatal("unknown incomplete reason guessed length")
			}
			for _, chunk := range append(chunks, delivered...) {
				if chunk.Kind == domainmodel.ChunkToolCall || reason != "max_output_tokens" && chunk.Kind == domainmodel.ChunkDone {
					t.Fatal("unknown incomplete stream published completion/tool")
				}
			}
		})
	}
}

func TestAssistantDraftObserverPreservesAllProviderParsersV1(t *testing.T) {
	tests := []struct{ format, payload, class, raw, final string }{
		{"chat_completions", strings.Join([]string{
			`data: {"choices":[{"delta":{"content":"<think>private inline "}}]}`,
			`data: {"choices":[{"delta":{"content":"reasoning</think>answer "}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}, "\n\n") + "\n\n", "unclassified_inline", "<think>private inline reasoning</think>answer ", "answer "},
		{"responses", strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"first "}`,
			`data: {"type":"response.output_text.delta","delta":"second "}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n") + "\n\n", "provider_text", "first second ", "first second "},
		{"messages", strings.Join([]string{
			`data: {"type":"message_start","message":{"model":"fixture","usage":{"input_tokens":4,"output_tokens":0}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"first "}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"second "}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
			`data: {"type":"message_stop"}`,
		}, "\n\n") + "\n\n", "provider_text", "first second ", "first second "},
	}
	for _, c := range tests {
		t.Run(c.format, func(t *testing.T) {
			baseline, usage, err := ParseSSE(c.format, strings.NewReader(c.payload))
			if err != nil {
				t.Fatal(err)
			}
			raw := ""
			count := 0
			chunks, gotUsage, err := ParseSSEWithPrivateDisplay(c.format, strings.NewReader(c.payload), nil, nil, func(frame domainmodel.ProcessFrameV1) {
				if frame.Source != "assistant_draft" || frame.ContentClassification != c.class || frame.Kind != "delta" || frame.NativeID != "" || frame.NativeIndex != 0 {
					t.Fatal("private draft source misclassified")
				}
				raw += frame.Text
				count++
				if count == 1 {
					panic("optional display observer failure")
				}
			})
			if err != nil || count != 2 || raw != c.raw || !reflect.DeepEqual(usage, gotUsage) {
				t.Fatal("draft observation changed parser outcome")
			}
			final := ""
			for i := range chunks {
				if chunks[i].ProcessFrame != nil {
					t.Fatal("draft reached original result")
				}
				if chunks[i].Kind == ChunkText {
					final += chunks[i].Text
				}
				chunks[i].Trace = domainmodel.ChunkTrace{}
				baseline[i].Trace = domainmodel.ChunkTrace{}
			}
			if final != c.final || !reflect.DeepEqual(chunks, baseline) {
				t.Fatal("draft observation altered original parsing or continuation")
			}
		})
	}
}

func TestAssistantDraftObserverPreservesOrphanInlineRecoveryV1(t *testing.T) {
	const private = "PRIVATE_ORPHAN_DRAFT_PREFIX"
	payload := `data: {"choices":[{"delta":{"content":"` + private + ` </think>accepted after orphan"}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" + `data: [DONE]` + "\n\n"
	raw := ""
	chunks, _, err := ParseSSEWithPrivateDisplay("chat_completions", strings.NewReader(payload), nil, nil, func(frame domainmodel.ProcessFrameV1) {
		if frame.ContentClassification != "unclassified_inline" {
			t.Fatal("orphan source claimed final classification")
		}
		raw += frame.Text
	})
	if err != nil || !strings.Contains(raw, private) {
		t.Fatal("orphan source observation/recovery changed")
	}
	text := ""
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil || strings.Contains(chunk.Text, private) || strings.Contains(chunk.Text, "</think>") {
			t.Fatal("orphan draft became parser output")
		}
		if chunk.Kind == ChunkText {
			text += chunk.Text
		}
	}
	if strings.TrimSpace(text) != "accepted after orphan" {
		t.Fatal("pre-existing orphan recovery changed")
	}
}

func TestToolPreparationSidecarUsesNamedFragmentsAndAuthoritativeFinalV1(t *testing.T) {
	streamed := `{"path":"draft.txt","content":"A😀"}`
	final := `{"path":"final.txt","content":"done"}`
	for _, tc := range []struct {
		name, format, payload, wantFinal string
	}{
		{"chat", "chat_completions", strings.Join([]string{
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"raw_chat","function":{"name":"write","arguments":"{\"path\":\"draft.txt\",\"content\":\"A"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"😀\"}"}}]}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}, "\n\n"), streamed},
		{"messages", "messages", strings.Join([]string{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":3,"output_tokens":0}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"raw_messages","name":"write","input":{}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"draft.txt\",\"content\":\"A"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"😀\"}"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`,
			`data: {"type":"message_stop"}`,
		}, "\n\n"), streamed},
		{"responses repeated final", "responses", strings.Join([]string{
			`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"item_responses","call_id":"raw_responses","name":"write","arguments":""}}`,
			`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"item_responses","delta":"{\"path\":\"draft.txt\",\"content\":\"A😀\"}"}`,
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"item_responses","call_id":"raw_responses","name":"write","arguments":"{\"path\":\"draft.txt\",\"content\":\"A😀\"}"}}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n"), streamed},
		{"responses conflicting final", "responses", strings.Join([]string{
			`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"item_responses","call_id":"raw_responses","name":"write","arguments":""}}`,
			`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"item_responses","delta":"{\"path\":\"draft.txt\",\"content\":\"A😀\"}"}`,
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"item_responses","call_id":"raw_responses","name":"write","arguments":"{\"path\":\"final.txt\",\"content\":\"done\"}"}}`,
			`data: {"type":"response.completed"}`,
		}, "\n\n"), final},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var frames []domainmodel.ProcessFrameV1
			chunks, _, err := ParseSSEWithPrivateDisplay(tc.format, strings.NewReader(tc.payload), nil, nil, func(frame domainmodel.ProcessFrameV1) {
				if frame.Source == "tool_preparation" {
					frames = append(frames, frame)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(frames) < 3 || frames[0].Kind != "start" || frames[0].ToolPreparation == nil || frames[len(frames)-1].Kind != "end" {
				t.Fatalf("real preparation boundaries = %#v", frames)
			}
			callID := frames[1].ToolPreparation.CallID
			for _, frame := range frames {
				if frame.Kind == "start" {
					if frame.ToolPreparation == nil || frame.ToolPreparation.ToolName != "" || frame.ToolPreparation.Arguments != "" || frame.Text != "" {
						t.Fatal("origin carried a name or body")
					}
					continue
				}
				if frame.ToolPreparation == nil || frame.ToolPreparation.CallID != callID || frame.ToolPreparation.ToolName != "write" || frame.ToolPreparation.Index != 0 {
					t.Fatalf("unstable call coordinate = %#v", frame)
				}
			}
			if frames[len(frames)-1].ToolPreparation.Arguments != tc.wantFinal {
				t.Fatal("end appended a full final as another delta")
			}
			finalCalls := 0
			for _, chunk := range chunks {
				if chunk.ProcessFrame != nil {
					t.Fatal("display frame entered retained continuation")
				}
				if chunk.Kind == ChunkToolCall {
					finalCalls++
					if string(chunk.ToolCall.Arguments) != tc.wantFinal {
						t.Fatal("Core final did not use the authoritative complete arguments")
					}
				}
			}
			if finalCalls != 1 {
				t.Fatalf("final calls = %d", finalCalls)
			}
			without, _, plainErr := ParseSSE(tc.format, strings.NewReader(tc.payload))
			for index := range chunks {
				chunks[index].Trace = domainmodel.ChunkTrace{}
			}
			for index := range without {
				without[index].Trace = domainmodel.ChunkTrace{}
			}
			if plainErr != nil || !reflect.DeepEqual(chunks, without) {
				t.Fatal("optional display observer changed continuation")
			}
			if _, _, callbackErr := ParseSSEWithPrivateDisplay(tc.format, strings.NewReader(tc.payload), nil, nil, func(domainmodel.ProcessFrameV1) { panic("observer failure") }); callbackErr != nil {
				t.Fatal("optional observer failure changed parsing")
			}
		})
	}
	var frames []domainmodel.ProcessFrameV1
	a := newToolCallAccumulator()
	a.onPreparation = func(frame domainmodel.ProcessFrameV1) { frames = append(frames, frame) }
	a.add(0, "raw_late_name", "", `{"path":`)
	if len(frames) != 1 || frames[0].Kind != "start" || frames[0].ToolPreparation.ToolName != "" || frames[0].ToolPreparation.Arguments != "" {
		t.Fatal("unnamed observation created a named view or carried arguments")
	}
	a.add(0, "", "write", `"a"}`)
	if len(frames) != 2 || frames[1].ToolPreparation.Arguments != `{"path":"a"}` {
		t.Fatal("first named view lost the actual previously accumulated prefix")
	}
	a.add(0, "", "", " ")
	if len(frames) != 3 || frames[2].ToolPreparation.Arguments != " " {
		t.Fatal("later named observation duplicated the first prefix")
	}
}

func TestChatReasoningContentUsesHostSegmentsAndPreservesContinuationV1(t *testing.T) {
	payload := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"first\n"}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"paragraph\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"answer"}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"second\n"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n")
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithCallback("chat_completions", strings.NewReader(payload), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, *chunk.ProcessFrame)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{}
	ordinals := []uint64{}
	for _, frame := range frames {
		if frame.Source != "provider_reasoning" || frame.NativeID != "" || frame.NativeIndex != 0 {
			t.Fatalf("provider reasoning invented a native coordinate: %#v", frame)
		}
		kinds = append(kinds, frame.Kind)
		ordinals = append(ordinals, frame.HostSegmentOrdinal)
	}
	if !reflect.DeepEqual(kinds, []string{"start", "delta", "delta", "end", "start", "delta", "end"}) ||
		!reflect.DeepEqual(ordinals, []uint64{1, 1, 1, 1, 2, 2, 2}) {
		t.Fatalf("host segments = %v / %v", kinds, ordinals)
	}
	reasoning := ""
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil || chunk.Kind == ChunkReasoning && chunk.Text == "" {
			t.Fatal("display-only shell entered continuation")
		}
		if chunk.Kind == ChunkReasoning {
			reasoning += chunk.Text
		}
	}
	if reasoning != "first\nparagraph\nsecond\n" {
		t.Fatal("existing private reasoning continuation changed")
	}
	ends := 0
	_, _, interrupted := ParseSSEWithCallback("chat_completions", strings.NewReader(`data: {"choices":[{"delta":{"reasoning_content":"incomplete"}}]}`), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil && chunk.ProcessFrame.Kind == "end" {
			ends++
		}
		return nil
	})
	if interrupted == nil || ends != 0 {
		t.Fatal("interrupted reasoning acquired a fabricated completion")
	}
}

func TestSameChatPayloadPreservesPrivateSourceOrderV1(t *testing.T) {
	payload := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"actual reasoning\n","tool_calls":[{"index":0,"id":"raw_same_payload","function":{"name":"write","arguments":"{\"path\":\"actual.txt\",\"content\":\"A\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n")
	var frames []string
	var logical []ChunkKind
	chunks, _, err := ParseSSEWithPrivateDisplay("chat_completions", strings.NewReader(payload), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, chunk.ProcessFrame.Source+"/"+chunk.ProcessFrame.Kind)
		}
		if chunk.Kind != ChunkReasoning || chunk.Text != "" {
			logical = append(logical, chunk.Kind)
		}
		return nil
	}, nil, func(frame domainmodel.ProcessFrameV1) {
		if frame.Source == "tool_preparation" {
			frames = append(frames, frame.Source+"/"+frame.Kind)
		}
	})
	wantFrames := []string{"provider_reasoning/start", "provider_reasoning/delta", "provider_reasoning/end", "tool_preparation/start", "tool_preparation/delta", "tool_preparation/end"}
	wantLogical := []ChunkKind{ChunkReasoning, ChunkToolCallStart, ChunkToolCall, ChunkDone}
	if err != nil || !reflect.DeepEqual(frames, wantFrames) || !reflect.DeepEqual(logical, wantLogical) {
		t.Fatalf("same-payload source order changed: frames=%v logical=%v err=%v", frames, logical, err)
	}
	if len(chunks) != len(wantLogical) {
		t.Fatal("private scheduling changed retained chunk count")
	}
	for i, chunk := range chunks {
		if chunk.ProcessFrame != nil || chunk.Kind != wantLogical[i] {
			t.Fatal("private scheduling entered or reordered continuation")
		}
	}
}

func TestChatUnnamedToolOriginPrecedesReasoningWithoutChangingContinuationV1(t *testing.T) {
	payload := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"raw_early_slot","function":{"arguments":"{\"path\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"reasoning_content":"R\n"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"write","arguments":"\"a.txt\",\"content\":\"x\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}, "\n\n")
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithPrivateDisplay("chat_completions", strings.NewReader(payload), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, *chunk.ProcessFrame)
		}
		return nil
	}, nil, func(frame domainmodel.ProcessFrameV1) {
		if frame.Source == "tool_preparation" {
			frames = append(frames, frame)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var lifecycle []string
	for _, f := range frames {
		lifecycle = append(lifecycle, f.Source+"/"+f.Kind)
	}
	want := []string{"tool_preparation/start", "provider_reasoning/start", "provider_reasoning/delta", "provider_reasoning/end", "tool_preparation/delta", "tool_preparation/end"}
	if !reflect.DeepEqual(lifecycle, want) {
		t.Fatalf("earliest real tool observation missing or reordered: %v", lifecycle)
	}
	origin := frames[0]
	if origin.Text != "" || origin.ToolPreparation == nil || origin.ToolPreparation.Index != 0 || origin.ToolPreparation.CallID != "raw_early_slot" || origin.ToolPreparation.ToolName != "" || origin.ToolPreparation.Arguments != "" {
		t.Fatal("unnamed origin released bytes or invented a named preparing view")
	}
	if frames[4].ToolPreparation.Arguments != `{"path":"a.txt","content":"x"}` {
		t.Fatal("named view lost the accumulated real prefix")
	}
	baseline, _, baselineErr := ParseSSE("chat_completions", strings.NewReader(payload))
	for i := range chunks {
		chunks[i].Trace = domainmodel.ChunkTrace{}
	}
	for i := range baseline {
		baseline[i].Trace = domainmodel.ChunkTrace{}
	}
	if baselineErr != nil || !reflect.DeepEqual(chunks, baseline) {
		t.Fatal("private origin changed logical continuation")
	}
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("private origin entered retained history")
		}
	}
}

func TestNativeCounterPresenceUsesExactEndpointAndPreservesReasoningZero(t *testing.T) {
	for _, tc := range []struct {
		body                     string
		input, output, reasoning bool
	}{
		{`{"usage":{"prompt_tokens":0,"completion_tokens":0,"completion_tokens_details":{"reasoning_tokens":0}},"choices":[]}`, true, true, true},
		{`{"usage":{"prompt_tokens":7},"choices":[]}`, true, false, false},
		{`{"usage":{"input_tokens":5,"output_tokens":7},"choices":[]}`, false, false, false},
		{`{"usage":{"prompt_tokens":null,"completion_tokens":null},"choices":[]}`, false, false, false},
	} {
		_, usage, _, _, err := ParseOpenAIChatSSEPayload(tc.body)
		if err != nil || !usage.UsagePresenceKnown || usage.HasPromptTokens != tc.input || usage.HasCompletionTokens != tc.output || usage.HasReasoningTokens != tc.reasoning {
			t.Fatalf("native presence mismatch: %#v err=%v", usage, err)
		}
	}
}

func TestNativeFirstContentTimingExcludesControlAndEmptyFrames(t *testing.T) {
	for _, tc := range []struct {
		endpoint, body string
		want           bool
	}{
		{"chat_completions", `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call"}]}}]}`, false},
		{"chat_completions", `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{"}}]}}]}`, true},
		{"responses", `{"type":"response.output_text.delta","delta":""}`, false},
		{"responses", `{"type":"response.output_item.added","item":{"type":"reasoning"}}`, false},
		{"responses", `{"type":"response.function_call_arguments.delta","delta":"{"}`, true},
		{"responses", `{"type":"response.output_item.done","item":{"type":"function_call","arguments":"{}"}}`, true},
		{"messages", `{"type":"content_block_start","content_block":{"type":"tool_use","input":{"a":1}}}`, true},
		{"messages", `{"type":"content_block_start","content_block":{"type":"tool_use","input":{}}}`, false},
		{"messages", `{"type":"content_block_delta","delta":{"type":"signature_delta","signature":"opaque"}}`, false},
		{"messages", `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"actual"}}`, true},
	} {
		if got := nativePayloadHasContentV1(tc.endpoint, tc.body); got != tc.want {
			t.Fatalf("%s first content=%v want=%v body=%s", tc.endpoint, got, tc.want, tc.body)
		}
	}
	at := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	calls := 0
	observe := nativeFirstContentObserverV1("chat_completions", []func(time.Time){func(got time.Time) {
		calls++
		if !got.Equal(at) {
			t.Fatal("first native timestamp changed")
		}
	}})
	observe(`{"choices":[{"delta":{"content":"first"}}]}`, at)
	observe(`{"choices":[{"delta":{"content":"second"}}]}`, at.Add(time.Second))
	if calls != 1 {
		t.Fatal("first token callback is not idempotent")
	}
}

func TestNativeUsageSnapshotsKeepReportedCountersSeparateFromFinalQualification(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint, body string
		output               int
		final                bool
	}{
		{"messages start", "messages", `data: {"type":"message_start","message":{"usage":{"input_tokens":3,"output_tokens":1}}}

data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"actual"}}`, 1, false},
		{"messages interim", "messages", `data: {"type":"message_start","message":{"usage":{"input_tokens":3,"output_tokens":0}}}

data: {"type":"message_delta","usage":{"output_tokens":3}}`, 3, false},
		{"messages split final", "messages", `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

data: {"type":"message_delta","usage":{"output_tokens":0}}`, 0, true},
		{"chat no upgrade on done", "chat_completions", `data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"completion_tokens_details":{"reasoning_tokens":0}}}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]`, 1, false},
		{"chat partial snapshots", "chat_completions", `data: {"choices":[],"usage":{"prompt_tokens":10}}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"completion_tokens":2}}

data: [DONE]`, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, usage, _ := ParseSSE(tc.endpoint, strings.NewReader(tc.body))
			if !usage.HasCompletionTokens || usage.CompletionTokens != tc.output || (usage.OutputTokensFinality == domaincache.ProviderOutputTokensFinalV1) != tc.final {
				t.Fatalf("reported/final qualification lost: %#v", usage)
			}
			if tc.name == "chat partial snapshots" && (!usage.HasPromptTokens || usage.PromptTokens != 10) {
				t.Fatal("missing later field erased earlier input")
			}
			if tc.name == "chat no upgrade on done" && usage.ReasoningTokensFinality != domaincache.ProviderOutputTokensProvisionalV1 {
				t.Fatal("Done upgraded interim reasoning")
			}
		})
	}
	_, partial, _, _, err := ParseOpenAIChatSSEPayload(`{"choices":[],"usage":{"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":5}}}`)
	if err != nil || partial.HasPromptTokens || !partial.HasCompletionTokens || !partial.HasCacheHit || partial.HasCacheMiss || partial.CacheHitTokens != 5 {
		t.Fatalf("partial numeric report rejected: %#v %v", partial, err)
	}
	if _, _, _, _, err := ParseOpenAIChatSSEPayload(`{"choices":[],"usage":{"prompt_tokens":-1,"completion_tokens":20}}`); err == nil {
		t.Fatal("negative input escaped cache gate")
	}
	for _, body := range []string{`{"type":"content_block_start","content_block":{"type":"tool_use","input":{ }}}`, `{"type":"content_block_start","content_block":{"type":"tool_use","input":{
 }}}`} {
		if nativePayloadHasContentV1("messages", body) {
			t.Fatal("empty input placeholder became a token")
		}
	}
	for _, tc := range []struct{ endpoint, body string }{{"responses", `{"type":"response.output_item.added","item":{"type":"function_call","name":"read"}}`}, {"messages", `{"type":"content_block_start","content_block":{"type":"tool_use","name":"read","input":{}}}`}} {
		if !nativePayloadHasContentV1(tc.endpoint, tc.body) {
			t.Fatal("native tool name was not observed")
		}
	}
}

func TestNativeReportedUsageSurvivesLocalConsumerFailure(t *testing.T) {
	stop := errors.New("synthetic local consumer stop")
	for _, tc := range []struct{ endpoint, body string }{
		{"chat_completions", `data: {"choices":[],"usage":{"completion_tokens":2}}

data: {"choices":[{"delta":{"content":"actual"}}]}`},
		{"responses", `data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":2}}}`},
	} {
		_, u, err := ParseSSEWithCallbackAndPayloadObservation(tc.endpoint, strings.NewReader(tc.body), func(Chunk) error { return stop }, nil)
		if err == nil || !u.HasCompletionTokens || u.CompletionTokens != 2 {
			t.Fatalf("reported usage dropped at %s callback: %#v %v", tc.endpoint, u, err)
		}
	}
}

func TestOutputCapRequiresProtocolTerminationAndPreservesMissingUsage(t *testing.T) {
	fixtures := map[string]string{
		"chat_completions": `data: {"choices":[{"delta":{},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n",
		"responses":        `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n",
		"messages":         `data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}` + "\n\ndata: {\"type\":\"message_stop\"}\n\n",
	}
	for format, payload := range fixtures {
		t.Run(format, func(t *testing.T) {
			chunks, usage, err := ParseSSE(format, strings.NewReader(payload))
			if err != nil || usage.FinishReason != "length" || !hasChunkKind(chunks, ChunkDone) ||
				!usage.UsagePresenceKnown || usage.HasPromptTokens || usage.HasCompletionTokens {
				t.Fatalf("empty cap changed native usage: chunks=%#v usage=%#v err=%v", chunks, usage, err)
			}
			if hasChunkKind(chunks, ChunkUsage) {
				t.Fatal("missing native usage became a usage frame")
			}
		})
	}
	// A stop reason is not the Messages protocol termination boundary.
	chunks, usage, err := ParseSSE("messages", strings.NewReader(`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}`+"\n\n"))
	if err == nil || hasChunkKind(chunks, ChunkDone) || usage.FinishReason != "length" {
		t.Fatal("missing message_stop became output-cap completion")
	}
}

func TestOutputCapDoesNotValidateOrFlushTruncatedToolJSON(t *testing.T) {
	for _, args := range []string{`{}`, `{"partial":`} {
		argJSON, _ := json.Marshal(args)
		payload := `data: {"choices":[{"delta":{"content":"safe partial","tool_calls":[{"index":0,"id":"cap-call","type":"function","function":{"name":"ls","arguments":` + string(argJSON) + `}}]}}]}` +
			"\n\n" + `data: {"choices":[{"delta":{},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n"
		chunks, usage, err := ParseSSE("chat_completions", strings.NewReader(payload))
		if err != nil || usage.FinishReason != "length" || !hasChunkKind(chunks, ChunkDone) || hasChunkKind(chunks, ChunkToolCall) {
			t.Fatalf("capped JSON should not be dispatched or decoded: args=%q chunks=%#v err=%v", args, chunks, err)
		}
	}
}

func TestOutputCapDefersJSONValidationUntilMessagesOrResponsesTerminal(t *testing.T) {
	for _, args := range []string{`{}`, `{"partial":`} {
		argJSON, _ := json.Marshal(args)
		fixtures := map[string]string{
			"responses": `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"wire-id","call_id":"cap-call","name":"ls"}}` + "\n\n" +
				`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":` + string(argJSON) + `}` + "\n\n" +
				`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"wire-id","call_id":"cap-call","name":"ls"}}` + "\n\n" +
				`data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n",
			"messages": `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"cap-call","name":"ls"}}` + "\n\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(argJSON) + `}}` + "\n\n" +
				`data: {"type":"content_block_stop","index":0}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"}}` + "\n\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for format, payload := range fixtures {
			chunks, usage, err := ParseSSE(format, strings.NewReader(payload))
			if err != nil || usage.FinishReason != "length" || !hasChunkKind(chunks, ChunkDone) {
				t.Fatalf("capped args validated before terminal: format=%s args=%s err=%v", format, args, err)
			}
			if args != `{}` {
				ordinary := strings.ReplaceAll(payload, "max_output_tokens", "stop")
				if format == "responses" {
					ordinary = strings.ReplaceAll(ordinary, "response.incomplete", "response.completed")
				} else {
					ordinary = strings.ReplaceAll(ordinary, "max_tokens", "tool_use")
				}
				if _, _, err := ParseSSE(format, strings.NewReader(ordinary)); err == nil {
					t.Fatal("non-cap malformed JSON lost fail-closed validation")
				}
			}
		}
	}
}

func TestOutputCapPreservesKnownZeroUsage(t *testing.T) {
	fixtures := map[string]string{
		"chat_completions": `data: {"choices":[{"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}` + "\n\ndata: [DONE]\n\n",
		"responses":        `data: {"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}` + "\n\n",
		"messages":         `data: {"type":"message_start","message":{"usage":{"input_tokens":0,"output_tokens":0}}}` + "\n\n" + `data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":0}}` + "\n\ndata: {\"type\":\"message_stop\"}\n\n",
	}
	for format, payload := range fixtures {
		_, usage, err := ParseSSE(format, strings.NewReader(payload))
		inputKnown := usage.HasPromptTokens
		if format == "messages" {
			inputKnown = usage.MessagesInput.Uncached.Known && usage.MessagesInput.Uncached.Value == 0
		}
		if err != nil || !inputKnown || !usage.HasCompletionTokens || usage.PromptTokens != 0 || usage.CompletionTokens != 0 {
			t.Fatalf("known-zero cap lost native presence: %s %#v %v", format, usage, err)
		}
	}
}

func TestOutputCapDiscardedBudgetMaterialIsPrivateAndOnlyPending(t *testing.T) {
	accumulator := newToolCallAccumulator()
	accumulator.add(0, "actual-call", "", `PRIVATE_PENDING_ARGUMENT`)
	material := accumulator.discardedOutputMaterialV1()
	if material != "actual-callPRIVATE_PENDING_ARGUMENT" {
		t.Fatal("discard budget invented a tool name or lost actual arguments")
	}
	done := Chunk{Kind: ChunkDone, DiscardedToolMaterial: material}
	privateWire, err := json.Marshal(done)
	legacyWire, _ := json.Marshal(Chunk{Kind: ChunkDone})
	if err != nil || string(privateWire) != string(legacyWire) {
		t.Fatal("private budget field changed historical chunk wire")
	}

	accumulator = newToolCallAccumulator()
	accumulator.add(0, "actual-call", "ls", `{}`)
	flushed, err := accumulator.flushIndex(0)
	if err != nil || len(flushed) != 1 || accumulator.discardedOutputMaterialV1() != "" {
		t.Fatal("already-flushed normal arguments were counted again")
	}
}
