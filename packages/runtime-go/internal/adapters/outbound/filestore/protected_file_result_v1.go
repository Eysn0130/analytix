package filestore

import (
	"context"
	"strconv"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	runtimediff "analytix.local/runtime-go/internal/diff"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// These adapters see the same typed result facts as the established model
// reply. They never decode output JSON, rerun an effect or read a later file.
func recordFileResultV1(ctx context.Context, result *domaintoolresult.ProtectedFileResultV1) {
	result.Classification = "source_exact_local"
	if capture, ok := domaintoolresult.BoundProtectedFileResultV1(result); ok {
		domaintoolresult.CaptureProtectedToolResultV1(ctx, capture)
	}
}
func captureListResultV1(ctx context.Context, input filetoolsapp.ListDirToolOutputInput) {
	d := &domaintoolresult.ProtectedFileListV1{Path: input.Path, RelativePath: input.RelativePath, Limit: input.Limit,
		ProducerTruncated: input.Truncated, OriginalCount: len(input.Entries), Entries: []domaintoolresult.ProtectedFileEntryV1{}, Names: append([]string{}, input.Names...)}
	for _, row := range input.Entries {
		d.Entries = append(d.Entries, domaintoolresult.ProtectedFileEntryV1{Name: row.Name, DisplayName: row.DisplayName, Kind: row.Kind})
	}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "ls", List: d})
}
func captureFindResultV1(ctx context.Context, input filetoolsapp.FindToolOutputInput) {
	d := &domaintoolresult.ProtectedFileFindV1{Path: input.Path, RelativePath: input.RelativePath, Pattern: input.Pattern, Limit: input.Limit,
		ProducerTruncated: input.Truncated, OriginalCount: len(input.Matches), Matches: []domaintoolresult.ProtectedFileMatchV1{},
		SkippedProtected: append([]string{}, input.SkippedProtected...), SkippedProtectedOriginalCount: len(input.SkippedProtected)}
	for _, row := range input.Matches {
		d.Matches = append(d.Matches, domaintoolresult.ProtectedFileMatchV1{Path: row.Path, RelativePath: row.RelativePath})
	}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "find", Find: d})
}
func captureCodeIndexResultV1(ctx context.Context, result filetoolsapp.CodeIndexToolResult) {
	input := result.Input
	text := result.Text
	d := &domaintoolresult.ProtectedFileCodeIndexV1{Action: input.Request.Action, Path: input.Path, RelativePath: input.RelativePath,
		Query: input.Request.Query, Kind: input.Request.Kind, Limit: input.Request.Limit, Symbols: []domaintoolresult.ProtectedFileSymbolV1{},
		SymbolCount: len(result.Symbols), OriginalCount: len(result.Symbols), Text: &text, ProducerTruncated: result.Truncated, LimitReached: result.LimitReached,
		SkippedProtected: append([]string{}, input.SkippedProtected...), SkippedProtectedOriginalCount: len(input.SkippedProtected),
		SkippedDirectories: append([]string{}, input.SkippedDirs...), SkippedDirectoriesOriginalCount: len(input.SkippedDirs)}
	for _, row := range result.Symbols {
		d.Symbols = append(d.Symbols, domaintoolresult.ProtectedFileSymbolV1{Name: row.Name, Kind: row.Kind, File: row.File, Path: row.Path, Line: row.Line, Parent: row.Parent, Signature: row.Signature})
	}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "code_index", CodeIndex: d})
}
func fileResultDiffV1(change runtimediff.Change) domaintoolresult.ProtectedFileDiffV1 {
	d := domaintoolresult.ProtectedFileDiffV1{Kind: string(change.Kind), Added: change.Added, Removed: change.Removed, Binary: change.Binary, Truncated: change.Truncated}
	if change.Diff != "" {
		patch := change.Diff
		d.Patch = &patch
	}
	if change.Binary {
		d.OmittedReason = "binary"
	}
	if change.Truncated {
		d.OmittedReason = "large_change"
	}
	return d
}
func captureMoveResultV1(ctx context.Context, input filetoolsapp.MoveToolOutputInput) {
	d := &domaintoolresult.ProtectedFileMoveV1{SourcePath: input.SourcePath, DestinationPath: input.DestinationPath,
		SourceRelativePath: input.SourceRelativePath, DestinationRelativePath: input.DestinationRelativePath,
		BytesMoved: strconv.FormatInt(input.BytesMoved, 10), Moved: input.Moved, Message: filetoolsapp.MoveToolResultMessage(input)}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "move_file", Move: d})
}
func captureNotebookEditResultV1(ctx context.Context, input filetoolsapp.NotebookEditToolOutputInput, change runtimediff.Change) {
	d := &domaintoolresult.ProtectedFileNotebookEditV1{Path: input.Path, RelativePath: input.RelativePath, EditMode: input.Request.EditMode,
		CellNumber: input.Change.CellIndex, CellCount: input.Change.CellCount, CellType: input.Change.CellType, Message: input.Change.Summary,
		BytesWritten: strconv.Itoa(input.BytesWritten), FirstChangedLine: filetoolsapp.FirstChangedLine(input.Before, input.Change.After), Diff: fileResultDiffV1(change)}
	if input.Request.CellID != "" {
		id := input.Request.CellID
		d.CellID = &id
	}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "notebook_edit", NotebookEdit: d})
}
func captureDeleteRangeResultV1(ctx context.Context, input filetoolsapp.DeleteRangeToolOutputInput, change runtimediff.Change) {
	d := &domaintoolresult.ProtectedFileDeleteRangeV1{Path: input.Path, RelativePath: input.RelativePath,
		StartLine: input.Change.StartLine + 1, EndLine: input.Change.EndLine + 1, FirstDeletedLine: input.Change.FirstDeletedLine + 1,
		DeletedLines: input.Change.DeletedLines, BytesWritten: strconv.Itoa(input.BytesWritten), Encoding: input.Encoding,
		Inclusive: input.Change.Inclusive, Diff: fileResultDiffV1(change)}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "delete_range", DeleteRange: d})
}
func captureDeleteSymbolResultV1(ctx context.Context, input filetoolsapp.DeleteSymbolToolOutputInput, change runtimediff.Change) {
	d := &domaintoolresult.ProtectedFileDeleteSymbolV1{Path: input.Path, RelativePath: input.RelativePath,
		Name: input.Change.Match.Name, Kind: input.Change.Match.Kind, Line: input.Change.Match.Line,
		FirstDeletedLine: input.Change.FirstDeletedLine, DeletedLines: input.Change.DeletedLines,
		BytesWritten: strconv.Itoa(input.BytesWritten), Encoding: input.Encoding, Diff: fileResultDiffV1(change)}
	if input.Change.Match.Parent != "" {
		parent := input.Change.Match.Parent
		d.Parent = &parent
	}
	recordFileResultV1(ctx, &domaintoolresult.ProtectedFileResultV1{Operation: "delete_symbol", DeleteSymbol: d})
}
