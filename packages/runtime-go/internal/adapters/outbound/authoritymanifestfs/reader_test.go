package authoritymanifestfs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	secureconfigfs "analytix.local/runtime-go/internal/adapters/outbound/secureconfigfs"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityanchorport "analytix.local/runtime-go/internal/ports/authorityanchor"
)

type anchorSourceFunc func(context.Context) (authorityanchorport.AnchorV1, error)

func (function anchorSourceFunc) Load(ctx context.Context) (authorityanchorport.AnchorV1, error) {
	return function(ctx)
}

type anchorSourceStub struct {
	anchors []authorityanchorport.AnchorV1
	calls   int
}

func (source *anchorSourceStub) Load(context.Context) (authorityanchorport.AnchorV1, error) {
	index := source.calls
	if index >= len(source.anchors) {
		index = len(source.anchors) - 1
	}
	source.calls++
	anchor := source.anchors[index]
	anchor.AuthorityPublicKey = append([]byte(nil), anchor.AuthorityPublicKey...)
	return anchor, nil
}

func TestReaderReturnsOnlyIndependentlyAnchoredManifest(t *testing.T) {
	manifest, anchor := manifestFixture(t)
	root := manifestRoot(t, manifest)
	source := &anchorSourceStub{anchors: []authorityanchorport.AnchorV1{anchor}}
	anchored, err := (Reader{Root: root, Name: "manifest.json", Anchor: source}).
		loadAnchoredV1ForMigrationWith(context.Background(), readManifestFixtureV1)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV1(anchored, domainenrollment.ThreadRiskNamespaceV1)
	if err != nil || projection.ManifestDigest != manifest.ManifestDigest || projection.InstallationID != anchor.InstallationID {
		t.Fatalf("anchored manifest projection mismatch: projection=%#v err=%v", projection, err)
	}
}

func TestReaderV2IsProductionOnlyAndRejectsV1Downgrade(t *testing.T) {
	manifestV2, anchorV2 := manifestV2Fixture(t)
	bodyV2, err := domainenrollment.ManifestV2Bytes(manifestV2)
	if err != nil {
		t.Fatal(err)
	}
	rootV2 := manifestBodyRoot(t, bodyV2)
	readerV2 := Reader{Root: rootV2, Name: "manifest.json", Anchor: &anchorSourceStub{anchors: []authorityanchorport.AnchorV1{anchorV2}}}
	anchored, err := readerV2.loadAnchoredV2With(context.Background(), readManifestFixtureV1)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(anchored, domainenrollment.ThreadRiskNamespaceV1)
	if err != nil || projection.ManifestDigest != manifestV2.ManifestDigest || projection.CredentialProfileGeneration != 1 {
		t.Fatalf("production manifest V2 projection mismatch: %#v err=%v", projection, err)
	}
	if _, err := readerV2.loadAnchoredV1ForMigrationWith(context.Background(), readManifestFixtureV1); err == nil {
		t.Fatal("production V2 manifest was accepted by the V1 migration parser")
	}

	manifestV1, anchorV1 := manifestFixture(t)
	rootV1 := manifestRoot(t, manifestV1)
	readerV1 := Reader{Root: rootV1, Name: "manifest.json", Anchor: &anchorSourceStub{anchors: []authorityanchorport.AnchorV1{anchorV1}}}
	if _, err := readerV1.loadAnchoredV2With(context.Background(), readManifestFixtureV1); err == nil {
		t.Fatal("legacy V1 manifest entered the production V2 loader")
	}
}

func TestReaderRejectsAnchorRollbackOrMidReadChange(t *testing.T) {
	manifest, anchor := manifestFixture(t)
	root := manifestRoot(t, manifest)
	t.Run("wrong current digest", func(t *testing.T) {
		wrong := anchor
		wrong.CurrentManifestDigest = domainsecurity.SHA256Hex([]byte("older-manifest"))
		_, err := (Reader{Root: root, Name: "manifest.json", Anchor: &anchorSourceStub{anchors: []authorityanchorport.AnchorV1{wrong}}}).
			loadAnchoredV1ForMigrationWith(context.Background(), readManifestFixtureV1)
		if err == nil {
			t.Fatal("rollbackable manifest selected itself without the independent current digest")
		}
	})
	t.Run("anchor changed during read", func(t *testing.T) {
		changed := anchor
		changed.CurrentManifestDigest = domainsecurity.SHA256Hex([]byte("replacement"))
		_, err := (Reader{Root: root, Name: "manifest.json", Anchor: &anchorSourceStub{anchors: []authorityanchorport.AnchorV1{anchor, changed}}}).
			loadAnchoredV1ForMigrationWith(context.Background(), readManifestFixtureV1)
		if err == nil {
			t.Fatal("manifest read crossed an independent anchor change")
		}
	})
}

func TestReaderPreservesSecondAnchorErrorAndCancellation(t *testing.T) {
	manifest, anchor := manifestFixture(t)
	root := manifestRoot(t, manifest)
	t.Run("second source error", func(t *testing.T) {
		sentinel := errors.New("protected anchor source failed")
		calls := 0
		source := anchorSourceFunc(func(context.Context) (authorityanchorport.AnchorV1, error) {
			calls++
			if calls == 2 {
				return authorityanchorport.AnchorV1{}, sentinel
			}
			return anchor, nil
		})
		_, err := (Reader{Root: root, Name: "manifest.json", Anchor: source}).
			loadAnchoredV1ForMigrationWith(context.Background(), readManifestFixtureV1)
		if !errors.Is(err, sentinel) {
			t.Fatalf("second anchor error was hidden: %v", err)
		}
	})
	t.Run("cancelled by second source", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		source := anchorSourceFunc(func(context.Context) (authorityanchorport.AnchorV1, error) {
			calls++
			if calls == 2 {
				cancel()
			}
			return anchor, nil
		})
		_, err := (Reader{Root: root, Name: "manifest.json", Anchor: source}).
			loadAnchoredV1ForMigrationWith(ctx, readManifestFixtureV1)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("manifest loader ignored cancellation after second anchor read: %v", err)
		}
	})
}

func manifestFixture(t *testing.T) (domainenrollment.ManifestV1, authorityanchorport.AnchorV1) {
	t.Helper()
	installationPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x11}, ed25519.SeedSize))
	installationPublic := installationPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("installation"))
	threadPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x21}, ed25519.SeedSize))
	sharedPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, ed25519.SeedSize))
	threadEnrollment := domainsecurity.SHA256Hex([]byte("thread-enrollment"))
	sharedEnrollment := domainsecurity.SHA256Hex([]byte("shared-enrollment"))
	manifest, err := domainenrollment.NewManifestV1(domainenrollment.ManifestInputV1{
		InstallationID: installationID, InstallationAuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		InstallationAuthorityPublicKey: installationPublic, IssuedAt: time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		ThreadRisk: domainenrollment.WitnessEnrollmentInputV1{
			EnrollmentID: threadEnrollment, EndpointOrigin: "https://risk.example.test",
			WitnessKeyID: domainsecurity.SHA256Hex(threadPrivate.Public().(ed25519.PublicKey)), WitnessPublicKey: threadPrivate.Public().(ed25519.PublicKey),
			RootCASHA256: domainsecurity.SHA256Hex([]byte("risk-root")), ServerName: "risk.example.test", TimeoutMS: 1_000,
			InitialCheckpoint: manifestCheckpoint(t, installationID, threadEnrollment, domainenrollment.ThreadRiskNamespaceV1, threadPrivate),
		},
		SharedEvidence: domainenrollment.WitnessEnrollmentInputV1{
			EnrollmentID: sharedEnrollment, EndpointOrigin: "https://evidence.example.test",
			WitnessKeyID: domainsecurity.SHA256Hex(sharedPrivate.Public().(ed25519.PublicKey)), WitnessPublicKey: sharedPrivate.Public().(ed25519.PublicKey),
			RootCASHA256: domainsecurity.SHA256Hex([]byte("evidence-root")), ServerName: "evidence.example.test", TimeoutMS: 1_000,
			InitialCheckpoint: manifestCheckpoint(t, installationID, sharedEnrollment, domainenrollment.SharedEvidenceNamespaceV1, sharedPrivate),
		},
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return manifest, authorityanchorport.AnchorV1{
		InstallationID: installationID, AuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		AuthorityPublicKey: append([]byte(nil), installationPublic...), CurrentManifestDigest: manifest.ManifestDigest,
	}
}

func manifestCheckpoint(t *testing.T, installationID, enrollmentID, namespace string, privateKey ed25519.PrivateKey) domainsecurity.MonotonicHeadCheckpointV1 {
	t.Helper()
	publicKey := privateKey.Public().(ed25519.PublicKey)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: namespace, Generation: 0,
		CurrentStateDigest: domainsecurity.SHA256Hex([]byte("state:" + namespace)), FenceNonce: domainsecurity.SHA256Hex([]byte("fence:" + namespace)),
		WitnessKeyID: domainsecurity.SHA256Hex(publicKey), WitnessPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func manifestRoot(t *testing.T, manifest domainenrollment.ManifestV1) string {
	t.Helper()
	body, err := domainenrollment.ManifestV1Bytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return manifestBodyRoot(t, body)
}

func manifestBodyRoot(t *testing.T, body []byte) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "manifest-root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "manifest.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return realRoot
}

func readManifestFixtureV1(input secureconfigfs.ReadExactInput) ([]byte, error) {
	if input.Target != "manifest.json" || len(input.AllowedNames) != 1 ||
		input.AllowedNames[0] != input.Target || input.MaxBytes <= 0 {
		return nil, errors.New("manifest test reader received an invalid exact inventory")
	}
	body, err := os.ReadFile(filepath.Join(input.Root, input.Target))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > input.MaxBytes {
		return nil, errors.New("manifest test reader exceeded its byte bound")
	}
	return body, nil
}

func manifestV2Fixture(t *testing.T) (domainenrollment.ManifestV2, authorityanchorport.AnchorV1) {
	t.Helper()
	legacy, _ := manifestFixture(t)
	installationPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	installationPublic := installationPrivate.Public().(ed25519.PublicKey)
	manifest, err := domainenrollment.NewManifestV2(domainenrollment.ManifestInputV2{
		InstallationID: legacy.InstallationID, InstallationAuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		InstallationAuthorityPublicKey: installationPublic, CredentialProfileGeneration: 1,
		CredentialProfileDigest: domainsecurity.SHA256Hex([]byte("credential-profile-v1")), IssuedAt: legacy.IssuedAt.Add(time.Minute),
		ThreadRisk: enrollmentInputV1(t, legacy.ThreadRisk), SharedEvidence: enrollmentInputV1(t, legacy.SharedEvidence),
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(installationPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return manifest, authorityanchorport.AnchorV1{
		InstallationID: legacy.InstallationID, AuthorityKeyID: domainsecurity.SHA256Hex(installationPublic),
		AuthorityPublicKey: append([]byte(nil), installationPublic...), CurrentManifestDigest: manifest.ManifestDigest,
	}
}

func enrollmentInputV1(t *testing.T, enrollment domainenrollment.WitnessEnrollmentV1) domainenrollment.WitnessEnrollmentInputV1 {
	t.Helper()
	publicKey, err := base64.RawURLEncoding.DecodeString(enrollment.WitnessPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return domainenrollment.WitnessEnrollmentInputV1{
		EnrollmentID: enrollment.EnrollmentID, EndpointOrigin: enrollment.EndpointOrigin,
		WitnessKeyID: enrollment.WitnessKeyID, WitnessPublicKey: publicKey,
		RootCASHA256:                        enrollment.RootCASHA256,
		MTLSClientIdentityCertificateSHA256: enrollment.MTLSClientIdentityCertificateSHA256,
		ServerName:                          enrollment.ServerName, TimeoutMS: enrollment.TimeoutMS, InitialCheckpoint: enrollment.InitialCheckpoint,
	}
}
