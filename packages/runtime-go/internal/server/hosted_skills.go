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

func (h *runtimeServerHandler) currentOfficeSkill(ctx context.Context, packageID string) (hostapp.HostedSkill, bool) {
	if h.officePackageHost == nil {
		return hostapp.HostedSkill{}, false
	}
	skills := h.officePackageHost.Skills(ctx)
	for _, skill := range skills {
		if skill.Binding.PackageID == packageID {
			return skill, true
		}
	}
	return hostapp.HostedSkill{}, false
}

func hostedSkillProjection(skill hostapp.HostedSkill) map[string]any {
	b := skill.Binding
	return map[string]any{"packageId": b.PackageID, "packageVersion": b.PackageVersion,
		"generationId": b.GenerationID, "activationRevision": b.ActivationRevision,
		"sourceRegistrationSha256": b.SourceRegistrationSHA256, "skillDigest": skill.Snapshot.Digest()}
}

func (h *runtimeServerHandler) executeOfficeSkill(ctx context.Context, args map[string]any, packageID string) (any, bool) {
	failure := map[string]any{"code": "skill_snapshot_invalid", "error": "The Office plugin is unavailable or changed. Reload its skill before continuing."}
	if subagentapp.SkillContinueOrForkRequested(args) {
		return failure, true
	}
	loaded, ok := ctx.Value(preparedHostedSkillKey{}).(hostapp.HostedSkill)
	if !ok {
		loaded, ok = h.currentOfficeSkill(ctx, packageID)
	}
	if !ok || loaded.Binding.PackageID != packageID {
		return failure, true
	}
	var output any
	err := h.officePackageHost.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(snapshot domainskill.PackageSnapshot) error {
		catalog := toolcatalogapp.WithOfficeSkills(toolcatalogapp.SkillCatalog{}, []toolcatalogapp.HostedOfficeSkill{{PackageID: packageID, Snapshot: snapshot}})
		record, exists := toolcatalogapp.SkillByName(catalog, packageID)
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
