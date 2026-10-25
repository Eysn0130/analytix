package server

import (
	"context"
	"encoding/json"
	"reflect"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func executePresentRuntimeToolV1(ctx context.Context, pending runtimePendingToolCall, args map[string]any, protectedReadDirs, readRoots []string) (any, bool) {
	failure := func(output map[string]any) (any, bool) {
		if message, ok := output["error"].(string); ok {
			domaintoolresult.CaptureProtectedToolErrorV1(ctx, domaintoolresult.ProjectProtectedOutputErrorV1([]domaintoolresult.ProtectedOutputErrorEntryV1{{Message: message}}))
		}
		return output, true
	}
	invalid := func() (any, bool) {
		return failure(map[string]any{"code": "validation_error", "error": "present requires 1 to 8 files with a path and optional description"})
	}
	if len(args) != 1 || pending.TurnID == "" {
		return invalid()
	}
	files, ok := args["files"].([]any)
	if !ok || len(files) < 1 || len(files) > domaintoolresult.ProtectedPresentMaxFilesV1 {
		return invalid()
	}
	receipt := domaintoolresult.ProtectedPresentReceiptV1{Files: make([]domaintoolresult.ProtectedPresentedFileV1, 0, len(files))}
	resultFiles := make([]any, 0, len(files))
	for index, raw := range files {
		file, ok := raw.(map[string]any)
		if !ok || len(file) < 1 || len(file) > 2 {
			return invalid()
		}
		for key := range file {
			if key != "path" && key != "description" {
				return invalid()
			}
		}
		path, ok := file["path"].(string)
		if !ok {
			return invalid()
		}
		declaration := domaintoolresult.ProtectedPresentedFileV1{Index: index, Path: path}
		result := map[string]any{"path": path}
		if raw, present := file["description"]; present {
			description, ok := raw.(string)
			if !ok {
				return invalid()
			}
			declaration.Description = &description
			result["description"] = description
		}
		receipt.Files = append(receipt.Files, declaration)
		resultFiles = append(resultFiles, result)
	}
	targets, blocked := filestore.ValidatePresentFilesV1(filestore.PresentFilesInputV1{Context: ctx, Workspace: pending.Workspace,
		SandboxMode: pending.SandboxMode, ReadRoots: readRoots, ProtectedReadDirs: protectedReadDirs, Receipt: receipt})
	if blocked != nil {
		return failure(blocked)
	}
	domaintoolresult.CaptureProtectedToolResultV1(ctx, domaintoolresult.ProtectedCaptureV1{Kind: "present", Status: "completed", Present: &receipt, PresentTargets: targets})
	// The native turn identity is a string; DSH's numeric turn counter is not
	// an Analytix identity. Raw declarations remain in this effect attempt only.
	return map[string]any{"turn": pending.TurnID, "files": resultFiles}, false
}

// Bind the selected declaration bytes to this admitted call; neither a result
// map nor a later live file listing can replace the original receipt.
func protectedPresentMatchesArgumentsV1(receipt *domaintoolresult.ProtectedPresentReceiptV1, arguments json.RawMessage) bool {
	args, err := domainsecurity.DecodeCanonicalJSONObject(arguments)
	if err != nil || len(args) != 1 || domaintoolresult.ValidateProtectedPresentReceiptV1(receipt) != nil {
		return false
	}
	files, ok := args["files"].([]any)
	if !ok || len(files) != len(receipt.Files) {
		return false
	}
	for index, declaration := range receipt.Files {
		want := map[string]any{"path": declaration.Path}
		if declaration.Description != nil {
			want["description"] = *declaration.Description
		}
		if !reflect.DeepEqual(files[index], want) {
			return false
		}
	}
	return true
}
