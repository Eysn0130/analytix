// Package managededitingfiles observes filesystem identity for managed leases.
package managededitingfiles

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	filesport "analytix.local/runtime-go/internal/ports/managedediting"
)

var errUnsafePath = errors.New("managed_editing_unsafe_path")

type Files struct{}
type identity struct{ info os.FileInfo }

func (i identity) Regular() bool    { return i.info.Mode().IsRegular() }
func (i identity) SingleLink() bool { return singleLink(i.info) }

var _ filesport.Files = (*Files)(nil)

func New() *Files { return &Files{} }

func (*Files) Inspect(path string, allowMissing bool) (filesport.Identity, []filesport.Identity, error) {
	info, parents, err := inspectPath(path, allowMissing)
	if err != nil {
		return nil, nil, err
	}
	ancestors := make([]filesport.Identity, len(parents))
	for index, parent := range parents {
		ancestors[index] = identity{parent}
	}
	if info == nil {
		return nil, ancestors, nil
	}
	return identity{info}, ancestors, nil
}

func (*Files) SameFile(left, right filesport.Identity) bool {
	a, okA := left.(identity)
	b, okB := right.(identity)
	return okA && okB && a.info != nil && b.info != nil && os.SameFile(a.info, b.info)
}

func (*Files) Contains(parent, path string) bool {
	if !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return false
	}
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// inspectPath rejects symlinks in every existing component, including the final
// component. It never normalizes an ambiguous caller path into an allowed path.
func inspectPath(path string, allowMissing bool) (os.FileInfo, []os.FileInfo, error) {
	if path == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, nil, errUnsafePath
	}
	root := filepath.VolumeName(path) + string(filepath.Separator)
	current := root
	info, err := os.Lstat(current)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errUnsafePath
	}
	var ancestors []os.FileInfo
	if path == root {
		return info, ancestors, nil
	}
	components := strings.Split(strings.TrimPrefix(path, root), string(filepath.Separator))
	for index, component := range components {
		ancestors = append(ancestors, info)
		current = filepath.Join(current, component)
		info, err = os.Lstat(current)
		if err != nil {
			if allowMissing && errors.Is(err, os.ErrNotExist) {
				return nil, ancestors, nil
			}
			return nil, nil, errUnsafePath
		}
		if info.Mode()&os.ModeSymlink != 0 || (index < len(components)-1 && !info.IsDir()) {
			return nil, nil, errUnsafePath
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil || resolved != current {
			return nil, nil, errUnsafePath
		}
	}
	return info, ancestors, nil
}

// Unix Stat_t exposes Nlink; some other FileInfo implementations expose
// NumberOfLinks. When the platform cannot prove nlink==1 we fail closed rather
// than equate os.SameFile (known identity comparison) with absence of aliases.
func singleLink(info os.FileInfo) bool {
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	for _, name := range []string{"Nlink", "NumberOfLinks"} {
		field := value.FieldByName(name)
		switch field.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return field.Uint() == 1
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return field.Int() == 1
		}
	}
	return false
}
