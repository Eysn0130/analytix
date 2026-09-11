package privatecas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// AccessAuthority is a root-scoped unit-test capability. Production code
// must use persistencefs.CompositeLease or the semantic-stage authority.
type AccessAuthority struct {
	root        string
	bindingRoot string
	identity    privatecasport.DirectoryIdentity
}

func (*AccessAuthority) IsSemanticStagePrivateCASAccessAuthority() bool { return true }

func NewAccessAuthority(root string) (*AccessAuthority, error) {
	absolute, err := canonicalTestRoot(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" {
		return nil, errors.New("test private CAS access root is invalid")
	}
	bindingRoot, err := existingTestBindingRoot(filepath.Dir(absolute))
	if err != nil {
		return nil, errors.New("test private CAS binding root is invalid")
	}
	if err := os.Chmod(bindingRoot, 0o700); err != nil {
		return nil, errors.New("test private CAS binding root could not be made private")
	}
	if err := makeExistingTestPathPrivate(bindingRoot, absolute); err != nil {
		return nil, errors.New("test private CAS path could not be made private")
	}
	identity, err := platformTestDirectoryIdentity(bindingRoot)
	if err != nil {
		return nil, err
	}
	return &AccessAuthority{root: absolute, bindingRoot: bindingRoot, identity: identity}, nil
}

func makeExistingTestPathPrivate(bindingRoot string, target string) error {
	relative, err := filepath.Rel(bindingRoot, target)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("test private CAS path is outside its binding root")
	}
	current := bindingRoot
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("test private CAS path contains an unsafe existing component")
		}
		if err := os.Chmod(current, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (authority *AccessAuthority) WithPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	if authority == nil || access == nil {
		return errors.New("test private CAS access authority is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	root, err := canonicalTestRoot(strings.TrimSpace(requestedRoot))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(authority.root, root)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("test private CAS access escaped its root")
	}
	relativeToBinding, err := filepath.Rel(authority.bindingRoot, root)
	if err != nil || relativeToBinding == "." || filepath.IsAbs(relativeToBinding) || relativeToBinding == ".." ||
		strings.HasPrefix(relativeToBinding, ".."+string(filepath.Separator)) {
		return errors.New("test private CAS binding is invalid")
	}
	return access(privatecasport.RootBinding{
		RootPath: authority.bindingRoot, RelativePath: filepath.Clean(relativeToBinding), RootIdentity: authority.identity,
	})
}

func (authority *AccessAuthority) WithExistingPrivateCASAccess(
	ctx context.Context,
	requestedRoot string,
	access func(privatecasport.RootBinding) error,
) error {
	return authority.WithPrivateCASAccess(ctx, requestedRoot, access)
}

func existingTestBindingRoot(value string) (string, error) {
	current := filepath.Clean(value)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return "", errors.New("test private CAS binding root is unsafe")
			}
			return filepath.EvalSymlinks(current)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("test private CAS binding has no existing ancestor")
		}
		current = parent
	}
}

func canonicalTestRoot(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	existing := absolute
	missing := make([]string, 0, 4)
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", errors.New("test private CAS root has no existing ancestor")
		}
		missing = append(missing, filepath.Base(existing))
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
	}
	return filepath.Clean(resolved), nil
}
