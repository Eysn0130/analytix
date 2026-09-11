package jsonschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

type DefinitionOptions struct {
	RequireObjectRoot            bool
	RequireClosedObjects         bool
	RequireExplicitClosedObjects bool
	RequireCompleteCollections   bool
}

// ValidateDefinition accepts the deliberately bounded JSON Schema subset
// enforced by the runtime's deterministic validator. Unsupported constraint
// keywords fail closed instead of being silently ignored.
func ValidateDefinition(value any, options DefinitionOptions) error {
	schema, ok := value.(map[string]any)
	if !ok || len(schema) == 0 {
		return errors.New("schema must be a non-empty object")
	}
	if options.RequireObjectRoot && !allowsType(schema["type"], "object") {
		return errors.New("schema root type must be object")
	}
	if err := validateSchemaSecurityPolicy(schema, "$", options); err != nil {
		return err
	}
	if err := validateSchemaEvaluationBudget(schema); err != nil {
		return err
	}
	return validateStandardDefinition(schema)
}

func validateSchema(schema map[string]any, path string, options DefinitionOptions) error {
	for keyword := range schema {
		switch keyword {
		case "type", "properties", "required", "additionalProperties", "items", "anyOf", "oneOf", "allOf",
			"enum", "const", "pattern", "minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength",
			"dependentRequired", "description", "title", "default", "examples", "$comment", "$schema":
		default:
			return fmt.Errorf("%s uses unsupported schema keyword %q", path, keyword)
		}
	}
	if rawType, exists := schema["type"]; exists {
		types, ok := typeNames(rawType)
		if !ok || len(types) == 0 {
			return fmt.Errorf("%s.type must be a string or non-empty string array", path)
		}
		for _, name := range types {
			switch name {
			case "null", "boolean", "object", "array", "number", "integer", "string":
			default:
				return fmt.Errorf("%s.type contains unsupported type %q", path, name)
			}
		}
	}
	objectKeywords := hasAny(schema, "properties", "required", "additionalProperties", "dependentRequired")
	if objectKeywords && !allowsType(schema["type"], "object") {
		return fmt.Errorf("%s uses object keywords without object type", path)
	}
	arrayKeywords := hasAny(schema, "items", "minItems", "maxItems")
	if arrayKeywords && !allowsType(schema["type"], "array") {
		return fmt.Errorf("%s uses array keywords without array type", path)
	}
	stringKeywords := hasAny(schema, "pattern", "minLength", "maxLength")
	if stringKeywords && !allowsType(schema["type"], "string") {
		return fmt.Errorf("%s uses string keywords without string type", path)
	}
	if hasAny(schema, "minimum", "maximum") && !allowsAnyType(schema["type"], "number", "integer") {
		return fmt.Errorf("%s uses numeric keywords without numeric type", path)
	}
	if _, hasType := schema["type"]; !hasType && !hasAny(schema, "anyOf", "oneOf", "allOf", "enum", "const") {
		return fmt.Errorf("%s must declare type or a bounded composition", path)
	}
	properties := map[string]any{}
	if rawProperties, exists := schema["properties"]; exists {
		var ok bool
		properties, ok = rawProperties.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.properties must be an object", path)
		}
		for name, rawProperty := range properties {
			property, ok := rawProperty.(map[string]any)
			if !ok || len(property) == 0 {
				return fmt.Errorf("%s.properties.%s must be a non-empty schema object", path, name)
			}
			if err := validateSchema(property, path+".properties."+name, options); err != nil {
				return err
			}
		}
	}
	if rawRequired, exists := schema["required"]; exists {
		required, ok := stringArray(rawRequired)
		if !ok {
			return fmt.Errorf("%s.required must be a string array", path)
		}
		for _, name := range required {
			if _, exists := properties[name]; !exists {
				return fmt.Errorf("%s.required references unknown property %q", path, name)
			}
		}
	}
	if rawAdditional, exists := schema["additionalProperties"]; exists {
		switch additional := rawAdditional.(type) {
		case bool:
			if options.RequireClosedObjects && allowsType(schema["type"], "object") && additional {
				return fmt.Errorf("%s.additionalProperties must be false", path)
			}
		case map[string]any:
			if options.RequireClosedObjects {
				return fmt.Errorf("%s.additionalProperties must be false", path)
			}
			if err := validateSchema(additional, path+".additionalProperties", options); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s.additionalProperties must be boolean or a schema object", path)
		}
	} else if options.RequireExplicitClosedObjects && allowsType(schema["type"], "object") {
		return fmt.Errorf("%s.additionalProperties must be explicitly false", path)
	}
	if rawItems, exists := schema["items"]; exists {
		items, ok := rawItems.(map[string]any)
		if !ok || len(items) == 0 {
			return fmt.Errorf("%s.items must be a non-empty schema object", path)
		}
		if err := validateSchema(items, path+".items", options); err != nil {
			return err
		}
	} else if options.RequireCompleteCollections && allowsType(schema["type"], "array") {
		return fmt.Errorf("%s.items is required for array schemas", path)
	}
	for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
		rawBranches, exists := schema[keyword]
		if !exists {
			continue
		}
		branches, ok := rawBranches.([]any)
		if !ok || len(branches) == 0 {
			return fmt.Errorf("%s.%s must be a non-empty schema array", path, keyword)
		}
		for index, rawBranch := range branches {
			branch, ok := rawBranch.(map[string]any)
			if !ok || len(branch) == 0 {
				return fmt.Errorf("%s.%s[%d] must be a non-empty schema object", path, keyword, index)
			}
			if err := validateSchema(branch, fmt.Sprintf("%s.%s[%d]", path, keyword, index), options); err != nil {
				return err
			}
		}
	}
	if rawEnum, exists := schema["enum"]; exists {
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 {
			return fmt.Errorf("%s.enum must be a non-empty array", path)
		}
	}
	if rawPattern, exists := schema["pattern"]; exists {
		pattern, ok := rawPattern.(string)
		if !ok {
			return fmt.Errorf("%s.pattern must be a string", path)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%s.pattern is invalid: %w", path, err)
		}
	}
	for _, keyword := range []string{"minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength"} {
		value, exists := schema[keyword]
		if !exists {
			continue
		}
		number, ok := number(value)
		integerConstraint := keyword == "minItems" || keyword == "maxItems" || keyword == "minLength" || keyword == "maxLength"
		if !ok || (integerConstraint && (number.Sign() < 0 || !number.IsInt())) {
			return fmt.Errorf("%s.%s has an invalid numeric constraint", path, keyword)
		}
	}
	if rawDependent, exists := schema["dependentRequired"]; exists {
		dependent, ok := rawDependent.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.dependentRequired must be an object", path)
		}
		for name, dependencies := range dependent {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("%s.dependentRequired contains an empty property", path)
			}
			if _, ok := stringArray(dependencies); !ok {
				return fmt.Errorf("%s.dependentRequired.%s must be a string array", path, name)
			}
		}
	}
	for _, keyword := range []string{"description", "title", "$comment", "$schema"} {
		if value, exists := schema[keyword]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s.%s must be a string", path, keyword)
			}
		}
	}
	if value, exists := schema["$schema"].(string); exists && !supportedDialect(value) {
		return fmt.Errorf("%s.$schema uses unsupported dialect", path)
	}
	if value, exists := schema["examples"]; exists {
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("%s.examples must be an array", path)
		}
	}
	return nil
}

func supportedDialect(value string) bool {
	switch strings.TrimSpace(value) {
	case "http://json-schema.org/draft-07/schema#", "https://json-schema.org/draft/2020-12/schema":
		return true
	default:
		return false
	}
}

func hasAny(schema map[string]any, keywords ...string) bool {
	for _, keyword := range keywords {
		if _, exists := schema[keyword]; exists {
			return true
		}
	}
	return false
}

func allowsAnyType(value any, expected ...string) bool {
	for _, name := range expected {
		if allowsType(value, name) {
			return true
		}
	}
	return false
}

func typeNames(value any) ([]string, bool) {
	switch typed := value.(type) {
	case string:
		return []string{typed}, typed != ""
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			name, ok := item.(string)
			if !ok || name == "" {
				return nil, false
			}
			out = append(out, name)
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func allowsType(value any, expected string) bool {
	types, ok := typeNames(value)
	if !ok {
		return false
	}
	for _, name := range types {
		if name == expected {
			return true
		}
	}
	return false
}

func stringArray(value any) ([]string, bool) {
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

func number(value any) (*big.Rat, bool) {
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
	case json.Number:
		text = typed.String()
	default:
		return nil, false
	}
	if err := domainjsonstrict.ValidateNumberText(text, 0, 0); err != nil {
		return nil, false
	}
	parsed, ok := new(big.Rat).SetString(text)
	return parsed, ok
}
