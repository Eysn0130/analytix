package runtimeapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestProviderRegistryAccountObservationUsesExactCurrentAuthorityAndProjectsOnlyBoundedQuota(t *testing.T) {
	const (
		providerID = "provider-observation-current"
		secret     = "synthetic-observation-credential"
	)
	observedRequests := 0
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedRequests++
		credentialMatched := r.Header.Get("Authorization") == "Bearer "+secret
		if r.Method != http.MethodGet || r.URL.Path != "/quota" || !credentialMatched {
			t.Errorf("observation request method_ok=%t path_ok=%t credential_matched=%t",
				r.Method == http.MethodGet, r.URL.Path == "/quota", credentialMatched)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schemaVersion":1,"quota":1000,"usage":125,"remaining":875}`))
	}))
	defer providerServer.Close()

	authority, initial, committed := openProviderRegistryProbeAuthorityV1(
		t, providerID, providerServer.URL+"/v1", "", secret,
	)
	updated := configureProviderRegistryAccountObservationV1(
		t, authority, initial, committed, providerServer.URL+"/quota",
	)

	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/account-observation",
		strings.NewReader(providerRegistryObservationExpectedBodyV1(initial, updated)),
	))
	if recorder.Code != http.StatusOK || observedRequests != 1 {
		t.Fatalf("account observation status=%d requests=%d body=%s", recorder.Code, observedRequests, recorder.Body.String())
	}
	var response struct {
		ProviderCredentialPurpose string  `json:"providerCredentialPurpose"`
		Status                    string  `json:"status"`
		Quota                     float64 `json:"quota"`
		Usage                     float64 `json:"usage"`
		Remaining                 float64 `json:"remaining"`
		ObservedAt                string  `json:"observedAt"`
		ExpiresAt                 string  `json:"expiresAt"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil ||
		response.ProviderCredentialPurpose != "provider-api-key" || response.Status != "available" ||
		response.Quota != 1000 || response.Usage != 125 || response.Remaining != 875 {
		t.Fatalf("account observation projection=%#v err=%v", response, err)
	}
	if _, err := time.Parse(time.RFC3339Nano, response.ObservedAt); err != nil {
		t.Fatalf("observedAt=%q: %v", response.ObservedAt, err)
	}
	if _, err := time.Parse(time.RFC3339Nano, response.ExpiresAt); err != nil {
		t.Fatalf("expiresAt=%q: %v", response.ExpiresAt, err)
	}
	public := recorder.Body.String()
	for _, forbidden := range []string{secret, "credentialRef", providerServer.URL, "Authorization", "raw"} {
		if strings.Contains(public, forbidden) {
			t.Fatalf("account observation public response disclosed %q: %s", forbidden, public)
		}
	}
}

func TestProviderRegistryAccountObservationDiscardsCredentialReplacementRace(t *testing.T) {
	const (
		providerID   = "provider-observation-stale"
		firstSecret  = "synthetic-observation-first"
		secondSecret = "synthetic-observation-second"
	)
	started := make(chan struct{})
	release := make(chan struct{})
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if credentialMatched := r.Header.Get("Authorization") == "Bearer "+firstSecret; !credentialMatched {
			t.Errorf("observation request credential_matched=%t", credentialMatched)
		}
		close(started)
		<-release
		_, _ = w.Write([]byte(`{"schemaVersion":1,"quota":10,"remaining":9}`))
	}))
	defer providerServer.Close()

	authority, initial, committed := openProviderRegistryProbeAuthorityV1(
		t, providerID, providerServer.URL+"/v1", "", firstSecret,
	)
	updated := configureProviderRegistryAccountObservationV1(
		t, authority, initial, committed, providerServer.URL+"/quota",
	)
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Handle(recorder, httptest.NewRequest(
			http.MethodPost,
			httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/account-observation",
			strings.NewReader(providerRegistryObservationExpectedBodyV1(initial, updated)),
		))
	}()
	<-started
	replacement, err := secretstoreport.SetCredential([]byte(secondSecret))
	if err != nil {
		t.Fatal(err)
	}
	_, err = authority.Manager().ReplaceCredential(context.Background(), providerregistryapp.CredentialReplaceCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: 2, RegistryIncarnation: initial.Incarnation,
			ProviderRevision: updated.Revision, ProviderGeneration: updated.Generation,
			ProviderIncarnation: updated.Incarnation, ProviderCredentialPurpose: updated.CredentialPurpose,
		},
		ProviderID: providerID, CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
		Credential: replacement,
	})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	if recorder.Code != http.StatusConflict || strings.Contains(recorder.Body.String(), firstSecret) ||
		strings.Contains(recorder.Body.String(), secondSecret) || strings.Contains(recorder.Body.String(), "quota") {
		t.Fatalf("stale observation was not discarded: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestProviderRegistryAccountObservationFailsClosedForAbsentRedirectOversizeAndMalformedCapability(t *testing.T) {
	t.Run("absent capability remains unavailable", func(t *testing.T) {
		authority, initial, committed := openProviderRegistryProbeAuthorityV1(
			t, "provider-observation-absent", "https://provider.invalid/v1", "", "synthetic-absent",
		)
		result, err := authority.Service().ObserveAccount(context.Background(), providerregistryapp.ProviderOperationCommand{
			Expected: domainregistry.ExpectedState{
				RegistryRevision: 1, RegistryIncarnation: initial.Incarnation,
				ProviderRevision: committed.Revision, ProviderGeneration: committed.Generation,
				ProviderIncarnation: committed.Incarnation, ProviderCredentialPurpose: committed.CredentialPurpose,
			},
			ProviderID: committed.ID,
		})
		if err != nil || result.Status != providerregistryapp.ProviderAccountObservationUnavailable ||
			result.Quota != nil || result.Usage != nil || result.Remaining != nil ||
			!result.ExpiresAt.After(result.ObservedAt) {
			t.Fatalf("absent observation=%#v err=%v", result, err)
		}
	})

	for _, testCase := range []struct {
		name    string
		handler http.HandlerFunc
		want    providerregistryapp.ProviderAccountObservationStatus
	}{
		{
			name: "redirect",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "https://other.invalid/quota")
				w.WriteHeader(http.StatusFound)
			},
			want: providerregistryapp.ProviderAccountObservationRedirectBlocked,
		},
		{
			name: "oversize",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(strings.Repeat("x", providerRegistryObservationMaxBodyBytesV1+1)))
			},
			want: providerregistryapp.ProviderAccountObservationInvalidResponse,
		},
		{
			name: "hostile values",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"schemaVersion":1,"quota":-1,"remaining":2}`))
			},
			want: providerregistryapp.ProviderAccountObservationInvalidResponse,
		},
		{
			name: "unknown response field",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"schemaVersion":1,"quota":10,"account":"must-not-project"}`))
			},
			want: providerregistryapp.ProviderAccountObservationInvalidResponse,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(testCase.handler)
			defer server.Close()
			provider := domainregistry.Provider{
				ID: "provider-observation-fixture", Kind: "openai-compatible",
				AccountObservation: &domainregistry.AccountObservationBinding{
					SchemaVersion: 1, Endpoint: server.URL, Method: http.MethodGet, Projection: "normalized-quota-v1",
				},
			}
			observedAt := time.Now().UTC()
			outcome := executeProviderRegistryAccountObservationV1(
				context.Background(), provider, []byte("synthetic-fixture"), observedAt,
			)
			if outcome.status != testCase.want || outcome.quota != nil || outcome.usage != nil ||
				outcome.remaining != nil || !outcome.expiresAt.After(observedAt) {
				t.Fatalf("outcome=%#v want=%q", outcome, testCase.want)
			}
		})
	}

	t.Run("cancelled request is a bounded timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"schemaVersion":1,"quota":10}`))
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		observedAt := time.Now().UTC()
		outcome := executeProviderRegistryAccountObservationV1(ctx, domainregistry.Provider{
			ID: "provider-observation-timeout", Kind: "openai-compatible",
			AccountObservation: &domainregistry.AccountObservationBinding{
				SchemaVersion: 1, Endpoint: server.URL, Method: http.MethodGet, Projection: "normalized-quota-v1",
			},
		}, []byte("synthetic-timeout"), observedAt)
		if outcome.status != providerregistryapp.ProviderAccountObservationTimeout ||
			!outcome.expiresAt.After(observedAt) {
			t.Fatalf("cancelled observation status=%q expiry_bounded=%t",
				outcome.status, outcome.expiresAt.After(observedAt))
		}
	})
}

func configureProviderRegistryAccountObservationV1(
	t *testing.T,
	authority *providerRegistryAuthorityV1,
	initial domainregistry.Registry,
	committed domainregistry.Provider,
	endpoint string,
) domainregistry.Provider {
	t.Helper()
	updated, err := authority.Manager().Update(context.Background(), providerregistryapp.UpdateCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: 1, RegistryIncarnation: initial.Incarnation,
			ProviderRevision: committed.Revision, ProviderGeneration: committed.Generation,
			ProviderIncarnation: committed.Incarnation, ProviderCredentialPurpose: committed.CredentialPurpose,
		},
		Provider: domainregistry.ProviderInput{
			ID: committed.ID, Kind: committed.Kind, Endpoint: committed.Endpoint, Proxy: committed.Proxy,
			Models: committed.Models, MediaModels: committed.MediaModels,
			SelectedModel: committed.SelectedModel, SelectedMedia: committed.SelectedMedia,
			SelectedRoutes: committed.SelectedRoutes, OAuthBinding: committed.OAuthBinding,
			AccountObservation: &domainregistry.AccountObservationBinding{
				SchemaVersion: 1, Endpoint: endpoint, Method: http.MethodGet, Projection: "normalized-quota-v1",
			},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("configure observation: %v", err)
	}
	return updated
}

func providerRegistryObservationExpectedBodyV1(initial domainregistry.Registry, provider domainregistry.Provider) string {
	return fmt.Sprintf(
		`{"schemaVersion":1,"expected":{"registryRevision":"2","registryIncarnation":%q,"providerRevision":"2","providerGeneration":"1","providerIncarnation":%q,"providerCredentialPurpose":%q}}`,
		initial.Incarnation, provider.Incarnation, provider.CredentialPurpose,
	)
}
