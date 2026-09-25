//go:build windows

package secretstore

import (
	"bytes"
	"context"
	"errors"
	"os"
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
		NumberOfLinks:  1,
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

func TestWindowsDPAPIMasterKeySurvivesRestartAndRejectsMissingKey(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "private", "provider-secrets", "credentials.v1.json")
	provider, err := defaultMasterKeyProvider(storePath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := provider.LoadOrCreate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer clearBytes(first)
	secondProvider, err := defaultMasterKeyProvider(storePath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondProvider.LoadOrCreate(context.Background())
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("DPAPI restart readback failed: %v", err)
	}
	clearBytes(second)
	if err := writePrivateFileAtomically(storePath, []byte("synthetic-ciphertext"), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(provider.(*dpapiMasterKeyProvider).path); err != nil {
		t.Fatal(err)
	}
	if _, err := secondProvider.LoadOrCreate(context.Background()); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("missing master key regenerated over ciphertext: %v", err)
	}
	content, err := readPrivateCommittedFile(storePath, 1024)
	if err != nil || !bytes.Equal(content, []byte("synthetic-ciphertext")) {
		t.Fatalf("ciphertext was not preserved after missing key: %v", err)
	}
	clearBytes(content)
}

func TestWindowsPrivateReplacementRetainsOwnerDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "provider-secrets", "credentials.v1.json")
	for _, content := range [][]byte{[]byte("first"), []byte("second")} {
		if err := writePrivateFileAtomically(path, content, nil); err != nil {
			t.Fatal(err)
		}
		readback, err := readPrivateCommittedFile(path, 64)
		if err != nil || !bytes.Equal(readback, content) {
			t.Fatalf("private replacement readback failed: %v", err)
		}
		clearBytes(readback)
	}
}

func TestWindowsPrivateFileRejectsForeignWriterACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "provider-secrets", "credentials.v1.json")
	if err := writePrivateFileAtomically(path, []byte("synthetic-ciphertext"), nil); err != nil {
		t.Fatal(err)
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	user, err := token.GetTokenUser()
	_ = token.Close()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;WD)",
	)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.WRITE_DAC|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateCommittedFile(path, 1024); err == nil {
		t.Fatal("foreign Windows ACL writer grant was accepted")
	}
}
