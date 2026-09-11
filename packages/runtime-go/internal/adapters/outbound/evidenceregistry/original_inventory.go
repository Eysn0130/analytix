package evidenceregistry

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// SnapshotOriginalLegacyInventoryV1 reads the complete original V1 index,
// capsules and projections under the prepared physical owner observation.
// It returns historical evidence only: no constructor, lock, recovery,
// backfill, signing, or current publication capability is involved.
func (prepared *PreparedRecoveryV2) SnapshotOriginalLegacyInventoryV1(ctx context.Context, contexts []domainsecurity.TurnSecurityContext, verifier finalauthorityport.Verifier) (records []registryport.InventoryRecord, resultErr error) {
	files, err := prepared.SnapshotOriginalLegacyFilesV1(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.RevalidatePhysicalV2(ctx), ctx.Err())
		if resultErr != nil {
			records = nil
		}
	}()
	return ParseOriginalLegacyInventoryV1(ctx, files, contexts, verifier)
}

// SnapshotOriginalLegacyFilesV1 separates original physical observation from
// complete graph validation. Authenticated semantic recovery needs existing
// bytes even when only a prefix of a multi-file transaction has been applied.
// This raw inventory does not assert that a transaction or registry is valid.
func (prepared *PreparedRecoveryV2) SnapshotOriginalLegacyFilesV1(ctx context.Context) (files map[string]OriginalLegacyEntryV1, resultErr error) {
	if ctx == nil {
		return nil, errors.New("original registry observation context is unavailable")
	}
	if err := prepared.RevalidatePhysicalV2(ctx); err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.RevalidatePhysicalV2(ctx), ctx.Err())
		if resultErr != nil {
			files = nil
		}
	}()
	if prepared.indexes.Present() || prepared.capsules.Present() {
		return nil, errors.New("original legacy registry inventory cannot interpret witnessed V2 authority")
	}
	files = map[string]OriginalLegacyEntryV1{}
	if !prepared.topology.PresentV1() {
		return files, nil
	}
	err := filepath.WalkDir(prepared.root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(prepared.root, filePath)
		if err != nil {
			return err
		}
		observed := OriginalLegacyEntryV1{Directory: before.IsDir(), Mode: uint32(before.Mode().Perm())}
		if !observed.Directory {
			if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maxRegistrySiblingBytes {
				return errors.New("original registry physical file is invalid")
			}
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			if err := validateOpenedRegistryFile(file, before); err != nil {
				return errors.Join(err, file.Close())
			}
			observed.Body, err = readOriginalRegistryBodyV1(file, before.Size())
			if err != nil {
				return err
			}
		}
		files[filepath.ToSlash(relative)] = observed
		return nil
	})
	return files, err
}

func readOriginalRegistryBodyV1(file io.ReadCloser, size int64) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(file, size+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if int64(len(body)) != size {
		return nil, errors.New("original registry file length changed")
	}
	return body, nil
}
