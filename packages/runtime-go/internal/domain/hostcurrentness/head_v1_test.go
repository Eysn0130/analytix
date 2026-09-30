package hostcurrentness_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestHostLocalHeadCanonicalChainAndModeBinding(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	input := domainhost.HeadInputV1{
		InstallationID:              domainsecurity.SHA256Hex([]byte("installation")),
		RootBindingDigest:           domainsecurity.SHA256Hex([]byte("root")),
		MutationID:                  domainsecurity.SHA256Hex([]byte("mode")),
		DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		AuthorityKeyID:              domainsecurity.SHA256Hex(public), AuthorityPublicKey: public,
	}
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(private, body), nil }
	genesis, err := domainhost.NewHeadV1(input, sign)
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainhost.HeadBytesV1(genesis)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domainhost.ParseHeadV1(body)
	if err != nil || parsed != genesis || domainhost.ValidateHeadForInstallationV1(parsed, input.InstallationID, input.RootBindingDigest, input.AuthorityKeyID, public) != nil {
		t.Fatalf("signed mode commitment did not round trip: err=%v", err)
	}
	if domainhost.ValidateHeadForInstallationV1(parsed, input.InstallationID, domainsecurity.SHA256Hex([]byte("other root")), input.AuthorityKeyID, public) == nil {
		t.Fatal("head accepted a different protected root")
	}
	input.Generation, input.PreviousHeadDigest = 1, genesis.RecordDigest
	input.MutationID = domainsecurity.SHA256Hex([]byte("dataset append"))
	input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte("dataset index 1"))
	input.DatasetSnapshotCount = 1
	next, err := domainhost.NewHeadV1(input, sign)
	if err != nil || domainhost.ValidateHeadTransitionV1(genesis, next) != nil {
		t.Fatalf("one exact child append was rejected: err=%v", err)
	}
	input.EvidenceRegistryIndexDigest = domainsecurity.SHA256Hex([]byte("registry index 1"))
	input.EvidenceRegistryCount = 1
	dual, err := domainhost.NewHeadV1(input, sign)
	if err != nil || domainhost.ValidateHeadTransitionV1(genesis, dual) == nil {
		t.Fatal("two-child transition gained local currentness")
	}

	duplicate := bytes.Replace(body, []byte(`"mode":"host_local"`), []byte(`"mode":"host_local","mode":"host_local"`), 1)
	if _, err := domainhost.ParseHeadV1(duplicate); err == nil {
		t.Fatal("duplicate-key mode commitment was accepted")
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["mode"] = "witnessed"
	tampered, _ := json.Marshal(object)
	if _, err := domainhost.ParseHeadV1(tampered); err == nil {
		t.Fatal("tampered mode commitment was accepted")
	}
}
