package model

import "testing"

func TestResultTextCollectsTextChunks(t *testing.T) {
	result := Result{Chunks: []Chunk{
		{Kind: ChunkReasoning, Text: "ignored"},
		{Kind: ChunkText, Text: " hello "},
		{Kind: ChunkText, Text: "world"},
		{Kind: ChunkDone},
	}}
	if got := ResultText(result); got != "helloworld" {
		t.Fatalf("result text mismatch: %q", got)
	}
}
