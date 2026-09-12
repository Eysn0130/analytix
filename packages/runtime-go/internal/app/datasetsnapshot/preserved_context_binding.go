package datasetsnapshot

import domainsecurity "analytix.local/runtime-go/internal/domain/security"

// RecordMatchesPreservedContextsV1 checks whether an already verified historical
// record belongs to a context held by startup preservation. It does not select
// a current snapshot or grant publication authority. The caller owns selecting
// the preserved context set; restricted record interpretation stays here.
func RecordMatchesPreservedContextsV1(record domainsecurity.VersionedDatasetSnapshotAuthorityRecord, contexts []domainsecurity.TurnSecurityContext) bool {
	var binding domainsecurity.DatasetSnapshotBindingKeyV1
	var snapshot string
	if record.V1 != nil {
		binding, _ = domainsecurity.DatasetSnapshotBindingKeyFromRecordV1(*record.V1)
		snapshot = record.V1.DatasetSnapshotID
	} else if record.V2 != nil {
		binding, snapshot = record.V2.Binding, record.V2.DatasetSnapshotID
	} else {
		return false
	}
	for _, frozen := range contexts {
		if snapshot == frozen.DatasetSnapshotID &&
			binding.TenantID == frozen.TenantID && binding.UserID == frozen.UserID && binding.WorkspaceRealPath == frozen.WorkspaceRealPath &&
			binding.CaseID == frozen.CaseID && binding.CaseBindingHash == frozen.CaseBindingHash && binding.BindingObservationDigest == frozen.PublicationPolicy.BindingObservationDigest {
			return true
		}
	}
	return false
}
