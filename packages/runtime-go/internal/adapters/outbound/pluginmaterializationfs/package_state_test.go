package pluginmaterializationfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

// These synthetic packages exercise the storage boundary only. The existing
// tree inspector still requires Funds-shaped MCP; this is not Office admission.
func packageStateFixtureV1(t *testing.T, runtimeHome, packageID string) storeFixture {
	t.Helper()
	source := writeFundsSource(t, realTempDir(t))
	declaration, err := domainpackage.ParseDeclarationV1(mustFundsDeclarationV1Bytes(t))
	if err != nil {
		t.Fatal(err)
	}
	declaration.PackageID = packageID
	declaration.RequestedCapabilities = []domainpackage.CapabilityRequestV1{}
	body, err := domainpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, domainpackage.DeclarationRelativePathV1), body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]string{"name": packageID, "version": fixturePluginVersionV1})
	if err := os.WriteFile(filepath.Join(source, domainplugin.ManifestRelativePathV1), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	identity := mustInspectSourceTree(t, source)
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	intent := newIntentForIdentity(t, source, identity, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}, now)
	store, err := NewPackageStoreV1(runtimeHome, packageID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return storeFixture{runtimeHome: runtimeHome, source: source, store: store, intent: intent, authority: newTestAuthority(), now: now}
}

func materializePackageStateFixtureV1(t *testing.T, fixture storeFixture) pluginport.ResultV1 {
	t.Helper()
	result, err := fixture.store.Materialize(context.Background(), fixture.intent, fixture.authority, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPackageStoresCoexistWithLegacyFundsAndPersistDisabledV1(t *testing.T) {
	funds := newStoreFixture(t, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	fundsResult := materializePackageStateFixtureV1(t, funds)
	fundsIndex, err := os.ReadFile(funds.store.activeIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, packageID := range []string{"synthetic-documents", "synthetic-spreadsheets", "synthetic-presentations"} {
		fixture := packageStateFixtureV1(t, funds.runtimeHome, packageID)
		result := materializePackageStateFixtureV1(t, fixture)
		if fixture.store.activeIndexPath() == funds.store.activeIndexPath() {
			t.Fatal("package shares legacy index")
		}
		if _, err := fixture.store.ReadActivation(context.Background(), fixture.authority); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing activation was not unavailable: %v", err)
		}
		state, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
			GenerationID: result.Receipt.GenerationID, DesiredState: domainplugin.DesiredDisabledV1,
		}, fixture.authority, fixture.now)
		if err != nil || state.Revision != 1 {
			t.Fatalf("disable did not commit: %v", err)
		}
		reopened, err := OpenExistingPackageStoreV1(funds.runtimeHome, packageID)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := reopened.ReadActivation(context.Background(), fixture.authority)
		if err != nil || restored != state || restored.DesiredState != domainplugin.DesiredDisabledV1 {
			t.Fatalf("disabled state not restored: %v", err)
		}
		if _, err := reopened.Materialize(context.Background(), funds.intent, funds.authority, funds.now); !errors.Is(err, pluginport.ErrInvalid) {
			t.Fatalf("accepted another package intent: %v", err)
		}
	}
	unchanged, err := os.ReadFile(funds.store.activeIndexPath())
	if err != nil || !bytes.Equal(fundsIndex, unchanged) {
		t.Fatalf("Office storage changed Funds index: %v", err)
	}
	legacy, err := OpenExistingStoreV1(funds.runtimeHome)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := legacy.ResolveActive(context.Background(), funds.authority)
	if err != nil || resolved != fundsResult {
		t.Fatalf("legacy Funds did not resolve unchanged: %v", err)
	}
	scopedFunds, err := OpenExistingPackageStoreV1(funds.runtimeHome, domainplugin.PluginNameV1)
	if err != nil || scopedFunds.activeIndexPath() != legacy.activeIndexPath() || scopedFunds.mu != legacy.mu {
		t.Fatalf("Funds acquired a second owner: %v", err)
	}
	for _, invalid := range []string{"", "../other", "synthetic/documents", " synthetic-documents"} {
		if _, err := NewPackageStoreV1(funds.runtimeHome, invalid, nil); !errors.Is(err, pluginport.ErrInvalid) {
			t.Fatalf("accepted invalid package id %q: %v", invalid, err)
		}
	}
}

func TestPackageActivationTwoInstancesSerializeCASV1(t *testing.T) {
	fixture := packageStateFixtureV1(t, realTempDir(t), "synthetic-documents")
	result := materializePackageStateFixtureV1(t, fixture)
	other, err := OpenExistingPackageStoreV1(fixture.runtimeHome, fixture.intent.PluginName)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsOut := make(chan error, 2)
	var workers sync.WaitGroup
	for _, store := range []*Store{fixture.store, other} {
		workers.Add(1)
		go func(store *Store) {
			defer workers.Done()
			<-start
			_, err := store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
				GenerationID: result.Receipt.GenerationID, ExpectedRevision: 0, DesiredState: domainplugin.DesiredEnabledV1,
			}, fixture.authority, fixture.now)
			errorsOut <- err
		}(store)
	}
	close(start)
	workers.Wait()
	close(errorsOut)
	succeeded, conflicts := 0, 0
	for err := range errorsOut {
		if err == nil {
			succeeded++
		} else if errors.Is(err, pluginport.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicts != 1 {
		t.Fatalf("CAS winners=%d conflicts=%d", succeeded, conflicts)
	}
	state, err := other.ReadActivation(context.Background(), fixture.authority)
	if err != nil || state.Revision != 1 {
		t.Fatalf("unexpected activation: %v", err)
	}
	disabled, err := other.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
		GenerationID: result.Receipt.GenerationID, ExpectedRevision: 1, DesiredState: domainplugin.DesiredDisabledV1,
	}, fixture.authority, fixture.now.Add(time.Second))
	if err != nil || disabled.Revision != 2 {
		t.Fatalf("current CAS failed: %v", err)
	}
	if _, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
		GenerationID: result.Receipt.GenerationID, ExpectedRevision: 1, DesiredState: domainplugin.DesiredEnabledV1,
	}, fixture.authority, fixture.now.Add(2*time.Second)); !errors.Is(err, pluginport.ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
}

func TestPackageActivationRejectsOldGenerationAndBadSignatureV1(t *testing.T) {
	fixture := packageStateFixtureV1(t, realTempDir(t), "synthetic-documents")
	first := materializePackageStateFixtureV1(t, fixture)
	oldActivation, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
		GenerationID: first.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1,
	}, fixture.authority, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.intent = newIntentForIdentityWithPackageAuthority(t, fixture.source, mustInspectSourceTree(t, fixture.source),
		fixture.intent.Target, strings.Repeat("d", 64), fixture.now.Add(time.Minute))
	second := materializePackageStateFixtureV1(t, fixture)
	if first.Receipt.GenerationID == second.Receipt.GenerationID {
		t.Fatal("fixture did not rotate generation")
	}
	if _, err := fixture.store.ReadActivation(context.Background(), fixture.authority); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inherited old activation: %v", err)
	}
	if _, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
		GenerationID: first.Receipt.GenerationID, ExpectedRevision: 1, DesiredState: domainplugin.DesiredDisabledV1,
	}, fixture.authority, fixture.now); !errors.Is(err, pluginport.ErrConflict) {
		t.Fatalf("old generation accepted: %v", err)
	}
	currentPath := fixture.store.activationPathV1(second.Receipt.GenerationID)
	oldBody, _ := domainplugin.ActivationV1Bytes(oldActivation)
	if err := os.WriteFile(currentPath, oldBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ReadActivation(context.Background(), fixture.authority); !errors.Is(err, pluginport.ErrCorrupt) {
		t.Fatalf("copied old receipt accepted: %v", err)
	}
	if err := os.Remove(currentPath); err != nil {
		t.Fatal(err)
	}
	current, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
		GenerationID: second.Receipt.GenerationID, DesiredState: domainplugin.DesiredDisabledV1,
	}, fixture.authority, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	current.AuthoritySignature = base64.RawURLEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	bad, _ := json.Marshal(current)
	if err := os.WriteFile(currentPath, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ReadActivation(context.Background(), fixture.authority); !errors.Is(err, pluginport.ErrCorrupt) {
		t.Fatalf("bad signature accepted: %v", err)
	}
	if _, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
		GenerationID: second.Receipt.GenerationID, ExpectedRevision: 1, DesiredState: domainplugin.DesiredEnabledV1,
	}, fixture.authority, fixture.now); !errors.Is(err, pluginport.ErrCorrupt) {
		t.Fatalf("bad state overwritten: %v", err)
	}
}

func TestPackageActivationFailureReceiptsAreHonestV1(t *testing.T) {
	for _, point := range []FaultPointV1{FaultBeforeActivationCommittedV1, FaultAfterActivationCommittedV1} {
		t.Run(string(point), func(t *testing.T) {
			fixture := packageStateFixtureV1(t, realTempDir(t), "synthetic-documents")
			result := materializePackageStateFixtureV1(t, fixture)
			disabled, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
				GenerationID: result.Receipt.GenerationID, DesiredState: domainplugin.DesiredDisabledV1,
			}, fixture.authority, fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("synthetic activation commit fault")
			fixture.store.fault = func(at FaultPointV1) error {
				if at == point {
					return injected
				}
				return nil
			}
			state, err := fixture.store.SetDesiredState(context.Background(), pluginport.SetDesiredStateRequestV1{
				GenerationID: result.Receipt.GenerationID, ExpectedRevision: 1, DesiredState: domainplugin.DesiredEnabledV1,
			}, fixture.authority, fixture.now.Add(time.Second))
			if !errors.Is(err, injected) || state != (domainplugin.ActivationV1{}) {
				t.Fatalf("failed mutation returned success: %v", err)
			}
			reopened, err := OpenExistingPackageStoreV1(fixture.runtimeHome, fixture.intent.PluginName)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := reopened.ReadActivation(context.Background(), fixture.authority)
			if err != nil {
				t.Fatal(err)
			}
			if point == FaultBeforeActivationCommittedV1 && actual != disabled {
				t.Fatal("pre-commit failure changed durable state")
			}
			if point == FaultAfterActivationCommittedV1 && (actual.Revision != 2 || actual.DesiredState != domainplugin.DesiredEnabledV1) {
				t.Fatal("post-commit recovery did not read actual durable state")
			}
		})
	}
}
