package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const bundledFundsMaterializationReadyMarkerV1 = "ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 "

type bundledFundsMaterializationDependenciesV1 struct {
	inspectPackage func(context.Context) (packagedauthorityfs.InspectionV2, error)
	inspectSource  func(context.Context, string) (pluginstore.SourceTreeIdentityV1, error)
	openAuthority  func(string, bool) (pluginport.InstallationAuthority, error)
	newStore       func(string) (pluginport.Store, error)
	now            func() time.Time
}

func defaultBundledFundsMaterializationDependenciesV1() bundledFundsMaterializationDependenciesV1 {
	return bundledFundsMaterializationDependenciesV1{
		inspectPackage: packagedauthorityfs.InspectCurrentPackageV2,
		inspectSource:  pluginstore.InspectPackagedFundsSourceTreeV1,
		openAuthority: func(dataDir string, stateExists bool) (pluginport.InstallationAuthority, error) {
			return pluginauthority.OpenOrCreateV1(dataDir, stateExists)
		},
		newStore: func(runtimeHome string) (pluginport.Store, error) {
			return pluginstore.NewStore(runtimeHome, nil)
		},
		now: time.Now,
	}
}

func runBundledPluginCommandV1(
	ctx context.Context,
	args []string,
	output io.Writer,
	dependencies bundledFundsMaterializationDependenciesV1,
) error {
	if ctx == nil || ctx.Err() != nil || len(args) == 0 || args[0] != "materialize-funds-v1" || output == nil ||
		dependencies.inspectPackage == nil || dependencies.inspectSource == nil || dependencies.openAuthority == nil ||
		dependencies.newStore == nil || dependencies.now == nil {
		return errors.New("bundled plugin materialization command is invalid")
	}
	flags := flag.NewFlagSet("materialize-funds-v1", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDirFlag := flags.String("data-dir", "", "Analytix runtime data root")
	invocationID := flags.String("invocation-id", "", "current Electron invocation nonce")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !exactAbsoluteCLIPath(*dataDirFlag) ||
		!domainplugin.IsCanonicalSHA256V1(*invocationID) {
		return errors.New("bundled plugin materialization command is invalid")
	}

	// Validate the independently anchored package, current runtime image, and
	// complete packaged plugin tree before creating any installation state.
	inspection, err := dependencies.inspectPackage(ctx)
	if err != nil || inspection.Authority.Core != nil {
		return errors.New("bundled funds package authority is unavailable")
	}
	source, err := dependencies.inspectSource(ctx, inspection.PluginSourceRoot)
	if err != nil || source != inspection.PluginSourceIdentity || source.LegacyV0 {
		return errors.New("bundled funds package source is invalid")
	}
	platform, arch, ok := inspection.Authority.Target()
	if !ok {
		return errors.New("bundled funds package target is invalid")
	}
	target := domainplugin.TargetV1{Platform: platform, Arch: arch}
	admission, err := domainpluginpackage.AdmitStaticFirstPartyV1(domainpluginpackage.StaticAdmissionInputV1{
		CanonicalDeclaration:       []byte(source.Declaration.CanonicalJSON),
		DeclarationRawSHA256:       source.Declaration.RawSHA256,
		DeclarationCanonicalSHA256: source.Declaration.CanonicalSHA256,
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
	})
	if err != nil {
		return errors.New("bundled funds package static admission is denied")
	}

	dataDir, err := prepareCanonicalDataDirV1(*dataDirFlag)
	if err != nil {
		return errors.New("bundled funds installation data root is invalid")
	}
	runtimeHome := dataDir
	if filepath.Base(dataDir) == "data" {
		runtimeHome = filepath.Dir(dataDir)
	}
	if _, err := canonicalExistingDirectoryV1(runtimeHome); err != nil {
		return errors.New("bundled funds installation root is invalid")
	}
	authorityPath := pluginauthority.KeyPathV1(dataDir)
	authorityStateExists, err := installationAuthorityStateExistsV1(
		dataDir, runtimeHome, authorityPath, admission.Identity.PackageID, admission.Identity.PackageVersion,
	)
	if err != nil {
		return errors.New("bundled funds installation authority state is invalid")
	}
	authority, err := dependencies.openAuthority(dataDir, authorityStateExists)
	if err != nil {
		return errors.New("bundled funds installation authority is unavailable")
	}
	store, err := dependencies.newStore(runtimeHome)
	if err != nil {
		return errors.New("bundled funds materialization store is unavailable")
	}
	binding := pluginapp.FormalPackageBindingV1{
		AuthoritySHA256: inspection.AuthorityFileSHA256, Target: target,
		PackageIdentity:            admission.Identity,
		DeclarationRawSHA256:       admission.DeclarationRawSHA256,
		DeclarationCanonicalSHA256: admission.DeclarationCanonicalSHA256,
		SourceTreeSHA256:           source.TreeSHA256, SourceTreeFileCount: source.FileCount,
		ManifestSHA256: source.ManifestSHA256, EntrypointSHA256: source.EntrypointSHA256,
	}
	service, err := pluginapp.NewService(store, authority, binding, dependencies.now)
	if err != nil {
		return errors.New("bundled funds materialization service is unavailable")
	}

	result, resolveErr := service.ResolveActive(ctx)
	if resolveErr != nil {
		if !errors.Is(resolveErr, os.ErrNotExist) && !errors.Is(resolveErr, pluginapp.ErrPackageAuthority) {
			return errors.New("existing bundled funds materialization is invalid")
		}
		requestedAt := dependencies.now().UTC()
		intent, intentErr := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
			PackageAuthoritySHA256: inspection.AuthorityFileSHA256, Target: target,
			PluginName: admission.Identity.PackageID, PluginVersion: admission.Identity.PackageVersion,
			SourceRoot: source.RootRealPath, SourceTreeSHA256: source.TreeSHA256,
			SourceTreeFileCount: source.FileCount, ManifestSHA256: source.ManifestSHA256,
			EntrypointSHA256: source.EntrypointSHA256, RequestedAt: requestedAt,
		})
		if intentErr != nil {
			return errors.New("bundled funds materialization intent is invalid")
		}
		result, err = service.Materialize(ctx, intent)
		if err != nil {
			return errors.New("bundled funds materialization failed")
		}
	}
	if err := domainplugin.ValidateTrustedReceiptV1(result.Receipt, authority.KeyID(), authority.PublicKey()); err != nil ||
		domainplugin.ValidateIndexForReceiptV1(result.Index, result.Receipt) != nil {
		return errors.New("bundled funds materialization receipt is invalid")
	}
	activeRoot := filepath.Join(runtimeHome, filepath.FromSlash(result.Index.ActiveRelativePath))
	activeRoot, err = canonicalExistingDirectoryV1(activeRoot)
	if err != nil {
		return errors.New("bundled funds active generation is invalid")
	}
	completedAt := dependencies.now().UTC()
	ready, err := domainplugin.NewReadyV1(domainplugin.ReadyInputV1{
		InvocationID: *invocationID,
		PackageAuthority: domainplugin.PackageAuthorityBindingV1{
			FileSHA256:      inspection.AuthorityFileSHA256,
			AuthorityDigest: inspection.Authority.Authority.AuthorityDigest,
			Classification:  inspection.Authority.Authority.Classification,
			DispositionKind: inspection.Authority.DispositionKind(), PlatformAnchor: inspection.PackageAnchor,
		},
		RuntimeIdentity: domainplugin.RuntimeIdentityV1{
			PayloadSHA256: inspection.RuntimeIdentity.PayloadSHA256,
			PayloadBytes:  inspection.RuntimeIdentity.PayloadBytes,
			Format:        inspection.RuntimeIdentity.Format, Arch: inspection.RuntimeIdentity.Arch,
		},
		ActivePluginRoot: activeRoot, Receipt: result.Receipt, Index: result.Index, CompletedAt: completedAt,
	})
	if err != nil {
		return errors.New("bundled funds current-run binding is invalid")
	}
	body, err := domainplugin.ReadyV1Bytes(ready)
	if err != nil {
		return errors.New("bundled funds current-run binding cannot be encoded")
	}
	if _, err := fmt.Fprintf(output, "%s%s\n", bundledFundsMaterializationReadyMarkerV1, body); err != nil {
		return errors.New("bundled funds current-run binding cannot be written")
	}
	return nil
}

func prepareCanonicalDataDirV1(path string) (string, error) {
	if !exactAbsoluteCLIPath(path) {
		return "", errors.New("data root path is invalid")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	return canonicalExistingDirectoryV1(path)
}

func canonicalExistingDirectoryV1(path string) (string, error) {
	if !exactAbsoluteCLIPath(path) {
		return "", errors.New("directory path is invalid")
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return "", errors.New("directory path contains a symbolic link")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("directory path is not a real directory")
	}
	return real, nil
}

func installationAuthorityStateExistsV1(_ string, runtimeHome, authorityPath, packageID, packageVersion string) (bool, error) {
	if info, err := os.Lstat(authorityPath); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return false, errors.New("installation authority path is unsafe")
		}
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	for _, relative := range []string{
		".state/bundled-plugin-materialization/v1",
		"plugins/cache/analytix-hub/" + packageID + "/" + packageVersion,
	} {
		if _, err := os.Lstat(filepath.Join(runtimeHome, filepath.FromSlash(relative))); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
