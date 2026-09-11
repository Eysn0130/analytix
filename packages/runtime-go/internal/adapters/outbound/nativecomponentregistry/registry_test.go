//go:build darwin

package nativecomponentregistry

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testPlatformVerifier struct {
	seen   map[string]bool
	policy PlatformPolicy
}

func (verifier *testPlatformVerifier) Policy() PlatformPolicy {
	if verifier != nil && verifier.policy.SHA256 != "" {
		return verifier.policy
	}
	return PlatformPolicy{
		SHA256: sha256Hex([]byte("signing-policy")), Mode: "developer-id", AppleTeamIdentifier: "ABCDE12345",
	}
}

func (verifier *testPlatformVerifier) Verify(path string, opened *os.File, component ComponentReceipt) error {
	if verifier == nil || opened == nil || path == "" || filepath.Base(path) != component.BinaryName {
		return errors.New("test platform identity mismatch")
	}
	if verifier.seen == nil {
		verifier.seen = map[string]bool{}
	}
	if verifier.seen[component.ID] {
		return errors.New("duplicate verification")
	}
	verifier.seen[component.ID] = true
	return nil
}

func TestNativeRegistryRejectsTamperedReceiptOrSibling(t *testing.T) {
	root, trust := writeNativeRegistryFixture(t)
	verifier := &testPlatformVerifier{}
	registry, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: verifier, AuthorityUse: AuthorityUseRelease})
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if registry.Digest() == "" || len(verifier.seen) != len(expectedComponents) {
		t.Fatalf("registry = %#v verifier=%#v", registry, verifier.seen)
	}
	for _, expected := range expectedComponents {
		component, ok := registry.Component(expected.ID)
		if !ok || component.ID != expected.ID || component.CurrentSHA256 == "" || component.path != "" {
			t.Fatalf("public component %s = %#v, ok=%t", expected.ID, component, ok)
		}
	}
	lease, err := registry.AcquireExecutionLease(context.Background(), "data-engine")
	if err != nil {
		t.Fatalf("acquire execution lease: %v", err)
	}
	leasedFile, identity, err := lease.TakeExecutionFile()
	if err != nil || leasedFile == nil || identity.ComponentID != "data-engine" ||
		identity.RegistryDigest != registry.Digest() || strings.Contains(leasedFile.Name(), root) {
		t.Fatalf("take lease file=%v identity=%#v err=%v", leasedFile, identity, err)
	}
	if _, _, err := lease.TakeExecutionFile(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("second take error = %v", err)
	}
	if err := leasedFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(); err != nil || registry.Digest() != "" {
		t.Fatalf("close registry: %v", err)
	}
	if _, err := registry.AcquireExecutionLease(context.Background(), "data-engine"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("closed registry lease error = %v", err)
	}

	t.Run("receipt", func(t *testing.T) {
		root, trust := writeNativeRegistryFixture(t)
		path := filepath.Join(root, ReceiptFileName)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body[:len(body)-2], []byte(" \n")...)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: &testPlatformVerifier{}, AuthorityUse: AuthorityUseRelease}); !errors.Is(err, ErrReceiptInvalid) {
			t.Fatalf("tampered receipt error = %v", err)
		}
	})

	t.Run("sibling", func(t *testing.T) {
		root, trust := writeNativeRegistryFixture(t)
		path := filepath.Join(root, expectedComponents[1].BinaryName)
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("tampered")); err != nil {
			t.Fatal(err)
		}
		_ = file.Close()
		if _, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: &testPlatformVerifier{}, AuthorityUse: AuthorityUseRelease}); !errors.Is(err, ErrComponentInvalid) {
			t.Fatalf("tampered sibling error = %v", err)
		}
	})
}

func TestNativeRegistryCloseNeverForgetsFailedDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "failed-registry-close-")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	registry := &Registry{
		digest: strings.Repeat("a", sha256.Size*2),
		components: map[string]Component{
			"data-engine": {ID: "data-engine", opened: file},
		},
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := registry.Close(); err == nil {
			t.Fatalf("close attempt %d forgot failed descriptor", attempt)
		}
		if registry.closed || !registry.closing || registry.components == nil || registry.Digest() != "" {
			t.Fatalf("close attempt %d exposed a reusable or completed registry", attempt)
		}
	}
}

func TestExecutionLeaseCloseNeverForgetsFailedDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "failed-lease-close-")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	lease := &ExecutionLease{file: file}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := lease.Close(); err == nil {
			t.Fatalf("close attempt %d forgot failed descriptor", attempt)
		}
		if lease.file == nil || !lease.consumed {
			t.Fatalf("close attempt %d lost retry authority", attempt)
		}
	}
}

func TestNativeRegistryRejectsExecutionAfterLoadedVnodeMutation(t *testing.T) {
	root, trust := writeNativeRegistryFixture(t)
	registry, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: &testPlatformVerifier{}, AuthorityUse: AuthorityUseRelease})
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	defer registry.Close()
	path := filepath.Join(root, "analytix-data-engine")
	displaced := path + ".displaced"
	if err := os.Rename(path, displaced); err != nil {
		t.Fatal(err)
	}
	binary, _ := syntheticMachO(t)
	if err := os.WriteFile(path, binary, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AcquireExecutionLease(context.Background(), "data-engine"); !errors.Is(err, ErrComponentInvalid) {
		t.Fatalf("path replacement lease error = %v", err)
	}
}

func TestNativeRegistryCanceledLeaseNeverHashesOrEscapes(t *testing.T) {
	root, trust := writeNativeRegistryFixture(t)
	registry, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: &testPlatformVerifier{}, AuthorityUse: AuthorityUseRelease})
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if lease, err := registry.AcquireExecutionLease(ctx, "data-engine"); lease != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("canceled lease escaped: lease=%#v err=%v", lease, err)
	}
}

func TestNativeRegistryRejectsAmbiguousOrUnpinnedReceipt(t *testing.T) {
	_, trust, raw := nativeRegistryReceiptFixture(t)
	duplicate := strings.Replace(string(raw), `"schemaVersion": 6,`, `"schemaVersion": 6,\n  "schemaVersion": 6,`, 1)
	duplicateTrust := trust
	duplicateTrust.ReceiptSHA256 = sha256Hex([]byte(duplicate))
	if _, err := parseReceipt([]byte(duplicate), duplicateTrust, AuthorityUseRelease); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("duplicate receipt error = %v", err)
	}
	unknown := strings.Replace(string(raw), `"schemaVersion": 6,`, `"schemaVersion": 6,\n  "unknown": true,`, 1)
	unknownTrust := trust
	unknownTrust.ReceiptSHA256 = sha256Hex([]byte(unknown))
	if _, err := parseReceipt([]byte(unknown), unknownTrust, AuthorityUseRelease); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("unknown receipt error = %v", err)
	}
	wrong := trust
	wrong.ReceiptSHA256 = strings.Repeat("0", sha256.Size*2)
	if _, err := parseReceipt(raw, wrong, AuthorityUseRelease); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("unpinned receipt error = %v", err)
	}
}

func TestNativeRegistryRejectsEveryExecutionAuthorityDowngrade(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Receipt)
	}{
		{name: "old receipt schema", mutate: func(receipt *Receipt) { receipt.SchemaVersion = 4 }},
		{name: "authority schema", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.SchemaVersion = 2 }},
		{name: "unknown trust class", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.TrustClass = "release" }},
		{name: "authority protocol", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.Protocol = "unknown" }},
		{name: "authority target", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.TargetKey = "linux-x64" }},
		{name: "cross target authority replay", mutate: func(receipt *Receipt) {
			receipt.ExecutionAuthority.TargetKey = "darwin-x64"
			receipt.ExecutionAuthority.GoToolchainKey = "darwin-x64"
			for index := range receipt.Components {
				receipt.Components[index].ExecutionProbe.HostArch = "x64"
			}
		}},
		{name: "authority digest mismatch", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.BinarySHA256 = strings.Repeat("0", sha256.Size*2) }},
		{name: "authority size", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.BinarySize = 0 }},
		{name: "authority source", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.SourceSetSHA256 = "0" }},
		{name: "authority environment", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.BuildEnvironmentSHA256 = "0" }},
		{name: "authority toolchain mismatch", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.GoToolchainKey = "darwin-x64" }},
		{name: "authority Go executable", mutate: func(receipt *Receipt) { receipt.ExecutionAuthority.GoExecutableSHA256 = "0" }},
		{name: "publication schema", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.SchemaVersion = 2 }},
		{name: "publication trust", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.TrustClass = "local_provisional" }},
		{name: "publication protocol", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.Protocol = "unknown" }},
		{name: "publication target", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.TargetKey = "darwin-x64" }},
		{name: "cargo execution", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.CargoExecutionID = "0" }},
		{name: "cargo receipt", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.CargoExecutionReceiptSHA256 = "0" }},
		{name: "publication binding", mutate: func(receipt *Receipt) { receipt.PublicationAuthority.PublicationBindingSHA256 = "0" }},
		{name: "raw build digest", mutate: func(receipt *Receipt) { receipt.Components[0].RawBuildSHA256 = "0" }},
		{name: "raw build size", mutate: func(receipt *Receipt) { receipt.Components[0].RawBuildSize = 0 }},
		{name: "staged image digest", mutate: func(receipt *Receipt) { receipt.Components[0].StagedImageSHA256 = "0" }},
		{name: "staged image size", mutate: func(receipt *Receipt) { receipt.Components[0].StagedImageSize = 0 }},
		{name: "probe kind", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.Kind = "unknown" }},
		{name: "probe schema", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.SchemaVersion = 2 }},
		{name: "failed probe", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.Status = "failed" }},
		{name: "wrong component", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.ComponentID = "cleaning-ops" }},
		{name: "invalid challenge", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.RequestNonce = "0" }},
		{name: "wrong executable digest", mutate: func(receipt *Receipt) {
			receipt.Components[0].ExecutionProbe.ExecutableSHA256 = strings.Repeat("0", sha256.Size*2)
		}},
		{name: "wrong executable size", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.ExecutableSize = 1 }},
		{name: "wrong manifest", mutate: func(receipt *Receipt) {
			receipt.Components[0].ExecutionProbe.ManifestSHA256 = strings.Repeat("0", sha256.Size*2)
		}},
		{name: "wrong policy", mutate: func(receipt *Receipt) {
			receipt.Components[0].ExecutionProbe.PolicySHA256 = strings.Repeat("0", sha256.Size*2)
		}},
		{name: "wrong authority", mutate: func(receipt *Receipt) {
			receipt.Components[0].ExecutionProbe.AuthoritySHA256 = strings.Repeat("0", sha256.Size*2)
		}},
		{name: "wrong host", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.HostArch = "x64" }},
		{name: "wrong host platform", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.HostPlatform = "linux" }},
		{name: "loaded image unbound", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.LoadedImageBound = false }},
		{name: "working directory unbound", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.WorkingDirectoryBound = false }},
		{name: "guardian unauthenticated", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.GuardianAuthenticated = false }},
		{name: "process tree not empty", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe.ProcessTreeEmpty = false }},
		{name: "cross component replay", mutate: func(receipt *Receipt) { receipt.Components[0].ExecutionProbe = receipt.Components[1].ExecutionProbe }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt, trust, _ := nativeRegistryReceiptFixture(t)
			test.mutate(&receipt)
			if err := validateReceipt(receipt, trust, AuthorityUseRelease); err == nil {
				t.Fatal("downgraded execution authority was accepted")
			}
		})
	}
}

func TestNativeRegistryDefaultsToReleaseAuthority(t *testing.T) {
	root, trust := writeNativeRegistryFixture(t)
	registry, err := Load(Config{
		RuntimeRoot: root,
		Trust:       trust,
		Verifier:    &testPlatformVerifier{},
	})
	if err != nil {
		t.Fatalf("controlled receipt failed release default: %v", err)
	}
	defer registry.Close()
	if _, err := Load(Config{
		RuntimeRoot: root, Trust: trust, Verifier: &testPlatformVerifier{}, AuthorityUse: AuthorityUseLocalBuild,
	}); !errors.Is(err, ErrTrustInvalid) {
		t.Fatalf("controlled receipt escaped into local authority mode: %v", err)
	}
	if _, err := Load(Config{
		RuntimeRoot:  root,
		Trust:        trust,
		Verifier:     &testPlatformVerifier{},
		AuthorityUse: AuthorityUse("unknown"),
	}); !errors.Is(err, ErrTrustInvalid) {
		t.Fatalf("unknown authority use error = %v", err)
	}
}

func TestNativeRegistryLoadsExactLocalBuildMarkerOnlyInLocalMode(t *testing.T) {
	root, trust := writeLocalBuildRegistryFixture(t)
	verifier := &testPlatformVerifier{policy: PlatformPolicy{
		SHA256: trust.SigningPolicySHA256, Mode: "ad-hoc",
	}}
	registry, err := Load(Config{
		RuntimeRoot: root, Trust: trust, Verifier: verifier, AuthorityUse: AuthorityUseLocalBuild,
	})
	if err != nil {
		t.Fatalf("exact local build marker was rejected: %v", err)
	}
	if registry.Digest() == "" {
		t.Fatal("local build registry did not retain a process-local digest")
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: &testPlatformVerifier{}}); !errors.Is(err, ErrTrustInvalid) {
		t.Fatalf("local marker escaped into release authority mode: %v", err)
	}

	markerPath := filepath.Join(root, LocalBuildMarkerFileName)
	body, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)/2] ^= 1
	if err := os.WriteFile(markerPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Config{
		RuntimeRoot: root, Trust: trust, Verifier: verifier, AuthorityUse: AuthorityUseLocalBuild,
	}); !errors.Is(err, ErrReceiptInvalid) {
		t.Fatalf("damaged local marker survived: %v", err)
	}
}

func TestNativeRegistryRejectsVerifierPolicyDrift(t *testing.T) {
	root, trust := writeNativeRegistryFixture(t)
	wrong := &testPlatformVerifier{policy: PlatformPolicy{
		SHA256: sha256Hex([]byte("other-policy")), Mode: "ad-hoc",
	}}
	if _, err := Load(Config{RuntimeRoot: root, Trust: trust, Verifier: wrong, AuthorityUse: AuthorityUseRelease}); !errors.Is(err, ErrTrustInvalid) {
		t.Fatalf("policy drift error = %v", err)
	}
	adHocTrust := trust
	adHocTrust.SigningMode = "ad-hoc"
	adHocTrust.AppleTeamIdentifier = ""
	if _, err := Load(Config{RuntimeRoot: root, Trust: adHocTrust, Verifier: &testPlatformVerifier{}, AuthorityUse: AuthorityUseRelease}); !errors.Is(err, ErrTrustInvalid) {
		t.Fatalf("signing mode drift error = %v", err)
	}
}

func TestMachOPayloadIdentityMatchesSigningStableContract(t *testing.T) {
	payload, expectedPayload := syntheticMachO(t)
	path := filepath.Join(t.TempDir(), "component")
	if err := os.WriteFile(path, payload, 0o500); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash, size, arch, err := machOPayloadIdentity(file, int64(len(payload)))
	if err != nil {
		t.Fatalf("payload identity: %v", err)
	}
	if hash != sha256Hex(expectedPayload) || size != int64(len(expectedPayload)) || arch != runtimeArch() {
		t.Fatalf("identity hash=%s size=%d arch=%s", hash, size, arch)
	}
}

func writeNativeRegistryFixture(t *testing.T) (string, Trust) {
	t.Helper()
	receipt, trust, raw := nativeRegistryReceiptFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ReceiptFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	binary, _ := syntheticMachO(t)
	for index, expected := range expectedComponents {
		if receipt.Components[index].BinaryName != expected.BinaryName {
			t.Fatalf("fixture order mismatch: %#v", receipt.Components[index])
		}
		if err := os.WriteFile(filepath.Join(root, expected.BinaryName), binary, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	return root, trust
}

func writeLocalBuildRegistryFixture(t *testing.T) (string, Trust) {
	t.Helper()
	release, _, _ := nativeRegistryReceiptFixture(t)
	marker := localBuildMarker{
		SchemaVersion: 1, Kind: "analytix_native_development_build",
		Classification: localBuildClassification, Publishable: false, ReleaseEligible: false,
		AuthorityUse: "development_only", TargetKey: release.TargetKey,
		TargetTriple: release.TargetTriple, Platform: release.Platform, Arch: release.Arch,
		SourceSetSHA256:        release.SourceSetSHA256,
		BuildContextSHA256:     sha256Hex([]byte("local-build-context")),
		BuildEnvironmentSHA256: release.BuildEnvironmentSHA256, Toolchain: release.Toolchain,
	}
	for _, component := range release.Components {
		marker.Components = append(marker.Components, localBuildComponentMarker{
			ID: component.ID, SourceDigest: component.SourceDigest,
			CargoLockSHA256:        component.CargoLockSHA256,
			BuildEnvironmentSHA256: component.BuildEnvironmentSHA256,
			BinaryName:             component.BinaryName, BinarySHA256: component.StagedImageSHA256,
			BinarySize: component.StagedImageSize, PayloadSHA256: component.PayloadSHA256,
			PayloadSize: component.PayloadSize, Format: component.Format, Arch: component.Arch,
		})
	}
	raw, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, LocalBuildMarkerFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	binary, _ := syntheticMachO(t)
	for _, expected := range expectedComponents {
		if err := os.WriteFile(filepath.Join(root, expected.BinaryName), binary, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	return root, Trust{
		ReceiptSHA256: sha256Hex(raw), ManifestSHA256: sha256Hex([]byte("local-package-authority")),
		TargetKey: marker.TargetKey, SigningPolicySHA256: sha256Hex([]byte("signing-policy")),
		SigningMode: "ad-hoc", Classification: "development_clean_non_publishable",
	}
}

func nativeRegistryReceiptFixture(t *testing.T) (Receipt, Trust, []byte) {
	t.Helper()
	binary, expectedPayload := syntheticMachO(t)
	payloadHash := sha256Hex(expectedPayload)
	platform := runtimePlatform()
	arch := runtimeArch()
	if platform != "darwin" {
		t.Skip("Darwin registry fixture")
	}
	targetKey := "darwin-" + arch
	_, _, triple, ok := targetContract(targetKey)
	if !ok {
		t.Fatalf("target contract %s", targetKey)
	}
	manifestHash := sha256Hex([]byte("manifest"))
	receipt := Receipt{
		SchemaVersion: ReceiptSchemaVersion, ManifestSHA256: manifestHash,
		SourceSetSHA256: sha256Hex([]byte("source-set")), BuildEnvironmentSHA256: sha256Hex([]byte("environment")),
		Toolchain: ToolchainReceipt{
			CargoExecutableSHA256: sha256Hex([]byte("cargo")), CargoVersion: "cargo 1.94.1 (fixture)",
			RustcExecutableSHA256: sha256Hex([]byte("rustc")), RustcVersion: "rustc 1.94.1 (fixture)",
		},
		TargetKey: targetKey, TargetTriple: triple, Platform: platform, Arch: arch,
		ExecutionAuthority: ExecutionAuthorityReceipt{
			SchemaVersion: 1, TrustClass: controlledReleaseTrustClass, Protocol: "analytix-native-build-probe-v1",
			TargetKey: targetKey, BinarySHA256: sha256Hex([]byte("authority")), BinarySize: 4096,
			SourceSetSHA256:        sha256Hex([]byte("authority-source")),
			BuildEnvironmentSHA256: sha256Hex([]byte("authority-environment")),
			GoToolchainKey:         targetKey, GoExecutableSHA256: sha256Hex([]byte("go")),
		},
		PublicationAuthority: PublicationAuthorityReceipt{
			SchemaVersion: 1, TrustClass: controlledReleaseTrustClass, Protocol: cargoPublicationProtocolV1,
			TargetKey: targetKey, CargoExecutionID: sha256Hex([]byte("cargo-execution")),
			CargoExecutionReceiptSHA256: sha256Hex([]byte("cargo-receipt")),
			PublicationBindingSHA256:    sha256Hex([]byte("publication-binding")),
		},
	}
	for _, expected := range expectedComponents {
		receipt.Components = append(receipt.Components, ComponentReceipt{
			ID: expected.ID, BinaryName: expected.BinaryName, PackagePath: expected.PackagePath,
			SourceDigest: sha256Hex([]byte("source:" + expected.ID)), CargoLockSHA256: sha256Hex([]byte("lock:" + expected.ID)),
			BuildEnvironmentSHA256: sha256Hex([]byte("env:" + expected.ID)),
			RawBuildSHA256:         sha256Hex(binary), RawBuildSize: int64(len(binary)),
			StagedImageSHA256: sha256Hex(binary), StagedImageSize: int64(len(binary)),
			PayloadSHA256: payloadHash, PayloadSize: int64(len(expectedPayload)), Format: "mach-o", Arch: arch,
			ExecutionProbe: ExecutionProbeReceipt{
				Kind: "analytix_native_build_probe_receipt", SchemaVersion: 1, Status: "passed",
				ComponentID: expected.ID, RequestNonce: sha256Hex([]byte("nonce:" + expected.ID)),
				ExecutableSHA256: sha256Hex(binary), ExecutableSize: int64(len(binary)),
				ManifestSHA256: manifestHash, PolicySHA256: expected.PolicySHA256,
				AuthoritySHA256: receipt.ExecutionAuthority.BinarySHA256,
				HostPlatform:    platform, HostArch: arch, LoadedImageBound: true,
				WorkingDirectoryBound: true, GuardianAuthenticated: true, ProcessTreeEmpty: true,
			},
		})
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	trust := Trust{
		ReceiptSHA256: sha256Hex(raw), ManifestSHA256: manifestHash, TargetKey: targetKey,
		SigningPolicySHA256: sha256Hex([]byte("signing-policy")), SigningMode: "developer-id",
		AppleTeamIdentifier: "ABCDE12345",
	}
	return receipt, trust, raw
}

func syntheticMachO(t *testing.T) ([]byte, []byte) {
	t.Helper()
	const (
		textOffset     = 32
		linkEditOffset = textOffset + 72
		signatureCmd   = linkEditOffset + 72
		payloadSize    = 256
		signatureSize  = 32
	)
	body := make([]byte, payloadSize+signatureSize)
	order := binary.LittleEndian
	order.PutUint32(body[0:4], 0xfeedfacf)
	cpu := uint32(0x0100000c)
	if runtimeArch() == "x64" {
		cpu = 0x01000007
	}
	order.PutUint32(body[4:8], cpu)
	order.PutUint32(body[12:16], machOExecutableFileType)
	order.PutUint32(body[16:20], 3)
	order.PutUint32(body[20:24], 72+72+16)
	writeSegment := func(offset int, name string, fileOffset, fileSize uint64, protection uint32) {
		order.PutUint32(body[offset:offset+4], machOSegment64Command)
		order.PutUint32(body[offset+4:offset+8], 72)
		copy(body[offset+8:offset+24], []byte(name))
		order.PutUint64(body[offset+32:offset+40], fileSize)
		order.PutUint64(body[offset+40:offset+48], fileOffset)
		order.PutUint64(body[offset+48:offset+56], fileSize)
		order.PutUint32(body[offset+60:offset+64], protection)
	}
	writeSegment(textOffset, "__TEXT", 0, payloadSize, 5)
	writeSegment(linkEditOffset, "__LINKEDIT", payloadSize, signatureSize, 1)
	order.PutUint32(body[signatureCmd:signatureCmd+4], machOCodeSignature)
	order.PutUint32(body[signatureCmd+4:signatureCmd+8], 16)
	order.PutUint32(body[signatureCmd+8:signatureCmd+12], payloadSize)
	order.PutUint32(body[signatureCmd+12:signatureCmd+16], signatureSize)
	binary.BigEndian.PutUint32(body[payloadSize:payloadSize+4], machOCodeSuperBlobMagic)
	binary.BigEndian.PutUint32(body[payloadSize+4:payloadSize+8], 12)

	expected := append([]byte(nil), body[:payloadSize]...)
	order.PutUint64(expected[linkEditOffset+32:linkEditOffset+40], 0)
	order.PutUint64(expected[linkEditOffset+48:linkEditOffset+56], 0)
	order.PutUint32(expected[signatureCmd+12:signatureCmd+16], 0)
	return body, expected
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
