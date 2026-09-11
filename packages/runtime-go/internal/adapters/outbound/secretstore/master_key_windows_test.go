//go:build windows

package secretstore

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"

	"golang.org/x/sys/windows"
)

func TestDPAPIMasterKeyBlobIsStrictAndVersioned(t *testing.T) {
	t.Parallel()

	protected := bytes.Repeat([]byte{0x45}, 96)
	blob := encodeDPAPIBlob(protected)
	decoded, err := decodeDPAPIBlob(blob)
	if err != nil {
		t.Fatalf("decodeDPAPIBlob() error = %v", err)
	}
	defer clearBytes(decoded)
	if !bytes.Equal(decoded, protected) {
		t.Fatal("decoded DPAPI blob does not match protected bytes")
	}

	for _, mutate := range []func([]byte) []byte{
		func(value []byte) []byte { return value[:dpapiHeaderSize-1] },
		func(value []byte) []byte {
			value[0] ^= 0xff
			return value
		},
		func(value []byte) []byte {
			value[4]++
			return value
		},
		func(value []byte) []byte {
			value[8]++
			return value
		},
	} {
		candidate := bytes.Clone(blob)
		candidate = mutate(candidate)
		decoded, err := decodeDPAPIBlob(candidate)
		clearBytes(decoded)
		clearBytes(candidate)
		if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
			t.Fatalf("decodeDPAPIBlob(invalid) error = %v, want unavailable", err)
		}
	}
}

func TestWindowsDefaultMasterKeyProviderUsesDPAPIFile(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(t.TempDir(), "credentials.enc.json")
	provider, err := defaultMasterKeyProvider(storePath, Options{})
	if err != nil {
		t.Fatalf("defaultMasterKeyProvider() error = %v", err)
	}
	dpapi, ok := provider.(*dpapiMasterKeyProvider)
	if !ok {
		t.Fatalf("provider type = %T, want DPAPI", provider)
	}
	want := filepath.Join(filepath.Dir(storePath), "master-key", "master-key.dpapi")
	if dpapi.path != want {
		t.Fatal("DPAPI provider path is not data-directory scoped")
	}
}

func TestWindowsPrivateFileHandleValidationRejectsReparseAndNonRegularTargets(t *testing.T) {
	t.Parallel()

	valid := windows.ByHandleFileInformation{
		FileAttributes: windows.FILE_ATTRIBUTE_NORMAL,
		FileSizeLow:    32,
	}
	if err := validateWindowsPrivateFileHandle(windows.FILE_TYPE_DISK, valid, 32); err != nil {
		t.Fatalf("validateWindowsPrivateFileHandle(valid) error = %v", err)
	}
	for _, testCase := range []struct {
		name        string
		fileType    uint32
		information windows.ByHandleFileInformation
		maximum     int64
	}{
		{
			name:     "reparse point",
			fileType: windows.FILE_TYPE_DISK,
			information: windows.ByHandleFileInformation{
				FileAttributes: windows.FILE_ATTRIBUTE_REPARSE_POINT,
			},
			maximum: 32,
		},
		{
			name:     "directory",
			fileType: windows.FILE_TYPE_DISK,
			information: windows.ByHandleFileInformation{
				FileAttributes: windows.FILE_ATTRIBUTE_DIRECTORY,
			},
			maximum: 32,
		},
		{
			name:        "pipe",
			fileType:    windows.FILE_TYPE_PIPE,
			information: valid,
			maximum:     32,
		},
		{
			name:     "oversized",
			fileType: windows.FILE_TYPE_DISK,
			information: windows.ByHandleFileInformation{
				FileAttributes: windows.FILE_ATTRIBUTE_NORMAL,
				FileSizeHigh:   1,
			},
			maximum: 1 << 20,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := validateWindowsPrivateFileHandle(testCase.fileType, testCase.information, testCase.maximum); err == nil {
				t.Fatal("validateWindowsPrivateFileHandle() error = nil, want rejection")
			}
		})
	}
}
