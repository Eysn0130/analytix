package stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	providerusage "analytix.local/runtime-go/internal/adapters/outbound/provider/usage"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
)

type Chunk = domainmodel.Chunk
type Usage = domainmodel.Usage
type ToolCall = domainmodel.ToolCall
type ChunkKind = domainmodel.ChunkKind

type PayloadObservation = domainmodel.ProviderOutputObservationV1

const (
	PayloadObservationControl          = domainmodel.ProviderOutputObservationControlV1
	PayloadObservationPrivateReasoning = domainmodel.ProviderOutputObservationPrivateReasoningV1
	PayloadObservationPublicText       = domainmodel.ProviderOutputObservationPublicTextV1
	PayloadObservationToolStarted      = domainmodel.ProviderOutputObservationToolStartedV1
)

const (
	ChunkReasoning     = domainmodel.ChunkReasoning
	ChunkText          = domainmodel.ChunkText
	ChunkToolCallStart = domainmodel.ChunkToolCallStart
	ChunkToolCall      = domainmodel.ChunkToolCall
	ChunkUsage         = domainmodel.ChunkUsage
	ChunkDone          = domainmodel.ChunkDone
)

func ParseSSE(endpointFormat string, body io.Reader) ([]Chunk, Usage, error) {
	return ParseSSEWithCallback(endpointFormat, body, nil)
}

func ParseSSEWithCallback(endpointFormat string, body io.Reader, onChunk func(Chunk) error) ([]Chunk, Usage, error) {
	return ParseSSEWithCallbackAndPayloadObservation(endpointFormat, body, onChunk, nil)
}

// ParseSSEWithCallbackAndPayloadObservation reports a typed, byte-free
// observation for each non-empty provider payload. This is deliberately
// separate from onChunk: the transactional reasoning-markup decoder may need
// to withhold text until a complete stream, while transport retry authority
// must still distinguish private reasoning from public text and tool state.
func ParseSSEWithCallbackAndPayloadObservation(endpointFormat string, body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation)) ([]Chunk, Usage, error) {
	if endpointFormat == "messages" {
		return parseAnthropicProviderSSEWithCallback(body, onChunk, onPayload)
	}
	if endpointFormat == "responses" {
		return parseResponsesProviderSSEWithCallback(body, onChunk, onPayload)
	}
	chunks := []Chunk{}
	usage := Usage{}
	usagePending := false
	toolCalls := newToolCallAccumulator()
	completed := false
	terminalFinishReason := ""
	thinkingTags := ThinkSplitter{}
	completeAtBoundary := func(rawSSEChunkAt time.Time, parsedAt time.Time) error {
		usage.FinishReason = domainmodel.NormalizeFinishReason(terminalFinishReason)
		if strings.TrimSpace(terminalFinishReason) == "" {
			return domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)
		}
		if err := openAIChatTerminalFinishReasonError(terminalFinishReason); err != nil {
			return err
		}
		if err := openAIChatTerminalToolCallError(terminalFinishReason, toolCalls.hasPending()); err != nil {
			return err
		}
		terminalToolChunks := []Chunk{}
		if domainmodel.NormalizeFinishReason(terminalFinishReason) == "tool_calls" {
			flushed, err := toolCalls.flush()
			if err != nil {
				return err
			}
			terminalToolChunks = flushed
		}
		publicChunks, err := thinkingTags.flushChunks()
		if err != nil {
			return err
		}
		if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(publicChunks, rawSSEChunkAt, parsedAt), onChunk); err != nil {
			return err
		}
		if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(terminalToolChunks, rawSSEChunkAt, parsedAt), onChunk); err != nil {
			return err
		}
		if usagePending {
			if err := appendProviderChunk(&chunks, annotateProviderChunkTime(Chunk{Kind: ChunkUsage, Usage: usage}, rawSSEChunkAt, parsedAt), onChunk); err != nil {
				return err
			}
			usagePending = false
		}
		if err := appendProviderChunk(&chunks, annotateProviderChunkTime(Chunk{Kind: ChunkDone}, rawSSEChunkAt, parsedAt), onChunk); err != nil {
			return err
		}
		completed = true
		return nil
	}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		rawSSEChunkAt := time.Now()
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			parsedAt := time.Now()
			if err := completeAtBoundary(rawSSEChunkAt, parsedAt); err != nil {
				return chunks, usage, err
			}
			break
		}
		observePayload(onPayload, PayloadObservationControl)
		itemChunks, itemUsage, ok, finishReason, observation, err := parseOpenAIChatSSEPayloadAtObserved(data, toolCalls, &thinkingTags, rawSSEChunkAt)
		parsedAt := time.Now()
		observePayload(onPayload, observation)
		if err != nil {
			return chunks, usage, err
		}
		if terminalFinishReason != "" && (finishReason != "" || observation != PayloadObservationControl) {
			return chunks, usage, domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
		}
		if finishReason != "" {
			terminalFinishReason = finishReason
		}
		if ok {
			itemUsage.FinishReason = domainmodel.NormalizeFinishReason(terminalFinishReason)
			usage = itemUsage
			usagePending = true
		}
		providerChunks := make([]Chunk, 0, len(itemChunks))
		for _, chunk := range itemChunks {
			if chunk.Kind != ChunkUsage {
				providerChunks = append(providerChunks, chunk)
			}
		}
		if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(providerChunks, rawSSEChunkAt, parsedAt), onChunk); err != nil {
			return nil, Usage{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		return chunks, usage, err
	}
	if !completed {
		now := time.Now()
		if err := completeAtBoundary(now, now); err != nil {
			return chunks, usage, err
		}
	}
	usage.FinishReason = domainmodel.NormalizeFinishReason(terminalFinishReason)
	return chunks, usage, nil
}

func openAIChatTerminalFinishReasonError(value string) error {
	switch domainmodel.NormalizeFinishReason(value) {
	case "length":
		return domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)
	case "content_filter":
		return domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
	case "unknown":
		if strings.TrimSpace(value) != "" {
			return domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
		}
	default:
	}
	return nil
}

func openAIChatTerminalToolCallError(finishReason string, hasPendingToolCalls bool) error {
	switch domainmodel.NormalizeFinishReason(finishReason) {
	case "tool_calls":
		if !hasPendingToolCalls {
			return domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
		}
	case "stop":
		if hasPendingToolCalls {
			return domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)
		}
	case "unknown":
		if strings.TrimSpace(finishReason) == "" && hasPendingToolCalls {
			return domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)
		}
	}
	return nil
}

type ThinkSplitter struct {
	decoder            *domainreasoningmarkup.Decoder
	parseErr           error
	firstRawSSEChunkAt time.Time
	finished           bool
}

func (t *ThinkSplitter) push(value string, rawSSEChunkAt time.Time) error {
	if t.finished {
		return domainreasoningmarkup.NewProtocolError(domainreasoningmarkup.ErrLifecycle)
	}
	if t.parseErr != nil {
		return domainreasoningmarkup.NewProtocolError(t.parseErr)
	}
	if t.decoder == nil {
		t.decoder = domainreasoningmarkup.NewDecoder()
	}
	if t.firstRawSSEChunkAt.IsZero() && !rawSSEChunkAt.IsZero() {
		t.firstRawSSEChunkAt = rawSSEChunkAt
	}
	if err := t.decoder.Push(value); err != nil {
		t.parseErr = err
		return domainreasoningmarkup.NewProtocolError(err)
	}
	return nil
}

func (t *ThinkSplitter) flushChunks() ([]Chunk, error) {
	if t.finished {
		return nil, nil
	}
	t.finished = true
	if t.parseErr != nil {
		return nil, domainreasoningmarkup.NewProtocolError(t.parseErr)
	}
	if t.decoder == nil {
		t.decoder = domainreasoningmarkup.NewDecoder()
	}
	outcome, err := t.decoder.Finish()
	if err != nil {
		return nil, domainreasoningmarkup.NewProtocolError(err)
	}
	if outcome.PublicText == "" {
		return nil, nil
	}
	return []Chunk{{
		Kind:  ChunkText,
		Text:  outcome.PublicText,
		Trace: domainmodel.ChunkTrace{ProviderRawSSEChunkAt: t.firstRawSSEChunkAt},
	}}, nil
}

func appendProviderChunk(chunks *[]Chunk, chunk Chunk, onChunk func(Chunk) error) error {
	*chunks = append(*chunks, chunk)
	if onChunk != nil {
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return nil
}

func annotateProviderChunkTimes(chunks []Chunk, rawSSEChunkAt time.Time, parsedAt time.Time) []Chunk {
	for index := range chunks {
		chunks[index] = annotateProviderChunkTime(chunks[index], rawSSEChunkAt, parsedAt)
	}
	return chunks
}

func annotateProviderChunkTime(chunk Chunk, rawSSEChunkAt time.Time, parsedAt time.Time) Chunk {
	if chunk.Trace.ProviderRawSSEChunkAt.IsZero() {
		chunk.Trace.ProviderRawSSEChunkAt = rawSSEChunkAt
	}
	if chunk.Trace.ProviderChunkParsedAt.IsZero() {
		chunk.Trace.ProviderChunkParsedAt = parsedAt
	}
	return chunk
}

func appendProviderChunks(chunks *[]Chunk, additions []Chunk, onChunk func(Chunk) error) error {
	for _, chunk := range additions {
		if err := appendProviderChunk(chunks, chunk, onChunk); err != nil {
			return err
		}
	}
	return nil
}

func observePayload(onPayload func(PayloadObservation), observation PayloadObservation) {
	if onPayload != nil {
		onPayload(observation)
	}
}

func observeProviderChunks(chunks []Chunk) PayloadObservation {
	observation := PayloadObservationControl
	for _, chunk := range chunks {
		switch chunk.Kind {
		case ChunkReasoning:
			if chunk.Text != "" || chunk.Signature != "" {
				observation = observation.MergeV1(PayloadObservationPrivateReasoning)
			}
		case ChunkText:
			if chunk.Text != "" {
				observation = observation.MergeV1(PayloadObservationPublicText)
			}
		case ChunkToolCallStart, ChunkToolCall:
			observation = observation.MergeV1(PayloadObservationToolStarted)
		}
	}
	return observation
}

func observeAnthropicPayload(payloadType string, deltaType string, deltaValue string, contentBlockType string, contentBlockValue string) PayloadObservation {
	observation := PayloadObservationControl
	if payloadType == "content_block_start" {
		switch contentBlockType {
		case "tool_use":
			observation = observation.MergeV1(PayloadObservationToolStarted)
		case "text":
			if contentBlockValue != "" {
				observation = observation.MergeV1(PayloadObservationPublicText)
			}
		case "thinking":
			if contentBlockValue != "" {
				observation = observation.MergeV1(PayloadObservationPrivateReasoning)
			}
		}
	}
	if payloadType == "content_block_delta" {
		switch deltaType {
		case "text_delta":
			if deltaValue != "" {
				observation = observation.MergeV1(PayloadObservationPublicText)
			}
		case "thinking_delta", "signature_delta":
			if deltaValue != "" {
				observation = observation.MergeV1(PayloadObservationPrivateReasoning)
			}
		case "input_json_delta":
			observation = observation.MergeV1(PayloadObservationToolStarted)
		}
	}
	return observation
}

type ToolCallAccumulator struct {
	order []int
	byKey map[int]*partialToolCall
}

type partialToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
	Started   bool
}

func newToolCallAccumulator() *ToolCallAccumulator {
	return &ToolCallAccumulator{byKey: map[int]*partialToolCall{}}
}

func (a *ToolCallAccumulator) add(index int, id string, name string, arguments string) []Chunk {
	if index < 0 {
		index = len(a.order)
	}
	item := a.byKey[index]
	if item == nil {
		item = &partialToolCall{}
		a.byKey[index] = item
		a.order = append(a.order, index)
	}
	if strings.TrimSpace(id) != "" && !item.Started {
		item.ID = strings.TrimSpace(id)
	}
	if strings.TrimSpace(name) != "" {
		item.Name = strings.TrimSpace(name)
	}
	if arguments != "" {
		item.Arguments.WriteString(arguments)
	}
	if item.Started || strings.TrimSpace(item.Name) == "" {
		return nil
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = fmt.Sprintf("call_%d", index)
	}
	item.Started = true
	return []Chunk{{
		Kind: ChunkToolCallStart,
		ToolCall: ToolCall{
			ID:   item.ID,
			Name: item.Name,
		},
	}}
}

func (a *ToolCallAccumulator) flush() ([]Chunk, error) {
	if len(a.order) == 0 {
		return nil, nil
	}
	chunks := []Chunk{}
	for _, index := range a.order {
		item := a.byKey[index]
		if item == nil {
			continue
		}
		id := item.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", index)
		}
		name := item.Name
		if name == "" {
			name = "unknown_tool"
		}
		args := strings.TrimSpace(item.Arguments.String())
		if args == "" {
			args = "{}"
		}
		var arguments map[string]json.RawMessage
		if !json.Valid([]byte(args)) || json.Unmarshal([]byte(args), &arguments) != nil || arguments == nil {
			return nil, domainfailure.NewError(domainfailure.CodeProviderToolArgumentsInvalid, nil)
		}
		chunks = append(chunks, Chunk{Kind: ChunkToolCall, ToolCall: ToolCall{
			ID:        id,
			Name:      name,
			Arguments: json.RawMessage(args),
		}})
	}
	a.order = nil
	a.byKey = map[int]*partialToolCall{}
	return chunks, nil
}

func (a *ToolCallAccumulator) hasPending() bool {
	return len(a.order) > 0
}

func parseResponsesProviderSSE(body io.Reader) ([]Chunk, Usage, error) {
	return parseResponsesProviderSSEWithCallback(body, nil, nil)
}

func parseResponsesProviderSSEWithCallback(body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation)) ([]Chunk, Usage, error) {
	chunks := []Chunk{}
	usage := Usage{}
	toolCalls := newToolCallAccumulator()
	itemIndexes := map[string]int{}
	completed := false
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			if completed {
				continue
			}
			flushed, err := toolCalls.flush()
			if err != nil {
				return chunks, usage, err
			}
			if err := appendProviderChunks(&chunks, flushed, onChunk); err != nil {
				return nil, Usage{}, err
			}
			if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkDone}, onChunk); err != nil {
				return nil, Usage{}, err
			}
			completed = true
			continue
		}
		observePayload(onPayload, PayloadObservationControl)
		itemChunks, itemUsage, ok, observation, err := parseResponsesSSEPayload(data, toolCalls, itemIndexes)
		observePayload(onPayload, observation)
		if err != nil {
			return nil, Usage{}, err
		}
		if err := appendProviderChunks(&chunks, itemChunks, onChunk); err != nil {
			return nil, Usage{}, err
		}
		if hasChunkKind(itemChunks, ChunkDone) {
			completed = true
		}
		if ok {
			usage = itemUsage
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, Usage{}, err
	}
	if !completed {
		if toolCalls.hasPending() {
			return nil, Usage{}, fmt.Errorf("provider responses stream ended before completing tool calls: %w", io.ErrUnexpectedEOF)
		}
		return nil, Usage{}, fmt.Errorf("provider responses stream ended before completion: %w", io.ErrUnexpectedEOF)
	}
	return chunks, usage, nil
}

func parseResponsesSSEPayload(data string, accumulator *ToolCallAccumulator, itemIndexes map[string]int) ([]Chunk, Usage, bool, PayloadObservation, error) {
	var payload struct {
		Type        string `json:"type"`
		Delta       string `json:"delta"`
		ItemID      string `json:"item_id"`
		OutputIndex int    `json:"output_index"`
		Item        *struct {
			ID        string `json:"id"`
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"item"`
		Response *struct {
			Usage *struct {
				InputTokens        int `json:"input_tokens"`
				OutputTokens       int `json:"output_tokens"`
				TotalTokens        int `json:"total_tokens"`
				InputTokensDetails *struct {
					CachedTokens *int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
				OutputTokensDetails *struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, Usage{}, false, PayloadObservationControl, err
	}
	switch payload.Type {
	case "response.output_text.delta":
		return []Chunk{{Kind: ChunkText, Text: payload.Delta}}, Usage{}, false, PayloadObservationPublicText, nil
	case "response.reasoning_text.delta":
		return []Chunk{{Kind: ChunkReasoning, Text: payload.Delta}}, Usage{}, false, PayloadObservationPrivateReasoning, nil
	case "response.output_item.added":
		if payload.Item != nil && payload.Item.Type == "function_call" {
			index := payload.OutputIndex
			if payload.Item.ID != "" {
				itemIndexes[payload.Item.ID] = index
			}
			return accumulator.add(index, firstNonEmpty(payload.Item.CallID, payload.Item.ID), payload.Item.Name, payload.Item.Arguments), Usage{}, false, PayloadObservationToolStarted, nil
		}
		return []Chunk{}, Usage{}, false, PayloadObservationControl, nil
	case "response.function_call_arguments.delta":
		index := payload.OutputIndex
		if payload.ItemID != "" {
			if existing, ok := itemIndexes[payload.ItemID]; ok {
				index = existing
			} else {
				itemIndexes[payload.ItemID] = index
			}
		}
		return accumulator.add(index, payload.ItemID, "", payload.Delta), Usage{}, false, PayloadObservationToolStarted, nil
	case "response.output_item.done":
		if payload.Item != nil && payload.Item.Type == "function_call" {
			index := payload.OutputIndex
			if payload.Item.ID != "" {
				itemIndexes[payload.Item.ID] = index
			}
			started := accumulator.add(index, firstNonEmpty(payload.Item.CallID, payload.Item.ID), payload.Item.Name, payload.Item.Arguments)
			flushed, err := accumulator.flush()
			return append(started, flushed...), Usage{}, false, PayloadObservationToolStarted, err
		}
		return []Chunk{}, Usage{}, false, PayloadObservationControl, nil
	case "response.completed":
		flushed, err := accumulator.flush()
		if err != nil {
			return nil, Usage{}, false, PayloadObservationControl, err
		}
		if payload.Response != nil && payload.Response.Usage != nil {
			u := payload.Response.Usage
			hit := 0
			hasCacheTelemetry := false
			if u.InputTokensDetails != nil && u.InputTokensDetails.CachedTokens != nil {
				hit = *u.InputTokensDetails.CachedTokens
				hasCacheTelemetry = true
			}
			miss := 0
			if hasCacheTelemetry && u.InputTokens > hit {
				miss = u.InputTokens - hit
			}
			reasoning := 0
			if u.OutputTokensDetails != nil {
				reasoning = u.OutputTokensDetails.ReasoningTokens
			}
			usage := providerUsage(u.InputTokens, u.OutputTokens, u.TotalTokens, reasoning, hit, miss, hasCacheTelemetry)
			return append(flushed, Chunk{Kind: ChunkUsage, Usage: usage}, Chunk{Kind: ChunkDone}), usage, true, observeProviderChunks(flushed), nil
		}
		return append(flushed, Chunk{Kind: ChunkDone}), Usage{}, false, observeProviderChunks(flushed), nil
	default:
		itemChunks, itemUsage, ok, err := parseOpenAISSEPayload(data)
		if err != nil {
			return nil, Usage{}, false, PayloadObservationControl, err
		}
		if err == nil && len(itemChunks) > 0 {
			return itemChunks, itemUsage, ok, observeProviderChunks(itemChunks), nil
		}
		return []Chunk{}, Usage{}, false, PayloadObservationControl, nil
	}
}

func parseAnthropicProviderSSE(body io.Reader) ([]Chunk, Usage, error) {
	return parseAnthropicProviderSSEWithCallback(body, nil, nil)
}

func parseAnthropicProviderSSEWithCallback(body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation)) ([]Chunk, Usage, error) {
	chunks := []Chunk{}
	usage := Usage{}
	inTok, outTok, cacheCreate, cacheRead := 0, 0, 0, 0
	hasCacheTelemetry := false
	toolCalls := newToolCallAccumulator()
	completed := false
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		observePayload(onPayload, PayloadObservationControl)
		var payload struct {
			Type    string `json:"type"`
			Message *struct {
				Usage *struct {
					InputTokens              int  `json:"input_tokens"`
					CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
			Usage *struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Index        int `json:"index"`
			ContentBlock *struct {
				Type     string          `json:"type"`
				ID       string          `json:"id"`
				Name     string          `json:"name"`
				Input    json.RawMessage `json:"input"`
				Text     string          `json:"text"`
				Thinking string          `json:"thinking"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return nil, Usage{}, err
		}
		deltaType := ""
		deltaValue := ""
		if payload.Delta != nil {
			deltaType = payload.Delta.Type
			deltaValue = firstNonEmpty(payload.Delta.Text, payload.Delta.Thinking, payload.Delta.Signature, payload.Delta.PartialJSON)
		}
		contentBlockType := ""
		contentBlockValue := ""
		if payload.ContentBlock != nil {
			contentBlockType = payload.ContentBlock.Type
			contentBlockValue = firstNonEmpty(payload.ContentBlock.Text, payload.ContentBlock.Thinking)
		}
		observePayload(onPayload, observeAnthropicPayload(payload.Type, deltaType, deltaValue, contentBlockType, contentBlockValue))
		switch payload.Type {
		case "message_start":
			if payload.Message != nil && payload.Message.Usage != nil {
				inTok = payload.Message.Usage.InputTokens
				if payload.Message.Usage.CacheCreationInputTokens != nil {
					cacheCreate = *payload.Message.Usage.CacheCreationInputTokens
					hasCacheTelemetry = true
				}
				if payload.Message.Usage.CacheReadInputTokens != nil {
					cacheRead = *payload.Message.Usage.CacheReadInputTokens
					hasCacheTelemetry = true
				}
			}
		case "content_block_start":
			if payload.ContentBlock != nil && payload.ContentBlock.Type == "tool_use" {
				args := ""
				if len(payload.ContentBlock.Input) > 0 {
					rawInput := strings.TrimSpace(string(payload.ContentBlock.Input))
					if rawInput != "" && rawInput != "{}" && rawInput != "null" {
						args = rawInput
					}
				}
				if err := appendProviderChunks(&chunks, toolCalls.add(payload.Index, payload.ContentBlock.ID, payload.ContentBlock.Name, args), onChunk); err != nil {
					return nil, Usage{}, err
				}
			}
		case "content_block_delta":
			if payload.Delta != nil {
				switch payload.Delta.Type {
				case "text_delta":
					if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkText, Text: payload.Delta.Text}, onChunk); err != nil {
						return nil, Usage{}, err
					}
				case "thinking_delta":
					if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkReasoning, Text: payload.Delta.Thinking}, onChunk); err != nil {
						return nil, Usage{}, err
					}
				case "signature_delta":
					if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkReasoning, Signature: payload.Delta.Signature}, onChunk); err != nil {
						return nil, Usage{}, err
					}
				case "input_json_delta":
					if err := appendProviderChunks(&chunks, toolCalls.add(payload.Index, "", "", payload.Delta.PartialJSON), onChunk); err != nil {
						return nil, Usage{}, err
					}
				}
			}
		case "content_block_stop":
			flushed, err := toolCalls.flush()
			if err != nil {
				return chunks, usage, err
			}
			if err := appendProviderChunks(&chunks, flushed, onChunk); err != nil {
				return nil, Usage{}, err
			}
		case "message_delta":
			if payload.Usage != nil {
				outTok = payload.Usage.OutputTokens
			}
		case "message_stop":
			flushed, err := toolCalls.flush()
			if err != nil {
				return chunks, usage, err
			}
			if err := appendProviderChunks(&chunks, flushed, onChunk); err != nil {
				return nil, Usage{}, err
			}
			prompt := inTok + cacheCreate + cacheRead
			miss := 0
			if hasCacheTelemetry {
				miss = inTok + cacheCreate
			}
			usage = providerUsage(prompt, outTok, prompt+outTok, 0, cacheRead, miss, hasCacheTelemetry)
			if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkUsage, Usage: usage}, onChunk); err != nil {
				return nil, Usage{}, err
			}
			if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkDone}, onChunk); err != nil {
				return nil, Usage{}, err
			}
			completed = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, Usage{}, err
	}
	if !completed {
		if toolCalls.hasPending() {
			return nil, Usage{}, fmt.Errorf("provider messages stream ended before completing tool calls: %w", io.ErrUnexpectedEOF)
		}
		return nil, Usage{}, fmt.Errorf("provider messages stream ended before completion: %w", io.ErrUnexpectedEOF)
	}
	return chunks, usage, nil
}

func parseOpenAISSEPayload(data string) ([]Chunk, Usage, bool, error) {
	var payload struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens            int                       `json:"prompt_tokens"`
			CompletionTokens        int                       `json:"completion_tokens"`
			TotalTokens             int                       `json:"total_tokens"`
			PromptCacheHitTokens    *int                      `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens   *int                      `json:"prompt_cache_miss_tokens"`
			PromptTokensDetails     *openAIPromptTokenDetails `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, Usage{}, false, err
	}
	chunks := []Chunk{}
	for _, choice := range payload.Choices {
		if choice.Delta.ReasoningContent != "" {
			chunks = append(chunks, Chunk{Kind: ChunkReasoning, Text: choice.Delta.ReasoningContent})
		}
		if choice.Delta.Content != "" {
			chunks = append(chunks, Chunk{Kind: ChunkText, Text: choice.Delta.Content})
		}
	}
	if payload.Usage == nil {
		return chunks, Usage{}, false, nil
	}
	hit, miss, hasCacheTelemetry, cacheErr := openAICacheUsage(
		payload.Usage.PromptTokens,
		payload.Usage.PromptCacheHitTokens,
		payload.Usage.PromptCacheMissTokens,
		payload.Usage.PromptTokensDetails,
	)
	if cacheErr != nil {
		return nil, Usage{}, false, cacheErr
	}
	if hasCacheTelemetry && payload.Usage.PromptCacheMissTokens == nil && payload.Usage.PromptTokens > hit {
		miss = payload.Usage.PromptTokens - hit
	}
	reasoning := 0
	if payload.Usage.CompletionTokensDetails != nil {
		reasoning = payload.Usage.CompletionTokensDetails.ReasoningTokens
	}
	usage := providerUsage(payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens, reasoning, hit, miss, hasCacheTelemetry)
	chunks = append(chunks, Chunk{Kind: ChunkUsage, Usage: usage})
	return chunks, usage, true, nil
}

func parseOpenAIChatSSEPayload(data string, accumulator *ToolCallAccumulator, thinkingTags *ThinkSplitter) ([]Chunk, Usage, bool, string, error) {
	return parseOpenAIChatSSEPayloadAt(data, accumulator, thinkingTags, time.Time{})
}

func parseOpenAIChatSSEPayloadAt(data string, accumulator *ToolCallAccumulator, thinkingTags *ThinkSplitter, rawSSEChunkAt time.Time) ([]Chunk, Usage, bool, string, error) {
	chunks, usage, ok, finishReason, _, err := parseOpenAIChatSSEPayloadAtObserved(data, accumulator, thinkingTags, rawSSEChunkAt)
	return chunks, usage, ok, finishReason, err
}

func parseOpenAIChatSSEPayloadAtObserved(data string, accumulator *ToolCallAccumulator, thinkingTags *ThinkSplitter, rawSSEChunkAt time.Time) ([]Chunk, Usage, bool, string, PayloadObservation, error) {
	var payload struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function *struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens            int                       `json:"prompt_tokens"`
			CompletionTokens        int                       `json:"completion_tokens"`
			TotalTokens             int                       `json:"total_tokens"`
			PromptCacheHitTokens    *int                      `json:"prompt_cache_hit_tokens"`
			PromptCacheMissTokens   *int                      `json:"prompt_cache_miss_tokens"`
			PromptTokensDetails     *openAIPromptTokenDetails `json:"prompt_tokens_details"`
			CompletionTokensDetails *struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, Usage{}, false, "", PayloadObservationControl, err
	}
	chunks := []Chunk{}
	finishReason := ""
	finishReasonSet := false
	observation := PayloadObservationControl
	for _, choice := range payload.Choices {
		if choice.Delta.ReasoningContent != "" {
			observation = observation.MergeV1(PayloadObservationPrivateReasoning)
			chunks = append(chunks, Chunk{Kind: ChunkReasoning, Text: choice.Delta.ReasoningContent})
		}
		if choice.Delta.Content != "" {
			observation = observation.MergeV1(PayloadObservationPublicText)
			if err := thinkingTags.push(choice.Delta.Content, rawSSEChunkAt); err != nil {
				return nil, Usage{}, false, "", observation, err
			}
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			observation = observation.MergeV1(PayloadObservationToolStarted)
			name := ""
			args := ""
			if toolCall.Function != nil {
				name = toolCall.Function.Name
				args = toolCall.Function.Arguments
			}
			chunks = append(chunks, accumulator.add(toolCall.Index, toolCall.ID, name, args)...)
		}
		if choice.FinishReason != nil {
			nextFinishReason := *choice.FinishReason
			if finishReasonSet && domainmodel.NormalizeFinishReason(nextFinishReason) != domainmodel.NormalizeFinishReason(finishReason) {
				return nil, Usage{}, false, "", observation, domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
			}
			if !finishReasonSet {
				finishReason = nextFinishReason
				finishReasonSet = true
			}
		}
	}
	if payload.Usage == nil {
		return chunks, Usage{}, false, finishReason, observation, nil
	}
	hit, miss, hasCacheTelemetry, cacheErr := openAICacheUsage(
		payload.Usage.PromptTokens,
		payload.Usage.PromptCacheHitTokens,
		payload.Usage.PromptCacheMissTokens,
		payload.Usage.PromptTokensDetails,
	)
	if cacheErr != nil {
		return nil, Usage{}, false, "", observation, cacheErr
	}
	if hasCacheTelemetry && payload.Usage.PromptCacheMissTokens == nil && payload.Usage.PromptTokens > hit {
		miss = payload.Usage.PromptTokens - hit
	}
	reasoning := 0
	if payload.Usage.CompletionTokensDetails != nil {
		reasoning = payload.Usage.CompletionTokensDetails.ReasoningTokens
	}
	usage := providerUsage(payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens, reasoning, hit, miss, hasCacheTelemetry)
	chunks = append(chunks, Chunk{Kind: ChunkUsage, Usage: usage})
	return chunks, usage, true, finishReason, observation, nil
}

func parseAnthropicSSEPayload(data string) ([]Chunk, Usage, bool, error) {
	var payload struct {
		Type    string `json:"type"`
		Message *struct {
			Usage *struct {
				InputTokens              int  `json:"input_tokens"`
				CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
		Delta *struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			Signature   string `json:"signature"`
			StopReason  string `json:"stop_reason"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
		Usage *struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, Usage{}, false, err
	}
	switch payload.Type {
	case "message_start":
		if payload.Message != nil && payload.Message.Usage != nil {
			cacheCreate, cacheRead := 0, 0
			hasCacheTelemetry := false
			if payload.Message.Usage.CacheCreationInputTokens != nil {
				cacheCreate = *payload.Message.Usage.CacheCreationInputTokens
				hasCacheTelemetry = true
			}
			if payload.Message.Usage.CacheReadInputTokens != nil {
				cacheRead = *payload.Message.Usage.CacheReadInputTokens
				hasCacheTelemetry = true
			}
			prompt := payload.Message.Usage.InputTokens +
				cacheCreate +
				cacheRead
			miss := 0
			if hasCacheTelemetry {
				miss = payload.Message.Usage.InputTokens + cacheCreate
			}
			usage := providerUsage(prompt, 0, prompt, 0, cacheRead, miss, hasCacheTelemetry)
			return []Chunk{{Kind: ChunkUsage, Usage: usage}}, usage, true, nil
		}
	case "content_block_delta":
		if payload.Delta != nil {
			switch payload.Delta.Type {
			case "text_delta":
				return []Chunk{{Kind: ChunkText, Text: payload.Delta.Text}}, Usage{}, false, nil
			case "thinking_delta":
				return []Chunk{{Kind: ChunkReasoning, Text: payload.Delta.Thinking}}, Usage{}, false, nil
			case "signature_delta":
				return []Chunk{{Kind: ChunkReasoning, Signature: payload.Delta.Signature}}, Usage{}, false, nil
			}
		}
	case "message_delta":
		if payload.Usage != nil {
			usage := providerUsage(0, payload.Usage.OutputTokens, payload.Usage.OutputTokens, 0, 0, 0, false)
			return []Chunk{{Kind: ChunkUsage, Usage: usage}}, usage, true, nil
		}
	case "message_stop":
		return []Chunk{{Kind: ChunkDone}}, Usage{}, false, nil
	}
	return nil, Usage{}, false, nil
}

type openAIPromptTokenDetails struct {
	CachedTokens *int `json:"cached_tokens"`
}

func openAICacheUsage(prompt int, nativeHit *int, nativeMiss *int, details *openAIPromptTokenDetails) (int, int, bool, error) {
	if prompt < 0 {
		return 0, 0, false, fmt.Errorf("provider cache usage prompt tokens are invalid")
	}
	hit, miss := 0, 0
	if nativeHit != nil || nativeMiss != nil {
		if nativeHit != nil {
			hit = *nativeHit
		}
		if nativeMiss != nil {
			miss = *nativeMiss
		}
		if hit < 0 || miss < 0 || hit > prompt || miss > prompt {
			return 0, 0, false, fmt.Errorf("provider cache usage native counters are invalid")
		}
		if nativeHit == nil {
			hit = prompt - miss
		}
		if nativeMiss == nil {
			miss = prompt - hit
		}
		if hit+miss != prompt {
			return 0, 0, false, fmt.Errorf("provider cache usage native counters do not cover prompt tokens")
		}
		if details != nil && details.CachedTokens != nil {
			if *details.CachedTokens < 0 || *details.CachedTokens != hit {
				return 0, 0, false, fmt.Errorf("provider cache usage native and nested counters conflict")
			}
		}
		return hit, miss, true, nil
	}
	if details != nil && details.CachedTokens != nil {
		hit = *details.CachedTokens
		if hit < 0 || hit > prompt {
			return 0, 0, false, fmt.Errorf("provider cache usage nested counter is invalid")
		}
		miss = prompt - hit
		return hit, miss, true, nil
	}
	return 0, 0, false, nil
}

func providerUsage(prompt, completion, total, reasoning, hit, miss int, hasCacheTelemetry bool) Usage {
	return providerusage.FromTokens(prompt, completion, total, reasoning, hit, miss, hasCacheTelemetry)
}

func hasChunkKind(chunks []Chunk, kind ChunkKind) bool {
	for _, chunk := range chunks {
		if chunk.Kind == kind {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func NewToolCallAccumulator() *ToolCallAccumulator {
	return newToolCallAccumulator()
}

func NewThinkSplitter() *ThinkSplitter {
	return &ThinkSplitter{}
}

func ParseOpenAIChatSSEPayloadWithState(data string, accumulator *ToolCallAccumulator, thinkingTags *ThinkSplitter) ([]Chunk, Usage, bool, string, error) {
	if accumulator == nil {
		accumulator = newToolCallAccumulator()
	}
	if thinkingTags == nil {
		thinkingTags = &ThinkSplitter{}
	}
	return parseOpenAIChatSSEPayload(data, accumulator, thinkingTags)
}

func ParseOpenAIChatSSEPayload(data string) ([]Chunk, Usage, bool, string, error) {
	return ParseOpenAIChatSSEPayloadWithState(data, nil, nil)
}

func ParseOpenAISSEPayload(data string) ([]Chunk, Usage, bool, error) {
	return parseOpenAISSEPayload(data)
}

func ParseAnthropicSSEPayload(data string) ([]Chunk, Usage, bool, error) {
	return parseAnthropicSSEPayload(data)
}
