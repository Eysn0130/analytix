# hub-account-menu Specification

## Purpose
Define the ordinary local Settings entry and the separate lazy Hub compatibility surface.

## Requirements

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
