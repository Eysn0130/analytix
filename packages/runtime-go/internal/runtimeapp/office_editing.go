package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	"analytix.local/runtime-go/internal/adapters/outbound/officeengineassets"
	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	docapp "analytix.local/runtime-go/internal/app/documentgeneration"
	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/app/officeediting"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func newOfficeEditingAdapters(ctx context.Context, config Config, identity identityport.Authority, protected []string) map[string]adapterport.Adapter {
	if config.DevelopmentPluginSourceRoot == "" || identity == nil ||
		(runtime.GOOS != "darwin" && runtime.GOOS != "linux") || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	current := func(ctx context.Context) bool {
		if ctx == nil || ctx.Err() != nil {
			return false
		}
		_, err := packagedauthority.InspectCurrentPackageV2(ctx)
		return errors.Is(err, packagedauthority.ErrNotPackagedRuntimeV2)
	}
	if !current(ctx) {
		return nil
	}
	result := composeOfficeEditingAdapters(config, identity, protected, current, "docx")
	if config.DevelopmentOfficeAssetRoot != "" {
		if assets, err := officeengineassets.Open(ctx, config.DevelopmentOfficeAssetRoot); err == nil {
			for id, adapter := range composeOfficeEditingAdapters(config, identity, protected, assets.Current, "xlsx", "pptx") {
				result[id] = adapter
			}
		}
	}
	return result
}

func composeOfficeEditingAdapters(config Config, identity identityport.Authority, protected []string, current func(context.Context) bool, kinds ...string) map[string]adapterport.Adapter {
	if identity == nil || current == nil || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	root := filepath.Join(config.DataDir, "object-editing")
	if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil
	}
	result := make(map[string]adapterport.Adapter)
	if len(kinds) == 0 {
		kinds = []string{"docx", "xlsx", "pptx"}
	}
	for _, kind := range kinds {
		id := map[string]string{"docx": "analytix-documents", "xlsx": "analytix-spreadsheets", "pptx": "analytix-presentations"}[kind]
		if id == "" {
			continue
		}
		files, err := filestore.NewOfficeObjectEditingFiles(root, protected, kind)
		if err != nil {
			continue
		}
		adapter := officeediting.New(kind, editingapp.New(identity, files), current)
		if kind == "docx" {
			if err := adapter.BindManualBeginObserverV1(func(state officeediting.NativeManualBeginPhaseV1, cause error) {
				emitNativeManualBeginPhaseV1(os.Stderr, nativeManualBeginPhaseV1{
					NativeManualBeginPhaseV1: state, ErrorKind: nativeManualBeginErrorKindV1(cause),
				})
			}); err != nil {
				continue
			}
			if err := adapter.BindUserCreator(docapp.NewUserCreator(identity, files)); err != nil {
				continue
			}
		}
		result[id] = adapter
	}
	return result
}

// One fixed failure-only stderr frame. This is diagnostic output, never part of
// HTTP results, readiness, mutation authority or persisted document history.
const nativeManualBeginPhasePrefixV1 = "ANALYTIX_NATIVE_MANUAL_BEGIN_PHASE_V1 "
const nativeManualBeginPhaseMaxBytesV1 = 1024

type nativeManualBeginPhaseV1 struct {
	officeediting.NativeManualBeginPhaseV1
	ErrorKind string `json:"errorKind"`
}

func nativeManualBeginErrorKindV1(err error) string {
	for _, known := range [...]struct {
		cause error
		kind  string
	}{
		{context.Canceled, "context_canceled"}, {context.DeadlineExceeded, "context_deadline"},
		{managededitingapp.ErrUnavailable, "capture_unavailable"}, {managededitingapp.ErrUnsafePath, "unsafe_path"},
		{managededitingapp.ErrSessionMismatch, "capture_session_mismatch"}, {managededitingapp.ErrUncontainedExecution, "opaque_execution"},
		{managededitingapp.ErrCaptureFailed, "baseline_failed"}, {os.ErrNotExist, "not_found"}, {os.ErrPermission, "permission"},
		{fileport.ErrInvalidInput, "invalid_request"}, {editingapp.ErrSession, "session_invalid"},
		{editingapp.ErrScope, "scope_invalid"}, {editingapp.ErrProjection, "projection_unavailable"},
		{editingapp.ErrDraftStale, "draft_stale"}, {fileport.ErrConflict, "conflict"},
		{fileport.ErrPersistence, "persistence_failure"}, {fileport.ErrOperationMismatch, "operation_mismatch"},
		{fileport.ErrOperationNotFound, "operation_not_found"}, {fileport.ErrForbidden, "forbidden"},
		{fileport.ErrNotText, "not_text"}, {fileport.ErrTooLarge, "too_large"}, {editingapp.ErrCapacity, "capacity"},
		{officeediting.ErrUnavailable, "unavailable"}, {editingapp.ErrUnavailable, "unavailable"},
	} {
		if errors.Is(err, known.cause) {
			return known.kind
		}
	}
	return "other"
}

func emitNativeManualBeginPhaseV1(output io.Writer, state nativeManualBeginPhaseV1) {
	if output == nil || state.Version != 1 {
		return
	}
	switch state.Phase {
	case "input", "authority", "dependencies", "capture_conflict", "capture", "baseline_open", "baseline_session", "baseline_revision", "prepare", "prepared_contract":
	default:
		return
	}
	switch state.ErrorKind {
	case "invalid_request", "session_invalid", "scope_invalid", "projection_unavailable", "draft_stale", "conflict", "persistence_failure", "operation_mismatch", "operation_not_found", "forbidden", "not_text", "too_large", "capacity", "unavailable", "capture_unavailable", "unsafe_path", "capture_session_mismatch", "opaque_execution", "baseline_failed", "context_canceled", "context_deadline", "not_found", "permission", "other":
	default:
		return
	}
	body, err := json.Marshal(state)
	if err != nil || len(nativeManualBeginPhasePrefixV1)+len(body)+1 > nativeManualBeginPhaseMaxBytesV1 {
		return
	}
	frame := append([]byte(nativeManualBeginPhasePrefixV1), body...)
	frame = append(frame, '\n')
	// One bounded write prevents interleaving fields. A diagnostic write failure
	// is ignored and never changes the operation's result or cleanup.
	_, _ = output.Write(frame)
}
