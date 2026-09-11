package skill

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"path"
	"sort"
	"strings"
)

const (
	PackageSnapshotVersion = 1
	maxSnapshotFiles       = 256
	maxSnapshotFileBytes   = 8 * 1024 * 1024
	maxSnapshotTotalBytes  = 32 * 1024 * 1024
)

type FileInput struct {
	RelativePath string
	Bytes        []byte
}

type snapshotFile struct {
	bytes  []byte
	digest string
}

// PackageSnapshot is the immutable, content-addressed authority for one skill
// package. Filesystem paths and mtimes are deliberately excluded from its
// digest so discovery roots cannot become execution authority.
type PackageSnapshot struct {
	digest            string
	entryRelativePath string
	files             map[string]snapshotFile
}

func NormalizeRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("skill package path is empty")
	}
	if strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") {
		return "", errors.New("skill package path must be a canonical relative slash path")
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("skill package path must not traverse or contain non-canonical segments")
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("skill package path contains an invalid segment")
		}
	}
	return cleaned, nil
}

func NewPackageSnapshot(entryRelativePath string, inputs []FileInput) (PackageSnapshot, error) {
	entryRelativePath, err := NormalizeRelativePath(entryRelativePath)
	if err != nil {
		return PackageSnapshot{}, err
	}
	if len(inputs) == 0 || len(inputs) > maxSnapshotFiles {
		return PackageSnapshot{}, errors.New("skill package snapshot has an invalid file count")
	}
	files := make(map[string]snapshotFile, len(inputs))
	totalBytes := 0
	for _, input := range inputs {
		relativePath, pathErr := NormalizeRelativePath(input.RelativePath)
		if pathErr != nil {
			return PackageSnapshot{}, pathErr
		}
		if _, exists := files[relativePath]; exists {
			return PackageSnapshot{}, errors.New("skill package snapshot contains a duplicate path")
		}
		if len(input.Bytes) > maxSnapshotFileBytes || totalBytes > maxSnapshotTotalBytes-len(input.Bytes) {
			return PackageSnapshot{}, errors.New("skill package snapshot exceeds its byte limit")
		}
		totalBytes += len(input.Bytes)
		bytesCopy := append([]byte(nil), input.Bytes...)
		fileHash := sha256.Sum256(bytesCopy)
		files[relativePath] = snapshotFile{bytes: bytesCopy, digest: hex.EncodeToString(fileHash[:])}
	}
	if _, ok := files[entryRelativePath]; !ok {
		return PackageSnapshot{}, errors.New("skill package snapshot does not contain its entry")
	}

	paths := make([]string, 0, len(files))
	for relativePath := range files {
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)
	hash := sha256.New()
	hash.Write([]byte("analytix.skill-package-snapshot.v1\x00"))
	writeDigestField(hash, entryRelativePath)
	for _, relativePath := range paths {
		file := files[relativePath]
		writeDigestField(hash, relativePath)
		writeDigestField(hash, file.digest)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(file.bytes)))
		hash.Write(size[:])
	}
	return PackageSnapshot{
		digest:            hex.EncodeToString(hash.Sum(nil)),
		entryRelativePath: entryRelativePath,
		files:             files,
	}, nil
}

func writeDigestField(hash interface{ Write([]byte) (int, error) }, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	hash.Write(size[:])
	hash.Write([]byte(value))
}

func (snapshot PackageSnapshot) Valid() bool {
	return len(snapshot.digest) == sha256.Size*2 && snapshot.entryRelativePath != "" && len(snapshot.files) > 0
}

func (snapshot PackageSnapshot) Digest() string {
	return snapshot.digest
}

func (snapshot PackageSnapshot) EntryRelativePath() string {
	return snapshot.entryRelativePath
}

func (snapshot PackageSnapshot) File(relativePath string) ([]byte, bool) {
	relativePath, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return nil, false
	}
	file, ok := snapshot.files[relativePath]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), file.bytes...), true
}

func (snapshot PackageSnapshot) Paths() []string {
	paths := make([]string, 0, len(snapshot.files))
	for relativePath := range snapshot.files {
		paths = append(paths, relativePath)
	}
	sort.Strings(paths)
	return paths
}

func (snapshot PackageSnapshot) FileDigest(relativePath string) (string, bool) {
	relativePath, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return "", false
	}
	file, ok := snapshot.files[relativePath]
	return file.digest, ok
}
