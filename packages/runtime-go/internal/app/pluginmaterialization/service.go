package pluginmaterialization

import (
	"context"
	"errors"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpluginpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

var (
	ErrUnavailable      = errors.New("bundled_plugin_materialization_service_unavailable")
	ErrPackageAuthority = errors.New("bundled_plugin_materialization_package_authority_invalid")
)

// FormalPackageBindingV1 is frozen when the service is assembled from a
// verified packaged build. Materialization requires the admitted package
// identity and declaration digests. A read-only existing-generation consumer
// may omit them, but then Materialize always rejects and ResolveActive relies
// on the signed receipt/index plus the re-inspected declaration-bound tree.
type FormalPackageBindingV1 struct {
	AuthoritySHA256            string
	Target                     domainplugin.TargetV1
	PackageIdentity            domainpluginpackage.PackageIdentityV1
	DeclarationRawSHA256       string
	DeclarationCanonicalSHA256 string
	SourceTreeSHA256           string
	SourceTreeFileCount        uint64
	ManifestSHA256             string
	EntrypointSHA256           string
}

type Service struct {
	store     pluginport.Store
	authority pluginport.InstallationAuthority
	binding   FormalPackageBindingV1
	now       func() time.Time
}

func NewService(store pluginport.Store, authority pluginport.InstallationAuthority, binding FormalPackageBindingV1, now func() time.Time) (*Service, error) {
	identityBound := binding.PackageIdentity != (domainpluginpackage.PackageIdentityV1{}) ||
		binding.DeclarationRawSHA256 != "" || binding.DeclarationCanonicalSHA256 != ""
	if store == nil || authority == nil || now == nil ||
		!domainplugin.IsCanonicalSHA256V1(binding.AuthoritySHA256) || domainplugin.ValidateTargetV1(binding.Target) != nil ||
		(identityBound && (!domainpluginpackage.ValidPackageIdentityV1(binding.PackageIdentity) ||
			!domainplugin.IsCanonicalSHA256V1(binding.DeclarationRawSHA256) ||
			!domainplugin.IsCanonicalSHA256V1(binding.DeclarationCanonicalSHA256))) ||
		!domainplugin.IsCanonicalSHA256V1(binding.SourceTreeSHA256) || binding.SourceTreeFileCount == 0 ||
		binding.SourceTreeFileCount > domainplugin.MaxSourceTreeFilesV1 ||
		!domainplugin.IsCanonicalSHA256V1(binding.ManifestSHA256) || !domainplugin.IsCanonicalSHA256V1(binding.EntrypointSHA256) {
		return nil, ErrUnavailable
	}
	return &Service{store: store, authority: authority, binding: binding, now: now}, nil
}

func (service *Service) Materialize(ctx context.Context, intent domainplugin.IntentV1) (pluginport.ResultV1, error) {
	if service == nil || service.store == nil || service.authority == nil || service.now == nil {
		return pluginport.ResultV1{}, ErrUnavailable
	}
	if ctx == nil || ctx.Err() != nil || domainplugin.ValidateIntentV1(intent) != nil ||
		intent.PackageAuthoritySHA256 != service.binding.AuthoritySHA256 || intent.Target != service.binding.Target ||
		intent.PluginName != service.binding.PackageIdentity.PackageID ||
		intent.PluginVersion != service.binding.PackageIdentity.PackageVersion ||
		intent.SourceTreeSHA256 != service.binding.SourceTreeSHA256 || intent.SourceTreeFileCount != service.binding.SourceTreeFileCount ||
		intent.ManifestSHA256 != service.binding.ManifestSHA256 || intent.EntrypointSHA256 != service.binding.EntrypointSHA256 {
		return pluginport.ResultV1{}, ErrPackageAuthority
	}
	return service.store.Materialize(ctx, intent, service.authority, service.now().UTC())
}

func (service *Service) ResolveActive(ctx context.Context) (pluginport.ResultV1, error) {
	if service == nil || service.store == nil || service.authority == nil || ctx == nil || ctx.Err() != nil {
		return pluginport.ResultV1{}, ErrUnavailable
	}
	result, err := service.store.ResolveActive(ctx, service.authority)
	if err != nil {
		return pluginport.ResultV1{}, err
	}
	if result.Receipt.PackageAuthoritySHA256 != service.binding.AuthoritySHA256 || result.Receipt.Target != service.binding.Target {
		return pluginport.ResultV1{}, ErrPackageAuthority
	}
	if service.binding.PackageIdentity != (domainpluginpackage.PackageIdentityV1{}) &&
		(result.Receipt.PluginName != service.binding.PackageIdentity.PackageID ||
			result.Receipt.PluginVersion != service.binding.PackageIdentity.PackageVersion) {
		return pluginport.ResultV1{}, ErrPackageAuthority
	}
	if result.Receipt.SourceTreeSHA256 != service.binding.SourceTreeSHA256 ||
		result.Receipt.SourceTreeFileCount != service.binding.SourceTreeFileCount ||
		result.Receipt.ManifestSHA256 != service.binding.ManifestSHA256 ||
		result.Receipt.EntrypointSHA256 != service.binding.EntrypointSHA256 {
		return pluginport.ResultV1{}, ErrPackageAuthority
	}
	return result, nil
}
