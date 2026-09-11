package sidecarlaunch

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestProofV2BindsEverySidecarAuthorityField(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	binding := BindingV2{
		RuntimeURL: "http://127.0.0.1:45123/", RuntimePID: 4242,
		ControlledArtifactHostURL: "https://127.0.0.1:45124",
		BackendGeneration:         17, AllocationRecordDigest: strings.Repeat("d", 64),
		TLSRootCertificateSHA256: strings.Repeat("b", 64),
		TLSLeafSPKISHA256:        strings.Repeat("a", 64),
		ControlledArtifactReady:  true,
		RuntimeTokenConfigured:   true, PersistenceRootsConfigured: true, ProductionRuntime: true,
	}
	proof, err := ProofV2(secret, binding)
	if err != nil {
		t.Fatal(err)
	}
	if proof != "5e055acc7f72b3c10dba94edd2375e1d22d3e62d05ac6c04b8cd1a1863ff5431" {
		t.Fatalf("launch binding proof changed: %s", proof)
	}

	mutations := []func(*BindingV2){
		func(value *BindingV2) { value.RuntimeURL = "http://127.0.0.1:45125/" },
		func(value *BindingV2) { value.RuntimePID++ },
		func(value *BindingV2) { value.ControlledArtifactHostURL = "https://127.0.0.1:45126" },
		func(value *BindingV2) { value.BackendGeneration++ },
		func(value *BindingV2) { value.AllocationRecordDigest = strings.Repeat("e", 64) },
		func(value *BindingV2) { value.TLSRootCertificateSHA256 = strings.Repeat("c", 64) },
		func(value *BindingV2) { value.TLSLeafSPKISHA256 = strings.Repeat("b", 64) },
		func(value *BindingV2) { value.ControlledArtifactReady = false },
	}
	for index, mutate := range mutations {
		candidate := binding
		mutate(&candidate)
		other, candidateErr := ProofV2(secret, candidate)
		if candidateErr != nil || other == proof {
			t.Fatalf("mutation %d did not change proof: proof=%s err=%v", index, other, candidateErr)
		}
	}
}

func TestProofV2RejectsWeakSecretUnsafeOriginsAndNonProductionState(t *testing.T) {
	valid := BindingV2{
		RuntimeURL: "http://[::1]:45123/", RuntimePID: 4242,
		ControlledArtifactHostURL: "https://127.0.0.1:45124",
		BackendGeneration:         1, AllocationRecordDigest: strings.Repeat("d", 64),
		TLSRootCertificateSHA256: strings.Repeat("b", 64),
		TLSLeafSPKISHA256:        strings.Repeat("a", 64),
		RuntimeTokenConfigured:   true, PersistenceRootsConfigured: true, ProductionRuntime: true,
	}
	secret := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	for _, test := range []struct {
		secret string
		mutate func(*BindingV2)
	}{
		{secret: "weak", mutate: func(*BindingV2) {}},
		{secret: secret, mutate: func(value *BindingV2) { value.RuntimeURL = "http://localhost:45123/" }},
		{secret: secret, mutate: func(value *BindingV2) { value.RuntimePID = 0 }},
		{secret: secret, mutate: func(value *BindingV2) { value.ControlledArtifactHostURL = "http://127.0.0.1:45124" }},
		{secret: secret, mutate: func(value *BindingV2) { value.BackendGeneration = 0 }},
		{secret: secret, mutate: func(value *BindingV2) { value.AllocationRecordDigest = strings.Repeat("D", 64) }},
		{secret: secret, mutate: func(value *BindingV2) { value.RuntimeTokenConfigured = false }},
		{secret: secret, mutate: func(value *BindingV2) { value.PersistenceRootsConfigured = false }},
		{secret: secret, mutate: func(value *BindingV2) { value.ProductionRuntime = false }},
	} {
		candidate := valid
		test.mutate(&candidate)
		if proof, err := ProofV2(test.secret, candidate); err == nil || proof != "" {
			t.Fatalf("invalid launch binding was accepted: proof=%q err=%v", proof, err)
		}
	}
}
