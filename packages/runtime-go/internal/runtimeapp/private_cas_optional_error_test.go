package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

// Inject one real access failure after the owner has been prepared. All
// subsequent access delegates normally, so revalidation cannot erase the error.
type runtimeOptionalOneShotAccessFailureV1 struct {
	finalauthority.SecurePrivateCASRecoveryAccessAuthority
	root  string
	err   error
	armed bool
	fired int
	seen  int
	skip  int
}

func (a *runtimeOptionalOneShotAccessFailureV1) fail(root string) error {
	if root == a.root {
		a.seen++
	}
	if a.armed && root == a.root {
		if a.skip > 0 {
			a.skip--
			return nil
		}
		a.armed = false
		a.fired++
		return a.err
	}
	return nil
}

type runtimeOptionalRecoveryJournalProbeV1 struct {
	privatecasport.RecoveryJournalV1
	preparations int
}

func (j *runtimeOptionalRecoveryJournalProbeV1) BeginPreparationAfterValidatedPreflight(ctx context.Context, request privatecasport.RecoveryPreparationRequestV1) (domainprivatecas.RecoveryJournalPreparationV1, error) {
	j.preparations++
	return j.RecoveryJournalV1.BeginPreparationAfterValidatedPreflight(ctx, request)
}

func TestRuntimeOptionalOwnerReadFailureStopsNativeRecovery(t *testing.T) {
	for _, ownerName := range []string{runtimePrivateCASCaseEntityOwnerV1, runtimePrivateCASEvidenceRegistryOwnerV1} {
		t.Run(ownerName, func(t *testing.T) {
			dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
			leaf := "bindings-v1"
			if ownerName == runtimePrivateCASEvidenceRegistryOwnerV1 {
				leaf = "indexes"
				for name, size := range map[string]int{"indexes": 256 << 10, "capsules": 16 << 20} {
					cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(dataDir, "private", ownerName, name), size, access)
					if err != nil {
						t.Fatal(err)
					}
					if err := cas.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			// A real orphan target in an earlier owner makes recovery effects
			// observable; the failed gate must not clean it or create a journal.
			runtimePrivateCASRecoveryTemp(t, dataDir, "accepted-finals", "records", "03"+strings.Repeat("d", 62))
			lease, err := persistencefs.AcquireCompositeLease(persistencefs.RootSet{DataDir: dataDir, DurableDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			journal, err := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
			if err != nil {
				t.Fatal(err)
			}
			probe := &runtimeOptionalRecoveryJournalProbeV1{RecoveryJournalV1: journal}
			path := filepath.Join(dataDir, "private", ownerName, leaf)
			fault := &runtimeOptionalOneShotAccessFailureV1{SecurePrivateCASRecoveryAccessAuthority: lease, root: path}
			// Count preparation accesses on the actual owner, without injecting
			// an error. Fail the first subsequent access in the native lifecycle.
			for _, owner := range runtimePrivateCASOwnerRecoveries(dataDir, fault) {
				if owner.name == ownerName {
					if _, err := owner.prepare(context.Background()); err != nil {
						t.Fatal(err)
					}
				}
			}
			preparationReads := fault.seen
			if preparationReads == 0 {
				t.Fatal("owner preparation did not access its exact leaf")
			}
			for _, failure := range []error{&os.PathError{Op: "read", Path: path, Err: syscall.EIO}, context.Canceled, errors.Join(errors.New("delegated access rejected"), syscall.EIO)} {
				fault.err, fault.armed, fault.skip, fault.fired = failure, true, preparationReads, 0
				before := startupWholeTreeRecordMapForTest(t, dataDir)
				err := recoverRuntimePrivateCASOwners(context.Background(), dataDir, fault, nil, probe)
				if fault.fired != 1 || !errors.Is(err, failure) {
					t.Fatalf("native recovery lost original access failure: fired=%d err=%v", fault.fired, err)
				}
				if probe.preparations != 0 || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, dataDir)) {
					t.Fatal("refused recovery began journal preparation or changed Original/residue")
				}
			}
		})
	}
}

func (a *runtimeOptionalOneShotAccessFailureV1) WithPrivateCASAccess(ctx context.Context, root string, visit func(privatecasport.RootBinding) error) error {
	if err := a.fail(root); err != nil {
		return err
	}
	return a.SecurePrivateCASRecoveryAccessAuthority.WithPrivateCASAccess(ctx, root, visit)
}

func (a *runtimeOptionalOneShotAccessFailureV1) WithExistingPrivateCASAccess(ctx context.Context, root string, visit func(privatecasport.RootBinding) error) error {
	if err := a.fail(root); err != nil {
		return err
	}
	return a.SecurePrivateCASRecoveryAccessAuthority.WithExistingPrivateCASAccess(ctx, root, visit)
}

func TestRuntimeOptionalOwnerRetainsTransientReadFailure(t *testing.T) {
	for _, ownerName := range []string{runtimePrivateCASCaseEntityOwnerV1, runtimePrivateCASEvidenceRegistryOwnerV1} {
		t.Run(ownerName, func(t *testing.T) {
			dataDir, access := initializedRuntimePrivateCASRecoveryTest(t)
			leaf := "bindings-v1"
			if ownerName == runtimePrivateCASEvidenceRegistryOwnerV1 {
				leaf = "indexes"
				for name, size := range map[string]int{"indexes": 256 << 10, "capsules": 16 << 20} {
					cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(dataDir, "private", ownerName, name), size, access)
					if err != nil {
						t.Fatal(err)
					}
					if err := cas.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			path := filepath.Join(dataDir, "private", ownerName, leaf)
			injected := &os.PathError{Op: "read", Path: path, Err: syscall.EIO}
			fault := &runtimeOptionalOneShotAccessFailureV1{SecurePrivateCASRecoveryAccessAuthority: access, root: path, err: injected}
			owners := runtimePrivateCASOwnerRecoveries(dataDir, fault)
			for i := range owners {
				if owners[i].name == ownerName {
					prepare := owners[i].prepare
					owners[i].prepare = func(ctx context.Context) (runtimePreparedPrivateCASOwnerRecovery, error) {
						plan, err := prepare(ctx)
						if err == nil {
							fault.armed = true
						}
						return plan, err
					}
				}
			}
			before := startupWholeTreeRecordMapForTest(t, dataDir)
			_, err := prepareAndValidateRuntimePrivateCASOwners(context.Background(), owners, true, nil)
			if fault.fired != 1 || !errors.Is(err, injected) || !errors.Is(err, syscall.EIO) {
				t.Fatalf("one-shot owner read failure was lost: fired=%d err=%v", fault.fired, err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, dataDir)) {
				t.Fatal("read refusal changed Original")
			}
		})
	}
}
