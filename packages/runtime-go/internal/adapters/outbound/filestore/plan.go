package filestore

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ExistingMarkdownRelativePaths(workspaceRoot string, relativeDir string) map[string]bool {
	existing := map[string]bool{}
	relativeDir = strings.Trim(strings.ReplaceAll(strings.TrimSpace(relativeDir), "\\", "/"), "/")
	if strings.TrimSpace(workspaceRoot) == "" || relativeDir == "" {
		return existing
	}
	entries, err := os.ReadDir(filepath.Join(workspaceRoot, filepath.FromSlash(relativeDir)))
	if err != nil {
		return existing
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}
		existing[relativeDir+"/"+entry.Name()] = true
	}
	return existing
}

func ResolveWorkspaceRelativePath(workspaceRoot string, relativePath string) (string, bool) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" || !filepath.IsAbs(workspaceRoot) {
		return "", false
	}
	target := filepath.Clean(filepath.Join(workspaceRoot, filepath.FromSlash(relativePath)))
	root := filepath.Clean(workspaceRoot)
	if !PathWithinRoot(root, target) {
		return "", false
	}
	return target, true
}

func WriteFileAtomic(path string, content []byte, now func() time.Time) (string, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	prepared, err := prepareAtomicFileWrite(path, content)
	if err != nil {
		return "", err
	}
	if err := executePreparedAtomicFileWrite(prepared); err != nil {
		return "", err
	}
	return now().UTC().Format(time.RFC3339Nano), nil
}

type preparedAtomicFileWrite struct {
	path           string
	content        []byte
	expectedExists bool
	expectedHash   string
}

func prepareAtomicFileWrite(path string, content []byte) (preparedAtomicFileWrite, error) {
	state, err := inspectAtomicTextTarget(path, true)
	if err != nil {
		return preparedAtomicFileWrite{}, err
	}
	expectedHash := ""
	if state.Exists {
		expectedHash = digestAtomicText(state.Content)
	}
	return preparedAtomicFileWrite{
		path: path, content: append([]byte(nil), content...), expectedExists: state.Exists, expectedHash: expectedHash,
	}, nil
}

func executePreparedAtomicFileWrite(prepared preparedAtomicFileWrite) error {
	return atomicReplaceText(atomicTextReplaceRequest{
		Path: prepared.path, Content: append([]byte(nil), prepared.content...), ExpectedExists: prepared.expectedExists,
		ExpectedHash: prepared.expectedHash, CreateParents: true, PreserveMode: true, DefaultMode: 0o644,
	})
}
