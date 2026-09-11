package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	recoveryport "analytix.local/runtime-go/internal/ports/generalterminalrecovery"
)

const maxAcceptedFinalCASFileBytes = 128 << 20

type AcceptedFinalCASReader struct {
	root acceptedFinalCASRootAuthority
}

func NewAcceptedFinalCASReader(durableRoot string) (*AcceptedFinalCASReader, error) {
	durableRoot = strings.TrimSpace(durableRoot)
	if durableRoot == "" {
		return nil, errors.New("accepted final CAS durable root is required")
	}
	root, err := captureAcceptedFinalCASRoot(durableRoot)
	if err != nil {
		return nil, err
	}
	return &AcceptedFinalCASReader{root: root}, nil
}

// ReadPrimaryThreadSnapshotV1 retains the exact bytes' digest and all original
// fields. The pinned root and stable read are shared with accepted-final CAS;
// no metadata/messages projection participates in this observation.
func (reader *AcceptedFinalCASReader) ReadPrimaryThreadSnapshotV1(ctx context.Context, threadID string) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if reader == nil || ctx == nil || !domainthread.IsCanonicalRecordID(threadID) {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("primary thread snapshot identity is invalid")
	}
	if err := ctx.Err(); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	body, err := reader.root.readPrimaryThreadJSON(threadID)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	return ParsePrimaryThreadSnapshotV1(ctx, threadID, body)
}

// ParsePrimaryThreadSnapshotV1 shares the strict primary owner for an exact
// authenticated semantic After. Parsed values do not establish disk authority.
func ParsePrimaryThreadSnapshotV1(ctx context.Context, threadID string, body []byte) (recoveryport.PrimaryThreadSnapshotV1, error) {
	if ctx == nil || !domainthread.IsCanonicalRecordID(threadID) {
		return recoveryport.PrimaryThreadSnapshotV1{}, errors.New("primary thread snapshot identity is invalid")
	}
	thread, err := decodePrimaryThreadJSONForCAS(body)
	if err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if err := domainthread.ValidatePrimaryIdentityV1(threadID, thread); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return recoveryport.PrimaryThreadSnapshotV1{}, err
	}
	return recoveryport.PrimaryThreadSnapshotV1{
		ThreadID: threadID, ThreadFileSHA256: domainsecurity.SHA256Hex(body), Thread: thread,
	}, nil
}

// ReadCommittedEventLogSHA256V1 uses the same anchored, single-link stable-file
// authority as thread.json. It never opens or recovers an atomic event journal.
// The event consumer must also validate the complete log and journal absence.
func (reader *AcceptedFinalCASReader) ReadCommittedEventLogSHA256V1(ctx context.Context, threadID string) (string, error) {
	if reader == nil || ctx == nil || !domainthread.IsCanonicalRecordID(threadID) {
		return "", errors.New("Core event snapshot identity is invalid")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	body, err := reader.root.readThreadFile(threadID, "events.jsonl")
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(body), nil
}

// ReadAcceptedFinalCASObservation reads only the primary thread.json record.
// Metadata/messages sidecars are projections and can never prove a public CAS
// winner or repair an unresolved private accepted-final attempt.
func (reader *AcceptedFinalCASReader) ReadAcceptedFinalCASObservation(
	ctx context.Context,
	threadID string,
	turnID string,
) (domainevidence.AcceptedFinalCASObservationV1, error) {
	observations, err := reader.ReadAcceptedFinalCASObservations(ctx, threadID, []string{turnID})
	if err != nil {
		return domainevidence.AcceptedFinalCASObservationV1{}, err
	}
	observation, found := observations[turnID]
	if !found {
		return domainevidence.AcceptedFinalCASObservationV1{}, errors.New("accepted final CAS primary turn is missing")
	}
	return observation, nil
}

// ReadAcceptedFinalCASObservations performs one stable primary-file read and
// strict decode for the complete requested turn set. It never consults
// metadata/messages sidecars.
func (reader *AcceptedFinalCASReader) ReadAcceptedFinalCASObservations(
	ctx context.Context,
	threadID string,
	turnIDs []string,
) (map[string]domainevidence.AcceptedFinalCASObservationV1, error) {
	if reader == nil || !validCASRecordID(threadID) || len(turnIDs) == 0 {
		return nil, errors.New("accepted final CAS identity is invalid")
	}
	requested := make(map[string]bool, len(turnIDs))
	for _, turnID := range turnIDs {
		if !validCASRecordID(turnID) || requested[turnID] {
			return nil, errors.New("accepted final CAS requested turn set is invalid")
		}
		requested[turnID] = true
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	body, err := reader.root.readPrimaryThreadJSON(threadID)
	if err != nil {
		return nil, err
	}
	return ParseAcceptedFinalCASObservationsV1(ctx, threadID, turnIDs, body)
}

// ParseAcceptedFinalCASObservationsV1 applies the same complete primary/turn
// contract to authenticated in-memory After bytes without creating a store.
func ParseAcceptedFinalCASObservationsV1(ctx context.Context, threadID string, turnIDs []string, body []byte) (map[string]domainevidence.AcceptedFinalCASObservationV1, error) {
	if !validCASRecordID(threadID) || len(turnIDs) == 0 {
		return nil, errors.New("accepted final CAS identity is invalid")
	}
	requested := make(map[string]bool, len(turnIDs))
	for _, turnID := range turnIDs {
		if !validCASRecordID(turnID) || requested[turnID] {
			return nil, errors.New("accepted final CAS requested turn set is invalid")
		}
		requested[turnID] = true
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	thread, err := decodePrimaryThreadJSONForCAS(body)
	if err != nil {
		return nil, err
	}
	if err := domainthread.ValidatePrimaryIdentityV1(threadID, thread); err != nil {
		return nil, err
	}
	current, err := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
	if err != nil {
		return nil, errors.New("accepted final CAS current context is invalid")
	}
	matched := make(map[string]map[string]any, len(requested))
	seenTurnIDs := map[string]bool{}
	turns, _ := thread["turns"].([]any)
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		turnID := casString(turn["id"])
		if !validCASRecordID(turnID) {
			return nil, errors.New("accepted final CAS contains an invalid turn identity")
		}
		if seenTurnIDs[turnID] {
			return nil, errors.New("accepted final CAS contains duplicate turn identity")
		}
		seenTurnIDs[turnID] = true
		if !requested[turnID] {
			continue
		}
		matched[turnID] = turn
	}
	if len(matched) != len(requested) {
		return nil, errors.New("accepted final CAS primary turn is missing")
	}
	threadFileSHA256 := domainsecurity.SHA256Hex(body)
	observations := make(map[string]domainevidence.AcceptedFinalCASObservationV1, len(requested))
	for _, turnID := range turnIDs {
		turn := matched[turnID]
		frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
		if err != nil {
			return nil, errors.New("accepted final CAS frozen context is invalid")
		}
		turnProjectionSHA256, err := domainevidence.AcceptedFinalTurnProjectionSHA256V1(turn)
		if err != nil {
			return nil, err
		}
		observation := domainevidence.AcceptedFinalCASObservationV1{
			ThreadID:             threadID,
			TurnID:               turnID,
			Status:               casString(turn["status"]),
			FrozenContext:        frozen,
			CurrentContext:       current,
			ThreadFileSHA256:     threadFileSHA256,
			TurnProjectionSHA256: turnProjectionSHA256,
		}
		if value := turn["acceptedFinal"]; value != nil {
			winner, err := domainevidence.ParseAcceptedFinalRecord(value)
			if err != nil {
				return nil, errors.New("accepted final CAS winner is invalid")
			}
			observation.HasWinner = true
			observation.Winner = winner
		}
		observation, err = domainevidence.NewAcceptedFinalCASObservationV1(observation)
		if err != nil {
			return nil, err
		}
		observations[turnID] = observation
	}
	return observations, nil
}

func decodePrimaryThreadJSONForCAS(body []byte) (map[string]any, error) {
	thread, err := domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{
		MaxBytes: maxAcceptedFinalCASFileBytes, MaxDepth: 256, MaxTokens: 8_000_000,
		MaxStringBytes: 16 << 20, MaxNumberBytes: 4096, MaxAbsExponent: 10000,
	})
	if err != nil {
		return nil, errors.New("accepted final CAS primary thread JSON is invalid")
	}
	return thread, nil
}

func acceptedFinalCASReadStableFile(file *os.File, expectedSize int64) ([]byte, error) {
	if file == nil || expectedSize <= 0 || expectedSize > maxAcceptedFinalCASFileBytes {
		return nil, errors.New("accepted final CAS primary thread size is invalid")
	}
	first, err := io.ReadAll(io.LimitReader(file, maxAcceptedFinalCASFileBytes+1))
	if err != nil || int64(len(first)) != expectedSize {
		return nil, errors.New("accepted final CAS primary thread read is incomplete")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	second, err := io.ReadAll(io.LimitReader(file, maxAcceptedFinalCASFileBytes+1))
	if err != nil || !bytes.Equal(first, second) {
		return nil, errors.New("accepted final CAS primary thread changed during read")
	}
	return first, nil
}

func validCASRecordID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && value != "." &&
		!strings.Contains(value, "..") && !strings.ContainsAny(value, "/\\:")
}

func casString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
