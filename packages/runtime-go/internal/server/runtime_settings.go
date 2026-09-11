package server

import (
	"strings"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	loopapp "analytix.local/runtime-go/internal/app/loop"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
)

func loadRuntimeSandboxSettings(config RuntimeServerConfig) (runtimeinfoapp.SandboxConfig, error) {
	settings := runtimeinfoapp.SandboxConfig{
		AllowWriteRoots:   append([]string(nil), config.AllowWriteRoots...),
		ProtectedReadDirs: append(filestore.DefaultProtectedReadDirs(), config.ProtectedReadDirs...),
	}
	document, ok, err := runtimeConfigDocument(config)
	if err != nil || !ok {
		settings.AllowWriteRoots = filestore.NormalizeRealRoots(settings.AllowWriteRoots)
		settings.ProtectedReadDirs = filestore.NormalizeRealRoots(settings.ProtectedReadDirs)
		return settings, err
	}
	settings = runtimeinfoapp.LoadSandboxConfig(document, settings)
	settings.AllowWriteRoots = filestore.NormalizeRealRoots(settings.AllowWriteRoots)
	settings.ProtectedReadDirs = filestore.NormalizeRealRoots(settings.ProtectedReadDirs)
	return settings, nil
}

func loadRuntimeWebConfig(config RuntimeServerConfig) (runtimeinfoapp.WebConfig, error) {
	settings := runtimeinfoapp.DefaultWebConfig()
	document, ok, err := runtimeConfigDocument(config)
	if err != nil || !ok {
		return settings, err
	}
	return runtimeinfoapp.LoadWebConfig(document), nil
}

func loadRuntimeVisionBridgeConfig(config RuntimeServerConfig) (runtimeVisionBridgeConfig, error) {
	settings := runtimeinfoapp.DefaultVisionBridgeConfig()
	document, ok, err := runtimeConfigDocument(config)
	if err != nil || !ok {
		return settings, err
	}
	return runtimeinfoapp.LoadVisionBridgeConfig(document), nil
}

func loadRuntimeStepLimitConfig(config RuntimeServerConfig) (loopapp.StepLimitConfig, error) {
	var settings loopapp.StepLimitConfig
	document, ok, err := runtimeConfigDocument(config)
	if err != nil || !ok {
		return settings, err
	}
	return loopapp.LoadStepLimitConfig(document), nil
}

func (h *runtimeServerHandler) resolveRuntimeModelStepLimit(thread map[string]any, request startRuntimeTurnRequest) int {
	mode := request.Mode
	if strings.TrimSpace(mode) == "" && thread != nil {
		mode = normalizeTurnMode(stringField(thread, "mode"))
	}
	return loopapp.ResolveModelStepLimit(
		h.stepLimits,
		mode,
		request.DisableUserInput,
		loopapp.OptionalThreadMaxModelSteps(thread),
		request.MaxModelSteps,
	)
}

func runtimeConfigDocument(config RuntimeServerConfig) (map[string]any, bool, error) {
	return filestore.RuntimeConfigDocument(config.MCPConfigJSON, config.MCPConfigPath)
}
