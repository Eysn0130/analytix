//go:build darwin || linux

package finalauthority

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPrivateCASUnixRecoveryFingerprintBodySHA256EncodingIsCompatible(t *testing.T) {
	body := []byte("private-cas-fingerprint")
	identity := unix.Stat_t{
		Dev: 3, Ino: 5, Mode: unix.S_IFREG | 0o600, Nlink: 1,
		Uid: 7, Gid: 11, Size: int64(len(body)),
		Mtim: unix.Timespec{Sec: 13, Nsec: 17},
		Ctim: unix.Timespec{Sec: 19, Nsec: 23},
	}
	fromBody := sha256.New()
	privateCASUnixFingerprintRecoveryObject(fromBody, "record:aa", identity, body)
	fromDigest := sha256.New()
	privateCASUnixFingerprintRecoveryObjectBodySHA256(
		fromDigest, "record:aa", identity, sha256.Sum256(body),
	)
	if !bytes.Equal(fromBody.Sum(nil), fromDigest.Sum(nil)) {
		t.Fatal("Unix recovery fingerprint digest encoder changed the v1 byte protocol")
	}
}
