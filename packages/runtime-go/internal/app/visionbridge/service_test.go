package visionbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type providerStub struct {
	requests []domainmodel.Request
	result   domainmodel.Result
	err      error
	inspect  func(domainmodel.Request) error
}

func (stub *providerStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	stub.requests = append(stub.requests, request)
	if stub.inspect != nil {
		if err := stub.inspect(request); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return stub.result, stub.err
}

type executionResolverStub struct {
	config       domainmodel.TurnConfig
	validate     func(context.Context) error
	resolveCalls int
	clearCalls   int
}

func (stub *executionResolverStub) ResolveVisionExecution(_ context.Context) (ExecutionLease, error) {
	stub.resolveCalls++
	config := stub.config
	if strings.TrimSpace(config.ProviderID) == "" {
		config = domainmodel.TurnConfig{
			ProviderID: "bridge", BaseURL: "https://bridge.example.test/v1", APIKey: "bridge-key",
			Model: "vision-model", EndpointFormat: "chat_completions", SupportsImageInput: true,
			InputModalities: []string{"text", "image"}, MessageParts: []string{"text", "image_url"},
		}
	}
	validate := stub.validate
	if validate == nil {
		validate = func(context.Context) error { return nil }
	}
	return NewExecutionLease(config, validate, func() { stub.clearCalls++ }), nil
}

type hostImageProjectorStub struct {
	projected hostProjectedImage
	err       error
	calls     int
}

func (stub *hostImageProjectorStub) ProjectImageForProvider(_ context.Context, _ runtimeinfoapp.ToolResultImage) (hostProjectedImage, error) {
	stub.calls++
	return stub.projected, stub.err
}

func readyConfig() runtimeinfoapp.VisionBridgeConfig {
	return runtimeinfoapp.VisionBridgeConfig{
		Enabled: true, Mode: "auto", ProviderID: "bridge", BaseURL: "https://bridge.example.test/v1",
		APIKey: "bridge-key", Model: "vision-model", EndpointFormat: "chat_completions",
		SemanticProbeStatus: "supported", FallbackWhenPrimaryImageUnsupported: true,
		MaxImageBytes: 1024, MaxScreenshotsPerTurn: 2,
	}
}

func successfulResult() domainmodel.Result {
	return domainmodel.Result{Chunks: []domainmodel.Chunk{{
		Kind: domainmodel.ChunkText,
		Text: `{"source_kind":"user_attachment","summary":"visible text","visible_text":["AX-731"],"uncertainties":[]}`,
	}}, StreamCompleted: true}
}

func TestObserveRequiresAuthorizationAndTrustedProjectionBeforeProviderStream(t *testing.T) {
	provider := &providerStub{result: successfulResult()}
	execution := &executionResolverStub{}
	service := Service{
		Provider: provider, ExecutionResolver: execution, DefaultConfig: readyConfig(),
	}
	input := runtimeinfoapp.VisionBridgeObserveInput{
		Config: readyConfig(), SourceKind: runtimeinfoapp.VisionBridgeSourceToolScreenshot,
		SourceName: "computer state", Images: []runtimeinfoapp.ToolResultImage{
			{MediaType: "image/png", DataBase64: "c2VjcmV0"},
			{MediaType: "image/png", DataBase64: "c2VjcmV0LTI="},
		},
	}

	if _, err := service.Observe(context.Background(), input, visionTelemetryBindingV1(t), nil); !IsAuthorizationError(err) || !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("missing authorization must fail closed, got %v", err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider ran without authorization: %#v", provider.requests)
	}

	authorityErr := errors.New("stale turn authority")
	if _, err := service.Observe(context.Background(), input, visionTelemetryBindingV1(t), func() error { return authorityErr }); !IsAuthorizationError(err) || !errors.Is(err, authorityErr) {
		t.Fatalf("authority failure must remain distinguishable, got %v", err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider ran after failed authorization: %#v", provider.requests)
	}
	if _, err := service.Observe(context.Background(), input, nil, func() error { return nil }); !IsAuthorizationError(err) {
		t.Fatalf("missing provider telemetry authority must fail closed, got %v", err)
	}

	authorizationCount := 0
	_, err := service.Observe(context.Background(), input, visionTelemetryBindingV1(t), func() error {
		authorizationCount++
		return nil
	})
	if !IsImageEffectUnavailable(err) || authorizationCount != 1 {
		t.Fatalf("missing trusted projector did not fail closed after authority: authorizations=%d err=%v", authorizationCount, err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("missing trusted projector reached provider: %#v", provider.requests)
	}
}

func TestObserveFinalProjectionPreventsPhysicalSendAndRetry(t *testing.T) {
	authorizationCount := 0
	provider := &providerStub{result: successfulResult(), inspect: func(request domainmodel.Request) error {
		if request.BeforeSend == nil {
			return errors.New("vision request omitted physical-send authority")
		}
		return request.BeforeSend(1)
	}}
	projector := &hostImageProjectorStub{projected: hostProjectedImage{part: domainmodel.MessagePart{
		Type: "image", MediaType: "image/png", Data: "RAW_PROJECTOR_SENTINEL_731",
	}}}
	execution := &executionResolverStub{}
	service := Service{Provider: provider, ExecutionResolver: execution, DefaultConfig: readyConfig(), projector: projector}
	_, err := service.Observe(context.Background(), runtimeinfoapp.VisionBridgeObserveInput{
		Config: readyConfig(), SourceKind: runtimeinfoapp.VisionBridgeSourceToolScreenshot,
		SourceName: "computer state", Images: []runtimeinfoapp.ToolResultImage{{MediaType: "image/png", DataBase64: "c2VjcmV0"}},
	}, visionTelemetryBindingV1(t), func() error {
		authorizationCount++
		return nil
	})
	if !IsImageEffectUnavailable(err) || !errors.Is(err, privacyprojectionapp.ErrUninspectableProviderPart) {
		t.Fatalf("uninspectable projector output did not fail at final projection: %v", err)
	}
	if authorizationCount != 1 || projector.calls != 1 || len(provider.requests) != 0 {
		t.Fatalf("final projection admitted a physical send/retry: authorizations=%d projector=%d requests=%d", authorizationCount, projector.calls, len(provider.requests))
	}
	if execution.resolveCalls != 1 || execution.clearCalls != 1 {
		t.Fatalf("vision execution lease was not bounded: resolves=%d clears=%d", execution.resolveCalls, execution.clearCalls)
	}
	if strings.Contains(err.Error(), "RAW_PROJECTOR_SENTINEL_731") {
		t.Fatalf("image-effect error reflected projector output: %v", err)
	}
}

func TestObserveQuarantinesProjectorFailureAndUninspectableOutputMatrix(t *testing.T) {
	const rawSentinel = "RAW_IMAGE_SOURCE_SENTINEL_731"
	tests := []struct {
		name              string
		projector         hostImageProjector
		wantUninspectable bool
	}{
		{name: "missing projector"},
		{name: "projector rejection", projector: &hostImageProjectorStub{err: errors.New("rejected " + rawSentinel)}},
		{name: "projector failure", projector: &hostImageProjectorStub{err: errors.New("failed " + rawSentinel)}},
		{name: "empty projector output", projector: &hostImageProjectorStub{}},
		{name: "inline data", projector: &hostImageProjectorStub{projected: hostProjectedImage{part: domainmodel.MessagePart{
			Type: "image", MediaType: "image/png", Data: rawSentinel,
		}}}, wantUninspectable: true},
		{name: "image URL", projector: &hostImageProjectorStub{projected: hostProjectedImage{part: domainmodel.MessagePart{
			Type: "image", MediaType: "image/png", ImageURL: "https://private.example.test/" + rawSentinel,
		}}}, wantUninspectable: true},
		{name: "data URL", projector: &hostImageProjectorStub{projected: hostProjectedImage{part: domainmodel.MessagePart{
			Type: "image", MediaType: "image/png", ImageURL: "data:image/png;base64," + rawSentinel,
		}}}, wantUninspectable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &providerStub{result: successfulResult()}
			execution := &executionResolverStub{}
			service := Service{
				Provider: provider, ExecutionResolver: execution, DefaultConfig: readyConfig(), projector: test.projector,
			}
			_, err := service.Observe(context.Background(), runtimeinfoapp.VisionBridgeObserveInput{
				Config: readyConfig(), SourceKind: runtimeinfoapp.VisionBridgeSourceToolScreenshot,
				SourceName: "PRIVATE_SOURCE_NAME_731", Images: []runtimeinfoapp.ToolResultImage{{
					MediaType: "image/png", DataBase64: rawSentinel,
				}},
			}, visionTelemetryBindingV1(t), func() error { return nil })
			if !IsImageEffectUnavailable(err) || err.Error() != imageEffectUnavailableReason {
				t.Fatalf("image effect did not return the fixed unavailable result: %v", err)
			}
			if test.wantUninspectable != errors.Is(err, privacyprojectionapp.ErrUninspectableProviderPart) {
				t.Fatalf("final projection classification mismatch: uninspectable=%v err=%v", test.wantUninspectable, err)
			}
			if len(provider.requests) != 0 {
				t.Fatalf("unsafe image effect reached provider: %#v", provider.requests)
			}
			if strings.Contains(err.Error(), rawSentinel) || strings.Contains(err.Error(), "PRIVATE_SOURCE_NAME_731") {
				t.Fatalf("image-effect unavailable reason reflected private source: %v", err)
			}
		})
	}
}

func TestNormalizeObservationRejectsOpenSchemaAndRawImageEcho(t *testing.T) {
	for name, observation := range map[string]map[string]any{
		"unknown reasoning": {"summary": "ok", "reasoning": "private chain"},
		"wrong field type":  {"summary": "ok", "visible_text": "not-a-list"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeObservation(runtimeinfoapp.VisionBridgeSourceToolScreenshot, observation); err == nil {
				t.Fatalf("unsafe vision observation was accepted: %#v", observation)
			}
		})
	}
	if !observationContainsExactString(map[string]any{"summary": "c2VjcmV0LWltYWdl"}, "c2VjcmV0LWltYWdl") {
		t.Fatal("raw image echo was not detected")
	}
}

func TestPrepareAttachmentsQuarantinesImageWhenProjectionUnavailable(t *testing.T) {
	const imageBase64 = "c2VjcmV0LWltYWdl"
	attachments := appturn.ResolvedAttachments{
		IDs: []string{"att_1"},
		Parts: []appturn.ResolvedAttachmentPart{{
			ID: "att_1", MIMEType: "image/png", Source: "text_fallback", IsImageFallback: true,
			MessagePart: domainmodel.MessagePart{Type: "text", Text: "Base64: " + imageBase64},
		}},
		MessageParts: []domainmodel.MessagePart{{Type: "text", Text: "Base64: " + imageBase64}},
		ImageCandidates: []appturn.ResolvedAttachmentImage{{
			ID: "att_1", Name: "PRIVATE_SOURCE_NAME_731.png", MIMEType: "image/png", DataBase64: imageBase64,
		}},
		TextFallbackCount: 1,
	}
	provider := &providerStub{result: successfulResult()}
	service := Service{Provider: provider, ExecutionResolver: &executionResolverStub{}, DefaultConfig: readyConfig()}

	prepared, err := service.PrepareAttachments(context.Background(), PrepareAttachmentsInput{
		Attachments:       attachments,
		Primary:           domainmodel.TurnConfig{SupportsImageInput: false, InputModalities: []string{"text"}, MessageParts: []string{"text"}},
		PrimaryProviderID: "primary", PrimaryModel: "text-model", ProviderTelemetry: visionTelemetryBindingV1(t), Authorize: func() error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.VisionBridgeStatus != "unavailable" || prepared.VisionBridgeUsed || prepared.VisionBridgeObservedCount != 0 ||
		prepared.VisionBridgeReason != imageEffectUnavailableReason {
		t.Fatalf("unavailable attachment projection mismatch: %#v", prepared.PipelineDetails())
	}
	assertNoAttachmentBase64(t, prepared, imageBase64)
	providerPartsJSON, _ := json.Marshal(prepared.MessageParts)
	if strings.Contains(string(providerPartsJSON), "PRIVATE_SOURCE_NAME_731") {
		t.Fatalf("unavailable provider projection reflected private source name: %s", providerPartsJSON)
	}
	if len(prepared.ImageCandidates) != 1 || prepared.ImageCandidates[0].Name != "PRIVATE_SOURCE_NAME_731.png" {
		t.Fatalf("host-private attachment metadata was not retained: %#v", prepared.ImageCandidates)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("unavailable image projection reached bridge provider: %#v", provider.requests)
	}

	provider.requests = nil
	authorityErr := errors.New("authority changed")
	blocked, err := service.PrepareAttachments(context.Background(), PrepareAttachmentsInput{
		Attachments:       attachments,
		Primary:           domainmodel.TurnConfig{SupportsImageInput: false, InputModalities: []string{"text"}, MessageParts: []string{"text"}},
		PrimaryProviderID: "primary", PrimaryModel: "text-model", ProviderTelemetry: visionTelemetryBindingV1(t), Authorize: func() error { return authorityErr },
	})
	if !IsAuthorizationError(err) || !errors.Is(err, authorityErr) {
		t.Fatalf("authorization failure mismatch: %v", err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("blocked attachment invoked provider: %#v", provider.requests)
	}
	if blocked.VisionBridgeStatus != "unavailable" || blocked.VisionBridgeReason != "current turn authority is unavailable" {
		t.Fatalf("blocked attachment projection mismatch: %#v", blocked.PipelineDetails())
	}
	assertNoAttachmentBase64(t, blocked, imageBase64)
}

func visionTelemetryBindingV1(t *testing.T) *domainmodel.ProviderTelemetryBindingV1 {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-vision", TurnID: "turn-vision", WorkspaceRealPath: "/workspace",
		CaseID: "case-vision", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("vision-snapshot"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest")), ContextEpoch: 2,
		IssuedAt: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &domainmodel.ProviderTelemetryBindingV1{
		SecurityContext: securityContext, UsageSource: domaincache.ProviderUsageSourceTurn,
		Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 1, OuterAttempt: 1,
	}
}

func assertNoAttachmentBase64(t *testing.T, attachments appturn.ResolvedAttachments, raw string) {
	t.Helper()
	encoded, err := json.Marshal(attachments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), raw) {
		t.Fatalf("attachment projection leaked raw image payload: %s", encoded)
	}
}
