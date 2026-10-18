package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type PresentFilesInputV1 struct {
	Context           context.Context
	Workspace         string
	SandboxMode       string
	ReadRoots         []string
	ProtectedReadDirs []string
	Receipt           domaintoolresult.ProtectedPresentReceiptV1
}

// ValidatePresentFilesV1 observes metadata only. It neither reads/copies file
// contents nor creates a second artifact registry or filesystem checkpoint.
func ValidatePresentFilesV1(input PresentFilesInputV1) ([]domaintoolresult.ProtectedPresentTargetV1, map[string]any) {
	failure := func(code, message string) ([]domaintoolresult.ProtectedPresentTargetV1, map[string]any) {
		return nil, map[string]any{"code": code, "error": message}
	}
	if input.Context == nil || input.Context.Err() != nil {
		return failure("tool_cancelled", "present was canceled")
	}
	if strings.TrimSpace(input.Workspace) == "" || domaintoolresult.ValidateProtectedPresentReceiptV1(&input.Receipt) != nil {
		return failure("validation_error", "present requires a workspace and 1 to 8 valid file declarations")
	}
	targets := make([]domaintoolresult.ProtectedPresentTargetV1, 0, len(input.Receipt.Files))
	for _, file := range input.Receipt.Files {
		if input.Context.Err() != nil {
			return failure("tool_cancelled", "present was canceled")
		}
		resolved, ok := ResolveReadPathInfo(input.Workspace, file.Path, input.SandboxMode, input.ReadRoots)
		if !ok {
			return failure("workspace_escape", "present path is outside the authorized read roots")
		}
		if blocked := protectedPathOutputForSandbox(input.Workspace, resolved.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
			return nil, resolved.DisplayOutput(blocked)
		}
		// Alias resolution canonicalizes its target. Inspect the declaration
		// entry before that canonicalization so a final symlink is still refused.
		entryPath := resolved.Path
		if resolved.ExternalAlias {
			sub, err := filepath.Rel(resolved.DisplayRoot, resolved.DisplayPath)
			if err != nil || !filepath.IsLocal(sub) {
				return failure("validation_error", "present declaration is invalid")
			}
			entryPath = filepath.Join(resolved.Root, sub)
		}
		entry, err := os.Lstat(entryPath)
		if errors.Is(err, os.ErrNotExist) {
			return failure("not_found", "present file does not exist; create it or correct its path")
		}
		if err != nil {
			return failure("present_failed", "present could not inspect the file")
		}
		if !entry.Mode().IsRegular() {
			return failure("validation_error", "present accepts existing regular files only")
		}
		info, err := os.Stat(resolved.Path)
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(entry, info) {
			return failure("present_failed", "present file changed during validation")
		}
		// Recheck the existing policy after metadata observation. This receipt
		// confers no authority to open a later replacement without a fresh check.
		current, ok := ResolveReadPathInfo(input.Workspace, file.Path, input.SandboxMode, input.ReadRoots)
		if !ok || current.Path != resolved.Path {
			return failure("workspace_escape", "present path changed during validation")
		}
		if blocked := protectedPathOutputForSandbox(input.Workspace, current.Path, input.ProtectedReadDirs, input.SandboxMode); blocked != nil {
			return nil, current.DisplayOutput(blocked)
		}
		targets = append(targets, domaintoolresult.ProtectedPresentTargetV1{Index: file.Index, Path: resolved.Path})
	}
	if input.Context.Err() != nil {
		return failure("tool_cancelled", "present was canceled")
	}
	if domaintoolresult.ValidateProtectedPresentTargetsV1(&input.Receipt, targets) != nil {
		return failure("validation_error", "present targets are invalid")
	}
	return targets, nil
}
