package job

import (
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	ErrPersistedModelExecutionInvalid = errors.New("job model execution metadata is invalid")
	ErrPersistedUsageInvalid          = errors.New("job usage metadata is invalid")
)

var sha256LowerV1 = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ProjectPersistedModelExecutionV1 rebuilds the only model-selection metadata
// allowed in a durable job. Provider request bodies, URLs, credentials,
// reasoning bytes, and unknown compatibility fields are never copied.
func ProjectPersistedModelExecutionV1(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	providerID, providerOK := closedJobMetadataStringV1(input["providerId"], 256)
	modelID, modelOK := closedJobMetadataStringV1(input["modelId"], 512)
	source, sourceOK := exactJobStringV1(input["source"])
	resolvedAt, resolvedOK := exactJobStringV1(input["resolvedAt"])
	capabilityFingerprint, capabilityOK := exactSHA256V1(input["capabilityFingerprint"])
	if !providerOK || !modelOK || !sourceOK || !validModelExecutionSourceV1(source) ||
		!resolvedOK || !validRFC3339NanoV1(resolvedAt) || !capabilityOK {
		return nil
	}
	out := map[string]any{
		"providerId":            providerID,
		"modelId":               modelID,
		"source":                source,
		"resolvedAt":            resolvedAt,
		"capabilityFingerprint": capabilityFingerprint,
	}
	for _, key := range []string{"variant", "endpointFormat"} {
		if _, present := input[key]; !present {
			continue
		}
		value, ok := closedJobMetadataStringV1(input[key], 256)
		if !ok || (key == "endpointFormat" && !validEndpointFormatV1(value)) {
			return nil
		}
		out[key] = value
	}
	baseFingerprint, hasBase := optionalSHA256V1(input, "baseUrlFingerprint")
	customFingerprint, hasCustom := optionalSHA256V1(input, "customFullEndpointFingerprint")
	if hasBase && hasCustom {
		return nil
	}
	if _, supplied := input["baseUrlFingerprint"]; supplied && !hasBase {
		return nil
	}
	if _, supplied := input["customFullEndpointFingerprint"]; supplied && !hasCustom {
		return nil
	}
	if hasBase {
		out["baseUrlFingerprint"] = baseFingerprint
	}
	if hasCustom {
		out["customFullEndpointFingerprint"] = customFingerprint
	}
	return out
}

func ValidatePersistedModelExecutionV1(input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	if projected := ProjectPersistedModelExecutionV1(input); projected == nil || !reflect.DeepEqual(input, projected) {
		return ErrPersistedModelExecutionInvalid
	}
	return nil
}

// ProjectPersistedUsageV1 keeps only process diagnostics for background shell
// jobs. Model token/cache usage is authoritative in accepted usage events and
// the usage index, not in a second mutable job map.
func ProjectPersistedUsageV1(kind string, input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	switch strings.TrimSpace(kind) {
	case "background-shell", "bash":
	default:
		return nil
	}
	exitCode, exitOK := boundedIntegerV1(input["exitCode"], -1, int64(1<<31-1))
	duration, durationOK := boundedIntegerV1(input["durationMs"], 0, maxSafeJSONIntegerV1)
	outputBytes, outputOK := boundedIntegerV1(input["outputBytes"], 0, maxSafeJSONIntegerV1)
	maxOutputBytes, maxOutputOK := boundedIntegerV1(input["maxOutputBytes"], 1, maxSafeJSONIntegerV1)
	truncated, truncatedOK := input["truncated"].(bool)
	if !exitOK || !durationOK || !outputOK || !maxOutputOK || !truncatedOK {
		return nil
	}
	out := map[string]any{
		"exitCode":       exitCode,
		"durationMs":     duration,
		"outputBytes":    outputBytes,
		"maxOutputBytes": maxOutputBytes,
		"truncated":      truncated,
	}
	if raw, present := input["timedOut"]; present {
		timedOut, ok := raw.(bool)
		if !ok {
			return nil
		}
		out["timedOut"] = timedOut
	}
	return out
}

func ValidatePersistedUsageV1(kind string, input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	if projected := ProjectPersistedUsageV1(kind, input); projected == nil || !reflect.DeepEqual(input, projected) {
		return ErrPersistedUsageInvalid
	}
	return nil
}

const maxSafeJSONIntegerV1 = int64(1<<53 - 1)

func boundedIntegerV1(value any, min int64, max int64) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		if typed < min || typed > max {
			return 0, false
		}
		number = float64(typed)
	case uint:
		if uint64(typed) > uint64(max) {
			return 0, false
		}
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		if typed > uint64(max) {
			return 0, false
		}
		number = float64(typed)
	case float32:
		number = float64(typed)
	case float64:
		number = typed
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number ||
		number < float64(min) || number > float64(max) {
		return 0, false
	}
	return number, true
}

func closedJobMetadataStringV1(value any, maxBytes int) (string, bool) {
	text, ok := exactJobStringV1(value)
	if !ok || len(text) > maxBytes || ProjectPersistableUntrustedOutputV1(text) != text {
		return "", false
	}
	return text, true
}

func exactJobStringV1(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok && text != "" && text == strings.TrimSpace(text)
}

func exactSHA256V1(value any) (string, bool) {
	text, ok := exactJobStringV1(value)
	return text, ok && sha256LowerV1.MatchString(text)
}

func optionalSHA256V1(input map[string]any, key string) (string, bool) {
	value, present := input[key]
	if !present {
		return "", false
	}
	return exactSHA256V1(value)
}

func validModelExecutionSourceV1(value string) bool {
	switch value {
	case "explicit-input", "subagent-profile", "session", "thread", "runtime-default":
		return true
	default:
		return false
	}
}

func validEndpointFormatV1(value string) bool {
	switch value {
	case "chat_completions", "responses", "messages", "custom_endpoint":
		return true
	default:
		return false
	}
}

func validRFC3339NanoV1(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
