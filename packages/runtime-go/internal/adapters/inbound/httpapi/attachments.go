package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	runtimeinfoapp "analytix.local/runtime-go/internal/app/runtimeinfo"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	maxAttachmentUploadRequestBytes = 24 << 20
	maxAttachmentUploadStringBytes  = 16 << 20
)

var attachmentUploadFields = map[string]struct{}{
	"name": {}, "mimeType": {}, "dataBase64": {}, "documentText": {},
	"pageCount": {}, "textFallback": {}, "threadId": {}, "workspace": {},
}

var attachmentTextFallbackFields = map[string]struct{}{
	"dataBase64": {}, "mimeType": {}, "byteSize": {}, "width": {},
	"height": {}, "wasCompressed": {},
}

type AttachmentStore interface {
	Create(body map[string]any) (map[string]any, error)
	Diagnostics() map[string]any
	Metadata(id string) (map[string]any, bool, error)
	Content(id string) (map[string]any, string, bool, error)
}

type AttachmentAuthorizer func(context.Context, map[string]any, string, string) bool
type AttachmentUploadPreparer func(body map[string]any) (map[string]any, error)
type AttachmentUploadCommitter func(context.Context, map[string]any) error
type AttachmentUploader func(context.Context, map[string]any) (map[string]any, error)
type AttachmentProjector func(metadata map[string]any) map[string]any

type AttachmentHandlers struct {
	Store         AttachmentStore
	Authorize     AttachmentAuthorizer
	PrepareUpload AttachmentUploadPreparer
	CommitUpload  AttachmentUploadCommitter
	Upload        AttachmentUploader
	Project       AttachmentProjector
}

func (h AttachmentHandlers) HandleCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if h.Store == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "attachment_store_missing", "message": "attachment store missing"})
		return
	}
	body, err := decodeAttachmentUploadBody(r)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid attachment upload body"})
		return
	}
	for _, key := range []string{"name", "dataBase64", "threadId", "workspace"} {
		if strings.TrimSpace(stringField(body, key)) == "" {
			WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": key + " is required"})
			return
		}
	}
	if h.Upload != nil {
		metadata, err := h.Upload(r.Context(), body)
		if err != nil {
			switch {
			case errors.Is(err, appturn.ErrAttachmentNotAuthorized), errors.Is(err, domainattachment.ErrUploadBindingUnsafe):
				WriteJSON(w, http.StatusForbidden, map[string]any{
					"code": "forbidden", "message": "attachment upload authority rejected",
				})
			case errors.Is(err, domainattachment.ErrUploadInputInvalid):
				WriteJSON(w, http.StatusBadRequest, map[string]any{
					"code": "validation_error", "message": err.Error(),
				})
			default:
				WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
					"code": "attachment_upload_unavailable", "message": "attachment upload transaction could not be committed",
				})
			}
			return
		}
		if h.Project != nil {
			metadata = h.Project(metadata)
		}
		WriteJSON(w, http.StatusCreated, map[string]any{"attachment": metadata})
		return
	}
	if h.PrepareUpload != nil {
		prepared, err := h.PrepareUpload(body)
		if err != nil {
			WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "attachment upload authority rejected"})
			return
		}
		body = prepared
	}
	metadata, err := h.Store.Create(body)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": err.Error()})
		return
	}
	if h.CommitUpload != nil {
		if err := h.CommitUpload(r.Context(), metadata); err != nil {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
				"code": "attachment_authority_unavailable", "message": "attachment upload authority could not be committed",
			})
			return
		}
	}
	if h.Project != nil {
		metadata = h.Project(metadata)
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"attachment": metadata})
}

func decodeAttachmentUploadBody(r *http.Request) (map[string]any, error) {
	if r == nil || r.Body == nil {
		return nil, errors.New("attachment upload body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxAttachmentUploadRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxAttachmentUploadRequestBytes {
		return nil, errors.New("attachment upload body is empty or exceeds size limit")
	}
	fields, err := domainjsonstrict.DecodeRawObject(body, domainjsonstrict.Options{
		MaxBytes:       maxAttachmentUploadRequestBytes,
		MaxDepth:       8,
		MaxTokens:      128,
		MaxStringBytes: maxAttachmentUploadStringBytes,
		MaxNumberBytes: 32,
		MaxAbsExponent: 64,
	})
	if err != nil {
		return nil, err
	}
	for key := range fields {
		if _, ok := attachmentUploadFields[key]; !ok {
			return nil, fmt.Errorf("unknown attachment upload field %q", key)
		}
	}
	decoded := map[string]any{}
	for _, key := range []string{"name", "mimeType", "dataBase64", "documentText", "threadId", "workspace"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("%s must be a string", key)
		}
		decoded[key] = value
	}
	if raw, ok := fields["pageCount"]; ok {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil || value <= 0 {
			return nil, errors.New("pageCount must be a positive integer")
		}
		decoded["pageCount"] = value
	}
	if raw, ok := fields["textFallback"]; ok {
		fallback, err := decodeAttachmentTextFallback(raw)
		if err != nil {
			return nil, err
		}
		decoded["textFallback"] = fallback
	}
	return decoded, nil
}

func decodeAttachmentTextFallback(raw json.RawMessage) (map[string]any, error) {
	fields, err := domainjsonstrict.DecodeRawObject(raw, domainjsonstrict.Options{
		MaxBytes:       maxAttachmentUploadRequestBytes,
		MaxDepth:       4,
		MaxTokens:      32,
		MaxStringBytes: maxAttachmentUploadStringBytes,
		MaxNumberBytes: 32,
		MaxAbsExponent: 64,
	})
	if err != nil {
		return nil, err
	}
	for key := range fields {
		if _, ok := attachmentTextFallbackFields[key]; !ok {
			return nil, fmt.Errorf("unknown attachment text fallback field %q", key)
		}
	}
	decoded := map[string]any{}
	for _, key := range []string{"dataBase64", "mimeType"} {
		rawValue, ok := fields[key]
		if !ok {
			return nil, fmt.Errorf("%s is required", key)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s must be a non-empty string", key)
		}
		decoded[key] = value
	}
	for _, key := range []string{"byteSize", "width", "height"} {
		rawValue, ok := fields[key]
		if !ok {
			if key == "byteSize" {
				return nil, errors.New("byteSize is required")
			}
			continue
		}
		var value int
		if err := json.Unmarshal(rawValue, &value); err != nil || value < 0 || (key != "byteSize" && value == 0) {
			return nil, fmt.Errorf("%s must be a valid integer", key)
		}
		decoded[key] = value
	}
	if rawValue, ok := fields["wasCompressed"]; ok {
		var value bool
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, errors.New("wasCompressed must be a boolean")
		}
		decoded["wasCompressed"] = value
	}
	return decoded, nil
}

func (h AttachmentHandlers) HandleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Store == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "attachment_store_missing", "message": "attachment store missing"})
		return
	}
	writeRuntimeDiagnosticsJSON(w, runtimeinfoapp.ProjectPublicAttachments(h.Store.Diagnostics(), true))
}

func (h AttachmentHandlers) HandlePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Store == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "attachment_store_missing", "message": "attachment store missing"})
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/attachments/")
	content := strings.HasSuffix(rest, "/content")
	id := rest
	if content {
		id = strings.TrimSuffix(rest, "/content")
	}
	threadID := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	if threadID == "" || workspace == "" || len(r.URL.Query()) != 2 || len(r.URL.Query()["thread_id"]) != 1 || len(r.URL.Query()["workspace"]) != 1 {
		WriteJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "thread_id and workspace are required"})
		return
	}
	metadata, ok, err := h.Store.Metadata(id)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	if !ok {
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "attachment not found"})
		return
	}
	if h.Authorize == nil || !h.Authorize(r.Context(), metadata, threadID, workspace) {
		WriteJSON(w, http.StatusForbidden, map[string]any{
			"code":    "forbidden",
			"message": "attachment is not authorized for this turn: " + id,
		})
		return
	}
	publicMetadata := metadata
	if h.Project != nil {
		publicMetadata = h.Project(metadata)
	}
	if !content {
		WriteJSON(w, http.StatusOK, map[string]any{"attachment": publicMetadata})
		return
	}
	contentMetadata, dataBase64, ok, err := h.Store.Content(id)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "internal_error", "message": err.Error()})
		return
	}
	if !ok || h.Authorize == nil || !h.Authorize(r.Context(), contentMetadata, threadID, workspace) {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "forbidden", "message": "attachment is not authorized for this turn: " + id})
		return
	}
	if h.Project != nil {
		publicMetadata = h.Project(contentMetadata)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"attachment": publicMetadata, "dataBase64": dataBase64})
}
