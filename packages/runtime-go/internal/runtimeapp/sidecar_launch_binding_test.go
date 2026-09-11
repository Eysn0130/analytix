package runtimeapp

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestControlledArtifactSidecarLaunchBindingV2IsAbsentOrExact(t *testing.T) {
	absent, err := ControlledArtifactSidecarLaunchBindingProofV2(
		Config{}, "http://127.0.0.1:45123/", 4242, true, false,
	)
	if err != nil || absent.Configured || absent.Ready || absent.BackendGeneration != 0 || absent.Proof != "" {
		t.Fatalf("absent V2 authority produced a launch binding: %#v err=%v", absent, err)
	}
	if _, err := ControlledArtifactSidecarLaunchBindingProofV2(
		Config{}, "http://127.0.0.1:45123/", 4242, true, true,
	); err == nil {
		t.Fatal("absent V2 authority was accepted as ready")
	}
	secret := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	rootDER := controlledArtifactHostV2RootCertificate(t)
	binding, err := ControlledArtifactSidecarLaunchBindingProofV2(Config{
		ControlledArtifactHostV2URL: "https://127.0.0.1:45124", ControlledArtifactHostV2Token: secret,
		ControlledArtifactHostV2BackendGeneration: "17",
		ControlledArtifactHostV2AllocationDigest:  strings.Repeat("d", 64),
		ControlledArtifactHostV2TLSRootCertDER:    base64.RawURLEncoding.EncodeToString(rootDER),
		ControlledArtifactHostV2TLSLeafSPKISHA256: strings.Repeat("a", 64),
	}, "http://127.0.0.1:45123/", 4242, true, false)
	if err != nil || !binding.Configured || binding.Ready || binding.BackendGeneration != 17 || len(binding.Proof) != 64 {
		t.Fatalf("unexpected V2 launch binding: %#v err=%v", binding, err)
	}
	if _, err := hex.DecodeString(binding.Proof); err != nil {
		t.Fatalf("launch binding proof is not canonical hex: %q", binding.Proof)
	}
}
