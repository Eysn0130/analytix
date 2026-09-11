package privatecas

import "context"

// OriginalCreateDirectoryV1 describes one already-proved empty physical
// directory. It conveys no logical record, recovery permission or write grant.
type OriginalCreateDirectoryV1 struct {
	RelativePath string
	Mode         uint32
}

// OriginalCreateResiduesV1 is issued by a complete native catalog observation.
// Consumers must revalidate it around their complete read. A copied stage
// requires an independently supplied stage access authority and a fresh proof
// of exactly the same relative directory set, before simulation starts.
type OriginalCreateResiduesV1 interface {
	DataRootV1() string
	OwnerRootV1() string
	RelativePathsV1() []string
	DirectoryStatesV1() []OriginalCreateDirectoryV1
	Revalidate(context.Context) error
	ObserveCopiedOriginalCreateResiduesV1(context.Context, string, RecoveryAccessAuthority) (OriginalCreateResiduesV1, error)
}
