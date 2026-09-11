package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

func TestRuntimeServerProviderRegistryRouteUsesInjectedAuthorityAndAuth(t *testing.T) {
	t.Parallel()

	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	service := &runtimeProviderRegistryServiceStub{snapshot: domainregistry.Registry{
		Version: domainregistry.FormatVersion, Incarnation: registryIncarnation,
		Providers: map[string]domainregistry.Provider{}, Transactions: map[string]domainregistry.Transaction{},
	}}
	handler := &runtimeServerHandler{
		runtimeToken: "runtime-secret-token", providerRegistry: service,
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	if recorder.Code != http.StatusUnauthorized || service.snapshotCalled {
		t.Fatalf("unauthorized response = (%d, called=%v, %s)", recorder.Code, service.snapshotCalled, recorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil)
	request.Header.Set("Authorization", "Bearer runtime-secret-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !service.snapshotCalled ||
		!strings.Contains(recorder.Body.String(), `"schemaVersion":1`) ||
		!strings.Contains(recorder.Body.String(), `"providers":[]`) {
		t.Fatalf("authorized response = (%d, called=%v, %s)", recorder.Code, service.snapshotCalled, recorder.Body.String())
	}
}

func TestRuntimeServerProviderRegistryRouteFailsClosedWithoutAuthority(t *testing.T) {
	t.Parallel()

	handler := &runtimeServerHandler{runtimeToken: "runtime-secret-token"}
	request := httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil)
	request.Header.Set("Authorization", "Bearer runtime-secret-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(recorder.Body.String(), `"code":"persistence_failure"`) {
		t.Fatalf("missing authority response = (%d, %s)", recorder.Code, recorder.Body.String())
	}
}

type runtimeProviderRegistryServiceStub struct {
	httpapi.ProviderRegistryService
	snapshot       domainregistry.Registry
	snapshotCalled bool
}

func (service *runtimeProviderRegistryServiceStub) Snapshot(context.Context) (domainregistry.Registry, error) {
	service.snapshotCalled = true
	return service.snapshot.Clone(), nil
}
