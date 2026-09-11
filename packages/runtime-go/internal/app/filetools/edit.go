package filetools

import (
	"fmt"
	"strings"
)

type EditInstruction struct {
	OldText    string
	NewText    string
	ReplaceAll bool
}

type DeleteRangePreview struct {
	After            string
	StartLine        int
	EndLine          int
	FirstDeletedLine int
	DeletedLines     int
	Inclusive        bool
}

type EditToolOutputInput struct {
	Path         string
	RelativePath string
	Replacements int
	BytesWritten int
	Encoding     string
	Before       string
	After        string
}

type WriteToolRequest struct {
	Path    string
	Content string
}

type WriteToolOutputInput struct {
	Path         string
	RelativePath string
	Content      string
	BytesWritten int
	Encoding     string
	Before       string
	IncludeDiff  bool
	DiffKind     DiffKind
}

type MoveToolOutputInput struct {
	SourcePath              string
	DestinationPath         string
	SourceRelativePath      string
	DestinationRelativePath string
	BytesMoved              int64
	Moved                   bool
}

type NotebookEditToolOutputInput struct {
	Path         string
	RelativePath string
	Request      NotebookEditRequest
	Change       NotebookEditPreview
	BytesWritten int
	Before       string
}

type DeleteRangeToolOutputInput struct {
	Path         string
	RelativePath string
	Change       DeleteRangePreview
	BytesWritten int
	Encoding     string
	Before       string
}

type DeleteSymbolToolOutputInput struct {
	Path         string
	RelativePath string
	Change       DeleteSymbolPreview
	BytesWritten int
	Encoding     string
	Before       string
}

func BuildEditToolOutput(input EditToolOutputInput) map[string]any {
	output := map[string]any{
		"path":               input.Path,
		"relative_path":      input.RelativePath,
		"replacements":       float64(input.Replacements),
		"bytes_written":      float64(input.BytesWritten),
		"encoding":           input.Encoding,
		"first_changed_line": float64(FirstChangedLine(input.Before, input.After)),
	}
	MergeDiffOutput(output, input.RelativePath, input.Before, input.After, DiffModify)
	return output
}

func ParseWriteToolRequest(args map[string]any) (WriteToolRequest, error) {
	path := strings.TrimSpace(firstNonEmptyAnyString(args["path"], args["filePath"], args["FilePath"]))
	if path == "" {
		return WriteToolRequest{}, ToolError{Code: "validation_error", Message: "path is required"}
	}
	return WriteToolRequest{Path: path, Content: fmt.Sprint(args["content"])}, nil
}

func BuildWriteToolOutput(input WriteToolOutputInput) map[string]any {
	output := map[string]any{
		"path":          input.Path,
		"relative_path": input.RelativePath,
		"bytes":         float64(len(input.Content)),
		"bytes_written": float64(input.BytesWritten),
		"encoding":      input.Encoding,
	}
	if input.IncludeDiff {
		MergeDiffOutput(output, input.RelativePath, input.Before, input.Content, input.DiffKind)
	}
	return output
}

func BuildMoveToolOutput(input MoveToolOutputInput) map[string]any {
	message := "source_path is already at destination_path; no changes made"
	if input.Moved {
		message = "moved " + input.SourceRelativePath + " to " + input.DestinationRelativePath
	}
	return map[string]any{
		"path":                      input.DestinationPath,
		"source_path":               input.SourcePath,
		"destination_path":          input.DestinationPath,
		"source_relative_path":      input.SourceRelativePath,
		"destination_relative_path": input.DestinationRelativePath,
		"bytes_moved":               float64(input.BytesMoved),
		"moved":                     input.Moved,
		"message":                   message,
	}
}

func BuildNotebookEditToolOutput(input NotebookEditToolOutputInput) map[string]any {
	output := map[string]any{
		"path":               input.Path,
		"relative_path":      input.RelativePath,
		"edit_mode":          input.Request.EditMode,
		"cell_number":        float64(input.Change.CellIndex),
		"cell_count":         float64(input.Change.CellCount),
		"cell_type":          input.Change.CellType,
		"message":            input.Change.Summary,
		"bytes_written":      float64(input.BytesWritten),
		"first_changed_line": float64(FirstChangedLine(input.Before, input.Change.After)),
	}
	if input.Request.CellID != "" {
		output["cell_id"] = input.Request.CellID
	}
	MergeDiffOutput(output, input.RelativePath, input.Before, input.Change.After, DiffModify)
	return output
}

func BuildDeleteRangeToolOutput(input DeleteRangeToolOutputInput) map[string]any {
	output := map[string]any{
		"path":               input.Path,
		"relative_path":      input.RelativePath,
		"start_line":         float64(input.Change.StartLine + 1),
		"end_line":           float64(input.Change.EndLine + 1),
		"first_deleted_line": float64(input.Change.FirstDeletedLine + 1),
		"deleted_lines":      float64(input.Change.DeletedLines),
		"bytes_written":      float64(input.BytesWritten),
		"encoding":           input.Encoding,
		"inclusive":          input.Change.Inclusive,
	}
	MergeDiffOutput(output, input.RelativePath, input.Before, input.Change.After, DiffModify)
	return output
}

func BuildDeleteSymbolToolOutput(input DeleteSymbolToolOutputInput) map[string]any {
	output := map[string]any{
		"path":               input.Path,
		"relative_path":      input.RelativePath,
		"name":               input.Change.Match.Name,
		"kind":               input.Change.Match.Kind,
		"line":               float64(input.Change.Match.Line),
		"first_deleted_line": float64(input.Change.FirstDeletedLine),
		"deleted_lines":      float64(input.Change.DeletedLines),
		"bytes_written":      float64(input.BytesWritten),
		"encoding":           input.Encoding,
	}
	if input.Change.Match.Parent != "" {
		output["parent"] = input.Change.Match.Parent
	}
	MergeDiffOutput(output, input.RelativePath, input.Before, input.Change.After, DiffModify)
	return output
}

func ParseEditInstructions(args map[string]any) ([]EditInstruction, error) {
	if raw, ok := args["edits"].([]any); ok && len(raw) > 0 {
		edits := make([]EditInstruction, 0, len(raw))
		for index, item := range raw {
			record, ok := item.(map[string]any)
			if !ok {
				return nil, ToolError{Code: "validation_error", Message: fmt.Sprintf("edits[%d] must be an object", index)}
			}
			oldText, oldOK := exactStringFromAny(record["oldText"], record["old_text"], record["old_string"])
			newText, newOK := exactStringFromAny(record["newText"], record["new_text"], record["new_string"])
			if !oldOK || !newOK {
				return nil, ToolError{Code: "validation_error", Message: fmt.Sprintf("edits[%d] requires oldText and newText", index)}
			}
			if oldText == "" {
				return nil, ToolError{Code: "validation_error", Message: fmt.Sprintf("edits[%d].oldText must not be empty", index)}
			}
			edits = append(edits, EditInstruction{
				OldText:    oldText,
				NewText:    newText,
				ReplaceAll: boolField(record, "replace_all") || boolField(record, "replaceAll"),
			})
		}
		return edits, nil
	}
	oldText, oldOK := exactStringFromAny(args["oldText"], args["old_text"], args["old_string"])
	newText, newOK := exactStringFromAny(args["newText"], args["new_text"], args["new_string"])
	if !oldOK || !newOK {
		return nil, ToolError{Code: "validation_error", Message: "oldText and newText are required when edits is not provided"}
	}
	if oldText == "" {
		return nil, ToolError{Code: "validation_error", Message: "oldText must not be empty"}
	}
	return []EditInstruction{{
		OldText:    oldText,
		NewText:    newText,
		ReplaceAll: boolField(args, "replace_all") || boolField(args, "replaceAll"),
	}}, nil
}

func ApplyExactTextEdits(content string, edits []EditInstruction) (string, int, error) {
	if len(edits) == 0 {
		return "", 0, ToolError{Code: "validation_error", Message: "at least one edit is required"}
	}
	updated := content
	replacements := 0
	for index, edit := range edits {
		matchCount := strings.Count(updated, edit.OldText)
		if matchCount == 0 {
			return "", 0, ToolError{Code: "old_text_not_found", Message: fmt.Sprintf("edits[%d].oldText was not found", index)}
		}
		if edit.ReplaceAll {
			updated = strings.ReplaceAll(updated, edit.OldText, edit.NewText)
			replacements += matchCount
			continue
		}
		if matchCount > 1 {
			return "", 0, ToolError{Code: "ambiguous_old_text", Message: fmt.Sprintf("edits[%d].oldText matched more than once", index)}
		}
		updated = strings.Replace(updated, edit.OldText, edit.NewText, 1)
		replacements++
	}
	return updated, replacements, nil
}

func PreviewDeleteRange(content string, args map[string]any) (DeleteRangePreview, error) {
	startAnchor := firstNonEmptyAnyString(args["start_anchor"], args["startAnchor"])
	endAnchor := firstNonEmptyAnyString(args["end_anchor"], args["endAnchor"])
	if startAnchor == "" {
		return DeleteRangePreview{}, ToolError{Code: "validation_error", Message: "start_anchor is required"}
	}
	if endAnchor == "" {
		return DeleteRangePreview{}, ToolError{Code: "validation_error", Message: "end_anchor is required"}
	}
	inclusive := true
	if raw, ok := args["inclusive"].(bool); ok {
		inclusive = raw
	}
	lineSep := "\n"
	if strings.Contains(content, "\r\n") {
		lineSep = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	startLine := findUniqueLine(lines, startAnchor)
	switch startLine {
	case -2:
		return DeleteRangePreview{}, ToolError{Code: "anchor_not_unique", Message: "start_anchor is not unique; add more surrounding context"}
	case -1:
		return DeleteRangePreview{}, ToolError{Code: "anchor_not_found", Message: "start_anchor not found"}
	}
	endLine := findUniqueLine(lines, endAnchor)
	switch endLine {
	case -2:
		return DeleteRangePreview{}, ToolError{Code: "anchor_not_unique", Message: "end_anchor is not unique; add more surrounding context"}
	case -1:
		return DeleteRangePreview{}, ToolError{Code: "anchor_not_found", Message: "end_anchor not found"}
	}
	if startLine > endLine {
		return DeleteRangePreview{}, ToolError{Code: "anchor_order", Message: fmt.Sprintf("start_anchor appears after end_anchor (lines %d and %d)", startLine+1, endLine+1)}
	}

	var keep []string
	firstDeletedLine := startLine
	deletedLines := endLine - startLine + 1
	if inclusive {
		keep = append(keep, lines[:startLine]...)
		keep = append(keep, lines[endLine+1:]...)
	} else {
		if startLine == endLine {
			return DeleteRangePreview{}, ToolError{Code: "empty_exclusive_range", Message: "start_anchor and end_anchor match the same line; with inclusive=false there is nothing between them to delete"}
		}
		keep = append(keep, lines[:startLine+1]...)
		keep = append(keep, lines[endLine:]...)
		firstDeletedLine = startLine + 1
		deletedLines = endLine - startLine - 1
	}
	after := strings.Join(keep, lineSep)
	if after != "" && strings.HasSuffix(content, lineSep) && !strings.HasSuffix(after, lineSep) {
		after += lineSep
	}
	return DeleteRangePreview{
		After:            after,
		StartLine:        startLine,
		EndLine:          endLine,
		FirstDeletedLine: firstDeletedLine,
		DeletedLines:     deletedLines,
		Inclusive:        inclusive,
	}, nil
}

func FirstChangedLine(before string, after string) int {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	limit := len(beforeLines)
	if len(afterLines) < limit {
		limit = len(afterLines)
	}
	for index := 0; index < limit; index++ {
		if beforeLines[index] != afterLines[index] {
			return index + 1
		}
	}
	if len(beforeLines) != len(afterLines) {
		return limit + 1
	}
	return 0
}

func findUniqueLine(lines []string, target string) int {
	index := -1
	for lineIndex, line := range lines {
		if line != target {
			continue
		}
		if index >= 0 {
			return -2
		}
		index = lineIndex
	}
	return index
}

func exactStringFromAny(values ...any) (string, bool) {
	for _, value := range values {
		text, ok := value.(string)
		if ok {
			return text, true
		}
	}
	return "", false
}

func boolField(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}
