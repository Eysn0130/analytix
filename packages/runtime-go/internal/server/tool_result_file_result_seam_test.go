package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	snapshotstore "analytix.local/runtime-go/internal/adapters/outbound/toolresultsnapshot"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	provider "analytix.local/runtime-go/internal/provider"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

// The read guard is satisfied by a real same-turn, separately granted Read.
// No registry, captured output, grant or successful settlement is injected.
func readBeforeFileResultMutationV1(t *testing.T, h *runtimeServerHandler, pending runtimePendingToolCall, path string) {
	t.Helper()
	args, _ := json.Marshal(map[string]any{"path": path})
	call := provider.ToolCall{ID: serverTestHostToolCallID("file_result_read_" + pending.Call.Name), Name: "read", Arguments: args}
	schemas := h.runtimeToolSchemasForPromptWithGoalToolsAndMCPNames(false, pending.ToolScope, false, false, pending.Prompt, false, nil)
	grant, err := executiongrantapp.IssueProvider(pending.SecurityContext, pending.ProviderID, call, schemas, pending.ToolScope, true, "not_required", 0, "", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := h.persistToolCallReady(context.Background(), pending.ThreadID, pending.TurnID, call, 1, pending.SecurityContext, grant)
	if err != nil {
		t.Fatal(err)
	}
	read := pending
	read.Call, read.ToolCallItemID, read.ExecutionGrant = call, itemID, grant
	read.ModelVisibleToolDefinition = domainmodel.FreezeModelVisibleToolDefinitionsV1(schemas)["read"]
	read.ApprovalPolicy = "never"
	if settled, err := h.executeAndSettleRuntimeTool(context.Background(), read, nil, nil); err != nil || settled.IsError {
		t.Fatal("real same-turn Read failed", err)
	}
}

func TestFileResultSevenActualProducersSettlementPrivateHTTPAndRestartV1(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]any
		file    string
		content string
	}{
		{"ls", map[string]any{"path": "."}, "PRIVATE_FILE_RESULT.txt", "original\n"},
		{"find", map[string]any{"pattern": "PRIVATE_FILE_RESULT*.txt"}, "PRIVATE_FILE_RESULT.txt", "original\n"},
		{"code_index", map[string]any{"action": "search", "query": "Needle", "limit": float64(1)}, "PRIVATE_FILE_RESULT.go", "package fixture\nfunc NeedleAlpha() {}\nfunc NeedleBeta() {}\n"},
		{"move_file", map[string]any{"source_path": "PRIVATE_FILE_RESULT.txt", "destination_path": "moved.txt"}, "PRIVATE_FILE_RESULT.txt", "original\n"},
		{"notebook_edit", map[string]any{"path": "PRIVATE_FILE_RESULT.ipynb", "cell_id": "intro", "new_source": "new actual cell"}, "PRIVATE_FILE_RESULT.ipynb", `{"cells":[{"cell_type":"markdown","id":"intro","metadata":{},"source":["old actual cell"]}],"metadata":{},"nbformat":4,"nbformat_minor":5}`},
		{"delete_range", map[string]any{"path": "PRIVATE_FILE_RESULT.txt", "start_anchor": "start", "end_anchor": "end", "inclusive": false}, "PRIVATE_FILE_RESULT.txt", "keep\nstart\ndelete\nend\nlast\n"},
		{"delete_symbol", map[string]any{"path": "PRIVATE_FILE_RESULT.go", "name": "Gone"}, "PRIVATE_FILE_RESULT.go", "package fixture\n\nfunc Gone() {}\nfunc Keep() {}\n"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			h, pending, durable, root := protectedSeamFixtureV1(t, test.name, test.args)
			pending.ApprovalPolicy = "auto"
			path := filepath.Join(pending.Workspace, test.file)
			if err := os.WriteFile(path, []byte(test.content), 0600); err != nil {
				t.Fatal(err)
			}
			if test.name == "delete_range" || test.name == "delete_symbol" {
				readBeforeFileResultMutationV1(t, h, pending, test.file)
			}
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, nil)
			if err != nil || settled.IsError {
				t.Fatal("real filesystem producer did not succeed", err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.ToolName != test.name || display.Capture.Kind != "file_result" || display.Capture.FileResult == nil || display.Capture.FileResult.Operation != test.name || display.Capture.Status != "completed" || display.Capture.Body != "" || display.Capture.Truncated || display.ReadTarget != nil || display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) || display.Inspect.SchemaJSON != pending.ModelVisibleToolDefinition.SchemaJSON {
				t.Fatalf("actual file result lost its own private observation: http=%d", status)
			}
			result := display.Capture.FileResult
			switch test.name {
			case "ls":
				if result.List == nil || result.List.OriginalCount != 1 || len(result.List.Entries) != 1 || result.List.Entries[0].Name != test.file || result.List.Names[0] != result.List.Entries[0].DisplayName || result.List.Limit != 200 {
					t.Fatal("actual directory result changed")
				}
			case "find":
				if result.Find == nil || result.Find.OriginalCount != 1 || len(result.Find.Matches) != 1 || result.Find.Matches[0].RelativePath != test.file || result.Find.Limit != 200 {
					t.Fatal("actual Find result changed")
				}
			case "code_index":
				if result.CodeIndex == nil || result.CodeIndex.SymbolCount != 1 || result.CodeIndex.OriginalCount != 1 || len(result.CodeIndex.Symbols) != 1 || result.CodeIndex.Symbols[0].Name != "NeedleAlpha" || result.CodeIndex.Symbols[0].Line != 2 || !result.CodeIndex.ProducerTruncated || !result.CodeIndex.LimitReached || result.CodeIndex.Text == nil {
					t.Fatal("actual filtered/limited symbol result changed")
				}
			case "move_file":
				if result.Move == nil || !result.Move.Moved || result.Move.BytesMoved != "9" || result.Move.SourceRelativePath != test.file || result.Move.DestinationRelativePath != "moved.txt" {
					t.Fatal("actual move receipt changed")
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("move receipt did not correspond to actual rename")
				}
			case "notebook_edit":
				if result.NotebookEdit == nil || result.NotebookEdit.CellNumber != 0 || result.NotebookEdit.CellCount != 1 || result.NotebookEdit.CellID == nil || *result.NotebookEdit.CellID != "intro" || result.NotebookEdit.Diff.Patch == nil {
					t.Fatal("actual notebook receipt changed")
				}
			case "delete_range":
				if result.DeleteRange == nil || result.DeleteRange.StartLine != 2 || result.DeleteRange.EndLine != 4 || result.DeleteRange.FirstDeletedLine != 3 || result.DeleteRange.DeletedLines != 1 || result.DeleteRange.Inclusive || result.DeleteRange.Diff.Patch == nil {
					t.Fatal("actual exclusive range coordinates changed")
				}
			case "delete_symbol":
				if result.DeleteSymbol == nil || result.DeleteSymbol.Name != "Gone" || result.DeleteSymbol.Line != 3 || result.DeleteSymbol.DeletedLines < 1 || result.DeleteSymbol.Diff.Patch == nil {
					t.Fatal("actual Go symbol receipt changed")
				}
			}
			if test.name == "notebook_edit" || test.name == "delete_range" || test.name == "delete_symbol" {
				bytes, err := os.ReadFile(path)
				if err != nil || string(bytes) == test.content {
					t.Fatal("receipt did not correspond to actual mutation", err)
				}
			}
			thread, err := h.store.GetThread(pending.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			item := protectedSeamResultV1(t, thread, pending)
			binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(item[domaintoolresult.ProtectedSnapshotBindingFieldV1])
			if err != nil || binding.SnapshotDigest != display.SnapshotDigest || item["isError"] != false {
				t.Fatal("actual primary result lost exact private binding")
			}
			assertPublicToolSeamV1(t, h, pending, "fileResult")
			assertPublicToolSeamV1(t, h, pending, "PRIVATE_FILE_RESULT")
			// Later directory/file state cannot replace the original sealed result.
			if err := os.WriteFile(filepath.Join(pending.Workspace, "later.txt"), []byte("later state"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := h.toolSnapshots.(interface{ Close() error }).Close(); err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := snapshotstore.NewStore(context.Background(), root, access)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			h.toolSnapshots = reopened
			h.store, err = NewTempDurableEventSessionStore(durable)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := finalauthority.NewAcceptedFinalCASReader(h.store.root)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.store.BindPrimaryThreadReaderV1(reader); err != nil {
				t.Fatal(err)
			}
			status, after, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || after.SnapshotDigest != display.SnapshotDigest || !reflect.DeepEqual(after.Capture, display.Capture) {
				t.Fatal("restart/current files replaced the immutable file result")
			}
			foreign := pending
			foreign.Call.ID = serverTestHostToolCallID("foreign_file_result")
			if status, _, _ := readProtectedSeamV1(t, h, foreign); status == http.StatusOK {
				t.Fatal("foreign call read another result")
			}
			identity := h.turnSecurity.Identity
			h.turnSecurity.Identity = nil
			status, _, _ = readProtectedSeamV1(t, h, pending)
			h.turnSecurity.Identity = identity
			if status == http.StatusOK {
				t.Fatal("result outlived current Host authority")
			}
		})
	}
}

func TestFileResultActualProducerLaterRejectedSettlementKeepsInspectOnlyV1(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "ls", map[string]any{"path": "."})
	if err := os.WriteFile(filepath.Join(pending.Workspace, "PRIVATE_LATE_REJECT.txt"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, func(_ any, failed bool) (any, bool, error) {
		if failed {
			t.Fatal("real ls unexpectedly failed before rejection")
		}
		return map[string]any{"code": "validation_error", "error": "Synthetic post-effect settlement rejection"}, true, nil
	})
	if err != nil || !settled.IsError {
		t.Fatal("actual post-effect rejection was lost", err)
	}
	status, display, response := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Kind != "inspect" || display.Capture.FileResult != nil || display.Capture.Status != "failed" || display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) || strings.Contains(response, "PRIVATE_LATE_REJECT") {
		t.Fatal("rejected settlement retained successful file results or lost request facts")
	}
}

func TestFileResultListAndFindScanFailuresSettleAsFailedInspectV1(t *testing.T) {
	for _, test := range []struct {
		name, tool, path, code string
		nonDirectory           bool
	}{
		{"ls-missing-root", "ls", "PRIVATE_LIST_MISSING_ROOT", "list_failed", false},
		{"find-missing-root", "find", "PRIVATE_FIND_MISSING_ROOT", "find_failed", false},
		{"find-non-directory-intermediate", "find", "PRIVATE_FIND_NON_DIRECTORY/child", "find_failed", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := map[string]any{"path": test.path}
			if test.tool == "find" {
				args["pattern"] = "*"
			}
			h, pending, _, _ := protectedSeamFixtureV1(t, test.tool, args)
			if test.nonDirectory {
				if err := os.WriteFile(filepath.Join(pending.Workspace, "PRIVATE_FIND_NON_DIRECTORY"), []byte("ordinary synthetic file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			observed := false
			var errorMessage string
			settled, err := h.executeAndSettleRuntimeTool(context.Background(), pending, nil, func(output any, failed bool) (any, bool, error) {
				// Observe the real producer's error without replacing any output.
				observed = true
				model, ok := output.(map[string]any)
				if !failed || !ok || model["code"] != test.code {
					t.Fatal("actual file scan failure became an empty successful result")
				}
				for _, key := range []string{"matches", "entries", "names"} {
					if _, success := model[key]; success {
						t.Fatal("failed file scan retained a successful payload")
					}
				}
				errorMessage, _ = model["error"].(string)
				if errorMessage == "" {
					t.Fatal("actual file scan failure lost its original filesystem error")
				}
				return output, failed, nil
			})
			if err != nil || !observed || !settled.IsError {
				t.Fatal("actual file scan failure did not settle as an error", err)
			}
			status, display, _ := readProtectedSeamV1(t, h, pending)
			if status != http.StatusOK || display.ToolName != test.tool || display.Capture.Kind != "inspect" || display.Capture.Status != "failed" || display.Capture.FileResult != nil || display.ReadTarget != nil || display.Inspect == nil || display.Inspect.InputJSON != string(pending.Call.Arguments) || display.Inspect.SchemaJSON != pending.ModelVisibleToolDefinition.SchemaJSON || display.Inspect.OutputError == nil {
				t.Fatal("failed file scan sealed a completed file result or lost its original request/error status")
			}
			outputError := display.Inspect.OutputError
			if outputError.Availability != "available" || len(outputError.Entries) != 1 || outputError.Entries[0].Message != errorMessage || outputError.Entries[0].Classification != "source_exact_local" {
				t.Fatal("file scan error was replaced instead of sealed by its existing protected observer")
			}
			thread, err := h.store.GetThread(pending.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			item := protectedSeamResultV1(t, thread, pending)
			binding, err := domaintoolresult.ParseProtectedSnapshotBindingV1(item[domaintoolresult.ProtectedSnapshotBindingFieldV1])
			if err != nil || binding.SnapshotDigest != display.SnapshotDigest || stringField(item, "status") != "failed" || item["isError"] != true {
				t.Fatal("failed file scan lost its exact failed primary result")
			}
			assertPublicToolSeamV1(t, h, pending, "fileResult")
			assertPublicToolSeamV1(t, h, pending, errorMessage)
		})
	}
}

func TestFileResultActualProducerSealProjectsHostKnownCredentialV1(t *testing.T) {
	const secret = "synthetic-host-credential-491687"
	h, pending, _, _ := protectedSeamFixtureV1(t, "ls", map[string]any{"path": "."})
	if err := os.WriteFile(filepath.Join(pending.Workspace, secret+".txt"), []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := domainprocess.WithKnownSecretsV1(context.Background(), secret)
	if settled, err := h.executeAndSettleRuntimeTool(ctx, pending, nil, nil); err != nil || settled.IsError {
		t.Fatal("actual credential fixture failed", err)
	}
	status, display, response := readProtectedSeamV1(t, h, pending)
	if status != http.StatusOK || display.Capture.Kind != "file_result" || display.Capture.FileResult == nil || display.Capture.FileResult.Classification != "credential_redacted" || strings.Contains(response, secret) {
		t.Fatal("host credential survived file result sealing")
	}
	list := display.Capture.FileResult.List
	if list == nil || list.OriginalCount != 1 || len(list.Entries) != 1 || list.Names[0] != list.Entries[0].DisplayName {
		t.Fatal("credential projection destroyed directory identity/count facts")
	}
	assertPublicToolSeamV1(t, h, pending, secret)
}
