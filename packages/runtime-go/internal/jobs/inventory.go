package jobs

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const ChildRunInventorySchemaVersionV1 = 1

const (
	childRunFilesystemIdentityUnixV1    = "unix_dev_inode"
	childRunFilesystemIdentityWindowsV1 = "windows_volume_file_id_128"
)

// ErrChildRunFilesystemIdentityUnavailable means this platform cannot prove
// the stable object identity and link-count binding required by the migration.
// A caller must leave the live child-runs tree untouched and keep migration
// activation disabled when this error is returned.
var ErrChildRunFilesystemIdentityUnavailable = errors.New("child-run stable filesystem identity is unavailable")

type ChildRunInventoryEntryKindV1 string

const (
	ChildRunInventoryRecordV1                 ChildRunInventoryEntryKindV1 = "record"
	ChildRunInventoryArtifactV1               ChildRunInventoryEntryKindV1 = "artifact"
	ChildRunInventoryTemporaryV1              ChildRunInventoryEntryKindV1 = "runtime_temporary"
	ChildRunInventoryLegacyTypeScriptRecordV1 ChildRunInventoryEntryKindV1 = "legacy_typescript_record"
	// ChildRunInventoryLegacyTemporaryV1 is accepted only so a later
	// migration can delete residue emitted by the previously deployed
	// os.CreateTemp writer. It is never a source of record content.
	ChildRunInventoryLegacyTemporaryV1 ChildRunInventoryEntryKindV1 = "legacy_runtime_temporary"
)

const childRunTemporaryRandomBytesV1 = 16

const childRunInventoryRecordMaxBytesV1 = 16 * 1024 * 1024

const (
	childRunInventoryOpaqueMaxBytesV1 = 64 * 1024 * 1024
	childRunInventoryMaxObjectsV1     = 100_000
	childRunInventoryReadDirBatchV1   = 512
)

// createChildRunTemporaryFileV1 is the sole producer for the versioned
// child-run atomic-write name grammar. The fixed 128-bit lowercase-hex suffix
// lets the read-only inventory distinguish producer-owned residue from
// lookalike names without trusting any residue content.
func createChildRunTemporaryFileV1(dir string, finalBase string) (*os.File, error) {
	jobID, ok := exactChildRunJobNameV1(finalBase, ".json")
	if !ok || finalBase != jobID+".json" || filepath.Base(finalBase) != finalBase {
		return nil, errors.New("child-run temporary final name is invalid")
	}
	for attempt := 0; attempt < 128; attempt++ {
		random := make([]byte, childRunTemporaryRandomBytesV1)
		if _, err := cryptorand.Read(random); err != nil {
			return nil, errors.New("child-run temporary randomness is unavailable")
		}
		name := "." + finalBase + ".tmp-v1-" + hex.EncodeToString(random)
		handle, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return handle, nil
	}
	return nil, errors.New("child-run temporary name collision limit reached")
}

// ChildRunFilesystemIdentityV1 is a host-observed filesystem identity. Unix
// device/inode values are lossless decimal strings. On Windows, Device carries
// the lossless volume serial and Inode carries the complete 128-bit FileIdInfo
// value as lowercase hex. LinkCount must be one for every admitted file.
type ChildRunFilesystemIdentityV1 struct {
	Scheme    string `json:"scheme"`
	Device    string `json:"device"`
	Inode     string `json:"inode"`
	LinkCount uint64 `json:"linkCount"`
}

// ChildRunInventoryEntryV1 binds a recognized child-run object to its exact
// path, filesystem identity, metadata, and complete byte digest. Inventorying
// does not parse artifact or temporary-file content.
type ChildRunInventoryEntryV1 struct {
	Name             string                       `json:"name"`
	Path             string                       `json:"path"`
	Kind             ChildRunInventoryEntryKindV1 `json:"kind"`
	JobID            string                       `json:"jobId"`
	Identity         ChildRunFilesystemIdentityV1 `json:"identity"`
	SizeBytes        int64                        `json:"sizeBytes"`
	Mode             uint32                       `json:"mode"`
	ModifiedUnixNano int64                        `json:"modifiedUnixNano"`
	SHA256           string                       `json:"sha256"`
}

// ChildRunInventoryV1 is a deterministic, read-only migration-plan input. It
// contains no observation timestamp: unchanged directory bytes produce the
// same manifest on every scan. Call ValidateChildRunInventoryV1 immediately
// before consuming a previously built plan to reject replacement or drift.
type ChildRunInventoryV1 struct {
	SchemaVersion        int                          `json:"schemaVersion"`
	Root                 string                       `json:"root"`
	RootExists           bool                         `json:"rootExists"`
	RootIdentity         ChildRunFilesystemIdentityV1 `json:"rootIdentity"`
	RootSizeBytes        int64                        `json:"rootSizeBytes"`
	RootMode             uint32                       `json:"rootMode"`
	RootModifiedUnixNano int64                        `json:"rootModifiedUnixNano"`
	Entries              []ChildRunInventoryEntryV1   `json:"entries"`
	ManifestSHA256       string                       `json:"manifestSha256"`
}

// SameChildRunRootAuthorityV1 compares two independently validated directory
// observations across permitted entry changes. Directory link count is mutable
// on supported filesystems; stable object identity and root mode must remain.
// Complete entry identity/link-count validation still belongs to the inventory.
func SameChildRunRootAuthorityV1(before, after ChildRunInventoryV1) bool {
	return before.RootExists && after.RootExists && before.Root == after.Root && before.RootMode == after.RootMode &&
		before.RootIdentity.Scheme == after.RootIdentity.Scheme && before.RootIdentity.Device == after.RootIdentity.Device &&
		before.RootIdentity.Inode == after.RootIdentity.Inode
}

type childRunInventoryHooksV1 struct {
	afterFileOpen    func(path string) error
	betweenSnapshots func() error
	maxObjects       int
}

// BuildChildRunInventoryV1 observes the child-runs root twice and succeeds
// only when both complete snapshots are identical. It never creates, rewrites,
// parses, renames, or removes any child-run object.
func BuildChildRunInventoryV1(root string) (ChildRunInventoryV1, error) {
	return buildChildRunInventoryV1(root, childRunInventoryHooksV1{})
}

// readChildRunInventoryEntryV1 returns only committed record bytes observed
// through the same handle whose complete digest and identity match expected.
// The retired TypeScript producer's exact child_<8>_<6>.json grammar is
// readable only so the isolated semantic-startup stage can project it into a
// current job-N record. Artifact and temporary bytes are never parseable
// migration input.
func readChildRunInventoryEntryV1(root string, expected ChildRunInventoryEntryV1) ([]byte, error) {
	return readChildRunInventoryEntryWithHookV1(root, expected, nil)
}

func readChildRunInventoryEntryWithHookV1(
	root string,
	expected ChildRunInventoryEntryV1,
	afterFileOpen func(path string) error,
) ([]byte, error) {
	canonicalRoot, err := canonicalChildRunInventoryRootV1(root)
	if err != nil {
		return nil, err
	}
	if err := validateChildRunInventoryEntryShapeV1(canonicalRoot, expected); err != nil {
		return nil, err
	}
	if expected.Kind != ChildRunInventoryRecordV1 && expected.Kind != ChildRunInventoryLegacyTypeScriptRecordV1 {
		return nil, errors.New("only committed child-run records may be read as migration input")
	}
	if expected.SizeBytes > childRunInventoryRecordMaxBytesV1 {
		return nil, errors.New("child-run record exceeds the migration input limit")
	}
	observed, data, err := observeChildRunInventoryEntryBytesV1(
		canonicalRoot, expected.Name, expected.Kind, expected.JobID, true, afterFileOpen,
	)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(expected, observed) {
		return nil, errors.New("child-run record no longer matches the inventory plan")
	}
	return data, nil
}

type childRunResidueRemovalHooksV1 struct {
	afterFirstValidation          func(path string) error
	allowLegacyTypeScriptRecordV1 bool
}

// removeChildRunInventoryResidueV1 removes only an exact, revalidated retired
// artifact, temporary residue, or retired TypeScript record after its current
// job-N projection has been staged. It must be called under the startup
// single-writer barrier: pathname unlink cannot itself lock out an unrelated
// host writer. Committed job-N.json records are never removable through this
// primitive.
func removeChildRunInventoryResidueV1(root string, expected ChildRunInventoryEntryV1) error {
	return removeChildRunInventoryResidueWithHooksV1(root, expected, childRunResidueRemovalHooksV1{})
}

func removeChildRunInventoryResidueWithHooksV1(
	root string,
	expected ChildRunInventoryEntryV1,
	hooks childRunResidueRemovalHooksV1,
) error {
	canonicalRoot, err := canonicalChildRunInventoryRootV1(root)
	if err != nil {
		return err
	}
	if err := validateChildRunInventoryEntryShapeV1(canonicalRoot, expected); err != nil {
		return err
	}
	switch expected.Kind {
	case ChildRunInventoryArtifactV1, ChildRunInventoryTemporaryV1, ChildRunInventoryLegacyTemporaryV1:
	case ChildRunInventoryLegacyTypeScriptRecordV1:
		if !hooks.allowLegacyTypeScriptRecordV1 {
			return errors.New("retired TypeScript child-run records require dedicated staged removal")
		}
	case ChildRunInventoryRecordV1:
		return errors.New("committed child-run records cannot be removed as residue")
	default:
		return errors.New("child-run residue kind is invalid")
	}
	rootBefore, err := os.Lstat(canonicalRoot)
	if err != nil || rootBefore.Mode()&os.ModeSymlink != 0 || !rootBefore.IsDir() {
		return errors.New("child-run inventory root is not an authoritative directory")
	}
	rootHandle, err := os.Open(canonicalRoot)
	if err != nil {
		return errors.New("child-run inventory root cannot be opened")
	}
	defer rootHandle.Close()
	openedRoot, err := rootHandle.Stat()
	if err != nil || !os.SameFile(rootBefore, openedRoot) {
		return errors.New("child-run inventory root identity changed while opening")
	}
	rootIdentity, err := childRunFilesystemIdentityFromHandleV1(rootHandle, false)
	if err != nil {
		return err
	}
	if err := validateObservedChildRunInventoryEntryV1(canonicalRoot, expected); err != nil {
		return err
	}
	if hooks.afterFirstValidation != nil {
		if err := hooks.afterFirstValidation(expected.Path); err != nil {
			return err
		}
	}
	// Re-read and re-hash after acquiring the caller's startup barrier and as
	// close as possible to unlink. This catches replacement after planning and
	// deterministic races injected between validation phases.
	if err := validateObservedChildRunInventoryEntryV1(canonicalRoot, expected); err != nil {
		return err
	}
	rootBeforeRemove, err := os.Lstat(canonicalRoot)
	if err != nil || !os.SameFile(openedRoot, rootBeforeRemove) {
		return errors.New("child-run inventory root path was replaced before residue removal")
	}
	if err := os.Remove(expected.Path); err != nil {
		return err
	}
	if err := syncChildRunDirectoryV1(canonicalRoot, rootIdentity); err != nil {
		return fmt.Errorf("child-run residue removal directory sync failed: %w", err)
	}
	if _, err := os.Lstat(expected.Path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("child-run residue removal did not remain absent")
	}
	rootAfter, err := os.Lstat(canonicalRoot)
	if err != nil || !os.SameFile(openedRoot, rootAfter) {
		return errors.New("child-run inventory root path was replaced after residue removal")
	}
	return nil
}

func removeLegacyTypeScriptChildRunSourceV1(root string, expected ChildRunInventoryEntryV1) error {
	if expected.Kind != ChildRunInventoryLegacyTypeScriptRecordV1 {
		return errors.New("retired TypeScript child-run source kind is invalid")
	}
	return removeChildRunInventoryResidueWithHooksV1(root, expected, childRunResidueRemovalHooksV1{
		allowLegacyTypeScriptRecordV1: true,
	})
}

func validateObservedChildRunInventoryEntryV1(root string, expected ChildRunInventoryEntryV1) error {
	observed, _, err := observeChildRunInventoryEntryBytesV1(
		root, expected.Name, expected.Kind, expected.JobID, false, nil,
	)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, observed) {
		return errors.New("child-run residue no longer matches the inventory plan")
	}
	return nil
}

// ValidateChildRunInventoryV1 validates an untrusted planned inventory,
// re-observes the authoritative root twice, and requires an exact match. The
// separate root argument prevents a self-consistent plan from redirecting the
// validator to another directory.
func ValidateChildRunInventoryV1(root string, expected ChildRunInventoryV1) error {
	canonicalRoot, err := canonicalChildRunInventoryRootV1(root)
	if err != nil {
		return err
	}
	if err := validateChildRunInventoryShapeV1(canonicalRoot, expected); err != nil {
		return err
	}
	observed, err := buildChildRunInventoryV1(canonicalRoot, childRunInventoryHooksV1{})
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, observed) {
		return errors.New("child-run inventory no longer matches the migration plan")
	}
	return nil
}

func buildChildRunInventoryV1(root string, hooks childRunInventoryHooksV1) (ChildRunInventoryV1, error) {
	canonicalRoot, err := canonicalChildRunInventoryRootV1(root)
	if err != nil {
		return ChildRunInventoryV1{}, err
	}
	maxObjects := hooks.maxObjects
	if maxObjects <= 0 {
		maxObjects = childRunInventoryMaxObjectsV1
	}
	first, err := scanChildRunInventoryV1(canonicalRoot, maxObjects, hooks.afterFileOpen)
	if err != nil {
		return ChildRunInventoryV1{}, err
	}
	if hooks.betweenSnapshots != nil {
		if err := hooks.betweenSnapshots(); err != nil {
			return ChildRunInventoryV1{}, err
		}
	}
	second, err := scanChildRunInventoryV1(canonicalRoot, maxObjects, hooks.afterFileOpen)
	if err != nil {
		return ChildRunInventoryV1{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return ChildRunInventoryV1{}, errors.New("child-run inventory changed between stable snapshots")
	}
	manifest, err := childRunInventoryManifestSHA256V1(first)
	if err != nil {
		return ChildRunInventoryV1{}, err
	}
	first.ManifestSHA256 = manifest
	return first, nil
}

func scanChildRunInventoryV1(root string, maxObjects int, afterFileOpen func(path string) error) (ChildRunInventoryV1, error) {
	inventory := ChildRunInventoryV1{
		SchemaVersion: ChildRunInventorySchemaVersionV1,
		Root:          root,
		Entries:       []ChildRunInventoryEntryV1{},
	}
	rootBefore, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return inventory, nil
	}
	if err != nil {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root cannot be inspected")
	}
	if rootBefore.Mode()&os.ModeSymlink != 0 || !rootBefore.IsDir() {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root is not an authoritative directory")
	}
	rootHandle, err := os.Open(root)
	if err != nil {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root cannot be opened")
	}
	defer rootHandle.Close()
	openedRoot, err := rootHandle.Stat()
	if err != nil || !os.SameFile(rootBefore, openedRoot) {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root identity changed while opening")
	}
	if !sameChildRunFileObservationV1(rootBefore, openedRoot) {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root metadata changed while opening")
	}
	rootIdentity, err := childRunFilesystemIdentityFromHandleV1(rootHandle, false)
	if err != nil {
		return ChildRunInventoryV1{}, err
	}
	entries := []os.DirEntry{}
	for {
		batch, readErr := rootHandle.ReadDir(childRunInventoryReadDirBatchV1)
		if len(entries)+len(batch) > maxObjects {
			return ChildRunInventoryV1{}, errors.New("child-run inventory object count exceeds the migration limit")
		}
		entries = append(entries, batch...)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return ChildRunInventoryV1{}, errors.New("child-run inventory root cannot be enumerated")
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	seenIdentities := map[string]string{}
	for _, directoryEntry := range entries {
		name := directoryEntry.Name()
		kind, jobID, ok := classifyChildRunInventoryNameV1(name)
		if !ok {
			return ChildRunInventoryV1{}, errors.New("child-run inventory contains an unknown object")
		}
		entry, err := observeChildRunInventoryEntryV1(root, name, kind, jobID, afterFileOpen)
		if err != nil {
			return ChildRunInventoryV1{}, err
		}
		identityKey := entry.Identity.Scheme + ":" + entry.Identity.Device + ":" + entry.Identity.Inode
		if _, exists := seenIdentities[identityKey]; exists {
			return ChildRunInventoryV1{}, errors.New("child-run inventory objects share a filesystem identity")
		}
		seenIdentities[identityKey] = name
		inventory.Entries = append(inventory.Entries, entry)
	}
	rootAfterHandle, err := rootHandle.Stat()
	if err != nil || !sameChildRunFileObservationV1(rootBefore, rootAfterHandle) {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root changed during enumeration")
	}
	rootAfterIdentity, err := childRunFilesystemIdentityFromHandleV1(rootHandle, false)
	if err != nil || rootAfterIdentity != rootIdentity {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root identity changed during enumeration")
	}
	rootAfterPath, err := os.Lstat(root)
	if err != nil || rootAfterPath.Mode()&os.ModeSymlink != 0 || !os.SameFile(rootBefore, rootAfterPath) ||
		!sameChildRunFileObservationV1(rootBefore, rootAfterPath) {
		return ChildRunInventoryV1{}, errors.New("child-run inventory root path was replaced during enumeration")
	}
	inventory.RootExists = true
	inventory.RootIdentity = rootIdentity
	inventory.RootSizeBytes = rootBefore.Size()
	inventory.RootMode = uint32(rootBefore.Mode())
	inventory.RootModifiedUnixNano = rootBefore.ModTime().UnixNano()
	return inventory, nil
}

func observeChildRunInventoryEntryV1(
	root string,
	name string,
	kind ChildRunInventoryEntryKindV1,
	jobID string,
	afterFileOpen func(path string) error,
) (ChildRunInventoryEntryV1, error) {
	entry, _, err := observeChildRunInventoryEntryBytesV1(root, name, kind, jobID, false, afterFileOpen)
	return entry, err
}

func observeChildRunInventoryEntryBytesV1(
	root string,
	name string,
	kind ChildRunInventoryEntryKindV1,
	jobID string,
	captureBytes bool,
	afterFileOpen func(path string) error,
) (ChildRunInventoryEntryV1, []byte, error) {
	path := filepath.Join(root, name)
	pathBefore, err := os.Lstat(path)
	if err != nil {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object cannot be inspected")
	}
	if pathBefore.Mode()&os.ModeSymlink != 0 {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object is a symlink")
	}
	if !pathBefore.Mode().IsRegular() {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object is not a regular file")
	}
	handle, err := os.Open(path)
	if err != nil {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object cannot be opened")
	}
	defer handle.Close()
	openedBefore, err := handle.Stat()
	if err != nil || !os.SameFile(pathBefore, openedBefore) || !sameChildRunFileObservationV1(pathBefore, openedBefore) {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object changed while opening")
	}
	identity, err := childRunFilesystemIdentityFromHandleV1(handle, true)
	if err != nil {
		return ChildRunInventoryEntryV1{}, nil, fmt.Errorf("child-run inventory object identity is invalid: %w", err)
	}
	if afterFileOpen != nil {
		if err := afterFileOpen(path); err != nil {
			return ChildRunInventoryEntryV1{}, nil, err
		}
	}
	maximumBytes, ok := childRunInventoryMaxBytesForKindV1(kind)
	if !ok || openedBefore.Size() < 0 || openedBefore.Size() > maximumBytes {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object exceeds the migration size limit")
	}
	hasher := sha256.New()
	var captured bytes.Buffer
	writer := io.Writer(hasher)
	if captureBytes {
		captured.Grow(int(openedBefore.Size()))
		writer = io.MultiWriter(hasher, &captured)
	}
	readBytes, err := io.Copy(writer, io.LimitReader(handle, maximumBytes+1))
	if err != nil || readBytes > maximumBytes || readBytes != openedBefore.Size() {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object changed while hashing")
	}
	openedAfter, err := handle.Stat()
	if err != nil || !sameChildRunFileObservationV1(openedBefore, openedAfter) {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object changed while hashing")
	}
	pathAfter, err := os.Lstat(path)
	if err != nil || pathAfter.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedAfter, pathAfter) ||
		!sameChildRunFileObservationV1(openedAfter, pathAfter) {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object path was replaced while hashing")
	}
	afterIdentity, err := childRunFilesystemIdentityFromHandleV1(handle, true)
	if err != nil || afterIdentity != identity {
		return ChildRunInventoryEntryV1{}, nil, errors.New("child-run inventory object identity changed while hashing")
	}
	return ChildRunInventoryEntryV1{
		Name:             name,
		Path:             path,
		Kind:             kind,
		JobID:            jobID,
		Identity:         identity,
		SizeBytes:        openedAfter.Size(),
		Mode:             uint32(openedAfter.Mode()),
		ModifiedUnixNano: openedAfter.ModTime().UnixNano(),
		SHA256:           hex.EncodeToString(hasher.Sum(nil)),
	}, captured.Bytes(), nil
}

func canonicalChildRunInventoryRootV1(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("child-run inventory root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("child-run inventory root is invalid")
	}
	absolute = filepath.Clean(absolute)
	if info, statErr := os.Lstat(absolute); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("child-run inventory root cannot be a symlink")
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", errors.New("child-run inventory root cannot be inspected")
	}
	canonical, err := canonicalJobRootWithoutCreate(absolute)
	if err != nil {
		return "", errors.New("child-run inventory root is invalid")
	}
	return canonical, nil
}

func classifyChildRunInventoryNameV1(name string) (ChildRunInventoryEntryKindV1, string, bool) {
	if jobID, ok := exactChildRunJobNameV1(name, ".json"); ok {
		return ChildRunInventoryRecordV1, jobID, true
	}
	if childID, ok := exactLegacyTypeScriptChildRunNameV1(name); ok {
		return ChildRunInventoryLegacyTypeScriptRecordV1, childID, true
	}
	if jobID, ok := exactChildRunJobNameV1(name, ".log"); ok {
		return ChildRunInventoryArtifactV1, jobID, true
	}
	const prefix = "."
	const currentSeparator = ".json.tmp-v1-"
	const legacySeparator = ".json.tmp-"
	if strings.HasPrefix(name, prefix) {
		remainder := strings.TrimPrefix(name, prefix)
		jobID, random, found := strings.Cut(remainder, currentSeparator)
		if found && validPersistedJobID(jobID) && validRuntimeTemporaryHexV1(random) {
			return ChildRunInventoryTemporaryV1, jobID, true
		}
		jobID, random, found = strings.Cut(remainder, legacySeparator)
		if found && validPersistedJobID(jobID) && validLegacyRuntimeTemporaryRandomV1(random) {
			return ChildRunInventoryLegacyTemporaryV1, jobID, true
		}
	}
	return "", "", false
}

// ClassifyChildRunInventoryNameV1 shares the exact on-disk producer grammar
// with root semantic preservation; near matches never acquire a job identity.
func ClassifyChildRunInventoryNameV1(name string) (ChildRunInventoryEntryKindV1, string, bool) {
	return classifyChildRunInventoryNameV1(name)
}

// exactLegacyTypeScriptChildRunNameV1 recognizes only the retired producer's
// default child_${Date.now().toString(36)}_${random6}.json form. It is not a
// live persisted identity and never becomes executable authority. Near-match,
// caller-selected, uppercase, path-like, or extended names remain unknown and
// fail closed.
func exactLegacyTypeScriptChildRunNameV1(name string) (string, bool) {
	const prefix = "child_"
	const separatorIndex = len(prefix) + 8
	const expectedLength = len(prefix) + 8 + 1 + 6 + len(".json")
	if len(name) != expectedLength || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") ||
		name[separatorIndex] != '_' {
		return "", false
	}
	childID := strings.TrimSuffix(name, ".json")
	for index, value := range childID {
		if index < len(prefix) || index == separatorIndex {
			continue
		}
		if (value < '0' || value > '9') && (value < 'a' || value > 'z') {
			return "", false
		}
	}
	return childID, name == childID+".json"
}

func exactChildRunJobNameV1(name string, suffix string) (string, bool) {
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	jobID := strings.TrimSuffix(name, suffix)
	return jobID, validPersistedJobID(jobID) && name == jobID+suffix
}

func validRuntimeTemporaryHexV1(value string) bool {
	if len(value) != childRunTemporaryRandomBytesV1*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == childRunTemporaryRandomBytesV1
}

// The deployed legacy writer used Go's canonical uint32 decimal suffix. This
// grammar remains recognizable only as deletion-only residue; new writes use
// the repository-owned, versioned grammar above.
func validLegacyRuntimeTemporaryRandomV1(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 32)
	return err == nil && value == strconv.FormatUint(parsed, 10)
}

func sameChildRunFileObservationV1(left os.FileInfo, right os.FileInfo) bool {
	if left == nil || right == nil || !os.SameFile(left, right) {
		return false
	}
	return left.Size() == right.Size() && left.Mode() == right.Mode() && left.ModTime().Equal(right.ModTime())
}

func childRunInventoryManifestSHA256V1(inventory ChildRunInventoryV1) (string, error) {
	inventory.ManifestSHA256 = ""
	data, err := json.Marshal(inventory)
	if err != nil {
		return "", errors.New("child-run inventory manifest cannot be encoded")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateChildRunInventoryShapeV1(root string, inventory ChildRunInventoryV1) error {
	if inventory.SchemaVersion != ChildRunInventorySchemaVersionV1 || inventory.Root != root || inventory.Entries == nil ||
		len(inventory.Entries) > childRunInventoryMaxObjectsV1 {
		return errors.New("child-run inventory plan shape is invalid")
	}
	if !inventory.RootExists {
		if inventory.RootIdentity != (ChildRunFilesystemIdentityV1{}) || inventory.RootSizeBytes != 0 ||
			inventory.RootMode != 0 || inventory.RootModifiedUnixNano != 0 || len(inventory.Entries) != 0 {
			return errors.New("absent child-run inventory root has invalid bindings")
		}
	} else if !validChildRunFilesystemIdentityShapeV1(inventory.RootIdentity, false) {
		return errors.New("child-run inventory root identity is invalid")
	}
	priorName := ""
	seenPaths := map[string]bool{}
	seenIdentities := map[string]bool{}
	for _, entry := range inventory.Entries {
		if err := validateChildRunInventoryEntryShapeV1(root, entry); err != nil {
			return err
		}
		identityKey := entry.Identity.Scheme + ":" + entry.Identity.Device + ":" + entry.Identity.Inode
		if (priorName != "" && entry.Name <= priorName) || seenPaths[entry.Path] || seenIdentities[identityKey] {
			return errors.New("child-run inventory entry binding is invalid")
		}
		priorName = entry.Name
		seenPaths[entry.Path] = true
		seenIdentities[identityKey] = true
	}
	manifest, err := childRunInventoryManifestSHA256V1(inventory)
	if err != nil || !isLowerSHA256V1(inventory.ManifestSHA256) || manifest != inventory.ManifestSHA256 {
		return errors.New("child-run inventory manifest integrity is invalid")
	}
	return nil
}

func validateChildRunInventoryEntryShapeV1(root string, entry ChildRunInventoryEntryV1) error {
	kind, jobID, ok := classifyChildRunInventoryNameV1(entry.Name)
	maximumBytes, hasMaximum := childRunInventoryMaxBytesForKindV1(entry.Kind)
	if !ok || entry.Kind != kind || entry.JobID != jobID || entry.Path != filepath.Join(root, entry.Name) ||
		!hasMaximum || entry.SizeBytes < 0 || entry.SizeBytes > maximumBytes || !os.FileMode(entry.Mode).IsRegular() || !isLowerSHA256V1(entry.SHA256) ||
		!validChildRunFilesystemIdentityShapeV1(entry.Identity, true) {
		return errors.New("child-run inventory entry binding is invalid")
	}
	return nil
}

func childRunInventoryMaxBytesForKindV1(kind ChildRunInventoryEntryKindV1) (int64, bool) {
	switch kind {
	case ChildRunInventoryRecordV1, ChildRunInventoryLegacyTypeScriptRecordV1:
		return childRunInventoryRecordMaxBytesV1, true
	case ChildRunInventoryArtifactV1, ChildRunInventoryTemporaryV1, ChildRunInventoryLegacyTemporaryV1:
		return childRunInventoryOpaqueMaxBytesV1, true
	default:
		return 0, false
	}
}

func validChildRunFilesystemIdentityShapeV1(identity ChildRunFilesystemIdentityV1, singleLink bool) bool {
	device, deviceErr := strconv.ParseUint(identity.Device, 10, 64)
	if deviceErr != nil || identity.Device != strconv.FormatUint(device, 10) || identity.LinkCount == 0 {
		return false
	}
	switch identity.Scheme {
	case childRunFilesystemIdentityUnixV1:
		inode, err := strconv.ParseUint(identity.Inode, 10, 64)
		if err != nil || identity.Inode != strconv.FormatUint(inode, 10) {
			return false
		}
	case childRunFilesystemIdentityWindowsV1:
		if len(identity.Inode) != 32 || identity.Inode != strings.ToLower(identity.Inode) {
			return false
		}
		decoded, err := hex.DecodeString(identity.Inode)
		if err != nil || len(decoded) != 16 {
			return false
		}
	default:
		return false
	}
	return !singleLink || identity.LinkCount == 1
}

func sameChildRunFilesystemObjectV1(left ChildRunFilesystemIdentityV1, right ChildRunFilesystemIdentityV1) bool {
	return left.Scheme == right.Scheme && left.Device == right.Device && left.Inode == right.Inode
}

func isLowerSHA256V1(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
