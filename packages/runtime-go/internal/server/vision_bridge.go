package server

import (
	"context"

	attachmentpipelineapp "analytix.local/runtime-go/internal/app/attachmentpipeline"
	appturn "analytix.local/runtime-go/internal/app/turn"
	visionbridgeapp "analytix.local/runtime-go/internal/app/visionbridge"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	provider "analytix.local/runtime-go/internal/provider"
)

func (h *runtimeServerHandler) runtimeVisionBridgeService() visionbridgeapp.Service {
	if h == nil {
		return visionbridgeapp.Service{}
	}
	executionResolver, _ := h.providerExecution.(visionbridgeapp.ExecutionResolver)
	intent := h.visionBridge
	intent.ProviderID = ""
	intent.BaseURL = ""
	intent.APIKey = ""
	intent.Model = ""
	intent.EndpointFormat = ""
	return visionbridgeapp.Service{
		Provider: h.provider, ExecutionResolver: executionResolver, DefaultConfig: intent,
	}
}

func (h *runtimeServerHandler) runtimeAttachmentPipeline() attachmentpipelineapp.Service {
	if h == nil || h.attachmentAccess == nil {
		return attachmentpipelineapp.Service{}
	}
	return attachmentpipelineapp.Service{
		Planner: appturn.AttachmentPlanner{Store: h.attachments, Owners: h.attachmentAccess.Owners},
		Uses:    h.attachmentUses,
		Vision:  h.runtimeVisionBridgeService(),
	}
}

func (h *runtimeServerHandler) planRuntimeAttachments(
	ctx context.Context,
	ids []string,
	securityContext domainsecurity.TurnSecurityContext,
	config provider.TurnConfig,
) (appturn.AttachmentPlan, error) {
	if len(ids) == 0 {
		return appturn.AttachmentPlan{}, nil
	}
	return h.runtimeAttachmentPipeline().Planner.Plan(ctx, appturn.AttachmentPlanInput{
		IDs: ids, SecurityContext: securityContext,
		ModelInputModalities: config.InputModalities, ModelMessageParts: config.MessageParts,
	})
}
