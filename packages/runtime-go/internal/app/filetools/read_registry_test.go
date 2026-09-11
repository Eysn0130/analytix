package filetools

import "testing"

func TestReadRegistryRecordInputAndClear(t *testing.T) {
	registry := NewReadRegistry()
	registry.Record(ReadRegistryRecordInput{
		ThreadID: "thread",
		Key:      "/workspace/file.go",
		Record: ReadRecord{
			TurnID:       "turn",
			Path:         "/workspace/file.go",
			RelativePath: "file.go",
			Content:      "package main\n",
		},
	})
	input := registry.Input(ReadRegistryInputQuery{
		ThreadID:     "thread",
		TurnID:       "turn",
		Key:          "/workspace/file.go",
		Path:         "/workspace/file.go",
		RelativePath: "file.go",
		IsGoFile:     true,
	})
	if !input.HasRecord || input.Record.Content != "package main\n" || !input.IsGoFile {
		t.Fatalf("input mismatch: %#v", input)
	}
	registry.Clear("thread")
	input = registry.Input(ReadRegistryInputQuery{
		ThreadID: "thread",
		TurnID:   "turn",
		Key:      "/workspace/file.go",
	})
	if input.HasRecord {
		t.Fatalf("record should be cleared: %#v", input)
	}
}
