package toolresult

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	"analytix.local/runtime-go/internal/domain/secretprojection"
)

var protectedFileResultOperationsForTestV1 = []string{"ls", "find", "code_index", "move_file", "notebook_edit", "delete_range", "delete_symbol"}

func protectedFileResultStringForTestV1(value string) *string { return &value }

// These fixtures are producer facts, not decoded model-return maps. Empty
// query/parent and zero-based notebook cell coordinates are intentional.
func protectedFileResultFixtureForTestV1(operation string) *ProtectedFileResultV1 {
	value := &ProtectedFileResultV1{Operation: operation, Classification: "source_exact_local"}
	diff := ProtectedFileDiffV1{Kind: "modify", Added: 1, Removed: 2, Patch: protectedFileResultStringForTestV1("@@ -1,2 +1 @@\n-old\n+new\n")}
	switch operation {
	case "ls":
		value.List = &ProtectedFileListV1{Path: "/synthetic/workspace", RelativePath: ".", Entries: []ProtectedFileEntryV1{{Name: "source.go", DisplayName: "source.go", Kind: "file"}, {Name: "nested", DisplayName: "nested/", Kind: "directory"}}, Names: []string{"source.go", "nested/"}, Limit: 1000, OriginalCount: 2}
	case "find":
		value.Find = &ProtectedFileFindV1{Path: "/synthetic/workspace", RelativePath: ".", Pattern: "**/*.go", Matches: []ProtectedFileMatchV1{{Path: "/synthetic/workspace/source.go", RelativePath: "source.go"}}, SkippedProtected: []string{"private"}, SkippedProtectedOriginalCount: 1, Limit: 1000, OriginalCount: 1}
	case "code_index":
		value.CodeIndex = &ProtectedFileCodeIndexV1{Action: "outline", Path: "/synthetic/workspace/source.go", RelativePath: "source.go", Limit: 1, Symbols: []ProtectedFileSymbolV1{{Name: "Example", Kind: "function", File: "source.go", Path: "/synthetic/workspace/source.go", Line: 1, Parent: "", Signature: "func Example()"}}, SymbolCount: 1, OriginalCount: 1, Text: protectedFileResultStringForTestV1("source.go:1 Example\n"), SkippedProtected: []string{"private.go"}, SkippedProtectedOriginalCount: 1, SkippedDirectories: []string{"vendor"}, SkippedDirectoriesOriginalCount: 1, LimitReached: true}
	case "move_file":
		value.Move = &ProtectedFileMoveV1{SourcePath: "/synthetic/workspace/source.go", DestinationPath: "/synthetic/workspace/destination.go", SourceRelativePath: "source.go", DestinationRelativePath: "destination.go", BytesMoved: "9007199254740993", Moved: true, Message: "Moved file"}
	case "notebook_edit":
		value.NotebookEdit = &ProtectedFileNotebookEditV1{Path: "/synthetic/workspace/book.ipynb", RelativePath: "book.ipynb", EditMode: "replace", CellNumber: 0, CellCount: 1, CellType: "code", CellID: protectedFileResultStringForTestV1("cell-a"), Message: "Updated cell", BytesWritten: "37", FirstChangedLine: 0, Diff: diff}
	case "delete_range":
		value.DeleteRange = &ProtectedFileDeleteRangeV1{Path: "/synthetic/workspace/source.go", RelativePath: "source.go", StartLine: 1, EndLine: 2, FirstDeletedLine: 1, DeletedLines: 2, BytesWritten: "5", Encoding: "utf-8", Inclusive: true, Diff: diff}
	case "delete_symbol":
		value.DeleteSymbol = &ProtectedFileDeleteSymbolV1{Path: "/synthetic/workspace/source.go", RelativePath: "source.go", Name: "Example", Kind: "function", Parent: protectedFileResultStringForTestV1("Container"), Line: 3, FirstDeletedLine: 3, DeletedLines: 2, BytesWritten: "5", Encoding: "utf-8", Diff: diff}
	default:
		panic("unknown protected file result test operation")
	}
	return value
}

func protectedFileResultCaptureForTestV1(t *testing.T, operation string) ProtectedCaptureV1 {
	t.Helper()
	capture, ok := BoundProtectedFileResultV1(protectedFileResultFixtureForTestV1(operation))
	if !ok {
		t.Fatalf("valid %s producer fixture was refused", operation)
	}
	return capture
}

func protectedFileResultSnapshotForTestV1(t *testing.T, operation string) ProtectedSnapshotV1 {
	t.Helper()
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	callID := "call_host_" + strings.Repeat("b", 64)
	return ProtectedSnapshotV1{Version: 1, Purpose: ProtectedSnapshotPurposeV1, Principal: principal, Workspace: "/synthetic/workspace", ThreadID: "thread-file-result", TurnID: "turn-file-result", CallID: callID, ResultItemID: ToolResultItemIDV1("turn-file-result", callID), ToolName: operation, ContextDigest: strings.Repeat("c", 64), ContextEpoch: 1, ExecutionGrantID: strings.Repeat("d", 64), CaseBindingHash: strings.Repeat("e", 64), Capture: protectedFileResultCaptureForTestV1(t, operation), Inspect: &ProtectedInspectV1{InputJSON: `{}`}}
}

func protectedFileResultJSONForTestV1(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestProtectedFileResultSevenTypedSnapshotsRoundTripV1(t *testing.T) {
	for _, operation := range protectedFileResultOperationsForTestV1 {
		t.Run(operation, func(t *testing.T) {
			snapshot := protectedFileResultSnapshotForTestV1(t, operation)
			body, err := ProtectedSnapshotBytesV1(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := ParseProtectedSnapshotV1(body)
			if err != nil || !reflect.DeepEqual(parsed.Capture.FileResult, snapshot.Capture.FileResult) {
				t.Fatal("typed producer facts did not roundtrip", err)
			}
			if parsed.Capture.Kind != "file_result" || parsed.Capture.Status != "completed" || parsed.Capture.Truncated {
				t.Fatal("successful complete observation changed its capture semantics")
			}
			changed := snapshot
			changed.ToolName = "mcp__other__" + operation
			if ValidateProtectedSnapshotV1(changed) == nil {
				t.Fatal("different tool acquired a successful file result")
			}
			changed = snapshot
			changed.Inspect = nil
			if ValidateProtectedSnapshotV1(changed) == nil {
				t.Fatal("file result acquired an invented request instead of requiring Inspect")
			}
			changed = snapshot
			changed.Capture.Kind = "read"
			if ValidateProtectedSnapshotV1(changed) == nil {
				t.Fatal("file facts escaped their exact capture kind")
			}
			for _, other := range protectedFileResultOperationsForTestV1 {
				if other == operation {
					continue
				}
				changed = snapshot
				changed.ToolName = other
				if ValidateProtectedSnapshotV1(changed) == nil {
					t.Fatal("a different admitted file tool acquired this operation's result")
				}
			}
			for _, malformed := range [][]byte{
				bytes.Replace(body, []byte(`"fileResult":`), []byte(`"unexpectedTarget":"/synthetic/private","fileResult":`), 1),
				bytes.Replace(body, []byte(`"fileResult":`), []byte(`"readTarget":null,"fileResult":`), 1),
				bytes.Replace(body, []byte(`"operation":`), []byte(`"operation":"invented","operation":`), 1),
			} {
				if _, err := ParseProtectedSnapshotV1(malformed); err == nil {
					t.Fatal("open, nullable or duplicate capture JSON accepted")
				}
			}
		})
	}
	value := protectedFileResultFixtureForTestV1("move_file")
	value.Move.Moved, value.Move.BytesMoved, value.Move.Message = false, "0", "Source and destination are the same file"
	if capture, ok := BoundProtectedFileResultV1(value); !ok || capture.FileResult.Move.Moved {
		t.Fatal("successful no-op was rewritten as a move")
	}
}

// Visit every object of each native fixture so strictness is checked inside
// array entries and diffs as well as at the discriminated-union boundary.
func protectedFileResultObjectPathsForTestV1(value any, path []any) [][]any {
	var paths [][]any
	switch typed := value.(type) {
	case map[string]any:
		paths = append(paths, append([]any(nil), path...))
		for key, child := range typed {
			paths = append(paths, protectedFileResultObjectPathsForTestV1(child, append(append([]any(nil), path...), key))...)
		}
	case []any:
		for index, child := range typed {
			paths = append(paths, protectedFileResultObjectPathsForTestV1(child, append(append([]any(nil), path...), index))...)
		}
	}
	return paths
}

func protectedFileResultJSONObjectForTestV1(t *testing.T, body []byte, path []any) (map[string]any, map[string]any) {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	var current any = root
	for _, segment := range path {
		switch typed := segment.(type) {
		case string:
			current = current.(map[string]any)[typed]
		case int:
			current = current.([]any)[typed]
		}
	}
	return root, current.(map[string]any)
}

func TestProtectedFileResultJSONRejectsMissingUnknownAndNullFactsV1(t *testing.T) {
	for _, operation := range protectedFileResultOperationsForTestV1 {
		t.Run(operation, func(t *testing.T) {
			body := protectedFileResultJSONForTestV1(t, protectedFileResultFixtureForTestV1(operation))
			var root map[string]any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			for _, path := range protectedFileResultObjectPathsForTestV1(root, nil) {
				_, object := protectedFileResultJSONObjectForTestV1(t, body, path)
				for key := range object {
					t.Run(fmt.Sprint(path)+"/null/"+key, func(t *testing.T) {
						changed, at := protectedFileResultJSONObjectForTestV1(t, body, path)
						at[key] = nil
						var parsed ProtectedFileResultV1
						if json.Unmarshal(protectedFileResultJSONForTestV1(t, changed), &parsed) == nil {
							t.Fatal("null fact accepted")
						}
					})
					// These exact optional producer facts may be absent. The
					// code-index symbol's parent is required even when empty.
					optional := key == "patch" || key == "cellId" || key == "parent" && operation == "delete_symbol"
					if optional {
						continue
					}
					t.Run(fmt.Sprint(path)+"/missing/"+key, func(t *testing.T) {
						changed, at := protectedFileResultJSONObjectForTestV1(t, body, path)
						delete(at, key)
						var parsed ProtectedFileResultV1
						if json.Unmarshal(protectedFileResultJSONForTestV1(t, changed), &parsed) == nil {
							t.Fatal("missing producer fact defaulted to a successful value")
						}
					})
				}
				t.Run(fmt.Sprint(path)+"/unknown", func(t *testing.T) {
					changed, at := protectedFileResultJSONObjectForTestV1(t, body, path)
					at["untrustedExtra"] = true
					var parsed ProtectedFileResultV1
					if json.Unmarshal(protectedFileResultJSONForTestV1(t, changed), &parsed) == nil {
						t.Fatal("unknown nested field accepted")
					}
				})
			}
			for other, branch := range map[string]string{"ls": "list", "find": "find", "code_index": "codeIndex", "move_file": "move", "notebook_edit": "notebookEdit", "delete_range": "deleteRange", "delete_symbol": "deleteSymbol"} {
				if _, exists := root[branch]; exists {
					continue
				}
				changed, _ := protectedFileResultJSONObjectForTestV1(t, body, nil)
				otherBody := protectedFileResultJSONForTestV1(t, protectedFileResultFixtureForTestV1(other))
				otherValue, _ := protectedFileResultJSONObjectForTestV1(t, otherBody, nil)
				changed[branch] = otherValue[branch]
				var parsed ProtectedFileResultV1
				if json.Unmarshal(protectedFileResultJSONForTestV1(t, changed), &parsed) == nil {
					t.Fatal("second operation branch accepted")
				}
			}
		})
	}
	for _, body := range []string{`null`, `[]`, `true`, `"ls"`, `{}`, `{"operation":"ls","classification":"source_exact_local"}`, `{"operation":"ls","classification":"source_exact_local"} {}`} {
		var parsed ProtectedFileResultV1
		if json.Unmarshal([]byte(body), &parsed) == nil {
			t.Fatal("non-object, incomplete or trailing result JSON accepted")
		}
	}
	value := protectedFileResultFixtureForTestV1("ls")
	value.Classification = "public"
	if ValidateProtectedFileResultV1(value) == nil {
		t.Fatal("unadmitted classification accepted")
	}
}

func TestProtectedFileResultCaptureIsSuccessfulReadonlyAndKindClosedV1(t *testing.T) {
	mutations := []struct {
		name   string
		change func(*ProtectedCaptureV1)
	}{
		{"failed", func(c *ProtectedCaptureV1) { c.Status = "failed" }},
		{"canceled", func(c *ProtectedCaptureV1) { c.Status = "canceled" }},
		{"timeout", func(c *ProtectedCaptureV1) { c.Status = "timeout" }},
		{"unknown", func(c *ProtectedCaptureV1) { c.Status = "unknown" }},
		{"body", func(c *ProtectedCaptureV1) { c.Body = "invented result" }},
		{"label", func(c *ProtectedCaptureV1) { c.Label = "source.go" }},
		{"exit", func(c *ProtectedCaptureV1) { n := 0; c.ExitCode = &n }},
		{"duration", func(c *ProtectedCaptureV1) { c.DurationMs = 1 }},
		{"start", func(c *ProtectedCaptureV1) { c.StartLine = 1 }},
		{"end", func(c *ProtectedCaptureV1) { c.EndLine = 1 }},
		{"total", func(c *ProtectedCaptureV1) { c.TotalLines = 1 }},
		{"read-target", func(c *ProtectedCaptureV1) {
			c.ReadTarget = &ProtectedReadTargetV1{Path: "/synthetic/source.go", StartLine: 1, ContentDigest: strings.Repeat("a", 64)}
		}},
		{"web-target", func(c *ProtectedCaptureV1) { c.WebTargets = &ProtectedWebTargetsV1{ResultURL: "https://example.test/"} }},
		{"present-target", func(c *ProtectedCaptureV1) { c.PresentTargets = []ProtectedPresentTargetV1{{}} }},
		{"other-result", func(c *ProtectedCaptureV1) { c.Search = &ProtectedSearchCaptureV1{} }},
		{"fake-clipping", func(c *ProtectedCaptureV1) { c.Truncated = true }},
		{"missing-result", func(c *ProtectedCaptureV1) { c.FileResult = nil }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			capture := protectedFileResultCaptureForTestV1(t, "ls")
			mutation.change(&capture)
			if ValidateProtectedCaptureV1(capture) == nil {
				t.Fatal("file result gained an error outcome, second payload or effect authority")
			}
		})
	}
	value := protectedFileResultFixtureForTestV1("ls")
	value.Operation = "read"
	if ValidateProtectedFileResultV1(value) == nil {
		t.Fatal("unadmitted operation acquired the new capture")
	}
}

func TestProtectedFileResultDecimalBytesAndSafeIntegerFactsV1(t *testing.T) {
	for _, count := range []string{"0", "9007199254740993", "9223372036854775807"} {
		value := protectedFileResultFixtureForTestV1("move_file")
		value.Move.BytesMoved = count
		body := protectedFileResultJSONForTestV1(t, value)
		var parsed ProtectedFileResultV1
		if json.Unmarshal(body, &parsed) != nil || parsed.Move.BytesMoved != count {
			t.Fatalf("exact decimal byte count %s was lost", count)
		}
	}
	for _, count := range []string{"", "-1", "+1", "01", " 1", "1 ", "1.0", "1e3", "9223372036854775808"} {
		value := protectedFileResultFixtureForTestV1("move_file")
		value.Move.BytesMoved = count
		if ValidateProtectedFileResultV1(value) == nil {
			t.Fatalf("noncanonical or overflowing byte count %q accepted", count)
		}
	}
	for _, operation := range []string{"notebook_edit", "delete_range", "delete_symbol"} {
		value := protectedFileResultFixtureForTestV1(operation)
		switch operation {
		case "notebook_edit":
			value.NotebookEdit.BytesWritten = "01"
		case "delete_range":
			value.DeleteRange.BytesWritten = "01"
		case "delete_symbol":
			value.DeleteSymbol.BytesWritten = "01"
		}
		if ValidateProtectedFileResultV1(value) == nil {
			t.Fatal("mutation byte count did not use the same exact-decimal contract")
		}
	}
	for _, count := range []int{-1, 9007199254740992} {
		for _, operation := range []string{"find", "code_index", "notebook_edit", "delete_range", "delete_symbol"} {
			value := protectedFileResultFixtureForTestV1(operation)
			switch operation {
			case "find":
				value.Find.SkippedProtectedOriginalCount = count
			case "code_index":
				value.CodeIndex.Symbols[0].Line = count
			case "notebook_edit":
				value.NotebookEdit.CellNumber = count
			case "delete_range":
				value.DeleteRange.DeletedLines = count
			case "delete_symbol":
				value.DeleteSymbol.Diff.Added = count
			}
			if ValidateProtectedFileResultV1(value) == nil {
				t.Fatalf("%s accepted unsafe integer %d", operation, count)
			}
		}
	}
}

func TestProtectedFileResultDiffPreservesProducerOmissionSemanticsV1(t *testing.T) {
	for _, operation := range []string{"notebook_edit", "delete_range", "delete_symbol"} {
		for _, name := range []string{"binary-patch", "binary-truncated", "false-binary-reason", "false-large-reason", "patch-and-unavailable"} {
			t.Run(operation+"/"+name, func(t *testing.T) {
				value := protectedFileResultFixtureForTestV1(operation)
				diff := fileResultDiffV1(value)
				switch name {
				case "binary-patch":
					diff.Binary, diff.OmittedReason = true, "binary"
				case "binary-truncated":
					diff.Patch, diff.Binary, diff.Truncated, diff.OmittedReason = nil, true, true, "binary"
				case "false-binary-reason":
					diff.OmittedReason = "binary"
				case "false-large-reason":
					diff.OmittedReason = "large_change"
				case "patch-and-unavailable":
					diff.PatchUnavailable = "limit"
				}
				if ValidateProtectedFileResultV1(value) == nil {
					t.Fatal("contradictory producer diff facts accepted")
				}
			})
		}
		for _, reason := range []string{"binary", "large_change"} {
			value := protectedFileResultFixtureForTestV1(operation)
			diff := fileResultDiffV1(value)
			diff.Patch, diff.OmittedReason = nil, reason
			diff.Binary, diff.Truncated = reason == "binary", reason == "large_change"
			capture, ok := BoundProtectedFileResultV1(value)
			if !ok || capture.Truncated {
				t.Fatal("producer diff omission became local capture clipping")
			}
		}
	}
}

func protectedFileResultMutateOwnedFactsForTestV1(value *ProtectedFileResultV1, marker string) {
	if d := value.List; d != nil {
		d.Path, d.RelativePath = marker, marker
		d.Entries[0].Name, d.Entries[0].DisplayName, d.Names[0] = marker, marker, marker
	}
	if d := value.Find; d != nil {
		d.Path, d.RelativePath, d.Pattern = marker, marker, marker
		d.Matches[0].Path, d.Matches[0].RelativePath, d.SkippedProtected[0] = marker, marker, marker
	}
	if d := value.CodeIndex; d != nil {
		d.Path, d.RelativePath, d.Query, d.Kind = marker, marker, marker, marker
		d.Symbols[0].Name, d.Symbols[0].Kind, d.Symbols[0].File, d.Symbols[0].Path, d.Symbols[0].Parent, d.Symbols[0].Signature = marker, marker, marker, marker, marker, marker
		*d.Text = marker
		d.SkippedProtected[0], d.SkippedDirectories[0] = marker, marker
	}
	if d := value.Move; d != nil {
		d.SourcePath, d.SourceRelativePath, d.DestinationPath, d.DestinationRelativePath, d.Message = marker, marker, marker, marker, marker
	}
	if d := value.NotebookEdit; d != nil {
		d.Path, d.RelativePath, d.CellType, d.Message, *d.CellID = marker, marker, marker, marker, marker
	}
	if d := value.DeleteRange; d != nil {
		d.Path, d.RelativePath, d.Encoding = marker, marker, marker
	}
	if d := value.DeleteSymbol; d != nil {
		d.Path, d.RelativePath, d.Name, d.Kind, d.Encoding, *d.Parent = marker, marker, marker, marker, marker, marker
	}
	if diff := fileResultDiffV1(value); diff != nil {
		*diff.Patch = marker
	}
}

func TestProtectedFileResultCollectorOwnsAllNestedFactsAndSealsOnceV1(t *testing.T) {
	for _, operation := range protectedFileResultOperationsForTestV1 {
		t.Run(operation, func(t *testing.T) {
			capture := protectedFileResultCaptureForTestV1(t, operation)
			original := protectedFileResultJSONForTestV1(t, capture)
			ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
			CaptureProtectedToolResultV1(ctx, capture)
			protectedFileResultMutateOwnedFactsForTestV1(capture.FileResult, "changed after capture")
			owned, present := ConsumeProtectedToolResultV1(ctx)
			if !present || !bytes.Equal(protectedFileResultJSONForTestV1(t, owned), original) {
				t.Fatal("collector borrowed a mutable branch, slice, text or diff")
			}
			CaptureProtectedToolResultV1(ctx, protectedFileResultCaptureForTestV1(t, operation))
			if _, present := ConsumeProtectedToolResultV1(ctx); present {
				t.Fatal("consumed collector revived for a late producer")
			}
		})
	}
	for _, sequence := range []string{"same", "different", "invalid-first"} {
		t.Run(sequence, func(t *testing.T) {
			ctx := WithProtectedCaptureV1(context.Background(), &ProtectedCaptureCollectorV1{})
			first := protectedFileResultCaptureForTestV1(t, "ls")
			if sequence == "invalid-first" {
				first.Status = "failed"
			}
			CaptureProtectedToolResultV1(ctx, first)
			operation := "ls"
			if sequence == "different" {
				operation = "find"
			}
			CaptureProtectedToolResultV1(ctx, protectedFileResultCaptureForTestV1(t, operation))
			if _, present := ConsumeProtectedToolResultV1(ctx); present {
				t.Fatal("duplicate or invalid producer selected a successful result")
			}
		})
	}
}

func TestProtectedFileResultBoundKeepsWholeRecordPrefixesAndOriginalCountsV1(t *testing.T) {
	for _, producerTruncated := range []bool{false, true} {
		for _, operation := range []string{"ls", "find", "code_index"} {
			t.Run(fmt.Sprintf("%s/producerTruncated=%t", operation, producerTruncated), func(t *testing.T) {
				value := protectedFileResultFixtureForTestV1(operation)
				for index := 0; index < 20; index++ {
					name := fmt.Sprintf("%02d-", index) + strings.Repeat("界", 900)
					switch operation {
					case "ls":
						if index == 0 {
							value.List.Entries, value.List.Names = []ProtectedFileEntryV1{}, []string{}
						}
						value.List.Entries = append(value.List.Entries, ProtectedFileEntryV1{Name: name, DisplayName: name, Kind: "file"})
						value.List.Names = append(value.List.Names, name)
					case "find":
						if index == 0 {
							value.Find.Matches = []ProtectedFileMatchV1{}
						}
						value.Find.Matches = append(value.Find.Matches, ProtectedFileMatchV1{Path: "/synthetic/" + name, RelativePath: name})
					case "code_index":
						if index == 0 {
							value.CodeIndex.Symbols = []ProtectedFileSymbolV1{}
						}
						value.CodeIndex.Symbols = append(value.CodeIndex.Symbols, ProtectedFileSymbolV1{Name: name, Kind: "function", File: "source.go", Path: "/synthetic/source.go", Line: index + 1, Signature: "func " + name + "()"})
					}
				}
				switch operation {
				case "ls":
					value.List.OriginalCount, value.List.ProducerTruncated = 20, producerTruncated
				case "find":
					value.Find.OriginalCount, value.Find.ProducerTruncated = 20, producerTruncated
				case "code_index":
					value.CodeIndex.Limit, value.CodeIndex.SymbolCount, value.CodeIndex.OriginalCount, value.CodeIndex.ProducerTruncated, value.CodeIndex.LimitReached = 20, 20, 20, producerTruncated, true
				}
				original := protectedFileResultJSONForTestV1(t, value)
				if len(original) <= ProtectedFileResultLimitV1 {
					t.Fatal("budget fixture is too small")
				}
				// Preserve native producer rows before Bound changes the
				// retained slices. Oversized producer values are intentionally
				// not fed through the bounded wire decoder.
				var originalValue ProtectedFileResultV1
				switch operation {
				case "ls":
					copy_ := *value.List
					copy_.Entries = append([]ProtectedFileEntryV1(nil), value.List.Entries...)
					copy_.Names = append([]string(nil), value.List.Names...)
					originalValue.List = &copy_
				case "find":
					copy_ := *value.Find
					copy_.Matches = append([]ProtectedFileMatchV1(nil), value.Find.Matches...)
					copy_.SkippedProtected = append([]string(nil), value.Find.SkippedProtected...)
					originalValue.Find = &copy_
				case "code_index":
					copy_ := *value.CodeIndex
					copy_.Symbols = append([]ProtectedFileSymbolV1(nil), value.CodeIndex.Symbols...)
					copy_.SkippedProtected = append([]string(nil), value.CodeIndex.SkippedProtected...)
					copy_.SkippedDirectories = append([]string(nil), value.CodeIndex.SkippedDirectories...)
					if value.CodeIndex.Text != nil {
						text := *value.CodeIndex.Text
						copy_.Text = &text
					}
					originalValue.CodeIndex = &copy_
				}
				capture, ok := BoundProtectedFileResultV1(value)
				if !ok || !capture.Truncated || ValidateProtectedCaptureV1(capture) != nil {
					t.Fatal("over-budget record set was not captured as a bounded omission")
				}
				if body := protectedFileResultJSONForTestV1(t, capture.FileResult); len(body) > 32<<10 || !utf8.Valid(body) {
					t.Fatal("serialized JSON exceeded its byte budget or broke UTF-8")
				}
				switch operation {
				case "ls":
					d := capture.FileResult.List
					if len(d.Entries) == 0 || len(d.Entries) >= 20 || d.OriginalCount != 20 || d.ProducerTruncated != producerTruncated || !reflect.DeepEqual(d.Entries, originalValue.List.Entries[:len(d.Entries)]) || !reflect.DeepEqual(d.Names, originalValue.List.Names[:len(d.Names)]) {
						t.Fatal("list count, order, names or producer truncation was rewritten")
					}
				case "find":
					d := capture.FileResult.Find
					if len(d.Matches) == 0 || len(d.Matches) >= 20 || d.OriginalCount != 20 || d.ProducerTruncated != producerTruncated || !reflect.DeepEqual(d.Matches, originalValue.Find.Matches[:len(d.Matches)]) || d.SkippedProtectedOriginalCount != 1 {
						t.Fatal("find retained a partial record or changed producer facts")
					}
				case "code_index":
					d := capture.FileResult.CodeIndex
					if len(d.Symbols) == 0 || len(d.Symbols) >= 20 || d.OriginalCount != 20 || d.SymbolCount != 20 || d.ProducerTruncated != producerTruncated || !d.LimitReached || !reflect.DeepEqual(d.Symbols, originalValue.CodeIndex.Symbols[:len(d.Symbols)]) {
						t.Fatal("index count, order or independent producer flags were rewritten")
					}
				}
			})
		}
	}
}

func TestProtectedFileResultBoundOmitsWholeTextAndPatchWithoutSlicingV1(t *testing.T) {
	for _, operation := range []string{"code_index", "notebook_edit", "delete_range", "delete_symbol"} {
		for _, text := range []string{strings.Repeat("界", 12000), strings.Repeat("\n", 20000)} {
			value := protectedFileResultFixtureForTestV1(operation)
			if operation == "code_index" {
				value.CodeIndex.Text = &text
			} else {
				fileResultDiffV1(value).Patch = &text
			}
			capture, ok := BoundProtectedFileResultV1(value)
			if !ok || !capture.Truncated {
				t.Fatal("oversized complete text was not explicitly omitted")
			}
			if operation == "code_index" {
				d := capture.FileResult.CodeIndex
				if d.Text != nil || d.TextUnavailable != "limit" || d.SymbolCount != 1 || d.ProducerTruncated || !d.LimitReached {
					t.Fatal("index text became a misleading prefix or changed producer counts")
				}
			} else {
				diff := fileResultDiffV1(capture.FileResult)
				if diff.Patch != nil || diff.PatchUnavailable != "limit" || diff.Truncated || diff.OmittedReason != "" || diff.Added != 1 || diff.Removed != 2 {
					t.Fatal("patch was sliced or local omission became producer truncation")
				}
			}
			if len(protectedFileResultJSONForTestV1(t, capture.FileResult)) > ProtectedFileResultLimitV1 {
				t.Fatal("text omission did not restore the serialized budget")
			}
		}
	}
	for _, text := range []string{"invalid\x00text", string([]byte{0xff, 0xfe})} {
		for _, operation := range []string{"code_index", "delete_range"} {
			value := protectedFileResultFixtureForTestV1(operation)
			if operation == "code_index" {
				value.CodeIndex.Text = &text
			} else {
				fileResultDiffV1(value).Patch = &text
			}
			if _, ok := BoundProtectedFileResultV1(value); ok {
				t.Fatal("invalid text was hidden behind a budget omission")
			}
		}
	}
	value := protectedFileResultFixtureForTestV1("move_file")
	value.Move.Message = strings.Repeat("x", ProtectedFileResultLimitV1)
	if _, ok := BoundProtectedFileResultV1(value); ok {
		t.Fatal("unrepresentable scalar facts were replaced with a successful empty result")
	}
}

func TestProtectedFileResultProjectionRedactsAllTextAndOwnsItsInputV1(t *testing.T) {
	for _, operation := range protectedFileResultOperationsForTestV1 {
		t.Run(operation, func(t *testing.T) {
			value := protectedFileResultFixtureForTestV1(operation)
			secret := "SyntheticOpaqueCredentialForFileResult12345"
			protectedFileResultMutateOwnedFactsForTestV1(value, "fact "+secret+" retained")
			original := protectedFileResultJSONForTestV1(t, value)
			projected, ok := ProjectProtectedFileResultV1(value, secret)
			if !ok || projected.Classification != "credential_redacted" {
				t.Fatal("known credential was not explicitly classified as redacted")
			}
			body := protectedFileResultJSONForTestV1(t, projected)
			if bytes.Contains(body, []byte(secret)) || secretprojection.ContainsCredentialV1(string(body)) || !bytes.Contains(body, []byte("fact ")) || !bytes.Contains(body, []byte(" retained")) {
				t.Fatal("projection retained credentials or discarded surrounding facts")
			}
			expected := protectedFileResultFixtureForTestV1(operation)
			protectedFileResultMutateOwnedFactsForTestV1(expected, "fact "+secretprojection.RedactedV1+" retained")
			expected.Classification = "credential_redacted"
			if !reflect.DeepEqual(projected, expected) {
				t.Fatal("projection changed operation, counters, producer flags or unrelated text")
			}
			if !bytes.Equal(original, protectedFileResultJSONForTestV1(t, value)) {
				t.Fatal("projection mutated its producer-owned input")
			}
		})
	}
}

func TestProtectedFileResultProjectionRejectsInvalidCredentialAuthorityV1(t *testing.T) {
	for _, secrets := range [][]string{{"abc"}, {strings.Repeat("s", (1<<20)+1)}, make([]string, 65)} {
		if value, ok := ProjectProtectedFileResultV1(protectedFileResultFixtureForTestV1("ls"), secrets...); ok || value != nil {
			t.Fatal("invalid known-secret authority acquired a result")
		}
	}
	// Local synthetic test fixture; never sourced from an account.
	const syntheticFileResultCredentialV1 = "sk-0123456789abcdef0123456789abcdef"
	credentialInput := "Authorization: Bearer " + syntheticFileResultCredentialV1
	if !secretprojection.ContainsCredentialV1(credentialInput) {
		t.Fatal("synthetic credential fixture no longer exercises API-token detection")
	}
	value := protectedFileResultFixtureForTestV1("move_file")
	value.Move.Message = credentialInput
	projected, ok := ProjectProtectedFileResultV1(value)
	if !ok || projected.Classification != "credential_redacted" || secretprojection.ContainsCredentialV1(projected.Move.Message) || strings.Contains(projected.Move.Message, syntheticFileResultCredentialV1) {
		t.Fatal("credential-shaped text survived without an explicit-secret hint")
	}
	projected, ok = ProjectProtectedFileResultV1(protectedFileResultFixtureForTestV1("ls"))
	if !ok || projected.Classification != "source_exact_local" {
		t.Fatal("ordinary local facts were falsely marked redacted")
	}
	value = protectedFileResultFixtureForTestV1("ls")
	value.Classification = "credential_redacted"
	if projected, ok := ProjectProtectedFileResultV1(value); !ok || projected.Classification != "credential_redacted" {
		t.Fatal("a previously redacted observation regained exact-local classification")
	}
}

func TestProtectedFileResultJSONRejectsCaseInsensitiveFieldAliasesV1(t *testing.T) {
	branches := map[string]string{"ls": "list", "find": "find", "code_index": "codeIndex", "move_file": "move", "notebook_edit": "notebookEdit", "delete_range": "deleteRange", "delete_symbol": "deleteSymbol"}
	for _, operation := range protectedFileResultOperationsForTestV1 {
		t.Run(operation, func(t *testing.T) {
			body := protectedFileResultJSONForTestV1(t, protectedFileResultFixtureForTestV1(operation))
			for _, level := range []string{"operation", "path"} {
				changed, _ := protectedFileResultJSONObjectForTestV1(t, body, nil)
				if level == "operation" {
					changed["Operation"] = changed["operation"]
				} else {
					branch := changed[branches[operation]].(map[string]any)
					if operation == "move_file" {
						branch["SourcePath"] = branch["sourcePath"]
					} else {
						branch["Path"] = branch["path"]
					}
				}
				var parsed ProtectedFileResultV1
				if json.Unmarshal(protectedFileResultJSONForTestV1(t, changed), &parsed) == nil {
					t.Fatalf("direct DTO decoder accepted %s and its noncanonical case alias", level)
				}
			}
		})
	}
}

func TestProtectedFileResultProjectionPreservesExactWhitespaceFactsV1(t *testing.T) {
	for _, operation := range []string{"code_index", "move_file", "notebook_edit", "delete_range", "delete_symbol"} {
		for _, whitespace := range []string{"", " \t\r\n", "\u2003\t\n"} {
			t.Run(fmt.Sprintf("%s/%q", operation, whitespace), func(t *testing.T) {
				value := protectedFileResultFixtureForTestV1(operation)
				switch operation {
				case "code_index":
					value.CodeIndex.Query, value.CodeIndex.Kind = whitespace, whitespace
					value.CodeIndex.Symbols[0].Parent, value.CodeIndex.Symbols[0].Signature = whitespace, whitespace
					*value.CodeIndex.Text = whitespace
				case "move_file":
					value.Move.Message = whitespace
				case "notebook_edit":
					value.NotebookEdit.Message = whitespace
				}
				if diff := fileResultDiffV1(value); diff != nil {
					*diff.Patch = whitespace
				}
				original := protectedFileResultJSONForTestV1(t, value)
				projected, ok := ProjectProtectedFileResultV1(value)
				if !ok || projected.Classification != "source_exact_local" || !bytes.Equal(protectedFileResultJSONForTestV1(t, projected), original) {
					t.Fatal("nonsecret whitespace was normalized or mislabeled as credential redaction")
				}
				if !bytes.Equal(protectedFileResultJSONForTestV1(t, value), original) {
					t.Fatal("whitespace projection mutated producer facts")
				}
			})
		}
	}
}
