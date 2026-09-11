param(
  [string]$AppName = "Notepad",
  [int]$InteractiveSessionId = 0,
  [string]$PsExecPath = ""
)

$ErrorActionPreference = "Stop"

function Step([string]$Name) {
  Write-Host ""
  Write-Host "== $Name ==" -ForegroundColor Cyan
}

function Invoke-Checked([string]$File, [string[]]$Arguments, [string]$Label) {
  & $File @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "$Label failed with exit code $LASTEXITCODE"
  }
}

function Quote-CmdArgument([string]$Value) {
  if ($Value -notmatch '[\s"&|<>^]') {
    return $Value
  }
  return '"' + ($Value -replace '"', '\"') + '"'
}

function Resolve-PsExecPath() {
  if ($PsExecPath -and (Test-Path $PsExecPath)) {
    return $PsExecPath
  }
  $Candidates = @(
    "C:\Tools\PSTools\PsExec64.exe",
    "C:\Users\analytix_remote\Projects\tools\pstools\PsExec64.exe",
    "C:\Sysinternals\PsExec64.exe"
  )
  foreach ($Candidate in $Candidates) {
    if (Test-Path $Candidate) {
      return $Candidate
    }
  }
  throw "PsExec64.exe not found. Pass -PsExecPath when using -InteractiveSessionId."
}

function Invoke-DesktopChecked([string]$File, [string[]]$Arguments, [string]$Label) {
  if ($InteractiveSessionId -le 0) {
    Invoke-Checked $File $Arguments $Label
    return
  }

  $PsExec = Resolve-PsExecPath
  $CommandLine = (@($File) + $Arguments | ForEach-Object { Quote-CmdArgument $_ }) -join " "
  & $PsExec -accepteula -i $InteractiveSessionId -s cmd /d /c $CommandLine
  if ($LASTEXITCODE -ne 0) {
    throw "$Label failed with exit code $LASTEXITCODE"
  }
}

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

Step "install dependency"
npm install

Step "build runtime"
npm run build:runtime

$OpenComputerUse = Join-Path $Root "node_modules\.bin\open-computer-use.cmd"
if (-not (Test-Path $OpenComputerUse)) {
  throw "open-computer-use.cmd not found at $OpenComputerUse"
}

if ($AppName -eq "Notepad") {
  if ($InteractiveSessionId -gt 0) {
    $PsExec = Resolve-PsExecPath
    & $PsExec -accepteula -i $InteractiveSessionId -s -d notepad.exe | Out-Host
  } else {
    Start-Process notepad | Out-Null
  }
  Start-Sleep -Seconds 2
}

Step "open-computer-use doctor"
Invoke-DesktopChecked $OpenComputerUse @("doctor") "open-computer-use doctor"

Step "open-computer-use list-apps"
Invoke-DesktopChecked $OpenComputerUse @("list-apps") "open-computer-use list-apps"

Step "open-computer-use snapshot"
Invoke-DesktopChecked $OpenComputerUse @("snapshot", $AppName) "open-computer-use snapshot"

Step "Go-only runtime boundary"
$RetiredToolProvider = Join-Path $Root "packages\runtime\dist\adapters\tool\computer-use-tool-provider.js"
if (Test-Path $RetiredToolProvider) {
  throw "retired TypeScript runtime tool provider should not be built: $RetiredToolProvider"
}

Write-Host ""
Write-Host "Windows computer_use smoke completed." -ForegroundColor Green
