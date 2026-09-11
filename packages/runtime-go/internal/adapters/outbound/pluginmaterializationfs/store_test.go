package pluginmaterializationfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const fixturePluginVersionV1 = "0.16.16"

type testInstallationAuthority struct{ private ed25519.PrivateKey }

func (authority testInstallationAuthority) KeyID() string {
	digest := sha256.Sum256(authority.PublicKey())
	return hex.EncodeToString(digest[:])
}
func (authority testInstallationAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.private.Public().(ed25519.PublicKey)...)
}
func (authority testInstallationAuthority) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ed25519.Sign(authority.private, body), nil
}

func TestMaterializeExactFundsPluginQuarantinesStaleGenerationAndKeepsFactsDisabled(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	legacy := filepath.Join(fixture.store.absolute(pluginParentRelativeV1(domainplugin.PluginNameV1)), "0.16.15")
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "legacy.txt"), []byte("preserve stale bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacySkill := writeManagedSkillProjection(t, fixture.runtimeHome, "quick-fact", domainplugin.PluginNameV1, "0.16.15", false)
	currentProjection := writeManagedSkillProjection(t, fixture.runtimeHome, "case-context", domainplugin.PluginNameV1, fixturePluginVersionV1, false)
	otherPluginProjection := writeManagedSkillProjection(t, fixture.runtimeHome, "data-quality", "other-plugin", "1.2.3", false)
	invalidProjection := writeManagedSkillProjection(t, fixture.runtimeHome, "account-dossier", domainplugin.PluginNameV1, "0.16.15", true)
	userSkill := filepath.Join(fixture.runtimeHome, "skills", "subject-dossier")
	if err := os.MkdirAll(userSkill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userSkill, "SKILL.md"), []byte("user-owned skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipt.PluginName != domainplugin.PluginNameV1 || result.Receipt.PluginVersion != fixturePluginVersionV1 ||
		result.Receipt.Target != fixture.intent.Target || result.Receipt.FactToolsEnabled || result.Index.FactToolsEnabled {
		t.Fatalf("materialization widened identity or fact authority: %#v %#v", result.Receipt, result.Index)
	}
	if err := domainplugin.ValidateTrustedReceiptV1(result.Receipt, fixture.authority.KeyID(), fixture.authority.PublicKey()); err != nil {
		t.Fatalf("receipt was not installation-authority signed: %v", err)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale 0.16.15 remained discoverable: %v", err)
	}
	if !treeContainsFile(t, fixture.store.absolute(controlRelativeV1+"/quarantine/"+fixture.intent.IntentID), "legacy.txt") {
		t.Fatal("stale 0.16.15 bytes were not preserved in non-discoverable quarantine")
	}
	if _, err := os.Stat(legacySkill); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale 0.16.15 global skill projection remained discoverable: %v", err)
	}
	if !treeContainsFileWithBody(t, fixture.store.absolute(controlRelativeV1+"/quarantine/"+fixture.intent.IntentID), "SKILL.md", "legacy skill quick-fact") {
		t.Fatal("stale 0.16.15 global skill bytes were not preserved in quarantine")
	}
	for label, path := range map[string]string{
		"current funds projection": currentProjection,
		"other plugin projection":  otherPluginProjection,
		"invalid projection":       invalidProjection,
		"unmarked user skill":      userSkill,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was moved outside the strict legacy ownership boundary: %v", label, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.source, "mcp", "server.mjs")); err != nil {
		t.Fatalf("caller source was deleted or moved: %v", err)
	}
	resolved, err := fixture.store.ResolveActive(context.Background(), fixture.authority)
	if err != nil || resolved.Receipt.ReceiptID != result.Receipt.ReceiptID || resolved.Index.IndexDigest != result.Index.IndexDigest {
		t.Fatalf("active materialization did not resolve exactly: err=%v result=%#v", err, resolved)
	}
	if err := fixture.store.requireSingleDiscoverableGeneration(domainplugin.PluginNameV1, fixturePluginVersionV1); err != nil {
		t.Fatal(err)
	}
	config, err := stableReadFile(
		filepath.Join(fixture.store.absolute(activeRelativeV1(domainplugin.PluginNameV1, fixturePluginVersionV1)), ".mcp.json"),
		maxMCPConfigBytesV1,
	)
	if err != nil || !strings.Contains(string(config), `"disabled":true`) {
		t.Fatalf("materializer changed disabled MCP policy: err=%v config=%s", err, config)
	}
}

func TestExistingActiveGenerationConvergesLaterLegacyFundsSkillProjection(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	first, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	legacySkill := writeManagedSkillProjection(t, fixture.runtimeHome, "quick-fact", domainplugin.PluginNameV1, "0.16.15", false)
	second, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.ReceiptID != second.Receipt.ReceiptID || first.Index.IndexDigest != second.Index.IndexDigest {
		t.Fatal("legacy projection convergence minted a new installation authority")
	}
	if _, err := os.Stat(legacySkill); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("late stale projection remained discoverable: %v", err)
	}
}

func TestVerifiedActiveGenerationRotatesOnlyForNewPackageAuthorityOnSameTarget(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	first, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	rotated := newIntentForIdentityWithPackageAuthority(
		t,
		fixture.source,
		mustInspectSourceTree(t, fixture.source),
		fixture.intent.Target,
		strings.Repeat("f", 64),
		fixture.now.Add(time.Minute),
	)
	second, err := fixture.store.Materialize(context.Background(), rotated, fixture.authority, fixture.now.Add(time.Minute))
	if err != nil {
		t.Fatalf("verified package authority rotation failed: %v", err)
	}
	if second.Receipt.ReceiptID == first.Receipt.ReceiptID || second.Receipt.GenerationID == first.Receipt.GenerationID ||
		second.Receipt.PackageAuthoritySHA256 != rotated.PackageAuthoritySHA256 || second.Receipt.Target != first.Receipt.Target ||
		second.Receipt.FactToolsEnabled || second.Index.FactToolsEnabled {
		t.Fatalf("package authority rotation widened or reused stale authority: first=%#v second=%#v", first, second)
	}
	quarantineRoot := fixture.store.absolute(controlRelativeV1 + "/quarantine/" + rotated.IntentID)
	if !treeContainsFileWithBody(t, quarantineRoot, "README.md", "fixture source\n") {
		t.Fatal("verified prior generation was not preserved in non-discoverable quarantine")
	}
	if err := fixture.store.requireSingleDiscoverableGeneration(domainplugin.PluginNameV1, fixturePluginVersionV1); err != nil {
		t.Fatal(err)
	}

	changedSource := filepath.Join(filepath.Dir(fixture.source), "changed-package-source")
	if err := copySourceTree(context.Background(), fixture.source, changedSource); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changedSource, "README.md"), []byte("changed source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedIdentity := mustInspectSourceTree(t, changedSource)
	contentDrift := newIntentForIdentityWithPackageAuthority(
		t, changedSource, changedIdentity, rotated.Target, rotated.PackageAuthoritySHA256, fixture.now.Add(2*time.Minute),
	)
	if _, err := fixture.store.Materialize(context.Background(), contentDrift, fixture.authority, fixture.now.Add(2*time.Minute)); !errors.Is(err, pluginport.ErrConflict) {
		t.Fatalf("content drift under the same package authority was not rejected: %v", err)
	}
	targetDrift := newIntentForIdentityWithPackageAuthority(
		t, fixture.source, mustInspectSourceTree(t, fixture.source),
		domainplugin.TargetV1{Platform: "windows", Arch: "amd64"}, strings.Repeat("e", 64), fixture.now.Add(3*time.Minute),
	)
	if _, err := fixture.store.Materialize(context.Background(), targetDrift, fixture.authority, fixture.now.Add(3*time.Minute)); !errors.Is(err, pluginport.ErrConflict) {
		t.Fatalf("cross-target package authority rotation was not rejected: %v", err)
	}
	resolved, err := fixture.store.ResolveActive(context.Background(), fixture.authority)
	if err != nil || resolved.Receipt.ReceiptID != second.Receipt.ReceiptID {
		t.Fatalf("rejected rotation changed the active generation: result=%#v err=%v", resolved, err)
	}
}

func TestOrdinaryInstalledMarkerCannotSelfMintDiscoverability(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	fixture.store.fault = func(point FaultPointV1) error {
		if point == FaultAfterGenerationCommittedV1 {
			return errors.New("simulated crash before signed index publication")
		}
		return nil
	}
	if _, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now); err == nil {
		t.Fatal("fault injection did not stop before active index")
	}
	if _, err := os.Stat(filepath.Join(
		fixture.store.absolute(activeRelativeV1(domainplugin.PluginNameV1, fixturePluginVersionV1)),
		domainplugin.InstallMarkerFileNameV1,
	)); err != nil {
		t.Fatalf("test did not reach ordinary marker state: %v", err)
	}
	if _, err := fixture.store.ResolveActive(context.Background(), fixture.authority); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary marker became discoverable without active signed index: %v", err)
	}
}

func TestMacOSArm64AndWindowsAMD64MaterializationFaultMatrix(t *testing.T) {
	points := []FaultPointV1{
		FaultAfterPreparedV1, FaultAfterStagedV1, FaultAfterAuthorizedV1,
		FaultAfterPriorQuarantinedV1, FaultAfterGenerationCommittedV1, FaultAfterIndexCommittedV1,
	}
	targets := []domainplugin.TargetV1{{Platform: "darwin", Arch: "arm64"}, {Platform: "windows", Arch: "amd64"}}
	for _, target := range targets {
		for _, point := range points {
			t.Run(target.Platform+"-"+target.Arch+"-"+string(point), func(t *testing.T) {
				fixture := newStoreFixture(t, target)
				triggered := false
				fixture.store.fault = func(candidate FaultPointV1) error {
					if candidate == point && !triggered {
						triggered = true
						return errors.New("simulated process crash")
					}
					return nil
				}
				if _, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now); err == nil || !triggered {
					t.Fatalf("fault point %s was not reached: %v", point, err)
				}
				recoveredStore, err := NewStore(fixture.runtimeHome, nil)
				if err != nil {
					t.Fatal(err)
				}
				result, err := recoveredStore.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now.Add(time.Minute))
				if err != nil {
					t.Fatalf("fault point %s did not recover deterministically: %v", point, err)
				}
				if result.Receipt.Target != target || result.Receipt.FactToolsEnabled || result.Index.FactToolsEnabled {
					t.Fatalf("recovery changed target or fact authority: %#v %#v", result.Receipt, result.Index)
				}
				if err := recoveredStore.requireSingleDiscoverableGeneration(domainplugin.PluginNameV1, fixturePluginVersionV1); err != nil {
					t.Fatal(err)
				}
				completed := filepath.Join(recoveredStore.absolute(controlRelativeV1+"/transactions/"+fixture.intent.IntentID), "07-completed.json")
				if _, err := os.Stat(completed); err != nil {
					t.Fatalf("recovered transaction was not completed: %v", err)
				}
			})
		}
	}
}

func TestPackageAuthorityRotationRecoversAcrossFaultMatrix(t *testing.T) {
	points := []FaultPointV1{
		FaultAfterPreparedV1, FaultAfterStagedV1, FaultAfterAuthorizedV1,
		FaultAfterPriorIndexQuarantinedV1, FaultAfterPriorGenerationQuarantinedV1,
		FaultAfterPriorQuarantinedV1, FaultAfterGenerationCommittedV1, FaultAfterIndexCommittedV1,
	}
	targets := []domainplugin.TargetV1{{Platform: "darwin", Arch: "arm64"}, {Platform: "windows", Arch: "amd64"}}
	for _, target := range targets {
		for _, point := range points {
			t.Run(target.Platform+"-"+target.Arch+"-"+string(point), func(t *testing.T) {
				fixture := newStoreFixture(t, target)
				first, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now)
				if err != nil {
					t.Fatal(err)
				}
				rotated := newIntentForIdentityWithPackageAuthority(
					t, fixture.source, mustInspectSourceTree(t, fixture.source), target,
					strings.Repeat("d", 64), fixture.now.Add(time.Minute),
				)
				triggered := false
				fixture.store.fault = func(candidate FaultPointV1) error {
					if candidate == point && !triggered {
						triggered = true
						return errors.New("simulated authority rotation crash")
					}
					return nil
				}
				if _, err := fixture.store.Materialize(context.Background(), rotated, fixture.authority, fixture.now.Add(time.Minute)); err == nil || !triggered {
					t.Fatalf("rotation fault point %s was not reached: %v", point, err)
				}
				recoveredStore, err := NewStore(fixture.runtimeHome, nil)
				if err != nil {
					t.Fatal(err)
				}
				result, err := recoveredStore.Materialize(context.Background(), rotated, fixture.authority, fixture.now.Add(2*time.Minute))
				if err != nil {
					t.Fatalf("rotation fault point %s did not recover: %v", point, err)
				}
				if result.Receipt.ReceiptID == first.Receipt.ReceiptID ||
					result.Receipt.PackageAuthoritySHA256 != rotated.PackageAuthoritySHA256 ||
					result.Receipt.Target != target || result.Receipt.FactToolsEnabled || result.Index.FactToolsEnabled {
					t.Fatalf("rotation recovery widened or reused authority: first=%#v result=%#v", first, result)
				}
				if err := recoveredStore.requireSingleDiscoverableGeneration(domainplugin.PluginNameV1, fixturePluginVersionV1); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestMaterializationRejectsEnabledMCPAndSymlinkedSource(t *testing.T) {
	t.Run("enabled MCP", func(t *testing.T) {
		root := realTempDir(t)
		source := writeFundsSource(t, root)
		if err := os.WriteFile(filepath.Join(source, ".mcp.json"), []byte(`{"mcpServers":{"analytix_funds":{"disabled":false,"enabled":true}}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectSourceTreeV1(context.Background(), source); err == nil || !strings.Contains(err.Error(), "remain disabled") {
			t.Fatalf("enabled MCP source was accepted: %v", err)
		}
	})
	t.Run("source symlink", func(t *testing.T) {
		root := realTempDir(t)
		source := writeFundsSource(t, root)
		if err := os.Symlink(filepath.Join(source, "mcp", "server.mjs"), filepath.Join(source, "alias.mjs")); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectSourceTreeV1(context.Background(), source); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("symlinked source was accepted: %v", err)
		}
	})
}

func TestInspectSourceTreeRequiresCanonicalPackageDeclaration(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		source := writeFundsSource(t, realTempDir(t))
		if err := os.Remove(filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectSourceTreeV1(context.Background(), source); err == nil {
			t.Fatal("source without .analytix-plugin/package.json was accepted")
		}
	})
	t.Run("non-regular", func(t *testing.T) {
		source := writeFundsSource(t, realTempDir(t))
		path := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectSourceTreeV1(context.Background(), source); err == nil {
			t.Fatal("source with a non-regular package declaration was accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		source := writeFundsSource(t, realTempDir(t))
		path := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(source, domainplugin.ManifestRelativePathV1), path); err != nil {
			t.Fatal(err)
		}
		if _, err := InspectSourceTreeV1(context.Background(), source); err == nil {
			t.Fatal("source with a symlinked package declaration was accepted")
		}
	})
	t.Run("changed-during-read", func(t *testing.T) {
		source := writeFundsSource(t, realTempDir(t))
		path := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
		if _, err := stableReadFileObserved(path, domainpluginpackage.MaxDeclarationBytesV1, func() {
			moved := path + ".moved"
			if renameErr := os.Rename(path, moved); renameErr != nil {
				t.Fatal(renameErr)
			}
			if writeErr := os.WriteFile(path, mustFundsDeclarationV1Bytes(t), 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}
		}); err == nil {
			t.Fatal("package declaration changed during stable read was accepted")
		}
	})

	validBody := mustFundsDeclarationV1Bytes(t)
	for name, body := range map[string][]byte{
		"unknown":   []byte(strings.Replace(string(validBody), "{", `{"unknown":true,`, 1)),
		"duplicate": []byte(strings.Replace(string(validBody), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)),
		"invalid":   []byte(strings.Replace(string(validBody), `"packageVersion":"0.16.16"`, `"packageVersion":"01.16.16"`, 1)),
		"unsafe-path": []byte(strings.Replace(
			string(validBody), `"entrypoint":"mcp/server.mjs"`, `"entrypoint":"../outside"`, 1,
		)),
		"missing-contribution": []byte(strings.Replace(
			string(validBody), `"entrypoint":"mcp/server.mjs"`, `"entrypoint":"mcp/missing.mjs"`, 1,
		)),
	} {
		t.Run(name, func(t *testing.T) {
			source := writeFundsSource(t, realTempDir(t))
			if err := os.WriteFile(
				filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1)),
				body,
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectSourceTreeV1(context.Background(), source); err == nil {
				t.Fatalf("source with %s package declaration was accepted", name)
			}
		})
	}
}

func TestInvalidPackageDeclarationStopsMaterializationBeforeEffects(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	declarationPath := filepath.Join(fixture.source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
	if err := os.WriteFile(declarationPath, []byte(`{"schemaVersion":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now); err == nil {
		t.Fatal("invalid package declaration reached materialization effects")
	}
	for _, path := range []string{
		fixture.store.activeIndexPath(),
		fixture.store.absolute(activeRelativeV1(domainplugin.PluginNameV1, fixturePluginVersionV1)),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid declaration created materialization state at %s: %v", path, err)
		}
	}
	for _, path := range []string{
		fixture.store.absolute(controlRelativeV1 + "/receipts"),
		fixture.store.absolute(controlRelativeV1 + "/transactions"),
	} {
		entries, err := os.ReadDir(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("invalid declaration created materialization records at %s: %#v", path, entries)
		}
	}
}

func TestProjectionParityFailureStopsMaterializationBeforeEffects(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	declarationPath := filepath.Join(fixture.source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
	staleProjection := bytes.Replace(
		mustFundsDeclarationV1Bytes(t),
		[]byte(`"packageVersion":"0.16.16"`),
		[]byte(`"packageVersion":"0.16.17"`),
		1,
	)
	if err := os.WriteFile(declarationPath, staleProjection, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now); err == nil ||
		!strings.Contains(err.Error(), "projection parity") {
		t.Fatalf("stale platform projection did not fail at the parity boundary: %v", err)
	}
	for _, path := range []string{
		fixture.store.activeIndexPath(),
		fixture.store.absolute(activeRelativeV1(domainplugin.PluginNameV1, fixturePluginVersionV1)),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("projection parity failure created materialization state at %s: %v", path, err)
		}
	}
	for _, path := range []string{
		fixture.store.absolute(controlRelativeV1 + "/receipts"),
		fixture.store.absolute(controlRelativeV1 + "/transactions"),
	} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("projection parity failure created materialization records at %s: %#v", path, entries)
		}
	}
}

func TestInspectSourceTreeRequiresExactCanonicalPlatformProjections(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"manifest version": func(t *testing.T, source string) {
			path := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
			body := bytes.Replace(
				mustFundsDeclarationV1Bytes(t),
				[]byte(`"packageVersion":"0.16.16"`),
				[]byte(`"packageVersion":"0.16.17"`),
				1,
			)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"MCP server id": func(t *testing.T, source string) {
			path := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
			body := bytes.Replace(mustFundsDeclarationV1Bytes(t), []byte(`"id":"analytix_funds"`), []byte(`"id":"analytix_other"`), 1)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"MCP command": func(t *testing.T, source string) {
			writeMCPConfigForProjectionTest(t, source, `{"disabled":true,"command":"bash","cwd":".","args":["./mcp/server.mjs"]}`)
		},
		"MCP working directory": func(t *testing.T, source string) {
			writeMCPConfigForProjectionTest(t, source, `{"disabled":true,"command":"node","cwd":"..","args":["./mcp/server.mjs"]}`)
		},
		"MCP extra argv": func(t *testing.T, source string) {
			writeMCPConfigForProjectionTest(t, source, `{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs","--unsafe"]}`)
		},
		"MCP entrypoint": func(t *testing.T, source string) {
			writeMCPConfigForProjectionTest(t, source, `{"disabled":true,"command":"node","cwd":".","args":["./mcp/other.mjs"]}`)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			source := writeFundsSource(t, realTempDir(t))
			mutate(t, source)
			if _, err := InspectSourceTreeV1(context.Background(), source); err == nil ||
				!strings.Contains(err.Error(), "projection parity") {
				t.Fatalf("%s projection drift was accepted: %v", name, err)
			}
		})
	}
}

func TestRequestedCapabilitiesDoNotMapToPlatformCapabilities(t *testing.T) {
	source := writeFundsSource(t, realTempDir(t))
	manifestPath := filepath.Join(source, filepath.FromSlash(domainplugin.ManifestRelativePathV1))
	if err := os.WriteFile(
		manifestPath,
		[]byte(`{"name":"analytix-fund-analysis","version":"0.16.16","interface":{"capabilities":["Interactive","Read"]}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSourceTreeV1(context.Background(), source); err != nil {
		t.Fatalf("platform metadata was incorrectly treated as a capability grant mapping: %v", err)
	}
}

func TestSourceTreeInspectionDoesNotOwnPublisherExactVersion(t *testing.T) {
	source := writeFundsSource(t, realTempDir(t))
	declarationPath := filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
	declarationBody, err := os.ReadFile(declarationPath)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domainpluginpackage.ParseDeclarationV1(declarationBody)
	if err != nil {
		t.Fatal(err)
	}
	declaration.PackageVersion = "0.16.17"
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(declarationPath, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(source, filepath.FromSlash(domainplugin.ManifestRelativePathV1))
	if err := os.WriteFile(
		manifestPath,
		[]byte(`{"name":"analytix-fund-analysis","version":"0.16.17"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	identity, err := InspectSourceTreeV1(context.Background(), source)
	if err != nil {
		t.Fatalf("internally consistent publisher version was rejected before Host admission: %v", err)
	}
	if identity.Declaration.PackageID != domainplugin.PluginNameV1 || identity.Declaration.PackageVersion != "0.16.17" {
		t.Fatalf("inspected declaration identity drifted: %#v", identity.Declaration)
	}
}

func TestPackageDeclarationIsRevalidatedBeforeExistingMaterializationFastPath(t *testing.T) {
	fixture := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	if _, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now); err != nil {
		t.Fatal(err)
	}
	declarationPath := filepath.Join(fixture.source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1))
	if err := os.WriteFile(declarationPath, []byte(`{"schemaVersion":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now.Add(time.Second)); err == nil {
		t.Fatal("existing materialization fast path accepted an invalid package declaration")
	}
}

func TestCurrentFundsSourceSatisfiesMacOSArm64MaterializationContract(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(file), "../../../../../.."))
	if err != nil {
		t.Fatal(err)
	}
	source, err := filepath.EvalSymlinks(filepath.Join(repoRoot, "plugins", domainplugin.PluginNameV1))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := InspectSourceTreeV1(context.Background(), source)
	if err != nil {
		t.Fatalf("current 0.16.16 source failed formal content validation: %v", err)
	}
	declarationBody, err := os.ReadFile(filepath.Join(source, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1)))
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domainpluginpackage.ParseDeclarationV1(declarationBody)
	if err != nil {
		t.Fatalf("current package declaration did not parse: %v", err)
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil || !bytes.Equal(bytes.TrimSpace(declarationBody), canonical) {
		t.Fatalf("current package declaration does not match its canonical semantics: err=%v", err)
	}
	rawDigest := sha256.Sum256(declarationBody)
	canonicalDigest := sha256.Sum256(canonical)
	if identity.Declaration.PackageID != domainplugin.PluginNameV1 ||
		identity.Declaration.PackageVersion != fixturePluginVersionV1 ||
		identity.Declaration.RawSHA256 != hex.EncodeToString(rawDigest[:]) ||
		identity.Declaration.CanonicalSHA256 != hex.EncodeToString(canonicalDigest[:]) {
		t.Fatalf("current package declaration identity drifted: %#v", identity.Declaration)
	}
	assertCurrentFundsDeclarationV1(t, source, declaration)
	runtimeHome := realTempDir(t)
	store, err := NewStore(runtimeHome, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC)
	intent := newIntentForIdentity(t, source, identity, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}, now)
	authority := newTestAuthority()
	result, err := store.Materialize(context.Background(), intent, authority, now)
	if err != nil {
		t.Fatalf("current source did not materialize through macOS arm64 contract: %v", err)
	}
	if result.Receipt.SourceTreeFileCount != identity.FileCount || result.Receipt.SourceTreeSHA256 != identity.TreeSHA256 {
		t.Fatalf("complete current source identity drifted: receipt=%#v source=%#v", result.Receipt, identity)
	}
}

func assertCurrentFundsDeclarationV1(
	t *testing.T,
	source string,
	declaration domainpluginpackage.DeclarationV1,
) {
	t.Helper()
	if declaration.IdentityV1() != (domainpluginpackage.PackageIdentityV1{
		PackageID: domainplugin.PluginNameV1, PackageVersion: fixturePluginVersionV1,
	}) {
		t.Fatalf("current declaration package identity drifted: %#v", declaration.IdentityV1())
	}
	declaredSkills := make(map[string]string, len(declaration.Contributions.Skills))
	for _, contribution := range declaration.Contributions.Skills {
		declaredSkills[contribution.ID] = contribution.Path
	}
	skillDirectories, err := os.ReadDir(filepath.Join(source, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	actualSkills := make(map[string]string, len(skillDirectories))
	for _, entry := range skillDirectories {
		if !entry.IsDir() {
			continue
		}
		relative := filepath.ToSlash(filepath.Join("skills", entry.Name(), "SKILL.md"))
		if info, statErr := os.Lstat(filepath.Join(source, filepath.FromSlash(relative))); statErr == nil && info.Mode().IsRegular() {
			actualSkills[entry.Name()] = relative
		}
	}
	if !reflect.DeepEqual(declaredSkills, actualSkills) {
		t.Fatalf("current skill declaration does not match the first-party package tree: declared=%#v actual=%#v", declaredSkills, actualSkills)
	}
	if !reflect.DeepEqual(declaration.Contributions.MCPServers, []domainpluginpackage.MCPServerContributionV1{
		{ID: "analytix_funds", Entrypoint: "mcp/server.mjs"},
	}) || len(declaration.Contributions.Hooks) != 0 ||
		!reflect.DeepEqual(declaration.Contributions.Assets, []domainpluginpackage.PathContributionV1{
			{ID: "funds-icon", Path: "assets/icon.png"},
			{ID: "funds-logo", Path: "assets/logo.png"},
		}) || !reflect.DeepEqual(declaration.Contributions.PublicUI, []domainpluginpackage.PathContributionV1{
		{ID: "openai-agent-ui", Path: "agents/openai.yaml"},
	}) {
		t.Fatalf("current non-skill contribution declaration drifted: %#v", declaration.Contributions)
	}
	if !reflect.DeepEqual(declaration.RequestedCapabilities, []domainpluginpackage.CapabilityRequestV1{
		{ID: "funds.case.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
		{ID: "funds.source.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
	}) || declaration.Lifecycle != (domainpluginpackage.LifecycleV1{
		ProtocolVersion: 1, EntryPolicy: "host-static-first-party",
	}) {
		t.Fatalf("current capability request or lifecycle declaration drifted: requests=%#v lifecycle=%#v", declaration.RequestedCapabilities, declaration.Lifecycle)
	}
}

func TestExternalStagedFundsArtifactMatchesSourceCanonicalEvidence(t *testing.T) {
	stagedRoot := os.Getenv("ANALYTIX_TEST_STAGED_FUNDS_PLUGIN_ROOT")
	authorityPath := os.Getenv("ANALYTIX_TEST_PACKAGED_BUILD_AUTHORITY")
	if stagedRoot == "" && authorityPath == "" {
		t.Skip("external staged package evidence was not requested")
	}
	if stagedRoot == "" || authorityPath == "" {
		t.Fatal("external staged package evidence paths must be supplied together")
	}
	stagedRoot, err := filepath.EvalSymlinks(stagedRoot)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := InspectSourceTreeV1(context.Background(), stagedRoot)
	if err != nil {
		t.Fatalf("staged Funds artifact failed production inspection: %v", err)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository source")
	}
	repoRoot, err := filepath.Abs(filepath.Join(filepath.Dir(file), "../../../../../.."))
	if err != nil {
		t.Fatal(err)
	}
	sourceDeclaration, err := os.ReadFile(filepath.Join(
		repoRoot, "plugins", domainplugin.PluginNameV1, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1),
	))
	if err != nil {
		t.Fatal(err)
	}
	stagedDeclaration, err := os.ReadFile(filepath.Join(stagedRoot, filepath.FromSlash(domainpluginpackage.DeclarationRelativePathV1)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stagedDeclaration, sourceDeclaration) {
		t.Fatal("staged package declaration bytes differ from canonical source bytes")
	}
	parsedDeclaration, err := domainpluginpackage.ParseDeclarationV1(stagedDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainpluginpackage.CanonicalDeclarationV1Bytes(parsedDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	rawDigest := sha256.Sum256(stagedDeclaration)
	canonicalDigest := sha256.Sum256(canonical)
	if identity.Declaration.RawSHA256 != hex.EncodeToString(rawDigest[:]) ||
		identity.Declaration.CanonicalSHA256 != hex.EncodeToString(canonicalDigest[:]) {
		t.Fatalf("staged declaration evidence drifted: %#v", identity.Declaration)
	}
	authorityBody, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := domainauthority.ParseV2(authorityBody)
	if err != nil {
		t.Fatalf("packaged build authority is invalid: %v", err)
	}
	binding := authority.Authority.Artifacts.FundsPlugin
	if binding.TreeSHA256 != identity.TreeSHA256 || binding.FileCount != int64(identity.FileCount) ||
		binding.ManifestSHA256 != identity.ManifestSHA256 || binding.EntrypointSHA256 != identity.EntrypointSHA256 ||
		binding.TotalBytes != identity.TotalBytes {
		t.Fatalf("packaged receipt does not bind the inspected Funds artifact: binding=%#v identity=%#v", binding, identity)
	}
}

type storeFixture struct {
	runtimeHome string
	source      string
	store       *Store
	intent      domainplugin.IntentV1
	authority   testInstallationAuthority
	now         time.Time
}

func newStoreFixture(t *testing.T, target domainplugin.TargetV1) storeFixture {
	t.Helper()
	root := realTempDir(t)
	runtimeHome := filepath.Join(root, "runtime-home")
	if err := os.Mkdir(runtimeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	source := writeFundsSource(t, root)
	identity, err := InspectSourceTreeV1(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)
	intent := newIntentForIdentity(t, source, identity, target, now)
	store, err := NewStore(runtimeHome, nil)
	if err != nil {
		t.Fatal(err)
	}
	return storeFixture{runtimeHome: runtimeHome, source: source, store: store, intent: intent, authority: newTestAuthority(), now: now}
}

func newIntentForIdentity(t *testing.T, source string, identity SourceTreeIdentityV1, target domainplugin.TargetV1, now time.Time) domainplugin.IntentV1 {
	t.Helper()
	packageDigest := sha256.Sum256([]byte("formal-package-authority:" + target.Platform + "/" + target.Arch))
	return newIntentForIdentityWithPackageAuthority(t, source, identity, target, sha256HexLocal(packageDigest[:]), now)
}

func newIntentForIdentityWithPackageAuthority(
	t *testing.T,
	source string,
	identity SourceTreeIdentityV1,
	target domainplugin.TargetV1,
	packageAuthoritySHA256 string,
	now time.Time,
) domainplugin.IntentV1 {
	t.Helper()
	intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		PackageAuthoritySHA256: packageAuthoritySHA256, Target: target,
		PluginName: identity.Declaration.PackageID, PluginVersion: identity.Declaration.PackageVersion, SourceRoot: source,
		SourceTreeSHA256: identity.TreeSHA256, SourceTreeFileCount: identity.FileCount,
		ManifestSHA256: identity.ManifestSHA256, EntrypointSHA256: identity.EntrypointSHA256, RequestedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func mustInspectSourceTree(t *testing.T, source string) SourceTreeIdentityV1 {
	t.Helper()
	identity, err := InspectSourceTreeV1(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func writeFundsSource(t *testing.T, parent string) string {
	t.Helper()
	source := filepath.Join(parent, "packaged-funds-source")
	if err := os.MkdirAll(filepath.Join(source, ".codex-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, ".analytix-plugin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "mcp"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		domainpluginpackage.DeclarationRelativePathV1: mustFundsDeclarationV1Bytes(t),
		".codex-plugin/plugin.json":                   []byte(`{"name":"analytix-fund-analysis","version":"0.16.16"}`),
		".mcp.json":                                   []byte(`{"mcpServers":{"analytix_funds":{"disabled":true,"command":"node","cwd":".","args":["./mcp/server.mjs"]}}}`),
		"mcp/server.mjs":                              []byte("export const factsEnabled = false\n"),
		"README.md":                                   []byte("fixture source\n"),
		"empty.txt":                                   {},
	}
	for relative, body := range files {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(relative)), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	real, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func writeMCPConfigForProjectionTest(t *testing.T, source, server string) {
	t.Helper()
	if err := os.WriteFile(
		filepath.Join(source, ".mcp.json"),
		[]byte(`{"mcpServers":{"analytix_funds":`+server+`}}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

func mustFundsDeclarationV1Bytes(t *testing.T) []byte {
	t.Helper()
	body, err := domainpluginpackage.CanonicalDeclarationV1Bytes(domainpluginpackage.DeclarationV1{
		SchemaVersion:  domainpluginpackage.SchemaVersionV1,
		PackageID:      domainplugin.PluginNameV1,
		PackageVersion: fixturePluginVersionV1,
		Contributions: domainpluginpackage.ContributionsV1{
			Skills:     []domainpluginpackage.PathContributionV1{},
			MCPServers: []domainpluginpackage.MCPServerContributionV1{{ID: "analytix_funds", Entrypoint: "mcp/server.mjs"}},
			Hooks:      []domainpluginpackage.PathContributionV1{},
			Assets:     []domainpluginpackage.PathContributionV1{},
			PublicUI:   []domainpluginpackage.PathContributionV1{},
		},
		RequestedCapabilities: []domainpluginpackage.CapabilityRequestV1{
			{ID: "funds.case.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
			{ID: "funds.source.read", ProtocolVersion: 1, ScopeConstraints: []string{"case:bound", "source:verified"}},
		},
		Lifecycle: domainpluginpackage.LifecycleV1{ProtocolVersion: 1, EntryPolicy: "host-static-first-party"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func realTempDir(t *testing.T) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func newTestAuthority() testInstallationAuthority {
	seed := sha256.Sum256([]byte("bundled-plugin-installation-authority"))
	return testInstallationAuthority{private: ed25519.NewKeyFromSeed(seed[:])}
}

func treeContainsFile(t *testing.T, root, base string) bool {
	t.Helper()
	found := false
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == base {
			found = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return found
}

func treeContainsFileWithBody(t *testing.T, root, base, expected string) bool {
	t.Helper()
	found := false
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != base {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(body) == expected {
			found = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return found
}

func writeManagedSkillProjection(t *testing.T, runtimeHome, skillName, pluginName, version string, extraKey bool) string {
	t.Helper()
	root := filepath.Join(runtimeHome, "skills", skillName)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("legacy skill "+skillName), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := map[string]any{
		"managedBy": "analytix-hub", "platform": "darwin-arm64", "pluginName": pluginName,
		"skillName": skillName, "skillPath": "skills/" + skillName + "/SKILL.md",
		"sourceKind": "plugin", "version": version,
	}
	if extraKey {
		marker["unexpected"] = true
	}
	body, err := json.Marshal(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, managedHubSkillProjectionMarkerFileNameV1), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

var _ pluginport.InstallationAuthority = testInstallationAuthority{}
