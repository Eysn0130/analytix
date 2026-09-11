package persistencefs

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"context"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

type StartupSnapshotReader struct {
	roots           RootSet
	originalCreates privatecasport.OriginalCreateResiduesV1
}

func NewStartupSnapshotReader(roots RootSet) StartupSnapshotReader {
	return StartupSnapshotReader{roots: roots}
}

func (reader StartupSnapshotReader) CaptureManagedSnapshotV1(ctx context.Context) (domainstartup.ManagedSnapshotV1, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return domainstartup.ManagedSnapshotV1{}, err
		}
	}
	raw, err := CaptureStrictWithOriginalCreateResiduesV1(ctx, reader.roots, reader.originalCreates)
	if err != nil {
		return domainstartup.ManagedSnapshotV1{}, err
	}
	entries := make([]domainstartup.ManagedEntryStateV1, 0, len(raw.Entries))
	for _, entry := range raw.Entries {
		entries = append(entries, domainstartup.ManagedEntryStateV1{
			Path: entry.Path, Type: entry.Type, Mode: entry.Mode, Size: entry.Size, ModTimeUnixNano: entry.ModTimeUnixNano,
			SHA256: entry.SHA256, RecordCount: entry.RecordCount,
		})
	}
	return domainstartup.NewManagedSnapshotV1(CanonicalRoots(reader.roots), raw.SHA256, entries)
}

func NewStartupSnapshotReaderWithOriginalCreateResiduesV1(roots RootSet, proof privatecasport.OriginalCreateResiduesV1) StartupSnapshotReader {
	return StartupSnapshotReader{roots: roots, originalCreates: proof}
}
