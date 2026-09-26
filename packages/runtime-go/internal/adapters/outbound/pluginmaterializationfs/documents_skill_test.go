package pluginmaterializationfs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	hostport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func TestOfficeSkillReaderAuthenticatesInstalledBytesAndGeneration(t *testing.T) {
	ctx := context.Background()
	source := writeDevelopmentSourceV1(t, "analytix-documents")
	declarationPath := filepath.Join(source, domainpackage.DeclarationRelativePathV1)
	body, err := os.ReadFile(declarationPath)
	if err != nil {
		t.Fatal(err)
	}
	var declaration domainpackage.DeclarationV1
	if err = json.Unmarshal(body, &declaration); err != nil {
		t.Fatal(err)
	}
	declaration.Contributions.Skills = []domainpackage.PathContributionV1{{ID: domainpackage.DocumentsSkillContributionIDV1, Path: domainpackage.DocumentsSkillRelativePathV1}}
	declaration.RequestedCapabilities = append(declaration.RequestedCapabilities, domainpackage.CapabilityRequestV1{ID: "office.document-generation", ProtocolVersion: 1, ScopeConstraints: []string{"new-file", "current-conversation"}})
	body, err = domainpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(declarationPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(source, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1))
	if err = os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
		t.Fatal(err)
	}
	instructions := []byte("---\nname: documents\ndescription: Synthetic generation.\n---\nCreate a report using generate_office_document.")
	if err = os.WriteFile(entry, instructions, 0600); err != nil {
		t.Fatal(err)
	}
	observed, err := InspectDevelopmentSourceTreeV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	if err != nil {
		t.Fatal(err)
	}
	binding := developmentBindingV1(t, source)
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	intent, err := binding.NewIntentV1(now)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPackageStoreV1(realTempDir(t), "analytix-documents", nil)
	if err != nil {
		t.Fatal(err)
	}
	authority := newTestAuthority()
	service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Materialize(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	expected := hostport.Binding{PackageID: registration.Identity.PackageID, PackageVersion: registration.Identity.PackageVersion, GenerationID: result.Receipt.GenerationID, ActivationRevision: 1, SourceRegistrationSHA256: observed.SourceRegistrationSHA256}
	reader := OfficeSkillReader{Store: store, Authority: authority, SHA256: registration.DocumentsSkillSHA256}
	snapshot, err := reader.ReadSkill(ctx, expected)
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok := snapshot.File("SKILL.md")
	if !ok || string(loaded) != string(instructions) {
		t.Fatal("wrong installed instructions")
	}
	stale := expected
	stale.GenerationID = "stale"
	if _, err = reader.ReadSkill(ctx, stale); err == nil {
		t.Fatal("stale generation accepted")
	}
	// Editing the source after installation cannot replace the installed body.
	if err = os.WriteFile(entry, []byte("Changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadSkill(ctx, expected); err != nil {
		t.Fatal("source became runtime authority", err)
	}
	installed := filepath.Join(store.absolute(result.Receipt.ActiveRelativePath), filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1))
	if err = os.WriteFile(installed, []byte("Tampered installed instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadSkill(ctx, expected); err == nil {
		t.Fatal("tampered instructions accepted")
	}
}

type officeSkillLifecycleIdentity struct{ principal domainidentity.PrincipalV1 }

func (i officeSkillLifecycleIdentity) ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error) {
	return i.principal, nil
}

func (i officeSkillLifecycleIdentity) ValidateCurrent(_ context.Context, principal domainidentity.PrincipalV1) error {
	if !domainidentity.SamePrincipalV1(i.principal, principal) {
		return errors.New("synthetic identity mismatch")
	}
	return nil
}

// This crosses real private filesystem materialization, signed activation, the
// installed skill reader and Host consumption. Reopening is not a desktop
// restart, and generation replacement is not a nonexistent Host uninstall API.
func TestOfficeSkillHostMaterializationLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	home, source := realTempDir(t), writeDevelopmentSourceV1(t, "analytix-documents")
	authority := newTestAuthority()
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	principal, err := domainidentity.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	declarationPath := filepath.Join(source, domainpackage.DeclarationRelativePathV1)
	body, err := os.ReadFile(declarationPath)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domainpackage.ParseDeclarationV1(body)
	if err != nil {
		t.Fatal(err)
	}
	declaration.Contributions.Skills = []domainpackage.PathContributionV1{{ID: domainpackage.DocumentsSkillContributionIDV1, Path: domainpackage.DocumentsSkillRelativePathV1}}
	declaration.RequestedCapabilities = append(declaration.RequestedCapabilities, domainpackage.CapabilityRequestV1{ID: "office.document-generation", ProtocolVersion: 1, ScopeConstraints: []string{"new-file", "current-conversation"}})
	body, err = domainpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(declarationPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(source, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1))
	if err := os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
		t.Fatal(err)
	}
	writeSkill := func(revision string) {
		t.Helper()
		instructions := "---\nname: documents\ndescription: Synthetic lifecycle " + revision + ".\n---\nCreate a report using generate_office_document. " + revision
		if err := os.WriteFile(entry, []byte(instructions), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeSkill("first")
	store, err := NewPackageStoreV1(home, "analytix-documents", nil)
	if err != nil {
		t.Fatal(err)
	}
	compose := func(store *Store, install bool) (*hostapp.Service, pluginport.ResultV1) {
		t.Helper()
		observed, err := InspectDevelopmentSourceTreeV1(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
		if err != nil {
			t.Fatal(err)
		}
		binding := developmentBindingV1(t, source)
		service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		var result pluginport.ResultV1
		if install {
			intent, intentErr := binding.NewIntentV1(now)
			if intentErr != nil {
				t.Fatal(intentErr)
			}
			result, err = service.Materialize(ctx, intent)
		} else {
			result, err = service.ResolveActive(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		host, err := hostapp.New(officeSkillLifecycleIdentity{principal}, authority, []hostapp.Registration{{
			Identity: registration.Identity, SourceRegistrationSHA256: observed.SourceRegistrationSHA256,
			Materialization: service, State: store,
			SkillReader: OfficeSkillReader{Store: store, Authority: authority, SHA256: registration.DocumentsSkillSHA256},
		}}, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		return host, result
	}
	discover := func(host *hostapp.Service, count int) []hostapp.HostedSkill {
		t.Helper()
		found := host.DiscoverSkills(ctx)
		if len(found.Skills) != count || found.ValidationErrorCount != 0 {
			t.Fatalf("discovery: skills=%d errors=%d, want %d complete", len(found.Skills), found.ValidationErrorCount, count)
		}
		return found.Skills
	}
	activate := func(host *hostapp.Service, generation string, revision uint64, state domainplugin.DesiredStateV1) {
		t.Helper()
		view, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-documents", GenerationID: generation, ExpectedRevision: revision, DesiredState: state})
		if err != nil || view.ActivationRevision != revision+1 || view.DesiredState != state {
			t.Fatalf("activation: %+v %v", view, err)
		}
	}
	consume := func(host *hostapp.Service, skill hostapp.HostedSkill, suffix string) {
		t.Helper()
		calls := 0
		err := host.WithSkill(ctx, skill.Binding, skill.Snapshot.Digest(), func(snapshot domainskill.PackageSnapshot) error {
			calls++
			body, ok := snapshot.File("SKILL.md")
			if !ok || !strings.HasSuffix(string(body), suffix) {
				t.Error("wrong installed skill body")
			}
			return nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("legal installed consumption: calls=%d err=%v", calls, err)
		}
	}
	firstHost, first := compose(store, true)
	discover(firstHost, 0)
	activate(firstHost, first.Receipt.GenerationID, 0, domainplugin.DesiredEnabledV1)
	firstSkill := discover(firstHost, 1)[0]
	consume(firstHost, firstSkill, "first")

	// Hold a real Host consumption after its installed bytes were authorized,
	// then replace the durable generation through the separate materializer.
	entered, release := make(chan struct{}), make(chan struct{})
	late := make(chan error, 1)
	go func() {
		late <- firstHost.WithSkill(ctx, firstSkill.Binding, firstSkill.Snapshot.Digest(), func(domainskill.PackageSnapshot) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("old consumption never entered")
	}
	writeSkill("second")
	secondHost, second := compose(store, true)
	if second.Receipt.GenerationID == first.Receipt.GenerationID {
		t.Fatal("changed installed source did not rotate generation")
	}
	discover(secondHost, 0) // A new generation must not inherit activation.
	activate(secondHost, second.Receipt.GenerationID, 0, domainplugin.DesiredEnabledV1)
	secondSkill := discover(secondHost, 1)[0]
	if secondSkill.Snapshot.Digest() == firstSkill.Snapshot.Digest() {
		t.Fatal("changed installed body retained the old digest")
	}
	close(release)
	select {
	case err := <-late:
		if !errors.Is(err, hostapp.ErrUnavailable) {
			t.Fatalf("old generation late consumption: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("old consumption did not finish")
	}
	for _, host := range []*hostapp.Service{firstHost, secondHost} {
		calls := 0
		if err := host.WithSkill(ctx, firstSkill.Binding, firstSkill.Snapshot.Digest(), func(domainskill.PackageSnapshot) error { calls++; return nil }); err == nil || calls != 0 {
			t.Fatalf("old binding consumed: calls=%d err=%v", calls, err)
		}
	}
	// The actual stale activation API must not revoke the new generation.
	if _, err := store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: first.Receipt.GenerationID, ExpectedRevision: 1, DesiredState: domainplugin.DesiredDisabledV1}, authority, now); !errors.Is(err, pluginport.ErrConflict) {
		t.Fatalf("old generation changed current activation: %v", err)
	}
	consume(secondHost, discover(secondHost, 1)[0], "second")
	reopenedStore, err := OpenExistingPackageStoreV1(home, "analytix-documents")
	if err != nil {
		t.Fatal(err)
	}
	reopenedHost, restored := compose(reopenedStore, false)
	restoredSkill := discover(reopenedHost, 1)[0]
	if restored != second || restoredSkill.Binding != secondSkill.Binding || restoredSkill.Snapshot.Digest() != secondSkill.Snapshot.Digest() {
		t.Fatal("reopen changed installed generation, activation or body")
	}
	consume(reopenedHost, restoredSkill, "second")
	// A separately reopened owner commits a disable during consumption. The
	// first Host's mutex cannot hide that durable transition from its late check.
	calls := 0
	err = secondHost.WithSkill(ctx, secondSkill.Binding, secondSkill.Snapshot.Digest(), func(domainskill.PackageSnapshot) error {
		calls++
		activate(reopenedHost, second.Receipt.GenerationID, 1, domainplugin.DesiredDisabledV1)
		return nil
	})
	if !errors.Is(err, hostapp.ErrDisabled) || calls != 1 {
		t.Fatalf("disabled late consumption: calls=%d err=%v", calls, err)
	}
	finalStore, err := OpenExistingPackageStoreV1(home, "analytix-documents")
	if err != nil {
		t.Fatal(err)
	}
	finalHost, _ := compose(finalStore, false)
	discover(finalHost, 0)
	calls = 0
	if err := finalHost.WithSkill(ctx, secondSkill.Binding, secondSkill.Snapshot.Digest(), func(domainskill.PackageSnapshot) error { calls++; return nil }); !errors.Is(err, hostapp.ErrDisabled) || calls != 0 {
		t.Fatalf("reopened disabled skill consumed: calls=%d err=%v", calls, err)
	}
	activate(finalHost, second.Receipt.GenerationID, 2, domainplugin.DesiredEnabledV1)
	calls = 0
	if err := finalHost.WithSkill(ctx, secondSkill.Binding, secondSkill.Snapshot.Digest(), func(domainskill.PackageSnapshot) error { calls++; return nil }); !errors.Is(err, hostapp.ErrConflict) || calls != 0 {
		t.Fatalf("reenable revived old activation: calls=%d err=%v", calls, err)
	}
	consume(finalHost, discover(finalHost, 1)[0], "second")
}
