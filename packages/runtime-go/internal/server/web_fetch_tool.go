package server

import (
	"context"

	webfetch "analytix.local/runtime-go/internal/adapters/outbound/webfetch"
)

func (h *runtimeServerHandler) executeWebFetchRuntimeTool(ctx context.Context, pending runtimePendingToolCall, args map[string]any) (any, bool) {
	return webfetch.ExecuteTool(webfetch.ToolInput{
		Context:       ctx,
		Args:          args,
		Config:        h.web,
		ModelProxyURL: h.modelProxyURL,
		OnProgress: func(host string) {
			h.recordRuntimeToolProgress(pending, "running", "fetching "+host)
		},
	})
}
