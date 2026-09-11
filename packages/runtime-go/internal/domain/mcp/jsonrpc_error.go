package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONRPCError is an in-process MCP protocol error. Message and Data remain
// available for bounded host diagnostics, but are deliberately excluded from
// generic JSON persistence and from Error so untrusted server text cannot
// leak through ordinary error logging.
type JSONRPCError struct {
	Code    int             `json:"-"`
	Message string          `json:"-"`
	Data    json.RawMessage `json:"-"`
}

func JSONRPCDiagnosticRecord(err *JSONRPCError) map[string]any {
	if err == nil {
		return nil
	}
	record := map[string]any{
		"code": err.Code, "class": err.Class(), "dataPresent": len(err.Data) > 0,
	}
	if len(err.Data) > 0 {
		digest := sha256.Sum256(err.Data)
		record["dataSHA256"] = hex.EncodeToString(digest[:])
	} else {
		record["dataSHA256"] = ""
	}
	return record
}

// BoundedJSONRPCDiagnostic accepts only the host-issued, non-content-bearing
// diagnostic projection. It rejects message/data fields and class/code/hash
// mismatches so persistence and public diagnostics cannot become a bypass.
func BoundedJSONRPCDiagnostic(value any) (map[string]any, bool) {
	record, ok := value.(map[string]any)
	if !ok || len(record) != 4 {
		return nil, false
	}
	for key := range record {
		switch key {
		case "code", "class", "dataPresent", "dataSHA256":
		default:
			return nil, false
		}
	}
	code, ok := jsonRPCDiagnosticCode(record["code"])
	if !ok {
		return nil, false
	}
	class, ok := record["class"].(string)
	class = strings.TrimSpace(class)
	if !ok || class != (&JSONRPCError{Code: code}).Class() {
		return nil, false
	}
	dataPresent, ok := record["dataPresent"].(bool)
	if !ok {
		return nil, false
	}
	digest, ok := record["dataSHA256"].(string)
	digest = strings.ToLower(strings.TrimSpace(digest))
	if !ok || (dataPresent && !isFullSHA256(digest)) || (!dataPresent && digest != "") {
		return nil, false
	}
	return map[string]any{
		"code": code, "class": class, "dataPresent": dataPresent, "dataSHA256": digest,
	}, true
}

func jsonRPCDiagnosticCode(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), int64(int(typed)) == typed
	case float64:
		return int(typed), typed == float64(int(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil && int64(int(parsed)) == parsed
	default:
		return 0, false
	}
}

func isFullSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (err *JSONRPCError) Error() string {
	if err == nil {
		return "mcp jsonrpc error"
	}
	return fmt.Sprintf("mcp jsonrpc error %d (%s)", err.Code, err.Class())
}

func (err *JSONRPCError) Class() string {
	if err == nil {
		return "application_error"
	}
	switch err.Code {
	case -32700:
		return "parse_error"
	case -32600:
		return "invalid_request"
	case -32601:
		return "method_not_found"
	case -32602:
		return "invalid_params"
	case -32603:
		return "internal_error"
	default:
		if err.Code >= -32099 && err.Code <= -32000 {
			return "server_error"
		}
		return "application_error"
	}
}
