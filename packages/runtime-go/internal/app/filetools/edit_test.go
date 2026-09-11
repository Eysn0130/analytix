package filetools

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyExactTextEditsRejectsAmbiguousSingleReplace(t *testing.T) {
	edits, err := ParseEditInstructions(map[string]any{
		"oldText": "alpha",
		"newText": "omega",
	})
	if err != nil {
		t.Fatalf("parse edits: %v", err)
	}
	_, _, err = ApplyExactTextEdits("alpha\nalpha\n", edits)
	var toolErr ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != "ambiguous_old_text" {
		t.Fatalf("code = %s", toolErr.Code)
	}
}

func TestPreviewDeleteRangePreservesCRLFAndExclusiveBounds(t *testing.T) {
	preview, err := PreviewDeleteRange("start\r\nremove\r\nend\r\nkeep\r\n", map[string]any{
		"start_anchor": "start",
		"end_anchor":   "end",
		"inclusive":    false,
	})
	if err != nil {
		t.Fatalf("preview delete range: %v", err)
	}
	if preview.After != "start\r\nend\r\nkeep\r\n" {
		t.Fatalf("after = %q", preview.After)
	}
	if preview.FirstDeletedLine != 1 || preview.DeletedLines != 1 || preview.Inclusive {
		t.Fatalf("unexpected preview metadata: %+v", preview)
	}
}

func TestPreviewDeleteRangeReportsDuplicateAnchor(t *testing.T) {
	_, err := PreviewDeleteRange("same\nsame\nend\n", map[string]any{
		"start_anchor": "same",
		"end_anchor":   "end",
	})
	var toolErr ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Code != "anchor_not_unique" || !strings.Contains(toolErr.Message, "start_anchor") {
		t.Fatalf("unexpected ToolError: %+v", toolErr)
	}
}

func TestBuildMutationToolOutputs(t *testing.T) {
	write := BuildWriteToolOutput(WriteToolOutputInput{
		Path:         "/work/new.txt",
		RelativePath: "new.txt",
		Content:      "hello\n",
		BytesWritten: 6,
		Encoding:     TextEncodingUTF8,
		Before:       "",
		IncludeDiff:  true,
		DiffKind:     DiffCreate,
	})
	if write["bytes"] != float64(6) || write["diff_kind"] != "create" || write["diff"] == nil {
		t.Fatalf("write output mismatch: %#v", write)
	}

	edit := BuildEditToolOutput(EditToolOutputInput{
		Path:         "/work/a.txt",
		RelativePath: "a.txt",
		Replacements: 1,
		BytesWritten: 6,
		Encoding:     TextEncodingUTF8,
		Before:       "hello\n",
		After:        "world\n",
	})
	if edit["replacements"] != float64(1) || edit["first_changed_line"] != float64(1) || edit["diff"] == nil {
		t.Fatalf("edit output mismatch: %#v", edit)
	}

	move := BuildMoveToolOutput(MoveToolOutputInput{
		SourcePath:              "/work/a.txt",
		DestinationPath:         "/work/b.txt",
		SourceRelativePath:      "a.txt",
		DestinationRelativePath: "b.txt",
		BytesMoved:              12,
		Moved:                   true,
	})
	if move["moved"] != true || move["message"] != "moved a.txt to b.txt" {
		t.Fatalf("move output mismatch: %#v", move)
	}

	notebook := BuildNotebookEditToolOutput(NotebookEditToolOutputInput{
		Path:         "/work/n.ipynb",
		RelativePath: "n.ipynb",
		Request:      NotebookEditRequest{EditMode: "replace", CellID: "cell-1"},
		Change:       NotebookEditPreview{After: "after", CellIndex: 0, CellCount: 1, CellType: "code", Summary: "replaced cell source"},
		BytesWritten: 5,
		Before:       "before",
	})
	if notebook["cell_id"] != "cell-1" || notebook["cell_number"] != float64(0) {
		t.Fatalf("notebook output mismatch: %#v", notebook)
	}

	deleteRange := BuildDeleteRangeToolOutput(DeleteRangeToolOutputInput{
		Path:         "/work/a.txt",
		RelativePath: "a.txt",
		Change:       DeleteRangePreview{After: "keep\n", StartLine: 0, EndLine: 1, FirstDeletedLine: 0, DeletedLines: 2, Inclusive: true},
		BytesWritten: 5,
		Encoding:     TextEncodingUTF8,
		Before:       "drop\nkeep\n",
	})
	if deleteRange["first_deleted_line"] != float64(1) || deleteRange["deleted_lines"] != float64(2) {
		t.Fatalf("delete range output mismatch: %#v", deleteRange)
	}

	deleteSymbol := BuildDeleteSymbolToolOutput(DeleteSymbolToolOutputInput{
		Path:         "/work/a.go",
		RelativePath: "a.go",
		Change: DeleteSymbolPreview{
			After:            "package demo\n",
			Match:            DeleteSymbolMatch{Name: "Run", Kind: "func", Parent: "Client", Line: 3},
			FirstDeletedLine: 3,
			DeletedLines:     2,
		},
		BytesWritten: 13,
		Encoding:     TextEncodingUTF8,
		Before:       "package demo\nfunc (Client) Run() {}\n",
	})
	if deleteSymbol["parent"] != "Client" || deleteSymbol["line"] != float64(3) {
		t.Fatalf("delete symbol output mismatch: %#v", deleteSymbol)
	}
}

func TestParseWriteToolRequest(t *testing.T) {
	request, err := ParseWriteToolRequest(map[string]any{"path": "a.txt", "content": 12})
	if err != nil {
		t.Fatalf("parse write request: %v", err)
	}
	if request.Path != "a.txt" || request.Content != "12" {
		t.Fatalf("write request mismatch: %#v", request)
	}
	if _, err := ParseWriteToolRequest(map[string]any{"content": "missing"}); err == nil {
		t.Fatal("missing path should fail")
	}
}
