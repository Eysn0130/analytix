package loop

import (
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func ProviderResultHasVisibleOutput(result domainmodel.Result) bool {
	for _, chunk := range result.Chunks {
		switch chunk.Kind {
		case domainmodel.ChunkText, domainmodel.ChunkReasoning:
			if strings.TrimSpace(chunk.Text) != "" {
				return true
			}
		case domainmodel.ChunkToolCall, domainmodel.ChunkToolCallStart:
			if strings.TrimSpace(chunk.ToolCall.Name) != "" {
				return true
			}
		}
	}
	return false
}

func ToolCallCount(chunks []domainmodel.Chunk) int {
	count := 0
	for _, chunk := range chunks {
		if chunk.Kind == domainmodel.ChunkToolCallStart || chunk.Kind == domainmodel.ChunkToolCall {
			count++
		}
	}
	return count
}
