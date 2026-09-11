//go:build !analytix_prod

package runtimego

import (
	"context"
	"net/http"

	readiness "analytix.local/runtime-go/internal/readiness"
)

// Compatibility shim: G6 readiness probes and evidence checks live in
// internal/readiness. The root package only preserves the public test API.
const (
	RuntimeDurableRestartEvidenceEnv = readiness.RuntimeDurableRestartEvidenceEnv
	RuntimeProviderMatrixEnv         = readiness.RuntimeProviderMatrixEnv
	RuntimeMCPMatrixEnv              = readiness.RuntimeMCPMatrixEnv
	RuntimePackagedQAEnv             = readiness.RuntimePackagedQAEnv
	RuntimeReadyEnv                  = readiness.RuntimeReadyEnv
	RuntimeMCPCommandEnv             = readiness.RuntimeMCPCommandEnv
	RuntimeMCPURLEnv                 = readiness.RuntimeMCPURLEnv
)

type RuntimeReadinessCheck = readiness.RuntimeReadinessCheck
type RuntimeReadinessStatus = readiness.RuntimeReadinessStatus
type RuntimeReadinessProbe = readiness.RuntimeReadinessProbe
type RuntimeProviderMatrixResult = readiness.RuntimeProviderMatrixResult
type RuntimeMCPMatrixResult = readiness.RuntimeMCPMatrixResult

func RuntimeReadinessStatusFromEnv(env map[string]string) RuntimeReadinessStatus {
	return readiness.RuntimeReadinessStatusFromEnv(env)
}

func RunProviderReadinessMatrix(ctx context.Context, env map[string]string, httpClient *http.Client) (RuntimeProviderMatrixResult, error) {
	return readiness.RunProviderReadinessMatrix(ctx, env, httpClient)
}

func RunMCPReadinessMatrix(env map[string]string) RuntimeMCPMatrixResult {
	return readiness.RunMCPReadinessMatrix(env)
}

func runtimeReadinessProbeContext() (context.Context, context.CancelFunc) {
	return readiness.RuntimeReadinessProbeContext()
}
