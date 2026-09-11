package persistencefs

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// DataDirPrivateCASScope is an exact, non-redirectable namespace capability
// issued by a live CompositeLease. Its fields are deliberately private: a
// caller can hold or copy a valid capability, but cannot retarget it from the
// DataDir namespace selected at issuance to DurableDir or another descendant.
type DataDirPrivateCASScope struct {
	lease     *CompositeLease
	namespace string
}

var _ privatecasport.RecoveryAccessAuthority = (*DataDirPrivateCASScope)(nil)

// IssueDataDirPrivateCASScope binds one single-component namespace below the
// lease's frozen DataDir. The scope authorizes that exact root only; traversal
// below it remains handle-relative inside the owning CAS adapter.
func (lease *CompositeLease) IssueDataDirPrivateCASScope(
	namespace string,
) (*DataDirPrivateCASScope, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || filepath.Base(namespace) != namespace || namespace == "." ||
		strings.ContainsRune(namespace, 0) || strings.ContainsAny(namespace, `/\`) {
		return nil, errors.New("DataDir private CAS namespace is invalid")
	}
	scope := &DataDirPrivateCASScope{lease: lease, namespace: namespace}
	if _, err := scope.exactRoot(); err != nil {
		return nil, err
	}
	return scope, nil
}

// Root returns the exact DataDir descendant bound by this live scope.
func (scope *DataDirPrivateCASScope) Root() (string, error) {
	return scope.exactRoot()
}

func (scope *DataDirPrivateCASScope) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	if access == nil {
		return errors.New("DataDir private CAS access callback is unavailable")
	}
	exact, err := scope.authorizeExactRoot(requestedRoot)
	if err != nil {
		return err
	}
	return scope.lease.WithPrivateCASAccess(ctx, exact, access)
}

func (scope *DataDirPrivateCASScope) WithExistingPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	if access == nil {
		return errors.New("existing DataDir private CAS access callback is unavailable")
	}
	exact, err := scope.authorizeExactRoot(requestedRoot)
	if err != nil {
		return err
	}
	return scope.lease.WithExistingPrivateCASAccess(ctx, exact, access)
}

func (scope *DataDirPrivateCASScope) authorizeExactRoot(requestedRoot string) (string, error) {
	exact, err := scope.exactRoot()
	if err != nil {
		return "", err
	}
	requested, err := canonicalPathWithoutCreate(requestedRoot)
	if err != nil || canonicalPathKey(requested) != canonicalPathKey(exact) {
		return "", errors.New("private CAS root is outside its exact DataDir scope")
	}
	return exact, nil
}

func (scope *DataDirPrivateCASScope) exactRoot() (string, error) {
	if scope == nil || scope.lease == nil || scope.namespace == "" {
		return "", errors.New("DataDir private CAS scope is unavailable")
	}
	roots, ok := scope.lease.FrozenRoots()
	if !ok || roots.DataDir == "" {
		return "", errors.New("DataDir private CAS lease is not live")
	}
	root, err := canonicalPathWithoutCreate(filepath.Join(roots.DataDir, scope.namespace))
	if err != nil || !pathContains(canonicalPathKey(roots.DataDir), canonicalPathKey(root)) {
		return "", errors.New("DataDir private CAS scope root is invalid")
	}
	return root, nil
}
