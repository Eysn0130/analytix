package evidenceregistry

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

type originalRegistryCommittedBodyV2 struct {
	size   int
	digest string
}

// SnapshotOriginalFilesV1 observes the complete original owner. V2 bytes are
// evidence, including unselected local candidates and opaque crash residues;
// observing them never selects a current registry or opens a writable store.
func (prepared *PreparedRecoveryV2) SnapshotOriginalFilesV1(ctx context.Context) (map[string]OriginalLegacyEntryV1, error) {
	if prepared == nil || prepared.indexes == nil || prepared.capsules == nil {
		return nil, errors.New("original registry physical observation is unavailable")
	}
	if !prepared.indexes.Present() && !prepared.capsules.Present() {
		return prepared.SnapshotOriginalLegacyFilesV1(ctx)
	}
	return prepared.snapshotOriginalFilesV2(ctx)
}

func (prepared *PreparedRecoveryV2) snapshotOriginalFilesV2(ctx context.Context) (files map[string]OriginalLegacyEntryV1, resultErr error) {
	if ctx == nil {
		return nil, errors.New("original registry V2 context is unavailable")
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
	committed := map[string]originalRegistryCommittedBodyV2{}
	var committedBytes int64
	for leaf, plan := range map[string]*finalauthorityadapter.PreparedSecurePrivateCASRecoveryV1{"indexes": prepared.indexes, "capsules": prepared.capsules} {
		if err := plan.VisitCommittedFiles(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
			committedBytes += int64(len(file.Body))
			if len(committed) >= domainstartup.MaxManagedSnapshotEntriesV1 || committedBytes > domainstartup.MaxSemanticStagedTotalBytesV1 {
				return errors.New("original registry V2 committed budget exceeded")
			}
			committed[leaf+"/"+file.Digest[:2]+"/"+file.Digest+".json"] = originalRegistryCommittedBodyV2{size: len(file.Body), digest: domainsecurity.SHA256Hex(file.Body)}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	files = map[string]OriginalLegacyEntryV1{}
	var total int64
	err := filepath.WalkDir(prepared.root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(files) >= domainstartup.MaxManagedSnapshotEntriesV1 {
			return errors.New("original registry V2 entry budget exceeded")
		}
		before, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(prepared.root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		value := OriginalLegacyEntryV1{Directory: before.IsDir(), Mode: uint32(before.Mode().Perm())}
		if !value.Directory {
			if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > domainstartup.MaxSemanticManagedFileBytesV1 {
				return errors.New("original registry V2 file kind or size is invalid")
			}
			total += before.Size()
			if total > domainstartup.MaxSemanticStagedTotalBytesV1 {
				return errors.New("original registry V2 byte budget exceeded")
			}
			file, err := os.Open(name)
			if err != nil {
				return err
			}
			if err := prepared.validateOriginalOpenedFileV2(ctx, file, before, relative, committed); err != nil {
				return errors.Join(err, file.Close())
			}
			value.Body, err = readOriginalRegistryBodyV1(file, before.Size())
			if err != nil {
				return err
			}
			if bound, found := committed[relative]; found && (bound.size != len(value.Body) || bound.digest != domainsecurity.SHA256Hex(value.Body)) {
				return errors.New("original registry V2 body differs from bound CAS observation")
			}
		}
		files[relative] = value
		return nil
	})
	if err != nil {
		return nil, err
	}
	for name, body := range committed {
		if value, found := files[name]; !found || value.Directory || len(value.Body) != body.size || domainsecurity.SHA256Hex(value.Body) != body.digest {
			return nil, errors.New("original registry V2 committed denominator is incomplete")
		}
	}
	return files, nil
}

// A two-link read is admitted only for the exact producer pair already
// accepted by this complete native prepared observation. Ordinary readers
// retain their single-link rule, and the enclosing snapshot repeats the
// native identity and full-link denominator proof before returning bytes.
func (prepared *PreparedRecoveryV2) validateOriginalOpenedFileV2(ctx context.Context, file *os.File, before os.FileInfo, relative string, committed map[string]originalRegistryCommittedBodyV2) error {
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || before.Mode() != opened.Mode() || before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return errors.Join(errors.New("original registry V2 opened file identity changed"), err)
	}
	links, err := registryRegularFileLinkCount(file, opened)
	if err != nil {
		return err
	}
	if links == 1 {
		return nil
	}
	parts := strings.Split(relative, "/")
	if links != 2 || len(parts) != 3 || (parts[0] != "indexes" && parts[0] != "capsules") || !domainprivatecas.ValidShardV1(parts[1]) {
		return errors.New("original registry V2 file has unsafe link authority")
	}
	base := parts[2]
	if residue, found := domainprivatecas.ClassifyRecordResidueNameV1(base, parts[1]); found {
		digest := residue.OriginalName[1:65]
		canonical := path.Join(parts[0], parts[1], digest+".json")
		if _, known := committed[canonical]; !known {
			return errors.New("original registry V2 linked residue lacks native committed proof")
		}
		peer, err := os.Lstat(filepath.Join(prepared.root, filepath.FromSlash(canonical)))
		if err != nil || !os.SameFile(before, peer) {
			return errors.Join(errors.New("original registry V2 linked residue lost its exact committed partner"), err)
		}
		return ctx.Err()
	}
	if _, known := committed[relative]; !known {
		return errors.New("original registry V2 linked record lacks native committed proof")
	}
	digest := strings.TrimSuffix(base, ".json")
	if !domainprivatecas.ValidDigestV1(digest) || base != digest+".json" || digest[:2] != parts[1] {
		return errors.New("original registry V2 linked committed address is invalid")
	}
	directory := filepath.Join(prepared.root, filepath.FromSlash(path.Dir(relative)))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	partners := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		residue, found := domainprivatecas.ClassifyRecordResidueNameV1(entry.Name(), parts[1])
		if !found || residue.OriginalName[1:65] != digest {
			continue
		}
		peer, err := entry.Info()
		if err != nil {
			return err
		}
		if os.SameFile(before, peer) {
			partners++
		}
	}
	if partners != 1 {
		return errors.New("original registry V2 linked committed record has no unique producer partner")
	}
	return ctx.Err()
}
