# hub-auth-test-bootstrap Specification

## Purpose
Define isolated normal local Provider onboarding for development and packaged acceptance, excluding Hub or development credential bootstrap.

## Requirements

### Requirement: Development uses normal protected local onboarding
Unpackaged development SHALL exercise developer-owned Provider credentials only through normal initial setup or Provider Settings backed by the production Registry and Secret Store, using an isolated temporary profile and data directory.

#### Scenario: Development requires a live Provider
- **WHEN** an authorized development run needs a developer-owned credential
- **THEN** the credential is entered through the visible protected product flow and is not acquired through Hub login bootstrap, copied tokens, plaintext settings, fixtures, or committed environment values

#### Scenario: Unit tests exercise Provider UI
- **WHEN** unit tests render startup, onboarding, sidebar, or Provider Settings behavior
- **THEN** they use key-free mocked local Provider APIs and synthetic credential markers without a live Hub account or real credential

### Requirement: Packaged users provide their own credentials
Packaged and production builds SHALL reject development or Hub test credential bootstrap and SHALL require each user to configure a Provider through normal protected onboarding or Provider Settings in their own profile.

#### Scenario: A fresh package sees bootstrap variables
- **WHEN** a packaged app starts with legacy Hub bootstrap or test credential environment variables
- **THEN** it ignores or rejects them, performs no automatic login or credential import, and presents local Provider setup when needed

#### Scenario: A package user completes setup
- **WHEN** the user enters a Provider credential through packaged onboarding
- **THEN** the production Secret Store and Registry commit it without exposing it to settings, renderer state, public IPC, or evidence

### Requirement: Credential-bearing acceptance is isolated and secret-free
Development and package acceptance SHALL use fresh isolated data directories, SHALL not open existing user profiles, and SHALL retain no real credential value, token, raw Provider body, or copied Secret Store in source, artifacts, logs, screenshots, telemetry, or evidence.

#### Scenario: Acceptance run completes
- **WHEN** a credential-bearing development or package run finishes
- **THEN** retained evidence contains only redacted status and synthetic markers and the task-owned isolated profile is handled according to its explicit cleanup or retention policy

#### Scenario: Credential setup fails
- **WHEN** a credential is absent, rejected, unreadable, or cannot be stored safely
- **THEN** the app reports a redacted local setup failure and does not fall back to Hub login, Hub gateway, or a production bypass
