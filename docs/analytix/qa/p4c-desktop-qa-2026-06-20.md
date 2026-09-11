# P4C desktop QA - 2026-06-20

Scope:

Confirmed checkpoint rewind restore apply desktop smoke for the local
`codex/p4c-confirmed-rewind-restore-apply` worktree.

Commands and observations:

```text
npm run dev
```

- `npm run build:runtime` completed.
- Electron main and preload dev bundles completed.
- Port `5173` was already occupied by another local Electron/Vite session, so
  the P4C dev renderer started on `http://localhost:5174/`.
- Electron launched a real desktop window named `Analytix`.
- Renderer dev server returned `HTTP/1.1 200 OK`.
- Runtime health returned:

```json
{"status":"ok","service":"analytix","mode":"serve"}
```

Process evidence:

```text
Electron main process ran from /Users/sun/Projects/analytix/node_modules/electron/dist/Electron.app
Runtime process listened on 127.0.0.1:8901
Renderer dev server listened on localhost:5174
```

Visual evidence:

An OS-level screenshot of the `Analytix` window showed the real desktop UI
rendered successfully: left sidebar, project list, top toolbar, empty-state
hero, composer, model selector, and bottom workspace/runtime status. The
screenshot was captured to a temp path for inspection:

```text
/var/folders/0j/8t8qxbl9411616hkk0k67ssm0000gn/T/codex-shot-2026-06-20_20-15-22.png
```

P4C limitation:

This smoke did not manually click a live rewind apply control because the
running user data did not contain an existing thread with a `meta.rewindPlan`
tool block. Apply route/provider/confirmation behavior is covered by automated
tests, and the real desktop smoke proves the app starts, renders, and connects
to the runtime. Treat packaged release closure and a live UI apply fixture as
future QA gates.

Inherited Spec 07 release and push blockers:

```text
legacy-origin still points to https://github.com/KunAgent/Kun.git and remains a push blocker.
Release base URL and repository metadata must be finalized before public updates.
macOS/Windows signing and notarization are not production-ready until credentials are configured.
Windows NSIS must be verified on Windows.
```
