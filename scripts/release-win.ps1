# Windows controlled publication: validate an already-built, Authenticode-signed
# artifact set before writing local metadata, archiving, uploading, or promoting.
# Local packages use the explicit non-publishable path and never create release
# metadata, an installer/update feed, an archive, or a publication receipt.
#
# Usage (PowerShell):
#   .\scripts\release-win.ps1 -Tag v0.1.1 -Authority receipt.json -AuthorityPublicKey release.pub
#   .\scripts\release-win.ps1 -Tag v0.1.1 -Authority receipt.json -AuthorityPublicKey release.pub -R2 -PromoteR2
#   .\scripts\release-win.ps1 -LocalNonPublishable
#
# npm:
#   npm run release:win -- -Tag v0.1.1 -Authority receipt.json -AuthorityPublicKey release.pub -R2 -PromoteR2

param(
  [string]$Tag = '',
  [string]$Authority = '',
  [string]$AuthorityPublicKey = '',
  [ValidateSet('', 'beta', 'stable')]
  [string]$Channel = '',
  [switch]$Stable,
  [switch]$Beta,
  [switch]$Publish,
  [switch]$R2,
  [switch]$PromoteR2,
  [switch]$NoCache,
  [switch]$LocalNonPublishable,
  [string]$LocalNonPublishableOutput = ''
)

$ErrorActionPreference = 'Stop'

function Write-Info([string]$Message) { Write-Host $Message -ForegroundColor Cyan }
function Write-Ok([string]$Message) { Write-Host $Message -ForegroundColor Green }
function Write-Err([string]$Message) { Write-Host "[ERROR] $Message" -ForegroundColor Red }

function Assert-Semver([string]$Version) {
  if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    Write-Err "Release tag must be vX.Y.Z. electron-updater cannot use four-part versions: $Version"
    exit 1
  }
}

function Normalize-ReleaseChannel([string]$Value) {
  if (-not $Value) { return 'stable' }
  if ($Value -eq 'beta' -or $Value -eq 'stable') { return $Value }
  Write-Err "Release channel must be beta or stable, got: $Value"
  exit 1
}

function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    Write-Err "$Name not found in PATH."
    exit 1
  }
}

function Resolve-NpmCommand {
  $npmCmd = Get-Command 'npm.cmd' -ErrorAction SilentlyContinue
  if ($npmCmd) { return $npmCmd.Source }

  $npm = Get-Command 'npm' -ErrorAction SilentlyContinue
  if ($npm) { return $npm.Source }

  Write-Err 'npm not found in PATH.'
  exit 1
}

function Resolve-NpxCommand {
  $npxCmd = Get-Command 'npx.cmd' -ErrorAction SilentlyContinue
  if ($npxCmd) { return $npxCmd.Source }

  $npx = Get-Command 'npx' -ErrorAction SilentlyContinue
  if ($npx) { return $npx.Source }

  Write-Err 'npx not found in PATH.'
  exit 1
}

function Assert-WindowsX64Host {
  if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT -or
      $env:PROCESSOR_ARCHITECTURE -ne 'AMD64') {
    Write-Err 'Windows release packaging must run from a Windows x64 PowerShell process.'
    exit 1
  }
}

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Set-Location $Root
# Do not import a local environment file here. An unrestricted environment
# assignment such as NODE_OPTIONS or PATH would execute before the controlled
# publication verifier. Formal callers must pass authority and credentials as
# explicit parameters or through the already-controlled process environment.

$WindowsSigningCert = @(
  $env:WIN_CSC_LINK,
  $env:CSC_LINK,
  $env:ANALYTIX_WIN_SIGNING_CERT_PATH
) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1
$WindowsSigningPassword = @(
  $env:WIN_CSC_KEY_PASSWORD,
  $env:CSC_KEY_PASSWORD,
  $env:ANALYTIX_WIN_SIGNING_CERT_PASSWORD
) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -First 1
$ReleaseAuthority = if ($Authority.Trim()) { $Authority.Trim() } else { [string]$env:ANALYTIX_RELEASE_AUTHORITY }
$ReleaseAuthorityPublicKey = if ($AuthorityPublicKey.Trim()) {
  $AuthorityPublicKey.Trim()
} else {
  [string]$env:ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY
}

if ($Stable -and $Beta) {
  Write-Err 'Use only one of -Stable or -Beta.'
  exit 1
}

$RequestedChannel = if ($Stable) {
  'stable'
} elseif ($Beta) {
  'beta'
} elseif ($Channel) {
  $Channel
} elseif ($env:RELEASE_CHANNEL) {
  $env:RELEASE_CHANNEL
} elseif ($env:ANALYTIX_UPDATE_CHANNEL) {
  $env:ANALYTIX_UPDATE_CHANNEL
} else {
  'stable'
}
$ReleaseChannel = Normalize-ReleaseChannel $RequestedChannel

if ($LocalNonPublishable) {
  if ($Publish -or $R2 -or $PromoteR2) {
    Write-Err '-LocalNonPublishable cannot upload, promote, or publish.'
    exit 1
  }
  if ($Tag.Trim() -or $ReleaseAuthority.Trim() -or $ReleaseAuthorityPublicKey.Trim()) {
    Write-Err '-LocalNonPublishable cannot accept a release tag or publication authority.'
    exit 1
  }
  if (-not [string]::IsNullOrWhiteSpace($WindowsSigningCert) -or
      -not [string]::IsNullOrWhiteSpace($WindowsSigningPassword) -or
      -not [string]::IsNullOrWhiteSpace($env:ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1)) {
    Write-Err '-LocalNonPublishable cannot accept signing credentials or a release signer pin.'
    exit 1
  }
  Assert-WindowsX64Host
  Require-Command 'node'
  $NpmCommand = Resolve-NpmCommand
  $NpxCommand = Resolve-NpxCommand
  $OutputRoot = if ($LocalNonPublishableOutput.Trim()) {
    [IO.Path]::GetFullPath($LocalNonPublishableOutput.Trim())
  } elseif ($env:ANALYTIX_LOCAL_NONPUBLISHABLE_WIN_DIST_DIR) {
    [IO.Path]::GetFullPath($env:ANALYTIX_LOCAL_NONPUBLISHABLE_WIN_DIST_DIR)
  } else {
    Join-Path $Root 'dist-local-nonpublishable-win'
  }
  $FormalDist = [IO.Path]::GetFullPath((Join-Path $Root 'dist'))
  $FormalWindowsDist = [IO.Path]::GetFullPath((Join-Path $Root 'dist-standard-win'))
  $Separator = [IO.Path]::DirectorySeparatorChar
  if ($OutputRoot -eq $FormalDist -or $OutputRoot.StartsWith("$FormalDist$Separator") -or
      $OutputRoot -eq $FormalWindowsDist -or $OutputRoot.StartsWith("$FormalWindowsDist$Separator")) {
    Write-Err 'Local non-publishable output must be isolated from formal release directories.'
    exit 1
  }
  if (Test-Path $OutputRoot) {
    Write-Err "Local non-publishable output already exists: $OutputRoot"
    exit 1
  }
  foreach ($name in @(
    'ANALYTIX_PACKAGED_REQUIRE_PUBLISHABLE', 'ANALYTIX_RELEASE_BUILD',
    'ANALYTIX_RELEASE_AUTHORITY', 'ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY', 'ANALYTIX_RELEASE_ENV',
    'WIN_CSC_LINK', 'CSC_LINK', 'ANALYTIX_WIN_SIGNING_CERT_PATH',
    'WIN_CSC_KEY_PASSWORD', 'CSC_KEY_PASSWORD', 'ANALYTIX_WIN_SIGNING_CERT_PASSWORD',
    'ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1', 'CARGO_TARGET_DIR'
  )) {
    Remove-Item "Env:$name" -ErrorAction SilentlyContinue
  }
  $env:ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA = '1'
  if ($NoCache) { $env:ANALYTIX_RELEASE_CACHE_DISABLE = '1' }
  Write-Info "Building explicitly non-publishable local Windows x64 app -> $OutputRoot"
  & node (Join-Path $Root 'scripts\build-data-analysis-native-tools.cjs') --development --platform win32 --arch x64
  if ($LASTEXITCODE -ne 0) {
    Write-Err 'Local non-publishable native development build failed.'
    exit 1
  }
  & $NpmCommand run build
  if ($LASTEXITCODE -ne 0) {
    Write-Err 'Local non-publishable renderer/main build failed.'
    exit 1
  }
  $env:ANALYTIX_DIST_DIR = $OutputRoot
  & $NpxCommand --yes electron-builder@26.15.3 --config electron-builder.config.cjs --publish never --win --dir --x64
  if ($LASTEXITCODE -ne 0) {
    Write-Err 'Local non-publishable Windows package failed.'
    exit 1
  }
  Write-Ok 'Local non-publishable Windows app is ready.'
  Write-Info "  Output: $OutputRoot"
  Write-Info '  Authority: development-only packaged marker; no release tag, metadata, installer/update feed, archive, upload, or publication receipt was created.'
  exit 0
}

if ($NoCache) {
  Write-Err '-NoCache is only valid with -LocalNonPublishable; formal publication consumes prebuilt authority-bound artifacts.'
  exit 1
}
$TagName = $Tag.Trim()
if ($TagName -notmatch '^v\d+\.\d+\.\d+$') {
  Write-Err 'Formal Windows publication requires -Tag vX.Y.Z; local metadata is not an authority.'
  exit 1
}
if ([string]::IsNullOrWhiteSpace($ReleaseAuthority)) {
  Write-Err 'release_publication_authority_missing'
  exit 1
}
if ([string]::IsNullOrWhiteSpace($ReleaseAuthorityPublicKey)) {
  Write-Err 'release_publication_authority_public_key_missing'
  exit 1
}
if ([string]::IsNullOrWhiteSpace($WindowsSigningCert) -or [string]::IsNullOrWhiteSpace($WindowsSigningPassword)) {
  Write-Err 'Official Windows release requires a signing certificate and password.'
  exit 1
}
if ($env:ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 -notmatch '^[A-Fa-f0-9]{40}$') {
  Write-Err 'Official Windows release requires ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 (40 hexadecimal characters).'
  exit 1
}
if (-not [string]::IsNullOrWhiteSpace($env:ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA)) {
  Write-Err 'Official Windows release forbids ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA.'
  exit 1
}

$ReleaseVersion = $TagName.Substring(1)
Assert-Semver $ReleaseVersion
Require-Command 'node'
Require-Command 'git'
$DistDir = Join-Path $Root 'dist-standard-win'

# This is intentionally the first formal-release operation. It is read-only
# and blocks before artifact cleanup/build, metadata/archive writes, or network.
& node (Join-Path $Root 'scripts\publish-r2.mjs') verify `
  --platform win `
  --tag $TagName `
  --channel $ReleaseChannel `
  --dist $DistDir `
  --authority $ReleaseAuthority `
  --public-key $ReleaseAuthorityPublicKey
if ($LASTEXITCODE -ne 0) {
  Write-Err 'Formal Windows publication authority preflight failed before build, tag, metadata, archive, or upload.'
  exit 1
}

Assert-WindowsX64Host
$env:ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1 = $env:ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1.ToUpperInvariant()
$env:WIN_CSC_LINK = $WindowsSigningCert
$env:CSC_LINK = $WindowsSigningCert
$env:ANALYTIX_WIN_SIGNING_CERT_PATH = $WindowsSigningCert
$env:WIN_CSC_KEY_PASSWORD = $WindowsSigningPassword
$env:CSC_KEY_PASSWORD = $WindowsSigningPassword
$env:ANALYTIX_WIN_SIGNING_CERT_PASSWORD = $WindowsSigningPassword
$env:ANALYTIX_APP_VERSION = $ReleaseVersion
$env:RELEASE_CHANNEL = $ReleaseChannel
$env:ANALYTIX_UPDATE_CHANNEL = $ReleaseChannel
Write-Info "Release tag: $TagName"
Write-Info "Release channel: $ReleaseChannel"
Write-Info "App version: $env:ANALYTIX_APP_VERSION"
Write-Info 'Using the exact prebuilt Windows artifact set bound by the controlled publication authority.'

Write-Info 'Running Windows package audit...'
& node (Join-Path $Root 'scripts\audit-windows-package.cjs') $ReleaseVersion
if ($LASTEXITCODE -ne 0) {
  Write-Err 'Windows package audit failed.'
  exit 1
}

$AssetSpecs = @(
  @{ Label = 'Standard Windows installer'; Filter = 'analytix-standard-*-x64.exe' },
  @{ Label = 'Standard Windows blockmap'; Filter = 'analytix-standard-*-x64.exe.blockmap' },
  @{ Label = 'Standard Windows update metadata'; Filter = 'latest.yml' }
)

$Assets = @()
foreach ($spec in $AssetSpecs) {
  $files = @(Get-ChildItem -Path $DistDir -Filter $spec.Filter -File -ErrorAction SilentlyContinue)
  if ($files.Count -eq 0) {
    Write-Err "Missing asset: $($spec.Label) ($($spec.Filter))"
    exit 1
  }
  foreach ($file in $files) {
    $Assets += $file.FullName
    Write-Ok "  ✓ $($spec.Label): $($file.Name)"
  }
}

$AssetListPath = Join-Path $DistDir '.release-assets.txt'
$Assets | Set-Content -Path $AssetListPath -Encoding UTF8
Write-Info "Asset list: $AssetListPath"

$AuditPath = Join-Path $Root 'validation-evidence\package-audit\package-audit.json'
Write-Info 'Archiving Standard Windows release assets...'
& (Join-Path $Root 'scripts\archive-standard-win-release.ps1') `
  -Version $ReleaseVersion `
  -Channel $ReleaseChannel `
  -DistDir $DistDir `
  -AuditPath $AuditPath `
  -DesktopLatest
if ($LASTEXITCODE -ne 0) {
  Write-Err 'Windows release archive failed.'
  exit 1
}

if ($R2 -or $PromoteR2) {
  Write-Info "Uploading Windows asset metadata to R2 ($TagName)..."
  & node (Join-Path $Root 'scripts\publish-r2.mjs') upload `
    --platform win `
    --tag $TagName `
    --channel $ReleaseChannel `
    --dist $DistDir `
    --authority $ReleaseAuthority `
    --public-key $ReleaseAuthorityPublicKey
  if ($LASTEXITCODE -ne 0) {
    Write-Err 'R2 upload failed for Windows assets.'
    exit 1
  }
}

if ($PromoteR2) {
  Write-Info "Promoting $TagName as R2 latest..."
  & node (Join-Path $Root 'scripts\publish-r2.mjs') promote `
    --tag $TagName `
    --channel $ReleaseChannel `
    --platforms win `
    --authority $ReleaseAuthority `
    --public-key $ReleaseAuthorityPublicKey
  if ($LASTEXITCODE -ne 0) {
    Write-Err 'R2 promote failed.'
    exit 1
  }
}

if ($Publish) {
  Write-Info '-Publish is a no-op for local releases. Use -PromoteR2 to update R2 latest metadata.'
}

Write-Ok "Windows release $TagName ready locally."
