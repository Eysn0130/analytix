package cachetelemetry

import (
	"encoding/json"
	"errors"
)

// ModelInputSegmentsV1 is a bounded, comparable value so old signed ledger
// records retain their exact canonical form when this optional field is absent.
// Digests use the existing installation or process HMAC authority, never SHA of
// a low-entropy prompt. Byte lengths are serialized bytes, not provider tokens.
type InputSegmentV1 struct {
	HMAC  string `json:"hmac"`
	Bytes int    `json:"bytes"`
	Items int    `json:"items"`
}

type ModelInputSegmentsV1 struct {
	Version     string         `json:"version"`
	OrderedHMAC string         `json:"orderedHmac"`
	System      InputSegmentV1 `json:"system"`
	Tools       InputSegmentV1 `json:"tools"`
	History     InputSegmentV1 `json:"history"`
	Current     InputSegmentV1 `json:"current"`
}

// CaptureModelInputSegmentsV1 consumes the final serialized request. It does
// not sort messages, rewrite system snapshots, or include off-model metadata.
// Unsupported request shapes remain unavailable, never an invented cache hit.
func CaptureModelInputSegmentsV1(body []byte, digest func(string, []byte) string) ModelInputSegmentsV1 {
	var wire map[string]json.RawMessage
	if digest == nil || json.Unmarshal(body, &wire) != nil {
		return ModelInputSegmentsV1{}
	}
	var messages []json.RawMessage
	field := "messages"
	if wire[field] == nil {
		field = "input"
	}
	if json.Unmarshal(wire[field], &messages) != nil {
		return ModelInputSegmentsV1{}
	}
	system := []json.RawMessage{}
	for _, key := range []string{"system", "instructions"} {
		if value := wire[key]; len(value) != 0 {
			system = append(system, value)
		}
	}
	history := make([]json.RawMessage, 0, len(messages))
	for _, message := range messages {
		var item struct {
			Role string `json:"role"`
		}
		if json.Unmarshal(message, &item) != nil {
			return ModelInputSegmentsV1{}
		}
		if item.Role == "system" || item.Role == "developer" {
			system = append(system, message)
		} else {
			history = append(history, message)
		}
	}
	current := []json.RawMessage{}
	if len(history) > 0 {
		current = append(current, history[len(history)-1])
		history = history[:len(history)-1]
	}
	var tools []json.RawMessage
	if wire["tools"] != nil && json.Unmarshal(wire["tools"], &tools) != nil {
		return ModelInputSegmentsV1{}
	}
	segment := func(name string, items []json.RawMessage) InputSegmentV1 {
		if items == nil {
			items = []json.RawMessage{}
		}
		encoded, err := json.Marshal(items)
		if err != nil {
			return InputSegmentV1{}
		}
		return InputSegmentV1{HMAC: digest("model-input/"+name, encoded), Bytes: len(encoded), Items: len(items)}
	}
	ordered := map[string]json.RawMessage{}
	for _, key := range []string{"system", "instructions", "tools", "messages", "input"} {
		if wire[key] != nil {
			ordered[key] = wire[key]
		}
	}
	orderedBytes, err := json.Marshal(ordered)
	if err != nil {
		return ModelInputSegmentsV1{}
	}
	return ModelInputSegmentsV1{Version: "wire-input-segments.v1", OrderedHMAC: digest("model-input/ordered", orderedBytes), System: segment("system", system), Tools: segment("tools", tools), History: segment("history", history), Current: segment("current", current)}
}

func (input ModelInputSegmentsV1) Validate() error {
	if input == (ModelInputSegmentsV1{}) {
		return nil
	}
	if input.Version != "wire-input-segments.v1" || !isLowerHexDigest(input.OrderedHMAC) {
		return errors.New("model input segments version is invalid")
	}
	for _, segment := range []InputSegmentV1{input.System, input.Tools, input.History, input.Current} {
		if !isLowerHexDigest(segment.HMAC) || segment.Bytes < 2 || segment.Items < 0 {
			return errors.New("model input segment is invalid")
		}
	}
	return nil
}

// CompareModelInputSegmentsV1 reports segment granularity only. The prefix is
// diagnostic serialization order (system/tools/history/current), not a claim
// about the server's tokenizer, internal ordering, or exact KV invalidation.
func CompareModelInputSegmentsV1(previous, current CacheVisibleShapeV1) (firstDifference string, comparablePrefixBytes int, comparable bool) {
	if previous.Validate() != nil || current.Validate() != nil || previous.ModelInput.Version == "" || current.ModelInput.Version == "" ||
		previous.DigestEpoch != current.DigestEpoch || previous.CredentialScopeHMAC != current.CredentialScopeHMAC ||
		previous.EndpointHMAC != current.EndpointHMAC || previous.ModelHMAC != current.ModelHMAC || previous.ProviderConfigHMAC != current.ProviderConfigHMAC {
		return "unavailable", 0, false
	}
	a, b := previous.ModelInput, current.ModelInput
	for _, pair := range []struct {
		name              string
		previous, current InputSegmentV1
	}{
		{"system", a.System, b.System}, {"tools", a.Tools, b.Tools}, {"history", a.History, b.History}, {"current", a.Current, b.Current},
	} {
		if pair.previous != pair.current {
			return pair.name, comparablePrefixBytes, true
		}
		comparablePrefixBytes += pair.current.Bytes
	}
	if a.OrderedHMAC != b.OrderedHMAC {
		return "ordering", 0, true
	}
	return "none", comparablePrefixBytes, true
}
