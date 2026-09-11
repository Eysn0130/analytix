package evidenceauthority

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestBundleStoreCanonicalRoundTripRestartAndDistinctDigestContracts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bundles")
	store, err := newTestBundleStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newEvidenceAuthorityStoreFixture(11)
	bundle := fixture.firstBundle(t, "roundtrip")
	canonical := canonicalEvidenceBundleBody(t, bundle)
	projectionBodyDigest := domainsecurity.SHA256Hex(canonical)
	if projectionBodyDigest == bundle.RecordDigest {
		t.Fatal("test fixture unexpectedly conflates the complete canonical body SHA with the domain-separated RecordDigest")
	}
	if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
		t.Fatalf("idempotent bundle write failed: %v", err)
	}
	storedBody, err := os.ReadFile(rawEvidenceCASRecordPath(root, bundle.RecordDigest))
	if err != nil || !bytes.Equal(storedBody, canonical) {
		t.Fatalf("bundle CAS did not preserve canonical bytes: err=%v", err)
	}
	if domainsecurity.SHA256Hex(storedBody) != projectionBodyDigest {
		t.Fatal("complete canonical body SHA changed on disk")
	}
	resolved, err := store.Resolve(context.Background(), bundle.RecordDigest)
	if err != nil || !equalBundle(resolved, bundle) {
		t.Fatalf("bundle round trip mismatch: bundle=%#v err=%v", resolved, err)
	}
	restarted, err := newTestBundleStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = restarted.Resolve(context.Background(), bundle.RecordDigest)
	if err != nil || !equalBundle(resolved, bundle) {
		t.Fatalf("restarted bundle round trip mismatch: bundle=%#v err=%v", resolved, err)
	}
	assertNoEvidenceAuthoritySelectionMethods(t, reflect.TypeOf(store))
	assertNoEvidenceAuthoritySelectionMethods(t, reflect.TypeOf(&Projection{}))
}

func TestObservationStoreCanonicalRoundTripRestartAndExactWitnessContract(t *testing.T) {
	root := filepath.Join(t.TempDir(), "observations")
	store, err := newTestObservationStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newEvidenceAuthorityStoreFixture(21)
	bundle := fixture.observationBundle(t, fixture.firstBundle(t, "observation"), "observation")
	if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
		t.Fatalf("idempotent observation write failed: %v", err)
	}
	resolved, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest)
	if err != nil || !equalObservation(resolved, bundle) {
		t.Fatalf("observation round trip mismatch: bundle=%#v err=%v", resolved, err)
	}
	restarted, err := newTestObservationStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = restarted.Resolve(context.Background(), bundle.Observation.ObservationDigest)
	if err != nil || !equalObservation(resolved, bundle) {
		t.Fatalf("restarted observation mismatch: bundle=%#v err=%v", resolved, err)
	}
	assertNoEvidenceAuthoritySelectionMethods(t, reflect.TypeOf(store))
}

func TestStoresConcurrentPutIfAbsentAreExactAndIdempotent(t *testing.T) {
	t.Run("bundle", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bundles")
		first, err := newTestBundleStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		second, err := newTestBundleStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		bundle := newEvidenceAuthorityStoreFixture(31).firstBundle(t, "concurrent-bundle")
		concurrentEvidenceExactWrites(t, 40, func(position int) error {
			if position%2 == 0 {
				return first.PutIfAbsent(context.Background(), bundle)
			}
			return second.PutIfAbsent(context.Background(), bundle)
		})
		resolved, err := first.Resolve(context.Background(), bundle.RecordDigest)
		if err != nil || !equalBundle(resolved, bundle) {
			t.Fatalf("concurrent bundle record mismatch: bundle=%#v err=%v", resolved, err)
		}
	})

	t.Run("observation", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		first, err := newTestObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		second, err := newTestObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newEvidenceAuthorityStoreFixture(41)
		bundle := fixture.observationBundle(t, fixture.firstBundle(t, "concurrent-observation"), "concurrent-observation")
		concurrentEvidenceExactWrites(t, 40, func(position int) error {
			if position%2 == 0 {
				return first.PutIfAbsent(context.Background(), bundle)
			}
			return second.PutIfAbsent(context.Background(), bundle)
		})
		resolved, err := first.Resolve(context.Background(), bundle.Observation.ObservationDigest)
		if err != nil || !equalObservation(resolved, bundle) {
			t.Fatalf("concurrent observation record mismatch: bundle=%#v err=%v", resolved, err)
		}
	})
}

func TestBundleStoreRejectsWrongDigestConflictDuplicateUnknownTrailingAndTruncation(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(51)
	bundle := fixture.firstBundle(t, "corrupt-bundle")
	canonical := canonicalEvidenceBundleBody(t, bundle)
	variants := map[string][]byte{
		"whitespace": append(append([]byte(nil), canonical...), '\n'),
		"duplicate":  append([]byte(`{"schemaVersion":1,`), canonical[1:]...),
		"unknown":    append(append([]byte(nil), canonical[:len(canonical)-1]...), []byte(`,"unknown":true}`)...),
		"trailing":   append(append([]byte(nil), canonical...), []byte(`{}`)...),
		"truncated":  append([]byte(nil), canonical[:len(canonical)-1]...),
		"attacker":   []byte(`{"generation":999,"current":true}`),
	}
	for name, body := range variants {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "bundles")
			raw, err := newTestEvidenceRawCAS(t, root, maxEvidenceAuthorityBundleBytes)
			if err != nil {
				t.Fatal(err)
			}
			if err := raw.PutIfAbsent(context.Background(), bundle.RecordDigest, body); err != nil {
				t.Fatal(err)
			}
			store, err := newTestBundleStore(t, root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Resolve(context.Background(), bundle.RecordDigest); err == nil {
				t.Fatal("corrupt bundle bytes were accepted")
			}
			if err := store.PutIfAbsent(context.Background(), bundle); err == nil {
				t.Fatal("canonical bundle overwrote conflicting CAS bytes")
			}
		})
	}

	t.Run("wrong-digest", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "bundles")
		raw, err := newTestEvidenceRawCAS(t, root, maxEvidenceAuthorityBundleBytes)
		if err != nil {
			t.Fatal(err)
		}
		wrong := domainsecurity.SHA256Hex([]byte("wrong-bundle-address"))
		if err := raw.PutIfAbsent(context.Background(), wrong, canonical); err != nil {
			t.Fatal(err)
		}
		store, err := newTestBundleStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), wrong); err == nil {
			t.Fatal("bundle stored at a wrong RecordDigest was accepted")
		}
	})
}

func TestObservationStoreRejectsClosedWireAndEveryWitnessBindingMismatch(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(61)
	bundle := fixture.observationBundle(t, fixture.firstBundle(t, "corrupt-observation"), "corrupt-observation")
	canonical, err := evidenceObservationBytes(bundle)
	if err != nil {
		t.Fatal(err)
	}
	variants := map[string][]byte{
		"whitespace": append(append([]byte(nil), canonical...), '\n'),
		"duplicate":  append([]byte(`{"schemaVersion":1,`), canonical[1:]...),
		"unknown":    append(append([]byte(nil), canonical[:len(canonical)-1]...), []byte(`,"unknown":true}`)...),
		"trailing":   append(append([]byte(nil), canonical...), []byte(`[]`)...),
		"truncated":  append([]byte(nil), canonical[:len(canonical)-1]...),
		"attacker":   []byte(`{"schemaVersion":1,"purpose":"analytix.evidence-authority-observation-bundle/v1","bundle":{},"request":{},"observation":{}}`),
	}
	for name, body := range variants {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "observations")
			raw, err := newTestEvidenceRawCAS(t, root, maxEvidenceObservationBytes)
			if err != nil {
				t.Fatal(err)
			}
			if err := raw.PutIfAbsent(context.Background(), bundle.Observation.ObservationDigest, body); err != nil {
				t.Fatal(err)
			}
			store, err := newTestObservationStore(t, root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest); err == nil {
				t.Fatal("corrupt observation bundle was accepted")
			}
			if err := store.PutIfAbsent(context.Background(), bundle); err == nil {
				t.Fatal("canonical observation overwrote conflicting CAS bytes")
			}
		})
	}

	store, err := newTestObservationStore(t, filepath.Join(t.TempDir(), "binding-mismatches"))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("mismatched-bundle", func(t *testing.T) {
		mismatch := bundle
		mismatch.Bundle = fixture.firstBundle(t, "other-bundle")
		if err := store.PutIfAbsent(context.Background(), mismatch); err == nil {
			t.Fatal("observation accepted a bundle not selected by its checkpoint")
		}
	})
	t.Run("mismatched-request", func(t *testing.T) {
		mismatch := bundle
		mismatch.Request = fixture.observeRequest(t, "other-request")
		if err := store.PutIfAbsent(context.Background(), mismatch); err == nil {
			t.Fatal("observation accepted a request it did not answer")
		}
	})
	t.Run("mismatched-observation", func(t *testing.T) {
		other := fixture.observationBundle(t, bundle.Bundle, "other-observation")
		mismatch := bundle
		mismatch.Observation = other.Observation
		if err := store.PutIfAbsent(context.Background(), mismatch); err == nil {
			t.Fatal("observation accepted a witness response for another request")
		}
	})
	t.Run("wrong-digest", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "wrong-observation")
		raw, err := newTestEvidenceRawCAS(t, root, maxEvidenceObservationBytes)
		if err != nil {
			t.Fatal(err)
		}
		wrong := domainsecurity.SHA256Hex([]byte("wrong-observation-address"))
		if err := raw.PutIfAbsent(context.Background(), wrong, canonical); err != nil {
			t.Fatal(err)
		}
		wrongStore, err := newTestObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := wrongStore.Resolve(context.Background(), wrong); err == nil {
			t.Fatal("observation stored at a wrong digest was accepted")
		}
	})
}

func TestStoresHonorCancellationAndStrictDigestInput(t *testing.T) {
	fixture := newEvidenceAuthorityStoreFixture(71)
	bundle := fixture.firstBundle(t, "cancel")
	observation := fixture.observationBundle(t, bundle, "cancel")
	bundleStore, err := newTestBundleStore(t, filepath.Join(t.TempDir(), "bundles"))
	if err != nil {
		t.Fatal(err)
	}
	observationStore, err := newTestObservationStore(t, filepath.Join(t.TempDir(), "observations"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bundleStore.PutIfAbsent(ctx, bundle); !errors.Is(err, context.Canceled) {
		t.Fatalf("bundle write ignored cancellation: %v", err)
	}
	if err := observationStore.PutIfAbsent(ctx, observation); !errors.Is(err, context.Canceled) {
		t.Fatalf("observation write ignored cancellation: %v", err)
	}
	for _, digest := range []string{"", " " + bundle.RecordDigest, strings.ToUpper(bundle.RecordDigest), bundle.RecordDigest + " "} {
		if _, err := bundleStore.Resolve(context.Background(), digest); err == nil {
			t.Fatalf("bundle store accepted non-canonical digest %q", digest)
		}
		if _, err := observationStore.Resolve(context.Background(), digest); err == nil {
			t.Fatalf("observation store accepted non-canonical digest %q", digest)
		}
	}
}

func concurrentEvidenceExactWrites(t *testing.T, count int, write func(int) error) {
	t.Helper()
	start := make(chan struct{})
	errorsByWriter := make(chan error, count)
	var wait sync.WaitGroup
	for position := 0; position < count; position++ {
		wait.Add(1)
		go func(position int) {
			defer wait.Done()
			<-start
			errorsByWriter <- write(position)
		}(position)
	}
	close(start)
	wait.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("concurrent exact write failed: %v", err)
		}
	}
}

func assertNoEvidenceAuthoritySelectionMethods(t *testing.T, typ reflect.Type) {
	t.Helper()
	for _, name := range []string{"List", "Current", "ResolveCurrent"} {
		if _, found := typ.MethodByName(name); found {
			t.Fatalf("%s exposes forbidden local authority-selection method %s", typ, name)
		}
	}
}

func rawEvidenceCASRecordPath(root, digest string) string {
	return filepath.Join(root, digest[:2], digest+".json")
}

func replaceEvidenceFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newTestBundleStore(t *testing.T, root string) (*BundleStore, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewBundleStore(root, mutation)
}

func newTestObservationStore(t *testing.T, root string) (*ObservationStore, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewObservationStore(root, mutation)
}

func newTestEvidenceRawCAS(t *testing.T, root string, maxBytes int) (*finalauthorityadapter.SecurePrivateCAS, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxBytes, mutation)
}
