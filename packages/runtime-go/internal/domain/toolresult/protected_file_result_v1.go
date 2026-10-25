package toolresult

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	"analytix.local/runtime-go/internal/domain/secretprojection"
)

const ProtectedFileResultLimitV1 = 32 << 10

// Only the seven concrete filesystem producers create these observations.
// Paths are display facts, never navigation or mutation capabilities.
type ProtectedFileResultV1 struct {
	Operation      string                       `json:"operation"`
	Classification string                       `json:"classification"`
	List           *ProtectedFileListV1         `json:"list,omitempty"`
	Find           *ProtectedFileFindV1         `json:"find,omitempty"`
	CodeIndex      *ProtectedFileCodeIndexV1    `json:"codeIndex,omitempty"`
	Move           *ProtectedFileMoveV1         `json:"move,omitempty"`
	NotebookEdit   *ProtectedFileNotebookEditV1 `json:"notebookEdit,omitempty"`
	DeleteRange    *ProtectedFileDeleteRangeV1  `json:"deleteRange,omitempty"`
	DeleteSymbol   *ProtectedFileDeleteSymbolV1 `json:"deleteSymbol,omitempty"`
}
type ProtectedFileEntryV1 struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Kind        string `json:"kind"`
}
type ProtectedFileListV1 struct {
	Path              string                 `json:"path"`
	RelativePath      string                 `json:"relativePath"`
	Entries           []ProtectedFileEntryV1 `json:"entries"`
	Names             []string               `json:"names"`
	Limit             int                    `json:"limit"`
	ProducerTruncated bool                   `json:"producerTruncated"`
	OriginalCount     int                    `json:"originalCount"`
}
type ProtectedFileMatchV1 struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
}
type ProtectedFileFindV1 struct {
	Path                          string                 `json:"path"`
	RelativePath                  string                 `json:"relativePath"`
	Pattern                       string                 `json:"pattern"`
	Matches                       []ProtectedFileMatchV1 `json:"matches"`
	SkippedProtected              []string               `json:"skippedProtected"`
	SkippedProtectedOriginalCount int                    `json:"skippedProtectedOriginalCount"`
	Limit                         int                    `json:"limit"`
	ProducerTruncated             bool                   `json:"producerTruncated"`
	OriginalCount                 int                    `json:"originalCount"`
}
type ProtectedFileSymbolV1 struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Parent    string `json:"parent"`
	Signature string `json:"signature"`
}
type ProtectedFileCodeIndexV1 struct {
	Action                          string                  `json:"action"`
	Path                            string                  `json:"path"`
	RelativePath                    string                  `json:"relativePath"`
	Query                           string                  `json:"query"`
	Kind                            string                  `json:"kind"`
	Limit                           int                     `json:"limit"`
	Symbols                         []ProtectedFileSymbolV1 `json:"symbols"`
	SymbolCount                     int                     `json:"symbolCount"`
	OriginalCount                   int                     `json:"originalCount"`
	Text                            *string                 `json:"text,omitempty"`
	TextUnavailable                 string                  `json:"textUnavailable,omitempty"`
	SkippedProtected                []string                `json:"skippedProtected"`
	SkippedProtectedOriginalCount   int                     `json:"skippedProtectedOriginalCount"`
	SkippedDirectories              []string                `json:"skippedDirectories"`
	SkippedDirectoriesOriginalCount int                     `json:"skippedDirectoriesOriginalCount"`
	ProducerTruncated               bool                    `json:"producerTruncated"`
	LimitReached                    bool                    `json:"limitReached"`
}
type ProtectedFileMoveV1 struct {
	SourcePath              string `json:"sourcePath"`
	DestinationPath         string `json:"destinationPath"`
	SourceRelativePath      string `json:"sourceRelativePath"`
	DestinationRelativePath string `json:"destinationRelativePath"`
	BytesMoved              string `json:"bytesMoved"`
	Moved                   bool   `json:"moved"`
	Message                 string `json:"message"`
}
type ProtectedFileDiffV1 struct {
	Kind             string  `json:"kind"`
	Added            int     `json:"added"`
	Removed          int     `json:"removed"`
	Patch            *string `json:"patch,omitempty"`
	Binary           bool    `json:"binary"`
	Truncated        bool    `json:"truncated"`
	OmittedReason    string  `json:"omittedReason,omitempty"`
	PatchUnavailable string  `json:"patchUnavailable,omitempty"`
}
type ProtectedFileNotebookEditV1 struct {
	Path             string              `json:"path"`
	RelativePath     string              `json:"relativePath"`
	EditMode         string              `json:"editMode"`
	CellNumber       int                 `json:"cellNumber"`
	CellCount        int                 `json:"cellCount"`
	CellType         string              `json:"cellType"`
	CellID           *string             `json:"cellId,omitempty"`
	Message          string              `json:"message"`
	BytesWritten     string              `json:"bytesWritten"`
	FirstChangedLine int                 `json:"firstChangedLine"`
	Diff             ProtectedFileDiffV1 `json:"diff"`
}
type ProtectedFileDeleteRangeV1 struct {
	Path             string              `json:"path"`
	RelativePath     string              `json:"relativePath"`
	StartLine        int                 `json:"startLine"`
	EndLine          int                 `json:"endLine"`
	FirstDeletedLine int                 `json:"firstDeletedLine"`
	DeletedLines     int                 `json:"deletedLines"`
	BytesWritten     string              `json:"bytesWritten"`
	Encoding         string              `json:"encoding"`
	Inclusive        bool                `json:"inclusive"`
	Diff             ProtectedFileDiffV1 `json:"diff"`
}
type ProtectedFileDeleteSymbolV1 struct {
	Path             string              `json:"path"`
	RelativePath     string              `json:"relativePath"`
	Name             string              `json:"name"`
	Kind             string              `json:"kind"`
	Parent           *string             `json:"parent,omitempty"`
	Line             int                 `json:"line"`
	FirstDeletedLine int                 `json:"firstDeletedLine"`
	DeletedLines     int                 `json:"deletedLines"`
	BytesWritten     string              `json:"bytesWritten"`
	Encoding         string              `json:"encoding"`
	Diff             ProtectedFileDiffV1 `json:"diff"`
}

func (v *ProtectedFileResultV1) UnmarshalJSON(body []byte) error {
	type plain ProtectedFileResultV1
	if err := decodeFileResultRequiredV1(body, (*plain)(v), []string{"operation", "classification"}, "list", "find", "codeIndex", "move", "notebookEdit", "deleteRange", "deleteSymbol"); err != nil {
		return err
	}
	return ValidateProtectedFileResultV1(v)
}
func decodeFileResultRequiredV1(b []byte, target any, required []string, optional ...string) error {
	if domainjsonstrict.Validate(b, domainjsonstrict.Options{RequireObject: true, MaxBytes: ProtectedFileResultLimitV1, MaxDepth: 8, MaxTokens: 20000, MaxStringBytes: ProtectedFileResultLimitV1}) != nil {
		return errors.New("protected file result JSON is invalid")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) != nil {
		return errors.New("protected file result must be an object")
	}
	for key, raw := range fields {
		if !oneOf(key, required...) && !oneOf(key, optional...) {
			return errors.New("protected file result field is unknown")
		}
		if oneOf(key, "textUnavailable", "patchUnavailable", "omittedReason") && string(raw) == `""` {
			return errors.New("protected file result omission is empty")
		}
	}
	return decodeClosedRequiredV1(b, target, required, nil)
}
func (v *ProtectedFileEntryV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileEntryV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"name", "displayName", "kind"})
}
func (v *ProtectedFileMatchV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileMatchV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"path", "relativePath"})
}
func (v *ProtectedFileSymbolV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileSymbolV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"name", "kind", "file", "path", "line", "parent", "signature"})
}
func (v *ProtectedFileListV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileListV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"path", "relativePath", "entries", "names", "limit", "producerTruncated", "originalCount"})
}
func (v *ProtectedFileFindV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileFindV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"path", "relativePath", "pattern", "matches", "skippedProtected", "skippedProtectedOriginalCount", "limit", "producerTruncated", "originalCount"})
}
func (v *ProtectedFileCodeIndexV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileCodeIndexV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"action", "path", "relativePath", "query", "kind", "limit", "symbols", "symbolCount", "originalCount", "skippedProtected", "skippedProtectedOriginalCount", "skippedDirectories", "skippedDirectoriesOriginalCount", "producerTruncated", "limitReached"}, "text", "textUnavailable")
}
func (v *ProtectedFileMoveV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileMoveV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"sourcePath", "destinationPath", "sourceRelativePath", "destinationRelativePath", "bytesMoved", "moved", "message"})
}
func (v *ProtectedFileDiffV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileDiffV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"kind", "added", "removed", "binary", "truncated"}, "patch", "patchUnavailable", "omittedReason")
}
func (v *ProtectedFileNotebookEditV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileNotebookEditV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"path", "relativePath", "editMode", "cellNumber", "cellCount", "cellType", "message", "bytesWritten", "firstChangedLine", "diff"}, "cellId")
}
func (v *ProtectedFileDeleteRangeV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileDeleteRangeV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"path", "relativePath", "startLine", "endLine", "firstDeletedLine", "deletedLines", "bytesWritten", "encoding", "inclusive", "diff"})
}
func (v *ProtectedFileDeleteSymbolV1) UnmarshalJSON(b []byte) error {
	type plain ProtectedFileDeleteSymbolV1
	return decodeFileResultRequiredV1(b, (*plain)(v), []string{"path", "relativePath", "name", "kind", "line", "firstDeletedLine", "deletedLines", "bytesWritten", "encoding", "diff"}, "parent")
}

func fileResultTextV1(s string, limit int, nonempty bool) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && len(s) <= limit && (!nonempty || s != "")
}
func fileResultIntegerV1(n int) bool { return n >= 0 && uint64(n) <= 9007199254740991 }
func fileResultCountV1(shown, original int) bool {
	return fileResultIntegerV1(original) && shown <= original
}
func fileResultBytesV1(s string) bool {
	n, err := strconv.ParseUint(s, 10, 63)
	return err == nil && strconv.FormatUint(n, 10) == s
}
func fileResultPathsV1(values []string, original int) bool {
	if values == nil || !fileResultCountV1(len(values), original) {
		return false
	}
	for _, s := range values {
		if !fileResultTextV1(s, 4096, true) {
			return false
		}
	}
	return true
}
func fileResultLocationV1(path, relative string) bool {
	return fileResultTextV1(path, 4096, true) && fileResultTextV1(relative, 4096, true)
}
func fileResultDiffValidV1(d ProtectedFileDiffV1) bool {
	return d.Kind == "modify" && fileResultIntegerV1(d.Added) && fileResultIntegerV1(d.Removed) &&
		(d.Patch == nil || fileResultTextV1(*d.Patch, ProtectedFileResultLimitV1, false)) &&
		(d.PatchUnavailable == "" || d.PatchUnavailable == "limit" && d.Patch == nil) &&
		(d.Binary && d.OmittedReason == "binary" && d.Patch == nil && !d.Truncated ||
			!d.Binary && (d.Truncated && d.OmittedReason == "large_change" || !d.Truncated && d.OmittedReason == ""))
}
func validateFileResultShapeV1(v *ProtectedFileResultV1) bool {
	if v == nil || !oneOf(v.Classification, "source_exact_local", "credential_redacted") {
		return false
	}
	if (v.List != nil) != (v.Operation == "ls") || (v.Find != nil) != (v.Operation == "find") ||
		(v.CodeIndex != nil) != (v.Operation == "code_index") || (v.Move != nil) != (v.Operation == "move_file") ||
		(v.NotebookEdit != nil) != (v.Operation == "notebook_edit") || (v.DeleteRange != nil) != (v.Operation == "delete_range") ||
		(v.DeleteSymbol != nil) != (v.Operation == "delete_symbol") {
		return false
	}
	switch v.Operation {
	case "ls":
		d := v.List
		if !fileResultLocationV1(d.Path, d.RelativePath) || d.Entries == nil || d.Names == nil || len(d.Entries) != len(d.Names) || !fileResultCountV1(len(d.Entries), d.OriginalCount) || d.Limit < 1 || d.Limit > 1000 || d.OriginalCount > d.Limit {
			return false
		}
		for i, row := range d.Entries {
			if !fileResultTextV1(row.Name, 4096, true) || !fileResultTextV1(row.DisplayName, 4096, true) || !oneOf(row.Kind, "file", "directory") || d.Names[i] != row.DisplayName {
				return false
			}
		}
	case "find":
		d := v.Find
		if !fileResultLocationV1(d.Path, d.RelativePath) || !fileResultTextV1(d.Pattern, ProtectedFileResultLimitV1, true) || d.Matches == nil || !fileResultCountV1(len(d.Matches), d.OriginalCount) || d.Limit < 1 || d.Limit > 1000 || d.OriginalCount > d.Limit || !fileResultPathsV1(d.SkippedProtected, d.SkippedProtectedOriginalCount) {
			return false
		}
		for _, row := range d.Matches {
			if !fileResultLocationV1(row.Path, row.RelativePath) {
				return false
			}
		}
	case "code_index":
		d := v.CodeIndex
		if !oneOf(d.Action, "outline", "search") || !fileResultLocationV1(d.Path, d.RelativePath) || !fileResultTextV1(d.Query, ProtectedFileResultLimitV1, false) || !fileResultTextV1(d.Kind, 4096, false) || d.Limit < 1 || d.Limit > 200 || d.Symbols == nil || !fileResultCountV1(len(d.Symbols), d.OriginalCount) || d.SymbolCount != d.OriginalCount || d.SymbolCount > d.Limit || d.LimitReached != (d.SymbolCount >= d.Limit) || !fileResultPathsV1(d.SkippedProtected, d.SkippedProtectedOriginalCount) || !fileResultPathsV1(d.SkippedDirectories, d.SkippedDirectoriesOriginalCount) || (d.Text == nil) != (d.TextUnavailable == "limit") || d.Text != nil && (d.TextUnavailable != "" || !fileResultTextV1(*d.Text, ProtectedFileResultLimitV1, false)) {
			return false
		}
		for _, row := range d.Symbols {
			if !fileResultTextV1(row.Name, 4096, false) || !fileResultTextV1(row.Kind, 4096, false) || !fileResultTextV1(row.File, 4096, false) || !fileResultTextV1(row.Path, 4096, false) || !fileResultIntegerV1(row.Line) || row.Line < 1 || !fileResultTextV1(row.Parent, 4096, false) || !fileResultTextV1(row.Signature, ProtectedFileResultLimitV1, false) {
				return false
			}
		}
	case "move_file":
		d := v.Move
		if !fileResultLocationV1(d.SourcePath, d.SourceRelativePath) || !fileResultLocationV1(d.DestinationPath, d.DestinationRelativePath) || !fileResultBytesV1(d.BytesMoved) || !fileResultTextV1(d.Message, ProtectedFileResultLimitV1, false) {
			return false
		}
	case "notebook_edit":
		d := v.NotebookEdit
		if !fileResultLocationV1(d.Path, d.RelativePath) || !oneOf(d.EditMode, "insert", "replace", "delete") || !fileResultIntegerV1(d.CellNumber) || !fileResultIntegerV1(d.CellCount) || !fileResultTextV1(d.CellType, 4096, false) || d.CellID != nil && !fileResultTextV1(*d.CellID, 4096, true) || !fileResultTextV1(d.Message, ProtectedFileResultLimitV1, false) || !fileResultBytesV1(d.BytesWritten) || !fileResultIntegerV1(d.FirstChangedLine) || !fileResultDiffValidV1(d.Diff) {
			return false
		}
	case "delete_range":
		d := v.DeleteRange
		if !fileResultLocationV1(d.Path, d.RelativePath) || !fileResultIntegerV1(d.StartLine) || d.StartLine < 1 || !fileResultIntegerV1(d.EndLine) || d.EndLine < d.StartLine || !fileResultIntegerV1(d.FirstDeletedLine) || d.FirstDeletedLine < 1 || !fileResultIntegerV1(d.DeletedLines) || !fileResultBytesV1(d.BytesWritten) || !fileResultTextV1(d.Encoding, 4096, false) || !fileResultDiffValidV1(d.Diff) {
			return false
		}
		if d.Inclusive && (d.FirstDeletedLine != d.StartLine || d.DeletedLines != d.EndLine-d.StartLine+1) || !d.Inclusive && (d.EndLine <= d.StartLine || d.FirstDeletedLine != d.StartLine+1 || d.DeletedLines != d.EndLine-d.StartLine-1) {
			return false
		}
	case "delete_symbol":
		d := v.DeleteSymbol
		if !fileResultLocationV1(d.Path, d.RelativePath) || !fileResultTextV1(d.Name, 4096, false) || !fileResultTextV1(d.Kind, 4096, false) || d.Parent != nil && !fileResultTextV1(*d.Parent, 4096, false) || !fileResultIntegerV1(d.Line) || d.Line < 1 || !fileResultIntegerV1(d.FirstDeletedLine) || d.FirstDeletedLine < 1 || !fileResultIntegerV1(d.DeletedLines) || !fileResultBytesV1(d.BytesWritten) || !fileResultTextV1(d.Encoding, 4096, false) || !fileResultDiffValidV1(d.Diff) {
			return false
		}
	default:
		return false
	}
	return true
}
func ValidateProtectedFileResultV1(v *ProtectedFileResultV1) error {
	if !validateFileResultShapeV1(v) {
		return errors.New("protected file result is invalid")
	}
	body, err := json.Marshal(v)
	if err != nil || len(body) > ProtectedFileResultLimitV1 {
		return errors.New("protected file result exceeds limit")
	}
	return nil
}
func fileResultClippedV1(v *ProtectedFileResultV1) bool {
	if d := v.List; d != nil {
		return len(d.Entries) < d.OriginalCount
	}
	if d := v.Find; d != nil {
		return len(d.Matches) < d.OriginalCount || len(d.SkippedProtected) < d.SkippedProtectedOriginalCount
	}
	if d := v.CodeIndex; d != nil {
		return len(d.Symbols) < d.OriginalCount || len(d.SkippedProtected) < d.SkippedProtectedOriginalCount || len(d.SkippedDirectories) < d.SkippedDirectoriesOriginalCount || d.TextUnavailable != ""
	}
	if d := v.NotebookEdit; d != nil {
		return d.Diff.PatchUnavailable != ""
	}
	if d := v.DeleteRange; d != nil {
		return d.Diff.PatchUnavailable != ""
	}
	if d := v.DeleteSymbol; d != nil {
		return d.Diff.PatchUnavailable != ""
	}
	return false
}
func validateProtectedFileResultCaptureV1(c ProtectedCaptureV1) error {
	if c.Status != "completed" || c.Body != "" || c.Label != "" || c.DurationMs != 0 || c.ExitCode != nil || c.StartLine != 0 || c.EndLine != 0 || c.TotalLines != 0 || c.ReadTarget != nil || c.Web != nil || c.WebTargets != nil || c.Search != nil || c.Goal != nil || c.Todo != nil || c.Skill != nil || c.Job != nil || c.Schedule != nil || c.Present != nil || c.PresentTargets != nil || ValidateProtectedFileResultV1(c.FileResult) != nil || c.Truncated != fileResultClippedV1(c.FileResult) {
		return errors.New("protected file result capture is invalid")
	}
	return nil
}
func cloneProtectedFileResultV1(v *ProtectedFileResultV1) *ProtectedFileResultV1 {
	if v == nil {
		return nil
	}
	body, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var copy_ ProtectedFileResultV1
	if json.Unmarshal(body, &copy_) != nil {
		return nil
	}
	return &copy_
}

// Bound already typed producer facts. Never cut a row, path, JSON or patch into
// a different fact; budget omission and producer truncation remain independent.
func BoundProtectedFileResultV1(v *ProtectedFileResultV1) (ProtectedCaptureV1, bool) {
	if v == nil {
		return ProtectedCaptureV1{}, false
	}
	if d := v.CodeIndex; d != nil && d.Text != nil && !fileResultTextV1(*d.Text, ProtectedFileResultLimitV1, false) {
		if !utf8.ValidString(*d.Text) || strings.ContainsRune(*d.Text, 0) {
			return ProtectedCaptureV1{}, false
		}
		d.Text = nil
		d.TextUnavailable = "limit"
	}
	diff := fileResultDiffV1(v)
	if diff != nil && diff.Patch != nil && !fileResultTextV1(*diff.Patch, ProtectedFileResultLimitV1, false) {
		if !utf8.ValidString(*diff.Patch) || strings.ContainsRune(*diff.Patch, 0) {
			return ProtectedCaptureV1{}, false
		}
		diff.Patch = nil
		diff.PatchUnavailable = "limit"
	}
	if !validateFileResultShapeV1(v) {
		return ProtectedCaptureV1{}, false
	}
	if !fileResultFitsV1(v) {
		if d := v.CodeIndex; d != nil && d.Text != nil {
			d.Text = nil
			d.TextUnavailable = "limit"
		}
		if diff != nil && diff.Patch != nil {
			diff.Patch = nil
			diff.PatchUnavailable = "limit"
		}
	}
	// Binary-search complete prefixes rather than repeatedly encoding thousands
	// of near-identical oversized observations on the foreground effect path.
	if d := v.Find; d != nil && !fileResultFitsV1(v) {
		all := d.SkippedProtected
		boundFileResultPrefixV1(v, len(all), func(n int) { d.SkippedProtected = all[:n] })
	}
	if d := v.CodeIndex; d != nil {
		if !fileResultFitsV1(v) {
			all := d.SkippedDirectories
			boundFileResultPrefixV1(v, len(all), func(n int) { d.SkippedDirectories = all[:n] })
		}
		if !fileResultFitsV1(v) {
			all := d.SkippedProtected
			boundFileResultPrefixV1(v, len(all), func(n int) { d.SkippedProtected = all[:n] })
		}
		if !fileResultFitsV1(v) {
			all := d.Symbols
			boundFileResultPrefixV1(v, len(all), func(n int) { d.Symbols = all[:n] })
		}
	}
	if d := v.List; d != nil && !fileResultFitsV1(v) {
		entries, names := d.Entries, d.Names
		boundFileResultPrefixV1(v, len(entries), func(n int) { d.Entries, d.Names = entries[:n], names[:n] })
	}
	if d := v.Find; d != nil && !fileResultFitsV1(v) {
		all := d.Matches
		boundFileResultPrefixV1(v, len(all), func(n int) { d.Matches = all[:n] })
	}
	if !fileResultFitsV1(v) {
		return ProtectedCaptureV1{}, false
	}
	c := ProtectedCaptureV1{Kind: "file_result", Status: "completed", FileResult: v, Truncated: fileResultClippedV1(v)}
	return c, ValidateProtectedCaptureV1(c) == nil
}
func fileResultFitsV1(v *ProtectedFileResultV1) bool {
	body, err := json.Marshal(v)
	return err == nil && len(body) <= ProtectedFileResultLimitV1
}
func boundFileResultPrefixV1(v *ProtectedFileResultV1, count int, keep func(int)) {
	low, high := 0, count
	for low < high {
		middle := low + (high-low+1)/2
		keep(middle)
		if fileResultFitsV1(v) {
			low = middle
		} else {
			high = middle - 1
		}
	}
	keep(low)
}
func fileResultDiffV1(v *ProtectedFileResultV1) *ProtectedFileDiffV1 {
	if v.NotebookEdit != nil {
		return &v.NotebookEdit.Diff
	}
	if v.DeleteRange != nil {
		return &v.DeleteRange.Diff
	}
	if v.DeleteSymbol != nil {
		return &v.DeleteSymbol.Diff
	}
	return nil
}

// Only untrusted text facts are projected. Operation, typed enums, numbers and
// receipt booleans retain their actual producer meaning. Nothing is exported.
func ProjectProtectedFileResultV1(v *ProtectedFileResultV1, secrets ...string) (*ProtectedFileResultV1, bool) {
	copy_ := cloneProtectedFileResultV1(v)
	if copy_ == nil || len(secrets) > 64 {
		return nil, false
	}
	for _, secret := range secrets {
		if secret != "" && (len(strings.TrimSpace(secret)) < 4 || len(secret) > 1<<20) {
			return nil, false
		}
	}
	valid := true
	text := func(value *string) {
		before := *value
		if strings.TrimSpace(before) == "" {
			return
		}
		*value = secretprojection.ProjectTextV1(before, secrets...)
		if secretprojection.ContainsCredentialV1(*value) {
			valid = false
		}
		if before != *value {
			copy_.Classification = "credential_redacted"
		}
	}
	location := func(path, relative *string) { text(path); text(relative) }
	paths := func(values []string) {
		for i := range values {
			text(&values[i])
		}
	}
	if d := copy_.List; d != nil {
		location(&d.Path, &d.RelativePath)
		for i := range d.Entries {
			text(&d.Entries[i].Name)
			text(&d.Entries[i].DisplayName)
			text(&d.Names[i])
		}
	}
	if d := copy_.Find; d != nil {
		location(&d.Path, &d.RelativePath)
		text(&d.Pattern)
		for i := range d.Matches {
			location(&d.Matches[i].Path, &d.Matches[i].RelativePath)
		}
		paths(d.SkippedProtected)
	}
	if d := copy_.CodeIndex; d != nil {
		location(&d.Path, &d.RelativePath)
		text(&d.Query)
		text(&d.Kind)
		for i := range d.Symbols {
			row := &d.Symbols[i]
			text(&row.Name)
			text(&row.Kind)
			text(&row.File)
			text(&row.Path)
			text(&row.Parent)
			text(&row.Signature)
		}
		if d.Text != nil {
			text(d.Text)
		}
		paths(d.SkippedProtected)
		paths(d.SkippedDirectories)
	}
	if d := copy_.Move; d != nil {
		location(&d.SourcePath, &d.SourceRelativePath)
		location(&d.DestinationPath, &d.DestinationRelativePath)
		text(&d.Message)
	}
	if d := copy_.NotebookEdit; d != nil {
		location(&d.Path, &d.RelativePath)
		text(&d.CellType)
		if d.CellID != nil {
			text(d.CellID)
		}
		text(&d.Message)
	}
	if d := copy_.DeleteRange; d != nil {
		location(&d.Path, &d.RelativePath)
		text(&d.Encoding)
	}
	if d := copy_.DeleteSymbol; d != nil {
		location(&d.Path, &d.RelativePath)
		text(&d.Name)
		text(&d.Kind)
		if d.Parent != nil {
			text(d.Parent)
		}
		text(&d.Encoding)
	}
	if diff := fileResultDiffV1(copy_); diff != nil && diff.Patch != nil {
		text(diff.Patch)
	}
	if !valid || ValidateProtectedFileResultV1(copy_) != nil {
		return nil, false
	}
	return copy_, true
}
