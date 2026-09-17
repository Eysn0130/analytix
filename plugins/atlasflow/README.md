# AtlasFlow

AtlasFlow is the tt-a1i diagram generation plugin for analytix and Codex.

It includes two Skills:

- `atlasflow`: the core AtlasFlow diagram renderer for architecture, workflow, sequence, dataflow, and lifecycle diagrams.
- `atlasflow-analytix-domain`: a vertical-domain routing layer for public-security, official-document, case, and general business diagrams.

The plugin ships as a standard `.codex-plugin` package with `skills/`, assets, and Hub catalog fixtures under `hub/`.

## Renderer prerequisites

Install the lockfile-defined dependencies in `skills/atlasflow` with `npm ci`
before using the diagram CLI. JSON-schema validation is mandatory: if Ajv or
its dependency closure cannot load, render, validate and inspect fail before
writing output. Help remains available. Do not treat an unavailable validator
as a valid degraded rendering mode. The pure Canvas adapter retains its
separate, data-only input checks and does not import the CLI validator.

## Analytix Hub Release Payload

Prepare the package and Hub catalog files:

```bash
node scripts/prepare-hub-package.mjs --force --write-hub --json
```

The script writes the release payload under `/tmp/analytix-hub-atlasflow-<version>/`,
creates `/tmp/atlasflow-<version>.tar.gz`, computes its SHA-256, and updates:

- `hub/marketplace.json`
- `hub/skills.json`
- `hub/install-policy.json`

Upload the generated tarball to the `source.url` in `hub/marketplace.json`.
The analytix plugin page discovers AtlasFlow from the Hub marketplace endpoint,
downloads the tarball, verifies the checksum, then installs it into the
Analytix-owned runtime plugin cache.
