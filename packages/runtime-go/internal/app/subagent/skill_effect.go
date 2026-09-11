package subagent

import (
	"encoding/json"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func SkillRunIsInline(arguments json.RawMessage, resolve func(string) (map[string]any, bool)) bool {
	if resolve == nil {
		return false
	}
	record, err := domainsecurity.DecodeCanonicalJSONObject(arguments)
	if err != nil {
		return false
	}
	skill, ok := resolve(SkillNameFromArgs(record))
	return ok && SkillRunAs(skill) != "subagent"
}
