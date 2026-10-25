package server

import (
	"context"
	"errors"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	contextsourceport "analytix.local/runtime-go/internal/ports/contextsource"
)

const initialUserSkillTokenLimit = 16_384

// This is an admission-local reader over already verified immutable snapshots,
// not a new registry. Only metadata enters the existing Epoch state.
type initialUserSkillContext struct {
	sources []domaincontextepoch.SourceEntry
	content map[string]contextsourceport.Content
	ids     map[string]string
}

func (skills initialUserSkillContext) ReadContextSource(ctx context.Context, _ string, entry domaincontextepoch.SourceEntry) (contextsourceport.Content, error) {
	if err := ctx.Err(); err != nil {
		return contextsourceport.Content{}, err
	}
	content, ok := skills.content[entry.SourceID]
	if !ok || entry.Kind != contextepochapp.InitialUserSkillKind || content.Digest != entry.Digest {
		return contextsourceport.Content{}, errors.New("initial skill source unavailable")
	}
	return content, nil
}

func (h *runtimeServerHandler) prepareInitialUserSkills(ctx context.Context, requested []string) (initialUserSkillContext, error) {
	result := initialUserSkillContext{content: map[string]contextsourceport.Content{}, ids: map[string]string{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if len(requested) == 0 || len(requested) > 16 {
		return result, nil
	}
	catalog := h.skills
	if h.officePackageHost != nil {
		discovery := h.officePackageHost.DiscoverSkills(ctx)
		hosted := make([]toolcatalogapp.HostedOfficeSkill, 0, len(discovery.Skills))
		for _, skill := range discovery.Skills {
			hosted = append(hosted, toolcatalogapp.HostedOfficeSkill{PackageID: skill.Binding.PackageID, Snapshot: skill.Snapshot})
		}
		catalog = toolcatalogapp.WithOfficeSkills(catalog, hosted)
	}
	if err := ctx.Err(); err != nil {
		return initialUserSkillContext{}, err
	}
	if !catalog.Enabled {
		return result, nil
	}
	total := 0
	seen := map[string]bool{}
	for _, requestedID := range requested {
		if err := ctx.Err(); err != nil {
			return initialUserSkillContext{}, err
		}
		skill, found := toolcatalogapp.SkillByName(catalog, requestedID)
		if !found {
			continue
		}
		summary, public := toolcatalogapp.PublicSkillSummaryV2(skill)
		id := contracts.StringField(summary, "id")
		if !public || id != requestedID || seen[id] || toolcatalogapp.NormalizeSkillRunAs(contracts.StringField(skill, "runAs")) == "subagent" {
			continue
		}
		seen[id] = true
		var body string
		var err error
		if packageID := toolcatalogapp.OfficeSkillIdentity(id); packageID != "" {
			loaded, ok := h.currentOfficeSkill(ctx, packageID)
			if !ok {
				continue
			}
			err = h.officePackageHost.WithSkill(ctx, loaded.Binding, loaded.Snapshot.Digest(), func(snapshot domainskill.PackageSnapshot) error {
				current := toolcatalogapp.WithOfficeSkills(toolcatalogapp.SkillCatalog{}, []toolcatalogapp.HostedOfficeSkill{{PackageID: packageID, Snapshot: snapshot}})
				record, exists := toolcatalogapp.SkillByName(current, id)
				if !exists {
					return errors.New("initial hosted skill unavailable")
				}
				var loadErr error
				body, loadErr = toolcatalogapp.SkillEntryBody(current, record)
				return loadErr
			})
		} else {
			body, err = toolcatalogapp.SkillEntryBody(catalog, skill)
		}
		if cancelErr := ctx.Err(); cancelErr != nil {
			return initialUserSkillContext{}, cancelErr
		}
		if err != nil || body == "" {
			continue
		}
		text := "[Skill " + id + "]\n" + body
		tokens := contextepochapp.EstimateTokens(text)
		if tokens > initialUserSkillTokenLimit-total {
			continue
		}
		total += tokens
		sourceID := "initial-skill:" + domaincontextepoch.SHA256Hex([]byte(id))
		digest := domaincontextepoch.SHA256Hex([]byte(text))
		result.sources = append(result.sources, domaincontextepoch.SourceEntry{
			Version: domaincontextepoch.ContractVersion, SourceID: sourceID, Kind: contextepochapp.InitialUserSkillKind,
			Digest: digest, TrustState: domaincontextepoch.TrustUntrusted, PromptBoundary: domaincontextepoch.BoundaryDynamicContext,
			TokenBudget: tokens, ActivationState: domaincontextepoch.ActivationActive,
		})
		result.content[sourceID] = contextsourceport.Content{Text: text, Digest: digest}
		result.ids[sourceID] = id
	}
	return result, nil
}

func (skills initialUserSkillContext) attachLoadedIDs(plan turnapp.StartPlan, providerContext domaincontextepoch.ProviderContext) {
	if plan.UserItemID == "" || contracts.StringField(plan.Record.UserItem, "id") != plan.UserItemID {
		return
	}
	ids := []any{}
	for _, fragment := range providerContext.Dynamic {
		if id := skills.ids[fragment.SourceID]; id != "" && fragment.Kind == contextepochapp.InitialUserSkillKind && fragment.Content != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	plan.Record.UserItem["activeSkillIds"] = contracts.CloneValue(ids)
	for _, raw := range listAny(plan.Record.Turn["items"]) {
		item, _ := raw.(map[string]any)
		if contracts.StringField(item, "id") == plan.UserItemID {
			item["activeSkillIds"] = contracts.CloneValue(ids)
		}
	}
	if turn, ok := plan.Record.TurnStartedEvent["turn"].(map[string]any); ok {
		for _, raw := range listAny(turn["items"]) {
			item, _ := raw.(map[string]any)
			if contracts.StringField(item, "id") == plan.UserItemID {
				item["activeSkillIds"] = contracts.CloneValue(ids)
			}
		}
	}
	if item, ok := plan.Record.UserItemCreatedEvent["item"].(map[string]any); ok && contracts.StringField(item, "id") == plan.UserItemID {
		item["activeSkillIds"] = contracts.CloneValue(ids)
	}
}
