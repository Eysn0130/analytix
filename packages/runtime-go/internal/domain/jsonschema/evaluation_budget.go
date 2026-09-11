package jsonschema

import (
	"errors"
	"strconv"
	"strings"
)

const (
	maxSchemaPhysicalNodes  = 4096
	maxSchemaPhysicalDepth  = 64
	maxSchemaEvaluationCost = 8192
	maxSchemaInstanceWork   = 1_000_000
	maxInstanceDepth        = 256
	maxInstanceNodes        = 200_000
)

// validateSchemaEvaluationBudget rejects compact schemas whose local-reference
// graph expands into disproportionate validation work. The standards engine
// remains authoritative for semantics; this is a host resource guard for
// untrusted MCP contracts.
func validateSchemaEvaluationBudget(schema map[string]any) error {
	_, err := schemaEvaluationCost(schema)
	return err
}

func schemaEvaluationCost(schema map[string]any) (int, error) {
	physicalNodes := 0
	if err := countSchemaPhysicalNodes(schema, 0, &physicalNodes); err != nil {
		return 0, err
	}
	schemaNodes := map[string]any{}
	indexSchemaNodes(schema, "#", schemaNodes)
	budget := schemaEvaluationBudget{
		schemaNodes: schemaNodes,
		memo:        map[string]int{},
		visiting:    map[string]bool{"#": true},
	}
	cost, err := budget.cost(schema, 0)
	if err != nil {
		return 0, err
	}
	if cost > maxSchemaEvaluationCost {
		return 0, errors.New("schema exceeds the deterministic evaluation budget")
	}
	return cost, nil
}

func validateSchemaInstanceBudget(schema map[string]any, instance any) error {
	cost, err := schemaEvaluationCost(schema)
	if err != nil {
		return err
	}
	nodes := 0
	if err := countInstanceNodes(instance, 0, &nodes); err != nil {
		return err
	}
	if cost > 0 && nodes > maxSchemaInstanceWork/cost {
		return errors.New("JSON value exceeds the deterministic schema evaluation work budget")
	}
	return nil
}

func countSchemaPhysicalNodes(value any, depth int, nodes *int) error {
	if depth > maxSchemaPhysicalDepth {
		return errors.New("schema exceeds the maximum nesting depth")
	}
	*nodes++
	if *nodes > maxSchemaPhysicalNodes {
		return errors.New("schema exceeds the physical node budget")
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedSchemaKeys(typed) {
			child := typed[key]
			if err := countSchemaPhysicalNodes(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := countSchemaPhysicalNodes(child, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	return nil
}

func countInstanceNodes(value any, depth int, nodes *int) error {
	if depth > maxInstanceDepth {
		return errors.New("JSON value exceeds the schema evaluation depth budget")
	}
	*nodes++
	if *nodes > maxInstanceNodes {
		return errors.New("JSON value exceeds the schema evaluation node budget")
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedSchemaKeys(typed) {
			if err := countInstanceNodes(typed[key], depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := countInstanceNodes(child, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	return nil
}

type schemaEvaluationBudget struct {
	schemaNodes map[string]any
	memo        map[string]int
	visiting    map[string]bool
}

func (b *schemaEvaluationBudget) cost(value any, depth int) (int, error) {
	if depth > maxSchemaPhysicalDepth {
		return 0, errors.New("schema exceeds the maximum evaluation depth")
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return 1, nil
	}
	cost := 1
	if values, ok := schema["enum"].([]any); ok {
		for _, value := range values {
			cost = addSchemaCost(cost, schemaLiteralCost(value, 0))
		}
	}
	if value, exists := schema["const"]; exists {
		cost = addSchemaCost(cost, schemaLiteralCost(value, 0))
	}
	if reference, exists := schema["$ref"]; exists {
		text, ok := reference.(string)
		if !ok {
			return 0, errors.New("schema $ref must be a string")
		}
		resolvedCost, err := b.referenceCost(text, depth+1)
		if err != nil {
			return 0, err
		}
		cost = addSchemaCost(cost, resolvedCost)
	}
	if _, exists := schema["$dynamicRef"]; exists {
		return 0, errors.New("schema $dynamicRef is disabled for deterministic evaluation")
	}

	for _, keyword := range []string{
		"additionalProperties", "unevaluatedProperties", "items", "contains", "propertyNames", "not", "if", "then", "else",
		"contentSchema", "additionalItems", "unevaluatedItems",
	} {
		if child, exists := schema[keyword]; exists {
			childCost, err := b.cost(child, depth+1)
			if err != nil {
				return 0, err
			}
			cost = addSchemaCost(cost, childCost)
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		children, _ := schema[keyword].([]any)
		for _, child := range children {
			childCost, err := b.cost(child, depth+1)
			if err != nil {
				return 0, err
			}
			cost = addSchemaCost(cost, childCost)
		}
	}
	for _, keyword := range []string{"properties", "patternProperties", "dependentSchemas"} {
		children, _ := schema[keyword].(map[string]any)
		for _, name := range sortedSchemaKeys(children) {
			child := children[name]
			childCost, err := b.cost(child, depth+1)
			if err != nil {
				return 0, err
			}
			cost = addSchemaCost(cost, childCost)
		}
	}
	if dependencies, ok := schema["dependencies"].(map[string]any); ok {
		for _, name := range sortedSchemaKeys(dependencies) {
			child := dependencies[name]
			if _, propertyList := child.([]any); propertyList {
				continue
			}
			childCost, err := b.cost(child, depth+1)
			if err != nil {
				return 0, err
			}
			cost = addSchemaCost(cost, childCost)
		}
	}
	return cost, nil
}

func schemaLiteralCost(value any, depth int) int {
	if depth > maxSchemaPhysicalDepth {
		return maxSchemaEvaluationCost + 1
	}
	cost := 1
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedSchemaKeys(typed) {
			cost = addSchemaCost(cost, schemaLiteralCost(typed[key], depth+1))
		}
	case []any:
		for _, child := range typed {
			cost = addSchemaCost(cost, schemaLiteralCost(child, depth+1))
		}
	}
	return cost
}

func (b *schemaEvaluationBudget) referenceCost(reference string, depth int) (int, error) {
	if reference != "#" && !strings.HasPrefix(reference, "#/") {
		return 0, errors.New("schema references must use local JSON pointers")
	}
	if cost, ok := b.memo[reference]; ok {
		return cost, nil
	}
	if b.visiting[reference] {
		return 0, errors.New("recursive schema references are disabled for deterministic evaluation")
	}
	target, allowedTarget := b.schemaNodes[reference]
	if !allowedTarget {
		return 0, errors.New("schema local reference must target a reviewed schema node")
	}
	b.visiting[reference] = true
	cost, err := b.cost(target, depth+1)
	delete(b.visiting, reference)
	if err != nil {
		return 0, err
	}
	b.memo[reference] = cost
	return cost, nil
}

func indexSchemaNodes(value any, pointer string, nodes map[string]any) {
	switch schema := value.(type) {
	case bool:
		nodes[pointer] = schema
		return
	case map[string]any:
		nodes[pointer] = schema
		for _, keyword := range []string{
			"additionalProperties", "unevaluatedProperties", "items", "contains", "propertyNames", "not", "if", "then", "else",
			"contentSchema", "additionalItems", "unevaluatedItems",
		} {
			if child, exists := schema[keyword]; exists {
				indexSchemaNodes(child, pointer+"/"+schemaPointerToken(keyword), nodes)
			}
		}
		for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
			children, _ := schema[keyword].([]any)
			for index, child := range children {
				indexSchemaNodes(child, pointer+"/"+schemaPointerToken(keyword)+"/"+strconv.Itoa(index), nodes)
			}
		}
		for _, keyword := range []string{"properties", "patternProperties", "dependentSchemas", "$defs", "definitions"} {
			children, _ := schema[keyword].(map[string]any)
			for _, name := range sortedSchemaKeys(children) {
				child := children[name]
				indexSchemaNodes(child, pointer+"/"+schemaPointerToken(keyword)+"/"+schemaPointerToken(name), nodes)
			}
		}
		if dependencies, ok := schema["dependencies"].(map[string]any); ok {
			for _, name := range sortedSchemaKeys(dependencies) {
				child := dependencies[name]
				if _, propertyList := child.([]any); propertyList {
					continue
				}
				indexSchemaNodes(child, pointer+"/dependencies/"+schemaPointerToken(name), nodes)
			}
		}
	}
}

func schemaPointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func addSchemaCost(left int, right int) int {
	if left > maxSchemaEvaluationCost || right > maxSchemaEvaluationCost-left {
		return maxSchemaEvaluationCost + 1
	}
	return left + right
}
