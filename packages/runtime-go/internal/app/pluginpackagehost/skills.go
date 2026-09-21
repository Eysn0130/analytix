package pluginpackagehost

import (
	"context"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainskill "analytix.local/runtime-go/internal/domain/skill"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// HostedSkill contains private immutable instruction bytes. Public discovery
// projects only its finite ID/name/description, never the binding or file roots.
type HostedSkill struct {
	Binding  adapterport.Binding
	Snapshot domainskill.PackageSnapshot
}

func (s *Service) loadSkillLocked(ctx context.Context, expected adapterport.Binding) (HostedSkill, error) {
	principal, err := s.principal(ctx)
	if err != nil {
		return HostedSkill{}, err
	}
	registration, ok := s.registrations[expected.PackageID]
	if !ok || registration.SkillReader == nil {
		return HostedSkill{}, ErrNotFound
	}
	current, err := s.resolve(ctx, registration)
	if err != nil {
		return HostedSkill{}, err
	}
	activation, exists, err := s.activation(ctx, registration, current)
	if err != nil {
		return HostedSkill{}, err
	}
	if !exists || activation.DesiredState != domainplugin.DesiredEnabledV1 {
		return HostedSkill{}, ErrDisabled
	}
	binding := bindingFor(registration, current, activation.Revision)
	if binding != expected {
		return HostedSkill{}, ErrConflict
	}
	snapshot, err := registration.SkillReader.ReadSkill(ctx, binding)
	if err != nil || !snapshot.Valid() || snapshot.EntryRelativePath() != "SKILL.md" || len(snapshot.Paths()) != 1 {
		return HostedSkill{}, ErrUnavailable
	}
	after, err := s.resolve(ctx, registration)
	if err != nil || after != current {
		return HostedSkill{}, ErrConflict
	}
	afterActivation, afterExists, err := s.activation(ctx, registration, after)
	if err != nil || !afterExists || afterActivation != activation {
		return HostedSkill{}, ErrConflict
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return HostedSkill{}, err
	}
	return HostedSkill{Binding: binding, Snapshot: snapshot}, nil
}

// Skills withdraws contributions whose signed activation or installed generation
// cannot be verified. Reading does not invoke an adapter or contact a Provider.
func (s *Service) Skills(ctx context.Context) []HostedSkill {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var skills []HostedSkill
	for _, id := range s.packageIDs {
		registration := s.registrations[id]
		if registration.SkillReader == nil {
			continue
		}
		current, err := s.resolve(ctx, registration)
		if err != nil {
			continue
		}
		activation, exists, err := s.activation(ctx, registration, current)
		if err != nil || !exists || activation.DesiredState != domainplugin.DesiredEnabledV1 {
			continue
		}
		loaded, err := s.loadSkillLocked(ctx, bindingFor(registration, current, activation.Revision))
		if err == nil {
			skills = append(skills, loaded)
		}
	}
	return skills
}

// WithSkill serializes consumption of an already prepared skill with disable
// and upgrade validation. A newer body never inherits an older task's identity.
// The callback must not re-enter this Host.
func (s *Service) WithSkill(ctx context.Context, expected adapterport.Binding, digest string, consume func(domainskill.PackageSnapshot) error) error {
	if s == nil || consume == nil || digest == "" {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	principal, err := s.principal(ctx)
	if err != nil {
		return err
	}
	loaded, err := s.loadSkillLocked(ctx, expected)
	if err != nil {
		return err
	}
	if loaded.Snapshot.Digest() != digest {
		return ErrConflict
	}
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return err
	}
	if err := consume(loaded.Snapshot); err != nil {
		return err
	}
	// The Host lock orders local activation calls, but not identity revocation
	// or independently changed installed state. A late result must retain both
	// the original principal and the exact installed snapshot authority.
	if err := s.validatePrincipal(ctx, principal); err != nil {
		return err
	}
	current, err := s.loadSkillLocked(ctx, expected)
	if err != nil {
		return err
	}
	if current.Snapshot.Digest() != digest {
		return ErrConflict
	}
	return s.validatePrincipal(ctx, principal)
}
