package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
)

const ExecutionGrantRegistryVersion = 1

type ExecutionGrantRegistryStatus string

const (
	GrantRegistryPending ExecutionGrantRegistryStatus = "pending"
	GrantRegistryActive  ExecutionGrantRegistryStatus = "active"
	GrantRegistryRevoked ExecutionGrantRegistryStatus = "revoked"
	GrantRegistrySettled ExecutionGrantRegistryStatus = "settled"
)

type ExecutionGrantRegistryEntry struct {
	Version       int                          `json:"version"`
	Sequence      uint64                       `json:"sequence"`
	ThreadID      string                       `json:"threadId"`
	TurnID        string                       `json:"turnId"`
	ContextDigest string                       `json:"contextDigest"`
	Grant         ExecutionGrant               `json:"grant"`
	Status        ExecutionGrantRegistryStatus `json:"status"`
	ParentGrantID string                       `json:"parentGrantId,omitempty"`
	RegisteredAt  string                       `json:"registeredAt"`
	UpdatedAt     string                       `json:"updatedAt"`
	EntryDigest   string                       `json:"entryDigest"`
}

type ExecutionGrantRegistry struct {
	Version     int                           `json:"version"`
	ThreadID    string                        `json:"threadId"`
	Sequence    uint64                        `json:"sequence"`
	Entries     []ExecutionGrantRegistryEntry `json:"entries"`
	StateDigest string                        `json:"stateDigest"`
}

func NewExecutionGrantRegistry(threadID string) ExecutionGrantRegistry {
	return SealExecutionGrantRegistry(ExecutionGrantRegistry{
		Version: ExecutionGrantRegistryVersion, ThreadID: strings.TrimSpace(threadID), Entries: []ExecutionGrantRegistryEntry{},
	})
}

func ParseExecutionGrantRegistry(value any) (ExecutionGrantRegistry, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ExecutionGrantRegistry{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var registry ExecutionGrantRegistry
	if err := decoder.Decode(&registry); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ExecutionGrantRegistry{}, errors.New("execution grant registry contains trailing JSON")
	}
	if err := ValidateExecutionGrantRegistry(registry); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	return registry, nil
}

func SealExecutionGrantRegistry(registry ExecutionGrantRegistry) ExecutionGrantRegistry {
	registry.Version = ExecutionGrantRegistryVersion
	registry.ThreadID = strings.TrimSpace(registry.ThreadID)
	registry.Entries = append([]ExecutionGrantRegistryEntry(nil), registry.Entries...)
	sort.Slice(registry.Entries, func(i, j int) bool { return registry.Entries[i].Sequence < registry.Entries[j].Sequence })
	var sequence uint64
	for index := range registry.Entries {
		registry.Entries[index] = sealExecutionGrantRegistryEntry(registry.Entries[index])
		if registry.Entries[index].Sequence > sequence {
			sequence = registry.Entries[index].Sequence
		}
	}
	registry.Sequence = sequence
	registry.StateDigest = ""
	registry.StateDigest = hashContract(registry)
	return registry
}

func ValidateExecutionGrantRegistry(registry ExecutionGrantRegistry) error {
	return validateExecutionGrantRegistry(registry, ValidateExecutionGrant)
}

// ValidateExecutionGrantRegistryForAudit checks immutable historical state.
// Every execution and registry mutation API retains current grant validation.
func ValidateExecutionGrantRegistryForAudit(registry ExecutionGrantRegistry) error {
	return validateExecutionGrantRegistry(registry, ValidateExecutionGrantForAudit)
}

func validateExecutionGrantRegistry(registry ExecutionGrantRegistry, validateGrant func(ExecutionGrant) error) error {
	if registry.Version != ExecutionGrantRegistryVersion || strings.TrimSpace(registry.ThreadID) == "" || !isSHA256Hex(registry.StateDigest) {
		return errors.New("execution grant registry identity is invalid")
	}
	seen := map[string]bool{}
	for index, entry := range registry.Entries {
		if err := validateExecutionGrantRegistryEntryWithGrant(entry, validateGrant); err != nil {
			return err
		}
		if entry.ThreadID != registry.ThreadID || entry.Sequence != uint64(index+1) || seen[entry.Grant.GrantID] {
			return errors.New("execution grant registry sequence or membership is invalid")
		}
		if entry.ParentGrantID != "" && !seen[entry.ParentGrantID] {
			return errors.New("execution grant registry parent membership is invalid")
		}
		seen[entry.Grant.GrantID] = true
	}
	if registry.Sequence != uint64(len(registry.Entries)) {
		return errors.New("execution grant registry sequence is invalid")
	}
	if expected := SealExecutionGrantRegistry(registry); expected.StateDigest != registry.StateDigest {
		return errors.New("execution grant registry integrity is invalid")
	}
	return nil
}

func RegisterExecutionGrant(registry ExecutionGrantRegistry, threadID string, grant ExecutionGrant, at time.Time) (ExecutionGrantRegistry, error) {
	if err := ValidateExecutionGrantRegistry(registry); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	threadID = strings.TrimSpace(threadID)
	if registry.ThreadID != threadID || grant.TurnID == "" || grant.ContextDigest == "" {
		return ExecutionGrantRegistry{}, errors.New("execution grant registry authority mismatch")
	}
	if err := ValidateExecutionGrant(grant); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	if _, found := ExecutionGrantRegistryEntryByID(registry, grant.GrantID); found {
		return ExecutionGrantRegistry{}, errors.New("execution grant is already registered")
	}
	registry.Entries = append([]ExecutionGrantRegistryEntry(nil), registry.Entries...)
	status := GrantRegistryActive
	if grant.ApprovalState == "pending" {
		status = GrantRegistryPending
	}
	stamp := registryTime(at)
	registry.Entries = append(registry.Entries, sealExecutionGrantRegistryEntry(ExecutionGrantRegistryEntry{
		Version: ExecutionGrantRegistryVersion, Sequence: registry.Sequence + 1, ThreadID: threadID,
		TurnID: grant.TurnID, ContextDigest: grant.ContextDigest, Grant: grant, Status: status,
		RegisteredAt: stamp, UpdatedAt: stamp,
	}))
	registry = SealExecutionGrantRegistry(registry)
	return registry, ValidateExecutionGrantRegistry(registry)
}

func ApproveRegisteredExecutionGrant(registry ExecutionGrantRegistry, pending ExecutionGrant, approved ExecutionGrant, at time.Time) (ExecutionGrantRegistry, error) {
	if err := ValidateExecutionGrantRegistry(registry); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	if err := ValidateExecutionGrant(pending); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	if err := ValidateExecutionGrant(approved); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	index, entry, found := executionGrantRegistryEntryIndex(registry, pending.GrantID)
	if !found || entry.Status != GrantRegistryPending || entry.Grant != pending || pending.ApprovalState != "pending" || approved.ApprovalState != "approved" || !sameExecutionGrantAuthority(pending, approved) {
		return ExecutionGrantRegistry{}, errors.New("pending execution grant approval transition is invalid")
	}
	if _, duplicate := ExecutionGrantRegistryEntryByID(registry, approved.GrantID); duplicate {
		return ExecutionGrantRegistry{}, errors.New("approved execution grant is already registered")
	}
	registry.Entries = append([]ExecutionGrantRegistryEntry(nil), registry.Entries...)
	stamp := registryTime(at)
	entry.Status = GrantRegistryRevoked
	entry.UpdatedAt = stamp
	registry.Entries[index] = sealExecutionGrantRegistryEntry(entry)
	registry.Entries = append(registry.Entries, sealExecutionGrantRegistryEntry(ExecutionGrantRegistryEntry{
		Version: ExecutionGrantRegistryVersion, Sequence: registry.Sequence + 1, ThreadID: registry.ThreadID,
		TurnID: approved.TurnID, ContextDigest: approved.ContextDigest, Grant: approved, Status: GrantRegistryActive,
		ParentGrantID: pending.GrantID, RegisteredAt: stamp, UpdatedAt: stamp,
	}))
	registry = SealExecutionGrantRegistry(registry)
	return registry, ValidateExecutionGrantRegistry(registry)
}

func SettleRegisteredExecutionGrant(registry ExecutionGrantRegistry, grantID string, at time.Time) (ExecutionGrantRegistry, error) {
	if err := ValidateExecutionGrantRegistry(registry); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	index, entry, found := executionGrantRegistryEntryIndex(registry, grantID)
	if !found || (entry.Status != GrantRegistryActive && entry.Status != GrantRegistryPending) {
		return ExecutionGrantRegistry{}, errors.New("execution grant is not settleable")
	}
	registry.Entries = append([]ExecutionGrantRegistryEntry(nil), registry.Entries...)
	entry.Status = GrantRegistrySettled
	entry.UpdatedAt = registryTime(at)
	registry.Entries[index] = sealExecutionGrantRegistryEntry(entry)
	registry = SealExecutionGrantRegistry(registry)
	return registry, ValidateExecutionGrantRegistry(registry)
}

func RevokeRegisteredExecutionGrant(registry ExecutionGrantRegistry, grantID string, at time.Time) (ExecutionGrantRegistry, error) {
	if err := ValidateExecutionGrantRegistry(registry); err != nil {
		return ExecutionGrantRegistry{}, err
	}
	index, entry, found := executionGrantRegistryEntryIndex(registry, grantID)
	if !found || (entry.Status != GrantRegistryActive && entry.Status != GrantRegistryPending) {
		return ExecutionGrantRegistry{}, errors.New("execution grant is not revocable")
	}
	registry.Entries = append([]ExecutionGrantRegistryEntry(nil), registry.Entries...)
	entry.Status = GrantRegistryRevoked
	entry.UpdatedAt = registryTime(at)
	registry.Entries[index] = sealExecutionGrantRegistryEntry(entry)
	registry = SealExecutionGrantRegistry(registry)
	return registry, ValidateExecutionGrantRegistry(registry)
}

func VerifyExecutionGrantMembership(registry ExecutionGrantRegistry, threadID string, turnID string, grant ExecutionGrant, allowed ...ExecutionGrantRegistryStatus) error {
	if err := ValidateExecutionGrantRegistry(registry); err != nil {
		return err
	}
	entry, found := ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !found || entry.Grant != grant || entry.ThreadID != strings.TrimSpace(threadID) || entry.TurnID != strings.TrimSpace(turnID) ||
		entry.ContextDigest != grant.ContextDigest {
		return errors.New("execution grant registry membership is invalid")
	}
	for _, status := range allowed {
		if entry.Status == status {
			return nil
		}
	}
	return errors.New("execution grant registry status is invalid")
}

func ExecutionGrantRegistryEntryByID(registry ExecutionGrantRegistry, grantID string) (ExecutionGrantRegistryEntry, bool) {
	_, entry, found := executionGrantRegistryEntryIndex(registry, grantID)
	return entry, found
}

func ExecutionGrantRegistryRecord(registry ExecutionGrantRegistry) map[string]any {
	body, _ := json.Marshal(registry)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func validateExecutionGrantRegistryEntryWithGrant(entry ExecutionGrantRegistryEntry, validateGrant func(ExecutionGrant) error) error {
	if entry.Version != ExecutionGrantRegistryVersion || entry.Sequence == 0 || strings.TrimSpace(entry.ThreadID) == "" ||
		entry.TurnID != entry.Grant.TurnID || entry.ContextDigest != entry.Grant.ContextDigest || !isSHA256Hex(entry.EntryDigest) {
		return errors.New("execution grant registry entry identity is invalid")
	}
	if err := validateGrant(entry.Grant); err != nil {
		return err
	}
	registeredAt, registeredErr := time.Parse(time.RFC3339Nano, entry.RegisteredAt)
	updatedAt, updatedErr := time.Parse(time.RFC3339Nano, entry.UpdatedAt)
	if registeredErr != nil || updatedErr != nil || updatedAt.Before(registeredAt) {
		return errors.New("execution grant registry entry time is invalid")
	}
	switch entry.Status {
	case GrantRegistryPending:
		if entry.Grant.ApprovalState != "pending" {
			return errors.New("execution grant registry pending state is invalid")
		}
	case GrantRegistryActive:
		if entry.Grant.ApprovalState != "not_required" && entry.Grant.ApprovalState != "approved" {
			return errors.New("execution grant registry active state is invalid")
		}
	case GrantRegistryRevoked, GrantRegistrySettled:
	default:
		return errors.New("execution grant registry status is invalid")
	}
	if expected := sealExecutionGrantRegistryEntry(entry); expected.EntryDigest != entry.EntryDigest {
		return errors.New("execution grant registry entry integrity is invalid")
	}
	return nil
}

func sealExecutionGrantRegistryEntry(entry ExecutionGrantRegistryEntry) ExecutionGrantRegistryEntry {
	entry.Version = ExecutionGrantRegistryVersion
	entry.ThreadID = strings.TrimSpace(entry.ThreadID)
	entry.TurnID = strings.TrimSpace(entry.TurnID)
	entry.ContextDigest = strings.TrimSpace(entry.ContextDigest)
	entry.ParentGrantID = strings.TrimSpace(entry.ParentGrantID)
	entry.RegisteredAt = strings.TrimSpace(entry.RegisteredAt)
	entry.UpdatedAt = strings.TrimSpace(entry.UpdatedAt)
	entry.EntryDigest = ""
	entry.EntryDigest = hashContract(entry)
	return entry
}

func executionGrantRegistryEntryIndex(registry ExecutionGrantRegistry, grantID string) (int, ExecutionGrantRegistryEntry, bool) {
	grantID = strings.TrimSpace(grantID)
	for index, entry := range registry.Entries {
		if entry.Grant.GrantID == grantID {
			return index, entry, true
		}
	}
	return -1, ExecutionGrantRegistryEntry{}, false
}

func sameExecutionGrantAuthority(left ExecutionGrant, right ExecutionGrant) bool {
	left.GrantID, right.GrantID = "", ""
	left.ApprovalState, right.ApprovalState = "", ""
	left.IssuedAt, right.IssuedAt = "", ""
	return left == right
}

func registryTime(at time.Time) string {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return at.UTC().Format(time.RFC3339Nano)
}
