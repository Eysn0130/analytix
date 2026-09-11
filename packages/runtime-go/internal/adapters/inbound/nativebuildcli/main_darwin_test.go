//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativebuildcli

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStandaloneJSONCannotMintNativeReleaseEligibility(t *testing.T) {
	repository := t.TempDir()
	publication := filepath.Join(repository, "runtime", "native-components", hostTargetV1())
	request := RequestV1{
		Kind: RequestKindV1, SchemaVersion: SchemaVersionV1,
		RequestNonce: strings.Repeat("a", 64), RepositoryRoot: repository,
		PublicationRoot: publication, TargetKey: hostTargetV1(),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRequestV1(body)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != request {
		t.Fatalf("decoded request drifted: %#v", decoded)
	}

	for name, mutated := range map[string][]byte{
		"caller release eligible": append(body[:len(body)-1], []byte(`,"release_eligible":true}`)...),
		"caller cargo receipt":    append(body[:len(body)-1], []byte(`,"cargo_execution_receipt":{"releaseEligible":true}}`)...),
		"duplicate nonce":         []byte(strings.Replace(string(body), `"request_nonce":"`, `"request_nonce":"`+strings.Repeat("b", 64)+`","request_nonce":"`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRequestV1(mutated); err == nil {
				t.Fatal("authority-shaped caller JSON was accepted")
			}
		})
	}
}

func TestCoordinatorRequestMustBeCanonical(t *testing.T) {
	request := RequestV1{
		Kind: RequestKindV1, SchemaVersion: SchemaVersionV1,
		RequestNonce: strings.Repeat("a", 64), RepositoryRoot: t.TempDir(),
		PublicationRoot: filepath.Join(t.TempDir(), "native"), TargetKey: hostTargetV1(),
	}
	body, _ := json.Marshal(request)
	if _, err := DecodeRequestV1(append([]byte(" "), body...)); err == nil {
		t.Fatal("noncanonical request accepted")
	}
}

func TestCoordinatorResponseCarriesComponentReceiptDigest(t *testing.T) {
	digest := strings.Repeat("c", 64)
	body, err := json.Marshal(ResponseV1{
		Kind: ResponseKindV1, SchemaVersion: SchemaVersionV1, Status: "published",
		RequestNonce: strings.Repeat("a", 64), ComponentReceiptSHA256: digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["component_receipt_sha256"] != digest {
		t.Fatalf("component receipt digest missing from coordinator response: %s", body)
	}
}

func hostTargetV1() string {
	if runtime.GOARCH == "arm64" {
		return "darwin-arm64"
	}
	return "darwin-x64"
}
