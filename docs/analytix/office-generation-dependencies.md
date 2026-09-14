# Office generation dependencies

Status: implementation inventory, 2026-09-15. These package dependencies support
Analytix-owned data-only adapters. They do not admit the native preview engine,
replace Provider/file authority, or establish packaged-product acceptance.

| Dependency | Exact source and integrity | Use / license |
| --- | --- | --- |
| PptxGenJS 4.0.1 | `gitbrent/PptxGenJS` commit `3c9ec1b687c174952166f6a34b5e87ebf69fa469`; npm SHA-512 integrity is pinned in `package-lock.json` | Node data-only PPTX generation; MIT, Brent Ely 2015–2022 |
| Excelize v2.11.0 | `xuri/excelize` commit `90ff348959dd842cbf3a4fbd93a78afbe18db476`; Go sum `h1:HxaEFl6sRN2+8J5a8HaKq+0M4FsjBGMnWWtjOCPSG88=` | Go data-only XLSX generation; BSD-3-Clause, Excelize Authors and Geoffrey J. Teale |

Official pinned sources: [PptxGenJS](https://github.com/gitbrent/PptxGenJS/tree/3c9ec1b687c174952166f6a34b5e87ebf69fa469)
and [Excelize](https://github.com/xuri/excelize/tree/90ff348959dd842cbf3a4fbd93a78afbe18db476).
The npm registry gitHead matches the upstream tag; the Go module download records
the version, Git origin/ref/commit and checksum. No upstream Skill or application
implementation was copied. Analytix owns the finite request, authorization,
validation, persistence and display adapters.

Unmodified primary license texts are retained in `THIRD_PARTY_NOTICES.md`.
Their original file SHA-256 values are:

- PptxGenJS `LICENSE`: `7a2bfe96150786ed1908b8e63f98ebab88875c1e79e28faff6649e0f11f77e52`.
- Excelize `LICENSE`: `0b8e4f8789b88fa44dee713d934e4d36f52854b33670ab4a0ce3de69e8e224cd`.

The package manager locks preserve transitive dependency versions and integrity.
Distribution must retain the complete applicable dependency licenses, including
transitive notices; the two primary notices alone are not a complete package
inventory. Bundling and installed license-resource checks remain required.

Excelize v2.11.0 requires Go 1.25.0. `packages/runtime-go/go.mod` therefore raises
the previous 1.22 minimum; current CI/release workflows already pin Go 1.26.4.
The dependency also updates the shared `golang.org/x/sys` and `x/text` versions,
so focused filesystem/privacy/runtime checks accompany the generator checks.
PptxGenJS is installed without lifecycle scripts. Its optional path/URL image
loading APIs are outside the admitted adapter; only validated inline image bytes
are accepted. No new Python executable or alternate Agent runtime is introduced.

The existing openpyxl 3.1.5 writer remains part of its current backend export
owner. Reusing it here would require an additional trusted macOS Python launch
and package contract. The Go adapter avoids that extra production execution path.

## Transitive notice closure for this candidate

`THIRD_PARTY_NOTICES.md` retains the actual locked-version texts for mscfb,
msoleps (Apache-2.0), go-deepcopy (MIT), efp/nfp (BSD-3-Clause), and the updated
Go x/crypto, x/net, x/text and x/sys modules (BSD-3-Clause and PATENTS). It also
retains image-size, queue, and the PptxGenJS-private Node/Undici type licenses
(MIT). Type-only dependencies are inventoried conservatively; this is not a
claim that they are shipped as executable content.

The npm `https@1.0.0` dependency archive contains only `package.json`, declaring
ISC, and no implementation or LICENSE file. PptxGenJS's `https` import resolves
to Node's builtin module. No third-party HTTPS implementation is copied into
this adapter. The final packaged dependency inventory must account for that
metadata-only package and retain its metadata if included; this record does not
invent a missing license text. JSZip and inherits retain their existing locked
versions and remain covered by the existing dependency inventory.

`golang.org/x/sys@v0.46.0` was downloaded and verified at its exact module sum
`h1:noSf2Fq6F8DBgS+LysIkx7rIExoNHJsxOAtPp4rthXw=`; its source revision is
`d58dcfa8a74514c0ef0fc401259156c5e2fc9ff5`. Final package resource/notice presence
and native engine/font admission are separate remaining checks.
