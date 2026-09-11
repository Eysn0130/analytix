//go:build windows

package evidenceregistry

import (
	"errors"
	"os"
	"syscall"
)

func registryRegularFileLinkCount(file *os.File, _ os.FileInfo) (uint64, error) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		return 0, err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return 0, errors.New("evidence registry file is a reparse point")
	}
	return uint64(info.NumberOfLinks), nil
}
