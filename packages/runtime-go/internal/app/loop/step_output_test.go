package loop

import (
	"errors"
	"reflect"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

type assistantDeltaRecorderStub struct {
	events []assistantDeltaCall
	err    error
}

type assistantDeltaCall struct {
	kind     string
	itemKind string
	text     string
}

func (stub *assistantDeltaRecorderStub) AssistantTextDelta(_, _ string, text string) error {
	stub.events = append(stub.events, assistantDeltaCall{kind: "assistant_text_delta", itemKind: "assistant_text", text: text})
	return stub.err
}

func TestCollectStepOutputRecordsNonStreamedDeltasAndToolCalls(t *testing.T) {
	recorder := &assistantDeltaRecorderStub{}
	output, err := CollectStepOutput(StepOutputInput{
		ThreadID: "thr_1",
		TurnID:   "turn_1",
		Events:   recorder,
		Stream: ProviderStreamOutput{
			Result: domainmodel.Result{Chunks: []domainmodel.Chunk{
				{Kind: domainmodel.ChunkReasoning, Text: "think", Signature: "sig-1"},
				{Kind: domainmodel.ChunkText, Text: "hello"},
				{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "call_1", Name: "read"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("collect step output: %v", err)
	}
	if output.Text != "hello" || output.AssistantTextDelta != "hello" || output.Reasoning != "think" || output.ReasoningSignature != "sig-1" {
		t.Fatalf("unexpected output: %#v", output)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0].Name != "read" {
		t.Fatalf("tool calls mismatch: %#v", output.ToolCalls)
	}
	expected := []assistantDeltaCall{
		{kind: "assistant_text_delta", itemKind: "assistant_text", text: "hello"},
	}
	if !reflect.DeepEqual(recorder.events, expected) {
		t.Fatalf("delta events mismatch: %#v", recorder.events)
	}
}

func TestCollectStepOutputDoesNotReplayStreamedDeltas(t *testing.T) {
	recorder := &assistantDeltaRecorderStub{}
	output, err := CollectStepOutput(StepOutputInput{
		ThreadID: "thr_1",
		TurnID:   "turn_1",
		Events:   recorder,
		Stream: ProviderStreamOutput{
			Text:           "streamed",
			Reasoning:      "thinking",
			StreamedDeltas: true,
			Result: domainmodel.Result{Chunks: []domainmodel.Chunk{
				{Kind: domainmodel.ChunkReasoning, Text: "ignored"},
				{Kind: domainmodel.ChunkText, Text: "ignored"},
				{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "call_1", Name: "grep"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("collect streamed output: %v", err)
	}
	if output.Text != "streamed" || output.AssistantTextDelta != "" || output.Reasoning != "thinking" {
		t.Fatalf("streamed output mismatch: %#v", output)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0].Name != "grep" {
		t.Fatalf("tool calls mismatch: %#v", output.ToolCalls)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("streamed chunks should not be re-recorded: %#v", recorder.events)
	}
}

func TestCollectStepOutputSuppressesNonStreamedHighRiskDraftEvents(t *testing.T) {
	recorder := &assistantDeltaRecorderStub{}
	output, err := CollectStepOutput(StepOutputInput{
		ThreadID: "thr_1", TurnID: "turn_1", Events: recorder, SuppressAssistantTextEvents: true,
		Stream: ProviderStreamOutput{Result: domainmodel.Result{Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "fabricated case fact"}}}},
	})
	if err != nil || output.Text != "fabricated case fact" || len(recorder.events) != 0 {
		t.Fatalf("high-risk non-streamed draft leaked: output=%#v events=%#v err=%v", output, recorder.events, err)
	}
}

func TestCollectStepOutputDropsNonStreamedThinkMarkup(t *testing.T) {
	recorder := &assistantDeltaRecorderStub{}
	output, err := CollectStepOutput(StepOutputInput{
		ThreadID: "thr_1",
		TurnID:   "turn_1",
		Events:   recorder,
		Stream: ProviderStreamOutput{Result: domainmodel.Result{Chunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "public<th"},
			{Kind: domainmodel.ChunkText, Text: "ink>PRIVATE_REASONING_SENTINEL</think>answer"},
		}}},
	})
	if err != nil {
		t.Fatalf("collect step output: %v", err)
	}
	if output.Text != "publicanswer" || len(recorder.events) != 1 || recorder.events[0].text != "publicanswer" {
		t.Fatalf("non-streamed private think markup leaked: output=%#v events=%#v", output, recorder.events)
	}
}

func TestCollectStepOutputRejectsIncompleteReasoningMarkupBeforeEvent(t *testing.T) {
	recorder := &assistantDeltaRecorderStub{}
	output, err := CollectStepOutput(StepOutputInput{
		ThreadID: "thr_1",
		TurnID:   "turn_1",
		Events:   recorder,
		Stream: ProviderStreamOutput{Result: domainmodel.Result{Chunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "public"},
			{Kind: domainmodel.ChunkText, Text: "<think>PRIVATE_REASONING_SENTINEL"},
		}}},
	})
	var markupErr domainreasoningmarkup.ProtocolError
	if !errors.As(err, &markupErr) || !errors.Is(err, domainreasoningmarkup.ErrIncomplete) ||
		!reflect.DeepEqual(output, StepOutput{}) || len(recorder.events) != 0 {
		t.Fatalf("incomplete reasoning markup crossed event boundary: output=%#v events=%#v err=%v", output, recorder.events, err)
	}
}

func TestCollectStepOutputRejectsProviderNativeReasoningBeforeEvent(t *testing.T) {
	recorder := &assistantDeltaRecorderStub{}
	output, err := CollectStepOutput(StepOutputInput{
		ThreadID: "thr_1",
		TurnID:   "turn_1",
		Events:   recorder,
		Stream: ProviderStreamOutput{Result: domainmodel.Result{Chunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: `{"kind":"thinking_delta",`},
			{Kind: domainmodel.ChunkText, Text: `"text":"PRIVATE_NATIVE_REASONING"}`},
		}}},
	})
	var protocolErr domainreasoningmarkup.ProtocolError
	if !errors.As(err, &protocolErr) || !errors.Is(err, domainreasoningmarkup.ErrPrivateContent) ||
		!reflect.DeepEqual(output, StepOutput{}) || len(recorder.events) != 0 {
		t.Fatalf("provider-native reasoning crossed event admission: output=%#v events=%#v err=%v", output, recorder.events, err)
	}
}

func TestCollectStepOutputRejectsPrefilledStreamTextBypass(t *testing.T) {
	t.Parallel()
	output, err := CollectStepOutput(StepOutputInput{
		Stream: ProviderStreamOutput{StreamedDeltas: true, Text: "public</thi"},
	})
	var protocolErr domainreasoningmarkup.ProtocolError
	if !errors.As(err, &protocolErr) || !errors.Is(err, domainreasoningmarkup.ErrIncomplete) ||
		!reflect.DeepEqual(output, StepOutput{}) {
		t.Fatalf("prefilled stream text bypassed canonical decoder: output=%#v err=%v", output, err)
	}
}
