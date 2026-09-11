package reasoningmarkup

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	maxProviderJSONCandidateBytes = 1 << 20
	maxProviderJSONAggregateBytes = 8 << 20
	maxProviderJSONDepth          = 64
	maxProviderJSONTokens         = 100_000
	maxProviderJSONStringBytes    = 1 << 20
	maxProviderJSONCandidates     = 32
	maxProviderNestedStrings      = 16
	maxProviderInspectionNodes    = 100_000
)

var ErrPrivateContent = errors.New("provider output contains private reasoning content")

var providerJSONOptions = domainjsonstrict.Options{
	MaxBytes:       maxProviderJSONCandidateBytes,
	MaxDepth:       maxProviderJSONDepth,
	MaxTokens:      maxProviderJSONTokens,
	MaxStringBytes: maxProviderJSONStringBytes,
}

// ValidateProviderNativeTextV1 rejects provider-native reasoning objects and
// fields after markup removal but before any callback, event, history, report,
// or child-run consumer can observe the text. JSON is decoded with the shared
// strict bounded parser; nested JSON strings use the same aggregate budget.
func ValidateProviderNativeTextV1(value string) error {
	if len(value) > MaxInputBytes {
		return ErrLimit
	}
	inspector := providerNativeInspector{remainingNodes: maxProviderInspectionNodes}
	return inspector.inspectText(value, 0)
}

// IsPrivateReasoningDiscriminatorV1 is shared by structured event sanitation.
// Typed public metadata such as reasoningTokens and reasoningEffort is not a
// private field and remains governed by its existing exact-value contract.
func IsPrivateReasoningDiscriminatorV1(key string, value any) bool {
	compactKey := compactProviderNativeName(key)
	switch compactKey {
	case "reasoning", "thinking", "reasoningcontent", "thinkingcontent",
		"assistantreasoning", "assistantthinking", "redactedthinking",
		"reasoningsummary", "reasoningtext", "reasoningsignature", "thinkingsignature",
		"thought", "thoughtcontent", "thoughtsignature":
		return true
	}
	if compactKey == "channel" {
		text, ok := value.(string)
		return ok && compactProviderNativeName(text) == "analysis"
	}
	if compactKey != "type" && compactKey != "kind" {
		return false
	}
	text, ok := value.(string)
	return ok && privateProviderNativeKind(text)
}

type providerNativeInspector struct {
	candidates     int
	aggregateBytes int
	remainingNodes int
}

func (inspector *providerNativeInspector) inspectText(value string, nestedDepth int) error {
	if nestedDepth > maxProviderNestedStrings {
		return ErrLimit
	}
	if containsProviderNativeFieldAssignment(value) {
		return ErrPrivateContent
	}
	trimmed := strings.TrimSpace(value)
	wholeInspected := false
	if looksLikeProviderJSON(trimmed) {
		decoded, err := inspector.decodeCandidate(trimmed)
		if err == nil {
			wholeInspected = true
			if err := inspector.inspectValue(decoded, nestedDepth); err != nil {
				return err
			}
		} else if closedErr := closedProviderCandidateError(trimmed, err); closedErr != nil {
			return closedErr
		}
	}
	candidates, overflow := balancedProviderJSONCandidates(value, maxProviderJSONCandidates)
	if overflow {
		return ErrLimit
	}
	for _, candidate := range candidates {
		if wholeInspected && strings.TrimSpace(candidate) == trimmed {
			continue
		}
		decoded, err := inspector.decodeCandidate(candidate)
		if err != nil {
			if closedErr := closedProviderCandidateError(candidate, err); closedErr != nil {
				return closedErr
			}
			continue
		}
		if err := inspector.inspectValue(decoded, nestedDepth); err != nil {
			return err
		}
	}
	return nil
}

func closedProviderCandidateError(candidate string, err error) error {
	if errors.Is(err, ErrLimit) {
		return err
	}
	if json.Valid([]byte(candidate)) {
		// encoding/json accepts duplicate keys and structures that exceed our
		// stricter authority limits. A provider-shaped JSON value that cannot
		// be inspected unambiguously is not publishable public text.
		return errors.Join(ErrLimit, err)
	}
	if containsProviderNativeFieldAssignment(candidate) {
		return errors.Join(ErrPrivateContent, err)
	}
	return nil
}

func (inspector *providerNativeInspector) decodeCandidate(candidate string) (any, error) {
	inspector.candidates++
	inspector.aggregateBytes += len(candidate)
	if inspector.candidates > maxProviderJSONCandidates ||
		len(candidate) > maxProviderJSONCandidateBytes ||
		inspector.aggregateBytes > maxProviderJSONAggregateBytes {
		return nil, ErrLimit
	}
	return domainjsonstrict.DecodeValue([]byte(candidate), providerJSONOptions)
}

func (inspector *providerNativeInspector) inspectValue(value any, nestedDepth int) error {
	inspector.remainingNodes--
	if inspector.remainingNodes < 0 {
		return ErrLimit
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if IsPrivateReasoningDiscriminatorV1(key, child) {
				return ErrPrivateContent
			}
			if err := inspector.inspectValue(child, nestedDepth); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := inspector.inspectValue(child, nestedDepth); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > maxProviderJSONStringBytes {
			return ErrLimit
		}
		return inspector.inspectText(typed, nestedDepth+1)
	}
	return nil
}

func looksLikeProviderJSON(value string) bool {
	if value == "" {
		return false
	}
	return value[0] == '{' || value[0] == '[' || value[0] == '"'
}

func privateProviderNativeKind(value string) bool {
	normalized := compactProviderNativeName(value)
	if strings.HasPrefix(normalized, "responsereasoning") {
		return true
	}
	switch normalized {
	case "assistantreasoning", "assistantreasoningdelta", "agentreasoning",
		"reasoning", "thinking", "redactedthinking", "thinkingdelta", "signaturedelta", "analysis":
		return true
	default:
		return false
	}
}

func compactProviderNativeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var compact strings.Builder
	compact.Grow(len(value))
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			compact.WriteRune(character)
		}
	}
	return compact.String()
}

func containsProviderNativeFieldAssignment(value string) bool {
	for cursor := 0; cursor < len(value); {
		start, ok := providerFieldBoundary(value, cursor)
		if ok {
			key, afterKey, found := readProviderFieldKey(value, start)
			if found {
				afterKey = skipProviderSpaces(value, afterKey)
				if afterKey < len(value) && (value[afterKey] == ':' || value[afterKey] == '=') {
					valueStart := skipProviderSpaces(value, afterKey+1)
					if IsPrivateReasoningDiscriminatorV1(key, nil) {
						return true
					}
					compactKey := compactProviderNativeName(key)
					if compactKey == "type" || compactKey == "kind" {
						fieldValue, _ := readProviderAssignmentValue(value, valueStart)
						if privateProviderNativeKind(fieldValue) {
							return true
						}
					}
					cursor = afterKey + 1
					continue
				}
			}
		}
		if value[cursor] == '"' || value[cursor] == '\'' {
			cursor = skipProviderQuoted(value, cursor)
			continue
		}
		cursor++
	}
	return false
}

func providerFieldBoundary(value string, cursor int) (int, bool) {
	if cursor > 0 {
		switch value[cursor-1] {
		case '{', '[', ',', ';', '|', '\n', '\r':
		default:
			return 0, false
		}
	}
	start := cursor
	if cursor > 0 {
		start = skipProviderSpaces(value, cursor)
	} else {
		start = skipProviderSpaces(value, 0)
	}
	return start, start < len(value)
}

func readProviderFieldKey(value string, start int) (string, int, bool) {
	if start >= len(value) {
		return "", start, false
	}
	if value[start] == '"' || value[start] == '\'' {
		end := skipProviderQuoted(value, start)
		if end <= start+1 || end > len(value) || value[end-1] != value[start] {
			return "", start, false
		}
		return decodeProviderQuotedValue(value[start:end]), end, true
	}
	end := start
	for end < len(value) && providerFieldKeyByte(value[end]) {
		end++
	}
	if end == start {
		return "", start, false
	}
	return value[start:end], end, true
}

func readProviderAssignmentValue(value string, start int) (string, int) {
	if start >= len(value) {
		return "", start
	}
	if value[start] == '"' || value[start] == '\'' {
		end := skipProviderQuoted(value, start)
		if end > start+1 && end <= len(value) && value[end-1] == value[start] {
			return decodeProviderQuotedValue(value[start:end]), end
		}
		return "", end
	}
	end := start
	for end < len(value) && !strings.ContainsRune(" \t\r\n,;|}]", rune(value[end])) {
		end++
	}
	return value[start:end], end
}

func decodeProviderQuotedValue(value string) string {
	if len(value) < 2 {
		return value
	}
	if value[0] == '"' {
		var decoded string
		if json.Unmarshal([]byte(value), &decoded) == nil {
			return decoded
		}
	}
	inner := value[1 : len(value)-1]
	inner = strings.ReplaceAll(inner, `\'`, `'`)
	return decodeProviderIdentifierUnicodeEscapes(inner)
}

func decodeProviderIdentifierUnicodeEscapes(value string) string {
	var out strings.Builder
	for cursor := 0; cursor < len(value); {
		if cursor+6 <= len(value) && value[cursor] == '\\' && value[cursor+1] == 'u' {
			hex := value[cursor+2 : cursor+6]
			if strings.IndexFunc(hex, func(r rune) bool {
				return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F')
			}) == -1 {
				decoded, _ := strconv.ParseUint(hex, 16, 16)
				// WriteRune preserves JSON's replacement-rune behavior for lone
				// UTF-16 surrogates, without constructing JSON from provider text.
				out.WriteRune(rune(decoded))
				cursor += 6
				continue
			}
		}
		out.WriteByte(value[cursor])
		cursor++
	}
	return out.String()
}

func providerFieldKeyByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '-' || value == '.'
}

func skipProviderSpaces(value string, cursor int) int {
	for cursor < len(value) {
		switch value[cursor] {
		case ' ', '\t', '\r', '\n':
			cursor++
		default:
			return cursor
		}
	}
	return cursor
}

func skipProviderQuoted(value string, start int) int {
	quote := value[start]
	for cursor := start + 1; cursor < len(value); cursor++ {
		if value[cursor] == '\\' {
			cursor++
			continue
		}
		if value[cursor] == quote {
			return cursor + 1
		}
	}
	return len(value)
}

func balancedProviderJSONCandidates(value string, maxCandidates int) ([]string, bool) {
	result := []string{}
	stack := make([]byte, 0, 8)
	start := -1
	inString := false
	escaped := false
	for cursor := 0; cursor < len(value); cursor++ {
		character := value[cursor]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			continue
		}
		switch character {
		case '{', '[':
			if len(stack) == 0 {
				start = cursor
			}
			stack = append(stack, character)
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			opening := stack[len(stack)-1]
			if opening == '{' && character != '}' || opening == '[' && character != ']' {
				stack = stack[:0]
				start = -1
				continue
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 && start >= 0 {
				if len(result) >= maxCandidates {
					return result, true
				}
				result = append(result, value[start:cursor+1])
				start = -1
			}
		}
	}
	return result, false
}
