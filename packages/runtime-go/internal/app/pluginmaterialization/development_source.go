package pluginmaterialization

import (
	"context"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

// DevelopmentSourceBindingV1 is immutable outside this package. It freezes a
// validated source registration; it is never a FormalPackageBindingV1.
// Runtime composition separately enforces source-only startup or a qualified,
// sealed private-local package. This binding alone grants neither admission.
type DevelopmentSourceBindingV1 struct {
	registration domainpackage.DevelopmentSourceRegistrationV1
	digest       string
	sourceRoot   string
	target       domainplugin.TargetV1
}

func NewDevelopmentSourceBindingV1(registration domainpackage.DevelopmentSourceRegistrationV1, sourceRoot string, target domainplugin.TargetV1) (DevelopmentSourceBindingV1, error) {
	if domainpackage.ValidateDevelopmentSourceRegistrationV1(registration) != nil || domainplugin.ValidateTargetV1(target) != nil {
		return DevelopmentSourceBindingV1{}, ErrPackageAuthority
	}
	binding := DevelopmentSourceBindingV1{registration: registration, digest: domainpackage.DevelopmentSourceRegistrationSHA256V1(registration), sourceRoot: sourceRoot, target: target}
	if _, err := binding.NewIntentV1(time.Unix(0, 0).UTC()); err != nil {
		return DevelopmentSourceBindingV1{}, ErrPackageAuthority
	}
	return binding, nil
}

func (binding DevelopmentSourceBindingV1) NewIntentV1(requestedAt time.Time) (domainplugin.IntentV1, error) {
	if binding.digest == "" || binding.digest != domainpackage.DevelopmentSourceRegistrationSHA256V1(binding.registration) || requestedAt.IsZero() {
		return domainplugin.IntentV1{}, ErrPackageAuthority
	}
	registration := binding.registration
	return domainplugin.NewIntentV1(domainplugin.IntentInputV1{
		Origin: domainplugin.DevelopmentSourceOriginV1, SourceRegistrationSHA256: binding.digest,
		Target: binding.target, PluginName: registration.Identity.PackageID, PluginVersion: registration.Identity.PackageVersion,
		SourceRoot: binding.sourceRoot, SourceTreeSHA256: registration.SourceTreeSHA256, SourceTreeFileCount: registration.SourceTreeFileCount,
		ManifestSHA256: registration.ManifestSHA256, RequestedAt: requestedAt,
	})
}

type DevelopmentSourceServiceV1 struct {
	store     pluginport.Store
	authority pluginport.InstallationAuthority
	binding   DevelopmentSourceBindingV1
	now       func() time.Time
}

func NewDevelopmentSourceServiceV1(store pluginport.Store, authority pluginport.InstallationAuthority, binding DevelopmentSourceBindingV1, now func() time.Time) (*DevelopmentSourceServiceV1, error) {
	if store == nil || authority == nil || now == nil {
		return nil, ErrUnavailable
	}
	if _, err := binding.NewIntentV1(time.Unix(0, 0).UTC()); err != nil {
		return nil, ErrPackageAuthority
	}
	return &DevelopmentSourceServiceV1{store: store, authority: authority, binding: binding, now: now}, nil
}

func (service *DevelopmentSourceServiceV1) Materialize(ctx context.Context, intent domainplugin.IntentV1) (pluginport.ResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil {
		return pluginport.ResultV1{}, ErrUnavailable
	}
	expected, err := service.binding.NewIntentV1(parseDevelopmentIntentTimeV1(intent.RequestedAt))
	if err != nil || domainplugin.ValidateIntentV1(intent) != nil || intent != expected {
		return pluginport.ResultV1{}, ErrPackageAuthority
	}
	result, err := service.store.Materialize(ctx, intent, service.authority, service.now().UTC())
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	if domainplugin.ValidateReceiptForIntentV1(result.Receipt, intent) != nil || service.validateResultV1(result) != nil {
		return pluginport.ResultV1{}, ErrPackageAuthority
	}
	return result, nil
}

func (service *DevelopmentSourceServiceV1) ResolveActive(ctx context.Context) (pluginport.ResultV1, error) {
	if service == nil || ctx == nil || ctx.Err() != nil {
		return pluginport.ResultV1{}, ErrUnavailable
	}
	result, err := service.store.ResolveActive(ctx, service.authority)
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	if err := service.validateResultV1(result); err != nil {
		return pluginport.ResultV1{}, err
	}
	return result, nil
}

func (service *DevelopmentSourceServiceV1) validateResultV1(result pluginport.ResultV1) error {
	receipt, binding := result.Receipt, service.binding
	if domainplugin.ValidateTrustedReceiptV1(receipt, service.authority.KeyID(), service.authority.PublicKey()) != nil || domainplugin.ValidateIndexForReceiptV1(result.Index, receipt) != nil ||
		receipt.Origin != domainplugin.DevelopmentSourceOriginV1 || receipt.PackageAuthoritySHA256 != "" || receipt.SourceRegistrationSHA256 != binding.digest ||
		receipt.Target != binding.target || receipt.PluginName != binding.registration.Identity.PackageID || receipt.PluginVersion != binding.registration.Identity.PackageVersion ||
		receipt.SourceTreeSHA256 != binding.registration.SourceTreeSHA256 || receipt.SourceTreeFileCount != binding.registration.SourceTreeFileCount || receipt.ManifestSHA256 != binding.registration.ManifestSHA256 || receipt.EntrypointSHA256 != "" {
		return ErrPackageAuthority
	}
	return nil
}

func parseDevelopmentIntentTimeV1(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}
