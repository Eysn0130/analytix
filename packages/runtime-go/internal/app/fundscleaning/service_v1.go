package fundscleaning

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainlocaldisplay "analytix.local/runtime-go/internal/domain/localdisplay"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
)

var (
	ErrInvalidRequest = errors.New("deterministic funds cleaning request is invalid")
	ErrPreCASFailed   = errors.New("deterministic funds cleaning failed before DSV2 CAS")
	ErrUnavailable    = errors.New("deterministic funds cleaning is unavailable")
)

const (
	ResultStatusCommittedV1      = "committed"
	ResultStatusOutcomeUnknownV1 = "outcome_unknown"
	activeCleaningLifetimeV1     = 10 * time.Minute
)

var cleaningBindingDomainV1 = []byte("AnalytixFundsDeterministicCleaningBindingV1\x00")

type CurrentSourceV1 interface {
	UseCurrentLocalDisplay(
		context.Context,
		string,
		string,
		string,
		func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
	) error
	ResolveCurrentLocalDisplay(
		context.Context,
		string,
		string,
		string,
	) (domainfundsquerysource.DescriptorV1, error)
}

type AdmissionV1 interface {
	CommitCleaningV1(
		context.Context,
		fundscsvadmissionapp.CleaningCommitInputV1,
	) (fundscsvadmissionapp.CleaningCommitResultV1, error)
}

type ConfigV1 struct {
	Identity  identityport.Authority
	Source    CurrentSourceV1
	Native    nativecomponentport.DeterministicCleaningRunner
	Admission AdmissionV1
	Now       func() time.Time
}

type ServiceV1 struct {
	identity  identityport.Authority
	source    CurrentSourceV1
	native    nativecomponentport.DeterministicCleaningRunner
	admission AdmissionV1
	now       func() time.Time

	mu        sync.Mutex
	epoch     uint64
	runCancel context.CancelFunc
	active    *activeCleaningV1
	timer     *time.Timer
}

type RunInputV1 struct {
	WorkspaceRoot string
}

type RunResultV1 struct {
	Status           string `json:"status"`
	Selector         string `json:"selector,omitempty"`
	RuleGeneration   string `json:"ruleGeneration,omitempty"`
	RuleDigest       string `json:"ruleDigest,omitempty"`
	InputSnapshot    string `json:"inputSnapshot,omitempty"`
	OutputSnapshot   string `json:"outputSnapshot,omitempty"`
	TransformLineage string `json:"transformLineage,omitempty"`
	RowCount         uint64 `json:"rowCount"`
	ChangedRowCount  uint64 `json:"changedRowCount"`
}

type exactCleaningCellV1 struct {
	field  string
	before string
	after  string
}

type exactCleaningRowV1 struct {
	status string
	cells  []exactCleaningCellV1
}

type activeCleaningV1 struct {
	epoch           uint64
	workspace       string
	principal       domainidentity.PrincipalV1
	caseBindingHash string
	outputDSV2      string
	authority       domainlocaldisplay.CleaningDiffAuthorityV1
	expiresAt       time.Time
	rows            []exactCleaningRowV1
}

func NewServiceV1(config ConfigV1) (*ServiceV1, error) {
	if config.Identity == nil || config.Source == nil || config.Native == nil || config.Admission == nil {
		return nil, ErrUnavailable
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &ServiceV1{
		identity: config.Identity, source: config.Source, native: config.Native,
		admission: config.Admission, now: config.Now,
	}, nil
}

func (service *ServiceV1) RunV1(
	ctx context.Context,
	input RunInputV1,
) (RunResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil || input.WorkspaceRoot == "" {
		return RunResultV1{}, ErrInvalidRequest
	}
	principal, err := service.identity.ResolveCurrent(ctx)
	if err != nil || domainidentity.ValidatePrincipalV1(principal) != nil ||
		service.identity.ValidateCurrent(ctx, principal) != nil {
		return RunResultV1{}, errors.Join(ErrPreCASFailed, ErrUnavailable, err)
	}
	service.mu.Lock()
	service.epoch++
	epoch := service.epoch
	if service.runCancel != nil {
		service.runCancel()
	}
	service.clearActiveLockedV1()
	runContext, runCancel := context.WithCancel(ctx)
	service.runCancel = runCancel
	service.mu.Unlock()
	defer func() {
		runCancel()
		service.mu.Lock()
		if service.epoch == epoch {
			service.runCancel = nil
		}
		service.mu.Unlock()
	}()

	var descriptor domainfundsquerysource.DescriptorV1
	var nativeResult domainnative.DeterministicCleaningResultV1
	var outputBody []byte
	err = service.source.UseCurrentLocalDisplay(
		runContext, input.WorkspaceRoot, domainsecurity.LocalTenantID, domainsecurity.LocalUserID,
		func(
			sourceContext context.Context,
			current domainfundsquerysource.DescriptorV1,
			lease fundsquerysourceport.ExactReadLease,
		) error {
			arguments, argumentErr := domainnative.NewDeterministicCleaningArgumentsV1(current)
			if argumentErr != nil {
				return argumentErr
			}
			result, body, runErr := service.native.DeterministicCleaning(
				sourceContext, arguments, current, lease,
			)
			if runErr != nil {
				clear(body)
				return runErr
			}
			descriptor = current
			nativeResult = result
			outputBody = body
			return nil
		},
	)
	if err != nil {
		clear(outputBody)
		return RunResultV1{}, errors.Join(ErrPreCASFailed, ErrUnavailable, err)
	}
	defer clear(outputBody)
	if service.identity.ValidateCurrent(runContext, principal) != nil ||
		domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil ||
		nativeResult.InputDatasetSnapshotID != descriptor.DatasetSnapshotID ||
		nativeResult.OutputArtifactSHA256 != domainsecurity.SHA256Hex(outputBody) ||
		nativeResult.OutputArtifactByteLength != uint64(len(outputBody)) {
		return RunResultV1{}, errors.Join(ErrPreCASFailed, ErrUnavailable)
	}
	rows, err := exactRowsFromNativeV1(outputBody, nativeResult)
	if err != nil {
		return RunResultV1{}, errors.Join(ErrPreCASFailed, ErrUnavailable, err)
	}
	defer clearExactRowsV1(rows)
	preLineageDigest := framedDigestV1(
		"pre-lineage", descriptor.DatasetSnapshotID, nativeResult.RuleGeneration,
		nativeResult.RuleDigest, nativeResult.OutputArtifactSHA256,
		fmt.Sprintf("%d", nativeResult.OutputArtifactByteLength), fmt.Sprintf("%d", nativeResult.RowCount),
		nativeResult.ResultDigest,
	)
	commit, err := service.admission.CommitCleaningV1(
		runContext,
		fundscsvadmissionapp.CleaningCommitInputV1{
			WorkspaceRoot: input.WorkspaceRoot, ExpectedInputSnapshotID: descriptor.DatasetSnapshotID,
			SourceBody: outputBody, SourceSHA256: nativeResult.OutputArtifactSHA256,
			SourceRowCount: nativeResult.RowCount, AcquisitionActorDigest: preLineageDigest,
			IntentNonceDigest: framedDigestV1("intent", descriptor.DatasetSnapshotID, nativeResult.RuleDigest),
		},
	)
	if err != nil {
		return RunResultV1{
			Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
			ChangedRowCount: nativeResult.ChangedRowCount,
		}, nil
	}
	if commit.Status == fundscsvadmissionapp.CleaningCommitStatusPreCASFailedV1 {
		if commit.InputDatasetSnapshotID != descriptor.DatasetSnapshotID {
			return RunResultV1{
				Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
				ChangedRowCount: nativeResult.ChangedRowCount,
			}, nil
		}
		return RunResultV1{}, errors.Join(ErrPreCASFailed, ErrUnavailable)
	}
	if commit.Status == fundscsvadmissionapp.CleaningCommitStatusOutcomeUnknownV1 {
		return RunResultV1{
			Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
			ChangedRowCount: nativeResult.ChangedRowCount,
		}, nil
	}
	if commit.Status != fundscsvadmissionapp.CleaningCommitStatusCommittedV1 ||
		commit.InputDatasetSnapshotID != descriptor.DatasetSnapshotID ||
		!domainsecurity.IsDatasetSnapshotIDV2Syntax(commit.OutputDatasetSnapshotID) {
		return RunResultV1{
			Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
			ChangedRowCount: nativeResult.ChangedRowCount,
		}, nil
	}
	current, err := service.source.ResolveCurrentLocalDisplay(
		runContext, input.WorkspaceRoot, domainsecurity.LocalTenantID, domainsecurity.LocalUserID,
	)
	if err != nil || current.DatasetSnapshotID != commit.OutputDatasetSnapshotID ||
		current.CaseBindingHash != descriptor.CaseBindingHash ||
		service.identity.ValidateCurrent(runContext, principal) != nil {
		return RunResultV1{
			Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
			ChangedRowCount: nativeResult.ChangedRowCount,
		}, nil
	}
	authority := cleaningAuthorityV1(principal, descriptor, commit.OutputDatasetSnapshotID, nativeResult)
	if domainlocaldisplay.ValidateCleaningDiffAuthorityV1(authority) != nil {
		return RunResultV1{
			Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
			ChangedRowCount: nativeResult.ChangedRowCount,
		}, nil
	}
	active := &activeCleaningV1{
		epoch: epoch, workspace: input.WorkspaceRoot, principal: principal,
		caseBindingHash: descriptor.CaseBindingHash, outputDSV2: commit.OutputDatasetSnapshotID,
		authority: authority, expiresAt: service.now().UTC().Add(activeCleaningLifetimeV1),
		rows: cloneExactRowsV1(rows),
	}
	service.mu.Lock()
	if service.epoch != epoch || runContext.Err() != nil || service.identity.ValidateCurrent(runContext, principal) != nil {
		service.mu.Unlock()
		clearActiveCleaningV1(active)
		return RunResultV1{
			Status: ResultStatusOutcomeUnknownV1, RowCount: nativeResult.RowCount,
			ChangedRowCount: nativeResult.ChangedRowCount,
		}, nil
	}
	service.active = active
	service.timer = time.AfterFunc(activeCleaningLifetimeV1, func() {
		service.mu.Lock()
		defer service.mu.Unlock()
		if service.active != nil && service.active.epoch == epoch {
			service.clearActiveLockedV1()
		}
	})
	service.mu.Unlock()
	return RunResultV1{
		Status: ResultStatusCommittedV1, Selector: authority.Selector,
		RuleGeneration: authority.RuleGeneration, RuleDigest: authority.RuleDigest,
		InputSnapshot: authority.InputSnapshot, OutputSnapshot: authority.OutputSnapshot,
		TransformLineage: authority.TransformLineage, RowCount: nativeResult.RowCount,
		ChangedRowCount: nativeResult.ChangedRowCount,
	}, nil
}

func (service *ServiceV1) UseCurrentCleaningDiffPreviewV1(
	ctx context.Context,
	principal domainidentity.PrincipalV1,
	selector string,
	fields []string,
	rowOffset uint32,
	rowLimit uint16,
	use func(context.Context, domainlocaldisplay.CleaningDiffAuthorityV1, []domainlocaldisplay.CleaningDiffRowV1, bool) error,
) error {
	if service == nil || ctx == nil || use == nil ||
		domainidentity.ValidatePrincipalV1(principal) != nil || !domainlocaldisplay.ValidSelectorV1(selector) ||
		domainlocaldisplay.ValidateFieldsV1(fields, domainlocaldisplay.CleaningDiffPreviewFieldsV1()) != nil ||
		rowLimit == 0 || rowLimit > domainlocaldisplay.MaximumRowLimitV1 {
		return ErrInvalidRequest
	}
	service.mu.Lock()
	active, err := service.currentActiveLockedV1(principal, selector)
	if err != nil {
		service.mu.Unlock()
		return err
	}
	authority := active.authority
	workspace := active.workspace
	outputDSV2 := active.outputDSV2
	caseBindingHash := active.caseBindingHash
	epoch := active.epoch
	start := uint64(rowOffset)
	if start > uint64(len(active.rows)) {
		start = uint64(len(active.rows))
	}
	end := start + uint64(rowLimit)
	if end > uint64(len(active.rows)) {
		end = uint64(len(active.rows))
	}
	rows, projectErr := projectRowsV1(active.rows[start:end], rowOffset, fields)
	if projectErr != nil {
		service.clearActiveLockedV1()
		service.mu.Unlock()
		return ErrUnavailable
	}
	hasMore := end < uint64(len(active.rows))
	service.mu.Unlock()
	defer clearDomainRowsV1(rows)
	current, err := service.source.ResolveCurrentLocalDisplay(
		ctx, workspace, domainsecurity.LocalTenantID, domainsecurity.LocalUserID,
	)
	if err != nil || current.DatasetSnapshotID != outputDSV2 || current.CaseBindingHash != caseBindingHash ||
		service.identity.ValidateCurrent(ctx, principal) != nil {
		service.revokeEpochV1(epoch)
		return ErrUnavailable
	}
	if err := use(ctx, authority, rows, hasMore); err != nil {
		return err
	}
	if service.ValidateCurrentCleaningDiffPreviewV1(ctx, principal, authority) != nil {
		return ErrUnavailable
	}
	return nil
}

func (service *ServiceV1) ValidateCurrentCleaningDiffPreviewV1(
	ctx context.Context,
	principal domainidentity.PrincipalV1,
	authority domainlocaldisplay.CleaningDiffAuthorityV1,
) error {
	if service == nil || ctx == nil || domainlocaldisplay.ValidateCleaningDiffAuthorityV1(authority) != nil ||
		domainidentity.ValidatePrincipalV1(principal) != nil || service.identity.ValidateCurrent(ctx, principal) != nil {
		return ErrUnavailable
	}
	service.mu.Lock()
	active, err := service.currentActiveLockedV1(principal, authority.Selector)
	if err != nil || active.authority != authority {
		service.mu.Unlock()
		return ErrUnavailable
	}
	workspace := active.workspace
	outputDSV2 := active.outputDSV2
	caseBindingHash := active.caseBindingHash
	epoch := active.epoch
	service.mu.Unlock()
	current, err := service.source.ResolveCurrentLocalDisplay(
		ctx, workspace, domainsecurity.LocalTenantID, domainsecurity.LocalUserID,
	)
	if err != nil || current.DatasetSnapshotID != outputDSV2 || current.CaseBindingHash != caseBindingHash {
		service.revokeEpochV1(epoch)
		return ErrUnavailable
	}
	return nil
}

func (service *ServiceV1) RevokeCleaningDiffPreviewV1(ctx context.Context, selector string) error {
	if service == nil || ctx == nil || ctx.Err() != nil || !domainlocaldisplay.ValidSelectorV1(selector) {
		return ErrUnavailable
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	active := service.active
	if active == nil || active.authority.Selector != selector {
		return ErrUnavailable
	}
	service.clearActiveLockedV1()
	return nil
}

func (service *ServiceV1) Close() error {
	if service == nil {
		return nil
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.epoch++
	if service.runCancel != nil {
		service.runCancel()
		service.runCancel = nil
	}
	service.clearActiveLockedV1()
	return nil
}

func cleaningAuthorityV1(
	principal domainidentity.PrincipalV1,
	input domainfundsquerysource.DescriptorV1,
	outputDSV2 string,
	result domainnative.DeterministicCleaningResultV1,
) domainlocaldisplay.CleaningDiffAuthorityV1 {
	inputSnapshot := "tlsnap1_" + framedDigestV1("input-snapshot", input.DatasetSnapshotID)
	outputSnapshot := "tlsnap1_" + framedDigestV1(
		"output-snapshot", outputDSV2, result.OutputArtifactSHA256, fmt.Sprintf("%d", result.OutputArtifactByteLength),
	)
	lineage := "tllin1_" + framedDigestV1(
		"transform-lineage", input.DatasetSnapshotID, outputDSV2, inputSnapshot, outputSnapshot,
		result.RuleGeneration, result.RuleDigest, result.ResultDigest,
	)
	selector := "tlsel1_" + framedDigestV1(
		"selector", lineage, principal.PrincipalDigest, input.CaseBindingHash,
	)
	return domainlocaldisplay.CleaningDiffAuthorityV1{
		Selector: selector, InputSnapshot: inputSnapshot, RuleGeneration: result.RuleGeneration,
		RuleDigest: result.RuleDigest, OutputSnapshot: outputSnapshot, TransformLineage: lineage,
	}
}

func exactRowsFromNativeV1(
	body []byte,
	result domainnative.DeterministicCleaningResultV1,
) ([]exactCleaningRowV1, error) {
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	header, err := reader.Read()
	columns := domainevidence.FundsTransactionCSVColumnsV1()
	if err != nil || len(header) != len(columns) {
		return nil, ErrUnavailable
	}
	for index := range header {
		if header[index] != columns[index].Header {
			return nil, ErrUnavailable
		}
	}
	indexes := map[string]int{
		"transactionTime": 4, "account": 1, "card": 0, "accountName": 2,
		"identityNumber": 3, "amountText": 5, "direction": 7, "counterpartyAccount": 8,
		"counterpartyName": 10, "counterpartyIdentityNumber": 11, "counterpartyBank": 12,
		"summary": 13, "currency": 14, "merchantName": 29, "remark": 31,
	}
	fields := domainlocaldisplay.CleaningDiffPreviewFieldsV1()
	rows := make([]exactCleaningRowV1, 0, result.RowCount)
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || len(record) != len(columns) || uint64(len(rows)) == result.RowCount {
			clearExactRowsV1(rows)
			return nil, ErrUnavailable
		}
		row := exactCleaningRowV1{status: "unchanged", cells: make([]exactCleaningCellV1, len(fields))}
		for fieldIndex, field := range fields {
			value := record[indexes[field]]
			row.cells[fieldIndex] = exactCleaningCellV1{field: field, before: value, after: value}
		}
		rows = append(rows, row)
	}
	if uint64(len(rows)) != result.RowCount {
		clearExactRowsV1(rows)
		return nil, ErrUnavailable
	}
	for _, changed := range result.ChangedRows {
		if uint64(changed.RowIndex) >= uint64(len(rows)) {
			clearExactRowsV1(rows)
			return nil, ErrUnavailable
		}
		row := &rows[changed.RowIndex]
		row.status = changed.Status
		for _, changedCell := range changed.Cells {
			found := false
			if err := changedCell.UseExactV1(func(
				field domainnative.DirectSourcePreviewFieldV1,
				beforeValue, afterValue, _, _ string,
			) error {
				for cellIndex := range row.cells {
					if row.cells[cellIndex].field == string(field) {
						if row.cells[cellIndex].after != afterValue {
							return ErrUnavailable
						}
						row.cells[cellIndex].before = beforeValue
						found = true
						return nil
					}
				}
				return ErrUnavailable
			}); err != nil || !found {
				clearExactRowsV1(rows)
				return nil, ErrUnavailable
			}
		}
	}
	return rows, nil
}

func projectRowsV1(
	rows []exactCleaningRowV1,
	rowOffset uint32,
	fields []string,
) ([]domainlocaldisplay.CleaningDiffRowV1, error) {
	result := make([]domainlocaldisplay.CleaningDiffRowV1, len(rows))
	for index, row := range rows {
		projected := domainlocaldisplay.CleaningDiffRowV1{
			RowIndex: rowOffset + uint32(index), Status: row.status,
			Cells: make([]domainlocaldisplay.CleaningDiffCellV1, len(fields)),
		}
		for fieldIndex, field := range fields {
			for _, cell := range row.cells {
				if cell.field != field {
					continue
				}
				before, beforeErr := domainlocaldisplay.NewExactValueV1(cell.before)
				after, afterErr := domainlocaldisplay.NewExactValueV1(cell.after)
				if beforeErr != nil || afterErr != nil {
					clearDomainRowsV1(result)
					return nil, ErrUnavailable
				}
				projected.Cells[fieldIndex] = domainlocaldisplay.CleaningDiffCellV1{
					Field: field, Before: before, After: after,
				}
				break
			}
		}
		result[index] = projected
	}
	return result, nil
}

func (service *ServiceV1) currentActiveLockedV1(
	principal domainidentity.PrincipalV1,
	selector string,
) (*activeCleaningV1, error) {
	active := service.active
	if active == nil || active.authority.Selector != selector {
		return nil, ErrUnavailable
	}
	if !domainidentity.SamePrincipalV1(active.principal, principal) || !service.now().UTC().Before(active.expiresAt) {
		service.clearActiveLockedV1()
		return nil, ErrUnavailable
	}
	return active, nil
}

func (service *ServiceV1) revokeEpochV1(epoch uint64) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.active != nil && service.active.epoch == epoch {
		service.clearActiveLockedV1()
	}
}

func (service *ServiceV1) clearActiveLockedV1() {
	if service.timer != nil {
		service.timer.Stop()
		service.timer = nil
	}
	clearActiveCleaningV1(service.active)
	service.active = nil
}

func clearActiveCleaningV1(active *activeCleaningV1) {
	if active == nil {
		return
	}
	clearExactRowsV1(active.rows)
	active.rows = nil
}

func cloneExactRowsV1(rows []exactCleaningRowV1) []exactCleaningRowV1 {
	cloned := make([]exactCleaningRowV1, len(rows))
	for index := range rows {
		cloned[index].status = rows[index].status
		cloned[index].cells = append([]exactCleaningCellV1(nil), rows[index].cells...)
	}
	return cloned
}

func clearExactRowsV1(rows []exactCleaningRowV1) {
	for rowIndex := range rows {
		for cellIndex := range rows[rowIndex].cells {
			rows[rowIndex].cells[cellIndex].before = ""
			rows[rowIndex].cells[cellIndex].after = ""
		}
		rows[rowIndex].cells = nil
	}
}

func clearDomainRowsV1(rows []domainlocaldisplay.CleaningDiffRowV1) {
	for index := range rows {
		rows[index].Cells = nil
	}
}

func framedDigestV1(values ...string) string {
	hasher := sha256.New()
	_, _ = hasher.Write(cleaningBindingDomainV1)
	for _, value := range values {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(value))))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
