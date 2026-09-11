package visionbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/ports"
)

var (
	ErrAuthorizationRequired  = errors.New("vision bridge authorization is required")
	ErrImageEffectUnavailable = errors.New("vision bridge image effect is unavailable")
)

const imageEffectUnavailableReason = "trusted local image privacy projection is unavailable"

type ExecutionResolver interface {
	ResolveVisionExecution(context.Context) (ExecutionLease, error)
}

// ExecutionLease owns one JIT-resolved Registry media route and credential.
// It is process-local and must be cleared immediately after the corresponding
// physical Provider effect.
type ExecutionLease struct {
	Config          domainmodel.TurnConfig
	validateCurrent func(context.Context) error
	clear           func()
}

func NewExecutionLease(
	config domainmodel.TurnConfig,
	validateCurrent func(context.Context) error,
	clearLease func(),
) ExecutionLease {
	return ExecutionLease{Config: config, validateCurrent: validateCurrent, clear: clearLease}
}

func (lease ExecutionLease) ValidateCurrent(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || lease.validateCurrent == nil {
		return errors.New("vision bridge provider authority is unavailable")
	}
	return lease.validateCurrent(ctx)
}

func (lease *ExecutionLease) Clear() {
	if lease == nil {
		return
	}
	lease.Config.APIKey = ""
	lease.Config.BaseURL = ""
	lease.Config.ProxyURL = ""
	if lease.clear != nil {
		lease.clear()
	}
	lease.validateCurrent = nil
	lease.clear = nil
}

type Service struct {
	Provider          ports.ProviderClient
	ExecutionResolver ExecutionResolver
	DefaultConfig     runtimeinfoapp.VisionBridgeConfig
	projector         hostImageProjector
}

// hostImageProjector is deliberately package-private: a Provider, plugin, or
// server configuration cannot assert that source-exact image bytes are safe.
// A future trusted local implementation must still pass the Core-owned final
// provider projection below before any network effect is possible.
type hostImageProjector interface {
	ProjectImageForProvider(context.Context, runtimeinfoapp.ToolResultImage) (hostProjectedImage, error)
}

type hostProjectedImage struct {
	part domainmodel.MessagePart
}

type PrepareAttachmentsInput struct {
	Attachments       appturn.ResolvedAttachments
	Primary           domainmodel.TurnConfig
	PrimaryProviderID string
	PrimaryModel      string
	ProviderTelemetry *domainmodel.ProviderTelemetryBindingV1
	Authorize         func() error
}

type authorizationError struct {
	cause error
}

type imageEffectUnavailableError struct {
	cause error
}

func (err authorizationError) Error() string {
	if err.cause == nil {
		return ErrAuthorizationRequired.Error()
	}
	return err.cause.Error()
}

func (err authorizationError) Unwrap() error {
	if err.cause == nil {
		return ErrAuthorizationRequired
	}
	return err.cause
}

func IsAuthorizationError(err error) bool {
	var target authorizationError
	return errors.As(err, &target)
}

func (err imageEffectUnavailableError) Error() string {
	return imageEffectUnavailableReason
}

func (err imageEffectUnavailableError) Unwrap() error {
	return err.cause
}

func (err imageEffectUnavailableError) Is(target error) bool {
	return target == ErrImageEffectUnavailable
}

func IsImageEffectUnavailable(err error) bool {
	return errors.Is(err, ErrImageEffectUnavailable)
}

func (service Service) PrepareAttachments(ctx context.Context, input PrepareAttachmentsInput) (appturn.ResolvedAttachments, error) {
	attachments := input.Attachments
	if len(attachments.ImageCandidates) == 0 {
		return attachments, nil
	}
	config := runtimeinfoapp.NormalizeVisionBridgeConfig(service.DefaultConfig)
	attachments.VisionBridgeProviderID = config.ProviderID
	attachments.VisionBridgeModel = config.Model
	attachments.VisionBridgeImageCount = len(attachments.ImageCandidates)

	primarySupportsImages := input.Primary.SupportsImageInput ||
		(stringSliceContains(input.Primary.InputModalities, "image") && runtimeinfoapp.MessagePartsSupportImage(input.Primary.MessageParts))
	if !runtimeinfoapp.ShouldRunVisionBridge(config, primarySupportsImages) {
		if primarySupportsImages {
			attachments.VisionBridgeStatus = "skipped"
			attachments.VisionBridgeReason = "primary model supports native image input"
		} else if !config.FallbackWhenPrimaryImageUnsupported {
			attachments.VisionBridgeStatus = "skipped"
			attachments.VisionBridgeReason = "vision bridge fallback for image-unsupported primary models is disabled"
		}
		return attachments, nil
	}

	attachments.VisionBridgeEligible = true
	images := make([]runtimeinfoapp.ToolResultImage, 0, len(attachments.ImageCandidates))
	sourceNames := make([]string, 0, len(attachments.ImageCandidates))
	for _, image := range attachments.ImageCandidates {
		if strings.TrimSpace(image.DataBase64) == "" {
			continue
		}
		images = append(images, runtimeinfoapp.ToolResultImage{MediaType: image.MIMEType, DataBase64: image.DataBase64})
		sourceName := strings.TrimSpace(image.Name)
		if sourceName == "" {
			sourceName = strings.TrimSpace(image.ID)
		}
		if sourceName != "" {
			sourceNames = append(sourceNames, sourceName)
		}
	}
	if len(images) == 0 {
		attachments.VisionBridgeStatus = "unavailable"
		attachments.VisionBridgeReason = "no bridge-compatible image attachment data was available"
		return bridgeBoundaryAttachments(attachments, runtimeinfoapp.VisionBridgeObserveInput{
			Config: config, SourceKind: runtimeinfoapp.VisionBridgeSourceUserAttachment,
			Status: "unavailable", Reason: attachments.VisionBridgeReason,
		}), nil
	}
	maxImages := runtimeinfoapp.VisionBridgeMaxScreenshots(config)
	omitted := 0
	if maxImages > 0 && len(images) > maxImages {
		omitted = len(images) - maxImages
	}
	observeInput := runtimeinfoapp.VisionBridgeObserveInput{
		Config: config, SourceKind: runtimeinfoapp.VisionBridgeSourceUserAttachment,
		SourceName: strings.Join(sourceNames, ", "), Images: images, OmittedCount: omitted,
		PrimaryModel: input.PrimaryModel, PrimaryProvider: input.PrimaryProviderID,
	}
	observations, err := service.Observe(ctx, observeInput, input.ProviderTelemetry, input.Authorize)
	attachments.VisionBridgeUsed = true
	attachments.VisionBridgeOmittedCount = omitted
	if err != nil {
		attachments.VisionBridgeObservedCount = 0
		if IsImageEffectUnavailable(err) {
			attachments.VisionBridgeUsed = false
			attachments.VisionBridgeStatus = "unavailable"
			attachments.VisionBridgeReason = imageEffectUnavailableReason
			observeInput.SourceName = ""
			observeInput.Status = "unavailable"
			observeInput.Reason = imageEffectUnavailableReason
			return bridgeBoundaryAttachments(attachments, observeInput), nil
		}
		if IsAuthorizationError(err) || providerRetryForbidden(err) {
			attachments.VisionBridgeStatus = "unavailable"
			attachments.VisionBridgeReason = "current turn authority is unavailable"
			observeInput.Status = "unavailable"
			observeInput.Reason = attachments.VisionBridgeReason
			return bridgeBoundaryAttachments(attachments, observeInput), err
		}
		attachments.VisionBridgeStatus = "failed"
		attachments.VisionBridgeReason = err.Error()
		observeInput.Status = "failed"
		observeInput.Reason = err.Error()
		return bridgeBoundaryAttachments(attachments, observeInput), nil
	}
	attachments.VisionBridgeStatus = "supported"
	attachments.VisionBridgeObservedCount = len(observations)
	observeInput.Status = "supported"
	observeInput.ObservedCount = len(observations)
	observeInput.Observations = observations
	return bridgeBoundaryAttachments(attachments, observeInput), nil
}

func (service Service) Observe(
	ctx context.Context,
	input runtimeinfoapp.VisionBridgeObserveInput,
	providerTelemetry *domainmodel.ProviderTelemetryBindingV1,
	authorize func() error,
) ([]map[string]any, error) {
	if len(input.Images) == 0 {
		return nil, errors.New("vision bridge found no image to observe")
	}
	if service.Provider == nil {
		return nil, errors.New("vision bridge provider client is unavailable")
	}
	config := runtimeinfoapp.NormalizeVisionBridgeConfig(input.Config)
	if authorize == nil {
		return nil, authorizationError{}
	}
	if err := validateProviderTelemetry(providerTelemetry); err != nil {
		return nil, authorizationError{cause: err}
	}
	images := input.Images
	if maxImages := runtimeinfoapp.VisionBridgeMaxScreenshots(config); maxImages > 0 && len(images) > maxImages {
		images = images[:maxImages]
	}
	observations := make([]map[string]any, 0, len(images))
	for index, image := range images {
		if err := authorize(); err != nil {
			return nil, authorizationError{cause: err}
		}
		if service.projector == nil {
			return nil, imageEffectUnavailableError{cause: ErrImageEffectUnavailable}
		}
		projectedImage, err := service.projector.ProjectImageForProvider(ctx, image)
		if err != nil {
			return nil, imageEffectUnavailableError{cause: err}
		}
		if strings.TrimSpace(projectedImage.part.Type) != "image" ||
			(strings.TrimSpace(projectedImage.part.Data) == "" && strings.TrimSpace(projectedImage.part.ImageURL) == "") {
			return nil, imageEffectUnavailableError{cause: ErrImageEffectUnavailable}
		}
		if service.ExecutionResolver == nil {
			return nil, errors.New("vision bridge provider execution authority is unavailable")
		}
		execution, err := service.ExecutionResolver.ResolveVisionExecution(ctx)
		if err != nil {
			return nil, authorizationError{cause: errors.New("vision bridge provider execution authority is unavailable")}
		}
		observation, err := service.observeProjectedImage(
			ctx, input, image, projectedImage, index, providerTelemetry, authorize, execution,
		)
		if err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, nil
}

func (service Service) observeProjectedImage(
	ctx context.Context,
	input runtimeinfoapp.VisionBridgeObserveInput,
	image runtimeinfoapp.ToolResultImage,
	projectedImage hostProjectedImage,
	index int,
	providerTelemetry *domainmodel.ProviderTelemetryBindingV1,
	authorize func() error,
	execution ExecutionLease,
) (map[string]any, error) {
	defer execution.Clear()
	requestConfig := execution.Config
	if strings.TrimSpace(requestConfig.ProviderID) == "" || strings.TrimSpace(requestConfig.BaseURL) == "" ||
		strings.TrimSpace(requestConfig.APIKey) == "" || strings.TrimSpace(requestConfig.Model) == "" {
		return nil, authorizationError{cause: errors.New("vision bridge provider execution authority is unavailable")}
	}
	validateCurrent := func() error {
		if err := authorize(); err != nil {
			return authorizationError{cause: err}
		}
		if err := execution.ValidateCurrent(ctx); err != nil {
			return authorizationError{cause: errors.New("vision bridge provider execution authority changed")}
		}
		return nil
	}
	beforeSend := func(_ int) error {
		return validateCurrent()
	}
	telemetry := *providerTelemetry
	telemetry.Channel = domaincache.ProviderChannelAttachmentVision
	telemetry.LaneCallSequence = uint32(index + 1)
	request := domainmodel.Request{
		ProviderID: requestConfig.ProviderID, Family: requestConfig.Family,
		EndpointFormat: requestConfig.EndpointFormat, BaseURL: requestConfig.BaseURL,
		APIKey: requestConfig.APIKey, Model: requestConfig.Model,
		ReasoningProtocol: requestConfig.ReasoningProtocol, ReasoningEffort: "off",
		SystemPrompt: runtimeinfoapp.VisionBridgeSystemPrompt,
		Messages: []domainmodel.Message{{
			Role: "user", Content: runtimeinfoapp.VisionBridgeObservationPromptForSource(input.SourceKind, input.SourceName),
			Parts: []domainmodel.MessagePart{projectedImage.part},
		}},
		Tools: nil, OnChunk: nil, BeforeSend: beforeSend, PrivateProviderTelemetry: &telemetry,
		ProxyURL: requestConfig.ProxyURL, PrivateProviderProxyAuthority: true,
		PrivateProviderCurrentnessBeforeSend: beforeSend,
	}
	projectedRequest, projectionErr := privacyprojectionapp.ProjectProviderRequestForEffect(
		telemetry.SecurityContext,
		request,
		telemetry.OrdinaryEffect,
	)
	if projectionErr != nil {
		return nil, imageEffectUnavailableError{cause: projectionErr}
	}
	request = domainmodel.Request{}
	result, err := service.Provider.Stream(ctx, projectedRequest)
	if err != nil {
		return nil, errors.New("vision bridge provider request failed")
	}
	if err := execution.ValidateCurrent(ctx); err != nil {
		return nil, authorizationError{cause: errors.New("vision bridge provider execution authority changed")}
	}
	text := domainmodel.ResultText(result)
	if text == "" {
		return nil, errors.New("vision bridge provider returned an invalid observation")
	}
	observation, err := runtimeinfoapp.ParseVisionBridgeObservation(text)
	if err != nil {
		return nil, errors.New("vision bridge provider returned an invalid observation")
	}
	observation, err = normalizeObservation(input.SourceKind, observation)
	if err != nil {
		return nil, errors.New("vision bridge provider returned an invalid observation")
	}
	if observationContainsExactString(observation, image.DataBase64) {
		return nil, errors.New("vision bridge provider returned an invalid observation")
	}
	if sourceName := strings.TrimSpace(input.SourceName); sourceName != "" {
		observation["sourceName"] = sourceName
	}
	observation["imageIndex"] = float64(index)
	return observation, nil
}

func validateProviderTelemetry(binding *domainmodel.ProviderTelemetryBindingV1) error {
	if binding == nil || binding.LogicalSequence == 0 || binding.OuterAttempt == 0 || binding.LaneCallSequence != 0 ||
		binding.Channel != domaincache.ProviderChannelPrimary ||
		(binding.UsageSource != domaincache.ProviderUsageSourceTurn && binding.UsageSource != domaincache.ProviderUsageSourceSubagent) ||
		domainsecurity.ValidateTurnSecurityContextForExecution(binding.SecurityContext) != nil {
		return errors.New("vision bridge provider telemetry authority is unavailable")
	}
	return nil
}

func providerRetryForbidden(err error) bool {
	var marker interface{ ProviderRetryForbidden() bool }
	return errors.As(err, &marker) && marker.ProviderRetryForbidden()
}

func normalizeObservation(kind runtimeinfoapp.VisionBridgeSourceKind, observation map[string]any) (map[string]any, error) {
	stringFields := map[string]bool{"summary": true}
	listFields := map[string]bool{"visible_text": true, "uncertainties": true}
	switch kind {
	case runtimeinfoapp.VisionBridgeSourceUserAttachment:
		listFields["objects"] = true
		listFields["layout_or_spatial_notes"] = true
		listFields["task_relevant_details"] = true
	case runtimeinfoapp.VisionBridgeSourceToolScreenshot:
		stringFields["selected_text"] = true
		listFields["ui_elements"] = true
		listFields["warnings"] = true
	default:
		return nil, errors.New("vision bridge observation source is invalid")
	}
	summary, ok := observation["summary"].(string)
	if !ok || strings.TrimSpace(summary) == "" {
		return nil, errors.New("vision bridge observation summary is missing")
	}
	normalized := make(map[string]any, len(observation)+1)
	for key, value := range observation {
		switch {
		case key == "source_kind":
			continue
		case stringFields[key]:
			text, ok := value.(string)
			if !ok || len(text) > 64<<10 {
				return nil, errors.New("vision bridge observation string field is invalid")
			}
			normalized[key] = text
		case listFields[key]:
			items, ok := value.([]any)
			if !ok || len(items) > 512 {
				return nil, errors.New("vision bridge observation list field is invalid")
			}
			texts := make([]string, len(items))
			for index, item := range items {
				text, ok := item.(string)
				if !ok || len(text) > 64<<10 {
					return nil, errors.New("vision bridge observation list item is invalid")
				}
				texts[index] = text
			}
			normalized[key] = texts
		case key == "confidence":
			confidence, ok := value.(float64)
			if !ok || confidence < 0 || confidence > 1 {
				return nil, errors.New("vision bridge observation confidence is invalid")
			}
			normalized[key] = confidence
		default:
			return nil, fmt.Errorf("vision bridge observation field %q is not allowed", key)
		}
	}
	normalized["source_kind"] = string(kind)
	return normalized, nil
}

func observationContainsExactString(observation map[string]any, expected string) bool {
	if expected == "" {
		return false
	}
	for _, value := range observation {
		switch typed := value.(type) {
		case string:
			if strings.Contains(typed, expected) {
				return true
			}
		case []string:
			for _, item := range typed {
				if strings.Contains(item, expected) {
					return true
				}
			}
		}
	}
	return false
}

func bridgeBoundaryAttachments(attachments appturn.ResolvedAttachments, input runtimeinfoapp.VisionBridgeObserveInput) appturn.ResolvedAttachments {
	attachments = attachments.WithVisionBridgeText(runtimeinfoapp.VisionBridgeObservationText(input))
	attachments.ImageCandidates = append([]appturn.ResolvedAttachmentImage(nil), attachments.ImageCandidates...)
	for index := range attachments.ImageCandidates {
		attachments.ImageCandidates[index].DataBase64 = ""
	}
	return attachments
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
