package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	fundscleaningapp "analytix.local/runtime-go/internal/app/fundscleaning"
	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	localdisplayapp "analytix.local/runtime-go/internal/app/localdisplay"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const maxLocalDisplayRequestBytesV1 = 16 * 1024

func validDirectSourcePreviewWorkspacePathV1(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value
}

type FrozenSecurityContextLoaderV1 func(threadID, turnID string) (domainsecurity.TurnSecurityContext, error)

// CurrentSecurityContextLoaderV1 loads the latest active context for the
// accepted-final case. DirectSourcePreview uses the service's current-case /
// current-snapshot source seam and does not load a frozen turn context.
type CurrentSecurityContextLoaderV1 func(threadID string) (domainsecurity.TurnSecurityContext, error)

type LocalDisplayMuxV1 struct {
	RuntimeToken string
	Insecure     bool
	Next         http.Handler
	LocalDisplay http.Handler
}

func (mux LocalDisplayMuxV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if MatchRuntimeRoute(r.URL.Path).Route != RouteLocalDisplay {
		if mux.Next == nil {
			WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "runtime_handler_missing", "message": "runtime handler missing"})
			return
		}
		mux.Next.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if !Authorized(r, mux.RuntimeToken, mux.Insecure) {
		WriteJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}
	values := r.Header.Values(LocalDisplayHeaderV1)
	if len(values) != 1 || values[0] != LocalDisplayHeaderValueV1 {
		WriteJSON(w, http.StatusForbidden, map[string]any{"code": "local_display_forbidden", "message": "typed local display authority is required"})
		return
	}
	if mux.LocalDisplay == nil {
		writeLocalDisplayUnavailableV1(w)
		return
	}
	mux.LocalDisplay.ServeHTTP(w, r)
}

func (mux LocalDisplayMuxV1) Shutdown(ctx context.Context) error {
	if handler, ok := mux.LocalDisplay.(LocalDisplayHandlerV1); ok {
		if handler.FundsCleaning != nil {
			_ = handler.FundsCleaning.Close()
		}
		if handler.FundsCSVAdmission != nil {
			_ = handler.FundsCSVAdmission.Close()
		}
	}
	if lifecycle, ok := mux.Next.(interface{ Shutdown(context.Context) error }); ok {
		return lifecycle.Shutdown(ctx)
	}
	return nil
}

type LocalDisplayHandlerV1 struct {
	GeneratedArtifacts http.Handler
	PackageHost        http.Handler
	ObjectEditing      http.Handler
	Service            *localdisplayapp.Service
	FundsCSVAdmission  *fundscsvadmissionapp.ServiceV1
	FundsCleaning      *fundscleaningapp.ServiceV1
	// LoadFrozenSecurityContext is retained for source compatibility with the
	// transitional composition. DirectSourcePreview never reads it.
	LoadFrozenSecurityContext  FrozenSecurityContextLoaderV1
	LoadCurrentSecurityContext CurrentSecurityContextLoaderV1
}

type directSourcePreviewRequestV1 struct {
	Kind          string   `json:"kind"`
	WorkspaceRoot string   `json:"workspaceRoot"`
	View          string   `json:"view"`
	Fields        []string `json:"fields"`
	RowOffset     uint32   `json:"rowOffset"`
	RowLimit      uint16   `json:"rowLimit"`
	DisplayMode   string   `json:"displayMode"`
}

type acceptedSlotDisplayRequestV1 struct {
	Kind                string `json:"kind"`
	ThreadID            string `json:"threadId"`
	TurnID              string `json:"turnId"`
	AcceptedFinalDigest string `json:"acceptedFinalDigest"`
	DisplayMode         string `json:"displayMode"`
}

type importMappingPreviewRequestV1 struct {
	Kind        string   `json:"kind"`
	Selector    string   `json:"selector"`
	Fields      []string `json:"fields"`
	RowOffset   uint32   `json:"rowOffset"`
	RowLimit    uint16   `json:"rowLimit"`
	DisplayMode string   `json:"displayMode"`
}

type cleaningDiffPreviewRequestV1 struct {
	Kind        string   `json:"kind"`
	Selector    string   `json:"selector"`
	Fields      []string `json:"fields"`
	RowOffset   uint32   `json:"rowOffset"`
	RowLimit    uint16   `json:"rowLimit"`
	DisplayMode string   `json:"displayMode"`
}

type fundsImportStageRequestV1 struct {
	WorkspaceRoot    string `json:"workspaceRoot"`
	SourcePath       string `json:"sourcePath"`
	CreateCaseIntent string `json:"createCaseIntent,omitempty"`
}

type fundsImportSelectorRequestV1 struct {
	Selector string `json:"selector"`
}

type fundsDeterministicCleaningRequestV1 struct {
	WorkspaceRoot string `json:"workspaceRoot"`
}

func (handler LocalDisplayHandlerV1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == GeneratedArtifactPath {
		if handler.GeneratedArtifacts == nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		handler.GeneratedArtifacts.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == PluginPackageHostPath {
		if handler.PackageHost == nil {
			PluginPackageHostHandler{}.ServeHTTP(w, r)
			return
		}
		handler.PackageHost.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == ObjectEditingPath {
		if handler.ObjectEditing == nil {
			ObjectEditingHandler{}.ServeHTTP(w, r)
			return
		}
		handler.ObjectEditing.ServeHTTP(w, r)
		return
	}
	if r != nil && r.URL != nil && r.URL.Path == HostFundsImportStagePathV1 && handler.FundsCSVAdmission == nil {
		writeFundsImportCapabilityUnavailableV1(w)
		return
	}
	if handler.Service == nil {
		if r != nil && r.URL != nil && r.URL.Path == HostFundsDeterministicCleaningPathV1 {
			writeFundsCleaningPreCASFailedV1(w)
			return
		}
		writeLocalDisplayUnavailableV1(w)
		return
	}
	switch r.URL.Path {
	case HostFundsDeterministicCleaningPathV1:
		if handler.FundsCleaning == nil {
			writeFundsCleaningPreCASFailedV1(w)
			return
		}
		var request fundsDeterministicCleaningRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			!validDirectSourcePreviewWorkspacePathV1(request.WorkspaceRoot) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		response, err := handler.FundsCleaning.RunV1(
			r.Context(),
			fundscleaningapp.RunInputV1{WorkspaceRoot: request.WorkspaceRoot},
		)
		writeFundsCleaningRunResponseV1(w, response, err)
	case HostFundsCleaningRevokePathV1:
		if handler.FundsCleaning == nil || handler.Service == nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		var request fundsImportSelectorRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			!domainlocaldisplay.ValidSelectorV1(request.Selector) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		if handler.FundsCleaning.RevokeCleaningDiffPreviewV1(r.Context(), request.Selector) != nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
	case HostFundsImportStagePathV1:
		var request fundsImportStageRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			!validDirectSourcePreviewWorkspacePathV1(request.WorkspaceRoot) ||
			!validDirectSourcePreviewWorkspacePathV1(request.SourcePath) ||
			(request.CreateCaseIntent != "" && !domainsecurity.IsSHA256Hex(request.CreateCaseIntent)) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		response, err := handler.FundsCSVAdmission.StageMainSelectedImportV1(
			r.Context(),
			fundscsvadmissionapp.StageInputV1{
				WorkspaceRoot:    request.WorkspaceRoot,
				SourcePath:       request.SourcePath,
				CreateCaseIntent: request.CreateCaseIntent,
			},
		)
		var creation *fundscsvadmissionapp.CaseCreationRequiredV1
		if errors.As(err, &creation) && domainsecurity.IsSHA256Hex(creation.Intent) {
			// This typed host-only negotiation has not read or imported source
			// bytes. Main must obtain explicit confirmation before resubmitting.
			WriteJSON(w, http.StatusAccepted, map[string]string{"status": "case_creation_required", "intent": creation.Intent})
			return
		}
		if errors.Is(err, fundscsvadmissionapp.ErrInvalidRequest) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		if err != nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		WriteJSON(w, http.StatusOK, response)
	case HostFundsImportConfirmPathV1, HostFundsImportCancelPathV1, HostFundsImportStatusPathV1:
		if handler.FundsCSVAdmission == nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		var request fundsImportSelectorRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			!domainlocaldisplay.ValidSelectorV1(request.Selector) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		var response any
		var err error
		switch r.URL.Path {
		case HostFundsImportConfirmPathV1:
			response, err = handler.FundsCSVAdmission.ConfirmImportV1(r.Context(), request.Selector)
		case HostFundsImportCancelPathV1:
			response, err = handler.FundsCSVAdmission.CancelImportV1(r.Context(), request.Selector)
		case HostFundsImportStatusPathV1:
			response, err = handler.FundsCSVAdmission.ImportStatusV1(r.Context(), request.Selector)
		}
		if errors.Is(err, fundscsvadmissionapp.ErrInvalidRequest) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		if err != nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		WriteJSON(w, http.StatusOK, response)
	case LocalDisplayImportMappingPathV1:
		var request importMappingPreviewRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			request.Kind != domainlocaldisplay.KindImportMappingPreviewV1 {
			writeLocalDisplayInvalidV1(w)
			return
		}
		response, err := handler.Service.ImportMappingPreview(r.Context(), localdisplayapp.ImportMappingPreviewInputV1{
			Selector: request.Selector, Fields: append([]string(nil), request.Fields...),
			RowOffset: request.RowOffset, RowLimit: request.RowLimit, DisplayMode: request.DisplayMode,
		})
		if err != nil {
			writeLocalDisplayServiceErrorV1(w, err)
			return
		}
		writeLocalDisplayResponseV1(w, response)
	case LocalDisplayCleaningDiffPathV1:
		var request cleaningDiffPreviewRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			request.Kind != domainlocaldisplay.KindCleaningDiffPreviewV1 {
			writeLocalDisplayInvalidV1(w)
			return
		}
		response, err := handler.Service.CleaningDiffPreview(r.Context(), localdisplayapp.CleaningDiffPreviewInputV1{
			Selector: request.Selector, Fields: append([]string(nil), request.Fields...),
			RowOffset: request.RowOffset, RowLimit: request.RowLimit, DisplayMode: request.DisplayMode,
		})
		if err != nil {
			writeLocalDisplayServiceErrorV1(w, err)
			return
		}
		writeLocalDisplayResponseV1(w, response)
	case LocalDisplayDirectPreviewPathV1:
		var request directSourcePreviewRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			request.Kind != domainlocaldisplay.KindDirectSourcePreviewV1 {
			writeLocalDisplayInvalidV1(w)
			return
		}
		if !validDirectSourcePreviewWorkspacePathV1(request.WorkspaceRoot) {
			writeLocalDisplayInvalidV1(w)
			return
		}
		response, err := handler.Service.DirectSourcePreview(r.Context(), localdisplayapp.DirectSourcePreviewInputV1{
			WorkspaceRoot: request.WorkspaceRoot,
			View:          request.View,
			Fields:        append([]string(nil), request.Fields...),
			RowOffset:     request.RowOffset,
			RowLimit:      request.RowLimit,
			DisplayMode:   request.DisplayMode,
		})
		if err != nil {
			writeLocalDisplayServiceErrorV1(w, err)
			return
		}
		writeLocalDisplayResponseV1(w, response)
	case LocalDisplayAcceptedSlotsPathV1:
		var request acceptedSlotDisplayRequestV1
		if decodeLocalDisplayRequestV1(r, &request) != nil ||
			request.Kind != domainlocaldisplay.KindAcceptedSlotDisplayV1 {
			writeLocalDisplayInvalidV1(w)
			return
		}
		if handler.LoadCurrentSecurityContext == nil {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		threadID := strings.TrimSpace(request.ThreadID)
		activeSecurityContext, err := handler.LoadCurrentSecurityContext(threadID)
		if err != nil ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(activeSecurityContext) != nil ||
			activeSecurityContext.ThreadID != threadID {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		response, err := handler.Service.AcceptedSlotDisplay(r.Context(), localdisplayapp.AcceptedSlotDisplayInputV1{
			ThreadID:              threadID,
			TurnID:                strings.TrimSpace(request.TurnID),
			AcceptedFinalDigest:   strings.TrimSpace(request.AcceptedFinalDigest),
			DisplayMode:           strings.TrimSpace(request.DisplayMode),
			ActiveSecurityContext: activeSecurityContext,
		})
		if err != nil {
			writeLocalDisplayServiceErrorV1(w, err)
			return
		}
		currentSecurityContext, err := handler.LoadCurrentSecurityContext(threadID)
		if err != nil ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(currentSecurityContext) != nil ||
			currentSecurityContext != activeSecurityContext {
			writeLocalDisplayUnavailableV1(w)
			return
		}
		writeLocalDisplayResponseV1(w, response)
	default:
		WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func writeFundsCleaningRunResponseV1(
	w http.ResponseWriter,
	response fundscleaningapp.RunResultV1,
	err error,
) {
	if errors.Is(err, fundscleaningapp.ErrInvalidRequest) {
		writeLocalDisplayInvalidV1(w)
		return
	}
	if errors.Is(err, fundscleaningapp.ErrPreCASFailed) {
		writeFundsCleaningPreCASFailedV1(w)
		return
	}
	if err != nil {
		writeLocalDisplayUnavailableV1(w)
		return
	}
	WriteJSON(w, http.StatusOK, response)
}

func writeLocalDisplayResponseV1(w http.ResponseWriter, response any) {
	body, err := json.Marshal(response)
	if err != nil || len(body) > domainlocaldisplay.MaximumResponseBytesV1 {
		writeLocalDisplayUnavailableV1(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func decodeLocalDisplayRequestV1(r *http.Request, target any) error {
	if r == nil || r.Body == nil || target == nil || r.ContentLength > maxLocalDisplayRequestBytesV1 {
		return errors.New("local display request is invalid")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxLocalDisplayRequestBytesV1+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("local display request has trailing JSON")
	}
	return nil
}

func writeLocalDisplayServiceErrorV1(w http.ResponseWriter, err error) {
	if errors.Is(err, localdisplayapp.ErrInvalidRequest) {
		writeLocalDisplayInvalidV1(w)
		return
	}
	writeLocalDisplayUnavailableV1(w)
}

func writeLocalDisplayInvalidV1(w http.ResponseWriter) {
	WriteJSON(w, http.StatusBadRequest, map[string]any{
		"code": "local_display_invalid", "message": "Typed local display request is invalid.",
	})
}

func writeLocalDisplayUnavailableV1(w http.ResponseWriter) {
	WriteJSON(w, http.StatusConflict, map[string]any{
		"code": "local_display_unavailable", "message": "Typed local display authority is unavailable.",
	})
}

func writeFundsCleaningPreCASFailedV1(w http.ResponseWriter) {
	WriteJSON(w, http.StatusOK, map[string]any{"status": "pre_cas_failed"})
}

// This fixed host-private capability response is not a generic error bypass:
// no caller/provider body or cause can enter it. LocalDisplayMuxV1 supplies the
// existing bearer-token and typed-header authorization boundary.
func writeFundsImportCapabilityUnavailableV1(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(w, `{"code":"funds_import_capability_unavailable","message":"Trusted data import capability is unavailable in this runtime environment."}`)
}
