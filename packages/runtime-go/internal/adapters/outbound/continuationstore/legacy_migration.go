package continuationstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

const (
	legacyReceiptPartition     = "receipts"
	legacyDispositionPartition = "dispositions"
)

// SemanticMigrationAccessAuthority is implemented only by the isolated
// semantic-startup stage capability (and its test oracle). Live runtime
// persistence authority cannot invoke a destructive legacy migration.
type SemanticMigrationAccessAuthority interface {
	finalauthorityadapter.SecurePrivateCASAccessAuthority
	IsSemanticStagePrivateCASAccessAuthority() bool
}

type legacyInventoryV1 struct {
	receipts     []domaincontinuation.Receipt
	dispositions []domaincontinuation.Disposition
}

// MigrateLegacyV1ForSemanticStage converts the pre-CAS continuation layout
// only inside the throwaway semantic stage. The outer signed startup journal
// owns publication of the resulting create/delete delta to live persistence.
func MigrateLegacyV1ForSemanticStage(
	ctx context.Context,
	root string,
	access SemanticMigrationAccessAuthority,
) error {
	if access == nil || !access.IsSemanticStagePrivateCASAccessAuthority() {
		return errors.New("legacy continuation migration requires semantic-stage authority")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" || filepath.Clean(absolute) != absolute {
		return errors.New("legacy continuation migration root is invalid")
	}
	present, err := legacyPartitionsPresent(absolute)
	if err != nil || !present {
		return err
	}
	before, err := readLegacyInventory(ctx, absolute)
	if err != nil {
		return err
	}
	store, err := openStoreContext(ctx, absolute, access, true)
	if err != nil {
		return err
	}
	defer store.Close()
	for _, receipt := range before.receipts {
		if err := store.PutReceiptIfAbsent(ctx, receipt); err != nil {
			return err
		}
	}
	for _, disposition := range before.dispositions {
		if err := store.PutDispositionIfAbsent(ctx, disposition); err != nil {
			return err
		}
	}
	if err := verifyMigratedInventory(ctx, store, before); err != nil {
		return err
	}
	if err := store.Close(); err != nil {
		return err
	}
	after, err := readLegacyInventory(ctx, absolute)
	if err != nil || !equalLegacyInventory(before, after) {
		return errors.Join(errors.New("legacy continuation inventory changed during migration"), err)
	}
	if err := removeLegacyPartitions(absolute); err != nil {
		return err
	}
	return validateContinuationOwnerRoot(absolute, false)
}

func verifyMigratedInventory(ctx context.Context, store *Store, legacy legacyInventoryV1) error {
	receipts := make([]domaincontinuation.Receipt, 0, len(legacy.receipts))
	if err := store.VisitReceipts(ctx, func(receipt domaincontinuation.Receipt) error {
		receipts = append(receipts, receipt)
		return nil
	}); err != nil {
		return err
	}
	dispositions := make([]domaincontinuation.Disposition, 0, len(legacy.dispositions))
	if err := store.VisitDispositions(ctx, func(disposition domaincontinuation.Disposition) error {
		dispositions = append(dispositions, disposition)
		return nil
	}); err != nil {
		return err
	}
	if !equalLegacyInventory(legacy, legacyInventoryV1{receipts: receipts, dispositions: dispositions}) {
		return errors.New("legacy continuation migration did not preserve exact authority bytes")
	}
	return nil
}

func equalLegacyInventory(left, right legacyInventoryV1) bool {
	if len(left.receipts) != len(right.receipts) || len(left.dispositions) != len(right.dispositions) {
		return false
	}
	for index := range left.receipts {
		leftBody, leftErr := domaincontinuation.ReceiptBytes(left.receipts[index])
		rightBody, rightErr := domaincontinuation.ReceiptBytes(right.receipts[index])
		if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
			return false
		}
	}
	for index := range left.dispositions {
		leftBody, leftErr := domaincontinuation.DispositionBytes(left.dispositions[index])
		rightBody, rightErr := domaincontinuation.DispositionBytes(right.dispositions[index])
		if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
			return false
		}
	}
	return true
}

func readLegacyInventory(ctx context.Context, root string) (legacyInventoryV1, error) {
	if err := contextError(ctx); err != nil {
		return legacyInventoryV1{}, err
	}
	receiptFiles, err := snapshotLegacyRecordFiles(ctx, filepath.Join(root, legacyReceiptPartition), "receipt.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return legacyInventoryV1{}, err
	}
	dispositionFiles, dispositionErr := snapshotLegacyRecordFiles(ctx, filepath.Join(root, legacyDispositionPartition), "disposition.json")
	if dispositionErr != nil && !errors.Is(dispositionErr, os.ErrNotExist) {
		return legacyInventoryV1{}, dispositionErr
	}
	inventory := legacyInventoryV1{
		receipts:     make([]domaincontinuation.Receipt, 0, len(receiptFiles)),
		dispositions: make([]domaincontinuation.Disposition, 0, len(dispositionFiles)),
	}
	receipts := make(map[string]domaincontinuation.Receipt, len(receiptFiles))
	for _, file := range receiptFiles {
		receipt, err := domaincontinuation.ParseReceipt(file.body)
		canonical, canonicalErr := domaincontinuation.ReceiptBytes(receipt)
		if err != nil || canonicalErr != nil || receipt.Payload.GateID != file.gateID || !bytes.Equal(canonical, file.body) {
			return legacyInventoryV1{}, errors.New("legacy continuation receipt inventory is non-canonical or path-mismatched")
		}
		if _, duplicate := receipts[file.gateID]; duplicate {
			return legacyInventoryV1{}, errors.New("legacy continuation receipt inventory repeats a gate")
		}
		receipts[file.gateID] = receipt
		inventory.receipts = append(inventory.receipts, receipt)
	}
	for _, file := range dispositionFiles {
		disposition, err := domaincontinuation.ParseDisposition(file.body)
		canonical, canonicalErr := domaincontinuation.DispositionBytes(disposition)
		receipt, found := receipts[file.gateID]
		if err != nil || canonicalErr != nil || disposition.GateID != file.gateID || !bytes.Equal(canonical, file.body) ||
			!found || !dispositionMatchesReceipt(disposition, receipt) {
			return legacyInventoryV1{}, errors.New("legacy continuation disposition lost its exact receipt authority")
		}
		inventory.dispositions = append(inventory.dispositions, disposition)
	}
	return inventory, nil
}

func legacyPartitionsPresent(root string) (bool, error) {
	present := false
	for _, name := range []string{legacyReceiptPartition, legacyDispositionPartition} {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, errors.New("legacy continuation partition is unsafe")
		}
		present = true
	}
	return present, nil
}

func removeLegacyPartitions(root string) error {
	for _, name := range []string{legacyReceiptPartition, legacyDispositionPartition} {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("legacy continuation partition changed before retirement")
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	directory, err := os.Open(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type legacyContinuationRecordFile struct {
	gateID string
	body   []byte
}

func snapshotLegacyRecordFiles(ctx context.Context, root, fileName string) ([]legacyContinuationRecordFile, error) {
	if err := validateLegacyDirectory(root); err != nil {
		return nil, err
	}
	shards, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	files := make([]legacyContinuationRecordFile, 0)
	for _, shard := range shards {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		shardName := shard.Name()
		shardPath := filepath.Join(root, shardName)
		if len(shardName) != 2 || !isLowerHex(shardName) || !shard.IsDir() || validateLegacyDirectory(shardPath) != nil {
			return nil, errors.New("legacy continuation inventory contains an unsafe shard")
		}
		leaves, err := os.ReadDir(shardPath)
		if err != nil {
			return nil, err
		}
		for _, leaf := range leaves {
			gateID := leaf.Name()
			leafPath := filepath.Join(shardPath, gateID)
			if !leaf.IsDir() || !validGateID(gateID) || !strings.HasPrefix(gateIDDigest(gateID), shardName) || validateLegacyDirectory(leafPath) != nil {
				return nil, errors.New("legacy continuation inventory contains an unsafe record leaf")
			}
			entries, err := os.ReadDir(leafPath)
			if err != nil {
				return nil, err
			}
			if len(entries) != 1 || entries[0].Name() != fileName || entries[0].IsDir() {
				return nil, errors.New("legacy continuation record leaf is incomplete or contains unknown files")
			}
			recordPath := filepath.Join(leafPath, fileName)
			if err := validateLegacyFile(recordPath); err != nil {
				return nil, err
			}
			body, err := readLegacyFile(recordPath)
			if err != nil {
				return nil, err
			}
			files = append(files, legacyContinuationRecordFile{gateID: gateID, body: body})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].gateID < files[j].gateID })
	return files, nil
}

func validateLegacyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || legacyModeIsBroad(info.Mode()) {
		return errors.New("legacy continuation directory permissions are unsafe")
	}
	return nil
}

func validateLegacyFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || legacyModeIsBroad(info.Mode()) || info.Size() <= 0 || info.Size() > maxRecordBytes {
		return errors.New("legacy continuation record permissions or size are unsafe")
	}
	return nil
}

func readLegacyFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maxRecordBytes {
		return nil, errors.New("legacy continuation record file is unsafe")
	}
	body, err := os.ReadFile(path)
	if err != nil || int64(len(body)) != info.Size() {
		return nil, errors.New("legacy continuation record read failed")
	}
	return body, nil
}

func legacyModeIsBroad(mode os.FileMode) bool {
	return runtime.GOOS != "windows" && mode.Perm()&0o077 != 0
}
