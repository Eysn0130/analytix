## ADDED Requirements

### Requirement: Sidebar footer opens local Settings
The desktop sidebar SHALL provide an ordinary Settings entry that opens local Analytix settings without fetching or displaying Hub identity, plan, usage, referral, or logout state.

#### Scenario: Sidebar is rendered
- **WHEN** the main workspace sidebar is visible
- **THEN** its footer provides the Settings entry without instantiating or refreshing a Hub account service

#### Scenario: User opens Settings
- **WHEN** the user selects the footer Settings entry
- **THEN** the app navigates directly to the local Settings view and Provider Settings remains available

### Requirement: Deprecated Hub account actions are lazy and isolated
Preserved Hub account, usage, referral, and logout interactions SHALL appear only inside an explicit deprecated Hub compatibility surface and SHALL NOT control local Provider readiness or ordinary sidebar state.

#### Scenario: User never opens Hub compatibility
- **WHEN** the user uses the sidebar, workspace, or Settings normally
- **THEN** no Hub account refresh, usage request, referral request, logout state, timer, or token read occurs

#### Scenario: User opens a deprecated account action
- **WHEN** the user explicitly enters the Hub compatibility surface and selects an account action
- **THEN** only that lazy compatibility lifecycle may access preserved Hub behavior and the local Registry remains authoritative

## REMOVED Requirements

### Requirement: Sidebar footer becomes account trigger
**Reason**: Ordinary sidebar state must not instantiate or depend on Hub account identity.
**Migration**: Restore the direct Settings footer and move preserved account access behind the explicit deprecated compatibility surface.

### Requirement: Account dropdown exposes account actions
**Reason**: Hub account actions are no longer ordinary navigation or model-readiness controls.
**Migration**: Keep supported account actions only in the lazy deprecated Hub surface; Settings opens directly from the sidebar.

### Requirement: Usage and referral are Hub-backed
**Reason**: Ordinary startup and sidebar must perform no Hub request or account refresh.
**Migration**: Preserve usage or referral access only as an explicit lazy compatibility action with no Provider fallback effect.

### Requirement: Logout clears account menu state
**Reason**: Local Provider authority and model access no longer depend on Hub logout state.
**Migration**: Hub logout clears only preserved compatibility state; local Provider deletion or disconnection requires an explicit Registry operation.
