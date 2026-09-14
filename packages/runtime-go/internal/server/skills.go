package server

import (
	"context"
	"net/http"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
)

func (h *runtimeServerHandler) handleSkills(w http.ResponseWriter, r *http.Request) {
	httpapi.SkillHandlers{Service: runtimeSkillHTTPService{handler: h}}.Handle(w, r)
}

type runtimeSkillHTTPService struct {
	handler *runtimeServerHandler
}

func (s runtimeSkillHTTPService) Skills() map[string]any {
	return s.handler.skillResponse()
}

func (h *runtimeServerHandler) skillCapabilityState() map[string]any {
	return toolcatalogapp.SkillCapabilityState(h.currentSkillCatalog())
}

func (h *runtimeServerHandler) skillToolDiagnostics() map[string]any {
	return toolcatalogapp.SkillToolDiagnostics(h.currentSkillCatalog())
}

func (h *runtimeServerHandler) skillResponse() map[string]any {
	return toolcatalogapp.SkillResponse(h.currentSkillCatalog())
}

func (h *runtimeServerHandler) runtimeSkillByName(name string) (map[string]any, bool) {
	return toolcatalogapp.SkillByName(h.currentSkillCatalog(), name)
}

func (h *runtimeServerHandler) runtimeSkillIDs() []any {
	return toolcatalogapp.SkillIDs(h.currentSkillCatalog())
}

func (h *runtimeServerHandler) runtimeSkillEntryBody(skill map[string]any) (string, error) {
	return toolcatalogapp.SkillEntryBody(h.currentSkillCatalog(), skill)
}

func (h *runtimeServerHandler) currentSkillCatalog() toolcatalogapp.SkillCatalog {
	var snapshot domainskill.PackageSnapshot
	if h.officePackageHost != nil {
		if skills := h.officePackageHost.Skills(context.Background()); len(skills) == 1 {
			snapshot = skills[0].Snapshot
		}
	}
	return toolcatalogapp.WithDocumentsSkill(h.skills, snapshot)
}

func (h *runtimeServerHandler) runtimeSkillSubagentPrompt(skill map[string]any, task string) (string, error) {
	body, err := h.runtimeSkillEntryBody(skill)
	if err != nil {
		return "", err
	}
	return toolcatalogapp.SkillSubagentPrompt(skill, body, task), nil
}

func loadRuntimeSkillCatalog(config RuntimeServerConfig) (runtimeSkillCatalog, error) {
	document, ok, err := runtimeConfigDocument(config)
	if err != nil {
		return runtimeSkillCatalog{}, err
	}
	return toolcatalogapp.LoadSkillCatalogFromDocument(
		document,
		ok,
		config.DataDir,
		toolcatalogapp.SkillCatalogFileSource{
			NormalizeRoot:     filestore.NormalizeSkillRoot,
			RootExists:        filestore.SkillRootDirectoryExists,
			PackageCandidates: filestore.SkillPackageCandidates,
			LoadPackage:       filestore.LoadSkillPackage,
		},
	)
}

func uniqueRuntimeStrings(values []string) []string {
	return toolcatalogapp.UniqueStringsSorted(values)
}
