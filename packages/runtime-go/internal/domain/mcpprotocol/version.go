package mcpprotocol

const PreferredVersion = "2025-11-25"

// SupportedVersion returns the protocol revisions whose standard contracts
// include Streamable HTTP and structured tool output. The 2024-11-05 revision
// used a different HTTP+SSE transport and cannot authorize current fact tools.
func SupportedVersion(version string) bool {
	switch version {
	case PreferredVersion, "2025-06-18":
		return true
	default:
		return false
	}
}

func FactAuthorityCapable(version string) bool {
	return SupportedVersion(version)
}
