package persistencefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// RootAuthority freezes the filesystem identity that gave meaning to each
// configured persistence root. Existing roots bind their own identity. A root
// that does not exist yet binds the nearest existing ancestor and its exact
// relative suffix, so cold-start creation cannot silently cross an ancestor
// replacement.
type RootAuthority struct {
	mu           sync.Mutex
	roots        RootSet
	dataKey      string
	durableKey   string
	capabilities map[string]frozenRootCapability
	strongRoots  map[string]privatecasport.DirectoryIdentity
	digest       string
}

type frozenRootCapability struct {
	Root           string `json:"root"`
	Anchor         string `json:"anchor"`
	AnchorIdentity string `json:"anchorIdentity"`
	RootIdentity   string `json:"rootIdentity,omitempty"`
	MissingSuffix  string `json:"missingSuffix,omitempty"`
}

func FreezeRootAuthority(roots RootSet) (*RootAuthority, error) {
	resolved, err := ResolveRootSet(roots.DataDir, roots.DurableDir)
	if err != nil {
		return nil, err
	}
	authority := &RootAuthority{
		roots: resolved, dataKey: canonicalPathKey(resolved.DataDir), durableKey: canonicalPathKey(resolved.DurableDir),
		capabilities: map[string]frozenRootCapability{},
		strongRoots:  map[string]privatecasport.DirectoryIdentity{},
	}
	for _, root := range CanonicalRoots(resolved) {
		capability, err := freezeOneRootCapability(root)
		if err != nil {
			return nil, err
		}
		key := canonicalPathKey(root)
		authority.capabilities[key] = capability
		if capability.RootIdentity != "" {
			recorded, parseErr := parseStrongDirectoryIdentity(capability.RootIdentity)
			strong, legacy, err := platformStrongDirectoryIdentity(root)
			if parseErr != nil || err != nil || legacy != capability.RootIdentity || strong != recorded {
				return nil, errors.New("persistence root strong identity is unavailable")
			}
			authority.strongRoots[key] = recorded
		}
	}
	authority.digest = rootCapabilityDigest(authority.capabilities)
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	return authority, nil
}

func freezeOneRootCapability(root string) (frozenRootCapability, error) {
	root = filepath.Clean(root)
	current := root
	missing := make([]string, 0, 4)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return frozenRootCapability{}, errors.New("persistence root authority crossed an unsafe ancestor")
			}
			identity, err := platformDirectoryIdentity(current)
			if err != nil {
				return frozenRootCapability{}, err
			}
			rootIdentity := ""
			if canonicalPathsEqual(current, root) {
				rootIdentity = identity
			}
			for left, right := 0, len(missing)-1; left < right; left, right = left+1, right-1 {
				missing[left], missing[right] = missing[right], missing[left]
			}
			return frozenRootCapability{
				Root: root, Anchor: current, AnchorIdentity: identity, RootIdentity: rootIdentity,
				MissingSuffix: filepath.Join(missing...),
			}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return frozenRootCapability{}, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return frozenRootCapability{}, errors.New("persistence root authority has no existing ancestor")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (authority *RootAuthority) Roots() (RootSet, bool) {
	if authority == nil {
		return RootSet{}, false
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.capabilities == nil {
		return RootSet{}, false
	}
	return authority.roots, true
}

func (authority *RootAuthority) Digest() string {
	if authority == nil {
		return ""
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.digest
}

func (authority *RootAuthority) bindings() []frozenRootCapability {
	if authority == nil {
		return nil
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	keys := make([]string, 0, len(authority.capabilities))
	for key := range authority.capabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bindings := make([]frozenRootCapability, 0, len(keys))
	for _, key := range keys {
		bindings = append(bindings, authority.capabilities[key])
	}
	return bindings
}

func validateRecordedRootCapabilities(roots RootSet, capabilities []frozenRootCapability, expectedDigest string) error {
	if err := validateRootCapabilityInventory(roots, capabilities, expectedDigest); err != nil {
		return err
	}
	for _, capability := range capabilities {
		if err := validateFrozenRootCapability(capability); err != nil {
			return errors.New("startup journal root capability no longer matches this installation")
		}
		if capability.RootIdentity == "" {
			if _, err := os.Lstat(capability.Root); err == nil {
				return errors.New("startup journal cold root appeared without a signed identity promotion")
			} else if !errors.Is(err, os.ErrNotExist) {
				return errors.New("startup journal cold root state is ambiguous")
			}
		}
	}
	return nil
}

func validateRootCapabilityInventory(roots RootSet, capabilities []frozenRootCapability, expectedDigest string) error {
	if len(capabilities) != len(CanonicalRoots(roots)) {
		return errors.New("startup journal root capability inventory is incomplete")
	}
	indexed := make(map[string]frozenRootCapability, len(capabilities))
	for _, capability := range capabilities {
		key := canonicalPathKey(capability.Root)
		if _, duplicate := indexed[key]; duplicate {
			return errors.New("startup journal root capability inventory is ambiguous")
		}
		indexed[key] = capability
	}
	for _, root := range CanonicalRoots(roots) {
		capability, ok := indexed[canonicalPathKey(root)]
		if !ok || capability.Root != root || !filepath.IsAbs(capability.Root) || !filepath.IsAbs(capability.Anchor) ||
			strings.TrimSpace(capability.AnchorIdentity) == "" || capability.RootIdentity == "" && strings.TrimSpace(capability.MissingSuffix) == "" ||
			capability.RootIdentity != "" && strings.TrimSpace(capability.MissingSuffix) != "" {
			return errors.New("startup journal root capability no longer matches this installation")
		}
		if relative, err := filepath.Rel(capability.Anchor, capability.Root); err != nil || relative == "." && capability.RootIdentity == "" ||
			capability.RootIdentity == "" && filepath.Clean(relative) != filepath.Clean(capability.MissingSuffix) ||
			filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("startup journal root capability path binding is invalid")
		}
		if _, err := parseStrongDirectoryIdentity(capability.AnchorIdentity); err != nil {
			return errors.New("startup journal root anchor identity is invalid")
		}
		if capability.RootIdentity != "" {
			if _, err := parseStrongDirectoryIdentity(capability.RootIdentity); err != nil {
				return errors.New("startup journal root identity is invalid")
			}
		}
	}
	if rootCapabilityDigest(indexed) != expectedDigest {
		return errors.New("startup journal root capability digest is invalid")
	}
	return nil
}

func rootPromotionIntents(capabilities []frozenRootCapability) []rootPromotionIntentV1 {
	intents := make([]rootPromotionIntentV1, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability.RootIdentity != "" {
			continue
		}
		intent := rootPromotionIntentV1{
			Root: capability.Root, Anchor: capability.Anchor, AnchorIdentity: capability.AnchorIdentity, MissingSuffix: capability.MissingSuffix,
		}
		intent.IntentDigest = rootPromotionIntentDigest(intent)
		intents = append(intents, intent)
	}
	sort.Slice(intents, func(left, right int) bool { return intents[left].Root < intents[right].Root })
	return intents
}

func rootPromotionIntentDigest(intent rootPromotionIntentV1) string {
	body, _ := json.Marshal(struct {
		Root           string `json:"root"`
		Anchor         string `json:"anchor"`
		AnchorIdentity string `json:"anchorIdentity"`
		MissingSuffix  string `json:"missingSuffix"`
	}{intent.Root, intent.Anchor, intent.AnchorIdentity, intent.MissingSuffix})
	return domainsecurity.SHA256Hex(body)
}

func validateRootPromotionIntents(capabilities []frozenRootCapability, intents []rootPromotionIntentV1) error {
	expected := rootPromotionIntents(capabilities)
	if len(expected) != len(intents) {
		return errors.New("startup journal root promotion intent inventory is incomplete")
	}
	for index := range expected {
		if intents[index] != expected[index] || !domainsecurity.IsSHA256Hex(intents[index].IntentDigest) || intents[index].IntentDigest != rootPromotionIntentDigest(intents[index]) {
			return errors.New("startup journal root promotion intent is invalid")
		}
	}
	return nil
}

func restoreRootAuthorityFromJournal(roots RootSet, journal semanticJournalV1) (*RootAuthority, error) {
	if err := validateRootCapabilityInventory(roots, journal.RootCapabilities, journal.RootCapabilityDigest); err != nil ||
		validateRootPromotionIntents(journal.RootCapabilities, journal.RootPromotionIntents) != nil {
		return nil, errors.New("startup journal root authority inventory is invalid")
	}
	capabilities := make(map[string]frozenRootCapability, len(journal.RootCapabilities))
	strongRoots := make(map[string]privatecasport.DirectoryIdentity, len(journal.RootCapabilities))
	for _, recorded := range journal.RootCapabilities {
		capability := recorded
		anchorIdentity, err := platformDirectoryIdentity(capability.Anchor)
		if err != nil || anchorIdentity != capability.AnchorIdentity {
			return nil, errors.New("startup journal root ancestor identity changed")
		}
		if capability.RootIdentity == "" {
			if _, err := os.Lstat(capability.Root); errors.Is(err, os.ErrNotExist) {
				capabilities[canonicalPathKey(capability.Root)] = capability
				continue
			} else if err != nil {
				return nil, errors.New("startup journal cold root state is ambiguous")
			}
			// A signed creation intent binds the ancestor and path, not the inode
			// created after the last durable journal write. Accepting an empty root
			// here would let a replacement directory inherit authority after a
			// crash. The live apply path signs the created inode into the journal
			// before any managed write; a crash before that promotion is quarantined.
			return nil, errors.New("startup journal cold root appeared without a signed inode promotion")
		} else if err := validateFrozenRootCapability(capability); err != nil {
			return nil, err
		}
		key := canonicalPathKey(capability.Root)
		capabilities[key] = capability
		recorded, parseErr := parseStrongDirectoryIdentity(capability.RootIdentity)
		strong, canonical, err := platformStrongDirectoryIdentity(capability.Root)
		if parseErr != nil || err != nil || canonical != capability.RootIdentity || strong != recorded {
			return nil, errors.New("startup journal root strong identity is unavailable")
		}
		strongRoots[key] = recorded
	}
	authority := &RootAuthority{
		roots: roots, dataKey: canonicalPathKey(roots.DataDir), durableKey: canonicalPathKey(roots.DurableDir),
		capabilities: capabilities, strongRoots: strongRoots,
	}
	authority.digest = rootCapabilityDigest(capabilities)
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	return authority, nil
}

func (authority *RootAuthority) Validate() error {
	if authority == nil {
		return errors.New("persistence root authority is unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.capabilities == nil || authority.strongRoots == nil || authority.digest == "" || rootCapabilityDigest(authority.capabilities) != authority.digest {
		return errors.New("persistence root authority is invalid")
	}
	for root, key := range map[string]string{
		authority.roots.DataDir: authority.dataKey, authority.roots.DurableDir: authority.durableKey,
	} {
		if root == "" {
			continue
		}
		if key == "" {
			return errors.New("persistence root authority key is invalid")
		}
		if _, ok := authority.capabilities[key]; !ok {
			return errors.New("persistence root authority key is outside its capability set")
		}
	}
	for key, capability := range authority.capabilities {
		strong, present := authority.strongRoots[key]
		if err := validateFrozenRootCapabilityWithStrongIdentity(
			capability,
			strong,
			present,
			platformStrongDirectoryIdentity,
		); err != nil {
			return err
		}
	}
	return nil
}

type strongDirectoryIdentityProbe func(string) (privatecasport.DirectoryIdentity, string, error)

func validateFrozenRootCapabilityWithStrongIdentity(
	capability frozenRootCapability,
	expectedStrong privatecasport.DirectoryIdentity,
	strongPresent bool,
	probe strongDirectoryIdentityProbe,
) error {
	if capability.Root == "" || capability.Anchor == "" || capability.AnchorIdentity == "" || probe == nil {
		return errors.New("persistence root capability is incomplete")
	}
	anchorStrong, anchorIdentity, err := probe(capability.Anchor)
	if err != nil || anchorIdentity != capability.AnchorIdentity {
		return errors.New("persistence root ancestor identity changed after authority freeze")
	}
	if capability.RootIdentity == "" {
		if strongPresent {
			return errors.New("cold persistence root has an unexpected strong identity")
		}
		if _, err := os.Lstat(capability.Root); err == nil {
			return errors.New("persistence root appeared after authority freeze")
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("persistence root state is ambiguous")
		}
		return nil
	}
	current := anchorStrong
	rootIdentity := anchorIdentity
	if !canonicalPathsEqualForDevice(capability.Anchor, capability.Root, anchorStrong.Device) {
		current, rootIdentity, err = probe(capability.Root)
		if err != nil {
			return errors.New("persistence root identity changed after authority freeze")
		}
	}
	if rootIdentity != capability.RootIdentity {
		return errors.New("persistence root identity changed after authority freeze")
	}
	if !strongPresent || current != expectedStrong {
		return errors.New("persistence root strong identity changed after authority freeze")
	}
	return nil
}

func (authority *RootAuthority) validateRoot(root string) error {
	if authority == nil {
		return errors.New("persistence root authority is unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	capability, ok := authority.capabilities[canonicalPathKey(root)]
	if !ok {
		return errors.New("persistence root is outside frozen authority")
	}
	return validateFrozenRootCapability(capability)
}

func (authority *RootAuthority) verifyOpenedRoot(root, identity string) error {
	if authority == nil || strings.TrimSpace(identity) == "" {
		return errors.New("opened persistence root authority is invalid")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	capability, ok := authority.capabilities[canonicalPathKey(root)]
	if !ok {
		return errors.New("opened persistence root is outside frozen authority")
	}
	if err := validateFrozenRootCapability(capability); err != nil {
		return err
	}
	if capability.RootIdentity != "" && identity != capability.RootIdentity {
		return errors.New("persistence root identity changed after authority freeze")
	}
	return nil
}

func (authority *RootAuthority) verifyOpenedStrongRoot(
	root string,
	openedStrong privatecasport.DirectoryIdentity,
	openedLegacy string,
) error {
	if authority == nil || strings.TrimSpace(openedLegacy) == "" {
		return errors.New("opened persistence root authority is invalid")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.capabilities == nil || authority.strongRoots == nil || authority.digest == "" ||
		rootCapabilityDigest(authority.capabilities) != authority.digest {
		return errors.New("opened persistence root authority is invalid")
	}
	key := canonicalPathKey(root)
	capability, ok := authority.capabilities[key]
	expectedStrong, strongPresent := authority.strongRoots[key]
	if !ok || capability.RootIdentity == "" {
		return errors.New("opened persistence root is outside frozen authority")
	}
	return validateFrozenRootCapabilityWithStrongIdentity(
		capability,
		expectedStrong,
		strongPresent,
		func(path string) (privatecasport.DirectoryIdentity, string, error) {
			if canonicalPathsEqualForDevice(path, root, openedStrong.Device) {
				return openedStrong, openedLegacy, nil
			}
			return platformStrongDirectoryIdentity(path)
		},
	)
}

func (authority *RootAuthority) recordedRootCapability(root string) (frozenRootCapability, error) {
	if authority == nil {
		return frozenRootCapability{}, errors.New("persistence root authority is unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	capability, ok := authority.capabilities[canonicalPathKey(root)]
	if !ok || authority.strongRoots == nil || authority.digest == "" ||
		rootCapabilityDigest(authority.capabilities) != authority.digest {
		return frozenRootCapability{}, errors.New("persistence root capability is unavailable")
	}
	return capability, nil
}

func (authority *RootAuthority) rootCapability(root string) (frozenRootCapability, error) {
	if authority == nil {
		return frozenRootCapability{}, errors.New("persistence root authority is unavailable")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	capability, ok := authority.capabilities[canonicalPathKey(root)]
	if !ok || validateFrozenRootCapability(capability) != nil {
		return frozenRootCapability{}, errors.New("persistence root capability is unavailable")
	}
	return capability, nil
}

func (authority *RootAuthority) pinCreatedRoot(root, identity string) error {
	if authority == nil || strings.TrimSpace(identity) == "" {
		return errors.New("created persistence root identity is invalid")
	}
	strong, legacy, err := platformStrongDirectoryIdentity(root)
	if err != nil || legacy != identity {
		return errors.New("created persistence root strong identity is invalid")
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	key := canonicalPathKey(root)
	capability, ok := authority.capabilities[key]
	if !ok || capability.RootIdentity != "" {
		return errors.New("created persistence root cannot be pinned")
	}
	anchorIdentity, err := platformDirectoryIdentity(capability.Anchor)
	if err != nil || anchorIdentity != capability.AnchorIdentity {
		return errors.New("created persistence root ancestor changed before pin")
	}
	capability.RootIdentity = identity
	capability.MissingSuffix = ""
	authority.capabilities[key] = capability
	authority.strongRoots[key] = strong
	authority.digest = rootCapabilityDigest(authority.capabilities)
	return validateFrozenRootCapability(capability)
}

func validateFrozenRootCapability(capability frozenRootCapability) error {
	if capability.Root == "" || capability.Anchor == "" || capability.AnchorIdentity == "" {
		return errors.New("persistence root capability is incomplete")
	}
	anchorIdentity, err := platformDirectoryIdentity(capability.Anchor)
	if err != nil || anchorIdentity != capability.AnchorIdentity {
		return errors.New("persistence root ancestor identity changed after authority freeze")
	}
	if capability.RootIdentity == "" {
		if _, err := os.Lstat(capability.Root); err == nil {
			return errors.New("persistence root appeared after authority freeze")
		} else if !errors.Is(err, os.ErrNotExist) {
			return errors.New("persistence root state is ambiguous")
		}
		return nil
	}
	rootIdentity := anchorIdentity
	if !canonicalPathsEqual(capability.Anchor, capability.Root) {
		rootIdentity, err = platformDirectoryIdentity(capability.Root)
	}
	if err != nil || rootIdentity != capability.RootIdentity {
		return errors.New("persistence root identity changed after authority freeze")
	}
	return nil
}

func rootCapabilityDigest(capabilities map[string]frozenRootCapability) string {
	keys := make([]string, 0, len(capabilities))
	for key := range capabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	records := make([]frozenRootCapability, 0, len(keys))
	for _, key := range keys {
		records = append(records, capabilities[key])
	}
	body, _ := json.Marshal(records)
	return domainsecurity.SHA256Hex(body)
}
