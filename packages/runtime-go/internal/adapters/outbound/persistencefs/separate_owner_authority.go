package persistencefs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SeparateOwnerRootAuthority freezes a filesystem root that is coordinated by
// the persistence lease but owned by another component. It deliberately does
// not enter RootSet, RootAuthority, or the semantic-startup journal.
//
// Existing roots bind every existing ancestor, the root's strong object
// identity, and platform security metadata. Missing roots bind the same
// ancestor chain plus the exact missing suffix and can only be observed; this
// authority never creates a separate-owner root.
type SeparateOwnerRootAuthority struct {
	mu            sync.Mutex
	root          string
	bindings      []separateOwnerPathBindingV1
	rootExists    bool
	missingSuffix string
	digest        string
	pin           separateOwnerRootPin
}

type separateOwnerRootPin interface {
	Validate(separateOwnerPathBindingV1) error
	OpenRoot() (*startupPrivateDirectory, error)
	Close() error
}

type separateOwnerPathBindingV1 struct {
	Path           string `json:"path"`
	Identity       string `json:"identity"`
	SecurityDigest string `json:"securityDigest"`
}

type separateOwnerAuthorityDigestV1 struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Root          string                       `json:"root"`
	RootExists    bool                         `json:"rootExists"`
	MissingSuffix string                       `json:"missingSuffix,omitempty"`
	Bindings      []separateOwnerPathBindingV1 `json:"bindings"`
}

func freezeSeparateOwnerRootAuthority(root string) (*SeparateOwnerRootAuthority, error) {
	root = filepath.Clean(root)
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("separate-owner root is invalid")
	}
	anchor := root
	missing := make([]string, 0, 4)
	for {
		info, err := os.Lstat(anchor)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, errors.New("separate-owner root crossed an unsafe ancestor")
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(anchor)
		if parent == anchor {
			return nil, errors.New("separate-owner root has no existing ancestor")
		}
		missing = append(missing, filepath.Base(anchor))
		anchor = parent
	}
	rootExists := canonicalPathKey(anchor) == canonicalPathKey(root)
	paths := separateOwnerExistingAncestorPaths(anchor)
	bindings := make([]separateOwnerPathBindingV1, 0, len(paths))
	for _, path := range paths {
		identity, securityDigest, err := platformSeparateOwnerDirectoryBinding(path, rootExists && canonicalPathKey(path) == canonicalPathKey(root))
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, separateOwnerPathBindingV1{
			Path: path, Identity: identity, SecurityDigest: securityDigest,
		})
	}
	for left, right := 0, len(missing)-1; left < right; left, right = left+1, right-1 {
		missing[left], missing[right] = missing[right], missing[left]
	}
	record := separateOwnerAuthorityDigestV1{
		SchemaVersion: 1,
		Root:          root,
		RootExists:    rootExists,
		MissingSuffix: filepath.Join(missing...),
		Bindings:      bindings,
	}
	if rootExists {
		record.MissingSuffix = ""
	}
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	pin, err := platformPinSeparateOwnerRoot(anchor, rootExists, bindings[len(bindings)-1])
	if err != nil {
		return nil, err
	}
	authority := &SeparateOwnerRootAuthority{
		root: root, bindings: bindings, rootExists: rootExists, missingSuffix: record.MissingSuffix,
		digest: separateOwnerSHA256(body), pin: pin,
	}
	if err := authority.Validate(); err != nil {
		_ = pin.Close()
		return nil, err
	}
	return authority, nil
}

func separateOwnerExistingAncestorPaths(anchor string) []string {
	paths := []string{}
	current := filepath.Clean(anchor)
	for {
		paths = append(paths, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	for left, right := 0, len(paths)-1; left < right; left, right = left+1, right-1 {
		paths[left], paths[right] = paths[right], paths[left]
	}
	return paths
}

func (authority *SeparateOwnerRootAuthority) Root() string {
	if authority == nil {
		return ""
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.root
}

func (authority *SeparateOwnerRootAuthority) Digest() string {
	if authority == nil {
		return ""
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.digest
}

func (authority *SeparateOwnerRootAuthority) RootExists() bool {
	if authority == nil {
		return false
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.rootExists
}

func (authority *SeparateOwnerRootAuthority) Validate() error {
	if authority == nil {
		return errors.New("separate-owner root authority is unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.root == "" || !filepath.IsAbs(authority.root) || len(authority.bindings) == 0 ||
		!separateOwnerIsSHA256(authority.digest) || authority.pin == nil {
		return errors.New("separate-owner root authority is invalid")
	}
	for index, binding := range authority.bindings {
		if binding.Path == "" || binding.Identity == "" || !separateOwnerIsSHA256(binding.SecurityDigest) {
			return errors.New("separate-owner ancestor authority is incomplete")
		}
		requirePrivateRoot := authority.rootExists && index == len(authority.bindings)-1 && canonicalPathKey(binding.Path) == canonicalPathKey(authority.root)
		identity, securityDigest, err := platformSeparateOwnerDirectoryBinding(binding.Path, requirePrivateRoot)
		if err != nil || identity != binding.Identity || securityDigest != binding.SecurityDigest {
			return errors.New("separate-owner root or ancestor identity/permissions changed")
		}
	}
	record := separateOwnerAuthorityDigestV1{
		SchemaVersion: 1,
		Root:          authority.root,
		RootExists:    authority.rootExists,
		MissingSuffix: authority.missingSuffix,
		Bindings:      append([]separateOwnerPathBindingV1(nil), authority.bindings...),
	}
	body, err := json.Marshal(record)
	if err != nil || separateOwnerSHA256(body) != authority.digest {
		return errors.New("separate-owner root authority digest is invalid")
	}
	if err := authority.pin.Validate(authority.bindings[len(authority.bindings)-1]); err != nil {
		return err
	}
	if authority.rootExists {
		if authority.missingSuffix != "" || canonicalPathKey(authority.bindings[len(authority.bindings)-1].Path) != canonicalPathKey(authority.root) {
			return errors.New("separate-owner root binding is incomplete")
		}
		return nil
	}
	if strings.TrimSpace(authority.missingSuffix) == "" {
		return errors.New("separate-owner missing root suffix is unavailable")
	}
	if _, err := os.Lstat(authority.root); err == nil {
		return errors.New("separate-owner root appeared after authority freeze")
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("separate-owner root state is ambiguous")
	}
	return nil
}

func (authority *SeparateOwnerRootAuthority) close() error {
	if authority == nil {
		return nil
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.pin == nil {
		return nil
	}
	err := authority.pin.Close()
	authority.pin = nil
	return err
}

func (authority *SeparateOwnerRootAuthority) openPinnedRoot() (*startupPrivateDirectory, error) {
	if authority == nil {
		return nil, errors.New("separate-owner root authority is unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if !authority.rootExists || authority.pin == nil {
		return nil, errors.New("separate-owner root is not present")
	}
	return authority.pin.OpenRoot()
}

func (authority *SeparateOwnerRootAuthority) rootBinding() (separateOwnerPathBindingV1, bool) {
	if authority == nil {
		return separateOwnerPathBindingV1{}, false
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if !authority.rootExists || len(authority.bindings) == 0 {
		return separateOwnerPathBindingV1{}, false
	}
	binding := authority.bindings[len(authority.bindings)-1]
	return binding, canonicalPathKey(binding.Path) == canonicalPathKey(authority.root)
}

func separateOwnerAuthoritiesDigest(authorities map[string]*SeparateOwnerRootAuthority) string {
	keys := make([]string, 0, len(authorities))
	for key := range authorities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	type entry struct {
		Root   string `json:"root"`
		Digest string `json:"digest"`
	}
	records := make([]entry, 0, len(keys))
	for _, key := range keys {
		records = append(records, entry{Root: authorities[key].Root(), Digest: authorities[key].Digest()})
	}
	body, _ := json.Marshal(records)
	return separateOwnerSHA256(body)
}

func separateOwnerSHA256(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func separateOwnerIsSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
