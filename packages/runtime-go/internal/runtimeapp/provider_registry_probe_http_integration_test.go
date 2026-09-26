package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	"analytix.local/runtime-go/internal/contracts"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestRuntimeHTTPNativeMessagesRegistrySurvivesRestartWithoutUIModelMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows native credential lifecycle has its own DPAPI fixture")
	}
	var mu sync.Mutex
	var observed []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "synthetic-native-messages" || json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		observed = append(observed, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"model\":\"deepseek-flash\",\"usage\":{\"input_tokens\":8,\"cache_read_input_tokens\":0,\"cache_creation_input_tokens\":0}}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Native Messages selected from Registry.\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":9}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer upstream.Close()
	root, workspace := t.TempDir(), workspacetest.New(t)
	config := Config{RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: filepath.Join(root, "durable"), DataDir: filepath.Join(root, "data"), UserDataDir: filepath.Join(root, "user")}
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, "native-messages", upstream.URL+"/v1", []string{"deepseek-flash"}, "deepseek-flash", "synthetic-native-messages", "deepseek-messages")
	// No ModelProvidersJSON, UI profile or legacy key supplies the protocol.
	for generation := 0; generation < 2; generation++ {
		func() {
			handler, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			defer shutdownOwnedRuntimeHandler(t, handler)
			client := &http.Client{Timeout: 30 * time.Second}
			for _, effort := range [][]string{{"low", "medium", "high"}, {"max", "off", "auto"}}[generation] {
				status, created := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodPost, "/v1/threads", map[string]any{"title": "Native Messages", "workspace": workspace})
				threadID := contracts.StringField(created, "id")
				if status != http.StatusCreated || threadID == "" {
					t.Fatalf("create thread status=%d", status)
				}
				status, _ = packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Explain the selected protocol briefly.", "reasoningEffort": effort})
				if status != http.StatusAccepted {
					t.Fatalf("start native Messages status=%d", status)
				}
				deadline := time.Now().Add(60 * time.Second)
				for {
					_, thread := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil)
					encoded, _ := json.Marshal(thread)
					if strings.Contains(string(encoded), "Native Messages selected from Registry.") && strings.Contains(string(encoded), `"status":"completed"`) {
						break
					}
					if strings.Contains(string(encoded), `"status":"failed"`) || time.Now().After(deadline) {
						t.Fatal("native Messages did not publish a completed answer")
					}
					time.Sleep(20 * time.Millisecond)
				}
				mu.Lock()
				body := observed[len(observed)-1]
				mu.Unlock()
				thinking, _ := body["thinking"].(map[string]any)
				output, _ := body["output_config"].(map[string]any)
				if body["model"] != "deepseek-flash" || body["max_tokens"] != float64(4096) || body["betas"] != nil {
					t.Fatal("native model/cap/baseline mismatch")
				}
				want := effort
				if want == "medium" {
					want = "high"
				}
				if effort == "auto" {
					if body["thinking"] != nil || body["output_config"] != nil {
						t.Fatal("auto changed the server default")
					}
				} else if effort == "off" {
					if thinking["type"] != "disabled" {
						t.Fatal("off did not disable thinking")
					}
				} else if thinking["type"] != "enabled" || output["effort"] != want {
					t.Fatal("native thinking/effort did not reach the wire")
				}
			}
		}()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(observed) != 6 {
		t.Fatal("unexpected physical request count")
	}
}

func TestProviderRegistryProbeUsesOnlyCommittedProviderAndProtectedCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}

	const (
		providerID = "provider-probe-loopback"
		purpose    = "provider-api-key"
		secret     = "synthetic-provider-probe-credential"
	)
	requestObserved := make(chan struct{}, 1)
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("provider path = %q", r.URL.Path)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer "+secret {
			t.Errorf("Authorization = %q", authorization)
		}
		requestObserved <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-beta"},{"id":"model-alpha"}]}`))
	}))
	defer providerServer.Close()

	dataDir := t.TempDir()
	installSyntheticProviderRegistryFallbackMasterKey(t, dataDir)
	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
	}
	t.Cleanup(func() {
		if err := authority.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	initial, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	committed, err := authority.Manager().Connect(context.Background(), providerregistryapp.ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: initial.Revision, RegistryIncarnation: initial.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: "openai-compatible", Endpoint: providerServer.URL + "/v1",
			Models: []string{"model-existing"}, SelectedModel: "model-existing",
		},
		CredentialPurpose: secretstoreport.Purpose(purpose),
		Credential:        credential,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	expected := fmt.Sprintf(
		`{"schemaVersion":1,"expected":{"registryRevision":"1","registryIncarnation":%q,"providerRevision":"1","providerGeneration":"1","providerIncarnation":%q,"providerCredentialPurpose":%q}}`,
		initial.Incarnation,
		committed.Incarnation,
		committed.CredentialPurpose,
	)
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	rejected := httptest.NewRecorder()
	handler.Handle(rejected, httptest.NewRequest(
		http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/probe",
		strings.NewReader(strings.TrimSuffix(expected, "}")+`,"endpoint":"http://renderer.invalid"}`),
	))
	if rejected.Code != http.StatusBadRequest || !strings.Contains(rejected.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("renderer-authority response = (%d, %s)", rejected.Code, rejected.Body.String())
	}
	select {
	case <-requestObserved:
		t.Fatal("renderer-supplied endpoint reached provider egress")
	default:
	}

	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/probe",
		strings.NewReader(expected),
	))

	if recorder.Code != http.StatusOK {
		t.Fatalf("probe response = (%d, %s)", recorder.Code, recorder.Body.String())
	}
	select {
	case <-requestObserved:
	default:
		t.Fatal("committed loopback Provider was not called")
	}
	credentialCheck := httptest.NewRecorder()
	handler.Handle(credentialCheck, httptest.NewRequest(http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/credential-check", strings.NewReader(expected)))
	if credentialCheck.Code != http.StatusOK || !strings.Contains(credentialCheck.Body.String(), `"credentialAvailable":true`) ||
		strings.Contains(credentialCheck.Body.String(), secret) || strings.Contains(credentialCheck.Body.String(), "credentialRef") {
		t.Fatal("credential availability did not resolve through the composed protected store")
	}
	select {
	case <-requestObserved:
		t.Fatal("local credential resolution made an external Provider call")
	default:
	}
	for _, required := range []string{
		`"status":"reachable"`, `"modelCount":2`, `"registryRevision":"1"`,
		`"providerRevision":"1"`, `"providerGeneration":"1"`,
	} {
		if !strings.Contains(recorder.Body.String(), required) {
			t.Fatalf("probe response is missing %q: %s", required, recorder.Body.String())
		}
	}
	for _, forbidden := range []string{
		secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		providerServer.URL,
		"credentialRef",
		"rawBody",
	} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("probe response leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestProviderRegistryProbeDiscardsStalePostCallResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const (
		providerID = "provider-probe-stale"
		purpose    = "provider-api-key"
		firstKey   = "synthetic-stale-probe-first"
		secondKey  = "synthetic-stale-probe-second"
	)
	started := make(chan struct{})
	release := make(chan struct{})
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer "+firstKey {
			t.Errorf("Authorization = %q", authorization)
		}
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"stale-model"}]}`))
	}))
	defer providerServer.Close()

	authority, initial, committed := openProviderRegistryProbeAuthorityV1(
		t, providerID, providerServer.URL+"/v1", "", firstKey,
	)
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Handle(recorder, httptest.NewRequest(
			http.MethodPost,
			httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/probe",
			strings.NewReader(providerRegistryProbeExpectedBodyV1(initial, committed)),
		))
	}()
	<-started
	replacement, err := secretstoreport.SetCredential([]byte(secondKey))
	if err != nil {
		t.Fatalf("SetCredential(replacement) error = %v", err)
	}
	_, err = authority.Manager().ReplaceCredential(context.Background(), providerregistryapp.CredentialReplaceCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: 1, RegistryIncarnation: initial.Incarnation,
			ProviderRevision: committed.Revision, ProviderGeneration: committed.Generation,
			ProviderIncarnation: committed.Incarnation, ProviderCredentialPurpose: committed.CredentialPurpose,
		},
		ProviderID: providerID, CredentialPurpose: secretstoreport.Purpose(purpose), Credential: replacement,
	})
	if err != nil {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	close(release)
	<-done
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"conflict"`) {
		t.Fatalf("stale probe response = (%d, %s)", recorder.Code, recorder.Body.String())
	}
	for _, forbidden := range []string{firstKey, secondKey, providerServer.URL, "stale-model"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("stale probe response leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestProviderRegistryDiscoveryDiscardsStalePostCallResultWithoutMutatingCommittedModels(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const (
		providerID = "provider-discovery-stale"
		secret     = "synthetic-stale-discovery-credential"
	)
	started := make(chan struct{})
	release := make(chan struct{})
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer "+secret {
			t.Errorf("Authorization = %q", authorization)
		}
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"stale-discovered-model"}]}`))
	}))
	defer providerServer.Close()

	authority, initial, committed := openProviderRegistryProbeAuthorityV1(
		t, providerID, providerServer.URL+"/v1", "", secret,
	)
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.Handle(recorder, httptest.NewRequest(
			http.MethodPost,
			httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/discover-models",
			strings.NewReader(providerRegistryProbeExpectedBodyV1(initial, committed)),
		))
	}()
	<-started
	postChange, err := authority.Manager().Update(context.Background(), providerregistryapp.UpdateCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: 1, RegistryIncarnation: initial.Incarnation,
			ProviderRevision: committed.Revision, ProviderGeneration: committed.Generation,
			ProviderIncarnation: committed.Incarnation, ProviderCredentialPurpose: committed.CredentialPurpose,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: committed.Kind, Endpoint: committed.Endpoint,
			Models: []string{"post-change-model"}, SelectedModel: "post-change-model",
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(post-change) error = %v", err)
	}
	close(release)
	<-done
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"conflict"`) {
		t.Fatalf("stale discovery response = (%d, %s)", recorder.Code, recorder.Body.String())
	}
	for _, forbidden := range []string{secret, providerServer.URL, "stale-discovered-model", "post-change-model"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("stale discovery response leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
	snapshot, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() after stale discovery error = %v", err)
	}
	current := snapshot.Providers[providerID]
	if snapshot.Revision != 2 || current.Revision != postChange.Revision || current.Generation != postChange.Generation ||
		strings.Join(current.Models, ",") != "post-change-model" || current.SelectedModel != "post-change-model" {
		t.Fatalf("post-change Registry facts were mutated: revision=%d provider=%#v", snapshot.Revision, current)
	}
}

func TestProviderRegistryProbeBlocksRedirectWithoutForwardingCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const secret = "synthetic-redirect-probe-credential"
	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedRequests.Add(1)
	}))
	defer redirectTarget.Close()
	redirectOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer "+secret {
			t.Errorf("Authorization = %q", authorization)
		}
		http.Redirect(w, r, redirectTarget.URL+"/v1/models", http.StatusTemporaryRedirect)
	}))
	defer redirectOrigin.Close()

	authority, initial, committed := openProviderRegistryProbeAuthorityV1(
		t, "provider-probe-redirect", redirectOrigin.URL+"/v1", "", secret,
	)
	recorder := httptest.NewRecorder()
	httpapi.ProviderRegistryHandlers{Service: authority.Service()}.Handle(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			httpapi.ProviderRegistryPathV1+"/providers/provider-probe-redirect/probe",
			strings.NewReader(providerRegistryProbeExpectedBodyV1(initial, committed)),
		),
	)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"redirect_blocked"`) ||
		!strings.Contains(recorder.Body.String(), `"code":307`) {
		t.Fatalf("redirect probe response = (%d, %s)", recorder.Code, recorder.Body.String())
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("redirect target requests = %d", redirectedRequests.Load())
	}
	for _, forbidden := range []string{secret, redirectTarget.URL, redirectOrigin.URL} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("redirect probe response leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestProviderRegistryDiscoveryCommitsBoundedModelsAndUsesRegistryProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}
	const (
		providerID = "provider-discovery-proxy"
		secret     = "synthetic-proxy-discovery-credential"
		rawCanary  = "raw-provider-body-canary"
	)
	var proxyRequests atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests.Add(1)
		if !r.URL.IsAbs() || r.URL.Host != "provider.invalid" || r.URL.Path != "/v1/models" {
			t.Errorf("proxied URL = %q", r.URL.String())
		}
		if authorization := r.Header.Get("Authorization"); authorization != "Bearer "+secret {
			t.Errorf("Authorization = %q", authorization)
		}
		if proxyAuthorization := r.Header.Get("Proxy-Authorization"); proxyAuthorization != "" {
			t.Errorf("Proxy-Authorization = %q", proxyAuthorization)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-z"},{"id":"model-a"},{"id":"model-a"}],"raw":"` + rawCanary + `"}`))
	}))
	defer proxyServer.Close()

	authority, initial, committed := openProviderRegistryProbeAuthorityV1(
		t, providerID, "http://provider.invalid/v1", proxyServer.URL, secret,
	)
	recorder := httptest.NewRecorder()
	httpapi.ProviderRegistryHandlers{Service: authority.Service()}.Handle(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			httpapi.ProviderRegistryPathV1+"/providers/"+providerID+"/discover-models",
			strings.NewReader(providerRegistryProbeExpectedBodyV1(initial, committed)),
		),
	)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"registryRevision":"2"`) ||
		!strings.Contains(recorder.Body.String(), `"revision":"2"`) ||
		!strings.Contains(recorder.Body.String(), `"generation":"1"`) ||
		!strings.Contains(recorder.Body.String(), `"models":["model-a","model-z"]`) {
		t.Fatalf("discovery response = (%d, %s)", recorder.Code, recorder.Body.String())
	}
	if proxyRequests.Load() != 1 {
		t.Fatalf("proxy requests = %d", proxyRequests.Load())
	}
	for _, forbidden := range []string{secret, rawCanary, "credentialRef", "rawBody"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("discovery response leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
	snapshot, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() after discovery error = %v", err)
	}
	discovered := snapshot.Providers[providerID]
	if strings.Join(discovered.Models, ",") != "model-a,model-z" || discovered.SelectedModel != "" ||
		discovered.CredentialRef != committed.CredentialRef || discovered.Generation != committed.Generation {
		t.Fatalf("discovered provider = %#v", discovered)
	}
}

func TestProviderRegistryProbeBoundsTimeoutResponseBytesAndModelCount(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		outcome := executeProviderRegistryProbeV1(
			context.Background(),
			domainregistry.Provider{Kind: "openai-compatible", Endpoint: server.URL + "/v1"},
			[]byte("synthetic-timeout-credential"),
		)
		if outcome.status != providerregistryapp.ProviderProbeStatusTimeout || outcome.code != 0 || len(outcome.models) != 0 {
			t.Fatalf("timeout outcome = %#v", outcome)
		}
	})

	t.Run("response bytes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(strings.Repeat("x", providerRegistryProbeMaxBodyBytesV1+1)))
		}))
		defer server.Close()
		outcome := executeProviderRegistryProbeV1(
			context.Background(),
			domainregistry.Provider{Kind: "openai-compatible", Endpoint: server.URL + "/v1"},
			[]byte("synthetic-bounded-response-credential"),
		)
		if outcome.status != providerregistryapp.ProviderProbeStatusInvalidResponse || outcome.code != http.StatusOK || len(outcome.models) != 0 {
			t.Fatalf("oversized response outcome = %#v", outcome)
		}
	})

	t.Run("model count", func(t *testing.T) {
		var body strings.Builder
		body.WriteString(`{"data":[`)
		for index := 0; index <= domainregistry.MaxModels; index++ {
			if index != 0 {
				body.WriteByte(',')
			}
			_, _ = fmt.Fprintf(&body, `{"id":"model-%d"}`, index)
		}
		body.WriteString(`]}`)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body.String()))
		}))
		defer server.Close()
		outcome := executeProviderRegistryProbeV1(
			context.Background(),
			domainregistry.Provider{Kind: "openai-compatible", Endpoint: server.URL + "/v1"},
			[]byte("synthetic-bounded-model-count-credential"),
		)
		if outcome.status != providerregistryapp.ProviderProbeStatusInvalidResponse || outcome.code != http.StatusOK || len(outcome.models) != 0 {
			t.Fatalf("model-count outcome = %#v", outcome)
		}
	})

	for _, testCase := range []struct {
		name    string
		modelID string
	}{
		{name: "raw credential echo", modelID: "synthetic-response-echo-credential"},
		{name: "canonical base64 credential echo", modelID: base64.StdEncoding.EncodeToString([]byte("synthetic-response-echo-credential"))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			const credential = "synthetic-response-echo-credential"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"data":[{"id":%q}]}`, testCase.modelID)
			}))
			defer server.Close()
			outcome := executeProviderRegistryProbeV1(
				context.Background(),
				domainregistry.Provider{Kind: "openai-compatible", Endpoint: server.URL + "/v1"},
				[]byte(credential),
			)
			if outcome.status != providerregistryapp.ProviderProbeStatusInvalidResponse || outcome.code != http.StatusOK || len(outcome.models) != 0 {
				t.Fatalf("credential-echo outcome status=%q code=%d modelCount=%d", outcome.status, outcome.code, len(outcome.models))
			}
		})
	}
}

func openProviderRegistryProbeAuthorityV1(
	t *testing.T,
	providerID string,
	endpoint string,
	proxy string,
	secret string,
) (*providerRegistryAuthorityV1, domainregistry.Registry, domainregistry.Provider) {
	t.Helper()
	dataDir := t.TempDir()
	installSyntheticProviderRegistryFallbackMasterKey(t, dataDir)
	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
	}
	t.Cleanup(func() {
		if err := authority.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	initial, err := authority.Manager().Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	committed, err := authority.Manager().Connect(context.Background(), providerregistryapp.ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: initial.Revision, RegistryIncarnation: initial.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: "openai-compatible", Endpoint: endpoint, Proxy: proxy,
			Models: []string{"model-existing"}, SelectedModel: "model-existing",
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return authority, initial, committed
}

func providerRegistryProbeExpectedBodyV1(
	initial domainregistry.Registry,
	committed domainregistry.Provider,
) string {
	return fmt.Sprintf(
		`{"schemaVersion":1,"expected":{"registryRevision":"1","registryIncarnation":%q,"providerRevision":"1","providerGeneration":"1","providerIncarnation":%q,"providerCredentialPurpose":%q}}`,
		initial.Incarnation,
		committed.Incarnation,
		committed.CredentialPurpose,
	)
}
