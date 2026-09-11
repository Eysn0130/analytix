package security

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	domainmcpprotocol "analytix.local/runtime-go/internal/domain/mcpprotocol"
)

const (
	VerifiedMCPServerIdentityVersion       = 2
	LegacyVerifiedMCPServerIdentityVersion = 1
	maxMCPIdentityComponentBytes           = 512
	maxJSONSafeUint64                      = uint64(1<<53 - 1)
)

// VerifiedMCPServerIdentity is a host-issued, connection-instance-bound MCP
// identity. The opaque string representation is the only representation that
// crosses persistence and runtime boundaries; parsing requires a canonical
// round trip so separator ambiguity and restart epoch reuse fail closed.
type VerifiedMCPServerIdentity struct {
	Version                   int
	ServerID                  string
	ObservedName              string
	ObservedVersion           string
	NegotiatedProtocolVersion string
	ConnectionInstanceID      string
	ConnectionEpoch           uint64
}

func NewVerifiedMCPServerIdentity(serverID, observedName, observedVersion, connectionInstanceID string, connectionEpoch uint64) (string, error) {
	return NewVerifiedMCPServerIdentityForProtocol(serverID, observedName, observedVersion, domainmcpprotocol.PreferredVersion, connectionInstanceID, connectionEpoch)
}

func NewVerifiedMCPServerIdentityForProtocol(serverID, observedName, observedVersion, negotiatedProtocolVersion, connectionInstanceID string, connectionEpoch uint64) (string, error) {
	identity := VerifiedMCPServerIdentity{
		Version:                   VerifiedMCPServerIdentityVersion,
		ServerID:                  serverID,
		ObservedName:              observedName,
		ObservedVersion:           observedVersion,
		NegotiatedProtocolVersion: negotiatedProtocolVersion,
		ConnectionInstanceID:      connectionInstanceID,
		ConnectionEpoch:           connectionEpoch,
	}
	if err := validateVerifiedMCPServerIdentity(identity); err != nil {
		return "", err
	}
	return formatVerifiedMCPServerIdentity(identity), nil
}

func ParseVerifiedMCPServerIdentity(value string) (VerifiedMCPServerIdentity, error) {
	parts := strings.Split(value, ":")
	if strings.TrimSpace(value) != value || len(parts) < 6 {
		return VerifiedMCPServerIdentity{}, errors.New("verified MCP server identity format is invalid")
	}
	version := 0
	switch {
	case parts[0] == "mcpv1" && len(parts) == 6:
		version = LegacyVerifiedMCPServerIdentityVersion
	case parts[0] == "mcpv2" && len(parts) == 7:
		version = VerifiedMCPServerIdentityVersion
	default:
		return VerifiedMCPServerIdentity{}, errors.New("verified MCP server identity format is invalid")
	}
	serverID, err := decodeMCPIdentityComponent(parts[1])
	if err != nil {
		return VerifiedMCPServerIdentity{}, err
	}
	observedName, err := decodeMCPIdentityComponent(parts[2])
	if err != nil {
		return VerifiedMCPServerIdentity{}, err
	}
	observedVersion, err := decodeMCPIdentityComponent(parts[3])
	if err != nil {
		return VerifiedMCPServerIdentity{}, err
	}
	protocolVersion := ""
	instanceIndex := 4
	epochIndex := 5
	if version == VerifiedMCPServerIdentityVersion {
		protocolVersion, err = decodeMCPIdentityComponent(parts[4])
		if err != nil {
			return VerifiedMCPServerIdentity{}, err
		}
		instanceIndex = 5
		epochIndex = 6
	}
	epoch, err := strconv.ParseUint(parts[epochIndex], 10, 64)
	if err != nil || strconv.FormatUint(epoch, 10) != parts[epochIndex] {
		return VerifiedMCPServerIdentity{}, errors.New("verified MCP connection epoch is invalid")
	}
	identity := VerifiedMCPServerIdentity{
		Version:                   version,
		ServerID:                  serverID,
		ObservedName:              observedName,
		ObservedVersion:           observedVersion,
		NegotiatedProtocolVersion: protocolVersion,
		ConnectionInstanceID:      parts[instanceIndex],
		ConnectionEpoch:           epoch,
	}
	if err := validateVerifiedMCPServerIdentity(identity); err != nil || formatVerifiedMCPServerIdentity(identity) != value {
		return VerifiedMCPServerIdentity{}, errors.New("verified MCP server identity is not canonical")
	}
	return identity, nil
}

func validateVerifiedMCPServerIdentity(identity VerifiedMCPServerIdentity) error {
	if identity.Version != VerifiedMCPServerIdentityVersion && identity.Version != LegacyVerifiedMCPServerIdentityVersion || !validMCPIdentityComponent(identity.ServerID) ||
		!validMCPIdentityComponent(identity.ObservedName) || !validMCPIdentityComponent(identity.ObservedVersion) ||
		!isSHA256Hex(identity.ConnectionInstanceID) || identity.ConnectionEpoch == 0 || identity.ConnectionEpoch > maxJSONSafeUint64 {
		return errors.New("verified MCP server identity is invalid")
	}
	if identity.Version == VerifiedMCPServerIdentityVersion {
		if !validMCPIdentityComponent(identity.NegotiatedProtocolVersion) || !domainmcpprotocol.SupportedVersion(identity.NegotiatedProtocolVersion) {
			return errors.New("verified MCP negotiated protocol version is invalid")
		}
	} else if identity.NegotiatedProtocolVersion != "" {
		return errors.New("legacy verified MCP identity cannot bind a protocol version")
	}
	return nil
}

func VerifiedMCPServerIdentityCanAuthorizeFacts(identity VerifiedMCPServerIdentity) bool {
	return identity.Version == VerifiedMCPServerIdentityVersion && domainmcpprotocol.FactAuthorityCapable(identity.NegotiatedProtocolVersion)
}

func validMCPIdentityComponent(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > maxMCPIdentityComponentBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func encodeMCPIdentityComponent(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeMCPIdentityComponent(value string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return "", errors.New("verified MCP server identity component is invalid")
	}
	return string(decoded), nil
}

func formatVerifiedMCPServerIdentity(identity VerifiedMCPServerIdentity) string {
	parts := []string{
		"mcpv2",
		encodeMCPIdentityComponent(identity.ServerID),
		encodeMCPIdentityComponent(identity.ObservedName),
		encodeMCPIdentityComponent(identity.ObservedVersion),
	}
	if identity.Version == LegacyVerifiedMCPServerIdentityVersion {
		parts[0] = "mcpv1"
	} else {
		parts = append(parts, encodeMCPIdentityComponent(identity.NegotiatedProtocolVersion))
	}
	parts = append(parts, identity.ConnectionInstanceID, strconv.FormatUint(identity.ConnectionEpoch, 10))
	return strings.Join(parts, ":")
}
