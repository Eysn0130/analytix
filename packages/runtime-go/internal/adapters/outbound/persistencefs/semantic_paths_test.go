package persistencefs

import "path/filepath"

// Test fixtures may create isolated namespaces explicitly. Production
// semantic paths are derived only from the frozen JournalNamespaceAuthority.
func semanticPlanningRoot(roots RootSet) (string, error) {
	namespace, err := persistentStartupNamespacePath(roots, true)
	if err != nil {
		return "", err
	}
	return filepath.Join(namespace, "planning-"+rootBindingDigest(roots)), nil
}

func semanticJournalRoot(roots RootSet) (string, error) {
	namespace, err := persistentStartupNamespacePath(roots, true)
	if err != nil {
		return "", err
	}
	if err := securePrepareStartupAuthorityNamespace(namespace); err != nil {
		return "", err
	}
	return filepath.Join(namespace, "journal"), nil
}
