package stream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	providerusage "analytix.local/runtime-go/internal/adapters/outbound/provider/usage"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	streamframe "analytix.local/runtime-go/internal/domain/streamframe"
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
	return ParseSSEWithPrivateDisplay(endpointFormat, body, onChunk, onPayload, nil)
}

// ParseSSEWithPrivateDisplay observes actual text deltas separately from the
// transactional output parser. Draft/argument callbacks never change chunks,
// parsing, replay admission or continuation and never observe native signatures.
func ParseSSEWithPrivateDisplay(endpointFormat string, body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation), onDraft func(domainmodel.ProcessFrameV1), onFirstContent ...func(time.Time)) ([]Chunk, Usage, error) {
	observePrivate := func(frame domainmodel.ProcessFrameV1) {
		if onDraft == nil {
			return
		}
		defer func() { _ = recover() }()
		onDraft(frame)
	}
	observeDraft := func(text, classification string) {
		if text == "" || onDraft == nil {
			return
		}
		defer func() { _ = recover() }()
		onDraft(domainmodel.ProcessFrameV1{Source: "assistant_draft", Kind: "delta", Text: text, ContentClassification: classification})
	}
	observeText := func(chunk Chunk) error {
		if chunk.Kind == ChunkText {
			observeDraft(chunk.Text, "provider_text")
		}
		if onChunk != nil {
			return onChunk(chunk)
		}
		return nil
	}
	if endpointFormat == "messages" {
		return parseAnthropicProviderSSEWithPrivateDisplay(body, observeText, onPayload, observePrivate, onFirstContent...)
	}
	if endpointFormat == "responses" {
		return parseResponsesProviderSSEWithPrivateDisplay(body, observeText, onPayload, observePrivate, onFirstContent...)
	}
	// Private preparation frames queued by the Chat payload parser share the
	// parsed reasoning order, while retaining their callback-only route.
	onChatChunk := func(chunk Chunk) error {
		if chunk.ProcessFrame != nil && chunk.ProcessFrame.Source == "tool_preparation" {
			observePrivate(*chunk.ProcessFrame)
			return nil
		}
		if onChunk != nil {
			return onChunk(chunk)
		}
		return nil
	}
	observeContent := nativeFirstContentObserverV1("chat_completions", onFirstContent)
	chunks := []Chunk{}
	usage := Usage{UsagePresenceKnown: true}
	usagePending := false
	toolCalls := newToolCallAccumulator()
	toolCalls.onPreparation = observePrivate
	completed := false
	responseModel := ""
	modelInspected := false
	terminalFinishReason := ""
	thinkingTags := ThinkSplitter{onDraft: func(text string) { observeDraft(text, "unclassified_inline") }}
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
		if frame := thinkingTags.endProviderReasoningV1(); frame != nil {
			notifyPrivateProcessChunkV1(onChatChunk, *frame)
		}
		publicChunks, err := thinkingTags.flushChunks()
		if err != nil {
			return err
		}
		if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(publicChunks, rawSSEChunkAt, parsedAt), onChatChunk); err != nil {
			return err
		}
		if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(terminalToolChunks, rawSSEChunkAt, parsedAt), onChatChunk); err != nil {
			return err
		}
		if usagePending {
			if err := appendProviderChunk(&chunks, annotateProviderChunkTime(Chunk{Kind: ChunkUsage, Usage: usage}, rawSSEChunkAt, parsedAt), onChatChunk); err != nil {
				return err
			}
			usagePending = false
		}
		done := Chunk{Kind: ChunkDone, Trace: domainmodel.ChunkTrace{ProviderResponseModel: responseModel}}
		if usage.FinishReason == "length" {
			done.DiscardedToolMaterial = toolCalls.discardedOutputMaterialV1()
		}
		if err := appendProviderChunk(&chunks, annotateProviderChunkTime(done, rawSSEChunkAt, parsedAt), onChatChunk); err != nil {
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
		if !modelInspected {
			responseModel = responseModelFromEnvelope(data)
			modelInspected = true
		}
		observeContent(data, rawSSEChunkAt)
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
			qualifyNativeUsageCountersV1(&itemUsage, terminalFinishReason != "")
			usage = mergeNativeUsageCountersV1(usage, itemUsage)
			usagePending = true
		}
		providerChunks := make([]Chunk, 0, len(itemChunks))
		for _, chunk := range itemChunks {
			if chunk.Kind != ChunkUsage {
				providerChunks = append(providerChunks, chunk)
			}
		}
		if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(providerChunks, rawSSEChunkAt, parsedAt), onChatChunk); err != nil {
			return chunks, usage, err
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
	providerReasoningOrdinal uint64
	providerReasoningOpen    bool
	onDraft                  func(string)
	decoder                  *domainreasoningmarkup.Decoder
	parseErr                 error
	firstRawSSEChunkAt       time.Time
	lastRawSSEChunkAt        time.Time
	finished                 bool
}

// Chat Completions has reasoning_content deltas, not native block IDs. These
// host segment ordinals belong only to this parser/call and never claim an
// upstream native index. The original reasoning chunk remains unchanged.
func (t *ThinkSplitter) providerReasoningV1(text string) []Chunk {
	if text == "" {
		return nil
	}
	chunks := []Chunk{}
	if !t.providerReasoningOpen {
		t.providerReasoningOrdinal++
		t.providerReasoningOpen = true
		chunks = append(chunks, Chunk{Kind: ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{
			Source: "provider_reasoning", Kind: "start", HostSegmentOrdinal: t.providerReasoningOrdinal,
		}})
	}
	return append(chunks, Chunk{Kind: ChunkReasoning, Text: text, ProcessFrame: &domainmodel.ProcessFrameV1{
		Source: "provider_reasoning", Kind: "delta", HostSegmentOrdinal: t.providerReasoningOrdinal, Text: text,
	}})
}

func (t *ThinkSplitter) endProviderReasoningV1() *Chunk {
	if !t.providerReasoningOpen {
		return nil
	}
	t.providerReasoningOpen = false
	return &Chunk{Kind: ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{
		Source: "provider_reasoning", Kind: "end", HostSegmentOrdinal: t.providerReasoningOrdinal,
	}}
}

func (t *ThinkSplitter) push(value string, rawSSEChunkAt time.Time) error {
	// Observation precedes classification and is confined to the explicitly
	// unclassified private draft slot. It cannot publish a final answer.
	if t.onDraft != nil {
		t.onDraft(value)
	}
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
	if value != "" && !rawSSEChunkAt.IsZero() {
		t.lastRawSSEChunkAt = rawSSEChunkAt
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
		Trace: domainmodel.ChunkTrace{ProviderRawSSEChunkAt: t.firstRawSSEChunkAt, ProviderLastRawSSEChunkAt: t.lastRawSSEChunkAt},
	}}, nil
}

func appendProviderChunk(chunks *[]Chunk, chunk Chunk, onChunk func(Chunk) error) error {
	// Framing is callback-only. Preserve the exact pre-existing continuation
	// chunks, without adding metadata chunks or retaining display frames.
	processOnly := chunk.ProcessFrame != nil && (chunk.ProcessFrame.Kind != "delta" || chunk.ProcessFrame.Source == "tool_preparation" || chunk.ProcessFrame.ReasoningForm == "summary") && chunk.Kind == ChunkReasoning && chunk.Text == "" && chunk.Signature == ""
	if !processOnly {
		continuation := chunk
		continuation.ProcessFrame = nil
		*chunks = append(*chunks, continuation)
	}
	if processOnly {
		notifyPrivateProcessChunkV1(onChunk, chunk)
		return nil
	}
	if onChunk != nil {
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return nil
}

// A display observer cannot change provider parsing or continuation outcomes.
func notifyPrivateProcessChunkV1(onChunk func(Chunk) error, chunk Chunk) {
	if onChunk == nil {
		return
	}
	defer func() { _ = recover() }()
	_ = onChunk(chunk)
}

func nativeProcessChunkV1(kind string, index int, id string, text string) Chunk {
	return Chunk{Kind: ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{
		Kind: kind, NativeIndex: index, NativeID: id, Text: text,
	}}
}

// Native display identity is retained for the complete physical stream. A
// conflicting, opaque or reused source index cannot become eligible again.
// Continuation chunks remain independent of this display-only tracker.
type nativeProcessIdentityV1 struct {
	id             string
	open, poisoned bool
}

type nativeProcessTrackerV1 struct {
	items     map[int]nativeProcessIdentityV1
	seenIDs   map[string]struct{}
	exhausted bool
}

func newNativeProcessTrackerV1() *nativeProcessTrackerV1 {
	return &nativeProcessTrackerV1{
		items: map[int]nativeProcessIdentityV1{}, seenIDs: map[string]struct{}{},
	}
}

// Remember every actual nonempty ID, including IDs supplied by conflicting
// events at indexes that already exist. Index identity alone cannot retain all
// conflicting IDs. Return the presence before this event and whether the ID
// fits the bounded physical-stream budget. Empty Messages IDs stay index-only.
func (tracker *nativeProcessTrackerV1) rememberID(id string) (bool, bool) {
	if id == "" {
		return false, true
	}
	if _, exists := tracker.seenIDs[id]; exists {
		return true, true
	}
	if tracker.exhausted || len(id) > 256 || len(tracker.seenIDs) >= 256 {
		tracker.exhausted = true
		for index, value := range tracker.items {
			if value.open {
				value.open, value.poisoned = false, true
				tracker.items[index] = value
			}
		}
		return false, false
	}
	tracker.seenIDs[id] = struct{}{}
	return false, true
}

func (tracker *nativeProcessTrackerV1) poison(index *int, id string) {
	_, _ = tracker.rememberID(id)
	hasIndex := index != nil && *index >= 0
	for key, value := range tracker.items {
		// A conflicting index/ID pair invalidates both possible sources.
		matches := hasIndex && key == *index || id != "" && value.id == id || !hasIndex && id == "" && value.open
		if matches {
			value.open, value.poisoned = false, true
			tracker.items[key] = value
		}
	}
	if !hasIndex || tracker.exhausted {
		return
	}
	if _, exists := tracker.items[*index]; exists {
		return
	}
	if len(tracker.items) >= 256 {
		tracker.exhausted = true
		tracker.poison(nil, "")
		return
	}
	if len(id) > 256 {
		id = ""
	}
	// Retain bounded actual source IDs even when the first boundary was
	// opaque or invalid, so reuse at another index cannot regain authority.
	tracker.items[*index] = nativeProcessIdentityV1{id: id, poisoned: true}
}

func (tracker *nativeProcessTrackerV1) start(index *int, id string, eligible bool) bool {
	seen, withinBudget := tracker.rememberID(id)
	if index == nil || *index < 0 {
		tracker.poison(index, "")
		return false
	}
	if seen || !withinBudget {
		tracker.poison(index, id)
		return false
	}
	if _, exists := tracker.items[*index]; exists {
		tracker.poison(index, id)
		return false
	}
	if tracker.exhausted || len(tracker.items) >= 256 {
		tracker.exhausted = true
		tracker.poison(nil, "")
		return false
	}
	if !eligible || len(id) > 256 {
		tracker.poison(index, id)
		return false
	}
	tracker.items[*index] = nativeProcessIdentityV1{id: id, open: true}
	return true
}

func (tracker *nativeProcessTrackerV1) delta(index *int, id string, requireID bool) (string, bool) {
	if tracker.exhausted || index == nil || *index < 0 || requireID && id == "" {
		tracker.poison(index, id)
		return "", false
	}
	value, exists := tracker.items[*index]
	if !exists || !value.open || value.poisoned || requireID && value.id != id {
		tracker.poison(index, id)
		return "", false
	}
	return value.id, true
}

func (tracker *nativeProcessTrackerV1) end(index *int, id string, eligible, requireID bool) (string, bool) {
	if !eligible {
		tracker.poison(index, id)
		return "", false
	}
	nativeID, ok := tracker.delta(index, id, requireID)
	if !ok {
		return "", false
	}
	value := tracker.items[*index]
	value.open = false
	tracker.items[*index] = value
	return nativeID, true
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
	deferredArgumentError error
	order                 []int
	byKey                 map[int]*partialToolCall
	onPreparation         func(domainmodel.ProcessFrameV1)
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
	firstObservation := item == nil
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
	if firstObservation {
		a.observePreparationV1("start", index, item, "")
	}
	if arguments != "" {
		item.Arguments.WriteString(arguments)
	}
	if item.Started {
		a.observePreparationV1("delta", index, item, arguments)
		return nil
	}
	if strings.TrimSpace(item.Name) == "" {
		return nil
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = fmt.Sprintf("call_%d", index)
	}
	item.Started = true
	// The first named observation includes fragments already accumulated before
	// the name arrived. Subsequent observations remain actual incremental deltas.
	a.observePreparationV1("delta", index, item, item.Arguments.String())
	return []Chunk{{
		Kind: ChunkToolCallStart,
		ToolCall: ToolCall{
			ID:   item.ID,
			Name: item.Name,
		},
	}}
}

func (a *ToolCallAccumulator) observePreparationV1(kind string, index int, item *partialToolCall, arguments string) {
	if a.onPreparation == nil || item == nil || kind != "start" && !item.Started {
		return
	}
	name := item.Name
	if kind == "start" {
		// The actual slot's first observation is control only. A name and an
		// execution identity may arrive later; neither is invented here.
		name, arguments = "", ""
	}
	defer func() { _ = recover() }()
	a.onPreparation(domainmodel.ProcessFrameV1{Source: "tool_preparation", Kind: kind,
		ToolPreparation: &streamframe.ToolPreparationV1{Index: index, CallID: item.ID, ToolName: name, Arguments: arguments}})
}

// A Responses done item carries the authoritative full argument string, not
// another delta. Replacing it also repairs a conflicting streamed prefix.
func (a *ToolCallAccumulator) replaceArgumentsV1(index int, arguments string) {
	if item := a.byKey[index]; item != nil {
		item.Arguments.Reset()
		item.Arguments.WriteString(arguments)
	}
}

func (a *ToolCallAccumulator) flush() ([]Chunk, error) {
	if a.deferredArgumentError != nil {
		return nil, a.deferredArgumentError
	}
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
		if json.Unmarshal([]byte(args), &arguments) != nil || arguments == nil {
			return nil, domainfailure.NewError(domainfailure.CodeProviderToolArgumentsInvalid, nil)
		}
		a.observePreparationV1("end", index, item, args)
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

// Each Messages block closes independently; a slow interleaved block must not
// be parsed prematurely when another block stops.
func (a *ToolCallAccumulator) flushIndex(index int) ([]Chunk, error) {
	item := a.byKey[index]
	if item == nil {
		return nil, nil
	}
	single := &ToolCallAccumulator{order: []int{index}, byKey: map[int]*partialToolCall{index: item}, onPreparation: a.onPreparation}
	chunks, err := single.flush()
	if err != nil {
		return nil, err
	}
	delete(a.byKey, index)
	for i, key := range a.order {
		if key == index {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}
	return chunks, nil
}

func (a *ToolCallAccumulator) discardedOutputMaterialV1() string {
	var material strings.Builder
	for _, index := range a.order {
		if item := a.byKey[index]; item != nil {
			material.WriteString(item.ID)
			material.WriteString(item.Name)
			material.WriteString(item.Arguments.String())
		}
	}
	return material.String()
}

func (a *ToolCallAccumulator) hasPending() bool {
	return len(a.order) > 0
}

func parseResponsesProviderSSE(body io.Reader) ([]Chunk, Usage, error) {
	return parseResponsesProviderSSEWithCallback(body, nil, nil)
}

func parseResponsesProviderSSEWithCallback(body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation)) ([]Chunk, Usage, error) {
	return parseResponsesProviderSSEWithPrivateDisplay(body, onChunk, onPayload, nil)
}

func parseResponsesProviderSSEWithPrivateDisplay(body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation), onPrivate func(domainmodel.ProcessFrameV1), onFirstContent ...func(time.Time)) ([]Chunk, Usage, error) {
	observeContent := nativeFirstContentObserverV1("responses", onFirstContent)
	chunks := []Chunk{}
	usage := Usage{UsagePresenceKnown: true}
	toolCalls := newToolCallAccumulator()
	toolCalls.onPreparation = onPrivate
	itemIndexes := map[string]int{}
	reasoningItems := newNativeProcessTrackerV1()
	summaryItems := newResponsesSummaryTrackerV1()
	completed := false
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	// The first actual terminal seals this physical stream. Do not read more
	// provider bytes: late frames, usage or read failures cannot reopen it.
	for !completed && scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		rawAt := time.Now()
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			flushed, err := toolCalls.flush()
			if err != nil {
				return chunks, usage, err
			}
			if err := appendProviderChunks(&chunks, flushed, onChunk); err != nil {
				return chunks, usage, err
			}
			if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkDone}, onChunk); err != nil {
				return chunks, usage, err
			}
			completed = true
			summaryItems.close()
			continue
		}
		observeContent(data, rawAt)
		observePayload(onPayload, PayloadObservationControl)
		itemChunks, itemUsage, ok, observation, err := parseResponsesSSEPayload(data, toolCalls, itemIndexes, reasoningItems, summaryItems)
		observePayload(onPayload, observation)
		if err != nil {
			if ok {
				// A real terminal usage report survives local display/tool errors.
				return chunks, itemUsage, err
			}
			return chunks, usage, err
		}
		if ok {
			usage = itemUsage
		} else if itemUsage.FinishReason != "" {
			usage.FinishReason = itemUsage.FinishReason
		}
		if err := appendProviderChunks(&chunks, itemChunks, onChunk); err != nil {
			return chunks, usage, err
		}
		if hasChunkKind(itemChunks, ChunkDone) {
			completed = true
			summaryItems.close()
		}
		if ok {
			usage = itemUsage
		}
	}
	if err := scanner.Err(); err != nil && !completed {
		return chunks, usage, err
	}
	if !completed {
		if toolCalls.deferredArgumentError != nil {
			return chunks, usage, toolCalls.deferredArgumentError
		}
		if toolCalls.hasPending() {
			return chunks, usage, fmt.Errorf("provider responses stream ended before completing tool calls: %w", io.ErrUnexpectedEOF)
		}
		return chunks, usage, fmt.Errorf("provider responses stream ended before completion: %w", io.ErrUnexpectedEOF)
	}
	return chunks, usage, nil
}

func parseResponsesSSEPayload(data string, accumulator *ToolCallAccumulator, itemIndexes map[string]int, reasoningItems *nativeProcessTrackerV1, summaryItems *responsesSummaryTrackerV1) ([]Chunk, Usage, bool, PayloadObservation, error) {
	var summaryFields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &summaryFields); err != nil {
		return nil, Usage{}, false, PayloadObservationControl, err
	}
	kind, _ := responsesSummaryStringV1(summaryFields["type"])
	switch kind {
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
		// Summary-only validation cannot fail a model response or turn a summary
		// into retained reasoning continuation. Its raw counterpart stays closed.
		id, _ := responsesSummaryStringV1(summaryFields["item_id"])
		index := responsesSummaryIndexV1(summaryFields["output_index"])
		if !summaryItems.rawSelected(index, id) {
			reasoningItems.poison(index, id)
		}
		chunks := summaryItems.consume(kind, summaryFields)
		observation := PayloadObservationControl
		for _, chunk := range chunks {
			if chunk.ProcessFrame != nil && chunk.ProcessFrame.Text != "" {
				observation = PayloadObservationPrivateReasoning
			}
		}
		return chunks, Usage{}, false, observation, nil
	}
	var payload struct {
		Type        string `json:"type"`
		Delta       string `json:"delta"`
		ItemID      string `json:"item_id"`
		OutputIndex *int   `json:"output_index"`
		Item        *struct {
			ID               string          `json:"id"`
			Type             string          `json:"type"`
			CallID           string          `json:"call_id"`
			Name             string          `json:"name"`
			Arguments        *string         `json:"arguments"`
			Summary          json.RawMessage `json:"summary"`
			Status           json.RawMessage `json:"status"`
			EncryptedContent json.RawMessage `json:"encrypted_content"`
			Redacted         json.RawMessage `json:"redacted"`
		} `json:"item"`
		Response *struct {
			IncompleteDetails *struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
			Usage *struct {
				InputTokens        int `json:"input_tokens"`
				OutputTokens       int `json:"output_tokens"`
				TotalTokens        int `json:"total_tokens"`
				InputTokensDetails *struct {
					CachedTokens *int `json:"cached_tokens"`
				} `json:"input_tokens_details"`
				OutputTokensDetails *struct {
					ReasoningTokens *int `json:"reasoning_tokens"`
				} `json:"output_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, Usage{}, false, PayloadObservationControl, err
	}
	outputIndex := 0
	if payload.OutputIndex != nil {
		outputIndex = *payload.OutputIndex
	}
	switch payload.Type {
	case "response.output_text.delta":
		reasoningItems.poison(payload.OutputIndex, payload.ItemID)
		summaryItems.poison(payload.OutputIndex, payload.ItemID)
		return []Chunk{{Kind: ChunkText, Text: payload.Delta}}, Usage{}, false, PayloadObservationPublicText, nil
	case "response.reasoning_text.delta":
		chunk := Chunk{Kind: ChunkReasoning, Text: payload.Delta}
		if id, ok := reasoningItems.delta(payload.OutputIndex, payload.ItemID, true); ok && payload.Delta != "" {
			if first, allowed := summaryItems.rawDelta(payload.OutputIndex, payload.ItemID); allowed {
				chunk.ProcessFrame = nativeProcessChunkV1("delta", outputIndex, id, payload.Delta).ProcessFrame
				if first {
					return []Chunk{nativeProcessChunkV1("start", outputIndex, id, ""), chunk}, Usage{}, false, PayloadObservationPrivateReasoning, nil
				}
			}
		}
		return []Chunk{chunk}, Usage{}, false, PayloadObservationPrivateReasoning, nil
	case "response.reasoning_text.done":
		// A mismatched text boundary cannot complete a previously opened source.
		_, _ = reasoningItems.delta(payload.OutputIndex, payload.ItemID, true)
		return []Chunk{}, Usage{}, false, PayloadObservationControl, nil
	case "response.output_item.added":
		// Track every native output index so cross-type reuse also poisons it.
		if payload.Item != nil {
			declaredSummary := summaryItems.begin(payload.OutputIndex, payload.Item.ID, payload.Item.Type, payload.Item.Summary)
			eligible := payload.Item.Type == "reasoning" && payload.Item.ID != "" && len(payload.Item.EncryptedContent) == 0 && len(payload.Item.Redacted) == 0 && !(declaredSummary && summaryItems.closed)
			if reasoningItems.start(payload.OutputIndex, payload.Item.ID, eligible) && !declaredSummary {
				return []Chunk{nativeProcessChunkV1("start", outputIndex, payload.Item.ID, "")}, Usage{}, false, PayloadObservationPrivateReasoning, nil
			}
		} else {
			summaryItems.poison(payload.OutputIndex, "")
			reasoningItems.poison(payload.OutputIndex, "")
		}
		if payload.Item != nil && payload.Item.Type == "function_call" {
			index := outputIndex
			if payload.Item.ID != "" {
				itemIndexes[payload.Item.ID] = index
			}
			arguments := ""
			if payload.Item.Arguments != nil {
				arguments = *payload.Item.Arguments
			}
			return accumulator.add(index, firstNonEmpty(payload.Item.CallID, payload.Item.ID), payload.Item.Name, arguments), Usage{}, false, PayloadObservationToolStarted, nil
		}
		return []Chunk{}, Usage{}, false, PayloadObservationControl, nil
	case "response.function_call_arguments.delta":
		reasoningItems.poison(payload.OutputIndex, payload.ItemID)
		summaryItems.poison(payload.OutputIndex, payload.ItemID)
		index := outputIndex
		if payload.ItemID != "" {
			if existing, ok := itemIndexes[payload.ItemID]; ok {
				index = existing
			} else {
				itemIndexes[payload.ItemID] = index
			}
		}
		return accumulator.add(index, payload.ItemID, "", payload.Delta), Usage{}, false, PayloadObservationToolStarted, nil
	case "response.output_item.done":
		if payload.Item != nil {
			summaryChunks := summaryItems.finish(payload.OutputIndex, payload.Item.ID, payload.Item.Type, payload.Item.Status, payload.Item.Summary)
			eligible := payload.Item.Type == "reasoning" && payload.Item.ID != "" && len(payload.Item.EncryptedContent) == 0 && len(payload.Item.Redacted) == 0
			if id, ok := reasoningItems.end(payload.OutputIndex, payload.Item.ID, eligible, true); ok && (!summaryItems.declared(payload.OutputIndex, payload.Item.ID) || summaryItems.rawSelected(payload.OutputIndex, payload.Item.ID)) {
				return []Chunk{nativeProcessChunkV1("end", outputIndex, id, "")}, Usage{}, false, PayloadObservationPrivateReasoning, nil
			}
			if len(summaryChunks) > 0 {
				return summaryChunks, Usage{}, false, PayloadObservationPrivateReasoning, nil
			}
		} else {
			summaryItems.poison(payload.OutputIndex, "")
			reasoningItems.poison(payload.OutputIndex, "")
		}
		if payload.Item != nil && payload.Item.Type == "function_call" {
			index := outputIndex
			if payload.Item.ID != "" {
				itemIndexes[payload.Item.ID] = index
			}
			started := accumulator.add(index, firstNonEmpty(payload.Item.CallID, payload.Item.ID), payload.Item.Name, "")
			// Empty final arguments are an omitted full snapshot in existing
			// Responses streams. Keep the real prefix so strict validation cannot
			// turn a truncated call into an executable empty object.
			if payload.Item.Arguments != nil && strings.TrimSpace(*payload.Item.Arguments) != "" {
				accumulator.replaceArgumentsV1(index, *payload.Item.Arguments)
			}
			flushed, err := accumulator.flushIndex(index)
			if err != nil {
				accumulator.deferredArgumentError = err
				return started, Usage{}, false, PayloadObservationToolStarted, nil
			}
			return append(started, flushed...), Usage{}, false, PayloadObservationToolStarted, nil
		}
		return []Chunk{}, Usage{}, false, PayloadObservationControl, nil
	case "response.completed", "response.incomplete":
		incomplete := payload.Type == "response.incomplete"
		var flushed []Chunk
		usage := Usage{}
		if incomplete {
			usage.FinishReason = "unknown"
			if payload.Response != nil && payload.Response.IncompleteDetails != nil && payload.Response.IncompleteDetails.Reason == "max_output_tokens" {
				usage.FinishReason = "length"
			}
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
			if u.OutputTokensDetails != nil && u.OutputTokensDetails.ReasoningTokens != nil {
				reasoning = *u.OutputTokensDetails.ReasoningTokens
			}
			observed := providerUsage(u.InputTokens, u.OutputTokens, u.TotalTokens, reasoning, hit, miss, hasCacheTelemetry)
			setProviderUsagePresenceV1(&observed, data, "input_tokens", "output_tokens", "response", "usage")
			observed.HasReasoningTokens = u.OutputTokensDetails != nil && u.OutputTokensDetails.ReasoningTokens != nil
			observed.HasCacheMiss = hasCacheTelemetry && observed.HasPromptTokens
			qualifyNativeUsageCountersV1(&observed, true)
			observed.FinishReason = usage.FinishReason
			usage = observed

		}
		observed := payload.Response != nil && payload.Response.Usage != nil
		if incomplete && usage.FinishReason != "length" {
			return nil, usage, observed, PayloadObservationControl, domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)
		}
		var err error
		if !incomplete {
			flushed, err = accumulator.flush()
		}
		if err != nil {
			return nil, usage, observed, PayloadObservationControl, err
		}
		done := Chunk{Kind: ChunkDone}
		if incomplete {
			done.DiscardedToolMaterial = accumulator.discardedOutputMaterialV1()
		}
		if observed {
			return append(flushed, Chunk{Kind: ChunkUsage, Usage: usage}, done), usage, true, observeProviderChunks(flushed), nil
		}
		return append(flushed, done), usage, false, observeProviderChunks(flushed), nil
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
	return parseAnthropicProviderSSEWithPrivateDisplay(body, onChunk, onPayload, nil)
}

func parseAnthropicProviderSSEWithPrivateDisplay(body io.Reader, onChunk func(Chunk) error, onPayload func(PayloadObservation), onPrivate func(domainmodel.ProcessFrameV1), onFirstContent ...func(time.Time)) ([]Chunk, Usage, error) {
	observeContent := nativeFirstContentObserverV1("messages", onFirstContent)
	chunks := []Chunk{}
	usage := Usage{UsagePresenceKnown: true}
	inTok, outTok, cacheCreate, cacheRead := 0, 0, 0, 0
	hasInput, hasOutput, hasCreate, hasRead := false, false, false, false
	outputFinality := domaincache.ProviderOutputTokensFinalityV1("")
	toolCalls := newToolCallAccumulator()
	toolCalls.onPreparation = onPrivate
	thinkingBlocks := newNativeProcessTrackerV1()
	completed := false
	stopReason := ""
	responseModel := ""
	hasToolCalls := false
	updateUsage := func() error {
		if inTok < 0 || outTok < 0 || cacheCreate < 0 || cacheRead < 0 || inTok > int(^uint(0)>>1)-cacheCreate || inTok+cacheCreate > int(^uint(0)>>1)-cacheRead || inTok+cacheCreate+cacheRead > int(^uint(0)>>1)-outTok {
			return domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
		}
		known := hasInput && hasCreate && hasRead
		prompt, hit, miss := inTok+cacheCreate+cacheRead, 0, 0
		if known {
			hit, miss = cacheRead, inTok+cacheCreate
		}
		usage = providerUsage(prompt, outTok, prompt+outTok, 0, hit, miss, known)
		usage.FinishReason = domainmodel.NormalizeFinishReason(stopReason)
		usage.UsagePresenceKnown, usage.HasPromptTokens, usage.HasCompletionTokens = true, known, hasOutput
		usage.OutputTokensFinality = outputFinality
		usage.MessagesInput = domaincache.MessagesInputV1{SchemaVersion: "messages-input.v1", Uncached: domaincache.TokenCountV1{Known: hasInput, Value: uint64(inTok)}, Read: domaincache.TokenCountV1{Known: hasRead, Value: uint64(cacheRead)}, Created: domaincache.TokenCountV1{Known: hasCreate, Value: uint64(cacheCreate)}}
		return nil
	}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		rawAt := time.Now()
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		observeContent(data, rawAt)
		observePayload(onPayload, PayloadObservationControl)
		var payload struct {
			Type    string `json:"type"`
			Message *struct {
				Model string `json:"model"`
				Usage *struct {
					InputTokens              *int `json:"input_tokens"`
					OutputTokens             *int `json:"output_tokens"`
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
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage *struct {
				OutputTokens *int `json:"output_tokens"`
			} `json:"usage"`
			NativeIndex  *int `json:"index"`
			ContentBlock *struct {
				Type      string          `json:"type"`
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Input     json.RawMessage `json:"input"`
				Text      string          `json:"text"`
				Thinking  string          `json:"thinking"`
				Signature string          `json:"signature"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return chunks, usage, err
		}
		index := 0
		if payload.NativeIndex != nil {
			index = *payload.NativeIndex
		}
		if completed && payload.Type != "ping" {
			return chunks, usage, domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
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
		case "error":
			return chunks, usage, domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
		case "message_start":
			if payload.Message != nil {
				responseModel = boundedResponseModel(payload.Message.Model)
			}
			if payload.Message != nil && payload.Message.Usage != nil {
				if payload.Message.Usage.InputTokens != nil {
					inTok = *payload.Message.Usage.InputTokens
					hasInput = true
				}
				if payload.Message.Usage.OutputTokens != nil {
					outTok = *payload.Message.Usage.OutputTokens
					hasOutput = true
					outputFinality = domaincache.ProviderOutputTokensProvisionalV1
				}
				if payload.Message.Usage.CacheCreationInputTokens != nil {
					cacheCreate = *payload.Message.Usage.CacheCreationInputTokens
					hasCreate = true
				}
				if payload.Message.Usage.CacheReadInputTokens != nil {
					cacheRead = *payload.Message.Usage.CacheReadInputTokens
					hasRead = true
				}
			}
			if err := updateUsage(); err != nil {
				return chunks, usage, err
			}
		case "content_block_start":
			if payload.ContentBlock != nil {
				block := payload.ContentBlock
				var initial []Chunk
				eligible := block.Type == "thinking"
				if thinkingBlocks.start(payload.NativeIndex, block.ID, eligible) {
					initial = append(initial, nativeProcessChunkV1("start", index, block.ID, ""))
				}
				switch block.Type {
				case "text":
					if block.Text != "" {
						initial = []Chunk{{Kind: ChunkText, Text: block.Text}}
					}
				case "thinking":
					if block.Thinking != "" || block.Signature != "" {
						chunk := Chunk{Kind: ChunkReasoning, Text: block.Thinking, Signature: block.Signature}
						if id, ok := thinkingBlocks.delta(payload.NativeIndex, "", false); ok && block.Thinking != "" {
							chunk.ProcessFrame = nativeProcessChunkV1("delta", index, id, block.Thinking).ProcessFrame
						}
						initial = append(initial, chunk)
					}
				}
				if err := appendProviderChunks(&chunks, annotateProviderChunkTimes(initial, rawAt, time.Now()), onChunk); err != nil {
					return chunks, usage, err
				}
			} else {
				thinkingBlocks.poison(payload.NativeIndex, "")
			}
			if payload.ContentBlock != nil && payload.ContentBlock.Type == "tool_use" {
				hasToolCalls = true
				args := ""
				if len(payload.ContentBlock.Input) > 0 {
					rawInput := strings.TrimSpace(string(payload.ContentBlock.Input))
					if rawInput != "" && rawInput != "{}" && rawInput != "null" {
						args = rawInput
					}
				}
				if err := appendProviderChunks(&chunks, toolCalls.add(index, payload.ContentBlock.ID, payload.ContentBlock.Name, args), onChunk); err != nil {
					return chunks, usage, err
				}
			}
		case "content_block_delta":
			if payload.Delta != nil {
				switch payload.Delta.Type {
				case "text_delta":
					thinkingBlocks.poison(payload.NativeIndex, "")
					if err := appendProviderChunk(&chunks, annotateProviderChunkTime(Chunk{Kind: ChunkText, Text: payload.Delta.Text}, rawAt, time.Now()), onChunk); err != nil {
						return chunks, usage, err
					}
				case "thinking_delta":
					chunk := Chunk{Kind: ChunkReasoning, Text: payload.Delta.Thinking}
					if id, ok := thinkingBlocks.delta(payload.NativeIndex, "", false); ok {
						chunk.ProcessFrame = nativeProcessChunkV1("delta", index, id, payload.Delta.Thinking).ProcessFrame
					}
					if err := appendProviderChunk(&chunks, annotateProviderChunkTime(chunk, rawAt, time.Now()), onChunk); err != nil {
						return chunks, usage, err
					}
				case "signature_delta":
					_, _ = thinkingBlocks.delta(payload.NativeIndex, "", false)
					if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkReasoning, Signature: payload.Delta.Signature}, onChunk); err != nil {
						return chunks, usage, err
					}
				case "input_json_delta":
					thinkingBlocks.poison(payload.NativeIndex, "")
					if err := appendProviderChunks(&chunks, toolCalls.add(index, "", "", payload.Delta.PartialJSON), onChunk); err != nil {
						return chunks, usage, err
					}
				default:
					thinkingBlocks.poison(payload.NativeIndex, "")
				}
			} else {
				thinkingBlocks.poison(payload.NativeIndex, "")
			}
		case "content_block_stop":
			if id, ok := thinkingBlocks.end(payload.NativeIndex, "", true, false); ok {
				if err := appendProviderChunk(&chunks, nativeProcessChunkV1("end", index, id, ""), onChunk); err != nil {
					return chunks, usage, err
				}
			}
			flushed, err := toolCalls.flushIndex(index)
			if err != nil {
				toolCalls.deferredArgumentError = err
				continue
			}
			if err := appendProviderChunks(&chunks, flushed, onChunk); err != nil {
				return chunks, usage, err
			}
		case "message_delta":
			if payload.Delta != nil && payload.Delta.StopReason != "" {
				stopReason = payload.Delta.StopReason
			}
			if payload.Usage != nil && payload.Usage.OutputTokens != nil {
				hasOutput = true
				outputFinality = domaincache.ProviderOutputTokensProvisionalV1
				if stopReason != "" {
					outputFinality = domaincache.ProviderOutputTokensFinalV1
				}
				outTok = *payload.Usage.OutputTokens
			}
			if err := updateUsage(); err != nil {
				return chunks, usage, err
			}
		case "message_stop":
			if inTok < 0 || outTok < 0 || cacheCreate < 0 || cacheRead < 0 {
				return chunks, usage, domainfailure.NewError(domainfailure.CodeProviderRequestRejected, nil)
			}
			if stopReason == "" {
				return chunks, usage, domainfailure.NewError(domainfailure.CodeProviderStreamInterrupted, nil)
			}
			if err := openAIChatTerminalFinishReasonError(stopReason); err != nil {
				usage.FinishReason = domainmodel.NormalizeFinishReason(stopReason)
				return chunks, usage, err
			}
			if err := openAIChatTerminalToolCallError(stopReason, hasToolCalls); err != nil {
				return chunks, usage, err
			}
			if domainmodel.NormalizeFinishReason(stopReason) != "length" {
				flushed, err := toolCalls.flush()
				if err != nil {
					return chunks, usage, err
				}
				if err := appendProviderChunks(&chunks, flushed, onChunk); err != nil {
					return chunks, usage, err
				}
			}
			if err := updateUsage(); err != nil {
				return chunks, usage, err
			}
			if hasInput || hasOutput || hasCreate || hasRead {
				if err := appendProviderChunk(&chunks, Chunk{Kind: ChunkUsage, Usage: usage}, onChunk); err != nil {
					return chunks, usage, err
				}
			}
			done := Chunk{Kind: ChunkDone, Trace: domainmodel.ChunkTrace{ProviderResponseModel: responseModel}}
			if domainmodel.NormalizeFinishReason(stopReason) == "length" {
				done.DiscardedToolMaterial = toolCalls.discardedOutputMaterialV1()
			}
			if err := appendProviderChunk(&chunks, annotateProviderChunkTime(done, rawAt, time.Now()), onChunk); err != nil {
				return chunks, usage, err
			}
			completed = true
		}
	}
	if err := scanner.Err(); err != nil {
		return chunks, usage, err
	}
	if !completed {
		if domainmodel.NormalizeFinishReason(stopReason) != "length" && toolCalls.deferredArgumentError != nil {
			return chunks, usage, toolCalls.deferredArgumentError
		}
		if toolCalls.hasPending() {
			return chunks, usage, fmt.Errorf("provider messages stream ended before completing tool calls: %w", io.ErrUnexpectedEOF)
		}
		return chunks, usage, fmt.Errorf("provider messages stream ended before completion: %w", io.ErrUnexpectedEOF)
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
				ReasoningTokens *int `json:"reasoning_tokens"`
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
	presence := Usage{}
	setProviderUsagePresenceV1(&presence, data, "prompt_tokens", "completion_tokens", "usage")
	hit, miss, hasCacheTelemetry, cacheErr := openAICacheUsageWithPresenceV1(
		payload.Usage.PromptTokens, presence.HasPromptTokens,
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
	if payload.Usage.CompletionTokensDetails != nil && payload.Usage.CompletionTokensDetails.ReasoningTokens != nil {
		reasoning = *payload.Usage.CompletionTokensDetails.ReasoningTokens
	}
	usage := providerUsage(payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens, reasoning, hit, miss, hasCacheTelemetry)
	setProviderUsagePresenceV1(&usage, data, "prompt_tokens", "completion_tokens", "usage")
	usage.HasReasoningTokens = payload.Usage.CompletionTokensDetails != nil && payload.Usage.CompletionTokensDetails.ReasoningTokens != nil
	usage.HasCacheHit = hasCacheTelemetry || payload.Usage.PromptCacheHitTokens != nil || payload.Usage.PromptTokensDetails != nil && payload.Usage.PromptTokensDetails.CachedTokens != nil
	usage.HasCacheMiss = hasCacheTelemetry || payload.Usage.PromptCacheMissTokens != nil
	qualifyNativeUsageCountersV1(&usage, false)
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
				ReasoningTokens *int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, Usage{}, false, "", PayloadObservationControl, err
	}
	chunks := []Chunk{}
	// add() observes named preparation synchronously. Queue those private
	// frames among this payload's reasoning chunks instead of overtaking them.
	observePreparation := accumulator.onPreparation
	if observePreparation != nil {
		accumulator.onPreparation = func(frame domainmodel.ProcessFrameV1) {
			chunks = append(chunks, Chunk{Kind: ChunkReasoning, ProcessFrame: &frame})
		}
		defer func() { accumulator.onPreparation = observePreparation }()
	}
	finishReason := ""
	finishReasonSet := false
	observation := PayloadObservationControl
	for _, choice := range payload.Choices {
		if choice.Delta.ReasoningContent != "" {
			observation = observation.MergeV1(PayloadObservationPrivateReasoning)
			chunks = append(chunks, thinkingTags.providerReasoningV1(choice.Delta.ReasoningContent)...)
		}
		if choice.Delta.Content != "" {
			if end := thinkingTags.endProviderReasoningV1(); end != nil {
				chunks = append(chunks, *end)
			}
			observation = observation.MergeV1(PayloadObservationPublicText)
			if err := thinkingTags.push(choice.Delta.Content, rawSSEChunkAt); err != nil {
				return nil, Usage{}, false, "", observation, err
			}
		}
		if len(choice.Delta.ToolCalls) > 0 {
			if end := thinkingTags.endProviderReasoningV1(); end != nil {
				chunks = append(chunks, *end)
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
			started := accumulator.add(toolCall.Index, toolCall.ID, name, args)
			chunks = append(chunks, started...)
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
	presence := Usage{}
	setProviderUsagePresenceV1(&presence, data, "prompt_tokens", "completion_tokens", "usage")
	hit, miss, hasCacheTelemetry, cacheErr := openAICacheUsageWithPresenceV1(
		payload.Usage.PromptTokens, presence.HasPromptTokens,
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
	if payload.Usage.CompletionTokensDetails != nil && payload.Usage.CompletionTokensDetails.ReasoningTokens != nil {
		reasoning = *payload.Usage.CompletionTokensDetails.ReasoningTokens
	}
	usage := providerUsage(payload.Usage.PromptTokens, payload.Usage.CompletionTokens, payload.Usage.TotalTokens, reasoning, hit, miss, hasCacheTelemetry)
	setProviderUsagePresenceV1(&usage, data, "prompt_tokens", "completion_tokens", "usage")
	usage.HasReasoningTokens = payload.Usage.CompletionTokensDetails != nil && payload.Usage.CompletionTokensDetails.ReasoningTokens != nil
	usage.HasCacheHit = hasCacheTelemetry || payload.Usage.PromptCacheHitTokens != nil || payload.Usage.PromptTokensDetails != nil && payload.Usage.PromptTokensDetails.CachedTokens != nil
	usage.HasCacheMiss = hasCacheTelemetry || payload.Usage.PromptCacheMissTokens != nil
	qualifyNativeUsageCountersV1(&usage, false)
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

// Numeric native counters remain usable when a comparison input is absent.
// Complete-prompt reports still pass the unchanged cache-consistency gate.
func openAICacheUsageWithPresenceV1(prompt int, hasPrompt bool, nativeHit *int, nativeMiss *int, details *openAIPromptTokenDetails) (int, int, bool, error) {
	if prompt < 0 {
		return 0, 0, false, fmt.Errorf("provider cache usage prompt tokens are invalid")
	}
	if hasPrompt {
		return openAICacheUsage(prompt, nativeHit, nativeMiss, details)
	}
	hit, miss := 0, 0
	if nativeHit != nil {
		hit = *nativeHit
	}
	if nativeMiss != nil {
		miss = *nativeMiss
	}
	if hit < 0 || miss < 0 {
		return 0, 0, false, fmt.Errorf("provider cache usage native counters are invalid")
	}
	if details != nil && details.CachedTokens != nil {
		nested := *details.CachedTokens
		if nested < 0 || nativeHit != nil && nested != hit {
			return 0, 0, false, fmt.Errorf("provider cache usage native and nested counters conflict")
		}
		hit = nested
	}
	return hit, miss, false, nil
}

func qualifyNativeUsageCountersV1(usage *Usage, terminalReport bool) {
	qualification := domaincache.ProviderOutputTokensProvisionalV1
	if terminalReport {
		qualification = domaincache.ProviderOutputTokensFinalV1
	}
	if usage.HasCompletionTokens {
		usage.OutputTokensFinality = qualification
	}
	if usage.HasReasoningTokens {
		usage.ReasoningTokensFinality = qualification
	}
}

// Provider snapshots are cumulative. Preserve absent fields, replace each
// actually reported field, and never add snapshots or upgrade on Done alone.
func mergeNativeUsageCountersV1(previous, next Usage) Usage {
	result := previous
	result.UsagePresenceKnown = next.UsagePresenceKnown
	result.FinishReason = next.FinishReason
	if next.HasPromptTokens {
		result.PromptTokens, result.HasPromptTokens = next.PromptTokens, true
	}
	if next.HasCompletionTokens {
		result.CompletionTokens, result.HasCompletionTokens, result.OutputTokensFinality = next.CompletionTokens, true, next.OutputTokensFinality
	}
	if next.HasReasoningTokens {
		result.ReasoningTokens, result.HasReasoningTokens, result.ReasoningTokensFinality = next.ReasoningTokens, true, next.ReasoningTokensFinality
	}
	if next.HasCacheHit {
		result.CacheHitTokens, result.HasCacheHit = next.CacheHitTokens, true
	}
	if next.HasCacheMiss {
		result.CacheMissTokens, result.HasCacheMiss = next.CacheMissTokens, true
	}
	if next.TotalTokens > result.TotalTokens {
		result.TotalTokens = next.TotalTokens
	}
	if result.TotalTokens < result.PromptTokens+result.CompletionTokens {
		result.TotalTokens = result.PromptTokens + result.CompletionTokens
	}
	if result.CacheHitTokens+result.CacheMissTokens > 0 {
		result.CacheHitRate = float64(result.CacheHitTokens) / float64(result.CacheHitTokens+result.CacheMissTokens)
	}
	return result
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

// Provider model echoes are diagnostics, never Registry or execution authority.
// Retain only a bounded protocol identifier; never arbitrary response prose.
func boundedResponseModel(value string) string {
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:/-", c)) {
			return ""
		}
	}
	return value
}
func responseModelFromEnvelope(data string) string {
	var envelope struct {
		Model    string `json:"model"`
		Response *struct {
			Model string `json:"model"`
		} `json:"response"`
	}
	if json.Unmarshal([]byte(data), &envelope) != nil {
		return ""
	}
	if envelope.Response != nil {
		return boundedResponseModel(envelope.Response.Model)
	}
	return boundedResponseModel(envelope.Model)
}

// Presence is read from the native payload before normalized integer defaults.
// A missing or null usage counter is not a provider-reported zero.
func setProviderUsagePresenceV1(usage *Usage, body, inputKey, outputKey string, path ...string) {
	var raw json.RawMessage = []byte(body)
	for _, key := range path {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return
		}
		raw = fields[key]
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	known := func(key string) bool {
		value, exists := fields[key]
		if !exists || string(value) == "null" {
			return false
		}
		var count int
		return json.Unmarshal(value, &count) == nil && count >= 0
	}
	usage.UsagePresenceKnown = true
	usage.HasPromptTokens = known(inputKey)
	usage.HasCompletionTokens = known(outputKey)
}

// This numeric observer reads native nonempty content only. Retry lifecycle
// flags deliberately include control frames and cannot establish token timing.
func nativeFirstContentObserverV1(endpoint string, observers []func(time.Time)) func(string, time.Time) {
	observed := false
	return func(body string, at time.Time) {
		if observed || len(observers) == 0 || !nativePayloadHasContentV1(endpoint, body) {
			return
		}
		observed = true
		for _, observer := range observers {
			if observer != nil {
				func() { defer func() { _ = recover() }(); observer(at) }()
			}
		}
	}
}

func nativePayloadHasContentV1(endpoint, body string) bool {
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &payload) != nil {
		return false
	}
	text := func(fields map[string]json.RawMessage, key string) string {
		var value string
		_ = json.Unmarshal(fields[key], &value)
		return value
	}
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		return fields
	}
	switch endpoint {
	case "responses":
		switch text(payload, "type") {
		case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.function_call_arguments.delta":
			return text(payload, "delta") != ""
		case "response.output_item.added", "response.output_item.done":
			item := object(payload["item"])
			return text(item, "type") == "function_call" && (text(item, "name") != "" || strings.TrimSpace(text(item, "arguments")) != "")
		}
	case "messages":
		switch text(payload, "type") {
		case "content_block_start":
			block := object(payload["content_block"])
			if text(block, "type") == "tool_use" {
				return text(block, "name") != "" || len(object(block["input"])) > 0
			}
			return (text(block, "type") == "text" && text(block, "text") != "") || (text(block, "type") == "thinking" && text(block, "thinking") != "")
		case "content_block_delta":
			delta := object(payload["delta"])
			switch text(delta, "type") {
			case "text_delta":
				return text(delta, "text") != ""
			case "thinking_delta":
				return text(delta, "thinking") != ""
			case "input_json_delta":
				return text(delta, "partial_json") != ""
			}
		}
	default:
		var choices []map[string]json.RawMessage
		if json.Unmarshal(payload["choices"], &choices) != nil {
			return false
		}
		for _, choice := range choices {
			delta := object(choice["delta"])
			if text(delta, "content") != "" || text(delta, "reasoning_content") != "" {
				return true
			}
			var calls []map[string]json.RawMessage
			_ = json.Unmarshal(delta["tool_calls"], &calls)
			for _, call := range calls {
				function := object(call["function"])
				if text(function, "name") != "" || text(function, "arguments") != "" {
					return true
				}
			}
		}
	}
	return false
}
