package providerregistry

const (
	MaxLegacySourceSnapshotBytes  = 256 << 10
	MaxLegacySourceLockOwnerBytes = 4096
)

// LegacySourceReader observes the exact single-link regular source generation
// and its single-owner migration lock. Implementations reject symlinks, inode
// mismatches, path replacement and oversized reads, returning only safe errors.
// It grants no migration authority: the application consumes its challenge and
// checks the returned source and owner against the expected recovery contract.
type LegacySourceReader interface {
	ReadLegacySource(LegacySourceRequest) (LegacySourceSnapshot, error)
}

type LegacySourceRequest struct {
	SourceLocator                string
	SourcePhysicalIdentitySHA256 string
	SourcePath                   string
	SourceDevice                 uint64
	SourceInode                  uint64
	LockOwnerToken               string
}

// LegacySourceSnapshot owns sensitive transient bytes. The caller must Clear it.
type LegacySourceSnapshot struct {
	PhysicalPath string
	Source       []byte
	LockOwner    []byte
}

func (snapshot *LegacySourceSnapshot) Clear() {
	for index := range snapshot.Source {
		snapshot.Source[index] = 0
	}
	for index := range snapshot.LockOwner {
		snapshot.LockOwner[index] = 0
	}
	snapshot.Source = nil
	snapshot.LockOwner = nil
	snapshot.PhysicalPath = ""
}
