//go:build windows

package persistencefs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type separateOwnerWindowsRootPin struct {
	handle     windows.Handle
	rootExists bool
}

func platformPinSeparateOwnerRoot(
	anchor string,
	rootExists bool,
	expected separateOwnerPathBindingV1,
) (separateOwnerRootPin, error) {
	handle, err := separateOwnerWindowsOpenAbsolutePinned(anchor)
	if err != nil {
		return nil, err
	}
	pin := &separateOwnerWindowsRootPin{handle: handle, rootExists: rootExists}
	if err := pin.Validate(expected); err != nil {
		_ = pin.Close()
		return nil, err
	}
	return pin, nil
}

func separateOwnerWindowsOpenAbsolutePinned(path string) (windows.Handle, error) {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) {
		return 0, errors.New("separate-owner Windows pinned path is invalid")
	}
	access := uint32(windows.FILE_LIST_DIRECTORY | windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.SYNCHRONIZE)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	volumeRoot := volume + string(os.PathSeparator)
	if err := separateOwnerWindowsValidateVolume(volumeRoot); err != nil {
		return 0, err
	}
	current, err := secureWindowsOpenAbsolute(volumeRoot, access, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	components := strings.FieldsFunc(remainder, func(char rune) bool { return char == '\\' || char == '/' })
	for index, component := range components {
		if !separateOwnerWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("separate-owner Windows pinned component is invalid")
		}
		componentShare := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
		if index == len(components)-1 {
			componentShare = share
		}
		next, openErr := secureWindowsOpenRelativeWithShare(
			current, component, access, windows.FILE_OPEN, true, componentShare,
		)
		if openErr == nil {
			openErr = separateOwnerWindowsVerifyOpenedComponent(next, component)
		}
		_ = windows.CloseHandle(current)
		if openErr != nil {
			if next != 0 {
				_ = windows.CloseHandle(next)
			}
			return 0, openErr
		}
		current = next
	}
	return current, nil
}

func (pin *separateOwnerWindowsRootPin) Validate(expected separateOwnerPathBindingV1) error {
	if pin == nil || pin.handle == 0 {
		return errors.New("separate-owner Windows pinned root is unavailable")
	}
	identity, err := windowsOpenedObjectIdentity(pin.handle, true)
	if err != nil || identity != expected.Identity {
		return errors.New("separate-owner Windows pinned root identity changed")
	}
	security, err := separateOwnerWindowsSecurityDigest(pin.handle, true, pin.rootExists, false)
	if err != nil || security != expected.SecurityDigest {
		return errors.New("separate-owner Windows pinned root permissions changed")
	}
	return nil
}

func (pin *separateOwnerWindowsRootPin) OpenRoot() (*startupPrivateDirectory, error) {
	if pin == nil || pin.handle == 0 || !pin.rootExists {
		return nil, errors.New("separate-owner Windows pinned root is unavailable")
	}
	handle, err := separateOwnerWindowsReopenRootForAccess(pin.handle)
	if err != nil {
		return nil, err
	}
	identity, err := windowsOpenedDirectoryIdentity(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &startupPrivateDirectory{handle: handle, identity: identity}, nil
}

func (pin *separateOwnerWindowsRootPin) Close() error {
	if pin == nil || pin.handle == 0 {
		return nil
	}
	handle := pin.handle
	pin.handle = 0
	return windows.CloseHandle(handle)
}
