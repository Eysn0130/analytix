package cachetelemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	CacheVisibleShapeV1SchemaVersion              = "cache-visible-shape.v1"
	ProviderCallObservationV1SchemaVersion        = "provider-call-observation.v1"
	maxProviderCallAttempt                 uint32 = 1_000_000
)

type ProviderFamilyV1 string

const (
	ProviderFamilyDeepSeek            ProviderFamilyV1 = "deepseek"
	ProviderFamilyOpenAICompatible    ProviderFamilyV1 = "openai_compatible"
	ProviderFamilyAnthropicCompatible ProviderFamilyV1 = "anthropic_compatible"
	ProviderFamilyCustomEndpoint      ProviderFamilyV1 = "custom_endpoint"
)

type EndpointFormatV1 string

const (
	EndpointFormatChatCompletions EndpointFormatV1 = "chat_completions"
	EndpointFormatResponses       EndpointFormatV1 = "responses"
	EndpointFormatMessages        EndpointFormatV1 = "messages"
	EndpointFormatCustomEndpoint  EndpointFormatV1 = "custom_endpoint"
)

type ProviderCallStatusV1 string

const (
	ProviderCallStatusSucceeded          ProviderCallStatusV1 = "succeeded"
	ProviderCallStatusFailed             ProviderCallStatusV1 = "failed"
	ProviderCallStatusCancelled          ProviderCallStatusV1 = "cancelled"
	ProviderCallStatusTimedOut           ProviderCallStatusV1 = "timed_out"
	ProviderCallStatusStreamAborted      ProviderCallStatusV1 = "stream_aborted"
	ProviderCallStatusRestartInterrupted ProviderCallStatusV1 = "restart_interrupted"
)

// CacheVisibleShapeV1 identifies one exact serialized provider attempt without
// retaining the endpoint URL or request body. EndpointHMAC binds the effective
// endpoint, including custom full endpoints, without persisting its secrets.
type CacheVisibleShapeV1 struct {
	SchemaVersion       string           `json:"schemaVersion"`
	LogicalCallHMAC     string           `json:"logicalCallHmac"`
	Attempt             uint32           `json:"attempt"`
	ProviderFamily      ProviderFamilyV1 `json:"providerFamily"`
	ModelHMAC           string           `json:"modelHmac"`
	Endpoint            EndpointFormatV1 `json:"endpointFormat"`
	EndpointHMAC        string           `json:"endpointHmac"`
	WireBodyHMAC        string           `json:"wireBodyHmac"`
	CredentialScopeHMAC string           `json:"credentialScopeHmac"`
	ProviderConfigHMAC  string           `json:"providerConfigHmac"`
	DigestEpoch         uint64           `json:"digestEpoch"`
	StartedAt           string           `json:"startedAt"`
}

// TokenCountV1 distinguishes an unavailable provider counter from a provider
// counter explicitly reported as zero. Unknown counters must carry value zero.
type TokenCountV1 struct {
	Known bool   `json:"known"`
	Value uint64 `json:"value"`
}

type ProviderUsageV1 struct {
	InputTokens     TokenCountV1 `json:"inputTokens"`
	OutputTokens    TokenCountV1 `json:"outputTokens"`
	CacheHitTokens  TokenCountV1 `json:"cacheHitTokens"`
	CacheMissTokens TokenCountV1 `json:"cacheMissTokens"`
	ReasoningTokens TokenCountV1 `json:"reasoningTokens"`
}

// ProviderCallObservationV1 is a terminal settlement for one registered
// CacheVisibleShapeV1. It contains numeric usage only; provider text,
// reasoning, request bytes, PII, credentials, and raw endpoint URLs have no
// representation in this contract.
type ProviderCallObservationV1 struct {
	SchemaVersion string               `json:"schemaVersion"`
	Shape         CacheVisibleShapeV1  `json:"shape"`
	Status        ProviderCallStatusV1 `json:"status"`
	Usage         ProviderUsageV1      `json:"usage"`
	SettledAt     string               `json:"settledAt"`
}

func (shape CacheVisibleShapeV1) Validate() error {
	if shape.SchemaVersion != CacheVisibleShapeV1SchemaVersion {
		return errors.New("cache visible shape schema version is invalid")
	}
	if !isLowerHexDigest(shape.LogicalCallHMAC) {
		return errors.New("cache visible shape logical call HMAC is invalid")
	}
	if shape.Attempt == 0 || shape.Attempt > maxProviderCallAttempt {
		return errors.New("cache visible shape attempt is invalid")
	}
	if !shape.ProviderFamily.valid() {
		return errors.New("cache visible shape provider family is invalid")
	}
	if !isLowerHexDigest(shape.ModelHMAC) {
		return errors.New("cache visible shape model HMAC is invalid")
	}
	if !shape.Endpoint.valid() {
		return errors.New("cache visible shape endpoint format is invalid")
	}
	if !isLowerHexDigest(shape.EndpointHMAC) {
		return errors.New("cache visible shape endpoint HMAC is invalid")
	}
	if !isLowerHexDigest(shape.WireBodyHMAC) {
		return errors.New("cache visible shape wire body HMAC is invalid")
	}
	if !isLowerHexDigest(shape.CredentialScopeHMAC) {
		return errors.New("cache visible shape credential scope HMAC is invalid")
	}
	if !isLowerHexDigest(shape.ProviderConfigHMAC) {
		return errors.New("cache visible shape provider config HMAC is invalid")
	}
	if shape.DigestEpoch == 0 {
		return errors.New("cache visible shape digest epoch is invalid")
	}
	if _, err := parseCanonicalTimestamp(shape.StartedAt); err != nil {
		return fmt.Errorf("cache visible shape started time is invalid: %w", err)
	}
	return nil
}

func (count TokenCountV1) Validate() error {
	if !count.Known && count.Value != 0 {
		return errors.New("unknown token count must have value zero")
	}
	return nil
}

func (usage ProviderUsageV1) Validate() error {
	counts := []struct {
		name  string
		value TokenCountV1
	}{
		{name: "inputTokens", value: usage.InputTokens},
		{name: "outputTokens", value: usage.OutputTokens},
		{name: "cacheHitTokens", value: usage.CacheHitTokens},
		{name: "cacheMissTokens", value: usage.CacheMissTokens},
		{name: "reasoningTokens", value: usage.ReasoningTokens},
	}
	for _, count := range counts {
		if err := count.value.Validate(); err != nil {
			return fmt.Errorf("provider usage %s is invalid: %w", count.name, err)
		}
	}
	return nil
}

// ValidateForProviderFamily prevents contradictory provider-native usage from
// entering the signed attempt ledger. Missing counters remain explicitly
// unknown; known cache counters must be complete and internally consistent.
func (usage ProviderUsageV1) ValidateForProviderFamily(family ProviderFamilyV1) error {
	if err := usage.Validate(); err != nil {
		return err
	}
	cacheKnown := usage.CacheHitTokens.Known || usage.CacheMissTokens.Known
	if !cacheKnown {
		return nil
	}
	if !usage.InputTokens.Known || !usage.CacheHitTokens.Known || !usage.CacheMissTokens.Known {
		return errors.New("provider cache usage counters are incomplete")
	}
	if usage.CacheHitTokens.Value > usage.InputTokens.Value || usage.CacheMissTokens.Value > usage.InputTokens.Value ||
		usage.CacheHitTokens.Value > ^uint64(0)-usage.CacheMissTokens.Value {
		return errors.New("provider cache usage counters exceed input tokens")
	}
	covered := usage.CacheHitTokens.Value + usage.CacheMissTokens.Value
	if covered > usage.InputTokens.Value {
		return errors.New("provider cache usage counters exceed input tokens")
	}
	if family == ProviderFamilyDeepSeek && covered != usage.InputTokens.Value {
		return errors.New("DeepSeek cache usage does not cover input tokens")
	}
	return nil
}

func (observation ProviderCallObservationV1) Validate() error {
	if observation.SchemaVersion != ProviderCallObservationV1SchemaVersion {
		return errors.New("provider call observation schema version is invalid")
	}
	if err := observation.Shape.Validate(); err != nil {
		return fmt.Errorf("provider call observation shape is invalid: %w", err)
	}
	if !observation.Status.valid() {
		return errors.New("provider call observation status is invalid")
	}
	if err := observation.Usage.Validate(); err != nil {
		return err
	}
	if err := observation.Usage.ValidateForProviderFamily(observation.Shape.ProviderFamily); err != nil {
		return err
	}
	startedAt, _ := parseCanonicalTimestamp(observation.Shape.StartedAt)
	settledAt, err := parseCanonicalTimestamp(observation.SettledAt)
	if err != nil {
		return fmt.Errorf("provider call observation settled time is invalid: %w", err)
	}
	if settledAt.Before(startedAt) {
		return errors.New("provider call observation settles before it starts")
	}
	return nil
}

func DecodeCacheVisibleShapeV1(body []byte) (CacheVisibleShapeV1, error) {
	var shape CacheVisibleShapeV1
	if err := json.Unmarshal(body, &shape); err != nil {
		return CacheVisibleShapeV1{}, err
	}
	return shape, nil
}

func DecodeProviderCallObservationV1(body []byte) (ProviderCallObservationV1, error) {
	var observation ProviderCallObservationV1
	if err := json.Unmarshal(body, &observation); err != nil {
		return ProviderCallObservationV1{}, err
	}
	return observation, nil
}

func (shape *CacheVisibleShapeV1) UnmarshalJSON(body []byte) error {
	type wireShape CacheVisibleShapeV1
	var decoded wireShape
	if err := decodeStrictObject(body, &decoded, []string{
		"schemaVersion", "logicalCallHmac", "attempt", "providerFamily", "modelHmac",
		"endpointFormat", "endpointHmac", "wireBodyHmac", "credentialScopeHmac", "providerConfigHmac",
		"digestEpoch", "startedAt",
	}); err != nil {
		return fmt.Errorf("decode cache visible shape: %w", err)
	}
	candidate := CacheVisibleShapeV1(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*shape = candidate
	return nil
}

func (count *TokenCountV1) UnmarshalJSON(body []byte) error {
	type wireCount TokenCountV1
	var decoded wireCount
	if err := decodeStrictObject(body, &decoded, []string{"known", "value"}); err != nil {
		return fmt.Errorf("decode token count: %w", err)
	}
	candidate := TokenCountV1(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*count = candidate
	return nil
}

func (usage *ProviderUsageV1) UnmarshalJSON(body []byte) error {
	type wireUsage ProviderUsageV1
	var decoded wireUsage
	if err := decodeStrictObject(body, &decoded, []string{
		"inputTokens", "outputTokens", "cacheHitTokens", "cacheMissTokens", "reasoningTokens",
	}); err != nil {
		return fmt.Errorf("decode provider usage: %w", err)
	}
	candidate := ProviderUsageV1(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*usage = candidate
	return nil
}

func (observation *ProviderCallObservationV1) UnmarshalJSON(body []byte) error {
	type wireObservation ProviderCallObservationV1
	var decoded wireObservation
	if err := decodeStrictObject(body, &decoded, []string{
		"schemaVersion", "shape", "status", "usage", "settledAt",
	}); err != nil {
		return fmt.Errorf("decode provider call observation: %w", err)
	}
	candidate := ProviderCallObservationV1(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*observation = candidate
	return nil
}

func (format EndpointFormatV1) valid() bool {
	switch format {
	case EndpointFormatChatCompletions, EndpointFormatResponses, EndpointFormatMessages, EndpointFormatCustomEndpoint:
		return true
	default:
		return false
	}
}

func (family ProviderFamilyV1) valid() bool {
	switch family {
	case ProviderFamilyDeepSeek, ProviderFamilyOpenAICompatible, ProviderFamilyAnthropicCompatible, ProviderFamilyCustomEndpoint:
		return true
	default:
		return false
	}
}

func (status ProviderCallStatusV1) valid() bool {
	switch status {
	case ProviderCallStatusSucceeded, ProviderCallStatusFailed, ProviderCallStatusCancelled,
		ProviderCallStatusTimedOut, ProviderCallStatusStreamAborted, ProviderCallStatusRestartInterrupted:
		return true
	default:
		return false
	}
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func parseCanonicalTimestamp(value string) (time.Time, error) {
	if value == "" || !strings.HasSuffix(value, "Z") {
		return time.Time{}, errors.New("timestamp must be UTC RFC3339Nano")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Format(time.RFC3339Nano) != value {
		return time.Time{}, errors.New("timestamp must use canonical RFC3339Nano encoding")
	}
	return parsed, nil
}

func decodeStrictObject(body []byte, target any, required []string) error {
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return errors.New("value must be a JSON object")
	}
	for _, name := range required {
		raw, ok := fields[name]
		if !ok {
			return fmt.Errorf("required property %q is missing", name)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("required property %q must not be null", name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("value contains trailing JSON")
	}
	return nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := scanJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("value contains trailing JSON")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return keyErr
			}
			key, keyOK := keyToken.(string)
			if !keyOK {
				return errors.New("JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate property %q at %s", key, path)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, path+"."+key); err != nil {
				return err
			}
		}
		end, endErr := decoder.Token()
		if endErr != nil || end != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
		end, endErr := decoder.Token()
		if endErr != nil || end != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return errors.New("JSON value has an invalid delimiter")
	}
	return nil
}
