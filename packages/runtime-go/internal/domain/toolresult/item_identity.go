package toolresult

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const ToolResultItemIDPrefixV1 = "item_result_"

var toolResultItemIDDomainV1 = []byte("analytix.tool-result-item/id/v1\x00")

// ToolResultItemIDV1 is an opaque stable reference. Neither a provider call
// identifier nor a turn identifier is copied into persisted record IDs.
func ToolResultItemIDV1(turnID, hostToolCallID string) string {
	if strings.TrimSpace(turnID) == "" || !domainmodel.IsHostToolCallIDV1(hostToolCallID) {
		return ""
	}
	hash := sha256.New()
	hash.Write(toolResultItemIDDomainV1)
	writeToolResultItemIDPartV1(hash, turnID)
	writeToolResultItemIDPartV1(hash, hostToolCallID)
	return ToolResultItemIDPrefixV1 + hex.EncodeToString(hash.Sum(nil))
}

func IsToolResultItemIDV1(value string) bool {
	if !strings.HasPrefix(value, ToolResultItemIDPrefixV1) {
		return false
	}
	digest := strings.TrimPrefix(value, ToolResultItemIDPrefixV1)
	if len(digest) != sha256.Size*2 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == sha256.Size
}

func writeToolResultItemIDPartV1(hash interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write([]byte(value))
}
