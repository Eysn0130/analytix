package loop

import (
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestProviderResultHasVisibleOutput(t *testing.T) {
	cases := []struct {
		name   string
		result domainmodel.Result
		want   bool
	}{
		{name: "empty", result: domainmodel.Result{}, want: false},
		{name: "blank text", result: domainmodel.Result{Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: " "}}}, want: false},
		{name: "visible text", result: domainmodel.Result{Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "hello"}}}, want: true},
		{name: "reasoning", result: domainmodel.Result{Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkReasoning, Text: "thinking"}}}, want: true},
		{name: "tool start", result: domainmodel.Result{Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{Name: "grep"}}}}, want: true},
		{name: "blank tool", result: domainmodel.Result{Chunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{Name: " "}}}}, want: false},
	}
	for _, tc := range cases {
		if got := ProviderResultHasVisibleOutput(tc.result); got != tc.want {
			t.Fatalf("%s visible output got %v want %v", tc.name, got, tc.want)
		}
	}
}
