package packagedbuildauthorityfs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	pluginmaterializationfs "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

// Set only by the canonical package builder; never read from settings or env.
var embeddedReleaseProfile = "full"

func CoreOnlyBuild() bool { return embeddedReleaseProfile == "core" }

const (
	authorityRelativePathV2 = "runtime/analytix-packaged-build-authority.json"
	pluginRelativePathV2    = "plugins/analytix-fund-analysis"
	maxRuntimeBinaryBytesV2 = 512 << 20
	maxAppASARBytesV2       = 2 << 30
)

// ErrPackagedFundsPluginRootUnavailable identifies the one package inspection
// failure that disables only the additive bundled funds capability. All
// application runner, app.asar, runtime-server, authority, target, and package
// anchor failures remain package-wide startup failures.
var ErrPackagedFundsPluginRootUnavailable = errors.New("packaged funds plugin root is unavailable")

// ErrNotPackagedRuntimeV2 is issued only after resolving a canonical, regular
// executable whose location is outside the packaged runtime layout. It does
// not classify missing or invalid material within an existing package.
var ErrNotPackagedRuntimeV2 = errors.New("current executable is not a packaged runtime")

type RuntimeIdentityV2 struct {
	PayloadSHA256 string
	PayloadBytes  int64
	Format        string
	Arch          string
}

type ContentIdentityV2 struct {
	SHA256     string
	ByteLength int64
}

type InspectionV2 struct {
	ExecutablePath            string
	ApplicationRunnerPath     string
	ApplicationRunnerIdentity RuntimeIdentityV2
	AppASARPath               string
	AppASARIdentity           ContentIdentityV2
	ResourcesRoot             string
	PluginSourceRoot          string
	PluginSourceIdentity      pluginmaterializationfs.SourceTreeIdentityV1
	AuthorityPath             string
	AuthorityFileSHA256       string
	Authority                 domainauthority.ParsedAuthorityV2
	RuntimeIdentity           RuntimeIdentityV2
	PackageAnchor             string
	Publishable               bool
	FactToolsEnabled          bool
}

type packageArtifactPathsV2 struct {
	authority         string
	applicationRunner string
	appASAR           string
	runtimeServer     string
	pluginRoot        string
}

type packageArtifactWitnessV2 struct {
	authorityBody             []byte
	authorityFile             os.FileInfo
	applicationRunnerFile     os.FileInfo
	applicationRunnerIdentity RuntimeIdentityV2
	appASARFile               os.FileInfo
	appASARIdentity           ContentIdentityV2
	runtimeServerFile         os.FileInfo
	runtimeServerIdentity     RuntimeIdentityV2
	pluginSourceIdentity      pluginmaterializationfs.SourceTreeIdentityV1
}

func InspectCurrentPackageV2(ctx context.Context) (InspectionV2, error) {
	executable, err := os.Executable()
	if err != nil {
		return InspectionV2{}, errors.New("current runtime-server executable identity is unavailable")
	}
	return InspectPackageV2(ctx, executable, runtime.GOOS, runtime.GOARCH)
}

func InspectPackageV2(ctx context.Context, executablePath, goos, goarch string) (InspectionV2, error) {
	if ctx == nil {
		return InspectionV2{}, errors.New("packaged authority inspection was canceled")
	}
	if err := ctx.Err(); err != nil {
		return InspectionV2{}, err
	}
	executable, resources, err := resolveLayoutV2(executablePath, goos)
	if err != nil {
		return InspectionV2{}, err
	}
	authorityPath := filepath.Join(resources, filepath.FromSlash(authorityRelativePathV2))
	authoritySnapshot, err := stableReadSnapshotFileV2(authorityPath, domainauthority.MaxAuthorityBytesV2, false)
	if err != nil {
		return InspectionV2{}, errors.New("current packaged build authority is unavailable")
	}
	authorityBody := authoritySnapshot.body
	parsed, err := domainauthority.ParseV2(authorityBody)
	if err != nil {
		return InspectionV2{}, err
	}
	if (parsed.Core != nil) != CoreOnlyBuild() {
		return InspectionV2{}, errors.New("packaged release profile does not match runtime")
	}
	platform, arch, ok := parsed.Target()
	if !ok || platform != normalizedPlatformV2(goos) || arch != normalizedArchV2(goarch) {
		return InspectionV2{}, errors.New("packaged build authority target does not match the current runtime")
	}
	if !nativeArtifactTargetsMatchV2(parsed.Authority.Artifacts, goos, goarch) {
		return InspectionV2{}, errors.New("packaged native artifact identities do not match the authority target")
	}
	applicationRunner, appASAR, err := resolveApplicationArtifactsV2(resources, goos)
	if err != nil {
		return InspectionV2{}, err
	}
	applicationSnapshot, err := stableReadSnapshotFileV2(applicationRunner, maxRuntimeBinaryBytesV2, false)
	if err != nil {
		return InspectionV2{}, errors.New("current Electron application runner cannot be read through a stable identity")
	}
	applicationIdentity, err := inspectNativePayloadV2(applicationSnapshot.body)
	clear(applicationSnapshot.body)
	if err != nil || !nativeBindingMatchesV2(applicationIdentity, parsed.Authority.Artifacts.Executable) {
		return InspectionV2{}, errors.New("current Electron application runner does not match the packaged build authority")
	}
	appASARSnapshot, err := stableContentSnapshotV2(appASAR, maxAppASARBytesV2)
	appASARIdentity := appASARSnapshot.identity
	if err != nil || appASARIdentity.SHA256 != parsed.Authority.Artifacts.AppASAR.SHA256 ||
		appASARIdentity.ByteLength != parsed.Authority.Artifacts.AppASAR.ByteLength {
		return InspectionV2{}, errors.New("current app.asar does not match the packaged build authority")
	}
	executableSnapshot, err := stableReadSnapshotFileV2(executable, maxRuntimeBinaryBytesV2, false)
	if err != nil {
		return InspectionV2{}, errors.New("current runtime-server cannot be read through a stable identity")
	}
	identity, err := inspectNativePayloadV2(executableSnapshot.body)
	clear(executableSnapshot.body)
	if err != nil || !nativeBindingMatchesV2(identity, parsed.Authority.Artifacts.RuntimeServer) {
		return InspectionV2{}, errors.New("current runtime-server does not match the packaged build authority")
	}
	anchor, err := verifyPackageAnchorV2(ctx, resources, parsed)
	if err != nil {
		return InspectionV2{}, err
	}
	// A missing or malformed additive plugin is classified only after the
	// package-wide resource seal has succeeded. Its exact tree identity is then
	// checked against the sealed authority before and after the return barrier.
	pluginRoot := filepath.Join(resources, filepath.FromSlash(pluginRelativePathV2))
	var pluginIdentity pluginmaterializationfs.SourceTreeIdentityV1
	if parsed.Core != nil {
		if err := verifyCoreResourcesAbsentV2(resources); err != nil {
			return InspectionV2{}, err
		}
	} else {
		if _, err := canonicalDirectoryV2(pluginRoot); err != nil {
			return InspectionV2{}, ErrPackagedFundsPluginRootUnavailable
		}
		pluginIdentity, err = pluginmaterializationfs.InspectPackagedFundsSourceTreeV1(ctx, pluginRoot)
		if err := classifyFundsPluginInspectionV2(
			ctx,
			err,
			fundsPluginBindingMatchesV2(pluginIdentity, parsed.Authority.Artifacts.FundsPlugin),
		); err != nil {
			return InspectionV2{}, err
		}

	}
	if err := revalidatePackageArtifactsV2(
		ctx,
		packageArtifactPathsV2{
			authority: authorityPath, applicationRunner: applicationRunner,
			appASAR: appASAR, runtimeServer: executable,
			pluginRoot: pluginRoot,
		},
		packageArtifactWitnessV2{
			authorityBody: authorityBody, authorityFile: authoritySnapshot.info,
			applicationRunnerFile: applicationSnapshot.info, applicationRunnerIdentity: applicationIdentity,
			appASARFile: appASARSnapshot.info, appASARIdentity: appASARIdentity,
			runtimeServerFile: executableSnapshot.info, runtimeServerIdentity: identity,
			pluginSourceIdentity: pluginIdentity,
		},
		parsed,
	); err != nil {
		return InspectionV2{}, err
	}
	secondAnchor, err := verifyPackageAnchorV2(ctx, resources, parsed)
	if err != nil || secondAnchor != anchor {
		return InspectionV2{}, errors.New("packaged resource seal changed after artifact revalidation")
	}
	return InspectionV2{
		ExecutablePath: executable, ApplicationRunnerPath: applicationRunner,
		ApplicationRunnerIdentity: applicationIdentity, AppASARPath: appASAR,
		AppASARIdentity: appASARIdentity, ResourcesRoot: resources, PluginSourceRoot: pluginRoot,
		PluginSourceIdentity: pluginIdentity,
		AuthorityPath:        authorityPath, AuthorityFileSHA256: domainauthority.AuthorityFileSHA256V2(authorityBody),
		Authority: parsed, RuntimeIdentity: identity, PackageAnchor: anchor,
		Publishable: false, FactToolsEnabled: false,
	}, nil
}

func revalidatePackageArtifactsV2(
	ctx context.Context,
	paths packageArtifactPathsV2,
	first packageArtifactWitnessV2,
	authority domainauthority.ParsedAuthorityV2,
) error {
	if ctx == nil {
		return errors.New("packaged artifact post-anchor validation was canceled")
	}
	if ctx.Err() != nil {
		return errors.Join(errors.New("packaged artifact post-anchor validation was canceled"), ctx.Err())
	}
	secondAuthority, err := stableReadSnapshotFileV2(paths.authority, domainauthority.MaxAuthorityBytesV2, false)
	if err != nil || !os.SameFile(first.authorityFile, secondAuthority.info) ||
		!bytes.Equal(first.authorityBody, secondAuthority.body) {
		clear(secondAuthority.body)
		return errors.New("packaged build authority changed after anchor verification")
	}
	clear(secondAuthority.body)

	secondRunner, err := stableReadSnapshotFileV2(paths.applicationRunner, maxRuntimeBinaryBytesV2, false)
	if err != nil {
		return errors.New("Electron application runner changed after anchor verification")
	}
	secondRunnerIdentity, identityErr := inspectNativePayloadV2(secondRunner.body)
	clear(secondRunner.body)
	if identityErr != nil || !os.SameFile(first.applicationRunnerFile, secondRunner.info) ||
		secondRunnerIdentity != first.applicationRunnerIdentity ||
		!nativeBindingMatchesV2(secondRunnerIdentity, authority.Authority.Artifacts.Executable) {
		return errors.New("Electron application runner changed after anchor verification")
	}

	secondAppASAR, err := stableContentSnapshotV2(paths.appASAR, maxAppASARBytesV2)
	if err != nil || !os.SameFile(first.appASARFile, secondAppASAR.info) ||
		secondAppASAR.identity != first.appASARIdentity ||
		secondAppASAR.identity.SHA256 != authority.Authority.Artifacts.AppASAR.SHA256 ||
		secondAppASAR.identity.ByteLength != authority.Authority.Artifacts.AppASAR.ByteLength {
		return errors.New("app.asar changed after anchor verification")
	}

	secondRuntime, err := stableReadSnapshotFileV2(paths.runtimeServer, maxRuntimeBinaryBytesV2, false)
	if err != nil {
		return errors.New("runtime-server changed after anchor verification")
	}
	secondRuntimeIdentity, identityErr := inspectNativePayloadV2(secondRuntime.body)
	clear(secondRuntime.body)
	if identityErr != nil || !os.SameFile(first.runtimeServerFile, secondRuntime.info) ||
		secondRuntimeIdentity != first.runtimeServerIdentity ||
		!nativeBindingMatchesV2(secondRuntimeIdentity, authority.Authority.Artifacts.RuntimeServer) {
		return errors.New("runtime-server changed after anchor verification")
	}

	if authority.Core != nil {
		return verifyCoreResourcesAbsentV2(filepath.Dir(filepath.Dir(paths.pluginRoot)))
	}
	secondPlugin, err := pluginmaterializationfs.InspectPackagedFundsSourceTreeV1(ctx, paths.pluginRoot)
	classificationErr := classifyFundsPluginInspectionV2(
		ctx,
		err,
		secondPlugin == first.pluginSourceIdentity &&
			fundsPluginBindingMatchesV2(secondPlugin, authority.Authority.Artifacts.FundsPlugin),
	)
	if classificationErr != nil && !errors.Is(classificationErr, ErrPackagedFundsPluginRootUnavailable) {
		return errors.Join(errors.New("packaged artifact post-anchor validation was canceled"), classificationErr)
	}
	if classificationErr != nil {
		return errors.Join(
			ErrPackagedFundsPluginRootUnavailable,
			errors.New("funds plugin source changed after anchor verification"),
		)
	}
	if ctx.Err() != nil {
		return errors.Join(errors.New("packaged artifact post-anchor validation was canceled"), ctx.Err())
	}
	return nil
}

func classifyFundsPluginInspectionV2(ctx context.Context, inspectionErr error, bindingMatches bool) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(inspectionErr, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(inspectionErr, context.Canceled) {
		return context.Canceled
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if inspectionErr != nil || !bindingMatches {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrPackagedFundsPluginRootUnavailable
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func fundsPluginBindingMatchesV2(
	identity pluginmaterializationfs.SourceTreeIdentityV1,
	binding domainauthority.FundsPluginArtifactBindingV2,
) bool {
	return identity.TreeSHA256 == binding.TreeSHA256 &&
		identity.FileCount == uint64(binding.FileCount) &&
		identity.ManifestSHA256 == binding.ManifestSHA256 &&
		identity.EntrypointSHA256 == binding.EntrypointSHA256 &&
		identity.TotalBytes == binding.TotalBytes
}

func nativeBindingMatchesV2(identity RuntimeIdentityV2, binding domainauthority.NativeArtifactBindingV2) bool {
	return identity.PayloadSHA256 == binding.PayloadSHA256 &&
		identity.PayloadBytes == binding.PayloadByteLength &&
		identity.Format == binding.Format && identity.Arch == binding.Arch
}

func nativeArtifactTargetsMatchV2(
	artifacts domainauthority.ArtifactBindingsV2,
	goos string,
	goarch string,
) bool {
	format := ""
	switch goos {
	case "darwin":
		format = "mach-o"
	case "windows":
		format = "pe"
	case "linux":
		format = "elf"
	}
	arch := ""
	switch goarch {
	case "arm64":
		arch = "arm64"
	case "amd64":
		arch = "x64"
	}
	return format != "" && arch != "" &&
		artifacts.Executable.Format == format && artifacts.Executable.Arch == arch &&
		artifacts.RuntimeServer.Format == format && artifacts.RuntimeServer.Arch == arch
}

func resolveApplicationArtifactsV2(resources, goos string) (string, string, error) {
	var runner string
	switch goos {
	case "darwin":
		runner = filepath.Join(filepath.Dir(resources), "MacOS", "analytix")
	case "windows":
		runner = filepath.Join(filepath.Dir(resources), "analytix.exe")
	default:
		return "", "", errors.New("packaged application artifact inventory is unsupported")
	}
	appASAR := filepath.Join(resources, "app.asar")
	for _, path := range []string{runner, appASAR} {
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return "", "", errors.New("packaged application artifact path is not canonical")
		}
	}
	return runner, appASAR, nil
}

func resolveLayoutV2(executablePath, goos string) (string, string, error) {
	if executablePath == "" || executablePath != strings.TrimSpace(executablePath) || !filepath.IsAbs(executablePath) ||
		filepath.Clean(executablePath) != executablePath {
		return "", "", errors.New("current runtime-server path is not canonical")
	}
	realExecutable, err := filepath.EvalSymlinks(executablePath)
	if err != nil || realExecutable != executablePath {
		return "", "", errors.New("current runtime-server path contains a symbolic link")
	}
	info, err := os.Lstat(executablePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 {
		return "", "", errors.New("current runtime-server is not a regular file")
	}
	if filepath.Base(filepath.Dir(executablePath)) != "bin" || filepath.Base(filepath.Dir(filepath.Dir(executablePath))) != "runtime-go" {
		return "", "", ErrNotPackagedRuntimeV2
	}
	expectedRuntimeName := "runtime-server"
	if goos == "windows" {
		expectedRuntimeName += ".exe"
	}
	if filepath.Base(executablePath) != expectedRuntimeName {
		return "", "", errors.New("current runtime-server has an unexpected packaged filename")
	}
	resources := filepath.Dir(filepath.Dir(filepath.Dir(executablePath)))
	if _, err := canonicalDirectoryV2(resources); err != nil {
		return "", "", errors.New("packaged resources root is not canonical")
	}
	if goos == "darwin" {
		contents := filepath.Dir(resources)
		bundle := filepath.Dir(contents)
		if filepath.Base(resources) != "Resources" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Base(bundle), ".app") {
			return "", "", errors.New("current runtime-server is outside a macOS app bundle")
		}
	} else if filepath.Base(resources) != "resources" {
		return "", "", errors.New("current runtime-server is outside packaged resources")
	}
	return executablePath, resources, nil
}

func canonicalDirectoryV2(path string) (string, error) {
	if path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("directory path is not canonical")
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

func normalizedPlatformV2(value string) string {
	if value == "windows" {
		return "windows"
	}
	if value == "darwin" || value == "linux" {
		return value
	}
	return ""
}

func normalizedArchV2(value string) string {
	if value == "amd64" || value == "arm64" {
		return value
	}
	return ""
}

func verifyCoreResourcesAbsentV2(resources string) error {
	for _, name := range []string{"backend", "plugins/analytix-fund-analysis", "office-private", "runtime/document-runtime", "runtime/native-components", "runtime/analytix-native-development-build.json", "runtime/analytix-native-components-receipt.json", "runtime/analytix-import-accelerator", "runtime/analytix-cleaning-ops", "runtime/analytix-analysis-compute", "runtime/analytix-data-engine"} {
		if _, err := os.Lstat(filepath.Join(resources, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
			return errors.New("core package contains unexpected professional resources")
		}
	}
	return nil
}
