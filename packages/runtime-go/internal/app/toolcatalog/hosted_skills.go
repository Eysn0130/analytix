package toolcatalog

import (
	contracts "analytix.local/runtime-go/internal/contracts"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

const DocumentsSkillID = "analytix-documents"

type HostedOfficeSkill struct {
	PackageID string
	Snapshot  domainskill.PackageSnapshot
}

func OfficeSkillForKind(kind string) string {
	switch kind {
	case "docx":
		return DocumentsSkillID
	case "xlsx":
		return "analytix-spreadsheets"
	case "pptx":
		return "analytix-presentations"
	}
	return ""
}
func OfficeSkillIdentity(name string) string {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		id := OfficeSkillForKind(kind)
		if _, ok := SkillByName(SkillCatalog{Skills: []map[string]any{{"id": id, "name": SkillDisplayName(id)}}}, name); ok {
			return id
		}
	}
	return ""
}

// WithOfficeSkills reserves first-party namespaces even while disabled. Only
// current installed snapshots become contributions; bodies remain private until
// run_skill. Local files cannot impersonate these packages.
func WithOfficeSkills(base SkillCatalog, hosted []HostedOfficeSkill) SkillCatalog {
	out := base
	out.Skills = nil
	out.snapshots = make(map[string]domainskill.PackageSnapshot, len(base.snapshots)+len(hosted))
	for digest, value := range base.snapshots {
		out.snapshots[digest] = value
	}
	if base.Enabled {
		for _, record := range base.Skills {
			if OfficeSkillIdentity(contracts.StringField(record, "id")) == "" && OfficeSkillIdentity(contracts.StringField(record, "name")) == "" {
				out.Skills = append(out.Skills, contracts.CloneMap(record))
			}
		}
	}
	counts := map[string]int{}
	for _, item := range hosted {
		counts[item.PackageID]++
	}
	descriptions := map[string]string{
		"analytix-documents":     "Create DOCX reports and revise selected document content with reviewed proposals in the current conversation.",
		"analytix-spreadsheets":  "Create typed XLSX workbooks with checked formulas, formatting and charts; analyze data and review selected cell changes.",
		"analytix-presentations": "Create structured PPTX presentations with stable objects, charts and images; review targeted slide changes.",
	}
	for _, item := range hosted {
		snapshot := item.Snapshot
		description, known := descriptions[item.PackageID]
		if !known || counts[item.PackageID] != 1 || !snapshot.Valid() || snapshot.EntryRelativePath() != "SKILL.md" || len(snapshot.Paths()) != 1 {
			continue
		}
		out.Enabled = true
		out.Reason = ""
		out.snapshots[snapshot.Digest()] = snapshot
		out.Skills = append(out.Skills, SkillRecord(SkillRecordInput{ID: item.PackageID, Name: SkillDisplayName(item.PackageID), Description: description, Scope: "global", Entry: "SKILL.md", PackageDigest: snapshot.Digest(), Metadata: map[string]any{"runAs": "inline"}}))
	}
	return out
}

func WithDocumentsSkill(base SkillCatalog, snapshot domainskill.PackageSnapshot) SkillCatalog {
	return WithOfficeSkills(base, []HostedOfficeSkill{{PackageID: DocumentsSkillID, Snapshot: snapshot}})
}
