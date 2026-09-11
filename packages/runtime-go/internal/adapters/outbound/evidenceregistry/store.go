package evidenceregistry

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const maxEvidenceRegistryAuthorityCapsuleBytes = 64 * 1024 * 1024

type Store struct {
	mu        sync.Mutex
	root      string
	authority finalauthorityport.Authority
	poisoned  error
	faults    *registryStoreFaultHooks
}

// EvidenceRegistryHistoricalV1Authority marks the pre-witness registry format.
// Its signed root/capsule inventory predates the separate prepared-settlement
// store and durable settlement marker. Startup may therefore retain already
// issued V1 entries after Store has revalidated their installation signature,
// exact durable context, root chain, capsule, and projection. The witnessed V2
// service deliberately does not implement this marker.
func (*Store) EvidenceRegistryHistoricalV1Authority() bool { return true }

var ErrEvidenceRegistryAuthorityCommitIndeterminate = errors.New("evidence registry authority commit is indeterminate")

type registryStoreFaultHooks struct {
	BeforeAuthorityIndexReplace  func() error
	AfterAuthorityIndexReplace   func() error
	AfterAuthorityCapsuleInstall func() error
}

// HasState is the pre-authority, read-only existence probe used only to decide
// whether a missing installation key is fatal. It does not create a lock,
// directory, migration, or seal and does not claim that any record is valid.
func HasState(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASRecoveryAccessAuthority,
) (bool, error) {
	root = strings.TrimSpace(root)
	if root == "" || access == nil {
		return false, errors.New("evidence registry root is required")
	}
	prepared, err := PrepareRecoveryV2(ctx, root, access)
	if err != nil {
		return false, err
	}
	if err := prepared.RevalidatePhysicalV2(ctx); err != nil {
		return false, err
	}
	return prepared.HasStateV2(), nil
}

func NewStore(root string, authority finalauthorityport.Authority) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" || authority == nil || !domainsecurity.IsSHA256Hex(authority.KeyID()) ||
		len(authority.PublicKey()) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(authority.PublicKey()) != authority.KeyID() {
		return nil, errors.New("evidence registry root is required")
	}
	var err error
	root, err = canonicalRegistryRootPath(root)
	if err != nil {
		return nil, err
	}
	_, beforeErr := os.Lstat(root)
	rootCreated := errors.Is(beforeErr, os.ErrNotExist)
	if beforeErr != nil && !rootCreated {
		return nil, beforeErr
	}
	if rootCreated {
		if err := validateRegistryCreationPath(root); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(root); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("evidence registry root is not a regular directory")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil || filepath.Clean(realRoot) != filepath.Clean(absRoot) {
		if err == nil {
			return nil, errors.New("evidence registry root traverses a symlink")
		}
		return nil, err
	}
	if rootCreated {
		if err := os.Chmod(realRoot, 0o700); err != nil {
			return nil, err
		}
	}
	if err := validateRegistryDirectory(realRoot); err != nil {
		return nil, err
	}
	if rootCreated {
		if err := syncRegistryDirectory(filepath.Dir(realRoot)); err != nil {
			return nil, err
		}
	}
	return &Store{root: realRoot, authority: authority}, nil
}

func (s *Store) HasRecords(ctx context.Context) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return false, err
	}
	defer unlock()
	index, err := s.readAuthorityIndexLocked()
	if errors.Is(err, os.ErrNotExist) {
		if err := s.validateRegistryInventoryLocked(nil); err != nil {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := s.validateRegistryInventoryLocked(&index); err != nil {
		return false, err
	}
	for _, entry := range index.Entries {
		capsule, err := s.readAuthorityCapsuleBlobLocked(index, entry)
		if err != nil {
			return false, err
		}
		if _, err := s.replayAuthorityCapsuleProjectionLocked(capsule.SecurityContext, s.registryPath(capsule.SecurityContext), capsule); err != nil {
			return false, err
		}
	}
	return len(index.Entries) > 0, nil
}

func (s *Store) ListRegistries(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	contextByIdentity := make(map[string]domainsecurity.TurnSecurityContext, len(contexts))
	for _, securityContext := range contexts {
		if domainsecurity.ValidateTurnSecurityContext(securityContext) != nil {
			return nil, errors.New("evidence registry inventory context is invalid")
		}
		identity := securityContext.ThreadID + "\x00" + securityContext.TurnID
		if _, duplicate := contextByIdentity[identity]; duplicate {
			return nil, errors.New("evidence registry inventory contains a duplicate turn identity")
		}
		contextByIdentity[identity] = securityContext
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	index, err := s.readAuthorityIndexLocked()
	if errors.Is(err, os.ErrNotExist) {
		if err := s.validateRegistryInventoryLocked(nil); err != nil {
			return nil, err
		}
		return []registryport.InventoryRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.validateRegistryInventoryLocked(&index); err != nil {
		return nil, err
	}
	records := make([]registryport.InventoryRecord, 0, len(index.Entries))
	for _, entry := range index.Entries {
		securityContext, ok := contextByIdentity[entry.ThreadID+"\x00"+entry.TurnID]
		if !ok || securityContext.ContextDigest != entry.ContextDigest ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
			return nil, errors.New("evidence registry is detached from durable turn authority")
		}
		capsule, err := s.readAuthorityCapsuleBlobLocked(index, entry)
		if err != nil {
			return nil, err
		}
		registry, err := s.replayAuthorityCapsuleProjectionLocked(securityContext, s.registryPath(securityContext), capsule)
		if err != nil {
			return nil, err
		}
		records = append(records, registryport.InventoryRecord{Context: securityContext, Registry: registry})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Context.ThreadID != records[j].Context.ThreadID {
			return records[i].Context.ThreadID < records[j].Context.ThreadID
		}
		return records[i].Context.TurnID < records[j].Context.TurnID
	})
	return records, nil
}

func (s *Store) CommitPrepared(ctx context.Context, input registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	if err := contextError(ctx); err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil ||
		!domainsecurity.IsHostToolCallIDV1(input.Draft.ToolCallID) {
		return domainevidence.EvidenceReceipt{}, errors.New("evidence registry issue requires current V2 case fact authority")
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	defer unlock()
	registry, err := s.replayLocked(input.Context)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	if existing, matched, err := domainevidence.MatchEvidenceReceiptRegistration(registry, input.Draft, input.CanonicalEvidence, input.SettlementProof); err != nil {
		return domainevidence.EvidenceReceipt{}, err
	} else if matched {
		return existing, nil
	}
	next, receipt, err := domainevidence.RegisterEvidenceReceipt(registry, input.Draft, input.CanonicalEvidence, input.SettlementProof, input.RegisteredAt)
	if err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	entry := next.Entries[len(next.Entries)-1]
	if err := s.appendEntryLocked(ctx, input.Context, entry); err != nil {
		return domainevidence.EvidenceReceipt{}, err
	}
	return receipt, nil
}

func (s *Store) Resolve(ctx context.Context, query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	if err := contextError(ctx); err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	defer unlock()
	registry, err := s.replayLocked(query.Context)
	if err != nil {
		return domainevidence.RegisteredEvidence{}, err
	}
	return domainevidence.VerifyEvidenceReceiptMembership(registry, query.Context, query.ReceiptID)
}

func (s *Store) Revoke(ctx context.Context, input registryport.RevokeInput) error {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(input.Context) != nil {
		return errors.New("evidence revocation requires current V2 case fact authority")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	registry, err := s.replayLocked(input.Context)
	if err != nil {
		return err
	}
	if matched, err := domainevidence.MatchEvidenceReceiptRevocation(registry, input.ReceiptID, input.ReasonCode); err != nil {
		return err
	} else if matched {
		return nil
	}
	next, err := domainevidence.RevokeEvidenceReceipt(registry, input.ReceiptID, input.ReasonCode, input.RevokedAt)
	if err != nil {
		return err
	}
	return s.appendEntryLocked(ctx, input.Context, next.Entries[len(next.Entries)-1])
}

func (s *Store) Replay(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	if err := contextError(ctx); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	defer unlock()
	return s.replayLocked(securityContext)
}

func (s *Store) ReplayAt(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, sequence uint64) (domainevidence.EvidenceReceiptRegistry, error) {
	if err := contextError(ctx); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	defer unlock()
	current, err := s.replayLocked(securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	if sequence > current.Sequence {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry historical sequence is unavailable")
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	for _, entry := range current.Entries[:sequence] {
		registry, err = domainevidence.ApplyEvidenceRegistryEntry(registry, entry)
		if err != nil {
			return domainevidence.EvidenceReceiptRegistry{}, err
		}
	}
	return registry, nil
}

func (s *Store) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, callback func(domainevidence.EvidenceReceiptRegistry) error) error {
	if callback == nil {
		return errors.New("evidence registry locked snapshot callback is required")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	registry, err := s.replayLocked(securityContext)
	if err != nil {
		return err
	}
	snapshot, err := domainevidence.ParseEvidenceReceiptRegistry(domainevidence.EvidenceReceiptRegistryRecord(registry))
	if err != nil {
		return err
	}
	return callback(snapshot)
}

func (s *Store) replayLocked(securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	return s.replayPathLocked(securityContext, s.registryPath(securityContext))
}

func (s *Store) replayPathLocked(securityContext domainsecurity.TurnSecurityContext, path string) (domainevidence.EvidenceReceiptRegistry, error) {
	empty, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	if err := validateRegistryDirectoryIfPresent(filepath.Dir(path)); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	index, capsule, err := s.currentAuthorityCapsuleLocked(securityContext)
	if errors.Is(err, os.ErrNotExist) {
		if index.SchemaVersion == 0 {
			if inventoryErr := s.validateRegistryInventoryLocked(nil); inventoryErr != nil {
				return domainevidence.EvidenceReceiptRegistry{}, inventoryErr
			}
		} else if inventoryErr := s.validateRegistryInventoryLocked(&index); inventoryErr != nil {
			return domainevidence.EvidenceReceiptRegistry{}, inventoryErr
		}
		for _, projectionPath := range []string{path, s.registryAuthoritySealPath(securityContext)} {
			if _, projectionErr := os.Lstat(projectionPath); projectionErr == nil {
				return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry projection has no signed root index authority")
			} else if !errors.Is(projectionErr, os.ErrNotExist) {
				return domainevidence.EvidenceReceiptRegistry{}, projectionErr
			}
		}
		return empty, nil
	}
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	if err := s.validateRegistryFastInventoryLocked(&index); err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	return s.replayAuthorityCapsuleProjectionLocked(securityContext, path, capsule)
}

func (s *Store) replayAuthorityCapsuleProjectionLocked(securityContext domainsecurity.TurnSecurityContext, path string, capsule domainevidence.EvidenceRegistryAuthorityCapsule) (domainevidence.EvidenceReceiptRegistry, error) {
	if !domainevidence.EvidenceReceiptRegistryMatchesContext(capsule.Registry, securityContext) || capsule.SecurityContext != securityContext {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry authority capsule context is mismatched")
	}
	if _, projectionErr := os.Lstat(s.registryAuthoritySealPath(securityContext)); projectionErr == nil {
		projected, err := s.readAuthorityCapsuleProjectionLocked(securityContext)
		if err != nil {
			return domainevidence.EvidenceReceiptRegistry{}, err
		}
		if !authorityCapsuleIsProjectionOf(capsule, projected) {
			return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry capsule projection diverges from the signed root index")
		}
	} else if !errors.Is(projectionErr, os.ErrNotExist) {
		return domainevidence.EvidenceReceiptRegistry{}, projectionErr
	}
	ledger, err := domainevidence.CanonicalEvidenceRegistryLedger(capsule.Registry)
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	projection, err := readRegistryProjection(path, len(ledger))
	if errors.Is(err, os.ErrNotExist) {
		return capsule.Registry, nil
	}
	if err != nil {
		return domainevidence.EvidenceReceiptRegistry{}, err
	}
	if len(projection) > len(ledger) || !bytes.Equal(projection, ledger[:len(projection)]) ||
		(len(projection) > 0 && projection[len(projection)-1] != '\n') {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry projection diverges from the trusted authority capsule")
	}
	return capsule.Registry, nil
}

func (s *Store) appendEntryLocked(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, entry domainevidence.EvidenceReceiptRegistryEntry) error {
	path := s.registryPath(securityContext)
	current, err := s.replayLocked(securityContext)
	if err != nil {
		return err
	}
	next, err := domainevidence.ApplyEvidenceRegistryEntry(current, entry)
	if err != nil {
		return err
	}
	var previous *domainevidence.EvidenceRegistryAuthorityIndex
	index, indexErr := s.readAuthorityIndexLocked()
	if indexErr == nil {
		if err := s.validateRegistryInventoryLocked(&index); err != nil {
			return err
		}
		previous = &index
	} else if !errors.Is(indexErr, os.ErrNotExist) {
		return indexErr
	} else if err := s.validateRegistryInventoryLocked(nil); err != nil {
		return err
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(securityContext, next, s.authority.KeyID(), s.authority.PublicKey(), func(message []byte) ([]byte, error) {
		return s.authority.Sign(ctx, message)
	})
	if err != nil {
		return err
	}
	ledger, err := domainevidence.CanonicalEvidenceRegistryLedger(next)
	if err != nil {
		return err
	}
	nextIndex, err := s.newAuthorityIndexLocked(ctx, previous, capsule)
	if err != nil {
		return err
	}
	preparedBlob, err := s.writeAuthorityCapsuleBlobLocked(capsule)
	if err != nil {
		return err
	}
	if err := s.writeAuthorityIndexLocked(nextIndex); err != nil {
		if !errors.Is(err, ErrEvidenceRegistryAuthorityCommitIndeterminate) {
			if cleanupErr := s.removeAuthorityCapsuleBlobLocked(preparedBlob); cleanupErr != nil {
				s.poisoned = errors.Join(err, errors.New("uncommitted evidence registry capsule cleanup failed"), cleanupErr)
				return s.poisoned
			}
		}
		return err
	}
	// The signed root index above is the only membership commit point. These
	// per-turn files are disposable projections and can be repaired later.
	_ = s.writeAuthorityCapsuleProjectionLocked(securityContext, capsule)
	_ = writeRegistryProjection(path, ledger)
	if previous != nil {
		if oldEntry, ok := domainevidence.EvidenceRegistryAuthorityIndexEntryForContext(*previous, securityContext); ok && oldEntry.CapsuleSHA256 != preparedBlob.CapsuleSHA256 {
			_ = s.removeAuthorityCapsuleBlobLocked(oldEntry)
		}
	}
	return nil
}

func (s *Store) registryPath(context domainsecurity.TurnSecurityContext) string {
	return filepath.Join(s.root, s.registryRelativePath(context))
}

func (s *Store) registryRelativePath(context domainsecurity.TurnSecurityContext) string {
	return filepath.Join(evidenceRegistryProjectionDirectory, domainevidence.EvidenceRegistryProjectionKey(context.ThreadID, context.TurnID)+".jsonl")
}

func (s *Store) registryAuthoritySealPath(context domainsecurity.TurnSecurityContext) string {
	return strings.TrimSuffix(s.registryPath(context), ".jsonl") + ".head.json"
}

func (s *Store) readAuthorityCapsuleProjectionLocked(securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	path := s.registryAuthoritySealPath(securityContext)
	capsule, err := s.readAuthorityCapsulePathLocked(path)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	if capsule.SecurityContext.ContextDigest != securityContext.ContextDigest || capsule.SecurityContext.ThreadID != securityContext.ThreadID ||
		capsule.SecurityContext.TurnID != securityContext.TurnID || !domainevidence.EvidenceReceiptRegistryMatchesContext(capsule.Registry, securityContext) {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule context is mismatched")
	}
	return capsule, nil
}

func (s *Store) readAuthorityCapsulePathLocked(path string) (domainevidence.EvidenceRegistryAuthorityCapsule, error) {
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	if before.Size() <= 0 || before.Size() > maxEvidenceRegistryAuthorityCapsuleBytes {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule size is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	defer file.Close()
	if err := validateOpenedRegistryFile(file, before); err != nil {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, maxEvidenceRegistryAuthorityCapsuleBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxEvidenceRegistryAuthorityCapsuleBytes {
		return domainevidence.EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule body is invalid")
	}
	return s.parseTrustedAuthorityCapsule(body)
}

func (s *Store) writeAuthorityCapsuleProjectionLocked(securityContext domainsecurity.TurnSecurityContext, capsule domainevidence.EvidenceRegistryAuthorityCapsule) error {
	if domainevidence.ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil ||
		capsule.SecurityContext.ContextDigest != securityContext.ContextDigest || !domainevidence.EvidenceReceiptRegistryMatchesContext(capsule.Registry, securityContext) {
		return errors.New("evidence registry authority capsule is invalid")
	}
	path := s.registryAuthoritySealPath(securityContext)
	dir := filepath.Dir(path)
	if err := ensureRegistryDirectory(dir); err != nil {
		return err
	}
	if before, err := validateRegistryRegularFile(path); err == nil {
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		validateErr := validateOpenedRegistryFile(file, before)
		closeErr := file.Close()
		if err := errors.Join(validateErr, closeErr); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, err := json.Marshal(capsule)
	if err != nil || len(body) == 0 || len(body) > maxEvidenceRegistryAuthorityCapsuleBytes {
		return errors.New("evidence registry authority capsule encoding is invalid")
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(body); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	closed = true
	if err := atomicReplaceRegistryFile(tempPath, path); err != nil {
		return err
	}
	if err := syncRegistryDirectory(dir); err != nil {
		return err
	}
	readback, err := s.readAuthorityCapsuleProjectionLocked(securityContext)
	if err != nil || readback.RecordDigest != capsule.RecordDigest {
		return errors.New("evidence registry authority capsule readback failed")
	}
	return nil
}

func readRegistryProjection(path string, maxLength int) ([]byte, error) {
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return nil, err
	}
	if maxLength < 0 || before.Size() < 0 || before.Size() > int64(maxLength) {
		return nil, errors.New("evidence registry projection size is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := validateOpenedRegistryFile(file, before); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxLength)+1))
	if err != nil || len(body) > maxLength {
		return nil, errors.New("evidence registry projection body is invalid")
	}
	return body, nil
}

func writeRegistryProjection(path string, ledger []byte) error {
	if len(ledger) == 0 || ledger[len(ledger)-1] != '\n' {
		return errors.New("evidence registry canonical projection is invalid")
	}
	dir := filepath.Dir(path)
	if err := ensureRegistryDirectory(dir); err != nil {
		return err
	}
	if before, err := validateRegistryRegularFile(path); err == nil {
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		validateErr := validateOpenedRegistryFile(file, before)
		closeErr := file.Close()
		if err := errors.Join(validateErr, closeErr); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	written, err := temp.Write(ledger)
	if err != nil || written != len(ledger) {
		return errors.New("evidence registry projection write was incomplete")
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	closed = true
	if err := atomicReplaceRegistryFile(tempPath, path); err != nil {
		return err
	}
	if err := syncRegistryDirectory(dir); err != nil {
		return err
	}
	readback, err := readRegistryProjection(path, len(ledger))
	if err != nil || !bytes.Equal(readback, ledger) {
		return errors.New("evidence registry projection readback failed")
	}
	return nil
}

func (s *Store) validateRegistryInventoryLocked(index *domainevidence.EvidenceRegistryAuthorityIndex) error {
	return s.validateRegistryInventoryModeLocked(index, true)
}

func (s *Store) validateRegistryFastInventoryLocked(index *domainevidence.EvidenceRegistryAuthorityIndex) error {
	return s.validateRegistryInventoryModeLocked(index, false)
}

func (s *Store) validateRegistryInventoryModeLocked(index *domainevidence.EvidenceRegistryAuthorityIndex, full bool) error {
	if err := validateRegistryDirectory(s.root); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	seenIndex := false
	seenCapsules := false
	seenProjections := false
	for _, entry := range entries {
		path := filepath.Join(s.root, entry.Name())
		switch entry.Name() {
		case ".registry.lock":
			if err := validateRegistryFilePath(path); err != nil {
				return err
			}
		case evidenceRegistryAuthorityIndexFile:
			if index == nil || seenIndex {
				return errors.New("evidence registry root contains an unexpected authority index")
			}
			if err := validateRegistryFilePath(path); err != nil {
				return err
			}
			seenIndex = true
		case evidenceRegistryCapsuleDirectory:
			if seenCapsules || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				return errors.New("evidence registry capsule directory is invalid")
			}
			seenCapsules = true
		case evidenceRegistryProjectionDirectory:
			if seenProjections || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
				return errors.New("evidence registry projection directory is invalid")
			}
			seenProjections = true
		default:
			if strings.HasPrefix(entry.Name(), ".registry-authority-index-") && strings.HasSuffix(entry.Name(), ".tmp") &&
				entry.Type()&os.ModeSymlink == 0 && !entry.IsDir() {
				if err := validateRegistryFilePath(path); err != nil {
					return err
				}
				continue
			}
			return errors.New("evidence registry root contains an unknown entry")
		}
	}
	if index != nil && !seenIndex {
		return errors.New("evidence registry authority index is missing from inventory")
	}
	var capsuleErr error
	if full {
		capsuleErr = s.validateCapsuleInventoryLocked(index, seenCapsules)
	} else {
		capsuleErr = s.validateCapsuleInventoryMetadataLocked(index, seenCapsules)
	}
	if capsuleErr != nil {
		return capsuleErr
	}
	if err := s.validateProjectionInventoryLocked(index, seenProjections); err != nil {
		return err
	}
	return nil
}

func (s *Store) validateCapsuleInventoryLocked(index *domainevidence.EvidenceRegistryAuthorityIndex, directoryPresent bool) error {
	referenced := map[string]domainevidence.EvidenceRegistryAuthorityIndexEntry{}
	currentByTurn := map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}
	currentByDigest := map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}
	if index != nil {
		for _, entry := range index.Entries {
			referenced[entry.CapsuleSHA256] = entry
			capsule, err := s.readAuthorityCapsuleBlobLocked(*index, entry)
			if err != nil {
				return err
			}
			currentByTurn[entry.ThreadID+"\x00"+entry.TurnID] = capsule
			currentByDigest[entry.CapsuleSHA256] = capsule
		}
	}
	if !directoryPresent {
		if len(referenced) > 0 {
			return errors.New("evidence registry authority capsule directory is missing")
		}
		return nil
	}
	dir := s.authorityCapsuleDirectory()
	if err := validateRegistryDirectory(dir); err != nil {
		return err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		if file.Type()&os.ModeSymlink != 0 || file.IsDir() {
			return errors.New("evidence registry capsule directory contains an unknown entry")
		}
		if strings.HasPrefix(file.Name(), ".capsule-") && strings.HasSuffix(file.Name(), ".tmp") {
			if err := s.validateAuthorityCapsuleTempLocked(path); err != nil {
				return err
			}
			continue
		}
		digest := strings.TrimSuffix(file.Name(), ".json")
		if !strings.HasSuffix(file.Name(), ".json") || !domainsecurity.IsSHA256Hex(digest) {
			return errors.New("evidence registry capsule directory contains an invalid blob name")
		}
		before, err := validateRegistryRegularFile(path)
		if err != nil || before.Size() <= 0 || before.Size() > maxEvidenceRegistryAuthorityCapsuleBytes {
			return errors.New("evidence registry capsule blob inventory is invalid")
		}
		body, _, err := s.readExactAuthorityCapsuleBlobLocked(path, int(before.Size()))
		if err != nil || domainsecurity.SHA256Hex(body) != digest {
			return errors.New("evidence registry capsule blob inventory integrity is invalid")
		}
		if entry, ok := referenced[digest]; ok {
			if index == nil {
				return errors.New("evidence registry capsule has no root authority index")
			}
			_ = entry
			if _, ok := currentByDigest[digest]; !ok {
				return errors.New("evidence registry current capsule inventory is incomplete")
			}
			seen[digest] = true
			continue
		}
		orphan, err := s.parseTrustedAuthorityCapsule(body)
		if err != nil {
			return errors.Join(errors.New("unindexed evidence registry capsule is not trusted by this installation"), err)
		}
		current, ok := currentByTurn[orphan.Registry.ThreadID+"\x00"+orphan.Registry.TurnID]
		if !ok {
			return errors.New("unindexed evidence registry capsule has no current turn authority")
		}
		if !authorityCapsuleIsProjectionOf(current, orphan) {
			return errors.New("unindexed evidence registry capsule proves a rollback or divergent authority chain")
		}
	}
	for digest := range referenced {
		if !seen[digest] {
			return errors.New("evidence registry authority index references a missing capsule blob")
		}
	}
	return nil
}

func (s *Store) validateCapsuleInventoryMetadataLocked(index *domainevidence.EvidenceRegistryAuthorityIndex, directoryPresent bool) error {
	referenced := map[string]domainevidence.EvidenceRegistryAuthorityIndexEntry{}
	currentByTurn := map[string]domainevidence.EvidenceRegistryAuthorityIndexEntry{}
	if index != nil {
		for _, entry := range index.Entries {
			referenced[entry.CapsuleSHA256] = entry
			currentByTurn[entry.ThreadID+"\x00"+entry.TurnID] = entry
		}
	}
	if !directoryPresent {
		if len(referenced) > 0 {
			return errors.New("evidence registry authority capsule directory is missing")
		}
		return nil
	}
	dir := s.authorityCapsuleDirectory()
	if err := validateRegistryDirectory(dir); err != nil {
		return err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		if file.Type()&os.ModeSymlink != 0 || file.IsDir() {
			return errors.New("evidence registry capsule directory contains an unknown entry")
		}
		if strings.HasPrefix(file.Name(), ".capsule-") && strings.HasSuffix(file.Name(), ".tmp") {
			if err := s.validateAuthorityCapsuleTempLocked(path); err != nil {
				return err
			}
			continue
		}
		digest := strings.TrimSuffix(file.Name(), ".json")
		if !strings.HasSuffix(file.Name(), ".json") || !domainsecurity.IsSHA256Hex(digest) {
			return errors.New("evidence registry capsule directory contains an invalid blob name")
		}
		before, err := validateRegistryRegularFile(path)
		if err != nil || before.Size() <= 0 || before.Size() > maxEvidenceRegistryAuthorityCapsuleBytes {
			return errors.New("evidence registry capsule blob inventory is invalid")
		}
		if _, ok := referenced[digest]; ok {
			seen[digest] = true
			continue
		}
		body, _, err := s.readExactAuthorityCapsuleBlobLocked(path, int(before.Size()))
		if err != nil || domainsecurity.SHA256Hex(body) != digest {
			return errors.New("unindexed evidence registry capsule integrity is invalid")
		}
		orphan, err := s.parseTrustedAuthorityCapsule(body)
		if err != nil {
			return errors.New("unindexed evidence registry capsule is not trusted by this installation")
		}
		currentEntry, ok := currentByTurn[orphan.Registry.ThreadID+"\x00"+orphan.Registry.TurnID]
		if !ok || index == nil {
			return errors.New("unindexed evidence registry capsule has no current turn authority")
		}
		current, err := s.readAuthorityCapsuleBlobLocked(*index, currentEntry)
		if err != nil || !authorityCapsuleIsProjectionOf(current, orphan) {
			return errors.New("unindexed evidence registry capsule proves a rollback or divergent authority chain")
		}
	}
	for digest := range referenced {
		if !seen[digest] {
			return errors.New("evidence registry authority index references a missing capsule blob")
		}
	}
	return nil
}

func (s *Store) validateProjectionInventoryLocked(index *domainevidence.EvidenceRegistryAuthorityIndex, directoryPresent bool) error {
	allowed := map[string]bool{}
	if index != nil {
		for _, entry := range index.Entries {
			allowed[entry.ProjectionKey] = true
		}
	}
	if !directoryPresent {
		return nil
	}
	dir := filepath.Join(s.root, evidenceRegistryProjectionDirectory)
	if err := validateRegistryDirectory(dir); err != nil {
		return err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		path := filepath.Join(dir, file.Name())
		if file.Type()&os.ModeSymlink != 0 || file.IsDir() {
			return errors.New("evidence registry projection directory contains an unknown entry")
		}
		if strings.HasPrefix(file.Name(), ".") && strings.HasSuffix(file.Name(), ".tmp") &&
			(strings.Contains(file.Name(), ".jsonl-") || strings.Contains(file.Name(), ".head.json-")) {
			if err := validateRegistryFilePath(path); err != nil {
				return err
			}
			continue
		}
		var key string
		switch {
		case strings.HasSuffix(file.Name(), ".head.json"):
			key = strings.TrimSuffix(file.Name(), ".head.json")
		case strings.HasSuffix(file.Name(), ".jsonl"):
			key = strings.TrimSuffix(file.Name(), ".jsonl")
		default:
			return errors.New("evidence registry projection has an invalid suffix")
		}
		if !domainsecurity.IsSHA256Hex(key) || !allowed[key] {
			return errors.New("evidence registry projection is detached from the signed root index")
		}
		if err := validateRegistryFilePath(path); err != nil {
			return err
		}
	}
	return nil
}

func ensureRegistryDirectory(path string) error {
	_, beforeErr := os.Lstat(path)
	created := errors.Is(beforeErr, os.ErrNotExist)
	if beforeErr != nil && !created {
		return beforeErr
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := validateRegistryDirectory(path); err != nil {
		return err
	}
	if created {
		return syncRegistryDirectory(filepath.Dir(path))
	}
	return nil
}

func validateRegistryDirectoryIfPresent(path string) error {
	err := validateRegistryDirectory(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func validateRegistryCreationPath(path string) error {
	abs, err := canonicalRegistryRootPath(path)
	if err != nil {
		return err
	}
	current := filepath.Clean(abs)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return errors.New("evidence registry creation path has an unsafe ancestor")
			}
			real, err := filepath.EvalSymlinks(current)
			if err != nil || filepath.Clean(real) != current {
				return errors.New("evidence registry creation path traverses a symlink")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("evidence registry creation path has no trusted ancestor")
		}
		current = parent
	}
}

func canonicalRegistryRootPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS == "darwin" {
		for alias, canonical := range map[string]string{"/var": "/private/var", "/tmp": "/private/tmp", "/etc": "/private/etc"} {
			if abs == alias {
				return canonical, nil
			}
			if strings.HasPrefix(abs, alias+string(filepath.Separator)) {
				return filepath.Join(canonical, strings.TrimPrefix(abs, alias+string(filepath.Separator))), nil
			}
		}
	}
	return abs, nil
}

func validateRegistryDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil || filepath.Clean(real) != filepath.Clean(abs) {
		if err != nil {
			return err
		}
		return errors.New("evidence registry directory traverses a symlink")
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("evidence registry directory is not regular")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return errors.New("evidence registry directory permissions are too broad")
	}
	return nil
}

func validateRegistryRegularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("evidence registry path is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("evidence registry file permissions are too broad")
	}
	return info, nil
}

func validateRegistryFilePath(path string) error {
	before, err := validateRegistryRegularFile(path)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	validateErr := validateOpenedRegistryFile(file, before)
	return errors.Join(validateErr, file.Close())
}

func validateOpenedRegistryFile(file *os.File, before os.FileInfo) error {
	return validateOpenedRegistryFileWithLinkCount(file, before, 1)
}

func validateOpenedRegistryFileWithLinkCount(file *os.File, before os.FileInfo, allowedLinks uint64) error {
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) {
		return errors.Join(errors.New("evidence registry file identity changed"), err)
	}
	links, err := registryRegularFileLinkCount(file, after)
	if err != nil || links != allowedLinks {
		return errors.Join(errors.New("evidence registry file has unsafe link authority"), err)
	}
	return nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (s *Store) lock(ctx context.Context) (func() error, error) {
	s.mu.Lock()
	if s.poisoned != nil {
		err := s.poisoned
		s.mu.Unlock()
		return nil, err
	}
	unlockFile, err := acquireRegistryFileLock(ctx, filepath.Join(s.root, ".registry.lock"))
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	return func() error {
		err := unlockFile()
		s.mu.Unlock()
		return err
	}, nil
}
