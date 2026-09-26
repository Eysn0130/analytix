package pluginmaterializationfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

func writeDocumentsSkillSourceV1(t *testing.T) (string, []byte) {
	t.Helper()
	root := writeDevelopmentSourceV1(t, "analytix-documents")
	declarationPath := filepath.Join(root, filepath.FromSlash(domainpackage.DeclarationRelativePathV1))
	body, err := os.ReadFile(declarationPath)
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domainpackage.ParseDeclarationV1(body)
	if err != nil {
		t.Fatal(err)
	}
	declaration.Contributions.Skills = []domainpackage.PathContributionV1{{ID: domainpackage.DocumentsSkillContributionIDV1, Path: domainpackage.DocumentsSkillRelativePathV1}}
	declaration.RequestedCapabilities = append(declaration.RequestedCapabilities, domainpackage.CapabilityRequestV1{
		ID: "office.document-generation", ProtocolVersion: 1, ScopeConstraints: []string{"new-file", "current-conversation"},
	})
	body, err = domainpackage.CanonicalDeclarationV1Bytes(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(declarationPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	skill := []byte("---\nname: documents\ndescription: Synthetic document creation\n---\n\n# Documents\n\nUse the controlled document generation tool.\n")
	skillPath := filepath.Join(root, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1))
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, skill, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, skill
}

func TestDocumentsSkillTreeBindsActualBytesAndInstalledGenerationV1(t *testing.T) {
	ctx := context.Background()
	source, skill := writeDocumentsSkillSourceV1(t)
	observed, err := InspectDevelopmentSourceTreeV1(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	digest := sha256.Sum256(skill)
	if err != nil || registration.DocumentsSkillSHA256 != hex.EncodeToString(digest[:]) || registration.SourceTreeFileCount != 5 {
		t.Fatal("fixed Skill bytes were not bound into registration", err)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(observed.SourceRegistrationJSON), &fields) != nil || len(fields) != 16 {
		t.Fatal("Skill tree registration shape is not exact")
	}
	home := realTempDir(t)
	authority := newTestAuthority()
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	store, err := NewPackageStoreV1(home, "analytix-documents", nil)
	if err != nil {
		t.Fatal(err)
	}
	binding := developmentBindingV1(t, source)
	intent, err := binding.NewIntentV1(now)
	if err != nil {
		t.Fatal(err)
	}
	service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Materialize(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: first.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1}, authority, now)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenExistingPackageStoreV1(home, "analytix-documents")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.ResolveActive(ctx, authority); err != nil || got != first {
		t.Fatal("installed Skill generation failed reopening", err)
	}
	changedSkill := append(append([]byte{}, skill...), []byte("\nChanged synthetic instructions.\n")...)
	if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1)), changedSkill, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Materialize(ctx, intent); err == nil {
		t.Fatal("changed Skill source materialized under stale registration")
	}
	if got, err := service.ResolveActive(ctx); err != nil || got != first {
		t.Fatal("source edit changed installed generation authority", err)
	}
	nextBinding := developmentBindingV1(t, source)
	nextIntent, err := nextBinding.NewIntentV1(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	nextService, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, nextBinding, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	next, err := nextService.Materialize(ctx, nextIntent)
	if err != nil || next.Receipt.GenerationID == first.Receipt.GenerationID || next.Receipt.SourceRegistrationSHA256 == first.Receipt.SourceRegistrationSHA256 {
		t.Fatal("Skill upgrade did not rotate frozen generation", err)
	}
	if _, err := service.ResolveActive(ctx); err == nil {
		t.Fatal("old binding resolved upgraded Skill")
	}
	if _, err := reopened.ReadActivation(ctx, authority); !errors.Is(err, pluginport.ErrNotFound) {
		t.Fatal("Skill upgrade inherited old activation", err)
	}
	active := store.absolute(next.Receipt.ActiveRelativePath)
	installed, err := inspectInstalledForOriginV1(ctx, active, domainplugin.DevelopmentSourceOriginV1)
	if err != nil || installed.SourceRegistrationSHA256 != next.Receipt.SourceRegistrationSHA256 {
		t.Fatal("installed reconstruction changed new registration digest", err)
	}
	if err := os.WriteFile(filepath.Join(active, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1)), []byte("tampered installed Skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.ResolveActive(ctx, authority); !errors.Is(err, pluginport.ErrCorrupt) {
		t.Fatal("tampered installed Skill retained signed authority", err)
	}
}

func TestDocumentsSkillTreeRejectsMissingAndSymlinkedContributionV1(t *testing.T) {
	for _, name := range []string{"missing", "symlink"} {
		t.Run(name, func(t *testing.T) {
			source, skill := writeDocumentsSkillSourceV1(t)
			path := filepath.Join(source, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if name == "symlink" {
				target := filepath.Join(realTempDir(t), "SKILL.md")
				if err := os.WriteFile(target, skill, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := InspectDevelopmentSourceTreeV1(context.Background(), source); err == nil {
				t.Fatal("unavailable fixed Skill contribution was admitted")
			}
		})
	}
}

func TestDocumentsSkillUndeclaredBytesDoNotCreateContributionV1(t *testing.T) {
	source := writeDevelopmentSourceV1(t, "analytix-documents")
	path := filepath.Join(source, filepath.FromSlash(domainpackage.DocumentsSkillRelativePathV1))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("unregistered instructions"), 0o600); err != nil {
		t.Fatal(err)
	}
	observed, err := InspectDevelopmentSourceTreeV1(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	if err != nil || registration.DocumentsSkillSHA256 != "" {
		t.Fatal("undeclared file acquired Skill contribution authority", err)
	}
}
