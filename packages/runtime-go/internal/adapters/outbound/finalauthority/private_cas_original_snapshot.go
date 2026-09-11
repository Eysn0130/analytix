package finalauthority

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// SecurePrivateCASOriginalEntryV1 is original physical evidence, not a live
// record or recovery capability. Modes follow os.FileInfo.Mode().Perm().
type SecurePrivateCASOriginalEntryV1 struct {
	Directory bool
	Mode      uint32
	Body      []byte
}

const maxPrivateCASOriginalSnapshotBytesV1 = 2 << 30
const maxPrivateCASOriginalSnapshotEntriesV1 = 100000

func putPrivateCASOriginalEntryV1(entries map[string]SecurePrivateCASOriginalEntryV1, name string, entry SecurePrivateCASOriginalEntryV1, bytes *int64) error {
	if _, found := entries[name]; found {
		return errors.New("original private CAS entry is duplicated")
	}
	*bytes += int64(len(entry.Body))
	if len(entries) >= maxPrivateCASOriginalSnapshotEntriesV1 || *bytes > maxPrivateCASOriginalSnapshotBytesV1 {
		return errors.New("original private CAS snapshot exceeds bounds")
	}
	entries[name] = entry
	return nil
}

// SnapshotOriginalEntriesV1 reads the entire fixed leaf plan, including opaque
// residues and empty shards, without accepting a caller-selected path. The
// native frozen plan remains responsible for exact paired-link authority.
func (prepared *PreparedSecurePrivateCASRecoveryV1) SnapshotOriginalEntriesV1(ctx context.Context) (entries map[string]SecurePrivateCASOriginalEntryV1, resultErr error) {
	return prepared.snapshotOriginalEntriesV1(ctx, prepared.Revalidate)
}

func (prepared *PreparedSecurePrivateCASRecoveryV1) snapshotOriginalEntriesV1(ctx context.Context, revalidate func(context.Context) error) (entries map[string]SecurePrivateCASOriginalEntryV1, resultErr error) {
	if ctx == nil || prepared == nil || prepared.access == nil || prepared.gate == nil || prepared.rootPath == "" {
		return nil, errors.New("original private CAS snapshot is unavailable")
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	release, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, revalidate(ctx), context.Cause(ctx))
		if resultErr != nil {
			entries = nil
		}
	}()
	if !prepared.present {
		return map[string]SecurePrivateCASOriginalEntryV1{}, nil
	}
	resultErr = withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("original private CAS snapshot binding changed")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || !present || authority != prepared.authority {
			return errors.Join(errors.New("original private CAS snapshot root changed"), err)
		}
		entries, err = securePrivateCASOriginalSnapshotV1(ctx, authority, prepared.observation, prepared.maxBytes, prepared.originalCreates != nil)
		return err
	})
	if resultErr == nil && prepared.originalCreates != nil {
		prefix := prepared.rootPath + string(filepath.Separator)
		for _, state := range prepared.originalCreates.DirectoryStatesV1() {
			absolute := filepath.Join(prepared.originalCreates.DataRootV1(), filepath.FromSlash(state.RelativePath))
			if !strings.HasPrefix(absolute, prefix) {
				continue
			}
			name := filepath.ToSlash(strings.TrimPrefix(absolute, prefix))
			if _, exists := entries[name]; exists {
				return nil, errors.New("original create directory overlaps canonical snapshot")
			}
			entries[name] = SecurePrivateCASOriginalEntryV1{Directory: true, Mode: state.Mode & 0o777}
		}
	}
	return entries, resultErr
}

// SnapshotOriginalEntriesV1 combines every leaf of a fixed owner. Mixed and
// discovered owners have separate grammars and cannot use this observation.
func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) SnapshotOriginalEntriesV1(ctx context.Context) (entries map[string]SecurePrivateCASOriginalEntryV1, resultErr error) {
	if ctx == nil || prepared == nil || prepared.container == nil || len(prepared.leaves) == 0 ||
		prepared.container.discovery != nil || len(prepared.container.regularFiles) != 0 {
		return nil, errors.New("original fixed private CAS owner snapshot is unavailable")
	}
	return prepared.snapshotOriginalEntriesV1(ctx, prepared.Revalidate)
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) snapshotOriginalEntriesV1(ctx context.Context, revalidate func(context.Context) error, originals ...*PreparedSecurePrivateCASOriginalCreateResiduesV1) (entries map[string]SecurePrivateCASOriginalEntryV1, resultErr error) {
	if ctx == nil || revalidate == nil {
		return nil, errors.New("original private CAS owner observation is unavailable")
	}
	if len(originals) > 1 {
		return nil, errors.New("original owner snapshot creation binding is ambiguous")
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	release, err := privateCASRecoveryExclusion.acquireObservationV1(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := revalidate(ctx); err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, revalidate(ctx), context.Cause(ctx))
		if resultErr != nil {
			entries = nil
		}
	}()
	entries = map[string]SecurePrivateCASOriginalEntryV1{}
	if !prepared.container.present {
		return entries, nil
	}
	container := prepared.container
	var mode uint32
	if err := withExistingPrivateCASAccess(ctx, container.access, container.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != container.binding {
			return errors.New("original private CAS owner snapshot binding changed")
		}
		if err := acquirePrivateCASGate(ctx, container.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(container.gate)
		var err error
		mode, err = privateCASOriginalRootModeV1(container.authority)
		return err
	}); err != nil {
		return nil, err
	}
	if container.originalRootMode != 0 && mode != container.originalRootMode {
		return nil, errors.New("original private CAS owner mode changed")
	}
	entries["."] = SecurePrivateCASOriginalEntryV1{Directory: true, Mode: mode}
	var bytes int64
	for _, leaf := range prepared.leaves {
		leafRevalidate := leaf.plan.Revalidate
		if len(originals) == 1 && originals[0] != nil {
			if leaf.plan.originalCreates != originals[0] || originals[0].OwnerRootV1() != prepared.container.rootPath {
				return nil, errors.New("original owner snapshot leaf creation binding differs")
			}
			leafRevalidate = leaf.plan.revalidateOriginalLeafV1
		}
		original, err := leaf.plan.snapshotOriginalEntriesV1(ctx, leafRevalidate)
		if err != nil {
			return nil, err
		}
		for name, entry := range original {
			path := leaf.name
			if name != "." {
				path += "/" + name
			}
			if err := putPrivateCASOriginalEntryV1(entries, path, entry, &bytes); err != nil {
				return nil, err
			}
		}
	}
	return entries, nil
}
