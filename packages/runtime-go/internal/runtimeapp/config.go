package runtimeapp

import (
	"errors"
	"math"
	"path/filepath"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	loopapp "analytix.local/runtime-go/internal/app/loop"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	mcp "analytix.local/runtime-go/internal/mcp"
)

func loadRuntimeConfigDocument(config Config) (map[string]any, bool, error) {
	return filestore.RuntimeConfigDocument(config.MCPConfigJSON, config.MCPConfigPath)
}

func loadRuntimeConfigurationSnapshot(config Config) (filestore.RuntimeConfigSnapshotV1, error) {
	return filestore.LoadRuntimeConfigSnapshotV1(config.MCPConfigJSON, config.MCPConfigPath)
}

func loadRuntimeMCPServerSpecs(snapshot filestore.RuntimeConfigSnapshotV1, workspaceRoot string) ([]mcp.ServerSpec, error) {
	if !snapshot.Present() {
		return nil, nil
	}
	return mcp.LoadMCPJSONDocument(snapshot.Bytes(), workspaceRoot)
}

func loadRuntimeConfigDocumentFromSnapshot(snapshot filestore.RuntimeConfigSnapshotV1) (map[string]any, bool, error) {
	return snapshot.Document()
}

func loadRuntimeConfiguration(config Config) (filestore.RuntimeConfigSnapshotV1, []mcp.ServerSpec, map[string]any, bool, error) {
	snapshot, err := loadRuntimeConfigurationSnapshot(config)
	if err != nil {
		return filestore.RuntimeConfigSnapshotV1{}, nil, nil, false, err
	}
	specs, err := loadRuntimeMCPServerSpecs(snapshot, config.DataDir)
	if err != nil {
		return filestore.RuntimeConfigSnapshotV1{}, nil, nil, false, err
	}
	document, ok, err := loadRuntimeConfigDocumentFromSnapshot(snapshot)
	if err != nil {
		return filestore.RuntimeConfigSnapshotV1{}, nil, nil, false, err
	}
	return snapshot, specs, document, ok, nil
}

func loadRuntimeMCPSearchSettings(document map[string]any, ok bool) runtimeinfoapp.MCPSearchConfig {
	return runtimeinfoapp.LoadMCPSearchConfigFromDocument(document, ok)
}

func loadRuntimeSkillCatalog(config Config, document map[string]any, ok bool) (toolcatalogapp.SkillCatalog, error) {
	return toolcatalogapp.LoadSkillCatalogFromDocument(
		document,
		ok,
		config.DataDir,
		toolcatalogapp.SkillCatalogFileSource{
			NormalizeRoot:     filestore.NormalizeSkillRoot,
			RootExists:        filestore.SkillRootDirectoryExists,
			PackageCandidates: filestore.SkillPackageCandidates,
			LoadPackage:       filestore.LoadSkillPackage,
		},
	)
}

// loadRuntimeUserInputConfig distinguishes an absent opt-in from an explicit
// policy. Defaults never mutate the frozen source document or its digest.
func loadRuntimeUserInputConfig(document map[string]any, ok bool) (toolcatalogapp.UserInputConfig, bool, error) {
	config := toolcatalogapp.UserInputConfig{}
	runtimeConfig, _ := document["runtime"].(map[string]any)
	if !ok || runtimeConfig == nil {
		normalized, err := toolcatalogapp.NormalizeUserInputConfig(config)
		return normalized, false, err
	}
	raw, present := runtimeConfig["userInput"]
	if !present {
		normalized, err := toolcatalogapp.NormalizeUserInputConfig(config)
		return normalized, false, err
	}
	leaf, object := raw.(map[string]any)
	if !object || leaf == nil {
		return config, true, errors.New("runtime.userInput must be an object")
	}
	for key := range leaf {
		if key != "policy" && key != "defaultTimeoutSeconds" {
			return config, true, errors.New("runtime.userInput contains an unsupported field")
		}
	}
	policy, valid := leaf["policy"].(string)
	if !valid || (policy != "legacy" && policy != "timed") {
		return config, true, errors.New("runtime.userInput.policy must be legacy or timed")
	}
	config.Mode = policy
	if rawTimeout, exists := leaf["defaultTimeoutSeconds"]; exists && rawTimeout != nil {
		seconds, number := rawTimeout.(float64)
		if !number || math.IsNaN(seconds) || math.IsInf(seconds, 0) || math.Trunc(seconds) != seconds || seconds < -1 || seconds == 0 || seconds > 2147483 {
			return config, true, errors.New("runtime.userInput.defaultTimeoutSeconds must be null, -1, or an integer from 1 to 2147483")
		}
		value := int(seconds)
		config.TimeoutSeconds = &value
	}
	normalized, err := toolcatalogapp.NormalizeUserInputConfig(config)
	return normalized, true, err
}

// Keep the original input sequence byte-for-byte when the opt-in leaf is absent.
func runtimeUserInputConfigurationDigest(config toolcatalogapp.UserInputConfig, present bool, legacyValues ...any) (string, error) {
	if present {
		legacyValues = append(legacyValues, config)
	}
	return runtimeStartupConfigurationDigest(legacyValues...)
}

func loadRuntimeSubagentConfig(document map[string]any, ok bool) (subagentapp.ProfileSettings, error) {
	return subagentapp.LoadProfileSettings(document, ok)
}

func loadRuntimeSandboxSettings(config Config, document map[string]any, ok bool) runtimeinfoapp.SandboxConfig {
	settings := runtimeinfoapp.SandboxConfig{
		AllowWriteRoots:   append([]string(nil), config.AllowWriteRoots...),
		ProtectedReadDirs: append(filestore.DefaultProtectedReadDirs(), config.ProtectedReadDirs...),
	}
	if ok {
		settings = runtimeinfoapp.LoadSandboxConfig(document, settings)
	}
	settings.AllowWriteRoots = filestore.NormalizeRealRoots(settings.AllowWriteRoots)
	settings.ProtectedReadDirs = filestore.NormalizeRealRoots(settings.ProtectedReadDirs)
	mandatoryRoots := []string{
		config.DevelopmentProviderAuthorityDir,
		filepath.Join(config.DataDir, "object-editing"),
		filepath.Join(config.DataDir, "private"),
		filepath.Join(config.DataDir, "child-runs"),
		config.UserDataDir,
		config.AuthorityManifestRoot,
		config.AuthorityCredentialProfileRoot,
		config.AuthorityCredentialBundleRoot,
	}
	for _, root := range filestore.NormalizeRealRoots(mandatoryRoots) {
		settings.ProtectedReadDirs = append(
			settings.ProtectedReadDirs,
			filestore.MandatoryProtectedRoot(root),
		)
	}
	return settings
}

func loadRuntimeWebConfig(document map[string]any, ok bool) runtimeinfoapp.WebConfig {
	if !ok {
		return runtimeinfoapp.DefaultWebConfig()
	}
	return runtimeinfoapp.LoadWebConfig(document)
}

func loadRuntimeVisionBridgeConfig(document map[string]any, ok bool) runtimeinfoapp.VisionBridgeConfig {
	if !ok {
		return runtimeinfoapp.DefaultVisionBridgeConfig()
	}
	return runtimeinfoapp.LoadVisionBridgeConfig(document)
}

func loadRuntimeStepLimitConfig(document map[string]any, ok bool) loopapp.StepLimitConfig {
	if !ok {
		return loopapp.StepLimitConfig{}
	}
	return loopapp.LoadStepLimitConfig(document)
}

func loadRuntimeStreamIdleTimeout(document map[string]any, ok bool) (time.Duration, bool) {
	if !ok {
		return 0, false
	}
	runtimeConfig, _ := document["runtime"].(map[string]any)
	if runtimeConfig == nil {
		return 0, false
	}
	var milliseconds float64
	switch value := runtimeConfig["streamIdleTimeoutMs"].(type) {
	case float64:
		milliseconds = value
	case float32:
		milliseconds = float64(value)
	case int:
		milliseconds = float64(value)
	case int64:
		milliseconds = float64(value)
	default:
		return 0, false
	}
	if milliseconds < 0 {
		return 0, false
	}
	return time.Duration(milliseconds * float64(time.Millisecond)), true
}
