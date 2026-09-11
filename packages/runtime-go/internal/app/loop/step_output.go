package loop

import (
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

type AssistantDeltaRecorder interface {
	AssistantTextDelta(threadID, turnID, text string) error
}

type StepOutputInput struct {
	ThreadID                    string
	TurnID                      string
	Stream                      ProviderStreamOutput
	Events                      AssistantDeltaRecorder
	SuppressAssistantTextEvents bool
}

type StepOutput struct {
	Text               string
	Reasoning          string
	ReasoningSignature string
	ToolCalls          []domainmodel.ToolCall
	AssistantTextDelta string
}

func CollectStepOutput(input StepOutputInput) (StepOutput, error) {
	streamText, err := domainevent.FilterPublicText(input.Stream.Text)
	if err != nil {
		return StepOutput{}, domainreasoningmarkup.NewProtocolError(err)
	}
	output := StepOutput{
		Text:               streamText,
		Reasoning:          input.Stream.Reasoning,
		ReasoningSignature: input.Stream.ReasoningSignature,
	}
	publicTextDecoder := domainreasoningmarkup.NewDecoder()
	recordPublicText := func(text string) error {
		if text == "" {
			return nil
		}
		output.Text += text
		output.AssistantTextDelta += text
		if input.Events != nil && !input.SuppressAssistantTextEvents {
			return input.Events.AssistantTextDelta(input.ThreadID, input.TurnID, text)
		}
		return nil
	}
	for _, chunk := range input.Stream.Result.Chunks {
		switch chunk.Kind {
		case domainmodel.ChunkReasoning:
			if chunk.Signature != "" && output.ReasoningSignature == "" {
				output.ReasoningSignature = chunk.Signature
			}
			if !input.Stream.StreamedDeltas && chunk.Text != "" {
				output.Reasoning += chunk.Text
			}
		case domainmodel.ChunkText:
			if !input.Stream.StreamedDeltas && chunk.Text != "" {
				if err := publicTextDecoder.Push(chunk.Text); err != nil {
					return StepOutput{}, domainreasoningmarkup.NewProtocolError(err)
				}
			}
		case domainmodel.ChunkToolCall:
			output.ToolCalls = append(output.ToolCalls, chunk.ToolCall)
		}
	}
	if !input.Stream.StreamedDeltas {
		outcome, err := publicTextDecoder.Finish()
		if err != nil {
			return StepOutput{}, domainreasoningmarkup.NewProtocolError(err)
		}
		if err := recordPublicText(outcome.PublicText); err != nil {
			return StepOutput{}, err
		}
	}
	return output, nil
}
