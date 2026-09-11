package filestore

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

const (
	maxSkillSnapshotReadBytes          = 8 * 1024 * 1024
	maxManagedHubProjectionMarkerBytes = 64 * 1024
	managedHubSkillProjectionMarkerV1  = ".analytix-hub-skill.json"
)

type managedHubSkillProjectionV1 struct {
	ManagedBy  string `json:"managedBy"`
	Platform   string `json:"platform"`
	PluginName string `json:"pluginName"`
	SkillName  string `json:"skillName"`
	SkillPath  string `json:"skillPath"`
	SourceKind string `json:"sourceKind"`
	Version    string `json:"version"`
}

func NormalizeSkillRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if root == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			root = home
		}
	}
	if strings.HasPrefix(root, "~/") || strings.HasPrefix(root, "~\\") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			root = filepath.Join(home, strings.TrimLeft(root[2:], `/\`))
		}
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	root = filepath.Clean(root)
	if evaluated, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(evaluated)
	} else {
		root = normalizeSkillRootExistingPrefix(root)
	}
	return root
}

func normalizeSkillRootExistingPrefix(root string) string {
	probe := root
	tail := []string{}
	for {
		if evaluated, err := filepath.EvalSymlinks(probe); err == nil {
			for index := len(tail) - 1; index >= 0; index-- {
				evaluated = filepath.Join(evaluated, tail[index])
			}
			return filepath.Clean(evaluated)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return root
		}
		tail = append(tail, filepath.Base(probe))
		probe = parent
	}
}

func SkillPackageCandidates(root string) ([]string, error) {
	root = NormalizeSkillRoot(root)
	candidates := []string{}
	if SkillPackageExists(root) {
		return []string{root}, nil
	}
	const maxDepth = 3
	var walk func(path string, depth int) error
	walk = func(path string, depth int) error {
		if depth > maxDepth {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if SkillPackageExists(path) {
			candidates = append(candidates, path)
			return nil
		}
		if depth > 0 && SkillFlatFileExists(path) {
			candidates = append(candidates, path)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 || SkillDiscoveryDirIgnored(entry.Name()) {
				continue
			}
			if err := walk(filepath.Join(path, entry.Name()), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	sort.Strings(candidates)
	return candidates, nil
}

func SkillRootDirectoryExists(root string) bool {
	info, err := os.Lstat(root)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

func SkillPackageExists(packagePath string) bool {
	info, err := os.Lstat(packagePath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	for _, name := range []string{"SKILL.md", "skill.json"} {
		entry, entryErr := os.Lstat(filepath.Join(packagePath, name))
		if entryErr == nil && entry.Mode()&os.ModeSymlink == 0 && entry.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func SkillFlatFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() &&
		strings.EqualFold(filepath.Ext(path), ".md") && !strings.EqualFold(filepath.Base(path), "SKILL.md")
}

func SkillDiscoveryDirIgnored(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	return name == "" || strings.HasPrefix(name, ".") || name == "node_modules" || name == "references" || name == "scripts"
}

func SkillNestedCandidate(path, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return len(strings.Split(rel, string(filepath.Separator))) > 1
}

func LoadSkillSummary(path, root, dataDir string) (map[string]any, error) {
	loaded, err := LoadSkillPackage(path, root, dataDir)
	return loaded.Record, err
}

func LoadSkillPackage(candidate, root, dataDir string) (toolcatalogapp.LoadedSkillPackage, error) {
	source, ok, err := SkillSummarySource(candidate, root, dataDir)
	if err != nil || !ok {
		return toolcatalogapp.LoadedSkillPackage{}, err
	}
	record, ok := toolcatalogapp.SkillSummaryRecord(source)
	if !ok {
		return toolcatalogapp.LoadedSkillPackage{}, nil
	}
	return toolcatalogapp.LoadedSkillPackage{Record: record, Snapshot: source.Snapshot}, nil
}

func SkillSummarySource(candidate, root, dataDir string) (toolcatalogapp.SkillSummarySource, bool, error) {
	root = NormalizeSkillRoot(root)
	candidate = filepath.Clean(candidate)
	if !pathIsInside(candidate, root) {
		return toolcatalogapp.SkillSummarySource{}, false, errors.New("skill candidate is outside its configured root")
	}
	scope := SkillScope(root, dataDir)
	if SkillFlatFileExists(candidate) {
		packagePath := filepath.Dir(candidate)
		entry := filepath.Base(candidate)
		snapshot, err := snapshotSkillPackage(packagePath, entry, nil)
		if err != nil {
			return toolcatalogapp.SkillSummarySource{}, false, err
		}
		entryBytes, _ := snapshot.File(entry)
		return toolcatalogapp.SkillSummarySource{
			Format:        "markdown",
			Root:          root,
			Path:          packagePath,
			EntryPath:     candidate,
			Entry:         entry,
			PackageDigest: snapshot.Digest(),
			Scope:         scope,
			DefaultID:     strings.TrimSuffix(filepath.Base(candidate), filepath.Ext(candidate)),
			Legacy:        true,
			Nested:        SkillNestedCandidate(candidate, root),
			MarkdownText:  string(entryBytes),
			Snapshot:      snapshot,
		}, true, nil
	}
	packageInfo, err := os.Lstat(candidate)
	if err != nil || packageInfo.Mode()&os.ModeSymlink != 0 || !packageInfo.IsDir() {
		return toolcatalogapp.SkillSummarySource{}, false, errors.New("skill package is not a regular directory")
	}
	if err := rejectInertOrInvalidManagedHubSkillProjection(candidate); err != nil {
		return toolcatalogapp.SkillSummarySource{}, false, err
	}

	manifestPath := filepath.Join(candidate, "skill.json")
	if manifestInfo, manifestErr := os.Lstat(manifestPath); manifestErr == nil {
		if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
			return toolcatalogapp.SkillSummarySource{}, false, errors.New("skill manifest must be a regular non-symlink file")
		}
		manifestBytes, readErr := readSkillSnapshotFile(candidate, "skill.json")
		if readErr != nil {
			return toolcatalogapp.SkillSummarySource{}, false, readErr
		}
		manifest := map[string]any{}
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return toolcatalogapp.SkillSummarySource{}, false, err
		}
		entry := strings.TrimSpace(contracts.StringField(manifest, "entry"))
		if entry == "" {
			entry = "SKILL.md"
		}
		entry, err = domainskill.NormalizeRelativePath(entry)
		if err != nil || !strings.EqualFold(filepath.Ext(filepath.FromSlash(entry)), ".md") {
			return toolcatalogapp.SkillSummarySource{}, false, errors.New("skill manifest entry must be a canonical in-package markdown path")
		}
		snapshot, snapshotErr := snapshotSkillPackage(candidate, entry, manifestBytes)
		if snapshotErr != nil {
			return toolcatalogapp.SkillSummarySource{}, false, snapshotErr
		}
		return toolcatalogapp.SkillSummarySource{
			Format:        "manifest",
			Root:          root,
			Path:          candidate,
			EntryPath:     filepath.Join(candidate, filepath.FromSlash(entry)),
			Entry:         entry,
			PackageDigest: snapshot.Digest(),
			Scope:         scope,
			DefaultID:     filepath.Base(candidate),
			Manifest:      manifest,
			Snapshot:      snapshot,
		}, true, nil
	}

	entry := "SKILL.md"
	snapshot, snapshotErr := snapshotSkillPackage(candidate, entry, nil)
	if snapshotErr != nil {
		return toolcatalogapp.SkillSummarySource{}, false, snapshotErr
	}
	entryBytes, _ := snapshot.File(entry)
	return toolcatalogapp.SkillSummarySource{
		Format:        "markdown",
		Root:          root,
		Path:          candidate,
		EntryPath:     filepath.Join(candidate, entry),
		Entry:         entry,
		PackageDigest: snapshot.Digest(),
		Scope:         scope,
		DefaultID:     filepath.Base(candidate),
		Legacy:        true,
		Nested:        SkillNestedCandidate(candidate, root),
		MarkdownText:  string(entryBytes),
		Snapshot:      snapshot,
	}, true, nil
}

func rejectInertOrInvalidManagedHubSkillProjection(candidate string) error {
	markerPath := filepath.Join(candidate, managedHubSkillProjectionMarkerV1)
	markerInfo, err := os.Lstat(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("managed Hub skill projection state is unreadable")
	}
	if markerInfo.Mode()&os.ModeSymlink != 0 || !markerInfo.Mode().IsRegular() {
		return errors.New("managed Hub skill projection marker must be a regular non-symlink file")
	}
	markerBytes, err := readSkillSnapshotFile(candidate, managedHubSkillProjectionMarkerV1)
	if err != nil {
		return errors.New("managed Hub skill projection marker is unreadable")
	}
	if err := domainjsonstrict.Validate(markerBytes, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxManagedHubProjectionMarkerBytes,
		MaxDepth:       4,
		MaxTokens:      32,
		MaxStringBytes: 4096,
	}); err != nil {
		return errors.New("managed Hub skill projection marker is invalid")
	}
	var marker managedHubSkillProjectionV1
	decoder := json.NewDecoder(bytes.NewReader(markerBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return errors.New("managed Hub skill projection marker is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("managed Hub skill projection marker contains trailing JSON")
	}
	skillName := filepath.Base(candidate)
	if strings.TrimSpace(marker.ManagedBy) != "analytix-hub" ||
		strings.TrimSpace(marker.Platform) == "" ||
		strings.TrimSpace(marker.PluginName) == "" ||
		strings.TrimSpace(marker.SkillName) != skillName ||
		strings.TrimSpace(marker.SkillPath) != "skills/"+skillName+"/SKILL.md" ||
		strings.TrimSpace(marker.SourceKind) != "plugin" ||
		strings.TrimSpace(marker.Version) == "" {
		return errors.New("managed Hub skill projection marker does not match its package")
	}
	if strings.TrimSpace(marker.PluginName) == "analytix-fund-analysis" {
		// Funds projections are user-writable convenience copies, never the
		// receipt-bound current-run plugin authority. The verified direct-cache
		// generation is discovered independently as its own root.
		return errors.New("managed funds skill projection is not executable skill authority")
	}
	return nil
}

func snapshotSkillPackage(packagePath, entry string, manifestBytes []byte) (domainskill.PackageSnapshot, error) {
	entry, err := domainskill.NormalizeRelativePath(entry)
	if err != nil {
		return domainskill.PackageSnapshot{}, err
	}
	inputs := []domainskill.FileInput{}
	if manifestBytes != nil {
		inputs = append(inputs, domainskill.FileInput{RelativePath: "skill.json", Bytes: manifestBytes})
	}
	entryBytes, err := readSkillSnapshotFile(packagePath, entry)
	if err != nil {
		return domainskill.PackageSnapshot{}, err
	}
	inputs = append(inputs, domainskill.FileInput{RelativePath: entry, Bytes: entryBytes})

	references, err := snapshotDirectoryFiles(packagePath, "references", func(ext string) bool {
		return ext == ".md" || ext == ".txt"
	})
	if err != nil {
		return domainskill.PackageSnapshot{}, err
	}
	inputs = append(inputs, references...)
	scripts, err := snapshotDirectoryFiles(packagePath, "scripts", SkillScriptExtensionAllowed)
	if err != nil {
		return domainskill.PackageSnapshot{}, err
	}
	inputs = append(inputs, scripts...)
	return domainskill.NewPackageSnapshot(entry, inputs)
}

func snapshotDirectoryFiles(packagePath, directory string, allowExtension func(string) bool) ([]domainskill.FileInput, error) {
	directoryPath := filepath.Join(packagePath, directory)
	info, err := os.Lstat(directoryPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("skill package supplemental directory must be a regular non-symlink directory")
	}
	directoryHandle, err := os.Open(directoryPath)
	if err != nil {
		return nil, err
	}
	defer directoryHandle.Close()
	openedInfo, err := directoryHandle.Stat()
	postInfo, postErr := os.Lstat(directoryPath)
	if err != nil || postErr != nil || !openedInfo.IsDir() || postInfo.Mode()&os.ModeSymlink != 0 || !postInfo.IsDir() ||
		!os.SameFile(info, openedInfo) || !os.SameFile(openedInfo, postInfo) {
		return nil, errors.New("skill package supplemental directory changed during snapshot")
	}
	entries, err := directoryHandle.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	inputs := []domainskill.FileInput{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("skill package supplemental files must not be symlinks")
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		if entryInfo.IsDir() {
			continue
		}
		if !entryInfo.Mode().IsRegular() {
			return nil, errors.New("skill package supplemental files must be regular files")
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !allowExtension(ext) {
			continue
		}
		relativePath := filepath.ToSlash(filepath.Join(directory, entry.Name()))
		bytes, readErr := readSkillSnapshotFile(packagePath, relativePath)
		if readErr != nil {
			return nil, readErr
		}
		inputs = append(inputs, domainskill.FileInput{RelativePath: relativePath, Bytes: bytes})
	}
	finalInfo, finalErr := os.Lstat(directoryPath)
	if finalErr != nil || finalInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, finalInfo) {
		return nil, errors.New("skill package supplemental directory changed during snapshot")
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].RelativePath < inputs[j].RelativePath })
	return inputs, nil
}

func readSkillSnapshotFile(packagePath, relativePath string) ([]byte, error) {
	return readSkillSnapshotFileWithHook(packagePath, relativePath, nil)
}

func readSkillSnapshotFileWithHook(packagePath, relativePath string, beforeOpen func()) ([]byte, error) {
	relativePath, err := domainskill.NormalizeRelativePath(relativePath)
	if err != nil {
		return nil, err
	}
	packagePath = filepath.Clean(packagePath)
	evaluatedPackage, err := filepath.EvalSymlinks(packagePath)
	if err != nil || filepath.Clean(evaluatedPackage) != packagePath {
		return nil, errors.New("skill package path must resolve without symlink indirection")
	}
	target := filepath.Join(packagePath, filepath.FromSlash(relativePath))
	if !pathIsInside(target, packagePath) {
		return nil, errors.New("skill package file escaped its package root")
	}
	preInfo, err := verifySkillPathComponents(packagePath, relativePath)
	if err != nil {
		return nil, err
	}
	if beforeOpen != nil {
		beforeOpen()
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		return nil, errors.New("skill package file is not regular")
	}
	postInfo, err := os.Lstat(target)
	if err != nil || postInfo.Mode()&os.ModeSymlink != 0 || !postInfo.Mode().IsRegular() ||
		!os.SameFile(preInfo, openedInfo) || !os.SameFile(openedInfo, postInfo) {
		return nil, errors.New("skill package file changed during snapshot")
	}
	bytes, err := io.ReadAll(io.LimitReader(file, maxSkillSnapshotReadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(bytes) > maxSkillSnapshotReadBytes {
		return nil, errors.New("skill package file exceeds snapshot byte limit")
	}
	return bytes, nil
}

func verifySkillPathComponents(packagePath, relativePath string) (os.FileInfo, error) {
	current := packagePath
	rootInfo, err := os.Lstat(current)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("skill package root is not a regular directory")
	}
	parts := strings.Split(filepath.FromSlash(relativePath), string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("skill package path contains a symlink")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("skill package path contains a non-directory component")
		}
		if index == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return nil, errors.New("skill package entry is not a regular file")
			}
			return info, nil
		}
	}
	return nil, errors.New("skill package file path is empty")
}

func pathIsInside(candidate, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func SkillScriptExtensionAllowed(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".bash", ".cjs", ".js", ".mjs", ".ps1", ".py", ".rb", ".sh", ".ts":
		return true
	default:
		return false
	}
}

func SkillScope(root, dataDir string) string {
	root = NormalizeSkillRoot(root)
	dataDir = NormalizeSkillRoot(dataDir)
	if root == "" || dataDir == "" {
		return "global"
	}
	relative, err := filepath.Rel(dataDir, root)
	if err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "project"
	}
	return "global"
}
