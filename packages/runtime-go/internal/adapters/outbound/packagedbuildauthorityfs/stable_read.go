package packagedbuildauthorityfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

type stableReadSnapshotV2 struct {
	body []byte
	info os.FileInfo
}

func stableReadV2(path string, maxBytes int64, allowEmpty bool) ([]byte, error) {
	snapshot, err := stableReadSnapshotFileV2(path, maxBytes, allowEmpty)
	return snapshot.body, err
}

func stableReadSnapshotFileV2(path string, maxBytes int64, allowEmpty bool) (stableReadSnapshotV2, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() < 0 ||
		(!allowEmpty && before.Size() == 0) || before.Size() > maxBytes {
		return stableReadSnapshotV2{}, errors.New("packaged authority file is not a bounded regular file")
	}
	file, err := openNoFollowV2(path)
	if err != nil {
		return stableReadSnapshotV2{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, opened) {
		return stableReadSnapshotV2{}, errors.New("packaged authority file changed before opening")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(body)) != opened.Size() {
		return stableReadSnapshotV2{}, errors.New("packaged authority file changed while reading")
	}
	afterDescriptor, descriptorErr := file.Stat()
	afterPath, pathErr := os.Lstat(path)
	if descriptorErr != nil || pathErr != nil || !afterPath.Mode().IsRegular() || afterPath.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(opened, afterDescriptor) || !os.SameFile(opened, afterPath) || afterDescriptor.Size() != opened.Size() {
		return stableReadSnapshotV2{}, errors.New("packaged authority file changed after reading")
	}
	return stableReadSnapshotV2{body: body, info: afterDescriptor}, nil
}

func stableContentIdentityV2(path string, maxBytes int64) (ContentIdentityV2, error) {
	snapshot, err := stableContentSnapshotV2(path, maxBytes)
	return snapshot.identity, err
}

type stableContentIdentitySnapshotV2 struct {
	identity ContentIdentityV2
	info     os.FileInfo
}

func stableContentSnapshotV2(path string, maxBytes int64) (stableContentIdentitySnapshotV2, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		before.Size() <= 0 || before.Size() > maxBytes {
		return stableContentIdentitySnapshotV2{}, errors.New("packaged content artifact is not a bounded regular file")
	}
	file, err := openNoFollowV2(path)
	if err != nil {
		return stableContentIdentitySnapshotV2{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(before, opened) || opened.Size() != before.Size() {
		return stableContentIdentitySnapshotV2{}, errors.New("packaged content artifact changed before opening")
	}
	digest := sha256.New()
	copied, err := io.Copy(digest, io.LimitReader(file, maxBytes+1))
	if err != nil || copied != opened.Size() || copied > maxBytes {
		return stableContentIdentitySnapshotV2{}, errors.New("packaged content artifact changed while reading")
	}
	afterDescriptor, descriptorErr := file.Stat()
	afterPath, pathErr := os.Lstat(path)
	if descriptorErr != nil || pathErr != nil || !afterPath.Mode().IsRegular() ||
		afterPath.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, afterDescriptor) ||
		!os.SameFile(opened, afterPath) || afterDescriptor.Size() != opened.Size() {
		return stableContentIdentitySnapshotV2{}, errors.New("packaged content artifact changed after reading")
	}
	return stableContentIdentitySnapshotV2{
		identity: ContentIdentityV2{
			SHA256: hex.EncodeToString(digest.Sum(nil)), ByteLength: copied,
		},
		info: afterDescriptor,
	}, nil
}
