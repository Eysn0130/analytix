package packagedbuildauthorityfs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

func TestCompiledCoreQualificationCannotBeRelabeled(t *testing.T) {
	disposition := &domainauthority.CoreDispositionV2{Kind: domainauthority.CoreControlledDispositionKindV2, TargetKey: "darwin-arm64", SigningPolicySHA256: strings.Repeat("a", 64), SigningMode: "developer-id", AppleTeamIdentifier: "TESTTEAM01"}
	body, _ := json.Marshal(disposition)
	sum := sha256.Sum256(body)
	compiled := hex.EncodeToString(sum[:])
	formal := domainauthority.ParsedAuthorityV2{Core: disposition}
	formal.Authority.NativeDisposition = body
	if validateCompiledCoreQualificationV2(formal, compiled) != nil {
		t.Fatal("exact compiled Core qualifier rejected")
	}
	for _, invalid := range []string{"", strings.Repeat("b", 64)} {
		if validateCompiledCoreQualificationV2(formal, invalid) == nil {
			t.Fatal("unbound Core qualifier accepted")
		}
	}
	private := domainauthority.ParsedAuthorityV2{Core: &domainauthority.CoreDispositionV2{Kind: domainauthority.CoreDispositionKindV2, TargetKey: "darwin-arm64"}}
	if validateCompiledCoreQualificationV2(private, compiled) == nil || validateCompiledCoreQualificationV2(domainauthority.ParsedAuthorityV2{}, compiled) == nil {
		t.Fatal("compiled qualification crossed profile or assurance boundary")
	}
	if validateCompiledCoreQualificationV2(private, "") != nil {
		t.Fatal("existing private Core rejected")
	}
	disposition.AppleTeamIdentifier = "OTHERTEAM1"
	formal.Authority.NativeDisposition, _ = json.Marshal(disposition)
	if validateCompiledCoreQualificationV2(formal, compiled) == nil {
		t.Fatal("signing identity relabel accepted")
	}
}
