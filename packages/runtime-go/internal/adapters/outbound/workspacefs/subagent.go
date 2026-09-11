package workspacefs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSubagentWorkspace resolves an untrusted task workspace to the
// existing real directory used as host authority by subagent admission.
func ValidateSubagentWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", nil
	}
	if workspace == "~" || strings.HasPrefix(workspace, "~/") || strings.HasPrefix(workspace, "~\\") {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			if workspace == "~" {
				workspace = home
			} else {
				workspace = filepath.Join(home, filepath.FromSlash(strings.ReplaceAll(workspace[2:], "\\", "/")))
			}
		}
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("workspace path is invalid: %s", workspace)
	}
	realPath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("workspace path does not exist: %s", absolute)
		}
		return "", fmt.Errorf("workspace path is not accessible: %s: %w", absolute, err)
	}
	info, err := os.Stat(realPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("workspace path does not exist: %s", realPath)
		}
		return "", fmt.Errorf("workspace path is not accessible: %s: %w", realPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path is not a directory: %s", realPath)
	}
	return filepath.Clean(realPath), nil
}
