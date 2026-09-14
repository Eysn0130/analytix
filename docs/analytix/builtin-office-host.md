# Built-in Office package host

Status: Reference. Describes source-development integration, not native editor
acceptance, packaged admission or release readiness.

Documents, Spreadsheets and Presentations are real first-party package sources
under `plugins/analytix-*`. Their static editor declarations request only local
editing of a user-selected object with explicit save. They install through the
existing plugin materialization owner and installation signing authority. They
contain no alternate Agent, credentials, shell entrypoint, MCP server or Hub
login dependency.

Unpackaged Electron supplies its own source root to the Go runtime. Ambient
copies of that configuration are removed before child launch. Both the
production build and packaged-executable inspection deny this source path.
Inspection binds the source tree, canonical declaration and adapter descriptor
to an explicit `development-source` receipt. It never becomes formal package
authority or a publishable artifact. Interrupted materialization keeps its first
durable intent rather than selecting a new transaction on restart.

The protected `/v1/local-display/plugin-package-host` endpoint lists packages,
changes desired state and invokes a finite static adapter operation. It uses the
existing runtime bearer and typed local-display header. Electron accepts this
IPC only from its own main frame. Browser previews and third-party frames cannot
manage the host. The renderer cannot register roots, executables or principals.

Signed activation is scoped to the exact installed generation and uses revision
compare-and-swap. Missing activation is `unset`, not a fabricated disabled
record. The plugin page reads and persists state through Core; it does not use
marketplace localStorage flags as installation or execution authority. After
either a successful or uncertain mutation it reads the current signed state.
Enabled and available are distinct: without a ready adapter, an enabled package
is displayed as an unavailable editor.

One Host orders activation changes and complete invocations. Disable waits for
an invocation already executing; after its commit, the previous enabled state
cannot start another call. An adapter must not re-enter the Host. Current
registrations have no native editor adapter attached, so source materialization
and persistent enable/disable are usable while native file editing remains an
integration gap. Loading a plugin page does not establish that an Office engine
has been admitted or executed.

Focused evidence includes all three actual repository package trees materialized
into an isolated installation, persisted activation across a fresh Host, exact
protected-route checks, interrupted-intent reuse, Host race tests, and Main
response/generation/revision checks. It does not replace the mandatory aggregate
gate or GUI/native file acceptance of the final candidate.
