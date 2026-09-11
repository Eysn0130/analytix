package packagedbuildauthorityfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	pluginmaterializationfs "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

var legacyFundsSkillNamesFixtureV0 = [...]string{
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

func TestPackagedArtifactBindingsDetectRunnerAndAppASARDrift(t *testing.T) {
	runnerBody := syntheticMachOV2(12)
	runnerIdentity, err := inspectNativePayloadV2(runnerBody)
	if err != nil {
		t.Fatal(err)
	}
	binding := domainauthority.NativeArtifactBindingV2{
		PayloadSHA256:     runnerIdentity.PayloadSHA256,
		PayloadByteLength: runnerIdentity.PayloadBytes,
		Format:            runnerIdentity.Format, Arch: runnerIdentity.Arch,
	}
	if !nativeBindingMatchesV2(runnerIdentity, binding) {
		t.Fatal("exact application runner binding was rejected")
	}
	tamperedRunner := append([]byte(nil), runnerBody...)
	tamperedRunner[240] ^= 0xff
	tamperedIdentity, err := inspectNativePayloadV2(tamperedRunner)
	if err != nil {
		t.Fatal(err)
	}
	if nativeBindingMatchesV2(tamperedIdentity, binding) {
		t.Fatal("application runner payload drift was accepted")
	}

	appASAR := filepath.Join(t.TempDir(), "app.asar")
	if err := os.WriteFile(appASAR, []byte("asar-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := stableContentIdentityV2(appASAR, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appASAR, []byte("asar-v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := stableContentIdentityV2(appASAR, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("app.asar content drift was accepted")
	}
}

func TestPackagedApplicationArtifactPathsAreFixed(t *testing.T) {
	darwinResources := filepath.Join(string(filepath.Separator), "Applications", "analytix.app", "Contents", "Resources")
	runner, appASAR, err := resolveApplicationArtifactsV2(darwinResources, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if runner != filepath.Join(string(filepath.Separator), "Applications", "analytix.app", "Contents", "MacOS", "analytix") ||
		appASAR != filepath.Join(darwinResources, "app.asar") {
		t.Fatalf("unexpected Darwin artifacts: runner=%q asar=%q", runner, appASAR)
	}
	if runtime.GOOS != "windows" {
		root := t.TempDir()
		target := filepath.Join(root, "app.asar")
		if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, "linked.asar")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := stableContentIdentityV2(link, 1024); err == nil {
			t.Fatal("symbolic-link app.asar was accepted")
		}
	}
}

func TestPackagedNativeArtifactBindingsMatchExactTarget(t *testing.T) {
	artifacts := domainauthority.ArtifactBindingsV2{
		Executable:    domainauthority.NativeArtifactBindingV2{Format: "mach-o", Arch: "arm64"},
		RuntimeServer: domainauthority.NativeArtifactBindingV2{Format: "mach-o", Arch: "arm64"},
	}
	if !nativeArtifactTargetsMatchV2(artifacts, "darwin", "arm64") {
		t.Fatal("matching packaged native artifact target was rejected")
	}
	artifacts.Executable.Arch = "x64"
	if nativeArtifactTargetsMatchV2(artifacts, "darwin", "arm64") {
		t.Fatal("mixed-architecture packaged native artifacts were accepted")
	}
	artifacts.Executable.Arch = "arm64"
	artifacts.RuntimeServer.Format = "pe"
	if nativeArtifactTargetsMatchV2(artifacts, "darwin", "arm64") {
		t.Fatal("mixed-format packaged native artifacts were accepted")
	}
}

func TestFundsPluginInspectionClassificationPreservesContext(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, test := range map[string]struct {
		ctx           context.Context
		inspectionErr error
		want          error
	}{
		"source canceled": {
			ctx: context.Background(), inspectionErr: context.Canceled, want: context.Canceled,
		},
		"source deadline": {
			ctx: context.Background(), inspectionErr: context.DeadlineExceeded, want: context.DeadlineExceeded,
		},
		"context canceled around source failure": {
			ctx: cancelled, inspectionErr: errors.New("source unavailable"), want: context.Canceled,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := classifyFundsPluginInspectionV2(test.ctx, test.inspectionErr, false)
			if !errors.Is(err, test.want) || errors.Is(err, ErrPackagedFundsPluginRootUnavailable) {
				t.Fatalf("source context was downgraded to Funds-only: %v", err)
			}
		})
	}
	if err := classifyFundsPluginInspectionV2(context.Background(), errors.New("source unavailable"), false); !errors.Is(err, ErrPackagedFundsPluginRootUnavailable) {
		t.Fatalf("non-context Funds source failure lost its local classification: %v", err)
	}
	if err := classifyFundsPluginInspectionV2(context.Background(), nil, true); err != nil {
		t.Fatalf("valid Funds source inspection was rejected: %v", err)
	}
}

func TestPostAnchorArtifactBarrierRejectsContentAndPathDrift(t *testing.T) {
	t.Run("stable baseline", func(t *testing.T) {
		paths, witness, authority := newPostAnchorBarrierFixtureV2(t)
		if err := revalidatePackageArtifactsV2(context.Background(), paths, witness, authority); err != nil {
			t.Fatalf("stable post-anchor artifacts were rejected: %v", err)
		}
	})
	t.Run("cancellation remains global", func(t *testing.T) {
		paths, witness, authority := newPostAnchorBarrierFixtureV2(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := revalidatePackageArtifactsV2(ctx, paths, witness, authority); !errors.Is(err, context.Canceled) ||
			errors.Is(err, ErrPackagedFundsPluginRootUnavailable) {
			t.Fatalf("package cancellation was downgraded to a Funds-only failure: %v", err)
		}
	})

	tests := map[string]func(*testing.T, packageArtifactPathsV2, packageArtifactWitnessV2){
		"authority content": func(t *testing.T, paths packageArtifactPathsV2, _ packageArtifactWitnessV2) {
			if err := os.WriteFile(paths.authority, []byte("authority-v2"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"application runner content": func(t *testing.T, paths packageArtifactPathsV2, _ packageArtifactWitnessV2) {
			body, err := os.ReadFile(paths.applicationRunner)
			if err != nil {
				t.Fatal(err)
			}
			body[240] ^= 0xff
			if err := os.WriteFile(paths.applicationRunner, body, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"app asar content": func(t *testing.T, paths packageArtifactPathsV2, _ packageArtifactWitnessV2) {
			if err := os.WriteFile(paths.appASAR, []byte("asar-v2"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"runtime server content": func(t *testing.T, paths packageArtifactPathsV2, _ packageArtifactWitnessV2) {
			body, err := os.ReadFile(paths.runtimeServer)
			if err != nil {
				t.Fatal(err)
			}
			body[240] ^= 0xff
			if err := os.WriteFile(paths.runtimeServer, body, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"funds plugin content": func(t *testing.T, paths packageArtifactPathsV2, _ packageArtifactWitnessV2) {
			if err := os.WriteFile(filepath.Join(paths.pluginRoot, "mcp", "server.mjs"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"authority path identity": func(t *testing.T, paths packageArtifactPathsV2, witness packageArtifactWitnessV2) {
			replacement := paths.authority + ".replacement"
			if err := os.WriteFile(replacement, witness.authorityBody, 0o600); err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS == "windows" {
				if err := os.Remove(paths.authority); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Rename(replacement, paths.authority); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			paths, witness, authority := newPostAnchorBarrierFixtureV2(t)
			mutate(t, paths, witness)
			err := revalidatePackageArtifactsV2(context.Background(), paths, witness, authority)
			if err == nil {
				t.Fatal("post-anchor artifact drift escaped the return barrier")
			}
			localFundsFailure := errors.Is(err, ErrPackagedFundsPluginRootUnavailable)
			if name == "funds plugin content" && !localFundsFailure {
				t.Fatalf("Funds plugin drift lost its capability-local classification: %v", err)
			}
			if name != "funds plugin content" && localFundsFailure {
				t.Fatalf("global package drift was downgraded to a Funds-only failure: %v", err)
			}
		})
	}
}

func TestPostAnchorArtifactBarrierRecognizesOnlyBoundedLegacyFundsV0(t *testing.T) {
	paths, witness, authority := newPostAnchorBarrierFixtureV2(t)
	if err := os.Remove(filepath.Join(paths.pluginRoot, ".analytix-plugin", "package.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(paths.pluginRoot, ".mcp.json"),
		[]byte(`{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for relative, body := range map[string]string{
		"assets/icon.png":    "legacy-icon\n",
		"assets/logo.png":    "legacy-logo\n",
		"agents/openai.yaml": "interface:\n  display_name: Analytix Funds\n",
	} {
		path := filepath.Join(paths.pluginRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, skill := range legacyFundsSkillNamesFixtureV0 {
		path := filepath.Join(paths.pluginRoot, "skills", skill, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: "+skill+"\n---\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := pluginmaterializationfs.InspectPackagedFundsSourceTreeV1(context.Background(), paths.pluginRoot)
	if err != nil || !identity.LegacyV0 {
		t.Fatalf("sealed historical Funds artifact was not recognized: identity=%#v err=%v", identity, err)
	}
	witness.pluginSourceIdentity = identity
	authority.Authority.Artifacts.FundsPlugin = domainauthority.FundsPluginArtifactBindingV2{
		TreeSHA256: identity.TreeSHA256, FileCount: int64(identity.FileCount),
		ManifestSHA256: identity.ManifestSHA256, EntrypointSHA256: identity.EntrypointSHA256,
		TotalBytes: identity.TotalBytes,
	}
	if err := revalidatePackageArtifactsV2(context.Background(), paths, witness, authority); err != nil {
		t.Fatalf("stable sealed legacy-v0 artifact failed the package return barrier: %v", err)
	}
	companion := filepath.Join(paths.pluginRoot, ".analytix-plugin", "package.json")
	if err := os.Mkdir(companion, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := revalidatePackageArtifactsV2(context.Background(), paths, witness, authority); err == nil ||
		!errors.Is(err, ErrPackagedFundsPluginRootUnavailable) {
		t.Fatalf("present directory companion did not fail as Funds-only package drift: %v", err)
	}
}

func newPostAnchorBarrierFixtureV2(t *testing.T) (
	packageArtifactPathsV2,
	packageArtifactWitnessV2,
	domainauthority.ParsedAuthorityV2,
) {
	t.Helper()
	root := t.TempDir()
	paths := packageArtifactPathsV2{
		authority: filepath.Join(root, "authority.json"), applicationRunner: filepath.Join(root, "analytix"),
		appASAR: filepath.Join(root, "app.asar"), runtimeServer: filepath.Join(root, "runtime-server"),
		pluginRoot: filepath.Join(root, "plugin"),
	}
	if err := os.MkdirAll(filepath.Join(paths.pluginRoot, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.pluginRoot, ".analytix-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.pluginRoot, "mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	for relative, body := range map[string]string{
		".analytix-plugin/package.json": `{"schemaVersion":1,"packageId":"analytix-fund-analysis","packageVersion":"0.16.16","contributions":{"skills":[],"mcpServers":[{"id":"analytix_funds","entrypoint":"mcp/server.mjs"}],"hooks":[],"assets":[],"publicUi":[]},"requestedCapabilities":[{"id":"funds.case.read","protocolVersion":1,"scopeConstraints":["case:bound","source:verified"]}],"lifecycle":{"protocolVersion":1,"entryPolicy":"host-static-first-party"}}`,
		".codex-plugin/plugin.json":     `{"name":"analytix-fund-analysis","version":"0.16.16"}`,
		".mcp.json":                     `{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`,
		"mcp/server.mjs":                "export const factsEnabled = false\n",
	} {
		if err := os.WriteFile(filepath.Join(paths.pluginRoot, filepath.FromSlash(relative)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runnerBody := syntheticMachOV2(12)
	runtimeBody := syntheticMachOV2(20)
	for path, fixture := range map[string]struct {
		body []byte
		mode os.FileMode
	}{
		paths.authority:         {body: []byte("authority-v1"), mode: 0o600},
		paths.applicationRunner: {body: runnerBody, mode: 0o700},
		paths.appASAR:           {body: []byte("asar-v1"), mode: 0o600},
		paths.runtimeServer:     {body: runtimeBody, mode: 0o700},
	} {
		if err := os.WriteFile(path, fixture.body, fixture.mode); err != nil {
			t.Fatal(err)
		}
	}
	authoritySnapshot, err := stableReadSnapshotFileV2(paths.authority, domainauthority.MaxAuthorityBytesV2, false)
	if err != nil {
		t.Fatal(err)
	}
	runnerSnapshot, err := stableReadSnapshotFileV2(paths.applicationRunner, maxRuntimeBinaryBytesV2, false)
	if err != nil {
		t.Fatal(err)
	}
	runnerIdentity, err := inspectNativePayloadV2(runnerSnapshot.body)
	clear(runnerSnapshot.body)
	if err != nil {
		t.Fatal(err)
	}
	appASARSnapshot, err := stableContentSnapshotV2(paths.appASAR, maxAppASARBytesV2)
	if err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, err := stableReadSnapshotFileV2(paths.runtimeServer, maxRuntimeBinaryBytesV2, false)
	if err != nil {
		t.Fatal(err)
	}
	runtimeIdentity, err := inspectNativePayloadV2(runtimeSnapshot.body)
	clear(runtimeSnapshot.body)
	if err != nil {
		t.Fatal(err)
	}
	pluginIdentity, err := pluginmaterializationfs.InspectSourceTreeV1(context.Background(), paths.pluginRoot)
	if err != nil {
		t.Fatal(err)
	}
	authority := domainauthority.ParsedAuthorityV2{
		Authority: domainauthority.AuthorityV2{
			Artifacts: domainauthority.ArtifactBindingsV2{
				Executable: domainauthority.NativeArtifactBindingV2{
					PayloadSHA256: runnerIdentity.PayloadSHA256, PayloadByteLength: runnerIdentity.PayloadBytes,
					Format: runnerIdentity.Format, Arch: runnerIdentity.Arch,
				},
				AppASAR: domainauthority.ContentArtifactBindingV2{
					SHA256: appASARSnapshot.identity.SHA256, ByteLength: appASARSnapshot.identity.ByteLength,
				},
				RuntimeServer: domainauthority.NativeArtifactBindingV2{
					PayloadSHA256: runtimeIdentity.PayloadSHA256, PayloadByteLength: runtimeIdentity.PayloadBytes,
					Format: runtimeIdentity.Format, Arch: runtimeIdentity.Arch,
				},
				FundsPlugin: domainauthority.FundsPluginArtifactBindingV2{
					TreeSHA256: pluginIdentity.TreeSHA256, FileCount: int64(pluginIdentity.FileCount),
					ManifestSHA256:   pluginIdentity.ManifestSHA256,
					EntrypointSHA256: pluginIdentity.EntrypointSHA256, TotalBytes: pluginIdentity.TotalBytes,
				},
			},
		},
	}
	return paths, packageArtifactWitnessV2{
		authorityBody: authoritySnapshot.body, authorityFile: authoritySnapshot.info,
		applicationRunnerFile: runnerSnapshot.info, applicationRunnerIdentity: runnerIdentity,
		appASARFile: appASARSnapshot.info, appASARIdentity: appASARSnapshot.identity,
		runtimeServerFile: runtimeSnapshot.info, runtimeServerIdentity: runtimeIdentity,
		pluginSourceIdentity: pluginIdentity,
	}, authority
}

func TestUnpackagedRuntimeClassificationRequiresExistingCanonicalExecutable(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "runtimeapp.test")
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := InspectPackageV2(context.Background(), executable, runtime.GOOS, runtime.GOARCH)
	if err != ErrNotPackagedRuntimeV2 || inspection.PackageAnchor != "" {
		t.Fatalf("nonpackage classification: %v", err)
	}
	for _, path := range []string{filepath.Join(root, "absent"), root} {
		if _, err := InspectPackageV2(context.Background(), path, runtime.GOOS, runtime.GOARCH); err == nil || errors.Is(err, ErrNotPackagedRuntimeV2) {
			t.Fatalf("unverified executable was classified as nonpackage: %v", err)
		}
	}
	if runtime.GOOS != "windows" {
		linked := filepath.Join(root, "linked")
		if err := os.Symlink(executable, linked); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectPackageV2(context.Background(), linked, runtime.GOOS, runtime.GOARCH); err == nil || errors.Is(err, ErrNotPackagedRuntimeV2) {
			t.Fatalf("symlink was classified as nonpackage: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectPackageV2(ctx, executable, runtime.GOOS, runtime.GOARCH); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	resources := filepath.Join(root, "resources")
	if runtime.GOOS == "darwin" {
		resources = filepath.Join(root, "Example.app", "Contents", "Resources")
	}
	name := "runtime-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	packaged := filepath.Join(resources, "runtime-go", "bin", name)
	if err := os.MkdirAll(filepath.Dir(packaged), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packaged, []byte("synthetic runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectPackageV2(context.Background(), packaged, runtime.GOOS, runtime.GOARCH); err == nil || errors.Is(err, ErrNotPackagedRuntimeV2) {
		t.Fatalf("missing package authority was classified as nonpackage: %v", err)
	}
}
