// Package officeediting adapts bounded native Office editing to Core-owned
// object sessions. Document bytes travel only on the protected-local display lane.
package officeediting

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	pathsyntax "path"
	"regexp"
	"strings"
	"sync"

	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// MaxOfficeBytes bounds decoded native documents; the JSON transport is separate.
const MaxOfficeBytes = 16 << 20
const MaxInputBytes = ((MaxOfficeBytes + 2) / 3 * 4) + (16 << 10)

var (
	ErrUnavailable   = errors.New("office_editor_unavailable")
	ErrBinding       = errors.New("office_editor_binding_invalid")
	operationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)
	revisionPattern  = regexp.MustCompile(`^[a-f0-9]{64}$`)
	sessionPattern   = regexp.MustCompile(`^[a-f0-9]{48}$`)
)

// EditingService reuses the existing principal-bound persistence authority.
type EditingService interface {
	Open(context.Context, string, string) (editingapp.Opened, error)
	Close(context.Context, string) error
	Commit(context.Context, string, string, string, string) (fileport.Receipt, error)
	Status(context.Context, string, string) (fileport.Receipt, error)
}

type Adapter struct {
	mu        sync.Mutex
	sessions  map[string]*nativeSession
	scopes    map[string]*nativeScope
	projector editingapp.TrustedSelectionProjector
	capture   func(context.Context, string, string, func() error) (func(), error)
	packageID string
	kind      string
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
	return &Adapter{packageID: id, kind: kind, service: service, ready: readiness, sessions: map[string]*nativeSession{}, scopes: map[string]*nativeScope{}}
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
	operations := []string{"open-object", "object-status", "close-object"}
	if _, ok := a.service.(AnnotationDraftService); ok && a.projector != nil {
		operations = append(operations, "annotation-read", "annotation-write")
	}
	if _, ok := a.service.(NativeRecoveryService); ok && a.projector != nil {
		operations = append(operations, "commit-object", "object-recovery", "undo-change", "cancel-change", "resume-change")
	}
	if a.projector != nil && a.capture != nil {
		operations = append(operations, "capture-selection", "selection-read", "selection-revoke", "proposal-read", "proposal-accept", "proposal-reject", "model-selection-read", "model-selection-propose")
	}
	return adapterport.Readiness{Available: a.available(ctx), Operations: operations}, nil
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
	a.mu.Lock()
	defer a.mu.Unlock()
	limit, stringLimit, depth := 16<<10, 4096, 2
	tokens := 2048
	if call.Operation == "annotation-write" {
		limit, stringLimit = fileport.MaxAnnotationRecordBytes, fileport.MaxAnnotationNoteBytes
	}
	if call.Operation == "capture-selection" || call.Operation == "model-selection-propose" {
		limit, stringLimit = (512 << 10), editingapp.MaxSelectionBytes
		depth, tokens = 6, 16384
	}
	if call.Operation == "commit-object" {
		limit, stringLimit = MaxInputBytes, base64.StdEncoding.EncodedLen(MaxOfficeBytes)
	}

	input, err := jsonstrict.DecodeObject(call.Input, jsonstrict.Options{MaxBytes: limit, MaxDepth: depth, MaxTokens: tokens, MaxStringBytes: stringLimit})
	if err != nil {
		return failure(fileport.ErrInvalidInput)
	}
	switch call.Operation {
	case "annotation-read", "annotation-write":
		return a.invokeAnnotation(ctx, call, input)
	case "object-recovery", "undo-change", "cancel-change", "resume-change":
		return a.invokeRecovery(ctx, call, input)
	case "capture-selection", "selection-read", "selection-revoke", "proposal-read", "proposal-accept", "proposal-reject", "model-selection-read", "model-selection-propose":
		return a.invokeSelection(ctx, call, input)
	case "open-object":
		object, ok := input["object"].(map[string]any)
		if !exactKeys(input, "object") || !ok || !exactKeys(object, "workspace", "path") {
			return failure(fileport.ErrInvalidInput)
		}
		workspace, validWorkspace := boundedString(object, "workspace", 4096)
		path, validPath := boundedString(object, "path", 4096)
		// Native source composition currently admits Linux/macOS only. This is
		// lexical grammar; the existing file port still authorizes the real path.
		if !validWorkspace || !pathsyntax.IsAbs(workspace) || !validPath {
			return failure(fileport.ErrInvalidInput)
		}
		doc, err := a.service.Open(ctx, workspace, path)
		if err != nil {
			return failure(err)
		}
		sessionDoc := doc
		sessionDoc.Content = ""
		if previous := a.sessions[doc.SessionID]; previous == nil {
			a.sessions[doc.SessionID] = &nativeSession{document: sessionDoc, principal: call.Principal, workspace: workspace}
		} else {
			previous.document = sessionDoc
		}
		return output(struct {
			OK       bool              `json:"ok"`
			Document editingapp.Opened `json:"document"`
		}{true, doc})
	case "commit-object", "object-status":
		keys := []string{"sessionId", "operationId"}
		if call.Operation == "commit-object" {
			keys = append(keys, "threadId", "changeId", "baseRevision", "content")
		}
		id, _ := input["sessionId"].(string)
		operation, _ := input["operationId"].(string)
		if !exactKeys(input, keys...) || !sessionPattern.MatchString(id) || !operationPattern.MatchString(operation) {
			return failure(fileport.ErrInvalidInput)
		}
		var receipt fileport.Receipt
		if call.Operation == "commit-object" {
			revision, _ := input["baseRevision"].(string)
			if !revisionPattern.MatchString(revision) {
				return failure(fileport.ErrInvalidInput)
			}
			content, validationErr := a.nativeContent(input["content"])
			if validationErr != nil {
				return failure(validationErr)
			}
			thread, _ := input["threadId"].(string)
			change, _ := input["changeId"].(string)
			if !revisionPattern.MatchString(change) {
				return failure(fileport.ErrInvalidInput)
			}
			if err := a.validateRecoveryThread(ctx, call, id, thread, "edit"); err != nil {
				return failure(err)
			}
			recovery, ok := a.service.(NativeRecoveryService)
			if !ok {
				return failure(ErrUnavailable)
			}
			release, captureErr := a.captureNativeMutation(ctx, id, thread, change, revision, "committed")
			if captureErr != nil {
				return failure(captureErr)
			}
			defer release()
			receipt, err = recovery.CommitNativeChange(ctx, id, thread, change, operation, revision, content)
		} else {
			receipt, err = a.service.Status(ctx, id, operation)
		}
		if err != nil {
			return failureReceipt(err, receipt)
		}
		if receipt.Status == fileport.StatusCommitted {
			for scopeID, scope := range a.scopes {
				if scope.SessionID == id && scope.BaseRevision != receipt.Revision {
					delete(a.scopes, scopeID)
				}
			}
		}
		return output(map[string]any{"ok": true, "receipt": receiptValue(receipt)})
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
		a.closeSelection(sessionID)
		return output(struct {
			OK     bool `json:"ok"`
			Closed bool `json:"closed"`
		}{true, true})
	default:
		return failure(fileport.ErrInvalidInput)
	}
}

// Verify the transport before crossing the persistence authority. The Office
// file codec independently inspects the decoded OPC package before any journal
// or target write, including callers that bypass this adapter.
func (a *Adapter) nativeContent(value any) (string, error) {
	content, ok := value.(map[string]any)
	if !ok || !exactKeys(content, "encoding", "kind", "byteLength", "sha256", "data") || content["encoding"] != "base64" || content["kind"] != a.kind {
		return "", fileport.ErrInvalidInput
	}
	number, ok := content["byteLength"].(json.Number)
	if !ok {
		return "", fileport.ErrInvalidInput
	}
	size, err := number.Int64()
	if err != nil || size <= 0 {
		return "", fileport.ErrInvalidInput
	}
	if size > MaxOfficeBytes {
		return "", fileport.ErrTooLarge
	}
	data, ok := content["data"].(string)
	digest, digestOK := content["sha256"].(string)
	if !ok || !digestOK || !revisionPattern.MatchString(digest) || len(data) != base64.StdEncoding.EncodedLen(int(size)) {
		return "", fileport.ErrInvalidInput
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(data)
	if err != nil || int64(len(raw)) != size || base64.StdEncoding.EncodeToString(raw) != data {
		return "", fileport.ErrInvalidInput
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != digest {
		return "", fileport.ErrInvalidInput
	}
	return data, nil
}

func receiptValue(receipt fileport.Receipt) map[string]any {
	return map[string]any{"operationId": receipt.OperationID, "revision": receipt.Revision, "status": receipt.Status, "savedAt": receipt.SavedAt}
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

func failure(err error) (adapterport.Result, error) { return failureReceipt(err, fileport.Receipt{}) }

func failureReceipt(err error, receipt fileport.Receipt) (adapterport.Result, error) {
	code := "unavailable"
	switch {
	case errors.Is(err, fileport.ErrInvalidInput):
		code = "invalid_request"
	case errors.Is(err, editingapp.ErrScope):
		code = "scope_invalid"
	case errors.Is(err, editingapp.ErrDraftStale):
		code = "draft_stale"
	case errors.Is(err, editingapp.ErrProjection):
		code = "projection_unavailable"
	case errors.Is(err, editingapp.ErrProposal):
		code = "proposal_invalid"
	case errors.Is(err, editingapp.ErrProtected):
		code = "protected_span_invalid"
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
	response := map[string]any{"ok": false, "code": code, "message": "The object operation could not be completed."}
	if receipt.OperationID != "" {
		response["receipt"] = receiptValue(receipt)
	}
	return output(response)
}

func output(value any) (adapterport.Result, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return adapterport.Result{}, ErrUnavailable
	}
	return adapterport.Result{Output: body}, nil
}
