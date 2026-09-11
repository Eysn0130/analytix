package pendingwork

import (
	"errors"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
)

// RestartGrantOutcomesByTurnV1 groups verified outcome-unknown write effects
// by their canonical turn identity. It rejects missing and duplicate grant
// authority instead of allowing a later map assignment to hide corruption.
func RestartGrantOutcomesByTurnV1(
	records []RestartOutcomeUnknownGrantV1,
) (map[string]executiongrantapp.RestartGrantOutcomesV1, error) {
	result := make(map[string]executiongrantapp.RestartGrantOutcomesV1)
	for _, record := range records {
		if strings.TrimSpace(record.ThreadID) == "" || strings.TrimSpace(record.TurnID) == "" || strings.TrimSpace(record.GrantID) == "" {
			return nil, errors.New("outcome-unknown write dispatch identity is invalid")
		}
		turnKey := strings.TrimSpace(record.ThreadID) + "\x00" + strings.TrimSpace(record.TurnID)
		if result[turnKey] == nil {
			result[turnKey] = executiongrantapp.RestartGrantOutcomesV1{}
		}
		if _, duplicate := result[turnKey][record.GrantID]; duplicate {
			return nil, errors.New("outcome-unknown write dispatch grant is duplicated")
		}
		result[turnKey][record.GrantID] = executiongrantapp.RestartGrantOutcomeAuthorityV1{
			Receipt: record.Receipt, Disposition: record.Disposition,
		}
	}
	return result, nil
}
