package jsonschema

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	standardschema "github.com/santhosh-tekuri/jsonschema/v5"
)

const inMemorySchemaURL = "https://analytix.invalid/runtime-tool-schema.json"
const (
	maxCompiledSchemaCacheEntries = 64
	maxCompiledSchemaCacheBytes   = 8 * 1024 * 1024
	maxCachedSchemaBytes          = 256 * 1024
	maxSchemaCompositionBranches  = 128
)

type compiledSchemaCacheEntry struct {
	key      [sha256.Size]byte
	compiled *standardschema.Schema
	size     int
}

var compiledSchemaCache = struct {
	sync.Mutex
	entries map[[sha256.Size]byte]*list.Element
	order   list.List
	bytes   int
}{entries: map[[sha256.Size]byte]*list.Element{}}

// ValidateStandardValue compiles and validates with a standards-compliant
// draft-07/2020-12 implementation. External references are deliberately
// disabled: MCP schemas are untrusted, self-contained contracts and must not
// cause network or filesystem reads during advertisement or execution.
func ValidateStandardValue(schemaValue any, instance any) error {
	if schema, ok := schemaValue.(map[string]any); ok {
		if err := validateSchemaSecurityPolicy(schema, "$", DefinitionOptions{}); err != nil {
			return err
		}
		if err := validateSchemaInstanceBudget(schema, instance); err != nil {
			return err
		}
	}
	compiled, err := compileStandardSchema(schemaValue)
	if err != nil {
		return err
	}
	if err := compiled.Validate(instance); err != nil {
		var validation *standardschema.ValidationError
		if errors.As(err, &validation) {
			location := deepestInstanceLocation(validation)
			if location == "" {
				location = "/"
			}
			return errors.New("JSON value violates schema at " + location)
		}
		return errors.New("JSON value violates schema")
	}
	return nil
}

func validateStandardDefinition(schemaValue any) error {
	_, err := compileStandardSchema(schemaValue)
	return err
}

func compileStandardSchema(schemaValue any) (*standardschema.Schema, error) {
	body, err := json.Marshal(schemaValue)
	if err != nil {
		return nil, errors.New("schema cannot be encoded")
	}
	cacheKey := sha256.Sum256(body)
	compiledSchemaCache.Lock()
	if element := compiledSchemaCache.entries[cacheKey]; element != nil {
		compiledSchemaCache.order.MoveToBack(element)
		cached := element.Value.(*compiledSchemaCacheEntry).compiled
		compiledSchemaCache.Unlock()
		return cached, nil
	}
	compiledSchemaCache.Unlock()
	compiler := standardschema.NewCompiler()
	compiler.Draft = standardschema.Draft2020
	compiler.AssertFormat = true
	compiler.AssertContent = true
	compiler.LoadURL = func(string) (io.ReadCloser, error) {
		return nil, errors.New("external schema references are disabled")
	}
	if err := compiler.AddResource(inMemorySchemaURL, bytes.NewReader(body)); err != nil {
		return nil, errors.New("schema is not valid JSON Schema")
	}
	compiled, err := compiler.Compile(inMemorySchemaURL)
	if err != nil {
		return nil, errors.New("schema is invalid or requires an external reference")
	}
	if len(body) <= maxCachedSchemaBytes {
		compiledSchemaCache.Lock()
		if element := compiledSchemaCache.entries[cacheKey]; element != nil {
			compiledSchemaCache.order.MoveToBack(element)
			cached := element.Value.(*compiledSchemaCacheEntry).compiled
			compiledSchemaCache.Unlock()
			return cached, nil
		}
		for compiledSchemaCache.order.Len() >= maxCompiledSchemaCacheEntries || compiledSchemaCache.bytes+len(body) > maxCompiledSchemaCacheBytes {
			oldest := compiledSchemaCache.order.Front()
			if oldest == nil {
				break
			}
			entry := oldest.Value.(*compiledSchemaCacheEntry)
			compiledSchemaCache.bytes -= entry.size
			delete(compiledSchemaCache.entries, entry.key)
			compiledSchemaCache.order.Remove(oldest)
		}
		entry := &compiledSchemaCacheEntry{key: cacheKey, compiled: compiled, size: len(body)}
		element := compiledSchemaCache.order.PushBack(entry)
		compiledSchemaCache.entries[cacheKey] = element
		compiledSchemaCache.bytes += len(body)
		compiledSchemaCache.Unlock()
	}
	return compiled, nil
}

func deepestInstanceLocation(validation *standardschema.ValidationError) string {
	if validation == nil {
		return ""
	}
	locations := []string{}
	var collect func(*standardschema.ValidationError)
	collect = func(current *standardschema.ValidationError) {
		if current == nil {
			return
		}
		if location := strings.TrimSpace(current.InstanceLocation); location != "" {
			locations = append(locations, location)
		}
		for _, cause := range current.Causes {
			collect(cause)
		}
	}
	collect(validation)
	sort.Slice(locations, func(left int, right int) bool {
		leftDepth := strings.Count(strings.Trim(locations[left], "/"), "/")
		rightDepth := strings.Count(strings.Trim(locations[right], "/"), "/")
		if locations[left] != "/" && locations[left] != "" {
			leftDepth++
		}
		if locations[right] != "/" && locations[right] != "" {
			rightDepth++
		}
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return locations[left] < locations[right]
	})
	if len(locations) == 0 {
		return ""
	}
	return locations[0]
}

func validateSchemaSecurityPolicy(schema map[string]any, path string, options DefinitionOptions) error {
	dialect, err := schemaDialect(schema, "2020-12")
	if err != nil {
		return err
	}
	return validateSchemaSecurityPolicyDialect(schema, path, options, dialect)
}

func validateSchemaSecurityPolicyDialect(schema map[string]any, path string, options DefinitionOptions, inheritedDialect string) error {
	dialect, err := schemaDialect(schema, inheritedDialect)
	if err != nil {
		return err
	}
	for _, keyword := range sortedSchemaKeys(schema) {
		if !standardKeyword(keyword, dialect) {
			return errors.New(path + " uses an unknown JSON Schema keyword")
		}
	}
	if _, exists := schema["$id"]; exists {
		return errors.New(path + " cannot declare $id in an untrusted runtime schema")
	}
	if hasAny(schema, "contains", "minContains", "maxContains") {
		return errors.New(path + " cannot use contains in an untrusted runtime schema")
	}
	if hasAny(schema, "contentEncoding", "contentMediaType", "contentSchema") {
		return errors.New(path + " cannot use content decoding in an untrusted runtime schema")
	}
	if hasAny(schema, "patternProperties", "propertyNames") {
		return errors.New(path + " cannot use dynamic property-name evaluation in an untrusted runtime schema")
	}
	if _, hasType := schema["type"]; !hasType && !hasAny(schema, "$ref", "enum", "const", "allOf", "anyOf", "oneOf") {
		return errors.New(path + " must declare a type or a bounded schema construct")
	}
	objectSchema := allowsType(schema["type"], "object") || hasAny(schema,
		"properties", "patternProperties", "required", "additionalProperties", "unevaluatedProperties",
		"dependentRequired", "dependentSchemas", "dependencies", "propertyNames", "minProperties", "maxProperties")
	if hasAny(schema, "properties", "patternProperties", "required", "additionalProperties", "unevaluatedProperties",
		"dependentRequired", "dependentSchemas", "dependencies", "propertyNames", "minProperties", "maxProperties") &&
		!allowsType(schema["type"], "object") {
		return errors.New(path + " uses object keywords without object type")
	}
	if hasAny(schema, "items", "prefixItems", "contains", "unevaluatedItems", "minItems", "maxItems", "uniqueItems", "minContains", "maxContains") &&
		!allowsType(schema["type"], "array") {
		return errors.New(path + " uses array keywords without array type")
	}
	if hasAny(schema, "minLength", "maxLength", "pattern", "format", "contentEncoding", "contentMediaType", "contentSchema") &&
		!allowsType(schema["type"], "string") {
		return errors.New(path + " uses string keywords without string type")
	}
	if hasAny(schema, "multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum") &&
		!allowsAnyType(schema["type"], "number", "integer") {
		return errors.New(path + " uses numeric keywords without numeric type")
	}
	if objectSchema {
		additional, present := schema["additionalProperties"]
		if options.RequireExplicitClosedObjects && (!present || additional != false) {
			return errors.New(path + ".additionalProperties must be explicitly false")
		}
		if options.RequireClosedObjects && present && additional != false {
			return errors.New(path + ".additionalProperties must be false")
		}
	}
	if options.RequireCompleteCollections && allowsType(schema["type"], "array") {
		if _, hasItems := schema["items"]; !hasItems {
			return errors.New(path + ".items is required to constrain array tails")
		}
	}
	if required, exists := schema["required"].([]any); exists {
		properties, _ := schema["properties"].(map[string]any)
		for _, value := range required {
			name, ok := value.(string)
			if !ok {
				return errors.New(path + ".required must contain exact property names")
			}
			if _, present := properties[name]; !present {
				return errors.New(path + ".required references an unknown property")
			}
		}
	}
	for _, keyword := range []string{
		"additionalProperties", "unevaluatedProperties", "items", "contains", "propertyNames", "not", "if", "then", "else",
		"contentSchema", "additionalItems", "unevaluatedItems",
	} {
		if child, exists := schema[keyword]; exists {
			if err := validateSchemaChild(child, path+"."+keyword, options, dialect); err != nil {
				return err
			}
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if values, exists := schema[keyword].([]any); exists {
			if keyword != "prefixItems" && len(values) > maxSchemaCompositionBranches {
				return errors.New(path + "." + keyword + " exceeds the composition branch budget")
			}
			for index, child := range values {
				if err := validateSchemaChild(child, path+"."+keyword+"["+jsonIndex(index)+"]", options, dialect); err != nil {
					return err
				}
			}
		}
	}
	for _, keyword := range []string{"properties", "patternProperties", "dependentSchemas", "$defs", "definitions"} {
		if values, exists := schema[keyword].(map[string]any); exists {
			for _, name := range sortedSchemaKeys(values) {
				child := values[name]
				if err := validateSchemaChild(child, path+"."+keyword+"."+name, options, dialect); err != nil {
					return err
				}
			}
		}
	}
	if dependencies, exists := schema["dependencies"].(map[string]any); exists {
		for _, name := range sortedSchemaKeys(dependencies) {
			child := dependencies[name]
			if _, isList := child.([]any); isList {
				continue
			}
			if err := validateSchemaChild(child, path+".dependencies."+name, options, dialect); err != nil {
				return err
			}
		}
	}
	return nil
}

func sortedSchemaKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateSchemaChild(value any, path string, options DefinitionOptions, dialect string) error {
	if boolean, ok := value.(bool); ok {
		if options.RequireClosedObjects && boolean {
			return errors.New(path + " cannot use an unconstrained true schema")
		}
		return nil
	}
	child, ok := value.(map[string]any)
	if !ok {
		return errors.New(path + " must be a schema")
	}
	return validateSchemaSecurityPolicyDialect(child, path, options, dialect)
}

func standardKeyword(keyword string, dialect string) bool {
	switch keyword {
	case "$schema", "$id", "$ref", "$anchor", "$dynamicRef", "$dynamicAnchor", "$defs", "$vocabulary", "$comment", "definitions",
		"type", "enum", "const", "multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum",
		"maxLength", "minLength", "pattern", "format", "contentEncoding", "contentMediaType", "contentSchema",
		"maxItems", "minItems", "uniqueItems", "maxContains", "minContains", "maxProperties", "minProperties", "required",
		"dependentRequired", "dependencies", "properties", "patternProperties", "additionalProperties", "unevaluatedProperties",
		"dependentSchemas", "propertyNames", "prefixItems", "items", "additionalItems", "contains", "unevaluatedItems",
		"allOf", "anyOf", "oneOf", "not", "if", "then", "else",
		"title", "description", "default", "deprecated", "readOnly", "writeOnly", "examples":
		switch dialect {
		case "draft-07":
			switch keyword {
			case "$anchor", "$dynamicRef", "$dynamicAnchor", "$defs", "$vocabulary", "prefixItems", "unevaluatedItems",
				"unevaluatedProperties", "dependentRequired", "dependentSchemas", "minContains", "maxContains", "contentSchema",
				"deprecated":
				return false
			}
		case "2020-12":
			switch keyword {
			case "definitions", "dependencies", "additionalItems":
				return false
			}
		default:
			return false
		}
		return true
	default:
		return false
	}
}

func schemaDialect(schema map[string]any, fallback string) (string, error) {
	value, exists := schema["$schema"]
	if !exists {
		return fallback, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", errors.New("$schema must be a string")
	}
	switch strings.TrimSpace(text) {
	case "http://json-schema.org/draft-07/schema#", "https://json-schema.org/draft-07/schema#":
		return "draft-07", nil
	case "https://json-schema.org/draft/2020-12/schema", "https://json-schema.org/draft/2020-12/schema#":
		return "2020-12", nil
	default:
		return "", errors.New("unsupported JSON Schema dialect")
	}
}

func jsonIndex(value int) string {
	return strconv.Itoa(value)
}
