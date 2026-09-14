// Package officeediting adapts fixed read-only Office previews to Core-owned
// object sessions. Document bytes travel only on the protected-local display lane.
package officeediting

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// MaxInputBytes bounds the open/close request; document bytes are output only.
const MaxInputBytes = 16 << 10

var (
	ErrUnavailable = errors.New("office_editor_unavailable")
	ErrBinding     = errors.New("office_editor_binding_invalid")
	sessionPattern = regexp.MustCompile(`^[a-f0-9]{48}$`)
)

// EditingService exposes only principal-bound preview session operations.
// Commit and Status are intentionally absent from this adapter capability.
type EditingService interface {
	Open(context.Context, string, string) (editingapp.Opened, error)
	Close(context.Context, string) error
}

type Adapter struct {
	packageID string
	service   EditingService
	ready     func(context.Context) bool
}

var _ adapterport.Adapter = (*Adapter)(nil)
var _ EditingService = (*editingapp.Service)(nil)

// New fixes the package binding; unknown kinds fail closed at every entrypoint.
// readiness is supplied by trusted composition after local engine verification.
func New(kind string, service EditingService, readiness func(context.Context) bool) *Adapter {
	id := ""
	switch kind {
	case "docx":
		id = "analytix-documents"
	case "xlsx":
		id = "analytix-spreadsheets"
	case "pptx":
		id = "analytix-presentations"
	}
	return &Adapter{packageID: id, service: service, ready: readiness}
}

func (a *Adapter) checkBinding(binding adapterport.Binding) error {
	if a == nil || a.packageID == "" || binding.PackageID != a.packageID {
		return ErrBinding
	}
	return nil
}

func (a *Adapter) available(ctx context.Context) bool {
	return ctx != nil && ctx.Err() == nil && a.service != nil && a.ready != nil && a.ready(ctx)
}

func (a *Adapter) Readiness(ctx context.Context, binding adapterport.Binding) (adapterport.Readiness, error) {
	if err := a.checkBinding(binding); err != nil {
		return adapterport.Readiness{}, err
	}
	return adapterport.Readiness{Available: a.available(ctx), Operations: []string{"open-object", "close-object"}}, nil
}

func (a *Adapter) Invoke(ctx context.Context, call adapterport.Call) (adapterport.Result, error) {
	if err := a.checkBinding(call.Binding); err != nil {
		return adapterport.Result{}, err
	}
	// The Host owns ValidateCurrent before/after dispatch. Structural validation
	// here rejects a malformed Host projection; it is not a second authority.
	if call.ContributionID != "workspace-editor" || identitydomain.ValidatePrincipalV1(call.Principal) != nil {
		return adapterport.Result{}, ErrBinding
	}
	if !a.available(ctx) {
		return adapterport.Result{}, ErrUnavailable
	}
	if call.Operation == "commit-object" || call.Operation == "object-status" {
		return failure(fileport.ErrForbidden)
	}
	input, err := jsonstrict.DecodeObject(call.Input, jsonstrict.Options{MaxBytes: MaxInputBytes, MaxDepth: 2, MaxTokens: 16, MaxStringBytes: 4096})
	if err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	switch call.Operation {
	case "open-object":
		object, ok := input["object"].(map[string]any)
		if !exactKeys(input, "object") || !ok || !exactKeys(object, "workspace", "path") {
			return failure(fileport.ErrInvalidInput)
		}
		workspace, validWorkspace := boundedString(object, "workspace", 4096)
		path, validPath := boundedString(object, "path", 4096)
		if !validWorkspace || !filepath.IsAbs(workspace) || !validPath {
			return failure(fileport.ErrInvalidInput)
		}
		doc, err := a.service.Open(ctx, workspace, path)
		if err != nil {
			return failure(err)
		}
		return output(struct {
			OK       bool              `json:"ok"`
			Document editingapp.Opened `json:"document"`
		}{true, doc})
	case "close-object":
		if !exactKeys(input, "sessionId") {
			return failure(fileport.ErrInvalidInput)
		}
		sessionID, _ := input["sessionId"].(string)
		if !sessionPattern.MatchString(sessionID) {
			return failure(fileport.ErrInvalidInput)
		}
		if err := a.service.Close(ctx, sessionID); err != nil {
			return failure(err)
		}
		return output(struct {
			OK     bool `json:"ok"`
			Closed bool `json:"closed"`
		}{true, true})
	default:
		return failure(fileport.ErrInvalidInput)
	}
}

func exactKeys(object map[string]any, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	return true
}

func boundedString(object map[string]any, key string, limit int) (string, bool) {
	value, ok := object[key].(string)
	return value, ok && strings.TrimSpace(value) != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

func failure(err error) (adapterport.Result, error) {
	code := "unavailable"
	switch {
	case errors.Is(err, fileport.ErrInvalidInput):
		code = "invalid_request"
	case errors.Is(err, editingapp.ErrSession):
		code = "session_invalid"
	case errors.Is(err, editingapp.ErrCapacity):
		code = "capacity"
	case errors.Is(err, fileport.ErrForbidden):
		code = "forbidden"
	case errors.Is(err, fileport.ErrNotText):
		code = "not_text"
	case errors.Is(err, fileport.ErrTooLarge):
		code = "too_large"
	case errors.Is(err, fileport.ErrConflict):
		code = "conflict"
	case errors.Is(err, fileport.ErrOperationMismatch):
		code = "operation_mismatch"
	case errors.Is(err, fileport.ErrOperationNotFound):
		code = "operation_not_found"
	case errors.Is(err, fileport.ErrPersistence):
		code = "persistence_failure"
	}
	response := struct {
		OK      bool   `json:"ok"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: "The object operation could not be completed."}
	return output(response)
}

func output(value any) (adapterport.Result, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return adapterport.Result{}, ErrUnavailable
	}
	return adapterport.Result{Output: body}, nil
}
