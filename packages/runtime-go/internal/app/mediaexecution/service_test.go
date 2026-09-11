package mediaexecution

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

type mediaRegistryStub struct {
	mu           sync.Mutex
	provider     domainregistry.Provider
	credential   string
	resolveCalls int
	validations  int
	validate     func(int) error
}

func (stub *mediaRegistryStub) ResolveSelectedMediaForExecution(context.Context) (providerregistryapp.ExecutionResolution, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.resolveCalls++
	return providerregistryapp.ExecutionResolution{
		RegistryRevision:    7,
		RegistryIncarnation: "reginc_media_test_123456",
		Provider:            stub.provider.Clone(),
		Credential:          []byte(stub.credential),
	}, nil
}

func (stub *mediaRegistryStub) ValidateMediaExecutionCurrent(_ context.Context, _ providerregistryapp.ExecutionAuthority) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.validations++
	if stub.validate != nil {
		return stub.validate(stub.validations)
	}
	return nil
}

func mediaProvider(endpoint, proxy, model string) domainregistry.Provider {
	return domainregistry.Provider{
		ID: "provider-media", Kind: "openai-compatible", Endpoint: endpoint, Proxy: proxy,
		Models: []string{"text-model"}, MediaModels: []string{model}, SelectedModel: "text-model", SelectedMedia: model,
		SelectedRoutes: []string{"primary"}, CredentialRef: "cred_media_test_1234567890",
		CredentialPurpose: "provider-api-key", Revision: 3, Generation: 4,
		Incarnation: "provinc_media_test_123456", Tombstone: false,
	}
}

func TestImageExecutionUsesCommittedMediaModelCredentialRouteAndProxy(t *testing.T) {
	const secret = "synthetic-media-secret"
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	var capturedAuthorization, capturedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthorization = r.Header.Get("Authorization")
		data := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(data)
		capturedBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer upstream.Close()

	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "http://proxy.registry.test:8080", "image-model-registry"), credential: secret}
	var observedProxy string
	executor := New(registry)
	executor.ClientFactory = func(proxy string, _ time.Duration) (*http.Client, error) {
		observedProxy = proxy
		return upstream.Client(), nil
	}
	result, err := executor.Execute(context.Background(), Request{
		Operation: OperationImageGenerate, Prompt: "bounded prompt", Size: "1024x1024", TimeoutMS: 5_000,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if capturedAuthorization != "Bearer "+secret || observedProxy != "http://proxy.registry.test:8080" {
		t.Fatalf("authority route mismatch: authorization=%q proxy=%q", capturedAuthorization, observedProxy)
	}
	if !strings.Contains(capturedBody, `"model":"image-model-registry"`) || strings.Contains(capturedBody, "caller-model") {
		t.Fatalf("request did not use committed media model: %s", capturedBody)
	}
	if string(result.Image) != string(png) || result.MIMEType != "image/png" || registry.resolveCalls != 1 || registry.validations != 3 {
		t.Fatalf("unexpected result/currentness: %#v resolves=%d validates=%d", result, registry.resolveCalls, registry.validations)
	}
}

func TestImageEditUsesCommittedEditEndpointAndMultipartModel(t *testing.T) {
	const secret = "synthetic-media-edit-secret"
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	var capturedPath, capturedAuthorization, capturedContentType, capturedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuthorization = r.Header.Get("Authorization")
		capturedContentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		capturedBody = string(data)
		clear(data)
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer upstream.Close()
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-edit-model"), credential: secret}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	result, err := executor.Execute(context.Background(), Request{
		Operation: OperationImageEdit, Prompt: "bounded edit", Size: "1024x1024",
		Images: []ReferenceImage{{
			Name: "reference.png", MIMEType: "image/png", DataBase64: base64.StdEncoding.EncodeToString(png),
		}},
	})
	if err != nil {
		t.Fatalf("Execute(image edit) error = %v", err)
	}
	if capturedPath != "/v1/images/edits" || capturedAuthorization != "Bearer "+secret ||
		!strings.HasPrefix(capturedContentType, "multipart/form-data;") ||
		!strings.Contains(capturedBody, "image-edit-model") || !strings.Contains(capturedBody, "reference.png") ||
		string(result.Image) != string(png) {
		t.Fatalf("image edit route mismatch: path=%q auth=%q contentType=%q body=%s result=%#v",
			capturedPath, capturedAuthorization, capturedContentType, capturedBody, result)
	}
}

func TestImageExecutionClassifiesMiniMaxFromCommittedProviderIdentity(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	var capturedPath, capturedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		capturedBody = string(data)
		clear(data)
		_, _ = w.Write([]byte(`{"data":{"image_base64":["` + base64.StdEncoding.EncodeToString(png) + `"]},"base_resp":{"status_code":0}}`))
	}))
	defer upstream.Close()
	committed := mediaProvider(upstream.URL+"/v1", "", "image-01")
	committed.ID = "minimax"
	registry := &mediaRegistryStub{provider: committed, credential: "synthetic-minimax-media-secret"}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	result, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "bounded prompt"})
	if err != nil || capturedPath != "/v1/image_generation" || !strings.Contains(capturedBody, `"model":"image-01"`) ||
		string(result.Image) != string(png) {
		t.Fatalf("MiniMax committed identity mismatch: err=%v path=%q body=%s result=%#v", err, capturedPath, capturedBody, result)
	}
}

func TestSafeFilenameCannotInjectMultipartHeaders(t *testing.T) {
	name := safeFilename("private\"\r\nX-Injected: yes.png")
	if name != "private___X-Injected__yes.png" || strings.ContainsAny(name, "\r\n\"") {
		t.Fatalf("unsafe multipart filename projection: %q", name)
	}
}

func TestSpeechExecutionUsesCommittedMediaModelAndReturnsBoundedTranscript(t *testing.T) {
	const secret = "synthetic-speech-secret"
	var capturedAuthorization, capturedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthorization = r.Header.Get("Authorization")
		data := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(data)
		capturedBody = string(data)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":" registry transcript "}}]}`))
	}))
	defer upstream.Close()
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "mimo-v2.5-asr"), credential: secret}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	result, err := executor.Execute(context.Background(), Request{
		Operation:   OperationSpeechTranscribe,
		AudioBase64: base64.StdEncoding.EncodeToString([]byte("synthetic-audio")),
		MIMEType:    "audio/wav", Language: "zh", TimeoutMS: 5_000,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if capturedAuthorization != "Bearer "+secret || !strings.Contains(capturedBody, `"model":"mimo-v2.5-asr"`) ||
		strings.Contains(capturedBody, "must-not-use-settings-model") || result.Transcript != "registry transcript" {
		t.Fatalf("speech authority/result mismatch: auth=%q body=%s result=%#v", capturedAuthorization, capturedBody, result)
	}
}

func TestOpenAITranscriptionUsesOneLanguageAndCommittedMediaModel(t *testing.T) {
	const secret = "synthetic-openai-speech-secret"
	var capturedAuthorization, capturedPath string
	var capturedModels, capturedLanguages []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthorization = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		if err := r.ParseMultipartForm(MaxProviderBodyBytes); err != nil {
			t.Errorf("ParseMultipartForm() error = %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		capturedModels = append([]string(nil), r.MultipartForm.Value["model"]...)
		capturedLanguages = append([]string(nil), r.MultipartForm.Value["language"]...)
		_, _ = w.Write([]byte(`{"text":" registry transcript "}`))
	}))
	defer upstream.Close()
	registry := &mediaRegistryStub{
		provider: mediaProvider(upstream.URL+"/v1", "", "whisper-registry"), credential: secret,
	}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	result, err := executor.Execute(context.Background(), Request{
		Operation: OperationSpeechTranscribe, AudioBase64: base64.StdEncoding.EncodeToString([]byte("synthetic-audio")),
		MIMEType: "audio/wav", Language: "zh", TimeoutMS: 5_000,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if capturedAuthorization != "Bearer "+secret || capturedPath != "/v1/audio/transcriptions" ||
		len(capturedModels) != 1 || capturedModels[0] != "whisper-registry" ||
		len(capturedLanguages) != 1 || capturedLanguages[0] != "zh" || result.Transcript != "registry transcript" {
		t.Fatalf("multipart authority mismatch: auth=%q path=%q models=%v languages=%v result=%#v",
			capturedAuthorization, capturedPath, capturedModels, capturedLanguages, result)
	}
}

func TestImageRedirectIsBlockedAndNeverForwardsCredential(t *testing.T) {
	const secret = "synthetic-redirect-secret"
	redirectAuthorization := "not-called"
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		redirectAuthorization = r.Header.Get("Authorization")
	}))
	defer redirectTarget.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, redirectTarget.URL, http.StatusFound)
	}))
	defer upstream.Close()
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: secret}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	_, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "prompt"})
	if !errors.Is(err, ErrProviderFailed) || redirectAuthorization != "not-called" {
		t.Fatalf("redirect was followed or misclassified: err=%v redirectAuthorization=%q", err, redirectAuthorization)
	}
}

func TestImageURLDownloadRevalidatesAndNeverForwardsCredential(t *testing.T) {
	const secret = "synthetic-download-secret"
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	downloadAuthorization := "not-called"
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer download.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"url":"` + download.URL + `"}]}`))
	}))
	defer upstream.Close()
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: secret}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	result, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "prompt"})
	if err != nil || downloadAuthorization != "" || string(result.Image) != string(png) || registry.resolveCalls != 2 || registry.validations != 5 {
		t.Fatalf("download authority mismatch: err=%v auth=%q result=%#v resolves=%d validates=%d", err, downloadAuthorization, result, registry.resolveCalls, registry.validations)
	}
}

func TestImageExecutionDiscardsBodyAfterAuthorityDrift(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	headersValidated := make(chan struct{})
	releaseBody := make(chan struct{})
	defer func() {
		select {
		case <-releaseBody:
		default:
			close(releaseBody)
		}
	}()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-releaseBody
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(png) + `"}]}`))
	}))
	defer upstream.Close()
	var drifted atomic.Bool
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: "synthetic"}
	registry.validate = func(call int) error {
		if call == 2 {
			close(headersValidated)
			return nil
		}
		if drifted.Load() {
			return errors.New("synthetic registry drift")
		}
		return nil
	}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	type outcome struct {
		result Result
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "prompt"})
		finished <- outcome{result: result, err: err}
	}()
	awaitSignal(t, headersValidated, "image response headers")
	drifted.Store(true)
	close(releaseBody)
	completed := awaitOutcome(t, finished, "image response body")
	if !errors.Is(completed.err, ErrAuthorityChanged) || len(completed.result.Image) != 0 || completed.result.MIMEType != "" || registry.validations != 3 {
		t.Fatalf("drifted image body escaped: err=%v result=%#v validates=%d", completed.err, completed.result, registry.validations)
	}
}

func TestSpeechExecutionDiscardsBodyAfterAuthorityDrift(t *testing.T) {
	headersValidated := make(chan struct{})
	releaseBody := make(chan struct{})
	defer func() {
		select {
		case <-releaseBody:
		default:
			close(releaseBody)
		}
	}()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-releaseBody
		_, _ = w.Write([]byte(`{"text":"stale transcript"}`))
	}))
	defer upstream.Close()
	var drifted atomic.Bool
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "whisper-registry"), credential: "synthetic"}
	registry.validate = func(call int) error {
		if call == 2 {
			close(headersValidated)
			return nil
		}
		if drifted.Load() {
			return errors.New("synthetic registry drift")
		}
		return nil
	}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	type outcome struct {
		result Result
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := executor.Execute(context.Background(), Request{
			Operation: OperationSpeechTranscribe, AudioBase64: base64.StdEncoding.EncodeToString([]byte("synthetic-audio")), MIMEType: "audio/wav",
		})
		finished <- outcome{result: result, err: err}
	}()
	awaitSignal(t, headersValidated, "speech response headers")
	drifted.Store(true)
	close(releaseBody)
	completed := awaitOutcome(t, finished, "speech response body")
	if !errors.Is(completed.err, ErrAuthorityChanged) || completed.result.Transcript != "" || registry.validations != 3 {
		t.Fatalf("drifted transcript escaped: err=%v result=%#v validates=%d", completed.err, completed.result, registry.validations)
	}
}

func TestImageDownloadDiscardsBodyAfterAuthorityDrift(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0, 0, 0, 0, 0, 0, 0, 0}
	headersValidated := make(chan struct{})
	releaseBody := make(chan struct{})
	defer func() {
		select {
		case <-releaseBody:
		default:
			close(releaseBody)
		}
	}()
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-releaseBody
		_, _ = w.Write(png)
	}))
	defer download.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"url":"` + download.URL + `"}]}`))
	}))
	defer upstream.Close()
	var drifted atomic.Bool
	registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: "synthetic"}
	registry.validate = func(call int) error {
		if call == 4 {
			close(headersValidated)
			return nil
		}
		if drifted.Load() {
			return errors.New("synthetic registry drift")
		}
		return nil
	}
	executor := New(registry)
	executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
	type outcome struct {
		result Result
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "prompt"})
		finished <- outcome{result: result, err: err}
	}()
	awaitSignal(t, headersValidated, "image download response headers")
	drifted.Store(true)
	close(releaseBody)
	completed := awaitOutcome(t, finished, "image download response body")
	if !errors.Is(completed.err, ErrAuthorityChanged) || len(completed.result.Image) != 0 || completed.result.MIMEType != "" || registry.validations != 5 {
		t.Fatalf("drifted downloaded image escaped: err=%v result=%#v validates=%d", completed.err, completed.result, registry.validations)
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func awaitOutcome[T any](t *testing.T, outcome <-chan T, label string) T {
	t.Helper()
	select {
	case value := <-outcome:
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
		var zero T
		return zero
	}
}

func TestMediaExecutionRejectsPreSendAndPostResponseAuthorityDrift(t *testing.T) {
	for _, test := range []struct {
		name      string
		driftAt   int
		wantCalls int
	}{
		{name: "pre-send", driftAt: 1, wantCalls: 0},
		{name: "post-response", driftAt: 2, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				_, _ = w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo="}]}`))
			}))
			defer upstream.Close()
			registry := &mediaRegistryStub{provider: mediaProvider(upstream.URL+"/v1", "", "image-model"), credential: "synthetic"}
			registry.validate = func(call int) error {
				if call == test.driftAt {
					return errors.New("synthetic registry drift")
				}
				return nil
			}
			executor := New(registry)
			executor.ClientFactory = func(string, time.Duration) (*http.Client, error) { return upstream.Client(), nil }
			result, err := executor.Execute(context.Background(), Request{Operation: OperationImageGenerate, Prompt: "prompt"})
			if !errors.Is(err, ErrAuthorityChanged) || len(result.Image) != 0 || calls != test.wantCalls {
				t.Fatalf("drift was not fail-closed: err=%v result=%#v calls=%d", err, result, calls)
			}
		})
	}
}
