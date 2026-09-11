package persistencefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

const (
	electronTaskRetirementAlgorithm          = "Ed25519"
	electronTaskRetirementJournalDirectoryV1 = ".analytix-electron-task-retirement-v1"
	electronTaskRetirementTargetV1           = "background-tasks.json"
	electronTaskRetirementMaxTargetBytesV1   = int64(16 << 20)
)

var (
	electronTaskRetirementPlanSignatureDomain = []byte(
		"analytix.electron-background-task-retirement-plan/signature/v2\x00",
	)
	electronTaskRetirementProtectionSignatureDomain = []byte(
		"analytix.electron-background-task-retirement-protection/signature/v2\x00",
	)
	electronTaskRetirementRemovalIntentSignatureDomain = []byte(
		"analytix.electron-background-task-retirement-removal-intent/signature/v2\x00",
	)
	electronTaskRetirementCommitSignatureDomain = []byte(
		"analytix.electron-background-task-retirement-commit/signature/v2\x00",
	)
)

// ElectronTaskRetirementAuthorityIdentityV2 is the public identity of the
// installation authority used for one retirement journal. It intentionally
// exposes neither private key material nor a generic signing capability.
type ElectronTaskRetirementAuthorityIdentityV2 struct {
	AuthorityAlgorithm string
	AuthorityKeyID     string
	RootBindingDigest  string
}

// ElectronTaskRetirementJournalAuthorityV2 binds the installation signing
// key, managed RootSet, and exact separate-owner root to one live lease.
type ElectronTaskRetirementJournalAuthorityV2 struct {
	lease       *CompositeLease
	roots       RootSet
	owner       *SeparateOwnerRootAuthority
	namespace   *JournalNamespaceAuthority
	journalRoot string
	rootBinding string
}

// ElectronTaskRetirementSigningSessionV2 pins one revalidated installation
// authority. Only purpose-specific methods are exported so callers cannot use
// it as a general signing oracle.
type ElectronTaskRetirementSigningSessionV2 struct {
	owner     *ElectronTaskRetirementJournalAuthorityV2
	authority *startupJournalAuthority
}

func IssueElectronTaskRetirementJournalAuthorityV2(
	lease *CompositeLease,
	ownerRoot string,
) (*ElectronTaskRetirementJournalAuthorityV2, error) {
	if lease == nil {
		return nil, errors.New("Electron task retirement lease is unavailable")
	}
	roots, rootsOK := lease.FrozenRoots()
	owner, ownerOK := lease.FrozenSeparateOwnerAuthority(ownerRoot)
	namespace, namespaceOK := lease.FrozenJournalAuthority()
	if !rootsOK || !ownerOK || !namespaceOK || owner == nil || namespace == nil ||
		owner.Root() == "" || namespace.Validate() != nil || !namespace.matchesRoots(roots) {
		return nil, errors.New("Electron task retirement authority is unavailable")
	}
	return &ElectronTaskRetirementJournalAuthorityV2{
		lease:       lease,
		roots:       roots,
		owner:       owner,
		namespace:   namespace,
		journalRoot: filepath.Join(namespace.path(), "journal"),
		rootBinding: rootBindingDigest(roots),
	}, nil
}

// PrepareNewTransaction may create the shared installation key only while no
// Electron retirement journal exists. It is never used for recovery.
func (authority *ElectronTaskRetirementJournalAuthorityV2) PrepareNewTransaction(
	ctx context.Context,
) (*ElectronTaskRetirementSigningSessionV2, error) {
	if err := authority.validate(ctx); err != nil {
		return nil, err
	}
	if err := authority.requireNewTransactionSource(ctx); err != nil {
		return nil, err
	}
	shared, err := loadStartupJournalAuthority(authority.namespace, authority.journalRoot)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		shared, err = openOrCreateStartupJournalAuthorityGuarded(
			authority.namespace,
			authority.journalRoot,
			func() error { return authority.requireNewTransactionSource(ctx) },
		)
		if err != nil {
			return nil, err
		}
	}
	if err := authority.requireNewTransactionSource(ctx); err != nil {
		return nil, err
	}
	return authority.newSession(ctx, shared)
}

// OpenExisting loads the installation key for recovery without creating,
// rotating, cleaning, or otherwise mutating authority state.
func (authority *ElectronTaskRetirementJournalAuthorityV2) OpenExisting(
	ctx context.Context,
) (*ElectronTaskRetirementSigningSessionV2, error) {
	if err := authority.validate(ctx); err != nil {
		return nil, err
	}
	shared, err := loadStartupJournalAuthority(authority.namespace, authority.journalRoot)
	if err != nil {
		return nil, err
	}
	return authority.newSession(ctx, shared)
}

func (authority *ElectronTaskRetirementJournalAuthorityV2) newSession(
	ctx context.Context,
	shared *startupJournalAuthority,
) (*ElectronTaskRetirementSigningSessionV2, error) {
	if err := authority.validate(ctx); err != nil || shared == nil || shared.revalidate() != nil || shared.keyID == "" {
		return nil, errors.New("Electron task retirement signing authority is unavailable")
	}
	return &ElectronTaskRetirementSigningSessionV2{owner: authority, authority: shared}, nil
}

func (authority *ElectronTaskRetirementJournalAuthorityV2) validate(ctx context.Context) error {
	if authority == nil || authority.lease == nil || authority.owner == nil || authority.namespace == nil ||
		authority.journalRoot == "" || authority.rootBinding == "" {
		return errors.New("Electron task retirement authority is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	roots, rootsOK := authority.lease.FrozenRoots()
	owner, ownerOK := authority.lease.FrozenSeparateOwnerAuthority(authority.owner.Root())
	namespace, namespaceOK := authority.lease.FrozenJournalAuthority()
	if !rootsOK || !ownerOK || !namespaceOK || roots != authority.roots || owner != authority.owner ||
		namespace != authority.namespace || authority.rootBinding != rootBindingDigest(roots) ||
		!namespace.matchesRoots(roots) || namespace.Validate() != nil || owner.Validate() != nil {
		return errors.New("Electron task retirement lease authority changed")
	}
	return nil
}

func (authority *ElectronTaskRetirementJournalAuthorityV2) requireNewTransactionSource(ctx context.Context) error {
	if err := authority.validate(ctx); err != nil {
		return err
	}
	return authority.lease.WithSeparateOwnerRootAccess(
		ctx,
		authority.owner,
		func(access *SeparateOwnerRootAccess) error {
			if !access.Present() {
				return errors.New("Electron task retirement source root is unavailable")
			}
			root, err := access.Directory()
			if err != nil {
				return err
			}
			target, _, targetPresent, err := root.CaptureFile(
				ctx,
				electronTaskRetirementTargetV1,
				electronTaskRetirementMaxTargetBytesV1,
				true,
				false,
			)
			if err != nil || !targetPresent || !target.Valid() {
				return errors.Join(errors.New("Electron task retirement source is unavailable"), err)
			}
			journal, present, err := root.OpenDirectory(ctx, electronTaskRetirementJournalDirectoryV1, true)
			if journal != nil {
				err = errors.Join(err, journal.Close())
			}
			if err != nil {
				return err
			}
			if present {
				return errors.New("Electron task retirement journal already exists")
			}
			return nil
		},
	)
}

func (session *ElectronTaskRetirementSigningSessionV2) Identity(
	ctx context.Context,
) (ElectronTaskRetirementAuthorityIdentityV2, error) {
	if err := session.validate(ctx); err != nil {
		return ElectronTaskRetirementAuthorityIdentityV2{}, err
	}
	return ElectronTaskRetirementAuthorityIdentityV2{
		AuthorityAlgorithm: electronTaskRetirementAlgorithm,
		AuthorityKeyID:     session.authority.keyID,
		RootBindingDigest:  session.owner.rootBinding,
	}, nil
}

func (session *ElectronTaskRetirementSigningSessionV2) SignPlanV2(ctx context.Context, payload []byte) (string, error) {
	return session.sign(ctx, electronTaskRetirementPlanSignatureDomain, payload)
}

func (session *ElectronTaskRetirementSigningSessionV2) VerifyPlanV2(
	ctx context.Context,
	keyID string,
	payload []byte,
	signature string,
) error {
	return session.verify(ctx, electronTaskRetirementPlanSignatureDomain, keyID, payload, signature)
}

func (session *ElectronTaskRetirementSigningSessionV2) SignProtectionV2(ctx context.Context, payload []byte) (string, error) {
	return session.sign(ctx, electronTaskRetirementProtectionSignatureDomain, payload)
}

func (session *ElectronTaskRetirementSigningSessionV2) VerifyProtectionV2(
	ctx context.Context,
	keyID string,
	payload []byte,
	signature string,
) error {
	return session.verify(ctx, electronTaskRetirementProtectionSignatureDomain, keyID, payload, signature)
}

func (session *ElectronTaskRetirementSigningSessionV2) SignRemovalIntentV2(ctx context.Context, payload []byte) (string, error) {
	return session.sign(ctx, electronTaskRetirementRemovalIntentSignatureDomain, payload)
}

func (session *ElectronTaskRetirementSigningSessionV2) VerifyRemovalIntentV2(
	ctx context.Context,
	keyID string,
	payload []byte,
	signature string,
) error {
	return session.verify(ctx, electronTaskRetirementRemovalIntentSignatureDomain, keyID, payload, signature)
}

func (session *ElectronTaskRetirementSigningSessionV2) SignCommitV2(ctx context.Context, payload []byte) (string, error) {
	return session.sign(ctx, electronTaskRetirementCommitSignatureDomain, payload)
}

func (session *ElectronTaskRetirementSigningSessionV2) VerifyCommitV2(
	ctx context.Context,
	keyID string,
	payload []byte,
	signature string,
) error {
	return session.verify(ctx, electronTaskRetirementCommitSignatureDomain, keyID, payload, signature)
}

func (session *ElectronTaskRetirementSigningSessionV2) sign(
	ctx context.Context,
	domain []byte,
	payload []byte,
) (string, error) {
	if err := session.validate(ctx); err != nil || len(payload) == 0 {
		return "", errors.New("Electron task retirement signing session is unavailable")
	}
	return session.authority.signDomain(domain, payload)
}

func (session *ElectronTaskRetirementSigningSessionV2) verify(
	ctx context.Context,
	domain []byte,
	keyID string,
	payload []byte,
	signature string,
) error {
	if err := session.validate(ctx); err != nil || len(payload) == 0 {
		return errors.New("Electron task retirement verification session is unavailable")
	}
	return session.authority.verifyDomain(domain, keyID, payload, signature)
}

func (session *ElectronTaskRetirementSigningSessionV2) validate(ctx context.Context) error {
	if session == nil || session.owner == nil || session.authority == nil {
		return errors.New("Electron task retirement signing session is unavailable")
	}
	if err := session.owner.validate(ctx); err != nil {
		return err
	}
	return session.authority.revalidate()
}
