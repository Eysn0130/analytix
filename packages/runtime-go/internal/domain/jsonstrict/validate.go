package jsonstrict

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	DefaultMaxDepth       = 256
	DefaultMaxNumberBytes = 4096
	DefaultMaxAbsExponent = 10000
)

type Options struct {
	RequireObject  bool
	MaxBytes       int
	MaxDepth       int
	MaxTokens      int
	MaxStringBytes int
	MaxNumberBytes int
	MaxAbsExponent int
}

// Validate rejects ambiguous JSON before it is decoded into a map. In
// particular, encoding/json otherwise accepts duplicate object keys and keeps
// the last value, which is unsafe at authority, schema, and evidence
// boundaries.
func Validate(data []byte, options Options) error {
	_, err := decodeDocument(data, options, false)
	return err
}

func decodeDocument(data []byte, options Options, construct bool) (any, error) {
	if options.MaxBytes > 0 && len(data) > options.MaxBytes {
		return nil, errors.New("JSON size limit exceeded")
	}
	if !utf8.Valid(data) {
		return nil, errors.New("JSON contains invalid UTF-8")
	}
	if err := validateUnicodeEscapes(data); err != nil {
		return nil, err
	}
	maxDepth := options.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	maxNumberBytes := options.MaxNumberBytes
	if maxNumberBytes <= 0 {
		maxNumberBytes = DefaultMaxNumberBytes
	}
	maxAbsExponent := options.MaxAbsExponent
	if maxAbsExponent <= 0 {
		maxAbsExponent = DefaultMaxAbsExponent
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	state := validationState{
		maxDepth: maxDepth, maxTokens: options.MaxTokens, maxStringBytes: options.MaxStringBytes,
		maxNumberBytes: maxNumberBytes, maxAbsExponent: maxAbsExponent,
	}
	first, err := decodeValue(decoder, 0, &state, construct)
	if err != nil {
		return nil, err
	}
	if options.RequireObject {
		if construct {
			if _, ok := first.(map[string]any); !ok {
				return nil, errors.New("top-level JSON value must be an object")
			}
		} else if first != json.Delim('{') {
			return nil, errors.New("top-level JSON value must be an object")
		}
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		_ = token
		return nil, errors.New("JSON contains trailing content")
	}
	return first, nil
}

func DecodeValue(data []byte, options Options) (any, error) {
	return decodeDocument(data, options, true)
}

func DecodeObject(data []byte, options Options) (map[string]any, error) {
	options.RequireObject = true
	value, err := DecodeValue(data, options)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("top-level JSON value must be an object")
	}
	return object, nil
}

func DecodeRawObject(data []byte, options Options) (map[string]json.RawMessage, error) {
	options.RequireObject = true
	if err := Validate(data, options); err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	return object, nil
}

type validationState struct {
	maxDepth       int
	maxTokens      int
	maxStringBytes int
	maxNumberBytes int
	maxAbsExponent int
	tokens         int
}

func decodeValue(decoder *json.Decoder, depth int, state *validationState, construct bool) (any, error) {
	if depth > state.maxDepth {
		return nil, errors.New("JSON nesting limit exceeded")
	}
	token, err := nextToken(decoder, state)
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		var object map[string]any
		if construct {
			object = map[string]any{}
		}
		for decoder.More() {
			keyToken, err := nextToken(decoder, state)
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return nil, errors.New("duplicate JSON object key")
			}
			seen[key] = struct{}{}
			child, err := decodeValue(decoder, depth+1, state, construct)
			if err != nil {
				return nil, err
			}
			if construct {
				object[key] = child
			}
		}
		closing, err := nextToken(decoder, state)
		if err != nil {
			return nil, err
		}
		if closing != json.Delim('}') {
			return nil, errors.New("JSON object is not closed")
		}
		if construct {
			return object, nil
		}
	case '[':
		var array []any
		if construct {
			array = []any{}
		}
		for decoder.More() {
			child, err := decodeValue(decoder, depth+1, state, construct)
			if err != nil {
				return nil, err
			}
			if construct {
				array = append(array, child)
			}
		}
		closing, err := nextToken(decoder, state)
		if err != nil {
			return nil, err
		}
		if closing != json.Delim(']') {
			return nil, errors.New("JSON array is not closed")
		}
		if construct {
			return array, nil
		}
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
	return delimiter, nil
}

func nextToken(decoder *json.Decoder, state *validationState) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	state.tokens++
	if state.maxTokens > 0 && state.tokens > state.maxTokens {
		return nil, errors.New("JSON token limit exceeded")
	}
	if text, ok := token.(string); ok && state.maxStringBytes > 0 && len(text) > state.maxStringBytes {
		return nil, errors.New("JSON string limit exceeded")
	}
	if number, ok := token.(json.Number); ok {
		if err := ValidateNumberText(number.String(), state.maxNumberBytes, state.maxAbsExponent); err != nil {
			return nil, err
		}
	}
	return token, nil
}

// ValidateNumberText bounds exact-number parsing before callers hand a value
// to math/big. A small JSON body can otherwise request an enormous exponent,
// and a large mantissa can cause disproportionate CPU and memory use.
func ValidateNumberText(text string, maxBytes int, maxAbsExponent int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxNumberBytes
	}
	if maxAbsExponent <= 0 {
		maxAbsExponent = DefaultMaxAbsExponent
	}
	if text == "" || len(text) > maxBytes {
		return errors.New("JSON number limit exceeded")
	}
	first := text[0]
	if (first < '0' || first > '9') && first != '-' || !json.Valid([]byte(text)) {
		return errors.New("invalid JSON number")
	}
	index := strings.IndexAny(text, "eE")
	if index < 0 {
		return nil
	}
	exponent := text[index+1:]
	if exponent == "" {
		return errors.New("invalid JSON number")
	}
	if exponent[0] == '+' || exponent[0] == '-' {
		exponent = exponent[1:]
	}
	exponent = strings.TrimLeft(exponent, "0")
	if exponent == "" {
		return nil
	}
	limit := strconv.Itoa(maxAbsExponent)
	if len(exponent) > len(limit) || len(exponent) == len(limit) && exponent > limit {
		return errors.New("JSON number exponent limit exceeded")
	}
	return nil
}

func validateUnicodeEscapes(data []byte) error {
	inString := false
	for index := 0; index < len(data); index++ {
		switch data[index] {
		case '"':
			inString = !inString
		case '\\':
			if !inString || index+1 >= len(data) {
				continue
			}
			index++
			if data[index] != 'u' || index+4 >= len(data) {
				continue
			}
			unit, err := strconv.ParseUint(string(data[index+1:index+5]), 16, 16)
			if err != nil {
				continue
			}
			index += 4
			switch {
			case unit >= 0xD800 && unit <= 0xDBFF:
				if index+6 >= len(data) || data[index+1] != '\\' || data[index+2] != 'u' {
					return errors.New("JSON contains an unpaired Unicode surrogate")
				}
				low, err := strconv.ParseUint(string(data[index+3:index+7]), 16, 16)
				if err != nil || low < 0xDC00 || low > 0xDFFF {
					return errors.New("JSON contains an unpaired Unicode surrogate")
				}
				index += 6
			case unit >= 0xDC00 && unit <= 0xDFFF:
				return errors.New("JSON contains an unpaired Unicode surrogate")
			}
		}
	}
	return nil
}
