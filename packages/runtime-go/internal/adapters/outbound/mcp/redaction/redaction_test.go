package redaction

import (
	"strings"
	"testing"
)

func TestRedactedDiagnosticRemovesHeadersEnvAndURLSecrets(t *testing.T) {
	diagnostic, rawSecretPresent := RedactedDiagnostic(
		map[string]string{"Authorization": "Bearer header-secret"},
		map[string]string{"MCP_API_KEY": "env-secret", "DEBUG": "true"},
		"https://user:pass@example.invalid/mcp?key=query-secret&debug=1",
	)
	if rawSecretPresent {
		t.Fatalf("redacted diagnostic still contains a raw secret: %s", diagnostic)
	}
	for _, forbidden := range []string{"header-secret", "env-secret", "query-secret", "user:pass"} {
		if strings.Contains(diagnostic, forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, diagnostic)
		}
	}
	for _, expected := range []string{"Authorization=<redacted>", "MCP_API_KEY=<redacted>", "key=%3Credacted%3E"} {
		if !strings.Contains(diagnostic, expected) {
			t.Fatalf("diagnostic missing %q: %s", expected, diagnostic)
		}
	}
}

func TestDiagnosticAndCatalogTextRedactAuthMaterial(t *testing.T) {
	diagnostic := DiagnosticText(
		`request failed: Authorization: Bearer header-secret; token=env-secret`,
		map[string]string{"Authorization": "Bearer header-secret"},
		map[string]string{"TOKEN": "env-secret"},
		"https://user:pass@example.invalid/mcp?key=query-secret",
	)
	for _, forbidden := range []string{"header-secret", "env-secret", "query-secret", "user:pass"} {
		if strings.Contains(diagnostic, forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, diagnostic)
		}
	}
	catalog := CatalogText("https://user:pass@example.invalid/mcp?access_token=secret")
	if strings.Contains(catalog, "user:pass") || strings.Contains(catalog, "secret") {
		t.Fatalf("catalog leaked auth material: %s", catalog)
	}
}

func TestDiagnosticTextMasksRestrictedPIIOutsideControlledEvidence(t *testing.T) {
	diagnostic := DiagnosticText(
		"account 6217000012345678901 identity 320000199001011234 phone 13800138000 device 00:11:22:33:44:55",
		nil, nil, "",
	)
	for _, forbidden := range []string{"6217000012345678901", "320000199001011234", "13800138000", "00:11:22:33:44:55"} {
		if strings.Contains(diagnostic, forbidden) {
			t.Fatalf("ordinary MCP diagnostic leaked restricted PII %q: %s", forbidden, diagnostic)
		}
	}
	if !strings.Contains(diagnostic, "[ACCOUNT]") {
		t.Fatalf("account projection did not use the shared ordinary projection: %s", diagnostic)
	}
}

func TestCatalogTextMasksRestrictedPIIEmbeddedInToolIdentity(t *testing.T) {
	const account = "6217000012345678901"
	redacted := CatalogText("bad_" + account + "_lookup")
	if strings.Contains(redacted, account) || !strings.Contains(redacted, "[ACCOUNT]") {
		t.Fatalf("untrusted MCP tool identity leaked an embedded account: %s", redacted)
	}
}

func TestMCPDiagnosticsUseSharedProjectionForShortAndObfuscatedAccounts(t *testing.T) {
	for _, input := range []string{
		"账\u200b号：００１２３４５６７",
		"account number: 00123456789",
		"账号：0012/3456/7890",
	} {
		redacted := DiagnosticText(input, nil, nil, "")
		if !strings.Contains(redacted, "[ACCOUNT]") {
			t.Fatalf("MCP diagnostic escaped shared ordinary projection: input=%q output=%q", input, redacted)
		}
	}
}
