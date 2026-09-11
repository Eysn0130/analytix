package thread

import (
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type ListProjectionFilter struct {
	ArchivedOnly    bool
	IncludeArchived bool
	IncludeSide     bool
	Search          string
	EventLogMatched bool
}

func ListIncludesSide(include string) bool {
	for _, part := range strings.Split(include, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "side") {
			return true
		}
	}
	return false
}

func IncludeThreadInList(thread map[string]any, summary map[string]any, filter ListProjectionFilter) bool {
	if !filter.IncludeSide && shouldHideThreadFromPrimaryList(thread) {
		return false
	}
	status := stringField(thread, "status")
	if filter.ArchivedOnly {
		if status != "archived" {
			return false
		}
	} else if !filter.IncludeArchived && status == "archived" {
		return false
	}
	needle := strings.TrimSpace(filter.Search)
	caseSensitive := caseSensitiveThreadSearch(thread)
	if needle != "" && !(filter.EventLogMatched && !caseSensitive) && !ThreadMatchesSearch(thread, summary, needle) {
		return false
	}
	return true
}

func shouldHideThreadFromPrimaryList(thread map[string]any) bool {
	relation := stringField(thread, "relation")
	if relation == "side" {
		return true
	}
	return relation != "fork" && stringField(thread, "parentThreadId") != ""
}

func ThreadMatchesSearch(thread map[string]any, summary map[string]any, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	if caseSensitiveThreadSearch(thread) {
		// Case search is a metadata-only product projection. Titles, workspace
		// paths, ids, user text, tool bodies, aliases, and typed-local bytes are
		// never search inputs. Only the closed runtime-owned lifecycle code may
		// match.
		status := strings.ToLower(strings.TrimSpace(stringField(summary, "status")))
		switch status {
		case "idle", "running", "archived", "deleted":
			return strings.Contains(status, needle)
		default:
			return false
		}
	}
	return valueContainsSearch(summary, needle) || valueContainsSearch(thread["turns"], needle)
}

func caseSensitiveThreadSearch(thread map[string]any) bool {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	return caseSensitive || err != nil
}

func SortThreadSummaries(threads []map[string]any) {
	sort.Slice(threads, func(i, j int) bool {
		left := stringField(threads[i], "updatedAt")
		right := stringField(threads[j], "updatedAt")
		if left == right {
			return stringField(threads[i], "id") > stringField(threads[j], "id")
		}
		return left > right
	})
}

func valueContainsSearch(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(strings.ToLower(typed), needle)
	case []any:
		for _, item := range typed {
			if valueContainsSearch(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if valueContainsSearch(item, needle) {
				return true
			}
		}
	}
	return false
}
