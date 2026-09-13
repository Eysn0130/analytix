package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mediaexecutionapp "analytix.local/runtime-go/internal/app/mediaexecution"
)

type mediaExecutionHTTPStub struct {
	request mediaexecutionapp.Request
	result  mediaexecutionapp.Result
	err     error
}

func (stub *mediaExecutionHTTPStub) Execute(_ context.Context, request mediaexecutionapp.Request) (mediaexecutionapp.Result, error) {
	stub.request = request
	return stub.result, stub.err
}

func TestMediaExecutionHTTPAcceptsOnlyBoundedKeyFreeIntent(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47}
	service := &mediaExecutionHTTPStub{result: mediaexecutionapp.Result{Image: png, MIMEType: "image/png"}}
	body := `{"schemaVersion":1,"operation":"image.generate","prompt":"bounded prompt","size":"1024x1024","timeoutMs":5000}`
	recorder := httptest.NewRecorder()
	MediaExecutionHandlers{Service: service}.Handle(recorder, httptest.NewRequest(http.MethodPost, MediaExecutionPathV1, strings.NewReader(body)))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" ||
		service.request.Operation != mediaexecutionapp.OperationImageGenerate || service.request.Prompt != "bounded prompt" ||
		!strings.Contains(recorder.Body.String(), base64.StdEncoding.EncodeToString(png)) {
		t.Fatalf("media response/request mismatch: status=%d request=%#v body=%s", recorder.Code, service.request, recorder.Body.String())
	}
	for _, forbidden := range []string{"credentialRef", "Authorization", "endpoint", "proxy", "providerId"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("media response exposed %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestMediaExecutionHTTPRejectsProviderAuthorityFieldsAndProjectsStableErrors(t *testing.T) {
	t.Run("privacy failure survives the closed HTTP projection", func(t *testing.T) {
		service := &mediaExecutionHTTPStub{err: mediaexecutionapp.ErrPrivacyUnavailable}
		recorder := httptest.NewRecorder()
		MediaExecutionHandlers{Service: service}.Handle(recorder, httptest.NewRequest(http.MethodPost, MediaExecutionPathV1,
			strings.NewReader(`{"schemaVersion":1,"operation":"image.generate","prompt":"ok"}`)))
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"code":"privacy_unavailable"`) {
			t.Fatalf("privacy capability status was lost: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})
	for _, body := range []string{
		`{"schemaVersion":1,"operation":"image.generate","prompt":"ok","endpoint":"https://forbidden.invalid"}`,
		`{"schemaVersion":1,"operation":"image.generate","prompt":"ok","credentialRef":"forbidden"}`,
		`{"schemaVersion":1,"operation":"image.generate","prompt":"ok","proxy":"http://forbidden.invalid"}`,
	} {
		service := &mediaExecutionHTTPStub{}
		recorder := httptest.NewRecorder()
		MediaExecutionHandlers{Service: service}.Handle(recorder, httptest.NewRequest(http.MethodPost, MediaExecutionPathV1, strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest || service.request.Operation != "" || !strings.Contains(recorder.Body.String(), `"code":"validation_error"`) {
			t.Fatalf("forbidden media authority input accepted: status=%d request=%#v body=%s", recorder.Code, service.request, recorder.Body.String())
		}
	}

	const sentinel = "/private/customer-13900000017 raw-provider-body"
	service := &mediaExecutionHTTPStub{err: errors.New(sentinel)}
	recorder := httptest.NewRecorder()
	MediaExecutionHandlers{Service: service}.Handle(recorder, httptest.NewRequest(http.MethodPost, MediaExecutionPathV1,
		strings.NewReader(`{"schemaVersion":1,"operation":"image.generate","prompt":"ok"}`)))
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), sentinel) ||
		!strings.Contains(recorder.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("unsafe media error projection: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
