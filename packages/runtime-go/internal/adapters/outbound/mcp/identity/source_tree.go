package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sourceTreeRecord struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func ComputeSourceTreeSHA256(root string) (string, error) {
	return computeSourceTreeSHA256(root, "")
}

func computeSourceTreeSHA256(root string, excludedPath string) (string, error) {
	realRoot, err := canonicalSourceTreeRoot(root)
	if err != nil {
		return "", err
	}
	records := []sourceTreeRecord{}
	err = filepath.WalkDir(realRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == realRoot {
			return nil
		}
		relative, err := filepath.Rel(realRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !portableSourcePath(relative) {
			return errors.New("plugin source path is not portable")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("plugin source tree contains a symbolic link")
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("plugin source tree contains a non-regular file")
		}
		if relative == excludedPath {
			return nil
		}
		body, err := readPinnedRegularFile(path, realRoot, 0)
		if err != nil || int64(len(body)) != info.Size() {
			clear(body)
			return errors.New("plugin source file changed while hashing")
		}
		digest := sha256.Sum256(body)
		clear(body)
		records = append(records, sourceTreeRecord{Path: relative, Size: info.Size(), SHA256: hex.EncodeToString(digest[:])})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	body, err := json.Marshal(records)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalSourceTreeRoot(root string) (string, error) {
	if root == "" || root != strings.TrimSpace(root) {
		return "", errors.New("plugin source root is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("plugin source root is invalid")
	}
	realRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil || filepath.Clean(absolute) != filepath.Clean(realRoot) {
		return "", errors.New("plugin source root must not be a symbolic link")
	}
	rootInfo, err := os.Lstat(realRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("plugin source root must be a real directory")
	}
	return realRoot, nil
}

func portableSourcePath(value string) bool {
	if value == "" || strings.Contains(value, `\`) {
		return false
	}
	for _, char := range []byte(value) {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}
