package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestPrivateMediaExecutionUsesRestartedRegistryModelRouteProxyAndCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const (
		providerID = "provider-media-integration"
		mediaModel = "image-model-registry"
		secret     = "synthetic-media-integration-credential"
	)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	var observedURL, observedAuthorization, observedBody string
	var upstreamCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		observedURL = r.URL.String()
		observedAuthorization = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		observedBody = string(body)
		clear(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer proxy.Close()

	dataDir := t.TempDir()
	seedMediaExecutionAuthorityV1(t, dataDir, providerID, "http://provider-route.invalid/v1", proxy.URL, mediaModel, secret)
	handler := NewRuntimeServerHandler(Config{
		RuntimeToken: "media-integration-token", DataDir: dataDir, DurableTempDir: t.TempDir(),
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, httpapi.MediaExecutionPathV1,
		strings.NewReader(`{"schemaVersion":1,"operation":"image.generate","prompt":"bounded intent phone 13800138000","size":"1024x1024"}`))
	request.Header.Set("Authorization", "Bearer media-integration-token")
	handler.ServeHTTP(recorder, request)
	if strings.Contains(observedBody, "13800138000") {
		t.Fatal("production media request retained private canary")
	}

	if recorder.Code != http.StatusOK || observedAuthorization != "Bearer "+secret ||
		observedURL != "http://provider-route.invalid/v1/images/generations" ||
		!strings.Contains(observedBody, `"model":"`+mediaModel+`"`) {
		t.Fatalf("private media Registry route mismatch: status=%d url=%q authorization=%q providerBody=%s response=%s",
			recorder.Code, observedURL, observedAuthorization, observedBody, recorder.Body.String())
	}
	for _, forbidden := range []string{secret, proxy.URL, "provider-route.invalid", mediaModel, "credentialRef"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("private media response leaked %q: %s", forbidden, recorder.Body.String())
		}
	}
	if !strings.Contains(recorder.Body.String(), base64.StdEncoding.EncodeToString(png)) {
		t.Fatalf("private media response omitted bounded image: %s", recorder.Body.String())
	}
	for _, payload := range []map[string]any{
		{"operation": "image.edit", "prompt": "bounded edit", "images": []map[string]string{{
			"name": "reference.png", "mimeType": "image/png", "dataBase64": base64.StdEncoding.EncodeToString(append(png, []byte("13800138000")...)),
		}}},
		{"operation": "speech.transcribe", "mimeType": "audio/wav", "audioBase64": base64.StdEncoding.EncodeToString([]byte("13800138000"))},
		{"operation": "image.generate", "prompt": "Draw /Users/private-owner/case.csv"},
	} {
		payload["schemaVersion"] = 1
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		before := upstreamCalls.Load()
		request := httptest.NewRequest(http.MethodPost, httpapi.MediaExecutionPathV1, strings.NewReader(string(encoded)))
		request.Header.Set("Authorization", "Bearer media-integration-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var failure struct {
			Code string `json:"code"`
		}
		decodeErr := json.Unmarshal(response.Body.Bytes(), &failure)
		if response.Code != http.StatusServiceUnavailable || decodeErr != nil || failure.Code != "privacy_unavailable" || upstreamCalls.Load() != before {
			t.Fatalf("production uninspectable media admission failed: operation=%s status=%d code=%q decoded=%t calls=%d", payload["operation"], response.Code, failure.Code, decodeErr == nil, upstreamCalls.Load()-before)
		}
		for _, canary := range []string{"13800138000", "/Users/private-owner", "dataBase64", "audioBase64"} {
			if strings.Contains(response.Body.String(), canary) {
				t.Fatal("media rejection exposed source content")
			}
		}
	}
}

func TestVisionExecutionResolverUsesSelectedMediaAndRejectsRegistryDrift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const (
		providerID  = "provider-vision-jit"
		firstModel  = "vision-model-first"
		secondModel = "vision-model-second"
		secret      = "synthetic-vision-jit-credential"
	)
	dataDir := t.TempDir()
	seedMediaExecutionAuthorityV1(
		t, dataDir, providerID, "https://vision-provider.invalid/v1", "http://vision-proxy.invalid:8080", firstModel, secret,
	)
	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open Vision Registry authority: %v", err)
	}
	defer func() {
		if err := authority.Close(); err != nil {
			t.Fatalf("close Vision Registry authority: %v", err)
		}
	}()
	resolver := newProviderRegistryExecutionResolverV1(authority.Manager())
	lease, err := resolver.ResolveVisionExecution(context.Background())
	if err != nil {
		t.Fatalf("ResolveVisionExecution() error = %v", err)
	}
	if lease.Config.ProviderID != providerID || lease.Config.Model != firstModel ||
		lease.Config.BaseURL != "https://vision-provider.invalid/v1" ||
		lease.Config.ProxyURL != "http://vision-proxy.invalid:8080" || lease.Config.APIKey != secret ||
		!lease.Config.SupportsImageInput {
		lease.Clear()
		t.Fatalf("Vision JIT config mismatch: %#v", lease.Config)
	}
	if err := lease.ValidateCurrent(context.Background()); err != nil {
		lease.Clear()
		t.Fatalf("initial Vision authority is not current: %v", err)
	}
	snapshot, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		lease.Clear()
		t.Fatalf("snapshot Vision Registry authority: %v", err)
	}
	current := snapshot.Providers[providerID]
	_, err = authority.Manager().Update(context.Background(), providerregistryapp.UpdateCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
			ProviderRevision: current.Revision, ProviderGeneration: current.Generation,
			ProviderIncarnation: current.Incarnation, ProviderCredentialPurpose: current.CredentialPurpose,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: current.Kind, Endpoint: current.Endpoint, Proxy: current.Proxy,
			Models: current.Models, MediaModels: []string{firstModel, secondModel},
			SelectedModel: current.SelectedModel, SelectedMedia: secondModel, SelectedRoutes: current.SelectedRoutes,
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		lease.Clear()
		t.Fatalf("update Vision Registry authority: %v", err)
	}
	if err := lease.ValidateCurrent(context.Background()); err == nil {
		lease.Clear()
		t.Fatal("drifted Vision execution lease remained current")
	}
	lease.Clear()
	if lease.Config.APIKey != "" || lease.Config.BaseURL != "" || lease.Config.ProxyURL != "" {
		t.Fatalf("Vision execution lease retained secret-bearing route: %#v", lease.Config)
	}
}

func TestPrivateMediaExecutionDiscardsResponseAfterRegistryDrift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const (
		providerID  = "provider-media-stale"
		firstModel  = "image-model-first"
		secondModel = "image-model-second"
		secret      = "synthetic-media-stale-credential"
	)
	started := make(chan struct{})
	release := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgoAAAAA"}]}`))
	}))
	defer proxy.Close()

	dataDir := t.TempDir()
	seedMediaExecutionAuthorityV1(t, dataDir, providerID, "http://provider-stale.invalid/v1", proxy.URL, firstModel, secret)
	handler := NewRuntimeServerHandler(Config{
		RuntimeToken: "media-stale-token", DataDir: dataDir, DurableTempDir: t.TempDir(),
	})
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := httptest.NewRequest(http.MethodPost, httpapi.MediaExecutionPathV1,
			strings.NewReader(`{"schemaVersion":1,"operation":"image.generate","prompt":"bounded intent"}`))
		request.Header.Set("Authorization", "Bearer media-stale-token")
		handler.ServeHTTP(recorder, request)
	}()
	<-started

	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open Provider Registry drift authority: %v", err)
	}
	snapshot, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		_ = authority.Close()
		t.Fatalf("snapshot Provider Registry drift authority: %v", err)
	}
	current := snapshot.Providers[providerID]
	_, err = authority.Manager().Update(context.Background(), providerregistryapp.UpdateCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
			ProviderRevision: current.Revision, ProviderGeneration: current.Generation,
			ProviderIncarnation: current.Incarnation, ProviderCredentialPurpose: current.CredentialPurpose,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: current.Kind, Endpoint: current.Endpoint, Proxy: current.Proxy,
			Models: current.Models, MediaModels: []string{firstModel, secondModel},
			SelectedModel: current.SelectedModel, SelectedMedia: secondModel, SelectedRoutes: current.SelectedRoutes,
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if closeErr := authority.Close(); err != nil || closeErr != nil {
		close(release)
		<-done
		t.Fatalf("update Provider Registry during media response: update=%v close=%v", err, closeErr)
	}
	close(release)
	<-done

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"conflict"`) ||
		strings.Contains(recorder.Body.String(), "imageBase64") {
		t.Fatalf("stale private media response was consumed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, forbidden := range []string{secret, proxy.URL, firstModel, secondModel, "provider-stale.invalid"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("stale private media response leaked %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func seedMediaExecutionAuthorityV1(
	t *testing.T,
	dataDir string,
	providerID string,
	endpoint string,
	proxy string,
	mediaModel string,
	secret string,
) {
	t.Helper()
	installSyntheticProviderRegistryFallbackMasterKey(t, dataDir)
	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("open Provider Registry media fixture: %v", err)
	}
	snapshot, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		_ = authority.Close()
		t.Fatalf("snapshot Provider Registry media fixture: %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		_ = authority.Close()
		t.Fatalf("prepare Provider Registry media fixture: %v", err)
	}
	_, err = authority.Manager().Connect(context.Background(), providerregistryapp.ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: "openai-compatible", Endpoint: endpoint, Proxy: proxy,
			Models: []string{"text-model"}, MediaModels: []string{mediaModel},
			SelectedModel: "text-model", SelectedMedia: mediaModel, SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if closeErr := authority.Close(); err != nil || closeErr != nil {
		t.Fatalf("seed Provider Registry media fixture: connect=%v close=%v", err, closeErr)
	}
}
