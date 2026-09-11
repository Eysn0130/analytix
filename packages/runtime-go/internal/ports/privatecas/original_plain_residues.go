package privatecas

import "context"

// OriginalPlainResidueV1 describes one physically proved, uncommitted file.
// It is not a logical record and grants no recovery or write permission.
type OriginalPlainResidueV1 struct {
	RelativePath string
	Mode         uint32
	Size         int64
	SHA256       string
}

type OriginalPlainResiduesV1 interface {
	DataRootV1() string
	FileStatesV1() []OriginalPlainResidueV1
	Revalidate(context.Context) error
	ObserveCopiedOriginalPlainResiduesV1(context.Context, string, RecoveryAccessAuthority) (OriginalPlainResiduesV1, error)
}
