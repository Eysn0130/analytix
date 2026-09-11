package providerregistryfs

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

const (
	registryDirectoryName = "provider-registry"
	registryFileName      = "registry.v1.json"
	registryLockFileName  = "registry.lock"
)

var processGates sync.Map

type commitHooks struct {
	beforeReplace                   func() error
	afterReplaceBeforeDirectorySync func() error
	afterReplaceDirectorySync       func() error
	observe                         func(commitFaultPoint) error
}

type commitFaultPoint string

const (
	faultRollbackRequiredEstablished     commitFaultPoint = "rollback-required-established"
	faultCommitMarkerDirectorySync       commitFaultPoint = "commit-marker-directory-sync"
	faultCommitConfirmationDirectorySync commitFaultPoint = "commit-confirmation-directory-sync"
	faultRollbackBackupReestablished     commitFaultPoint = "rollback-backup-reestablished"
	faultRollbackRequiredReestablished   commitFaultPoint = "rollback-required-reestablished"
	faultCommitBackupCleanup             commitFaultPoint = "commit-backup-cleanup"
	faultCommitJournalCleanup            commitFaultPoint = "commit-journal-cleanup"
	faultCommitCleanupDirectorySync      commitFaultPoint = "commit-cleanup-directory-sync"
	faultCommitMarkerRemoval             commitFaultPoint = "commit-marker-removal"
	faultCommitMarkerRemovalSync         commitFaultPoint = "commit-marker-removal-sync"
)

func (hooks *commitHooks) at(point commitFaultPoint) error {
	if hooks == nil || hooks.observe == nil {
		return nil
	}
	return hooks.observe(point)
}

type processGate struct {
	token chan struct{}
}

func newProcessGate() *processGate {
	gate := &processGate{token: make(chan struct{}, 1)}
	gate.token <- struct{}{}
	return gate
}

type Store struct {
	dataDir      string
	directory    string
	registryPath string
	lockPath     string
	gate         *processGate
	hooks        *commitHooks

	mu     sync.RWMutex
	closed bool
}

var _ registryport.Store = (*Store)(nil)

func New(dataDir string) (*Store, error) {
	return newWithHooks(dataDir, nil)
}

func newWithHooks(dataDir string, hooks *commitHooks) (*Store, error) {
	if dataDir == "" || !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir {
		return nil, registryport.ErrInvalidRequest
	}
	resolved, err := filepath.EvalSymlinks(dataDir)
	if err != nil || !filepath.IsAbs(resolved) || filepath.Clean(resolved) != resolved {
		return nil, registryport.ErrInvalidRequest
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, registryport.ErrInvalidRequest
	}
	directory := filepath.Join(resolved, "private", registryDirectoryName)
	loaded, _ := processGates.LoadOrStore(resolved, newProcessGate())
	return &Store{
		dataDir: resolved, directory: directory,
		registryPath: filepath.Join(directory, registryFileName),
		lockPath:     filepath.Join(directory, registryLockFileName),
		gate:         loaded.(*processGate), hooks: hooks,
	}, nil
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.closed = true
	return nil
}

func (store *Store) WithExclusive(
	ctx context.Context,
	use func(registryport.Transaction) error,
) error {
	if store == nil || ctx == nil || use == nil {
		return registryport.ErrInvalidRequest
	}
	store.mu.RLock()
	closed := store.closed
	store.mu.RUnlock()
	if closed {
		return registryport.ErrClosed
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-store.gate.token:
	}
	defer func() { store.gate.token <- struct{}{} }()
	if err := ensureRegistryDirectory(store.dataDir, store.directory); err != nil {
		return registryport.ErrPersistence
	}
	lock, err := acquireRegistryLock(ctx, store.lockPath)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return registryport.ErrPersistence
	}
	defer lock.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(&transaction{path: store.registryPath, hooks: store.hooks})
}

type transaction struct {
	path  string
	hooks *commitHooks
}

func (transaction *transaction) Load(ctx context.Context) (domainregistry.Registry, error) {
	if transaction == nil || ctx == nil {
		return domainregistry.Registry{}, registryport.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return domainregistry.Registry{}, err
	}
	if err := recoverAtomicCommit(transaction.path); err != nil {
		return domainregistry.Registry{}, registryport.ErrPersistence
	}
	content, err := readPrivateRegistryFile(transaction.path, domainregistry.MaxRegistryBytes)
	if errors.Is(err, os.ErrNotExist) {
		incarnation, generationErr := newIncarnation()
		if generationErr != nil {
			return domainregistry.Registry{}, registryport.ErrPersistence
		}
		registry, registryErr := domainregistry.NewRegistry(incarnation)
		if registryErr != nil {
			return domainregistry.Registry{}, registryport.ErrPersistence
		}
		if commitErr := transaction.Commit(ctx, registry); commitErr != nil {
			return domainregistry.Registry{}, commitErr
		}
		return registry, nil
	}
	if err != nil {
		return domainregistry.Registry{}, registryport.ErrPersistence
	}
	registry, err := domainregistry.Unmarshal(content)
	clear(content)
	if err != nil {
		return domainregistry.Registry{}, registryport.ErrPersistence
	}
	return registry, nil
}

func (transaction *transaction) Commit(ctx context.Context, registry domainregistry.Registry) error {
	if transaction == nil || ctx == nil || registry.Validate() != nil {
		return registryport.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	content, err := domainregistry.Marshal(registry)
	if err != nil {
		return registryport.ErrInvalidRequest
	}
	defer clear(content)
	if err := writePrivateRegistryAtomically(transaction.path, content, transaction.hooks); err != nil {
		return registryport.ErrPersistence
	}
	return nil
}

func newIncarnation() (string, error) {
	value := make([]byte, 32)
	defer clear(value)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return "inc_" + base64.RawURLEncoding.EncodeToString(value), nil
}

func clear(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
