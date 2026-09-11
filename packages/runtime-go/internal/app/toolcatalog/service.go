package toolcatalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type Service struct {
	builtin []domainmodel.ToolSchema
}

func NewService(builtin []domainmodel.ToolSchema) *Service {
	return &Service{builtin: append([]domainmodel.ToolSchema(nil), builtin...)}
}

func (s *Service) BuiltinTools() []domainmodel.ToolSchema {
	if s == nil {
		return nil
	}
	return append([]domainmodel.ToolSchema(nil), s.builtin...)
}

type CanonicalToolSchema struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Parameters   any    `json:"parameters"`
	OutputSchema any    `json:"outputSchema,omitempty"`
	TaskSupport  string `json:"taskSupport,omitempty"`
}

func ToolSchemaHash(tools []domainmodel.ToolSchema) string {
	canonical := CanonicalToolSchemas(tools)
	data, _ := json.Marshal(canonical)
	return domainmodel.BytesHash(data)
}

// ToolSchemaNameSetHash binds only the sorted, unique provider-visible names.
// It is diagnostic metadata, not execution authority; ToolSchemaHash remains
// the immutable admission binding for complete schemas.
func ToolSchemaNameSetHash(tools []domainmodel.ToolSchema) string {
	names := make([]string, 0, len(tools))
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	data, _ := json.Marshal(names)
	return domainmodel.BytesHash(data)
}

func CanonicalToolSchemas(tools []domainmodel.ToolSchema) []CanonicalToolSchema {
	canonical := make([]CanonicalToolSchema, 0, len(tools))
	for _, tool := range tools {
		taskSupport := ""
		if strings.TrimSpace(tool.Source) == "mcp" || strings.HasPrefix(strings.TrimSpace(tool.Name), "mcp__") {
			normalized, ok := domainmcp.NormalizeToolTaskSupport(domainmcp.ToolTaskSupport(strings.TrimSpace(tool.TaskSupport)))
			if !ok {
				taskSupport = "invalid"
			} else {
				taskSupport = string(normalized)
			}
		}
		canonical = append(canonical, CanonicalToolSchema{
			Name:         strings.TrimSpace(tool.Name),
			Description:  strings.TrimSpace(tool.Description),
			Parameters:   CanonicalToolParameters(tool.Parameters),
			OutputSchema: CanonicalToolParameters(tool.OutputSchema),
			TaskSupport:  taskSupport,
		})
	}
	sort.SliceStable(canonical, func(i, j int) bool {
		if canonical[i].Name != canonical[j].Name {
			return canonical[i].Name < canonical[j].Name
		}
		if canonical[i].Description != canonical[j].Description {
			return canonical[i].Description < canonical[j].Description
		}
		leftParameters := CanonicalToolParametersText(canonical[i].Parameters)
		rightParameters := CanonicalToolParametersText(canonical[j].Parameters)
		if leftParameters != rightParameters {
			return leftParameters < rightParameters
		}
		leftOutput := CanonicalToolParametersText(canonical[i].OutputSchema)
		rightOutput := CanonicalToolParametersText(canonical[j].OutputSchema)
		if leftOutput != rightOutput {
			return leftOutput < rightOutput
		}
		return canonical[i].TaskSupport < canonical[j].TaskSupport
	})
	return canonical
}

func CanonicalToolParameters(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	canonical := domainmodel.CanonicalJSONSchema(raw)
	if !json.Valid([]byte(canonical)) {
		return strings.TrimSpace(canonical)
	}
	return json.RawMessage(canonical)
}

func CanonicalToolParametersText(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}
