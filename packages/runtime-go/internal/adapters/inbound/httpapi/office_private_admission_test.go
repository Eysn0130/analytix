package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type officeAdmissionFixture struct {
	calls int
	drift bool
}

func (f *officeAdmissionFixture) Current(context.Context) bool {
	f.calls++
	return !f.drift || f.calls < 2
}
func (*officeAdmissionFixture) Root() string {
	return "/synthetic/Analytix.app/Contents/Resources/office-private"
}
func (*officeAdmissionFixture) QualificationDigest() string { return strings.Repeat("a", 64) }
func TestOfficePrivateAdmissionProtectedRoute(t *testing.T) {
	const nonce = "82e277b0-85ad-4ebf-8d7b-8541718bdc4b"
	body := `{"requestId":"` + nonce + `"}`
	for _, tc := range []struct {
		name, body, token, header, method, suffix string
		drift                                     bool
		status                                    int
	}{
		{name: "current", body: body, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "POST", status: 200},
		{name: "no bearer", body: body, header: LocalDisplayHeaderValueV1, method: "POST", status: 401},
		{name: "no typed authority", body: body, token: "synthetic", method: "POST", status: 403},
		{name: "get", body: body, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "GET", status: 405},
		{name: "caller root", body: `{"requestId":"` + nonce + `","root":"/fake"}`, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "POST", status: 503},
		{name: "duplicate", body: `{"requestId":"` + nonce + `","requestId":"` + nonce + `"}`, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "POST", status: 503},
		{name: "query", body: body, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "POST", suffix: "?root=/fake", status: 503},
		{name: "no nonce", body: `{}`, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "POST", status: 503},
		{name: "drift", body: body, token: "synthetic", header: LocalDisplayHeaderValueV1, method: "POST", drift: true, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assets := &officeAdmissionFixture{drift: tc.drift}
			mux := LocalDisplayMuxV1{RuntimeToken: "synthetic", LocalDisplay: LocalDisplayHandlerV1{OfficePrivateAdmission: OfficePrivateAdmissionHandler{Assets: assets}}}
			req := httptest.NewRequest(tc.method, OfficePrivateAdmissionPath+tc.suffix, strings.NewReader(tc.body))
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.header != "" {
				req.Header.Set(LocalDisplayHeaderV1, tc.header)
			}
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != tc.status {
				t.Fatalf("status %d, expected %d", recorder.Code, tc.status)
			}
			if tc.status == 200 {
				var result map[string]any
				if json.Unmarshal(recorder.Body.Bytes(), &result) != nil || len(result) != 4 || result["requestId"] != nonce || result["root"] != assets.Root() || result["qualificationDigest"] != assets.QualificationDigest() {
					t.Fatal("invalid current witness")
				}
			} else if strings.Contains(recorder.Body.String(), "/synthetic/") {
				t.Fatal("failure leaked private root")
			}
			if (tc.status == http.StatusUnauthorized || tc.status == http.StatusForbidden) && assets.calls != 0 {
				t.Fatal("unauthorized request reached private witness")
			}
		})
	}
}
func TestOfficePrivateAdmissionUnavailableWithoutComposition(t *testing.T) {
	req := httptest.NewRequest("POST", OfficePrivateAdmissionPath, strings.NewReader(`{"requestId":"82e277b0-85ad-4ebf-8d7b-8541718bdc4b"}`))
	recorder := httptest.NewRecorder()
	LocalDisplayHandlerV1{}.ServeHTTP(recorder, req)
	if recorder.Code != 503 || strings.Contains(recorder.Body.String(), "root") {
		t.Fatal("uncomposed private admission did not fail closed")
	}
}

func TestOfficePrivateAdmissionNeverUsesInsecureBypass(t *testing.T) {
	for _, token := range []string{"", "synthetic"} {
		assets := &officeAdmissionFixture{}
		mux := LocalDisplayMuxV1{RuntimeToken: token, Insecure: true, LocalDisplay: LocalDisplayHandlerV1{OfficePrivateAdmission: OfficePrivateAdmissionHandler{Assets: assets}}}
		req := httptest.NewRequest("POST", OfficePrivateAdmissionPath, strings.NewReader(`{"requestId":"82e277b0-85ad-4ebf-8d7b-8541718bdc4b"}`))
		req.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
		if token == "" {
			req.Header.Set("Authorization", "Bearer ")
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		if recorder.Code != 401 || assets.calls != 0 {
			t.Fatal("diagnostic insecure mode reached private assets")
		}
	}
}
