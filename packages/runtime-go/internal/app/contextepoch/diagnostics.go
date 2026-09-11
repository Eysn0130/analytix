package contextepoch

func AppendThreadDiagnostics(diagnostics map[string]any, thread map[string]any) map[string]any {
	state, ok, err := StateFromThread(thread)
	if err != nil {
		diagnostics["contextEpochStateValid"] = false
		return diagnostics
	}
	if !ok {
		return diagnostics
	}
	snapshot := state.AcceptedSnapshot
	diagnostics["contextEpochStateValid"] = true
	diagnostics["contextEpoch"] = snapshot.Epoch
	diagnostics["contextEpochDigest"] = snapshot.ContextDigest
	diagnostics["contextEpochRegistryDigest"] = snapshot.RegistryDigest
	reasons := make([]string, 0, len(snapshot.ChangeReasons))
	for _, reason := range snapshot.ChangeReasons {
		reasons = append(reasons, string(reason))
	}
	diagnostics["contextEpochChangeReasons"] = reasons
	diagnostics["contextEpochImpact"] = map[string]any{
		"stablePrefix": snapshot.Impact.StablePrefix, "dynamicContext": snapshot.Impact.DynamicContext,
		"turnTail": snapshot.Impact.TurnTail, "diagnosticsOnly": snapshot.Impact.DiagnosticsOnly,
	}
	return diagnostics
}
