//go:build darwin || linux

package runtimeapp

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func runtimeAdvanceIntentFixtureV2(t *testing.T, authority authorityport.Authority) domainauthority.MonotonicAdvanceIntentV2 {
	t.Helper()
	hash := func(label string) string {
		return domainsecurity.SHA256Hex([]byte("synthetic-runtime-journal-" + label))
	}
	seed := sha256.Sum256([]byte("synthetic-runtime-journal-witness"))
	witness := ed25519.NewKeyFromSeed(seed[:])
	witnessPublic := witness.Public().(ed25519.PublicKey)
	witnessSign := func(body []byte) ([]byte, error) { return ed25519.Sign(witness, body), nil }
	sign := func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) }
	enrollment, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: hash("installation"), EnrollmentID: hash("enrollment"), Namespace: domainsecurity.ThreadRiskAuthorityNamespaceV1,
		CurrentStateDigest: hash("enrollment-state"), FenceNonce: hash("enrollment-fence"), WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessPublicKey: witnessPublic,
	}, witnessSign)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainsecurity.NewThreadRiskAuthorityIndexV1(domainsecurity.ThreadRiskAuthorityIndexInputV1{
		InstallationID: enrollment.InstallationID, EnrollmentID: enrollment.EnrollmentID, Namespace: enrollment.Namespace, Generation: 1,
		Entries:    []domainsecurity.ThreadRiskAuthorityEntryV1{{ThreadID: "thread-synthetic-journal", WorkspaceRealPath: "/synthetic/journal", RiskClass: domainsecurity.RiskClassGeneral, CurrentPolicyDigest: hash("policy")}},
		MutationID: hash("mutation"), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := domainauthority.NewThreadRiskGenesisTransitionBindingV2(enrollment, index)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: enrollment.InstallationID, EnrollmentID: enrollment.EnrollmentID, Namespace: enrollment.Namespace,
		ExpectedGeneration: enrollment.Generation, ExpectedCheckpointDigest: enrollment.CheckpointDigest, ExpectedStateDigest: enrollment.CurrentStateDigest,
		NextGeneration: index.Generation, NextStateDigest: index.IndexDigest, ExpectedFenceNonce: enrollment.FenceNonce,
		MutationID: index.MutationID, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainauthority.NewMonotonicAdvanceIntentV2(domainauthority.MonotonicAdvanceIntentInputV2{
		Root: domainauthority.AdvanceRootThreadRiskV2, PreviousCheckpoint: enrollment, AdvanceRequest: request, Transition: transition,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

func TestRuntimeAuthorityAdvanceStartupUsesCurrentKeyWithoutSettlingIntent(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "foreign"}[foreign], func(t *testing.T) {
			config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
			handler, err := NewRuntimeServerHandlerE(config)
			if err != nil {
				t.Fatal(err)
			}
			shutdownOwnedRuntimeHandler(t, handler)
			keyPath := filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json")
			if foreign {
				keyPath = filepath.Join(t.TempDir(), "synthetic-foreign-key.json")
			}
			key, err := finalauthority.OpenOrCreateFileAuthority(keyPath, !foreign)
			if err != nil {
				t.Fatal(err)
			}
			intent := runtimeAdvanceIntentFixtureV2(t, key)
			body, err := domainauthority.MonotonicAdvanceIntentV2Bytes(intent)
			if err != nil {
				t.Fatal(err)
			}
			access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(config.DataDir, "private", "authority-advance", "v2")
			cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(journal, "intents"), domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2, access)
			if err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(cas.PutIfAbsent(context.Background(), intent.MutationID, body), cas.Close()); err != nil {
				t.Fatal(err)
			}
			roots, err := persistencefs.ResolveRootSet(config.DataDir, config.DurableTempDir)
			if err != nil {
				t.Fatal(err)
			}
			rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			prepared, err := prepareRuntimeAuthorityAdvanceStartupV2(context.Background(), roots, rootAuthority, access, nil)
			if foreign != (err != nil) {
				t.Fatalf("current key preflight result: %v", err)
			}
			if !foreign && (prepared == nil || !prepared.inventory.HasRecords()) {
				t.Fatal("nonempty journal omitted")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("journal preflight changed state")
			}
			if foreign {
				residue := filepath.Join(config.DataDir, "private", "accepted-finals", domainprivatecas.CreateDirectoryResidueNameV1("records"))
				if err := os.Mkdir(residue, 0o700); err != nil {
					t.Fatal(err)
				}
				before = startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			}
			handler, err = NewRuntimeServerHandlerE(config)
			if handler != nil {
				shutdownOwnedRuntimeHandler(t, handler)
			}
			if foreign != (err != nil) {
				t.Fatalf("nonempty journal fresh restart result: %v", err)
			}
			if foreign && !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
				t.Fatal("foreign authority refusal changed earlier recovery state")
			}
			stored, err := os.ReadFile(filepath.Join(journal, "intents", intent.MutationID[:2], intent.MutationID+".json"))
			if err != nil || !reflect.DeepEqual(body, stored) {
				t.Fatal("journal intent changed on restart")
			}
			if _, err := os.Lstat(filepath.Join(journal, "settlements", intent.MutationID[:2], intent.MutationID+".json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("startup settled the historical intent")
			}
		})
	}
}

func TestRuntimeAuthorityAdvanceRefusalPrecedesCreateRecovery(t *testing.T) {
	config := Config{RuntimeToken: DefaultRuntimeToken, DataDir: t.TempDir(), DurableTempDir: t.TempDir()}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	access, err := privatecastest.NewAccessAuthority(filepath.Join(config.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(config.DataDir, "private", "authority-advance", "v2", "intents"), domainauthority.MaxMonotonicAdvanceJournalRecordBytesV2, access)
	if err != nil {
		t.Fatal(err)
	}
	err = errors.Join(cas.PutIfAbsent(context.Background(), domainsecurity.SHA256Hex([]byte("synthetic-invalid-advance")), []byte(`{}`)), cas.Close())
	if err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(config.DataDir, "private", "accepted-finals", domainprivatecas.CreateDirectoryResidueNameV1("records"))
	if err := os.Mkdir(residue, 0o700); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, config.DataDir, config.DurableTempDir)
	handler, err = NewRuntimeServerHandlerE(config)
	if handler != nil {
		shutdownOwnedRuntimeHandler(t, handler)
	}
	if err == nil || !strings.Contains(err.Error(), "authority advance") {
		t.Fatalf("corrupt Core advance inventory did not refuse startup: %v", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, config.DataDir, config.DurableTempDir)) {
		t.Fatal("Core authority advance refusal occurred after startup changed state")
	}
}
