//go:build !analytix_prod

package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	agent "analytix.local/runtime-go/internal/agent"
	contracts "analytix.local/runtime-go/internal/contracts"
)

const LiveLocalG4ManagerPrefix = "/v1/conformance/g4/manager"
const defaultRuntimeToken = "tok-1"

type LiveLocalG4ReplayStore interface {
	NoteG4ApprovalReplay()
	NoteG4UserInputReplay()
	NoteG4MCPReplay()
	NoteG4ValidationReplay()
}

type LiveLocalG4ManagerHandler struct {
	runtimeToken      string
	contract          G4ToolsConformanceContract
	approvalUserInput agent.ApprovalUserInputRouteContract
	mcp               MCPToolLifecycleContract
	store             LiveLocalG4ReplayStore
	enabled           bool

	mu               sync.Mutex
	approvalResolved bool
	cancelResolved   bool
	submitResolved   bool
}

func NewLiveLocalG4ManagerHandler(
	runtimeToken string,
	contract G4ToolsConformanceContract,
	approvalUserInput agent.ApprovalUserInputRouteContract,
	mcp MCPToolLifecycleContract,
	store LiveLocalG4ReplayStore,
) *LiveLocalG4ManagerHandler {
	if strings.TrimSpace(runtimeToken) == "" {
		runtimeToken = defaultRuntimeToken
	}
	return &LiveLocalG4ManagerHandler{
		runtimeToken:      runtimeToken,
		contract:          contract,
		approvalUserInput: approvalUserInput,
		mcp:               mcp,
		store:             store,
		enabled:           contract.Mode != "" || approvalUserInput.ID != "" || mcp.ID != "",
	}
}

func (h *LiveLocalG4ManagerHandler) Handles(path string) bool {
	return h != nil && h.enabled && strings.HasPrefix(path, LiveLocalG4ManagerPrefix+"/")
}

func (h *LiveLocalG4ManagerHandler) Enabled() bool {
	return h != nil && h.enabled
}

func (h *LiveLocalG4ManagerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, h.runtimeToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": "unauthorized", "message": "unauthorized"})
		return
	}

	switch r.URL.Path {
	case LiveLocalG4ManagerPrefix + "/boundary":
		h.handleBoundary(w, r)
	case LiveLocalG4ManagerPrefix + "/summary":
		h.handleSummary(w, r)
	case LiveLocalG4ManagerPrefix + "/tool-catalog":
		h.handleToolCatalog(w, r)
	case LiveLocalG4ManagerPrefix + "/approval/decision":
		h.handleApprovalDecision(w, r)
	case LiveLocalG4ManagerPrefix + "/approval/replay":
		h.handleApprovalReplay(w, r)
	case LiveLocalG4ManagerPrefix + "/user-input/cancel":
		h.handleUserInputCancel(w, r)
	case LiveLocalG4ManagerPrefix + "/user-input/submit":
		h.handleUserInputSubmit(w, r)
	case LiveLocalG4ManagerPrefix + "/user-input/validate":
		h.handleUserInputValidation(w, r)
	case LiveLocalG4ManagerPrefix + "/user-input/replay":
		h.handleUserInputReplay(w, r)
	case LiveLocalG4ManagerPrefix + "/remote-entry":
		h.handleRemoteEntry(w, r)
	case LiveLocalG4ManagerPrefix + "/mcp/lifecycle":
		h.handleMCPLifecycle(w, r)
	case LiveLocalG4ManagerPrefix + "/mcp/approval-annotations":
		h.handleMCPApprovalAnnotations(w, r)
	case LiveLocalG4ManagerPrefix + "/mcp/search":
		h.handleMCPSearch(w, r)
	case LiveLocalG4ManagerPrefix + "/mcp/call-reconnect":
		h.handleMCPCallReconnect(w, r)
	case LiveLocalG4ManagerPrefix + "/mcp/background-reconnect":
		h.handleMCPBackgroundReconnect(w, r)
	case LiveLocalG4ManagerPrefix + "/mcp/diagnostics":
		h.handleMCPDiagnostics(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "route not found"})
	}
}

func (h *LiveLocalG4ManagerHandler) handleBoundary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                true,
		"testConformanceOnly":        true,
		"approvalExecutionAllowed":   false,
		"toolExecutionAllowed":       false,
		"mcpConnectionAllowed":       false,
		"mcpCredentialsAllowed":      false,
		"credentialReadAllowed":      false,
		"fileMutationAllowed":        false,
		"realWorkspaceWriteAllowed":  false,
		"eventsJsonlMutationAllowed": false,
		"productBoundary":            contracts.LiveLocalSidecarProductBoundary(),
	})
}

func (h *LiveLocalG4ManagerHandler) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4ValidationReplay()
	writeJSON(w, http.StatusOK, BuildG4ToolsConformanceOutput(h.contract))
}

func (h *LiveLocalG4ManagerHandler) handleToolCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4ValidationReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":             true,
		"toolNames":               append([]string(nil), h.contract.ToolCatalog.AdvertisedToolNames...),
		"canonicalOrderStable":    h.contract.ToolCatalog.CanonicalOrderStable,
		"forbiddenTopLevelRoutes": append([]string(nil), h.contract.ToolCatalog.ForbiddenTopLevelRoutes...),
		"plannerReadOnlyToolset":  append([]string(nil), h.contract.PlannerExecutor.PlannerReadOnlyToolset...),
	})
}

func (h *LiveLocalG4ManagerHandler) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body, ok := requestBody(w, r)
	if !ok {
		return
	}
	if !liveLocalBodyMatches(body, h.approvalUserInput.Approval.DecisionRequest) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "approval decision body does not match fixture"})
		return
	}

	h.mu.Lock()
	if h.approvalResolved {
		h.mu.Unlock()
		writeJSON(w, h.approvalUserInput.Approval.SecondDecisionStatus, map[string]any{
			"code":       "approval_already_resolved",
			"approvalId": h.approvalUserInput.Approval.ID,
		})
		return
	}
	h.approvalResolved = true
	h.store.NoteG4ApprovalReplay()
	h.mu.Unlock()

	writeJSON(w, h.approvalUserInput.Approval.ExpectedResponse.Status, h.approvalUserInput.Approval.ExpectedResponse.Body)
}

func (h *LiveLocalG4ManagerHandler) handleApprovalReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4ApprovalReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":          true,
		"approvalId":           h.approvalUserInput.Approval.ID,
		"toolName":             h.approvalUserInput.Approval.ToolName,
		"decision":             h.approvalUserInput.Approval.DecisionRequest.Decision,
		"status":               stringField(h.approvalUserInput.Approval.ExpectedResponse.Body, "status"),
		"pendingBefore":        h.approvalUserInput.Approval.PendingBefore,
		"pendingAfter":         h.approvalUserInput.Approval.PendingAfter,
		"secondDecisionStatus": h.approvalUserInput.Approval.SecondDecisionStatus,
		"deniedNoExecute":      h.contract.Approval.MustNotExecuteDeniedTool,
		"eventKinds":           []string{"approval_requested", "approval_resolved"},
		"replayKindsInOrder":   append([]string(nil), h.approvalUserInput.Replay.ExpectedKindsInOrder...),
	})
}

func (h *LiveLocalG4ManagerHandler) handleUserInputCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body, ok := requestBody(w, r)
	if !ok {
		return
	}
	if !liveLocalBodyMatches(body, h.approvalUserInput.UserInput.ResolveRequest) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "user-input cancel body does not match fixture"})
		return
	}

	h.mu.Lock()
	if h.cancelResolved {
		h.mu.Unlock()
		writeJSON(w, h.approvalUserInput.UserInput.SecondResolveStatus, map[string]any{
			"code":    "user_input_not_pending",
			"inputId": h.approvalUserInput.UserInput.ID,
		})
		return
	}
	h.cancelResolved = true
	h.store.NoteG4UserInputReplay()
	h.mu.Unlock()

	writeJSON(w, h.approvalUserInput.UserInput.ExpectedResponse.Status, h.approvalUserInput.UserInput.ExpectedResponse.Body)
}

func (h *LiveLocalG4ManagerHandler) handleUserInputSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body, ok := requestBody(w, r)
	if !ok {
		return
	}
	if !liveLocalBodyMatches(body, h.approvalUserInput.SubmittedUserInput.ResolveRequest) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "user-input submit body does not match fixture"})
		return
	}

	h.mu.Lock()
	if h.submitResolved {
		h.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{
			"code":    "user_input_not_pending",
			"inputId": h.approvalUserInput.SubmittedUserInput.ID,
		})
		return
	}
	h.submitResolved = true
	h.store.NoteG4UserInputReplay()
	h.mu.Unlock()

	writeJSON(w, h.approvalUserInput.SubmittedUserInput.ExpectedResponse.Status, h.approvalUserInput.SubmittedUserInput.ExpectedResponse.Body)
}

func (h *LiveLocalG4ManagerHandler) handleUserInputValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	body, ok := requestBody(w, r)
	if !ok {
		return
	}
	var request struct {
		CaseID string `json:"caseId"`
	}
	if err := json.Unmarshal(body, &request); err != nil || strings.TrimSpace(request.CaseID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "caseId is required"})
		return
	}
	if !liveLocalStringInSlice(request.CaseID, h.contract.UserInput.StructuredChoiceValidation.InvalidCases) {
		writeJSON(w, http.StatusNotFound, map[string]any{"code": "not_found", "message": "validation case not found"})
		return
	}
	h.store.NoteG4ValidationReplay()
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"code":         h.contract.UserInput.StructuredChoiceValidation.InvalidResultCode,
		"caseId":       request.CaseID,
		"opensGate":    h.contract.UserInput.StructuredChoiceValidation.OpensGateOnInvalid,
		"pendingAfter": 0,
		"fixtureOnly":  true,
	})
}

func (h *LiveLocalG4ManagerHandler) handleUserInputReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4UserInputReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                      true,
		"cancelledInputId":                 h.approvalUserInput.UserInput.ID,
		"cancelledStatus":                  stringField(h.approvalUserInput.UserInput.ExpectedResponse.Body, "status"),
		"cancelPendingBefore":              h.approvalUserInput.UserInput.PendingBefore,
		"cancelPendingAfter":               h.approvalUserInput.UserInput.PendingAfter,
		"secondResolveStatus":              h.approvalUserInput.UserInput.SecondResolveStatus,
		"submittedInputId":                 h.approvalUserInput.SubmittedUserInput.ID,
		"submittedStatus":                  h.approvalUserInput.SubmittedUserInput.ExpectedResolution.Status,
		"submittedAnswersEchoed":           h.contract.UserInput.SubmittedRoute.AnswersEchoed,
		"submittedAnswerCount":             len(h.approvalUserInput.SubmittedUserInput.ExpectedResolution.Answers),
		"resolvedEventKind":                h.approvalUserInput.SubmittedUserInput.ResolvedEvent.Kind,
		"resolvedEventStatus":              h.approvalUserInput.SubmittedUserInput.ResolvedEvent.Status,
		"resolvedEventIncludesAnswers":     h.approvalUserInput.SubmittedUserInput.ResolvedEvent.IncludesAnswers,
		"resolvedEventOmitsAnswers":        !h.approvalUserInput.SubmittedUserInput.ResolvedEvent.IncludesAnswers,
		"remoteDisableUserInputPreserved":  h.contract.UserInput.RemoteDisableUserInput,
		"structuredValidationInvalidCases": append([]string(nil), h.contract.UserInput.StructuredChoiceValidation.InvalidCases...),
	})
}

func (h *LiveLocalG4ManagerHandler) handleRemoteEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4ValidationReplay()
	disableUserInput, _ := h.approvalUserInput.RemoteEntry.StartRequest["disableUserInput"].(bool)
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                    true,
		"threadId":                       h.approvalUserInput.RemoteEntry.ThreadID,
		"disableUserInputPreserved":      disableUserInput,
		"expectedPortKeys":               append([]string(nil), h.approvalUserInput.RemoteEntry.ExpectedPortKeys...),
		"forbiddenPortKeys":              append([]string(nil), h.approvalUserInput.RemoteEntry.ForbiddenPortKeys...),
		"contractExpectedPortKeys":       append([]string(nil), h.contract.RemoteEntry.ExpectedPortKeys...),
		"contractForbiddenPortKeys":      append([]string(nil), h.contract.RemoteEntry.ForbiddenPortKeys...),
		"reasonixControlPlaneExposed":    false,
		"rejectedApprovalPolicyOverride": h.approvalUserInput.RemoteEntry.RejectedOverride["approvalPolicy"],
	})
}

func (h *LiveLocalG4ManagerHandler) handleMCPLifecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4MCPReplay()
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":          true,
		"providerId":           h.mcp.ProviderID,
		"connectToolNames":     append([]string(nil), h.mcp.Connect.ToolNames...),
		"connectDiagnostic":    h.mcp.Connect.Diagnostic,
		"reloadToolNames":      append([]string(nil), h.mcp.Reload.ToolNames...),
		"schemaOrderStable":    h.mcp.Reload.SchemaOrderStable,
		"disconnectReason":     h.mcp.Disconnect.Reason,
		"disconnectToolNames":  append([]string(nil), h.mcp.Disconnect.ToolNames...),
		"disconnectDiagnostic": h.mcp.Disconnect.Diagnostic,
		"cancelErrorSubstring": h.mcp.Cancel.ErrorSubstring,
		"cancelExecuted":       h.mcp.Cancel.Executed,
		"errorCode":            h.mcp.Error.Code,
		"errorApproved":        h.mcp.Error.Approved,
		"mcpConnectionUsed":    false,
		"credentialRead":       false,
	})
}

func (h *LiveLocalG4ManagerHandler) handleMCPApprovalAnnotations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4MCPReplay()
	approval := h.mcp.ApprovalAnnotations
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":        true,
		"serverId":           approval.ServerID,
		"toolName":           approval.ToolName,
		"normalizedToolName": approval.NormalizedToolName,
		"destructiveHint":    approval.Annotations.DestructiveHint,
		"openWorldHint":      approval.Annotations.OpenWorldHint,
		"approvalId":         approval.ApprovalID,
		"decision":           approval.Decision,
		"resultKind":         approval.ResultKind,
		"executed":           approval.Executed,
		"deniedNoExecute":    !approval.Executed,
	})
}

func (h *LiveLocalG4ManagerHandler) handleMCPSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4MCPReplay()
	search := h.mcp.SearchMetaTools
	workspace := r.URL.Query().Get("workspace")
	untrusted := workspace == search.UntrustedWorkspace
	trusted := workspace == "" || workspace == search.TrustedWorkspace
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":                true,
		"toolNames":                  append([]string(nil), search.ToolNames...),
		"toolCount":                  len(search.ToolNames),
		"refreshToolAdvertised":      liveLocalStringInSlice("mcp_refresh_catalog", search.ToolNames),
		"trustedWorkspace":           search.TrustedWorkspace,
		"untrustedWorkspace":         search.UntrustedWorkspace,
		"trustedWorkspaceSelected":   trusted,
		"untrustedWorkspaceSelected": untrusted,
		"trustedToolId":              search.TrustedToolID,
		"untrustedSearchedTools":     search.UntrustedSearchedTools,
		"unknownToolError":           search.UnknownToolError,
		"callPolicy":                 search.CallPolicy,
		"deniedCallExecuted":         search.DeniedCallExecuted,
		"deniedNoExecute":            !search.DeniedCallExecuted,
		"refreshDrift": map[string]any{
			"serverId":      search.RefreshDrift.ServerID,
			"initialTools":  append([]string(nil), search.RefreshDrift.InitialToolNames...),
			"expandedTools": append([]string(nil), search.RefreshDrift.ExpandedToolNames...),
			"totalIndexed":  search.RefreshDrift.ExpectedTotalIndexed,
			"catalogDrift":  search.RefreshDrift.ExpectedCatalogDrift,
		},
	})
}

func (h *LiveLocalG4ManagerHandler) handleMCPCallReconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4MCPReplay()
	reconnect := h.mcp.CallReconnect
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":               true,
		"serverId":                  reconnect.ServerID,
		"toolName":                  reconnect.ToolName,
		"normalizedToolName":        reconnect.NormalizedToolName,
		"transportError":            reconnect.TransportError,
		"protocolError":             reconnect.ProtocolError,
		"transportErrorRetried":     reconnect.RetryOnTransportError && reconnect.StaleConnection.FactoryAttempts == reconnect.MaxAttempts,
		"protocolErrorRetried":      reconnect.RetryOnProtocolError,
		"maxAttempts":               reconnect.MaxAttempts,
		"staleFactoryAttempts":      reconnect.StaleConnection.FactoryAttempts,
		"staleCloseCount":           reconnect.StaleConnection.CloseCount,
		"staleResultInstance":       reconnect.StaleConnection.ResultInstance,
		"staleCallSucceeded":        !reconnect.StaleConnection.IsError,
		"protocolFactoryAttempts":   reconnect.ProtocolFailure.FactoryAttempts,
		"protocolCloseCount":        reconnect.ProtocolFailure.CloseCount,
		"protocolErrorCode":         reconnect.ProtocolFailure.Code,
		"protocolCallReturnedError": reconnect.ProtocolFailure.IsError,
		"mcpConnectionUsed":         false,
	})
}

func (h *LiveLocalG4ManagerHandler) handleMCPBackgroundReconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4MCPReplay()
	background := h.mcp.BackgroundReconnect
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":              true,
		"failedServerIds":          append([]string(nil), background.FailedServerIDs...),
		"suspendedProviderId":      background.SuspendedProviderID,
		"suspendedReason":          background.SuspendedReason,
		"connectedServerIds":       append([]string(nil), background.ExpectedConnectedServerIDs...),
		"errorServerIds":           append([]string(nil), background.ExpectedErrorServerIDs...),
		"attemptsPerFailedServer":  background.AttemptsPerFailedServer,
		"requiresRuntimeRestart":   background.RequiresRuntimeRestart,
		"backgroundClassification": "fixture-reconnect-classification",
		"mcpConnectionUsed":        false,
	})
}

func (h *LiveLocalG4ManagerHandler) handleMCPDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	h.store.NoteG4MCPReplay()
	redaction := h.mcp.DiagnosticsRedaction
	if redaction.Secret == "" {
		redaction = h.contract.MCP.DiagnosticsRedaction
	}
	secretSafeDiagnostic := strings.ReplaceAll("Authorization="+redaction.Secret, redaction.Secret, redaction.Replacement)
	diagnostics := make([]map[string]any, 0, len(h.mcp.KnownOverrideVars))
	for _, item := range h.mcp.KnownOverrideVars {
		diagnostics = append(diagnostics, map[string]any{
			"serverId":            item.ServerID,
			"knownOverride":       item.Diagnostic.KnownOverride,
			"effectiveCwd":        item.Diagnostic.EffectiveCWD,
			"workspaceRoot":       item.WorkspaceRoot,
			"lowPriority":         item.Diagnostic.LowPriority,
			"backgroundStart":     item.Diagnostic.BackgroundStart,
			"explicitCwd":         item.ExplicitCWD,
			"daemonIdleTimeoutMs": item.DaemonIdleTimeoutMs,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fixtureOnly":          true,
		"knownOverride":        h.mcp.KnownOverride.Diagnostic.KnownOverride,
		"effectiveCwd":         h.mcp.KnownOverride.Diagnostic.EffectiveCWD,
		"lowPriority":          h.mcp.KnownOverride.Diagnostic.LowPriority,
		"backgroundStart":      h.mcp.KnownOverride.Diagnostic.BackgroundStart,
		"variantCount":         len(diagnostics),
		"diagnostics":          diagnostics,
		"redactionReplacement": redaction.Replacement,
		"secretSafeDiagnostic": secretSafeDiagnostic,
		"secretLeaked":         strings.Contains(secretSafeDiagnostic, redaction.Secret),
		"credentialRead":       false,
	})
}

func liveLocalBodyMatches(body json.RawMessage, expected any) bool {
	data, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	return canonicalJSON(body) == canonicalJSON(data)
}

func liveLocalStringInSlice(needle string, values []string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func authorized(r *http.Request, token string) bool {
	return r.Header.Get("Authorization") == "Bearer "+token
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requestBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	if r.Body == nil {
		return nil, true
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "validation_error", "message": "invalid request body"})
		return nil, false
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, true
	}
	return json.RawMessage(data), true
}

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}
