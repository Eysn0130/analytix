package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

// RunDesktopInstallationKeyPreflightV1 performs the ordinary semantic Prepare
// under one lease, then verifies that Prepare left an existing installation
// signer at the frozen data root. It neither activates a listener nor enrolls
// a witness. The future enrollment writer must run inside this same lease,
// before its final close, rather than treating this check as publication.
func RunDesktopInstallationKeyPreflightV1(ctx context.Context, config Config) (resultErr error) {
	if ctx == nil {
		return errors.New("desktop installation preflight context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	config = normalizeConfig(config)
	if config.UserDataDir == "" || config.AuthorityAnchorV1 != "" ||
		config.AuthorityManifestRoot != "" || config.AuthorityCredentialProfileRoot != "" ||
		config.AuthorityCredentialBundleRoot != "" {
		return errors.New("desktop installation preflight authority input is invalid")
	}
	if err := validateRuntimeProtectedRootTopology(config); err != nil {
		return err
	}
	roots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		return err
	}
	lease, err := persistencefs.AcquireCompositeLeaseWithStartupUserDataAndSeparateOwnerRoots(
		roots, config.UserDataDir, config.UserDataDir,
	)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lease.Close()) }()
	if err := RunRuntimeSemanticStartupMigrationWithPersistenceLeaseContextE(ctx, config, lease); err != nil {
		return err
	}
	rootAuthority, held := lease.FrozenAuthority()
	if !held || rootAuthority.Validate() != nil {
		return errors.New("desktop installation preflight root authority changed")
	}
	keyPath := filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json")
	key, err := finalauthority.OpenExistingFileAuthority(keyPath)
	if err != nil {
		return err
	}
	anchor, err := finalauthority.NewExistingFileAuthorityAnchor(rootAuthority, key.KeyID(), key.PublicKey())
	if err != nil {
		return err
	}
	anchored, err := anchor.Open(keyPath)
	if err != nil {
		return err
	}
	return anchored.ValidateCurrentInstallation(ctx)
}
