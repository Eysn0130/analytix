//go:build darwin || linux || windows

package evidenceauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

type originalEvidenceVerifierV1 struct {
	key         []byte
	afterVerify func()
}

func (verifier originalEvidenceVerifierV1) KeyID() string {
	return domainsecurity.SHA256Hex(verifier.key)
}
func (verifier originalEvidenceVerifierV1) PublicKey() []byte {
	return append([]byte(nil), verifier.key...)
}
func (verifier originalEvidenceVerifierV1) VerifyTrusted(ctx context.Context, keyID string, key, body, signature []byte) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if keyID != verifier.KeyID() || !bytes.Equal(key, verifier.key) || !ed25519.Verify(verifier.key, body, signature) {
		return errors.New("original test installation key mismatch")
	}
	if verifier.afterVerify != nil {
		verifier.afterVerify()
	}
	return context.Cause(ctx)
}

func originalEvidenceGraphFixtureV1(t *testing.T) (evidenceAuthorityStoreFixture, string, *PreparedRecoveryV1, OriginalGraphV1) {
	t.Helper()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "evidence-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := NewBundleStore(filepath.Join(root, bundlesLeafV1), access)
	if err != nil {
		t.Fatal(err)
	}
	defer bundles.Close()
	observations, err := NewObservationStore(filepath.Join(root, observationsLeafV1), access)
	if err != nil {
		t.Fatal(err)
	}
	defer observations.Close()
	fixture := newEvidenceAuthorityStoreFixture(102)
	first := fixture.firstBundle(t, "original-graph")
	selected := fixture.nextBundle(t, first, "dataset", "observed")
	sibling := fixture.nextBundle(t, first, "registry", "unobserved-sibling")
	graph := OriginalGraphV1{Bundles: map[string]domainevidence.EvidenceAuthorityBundleV1{}, Observations: map[string]storeport.ObservationBundle{}}
	for _, bundle := range []domainevidence.EvidenceAuthorityBundleV1{first, selected, sibling} {
		if err := bundles.PutIfAbsent(ctx, bundle); err != nil {
			t.Fatal(err)
		}
		graph.Bundles[bundle.RecordDigest] = bundle
	}
	for _, bundle := range []domainevidence.EvidenceAuthorityBundleV1{first, selected} {
		observation := fixture.observationBundle(t, bundle, bundle.RecordDigest)
		if err := observations.PutIfAbsent(ctx, observation); err != nil {
			t.Fatal(err)
		}
		graph.Observations[observation.Observation.ObservationDigest] = observation
	}
	prepared, err := PrepareRecoveryV1(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, root, prepared, graph
}

func TestOriginalEvidenceAuthorityGraphRetainsUnselectedBranchAndCompleteExchanges(t *testing.T) {
	fixture, _, prepared, expected := originalEvidenceGraphFixtureV1(t)
	graph, err := prepared.ObserveOriginalGraphV1(context.Background(), fixture.installationID, fixture.enrollmentID,
		domainsecurity.SHA256Hex(fixture.witnessPublic), fixture.witnessPublic, originalEvidenceVerifierV1{key: fixture.authorityPublic})
	if err != nil || !reflect.DeepEqual(graph, expected) {
		t.Fatalf("complete original graph lost local branch or exchange: bundles=%d observations=%d err=%v", len(graph.Bundles), len(graph.Observations), err)
	}
}

func TestOriginalEvidenceAuthorityGraphRejectsForeignTrust(t *testing.T) {
	for _, changed := range []string{"installation", "enrollment", "authority", "witness"} {
		t.Run(changed, func(t *testing.T) {
			fixture, _, prepared, _ := originalEvidenceGraphFixtureV1(t)
			installation, enrollment := fixture.installationID, fixture.enrollmentID
			authority, witness := fixture.authorityPublic, fixture.witnessPublic
			foreign := newEvidenceAuthorityStoreFixture(104)
			switch changed {
			case "installation":
				installation = foreign.installationID
			case "enrollment":
				enrollment = foreign.enrollmentID
			case "authority":
				authority = foreign.authorityPublic
			case "witness":
				witness = foreign.witnessPublic
			}
			graph, err := prepared.ObserveOriginalGraphV1(context.Background(), installation, enrollment,
				domainsecurity.SHA256Hex(witness), witness, originalEvidenceVerifierV1{key: authority})
			if err == nil || graph.Bundles != nil || graph.Observations != nil {
				t.Fatalf("foreign %s accepted or leaked provisional graph: %v", changed, err)
			}
		})
	}
}

func TestOriginalEvidenceAuthorityGraphRejectsMissingAncestryAndPhysicalDrift(t *testing.T) {
	for _, changed := range []string{"predecessor", "observed bundle", "late physical drift", "cancel"} {
		t.Run(changed, func(t *testing.T) {
			fixture, root, prepared, expected := originalEvidenceGraphFixtureV1(t)
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cause := errors.New("original evidence graph cancelled")
			verifier := originalEvidenceVerifierV1{key: fixture.authorityPublic}
			switch changed {
			case "predecessor", "observed bundle":
				for digest, bundle := range expected.Bundles {
					if (changed == "predecessor" && bundle.Generation == 1) || (changed == "observed bundle" && bundle.DatasetSnapshotCount == 1) {
						if err := os.Remove(rawEvidenceCASRecordPath(filepath.Join(root, bundlesLeafV1), digest)); err != nil {
							t.Fatal(err)
						}
						break
					}
				}
				access, err := privatecastest.NewAccessAuthority(root)
				if err != nil {
					t.Fatal(err)
				}
				prepared, err = PrepareRecoveryV1(ctx, root, access)
				if err != nil {
					t.Fatal(err)
				}
			case "late physical drift":
				verifier.afterVerify = func() {
					if err := os.WriteFile(filepath.Join(root, "unexpected-original-entry"), []byte("synthetic drift"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			case "cancel":
				verifier.afterVerify = func() { cancel(cause) }
			}
			graph, err := prepared.ObserveOriginalGraphV1(ctx, fixture.installationID, fixture.enrollmentID,
				domainsecurity.SHA256Hex(fixture.witnessPublic), fixture.witnessPublic, verifier)
			if err == nil || graph.Bundles != nil || graph.Observations != nil {
				t.Fatalf("broken %s accepted or leaked graph: %v", changed, err)
			}
			if changed == "cancel" && !errors.Is(err, cause) {
				t.Fatalf("original cancellation cause lost: %v", err)
			}
		})
	}
}

func TestOriginalEvidenceAuthorityRawEndpointsRetainOpaquePairAndRejectBrokenGrammar(t *testing.T) {
	fixture, root, _, expected := originalEvidenceGraphFixtureV1(t)
	var digest string
	for key := range expected.Bundles {
		digest = key
		break
	}
	record := rawEvidenceCASRecordPath(filepath.Join(root, bundlesLeafV1), digest)
	pairedName := "." + digest + ".json-0123456789abcdef01234567.tmp"
	if err := os.Link(record, filepath.Join(filepath.Dir(record), pairedName)); err != nil {
		t.Fatal(err)
	}
	// An unrelated opaque producer residue must have its own target address;
	// a second temp naming a paired canonical record violates native topology.
	opaqueDigest := digest[:2] + domainsecurity.SHA256Hex([]byte("unpaired opaque original"))[2:]
	opaqueName := "." + opaqueDigest + ".json-fedcba9876543210fedcba98.tmp"
	opaqueBody := []byte("uninterpreted original evidence residue")
	if err := os.WriteFile(filepath.Join(filepath.Dir(record), opaqueName), opaqueBody, 0o600); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	files, err := prepared.SnapshotOriginalFilesV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files[bundlesLeafV1+"/"+digest[:2]+"/"+opaqueName].Body, opaqueBody) ||
		!bytes.Equal(files[bundlesLeafV1+"/"+digest[:2]+"/"+pairedName].Body, files[bundlesLeafV1+"/"+digest[:2]+"/"+digest+".json"].Body) {
		t.Fatal("raw original snapshot discarded residue bytes")
	}
	parse := func(input map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1) (OriginalGraphV1, error) {
		return ParseOriginalGraphV1(context.Background(), input, fixture.installationID, fixture.enrollmentID, domainsecurity.SHA256Hex(fixture.witnessPublic), fixture.witnessPublic, originalEvidenceVerifierV1{key: fixture.authorityPublic})
	}
	graph, err := parse(files)
	if err != nil || !reflect.DeepEqual(graph, expected) {
		t.Fatalf("full raw graph acquired residue authority or lost history: %v", err)
	}
	for _, fault := range []string{"missing root", "missing leaf", "unknown owner entry", "invalid mode", "misaddressed body"} {
		t.Run(fault, func(t *testing.T) {
			candidate := map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1{}
			for name, value := range files {
				candidate[name] = value
			}
			switch fault {
			case "missing root":
				delete(candidate, ".")
			case "missing leaf":
				delete(candidate, observationsLeafV1)
			case "unknown owner entry":
				candidate["unowned"] = finalauthorityadapter.SecurePrivateCASOriginalEntryV1{Directory: true, Mode: 0o700}
			case "invalid mode":
				entry := candidate["."]
				entry.Mode = 0o4000
				candidate["."] = entry
			case "misaddressed body":
				for key, bundle := range expected.Bundles {
					if key != digest {
						body, err := domainevidence.EvidenceAuthorityBundleV1Bytes(bundle)
						if err != nil {
							t.Fatal(err)
						}
						entry := candidate[bundlesLeafV1+"/"+digest[:2]+"/"+digest+".json"]
						entry.Body = body
						candidate[bundlesLeafV1+"/"+digest[:2]+"/"+digest+".json"] = entry
						break
					}
				}
			}
			graph, err := parse(candidate)
			if err == nil || graph.Bundles != nil || graph.Observations != nil {
				t.Fatalf("broken raw endpoint returned trusted graph: %v", err)
			}
		})
	}
	if graph, err := parse(files); err != nil || !reflect.DeepEqual(graph, expected) {
		t.Fatalf("candidate validation changed original endpoint: %v", err)
	}
}
