package finalauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// SecurePrivateCASOwnerLeafV1 describes one required CAS leaf in a private
// authority container. The container is either wholly absent or contains
// exactly the declared leaves.
type SecurePrivateCASOwnerLeafV1 struct {
	Name     string
	MaxBytes int
}

const maxSecurePrivateCASOwnerEntriesV1 = 64

// SecurePrivateCASRecoveryTopologyAuthorityV3 is the parent-container
// authority retained by the V3 recovery transaction. Leaf recovery plans bind
// the CAS directories themselves; this authority additionally prevents an
// attacker from replacing their parent and moving the same leaf identities
// into the replacement between the global validation barrier and mutation.
type SecurePrivateCASRecoveryTopologyAuthorityV3 interface {
	PrivateCASRecoveryTopologyRootV3() string
	RevalidatePrivateCASRecoveryTopologyV3(context.Context) error
}

// SecurePrivateCASRecoveryTopologyAuthorityV4 adds the stable, non-secret
// digest required by the signed crash-recovery journal. The digest binds the
// persistence lease, exact owner path, parent identity and complete direct
// child inventory; it deliberately excludes metadata changed by residue
// renames inside CAS leaves.
type SecurePrivateCASRecoveryTopologyAuthorityV4 interface {
	SecurePrivateCASRecoveryTopologyAuthorityV3
	PrivateCASRecoveryTopologyDigestV4() string
}

// PreparedSecurePrivateCASOwnerTopologyV1 freezes only an exact owner
// container. It is used when an owner has non-CAS migration directories next
// to CAS leaves, so those directories can be identity-bound without being
// interpreted as CAS data.
type PreparedSecurePrivateCASOwnerTopologyV1 struct {
	container *preparedSecurePrivateCASOwnerContainerV1
}

// SecurePrivateCASOwnerRegularSiblingV2 describes an exact non-CAS regular
// file retained beside owner CAS leaves. It grants no mutation authority; the
// owner topology binds its identity and exact byte length across recovery.
type SecurePrivateCASOwnerRegularSiblingV2 struct {
	Name       string
	ByteLength int64
}

// SecurePrivateCASOwnerEntryV2 is a physically bound direct-child
// observation. It carries no semantic authority; the returned owner topology
// prevents name, kind, identity, link, size, and parent/root drift while each
// observation rechecks the required ownership and security policy.
type SecurePrivateCASOwnerEntryV2 struct {
	Name       string
	Directory  bool
	ByteLength int64
}

type securePrivateCASOwnerDiscoveryV2 struct {
	mutableDirectories map[string]struct{}
	maxEntries         int
	maxRegularBytes    int64
}

// PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2 keeps filesystem
// discovery in the private-CAS owner. Every direct and immutable descendant
// is enumerated and reopened relative to the retained owner handle. Mutable
// CAS leaves are identity-bound at their root and remain owned by their exact
// leaf recovery plans; no path walk grants authority over either family.
func PrepareSecurePrivateCASOwnerDiscoveredMixedTopologyV2(
	ctx context.Context,
	root string,
	mutableDirectoryNames []string,
	maxEntries int,
	maxRegularBytes int64,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASOwnerTopologyV1, []SecurePrivateCASOwnerEntryV2, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil || maxEntries <= 0 || maxEntries > domainstartup.MaxManagedSnapshotEntriesV1 || maxRegularBytes < 0 {
		return nil, nil, errors.New("private CAS discovered mixed owner topology configuration is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, nil, errors.New("private CAS discovered mixed owner topology root is invalid")
	}
	mutable := make(map[string]struct{}, len(mutableDirectoryNames))
	for _, rawName := range mutableDirectoryNames {
		name := strings.TrimSpace(rawName)
		folded := strings.ToLower(name)
		if name == "" || name != rawName || name == "." || name == ".." || filepath.Base(name) != name ||
			strings.ContainsAny(name, `/\\`) {
			return nil, nil, errors.New("private CAS discovered mixed owner mutable directory is invalid")
		}
		for existing := range mutable {
			if strings.ToLower(existing) == folded {
				return nil, nil, errors.New("private CAS discovered mixed owner mutable directory aliases by case")
			}
		}
		mutable[name] = struct{}{}
	}
	discovery := &securePrivateCASOwnerDiscoveryV2{
		mutableDirectories: mutable, maxEntries: maxEntries, maxRegularBytes: maxRegularBytes,
	}
	requestedRoot, err := lexicalPrivateCASRootPath(absolute)
	if err != nil {
		return nil, nil, err
	}
	var prepared *preparedSecurePrivateCASOwnerContainerV1
	var inventory []SecurePrivateCASOwnerEntryV2
	err = withExistingPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		prepared = &preparedSecurePrivateCASOwnerContainerV1{
			rootPath: requestedRoot, access: access, binding: binding, gate: gate,
			present: present, authority: authority, discovery: cloneSecurePrivateCASOwnerDiscoveryV2(discovery),
		}
		if !present {
			return nil
		}
		first, firstInventory, err := discoverSecurePrivateCASOwnerContainerV2(authority, discovery)
		if err != nil {
			return err
		}
		second, secondInventory, err := discoverSecurePrivateCASOwnerContainerV2(authority, discovery)
		if err != nil || first != second || !sameSecurePrivateCASOwnerInventoryV2(firstInventory, secondInventory) {
			return errors.Join(errors.New("private CAS discovered mixed owner changed during preparation"), err)
		}
		prepared.fingerprint = second.fingerprint
		prepared.durableFingerprint = second.durableFingerprint
		prepared.originalRootMode = second.originalRootMode
		prepared.expected = make([]string, len(secondInventory))
		for index, entry := range secondInventory {
			prepared.expected[index] = entry.Name
		}
		inventory = append([]SecurePrivateCASOwnerEntryV2(nil), secondInventory...)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if prepared == nil {
		return nil, nil, errors.New("private CAS discovered mixed owner was not bound")
	}
	topology := &PreparedSecurePrivateCASOwnerTopologyV1{container: prepared}
	if err := topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return nil, nil, err
	}
	return topology, inventory, nil
}

func cloneSecurePrivateCASOwnerDiscoveryV2(input *securePrivateCASOwnerDiscoveryV2) *securePrivateCASOwnerDiscoveryV2 {
	if input == nil {
		return nil
	}
	cloned := &securePrivateCASOwnerDiscoveryV2{
		mutableDirectories: make(map[string]struct{}, len(input.mutableDirectories)),
		maxEntries:         input.maxEntries, maxRegularBytes: input.maxRegularBytes,
	}
	for name := range input.mutableDirectories {
		cloned.mutableDirectories[name] = struct{}{}
	}
	return cloned
}

func sameSecurePrivateCASOwnerInventoryV2(left, right []SecurePrivateCASOwnerEntryV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// PreparedSecurePrivateCASOwnerRecoveryV1 freezes the exact container
// topology and every leaf recovery plan. Owner-specific semantic validation
// streams committed bodies before any cleanup; the plan retains no bodies.
type PreparedSecurePrivateCASOwnerRecoveryV1 struct {
	container *preparedSecurePrivateCASOwnerContainerV1
	leaves    []preparedSecurePrivateCASOwnerLeafV1
}

type preparedSecurePrivateCASOwnerLeafV1 struct {
	name     string
	maxBytes int
	plan     *PreparedSecurePrivateCASRecoveryV1
}

type preparedSecurePrivateCASOwnerContainerV1 struct {
	rootPath           string
	access             SecurePrivateCASRecoveryAccessAuthority
	binding            privatecasport.RootBinding
	gate               chan struct{}
	present            bool
	authority          privateCASRootAuthority
	expected           []string
	regularFiles       map[string]int64
	discovery          *securePrivateCASOwnerDiscoveryV2
	fingerprint        [32]byte
	durableFingerprint [32]byte
	originalRootMode   uint32
}

type securePrivateCASOwnerContainerObservationV1 struct {
	fingerprint        [32]byte
	durableFingerprint [32]byte
	originalRootMode   uint32
}

// PrepareSecurePrivateCASOwnerTopologyV1 treats expectedNames as an
// untrusted discovery result and accepts it only after the handle-bound owner
// observer proves that it is the complete, exact directory inventory. The
// returned authority retains the parent and child identities for later V3
// transaction revalidation.
func PrepareSecurePrivateCASOwnerTopologyV1(
	ctx context.Context,
	root string,
	expectedNames []string,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASOwnerTopologyV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil || len(expectedNames) > maxSecurePrivateCASOwnerEntriesV1 {
		return nil, errors.New("private CAS owner topology configuration is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private CAS owner topology root is invalid")
	}
	normalized := make([]SecurePrivateCASOwnerLeafV1, len(expectedNames))
	seenFolded := make(map[string]struct{}, len(expectedNames))
	for index, rawName := range expectedNames {
		name := strings.TrimSpace(rawName)
		folded := strings.ToLower(name)
		if name == "" || name != rawName || name == "." || name == ".." || filepath.Base(name) != name ||
			strings.ContainsAny(name, `/\\`) {
			return nil, errors.New("private CAS owner topology entry is invalid")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return nil, errors.New("private CAS owner topology aliases an entry by case")
		}
		seenFolded[folded] = struct{}{}
		normalized[index] = SecurePrivateCASOwnerLeafV1{Name: name, MaxBytes: 1}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Name < normalized[j].Name })
	container, err := prepareSecurePrivateCASOwnerContainerV1(ctx, absolute, normalized, nil, access)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedSecurePrivateCASOwnerTopologyV1{container: container}
	if err := prepared.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

// PrepareSecurePrivateCASOwnerMixedTopologyV2 preserves the V1 directory-only
// contract while allowing an owner to bind an explicit, fixed set of regular
// non-CAS siblings. Both name discovery and kind claims remain untrusted until
// the handle-relative observer validates the complete inventory twice.
func PrepareSecurePrivateCASOwnerMixedTopologyV2(
	ctx context.Context,
	root string,
	directoryNames []string,
	regularFiles []SecurePrivateCASOwnerRegularSiblingV2,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASOwnerTopologyV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil || len(directoryNames)+len(regularFiles) > maxSecurePrivateCASOwnerEntriesV1 {
		return nil, errors.New("private CAS mixed owner topology configuration is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private CAS mixed owner topology root is invalid")
	}
	normalized := make([]SecurePrivateCASOwnerLeafV1, 0, len(directoryNames)+len(regularFiles))
	seenFolded := make(map[string]struct{}, len(directoryNames)+len(regularFiles))
	regularByName := make(map[string]int64, len(regularFiles))
	appendName := func(rawName string, byteLength *int64) error {
		name := strings.TrimSpace(rawName)
		folded := strings.ToLower(name)
		if name == "" || name != rawName || name == "." || name == ".." || filepath.Base(name) != name ||
			strings.ContainsAny(name, `/\\`) {
			return errors.New("private CAS mixed owner topology entry is invalid")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return errors.New("private CAS mixed owner topology aliases an entry by case")
		}
		seenFolded[folded] = struct{}{}
		normalized = append(normalized, SecurePrivateCASOwnerLeafV1{Name: name, MaxBytes: 1})
		if byteLength != nil {
			if *byteLength < 0 {
				return errors.New("private CAS mixed owner topology regular file length is invalid")
			}
			regularByName[name] = *byteLength
		}
		return nil
	}
	for _, name := range directoryNames {
		if err := appendName(name, nil); err != nil {
			return nil, err
		}
	}
	for _, file := range regularFiles {
		length := file.ByteLength
		if err := appendName(file.Name, &length); err != nil {
			return nil, err
		}
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Name < normalized[j].Name })
	container, err := prepareSecurePrivateCASOwnerContainerV1(ctx, absolute, normalized, regularByName, access)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedSecurePrivateCASOwnerTopologyV1{container: container}
	if err := prepared.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

// PrepareSecurePrivateCASOwnerRecoveryV1 is the shared owner-level topology
// coordinator. It reuses the one production CAS recovery primitive; it does
// not discover or mutate residue itself.
func PrepareSecurePrivateCASOwnerRecoveryV1(
	ctx context.Context,
	root string,
	leaves []SecurePrivateCASOwnerLeafV1,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedSecurePrivateCASOwnerRecoveryV1, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil || len(leaves) == 0 || len(leaves) > maxSecurePrivateCASOwnerEntriesV1 {
		return nil, errors.New("private CAS owner recovery configuration is invalid")
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		return nil, errors.New("private CAS owner recovery root is invalid")
	}
	normalized := make([]SecurePrivateCASOwnerLeafV1, len(leaves))
	copy(normalized, leaves)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Name < normalized[j].Name })
	seenFolded := make(map[string]struct{}, len(normalized))
	for index, leaf := range normalized {
		name := strings.TrimSpace(leaf.Name)
		folded := strings.ToLower(name)
		if name == "" || name != leaf.Name || name == "." || name == ".." || filepath.Base(name) != name ||
			strings.ContainsAny(name, `/\\`) || leaf.MaxBytes <= 0 || leaf.MaxBytes > maxPrivateAcceptedFinalBytes {
			return nil, errors.New("private CAS owner recovery leaf is invalid")
		}
		if _, duplicate := seenFolded[folded]; duplicate {
			return nil, errors.New("private CAS owner recovery aliases a leaf by case")
		}
		seenFolded[folded] = struct{}{}
		normalized[index].Name = name
	}
	container, err := prepareSecurePrivateCASOwnerContainerV1(ctx, absolute, normalized, nil, access)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedSecurePrivateCASOwnerRecoveryV1{
		container: container,
		leaves:    make([]preparedSecurePrivateCASOwnerLeafV1, 0, len(normalized)),
	}
	for _, leaf := range normalized {
		var plan *PreparedSecurePrivateCASRecoveryV1
		if container.present {
			plan, err = PrepareSecurePrivateCASRecoveryIfPresent(
				ctx, filepath.Join(absolute, leaf.Name), leaf.MaxBytes, access,
			)
			if err != nil {
				return nil, err
			}
		} else {
			plan, err = prepareAbsentSecurePrivateCASOwnerLeafV1(container, leaf)
			if err != nil {
				return nil, err
			}
		}
		if plan.Present() != container.present {
			return nil, errors.New("private CAS owner recovery topology is incomplete")
		}
		prepared.leaves = append(prepared.leaves, preparedSecurePrivateCASOwnerLeafV1{
			name: leaf.Name, maxBytes: leaf.MaxBytes, plan: plan,
		})
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

// A handle-relative proof that the exact owner container is absent also
// proves every declared direct child absent. Retain an exact leaf binding for
// later direct use without repeating one filesystem traversal per leaf during
// the runtime-wide missing-owner startup path.
func prepareAbsentSecurePrivateCASOwnerLeafV1(
	container *preparedSecurePrivateCASOwnerContainerV1,
	leaf SecurePrivateCASOwnerLeafV1,
) (*PreparedSecurePrivateCASRecoveryV1, error) {
	if container == nil || container.present || container.access == nil || container.gate == nil {
		return nil, errors.New("absent private CAS owner leaf authority is invalid")
	}
	rootPath := filepath.Join(container.rootPath, leaf.Name)
	binding := container.binding
	binding.RelativePath = filepath.Join(binding.RelativePath, leaf.Name)
	if err := validatePrivateCASRootBinding(rootPath, binding); err != nil {
		return nil, err
	}
	gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
	if gate != container.gate {
		return nil, errors.New("absent private CAS owner leaf gate changed")
	}
	return &PreparedSecurePrivateCASRecoveryV1{
		rootPath: rootPath,
		maxBytes: leaf.MaxBytes,
		access:   container.access,
		binding:  binding,
		gate:     gate,
	}, nil
}

func prepareSecurePrivateCASOwnerContainerV1(
	ctx context.Context,
	root string,
	leaves []SecurePrivateCASOwnerLeafV1,
	regularFiles map[string]int64,
	access SecurePrivateCASRecoveryAccessAuthority,
) (*preparedSecurePrivateCASOwnerContainerV1, error) {
	requestedRoot, err := lexicalPrivateCASRootPath(root)
	if err != nil {
		return nil, err
	}
	expected := make([]string, len(leaves))
	for index, leaf := range leaves {
		expected[index] = leaf.Name
	}
	var prepared *preparedSecurePrivateCASOwnerContainerV1
	err = withExistingPrivateCASAccess(ctx, access, requestedRoot, func(binding privatecasport.RootBinding) error {
		gate := privateCASProcessGates[privateCASAccessGateIndex(binding)%uint8(len(privateCASProcessGates))]
		if err := acquirePrivateCASGate(ctx, gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil {
			return err
		}
		prepared = &preparedSecurePrivateCASOwnerContainerV1{
			rootPath: requestedRoot, access: access, binding: binding, gate: gate,
			present: present, authority: authority, expected: append([]string(nil), expected...),
			regularFiles: cloneSecurePrivateCASOwnerRegularFilesV2(regularFiles),
		}
		if !present {
			return nil
		}
		first, err := observeSecurePrivateCASOwnerContainerV1(authority, expected, regularFiles)
		if err != nil {
			return err
		}
		second, err := observeSecurePrivateCASOwnerContainerV1(authority, expected, regularFiles)
		if err != nil || first != second {
			return errors.Join(errors.New("private CAS owner container changed during preparation"), err)
		}
		prepared.fingerprint = second.fingerprint
		prepared.durableFingerprint = second.durableFingerprint
		prepared.originalRootMode = second.originalRootMode
		return nil
	})
	if err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, errors.New("private CAS owner container was not bound")
	}
	return prepared, nil
}

func cloneSecurePrivateCASOwnerRegularFilesV2(input map[string]int64) map[string]int64 {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]int64, len(input))
	for name, size := range input {
		cloned[name] = size
	}
	return cloned
}

func (prepared *preparedSecurePrivateCASOwnerContainerV1) revalidate(ctx context.Context) error {
	if prepared == nil || prepared.access == nil || prepared.gate == nil || prepared.rootPath == "" {
		return errors.New("private CAS owner container recovery is invalid")
	}
	return withExistingPrivateCASAccess(ctx, prepared.access, prepared.rootPath, func(binding privatecasport.RootBinding) error {
		if binding != prepared.binding {
			return errors.New("private CAS owner container binding changed")
		}
		if err := acquirePrivateCASGate(ctx, prepared.gate); err != nil {
			return err
		}
		defer releasePrivateCASGate(prepared.gate)
		authority, present, err := existingPrivateCASRootAuthority(binding)
		if err != nil || present != prepared.present {
			return errors.Join(errors.New("private CAS owner container presence changed"), err)
		}
		if !present {
			return nil
		}
		if authority != prepared.authority {
			return errors.New("private CAS owner container identity changed")
		}
		var observation securePrivateCASOwnerContainerObservationV1
		if prepared.discovery != nil {
			observation, _, err = discoverSecurePrivateCASOwnerContainerV2(authority, prepared.discovery)
		} else {
			observation, err = observeSecurePrivateCASOwnerContainerV1(authority, prepared.expected, prepared.regularFiles)
		}
		if err != nil || observation.fingerprint != prepared.fingerprint ||
			observation.durableFingerprint != prepared.durableFingerprint {
			return errors.Join(errors.New("private CAS owner container topology changed"), err)
		}
		return nil
	})
}

func (prepared *PreparedSecurePrivateCASOwnerTopologyV1) PrivateCASRecoveryTopologyRootV3() string {
	if prepared == nil || prepared.container == nil {
		return ""
	}
	return prepared.container.rootPath
}

// ContainsEntryV1 reports membership in the frozen, handle-verified owner
// inventory. Callers use it to prove that an optional CAS leaf plan's presence
// agrees with the exact parent topology rather than with path discovery.
func (prepared *PreparedSecurePrivateCASOwnerTopologyV1) ContainsEntryV1(name string) bool {
	if prepared == nil || prepared.container == nil {
		return false
	}
	for _, expected := range prepared.container.expected {
		if expected == name {
			return true
		}
	}
	return false
}

// PresentV1 reports whether the exact frozen owner container existed during
// preparation. It is not a discovery authority: callers must also compare the
// complete child membership they derive from their own handle-bound semantic
// inventory before accepting an optional leaf plan.
func (prepared *PreparedSecurePrivateCASOwnerTopologyV1) PresentV1() bool {
	return prepared != nil && prepared.container != nil && prepared.container.present
}

func (prepared *PreparedSecurePrivateCASOwnerTopologyV1) RevalidatePrivateCASRecoveryTopologyV3(ctx context.Context) error {
	if prepared == nil || prepared.container == nil {
		return errors.New("private CAS owner topology authority is invalid")
	}
	return prepared.container.revalidate(ctx)
}

func (prepared *PreparedSecurePrivateCASOwnerTopologyV1) PrivateCASRecoveryTopologyDigestV4() string {
	if prepared == nil {
		return ""
	}
	return securePrivateCASOwnerContainerDigestV4(prepared.container)
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) PrivateCASRecoveryTopologyRootV3() string {
	if prepared == nil || prepared.container == nil {
		return ""
	}
	return prepared.container.rootPath
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) RevalidatePrivateCASRecoveryTopologyV3(ctx context.Context) error {
	if prepared == nil || prepared.container == nil {
		return errors.New("private CAS owner recovery topology authority is invalid")
	}
	return prepared.container.revalidate(ctx)
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) PrivateCASRecoveryTopologyDigestV4() string {
	if prepared == nil {
		return ""
	}
	return securePrivateCASOwnerContainerDigestV4(prepared.container)
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) PrivateCASRecoveryTopologiesV3() []SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil {
		return nil
	}
	return []SecurePrivateCASRecoveryTopologyAuthorityV3{prepared}
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) Present() bool {
	return prepared != nil && prepared.container != nil && prepared.container.present
}

// VisitCommittedFiles streams one exact prepared leaf. Unknown leaf names are
// rejected rather than being confused with a valid empty inventory.
func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) VisitCommittedFiles(
	ctx context.Context,
	leafName string,
	visit func(SecurePrivateCASFile) error,
) error {
	if prepared == nil || visit == nil {
		return errors.New("private CAS owner recovery visitor is invalid")
	}
	for _, leaf := range prepared.leaves {
		if leaf.name == leafName {
			return leaf.plan.VisitCommittedFiles(ctx, visit)
		}
	}
	return errors.New("private CAS owner recovery leaf is unknown")
}

// VisitCommittedMaterials visits body-free frozen metadata for one exact leaf.
func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) VisitCommittedMaterials(
	ctx context.Context,
	leafName string,
	visit func(SecurePrivateCASPreparedMaterialV1) error,
) error {
	if prepared == nil || visit == nil {
		return errors.New("private CAS owner recovery material visitor is invalid")
	}
	for _, leaf := range prepared.leaves {
		if leaf.name == leafName {
			return leaf.plan.VisitCommittedMaterials(ctx, visit)
		}
	}
	return errors.New("private CAS owner recovery leaf is unknown")
}

// SecurePrivateCASRecoveryPlansV2 returns the exact frozen leaf plans for the
// runtime-wide quarantine transaction. The slice is defensive; the plans
// remain owned by this prepared semantic recovery object.
func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil {
		return nil
	}
	plans := make([]*PreparedSecurePrivateCASRecoveryV1, 0, len(prepared.leaves))
	for _, leaf := range prepared.leaves {
		plans = append(plans, leaf.plan)
	}
	return plans
}

func (prepared *PreparedSecurePrivateCASOwnerRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.container == nil || len(prepared.leaves) == 0 {
		return errors.New("private CAS owner recovery plan is invalid")
	}
	if !prepared.container.present {
		if len(prepared.leaves) != len(prepared.container.expected) {
			return errors.New("private CAS owner recovery topology changed")
		}
		for index, leaf := range prepared.leaves {
			if err := revalidateAbsentSecurePrivateCASOwnerLeafV1(
				prepared.container, prepared.container.expected[index], leaf,
			); err != nil {
				return err
			}
		}
		return prepared.container.revalidate(ctx)
	}
	if err := prepared.container.revalidate(ctx); err != nil {
		return err
	}
	for _, leaf := range prepared.leaves {
		if err := leaf.plan.Revalidate(ctx); err != nil {
			return err
		}
		if leaf.plan.Present() != prepared.container.present {
			return errors.New("private CAS owner recovery topology changed")
		}
	}
	return prepared.container.revalidate(ctx)
}

func revalidateAbsentSecurePrivateCASOwnerLeafV1(
	container *preparedSecurePrivateCASOwnerContainerV1,
	expectedName string,
	leaf preparedSecurePrivateCASOwnerLeafV1,
) error {
	if container == nil || container.present || leaf.plan == nil || leaf.name != expectedName ||
		leaf.maxBytes <= 0 || leaf.plan.maxBytes != leaf.maxBytes || leaf.plan.access == nil ||
		leaf.plan.present || leaf.plan.rootPath != filepath.Join(container.rootPath, expectedName) {
		return errors.New("absent private CAS owner leaf plan changed")
	}
	binding := container.binding
	binding.RelativePath = filepath.Join(binding.RelativePath, expectedName)
	if leaf.plan.binding != binding || leaf.plan.gate != container.gate {
		return errors.New("absent private CAS owner leaf authority changed")
	}
	return validatePrivateCASRootBinding(leaf.plan.rootPath, leaf.plan.binding)
}

func securePrivateCASOwnerContainerDigestV4(container *preparedSecurePrivateCASOwnerContainerV1) string {
	if container == nil || container.rootPath == "" {
		return ""
	}
	hasher := sha256.New()
	privateCASWriteFingerprintField(hasher, []byte("analytix.private-cas-recovery-owner-topology/v4"))
	privateCASWriteFingerprintField(hasher, []byte(container.rootPath))
	privateCASWriteFingerprintField(hasher, []byte(container.binding.RootPath))
	privateCASWriteFingerprintField(hasher, []byte(container.binding.RelativePath))
	privateCASWriteFingerprintField(hasher, []byte(container.binding.RootIdentity.Kind))
	var number [8]byte
	binary.BigEndian.PutUint64(number[:], container.binding.RootIdentity.Device)
	privateCASWriteFingerprintField(hasher, number[:])
	binary.BigEndian.PutUint64(number[:], container.binding.RootIdentity.Inode)
	privateCASWriteFingerprintField(hasher, number[:])
	binary.BigEndian.PutUint64(number[:], container.binding.RootIdentity.VolumeSerial)
	privateCASWriteFingerprintField(hasher, number[:])
	privateCASWriteFingerprintField(hasher, container.binding.RootIdentity.FileID[:])
	if container.present {
		privateCASWriteFingerprintField(hasher, []byte{1})
	} else {
		privateCASWriteFingerprintField(hasher, []byte{0})
	}
	privateCASWriteFingerprintField(hasher, container.durableFingerprint[:])
	return hex.EncodeToString(hasher.Sum(nil))
}
