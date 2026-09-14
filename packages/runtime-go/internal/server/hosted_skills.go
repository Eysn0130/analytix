package server

import (
	"context"
	"errors"

	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

type preparedHostedSkillKey struct{}

func (h *runtimeServerHandler) currentDocumentsSkill(ctx context.Context) (hostapp.HostedSkill, bool) {
	if h.officePackageHost == nil {
		return hostapp.HostedSkill{}, false
	}
	skills := h.officePackageHost.Skills(ctx)
	if len(skills) != 1 {
		return hostapp.HostedSkill{}, false
	}
	return skills[0], true
}

func isDocumentsSkillName(name string) bool {
	// Reuse the public skill name normalization, including $ and @ prefixes.
	catalog := toolcatalogapp.SkillCatalog{Skills: []map[string]any{{"id": toolcatalogapp.DocumentsSkillID, "name": "Analytix Documents"}}}
	_, ok := toolcatalogapp.SkillByName(catalog, name)
	return ok
}

func hostedSkillProjection(skill hostapp.HostedSkill) map[string]any {
	b := skill.Binding
	return map[string]any{"packageId": b.PackageID, "packageVersion": b.PackageVersion,
		"generationId": b.GenerationID, "activationRevision": b.ActivationRevision,
		"sourceRegistrationSha256": b.SourceRegistrationSHA256, "skillDigest": skill.Snapshot.Digest()}
}

func (h *runtimeServerHandler) executeDocumentsSkill(ctx context.Context, args map[string]any) (any, bool) {
	failure := map[string]any{"code": "skill_snapshot_invalid", "error": "The Documents plugin is unavailable or changed. Reload its skill before continuing."}
	if subagentapp.SkillContinueOrForkRequested(args) {
		return failure, true
	}
	loaded, ok := ctx.Value(preparedHostedSkillKey{}).(hostapp.HostedSkill)
	if !ok {
		loaded, ok = h.currentDocumentsSkill(ctx)
	}
	if !ok {
		return failure, true
	}
	var output any
	err := h.officePackageHost.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(snapshot domainskill.PackageSnapshot) error {
		catalog := toolcatalogapp.WithDocumentsSkill(toolcatalogapp.SkillCatalog{}, snapshot)
		record, exists := toolcatalogapp.SkillByName(catalog, toolcatalogapp.DocumentsSkillID)
		if !exists {
			return errors.New("hosted skill unavailable")
		}
		body, err := toolcatalogapp.SkillEntryBody(catalog, record)
		if err != nil {
			return err
		}
		output = subagentapp.InlineSkillOutput(record, args, "inline", body)
		return nil
	})
	if err != nil {
		return failure, true
	}
	return output, false
}
