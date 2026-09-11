package evidencesettlement

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	settlementport "analytix.local/runtime-go/internal/ports/evidencesettlement"
)

const maxPreparedEvidenceSettlementBytes = 32 * 1024 * 1024

type Store struct {
	mu                   sync.Mutex
	prepared             string
	originalInventory    func(context.Context) ([]domainevidence.PreparedEvidenceSettlement, error)
	validateRestartWrite func(context.Context, domainevidence.PreparedEvidenceSettlement) error
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("private evidence settlement root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, err
	}
	root = realRoot
	prepared := filepath.Join(root, "prepared")
	if err := ensurePrivateDirectory(prepared); err != nil {
		return nil, err
	}
	return &Store{prepared: prepared}, nil
}

func (store *Store) PutPreparedIfAbsent(ctx context.Context, record domainevidence.PreparedEvidenceSettlement) error {
	if store == nil || domainevidence.ValidatePreparedEvidenceSettlementForExecution(record) != nil {
		return errors.New("prepared evidence settlement is invalid")
	}
	return store.putPreparedBody(ctx, record)
}

// PutPreparedIfAbsentWithHostAuthority is the narrow DSV2 host-authority
// persistence seam. The legacy PutPreparedIfAbsent path deliberately keeps
// rejecting bare DSV2 probes; only this method accepts a signed host witness
// while its caller holds the non-serializable capability lease.
func (store *Store) PutPreparedIfAbsentWithHostAuthority(
	ctx context.Context,
	record domainevidence.PreparedEvidenceSettlement,
	input settlementport.HostAuthorityInput,
) error {
	if store == nil || input.Capability == nil ||
		domainevidence.ValidatePreparedEvidenceSettlementForHostAuthorityV2(
			record, input.Context, input.CurrentProbe, input.SelectionDigest,
		) != nil || record.HostAuthority == nil || record.HostAuthority.Binding != input.Binding {
		return errors.New("prepared evidence settlement host authority is invalid")
	}
	selection, err := input.Capability.DatasetSelection()
	if err != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
		selection.SelectionDigest != input.SelectionDigest {
		return errors.New("prepared evidence settlement host selection is unavailable")
	}
	return store.putPreparedBody(ctx, record)
}

func (store *Store) putPreparedBody(ctx context.Context, record domainevidence.PreparedEvidenceSettlement) (resultErr error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	body, err := domainevidence.PreparedEvidenceSettlementBytes(record)
	if err != nil || len(body) == 0 || len(body) > maxPreparedEvidenceSettlementBytes {
		return errors.New("prepared evidence settlement exceeds private storage limit")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.validateRestartWrite != nil {
		if err := store.validateRestartWrite(ctx, record); err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, store.validateRestartWrite(ctx, record)) }()
	}
	path := store.recordPath(record.SettlementID)
	if existing, err := store.readPath(path); err == nil {
		existingBody, _ := domainevidence.PreparedEvidenceSettlementBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("prepared evidence settlement conflicts with existing authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writePrivateFileExclusive(path, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := store.readPath(path)
		if readErr != nil {
			return readErr
		}
		existingBody, _ := domainevidence.PreparedEvidenceSettlementBytes(existing)
		if bytes.Equal(existingBody, body) {
			return nil
		}
		return errors.New("prepared evidence settlement conflicts with concurrently created authority")
	}
	written, err := store.readPath(path)
	if err != nil || written.RecordDigest != record.RecordDigest {
		return errors.New("prepared evidence settlement write verification failed")
	}
	return nil
}

func (store *Store) ResolvePrepared(ctx context.Context, settlementID string) (domainevidence.PreparedEvidenceSettlement, error) {
	settlementID = strings.TrimSpace(settlementID)
	if store == nil || !domainsecurity.IsSHA256Hex(settlementID) {
		return domainevidence.PreparedEvidenceSettlement{}, errors.New("evidence settlement id is invalid")
	}
	if err := contextError(ctx); err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readPath(store.recordPath(settlementID))
}

func (store *Store) ListPrepared(ctx context.Context) ([]domainevidence.PreparedEvidenceSettlement, error) {
	if store == nil {
		return nil, errors.New("private evidence settlement store is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.originalInventory != nil {
		return store.originalInventory(ctx)
	}
	records := []domainevidence.PreparedEvidenceSettlement{}
	err := filepath.WalkDir(store.prepared, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == store.prepared {
			return validatePrivateDirectory(path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("private evidence settlement store contains a symlink")
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(store.prepared, path)
			if err != nil || len(relative) != 2 || strings.ContainsAny(relative, `/\\`) || !domainsecurity.IsSHA256Hex(relative+strings.Repeat("0", 62)) {
				return errors.New("private evidence settlement shard address is invalid")
			}
			return validatePrivateDirectory(path)
		}
		if filepath.Ext(entry.Name()) != ".json" {
			return errors.New("private evidence settlement store contains an unknown file")
		}
		record, err := store.readPath(path)
		if err != nil {
			return err
		}
		if path != store.recordPath(record.SettlementID) {
			return errors.New("prepared evidence settlement filename does not match its authority")
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].SettlementID < records[j].SettlementID })
	return records, nil
}

func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	records, err := store.ListPrepared(ctx)
	return len(records) > 0, err
}

func (store *Store) recordPath(settlementID string) string {
	return filepath.Join(store.prepared, settlementID[:2], settlementID+".json")
}

func (store *Store) readPath(path string) (domainevidence.PreparedEvidenceSettlement, error) {
	record, _, err := store.readPathBytes(path)
	return record, err
}

func (store *Store) readPathBytes(path string) (domainevidence.PreparedEvidenceSettlement, []byte, error) {
	info, err := validatePrivateRegularFile(path)
	if err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, nil, err
	}
	if info.Size() <= 0 || info.Size() > maxPreparedEvidenceSettlementBytes {
		return domainevidence.PreparedEvidenceSettlement{}, nil, errors.New("prepared evidence settlement file size is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, nil, err
	}
	openedInfo, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, openedInfo) {
		return domainevidence.PreparedEvidenceSettlement{}, nil, errors.Join(errors.New("prepared evidence settlement file identity changed"), statErr, file.Close())
	}
	body, readErr := io.ReadAll(io.LimitReader(file, maxPreparedEvidenceSettlementBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, nil, err
	}
	record, err := domainevidence.ParsePreparedEvidenceSettlement(body)
	if err != nil {
		return domainevidence.PreparedEvidenceSettlement{}, nil, fmt.Errorf("read prepared evidence settlement: %w", err)
	}
	return record, body, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return validatePrivateDirectory(path)
}

func validatePrivateDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil || filepath.Clean(real) != filepath.Clean(abs) {
		return errors.New("private evidence settlement directory traverses a symlink")
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("private evidence settlement directory is not regular")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("private evidence settlement directory permissions are too broad")
	}
	return nil
}

func validatePrivateRegularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("private evidence settlement path is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("private evidence settlement file permissions are too broad")
	}
	return info, nil
}

func writePrivateFileExclusive(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateDirectory(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Link(tempPath, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(path string) error {
	directory, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
