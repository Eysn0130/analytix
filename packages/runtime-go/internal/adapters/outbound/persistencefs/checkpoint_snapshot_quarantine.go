package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	LegacyCheckpointSnapshotDirectoryV1        = "checkpoint-snapshots"
	LegacyCheckpointSnapshotQuarantineV1       = "checkpoint-snapshot-quarantine"
	LegacyCheckpointSnapshotAuditRecordsV1     = "audit-records"
	LegacyCheckpointSnapshotPayloadsV1         = "payloads"
	LegacyCheckpointSnapshotQuarantineSchemaV1 = "analytix.legacy-checkpoint-snapshot-quarantine.v1"

	maxLegacyCheckpointSnapshotFilesV1      = 8_192
	maxLegacyCheckpointSnapshotEntriesV1    = 16_387
	maxLegacyCheckpointSnapshotFileBytesV1  = 2 << 20
	maxLegacyCheckpointSnapshotTotalBytesV1 = 256 << 20
)

// LegacyCheckpointSnapshotEntryV1 is audit metadata only. It never grants
// checkpoint authority and deliberately excludes file contents.
type LegacyCheckpointSnapshotEntryV1 struct {
	Path            string `json:"path"`
	Type            string `json:"type"`
	Mode            uint32 `json:"mode"`
	PlatformFlags   uint32 `json:"platformFlags,omitempty"`
	Size            int64  `json:"size,omitempty"`
	ModTimeUnixNano int64  `json:"modTimeUnixNano"`
	SHA256          string `json:"sha256,omitempty"`
}

// LegacyCheckpointSnapshotTreeV1 binds one stable, no-follow inventory. The
// root identity survives an in-filesystem rename and lets recovery prove that
// the quarantined payload is the exact legacy directory that was moved.
type LegacyCheckpointSnapshotTreeV1 struct {
	RootIdentity string                            `json:"rootIdentity"`
	SHA256       string                            `json:"sha256"`
	FileCount    int                               `json:"fileCount"`
	TotalBytes   int64                             `json:"totalBytes"`
	Entries      []LegacyCheckpointSnapshotEntryV1 `json:"entries"`
}

type LegacyCheckpointSnapshotPayloadV1 struct {
	Name string
	Tree LegacyCheckpointSnapshotTreeV1
}

type LegacyCheckpointSnapshotQuarantineStateV1 struct {
	SourcePresent       bool
	Source              LegacyCheckpointSnapshotTreeV1
	QuarantinePresent   bool
	AuditRecordsPresent bool
	PayloadsPresent     bool
	Payload             *LegacyCheckpointSnapshotPayloadV1
}

// LegacyCheckpointSnapshotQuarantineRootV1 returns the only supported private
// quarantine. The caller must still bind it through a live persistence access
// authority before inspecting or mutating it.
func LegacyCheckpointSnapshotQuarantineRootV1(dataDir string) (string, error) {
	trimmed := strings.TrimSpace(dataDir)
	absolute, err := filepath.Abs(trimmed)
	if err != nil || trimmed == "" || dataDir != trimmed || filepath.Clean(absolute) != absolute {
		return "", errors.New("legacy checkpoint quarantine data root is invalid")
	}
	canonical, err := canonicalPathWithoutCreate(absolute)
	if err != nil {
		return "", errors.New("legacy checkpoint quarantine data root identity is unavailable")
	}
	return filepath.Join(canonical, "private", LegacyCheckpointSnapshotQuarantineV1), nil
}

func LegacyCheckpointSnapshotAuditRootV1(dataDir string) (string, error) {
	root, err := LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, LegacyCheckpointSnapshotAuditRecordsV1), nil
}

// InspectLegacyCheckpointSnapshotQuarantineV1 is read-only. Platform adapters
// use handle-relative no-follow traversal and perform two complete inventories
// of every payload so concurrent edits become a startup error.
func InspectLegacyCheckpointSnapshotQuarantineV1(
	ctx context.Context,
	dataDir string,
	access privatecasport.AccessAuthority,
) (LegacyCheckpointSnapshotQuarantineStateV1, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotQuarantineStateV1{}, err
	}
	quarantineRoot, err := LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if err != nil || access == nil {
		return LegacyCheckpointSnapshotQuarantineStateV1{}, errors.New("legacy checkpoint quarantine access is invalid")
	}
	// A first cold start may hold a signed ancestor-bound lease before the data
	// root itself exists. In that state no legacy sidecar can exist, but normal
	// descendant binding is intentionally unavailable until semantic startup
	// signs the new root inode. Prove the complete missing suffix absent through
	// the live lease instead of weakening private-CAS descendant authority.
	if lease, ok := access.(*CompositeLease); ok {
		absent, err := lease.legacyCheckpointColdDataRootAbsent(ctx, dataDir)
		if err != nil {
			return LegacyCheckpointSnapshotQuarantineStateV1{}, errors.Join(
				errors.New("legacy checkpoint cold data root inspection failed"), err,
			)
		}
		if absent {
			return LegacyCheckpointSnapshotQuarantineStateV1{}, nil
		}
	}
	var state LegacyCheckpointSnapshotQuarantineStateV1
	err = access.WithPrivateCASAccess(ctx, quarantineRoot, func(binding privatecasport.RootBinding) error {
		if err := validateLegacyCheckpointQuarantineBinding(dataDir, quarantineRoot, binding); err != nil {
			return err
		}
		var inspectErr error
		state, inspectErr = platformInspectLegacyCheckpointSnapshotQuarantine(ctx, binding)
		return inspectErr
	})
	if err != nil {
		return LegacyCheckpointSnapshotQuarantineStateV1{}, errors.Join(errors.New("legacy checkpoint quarantine inspection failed"), err)
	}
	return state, nil
}

func (lease *CompositeLease) legacyCheckpointColdDataRootAbsent(ctx context.Context, dataDir string) (bool, error) {
	canonical, err := canonicalPathWithoutCreate(dataDir)
	if err != nil {
		return false, errors.New("legacy checkpoint cold data root is invalid")
	}
	lease.mu.Lock()
	lease.ensureAccessCondLocked()
	if !lease.liveLocked() {
		lease.mu.Unlock()
		return false, errors.New("legacy checkpoint cold data root is outside a live persistence lease")
	}
	lease.activeAccesses++
	authority := lease.authority
	lease.mu.Unlock()
	defer func() {
		lease.mu.Lock()
		lease.activeAccesses--
		lease.accessCond.Broadcast()
		lease.mu.Unlock()
	}()

	capability, err := authority.rootCapability(canonical)
	if err != nil {
		return false, errors.New("legacy checkpoint data root is outside frozen persistence authority")
	}
	if capability.RootIdentity != "" {
		return false, nil
	}
	if err := platformValidateColdRootAbsent(capability); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := platformValidateColdRootAbsent(capability); err != nil {
		return false, err
	}
	current, err := authority.rootCapability(canonical)
	if err != nil || current != capability {
		return false, errors.New("legacy checkpoint cold data root authority changed during inspection")
	}
	return true, nil
}

// MoveLegacyCheckpointSnapshotsToQuarantineV1 performs one no-replace rename
// from dataDir/checkpoint-snapshots into the protected payload directory. It
// never copies, truncates, unlinks, or overwrites legacy bytes.
func MoveLegacyCheckpointSnapshotsToQuarantineV1(
	ctx context.Context,
	dataDir string,
	targetName string,
	expected LegacyCheckpointSnapshotTreeV1,
	access privatecasport.AccessAuthority,
) (LegacyCheckpointSnapshotTreeV1, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if !ValidLegacyCheckpointSnapshotPayloadNameV1(targetName) ||
		LegacyCheckpointSnapshotPayloadDigestV1(targetName) != expected.SHA256 ||
		!canonicalLegacyCheckpointTree(expected) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine move input is invalid")
	}
	quarantineRoot, err := LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if err != nil || access == nil {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine access is invalid")
	}
	var moved LegacyCheckpointSnapshotTreeV1
	err = access.WithPrivateCASAccess(ctx, quarantineRoot, func(binding privatecasport.RootBinding) error {
		if err := validateLegacyCheckpointQuarantineBinding(dataDir, quarantineRoot, binding); err != nil {
			return err
		}
		var moveErr error
		moved, moveErr = platformMoveLegacyCheckpointSnapshotsToQuarantine(ctx, binding, targetName, expected)
		return moveErr
	})
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint quarantine move failed"), err)
	}
	if !equalLegacyCheckpointTrees(expected, moved) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine payload changed during move")
	}
	return moved, nil
}

func ValidLegacyCheckpointSnapshotPayloadNameV1(value string) bool {
	const prefix = "legacy-v1-"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(value, prefix), "-")
	return len(parts) == 2 && len(parts[0]) == 64 && len(parts[1]) == 32 &&
		validLowerHex(parts[0]) && validLowerHex(parts[1])
}

func LegacyCheckpointSnapshotPayloadDigestV1(value string) string {
	if !ValidLegacyCheckpointSnapshotPayloadNameV1(value) {
		return ""
	}
	return strings.Split(strings.TrimPrefix(value, "legacy-v1-"), "-")[0]
}

func validateLegacyCheckpointQuarantineBinding(dataDir, quarantineRoot string, binding privatecasport.RootBinding) error {
	trimmedDataRoot := strings.TrimSpace(dataDir)
	dataRoot, err := filepath.Abs(trimmedDataRoot)
	if err != nil || trimmedDataRoot == "" || dataDir != trimmedDataRoot || filepath.Clean(dataRoot) != dataRoot {
		return errors.New("legacy checkpoint data root is invalid")
	}
	dataRoot, err = canonicalPathWithoutCreate(dataRoot)
	if err != nil {
		return errors.New("legacy checkpoint data root identity is unavailable")
	}
	quarantineAbsolute, err := filepath.Abs(strings.TrimSpace(quarantineRoot))
	if err != nil || quarantineRoot != strings.TrimSpace(quarantineRoot) || filepath.Clean(quarantineAbsolute) != quarantineAbsolute ||
		quarantineAbsolute != filepath.Join(dataRoot, "private", LegacyCheckpointSnapshotQuarantineV1) {
		return errors.New("legacy checkpoint quarantine root is invalid")
	}
	boundRoot, err := filepath.Abs(strings.TrimSpace(binding.RootPath))
	if err != nil || filepath.Clean(boundRoot) != boundRoot {
		return errors.New("legacy checkpoint persistence binding is invalid")
	}
	relativeRoot, err := filepath.Rel(boundRoot, dataRoot)
	if err != nil || relativeRoot != "." {
		return errors.New("legacy checkpoint persistence binding is not the data root")
	}
	expectedQuarantineRoot := filepath.Join(dataRoot, "private", LegacyCheckpointSnapshotQuarantineV1)
	expectedRelative, err := filepath.Rel(dataRoot, expectedQuarantineRoot)
	if err != nil || expectedRelative == "." || filepath.IsAbs(expectedRelative) ||
		expectedRelative == ".." || strings.HasPrefix(expectedRelative, ".."+string(filepath.Separator)) ||
		filepath.Clean(binding.RelativePath) != filepath.Clean(expectedRelative) {
		return errors.New("legacy checkpoint quarantine binding path is invalid")
	}
	identity := binding.RootIdentity
	switch identity.Kind {
	case privatecasport.DirectoryIdentityUnix:
		if identity.Device == 0 || identity.Inode == 0 || identity.VolumeSerial != 0 || identity.FileID != [16]byte{} {
			return errors.New("legacy checkpoint Unix root binding is non-canonical")
		}
	case privatecasport.DirectoryIdentityWindows:
		if identity.Device != 0 || identity.Inode != 0 || identity.VolumeSerial == 0 || identity.FileID == [16]byte{} {
			return errors.New("legacy checkpoint Windows root binding is non-canonical")
		}
	default:
		return errors.New("legacy checkpoint root binding kind is unsupported")
	}
	return nil
}

func stableLegacyCheckpointTree(ctx context.Context, inspect func() (LegacyCheckpointSnapshotTreeV1, bool, error)) (LegacyCheckpointSnapshotTreeV1, bool, error) {
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, err
	}
	first, present, err := inspect()
	if err != nil || !present {
		return first, present, err
	}
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, err
	}
	second, secondPresent, err := inspect()
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, err
	}
	if !secondPresent || !equalLegacyCheckpointTrees(first, second) {
		return LegacyCheckpointSnapshotTreeV1{}, false, errors.New("legacy checkpoint snapshot tree changed during inspection")
	}
	return second, true, nil
}

func finalizeLegacyCheckpointTree(tree LegacyCheckpointSnapshotTreeV1) (LegacyCheckpointSnapshotTreeV1, error) {
	if tree.RootIdentity == "" || len(tree.Entries) == 0 || len(tree.Entries) > maxLegacyCheckpointSnapshotEntriesV1 ||
		tree.FileCount < 0 || tree.FileCount > maxLegacyCheckpointSnapshotFilesV1 ||
		tree.TotalBytes < 0 || tree.TotalBytes > maxLegacyCheckpointSnapshotTotalBytesV1 {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint snapshot inventory exceeds its safety bounds")
	}
	sort.Slice(tree.Entries, func(left, right int) bool { return tree.Entries[left].Path < tree.Entries[right].Path })
	hasher := sha256.New()
	writeLegacyCheckpointDigestField(hasher, []byte(LegacyCheckpointSnapshotQuarantineSchemaV1))
	writeLegacyCheckpointDigestField(hasher, []byte(tree.RootIdentity))
	for _, entry := range tree.Entries {
		if !validLegacyCheckpointSnapshotEntry(entry) {
			return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint snapshot contains an unknown entry")
		}
		writeLegacyCheckpointDigestField(hasher, []byte(entry.Path))
		writeLegacyCheckpointDigestField(hasher, []byte(entry.Type))
		writeLegacyCheckpointDigestField(hasher, []byte(strconv.FormatUint(uint64(entry.Mode), 10)))
		writeLegacyCheckpointDigestField(hasher, []byte(strconv.FormatUint(uint64(entry.PlatformFlags), 10)))
		writeLegacyCheckpointDigestField(hasher, []byte(strconv.FormatInt(entry.Size, 10)))
		writeLegacyCheckpointDigestField(hasher, []byte(strconv.FormatInt(entry.ModTimeUnixNano, 10)))
		writeLegacyCheckpointDigestField(hasher, []byte(entry.SHA256))
	}
	tree.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	if !validLegacyCheckpointTree(tree) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint snapshot inventory is invalid")
	}
	return tree, nil
}

func writeLegacyCheckpointDigestField(writer io.Writer, value []byte) {
	_, _ = io.WriteString(writer, strconv.Itoa(len(value)))
	_, _ = io.WriteString(writer, ":")
	_, _ = writer.Write(value)
	_, _ = io.WriteString(writer, "\n")
}

func validLegacyCheckpointTree(tree LegacyCheckpointSnapshotTreeV1) bool {
	return tree.RootIdentity != "" && len(tree.SHA256) == 64 && validLowerHex(tree.SHA256) &&
		len(tree.Entries) > 0 && len(tree.Entries) <= maxLegacyCheckpointSnapshotEntriesV1 &&
		tree.FileCount >= 0 && tree.FileCount <= maxLegacyCheckpointSnapshotFilesV1 &&
		tree.TotalBytes >= 0 && tree.TotalBytes <= maxLegacyCheckpointSnapshotTotalBytesV1
}

func equalLegacyCheckpointTrees(left, right LegacyCheckpointSnapshotTreeV1) bool {
	if left.RootIdentity != right.RootIdentity || left.SHA256 != right.SHA256 || left.FileCount != right.FileCount ||
		left.TotalBytes != right.TotalBytes || len(left.Entries) != len(right.Entries) {
		return false
	}
	for index := range left.Entries {
		if left.Entries[index] != right.Entries[index] {
			return false
		}
	}
	return true
}

func canonicalLegacyCheckpointTree(tree LegacyCheckpointSnapshotTreeV1) bool {
	if !validLegacyCheckpointTree(tree) {
		return false
	}
	candidate := tree
	candidate.SHA256 = ""
	candidate.Entries = append([]LegacyCheckpointSnapshotEntryV1(nil), tree.Entries...)
	finalized, err := finalizeLegacyCheckpointTree(candidate)
	if err != nil || !equalLegacyCheckpointTrees(finalized, tree) {
		return false
	}
	seen := make(map[string]string, len(tree.Entries))
	files := 0
	var totalBytes int64
	for _, entry := range tree.Entries {
		if _, duplicate := seen[entry.Path]; duplicate {
			return false
		}
		seen[entry.Path] = entry.Type
		if entry.Path != "." {
			parent := filepath.ToSlash(filepath.Dir(entry.Path))
			if parent == "" {
				parent = "."
			}
			if seen[parent] != "directory" {
				return false
			}
		}
		if entry.Type == "file" {
			if entry.Size > maxLegacyCheckpointSnapshotTotalBytesV1-totalBytes {
				return false
			}
			files++
			totalBytes += entry.Size
		}
	}
	return files == tree.FileCount && totalBytes == tree.TotalBytes
}

func validLegacyCheckpointSnapshotEntry(entry LegacyCheckpointSnapshotEntryV1) bool {
	parts := strings.Split(filepath.ToSlash(entry.Path), "/")
	if entry.Path == "." {
		return entry.Type == "directory" && entry.Size == 0 && entry.SHA256 == ""
	}
	if filepath.Clean(entry.Path) != entry.Path || filepath.IsAbs(entry.Path) || strings.Contains(entry.Path, "\\") {
		return false
	}
	switch len(parts) {
	case 1:
		return entry.Type == "directory" && safeLegacyCheckpointID(parts[0]) && entry.Size == 0 && entry.SHA256 == ""
	case 2:
		return entry.Type == "directory" && safeLegacyCheckpointID(parts[0]) && safeLegacyCheckpointID(parts[1]) &&
			entry.Size == 0 && entry.SHA256 == ""
	case 3:
		return entry.Type == "file" && safeLegacyCheckpointID(parts[0]) && safeLegacyCheckpointID(parts[1]) &&
			validLegacyCheckpointRecordName(parts[2]) && entry.Size >= 0 && entry.Size <= maxLegacyCheckpointSnapshotFileBytesV1 &&
			len(entry.SHA256) == 64 && validLowerHex(entry.SHA256)
	default:
		return false
	}
}

func safeLegacyCheckpointID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." && value != ".." &&
		!strings.Contains(value, "..") && !strings.ContainsAny(value, "/\\:")
}

func validLegacyCheckpointRecordName(value string) bool {
	return len(value) == 69 && strings.HasSuffix(value, ".json") && validLowerHex(strings.TrimSuffix(value, ".json"))
}

func validLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}
