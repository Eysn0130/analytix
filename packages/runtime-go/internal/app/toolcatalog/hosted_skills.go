package toolcatalog

import (
	contracts "analytix.local/runtime-go/internal/contracts"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

const DocumentsSkillID = "analytix-documents"

// WithDocumentsSkill reserves the host namespace even while the plugin is
// disabled. A project or global file cannot impersonate the built-in package.
// Snapshot bytes stay private; the model receives only summary metadata until
// run_skill requests the instructions.
func WithDocumentsSkill(base SkillCatalog, snapshot domainskill.PackageSnapshot) SkillCatalog {
	out := base
	out.Skills = nil
	out.snapshots = make(map[string]domainskill.PackageSnapshot, len(base.snapshots)+1)
	for digest, value := range base.snapshots {
		out.snapshots[digest] = value
	}
	if base.Enabled {
		for _, record := range base.Skills {
			if SkillSlug(contracts.StringField(record, "id")) != DocumentsSkillID && SkillSlug(contracts.StringField(record, "name")) != DocumentsSkillID {
				out.Skills = append(out.Skills, contracts.CloneMap(record))
			}
		}
	}
	if snapshot.Valid() && snapshot.EntryRelativePath() == "SKILL.md" && len(snapshot.Paths()) == 1 {
		out.Enabled = true
		out.Reason = ""
		out.snapshots[snapshot.Digest()] = snapshot
		out.Skills = append(out.Skills, SkillRecord(SkillRecordInput{
			ID: DocumentsSkillID, Name: "Analytix Documents", Scope: "global",
			Description: "Create DOCX reports and revise selected document content with reviewed proposals in the current conversation.",
			Entry:       "SKILL.md", PackageDigest: snapshot.Digest(), Metadata: map[string]any{"runAs": "inline"},
		}))
	}
	return out
}
