package packagedbuildauthority

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

//go:embed testdata/packaged-build-authority-v2-js-golden.json
var packagedBuildAuthorityV2JSGolden []byte

func TestPackagedBuildAuthorityV2AcceptsJavaScriptProducerGolden(t *testing.T) {
	bodyWithNewline := packagedBuildAuthorityV2JSGolden
	if !bytes.HasSuffix(bodyWithNewline, []byte("\n")) || bytes.HasSuffix(bodyWithNewline, []byte("\n\n")) {
		t.Fatal("JavaScript producer golden must contain exactly one trailing newline")
	}
	body := bytes.TrimSuffix(bodyWithNewline, []byte("\n"))
	parsed, err := ParseV2(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Controlled == nil || parsed.Development != nil {
		t.Fatalf("JavaScript producer disposition parsed incorrectly: %#v", parsed)
	}
	if parsed.Authority.AuthorityDigest != "f310dbe02a258009085562dee8cd293daf2e1f4f69523ca4229af1daf573885b" {
		t.Fatalf("JavaScript producer authority digest drifted: %q", parsed.Authority.AuthorityDigest)
	}
	if parsed.Authority.BuildContext.ContextDigest != "a5f878cf3708c5dcd27c8ba8c70564ad7880a1cdf7e2b162a43d8874b21c5163" {
		t.Fatalf("JavaScript producer build context digest drifted: %q", parsed.Authority.BuildContext.ContextDigest)
	}
}

func TestPackagedBuildAuthorityV2StrictRoundTrip(t *testing.T) {
	authority := authorityFixtureV2(false, ControlledDispositionKindV2)
	body, err := json.Marshal(authority)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseV2(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Controlled == nil || parsed.Development != nil || parsed.DispositionKind() != ControlledDispositionKindV2 {
		t.Fatalf("controlled disposition parsed incorrectly: %#v", parsed)
	}
	platform, arch, ok := parsed.Target()
	if !ok || platform != "darwin" || arch != "arm64" {
		t.Fatalf("target parsed incorrectly: platform=%q arch=%q ok=%v", platform, arch, ok)
	}
	if AuthorityFileSHA256V2(body) == authority.AuthorityDigest {
		t.Fatal("raw authority file identity collapsed into its inner self-digest")
	}
}

func TestPackagedBuildAuthorityV2RejectsUnknownDuplicateNonCanonicalAndSelfHashDrift(t *testing.T) {
	authority := authorityFixtureV2(false, ControlledDispositionKindV2)
	body, _ := json.Marshal(authority)
	mutations := map[string][]byte{
		"unknown":         []byte(strings.Replace(string(body), `"schemaVersion":2`, `"schemaVersion":2,"unknown":true`, 1)),
		"duplicate":       []byte(strings.Replace(string(body), `"schemaVersion":2`, `"schemaVersion":2,"schemaVersion":2`, 1)),
		"noncanonical":    append([]byte(" "), body...),
		"self hash drift": []byte(strings.Replace(string(body), authority.AuthorityDigest, digestFixtureV2("other-authority"), 1)),
	}
	for name, mutated := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseV2(mutated); err == nil {
				t.Fatal("invalid packaged authority was accepted")
			}
		})
	}
}

func TestPackagedBuildAuthorityV2SeparatesDevelopmentFromControlledRelease(t *testing.T) {
	development := authorityFixtureV2(true, DevelopmentDispositionKindV2)
	body, _ := json.Marshal(development)
	parsed, err := ParseV2(body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Development == nil || parsed.Controlled != nil || parsed.Authority.Classification != "development_dirty_non_publishable" {
		t.Fatalf("development authority parsed incorrectly: %#v", parsed)
	}

	for name, mutate := range map[string]func(*AuthorityV2){
		"publishable":         func(value *AuthorityV2) { value.Publishable = true },
		"release eligible":    func(value *AuthorityV2) { value.ReleaseEligible = true },
		"publication receipt": func(value *AuthorityV2) { value.PublicationReceiptIssued = true },
		"classification":      func(value *AuthorityV2) { value.Classification = "controlled_release_clean_candidate_non_publishable" },
	} {
		t.Run(name, func(t *testing.T) {
			value := development
			mutate(&value)
			value.AuthorityDigest = authorityDigest(value)
			mutated, _ := json.Marshal(value)
			if _, err := ParseV2(mutated); err == nil {
				t.Fatal("unsafe development authority was accepted")
			}
		})
	}
}

func TestPackagedBuildAuthorityV2RejectsBuilderAndStagedPayloadDrift(t *testing.T) {
	baseline := authorityFixtureV2(false, ControlledDispositionKindV2)
	for name, mutate := range map[string]func(*AuthorityV2){
		"builder target": func(value *AuthorityV2) {
			value.BuildContext.Target.Key = "darwin-x64"
			value.BuildContext = sealEffectiveBuilderContextFixtureV1(value.BuildContext)
		},
		"builder context digest": func(value *AuthorityV2) {
			value.BuildContext.ContextDigest = digestFixtureV2("wrong-builder-context")
		},
		"fuse policy": func(value *AuthorityV2) {
			value.BuildContext.FusePolicySHA256 = digestFixtureV2("weaker-fuse-policy")
			value.BuildContext = sealEffectiveBuilderContextFixtureV1(value.BuildContext)
		},
		"unsorted target inventory": func(value *AuthorityV2) {
			value.BuildContext.Target.Targets = []string{"zip", "dmg"}
			value.BuildContext.Target = sealEffectiveBuilderTargetFixtureV1(value.BuildContext.Target)
			value.BuildContext = sealEffectiveBuilderContextFixtureV1(value.BuildContext)
		},
		"staged payload count": func(value *AuthorityV2) {
			value.StagedPayload.EntryCount++
		},
		"staged payload manifest": func(value *AuthorityV2) {
			value.StagedPayload.ManifestSHA256 = "INVALID"
		},
		"staged exclusion policy": func(value *AuthorityV2) {
			value.StagedPayload.ExclusionPolicySHA256 = digestFixtureV2("broader-exclusion-policy")
		},
		"funds plugin tree": func(value *AuthorityV2) {
			value.Artifacts.FundsPlugin.TreeSHA256 = "INVALID"
		},
		"worktree exclusion policy": func(value *AuthorityV2) {
			value.WorktreeSnapshot.ExclusionPolicySHA256 = digestFixtureV2("broader-worktree-exclusion-policy")
			value.WorktreeSnapshot.SnapshotDigest = snapshotDigestFixtureV2(value.WorktreeSnapshot)
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := baseline
			mutate(&value)
			value.AuthorityDigest = authorityDigest(value)
			body, _ := json.Marshal(value)
			if _, err := ParseV2(body); err == nil {
				t.Fatal("drifted build lifecycle authority was accepted")
			}
		})
	}
}

func TestPackagedBuildAuthorityV2RejectsDevelopmentNativeInventoryDrift(t *testing.T) {
	baseline := authorityFixtureV2(false, DevelopmentDispositionKindV2)
	for name, mutate := range map[string]func(*DevelopmentDispositionV2){
		"missing component": func(value *DevelopmentDispositionV2) {
			value.Components = value.Components[:len(value.Components)-1]
		},
		"component order": func(value *DevelopmentDispositionV2) {
			value.Components[0], value.Components[1] = value.Components[1], value.Components[0]
		},
		"component binary": func(value *DevelopmentDispositionV2) {
			value.Components[0].BinaryName = "substitute-native"
		},
		"component target": func(value *DevelopmentDispositionV2) {
			value.Components[0].Arch = "x64"
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := baseline
			var disposition DevelopmentDispositionV2
			if err := json.Unmarshal(value.NativeDisposition, &disposition); err != nil {
				t.Fatal(err)
			}
			mutate(&disposition)
			value.NativeDisposition, _ = json.Marshal(disposition)
			value.AuthorityDigest = authorityDigest(value)
			body, _ := json.Marshal(value)
			if _, err := ParseV2(body); err == nil {
				t.Fatal("drifted development native inventory was accepted")
			}
		})
	}
}

func authorityFixtureV2(dirty bool, dispositionKind string) AuthorityV2 {
	summary := ManifestSummaryV1{Count: 0, ByteLength: 0, SHA256: digestFixtureV2("empty")}
	worktree := WorktreeSnapshotV1{
		SchemaVersion: 1, Contract: worktreeSnapshotContractV1,
		SourceCommit: strings.Repeat("a", 40), State: "clean", Dirty: false,
		ExclusionPolicySHA256: worktreeExclusionSHA256V1, StagedPatch: summary,
		UnstagedPatch: summary, UntrackedSource: summary,
	}
	if dirty {
		worktree.State = "dirty"
		worktree.Dirty = true
		worktree.UntrackedSource = ManifestSummaryV1{Count: 1, ByteLength: 7, SHA256: digestFixtureV2("untracked")}
	}
	worktree.SnapshotDigest = snapshotDigestFixtureV2(worktree)

	var disposition any
	if dispositionKind == DevelopmentDispositionKindV2 {
		disposition = DevelopmentDispositionV2{
			Kind: DevelopmentDispositionKindV2, TargetKey: "darwin-arm64",
			Marker:     ContentArtifactBindingV2{SHA256: digestFixtureV2("marker"), ByteLength: 64},
			Components: developmentComponentFixturesV2(),
		}
	} else {
		disposition = ControlledReleaseDispositionV2{
			Kind: ControlledDispositionKindV2, TargetKey: "darwin-arm64",
			ReceiptSHA256: digestFixtureV2("receipt"), ManifestSHA256: digestFixtureV2("manifest"),
			SigningPolicySHA256: digestFixtureV2("signing-policy"), SigningMode: "developer-id",
			AppleTeamIdentifier: "ABCDE12345",
		}
	}
	rawDisposition, _ := json.Marshal(disposition)
	authority := AuthorityV2{
		SchemaVersion: 2, Contract: ContractV2, SourceCommit: worktree.SourceCommit,
		WorktreeSnapshot: worktree, Classification: classification(dirty, dispositionKind),
		Publishable: false, ReleaseEligible: false, PublicationReceiptIssued: false,
		TargetKey: "darwin-arm64", BuildContext: effectiveBuilderContextFixtureV1(),
		NativeDisposition: rawDisposition,
		Artifacts: ArtifactBindingsV2{
			Executable:    nativeFixtureV2("executable"),
			AppASAR:       ContentArtifactBindingV2{SHA256: digestFixtureV2("asar"), ByteLength: 4096},
			RuntimeServer: nativeFixtureV2("runtime"),
			FundsPlugin: FundsPluginArtifactBindingV2{
				TreeSHA256: digestFixtureV2("funds-plugin-tree"), FileCount: 12,
				ManifestSHA256:   digestFixtureV2("funds-plugin-manifest"),
				EntrypointSHA256: digestFixtureV2("funds-plugin-entrypoint"), TotalBytes: 8192,
			},
		},
		StagedPayload: StagedPayloadClosureV1{
			SchemaVersion: 1, Contract: stagedPayloadContractV1,
			ExclusionPolicySHA256: stagedPayloadExclusionSHA256V1,
			EntryCount:            8, DirectoryCount: 3, RegularFileCount: 2,
			NativeFileCount: 2, SymlinkCount: 1, ContentByteLength: 8192,
			ManifestSHA256: digestFixtureV2("staged-manifest"),
		},
	}
	authority.AuthorityDigest = authorityDigest(authority)
	return authority
}

func developmentComponentFixturesV2() []DevelopmentComponentBindingV2 {
	components := make([]DevelopmentComponentBindingV2, 0, len(developmentComponentsV2))
	for index, component := range developmentComponentsV2 {
		label := fmt.Sprintf("component-%d", index)
		components = append(components, DevelopmentComponentBindingV2{
			ID: component.ID, BinaryName: component.BinaryName,
			MarkerBinarySHA256: digestFixtureV2(label + "-binary"), MarkerBinaryByteLength: 1024,
			PayloadSHA256: digestFixtureV2(label + "-payload"), PayloadByteLength: 768,
			Format: "mach-o", Arch: "arm64",
		})
	}
	return components
}

func effectiveBuilderContextFixtureV1() EffectiveBuilderContextV1 {
	context := EffectiveBuilderContextV1{
		SchemaVersion: 1, Contract: effectiveBuilderContextV1,
		EffectiveConfigSHA256: digestFixtureV2("effective-config"),
		Target: EffectiveBuilderTargetV1{
			Key: "darwin-arm64", Platform: "darwin", Arch: "arm64",
			Targets: []string{"dmg", "zip"},
		},
		FusePolicyContract: electronFusePolicyContractV1,
		FusePolicySHA256:   electronFusePolicySHA256V1,
	}
	context.Target = sealEffectiveBuilderTargetFixtureV1(context.Target)
	return sealEffectiveBuilderContextFixtureV1(context)
}

func sealEffectiveBuilderTargetFixtureV1(value EffectiveBuilderTargetV1) EffectiveBuilderTargetV1 {
	without := struct {
		Key      string   `json:"key"`
		Platform string   `json:"platform"`
		Arch     string   `json:"arch"`
		Targets  []string `json:"targets"`
	}{value.Key, value.Platform, value.Arch, value.Targets}
	body, _ := json.Marshal(without)
	value.Digest = domainDigest(effectiveBuilderTargetDomain, body)
	return value
}

func sealEffectiveBuilderContextFixtureV1(value EffectiveBuilderContextV1) EffectiveBuilderContextV1 {
	without := struct {
		SchemaVersion         int                      `json:"schemaVersion"`
		Contract              string                   `json:"contract"`
		EffectiveConfigSHA256 string                   `json:"effectiveConfigSha256"`
		Target                EffectiveBuilderTargetV1 `json:"target"`
		FusePolicyContract    string                   `json:"fusePolicyContract"`
		FusePolicySHA256      string                   `json:"fusePolicySha256"`
	}{
		value.SchemaVersion, value.Contract, value.EffectiveConfigSHA256,
		value.Target, value.FusePolicyContract, value.FusePolicySHA256,
	}
	body, _ := json.Marshal(without)
	value.ContextDigest = domainDigest(effectiveBuilderContextDomain, body)
	return value
}

func nativeFixtureV2(label string) NativeArtifactBindingV2 {
	return NativeArtifactBindingV2{
		PreSignSHA256: digestFixtureV2(label + "-full"), PreSignByteLength: 2048,
		PayloadSHA256: digestFixtureV2(label + "-payload"), PayloadByteLength: 1536,
		Format: "mach-o", Arch: "arm64",
	}
}

func snapshotDigestFixtureV2(snapshot WorktreeSnapshotV1) string {
	without := struct {
		SchemaVersion         int               `json:"schemaVersion"`
		Contract              string            `json:"contract"`
		SourceCommit          string            `json:"sourceCommit"`
		State                 string            `json:"state"`
		Dirty                 bool              `json:"dirty"`
		ExclusionPolicySHA256 string            `json:"exclusionPolicySha256"`
		StagedPatch           ManifestSummaryV1 `json:"stagedPatch"`
		UnstagedPatch         ManifestSummaryV1 `json:"unstagedPatch"`
		UntrackedSource       ManifestSummaryV1 `json:"untrackedSource"`
	}{
		SchemaVersion: snapshot.SchemaVersion, Contract: snapshot.Contract,
		SourceCommit: snapshot.SourceCommit, State: snapshot.State, Dirty: snapshot.Dirty,
		ExclusionPolicySHA256: snapshot.ExclusionPolicySHA256, StagedPatch: snapshot.StagedPatch,
		UnstagedPatch: snapshot.UnstagedPatch, UntrackedSource: snapshot.UntrackedSource,
	}
	body, _ := json.Marshal(without)
	return domainDigest(worktreeDigestDomainV1, body)
}

func digestFixtureV2(label string) string {
	return AuthorityFileSHA256V2([]byte(label))
}

func TestCoreProfileRequiresAbsentFundsAndExactTarget(t *testing.T) {
	original := authorityFixtureV2(false, ControlledDispositionKindV2)
	core := original
	core.NativeDisposition = json.RawMessage(`{"kind":"core_no_professional_components","targetKey":"darwin-arm64"}`)
	core.Artifacts.FundsPlugin = FundsPluginArtifactBindingV2{}
	core.Classification = "development_clean_non_publishable"
	check := func(t *testing.T, value AuthorityV2, want bool) {
		t.Helper()
		value.AuthorityDigest = authorityDigest(value)
		body, _ := json.Marshal(value)
		parsed, err := ParseV2(body)
		if (err == nil) != want {
			t.Fatalf("core authority acceptance=%v, want=%v", err == nil, want)
		}
		if want && (parsed.Core == nil || parsed.Development != nil || parsed.Controlled != nil) {
			t.Fatal("core disposition promoted to native authority")
		}
	}
	check(t, core, true)
	mixed := core
	mixed.Artifacts.FundsPlugin = original.Artifacts.FundsPlugin
	check(t, mixed, false)
	absentFull := original
	absentFull.Artifacts.FundsPlugin = FundsPluginArtifactBindingV2{}
	check(t, absentFull, false)
	unknown := core
	unknown.NativeDisposition = json.RawMessage(`{"kind":"core_no_professional_components","targetKey":"linux-x64"}`)
	check(t, unknown, false)
	published := core
	published.Publishable = true
	check(t, published, false)
}

func TestControlledCoreQualificationIsNotPublication(t *testing.T) {
	base := authorityFixtureV2(false, ControlledDispositionKindV2)
	base.Artifacts.FundsPlugin = FundsPluginArtifactBindingV2{}
	disposition := CoreDispositionV2{Kind: CoreControlledDispositionKindV2, TargetKey: "darwin-arm64", SigningPolicySHA256: digestFixtureV2("policy"), SigningMode: "developer-id", AppleTeamIdentifier: "TESTTEAM01"}
	check := func(t *testing.T, d CoreDispositionV2, value AuthorityV2, want bool) {
		t.Helper()
		value.NativeDisposition, _ = json.Marshal(d)
		value.AuthorityDigest = authorityDigest(value)
		body, _ := json.Marshal(value)
		parsed, err := ParseV2(body)
		if (err == nil) != want {
			t.Fatalf("controlled Core accepted=%v want=%v", err == nil, want)
		}
		if want && (parsed.Core == nil || parsed.DeveloperIDTeam() != "TESTTEAM01" || parsed.Controlled != nil || parsed.Authority.Publishable) {
			t.Fatal("Core qualification crossed authority boundary")
		}
	}
	check(t, disposition, base, true)
	for _, mutate := range []func(*CoreDispositionV2){
		func(d *CoreDispositionV2) { d.Kind = "unknown" },
		func(d *CoreDispositionV2) { d.SigningMode = "ad-hoc" },
		func(d *CoreDispositionV2) { d.SigningPolicySHA256 = "" },
		func(d *CoreDispositionV2) { d.AppleTeamIdentifier = "" },
		func(d *CoreDispositionV2) { d.TargetKey = "darwin-x64" },
		func(d *CoreDispositionV2) { d.Kind = CoreDispositionKindV2 },
	} {
		d := disposition
		mutate(&d)
		check(t, d, base, false)
	}
	published := base
	published.Publishable = true
	check(t, disposition, published, false)
	development := base
	development.Classification = "development_clean_non_publishable"
	check(t, disposition, development, false)
	mixed := base
	mixed.Artifacts.FundsPlugin = authorityFixtureV2(false, ControlledDispositionKindV2).Artifacts.FundsPlugin
	check(t, disposition, mixed, false)
}
