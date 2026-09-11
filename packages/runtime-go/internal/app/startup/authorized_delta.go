package startup

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// AuthorizedManagedFileAdditionV1 identifies one exact host-authored file that
// may appear between two otherwise immutable startup snapshots.
type AuthorizedManagedFileAdditionV1 struct {
	Path            string
	Mode            uint32
	Size            int64
	ModTimeUnixNano int64
	SHA256          string
	RecordCount     int
}

type AuthorizedManagedDirectoryV1 struct {
	Path            string
	Mode            uint32
	Size            int64
	ModTimeUnixNano int64
}

// AuthorizedManagedDeltaV1 is deliberately narrower than a generic snapshot
// diff. It permits exact new files plus metadata changes to their explicitly
// named parent directories; every other entry must remain byte-for-byte equal.
type AuthorizedManagedDeltaV1 struct {
	AddedFiles         []AuthorizedManagedFileAdditionV1
	MutableDirectories []AuthorizedManagedDirectoryV1
}

func (delta AuthorizedManagedDeltaV1) HasChanges() bool {
	return len(delta.AddedFiles) > 0
}

// ValidateAuthorizedManagedDeltaV1 proves that after differs from before only
// by the exact host-issued additions in delta. It is used during authenticated
// old-transaction recovery before the first semantic baseline is captured; an
// unconstrained second capture must never become startup authority.
func ValidateAuthorizedManagedDeltaV1(
	before domainstartup.ManagedSnapshotV1,
	after domainstartup.ManagedSnapshotV1,
	delta AuthorizedManagedDeltaV1,
) error {
	if domainstartup.ValidateManagedSnapshotV1(before) != nil ||
		domainstartup.ValidateManagedSnapshotV1(after) != nil ||
		before.RootBindingDigest != after.RootBindingDigest || !delta.HasChanges() {
		return errors.New("authorized startup delta input is invalid")
	}
	files := make(map[string]AuthorizedManagedFileAdditionV1, len(delta.AddedFiles))
	for _, file := range delta.AddedFiles {
		file.Path = strings.TrimSpace(file.Path)
		file.SHA256 = strings.TrimSpace(file.SHA256)
		if !validAuthorizedManagedPath(file.Path) || !domainsecurity.IsSHA256Hex(file.SHA256) ||
			file.Size <= 0 || file.RecordCount <= 0 {
			return errors.New("authorized startup file addition is invalid")
		}
		if _, exists := files[file.Path]; exists {
			return errors.New("authorized startup file addition is duplicated")
		}
		files[file.Path] = file
	}
	directories := make(map[string]AuthorizedManagedDirectoryV1, len(delta.MutableDirectories))
	for _, directory := range delta.MutableDirectories {
		directory.Path = strings.TrimSpace(directory.Path)
		if !validAuthorizedManagedPath(directory.Path) || directory.Mode == 0 || directory.Size < 0 {
			return errors.New("authorized startup directory is invalid")
		}
		if _, exists := directories[directory.Path]; exists {
			return errors.New("authorized startup directory is duplicated")
		}
		directories[directory.Path] = directory
	}
	if len(directories) == 0 {
		return errors.New("authorized startup delta has no directory authority")
	}

	beforeEntries := managedEntriesByPath(before.Entries)
	afterEntries := managedEntriesByPath(after.Entries)
	paths := make([]string, 0, len(beforeEntries)+len(afterEntries))
	seen := make(map[string]struct{}, len(beforeEntries)+len(afterEntries))
	for path := range beforeEntries {
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for path := range afterEntries {
		if _, exists := seen[path]; !exists {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	observedFiles := make(map[string]struct{}, len(files))
	observedDirectories := make(map[string]struct{}, len(directories))
	for _, path := range paths {
		prior, hadPrior := beforeEntries[path]
		current, hasCurrent := afterEntries[path]
		if expected, authorized := files[path]; authorized {
			want := domainstartup.ManagedEntryStateV1{
				Path: expected.Path, Type: domainstartup.ManagedEntryTypeFile, Mode: expected.Mode,
				Size: expected.Size, ModTimeUnixNano: expected.ModTimeUnixNano,
				SHA256: expected.SHA256, RecordCount: expected.RecordCount,
			}
			if hadPrior || !hasCurrent || current != want {
				return fmt.Errorf("authorized startup file delta does not match %s", path)
			}
			observedFiles[path] = struct{}{}
			continue
		}
		if expected, authorized := directories[path]; authorized {
			want := domainstartup.ManagedEntryStateV1{
				Path: expected.Path, Type: domainstartup.ManagedEntryTypeDirectory, Mode: expected.Mode,
				Size: expected.Size, ModTimeUnixNano: expected.ModTimeUnixNano,
			}
			if !hasCurrent || current != want {
				return fmt.Errorf("authorized startup directory delta does not match %s", path)
			}
			if hadPrior && prior.Type != domainstartup.ManagedEntryTypeDirectory {
				return fmt.Errorf("authorized startup directory identity changed at %s", path)
			}
			observedDirectories[path] = struct{}{}
			continue
		}
		if hadPrior && hasCurrent && prior == current {
			continue
		}
		return fmt.Errorf("managed persistence changed outside authorized startup delta at %s", path)
	}
	if len(observedFiles) != len(files) {
		return errors.New("authorized startup delta did not materialize every exact file")
	}
	if len(observedDirectories) != len(directories) {
		return errors.New("authorized startup delta did not bind every exact directory")
	}
	return nil
}

func managedEntriesByPath(entries []domainstartup.ManagedEntryStateV1) map[string]domainstartup.ManagedEntryStateV1 {
	result := make(map[string]domainstartup.ManagedEntryStateV1, len(entries))
	for _, entry := range entries {
		result[entry.Path] = entry
	}
	return result
}

func validAuthorizedManagedPath(path string) bool {
	if path == "" || path != strings.TrimSpace(path) || strings.HasPrefix(path, "/") ||
		strings.HasSuffix(path, "/") || strings.Contains(path, "\\") {
		return false
	}
	for _, component := range strings.Split(path, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}
