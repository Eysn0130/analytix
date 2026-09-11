package contracts

import (
	"crypto/sha256"
	"encoding/hex"
)

func ShortHexHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}
