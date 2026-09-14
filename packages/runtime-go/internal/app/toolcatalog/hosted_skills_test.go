package toolcatalog

import (
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	"strings"
	"testing"
)

func TestHostedDocumentsSkillUsesPrivateSnapshotWithoutConfiguredRoot(t *testing.T) {
	snapshot, _ := domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte("---\nname: ignored\n---\nOriginal host instructions.")}})
	base := SkillCatalog{Enabled: true, Skills: []map[string]any{{"id": DocumentsSkillID, "name": "forged", "entryPath": "/private/forged"}, {"id": "ordinary", "name": "ordinary", "scope": "project"}}}
	catalog := WithDocumentsSkill(base, snapshot)
	if len(catalog.Skills) != 2 || len(base.Skills) != 2 {
		t.Fatal("merge changed source or duplicated reserved skill")
	}
	record, ok := SkillByName(catalog, DocumentsSkillID)
	if !ok || record["entryPath"] != "" || record["runAs"] != "inline" {
		t.Fatal(record)
	}
	body, err := SkillEntryBody(catalog, record)
	if err != nil || !strings.Contains(body, "Original host instructions.") {
		t.Fatal(err)
	}
	response := SkillResponse(catalog)
	if response["skillCount"] != float64(2) || response["configuredRootCount"] != float64(0) {
		t.Fatal(response)
	}
	disabled := WithDocumentsSkill(base, domainskill.PackageSnapshot{})
	if _, ok := SkillByName(disabled, DocumentsSkillID); ok {
		t.Fatal("project impersonation survived disable")
	}
	if len(disabled.Skills) != 1 {
		t.Fatal("ordinary skill lost")
	}
}

func TestHostedCanvasSkillReservesBothNamesAndUsesInstalledSnapshot(t *testing.T) {
	snapshot, err := domainskill.NewPackageSnapshot("SKILL.md", []domainskill.FileInput{{RelativePath: "SKILL.md", Bytes: []byte("---\nname: canvas\ndescription: Synthetic Canvas\n---\nPrivate installed instructions.")}})
	if err != nil {
		t.Fatal(err)
	}
	name := SkillDisplayName(CanvasSkillID)
	base := SkillCatalog{Enabled: true, Skills: []map[string]any{
		{"id": CanvasSkillID, "name": "forged", "entryPath": "/private/forged"},
		{"id": "forged-by-name", "name": name, "entryPath": "/private/forged"},
		{"id": "ordinary", "name": "ordinary"},
	}}
	disabled := WithOfficeSkills(base, nil)
	if len(disabled.Skills) != 1 {
		t.Fatal("disabled Canvas namespace can be impersonated")
	}
	for _, alias := range []string{CanvasSkillID, name} {
		if OfficeSkillIdentity(alias) != CanvasSkillID {
			t.Fatal("run_skill dispatch lost Canvas identity", alias)
		}
	}
	if OfficeSkillForKind("canvas") != "" || OfficeSkillForKind("png") != "" {
		t.Fatal("Canvas entered Office codec routing")
	}
	catalog := WithOfficeSkills(base, []HostedOfficeSkill{{PackageID: CanvasSkillID, Snapshot: snapshot}})
	record, ok := SkillByName(catalog, CanvasSkillID)
	if !ok || len(catalog.Skills) != 2 || record["entryPath"] != "" || record["runAs"] != "inline" {
		t.Fatal("installed Canvas discovery failed", record)
	}
	body, err := SkillEntryBody(catalog, record)
	if err != nil || !strings.Contains(body, "Private installed instructions.") {
		t.Fatal("wrong installed body", err)
	}
	duplicate := WithOfficeSkills(base, []HostedOfficeSkill{{PackageID: CanvasSkillID, Snapshot: snapshot}, {PackageID: CanvasSkillID, Snapshot: snapshot}})
	if len(duplicate.Skills) != 1 {
		t.Fatal("duplicate Canvas contribution exposed")
	}
	if len(base.Skills) != 3 {
		t.Fatal("source catalog mutated")
	}
}
