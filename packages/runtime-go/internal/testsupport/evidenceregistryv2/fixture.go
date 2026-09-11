package evidenceregistryv2

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	evidenceregistryadapter "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

// PreparedRecovery is the narrow semantic/revalidation seam consumed by
// higher-layer tests. Filesystem paths and recovery adapters stay owned here.
type PreparedRecovery interface {
	ValidateSemantics(context.Context) error
	Revalidate(context.Context) error
}

type Stores struct {
	Indexes  registryport.AuthorityIndexStore
	Capsules registryport.AuthorityCapsuleStore
}

// Fixture owns one isolated real V2 evidence-registry CAS topology. It exposes
// only registry ports plus bounded recovery and fresh-adapter operations.
type Fixture struct {
	root   string
	access *privatecastest.AccessAuthority
	stores Stores
}

func New(baseRoot string) (*Fixture, error) {
	baseRoot = strings.TrimSpace(baseRoot)
	if baseRoot == "" {
		return nil, errors.New("evidence registry V2 fixture root is required")
	}
	root := filepath.Join(baseRoot, "evidence-registry")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	fixture := &Fixture{root: root, access: access}
	stores, err := fixture.FreshStores()
	if err != nil {
		return nil, err
	}
	fixture.stores = stores
	return fixture, nil
}

func (fixture *Fixture) Stores() Stores {
	if fixture == nil {
		return Stores{}
	}
	return fixture.stores
}

func (fixture *Fixture) PrepareRecovery(ctx context.Context) (PreparedRecovery, error) {
	if fixture == nil || fixture.access == nil || fixture.root == "" {
		return nil, errors.New("evidence registry V2 fixture is unavailable")
	}
	return evidenceregistryadapter.PrepareRecoveryV2(ctx, fixture.root, fixture.access)
}

// FreshStores returns new real adapter instances over the same durable V2
// leaves. Existing wrappers remain live; this is not a Close or process restart.
func (fixture *Fixture) FreshStores() (Stores, error) {
	if fixture == nil || fixture.access == nil || fixture.root == "" {
		return Stores{}, errors.New("evidence registry V2 fixture is unavailable")
	}
	indexes, err := evidenceregistryadapter.NewAuthorityIndexStoreV2(
		filepath.Join(fixture.root, "indexes"), fixture.access,
	)
	if err != nil {
		return Stores{}, err
	}
	capsules, err := evidenceregistryadapter.NewAuthorityCapsuleStoreV2(
		filepath.Join(fixture.root, "capsules"), fixture.access,
	)
	if err != nil {
		return Stores{}, err
	}
	return Stores{Indexes: indexes, Capsules: capsules}, nil
}
