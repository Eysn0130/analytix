package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
)

var (
	ErrTextFileIsDirectory = errors.New("path is a directory")
	ErrTextFileBinary      = errors.New("path is not a text file")
)

type TextFileReadResult struct {
	Content  string
	Encoding string
	RawBytes []byte
}

type TextWritePlan struct {
	Path         string
	RelativePath string
	Before       string
	BeforeExists bool
	BeforeHash   string
	Encoding     string
	Mode         os.FileMode
	DiffKind     filetoolsapp.DiffKind
	IncludeDiff  bool
}

type ExistingTextMutationOptions struct {
	Workspace                  string
	Path                       string
	SandboxMode                string
	AllowWriteRoots            []string
	RequiredExtension          string
	UnsupportedExtensionError  string
	BinaryError                string
	WorkspaceEscapeFieldPrefix string
}

type ExistingTextMutationPlan struct {
	Path         string
	RelativePath string
	Before       string
	BeforeHash   string
	Encoding     string
	Mode         os.FileMode
}

type OperationError struct {
	Code         string
	Message      string
	Path         string
	RelativePath string
}

func (err OperationError) Error() string {
	return err.Message
}

func ReadTextFile(path string) (TextFileReadResult, error) {
	result := TextFileReadResult{}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return result, ErrTextFileIsDirectory
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	content, encoding, textOK := filetoolsapp.DecodeTextBytes(data)
	if !textOK {
		return result, ErrTextFileBinary
	}
	result.Content = content
	result.Encoding = encoding
	result.RawBytes = append([]byte(nil), data...)
	return result, nil
}

func PrepareExistingTextMutation(options ExistingTextMutationOptions) (ExistingTextMutationPlan, error) {
	path := strings.TrimSpace(options.Path)
	if path == "" {
		return ExistingTextMutationPlan{}, filetoolsapp.ToolError{Code: "validation_error", Message: "path is required"}
	}
	resolved, ok := ResolveWritePathForSandbox(options.Workspace, path, options.SandboxMode, options.AllowWriteRoots)
	if !ok {
		field := strings.TrimSpace(options.WorkspaceEscapeFieldPrefix)
		if field == "" {
			field = "path"
		}
		return ExistingTextMutationPlan{}, filetoolsapp.ToolError{Code: "workspace_escape", Message: field + " must stay inside workspace or configured allow_write root"}
	}
	relativePath := WorkspaceRelativePath(options.Workspace, resolved)
	if required := strings.TrimSpace(options.RequiredExtension); required != "" && strings.ToLower(filepath.Ext(resolved)) != strings.ToLower(required) {
		message := strings.TrimSpace(options.UnsupportedExtensionError)
		if message == "" {
			message = "unsupported file type"
		}
		return ExistingTextMutationPlan{}, OperationError{Code: "unsupported_file_type", Message: message, Path: resolved, RelativePath: relativePath}
	}
	state, err := inspectAtomicTextTarget(resolved, false)
	if err != nil {
		return ExistingTextMutationPlan{}, OperationError{Code: "read_failed", Message: err.Error(), Path: resolved, RelativePath: relativePath}
	}
	if !state.Exists {
		return ExistingTextMutationPlan{}, OperationError{Code: "read_failed", Message: os.ErrNotExist.Error(), Path: resolved, RelativePath: relativePath}
	}
	before, encoding, textOK := filetoolsapp.DecodeTextBytes(state.Content)
	if !textOK {
		message := strings.TrimSpace(options.BinaryError)
		if message == "" {
			message = "operation only supports text files"
		}
		return ExistingTextMutationPlan{}, OperationError{Code: "binary_file", Message: message, Path: resolved, RelativePath: relativePath}
	}
	return ExistingTextMutationPlan{
		Path:         resolved,
		RelativePath: relativePath,
		Before:       before,
		BeforeHash:   digestAtomicText(state.Content),
		Encoding:     encoding,
		Mode:         state.Mode.Perm(),
	}, nil
}

func ApplyExistingTextMutation(plan ExistingTextMutationPlan, content string) ([]byte, error) {
	encoded := filetoolsapp.EncodeTextBytes(content, plan.Encoding)
	if err := atomicReplaceText(atomicTextReplaceRequest{
		Path: plan.Path, Content: encoded, ExpectedExists: true, ExpectedHash: plan.BeforeHash,
		PreserveMode: true, DefaultMode: plan.Mode,
	}); err != nil {
		return nil, OperationError{Code: "write_failed", Message: err.Error(), Path: plan.Path, RelativePath: plan.RelativePath}
	}
	return encoded, nil
}

func PrepareTextWrite(workspace string, path string, allowWriteRoots []string) (TextWritePlan, error) {
	return PrepareTextWriteForSandbox(workspace, path, "", allowWriteRoots)
}

func PrepareTextWriteForSandbox(workspace string, path string, sandboxMode string, allowWriteRoots []string) (TextWritePlan, error) {
	if path == "" {
		return TextWritePlan{}, filetoolsapp.ToolError{Code: "validation_error", Message: "path is required"}
	}
	resolved, ok := ResolveWritePathForSandbox(workspace, path, sandboxMode, allowWriteRoots)
	if !ok {
		return TextWritePlan{}, filetoolsapp.ToolError{Code: "workspace_escape", Message: "path must stay inside workspace or configured allow_write root"}
	}
	plan := TextWritePlan{
		Path:         resolved,
		RelativePath: WorkspaceRelativePath(workspace, resolved),
		Encoding:     filetoolsapp.TextEncodingUTF8,
		DiffKind:     filetoolsapp.DiffCreate,
		IncludeDiff:  true,
		Mode:         0o644,
	}
	state, err := inspectAtomicTextTarget(resolved, true)
	if err != nil {
		return TextWritePlan{}, OperationError{Code: "read_failed", Message: err.Error(), Path: resolved, RelativePath: plan.RelativePath}
	}
	if state.Exists {
		plan.BeforeExists = true
		plan.BeforeHash = digestAtomicText(state.Content)
		plan.Mode = state.Mode.Perm()
		if decoded, encoding, ok := filetoolsapp.DecodeTextBytes(state.Content); ok {
			plan.Before = decoded
			plan.Encoding = encoding
		} else {
			plan.IncludeDiff = false
		}
		plan.DiffKind = filetoolsapp.DiffModify
	}
	return plan, nil
}

func ApplyTextWrite(plan TextWritePlan, content string) ([]byte, error) {
	encoded := filetoolsapp.EncodeTextBytes(content, plan.Encoding)
	if err := atomicReplaceText(atomicTextReplaceRequest{
		Path: plan.Path, Content: encoded, ExpectedExists: plan.BeforeExists, ExpectedHash: plan.BeforeHash,
		CreateParents: true, PreserveMode: true, DefaultMode: plan.Mode,
	}); err != nil {
		return nil, filetoolsapp.ToolError{Code: "write_failed", Message: err.Error()}
	}
	return encoded, nil
}
