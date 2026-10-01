package managededitingfiles

import (
	"os"
	"testing"
)

type fileInfoWithSystem struct {
	os.FileInfo
	system any
}

func (info fileInfoWithSystem) Sys() any { return info.system }

func TestSingleLinkRequiresExplicitPlatformProof(t *testing.T) {
	var missing *struct{ Nlink uint64 }
	for _, test := range []struct {
		name   string
		system any
		want   bool
	}{
		{"unix", struct{ Nlink uint64 }{1}, true},
		{"unix-pointer", &struct{ Nlink uint16 }{1}, true},
		{"alternate", struct{ NumberOfLinks int64 }{1}, true},
		{"hardlinks", struct{ Nlink uint64 }{2}, false},
		{"zero", struct{ Nlink uint64 }{0}, false},
		{"negative", struct{ Nlink int64 }{-1}, false},
		{"unknown-platform", struct{}{}, false},
		{"wrong-type", struct{ Nlink string }{"1"}, false},
		{"nil", nil, false},
		{"nil-pointer", missing, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := singleLink(fileInfoWithSystem{system: test.system}); got != test.want {
				t.Fatalf("single link proof = %v, want %v", got, test.want)
			}
		})
	}
}

type foreignIdentity struct{}

func (foreignIdentity) Regular() bool    { return true }
func (foreignIdentity) SingleLink() bool { return true }

func TestSameFileRejectsUnknownOrMissingIdentity(t *testing.T) {
	files := New()
	if files.SameFile(nil, nil) || files.SameFile(identity{}, identity{}) || files.SameFile(foreignIdentity{}, foreignIdentity{}) {
		t.Fatal("identity was inferred without a filesystem observation")
	}
}
