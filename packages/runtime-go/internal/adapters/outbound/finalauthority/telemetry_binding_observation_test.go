//go:build darwin || linux || windows

package finalauthority

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestExistingTelemetryBindingObservationMatchesOriginalProducerWithoutSigningCapability(t *testing.T) {
	for _, mode := range []string{"local", "enrolled"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			fixture := newExistingFileAuthorityAnchorFixture(t)
			local, err := OpenExistingFileVerificationV1(fixture.rootAuthority)
			if err != nil {
				t.Fatal(err)
			}
			observe := local.ObserveProviderTurnBindingHMACV1
			if mode == "enrolled" {
				anchored, err := fixture.anchor.Open(fixture.path)
				if err != nil {
					t.Fatal(err)
				}
				observe = anchored.ObserveProviderTurnBindingHMACV1
			}
			frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID: "thread-binding", TurnID: "turn-binding", WorkspaceRealPath: "/synthetic/telemetry",
				CaseID: "case-binding", CaseBindingHash: domainsecurity.SHA256Hex([]byte("synthetic-case-binding")),
				ContextEpoch: 1, IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			// Independent reference freezes the pre-extraction producer's exact
			// KDF purpose, HMAC framing, label and complete context encoding.
			key, err := fixture.authority.Sign(ctx, []byte("analytix/provider-attempt-telemetry-hmac-key/v1\x00"))
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(frozen)
			if err != nil {
				t.Fatal(err)
			}
			mac := hmac.New(sha256.New, key)
			for _, part := range [][]byte{[]byte("analytix/provider-attempt-telemetry-hmac/v1\x00"), []byte("turn-binding"), body} {
				var length [8]byte
				binary.BigEndian.PutUint64(length[:], uint64(len(part)))
				_, _ = mac.Write(length[:])
				_, _ = mac.Write(part)
			}
			expected := hex.EncodeToString(mac.Sum(nil))
			for index := range key {
				key[index] = 0
			}
			before, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				observed, err := observe(ctx, frozen)
				if err != nil || observed != expected {
					t.Fatalf("read-only binding differs from original ledger producer: %v", err)
				}
			}
			if signature, err := local.Sign(ctx, []byte("analytix/provider-attempt-telemetry-hmac-key/v1\x00")); err == nil || len(signature) != 0 {
				t.Fatal("read-only observer exposed generic signing or KDF material")
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if binding, err := observe(cancelled, frozen); err == nil || binding != "" {
				t.Fatal("cancelled observation returned a binding")
			}
			invalid := frozen
			invalid.ContextDigest = domainsecurity.SHA256Hex([]byte("changed-context"))
			if binding, err := observe(ctx, invalid); err == nil || binding != "" {
				t.Fatal("unbound context returned an original telemetry binding")
			}
			after, err := os.ReadFile(fixture.path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("telemetry observation rewrote the key")
			}
			if err := os.Rename(fixture.path, filepath.Join(t.TempDir(), "preserved-original-key.json")); err != nil {
				t.Fatal(err)
			}
			if binding, err := observe(ctx, frozen); err == nil || binding != "" {
				t.Fatal("missing key retained read-only derivation authority")
			}
			if _, err := os.Stat(fixture.path); !os.IsNotExist(err) {
				t.Fatal("read-only observer recreated a missing key")
			}
			if err := os.WriteFile(fixture.path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			if binding, err := observe(ctx, frozen); err == nil || binding != "" {
				t.Fatal("same key bytes at a replaced identity retained authority")
			}
		})
	}
}
