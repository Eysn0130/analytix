//go:build analytix_prod

package readiness

import (
	"context"
	"net/http"
)

func RunProviderReadinessMatrix(ctx context.Context, env map[string]string, httpClient *http.Client) (RuntimeProviderMatrixResult, error) {
	return RuntimeProviderMatrixResult{
		SchemaVersion:              1,
		ProviderMatrixScaffold:     false,
		FixtureMatrixRequired:      false,
		CredentialedMatrixEnvGated: true,
		ReadsRealAPIKeysByDefault:  false,
		CredentialedProbes: []RuntimeReadinessProbe{{
			ID:           "credentialed-provider-matrix",
			Status:       "skipped",
			Skipped:      true,
			Credentialed: true,
			Message:      "provider matrix scaffold is available only in conformance builds; production runtime uses external evidence gates",
		}},
	}, nil
}
