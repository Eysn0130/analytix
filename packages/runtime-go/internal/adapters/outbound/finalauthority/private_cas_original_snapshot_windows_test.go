//go:build windows

package finalauthority

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrivateCASOriginalWindowsModeRequiresFrozenAttributes(t *testing.T) {
	for _, tc := range []struct{ attributes, mode uint32 }{
		{0, 0o666}, {windows.FILE_ATTRIBUTE_READONLY, 0o444},
		{windows.FILE_ATTRIBUTE_DIRECTORY, 0o777}, {windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_READONLY, 0o555},
	} {
		identity := privateWindowsObjectIdentity{attributes: tc.attributes}
		if err := validatePrivateCASOriginalWindowsModeV1(tc.mode, identity); err != nil {
			t.Fatal(err)
		}
		if err := validatePrivateCASOriginalWindowsModeV1(tc.mode^0o222, identity); err == nil {
			t.Fatal("transient READONLY metadata accepted as original mode")
		}
		identity.attributes |= windows.FILE_ATTRIBUTE_REPARSE_POINT
		if err := validatePrivateCASOriginalWindowsModeV1(tc.mode, identity); err == nil {
			t.Fatal("reparse attributes acquired original mode")
		}
	}
}
