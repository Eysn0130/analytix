package toolcatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	domainjsonschema "analytix.local/runtime-go/internal/domain/jsonschema"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const maxToolContractJSONBytes = 4 * 1024 * 1024
const maxToolContractJSONTokens = 200_000
const maxToolContractStringBytes = 1024 * 1024

func strictToolContractJSONOptions(requireObject bool) domainjsonstrict.Options {
	return domainjsonstrict.Options{
		RequireObject: requireObject,
		MaxBytes:      maxToolContractJSONBytes, MaxTokens: maxToolContractJSONTokens, MaxStringBytes: maxToolContractStringBytes,
	}
}

func ValidateToolCallArguments(call domainmodel.ToolCall, schemas []domainmodel.ToolSchema) (map[string]any, bool) {
	schema, ok := toolSchemaByName(call.Name, schemas)
	if !ok {
		return map[string]any{
			"code":     "tool_schema_missing",
			"error":    fmt.Sprintf("%s has no advertised schema", call.Name),
			"toolName": call.Name,
		}, true
	}
	args, invalidJSON := toolCallArgumentObject(call.Arguments)
	if invalidJSON != "" {
		return map[string]any{
			"code":     "validation_error",
			"error":    fmt.Sprintf("%s arguments were not valid JSON: %s", call.Name, invalidJSON),
			"toolName": call.Name,
		}, true
	}
	definition, schemaError := toolArgumentSchema(schema.Parameters)
	if schemaError != "" {
		return map[string]any{
			"code":     "tool_schema_invalid",
			"error":    fmt.Sprintf("%s schema is invalid: %s", call.Name, schemaError),
			"toolName": call.Name,
		}, true
	}
	standardError := domainjsonschema.ValidateStandardValue(definition, args)
	if standardError == nil {
		return nil, false
	}
	validationError := "arguments do not satisfy the advertised JSON Schema"
	if missing := firstMissingRequiredProperty(args, definition); missing != "" {
		validationError = missing + " is required"
	}
	return map[string]any{
		"code":     "validation_error",
		"error":    validationError,
		"toolName": call.Name,
	}, true
}

// ToolRequiresExactEmptyObjectArguments reports the narrow provider contract
// used by strict zero-argument tools. It is intentionally conservative: only
// a closed root object with no declared properties qualifies, so recovery
// guidance can never widen a tool's advertised schema.
func ToolRequiresExactEmptyObjectArguments(name string, schemas []domainmodel.ToolSchema) bool {
	schema, ok := toolSchemaByName(strings.TrimSpace(name), schemas)
	if !ok {
		return false
	}
	definition, schemaError := toolArgumentSchema(schema.Parameters)
	if schemaError != "" {
		return false
	}
	if rawProperties, present := definition["properties"]; present {
		properties, ok := rawProperties.(map[string]any)
		if !ok || len(properties) != 0 {
			return false
		}
	}
	additionalProperties, ok := definition["additionalProperties"].(bool)
	if !ok || additionalProperties {
		return false
	}
	for keyword := range definition {
		switch keyword {
		case "type", "properties", "required", "additionalProperties",
			"description", "title", "default", "examples", "$comment", "$schema",
			"minProperties", "maxProperties":
		default:
			return false
		}
	}
	return domainjsonschema.ValidateStandardValue(definition, map[string]any{}) == nil
}

// ValidateProviderVisibleToolSchemas enforces one closed contract before the
// same catalog is hashed, advertised, and used to issue execution grants. It
// never rewrites a schema: source definitions must already be complete so the
// provider-visible bytes and execution authority cannot diverge.
func ValidateProviderVisibleToolSchemas(schemas []domainmodel.ToolSchema) error {
	seen := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Name)
		if name == "" || name != schema.Name {
			return errors.New("tool_catalog_invalid: tool name is empty or non-canonical")
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("tool_catalog_invalid: duplicate tool %q", name)
		}
		seen[name] = struct{}{}
		if err := validateClosedProviderSchema(schema.Parameters); err != nil {
			return fmt.Errorf("tool_catalog_invalid: input schema for %q: %w", name, err)
		}
		if len(schema.OutputSchema) > 0 {
			if err := validateClosedProviderSchema(schema.OutputSchema); err != nil {
				return fmt.Errorf("tool_catalog_invalid: output schema for %q: %w", name, err)
			}
		}
	}
	return nil
}

func validateClosedProviderSchema(raw json.RawMessage) error {
	definition, schemaError := toolArgumentSchema(raw)
	if schemaError != "" {
		return errors.New(schemaError)
	}
	return domainjsonschema.ValidateDefinition(definition, domainjsonschema.DefinitionOptions{
		RequireObjectRoot:            true,
		RequireClosedObjects:         true,
		RequireExplicitClosedObjects: true,
		RequireCompleteCollections:   true,
	})
}

// ValidateJSONSchemaRawValue validates an MCP structured result without
// coercion. Required means property presence (the JSON Schema meaning), so an
// explicitly empty array/object remains distinct from a missing value.
func ValidateJSONSchemaRawValue(value json.RawMessage, schema json.RawMessage, path string) error {
	definition, schemaError := toolArgumentSchema(schema)
	if schemaError != "" {
		return fmt.Errorf("schema is invalid: %s", schemaError)
	}
	if err := domainjsonstrict.Validate(value, strictToolContractJSONOptions(false)); err != nil {
		return fmt.Errorf("value is not strict JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("value is not valid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("value contains trailing JSON")
	}
	if strings.TrimSpace(path) == "" {
		path = "value"
	}
	if standardError := domainjsonschema.ValidateStandardValue(definition, decoded); standardError != nil {
		return errors.New(path + " does not satisfy the advertised JSON Schema")
	}
	return nil
}

// firstMissingRequiredProperty is intentionally limited to root-property
// presence. Standards-compliant validation remains authoritative; this helper
// only preserves a deterministic, data-free diagnostic for the common missing
// argument case and never attempts to interpret arbitrary child schemas.
func firstMissingRequiredProperty(value map[string]any, schema map[string]any) string {
	required, ok := schema["required"].([]any)
	if !ok {
		return ""
	}
	for _, item := range required {
		name, ok := item.(string)
		if !ok {
			continue
		}
		if _, present := value[name]; !present {
			return name
		}
	}
	return ""
}

func toolArgumentSchema(raw json.RawMessage) (map[string]any, string) {
	if len(raw) == 0 {
		return nil, "schema is empty"
	}
	decoded, err := domainjsonstrict.DecodeValue(raw, strictToolContractJSONOptions(true))
	if err != nil {
		return nil, err.Error()
	}
	schema, ok := decoded.(map[string]any)
	if !ok || len(schema) == 0 {
		return nil, "schema must be a non-empty JSON object"
	}
	if !schemaAllowsType(schema, "object") {
		return nil, "root schema must allow object arguments"
	}
	if err := domainjsonschema.ValidateDefinition(schema, domainjsonschema.DefinitionOptions{RequireObjectRoot: true}); err != nil {
		return nil, err.Error()
	}
	return schema, ""
}

func validateJSONSchemaDefinition(schema map[string]any, root bool) string {
	if root && len(schema) == 0 {
		return "schema has no constraints"
	}
	for keyword := range schema {
		switch keyword {
		case "type", "properties", "required", "additionalProperties", "items", "anyOf", "oneOf", "allOf",
			"enum", "const", "pattern", "minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength",
			"minProperties", "maxProperties", "dependentRequired", "description", "title", "default", "examples", "$comment", "$schema":
		default:
			return "unsupported schema keyword " + keyword
		}
	}
	if rawType, ok := schema["type"]; ok {
		types := schemaTypeNames(rawType)
		if len(types) == 0 {
			return "type must be a string or non-empty string array"
		}
		for _, name := range types {
			switch name {
			case "null", "boolean", "object", "array", "number", "integer", "string":
			default:
				return "unsupported type " + name
			}
		}
	}
	if properties, ok := schema["properties"]; ok {
		propertyMap, ok := properties.(map[string]any)
		if !ok {
			return "properties must be an object"
		}
		for name, rawProperty := range propertyMap {
			property, ok := rawProperty.(map[string]any)
			if !ok {
				return "property " + name + " must be a schema object"
			}
			if err := validateJSONSchemaDefinition(property, false); err != "" {
				return "property " + name + ": " + err
			}
		}
	}
	if required, ok := schema["required"]; ok {
		requiredFields, valid := stringArray(required)
		if !valid {
			return "required must be a string array"
		}
		properties, _ := schema["properties"].(map[string]any)
		for _, name := range requiredFields {
			if _, exists := properties[name]; !exists {
				return "required references unknown property " + name
			}
		}
	}
	if additional, ok := schema["additionalProperties"]; ok {
		switch typed := additional.(type) {
		case bool:
		case map[string]any:
			if err := validateJSONSchemaDefinition(typed, false); err != "" {
				return "additionalProperties: " + err
			}
		default:
			return "additionalProperties must be boolean or a schema object"
		}
	}
	if items, ok := schema["items"]; ok {
		itemSchema, ok := items.(map[string]any)
		if !ok {
			return "items must be a schema object"
		}
		if err := validateJSONSchemaDefinition(itemSchema, false); err != "" {
			return "items: " + err
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
		branches, ok := schema[keyword]
		if !ok {
			continue
		}
		branchList, ok := branches.([]any)
		if !ok || len(branchList) == 0 {
			return keyword + " must be a non-empty schema array"
		}
		for _, rawBranch := range branchList {
			branch, ok := rawBranch.(map[string]any)
			if !ok {
				return keyword + " entries must be schema objects"
			}
			if err := validateJSONSchemaDefinition(branch, false); err != "" {
				return keyword + ": " + err
			}
		}
	}
	if rawEnum, ok := schema["enum"]; ok {
		values, valid := rawEnum.([]any)
		if !valid || len(values) == 0 {
			return "enum must be a non-empty array"
		}
	}
	if patternValue, exists := schema["pattern"]; exists {
		pattern, ok := patternValue.(string)
		if !ok {
			return "pattern must be a string"
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return "pattern is invalid: " + err.Error()
		}
	}
	for _, keyword := range []string{"minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength", "minProperties", "maxProperties"} {
		if value, exists := schema[keyword]; exists {
			number, valid := numberKeyword(value)
			integerConstraint := keyword == "minItems" || keyword == "maxItems" || keyword == "minLength" || keyword == "maxLength" ||
				keyword == "minProperties" || keyword == "maxProperties"
			if !valid || (integerConstraint && (number.Sign() < 0 || !number.IsInt())) {
				return keyword + " must be a valid non-negative integer constraint"
			}
		}
	}
	if dependentValue, exists := schema["dependentRequired"]; exists {
		dependent, valid := dependentValue.(map[string]any)
		if !valid {
			return "dependentRequired must be an object"
		}
		for name, dependencies := range dependent {
			if strings.TrimSpace(name) == "" {
				return "dependentRequired contains an empty property"
			}
			if _, valid := stringArray(dependencies); !valid {
				return "dependentRequired for " + name + " must be a string array"
			}
		}
	}
	for _, keyword := range []string{"description", "title", "$comment", "$schema"} {
		if value, exists := schema[keyword]; exists {
			if _, valid := value.(string); !valid {
				return keyword + " must be a string"
			}
		}
	}
	if dialect, exists := schema["$schema"].(string); exists {
		switch strings.TrimSpace(dialect) {
		case "http://json-schema.org/draft-07/schema#", "https://json-schema.org/draft/2020-12/schema":
		default:
			return "$schema uses unsupported dialect"
		}
	}
	if examples, exists := schema["examples"]; exists {
		if _, valid := examples.([]any); !valid {
			return "examples must be an array"
		}
	}
	return ""
}

func validateJSONSchemaValue(value any, schema map[string]any, path string) string {
	return validateJSONSchemaValueWithOptions(value, schema, path, false)
}

func validateJSONSchemaValueWithOptions(value any, schema map[string]any, path string, requiredPresenceOnly bool) string {
	if branches, ok := schema["allOf"].([]any); ok {
		for _, rawBranch := range branches {
			if message := validateJSONSchemaValueWithOptions(value, rawBranch.(map[string]any), path, requiredPresenceOnly); message != "" {
				return message
			}
		}
	}
	if branches, ok := schema["anyOf"].([]any); ok {
		matched := false
		for _, rawBranch := range branches {
			if validateJSONSchemaValueWithOptions(value, rawBranch.(map[string]any), path, requiredPresenceOnly) == "" {
				matched = true
				break
			}
		}
		if !matched {
			return path + " does not match any allowed schema"
		}
	}
	if branches, ok := schema["oneOf"].([]any); ok {
		matched := 0
		for _, rawBranch := range branches {
			if validateJSONSchemaValueWithOptions(value, rawBranch.(map[string]any), path, requiredPresenceOnly) == "" {
				matched++
			}
		}
		if matched != 1 {
			return path + " must match exactly one allowed schema"
		}
	}
	if !valueMatchesSchemaType(value, schema) {
		return path + " has the wrong type"
	}
	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, allowed := range enum {
			if jsonSchemaValuesEqual(value, allowed) {
				matched = true
				break
			}
		}
		if !matched {
			return path + " is not one of the allowed values"
		}
	}
	if expected, exists := schema["const"]; exists && !jsonSchemaValuesEqual(value, expected) {
		return path + " does not match the required constant"
	}
	if object, ok := value.(map[string]any); ok {
		if minimum, ok := numberKeyword(schema["minProperties"]); ok && compareInteger(len(object), minimum) < 0 {
			return path + " has too few properties"
		}
		if maximum, ok := numberKeyword(schema["maxProperties"]); ok && compareInteger(len(object), maximum) > 0 {
			return path + " has too many properties"
		}
		properties, _ := schema["properties"].(map[string]any)
		required, _ := stringArray(schema["required"])
		for _, name := range required {
			item, present := object[name]
			if !present || (!requiredPresenceOnly && requiredArgumentMissing(item)) {
				return name + " is required"
			}
		}
		for name, item := range object {
			rawProperty, known := properties[name]
			if known {
				if message := validateJSONSchemaValueWithOptions(item, rawProperty.(map[string]any), path+"."+name, requiredPresenceOnly); message != "" {
					return message
				}
				continue
			}
			switch additional := schema["additionalProperties"].(type) {
			case bool:
				if !additional {
					return path + " contains unknown field " + name
				}
			case map[string]any:
				if message := validateJSONSchemaValueWithOptions(item, additional, path+"."+name, requiredPresenceOnly); message != "" {
					return message
				}
			}
		}
		if dependent, ok := schema["dependentRequired"].(map[string]any); ok {
			for name, rawDependencies := range dependent {
				if _, present := object[name]; !present {
					continue
				}
				dependencies, valid := stringArray(rawDependencies)
				if !valid {
					return "schema dependentRequired for " + name + " is invalid"
				}
				for _, dependency := range dependencies {
					if _, present := object[dependency]; !present {
						return path + "." + dependency + " is required when " + name + " is present"
					}
				}
			}
		}
	}
	if array, ok := value.([]any); ok {
		if minimum, ok := numberKeyword(schema["minItems"]); ok && compareInteger(len(array), minimum) < 0 {
			return path + " has too few items"
		}
		if maximum, ok := numberKeyword(schema["maxItems"]); ok && compareInteger(len(array), maximum) > 0 {
			return path + " has too many items"
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for index, item := range array {
				if message := validateJSONSchemaValueWithOptions(item, items, fmt.Sprintf("%s[%d]", path, index), requiredPresenceOnly); message != "" {
					return message
				}
			}
		}
	}
	if text, ok := value.(string); ok {
		if minimum, ok := numberKeyword(schema["minLength"]); ok && compareInteger(len([]rune(text)), minimum) < 0 {
			return path + " is too short"
		}
		if maximum, ok := numberKeyword(schema["maxLength"]); ok && compareInteger(len([]rune(text)), maximum) > 0 {
			return path + " is too long"
		}
		if pattern, ok := schema["pattern"].(string); ok {
			compiled, _ := regexp.Compile(pattern)
			if !compiled.MatchString(text) {
				return path + " does not match the required pattern"
			}
		}
	}
	if number, ok := jsonNumber(value); ok {
		if minimum, ok := numberKeyword(schema["minimum"]); ok && number.Cmp(minimum) < 0 {
			return path + " is below the minimum"
		}
		if maximum, ok := numberKeyword(schema["maximum"]); ok && number.Cmp(maximum) > 0 {
			return path + " is above the maximum"
		}
	}
	return ""
}

func jsonSchemaValuesEqual(left any, right any) bool {
	if leftNumber, leftOK := jsonNumber(left); leftOK {
		if rightNumber, rightOK := jsonNumber(right); rightOK {
			return leftNumber.Cmp(rightNumber) == 0
		}
	}
	if reflect.DeepEqual(left, right) {
		return true
	}
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func schemaAllowsType(schema map[string]any, wanted string) bool {
	types := schemaTypeNames(schema["type"])
	if len(types) == 0 {
		_, hasProperties := schema["properties"]
		return wanted == "object" && hasProperties
	}
	for _, name := range types {
		if name == wanted {
			return true
		}
	}
	return false
}

func schemaTypeNames(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			name, ok := item.(string)
			if !ok || name == "" {
				return nil
			}
			out = append(out, name)
		}
		return out
	default:
		return nil
	}
}

func valueMatchesSchemaType(value any, schema map[string]any) bool {
	types := schemaTypeNames(schema["type"])
	if len(types) == 0 {
		return true
	}
	for _, name := range types {
		switch name {
		case "null":
			if value == nil {
				return true
			}
		case "boolean":
			_, ok := value.(bool)
			if ok {
				return true
			}
		case "object":
			_, ok := value.(map[string]any)
			if ok {
				return true
			}
		case "array":
			_, ok := value.([]any)
			if ok {
				return true
			}
		case "string":
			_, ok := value.(string)
			if ok {
				return true
			}
		case "number":
			if _, ok := jsonNumber(value); ok {
				return true
			}
		case "integer":
			if number, ok := jsonNumber(value); ok && number.IsInt() {
				return true
			}
		}
	}
	return false
}

func stringArray(value any) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		name, ok := item.(string)
		if !ok || name == "" {
			return nil, false
		}
		out = append(out, name)
	}
	return out, true
}

func numberKeyword(value any) (*big.Rat, bool) {
	return jsonNumber(value)
}

func jsonNumber(value any) (*big.Rat, bool) {
	var text string
	switch typed := value.(type) {
	case float64:
		text = strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		text = strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case int:
		return new(big.Rat).SetInt64(int64(typed)), true
	case int64:
		return new(big.Rat).SetInt64(typed), true
	case int32:
		return new(big.Rat).SetInt64(int64(typed)), true
	case json.Number:
		text = typed.String()
	default:
		return nil, false
	}
	if err := domainjsonstrict.ValidateNumberText(text, 0, 0); err != nil {
		return nil, false
	}
	number, ok := new(big.Rat).SetString(text)
	return number, ok
}

func compareInteger(value int, constraint *big.Rat) int {
	return new(big.Rat).SetInt64(int64(value)).Cmp(constraint)
}

func toolSchemaByName(name string, schemas []domainmodel.ToolSchema) (domainmodel.ToolSchema, bool) {
	name = strings.TrimSpace(name)
	for _, schema := range schemas {
		if strings.TrimSpace(schema.Name) == name {
			return schema, true
		}
	}
	return domainmodel.ToolSchema{}, false
}

func toolCallArgumentObject(raw json.RawMessage) (map[string]any, string) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		text = "{}"
	}
	decoded, err := domainjsonstrict.DecodeValue([]byte(text), strictToolContractJSONOptions(true))
	if err != nil {
		return nil, err.Error()
	}
	if decoded == nil {
		return map[string]any{}, ""
	}
	args, ok := decoded.(map[string]any)
	if !ok {
		return nil, "arguments must be a JSON object"
	}
	return args, ""
}

func missingRequiredArguments(args map[string]any, schema json.RawMessage) []string {
	required := requiredFields(schema)
	if len(required) == 0 {
		return nil
	}
	missing := []string{}
	for _, name := range required {
		value, ok := args[name]
		if !ok || requiredArgumentMissing(value) {
			missing = append(missing, name)
		}
	}
	return missing
}

func requiredFields(schema json.RawMessage) []string {
	var decoded struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, name := range decoded.Required {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func requiredArgumentMissing(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}

func stringSliceAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
