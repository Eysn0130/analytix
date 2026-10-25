package stream

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func summaryEventForTest(kind string, fields map[string]any) map[string]any {
	fields["type"] = kind
	return fields
}

func summaryItemForTest(kind string, index int, id string, texts ...string) map[string]any {
	parts := []any{}
	for _, text := range texts {
		parts = append(parts, map[string]any{"type": "summary_text", "text": text})
	}
	return summaryEventForTest(kind, map[string]any{"output_index": index, "item": map[string]any{"id": id, "type": "reasoning", "summary": parts}})
}

func summaryPartForTest(kind string, index int, id string, part, seq int, text string) map[string]any {
	fields := map[string]any{"output_index": index, "item_id": id, "summary_index": part, "sequence_number": seq}
	switch kind {
	case "response.reasoning_summary_text.delta":
		fields["delta"] = text
	case "response.reasoning_summary_text.done":
		fields["text"] = text
	default:
		fields["part"] = map[string]any{"type": "summary_text", "text": text}
	}
	return summaryEventForTest(kind, fields)
}

func summaryFinishForTest(events []map[string]any) []map[string]any {
	return append(events,
		summaryEventForTest("response.output_text.delta", map[string]any{"output_index": 1000, "item_id": "answer", "delta": "summary-final-answer"}),
		summaryEventForTest("response.completed", map[string]any{"response": map[string]any{}}),
	)
}

func summaryTranscriptForTest(t *testing.T, events []map[string]any) string {
	t.Helper()
	var body strings.Builder
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		body.WriteString("data: ")
		body.Write(raw)
		body.WriteString("\n\n")
	}
	return body.String()
}

func parseSummaryForTest(t *testing.T, events []map[string]any) ([]domainmodel.ProcessFrameV1, []Chunk) {
	t.Helper()
	var frames []domainmodel.ProcessFrameV1
	chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(summaryTranscriptForTest(t, events)), func(chunk Chunk) error {
		if chunk.ProcessFrame != nil {
			frames = append(frames, *chunk.ProcessFrame)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("private summary changed provider completion: %v", err)
	}
	public := ""
	for _, chunk := range chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("private frame entered retained continuation")
		}
		if chunk.Kind == ChunkText {
			public += chunk.Text
		}
	}
	if public != "summary-final-answer" {
		t.Fatal("private display changed the final answer")
	}
	return frames, chunks
}

func TestResponsesSummaryPartsCompleteWithoutRetainingSummaryOrOpaqueBytes(t *testing.T) {
	parent := summaryItemForTest("response.output_item.added", 0, "reasoning-a")
	parent["item"].(map[string]any)["encrypted_content"] = map[string]any{"opaque": "ENCRYPTED_SUMMARY_CANARY"}
	parent["item"].(map[string]any)["redacted"] = "REDACTED_SUMMARY_CANARY"
	final := summaryItemForTest("response.output_item.done", 0, "reasoning-a", "first summary", "second summary")
	final["item"].(map[string]any)["status"] = "completed"
	final["item"].(map[string]any)["encrypted_content"] = "ENCRYPTED_SUMMARY_CANARY"
	frames, chunks := parseSummaryForTest(t, summaryFinishForTest([]map[string]any{
		parent,
		summaryPartForTest("response.reasoning_summary_part.added", 0, "reasoning-a", 0, 1, ""),
		summaryPartForTest("response.reasoning_summary_text.delta", 0, "reasoning-a", 0, 2, "first "),
		summaryPartForTest("response.reasoning_summary_text.delta", 0, "reasoning-a", 0, 2, "first "),
		summaryPartForTest("response.reasoning_summary_text.done", 0, "reasoning-a", 0, 3, "first summary"),
		summaryPartForTest("response.reasoning_summary_text.done", 0, "reasoning-a", 0, 4, "first summary"),
		summaryPartForTest("response.reasoning_summary_part.done", 0, "reasoning-a", 0, 5, "first summary"),
		summaryPartForTest("response.reasoning_summary_part.added", 0, "reasoning-a", 1, 6, "second "),
		summaryPartForTest("response.reasoning_summary_text.delta", 0, "reasoning-a", 1, 7, "summary"),
		summaryPartForTest("response.reasoning_summary_part.done", 0, "reasoning-a", 1, 8, "second summary"),
		final,
	}))
	bodies := map[uint64]string{}
	starts, ends := 0, 0
	for _, frame := range frames {
		if frame.Source != "provider_reasoning" || frame.ReasoningForm != "summary" || frame.NativeID != "" || frame.NativeIndex != 0 {
			t.Fatal("summary fabricated native coordinates or a raw reasoning seat")
		}
		if frame.Kind == "start" {
			starts++
		}
		if frame.Kind == "end" {
			ends++
		}
		bodies[frame.HostSegmentOrdinal] += frame.Text
	}
	if starts != 2 || ends != 2 || len(frames) != 8 || !reflect.DeepEqual(bodies, map[uint64]string{1: "first summary", 2: "second summary"}) {
		t.Fatalf("part snapshots duplicated or lost real text: starts=%d ends=%d frames=%d", starts, ends, len(frames))
	}
	for _, chunk := range chunks {
		if chunk.Kind == ChunkReasoning || strings.Contains(chunk.Text, "summary") && chunk.Kind != ChunkText || strings.Contains(chunk.Text, "CANARY") {
			t.Fatal("summary or opaque bytes became model continuation")
		}
	}
}

func TestResponsesSummaryDoneSnapshotsAndInterleavedItemsKeepDistinctParts(t *testing.T) {
	frames, _ := parseSummaryForTest(t, summaryFinishForTest([]map[string]any{
		summaryItemForTest("response.output_item.added", 0, "a"),
		summaryItemForTest("response.output_item.added", 1, "b"),
		summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 10, "prefix "),
		summaryPartForTest("response.reasoning_summary_part.added", 1, "b", 0, 1, ""),
		summaryPartForTest("response.reasoning_summary_text.done", 1, "b", 0, 2, "done-only"),
		summaryItemForTest("response.output_item.done", 1, "b", "done-only"),
		summaryItemForTest("response.output_item.done", 0, "a", "prefix suffix"),
	}))
	bodies := map[uint64]string{}
	ends := map[uint64]int{}
	for _, frame := range frames {
		bodies[frame.HostSegmentOrdinal] += frame.Text
		if frame.Kind == "end" {
			ends[frame.HostSegmentOrdinal]++
		}
	}
	if !reflect.DeepEqual(bodies, map[uint64]string{1: "prefix suffix", 2: "done-only"}) || !reflect.DeepEqual(ends, map[uint64]int{1: 1, 2: 1}) {
		t.Fatal("item/part identities or full-text suffix completion diverged")
	}
}

func TestResponsesSummaryInvalidEvidenceNeverCompletesPart(t *testing.T) {
	for _, name := range []string{"missing-item", "wrong-item", "missing-index", "wrong-index", "missing-part", "wrong-part", "missing-sequence", "negative-sequence", "fractional-sequence", "invalid-part", "divergent-snapshot", "late-delta", "invalid-status"} {
		t.Run(name, func(t *testing.T) {
			event := summaryPartForTest("response.reasoning_summary_text.delta", 0, "a", 0, 2, "REJECTED_SUMMARY_CANARY")
			switch name {
			case "missing-item":
				delete(event, "item_id")
			case "wrong-item":
				event["item_id"] = "other"
			case "missing-index":
				delete(event, "output_index")
			case "wrong-index":
				event["output_index"] = 9
			case "missing-part":
				delete(event, "summary_index")
			case "wrong-part":
				event["summary_index"] = 1
			case "missing-sequence":
				delete(event, "sequence_number")
			case "negative-sequence":
				event["sequence_number"] = -1
			case "fractional-sequence":
				event["sequence_number"] = 2.5
			case "invalid-part":
				event = summaryPartForTest("response.reasoning_summary_part.done", 0, "a", 0, 2, "REJECTED_SUMMARY_CANARY")
				event["part"].(map[string]any)["type"] = "encrypted_text"
			case "divergent-snapshot":
				event = summaryPartForTest("response.reasoning_summary_text.done", 0, "a", 0, 2, "REJECTED_SUMMARY_CANARY")
			case "invalid-status":
				event = summaryPartForTest("response.reasoning_summary_part.done", 0, "a", 0, 2, "safe ")
				event["status"] = "completed"
			}
			events := []map[string]any{summaryItemForTest("response.output_item.added", 0, "a"), summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 1, "safe ")}
			if name == "late-delta" {
				events = append(events, summaryPartForTest("response.reasoning_summary_text.done", 0, "a", 0, 2, "safe "))
				event["sequence_number"] = 3
			}
			events = append(events, event, summaryItemForTest("response.output_item.done", 0, "a", "safe "))
			frames, _ := parseSummaryForTest(t, summaryFinishForTest(events))
			for _, frame := range frames {
				if frame.Kind == "end" || strings.Contains(frame.Text, "REJECTED_SUMMARY_CANARY") {
					t.Fatal("invalid or late evidence completed/changed a summary")
				}
			}
		})
	}
}

func TestResponsesSummaryRejectedIdentityCannotReopenAtAnotherIndex(t *testing.T) {
	for _, invalidIndex := range []any{0, nil, -1, "0"} {
		orphan := summaryPartForTest("response.reasoning_summary_text.delta", 0, "reused", 0, 1, "orphan")
		orphan["output_index"] = invalidIndex
		frames, _ := parseSummaryForTest(t, summaryFinishForTest([]map[string]any{
			orphan,
			summaryItemForTest("response.output_item.added", 1, "reused"),
			summaryPartForTest("response.reasoning_summary_part.added", 1, "reused", 0, 2, "resurrected"),
			summaryItemForTest("response.output_item.done", 1, "reused", "resurrected"),
		}))
		if len(frames) != 0 {
			t.Fatal("poisoned native item ID regained summary authority at another index")
		}
	}
}

func TestResponsesSummaryIncompleteAndLateEventsCannotInventCompletion(t *testing.T) {
	incomplete := summaryPartForTest("response.reasoning_summary_part.done", 0, "a", 0, 2, "partial")
	incomplete["status"] = "incomplete"
	events := summaryFinishForTest([]map[string]any{
		summaryItemForTest("response.output_item.added", 0, "a"),
		summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 1, "partial"),
		incomplete,
		summaryItemForTest("response.output_item.done", 0, "a", "partial"),
		summaryPartForTest("response.reasoning_summary_text.delta", 0, "a", 0, 3, "AFTER_ITEM_DONE"),
	})
	events = append(events,
		summaryItemForTest("response.output_item.added", 1, "late"),
		summaryPartForTest("response.reasoning_summary_part.added", 1, "late", 0, 4, "AFTER_RESPONSE_DONE"),
		summaryItemForTest("response.output_item.done", 1, "late", "AFTER_RESPONSE_DONE"),
	)
	frames, _ := parseSummaryForTest(t, events)
	if len(frames) != 2 || frames[0].Kind != "start" || frames[1].Text != "partial" {
		t.Fatal("incomplete or closed stream produced a false end or late source")
	}
}

func TestResponsesSummaryBoundsDegradeOnlyPrivateDisplay(t *testing.T) {
	for _, mode := range []string{"bytes", "parts", "malformed-parent", "malformed-item-status"} {
		t.Run(mode, func(t *testing.T) {
			parent := summaryItemForTest("response.output_item.added", 0, "a")
			events := []map[string]any{parent}
			switch mode {
			case "bytes":
				events = append(events, summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 1, ""))
				for seq := 2; seq <= 6; seq++ {
					events = append(events, summaryPartForTest("response.reasoning_summary_text.delta", 0, "a", 0, seq, strings.Repeat("x", 256*1024)))
				}
			case "parts":
				for part := 0; part <= responsesSummaryPartLimitV1; part++ {
					events = append(events, summaryPartForTest("response.reasoning_summary_part.added", 0, "a", part, part+1, ""))
				}
			case "malformed-parent":
				parent["item"].(map[string]any)["summary"] = map[string]any{"text": "not-a-part-array"}
			case "malformed-item-status":
				events = append(events, summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 1, "safe "))
			}
			final := summaryItemForTest("response.output_item.done", 0, "a")
			if mode == "malformed-item-status" {
				final = summaryItemForTest("response.output_item.done", 0, "a", "safe ")
				final["item"].(map[string]any)["status"] = 7
			}
			events = append(events, final)
			frames, _ := parseSummaryForTest(t, summaryFinishForTest(events))
			bytes := 0
			for _, frame := range frames {
				bytes += len(frame.Text)
				if frame.Kind == "end" {
					t.Fatal("overflow or malformed summary was declared complete")
				}
			}
			if bytes > responsesSummaryByteLimitV1 || len(frames) > responsesSummaryPartLimitV1+6 {
				t.Fatal("private summary display exceeded its bounded stream budget")
			}
		})
	}
}

func TestResponsesSummaryDisplayObserverCannotFailModelResponse(t *testing.T) {
	body := summaryTranscriptForTest(t, summaryFinishForTest([]map[string]any{
		summaryItemForTest("response.output_item.added", 0, "a"),
		summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 1, "explicit summary"),
		summaryItemForTest("response.output_item.done", 0, "a", "explicit summary"),
	}))
	for _, panicObserver := range []bool{false, true} {
		chunks, _, err := ParseSSEWithCallback("responses", strings.NewReader(body), func(chunk Chunk) error {
			if chunk.ProcessFrame != nil {
				if panicObserver {
					panic("private observer unavailable")
				}
				return errors.New("private observer unavailable")
			}
			return nil
		})
		if err != nil || len(chunks) != 2 || chunks[0].Text != "summary-final-answer" || chunks[1].Kind != ChunkDone {
			t.Fatal("summary display observer changed the provider outcome")
		}
	}
}

func TestResponsesSummaryAndRawDisplayChooseRealEvidenceWithoutCoordinateCollision(t *testing.T) {
	for _, test := range []struct{ rawFirst, opaque bool }{{false, false}, {true, false}, {true, true}} {
		parent := summaryItemForTest("response.output_item.added", 0, "a")
		if test.opaque {
			parent["item"].(map[string]any)["encrypted_content"] = "OPAQUE_RAW_GUARD_CANARY"
		}
		raw := summaryEventForTest("response.reasoning_text.delta", map[string]any{"output_index": 0, "item_id": "a", "delta": "actual raw reasoning"})
		part := summaryPartForTest("response.reasoning_summary_part.added", 0, "a", 0, 1, "explicit summary")
		events := []map[string]any{parent}
		if test.rawFirst {
			events = append(events, raw, part)
		} else {
			events = append(events, part, raw)
		}
		events = append(events, summaryItemForTest("response.output_item.done", 0, "a", "explicit summary"),
			summaryItemForTest("response.output_item.added", 1, "b"),
			summaryPartForTest("response.reasoning_summary_part.added", 1, "b", 0, 2, "other summary"),
			summaryItemForTest("response.output_item.done", 1, "b", "other summary"),
		)
		frames, chunks := parseSummaryForTest(t, summaryFinishForTest(events))
		var rawKinds []string
		summaryBodies := map[uint64]string{}
		for _, frame := range frames {
			if frame.Source == "" {
				if frame.NativeIndex != 0 || frame.NativeID != "a" || frame.ReasoningForm != "" {
					t.Fatal("raw display lost its actual native identity")
				}
				rawKinds = append(rawKinds, frame.Kind)
			} else {
				if frame.Source != "provider_reasoning" || frame.ReasoningForm != "summary" {
					t.Fatal("summary lost explicit form")
				}
				summaryBodies[frame.HostSegmentOrdinal] += frame.Text
			}
		}
		if test.rawFirst && !test.opaque {
			if !reflect.DeepEqual(rawKinds, []string{"start", "delta", "end"}) || !reflect.DeepEqual(summaryBodies, map[uint64]string{1: "other summary"}) {
				t.Fatal("empty summary declaration swallowed actual raw display or shared its source")
			}
		} else if len(rawKinds) != 0 || !reflect.DeepEqual(summaryBodies, map[uint64]string{1: "explicit summary", 2: "other summary"}) {
			t.Fatal("one item emitted both display forms or two actual summary parts shared an ordinal")
		}
		retainedRaw := ""
		for _, chunk := range chunks {
			if chunk.Kind == ChunkReasoning {
				retainedRaw += chunk.Text
			}
		}
		if retainedRaw != "actual raw reasoning" {
			t.Fatal("display selection changed the real raw continuation")
		}
	}
}

func TestResponsesFirstTerminalSealsRawSummaryTextToolsAndUsage(t *testing.T) {
	for _, terminal := range []string{"completed", "done-marker", "incomplete", "incomplete-error"} {
		t.Run(terminal, func(t *testing.T) {
			prefix := []map[string]any{
				summaryEventForTest("response.output_item.added", map[string]any{"output_index": 0, "item": map[string]any{"id": "raw-before", "type": "reasoning"}}),
				summaryEventForTest("response.reasoning_text.delta", map[string]any{"output_index": 0, "item_id": "raw-before", "delta": "actual pre-terminal raw reasoning"}),
				summaryEventForTest("response.output_item.done", map[string]any{"output_index": 0, "item": map[string]any{"id": "raw-before", "type": "reasoning"}}),
				summaryItemForTest("response.output_item.added", 1, "summary-before"),
				summaryPartForTest("response.reasoning_summary_part.added", 1, "summary-before", 0, 1, "actual pre-terminal summary"),
				summaryItemForTest("response.output_item.done", 1, "summary-before", "actual pre-terminal summary"),
				summaryEventForTest("response.output_text.delta", map[string]any{"output_index": 2, "item_id": "answer-before", "delta": "verified pre-terminal answer"}),
				summaryEventForTest("response.output_item.added", map[string]any{"output_index": 3, "item": map[string]any{"id": "tool-before", "call_id": "tool-call-before", "type": "function_call", "name": "read", "arguments": `{"path":"synthetic.txt"}`}}),
			}
			body := summaryTranscriptForTest(t, prefix)
			if terminal == "done-marker" {
				body += "data: [DONE]\n\n"
			} else {
				response := map[string]any{"usage": map[string]any{"input_tokens": 7, "output_tokens": 9, "total_tokens": 16}}
				kind := "response.completed"
				if terminal == "incomplete" || terminal == "incomplete-error" {
					kind = "response.incomplete"
					reason := "max_output_tokens"
					if terminal == "incomplete-error" {
						reason = "unknown"
					}
					response["incomplete_details"] = map[string]any{"reason": reason}
				}
				body += summaryTranscriptForTest(t, []map[string]any{summaryEventForTest(kind, map[string]any{"response": response})})
			}
			late := summaryTranscriptForTest(t, []map[string]any{
				summaryEventForTest("response.output_item.added", map[string]any{"output_index": 20, "item": map[string]any{"id": "late-raw", "type": "reasoning"}}),
				summaryEventForTest("response.reasoning_text.delta", map[string]any{"output_index": 20, "item_id": "late-raw", "delta": "LATE_RAW_CANARY"}),
				summaryEventForTest("response.output_item.done", map[string]any{"output_index": 20, "item": map[string]any{"id": "late-raw", "type": "reasoning"}}),
				summaryItemForTest("response.output_item.added", 21, "late-summary"),
				summaryPartForTest("response.reasoning_summary_part.added", 21, "late-summary", 0, 2, "LATE_SUMMARY_CANARY"),
				summaryItemForTest("response.output_item.done", 21, "late-summary", "LATE_SUMMARY_CANARY"),
				summaryEventForTest("response.output_item.added", map[string]any{"output_index": 22, "item": map[string]any{"id": "late-tool", "call_id": "late-tool-call", "type": "function_call", "name": "read", "arguments": `{"path":"late.txt"}`}}),
				summaryEventForTest("response.output_item.done", map[string]any{"output_index": 22, "item": map[string]any{"id": "late-tool", "call_id": "late-tool-call", "type": "function_call", "name": "read", "arguments": `{"path":"late.txt"}`}}),
				summaryEventForTest("response.output_text.delta", map[string]any{"output_index": 23, "item_id": "late-answer", "delta": "LATE_TEXT_CANARY"}),
				summaryEventForTest("response.completed", map[string]any{"response": map[string]any{"usage": map[string]any{"input_tokens": 100, "output_tokens": 200, "total_tokens": 300}}}),
			}) + "data: {malformed-after-terminal\n\ndata: [DONE]\n\n"
			parse := func(payload string) ([]Chunk, Usage, []domainmodel.ProcessFrameV1, []PayloadObservation, error) {
				var frames []domainmodel.ProcessFrameV1
				var observations []PayloadObservation
				chunks, usage, err := ParseSSEWithCallbackAndPayloadObservation("responses", strings.NewReader(payload), func(chunk Chunk) error {
					if chunk.ProcessFrame != nil {
						frames = append(frames, *chunk.ProcessFrame)
					}
					return nil
				}, func(observation PayloadObservation) { observations = append(observations, observation) })
				return chunks, usage, frames, observations, err
			}
			wantChunks, wantUsage, wantFrames, wantObservations, wantErr := parse(body)
			gotChunks, gotUsage, gotFrames, gotObservations, gotErr := parse(body + late)
			if !reflect.DeepEqual(gotChunks, wantChunks) || !reflect.DeepEqual(gotUsage, wantUsage) || !reflect.DeepEqual(gotFrames, wantFrames) || !reflect.DeepEqual(gotObservations, wantObservations) || (gotErr == nil) != (wantErr == nil) || gotErr != nil && gotErr.Error() != wantErr.Error() {
				t.Fatal("late provider data changed the first terminal's actual result, private frames or usage")
			}
			public, raw := "", ""
			tools, dones := 0, 0
			for _, chunk := range gotChunks {
				if chunk.Kind == ChunkText {
					public += chunk.Text
				}
				if chunk.Kind == ChunkReasoning {
					raw += chunk.Text
				}
				if chunk.Kind == ChunkToolCall {
					tools++
				}
				if chunk.Kind == ChunkDone {
					dones++
				}
			}
			if public != "verified pre-terminal answer" || raw != "actual pre-terminal raw reasoning" || len(gotFrames) != 6 {
				t.Fatal("terminal fence lost legal pre-terminal text or process sources")
			}
			if terminal == "completed" || terminal == "done-marker" {
				if gotErr != nil || tools != 1 || dones != 1 {
					t.Fatal("legal terminal tool flush or completion changed")
				}
			} else if tools != 0 || terminal == "incomplete" && (gotErr != nil || dones != 1 || gotUsage.FinishReason != "length") || terminal == "incomplete-error" && (gotErr == nil || dones != 0) {
				t.Fatal("incomplete stream gained an executable tool or lost its actual error/cap")
			}
			if terminal != "done-marker" && (gotUsage.PromptTokens != 7 || gotUsage.CompletionTokens != 9 || gotUsage.TotalTokens != 16) {
				t.Fatal("terminal usage was lost or overwritten")
			}
		})
	}
}

type responsesTerminalReadFenceForTest struct {
	first         *strings.Reader
	lateRead      bool
	sameReadError error
}

func (r *responsesTerminalReadFenceForTest) Read(p []byte) (int, error) {
	if r.first.Len() != 0 {
		n, err := r.first.Read(p)
		if r.first.Len() == 0 && r.sameReadError != nil {
			return n, r.sameReadError
		}
		return n, err
	}
	r.lateRead = true
	return 0, errors.New("late provider read must not run")
}

func TestResponsesTerminalStopsReadingBeforeLateTransportFailure(t *testing.T) {
	for _, terminal := range []string{
		"data: [DONE]\n\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":7,\"output_tokens\":9,\"total_tokens\":16}}}\n\n",
		"data: {\"type\":\"response.incomplete\",\"response\":{\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n",
	} {
		for _, sameReadError := range []bool{false, true} {
			reader := &responsesTerminalReadFenceForTest{first: strings.NewReader(terminal)}
			if sameReadError {
				reader.sameReadError = errors.New("read error accompanying complete terminal bytes")
			}
			chunks, usage, err := ParseSSE("responses", reader)
			if err != nil || reader.lateRead || !hasChunkKind(chunks, ChunkDone) {
				t.Fatal("completed stream read late transport bytes or lost terminal completion")
			}
			if strings.Contains(terminal, "input_tokens") && (usage.PromptTokens != 7 || usage.CompletionTokens != 9 || usage.TotalTokens != 16) {
				t.Fatal("same-read error discarded actual terminal usage")
			}
		}
	}
	// EOF before an actual terminal keeps its existing protocol failure.
	_, _, err := ParseSSE("responses", strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"unfinished\"}\n\n"))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("terminal fence concealed a genuinely unfinished stream")
	}
	readFailure := errors.New("actual pre-terminal read failure")
	reader := &responsesTerminalReadFenceForTest{first: strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"unfinished\"}\n\n"), sameReadError: readFailure}
	_, _, err = ParseSSE("responses", reader)
	if !errors.Is(err, readFailure) {
		t.Fatal("terminal fence concealed an actual pre-terminal transport failure")
	}
}
