package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	providerclient "analytix.local/runtime-go/internal/adapters/outbound/provider/client"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
)

func TestControlledArtifactHostV2ConfigurationIsTLSBoundAndComplete(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x54}, 32))
	rootDER := controlledArtifactHostV2RootCertificate(t)
	rootText := base64.RawURLEncoding.EncodeToString(rootDER)
	pin := hex.EncodeToString(bytes.Repeat([]byte{0xab}, sha256.Size))
	valid := Config{
		ControlledArtifactHostV2URL: "https://127.0.0.1:45678", ControlledArtifactHostV2Token: secret,
		ControlledArtifactHostV2BackendGeneration: "9", ControlledArtifactHostV2AllocationDigest: strings.Repeat("c", 64),
		ControlledArtifactHostV2TLSRootCertDER:    rootText,
		ControlledArtifactHostV2TLSLeafSPKISHA256: pin,
	}
	if err := validateControlledArtifactHostConfigV2(valid); err != nil {
		t.Fatalf("valid desktop controlled artifact V2 host was rejected: %v", err)
	}
	if err := validateControlledArtifactHostConfigV2(Config{}); err != nil {
		t.Fatalf("absent desktop controlled artifact V2 host should disable the capability: %v", err)
	}
	for _, mutate := range []func(*Config){
		func(config *Config) { config.ControlledArtifactHostV2URL = "" },
		func(config *Config) { config.ControlledArtifactHostV2Token = "" },
		func(config *Config) { config.ControlledArtifactHostV2BackendGeneration = "" },
		func(config *Config) { config.ControlledArtifactHostV2AllocationDigest = "" },
		func(config *Config) { config.ControlledArtifactHostV2TLSRootCertDER = "" },
		func(config *Config) { config.ControlledArtifactHostV2TLSLeafSPKISHA256 = "" },
		func(config *Config) { config.ControlledArtifactHostV2URL = "http://127.0.0.1:45678" },
		func(config *Config) { config.ControlledArtifactHostV2URL = "https://localhost:45678" },
		func(config *Config) { config.ControlledArtifactHostV2Token = "weak-token" },
		func(config *Config) { config.ControlledArtifactHostV2BackendGeneration = "01" },
		func(config *Config) { config.ControlledArtifactHostV2BackendGeneration = "9007199254740992" },
		func(config *Config) { config.ControlledArtifactHostV2AllocationDigest = strings.Repeat("C", 64) },
		func(config *Config) { config.ControlledArtifactHostV2TLSRootCertDER += "=" },
		func(config *Config) { config.ControlledArtifactHostV2TLSLeafSPKISHA256 = strings.ToUpper(pin) },
	} {
		config := valid
		mutate(&config)
		if err := validateControlledArtifactHostConfigV2(config); err == nil {
			t.Fatalf("invalid desktop controlled artifact V2 host configuration was accepted: %#v", config)
		}
	}
}

func TestControlledArtifactHostV2ProbeIsAbsentOrFailClosed(t *testing.T) {
	if err := ProbeControlledArtifactHostV2(context.Background(), Config{}); err != nil {
		t.Fatalf("absent V2 host should not require a probe: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ProbeControlledArtifactHostV2(cancelled, Config{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled absent probe lost cancellation: %v", err)
	}
	if err := ProbeControlledArtifactHostV2(context.Background(), Config{
		ControlledArtifactHostV2URL: "https://127.0.0.1:1",
	}); err == nil {
		t.Fatal("partial V2 host configuration reached a network probe")
	}
}

func controlledArtifactHostV2RootCertificate(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "analytix controlled host config root"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, IsCA: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestRuntimeProtectedRootTopologyKeepsGeneralRuntimeUsable(t *testing.T) {
	root := t.TempDir()
	for _, testCase := range []struct {
		name   string
		config Config
	}{
		{
			name: "same",
			config: Config{
				DataDir: filepath.Join(root, "same"), UserDataDir: filepath.Join(root, "same"),
			},
		},
		{
			name: "data inside user data",
			config: Config{
				DataDir: filepath.Join(root, "user-data", "data"), UserDataDir: filepath.Join(root, "user-data"),
			},
		},
		{
			name: "user data inside data",
			config: Config{
				DataDir: filepath.Join(root, "data"), UserDataDir: filepath.Join(root, "data", "user-data"),
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := validateRuntimeProtectedRootTopology(normalizeConfig(testCase.config)); err == nil {
				t.Fatal("overlapping data and user-data roots were accepted")
			}
		})
	}

	separate := normalizeConfig(Config{
		DataDir: filepath.Join(root, "runtime-data"), UserDataDir: filepath.Join(root, "electron-user-data"),
	})
	if err := validateRuntimeProtectedRootTopology(separate); err != nil {
		t.Fatalf("separate runtime and user-data roots were rejected: %v", err)
	}
	for name, protectedRoot := range map[string]string{
		"same as data":     separate.DataDir,
		"ancestor of data": root,
	} {
		t.Run("authority "+name, func(t *testing.T) {
			config := separate
			config.AuthorityManifestRoot = protectedRoot
			if err := validateRuntimeProtectedRootTopology(config); err == nil {
				t.Fatal("authority root containing runtime data was accepted")
			}
		})
	}
	insideData := separate
	insideData.AuthorityManifestRoot = filepath.Join(separate.DataDir, "private", "authority-manifest")
	insideData.AuthorityCredentialProfileRoot = filepath.Join(separate.DataDir, "private", "authority-profile")
	insideData.AuthorityCredentialBundleRoot = filepath.Join(separate.DataDir, "private", "authority-bundle")
	if err := validateRuntimeProtectedRootTopology(insideData); err != nil {
		t.Fatalf("mandatory private child roots were rejected: %v", err)
	}
}

func TestRuntimeAppLoadsSettingsFromSingleConfigDocument(t *testing.T) {
	config := Config{
		DataDir:                        t.TempDir(),
		UserDataDir:                    t.TempDir(),
		AuthorityManifestRoot:          t.TempDir(),
		AuthorityCredentialProfileRoot: t.TempDir(),
		AuthorityCredentialBundleRoot:  t.TempDir(),
		AllowWriteRoots:                []string{t.TempDir()},
		MCPConfigJSON: `{
			"capabilities": {
				"mcp": {
					"search": {"enabled": true, "topKDefault": 6},
					"sandbox": {"allow_write": ["` + filepath.ToSlash(t.TempDir()) + `"]}
				},
				"web": {"enabled": true, "fetch_enabled": true},
				"visionBridge": {"enabled": true, "mode": "always"}
			},
			"runtime": {
				"streamIdleTimeoutMs": 0,
				"stepLimits": {"defaultMaxModelSteps": 9}
			}
		}`,
	}
	document, ok, err := loadRuntimeConfigDocument(config)
	if err != nil || !ok {
		t.Fatalf("config document should load: ok=%v err=%v", ok, err)
	}
	mcpSearch := loadRuntimeMCPSearchSettings(document, ok)
	if !mcpSearch.Enabled || mcpSearch.TopKDefault != 6 {
		t.Fatalf("mcp search mismatch: %#v", mcpSearch)
	}
	sandbox := loadRuntimeSandboxSettings(config, document, ok)
	if len(sandbox.AllowWriteRoots) != 2 {
		t.Fatalf("sandbox should include config and document roots: %#v", sandbox)
	}
	privateRoot := filestore.NormalizeRealRoots([]string{filepath.Join(config.DataDir, "private")})[0]
	if !slices.Contains(sandbox.ProtectedReadDirs, filestore.MandatoryProtectedRoot(privateRoot)) {
		t.Fatalf("runtime private authority root must be protected from file tools: %#v", sandbox.ProtectedReadDirs)
	}
	childRunRoot := filestore.NormalizeRealRoots([]string{filepath.Join(config.DataDir, "child-runs")})[0]
	if !slices.Contains(sandbox.ProtectedReadDirs, filestore.MandatoryProtectedRoot(childRunRoot)) {
		t.Fatalf("runtime child-run output root must be protected from file tools: %#v", sandbox.ProtectedReadDirs)
	}
	for name, root := range map[string]string{
		"user-data":                    config.UserDataDir,
		"authority manifest":           config.AuthorityManifestRoot,
		"authority credential profile": config.AuthorityCredentialProfileRoot,
		"authority credential bundle":  config.AuthorityCredentialBundleRoot,
	} {
		normalizedRoot := filestore.NormalizeRealRoots([]string{root})[0]
		if !slices.Contains(sandbox.ProtectedReadDirs, filestore.MandatoryProtectedRoot(normalizedRoot)) {
			t.Fatalf("runtime %s root must be protected from untrusted tools: %#v", name, sandbox.ProtectedReadDirs)
		}
	}
	if web := loadRuntimeWebConfig(document, ok); !web.Enabled || !web.FetchEnabled {
		t.Fatalf("web config mismatch: %#v", web)
	}
	if vision := loadRuntimeVisionBridgeConfig(document, ok); !vision.Enabled || vision.Mode != "always" {
		t.Fatalf("vision bridge config mismatch: %#v", vision)
	}
	stepLimits := loadRuntimeStepLimitConfig(document, ok)
	if stepLimits.DefaultMaxModelSteps == nil || *stepLimits.DefaultMaxModelSteps != 9 {
		t.Fatalf("step limits mismatch: %#v", stepLimits)
	}
	streamIdleTimeout, configured := loadRuntimeStreamIdleTimeout(document, ok)
	if !configured || streamIdleTimeout != 0 {
		t.Fatalf("explicit zero stream idle timeout should disable the watchdog: configured=%v timeout=%s", configured, streamIdleTimeout)
	}
}

func TestRuntimeConfigurationConsumersUseOneImmutableSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	oldBody := `{"servers":{"old-server":{"command":"old-command","trustScope":"user"}},"runtime":{"streamIdleTimeoutMs":1000,"userInput":{"policy":"timed","defaultTimeoutSeconds":7}}}`
	newBody := `{"servers":{"new-server":{"command":"new-command","trustScope":"user"}},"runtime":{"streamIdleTimeoutMs":2000,"userInput":{"policy":"legacy","defaultTimeoutSeconds":2}}}`
	if err := os.WriteFile(path, []byte(oldBody), 0o600); err != nil {
		t.Fatal(err)
	}
	config := Config{DataDir: t.TempDir(), MCPConfigPath: path}
	snapshot, err := loadRuntimeConfigurationSnapshot(config)
	if err != nil {
		t.Fatalf("load immutable runtime configuration: %v", err)
	}
	if err := os.WriteFile(path, []byte(newBody), 0o600); err != nil {
		t.Fatal(err)
	}
	specs, err := loadRuntimeMCPServerSpecs(snapshot, config.DataDir)
	if err != nil || len(specs) != 1 || specs[0].ID != "old-server" || specs[0].Command != "old-command" {
		t.Fatalf("MCP consumer observed a different generation: specs=%#v err=%v", specs, err)
	}
	document, ok, err := loadRuntimeConfigDocumentFromSnapshot(snapshot)
	if err != nil || !ok {
		t.Fatalf("document consumer rejected frozen generation: ok=%v err=%v", ok, err)
	}
	timeout, configured := loadRuntimeStreamIdleTimeout(document, ok)
	if !configured || timeout != time.Second {
		t.Fatalf("document consumer observed a different generation: timeout=%s configured=%v", timeout, configured)
	}
	userInput, present, err := loadRuntimeUserInputConfig(document, ok)
	if err != nil || !present || userInput.Mode != "timed" || userInput.TimeoutSeconds == nil || *userInput.TimeoutSeconds != 7 {
		t.Fatalf("user input consumer observed a different generation: present=%v config=%#v err=%v", present, userInput, err)
	}
}

func TestRuntimeConfigurationSnapshotPreservesMCPWorkspaceOverrides(t *testing.T) {
	workspaceRoot := t.TempDir()
	snapshot, err := loadRuntimeConfigurationSnapshot(Config{
		DataDir:       workspaceRoot,
		MCPConfigJSON: `{"servers":{"codegraph":{"command":"codegraph","trustScope":"user"}}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	specs, err := loadRuntimeMCPServerSpecs(snapshot, workspaceRoot)
	if err != nil || len(specs) != 1 {
		t.Fatalf("load MCP specs from immutable snapshot: specs=%#v err=%v", specs, err)
	}
	if specs[0].ID != "codegraph" || specs[0].CWD != workspaceRoot || !specs[0].LowPriority || !specs[0].BackgroundStart {
		t.Fatalf("immutable snapshot lost ordinary workspace overrides: %#v", specs[0])
	}
}

func TestRuntimeAppConfigDefaultsWithoutDocument(t *testing.T) {
	var document map[string]any
	if loadRuntimeMCPSearchSettings(document, false).Enabled {
		t.Fatal("mcp search should default disabled")
	}
	if loadRuntimeWebConfig(document, false).MaxFetchBytes <= 0 {
		t.Fatal("web config should keep default limits")
	}
	if loadRuntimeVisionBridgeConfig(document, false).Mode != "auto" {
		t.Fatal("vision bridge should default to auto")
	}
	if loadRuntimeStepLimitConfig(document, false).DefaultMaxModelSteps != nil {
		t.Fatal("step limits should default empty without config")
	}
	if _, configured := loadRuntimeStreamIdleTimeout(document, false); configured {
		t.Fatal("stream idle timeout should remain unconfigured without a document")
	}
	if timeout, configured := loadRuntimeStreamIdleTimeout(map[string]any{
		"runtime": map[string]any{"streamIdleTimeoutMs": float64(45000)},
	}, true); !configured || timeout != 45*time.Second {
		t.Fatalf("stream idle timeout mismatch: configured=%v timeout=%s", configured, timeout)
	}
}

func TestRuntimeAppNormalizeConfigDefaultsToSafeInteractivePolicy(t *testing.T) {
	config := normalizeConfig(Config{Insecure: true})
	if config.ApprovalPolicy != "on-request" {
		t.Fatalf("approval policy = %q, want on-request", config.ApprovalPolicy)
	}
	if config.SandboxMode != "workspace-write" {
		t.Fatalf("sandbox mode = %q, want workspace-write", config.SandboxMode)
	}
}

func TestRuntimeUserInputConfiguration(t *testing.T) {
	cases := []struct {
		name, document, policy string
		seconds                int
		present, invalid       bool
	}{
		{name: "absent document", policy: "legacy", seconds: 120},
		{name: "empty document", document: `{}`, policy: "legacy", seconds: 120},
		{name: "empty runtime", document: `{"runtime":{}}`, policy: "legacy", seconds: 120},
		{name: "MCP only", document: `{"servers":{}}`, policy: "legacy", seconds: 120},
		{name: "legacy", document: `{"runtime":{"userInput":{"policy":"legacy"}}}`, policy: "legacy", seconds: 120, present: true},
		{name: "legacy timeout", document: `{"runtime":{"userInput":{"policy":"legacy","defaultTimeoutSeconds":7}}}`, policy: "legacy", seconds: 7, present: true},
		{name: "timed default", document: `{"runtime":{"userInput":{"policy":"timed"}}}`, policy: "timed", seconds: 120, present: true},
		{name: "timed null", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":null}}}`, policy: "timed", seconds: 120, present: true},
		{name: "timed infinite", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":-1}}}`, policy: "timed", seconds: -1, present: true},
		{name: "timed minimum", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":1}}}`, policy: "timed", seconds: 1, present: true},
		{name: "timed maximum", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":2147483}}}`, policy: "timed", seconds: 2147483, present: true},
		{name: "null leaf", document: `{"runtime":{"userInput":null}}`, present: true, invalid: true},
		{name: "array leaf", document: `{"runtime":{"userInput":[]}}`, present: true, invalid: true},
		{name: "string leaf", document: `{"runtime":{"userInput":"timed"}}`, present: true, invalid: true},
		{name: "boolean leaf", document: `{"runtime":{"userInput":true}}`, present: true, invalid: true},
		{name: "missing policy", document: `{"runtime":{"userInput":{}}}`, present: true, invalid: true},
		{name: "timeout alone", document: `{"runtime":{"userInput":{"defaultTimeoutSeconds":7}}}`, present: true, invalid: true},
		{name: "null policy", document: `{"runtime":{"userInput":{"policy":null}}}`, present: true, invalid: true},
		{name: "boolean policy", document: `{"runtime":{"userInput":{"policy":true}}}`, present: true, invalid: true},
		{name: "unknown policy", document: `{"runtime":{"userInput":{"policy":"auto"}}}`, present: true, invalid: true},
		{name: "empty policy", document: `{"runtime":{"userInput":{"policy":""}}}`, present: true, invalid: true},
		{name: "uppercase policy", document: `{"runtime":{"userInput":{"policy":"Timed"}}}`, present: true, invalid: true},
		{name: "padded policy", document: `{"runtime":{"userInput":{"policy":"timed "}}}`, present: true, invalid: true},
		{name: "mode alias", document: `{"runtime":{"userInput":{"mode":"timed"}}}`, present: true, invalid: true},
		{name: "timeout alias", document: `{"runtime":{"userInput":{"policy":"timed","timeoutSeconds":7}}}`, present: true, invalid: true},
		{name: "unknown field", document: `{"runtime":{"userInput":{"policy":"timed","enabled":true}}}`, present: true, invalid: true},
		{name: "zero timeout", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":0}}}`, present: true, invalid: true},
		{name: "negative timeout", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":-2}}}`, present: true, invalid: true},
		{name: "above maximum", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":2147484}}}`, present: true, invalid: true},
		{name: "fractional timeout", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":1.5}}}`, present: true, invalid: true},
		{name: "string timeout", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":"7"}}}`, present: true, invalid: true},
		{name: "boolean timeout", document: `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":true}}}`, present: true, invalid: true},
		{name: "invalid legacy timeout", document: `{"runtime":{"userInput":{"policy":"legacy","defaultTimeoutSeconds":0}}}`, present: true, invalid: true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			snapshot, err := loadRuntimeConfigurationSnapshot(Config{MCPConfigJSON: item.document})
			if err != nil {
				t.Fatal(err)
			}
			document, ok, err := loadRuntimeConfigDocumentFromSnapshot(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			original, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			config, present, err := loadRuntimeUserInputConfig(document, ok)
			if present != item.present {
				t.Fatalf("leaf presence = %v, want %v", present, item.present)
			}
			if item.invalid {
				if err == nil || !strings.HasPrefix(err.Error(), "runtime.userInput") {
					t.Fatalf("invalid leaf did not fail explicitly: %v", err)
				}
				return
			}
			if err != nil || config.Mode != item.policy || config.TimeoutSeconds == nil || *config.TimeoutSeconds != item.seconds {
				t.Fatalf("config = %#v, error = %v; want %s/%d", config, err, item.policy, item.seconds)
			}
			wait, err := toolcatalogapp.UserInputTimedWait(config, json.RawMessage(`{"prompt":"synthetic"}`))
			if err != nil {
				t.Fatal(err)
			}
			if config.Mode == "legacy" && wait != nil {
				t.Fatal("legacy configuration enabled a timer")
			}
			if config.Mode == "timed" && (wait == nil || wait.TimeoutSeconds != item.seconds) {
				t.Fatal("timed configuration did not reach the wait policy")
			}
			after, err := json.Marshal(document)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("normalization mutated the immutable document")
			}
		})
	}
}

func TestRuntimeUserInputConfigurationDigest(t *testing.T) {
	for _, body := range []string{"", `{}`, `{"servers":{}}`, `{"runtime":{}}`, `{"runtime":{"userInput":{"policy":"legacy"}}}`, `{"runtime":{"userInput":{"policy":"timed"}}}`} {
		t.Run(body, func(t *testing.T) {
			config := Config{MCPConfigJSON: body, DataDir: t.TempDir()}
			_, specs, document, ok, err := loadRuntimeConfiguration(config)
			if err != nil {
				t.Fatal(err)
			}
			skills, err := loadRuntimeSkillCatalog(config, document, ok)
			if err != nil {
				t.Fatal(err)
			}
			subagents, err := loadRuntimeSubagentConfig(document, ok)
			if err != nil {
				t.Fatal(err)
			}
			// Preserve the pre-opt-in twelve input positions, including a present
			// runtime/MCP document that has no userInput leaf.
			legacyValues := []any{
				runtimeStartupSecurityConfiguration(config), specs, document, ok,
				loadRuntimeMCPSearchSettings(document, ok), skills, subagents,
				loadRuntimeSandboxSettings(config, document, ok), loadRuntimeWebConfig(document, ok),
				loadRuntimeVisionBridgeConfig(document, ok), loadRuntimeStepLimitConfig(document, ok), providerclient.DefaultStreamIdleTimeout,
			}
			legacyDigest, err := runtimeStartupConfigurationDigest(legacyValues...)
			if err != nil {
				t.Fatal(err)
			}
			userInput, present, err := loadRuntimeUserInputConfig(document, ok)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := runtimeUserInputConfigurationDigest(userInput, present, legacyValues...)
			if err != nil {
				t.Fatal(err)
			}
			if !present {
				if actual != legacyDigest {
					t.Fatal("absent leaf changed the legacy twelve-input digest")
				}
				return
			}
			if actual == legacyDigest {
				t.Fatal("explicit policy was not bound into the configuration digest")
			}
			changed := 7
			userInput.TimeoutSeconds = &changed
			changedDigest, err := runtimeUserInputConfigurationDigest(userInput, true, legacyValues...)
			if err != nil || changedDigest == actual {
				t.Fatal("normalized timeout is not bound into the configuration digest")
			}
		})
	}
}
