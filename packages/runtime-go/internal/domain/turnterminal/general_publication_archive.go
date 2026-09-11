package turnterminal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	GeneralTerminalPublicationArchiveV1SchemaVersion = "general-terminal-publication-archive.v1"
	GeneralTerminalPublicationArchiveV1Purpose       = "analytix.general-terminal-publication-archive/v1"
)

var generalTerminalPublicationArchiveDomain = []byte("analytix/general-terminal-publication-archive/v1\x00")

// GeneralTerminalPublicationArchiveV1 retains only deterministic commit and
// manifest metadata after compaction removes a canonical turn. It never stores
// assistant text and is not evidence or case-fact publication authority.
type GeneralTerminalPublicationArchiveV1 struct {
	SchemaVersion string                               `json:"schemaVersion"`
	Purpose       string                               `json:"purpose"`
	Commits       []GeneralTerminalPublicationCommitV1 `json:"commits"`
	ArchiveDigest string                               `json:"archiveDigest"`
}

func NewGeneralTerminalPublicationArchiveV1(
	commits []GeneralTerminalPublicationCommitV1,
) (GeneralTerminalPublicationArchiveV1, error) {
	archive := GeneralTerminalPublicationArchiveV1{
		SchemaVersion: GeneralTerminalPublicationArchiveV1SchemaVersion,
		Purpose:       GeneralTerminalPublicationArchiveV1Purpose,
		Commits:       make([]GeneralTerminalPublicationCommitV1, 0, len(commits)),
	}
	for _, commit := range commits {
		archive.Commits = append(archive.Commits, cloneGeneralTerminalPublicationCommitV1(commit))
	}
	archive.ArchiveDigest = generalTerminalPublicationArchiveDigestV1(archive)
	if err := ValidateGeneralTerminalPublicationArchiveV1(archive); err != nil {
		return GeneralTerminalPublicationArchiveV1{}, err
	}
	return cloneGeneralTerminalPublicationArchiveV1(archive), nil
}

func AppendGeneralTerminalPublicationArchiveV1(
	archive GeneralTerminalPublicationArchiveV1,
	commit GeneralTerminalPublicationCommitV1,
) (GeneralTerminalPublicationArchiveV1, error) {
	if ValidateGeneralTerminalPublicationArchiveV1(archive) != nil || ValidateGeneralTerminalPublicationCommitV1(commit) != nil {
		return GeneralTerminalPublicationArchiveV1{}, errors.New("general terminal publication archive append is invalid")
	}
	for _, existing := range archive.Commits {
		if existing.TurnID == commit.TurnID || existing.CommitID == commit.CommitID {
			if existing.CommitDigest != commit.CommitDigest {
				return GeneralTerminalPublicationArchiveV1{}, errors.New("general terminal publication archive conflicts with its canonical commit")
			}
			return cloneGeneralTerminalPublicationArchiveV1(archive), nil
		}
	}
	archive.Commits = append(archive.Commits, cloneGeneralTerminalPublicationCommitV1(commit))
	archive.ArchiveDigest = generalTerminalPublicationArchiveDigestV1(archive)
	if err := ValidateGeneralTerminalPublicationArchiveV1(archive); err != nil {
		return GeneralTerminalPublicationArchiveV1{}, err
	}
	return cloneGeneralTerminalPublicationArchiveV1(archive), nil
}

func ValidateGeneralTerminalPublicationArchiveV1(archive GeneralTerminalPublicationArchiveV1) error {
	if archive.SchemaVersion != GeneralTerminalPublicationArchiveV1SchemaVersion ||
		archive.Purpose != GeneralTerminalPublicationArchiveV1Purpose || archive.Commits == nil ||
		!domainsecurity.IsSHA256Hex(archive.ArchiveDigest) || archive.ArchiveDigest != generalTerminalPublicationArchiveDigestV1(archive) {
		return errors.New("general terminal publication archive is invalid")
	}
	seenTurns := map[string]bool{}
	seenCommits := map[string]bool{}
	for _, commit := range archive.Commits {
		if ValidateGeneralTerminalPublicationCommitV1(commit) != nil || seenTurns[commit.TurnID] || seenCommits[commit.CommitID] {
			return errors.New("general terminal publication archive contains an invalid or duplicate commit")
		}
		seenTurns[commit.TurnID] = true
		seenCommits[commit.CommitID] = true
	}
	return nil
}

func ParseGeneralTerminalPublicationArchiveV1(value any) (GeneralTerminalPublicationArchiveV1, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return GeneralTerminalPublicationArchiveV1{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var archive GeneralTerminalPublicationArchiveV1
	if err := decoder.Decode(&archive); err != nil {
		return GeneralTerminalPublicationArchiveV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return GeneralTerminalPublicationArchiveV1{}, errors.New("general terminal publication archive contains trailing JSON")
	}
	if err := ValidateGeneralTerminalPublicationArchiveV1(archive); err != nil {
		return GeneralTerminalPublicationArchiveV1{}, err
	}
	return cloneGeneralTerminalPublicationArchiveV1(archive), nil
}

func GeneralTerminalPublicationArchiveV1Map(archive GeneralTerminalPublicationArchiveV1) map[string]any {
	body, _ := json.Marshal(archive)
	value := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&value)
	return value
}

func generalTerminalPublicationArchiveDigestV1(archive GeneralTerminalPublicationArchiveV1) string {
	archive.ArchiveDigest = ""
	body, _ := json.Marshal(archive)
	return domainSeparatedSHA256Hex(generalTerminalPublicationArchiveDomain, body)
}

func cloneGeneralTerminalPublicationArchiveV1(
	archive GeneralTerminalPublicationArchiveV1,
) GeneralTerminalPublicationArchiveV1 {
	body, _ := json.Marshal(archive)
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	out := GeneralTerminalPublicationArchiveV1{}
	_ = decoder.Decode(&out)
	return out
}
