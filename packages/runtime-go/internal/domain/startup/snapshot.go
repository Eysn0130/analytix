package startup

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ManagedSnapshotSchemaVersion    = 1
	ReadOnlyBaselineSchemaVersion   = 1
	ReadOnlyStartupPlannerVersionV1 = "read-only-startup-planner-v1"
	ManagedEntryTypeAbsent          = "absent"
	ManagedEntryTypeDirectory       = "directory"
	ManagedEntryTypeFile            = "file"
	MaxManagedSnapshotEntriesV1     = 100_000
)

type ManagedEntryStateV1 struct {
	Path            string `json:"path"`
	Type            string `json:"type"`
	Mode            uint32 `json:"mode"`
	Size            int64  `json:"size"`
	ModTimeUnixNano int64  `json:"modTimeUnixNano"`
	SHA256          string `json:"sha256,omitempty"`
	RecordCount     int    `json:"recordCount"`
}

type ManagedSnapshotV1 struct {
	SchemaVersion     int                   `json:"schemaVersion"`
	RootBindingDigest string                `json:"rootBindingDigest"`
	RawCaptureDigest  string                `json:"rawCaptureDigest"`
	Entries           []ManagedEntryStateV1 `json:"entries"`
	SnapshotDigest    string                `json:"snapshotDigest"`
}

type ReadOnlyStartupBaselineV1 struct {
	SchemaVersion         int    `json:"schemaVersion"`
	PlannerVersion        string `json:"plannerVersion"`
	RootBindingDigest     string `json:"rootBindingDigest"`
	RawCaptureDigest      string `json:"rawCaptureDigest"`
	ManagedSnapshotDigest string `json:"managedSnapshotDigest"`
	ConfigurationDigest   string `json:"configurationDigest"`
	CapturedAt            string `json:"capturedAt"`
	BaselineDigest        string `json:"baselineDigest"`
}

func NewManagedSnapshotV1(rootPaths []string, rawCaptureDigest string, entries []ManagedEntryStateV1) (ManagedSnapshotV1, error) {
	if len(entries) > MaxManagedSnapshotEntriesV1 {
		return ManagedSnapshotV1{}, errors.New("startup snapshot entry budget is exceeded")
	}
	paths := append([]string(nil), rootPaths...)
	seenRoots := map[string]bool{}
	for index := range paths {
		paths[index] = strings.TrimSpace(paths[index])
		if paths[index] == "" || seenRoots[paths[index]] {
			return ManagedSnapshotV1{}, errors.New("startup snapshot roots are invalid")
		}
		seenRoots[paths[index]] = true
	}
	sort.Strings(paths)
	rootBytes, err := json.Marshal(paths)
	if err != nil || len(paths) == 0 {
		return ManagedSnapshotV1{}, errors.New("startup snapshot roots are invalid")
	}
	snapshot := ManagedSnapshotV1{
		SchemaVersion:     ManagedSnapshotSchemaVersion,
		RootBindingDigest: domainsecurity.SHA256Hex(rootBytes),
		RawCaptureDigest:  strings.TrimSpace(rawCaptureDigest),
		Entries:           append([]ManagedEntryStateV1(nil), entries...),
	}
	sort.Slice(snapshot.Entries, func(left int, right int) bool {
		if snapshot.Entries[left].Path != snapshot.Entries[right].Path {
			return snapshot.Entries[left].Path < snapshot.Entries[right].Path
		}
		return snapshot.Entries[left].Type < snapshot.Entries[right].Type
	})
	if err := validateManagedSnapshotFields(snapshot); err != nil {
		return ManagedSnapshotV1{}, err
	}
	snapshot.SnapshotDigest = managedSnapshotDigest(snapshot)
	return snapshot, nil
}

func ValidateManagedSnapshotV1(snapshot ManagedSnapshotV1) error {
	if err := validateManagedSnapshotFields(snapshot); err != nil || !domainsecurity.IsSHA256Hex(snapshot.SnapshotDigest) || managedSnapshotDigest(snapshot) != snapshot.SnapshotDigest {
		return errors.New("startup snapshot integrity is invalid")
	}
	return nil
}

// ManagedSnapshotHasSemanticInputV1 reports whether the host-captured managed
// persistence inventory contains any file that must pass through semantic
// startup migration before activation. Absent targets and empty directories do
// not create migration work; callers must not infer this state from entry
// cardinality because every absent target has an explicit snapshot entry.
func ManagedSnapshotHasSemanticInputV1(snapshot ManagedSnapshotV1) (bool, error) {
	if err := ValidateManagedSnapshotV1(snapshot); err != nil {
		return false, err
	}
	for _, entry := range snapshot.Entries {
		if entry.Type == ManagedEntryTypeFile {
			return true, nil
		}
	}
	return false, nil
}

func NewReadOnlyStartupBaselineV1(snapshot ManagedSnapshotV1, configurationDigest string, capturedAt time.Time) (ReadOnlyStartupBaselineV1, error) {
	if ValidateManagedSnapshotV1(snapshot) != nil || !domainsecurity.IsSHA256Hex(strings.TrimSpace(configurationDigest)) {
		return ReadOnlyStartupBaselineV1{}, errors.New("startup baseline input is invalid")
	}
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	baseline := ReadOnlyStartupBaselineV1{
		SchemaVersion: ReadOnlyBaselineSchemaVersion, PlannerVersion: ReadOnlyStartupPlannerVersionV1,
		RootBindingDigest: snapshot.RootBindingDigest, RawCaptureDigest: snapshot.RawCaptureDigest,
		ManagedSnapshotDigest: snapshot.SnapshotDigest, ConfigurationDigest: strings.TrimSpace(configurationDigest),
		CapturedAt: capturedAt.UTC().Format(time.RFC3339Nano),
	}
	baseline.BaselineDigest = readOnlyBaselineDigest(baseline)
	return baseline, nil
}

func ValidateReadOnlyStartupBaselineV1(baseline ReadOnlyStartupBaselineV1) error {
	if baseline.SchemaVersion != ReadOnlyBaselineSchemaVersion || baseline.PlannerVersion != ReadOnlyStartupPlannerVersionV1 ||
		!domainsecurity.IsSHA256Hex(baseline.RootBindingDigest) || !domainsecurity.IsSHA256Hex(baseline.RawCaptureDigest) ||
		!domainsecurity.IsSHA256Hex(baseline.ManagedSnapshotDigest) || !domainsecurity.IsSHA256Hex(baseline.ConfigurationDigest) ||
		!domainsecurity.IsSHA256Hex(baseline.BaselineDigest) || readOnlyBaselineDigest(baseline) != baseline.BaselineDigest {
		return errors.New("startup baseline integrity is invalid")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, baseline.CapturedAt); err != nil || parsed.Format(time.RFC3339Nano) != baseline.CapturedAt {
		return errors.New("startup baseline time is invalid")
	}
	return nil
}

func validateManagedSnapshotFields(snapshot ManagedSnapshotV1) error {
	if snapshot.SchemaVersion != ManagedSnapshotSchemaVersion || !domainsecurity.IsSHA256Hex(snapshot.RootBindingDigest) ||
		!domainsecurity.IsSHA256Hex(snapshot.RawCaptureDigest) || len(snapshot.Entries) == 0 ||
		len(snapshot.Entries) > MaxManagedSnapshotEntriesV1 {
		return errors.New("startup snapshot fields are invalid")
	}
	seen := map[string]bool{}
	previous := ""
	for _, entry := range snapshot.Entries {
		path := strings.TrimSpace(entry.Path)
		if path == "" || len(path) > MaxSemanticManagedPathBytesV1 || strings.Count(path, "/")+1 > MaxSemanticManagedPathDepthV1 ||
			strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains("/"+path+"/", "/../") ||
			seen[path] || (previous != "" && path < previous) || entry.Size < 0 || entry.RecordCount < 0 {
			return errors.New("startup snapshot entry is invalid")
		}
		seen[path] = true
		previous = path
		switch entry.Type {
		case ManagedEntryTypeAbsent:
			if entry.Mode != 0 || entry.Size != 0 || entry.ModTimeUnixNano != 0 || entry.SHA256 != "" || entry.RecordCount != 0 {
				return errors.New("startup absent entry is invalid")
			}
		case ManagedEntryTypeDirectory:
			if entry.SHA256 != "" || entry.RecordCount != 0 {
				return errors.New("startup directory entry is invalid")
			}
		case ManagedEntryTypeFile:
			if entry.Size > MaxSemanticManagedFileBytesV1 || !domainsecurity.IsSHA256Hex(entry.SHA256) {
				return errors.New("startup file entry is invalid")
			}
		default:
			return errors.New("startup snapshot entry type is invalid")
		}
	}
	return nil
}

func managedSnapshotDigest(snapshot ManagedSnapshotV1) string {
	body := struct {
		SchemaVersion     int                   `json:"schemaVersion"`
		RootBindingDigest string                `json:"rootBindingDigest"`
		RawCaptureDigest  string                `json:"rawCaptureDigest"`
		Entries           []ManagedEntryStateV1 `json:"entries"`
	}{snapshot.SchemaVersion, snapshot.RootBindingDigest, snapshot.RawCaptureDigest, snapshot.Entries}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded)
}

func readOnlyBaselineDigest(baseline ReadOnlyStartupBaselineV1) string {
	body := struct {
		SchemaVersion         int    `json:"schemaVersion"`
		PlannerVersion        string `json:"plannerVersion"`
		RootBindingDigest     string `json:"rootBindingDigest"`
		RawCaptureDigest      string `json:"rawCaptureDigest"`
		ManagedSnapshotDigest string `json:"managedSnapshotDigest"`
		ConfigurationDigest   string `json:"configurationDigest"`
		CapturedAt            string `json:"capturedAt"`
	}{baseline.SchemaVersion, baseline.PlannerVersion, baseline.RootBindingDigest, baseline.RawCaptureDigest, baseline.ManagedSnapshotDigest, baseline.ConfigurationDigest, baseline.CapturedAt}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded)
}
