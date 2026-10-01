package pluginmaterializationfs

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

const (
	FaultBeforeActivationCommittedV1 FaultPointV1 = "before_activation_committed"
	FaultAfterActivationCommittedV1  FaultPointV1 = "after_activation_committed"
)

// One runtime owns installation writes. This registry serializes every Store
// instance for the same canonical runtime home inside that process only. It is
// not a cross-process lock and does not replace the runtime's sole-writer rule.
var installationLocksV1 sync.Map

var _ pluginport.PackageStateStore = (*Store)(nil)

func packageStoreV1(runtimeHome, packageID string, fault FaultInjectorV1) *Store {
	mutex, _ := installationLocksV1.LoadOrStore(runtimeHome, &sync.Mutex{})
	indexRoot := controlRelativeV1
	if packageID != domainplugin.PluginNameV1 {
		indexRoot += "/packages/" + packageID
	}
	return &Store{runtimeHome: runtimeHome, packageID: packageID, indexRoot: indexRoot, fault: fault, mu: mutex.(*sync.Mutex)}
}

func validStorePackageIDV1(packageID string) bool {
	return domainpackage.ValidPackageIdentityV1(domainpackage.PackageIdentityV1{PackageID: packageID, PackageVersion: "0.0.0"})
}

// NewPackageStoreV1 extends the existing materialization owner with a scoped
// index. Funds uses its original V1 path; receipts and journals remain shared.
// This constructor creates no authority and performs no admission or activation.
func NewPackageStoreV1(runtimeHome, packageID string, fault FaultInjectorV1) (*Store, error) {
	if !validStorePackageIDV1(packageID) {
		return nil, pluginport.ErrInvalid
	}
	if packageID == domainplugin.PluginNameV1 {
		return NewStore(runtimeHome, fault)
	}
	realHome, err := canonicalDirectory(runtimeHome)
	if err != nil {
		return nil, errors.Join(pluginport.ErrInvalid, err)
	}
	store := packageStoreV1(realHome, packageID, fault)
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, relative := range []string{
		pluginCacheRelativeV1, pluginParentRelativeV1(packageID), store.indexRoot,
		controlRelativeV1 + "/transactions", controlRelativeV1 + "/receipts", controlRelativeV1 + "/quarantine",
	} {
		if _, err := store.ensurePrivateDirectory(relative); err != nil {
			return nil, errors.Join(pluginport.ErrUnavailable, err)
		}
	}
	return store, nil
}

// OpenExistingPackageStoreV1 never creates paths, records, or keys.
func OpenExistingPackageStoreV1(runtimeHome, packageID string) (*Store, error) {
	if !validStorePackageIDV1(packageID) {
		return nil, pluginport.ErrInvalid
	}
	if packageID == domainplugin.PluginNameV1 {
		return OpenExistingStoreV1(runtimeHome)
	}
	realHome, err := canonicalDirectory(runtimeHome)
	if err != nil {
		return nil, errors.Join(pluginport.ErrInvalid, err)
	}
	store := packageStoreV1(realHome, packageID, nil)
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, relative := range []string{
		pluginCacheRelativeV1, pluginParentRelativeV1(packageID), store.indexRoot, controlRelativeV1 + "/receipts",
	} {
		path := store.absolute(relative)
		if real, err := canonicalDirectory(path); err != nil || real != path {
			return nil, errors.Join(pluginport.ErrUnavailable, err)
		}
	}
	return store, nil
}

func (store *Store) quarantinePackageLegacyProjectionsV1(intent domainplugin.IntentV1) error {
	if intent.PluginName != domainplugin.PluginNameV1 {
		return nil
	}
	return store.quarantineLegacyFundsSkillProjections(intent)
}

func (store *Store) activationPathV1(generationID string) string {
	return filepath.Join(store.absolute(store.indexRoot), "activation-"+generationID+".v1.json")
}

func (store *Store) readActivationV1(receipt domainplugin.ReceiptV1, keyID string, publicKey []byte) (domainplugin.ActivationV1, error) {
	path := store.activationPathV1(receipt.GenerationID)
	if _, err := canonicalDirectory(filepath.Dir(path)); err != nil {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	body, err := stableReadFile(path, domainplugin.MaxContractBytesV1)
	if errors.Is(err, os.ErrNotExist) {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrUnavailable, pluginport.ErrNotFound)
	}
	if err != nil {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	activation, err := domainplugin.ParseActivationV1(body)
	if err != nil || domainplugin.ValidateTrustedActivationForReceiptV1(activation, receipt, keyID, publicKey) != nil {
		return domainplugin.ActivationV1{}, pluginport.ErrCorrupt
	}
	return activation, nil
}

func (store *Store) ReadActivation(ctx context.Context, authority pluginport.InstallationAuthority) (domainplugin.ActivationV1, error) {
	if store == nil || ctx == nil || ctx.Err() != nil {
		return domainplugin.ActivationV1{}, pluginport.ErrUnavailable
	}
	keyID, publicKey, err := validateAuthority(authority)
	if err != nil {
		return domainplugin.ActivationV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	current, err := store.resolveActive(ctx, keyID, publicKey)
	if err != nil {
		return domainplugin.ActivationV1{}, err
	}
	return store.readActivationV1(current.Receipt, keyID, publicKey)
}

func (store *Store) SetDesiredState(ctx context.Context, request pluginport.SetDesiredStateRequestV1, authority pluginport.InstallationAuthority, now time.Time) (domainplugin.ActivationV1, error) {
	if store == nil || ctx == nil || ctx.Err() != nil || now.IsZero() ||
		!domainplugin.IsCanonicalSHA256V1(request.GenerationID) || request.ExpectedRevision == math.MaxUint64 ||
		!domainplugin.ValidDesiredStateV1(request.DesiredState) {
		return domainplugin.ActivationV1{}, pluginport.ErrInvalid
	}
	keyID, publicKey, err := validateAuthority(authority)
	if err != nil {
		return domainplugin.ActivationV1{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	current, err := store.resolveActive(ctx, keyID, publicKey)
	if err != nil {
		return domainplugin.ActivationV1{}, err
	}
	if current.Receipt.GenerationID != request.GenerationID {
		return domainplugin.ActivationV1{}, pluginport.ErrConflict
	}
	previous, err := store.readActivationV1(current.Receipt, keyID, publicKey)
	if err != nil && !errors.Is(err, pluginport.ErrNotFound) {
		return domainplugin.ActivationV1{}, err
	}
	if previous.Revision != request.ExpectedRevision {
		return domainplugin.ActivationV1{}, pluginport.ErrConflict
	}
	activation, err := domainplugin.NewActivationV1(current.Receipt, previous.Revision+1, request.DesiredState, now, keyID, publicKey,
		func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrUnavailable, err)
	}
	body, err := domainplugin.ActivationV1Bytes(activation)
	if err != nil {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return domainplugin.ActivationV1{}, err
	}
	path := store.activationPathV1(current.Receipt.GenerationID)
	temporary := path + ".tmp-" + sha256HexLocal(body)
	if err := store.putExact(temporary, body); err != nil {
		return domainplugin.ActivationV1{}, err
	}
	if err := store.inject(FaultBeforeActivationCommittedV1); err != nil {
		return domainplugin.ActivationV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return domainplugin.ActivationV1{}, err
	}
	if err := os.Rename(temporary, path); err != nil {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrUnavailable, err)
	}
	if err := store.inject(FaultAfterActivationCommittedV1); err != nil {
		return domainplugin.ActivationV1{}, err
	}
	readback, err := stableReadFile(path, domainplugin.MaxContractBytesV1)
	if err != nil || !bytes.Equal(readback, body) {
		return domainplugin.ActivationV1{}, errors.Join(pluginport.ErrCorrupt, err)
	}
	return store.readActivationV1(current.Receipt, keyID, publicKey)
}
