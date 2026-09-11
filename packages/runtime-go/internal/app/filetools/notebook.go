package filetools

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ToolError struct {
	Code    string
	Message string
}

func (err ToolError) Error() string {
	return err.Message
}

type NotebookEditRequest struct {
	Path      string
	CellIndex *int
	CellID    string
	NewSource string
	CellType  string
	EditMode  string
}

type NotebookEditPreview struct {
	After     string
	CellIndex int
	CellID    string
	CellCount int
	CellType  string
	Summary   string
}

type notebookDocument struct {
	rest  map[string]json.RawMessage
	cells []map[string]json.RawMessage
}

func ParseNotebookEditRequest(args map[string]any) (NotebookEditRequest, error) {
	request := NotebookEditRequest{
		Path:      strings.TrimSpace(firstNonEmptyAnyString(args["path"], args["filePath"], args["FilePath"])),
		CellID:    strings.TrimSpace(firstNonEmptyAnyString(args["cell_id"], args["cellId"])),
		NewSource: firstNotebookSourceAnyString(args),
		CellType:  strings.ToLower(strings.TrimSpace(firstNonEmptyAnyString(args["cell_type"], args["cellType"]))),
		EditMode:  strings.ToLower(strings.TrimSpace(firstNonEmptyAnyString(args["edit_mode"], args["editMode"]))),
	}
	if request.Path == "" {
		return request, ToolError{Code: "validation_error", Message: "path is required"}
	}
	if request.EditMode == "" {
		request.EditMode = "replace"
	}
	switch request.EditMode {
	case "replace", "insert", "delete":
	default:
		return request, ToolError{Code: "validation_error", Message: fmt.Sprintf("edit_mode must be replace, insert, or delete (got %q)", request.EditMode)}
	}
	if request.CellType != "" && request.CellType != "code" && request.CellType != "markdown" {
		return request, ToolError{Code: "validation_error", Message: "cell_type must be code or markdown"}
	}
	if request.EditMode == "insert" && request.CellType == "" {
		return request, ToolError{Code: "validation_error", Message: "cell_type is required for insert"}
	}
	if value, ok := firstPresentNotebookCellNumber(args); ok {
		number, numeric := numericAny(value)
		if !numeric {
			return request, ToolError{Code: "validation_error", Message: "cell_number must be an integer"}
		}
		request.CellIndex = &number
	}
	return request, nil
}

func PreviewNotebookEdit(before string, request NotebookEditRequest) (NotebookEditPreview, error) {
	notebook, err := parseNotebookDocument([]byte(before))
	if err != nil {
		return NotebookEditPreview{}, err
	}
	targetCellID := ""
	targetCellType := ""
	if request.EditMode != "insert" {
		targetIndex, targetErr := notebook.targetIndex(request)
		if targetErr != nil {
			return NotebookEditPreview{}, targetErr
		}
		targetCellID = notebookCellID(notebook.cells[targetIndex])
		targetCellType = notebookCellType(notebook.cells[targetIndex])
	}
	index, summary, err := applyNotebookEdit(notebook, request)
	if err != nil {
		return NotebookEditPreview{}, err
	}
	after, err := notebook.marshal()
	if err != nil {
		return NotebookEditPreview{}, err
	}
	cellType := targetCellType
	if request.EditMode != "delete" && index >= 0 && index < len(notebook.cells) {
		cellType = notebookCellType(notebook.cells[index])
	} else if request.CellType != "" {
		cellType = request.CellType
	}
	return NotebookEditPreview{
		After:     string(after),
		CellIndex: index,
		CellID:    targetCellID,
		CellCount: len(notebook.cells),
		CellType:  cellType,
		Summary:   summary,
	}, nil
}

func firstNotebookSourceAnyString(args map[string]any) string {
	for _, key := range []string{"new_source", "newSource", "content", "source", "new_string", "newString"} {
		if text, ok := args[key].(string); ok {
			return text
		}
	}
	return ""
}

func firstPresentNotebookCellNumber(args map[string]any) (any, bool) {
	for _, key := range []string{"cell_number", "cellNumber"} {
		if value, ok := args[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func parseNotebookDocument(data []byte) (*notebookDocument, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, ToolError{Code: "invalid_notebook", Message: "not valid notebook JSON: " + err.Error()}
	}
	rawCells, ok := top["cells"]
	if !ok {
		return nil, ToolError{Code: "invalid_notebook", Message: "no cells array; not a notebook"}
	}
	var cells []map[string]json.RawMessage
	if err := json.Unmarshal(rawCells, &cells); err != nil {
		return nil, ToolError{Code: "invalid_notebook", Message: "cells is not an array of objects: " + err.Error()}
	}
	return &notebookDocument{rest: top, cells: cells}, nil
}

func (notebook *notebookDocument) marshal() ([]byte, error) {
	cellsJSON, err := json.Marshal(notebook.cells)
	if err != nil {
		return nil, err
	}
	notebook.rest["cells"] = cellsJSON
	out, err := json.MarshalIndent(notebook.rest, "", " ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func applyNotebookEdit(notebook *notebookDocument, request NotebookEditRequest) (int, string, error) {
	if request.EditMode == "insert" {
		after := -1
		if request.CellIndex != nil {
			after = *request.CellIndex
		}
		if after < -1 || after >= len(notebook.cells) {
			return 0, "", ToolError{Code: "cell_out_of_range", Message: fmt.Sprintf("cell_number %d out of range for insert (notebook has %d cells; use -1 to prepend)", after, len(notebook.cells))}
		}
		cell := newNotebookCell(request.CellType, request.NewSource)
		at := after + 1
		notebook.cells = append(notebook.cells[:at], append([]map[string]json.RawMessage{cell}, notebook.cells[at:]...)...)
		return at, "inserted " + request.CellType + " cell", nil
	}
	index, err := notebook.targetIndex(request)
	if err != nil {
		return 0, "", err
	}
	if request.EditMode == "delete" {
		notebook.cells = append(notebook.cells[:index], notebook.cells[index+1:]...)
		return index, "deleted cell", nil
	}
	notebookSetSource(notebook.cells[index], request.NewSource)
	if request.CellType != "" {
		notebook.cells[index]["cell_type"] = notebookJSONString(request.CellType)
	}
	notebookNormalizeOutputs(notebook.cells[index], notebookCellType(notebook.cells[index]))
	return index, "replaced cell source", nil
}

func (notebook *notebookDocument) targetIndex(request NotebookEditRequest) (int, error) {
	if request.CellID != "" {
		for index, cell := range notebook.cells {
			if notebookCellID(cell) == request.CellID {
				return index, nil
			}
		}
		return 0, ToolError{Code: "cell_not_found", Message: fmt.Sprintf("no cell with id %q", request.CellID)}
	}
	if request.CellIndex == nil {
		if len(notebook.cells) == 1 {
			return 0, nil
		}
		return 0, ToolError{Code: "validation_error", Message: fmt.Sprintf("cell_number or cell_id is required for %s (notebook has %d cells; pass the 0-based cell_number)", request.EditMode, len(notebook.cells))}
	}
	index := *request.CellIndex
	if index < 0 || index >= len(notebook.cells) {
		return 0, ToolError{Code: "cell_out_of_range", Message: fmt.Sprintf("cell_number %d out of range (notebook has %d cells)", index, len(notebook.cells))}
	}
	return index, nil
}

func newNotebookCell(cellType string, source string) map[string]json.RawMessage {
	cell := map[string]json.RawMessage{
		"cell_type": notebookJSONString(cellType),
		"metadata":  json.RawMessage(`{}`),
		"source":    notebookSourceLines(source),
	}
	if cellType == "code" {
		cell["outputs"] = json.RawMessage(`[]`)
		cell["execution_count"] = json.RawMessage(`null`)
	}
	return cell
}

func notebookSetSource(cell map[string]json.RawMessage, source string) {
	cell["source"] = notebookSourceLines(source)
}

func notebookNormalizeOutputs(cell map[string]json.RawMessage, cellType string) {
	if cellType == "markdown" {
		delete(cell, "outputs")
		delete(cell, "execution_count")
		return
	}
	cell["outputs"] = json.RawMessage(`[]`)
	cell["execution_count"] = json.RawMessage(`null`)
}

func notebookCellType(cell map[string]json.RawMessage) string {
	var cellType string
	_ = json.Unmarshal(cell["cell_type"], &cellType)
	return cellType
}

func notebookCellID(cell map[string]json.RawMessage) string {
	raw, ok := cell["id"]
	if !ok {
		return ""
	}
	var id string
	_ = json.Unmarshal(raw, &id)
	return id
}

func notebookSourceLines(source string) json.RawMessage {
	if source == "" {
		return json.RawMessage(`[]`)
	}
	parts := strings.SplitAfter(source, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	data, _ := json.Marshal(parts)
	return data
}

func notebookJSONString(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text, ok := value.(string)
		if ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func numericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	case json.Number:
		number, err := typed.Float64()
		if err == nil && number == float64(int(number)) {
			return int(number), true
		}
	}
	return 0, false
}
