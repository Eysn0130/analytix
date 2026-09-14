// Package officeengineassets verifies the fixed local experiment dependencies.
// A matching binary is not a license, packaged admission, or GUI acceptance.
package officeengineassets

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

//go:embed manifest.json
var manifestBytes []byte

type asset struct {
	Name, Path, SHA256, MIME string
	Bytes                    int64
}
type manifest struct{ Assets []asset }
type observation struct {
	path string
	info os.FileInfo
}
type Assets struct{ entries []observation }

var ErrUnavailable = errors.New("office_engine_assets_unavailable")

func Open(ctx context.Context, root string) (*Assets, error) {
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, ErrUnavailable
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || canonical != root {
		return nil, ErrUnavailable
	}
	var fixed manifest
	if json.Unmarshal(manifestBytes, &fixed) != nil || len(fixed.Assets) != 6 {
		return nil, ErrUnavailable
	}
	result := &Assets{}
	for _, expected := range fixed.Assets {
		if ctx.Err() != nil {
			return nil, ErrUnavailable
		}
		path := filepath.Join(root, filepath.FromSlash(expected.Path))
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil || canonical != path {
			return nil, ErrUnavailable
		}
		before, err := os.Lstat(path)
		if err != nil || !before.Mode().IsRegular() || before.Size() != expected.Bytes {
			return nil, ErrUnavailable
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, ErrUnavailable
		}
		held, statErr := file.Stat()
		hash := sha256.New()
		count, readErr := io.Copy(hash, io.LimitReader(file, expected.Bytes+1))
		after, afterErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil || afterErr != nil || closeErr != nil || readErr != nil || count != expected.Bytes ||
			!os.SameFile(before, held) || !os.SameFile(held, after) || after.Size() != held.Size() || !after.ModTime().Equal(held.ModTime()) ||
			hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
			return nil, ErrUnavailable
		}
		result.entries = append(result.entries, observation{path, after})
	}
	return result, nil
}

// Current detects replacement or ordinary source mutation after verification.
// Main independently verifies and retains the exact served asset buffers; no
// mutable filesystem bytes are fetched by the Office surface after that load.
func (a *Assets) Current(ctx context.Context) bool {
	if a == nil || ctx == nil || ctx.Err() != nil || len(a.entries) != 6 {
		return false
	}
	for _, entry := range a.entries {
		info, err := os.Lstat(entry.path)
		if err != nil || !info.Mode().IsRegular() || !os.SameFile(entry.info, info) || info.Size() != entry.info.Size() || !info.ModTime().Equal(entry.info.ModTime()) {
			return false
		}
	}
	return true
}
