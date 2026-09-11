package filetools

import "testing"

func TestValidateReadBeforeEditMissingAndStaleReads(t *testing.T) {
	missing, blocked := ValidateReadBeforeEdit(ReadBeforeChangeInput{
		TurnID:       "turn-1",
		Path:         "/workspace/a.txt",
		RelativePath: "a.txt",
		Args:         map[string]any{"oldText": "a", "newText": "b"},
	})
	if !blocked || missing["code"] != "read_before_edit_required" || missing["relative_path"] != "a.txt" {
		t.Fatalf("missing read should block edit: %#v", missing)
	}
	if missing["error"] != "read-before-edit guard blocked edit for a.txt. Read the current file contents in this turn before editing." {
		t.Fatalf("missing read message mismatch: %#v", missing)
	}

	stale, blocked := ValidateReadBeforeEdit(ReadBeforeChangeInput{
		TurnID: "turn-2",
		Path:   "/workspace/a.txt",
		Args:   map[string]any{"oldText": "a", "newText": "b"},
		Record: ReadRecord{
			TurnID:       "turn-1",
			RelativePath: "a.txt",
			Content:      "a\n",
		},
		HasRecord: true,
	})
	if !blocked || stale["error"] != "read-before-edit guard requires a fresh read in the current turn before editing a.txt." {
		t.Fatalf("stale read should block edit: %#v", stale)
	}
}

func TestValidateReadBeforeEditChecksLatestReadContent(t *testing.T) {
	output, blocked := ValidateReadBeforeEdit(ReadBeforeChangeInput{
		TurnID: "turn-1",
		Path:   "/workspace/a.txt",
		Args:   map[string]any{"oldText": "missing", "newText": "b"},
		Record: ReadRecord{
			TurnID:       "turn-1",
			RelativePath: "a.txt",
			Content:      "a\n",
		},
		HasRecord: true,
	})
	if !blocked || output["cause_code"] != "old_text_not_found" {
		t.Fatalf("edit mismatch should block with cause code: %#v", output)
	}

	output, blocked = ValidateReadBeforeEdit(ReadBeforeChangeInput{
		TurnID: "turn-1",
		Path:   "/workspace/a.txt",
		Args:   map[string]any{"oldText": "a", "newText": "b"},
		Record: ReadRecord{
			TurnID:       "turn-1",
			RelativePath: "a.txt",
			Content:      "a\n",
		},
		HasRecord: true,
	})
	if blocked || output != nil {
		t.Fatalf("matching edit should pass: %#v", output)
	}
}

func TestValidateReadBeforeDeleteRangeAndSymbol(t *testing.T) {
	rangeOutput, blocked := ValidateReadBeforeDeleteRange(ReadBeforeChangeInput{
		TurnID: "turn-1",
		Path:   "/workspace/a.txt",
		Args:   map[string]any{"start_anchor": "missing", "end_anchor": "end"},
		Record: ReadRecord{
			TurnID:       "turn-1",
			RelativePath: "a.txt",
			Content:      "start\nend\n",
		},
		HasRecord: true,
	})
	if !blocked || rangeOutput["cause_code"] != "anchor_not_found" {
		t.Fatalf("delete_range mismatch should block with cause code: %#v", rangeOutput)
	}

	symbolOutput, blocked := ValidateReadBeforeDeleteSymbol(ReadBeforeChangeInput{
		TurnID:       "turn-1",
		Path:         "/workspace/a.txt",
		RelativePath: "a.txt",
		Args:         map[string]any{"name": "Target"},
	})
	if !blocked || symbolOutput["code"] != "unsupported_file_type" {
		t.Fatalf("delete_symbol non-Go file should block: %#v", symbolOutput)
	}
}

func TestToolErrorOutput(t *testing.T) {
	output := ToolErrorOutput(ToolError{Code: "validation_error", Message: "bad args"})
	if output["code"] != "validation_error" || output["error"] != "bad args" {
		t.Fatalf("tool error output mismatch: %#v", output)
	}
}
