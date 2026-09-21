package model

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestMissingProviderKeyErrorCarriesIdentityNotCredential(t *testing.T) {
	const providerID = "d5-private-provider-canary"
	const unrelatedCredential = "sk-d5-unrelated-credential-canary"
	for _, test := range []struct{ name, value string }{{"empty", ""}, {"whitespace", " \t\n"}} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(domainmodel.ModelProvidersConfig{
				DefaultProviderID: providerID,
				Providers: []domainmodel.ModelProviderConfig{
					{ID: providerID, BaseURL: "https://provider.invalid", APIKey: test.value, Models: []string{"model"}},
					{ID: "other", BaseURL: "https://other.invalid", APIKey: unrelatedCredential, Models: []string{"model"}},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			set := NewRuntimeProviderConfigSet(domainmodel.RuntimeProviderConfigInput{ModelProvidersJSON: string(encoded)})
			_, err = set.ResolveTurnExecution(TurnExecutionInput{})
			if err == nil || err.Error() != "provider configuration error: apiKey is required for provider "+providerID {
				t.Fatal("missing-key source did not return its identity-only diagnostic")
			}
			if strings.Contains(err.Error(), unrelatedCredential) {
				t.Fatal("missing-key diagnostic retained another provider's credential")
			}
		})
	}
}
