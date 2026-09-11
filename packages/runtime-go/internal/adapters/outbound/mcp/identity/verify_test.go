package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
)

func TestVerifyConfiguredProvenanceRejectsHighRiskHTTPServer(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	spec.Transport = "http"
	spec.URL = "https://attacker.invalid/mcp"
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "local stdio") {
		t.Fatalf("spoofed HTTP funds server must fail closed: %v", err)
	}
}

func TestVerifyConfiguredProvenanceRejectsMissingPins(t *testing.T) {
	base := validPinnedFundsSpec(t)
	tests := map[string]func(*domainmcp.ServerSpec){
		"identity source": func(spec *domainmcp.ServerSpec) { spec.IdentitySource = "" },
		"expected name":   func(spec *domainmcp.ServerSpec) { spec.ExpectedServerName = "" },
		"expected version": func(spec *domainmcp.ServerSpec) {
			spec.ExpectedServerVersion = ""
		},
		"manifest digest":   func(spec *domainmcp.ServerSpec) { spec.ManifestSHA256 = "" },
		"entrypoint path":   func(spec *domainmcp.ServerSpec) { spec.EntrypointPath = "" },
		"entrypoint digest": func(spec *domainmcp.ServerSpec) { spec.EntrypointSHA256 = "" },
		"plugin root":       func(spec *domainmcp.ServerSpec) { spec.PluginRootPath = "" },
		"source tree digest": func(spec *domainmcp.ServerSpec) {
			spec.SourceTreeSHA256 = ""
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := base
			mutate(&spec)
			if err := VerifyConfiguredProvenance(spec); err == nil {
				t.Fatalf("missing %s pin must fail closed", name)
			}
		})
	}
}

func TestVerifyConfiguredProvenanceRejectsEntrypointIntegrityMismatch(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	spec.EntrypointSHA256 = strings.Repeat("0", 64)
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("entrypoint hash mismatch must fail closed: %v", err)
	}
}

func TestVerifyConfiguredProvenanceRejectsManifestIntegrityMismatch(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	spec.ManifestSHA256 = strings.Repeat("0", 64)
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "manifest integrity mismatch") {
		t.Fatalf("manifest hash mismatch must fail closed: %v", err)
	}
}

func TestVerifyConfiguredProvenanceRejectsEntrypointOutsidePluginRoot(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	outsideRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideRoot, "server.mjs")
	body := []byte("export const outside = true\n")
	if err := os.WriteFile(outside, body, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	spec.Command = outside
	spec.EntrypointPath = outside
	spec.EntrypointSHA256 = hex.EncodeToString(digest[:])
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "outside the plugin root") {
		t.Fatalf("external entrypoint must fail closed: %v", err)
	}
}

func TestVerifyConfiguredProvenanceRejectsImportedSourceMutation(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	if err := os.WriteFile(filepath.Join(spec.PluginRootPath, "runtime.mjs"), []byte("export const changed = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "source-tree integrity mismatch") {
		t.Fatalf("imported source mutation must fail closed: %v", err)
	}
}

func TestVerifyConfiguredProvenanceBindsCompleteHostInstalledGeneration(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	markerPath := filepath.Join(spec.PluginRootPath, ".analytix-hub-installed-plugin.json")
	markerBody, err := json.Marshal(struct {
		ManagedBy           string `json:"managedBy"`
		MarketplaceName     string `json:"marketplaceName"`
		PluginName          string `json:"pluginName"`
		Version             string `json:"version"`
		PackageSHA256       string `json:"packageSha256"`
		SourcePath          string `json:"sourcePath"`
		SourceTreeSHA256    string `json:"sourceTreeSha256"`
		SourceTreeFileCount uint64 `json:"sourceTreeFileCount"`
		InstallType         string `json:"installType"`
	}{
		ManagedBy: "analytix-hub", MarketplaceName: "analytix-hub",
		PluginName: "analytix-fund-analysis", Version: spec.ExpectedServerVersion,
		PackageSHA256: strings.Repeat("a", 64), SourcePath: spec.PluginRootPath,
		SourceTreeSHA256: spec.SourceTreeSHA256, SourceTreeFileCount: 2, InstallType: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, markerBody, 0o600); err != nil {
		t.Fatal(err)
	}
	spec.IdentitySource = HostInstalledGenerationSourceV1
	markerDigest := sha256.Sum256(markerBody)
	spec.HostInstallMarkerSHA256 = hex.EncodeToString(markerDigest[:])
	if err := VerifyConfiguredProvenance(spec); err != nil {
		t.Fatalf("complete host-installed generation was rejected: %v", err)
	}
	importedPath := filepath.Join(spec.PluginRootPath, "runtime.mjs")
	if err := os.WriteFile(importedPath, []byte("export const drift = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfiguredProvenance(spec); err == nil {
		t.Fatal("unsigned imported host source drift was accepted")
	}
	if err := os.Remove(importedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte(`{"managedBy":"attacker"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConfiguredProvenance(spec); err == nil {
		t.Fatalf("host installation marker drift must fail closed: %v", err)
	}
}

func TestSourceTreeDigestMatchesDesktopContract(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.mjs"), []byte("export const a = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "b.json"), []byte("{\"b\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := ComputeSourceTreeSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	if digest != "b61f6361ff562bcc58ec5e40f258cb462857d2ae423b4639936df377e1e4ddf3" {
		t.Fatalf("desktop/go source-tree digest drift: %s", digest)
	}
}

func TestVerifyConfiguredProvenanceRequiresCommandBindingToPinnedEntrypoint(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	spec.Command = "node"
	spec.Args = []string{"--no-warnings", filepath.Join(t.TempDir(), "decoy.mjs"), spec.EntrypointPath}
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("a later decoy-bypassed entrypoint argument must fail closed: %v", err)
	}

	spec.Args = []string{"--no-warnings"}
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("an unbound runtime command must fail closed: %v", err)
	}

	spec.Command = ""
	spec.Args = []string{spec.EntrypointPath}
	if err := VerifyConfiguredProvenance(spec); err == nil || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("entrypoint arguments without a configured command must fail closed: %v", err)
	}
}

func TestVerifyConfiguredProvenanceAcceptsPinnedStdioBindings(t *testing.T) {
	t.Run("command is entrypoint", func(t *testing.T) {
		spec := validPinnedFundsSpec(t)
		if err := VerifyConfiguredProvenance(spec); err != nil {
			t.Fatalf("direct pinned command was rejected: %v", err)
		}
	})

	t.Run("first non flag argument is entrypoint", func(t *testing.T) {
		spec := validPinnedFundsSpec(t)
		spec.Command = "node"
		spec.Args = []string{"--no-warnings", "--", spec.EntrypointPath, "--stdio"}
		if err := VerifyConfiguredProvenance(spec); err != nil {
			t.Fatalf("pinned script argument was rejected: %v", err)
		}
	})
}

func TestVerifyObservedRejectsMissingOrMismatchedPinnedIdentity(t *testing.T) {
	spec := validPinnedFundsSpec(t)
	tests := map[string]domainmcp.ServerIdentity{
		"missing name":    {Version: spec.ExpectedServerVersion},
		"wrong name":      {Name: "spoofed-funds", Version: spec.ExpectedServerVersion},
		"missing version": {Name: spec.ExpectedServerName},
		"wrong version":   {Name: spec.ExpectedServerName, Version: "0.0.0-attacker"},
	}
	for name, observed := range tests {
		t.Run(name, func(t *testing.T) {
			if err := VerifyObserved(spec, observed); err == nil {
				t.Fatalf("%s must fail observed identity verification", name)
			}
		})
	}

	observed := domainmcp.ServerIdentity{
		ProtocolVersion: "2025-11-25",
		Name:            spec.ExpectedServerName,
		Version:         spec.ExpectedServerVersion,
	}
	if err := VerifyObserved(spec, observed); err != nil {
		t.Fatalf("matching pinned identity was rejected: %v", err)
	}
}

func validPinnedFundsSpec(t *testing.T) domainmcp.ServerSpec {
	t.Helper()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	entrypoint := filepath.Join(root, "server.mjs")
	body := []byte("export const server = 'analytix_funds'\n")
	if err := os.WriteFile(entrypoint, body, 0o600); err != nil {
		t.Fatalf("write test entrypoint: %v", err)
	}
	manifestBody := []byte("{\"name\":\"analytix-fund-analysis\",\"version\":\"1.2.3\"}\n")
	if err := os.MkdirAll(filepath.Join(root, ".codex-plugin"), 0o700); err != nil {
		t.Fatalf("create test manifest directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex-plugin", "plugin.json"), manifestBody, 0o600); err != nil {
		t.Fatalf("write test manifest: %v", err)
	}
	digest := sha256.Sum256(body)
	manifestDigest := sha256.Sum256(manifestBody)
	treeDigest, err := ComputeSourceTreeSHA256(root)
	if err != nil {
		t.Fatalf("hash test plugin source tree: %v", err)
	}
	return domainmcp.ServerSpec{
		ID:                    "analytix_funds",
		Transport:             "stdio",
		Command:               entrypoint,
		ExpectedServerName:    "analytix_funds",
		ExpectedServerVersion: "1.2.3",
		IdentitySource:        installedPluginManifestSource,
		ManifestSHA256:        hex.EncodeToString(manifestDigest[:]),
		EntrypointPath:        entrypoint,
		EntrypointSHA256:      hex.EncodeToString(digest[:]),
		PluginRootPath:        root,
		SourceTreeSHA256:      treeDigest,
	}
}
