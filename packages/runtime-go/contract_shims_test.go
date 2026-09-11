//go:build !analytix_prod

package runtimego

import (
	"encoding/json"
	"net/http"
	"net/url"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	agent "analytix.local/runtime-go/internal/agent"
	conformance "analytix.local/runtime-go/internal/conformance"
	livelocal "analytix.local/runtime-go/internal/conformance/livelocal"
	contracts "analytix.local/runtime-go/internal/contracts"
	mcpdomain "analytix.local/runtime-go/internal/mcp"
	"analytix.local/runtime-go/internal/protocol"
	provider "analytix.local/runtime-go/internal/provider"
)

const defaultRuntimeToken = httpapi.DefaultRuntimeToken

type G1ShadowConfig = livelocal.G1ShadowConfig
type G2ShadowConfig = livelocal.G2ShadowConfig
type G2RouteReplayCase = protocol.G2RouteReplayCase
type G2RouteResponse = protocol.G2RouteResponse

type LiveLocalSidecarConfig = livelocal.LiveLocalSidecarConfig
type LiveLocalSidecarHarness = livelocal.LiveLocalSidecarHarness
type LiveLocalSidecarSnapshot = livelocal.LiveLocalSidecarSnapshot

type G3ProviderConformanceContract = provider.G3ProviderConformanceContract
type G3ProviderUsageCaseSummary = provider.G3ProviderUsageCaseSummary
type G3ProviderRequestShapeCase = provider.G3ProviderRequestShapeCase
type G3ProviderUsageSummary = provider.G3ProviderUsageSummary
type G3ProviderCacheAccounting = provider.G3ProviderCacheAccounting
type G3ProviderStreamingConformance = provider.G3ProviderStreamingConformance
type G3ProviderCacheDiagnostics = provider.G3ProviderCacheDiagnostics
type ProviderPrefixShape = provider.ProviderPrefixShape
type ProviderDriftAttribution = provider.ProviderDriftAttribution
type G5ProviderDriftAttribution = provider.G5ProviderDriftAttribution

type MCPDiagnosticsRedaction = mcpdomain.DiagnosticsRedaction
type MCPProviderCancel = mcpdomain.ProviderCancel
type G4ToolsConformanceContract = mcpdomain.G4ToolsConformanceContract
type G4ToolCatalogConformance = mcpdomain.G4ToolCatalogConformance
type G4ApprovalConformance = mcpdomain.G4ApprovalConformance
type G4UserInputConformance = mcpdomain.G4UserInputConformance
type G4UserInputStructuredChoiceValidation = mcpdomain.G4UserInputStructuredChoiceValidation
type G4UserInputSubmittedRoute = mcpdomain.G4UserInputSubmittedRoute
type G4MCPConformance = mcpdomain.G4MCPConformance
type G4MCPKnownOverrideDiagnostic = mcpdomain.G4MCPKnownOverrideDiagnostic
type G4MCPApprovalAnnotations = mcpdomain.G4MCPApprovalAnnotations
type G4MCPSearchMetaTools = mcpdomain.G4MCPSearchMetaTools
type G4PlannerExecutorConformance = mcpdomain.G4PlannerExecutorConformance
type G4RemoteEntryBoundary = mcpdomain.G4RemoteEntryBoundary

type G5FullLoopConformanceContract = conformance.G5FullLoopConformanceContract
type G5ShadowSourceFixtures = conformance.G5ShadowSourceFixtures
type TaskJobOrchestrationContract = conformance.TaskJobOrchestrationContract
type ProviderCacheContract = conformance.ProviderCacheContract
type ApprovalUserInputRouteContract = conformance.ApprovalUserInputRouteContract
type MCPToolLifecycleContract = conformance.MCPToolLifecycleContract
type G5ControlExecutableCases = conformance.G5ControlExecutableCases
type G5ControlExecutableOutput = conformance.G5ControlExecutableOutput

type GoMinimalAgentLoopContract = agent.GoMinimalAgentLoopContract
type GoMinimalAgentLoopControl = agent.GoMinimalAgentLoopControl
type GoMinimalAgentLoopControlEventSet = agent.GoMinimalAgentLoopControlEventSet
type GoMinimalAgentLoopExpectedConformity = agent.GoMinimalAgentLoopExpectedConformity
type GoKernelLiveScaffoldContract = livelocal.GoKernelLiveScaffoldContract
type GoKernelComponent = livelocal.GoKernelComponent
type GoKernelAbsorptionDecision = livelocal.GoKernelAbsorptionDecision
type GoKernelJobOrchestration = livelocal.GoKernelJobOrchestration
type GoKernelRetirementCleanup = livelocal.GoKernelRetirementCleanup
type GoKernelRetirementPath = livelocal.GoKernelRetirementPath
type GoKernelForbiddenRedundancy = livelocal.GoKernelForbiddenRedundancy
type GoKernelLiveScaffoldExpected = livelocal.GoKernelLiveScaffoldExpected

type liveLocalG2MutableHandler = livelocal.LiveLocalG2MutableHandler
type liveLocalG2Store = livelocal.LiveLocalG2Store
type liveLocalG3ProviderHandler = provider.LiveLocalG3ProviderHandler
type liveLocalG4ManagerHandler = mcpdomain.LiveLocalG4ManagerHandler
type liveLocalDurableHandler = livelocal.LiveLocalDurableHandler
type liveLocalLoopHandler = livelocal.LiveLocalLoopHandler
type liveLocalKernelHandler = livelocal.LiveLocalKernelHandler

func G1ShadowProductBoundary() ProductBoundary {
	return contracts.G1ShadowProductBoundary()
}

func G2ShadowProductBoundary() ProductBoundary {
	return contracts.G1ShadowProductBoundary()
}

func LiveLocalSidecarProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarProductBoundary()
}

func LiveLocalSidecarDurableProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarDurableProductBoundary()
}

func LiveLocalSidecarLoopProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarLoopProductBoundary()
}

func LiveLocalSidecarKernelProductBoundary() LiveLocalSidecarBoundary {
	return contracts.LiveLocalSidecarKernelProductBoundary()
}

func NewG1ShadowHandler(config G1ShadowConfig) http.Handler {
	return livelocal.NewG1ShadowHandler(config)
}

func NewG2ShadowHandler(config G2ShadowConfig) http.Handler {
	return livelocal.NewG2ShadowHandler(config)
}

func NewLiveLocalSidecarHandler(config LiveLocalSidecarConfig) http.Handler {
	return livelocal.NewLiveLocalSidecarHandler(config)
}

func NewLiveLocalSidecarHarness(config LiveLocalSidecarConfig) *LiveLocalSidecarHarness {
	return livelocal.NewLiveLocalSidecarHarness(config)
}

func authorized(r *http.Request, token string) bool {
	return httpapi.Authorized(r, token)
}

func methodNotAllowed(w http.ResponseWriter) {
	httpapi.MethodNotAllowed(w)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	httpapi.WriteJSON(w, status, body)
}

func writeRawJSON(w http.ResponseWriter, status int, body json.RawMessage) {
	httpapi.WriteRawJSON(w, status, body)
}

func writeSSE(w http.ResponseWriter, status int, frames []string) {
	httpapi.WriteSSE(w, status, frames)
}

func healthResponse() map[string]any {
	return httpapi.HealthResponse()
}

func runtimeInfoResponse(startedAt string) any {
	return httpapi.RuntimeInfoResponse(startedAt)
}

func runtimeCapabilitiesResponse() map[string]any {
	return httpapi.RuntimeCapabilitiesResponse()
}

func runtimeToolsResponse() any {
	return httpapi.RuntimeToolsResponse()
}

func requestBody(w http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	return httpapi.RequestBody(w, r)
}

func routeKey(method string, path string, body json.RawMessage) string {
	return httpapi.RouteKey(method, path, body)
}

func canonicalJSON(body json.RawMessage) string {
	return httpapi.CanonicalJSON(body)
}

func newLiveLocalG2MutableHandler(runtimeToken string, routes []G2RouteReplayCase) *liveLocalG2MutableHandler {
	return livelocal.NewLiveLocalG2MutableHandler(runtimeToken, routes)
}

func mutatingG2Routes(routes []G2RouteReplayCase) []G2RouteReplayCase {
	return livelocal.MutatingG2Routes(routes)
}

func newLiveLocalG3ProviderHandler(runtimeToken string, contract G3ProviderConformanceContract, store *liveLocalG2Store) *liveLocalG3ProviderHandler {
	return provider.NewLiveLocalG3ProviderHandler(runtimeToken, contract, store)
}

func newLiveLocalG4ManagerHandler(
	runtimeToken string,
	contract G4ToolsConformanceContract,
	approvalUserInput ApprovalUserInputRouteContract,
	mcpContract MCPToolLifecycleContract,
	store *liveLocalG2Store,
) *liveLocalG4ManagerHandler {
	return mcpdomain.NewLiveLocalG4ManagerHandler(runtimeToken, contract, approvalUserInput, mcpContract, store)
}

func newLiveLocalDurableHandler(runtimeToken string, tempDir string, routes []G2RouteReplayCase) (*liveLocalDurableHandler, error) {
	return livelocal.NewLiveLocalDurableHandler(runtimeToken, tempDir, routes)
}

func writeDurableSSE(w http.ResponseWriter, status int, events []map[string]any) {
	httpapi.WriteDurableSSE(w, status, events)
}

func intQuery(values url.Values, key string) int {
	return httpapi.IntQuery(values, key)
}

func newLiveLocalLoopHandler(
	runtimeToken string,
	contract GoMinimalAgentLoopContract,
	store *tempDurableEventSessionStore,
) *liveLocalLoopHandler {
	return livelocal.NewLiveLocalLoopHandler(runtimeToken, contract, store)
}

func newLiveLocalKernelHandler(
	runtimeToken string,
	contract GoKernelLiveScaffoldContract,
	g3 *liveLocalG3ProviderHandler,
	g4 *liveLocalG4ManagerHandler,
	durable *liveLocalDurableHandler,
	loop *liveLocalLoopHandler,
	store *liveLocalG2Store,
) *liveLocalKernelHandler {
	return livelocal.NewLiveLocalKernelHandler(runtimeToken, contract, g3, g4, durable, loop, store)
}

func BuildG3ProviderConformanceOutput(contract G3ProviderConformanceContract) map[string]any {
	return provider.BuildG3ProviderConformanceOutput(contract)
}

func buildG3ProviderCacheAccounting(cases []G3ProviderUsageCaseSummary) G3ProviderCacheAccounting {
	return provider.BuildG3ProviderCacheAccounting(cases)
}

func buildG3ProviderRequestShapeSummary(cases []G3ProviderRequestShapeCase) map[string]any {
	return provider.BuildG3ProviderRequestShapeSummary(cases)
}

func cloneG3ProviderRequestShapeCases(cases []G3ProviderRequestShapeCase) []G3ProviderRequestShapeCase {
	return provider.CloneG3ProviderRequestShapeCases(cases)
}

func derivedG3ProviderRequestURL(item G3ProviderRequestShapeCase) string {
	return provider.DerivedG3ProviderRequestURL(item)
}

func derivedG3ProviderToolShape(item G3ProviderRequestShapeCase) string {
	return provider.DerivedG3ProviderToolShape(item)
}

func derivedG3ProviderRequiredHeaders(item G3ProviderRequestShapeCase) []string {
	return provider.DerivedG3ProviderRequiredHeaders(item)
}

func derivedG3ProviderForbiddenHeaders(item G3ProviderRequestShapeCase) []string {
	return provider.DerivedG3ProviderForbiddenHeaders(item)
}

func derivedG3ProviderRequiredBodyFields(item G3ProviderRequestShapeCase) []string {
	return provider.DerivedG3ProviderRequiredBodyFields(item)
}

func derivedG3ProviderForbiddenBodyFields(item G3ProviderRequestShapeCase) []string {
	return provider.DerivedG3ProviderForbiddenBodyFields(item)
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func BuildG4ToolsConformanceOutput(contract G4ToolsConformanceContract) map[string]any {
	return mcpdomain.BuildG4ToolsConformanceOutput(contract)
}

func BuildG5FullLoopConformanceOutput(contract G5FullLoopConformanceContract) map[string]any {
	return conformance.BuildG5FullLoopConformanceOutput(contract)
}

func BuildG5ShadowSlicesOutput(g5 G5FullLoopConformanceContract, sources G5ShadowSourceFixtures) map[string]any {
	return conformance.BuildG5ShadowSlicesOutput(g5, sources)
}

func BuildG5ControlExecutableOutput(cases G5ControlExecutableCases) G5ControlExecutableOutput {
	return conformance.BuildG5ControlExecutableOutput(cases)
}

func buildProviderDriftAttribution(drift ProviderDriftAttribution) G5ProviderDriftAttribution {
	return provider.BuildProviderDriftAttribution(drift)
}

func g3ProviderUsageCaseByID(cases []G3ProviderUsageCaseSummary, id string) G3ProviderUsageCaseSummary {
	for _, item := range cases {
		if item.ID == id {
			return item
		}
	}
	return G3ProviderUsageCaseSummary{}
}

func usagePayloadFromSSEFrames(frames []string) map[string]any {
	return conformance.UsagePayloadFromSSEFrames(frames)
}

func usagePayloadMatchesExpected(usage map[string]any, expected G3ProviderUsageSummary) bool {
	return conformance.UsagePayloadMatchesExpected(usage, expected)
}

func usageSummaryMatchesExpected(parsed G3ProviderUsageSummary, expected G3ProviderUsageSummary) bool {
	return conformance.UsageSummaryMatchesExpected(parsed, expected)
}

func intFromUsagePayload(usage map[string]any, field string) int {
	return conformance.IntFromUsagePayload(usage, field)
}

func floatFromUsagePayload(usage map[string]any, field string) float64 {
	return conformance.FloatFromUsagePayload(usage, field)
}

func sseEventNames(frames []string) []string {
	return conformance.SSEEventNames(frames)
}
