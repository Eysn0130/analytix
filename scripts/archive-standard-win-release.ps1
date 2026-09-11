param(
  [string]$Version = '',
  [ValidateSet('stable', 'beta')]
  [string]$Channel = 'stable',
  [string]$DistDir = '',
  [string]$AuditPath = '',
  [string]$ArchiveRoot = '',
  [switch]$DesktopLatest
)

$ErrorActionPreference = 'Stop'

function Write-Info([string]$Message) { Write-Host $Message -ForegroundColor Cyan }
function Write-Ok([string]$Message) { Write-Host $Message -ForegroundColor Green }
function Write-Err([string]$Message) { Write-Host "[ERROR] $Message" -ForegroundColor Red }

function Assert-File([string]$Path, [string]$Label) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "Missing $Label`: $Path"
  }
}

function Resolve-DesktopPath {
  $desktop = [Environment]::GetFolderPath([Environment+SpecialFolder]::DesktopDirectory)
  if ([string]::IsNullOrWhiteSpace($desktop)) {
    throw 'Could not resolve the current user Desktop directory.'
  }
  return $desktop
}

$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$CleanVersion = $Version.Trim()
if ($CleanVersion.StartsWith('v')) {
  $CleanVersion = $CleanVersion.Substring(1)
}
if ($CleanVersion -notmatch '^\d+\.\d+\.\d+$') {
  Write-Err "Version must be X.Y.Z or vX.Y.Z, got: $Version"
  exit 1
}

if ([string]::IsNullOrWhiteSpace($DistDir)) {
  $DistDir = Join-Path $Root 'dist-standard-win'
}
$DistDir = (Resolve-Path $DistDir).Path

if ([string]::IsNullOrWhiteSpace($AuditPath)) {
  $AuditPath = Join-Path $Root 'validation-evidence\package-audit\package-audit.json'
}

if ([string]::IsNullOrWhiteSpace($ArchiveRoot)) {
  if (-not [string]::IsNullOrWhiteSpace($env:ANALYTIX_WINDOWS_RELEASE_ARCHIVE_ROOT)) {
    $ArchiveRoot = $env:ANALYTIX_WINDOWS_RELEASE_ARCHIVE_ROOT
  } else {
    $ArchiveRoot = Join-Path (Resolve-DesktopPath) 'Analytix-Releases\win'
  }
}

$TagName = "v$CleanVersion"
$Destination = Join-Path (Join-Path $ArchiveRoot $Channel) $TagName
New-Item -ItemType Directory -Force -Path $Destination | Out-Null

$InstallerName = "analytix-standard-$CleanVersion-x64.exe"
$BlockmapName = "$InstallerName.blockmap"
$AuditName = "analytix-standard-$CleanVersion-package-audit.json"

$Assets = @(
  @{ Source = (Join-Path $DistDir $InstallerName); Target = $InstallerName; Label = 'installer' },
  @{ Source = (Join-Path $DistDir $BlockmapName); Target = $BlockmapName; Label = 'blockmap' },
  @{ Source = (Join-Path $DistDir 'latest.yml'); Target = 'latest.yml'; Label = 'latest.yml' },
  @{ Source = $AuditPath; Target = $AuditName; Label = 'package audit' }
)

foreach ($asset in $Assets) {
  Assert-File $asset.Source $asset.Label
  Copy-Item -Force -LiteralPath $asset.Source -Destination (Join-Path $Destination $asset.Target)
}

$HashLines = @()
foreach ($asset in $Assets) {
  $targetName = $asset.Target
  $targetPath = Join-Path $Destination $targetName
  $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $targetPath).Hash.ToLowerInvariant()
  $HashLines += "$hash  $targetName"
}
$HashLines | Set-Content -Encoding ASCII -Path (Join-Path $Destination 'SHA256SUMS.txt')

Write-Ok "Archived Standard Windows release: $Destination"
if ($DesktopLatest -or $env:ANALYTIX_WINDOWS_RELEASE_DESKTOP_LATEST -eq '1') {
  $desktopInstaller = Join-Path (Resolve-DesktopPath) $InstallerName
  Copy-Item -Force -LiteralPath (Join-Path $Destination $InstallerName) -Destination $desktopInstaller
  Write-Ok "Copied current Windows installer to Desktop: $desktopInstaller"
}
