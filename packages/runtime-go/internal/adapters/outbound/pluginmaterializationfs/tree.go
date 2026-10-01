package pluginmaterializationfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
)

const (
	maxPluginFileBytesV1 int64 = 128 << 20
	maxPluginTreeBytesV1 int64 = 512 << 20
	maxManifestBytesV1   int64 = 256 << 10
	maxMCPConfigBytesV1  int64 = 256 << 10
)

type SourceTreeIdentityV1 struct {
	SourceRegistrationSHA256 string
	SourceRegistrationJSON   string
	RootRealPath             string
	TreeSHA256               string
	FileCount                uint64
	Declaration              PackageDeclarationIdentityV1
	LegacyV0                 bool
	LegacyPackageID          string
	LegacyVersion            string
	ManifestSHA256           string
	EntrypointSHA256         string
	TotalBytes               int64
}

type PackageDeclarationIdentityV1 struct {
	RawSHA256       string
	CanonicalSHA256 string
	CanonicalJSON   string
	PackageID       string
	PackageVersion  string
}

type sourceTreeRecordV1 struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func InspectSourceTreeV1(ctx context.Context, root string) (SourceTreeIdentityV1, error) {
	return inspectTreeV1(ctx, root, false, false)
}

// InspectPackagedFundsSourceTreeV1 keeps a present companion authoritative,
// including when it is invalid. Only its exact absence enables the bounded
// first-party legacy-v0 inspection path.
func InspectPackagedFundsSourceTreeV1(ctx context.Context, root string) (SourceTreeIdentityV1, error) {
	return inspectTreeV1(ctx, root, false, true)
}

func inspectInstalledTreeV1(ctx context.Context, root string) (SourceTreeIdentityV1, error) {
	return inspectTreeV1(ctx, root, true, true)
}

// InspectDevelopmentSourceTreeV1 inspects only the closed static first-party
// editor declarations. It does not establish packaged admission or execution.
func InspectDevelopmentSourceTreeV1(ctx context.Context, root string) (SourceTreeIdentityV1, error) {
	return inspectTreeForOriginV1(ctx, root, false, false, domainplugin.DevelopmentSourceOriginV1)
}

func inspectSourceForOriginV1(ctx context.Context, root, origin string) (SourceTreeIdentityV1, error) {
	if origin == domainplugin.DevelopmentSourceOriginV1 {
		return InspectDevelopmentSourceTreeV1(ctx, root)
	}
	if origin != "" {
		return SourceTreeIdentityV1{}, errors.New("unknown plugin materialization origin")
	}
	return InspectSourceTreeV1(ctx, root)
}

func inspectInstalledForOriginV1(ctx context.Context, root, origin string) (SourceTreeIdentityV1, error) {
	if origin == domainplugin.DevelopmentSourceOriginV1 {
		return inspectTreeForOriginV1(ctx, root, true, false, origin)
	}
	if origin != "" {
		return SourceTreeIdentityV1{}, errors.New("unknown plugin materialization origin")
	}
	return inspectInstalledTreeV1(ctx, root)
}

func inspectTreeV1(ctx context.Context, root string, excludeHostMarker, allowLegacyV0 bool) (SourceTreeIdentityV1, error) {
	return inspectTreeForOriginV1(ctx, root, excludeHostMarker, allowLegacyV0, "")
}

func inspectTreeForOriginV1(ctx context.Context, root string, excludeHostMarker, allowLegacyV0 bool, origin string) (SourceTreeIdentityV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return SourceTreeIdentityV1{}, context.Canceled
	}
	realRoot, err := canonicalDirectory(root)
	if err != nil {
		return SourceTreeIdentityV1{}, err
	}
	rootBefore, err := os.Lstat(realRoot)
	if err != nil {
		return SourceTreeIdentityV1{}, err
	}
	records := make([]sourceTreeRecordV1, 0, 256)
	var totalBytes int64
	err = filepath.WalkDir(realRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == realRoot {
			return nil
		}
		relative, err := filepath.Rel(realRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !portablePath(relative) {
			return errors.New("bundled plugin source path is not portable")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("bundled plugin source tree contains a symbolic link")
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("bundled plugin source tree contains a non-regular file")
		}
		if relative == domainplugin.InstallMarkerFileNameV1 {
			if !excludeHostMarker {
				return errors.New("packaged plugin source must not carry an installation marker")
			}
			return nil
		}
		digest, size, err := hashStableFile(path, info)
		if err != nil {
			return err
		}
		if size < 0 || size > maxPluginFileBytesV1 || totalBytes > maxPluginTreeBytesV1-size {
			return errors.New("bundled plugin source tree exceeds its byte bound")
		}
		totalBytes += size
		records = append(records, sourceTreeRecordV1{Path: relative, Size: size, SHA256: digest})
		if len(records) > domainplugin.MaxSourceTreeFilesV1 {
			return errors.New("bundled plugin source tree exceeds its file-count bound")
		}
		return nil
	})
	if err != nil {
		return SourceTreeIdentityV1{}, err
	}
	rootAfter, err := os.Lstat(realRoot)
	if err != nil || !rootAfter.IsDir() || rootAfter.Mode()&os.ModeSymlink != 0 || !os.SameFile(rootBefore, rootAfter) {
		return SourceTreeIdentityV1{}, errors.New("bundled plugin source root changed while reading")
	}
	if len(records) == 0 {
		return SourceTreeIdentityV1{}, errors.New("bundled plugin source tree is empty")
	}
	sort.Slice(records, func(left, right int) bool { return records[left].Path < records[right].Path })
	body, err := json.Marshal(records)
	if err != nil {
		return SourceTreeIdentityV1{}, err
	}
	treeDigest := sha256.Sum256(body)
	identity := SourceTreeIdentityV1{
		RootRealPath: realRoot, TreeSHA256: hex.EncodeToString(treeDigest[:]),
		FileCount: uint64(len(records)), TotalBytes: totalBytes,
	}
	recordPaths := make(map[string]struct{}, len(records))
	recordSHA256 := make(map[string]string, len(records))
	for _, record := range records {
		recordPaths[record.Path] = struct{}{}
		recordSHA256[record.Path] = record.SHA256
		switch record.Path {
		case domainpluginpackage.DeclarationRelativePathV1:
			identity.Declaration.RawSHA256 = record.SHA256
		case domainplugin.ManifestRelativePathV1:
			identity.ManifestSHA256 = record.SHA256
		}
	}
	if identity.ManifestSHA256 == "" {
		return SourceTreeIdentityV1{}, errors.New("bundled plugin source tree is missing its manifest")
	}
	if identity.Declaration.RawSHA256 == "" {
		if !allowLegacyV0 {
			return SourceTreeIdentityV1{}, errors.New("bundled plugin source tree is missing its declaration")
		}
		declarationAbsent, presenceErr := legacyDeclarationAbsentV1(realRoot)
		if presenceErr != nil {
			return SourceTreeIdentityV1{}, presenceErr
		}
		if !declarationAbsent {
			return SourceTreeIdentityV1{}, errors.New("bundled plugin package declaration is present but invalid")
		}
		packageID, packageVersion, legacyErr := inspectLegacyFundsV0Projections(
			realRoot,
			recordPaths,
			recordSHA256[domainplugin.ManifestRelativePathV1],
			recordSHA256[".mcp.json"],
		)
		if legacyErr != nil {
			return SourceTreeIdentityV1{}, legacyErr
		}
		identity.EntrypointSHA256 = recordSHA256[legacyFundsMCPEntrypointV0]
		if identity.EntrypointSHA256 == "" {
			return SourceTreeIdentityV1{}, errors.New("bundled plugin legacy-v0 entrypoint is missing")
		}
		declarationAbsent, presenceErr = legacyDeclarationAbsentV1(realRoot)
		if presenceErr != nil || !declarationAbsent {
			return SourceTreeIdentityV1{}, errors.New("bundled plugin package declaration changed during legacy-v0 inspection")
		}
		identity.LegacyV0 = true
		identity.LegacyPackageID = packageID
		identity.LegacyVersion = packageVersion
		return identity, nil
	}
	declarationIdentity, declaration, err := inspectPackageDeclarationV1(realRoot, identity.Declaration.RawSHA256, recordPaths)
	if err != nil {
		return SourceTreeIdentityV1{}, err
	}
	identity.Declaration = declarationIdentity
	if origin == domainplugin.DevelopmentSourceOriginV1 {
		if !excludeHostMarker {
			if err := domainpluginpackage.ValidateDevelopmentSourceDeclarationV1(declaration); err != nil {
				return SourceTreeIdentityV1{}, err
			}
		}
		name, version, err := inspectManifestIdentityV1(realRoot, identity.ManifestSHA256)
		if err != nil || name != declaration.PackageID || version != declaration.PackageVersion {
			return SourceTreeIdentityV1{}, errors.New("development source manifest does not match the declaration")
		}
		if _, exists := recordPaths[".mcp.json"]; exists {
			return SourceTreeIdentityV1{}, errors.New("static development source must not carry MCP configuration")
		}
		observed := domainpluginpackage.DevelopmentSourceRegistrationV1{
			Identity: declaration.IdentityV1(), DeclarationRawSHA256: identity.Declaration.RawSHA256,
			DeclarationCanonicalSHA256: identity.Declaration.CanonicalSHA256, DeclarationCanonicalJSON: identity.Declaration.CanonicalJSON,
			SourceTreeSHA256: identity.TreeSHA256, SourceTreeFileCount: identity.FileCount, ManifestSHA256: identity.ManifestSHA256,
			PublicUISHA256: recordSHA256["ui/editor.json"], AdapterSHA256: recordSHA256["assets/adapter.json"],
		}
		if len(declaration.Contributions.Skills) != 0 {
			skill, ok := domainpluginpackage.StaticEditorSkillContributionV1(declaration.PackageID)
			if !ok {
				return SourceTreeIdentityV1{}, errors.New("unrecognized static editor skill")
			}
			switch declaration.PackageID {
			case "analytix-documents":
				observed.DocumentsSkillSHA256 = recordSHA256[skill.Path]
			case "analytix-spreadsheets":
				observed.SpreadsheetsSkillSHA256 = recordSHA256[skill.Path]
			case "analytix-presentations":
				observed.PresentationsSkillSHA256 = recordSHA256[skill.Path]
			case "analytix-canvas":
				observed.CanvasSkillSHA256 = recordSHA256[skill.Path]
			}
		}
		if excludeHostMarker {
			// Installed bytes may predate preview-only admission. Reconstruct
			// their historical digest; Store still verifies the signed receipt,
			// full tree, identity and marker before resolving or upgrading it.
			canonical, digest, err := domainpluginpackage.ReconstructHistoricalDevelopmentSourceRegistrationV1(observed)
			if err != nil {
				return SourceTreeIdentityV1{}, err
			}
			identity.SourceRegistrationJSON = string(canonical)
			identity.SourceRegistrationSHA256 = digest
		} else {
			registration, err := domainpluginpackage.NewDevelopmentSourceRegistrationV1(observed)
			if err != nil {
				return SourceTreeIdentityV1{}, err
			}
			canonical, err := domainpluginpackage.DevelopmentSourceRegistrationV1Bytes(registration)
			if err != nil {
				return SourceTreeIdentityV1{}, err
			}
			identity.SourceRegistrationJSON = string(canonical)
			identity.SourceRegistrationSHA256 = domainpluginpackage.DevelopmentSourceRegistrationSHA256V1(registration)
		}
		return identity, nil
	}
	if err := validateManifestAndDisabledMCP(
		realRoot,
		declaration,
		recordSHA256[domainplugin.ManifestRelativePathV1],
		recordSHA256[".mcp.json"],
	); err != nil {
		return SourceTreeIdentityV1{}, err
	}
	entrypointRelativePath, err := fundsMCPEntrypointFromDeclarationV1(declaration)
	if err != nil {
		return SourceTreeIdentityV1{}, err
	}
	identity.EntrypointSHA256 = recordSHA256[entrypointRelativePath]
	if identity.EntrypointSHA256 == "" {
		return SourceTreeIdentityV1{}, errors.New("bundled plugin declared entrypoint is missing")
	}
	return identity, nil
}

// FundsMCPEntrypointFromCanonicalDeclarationV1 returns the exact MCP
// contribution selected by a canonical declaration identity. Admission and
// signed-receipt verification remain the caller's responsibility.
func FundsMCPEntrypointFromCanonicalDeclarationV1(identity PackageDeclarationIdentityV1) (string, error) {
	declaration, err := domainpluginpackage.ParseDeclarationV1([]byte(identity.CanonicalJSON))
	if err != nil {
		return "", errors.New("bundled plugin canonical declaration identity is invalid")
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil || string(canonical) != identity.CanonicalJSON {
		return "", errors.New("bundled plugin canonical declaration identity is invalid")
	}
	digest := sha256.Sum256(canonical)
	declarationIdentity := declaration.IdentityV1()
	if hex.EncodeToString(digest[:]) != identity.CanonicalSHA256 ||
		declarationIdentity.PackageID != identity.PackageID ||
		declarationIdentity.PackageVersion != identity.PackageVersion {
		return "", errors.New("bundled plugin canonical declaration identity is invalid")
	}
	return fundsMCPEntrypointFromDeclarationV1(declaration)
}

func fundsMCPEntrypointFromDeclarationV1(declaration domainpluginpackage.DeclarationV1) (string, error) {
	if len(declaration.Contributions.MCPServers) != 1 ||
		declaration.Contributions.MCPServers[0].ID != "analytix_funds" {
		return "", errors.New("bundled plugin Funds MCP contribution is invalid")
	}
	return declaration.Contributions.MCPServers[0].Entrypoint, nil
}

func legacyDeclarationAbsentV1(root string) (bool, error) {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1)))
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, errors.New("bundled plugin package declaration presence cannot be verified")
}

func inspectPackageDeclarationV1(
	root string,
	expectedRawSHA256 string,
	recordPaths map[string]struct{},
) (PackageDeclarationIdentityV1, domainpluginpackage.DeclarationV1, error) {
	body, err := stableReadFile(
		filepath.Join(root, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1)),
		domainpluginpackage.MaxDeclarationBytesV1,
	)
	if err != nil {
		return PackageDeclarationIdentityV1{}, domainpluginpackage.DeclarationV1{}, errors.New("bundled plugin package declaration is missing or unstable")
	}
	rawDigest := sha256.Sum256(body)
	rawSHA256 := hex.EncodeToString(rawDigest[:])
	if rawSHA256 != expectedRawSHA256 {
		return PackageDeclarationIdentityV1{}, domainpluginpackage.DeclarationV1{}, errors.New("bundled plugin package declaration changed during inspection")
	}
	declaration, err := domainpluginpackage.ParseDeclarationV1(body)
	if err != nil {
		return PackageDeclarationIdentityV1{}, domainpluginpackage.DeclarationV1{}, errors.New("bundled plugin package declaration is invalid")
	}
	if !declarationPathsExistV1(declaration, recordPaths) {
		return PackageDeclarationIdentityV1{}, domainpluginpackage.DeclarationV1{}, errors.New("bundled plugin package declaration references a missing contribution")
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		return PackageDeclarationIdentityV1{}, domainpluginpackage.DeclarationV1{}, errors.New("bundled plugin package declaration cannot be canonicalized")
	}
	canonicalDigest := sha256.Sum256(canonical)
	identity := declaration.IdentityV1()
	return PackageDeclarationIdentityV1{
		RawSHA256:       rawSHA256,
		CanonicalSHA256: hex.EncodeToString(canonicalDigest[:]),
		CanonicalJSON:   string(canonical),
		PackageID:       identity.PackageID,
		PackageVersion:  identity.PackageVersion,
	}, declaration, nil
}

func declarationPathsExistV1(
	declaration domainpluginpackage.DeclarationV1,
	recordPaths map[string]struct{},
) bool {
	for _, contributions := range [][]domainpluginpackage.PathContributionV1{
		declaration.Contributions.Skills,
		declaration.Contributions.Hooks,
		declaration.Contributions.Assets,
		declaration.Contributions.PublicUI,
	} {
		for _, contribution := range contributions {
			if _, exists := recordPaths[contribution.Path]; !exists {
				return false
			}
		}
	}
	for _, server := range declaration.Contributions.MCPServers {
		if _, exists := recordPaths[server.Entrypoint]; !exists {
			return false
		}
	}
	return true
}

func validateManifestAndDisabledMCP(
	root string,
	declaration domainpluginpackage.DeclarationV1,
	expectedManifestSHA256 string,
	expectedMCPConfigSHA256 string,
) error {
	name, version, err := inspectManifestIdentityV1(root, expectedManifestSHA256)
	if err != nil {
		return err
	}
	if name != declaration.PackageID || version != declaration.PackageVersion {
		return errors.New("bundled plugin projection parity is invalid")
	}
	return validateDisabledMCPV1(root, expectedMCPConfigSHA256, declaration.Contributions.MCPServers)
}

func inspectManifestIdentityV1(root, expectedManifestSHA256 string) (string, string, error) {
	manifestBody, err := stableReadFile(filepath.Join(root, filepath.FromSlash(domainplugin.ManifestRelativePathV1)), maxManifestBytesV1)
	if err != nil {
		return "", "", err
	}
	manifestDigest := sha256.Sum256(manifestBody)
	if expectedManifestSHA256 == "" || hex.EncodeToString(manifestDigest[:]) != expectedManifestSHA256 {
		return "", "", errors.New("bundled plugin manifest changed during inspection")
	}
	manifestFields, err := domainjsonstrict.DecodeRawObject(manifestBody, domainjsonstrict.Options{
		MaxBytes: int(maxManifestBytesV1), MaxDepth: 16, MaxTokens: 4096, MaxStringBytes: 64 << 10,
	})
	if err != nil {
		return "", "", errors.New("bundled plugin manifest is not strict JSON")
	}
	var name, version string
	if json.Unmarshal(manifestFields["name"], &name) != nil || json.Unmarshal(manifestFields["version"], &version) != nil {
		return "", "", errors.New("bundled plugin manifest name or version is invalid")
	}
	return name, version, nil
}

func validateDisabledMCPV1(
	root string,
	expectedMCPConfigSHA256 string,
	declaredServers []domainpluginpackage.MCPServerContributionV1,
) error {
	mcpBody, err := stableReadFile(filepath.Join(root, ".mcp.json"), maxMCPConfigBytesV1)
	if err != nil {
		return errors.New("bundled plugin MCP configuration is missing")
	}
	mcpDigest := sha256.Sum256(mcpBody)
	if expectedMCPConfigSHA256 == "" || hex.EncodeToString(mcpDigest[:]) != expectedMCPConfigSHA256 {
		return errors.New("bundled plugin MCP configuration changed during inspection")
	}
	mcpTop, err := domainjsonstrict.DecodeRawObject(mcpBody, domainjsonstrict.Options{
		MaxBytes: int(maxMCPConfigBytesV1), MaxDepth: 16, MaxTokens: 8192, MaxStringBytes: 64 << 10,
	})
	if err != nil || len(mcpTop) != 1 {
		return errors.New("bundled plugin MCP configuration is invalid")
	}
	servers, err := domainjsonstrict.DecodeRawObject(mcpTop["mcpServers"], domainjsonstrict.Options{
		MaxBytes: int(maxMCPConfigBytesV1), MaxDepth: 15, MaxTokens: 8192, MaxStringBytes: 64 << 10,
	})
	if err != nil || len(servers) == 0 {
		return errors.New("bundled plugin MCP server set is invalid")
	}
	if len(servers) != len(declaredServers) {
		return errors.New("bundled plugin projection parity is invalid")
	}
	for _, declared := range declaredServers {
		raw, exists := servers[declared.ID]
		if !exists {
			return errors.New("bundled plugin projection parity is invalid")
		}
		fields, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{
			MaxBytes: int(maxMCPConfigBytesV1), MaxDepth: 14, MaxTokens: 4096, MaxStringBytes: 64 << 10,
		})
		if err != nil {
			return errors.New("bundled plugin MCP server is invalid")
		}
		var disabled bool
		if json.Unmarshal(fields["disabled"], &disabled) != nil || !disabled {
			return errors.New("bundled plugin MCP server must remain disabled")
		}
		if enabledRaw, ok := fields["enabled"]; ok {
			var enabled bool
			if json.Unmarshal(enabledRaw, &enabled) != nil || enabled {
				return errors.New("bundled plugin MCP server cannot be enabled by materialization")
			}
		}
		var command, cwd string
		var args []string
		if json.Unmarshal(fields["command"], &command) != nil ||
			json.Unmarshal(fields["cwd"], &cwd) != nil ||
			json.Unmarshal(fields["args"], &args) != nil ||
			normalizedMCPEntrypointV1(command, cwd, args) != declared.Entrypoint {
			return errors.New("bundled plugin projection parity is invalid")
		}
	}
	return nil
}

func normalizedMCPEntrypointV1(command, cwd string, args []string) string {
	if command != "node" || cwd != "." || len(args) != 1 || !strings.HasPrefix(args[0], "./") {
		return ""
	}
	relative := strings.TrimPrefix(args[0], "./")
	if !portablePath(relative) {
		return ""
	}
	return relative
}

func copySourceTree(ctx context.Context, sourceRoot, destinationRoot string) error {
	if err := os.Mkdir(destinationRoot, 0o700); err != nil {
		return err
	}
	directories := []string{destinationRoot}
	err := filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == sourceRoot {
			return nil
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil || !portablePath(filepath.ToSlash(relative)) {
			return errors.New("bundled plugin copy path is invalid")
		}
		target := filepath.Join(destinationRoot, relative)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("bundled plugin copy encountered an unstable path")
		}
		if info.IsDir() {
			if err := os.Mkdir(target, 0o700); err != nil {
				return err
			}
			directories = append(directories, target)
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("bundled plugin copy encountered a non-regular file")
		}
		return copyStableFile(path, target, info)
	})
	if err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectory(directories[index]); err != nil {
			return fmt.Errorf("sync bundled plugin directory: %w", err)
		}
	}
	return nil
}

func copyStableFile(source, destination string, before os.FileInfo) error {
	input, opened, err := openStableFile(source, before)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, maxPluginFileBytesV1+1))
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != opened.Size() || written > maxPluginFileBytesV1 {
		return errors.New("bundled plugin source file changed while copying")
	}
	return validateStableFile(source, input, opened)
}

func hashStableFile(path string, before os.FileInfo) (string, int64, error) {
	file, opened, err := openStableFile(path, before)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, maxPluginFileBytesV1+1))
	if err != nil || size != opened.Size() || size > maxPluginFileBytesV1 {
		return "", 0, errors.New("bundled plugin source file changed while hashing")
	}
	if err := validateStableFile(path, file, opened); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func stableReadFile(path string, maxBytes int64) ([]byte, error) {
	return stableReadFileObserved(path, maxBytes, nil)
}

func stableReadFileObserved(path string, maxBytes int64, afterRead func()) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	file, opened, err := openStableFile(path, before)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if opened.Size() <= 0 || opened.Size() > maxBytes {
		return nil, errors.New("bundled plugin file exceeds its bound")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != opened.Size() {
		return nil, errors.New("bundled plugin file changed while reading")
	}
	if afterRead != nil {
		afterRead()
	}
	if err := validateStableFile(path, file, opened); err != nil {
		return nil, err
	}
	return body, nil
}

func openStableFile(path string, before os.FileInfo) (*os.File, os.FileInfo, error) {
	if before == nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() < 0 || before.Size() > maxPluginFileBytesV1 {
		return nil, nil, errors.New("bundled plugin source file is not a stable regular file")
	}
	file, err := openNoFollow(path)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) {
		file.Close()
		return nil, nil, errors.New("bundled plugin source file changed before opening")
	}
	return file, opened, nil
}

func validateStableFile(path string, file *os.File, opened os.FileInfo) error {
	afterDescriptor, descriptorErr := file.Stat()
	afterPath, pathErr := os.Lstat(path)
	if descriptorErr != nil || pathErr != nil || !afterDescriptor.Mode().IsRegular() || afterPath.Mode()&os.ModeSymlink != 0 ||
		!afterPath.Mode().IsRegular() || !os.SameFile(opened, afterDescriptor) || !os.SameFile(opened, afterPath) || afterDescriptor.Size() != opened.Size() {
		return errors.New("bundled plugin source file changed after reading")
	}
	return nil
}

func canonicalDirectory(root string) (string, error) {
	if root == "" || root != strings.TrimSpace(root) || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", errors.New("bundled plugin directory is not canonical")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil || realRoot != root {
		return "", errors.New("bundled plugin directory must not contain a symbolic link")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("bundled plugin directory is not a real directory")
	}
	return realRoot, nil
}

func portablePath(value string) bool {
	return domainpluginpackage.ValidPackageRelativePathV1(value)
}
