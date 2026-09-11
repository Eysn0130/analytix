# Windows QA Operator Runbook

Status: Operational
Applies to: approved analytix Windows 11 QA host on the local LAN
Current as of: 2026-07-12
Source of truth: local SSH configuration, approved secret storage, and the
current packaging/QA scripts
Supersedes: the retired credential-bearing Windows remote-control note

This runbook deliberately records the stable connection method and approved
non-secret machine metadata. It never records a Windows password, private key,
RustDesk identifier, permanent access code, token, product key, or recovery
code.

## Approved host metadata

| Field | Value |
| --- | --- |
| LAN address | `192.168.31.90` |
| Hostname | `DESKTOP-HNJ503B` |
| Operating system | Windows 11 Professional |
| QA account | `analytix_remote` |
| SSH | TCP `22` |
| RDP | TCP `3389` |
| Local SSH alias | `analytix-win-qa` |
| Local identity file | `~/.ssh/analytix_win_test_ed25519` |

The LAN address, hostname, account name, and ports identify the approved QA
machine but do not authenticate an operator. Do not extend this table with
credential values.

## Current operator state

The 2026-07-10 operator maintenance established the following local state:

- `ssh analytix-win-qa` uses the Ed25519 identity above and has been verified
  against `DESKTOP-HNJ503B` without password input.
- The Windows/RDP credential was rotated and is stored in macOS Keychain under
  service `analytix.windows.qa.rdp`, account `analytix_remote`.
- The RustDesk permanent credential was rotated and is stored under Keychain
  service `analytix.windows.qa.rustdesk`.
- The RustDesk machine identifier is stored separately under Keychain service
  `analytix.windows.qa.rustdesk-id`.
- The Analytix Hub QA login is stored under Keychain service
  `analytix.qa.hub-test-account`; its account field is the login email and its
  password value is the login password.
- `~/.ssh/config` is locally configured with mode `0600`.

These statements describe operator access state, not product QA evidence. On
2026-07-10, every locally retained branch, tag, stash, and Codex snapshot ref
was sanitized, unreachable objects were pruned, and the temporary recovery
bundles were removed after the redacted history verifier passed. This checkout
has no configured Git remote. Any independently held pre-rewrite clone, bundle,
backup, or release archive remains untrusted and must be discarded or sanitized
before it can exchange refs or objects with this repository.

## SSH alias

The local, untracked `~/.ssh/config` entry is:

```sshconfig
Host analytix-win-qa
  HostName 192.168.31.90
  User analytix_remote
  Port 22
  IdentityFile ~/.ssh/analytix_win_test_ed25519
  IdentitiesOnly yes
```

Keep the local files private:

```bash
chmod 600 ~/.ssh/config ~/.ssh/analytix_win_test_ed25519
chmod 644 ~/.ssh/analytix_win_test_ed25519.pub
stat -f '%Lp %N' ~/.ssh/config ~/.ssh/analytix_win_test_ed25519
```

Before accepting a changed host key, compare its fingerprint with the Windows
operator through an independent channel. Do not remove a known-host entry just
to bypass a mismatch.

Routine connectivity verification must not fall back to a password prompt:

```bash
ssh -o BatchMode=yes analytix-win-qa 'hostname'
```

The expected hostname is `DESKTOP-HNJ503B`. A non-zero exit is a blocker; do
not embed a fallback credential in the command.

## Ed25519 key enrollment or replacement

The current public key is already enrolled. Use this procedure only for an
authorized key rotation:

1. Generate the replacement locally without placing key material in the
   repository:

   ```bash
   umask 077
   ssh-keygen -t ed25519 -a 100 -f ~/.ssh/analytix_win_test_ed25519.next -C 'analytix Windows QA'
   ```

2. Through an already authenticated Windows console or RDP session, confirm
   which `AuthorizedKeysFile` rule applies before writing a key:

   ```powershell
   whoami /groups | findstr /i "S-1-5-32-544"
   findstr /n /i /c:"Match Group administrators" /c:"AuthorizedKeysFile" C:\ProgramData\ssh\sshd_config
   ```

   The approved `analytix_remote` account currently belongs to the local
   Administrators group (`S-1-5-32-544`). The active `Match Group
   administrators` rule therefore uses
   `C:\ProgramData\ssh\administrators_authorized_keys`, not the profile-local
   file. Add only the one-line public key and set ACLs by well-known SID so the
   procedure is independent of Windows display language:

   ```powershell
   $authorizedKeys = Join-Path $env:ProgramData 'ssh\administrators_authorized_keys'
   $publicKey = '<one-line Ed25519 public key>'
   if (-not (Test-Path -LiteralPath $authorizedKeys)) {
     New-Item -ItemType File -Path $authorizedKeys | Out-Null
   }
   if (-not (Select-String -LiteralPath $authorizedKeys -SimpleMatch $publicKey -Quiet)) {
     Add-Content -LiteralPath $authorizedKeys -Value $publicKey
   }
   icacls $authorizedKeys /inheritance:r /grant:r '*S-1-5-32-544:F' '*S-1-5-18:F'
   ```

   Only a confirmed non-administrator account that is not captured by another
   `Match` block should use `%USERPROFILE%\.ssh\authorized_keys`; apply that
   account's own SID ACL and the effective `sshd_config`, rather than copying
   the administrator procedure blindly.

3. Test the replacement in a second terminal with `BatchMode=yes`. Keep the
   existing key until the new key succeeds and a separate RDP recovery path is
   confirmed.
4. Update the alias to the replacement identity, retest, then remove the old
   public key from Windows and securely remove the retired local private key.

The current administrator file has been verified with full control only for
Administrators and SYSTEM. Re-check both the effective rule and ACL after an
account-role or OpenSSH configuration change. Do not guess the active
authorization file and do not print its key contents during verification.

## Local secret-manager locators

The Keychain service names are safe locators; their values are not repository
content. Check only that each item exists, suppressing command output:

```bash
security find-generic-password -a analytix_remote -s analytix.windows.qa.rdp >/dev/null 2>&1
security find-generic-password -s analytix.windows.qa.rustdesk >/dev/null 2>&1
security find-generic-password -s analytix.windows.qa.rustdesk-id >/dev/null 2>&1
security find-generic-password -s analytix.qa.hub-test-account >/dev/null 2>&1
```

Use Keychain Access or the destination application integration to retrieve a
value. Do not use a command option that prints it into a recorded terminal,
paste it into chat, or place it in an environment file under the repository.
When rotating an item from macOS, `security add-generic-password ... -w` must
put `-w` last so the value is requested interactively rather than entering the
process argument list.

## Local Provider live QA

Ordinary current-source acceptance uses local Provider onboarding or Provider
Settings backed by the Registry and protected Secret Store; it does not use
Hub account readiness or a managed gateway token. See the accepted
[credential authority](../../../openspec/specs/local-provider-credential-authority/spec.md)
and [isolated acceptance](../../../openspec/specs/hub-auth-test-bootstrap/spec.md).

1. Use a fresh isolated profile and the credential authorized for this exact
   Provider test. Keep package bootstrap disabled and do not copy prior state.
2. Enter the credential through the normal protected product UI, then verify
   the committed local connection and selected model are usable.
3. Use the model and reasoning selection required by the currently authorized
   acceptance scenario; a model from an older QA run is not a default.
4. Record package/source identity, input method and redacted outcome. Setup
   success alone does not establish model, privacy or release acceptance.

The Keychain service `analytix.qa.hub-test-account` above is retained only for
explicit legacy Hub compatibility QA. Its presence is not authority for local
Provider setup, and its gateway token must not be reused as a manual API key.
The host connection metadata in this runbook remains a dated configuration
reference and was not revalidated by this documentation correction.

## RDP fallback

Use RDP only when SSH cannot perform the required GUI QA or when SSH recovery
is necessary.

1. Open Microsoft Windows App / Remote Desktop locally.
2. Set the PC endpoint to `192.168.31.90:3389` and the account to
   `analytix_remote`.
3. Resolve the credential through Keychain service
   `analytix.windows.qa.rdp`; never copy it from repository text.
4. Confirm the displayed host is `DESKTOP-HNJ503B` before changing state.
5. End the RDP session after QA and remove screenshots or logs that reveal
   authentication or unrelated desktop content.

RDP success proves operator access only. It is not evidence that the packaged
application, Go runtime, updater, or uninstall flow passed.

## RustDesk fallback

RustDesk is the last-resort GUI path when both SSH and RDP are unsuitable.

1. Resolve the machine identifier from Keychain service
   `analytix.windows.qa.rustdesk-id` inside the local operator session.
2. Resolve the rotated permanent credential from service
   `analytix.windows.qa.rustdesk`, or prefer a newly issued one-time session
   code when an operator is present.
3. Confirm `DESKTOP-HNJ503B` in the remote session before interacting.
4. Limit work to the intended QA surface. Do not open activation, credential,
   personal-account, or unrelated data views while recording evidence.
5. End the session and revoke any one-time code after use.

Neither the identifier nor an access code may be copied into this runbook,
scripts, issue text, command output, screenshots, or QA logs.

## Analytix QA workflow

1. Confirm SSH identity and host without an interactive credential:

   ```bash
   ssh -o BatchMode=yes analytix-win-qa 'hostname'
   ```

2. On the Windows host, enter the intended project checkout and record the
   tested revision:

   ```powershell
   git status --short --branch
   git rev-parse HEAD
   ```

3. Run the smallest checks covering the change. Official Windows package work
   follows `npm run dist:win:official` and the current release scripts; dated
   QA reports do not authorize a new release.
4. For startup diagnosis, combine process identity, logs, listener ownership,
   and runtime health. A listening port alone is not sufficient evidence.
5. Store only sanitized evidence tied to the tested commit, package version,
   environment, exact command, and honest result status.
6. End remote sessions and verify that no credential-bearing file, screenshot,
   terminal transcript, or generated output is being added to Git.

## Repository verification and incident boundary

Run the secret-safe worktree check after changing this runbook or its links:

```bash
npm run verify:windows-qa-runbook
```

After the rename is indexed, the stronger check also proves that this runbook
is tracked and the retired path is no longer in the index:

```bash
npm run verify:windows-qa-runbook:indexed
```

The 2026-07-10 history rewrite is locally complete. The closure check verifies
that no locally retained ref can reach the retired path and that no supported
retained text object matches the redacted Windows-access credential or
identifier patterns:

```bash
npm run verify:windows-qa-runbook:history
```

The checker reports only a path, line number, and rule name. It never prints a
matched value.

Local incident closure completed on 2026-07-10:

1. Windows/RDP and RustDesk operator credentials were rotated and the approved
   SSH key path was verified;
2. all local branches, tags, stashes, and Codex snapshot refs were rewritten or
   sanitized;
3. both exact-path and redacted retained-object scans passed;
4. reflogs were expired and unreachable objects were pruned; and
5. temporary recovery bundles were removed after verification.

No Git remote is configured in this checkout, so there was no remote ref to
rewrite or publish. An independently retained pre-rewrite clone, bundle,
backup, or release archive is outside this local closure boundary. Do not use
one as a source of refs or objects unless its owner first performs the same
sanitization and verification.

Never restore an old ref merely to recover convenience access. History cleanup
cannot revoke a credential, and credential rotation cannot remove a copied Git
blob; both gates are required.
