package loop

func CaseFundHasSuccessfulToolResult(thread map[string]any, turnID string) bool {
	// P0 fail-closed bridge: transport/tool success is not an EvidenceReceipt.
	// The P1 host registry will replace this compatibility predicate with
	// authoritative same-context receipt and claim verification.
	_ = thread
	_ = turnID
	return false
}
