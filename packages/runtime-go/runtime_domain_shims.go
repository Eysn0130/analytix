//go:build !analytix_prod

package runtimego

import (
	"net/http"

	controlapp "analytix.local/runtime-go/internal/app/control"
	jobs "analytix.local/runtime-go/internal/jobs"
	mcp "analytix.local/runtime-go/internal/mcp"
	provider "analytix.local/runtime-go/internal/provider"
)

// Compatibility shim: provider, approval/user-input, MCP, and job runtime
// helpers live in internal domain packages. Root aliases preserve public
// construction and legacy conformance helper names.
type GoRuntimeProviderClient = provider.RuntimeProviderClient

type GoHTTPProviderClient = provider.HTTPProviderClient
type GoProviderMessage = provider.Message
type GoProviderToolSchema = provider.ToolSchema
type GoProviderRequest = provider.Request
type GoProviderUsage = provider.Usage
type GoProviderChunk = provider.Chunk
type GoProviderPrefixShape = provider.PrefixShape
type GoProviderResult = provider.Result

func NewGoHTTPProviderClient(httpClient *http.Client) *GoHTTPProviderClient {
	return provider.NewHTTPProviderClient(httpClient)
}

func CaptureGoProviderPrefixShape(request GoProviderRequest) GoProviderPrefixShape {
	return provider.CapturePrefixShape(request)
}

func appendProviderEndpointPath(baseURL string, versionedPath string) string {
	return provider.AppendEndpointPath(baseURL, versionedPath)
}

type liveApprovalUserInputManager = controlapp.ApprovalUserInputManager

func newLiveApprovalUserInputManager() *liveApprovalUserInputManager {
	return controlapp.NewApprovalUserInputManager()
}

func runApprovalUserInputContractExercise() map[string]any {
	return controlapp.RunApprovalUserInputContractExercise()
}

type liveTestMCPManager = mcp.ContractReplayManager

func runMCPManagerContractExercise() map[string]any {
	return mcp.RunManagerContractExercise()
}

type liveJobManager = jobs.Manager

func runJobLineageContractExercise() map[string]any {
	return jobs.RunLineageContractExercise()
}
