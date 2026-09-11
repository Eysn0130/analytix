//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	authoritycompositionfixture "analytix.local/runtime-go/internal/formalauthority"
)

func TestRuntimePrivateAuthorityFrameReachesRealHandlerHTTPWithExactReadyWitness(t *testing.T) {
	fixture, err := authoritycompositionfixture.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if blocker := authoritycompositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	if !fixture.SetWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("shared witness outage setup failed")
	}
	var anchor authorityAnchorEnvelopeV1
	if err := json.Unmarshal([]byte(fixture.AnchorEnvelope), &anchor); err != nil {
		t.Fatal(err)
	}
	envelope := runtimeMainOwnedAuthorityEnvelopeV1{
		SchemaVersion: 1, Purpose: runtimeMainOwnedAuthorityPurposeV1, AuthorityAnchorV1: anchor,
		AuthorityManifestRoot: fixture.ManifestRoot, AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot,
		AuthorityCredentialBundleRoot: fixture.CredentialBundleRoot,
	}
	body, err := json.Marshal(runtimeStartupPrivateFrameV1{
		SchemaVersion: 1, Purpose: runtimeStartupPrivateFramePurposeV1, ProtectedAuthorityV1: &envelope,
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := runtimeStartupPrivateFrameBytesV1(body)
	defer clear(frame)
	defer clear(body)

	workspace := t.TempDir()
	runtimePrivateFrameWriteCaseBindingV1(t, workspace)
	const runtimeToken = "private-authority-frame-http-token"
	t.Setenv("ANALYTIX_AUTHORITY_ANCHOR_V1", `{"ambient":"must-not-be-used"}`)
	t.Setenv("ANALYTIX_AUTHORITY_MANIFEST_ROOT", "/ambient/must-not-be-used/manifest")
	t.Setenv("ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT", "/ambient/must-not-be-used/profile")
	t.Setenv("ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT", "/ambient/must-not-be-used/bundle")
	cmd := exec.Command(os.Args[0],
		"-test.run=TestRuntimeServerCLISubprocess",
		"--",
		"--host", "127.0.0.1", "--port", "0",
		"--data-dir", fixture.DataDir,
		"--runtime-durable-root", t.TempDir(),
		"--provider-id", "private-frame-http",
		"--base-url", "https://provider.invalid",
		"--model", "private-frame-http-model",
		"--endpoint-format", "chat_completions",
		"--private-startup-frame-v1",
	)
	for _, argument := range cmd.Args {
		if strings.Contains(argument, fixture.AnchorEnvelope) || strings.Contains(argument, fixture.ManifestRoot) ||
			strings.Contains(argument, fixture.CredentialProfileRoot) || strings.Contains(argument, fixture.CredentialBundleRoot) {
			t.Fatalf("private authority escaped into argv: %q", argument)
		}
	}
	cmd.Env = append(os.Environ(),
		"ANALYTIX_RUNTIME_SERVER_SUBPROCESS=1",
		"ANALYTIX_RUNTIME_TOKEN="+runtimeToken,
	)
	cmd.Stdin = bytes.NewReader(frame)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer stopRuntimeLeaseTestProcess(cmd)
	stderrDone := make(chan string, 1)
	go func() {
		scanner := newLimitedScanner(stderr)
		var tail strings.Builder
		for scanner.Scan() {
			tail.WriteString(scanner.Text())
			tail.WriteByte('\n')
		}
		stderrDone <- tail.String()
	}()
	ready := waitForRuntimeReadyFromStdout(t, stdout)
	if !ready.WitnessedAuthorityV2Configured ||
		ready.WitnessedAuthorityInstallationID != anchor.InstallationID ||
		ready.WitnessedAuthorityKeyID != anchor.AuthorityKeyID ||
		ready.WitnessedAuthorityManifestDigest != anchor.CurrentManifestDigest {
		t.Fatalf("ready payload did not witness the exact startup authority identity: %#v", ready)
	}
	if ready.DatasetSnapshotSelectionV2Configured ||
		ready.DatasetSnapshotAdmissionV2State != "absent" ||
		ready.DatasetSnapshotAdmissionV2InstallationID != "" ||
		ready.DatasetSnapshotAdmissionV2RuntimeLaunchNonce != "" ||
		ready.DatasetSnapshotAdmissionV2StagingBindingDigest != "" ||
		ready.DatasetSnapshotAdmissionV2SelectionDigest != "" ||
		ready.DatasetSnapshotAdmissionV2SnapshotID != "" ||
		ready.DatasetSnapshotAdmissionV2AuthorityRecordDigest != "" ||
		ready.DatasetSnapshotAdmissionV2ACKHMACSHA256 != "" {
		t.Fatalf("authority-only startup unexpectedly activated dataset authority: %#v", ready)
	}
	if attempts := fixture.TotalAttempts(); attempts != 0 {
		t.Fatalf("private authority startup reached a witness: attempts=%d", attempts)
	}
	for _, privateValue := range []string{
		fixture.AnchorEnvelope, anchor.AuthorityPublicKey, fixture.ManifestRoot,
		fixture.CredentialProfileRoot, fixture.CredentialBundleRoot,
	} {
		if strings.Contains(ready.RawPayload, privateValue) {
			t.Fatalf("ready payload exposed private authority input: %s", ready.RawPayload)
		}
	}

	status, health := runtimePrivateFrameHTTPJSON(t, ready.URL, runtimeToken, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("private frame runtime health: status=%d body=%#v", status, health)
	}
	status, thread := runtimePrivateFrameHTTPJSON(t, ready.URL, runtimeToken, http.MethodPost, "/v1/threads", map[string]any{
		"title": "private authority CLI public seam", "workspace": workspace,
		"providerId": "private-frame-http", "model": "private-frame-http-model",
	})
	threadID, _ := thread["id"].(string)
	if status != http.StatusCreated || threadID == "" {
		t.Fatalf("create case thread: status=%d body=%#v", status, thread)
	}
	status, turn := runtimePrivateFrameHTTPJSON(
		t, ready.URL, runtimeToken, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		map[string]any{"prompt": "核验当前案件资料", "riskIntent": "case"},
	)
	if status != http.StatusAccepted {
		t.Fatalf("case outage turn: status=%d body=%#v", status, turn)
	}
	if attempts := fixture.TotalAttempts(); attempts == 0 {
		t.Fatal("CLI private authority did not reach the shared witness on a protected case effect")
	}
	status, health = runtimePrivateFrameHTTPJSON(t, ready.URL, runtimeToken, http.MethodGet, "/health", nil)
	if status != http.StatusOK || health["status"] != "ok" {
		t.Fatalf("case witness outage disabled ordinary health: status=%d body=%#v", status, health)
	}
	select {
	case stderrTail := <-stderrDone:
		if stderrTail != "" {
			t.Fatalf("runtime subprocess exited unexpectedly: %s", stderrTail)
		}
	default:
	}
}

func runtimePrivateFrameWriteCaseBindingV1(t *testing.T, workspace string) {
	t.Helper()
	metadata := filepath.Join(workspace, ".analytix")
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"version": 1, "workspaceRoot": workspace, "caseId": "case_private_frame_http_001",
		"source": "analytix-data-analysis", "updatedAt": "2026-07-30T09:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "case-project.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runtimePrivateFrameHTTPJSON(
	t *testing.T,
	serverURL string,
	token string,
	method string,
	path string,
	body map[string]any,
) (int, map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, serverURL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoded := map[string]any{}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, decoded
}
