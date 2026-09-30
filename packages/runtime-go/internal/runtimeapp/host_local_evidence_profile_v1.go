package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	evidenceauthorityhostlocal "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthorityhostlocal"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

type runtimeHostLocalProfileDecisionV1 uint8

const (
	runtimeHostLocalProfileUnavailableV1 runtimeHostLocalProfileDecisionV1 = iota
	runtimeHostLocalProfileAdmitV1
	runtimeHostLocalProfileResumeV1
)

// classifyRuntimeHostLocalProfileV1 is the read-only mode fixed point. Any
// single-sided marker/history residue or mixed witnessed state blocks this
// case authority. The ordinary Agent remains independently available.
func classifyRuntimeHostLocalProfileV1(witnessConfigured, protectedLineage,
	selectorRootPresent, headOwnerPresent, simulation bool) runtimeHostLocalProfileDecisionV1 {
	if witnessConfigured || simulation {
		return runtimeHostLocalProfileUnavailableV1
	}
	if selectorRootPresent || headOwnerPresent {
		if selectorRootPresent && headOwnerPresent {
			return runtimeHostLocalProfileResumeV1
		}
		return runtimeHostLocalProfileUnavailableV1
	}
	if protectedLineage {
		return runtimeHostLocalProfileUnavailableV1
	}
	return runtimeHostLocalProfileAdmitV1
}

func runtimeHostLocalProfileRootsPresentV1(dataDir string) (bool, bool, error) {
	privateRoot := filepath.Join(dataDir, "private")
	selector, err := runtimeHostLocalRootPresentV1(filepath.Join(privateRoot, "evidence-authority-host-local-projection"))
	if err != nil {
		return false, false, err
	}
	headOwner, err := runtimeHostLocalRootPresentV1(filepath.Join(privateRoot, "evidence-authority-host-local"))
	return selector, headOwner, err
}

func runtimeHostLocalRootPresentV1(root string) (bool, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("host-local authority root inventory is invalid")
	}
	return true, nil
}

func openRuntimeHostLocalProfileV1(ctx context.Context, config Config,
	rootAuthority *persistencefs.RootAuthority,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	authority finalauthorityport.Authority,
	decision runtimeHostLocalProfileDecisionV1) (*evidenceauthorityhostlocal.HostLocalStore, error) {
	if decision == runtimeHostLocalProfileUnavailableV1 {
		return nil, nil
	}
	if ctx == nil || rootAuthority == nil || rootAuthority.Validate() != nil || access == nil ||
		authority == nil || !domainsecurity.IsSHA256Hex(authority.KeyID()) ||
		!filepath.IsAbs(config.DataDir) {
		return nil, errors.New("host-local mode admission inputs are invalid")
	}
	rootBindingDigest := domainsecurity.SHA256Hex([]byte("analytix.host-local-protected-roots/v1\x00" +
		rootAuthority.Digest() + "\x00" + config.DataDir + "\x00" + config.UserDataDir))
	privateRoot := filepath.Join(config.DataDir, "private")
	store, err := evidenceauthorityhostlocal.OpenHostLocalStore(ctx,
		filepath.Join(privateRoot, "evidence-authority-host-local", "heads"),
		filepath.Join(privateRoot, "evidence-authority-host-local-projection"),
		access, authority.KeyID(), rootBindingDigest, authority)
	if err != nil {
		return nil, err
	}
	if decision == runtimeHostLocalProfileAdmitV1 {
		genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
			InstallationID: authority.KeyID(), RootBindingDigest: rootBindingDigest,
			MutationID: domainsecurity.SHA256Hex([]byte("analytix.host-local-mode-commitment/v1\x00" +
				authority.KeyID() + "\x00" + rootBindingDigest)),
			DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
			PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
			AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
		}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
		if err == nil {
			err = store.CommitGenesis(ctx, genesis)
		}
		if err != nil {
			return nil, errors.Join(err, store.Close())
		}
	}
	selected, found, err := store.Current(ctx)
	if err != nil || !found || selected.InstallationID != authority.KeyID() ||
		selected.RootBindingDigest != rootBindingDigest {
		return nil, errors.Join(errors.New("host-local mode commitment is unavailable"), err, store.Close())
	}
	if _, err := store.CurrentModeCommitment(ctx); err != nil {
		return nil, errors.Join(err, store.Close())
	}
	return store, nil
}
