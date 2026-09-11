package runtimeapp

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
)

type runtimeSettlementPrimaryReaderV1 struct {
	ctx       context.Context
	primaries runtimeReportSemanticPrimariesV1
}

func (reader runtimeSettlementPrimaryReaderV1) AllThreadIDs() ([]string, error) {
	ids := make([]string, 0, len(reader.primaries))
	for id := range reader.primaries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, reader.ctx.Err()
}

func (reader runtimeSettlementPrimaryReaderV1) GetThread(id string) (map[string]any, error) {
	primary, err := reader.primaries.ReadPrimaryThreadSnapshotV1(reader.ctx, id)
	return primary.Thread, err
}

func runtimeSettlementPrimaryAddressV1(target string) (string, bool) {
	for _, prefix := range []string{"durable/threads/", "durable/runtime-go/threads/"} {
		if strings.HasPrefix(target, prefix) {
			parts := strings.Split(strings.TrimPrefix(target, prefix), "/")
			if len(parts) == 2 && parts[1] == "thread.json" && domainthread.IsCanonicalRecordID(parts[0]) {
				return parts[0], true
			}
		}
	}
	return "", false
}

// The managed snapshot supplies both primary families, including newly added
// identities. Authenticated journal Before may only use retained Original
// bytes; already applied After never creates historical qualification.
func observeRuntimeSettlementPrimariesV1(ctx context.Context, core *runtimeChildIdentityStartupV1, saved runtimeReportSemanticPrimariesV1, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWrite string) (original, candidate runtimeReportSemanticPrimariesV1, revalidate func(context.Context) error, resultErr error) {
	snapshot, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, core.roots)
	if err != nil {
		return nil, nil, nil, err
	}
	inventory, _, err := readRuntimeOriginalPrimaryInventoryV1(ctx, core.roots, snapshot)
	if err != nil {
		return nil, nil, nil, err
	}
	journal, err := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
	if err != nil {
		return nil, nil, nil, err
	}
	revalidate = func(ctx context.Context) error {
		current, err := persistencefs.CaptureManagedPreRecoverySnapshotV1(ctx, core.roots)
		if err == nil && !reflect.DeepEqual(snapshot, current) {
			err = errors.New("original settlement primary inventory changed during audit")
		}
		if journal != nil {
			err = errors.Join(err, journal.Revalidate(ctx))
		} else {
			currentJournal, journalErr := persistencefs.ObserveAuthenticatedSemanticJournalV1(ctx, core.roots, core.originalCreateProofV1())
			if currentJournal != nil {
				journalErr = errors.Join(journalErr, errors.New("original settlement journal appeared during audit"))
			}
			err = errors.Join(err, journalErr)
		}
		return errors.Join(err, core.revalidateKey(ctx), ctx.Err())
	}
	defer func() {
		resultErr = errors.Join(resultErr, revalidate(ctx))
		if resultErr != nil {
			original, candidate = nil, nil
		}
	}()
	original, candidate = runtimeReportSemanticPrimariesV1{}, runtimeReportSemanticPrimariesV1{}
	paths := map[string]string{}
	layout := map[string]string{}
	for _, entry := range snapshot.Entries {
		layout[entry.Path] = entry.Type
	}
	for id, entry := range inventory.entries {
		primary, err := inventory.ReadPrimaryThreadSnapshotV1(ctx, id)
		if err != nil {
			return nil, nil, revalidate, err
		}
		original[id], candidate[id], paths[id] = primary, primary, entry.primary.Path
	}
	for index, operation := range journal.OperationsV1() {
		id, owned := runtimeSettlementPrimaryAddressV1(operation.Path)
		if !owned {
			continue
		}
		if paths[id] != "" && paths[id] != operation.Path {
			return nil, nil, revalidate, errors.New("original settlement primary has conflicting families")
		}
		physical, exists := candidate[id]
		beforeMatches := operation.Before.Type == domainstartup.ManagedEntryTypeAbsent && !exists || operation.Before.Type == domainstartup.ManagedEntryTypeFile && exists && physical.ThreadFileSHA256 == operation.Before.SHA256
		if !beforeMatches {
			if index > journal.NextOperationV1() {
				return nil, nil, revalidate, errors.New("original settlement primary is outside signed prefix")
			}
			after, err := journal.PhysicallyAfterV1(ctx, operation)
			if err != nil || !after {
				return nil, nil, revalidate, errors.Join(errors.New("original settlement primary is not signed After"), err)
			}
		}
		switch operation.Before.Type {
		case domainstartup.ManagedEntryTypeAbsent:
			delete(original, id)
		case domainstartup.ManagedEntryTypeFile:
			if !beforeMatches {
				retained, found := saved[id]
				if !found || retained.ThreadFileSHA256 != operation.Before.SHA256 {
					return nil, nil, revalidate, errors.New("original settlement primary Before is unavailable")
				}
				original[id] = retained
			}
		default:
			return nil, nil, revalidate, errors.New("original settlement primary Before type is invalid")
		}
	}
	apply := func(ops []domainstartup.SemanticStartupOperationV1, read func(domainstartup.SemanticStartupOperationV1) ([]byte, error), omitted string) error {
		for _, operation := range ops {
			if omitted != "" && operation.OperationID == omitted {
				continue
			}
			layout[operation.Path] = string(operation.After.Type)
			for _, target := range paths {
				if strings.HasPrefix(target, operation.Path+"/") {
					return errors.New("settlement candidate changed a primary ancestor")
				}
			}
			id, owned := runtimeSettlementPrimaryAddressV1(operation.Path)
			if !owned {
				continue
			}
			if paths[id] != "" && paths[id] != operation.Path {
				return errors.New("settlement candidate primary has conflicting families")
			}
			paths[id] = operation.Path
			switch operation.Kind {
			case domainstartup.SemanticOperationSetMode:
				continue
			case domainstartup.SemanticOperationRemoveFile:
				delete(candidate, id)
			case domainstartup.SemanticOperationInstallFile:
				if read == nil || operation.After.Type != domainstartup.ManagedEntryTypeFile {
					return errors.New("settlement candidate primary After is unavailable")
				}
				body, err := read(operation)
				if err != nil || int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
					return errors.Join(errors.New("settlement candidate primary lost After integrity"), err)
				}
				primary, err := finalauthority.ParsePrimaryThreadSnapshotV1(ctx, id, body)
				if err != nil {
					return err
				}
				candidate[id] = primary
			default:
				return errors.New("settlement candidate primary transition is invalid")
			}
		}
		return nil
	}
	if err := apply(journal.OperationsV1(), func(op domainstartup.SemanticStartupOperationV1) ([]byte, error) { return journal.ReadAfterV1(ctx, op) }, ""); err != nil {
		return nil, nil, revalidate, err
	}
	if err := apply(operations, readAfter, noWrite); err != nil {
		return nil, nil, revalidate, err
	}
	// Project directory presence as well as file contents. An empty second
	// family or incomplete new thread must not be installed before detection.
	seen := map[string]bool{}
	for _, family := range []string{"durable/threads", "durable/runtime-go/threads"} {
		rootType := layout[family]
		if rootType != string(domainstartup.ManagedEntryTypeDirectory) && rootType != string(domainstartup.ManagedEntryTypeAbsent) {
			return nil, nil, revalidate, errors.New("settlement candidate primary family is invalid")
		}
		for target, kind := range layout {
			if kind == string(domainstartup.ManagedEntryTypeAbsent) || !strings.HasPrefix(target, family+"/") {
				continue
			}
			id := strings.TrimPrefix(target, family+"/")
			if strings.Contains(id, "/") {
				continue
			}
			if rootType != string(domainstartup.ManagedEntryTypeDirectory) || kind != string(domainstartup.ManagedEntryTypeDirectory) || !domainthread.IsCanonicalRecordID(id) || seen[id] {
				return nil, nil, revalidate, errors.New("settlement candidate primary directory denominator conflicts")
			}
			primary, found := candidate[id]
			if !found || primary.ThreadID != id || paths[id] != target+"/thread.json" || layout[target+"/thread.json"] != string(domainstartup.ManagedEntryTypeFile) {
				return nil, nil, revalidate, errors.New("settlement candidate thread directory lacks its exact primary")
			}
			seen[id] = true
		}
	}
	if len(seen) != len(candidate) {
		return nil, nil, revalidate, errors.New("settlement candidate primary lost its directory")
	}
	return original, candidate, revalidate, nil
}
