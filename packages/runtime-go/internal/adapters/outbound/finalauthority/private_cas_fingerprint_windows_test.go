//go:build windows

package finalauthority

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateCASWindowsRecoveryFingerprintBodySHA256EncodingIsCompatible(t *testing.T) {
	body := []byte("private-cas-fingerprint")
	identity := privateWindowsObjectIdentity{
		id: privateWindowsFileIDInfo{
			VolumeSerialNumber: 3,
			FileID:             [16]byte{5, 7, 11, 13},
		},
		attributes:    windows.FILE_ATTRIBUTE_NORMAL,
		links:         1,
		sizeLow:       uint32(len(body)),
		creationTime:  windows.Filetime{LowDateTime: 17, HighDateTime: 19},
		lastWriteTime: windows.Filetime{LowDateTime: 23, HighDateTime: 29},
	}
	fromBody := sha256.New()
	privateCASWindowsFingerprintRecord(fromBody, "record:aa", body, identity)
	fromDigest := sha256.New()
	privateCASWindowsFingerprintRecordBodySHA256(
		fromDigest, "record:aa", sha256.Sum256(body), identity,
	)
	if !bytes.Equal(fromBody.Sum(nil), fromDigest.Sum(nil)) {
		t.Fatal("Windows recovery fingerprint digest encoder changed the v1 byte protocol")
	}
}
