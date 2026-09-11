//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"analytix.local/runtime-go/internal/formalauthority"
)

// Inputs are supplied by the task-owned runner, never by runtime configuration.
// This corpus is synthetic public transport evidence, not publication authority.
type runtimeOptionalPublicInputV1 struct {
	CommandID      string            `json:"commandId"`
	Head           string            `json:"head"`
	NodeVersion    string            `json:"nodeVersion"`
	GoVersion      string            `json:"goVersion"`
	ScheduleBundle string            `json:"scheduleBundle"`
	Files          map[string]string `json:"files"`
}

type runtimeOptionalPublicResponseV1 struct {
	Ordinal int    `json:"ordinal"`
	Phase   string `json:"phase"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Status  int    `json:"status"`
	Body    string `json:"body"`
	SHA256  string `json:"sha256"`
}

type runtimeOptionalPublicCollectorV1 struct {
	t         *testing.T
	fault     string
	root      string
	input     runtimeOptionalPublicInputV1
	phase     string
	requests  int
	bytesRead int64
	transport http.RoundTripper
	responses map[string]runtimeOptionalPublicResponseV1
	authority map[string]string
}

func (collector *runtimeOptionalPublicCollectorV1) bindHandler(t *testing.T, handler http.Handler) {
	if collector == nil {
		return
	}
	source, ok := handler.(FinalPublicationAuthorityIdentitySourceV1)
	if !ok {
		t.Fatal("actual handler omitted its public launcher identity")
	}
	identity, err := source.FinalPublicationAuthorityIdentityV1()
	if err != nil || ValidateFinalPublicationAuthorityIdentityV1(identity) != nil {
		t.Fatal("actual handler public identity is invalid")
	}
	publicKey := base64.RawURLEncoding.EncodeToString(identity.PublicKey)
	if collector.authority != nil && (collector.authority["keyId"] != identity.KeyID || collector.authority["publicKey"] != publicKey) {
		t.Fatal("public launcher authority changed across fixture restart")
	}
	collector.authority = map[string]string{"keyId": identity.KeyID, "publicKey": publicKey}
}

func runtimeOptionalPublicSHA256V1(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func (collector *runtimeOptionalPublicCollectorV1) nextPhase(phase string) {
	if collector != nil {
		collector.phase = phase
	}
}

func (collector *runtimeOptionalPublicCollectorV1) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := collector.transport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	closeErr := response.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if len(body) > 8<<20 {
		return nil, errors.New("shared public response exceeds evidence bound")
	}
	// Check every actual body before sampling, deduplication or serialization.
	// Do not redact a failed response into apparently passing evidence.
	for _, needle := range []string{runtimeOptionalDomainPrivateCanary, "r131-synthetic-schedule-secret", "-----BEGIN PRIVATE KEY-----", "\"apiKey\":\"test-only\""} {
		if bytes.Contains(body, []byte(needle)) {
			collector.t.Error("private fixture material reached raw public response")
			return nil, errors.New("shared corpus privacy rejection")
		}
	}
	collector.requests++
	collector.bytesRead += int64(len(body))
	if collector.bytesRead > 128<<20 {
		return nil, errors.New("shared cell exceeds observed public byte budget")
	}
	path := request.URL.RequestURI()
	// Retain the final exact observation for each phase/route/method/status.
	// All superseded polling observations were privacy-checked above.
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%d", collector.phase, request.Method, path, response.StatusCode)
	collector.responses[key] = runtimeOptionalPublicResponseV1{
		Ordinal: collector.requests, Phase: collector.phase, Method: request.Method, Path: path,
		Status: response.StatusCode, Body: string(body), SHA256: runtimeOptionalPublicSHA256V1(body),
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

func runtimeOptionalPublicCheckInputsV1(t *testing.T, input runtimeOptionalPublicInputV1) {
	t.Helper()
	if input.CommandID == "" || len(input.Head) != 40 || input.NodeVersion == "" || input.GoVersion != runtime.Version() || len(input.Files) < 4 ||
		input.ScheduleBundle != os.Getenv("ANALYTIX_TEST_SCHEDULE_MCP_ENTRYPOINT_V1") || input.Files[input.ScheduleBundle] == "" {
		t.Fatal("missing or mismatched source/toolchain/schedule input binding")
	}
	for path, expected := range input.Files {
		body, err := os.ReadFile(path)
		if err != nil || !filepath.IsAbs(path) || runtimeOptionalPublicSHA256V1(body) != expected {
			t.Fatal("source or tool input hash changed")
		}
	}
}

func (collector *runtimeOptionalPublicCollectorV1) finish(t *testing.T, guard *runtimeAsyncTurnGuardV1) {
	if t.Failed() || t.Skipped() {
		return
	}
	guard.assertDrained(t)
	runtimeOptionalPublicCheckInputsV1(t, collector.input)
	rows := make([]runtimeOptionalPublicResponseV1, 0, len(collector.responses))
	phases := map[string]bool{}
	for _, row := range collector.responses {
		rows = append(rows, row)
		phases[row.Phase] = true
	}
	for _, phase := range []string{"startup", "coding", "writing", "research", "skills", "mcp", "jobs", "protected", "post-denial", "manual-compaction", "fresh-composition", "restart", "resume"} {
		if !phases[phase] {
			t.Fatalf("missing actual public phase %s", phase)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ordinal < rows[j].Ordinal })
	guard.mu.Lock()
	operations := make([]map[string]any, 0, len(guard.records))
	for identity, record := range guard.records {
		ids := strings.Split(identity, "\x00")
		operations = append(operations, map[string]any{
			"ordinal": record.ordinal, "phase": record.phase, "threadId": ids[0], "turnId": ids[1],
			"starts": record.starts, "ends": record.ends, "completionPhase": record.result.CompletionPhase,
			"completionErrorClass": record.result.CompletionErrorClass, "failureRecordErrorClass": record.result.FailureRecordErrorClass,
			"completionDetailClass": record.result.CompletionDetailClass, "failureRecordDetailClass": record.result.FailureRecordDetailClass,
			"terminalStatus": record.result.TerminalStatus,
		})
	}
	guard.mu.Unlock()
	sort.Slice(operations, func(i, j int) bool { return operations[i]["ordinal"].(int) < operations[j]["ordinal"].(int) })
	wantOperations := 10
	if collector.fault == "missing" {
		wantOperations = 15
	}
	if len(operations) != wantOperations {
		t.Fatalf("incomplete asynchronous operation denominator: got=%d want=%d", len(operations), wantOperations)
	}
	body, err := json.MarshalIndent(map[string]any{
		"schemaVersion": 1, "fault": collector.fault, "input": collector.input, "responses": rows,
		"operations": operations, "observedResponses": collector.requests, "observedBytes": collector.bytesRead,
		"privacyCheckedBeforeSampling": true, "drained": true,
		"publicLauncherAuthority": collector.authority,
	}, "", "  ")
	if err != nil || len(body) == 0 || len(body) > 16<<20 {
		t.Fatal("invalid or oversized public corpus")
	}
	path := filepath.Join(collector.root, collector.fault+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(append(body, '\n'))
	if err := errors.Join(writeErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	t.Logf("shared corpus fault=%s responses=%d operations=%d sha256=%s", collector.fault, len(rows), len(operations), runtimeOptionalPublicSHA256V1(append(body, '\n')))
}

func TestRuntimeOptionalPluginPublicConsumerV1(t *testing.T) {
	activation := os.Getenv("ANALYTIX_TEST_SHARED_PUBLIC_CONSUMER_V1")
	if activation == "" {
		t.Skip("NOT_CONFIGURED: explicit Owner public-consumer acceptance is inactive")
	}
	if activation != "1" {
		t.Fatal("invalid explicit public-consumer acceptance activation")
	}
	root := os.Getenv("ANALYTIX_TEST_SHARED_PUBLIC_CORPUS_V1")
	profile := os.Getenv("ANALYTIX_TEST_PROFILE_ROOT")
	for _, path := range []string{root, profile} {
		info, err := os.Lstat(path)
		if err != nil || !filepath.IsAbs(path) || !info.IsDir() || info.Mode().Perm() != 0o700 || info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("shared acceptance requires isolated explicit 0700 corpus/profile roots")
		}
	}
	if blocker := formalauthority.SecureConfigurationFilesystemBlocker(profile); blocker != "" {
		t.Fatal("shared acceptance profile filesystem does not meet production secure configuration requirements")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("shared acceptance requires a fresh empty corpus directory")
	}
	inputBody, err := os.ReadFile(os.Getenv("ANALYTIX_TEST_SHARED_PUBLIC_INPUT_V1"))
	var input runtimeOptionalPublicInputV1
	if err != nil || json.Unmarshal(inputBody, &input) != nil {
		t.Fatal("shared input manifest is unavailable")
	}
	runtimeOptionalPublicCheckInputsV1(t, input)
	for _, fault := range []string{"missing", "disabled", "incompatible", "unauthorized", "domain-semantic"} {
		passed := t.Run(fault, func(t *testing.T) {
			collector := &runtimeOptionalPublicCollectorV1{t: t, fault: fault, root: root, input: input, responses: map[string]runtimeOptionalPublicResponseV1{}}
			runRuntimeOptionalPluginOrdinaryLifecycleV1(t, fault, collector)
		})
		if !passed {
			t.FailNow()
		}
		if _, err := os.Stat(filepath.Join(root, fault+".json")); err != nil {
			t.Fatal("missing fault corpus; a skipped cell is not acceptance")
		}
	}
	runtimeOptionalPublicCheckInputsV1(t, input)
}

// The diagnostic has no response collector and can never serialize a complete
// public-consumer corpus. Only this explicit selector can stop after first MCP.
type runtimeOptionalTerminalDiagnosticV1 struct {
	root  string
	input runtimeOptionalPublicInputV1
	guard *runtimeAsyncTurnGuardV1
}

func (diagnostic *runtimeOptionalTerminalDiagnosticV1) finish(t *testing.T) {
	runtimeOptionalPublicCheckInputsV1(t, diagnostic.input)
	operations := []map[string]any{}
	outcome := "UNKNOWN"
	if diagnostic.guard != nil {
		diagnostic.guard.mu.Lock()
		for _, record := range diagnostic.guard.records {
			result := record.result
			operations = append(operations, map[string]any{
				"ordinal": record.ordinal, "phase": record.phase, "starts": record.starts, "ends": record.ends,
				"completionPhase": result.CompletionPhase, "completionErrorClass": result.CompletionErrorClass,
				"failureRecordErrorClass": result.FailureRecordErrorClass, "completionDetailClass": result.CompletionDetailClass,
				"failureRecordDetailClass": result.FailureRecordDetailClass, "terminalStatus": result.TerminalStatus,
			})
			if len(diagnostic.guard.records) == 1 && record.phase == "mcp" && record.starts == 1 && record.ends == 1 {
				if result.CompletionErrorClass == "none" && result.FailureRecordErrorClass == "none" && result.TerminalStatus == "completed" && !t.Failed() {
					outcome = "NOT_REPRODUCED"
				} else if result.CompletionErrorClass != "none" || result.FailureRecordErrorClass != "none" {
					outcome = "FAILURE_OBSERVED"
				}
			}
		}
		diagnostic.guard.mu.Unlock()
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i]["ordinal"].(int) < operations[j]["ordinal"].(int) })
	body, err := json.MarshalIndent(map[string]any{
		"schemaVersion": 1, "diagnosticOnly": true, "completeCorpus": false, "acceptance": false,
		"fault": "unauthorized", "outcome": outcome, "operations": operations, "input": diagnostic.input,
	}, "", "  ")
	if err != nil || len(body) > 2<<20 {
		t.Fatal("terminal diagnostic exceeds its closed evidence bound")
	}
	file, err := os.OpenFile(filepath.Join(diagnostic.root, "terminal-diagnostic.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal("cannot create new terminal diagnostic evidence")
	}
	_, writeErr := file.Write(append(body, '\n'))
	if errors.Join(writeErr, file.Close()) != nil {
		t.Fatal("cannot persist terminal diagnostic evidence")
	}
	t.Logf("shared terminal diagnostic outcome=%s operations=%d complete_corpus=false", outcome, len(operations))
	if len(operations) != 1 || outcome == "UNKNOWN" {
		t.Error("terminal diagnostic did not observe its exact single-operation denominator")
	}
}

func TestRuntimeOptionalPluginUnauthorizedFirstMCPTerminalDiagnosticV1(t *testing.T) {
	activation := os.Getenv("ANALYTIX_TEST_SHARED_TERMINAL_DIAGNOSTIC_V1")
	if activation == "" {
		t.Skip("NOT_CONFIGURED: explicit Owner terminal diagnostic is inactive")
	}
	if activation != "1" {
		t.Fatal("invalid explicit terminal diagnostic activation")
	}
	root := os.Getenv("ANALYTIX_TEST_SHARED_DIAGNOSTIC_ROOT_V1")
	profile := os.Getenv("ANALYTIX_TEST_PROFILE_ROOT")
	for _, path := range []string{root, profile} {
		info, err := os.Lstat(path)
		if err != nil || !filepath.IsAbs(path) || !info.IsDir() || info.Mode().Perm() != 0o700 || info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("terminal diagnostic requires isolated explicit 0700 output/profile roots")
		}
	}
	if blocker := formalauthority.SecureConfigurationFilesystemBlocker(profile); blocker != "" {
		t.Fatal("terminal diagnostic profile filesystem does not meet production secure configuration requirements")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("terminal diagnostic requires a fresh empty output directory")
	}
	body, err := os.ReadFile(os.Getenv("ANALYTIX_TEST_SHARED_PUBLIC_INPUT_V1"))
	var input runtimeOptionalPublicInputV1
	if err != nil || json.Unmarshal(body, &input) != nil {
		t.Fatal("terminal diagnostic input manifest is unavailable")
	}
	runtimeOptionalPublicCheckInputsV1(t, input)
	diagnostic := &runtimeOptionalTerminalDiagnosticV1{root: root, input: input}
	defer diagnostic.finish(t)
	runRuntimeOptionalPluginOrdinaryLifecycleModeV1(t, "unauthorized", diagnostic)
}
