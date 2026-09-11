package filetools

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNotebookEditReplaceByCellIDNormalizesMarkdown(t *testing.T) {
	request, err := ParseNotebookEditRequest(map[string]any{
		"path":       "analysis.ipynb",
		"cell_id":    "intro",
		"new_source": "# Updated\nbody\n",
		"cell_type":  "markdown",
	})
	if err != nil {
		t.Fatalf("parse request: %v", err)
	}
	before := `{
 "cells": [
  {
   "cell_type": "code",
   "id": "intro",
   "metadata": {},
   "source": ["print(1)\n"],
   "outputs": [{"name":"stdout"}],
   "execution_count": 1
  }
 ],
 "metadata": {},
 "nbformat": 4,
 "nbformat_minor": 5
}`
	preview, err := PreviewNotebookEdit(before, request)
	if err != nil {
		t.Fatalf("preview edit: %v", err)
	}
	if preview.CellIndex != 0 || preview.CellCount != 1 || preview.CellType != "markdown" {
		t.Fatalf("unexpected preview metadata: %+v", preview)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(preview.After), &decoded); err != nil {
		t.Fatalf("after is not valid json: %v", err)
	}
	cell := decoded["cells"].([]any)[0].(map[string]any)
	if got := cell["cell_type"]; got != "markdown" {
		t.Fatalf("cell_type = %v", got)
	}
	if _, ok := cell["outputs"]; ok {
		t.Fatalf("markdown cell retained outputs: %#v", cell)
	}
	source := cell["source"].([]any)
	if len(source) != 2 || source[0] != "# Updated\n" || source[1] != "body\n" {
		t.Fatalf("unexpected source lines: %#v", source)
	}
}

func TestNotebookEditInsertOutOfRangeReturnsStructuredError(t *testing.T) {
	index := 4
	_, err := PreviewNotebookEdit(`{"cells":[]}`, NotebookEditRequest{
		Path:      "analysis.ipynb",
		CellIndex: &index,
		NewSource: "print(1)\n",
		CellType:  "code",
		EditMode:  "insert",
	})
	var toolErr ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != "cell_out_of_range" || !strings.Contains(toolErr.Message, "notebook has 0 cells") {
		t.Fatalf("unexpected ToolError: %+v", toolErr)
	}
}
