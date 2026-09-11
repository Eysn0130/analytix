//go:build darwin && analytix_prod

package runtimeapp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const b1ProcessEvidenceRoot = "/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6"
const b1ProcessHostParent = "/Users/sun/.codex/instruction-maintenance/2026-09-07-resume/own2-b1-host-private-roots"

// Exercise the real HTTP adapter without a listener, runtime, credential store,
// or Security command. Unexpected service operations fail through the nil
// embedded interface; only the in-memory Snapshot operation is implemented.
type b1RegistryRequestService struct {
	httpapi.ProviderRegistryService
	snapshotCalls int
}

func (service *b1RegistryRequestService) Snapshot(context.Context) (domainregistry.Registry, error) {
	service.snapshotCalls++
	return domainregistry.Registry{Revision: 1, Incarnation: "b1-synthetic-registry"}, nil
}

type b1MemoryRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip b1MemoryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestFundsAccountFlowB1RegistryRequestBodyContract(t *testing.T) {
	t.Run("explicit null rejected before service", func(t *testing.T) {
		service := &b1RegistryRequestService{}
		handler := httpapi.ProviderRegistryHandlers{Service: service}
		request := httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, strings.NewReader("null"))
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		response := httptest.NewRecorder()
		handler.Handle(response, request)
		var body map[string]any
		if json.Unmarshal(response.Body.Bytes(), &body) != nil {
			t.Fatal("real handler returned invalid error JSON")
		}
		nested, _ := body["error"].(map[string]any)
		if response.Code != http.StatusBadRequest || body["schemaVersion"] != float64(1) ||
			nested["code"] != "invalid_request" || service.snapshotCalls != 0 {
			t.Fatal("real handler did not reject GET null before service effects")
		}
		if b1PublicChainRequestFailure(http.MethodGet, httpapi.ProviderRegistryPathV1, response.Code, http.StatusOK, body) !=
			"B1 request GET: phase=OTHER status=400 want=200 code=invalid_request" {
			t.Fatal("real versioned Registry error was not preserved in bounded diagnostics")
		}
	})
	t.Run("shared helper preserves absent body", func(t *testing.T) {
		service := &b1RegistryRequestService{}
		handler := httpapi.ProviderRegistryHandlers{Service: service}
		calls := 0
		client := &http.Client{Transport: b1MemoryRoundTripper(func(outbound *http.Request) (*http.Response, error) {
			calls++
			var raw []byte
			if outbound.Body != nil {
				var err error
				raw, err = io.ReadAll(outbound.Body)
				_ = outbound.Body.Close()
				if err != nil {
					t.Fatal("synthetic transport could not read the request")
				}
			}
			// net/http normalizes an absent inbound body to http.NoBody.
			inbound := httptest.NewRequest(outbound.Method, outbound.URL.String(), bytes.NewReader(raw))
			inbound.Header = outbound.Header.Clone()
			response := httptest.NewRecorder()
			handler.Handle(response, inbound)
			t.Logf("real handler GET status=%d body_absent=%t literal_null=%t snapshot_calls=%d",
				response.Code, len(raw) == 0, bytes.Equal(raw, []byte("null")), service.snapshotCalls)
			return response.Result(), nil
		})}
		result := b1PublicChainRequest(t, client, "http://b1.invalid", http.MethodGet, httpapi.ProviderRegistryPathV1, nil, http.StatusOK)
		providers, ok := result["providers"].([]any)
		if !ok || len(providers) != 0 || result["registryRevision"] != "1" || calls != 1 || service.snapshotCalls != 1 {
			t.Fatal("bodyless shared helper did not read exactly one real Registry snapshot")
		}
	})
}

func TestFundsAccountFlowB1RequestJSONBodyPreserved(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		for _, body := range []map[string]any{
			{"credential": map[string]any{"synthetic": "b1-private-body-canary"}, "enabled": true}, {},
		} {
			calls := 0
			client := &http.Client{Transport: b1MemoryRoundTripper(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Method != method || request.Body == nil || request.Body == http.NoBody ||
					request.Header.Get("Authorization") != "Bearer "+DefaultRuntimeToken ||
					request.Header.Get("Content-Type") != "application/json" ||
					request.Header.Get(httpapi.LocalDisplayHeaderV1) != httpapi.LocalDisplayHeaderValueV1 {
					t.Fatal("JSON request lost its body, method or existing headers")
				}
				defer request.Body.Close()
				var received map[string]any
				if json.NewDecoder(request.Body).Decode(&received) != nil || !reflect.DeepEqual(received, body) {
					t.Fatal("JSON request payload was changed")
				}
				response := httptest.NewRecorder()
				response.Header().Set("Content-Type", "application/json")
				_, _ = response.WriteString(`{"ok":true}`)
				return response.Result(), nil
			})}
			result := b1PublicChainRequest(t, client, "http://b1.invalid", method, "/v1/local-display/synthetic", body, http.StatusOK)
			if calls != 1 || result["ok"] != true {
				t.Fatal("JSON request did not complete exactly once")
			}
		}
	}
}

func TestFundsAccountFlowB1RequestFailurePrivacy(t *testing.T) {
	const canary = "b1-private-response-canary"
	for _, code := range []string{"invalid_request", "unauthorized", "method_not_allowed", "not_found", "conflict",
		"persistence_failure", "verification_failure", "request_too_large"} {
		body := map[string]any{"schemaVersion": float64(1), "error": map[string]any{"code": code, "message": canary},
			"code": canary, "credential": canary, "path": "/private/" + canary}
		got := b1PublicChainRequestFailure(http.MethodGet, "/private/"+canary, 400, 200, body)
		if got != "B1 request GET: phase=OTHER status=400 want=200 code="+code || strings.Contains(got, canary) {
			t.Fatal("known nested code diagnostic lost its method or leaked a private field")
		}
	}
	for _, body := range []map[string]any{
		nil, {"code": "invalid_request"},
		{"schemaVersion": float64(2), "error": map[string]any{"code": "invalid_request"}},
		{"schemaVersion": float64(1), "error": map[string]any{"code": canary, "message": canary}},
		{"schemaVersion": float64(1), "error": canary},
		{"schemaVersion": float64(1), "error": map[string]any{"code": []any{canary}}},
	} {
		if b1PublicChainRequestFailure(canary, "/private/"+canary, 400, 200, body) != "B1 request UNKNOWN: phase=OTHER status=400 want=200 code=unavailable" {
			t.Fatal("unknown method, version, shape or code escaped the diagnostic allowlist")
		}
	}
	for path, phase := range map[string]string{
		"/v1/local-display/funds-import/stage":                     "IMPORT_STAGE",
		"/v1/local-display/funds-import/confirm":                   "IMPORT_CONFIRM",
		"/v1/local-display/funds-import/confirm?private=" + canary: "OTHER",
	} {
		got := b1PublicChainRequestFailure(http.MethodPost, path, 409, 200, nil)
		if got != "B1 request POST: phase="+phase+" status=409 want=200 code=unavailable" {
			t.Fatal("fixed import phase lost or private path admitted")
		}
	}
}

type b1ProcessObservation struct {
	SchemaVersion     int     `json:"schemaVersion"`
	ManifestSHA256    string  `json:"manifestSHA256"`
	Event             string  `json:"event"`
	Role              string  `json:"role"`
	PID               *int    `json:"pid"`
	ExitCode          *int    `json:"exitCode"`
	Signal            *string `json:"signal"`
	ProcessGroupEmpty *bool   `json:"processGroupEmpty"`
	Layer             string  `json:"layer"`
	Stage             string  `json:"stage"`
	Class             string  `json:"class"`
	ChildrenDrained   *bool   `json:"childrenDrained"`
	ResidualRoot      *string `json:"residualRoot"`
	ResidualWorkspace *string `json:"residualWorkspace"`
}

var b1ProcessRootPattern = regexp.MustCompile(`^` + regexp.QuoteMeta(b1ProcessHostParent) + `/analytix-own2-b1-process-[A-Za-z0-9]{6}$`)

func b1ChildStopObservationShape(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	fields := map[string]bool{"schemaVersion": false, "manifestSHA256": false, "event": false,
		"role": false, "pid": false, "exitCode": false, "signal": false, "processGroupEmpty": false}
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		seen, allowed := fields[name]
		if err != nil || !ok || !allowed || seen {
			return false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return false
		}
		fields[name] = true
	}
	for _, seen := range fields {
		if !seen {
			return false
		}
	}
	_, err := decoder.Token()
	return err == nil && decoder.Decode(new(any)) == io.EOF
}

func b1ValidNativeFailure(layer, stage, class string) bool {
	validStage := false
	switch layer {
	case "ADMISSION":
		switch stage {
		case "ARGUMENTS", "NATIVE_CALL", "OBJECT_VALIDATION", "SOURCE_STATS", "MATERIALIZATION", "SOURCE_ROWS", "RESULT_CONSTRUCTION", "UNKNOWN":
			validStage = true
		}
	case "RUNNER":
		switch stage {
		case "ADMISSION", "ACQUIRE", "PREVIOUS_SESSION", "SOURCE_MATERIALIZATION", "INPUT_DUPLICATE", "OUTPUT_PREPARE",
			"REQUEST_FRAME", "OUTPUT_HANDOFF", "SESSION_OPEN", "ROUND_TRIP", "SESSION_CLOSE", "POST_SESSION", "RESULT_PARSE",
			"OUTPUT_SEAL", "SOURCE_SETTLE", "INSTALL", "INSTALLED_VALIDATION", "CLEANUP", "UNKNOWN":
			validStage = true
		}
	}
	switch class {
	case "TERMINATION", "CANCELLED", "DEADLINE", "PROTOCOL", "REGISTRY", "REQUEST_INVALID", "SOURCE_NOT_FOUND", "SOURCE_MISMATCH", "SOURCE_CORRUPT", "UNAVAILABLE", "UNKNOWN":
		return validStage
	default:
		return false
	}
}

func (event b1ProcessObservation) diagnostic(manifestSHA, workspace string) (string, bool) {
	if event.SchemaVersion != 1 || !domainsecurity.IsSHA256Hex(manifestSHA) || event.ManifestSHA256 != manifestSHA {
		return "", false
	}
	base := "B1 process observation manifest_sha256=" + manifestSHA
	switch event.Event {
	case "CHILD_STOP_OUTCOME":
		if event.Role != "RUNTIME" && event.Role != "SIDECAR" && event.Role != "MATERIALIZER" ||
			event.PID != nil && *event.PID <= 0 ||
			event.ExitCode != nil && (*event.ExitCode < 0 || *event.ExitCode > 255) ||
			event.ExitCode != nil && event.Signal != nil ||
			event.PID == nil && (event.ExitCode != nil || event.Signal != nil || event.ProcessGroupEmpty != nil) {
			return "", false
		}
		pid, exitCode, signal, group := "UNKNOWN", "UNKNOWN", "UNKNOWN", "UNKNOWN"
		if event.PID != nil {
			pid = fmt.Sprint(*event.PID)
		}
		if event.ExitCode != nil {
			exitCode, signal = fmt.Sprint(*event.ExitCode), "NONE"
		}
		if event.Signal != nil {
			switch *event.Signal {
			case "SIGHUP", "SIGINT", "SIGQUIT", "SIGILL", "SIGTRAP", "SIGABRT", "SIGEMT", "SIGFPE",
				"SIGKILL", "SIGBUS", "SIGSEGV", "SIGSYS", "SIGPIPE", "SIGALRM", "SIGTERM", "SIGURG", "SIGSTOP", "SIGTSTP",
				"SIGCONT", "SIGCHLD", "SIGTTIN", "SIGTTOU", "SIGIO", "SIGXCPU", "SIGXFSZ", "SIGVTALRM", "SIGPROF",
				"SIGWINCH", "SIGINFO", "SIGUSR1", "SIGUSR2":
				signal = *event.Signal
			default:
				return "", false
			}
		}
		if event.ProcessGroupEmpty != nil {
			group = fmt.Sprint(*event.ProcessGroupEmpty)
		}
		return base + " event=CHILD_STOP_OUTCOME role=" + event.Role + " pid=" + pid +
			" exit=" + exitCode + " signal=" + signal + " process_group_empty=" + group, true
	case "NATIVE_FAILURE":
		if event.Role != "RUNTIME" || event.PID == nil || *event.PID <= 0 || !b1ValidNativeFailure(event.Layer, event.Stage, event.Class) {
			return "", false
		}
		return base + " event=NATIVE_FAILURE role=RUNTIME pid=" + fmt.Sprint(*event.PID) +
			" layer=" + event.Layer + " stage=" + event.Stage + " class=" + event.Class, true
	case "CHILD_STARTED", "CHILD_DRAINED", "CHILD_DRAIN_UNVERIFIED":
		switch event.Role {
		case "RUNTIME", "SIDECAR", "MATERIALIZER":
		default:
			return "", false
		}
		pid := "UNKNOWN"
		if event.PID != nil && *event.PID > 0 {
			pid = fmt.Sprint(*event.PID)
		} else if event.Event != "CHILD_DRAIN_UNVERIFIED" {
			return "", false
		}
		message := base + " event=" + event.Event + " role=" + event.Role + " pid=" + pid
		if event.Event == "CHILD_DRAINED" {
			message += " exit=0 process_group_empty=true"
		} else if event.Event == "CHILD_DRAIN_UNVERIFIED" {
			message += " exit=UNKNOWN process_group_empty=UNKNOWN"
		}
		return message, true
	case "FAILURE_RETAINED":
		root, scenario, drained := "UNKNOWN", "UNKNOWN", "UNKNOWN"
		if event.ResidualRoot != nil {
			if !b1ProcessRootPattern.MatchString(*event.ResidualRoot) {
				return "", false
			}
			root = *event.ResidualRoot
		}
		if event.ResidualWorkspace != nil {
			if *event.ResidualWorkspace != workspace {
				return "", false
			}
			scenario = workspace
		}
		if event.ChildrenDrained != nil {
			drained = fmt.Sprint(*event.ChildrenDrained)
		}
		return base + " event=FAILURE_RETAINED children_drained=" + drained + " residual_root=" + root + " residual_workspace=" + scenario, true
	case "CLEANUP_COMPLETE":
		return base + " event=CLEANUP_COMPLETE", true
	default:
		return "", false
	}
}

// os/exec alone owns this stderr writer. It never touches the stdout Scanner
// and never calls testing.T from a goroutine that could outlive close's timeout.
type b1ProcessObservations struct {
	mu                       sync.Mutex
	manifestSHA, workspace   string
	pending                  []byte
	lines                    []string
	dropping, invalid, final bool
	runtimePIDs              map[int]bool
}

func (observations *b1ProcessObservations) Write(body []byte) (int, error) {
	observations.mu.Lock()
	defer observations.mu.Unlock()
	for _, value := range body {
		if value != '\n' {
			if len(observations.pending) >= 4096 {
				observations.dropping, observations.invalid = true, true
			}
			if !observations.dropping {
				observations.pending = append(observations.pending, value)
			}
			continue
		}
		const prefix = "ANALYTIX_B1_PROCESS_OBSERVATION_V1 "
		var event b1ProcessObservation
		line := observations.pending
		if !observations.dropping && bytes.HasPrefix(line, []byte(prefix)) && len(observations.lines) < 24 {
			decoder := json.NewDecoder(bytes.NewReader(line[len(prefix):]))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&event) == nil && decoder.Decode(new(any)) == io.EOF {
				message, ok := event.diagnostic(observations.manifestSHA, observations.workspace)
				if event.Event == "CHILD_STOP_OUTCOME" && !b1ChildStopObservationShape(line[len(prefix):]) {
					ok = false
				}
				if event.Event == "NATIVE_FAILURE" && (event.PID == nil || !observations.runtimePIDs[*event.PID]) {
					ok = false
				}
				if ok {
					if event.Event == "CHILD_STARTED" && event.Role == "RUNTIME" {
						if observations.runtimePIDs == nil {
							observations.runtimePIDs = make(map[int]bool)
						}
						observations.runtimePIDs[*event.PID] = true
					}
					observations.lines = append(observations.lines, message)
					observations.final = event.Event == "CLEANUP_COMPLETE" || event.Event == "FAILURE_RETAINED"
				} else {
					observations.invalid = true
				}
			} else {
				observations.invalid = true
			}
		} else {
			observations.invalid = true
		}
		observations.pending = observations.pending[:0]
		observations.dropping = false
	}
	return len(body), nil
}

func (observations *b1ProcessObservations) snapshot() ([]string, bool, bool) {
	observations.mu.Lock()
	defer observations.mu.Unlock()
	return append([]string(nil), observations.lines...), observations.invalid || len(observations.pending) != 0, observations.final
}

func TestFundsAccountFlowB1ProcessObservationStream(t *testing.T) {
	const prefix = "ANALYTIX_B1_PROCESS_OBSERVATION_V1 "
	manifest := strings.Repeat("a", 64)
	workspace := b1ProcessEvidenceRoot + "/scenario-123"
	encode := func(event map[string]any) []byte {
		event["schemaVersion"], event["manifestSHA256"] = 1, manifest
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal("synthetic observation encoding failed")
		}
		return []byte(prefix + string(body) + "\n")
	}
	t.Run("failure before first stop retains startup and unverified drain", func(t *testing.T) {
		observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
		for _, event := range []map[string]any{
			{"event": "CHILD_STARTED", "role": "SIDECAR", "pid": 123},
			{"event": "CHILD_STARTED", "role": "RUNTIME", "pid": 456},
			{"event": "CHILD_DRAIN_UNVERIFIED", "role": "RUNTIME", "pid": 456},
			{"event": "CHILD_DRAINED", "role": "SIDECAR", "pid": 123},
			{"event": "FAILURE_RETAINED", "childrenDrained": false,
				"residualRoot": b1ProcessHostParent + "/analytix-own2-b1-process-A1b2C3", "residualWorkspace": workspace},
		} {
			// One byte at a time also covers split prefixes, UTF-8 and JSON lines.
			for _, value := range encode(event) {
				_, _ = observations.Write([]byte{value})
			}
		}
		lines, invalid, final := observations.snapshot()
		if invalid || !final || len(lines) != 5 || !strings.Contains(lines[1], "pid=456") ||
			!strings.Contains(lines[2], "exit=UNKNOWN process_group_empty=UNKNOWN") ||
			!strings.Contains(lines[3], "exit=0 process_group_empty=true") ||
			!strings.Contains(lines[4], "children_drained=false") {
			t.Fatal("failure lifecycle evidence was lost or promoted to success")
		}
	})
	t.Run("only the fixed host task root can be an inspection target", func(t *testing.T) {
		root := b1ProcessHostParent + "/analytix-own2-b1-process-A1b2C3"
		for _, target := range []string{root, "/private/tmp/analytix-own2-b1-process-A1b2C3",
			strings.Replace(root, "/own2-b1-host-private-roots/", "/other-parent/", 1), b1ProcessHostParent,
			b1ProcessEvidenceRoot + "/analytix-own2-b1-process-A1b2C3", strings.Replace(root, "analytix-own2", "Analytix-own2", 1),
			root + "/child", root + "/..", root + "x", workspace,
			"/Volumes/AnalytixCache/tmp/analytix-own2-b1-process-A1b2C3"} {
			observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
			_, _ = observations.Write(encode(map[string]any{"event": "FAILURE_RETAINED", "childrenDrained": false,
				"residualRoot": target, "residualWorkspace": workspace}))
			lines, invalid, final := observations.snapshot()
			if target == root {
				if invalid || !final || len(lines) != 1 || !strings.Contains(lines[0], "residual_root="+root) {
					t.Fatal("new exact host task inspection target was lost")
				}
			} else if !invalid || final || len(lines) != 0 {
				t.Fatal("out-of-scope root became an inspection target")
			}
		}
	})
	t.Run("failed child outcomes keep independent exit and drain evidence", func(t *testing.T) {
		for _, test := range []struct {
			pid, code, signal, group any
			want                     string
		}{
			{40, 1, nil, true, "pid=40 exit=1 signal=NONE process_group_empty=true"},
			{40, 1, nil, false, "pid=40 exit=1 signal=NONE process_group_empty=false"},
			{40, 1, nil, nil, "pid=40 exit=1 signal=NONE process_group_empty=UNKNOWN"},
			{40, nil, "SIGKILL", true, "pid=40 exit=UNKNOWN signal=SIGKILL process_group_empty=true"},
			{40, nil, nil, true, "pid=40 exit=UNKNOWN signal=UNKNOWN process_group_empty=true"},
			{nil, nil, nil, nil, "pid=UNKNOWN exit=UNKNOWN signal=UNKNOWN process_group_empty=UNKNOWN"},
			{40, 0, nil, true, "pid=40 exit=0 signal=NONE process_group_empty=true"},
		} {
			observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
			_, _ = observations.Write(encode(map[string]any{"event": "CHILD_STOP_OUTCOME", "role": "SIDECAR",
				"pid": test.pid, "exitCode": test.code, "signal": test.signal, "processGroupEmpty": test.group}))
			lines, invalid, final := observations.snapshot()
			if invalid || final || len(lines) != 1 || !strings.Contains(lines[0], test.want) || strings.Contains(lines[0], "event=CHILD_DRAINED") {
				t.Fatal("failed child exit or drain was discarded or promoted to success")
			}
			_, _ = observations.Write(encode(map[string]any{"event": "FAILURE_RETAINED", "childrenDrained": test.group == true}))
			_, invalid, final = observations.snapshot()
			if invalid || !final {
				t.Fatal("failure closure was lost after independent drain evidence")
			}
		}
	})
	t.Run("failed child outcomes reject incomplete contradictory and extra fields", func(t *testing.T) {
		fresh := func() map[string]any {
			return map[string]any{"event": "CHILD_STOP_OUTCOME", "role": "SIDECAR", "pid": 40,
				"exitCode": 1, "signal": nil, "processGroupEmpty": true}
		}
		var invalidBodies [][]byte
		for _, change := range []map[string]any{{"pid": nil}, {"pid": 0}, {"exitCode": -1}, {"exitCode": 256},
			{"exitCode": "1"}, {"signal": "SIGTERM"}, {"signal": "private-canary"},
			{"processGroupEmpty": "true"}, {"layer": "RUNNER"}, {"private": "private-canary"}} {
			value := fresh()
			for key, field := range change {
				value[key] = field
			}
			invalidBodies = append(invalidBodies, encode(value))
		}
		for _, key := range []string{"pid", "exitCode", "signal", "processGroupEmpty"} {
			value := fresh()
			delete(value, key)
			invalidBodies = append(invalidBodies, encode(value))
		}
		invalidBodies = append(invalidBodies, []byte(strings.TrimSuffix(string(encode(fresh())), "}\n")+`,"exitCode":0}`+"\n"))
		for _, body := range invalidBodies {
			observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
			_, _ = observations.Write(body)
			lines, invalid, final := observations.snapshot()
			if !invalid || final || len(lines) != 0 {
				t.Fatal("malformed child outcome escaped strict bounded observation")
			}
		}
	})
	t.Run("private malformed mismatched and oversized input is withheld", func(t *testing.T) {
		for _, body := range [][]byte{
			[]byte("private-canary\n"),
			encode(map[string]any{"event": "CHILD_STARTED", "role": "private-canary", "pid": 123}),
			encode(map[string]any{"event": "CHILD_STARTED", "role": "RUNTIME", "pid": 123, "credential": "private-canary"}),
			encode(map[string]any{"event": "FAILURE_RETAINED", "residualRoot": "/private/private-canary"}),
			encode(map[string]any{"event": "FAILURE_RETAINED", "residualWorkspace": workspace + "-private-canary"}),
			[]byte(strings.ReplaceAll(string(encode(map[string]any{"event": "CLEANUP_COMPLETE"})), manifest, strings.Repeat("b", 64))),
			[]byte(prefix + strings.Repeat("private-canary", 400) + "\n"),
			[]byte(prefix + "{\"event\":"),
		} {
			observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
			_, _ = observations.Write(body)
			lines, invalid, final := observations.snapshot()
			if !invalid || final || len(lines) != 0 || len(observations.pending) > 4096 {
				t.Fatal("unsafe observation admitted or pending storage unbounded")
			}
		}
	})
	t.Run("missing cleanup remains unknown", func(t *testing.T) {
		observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
		_, _ = observations.Write(encode(map[string]any{"event": "CHILD_DRAIN_UNVERIFIED", "role": "RUNTIME", "pid": nil}))
		lines, invalid, final := observations.snapshot()
		if invalid || final || len(lines) != 1 || !strings.Contains(lines[0], "pid=UNKNOWN exit=UNKNOWN") {
			t.Fatal("missing identity or final cleanup was promoted")
		}
	})
	t.Run("native failure requires the observed runtime and exact enum", func(t *testing.T) {
		for _, test := range []struct {
			layer, stage, class string
			pid                 int
			valid               bool
		}{
			{"RUNNER", "SESSION_OPEN", "REGISTRY", 456, true},
			{"ADMISSION", "UNKNOWN", "UNKNOWN", 456, true},
			{"RUNNER", "CLEANUP", "TERMINATION", 789, false},
			{"private-canary", "CLEANUP", "TERMINATION", 456, false},
			{"RUNNER", "private-canary", "TERMINATION", 456, false},
			{"RUNNER", "CLEANUP", "private-canary", 456, false},
			{"RUNNER", "ARGUMENTS", "UNKNOWN", 456, false},
		} {
			observations := &b1ProcessObservations{manifestSHA: manifest, workspace: workspace}
			_, _ = observations.Write(encode(map[string]any{"event": "CHILD_STARTED", "role": "RUNTIME", "pid": 456}))
			_, _ = observations.Write(encode(map[string]any{"event": "NATIVE_FAILURE", "role": "RUNTIME", "pid": test.pid,
				"layer": test.layer, "stage": test.stage, "class": test.class}))
			lines, invalid, final := observations.snapshot()
			expectedLines := 1
			if test.valid {
				expectedLines = 2
			}
			if invalid == test.valid || final || len(lines) != expectedLines {
				t.Fatal("native diagnostic lost runtime identity or fixed enum binding")
			}
			if test.valid && (!strings.Contains(lines[1], "manifest_sha256="+manifest) || strings.Contains(lines[1], "exit=0")) {
				t.Fatal("native failure lost source binding or was promoted to successful exit")
			}
		}
	})
}

func b1ProcessIdentityDifferences(value map[string]any) map[string]bool {
	differences, _ := value["identityDifferences"].(map[string]any)
	result := make(map[string]bool)
	for _, field := range []string{"pathChanged", "deviceChanged", "inodeChanged", "ownerChanged", "modeChanged",
		"linksChanged", "canonicalChanged", "kindChanged", "sizeInvalid"} {
		if changed, ok := differences[field].(bool); ok {
			result[field] = changed
		}
	}
	return result
}

func TestFundsAccountFlowB1IdentityDiagnosticPrivacy(t *testing.T) {
	value := map[string]any{"identityDifferences": map[string]any{
		"inodeChanged": true, "deviceChanged": false, "ownerChanged": "private-canary-value",
		"path": "/private/canary", "unexpected": true,
	}}
	if !reflect.DeepEqual(b1ProcessIdentityDifferences(value), map[string]bool{"inodeChanged": true, "deviceChanged": false}) {
		t.Fatal("identity diagnostic admitted private or unknown fields")
	}
	if len(b1ProcessIdentityDifferences(map[string]any{"identityDifferences": "private-canary-body"})) != 0 {
		t.Fatal("malformed identity diagnostic was admitted")
	}
}

// This regression uses only an in-memory HTTP request/recorder. It does not
// launch a Provider, runtime, sidecar or Keychain command.
func TestFundsAccountFlowB1SyntheticProviderCredentialBinding(t *testing.T) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal("synthetic credential generation failed")
	}
	credential := "b1-synthetic-" + hex.EncodeToString(random)
	clear(random)
	for _, test := range []struct {
		name, expected, supplied string
		status                   int
	}{
		{"random accepted", credential, credential, http.StatusOK},
		{"wrong rejected", credential, "incorrect-synthetic", http.StatusUnauthorized},
		{"legacy cannot replace random", credential, "test-only", http.StatusUnauthorized},
		{"historical unchanged", "test-only", "test-only", http.StatusOK},
		{"historical rejects random", "test-only", credential, http.StatusUnauthorized},
		{"unbound fixture rejected", "", "test-only", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &b1PublicChainModel{t: t, expectedCredential: test.expected}
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"B1_ORDINARY"}]}`))
			originalHeader := "Bearer " + test.supplied
			request.Header.Set("Authorization", originalHeader)
			response := httptest.NewRecorder()
			model.serve(response, request)
			if response.Code != test.status {
				t.Errorf("synthetic authorization status=%d want=%d", response.Code, test.status)
			}
			if request.Header.Get("Authorization") != originalHeader {
				t.Error("synthetic fixture rewrote authorization")
			}
			for _, private := range []string{credential, "test-only", "incorrect-synthetic"} {
				if strings.Contains(response.Body.String(), private) {
					t.Error("synthetic response exposed credential")
				}
			}
			if test.status == http.StatusOK {
				if len(model.requests) != 1 || !strings.Contains(response.Body.String(), "B1_ORDINARY_COMPLETE") {
					t.Error("authorized synthetic response missing")
				}
			} else if len(model.requests) != 0 || response.Body.Len() != 0 {
				t.Error("rejected synthetic request reached model or exposed body")
			}
		})
	}
}

// This test is a black-box client. Only an Owner-authorized invocation may
// execute it: it provisions a new synthetic profile/Keychain via the reviewed
// Node adapter and launches the actual packaged executable. No runtimeapp
// handler, native Owner opener, Registry seeder, or context observer is injected.
func TestFundsAccountFlowB1CurrentProcessPublicChain(t *testing.T) {
	if os.Getenv("ANALYTIX_B1_PROCESS_OWNER_RUN") != "OWN2-B1-PROCESS-CHAIN-20260910" {
		t.Skip("Owner-run process/provisioning authority is required")
	}
	manifestPath := os.Getenv("ANALYTIX_B1_PROCESS_INPUTS")
	manifestSHA := os.Getenv("ANALYTIX_B1_PROCESS_INPUTS_SHA256")
	if filepath.Dir(manifestPath) != b1ProcessEvidenceRoot || !domainsecurity.IsSHA256Hex(manifestSHA) {
		t.Fatal("B1 process inputs are not explicitly bound")
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil || domainsecurity.SHA256Hex(manifestBytes) != manifestSHA {
		t.Fatal("B1 process input fingerprint mismatch")
	}
	var input struct {
		Runtime string `json:"runtime"`
	}
	if json.Unmarshal(manifestBytes, &input) != nil {
		t.Fatal("B1 process manifest is invalid")
	}
	inspection, err := packagedauthority.InspectPackageV2(context.Background(), input.Runtime, "darwin", "arm64")
	if err != nil || inspection.Authority.Development == nil || inspection.Authority.Controlled != nil ||
		inspection.Publishable || inspection.FactToolsEnabled || inspection.PackageAnchor != "macos_nonpublishable_resource_seal" {
		t.Fatal("B1 current package did not pass normal production inspection")
	}
	root, err := os.MkdirTemp(b1ProcessEvidenceRoot, "scenario-")
	if err != nil {
		t.Fatal("B1 process scenario allocation failed")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	// The adapter owns exact cleanup after both real runtime PIDs drain. On
	// failure this directory is deliberately retained and reported to Owner.
	repository := filepath.Clean(filepath.Join("..", "..", "..", ".."))
	repository, err = filepath.Abs(repository)
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("B1 Node runtime is unavailable")
	}
	owner := b1StartProcessController(t, node, filepath.Join(repository, "scripts", "runtime-go-b1-process-harness.mjs"), manifestPath, manifestSHA, root)
	defer owner.close(t)
	credential := make([]byte, 32)
	if _, err := rand.Read(credential); err != nil {
		t.Fatal(err)
	}
	key := "b1-synthetic-" + hex.EncodeToString(credential)
	clear(credential)
	provisioned := owner.request(t, map[string]any{"action": "begin", "manifestSHA256": manifestSHA, "workspace": root,
		"privateMarkers": []string{rev14PrivateAccount, rev14PrivateCard, rev14PrivateCounterparty, rev14PrivateSourceSentinel, "cer1_", ".duckdb", key}})
	config := Config{DataDir: contracts.StringField(provisioned, "dataDir"), UserDataDir: contracts.StringField(provisioned, "userDataDir"),
		ProductionDurableRoot: filepath.Join(root, "durable"), ProviderID: "b1-process-synthetic", Model: "b1-process-model"}
	authorityKeyID := contracts.StringField(provisioned, "authorityKeyID")
	publicKey := contracts.StringField(provisioned, "authorityPublicKey")
	if !domainsecurity.IsSHA256Hex(authorityKeyID) {
		t.Fatal("B1 witnessed installation key is absent")
	}
	started := owner.request(t, map[string]any{"action": "start"})
	baseURL := contracts.StringField(started, "url")
	client := &http.Client{Timeout: 60 * time.Second}
	model := &b1PublicChainModel{t: t, expectedCredential: key}
	defer model.diagnostics()
	var probeCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("B1 Provider authorization mismatch")
			http.Error(w, "denied", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			probeCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]any{"id": config.Model, "object": "model"}}})
			return
		}
		model.serve(w, r)
	}))
	defer provider.Close()
	config.BaseURL = provider.URL + "/v1"
	owner.request(t, map[string]any{"action": "begin-provider-write"})
	b1ProcessConnectProvider(t, client, baseURL, config, key)
	verifyProvider := func(expectedCalls int64) {
		t.Helper()
		result := owner.request(t, map[string]any{"action": "verify-provider"})
		transition, _ := result["keychainTransition"].(map[string]any)
		if probeCalls.Load() != expectedCalls || transition["stableReadback"] != true {
			t.Fatal("B1 protected Provider readback did not reach the exact credential-bound synthetic endpoint")
		}
		changed, ok := transition["inodeChanged"].(bool)
		if !ok {
			t.Fatal("B1 Keychain transition lacks privacy-safe identity evidence")
		}
		t.Logf("B1 protected Provider readback verified; inode_changed=%t", changed)
	}
	verifyProvider(1)
	keychainRoot := filepath.Join(config.DataDir, "private", "provider-secrets", "master-key")
	if _, err := os.Lstat(filepath.Join(keychainRoot, "master.key")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("B1 process selected forbidden fallback key")
	}
	if _, err := os.Stat(filepath.Join(keychainRoot, "explicit-task-keychain-binding.v1")); err != nil {
		t.Fatal("B1 process omitted explicit Keychain binding")
	}
	workspace := filepath.Join(root, "synthetic-case")
	runtimeSharedEvidenceWriteCaseBindingV2(t, workspace)
	sourcePath := filepath.Join(workspace, "baseline.csv")
	if err := os.WriteFile(sourcePath, rev14AccountFlowCSV(), 0o600); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body map[string]any, status int) map[string]any {
		return b1PublicChainRequest(t, client, baseURL, method, path, body, status)
	}
	// Cold protection is exercised before the first native import, without
	// advertising a source or asking the synthetic Provider to fabricate one.
	coldThread := request(http.MethodPost, "/v1/threads", map[string]any{"title": "B1 cold", "workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
	coldID := contracts.StringField(coldThread, "id")
	coldStart := request(http.MethodPost, "/v1/threads/"+coldID+"/turns", map[string]any{
		"prompt": "查询当前案件账户在指定期间的流入、流出、净额和交易笔数。", "mode": "agent", "async": true,
		"approvalPolicy": "auto", "sandboxMode": "workspace-write"}, http.StatusAccepted)
	coldTurn := b1WaitTerminal(t, client, baseURL, coldID, contracts.StringField(coldStart, "turnId"))
	coldBody, _ := json.Marshal(coldTurn)
	model.mu.Lock()
	coldCalls := len(model.requests)
	model.mu.Unlock()
	if coldCalls != 0 || probeCalls.Load() != 1 || !bytes.Contains(coldBody, []byte("当前案件资金分析来源在本轮未通过可用性核验")) {
		t.Fatal("B1 cold case did not fail closed before Provider dispatch")
	}
	if _, err := os.Stat(filepath.Join(config.DataDir, "private", "evidence-registry")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("B1 cold startup created Evidence Registry")
	}
	var turns []b1ProcessTurn
	waitTurn := func(threadID, turnID, phase string) {
		turn := b1ProcessTurn{threadID: threadID, turnID: turnID, caseFact: phase == "baseline" || phase == "evolved"}
		turns = append(turns, turn)
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			ok, err := b1ProcessTail(config, turn, authorityKeyID, publicKey)
			if err != nil {
				t.Fatal("B1 process durable tail is invalid: " + err.Error())
			}
			if ok {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("B1 process signed terminal/longitudinal completion was not observed")
	}
	assertDrained := func() {
		drain := owner.request(t, map[string]any{"action": "stop"})
		pid, ok := drain["pid"].(float64)
		if !ok || pid <= 0 || drain["exit"] != float64(0) || drain["processGroupEmpty"] != true {
			t.Fatal("B1 actual process drain proof is invalid")
		}
		t.Logf("B1 actual runtime PID=%d exit=0 process_group_empty=true", int(pid))
		for _, turn := range turns {
			ok, err := b1ProcessTail(config, turn, authorityKeyID, publicKey)
			if err != nil || !ok {
				t.Fatal("B1 drained process lacks authentic durable terminal closure")
			}
		}
		t.Logf("B1 process drained; durable closures=%d; no injected observer", len(turns))
	}
	b1AssertPublicChain(t, config, root, workspace, sourcePath, authorityKeyID, model, nil, b1PublicChainDriver{
		baseURL: func() string { return baseURL }, waitTurn: waitTurn,
		reopen: func() {
			assertDrained()
			started = owner.request(t, map[string]any{"action": "start"})
			baseURL = contracts.StringField(started, "url")
			verifyProvider(2)
			// No credential re-entry or Registry recreation on restart.
			snapshot := b1PublicChainRequest(t, client, baseURL, http.MethodGet, "/v1/provider-registry", nil, http.StatusOK)
			providers, _ := snapshot["providers"].([]any)
			if len(providers) != 1 {
				t.Fatal("B1 restart lost the unique Provider Registry")
			}
		},
	})
	assertDrained()
	if len(turns) != 3 {
		t.Fatal("B1 process completed an unexpected public-chain turn inventory")
	}
	finished := owner.request(t, map[string]any{"action": "finish"})
	if finished["defaultKeychainConfigurationPreserved"] != true {
		t.Fatal("B1 Keychain configuration was not preserved")
	}
	for _, name := range []string{"cleanup", "workspaceCleanup"} {
		proof, _ := finished[name].(map[string]any)
		manifest := contracts.StringField(proof, "manifestSHA256")
		if proof["removed"] != true || proof["residuals"] != float64(0) || !domainsecurity.IsSHA256Hex(manifest) {
			t.Fatal("B1 exact cleanup proof is invalid")
		}
		t.Logf("B1 %s removed=true residuals=0 manifest_sha256=%s", name, manifest)
	}
	owner.finished = true
	t.Log("B1 current-source native/current Go real-process public chain passed; exact private/scenario cleanup acknowledged")
}

type b1ProcessController struct {
	command      *exec.Cmd
	input        io.WriteCloser
	output       *bufio.Scanner
	finished     bool
	observations *b1ProcessObservations
}

func b1StartProcessController(t *testing.T, node, script, manifest, manifestSHA, workspace string) *b1ProcessController {
	t.Helper()
	command := exec.Command(node, script, "--owner-run", manifest)
	// TestMain isolates Go-only configuration by changing HOME. The Darwin
	// authority subprocess must use the actual OS account home; its explicit
	// Keychain binding, not HOME substitution, supplies profile isolation.
	account, err := user.Current()
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		t.Fatal("B1 OS account home is unavailable")
	}
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + account.HomeDir, "LANG=C", "LC_ALL=C"}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	observations := &b1ProcessObservations{manifestSHA: manifestSHA, workspace: workspace}
	command.Stderr = observations
	if err := command.Start(); err != nil {
		t.Fatal("B1 Owner controller launch failed")
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 65536)
	return &b1ProcessController{command: command, input: input, output: scanner, observations: observations}
}
func (controller *b1ProcessController) request(t *testing.T, request map[string]any) map[string]any {
	t.Helper()
	if err := json.NewEncoder(controller.input).Encode(request); err != nil {
		t.Fatal("B1 private control write failed")
	}
	type response struct {
		body []byte
		ok   bool
	}
	result := make(chan response, 1)
	go func() {
		ok := controller.output.Scan()
		result <- response{append([]byte(nil), controller.output.Bytes()...), ok}
	}()
	select {
	case got := <-result:
		var value map[string]any
		if !got.ok || json.Unmarshal(got.body, &value) != nil {
			t.Fatal("B1 Owner controller response unavailable")
		}
		if value["ok"] != true {
			t.Logf("B1 Owner adapter identity differences=%v", b1ProcessIdentityDifferences(value))
			// Fixed code plus generated root only; never serialize bootstrap or credential bodies.
			t.Logf("B1 Owner adapter residual root=%v workspace=%v drained=%v codes=%v", value["residualRoot"], value["residualWorkspace"], value["childrenDrained"], value["observedFailureCodes"])
			t.Fatal("B1 Owner adapter failed closed: " + contracts.StringField(value, "code"))
		}
		return value
	case <-time.After(10 * time.Minute):
		t.Fatal("B1 Owner controller command timed out; exact residual inspection required")
	}
	return nil
}
func (controller *b1ProcessController) close(t *testing.T) {
	_ = controller.input.Close()
	done := make(chan error, 1)
	go func() { done <- controller.command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Error("B1 Owner controller returned nonzero or failed to wait; no successful controller exit claim")
		}
	case <-time.After(130 * time.Second):
		t.Error("B1 Owner controller remains active; no cleanup claim")
	}
	lines, invalid, final := controller.observations.snapshot()
	for _, line := range lines {
		t.Log(line)
	}
	if invalid {
		t.Error("B1 process observation stream invalid; raw stderr withheld")
	}
	if !final {
		t.Log("B1 process cleanup outcome=UNKNOWN; final observation unavailable")
		if controller.finished {
			t.Error("B1 completed controller lacks final cleanup observation")
		}
	}
}

func b1ProcessConnectProvider(t *testing.T, client *http.Client, base string, config Config, key string) {
	t.Helper()
	request := func(method, path string, body map[string]any) map[string]any {
		return b1PublicChainRequest(t, client, base, method, path, body, http.StatusOK)
	}
	snapshot := request(http.MethodGet, "/v1/provider-registry", nil)
	expected := map[string]any{"registryRevision": snapshot["registryRevision"], "registryIncarnation": snapshot["registryIncarnation"],
		"providerRevision": "0", "providerGeneration": "0", "providerIncarnation": "", "providerCredentialPurpose": ""}
	request(http.MethodPost, "/v1/provider-registry", map[string]any{"schemaVersion": 1, "expected": expected,
		"provider": map[string]any{"id": config.ProviderID, "kind": "openai-compatible", "endpoint": config.BaseURL, "proxy": "",
			"models": []string{config.Model}, "mediaModels": []string{}, "selectedModel": config.Model, "selectedMediaModel": "", "selectedRoutes": []string{}},
		"credential": map[string]any{"kind": "set", "purpose": "provider-api-key", "valueBase64": base64.StdEncoding.EncodeToString([]byte(key))}})
	snapshot = request(http.MethodGet, "/v1/provider-registry", nil)
	providers, _ := snapshot["providers"].([]any)
	if len(providers) != 1 {
		t.Fatal("B1 normal Provider creation failed")
	}
	provider, _ := providers[0].(map[string]any)
	if provider["credentialConfigured"] != true || provider["id"] != config.ProviderID {
		t.Fatal("B1 synthetic credential was not protected by Registry")
	}
	expected = map[string]any{"registryRevision": snapshot["registryRevision"], "registryIncarnation": snapshot["registryIncarnation"],
		"providerRevision": provider["revision"], "providerGeneration": provider["generation"], "providerIncarnation": provider["incarnation"], "providerCredentialPurpose": provider["credentialPurpose"]}
	request(http.MethodPost, "/v1/provider-registry/providers/"+config.ProviderID+"/select", map[string]any{"schemaVersion": 1, "expected": expected})
}

type b1ProcessTurn struct {
	threadID, turnID string
	caseFact         bool
}

func b1ProcessCAS(root string) ([][]byte, error) {
	var bodies [][]byte
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return bodies, nil
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink in private CAS")
		}
		if entry.IsDir() || len(entry.Name()) != 69 || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		state, err := entry.Info()
		if err != nil || !state.Mode().IsRegular() || state.Size() > 16<<20 {
			return errors.New("invalid private CAS file")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		bodies = append(bodies, body)
		return nil
	})
	return bodies, err
}

// The tail check is read-only. Parsers verify canonical records, digests and
// Ed25519 signatures; signer identity is pinned to the same live witness. The
// longitudinal record itself is hash-chained, and is bound below to signed
// final claims/receipt and the actual thread continuation (not called signed).
func b1ProcessTail(config Config, turn b1ProcessTurn, keyID, publicKey string) (bool, error) {
	body, err := os.ReadFile(filepath.Join(config.ProductionDurableRoot, "threads", turn.threadID, "thread.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("thread read")
	}
	var thread map[string]any
	if json.Unmarshal(body, &thread) != nil {
		return false, errors.New("thread parse")
	}
	var publicTurn map[string]any
	for _, raw := range arrayMaps(thread["turns"]) {
		if raw["id"] == turn.turnID {
			publicTurn = raw
		}
	}
	if publicTurn == nil || publicTurn["status"] == "running" || publicTurn["status"] == "queued" {
		return false, nil
	}
	if publicTurn["status"] != "completed" {
		return false, errors.New("turn did not complete")
	}
	if status, found, terminal, err := appturn.InspectTerminalAuthorityV1(thread, turn.turnID); err != nil || !found || !terminal || status != "completed" {
		return false, errors.New("durable public terminal authority")
	}
	if !turn.caseFact {
		commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(publicTurn["generalTerminalPublication"])
		if err != nil {
			return false, nil
		}
		return commit.ThreadID == turn.threadID && commit.TurnID == turn.turnID && commit.TerminalStatus == "completed" && commit.AuthorityDigest != "", nil
	}
	read := func(leaf string) ([][]byte, error) {
		return b1ProcessCAS(filepath.Join(config.DataDir, "private", leaf))
	}
	files, err := read("accepted-finals/records")
	if err != nil {
		return false, errors.New("private final read")
	}
	var final *domainevidence.PrivateAcceptedFinalRecord
	for _, body := range files {
		r, err := domainevidence.ParsePrivateAcceptedFinalRecord(body)
		if err != nil {
			return false, errors.New("private final integrity")
		}
		if r.SecurityContext.ThreadID == turn.threadID && r.SecurityContext.TurnID == turn.turnID {
			if final != nil {
				return false, errors.New("duplicate private final")
			}
			copy := r
			final = &copy
		}
	}
	if final == nil {
		return false, nil
	}
	if final.AcceptedFinal.AuthorityKeyID != keyID || final.AcceptedFinal.AuthorityPublicKey != publicKey {
		return false, errors.New("final installation identity")
	}
	publicFinal, err := domainevidence.ParseAcceptedFinalRecord(publicTurn["acceptedFinal"])
	if err != nil || !reflect.DeepEqual(publicFinal, final.AcceptedFinal) {
		return false, errors.New("private final does not match durable public winner")
	}
	var disposition *domainevidence.AcceptedFinalDispositionRecord
	files, err = read("accepted-finals/dispositions")
	if err != nil {
		return false, errors.New("disposition read")
	}
	for _, body := range files {
		r, err := domainevidence.ParseAcceptedFinalDispositionRecord(body)
		if err != nil {
			return false, errors.New("disposition integrity")
		}
		if r.AcceptedFinalDigest == final.AcceptedFinal.RecordDigest && r.State == domainevidence.AcceptedFinalCommitted {
			if disposition != nil {
				return false, errors.New("duplicate committed disposition")
			}
			if r.AuthorityKeyID != keyID || r.AuthorityPublicKey != publicKey || r.WinnerDigest != final.AcceptedFinal.RecordDigest {
				return false, errors.New("disposition binding")
			}
			copy := r
			disposition = &copy
		}
	}
	if disposition == nil {
		return false, nil
	}
	var intent *domainturnterminal.TurnTerminalIntentV1
	files, err = read("turn-terminal-authority/intents")
	if err != nil {
		return false, errors.New("intent read")
	}
	for _, body := range files {
		r, err := domainturnterminal.ParseTurnTerminalIntentV1(body)
		if err != nil {
			return false, errors.New("intent integrity")
		}
		if r.AcceptedFinalDigest == final.AcceptedFinal.RecordDigest {
			if intent != nil {
				return false, errors.New("duplicate terminal intent")
			}
			if r.AuthorityKeyID != keyID || r.AuthorityPublicKey != publicKey || r.PrivateFinalStoreDigest != final.StoreDigest || !reflect.DeepEqual(r.SecurityContext, final.SecurityContext) {
				return false, errors.New("intent binding")
			}
			copy := r
			intent = &copy
		}
	}
	if intent == nil {
		return false, nil
	}
	var terminal *domainturnterminal.TurnTerminalDispositionV1
	files, err = read("turn-terminal-authority/dispositions")
	if err != nil {
		return false, errors.New("terminal read")
	}
	for _, body := range files {
		r, err := domainturnterminal.ParseTurnTerminalDispositionV1(body)
		if err != nil {
			return false, errors.New("terminal integrity")
		}
		if r.IntentID == intent.IntentID {
			if terminal != nil {
				return false, errors.New("duplicate terminal disposition")
			}
			if domainturnterminal.ValidateTurnTerminalDispositionForIntentV1(r, *intent) != nil || r.AcceptedFinalDispositionDigest != disposition.RecordDigest {
				return false, errors.New("terminal binding")
			}
			copy := r
			terminal = &copy
		}
	}
	if terminal == nil {
		return false, nil
	}
	files, err = read("provider-cache-telemetry/turn-closures")
	if err != nil {
		return false, errors.New("provider closure read")
	}
	closureFound := false
	for _, body := range files {
		r, err := domaincachetelemetry.ParseProviderTurnClosureV1(body)
		if err != nil {
			return false, errors.New("provider closure integrity")
		}
		if r.ClosureID == terminal.ProviderClosureID {
			if closureFound || domainturnterminal.ValidateTurnTerminalDispositionForAuthoritiesV1(*terminal, *intent, r, *disposition) != nil {
				return false, errors.New("provider closure binding")
			}
			closureFound = true
		}
	}
	if !closureFound {
		return false, nil
	}
	files, err = read("evidence-registry/capsules")
	if err != nil {
		return false, errors.New("receipt registry read")
	}
	receipts := map[string]string{}
	for _, body := range files {
		capsule, err := domainevidence.ParseEvidenceRegistryAuthorityCapsule(body)
		if err != nil {
			return false, errors.New("receipt registry integrity")
		}
		if capsule.Registry.StateDigest == final.RegistryHead.StateDigest {
			for _, entry := range capsule.Registry.Entries {
				if entry.Receipt != nil {
					receipts[entry.ReceiptID] = entry.Receipt.ReceiptDigest
				}
			}
		}
	}
	files, err = read("case-entity/thread-context-v1")
	if err != nil {
		return false, errors.New("longitudinal read")
	}
	var states []domaincaseentity.ThreadCaseContextRecord
	for _, body := range files {
		r, err := domaincaseentity.ParseThreadCaseContextRecordV1(body)
		if err != nil {
			return false, errors.New("longitudinal integrity")
		}
		if r.CaseID == final.SecurityContext.CaseID && r.CaseBindingHash == final.SecurityContext.CaseBindingHash && r.ThreadID == turn.threadID {
			states = append(states, r)
		}
	}
	if len(states) == 0 {
		return false, nil
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Generation < states[j].Generation })
	if states[0].Generation != 1 {
		return false, errors.New("longitudinal genesis missing")
	}
	for i := 1; i < len(states); i++ {
		if domaincaseentity.ValidateThreadCaseContextEvolutionV1(states[i-1], states[i]) != nil {
			return false, errors.New("longitudinal chain")
		}
	}
	latest := states[len(states)-1]
	currentness := domaincaseentity.SnapshotStaleV1
	if latest.CurrentDatasetSnapshotID == final.SecurityContext.DatasetSnapshotID {
		currentness = domaincaseentity.SnapshotCurrentV1
	}
	for _, claim := range final.Envelope.Claims {
		found := false
		for _, saved := range latest.Claims {
			if saved.ClaimReference == claim.ClaimID && saved.ClaimDigest == claim.RecordDigest && saved.DatasetSnapshotID == final.SecurityContext.DatasetSnapshotID && saved.Currentness == currentness {
				found = true
			}
		}
		if !found {
			return false, nil
		}
	}
	for _, id := range final.Envelope.EvidenceReceiptIDs {
		found := false
		for _, saved := range latest.Evidence {
			if saved.EvidenceReference == id && saved.EvidenceDigest == receipts[id] && receipts[id] != "" && saved.DatasetSnapshotID == final.SecurityContext.DatasetSnapshotID && saved.Currentness == currentness {
				found = true
			}
		}
		if !found {
			return false, nil
		}
	}
	return b1ProcessDisplayContinuationTail(thread, final, disposition, latest, receipts, currentness)
}

// The caller has already validated signed final/terminal/provider authority,
// the complete longitudinal evolution, and current claim/receipt state.
func b1ProcessDisplayContinuationTail(
	thread map[string]any,
	final *domainevidence.PrivateAcceptedFinalRecord,
	disposition *domainevidence.AcceptedFinalDispositionRecord,
	latest domaincaseentity.ThreadCaseContextRecord,
	receipts map[string]string,
	currentness string,
) (bool, error) {
	// Use the same validated envelope-to-slot projection as the production
	// finalizer. A partial answer may have slots; an aggregate without typed
	// eligibility has none. Neither variant nor claim count decides cardinality.
	slots, err := domainevidence.BuildAcceptedEntitySlotBindingsV1(final.Envelope)
	if err != nil {
		return false, errors.New("accepted final slot projection")
	}
	expected := make(map[string]domainevidence.AcceptedEntitySlotBindingV1, len(slots))
	for _, slot := range slots {
		expected[slot.SlotID] = slot
	}
	claims := make(map[string]string, len(final.Envelope.Claims))
	for _, claim := range final.Envelope.Claims {
		claims[claim.ClaimID] = claim.RecordDigest
	}
	seen := make(map[string]bool, len(slots))
	for _, binding := range latest.DisplayBindings {
		if binding.AcceptedFinalDigest != final.AcceptedFinal.RecordDigest {
			if binding.OriginalThreadID == final.SecurityContext.ThreadID && binding.OriginalTurnID == final.SecurityContext.TurnID {
				return false, errors.New("display final binding")
			}
			// Other turns' retained bindings remain subject to the caller's
			// full hash-chain/history validation, not this final's slot count.
			continue
		}
		slot, required := expected[binding.SlotID]
		if !required || seen[binding.SlotID] {
			return false, errors.New("unexpected or duplicate display slot")
		}
		if domaincaseentity.ValidateCaseAcceptedDisplayBindingV1(binding) != nil ||
			binding.CaseBindingHash != final.SecurityContext.CaseBindingHash ||
			binding.OriginalThreadID != final.SecurityContext.ThreadID || binding.OriginalTurnID != final.SecurityContext.TurnID ||
			binding.DispositionDigest != disposition.RecordDigest || binding.FinalGateVersion != final.AcceptedFinal.FinalGateVersion ||
			binding.ContextDigest != final.SecurityContext.ContextDigest || binding.ContextEpoch != final.SecurityContext.ContextEpoch ||
			binding.DatasetSnapshotID != final.SecurityContext.DatasetSnapshotID || binding.Currentness != currentness {
			return false, errors.New("longitudinal display binding")
		}
		if err := slot.UseReferenceV1(func(reference domaincaseentity.ReferenceV1) error {
			if reference != binding.EntityReference {
				return errors.New("display entity binding")
			}
			return nil
		}); err != nil {
			return false, errors.New("display entity binding")
		}
		claimSet := make(map[string]string, len(slot.ClaimIDs))
		for _, id := range slot.ClaimIDs {
			claimSet[id] = claims[id]
		}
		if len(binding.ClaimBindings) != len(claimSet) {
			return false, errors.New("display claim binding")
		}
		for _, claim := range binding.ClaimBindings {
			if claimSet[claim.ClaimReference] == "" || claimSet[claim.ClaimReference] != claim.ClaimDigest {
				return false, errors.New("display claim binding")
			}
		}
		receiptSet := make(map[string]string, len(slot.ReceiptIDs))
		for _, id := range slot.ReceiptIDs {
			receiptSet[id] = receipts[id]
		}
		if len(binding.EvidenceReceiptBindings) != len(receiptSet) {
			return false, errors.New("display receipt binding")
		}
		for _, evidence := range binding.EvidenceReceiptBindings {
			if receiptSet[evidence.EvidenceReference] == "" || receiptSet[evidence.EvidenceReference] != evidence.EvidenceDigest {
				return false, errors.New("display receipt binding")
			}
		}
		seen[binding.SlotID] = true
	}
	if len(seen) != len(expected) {
		return false, nil
	}
	continuation, err := appturn.BuildTaskContinuationSnapshotV1(thread)
	if err != nil {
		return false, errors.New("continuation digest")
	}
	for _, saved := range latest.Continuations {
		if saved.ContinuationDigest == continuation.StateDigest {
			return true, nil
		}
	}
	return false, nil
}

func arrayMaps(value any) []map[string]any {
	var result []map[string]any
	items, _ := value.([]any)
	for _, item := range items {
		if object, ok := item.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

// These fixtures exercise only the post-authority display/continuation seam.
// They construct valid envelopes and bindings, not signed-final acceptance.
type b1ProcessTailFixture struct {
	thread      map[string]any
	final       domainevidence.PrivateAcceptedFinalRecord
	disposition domainevidence.AcceptedFinalDispositionRecord
	latest      domaincaseentity.ThreadCaseContextRecord
	receipts    map[string]string
	inputs      []domaincaseentity.CaseAcceptedDisplayBindingInputV1
}

func b1NewProcessTailFixture(t *testing.T, partial, aggregate bool) b1ProcessTailFixture {
	t.Helper()
	ctx, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-b1-tail", TurnID: "turn-b1-tail", WorkspaceRealPath: "/workspace-b1-tail",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := b1ProcessTailFixture{
		thread:   map[string]any{"id": ctx.ThreadID},
		receipts: map[string]string{"receipt-tail": domainsecurity.SHA256Hex([]byte("receipt-tail"))},
	}
	fixture.final.SecurityContext = ctx
	fixture.final.AcceptedFinal.RecordDigest = domainsecurity.SHA256Hex([]byte("final-tail"))
	fixture.final.AcceptedFinal.FinalGateVersion = domainevidence.FinalEvidenceGateVersion
	fixture.disposition.RecordDigest = domainsecurity.SHA256Hex([]byte("disposition-tail"))
	variant, support := domainevidence.EvidenceBackedAnswer, domainevidence.ClaimVerified
	missing := []string{}
	if partial {
		variant, support = domainevidence.PartialEvidenceAnswer, domainevidence.ClaimPartial
		missing = []string{"incomplete_rows"}
	}
	claims := []domainevidence.ClaimRecord{}
	count := 2
	if aggregate {
		count = 3
	}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("entity-tail-%d", i)
		if aggregate {
			key = "entity-tail-0"
		}
		reference, refErr := domaincaseentity.NewReferenceV1FromKeyedDigest(domainsecurity.SHA256Hex([]byte(key)))
		if refErr != nil {
			t.Fatal(refErr)
		}
		payload := domainevidence.NormalizedClaimPayload{
			SubjectID: string(reference), EntityID: string(reference), Count: "2", Granularity: "transaction",
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z",
		}
		kind := domainevidence.ClaimCount
		if aggregate {
			payload.Granularity = "aggregate"
			if i < 2 {
				kind, payload.Count, payload.AmountMinor, payload.Currency = domainevidence.ClaimAmount, "", "100", "CNY"
				payload.Direction = []string{"in", "out"}[i]
				payload.AccountID = string(reference)
			}
		}
		scope := domainevidence.EvidenceQueryRange{
			EntityIDs: []string{string(reference)}, AccountIDs: []string{}, Directions: []string{},
			StartAt: payload.StartAt, EndAt: payload.EndAt,
			SourceIDs: []string{"source-tail"}, FiltersHash: domainsecurity.SHA256Hex([]byte("scope-tail")),
		}
		id := fmt.Sprintf("claim-tail-%d", i)
		claim, claimErr := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
			ClaimID: id, Proposal: domainevidence.ClaimProposal{
				SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: id,
				ClaimType: kind, NormalizedPayload: payload,
			}, SupportState: support, EvidenceIDs: []string{"receipt-tail"}, SupportedScope: &scope,
			AllowedWording: []string{"exact_verified_fact"}, ProhibitedUpgrades: []string{"whole_case_conclusion"},
			VerifierReceiptID:  domainevidence.VerifierReceiptDigest(id, kind, payload, []string{"receipt-tail"}, nil, support),
			VerificationReason: "deterministic tail fixture", VerifiedAt: time.Unix(2, 0),
		})
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		claims = append(claims, claim)
	}
	fixture.final.Envelope, err = domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: variant, Context: ctx, TerminalReason: "success", Claims: claims,
		EvidenceReceiptIDs: []string{"receipt-tail"}, CheckedScope: claims[0].SupportedScope,
		MissingScope: missing, IssuedAt: time.Unix(2, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = domainevidence.RenderFinalAnswer(fixture.final.Envelope); err != nil {
		t.Fatal(err)
	}
	slots, err := domainevidence.BuildAcceptedEntitySlotBindingsV1(fixture.final.Envelope)
	if err != nil || (aggregate && len(slots) != 0) || (!aggregate && len(slots) != 2) {
		t.Fatal("fixture did not produce the expected authoritative slot set")
	}
	for _, slot := range slots {
		input := domaincaseentity.CaseAcceptedDisplayBindingInputV1{
			CaseBindingHash: ctx.CaseBindingHash, OriginalThreadID: ctx.ThreadID, OriginalTurnID: ctx.TurnID,
			AcceptedFinalDigest: fixture.final.AcceptedFinal.RecordDigest, DispositionDigest: fixture.disposition.RecordDigest,
			FinalGateVersion: fixture.final.AcceptedFinal.FinalGateVersion, ContextDigest: ctx.ContextDigest,
			DatasetSnapshotID: ctx.DatasetSnapshotID, ContextEpoch: ctx.ContextEpoch,
			EntityBindingDigest: domainsecurity.SHA256Hex([]byte("entity-binding-tail")), SlotID: slot.SlotID,
			Currentness: domaincaseentity.SnapshotCurrentV1,
		}
		if err := slot.UseReferenceV1(func(reference domaincaseentity.ReferenceV1) error {
			input.EntityReference = reference
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for _, id := range slot.ClaimIDs {
			for _, claim := range claims {
				if claim.ClaimID == id {
					input.ClaimBindings = append(input.ClaimBindings, domaincaseentity.CaseAcceptedDisplayClaimBindingV1{
						ClaimReference: id, ClaimDigest: claim.RecordDigest,
					})
				}
			}
		}
		for _, id := range slot.ReceiptIDs {
			input.EvidenceReceiptBindings = append(input.EvidenceReceiptBindings, domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1{
				EvidenceReference: id, EvidenceDigest: fixture.receipts[id],
			})
		}
		fixture.inputs = append(fixture.inputs, input)
		fixture.latest.DisplayBindings = append(fixture.latest.DisplayBindings, b1ProcessTailBinding(t, input))
	}
	continuation, err := appturn.BuildTaskContinuationSnapshotV1(fixture.thread)
	if err != nil {
		t.Fatal(err)
	}
	fixture.latest.Continuations = []domaincaseentity.CaseContinuationStateV1{{
		ContinuationDigest: continuation.StateDigest, DatasetSnapshotID: ctx.DatasetSnapshotID,
		Currentness: domaincaseentity.SnapshotCurrentV1,
	}}
	return fixture
}

func b1ProcessTailBinding(t *testing.T, input domaincaseentity.CaseAcceptedDisplayBindingInputV1) domaincaseentity.CaseAcceptedDisplayBindingV1 {
	t.Helper()
	binding, err := domaincaseentity.NewCaseAcceptedDisplayBindingV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestFundsAccountFlowB1ProcessDisplayContinuationTail(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		partial     bool
		aggregate   bool
		mutate      func(*testing.T, *b1ProcessTailFixture)
		wantReady   bool
		wantError   bool
		currentness string
	}{
		{name: "two_required_slots", wantReady: true},
		{name: "stale_final_retains_exact_slots", wantReady: true, currentness: domaincaseentity.SnapshotStaleV1, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			for i, input := range f.inputs {
				input.Currentness = domaincaseentity.SnapshotStaleV1
				f.latest.DisplayBindings[i] = b1ProcessTailBinding(t, input)
			}
		}},
		{name: "partial_with_slots", partial: true, wantReady: true},
		{name: "required_slot_missing", mutate: func(t *testing.T, f *b1ProcessTailFixture) { f.latest.DisplayBindings = f.latest.DisplayBindings[:1] }},
		{name: "all_required_slots_missing", mutate: func(t *testing.T, f *b1ProcessTailFixture) { f.latest.DisplayBindings = nil }},
		{name: "duplicate_slot", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			f.latest.DisplayBindings = append(f.latest.DisplayBindings, f.latest.DisplayBindings[0])
		}},
		{name: "unexpected_slot", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.SlotID = "account-slot-9"
			f.latest.DisplayBindings = append(f.latest.DisplayBindings, b1ProcessTailBinding(t, input))
		}},
		{name: "wrong_slot", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.SlotID = "account-slot-9"
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_entity", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.EntityReference = f.inputs[1].EntityReference
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_claim", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.ClaimBindings = f.inputs[1].ClaimBindings
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_claim_digest", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.ClaimBindings[0].ClaimDigest = domainsecurity.SHA256Hex([]byte("wrong-claim"))
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_receipt_digest", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.EvidenceReceiptBindings[0].EvidenceDigest = domainsecurity.SHA256Hex([]byte("wrong-receipt"))
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "extra_receipt", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			f.receipts["receipt-extra"] = domainsecurity.SHA256Hex([]byte("extra-receipt"))
			input.EvidenceReceiptBindings = append(input.EvidenceReceiptBindings, domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1{EvidenceReference: "receipt-extra", EvidenceDigest: f.receipts["receipt-extra"]})
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_context_epoch", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.ContextEpoch++
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_turn", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.OriginalTurnID = "turn-other"
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_disposition", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.DispositionDigest = domainsecurity.SHA256Hex([]byte("wrong-disposition"))
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "wrong_currentness", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			input := f.inputs[0]
			input.Currentness = domaincaseentity.SnapshotStaleV1
			f.latest.DisplayBindings[0] = b1ProcessTailBinding(t, input)
		}},
		{name: "corrupt_binding_digest", wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			f.latest.DisplayBindings[0].BindingDigest = domainsecurity.SHA256Hex([]byte("corrupt-binding"))
		}},
		{name: "slot_free_partial", partial: true, aggregate: true, wantReady: true},
		{name: "slot_free_partial_with_historical_slots", partial: true, aggregate: true, wantReady: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			historical := b1NewProcessTailFixture(t, false, false)
			for _, input := range historical.inputs {
				input.AcceptedFinalDigest = domainsecurity.SHA256Hex([]byte("historical-final"))
				input.OriginalTurnID = "turn-historical"
				input.Currentness = domaincaseentity.SnapshotStaleV1
				f.latest.DisplayBindings = append(f.latest.DisplayBindings, b1ProcessTailBinding(t, input))
			}
		}},
		{name: "slot_free_partial_fabricated_binding", partial: true, aggregate: true, wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			other := b1NewProcessTailFixture(t, false, false)
			f.latest.DisplayBindings = append(f.latest.DisplayBindings, other.latest.DisplayBindings[0])
		}},
		{name: "slot_free_partial_wrong_final_same_turn", partial: true, aggregate: true, wantError: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			other := b1NewProcessTailFixture(t, false, false)
			input := other.inputs[0]
			input.AcceptedFinalDigest = domainsecurity.SHA256Hex([]byte("wrong-final"))
			f.latest.DisplayBindings = append(f.latest.DisplayBindings, b1ProcessTailBinding(t, input))
		}},
		{name: "slot_free_partial_missing_continuation", partial: true, aggregate: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) { f.latest.Continuations = nil }},
		{name: "slot_free_partial_wrong_continuation", partial: true, aggregate: true, mutate: func(t *testing.T, f *b1ProcessTailFixture) {
			f.latest.Continuations[0].ContinuationDigest = domainsecurity.SHA256Hex([]byte("wrong-continuation"))
		}},
		{name: "slots_missing_continuation", mutate: func(t *testing.T, f *b1ProcessTailFixture) { f.latest.Continuations = nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := b1NewProcessTailFixture(t, scenario.partial, scenario.aggregate)
			if scenario.mutate != nil {
				scenario.mutate(t, &fixture)
			}
			bindingsBefore := append([]domaincaseentity.CaseAcceptedDisplayBindingV1(nil), fixture.latest.DisplayBindings...)
			currentness := scenario.currentness
			if currentness == "" {
				currentness = domaincaseentity.SnapshotCurrentV1
			}
			ready, err := b1ProcessDisplayContinuationTail(fixture.thread, &fixture.final, &fixture.disposition, fixture.latest, fixture.receipts, currentness)
			if ready != scenario.wantReady || (err != nil) != scenario.wantError {
				t.Fatalf("tail ready=%v error=%v; want ready=%v error=%v", ready, err != nil, scenario.wantReady, scenario.wantError)
			}
			if !reflect.DeepEqual(bindingsBefore, fixture.latest.DisplayBindings) {
				t.Fatal("tail changed retained display history")
			}
		})
	}
}
