package webfetch

import (
	"context"
	"time"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
)

type ToolInput struct {
	Context       context.Context
	Args          map[string]any
	Config        runtimeinfoapp.WebConfig
	ModelProxyURL string
	OnProgress    func(host string)
}

func ExecuteTool(input ToolInput) (any, bool) {
	if !runtimeinfoapp.WebFetchEnabled(input.Config) {
		return map[string]any{"code": "provider_unavailable", "error": "web fetch is disabled by config"}, true
	}
	request, failure, failed := ParseRequest(
		input.Args,
		runtimeinfoapp.WebMaxFetchBytes(input.Config),
		runtimeinfoapp.DefaultWebMaxFetchBytes,
		runtimeinfoapp.MinWebFetchBytes,
		time.Duration(runtimeinfoapp.DefaultWebTimeoutMS)*time.Millisecond,
	)
	if failed {
		return failure, true
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return Client{
		Policy:          Policy{AllowDomains: input.Config.AllowDomains, DenyDomains: input.Config.DenyDomains},
		ProxySpec:       ProxySpecForURL(input.ModelProxyURL),
		ProviderID:      runtimeinfoapp.WebProviderID(input.Config),
		DefaultMaxBytes: runtimeinfoapp.DefaultWebMaxFetchBytes,
		DefaultTimeout:  time.Duration(runtimeinfoapp.DefaultWebTimeoutMS) * time.Millisecond,
		OnProgress:      input.OnProgress,
	}.Fetch(ctx, request)
}
