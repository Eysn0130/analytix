package pluginmaterializationfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const (
	legacyFundsPackageVersionV0 = "0.16.16"
	legacyFundsMCPEntrypointV0  = "mcp/server.mjs"
)

var legacyFundsSkillNamesV0 = [...]string{
	"analytix-fund-analysis",
	"index",
	"case-context",
	"data-quality",
	"quick-fact",
	"pair-amount-investigation",
	"account-dossier",
	"subject-dossier",
	"counterparty-analysis",
	"fund-tracing",
	"investigation-lab",
	"full-case-analysis",
	"report-builder",
	"evidence-request",
	"analysis-critique",
	"claim-review",
	"delivery-qc",
	"graph-visualization",
	"visual-evidence",
	"case-workbench",
}

var legacyFundsMCPServersV0 = []domainpluginpackage.MCPServerContributionV1{
	{ID: "analytix_funds", Entrypoint: legacyFundsMCPEntrypointV0},
}

func inspectLegacyFundsV0Projections(
	root string,
	recordPaths map[string]struct{},
	expectedManifestSHA256 string,
	expectedMCPConfigSHA256 string,
) (string, string, error) {
	name, version, err := inspectManifestIdentityV1(root, expectedManifestSHA256)
	if err != nil || name != domainplugin.PluginNameV1 || version != legacyFundsPackageVersionV0 {
		return "", "", errors.New("bundled plugin legacy-v0 manifest is unsupported")
	}
	if err := validateDisabledMCPV1(root, expectedMCPConfigSHA256, legacyFundsMCPServersV0); err != nil {
		return "", "", err
	}
	required := map[string]struct{}{
		"assets/icon.png":    {},
		"assets/logo.png":    {},
		"agents/openai.yaml": {},
	}
	for _, server := range legacyFundsMCPServersV0 {
		required[server.Entrypoint] = struct{}{}
	}
	knownSkills := make(map[string]struct{}, len(legacyFundsSkillNamesV0))
	for _, skill := range legacyFundsSkillNamesV0 {
		knownSkills[skill] = struct{}{}
		required["skills/"+skill+"/SKILL.md"] = struct{}{}
	}
	for path := range required {
		if _, ok := recordPaths[path]; !ok {
			return "", "", errors.New("bundled plugin legacy-v0 projection is incomplete")
		}
	}
	for path := range recordPaths {
		if !strings.HasPrefix(path, "skills/") || !strings.HasSuffix(path, "/SKILL.md") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) != 3 {
			continue
		}
		if _, ok := knownSkills[parts[1]]; !ok {
			return "", "", errors.New("bundled plugin legacy-v0 skill projection is unsupported")
		}
	}
	return name, version, nil
}

// MigrateVerifiedLegacyFundsV0ToDeclarationV1 derives one in-memory schema-v1
// declaration from an exact historical Funds artifact. It neither writes the
// package nor treats the legacy manifest, marker, or package presence as a
// grant. The caller must still run the returned declaration through Host
// static admission.
func MigrateVerifiedLegacyFundsV0ToDeclarationV1(
	ctx context.Context,
	root string,
	expected SourceTreeIdentityV1,
	result pluginport.ResultV1,
	trustedKeyID string,
	trustedPublicKey []byte,
) (PackageDeclarationIdentityV1, error) {
	if ctx == nil || ctx.Err() != nil || !expected.LegacyV0 ||
		expected.LegacyPackageID != domainplugin.PluginNameV1 ||
		expected.LegacyVersion != legacyFundsPackageVersionV0 ||
		domainplugin.ValidateTrustedReceiptV1(result.Receipt, trustedKeyID, trustedPublicKey) != nil ||
		domainplugin.ValidateIndexForReceiptV1(result.Index, result.Receipt) != nil {
		return PackageDeclarationIdentityV1{}, errors.New("bundled plugin legacy-v0 authority is invalid")
	}
	observed, err := InspectPackagedFundsSourceTreeV1(ctx, root)
	if err != nil || observed != expected ||
		result.Receipt.PluginName != observed.LegacyPackageID ||
		result.Receipt.PluginVersion != observed.LegacyVersion ||
		result.Receipt.SourceTreeSHA256 != observed.TreeSHA256 ||
		result.Receipt.SourceTreeFileCount != observed.FileCount ||
		result.Receipt.ManifestSHA256 != observed.ManifestSHA256 ||
		result.Receipt.EntrypointSHA256 != observed.EntrypointSHA256 {
		return PackageDeclarationIdentityV1{}, errors.New("bundled plugin legacy-v0 artifact does not match its signed receipt")
	}
	declaration := domainpluginpackage.DeclarationV1{
		SchemaVersion:  domainpluginpackage.SchemaVersionV1,
		PackageID:      result.Receipt.PluginName,
		PackageVersion: result.Receipt.PluginVersion,
		Contributions: domainpluginpackage.ContributionsV1{
			MCPServers: append([]domainpluginpackage.MCPServerContributionV1(nil), legacyFundsMCPServersV0...),
			Hooks:      []domainpluginpackage.PathContributionV1{},
			Assets: []domainpluginpackage.PathContributionV1{
				{ID: "funds-icon", Path: "assets/icon.png"},
				{ID: "funds-logo", Path: "assets/logo.png"},
			},
			PublicUI: []domainpluginpackage.PathContributionV1{
				{ID: "openai-agent-ui", Path: "agents/openai.yaml"},
			},
		},
		RequestedCapabilities: []domainpluginpackage.CapabilityRequestV1{
			{ID: "funds.case.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
			{ID: "funds.source.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
		},
		Lifecycle: domainpluginpackage.LifecycleV1{
			ProtocolVersion: domainpluginpackage.SupportedLifecycleProtocolV1,
			EntryPolicy:     domainpluginpackage.StaticFirstPartyEntryPolicyV1,
		},
	}
	for _, skill := range legacyFundsSkillNamesV0 {
		declaration.Contributions.Skills = append(declaration.Contributions.Skills, domainpluginpackage.PathContributionV1{
			ID: skill, Path: "skills/" + skill + "/SKILL.md",
		})
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		return PackageDeclarationIdentityV1{}, errors.New("bundled plugin legacy-v0 declaration migration failed")
	}
	digest := sha256.Sum256(canonical)
	sha256Hex := hex.EncodeToString(digest[:])
	return PackageDeclarationIdentityV1{
		RawSHA256: sha256Hex, CanonicalSHA256: sha256Hex, CanonicalJSON: string(canonical),
		PackageID: declaration.PackageID, PackageVersion: declaration.PackageVersion,
	}, nil
}
