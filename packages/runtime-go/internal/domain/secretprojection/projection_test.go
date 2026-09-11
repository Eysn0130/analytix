package secretprojection

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

func TestProjectTextV1ClosesCommonCredentialForms(t *testing.T) {
	secrets := []string{
		"sk-1234567890abcdef", "bearer-value-123", "basic-value-123", "api-value-123",
		"password-value-123", "query-value-123", "AKIA1234567890ABCDEF",
	}
	input := strings.Join([]string{
		"Authorization: Bearer " + secrets[1],
		"Proxy-Authorization: Basic " + secrets[2],
		"X_API_KEY=" + secrets[3],
		"postgres://analyst:" + secrets[4] + "@db.example/case",
		"https://example.test/data?access_token=" + secrets[5],
		`{"api_key":"` + secrets[3] + `"}`,
		"--api-key " + secrets[3],
		secrets[0], secrets[6],
	}, " ")
	projected := ProjectTextV1(input)
	for _, secret := range secrets {
		if strings.Contains(projected, secret) {
			t.Fatalf("credential %q survived projection: %s", secret, projected)
		}
	}
	if ContainsCredentialV1(projected) {
		t.Fatalf("projected text still contains recognizable credentials: %s", projected)
	}
}

func TestProjectValueV1ClosesNestedCredentialText(t *testing.T) {
	const secret = "sk-nested-secret-123456"
	projected := ProjectValueV1(map[string]any{
		"message": "Bearer " + secret,
		"nested": []any{
			"postgres://analyst:" + secret + "@db.example/case",
			map[string]string{"detail": `{"api_key":"` + secret + `"}`},
		},
		"credential-" + secret: "key",
	})
	encoded := projected.(map[string]any)
	if strings.Contains(encoded["message"].(string), secret) ||
		strings.Contains(encoded["nested"].([]any)[0].(string), secret) ||
		strings.Contains(encoded["nested"].([]any)[0].(string), "analyst") ||
		strings.Contains(encoded["nested"].([]any)[1].(map[string]string)["detail"], secret) {
		t.Fatalf("nested credential survived projection: %#v", projected)
	}
	for key := range encoded {
		if strings.Contains(key, secret) {
			t.Fatalf("credential survived as a map key: %#v", projected)
		}
	}
}

func TestProjectValueV1FailsClosedOnProjectedKeyCollision(t *testing.T) {
	projected := ProjectValueV1(map[string]any{
		"Authorization: Bearer first-secret":  "left",
		"Authorization: Bearer second-secret": "right",
	})
	if projected != RedactedV1 {
		t.Fatalf("projected key collision = %#v, want fail-closed sentinel", projected)
	}

	projectedStrings := ProjectValueV1(map[string]string{
		"api_key=first-secret":  "left",
		"api_key=second-secret": "right",
	})
	if projectedStrings != RedactedV1 {
		t.Fatalf("string-map projected key collision = %#v, want fail-closed sentinel", projectedStrings)
	}
}

func TestProjectValueV1RedactsOpaqueValuesUnderCredentialKeys(t *testing.T) {
	const secret = "opaque-value-123"
	projected := ProjectValueV1(map[string]any{
		"apiKey":       secret,
		"password":     secret,
		"github_token": secret,
		"key":          "run:run-1",
		"hasApiKey":    true,
		"token_budget": 64000,
	}).(map[string]any)
	for _, key := range []string{"apiKey", "password", "github_token"} {
		if projected[key] != RedactedV1 {
			t.Fatalf("credential key %q was not redacted: %#v", key, projected)
		}
	}
	if projected["token_budget"] != 64000 {
		t.Fatalf("non-credential token budget changed: %#v", projected)
	}
	if projected["key"] != "run:run-1" {
		t.Fatalf("generic identity key changed: %#v", projected)
	}
	if projected["hasApiKey"] != true {
		t.Fatalf("credential-presence metadata changed: %#v", projected)
	}
}

func TestValidateValueV1RejectsStructuredAndTextCredentials(t *testing.T) {
	for _, value := range []any{
		map[string]any{"password": "opaque-value-123"},
		map[string]string{"api_key": "opaque-value-123"},
		map[string]any{"message": "Authorization: Bearer opaque-value-123"},
		json.RawMessage(`{"client_secret":"opaque-value-123"}`),
	} {
		if err := ValidateValueV1(value); !errors.Is(err, ErrCredentialMaterialV1) {
			t.Fatalf("ValidateValueV1(%#v) error = %v, want credential rejection", value, err)
		}
	}
	if err := ValidateValueV1(map[string]any{"token_budget": 64000, "message": "credential withheld"}); err != nil {
		t.Fatalf("safe public value rejected: %v", err)
	}
}

func TestCommandSecretsProjectEchoedOutput(t *testing.T) {
	const secret = "literal-command-secret-123"
	command := "API_TOKEN=" + secret + " sh -c 'echo $API_TOKEN'"
	projected := ProjectTextV1("stdout="+secret, ExtractExplicitSecretsV1(command)...)
	if strings.Contains(projected, secret) || !strings.Contains(projected, RedactedV1) {
		t.Fatalf("literal command secret escaped echoed-output projection: %s", projected)
	}
}

func TestProjectTextV1RedactsUnterminatedPrivateKeyToEOF(t *testing.T) {
	for _, kind := range []string{"RSA PRIVATE KEY", "OPENSSH PRIVATE KEY", "EC PRIVATE KEY"} {
		input := "diagnostic\n-----BEGIN " + kind + "-----\nQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo="
		projected := ProjectTextV1(input)
		if strings.Contains(projected, "QUJDREVGR0hJ") || strings.Contains(projected, "BEGIN "+kind) || !strings.Contains(projected, RedactedV1) {
			t.Fatalf("unterminated %s survived projection: %s", kind, projected)
		}
	}
}

func TestProjectTextV1RedactsDriverQualifiedDSNs(t *testing.T) {
	const secret = "driver-password-123456"
	for _, dsn := range []string{
		"postgresql+psycopg://analyst:" + secret + "@db.example/case",
		"postgresql+asyncpg://analyst:" + secret + "@db.example/case",
		"mysql+pymysql://analyst:" + secret + "@db.example/case",
	} {
		projected := ProjectTextV1("dsn=" + dsn)
		if strings.Contains(projected, secret) || strings.Contains(projected, "analyst") || ContainsCredentialV1(projected) {
			t.Fatalf("driver-qualified DSN survived projection: %s", projected)
		}
	}
}

func TestProjectTextV1RedactsQuotedCredentialValuesContainingSpaces(t *testing.T) {
	secrets := []string{
		"assignment secret with spaces",
		"flag secret with spaces",
		"json secret with spaces",
	}
	input := strings.Join([]string{
		`API_KEY="` + secrets[0] + `"`,
		`--password '` + secrets[1] + `'`,
		`{"client_secret": "` + secrets[2] + `"}`,
	}, " ")
	projected := ProjectTextV1(input)
	for _, secret := range secrets {
		if strings.Contains(projected, secret) {
			t.Fatalf("quoted credential %q survived projection: %s", secret, projected)
		}
	}
	if ContainsCredentialV1(projected) {
		t.Fatalf("projected quoted credentials remain recognizable: %s", projected)
	}

	extracted := ExtractExplicitSecretsV1(input)
	for _, secret := range secrets {
		if !containsString(extracted, secret) {
			t.Fatalf("quoted credential %q was not extracted: %#v", secret, extracted)
		}
	}
}

func TestAuthishKeyV1UsesExactSafeMetadataExceptions(t *testing.T) {
	for _, key := range []string{"hasApiKey", "has_access_token", "hasClientSecret"} {
		if AuthishKeyV1(key) {
			t.Fatalf("safe presence metadata %q classified as credential material", key)
		}
	}
	for _, key := range []string{"hashicorpToken", "issuerSecret", "aws_secret_access_key"} {
		if !AuthishKeyV1(key) {
			t.Fatalf("credential-bearing key %q escaped classification", key)
		}
	}
	for _, key := range []string{"Hashicorp", "Issuer", "token_budget", "credentialProfileDigest", "authorizationAuditDigest"} {
		if AuthishKeyV1(key) {
			t.Fatalf("ordinary metadata key %q was falsely classified", key)
		}
	}

	projected := ProjectValueV1(map[string]any{
		"hasApiKey":      true,
		"hashicorpToken": "vault-token-value",
		"issuerSecret":   "issuer-secret-value",
	}).(map[string]any)
	if projected["hasApiKey"] != true || projected["hashicorpToken"] != RedactedV1 || projected["issuerSecret"] != RedactedV1 {
		t.Fatalf("exact metadata projection mismatch: %#v", projected)
	}
	if projected := ProjectValueV1(map[string]any{"hasApiKey": "opaque-key-value"}).(map[string]any); projected["hasApiKey"] != RedactedV1 {
		t.Fatalf("non-boolean presence metadata carried credential material: %#v", projected)
	}
	for _, text := range []string{"hasApiKey=true", "has_secret=false", "token_budget=64000", "passwordHash=sha256:abcdef123456"} {
		if projected := ProjectTextV1(text); projected != text || ContainsCredentialV1(projected) {
			t.Fatalf("safe metadata %q changed to %q", text, projected)
		}
	}
	for _, text := range []string{"HashicorpToken=opaque-vault-value", "IssuerSecret='opaque issuer value'"} {
		if projected := ProjectTextV1(text); projected == text || ContainsCredentialV1(projected) {
			t.Fatalf("credential-bearing metadata was not safely projected: raw=%q projected=%q", text, projected)
		}
	}
}

func TestProjectTextV1RedactsProviderTokenVariants(t *testing.T) {
	secrets := []string{
		"ghr_1234567890abcdef",
		"github_pat_1234567890abcdef",
		"xapp-1-1234567890-abc.def",
		"xoxe.xoxp-1234567890-abc.def",
		"aws-secret-access-value-1234567890",
	}
	input := strings.Join([]string{
		secrets[0],
		secrets[1],
		secrets[2],
		secrets[3],
		`AWS_SECRET_ACCESS_KEY="` + secrets[4] + `"`,
	}, " ")
	projected := ProjectTextV1(input)
	for _, secret := range secrets {
		if strings.Contains(projected, secret) {
			t.Fatalf("provider credential %q survived projection: %s", secret, projected)
		}
	}
	if err := ValidateValueV1(map[string]any{"aws_secret_access_key": secrets[4]}); !errors.Is(err, ErrCredentialMaterialV1) {
		t.Fatalf("structured AWS secret-access-key error = %v, want credential rejection", err)
	}
}

func TestProjectionLimitsFailClosed(t *testing.T) {
	oversized := strings.Repeat("x", maxTextBytesV1+1)
	if projected := ProjectTextV1(oversized); projected != RedactedV1 {
		t.Fatalf("oversized text projection = %q, want redaction", projected)
	}
	if !ContainsCredentialV1(oversized) {
		t.Fatal("oversized text did not fail closed in credential detection")
	}
	if err := ValidateValueV1(oversized); !errors.Is(err, ErrProjectionLimitV1) {
		t.Fatalf("oversized value error = %v, want projection limit", err)
	}
	if projected := ProjectValueV1(oversized); projected != RedactedV1 {
		t.Fatalf("oversized value projection = %#v, want redaction", projected)
	}

	tooMany := make([]any, maxValueNodesV1+1)
	if err := ValidateValueV1(tooMany); !errors.Is(err, ErrProjectionLimitV1) {
		t.Fatalf("over-node value error = %v, want projection limit", err)
	}
	if projected := ProjectValueV1(tooMany); projected != RedactedV1 {
		t.Fatalf("over-node value projection = %#v, want redaction", projected)
	}

	aggregate := make([]string, maxAggregateBytesV1/(64<<10)+1)
	for index := range aggregate {
		aggregate[index] = strings.Repeat("a", 64<<10)
	}
	if err := ValidateValueV1(aggregate); !errors.Is(err, ErrProjectionLimitV1) {
		t.Fatalf("over-aggregate value error = %v, want projection limit", err)
	}
	if projected := ProjectValueV1(aggregate); projected != RedactedV1 {
		t.Fatalf("over-aggregate value projection = %#v, want redaction", projected)
	}

	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if err := ValidateValueV1(cyclic); !errors.Is(err, ErrProjectionLimitV1) {
		t.Fatalf("cyclic value error = %v, want projection limit", err)
	}
	if projected := ProjectValueV1(cyclic); projected != RedactedV1 {
		t.Fatalf("cyclic value projection = %#v, want redaction", projected)
	}
}

func TestProjectTextV1HintsMatchUnconditionalProjection(t *testing.T) {
	tests := []string{
		"plain diagnostic without credential material",
		"Authorization: Bearer bearer-value-123",
		"Baſic basic-value-123",
		"apiKey=unicode-fold-value-123",
		"--client-secret 'flag secret with spaces'",
		"sk-1234567890abcdef ghr_1234567890abcdef github_pat_1234567890abcdef",
		"AKIA1234567890ABCDEF ASIA1234567890ABCDEF",
		"postgres://analyst:driver-password-123456@db.example/case?access_token=query-value-123",
		"https://example.test/?q=%73k-12345678",
		"https://example.test/?q=%41KIA1234567890ABCDEF",
		"http://json-schema.org/draft-07/schema#",
		"diagnostic\n-----BEGIN PRIVATE KEY-----\nQUJDREVGRw==\n-----END PRIVATE KEY-----",
		"hasApiKey=true token_budget=64000 passwordHash=sha256:abcdef123456",
	}
	for _, input := range tests {
		if got, want := ProjectTextV1(input), projectTextV1UnconditionalForTest(input); got != want {
			t.Fatalf("hinted projection differs from unconditional projection\ninput: %q\n got: %q\nwant: %q", input, got, want)
		}
	}
}

func TestProjectTextV1PreservesSafeTrailingEmptyURLFragment(t *testing.T) {
	const schemaURL = "http://json-schema.org/draft-07/schema#"
	if projected := ProjectTextV1(schemaURL); projected != schemaURL {
		t.Fatalf("safe trailing empty fragment changed: got %q, want %q", projected, schemaURL)
	}

	const secret = "sk-trailing-fragment-123456"
	projected := ProjectTextV1("https://analyst:" + secret + "@example.test/schema?access_token=" + secret + "#")
	if strings.Contains(projected, secret) || ContainsCredentialV1(projected) {
		t.Fatalf("credential survived trailing empty fragment projection: %q", projected)
	}
}

func TestProjectTextV1PreservesCredentialFreeURLInsideMarkdownCode(t *testing.T) {
	input := "Preserve the `http://guides.github.com/overviews/forking/` target."
	if projected := ProjectTextV1(input); projected != input {
		t.Fatalf("credential-free Markdown URL changed: got %q, want %q", projected, input)
	}
	if err := ValidateValueV1(input); err != nil {
		t.Fatalf("credential-free Markdown URL was rejected: %v", err)
	}

	const secret = "markdown-url-secret-123456"
	credential := "Inspect `https://example.test/?access_token=" + secret + "` only."
	projected := ProjectTextV1(credential)
	if strings.Contains(projected, secret) || ContainsCredentialV1(projected) {
		t.Fatalf("credential inside Markdown URL survived projection: %q", projected)
	}
}

func TestProjectTextV1RedactsCredentialsIntroducedByURLNormalization(t *testing.T) {
	for _, secret := range []string{"sk-12345678", "AKIA1234567890ABCDEF"} {
		encoded := strings.NewReplacer("s", "%73", "A", "%41").Replace(secret)
		projected := ProjectTextV1("https://example.test/?q=" + encoded)
		if strings.Contains(projected, secret) || ContainsCredentialV1(projected) {
			t.Fatalf("URL normalization exposed credential %q: %s", secret, projected)
		}
	}
}

func TestProjectTextV1PreservesLongUnicodeTextWithoutCredentialCandidates(t *testing.T) {
	input := strings.Repeat("案件事实尚未验证；仅可说明能力边界。", 2_000)
	if got := ProjectTextV1(input); got != input {
		t.Fatalf("safe long Unicode text changed: got %d bytes, want %d", len(got), len(input))
	}
}

// projectTextV1UnconditionalForTest preserves the pre-optimization execution
// model so the lexical hints are checked against the complete regex pipeline.
func projectTextV1UnconditionalForTest(text string, explicitSecrets ...string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if len(text) > maxTextBytesV1 || len(explicitSecrets) > maxExplicitSecretsV1 {
		return RedactedV1
	}
	for _, secret := range explicitSecrets {
		if len(secret) > maxTextBytesV1 {
			return RedactedV1
		}
	}
	out := urlCandidateV1.ReplaceAllStringFunc(text, redactURLV1)
	allHints := credentialScanHintsV1{
		url: true, assignment: true, flag: true, authScheme: true,
		knownToken: true, awsAccessKey: true, privateKey: true,
	}
	secrets := append([]string(nil), explicitSecrets...)
	secrets = append(secrets, extractExplicitSecretsWithHintsV1(text, allHints)...)
	sort.SliceStable(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if len(secret) >= 4 && secret != RedactedV1 {
			out = strings.ReplaceAll(out, secret, RedactedV1)
		}
	}
	out = privateKeyBlockV1.ReplaceAllString(out, RedactedV1)
	out = privateKeyRemainderV1.ReplaceAllString(out, RedactedV1)
	out = knownTokenV1.ReplaceAllString(out, RedactedV1)
	out = awsAccessKeyV1.ReplaceAllString(out, RedactedV1)
	out = authSchemeV1.ReplaceAllStringFunc(out, func(match string) string {
		parts := authSchemeV1.FindStringSubmatch(match)
		if len(parts) < 3 || normalizedCredentialLiteralV1(firstNonEmptyV1(parts[2:]...)) == RedactedV1 {
			return match
		}
		return parts[1] + " " + RedactedV1
	})
	out = credentialAssignmentV1.ReplaceAllStringFunc(out, func(match string) string {
		parts := credentialAssignmentV1.FindStringSubmatch(match)
		if len(parts) < 4 {
			return RedactedV1
		}
		rawValue := firstNonEmptyV1(parts[3:]...)
		if !credentialTextKeyHasMaterialV1(parts[2], rawValue) {
			return match
		}
		return parts[1] + parts[2] + "=" + RedactedV1
	})
	out = credentialFlagV1.ReplaceAllStringFunc(out, func(match string) string {
		parts := credentialFlagV1.FindStringSubmatch(match)
		if len(parts) < 4 {
			return "--credential=" + RedactedV1
		}
		rawValue := firstNonEmptyV1(parts[3:]...)
		if !credentialTextKeyHasMaterialV1(parts[2], rawValue) {
			return match
		}
		return parts[1] + "--" + parts[2] + "=" + RedactedV1
	})
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
