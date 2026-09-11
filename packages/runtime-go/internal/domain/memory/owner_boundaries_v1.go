package memory

// OwnerBoundaryV1 makes the five retention authorities explicit. It is an
// executable architecture contract, not a registry and not a persistence
// source.
type OwnerBoundaryV1 struct {
	Name                  string
	PackageOwner          string
	ContentKind           string
	Retention             string
	RevokeTrigger         string
	AllowsGeneralFreeform bool
}

func OwnerBoundariesV1() []OwnerBoundaryV1 {
	return []OwnerBoundaryV1{
		{
			Name:          "turn_working_set",
			PackageOwner:  "app/loop",
			ContentKind:   "process_ephemeral",
			Retention:     "current_turn_only",
			RevokeTrigger: "turn_terminal_or_process_exit",
		},
		{
			Name:          "thread_typed_continuation",
			PackageOwner:  "caseentity/threadcontext",
			ContentKind:   "typed_provider_safe",
			Retention:     "bound_thread_generation",
			RevokeTrigger: "thread_case_binding_or_epoch_change",
		},
		{
			Name:          "case_longitudinal_state",
			PackageOwner:  "caseentity",
			ContentKind:   "typed_case_private",
			Retention:     "bound_case_generation",
			RevokeTrigger: "case_binding_revoke_or_scope_change",
		},
		{
			Name:          "retained_evidence_display_binding",
			PackageOwner:  "evidence/localdisplay",
			ContentKind:   "immutable_typed_binding",
			Retention:     "bound_snapshot_and_openable_result",
			RevokeTrigger: "binding_snapshot_or_display_lease_invalidation",
		},
		{
			Name:                  "user_approved_general_memory",
			PackageOwner:          "memory",
			ContentKind:           "manual_general_freeform",
			Retention:             "until_manual_disable_or_delete",
			RevokeTrigger:         "manual_disable_or_privacy_preserving_tombstone",
			AllowsGeneralFreeform: true,
		},
	}
}
