//go:build windows

package providerregistryfs

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsRegistryLockAndDirectorySyncAccess(t *testing.T) {
	if registryLockShareMode&windows.FILE_SHARE_DELETE != 0 {
		t.Fatal("registry lock permits pathname deletion while held")
	}
	if registryLockShareMode != windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE {
		t.Fatalf("registry lock share mode = %d", registryLockShareMode)
	}
	if registryFileReadAccess != windows.GENERIC_READ {
		t.Fatalf("registry file read access = %d", registryFileReadAccess)
	}
	if registryDirectorySyncAccess&windows.GENERIC_WRITE == 0 {
		t.Fatal("directory sync handle lacks GENERIC_WRITE")
	}
}

func TestWindowsRegistryFileValidationRejectsReparseAndUnsafeTypes(t *testing.T) {
	t.Parallel()

	valid := windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_NORMAL, FileSizeLow: 32}
	if err := validateWindowsRegistryFile(windows.FILE_TYPE_DISK, valid, 32); err != nil {
		t.Fatalf("validateWindowsRegistryFile(valid) error = %v", err)
	}
	for _, testCase := range []struct {
		name     string
		fileType uint32
		info     windows.ByHandleFileInformation
		maximum  int
	}{
		{name: "reparse", fileType: windows.FILE_TYPE_DISK, info: windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_REPARSE_POINT}, maximum: 32},
		{name: "directory", fileType: windows.FILE_TYPE_DISK, info: windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_DIRECTORY}, maximum: 32},
		{name: "device", fileType: windows.FILE_TYPE_DISK, info: windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_DEVICE}, maximum: 32},
		{name: "pipe", fileType: windows.FILE_TYPE_PIPE, info: valid, maximum: 32},
		{name: "oversized", fileType: windows.FILE_TYPE_DISK, info: windows.ByHandleFileInformation{FileAttributes: windows.FILE_ATTRIBUTE_NORMAL, FileSizeHigh: 1}, maximum: 32},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := validateWindowsRegistryFile(testCase.fileType, testCase.info, testCase.maximum); err == nil {
				t.Fatal("validateWindowsRegistryFile() error = nil, want rejection")
			}
		})
	}
}
