package caseentity

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
)

const (
	bindingPartitionV1       = "bindings-v1"
	ingressPartitionV1       = "ingress-v1"
	threadContextPartitionV1 = "thread-context-v1"
)

type Store struct {
	mu                 sync.Mutex
	bindingOrdinalGate *sync.Mutex
	root               string
	bindings           *finalauthorityadapter.SecurePrivateCAS
	ingress            *finalauthorityadapter.SecurePrivateCAS
	threadContext      *finalauthorityadapter.SecurePrivateCAS
}

type bindingOrdinalScopeV1 struct {
	tenantID        string
	userID          string
	caseID          string
	caseBindingHash string
}

type bindingInventoryV1 struct {
	byKey      map[string]domaincaseentity.CaseEntityBindingRecord
	maxByScope map[bindingOrdinalScopeV1]uint32
}

// Sibling Store handles for one canonical binding root must serialize the
// complete ordinal inventory-read + immutable binding-write transaction. The
// production runtime's existing persistence single-owner lease provides the
// cross-process exclusion before stores are constructed; this gate closes the
// remaining same-process sibling-handle race.
var bindingOrdinalProcessGatesV1 = struct {
	sync.Mutex
	byRoot map[string]*sync.Mutex
}{byRoot: make(map[string]*sync.Mutex)}

func bindingOrdinalProcessGateV1(root string) *sync.Mutex {
	bindingRoot := filepath.Clean(filepath.Join(root, bindingPartitionV1))
	bindingOrdinalProcessGatesV1.Lock()
	defer bindingOrdinalProcessGatesV1.Unlock()
	gate := bindingOrdinalProcessGatesV1.byRoot[bindingRoot]
	if gate == nil {
		gate = &sync.Mutex{}
		bindingOrdinalProcessGatesV1.byRoot[bindingRoot] = gate
	}
	return gate
}

var _ caseentityport.Store = (*Store)(nil)

func NewStore(
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (*Store, error) {
	return NewStoreContext(context.Background(), root, access)
}

func NewStoreContext(
	ctx context.Context,
	root string,
	access finalauthorityadapter.SecurePrivateCASAccessAuthority,
) (*Store, error) {
	if access == nil {
		return nil, errors.New("case entity private state access authority is required")
	}
	root = strings.TrimSpace(root)
	absolute, err := filepath.Abs(root)
	if root == "" || err != nil || !filepath.IsAbs(absolute) || filepath.Clean(absolute) != absolute {
		return nil, errors.New("case entity private state root is invalid")
	}
	bindings, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx,
		filepath.Join(absolute, bindingPartitionV1),
		domaincaseentity.MaxCaseEntityBindingRecordBytesV1,
		access,
	)
	if err != nil {
		return nil, err
	}
	ingress, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx,
		filepath.Join(absolute, ingressPartitionV1),
		domaincaseentity.MaxCaseIngressRecordBytesV1,
		access,
	)
	if err != nil {
		_ = bindings.Close()
		return nil, err
	}
	threadContext, err := finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthorityContext(
		ctx,
		filepath.Join(absolute, threadContextPartitionV1),
		domaincaseentity.MaxThreadCaseContextRecordBytesV1,
		access,
	)
	if err != nil {
		_ = ingress.Close()
		_ = bindings.Close()
		return nil, err
	}
	return &Store{
		bindingOrdinalGate: bindingOrdinalProcessGateV1(absolute),
		root:               absolute,
		bindings:           bindings,
		ingress:            ingress,
		threadContext:      threadContext,
	}, nil
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return errors.Join(
		store.bindings.Close(),
		store.ingress.Close(),
		store.threadContext.Close(),
	)
}

// EnsureBinding atomically reuses an existing immutable binding or assigns
// the next case-scoped ordinal before the first write. The ordinal namespace
// spans every supported financial entity type in the same case binding.
func (store *Store) EnsureBinding(
	ctx context.Context,
	input domaincaseentity.CaseEntityBindingRecordInputV1,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if store == nil || ctx == nil || store.bindingOrdinalGate == nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	bindingKey, err := domaincaseentity.CaseEntityBindingLookupKeyV1(
		input.SecurityContext,
		input.EntityType,
		input.Reference,
	)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.bindingOrdinalGate.Lock()
	defer store.bindingOrdinalGate.Unlock()

	inventory, err := store.loadBindingInventoryV1(ctx)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, err
	}
	if existing, found := inventory.byKey[bindingKey]; found {
		expected, expectedErr := domaincaseentity.NewCaseEntityBindingRecordV1(
			domaincaseentity.WithCaseEntityBindingStableOrdinalV1(input, existing.StableOrdinal),
		)
		matches, matchErr := sameBindingIdentityAndPrivateValueV1(existing, expected)
		if expectedErr != nil || matchErr != nil || !matches {
			return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrConflict
		}
		return existing, nil
	}

	scope := bindingOrdinalScopeForContextV1(input.SecurityContext)
	maxOrdinal := inventory.maxByScope[scope]
	if maxOrdinal == math.MaxUint32 {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	record, err := domaincaseentity.NewCaseEntityBindingRecordV1(
		domaincaseentity.WithCaseEntityBindingStableOrdinalV1(input, maxOrdinal+1),
	)
	if err != nil || record.BindingKey != bindingKey {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	body, err := domaincaseentity.CaseEntityBindingRecordV1Bytes(record)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	if err := putCanonicalIfAbsentV1(ctx, store.bindings, record.BindingKey, body); err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, err
	}
	persistedBody, err := readCanonicalV1(ctx, store.bindings, record.BindingKey)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, err
	}
	persisted, err := domaincaseentity.ParseCaseEntityBindingRecordV1(persistedBody)
	if err != nil || persisted.RecordDigest != record.RecordDigest {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	return persisted, nil
}

func (store *Store) ResolveBinding(
	ctx context.Context,
	bindingKey string,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if store == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(bindingKey)) {
		return domaincaseentity.CaseEntityBindingRecord{}, errors.New("case entity private binding lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadBindingInventoryV1(ctx)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, err
	}
	record, found := inventory.byKey[bindingKey]
	if !found {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrNotFound
	}
	return record, nil
}

// ResolveBindingByStableOrdinal reuses the binding inventory's canonical
// case-scoped ordinal namespace. It never returns an inventory or a maximum
// ordinal and therefore cannot be used for ambient entity discovery.
func (store *Store) ResolveBindingByStableOrdinal(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	entityType string,
	stableOrdinal uint32,
) (domaincaseentity.CaseEntityBindingRecord, error) {
	if store == nil || ctx == nil || stableOrdinal == 0 ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!domaincaseentity.IsFinancialEntityTypeV1(entityType) {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	inventory, err := store.loadBindingInventoryV1(ctx)
	if err != nil {
		return domaincaseentity.CaseEntityBindingRecord{}, err
	}
	scope := bindingOrdinalScopeForContextV1(securityContext)
	var resolved domaincaseentity.CaseEntityBindingRecord
	for _, record := range inventory.byKey {
		if bindingOrdinalScopeForRecordV1(record) != scope ||
			record.StableOrdinal != stableOrdinal || record.EntityType != entityType {
			continue
		}
		if resolved.Reference != "" {
			return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrIntegrity
		}
		resolved = record
	}
	if resolved.Reference == "" {
		return domaincaseentity.CaseEntityBindingRecord{}, caseentityport.ErrNotFound
	}
	return resolved, nil
}

func (store *Store) loadBindingInventoryV1(ctx context.Context) (bindingInventoryV1, error) {
	if store == nil || store.bindings == nil {
		return bindingInventoryV1{}, caseentityport.ErrIntegrity
	}
	records := make(map[string]domaincaseentity.CaseEntityBindingRecord)
	err := store.bindings.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		record, parseErr := domaincaseentity.ParseCaseEntityBindingRecordV1(file.Body)
		canonical, canonicalErr := domaincaseentity.CaseEntityBindingRecordV1Bytes(record)
		if parseErr != nil || canonicalErr != nil || record.BindingKey != file.Digest ||
			!bytes.Equal(canonical, file.Body) {
			return caseentityport.ErrIntegrity
		}
		if _, duplicate := records[record.BindingKey]; duplicate {
			return caseentityport.ErrIntegrity
		}
		records[record.BindingKey] = record
		return nil
	})
	if err != nil {
		return bindingInventoryV1{}, err
	}
	return normalizeBindingInventoryV1(records)
}

func normalizeBindingInventoryV1(
	records map[string]domaincaseentity.CaseEntityBindingRecord,
) (bindingInventoryV1, error) {
	ordinals := make(map[bindingOrdinalScopeV1]map[uint32]string)
	legacyKeys := make(map[bindingOrdinalScopeV1][]string)
	maxByScope := make(map[bindingOrdinalScopeV1]uint32)
	for key, record := range records {
		if key != record.BindingKey || domaincaseentity.ValidateCaseEntityBindingRecordV1(record) != nil {
			return bindingInventoryV1{}, caseentityport.ErrIntegrity
		}
		scope := bindingOrdinalScopeForRecordV1(record)
		if domaincaseentity.CaseEntityBindingNeedsStableOrdinalRecoveryV1(record) {
			legacyKeys[scope] = append(legacyKeys[scope], key)
			continue
		}
		if record.StableOrdinal == 0 {
			return bindingInventoryV1{}, caseentityport.ErrIntegrity
		}
		if ordinals[scope] == nil {
			ordinals[scope] = make(map[uint32]string)
		}
		if previous, duplicate := ordinals[scope][record.StableOrdinal]; duplicate && previous != key {
			return bindingInventoryV1{}, caseentityport.ErrIntegrity
		}
		ordinals[scope][record.StableOrdinal] = key
		if record.StableOrdinal > maxByScope[scope] {
			maxByScope[scope] = record.StableOrdinal
		}
	}

	for scope, keys := range legacyKeys {
		sort.Strings(keys)
		if uint64(len(keys)) > uint64(math.MaxUint32) {
			return bindingInventoryV1{}, caseentityport.ErrIntegrity
		}
		if ordinals[scope] == nil {
			ordinals[scope] = make(map[uint32]string)
		}
		for index, key := range keys {
			ordinal := uint32(index + 1)
			if previous, collision := ordinals[scope][ordinal]; collision && previous != key {
				return bindingInventoryV1{}, caseentityport.ErrIntegrity
			}
			recovered, err := domaincaseentity.WithRecoveredCaseEntityBindingStableOrdinalV1(records[key], ordinal)
			if err != nil {
				return bindingInventoryV1{}, caseentityport.ErrIntegrity
			}
			records[key] = recovered
			ordinals[scope][ordinal] = key
			if ordinal > maxByScope[scope] {
				maxByScope[scope] = ordinal
			}
		}
	}
	return bindingInventoryV1{byKey: records, maxByScope: maxByScope}, nil
}

func bindingOrdinalScopeForContextV1(securityContext domainsecurity.TurnSecurityContext) bindingOrdinalScopeV1 {
	return bindingOrdinalScopeV1{
		tenantID: securityContext.TenantID, userID: securityContext.UserID,
		caseID: securityContext.CaseID, caseBindingHash: securityContext.CaseBindingHash,
	}
}

func bindingOrdinalScopeForRecordV1(record domaincaseentity.CaseEntityBindingRecord) bindingOrdinalScopeV1 {
	return bindingOrdinalScopeV1{
		tenantID: record.TenantID, userID: record.UserID,
		caseID: record.CaseID, caseBindingHash: record.CaseBindingHash,
	}
}

func sameBindingIdentityAndPrivateValueV1(
	left domaincaseentity.CaseEntityBindingRecord,
	right domaincaseentity.CaseEntityBindingRecord,
) (bool, error) {
	if domaincaseentity.ValidateCaseEntityBindingRecordV1(left) != nil ||
		domaincaseentity.ValidateCaseEntityBindingRecordV1(right) != nil ||
		left.SchemaVersion != right.SchemaVersion || left.Purpose != right.Purpose ||
		left.TenantID != right.TenantID || left.UserID != right.UserID ||
		left.CaseID != right.CaseID || left.CaseBindingHash != right.CaseBindingHash ||
		left.EntityType != right.EntityType || left.Reference != right.Reference ||
		left.StableOrdinal != right.StableOrdinal || left.Canonicalizer != right.Canonicalizer ||
		left.BindingKey != right.BindingKey {
		return false, nil
	}
	var leftValue string
	if err := left.UseCanonicalValueV1(func(value string) error {
		leftValue = value
		return nil
	}); err != nil {
		return false, err
	}
	var rightValue string
	if err := right.UseCanonicalValueV1(func(value string) error {
		rightValue = value
		return nil
	}); err != nil {
		return false, err
	}
	return leftValue == rightValue, nil
}

func (store *Store) PutIngressIfAbsent(
	ctx context.Context,
	record domaincaseentity.CaseIngressRecord,
) error {
	if store == nil || domaincaseentity.ValidateCaseIngressRecordV1(record) != nil {
		return errors.New("case entity private ingress is invalid")
	}
	body, err := domaincaseentity.CaseIngressRecordV1Bytes(record)
	if err != nil {
		return errors.New("case entity private ingress is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return putCanonicalIfAbsentV1(ctx, store.ingress, record.IngressID, body)
}

func (store *Store) ResolveIngress(
	ctx context.Context,
	ingressID string,
) (domaincaseentity.CaseIngressRecord, error) {
	if store == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(ingressID)) {
		return domaincaseentity.CaseIngressRecord{}, errors.New("case entity private ingress lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	body, err := readCanonicalV1(ctx, store.ingress, ingressID)
	if err != nil {
		return domaincaseentity.CaseIngressRecord{}, err
	}
	record, err := domaincaseentity.ParseCaseIngressRecordV1(body)
	if err != nil || record.IngressID != ingressID {
		return domaincaseentity.CaseIngressRecord{}, caseentityport.ErrIntegrity
	}
	return record, nil
}

func (store *Store) PutThreadContextIfAbsent(
	ctx context.Context,
	record domaincaseentity.ThreadCaseContextRecord,
) error {
	if store == nil || domaincaseentity.ValidateThreadCaseContextRecordV1(record) != nil {
		return errors.New("thread case private context is invalid")
	}
	body, err := domaincaseentity.ThreadCaseContextRecordV1Bytes(record)
	if err != nil {
		return errors.New("thread case private context is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return putCanonicalIfAbsentV1(ctx, store.threadContext, record.StorageKey, body)
}

func (store *Store) ResolveThreadContext(
	ctx context.Context,
	storageKey string,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	if store == nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(storageKey)) {
		return domaincaseentity.ThreadCaseContextRecord{}, errors.New("thread case private context lookup is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	body, err := readCanonicalV1(ctx, store.threadContext, storageKey)
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, err
	}
	record, err := domaincaseentity.ParseThreadCaseContextRecordV1(body)
	if err != nil || record.StorageKey != storageKey {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrIntegrity
	}
	return record, nil
}

func (store *Store) ResolveLatestCaseLongitudinalContext(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	return store.resolveLatestThreadContextForScopeV1(ctx, securityContext, "", true)
}

func (store *Store) ResolveLatestThreadContextForScope(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	threadID string,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	return store.resolveLatestThreadContextForScopeV1(ctx, securityContext, threadID, false)
}

func (store *Store) resolveLatestThreadContextForScopeV1(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	threadID string,
	caseLongitudinal bool,
) (domaincaseentity.ThreadCaseContextRecord, error) {
	if store == nil || ctx == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		(!caseLongitudinal && (strings.TrimSpace(threadID) == "" || threadID != strings.TrimSpace(threadID))) {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrIntegrity
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	records := make(map[uint64]domaincaseentity.ThreadCaseContextRecord)
	err := store.threadContext.Visit(ctx, func(file finalauthorityadapter.SecurePrivateCASFile) error {
		record, parseErr := domaincaseentity.ParseThreadCaseContextRecordV1(file.Body)
		canonical, canonicalErr := domaincaseentity.ThreadCaseContextRecordV1Bytes(record)
		if parseErr != nil || canonicalErr != nil || record.StorageKey != file.Digest ||
			!bytes.Equal(canonical, file.Body) {
			return caseentityport.ErrIntegrity
		}
		isIndex := domaincaseentity.IsCaseLongitudinalIndexRecordV1(record)
		if record.TenantID != securityContext.TenantID || record.UserID != securityContext.UserID ||
			record.CaseID != securityContext.CaseID || record.CaseBindingHash != securityContext.CaseBindingHash ||
			isIndex != caseLongitudinal || (!caseLongitudinal && record.ThreadID != threadID) {
			return nil
		}
		if _, duplicate := records[record.Generation]; duplicate {
			return caseentityport.ErrIntegrity
		}
		records[record.Generation] = record
		return nil
	})
	if err != nil {
		return domaincaseentity.ThreadCaseContextRecord{}, err
	}
	if len(records) == 0 {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrNotFound
	}
	latest := records[uint64(len(records))]
	if latest.Generation != uint64(len(records)) {
		return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrIntegrity
	}
	for generation := uint64(2); generation <= latest.Generation; generation++ {
		if domaincaseentity.ValidateThreadCaseContextEvolutionV1(records[generation-1], records[generation]) != nil {
			return domaincaseentity.ThreadCaseContextRecord{}, caseentityport.ErrIntegrity
		}
	}
	return latest, nil
}

// HasRecords performs a bounded presence probe across the three fixed private
// partitions. It stops at the first committed record and never materializes a
// complete private-state inventory.
func (store *Store) HasRecords(ctx context.Context) (bool, error) {
	if store == nil {
		return false, caseentityport.ErrIntegrity
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	partitions := make([]*finalauthorityadapter.SecurePrivateCAS, 0, 3)
	partitions = append(partitions, store.bindings, store.ingress, store.threadContext)
	found := errors.New("case entity private state record found")
	for _, partition := range partitions {
		if partition == nil {
			return false, caseentityport.ErrIntegrity
		}
		err := partition.Visit(ctx, func(finalauthorityadapter.SecurePrivateCASFile) error {
			return found
		})
		if errors.Is(err, found) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

func putCanonicalIfAbsentV1(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	key string,
	body []byte,
) error {
	if cas == nil || !domainsecurity.IsSHA256Hex(key) || len(body) == 0 {
		return caseentityport.ErrIntegrity
	}
	existing, err := cas.Read(ctx, key)
	switch {
	case err == nil:
		if bytes.Equal(existing, body) {
			return nil
		}
		return caseentityport.ErrConflict
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	if err := cas.PutIfAbsent(ctx, key, body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, readErr := cas.Read(ctx, key)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(existing, body) {
			return nil
		}
		return caseentityport.ErrConflict
	}
	written, err := cas.Read(ctx, key)
	if err != nil || !bytes.Equal(written, body) {
		return caseentityport.ErrIntegrity
	}
	return nil
}

func readCanonicalV1(
	ctx context.Context,
	cas *finalauthorityadapter.SecurePrivateCAS,
	key string,
) ([]byte, error) {
	if cas == nil {
		return nil, caseentityport.ErrIntegrity
	}
	body, err := cas.Read(ctx, key)
	if errors.Is(err, os.ErrNotExist) {
		return nil, caseentityport.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return body, nil
}
