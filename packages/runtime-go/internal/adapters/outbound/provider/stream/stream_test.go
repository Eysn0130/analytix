package stream

import (
	"strings"
	"testing"
	"time"

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
		{finishReason: "length", failureCode: domainfailure.CodeProviderStreamInterrupted},
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
	chunks, _, err := ParseSSEWithCallback("chat_completions", strings.NewReader(strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`data: {"choices":[{"delta":{"content":"1\n"}}]}`,
		`data: {"choices":[{"delta":{"content":"2\n"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}, "\n\n")), func(chunk Chunk) error {
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
			if reason != "end_turn" {
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
			if err != nil || usage.CompletionTokens != 5 || usage.PromptTokens != 5 || usage.FinishReason != "stop" || !usage.HasCacheHit || usage.CacheHitTokens != 0 {
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
