package pluginmaterializationauthority

import (
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

const KeyFileNameV1 = "bundled-plugin-materialization-ed25519-v1.json"

// Authority is a separate installation-local plugin authority. It uses a
// distinct key file and receipt signature domain from the Final Evidence /
// Final Answer authority. finalauthority is reused only as the hardened
// Ed25519 key-file primitive; no final-publication key is read or copied.
type Authority struct {
	key *finalauthority.FileAuthority
}

func KeyPathV1(dataDir string) string {
	return filepath.Join(dataDir, "private", "plugin-materialization-authority", KeyFileNameV1)
}

func OpenOrCreateV1(dataDir string, materializationStateExists bool) (*Authority, error) {
	if dataDir == "" || !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir {
		return nil, errors.New("plugin materialization authority data root is invalid")
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(KeyPathV1(dataDir), materializationStateExists)
	if err != nil {
		return nil, errors.Join(errors.New("plugin materialization installation authority is unavailable"), err)
	}
	return &Authority{key: key}, nil
}

func (authority *Authority) KeyID() string {
	if authority == nil || authority.key == nil {
		return ""
	}
	return authority.key.KeyID()
}

func (authority *Authority) PublicKey() []byte {
	if authority == nil || authority.key == nil {
		return nil
	}
	return authority.key.PublicKey()
}

func (authority *Authority) Sign(ctx context.Context, body []byte) ([]byte, error) {
	if authority == nil || authority.key == nil {
		return nil, errors.New("plugin materialization installation authority is unavailable")
	}
	return authority.key.Sign(ctx, body)
}
