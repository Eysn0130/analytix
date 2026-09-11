package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
)

const (
	installedPluginManifestSource = "installed-plugin-manifest"

	// HostInstalledGenerationSourceV1 identifies a complete host-materialized
	// generation, including its host installation marker. Ordinary MCP JSON
	// intentionally cannot select this source; the runtime composition layer
	// uses it only after validating the signed materialization receipt/index.
	HostInstalledGenerationSourceV1 = "host-installed-generation-v1"
)

const maxInstalledPluginManifestBytes = 256 * 1024

func IsHighRiskServerID(serverID string) bool {
	normalized := strings.ToLower(strings.TrimSpace(serverID))
	return normalized == "analytix_funds" || normalized == "analytix-fund-analysis"
}

func VerifyConfiguredProvenance(spec domainmcp.ServerSpec) error {
	if !IsHighRiskServerID(spec.ID) {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(spec.Transport)) != "stdio" {
		return errors.New("high-risk funds MCP requires a pinned local stdio entrypoint")
	}
	identitySource := strings.TrimSpace(spec.IdentitySource)
	if identitySource != installedPluginManifestSource &&
		identitySource != HostInstalledGenerationSourceV1 {
		return errors.New("high-risk funds MCP is missing installed-plugin identity provenance")
	}
	if strings.TrimSpace(spec.ExpectedServerName) == "" || strings.TrimSpace(spec.ExpectedServerVersion) == "" {
		return errors.New("high-risk funds MCP is missing expected server identity")
	}
	if !validSHA256(spec.ManifestSHA256) || !validSHA256(spec.EntrypointSHA256) {
		return errors.New("high-risk funds MCP is missing manifest or entrypoint integrity proof")
	}
	if strings.TrimSpace(spec.PluginRootPath) == "" || !validSHA256(spec.SourceTreeSHA256) {
		return errors.New("high-risk funds MCP is missing complete source-tree integrity proof")
	}
	if identitySource == HostInstalledGenerationSourceV1 &&
		(spec.HostInstallMarkerSHA256 != strings.ToLower(strings.TrimSpace(spec.HostInstallMarkerSHA256)) ||
			!validSHA256(spec.HostInstallMarkerSHA256)) {
		return errors.New("high-risk funds MCP is missing host installation marker integrity proof")
	}
	pluginRoot, err := canonicalSourceTreeRoot(spec.PluginRootPath)
	if err != nil {
		return errors.New("high-risk funds MCP plugin root is invalid")
	}
	beforeTreeDigest, err := configuredSourceTreeDigestV1(pluginRoot, identitySource)
	if err != nil || !strings.EqualFold(beforeTreeDigest, strings.TrimSpace(spec.SourceTreeSHA256)) {
		return errors.New("high-risk funds MCP source-tree integrity mismatch")
	}
	if identitySource == HostInstalledGenerationSourceV1 &&
		verifyConfiguredHostInstallMarkerV1(pluginRoot, spec) != nil {
		return errors.New("high-risk funds MCP host installation marker is invalid")
	}
	manifestBody, err := readPinnedRegularFile(
		filepath.Join(pluginRoot, ".codex-plugin", "plugin.json"),
		pluginRoot,
		maxInstalledPluginManifestBytes,
	)
	if err != nil {
		return errors.New("high-risk funds MCP manifest is invalid")
	}
	manifestDigest := sha256.Sum256(manifestBody)
	if !strings.EqualFold(hex.EncodeToString(manifestDigest[:]), strings.TrimSpace(spec.ManifestSHA256)) {
		return errors.New("high-risk funds MCP manifest integrity mismatch")
	}
	if err := verifyFundsPluginManifest(manifestBody, spec.ExpectedServerVersion); err != nil {
		return err
	}
	entrypoint := spec.EntrypointPath
	if entrypoint == "" || entrypoint != strings.TrimSpace(entrypoint) {
		return errors.New("high-risk funds MCP is missing pinned entrypoint path")
	}
	if !configuredEntrypointBound(spec.Command, spec.Args, entrypoint) {
		return errors.New("high-risk funds MCP command is not bound to the pinned entrypoint")
	}
	body, err := readPinnedRegularFile(entrypoint, pluginRoot, 0)
	if err != nil {
		return fmt.Errorf("read high-risk funds MCP entrypoint: %w", err)
	}
	digest := sha256.Sum256(body)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), strings.TrimSpace(spec.EntrypointSHA256)) {
		return errors.New("high-risk funds MCP entrypoint integrity mismatch")
	}
	afterTreeDigest, err := configuredSourceTreeDigestV1(pluginRoot, identitySource)
	if err != nil || afterTreeDigest != beforeTreeDigest || !strings.EqualFold(afterTreeDigest, strings.TrimSpace(spec.SourceTreeSHA256)) {
		return errors.New("high-risk funds MCP source-tree integrity mismatch")
	}
	if identitySource == HostInstalledGenerationSourceV1 &&
		verifyConfiguredHostInstallMarkerV1(pluginRoot, spec) != nil {
		return errors.New("high-risk funds MCP host installation marker is invalid")
	}
	return nil
}

func configuredSourceTreeDigestV1(pluginRoot, identitySource string) (string, error) {
	if identitySource == HostInstalledGenerationSourceV1 {
		return computeSourceTreeSHA256(pluginRoot, domainplugin.InstallMarkerFileNameV1)
	}
	return ComputeSourceTreeSHA256(pluginRoot)
}

func verifyConfiguredHostInstallMarkerV1(pluginRoot string, spec domainmcp.ServerSpec) error {
	body, err := readPinnedRegularFile(
		filepath.Join(pluginRoot, domainplugin.InstallMarkerFileNameV1),
		pluginRoot,
		domainplugin.MaxContractBytesV1,
	)
	if err != nil {
		return err
	}
	defer clear(body)
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != spec.HostInstallMarkerSHA256 {
		return errors.New("host installation marker integrity mismatch")
	}
	return verifyHostInstallMarkerV1(body, spec.ExpectedServerVersion, spec.SourceTreeSHA256)
}

func verifyHostInstallMarkerV1(body []byte, expectedVersion, expectedSourceTreeSHA256 string) error {
	type markerV1 struct {
		ManagedBy           string `json:"managedBy"`
		MarketplaceName     string `json:"marketplaceName"`
		PluginName          string `json:"pluginName"`
		Version             string `json:"version"`
		PackageSHA256       string `json:"packageSha256"`
		SourcePath          string `json:"sourcePath"`
		SourceTreeSHA256    string `json:"sourceTreeSha256"`
		SourceTreeFileCount uint64 `json:"sourceTreeFileCount"`
		InstallType         string `json:"installType"`
	}
	var marker markerV1
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&marker) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		marker.ManagedBy != "analytix-hub" || marker.MarketplaceName != "analytix-hub" ||
		marker.PluginName != domainplugin.PluginNameV1 ||
		marker.Version != strings.TrimSpace(expectedVersion) ||
		marker.PackageSHA256 != strings.ToLower(strings.TrimSpace(marker.PackageSHA256)) ||
		!validSHA256(marker.PackageSHA256) ||
		marker.SourcePath == "" || marker.SourcePath != strings.TrimSpace(marker.SourcePath) ||
		!filepath.IsAbs(marker.SourcePath) || filepath.Clean(marker.SourcePath) != marker.SourcePath ||
		marker.SourceTreeSHA256 != strings.ToLower(strings.TrimSpace(marker.SourceTreeSHA256)) ||
		!validSHA256(marker.SourceTreeSHA256) ||
		marker.SourceTreeSHA256 != expectedSourceTreeSHA256 ||
		marker.SourceTreeFileCount == 0 ||
		marker.SourceTreeFileCount > domainplugin.MaxSourceTreeFilesV1 ||
		marker.InstallType != "user" {
		return errors.New("host installation marker contract is invalid")
	}
	canonical, err := json.Marshal(marker)
	if err != nil || !bytes.Equal(canonical, body) {
		return errors.New("host installation marker is not canonical")
	}
	return nil
}

func verifyFundsPluginManifest(body []byte, expectedVersion string) error {
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		MaxBytes:       maxInstalledPluginManifestBytes,
		MaxDepth:       16,
		MaxTokens:      4096,
		MaxStringBytes: 64 * 1024,
	})
	if err != nil {
		return errors.New("high-risk funds MCP manifest JSON is invalid")
	}
	var name string
	var version string
	if err := json.Unmarshal(fields["name"], &name); err != nil || name != "analytix-fund-analysis" {
		return errors.New("high-risk funds MCP manifest name is invalid")
	}
	if err := json.Unmarshal(fields["version"], &version); err != nil || version == "" || version != strings.TrimSpace(version) || version != expectedVersion {
		return errors.New("high-risk funds MCP manifest version is invalid")
	}
	return nil
}

func readPinnedRegularFile(path string, pluginRoot string, maxBytes int64) ([]byte, error) {
	if path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) {
		return nil, errors.New("pinned plugin file path is invalid")
	}
	absolute := filepath.Clean(path)
	if absolute != path {
		return nil, errors.New("pinned plugin file path is not canonical")
	}
	realPath, err := filepath.EvalSymlinks(absolute)
	if err != nil || realPath != absolute {
		return nil, errors.New("pinned plugin file must not be a symbolic link")
	}
	relative, err := filepath.Rel(pluginRoot, realPath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, errors.New("pinned plugin file is outside the plugin root")
	}
	beforePathInfo, err := os.Lstat(realPath)
	if err != nil || !beforePathInfo.Mode().IsRegular() || beforePathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("pinned plugin file must be regular")
	}
	file, err := os.Open(realPath)
	if err != nil {
		return nil, errors.New("open pinned plugin file")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(beforePathInfo, openedInfo) {
		return nil, errors.New("pinned plugin file changed before reading")
	}
	if maxBytes > 0 && openedInfo.Size() > maxBytes {
		return nil, errors.New("pinned plugin file exceeds size limit")
	}
	var body []byte
	if maxBytes > 0 {
		body, err = io.ReadAll(io.LimitReader(file, maxBytes+1))
	} else {
		body, err = io.ReadAll(file)
	}
	if err != nil || int64(len(body)) != openedInfo.Size() || (maxBytes > 0 && int64(len(body)) > maxBytes) {
		return nil, errors.New("pinned plugin file changed while reading")
	}
	afterOpenedInfo, descriptorErr := file.Stat()
	afterPathInfo, pathErr := os.Lstat(realPath)
	if descriptorErr != nil || pathErr != nil || !os.SameFile(openedInfo, afterOpenedInfo) || !os.SameFile(openedInfo, afterPathInfo) {
		return nil, errors.New("pinned plugin file changed after reading")
	}
	return body, nil
}

func configuredEntrypointBound(command string, args []string, entrypoint string) bool {
	command = strings.TrimSpace(command)
	entrypoint = strings.TrimSpace(entrypoint)
	if command == "" || entrypoint == "" {
		return false
	}
	if command == entrypoint {
		return true
	}
	for _, arg := range args {
		candidate := strings.TrimSpace(arg)
		if candidate == "" || strings.HasPrefix(candidate, "-") {
			continue
		}
		return candidate == entrypoint
	}
	return false
}

func VerifyObserved(spec domainmcp.ServerSpec, observed domainmcp.ServerIdentity) error {
	expectedName := strings.TrimSpace(spec.ExpectedServerName)
	expectedVersion := strings.TrimSpace(spec.ExpectedServerVersion)
	if IsHighRiskServerID(spec.ID) && (expectedName == "" || expectedVersion == "") {
		return errors.New("high-risk funds MCP observed identity cannot be verified without a pin")
	}
	if expectedName != "" && observed.Name != expectedName {
		return fmt.Errorf("mcp server identity mismatch: observed %q expected %q", observed.Name, expectedName)
	}
	if expectedVersion != "" && observed.Version != expectedVersion {
		return fmt.Errorf("mcp server version mismatch: observed %q expected %q", observed.Version, expectedVersion)
	}
	return nil
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
