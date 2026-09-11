package pluginmaterializationfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const (
	pluginCacheRelativeV1 = "plugins/cache/analytix-hub"
	controlRelativeV1     = ".state/bundled-plugin-materialization/v1"
	activeIndexFileNameV1 = "active-index.v1.json"
)

func pluginParentRelativeV1(pluginName string) string {
	return pluginCacheRelativeV1 + "/" + pluginName
}

func activeRelativeV1(pluginName, pluginVersion string) string {
	return pluginParentRelativeV1(pluginName) + "/" + pluginVersion
}

type FaultPointV1 string

const (
	FaultAfterPreparedV1                   FaultPointV1 = "after_prepared"
	FaultAfterStagedV1                     FaultPointV1 = "after_staged"
	FaultAfterAuthorizedV1                 FaultPointV1 = "after_authorized"
	FaultAfterPriorIndexQuarantinedV1      FaultPointV1 = "after_prior_index_quarantined"
	FaultAfterPriorGenerationQuarantinedV1 FaultPointV1 = "after_prior_generation_quarantined"
	FaultAfterPriorQuarantinedV1           FaultPointV1 = "after_prior_quarantined"
	FaultAfterGenerationCommittedV1        FaultPointV1 = "after_generation_committed"
	FaultAfterIndexCommittedV1             FaultPointV1 = "after_index_committed"
)

type FaultInjectorV1 func(FaultPointV1) error

type Store struct {
	runtimeHome string
	fault       FaultInjectorV1
	mu          sync.Mutex
}

var _ pluginport.Store = (*Store)(nil)

func NewStore(runtimeHome string, fault FaultInjectorV1) (*Store, error) {
	realHome, err := canonicalDirectory(runtimeHome)
	if err != nil {
		return nil, errors.Join(pluginport.ErrInvalid, err)
	}
	store := &Store{runtimeHome: realHome, fault: fault}
	for _, relative := range []string{
		pluginCacheRelativeV1,
		pluginParentRelativeV1(domainplugin.PluginNameV1),
		controlRelativeV1,
		controlRelativeV1 + "/transactions",
		controlRelativeV1 + "/receipts",
		controlRelativeV1 + "/quarantine",
	} {
		if _, err := store.ensurePrivateDirectory(relative); err != nil {
			return nil, errors.Join(pluginport.ErrUnavailable, err)
		}
	}
	return store, nil
}

// OpenExistingStoreV1 opens only an already-materialized installation. Unlike
// NewStore it creates no directories or transaction state during activation.
func OpenExistingStoreV1(runtimeHome string) (*Store, error) {
	realHome, err := canonicalDirectory(runtimeHome)
	if err != nil {
		return nil, errors.Join(pluginport.ErrInvalid, err)
	}
	store := &Store{runtimeHome: realHome}
	for _, relative := range []string{
		pluginCacheRelativeV1,
		pluginParentRelativeV1(domainplugin.PluginNameV1),
		controlRelativeV1,
		controlRelativeV1 + "/receipts",
	} {
		path := store.absolute(relative)
		if real, err := canonicalDirectory(path); err != nil || real != path {
			return nil, errors.Join(
				pluginport.ErrUnavailable,
				errors.New("bundled plugin materialization state is unavailable"),
				err,
			)
		}
	}
	return store, nil
}

func (store *Store) Materialize(ctx context.Context, intent domainplugin.IntentV1, authority pluginport.InstallationAuthority, now time.Time) (pluginport.ResultV1, error) {
	if store == nil || ctx == nil || ctx.Err() != nil || domainplugin.ValidateIntentV1(intent) != nil || now.IsZero() {
		return pluginport.ResultV1{}, pluginport.ErrInvalid
	}
	keyID, publicKey, err := validateAuthority(authority)
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	sourceBefore, err := InspectSourceTreeV1(ctx, intent.SourceRoot)
	if err != nil || !identityMatchesIntent(sourceBefore, intent) {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrInvalid, errors.New("formal package plugin source does not match its frozen authority"), err)
	}
	if _, err := store.ensurePrivateDirectory(pluginParentRelativeV1(intent.PluginName)); err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrUnavailable, err)
	}

	if current, resolveErr := store.resolveActive(ctx, keyID, publicKey); resolveErr == nil {
		if current.Receipt.IntentID != intent.IntentID || current.Receipt.PackageAuthoritySHA256 != intent.PackageAuthoritySHA256 ||
			current.Receipt.SourceTreeSHA256 != intent.SourceTreeSHA256 || current.Receipt.SourceTreeFileCount != intent.SourceTreeFileCount {
			// A rebuilt package has a different independently verified package
			// authority even when the bundled plugin version and bytes are
			// unchanged. The existing generation has already been verified under
			// this installation authority by resolveActive. Permit the normal
			// journaled quarantine/materialization path only for an authority
			// rotation on the same target. A target change, or content drift under
			// the same package authority, remains a conflict.
			if current.Receipt.Target != intent.Target ||
				current.Receipt.PackageAuthoritySHA256 == intent.PackageAuthoritySHA256 {
				return pluginport.ResultV1{}, pluginport.ErrConflict
			}
		} else {
			if err := store.quarantineLegacyFundsSkillProjections(intent); err != nil {
				return pluginport.ResultV1{}, err
			}
			store.completeJournalBestEffort(intent, current.Receipt.GenerationID, now)
			return current, nil
		}
	} else if !errors.Is(resolveErr, os.ErrNotExist) {
		return pluginport.ResultV1{}, resolveErr
	}

	generationID := generationIDV1(intent, keyID)
	stagingRelative := pluginParentRelativeV1(intent.PluginName) + "/.staging-" + intent.IntentID
	transactionRelative := controlRelativeV1 + "/transactions/" + intent.IntentID
	transactionRoot, err := store.ensurePrivateDirectory(transactionRelative)
	if err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := store.putExact(filepath.Join(transactionRoot, "intent.v1.json"), mustIntentBytes(intent)); err != nil {
		return pluginport.ResultV1{}, err
	}
	lastPhase, err := store.latestJournal(transactionRoot, intent, generationID, stagingRelative)
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	if lastPhase < 1 {
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 1, domainplugin.JournalPreparedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.inject(FaultAfterPreparedV1); err != nil {
			return pluginport.ResultV1{}, err
		}
		lastPhase = 1
	}

	stagingPath := store.absolute(stagingRelative)
	if lastPhase < 2 {
		if err := store.prepareStaging(ctx, intent, sourceBefore, stagingPath); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 2, domainplugin.JournalStagedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.inject(FaultAfterStagedV1); err != nil {
			return pluginport.ResultV1{}, err
		}
		lastPhase = 2
	}

	receiptPath := filepath.Join(store.absolute(controlRelativeV1+"/receipts"), generationID+".json")
	receipt, err := store.loadOrCreateReceipt(ctx, receiptPath, intent, generationID, keyID, publicKey, authority, now)
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	if lastPhase < 3 {
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 3, domainplugin.JournalAuthorizedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.inject(FaultAfterAuthorizedV1); err != nil {
			return pluginport.ResultV1{}, err
		}
		lastPhase = 3
	}

	if lastPhase < 4 {
		if err := store.quarantineLegacyFundsSkillProjections(intent); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.quarantineDiscoverable(intent); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 4, domainplugin.JournalPriorQuarantinedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.inject(FaultAfterPriorQuarantinedV1); err != nil {
			return pluginport.ResultV1{}, err
		}
		lastPhase = 4
	}

	activePath := store.absolute(activeRelativeV1(intent.PluginName, intent.PluginVersion))
	if lastPhase < 5 {
		if err := store.commitGeneration(ctx, stagingPath, activePath, intent); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 5, domainplugin.JournalGenerationCommittedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.inject(FaultAfterGenerationCommittedV1); err != nil {
			return pluginport.ResultV1{}, err
		}
		lastPhase = 5
	}

	receiptIssuedAt, parseTimeErr := time.Parse(time.RFC3339Nano, receipt.IssuedAt)
	if parseTimeErr != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, parseTimeErr)
	}
	index, err := domainplugin.NewIndexV1(receipt, receiptIssuedAt)
	if err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	if lastPhase < 6 {
		body, _ := domainplugin.IndexV1Bytes(index)
		if err := store.publishActiveIndex(body); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 6, domainplugin.JournalIndexCommittedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
		if err := store.inject(FaultAfterIndexCommittedV1); err != nil {
			return pluginport.ResultV1{}, err
		}
		lastPhase = 6
	} else {
		activeIndexBody, readErr := stableReadFile(store.activeIndexPath(), domainplugin.MaxContractBytesV1)
		persisted, parseErr := domainplugin.ParseIndexV1(activeIndexBody)
		if readErr != nil || parseErr != nil || domainplugin.ValidateIndexForReceiptV1(persisted, receipt) != nil {
			return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, readErr, parseErr)
		}
		index = persisted
	}
	if lastPhase < 7 {
		if err := store.appendJournal(transactionRoot, intent, generationID, stagingRelative, 7, domainplugin.JournalCompletedV1, now); err != nil {
			return pluginport.ResultV1{}, err
		}
	}
	return store.resolveActive(ctx, keyID, publicKey)
}

func (store *Store) ResolveActive(ctx context.Context, authority pluginport.InstallationAuthority) (pluginport.ResultV1, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return pluginport.ResultV1{}, pluginport.ErrUnavailable
	}
	keyID, publicKey, err := validateAuthority(authority)
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.resolveActive(ctx, keyID, publicKey)
}

func (store *Store) resolveActive(ctx context.Context, keyID string, publicKey []byte) (pluginport.ResultV1, error) {
	indexBody, err := stableReadFile(store.activeIndexPath(), domainplugin.MaxContractBytesV1)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pluginport.ResultV1{}, errors.Join(pluginport.ErrUnavailable, os.ErrNotExist)
		}
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	index, err := domainplugin.ParseIndexV1(indexBody)
	expectedActiveRelative := activeRelativeV1(index.PluginName, index.PluginVersion)
	if err != nil || index.ActiveRelativePath != expectedActiveRelative {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	receiptPath := filepath.Join(store.absolute(controlRelativeV1+"/receipts"), index.GenerationID+".json")
	receiptBody, err := stableReadFile(receiptPath, domainplugin.MaxContractBytesV1)
	if err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	receipt, err := domainplugin.ParseReceiptV1(receiptBody)
	if err != nil || domainplugin.ValidateTrustedReceiptV1(receipt, keyID, publicKey) != nil ||
		domainplugin.ValidateIndexForReceiptV1(index, receipt) != nil || receipt.GenerationID != index.GenerationID {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	activeRoot := store.absolute(expectedActiveRelative)
	identity, err := inspectInstalledTreeV1(ctx, activeRoot)
	identityMatchesReceipt := identity.Declaration.PackageID == receipt.PluginName &&
		identity.Declaration.PackageVersion == receipt.PluginVersion
	if identity.LegacyV0 {
		identityMatchesReceipt = identity.LegacyPackageID == receipt.PluginName &&
			identity.LegacyVersion == receipt.PluginVersion
	}
	if err != nil || identity.TreeSHA256 != receipt.SourceTreeSHA256 || identity.FileCount != receipt.SourceTreeFileCount ||
		!identityMatchesReceipt ||
		identity.ManifestSHA256 != receipt.ManifestSHA256 || identity.EntrypointSHA256 != receipt.EntrypointSHA256 {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, errors.New("active bundled plugin tree does not match its signed receipt"), err)
	}
	if err := store.validateInstallMarker(receipt); err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	if _, err := InspectInstallMarkerV1(activeRoot, receipt); err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	if err := store.requireSingleDiscoverableGeneration(receipt.PluginName, receipt.PluginVersion); err != nil {
		return pluginport.ResultV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	return pluginport.ResultV1{Receipt: receipt, Index: index}, nil
}

func (store *Store) prepareStaging(ctx context.Context, intent domainplugin.IntentV1, sourceBefore SourceTreeIdentityV1, stagingPath string) error {
	if info, err := os.Lstat(stagingPath); err == nil {
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			if identity, inspectErr := inspectInstalledTreeV1(ctx, stagingPath); inspectErr == nil && identityMatchesIntent(identity, intent) && store.validateMarkerAt(stagingPath, intent) == nil {
				return nil
			}
		}
		if err := removeOwnedStaging(stagingPath); err != nil {
			return errors.Join(pluginport.ErrCorrupt, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := copySourceTree(ctx, intent.SourceRoot, stagingPath); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := store.writeInstallMarker(stagingPath, intent); err != nil {
		return err
	}
	sourceAfter, sourceErr := InspectSourceTreeV1(ctx, intent.SourceRoot)
	installed, installedErr := inspectInstalledTreeV1(ctx, stagingPath)
	if sourceErr != nil || installedErr != nil || sourceBefore != sourceAfter || !identityMatchesIntent(sourceAfter, intent) || !identityMatchesIntent(installed, intent) {
		return errors.Join(pluginport.ErrCorrupt, errors.New("bundled plugin full-tree revalidation failed"), sourceErr, installedErr)
	}
	return nil
}

func (store *Store) loadOrCreateReceipt(ctx context.Context, path string, intent domainplugin.IntentV1, generationID, keyID string, publicKey []byte, authority pluginport.InstallationAuthority, now time.Time) (domainplugin.ReceiptV1, error) {
	activeRelative := activeRelativeV1(intent.PluginName, intent.PluginVersion)
	if body, err := stableReadFile(path, domainplugin.MaxContractBytesV1); err == nil {
		receipt, parseErr := domainplugin.ParseReceiptV1(body)
		if parseErr != nil || domainplugin.ValidateTrustedReceiptV1(receipt, keyID, publicKey) != nil ||
			domainplugin.ValidateReceiptForIntentV1(receipt, intent) != nil || receipt.GenerationID != generationID || receipt.ActiveRelativePath != activeRelative {
			return domainplugin.ReceiptV1{}, errors.Join(pluginport.ErrCorrupt, parseErr)
		}
		return receipt, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainplugin.ReceiptV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	receipt, err := domainplugin.NewReceiptV1(intent, generationID, activeRelative, now, keyID, publicKey, func(message []byte) ([]byte, error) {
		return authority.Sign(ctx, message)
	})
	if err != nil {
		return domainplugin.ReceiptV1{}, errors.Join(pluginport.ErrUnavailable, err)
	}
	body, _ := domainplugin.ReceiptV1Bytes(receipt)
	if err := store.putExact(path, body); err != nil {
		return domainplugin.ReceiptV1{}, err
	}
	return receipt, nil
}

func (store *Store) quarantineDiscoverable(intent domainplugin.IntentV1) error {
	pluginRoot := store.absolute(pluginParentRelativeV1(intent.PluginName))
	quarantineRoot, err := store.ensurePrivateDirectory(controlRelativeV1 + "/quarantine/" + intent.IntentID)
	if err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	// Withdraw the signed discoverability pointer before moving any active
	// generation. A crash can therefore leave either a fully discoverable old
	// generation or no discoverable generation, but never an index that points
	// at a tree already moved into quarantine.
	if _, err := os.Lstat(store.activeIndexPath()); err == nil {
		target := filepath.Join(quarantineRoot, "prior-active-index.v1.json")
		if _, targetErr := os.Lstat(target); errors.Is(targetErr, os.ErrNotExist) {
			if err := os.Rename(store.activeIndexPath(), target); err != nil {
				return errors.Join(pluginport.ErrUnavailable, err)
			}
		} else if targetErr == nil {
			return errors.Join(pluginport.ErrConflict, errors.New("bundled plugin prior active index quarantine already exists"))
		} else {
			return errors.Join(pluginport.ErrUnavailable, targetErr)
		}
		if err := syncDirectory(store.absolute(controlRelativeV1)); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
		if err := syncDirectory(quarantineRoot); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
		if err := store.inject(FaultAfterPriorIndexQuarantinedV1); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	entries, err := os.ReadDir(pluginRoot)
	if err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		digest := sha256.Sum256([]byte("analytix.bundled-plugin-quarantine/v1\x00" + entry.Name()))
		target := filepath.Join(quarantineRoot, hex.EncodeToString(digest[:]))
		source := filepath.Join(pluginRoot, entry.Name())
		if _, targetErr := os.Lstat(target); targetErr == nil {
			return errors.Join(pluginport.ErrConflict, errors.New("bundled plugin quarantine target already exists"))
		} else if !errors.Is(targetErr, os.ErrNotExist) {
			return errors.Join(pluginport.ErrUnavailable, targetErr)
		}
		if err := os.Rename(source, target); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
		if err := syncDirectory(pluginRoot); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
		if err := syncDirectory(quarantineRoot); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
		if err := store.inject(FaultAfterPriorGenerationQuarantinedV1); err != nil {
			return err
		}
	}
	if err := syncDirectory(pluginRoot); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := syncDirectory(quarantineRoot); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	return nil
}

func (store *Store) commitGeneration(ctx context.Context, stagingPath, activePath string, intent domainplugin.IntentV1) error {
	if info, err := os.Lstat(activePath); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.Join(pluginport.ErrCorrupt, errors.New("active bundled plugin path is not a real directory"))
		}
		identity, inspectErr := inspectInstalledTreeV1(ctx, activePath)
		if inspectErr != nil || !identityMatchesIntent(identity, intent) || store.validateMarkerAt(activePath, intent) != nil {
			return errors.Join(pluginport.ErrCorrupt, inspectErr)
		}
		if stagingPath != activePath {
			_ = removeOwnedStaging(stagingPath)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	identity, err := inspectInstalledTreeV1(ctx, stagingPath)
	if err != nil || !identityMatchesIntent(identity, intent) || store.validateMarkerAt(stagingPath, intent) != nil {
		return errors.Join(pluginport.ErrCorrupt, errors.New("staged bundled plugin is not authorized for commit"), err)
	}
	if err := os.Rename(stagingPath, activePath); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := syncDirectory(filepath.Dir(activePath)); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	committed, err := inspectInstalledTreeV1(ctx, activePath)
	if err != nil || !identityMatchesIntent(committed, intent) {
		return errors.Join(pluginport.ErrCorrupt, errors.New("committed bundled plugin failed readback"), err)
	}
	return nil
}

func (store *Store) publishActiveIndex(body []byte) error {
	path := store.activeIndexPath()
	if current, err := stableReadFile(path, domainplugin.MaxContractBytesV1); err == nil {
		if bytes.Equal(current, body) {
			return nil
		}
		return pluginport.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(pluginport.ErrCorrupt, err)
	}
	temporary := path + ".tmp-" + sha256HexLocal(body)
	if current, err := stableReadFile(temporary, domainplugin.MaxContractBytesV1); err == nil {
		if !bytes.Equal(current, body) {
			return pluginport.ErrConflict
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := writeExclusiveSync(temporary, body); err != nil {
			return errors.Join(pluginport.ErrUnavailable, err)
		}
	} else {
		return errors.Join(pluginport.ErrCorrupt, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	return nil
}

func (store *Store) latestJournal(transactionRoot string, intent domainplugin.IntentV1, generationID, stagingRelative string) (uint64, error) {
	var latest uint64
	for _, item := range journalPhases() {
		sequence, phase := item.sequence, item.phase
		path := filepath.Join(transactionRoot, journalFileName(sequence, phase))
		body, err := stableReadFile(path, domainplugin.MaxContractBytesV1)
		if errors.Is(err, os.ErrNotExist) {
			for later := sequence + 1; later <= 7; later++ {
				if matches, _ := filepath.Glob(filepath.Join(transactionRoot, fmt.Sprintf("%02d-*.json", later))); len(matches) != 0 {
					return 0, errors.Join(pluginport.ErrCorrupt, errors.New("bundled plugin journal contains a phase gap"))
				}
			}
			break
		}
		if err != nil {
			return 0, errors.Join(pluginport.ErrCorrupt, err)
		}
		record, err := domainplugin.ParseJournalV1(body)
		if err != nil || record.Sequence != sequence || record.Phase != phase || record.IntentID != intent.IntentID ||
			record.GenerationID != generationID || record.StagingRelativePath != stagingRelative ||
			record.ActiveRelativePath != activeRelativeV1(intent.PluginName, intent.PluginVersion) {
			return 0, errors.Join(pluginport.ErrCorrupt, err)
		}
		latest = sequence
	}
	return latest, nil
}

func (store *Store) appendJournal(transactionRoot string, intent domainplugin.IntentV1, generationID, stagingRelative string, sequence uint64, phase domainplugin.JournalPhaseV1, now time.Time) error {
	path := filepath.Join(transactionRoot, journalFileName(sequence, phase))
	if body, err := stableReadFile(path, domainplugin.MaxContractBytesV1); err == nil {
		record, parseErr := domainplugin.ParseJournalV1(body)
		if parseErr != nil || record.Sequence != sequence || record.Phase != phase || record.IntentID != intent.IntentID || record.GenerationID != generationID {
			return errors.Join(pluginport.ErrCorrupt, parseErr)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(pluginport.ErrCorrupt, err)
	}
	record, err := domainplugin.NewJournalV1(
		intent, generationID, stagingRelative, activeRelativeV1(intent.PluginName, intent.PluginVersion), sequence, phase, now,
	)
	if err != nil {
		return errors.Join(pluginport.ErrInvalid, err)
	}
	body, _ := domainplugin.JournalV1Bytes(record)
	return store.putExact(path, body)
}

func (store *Store) completeJournalBestEffort(intent domainplugin.IntentV1, generationID string, now time.Time) {
	transactionRoot := store.absolute(controlRelativeV1 + "/transactions/" + intent.IntentID)
	if _, err := os.Lstat(transactionRoot); err != nil {
		return
	}
	stagingRelative := pluginParentRelativeV1(intent.PluginName) + "/.staging-" + intent.IntentID
	latest, err := store.latestJournal(transactionRoot, intent, generationID, stagingRelative)
	if err != nil {
		return
	}
	for _, item := range journalPhases() {
		sequence, phase := item.sequence, item.phase
		if sequence > latest {
			_ = store.appendJournal(transactionRoot, intent, generationID, stagingRelative, sequence, phase, now)
		}
	}
}

type journalPhaseItem struct {
	sequence uint64
	phase    domainplugin.JournalPhaseV1
}

func journalPhases() []journalPhaseItem {
	return []journalPhaseItem{
		{1, domainplugin.JournalPreparedV1}, {2, domainplugin.JournalStagedV1}, {3, domainplugin.JournalAuthorizedV1},
		{4, domainplugin.JournalPriorQuarantinedV1}, {5, domainplugin.JournalGenerationCommittedV1},
		{6, domainplugin.JournalIndexCommittedV1}, {7, domainplugin.JournalCompletedV1},
	}
}

func journalFileName(sequence uint64, phase domainplugin.JournalPhaseV1) string {
	return fmt.Sprintf("%02d-%s.json", sequence, phase)
}

func (store *Store) putExact(path string, body []byte) error {
	if current, err := stableReadFile(path, domainplugin.MaxContractBytesV1); err == nil {
		if bytes.Equal(current, body) {
			return nil
		}
		return pluginport.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(pluginport.ErrCorrupt, err)
	}
	if err := writeExclusiveSync(path, body); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	readback, err := stableReadFile(path, domainplugin.MaxContractBytesV1)
	if err != nil || !bytes.Equal(readback, body) {
		return errors.Join(pluginport.ErrCorrupt, err)
	}
	return nil
}

func writeExclusiveSync(path string, body []byte) error {
	if len(body) == 0 || len(body) > domainplugin.MaxContractBytesV1 {
		return pluginport.ErrInvalid
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (store *Store) writeInstallMarker(root string, intent domainplugin.IntentV1) error {
	type markerV1 struct {
		ManagedBy           string `json:"managedBy"`
		MarketplaceName     string `json:"marketplaceName"`
		PluginName          string `json:"pluginName"`
		Version             string `json:"version"`
		PackageSHA256       string `json:"packageSha256"`
		SourcePath          string `json:"sourcePath"`
		SourceTreeSHA256    string `json:"sourceTreeSha256"`
		SourceTreeFileCount uint64 `json:"sourceTreeFileCount"`
		InstallType         string `json:"installType"`
	}
	marker := markerV1{
		ManagedBy: "analytix-hub", MarketplaceName: "analytix-hub", PluginName: intent.PluginName,
		Version: intent.PluginVersion, PackageSHA256: intent.PackageAuthoritySHA256, SourcePath: intent.SourceRoot,
		SourceTreeSHA256: intent.SourceTreeSHA256, SourceTreeFileCount: intent.SourceTreeFileCount, InstallType: "user",
	}
	body, _ := json.Marshal(marker)
	path := filepath.Join(root, domainplugin.InstallMarkerFileNameV1)
	if err := writeExclusiveSync(path, body); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	return syncDirectory(root)
}

func (store *Store) validateInstallMarker(receipt domainplugin.ReceiptV1) error {
	activeRoot := store.absolute(activeRelativeV1(receipt.PluginName, receipt.PluginVersion))
	return store.validateMarkerAt(activeRoot, domainplugin.IntentV1{
		SchemaVersion: domainplugin.SchemaVersionV1, Purpose: domainplugin.IntentPurposeV1, IntentID: receipt.IntentID,
		PackageAuthoritySHA256: receipt.PackageAuthoritySHA256, Target: receipt.Target,
		PluginName: receipt.PluginName, PluginVersion: receipt.PluginVersion,
		SourceRoot: markerSourcePath(activeRoot), SourceTreeSHA256: receipt.SourceTreeSHA256,
		SourceTreeFileCount: receipt.SourceTreeFileCount, ManifestSHA256: receipt.ManifestSHA256,
		EntrypointSHA256: receipt.EntrypointSHA256, RequestedAt: receipt.IssuedAt,
	})
}

func markerSourcePath(activeRoot string) string {
	body, err := stableReadFile(filepath.Join(activeRoot, domainplugin.InstallMarkerFileNameV1), domainplugin.MaxContractBytesV1)
	if err != nil {
		return ""
	}
	var value struct {
		SourcePath string `json:"sourcePath"`
	}
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	return value.SourcePath
}

func (store *Store) validateMarkerAt(root string, intent domainplugin.IntentV1) error {
	body, err := stableReadFile(filepath.Join(root, domainplugin.InstallMarkerFileNameV1), domainplugin.MaxContractBytesV1)
	if err != nil {
		return err
	}
	var marker struct {
		ManagedBy           string `json:"managedBy"`
		MarketplaceName     string `json:"marketplaceName"`
		PluginName          string `json:"pluginName"`
		Version             string `json:"version"`
		PackageSHA256       string `json:"packageSha256"`
		SourcePath          string `json:"sourcePath"`
		SourceTreeSHA256    string `json:"sourceTreeSha256"`
		SourceTreeFileCount uint64 `json:"sourceTreeFileCount"`
		InstallType         string `json:"installType"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&marker) != nil || marker.ManagedBy != "analytix-hub" || marker.MarketplaceName != "analytix-hub" ||
		marker.PluginName != intent.PluginName || marker.Version != intent.PluginVersion ||
		marker.PackageSHA256 != intent.PackageAuthoritySHA256 || marker.SourcePath != intent.SourceRoot ||
		marker.SourceTreeSHA256 != intent.SourceTreeSHA256 || marker.SourceTreeFileCount != intent.SourceTreeFileCount || marker.InstallType != "user" {
		return errors.New("bundled plugin installation marker does not match the host receipt")
	}
	canonical, _ := json.Marshal(marker)
	if !bytes.Equal(body, canonical) {
		return errors.New("bundled plugin installation marker is not canonical")
	}
	return nil
}

func (store *Store) requireSingleDiscoverableGeneration(pluginName, pluginVersion string) error {
	entries, err := os.ReadDir(store.absolute(pluginParentRelativeV1(pluginName)))
	if err != nil {
		return err
	}
	visible := make([]string, 0, 2)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() {
			return errors.New("bundled plugin cache contains a visible non-directory entry")
		}
		visible = append(visible, entry.Name())
	}
	sort.Strings(visible)
	if len(visible) != 1 || visible[0] != pluginVersion {
		return errors.New("bundled plugin cache does not contain exactly one discoverable generation")
	}
	return nil
}

func (store *Store) ensurePrivateDirectory(relative string) (string, error) {
	if !portablePath(relative) {
		return "", errors.New("bundled plugin state path is invalid")
	}
	current := store.runtimeHome
	for _, part := range strings.Split(relative, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, 0o700); err != nil {
				return "", err
			}
			if err := syncDirectory(filepath.Dir(current)); err != nil {
				return "", err
			}
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("bundled plugin state path contains a non-directory or symbolic link")
		}
	}
	return current, nil
}

func (store *Store) absolute(relative string) string {
	return filepath.Join(store.runtimeHome, filepath.FromSlash(relative))
}

func (store *Store) activeIndexPath() string {
	return filepath.Join(store.absolute(controlRelativeV1), activeIndexFileNameV1)
}

func (store *Store) inject(point FaultPointV1) error {
	if store.fault == nil {
		return nil
	}
	if err := store.fault(point); err != nil {
		return errors.Join(pluginport.ErrUnavailable, err)
	}
	return nil
}

func validateAuthority(authority pluginport.InstallationAuthority) (string, []byte, error) {
	if authority == nil {
		return "", nil, pluginport.ErrUnavailable
	}
	keyID := authority.KeyID()
	publicKey := authority.PublicKey()
	if len(publicKey) != ed25519.PublicKeySize || keyID != sha256HexLocal(publicKey) {
		return "", nil, pluginport.ErrUnavailable
	}
	return keyID, append([]byte(nil), publicKey...), nil
}

func generationIDV1(intent domainplugin.IntentV1, keyID string) string {
	digest := sha256.Sum256([]byte("analytix.bundled-plugin-materialization-generation/v1\x00" + intent.IntentID + "\x00" + keyID))
	return hex.EncodeToString(digest[:])
}

func identityMatchesIntent(identity SourceTreeIdentityV1, intent domainplugin.IntentV1) bool {
	return identity.TreeSHA256 == intent.SourceTreeSHA256 && identity.FileCount == intent.SourceTreeFileCount &&
		identity.Declaration.PackageID == intent.PluginName && identity.Declaration.PackageVersion == intent.PluginVersion &&
		identity.ManifestSHA256 == intent.ManifestSHA256 && identity.EntrypointSHA256 == intent.EntrypointSHA256
}

func mustIntentBytes(intent domainplugin.IntentV1) []byte {
	body, _ := domainplugin.IntentV1Bytes(intent)
	return body
}

func sha256HexLocal(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func removeOwnedStaging(path string) error {
	if !strings.HasPrefix(filepath.Base(path), ".staging-") {
		return errors.New("refusing to remove a non-staging bundled plugin path")
	}
	return os.RemoveAll(path)
}
