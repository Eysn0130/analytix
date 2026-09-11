package authorityanchor

import "context"

// AnchorV1 is externally selected comparison material. It contains no secret
// and is deliberately separate from the self-authenticating manifest and the
// rollbackable runtime data root. Production independence depends on the
// Source being backed by a protected launcher, OS trust store, or equivalent
// deployment authority; the value alone cannot prove that provenance.
type AnchorV1 struct {
	InstallationID        string
	AuthorityKeyID        string
	AuthorityPublicKey    []byte
	CurrentManifestDigest string
}

type Source interface {
	Load(context.Context) (AnchorV1, error)
}
