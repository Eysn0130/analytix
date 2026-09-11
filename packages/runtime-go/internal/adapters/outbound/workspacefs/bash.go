package workspacefs

import (
	"os"
	"path/filepath"
	"strings"
)

func ValidateBashWorkspace(workspace string, workspaceAbsolute bool) (string, map[string]any, bool) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || !workspaceAbsolute {
		return "", map[string]any{"code": "invalid_workspace", "error": "workspace must be an absolute path"}, true
	}
	cleanWorkspace := filepath.Clean(workspace)
	info, err := os.Stat(cleanWorkspace)
	if err != nil {
		if os.IsNotExist(err) {
			return "", map[string]any{"code": "invalid_workspace", "error": "workspace path does not exist: " + cleanWorkspace, "cwd": cleanWorkspace}, true
		}
		return "", map[string]any{"code": "invalid_workspace", "error": "workspace path is not accessible: " + cleanWorkspace + ": " + err.Error(), "cwd": cleanWorkspace}, true
	}
	if !info.IsDir() {
		return "", map[string]any{"code": "invalid_workspace", "error": "workspace path is not a directory: " + cleanWorkspace, "cwd": cleanWorkspace}, true
	}
	return cleanWorkspace, nil, false
}
