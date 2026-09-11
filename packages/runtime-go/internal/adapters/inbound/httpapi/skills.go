package httpapi

import "net/http"

type SkillCatalogService interface {
	Skills() map[string]any
}

type SkillHandlers struct {
	Service SkillCatalogService
}

func (h SkillHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if h.Service == nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]any{"code": "skill_catalog_missing", "message": "skill catalog missing"})
		return
	}
	WriteJSON(w, http.StatusOK, h.Service.Skills())
}
