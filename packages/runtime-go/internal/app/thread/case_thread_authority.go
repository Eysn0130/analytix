package thread

import (
	"context"
	"errors"
	"strings"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type AuthorityThreadReader interface {
	AllThreadIDs() ([]string, error)
	GetThread(string) (map[string]any, error)
}

type caseThreadAuthorityReader struct {
	reader         AuthorityThreadReader
	authority      CaseThreadAuthority
	primary        any
	restartContext context.Context
}

func NewCaseThreadAuthorityReader(reader AuthorityThreadReader, authority CaseThreadAuthority) AuthorityThreadReader {
	return &caseThreadAuthorityReader{reader: reader, authority: authority, primary: reader}
}

// NewCaseThreadRestartAuthorityReaderV1 permits exact original held compaction
// proofs for startup inventory validation. It must not supply executable or
// provider-history authority; those consumers retain the ordinary reader.
func NewCaseThreadRestartAuthorityReaderV1(ctx context.Context, reader AuthorityThreadReader, authority CaseThreadAuthority) AuthorityThreadReader {
	return &caseThreadAuthorityReader{reader: reader, authority: authority, primary: reader, restartContext: ctx}
}

type originalCompactionContextReaderV1 struct {
	reader  caseCompactionCommittedContextReaderV1
	observe func(context.Context, domainsecurity.TurnSecurityContext) error
	ctx     context.Context
	err     error
}

func (reader *originalCompactionContextReaderV1) CommittedContext(threadID, turnID string) (casethreadapp.CommittedContext, bool) {
	if reader.err != nil {
		return casethreadapp.CommittedContext{}, false
	}
	committed, found := reader.reader.CommittedContext(threadID, turnID)
	if found {
		reader.err = reader.observe(reader.ctx, committed.SecurityContext)
	}
	return committed, found && reader.err == nil
}

func (reader *caseThreadAuthorityReader) AllThreadIDs() ([]string, error) {
	if reader == nil || reader.reader == nil {
		return nil, errors.New("case thread authority reader is unavailable")
	}
	return reader.reader.AllThreadIDs()
}

func (reader *caseThreadAuthorityReader) GetThread(threadID string) (map[string]any, error) {
	if reader == nil || reader.reader == nil {
		return nil, errors.New("case thread authority reader is unavailable")
	}
	thread, err := reader.reader.GetThread(threadID)
	if err != nil || thread == nil {
		return thread, err
	}
	return CaseThreadAuthorityView(reader.authority, threadID, thread)
}

// ValidateCaseCompactionAuthorityTurnV1 binds the narrow metadata-only
// terminal exception to the existing installation-signed committed-context
// inventory. Durable JSON consistency alone is never sufficient authority.
func (reader *caseThreadAuthorityReader) ValidateCaseCompactionAuthorityTurnV1(
	threadID string,
	thread map[string]any,
	turn map[string]any,
) (resultErr error) {
	threadID = strings.TrimSpace(threadID)
	if reader == nil || reader.authority == nil || thread == nil || turn == nil ||
		threadID == "" || !reader.authority.IsCaseThread(threadID) {
		return errors.New("case compaction committed authority is unavailable")
	}
	committedReader, ok := reader.authority.(caseCompactionCommittedContextReaderV1)
	if !ok {
		return errors.New("case compaction committed authority is unavailable")
	}
	historical := reader.restartContext != nil && reader.authority.RestartPreservesThreadV1(threadID)
	if historical {
		observer, ok := reader.authority.(interface {
			ObserveOriginalContextForRestartV1(context.Context, domainsecurity.TurnSecurityContext) error
		})
		if !ok {
			return errors.New("case compaction original context observer is unavailable")
		}
		original := &originalCompactionContextReaderV1{reader: committedReader, observe: observer.ObserveOriginalContextForRestartV1, ctx: reader.restartContext}
		committedReader = original
		defer func() { resultErr = errors.Join(resultErr, original.err) }()
	}
	turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	target, found := committedReader.CommittedContext(threadID, turnID)
	at, atErr := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
	if err != nil || atErr != nil || !found || target.SecurityContext != frozen ||
		!historical && !reader.authority.ContainsContext(frozen) {
		return errors.New("case compaction target is not installation-signed committed authority")
	}
	var inherited []domainsecurity.ActiveInheritedTurnV1
	if !historical {
		ctx := reader.restartContext
		if ctx == nil {
			ctx = context.Background()
		}
		record, err := ActiveInheritedCompactionRecordV1(ctx, thread, reader.authority, reader.primary)
		if err != nil {
			return err
		}
		if record != nil {
			inherited = record.ActiveInheritedHistory.Turns
		}
	}
	if err := appturn.ValidateCaseCompactionAuthorityTurnV1(threadID, thread, turn, target.EpochState, inherited); err != nil {
		return err
	}
	return validateDurableCommittedCaseCompactionTarget(committedReader, thread, target, at.UTC())
}

func (reader *caseThreadAuthorityReader) CommittedContext(
	threadID string,
	turnID string,
) (casethreadapp.CommittedContext, bool) {
	if reader == nil || reader.authority == nil {
		return casethreadapp.CommittedContext{}, false
	}
	committedReader, ok := reader.authority.(caseCompactionCommittedContextReaderV1)
	if !ok {
		return casethreadapp.CommittedContext{}, false
	}
	return committedReader.CommittedContext(strings.TrimSpace(threadID), strings.TrimSpace(turnID))
}

// TrustedCaseCompactionTurnIDsV1 returns only installation-signed compaction
// targets whose continuation, operation binding, epoch snapshot, and durable
// ancestry still validate against the current committed case authority. The
// returned identities may be used as provider-history cut points; raw durable
// marker shape alone is never sufficient.
func TrustedCaseCompactionTurnIDsV1(
	thread map[string]any,
	authority CaseThreadAuthority,
	primary ...any,
) map[string]bool {
	trusted := map[string]bool{}
	threadID := strings.TrimSpace(contracts.StringField(thread, "id"))
	turns, turnsOK := thread["turns"].([]any)
	if thread == nil || authority == nil || threadID == "" || !turnsOK || len(turns) == 0 ||
		!authority.IsCaseThread(threadID) {
		return trusted
	}
	reader := &caseThreadAuthorityReader{authority: authority}
	if len(primary) > 1 {
		return trusted
	}
	if len(primary) == 1 {
		reader.primary = primary[0]
	}
	for _, raw := range turns {
		turn, _ := raw.(map[string]any)
		turnID := strings.TrimSpace(contracts.StringField(turn, "id"))
		if turnID == "" || strings.TrimSpace(contracts.StringField(turn, "caseHistoryProjection")) != "compaction_authority_v1" {
			continue
		}
		if reader.ValidateCaseCompactionAuthorityTurnV1(threadID, thread, turn) == nil {
			trusted[turnID] = true
		}
	}
	return trusted
}

func CaseThreadAuthorityView(authority CaseThreadAuthority, threadID string, thread map[string]any) (map[string]any, error) {
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(thread)
	knownCaseThread := authority != nil && authority.IsCaseThread(strings.TrimSpace(threadID))
	if err != nil && knownCaseThread {
		view := contracts.CloneMap(thread)
		view["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
		return view, nil
	}
	if err != nil || caseSensitive || !knownCaseThread {
		return thread, err
	}
	view := contracts.CloneMap(thread)
	view["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
	return view, nil
}
