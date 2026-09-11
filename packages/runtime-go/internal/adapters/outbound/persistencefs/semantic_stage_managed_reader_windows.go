//go:build windows

package persistencefs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"golang.org/x/sys/windows"
)

func readSemanticStageManagedFile(
	ctx context.Context,
	root *startupPrivateDirectory,
	relative string,
	expected domainstartup.SemanticEntryStateV1,
) (body []byte, resultErr error) {
	return readSemanticStageManagedFileWithHook(ctx, root, relative, expected, nil)
}

func readSemanticStageManagedFileWithHook(
	ctx context.Context,
	root *startupPrivateDirectory,
	relative string,
	expected domainstartup.SemanticEntryStateV1,
	hook func(string) error,
) (body []byte, resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == nil || root.handle == 0 || expected.Type != domainstartup.ManagedEntryTypeFile ||
		expected.Mode == 0 || expected.Size < 0 || expected.Size > domainstartup.MaxSemanticManagedFileBytesV1 ||
		!domainsecurity.IsSHA256Hex(expected.SHA256) {
		return nil, errors.New("semantic stage managed file authority is invalid")
	}
	parts, err := secureWindowsRelativeComponents(relative)
	if err != nil {
		return nil, err
	}
	directories := []windows.Handle{root.handle}
	directoryNames := make([]string, 0, len(parts)-1)
	defer func() {
		for index := len(directories) - 1; index > 0; index-- {
			resultErr = errors.Join(resultErr, windows.CloseHandle(directories[index]))
		}
		if resultErr != nil {
			clear(body)
			body = nil
		}
	}()
	if identity, err := windowsOpenedDirectoryIdentity(root.handle); err != nil || identity != root.Identity() {
		return nil, errors.New("semantic stage managed root identity changed")
	}
	for _, component := range parts[:len(parts)-1] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		next, err := secureWindowsOpenRelative(
			directories[len(directories)-1], component, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true,
		)
		if err != nil {
			return nil, err
		}
		directories = append(directories, next)
		directoryNames = append(directoryNames, component)
	}
	parent := directories[len(directories)-1]
	name := parts[len(parts)-1]
	handle, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("semantic stage managed Windows file handle is invalid")
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	initialInfo, err := file.Stat()
	var initial windows.ByHandleFileInformation
	initialIdentity, identityErr := windowsOpenedObjectIdentity(handle, false)
	if err != nil || windows.GetFileInformationByHandle(handle, &initial) != nil || identityErr != nil ||
		initial.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 ||
		initial.NumberOfLinks != 1 || !initialInfo.Mode().IsRegular() ||
		uint32(initialInfo.Mode()) != expected.Mode || initialInfo.Size() != expected.Size {
		return nil, errors.New("semantic stage managed Windows file state is invalid")
	}
	capacity := expected.Size
	if capacity > 1<<20 {
		capacity = 1 << 20
	}
	body = make([]byte, 0, int(capacity))
	hasher := sha256.New()
	buffer := make([]byte, 1<<20)
	readBytes := int64(0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			readBytes += int64(count)
			if readBytes > expected.Size || readBytes > domainstartup.MaxSemanticManagedFileBytesV1 {
				return nil, errors.New("semantic stage managed Windows file exceeded its bound")
			}
			_, _ = hasher.Write(buffer[:count])
			body = append(body, buffer[:count]...)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	clear(buffer)
	if readBytes != expected.Size || hex.EncodeToString(hasher.Sum(nil)) != expected.SHA256 {
		return nil, errors.New("semantic stage managed Windows file content changed")
	}
	if hook != nil {
		if err := hook("before_final_name_check"); err != nil {
			return nil, err
		}
	}
	refreshedInfo, statErr := file.Stat()
	var refreshed windows.ByHandleFileInformation
	refreshedIdentity, refreshedIdentityErr := windowsOpenedObjectIdentity(handle, false)
	if statErr != nil || windows.GetFileInformationByHandle(handle, &refreshed) != nil || refreshedIdentityErr != nil ||
		!os.SameFile(initialInfo, refreshedInfo) || refreshedIdentity != initialIdentity ||
		refreshed.FileAttributes != initial.FileAttributes || refreshed.NumberOfLinks != 1 ||
		refreshed.FileSizeHigh != initial.FileSizeHigh || refreshed.FileSizeLow != initial.FileSizeLow ||
		uint32(refreshedInfo.Mode()) != expected.Mode {
		return nil, errors.New("semantic stage managed Windows file identity changed while reading")
	}
	if err := semanticStageWindowsVerifyNameMapping(parent, name, initialIdentity, false); err != nil {
		return nil, err
	}
	for index, component := range directoryNames {
		identity, err := windowsOpenedDirectoryIdentity(directories[index+1])
		if err != nil || semanticStageWindowsVerifyNameMapping(directories[index], component, identity, true) != nil {
			return nil, errors.New("semantic stage managed Windows directory name mapping changed")
		}
	}
	if identity, err := windowsOpenedDirectoryIdentity(root.handle); err != nil || identity != root.Identity() || ctx.Err() != nil {
		return nil, errors.Join(errors.New("semantic stage managed root identity changed"), err, ctx.Err())
	}
	return body, nil
}

func semanticStageWindowsVerifyNameMapping(
	parent windows.Handle,
	name string,
	expectedIdentity string,
	directory bool,
) error {
	handle, err := secureWindowsOpenRelative(parent, name, windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, windows.FILE_OPEN, directory)
	if err != nil {
		return err
	}
	identity, identityErr := windowsOpenedObjectIdentity(handle, directory)
	closeErr := windows.CloseHandle(handle)
	if identityErr != nil || identity != expectedIdentity || closeErr != nil {
		return errors.Join(errors.New("semantic stage managed Windows name mapping changed"), identityErr, closeErr)
	}
	return nil
}
