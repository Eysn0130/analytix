//go:build windows

package providerregistryfs

import (
	"os"
	"path/filepath"

	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	"golang.org/x/sys/windows"
)

func openLegacySourceFile(path string) (*os.File, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	handle, err := windows.CreateFile(
		pathUTF16, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0,
	)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	fileType, typeErr := windows.GetFileType(handle)
	var information windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(handle, &information)
	invalidAttributes := uint32(windows.FILE_ATTRIBUTE_REPARSE_POINT | windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_DEVICE)
	if typeErr != nil || infoErr != nil || fileType != windows.FILE_TYPE_DISK ||
		information.FileAttributes&invalidAttributes != 0 || information.NumberOfLinks != 1 {
		_ = windows.CloseHandle(handle)
		return nil, registryport.ErrVerification
	}
	file := os.NewFile(uintptr(handle), filepath.Base(path))
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, registryport.ErrVerification
	}
	return file, nil
}
