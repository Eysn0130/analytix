package threadriskauthority

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

func TestIndexStoreCanonicalRoundTripRestartAndNoCurrentAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "indexes")
	store, err := newTestRiskIndexStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newAuthorityStoreFixture(11)
	index := fixture.firstIndex(t, "roundtrip")
	if err := store.PutIfAbsent(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), index); err != nil {
		t.Fatalf("idempotent index write failed: %v", err)
	}
	resolved, err := store.Resolve(context.Background(), index.IndexDigest)
	if err != nil || !equalIndex(resolved, index) {
		t.Fatalf("index round trip mismatch: index=%#v err=%v", resolved, err)
	}
	restarted, err := newTestRiskIndexStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = restarted.Resolve(context.Background(), index.IndexDigest)
	if err != nil || !equalIndex(resolved, index) {
		t.Fatalf("restarted index round trip mismatch: index=%#v err=%v", resolved, err)
	}
	assertNoAuthoritySelectionMethods(t, reflect.TypeOf(store))
}

func TestObservationStoreCanonicalRoundTripRestartAndNoCurrentAPI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "observations")
	store, err := newTestRiskObservationStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newAuthorityStoreFixture(21)
	bundle := fixture.bundle(t, fixture.firstIndex(t, "bundle"), "bundle")
	if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := store.PutIfAbsent(context.Background(), bundle); err != nil {
		t.Fatalf("idempotent observation write failed: %v", err)
	}
	resolved, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest)
	if err != nil || !equalBundle(resolved, bundle) {
		t.Fatalf("observation round trip mismatch: bundle=%#v err=%v", resolved, err)
	}
	restarted, err := newTestRiskObservationStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = restarted.Resolve(context.Background(), bundle.Observation.ObservationDigest)
	if err != nil || !equalBundle(resolved, bundle) {
		t.Fatalf("restarted observation mismatch: bundle=%#v err=%v", resolved, err)
	}
	assertNoAuthoritySelectionMethods(t, reflect.TypeOf(store))
	assertNoAuthoritySelectionMethods(t, reflect.TypeOf(&Projection{}))
}

func TestStoresConcurrentPutIfAbsentAreExactAndIdempotent(t *testing.T) {
	t.Run("index", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		first, err := newTestRiskIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		second, err := newTestRiskIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		index := newAuthorityStoreFixture(31).firstIndex(t, "concurrent-index")
		concurrentExactWrites(t, 40, func(position int) error {
			if position%2 == 0 {
				return first.PutIfAbsent(context.Background(), index)
			}
			return second.PutIfAbsent(context.Background(), index)
		})
		resolved, err := first.Resolve(context.Background(), index.IndexDigest)
		if err != nil || !equalIndex(resolved, index) {
			t.Fatalf("concurrent index record mismatch: index=%#v err=%v", resolved, err)
		}
	})

	t.Run("observation", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		first, err := newTestRiskObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		second, err := newTestRiskObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newAuthorityStoreFixture(41)
		bundle := fixture.bundle(t, fixture.firstIndex(t, "concurrent-bundle"), "concurrent-bundle")
		concurrentExactWrites(t, 40, func(position int) error {
			if position%2 == 0 {
				return first.PutIfAbsent(context.Background(), bundle)
			}
			return second.PutIfAbsent(context.Background(), bundle)
		})
		resolved, err := first.Resolve(context.Background(), bundle.Observation.ObservationDigest)
		if err != nil || !equalBundle(resolved, bundle) {
			t.Fatalf("concurrent observation record mismatch: bundle=%#v err=%v", resolved, err)
		}
	})
}

func TestIndexStoreRejectsConflictNonCanonicalAndAttackerBytes(t *testing.T) {
	fixture := newAuthorityStoreFixture(51)
	index := fixture.firstIndex(t, "corrupt-index")
	canonical, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil {
		t.Fatal(err)
	}
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
			root := filepath.Join(t.TempDir(), "indexes")
			raw, err := newTestThreadRiskRawCAS(t, root, maxThreadRiskAuthorityIndexBytes)
			if err != nil {
				t.Fatal(err)
			}
			if err := raw.PutIfAbsent(context.Background(), index.IndexDigest, body); err != nil {
				t.Fatal(err)
			}
			store, err := newTestRiskIndexStore(t, root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Resolve(context.Background(), index.IndexDigest); err == nil {
				t.Fatal("corrupt index bytes were accepted")
			}
			if err := store.PutIfAbsent(context.Background(), index); err == nil {
				t.Fatal("canonical index overwrote conflicting CAS bytes")
			}
		})
	}

	t.Run("wrong-digest", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "indexes")
		raw, err := newTestThreadRiskRawCAS(t, root, maxThreadRiskAuthorityIndexBytes)
		if err != nil {
			t.Fatal(err)
		}
		wrong := domainsecurity.SHA256Hex([]byte("wrong-index-address"))
		if err := raw.PutIfAbsent(context.Background(), wrong, canonical); err != nil {
			t.Fatal(err)
		}
		store, err := newTestRiskIndexStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), wrong); err == nil {
			t.Fatal("index stored at a wrong digest was accepted")
		}
	})
}

func TestObservationStoreRejectsClosedWireAndMismatchedWitnessBinding(t *testing.T) {
	fixture := newAuthorityStoreFixture(61)
	index := fixture.firstIndex(t, "corrupt-bundle")
	bundle := fixture.bundle(t, index, "corrupt-bundle")
	canonical, err := observationBundleBytes(bundle)
	if err != nil {
		t.Fatal(err)
	}
	variants := map[string][]byte{
		"whitespace": append(append([]byte(nil), canonical...), '\n'),
		"duplicate":  append([]byte(`{"schemaVersion":1,`), canonical[1:]...),
		"unknown":    append(append([]byte(nil), canonical[:len(canonical)-1]...), []byte(`,"unknown":true}`)...),
		"trailing":   append(append([]byte(nil), canonical...), []byte(`[]`)...),
		"truncated":  append([]byte(nil), canonical[:len(canonical)-1]...),
		"attacker":   []byte(`{"schemaVersion":1,"purpose":"analytix.thread-risk-authority-observation-bundle/v1","index":{},"request":{},"observation":{}}`),
	}
	for name, body := range variants {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "observations")
			raw, err := newTestThreadRiskRawCAS(t, root, maxObservationBundleBytes)
			if err != nil {
				t.Fatal(err)
			}
			if err := raw.PutIfAbsent(context.Background(), bundle.Observation.ObservationDigest, body); err != nil {
				t.Fatal(err)
			}
			store, err := newTestRiskObservationStore(t, root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Resolve(context.Background(), bundle.Observation.ObservationDigest); err == nil {
				t.Fatal("corrupt observation bundle was accepted")
			}
		})
	}

	t.Run("mismatched-index", func(t *testing.T) {
		store, err := newTestRiskObservationStore(t, filepath.Join(t.TempDir(), "observations"))
		if err != nil {
			t.Fatal(err)
		}
		mismatch := bundle
		mismatch.Index = fixture.firstIndex(t, "other-index")
		if err := store.PutIfAbsent(context.Background(), mismatch); err == nil {
			t.Fatal("observation bundle accepted an index not selected by its checkpoint")
		}
	})

	t.Run("mismatched-request", func(t *testing.T) {
		store, err := newTestRiskObservationStore(t, filepath.Join(t.TempDir(), "observations"))
		if err != nil {
			t.Fatal(err)
		}
		other := fixture.bundle(t, index, "other-request")
		mismatch := bundle
		mismatch.Request = other.Request
		if err := store.PutIfAbsent(context.Background(), mismatch); err == nil {
			t.Fatal("observation bundle accepted a request not answered by the observation")
		}
	})

	t.Run("wrong-digest", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "observations")
		raw, err := newTestThreadRiskRawCAS(t, root, maxObservationBundleBytes)
		if err != nil {
			t.Fatal(err)
		}
		wrong := domainsecurity.SHA256Hex([]byte("wrong-observation-address"))
		if err := raw.PutIfAbsent(context.Background(), wrong, canonical); err != nil {
			t.Fatal(err)
		}
		store, err := newTestRiskObservationStore(t, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(context.Background(), wrong); err == nil {
			t.Fatal("observation stored at a wrong digest was accepted")
		}
	})
}

func TestStoresHonorCancellationAndStrictDigestInput(t *testing.T) {
	fixture := newAuthorityStoreFixture(71)
	index := fixture.firstIndex(t, "cancel")
	bundle := fixture.bundle(t, index, "cancel")
	indexStore, err := newTestRiskIndexStore(t, filepath.Join(t.TempDir(), "indexes"))
	if err != nil {
		t.Fatal(err)
	}
	observationStore, err := newTestRiskObservationStore(t, filepath.Join(t.TempDir(), "observations"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := indexStore.PutIfAbsent(ctx, index); !errors.Is(err, context.Canceled) {
		t.Fatalf("index write ignored cancellation: %v", err)
	}
	if err := observationStore.PutIfAbsent(ctx, bundle); !errors.Is(err, context.Canceled) {
		t.Fatalf("observation write ignored cancellation: %v", err)
	}
	for _, digest := range []string{"", " " + index.IndexDigest, strings.ToUpper(index.IndexDigest), index.IndexDigest + " "} {
		if _, err := indexStore.Resolve(context.Background(), digest); err == nil {
			t.Fatalf("index store accepted non-canonical digest %q", digest)
		}
		if _, err := observationStore.Resolve(context.Background(), digest); err == nil {
			t.Fatalf("observation store accepted non-canonical digest %q", digest)
		}
	}
}

func concurrentExactWrites(t *testing.T, count int, write func(int) error) {
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

func assertNoAuthoritySelectionMethods(t *testing.T, typ reflect.Type) {
	t.Helper()
	for _, name := range []string{"List", "Current", "ResolveCurrent", "Observe", "Read"} {
		if _, found := typ.MethodByName(name); found {
			t.Fatalf("%s exposes forbidden local authority-selection method %s", typ, name)
		}
	}
}

func rawCASRecordPath(root, digest string) string {
	return filepath.Join(root, digest[:2], digest+".json")
}

func replaceWithFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func canonicalIndexBody(t *testing.T, index domainsecurity.ThreadRiskAuthorityIndexV1) []byte {
	t.Helper()
	body, err := domainsecurity.ThreadRiskAuthorityIndexV1Bytes(index)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(body)
}

func newTestRiskIndexStore(t *testing.T, root string) (*IndexStore, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewIndexStore(root, mutation)
}

func newTestRiskObservationStore(t *testing.T, root string) (*ObservationStore, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return NewObservationStore(root, mutation)
}

func newTestThreadRiskRawCAS(t *testing.T, root string, maxBytes int) (*finalauthorityadapter.SecurePrivateCAS, error) {
	t.Helper()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return finalauthorityadapter.OpenSecurePrivateCASWithAccessAuthority(root, maxBytes, mutation)
}
