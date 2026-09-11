package job

import (
	"strings"
	"testing"
)

func TestProjectPersistableUntrustedOutputV1WithholdsReasoningAndMasksPII(t *testing.T) {
	for name, input := range map[string]string{
		"unterminated reasoning": "public<think>PRIVATE_REASONING",
		"serialized reasoning":   `{"reasoning_content":"PRIVATE_REASONING"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := ProjectPersistableUntrustedOutputV1(input); got != "" {
				t.Fatalf("private reasoning output was retained: %q", got)
			}
		})
	}
	got := ProjectPersistableUntrustedOutputV1("public<think>PRIVATE_REASONING</think> 账号 6222020202020202020")
	if strings.Contains(got, "PRIVATE_REASONING") || strings.Contains(got, "6222020202020202020") ||
		!strings.Contains(got, "public") || !strings.Contains(got, "[ACCOUNT]") {
		t.Fatalf("ordinary job output projection = %q", got)
	}
}

func TestProjectPersistableUntrustedOutputV1RedactsCredentials(t *testing.T) {
	for name, input := range map[string]string{
		"bearer":  "Authorization: Bearer bearer-secret-123456",
		"api key": "X_API_KEY=api-secret-123456",
		"token":   "sk-1234567890abcdef",
		"dsn":     "postgres://analyst:password-secret-123@db.example/case",
		"query":   "https://example.test/data?access_token=query-secret-123",
	} {
		t.Run(name, func(t *testing.T) {
			projected := ProjectPersistableUntrustedOutputV1(input)
			for _, secret := range []string{"bearer-secret-123456", "api-secret-123456", "sk-1234567890abcdef", "password-secret-123", "query-secret-123"} {
				if strings.Contains(projected, secret) {
					t.Fatalf("credential %q survived ordinary job projection: %q", secret, projected)
				}
			}
		})
	}
}
