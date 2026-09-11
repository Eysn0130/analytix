package contracts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// IsCanonicalOpaqueUUIDV4 accepts only lowercase RFC 4122 UUIDv4 values.
// Public correlation identifiers use this closed grammar so caller-controlled
// account numbers or other private text cannot become durable metadata.
func IsCanonicalOpaqueUUIDV4(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '4' {
		return false
	}
	if value[19] != '8' && value[19] != '9' && value[19] != 'a' && value[19] != 'b' {
		return false
	}
	for index := 0; index < len(value); index++ {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", rune(value[index])) {
			return false
		}
	}
	return true
}

// NewOpaqueUUIDV4 creates a host-owned UUIDv4. The digest fallback contains
// no caller text in the returned identifier and is used only if the platform
// secure random source is unavailable.
func NewOpaqueUUIDV4(namespace, seed string, now time.Time) string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		fallback := sha256.Sum256([]byte(
			strings.TrimSpace(namespace) + "\x00" + strings.TrimSpace(seed) + "\x00" + strconv.FormatInt(now.UnixNano(), 10),
		))
		copy(bytes, fallback[:16])
	}
	bytes[6] = bytes[6]&0x0f | 0x40
	bytes[8] = bytes[8]&0x3f | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
