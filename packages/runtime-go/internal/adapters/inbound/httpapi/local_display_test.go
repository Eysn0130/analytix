package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	fundscleaningapp "analytix.local/runtime-go/internal/app/fundscleaning"
	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	localdisplayapp "analytix.local/runtime-go/internal/app/localdisplay"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func localDisplayRequestPropertiesV1(value any) []string {
	typeOf := reflect.TypeOf(value)
	properties := make([]string, 0, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		properties = append(properties, typeOf.Field(index).Tag.Get("json"))
	}
	return properties
}

func TestLocalDisplayHTTPRequestPropertiesMatchCanonicalFamily(t *testing.T) {
	for _, test := range []struct {
		kind    string
		request any
	}{
		{domainlocaldisplay.KindImportMappingPreviewV1, importMappingPreviewRequestV1{}},
		{domainlocaldisplay.KindCleaningDiffPreviewV1, cleaningDiffPreviewRequestV1{}},
		{domainlocaldisplay.KindAcceptedSlotDisplayV1, acceptedSlotDisplayRequestV1{}},
	} {
		if got, want := localDisplayRequestPropertiesV1(test.request), domainlocaldisplay.ContractRequestPropertiesV1(test.kind); !reflect.DeepEqual(got, want) {
			t.Fatalf("HTTP request properties drifted for %s: got=%v want=%v", test.kind, got, want)
		}
	}
	direct := localDisplayRequestPropertiesV1(directSourcePreviewRequestV1{})
	if !reflect.DeepEqual(direct, []string{"kind", "workspaceRoot", "view", "fields", "rowOffset", "rowLimit", "displayMode"}) {
		t.Fatalf("Direct HTTP request admitted an unexpected Main-private envelope: %v", direct)
	}
	direct = append(direct[:1], direct[2:]...)
	if want := domainlocaldisplay.ContractRequestPropertiesV1(domainlocaldisplay.KindDirectSourcePreviewV1); !reflect.DeepEqual(direct, want) {
		t.Fatalf("Direct public request properties drifted after the sole Main-owned workspace augmentation: got=%v want=%v", direct, want)
	}
}

func TestLocalDisplayDirectRouteUsesCurrentCaseRequestOnly(t *testing.T) {
	handler := LocalDisplayHandlerV1{
		Service:                   localdisplayapp.NewService(nil, nil),
		LoadFrozenSecurityContext: nil,
	}
	request := httptest.NewRequest(
		http.MethodPost,
		LocalDisplayDirectPreviewPathV1,
		strings.NewReader(`{"kind":"direct_source_preview","workspaceRoot":`+strconv.Quote(t.TempDir())+`,"view":"transactions","fields":["account"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`),
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("direct route unexpectedly depended on frozen security loader: status=%d", response.Code)
	}
}

func TestLocalDisplayDirectRouteStrictDecodeAndBoundaries(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "entity reference is not accepted",
			body: `{"kind":"direct_source_preview","workspaceRoot":"/tmp/case-local-display","view":"transactions","fields":["account"],"rowOffset":0,"rowLimit":25,"displayMode":"full","entityRef":"synthetic"}`,
		},
		{
			name: "unknown field is not accepted",
			body: `{"kind":"direct_source_preview","workspaceRoot":"/tmp/case-local-display","view":"transactions","fields":["unknown"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`,
		},
		{
			name: "row limit is bounded",
			body: `{"kind":"direct_source_preview","workspaceRoot":"/tmp/case-local-display","view":"transactions","fields":["account"],"rowOffset":0,"rowLimit":101,"displayMode":"full"}`,
		},
		{
			name: "relative workspace is not accepted",
			body: `{"kind":"direct_source_preview","workspaceRoot":"case-local-display","view":"transactions","fields":["account"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`,
		},
		{
			name: "unclean workspace is not accepted",
			body: `{"kind":"direct_source_preview","workspaceRoot":"/tmp/../case-local-display","view":"transactions","fields":["account"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := LocalDisplayHandlerV1{Service: localdisplayapp.NewService(nil, nil)}
			request := httptest.NewRequest(http.MethodPost, LocalDisplayDirectPreviewPathV1, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "synthetic") {
				t.Fatalf("invalid direct request was not safely rejected: status=%d", response.Code)
			}
		})
	}
}

func TestLocalDisplayAcceptedRouteStillUsesCurrentSecurityLoader(t *testing.T) {
	handler := LocalDisplayHandlerV1{
		Service: localdisplayapp.NewService(nil, nil),
		LoadFrozenSecurityContext: func(string, string) (domainsecurity.TurnSecurityContext, error) {
			return domainsecurity.TurnSecurityContext{}, nil
		},
		LoadCurrentSecurityContext: nil,
	}
	request := httptest.NewRequest(
		http.MethodPost,
		LocalDisplayAcceptedSlotsPathV1,
		strings.NewReader(`{"kind":"accepted_slot_display","threadId":"thread-local-display","turnId":"turn-local-display","acceptedFinalDigest":"`+strings.Repeat("a", 64)+`","displayMode":"full"}`),
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("accepted route did not fail closed without current security loader: status=%d", response.Code)
	}
}

func TestTypedFamilyImportAndCleaningRoutesAreClosedWithoutAuthority(t *testing.T) {
	handler := LocalDisplayHandlerV1{
		Service: localdisplayapp.NewServiceWithTypedLocalDataSurface(
			nil, nil,
			localdisplayapp.ImportMappingPreviewDependenciesV1{},
			localdisplayapp.CleaningDiffPreviewDependenciesV1{},
			localdisplayapp.DirectSourcePreviewDependenciesV1{},
			localdisplayapp.AcceptedSlotDisplayDependenciesV1{},
		),
	}
	selector := "tlsel1_" + strings.Repeat("a", 64)
	for _, test := range []struct {
		name string
		path string
		body string
	}{
		{
			name: "import authority producer is absent",
			path: LocalDisplayImportMappingPathV1,
			body: `{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`,
		},
		{
			name: "cleaning authority producer is absent",
			path: LocalDisplayCleaningDiffPathV1,
			body: `{"kind":"cleaning_diff_preview","selector":"` + selector + `","fields":["account"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), selector) {
				t.Fatalf("unowned production authority did not fail closed: status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}

	for _, body := range []string{
		`{"kind":"unknown_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full"}`,
		`{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full","path":"/private/source.csv"}`,
		`{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full","caseId":"forged"}`,
		`{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full","principal":"forged"}`,
		`{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full","grant":"forged"}`,
		`{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full","snapshotId":"forged"}`,
		`{"kind":"import_mapping_preview","selector":"` + selector + `","fields":["sourceColumn"],"rowOffset":0,"rowLimit":25,"displayMode":"full","rawSQL":"select * from source"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, LocalDisplayImportMappingPathV1, strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "forged") ||
			strings.Contains(response.Body.String(), "/private/source.csv") {
			t.Fatalf("renderer-selected import authority was not rejected: status=%d body=%q", response.Code, response.Body.String())
		}
	}
}

func TestTypedLocalHTTPResponseLimitFailsClosed(t *testing.T) {
	response := httptest.NewRecorder()
	writeLocalDisplayResponseV1(response, map[string]string{
		"displayValue": strings.Repeat("SOURCE_EXACT_CANARY", 70_000),
	})
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "SOURCE_EXACT_CANARY") {
		t.Fatalf("oversized typed-local response escaped: status=%d", response.Code)
	}
}

func TestFundsImportStageRouteRejectsRendererSelectedAuthority(t *testing.T) {
	handler := LocalDisplayHandlerV1{
		Service:           localdisplayapp.NewService(nil, nil),
		FundsCSVAdmission: &fundscsvadmissionapp.ServiceV1{},
	}
	tests := []string{
		`{"workspaceRoot":"/tmp/case-local-display","sourcePath":"/tmp/case-local-display/source.csv","caseId":"forged"}`,
		`{"workspaceRoot":"case-local-display","sourcePath":"/tmp/case-local-display/source.csv"}`,
		`{"workspaceRoot":"/tmp/case-local-display","sourcePath":"source.csv"}`,
		`{"workspaceRoot":"/tmp/../case-local-display","sourcePath":"/tmp/case-local-display/source.csv"}`,
	}
	for _, body := range tests {
		request := httptest.NewRequest(http.MethodPost, HostFundsImportStagePathV1, strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "forged") {
			t.Fatalf("invalid funds CSV authority reached admission: status=%d", response.Code)
		}
	}
}

func TestFundsImportRoutesFailClosedWithoutService(t *testing.T) {
	handler := LocalDisplayHandlerV1{Service: localdisplayapp.NewService(nil, nil)}
	selector := "tlsel1_" + strings.Repeat("a", 64)
	for _, test := range []struct {
		path string
		body string
	}{
		{HostFundsImportStagePathV1, `{"workspaceRoot":"/tmp/case-local-display","sourcePath":"/tmp/case-local-display/source.csv"}`},
		{HostFundsImportConfirmPathV1, `{"selector":"` + selector + `"}`},
		{HostFundsImportCancelPathV1, `{"selector":"` + selector + `"}`},
		{HostFundsImportStatusPathV1, `{"selector":"` + selector + `"}`},
	} {
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusConflict {
			t.Fatalf("missing funds import service did not fail closed: path=%s status=%d", test.path, response.Code)
		}
	}
}

func TestFundsCleaningRoutesAreClosedAndFailClosedWithoutService(t *testing.T) {
	selector := "tlsel1_" + strings.Repeat("a", 64)
	handler := LocalDisplayHandlerV1{Service: localdisplayapp.NewService(nil, nil)}
	for _, test := range []struct {
		path       string
		body       string
		wantStatus int
	}{
		{HostFundsDeterministicCleaningPathV1, `{"workspaceRoot":"/tmp/case-local-display"}`, http.StatusOK},
		{HostFundsCleaningRevokePathV1, `{"selector":"` + selector + `"}`, http.StatusConflict},
	} {
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.wantStatus {
			t.Fatalf("missing cleaning service did not fail closed: path=%s status=%d", test.path, response.Code)
		}
	}

	handler.FundsCleaning = &fundscleaningapp.ServiceV1{}
	for _, test := range []struct {
		path string
		body string
	}{
		{HostFundsDeterministicCleaningPathV1, `{"workspaceRoot":"relative","caseId":"forged"}`},
		{HostFundsDeterministicCleaningPathV1, `{"workspaceRoot":"/tmp/case-local-display","snapshot":"forged"}`},
		{HostFundsCleaningRevokePathV1, `{"selector":"` + selector + `","path":"/private/source.csv"}`},
	} {
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "forged") ||
			strings.Contains(response.Body.String(), "/private/source.csv") {
			t.Fatalf("cleaning route accepted caller authority: path=%s status=%d", test.path, response.Code)
		}
	}
}

func TestFundsCleaningHTTPProjectsProvenPreCASFailure(t *testing.T) {
	response := httptest.NewRecorder()
	writeFundsCleaningRunResponseV1(
		response,
		fundscleaningapp.RunResultV1{},
		fundscleaningapp.ErrPreCASFailed,
	)
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body["status"] != "pre_cas_failed" || len(body) != 1 ||
		strings.Contains(response.Body.String(), "/tmp/") || strings.Contains(response.Body.String(), "dsv2_") {
		t.Fatalf("proven pre-CAS failure was misprojected: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFundsImportSelectorRoutesRejectExtraAuthority(t *testing.T) {
	handler := LocalDisplayHandlerV1{
		Service:           localdisplayapp.NewService(nil, nil),
		FundsCSVAdmission: &fundscsvadmissionapp.ServiceV1{},
	}
	selector := "tlsel1_" + strings.Repeat("a", 64)
	for _, path := range []string{HostFundsImportConfirmPathV1, HostFundsImportCancelPathV1, HostFundsImportStatusPathV1} {
		for _, body := range []string{
			`{"selector":"bad"}`,
			`{"selector":"` + selector + `","caseId":"forged"}`,
			`{"selector":"` + selector + `","sourcePath":"/private/source.csv"}`,
		} {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "forged") ||
				strings.Contains(response.Body.String(), "/private/source.csv") {
				t.Fatalf("selector route accepted caller authority: path=%s status=%d", path, response.Code)
			}
		}
	}
}
