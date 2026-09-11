package finalauthority

import (
	"context"
	"errors"
	"path"
	"runtime"
	"strings"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
)

// ValidateOriginalFixedOwnerEntriesV1 validates the entire raw endpoint before
// returning its committed bodies by leaf and digest. The input must separately
// be bound to a native original observation or an authenticated journal
// endpoint. Legal residues remain in that raw input and confer no authority.
func ValidateOriginalFixedOwnerEntriesV1(ctx context.Context, files map[string]SecurePrivateCASOriginalEntryV1, leaves []SecurePrivateCASOwnerLeafV1) (map[string]map[string][]byte, error) {
	if ctx == nil || len(leaves) == 0 {
		return nil, errors.New("original fixed owner grammar is unavailable")
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	limits := map[string]int{}
	committed := map[string]map[string][]byte{}
	for _, leaf := range leaves {
		if leaf.Name == "" || leaf.Name == "." || leaf.Name == ".." || strings.ContainsAny(leaf.Name, "/\\") || leaf.MaxBytes <= 0 || limits[leaf.Name] != 0 {
			return nil, errors.New("original fixed owner leaf grammar is invalid")
		}
		limits[leaf.Name] = leaf.MaxBytes
		committed[leaf.Name] = map[string][]byte{}
	}
	if len(files) > maxPrivateCASOriginalSnapshotEntriesV1 {
		return nil, errors.New("original fixed owner entry budget exceeded")
	}
	if len(files) == 0 {
		return committed, nil
	}
	root, found := files["."]
	if !found || !root.Directory || len(root.Body) != 0 {
		return nil, errors.New("original fixed owner root is invalid")
	}
	for _, leaf := range leaves {
		entry, found := files[leaf.Name]
		if !found || !entry.Directory || len(entry.Body) != 0 {
			return nil, errors.New("original fixed owner leaf is absent or invalid")
		}
	}
	var total int64
	for name, entry := range files {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		if name == "" || path.Clean(name) != name || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") ||
			entry.Mode == 0 || entry.Mode & ^uint32(0o777) != 0 || runtime.GOOS != "windows" && entry.Mode&0o077 != 0 {
			return nil, errors.New("original fixed owner address or mode is invalid")
		}
		if name == "." {
			continue
		}
		parent, found := files[path.Dir(name)]
		if !found || !parent.Directory {
			return nil, errors.New("original fixed owner parent is absent")
		}
		parts := strings.Split(name, "/")
		limit, known := limits[parts[0]]
		if !known || len(parts) > 3 || len(parts) >= 2 && !domainprivatecas.ValidShardV1(parts[1]) {
			return nil, errors.New("original fixed owner target is outside its grammar")
		}
		if len(parts) < 3 {
			if !entry.Directory || len(entry.Body) != 0 {
				return nil, errors.New("original fixed owner directory is invalid")
			}
			continue
		}
		if entry.Directory || len(entry.Body) > limit {
			return nil, errors.New("original fixed owner record kind or size is invalid")
		}
		total += int64(len(entry.Body))
		if total > maxPrivateCASOriginalSnapshotBytesV1 {
			return nil, errors.New("original fixed owner byte budget exceeded")
		}
		if _, residue := domainprivatecas.ClassifyRecordResidueNameV1(parts[2], parts[1]); residue {
			continue
		}
		digest := strings.TrimSuffix(parts[2], ".json")
		if !domainprivatecas.ValidDigestV1(digest) || parts[2] != digest+".json" || digest[:2] != parts[1] || len(entry.Body) == 0 {
			return nil, errors.New("original fixed owner committed address is invalid")
		}
		committed[parts[0]][digest] = entry.Body
	}
	return committed, nil
}
