package persistencefs

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

var _ privatecasport.AccessAuthority = (*CompositeLease)(nil)
var _ privatecasport.ExistingAccessAuthority = (*CompositeLease)(nil)

// WithPrivateCASAccess holds the composite persistence lease across a complete
// CAS operation and supplies a frozen-root binding. Close marks the lease
// closing before waiting, so no new CAS access can enter while shutdown drains.
func (lease *CompositeLease) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	if lease == nil || access == nil {
		return errors.New("private CAS access lease is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	lease.mu.Lock()
	lease.ensureAccessCondLocked()
	if !lease.liveLockedWithRootValidation(false) {
		lease.mu.Unlock()
		return errors.New("private CAS access is outside a live persistence lease")
	}
	lease.activeAccesses++
	rootAuthority := lease.authority
	lease.mu.Unlock()
	defer func() {
		lease.mu.Lock()
		lease.activeAccesses--
		lease.accessCond.Broadcast()
		lease.mu.Unlock()
	}()
	binding, err := rootAuthority.validateAndBindPrivateCASDescendant(requestedRoot)
	if err != nil {
		return errors.New("private CAS access root is outside its frozen persistence authority")
	}
	accessErr := access(binding)
	current, err := rootAuthority.bindPrivateCASDescendant(requestedRoot)
	if err != nil || current != binding {
		return errors.Join(accessErr, errors.New("private CAS persistence root changed during access"))
	}
	return accessErr
}

// WithExistingPrivateCASAccess differs from write-capable access only for a
// cold configured persistence root. It binds the frozen existing ancestor and
// exact missing suffix, allowing a no-create adapter traversal while keeping
// root promotion exclusively owned by semantic startup.
func (lease *CompositeLease) WithExistingPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	if lease == nil || access == nil {
		return errors.New("existing private CAS access lease is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	lease.mu.Lock()
	lease.ensureAccessCondLocked()
	if !lease.liveLockedWithRootValidation(false) {
		lease.mu.Unlock()
		return errors.New("existing private CAS access is outside a live persistence lease")
	}
	lease.activeAccesses++
	rootAuthority := lease.authority
	lease.mu.Unlock()
	defer func() {
		lease.mu.Lock()
		lease.activeAccesses--
		lease.accessCond.Broadcast()
		lease.mu.Unlock()
	}()
	binding, err := rootAuthority.validateAndBindExistingPrivateCASDescendant(requestedRoot)
	if err != nil {
		return errors.Join(errors.New("existing private CAS access root is outside its frozen persistence authority"), err)
	}
	accessErr := access(binding)
	current, err := rootAuthority.bindExistingPrivateCASDescendant(requestedRoot)
	if err != nil || current != binding {
		return errors.Join(accessErr, errors.New("existing private CAS persistence root changed during access"))
	}
	return accessErr
}

func (authority *RootAuthority) validateAndBindPrivateCASDescendant(requestedRoot string) (privatecasport.RootBinding, error) {
	if err := authority.Validate(); err != nil {
		return privatecasport.RootBinding{}, err
	}
	return authority.bindValidatedPrivateCASDescendant(requestedRoot)
}

func (authority *RootAuthority) validateAndBindExistingPrivateCASDescendant(requestedRoot string) (privatecasport.RootBinding, error) {
	if err := authority.Validate(); err != nil {
		return privatecasport.RootBinding{}, err
	}
	requestedRoot = strings.TrimSpace(requestedRoot)
	requested, err := canonicalPathWithoutCreate(requestedRoot)
	if err != nil || requestedRoot == "" {
		return privatecasport.RootBinding{}, errors.New("existing private CAS requested root is invalid")
	}
	matchedRoot, err := authority.matchedPrivateCASPersistenceRoot(requested)
	if err != nil {
		return privatecasport.RootBinding{}, err
	}
	capability, err := authority.recordedRootCapability(matchedRoot)
	if err != nil {
		return privatecasport.RootBinding{}, err
	}
	if capability.RootIdentity == "" {
		return authority.bindExistingPrivateCASDescendant(requested)
	}
	return authority.bindValidatedPrivateCASDescendant(requested)
}

func (authority *RootAuthority) bindValidatedPrivateCASDescendant(requestedRoot string) (privatecasport.RootBinding, error) {
	requestedRoot = strings.TrimSpace(requestedRoot)
	requested, err := canonicalPathWithoutCreate(requestedRoot)
	if err != nil || requestedRoot == "" {
		return privatecasport.RootBinding{}, errors.New("private CAS requested root is invalid")
	}
	matchedRoot, err := authority.matchedPrivateCASPersistenceRoot(requested)
	if err != nil {
		return privatecasport.RootBinding{}, err
	}
	relative, err := filepath.Rel(matchedRoot, requested)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return privatecasport.RootBinding{}, errors.New("private CAS relative root is invalid")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	key := canonicalPathKey(matchedRoot)
	capability, capabilityPresent := authority.capabilities[key]
	identity, identityPresent := authority.strongRoots[key]
	recorded, parseErr := parseStrongDirectoryIdentity(capability.RootIdentity)
	if !capabilityPresent || capability.RootIdentity == "" || !identityPresent || parseErr != nil ||
		identity != recorded || authority.digest == "" || rootCapabilityDigest(authority.capabilities) != authority.digest {
		return privatecasport.RootBinding{}, errors.New("private CAS frozen persistence root identity is invalid")
	}
	return privatecasport.RootBinding{
		RootPath: matchedRoot, RelativePath: filepath.Clean(relative), RootIdentity: identity,
	}, nil
}

func (authority *RootAuthority) matchedPrivateCASPersistenceRoot(requested string) (string, error) {
	requestedKey := canonicalPathKey(requested)
	roots, err := authority.privateCASPersistenceRootKeys()
	if err != nil {
		return "", err
	}
	matchedRoot := ""
	for _, candidate := range roots {
		if candidate.path == "" || !pathContains(candidate.key, requestedKey) {
			continue
		}
		if matchedRoot != "" && matchedRoot != candidate.path {
			return "", errors.New("private CAS requested root has ambiguous authority")
		}
		matchedRoot = candidate.path
	}
	if matchedRoot == "" {
		return "", errors.New("private CAS requested root is not a strict persistence descendant")
	}
	return matchedRoot, nil
}

type privateCASPersistenceRootKey struct {
	path string
	key  string
}

func (authority *RootAuthority) privateCASPersistenceRootKeys() ([]privateCASPersistenceRootKey, error) {
	if authority == nil {
		return nil, errors.New("private CAS persistence roots are unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.capabilities == nil || authority.digest == "" ||
		rootCapabilityDigest(authority.capabilities) != authority.digest {
		return nil, errors.New("private CAS persistence roots are unavailable")
	}
	result := make([]privateCASPersistenceRootKey, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, root := range []privateCASPersistenceRootKey{
		{path: authority.roots.DataDir, key: authority.dataKey},
		{path: authority.roots.DurableDir, key: authority.durableKey},
	} {
		if root.path == "" {
			continue
		}
		if root.key == "" {
			return nil, errors.New("private CAS persistence root capability is unavailable")
		}
		if _, ok := authority.capabilities[root.key]; !ok {
			return nil, errors.New("private CAS persistence root capability is unavailable")
		}
		if _, duplicate := seen[root.key]; duplicate {
			continue
		}
		seen[root.key] = struct{}{}
		result = append(result, root)
	}
	return result, nil
}

func (authority *RootAuthority) bindPrivateCASDescendant(requestedRoot string) (privatecasport.RootBinding, error) {
	requestedRoot = strings.TrimSpace(requestedRoot)
	requested, err := canonicalPathWithoutCreate(requestedRoot)
	if err != nil || requestedRoot == "" {
		return privatecasport.RootBinding{}, errors.New("private CAS requested root is invalid")
	}
	matchedRoot, err := authority.matchedPrivateCASPersistenceRoot(requested)
	if err != nil {
		return privatecasport.RootBinding{}, err
	}
	relative, err := filepath.Rel(matchedRoot, requested)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return privatecasport.RootBinding{}, errors.New("private CAS relative root is invalid")
	}
	identity, legacyIdentity, err := platformStrongDirectoryIdentity(matchedRoot)
	if err != nil || authority.verifyOpenedStrongRoot(matchedRoot, identity, legacyIdentity) != nil {
		return privatecasport.RootBinding{}, errors.New("private CAS frozen persistence root identity is invalid")
	}
	return privatecasport.RootBinding{
		RootPath: matchedRoot, RelativePath: filepath.Clean(relative), RootIdentity: identity,
	}, nil
}

func (authority *RootAuthority) bindExistingPrivateCASDescendant(requestedRoot string) (privatecasport.RootBinding, error) {
	requestedRoot = strings.TrimSpace(requestedRoot)
	requested, err := canonicalPathWithoutCreate(requestedRoot)
	if err != nil || requestedRoot == "" {
		return privatecasport.RootBinding{}, errors.New("existing private CAS requested root is invalid")
	}
	matchedRoot, err := authority.matchedPrivateCASPersistenceRoot(requested)
	if err != nil {
		return privatecasport.RootBinding{}, err
	}
	capability, err := authority.recordedRootCapability(matchedRoot)
	if err != nil {
		return privatecasport.RootBinding{}, err
	}
	if capability.RootIdentity != "" {
		return authority.bindPrivateCASDescendant(requested)
	}
	if err := validateFrozenRootCapability(capability); err != nil {
		return privatecasport.RootBinding{}, err
	}
	if err := platformValidateColdRootAbsent(capability); err != nil {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root suffix changed after authority freeze")
	}
	anchor := matchedRoot
	missingSuffix := filepath.Clean(capability.MissingSuffix)
	if missingSuffix == "." || filepath.IsAbs(missingSuffix) {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root suffix is invalid")
	}
	for range strings.Split(missingSuffix, string(filepath.Separator)) {
		anchor = filepath.Dir(anchor)
	}
	if canonicalPathKey(anchor) != canonicalPathKey(capability.Anchor) {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root anchor is invalid")
	}
	identity, legacyIdentity, err := platformStrongDirectoryIdentity(anchor)
	if err != nil || legacyIdentity != capability.AnchorIdentity {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root identity is invalid")
	}
	relative, err := filepath.Rel(anchor, requested)
	if err != nil {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root relative path is unavailable")
	}
	if relative == "." {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root target equals its anchor")
	}
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return privatecasport.RootBinding{}, errors.New("existing private CAS cold-root target escapes its anchor")
	}
	return privatecasport.RootBinding{
		RootPath: anchor, RelativePath: filepath.Clean(relative), RootIdentity: identity,
	}, nil
}

// SemanticStagePrivateCASAccessAuthority is limited to one isolated
// semantic-startup stage. It does not replace the active CompositeLease and is
// invalidated before the stage callback returns.
type SemanticStagePrivateCASAccessAuthority struct {
	mu        sync.Mutex
	roots     RootSet
	authority *RootAuthority
	closed    bool
}

var _ privatecasport.AccessAuthority = (*SemanticStagePrivateCASAccessAuthority)(nil)
var _ privatecasport.ExistingAccessAuthority = (*SemanticStagePrivateCASAccessAuthority)(nil)

func (*SemanticStagePrivateCASAccessAuthority) IsSemanticStagePrivateCASAccessAuthority() bool {
	return true
}

func NewSemanticStagePrivateCASAccessAuthority(
	roots RootSet,
	authority *RootAuthority,
) (*SemanticStagePrivateCASAccessAuthority, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil || authority == nil || authority.Validate() != nil {
		return nil, errors.New("semantic stage private CAS authority is invalid")
	}
	bound, ok := authority.Roots()
	if !ok || bound != resolved {
		return nil, errors.New("semantic stage private CAS roots do not match their authority")
	}
	return &SemanticStagePrivateCASAccessAuthority{roots: resolved, authority: authority}, nil
}

func (authority *SemanticStagePrivateCASAccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	if authority == nil || access == nil {
		return errors.New("semantic stage private CAS access is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.closed || authority.authority == nil {
		return errors.New("semantic stage private CAS authority is no longer live")
	}
	// The stage constructor and Close validate the complete root set. Each
	// operation binds and revalidates its exact requested root around the
	// callback, avoiding repeated unrelated-root opens inside one read-only
	// simulation while preserving full cross-root checks at stage boundaries.
	binding, err := authority.authority.bindPrivateCASDescendant(requestedRoot)
	if err != nil {
		return errors.New("semantic stage private CAS root is outside its frozen authority")
	}
	accessErr := access(binding)
	current, err := authority.authority.bindPrivateCASDescendant(requestedRoot)
	if authority.closed || err != nil || current != binding {
		return errors.Join(accessErr, errors.New("semantic stage private CAS authority changed during access"))
	}
	return accessErr
}

func (authority *SemanticStagePrivateCASAccessAuthority) WithExistingPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	return authority.WithPrivateCASAccess(ctx, requestedRoot, access)
}

func (authority *SemanticStagePrivateCASAccessAuthority) Close() error {
	if authority == nil {
		return nil
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	var validationErr error
	if !authority.closed && authority.authority != nil {
		if err := authority.authority.Validate(); err != nil {
			validationErr = errors.New("semantic stage private CAS root authority changed before close")
		}
	}
	authority.closed = true
	authority.authority = nil
	return validationErr
}
