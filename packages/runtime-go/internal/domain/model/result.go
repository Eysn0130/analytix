package model

import "strings"

func ResultText(result Result) string {
	parts := []string{}
	for _, chunk := range result.Chunks {
		if chunk.Kind == ChunkText && strings.TrimSpace(chunk.Text) != "" {
			parts = append(parts, strings.TrimSpace(chunk.Text))
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}
