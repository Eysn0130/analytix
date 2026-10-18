package thread

import (
	"encoding/json"
	"errors"
	"strings"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var (
	ErrThreadMutationBaselineConflict = errors.New("durable thread changed before mutation commit")
	ErrThreadMutationTransition       = errors.New("thread mutation security transition is unavailable")
	ErrCaseWorkspaceSignedRebind      = errors.New("case workspace change requires signed host rebind authority")
	ErrCaseRewindSignedArchive        = errors.New("case rewind requires a signed authority archive")
	ErrCaseDeleteSignedTombstone      = errors.New("case delete requires a signed authority tombstone")
)

func MutationBaselineDigest(thread map[string]any) (string, error) {
	body, err := json.Marshal(thread)
	if err != nil || len(body) == 0 {
		return "", errors.New("durable thread mutation baseline is not canonical")
	}
	if digest := domainsecurity.CanonicalJSONHash(body); digest != "" {
		return digest, nil
	}
	// Execution-grant limits are not accumulated history limits. Decode the
	// complete original value with the same exact-number canonical semantics,
	// then apply the existing independent record budgets to its JSON scalars.
	// Checking the decoded value also covers bytes, structs and custom JSON
	// marshalers that become strings or containers at the durable boundary.
	value, err := domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{
		MaxBytes: len(body), MaxStringBytes: domainevent.MaxQuestionReplyRecordBytesV1,
	})
	if err != nil {
		return "", errors.Join(errors.New("durable thread mutation baseline is not canonical"), err)
	}
	if err := validateMutationBaselineRecordsV1(value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) == 0 {
		return "", errors.New("durable thread mutation baseline is not canonical")
	}
	// Resource views and parsed archives are validation inputs only. Every
	// original answer, identity, workspace and private archive remains bound.
	return domainsecurity.SHA256Hex(canonical), nil
}

func validateMutationBaselineRecordsV1(thread map[string]any) error {
	root := make(map[string]any, len(thread))
	for key, value := range thread {
		if key != "turns" && key != domainturnterminal.GeneralTerminalPublicationArchiveFieldV1 {
			root[key] = value
		}
	}
	if err := domainevent.ValidateQuestionReplyResourceLimitsV1(root); err != nil {
		return err
	}
	if rawTurns, present := thread["turns"]; present {
		turns, ok := rawTurns.([]any)
		if !ok {
			return errors.New("durable thread mutation turns are invalid")
		}
		for _, rawTurn := range turns {
			turn, ok := rawTurn.(map[string]any)
			if !ok || turn == nil {
				return errors.New("durable thread mutation turn is invalid")
			}
			if err := domainevent.ValidateQuestionReplyResourceLimitsV1(turn); err != nil {
				return err
			}
		}
	}
	if rawArchive, present := thread[domainturnterminal.GeneralTerminalPublicationArchiveFieldV1]; present {
		archive, ok := rawArchive.(map[string]any)
		if !ok || archive == nil {
			return errors.New("durable thread mutation archive is invalid")
		}
		header := make(map[string]any, len(archive))
		for key, value := range archive {
			if key != "commits" {
				header[key] = value
			}
		}
		if err := domainevent.ValidateQuestionReplyResourceLimitsV1(header); err != nil {
			return err
		}
		commits, ok := archive["commits"].([]any)
		if !ok {
			return errors.New("durable thread mutation archive commits are invalid")
		}
		for _, rawCommit := range commits {
			commit, ok := rawCommit.(map[string]any)
			if !ok || commit == nil {
				return errors.New("durable thread mutation archive commit is invalid")
			}
			if err := domainevent.ValidateQuestionReplyResourceLimitsV1(commit); err != nil {
				return err
			}
		}
		// Typed parsing can ignore a case-insensitive field alias. Budget all
		// original fields first, then retain the existing archive validation.
		if _, err := domainturnterminal.ParseGeneralTerminalPublicationArchiveV1(rawArchive); err != nil {
			return err
		}
	}
	return nil
}

func ValidateMutationBaseline(thread map[string]any, threadID, expectedDigest string) error {
	currentDigest, err := MutationBaselineDigest(thread)
	if err != nil || !domainsecurity.IsSHA256Hex(expectedDigest) || currentDigest != expectedDigest ||
		strings.TrimSpace(stringField(thread, "id")) != strings.TrimSpace(threadID) {
		return ErrThreadMutationBaselineConflict
	}
	return nil
}

func ThreadIsCaseBound(authority CaseThreadAuthority, threadID string, thread map[string]any) bool {
	if authority != nil && authority.IsCaseThread(strings.TrimSpace(threadID)) {
		return true
	}
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	return err != nil || caseSensitive
}
