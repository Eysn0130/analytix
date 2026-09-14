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
