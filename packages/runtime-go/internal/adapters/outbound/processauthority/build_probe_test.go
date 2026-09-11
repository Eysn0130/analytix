//go:build analytix_native_build_probe && !analytix_prod

package processauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	buildProbeTestManifest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	buildProbeTestTarget   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	buildProbeTestNonce    = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

var (
	_ func(context.Context, *os.File, BuildProbeRequestV1, BuildProbeProtocol) (BuildProbeReceipt, error) = ProbePinnedBuildArtifact
	_ func(context.Context) (BuildProbeAuthorityIdentityV1, error)                                        = CurrentBuildProbeAuthorityIdentity
)

func TestBuildProbeExportedAPISurfaceIsClosed(t *testing.T) {
	requestType := reflect.TypeOf(BuildProbeRequestV1{})
	wantFields := []struct {
		name string
		json string
	}{
		{"Kind", "kind"},
		{"SchemaVersion", "schema_version"},
		{"ComponentID", "component_id"},
		{"ManifestSHA256", "manifest_sha256"},
		{"ExpectedExecutableSHA256", "expected_executable_sha256"},
		{"ExpectedExecutableSize", "expected_executable_size"},
		{"RequestNonce", "request_nonce"},
	}
	if requestType.NumField() != len(wantFields) {
		t.Fatalf("BuildProbeRequestV1 fields = %d, want %d", requestType.NumField(), len(wantFields))
	}
	for index, want := range wantFields {
		field := requestType.Field(index)
		if field.Name != want.name || field.Tag.Get("json") != want.json {
			t.Fatalf("request field %d = %s/%q, want %s/%q", index, field.Name, field.Tag.Get("json"), want.name, want.json)
		}
	}
	identityType := reflect.TypeOf(BuildProbeAuthorityIdentityV1{})
	if identityType.NumField() != 3 || identityType.Field(0).Name != "SHA256" ||
		identityType.Field(1).Name != "Size" || identityType.Field(2).Name != "HostTarget" {
		t.Fatalf("authority identity surface drifted: %v", identityType)
	}
}

func TestBuildProbeExportedAPIFailsClosedOutsideAuthorityMain(t *testing.T) {
	if currentBuildProbeMainAllowed() {
		t.Fatal("processauthority test binary was accepted as the build-probe authority main")
	}
	if _, err := CurrentBuildProbeAuthorityIdentity(context.Background()); !errors.Is(err, ErrBuildProbeInvalid) {
		t.Fatalf("current authority identity outside exact main error = %v", err)
	}
	request := validBuildProbeTestRequest(BuildProbeDataEngine)
	if _, err := ProbePinnedBuildArtifact(
		context.Background(), os.Stdin, request,
		BuildProbeProtocol{
			EncodePing:       func(string) ([]byte, error) { return []byte("{}\n"), nil },
			ValidateReady:    func([]byte, string, string, int) bool { return true },
			ValidateResponse: func([]byte, string, string, int) bool { return true },
		},
	); !errors.Is(err, ErrBuildProbeInvalid) {
		t.Fatalf("artifact probe outside exact main error = %v", err)
	}
}

func validBuildProbeTestRequest(component BuildProbeComponent) buildProbeRequest {
	return buildProbeRequest{
		Kind:                     "analytix_native_build_probe_request",
		SchemaVersion:            1,
		ComponentID:              string(component),
		ManifestSHA256:           buildProbeTestManifest,
		ExpectedExecutableSHA256: buildProbeTestTarget,
		ExpectedExecutableSize:   4096,
		RequestNonce:             buildProbeTestNonce,
	}
}

func encodeBuildProbeTestRequest(t *testing.T, request buildProbeRequest) []byte {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return append(body, '\n')
}

func TestBuildProbeRequestIsClosedAndBounded(t *testing.T) {
	valid := encodeBuildProbeTestRequest(t, validBuildProbeTestRequest(BuildProbeDataEngine))
	if request, err := readBuildProbeRequest(context.Background(), bytes.NewReader(valid)); err != nil || request.ComponentID != string(BuildProbeDataEngine) {
		t.Fatalf("valid request rejected: request=%+v err=%v", request, err)
	}

	cases := map[string][]byte{
		"missing newline": valid[:len(valid)-1],
		"second frame":    append(append(append([]byte(nil), valid...), []byte("{}")...), '\n'),
		"carriage return": append(append([]byte(nil), valid[:len(valid)-1]...), '\r', '\n'),
		"unknown field":   bytes.Replace(valid, []byte(`"request_nonce"`), []byte(`"unknown":true,"request_nonce"`), 1),
		"duplicate field": bytes.Replace(valid, []byte(`"request_nonce"`), []byte(`"kind":"analytix_native_build_probe_request","request_nonce"`), 1),
		"oversized":       append(bytes.Repeat([]byte{' '}, BuildProbeRequestLimit), '\n'),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readBuildProbeRequest(context.Background(), bytes.NewReader(payload)); err == nil {
				t.Fatal("host accepted a non-closed build probe request")
			}
		})
	}
}

func TestBuildProbeRequestAuthorityFieldsFailClosed(t *testing.T) {
	cases := map[string]buildProbeRequest{
		"unknown component": func() buildProbeRequest {
			request := validBuildProbeTestRequest(BuildProbeDataEngine)
			request.ComponentID = "other"
			return request
		}(),
		"wrong kind": func() buildProbeRequest {
			request := validBuildProbeTestRequest(BuildProbeDataEngine)
			request.Kind = "other"
			return request
		}(),
		"wrong schema": func() buildProbeRequest {
			request := validBuildProbeTestRequest(BuildProbeDataEngine)
			request.SchemaVersion = 2
			return request
		}(),
		"uppercase hash": func() buildProbeRequest {
			request := validBuildProbeTestRequest(BuildProbeDataEngine)
			request.ManifestSHA256 = strings.ToUpper(request.ManifestSHA256)
			return request
		}(),
		"zero size": func() buildProbeRequest {
			request := validBuildProbeTestRequest(BuildProbeDataEngine)
			request.ExpectedExecutableSize = 0
			return request
		}(),
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readBuildProbeRequest(context.Background(), bytes.NewReader(encodeBuildProbeTestRequest(t, request))); err == nil {
				t.Fatal("host accepted caller-controlled authority")
			}
		})
	}
}

func TestBuildProbePoliciesAreFixedForExactlyFourComponents(t *testing.T) {
	if len(buildProbePolicies) != 4 {
		t.Fatalf("policy count = %d, want 4", len(buildProbePolicies))
	}
	wantEnvironment := []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
	for _, component := range []BuildProbeComponent{
		BuildProbeImportAccelerator,
		BuildProbeCleaningOps,
		BuildProbeAnalysisCompute,
		BuildProbeDataEngine,
	} {
		policy, ok := buildProbePolicies[component]
		if !ok || policy.component != component || !validBuildProbeHex(policy.policySHA256) {
			t.Fatalf("invalid policy for %q: %+v", component, policy)
		}
		if strings.Join(policy.environment, "\x00") != strings.Join(wantEnvironment, "\x00") {
			t.Fatalf("environment for %q = %#v", component, policy.environment)
		}
		wantArguments := []string{"--analytix-native-probe"}
		if component == BuildProbeDataEngine {
			if policy.policySHA256 != "d3c87f7dc429e9451c8949ab93ee60ae8525542814859a8887ea3bedc2b5934d" {
				t.Fatalf("data-engine policy hash = %q", policy.policySHA256)
			}
		}
		if strings.Join(policy.arguments, "\x00") != strings.Join(wantArguments, "\x00") || len(policy.arguments) != len(wantArguments) {
			t.Fatalf("arguments for %q = %#v, want %#v", component, policy.arguments, wantArguments)
		}
	}
}

func TestBuildProbeReceiptRequiresEveryProof(t *testing.T) {
	request := validBuildProbeTestRequest(BuildProbeDataEngine)
	policy := buildProbePolicies[BuildProbeDataEngine]
	receipt := BuildProbeReceipt{
		Kind: "analytix_native_build_probe_receipt", SchemaVersion: 1, Status: "passed",
		ComponentID: request.ComponentID, RequestNonce: request.RequestNonce,
		ExecutableSHA256: request.ExpectedExecutableSHA256, ExecutableSize: request.ExpectedExecutableSize,
		ManifestSHA256: request.ManifestSHA256, PolicySHA256: policy.policySHA256,
		AuthoritySHA256: buildProbeTestTarget, HostPlatform: runtime.GOOS, HostArch: buildProbeHostArch(),
		LoadedImageBound: true, WorkingDirectoryBound: true, GuardianAuthenticated: true, ProcessTreeEmpty: true,
	}
	if !validBuildProbeReceipt(receipt, request) {
		t.Fatal("complete receipt rejected")
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	if decoded, decodeErr := decodeBuildProbeReceipt(body); decodeErr != nil || !validBuildProbeReceipt(decoded, request) {
		t.Fatalf("closed receipt frame rejected: receipt=%+v err=%v", decoded, decodeErr)
	}
	for name, hostile := range map[string][]byte{
		"unknown": bytes.Replace(body, []byte(`"status"`), []byte(`"unknown":true,"status"`), 1),
		"duplicate": bytes.Replace(
			body,
			[]byte(`"status"`),
			[]byte(`"kind":"analytix_native_build_probe_receipt","status"`),
			1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if _, decodeErr := decodeBuildProbeReceipt(hostile); decodeErr == nil {
				t.Fatal("non-closed receipt frame accepted")
			}
		})
	}
	receipt.ProcessTreeEmpty = false
	if validBuildProbeReceipt(receipt, request) {
		t.Fatal("receipt without process-tree proof accepted")
	}
	receipt.ProcessTreeEmpty = true
	receipt.AuthoritySHA256 = ""
	if validBuildProbeReceipt(receipt, request) {
		t.Fatal("receipt without coordinator loaded-image hash accepted")
	}
}

func TestBuildProbeReceiptBindsButDoesNotAuthorizeCallerManifestHash(t *testing.T) {
	request := validBuildProbeTestRequest(BuildProbeDataEngine)
	request.ManifestSHA256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	policy := buildProbePolicies[BuildProbeDataEngine]
	receipt := BuildProbeReceipt{
		Kind: "analytix_native_build_probe_receipt", SchemaVersion: 1, Status: "passed",
		ComponentID: request.ComponentID, RequestNonce: request.RequestNonce,
		ExecutableSHA256: request.ExpectedExecutableSHA256, ExecutableSize: request.ExpectedExecutableSize,
		ManifestSHA256: request.ManifestSHA256, PolicySHA256: policy.policySHA256,
		AuthoritySHA256: buildProbeTestTarget, HostPlatform: runtime.GOOS, HostArch: buildProbeHostArch(),
		LoadedImageBound: true, WorkingDirectoryBound: true, GuardianAuthenticated: true, ProcessTreeEmpty: true,
	}
	if !validBuildProbeReceipt(receipt, request) {
		t.Fatal("receipt did not bind the caller manifest assertion")
	}
	otherRequest := request
	otherRequest.ManifestSHA256 = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if validBuildProbeReceipt(receipt, otherRequest) {
		t.Fatal("receipt crossed caller manifest assertions")
	}
}

func TestBuildProbeHostArchitectureUsesManifestName(t *testing.T) {
	want := runtime.GOARCH
	if want == "amd64" {
		want = "x64"
	}
	if got := buildProbeHostArch(); got != want {
		t.Fatalf("host architecture = %q, want %q", got, want)
	}
}
