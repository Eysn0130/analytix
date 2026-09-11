package pluginmaterializationfs

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const legacyFundsCanonicalDeclarationV0 = `{"schemaVersion":1,"packageId":"analytix-fund-analysis","packageVersion":"0.16.16","contributions":{"skills":[{"id":"account-dossier","path":"skills/account-dossier/SKILL.md"},{"id":"analysis-critique","path":"skills/analysis-critique/SKILL.md"},{"id":"analytix-fund-analysis","path":"skills/analytix-fund-analysis/SKILL.md"},{"id":"case-context","path":"skills/case-context/SKILL.md"},{"id":"case-workbench","path":"skills/case-workbench/SKILL.md"},{"id":"claim-review","path":"skills/claim-review/SKILL.md"},{"id":"counterparty-analysis","path":"skills/counterparty-analysis/SKILL.md"},{"id":"data-quality","path":"skills/data-quality/SKILL.md"},{"id":"delivery-qc","path":"skills/delivery-qc/SKILL.md"},{"id":"evidence-request","path":"skills/evidence-request/SKILL.md"},{"id":"full-case-analysis","path":"skills/full-case-analysis/SKILL.md"},{"id":"fund-tracing","path":"skills/fund-tracing/SKILL.md"},{"id":"graph-visualization","path":"skills/graph-visualization/SKILL.md"},{"id":"index","path":"skills/index/SKILL.md"},{"id":"investigation-lab","path":"skills/investigation-lab/SKILL.md"},{"id":"pair-amount-investigation","path":"skills/pair-amount-investigation/SKILL.md"},{"id":"quick-fact","path":"skills/quick-fact/SKILL.md"},{"id":"report-builder","path":"skills/report-builder/SKILL.md"},{"id":"subject-dossier","path":"skills/subject-dossier/SKILL.md"},{"id":"visual-evidence","path":"skills/visual-evidence/SKILL.md"}],"mcpServers":[{"id":"analytix_funds","entrypoint":"mcp/server.mjs"}],"hooks":[],"assets":[{"id":"funds-icon","path":"assets/icon.png"},{"id":"funds-logo","path":"assets/logo.png"}],"publicUi":[{"id":"openai-agent-ui","path":"agents/openai.yaml"}]},"requestedCapabilities":[{"id":"funds.case.read","protocolVersion":1,"scopeConstraints":["case:bound","source:verified"]},{"id":"funds.source.read","protocolVersion":1,"scopeConstraints":["case:bound","source:verified"]}],"lifecycle":{"protocolVersion":1,"entryPolicy":"host-static-first-party"}}`

func TestLegacyFundsV0MigrationDerivesExactCanonicalV1AndReentersStaticAdmission(t *testing.T) {
	source := writeLegacyFundsSourceV0(t, realTempDir(t), domainplugin.PluginNameV1, legacyFundsPackageVersionV0)
	identity, err := InspectPackagedFundsSourceTreeV1(context.Background(), source)
	if err != nil || !identity.LegacyV0 || identity.Declaration != (PackageDeclarationIdentityV1{}) {
		t.Fatalf("historical Funds artifact was not identified only as legacy-v0: identity=%#v err=%v", identity, err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := sha256HexLocal(publicKey)
	now := time.Date(2026, 8, 27, 4, 5, 6, 0, time.UTC)
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		PackageAuthoritySHA256: strings.Repeat("a", 64),
		Target:                 domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"},
		PluginName:             identity.LegacyPackageID,
		PluginVersion:          identity.LegacyVersion,
		SourceRoot:             identity.RootRealPath,
		SourceTreeSHA256:       identity.TreeSHA256,
		SourceTreeFileCount:    identity.FileCount,
		ManifestSHA256:         identity.ManifestSHA256,
		EntrypointSHA256:       identity.EntrypointSHA256,
		RequestedAt:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainplugin.NewReceiptV1(
		intent,
		strings.Repeat("b", 64),
		"plugins/cache/analytix-hub/analytix-fund-analysis/0.16.16",
		now,
		keyID,
		publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainplugin.NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateVerifiedLegacyFundsV0ToDeclarationV1(
		context.Background(), source, identity,
		pluginport.ResultV1{Receipt: receipt, Index: index}, keyID, publicKey,
	)
	if err != nil {
		t.Fatalf("trusted legacy-v0 chain did not migrate to schema v1: %v", err)
	}
	if migrated.CanonicalJSON != legacyFundsCanonicalDeclarationV0 || migrated.RawSHA256 != migrated.CanonicalSHA256 {
		t.Fatalf("legacy-v0 projection is not the exact canonical v1 declaration: %#v", migrated)
	}
	decision, err := domainpluginpackage.AdmitStaticFirstPartyV1(domainpluginpackage.StaticAdmissionInputV1{
		CanonicalDeclaration:       []byte(migrated.CanonicalJSON),
		DeclarationRawSHA256:       migrated.RawSHA256,
		DeclarationCanonicalSHA256: migrated.CanonicalSHA256,
		Evidence: domainpluginpackage.StaticAdmissionEvidenceV1{
			ArtifactIntegrityVerified: true,
			PackageAuthoritySHA256:    strings.Repeat("a", 64),
			ProvenanceAuthorityDigest: strings.Repeat("c", 64),
			ProvenanceClassification:  "development_dirty_non_publishable",
			ProvenanceDispositionKind: "development_non_publishable",
			PlatformAnchor:            "macos_nonpublishable_resource_seal",
			SigningAlgorithm:          domainpluginpackage.StaticAdmissionSigningAlgorithmV1,
		},
	})
	if err != nil || decision.Identity.PackageVersion != receipt.PluginVersion ||
		len(decision.RequestedCapabilities) != 2 {
		t.Fatalf("migrated declaration did not reenter the existing static admission: decision=%#v err=%v", decision, err)
	}
	if _, statErr := os.Lstat(filepath.Join(source, ".analytix-plugin", "package.json")); !os.IsNotExist(statErr) {
		t.Fatalf("legacy-v0 migration wrote back a companion: %v", statErr)
	}
}

func TestLegacyFundsV0CoexistsWithFutureCanonicalPublisherVersion(t *testing.T) {
	legacySource := writeLegacyFundsSourceV0(t, realTempDir(t), domainplugin.PluginNameV1, legacyFundsPackageVersionV0)
	legacyIdentity, err := InspectPackagedFundsSourceTreeV1(context.Background(), legacySource)
	if err != nil || !legacyIdentity.LegacyV0 || legacyIdentity.LegacyVersion != "0.16.16" {
		t.Fatalf("fixed historical legacy-v0 identity drifted: identity=%#v err=%v", legacyIdentity, err)
	}

	const futureMCPEntrypoint = "mcp/future-server.mjs"
	futureSource := writeLegacyFundsSourceV0(t, realTempDir(t), domainplugin.PluginNameV1, "0.16.17")
	if err := os.Remove(filepath.Join(futureSource, filepath.FromSlash(legacyFundsMCPEntrypointV0))); err != nil {
		t.Fatal(err)
	}
	futureEntrypoint := filepath.Join(futureSource, filepath.FromSlash(futureMCPEntrypoint))
	if err := os.WriteFile(futureEntrypoint, []byte("export const server = 'analytix_funds_future'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(futureSource, ".mcp.json"),
		[]byte(`{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/future-server.mjs"]}}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	futureDeclaration, err := domainpluginpackage.ParseDeclarationV1([]byte(legacyFundsCanonicalDeclarationV0))
	if err != nil {
		t.Fatal(err)
	}
	futureDeclaration.PackageVersion = "0.16.17"
	futureDeclaration.Contributions.MCPServers[0].Entrypoint = futureMCPEntrypoint
	futureCanonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(futureDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	futureCompanion := filepath.Join(futureSource, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
	if err := os.MkdirAll(filepath.Dir(futureCompanion), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(futureCompanion, futureCanonical, 0o600); err != nil {
		t.Fatal(err)
	}
	futureIdentity, err := InspectPackagedFundsSourceTreeV1(context.Background(), futureSource)
	if err != nil || futureIdentity.LegacyV0 || futureIdentity.Declaration.PackageVersion != "0.16.17" {
		t.Fatalf("future canonical publisher declaration did not coexist with fixed legacy-v0: identity=%#v err=%v", futureIdentity, err)
	}

	unsupportedLegacy := writeLegacyFundsSourceV0(t, realTempDir(t), domainplugin.PluginNameV1, "0.16.17")
	if identity, err := InspectPackagedFundsSourceTreeV1(context.Background(), unsupportedLegacy); err == nil || identity.LegacyV0 {
		t.Fatalf("future publisher version was accepted without a canonical companion: identity=%#v err=%v", identity, err)
	}
}

func TestLegacyFundsV0NeverFallsBackFromPresentCompanion(t *testing.T) {
	tests := map[string]string{
		"unknown field":      `{"schemaVersion":1,"unknown":true}`,
		"duplicate field":    `{"schemaVersion":1,"schemaVersion":1}`,
		"unsupported schema": `{"schemaVersion":2}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := writeLegacyFundsSourceV0(t, realTempDir(t), domainplugin.PluginNameV1, legacyFundsPackageVersionV0)
			path := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			identity, err := InspectPackagedFundsSourceTreeV1(context.Background(), source)
			if err == nil || identity.LegacyV0 {
				t.Fatalf("present invalid companion fell back to legacy-v0: identity=%#v err=%v", identity, err)
			}
		})
	}
}

func TestLegacyFundsV0RejectsDirectoryCompanion(t *testing.T) {
	source := writeLegacyFundsSourceV0(t, realTempDir(t), domainplugin.PluginNameV1, legacyFundsPackageVersionV0)
	companion := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
	if err := os.MkdirAll(companion, 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := InspectPackagedFundsSourceTreeV1(context.Background(), source)
	if err == nil || identity.LegacyV0 {
		t.Fatalf("present directory companion fell back to legacy-v0: identity=%#v err=%v", identity, err)
	}
}

func TestLegacyFundsV0RejectsUnsupportedIdentityAndVersion(t *testing.T) {
	for name, identity := range map[string]struct{ packageID, version string }{
		"third-party identity":  {packageID: "third-party-funds", version: legacyFundsPackageVersionV0},
		"new publisher version": {packageID: domainplugin.PluginNameV1, version: "0.16.17"},
	} {
		t.Run(name, func(t *testing.T) {
			source := writeLegacyFundsSourceV0(t, realTempDir(t), identity.packageID, identity.version)
			if observed, err := InspectPackagedFundsSourceTreeV1(context.Background(), source); err == nil || observed.LegacyV0 {
				t.Fatalf("unsupported legacy-v0 artifact was admitted: identity=%#v err=%v", observed, err)
			}
		})
	}
}

func writeLegacyFundsSourceV0(t *testing.T, root, packageID, packageVersion string) string {
	t.Helper()
	source := filepath.Join(root, "legacy-funds")
	files := map[string]string{
		filepath.Join(".codex-plugin", "plugin.json"): `{"name":"` + packageID + `","version":"` + packageVersion + `","interface":{"capabilities":["Interactive","Read"]},"requestedCapabilities":["funds.admin"]}`,
		".mcp.json": `{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`,
		filepath.FromSlash(legacyFundsMCPEntrypointV0): "export const server = 'analytix_funds'\n",
		"assets/icon.png":    "legacy-icon\n",
		"assets/logo.png":    "legacy-logo\n",
		"agents/openai.yaml": "interface:\n  display_name: Analytix Funds\n",
	}
	for _, skill := range legacyFundsSkillNamesV0 {
		files["skills/"+skill+"/SKILL.md"] = "---\nname: " + skill + "\n---\n"
	}
	for relative, body := range files {
		path := filepath.Join(source, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return source
}
