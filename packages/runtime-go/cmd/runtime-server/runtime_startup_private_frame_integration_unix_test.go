//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	authoritycompositionfixture "analytix.local/runtime-go/internal/formalauthority"
	"analytix.local/runtime-go/internal/testsupport/userconfigtest"
)

func TestRuntimePrivateAuthorityFrameReachesRealHandlerHTTPWithExactReadyWitness(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "cold-registry"
		if committed {
			name = "committed-registry"
		}
		t.Run(name, func(t *testing.T) { runtimePrivateAuthorityFrameHTTPV1(t, committed) })
	}
}

func runtimePrivateAuthorityFrameHTTPV1(t *testing.T, committed bool) {
	t.Helper()
	profileRoot := runtimePrivateFrameAuthorityRootV1(t)
	// Semantic startup may construct temporary durable owners. Keep those
	// owners under the child process's actual isolated TMPDIR, on the same
	// filesystem as its authority, without moving any compiler cache.
	t.Setenv("TMPDIR", profileRoot)
	t.Setenv("ANALYTIX_AUTHORITY_ANCHOR_V1", `{"ambient":"must-not-be-used"}`)
	t.Setenv("ANALYTIX_AUTHORITY_MANIFEST_ROOT", "/ambient/must-not-be-used/manifest")
	t.Setenv("ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT", "/ambient/must-not-be-used/profile")
	t.Setenv("ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT", "/ambient/must-not-be-used/bundle")
	childEnvironment, cleanup, err := userconfigtest.CreateChildEnvironment(os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	fixtureRoot := ""
	for _, entry := range childEnvironment {
		if value, found := strings.CutPrefix(entry, "TMPDIR="); found {
			fixtureRoot = value
		}
	}
	if fixtureRoot == "" {
		t.Fatal("private frame child temporary root unavailable")
	}
	fixture, err := authoritycompositionfixture.New(fixtureRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if blocker := authoritycompositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
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

	// Keep the complete small synthetic profile on the selected filesystem;
	// the host authority and its durable reconciliation then share its guarantees.
	workspace := filepath.Join(fixtureRoot, "workspace")
	durableRoot := filepath.Join(fixtureRoot, "durable")
	for _, root := range []string{workspace, durableRoot} {
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	runtimePrivateFrameWriteCaseBindingV1(t, workspace)
	if committed {
		runtimePrivateFrameSeedCommittedRegistryV1(t, fixture, durableRoot, workspace)
	}
	baselineAttempts := fixture.TotalAttempts()
	baselineShared, found := fixture.Snapshot(domainenrollment.SharedEvidenceNamespaceV1)
	if !found {
		t.Fatal("shared witness namespace is missing")
	}
	if committed && baselineAttempts == 0 {
		t.Fatal("registry provisioning omitted the real witness")
	}
	if !committed && !fixture.SetWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("shared witness outage setup failed")
	}
	const runtimeToken = "private-authority-frame-http-token"
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestRuntimePrivateAuthorityFrameCLISubprocess$",
		"--",
		"--host", "127.0.0.1", "--port", "0",
		"--data-dir", fixture.DataDir,
		"--runtime-durable-root", durableRoot,
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
	cmd.Env = append(childEnvironment,
		"ANALYTIX_RUNTIME_PRIVATE_FRAME_SUBPROCESS=1",
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
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		select {
		case diagnostic := <-stderrDone:
			for _, line := range strings.Split(diagnostic, "\n") {
				if strings.HasPrefix(line, "PRIVATE_FRAME_STARTUP_ERROR ") {
					t.Log(line)
				}
			}
		default:
		}
	})
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
	// Cold startup remains entirely local even during an outage. The
	// committed historical preparation requires a fresh read for settlement
	// reconciliation; observing that history must not create another commit.
	startupShared, found := fixture.Snapshot(domainenrollment.SharedEvidenceNamespaceV1)
	if !found || (!committed && fixture.TotalAttempts() != baselineAttempts) ||
		(committed && startupShared.ObserveCalls <= baselineShared.ObserveCalls) ||
		startupShared.AdvanceCalls != baselineShared.AdvanceCalls || startupShared.Checkpoint != baselineShared.Checkpoint {
		t.Fatal("private authority startup violated cold/read-only historical reconciliation")
	}
	if !fixture.SetWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("shared witness outage setup failed")
	}
	baselineAttempts = fixture.TotalAttempts()
	baselineShared = startupShared
	// The ready protocol intentionally pins the final-publication verification
	// key for the host. The private envelope and its filesystem locators must
	// remain absent; that public key may occur only in its declared ready field.
	if ready.FinalPublicationAuthorityKeyID != anchor.AuthorityKeyID ||
		ready.FinalPublicationAuthorityPublicKey != anchor.AuthorityPublicKey {
		t.Fatal("ready final-publication verification key did not match the startup authority")
	}
	publicKeyField, err := json.Marshal(map[string]string{
		"finalPublicationAuthorityPublicKey": anchor.AuthorityPublicKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	publicKeyEntry := strings.TrimSuffix(strings.TrimPrefix(string(publicKeyField), "{"), "}")
	if strings.Count(ready.RawPayload, publicKeyEntry) != 1 {
		t.Fatal("ready verification key is not confined to exactly one declared field")
	}
	if strings.Contains(strings.Replace(ready.RawPayload, publicKeyEntry, "", 1), anchor.AuthorityPublicKey) {
		t.Fatal("ready exposed the authority public key outside its declared verification field")
	}
	for _, privateValue := range []string{
		fixture.AnchorEnvelope, fixture.ManifestRoot,
		fixture.CredentialProfileRoot, fixture.CredentialBundleRoot,
	} {
		if strings.Contains(ready.RawPayload, privateValue) {
			t.Fatal("ready payload exposed private authority input")
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
	turnID, _ := turn["turnId"].(string)
	if status != http.StatusAccepted || turnID == "" {
		t.Fatalf("case outage turn: status=%d body=%#v", status, turn)
	}
	runtimePrivateFrameAssertUnavailableV1(t, ready.URL, runtimeToken, threadID, turnID)
	// Missing local Registry authority must fail before any shared witness
	// operation. A real committed Registry admits this same HTTP request far
	// enough to prove that the private frame connected the enrolled witness.
	attempts := fixture.TotalAttempts()
	if committed && attempts <= baselineAttempts {
		t.Fatal("CLI private authority did not reach the shared witness on a protected case effect")
	}
	if !committed && attempts != baselineAttempts {
		t.Fatal("cold local Registry admission reached the shared witness")
	}
	shared, found := fixture.Snapshot(domainenrollment.SharedEvidenceNamespaceV1)
	if !found || (committed && shared.AttemptCalls <= baselineShared.AttemptCalls) || (!committed && shared.AttemptCalls != baselineShared.AttemptCalls) {
		t.Fatal("case admission did not respect the exact shared witness namespace boundary")
	}
	if shared.AdvanceCalls != baselineShared.AdvanceCalls || shared.Checkpoint != baselineShared.Checkpoint {
		t.Fatal("unavailable case admission advanced the shared authority")
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

func runtimePrivateFrameAssertUnavailableV1(t *testing.T, url, token, threadID, turnID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, detail := runtimePrivateFrameHTTPJSON(t, url, token, http.MethodGet, "/v1/threads/"+threadID, nil)
		turns, _ := detail["turns"].([]any)
		for _, raw := range turns {
			turn, _ := raw.(map[string]any)
			if turn["id"] != turnID || turn["status"] != "completed" {
				continue
			}
			view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
			if err != nil || turn["acceptedFinal"] != nil || view.Variant != domainevidence.SourceUnavailableAnswer || view.TerminalReason != "source_unavailable" || view.CoverageStatus != domainevidence.AcceptedFinalCoverageUnavailable || view.ClaimCount != 0 || view.ReceiptMetadata.Count != 0 {
				t.Fatal("CLI case admission did not preserve the typed zero-fact source-unavailable boundary")
			}
			return
		}
		if status != http.StatusOK && status != http.StatusServiceUnavailable {
			t.Fatalf("case outage hydration status=%d", status)
		}
		if time.Now().After(deadline) {
			t.Fatal("case outage did not publish its typed source-unavailable boundary")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// This test-only subprocess runs the exact CLI parser/private-fd path and
// default production dependencies. Unlike main's public fixed error marker,
// its synthetic test failures retain a bounded, redacted startup diagnostic.
func TestRuntimePrivateAuthorityFrameCLISubprocess(t *testing.T) {
	if os.Getenv("ANALYTIX_RUNTIME_PRIVATE_FRAME_SUBPROCESS") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg != "--" {
			continue
		}
		if err := runRuntimeServer(os.Args[index+1:]); err != nil {
			message := strings.SplitN(err.Error(), "\n", 2)[0]
			message = regexp.MustCompile(`"[^"\n]*"|[A-Za-z0-9_+/=.-]{32,}|/[^ ]+`).ReplaceAllString(message, "<redacted>")
			if len(message) > 256 {
				message = message[:256]
			}
			_, _ = os.Stderr.WriteString("PRIVATE_FRAME_STARTUP_ERROR " + message + "\n")
			t.Fail()
		}
		return
	}
	t.Fatal("private frame subprocess arguments unavailable")
}
