package webfetch

import (
	"context"
	"time"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type ToolInput struct {
	Context       context.Context
	Args          map[string]any
	Config        runtimeinfoapp.WebConfig
	ModelProxyURL string
	OnProgress    func(host string)
}

func ExecuteTool(input ToolInput) (any, bool) {
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now()
	if !runtimeinfoapp.WebFetchEnabled(input.Config) {
		failure := map[string]any{"code": "provider_unavailable", "outcome": "disabled", "error": "web fetch is disabled by config"}
		captureWebToolFailureV1(ctx, input.Args, failure, startedAt)
		return failure, true
	}
	request, failure, failed := ParseRequest(
		input.Args,
		runtimeinfoapp.WebMaxFetchBytes(input.Config),
		runtimeinfoapp.DefaultWebMaxFetchBytes,
		runtimeinfoapp.MinWebFetchBytes,
		time.Duration(runtimeinfoapp.DefaultWebTimeoutMS)*time.Millisecond,
	)
	if failed {
		captureWebToolFailureV1(ctx, input.Args, failure, startedAt)
		return failure, true
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

func captureWebToolFailureV1(ctx context.Context, args map[string]any, failure map[string]any, startedAt time.Time) {
	web := domaintoolresult.ProtectedWebCaptureV1{StartedAt: startedAt.UTC().Format(time.RFC3339Nano), ToolIsError: true}
	setWebCaptureURLV1(&web.RequestURL, &web.RequestURLTruncated, firstNonEmptyString(args["url"]))
	message, clipped := domaintoolresult.BoundProtectedTextV1(firstNonEmptyString(failure["error"]), domaintoolresult.ProtectedWebSnapshotErrorLimitV1)
	web.Error = &domaintoolresult.ProtectedWebErrorV1{Code: firstNonEmptyString(failure["code"]), Message: message}
	domaintoolresult.CaptureProtectedToolResultV1(ctx, domaintoolresult.ProtectedCaptureV1{Kind: "web_fetch", Status: "failed", Body: message, Label: "Fetch", DurationMs: time.Since(startedAt).Milliseconds(), Truncated: clipped || firstNonEmptyString(args["url"]) != "" && web.RequestURL == "" && !web.RequestURLTruncated, Web: &web})
}
