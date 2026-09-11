package filestore

import (
	"context"
	"errors"
	"strings"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

type MutationCheckpointDraft = checkpointapp.SnapshotDraft

type MutationCheckpointHooks struct {
	BeginOperation  func([]MutationOperationPath) (MutationOperationDraft, error)
	SettleOperation func(MutationOperationDraft, bool) (string, error)
	Prepare         func(resolvedPath string) (MutationCheckpointDraft, error)
	Complete        func(draft MutationCheckpointDraft, resolvedPath string) error
	Abort           func(draft MutationCheckpointDraft) error
}

type MutationOperationPath = checkpointapp.OperationPathRequest

type MutationOperationDraft = checkpointapp.OperationDraft

type MutationToolInput struct {
	Context           context.Context
	Workspace         string
	Args              map[string]any
	ArgumentsJSON     []byte
	ToolName          string
	SandboxMode       string
	AllowWriteRoots   []string
	ProtectedReadDirs []string
	MutationAuthority ConditionalMutationAuthority
	Checkpoint        MutationCheckpointHooks
	AcquireMutation   func(context.Context) (func(), error)
}

type PreparedNotebookEditTool struct {
	plan          ExistingTextMutationPlan
	request       filetoolsapp.NotebookEditRequest
	change        filetoolsapp.NotebookEditPreview
	argumentKey   string
	requestedPath string
	ownerVersion  int
}

func PreparedNotebookEditSemanticEffectProjection(prepared PreparedNotebookEditTool) (any, error) {
	if prepared.ownerVersion != 1 || strings.TrimSpace(prepared.plan.Path) == "" {
		return nil, errors.New("notebook_edit prepared owner state is invalid")
	}
	projection := map[string]any{
		"path": prepared.plan.Path, "editMode": prepared.request.EditMode,
		"cellIndex": prepared.change.CellIndex,
	}
	if prepared.request.EditMode != "delete" {
		projection["newSource"] = prepared.request.NewSource
		projection["cellType"] = prepared.change.CellType
	}
	return projection, nil
}

type PreparedDeleteSymbolTool struct {
	plan          ExistingTextMutationPlan
	change        filetoolsapp.DeleteSymbolPreview
	argumentKey   string
	requestedPath string
	ownerVersion  int
}

func PreparedDeleteSymbolSemanticEffectProjection(prepared PreparedDeleteSymbolTool) (any, error) {
	if prepared.ownerVersion != 1 || strings.TrimSpace(prepared.plan.Path) == "" {
		return nil, errors.New("delete_symbol prepared owner state is invalid")
	}
	return map[string]any{
		"path": prepared.plan.Path, "name": prepared.change.Match.Name,
		"kind": prepared.change.Match.Kind, "parent": prepared.change.Match.Parent,
	}, nil
}

func ExecuteWriteTextTool(input MutationToolInput) (any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	request, err := filetoolsapp.ParseWriteToolRequest(input.Args)
	if err != nil {
		return filetoolsapp.ToolErrorOutput(err), true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, request.Path); blocked {
		return output, true
	}
	plan, err := PrepareTextWriteForSandbox(input.Workspace, request.Path, input.SandboxMode, input.AllowWriteRoots)
	if err != nil {
		return MutationOperationErrorOutput(err), true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, plan.Path); blocked {
		return output, true
	}
	var checkpointDraft MutationCheckpointDraft
	var operationDraft MutationOperationDraft
	if input.Checkpoint.BeginOperation != nil {
		argumentKey, requestedPath := selectedStringArgument(input.Args, "path", "filePath", "FilePath")
		operationDraft, err = input.beginOperation([]MutationOperationPath{{
			ResolvedPath: plan.Path, ArgumentKey: argumentKey, RequestedPath: requestedPath, Role: "target",
			ExpectedAfterExisted: true,
			ExpectedAfterHash:    checkpointapp.HashBytes(filetoolsapp.EncodeTextBytes(request.Content, plan.Encoding)),
		}})
	} else {
		checkpointDraft, err = input.prepareCheckpoint(plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	encoded, err := ApplyTextWrite(plan, request.Content)
	if err != nil {
		if operationDraft.Enabled {
			if input.settleOperation(operationDraft, false) != nil {
				return checkpointUnavailableOutput(), true
			}
		} else if input.abortCheckpoint(checkpointDraft) != nil {
			return checkpointUnavailableOutput(), true
		}
		return MutationOperationErrorOutput(err), true
	}
	if operationDraft.Enabled {
		err = input.settleOperation(operationDraft, true)
	} else {
		err = input.completeCheckpoint(checkpointDraft, plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	return filetoolsapp.BuildWriteToolOutput(filetoolsapp.WriteToolOutputInput{
		Path:         plan.Path,
		RelativePath: plan.RelativePath,
		Content:      request.Content,
		BytesWritten: len(encoded),
		Encoding:     plan.Encoding,
		Before:       plan.Before,
		IncludeDiff:  plan.IncludeDiff,
		DiffKind:     plan.DiffKind,
	}), false
}

func ExecuteEditTextTool(input MutationToolInput) (any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	path := strings.TrimSpace(firstNonEmptyAnyString(input.Args["path"], input.Args["filePath"], input.Args["FilePath"]))
	if path == "" {
		return map[string]any{"code": "validation_error", "error": "path is required"}, true
	}
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, path); blocked {
		return output, true
	}
	edits, err := filetoolsapp.ParseEditInstructions(input.Args)
	if err != nil {
		return filetoolsapp.ToolErrorOutput(err), true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	plan, err := PrepareExistingTextMutation(ExistingTextMutationOptions{
		Workspace:       input.Workspace,
		Path:            path,
		SandboxMode:     input.SandboxMode,
		AllowWriteRoots: input.AllowWriteRoots,
		BinaryError:     "edit only supports text files",
	})
	if err != nil {
		return MutationOperationErrorOutput(err), true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, plan.Path); blocked {
		return output, true
	}
	after, replacements, err := filetoolsapp.ApplyExactTextEdits(plan.Before, edits)
	if err != nil {
		return filetoolsapp.ToolErrorOutput(err), true
	}
	var checkpointDraft MutationCheckpointDraft
	var operationDraft MutationOperationDraft
	if input.Checkpoint.BeginOperation != nil {
		argumentKey, requestedPath := selectedStringArgument(input.Args, "path", "filePath", "FilePath")
		operationDraft, err = input.beginOperation([]MutationOperationPath{{
			ResolvedPath: plan.Path, ArgumentKey: argumentKey, RequestedPath: requestedPath, Role: "target",
			ExpectedAfterExisted: true,
			ExpectedAfterHash:    checkpointapp.HashBytes(filetoolsapp.EncodeTextBytes(after, plan.Encoding)),
		}})
	} else {
		checkpointDraft, err = input.prepareCheckpoint(plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	encoded, err := ApplyExistingTextMutation(plan, after)
	if err != nil {
		if operationDraft.Enabled {
			if input.settleOperation(operationDraft, false) != nil {
				return checkpointUnavailableOutput(), true
			}
		} else if input.abortCheckpoint(checkpointDraft) != nil {
			return checkpointUnavailableOutput(), true
		}
		return MutationOperationErrorOutput(err), true
	}
	if operationDraft.Enabled {
		err = input.settleOperation(operationDraft, true)
	} else {
		err = input.completeCheckpoint(checkpointDraft, plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	return filetoolsapp.BuildEditToolOutput(filetoolsapp.EditToolOutputInput{
		Path:         plan.Path,
		RelativePath: plan.RelativePath,
		Replacements: replacements,
		BytesWritten: len(encoded),
		Encoding:     plan.Encoding,
		Before:       plan.Before,
		After:        after,
	}), false
}

func ExecuteMoveFileTool(input MutationToolInput) (any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	sourcePath := strings.TrimSpace(firstNonEmptyAnyString(input.Args["source_path"], input.Args["sourcePath"], input.Args["source"], input.Args["from"], input.Args["path"]))
	destinationPath := strings.TrimSpace(firstNonEmptyAnyString(input.Args["destination_path"], input.Args["destinationPath"], input.Args["destination"], input.Args["to"]))
	if sourcePath == "" {
		return map[string]any{"code": "validation_error", "error": "source_path is required"}, true
	}
	if destinationPath == "" {
		return map[string]any{"code": "validation_error", "error": "destination_path is required"}, true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, sourcePath, destinationPath); blocked {
		return output, true
	}
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace:         input.Workspace,
		SourcePath:        sourcePath,
		DestinationPath:   destinationPath,
		SandboxMode:       input.SandboxMode,
		AllowWriteRoots:   input.AllowWriteRoots,
		MutationAuthority: input.MutationAuthority,
	})
	if err != nil {
		return MutationOperationErrorOutput(err), true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, plan.SourcePath, plan.DestinationPath); blocked {
		return output, true
	}
	if plan.Noop {
		return filetoolsapp.BuildMoveToolOutput(filetoolsapp.MoveToolOutputInput{
			SourcePath:              plan.SourcePath,
			DestinationPath:         plan.DestinationPath,
			SourceRelativePath:      plan.SourceRelativePath,
			DestinationRelativePath: plan.DestinationRelativePath,
			BytesMoved:              plan.BytesMoved,
			Moved:                   false,
		}), false
	}
	var operationDraft MutationOperationDraft
	if input.Checkpoint.BeginOperation == nil {
		return checkpointUnavailableOutput(), true
	}
	sourceKey, requestedSource := selectedStringArgument(input.Args, "source_path", "sourcePath", "source", "from", "path")
	destinationKey, requestedDestination := selectedStringArgument(input.Args, "destination_path", "destinationPath", "destination", "to")
	operationDraft, err = input.beginOperation([]MutationOperationPath{
		{ResolvedPath: plan.SourcePath, ArgumentKey: sourceKey, RequestedPath: requestedSource, Role: "source", ExpectedAfterExisted: false},
		{ResolvedPath: plan.DestinationPath, ArgumentKey: destinationKey, RequestedPath: requestedDestination, Role: "destination", ExpectedAfterExisted: true},
	})
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	plan, err = BindMoveRegularFilePlanToIntent(plan, operationDraft.AuthorityIntent)
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	if err := ApplyMoveRegularFile(plan); err != nil {
		if input.settleOperation(operationDraft, false) != nil {
			return checkpointUnavailableOutput(), true
		}
		return MutationOperationErrorOutput(err), true
	}
	if err := input.settleOperation(operationDraft, true); err != nil {
		return checkpointUnavailableOutput(), true
	}
	return filetoolsapp.BuildMoveToolOutput(filetoolsapp.MoveToolOutputInput{
		SourcePath:              plan.SourcePath,
		DestinationPath:         plan.DestinationPath,
		SourceRelativePath:      plan.SourceRelativePath,
		DestinationRelativePath: plan.DestinationRelativePath,
		BytesMoved:              plan.BytesMoved,
		Moved:                   true,
	}), false
}

func ExecuteNotebookEditTool(input MutationToolInput) (any, bool) {
	prepared, output, failed := PrepareNotebookEditTool(input)
	if failed {
		return output, true
	}
	return ExecutePreparedNotebookEditTool(input, prepared)
}

func PrepareNotebookEditTool(input MutationToolInput) (PreparedNotebookEditTool, map[string]any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return PreparedNotebookEditTool{}, output, true
	}
	request, err := filetoolsapp.ParseNotebookEditRequest(input.Args)
	if err != nil {
		return PreparedNotebookEditTool{}, filetoolsapp.ToolErrorOutput(err), true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return PreparedNotebookEditTool{}, mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, request.Path); blocked {
		return PreparedNotebookEditTool{}, output, true
	}
	plan, err := PrepareExistingTextMutation(ExistingTextMutationOptions{
		Workspace:                 input.Workspace,
		Path:                      request.Path,
		SandboxMode:               input.SandboxMode,
		AllowWriteRoots:           input.AllowWriteRoots,
		RequiredExtension:         ".ipynb",
		UnsupportedExtensionError: "notebook_edit only supports .ipynb files",
		BinaryError:               "notebook_edit only supports text JSON notebooks",
	})
	if err != nil {
		return PreparedNotebookEditTool{}, MutationOperationErrorOutput(err), true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, plan.Path); blocked {
		return PreparedNotebookEditTool{}, output, true
	}
	change, err := filetoolsapp.PreviewNotebookEdit(plan.Before, request)
	if err != nil {
		return PreparedNotebookEditTool{}, filetoolsapp.ToolErrorOutput(err), true
	}
	argumentKey, requestedPath := selectedStringArgument(input.Args, "path", "filePath", "FilePath")
	return PreparedNotebookEditTool{
		plan: plan, request: request, change: change, argumentKey: argumentKey, requestedPath: requestedPath, ownerVersion: 1,
	}, nil, false
}

func ExecutePreparedNotebookEditTool(input MutationToolInput, prepared PreparedNotebookEditTool) (any, bool) {
	if prepared.ownerVersion != 1 {
		return map[string]any{"code": "prepared_mutation_invalid", "error": "notebook_edit prepared owner state is invalid"}, true
	}
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedMutationOutput(input, prepared.plan.Path); blocked {
		return output, true
	}
	var checkpointDraft MutationCheckpointDraft
	var operationDraft MutationOperationDraft
	if input.Checkpoint.BeginOperation != nil {
		operationDraft, err = input.beginOperation([]MutationOperationPath{{
			ResolvedPath: prepared.plan.Path, ArgumentKey: prepared.argumentKey, RequestedPath: prepared.requestedPath, Role: "target",
			ExpectedAfterExisted: true,
			ExpectedAfterHash:    checkpointapp.HashBytes(filetoolsapp.EncodeTextBytes(prepared.change.After, prepared.plan.Encoding)),
		}})
	} else {
		checkpointDraft, err = input.prepareCheckpoint(prepared.plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	encoded, err := ApplyExistingTextMutation(prepared.plan, prepared.change.After)
	if err != nil {
		if operationDraft.Enabled {
			if input.settleOperation(operationDraft, false) != nil {
				return checkpointUnavailableOutput(), true
			}
		} else if input.abortCheckpoint(checkpointDraft) != nil {
			return checkpointUnavailableOutput(), true
		}
		return MutationOperationErrorOutput(err), true
	}
	if operationDraft.Enabled {
		err = input.settleOperation(operationDraft, true)
	} else {
		err = input.completeCheckpoint(checkpointDraft, prepared.plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	return filetoolsapp.BuildNotebookEditToolOutput(filetoolsapp.NotebookEditToolOutputInput{
		Path:         prepared.plan.Path,
		RelativePath: prepared.plan.RelativePath,
		Request:      prepared.request,
		Change:       prepared.change,
		BytesWritten: len(encoded),
		Before:       prepared.plan.Before,
	}), false
}

func ExecuteDeleteRangeTool(input MutationToolInput) (any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	path := strings.TrimSpace(firstNonEmptyAnyString(input.Args["path"], input.Args["filePath"], input.Args["FilePath"]))
	if path == "" {
		return map[string]any{"code": "validation_error", "error": "path is required"}, true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, path); blocked {
		return output, true
	}
	plan, err := PrepareExistingTextMutation(ExistingTextMutationOptions{
		Workspace:       input.Workspace,
		Path:            path,
		SandboxMode:     input.SandboxMode,
		AllowWriteRoots: input.AllowWriteRoots,
		BinaryError:     "delete_range only supports text files",
	})
	if err != nil {
		return MutationOperationErrorOutput(err), true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, plan.Path); blocked {
		return output, true
	}
	change, err := filetoolsapp.PreviewDeleteRange(plan.Before, input.Args)
	if err != nil {
		return filetoolsapp.ToolErrorOutput(err), true
	}
	var checkpointDraft MutationCheckpointDraft
	var operationDraft MutationOperationDraft
	if input.Checkpoint.BeginOperation != nil {
		argumentKey, requestedPath := selectedStringArgument(input.Args, "path", "filePath", "FilePath")
		operationDraft, err = input.beginOperation([]MutationOperationPath{{
			ResolvedPath: plan.Path, ArgumentKey: argumentKey, RequestedPath: requestedPath, Role: "target",
			ExpectedAfterExisted: true,
			ExpectedAfterHash:    checkpointapp.HashBytes(filetoolsapp.EncodeTextBytes(change.After, plan.Encoding)),
		}})
	} else {
		checkpointDraft, err = input.prepareCheckpoint(plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	encoded, err := ApplyExistingTextMutation(plan, change.After)
	if err != nil {
		if operationDraft.Enabled {
			if input.settleOperation(operationDraft, false) != nil {
				return checkpointUnavailableOutput(), true
			}
		} else if input.abortCheckpoint(checkpointDraft) != nil {
			return checkpointUnavailableOutput(), true
		}
		return MutationOperationErrorOutput(err), true
	}
	if operationDraft.Enabled {
		err = input.settleOperation(operationDraft, true)
	} else {
		err = input.completeCheckpoint(checkpointDraft, plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	return filetoolsapp.BuildDeleteRangeToolOutput(filetoolsapp.DeleteRangeToolOutputInput{
		Path:         plan.Path,
		RelativePath: plan.RelativePath,
		Change:       change,
		BytesWritten: len(encoded),
		Encoding:     plan.Encoding,
		Before:       plan.Before,
	}), false
}

func ExecuteDeleteSymbolTool(input MutationToolInput) (any, bool) {
	prepared, output, failed := PrepareDeleteSymbolTool(input)
	if failed {
		return output, true
	}
	return ExecutePreparedDeleteSymbolTool(input, prepared)
}

func PrepareDeleteSymbolTool(input MutationToolInput) (PreparedDeleteSymbolTool, map[string]any, bool) {
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return PreparedDeleteSymbolTool{}, output, true
	}
	path := strings.TrimSpace(firstNonEmptyAnyString(input.Args["path"], input.Args["filePath"], input.Args["FilePath"]))
	if path == "" {
		return PreparedDeleteSymbolTool{}, map[string]any{"code": "validation_error", "error": "path is required"}, true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return PreparedDeleteSymbolTool{}, mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedRequestedMutationOutput(input, path); blocked {
		return PreparedDeleteSymbolTool{}, output, true
	}
	plan, err := PrepareExistingTextMutation(ExistingTextMutationOptions{
		Workspace:                 input.Workspace,
		Path:                      path,
		SandboxMode:               input.SandboxMode,
		AllowWriteRoots:           input.AllowWriteRoots,
		RequiredExtension:         ".go",
		UnsupportedExtensionError: "delete_symbol only supports Go files; use delete_range for non-Go files",
		BinaryError:               "delete_symbol only supports text Go files",
	})
	if err != nil {
		return PreparedDeleteSymbolTool{}, MutationOperationErrorOutput(err), true
	}
	if output, blocked := mandatoryProtectedMutationOutput(input, plan.Path); blocked {
		return PreparedDeleteSymbolTool{}, output, true
	}
	change, err := filetoolsapp.PreviewDeleteSymbol(plan.Path, plan.Before, input.Args)
	if err != nil {
		return PreparedDeleteSymbolTool{}, filetoolsapp.ToolErrorOutput(err), true
	}
	argumentKey, requestedPath := selectedStringArgument(input.Args, "path", "filePath", "FilePath")
	return PreparedDeleteSymbolTool{
		plan: plan, change: change, argumentKey: argumentKey, requestedPath: requestedPath, ownerVersion: 1,
	}, nil, false
}

func ExecutePreparedDeleteSymbolTool(input MutationToolInput, prepared PreparedDeleteSymbolTool) (any, bool) {
	if prepared.ownerVersion != 1 {
		return map[string]any{"code": "prepared_mutation_invalid", "error": "delete_symbol prepared owner state is invalid"}, true
	}
	if output, blocked := sandboxBlockedMutationOutput(input); blocked {
		return output, true
	}
	release, err := input.acquireMutation()
	if err != nil {
		return mutationCoordinatorUnavailableOutput(), true
	}
	defer release()
	if output, blocked := mandatoryProtectedMutationOutput(input, prepared.plan.Path); blocked {
		return output, true
	}
	var checkpointDraft MutationCheckpointDraft
	var operationDraft MutationOperationDraft
	if input.Checkpoint.BeginOperation != nil {
		operationDraft, err = input.beginOperation([]MutationOperationPath{{
			ResolvedPath: prepared.plan.Path, ArgumentKey: prepared.argumentKey, RequestedPath: prepared.requestedPath, Role: "target",
			ExpectedAfterExisted: true,
			ExpectedAfterHash:    checkpointapp.HashBytes(filetoolsapp.EncodeTextBytes(prepared.change.After, prepared.plan.Encoding)),
		}})
	} else {
		checkpointDraft, err = input.prepareCheckpoint(prepared.plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	encoded, err := ApplyExistingTextMutation(prepared.plan, prepared.change.After)
	if err != nil {
		if operationDraft.Enabled {
			if input.settleOperation(operationDraft, false) != nil {
				return checkpointUnavailableOutput(), true
			}
		} else if input.abortCheckpoint(checkpointDraft) != nil {
			return checkpointUnavailableOutput(), true
		}
		return MutationOperationErrorOutput(err), true
	}
	if operationDraft.Enabled {
		err = input.settleOperation(operationDraft, true)
	} else {
		err = input.completeCheckpoint(checkpointDraft, prepared.plan.Path)
	}
	if err != nil {
		return checkpointUnavailableOutput(), true
	}
	return filetoolsapp.BuildDeleteSymbolToolOutput(filetoolsapp.DeleteSymbolToolOutputInput{
		Path:         prepared.plan.Path,
		RelativePath: prepared.plan.RelativePath,
		Change:       prepared.change,
		BytesWritten: len(encoded),
		Encoding:     prepared.plan.Encoding,
		Before:       prepared.plan.Before,
	}), false
}

func MutationOperationErrorOutput(err error) map[string]any {
	var operationErr OperationError
	if errors.As(err, &operationErr) {
		output := map[string]any{"code": operationErr.Code, "error": operationErr.Message}
		if operationErr.Path != "" {
			output["path"] = operationErr.Path
		}
		if operationErr.RelativePath != "" {
			output["relative_path"] = operationErr.RelativePath
		}
		return output
	}
	return filetoolsapp.ToolErrorOutput(err)
}

func sandboxBlockedMutationOutput(input MutationToolInput) (map[string]any, bool) {
	if input.SandboxMode != "read-only" && input.SandboxMode != "external-sandbox" {
		return nil, false
	}
	toolName := strings.TrimSpace(input.ToolName)
	if toolName == "" {
		toolName = "file mutation"
	}
	return map[string]any{"code": "sandbox_blocked", "error": toolName + " is blocked by sandbox"}, true
}

func mandatoryProtectedMutationOutput(input MutationToolInput, paths ...string) (map[string]any, bool) {
	for _, path := range paths {
		if output := MandatoryProtectedPathOutput(input.Workspace, path, input.ProtectedReadDirs); output != nil {
			return output, true
		}
	}
	return nil, false
}

func mandatoryProtectedRequestedMutationOutput(input MutationToolInput, paths ...string) (map[string]any, bool) {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		value, ok := ResolveAnyPath(input.Workspace, path)
		if !ok {
			continue
		}
		resolved = append(resolved, value)
	}
	return mandatoryProtectedMutationOutput(input, resolved...)
}

func (input MutationToolInput) prepareCheckpoint(resolvedPath string) (MutationCheckpointDraft, error) {
	if input.Checkpoint.Prepare == nil {
		return MutationCheckpointDraft{}, nil
	}
	return input.Checkpoint.Prepare(resolvedPath)
}

func (input MutationToolInput) completeCheckpoint(draft MutationCheckpointDraft, resolvedPath string) error {
	if input.Checkpoint.Complete != nil {
		return input.Checkpoint.Complete(draft, resolvedPath)
	}
	return nil
}

func (input MutationToolInput) abortCheckpoint(draft MutationCheckpointDraft) error {
	if !draft.Enabled || input.Checkpoint.Abort == nil {
		return nil
	}
	return input.Checkpoint.Abort(draft)
}

func (input MutationToolInput) beginOperation(paths []MutationOperationPath) (MutationOperationDraft, error) {
	if input.Checkpoint.BeginOperation == nil {
		return MutationOperationDraft{}, nil
	}
	return input.Checkpoint.BeginOperation(paths)
}

func (input MutationToolInput) settleOperation(draft MutationOperationDraft, mutationSucceeded bool) error {
	if !draft.Enabled || input.Checkpoint.SettleOperation == nil {
		return nil
	}
	status, err := input.Checkpoint.SettleOperation(draft, mutationSucceeded)
	if err != nil {
		return err
	}
	if mutationSucceeded && status != "completed" {
		return errors.New("checkpoint operation did not settle as completed")
	}
	if !mutationSucceeded && status == "quarantined" {
		return errors.New("checkpoint operation entered quarantine")
	}
	return nil
}

func selectedStringArgument(args map[string]any, keys ...string) (string, string) {
	for _, key := range keys {
		value, ok := args[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		return key, strings.TrimSpace(value)
	}
	return "", ""
}

func (input MutationToolInput) acquireMutation() (func(), error) {
	if input.AcquireMutation == nil {
		return func() {}, nil
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	release, err := input.AcquireMutation(ctx)
	if err != nil || release == nil {
		return nil, errors.Join(errors.New("workspace mutation lease is unavailable"), err)
	}
	return release, nil
}

func mutationCoordinatorUnavailableOutput() map[string]any {
	return map[string]any{
		"code":  "mutation_coordinator_unavailable",
		"error": "workspace mutation coordinator is unavailable; the file mutation was not started",
	}
}

func checkpointUnavailableOutput() map[string]any {
	return map[string]any{
		"code":  "checkpoint_unavailable",
		"error": "checkpoint authority is unavailable; the file mutation was not accepted",
	}
}
