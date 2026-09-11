package attachmentpipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	appattachmentuse "analytix.local/runtime-go/internal/app/attachmentuse"
	appmodel "analytix.local/runtime-go/internal/app/model"
	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	appturn "analytix.local/runtime-go/internal/app/turn"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

var (
	ErrUnavailable = errors.New("attachment provider pipeline is unavailable")
	ErrMismatch    = errors.New("attachment provider pipeline authority is invalid")
)

type Service struct {
	Planner appturn.AttachmentPlanner
	Uses    *appattachmentuse.Service
	Vision  visionbridgeapp.Service
}

type ProviderAttemptInput struct {
	SecurityContext  domainsecurity.TurnSecurityContext
	Plan             appturn.AttachmentPlan
	Request          domainmodel.Request
	Primary          domainmodel.TurnConfig
	PrimaryProvider  string
	PrimaryModel     string
	PromptRoute      string
	ToolManifestHash string
	Sequence         uint64
	Attempt          int
}

type PreparedProviderAttempt struct {
	Request domainmodel.Request
	Token   ProviderAttemptToken
}

type ProviderAttemptToken struct {
	receipts []domainattachment.AttachmentUseReceiptV1
}

func (service Service) Available() bool {
	return service.Planner.Store != nil && service.Planner.Owners != nil && service.Uses != nil && service.Uses.Available()
}

func (service Service) PrepareProviderAttempt(ctx context.Context, input ProviderAttemptInput) (PreparedProviderAttempt, error) {
	if !service.Available() || input.Plan.Empty() || input.Attempt <= 0 || input.Sequence == 0 ||
		strings.TrimSpace(input.Request.PrivateAttachmentPlanDigest) != input.Plan.PlanDigest ||
		strings.TrimSpace(input.PromptRoute) == "" || !domainsecurity.IsSHA256Hex(strings.TrimSpace(input.ToolManifestHash)) ||
		appturn.ValidateAttachmentPlanForContext(input.Plan, input.SecurityContext) != nil {
		return PreparedProviderAttempt{}, ErrUnavailable
	}
	consumerKind := domainattachment.AttachmentUseConsumerPrimaryProviderV1
	if attachmentAttemptUsesVisionBridge(input.Plan, input.Primary, service.Vision.DefaultConfig) {
		consumerKind = domainattachment.AttachmentUseConsumerProviderPipelineV1
	}
	effectBindingDigest := providerAttemptEffectDigest(input)
	consumerBindingDigest := providerAttemptConsumerDigest(input, service.Vision.DefaultConfig, consumerKind)
	if !domainsecurity.IsSHA256Hex(effectBindingDigest) || !domainsecurity.IsSHA256Hex(consumerBindingDigest) {
		return PreparedProviderAttempt{}, ErrMismatch
	}
	useInput := appattachmentuse.IssueBatchInput{
		SecurityContext:     input.SecurityContext,
		Owners:              input.Plan.Owners(),
		Projections:         input.Plan.ProjectionSet(),
		EffectBindingDigest: effectBindingDigest,
		ConsumerKind:        consumerKind, ConsumerBindingDigest: consumerBindingDigest,
		IssuedAt: time.Now().UTC(),
	}
	receipts, err := service.Uses.IssueBatch(ctx, useInput)
	if err != nil {
		return PreparedProviderAttempt{}, err
	}
	reject := func(cause error, reason string) (PreparedProviderAttempt, error) {
		closeErr := service.closePrepared(ctx, receipts, domainattachment.AttachmentUseDispositionRejectedV1, reason)
		return PreparedProviderAttempt{}, errors.Join(cause, closeErr)
	}
	if err := service.Uses.VerifyOpenBatch(ctx, useInput, receipts); err != nil {
		return reject(err, "pre_materialize_verification_failed")
	}
	materialized, err := service.Planner.Materialize(ctx, appturn.AttachmentMaterializeInput{
		Plan: input.Plan, SecurityContext: input.SecurityContext,
	})
	if err != nil {
		return reject(err, "materialization_failed")
	}
	materialized, err = service.Vision.PrepareAttachments(ctx, visionbridgeapp.PrepareAttachmentsInput{
		Attachments: materialized,
		Primary:     input.Primary, PrimaryProviderID: input.PrimaryProvider, PrimaryModel: input.PrimaryModel,
		ProviderTelemetry: input.Request.PrivateProviderTelemetry,
		Authorize:         func() error { return service.Uses.VerifyOpenBatch(ctx, useInput, receipts) },
	})
	if err != nil {
		materialized = appturn.ResolvedAttachments{}
		return reject(err, "vision_bridge_authorization_failed")
	}
	if err := service.Uses.VerifyOpenBatch(ctx, useInput, receipts); err != nil {
		materialized = appturn.ResolvedAttachments{}
		return reject(err, "pre_send_verification_failed")
	}
	request, err := bindAttachmentParts(input.Request, input.Plan.PlanDigest, materialized.MessageParts)
	materialized = appturn.ResolvedAttachments{}
	if err != nil {
		return reject(err, "provider_message_binding_failed")
	}
	beforeSend := request.BeforeSend
	request.BeforeSend = func(attempt int) error {
		if err := service.Uses.VerifyOpenBatch(ctx, useInput, receipts); err != nil {
			return err
		}
		if beforeSend != nil {
			return beforeSend(attempt)
		}
		return nil
	}
	return PreparedProviderAttempt{Request: request, Token: ProviderAttemptToken{receipts: receipts}}, nil
}

func (service Service) SettleProviderAttempt(ctx context.Context, token ProviderAttemptToken, status, reason string) error {
	if !service.Available() || len(token.receipts) == 0 {
		return ErrUnavailable
	}
	status = strings.TrimSpace(status)
	switch status {
	case domainattachment.AttachmentUseDispositionConsumedV1,
		domainattachment.AttachmentUseDispositionCancelledV1,
		domainattachment.AttachmentUseDispositionFailedV1:
	default:
		return ErrMismatch
	}
	_, err := service.Uses.CloseBatch(ctx, token.receipts, status, strings.TrimSpace(reason), time.Now().UTC())
	token.receipts = nil
	return err
}

func (service Service) closePrepared(ctx context.Context, receipts []domainattachment.AttachmentUseReceiptV1, status, reason string) error {
	if len(receipts) == 0 {
		return nil
	}
	_, err := service.Uses.CloseBatch(ctx, receipts, status, reason, time.Now().UTC())
	return err
}

func bindAttachmentParts(request domainmodel.Request, planDigest string, parts []domainmodel.MessagePart) (domainmodel.Request, error) {
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(planDigest)) || len(parts) == 0 ||
		strings.TrimSpace(request.PrivateAttachmentPlanDigest) != strings.TrimSpace(planDigest) {
		return domainmodel.Request{}, ErrMismatch
	}
	messages := appmodel.CloneProviderMessages(request.Messages)
	matched := -1
	for index := range messages {
		if messages[index].PrivateAttachmentPlanDigest != planDigest {
			continue
		}
		if matched >= 0 || strings.TrimSpace(messages[index].Role) != "user" || len(messages[index].Parts) != 0 {
			return domainmodel.Request{}, ErrMismatch
		}
		matched = index
	}
	if matched < 0 {
		return domainmodel.Request{}, ErrMismatch
	}
	messages[matched].Parts = cloneMessageParts(parts)
	request.Messages = messages
	return request, nil
}

func attachmentAttemptUsesVisionBridge(plan appturn.AttachmentPlan, primary domainmodel.TurnConfig, config runtimeinfoapp.VisionBridgeConfig) bool {
	hasImage := false
	for _, item := range plan.Items {
		if strings.HasPrefix(strings.ToLower(item.MIMEType), "image/") {
			hasImage = true
			break
		}
	}
	primarySupportsImages := primary.SupportsImageInput ||
		(stringSliceContains(primary.InputModalities, "image") && runtimeinfoapp.MessagePartsSupportImage(primary.MessageParts))
	return hasImage && runtimeinfoapp.ShouldRunVisionBridge(runtimeinfoapp.NormalizeVisionBridgeConfig(config), primarySupportsImages)
}

func providerAttemptEffectDigest(input ProviderAttemptInput) string {
	value := struct {
		Version          string `json:"version"`
		ContextDigest    string `json:"contextDigest"`
		AttachmentPlan   string `json:"attachmentPlanDigest"`
		ProviderCall     uint64 `json:"providerCallSequence"`
		Attempt          int    `json:"attempt"`
		PromptRoute      string `json:"promptRoute"`
		ToolManifestHash string `json:"toolManifestHash"`
	}{
		Version: "analytix.attachment-provider-effect/v1", ContextDigest: input.SecurityContext.ContextDigest,
		AttachmentPlan: input.Plan.PlanDigest, ProviderCall: input.Sequence, Attempt: input.Attempt,
		PromptRoute: strings.TrimSpace(input.PromptRoute), ToolManifestHash: strings.TrimSpace(input.ToolManifestHash),
	}
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(body)
}

func providerAttemptConsumerDigest(input ProviderAttemptInput, config runtimeinfoapp.VisionBridgeConfig, consumerKind string) string {
	config = runtimeinfoapp.NormalizeVisionBridgeConfig(config)
	value := struct {
		Version         string `json:"version"`
		ConsumerKind    string `json:"consumerKind"`
		PrimaryProvider string `json:"primaryProvider"`
		PrimaryFamily   string `json:"primaryFamily"`
		PrimaryEndpoint string `json:"primaryEndpointFormat"`
		PrimaryBaseURL  string `json:"primaryBaseUrl"`
		PrimaryModel    string `json:"primaryModel"`
		VisionProvider  string `json:"visionProvider,omitempty"`
		VisionEndpoint  string `json:"visionEndpointFormat,omitempty"`
		VisionBaseURL   string `json:"visionBaseUrl,omitempty"`
		VisionModel     string `json:"visionModel,omitempty"`
	}{
		Version: "analytix.attachment-consumer-route/v1", ConsumerKind: consumerKind,
		PrimaryProvider: strings.TrimSpace(input.Primary.ProviderID), PrimaryFamily: strings.TrimSpace(input.Primary.Family),
		PrimaryEndpoint: strings.TrimSpace(input.Primary.EndpointFormat), PrimaryBaseURL: strings.TrimSpace(input.Primary.BaseURL),
		PrimaryModel: strings.TrimSpace(input.Primary.Model),
	}
	if consumerKind == domainattachment.AttachmentUseConsumerProviderPipelineV1 ||
		consumerKind == domainattachment.AttachmentUseConsumerVisionBridgeV1 {
		value.VisionProvider = strings.TrimSpace(config.ProviderID)
		value.VisionEndpoint = strings.TrimSpace(config.EndpointFormat)
		value.VisionBaseURL = strings.TrimSpace(config.BaseURL)
		value.VisionModel = strings.TrimSpace(config.Model)
	}
	body, _ := json.Marshal(value)
	return domainsecurity.SHA256Hex(body)
}

func cloneMessageParts(parts []domainmodel.MessagePart) []domainmodel.MessagePart {
	return append([]domainmodel.MessagePart(nil), parts...)
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
