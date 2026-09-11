package mcpname

import (
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
)

const prefix = "mcp__"

var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
var authoritySegment = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)
var remoteToolName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Canonical returns the provider-visible v1 MCP tool identity. The v1
// separator is reserved in the server identity. The remote operation is the
// exact case-sensitive SEP-986 name and may contain repeated underscores;
// Parse splits only the first server/operation delimiter, so the tuple remains
// injective without lossy normalization.
func Canonical(serverID, rawTool string) (string, error) {
	server, err := serverComponent(serverID)
	if err != nil {
		return "", err
	}
	tool, err := toolComponent(rawTool)
	if err != nil {
		return "", err
	}
	return prefix + server + "__" + tool, nil
}

func Namespace(serverID string) (string, error) {
	server, err := serverComponent(serverID)
	if err != nil {
		return "", err
	}
	return prefix + server + "__", nil
}

func ValidateToolName(value string) error {
	_, err := toolComponent(value)
	return err
}

// Parse accepts mcp__<server>__<operation> and splits at the first delimiter
// after the canonical lowercase server identity. It never normalizes or
// changes the exact remote operation.
func Parse(value string) (serverNamespace, operation string, ok bool) {
	if !strings.HasPrefix(value, prefix) {
		return "", "", false
	}
	server, tool, found := strings.Cut(strings.TrimPrefix(value, prefix), "__")
	if !found || server == "" || tool == "" || !authoritySegment.MatchString(server) || !remoteToolName.MatchString(tool) || strings.Contains(server, "__") {
		return "", "", false
	}
	return server, tool, true
}

func Normalize(value string) string {
	raw := value
	value = strings.Trim(invalidNameChars.ReplaceAllString(value, "_"), "_")
	if value == "" {
		value = "unnamed"
	}
	if value != raw {
		value += "_" + shortHash(raw)
	}
	return value
}

func serverComponent(value string) (string, error) {
	if value == "" {
		return "", errors.New("MCP server namespace is empty")
	}
	if strings.Contains(value, "__") {
		return "", errors.New("MCP server namespace contains reserved separator")
	}
	if !authoritySegment.MatchString(value) {
		return "", errors.New("MCP server namespace is not canonical lowercase identity")
	}
	if Normalize(value) != value {
		return "", errors.New("MCP namespace normalization would change authority identity")
	}
	return value, nil
}

func toolComponent(value string) (string, error) {
	if !remoteToolName.MatchString(value) {
		return "", fmt.Errorf("MCP tool name is not a valid case-sensitive SEP-986 identity")
	}
	return value, nil
}

func shortHash(value string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return fmt.Sprintf("%08x", hash.Sum32())[:6]
}
