package server

import (
	"context"
	"path/filepath"
	"testing"

	casethreadauthority "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	privatecasrecoverytest "analytix.local/runtime-go/internal/testsupport/privatecasrecovery"
)

func newServerTestPrivateFinalStore(t *testing.T, root string) (*finalauthority.PrivateStore, error) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return finalauthority.NewPrivateStore(root, access)
}

func newRecoveredServerTestPrivateFinalStore(t *testing.T, root string) (*finalauthority.PrivateStore, error) {
	t.Helper()
	privateRoot := filepath.Dir(root)
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	createResidues, err := finalauthority.PrepareSecurePrivateCASCreateResidueRecoveryV1(
		ctx, filepath.Dir(privateRoot), access,
	)
	if err != nil {
		return nil, err
	}
	if err := finalauthority.ValidateNoUnsignedSecurePrivateCASRecoveryPhasesWithPreparedCreateResiduesV4(
		ctx,
		[]string{filepath.Join(root, "records"), filepath.Join(root, "dispositions")},
		access,
		createResidues,
	); err != nil {
		return nil, err
	}
	if err := createResidues.Apply(ctx); err != nil {
		return nil, err
	}
	prepared, err := finalauthority.PreparePrivateStoreRecoveryV1(ctx, root, access)
	if err != nil {
		return nil, err
	}
	if err := prepared.ValidateSemantics(ctx); err != nil {
		return nil, err
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	if err := privatecasrecoverytest.ApplyV4(ctx, "accepted-finals-test", prepared); err != nil {
		return nil, err
	}
	return finalauthority.NewPrivateStore(root, access)
}

func newServerTestCaseThreadStore(t *testing.T, root string) (*casethreadauthority.Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return casethreadauthority.NewStore(root, mutation)
}

func newServerTestPendingWorkStore(t *testing.T, root string) (*pendingworkstore.Store, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return pendingworkstore.NewStore(root, mutation)
}

func newServerTestContinuationService(
	t *testing.T,
	root string,
	authority finalauthorityport.Authority,
) (*continuationapp.Service, error) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	store, err := continuationstore.NewStore(root, access)
	if err != nil {
		return nil, err
	}
	return continuationapp.NewService(authority, store), nil
}
