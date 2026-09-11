package nativecomponentregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ReceiptFileName             = "analytix-native-components-receipt.json"
	LocalBuildMarkerFileName    = "analytix-native-development-build.json"
	ReceiptSchemaVersion        = 6
	MaxReceiptBytes             = 256 * 1024
	MaxNativeBinaryBytes        = int64(2 * 1024 * 1024 * 1024)
	controlledReleaseTrustClass = "controlled_release"
	localBuildClassification    = "development_non_publishable"
	cargoPublicationProtocolV1  = "analytix-cargo-execution-authority-v1"
)

var (
	ErrUnavailable      = errors.New("native_component_registry_unavailable")
	ErrTrustInvalid     = errors.New("native_component_registry_trust_invalid")
	ErrReceiptInvalid   = errors.New("native_component_registry_receipt_invalid")
	ErrComponentInvalid = errors.New("native_component_registry_component_invalid")
)

// These values are populated only by the package builder after it has
// validated the exact target native bundle. They are deliberately not
// configurable through CLI flags or environment variables.
var (
	embeddedReceiptSHA256       string
	embeddedManifestSHA256      string
	embeddedTargetKey           string
	embeddedSigningPolicySHA256 string
	embeddedSigningMode         string
	embeddedAppleTeamIdentifier string
)

type Trust struct {
	ReceiptSHA256       string
	ManifestSHA256      string
	TargetKey           string
	SigningPolicySHA256 string
	SigningMode         string
	AppleTeamIdentifier string
	Classification      string
}

type PlatformPolicy struct {
	SHA256              string
	Mode                string
	AppleTeamIdentifier string
}

type PlatformVerifier interface {
	Policy() PlatformPolicy
	Verify(path string, opened *os.File, component ComponentReceipt) error
}

type AuthorityUse string

const (
	AuthorityUseRelease    AuthorityUse = "release"
	AuthorityUseLocalBuild AuthorityUse = "local-build"
)

type Config struct {
	RuntimeRoot  string
	Trust        Trust
	Verifier     PlatformVerifier
	AuthorityUse AuthorityUse
}

type Registry struct {
	mu         sync.RWMutex
	closing    bool
	closed     bool
	digest     string
	receipt    Receipt
	components map[string]Component
}

type Component struct {
	ID            string
	BinaryName    string
	PackagePath   string
	PayloadSHA256 string
	PayloadSize   int64
	CurrentSHA256 string
	CurrentSize   int64
	Format        string
	Arch          string
	path          string
	opened        *os.File
	fileIdentity  componentFileIdentity
}

type componentFileIdentity struct {
	device      uint64
	inode       uint64
	size        int64
	mode        uint32
	modifiedSec int64
	modifiedNS  int64
	changedSec  int64
	changedNS   int64
}

type ExecutionIdentity struct {
	ComponentID    string
	BinaryName     string
	CurrentSHA256  string
	CurrentSize    int64
	PayloadSHA256  string
	PayloadSize    int64
	Format         string
	Arch           string
	RegistryDigest string
}

type ExecutionLease struct {
	mu       sync.Mutex
	file     *os.File
	identity ExecutionIdentity
	consumed bool
}

type Receipt struct {
	SchemaVersion          int                         `json:"schemaVersion"`
	ManifestSHA256         string                      `json:"manifestSha256"`
	SourceSetSHA256        string                      `json:"sourceSetSha256"`
	BuildEnvironmentSHA256 string                      `json:"buildEnvironmentSha256"`
	Toolchain              ToolchainReceipt            `json:"toolchain"`
	TargetKey              string                      `json:"targetKey"`
	TargetTriple           string                      `json:"targetTriple"`
	Platform               string                      `json:"platform"`
	Arch                   string                      `json:"arch"`
	ExecutionAuthority     ExecutionAuthorityReceipt   `json:"executionAuthority"`
	PublicationAuthority   PublicationAuthorityReceipt `json:"publicationAuthority"`
	Components             []ComponentReceipt          `json:"components"`
}

type ExecutionAuthorityReceipt struct {
	SchemaVersion          int    `json:"schemaVersion"`
	TrustClass             string `json:"trustClass"`
	Protocol               string `json:"protocol"`
	TargetKey              string `json:"targetKey"`
	BinarySHA256           string `json:"binarySha256"`
	BinarySize             int64  `json:"binarySize"`
	SourceSetSHA256        string `json:"sourceSetSha256"`
	BuildEnvironmentSHA256 string `json:"buildEnvironmentSha256"`
	GoToolchainKey         string `json:"goToolchainKey"`
	GoExecutableSHA256     string `json:"goExecutableSha256"`
}

type PublicationAuthorityReceipt struct {
	SchemaVersion               int    `json:"schemaVersion"`
	TrustClass                  string `json:"trustClass"`
	Protocol                    string `json:"protocol"`
	TargetKey                   string `json:"targetKey"`
	CargoExecutionID            string `json:"cargoExecutionId"`
	CargoExecutionReceiptSHA256 string `json:"cargoExecutionReceiptSha256"`
	PublicationBindingSHA256    string `json:"publicationBindingSha256"`
}

type ToolchainReceipt struct {
	CargoExecutableSHA256 string `json:"cargoExecutableSha256"`
	CargoVersion          string `json:"cargoVersion"`
	RustcExecutableSHA256 string `json:"rustcExecutableSha256"`
	RustcVersion          string `json:"rustcVersion"`
}

type ComponentReceipt struct {
	ID                     string                `json:"id"`
	BinaryName             string                `json:"binaryName"`
	PackagePath            string                `json:"packagePath"`
	SourceDigest           string                `json:"sourceDigest"`
	CargoLockSHA256        string                `json:"cargoLockSha256"`
	BuildEnvironmentSHA256 string                `json:"buildEnvironmentSha256"`
	RawBuildSHA256         string                `json:"rawBuildSha256"`
	RawBuildSize           int64                 `json:"rawBuildSize"`
	StagedImageSHA256      string                `json:"stagedImageSha256"`
	StagedImageSize        int64                 `json:"stagedImageSize"`
	PayloadSHA256          string                `json:"payloadSha256"`
	PayloadSize            int64                 `json:"payloadSize"`
	Format                 string                `json:"format"`
	Arch                   string                `json:"arch"`
	ExecutionProbe         ExecutionProbeReceipt `json:"executionProbe"`
}

type localBuildMarker struct {
	SchemaVersion          int                         `json:"schemaVersion"`
	Kind                   string                      `json:"kind"`
	Classification         string                      `json:"classification"`
	Publishable            bool                        `json:"publishable"`
	ReleaseEligible        bool                        `json:"releaseEligible"`
	AuthorityUse           string                      `json:"authorityUse"`
	TargetKey              string                      `json:"targetKey"`
	TargetTriple           string                      `json:"targetTriple"`
	Platform               string                      `json:"platform"`
	Arch                   string                      `json:"arch"`
	SourceSetSHA256        string                      `json:"sourceSetSha256"`
	BuildContextSHA256     string                      `json:"buildContextSha256"`
	BuildEnvironmentSHA256 string                      `json:"buildEnvironmentSha256"`
	Toolchain              ToolchainReceipt            `json:"toolchain"`
	Components             []localBuildComponentMarker `json:"components"`
}

type localBuildComponentMarker struct {
	ID                     string `json:"id"`
	SourceDigest           string `json:"sourceDigest"`
	CargoLockSHA256        string `json:"cargoLockSha256"`
	BuildEnvironmentSHA256 string `json:"buildEnvironmentSha256"`
	BinaryName             string `json:"binaryName"`
	BinarySHA256           string `json:"binarySha256"`
	BinarySize             int64  `json:"binarySize"`
	PayloadSHA256          string `json:"payloadSha256"`
	PayloadSize            int64  `json:"payloadSize"`
	Format                 string `json:"format"`
	Arch                   string `json:"arch"`
}

type ExecutionProbeReceipt struct {
	Kind                  string `json:"kind"`
	SchemaVersion         int    `json:"schema_version"`
	Status                string `json:"status"`
	ComponentID           string `json:"component_id"`
	RequestNonce          string `json:"request_nonce"`
	ExecutableSHA256      string `json:"executable_sha256"`
	ExecutableSize        int64  `json:"executable_size"`
	ManifestSHA256        string `json:"manifest_sha256"`
	PolicySHA256          string `json:"policy_sha256"`
	AuthoritySHA256       string `json:"authority_sha256"`
	HostPlatform          string `json:"host_platform"`
	HostArch              string `json:"host_arch"`
	LoadedImageBound      bool   `json:"loaded_image_bound"`
	WorkingDirectoryBound bool   `json:"working_directory_bound"`
	GuardianAuthenticated bool   `json:"guardian_authenticated"`
	ProcessTreeEmpty      bool   `json:"process_tree_empty"`
}

type expectedComponent struct {
	ID           string
	BinaryName   string
	PackagePath  string
	PolicySHA256 string
}

var expectedComponents = []expectedComponent{
	{ID: domainnative.ComponentImportAccelerator, BinaryName: "analytix-import-accelerator", PackagePath: "runtime/analytix-import-accelerator", PolicySHA256: "0cc3872164fdadfb687a8642846913bf0f2394cd21c26799a5d483dee1ea8d2a"},
	{ID: domainnative.ComponentCleaningOps, BinaryName: "analytix-cleaning-ops", PackagePath: "runtime/analytix-cleaning-ops", PolicySHA256: "59c4287d312b4972eb6890883865e3c37d226650caf78f65168363413ac85e35"},
	{ID: domainnative.ComponentAnalysisCompute, BinaryName: "analytix-analysis-compute", PackagePath: "runtime/analytix-analysis-compute", PolicySHA256: "fd2227234c61de1505e50aa33e1e24c2f58c2545fe4d07e41b8064709c18f4d8"},
	{ID: domainnative.ComponentDataEngine, BinaryName: "analytix-data-engine", PackagePath: "runtime/analytix-data-engine", PolicySHA256: "97ce13396fb6f5ba7e9d851ab4abc2f23ac8abdd13acaa3e983a326d6bf4a1e0"},
}

func EmbeddedTrust() Trust {
	return Trust{
		ReceiptSHA256:       strings.TrimSpace(embeddedReceiptSHA256),
		ManifestSHA256:      strings.TrimSpace(embeddedManifestSHA256),
		TargetKey:           strings.TrimSpace(embeddedTargetKey),
		SigningPolicySHA256: strings.TrimSpace(embeddedSigningPolicySHA256),
		SigningMode:         strings.TrimSpace(embeddedSigningMode),
		AppleTeamIdentifier: strings.TrimSpace(embeddedAppleTeamIdentifier),
	}
}

func Load(config Config) (*Registry, error) {
	if config.AuthorityUse == "" {
		config.AuthorityUse = AuthorityUseRelease
	}
	if !validSHA256(config.Trust.ReceiptSHA256) || !validSHA256(config.Trust.ManifestSHA256) ||
		strings.TrimSpace(config.Trust.TargetKey) == "" || strings.TrimSpace(config.RuntimeRoot) == "" || config.Verifier == nil ||
		!platformPolicyMatchesTrust(config.Verifier.Policy(), config.Trust) ||
		(config.AuthorityUse != AuthorityUseRelease && config.AuthorityUse != AuthorityUseLocalBuild) {
		return nil, ErrTrustInvalid
	}
	switch config.AuthorityUse {
	case AuthorityUseRelease:
		if config.Trust.SigningMode != "developer-id" || config.Trust.Classification != "" {
			return nil, ErrTrustInvalid
		}
	case AuthorityUseLocalBuild:
		if config.Trust.SigningMode != "ad-hoc" ||
			(config.Trust.Classification != "development_clean_non_publishable" &&
				config.Trust.Classification != "development_dirty_non_publishable") {
			return nil, ErrTrustInvalid
		}
	}
	return load(config)
}

func platformPolicyMatchesTrust(policy PlatformPolicy, trust Trust) bool {
	if !strings.HasPrefix(trust.TargetKey, "darwin-") {
		return false
	}
	if !validSHA256(trust.SigningPolicySHA256) || policy.SHA256 != trust.SigningPolicySHA256 ||
		policy.Mode != trust.SigningMode || policy.AppleTeamIdentifier != trust.AppleTeamIdentifier {
		return false
	}
	switch trust.SigningMode {
	case "ad-hoc":
		return trust.AppleTeamIdentifier == ""
	case "developer-id":
		return validAppleTeamIdentifier(trust.AppleTeamIdentifier)
	default:
		return false
	}
}

func validAppleTeamIdentifier(value string) bool {
	if len(value) != 10 {
		return false
	}
	for _, character := range value {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func (registry *Registry) Digest() string {
	if registry == nil {
		return ""
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.closed || registry.closing {
		return ""
	}
	return registry.digest
}

func (registry *Registry) Component(id string) (Component, bool) {
	if registry == nil {
		return Component{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.closed || registry.closing {
		return Component{}, false
	}
	component, ok := registry.components[strings.TrimSpace(id)]
	if !ok {
		return Component{}, false
	}
	component.path = ""
	component.opened = nil
	component.fileIdentity = componentFileIdentity{}
	return component, true
}

func (registry *Registry) AcquireExecutionLease(ctx context.Context, id string) (*ExecutionLease, error) {
	if registry == nil || ctx == nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.closed || registry.closing {
		return nil, ErrUnavailable
	}
	component, ok := registry.components[strings.TrimSpace(id)]
	if !ok || component.opened == nil {
		return nil, ErrUnavailable
	}
	duplicated, err := duplicateExecutionFile(ctx, component)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrUnavailable
		}
		return nil, ErrComponentInvalid
	}
	return &ExecutionLease{
		file: duplicated,
		identity: ExecutionIdentity{
			ComponentID: component.ID, BinaryName: component.BinaryName,
			CurrentSHA256: component.CurrentSHA256, CurrentSize: component.CurrentSize,
			PayloadSHA256: component.PayloadSHA256, PayloadSize: component.PayloadSize,
			Format: component.Format, Arch: component.Arch, RegistryDigest: registry.digest,
		},
	}, nil
}

func (lease *ExecutionLease) TakeExecutionFile() (*os.File, ExecutionIdentity, error) {
	if lease == nil {
		return nil, ExecutionIdentity{}, ErrUnavailable
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.consumed || lease.file == nil {
		return nil, ExecutionIdentity{}, ErrUnavailable
	}
	file := lease.file
	identity := lease.identity
	lease.file = nil
	lease.identity = ExecutionIdentity{}
	lease.consumed = true
	return file, identity, nil
}

func (lease *ExecutionLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	lease.consumed = true
	lease.identity = ExecutionIdentity{}
	if lease.file == nil {
		return nil
	}
	if err := lease.file.Close(); err != nil {
		return err
	}
	lease.file = nil
	return nil
}

func (registry *Registry) Close() error {
	if registry == nil {
		return nil
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return nil
	}
	registry.closing = true
	var closeErrs []error
	for id, component := range registry.components {
		if component.opened != nil {
			if err := component.opened.Close(); err != nil {
				closeErrs = append(closeErrs, fmt.Errorf("close native component %s: %w", id, err))
				continue
			}
			component.opened = nil
			registry.components[id] = component
		}
	}
	if closeErr := errors.Join(closeErrs...); closeErr != nil {
		return closeErr
	}
	registry.closed = true
	registry.closing = false
	registry.components = nil
	registry.receipt = Receipt{}
	registry.digest = ""
	return nil
}

func parseReceipt(raw []byte, trust Trust, authorityUse AuthorityUse) (Receipt, error) {
	if len(raw) == 0 || len(raw) > MaxReceiptBytes || !validSHA256(trust.ReceiptSHA256) ||
		domainsecurity.SHA256Hex(raw) != trust.ReceiptSHA256 {
		return Receipt{}, ErrReceiptInvalid
	}
	if authorityUse == AuthorityUseLocalBuild {
		return parseLocalBuildMarker(raw, trust)
	}
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxReceiptBytes, MaxDepth: 16, MaxTokens: 16_384,
		MaxStringBytes: 64 * 1024, MaxNumberBytes: 64, MaxAbsExponent: 64,
	}); err != nil {
		return Receipt{}, ErrReceiptInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var receipt Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, ErrReceiptInvalid
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return Receipt{}, ErrReceiptInvalid
	}
	canonical, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil || !bytes.Equal(append(canonical, '\n'), raw) {
		return Receipt{}, ErrReceiptInvalid
	}
	if err := validateReceipt(receipt, trust, authorityUse); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func parseLocalBuildMarker(raw []byte, trust Trust) (Receipt, error) {
	if err := domainjsonstrict.Validate(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxReceiptBytes, MaxDepth: 16, MaxTokens: 16_384,
		MaxStringBytes: 64 * 1024, MaxNumberBytes: 64, MaxAbsExponent: 64,
	}); err != nil {
		return Receipt{}, ErrReceiptInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var marker localBuildMarker
	if err := decoder.Decode(&marker); err != nil {
		return Receipt{}, ErrReceiptInvalid
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return Receipt{}, ErrReceiptInvalid
	}
	canonical, err := json.MarshalIndent(marker, "", "  ")
	if err != nil || !bytes.Equal(append(canonical, '\n'), raw) {
		return Receipt{}, ErrReceiptInvalid
	}
	expectedPlatform, expectedArch, expectedTriple, ok := targetContract(trust.TargetKey)
	if !ok || marker.SchemaVersion != 1 || marker.Kind != "analytix_native_development_build" ||
		marker.Classification != localBuildClassification || marker.Publishable || marker.ReleaseEligible ||
		marker.AuthorityUse != "development_only" || marker.TargetKey != trust.TargetKey ||
		marker.TargetTriple != expectedTriple || marker.Platform != expectedPlatform || marker.Arch != expectedArch ||
		marker.Platform != runtimePlatform() || marker.Arch != runtimeArch() ||
		!validSHA256(marker.SourceSetSHA256) || !validSHA256(marker.BuildContextSHA256) ||
		!validSHA256(marker.BuildEnvironmentSHA256) ||
		!validSHA256(marker.Toolchain.CargoExecutableSHA256) ||
		!validSHA256(marker.Toolchain.RustcExecutableSHA256) ||
		!strings.HasPrefix(marker.Toolchain.CargoVersion, "cargo 1.94.1 ") ||
		!strings.HasPrefix(marker.Toolchain.RustcVersion, "rustc 1.94.1 ") ||
		len(marker.Components) != len(expectedComponents) {
		return Receipt{}, ErrReceiptInvalid
	}
	receipt := Receipt{
		SchemaVersion:          marker.SchemaVersion,
		ManifestSHA256:         trust.ManifestSHA256,
		SourceSetSHA256:        marker.SourceSetSHA256,
		BuildEnvironmentSHA256: marker.BuildEnvironmentSHA256,
		Toolchain:              marker.Toolchain,
		TargetKey:              marker.TargetKey,
		TargetTriple:           marker.TargetTriple,
		Platform:               marker.Platform,
		Arch:                   marker.Arch,
		Components:             make([]ComponentReceipt, 0, len(marker.Components)),
	}
	for index, recorded := range marker.Components {
		expected := expectedComponents[index]
		if recorded.ID != expected.ID || recorded.BinaryName != expected.BinaryName ||
			recorded.Format != expectedFormat(marker.Platform) || recorded.Arch != marker.Arch ||
			!validSHA256(recorded.SourceDigest) || !validSHA256(recorded.CargoLockSHA256) ||
			!validSHA256(recorded.BuildEnvironmentSHA256) || !validSHA256(recorded.BinarySHA256) ||
			!validSHA256(recorded.PayloadSHA256) || recorded.BinarySize <= 0 ||
			recorded.BinarySize > MaxNativeBinaryBytes || recorded.PayloadSize <= 0 ||
			recorded.PayloadSize > recorded.BinarySize {
			return Receipt{}, ErrComponentInvalid
		}
		receipt.Components = append(receipt.Components, ComponentReceipt{
			ID: recorded.ID, BinaryName: recorded.BinaryName, PackagePath: expected.PackagePath,
			SourceDigest: recorded.SourceDigest, CargoLockSHA256: recorded.CargoLockSHA256,
			BuildEnvironmentSHA256: recorded.BuildEnvironmentSHA256,
			RawBuildSHA256:         recorded.BinarySHA256, RawBuildSize: recorded.BinarySize,
			StagedImageSHA256: recorded.BinarySHA256, StagedImageSize: recorded.BinarySize,
			PayloadSHA256: recorded.PayloadSHA256, PayloadSize: recorded.PayloadSize,
			Format: recorded.Format, Arch: recorded.Arch,
		})
	}
	return receipt, nil
}

// ParseControlledBuildReceipt validates the canonical controlled-release
// receipt before it is staged for platform signing. It does not load or
// execute components; packaged admission still requires embedded receipt
// trust plus the exact platform signature policy.
func ParseControlledBuildReceipt(raw []byte, manifestSHA256, targetKey string) (Receipt, error) {
	trust := Trust{
		ReceiptSHA256:  domainsecurity.SHA256Hex(raw),
		ManifestSHA256: strings.TrimSpace(manifestSHA256),
		TargetKey:      strings.TrimSpace(targetKey),
	}
	return parseReceipt(raw, trust, AuthorityUseRelease)
}

func validateReceipt(receipt Receipt, trust Trust, authorityUse AuthorityUse) error {
	expectedPlatform, expectedArch, expectedTriple, ok := targetContract(trust.TargetKey)
	authorityPlatform, authorityArch, _, authorityTargetOK := targetContract(receipt.ExecutionAuthority.TargetKey)
	if !ok || receipt.SchemaVersion != ReceiptSchemaVersion || receipt.ManifestSHA256 != trust.ManifestSHA256 ||
		receipt.TargetKey != trust.TargetKey || receipt.Platform != expectedPlatform || receipt.Arch != expectedArch ||
		receipt.TargetTriple != expectedTriple || receipt.Platform != runtimePlatform() || receipt.Arch != runtimeArch() ||
		!validSHA256(receipt.SourceSetSHA256) || !validSHA256(receipt.BuildEnvironmentSHA256) ||
		!validSHA256(receipt.Toolchain.CargoExecutableSHA256) || !validSHA256(receipt.Toolchain.RustcExecutableSHA256) ||
		len(receipt.Toolchain.CargoVersion) > 256 || len(receipt.Toolchain.RustcVersion) > 256 ||
		!strings.HasPrefix(receipt.Toolchain.CargoVersion, "cargo 1.94.1 ") ||
		!strings.HasPrefix(receipt.Toolchain.RustcVersion, "rustc 1.94.1 ") || len(receipt.Components) != len(expectedComponents) {
		return ErrReceiptInvalid
	}
	authority := receipt.ExecutionAuthority
	publication := receipt.PublicationAuthority
	if authorityUse != AuthorityUseRelease || !authorityTargetOK || authorityPlatform != "darwin" ||
		authority.SchemaVersion != 1 || authority.TrustClass != controlledReleaseTrustClass ||
		authority.Protocol != "analytix-native-build-probe-v1" || authority.TargetKey != receipt.TargetKey ||
		authority.GoToolchainKey != authority.TargetKey ||
		authority.BinarySize <= 0 || !validSHA256(authority.BinarySHA256) ||
		!validSHA256(authority.SourceSetSHA256) || !validSHA256(authority.BuildEnvironmentSHA256) ||
		!validSHA256(authority.GoExecutableSHA256) || publication.SchemaVersion != 1 ||
		publication.TrustClass != controlledReleaseTrustClass || publication.Protocol != cargoPublicationProtocolV1 ||
		publication.TargetKey != receipt.TargetKey || !validSHA256(publication.CargoExecutionID) ||
		!validSHA256(publication.CargoExecutionReceiptSHA256) || !validSHA256(publication.PublicationBindingSHA256) {
		return ErrReceiptInvalid
	}
	for index, expected := range expectedComponents {
		component := receipt.Components[index]
		binaryName := expected.BinaryName
		packagePath := expected.PackagePath
		if receipt.Platform == "win32" {
			binaryName += ".exe"
			packagePath += ".exe"
		}
		if component.ID != expected.ID || component.BinaryName != binaryName || component.PackagePath != packagePath ||
			component.Format != expectedFormat(receipt.Platform) || component.Arch != receipt.Arch ||
			!validSHA256(component.SourceDigest) || !validSHA256(component.CargoLockSHA256) ||
			!validSHA256(component.BuildEnvironmentSHA256) || !validSHA256(component.RawBuildSHA256) ||
			!validSHA256(component.StagedImageSHA256) || !validSHA256(component.PayloadSHA256) ||
			component.RawBuildSize <= 0 || component.StagedImageSize <= 0 || component.PayloadSize <= 0 ||
			component.RawBuildSize > MaxNativeBinaryBytes || component.StagedImageSize > MaxNativeBinaryBytes ||
			component.PayloadSize > component.RawBuildSize || component.PayloadSize > component.StagedImageSize ||
			!validExecutionProbeReceipt(
				component.ExecutionProbe,
				component,
				expected.PolicySHA256,
				receipt.ManifestSHA256,
				authority,
				authorityPlatform,
				authorityArch,
			) {
			return ErrComponentInvalid
		}
	}
	return nil
}

func validExecutionProbeReceipt(
	probe ExecutionProbeReceipt,
	component ComponentReceipt,
	expectedPolicySHA256 string,
	manifestSHA256 string,
	authority ExecutionAuthorityReceipt,
	authorityPlatform string,
	authorityArch string,
) bool {
	return probe.Kind == "analytix_native_build_probe_receipt" && probe.SchemaVersion == 1 &&
		probe.Status == "passed" && probe.ComponentID == component.ID && validSHA256(probe.RequestNonce) &&
		probe.ExecutableSHA256 == component.StagedImageSHA256 && probe.ExecutableSize == component.StagedImageSize &&
		probe.ManifestSHA256 == manifestSHA256 && probe.PolicySHA256 == expectedPolicySHA256 &&
		probe.AuthoritySHA256 == authority.BinarySHA256 && probe.HostPlatform == authorityPlatform &&
		probe.HostArch == authorityArch && probe.LoadedImageBound && probe.WorkingDirectoryBound &&
		probe.GuardianAuthenticated && probe.ProcessTreeEmpty
}

func newRegistry(receipt Receipt, components map[string]Component, trust Trust) (*Registry, error) {
	if len(components) != len(expectedComponents) {
		return nil, ErrComponentInvalid
	}
	identities := make([]struct {
		ID            string `json:"id"`
		CurrentSHA256 string `json:"currentSha256"`
		CurrentSize   int64  `json:"currentSize"`
		PayloadSHA256 string `json:"payloadSha256"`
		PayloadSize   int64  `json:"payloadSize"`
	}, 0, len(expectedComponents))
	for _, expected := range expectedComponents {
		component, ok := components[expected.ID]
		if !ok {
			return nil, ErrComponentInvalid
		}
		identities = append(identities, struct {
			ID            string `json:"id"`
			CurrentSHA256 string `json:"currentSha256"`
			CurrentSize   int64  `json:"currentSize"`
			PayloadSHA256 string `json:"payloadSha256"`
			PayloadSize   int64  `json:"payloadSize"`
		}{component.ID, component.CurrentSHA256, component.CurrentSize, component.PayloadSHA256, component.PayloadSize})
	}
	body, err := json.Marshal(struct {
		Version             int    `json:"version"`
		ReceiptSHA256       string `json:"receiptSha256"`
		TrustBindingSHA256  string `json:"trustBindingSha256"`
		TargetKey           string `json:"targetKey"`
		Components          any    `json:"components"`
		SigningPolicySHA256 string `json:"signingPolicySha256"`
		SigningMode         string `json:"signingMode"`
		AppleTeamIdentifier string `json:"appleTeamIdentifier"`
	}{
		Version: 1, ReceiptSHA256: trust.ReceiptSHA256, TrustBindingSHA256: trust.ManifestSHA256,
		TargetKey: receipt.TargetKey, Components: identities,
		SigningPolicySHA256: trust.SigningPolicySHA256, SigningMode: trust.SigningMode,
		AppleTeamIdentifier: trust.AppleTeamIdentifier,
	})
	if err != nil {
		return nil, ErrComponentInvalid
	}
	return &Registry{
		digest: domainsecurity.CanonicalJSONHash(body), receipt: receipt, components: components,
	}, nil
}

func fullSHA256(file *os.File, expectedSize int64) (string, error) {
	if file == nil || expectedSize <= 0 || expectedSize > MaxNativeBinaryBytes {
		return "", ErrComponentInvalid
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, expectedSize+1))
	if err != nil || written != expectedSize {
		return "", ErrComponentInvalid
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func targetContract(key string) (platform, arch, triple string, ok bool) {
	switch key {
	case "darwin-arm64":
		return "darwin", "arm64", "aarch64-apple-darwin", true
	case "darwin-x64":
		return "darwin", "x64", "x86_64-apple-darwin", true
	case "linux-x64":
		return "linux", "x64", "x86_64-unknown-linux-gnu", true
	case "win32-x64":
		return "win32", "x64", "x86_64-pc-windows-msvc", true
	default:
		return "", "", "", false
	}
}

func expectedFormat(platform string) string {
	switch platform {
	case "darwin":
		return "mach-o"
	case "linux":
		return "elf"
	case "win32":
		return "pe"
	default:
		return ""
	}
}

func runtimePlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

func runtimeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	default:
		return runtime.GOARCH
	}
}

func wrapComponentError(id string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrComponentInvalid, id)
}
