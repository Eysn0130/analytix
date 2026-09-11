//go:build darwin || linux

package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"golang.org/x/sys/unix"
)

func platformInspectLegacyCheckpointSnapshotQuarantine(
	ctx context.Context,
	binding privatecasport.RootBinding,
) (LegacyCheckpointSnapshotQuarantineStateV1, error) {
	root, err := openLegacyCheckpointUnixBoundRoot(binding)
	if err != nil {
		return LegacyCheckpointSnapshotQuarantineStateV1{}, err
	}
	defer unix.Close(root)

	state := LegacyCheckpointSnapshotQuarantineStateV1{}
	source, present, err := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
		return inspectLegacyCheckpointUnixTreeAt(ctx, root, LegacyCheckpointSnapshotDirectoryV1)
	})
	if err != nil {
		return state, err
	}
	state.SourcePresent = present
	state.Source = source

	privateDirectory, present, err := openLegacyCheckpointUnixProtectedDirectoryAt(root, "private")
	if err != nil || !present {
		return state, err
	}
	defer unix.Close(privateDirectory)
	quarantine, present, err := openLegacyCheckpointUnixProtectedDirectoryAt(privateDirectory, LegacyCheckpointSnapshotQuarantineV1)
	if err != nil || !present {
		return state, err
	}
	defer unix.Close(quarantine)
	state.QuarantinePresent = true

	entries, err := readLegacyCheckpointUnixDirectoryNames(ctx, quarantine, 3)
	if err != nil {
		return state, err
	}
	for _, name := range entries {
		switch name {
		case LegacyCheckpointSnapshotAuditRecordsV1:
			directory, found, openErr := openLegacyCheckpointUnixProtectedDirectoryAt(quarantine, name)
			if openErr != nil || !found {
				return state, errors.New("legacy checkpoint quarantine audit namespace is unsafe")
			}
			state.AuditRecordsPresent = true
			_ = unix.Close(directory)
		case LegacyCheckpointSnapshotPayloadsV1:
			payloads, found, openErr := openLegacyCheckpointUnixProtectedDirectoryAt(quarantine, name)
			if openErr != nil || !found {
				return state, errors.New("legacy checkpoint quarantine payload namespace is unsafe")
			}
			state.PayloadsPresent = true
			payloadNames, inventoryErr := readLegacyCheckpointUnixDirectoryNames(ctx, payloads, 1)
			if inventoryErr != nil {
				_ = unix.Close(payloads)
				return state, inventoryErr
			}
			if len(payloadNames) == 1 {
				payloadName := payloadNames[0]
				if !ValidLegacyCheckpointSnapshotPayloadNameV1(payloadName) {
					_ = unix.Close(payloads)
					return state, errors.New("legacy checkpoint quarantine contains an unknown payload")
				}
				tree, found, inspectErr := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
					return inspectLegacyCheckpointUnixTreeAt(ctx, payloads, payloadName)
				})
				if inspectErr != nil || !found {
					_ = unix.Close(payloads)
					return state, errors.Join(errors.New("legacy checkpoint quarantine payload is unsafe"), inspectErr)
				}
				if LegacyCheckpointSnapshotPayloadDigestV1(payloadName) != tree.SHA256 {
					_ = unix.Close(payloads)
					return state, errors.New("legacy checkpoint quarantine payload name does not match its contents")
				}
				state.Payload = &LegacyCheckpointSnapshotPayloadV1{Name: payloadName, Tree: tree}
			}
			_ = unix.Close(payloads)
		default:
			return state, errors.New("legacy checkpoint quarantine contains an unknown entry")
		}
	}
	return state, nil
}

func platformMoveLegacyCheckpointSnapshotsToQuarantine(
	ctx context.Context,
	binding privatecasport.RootBinding,
	targetName string,
	expected LegacyCheckpointSnapshotTreeV1,
) (LegacyCheckpointSnapshotTreeV1, error) {
	root, err := openLegacyCheckpointUnixBoundRoot(binding)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	defer unix.Close(root)
	privateDirectory, present, err := openLegacyCheckpointUnixProtectedDirectoryAt(root, "private")
	if err != nil || !present {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint private quarantine parent is unavailable")
	}
	defer unix.Close(privateDirectory)
	quarantine, present, err := openLegacyCheckpointUnixProtectedDirectoryAt(privateDirectory, LegacyCheckpointSnapshotQuarantineV1)
	if err != nil || !present {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine namespace is unavailable")
	}
	defer unix.Close(quarantine)
	payloads, present, err := openLegacyCheckpointUnixProtectedDirectoryAt(quarantine, LegacyCheckpointSnapshotPayloadsV1)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if !present {
		if err := unix.Mkdirat(quarantine, LegacyCheckpointSnapshotPayloadsV1, 0o700); err != nil {
			if !errors.Is(err, unix.EEXIST) {
				return LegacyCheckpointSnapshotTreeV1{}, err
			}
		} else if err := unix.Fsync(quarantine); err != nil {
			return LegacyCheckpointSnapshotTreeV1{}, err
		}
		payloads, present, err = openLegacyCheckpointUnixProtectedDirectoryAt(quarantine, LegacyCheckpointSnapshotPayloadsV1)
		if err != nil || !present {
			return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint payload namespace creation was not durable")
		}
	}
	defer unix.Close(payloads)

	payloadNames, err := readLegacyCheckpointUnixDirectoryNames(ctx, payloads, 1)
	if err != nil || len(payloadNames) != 0 {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint quarantine target is not empty"), err)
	}
	if _, statErr := legacyCheckpointUnixStatAt(payloads, targetName); statErr == nil {
		return LegacyCheckpointSnapshotTreeV1{}, os.ErrExist
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return LegacyCheckpointSnapshotTreeV1{}, statErr
	}

	current, present, err := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
		return inspectLegacyCheckpointUnixTreeAt(ctx, root, LegacyCheckpointSnapshotDirectoryV1)
	})
	if err != nil || !present || !equalLegacyCheckpointTrees(current, expected) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint source changed before quarantine"), err)
	}
	source, err := unix.Openat(root, LegacyCheckpointSnapshotDirectoryV1, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	defer unix.Close(source)
	var sourceStat unix.Stat_t
	if err := unix.Fstat(source, &sourceStat); err != nil || unixDirectoryObjectIdentity(sourceStat) != expected.RootIdentity {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint source identity changed before quarantine")
	}
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if err := legacyCheckpointUnixRenameNoReplace(root, LegacyCheckpointSnapshotDirectoryV1, payloads, targetName); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if err := errors.Join(unix.Fsync(root), unix.Fsync(payloads)); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	destination, err := unix.Openat(payloads, targetName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	var destinationStat unix.Stat_t
	identityErr := unix.Fstat(destination, &destinationStat)
	_ = unix.Close(destination)
	if identityErr != nil || uint64(destinationStat.Dev) != uint64(sourceStat.Dev) || destinationStat.Ino != sourceStat.Ino {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine rename changed the source identity")
	}
	moved, present, err := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
		return inspectLegacyCheckpointUnixTreeAt(ctx, payloads, targetName)
	})
	if err != nil || !present || !equalLegacyCheckpointTrees(moved, expected) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint quarantine readback failed"), err)
	}
	return moved, nil
}

func inspectLegacyCheckpointUnixTreeAt(
	ctx context.Context,
	parent int,
	name string,
) (LegacyCheckpointSnapshotTreeV1, bool, error) {
	root, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return LegacyCheckpointSnapshotTreeV1{}, false, nil
	}
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, errors.New("legacy checkpoint snapshot root is unsafe")
	}
	defer unix.Close(root)
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o7777 != 0o700 {
		return LegacyCheckpointSnapshotTreeV1{}, false, errors.New("legacy checkpoint snapshot root is not a directory")
	}
	tree := LegacyCheckpointSnapshotTreeV1{RootIdentity: unixDirectoryObjectIdentity(stat)}
	if err := inspectLegacyCheckpointUnixDirectory(ctx, root, ".", &tree, 0); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, err
	}
	finalized, err := finalizeLegacyCheckpointTree(tree)
	return finalized, err == nil, err
}

func inspectLegacyCheckpointUnixDirectory(
	ctx context.Context,
	directory int,
	relative string,
	tree *LegacyCheckpointSnapshotTreeV1,
	depth int,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 2 || len(tree.Entries) >= maxLegacyCheckpointSnapshotEntriesV1 {
		return errors.New("legacy checkpoint snapshot directory depth or entry bound exceeded")
	}
	var before unix.Stat_t
	if err := unix.Fstat(directory, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFDIR || before.Mode&0o7777 != 0o700 {
		return errors.New("legacy checkpoint snapshot directory changed during inspection")
	}
	tree.Entries = append(tree.Entries, LegacyCheckpointSnapshotEntryV1{
		Path: relative, Type: "directory", Mode: legacyCheckpointUnixFileMode(before), ModTimeUnixNano: unixStatModTimeNano(before),
	})
	names, err := readLegacyCheckpointUnixDirectoryNames(ctx, directory, maxLegacyCheckpointSnapshotEntriesV1-len(tree.Entries))
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		childRelative := name
		if relative != "." {
			childRelative = relative + "/" + name
		}
		var childStat unix.Stat_t
		if err := unix.Fstatat(directory, name, &childStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return errors.New("legacy checkpoint snapshot entry could not be inspected")
		}
		switch childStat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			child, err := unix.Openat(directory, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return errors.New("legacy checkpoint snapshot directory crossed a link")
			}
			var opened unix.Stat_t
			if err := unix.Fstat(child, &opened); err != nil || uint64(opened.Dev) != uint64(childStat.Dev) || opened.Ino != childStat.Ino {
				_ = unix.Close(child)
				return errors.New("legacy checkpoint snapshot directory identity changed")
			}
			inspectErr := inspectLegacyCheckpointUnixDirectory(ctx, child, childRelative, tree, depth+1)
			closeErr := unix.Close(child)
			if inspectErr != nil || closeErr != nil {
				return errors.Join(inspectErr, closeErr)
			}
		case unix.S_IFREG:
			if depth != 2 || !validLegacyCheckpointRecordName(name) {
				return errors.New("legacy checkpoint snapshot contains an unknown file")
			}
			entry, err := inspectLegacyCheckpointUnixFile(directory, name, childRelative, childStat)
			if err != nil {
				return err
			}
			if tree.FileCount == maxLegacyCheckpointSnapshotFilesV1 || entry.Size > maxLegacyCheckpointSnapshotTotalBytesV1-tree.TotalBytes {
				return errors.New("legacy checkpoint snapshot byte or file bound exceeded")
			}
			tree.FileCount++
			tree.TotalBytes += entry.Size
			tree.Entries = append(tree.Entries, entry)
		default:
			return errors.New("legacy checkpoint snapshot contains a link or special entry")
		}
	}
	var after unix.Stat_t
	if err := unix.Fstat(directory, &after); err != nil || !equalLegacyCheckpointUnixStat(before, after) {
		return errors.New("legacy checkpoint snapshot directory changed during inspection")
	}
	return nil
}

func inspectLegacyCheckpointUnixFile(parent int, name, relative string, initial unix.Stat_t) (LegacyCheckpointSnapshotEntryV1, error) {
	if initial.Nlink != 1 || initial.Mode&0o7777 != 0o600 || initial.Size < 0 || initial.Size > maxLegacyCheckpointSnapshotFileBytesV1 {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file is unsafe or too large")
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file crossed a link")
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file handle is invalid")
	}
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || !equalLegacyCheckpointUnixStat(initial, before) || before.Mode&unix.S_IFMT != unix.S_IFREG {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file identity changed")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maxLegacyCheckpointSnapshotFileBytesV1+1))
	if err != nil || written != before.Size {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file read was incomplete")
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil || !equalLegacyCheckpointUnixStat(before, after) {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file changed during inspection")
	}
	info, err := file.Stat()
	if err != nil {
		return LegacyCheckpointSnapshotEntryV1{}, err
	}
	return LegacyCheckpointSnapshotEntryV1{
		Path: relative, Type: "file", Mode: uint32(info.Mode()), Size: before.Size,
		ModTimeUnixNano: info.ModTime().UnixNano(), SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func openLegacyCheckpointUnixBoundRoot(binding privatecasport.RootBinding) (int, error) {
	if binding.RootIdentity.Kind != privatecasport.DirectoryIdentityUnix {
		return -1, errors.New("legacy checkpoint Unix binding kind is invalid")
	}
	root, err := secureOpenAbsoluteDirectory(binding.RootPath, false)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(root, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
		uint64(stat.Dev) != binding.RootIdentity.Device || stat.Ino != binding.RootIdentity.Inode {
		_ = unix.Close(root)
		return -1, errors.New("legacy checkpoint persistence root identity changed")
	}
	return root, nil
}

func platformValidateColdRootAbsent(capability frozenRootCapability) error {
	if capability.RootIdentity != "" || capability.MissingSuffix == "" {
		return errors.New("legacy checkpoint cold root capability is invalid")
	}
	current, err := secureOpenAbsoluteDirectory(capability.Anchor, false)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(current) }()
	identity, err := unixOpenedDirectoryIdentity(current)
	if err != nil || identity != capability.AnchorIdentity {
		return errors.New("legacy checkpoint cold root ancestor identity changed")
	}
	parts, err := secureRelativeComponents(capability.MissingSuffix)
	if err != nil {
		return err
	}
	next, openErr := unix.Openat(current, parts[0], unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(openErr, unix.ENOENT) {
		return nil
	}
	if openErr != nil {
		return errors.New("legacy checkpoint cold root crossed an unsafe path component")
	}
	_ = unix.Close(next)
	return errors.New("legacy checkpoint cold root appeared after authority freeze")
}

func openLegacyCheckpointUnixProtectedDirectoryAt(parent int, name string) (int, bool, error) {
	if !startupAuthorityNamedComponent(name) {
		return -1, false, errors.New("legacy checkpoint protected directory name is invalid")
	}
	directory, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return -1, false, nil
	}
	if err != nil {
		return -1, false, errors.New("legacy checkpoint protected directory crossed a link")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(directory, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 {
		_ = unix.Close(directory)
		return -1, false, errors.New("legacy checkpoint protected directory permissions are unsafe")
	}
	return directory, true, nil
}

func readLegacyCheckpointUnixDirectoryNames(ctx context.Context, directory int, limit int) ([]string, error) {
	if limit < 0 || limit > maxLegacyCheckpointSnapshotEntriesV1 {
		return nil, errors.New("legacy checkpoint directory inventory bound is invalid")
	}
	duplicate, err := unix.Openat(directory, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "legacy-checkpoint-directory")
	if file == nil {
		_ = unix.Close(duplicate)
		return nil, errors.New("legacy checkpoint directory handle is invalid")
	}
	defer file.Close()
	names := make([]string, 0, min(limit, 256))
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := file.ReadDir(256)
		if len(names)+len(entries) > limit {
			return nil, errors.New("legacy checkpoint directory inventory bound exceeded")
		}
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Strings(names)
	return names, nil
}

func legacyCheckpointUnixStatAt(parent int, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return unix.Stat_t{}, os.ErrNotExist
	}
	return stat, err
}

func unixDirectoryObjectIdentity(stat unix.Stat_t) string {
	return formatUnixObjectIdentity(uint64(stat.Dev), stat.Ino)
}

func legacyCheckpointUnixFileMode(stat unix.Stat_t) uint32 {
	mode := os.FileMode(stat.Mode & 0o777)
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		mode |= os.ModeDir
	case unix.S_IFLNK:
		mode |= os.ModeSymlink
	}
	return uint32(mode)
}

func equalLegacyCheckpointUnixStat(left, right unix.Stat_t) bool {
	return uint64(left.Dev) == uint64(right.Dev) && left.Ino == right.Ino && left.Mode == right.Mode &&
		left.Nlink == right.Nlink && left.Size == right.Size && unixStatModTimeNano(left) == unixStatModTimeNano(right)
}
