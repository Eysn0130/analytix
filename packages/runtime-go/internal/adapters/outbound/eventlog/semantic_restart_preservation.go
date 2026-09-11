package eventlog

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	threadsummaryindexfs "analytix.local/runtime-go/internal/adapters/outbound/threadsummaryindexfs"
	historymigrationapp "analytix.local/runtime-go/internal/app/historymigration"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

// SemanticRestartPreservationV1 denies transformations of an exact original
// thread tree. The caller supplies IDs from the trusted restart scope and owns
// the isolated semantic stage. This local byte guard neither classifies a
// thread nor replaces the root's complete physical and Core authority proof.
type RestartInheritedHistoryV1 interface {
	ValidateInheritedTurnV1(context.Context, map[string]any, map[string]any) error
}

type SemanticRestartPreservationV1 struct {
	inherited   RestartInheritedHistoryV1
	root        string
	ancestors   map[string]os.FileInfo
	threads     map[string]map[string]semanticPreservedEntryV1
	absent      map[string]bool
	threadPaths map[string]string
	summaries   *threadsummaryindexfs.PreservedRecordsV1
	indexPath   string
}

type semanticPreservedEntryV1 struct {
	info   os.FileInfo
	digest [sha256.Size]byte
}

func PrepareSemanticRestartPreservationV1(ctx context.Context, root, indexPath string, threadIDs []string) (*SemanticRestartPreservationV1, error) {
	return PrepareSemanticRestartPreservationWithAbsentThreadsV1(ctx, root, indexPath, threadIDs, nil)
}

// Absent IDs come from the root's complete original signed child closure.
// They reserve both primary families and shared-index rows without inventing
// an original primary or an event inventory.
func PrepareSemanticRestartPreservationWithAbsentThreadsV1(ctx context.Context, root, indexPath string, threadIDs, absentIDs []string, inherited ...RestartInheritedHistoryV1) (*SemanticRestartPreservationV1, error) {
	if ctx == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || indexPath != filepath.Join(root, "thread_summaries.jsonl") {
		return nil, errors.New("semantic preservation root binding is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() {
		return nil, errors.New("semantic preservation root is unavailable")
	}
	preserved := &SemanticRestartPreservationV1{root: root, ancestors: map[string]os.FileInfo{root: rootInfo}, threads: map[string]map[string]semanticPreservedEntryV1{}, absent: map[string]bool{}, threadPaths: map[string]string{}, indexPath: indexPath}
	if len(inherited) > 1 {
		return nil, errors.New("ambiguous inherited history observation")
	}
	if len(inherited) == 1 {
		preserved.inherited = inherited[0]
	}
	for _, id := range threadIDs {
		if !domainthread.IsCanonicalRecordID(id) || preserved.threads[id] != nil {
			return nil, errors.New("semantic preservation thread inventory is invalid")
		}
		path, err := originalSemanticThreadPathV1(root, id)
		if err != nil {
			return nil, err
		}
		for parent := filepath.Dir(path); parent != root; parent = filepath.Dir(parent) {
			info, err := os.Lstat(parent)
			if err != nil || !info.IsDir() {
				return nil, errors.New("semantic preservation ancestor is invalid")
			}
			preserved.ancestors[parent] = info
		}
		entries, err := readSemanticPreservedTreeV1(ctx, path, id, preserved.inherited)
		if err != nil {
			return nil, err
		}
		preserved.threads[id] = entries
		preserved.threadPaths[id] = path
	}
	for _, id := range absentIDs {
		if !domainthread.IsCanonicalRecordID(id) || preserved.threads[id] != nil {
			return nil, errors.New("semantic absent thread inventory is invalid")
		}
		ancestors, err := observeSemanticAbsentThreadV1(root, id)
		if err != nil {
			return nil, err
		}
		for path, info := range ancestors {
			preserved.ancestors[path] = info
		}
		preserved.absent[id] = true
		preserved.threads[id] = map[string]semanticPreservedEntryV1{}
	}
	allIDs := append(append([]string(nil), threadIDs...), absentIDs...)
	preserved.summaries, err = threadsummaryindexfs.PreparePreservedRecordsV1(ctx, indexPath, allIDs)
	if err != nil {
		return nil, err
	}
	if err := preserved.revalidate(ctx, root, indexPath); err != nil {
		return nil, err
	}
	return preserved, nil
}

func (preserved *SemanticRestartPreservationV1) ownsThread(id string) bool {
	return preserved != nil && preserved.threads[id] != nil
}

// OwnsThread is denial-only and retains IDs whose original files are in the
// legacy family. It does not promote or move those files into the live family.
func (preserved *SemanticRestartPreservationV1) OwnsThread(id string) bool {
	return preserved.ownsThread(id)
}

func (preserved *SemanticRestartPreservationV1) ThreadIDsV1() []string {
	if preserved == nil {
		return nil
	}
	ids := make([]string, 0, len(preserved.threads))
	for id := range preserved.threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (preserved *SemanticRestartPreservationV1) Revalidate(ctx context.Context, root string) error {
	return preserved.revalidate(ctx, root, "")
}

// ReadOriginalEventInventoryV1 observes every original row in its original
// family and order. It neither recovers a journal nor normalizes private or
// historical event contents. This is internal preservation evidence, never
// ordinary replay, public projection, or committed publication authority.
func (preserved *SemanticRestartPreservationV1) ReadOriginalEventInventoryV1(ctx context.Context, id string) ([]map[string]any, error) {
	if !preserved.ownsThread(id) || preserved.absent[id] {
		return nil, errors.New("thread is outside original event preservation inventory")
	}
	if err := preserved.Revalidate(ctx, preserved.root); err != nil {
		return nil, err
	}
	rows, _, readErr := readStrictMigrationRecords(filepath.Join(preserved.threadPaths[id], "events.jsonl"), func(record map[string]any, _ int, _ string) (map[string]any, bool, error) {
		return record, false, nil
	})
	if err := preserved.Revalidate(ctx, preserved.root); err != nil {
		return nil, err
	}
	if readErr != nil {
		return nil, readErr
	}
	return rows, nil
}

func observeSemanticAbsentThreadV1(root, id string) (map[string]os.FileInfo, error) {
	ancestors := map[string]os.FileInfo{}
	for _, parts := range [][]string{{"threads", id}, {"runtime-go", "threads", id}} {
		path := root
		for index, part := range parts {
			path = filepath.Join(path, part)
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return nil, err
			}
			if !info.IsDir() {
				return nil, errors.New("semantic absent thread ancestor is invalid")
			}
			if index == len(parts)-1 {
				return nil, errors.New("semantic reserved absent thread appeared")
			}
			ancestors[path] = info
		}
	}
	return ancestors, nil
}

func (preserved *SemanticRestartPreservationV1) SummaryRecordsV1() *threadsummaryindexfs.PreservedRecordsV1 {
	if preserved == nil {
		return nil
	}
	return preserved.summaries
}

func originalSemanticThreadPathV1(root, id string) (string, error) {
	var original string
	for _, path := range []string{filepath.Join(root, "threads", id), filepath.Join(root, "runtime-go", "threads", id)} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() {
			return "", errors.New("semantic preserved thread directory is unavailable")
		}
		if original != "" {
			return "", errors.New("semantic preserved thread exists in both primary families")
		}
		original = path
	}
	if original == "" {
		return "", errors.New("semantic preserved original thread is missing")
	}
	return original, nil
}

func (preserved *SemanticRestartPreservationV1) revalidate(ctx context.Context, root, indexPath string) error {
	if preserved == nil {
		return nil
	}
	if root != preserved.root || (indexPath != "" && indexPath != preserved.indexPath) {
		return errors.New("semantic preservation belongs to another root")
	}
	if ctx == nil {
		return errors.New("semantic preservation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for path, want := range preserved.ancestors {
		got, err := os.Lstat(path)
		if err != nil || !os.SameFile(want, got) || want.Mode() != got.Mode() {
			return errors.New("semantic preservation ancestor changed")
		}
	}
	for id, want := range preserved.threads {
		if preserved.absent[id] {
			if _, err := observeSemanticAbsentThreadV1(root, id); err != nil {
				return err
			}
			continue
		}
		path, err := originalSemanticThreadPathV1(root, id)
		if err != nil {
			return err
		}
		if path != preserved.threadPaths[id] {
			return errors.New("semantic preserved primary family changed")
		}
		got, err := readSemanticPreservedTreeV1(ctx, path, id, preserved.inherited)
		if err != nil {
			return err
		}
		if len(want) != len(got) {
			return errors.New("semantic preserved tree inventory changed")
		}
		for path, before := range want {
			after, ok := got[path]
			if !ok || !sameSemanticPreservedEntryV1(before, after) {
				return errors.New("semantic preserved tree changed")
			}
		}
	}
	return preserved.summaries.Revalidate(ctx)
}

func sameSemanticPreservedEntryV1(before, after semanticPreservedEntryV1) bool {
	if !os.SameFile(before.info, after.info) || before.info.Mode() != after.info.Mode() || before.digest != after.digest {
		return false
	}
	return before.info.IsDir() || (before.info.Size() == after.info.Size() && before.info.ModTime().Equal(after.info.ModTime()))
}

func readSemanticPreservedTreeV1(ctx context.Context, dir, id string, inherited ...RestartInheritedHistoryV1) (map[string]semanticPreservedEntryV1, error) {
	entries := map[string]semanticPreservedEntryV1{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := os.Lstat(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if (name == "thread.json" || name == "messages.jsonl" || name == "metadata.jsonl" || name == "events.jsonl") && !before.Mode().IsRegular() {
			return errors.New("semantic preserved file slot is not a regular file")
		}
		if before.IsDir() {
			entries[name] = semanticPreservedEntryV1{info: before}
			return nil
		}
		if !before.Mode().IsRegular() || before.Size() > domainstartup.MaxSemanticManagedFileBytesV1 {
			return errors.New("semantic preserved tree has an invalid entry")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(before, opened) {
			return errors.Join(errors.New("semantic preserved file changed while opening"), statErr, file.Close())
		}
		body, readErr := io.ReadAll(io.LimitReader(file, domainstartup.MaxSemanticManagedFileBytesV1+1))
		if err := errors.Join(readErr, file.Close()); err != nil {
			return err
		}
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != int64(len(body)) || !before.ModTime().Equal(after.ModTime()) {
			return errors.New("semantic preserved file changed during read")
		}
		if name == "thread.json" {
			primary, err := strictPreservedHistoryObjectV1(body, int(domainstartup.MaxSemanticManagedFileBytesV1))
			if err != nil {
				return err
			}
			if err := domainthread.ValidatePrimaryIdentityV1(id, primary); err != nil {
				return err
			}
			if err := validatePreservedSemanticThreadAuthorityWithHistoryV1(ctx, primary, inherited...); err != nil {
				return err
			}
			if semanticTypedCurrentAuthorityContainerV1(primary) {
				if err := preflightCurrentExecutionEventsV1(filepath.Join(dir, "events.jsonl"), primary); err != nil {
					return err
				}
			}
		}
		if name == "messages.jsonl" || name == "metadata.jsonl" || name == "events.jsonl" {
			if _, _, err := readStrictMigrationRecordsWithDecoderV1(path, func(record map[string]any, _ int, _ string) (map[string]any, bool, error) {
				if name == "metadata.jsonl" {
					thread, ok := record["thread"].(map[string]any)
					if !ok || migrationStringField(thread, "id") != id {
						return nil, false, errors.New("preserved metadata thread identity is invalid")
					}
					if err := validatePreservedSemanticThreadAuthorityWithHistoryV1(ctx, thread, inherited...); err != nil {
						return nil, false, err
					}
				}
				return record, false, nil
			}, strictPreservedHistoryObjectV1); err != nil {
				return err
			}
		}
		entries[name] = semanticPreservedEntryV1{info: before, digest: sha256.Sum256(body)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, exists := entries["thread.json"]; !exists {
		return nil, errors.New("semantic preservation requires the original primary")
	}
	allowHistoricalOrder, err := isMigratableEventDirectoryV1(dir)
	if err != nil {
		return nil, err
	}
	if err := processLegacyEventSequenceFileV1(filepath.Join(dir, "events.jsonl"), id, allowHistoricalOrder, false); err != nil {
		return nil, err
	}
	return entries, nil
}

func strictPreservedHistoryObjectV1(body []byte, maxBytes int) (map[string]any, error) {
	// Original preservation compares exact records with the strict primary
	// scope. Preserve numeric values and spelling instead of rounding float64.
	return jsonstrict.DecodeObject(body, strictMigrationJSONOptionsV1(maxBytes))
}

func validatePreservedSemanticThreadAuthorityV1(thread map[string]any) error {
	return validatePreservedSemanticThreadAuthorityWithHistoryV1(context.Background(), thread)
}

func validatePreservedSemanticThreadAuthorityWithHistoryV1(ctx context.Context, thread map[string]any, inherited ...RestartInheritedHistoryV1) error {
	// Preserve the existing execution owner's rejecting checks without applying
	// its projection. Derived layouts cannot use the ordinary migration's
	// stripping exception when their original authority must remain intact.
	if _, _, err := historymigrationapp.StripUntrustedExecutionAuthority(thread); err != nil {
		return err
	}
	if semanticTypedCurrentAuthorityContainerV1(thread) {
		if len(inherited) == 1 && inherited[0] != nil {
			return validateCurrentAuthorityThreadPrivacyV1(thread, func(thread, turn map[string]any) error {
				return inherited[0].ValidateInheritedTurnV1(ctx, thread, turn)
			})
		}
		return validateCurrentAuthorityThreadPrivacyV1(thread)
	}
	return nil
}
