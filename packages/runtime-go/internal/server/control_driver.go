package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	controlapp "analytix.local/runtime-go/internal/app/control"
	turnapp "analytix.local/runtime-go/internal/app/turn"
)

type runtimeControlDriver struct {
	handler *runtimeServerHandler
}

func (d runtimeControlDriver) SendTurn(ctx context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
	if d.handler == nil {
		return nil, errors.New("runtime control handler is nil")
	}
	response, err := d.handler.startRuntimeTurn(ctx, request.ThreadID, request)
	if errors.Is(err, os.ErrNotExist) {
		return nil, controlapp.ErrThreadNotFound
	}
	if errors.Is(err, turnapp.ErrAttachmentNotAuthorized) {
		detail := strings.TrimSpace(strings.TrimPrefix(err.Error(), turnapp.ErrAttachmentNotAuthorized.Error()))
		detail = strings.TrimSpace(strings.TrimPrefix(detail, ":"))
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", controlapp.ErrAttachmentNotAuthorized, detail)
		}
		return nil, controlapp.ErrAttachmentNotAuthorized
	}
	return response, err
}

func (d runtimeControlDriver) SteerTurn(ctx context.Context, request controlapp.SteerTurnRequest) (controlapp.ActionResult, error) {
	if d.handler == nil {
		return controlapp.ActionResult{}, errors.New("runtime control handler is nil")
	}
	return d.handler.steerRuntimeTurn(ctx, request)
}

func (d runtimeControlDriver) InterruptTurn(ctx context.Context, request controlapp.InterruptTurnRequest) (controlapp.ActionResult, error) {
	if d.handler == nil {
		return controlapp.ActionResult{}, errors.New("runtime control handler is nil")
	}
	return d.handler.interruptRuntimeTurn(ctx, request)
}

func (d runtimeControlDriver) ApproveTool(ctx context.Context, request controlapp.ApprovalDecision) (controlapp.ActionResult, error) {
	if d.handler == nil {
		return controlapp.ActionResult{}, errors.New("runtime control handler is nil")
	}
	return d.handler.approveRuntimeTool(ctx, request)
}

func (d runtimeControlDriver) RespondUserInput(ctx context.Context, request controlapp.UserInputResponse) (controlapp.ActionResult, error) {
	if d.handler == nil {
		return controlapp.ActionResult{}, errors.New("runtime control handler is nil")
	}
	return d.handler.respondRuntimeUserInput(ctx, request)
}

func (h *runtimeServerHandler) runtimeControl() *controlapp.Controller {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.control == nil {
		h.control = controlapp.NewController(runtimeControlDriver{handler: h})
	}
	return h.control
}
