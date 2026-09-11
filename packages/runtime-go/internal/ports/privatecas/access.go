package privatecas

import "context"

const (
	DirectoryIdentityUnix    = "unix-dev-inode-v1"
	DirectoryIdentityWindows = "windows-file-id-128-v1"
)

// DirectoryIdentity is the comparable host identity of a frozen persistence
// root. Windows FileID carries the complete 128-bit FILE_ID_INFO value; the
// volume serial remains part of the binding.
type DirectoryIdentity struct {
	Kind         string
	Device       uint64
	Inode        uint64
	VolumeSerial uint64
	FileID       [16]byte
}

// RootBinding authorizes one strict descendant of a frozen persistence root.
// It is only valid during its AccessAuthority callback. Adapters must open
// RootPath, verify RootIdentity, and traverse RelativePath handle-relatively
// without following links or reparses.
type RootBinding struct {
	RootPath     string
	RelativePath string
	RootIdentity DirectoryIdentity
}

// AccessAuthority keeps host persistence ownership live across complete CAS
// Open, Put, Read, and List operations. Implementations must reject roots
// outside their frozen scope and must not let Close release ownership until
// access returns.
type AccessAuthority interface {
	WithPrivateCASAccess(ctx context.Context, requestedRoot string, access func(RootBinding) error) error
}

// ExistingAccessAuthority authorizes only a no-create traversal. When a
// configured persistence root is still cold, implementations bind the nearest
// frozen ancestor so the adapter can prove the requested descendant absent
// handle-relatively without promoting or creating that root.
type ExistingAccessAuthority interface {
	WithExistingPrivateCASAccess(ctx context.Context, requestedRoot string, access func(RootBinding) error) error
}

// RecoveryAccessAuthority makes no-create observation an explicit compile-time
// part of every recovery authority. Recovery code must never accept a plain
// write authority and discover the missing capability by runtime assertion.
type RecoveryAccessAuthority interface {
	AccessAuthority
	ExistingAccessAuthority
}

// SnapshotMetadataV1 is the snapshot-visible metadata bound into a
// host-issued immutable CAS commit receipt. Object identity is carried by the
// separate opaque identity digests so callers cannot confuse matching path
// metadata with matching filesystem objects.
type SnapshotMetadataV1 struct {
	Mode            uint32 `json:"mode"`
	Size            int64  `json:"size"`
	ModTimeUnixNano int64  `json:"modTimeUnixNano"`
}

// AdditionReceiptV2 is issued only by a successful no-replace CAS commit in
// the current call. Observing an already-existing equal record is never
// sufficient to create this receipt.
type AdditionReceiptV2 struct {
	SchemaVersion            int                `json:"schemaVersion"`
	RootPath                 string             `json:"rootPath"`
	RecordDigest             string             `json:"recordDigest"`
	BodySHA256               string             `json:"bodySha256"`
	ShardName                string             `json:"shardName"`
	CreatedByThisCall        bool               `json:"createdByThisCall"`
	ShardCreatedByThisCall   bool               `json:"shardCreatedByThisCall"`
	ShardExistedBeforeCommit bool               `json:"shardExistedBeforeCommit"`
	Finalized                bool               `json:"finalized"`
	RootIdentityDigest       string             `json:"rootIdentityDigest"`
	ShardIdentityDigest      string             `json:"shardIdentityDigest"`
	RecordIdentityDigest     string             `json:"recordIdentityDigest"`
	RootMetadata             SnapshotMetadataV1 `json:"rootMetadata"`
	ShardMetadata            SnapshotMetadataV1 `json:"shardMetadata"`
	RecordMetadata           SnapshotMetadataV1 `json:"recordMetadata"`
	ReceiptDigest            string             `json:"receiptDigest"`
}
