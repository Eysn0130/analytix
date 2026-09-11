//go:build !darwin && !linux && !windows

package persistencefs

import "errors"

func semanticJournalV3MigrationCode() string {
	return "semantic_journal_v3_platform_identity_migration_required"
}

func semanticPlanningV1MigrationCode() string {
	return "semantic_planning_v1_platform_identity_migration_required"
}

func semanticRetirementV1MigrationCode() string {
	return "semantic_retirement_v1_platform_identity_migration_required"
}

func migrateSemanticPlanningMarkerV1(semanticPlanningMarkerV1) (semanticPlanningMarkerV1, error) {
	return semanticPlanningMarkerV1{}, semanticPlanningV1Blocker()
}

func migrateAuthenticatedSemanticJournalV3(*startupPrivateDirectory, semanticJournalV1, *startupJournalAuthority, func(string) error) (semanticJournalV1, error) {
	return semanticJournalV1{}, semanticJournalV3Blocker()
}

func decodeSemanticJournalForRetirement(
	_ *startupPrivateDirectory,
	body []byte,
	authority *startupJournalAuthority,
) (semanticJournalV1, error) {
	journal, err := decodeSemanticJournal(body, authority)
	var legacy authenticatedSemanticJournalV3
	if errors.As(err, &legacy) {
		return semanticJournalV1{}, semanticJournalV3Blocker()
	}
	return journal, err
}

func migrateAuthenticatedSemanticRetirementV1(
	_ *startupPrivateDirectory,
	_ semanticJournalRetirementV1,
	_ *startupJournalAuthority,
	_ RootSet,
) (semanticJournalRetirementV1, error) {
	return semanticJournalRetirementV1{}, semanticRetirementV1Blocker()
}
