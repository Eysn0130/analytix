package visionbridge

import (
	modelapp "analytix.local/runtime-go/internal/app/model"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type ToolResultProjectionInput struct {
	ToolName                string
	RawOutput               any
	PersistOutput           any
	IsError                 bool
	Config                  runtimeinfoapp.VisionBridgeConfig
	ExactPrivateModelOutput any
	ExactPublicOutput       any
	HasExactPrivateOutput   bool
}

// PrepareToolResultForModel is the closed projection for tool output sent to
// persistence and back to the provider. Raw media and process-private handoff
// capabilities never enter the public projection.
func PrepareToolResultForModel(input ToolResultProjectionInput) (domaintoolresult.PublicToolResultProjectionV1, string) {
	project := func(value any) domaintoolresult.PublicToolResultProjectionV1 {
		return toolcatalogapp.BuildPublicToolResultProjectionV1(input.ToolName, value, input.IsError)
	}
	if input.HasExactPrivateOutput && !input.IsError {
		return project(input.ExactPublicOutput), modelapp.ToolResultContentForModel(input.ExactPrivateModelOutput)
	}
	config := runtimeinfoapp.NormalizeVisionBridgeConfig(input.Config)
	imageExtraction := runtimeinfoapp.ExtractToolResultImagesForConfigWithStats(input.RawOutput, config)
	persistOutput := runtimeinfoapp.RedactToolResultImageDataForPersistence(input.PersistOutput)
	if toolcatalogapp.MCPToolNeedsAnalytixCaseContext(input.ToolName) || input.IsError {
		return project(persistOutput), modelapp.ToolResultContentForModel(persistOutput)
	}
	if toolcatalogapp.IsUserInputTool(input.ToolName) {
		return project(persistOutput), modelapp.ToolResultContentForModel(input.RawOutput)
	}
	if len(imageExtraction.Images) == 0 {
		return project(persistOutput), modelapp.ToolResultContentForModel(persistOutput)
	}
	status := runtimeinfoapp.BuildVisionBridgeToolStatus(
		config, input.ToolName, len(imageExtraction.Images), "unavailable",
		"tool result media requires attempt-local private authority", nil,
	)
	if imageExtraction.OmittedCount > 0 {
		status["omittedCount"] = float64(imageExtraction.OmittedCount)
	}
	enriched := runtimeinfoapp.ToolResultWithVisionBridgeStatus(persistOutput, status)
	return project(enriched), modelapp.ToolResultContentForModel(enriched)
}
