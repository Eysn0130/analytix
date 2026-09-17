package pluginmaterializationfs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
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
