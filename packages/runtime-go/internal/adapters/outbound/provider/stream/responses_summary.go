package stream

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const responsesSummaryPartLimitV1 = 128
const responsesSummaryByteLimitV1 = 1 << 20

type responsesSummaryPartV1 struct {
	ordinal                              uint64
	text                                 string
	textDone, done, incomplete, poisoned bool
}

type responsesSummaryItemV1 struct {
	id                            string
	declared, open, poisoned, raw bool
	parts                         map[int]*responsesSummaryPartV1
	lastSequence                  uint64
	hasSequence                   bool
}

// Summary text is a separate, callback-only source. Native item/part identities
// stay in this physical-stream tracker; host ordinals never claim to be native
// output indexes. Opaque reasoning bytes are neither inspected nor copied.
type responsesSummaryTrackerV1 struct {
	items   map[int]*responsesSummaryItemV1
	seenIDs map[string]bool
	ordinal uint64
	bytes   int
	closed  bool
}

func newResponsesSummaryTrackerV1() *responsesSummaryTrackerV1 {
	return &responsesSummaryTrackerV1{items: map[int]*responsesSummaryItemV1{}, seenIDs: map[string]bool{}}
}

func (s *responsesSummaryTrackerV1) poison(index *int, id string) {
	// A rejected native identity cannot reappear at another output index.
	if id != "" && len(id) <= 256 && len(s.seenIDs) < 256 {
		s.seenIDs[id] = true
	}
	for key, item := range s.items {
		if index != nil && key == *index || id != "" && item.id == id || index == nil && id == "" {
			item.open, item.poisoned = false, true
			for _, part := range item.parts {
				s.bytes -= len(part.text)
				part.text = ""
				part.poisoned = true
			}
		}
	}
	if index != nil && *index >= 0 && uint64(*index) <= 1<<53-1 && s.items[*index] == nil && len(s.items) < 256 {
		if len(id) > 256 {
			id = ""
		}
		s.items[*index] = &responsesSummaryItemV1{id: id, declared: true, poisoned: true}
	}
}

func (s *responsesSummaryTrackerV1) begin(index *int, id, kind string, summary json.RawMessage) bool {
	declared := len(summary) != 0
	if s.closed {
		return declared
	}
	if index == nil || *index < 0 || uint64(*index) > 1<<53-1 || id == "" || len(id) > 256 || s.seenIDs[id] || len(s.seenIDs) >= 256 || len(s.items) >= 256 || s.items[*index] != nil {
		s.poison(index, id)
		return declared
	}
	s.seenIDs[id] = true
	_, valid := responsesSummaryPartsV1(summary)
	s.items[*index] = &responsesSummaryItemV1{id: id, declared: declared, open: kind == "reasoning" && declared && valid, parts: map[int]*responsesSummaryPartV1{}}
	return declared
}

func (s *responsesSummaryTrackerV1) declared(index *int, id string) bool {
	return index != nil && s.items[*index] != nil && s.items[*index].id == id && s.items[*index].declared
}

func (s *responsesSummaryTrackerV1) rawSelected(index *int, id string) bool {
	return index != nil && s.items[*index] != nil && s.items[*index].id == id && s.items[*index].raw
}

// A declared but empty summary array is not readable summary evidence. A
// compatible provider may emit the actual raw channel instead. Defer its native
// start until that first real delta, and select only one display form per item.
func (s *responsesSummaryTrackerV1) rawDelta(index *int, id string) (bool, bool) {
	if !s.declared(index, id) {
		return false, true
	}
	item := s.items[*index]
	if s.closed || !item.open || item.poisoned || len(item.parts) != 0 {
		return false, false
	}
	first := !item.raw
	item.raw = true
	return first, true
}

func responsesSummaryPartsV1(raw json.RawMessage) ([]string, bool) {
	var parts []json.RawMessage
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &parts) != nil || len(parts) > responsesSummaryPartLimitV1 {
		return nil, false
	}
	texts := make([]string, 0, len(parts))
	bytes := 0
	for _, rawPart := range parts {
		text, valid := responsesSummaryPartTextV1(rawPart)
		if !valid || len(text) > responsesSummaryByteLimitV1-bytes {
			return nil, false
		}
		bytes += len(text)
		texts = append(texts, text)
	}
	return texts, true
}

func responsesSummaryStringV1(raw json.RawMessage) (string, bool) {
	var text *string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil || text == nil {
		return "", false
	}
	return *text, true
}

func responsesSummaryPartTextV1(raw json.RawMessage) (string, bool) {
	var part map[string]json.RawMessage
	if json.Unmarshal(raw, &part) != nil || part == nil {
		return "", false
	}
	kind, valid := responsesSummaryStringV1(part["type"])
	text, textValid := responsesSummaryStringV1(part["text"])
	return text, valid && textValid && kind == "summary_text"
}

func responsesSummaryIndexV1(raw json.RawMessage) *int {
	var value *int
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil || *value < 0 || uint64(*value) > 1<<53-1 {
		return nil
	}
	return value
}

func responsesSummaryChunkV1(kind string, part *responsesSummaryPartV1, text string) Chunk {
	return Chunk{Kind: ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{
		Source: "provider_reasoning", ReasoningForm: "summary", Kind: kind, HostSegmentOrdinal: part.ordinal, Text: text,
	}}
}

func (s *responsesSummaryTrackerV1) append(part *responsesSummaryPartV1, text string) []Chunk {
	if len(text) > responsesSummaryByteLimitV1-s.bytes {
		s.bytes -= len(part.text)
		part.text, part.poisoned = "", true
		return nil
	}
	part.text += text
	s.bytes += len(text)
	if text == "" {
		return nil
	}
	return []Chunk{responsesSummaryChunkV1("delta", part, text)}
}

func (s *responsesSummaryTrackerV1) snapshot(part *responsesSummaryPartV1, text string) []Chunk {
	if part.poisoned || !strings.HasPrefix(text, part.text) || (part.textDone || part.done) && text != part.text {
		part.poisoned = true
		return nil
	}
	return s.append(part, text[len(part.text):])
}

func (s *responsesSummaryTrackerV1) consume(kind string, fields map[string]json.RawMessage) []Chunk {
	if s.closed {
		return nil
	}
	index := responsesSummaryIndexV1(fields["output_index"])
	id, idValid := responsesSummaryStringV1(fields["item_id"])
	if s.rawSelected(index, id) {
		return nil
	}
	partIndex := responsesSummaryIndexV1(fields["summary_index"])
	sequence := responsesSummaryIndexV1(fields["sequence_number"])
	if index == nil || !idValid || id == "" || partIndex == nil || *partIndex >= responsesSummaryPartLimitV1 || sequence == nil {
		s.poison(index, id)
		return nil
	}
	item := s.items[*index]
	if item == nil || item.id != id || !item.open || item.poisoned {
		s.poison(index, id)
		return nil
	}
	// SSE replay/late deliveries cannot append the same delta twice or reopen a
	// completed part. The sequence is native evidence, not a host-generated ID.
	if item.hasSequence && uint64(*sequence) <= item.lastSequence {
		return nil
	}
	item.lastSequence, item.hasSequence = uint64(*sequence), true
	part := item.parts[*partIndex]
	if kind == "response.reasoning_summary_part.added" {
		text, valid := responsesSummaryPartTextV1(fields["part"])
		if !valid || part != nil || *partIndex != len(item.parts) || s.ordinal >= responsesSummaryPartLimitV1 {
			s.poison(index, id)
			return nil
		}
		s.ordinal++
		part = &responsesSummaryPartV1{ordinal: s.ordinal}
		item.parts[*partIndex] = part
		chunks := []Chunk{responsesSummaryChunkV1("start", part, "")}
		return append(chunks, s.append(part, text)...)
	}
	if part == nil || part.poisoned {
		s.poison(index, id)
		return nil
	}
	switch kind {
	case "response.reasoning_summary_text.delta":
		text, valid := responsesSummaryStringV1(fields["delta"])
		if !valid || part.textDone || part.done {
			part.poisoned = true
			return nil
		}
		return s.append(part, text)
	case "response.reasoning_summary_text.done":
		text, valid := responsesSummaryStringV1(fields["text"])
		if !valid {
			part.poisoned = true
			return nil
		}
		chunks := s.snapshot(part, text)
		part.textDone = true
		return chunks
	case "response.reasoning_summary_part.done":
		text, valid := responsesSummaryPartTextV1(fields["part"])
		status := ""
		if raw, present := fields["status"]; present {
			var statusValid bool
			status, statusValid = responsesSummaryStringV1(raw)
			valid = valid && statusValid && status == "incomplete"
		}
		if !valid {
			part.poisoned = true
			return nil
		}
		chunks := s.snapshot(part, text)
		part.done = true
		part.incomplete = part.incomplete || status == "incomplete"
		return chunks
	}
	return nil
}

func (s *responsesSummaryTrackerV1) finish(index *int, id, kind string, status, summary json.RawMessage) []Chunk {
	if s.closed {
		return nil
	}
	if !s.declared(index, id) {
		s.poison(index, id)
		return nil
	}
	item := s.items[*index]
	if item.raw {
		item.open = false
		return nil
	}
	completionStatus, validStatus := responsesSummaryStringV1(status)
	if len(status) == 0 {
		validStatus = true
	}
	if !item.open || item.poisoned || kind != "reasoning" || !validStatus || completionStatus != "" && completionStatus != "completed" {
		s.poison(index, id)
		return nil
	}
	item.open = false
	texts, valid := responsesSummaryPartsV1(summary)
	if !valid || len(texts) != len(item.parts) {
		s.poison(index, id)
		return nil
	}
	extra := 0
	for partIndex, text := range texts {
		part := item.parts[partIndex]
		if part == nil || part.poisoned || !strings.HasPrefix(text, part.text) || (part.textDone || part.done) && text != part.text {
			s.poison(index, id)
			return nil
		}
		extra += len(text) - len(part.text)
	}
	if extra > responsesSummaryByteLimitV1-s.bytes {
		s.poison(index, id)
		return nil
	}
	chunks := []Chunk{}
	for partIndex, text := range texts {
		part := item.parts[partIndex]
		chunks = append(chunks, s.snapshot(part, text)...)
		if !part.incomplete {
			chunks = append(chunks, responsesSummaryChunkV1("end", part, ""))
		}
		s.bytes -= len(part.text)
		part.text = ""
	}
	return chunks
}

func (s *responsesSummaryTrackerV1) close() {
	s.closed = true
	s.poison(nil, "")
}
