package pluginmaterialization

import (
	"context"
	"errors"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
)

var (
	ErrUnavailable = errors.New("bundled_plugin_materialization_unavailable")
	ErrNotFound    = errors.New("bundled_plugin_materialization_not_found")
	ErrInvalid     = errors.New("bundled_plugin_materialization_invalid")
	ErrCorrupt     = errors.New("bundled_plugin_materialization_corrupt")
	ErrConflict    = errors.New("bundled_plugin_materialization_conflict")
)

// InstallationAuthority is an already-enrolled installation Ed25519
// authority. Implementations must not create or replace a key while a
// materialization transaction is in progress.
type InstallationAuthority interface {
	KeyID() string
	PublicKey() []byte
	Sign(context.Context, []byte) ([]byte, error)
}

type ResultV1 struct {
	Receipt domainplugin.ReceiptV1
	Index   domainplugin.IndexV1
}

type Store interface {
	Materialize(context.Context, domainplugin.IntentV1, InstallationAuthority, time.Time) (ResultV1, error)
	ResolveActive(context.Context, InstallationAuthority) (ResultV1, error)
}
