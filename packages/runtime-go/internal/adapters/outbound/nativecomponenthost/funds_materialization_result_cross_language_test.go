//go:build analytix_funds_cross_language

package nativecomponenthost

import (
	"os"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRustTypedFundsProducerBytesMatchGoCanonicalV1(t *testing.T) {
	body := []byte(os.Getenv("ANALYTIX_FPC1_TYPED_RESULT_JSON"))
	if len(body) == 0 {
		t.Fatal("ANALYTIX_FPC1_TYPED_RESULT_JSON is required")
	}
	result, err := domainsecurity.ParseFundsMaterializationResultV1(body)
	if err != nil {
		t.Fatalf("Rust typed materialization result failed Go verification: %v", err)
	}
	if result.ProducerContentID == "" ||
		result.ProducerContentID != domainsecurity.DeriveFundsProducerContentIDV1(result.ProducerContentManifest) {
		t.Fatal("Rust producer content id was not derived from the exact Go-canonical bytes")
	}
	if result.RawArtifactManifestSHA256 != result.ProducerContentManifest.RawManifestSHA256 ||
		result.RawSourceManifestSHA256 == result.RawArtifactManifestSHA256 ||
		result.DuckDBContentSnapshotDigest == "" ||
		result.DuckDBSnapshotManifestSHA256 == "" ||
		result.SchemaDigest != domainsecurity.FixedFundsAnalyticalSchemaDigestV1() ||
		len(result.RawSourceManifest) == 0 {
		t.Fatal("Rust typed materialization bindings did not match the Go contract")
	}
}
