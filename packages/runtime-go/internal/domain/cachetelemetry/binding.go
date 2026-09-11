package cachetelemetry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const ProviderTelemetryKeyDerivationDomainV1 = "analytix/provider-attempt-telemetry-hmac-key/v1\x00"

// ProviderTelemetryHMACV1 is the existing length-prefixed ledger identity
// algorithm, shared by the producer and read-only restart binding observer.
// It never signs a ledger record or returns its derivation key.
func ProviderTelemetryHMACV1(key []byte, label string, parts ...[]byte) string {
	mac := hmac.New(sha256.New, key)
	write := func(value []byte) {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = mac.Write(size[:])
		_, _ = mac.Write(value)
	}
	write([]byte("analytix/provider-attempt-telemetry-hmac/v1\x00"))
	write([]byte(label))
	for _, part := range parts {
		write(part)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func ProviderTurnBindingHMACV1(key []byte, frozen domainsecurity.TurnSecurityContext) (string, error) {
	if len(key) < sha256.Size || domainsecurity.ValidateTurnSecurityContext(frozen) != nil {
		return "", errors.New("provider telemetry turn binding input is invalid")
	}
	body, err := json.Marshal(frozen)
	if err != nil {
		return "", err
	}
	return ProviderTelemetryHMACV1(key, "turn-binding", body), nil
}
