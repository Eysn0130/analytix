package evidence

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

//go:embed testdata/accepted-final-v6-host-local.json
var acceptedFinalHostLocalV6Golden []byte

func testHostLocalFactFinalV6(t *testing.T, witness FactFinalWitnessAdmissionInputV1,
	intent TerminalPublicationIntent, sign func([]byte) ([]byte, error)) {
	t.Helper()
	genesis, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: witness.InstallationID, RootBindingDigest: domainsecurity.SHA256Hex([]byte("host-local-root-binding")),
		MutationID:                  domainsecurity.SHA256Hex([]byte("host-local-genesis")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainsecurity.SHA256Hex([]byte("host-local-publication-genesis")),
		AuthorityKeyID:              witness.AuthorityKeyID, AuthorityPublicKey: witness.AuthorityPublicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	path := make([]domainsecurity.DatasetSnapshotIndexV1, len(witness.DatasetIndexPath))
	for offset := len(path) - 1; offset >= 0; offset-- {
		original := witness.DatasetIndexPath[offset]
		previous := domainsecurity.DatasetSnapshotIndexGenesisDigestV1()
		if offset+1 < len(path) {
			previous = path[offset+1].IndexDigest
		}
		path[offset], err = domainsecurity.NewDatasetSnapshotIndexHostLocalV2(domainsecurity.DatasetSnapshotIndexHostLocalInputV2{
			InstallationID: witness.InstallationID, ModeCommitmentDigest: genesis.RecordDigest, Generation: original.Generation,
			PreviousIndexDigest: previous, MutationID: original.MutationID, Binding: original.Binding, SnapshotRecordDigest: original.SnapshotRecordDigest,
			AuthorityKeyID: witness.AuthorityKeyID, AuthorityPublicKey: witness.AuthorityPublicKey,
		}, sign)
		if err != nil {
			t.Fatal(err)
		}
	}
	registry, err := NewEvidenceRegistryAuthorityIndexHostLocalV3(EvidenceRegistryAuthorityIndexHostLocalInputV3{
		InstallationID: witness.InstallationID, ModeCommitmentDigest: genesis.RecordDigest, Generation: 1,
		PreviousIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigestV2(), MutationID: domainsecurity.SHA256Hex([]byte("host-local-registry")),
	}, witness.SelectedCapsule, witness.AuthorityKeyID, witness.AuthorityPublicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainhost.NewHeadV1(domainhost.HeadInputV1{
		InstallationID: witness.InstallationID, RootBindingDigest: genesis.RootBindingDigest, Generation: 1,
		PreviousHeadDigest: genesis.RecordDigest, MutationID: domainsecurity.SHA256Hex([]byte("host-local-selected-head")),
		DatasetSnapshotIndexDigest: path[0].IndexDigest, DatasetSnapshotCount: uint64(len(path)),
		EvidenceRegistryIndexDigest: registry.IndexDigest, EvidenceRegistryCount: 1, PublicationIndexDigest: genesis.PublicationIndexDigest,
		AuthorityKeyID: witness.AuthorityKeyID, AuthorityPublicKey: witness.AuthorityPublicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	input := FactFinalHostLocalAdmissionInputV1{Context: witness.Context, Envelope: witness.Envelope, RenderedText: witness.RenderedText,
		PublicationProof: witness.PublicationProof, Registry: witness.Registry, Head: head, ModeCommitment: genesis,
		RegistryIndexPath: []EvidenceRegistryAuthorityIndexV2{registry}, SelectedCapsule: witness.SelectedCapsule,
		DatasetIndexPath: path, SelectedDatasetIndex: path[0], DatasetRecord: witness.DatasetRecord, DatasetManifest: witness.DatasetManifest,
		FundsProducerContent: witness.FundsProducerContent, BindingObservation: witness.BindingObservation,
		InstallationID: witness.InstallationID, RootBindingDigest: genesis.RootBindingDigest, AuthorityKeyID: witness.AuthorityKeyID,
		AuthorityPublicKey: witness.AuthorityPublicKey, AdmittedAt: witness.AdmittedAt}
	admission, err := NewFactFinalHostLocalAdmissionV1(input)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*FactFinalHostLocalAdmissionInputV1){
		"witnessed dataset":     func(v *FactFinalHostLocalAdmissionInputV1) { v.DatasetIndexPath = witness.DatasetIndexPath },
		"witnessed registry":    func(v *FactFinalHostLocalAdmissionInputV1) { v.RegistryIndexPath = witness.RegistryIndexPath },
		"wrong binding":         func(v *FactFinalHostLocalAdmissionInputV1) { v.BindingObservation.CaseID = "other-case" },
		"missing retained path": func(v *FactFinalHostLocalAdmissionInputV1) { v.DatasetIndexPath = path[:1] },
		"wrong root": func(v *FactFinalHostLocalAdmissionInputV1) {
			v.RootBindingDigest = domainsecurity.SHA256Hex([]byte("other-root"))
		},
		"proof after admission": func(v *FactFinalHostLocalAdmissionInputV1) { v.AdmittedAt = witness.AdmittedAt.Add(-3 * time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := input
			mutate(&candidate)
			if _, err := NewFactFinalHostLocalAdmissionV1(candidate); err == nil {
				t.Fatal("detached host-local authority admitted")
			}
		})
	}
	registryHead, err := NewEvidenceRegistryHead(witness.Registry)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := PrivateAcceptedFinalDigestWithHostLocalV1(input.Context, input.Envelope, input.RenderedText, intent, input.PublicationProof, &admission)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: input.Context, Envelope: input.Envelope, RenderedText: input.RenderedText, RegistryHead: registryHead,
		PublicationSnapshotProof: input.PublicationProof, FactFinalHostLocalAdmission: &admission, FactFinalHostLocalAuthority: &input,
		PrivateRecordDigest: digest, AcceptedAt: input.AdmittedAt, AuthorityKeyID: input.AuthorityKeyID, AuthorityPublicKey: input.AuthorityPublicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	private, err := NewPrivateAcceptedFinalRecord(input.Context, input.Envelope, input.RenderedText, registryHead, intent, accepted, input.PublicationProof)
	if err != nil || private.SchemaVersion != 6 || ValidatePrivateAcceptedFinalRecord(private) != nil {
		t.Fatalf("host-local private Final failed: %v", err)
	}
	for name, mutate := range map[string]func(*AcceptedFinalRecord){
		"missing public view": func(v *AcceptedFinalRecord) { v.PublicView = nil },
		"missing admission":   func(v *AcceptedFinalRecord) { v.FactFinalHostLocalAdmission = nil },
		"witnessed alias":     func(v *AcceptedFinalRecord) { v.FactFinalWitnessAdmission = &FactFinalWitnessAdmissionV1{} },
		"V5 alias":            func(v *AcceptedFinalRecord) { v.SchemaVersion = 5 },
		"boundary alias":      func(v *AcceptedFinalRecord) { v.Variant = GeneralGuidanceAnswer },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := accepted
			mutate(&candidate)
			signature, err := sign(AcceptedFinalSigningBytes(candidate))
			if err != nil {
				t.Fatal(err)
			}
			candidate.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
			candidate.RecordDigest = acceptedFinalRecordDigest(candidate)
			if ValidateAcceptedFinalRecord(candidate) == nil {
				t.Fatal("invalid V6 record admitted")
			}
		})
	}
	if ValidatePrivateAcceptedFinalPublicationAuthority(private) == nil {
		t.Fatal("serialized host-local fact granted live authority")
	}
	body, err := json.Marshal(accepted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, bytes.TrimSpace(acceptedFinalHostLocalV6Golden)) {
		t.Fatalf("Go host-local V6 golden drifted: %s", body)
	}
}
