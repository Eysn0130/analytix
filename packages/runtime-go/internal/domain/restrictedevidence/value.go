package restrictedevidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
)

const (
	maxSerializedJSONBytes = 1024 * 1024
	maxInspectionDepth     = 64
	maxInspectionNodes     = 100_000
)

var (
	ErrRestrictedEvidence = errors.New("restricted evidence is not projectable")
	ErrInspectionLimit    = errors.New("restricted evidence inspection limit exceeded")
	errDuplicateJSONKey   = errors.New("serialized JSON contains a duplicate key")
)

// Validate rejects private evidence contracts that have been laundered into
// generic maps, arrays, byte slices, or serialized JSON strings. It does not
// reject arbitrary SHA-256 values or dataset snapshot identifiers: those are
// legitimate fields in several closed public projections.
func Validate(value any) error {
	budget := inspectionBudget{nodes: maxInspectionNodes}
	return validate(value, 0, &budget)
}

// Contains is the conservative sink helper. An input that cannot be inspected
// within the fixed bounds is unsafe at an ordinary projection boundary.
func Contains(value any) bool {
	return Validate(value) != nil
}

func isRestrictedObject(value map[string]any) bool {
	if isTypedLocalResponseIdentityV1(value) {
		return true
	}
	var normalized map[string][]any
	for key, child := range value {
		normalizedKey := normalizeKey(key)
		purpose, ok := child.(string)
		if normalizedKey == "purpose" && ok && restrictedPurpose(purpose) {
			return true
		}
		if !canonicalTextRelevantKey(normalizedKey) && normalizedKey != "facts" &&
			normalizedKey != "sourcefieldbindings" && normalizedKey != "sourcefieldbindingsetdigest" {
			continue
		}
		if normalized == nil {
			normalized = make(map[string][]any, 4)
		}
		normalized[normalizedKey] = append(normalized[normalizedKey], child)
	}
	return isObject(normalized)
}

func isTypedLocalResponseIdentityV1(value map[string]any) bool {
	if value == nil || !isTypedLocalResponseSchemaVersionV1(value["schemaVersion"]) {
		return false
	}
	kind, ok := value["kind"].(string)
	return ok && isTypedLocalResponseKindV1(kind)
}

func isTypedLocalResponseValuesV1(value map[string][]any) bool {
	if value == nil {
		return false
	}
	for _, schemaVersion := range value["schemaversion"] {
		if !isTypedLocalResponseSchemaVersionV1(schemaVersion) {
			continue
		}
		for _, candidate := range value["kind"] {
			if kind, ok := candidate.(string); ok && isTypedLocalResponseKindV1(kind) {
				return true
			}
		}
	}
	return false
}

func isTypedLocalResponseSchemaVersionV1(value any) bool {
	switch typed := value.(type) {
	case json.Number:
		return string(typed) == strconv.Itoa(domainlocaldisplay.ResponseSchemaVersionV1)
	case float64:
		return typed == float64(domainlocaldisplay.ResponseSchemaVersionV1)
	case float32:
		return typed == float32(domainlocaldisplay.ResponseSchemaVersionV1)
	case int:
		return typed == domainlocaldisplay.ResponseSchemaVersionV1
	case int8:
		return typed == int8(domainlocaldisplay.ResponseSchemaVersionV1)
	case int16:
		return typed == int16(domainlocaldisplay.ResponseSchemaVersionV1)
	case int32:
		return typed == int32(domainlocaldisplay.ResponseSchemaVersionV1)
	case int64:
		return typed == int64(domainlocaldisplay.ResponseSchemaVersionV1)
	case uint:
		return typed == uint(domainlocaldisplay.ResponseSchemaVersionV1)
	case uint8:
		return typed == uint8(domainlocaldisplay.ResponseSchemaVersionV1)
	case uint16:
		return typed == uint16(domainlocaldisplay.ResponseSchemaVersionV1)
	case uint32:
		return typed == uint32(domainlocaldisplay.ResponseSchemaVersionV1)
	case uint64:
		return typed == uint64(domainlocaldisplay.ResponseSchemaVersionV1)
	default:
		return false
	}
}

func isTypedLocalResponseKindV1(kind string) bool {
	switch kind {
	case domainlocaldisplay.KindImportMappingPreviewV1,
		domainlocaldisplay.KindCleaningDiffPreviewV1,
		domainlocaldisplay.KindDirectSourcePreviewV1,
		domainlocaldisplay.KindAcceptedSlotDisplayV1:
		return true
	default:
		return false
	}
}

type inspectionBudget struct {
	nodes int
}

func validate(value any, depth int, budget *inspectionBudget) error {
	if depth > maxInspectionDepth || budget == nil || budget.nodes <= 0 {
		return ErrInspectionLimit
	}
	budget.nodes--
	switch typed := value.(type) {
	case nil, bool, json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case map[string]any:
		if isRestrictedObject(typed) {
			return ErrRestrictedEvidence
		}
		for _, child := range typed {
			if err := validate(child, depth+1, budget); err != nil {
				return err
			}
		}
		return nil
	case map[string]string:
		object := make(map[string]any, len(typed))
		for key, child := range typed {
			object[key] = child
		}
		return validate(object, depth, budget)
	case []any:
		for _, child := range typed {
			if err := validate(child, depth+1, budget); err != nil {
				return err
			}
		}
		return nil
	case json.RawMessage:
		return validateSerializedJSON([]byte(typed), depth, budget)
	case []byte:
		return validateSerializedJSON(typed, depth, budget)
	case string:
		return validateSerializedJSON([]byte(typed), depth, budget)
	default:
		return validateJSONShapedReflection(reflect.ValueOf(value), depth, budget)
	}
}

func validateJSONShapedReflection(value reflect.Value, depth int, budget *inspectionBudget) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() || !value.Elem().CanInterface() {
			return nil
		}
		return validate(value.Elem().Interface(), depth+1, budget)
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return ErrInspectionLimit
		}
		object := make(map[string]any, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			child := iterator.Value()
			if !child.IsValid() || !child.CanInterface() {
				return ErrInspectionLimit
			}
			object[iterator.Key().String()] = child.Interface()
		}
		return validate(object, depth, budget)
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			child := value.Index(index)
			if !child.CanInterface() {
				return ErrInspectionLimit
			}
			if err := validate(child.Interface(), depth+1, budget); err != nil {
				return err
			}
		}
		return nil
	default:
		// Runtime public values must remain JSON-shaped. In particular, do not
		// invoke custom marshalers that could conceal evidence during inspection.
		return ErrInspectionLimit
	}
}

func validateSerializedJSON(body []byte, depth int, budget *inspectionBudget) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return nil
	}
	if len(trimmed) > maxSerializedJSONBytes || exceedsJSONDepth(trimmed, maxInspectionDepth-depth) {
		return ErrInspectionLimit
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	decoded, err := decodeStrictJSONValue(decoder, depth+1)
	if errors.Is(err, errDuplicateJSONKey) {
		return ErrInspectionLimit
	}
	if err != nil {
		// Ordinary text that merely starts with punctuation is not a private
		// evidence contract. Only a complete JSON value can launder one.
		return nil
	}
	if err := consumeJSONEOF(decoder); err != nil {
		return nil
	}
	return validate(decoded, depth+1, budget)
}

func decodeStrictJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if decoder == nil || depth > maxInspectionDepth {
		return nil, ErrInspectionLimit
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			key, keyOK := keyToken.(string)
			if keyErr != nil || !keyOK {
				return nil, errors.Join(errors.New("serialized JSON object key is invalid"), keyErr)
			}
			if seen[key] {
				return nil, errDuplicateJSONKey
			}
			seen[key] = true
			child, childErr := decodeStrictJSONValue(decoder, depth+1)
			if childErr != nil {
				return nil, childErr
			}
			object[key] = child
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim('}') {
			return nil, errors.Join(errors.New("serialized JSON object is incomplete"), closeErr)
		}
		return object, nil
	case '[':
		array := []any{}
		for decoder.More() {
			child, childErr := decodeStrictJSONValue(decoder, depth+1)
			if childErr != nil {
				return nil, childErr
			}
			array = append(array, child)
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim(']') {
			return nil, errors.Join(errors.New("serialized JSON array is incomplete"), closeErr)
		}
		return array, nil
	default:
		return nil, errors.New("serialized JSON delimiter is invalid")
	}
}

func consumeJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("serialized JSON contains multiple values")
	}
	return err
}

func exceedsJSONDepth(body []byte, remaining int) bool {
	if remaining <= 0 {
		return true
	}
	depth := 0
	inString := false
	escaped := false
	for _, current := range body {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch current {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch current {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > remaining {
				return true
			}
		case '}', ']':
			depth--
		}
	}
	return false
}

func isObject(value map[string][]any) bool {
	if value == nil {
		return false
	}
	if isTypedLocalResponseValuesV1(value) {
		return true
	}
	for _, triple := range restrictedExactReferenceTriples {
		if hasSHA256(value, triple.digest) && hasSHA256(value, triple.sha256) && hasPositiveJSONSafeInteger(value, triple.byteLength) {
			return true
		}
	}
	return hasNonEmptyString(value, "sourceexactvalue") && hasSHA256(value, "sourceexactvaluesha256") && hasSHA256(value, "bindingdigest") ||
		hasSHA256(value, "rawartifactsha256") && hasSHA256(value, "sourcerecordsha256") && hasSHA256(value, "sourceexactvaluesha256") ||
		hasNonEmptyArray(value, "facts") && hasNonEmptyArray(value, "sourcefieldbindings") && hasSHA256(value, "sourcefieldbindingsetdigest") ||
		hasNonEmptyString(value, "datasetsnapshotid") && hasSHA256(value, "manifestdigest") && hasSHA256(value, "manifestsha256") &&
			hasPositiveJSONSafeInteger(value, "manifestbytelength") && hasNonEmptyString(value, "authoritysignature") && hasSHA256(value, "recorddigest") ||
		hasSHA256(value, "snapshotauthorityrecorddigest") && hasSHA256(value, "ledgerrootdigest") &&
			hasSHA256(value, "ledgerindexpagedigest") && hasSHA256(value, "ledgerpagedigest") && hasSHA256(value, "witnessdigest") ||
		hasSHA256(value, "sourcefileiddigest") && hasPositiveJSONSafeInteger(value, "sourcerownumber") && hasSHA256(value, "locatordigest")
}

type exactReferenceTriple struct {
	digest     string
	sha256     string
	byteLength string
}

var restrictedExactReferenceTriples = []exactReferenceTriple{
	{digest: "rawartifactmanifestdigest", sha256: "rawartifactmanifestsha256", byteLength: "rawartifactmanifestbytelength"},
	{digest: "parsedgenerationreceiptdigest", sha256: "parsedgenerationreceiptsha256", byteLength: "parsedgenerationreceiptbytelength"},
	{digest: "classificationledgerdigest", sha256: "classificationledgersha256", byteLength: "classificationledgerbytelength"},
	{digest: "sourcerowledgerrootdigest", sha256: "sourcerowledgerrootsha256", byteLength: "sourcerowledgerrootbytelength"},
	{digest: "lineagedigest", sha256: "lineagesha256", byteLength: "lineagebytelength"},
	{digest: "acquisitionintentdigest", sha256: "acquisitionintentsha256", byteLength: "acquisitionintentbytelength"},
	{digest: "sourcelocatordigest", sha256: "sourcelocatorsha256", byteLength: "sourcelocatorbytelength"},
	{digest: "contentrootdigest", sha256: "contentrootsha256", byteLength: "contentrootbytelength"},
	{digest: "rawartifactmanifestpagedigest", sha256: "rawartifactmanifestpagesha256", byteLength: "rawartifactmanifestpagebytelength"},
	{digest: "rawartifactentrydigest", sha256: "rawartifactentrysha256", byteLength: "rawartifactentrybytelength"},
	{digest: "rawsourcelocatordigest", sha256: "rawsourcelocatorsha256", byteLength: "rawsourcelocatorbytelength"},
	{digest: "parsedpagedigest", sha256: "parsedpagesha256", byteLength: "parsedpagebytelength"},
}

func hasSHA256(value map[string][]any, key string) bool {
	for _, candidate := range value[key] {
		text, ok := candidate.(string)
		if !ok || len(text) != 64 || text != strings.ToLower(text) {
			continue
		}
		if _, err := strconv.ParseUint(text[:16], 16, 64); err != nil {
			continue
		}
		valid := true
		for offset := 16; offset < len(text); offset += 16 {
			if _, err := strconv.ParseUint(text[offset:offset+16], 16, 64); err != nil {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func hasPositiveJSONSafeInteger(value map[string][]any, key string) bool {
	for _, candidate := range value[key] {
		if positiveJSONSafeInteger(candidate) {
			return true
		}
	}
	return false
}

func positiveJSONSafeInteger(value any) bool {
	const maximum = uint64(9_007_199_254_740_991)
	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseUint(string(typed), 10, 64)
		return err == nil && parsed > 0 && parsed <= maximum
	case float64:
		return typed > 0 && typed <= float64(maximum) && typed == float64(uint64(typed))
	case float32:
		return typed > 0 && float64(typed) <= float64(maximum) && typed == float32(uint64(typed))
	case int:
		return typed > 0 && uint64(typed) <= maximum
	case int8:
		return typed > 0
	case int16:
		return typed > 0
	case int32:
		return typed > 0
	case int64:
		return typed > 0 && uint64(typed) <= maximum
	case uint:
		return typed > 0 && uint64(typed) <= maximum
	case uint8:
		return typed > 0
	case uint16:
		return typed > 0
	case uint32:
		return typed > 0
	case uint64:
		return typed > 0 && typed <= maximum
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		return err == nil && parsed > 0 && parsed <= maximum
	default:
		return false
	}
}

func hasNonEmptyString(value map[string][]any, key string) bool {
	for _, candidate := range value[key] {
		if text, ok := candidate.(string); ok && strings.TrimSpace(text) != "" {
			return true
		}
	}
	return false
}

func hasNonEmptyArray(value map[string][]any, key string) bool {
	for _, candidate := range value[key] {
		reflected := reflect.ValueOf(candidate)
		if reflected.IsValid() && (reflected.Kind() == reflect.Array || reflected.Kind() == reflect.Slice) && reflected.Len() > 0 {
			return true
		}
	}
	return false
}

func restrictedPurpose(value string) bool {
	purpose := strings.ToLower(strings.TrimSpace(value))
	switch purpose {
	case "analytix.raw-artifact-acquisition-intent/v1",
		"analytix.raw-artifact-content-chunk-descriptor/v1",
		"analytix.raw-artifact-content-index-page-descriptor/v1",
		"analytix.raw-artifact-content-index-page/v1",
		"analytix.raw-artifact-content-root/v1",
		"analytix.raw-artifact-entry/v1",
		"analytix.raw-artifact-manifest-page-descriptor/v1",
		"analytix.raw-artifact-manifest-page/v1",
		"analytix.raw-artifact-manifest/v1",
		"analytix.raw-artifact-source-locator/v1",
		"analytix.parsed-generation-identity/v1",
		"analytix.parsed-generation-receipt/v1",
		"analytix.parsed-outcome/v1",
		"analytix.parsed-page-descriptor/v1",
		"analytix.parsed-page-index-descriptor/v1",
		"analytix.parsed-page-index/v1",
		"analytix.parsed-page/v1",
		"analytix.source-row-ledger-index-page-descriptor/v1",
		"analytix.source-row-ledger-index-page/v1",
		"analytix.source-row-ledger-page-descriptor/v1",
		"analytix.source-row-ledger-page-entry/v1",
		"analytix.source-row-ledger-page/v1",
		"analytix.source-row-ledger-root/v1",
		"analytix.source-row-lineage/v1",
		"analytix.source-row-locator/v1",
		"analytix.source-row-record/v1",
		"analytix.source-row-witness/v1",
		"analytix.dataset-snapshot-authority/v1",
		"analytix.dataset-snapshot-authority/v2",
		"analytix.dataset-snapshot-index/v1",
		"analytix.dataset-snapshot-manifest/v2",
		"analytix.source-field-binding/v2",
		"analytix.canonical-evidence/v2":
		return true
	}
	return false
}

func normalizeKey(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, current := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			builder.WriteRune(current)
		}
	}
	return builder.String()
}
