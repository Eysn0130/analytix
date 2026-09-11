package architecture_test

import "testing"

func TestAuthorityAdvanceJournalDoesNotCreateEpochOrCompositionDependencies(t *testing.T) {
	root := runtimeGoRoot(t)
	for _, directory := range []string{
		"internal/domain/authorityadvance",
		"internal/app/authorityadvance",
		"internal/ports/authorityadvance",
		"internal/adapters/outbound/authorityadvancefs",
	} {
		checkImports(t, root, directory, []string{
			"analytix.local/runtime-go/internal/domain/contextepoch",
			"analytix.local/runtime-go/internal/app/contextepoch",
			"analytix.local/runtime-go/internal/runtimeapp",
			"analytix.local/runtime-go/internal/server",
		})
	}
}
