package persistencefs

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	maxStrictJSONBytes = domainstartup.MaxSemanticManagedFileBytesV1
	maxStrictJSONLLine = 4 << 20
)

var errEventPhysicalSequenceMismatchV1 = errors.New("event physical sequence mismatch")

type StartupResourceLimitError struct {
	Code  string
	Limit int64
}

func (err StartupResourceLimitError) Error() string {
	return fmt.Sprintf("semantic startup resource limit exceeded [%s]: limit=%d", err.Code, err.Limit)
}

type IntegrityError struct {
	Code string
	Path string
	Line int
	Err  error
}

func (err IntegrityError) Error() string {
	if err.Line > 0 {
		return fmt.Sprintf("persistence integrity violation [%s] at managed record line %s", err.Code, strconv.Itoa(err.Line))
	}
	return fmt.Sprintf("persistence integrity violation [%s]", err.Code)
}

func (err IntegrityError) Unwrap() error {
	return err.Err
}

type FileRecord struct {
	Path        string
	SHA256      string
	Size        int64
	RecordCount int
}

type EntryRecord struct {
	Path            string
	Type            string
	Mode            uint32
	Size            int64
	ModTimeUnixNano int64
	SHA256          string
	RecordCount     int
}

type RawSnapshot struct {
	Roots     RootSet
	FileCount int
	Entries   []EntryRecord
	SHA256    string
}

type managedTarget struct {
	path       string
	label      string
	threadTree bool
}

type snapshotScanner struct {
	ctx                       context.Context
	entries                   []EntryRecord
	identities                map[string]string
	allowedPrivateResidues    map[string]struct{}
	originalPlainFiles        map[string]privatecasport.OriginalPlainResidueV1
	originalCreateDirectories map[string]uint32
	semanticValidationMemo    *startupSemanticValidationMemoV1
	rootBindingDigest         string
	totalBytes                int64
	fileCount                 int
}

func CaptureStrict(roots RootSet) (RawSnapshot, error) {
	return CaptureStrictContext(context.Background(), roots)
}

func CaptureStrictContext(ctx context.Context, roots RootSet) (RawSnapshot, error) {
	return captureStrictWithContextAndHook(ctx, roots, nil)
}

func captureStrictWithHook(roots RootSet, beforeSecondCapture func()) (RawSnapshot, error) {
	return captureStrictWithContextAndHook(context.Background(), roots, beforeSecondCapture)
}

func captureStrictWithContextAndHook(ctx context.Context, roots RootSet, beforeSecondCapture func()) (RawSnapshot, error) {
	return captureStrictWithAllowedPrivateResidues(ctx, roots, beforeSecondCapture, nil)
}

func captureStrictWithAllowedPrivateResidues(
	ctx context.Context,
	roots RootSet,
	beforeSecondCapture func(),
	allowed map[string]struct{},
	originals ...privatecasport.OriginalCreateResiduesV1,
) (RawSnapshot, error) {
	return captureStrictWithScope(ctx, roots, beforeSecondCapture, allowed, true, originals...)
}

// ValidateManagedPreRecoveryBoundaryV1 validates every managed surface that is
// not owned by the signed private-CAS recovery transaction. Private authority
// topology and residues are deliberately excluded here and must be validated
// by that transaction before it applies any mutation.
func ValidateManagedPreRecoveryBoundaryV1(ctx context.Context, roots RootSet) error {
	_, err := CaptureManagedPreRecoverySnapshotV1(ctx, roots)
	return err
}

// CaptureManagedPreRecoverySnapshotV1 retains the complete non-private
// denominator used by the pre-recovery boundary. Private owners must supply
// their separately authenticated prepared inventories before any writer runs.
func CaptureManagedPreRecoverySnapshotV1(ctx context.Context, roots RootSet) (RawSnapshot, error) {
	return captureStrictWithScope(ctx, roots, nil, nil, false)
}

func captureStrictWithScope(
	ctx context.Context,
	roots RootSet,
	beforeSecondCapture func(),
	allowed map[string]struct{},
	includePrivateAuthority bool,
	originals ...privatecasport.OriginalCreateResiduesV1,
) (result RawSnapshot, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var plainProof privatecasport.OriginalPlainResiduesV1
	var plainFiles map[string]privatecasport.OriginalPlainResidueV1
	if includePrivateAuthority {
		var err error
		plainProof, plainFiles, err = originalPlainSnapshotProofV1(ctx, roots)
		if err != nil {
			return RawSnapshot{}, err
		}
		if plainProof != nil {
			defer func() {
				resultErr = errors.Join(resultErr, plainProof.Revalidate(ctx))
				if resultErr != nil {
					result = RawSnapshot{}
				}
			}()
		}
	}
	proof, originalDirectories, err := originalCreateSnapshotProofV1(ctx, roots, originals)
	if err != nil {
		return RawSnapshot{}, err
	}
	if proof != nil {
		defer func() {
			resultErr = errors.Join(resultErr, proof.Revalidate(ctx))
			if resultErr != nil {
				result = RawSnapshot{}
			}
		}()
	}
	first, err := captureStrictOnce(ctx, roots, allowed, includePrivateAuthority, originalDirectories, plainFiles)
	if err != nil {
		return RawSnapshot{}, err
	}
	if err := validateOriginalPlainSnapshotV1(first, plainFiles); err != nil {
		return RawSnapshot{}, err
	}
	if beforeSecondCapture != nil {
		beforeSecondCapture()
	}
	second, err := captureStrictOnce(ctx, roots, allowed, includePrivateAuthority, originalDirectories, plainFiles)
	if err != nil {
		return RawSnapshot{}, err
	}
	if err := validateOriginalPlainSnapshotV1(second, plainFiles); err != nil {
		return RawSnapshot{}, err
	}
	if first.SHA256 != second.SHA256 || first.FileCount != second.FileCount || len(first.Entries) != len(second.Entries) {
		return RawSnapshot{}, integrity("snapshot_changed", "managed-roots", 0, nil)
	}
	return second, nil
}

func captureStrictOnce(
	ctx context.Context,
	roots RootSet,
	allowed map[string]struct{},
	includePrivateAuthority bool,
	originalDirectories map[string]uint32,
	plainFiles map[string]privatecasport.OriginalPlainResidueV1,
) (RawSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return RawSnapshot{}, err
	}
	stableAllowed := make(map[string]struct{}, len(allowed))
	for label := range allowed {
		stableAllowed[label] = struct{}{}
	}
	scanner := snapshotScanner{
		ctx: ctx, identities: map[string]string{}, allowedPrivateResidues: stableAllowed, originalCreateDirectories: originalDirectories, originalPlainFiles: plainFiles,
		semanticValidationMemo: startupSemanticValidationMemoFromContextV1(ctx),
		rootBindingDigest:      rootBindingDigest(roots),
	}
	for _, candidate := range []struct{ label, root string }{{"data-root", roots.DataDir}, {"durable-root", roots.DurableDir}} {
		info, err := os.Lstat(candidate.root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return RawSnapshot{}, integrity("persistence_root_retargeted", candidate.label, 0, err)
		}
	}
	if includePrivateAuthority {
		if err := rejectDataRootPrivateCASCreateResidues(ctx, roots.DataDir); err != nil {
			return RawSnapshot{}, err
		}
	}
	targets := []managedTarget{
		{path: filepath.Join(roots.DurableDir, "durable-meta.json"), label: "durable/durable-meta.json"},
		{path: filepath.Join(roots.DurableDir, "thread_summaries.jsonl"), label: "durable/thread_summaries.jsonl"},
		{path: filepath.Join(roots.DurableDir, "threads"), label: "durable/threads", threadTree: true},
		{path: filepath.Join(roots.DurableDir, "runtime-go", "threads"), label: "durable/runtime-go/threads", threadTree: true},
		{path: filepath.Join(roots.DurableDir, "usage_events"), label: "durable/usage_events"},
	}
	if includePrivateAuthority {
		targets = append(targets, managedTarget{path: filepath.Join(roots.DataDir, "private"), label: "data/private"})
	}
	targets = append(targets,
		managedTarget{path: filepath.Join(roots.DataDir, "memory"), label: "data/memory"},
		managedTarget{path: filepath.Join(roots.DataDir, "child-runs"), label: "data/child-runs"},
		managedTarget{path: filepath.Join(roots.DataDir, "attachments"), label: "data/attachments"},
		managedTarget{path: filepath.Join(roots.DataDir, "mcp-schema-cache"), label: "data/mcp-schema-cache"},
	)
	for _, target := range targets {
		if err := scanner.scanTarget(target); err != nil {
			return RawSnapshot{}, err
		}
	}
	sort.Slice(scanner.entries, func(left int, right int) bool {
		if scanner.entries[left].Path != scanner.entries[right].Path {
			return scanner.entries[left].Path < scanner.entries[right].Path
		}
		return scanner.entries[left].Type < scanner.entries[right].Type
	})
	digest := sha256.New()
	for _, entry := range scanner.entries {
		_, _ = io.WriteString(digest, entry.Path)
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, entry.Type)
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, strconv.FormatUint(uint64(entry.Mode), 10))
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, strconv.FormatInt(entry.Size, 10))
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, strconv.FormatInt(entry.ModTimeUnixNano, 10))
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, entry.SHA256)
		_, _ = digest.Write([]byte{0})
		_, _ = io.WriteString(digest, strconv.Itoa(entry.RecordCount))
		_, _ = digest.Write([]byte{'\n'})
	}
	return RawSnapshot{
		Roots: roots, FileCount: scanner.fileCount,
		Entries: append([]EntryRecord(nil), scanner.entries...),
		SHA256:  hex.EncodeToString(digest.Sum(nil)),
	}, nil
}

func rejectDataRootPrivateCASCreateResidues(ctx context.Context, dataRoot string) (resultErr error) {
	info, err := os.Lstat(dataRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return integrity("persistence_root_retargeted", "data-root", 0, err)
	}
	directory, err := os.Open(dataRoot)
	if err != nil {
		return integrity("open_failed", "data-root", 0, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	openedInfo, err := directory.Stat()
	if err != nil || !openedInfo.IsDir() || !os.SameFile(info, openedInfo) {
		return integrity("file_changed_during_snapshot", "data-root", 0, err)
	}
	entryCount := 0
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		entries, readErr := directory.ReadDir(256)
		entryCount += len(entries)
		if entryCount > domainstartup.MaxManagedSnapshotEntriesV1 {
			return StartupResourceLimitError{Code: "data_root_entries", Limit: domainstartup.MaxManagedSnapshotEntriesV1}
		}
		for _, entry := range entries {
			if domainprivatecas.LooksLikeCreateDirectoryResidueNameV1(entry.Name()) {
				return integrity(
					"unknown_final_authority_crash_temp",
					"data/"+entry.Name(),
					0,
					nil,
				)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return integrity("walk_failed", "data-root", 0, readErr)
		}
	}
	refreshedInfo, statErr := directory.Stat()
	currentInfo, lstatErr := os.Lstat(dataRoot)
	if statErr != nil || lstatErr != nil || currentInfo.Mode()&os.ModeSymlink != 0 ||
		!currentInfo.IsDir() || !os.SameFile(openedInfo, refreshedInfo) ||
		!os.SameFile(refreshedInfo, currentInfo) || currentInfo.Mode() != info.Mode() ||
		!currentInfo.ModTime().Equal(info.ModTime()) {
		return integrity(
			"file_changed_during_snapshot",
			"data-root",
			0,
			errors.Join(statErr, lstatErr),
		)
	}
	return nil
}

func (scanner *snapshotScanner) scanTarget(target managedTarget) error {
	if err := scanner.contextError(); err != nil {
		return err
	}
	info, err := os.Lstat(target.path)
	if errors.Is(err, os.ErrNotExist) {
		return scanner.appendEntry(EntryRecord{Path: target.label, Type: "absent"})
	}
	if err != nil {
		return integrity("stat_failed", target.label, 0, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return integrity("managed_symlink", target.label, 0, nil)
	}
	if !info.IsDir() {
		return scanner.scanFile(target, target.path, target.label, ".", info)
	}
	return scanner.scanDirectory(target, target.path, target.label, ".", info, 1)
}

func (scanner *snapshotScanner) scanDirectory(
	target managedTarget,
	path string,
	label string,
	relative string,
	initialInfo os.FileInfo,
	depth int,
) (resultErr error) {
	if depth > domainstartup.MaxSemanticManagedPathDepthV1 {
		return StartupResourceLimitError{Code: "managed_depth", Limit: domainstartup.MaxSemanticManagedPathDepthV1}
	}
	if target.threadTree {
		parts := splitRelativePath(relative)
		if len(parts) == 1 && parts[0] != "." && !exactSafeRecordID(parts[0]) {
			return integrity("unsafe_thread_directory", label, 0, nil)
		}
	}
	if mode, original := scanner.originalCreateDirectories[label]; original && uint32(initialInfo.Mode()) != mode {
		return integrity("original_creation_directory_changed", label, 0, nil)
	}
	if err := scanner.appendEntry(entryRecordForInfo(label, "directory", initialInfo, FileRecord{})); err != nil {
		return err
	}
	directory, err := os.Open(path)
	if err != nil {
		return integrity("open_failed", label, 0, err)
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	openedInfo, err := directory.Stat()
	if err != nil || !openedInfo.IsDir() || !os.SameFile(initialInfo, openedInfo) {
		return integrity("file_changed_during_snapshot", label, 0, err)
	}
	for {
		if err := scanner.contextError(); err != nil {
			return err
		}
		entries, readErr := directory.ReadDir(256)
		if _, original := scanner.originalCreateDirectories[label]; original && len(entries) != 0 {
			return integrity("original_creation_directory_not_empty", label, 0, nil)
		}
		for _, entry := range entries {
			childPath := filepath.Join(path, entry.Name())
			label := targetLabel(target, childPath)
			residue := classifyPrivateAuthorityResidueLabel(label)
			if residue.state == privateAuthorityResidueMalformed {
				return integrity("unknown_final_authority_crash_temp", label, 0, nil)
			}
			if residue.state == privateAuthorityResidueKnown {
				if _, original := scanner.originalCreateDirectories[label]; original && residue.kind == domainprivatecas.ResidueCreateDirectoryV1 && entry.IsDir() {
					continue
				}
				_, transactionAllowed := scanner.allowedPrivateResidues[label]
				physicallyDeferred := residue.deferredOwner &&
					residue.kind == domainprivatecas.ResidueOrdinaryWriteV1
				_, originalPlain := scanner.originalPlainFiles[label]
				if (!transactionAllowed && !physicallyDeferred && !originalPlain) ||
					residue.kind != domainprivatecas.ResidueOrdinaryWriteV1 {
					return integrity("unknown_final_authority_crash_temp", label, 0, nil)
				}
			}
		}
		for _, entry := range entries {
			childPath := filepath.Join(path, entry.Name())
			childLabel := targetLabel(target, childPath)
			childRelative, relErr := filepath.Rel(target.path, childPath)
			if relErr != nil {
				return integrity("walk_failed", childLabel, 0, relErr)
			}
			entryInfo, statErr := os.Lstat(childPath)
			if statErr != nil {
				return integrity("stat_failed", childLabel, 0, statErr)
			}
			if entryInfo.Mode()&os.ModeSymlink != 0 {
				return integrity("managed_symlink", childLabel, 0, nil)
			}
			if entryInfo.IsDir() {
				if err := scanner.scanDirectory(target, childPath, childLabel, childRelative, entryInfo, depth+1); err != nil {
					return err
				}
				continue
			}
			if !entryInfo.Mode().IsRegular() {
				return integrity("managed_special_file", childLabel, 0, nil)
			}
			if err := scanner.scanFile(target, childPath, childLabel, childRelative, entryInfo); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return integrity("walk_failed", label, 0, readErr)
		}
	}
	refreshedInfo, statErr := directory.Stat()
	currentInfo, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil || currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.IsDir() ||
		!os.SameFile(openedInfo, refreshedInfo) || !os.SameFile(refreshedInfo, currentInfo) ||
		currentInfo.Mode() != initialInfo.Mode() || !currentInfo.ModTime().Equal(initialInfo.ModTime()) {
		return integrity("file_changed_during_snapshot", label, 0, errors.Join(statErr, lstatErr))
	}
	return nil
}

func (scanner *snapshotScanner) contextError() error {
	if scanner == nil || scanner.ctx == nil {
		return nil
	}
	return scanner.ctx.Err()
}

func (scanner *snapshotScanner) scanFile(target managedTarget, path string, label string, relative string, initialInfo os.FileInfo) (resultErr error) {
	if err := scanner.contextError(); err != nil {
		return err
	}
	if initialInfo == nil || initialInfo.Size() < 0 || initialInfo.Size() > domainstartup.MaxSemanticManagedFileBytesV1 {
		return StartupResourceLimitError{Code: "managed_file_bytes", Limit: domainstartup.MaxSemanticManagedFileBytesV1}
	}
	file, err := os.Open(path)
	if err != nil {
		return integrity("open_failed", label, 0, err)
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	openedInfo, err := file.Stat()
	if err != nil {
		return integrity("stat_failed", label, 0, err)
	}
	if !os.SameFile(initialInfo, openedInfo) || openedInfo.Size() != initialInfo.Size() {
		return integrity("file_changed_during_snapshot", label, 0, err)
	}
	initialIdentity, linkCount, err := regularFileIdentity(file, openedInfo)
	if err != nil {
		return integrity("identity_unavailable", label, 0, err)
	}
	if linkCount > 1 {
		return integrity("managed_hardlink", label, 0, nil)
	}
	if previous, exists := scanner.identities[initialIdentity]; exists && previous != label {
		return integrity("file_identity_alias", label, 0, nil)
	}
	scanner.identities[initialIdentity] = label

	extension := strings.ToLower(filepath.Ext(path))
	isThreadEventJSONL := target.threadTree && extension == ".jsonl" && filepath.Base(relative) == "events.jsonl"
	eventSequenceMode := startupEventSequenceModeNoneV1
	if isThreadEventJSONL {
		eventSequenceMode = startupEventSequenceModeStrictV1
	}
	profile := startupSemanticValidationProfileV1{
		ValidatorSchemaVersion: startupSemanticValidatorSchemaVersionV1,
		RootBindingDigest:      scanner.rootBindingDigest,
		Extension:              extension, TargetLabel: target.label, RelativePath: filepath.ToSlash(relative),
		ThreadTree: target.threadTree, StrictJSONLFraming: requiresStrictJSONLFraming(target, relative),
		EventSequenceMode: eventSequenceMode,
	}
	semanticMemoEligible := scanner.semanticValidationMemo != nil &&
		(extension == ".json" || extension == ".jsonl") &&
		!privateOpaqueCASRecordLabel(label)
	rememberSemanticValidation := semanticMemoEligible
	var record FileRecord
	semanticValidationReused := false
	if semanticMemoEligible && scanner.semanticValidationMemo.hasCandidate(label, profile) {
		hashed, hashErr := hashOpaqueFile(
			&snapshotContextReader{ctx: scanner.ctx, reader: file}, label, openedInfo.Size(),
		)
		if hashErr != nil {
			return hashErr
		}
		if cached, ok := scanner.semanticValidationMemo.reuse(label, profile, hashed); ok {
			record = cached
			semanticValidationReused = true
		} else if _, err := file.Seek(0, io.SeekStart); err != nil {
			return integrity("read_failed", label, 0, err)
		}
	}
	if !semanticValidationReused {
		if semanticMemoEligible {
			scanner.semanticValidationMemo.noteSemanticDecode()
		}
		reader := &snapshotContextReader{ctx: scanner.ctx, reader: file}
		switch extension {
		case ".json":
			if privateOpaqueCASRecordLabel(label) {
				if openedInfo.Size() == 0 {
					err = integrity("invalid_opaque_size", label, 0, nil)
				} else {
					record, err = hashOpaqueFile(reader, label, openedInfo.Size())
					if err == nil {
						record.RecordCount = 1
					}
				}
			} else {
				record, err = scanStrictJSONFile(reader, label, openedInfo.Size())
			}
		case ".jsonl":
			record, err = scanStrictJSONLFile(reader, target, label, relative, openedInfo.Size(), false)
			if isThreadEventJSONL && errors.Is(err, errEventPhysicalSequenceMismatchV1) {
				if contextErr := scanner.contextError(); contextErr != nil {
					return contextErr
				}
				allowHistoricalEventOrder, classifyErr := classifyMigratableEventOrderV1(target, path, relative)
				if classifyErr != nil {
					return integrity("thread_record_identity", label, 0, classifyErr)
				}
				if allowHistoricalEventOrder {
					if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
						return integrity("read_failed", label, 0, seekErr)
					}
					if contextErr := scanner.contextError(); contextErr != nil {
						return contextErr
					}
					if semanticMemoEligible {
						scanner.semanticValidationMemo.noteSemanticDecode()
					}
					record, err = scanStrictJSONLFile(
						&snapshotContextReader{ctx: scanner.ctx, reader: file},
						target, label, relative, openedInfo.Size(), true,
					)
					rememberSemanticValidation = false
				}
			}
		default:
			record, err = hashOpaqueFile(reader, label, openedInfo.Size())
		}
	}
	if err != nil {
		return err
	}
	if !semanticValidationReused && target.threadTree && extension == ".json" {
		if err := validateThreadJSONFile(file, target, label, relative); err != nil {
			return err
		}
	}
	currentInfo, err := os.Lstat(path)
	if err != nil || currentInfo.Mode()&os.ModeSymlink != 0 {
		return integrity("file_changed_during_snapshot", label, 0, err)
	}
	refreshedInfo, refreshedErr := file.Stat()
	currentIdentity := ""
	currentLinks := uint64(0)
	var identityErr error
	if refreshedErr == nil {
		currentIdentity, currentLinks, identityErr = regularFileIdentity(file, refreshedInfo)
	}
	if refreshedErr != nil || identityErr != nil || refreshedInfo.Size() != initialInfo.Size() || !os.SameFile(openedInfo, currentInfo) || currentIdentity != initialIdentity || currentLinks != linkCount || currentInfo.Size() != initialInfo.Size() || currentInfo.Mode() != initialInfo.Mode() || !currentInfo.ModTime().Equal(initialInfo.ModTime()) {
		return integrity("file_changed_during_snapshot", label, 0, identityErr)
	}
	if rememberSemanticValidation && !semanticValidationReused {
		scanner.semanticValidationMemo.remember(label, profile, record)
	}
	if scanner.totalBytes > domainstartup.MaxSemanticStagedTotalBytesV1-record.Size {
		return StartupResourceLimitError{Code: "managed_total_bytes", Limit: domainstartup.MaxSemanticStagedTotalBytesV1}
	}
	if scanner.fileCount >= domainstartup.MaxManagedSnapshotEntriesV1 {
		return StartupResourceLimitError{Code: "managed_entries", Limit: domainstartup.MaxManagedSnapshotEntriesV1}
	}
	scanner.totalBytes += record.Size
	scanner.fileCount++
	return scanner.appendEntry(entryRecordForInfo(label, "file", currentInfo, record))
}

type snapshotContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *snapshotContextReader) Read(buffer []byte) (int, error) {
	if reader.ctx != nil {
		if err := reader.ctx.Err(); err != nil {
			return 0, err
		}
	}
	return reader.reader.Read(buffer)
}

func (scanner *snapshotScanner) appendEntry(entry EntryRecord) error {
	if len(scanner.entries) >= domainstartup.MaxManagedSnapshotEntriesV1 {
		return StartupResourceLimitError{Code: "managed_entries", Limit: domainstartup.MaxManagedSnapshotEntriesV1}
	}
	if len(entry.Path) > domainstartup.MaxSemanticManagedPathBytesV1 ||
		strings.Count(entry.Path, "/")+1 > domainstartup.MaxSemanticManagedPathDepthV1 {
		return StartupResourceLimitError{Code: "managed_path", Limit: domainstartup.MaxSemanticManagedPathBytesV1}
	}
	scanner.entries = append(scanner.entries, entry)
	return nil
}

func entryRecordForInfo(path string, entryType string, info os.FileInfo, file FileRecord) EntryRecord {
	entry := EntryRecord{Path: path, Type: entryType, SHA256: file.SHA256, RecordCount: file.RecordCount}
	if info != nil {
		entry.Mode = uint32(info.Mode())
		entry.Size = info.Size()
		entry.ModTimeUnixNano = info.ModTime().UnixNano()
	}
	return entry
}

type privateAuthorityResidueState uint8

const (
	privateAuthorityResidueUnrelated privateAuthorityResidueState = iota
	privateAuthorityResidueKnown
	privateAuthorityResidueMalformed
)

type privateAuthorityResidueClassification struct {
	state            privateAuthorityResidueState
	kind             domainprivatecas.ResidueKindV1
	journalRemovable bool
	deferredOwner    bool
}

func classifyPrivateAuthorityResidueLabel(label string) privateAuthorityResidueClassification {
	const prefix = "data/private/"
	if !strings.HasPrefix(label, prefix) {
		return privateAuthorityResidueClassification{}
	}
	relative := strings.TrimPrefix(label, prefix)
	parts := strings.Split(relative, "/")
	if len(parts) == 0 {
		return privateAuthorityResidueClassification{}
	}
	name := parts[len(parts)-1]
	if domainprivatecas.LooksLikeCreateDirectoryResidueNameV1(name) {
		if !domainprivatecas.IsCreateDirectoryResidueNameV1(name) {
			return privateAuthorityResidueClassification{state: privateAuthorityResidueMalformed}
		}
		if privateCASCreateResidueMatchesKnownPath(relative, name) {
			return privateAuthorityResidueClassification{
				state: privateAuthorityResidueKnown,
				kind:  domainprivatecas.ResidueCreateDirectoryV1,
			}
		}
		return privateAuthorityResidueClassification{state: privateAuthorityResidueMalformed}
	}
	if len(parts) == 2 && parts[0] == "authority" {
		if strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".tmp") {
			return privateAuthorityResidueClassification{state: privateAuthorityResidueKnown}
		}
		return privateAuthorityResidueClassification{}
	}
	spec, remainder, found := domainprivatecas.RootContainingRelativePathV1(relative)
	if found {
		if len(remainder) == 2 {
			classified, ok := domainprivatecas.ClassifyRecordResidueNameV1(remainder[1], remainder[0])
			if ok {
				return privateAuthorityResidueClassification{
					state: privateAuthorityResidueKnown, kind: classified.Kind,
					journalRemovable: classified.Kind == domainprivatecas.ResidueOrdinaryWriteV1,
					deferredOwner:    spec.RecoveryGroupID == domainprivatecas.EvidenceRegistryRecoveryGroupID,
				}
			}
		}
		if domainprivatecas.LooksLikeRecordResidueNameV1(name) {
			return privateAuthorityResidueClassification{state: privateAuthorityResidueMalformed}
		}
		return privateAuthorityResidueClassification{}
	}
	if domainprivatecas.LooksLikeRecordResidueNameV1(name) &&
		(domainprivatecas.KnownTopLevelOwnerV1(parts[0]) ||
			domainprivatecas.KnownTopLevelOwnerAliasV1(parts[0])) {
		return privateAuthorityResidueClassification{state: privateAuthorityResidueMalformed}
	}
	return privateAuthorityResidueClassification{}
}

func privateAuthorityTempLabelCandidate(label string) bool {
	return classifyPrivateAuthorityResidueLabel(label).state == privateAuthorityResidueKnown
}

func privateAuthorityJournalRemovableResidue(label string) bool {
	classified := classifyPrivateAuthorityResidueLabel(label)
	return classified.state == privateAuthorityResidueKnown && classified.journalRemovable
}

func privateCASCreateResidueMatchesKnownPath(relative string, name string) bool {
	suffix := "/" + name
	parent := ""
	if relative != name {
		if !strings.HasSuffix(relative, suffix) {
			return false
		}
		parent = strings.TrimSuffix(relative, suffix)
	}
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		components := strings.Split(spec.RelativeCASRoot, "/")
		for index, component := range components {
			expectedParent := strings.Join(components[:index], "/")
			if parent == expectedParent &&
				domainprivatecas.CreateDirectoryResidueMatchesComponentV1(name, component) {
				return true
			}
		}
		if parent == spec.RelativeCASRoot && domainprivatecas.CreateDirectoryResidueMatchesShardV1(name) {
			return true
		}
	}
	return false
}

func privateOpaqueCASRecordLabel(label string) bool {
	const prefix = "data/private/"
	if !strings.HasPrefix(label, prefix) {
		return false
	}
	spec, remainder, found := domainprivatecas.RootContainingRelativePathV1(strings.TrimPrefix(label, prefix))
	if !found || spec.SnapshotBodyPolicy != domainprivatecas.SnapshotOpaqueBytesV1 || len(remainder) != 2 {
		return false
	}
	const suffix = ".json"
	if !strings.HasSuffix(remainder[1], suffix) {
		return false
	}
	digest := strings.TrimSuffix(remainder[1], suffix)
	return domainprivatecas.ValidDigestV1(digest) && remainder[0] == digest[:2]
}

func isLowerHex(value string) bool {
	return domainprivatecas.IsLowerHexV1(value)
}

func scanStrictJSONFile(file io.Reader, label string, size int64) (FileRecord, error) {
	if size <= 0 || size > maxStrictJSONBytes {
		return FileRecord{}, integrity("invalid_json_size", label, 0, nil)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStrictJSONBytes+1))
	if err != nil {
		return FileRecord{}, integrity("read_failed", label, 0, err)
	}
	if int64(len(data)) != size {
		return FileRecord{}, integrity("file_changed_during_snapshot", label, 0, nil)
	}
	if err := validateStrictJSON(data, true); err != nil {
		return FileRecord{}, integrity("invalid_json", label, 0, err)
	}
	digest := sha256.Sum256(data)
	return FileRecord{Path: label, SHA256: hex.EncodeToString(digest[:]), Size: size, RecordCount: 1}, nil
}

func scanStrictJSONLFile(file io.Reader, target managedTarget, label string, relative string, size int64, allowLegacyEventOrder bool) (FileRecord, error) {
	hasher := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(file, hasher), 64*1024)
	lineNumber := 0
	recordCount := 0
	eventSequences := domainevent.SequenceSetV1{}
	previousEventSequence := int64(0)
	legacyOrderAuthoritySeen := false
	legacyPhysicalOrderDefect := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > maxStrictJSONLLine {
			return FileRecord{}, integrity("jsonl_line_too_large", label, lineNumber+1, nil)
		}
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return FileRecord{}, integrity("read_failed", label, lineNumber+1, readErr)
		}
		if errors.Is(readErr, io.EOF) && requiresStrictJSONLFraming(target, relative) {
			return FileRecord{}, integrity("jsonl_missing_final_newline", label, lineNumber+1, nil)
		}
		lineNumber++
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(bytes.TrimSpace(line)) == 0 {
			if requiresStrictJSONLFraming(target, relative) {
				return FileRecord{}, integrity("jsonl_blank_line", label, lineNumber, nil)
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		record, err := decodeStrictJSONObject(line)
		if err != nil {
			return FileRecord{}, integrity("invalid_jsonl_record", label, lineNumber, err)
		}
		if target.threadTree {
			threadID := threadIDFromRelative(relative)
			previousSequence := previousEventSequence
			if err := validateThreadSidecarRecord(
				threadID,
				filepath.Base(relative),
				record,
				&eventSequences,
				&previousEventSequence,
				allowLegacyEventOrder,
			); err != nil {
				return FileRecord{}, integrity("thread_record_identity", label, lineNumber, err)
			}
			if filepath.Base(relative) == "events.jsonl" && allowLegacyEventOrder {
				legacyOrderAuthoritySeen = legacyOrderAuthoritySeen || domainstartup.ContainsCurrentEventOrderAuthorityV1(record)
				legacyPhysicalOrderDefect = legacyPhysicalOrderDefect ||
					previousSequence != 0 && previousEventSequence != previousSequence+1
			}
		}
		recordCount++
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if target.threadTree && filepath.Base(relative) == "events.jsonl" {
		if err := eventSequences.ValidateContiguous(); err != nil {
			return FileRecord{}, integrity("thread_record_identity", label, lineNumber, err)
		}
		if legacyPhysicalOrderDefect && legacyOrderAuthoritySeen {
			return FileRecord{}, integrity(
				"thread_record_identity", label, lineNumber,
				errors.New("current execution or terminal authority cannot use legacy event order"),
			)
		}
	}
	if size == 0 {
		return FileRecord{Path: label, SHA256: hex.EncodeToString(hasher.Sum(nil)), Size: 0, RecordCount: 0}, nil
	}
	return FileRecord{Path: label, SHA256: hex.EncodeToString(hasher.Sum(nil)), Size: size, RecordCount: recordCount}, nil
}

func requiresStrictJSONLFraming(target managedTarget, relative string) bool {
	if target.label == "data/private" {
		return true
	}
	return target.threadTree && filepath.Base(relative) == "events.jsonl"
}

// classifyMigratableEventOrderV1 identifies historical thread layouts whose
// complete event sequence may be physically out of order before the signed
// semantic-startup migration canonicalizes it. Both primary thread.json and
// sidecar-only metadata layouts are host-owned inputs. The scanner still
// rejects gaps, duplicates, identity mismatches, torn framing, links, and
// unowned event-only directories before a migration journal can be created.
func classifyMigratableEventOrderV1(target managedTarget, path, relative string) (bool, error) {
	if !target.threadTree || filepath.Base(relative) != "events.jsonl" {
		return false, nil
	}
	threadDir := filepath.Dir(path)
	residue, err := eventOrderTransactionResidueV1(threadDir)
	if err != nil {
		return false, err
	}
	if residue {
		return false, nil
	}
	primary, err := os.Lstat(filepath.Join(threadDir, "thread.json"))
	if err == nil {
		if primary.Mode()&os.ModeSymlink != 0 || !primary.Mode().IsRegular() {
			return false, errors.New("primary thread authority is not a regular file")
		}
		return legacyPrimaryThreadAllowsEventOrderMigrationV1(
			filepath.Join(threadDir, "thread.json"),
			threadIDFromRelative(relative),
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	metadata, err := os.Lstat(filepath.Join(threadDir, "metadata.jsonl"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if metadata.Mode()&os.ModeSymlink != 0 || !metadata.Mode().IsRegular() {
		return false, errors.New("legacy sidecar metadata is not a regular file")
	}
	return true, nil
}

func legacyPrimaryThreadAllowsEventOrderMigrationV1(path, threadID string) (result bool, resultErr error) {
	initialInfo, err := os.Lstat(path)
	if err != nil || initialInfo.Mode()&os.ModeSymlink != 0 || !initialInfo.Mode().IsRegular() ||
		initialInfo.Size() <= 0 || initialInfo.Size() > maxStrictJSONBytes {
		return false, errors.New("legacy primary thread authority is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return false, errors.New("legacy primary thread authority is unavailable")
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(initialInfo, openedInfo) || openedInfo.Size() != initialInfo.Size() {
		return false, errors.New("legacy primary thread authority changed before classification")
	}
	initialIdentity, linkCount, err := regularFileIdentity(file, openedInfo)
	if err != nil || linkCount != 1 {
		return false, errors.New("legacy primary thread authority identity is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxStrictJSONBytes+1))
	if err != nil || int64(len(body)) != initialInfo.Size() {
		return false, errors.New("legacy primary thread authority could not be read exactly")
	}
	record, err := decodeStrictJSONObject(body)
	for index := range body {
		body[index] = 0
	}
	if err != nil {
		return false, errors.New("legacy primary thread authority is not strict JSON")
	}
	if recordID := stringValue(record["id"]); !exactSafeRecordID(threadID) || recordID != "" && recordID != threadID {
		return false, errors.New("legacy primary thread authority identity is invalid")
	}
	if !domainstartup.LegacyPrimaryThreadAllowsEventOrderMigrationV1(record, threadID) {
		return false, nil
	}
	currentInfo, lstatErr := os.Lstat(path)
	refreshedInfo, statErr := file.Stat()
	currentIdentity := ""
	currentLinks := uint64(0)
	var identityErr error
	if statErr == nil {
		currentIdentity, currentLinks, identityErr = regularFileIdentity(file, refreshedInfo)
	}
	if lstatErr != nil || statErr != nil || identityErr != nil || currentInfo.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(openedInfo, currentInfo) || currentIdentity != initialIdentity || currentLinks != linkCount ||
		currentInfo.Size() != initialInfo.Size() || currentInfo.Mode() != initialInfo.Mode() ||
		!currentInfo.ModTime().Equal(initialInfo.ModTime()) {
		return false, errors.New("legacy primary thread authority changed during classification")
	}
	return true, nil
}

func eventOrderTransactionResidueV1(threadDir string) (bool, error) {
	directory, err := os.Open(threadDir)
	if err != nil {
		return false, err
	}
	defer directory.Close()
	const maxEntries = 64
	entries, readErr := directory.ReadDir(maxEntries + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, readErr
	}
	if len(entries) > maxEntries {
		return false, errors.New("legacy event-order thread directory exceeds the classification limit")
	}
	for _, entry := range entries {
		if domainstartup.IsEventOrderTransactionResidueV1(entry.Name()) {
			return true, nil
		}
	}
	return false, nil
}

func hashOpaqueFile(file io.Reader, label string, size int64) (FileRecord, error) {
	hasher := sha256.New()
	written, err := io.Copy(hasher, file)
	if err != nil {
		return FileRecord{}, integrity("read_failed", label, 0, err)
	}
	if written != size {
		return FileRecord{}, integrity("file_changed_during_snapshot", label, 0, nil)
	}
	return FileRecord{Path: label, SHA256: hex.EncodeToString(hasher.Sum(nil)), Size: size}, nil
}

func validateThreadJSONFile(file *os.File, target managedTarget, label string, relative string) error {
	if filepath.Base(relative) != "thread.json" {
		return nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return integrity("read_failed", label, 0, err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStrictJSONBytes+1))
	if err != nil {
		return integrity("read_failed", label, 0, err)
	}
	record, err := decodeStrictJSONObject(data)
	if err != nil {
		return integrity("invalid_json", label, 0, err)
	}
	threadID := threadIDFromRelative(relative)
	if recordID := stringValue(record["id"]); !exactSafeRecordID(threadID) || (recordID != "" && recordID != threadID) {
		return integrity("thread_id_mismatch", label, 0, nil)
	}
	_ = target
	return nil
}

func validateThreadSidecarRecord(
	threadID string,
	fileName string,
	record map[string]any,
	eventSequences *domainevent.SequenceSetV1,
	previousEventSequence *int64,
	allowLegacyEventOrder bool,
) error {
	if !exactSafeRecordID(threadID) {
		return errors.New("unsafe thread directory")
	}
	switch fileName {
	case "events.jsonl":
		if stringValue(record["threadId"]) != threadID {
			return errors.New("event threadId mismatch")
		}
		seq, err := integerValue(record["seq"])
		if err != nil || seq <= 0 {
			return errors.New("event seq must be a positive integer")
		}
		if err := eventSequences.AddPositiveUnique(seq); err != nil {
			return err
		}
		if !allowLegacyEventOrder && previousEventSequence != nil && *previousEventSequence != 0 && seq != *previousEventSequence+1 {
			return fmt.Errorf("%w: expected %d", errEventPhysicalSequenceMismatchV1, *previousEventSequence+1)
		}
		if previousEventSequence != nil {
			*previousEventSequence = seq
		}
	case "messages.jsonl":
		if value := stringValue(record["threadId"]); value != "" && value != threadID {
			return errors.New("message threadId mismatch")
		}
	case "metadata.jsonl":
		if value := stringValue(record["threadId"]); value != "" && value != threadID {
			return errors.New("metadata threadId mismatch")
		}
		if nested, ok := record["thread"].(map[string]any); ok {
			if value := stringValue(nested["id"]); value != "" && value != threadID {
				return errors.New("metadata thread.id mismatch")
			}
		}
	}
	return nil
}

func integerValue(value any) (int64, error) {
	switch typed := value.(type) {
	case json.Number:
		return typed.Int64()
	case float64:
		integer := int64(typed)
		if float64(integer) != typed {
			return 0, errors.New("not an integer")
		}
		return integer, nil
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	default:
		return 0, errors.New("not a number")
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func exactSafeRecordID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." &&
		!strings.Contains(value, "..") && !strings.ContainsAny(value, "/\\:")
}

func threadIDFromRelative(relative string) string {
	parts := splitRelativePath(relative)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func splitRelativePath(value string) []string {
	value = filepath.ToSlash(filepath.Clean(value))
	if value == "." || value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func targetLabel(target managedTarget, path string) string {
	relative, err := filepath.Rel(target.path, path)
	if err != nil || relative == "." {
		return target.label
	}
	return target.label + "/" + filepath.ToSlash(relative)
}

func integrity(code string, path string, line int, err error) error {
	return IntegrityError{Code: code, Path: path, Line: line, Err: err}
}
