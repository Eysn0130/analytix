package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcpidentity "analytix.local/runtime-go/internal/adapters/outbound/mcp/identity"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	mcp "analytix.local/runtime-go/internal/mcp"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

var errBundledFundsHostUnavailableV1 = errors.New("bundled funds host capability is unavailable")

type bundledFundsHostValidationDependenciesV1 struct {
	resolveRuntimeRoots    func(string) (string, string, error)
	inspectPackage         func(context.Context) (packagedauthorityfs.InspectionV2, error)
	inspectSource          func(context.Context, string) (pluginstore.SourceTreeIdentityV1, error)
	openAuthority          func(string, bool) (pluginport.InstallationAuthority, error)
	openStore              func(string) (pluginport.Store, error)
	newService             func(pluginport.Store, pluginport.InstallationAuthority, pluginapp.FormalPackageBindingV1, func() time.Time) (*pluginapp.Service, error)
	newHostSpec            func(mcp.ServerSpec, domainpluginpackage.StaticAdmissionInputV1) (*mcp.HostFundsServerSpecV1, error)
	observeStaticAdmission func(domainpluginpackage.StaticAdmissionInputV1, error)
	observeHostSourceRead  func(func() []domainplugincapability.FundsSourceReadDecisionEventV1)
	// Test-only assembly input; production leaves this nil. A composition test
	// can open its explicitly selected native generation through the normal
	// Host admission without making the test executable a packaged runtime.
	openNativeOwnerForTest func(string) (*nativecomponenthost.Owner, error)
}

// This private assembly key lets composition tests retain the real validator
// and static admission owner while supplying isolated package artifacts.
type bundledFundsHostValidationContextKeyV1 struct{}

func validateBundledFundsHostMaterializationV1(
	ctx context.Context,
	config Config,
) (*mcp.HostFundsServerSpecV1, error) {
	return validateBundledFundsHostMaterializationWithDependenciesV1(
		ctx,
		config,
		defaultBundledFundsHostValidationDependenciesV1(),
	)
}

func defaultBundledFundsHostValidationDependenciesV1() bundledFundsHostValidationDependenciesV1 {
	return bundledFundsHostValidationDependenciesV1{
		inspectPackage: packagedauthorityfs.InspectCurrentPackageV2,
		inspectSource:  pluginstore.InspectPackagedFundsSourceTreeV1,
		openAuthority: func(dataDir string, stateExists bool) (pluginport.InstallationAuthority, error) {
			return pluginauthority.OpenOrCreateV1(dataDir, stateExists)
		},
		openStore: func(runtimeHome string) (pluginport.Store, error) {
			return pluginstore.OpenExistingStoreV1(runtimeHome)
		},
		newService:  pluginapp.NewService,
		newHostSpec: mcp.NewHostFundsServerSpecV1,
	}
}

// admitBundledFundsHostForStartupV1 keeps package or installed-generation
// failures local to the additive funds capability. Cancellation and unexpected
// host inspection failures remain startup errors.
func admitBundledFundsHostForStartupV1(
	ctx context.Context,
	config Config,
) (*mcp.HostFundsServerSpecV1, error) {
	dependencies := defaultBundledFundsHostValidationDependenciesV1()
	if ctx != nil {
		if supplied, ok := ctx.Value(bundledFundsHostValidationContextKeyV1{}).(bundledFundsHostValidationDependenciesV1); ok {
			dependencies = supplied
		}
	}
	return admitBundledFundsHostForStartupWithDependenciesV1(ctx, config, dependencies)
}

func admitBundledFundsHostForStartupWithDependenciesV1(
	ctx context.Context,
	config Config,
	dependencies bundledFundsHostValidationDependenciesV1,
) (*mcp.HostFundsServerSpecV1, error) {
	spec, err := validateBundledFundsHostMaterializationWithDependenciesV1(ctx, config, dependencies)
	if errors.Is(err, errBundledFundsHostUnavailableV1) {
		return nil, nil
	}
	return spec, err
}

func validateBundledFundsHostMaterializationWithDependenciesV1(
	ctx context.Context,
	config Config,
	dependencies bundledFundsHostValidationDependenciesV1,
) (*mcp.HostFundsServerSpecV1, error) {
	if ctx == nil {
		return nil, errors.New("bundled funds host validation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dependencies.inspectPackage == nil || dependencies.inspectSource == nil ||
		dependencies.openAuthority == nil || dependencies.openStore == nil ||
		dependencies.newService == nil || dependencies.newHostSpec == nil {
		return nil, errors.New("bundled funds host materialization validator is unavailable")
	}
	inspection, err := dependencies.inspectPackage(ctx)
	if err != nil {
		if bundledFundsHostContextErrorV1(ctx, err) {
			return nil, bundledFundsHostContextCauseV1(ctx, err)
		}
		if err == packagedauthorityfs.ErrNotPackagedRuntimeV2 || errors.Is(err, packagedauthorityfs.ErrPackagedFundsPluginRootUnavailable) {
			return nil, errBundledFundsHostUnavailableV1
		}
		return nil, errors.New("bundled funds current package inspection failed")
	}
	if inspection.Authority.Core != nil || inspection.Publishable || inspection.FactToolsEnabled || inspection.PackageAnchor == "" {
		return nil, errBundledFundsHostUnavailableV1
	}
	source, err := dependencies.inspectSource(ctx, inspection.PluginSourceRoot)
	if err != nil {
		if bundledFundsHostContextErrorV1(ctx, err) {
			return nil, bundledFundsHostContextCauseV1(ctx, err)
		}
		return nil, errBundledFundsHostUnavailableV1
	}
	if source != inspection.PluginSourceIdentity {
		return nil, errBundledFundsHostUnavailableV1
	}
	platform, arch, ok := inspection.Authority.Target()
	if !ok {
		return nil, errBundledFundsHostUnavailableV1
	}
	target := domainplugin.TargetV1{Platform: platform, Arch: arch}
	resolveRoots := bundledFundsRuntimeRootsV1
	if dependencies.resolveRuntimeRoots != nil {
		resolveRoots = dependencies.resolveRuntimeRoots
	}
	dataDir, runtimeHome, err := resolveRoots(config.DataDir)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	authority, err := dependencies.openAuthority(dataDir, true)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	store, err := dependencies.openStore(runtimeHome)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	binding := pluginapp.FormalPackageBindingV1{
		AuthoritySHA256:  inspection.AuthorityFileSHA256,
		Target:           target,
		SourceTreeSHA256: source.TreeSHA256, SourceTreeFileCount: source.FileCount,
		ManifestSHA256: source.ManifestSHA256, EntrypointSHA256: source.EntrypointSHA256,
	}
	if !source.LegacyV0 {
		binding.PackageIdentity = domainpluginpackage.PackageIdentityV1{
			PackageID: source.Declaration.PackageID, PackageVersion: source.Declaration.PackageVersion,
		}
		binding.DeclarationRawSHA256 = source.Declaration.RawSHA256
		binding.DeclarationCanonicalSHA256 = source.Declaration.CanonicalSHA256
	}
	service, err := dependencies.newService(store, authority, binding, time.Now)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	result, err := service.ResolveActive(ctx)
	if err != nil {
		if bundledFundsHostContextErrorV1(ctx, err) {
			return nil, bundledFundsHostContextCauseV1(ctx, err)
		}
		return nil, errBundledFundsHostUnavailableV1
	}
	if domainplugin.ValidateTrustedReceiptV1(
		result.Receipt,
		authority.KeyID(),
		authority.PublicKey(),
	) != nil || domainplugin.ValidateIndexForReceiptV1(result.Index, result.Receipt) != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	admittedDeclaration := source.Declaration
	if source.LegacyV0 {
		migrated, migrationErr := pluginstore.MigrateVerifiedLegacyFundsV0ToDeclarationV1(
			ctx,
			source.RootRealPath,
			source,
			result,
			authority.KeyID(),
			authority.PublicKey(),
		)
		if migrationErr != nil {
			return nil, errBundledFundsHostUnavailableV1
		}
		admittedDeclaration = migrated
	}
	admissionInput := domainpluginpackage.StaticAdmissionInputV1{
		CanonicalDeclaration:       []byte(admittedDeclaration.CanonicalJSON),
		DeclarationRawSHA256:       admittedDeclaration.RawSHA256,
		DeclarationCanonicalSHA256: admittedDeclaration.CanonicalSHA256,
		Evidence: domainpluginpackage.StaticAdmissionEvidenceV1{
			ArtifactIntegrityVerified: true,
			PackageAuthoritySHA256:    inspection.AuthorityFileSHA256,
			ProvenanceAuthorityDigest: inspection.Authority.Authority.AuthorityDigest,
			ProvenanceClassification:  inspection.Authority.Authority.Classification,
			ProvenanceDispositionKind: inspection.Authority.DispositionKind(),
			PlatformAnchor:            inspection.PackageAnchor,
			Publishable:               inspection.Publishable,
			FactToolsEnabled:          inspection.FactToolsEnabled,
			SigningAlgorithm:          domainpluginpackage.StaticAdmissionSigningAlgorithmV1,
		},
	}
	admission, admissionErr := domainpluginpackage.AdmitStaticFirstPartyV1(admissionInput)
	if dependencies.observeStaticAdmission != nil {
		dependencies.observeStaticAdmission(admissionInput, admissionErr)
	}
	if admissionErr != nil || admission.Identity.PackageID != result.Receipt.PluginName ||
		admission.Identity.PackageVersion != result.Receipt.PluginVersion {
		return nil, errBundledFundsHostUnavailableV1
	}
	entrypointRelativePath, err := pluginstore.FundsMCPEntrypointFromCanonicalDeclarationV1(admittedDeclaration)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	expectedActiveRoot := filepath.Join(runtimeHome, filepath.FromSlash(result.Index.ActiveRelativePath))
	activeRoot, err := canonicalBundledFundsDirectoryV1(expectedActiveRoot)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	installMarker, err := pluginstore.InspectInstallMarkerV1(activeRoot, result.Receipt)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	entrypoint := filepath.Join(activeRoot, filepath.FromSlash(entrypointRelativePath))
	spec := mcp.ServerSpec{
		ID: "analytix_funds", Transport: "stdio",
		Command: inspection.ApplicationRunnerPath, Args: []string{entrypoint},
		Env: map[string]string{"ELECTRON_RUN_AS_NODE": "1"}, CWD: activeRoot,
		ExpectedServerName: "analytix_funds", ExpectedServerVersion: result.Receipt.PluginVersion,
		IdentitySource: mcpidentity.HostInstalledGenerationSourceV1,
		ManifestSHA256: result.Receipt.ManifestSHA256,
		EntrypointPath: entrypoint, EntrypointSHA256: result.Receipt.EntrypointSHA256,
		PluginRootPath: activeRoot, SourceTreeSHA256: result.Receipt.SourceTreeSHA256,
		HostInstallMarkerSHA256: installMarker.SHA256,
		TrustScope:              "user", TimeoutMS: 120_000,
		ReadOnlyToolNames: map[string]bool{
			"count_case_rows":       true,
			"analyze_account_flows": true,
		},
	}
	hostSpec, err := dependencies.newHostSpec(spec, admissionInput)
	if err != nil {
		return nil, errBundledFundsHostUnavailableV1
	}
	return hostSpec, nil
}

func bundledFundsHostContextErrorV1(ctx context.Context, err error) bool {
	return (ctx != nil && ctx.Err() != nil) || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func bundledFundsHostContextCauseV1(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return context.Canceled
}

func bundledFundsRuntimeRootsV1(dataDir string) (string, string, error) {
	dataDir, err := canonicalBundledFundsDirectoryV1(dataDir)
	if err != nil {
		return "", "", err
	}
	runtimeHome := dataDir
	if filepath.Base(dataDir) == "data" {
		runtimeHome, err = canonicalBundledFundsDirectoryV1(filepath.Dir(dataDir))
		if err != nil {
			return "", "", err
		}
	}
	return dataDir, runtimeHome, nil
}

func canonicalBundledFundsDirectoryV1(path string) (string, error) {
	if path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) ||
		filepath.Clean(path) != path {
		return "", errors.New("directory path is invalid")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return "", errors.New("directory path contains a symbolic link")
	}
	info, err := os.Lstat(real)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("directory path is not a real directory")
	}
	return real, nil
}
