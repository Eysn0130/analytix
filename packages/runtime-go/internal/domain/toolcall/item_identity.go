package toolcall

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

const ToolCallItemIDPrefixV1 = "item_tool_host_v1_"

var toolCallItemIDDomainV1 = []byte("analytix.tool-call-item/id/v1\x00")

// ToolCallItemIDV1 is the public record identity for a current host-issued
// tool call. It never copies a turn ID or provider-originated identifier.
func ToolCallItemIDV1(turnID, hostToolCallID string) string {
	if strings.TrimSpace(turnID) == "" || !domainmodel.IsHostToolCallIDV1(hostToolCallID) {
		return ""
	}
	hash := sha256.New()
	hash.Write(toolCallItemIDDomainV1)
	writeToolCallItemIDPartV1(hash, turnID)
	writeToolCallItemIDPartV1(hash, hostToolCallID)
	return ToolCallItemIDPrefixV1 + hex.EncodeToString(hash.Sum(nil))
}

func IsToolCallItemIDV1(value string) bool {
	if !strings.HasPrefix(value, ToolCallItemIDPrefixV1) {
		return false
	}
	digest := strings.TrimPrefix(value, ToolCallItemIDPrefixV1)
	if len(digest) != sha256.Size*2 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == sha256.Size
}

func writeToolCallItemIDPartV1(hash interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:])
	_, _ = hash.Write([]byte(value))
}
