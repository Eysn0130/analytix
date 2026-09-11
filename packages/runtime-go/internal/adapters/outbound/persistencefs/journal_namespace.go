package persistencefs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// JournalNamespaceAuthority freezes one persistent private namespace for
// signing keys, recovery journals, and rollback-safe planning stages. Planning
// can contain sensitive persisted bytes, so it must not spill into the shared
// operating-system temporary directory.
type JournalNamespaceAuthority struct {
	root          startupAuthorityRoot
	planningRoot  startupAuthorityRoot
	roots         RootSet
	bindingDigest string
}

func FreezeJournalNamespaceAuthorityForRoots(roots RootSet) (*JournalNamespaceAuthority, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	directory, err := persistentStartupNamespacePath(resolved, true)
	if err != nil {
		return nil, err
	}
	return freezeJournalNamespaceAuthorityForResolvedPath(resolved, directory, true)
}

func freezeJournalNamespaceAuthorityForResolvedPath(
	resolved RootSet,
	directory string,
	create bool,
) (*JournalNamespaceAuthority, error) {
	var authority *JournalNamespaceAuthority
	var err error
	if create {
		authority, err = freezeJournalNamespaceAuthorityAt(directory)
	} else {
		var root startupAuthorityRoot
		root, err = captureStartupAuthorityRoot(directory)
		if err == nil {
			authority = &JournalNamespaceAuthority{root: root, planningRoot: root}
		}
	}
	if err != nil {
		if !create && startupAuthorityPathNotFound(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	authority.roots = resolved
	authority.bindingDigest = rootBindingDigest(resolved)
	authority.planningRoot = authority.root
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	return authority, nil
}

// FreezeExistingJournalNamespaceAuthorityForRoots opens and freezes an
// already-created startup authority namespace without creating any directory,
// key, journal, or cleanup residue. It is the phase-zero discovery primitive
// used before a composite read-only fixed point permits authority bootstrap.
func FreezeExistingJournalNamespaceAuthorityForRoots(roots RootSet) (*JournalNamespaceAuthority, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	directory, err := persistentStartupNamespacePath(resolved, false)
	if err != nil {
		return nil, err
	}
	return freezeJournalNamespaceAuthorityForResolvedPath(resolved, directory, false)
}

func freezeJournalNamespaceAuthorityAt(directory string) (*JournalNamespaceAuthority, error) {
	if err := securePrepareStartupAuthorityNamespace(directory); err != nil {
		return nil, err
	}
	resolved, err := canonicalPathWithoutCreate(directory)
	if err != nil {
		return nil, err
	}
	root, err := captureStartupAuthorityRoot(resolved)
	if err != nil {
		return nil, err
	}
	return &JournalNamespaceAuthority{root: root, planningRoot: root}, nil
}

func (authority *JournalNamespaceAuthority) Validate() error {
	if authority == nil {
		return errors.New("startup journal namespace authority is unavailable")
	}
	if authority.bindingDigest != "" && authority.bindingDigest != rootBindingDigest(authority.roots) {
		return errors.New("startup journal namespace root binding changed")
	}
	err := validateStartupAuthorityRoot(authority.root)
	if authority.planningRoot.pathValue() != "" {
		err = errors.Join(err, validateStartupAuthorityRoot(authority.planningRoot))
	}
	return err
}

func (authority *JournalNamespaceAuthority) matchesRoots(roots RootSet) bool {
	return authority != nil && authority.bindingDigest == rootBindingDigest(roots) && authority.roots == roots
}

func (authority *JournalNamespaceAuthority) path() string {
	if authority == nil {
		return ""
	}
	return authority.root.pathValue()
}

func (authority *JournalNamespaceAuthority) planningPath() string {
	if authority == nil {
		return ""
	}
	return authority.planningRoot.pathValue()
}

func persistentStartupNamespacePath(roots RootSet, create bool) (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	configRoot, err = canonicalPathWithoutCreate(configRoot)
	if err != nil {
		return "", err
	}
	base := filepath.Join(configRoot, "analytix", "startup-authority-v1")
	name := "root-" + rootBindingDigest(roots)
	if !filepath.IsAbs(base) || filepath.Base(name) != name {
		return "", errors.New("startup journal persistent namespace input is invalid")
	}
	if create {
		if err := securePrepareStartupAuthorityBase(base); err != nil {
			return "", err
		}
	}
	return filepath.Join(base, name), nil
}

// persistentStartupNamespacePathForUserData binds the desktop startup
// authority to the exact Electron userData root supplied by the host. It must
// never consult HOME, XDG_CONFIG_HOME, APPDATA, or another ambient process
// setting: migration and ordinary runtime activation have to name the same
// namespace even when the host runs against a diagnostic data copy.
func persistentStartupNamespacePathForUserData(
	roots RootSet,
	userDataDir string,
	create bool,
) (string, string, error) {
	if userDataDir == "" || userDataDir != strings.TrimSpace(userDataDir) ||
		!filepath.IsAbs(userDataDir) || filepath.Clean(userDataDir) != userDataDir {
		return "", "", errors.New("startup authority user data root is invalid")
	}
	owners, err := ResolveSeparateOwnerRoots(roots, userDataDir)
	if err != nil || len(owners) != 1 {
		return "", "", errors.New("startup authority user data root is invalid")
	}
	canonicalUserData := owners[0]
	base := filepath.Join(canonicalUserData, "startup-authority-v1")
	name := "root-" + rootBindingDigest(roots)
	if !filepath.IsAbs(base) || filepath.Base(name) != name {
		return "", "", errors.New("startup journal persistent namespace input is invalid")
	}
	if create {
		if err := securePrepareStartupAuthorityBase(base); err != nil {
			return "", "", err
		}
	}
	return filepath.Join(base, name), canonicalUserData, nil
}
