package job

import (
	"errors"
	"reflect"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const legacyUnboundSteerTombstoneDomainV1 = "analytix.legacy-unbound-steer-tombstone/v1"

// NormalizePersistedRecordV1 projects every provider-originated display field
// admitted to ordinary child-run storage. Host identifiers, digests, and
// user-owned parent text are not rewritten by this projection. The retired
// job artifact path is removed because its legacy file may contain reasoning.
func NormalizePersistedRecordV1(record Record) Record {
	failureObserved := strings.TrimSpace(record.Error) != ""
	record.Effort, _ = domainmodel.ProjectReasoningEffortV1(record.Effort)
	record.ModelExecution = ProjectPersistedModelExecutionV1(record.ModelExecution)
	record.CaseDelegation = CloneCaseDelegationContextV1(record.CaseDelegation)
	record.ChildCompletionReceipt = CloneChildCompletionReceiptV1(record.ChildCompletionReceipt)
	record.ForegroundChildHandoffReceipt = CloneForegroundChildHandoffReceiptV1(record.ForegroundChildHandoffReceipt)
	record.Usage = ProjectPersistedUsageV1(record.Kind, record.Usage)
	hostCasePrompt := record.CaseDelegation != nil && ValidateExecutableCaseDelegationV1(
		record.Kind, record.Name, record.Label, record.Prompt, record.SecurityBinding, record.CaseDelegation,
	) == nil
	record.Name = strings.TrimSpace(ProjectPersistableUntrustedOutputV1(record.Name))
	record.Label = strings.TrimSpace(ProjectPersistableUntrustedOutputV1(record.Label))
	if hostCasePrompt {
		// The case prompt is a closed host projection whose exact bytes are
		// covered by the delegation context. Sending its SHA-256 commitments
		// through the ordinary PII projector can mistake a digit-heavy digest
		// for an account number and detach the durable prompt from that binding.
		record.Prompt = strings.TrimSpace(record.Prompt)
	} else {
		record.Prompt = strings.TrimSpace(ProjectPersistableUntrustedOutputV1(record.Prompt))
	}
	record.ProfileDescription = strings.TrimSpace(ProjectPersistableUntrustedOutputV1(record.ProfileDescription))
	record.DiffSummary = strings.TrimSpace(ProjectPersistableUntrustedOutputV1(record.DiffSummary))
	record.Output = ProjectPersistableUntrustedOutputV1(record.Output)
	record.ChangedFiles = normalizePersistedChangedFilesV1(record.ChangedFiles)
	record.MergeDecisions = normalizePersistedMergeDecisionsV1(record.MergeDecisions)
	record.CleanupReceipts = normalizePersistedCleanupReceiptsV1(record.CleanupReceipts)
	record.AcceptDecisions = normalizePersistedAcceptDecisionsV1(record.AcceptDecisions)
	record.ConflictReports = normalizePersistedConflictReportsV1(record.ConflictReports)
	record.RepairPatchReviews = normalizePersistedRepairPatchReviewsV1(record.RepairPatchReviews)
	record.ChildTodoLists = normalizePersistedChildTodoListsV1(record.ChildTodoLists)
	record.ChildTodoProjections = normalizePersistedChildTodoProjectionsV1(record.ChildTodoProjections)
	record.ProjectionDecisions = normalizePersistedProjectionDecisionsV1(record.ProjectionDecisions)
	record.ArtifactPath = ""
	record.FailureCode = NormalizeFailureCode(record.FailureCode, record.Status, failureObserved)
	// Error text is attempt-private. Durable job state retains only the closed
	// FailureCode classification, including ordinary/background-shell jobs.
	record.Error = ""
	if strings.TrimSpace(record.Kind) == "background-shell" || strings.TrimSpace(record.Kind) == "bash" {
		record.Name = "bash"
		record.Label = "background shell"
		if record.SecurityBinding != nil {
			record.Output = ""
			record.Error = ""
		}
	}
	record.AutoContinueReason = NormalizeOperationalReasonV1(record.AutoContinueReason)
	record.AutoContinueError = ""
	record.LateCompletionReason = NormalizeOperationalReasonV1(record.LateCompletionReason)
	record.CompletionDeliveryReason = NormalizeOperationalReasonV1(record.CompletionDeliveryReason)
	record.CompletionDeliveryError = ""
	record.RecoveryReason = NormalizeOperationalReasonV1(record.RecoveryReason)
	record.DeadLetterReason = NormalizeOperationalReasonV1(record.DeadLetterReason)
	if len(record.PauseRequests) > 0 {
		record.PauseRequests = append([]PauseRequest(nil), record.PauseRequests...)
		for index := range record.PauseRequests {
			request := &record.PauseRequests[index]
			request.RejectedReason = NormalizeOperationalReasonV1(request.RejectedReason)
			if strings.TrimSpace(request.Status) == "resumed" {
				// A retired producer retained the already-consumed resume
				// capability after settlement. The issued/expires audit times stay
				// intact; only the unusable bearer value is removed.
				request.ResumeToken = ""
			}
		}
	}
	if len(record.Steers) > 0 {
		record.Steers = append([]SteerMessage(nil), record.Steers...)
		for index := range record.Steers {
			steer := &record.Steers[index]
			steer.RejectedReason = NormalizeOperationalReasonV1(steer.RejectedReason)
			if legacyUnboundSteerV0(record, *steer) {
				// The retired producer persisted steer text and lifecycle labels
				// without a projection or queue authority. Never infer that these
				// messages were admitted. Preserve only their already-matching
				// structural identity as a closed, non-executable tombstone.
				projectLegacyUnboundSteerTombstoneV1(steer)
				continue
			}
			if (strings.TrimSpace(steer.Status) == "queued" || strings.TrimSpace(steer.Status) == "admitted") &&
				ValidateSteerTextProjectionV1(steer.Text) != nil {
				// Semantic startup may encounter a structurally valid legacy steer
				// whose raw text predates host projection. Preserve only its opaque
				// identity/digests as a non-executable tombstone; never rewrite the
				// text and recompute authority from attacker-controlled bytes.
				steer.Status = "expired"
				steer.Text = ""
				steer.SourceTurnID = ""
				steer.SourceToolCallID = ""
				steer.AdmittedAt = ""
				steer.PromotionCommitID = ""
				steer.PromotionEntryID = ""
				steer.RejectedReason = "legacy_private_steer_removed"
			}
		}
	}
	if record.SecurityBinding != nil && strings.TrimSpace(record.Kind) == "subagent" {
		record.Output = ""
		record.Error = ""
		record.Usage = nil
	}
	return record
}

func legacyUnboundSteerV0(record Record, steer SteerMessage) bool {
	status := strings.TrimSpace(steer.Status)
	if record.SecurityBinding != nil || (status != "queued" && status != "admitted" && status != "rejected" && status != "expired") ||
		strings.TrimSpace(steer.ID) == "" || len(strings.TrimSpace(steer.ID)) > 256 ||
		strings.TrimSpace(steer.ParentThreadID) != strings.TrimSpace(record.ParentThreadID) ||
		strings.TrimSpace(steer.ChildRunID) != strings.TrimSpace(record.ID) || strings.TrimSpace(steer.JobID) != strings.TrimSpace(record.ID) {
		return false
	}
	return steer.ProjectionVersion == 0 && strings.TrimSpace(steer.ContentDigest) == "" &&
		strings.TrimSpace(steer.QueueAuthorityDigest) == "" && strings.TrimSpace(steer.AuthorityDigest) == "" &&
		strings.TrimSpace(steer.ContextDigest) == ""
}

func projectLegacyUnboundSteerTombstoneV1(steer *SteerMessage) {
	if steer == nil {
		return
	}
	steer.Status = "expired"
	steer.Text = ""
	steer.SourceTurnID = ""
	steer.SourceToolCallID = ""
	steer.AdmittedAt = ""
	steer.PromotionCommitID = ""
	steer.PromotionEntryID = ""
	steer.ContextDigest = ""
	steer.AuthorityDigest = ""
	steer.ProjectionVersion = SteerMessageProjectionVersionV1
	steer.RejectedReason = "legacy_private_steer_removed"
	steer.ContentDigest = SteerMessageContentDigestV1(*steer)
	steer.QueueAuthorityDigest = domainsecurity.SHA256Hex([]byte(strings.Join([]string{
		legacyUnboundSteerTombstoneDomainV1,
		strings.TrimSpace(steer.ID),
		strings.TrimSpace(steer.ParentThreadID),
		strings.TrimSpace(steer.ChildRunID),
		strings.TrimSpace(steer.JobID),
		strings.TrimSpace(steer.ContentDigest),
		strings.TrimSpace(steer.CreatedAt),
	}, "\x00")))
}

// ValidateSemanticMigrationSourceV1 rejects authority-bound private payloads
// before normalization can erase the evidence that the persisted record was
// tampered with. Authority-free legacy display text may still be projected in
// the isolated semantic stage.
func ValidateSemanticMigrationSourceV1(record Record) error {
	if record.SecurityBinding == nil {
		return nil
	}
	if ValidateSecurityBinding(record.SecurityBinding) != nil {
		return errors.New("job semantic migration source security binding is invalid")
	}
	// A valid binding marks the record as already admitted under host-issued
	// context and grant authority. Semantic startup may validate that record,
	// but it may not rewrite any of its fields and silently retain the old
	// binding. Legacy projection is reserved for authority-free records.
	if !reflect.DeepEqual(record, NormalizePersistedRecordV1(record)) {
		return errors.New("security-bound job record cannot be migrated")
	}
	if strings.TrimSpace(record.Kind) != "subagent" {
		return nil
	}
	if record.Output != "" {
		return errors.New("security-bound child output cannot be migrated")
	}
	if record.Error != "" {
		return errors.New("security-bound child error text cannot be migrated")
	}
	if record.Usage != nil {
		return errors.New("security-bound child usage cannot be migrated")
	}
	return nil
}

// ValidatePersistedProjectionV1 rejects live-tree records that bypassed the
// semantic startup migration. Rewriting is allowed only in the isolated stage.
func ValidatePersistedProjectionV1(record Record) error {
	projected := NormalizePersistedRecordV1(record)
	if !reflect.DeepEqual(record.ModelExecution, projected.ModelExecution) {
		return errors.Join(errors.New("job record contains unprojected provider-originated content"), ErrPersistedModelExecutionInvalid)
	}
	if !reflect.DeepEqual(record.Usage, projected.Usage) {
		return errors.Join(errors.New("job record contains unprojected provider-originated content"), ErrPersistedUsageInvalid)
	}
	if record.Name != projected.Name || record.Label != projected.Label || record.Prompt != projected.Prompt ||
		record.ProfileDescription != projected.ProfileDescription || record.DiffSummary != projected.DiffSummary ||
		record.Effort != projected.Effort ||
		record.Output != projected.Output || record.Error != projected.Error || record.FailureCode != projected.FailureCode ||
		record.ArtifactPath != projected.ArtifactPath ||
		record.AutoContinueReason != projected.AutoContinueReason || record.AutoContinueError != "" ||
		record.LateCompletionReason != projected.LateCompletionReason ||
		record.CompletionDeliveryReason != projected.CompletionDeliveryReason || record.CompletionDeliveryError != "" ||
		record.RecoveryReason != projected.RecoveryReason || record.DeadLetterReason != projected.DeadLetterReason ||
		!samePersistedPauseReasonsV1(record.PauseRequests, projected.PauseRequests) ||
		!reflect.DeepEqual(record.Steers, projected.Steers) ||
		!reflect.DeepEqual(record.ChangedFiles, projected.ChangedFiles) ||
		!reflect.DeepEqual(record.MergeDecisions, projected.MergeDecisions) ||
		!reflect.DeepEqual(record.CleanupReceipts, projected.CleanupReceipts) ||
		!reflect.DeepEqual(record.AcceptDecisions, projected.AcceptDecisions) ||
		!reflect.DeepEqual(record.ConflictReports, projected.ConflictReports) ||
		!reflect.DeepEqual(record.RepairPatchReviews, projected.RepairPatchReviews) ||
		!reflect.DeepEqual(record.ChildTodoLists, projected.ChildTodoLists) ||
		!reflect.DeepEqual(record.ChildTodoProjections, projected.ChildTodoProjections) ||
		!reflect.DeepEqual(record.ProjectionDecisions, projected.ProjectionDecisions) ||
		(record.SecurityBinding != nil && strings.TrimSpace(record.Kind) == "subagent" && record.Usage != nil) {
		return errors.New("job record contains unprojected provider-originated content")
	}
	if err := ValidatePersistedOperationalStatusesV1(record); err != nil {
		return err
	}
	return nil
}

func normalizePersistedDisplayTextV1(value string) string {
	return strings.TrimSpace(ProjectPersistableUntrustedOutputV1(value))
}

func normalizePersistedChangedFilesV1(values []ChangedFile) []ChangedFile {
	if len(values) == 0 {
		return nil
	}
	out := append([]ChangedFile(nil), values...)
	for index := range out {
		out[index].Path = normalizePersistedDisplayTextV1(out[index].Path)
		out[index].Status = normalizePersistedDisplayTextV1(out[index].Status)
	}
	return out
}

func normalizePersistedMergeDecisionsV1(values []MergeDecision) []MergeDecision {
	if len(values) == 0 {
		return nil
	}
	out := append([]MergeDecision(nil), values...)
	for index := range out {
		out[index].Reason = NormalizeOperationalReasonV1(out[index].Reason)
	}
	return out
}

func normalizePersistedCleanupReceiptsV1(values []CleanupReceipt) []CleanupReceipt {
	if len(values) == 0 {
		return nil
	}
	out := append([]CleanupReceipt(nil), values...)
	for index := range out {
		out[index].RetainedReason = NormalizeOperationalReasonV1(out[index].RetainedReason)
	}
	return out
}

func normalizePersistedAcceptDecisionsV1(values []AcceptDecision) []AcceptDecision {
	if len(values) == 0 {
		return nil
	}
	out := append([]AcceptDecision(nil), values...)
	for index := range out {
		out[index].ChangedFiles = normalizePersistedChangedFilesV1(out[index].ChangedFiles)
	}
	return out
}

func normalizePersistedConflictReportsV1(values []ConflictReport) []ConflictReport {
	if len(values) == 0 {
		return nil
	}
	out := append([]ConflictReport(nil), values...)
	for index := range out {
		out[index].ConflictFiles = normalizePersistedChangedFilesV1(out[index].ConflictFiles)
		out[index].ConflictSummary = normalizePersistedDisplayTextV1(out[index].ConflictSummary)
	}
	return out
}

func normalizePersistedRepairPatchReviewsV1(values []RepairPatchReview) []RepairPatchReview {
	if len(values) == 0 {
		return nil
	}
	out := append([]RepairPatchReview(nil), values...)
	for index := range out {
		out[index].ChangedFiles = normalizePersistedChangedFilesV1(out[index].ChangedFiles)
		out[index].RejectedReason = NormalizeOperationalReasonV1(out[index].RejectedReason)
		// A raw repair patch is controlled workspace content, not ordinary job
		// metadata. Persist only its host-issued digest and dry-run receipt.
		out[index].RepairPatch = ""
	}
	return out
}

func normalizePersistedChildTodoListsV1(values []ChildTodoList) []ChildTodoList {
	if len(values) == 0 {
		return nil
	}
	out := append([]ChildTodoList(nil), values...)
	for listIndex := range out {
		out[listIndex].Scope = normalizePersistedDisplayTextV1(out[listIndex].Scope)
		out[listIndex].Items = append([]ChildTodoItem(nil), out[listIndex].Items...)
		for itemIndex := range out[listIndex].Items {
			item := &out[listIndex].Items[itemIndex]
			item.Content = normalizePersistedDisplayTextV1(item.Content)
			item.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
		}
	}
	return out
}

func normalizePersistedChildTodoProjectionsV1(values []ChildTodoProjection) []ChildTodoProjection {
	if len(values) == 0 {
		return nil
	}
	out := append([]ChildTodoProjection(nil), values...)
	for projectionIndex := range out {
		projection := &out[projectionIndex]
		projection.Summary = normalizePersistedDisplayTextV1(projection.Summary)
		projection.EvidenceIDs = append([]string(nil), projection.EvidenceIDs...)
		projection.ProjectedItems = append([]ChildTodoProjectionItem(nil), projection.ProjectedItems...)
		for itemIndex := range projection.ProjectedItems {
			item := &projection.ProjectedItems[itemIndex]
			item.Content = normalizePersistedDisplayTextV1(item.Content)
			item.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
		}
	}
	return out
}

func normalizePersistedProjectionDecisionsV1(values []ProjectionDecision) []ProjectionDecision {
	if len(values) == 0 {
		return nil
	}
	out := append([]ProjectionDecision(nil), values...)
	for decisionIndex := range out {
		decision := &out[decisionIndex]
		decision.AcceptedItems = append([]AcceptedProjectionItem(nil), decision.AcceptedItems...)
		for itemIndex := range decision.AcceptedItems {
			decision.AcceptedItems[itemIndex].EvidenceIDs = append([]string(nil), decision.AcceptedItems[itemIndex].EvidenceIDs...)
		}
		decision.SkippedItems = append([]SkippedProjectionItem(nil), decision.SkippedItems...)
		for itemIndex := range decision.SkippedItems {
			item := &decision.SkippedItems[itemIndex]
			item.Reason = NormalizeOperationalReasonV1(item.Reason)
			item.EvidenceIDs = append([]string(nil), item.EvidenceIDs...)
		}
	}
	return out
}

func samePersistedPauseReasonsV1(left []PauseRequest, right []PauseRequest) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].RejectedReason != right[index].RejectedReason {
			return false
		}
	}
	return true
}
