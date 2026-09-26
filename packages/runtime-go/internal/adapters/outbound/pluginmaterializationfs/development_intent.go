package pluginmaterializationfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

// PrepareDevelopmentIntentV1 retains the first request identity for an inspected
// source registration. A restart resumes the same existing transaction instead
// of fabricating a new timestamp and competing with its incomplete journal.
func (store *Store) PrepareDevelopmentIntentV1(ctx context.Context, proposed domainplugin.IntentV1) (domainplugin.IntentV1, error) {
	if store == nil || ctx == nil || ctx.Err() != nil || proposed.PluginName != store.packageID ||
		proposed.Origin != domainplugin.DevelopmentSourceOriginV1 || domainplugin.ValidateIntentV1(proposed) != nil {
		return domainplugin.IntentV1{}, pluginport.ErrInvalid
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	path := filepath.Join(store.absolute(store.indexRoot), "source-intent-"+proposed.SourceRegistrationSHA256+".v1.json")
	if body, err := stableReadFile(path, domainplugin.MaxContractBytesV1); err == nil {
		prior, err := domainplugin.ParseIntentV1(body)
		if err != nil {
			return domainplugin.IntentV1{}, pluginport.ErrCorrupt
		}
		expected := proposed
		expected.RequestedAt, expected.IntentID = prior.RequestedAt, prior.IntentID
		if expected != prior || domainplugin.ValidateIntentV1(expected) != nil {
			return domainplugin.IntentV1{}, pluginport.ErrConflict
		}
		return prior, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return domainplugin.IntentV1{}, pluginport.ErrCorrupt
	}
	body, err := domainplugin.IntentV1Bytes(proposed)
	if err != nil {
		return domainplugin.IntentV1{}, pluginport.ErrInvalid
	}
	if err := store.putExact(path, body); err != nil {
		return domainplugin.IntentV1{}, err
	}
	return proposed, nil
}
