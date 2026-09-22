# Exact dependency legal inputs

The artifact legal owner consumes `manifest.json`; the existing after-pack owner
copies its materials and both exact lockfiles into `Resources/dependency-legal`
before resource sealing and signing. These inputs do not change third-party
package metadata, execute upstream scripts, or grant publication authority.

The 31 records originate from the complete retained Core96e55 artifact inventory.
Every npm archive was downloaded from the public npm registry and matched against
the SHA-512 integrity in the current root/runtime locks. `bindings` records each
lock path, resolved URL and integrity, including duplicate installations.
`files` contains the archive's original file digests, not a package-name exemption.
The degenerator5.0.1 archive contains two identical `dist/index.js` entries (one
spelled `./dist/index.js`); they were compared byte-for-byte without extraction
or execution. Conflicting duplicate entries are not accepted.

`packageJsonSha256` pins the original archive bytes and the existing pinned
electron-builder26.15.3 metadata projection. Each projection was compared with
the archive: only builder-owned removal of scripts, keywords, bugs and its
documented metadata exclusions was observed. No name, version, author, license,
dependency or code field was changed. All other retained package files must match
their archive digest. A changed builder transformation requires fresh review.

Materials are unmodified upstream license files or complete license-bearing
README files. Each material records its npm integrity or fixed Git commit/path,
content digest and, where supplied, the separately verified Git blob from the
2026-09-22 closure attachment. The original declaration remains visible in audit
output. The MIT alternative of type-fest4.41.0 is explicitly selected and its CC0
text retained. Unknown AND/WITH terms cannot use this supplement.

Sixteen of the original package records have complete MIT text inputs.
Fifteen retain explicit unresolved obligations; a root license on canvas does
not establish the provenance/notices of its native embedded components. Missing
texts and provenance are not fabricated from a standard license template or a
sibling version. Review of public material: Codex, 2026-09-22; exact archive and
Git evidence is retained in the task's private evidence directory. No upstream
runtime implementation was adopted by this change.

The artifact reader verifies the copied catalog against this canonical input,
both lockfiles, each actual package owner/path and retained content, and every
material digest. ASAR and its actual physical unpacked namespace are distinct;
arbitrary suffixes cannot acquire another instance's evidence. External runtime
dependencies remain external even inside `packages/runtime/node_modules`.

The catalog is a bounded supplement to the existing legal audit, not a complete
SBOM or proof of every font, WASM and native component's redistribution duties.
The dated QA report records remaining source and installed-release boundaries.
