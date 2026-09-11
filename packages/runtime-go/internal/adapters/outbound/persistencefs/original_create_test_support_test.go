package persistencefs

// Test-only bridge lets the external integration test combine the real native
// proof producer with the existing semantic transaction interruption seam.
func SetOriginalCreateSemanticFaultForTestV1(builder *SemanticPlanBuilder, fault func(string, int) error) {
	builder.fault = fault
}
