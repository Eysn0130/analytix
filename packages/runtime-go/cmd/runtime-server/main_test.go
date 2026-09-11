package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	electronlegacytask "analytix.local/runtime-go/internal/adapters/outbound/electronlegacytask"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/runtimeapp"
)

func TestRuntimeAuthorityConsumesCanonicalBackendGenerationAndBurnsLostMarker(t *testing.T) {
	userData := t.TempDir()
	var first bytes.Buffer
	if err := runRuntimeAuthorityCommand(
		[]string{"consume-backend-generation-v1", "--user-data-dir", userData},
		&first,
		bytes.NewReader(bytes.Repeat([]byte{0x11}, 32)),
	); err != nil {
		t.Fatal(err)
	}
	firstLine := strings.TrimSuffix(first.String(), "\n")
	if !strings.HasPrefix(firstLine, backendGenerationConsumedMarkerV1) {
		t.Fatalf("authority command emitted an invalid marker: %q", firstLine)
	}
	firstPayload := strings.TrimPrefix(firstLine, backendGenerationConsumedMarkerV1)
	var firstMarker backendGenerationConsumedMarkerPayloadV1
	if err := json.Unmarshal([]byte(firstPayload), &firstMarker); err != nil ||
		firstMarker.SchemaVersion != 1 || firstMarker.Generation != 1 ||
		len(firstMarker.AllocationRecordDigest) != 64 {
		t.Fatalf("first authority marker is invalid: %#v err=%v", firstMarker, err)
	}
	expectedFirst, _ := json.Marshal(firstMarker)
	if firstPayload != string(expectedFirst) {
		t.Fatalf("authority marker is not canonical: %q", firstPayload)
	}

	if err := runRuntimeAuthorityCommand(
		[]string{"consume-backend-generation-v1", "--user-data-dir", userData},
		failingRuntimeAuthorityWriter{},
		bytes.NewReader(bytes.Repeat([]byte{0x22}, 32)),
	); err == nil {
		t.Fatal("lost authority marker write was reported as success")
	}
	var third bytes.Buffer
	if err := runRuntimeAuthorityCommand(
		[]string{"consume-backend-generation-v1", "--user-data-dir", userData},
		&third,
		bytes.NewReader(bytes.Repeat([]byte{0x33}, 32)),
	); err != nil {
		t.Fatal(err)
	}
	var thirdMarker backendGenerationConsumedMarkerPayloadV1
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(third.String(), backendGenerationConsumedMarkerV1))), &thirdMarker); err != nil {
		t.Fatal(err)
	}
	if thirdMarker.Generation != 3 {
		t.Fatalf("lost marker reused a committed generation: %#v", thirdMarker)
	}
}

func TestRuntimeAuthorityRejectsNonCanonicalUserDataBeforeOutput(t *testing.T) {
	var output bytes.Buffer
	for _, args := range [][]string{
		nil,
		{"consume-backend-generation-v1"},
		{"consume-backend-generation-v1", "--user-data-dir", "relative"},
		{"consume-backend-generation-v1", "--user-data-dir", t.TempDir(), "extra"},
	} {
		output.Reset()
		if err := runRuntimeAuthorityCommand(args, &output, bytes.NewReader(bytes.Repeat([]byte{1}, 32))); err == nil {
			t.Fatalf("invalid authority command was accepted: %#v", args)
		}
		if output.Len() != 0 {
			t.Fatalf("invalid authority command emitted output: %q", output.String())
		}
	}
}

func TestDesktopPrivateHistoryMigrationCommandUsesFixedMarkerAndIgnoresAmbientRuntimeConfig(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "runtime-data")
	userDataDir := filepath.Join(base, "electron-user-data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "existing-runtime-state.bin"), []byte("must remain byte-stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	childRoot := filepath.Join(dataDir, "child-runs")
	if err := os.MkdirAll(childRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(childRoot, "job-1.json")
	legacyChild := []byte(`{"id":"job-1","parentGoalId":"goal","parentThreadId":"thread","kind":"background-shell","status":"completed","output":"ordinary child output<think>PRIVATE_CLI_REASONING</think> account 6222020202020202020","toolInvocations":0}`)
	if err := os.WriteFile(childPath, legacyChild, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(userDataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(userDataDir, electronlegacytask.BackgroundTaskFileV1)
	body := []byte("not-json\nprivate reasoning and raw provider output")
	if err := os.WriteFile(target, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANALYTIX_API_KEY", "must-not-be-loaded")
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "must-not-be-required")
	t.Setenv("ANALYTIX_MCP_CONFIG_PATH", filepath.Join(base, "missing-hostile-mcp.json"))
	t.Setenv("ANALYTIX_MCP_CONFIG_JSON", "{not-json")
	t.Setenv("ANALYTIX_MODEL_PROVIDERS", "{not-json")

	var output bytes.Buffer
	err := runRuntimeMigrationCommand([]string{
		"migrate-desktop-private-history-v2",
		"--data-dir", dataDir,
		"--durable-root", dataDir,
		"--user-data-dir", userDataDir,
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != desktopPrivateHistoryMigrationReadyMarkerV2+"\n" {
		t.Fatalf("migration stdout = %q", output.String())
	}
	if strings.Contains(output.String(), dataDir) || strings.Contains(output.String(), userDataDir) ||
		strings.Contains(output.String(), "reasoning") {
		t.Fatalf("migration marker disclosed private state: %q", output.String())
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy Electron task store survived migration: %v", err)
	}
	childAfter, err := os.ReadFile(childPath)
	if err != nil {
		t.Fatal(err)
	}
	lowerChild := strings.ToLower(string(childAfter))
	for _, forbidden := range []string{"private_cli_reasoning", "<think", "6222020202020202020"} {
		if strings.Contains(lowerChild, forbidden) {
			t.Fatalf("CLI semantic migration retained %q: %s", forbidden, childAfter)
		}
	}
	if !strings.Contains(string(childAfter), "ordinary child output") || !strings.Contains(string(childAfter), "[ACCOUNT]") {
		t.Fatalf("CLI semantic migration removed public diagnostics: %s", childAfter)
	}
	existingAfter, err := os.ReadFile(filepath.Join(dataDir, "existing-runtime-state.bin"))
	if err != nil || !bytes.Equal(existingAfter, []byte("must remain byte-stable")) {
		t.Fatalf("CLI semantic migration changed unrelated runtime state: body=%q err=%v", existingAfter, err)
	}
	settledDataDigest := testDirectoryTreeDigest(t, dataDir)
	output.Reset()
	if err := runRuntimeMigrationCommand([]string{
		"migrate-desktop-private-history-v2",
		"--data-dir", dataDir,
		"--durable-root", dataDir,
		"--user-data-dir", userDataDir,
	}, &output); err != nil {
		t.Fatalf("repeat CLI migration: %v", err)
	}
	if output.String() != desktopPrivateHistoryMigrationReadyMarkerV2+"\n" {
		t.Fatalf("repeat migration stdout = %q", output.String())
	}
	if repeatedDataDigest := testDirectoryTreeDigest(t, dataDir); repeatedDataDigest != settledDataDigest {
		t.Fatalf("repeat CLI migration was not byte-stable: before=%s after=%s", settledDataDigest, repeatedDataDigest)
	}
}

func testDirectoryTreeDigest(t *testing.T, root string) string {
	t.Helper()
	hasher := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hasher, "%s\x00%o\x00%d\x00", relative, uint32(info.Mode()), info.Size())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hasher.Write(body)
		}
		hasher.Write([]byte{0})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func TestDesktopPrivateHistoryMigrationCommandAbsentCreatesNoManagedRoots(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "runtime-data")
	userDataDir := filepath.Join(base, "missing-electron-user-data")
	var output bytes.Buffer
	if err := runRuntimeMigrationCommand([]string{
		"migrate-desktop-private-history-v2",
		"--data-dir", dataDir,
		"--durable-root", dataDir,
		"--user-data-dir", userDataDir,
	}, &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != desktopPrivateHistoryMigrationReadyMarkerV2+"\n" {
		t.Fatalf("absent migration stdout = %q", output.String())
	}
	for _, path := range []string{dataDir, userDataDir} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("absent migration created %s: %v", filepath.Base(path), err)
		}
	}
}

func TestDesktopPrivateHistoryMigrationCommandRejectsInvalidArgumentsWithoutOutput(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"migrate-desktop-private-history-v2"},
		{"migrate-desktop-private-history-v2", "--data-dir", "relative", "--durable-root", "/tmp/durable", "--user-data-dir", "/tmp/user"},
		{"migrate-desktop-private-history-v2", "--data-dir", "/tmp/data", "--durable-root", "/tmp/durable", "--user-data-dir", "/tmp/user", "extra"},
	} {
		var output bytes.Buffer
		if err := runRuntimeMigrationCommand(args, &output); err == nil {
			t.Fatalf("invalid migration command was accepted: %#v", args)
		}
		if output.Len() != 0 {
			t.Fatalf("invalid migration command emitted output: %q", output.String())
		}
	}
}

type failingRuntimeAuthorityWriter struct{}

func (failingRuntimeAuthorityWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated stdout loss")
}

func TestParseRuntimeServerCLIAcceptsElectronServeFlags(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "env-runtime-token")

	cli, err := parseRuntimeServerCLI([]string{
		"--host", "127.0.0.1",
		"--port", "8899",
		"--data-dir", "/tmp/analytix-runtime",
		"--base-url", "https://api.example.test",
		"--model-proxy-url", "http://127.0.0.1:12345",
		"--mcp-proxy-url", "http://127.0.0.1:12346",
		"--endpoint-format", "messages",
		"--model", "claude-test",
		"--approval-policy", "on-request",
		"--sandbox-mode", "workspace-write",
		"--token-economy-mode", "true",
		"--runtime-durable-root", "",
		"--insecure",
	})
	if err != nil {
		t.Fatalf("parseRuntimeServerCLI returned error: %v", err)
	}

	if cli.Addr != "127.0.0.1:8899" {
		t.Fatalf("addr mismatch: %q", cli.Addr)
	}
	if cli.Host != "127.0.0.1" {
		t.Fatalf("host mismatch: %q", cli.Host)
	}
	if cli.RuntimeToken != "env-runtime-token" {
		t.Fatalf("runtime token should come from ANALYTIX_RUNTIME_TOKEN, got %q", cli.RuntimeToken)
	}
	if !cli.Insecure {
		t.Fatalf("insecure flag was not parsed")
	}
	if cli.DataDir != "/tmp/analytix-runtime" {
		t.Fatalf("data dir mismatch: %q", cli.DataDir)
	}
	if cli.ProductionDurableRoot != filepath.Clean("/tmp/analytix-runtime") {
		t.Fatalf("production durable root mismatch: %q", cli.ProductionDurableRoot)
	}
	if cli.BaseURL != "https://api.example.test" {
		t.Fatalf("base URL mismatch: %q", cli.BaseURL)
	}
	if cli.EndpointFormat != "messages" {
		t.Fatalf("endpoint format mismatch: %q", cli.EndpointFormat)
	}
	if cli.Model != "claude-test" {
		t.Fatalf("model mismatch: %q", cli.Model)
	}
	if cli.ModelProxyURL != "http://127.0.0.1:12345" {
		t.Fatalf("model proxy URL mismatch: %q", cli.ModelProxyURL)
	}
	if cli.MCPProxyURL != "http://127.0.0.1:12346" {
		t.Fatalf("MCP proxy URL mismatch: %q", cli.MCPProxyURL)
	}
	if cli.ApprovalPolicy != "on-request" {
		t.Fatalf("approval policy mismatch: %q", cli.ApprovalPolicy)
	}
	if cli.SandboxMode != "workspace-write" {
		t.Fatalf("sandbox mode mismatch: %q", cli.SandboxMode)
	}
	if cli.TokenEconomyMode != "true" {
		t.Fatalf("token economy mode mismatch: %q", cli.TokenEconomyMode)
	}
}

func TestParseRuntimeServerCLIAcceptsRuntimeDurableRootWithoutCandidateAlias(t *testing.T) {
	cli, err := parseRuntimeServerCLI([]string{
		"--runtime-durable-root=/tmp/runtime-root",
	})
	if err != nil {
		t.Fatalf("parseRuntimeServerCLI returned error: %v", err)
	}
	if cli.RuntimeDurableRoot != "/tmp/runtime-root" {
		t.Fatalf("runtime durable root mismatch: %q", cli.RuntimeDurableRoot)
	}
	if cli.ProductionDurableRoot != "" {
		t.Fatalf("runtime durable root should not synthesize a production durable root, got %q", cli.ProductionDurableRoot)
	}
}

func TestRuntimeConfigRejectsAmbientProviderCredentialAuthority(t *testing.T) {
	t.Setenv("ANALYTIX_API_KEY", "synthetic-ambient-provider-secret")
	t.Setenv("ANALYTIX_MODEL_PROVIDERS", `{"defaultProviderId":"ambient","providers":[{"id":"ambient","apiKey":"synthetic-ambient-provider-secret","baseUrl":"https://provider.invalid/v1"}]}`)
	cli, err := parseRuntimeServerCLI([]string{
		"--model-providers-json", `{"defaultProviderId":"explicit","providers":[{"id":"explicit","baseUrl":"https://provider.invalid/v1"}]}`,
	})
	if err != nil {
		t.Fatalf("parseRuntimeServerCLI() error = %v", err)
	}
	config := runtimeConfigFromCLI(cli, "", 0)
	if config.APIKey != "" || config.ProviderID != "" || config.BaseURL != "" || config.Model != "" ||
		config.EndpointFormat != "" || config.ModelProvidersJSON != "" || config.ModelProxyURL != "" {
		t.Fatalf("legacy CLI Provider authority reached runtime configuration: %#v", config)
	}
	if _, err := parseRuntimeServerCLI([]string{
		"--model-providers-json", `{"providers":[{"id":"explicit","apiKey":"synthetic-cli-provider-secret","baseUrl":"https://provider.invalid/v1"}]}`,
	}); err == nil {
		t.Fatal("runtime accepted credential-bearing Provider metadata in argv")
	}
}

func TestRuntimeCLISelectsOnlyTheExactDataDirectoryForProviderAuthority(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "profile-a")
	second := filepath.Join(root, "profile-b")
	for _, dataDir := range []string{first, second} {
		cli, err := parseRuntimeServerCLI([]string{
			"--data-dir", dataDir,
			"--base-url", "https://copied.invalid/v1",
			"--model-proxy-url", "https://copied-proxy.invalid",
			"--endpoint-format", "messages",
			"--model", "copied-model",
		})
		if err != nil {
			t.Fatal(err)
		}
		config := runtimeConfigFromCLI(cli, "", 0)
		if config.DataDir != dataDir || config.ProviderID != "" || config.BaseURL != "" || config.Model != "" ||
			config.EndpointFormat != "" || config.ModelProvidersJSON != "" || config.ModelProxyURL != "" {
			t.Fatalf("CLI authority projection for %q = %#v", filepath.Base(dataDir), config)
		}
	}
	if _, err := parseRuntimeServerCLI([]string{"--data-dir", "relative/profile"}); err == nil {
		t.Fatal("runtime accepted a non-exact data directory authority")
	}
}

func TestRuntimeConfigReadsControlledArtifactHostOnlyFromReservedEnvironment(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x54}, 32))
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL", "http://127.0.0.1:45677")
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN", "retired-v1-token")
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL", "https://127.0.0.1:45678")
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN", secret)
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION", "9")
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST", strings.Repeat("c", 64))
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER", "cm9vdC1kZXI")
	t.Setenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256", strings.Repeat("a", 64))
	cli, err := parseRuntimeServerCLI([]string{"--insecure"})
	if err != nil {
		t.Fatal(err)
	}
	config := runtimeConfigFromCLI(cli, "", 0)
	if config.ControlledArtifactHostV2URL != "https://127.0.0.1:45678" ||
		config.ControlledArtifactHostV2Token != secret || config.ControlledArtifactHostV2BackendGeneration != "9" ||
		config.ControlledArtifactHostV2AllocationDigest != strings.Repeat("c", 64) ||
		config.ControlledArtifactHostV2TLSRootCertDER != "cm9vdC1kZXI" ||
		config.ControlledArtifactHostV2TLSLeafSPKISHA256 != strings.Repeat("a", 64) {
		t.Fatal("reserved controlled artifact host environment did not reach the private runtime config")
	}
}

func TestRuntimeConfigIgnoresAmbientAuthorityAndUsesOnlyPrivateStartupFrameProjection(t *testing.T) {
	t.Setenv("ANALYTIX_AUTHORITY_ANCHOR_V1", `{"schemaVersion":1}`)
	t.Setenv("ANALYTIX_AUTHORITY_MANIFEST_ROOT", "/ambient/authority/manifest")
	t.Setenv("ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT", "/ambient/authority/profile")
	t.Setenv("ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT", "/ambient/authority/bundle")
	cli, err := parseRuntimeServerCLI([]string{"--insecure"})
	if err != nil {
		t.Fatal(err)
	}
	config := runtimeConfigFromCLI(cli, "", 0)
	if config.AuthorityAnchorV1 != "" || config.AuthorityManifestRoot != "" ||
		config.AuthorityCredentialProfileRoot != "" || config.AuthorityCredentialBundleRoot != "" {
		t.Fatalf("ambient authority reached runtime config: %#v", config)
	}
	cli.MainOwnedAuthorityV1 = &runtimeMainOwnedAuthorityConfigV1{
		AnchorJSON: `{"schemaVersion":1}`, ManifestRoot: "/private/authority/manifest",
		CredentialProfileRoot: "/private/authority/profile", CredentialBundleRoot: "/private/authority/bundle",
	}
	config = runtimeConfigFromCLI(cli, "", 0)
	if config.AuthorityAnchorV1 != `{"schemaVersion":1}` ||
		config.AuthorityManifestRoot != "/private/authority/manifest" ||
		config.AuthorityCredentialProfileRoot != "/private/authority/profile" ||
		config.AuthorityCredentialBundleRoot != "/private/authority/bundle" {
		t.Fatalf("private startup authority projection was not captured exactly: %#v", config)
	}
}

func TestRuntimeConfigCarriesHostScheduleBindingImmutablyFromPrivateStartupFrame(t *testing.T) {
	cli, err := parseRuntimeServerCLI([]string{"--insecure"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := runtimeHostScheduleMCPBindingFixtureV1().spec()
	if err != nil {
		t.Fatal(err)
	}
	cli.HostScheduleMCPServer = &spec
	config := runtimeConfigFromCLI(cli, "", 0)
	spec.Args[3] = "http://127.0.0.1:9788"
	spec.Env["ELECTRON_RUN_AS_NODE"] = "0"
	if config.HostScheduleMCPServer == nil ||
		config.HostScheduleMCPServer.Args[3] != "http://127.0.0.1:9787" ||
		config.HostScheduleMCPServer.Env["ELECTRON_RUN_AS_NODE"] != "1" {
		t.Fatal("private host schedule binding did not reach runtime composition immutably")
	}
	withoutBinding := runtimeConfigFromCLI(runtimeServerCLIConfig{}, "", 0)
	if withoutBinding.HostScheduleMCPServer != nil {
		t.Fatal("ordinary runtime config synthesized a host schedule binding")
	}
}

func TestRuntimeConfigAdmitsProviderAuditSocketOnlyForPackagedMilestoneB(t *testing.T) {
	t.Setenv("ANALYTIX_PROVIDER_AUDIT_SOCKET_PATH", "/private/formal/provider-audit.sock")
	t.Setenv("ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_B", "")
	config := runtimeConfigFromCLI(runtimeServerCLIConfig{}, "", 0)
	if config.ProviderAuditSocketPath != "" {
		t.Fatal("ambient provider audit socket reached an ordinary runtime")
	}
	t.Setenv("ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_B", "1")
	config = runtimeConfigFromCLI(runtimeServerCLIConfig{}, "", 0)
	if config.ProviderAuditSocketPath != "/private/formal/provider-audit.sock" {
		t.Fatalf("formal provider audit socket was not captured exactly: %#v", config)
	}
}

func TestRuntimeServerClearsEveryLaunchSecretBeforeListenerActivation(t *testing.T) {
	var cleared []string
	err := clearRuntimeServerSecretEnvironment(func(name string) error {
		cleared = append(cleared, name)
		if name == "ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL" {
			return errors.New("injected unset failure")
		}
		return nil
	})
	expected := []string{
		"ANALYTIX_API_KEY",
		"ANALYTIX_MODEL_PROVIDERS",
		"ANALYTIX_HUB_TEST_GATEWAY_TOKEN",
		"ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN",
		"ANALYTIX_RUNTIME_TOKEN",
		"ANALYTIX_PROVIDER_AUDIT_SOCKET_PATH",
		"ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_B",
		"ANALYTIX_AUTHORITY_ANCHOR_V1",
		"ANALYTIX_AUTHORITY_MANIFEST_ROOT",
		"ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT",
		"ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256",
	}
	if err == nil || len(cleared) != len(expected) {
		t.Fatalf("runtime secret clear did not attempt every reserved value: cleared=%#v err=%v", cleared, err)
	}
	for index := range expected {
		if cleared[index] != expected[index] {
			t.Fatalf("runtime secret clear order changed: cleared=%#v", cleared)
		}
	}
}

func TestParseRuntimeServerCLIRejectsCandidateDurableRootAlias(t *testing.T) {
	if _, err := parseRuntimeServerCLI([]string{
		"--candidate-durable-root=/tmp/compat-root",
	}); err == nil {
		t.Fatalf("candidate durable root alias should be rejected")
	}
}

func TestParseRuntimeServerCLIRejectsRuntimeTokenInArgv(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "env-runtime-token")
	if _, err := parseRuntimeServerCLI([]string{
		"--addr", "127.0.0.1:7777", "--runtime-token", "flag-runtime-token",
	}); err == nil {
		t.Fatal("production runtime accepted a bearer token in argv")
	}
}

func TestRuntimeServerRequiresTokenUnlessInsecureIsExplicit(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "")
	if err := runRuntimeServer([]string{
		"--host", "127.0.0.1", "--port", "0", "--data-dir", t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "runtime token is required") {
		t.Fatalf("blank authenticated token should fail closed: %v", err)
	}
}

func TestInsecureRuntimeRejectsNonLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", "[::]:0", "192.0.2.1:0"} {
		cli, err := parseRuntimeServerCLI([]string{"--addr", addr, "--insecure"})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateRuntimeListenSecurity(cli); err == nil {
			t.Fatalf("insecure runtime accepted non-loopback address %q", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0", "localhost:0"} {
		cli, err := parseRuntimeServerCLI([]string{"--addr", addr, "--insecure"})
		if err != nil || validateRuntimeListenSecurity(cli) != nil {
			t.Fatalf("insecure runtime rejected loopback address %q: parse=%v validate=%v", addr, err, validateRuntimeListenSecurity(cli))
		}
	}
}

func TestRuntimeServerCLIStartsWithElectronServeFlagsAndEnvToken(t *testing.T) {
	const runtimeToken = "env-runtime-token"
	dataDir := t.TempDir()
	cmd := exec.Command(os.Args[0],
		"-test.run=TestRuntimeServerCLISubprocess",
		"--",
		"--host", "127.0.0.1",
		"--port", "0",
		"--data-dir", dataDir,
		"--model-proxy-url", "http://127.0.0.1:12345",
		"--mcp-proxy-url", "http://127.0.0.1:12345",
		"--approval-policy", "auto",
		"--sandbox-mode", "read-only",
		"--token-economy-mode", "true",
	)
	cmd.Env = append(os.Environ(),
		"ANALYTIX_RUNTIME_SERVER_SUBPROCESS=1",
		"ANALYTIX_RUNTIME_TOKEN="+runtimeToken,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start runtime subprocess: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

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
	if cmd.Process == nil || ready.RuntimePID != cmd.Process.Pid {
		t.Fatalf("ready payload PID mismatch: ready=%d child=%v", ready.RuntimePID, cmd.Process)
	}
	if !ready.RuntimeTokenConfigured {
		t.Fatal("ready payload did not confirm token configuration")
	}
	if !ready.PersistenceRootsConfigured {
		t.Fatal("ready payload did not confirm persistence root configuration")
	}
	publicKey, err := base64.RawURLEncoding.Strict().DecodeString(ready.FinalPublicationAuthorityPublicKey)
	if err != nil || runtimeapp.ValidateFinalPublicationAuthorityIdentityV1(runtimeapp.FinalPublicationAuthorityIdentityV1{
		KeyID: ready.FinalPublicationAuthorityKeyID, PublicKey: publicKey,
	}) != nil {
		t.Fatal("ready payload did not expose a valid final publication authority identity")
	}
	if ready.ControlledArtifactHostV2Configured || ready.ControlledArtifactHostV2Ready {
		t.Fatal("absent controlled artifact V2 authority was reported as ready")
	}
	if ready.WitnessedAuthorityV2Configured ||
		ready.WitnessedAuthorityInstallationID != "" ||
		ready.WitnessedAuthorityKeyID != "" ||
		ready.WitnessedAuthorityManifestDigest != "" {
		t.Fatalf("absent main-owned authority carried witnessed identity: %#v", ready)
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
		t.Fatalf("absent dataset snapshot admission carried details: %#v", ready)
	}
	if strings.Contains(ready.RawPayload, runtimeToken) || strings.Contains(ready.RawPayload, dataDir) {
		t.Fatalf("ready payload leaked a credential or persistence path: %s", ready.RawPayload)
	}
	req, err := http.NewRequest(http.MethodGet, ready.URL+"/v1/runtime/info", nil)
	if err != nil {
		t.Fatalf("build runtime info request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+runtimeToken)
	res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		select {
		case stderrTail := <-stderrDone:
			t.Fatalf("runtime info request failed: %v\nstderr: %s", err, stderrTail)
		default:
			t.Fatalf("runtime info request failed: %v", err)
		}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("runtime info status = %d", res.StatusCode)
	}
	var info map[string]any
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatalf("decode runtime info: %v", err)
	}
	executionPolicy, _ := info["executionPolicy"].(map[string]any)
	if executionPolicy["approvalPolicy"] != "auto" || executionPolicy["sandboxMode"] != "read-only" {
		t.Fatalf("runtime info must reflect Electron serve policy flags, got %#v", info)
	}
	networkProxy, _ := info["networkProxy"].(map[string]any)
	if networkProxy["configured"] != false ||
		networkProxy["valid"] != true ||
		networkProxy["mode"] != "auto" ||
		networkProxy["source"] != "environment" ||
		networkProxy["credentialsMasked"] != true {
		t.Fatalf("runtime info must not treat the Electron serve model proxy flag as Provider authority, got %#v", info)
	}
	if _, exists := networkProxy["summary"]; exists {
		t.Fatalf("runtime info must not expose the configured proxy endpoint summary: %#v", networkProxy)
	}
}

func TestRuntimeInfoHostMatchesActualListener(t *testing.T) {
	process := runtimeLeaseTestCommand([]string{
		"--addr", "127.0.0.1:0", "--host", "0.0.0.0", "--data-dir", t.TempDir(),
	})
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer stopRuntimeLeaseTestProcess(process)
	ready := waitForRuntimeReadyFromStdout(t, stdout)
	req, err := http.NewRequest(http.MethodGet, ready.URL+"/v1/runtime/info", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer lease-test-token")
	res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var info map[string]any
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info["listenerScope"] != "loopback" {
		t.Fatalf("runtime info listener scope does not match actual listener: %#v", info)
	}
	if _, exists := info["host"]; exists {
		t.Fatalf("runtime info must not expose the listener host: %#v", info)
	}
}

func TestCompositeLeaseBlocksSecondProcessSharingEitherRoot(t *testing.T) {
	tests := []struct {
		name       string
		firstArgs  []string
		secondArgs []string
	}{
		{
			name: "data root",
			firstArgs: []string{
				"--data-dir", t.TempDir(),
				"--runtime-durable-root", t.TempDir(),
			},
			secondArgs: []string{
				"--data-dir", "",
				"--runtime-durable-root", t.TempDir(),
			},
		},
		{
			name: "durable root",
			firstArgs: []string{
				"--data-dir", t.TempDir(),
				"--runtime-durable-root", t.TempDir(),
			},
			secondArgs: []string{
				"--data-dir", t.TempDir(),
				"--runtime-durable-root", "",
			},
		},
	}
	tests[0].secondArgs[1] = tests[0].firstArgs[1]
	tests[1].secondArgs[3] = tests[1].firstArgs[3]
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := startRuntimeLeaseTestProcess(t, test.firstArgs)
			t.Cleanup(func() { stopRuntimeLeaseTestProcess(first) })
			second := runtimeLeaseTestCommand(test.secondArgs)
			output, err := second.CombinedOutput()
			assertRuntimeServerFatalProjection(t, output, err)
			stopRuntimeLeaseTestProcess(first)

			restarted := startRuntimeLeaseTestProcess(t, test.secondArgs)
			stopRuntimeLeaseTestProcess(restarted)
		})
	}
}

func TestCompositeLeaseBlocksSecondProcessWithNestedRoot(t *testing.T) {
	base := t.TempDir()
	first := startRuntimeLeaseTestProcess(t, []string{
		"--data-dir", filepath.Join(base, "data"),
		"--runtime-durable-root", filepath.Join(base, "durable-a"),
	})
	t.Cleanup(func() { stopRuntimeLeaseTestProcess(first) })
	secondArgs := []string{
		"--data-dir", filepath.Join(base, "data", "private"),
		"--runtime-durable-root", filepath.Join(base, "durable-b"),
	}
	second := runtimeLeaseTestCommand(secondArgs)
	output, err := second.CombinedOutput()
	assertRuntimeServerFatalProjection(t, output, err)
	stopRuntimeLeaseTestProcess(first)

	restarted := startRuntimeLeaseTestProcess(t, secondArgs)
	stopRuntimeLeaseTestProcess(restarted)
}

func TestFailedConstructorReleasesPersistenceLease(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "lease-test-token")
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	threadDir := filepath.Join(durableRoot, "threads", "thr_corrupt")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	threadPath := filepath.Join(threadDir, "thread.json")
	if err := os.WriteFile(threadPath, []byte(`{"id":"thr_corrupt","id":"forged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--host", "127.0.0.1",
		"--port", "0",
		"--data-dir", dataDir,
		"--runtime-durable-root", durableRoot,
	}
	if err := runRuntimeServer(args); err == nil || !strings.Contains(err.Error(), "invalid_json") {
		t.Fatalf("corrupt startup should fail before serving: %v", err)
	}
	cli, err := parseRuntimeServerCLI(args)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := runtimeapp.AcquireRuntimePersistenceLease(runtimeConfigFromCLI(cli, "lease-test-token", 0))
	if err != nil {
		t.Fatalf("failed constructor retained the persistence lease: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close reacquired persistence lease: %v", err)
	}
	if err := os.RemoveAll(threadDir); err != nil {
		t.Fatalf("remove deliberately corrupt test fixture: %v", err)
	}
	restarted := startRuntimeLeaseTestProcess(t, []string{
		"--data-dir", dataDir,
		"--runtime-durable-root", durableRoot,
	})
	stopRuntimeLeaseTestProcess(restarted)
}

func TestSemanticStartupFailurePrecedesEnvironmentAndListenerActivation(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "pre-activation-token")
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	threadDir := filepath.Join(durableRoot, "threads", "thr_pre_activation_corrupt")
	if err := os.MkdirAll(threadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threadDir, "thread.json"), []byte(`{"id":"one","id":"two"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dependencies := defaultRuntimeServerDependencies()
	listenCalls, unsetCalls := 0, 0
	dependencies.listen = func(string, string) (net.Listener, error) {
		listenCalls++
		return nil, errors.New("listener should not be reached")
	}
	dependencies.unsetenv = func(string) error {
		unsetCalls++
		return nil
	}
	err := runRuntimeServerWithDependencies([]string{
		"--host", "127.0.0.1", "--port", "0", "--data-dir", dataDir, "--runtime-durable-root", durableRoot,
	}, dependencies)
	if err == nil || !strings.Contains(err.Error(), "invalid_json") {
		t.Fatalf("corrupt semantic startup returned the wrong failure: %v", err)
	}
	if listenCalls != 0 || unsetCalls != 0 {
		t.Fatalf("semantic failure activated external state: listenCalls=%d unsetCalls=%d", listenCalls, unsetCalls)
	}
	if os.Getenv("ANALYTIX_RUNTIME_TOKEN") != "pre-activation-token" {
		t.Fatal("semantic failure changed the process environment before activation")
	}
}

func TestListenFailureReleasesPersistenceLease(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "lease-test-token")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	if err := runRuntimeServer([]string{
		"--addr", listener.Addr().String(),
		"--data-dir", dataDir, "--runtime-durable-root", durableRoot,
	}); err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("occupied listener should fail startup: %v", err)
	}
	_ = listener.Close()
	restarted := startRuntimeLeaseTestProcess(t, []string{
		"--data-dir", dataDir, "--runtime-durable-root", durableRoot,
	})
	stopRuntimeLeaseTestProcess(restarted)
}

func TestGracefulShutdownReleasesPersistenceLease(t *testing.T) {
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	args := []string{"--data-dir", dataDir, "--runtime-durable-root", durableRoot}
	first := startRuntimeLeaseTestProcess(t, args)
	if err := first.Process.Signal(syscall.SIGTERM); err != nil {
		stopRuntimeLeaseTestProcess(first)
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- first.Wait() }()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("graceful runtime exit failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		stopRuntimeLeaseTestProcess(first)
		t.Fatal("graceful runtime shutdown timed out")
	}
	restarted := startRuntimeLeaseTestProcess(t, args)
	stopRuntimeLeaseTestProcess(restarted)
}

func TestShutdownRuntimeServerNotifiesLifecycleHandlerWithDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	handler := &shutdownLifecycleHandler{}
	if err := shutdownRuntimeServer(ctx, &http.Server{}, handler); err != nil {
		t.Fatalf("shutdownRuntimeServer returned error: %v", err)
	}
	if !handler.called {
		t.Fatalf("shutdownRuntimeServer did not call handler lifecycle shutdown")
	}
	if !handler.hadDeadline {
		t.Fatalf("handler shutdown should receive a bounded context")
	}
}

func TestShutdownRuntimeServerStopsAdmissionsBeforeLifecycleDrain(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handler := &blockingShutdownLifecycleHandler{entered: make(chan struct{}), release: make(chan struct{})}
	server := &http.Server{Handler: handler}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	shutdownDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { shutdownDone <- shutdownRuntimeServer(ctx, server, handler) }()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("lifecycle drain did not start")
	}
	client := &http.Client{Timeout: 100 * time.Millisecond}
	if response, err := client.Get("http://" + listener.Addr().String()); err == nil {
		_ = response.Body.Close()
		t.Fatal("runtime accepted a new request after shutdown admission stop")
	}
	close(handler.release)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if err := <-serveDone; err != nil && !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("serve returned unexpected error: %v", err)
	}
}

func TestRuntimeServerShutdownRetryRetainsOwnershipUntilDrainSucceeds(t *testing.T) {
	attempts := 0
	waits := 0
	first := errors.New("background job did not stop")
	err := retryRuntimeServerShutdown(func() error {
		attempts++
		if attempts == 1 {
			return first
		}
		return nil
	}, func(err error) {
		waits++
		if !errors.Is(err, first) {
			t.Fatalf("shutdown retry lost the drain blocker: %v", err)
		}
	})
	if err != nil || attempts != 2 || waits != 1 {
		t.Fatalf("shutdown retry released ownership early: attempts=%d waits=%d err=%v", attempts, waits, err)
	}
}

func TestSignalDuringStartupDoesNotActivateListener(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "startup-cancel-token")
	args := []string{
		"--host", "127.0.0.1", "--port", "0", "--data-dir", t.TempDir(), "--runtime-durable-root", t.TempDir(),
	}
	runtimeCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	dependencies := defaultRuntimeServerDependencies()
	dependencies.notifyContext = func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return runtimeCtx, func() {}
	}
	dependencies.prepare = func(ctx context.Context, _ runtimeapp.Config, _ *runtimeapp.PersistenceLease) (runtimeServerStartup, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	listenCalled := false
	dependencies.listen = func(string, string) (net.Listener, error) {
		listenCalled = true
		return nil, errors.New("listener must remain inactive")
	}
	done := make(chan error, 1)
	go func() { done <- runRuntimeServerWithDependencies(args, dependencies) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("startup preparation did not receive the process context")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("startup signal was not handled as graceful cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup cancellation did not return")
	}
	if listenCalled {
		t.Fatal("startup cancellation activated a listener")
	}
}

func TestCLIRetainsPersistenceLeaseAcrossFailedLifecycleDrain(t *testing.T) {
	const shutdownErrorSentinel = "background job did not stop: /private/shutdown-pii-13900000004"
	t.Setenv("ANALYTIX_RUNTIME_TOKEN", "lease-retention-token")
	dataDir := t.TempDir()
	durableRoot := t.TempDir()
	args := []string{
		"--host", "127.0.0.1", "--port", "0", "--data-dir", dataDir, "--runtime-durable-root", durableRoot,
	}
	lifecycle := &retryLeaseLifecycleHandler{
		firstError:    errors.New(shutdownErrorSentinel),
		secondEntered: make(chan struct{}),
		releaseSecond: make(chan struct{}),
	}
	dependencies := defaultRuntimeServerDependencies()
	dependencies.prepare = func(context.Context, runtimeapp.Config, *runtimeapp.PersistenceLease) (runtimeServerStartup, error) {
		return staticRuntimeServerStartup{handler: lifecycle}, nil
	}
	dependencies.listen = func(string, string) (net.Listener, error) {
		return &immediateFailureListener{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}}, nil
	}
	dependencies.unsetenv = func(string) error { return nil }
	finishStderrCapture := beginRuntimeServerStderrCapture(t)
	runDone := make(chan error, 1)
	go func() { runDone <- runRuntimeServerWithDependencies(args, dependencies) }()
	select {
	case <-lifecycle.secondEntered:
	case <-time.After(3 * time.Second):
		close(lifecycle.releaseSecond)
		t.Fatal("CLI did not retry the failed lifecycle drain")
	}
	stderr := finishStderrCapture()
	const expectedStderr = "[analytix] event=ANALYTIX_RUNTIME_SHUTDOWN_DRAIN_RETRY\n"
	if stderr != expectedStderr {
		close(lifecycle.releaseSecond)
		t.Fatalf("shutdown drain retry stderr projection mismatch: got=%q want=%q", stderr, expectedStderr)
	}
	if strings.Contains(stderr, shutdownErrorSentinel) {
		close(lifecycle.releaseSecond)
		t.Fatalf("shutdown drain retry stderr leaked hostile error sentinel: %q", stderr)
	}
	cli, err := parseRuntimeServerCLI(args)
	if err != nil {
		close(lifecycle.releaseSecond)
		t.Fatal(err)
	}
	config := runtimeConfigFromCLI(cli, "lease-retention-token", 0)
	contender, acquireErr := runtimeapp.AcquireRuntimePersistenceLease(config)
	if contender != nil {
		_ = contender.Close()
	}
	if !errors.Is(acquireErr, persistencefs.ErrPersistenceInUse) {
		close(lifecycle.releaseSecond)
		t.Fatalf("failed lifecycle drain released persistence lease: %v", acquireErr)
	}
	close(lifecycle.releaseSecond)
	select {
	case runErr := <-runDone:
		if runErr == nil || !strings.Contains(runErr.Error(), "serve") {
			t.Fatalf("immediate serve failure was not reported after safe drain: %v", runErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("CLI did not return after lifecycle drain succeeded")
	}
	restarted, err := runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatalf("successful lifecycle drain did not release persistence lease: %v", err)
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeServerShutdownTimeoutUsesEnvOverride(t *testing.T) {
	t.Setenv("ANALYTIX_RUNTIME_SERVER_SHUTDOWN_TIMEOUT_MS", "25")
	if got := runtimeServerShutdownTimeout(); got != 25*time.Millisecond {
		t.Fatalf("shutdown timeout env override mismatch: %v", got)
	}
	t.Setenv("ANALYTIX_RUNTIME_SERVER_SHUTDOWN_TIMEOUT_MS", "0")
	if got := runtimeServerShutdownTimeout(); got != defaultRuntimeServerShutdownTimeout {
		t.Fatalf("invalid shutdown timeout should fall back to default, got %v", got)
	}
}

func TestRuntimeServerMainProjectsFatalErrorsAsFixedEvent(t *testing.T) {
	const errorSentinel = "/private/runtime-error-pii-13900000003"
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestRuntimeServerCLISubprocess$",
		"--",
		"--port", errorSentinel,
	)
	cmd.Env = append(os.Environ(), "ANALYTIX_RUNTIME_SERVER_SUBPROCESS=1")
	output, err := cmd.CombinedOutput()
	assertRuntimeServerFatalProjection(t, output, err)
	if strings.Contains(string(output), errorSentinel) {
		t.Fatalf("runtime-server fatal stderr leaked hostile error sentinel: %q", output)
	}
}

func assertRuntimeServerFatalProjection(t *testing.T, output []byte, err error) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("runtime-server fatal subprocess exit mismatch: err=%v output=%q", err, output)
	}
	const expected = "[analytix] event=ANALYTIX_RUNTIME_SERVER_FAILED\n"
	if string(output) != expected {
		t.Fatalf("runtime-server fatal stderr projection mismatch: got=%q want=%q", output, expected)
	}
}

func beginRuntimeServerStderrCapture(t *testing.T) func() string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = writer
	finished := false
	t.Cleanup(func() {
		if !finished {
			os.Stderr = previous
			_ = writer.Close()
			_ = reader.Close()
		}
	})
	return func() string {
		t.Helper()
		if finished {
			t.Fatal("runtime-server stderr capture already finished")
		}
		finished = true
		os.Stderr = previous
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
}

func TestRuntimeServerCLISubprocess(t *testing.T) {
	if os.Getenv("ANALYTIX_RUNTIME_SERVER_SUBPROCESS") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[index+1:]...)
			break
		}
	}
	main()
}

func runtimeLeaseTestCommand(extraArgs []string) *exec.Cmd {
	args := []string{
		"-test.run=TestRuntimeServerCLISubprocess",
		"--",
		"--host", "127.0.0.1",
		"--port", "0",
	}
	args = append(args, extraArgs...)
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "ANALYTIX_RUNTIME_SERVER_SUBPROCESS=1", "ANALYTIX_RUNTIME_TOKEN=lease-test-token")
	return cmd
}

func startRuntimeLeaseTestProcess(t *testing.T, extraArgs []string) *exec.Cmd {
	t.Helper()
	cmd := runtimeLeaseTestCommand(extraArgs)
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
	if _, err := readRuntimeReadyFromStdout(t, stdout); err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		t.Fatalf("runtime subprocess ended before ready: %v\nstderr: %s", err, <-stderrDone)
	}
	return cmd
}

func stopRuntimeLeaseTestProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

type shutdownLifecycleHandler struct {
	called      bool
	hadDeadline bool
}

type blockingShutdownLifecycleHandler struct {
	entered chan struct{}
	release chan struct{}
}

type staticRuntimeServerStartup struct {
	handler http.Handler
}

func (startup staticRuntimeServerStartup) ActivateContext(context.Context, runtimeapp.Config) (http.Handler, error) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	return staticRuntimeIdentityHandler{Handler: startup.handler, identity: runtimeapp.FinalPublicationAuthorityIdentityV1{
		KeyID: domainsecurity.SHA256Hex(publicKey), PublicKey: publicKey,
	}}, nil
}

type staticRuntimeIdentityHandler struct {
	http.Handler
	identity runtimeapp.FinalPublicationAuthorityIdentityV1
}

func (handler staticRuntimeIdentityHandler) FinalPublicationAuthorityIdentityV1() (runtimeapp.FinalPublicationAuthorityIdentityV1, error) {
	return handler.identity, nil
}

func (handler staticRuntimeIdentityHandler) Shutdown(ctx context.Context) error {
	if lifecycle, ok := handler.Handler.(interface{ Shutdown(context.Context) error }); ok {
		return lifecycle.Shutdown(ctx)
	}
	return nil
}

type immediateFailureListener struct {
	addr net.Addr
}

func (*immediateFailureListener) Accept() (net.Conn, error) {
	return nil, errors.New("injected listener failure")
}

func (*immediateFailureListener) Close() error { return nil }
func (listener *immediateFailureListener) Addr() net.Addr {
	return listener.addr
}

type retryLeaseLifecycleHandler struct {
	mu            sync.Mutex
	calls         int
	secondOnce    sync.Once
	firstError    error
	secondEntered chan struct{}
	releaseSecond chan struct{}
}

func (*retryLeaseLifecycleHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func (handler *retryLeaseLifecycleHandler) Shutdown(ctx context.Context) error {
	handler.mu.Lock()
	handler.calls++
	call := handler.calls
	handler.mu.Unlock()
	if call == 1 {
		if handler.firstError != nil {
			return handler.firstError
		}
		return errors.New("background job did not stop")
	}
	handler.secondOnce.Do(func() { close(handler.secondEntered) })
	select {
	case <-handler.releaseSecond:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *blockingShutdownLifecycleHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *blockingShutdownLifecycleHandler) Shutdown(ctx context.Context) error {
	close(h.entered)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *shutdownLifecycleHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func (h *shutdownLifecycleHandler) Shutdown(ctx context.Context) error {
	h.called = true
	_, h.hadDeadline = ctx.Deadline()
	return nil
}

type runtimeReadyLine struct {
	URL                                             string `json:"url"`
	RuntimePID                                      int    `json:"runtimePid"`
	RuntimeTokenConfigured                          bool   `json:"runtimeTokenConfigured"`
	PersistenceRootsConfigured                      bool   `json:"persistenceRootsConfigured"`
	ControlledArtifactHostV2Configured              bool   `json:"controlledArtifactHostV2Configured"`
	ControlledArtifactHostV2Ready                   bool   `json:"controlledArtifactHostV2Ready"`
	FinalPublicationAuthorityKeyID                  string `json:"finalPublicationAuthorityKeyId"`
	FinalPublicationAuthorityPublicKey              string `json:"finalPublicationAuthorityPublicKey"`
	WitnessedAuthorityV2Configured                  bool   `json:"witnessedAuthorityV2Configured"`
	WitnessedAuthorityInstallationID                string `json:"witnessedAuthorityInstallationId"`
	WitnessedAuthorityKeyID                         string `json:"witnessedAuthorityKeyId"`
	WitnessedAuthorityManifestDigest                string `json:"witnessedAuthorityManifestDigest"`
	DatasetSnapshotSelectionV2Configured            bool   `json:"datasetSnapshotSelectionV2Configured"`
	DatasetSnapshotAdmissionV2State                 string `json:"datasetSnapshotAdmissionV2State"`
	DatasetSnapshotAdmissionV2InstallationID        string `json:"datasetSnapshotAdmissionV2InstallationId"`
	DatasetSnapshotAdmissionV2RuntimeLaunchNonce    string `json:"datasetSnapshotAdmissionV2RuntimeLaunchNonce"`
	DatasetSnapshotAdmissionV2StagingBindingDigest  string `json:"datasetSnapshotAdmissionV2StagingBindingDigest"`
	DatasetSnapshotAdmissionV2SelectionDigest       string `json:"datasetSnapshotAdmissionV2SelectionDigest"`
	DatasetSnapshotAdmissionV2SnapshotID            string `json:"datasetSnapshotAdmissionV2SnapshotId"`
	DatasetSnapshotAdmissionV2AuthorityRecordDigest string `json:"datasetSnapshotAdmissionV2AuthorityRecordDigest"`
	DatasetSnapshotAdmissionV2ACKHMACSHA256         string `json:"datasetSnapshotAdmissionV2AckHmacSha256"`
	RawPayload                                      string `json:"-"`
}

func waitForRuntimeReadyFromStdout(t *testing.T, stdout interface{ Read([]byte) (int, error) }) runtimeReadyLine {
	t.Helper()
	ready, err := readRuntimeReadyFromStdout(t, stdout)
	if err != nil {
		t.Fatalf("runtime subprocess ended before ready: %v", err)
	}
	return ready
}

func readRuntimeReadyFromStdout(t *testing.T, stdout interface{ Read([]byte) (int, error) }) (runtimeReadyLine, error) {
	t.Helper()
	readyCh := make(chan runtimeReadyLine, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := newLimitedScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "ANALYTIX_RUNTIME_SERVER_READY ") {
				continue
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "ANALYTIX_RUNTIME_SERVER_READY ")), &raw); err != nil {
				errCh <- err
				return
			}
			for _, forbidden := range []string{"runtimeToken", "durableRoot", "productionDurableRoot", "candidateDurableRoot", "durableTempDir"} {
				if _, ok := raw[forbidden]; ok {
					errCh <- fmt.Errorf("ready payload must not expose %s", forbidden)
					return
				}
			}
			var ready runtimeReadyLine
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "ANALYTIX_RUNTIME_SERVER_READY ")), &ready); err != nil {
				errCh <- err
				return
			}
			ready.RawPayload = line
			readyCh <- ready
			return
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
			return
		}
		errCh <- os.ErrClosed
	}()
	select {
	case ready := <-readyCh:
		return ready, nil
	case err := <-errCh:
		return runtimeReadyLine{}, err
	case <-time.After(runtimeReadyTimeout(t)):
		return runtimeReadyLine{}, errors.New("runtime subprocess did not become ready")
	}
}

func runtimeReadyTimeout(t *testing.T) time.Duration {
	t.Helper()
	const maximum = 30 * time.Second
	timeout := maximum
	if deadline, ok := t.Deadline(); ok {
		const cleanupBudget = time.Second
		remaining := time.Until(deadline) - cleanupBudget
		if remaining <= 0 {
			return time.Millisecond
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return timeout
}

func newLimitedScanner(reader interface{ Read([]byte) (int, error) }) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return scanner
}
