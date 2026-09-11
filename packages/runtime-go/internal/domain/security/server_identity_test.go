package security

import (
	"strings"
	"testing"
)

func TestVerifiedMCPServerIdentityCanonicalRoundTripAndConnectionIsolation(t *testing.T) {
	instanceA := SHA256Hex([]byte("runtime-instance-a"))
	identity, err := NewVerifiedMCPServerIdentity("funds-source", "analytix_funds/公安", "0.16.16+evidence", instanceA, 7)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseVerifiedMCPServerIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ServerID != "funds-source" || parsed.ObservedName != "analytix_funds/公安" || parsed.ObservedVersion != "0.16.16+evidence" ||
		parsed.NegotiatedProtocolVersion != "2025-11-25" || !VerifiedMCPServerIdentityCanAuthorizeFacts(parsed) ||
		parsed.ConnectionInstanceID != instanceA || parsed.ConnectionEpoch != 7 {
		t.Fatalf("verified identity round trip mismatch: %#v", parsed)
	}
	restarted, err := NewVerifiedMCPServerIdentity("funds-source", parsed.ObservedName, parsed.ObservedVersion, SHA256Hex([]byte("runtime-instance-b")), 7)
	if err != nil {
		t.Fatal(err)
	}
	if restarted == identity {
		t.Fatal("same numeric epoch from another runtime instance reused verified identity")
	}
}

func TestLegacyMCPIdentityCannotAuthorizeNewFactCall(t *testing.T) {
	instance := SHA256Hex([]byte("legacy-runtime-instance"))
	legacy := "mcpv1:ZG9jcw:ZG9jcy1zZXJ2ZXI:MS4wLjA:" + instance + ":1"
	parsed, err := ParseVerifiedMCPServerIdentity(legacy)
	if err != nil || parsed.Version != LegacyVerifiedMCPServerIdentityVersion {
		t.Fatalf("historical v1 identity could not be parsed for audit: parsed=%#v err=%v", parsed, err)
	}
	if VerifiedMCPServerIdentityCanAuthorizeFacts(parsed) {
		t.Fatal("legacy MCP identity authorized a new fact call")
	}
}

func TestVerifiedMCPServerIdentityRejectsAmbiguousAndUnsafeForms(t *testing.T) {
	instance := SHA256Hex([]byte("runtime-instance"))
	valid, err := NewVerifiedMCPServerIdentity("docs", "docs-server", "1.0.0", instance, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		" " + valid, valid + " ", strings.Replace(valid, ":1", ":01", 1), valid + ":extra",
		"mcp:docs:docs-server@1.0.0#1", "mcpv1:!!!!:ZG9jcw:MS4wLjA:" + instance + ":1",
	} {
		if _, err := ParseVerifiedMCPServerIdentity(value); err == nil {
			t.Fatalf("unsafe verified identity was accepted: %q", value)
		}
	}
	if _, err := NewVerifiedMCPServerIdentity("docs", "docs\nserver", "1.0.0", instance, 1); err == nil {
		t.Fatal("control character entered verified identity")
	}
	if _, err := NewVerifiedMCPServerIdentity("docs", "docs-server", "1.0.0", instance, maxJSONSafeUint64+1); err == nil {
		t.Fatal("JSON-unsafe connection epoch entered verified identity")
	}
}
