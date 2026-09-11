//go:build windows

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
	"golang.org/x/sys/windows"
)

func platformInspectLegacyCheckpointSnapshotQuarantine(
	ctx context.Context,
	binding privatecasport.RootBinding,
) (LegacyCheckpointSnapshotQuarantineStateV1, error) {
	root, err := openLegacyCheckpointWindowsBoundRoot(binding)
	if err != nil {
		return LegacyCheckpointSnapshotQuarantineStateV1{}, err
	}
	defer windows.CloseHandle(root)
	state := LegacyCheckpointSnapshotQuarantineStateV1{}
	source, present, err := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
		return inspectLegacyCheckpointWindowsTreeAt(ctx, root, LegacyCheckpointSnapshotDirectoryV1)
	})
	if err != nil {
		return state, err
	}
	state.SourcePresent, state.Source = present, source
	privateDirectory, present, err := openLegacyCheckpointWindowsDirectoryAt(root, "private")
	if err != nil || !present {
		return state, err
	}
	defer windows.CloseHandle(privateDirectory)
	quarantine, present, err := openLegacyCheckpointWindowsDirectoryAt(privateDirectory, LegacyCheckpointSnapshotQuarantineV1)
	if err != nil || !present {
		return state, err
	}
	defer windows.CloseHandle(quarantine)
	state.QuarantinePresent = true
	entries, err := readLegacyCheckpointWindowsDirectoryNames(ctx, quarantine, 3)
	if err != nil {
		return state, err
	}
	for _, name := range entries {
		switch name {
		case LegacyCheckpointSnapshotAuditRecordsV1:
			directory, found, openErr := openLegacyCheckpointWindowsDirectoryAt(quarantine, name)
			if openErr != nil || !found {
				return state, errors.New("legacy checkpoint quarantine audit namespace is unsafe")
			}
			state.AuditRecordsPresent = true
			_ = windows.CloseHandle(directory)
		case LegacyCheckpointSnapshotPayloadsV1:
			payloads, found, openErr := openLegacyCheckpointWindowsDirectoryAt(quarantine, name)
			if openErr != nil || !found {
				return state, errors.New("legacy checkpoint quarantine payload namespace is unsafe")
			}
			state.PayloadsPresent = true
			payloadNames, inventoryErr := readLegacyCheckpointWindowsDirectoryNames(ctx, payloads, 1)
			if inventoryErr != nil {
				_ = windows.CloseHandle(payloads)
				return state, inventoryErr
			}
			if len(payloadNames) == 1 {
				payloadName := payloadNames[0]
				if !ValidLegacyCheckpointSnapshotPayloadNameV1(payloadName) {
					_ = windows.CloseHandle(payloads)
					return state, errors.New("legacy checkpoint quarantine contains an unknown payload")
				}
				tree, found, inspectErr := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
					return inspectLegacyCheckpointWindowsTreeAt(ctx, payloads, payloadName)
				})
				if inspectErr != nil || !found {
					_ = windows.CloseHandle(payloads)
					return state, errors.Join(errors.New("legacy checkpoint quarantine payload is unsafe"), inspectErr)
				}
				if LegacyCheckpointSnapshotPayloadDigestV1(payloadName) != tree.SHA256 {
					_ = windows.CloseHandle(payloads)
					return state, errors.New("legacy checkpoint quarantine payload name does not match its contents")
				}
				state.Payload = &LegacyCheckpointSnapshotPayloadV1{Name: payloadName, Tree: tree}
			}
			_ = windows.CloseHandle(payloads)
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
	root, err := openLegacyCheckpointWindowsBoundRoot(binding)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	defer windows.CloseHandle(root)
	privateDirectory, present, err := openLegacyCheckpointWindowsDirectoryAt(root, "private")
	if err != nil || !present {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint private quarantine parent is unavailable")
	}
	defer windows.CloseHandle(privateDirectory)
	quarantine, present, err := openLegacyCheckpointWindowsDirectoryAt(privateDirectory, LegacyCheckpointSnapshotQuarantineV1)
	if err != nil || !present {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine namespace is unavailable")
	}
	defer windows.CloseHandle(quarantine)
	payloads, present, err := openLegacyCheckpointWindowsDirectoryAt(quarantine, LegacyCheckpointSnapshotPayloadsV1)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if !present {
		created, createErr := secureStartupCreateDirectoryAt(quarantine, LegacyCheckpointSnapshotPayloadsV1)
		if createErr != nil && !errors.Is(createErr, os.ErrExist) {
			return LegacyCheckpointSnapshotTreeV1{}, createErr
		}
		if created != nil {
			payloads = created.handle
			created.handle = 0
			present = true
		} else {
			payloads, present, err = openLegacyCheckpointWindowsDirectoryAt(quarantine, LegacyCheckpointSnapshotPayloadsV1)
			if err != nil || !present {
				return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint payload namespace creation was not durable")
			}
		}
	}
	defer windows.CloseHandle(payloads)
	payloadNames, err := readLegacyCheckpointWindowsDirectoryNames(ctx, payloads, 1)
	if err != nil || len(payloadNames) != 0 {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint quarantine target is not empty"), err)
	}
	current, present, err := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
		return inspectLegacyCheckpointWindowsTreeAt(ctx, root, LegacyCheckpointSnapshotDirectoryV1)
	})
	if err != nil || !present || !equalLegacyCheckpointTrees(current, expected) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint source changed before quarantine"), err)
	}
	source, err := secureWindowsOpenRelative(
		root, LegacyCheckpointSnapshotDirectoryV1,
		windows.FILE_GENERIC_READ|windows.DELETE, windows.FILE_OPEN, true,
	)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	defer windows.CloseHandle(source)
	sourceIdentity, err := windowsOpenedDirectoryFileID(source)
	if err != nil || formatWindowsObjectIdentity(sourceIdentity.VolumeSerialNumber, sourceIdentity.FileID) != expected.RootIdentity {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint source identity changed before quarantine")
	}
	if err := ctx.Err(); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if err := startupWindowsRenameNoReplace(source, payloads, targetName); err != nil {
		if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
			return LegacyCheckpointSnapshotTreeV1{}, os.ErrExist
		}
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	if err := errors.Join(secureWindowsSyncDirectory(root), secureWindowsSyncDirectory(payloads)); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	destination, err := secureWindowsOpenRelative(payloads, targetName, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, err
	}
	destinationIdentity, identityErr := windowsOpenedDirectoryFileID(destination)
	_ = windows.CloseHandle(destination)
	if identityErr != nil || destinationIdentity != sourceIdentity {
		return LegacyCheckpointSnapshotTreeV1{}, errors.New("legacy checkpoint quarantine rename changed the source identity")
	}
	moved, present, err := stableLegacyCheckpointTree(ctx, func() (LegacyCheckpointSnapshotTreeV1, bool, error) {
		return inspectLegacyCheckpointWindowsTreeAt(ctx, payloads, targetName)
	})
	if err != nil || !present || !equalLegacyCheckpointTrees(moved, expected) {
		return LegacyCheckpointSnapshotTreeV1{}, errors.Join(errors.New("legacy checkpoint quarantine readback failed"), err)
	}
	return moved, nil
}

func inspectLegacyCheckpointWindowsTreeAt(
	ctx context.Context,
	parent windows.Handle,
	name string,
) (LegacyCheckpointSnapshotTreeV1, bool, error) {
	root, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if secureWindowsNotFound(err) {
		return LegacyCheckpointSnapshotTreeV1{}, false, nil
	}
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, errors.New("legacy checkpoint snapshot root is unsafe or reparse-backed")
	}
	defer windows.CloseHandle(root)
	identity, err := windowsOpenedDirectoryFileID(root)
	if err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, err
	}
	tree := LegacyCheckpointSnapshotTreeV1{RootIdentity: formatWindowsObjectIdentity(identity.VolumeSerialNumber, identity.FileID)}
	if err := inspectLegacyCheckpointWindowsDirectory(ctx, root, ".", &tree, 0); err != nil {
		return LegacyCheckpointSnapshotTreeV1{}, false, err
	}
	finalized, err := finalizeLegacyCheckpointTree(tree)
	return finalized, err == nil, err
}

func inspectLegacyCheckpointWindowsDirectory(
	ctx context.Context,
	directory windows.Handle,
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
	before, identity, err := legacyCheckpointWindowsHandleInfo(directory, true)
	if err != nil {
		return err
	}
	tree.Entries = append(tree.Entries, LegacyCheckpointSnapshotEntryV1{
		Path: relative, Type: "directory", Mode: legacyCheckpointWindowsMode(before, true),
		PlatformFlags: before.FileAttributes, ModTimeUnixNano: before.LastWriteTime.Nanoseconds(),
	})
	names, err := readLegacyCheckpointWindowsDirectoryNames(ctx, directory, maxLegacyCheckpointSnapshotEntriesV1-len(tree.Entries))
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
		if depth < 2 {
			if !safeLegacyCheckpointID(name) {
				return errors.New("legacy checkpoint snapshot contains an unknown directory")
			}
			child, err := secureWindowsOpenRelative(directory, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
			if err != nil {
				return errors.New("legacy checkpoint snapshot directory is unsafe or reparse-backed")
			}
			inspectErr := inspectLegacyCheckpointWindowsDirectory(ctx, child, childRelative, tree, depth+1)
			closeErr := windows.CloseHandle(child)
			if inspectErr != nil || closeErr != nil {
				return errors.Join(inspectErr, closeErr)
			}
			continue
		}
		if !validLegacyCheckpointRecordName(name) {
			return errors.New("legacy checkpoint snapshot contains an unknown file")
		}
		entry, err := inspectLegacyCheckpointWindowsFile(directory, name, childRelative)
		if err != nil {
			return err
		}
		if tree.FileCount == maxLegacyCheckpointSnapshotFilesV1 || entry.Size > maxLegacyCheckpointSnapshotTotalBytesV1-tree.TotalBytes {
			return errors.New("legacy checkpoint snapshot byte or file bound exceeded")
		}
		tree.FileCount++
		tree.TotalBytes += entry.Size
		tree.Entries = append(tree.Entries, entry)
	}
	after, afterIdentity, err := legacyCheckpointWindowsHandleInfo(directory, true)
	if err != nil || before != after || identity != afterIdentity {
		return errors.New("legacy checkpoint snapshot directory changed during inspection")
	}
	return nil
}

func inspectLegacyCheckpointWindowsFile(parent windows.Handle, name, relative string) (LegacyCheckpointSnapshotEntryV1, error) {
	handle, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file is unsafe or reparse-backed")
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file handle is invalid")
	}
	defer file.Close()
	before, identity, err := legacyCheckpointWindowsHandleInfo(handle, false)
	if err != nil || before.NumberOfLinks != 1 {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file link count is unsafe")
	}
	size := int64(uint64(before.FileSizeHigh)<<32 | uint64(before.FileSizeLow))
	if size < 0 || size > maxLegacyCheckpointSnapshotFileBytesV1 {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file is too large")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maxLegacyCheckpointSnapshotFileBytesV1+1))
	if err != nil || written != size {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file read was incomplete")
	}
	after, afterIdentity, err := legacyCheckpointWindowsHandleInfo(handle, false)
	if err != nil || before != after || identity != afterIdentity {
		return LegacyCheckpointSnapshotEntryV1{}, errors.New("legacy checkpoint snapshot file changed during inspection")
	}
	return LegacyCheckpointSnapshotEntryV1{
		Path: relative, Type: "file", Mode: legacyCheckpointWindowsMode(before, false), PlatformFlags: before.FileAttributes,
		Size: size, ModTimeUnixNano: before.LastWriteTime.Nanoseconds(), SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func openLegacyCheckpointWindowsBoundRoot(binding privatecasport.RootBinding) (windows.Handle, error) {
	if binding.RootIdentity.Kind != privatecasport.DirectoryIdentityWindows {
		return 0, errors.New("legacy checkpoint Windows binding kind is invalid")
	}
	root, err := secureWindowsOpenAbsoluteDirectory(binding.RootPath, false)
	if err != nil {
		return 0, err
	}
	identity, err := windowsOpenedDirectoryFileID(root)
	if err != nil || identity.VolumeSerialNumber != binding.RootIdentity.VolumeSerial || identity.FileID != binding.RootIdentity.FileID {
		_ = windows.CloseHandle(root)
		return 0, errors.New("legacy checkpoint persistence root identity changed")
	}
	return root, nil
}

func platformValidateColdRootAbsent(capability frozenRootCapability) error {
	if capability.RootIdentity != "" || capability.MissingSuffix == "" {
		return errors.New("legacy checkpoint cold Windows root capability is invalid")
	}
	current, err := secureWindowsOpenAbsoluteDirectory(capability.Anchor, false)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(current) }()
	identity, err := windowsOpenedDirectoryIdentity(current)
	if err != nil || identity != capability.AnchorIdentity {
		return errors.New("legacy checkpoint cold Windows root ancestor identity changed")
	}
	parts, err := secureWindowsRelativeComponents(capability.MissingSuffix)
	if err != nil {
		return err
	}
	next, openErr := secureWindowsOpenRelative(current, parts[0], windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if secureWindowsNotFound(openErr) {
		return nil
	}
	if openErr != nil {
		return errors.New("legacy checkpoint cold Windows root crossed an unsafe path component")
	}
	_ = windows.CloseHandle(next)
	return errors.New("legacy checkpoint cold Windows root appeared after authority freeze")
}

func openLegacyCheckpointWindowsDirectoryAt(parent windows.Handle, name string) (windows.Handle, bool, error) {
	directory, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, true)
	if secureWindowsNotFound(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, errors.New("legacy checkpoint protected directory is unsafe or reparse-backed")
	}
	return directory, true, nil
}

func readLegacyCheckpointWindowsDirectoryNames(ctx context.Context, directory windows.Handle, limit int) ([]string, error) {
	if limit < 0 || limit > maxLegacyCheckpointSnapshotEntriesV1 {
		return nil, errors.New("legacy checkpoint directory inventory bound is invalid")
	}
	duplicate, err := secureWindowsReopenDirectory(directory)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(duplicate), "legacy-checkpoint-directory")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
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

func legacyCheckpointWindowsHandleInfo(handle windows.Handle, directory bool) (windows.ByHandleFileInformation, privateCASWindowsFileIDInfo, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		return windows.ByHandleFileInformation{}, privateCASWindowsFileIDInfo{}, errors.New("legacy checkpoint Windows object is unsafe")
	}
	identity, err := windowsOpenedObjectFileID(handle, directory)
	return info, identity, err
}

func legacyCheckpointWindowsMode(info windows.ByHandleFileInformation, directory bool) uint32 {
	mode := os.FileMode(0o600)
	if directory {
		mode = os.ModeDir | 0o700
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		mode &^= 0o222
	}
	return uint32(mode)
}
