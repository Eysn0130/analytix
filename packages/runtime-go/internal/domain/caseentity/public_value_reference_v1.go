package caseentity

import (
	"encoding"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	maxPublicValueReferenceDepthV1     = 64
	maxPublicValueReferenceNodesV1     = 100_000
	maxPublicValueReferenceStringBytes = 1 << 20
	maxPublicValueReferenceJSONBytes   = 1 << 20
	privateSourceRowReferencePrefixV1  = "srow1_"
	privateSourceRowReferenceLengthV1  = len(privateSourceRowReferencePrefixV1) + 64
)

var (
	ErrPublicValueInternalReferenceV1   = errors.New("public value contains an internal case or evidence reference")
	ErrPublicValueReferenceInspectionV1 = errors.New(
		"public value internal-reference inspection failed closed",
	)

	jsonRawMessageTypeV1 = reflect.TypeOf(json.RawMessage(nil))
	jsonNumberTypeV1     = reflect.TypeOf(json.Number(""))
	jsonMarshalerTypeV1  = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerTypeV1  = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// ValidatePublicValueWithoutReferenceV1 recursively inspects a JSON-like
// public value and rejects every complete canonical ReferenceV1. The
// inspection deliberately does not invoke custom marshalers: an opaque shape,
// cycle, invalid JSON value, or fixed depth/node/byte bound is unsafe at a
// public sink and therefore fails closed.
func ValidatePublicValueWithoutReferenceV1(value any) error {
	state := publicValueReferenceInspectionV1{
		remainingNodes: maxPublicValueReferenceNodesV1,
		active:         make(map[publicValueReferenceVisitV1]struct{}),
	}
	return state.inspect(reflect.ValueOf(value), 0)
}

// ContainsReferenceInPublicValueV1 is the conservative public-sink helper.
// It returns true both for a detected canonical reference and when the value
// cannot be completely inspected within the closed JSON-like contract.
func ContainsReferenceInPublicValueV1(value any) bool {
	return ValidatePublicValueWithoutReferenceV1(value) != nil
}

// ContainsInternalReferenceV1 detects the two provider-visible private
// identities that must never cross an ordinary/public sink: stable case entity
// references and case/source-artifact-bound source-row evidence references.
// Snapshot/currentness authority remains a separate result/receipt binding.
// JSON Unicode escape spellings are decoded for inspection because provider
// text and tool fields can otherwise preserve an equivalent reference as
// literal text.
func ContainsInternalReferenceV1(value string) bool {
	if containsCanonicalInternalReferenceV1(value) {
		return true
	}
	decoded, changed := decodeASCIIJSONUnicodeEscapesV1(value)
	return changed && containsCanonicalInternalReferenceV1(decoded)
}

func containsCanonicalInternalReferenceV1(value string) bool {
	return ContainsReferenceV1(value) || containsPrivateSourceRowReferenceV1(value)
}

func containsPrivateSourceRowReferenceV1(value string) bool {
	for searchAt := 0; searchAt < len(value); {
		offset := strings.Index(value[searchAt:], privateSourceRowReferencePrefixV1)
		if offset < 0 {
			return false
		}
		start := searchAt + offset
		end := start + privateSourceRowReferenceLengthV1
		if end <= len(value) {
			valid := true
			for index := start + len(privateSourceRowReferencePrefixV1); index < end; index++ {
				current := value[index]
				if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
					valid = false
					break
				}
			}
			if valid {
				return true
			}
		}
		searchAt = start + len(privateSourceRowReferencePrefixV1)
	}
	return false
}

func decodeASCIIJSONUnicodeEscapesV1(value string) (string, bool) {
	if !strings.Contains(value, `\u`) {
		return value, false
	}
	var decoded strings.Builder
	decoded.Grow(len(value))
	changed := false
	for index := 0; index < len(value); {
		if index+6 <= len(value) && value[index] == '\\' && value[index+1] == 'u' {
			if current, ok := asciiJSONUnicodeEscapeV1(value[index+2 : index+6]); ok {
				decoded.WriteByte(current)
				index += 6
				changed = true
				continue
			}
		}
		decoded.WriteByte(value[index])
		index++
	}
	return decoded.String(), changed
}

func asciiJSONUnicodeEscapeV1(value string) (byte, bool) {
	if len(value) != 4 {
		return 0, false
	}
	var decoded uint16
	for index := range value {
		current := value[index]
		var digit byte
		switch {
		case current >= '0' && current <= '9':
			digit = current - '0'
		case current >= 'a' && current <= 'f':
			digit = current - 'a' + 10
		case current >= 'A' && current <= 'F':
			digit = current - 'A' + 10
		default:
			return 0, false
		}
		decoded = decoded*16 + uint16(digit)
	}
	if decoded > 0x7f {
		return 0, false
	}
	return byte(decoded), true
}

type publicValueReferenceInspectionV1 struct {
	remainingNodes int
	active         map[publicValueReferenceVisitV1]struct{}
}

type publicValueReferenceVisitV1 struct {
	kind    reflect.Kind
	typeID  reflect.Type
	pointer uintptr
	length  int
	cap     int
}

func (state *publicValueReferenceInspectionV1) inspect(value reflect.Value, depth int) error {
	if state == nil || depth > maxPublicValueReferenceDepthV1 || state.remainingNodes <= 0 {
		return ErrPublicValueReferenceInspectionV1
	}
	state.remainingNodes--
	if !value.IsValid() {
		return nil
	}

	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return state.inspect(value.Elem(), depth+1)
	}

	if value.Type() == jsonRawMessageTypeV1 {
		if !value.CanInterface() {
			return ErrPublicValueReferenceInspectionV1
		}
		return state.inspectRawJSON(value.Interface().(json.RawMessage), depth)
	}
	if value.Type() == jsonNumberTypeV1 {
		if !value.CanInterface() || domainjsonstrict.ValidateNumberText(
			value.Interface().(json.Number).String(),
			domainjsonstrict.DefaultMaxNumberBytes,
			domainjsonstrict.DefaultMaxAbsExponent,
		) != nil {
			return ErrPublicValueReferenceInspectionV1
		}
		return nil
	}

	if nilJSONLikeValueV1(value) {
		return nil
	}
	if hasOpaqueJSONMarshalerV1(value.Type()) {
		return ErrPublicValueReferenceInspectionV1
	}

	switch value.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return nil
	case reflect.Float32, reflect.Float64:
		current := value.Float()
		if math.IsNaN(current) || math.IsInf(current, 0) {
			return ErrPublicValueReferenceInspectionV1
		}
		return nil
	case reflect.String:
		return validatePublicReferenceTextV1(value.String())
	case reflect.Pointer:
		return state.inspectPointer(value, depth)
	case reflect.Map:
		return state.inspectMap(value, depth)
	case reflect.Slice:
		// RawMessage is handled above. Other byte slices are binary values,
		// not a closed JSON-like tree, and are withheld rather than guessed.
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return ErrPublicValueReferenceInspectionV1
		}
		return state.inspectSlice(value, depth)
	case reflect.Array:
		return state.inspectArray(value, depth)
	case reflect.Struct:
		return state.inspectStruct(value, depth)
	default:
		return ErrPublicValueReferenceInspectionV1
	}
}

func (state *publicValueReferenceInspectionV1) inspectPointer(value reflect.Value, depth int) error {
	leave, err := state.enter(value)
	if err != nil {
		return err
	}
	defer leave()
	if !value.Elem().CanInterface() {
		return ErrPublicValueReferenceInspectionV1
	}
	return state.inspect(value.Elem(), depth+1)
}

func (state *publicValueReferenceInspectionV1) inspectMap(value reflect.Value, depth int) error {
	keyType := value.Type().Key()
	if keyType.Kind() != reflect.String || hasOpaqueJSONMarshalerV1(keyType) {
		return ErrPublicValueReferenceInspectionV1
	}
	leave, err := state.enter(value)
	if err != nil {
		return err
	}
	defer leave()

	iterator := value.MapRange()
	for iterator.Next() {
		if err := validatePublicReferenceTextV1(iterator.Key().String()); err != nil {
			return err
		}
		child := iterator.Value()
		if !child.IsValid() || !child.CanInterface() {
			return ErrPublicValueReferenceInspectionV1
		}
		if err := state.inspect(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (state *publicValueReferenceInspectionV1) inspectSlice(value reflect.Value, depth int) error {
	leave, err := state.enter(value)
	if err != nil {
		return err
	}
	defer leave()
	return state.inspectIndexed(value, depth)
}

func (state *publicValueReferenceInspectionV1) inspectArray(value reflect.Value, depth int) error {
	return state.inspectIndexed(value, depth)
}

func (state *publicValueReferenceInspectionV1) inspectIndexed(value reflect.Value, depth int) error {
	for index := 0; index < value.Len(); index++ {
		child := value.Index(index)
		if !child.CanInterface() {
			return ErrPublicValueReferenceInspectionV1
		}
		if err := state.inspect(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (state *publicValueReferenceInspectionV1) inspectStruct(value reflect.Value, depth int) error {
	typeOfValue := value.Type()
	for index := 0; index < value.NumField(); index++ {
		field := typeOfValue.Field(index)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" || strings.HasPrefix(jsonTag, "-,") {
			continue
		}
		if field.PkgPath != "" {
			// Ordinary unexported fields are not JSON-visible. An unexported
			// anonymous field can promote visible fields, so its exact public
			// shape is ambiguous here and must fail closed.
			if field.Anonymous {
				return ErrPublicValueReferenceInspectionV1
			}
			continue
		}
		if name := strings.SplitN(jsonTag, ",", 2)[0]; name != "" {
			if err := validatePublicReferenceTextV1(name); err != nil {
				return err
			}
		}
		child := value.Field(index)
		if !child.CanInterface() {
			return ErrPublicValueReferenceInspectionV1
		}
		if err := state.inspect(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (state *publicValueReferenceInspectionV1) inspectRawJSON(body json.RawMessage, depth int) error {
	if body == nil {
		return nil
	}
	remainingDepth := maxPublicValueReferenceDepthV1 - depth
	if remainingDepth <= 0 || len(body) == 0 || len(body) > maxPublicValueReferenceJSONBytes {
		return ErrPublicValueReferenceInspectionV1
	}
	decoded, err := domainjsonstrict.DecodeValue(body, domainjsonstrict.Options{
		MaxBytes:       maxPublicValueReferenceJSONBytes,
		MaxDepth:       remainingDepth,
		MaxTokens:      state.remainingNodes,
		MaxStringBytes: maxPublicValueReferenceStringBytes,
	})
	if err != nil {
		return ErrPublicValueReferenceInspectionV1
	}
	return state.inspect(reflect.ValueOf(decoded), depth+1)
}

func (state *publicValueReferenceInspectionV1) enter(value reflect.Value) (func(), error) {
	visit := publicValueReferenceVisitV1{
		kind: value.Kind(), typeID: value.Type(), pointer: value.Pointer(),
	}
	if value.Kind() == reflect.Slice {
		visit.length = value.Len()
		visit.cap = value.Cap()
	}
	if _, exists := state.active[visit]; exists {
		return nil, ErrPublicValueReferenceInspectionV1
	}
	state.active[visit] = struct{}{}
	return func() { delete(state.active, visit) }, nil
}

func nilJSONLikeValueV1(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func hasOpaqueJSONMarshalerV1(valueType reflect.Type) bool {
	if valueType == nil {
		return false
	}
	if valueType.Implements(jsonMarshalerTypeV1) || valueType.Implements(textMarshalerTypeV1) {
		return true
	}
	if valueType.Kind() != reflect.Pointer {
		pointerType := reflect.PointerTo(valueType)
		return pointerType.Implements(jsonMarshalerTypeV1) || pointerType.Implements(textMarshalerTypeV1)
	}
	return false
}

func validatePublicReferenceTextV1(value string) error {
	if len(value) > maxPublicValueReferenceStringBytes || !utf8.ValidString(value) {
		return ErrPublicValueReferenceInspectionV1
	}
	if ContainsInternalReferenceV1(value) {
		return ErrPublicValueInternalReferenceV1
	}
	return nil
}
