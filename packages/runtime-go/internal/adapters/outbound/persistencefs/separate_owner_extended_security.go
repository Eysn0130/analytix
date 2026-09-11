//go:build darwin || linux

package persistencefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"

	"golang.org/x/sys/unix"
)

const (
	separateOwnerSecuritySchemaV1 = 1
	separateOwnerMaxXattrNames    = 128
	separateOwnerMaxXattrBytes    = 64 << 10
	separateOwnerMaxXattrValue    = 64 << 10
	separateOwnerMaxXattrTotal    = 256 << 10
)

type separateOwnerUnixSecurityScope uint8

const (
	separateOwnerAncestorScope separateOwnerUnixSecurityScope = iota + 1
	separateOwnerObjectScope
)

type separateOwnerUnixXattrV1 struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
	value  []byte
}

type separateOwnerUnixExtendedSecurityV1 struct {
	Platform   string                     `json:"platform"`
	Flags      uint64                     `json:"flags"`
	ACLKind    string                     `json:"aclKind"`
	ACLSHA256  string                     `json:"aclSha256"`
	ACLEntries uint32                     `json:"aclEntries"`
	Xattrs     []separateOwnerUnixXattrV1 `json:"xattrs"`
}

type separateOwnerUnixSecurityV1 struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	ObjectType    string                              `json:"objectType"`
	UID           uint32                              `json:"uid"`
	GID           uint32                              `json:"gid"`
	Mode          uint32                              `json:"mode"`
	Extended      separateOwnerUnixExtendedSecurityV1 `json:"extended"`
}

func separateOwnerUnixSecurityDigest(
	fd int,
	stat unix.Stat_t,
	objectType string,
	scope separateOwnerUnixSecurityScope,
) (string, error) {
	if fd < 0 || objectType != "directory" && objectType != "file" {
		return "", errors.New("separate-owner Unix security input is invalid")
	}
	extended, err := platformSeparateOwnerExtendedSecurity(fd, scope)
	if err != nil {
		return "", err
	}
	defer clearSeparateOwnerUnixXattrs(extended.Xattrs)
	record := separateOwnerUnixSecurityV1{
		SchemaVersion: separateOwnerSecuritySchemaV1,
		ObjectType:    objectType,
		UID:           stat.Uid,
		GID:           stat.Gid,
		Mode:          uint32(stat.Mode & 0o7777),
		Extended:      extended,
	}
	body, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return separateOwnerSHA256(body), nil
}

func readSeparateOwnerUnixXattrs(fd int) ([]separateOwnerUnixXattrV1, error) {
	if fd < 0 {
		return nil, errors.New("separate-owner Unix xattr handle is invalid")
	}
	size, err := unix.Flistxattr(fd, nil)
	if err != nil || size < 0 || size > separateOwnerMaxXattrBytes {
		return nil, errors.Join(errors.New("separate-owner Unix xattr names are unavailable"), err)
	}
	if size == 0 {
		return []separateOwnerUnixXattrV1{}, nil
	}
	rawNames := make([]byte, size)
	read, err := unix.Flistxattr(fd, rawNames)
	if err != nil || read != size || rawNames[len(rawNames)-1] != 0 {
		return nil, errors.Join(errors.New("separate-owner Unix xattr names changed"), err)
	}
	parts := bytes.Split(rawNames[:len(rawNames)-1], []byte{0})
	if len(parts) == 0 || len(parts) > separateOwnerMaxXattrNames {
		return nil, errors.New("separate-owner Unix xattr name count is unsafe")
	}
	names := make([]string, 0, len(parts))
	for _, raw := range parts {
		if len(raw) == 0 || len(raw) > 1024 || bytes.IndexByte(raw, '/') >= 0 {
			return nil, errors.New("separate-owner Unix xattr name is invalid")
		}
		names = append(names, string(raw))
	}
	sort.Strings(names)
	result := make([]separateOwnerUnixXattrV1, 0, len(names))
	total := 0
	for index, name := range names {
		if index > 0 && names[index-1] == name {
			clearSeparateOwnerUnixXattrs(result)
			return nil, errors.New("separate-owner Unix xattr names are duplicated")
		}
		valueSize, err := unix.Fgetxattr(fd, name, nil)
		if err != nil || valueSize < 0 || valueSize > separateOwnerMaxXattrValue || total > separateOwnerMaxXattrTotal-valueSize {
			clearSeparateOwnerUnixXattrs(result)
			return nil, errors.Join(errors.New("separate-owner Unix xattr value is unavailable"), err)
		}
		value := make([]byte, valueSize)
		if valueSize > 0 {
			read, err = unix.Fgetxattr(fd, name, value)
			if err != nil || read != valueSize {
				clear(value)
				clearSeparateOwnerUnixXattrs(result)
				return nil, errors.Join(errors.New("separate-owner Unix xattr value changed"), err)
			}
		}
		total += valueSize
		result = append(result, separateOwnerUnixXattrV1{
			Name: name, Size: valueSize, SHA256: separateOwnerSHA256(value), value: value,
		})
	}
	return result, nil
}

func clearSeparateOwnerUnixXattrs(xattrs []separateOwnerUnixXattrV1) {
	for index := range xattrs {
		clear(xattrs[index].value)
		xattrs[index].value = nil
	}
}
