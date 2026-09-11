package model

import (
	"encoding/json"
	"sort"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

func CanonicalJSONSchema(body json.RawMessage) string {
	if len(body) == 0 {
		return `{"type":"object","properties":{},"additionalProperties":false}`
	}
	value, err := domainjsonstrict.DecodeValue(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil {
		return strings.TrimSpace(string(body))
	}
	data, err := json.Marshal(canonicalJSONSchemaValue(value))
	if err != nil {
		return strings.TrimSpace(string(body))
	}
	return string(data)
}

// CanonicalProviderJSONSchema produces the defensive provider-facing schema
// projection. Invalid keyword members are removed instead of being sent to a
// model API, while CanonicalJSONSchema remains the loss-preserving authority
// used for execution hashes so malformed and valid contracts never collide.
func CanonicalProviderJSONSchema(body json.RawMessage) string {
	if len(body) == 0 {
		return `{"type":"object","properties":{},"additionalProperties":false}`
	}
	value, err := domainjsonstrict.DecodeValue(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 * 1024 * 1024, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil {
		return `{"type":"object","properties":{},"additionalProperties":false}`
	}
	data, err := json.Marshal(canonicalProviderJSONSchemaValue(value))
	if err != nil {
		return `{"type":"object","properties":{},"additionalProperties":false}`
	}
	return string(data)
}

func canonicalProviderJSONSchemaValue(value any) any {
	schema, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(schema))
	for key, child := range schema {
		switch key {
		case "required":
			if required := canonicalProviderStringSetArray(child); len(required) > 0 {
				out[key] = required
			}
		case "dependentRequired":
			if required := canonicalProviderDependentRequired(child); len(required) > 0 {
				out[key] = required
			}
		case "dependencies":
			if dependencies, valid := canonicalProviderDependencies(child); valid && len(dependencies) > 0 {
				out[key] = dependencies
			} else if !valid {
				out[key] = child
			}
		case "type", "enum":
			out[key] = canonicalSchemaScalarSet(child)
		case "properties":
			out[key] = canonicalProviderSchemaProperties(child)
		case "items", "additionalProperties":
			if _, nested := child.(map[string]any); nested {
				out[key] = canonicalProviderJSONSchemaValue(child)
			} else {
				out[key] = child
			}
		case "anyOf", "oneOf", "allOf":
			out[key] = canonicalProviderSchemaBranches(child)
		default:
			out[key] = child
		}
	}
	return out
}

func canonicalProviderSchemaProperties(value any) any {
	properties, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(properties))
	for name, schema := range properties {
		out[name] = canonicalProviderJSONSchemaValue(schema)
	}
	return out
}

func canonicalProviderSchemaBranches(value any) any {
	branches, ok := value.([]any)
	if !ok {
		return value
	}
	out := make([]any, 0, len(branches))
	for _, branch := range branches {
		out = append(out, canonicalProviderJSONSchemaValue(branch))
	}
	return out
}

func canonicalProviderDependentRequired(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(typed))
	for key, child := range typed {
		if required := canonicalProviderStringSetArray(child); len(required) > 0 {
			out[key] = required
		}
	}
	return out
}

func canonicalProviderDependencies(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]any, len(typed))
	for key, child := range typed {
		switch dependency := child.(type) {
		case []any:
			required := canonicalProviderStringSetArray(dependency)
			if len(required) > 0 {
				out[key] = required
			}
		case map[string]any:
			out[key] = canonicalProviderJSONSchemaValue(dependency)
		case bool:
			out[key] = dependency
		default:
			return nil, false
		}
	}
	return out, true
}

func canonicalProviderStringSetArray(value any) []any {
	typed, ok := value.([]any)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	out := make([]any, 0, len(typed))
	for _, child := range typed {
		text, ok := child.(string)
		if !ok || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].(string) < out[j].(string) })
	return out
}

func canonicalJSONSchemaValue(value any) any {
	schema, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(schema))
	for key, child := range schema {
		switch key {
		case "required":
			if required, valid := canonicalStringSetArray(child); !valid {
				out[key] = child
			} else if len(required) > 0 {
				out[key] = required
			}
		case "dependentRequired":
			if dependentRequired, valid := canonicalDependentRequired(child); !valid {
				out[key] = child
			} else if len(dependentRequired) > 0 {
				out[key] = dependentRequired
			}
		case "dependencies":
			if dependencies, valid := canonicalDependencies(child); !valid {
				out[key] = child
			} else if len(dependencies) > 0 {
				out[key] = dependencies
			}
		case "type", "enum":
			out[key] = canonicalSchemaScalarSet(child)
		case "properties":
			out[key] = canonicalSchemaProperties(child)
		case "items", "additionalProperties":
			if _, nested := child.(map[string]any); nested {
				out[key] = canonicalJSONSchemaValue(child)
			} else {
				out[key] = child
			}
		case "anyOf", "oneOf", "allOf":
			out[key] = canonicalSchemaBranches(child)
		default:
			// const, default, and examples are instance data. Unknown keys are
			// retained byte-semantically rather than recursively interpreted as
			// schema, which also keeps property names such as "required" safe.
			out[key] = child
		}
	}
	return out
}

func canonicalSchemaProperties(value any) any {
	properties, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(properties))
	for name, schema := range properties {
		out[name] = canonicalJSONSchemaValue(schema)
	}
	return out
}

func canonicalSchemaBranches(value any) any {
	branches, ok := value.([]any)
	if !ok {
		return value
	}
	out := make([]any, 0, len(branches))
	for _, branch := range branches {
		out = append(out, canonicalJSONSchemaValue(branch))
	}
	return out
}

func canonicalSchemaScalarSet(value any) any {
	values, ok := value.([]any)
	if !ok || !schemaArrayIsScalar(values) {
		return value
	}
	out := append([]any(nil), values...)
	sort.SliceStable(out, func(i, j int) bool {
		left, _ := json.Marshal(out[i])
		right, _ := json.Marshal(out[j])
		return string(left) < string(right)
	})
	return out
}

func canonicalDependentRequired(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]any, len(typed))
	for key, child := range typed {
		required, valid := canonicalStringSetArray(child)
		if !valid {
			return nil, false
		}
		if len(required) > 0 {
			out[key] = required
		}
	}
	return out, true
}

func canonicalDependencies(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]any, len(typed))
	for key, child := range typed {
		switch dependency := child.(type) {
		case []any:
			required, valid := canonicalStringSetArray(dependency)
			if !valid {
				return nil, false
			}
			if len(required) > 0 {
				out[key] = required
			}
		case map[string]any:
			out[key] = canonicalJSONSchemaValue(dependency)
		case bool:
			out[key] = dependency
		default:
			return nil, false
		}
	}
	return out, true
}

func canonicalStringSetArray(value any) ([]any, bool) {
	typed, ok := value.([]any)
	if !ok {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]any, 0, len(typed))
	for _, child := range typed {
		text, ok := child.(string)
		if !ok {
			return nil, false
		}
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].(string) < out[j].(string)
	})
	return out, true
}

func schemaArrayIsScalar(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case string, json.Number, float64, bool, nil:
			continue
		default:
			return false
		}
	}
	return true
}
