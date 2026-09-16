package canvasediting

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	canvas "analytix.local/runtime-go/internal/domain/canvas"
	identity "analytix.local/runtime-go/internal/domain/identity"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	files "analytix.local/runtime-go/internal/ports/objectediting"
	host "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// Adapter exposes finite protected-local UI operations through the existing
// plugin Host. It accepts neither candidate bytes nor executable commands.
// Model-facing selection operations use a separate projected scope interface.
type Adapter struct {
	service *Service
	current func(context.Context) bool
}

var _ host.Adapter = (*Adapter)(nil)
var sessionPattern = regexp.MustCompile(`^[a-f0-9]{48}$`)
var revisionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func NewAdapter(service *Service, current func(context.Context) bool) *Adapter {
	return &Adapter{service: service, current: current}
}

func (a *Adapter) BindHost(projector objectapp.TrustedSelectionProjector, capture func(context.Context, string, string, func() error) (func(), error)) error {
	if a == nil || a.service == nil {
		return ErrUnavailable
	}
	return a.service.BindHost(projector, capture)
}

func (a *Adapter) available(ctx context.Context) bool {
	return a != nil && a.service != nil && a.service.projector != nil && a.service.capture != nil && a.current != nil && ctx != nil && ctx.Err() == nil && a.current(ctx)
}

func (a *Adapter) Readiness(ctx context.Context, binding host.Binding) (host.Readiness, error) {
	if binding.PackageID != "analytix-canvas" {
		return host.Readiness{}, ErrUnavailable
	}
	return host.Readiness{Available: a.available(ctx), Operations: []string{
		"open-object", "read-object", "close-object", "propose-scene", "propose-image",
		"proposal-read", "proposal-apply", "proposal-reject", "object-recovery", "undo-change", "resume-change", "cancel-change",
	}}, nil
}

func keys(input map[string]any, wanted ...string) bool {
	if len(input) != len(wanted) {
		return false
	}
	for _, key := range wanted {
		if _, ok := input[key]; !ok {
			return false
		}
	}
	return true
}

func field(input map[string]any, name string) string { v, _ := input[name].(string); return v }

func (a *Adapter) Invoke(ctx context.Context, call host.Call) (host.Result, error) {
	if call.Binding.PackageID != "analytix-canvas" || call.ContributionID != "workspace-editor" || identity.ValidatePrincipalV1(call.Principal) != nil || !a.available(ctx) {
		return host.Result{}, ErrUnavailable
	}
	input, err := jsonstrict.DecodeObject(call.Input, jsonstrict.Options{MaxBytes: 512 << 10, MaxDepth: 8, MaxTokens: 32768, MaxStringBytes: 4096})
	if err != nil {
		return adapterFailure(ErrInvalid, files.Receipt{})
	}
	thread := field(input, "threadId")
	if !threadPattern.MatchString(thread) {
		return adapterFailure(ErrInvalid, files.Receipt{})
	}
	if call.Operation == "open-object" {
		object, ok := input["object"].(map[string]any)
		workspace, path, kind := field(object, "workspace"), field(object, "path"), field(input, "kind")
		if !keys(input, "threadId", "object", "kind") || !ok || !keys(object, "workspace", "path") || workspace == "" || path == "" || strings.ContainsAny(workspace+path, "\x00\r\n") || (kind != "canvas" && kind != "png") {
			return adapterFailure(ErrInvalid, files.Receipt{})
		}
		document, e := a.service.Open(ctx, call.Principal, thread, workspace, path, kind)
		return a.result(ctx, "document", document, e)
	}
	id := field(input, "sessionId")
	if !sessionPattern.MatchString(id) {
		return adapterFailure(ErrInvalid, files.Receipt{})
	}
	switch call.Operation {
	case "read-object", "close-object", "object-recovery":
		if !keys(input, "sessionId", "threadId") {
			break
		}
		switch call.Operation {
		case "read-object":
			v, e := a.service.Read(ctx, call.Principal, id, thread)
			return a.result(ctx, "document", v, e)
		case "close-object":
			return a.result(ctx, "closed", true, a.service.Close(ctx, call.Principal, id, thread))
		default:
			v, e := a.service.Recovery(ctx, call.Principal, id, thread)
			return a.result(ctx, "recovery", v, e)
		}
	case "propose-scene", "propose-image":
		wanted := []string{"sessionId", "threadId", "baseRevision", "operations"}
		if call.Operation == "propose-scene" {
			wanted = append(wanted, "selectedIds")
		}
		base := field(input, "baseRevision")
		if !keys(input, wanted...) || !revisionPattern.MatchString(base) {
			break
		}
		raw, e := json.Marshal(input["operations"])
		if e != nil {
			break
		}
		if call.Operation == "propose-scene" {
			selected, ok := input["selectedIds"].([]any)
			if !ok || len(selected) == 0 || len(selected) > 1280 {
				break
			}
			ids := make([]string, len(selected))
			for i, value := range selected {
				ids[i], ok = value.(string)
				if !ok {
					return adapterFailure(ErrInvalid, files.Receipt{})
				}
			}
			ops, e := canvas.ParseOperations(raw)
			if e != nil {
				break
			}
			v, e := a.service.ProposeScene(ctx, call.Principal, id, thread, base, ids, ops)
			return a.result(ctx, "proposal", v, e)
		}
		ops, e := canvas.ParseImageOperations(raw)
		if e != nil {
			break
		}
		v, e := a.service.ProposeImage(ctx, call.Principal, id, thread, base, ops)
		return a.result(ctx, "proposal", v, e)
	case "proposal-read", "proposal-apply", "proposal-reject":
		proposal := field(input, "proposalId")
		if !keys(input, "sessionId", "threadId", "proposalId") || !sessionPattern.MatchString(proposal) {
			break
		}
		switch call.Operation {
		case "proposal-read":
			v, e := a.service.Proposal(ctx, call.Principal, id, thread, proposal)
			return a.result(ctx, "proposal", v, e)
		case "proposal-reject":
			return a.result(ctx, "rejected", true, a.service.Reject(ctx, call.Principal, id, thread, proposal))
		default:
			v, e := a.service.Apply(ctx, call.Principal, id, thread, proposal)
			return a.receipt(ctx, v, e)
		}
	case "undo-change", "resume-change", "cancel-change":
		change, base := field(input, "changeId"), field(input, "baseRevision")
		if !keys(input, "sessionId", "threadId", "changeId", "baseRevision") || !revisionPattern.MatchString(change) || !revisionPattern.MatchString(base) {
			break
		}
		if call.Operation == "cancel-change" {
			v, e := a.service.Cancel(ctx, call.Principal, id, thread, change, base)
			return a.result(ctx, "recovery", v, e)
		}
		action := strings.TrimSuffix(call.Operation, "-change")
		v, e := a.service.RecoverOperation(ctx, call.Principal, id, thread, action, change, base)
		return a.receipt(ctx, v, e)
	}
	return adapterFailure(ErrInvalid, files.Receipt{})
}

func (a *Adapter) result(ctx context.Context, key string, value any, err error) (host.Result, error) {
	if err != nil {
		return adapterFailure(err, files.Receipt{})
	}
	if !a.available(ctx) {
		return adapterFailure(ErrUnavailable, files.Receipt{})
	}
	return adapterOutput(map[string]any{"ok": true, key: value})
}
func (a *Adapter) receipt(ctx context.Context, value files.Receipt, err error) (host.Result, error) {
	if !a.available(ctx) {
		return adapterFailure(ErrUnavailable, files.Receipt{OperationID: value.OperationID, Status: files.StatusUnknown})
	}
	if err != nil {
		return adapterFailure(err, value)
	}
	return adapterOutput(map[string]any{"ok": true, "receipt": adapterReceipt(value)})
}
func adapterFailure(err error, receipt files.Receipt) (host.Result, error) {
	code := "unavailable"
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, files.ErrInvalidInput):
		code = "invalid_request"
	case errors.Is(err, ErrStale), errors.Is(err, files.ErrConflict):
		code = "conflict"
	case errors.Is(err, files.ErrForbidden):
		code = "forbidden"
	case errors.Is(err, files.ErrTooLarge):
		code = "too_large"
	case errors.Is(err, files.ErrPersistence):
		code = "persistence_failure"
	case errors.Is(err, files.ErrOperationMismatch):
		code = "operation_mismatch"
	case errors.Is(err, files.ErrOperationNotFound):
		code = "operation_not_found"
	}
	v := map[string]any{"ok": false, "code": code}
	if receipt.OperationID != "" {
		v["receipt"] = adapterReceipt(receipt)
	}
	return adapterOutput(v)
}
func adapterReceipt(value files.Receipt) map[string]any {
	return map[string]any{"operationId": value.OperationID, "revision": value.Revision, "status": value.Status, "savedAt": value.SavedAt}
}
func adapterOutput(value any) (host.Result, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return host.Result{}, ErrUnavailable
	}
	return host.Result{Output: raw}, nil
}
