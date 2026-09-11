package filetools

import (
	"strings"
	"testing"
)

func TestPreviewDeleteSymbolDeletesMethodWithParent(t *testing.T) {
	content := `package sample

type Runner struct{}

func (r *Runner) Keep() {}

// Stop halts the runner.
func (r *Runner) Stop() {
	println("stop")
}
`
	preview, err := PreviewDeleteSymbol("sample.go", content, map[string]any{
		"name":   "Stop",
		"kind":   "method",
		"parent": "Runner",
	})
	if err != nil {
		t.Fatalf("preview delete symbol: %v", err)
	}
	if preview.Match.Name != "Stop" || preview.Match.Kind != "method" || preview.Match.Parent != "Runner" {
		t.Fatalf("match metadata mismatch: %#v", preview.Match)
	}
	if strings.Contains(preview.After, "Stop") || strings.Contains(preview.After, "halts the runner") {
		t.Fatalf("method and doc comment should be removed:\n%s", preview.After)
	}
	if !strings.Contains(preview.After, "Keep") || preview.FirstDeletedLine != 7 || preview.DeletedLines != 4 {
		t.Fatalf("delete range metadata mismatch: %#v after=%q", preview, preview.After)
	}
}

func TestPreviewDeleteSymbolRequiresDisambiguation(t *testing.T) {
	content := `package sample

func Target() {}

type Runner struct{}

func (r Runner) Target() {}
`
	_, err := PreviewDeleteSymbol("sample.go", content, map[string]any{"name": "Target"})
	toolErr, ok := err.(ToolError)
	if !ok || toolErr.Code != "ambiguous_symbol" {
		t.Fatalf("expected ambiguous_symbol, got %#v", err)
	}
}

func TestPreviewDeleteSymbolRejectsMultiNameSpec(t *testing.T) {
	content := `package sample

const A, B = 1, 2
`
	_, err := PreviewDeleteSymbol("sample.go", content, map[string]any{"name": "A"})
	toolErr, ok := err.(ToolError)
	if !ok || toolErr.Code != "multi_name_spec" {
		t.Fatalf("expected multi_name_spec, got %#v", err)
	}
}
