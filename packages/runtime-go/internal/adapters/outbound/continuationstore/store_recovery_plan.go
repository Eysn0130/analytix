package continuationstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
)

type PreparedRecoveryV1 struct {
	topology     *finalauthorityadapter.PreparedSecurePrivateCASOwnerTopologyV1
	receipts     *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1
	dispositions *finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1
	validated    bool
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return nil, errors.New("private continuation prepared recovery root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private continuation prepared recovery root is invalid")
	}
	expectedEntries, err := discoverPreparedOwnerEntries(absolute)
	if err != nil {
		return nil, err
	}
	topology, err := finalauthorityadapter.PrepareSecurePrivateCASOwnerTopologyV1(
		ctx, absolute, expectedEntries, access,
	)
	if err != nil {
		return nil, err
	}
	receipts, err := finalauthorityadapter.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, filepath.Join(absolute, receiptPartitionV2), maxRecordBytes, access,
	)
	if err != nil {
		return nil, err
	}
	dispositions, err := finalauthorityadapter.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, filepath.Join(absolute, dispositionPartitionV2), maxRecordBytes, access,
	)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedRecoveryV1{topology: topology, receipts: receipts, dispositions: dispositions}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil || prepared.receipts == nil || prepared.dispositions == nil {
		return errors.New("private continuation recovery plan is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	receipts := make(map[string]domaincontinuation.Receipt)
	if err := prepared.receipts.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		receipt, err := domaincontinuation.ParseReceipt(file.Body)
		canonical, canonicalErr := domaincontinuation.ReceiptBytes(receipt)
		if err != nil || canonicalErr != nil || gateIDDigest(receipt.Payload.GateID) != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(errors.New("private continuation prepared receipt is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		receipts[receipt.Payload.GateID] = receipt
		return nil
	}); err != nil {
		return err
	}
	if err := prepared.dispositions.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		disposition, err := domaincontinuation.ParseDisposition(file.Body)
		canonical, canonicalErr := domaincontinuation.DispositionBytes(disposition)
		receipt, found := receipts[disposition.GateID]
		if err != nil || canonicalErr != nil || gateIDDigest(disposition.GateID) != file.Digest || !bytes.Equal(canonical, file.Body) ||
			!found || !dispositionMatchesReceipt(disposition, receipt) {
			return errors.Join(errors.New("private continuation prepared disposition lost its exact receipt authority"), err, canonicalErr)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil || prepared.receipts == nil || prepared.dispositions == nil {
		return errors.New("private continuation recovery plan is invalid")
	}
	if err := prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return err
	}
	if err := prepared.receipts.Revalidate(ctx); err != nil {
		return err
	}
	if err := prepared.dispositions.Revalidate(ctx); err != nil {
		return err
	}
	if prepared.receipts.Present() != prepared.topology.ContainsEntryV1(receiptPartitionV2) ||
		prepared.dispositions.Present() != prepared.topology.ContainsEntryV1(dispositionPartitionV2) ||
		prepared.receipts.Present() != prepared.dispositions.Present() {
		return errors.New("private continuation recovery CAS membership disagrees with its frozen owner topology")
	}
	return prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx)
}

func (prepared *PreparedRecoveryV1) RevalidatePrivateCASRecoveryTopologyV3(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil {
		return errors.New("private continuation recovery topology authority is invalid")
	}
	return prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.topology == nil {
		return nil
	}
	return []finalauthorityadapter.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.topology}
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil {
		return nil
	}
	return []*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1{prepared.receipts, prepared.dispositions}
}

// discoverPreparedOwnerEntries is only a bounded candidate-name discovery
// step. PrepareSecurePrivateCASOwnerTopologyV1 subsequently reopens the owner
// through the host recovery authority and proves the exact no-follow child
// inventory and identities before any result is trusted.
func discoverPreparedOwnerEntries(root string) ([]string, error) {
	directory, err := os.Open(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(5)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errors.Join(readErr, directory.Close())
	}
	if len(entries) > 4 {
		return nil, errors.Join(errors.New("private continuation recovery owner contains too many entries"), directory.Close())
	}
	if readErr == nil {
		extra, extraErr := directory.ReadDir(1)
		if len(extra) != 0 || !errors.Is(extraErr, io.EOF) {
			return nil, errors.Join(errors.New("private continuation recovery owner contains too many entries"), extraErr, directory.Close())
		}
	}
	if err := directory.Close(); err != nil {
		return nil, err
	}
	allowed := map[string]bool{
		receiptPartitionV2: true, dispositionPartitionV2: true,
		legacyReceiptPartition: true, legacyDispositionPartition: true,
	}
	names := make([]string, 0, len(entries))
	seenFolded := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		folded := strings.ToLower(name)
		if !entry.IsDir() || !allowed[name] {
			return nil, errors.New("private continuation recovery owner contains an unknown entry")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return nil, errors.New("private continuation recovery owner aliases an entry by case")
		}
		seenFolded[folded] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
