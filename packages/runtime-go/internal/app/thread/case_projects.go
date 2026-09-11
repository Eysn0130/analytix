package thread

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"analytix.local/runtime-go/internal/contracts"
)

var ErrCaseProjectNotFound = errors.New("case project not found")

func (s *Service) ListCaseProjectSummaries(limit int) ([]map[string]any, string, error) {
	threads, err := s.List(ListInput{IncludeArchived: true, IncludeSide: true})
	if err != nil {
		return nil, "ready", err
	}
	projects := CaseProjectSummariesFromThreads(threads)
	if limit > 0 && len(projects) > limit {
		projects = projects[:limit]
	}
	return projects, "ready", nil
}

func (s *Service) ListCaseProjectThreads(caseProjectID string, limit int) ([]map[string]any, error) {
	threads, err := s.List(ListInput{IncludeArchived: true, IncludeSide: true})
	if err != nil {
		return nil, err
	}
	filtered := make([]map[string]any, 0, len(threads))
	for _, thread := range threads {
		workspace := NormalizeCaseProjectRoot(contracts.StringField(thread, "workspace"))
		if workspace != "" && CaseProjectIDForRoot(workspace) == strings.TrimSpace(caseProjectID) {
			filtered = append(filtered, contracts.CloneMap(thread))
		}
	}
	SortThreadSummaries(filtered)
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (s *Service) GetCaseProjectDetail(caseProjectID string, limit int) (map[string]any, error) {
	projects, _, err := s.ListCaseProjectSummaries(0)
	if err != nil {
		return nil, err
	}
	for _, project := range projects {
		if contracts.StringField(project, "id") == strings.TrimSpace(caseProjectID) {
			threads, err := s.ListCaseProjectThreads(caseProjectID, limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"project": project, "threads": threads}, nil
		}
	}
	return nil, ErrCaseProjectNotFound
}

func CaseProjectSummariesFromThreads(threadSummaries []map[string]any) []map[string]any {
	projectsByRoot := map[string]map[string]any{}
	for _, thread := range threadSummaries {
		if strings.EqualFold(contracts.StringField(thread, "status"), "deleted") {
			continue
		}
		root := NormalizeCaseProjectRoot(contracts.StringField(thread, "workspace"))
		if root == "" {
			continue
		}
		project := projectsByRoot[root]
		if project == nil {
			project = map[string]any{
				"id": CaseProjectIDForRoot(root), "name": CaseProjectName(root), "rootPath": root,
				"updatedAt": contracts.StringField(thread, "updatedAt"), "threadCount": float64(0), "runningCount": float64(0),
				"archivedCount": float64(0), "lastThreadId": "", "lastPreview": "", "dataSizeEstimate": float64(0), "status": "ready",
			}
			projectsByRoot[root] = project
		}
		project["threadCount"] = caseProjectFloat(project["threadCount"]) + 1
		if strings.EqualFold(contracts.StringField(thread, "status"), "running") || thread["hasRunningTurn"] == true {
			project["runningCount"] = caseProjectFloat(project["runningCount"]) + 1
		}
		if thread["archived"] == true || strings.EqualFold(contracts.StringField(thread, "status"), "archived") {
			project["archivedCount"] = caseProjectFloat(project["archivedCount"]) + 1
		}
		threadUpdatedAt := contracts.StringField(thread, "updatedAt")
		if threadUpdatedAt >= contracts.StringField(project, "updatedAt") {
			project["updatedAt"] = threadUpdatedAt
			project["lastThreadId"] = contracts.StringField(thread, "id")
			project["lastPreview"] = contracts.StringField(thread, "preview")
		}
	}
	projects := make([]map[string]any, 0, len(projectsByRoot))
	for _, project := range projectsByRoot {
		projects = append(projects, contracts.CloneMap(project))
	}
	sort.SliceStable(projects, func(i, j int) bool {
		left, right := contracts.StringField(projects[i], "updatedAt"), contracts.StringField(projects[j], "updatedAt")
		if left == right {
			return contracts.StringField(projects[i], "id") > contracts.StringField(projects[j], "id")
		}
		return left > right
	})
	return projects
}

func NormalizeCaseProjectRoot(value string) string {
	root := strings.TrimSpace(value)
	if root == "" {
		return ""
	}
	root = strings.ReplaceAll(root, "\\", "/")
	if len(root) >= 2 && root[1] == ':' {
		root = strings.ToUpper(root[:1]) + root[1:]
	}
	for strings.HasSuffix(root, "/") && root != "/" && !(len(root) == 3 && root[1] == ':' && root[2] == '/') {
		root = strings.TrimSuffix(root, "/")
	}
	return root
}

func CaseProjectIDForRoot(root string) string {
	sum := sha256.Sum256([]byte(NormalizeCaseProjectRoot(root)))
	return "case_" + hex.EncodeToString(sum[:])[:24]
}

func CaseProjectName(root string) string {
	clean := NormalizeCaseProjectRoot(root)
	if clean == "" || clean == "/" {
		return clean
	}
	trimmed := strings.TrimSuffix(clean, "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

func caseProjectFloat(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}
