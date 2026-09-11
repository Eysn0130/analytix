package toolcatalog

import (
	"strings"

	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

func ToolFailureProgressMessage(output any) string {
	summary := toolErrorSummary(output)
	if summary == "" {
		return "tool failed"
	}
	return "tool failed: " + summary
}

func toolErrorSummary(output any) string {
	projection, err := domaintoolresult.ParsePublicToolResultProjectionV1(output)
	if err != nil {
		return ""
	}
	code := strings.TrimSpace(projection.Code)
	if code != "" {
		return code
	}
	return ""
}
