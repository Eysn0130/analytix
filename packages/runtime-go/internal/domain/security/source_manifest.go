package security

import (
	"encoding/json"
	"sort"
	"strings"
)

// SourceManifestHash seals the host-observed connected source catalog. Probe
// state is deliberately excluded to avoid a context/probe digest cycle.
func SourceManifestHash(diagnostics []any) string {
	type sourceRecord struct {
		ID                     string   `json:"id"`
		CatalogFingerprint     string   `json:"catalogFingerprint"`
		SpecFingerprint        string   `json:"specFingerprint"`
		ConnectionEpoch        uint64   `json:"connectionEpoch"`
		ExpectedServerName     string   `json:"expectedServerName"`
		ExpectedServerVersion  string   `json:"expectedServerVersion"`
		VerifiedServerIdentity string   `json:"verifiedServerIdentity"`
		IdentitySource         string   `json:"identitySource"`
		ToolNames              []string `json:"toolNames"`
	}
	records := []sourceRecord{}
	for _, raw := range diagnostics {
		record, ok := raw.(map[string]any)
		if !ok || !manifestBoolField(record, "connected") || !manifestBoolField(record, "available") {
			continue
		}
		toolNames := manifestStringList(record["toolNames"])
		sort.Strings(toolNames)
		records = append(records, sourceRecord{
			ID: manifestStringField(record, "id"), CatalogFingerprint: manifestStringField(record, "catalogFingerprint"),
			SpecFingerprint: manifestStringField(record, "specFingerprint"), ConnectionEpoch: manifestUint64Field(record, "connectionEpoch"),
			ExpectedServerName: manifestStringField(record, "expectedServerName"), ExpectedServerVersion: manifestStringField(record, "expectedServerVersion"),
			VerifiedServerIdentity: manifestStringField(record, "verifiedServerIdentity"),
			IdentitySource:         manifestStringField(record, "identitySource"), ToolNames: toolNames,
		})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	body, _ := json.Marshal(records)
	return SHA256Hex(body)
}

func manifestBoolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func manifestStringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func manifestStringList(value any) []string {
	raw, _ := value.([]any)
	if typed, ok := value.([]string); ok {
		raw = make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			values = append(values, strings.TrimSpace(text))
		}
	}
	return values
}

func manifestUint64Field(record map[string]any, key string) uint64 {
	switch value := record[key].(type) {
	case float64:
		if value > 0 && value == float64(uint64(value)) {
			return uint64(value)
		}
	case json.Number:
		parsed, err := value.Int64()
		if err == nil && parsed > 0 {
			return uint64(parsed)
		}
	}
	return 0
}
