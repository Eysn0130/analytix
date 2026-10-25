package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func fileResultContextV1() context.Context {
	return domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
}

func fileResultWriteV1(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fileResultModelV1(t *testing.T, output any, failed bool) map[string]any {
	t.Helper()
	model, ok := output.(map[string]any)
	if failed || !ok {
		t.Fatalf("actual filesystem effect failed: output=%#v failed=%v", output, failed)
	}
	return model
}

func fileResultCaptureV1(t *testing.T, ctx context.Context, operation string) domaintoolresult.ProtectedCaptureV1 {
	t.Helper()
	capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !ok || capture.Kind != "file_result" || capture.Status != "completed" || capture.FileResult == nil || capture.FileResult.Operation != operation {
		t.Fatalf("actual %s producer did not emit its successful file result: %#v present=%v", operation, capture, ok)
	}
	if err := domaintoolresult.ValidateProtectedCaptureV1(capture); err != nil {
		t.Fatal(err)
	}
	if capture.Body != "" || capture.Label != "" || capture.FileResult.Classification != "source_exact_local" {
		t.Fatal("file facts escaped their typed local result or lost source classification")
	}
	if _, again := domaintoolresult.ConsumeProtectedToolResultV1(ctx); again {
		t.Fatal("producer observation was released twice")
	}
	return capture
}

func fileResultNoCaptureV1(t *testing.T, ctx context.Context) {
	t.Helper()
	if capture, ok := domaintoolresult.ConsumeProtectedToolResultV1(ctx); ok {
		t.Fatalf("failed effect emitted a success observation: %#v", capture)
	}
}

func fileResultErrorObservationV1(t *testing.T, ctx context.Context, model map[string]any) {
	t.Helper()
	message, ok := model["error"].(string)
	if !ok || message == "" {
		t.Fatal("actual scan failure lost its model error message")
	}
	candidate, present, observedError := domaintoolresult.ConsumeProtectedToolObservationV1(ctx)
	if present || candidate.Kind != "" || candidate.FileResult != nil {
		t.Fatal("scan failure emitted a successful protected candidate")
	}
	if observedError == nil || observedError.Availability != "available" || observedError.Reason != "" || len(observedError.Entries) != 1 || observedError.Entries[0].Message != message || observedError.Entries[0].Classification != "source_exact_local" {
		t.Fatal("protected scan error did not preserve the same single actual model error")
	}
	if err := domaintoolresult.ValidateProtectedOutputErrorV1(observedError); err != nil {
		t.Fatal(err)
	}
	if _, present, again := domaintoolresult.ConsumeProtectedToolObservationV1(ctx); present || again != nil {
		t.Fatal("scan error observation was released twice")
	}
}

func fileResultIntV1(t *testing.T, value any) int {
	t.Helper()
	switch number := value.(type) {
	case int:
		return number
	case float64:
		if number == float64(int(number)) {
			return int(number)
		}
	}
	t.Fatalf("expected an exact model integer, got %#v", value)
	return 0
}

func fileResultDiffMatchesV1(t *testing.T, got domaintoolresult.ProtectedFileDiffV1, model map[string]any) {
	t.Helper()
	if got.Kind != model["diff_kind"] || got.Added != fileResultIntV1(t, model["added"]) || got.Removed != fileResultIntV1(t, model["removed"]) {
		t.Fatalf("capture diff changed actual mutation counts: %#v model=%#v", got, model)
	}
	if patch, ok := model["diff"].(string); ok {
		if got.Patch == nil || *got.Patch != patch {
			t.Fatal("capture patch differed from the actual mutation result")
		}
	} else if got.Patch != nil {
		t.Fatal("capture invented a patch missing from the actual mutation result")
	}
	binary, _ := model["diff_binary"].(bool)
	truncated, _ := model["diff_truncated"].(bool)
	omitted, _ := model["diff_omitted_reason"].(string)
	if got.Binary != binary || got.Truncated != truncated || got.OmittedReason != omitted || got.PatchUnavailable != "" {
		t.Fatal("capture changed the producer diff availability")
	}
}

func TestFileResultListCapturesActualEntriesNamesAndProducerLimitV1(t *testing.T) {
	root := t.TempDir()
	fileResultWriteV1(t, root, "a.txt", "alpha")
	fileResultWriteV1(t, root, "z-dir/nested.txt", "nested")
	fileResultWriteV1(t, root, ".analytix/hidden.txt", "synthetic protected host metadata")
	for _, limit := range []int{20, 1} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			ctx := fileResultContextV1()
			output, failed := ExecuteListTool(WorkspaceSearchToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", Args: map[string]any{"path": ".", "limit": limit}})
			model := fileResultModelV1(t, output, failed)
			capture := fileResultCaptureV1(t, ctx, "ls")
			got := capture.FileResult.List
			rows, ok := model["entries"].([]map[string]any)
			expectedCount := 2
			if limit == 1 {
				expectedCount = 1
			}
			if !ok || got.Path != model["path"] || got.RelativePath != model["relative_path"] || got.RelativePath != "." || got.Limit != limit || model["entry_limit_reached"] != float64(limit) {
				t.Fatal("ls path or numeric producer limit changed")
			}
			if got.OriginalCount != expectedCount || len(rows) != expectedCount || len(got.Entries) != len(rows) || !reflect.DeepEqual(got.Names, model["names"]) || got.ProducerTruncated != (limit == 1) || model["truncated"] != got.ProducerTruncated || capture.Truncated {
				t.Fatal("ls lost producer count, order, names or truncation distinction")
			}
			for index, row := range rows {
				entry := got.Entries[index]
				if entry.Name != row["name"] || entry.DisplayName != row["display_name"] || entry.Kind != row["kind"] || got.Names[index] != entry.DisplayName || entry.Name == ".analytix" {
					t.Fatal("ls entry was reconstructed, reordered or exposed host metadata")
				}
			}
			if got.Entries[0].Name != "a.txt" || limit == 20 && (got.Entries[1].DisplayName != "z-dir/" || got.Entries[1].Kind != "directory") {
				t.Fatal("real file/directory display facts were lost")
			}
		})
	}
}

func TestFileResultFindCapturesActualMatchesAndProtectedPruningV1(t *testing.T) {
	root := t.TempDir()
	fileResultWriteV1(t, root, "a.match", "a")
	fileResultWriteV1(t, root, "z.match", "z")
	fileResultWriteV1(t, root, "ignore.txt", "ignore")
	protected := filepath.Join(root, "private")
	fileResultWriteV1(t, root, "private/hidden.match", "synthetic hidden fixture")
	for _, limit := range []int{20, 1} {
		t.Run(fmt.Sprintf("limit-%d", limit), func(t *testing.T) {
			ctx := fileResultContextV1()
			output, failed := ExecuteFindTool(WorkspaceSearchToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ProtectedReadDirs: []string{protected}, Args: map[string]any{"path": ".", "pattern": "*.match", "limit": limit}})
			model := fileResultModelV1(t, output, failed)
			capture := fileResultCaptureV1(t, ctx, "find")
			got := capture.FileResult.Find
			rows, ok := model["matches"].([]map[string]any)
			expectedCount := 2
			if limit == 1 {
				expectedCount = 1
			}
			if !ok || got.Path != model["path"] || got.RelativePath != model["relative_path"] || got.Pattern != "*.match" || got.Limit != limit || model["result_limit_reached"] != float64(limit) {
				t.Fatal("find path, pattern or numeric producer limit changed")
			}
			if len(got.Matches) != expectedCount || len(rows) != expectedCount || got.OriginalCount != len(rows) || got.ProducerTruncated != (limit == 1) || got.ProducerTruncated != model["truncated"] || capture.Truncated {
				t.Fatal("find confused producer and capture truncation")
			}
			for index, row := range rows {
				if got.Matches[index].Path != row["path"] || got.Matches[index].RelativePath != row["relative_path"] || strings.Contains(got.Matches[index].RelativePath, "hidden") {
					t.Fatal("find capture differs from real ordered matches")
				}
			}
			if got.Matches[0].RelativePath != "a.match" {
				t.Fatal("find did not preserve the first real match")
			}
			if limit == 20 && (!reflect.DeepEqual(got.SkippedProtected, model["skipped_protected"]) || got.SkippedProtectedOriginalCount != 1 || got.SkippedProtected[0] != "private") {
				t.Fatal("find protected pruning was not captured as observed")
			}
		})
	}
}

func TestFileResultCodeIndexKeepsPostFilterLimitCountAndTextV1(t *testing.T) {
	root := t.TempDir()
	fileResultWriteV1(t, root, "sample.go", "package sample\ntype Widget struct{}\nfunc KeepAlpha() {}\nfunc KeepBeta() {}\nfunc Other() {}\n")
	fileResultWriteV1(t, root, "node_modules/ignored.go", "package ignored\nfunc KeepHiddenDependency() {}\n")
	protected := filepath.Join(root, "private")
	fileResultWriteV1(t, root, "private/hidden.go", "package hidden\nfunc KeepPrivate() {}\n")
	for _, test := range []struct {
		name         string
		query        string
		count        int
		truncated    bool
		limitReached bool
	}{
		{name: "filter-before-limit", query: "Keep", count: 1, truncated: true, limitReached: true},
		{name: "exact-limit-is-not-truncated", query: "KeepAlpha", count: 1, limitReached: true},
		{name: "no-symbols", query: "Missing", count: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := fileResultContextV1()
			output, failed := ExecuteCodeIndexTool(WorkspaceSearchToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ProtectedReadDirs: []string{protected}, Args: map[string]any{"path": ".", "action": "search", "query": test.query, "kind": "func", "limit": 1}})
			model := fileResultModelV1(t, output, failed)
			capture := fileResultCaptureV1(t, ctx, "code_index")
			got := capture.FileResult.CodeIndex
			rows, ok := model["symbols"].([]map[string]any)
			if !ok || got.Action != "search" || got.Query != test.query || got.Kind != "func" || got.Limit != 1 || got.Path != model["path"] || got.RelativePath != model["relative_path"] {
				t.Fatal("code_index request/display facts changed")
			}
			if got.SymbolCount != test.count || got.OriginalCount != test.count || len(got.Symbols) != test.count || got.SymbolCount != fileResultIntV1(t, model["symbol_count"]) || got.ProducerTruncated != test.truncated || got.ProducerTruncated != model["truncated"] || got.LimitReached != test.limitReached || got.LimitReached != model["result_limit_reached"] || capture.Truncated {
				t.Fatal("code_index lost filtered count or independent truncation facts")
			}
			if got.Text == nil || *got.Text != model["text"] || got.TextUnavailable != "" || !reflect.DeepEqual(got.SkippedProtected, model["skipped_protected"]) || !reflect.DeepEqual(got.SkippedDirectories, model["skipped_directories"]) || got.SkippedProtectedOriginalCount != 1 || got.SkippedDirectoriesOriginalCount != 1 {
				t.Fatal("code_index text or scan-pruning facts changed")
			}
			for index, row := range rows {
				symbol := got.Symbols[index]
				if symbol.Name != row["name"] || symbol.Kind != row["kind"] || symbol.File != row["file"] || symbol.Path != row["path"] || symbol.Line != fileResultIntV1(t, row["line"]) || symbol.Parent != row["parent"] || symbol.Signature != row["signature"] || symbol.Name != "KeepAlpha" || symbol.Line != 3 {
					t.Fatal("code_index capture did not use the returned filtered symbol")
				}
			}
		})
	}
}

type fileResultMutationFixtureV1 struct {
	tool   string
	path   string
	before string
	args   map[string]any
	run    func(MutationToolInput) (any, bool)
}

func fileResultMutationFixturesV1() []fileResultMutationFixtureV1 {
	return []fileResultMutationFixtureV1{
		{tool: "notebook_edit", path: "notes.ipynb", before: `{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["before"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`, args: map[string]any{"path": "notes.ipynb", "cell_id": "intro", "new_source": "replacement", "edit_mode": "replace"}, run: ExecuteNotebookEditTool},
		{tool: "delete_range", path: "range.txt", before: "keep\r\nstart\r\nremove\r\nend\r\ntail\r\n", args: map[string]any{"path": "range.txt", "start_anchor": "start", "end_anchor": "end", "inclusive": false}, run: ExecuteDeleteRangeTool},
		{tool: "delete_symbol", path: "sample.go", before: "package sample\n\ntype Widget struct{}\n\nfunc (w *Widget) Target() {\n println(\"remove\")\n}\n\nfunc Keep() {}\n", args: map[string]any{"path": "sample.go", "name": "Target", "kind": "method", "parent": "Widget"}, run: ExecuteDeleteSymbolTool},
	}
}

func TestFileResultMutationsCaptureAppliedAndCheckpointedFactsV1(t *testing.T) {
	for _, fixture := range fileResultMutationFixturesV1() {
		t.Run(fixture.tool, func(t *testing.T) {
			root := t.TempDir()
			path := fileResultWriteV1(t, root, fixture.path, fixture.before)
			ctx := fileResultContextV1()
			prepared, completed := 0, 0
			input := MutationToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ToolName: fixture.tool, Args: fixture.args, Checkpoint: MutationCheckpointHooks{
				Prepare: func(actual string) (MutationCheckpointDraft, error) {
					prepared++
					body, err := os.ReadFile(actual)
					if err != nil || string(body) != fixture.before {
						t.Fatal("checkpoint preparation did not precede the real effect", err)
					}
					return MutationCheckpointDraft{Enabled: true}, nil
				},
				Complete: func(draft MutationCheckpointDraft, actual string) error {
					completed++
					body, err := os.ReadFile(actual)
					if err != nil || !draft.Enabled || string(body) == fixture.before {
						t.Fatal("checkpoint completion did not observe the applied effect", err)
					}
					return nil
				},
			}}
			output, failed := fixture.run(input)
			model := fileResultModelV1(t, output, failed)
			capture := fileResultCaptureV1(t, ctx, fixture.tool)
			body, err := os.ReadFile(path)
			if err != nil || prepared != 1 || completed != 1 || string(body) == fixture.before || capture.Truncated {
				t.Fatal("real mutation/checkpoint sequence was not completed exactly once", err)
			}
			if model["relative_path"] != fixture.path || fileResultIntV1(t, model["bytes_written"]) != len(body) {
				t.Fatal("model output no longer describes the applied file")
			}
			switch fixture.tool {
			case "notebook_edit":
				d := capture.FileResult.NotebookEdit
				if d.Path != model["path"] || d.RelativePath != model["relative_path"] || d.EditMode != model["edit_mode"] || d.CellNumber != 0 || model["cell_number"] != float64(0) || d.CellCount != fileResultIntV1(t, model["cell_count"]) || d.CellType != model["cell_type"] || d.CellID == nil || *d.CellID != "intro" || d.Message != model["message"] || d.FirstChangedLine != fileResultIntV1(t, model["first_changed_line"]) || d.BytesWritten != strconv.Itoa(len(body)) || !strings.Contains(string(body), "replacement") {
					t.Fatal("notebook capture changed its 0-based selector or actual mutation facts")
				}
				fileResultDiffMatchesV1(t, d.Diff, model)
			case "delete_range":
				d := capture.FileResult.DeleteRange
				if d.Path != model["path"] || d.RelativePath != model["relative_path"] || d.StartLine != 2 || d.EndLine != 4 || d.FirstDeletedLine != 3 || d.DeletedLines != 1 || d.Inclusive || d.StartLine != fileResultIntV1(t, model["start_line"]) || d.EndLine != fileResultIntV1(t, model["end_line"]) || d.FirstDeletedLine != fileResultIntV1(t, model["first_deleted_line"]) || d.DeletedLines != fileResultIntV1(t, model["deleted_lines"]) || d.Encoding != model["encoding"] || d.BytesWritten != strconv.Itoa(len(body)) || string(body) != "keep\r\nstart\r\nend\r\ntail\r\n" {
					t.Fatal("delete_range capture changed 1-based bounds or CRLF effect")
				}
				fileResultDiffMatchesV1(t, d.Diff, model)
			case "delete_symbol":
				d := capture.FileResult.DeleteSymbol
				if d.Path != model["path"] || d.RelativePath != model["relative_path"] || d.Name != "Target" || d.Kind != "method" || d.Parent == nil || *d.Parent != "Widget" || d.Line != 5 || d.FirstDeletedLine != 5 || d.DeletedLines != 3 || d.Line != fileResultIntV1(t, model["line"]) || d.FirstDeletedLine != fileResultIntV1(t, model["first_deleted_line"]) || d.DeletedLines != fileResultIntV1(t, model["deleted_lines"]) || d.Encoding != model["encoding"] || d.BytesWritten != strconv.Itoa(len(body)) || strings.Contains(string(body), "Target") || !strings.Contains(string(body), "func Keep() {}") {
					t.Fatal("delete_symbol capture shifted original line coordinates or changed the real effect")
				}
				fileResultDiffMatchesV1(t, d.Diff, model)
			}
		})
	}
}

func TestFileResultMoveCapturesRealSettledMoveAndNoopV1(t *testing.T) {
	for _, noop := range []bool{false, true} {
		t.Run(fmt.Sprintf("noop-%t", noop), func(t *testing.T) {
			root := t.TempDir()
			source := fileResultWriteV1(t, root, "source.txt", "move fixture\n")
			destination := "nested/destination.txt"
			if noop {
				destination = "source.txt"
			}
			args := map[string]any{"source_path": "source.txt", "destination_path": destination}
			arguments, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			hooks := moveOperationHooksForTest(t, root, arguments)
			begin, settle := hooks.BeginOperation, hooks.SettleOperation
			begun, settled := 0, 0
			hooks.BeginOperation = func(paths []MutationOperationPath) (MutationOperationDraft, error) {
				begun++
				return begin(paths)
			}
			hooks.SettleOperation = func(draft MutationOperationDraft, applied bool) (string, error) {
				settled++
				if !applied {
					t.Fatal("successful move did not reach settlement as applied")
				}
				if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("move settlement preceded the real rename", err)
				}
				return settle(draft, applied)
			}
			ctx := fileResultContextV1()
			output, failed := ExecuteMoveFileTool(MutationToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ToolName: "move_file", Args: args, ArgumentsJSON: arguments, Checkpoint: hooks, MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(t.TempDir(), "authority"))})
			model := fileResultModelV1(t, output, failed)
			capture := fileResultCaptureV1(t, ctx, "move_file")
			got := capture.FileResult.Move
			body, err := os.ReadFile(filepath.Join(root, destination))
			if err != nil || string(body) != "move fixture\n" || got.SourcePath != model["source_path"] || got.DestinationPath != model["destination_path"] || got.SourceRelativePath != "source.txt" || got.DestinationRelativePath != destination || got.BytesMoved != strconv.Itoa(len(body)) || got.Moved != !noop || got.Moved != model["moved"] || got.Message != model["message"] || capture.Truncated {
				t.Fatal("move/noop capture changed actual filesystem facts", err)
			}
			if noop && (begun != 0 || settled != 0) || !noop && (begun != 1 || settled != 1) {
				t.Fatal("move/noop changed the existing checkpoint lifecycle")
			}
		})
	}
}

func TestFileResultMutationsDoNotCaptureCheckpointFailuresV1(t *testing.T) {
	for _, fixture := range fileResultMutationFixturesV1() {
		for _, stage := range []string{"prepare", "complete"} {
			t.Run(fixture.tool+"/"+stage, func(t *testing.T) {
				root := t.TempDir()
				path := fileResultWriteV1(t, root, fixture.path, fixture.before)
				ctx := fileResultContextV1()
				prepared, completed := 0, 0
				output, failed := fixture.run(MutationToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ToolName: fixture.tool, Args: fixture.args, Checkpoint: MutationCheckpointHooks{
					Prepare: func(string) (MutationCheckpointDraft, error) {
						prepared++
						if stage == "prepare" {
							return MutationCheckpointDraft{}, errors.New("synthetic checkpoint prepare failure")
						}
						return MutationCheckpointDraft{Enabled: true}, nil
					},
					Complete: func(MutationCheckpointDraft, string) error {
						completed++
						return errors.New("synthetic checkpoint completion failure")
					},
				}})
				model, ok := output.(map[string]any)
				if !failed || !ok || model["code"] != "checkpoint_unavailable" || prepared != 1 || stage == "prepare" && completed != 0 || stage == "complete" && completed != 1 {
					t.Fatal("checkpoint failure was misreported or checkpoint lifecycle repeated")
				}
				fileResultNoCaptureV1(t, ctx)
				body, err := os.ReadFile(path)
				if err != nil || stage == "prepare" && string(body) != fixture.before || stage == "complete" && string(body) == fixture.before {
					t.Fatal("checkpoint failure fixture did not exercise the expected real effect phase", err)
				}
			})
		}
	}
}

func TestFileResultMoveDoesNotCaptureOperationPrepareOrSettleFailureV1(t *testing.T) {
	for _, stage := range []string{"prepare", "settle-error", "settle-not-completed"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			source := fileResultWriteV1(t, root, "source.txt", "move failure fixture")
			args := map[string]any{"source_path": "source.txt", "destination_path": "destination.txt"}
			arguments, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			hooks := moveOperationHooksForTest(t, root, arguments)
			if stage == "prepare" {
				hooks.BeginOperation = func([]MutationOperationPath) (MutationOperationDraft, error) {
					return MutationOperationDraft{}, errors.New("synthetic begin failure")
				}
			} else {
				hooks.SettleOperation = func(MutationOperationDraft, bool) (string, error) {
					if stage == "settle-error" {
						return "failed", errors.New("synthetic settlement failure")
					}
					return "pending", nil
				}
			}
			ctx := fileResultContextV1()
			output, failed := ExecuteMoveFileTool(MutationToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ToolName: "move_file", Args: args, ArgumentsJSON: arguments, Checkpoint: hooks, MutationAuthority: mustMoveMutationAuthority(t, filepath.Join(t.TempDir(), "authority"))})
			model, ok := output.(map[string]any)
			if !failed || !ok || model["code"] != "checkpoint_unavailable" {
				t.Fatal("unsuccessful operation settlement became move success")
			}
			fileResultNoCaptureV1(t, ctx)
			if stage == "prepare" {
				if body, err := os.ReadFile(source); err != nil || string(body) != "move failure fixture" {
					t.Fatal("prepare failure moved the source", err)
				}
			} else if body, err := os.ReadFile(filepath.Join(root, "destination.txt")); err != nil || string(body) != "move failure fixture" {
				t.Fatal("settlement failure did not follow the real rename", err)
			}
		})
	}
}

func TestFileResultFailuresNeverEmitSuccessCaptureV1(t *testing.T) {
	for _, operation := range []string{"ls", "find", "code_index", "move_file", "notebook_edit", "delete_range", "delete_symbol"} {
		for _, failure := range []string{"missing", "mandatory-protected"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				root := t.TempDir()
				ctx := fileResultContextV1()
				path := "missing"
				var protected []string
				if failure == "mandatory-protected" {
					path = "private"
					fileResultWriteV1(t, root, "private/source.go", "package fixture\nfunc Target() {}\n")
					protected = []string{MandatoryProtectedRoot(filepath.Join(root, "private"))}
				}
				var output any
				var failed bool
				if operation == "ls" || operation == "find" || operation == "code_index" {
					input := WorkspaceSearchToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ProtectedReadDirs: protected, Args: map[string]any{"path": path, "pattern": "*", "action": "outline"}}
					switch operation {
					case "ls":
						output, failed = ExecuteListTool(input)
					case "find":
						output, failed = ExecuteFindTool(input)
					case "code_index":
						output, failed = ExecuteCodeIndexTool(input)
					}
				} else {
					target := filepath.Join(path, "source.go")
					if operation == "notebook_edit" {
						target = filepath.Join(path, "source.ipynb")
					}
					input := MutationToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", ToolName: operation, ProtectedReadDirs: protected, Args: map[string]any{"path": target, "source_path": target, "destination_path": "destination.go", "name": "Target", "new_source": "replacement", "cell_number": 0, "start_anchor": "start", "end_anchor": "end"}}
					switch operation {
					case "move_file":
						output, failed = ExecuteMoveFileTool(input)
					case "notebook_edit":
						output, failed = ExecuteNotebookEditTool(input)
					case "delete_range":
						output, failed = ExecuteDeleteRangeTool(input)
					case "delete_symbol":
						output, failed = ExecuteDeleteSymbolTool(input)
					}
				}
				if !failed {
					t.Fatalf("%s path was represented as success: %#v", failure, output)
				}
				if (operation == "find" || operation == "ls") && failure == "missing" {
					model, ok := output.(map[string]any)
					expectedCode := "find_failed"
					if operation == "ls" {
						expectedCode = "list_failed"
					}
					if !ok || model["code"] != expectedCode {
						t.Fatalf("missing %s root did not preserve the filesystem error: %#v", operation, output)
					}
					for _, key := range []string{"matches", "entries", "names"} {
						if _, successPayload := model[key]; successPayload {
							t.Fatalf("missing %s root emitted a successful %s payload", operation, key)
						}
					}
					fileResultErrorObservationV1(t, ctx, model)
				} else {
					fileResultNoCaptureV1(t, ctx)
				}
			})
		}
	}
}

func TestFileResultFindMissingPatternDoesNotEmitSuccessCaptureV1(t *testing.T) {
	ctx := fileResultContextV1()
	output, failed := ExecuteFindTool(WorkspaceSearchToolInput{Context: ctx, Workspace: t.TempDir(), SandboxMode: "workspace-write", Args: map[string]any{"path": ".", "pattern": ""}})
	model, ok := output.(map[string]any)
	if !failed || !ok || model["code"] != "validation_error" {
		t.Fatalf("missing find pattern was not rejected: %#v failed=%v", output, failed)
	}
	if _, successMatches := model["matches"]; successMatches {
		t.Fatal("invalid find request emitted a successful matches payload")
	}
	if _, present, observedError := domaintoolresult.ConsumeProtectedToolObservationV1(ctx); present || observedError != nil {
		t.Fatal("find parameter validation invented a protected scan observation")
	}
}

func TestFileResultFindNonDirectoryScanFailureDoesNotEmitSuccessCaptureV1(t *testing.T) {
	root := t.TempDir()
	fileResultWriteV1(t, root, "non-directory", "ordinary fixture file")
	requested := filepath.Join("non-directory", "child")
	resolved, ok := ResolveReadPathInfo(root, requested, "workspace-write", nil)
	if !ok {
		t.Fatal("non-directory intermediate fixture did not reach the filesystem scan boundary")
	}
	var pathError *os.PathError
	if _, err := os.Stat(resolved.Path); !errors.As(err, &pathError) || pathError.Path != resolved.Path {
		t.Fatalf("fixture did not produce a reliable non-directory filesystem failure: %v", err)
	}
	ctx := fileResultContextV1()
	output, failed := ExecuteFindTool(WorkspaceSearchToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", Args: map[string]any{"path": requested, "pattern": "*"}})
	model, ok := output.(map[string]any)
	if !failed || !ok || model["code"] != "find_failed" {
		t.Fatalf("non-directory scan error was not preserved: %#v failed=%v", output, failed)
	}
	if _, successMatches := model["matches"]; successMatches {
		t.Fatal("failed find scan emitted a successful matches payload")
	}
	fileResultErrorObservationV1(t, ctx, model)
}

func TestFileResultLargeListAndFindKeepWholeOrderedPrefixV1(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 180; index++ {
		fileResultWriteV1(t, root, fmt.Sprintf("%03d-%s.txt", index, strings.Repeat("x", 180)), "fixture")
	}
	for _, operation := range []string{"ls", "find"} {
		t.Run(operation, func(t *testing.T) {
			ctx := fileResultContextV1()
			input := WorkspaceSearchToolInput{Context: ctx, Workspace: root, SandboxMode: "workspace-write", Args: map[string]any{"path": ".", "pattern": "*.txt", "limit": 1000}}
			var output any
			var failed bool
			if operation == "ls" {
				output, failed = ExecuteListTool(input)
			} else {
				output, failed = ExecuteFindTool(input)
			}
			model := fileResultModelV1(t, output, failed)
			capture := fileResultCaptureV1(t, ctx, operation)
			body, err := json.Marshal(capture.FileResult)
			if err != nil || len(body) > domaintoolresult.ProtectedFileResultLimitV1 || !capture.Truncated || model["truncated"] != false {
				t.Fatal("capture clipping widened budget or changed producer completeness", err)
			}
			if operation == "ls" {
				got := capture.FileResult.List
				rows := model["entries"].([]map[string]any)
				if got.OriginalCount != 180 || len(rows) != 180 || len(got.Entries) == 0 || len(got.Entries) >= 180 || len(got.Entries) != len(got.Names) || got.ProducerTruncated || got.Limit != 1000 {
					t.Fatal("ls local clipping changed the original count or producer limit")
				}
				for index, row := range got.Entries {
					if row.Name != rows[index]["name"] || row.DisplayName != rows[index]["display_name"] || row.Kind != rows[index]["kind"] || got.Names[index] != row.DisplayName {
						t.Fatal("ls byte budget split/reordered an entry or its name")
					}
				}
			} else {
				got := capture.FileResult.Find
				rows := model["matches"].([]map[string]any)
				if got.OriginalCount != 180 || len(rows) != 180 || len(got.Matches) == 0 || len(got.Matches) >= 180 || got.ProducerTruncated || got.Limit != 1000 {
					t.Fatal("find local clipping changed the original count or producer limit")
				}
				for index, row := range got.Matches {
					if row.Path != rows[index]["path"] || row.RelativePath != rows[index]["relative_path"] {
						t.Fatal("find byte budget split/reordered a path match")
					}
				}
			}
		})
	}
}
