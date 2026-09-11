package filetools

import "errors"

type ReadRecord struct {
	TurnID       string
	Path         string
	RelativePath string
	Content      string
	Truncated    bool
}

type ReadBeforeChangeInput struct {
	TurnID       string
	Path         string
	RelativePath string
	Args         map[string]any
	Record       ReadRecord
	HasRecord    bool
	IsGoFile     bool
}

func ValidateReadBeforeDeleteRange(input ReadBeforeChangeInput) (map[string]any, bool) {
	if output, blocked := validateReadRecord(input, "delete_range", "deleting a range", "deleting a range from"); blocked {
		return output, true
	}
	if _, err := PreviewDeleteRange(input.Record.Content, input.Args); err != nil {
		output := readBeforeEditOutput(
			"delete_range anchors did not match the latest read output for "+input.Record.RelativePath+": "+err.Error()+". Re-read the file before deleting a range.",
			input.Path,
			input.Record.RelativePath,
		)
		addToolErrorCause(output, err)
		return output, true
	}
	return nil, false
}

func ValidateReadBeforeDeleteSymbol(input ReadBeforeChangeInput) (map[string]any, bool) {
	relativePath := firstNonEmptyAnyString(input.RelativePath, input.Record.RelativePath)
	if !input.IsGoFile {
		return map[string]any{
			"code":          "unsupported_file_type",
			"error":         "delete_symbol only supports Go files; use delete_range for non-Go files",
			"path":          input.Path,
			"relative_path": relativePath,
		}, true
	}
	if output, blocked := validateReadRecord(input, "delete_symbol", "deleting a symbol", "deleting a symbol from"); blocked {
		return output, true
	}
	if _, err := PreviewDeleteSymbol(input.Path, input.Record.Content, input.Args); err != nil {
		output := readBeforeEditOutput(
			"delete_symbol target did not match the latest read output for "+input.Record.RelativePath+": "+err.Error()+". Re-read the file before deleting a symbol.",
			input.Path,
			input.Record.RelativePath,
		)
		addToolErrorCause(output, err)
		return output, true
	}
	return nil, false
}

func ValidateReadBeforeEdit(input ReadBeforeChangeInput) (map[string]any, bool) {
	edits, err := ParseEditInstructions(input.Args)
	if err != nil {
		return ToolErrorOutput(err), true
	}
	if output, blocked := validateReadRecord(input, "edit", "editing", "editing"); blocked {
		return output, true
	}
	if _, _, err := ApplyExactTextEdits(input.Record.Content, edits); err != nil {
		output := readBeforeEditOutput(
			"edit instructions did not match the latest read output for "+input.Record.RelativePath+": "+err.Error()+". Re-read the file before editing.",
			input.Path,
			input.Record.RelativePath,
		)
		addToolErrorCause(output, err)
		return output, true
	}
	return nil, false
}

func validateReadRecord(input ReadBeforeChangeInput, toolName string, missingAction string, staleAction string) (map[string]any, bool) {
	relativePath := firstNonEmptyAnyString(input.RelativePath, input.Record.RelativePath)
	if !input.HasRecord {
		return readBeforeEditOutput(
			"read-before-edit guard blocked "+toolName+" for "+relativePath+". Read the current file contents in this turn before "+missingAction+".",
			input.Path,
			relativePath,
		), true
	}
	if input.Record.TurnID != input.TurnID {
		return readBeforeEditOutput(
			"read-before-edit guard requires a fresh read in the current turn before "+staleAction+" "+input.Record.RelativePath+".",
			input.Path,
			input.Record.RelativePath,
		), true
	}
	return nil, false
}

func readBeforeEditOutput(message string, path string, relativePath string) map[string]any {
	return map[string]any{
		"code":          "read_before_edit_required",
		"error":         message,
		"path":          path,
		"relative_path": relativePath,
	}
}

func addToolErrorCause(output map[string]any, err error) {
	var fileToolErr ToolError
	if errors.As(err, &fileToolErr) {
		output["cause_code"] = fileToolErr.Code
	}
}

func ToolErrorOutput(err error) map[string]any {
	var fileToolErr ToolError
	if errors.As(err, &fileToolErr) {
		return map[string]any{"code": fileToolErr.Code, "error": fileToolErr.Message}
	}
	return map[string]any{"code": "tool_failed", "error": err.Error()}
}
