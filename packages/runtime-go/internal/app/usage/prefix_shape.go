package usage

import (
	"encoding/json"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func PrefixShapeFromCacheDiagnostics(diagnostics map[string]any) domainmodel.PrefixShape {
	shape := domainmodel.PrefixShape{
		SystemHash: stringField(diagnostics, "systemHash"), ToolsHash: stringField(diagnostics, "toolsHash"),
		PrefixHash: stringField(diagnostics, "prefixHash"), PrefixItemsHash: stringField(diagnostics, "prefixItemsHash"),
		ToolSchemaTokens: intField(diagnostics, "toolSchemaTokens"), ToolCount: intField(diagnostics, "toolCount"),
		ToolSourcesHash: stringField(diagnostics, "toolSourcesHash"), ToolSourceIDs: stringListField(diagnostics, "toolSourceIds"),
		Route: stringField(diagnostics, "route"), Provider: stringField(diagnostics, "provider"),
		ProviderID: stringField(diagnostics, "providerId"), EndpointFormat: stringField(diagnostics, "endpointFormat"), Model: stringField(diagnostics, "model"),
	}
	if shape.PrefixHash == "" {
		return domainmodel.PrefixShape{}
	}
	return shape
}

func intField(record map[string]any, key string) int {
	switch value := record[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err == nil && int64(int(parsed)) == parsed {
			return int(parsed)
		}
		return 0
	default:
		return 0
	}
}

func stringListField(record map[string]any, key string) []string {
	values, ok := record[key].([]any)
	if !ok {
		if value := strings.TrimSpace(stringField(record, key)); value != "" {
			return []string{value}
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}
