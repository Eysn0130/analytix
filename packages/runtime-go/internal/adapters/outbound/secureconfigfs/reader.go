package secureconfigfs

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

var ErrUnsupported = errors.New("secure configuration file reading is unsupported on this platform")

const maxBundleFiles = 16

type ReadExactInput struct {
	Root         string
	Target       string
	AllowedNames []string
	MaxBytes     int64
}

type BundleFile struct {
	Name      string
	MaxBytes  int64
	Sensitive bool
}

type ReadBundleInput struct {
	Root          string
	Files         []BundleFile
	MaxTotalBytes int64
}

type normalizedBundle struct {
	root          string
	expectedNames []string
	files         []BundleFile
	maxTotalBytes int64
}

type normalizedBundleReader func(normalizedBundle) (map[string][]byte, error)

// ReadExact samples one configuration file twice through one validated private
// directory handle and rejects observed identity, content, inventory, or root
// changes. A normal filesystem is not transactional: callers must still bind
// the returned bytes to an independent digest, signature, or fingerprint.
func ReadExact(input ReadExactInput) ([]byte, error) {
	return readExactWith(input, readBundle)
}

func readExactWith(input ReadExactInput, reader normalizedBundleReader) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("secure configuration reader is unavailable")
	}
	normalized, err := normalizeInput(input)
	if err != nil {
		return nil, err
	}
	files, err := reader(normalizedBundle{
		root:          normalized.Root,
		expectedNames: normalized.AllowedNames,
		files:         []BundleFile{{Name: normalized.Target, MaxBytes: normalized.MaxBytes}},
		maxTotalBytes: normalized.MaxBytes,
	})
	if err != nil {
		return nil, err
	}
	return files[normalized.Target], nil
}

// ReadBundle reads every file in one exact private directory inventory while
// retaining the same directory handle across validation and two complete read
// phases. Callers must still cryptographically verify the signed manifest,
// file fingerprints, and certificate/key relationships: a normal filesystem
// directory is not a transactional or cryptographic snapshot. Returned
// Sensitive buffers are caller-owned and must be cleared as soon as parsed.
func ReadBundle(input ReadBundleInput) (map[string][]byte, error) {
	return readBundleWith(input, readBundle)
}

func readBundleWith(input ReadBundleInput, reader normalizedBundleReader) (map[string][]byte, error) {
	if reader == nil {
		return nil, errors.New("secure configuration reader is unavailable")
	}
	normalized, err := normalizeBundleInput(input)
	if err != nil {
		return nil, err
	}
	return reader(normalized)
}

func normalizeInput(input ReadExactInput) (ReadExactInput, error) {
	root := strings.TrimSpace(input.Root)
	if root == "" || root != input.Root || !filepath.IsAbs(root) || filepath.Clean(root) != root ||
		!safeName(input.Target) || input.MaxBytes <= 0 {
		return ReadExactInput{}, errors.New("secure configuration read input is invalid")
	}
	allowed := append([]string(nil), input.AllowedNames...)
	if len(allowed) != 1 || allowed[0] != input.Target {
		return ReadExactInput{}, errors.New("secure configuration exact read requires a singleton target inventory")
	}
	for _, name := range allowed {
		if !safeName(name) {
			return ReadExactInput{}, errors.New("secure configuration inventory name is invalid")
		}
	}
	sort.Strings(allowed)
	return ReadExactInput{Root: root, Target: input.Target, AllowedNames: allowed, MaxBytes: input.MaxBytes}, nil
}

func normalizeBundleInput(input ReadBundleInput) (normalizedBundle, error) {
	root := strings.TrimSpace(input.Root)
	if root == "" || root != input.Root || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return normalizedBundle{}, errors.New("secure configuration bundle root is invalid")
	}
	if len(input.Files) == 0 {
		return normalizedBundle{}, errors.New("secure configuration bundle inventory is empty")
	}
	if len(input.Files) > maxBundleFiles || input.MaxTotalBytes <= 0 {
		return normalizedBundle{}, errors.New("secure configuration bundle bounds are invalid")
	}
	files := append([]BundleFile(nil), input.Files...)
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if !safeName(file.Name) || file.MaxBytes <= 0 || file.MaxBytes > input.MaxTotalBytes {
			return normalizedBundle{}, errors.New("secure configuration bundle file is invalid")
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return normalizedBundle{}, errors.New("secure configuration bundle contains duplicates")
		}
		seen[file.Name] = struct{}{}
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Name < files[right].Name })
	expectedNames := make([]string, len(files))
	for index, file := range files {
		expectedNames[index] = file.Name
	}
	return normalizedBundle{
		root: root, expectedNames: expectedNames, files: files, maxTotalBytes: input.MaxTotalBytes,
	}, nil
}

func safeName(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." && value != ".." &&
		filepath.Base(value) == value && !strings.ContainsAny(value, "/\\\x00")
}
