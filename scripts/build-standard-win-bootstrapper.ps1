param(
  [string] $ProjectDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
  [string] $WebView2Version = "1.0.3967.48",
  [string] $WebView2RuntimeInstallerSource = $env:ANALYTIX_WEBVIEW2_RUNTIME_INSTALLER_SOURCE,
  [string] $WebView2RuntimeInstallerUrl = $env:ANALYTIX_WEBVIEW2_RUNTIME_INSTALLER_URL,
  [string] $WebView2RuntimeInstallerSha256 = $env:ANALYTIX_WEBVIEW2_RUNTIME_INSTALLER_SHA256,
  [string] $PrebuiltPayloadArchive = "",
  [string] $PrebuiltPayloadManifest = "",
  [string] $PrebuiltInstallerUiZip = ""
)

$ErrorActionPreference = "Stop"

$distDir = Join-Path $ProjectDir "dist-standard-win"
$packageJsonPath = Join-Path $ProjectDir "package.json"
$sourcePath = Join-Path $ProjectDir "installer\winforms-bootstrapper\AnalytixWinFormsBootstrapper.cs"
$winUnpackedDir = Join-Path $distDir "win-unpacked"
$standardInstallerUiDir = Join-Path $ProjectDir "build\standard-web-dist"
$rendererDistDir = Join-Path $ProjectDir "out\renderer"
$installerUiDir = if (Test-Path $standardInstallerUiDir) { $standardInstallerUiDir } else { $rendererDistDir }
$iconPath = Join-Path $ProjectDir "build\icon.ico"
$workDir = Join-Path $ProjectDir "build\winforms-bootstrapper"

if (-not (Test-Path $packageJsonPath)) {
  throw "package.json not found: $packageJsonPath"
}
if (-not (Test-Path $sourcePath)) {
  throw "WinForms bootstrapper source not found: $sourcePath"
}
if ([string]::IsNullOrWhiteSpace($PrebuiltPayloadArchive) -and -not (Test-Path $winUnpackedDir)) {
  throw "Standard Windows payload directory not found: $winUnpackedDir"
}
if (-not [string]::IsNullOrWhiteSpace($PrebuiltPayloadArchive) -or -not [string]::IsNullOrWhiteSpace($PrebuiltPayloadManifest)) {
  throw "Prebuilt payload archives are forbidden; the final manifest and archive must come from one immutable package snapshot."
}
if ([string]::IsNullOrWhiteSpace($PrebuiltInstallerUiZip) -and -not (Test-Path $installerUiDir)) {
  throw "Standard installer UI dist not found. Expected build\standard-web-dist or out\renderer."
}

$package = Get-Content -Raw -Path $packageJsonPath | ConvertFrom-Json
$version = [string] $env:ANALYTIX_APP_VERSION
if ([string]::IsNullOrWhiteSpace($version)) {
  $version = [string] $package.version
}
if ([string]::IsNullOrWhiteSpace($version)) {
  throw "Package version is empty: $packageJsonPath"
}
if ($version -notmatch '^\d+\.\d+\.\d+$') {
  throw "Bootstrapper version must be x.y.z: $version"
}

$fileVersion = $version
if ($fileVersion -notmatch '^\d+\.\d+\.\d+\.\d+$') {
  $fileVersion = "$version.0"
}
$versionCtor = (($fileVersion -split '\.') | ForEach-Object { [int] $_ }) -join ", "
$allowUnsignedQa = [string]$env:ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA -match '^(1|true|yes|on)$'
$expectedWindowsSignerSha1 = [string]$env:ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1
if (-not $allowUnsignedQa -and $expectedWindowsSignerSha1 -notmatch '^[A-Fa-f0-9]{40}$') {
  throw "Official bootstrapper build requires ANALYTIX_WINDOWS_EXPECTED_SIGNER_SHA1."
}
$expectedWindowsSignerSha1 = $expectedWindowsSignerSha1.ToUpperInvariant()

$csc = Join-Path $env:SystemRoot "Microsoft.NET\Framework64\v4.0.30319\csc.exe"
if (-not (Test-Path $csc)) {
  $csc = Join-Path $env:SystemRoot "Microsoft.NET\Framework\v4.0.30319\csc.exe"
}
if (-not (Test-Path $csc)) {
  throw "C# compiler not found. Expected .NET Framework csc.exe under $env:SystemRoot\Microsoft.NET."
}

function ConvertTo-ExtendedWindowsPath {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Path
  )

  $fullPath = [System.IO.Path]::GetFullPath($Path)
  if ($fullPath.StartsWith("\\?\")) {
    return $fullPath
  }
  if ($fullPath.StartsWith("\\")) {
    return "\\?\UNC\" + $fullPath.Substring(2)
  }
  return "\\?\" + $fullPath
}

function Remove-DirectoryTree {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Path
  )

  if (-not (Test-Path -LiteralPath $Path)) {
    return
  }

  try {
    Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop
  } catch {
    $extendedPath = ConvertTo-ExtendedWindowsPath $Path
    & cmd.exe /d /c "rmdir /s /q ""$extendedPath"""
    if ($LASTEXITCODE -ne 0 -and (Test-Path -LiteralPath $Path)) {
      throw
    }
  }
}

function Remove-BuildOnlyPayloadFiles {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Root
  )

  $removedCount = 0
  $moduleRoots = @(
    (Join-Path $Root "resources\app.asar.unpacked\packages\runtime\node_modules\@jimp"),
    (Join-Path $Root "resources\app.asar.unpacked\node_modules\@jimp")
  )
  foreach ($moduleRoot in $moduleRoots) {
    if (-not (Test-Path -LiteralPath $moduleRoot)) {
      continue
    }

    Get-ChildItem -LiteralPath $moduleRoot -Directory -Force -ErrorAction SilentlyContinue | ForEach-Object {
      foreach ($relativePath in @("src\__image_snapshots__", "src\__snapshots__")) {
        $snapshotDir = Join-Path $_.FullName $relativePath
        if (Test-Path -LiteralPath $snapshotDir) {
          Remove-DirectoryTree $snapshotDir
          $removedCount += 1
        }
      }
    }
  }

  if ($removedCount -gt 0) {
    Write-Host "[bootstrapper] removed $removedCount build-only snapshot directories from payload."
  }
}

function Remove-BuildMetadataFiles {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Root
  )

  if (-not (Test-Path -LiteralPath $Root)) {
    return
  }

  $removedCount = 0
  Get-ChildItem -LiteralPath $Root -Recurse -Force -File -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -eq ".DS_Store" -or $_.Name.StartsWith("._") } |
    ForEach-Object {
      Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue
      $removedCount += 1
    }

  if ($removedCount -gt 0) {
    Write-Host "[bootstrapper] removed $removedCount build metadata files from payload."
  }
}

function New-ZipFromDirectory {
  param(
    [Parameter(Mandatory = $true)]
    [string] $SourceDirectory,
    [Parameter(Mandatory = $true)]
    [string] $DestinationZip
  )

  $extendedSourceDirectory = ConvertTo-ExtendedWindowsPath $SourceDirectory
  $extendedDestinationZip = ConvertTo-ExtendedWindowsPath $DestinationZip
  [System.IO.Compression.ZipFile]::CreateFromDirectory(
    $extendedSourceDirectory,
    $extendedDestinationZip,
    [System.IO.Compression.CompressionLevel]::Optimal,
    $false
  )
}

function Resolve-SevenZipPath {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Root
  )

  $candidatePaths = @(
    (Join-Path $Root "node_modules\7zip-bin\win\x64\7za.exe"),
    (Join-Path $Root "runtime\7za.exe")
  )
  foreach ($candidatePath in $candidatePaths) {
    if (Test-Path -LiteralPath $candidatePath) {
      return $candidatePath
    }
  }

  throw "7za.exe not found. Run npm ci/build:backend-win-runtime or install 7zip-bin."
}

function New-SevenZipFromDirectory {
  param(
    [Parameter(Mandatory = $true)]
    [string] $SourceDirectory,
    [Parameter(Mandatory = $true)]
    [string] $DestinationArchive,
    [Parameter(Mandatory = $true)]
    [string] $SevenZip
  )

  $listPath = "$DestinationArchive.files.txt"
  Remove-Item -Force -ErrorAction SilentlyContinue $DestinationArchive, $listPath

  $sourceRoot = [System.IO.Path]::GetFullPath($SourceDirectory).TrimEnd('\')
  $entries = Get-ChildItem -LiteralPath $SourceDirectory -Recurse -Force -File -ErrorAction Stop |
    ForEach-Object {
      $_.FullName.Substring($sourceRoot.Length + 1).Replace('\', '/')
    } |
    Sort-Object
  [System.IO.File]::WriteAllLines($listPath, [string[]] $entries, (New-Object System.Text.UTF8Encoding($false)))

  Push-Location $SourceDirectory
  try {
    & $SevenZip "a" "-t7z" $DestinationArchive "@$listPath" "-scsUTF-8" "-mx=9" "-m0=LZMA2" "-md=128m" "-mfb=273" "-ms=on" "-mmt=on" "-bb0"
    if ($LASTEXITCODE -ne 0) {
      throw "7z payload archive creation failed with exit code $LASTEXITCODE."
    }
  } finally {
    Pop-Location
    Remove-Item -Force -ErrorAction SilentlyContinue $listPath
  }

  if (-not (Test-Path -LiteralPath $DestinationArchive) -or (Get-Item -LiteralPath $DestinationArchive).Length -le 0) {
    throw "7z payload archive was not created: $DestinationArchive"
  }
}

function Save-DownloadFile {
  param(
    [Parameter(Mandatory = $true)]
    [string[]] $Urls,
    [Parameter(Mandatory = $true)]
    [string] $Destination
  )

  $tempPath = "$Destination.download"
  $lastError = ""
  Remove-Item -Force -ErrorAction SilentlyContinue $tempPath
  foreach ($url in $Urls) {
    try {
      Write-Host "[bootstrapper] downloading $url"
      Invoke-WebRequest -Uri $url -OutFile $tempPath -UseBasicParsing
      $download = Get-Item -LiteralPath $tempPath -ErrorAction Stop
      if ($download.Length -gt 0) {
        Move-Item -Force $tempPath $Destination
        return
      }
      $lastError = "Downloaded file is empty: $url"
    } catch {
      $lastError = $_.Exception.Message
    } finally {
      Remove-Item -Force -ErrorAction SilentlyContinue $tempPath
    }
  }

  throw "Download failed for $Destination. Last error: $lastError"
}

function Resolve-AppBuilderPath {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Root
  )

  $candidatePaths = @(
    (Join-Path $Root "node_modules\app-builder-bin\win\x64\app-builder.exe"),
    (Join-Path $Root "node_modules\electron-builder\node_modules\app-builder-bin\win\x64\app-builder.exe"),
    (Join-Path $Root "node_modules\app-builder-lib\node_modules\app-builder-bin\win\x64\app-builder.exe")
  )
  foreach ($candidatePath in $candidatePaths) {
    if (Test-Path -LiteralPath $candidatePath) {
      return $candidatePath
    }
  }

  $cacheRoots = @()
  if (-not [string]::IsNullOrWhiteSpace($env:npm_config_cache)) {
    $cacheRoots += [string] $env:npm_config_cache
  }
  try {
    $npmCache = (& npm config get cache 2>$null | Select-Object -First 1)
    if (-not [string]::IsNullOrWhiteSpace($npmCache)) {
      $cacheRoots += [string] $npmCache
    }
  } catch {
  }

  foreach ($cacheRoot in ($cacheRoots | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Unique)) {
    $npxRoot = Join-Path $cacheRoot "_npx"
    if (-not (Test-Path -LiteralPath $npxRoot)) {
      continue
    }

    $cachedCandidate = Get-ChildItem -LiteralPath $npxRoot -Directory -Force -ErrorAction SilentlyContinue |
      Sort-Object LastWriteTime -Descending |
      ForEach-Object { Join-Path $_.FullName "node_modules\app-builder-bin\win\x64\app-builder.exe" } |
      Where-Object { Test-Path -LiteralPath $_ } |
      Select-Object -First 1
    if (-not [string]::IsNullOrWhiteSpace($cachedCandidate)) {
      return [string] $cachedCandidate
    }
  }

  throw "WinForms bootstrapper build dependency missing: app-builder.exe. Expected app-builder-bin under project node_modules or npm _npx cache after electron-builder runs."
}

function Resolve-FirstValue {
  param(
    [AllowNull()]
    [object[]] $Values
  )

  if ($null -eq $Values) {
    return ""
  }
  foreach ($value in $Values) {
    if ($null -ne $value -and -not [string]::IsNullOrWhiteSpace([string] $value)) {
      return [string] $value
    }
  }
  return ""
}

function Resolve-WebView2RuntimeInstaller {
  param(
    [Parameter(Mandatory = $true)]
    [string] $WorkDir,
    [AllowNull()]
    [string] $Source,
    [AllowNull()]
    [string] $Url,
    [AllowNull()]
    [string] $ExpectedSha256
  )

  $targetPath = Join-Path $WorkDir "MicrosoftEdgeWebView2RuntimeInstallerX64.exe"
  $sourceDescription = ""
  if (-not [string]::IsNullOrWhiteSpace($Source)) {
    if (-not (Test-Path -LiteralPath $Source)) {
      throw "ANALYTIX_WEBVIEW2_RUNTIME_INSTALLER_SOURCE does not exist: $Source"
    }
    Copy-Item -Force -LiteralPath $Source -Destination $targetPath
    $sourceDescription = "local-source:$Source"
  } else {
    if ([string]::IsNullOrWhiteSpace($Url)) {
      $Url = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
    }
    Save-DownloadFile -Urls @($Url) -Destination $targetPath
    $sourceDescription = "url:$Url"
  }

  if (-not (Test-Path -LiteralPath $targetPath)) {
    throw "WebView2 Evergreen Runtime installer was not staged: $targetPath"
  }
  $actualSha256 = (Get-FileHash -Algorithm SHA256 $targetPath).Hash.ToLowerInvariant()
  if (-not [string]::IsNullOrWhiteSpace($ExpectedSha256) -and $actualSha256 -ne $ExpectedSha256.ToLowerInvariant()) {
    throw "WebView2 Evergreen Runtime installer SHA256 mismatch. expected=$ExpectedSha256 actual=$actualSha256"
  }

  $manifestPath = Join-Path $WorkDir "analytix-webview2-runtime-manifest.json"
  [ordered] @{
    schemaVersion = 1
    source = $sourceDescription
    fileName = [System.IO.Path]::GetFileName($targetPath)
    sha256 = $actualSha256
    generatedAt = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ss.fffZ")
  } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $manifestPath -Encoding UTF8
  Write-Host "[bootstrapper] webview2RuntimeInstaller=$targetPath"
  Write-Host "[bootstrapper] webview2RuntimeSha256=$actualSha256"
  return $targetPath
}

function Resolve-SigntoolPath {
  $explicitPath = Resolve-FirstValue @($env:ANALYTIX_WIN_SIGNTOOL_PATH)
  if (-not [string]::IsNullOrWhiteSpace($explicitPath)) {
    if (Test-Path -LiteralPath $explicitPath) {
      return $explicitPath
    }
    throw "ANALYTIX_WIN_SIGNTOOL_PATH does not exist: $explicitPath"
  }

  $command = Get-Command "signtool.exe" -ErrorAction SilentlyContinue
  if ($command -and -not [string]::IsNullOrWhiteSpace($command.Source)) {
    return [string] $command.Source
  }

  $kitsRoot = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"
  if (Test-Path -LiteralPath $kitsRoot) {
    $candidate = Get-ChildItem -LiteralPath $kitsRoot -Directory -Force -ErrorAction SilentlyContinue |
      Sort-Object Name -Descending |
      ForEach-Object { Join-Path $_.FullName "x64\signtool.exe" } |
      Where-Object { Test-Path -LiteralPath $_ } |
      Select-Object -First 1
    if (-not [string]::IsNullOrWhiteSpace($candidate)) {
      return [string] $candidate
    }
  }

  throw "signtool.exe not found. Run from a Visual Studio developer prompt or set ANALYTIX_WIN_SIGNTOOL_PATH."
}

function Resolve-SigningCertificatePath {
  param(
    [Parameter(Mandatory = $true)]
    [string] $WorkDir
  )

  $certificate = Resolve-FirstValue @(
    $env:ANALYTIX_WIN_SIGNING_CERT_PATH,
    $env:WIN_CSC_LINK,
    $env:CSC_LINK
  )
  if ([string]::IsNullOrWhiteSpace($certificate)) {
    return ""
  }

  if ($certificate.StartsWith("file://")) {
    $certificate = ([System.Uri] $certificate).LocalPath
  }
  if (Test-Path -LiteralPath $certificate) {
    return $certificate
  }
  if ($certificate -match '^https?://') {
    throw "Bootstrapper signing certificate URLs are not supported. Set ANALYTIX_WIN_SIGNING_CERT_PATH, WIN_CSC_LINK, or CSC_LINK to a local PFX path or base64 PFX."
  }

  try {
    $decoded = [Convert]::FromBase64String($certificate)
    $certificatePath = Join-Path $WorkDir "AnalytixBootstrapperSigningCert.pfx"
    [System.IO.File]::WriteAllBytes($certificatePath, $decoded)
    return $certificatePath
  } catch {
    throw "Bootstrapper signing certificate was provided but is neither an existing file path nor valid base64 PFX data."
  }
}

function Sign-BootstrapperIfConfigured {
  param(
    [Parameter(Mandatory = $true)]
    [string] $Path,
    [Parameter(Mandatory = $true)]
    [string] $WorkDir
  )

  $certificatePath = Resolve-SigningCertificatePath $WorkDir
  if ([string]::IsNullOrWhiteSpace($certificatePath)) {
    if (-not $allowUnsignedQa) {
      throw "Windows signing certificate is required unless ANALYTIX_ALLOW_UNSIGNED_WINDOWS_QA=1."
    }
    Write-Host "[bootstrapper] signing=NotSigned; no Windows signing certificate was configured."
    return "NotSigned"
  }

  $password = Resolve-FirstValue @(
    $env:ANALYTIX_WIN_SIGNING_CERT_PASSWORD,
    $env:WIN_CSC_KEY_PASSWORD,
    $env:CSC_KEY_PASSWORD
  )
  $timestampServer = Resolve-FirstValue @(
    $env:ANALYTIX_WIN_TIMESTAMP_URL,
    "http://timestamp.digicert.com"
  )
  $signtool = Resolve-SigntoolPath
  $signArgs = @("sign", "/fd", "SHA256", "/td", "SHA256")
  if (-not [string]::IsNullOrWhiteSpace($timestampServer)) {
    $signArgs += @("/tr", $timestampServer)
  }
  $signArgs += @("/f", $certificatePath)
  if (-not [string]::IsNullOrWhiteSpace($password)) {
    $signArgs += @("/p", $password)
  }
  $signArgs += $Path

  & $signtool @signArgs | ForEach-Object { Write-Host $_ }
  if ($LASTEXITCODE -ne 0) {
    throw "WinForms bootstrapper signing failed with exit code $LASTEXITCODE."
  }
  return "Signed"
}

New-Item -ItemType Directory -Force -Path $workDir | Out-Null
$payloadArchive = Join-Path $workDir "AnalytixPayload.7z"
$payloadManifest = Join-Path $workDir "AnalytixPayloadManifest.json"
$installerUiZip = Join-Path $workDir "AnalytixInstallerUiZip.zip"
$payloadExtractor = Join-Path $workDir "7za.exe"
$payloadSourceDir = Join-Path $workDir "payload-source"
$payloadVerificationDir = Join-Path $workDir "payload-verification"
$generatedSourcePath = Join-Path $workDir "AnalytixWinFormsBootstrapper.generated.cs"
$webView2Package = Join-Path $workDir ("Microsoft.Web.WebView2." + $WebView2Version + ".nupkg")
$webView2PackageZip = Join-Path $workDir ("Microsoft.Web.WebView2." + $WebView2Version + ".zip")
$webView2Dir = Join-Path $workDir ("Microsoft.Web.WebView2." + $WebView2Version)
$outputPath = Join-Path $distDir ("analytix-standard-" + $version + "-x64.exe")
$nsisBackupPath = Join-Path $distDir ("analytix-standard-" + $version + "-x64.nsis-payload.exe")

Add-Type -AssemblyName System.IO.Compression.FileSystem
$sevenZip = Resolve-SevenZipPath $ProjectDir
Copy-Item -Force -LiteralPath $sevenZip -Destination $payloadExtractor
Sign-BootstrapperIfConfigured -Path $payloadExtractor -WorkDir $workDir | Out-Null
Remove-Item -Force -ErrorAction SilentlyContinue $payloadArchive, $payloadManifest, $installerUiZip, (Join-Path $workDir "AnalytixPayloadZip.zip")
Remove-DirectoryTree $payloadSourceDir
Remove-DirectoryTree $payloadVerificationDir
$reparseEntries = @(Get-ChildItem -LiteralPath $winUnpackedDir -Recurse -Force -ErrorAction Stop | Where-Object {
  ($_.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
})
if ($reparseEntries.Count -gt 0) {
  throw "Windows package snapshot contains a reparse point: $($reparseEntries[0].FullName)"
}
New-Item -ItemType Directory -Force -Path $payloadSourceDir | Out-Null
Get-ChildItem -LiteralPath $winUnpackedDir -Force -ErrorAction Stop |
  Copy-Item -Destination $payloadSourceDir -Recurse -Force -ErrorAction Stop
Remove-BuildOnlyPayloadFiles $payloadSourceDir
Remove-BuildMetadataFiles $payloadSourceDir
New-SevenZipFromDirectory $payloadSourceDir $payloadArchive $payloadExtractor
& node (Join-Path $ProjectDir "scripts\windows-payload-contract.cjs") create `
  --root $payloadSourceDir `
  --archive $payloadArchive `
  --extractor $payloadExtractor `
  --output $payloadManifest
if ($LASTEXITCODE -ne 0) {
  throw "Windows payload manifest creation failed with exit code $LASTEXITCODE."
}
New-Item -ItemType Directory -Force -Path $payloadVerificationDir | Out-Null
& $payloadExtractor "t" "-bd" "-bb0" $payloadArchive
if ($LASTEXITCODE -ne 0) {
  throw "Windows payload archive test failed with exit code $LASTEXITCODE."
}
& $payloadExtractor "x" "-y" "-bd" "-bb0" $payloadArchive "-o$payloadVerificationDir"
if ($LASTEXITCODE -ne 0) {
  throw "Windows payload archive verification extraction failed with exit code $LASTEXITCODE."
}
& node (Join-Path $ProjectDir "scripts\windows-payload-contract.cjs") verify `
  --manifest $payloadManifest `
  --root $payloadVerificationDir `
  --archive $payloadArchive `
  --extractor $payloadExtractor
if ($LASTEXITCODE -ne 0) {
  throw "Windows payload manifest verification failed with exit code $LASTEXITCODE."
}
Remove-DirectoryTree $payloadVerificationDir
if ([string]::IsNullOrWhiteSpace($PrebuiltInstallerUiZip)) {
  Remove-BuildMetadataFiles $installerUiDir
  New-ZipFromDirectory $installerUiDir $installerUiZip
} else {
  if (-not (Test-Path $PrebuiltInstallerUiZip)) {
    throw "Prebuilt installer UI zip not found: $PrebuiltInstallerUiZip"
  }
  Copy-Item -Force $PrebuiltInstallerUiZip $installerUiZip
}

if (-not (Test-Path $webView2Package)) {
  $webView2VersionLower = $WebView2Version.ToLowerInvariant()
  $webView2Urls = @(
    "https://api.nuget.org/v3-flatcontainer/microsoft.web.webview2/$webView2VersionLower/microsoft.web.webview2.$webView2VersionLower.nupkg",
    "https://www.nuget.org/api/v2/package/Microsoft.Web.WebView2/$WebView2Version"
  )
  Save-DownloadFile -Urls $webView2Urls -Destination $webView2Package
} elseif ((Get-Item -LiteralPath $webView2Package).Length -le 0) {
  Remove-Item -Force -ErrorAction SilentlyContinue $webView2Package
  $webView2VersionLower = $WebView2Version.ToLowerInvariant()
  $webView2Urls = @(
    "https://api.nuget.org/v3-flatcontainer/microsoft.web.webview2/$webView2VersionLower/microsoft.web.webview2.$webView2VersionLower.nupkg",
    "https://www.nuget.org/api/v2/package/Microsoft.Web.WebView2/$WebView2Version"
  )
  Save-DownloadFile -Urls $webView2Urls -Destination $webView2Package
}
if (-not (Test-Path $webView2Dir)) {
  New-Item -ItemType Directory -Force -Path $webView2Dir | Out-Null
  Copy-Item -Force $webView2Package $webView2PackageZip
  Expand-Archive -Path $webView2PackageZip -DestinationPath $webView2Dir -Force
}

$webView2Core = Join-Path $webView2Dir "lib\net462\Microsoft.Web.WebView2.Core.dll"
$webView2WinForms = Join-Path $webView2Dir "lib\net462\Microsoft.Web.WebView2.WinForms.dll"
$webView2Loader = Join-Path $webView2Dir "runtimes\win-x64\native\WebView2Loader.dll"
$webView2RuntimeInstaller = Resolve-WebView2RuntimeInstaller `
  -WorkDir $workDir `
  -Source $WebView2RuntimeInstallerSource `
  -Url $WebView2RuntimeInstallerUrl `
  -ExpectedSha256 $WebView2RuntimeInstallerSha256
$appBuilder = Resolve-AppBuilderPath $ProjectDir
foreach ($required in @($webView2Core, $webView2WinForms, $webView2Loader)) {
  if (-not (Test-Path $required)) {
    throw "WinForms bootstrapper build dependency missing: $required"
  }
}

$sourceText = Get-Content -Raw -Encoding UTF8 -Path $sourcePath
$sourceText = [regex]::Replace($sourceText, '\[assembly:\s*AssemblyFileVersion\("[^"]*"\)\]', "[assembly: AssemblyFileVersion(`"$fileVersion`")]")
$sourceText = [regex]::Replace($sourceText, '\[assembly:\s*AssemblyInformationalVersion\("[^"]*"\)\]', "[assembly: AssemblyInformationalVersion(`"$version`")]")
$sourceText = [regex]::Replace($sourceText, '\[assembly:\s*AssemblyVersion\("[^"]*"\)\]', "[assembly: AssemblyVersion(`"$fileVersion`")]")
$sourceText = [regex]::Replace($sourceText, 'public\s+const\s+string\s+CurrentVersionText\s*=\s*"[^"]*";', "public const string CurrentVersionText = `"$version`";")
$sourceText = [regex]::Replace($sourceText, 'private\s+static\s+readonly\s+Version\s+CurrentPackageVersion\s*=\s*new\s+Version\([^)]+\);', "private static readonly Version CurrentPackageVersion = new Version($versionCtor);")
$sourceText = $sourceText.Replace('ANALYTIX_EXPECTED_WINDOWS_SIGNER_SHA1', $expectedWindowsSignerSha1)
$sourceText = [regex]::Replace($sourceText, 'private\s+const\s+bool\s+AllowUnsignedWindowsQa\s*=\s*(?:true|false);', "private const bool AllowUnsignedWindowsQa = $($allowUnsignedQa.ToString().ToLowerInvariant());")
$sourceText = [regex]::Replace($sourceText, "version:\s*'winforms-webview2-bootstrapper-[^']*'", "version: 'winforms-webview2-bootstrapper-$version'")
[System.IO.File]::WriteAllText($generatedSourcePath, $sourceText, (New-Object System.Text.UTF8Encoding($false)))

if (Test-Path $outputPath) {
  Copy-Item -Force $outputPath $nsisBackupPath
}

$references = @(
  "System.dll",
  "System.Core.dll",
  "System.Drawing.dll",
  "System.Windows.Forms.dll",
  "System.IO.Compression.dll",
  "System.IO.Compression.FileSystem.dll",
  "System.Web.Extensions.dll",
  $webView2Core,
  $webView2WinForms
)

$args = @(
  "/nologo",
  "/target:winexe",
  "/platform:x64",
  "/optimize+",
  "/define:ANALYTIX_WEBVIEW2_INSTALLER",
  "/out:$outputPath",
  "/resource:$payloadArchive,AnalytixPayloadArchive",
  "/resource:$payloadExtractor,PayloadExtractorExe",
  "/resource:$payloadManifest,AnalytixPayloadManifest",
  "/resource:$installerUiZip,AnalytixInstallerUiZip",
  "/resource:$webView2Core,WebView2CoreDll",
  "/resource:$webView2WinForms,WebView2WinFormsDll",
  "/resource:$webView2Loader,WebView2LoaderDll",
  "/resource:$webView2RuntimeInstaller,WebView2RuntimeInstaller"
)
if (Test-Path $iconPath) {
  $args += "/win32icon:$iconPath"
}
foreach ($reference in $references) {
  $args += "/reference:$reference"
}
$args += $generatedSourcePath

& $csc @args
if ($LASTEXITCODE -ne 0) {
  throw "WinForms bootstrapper compilation failed with exit code $LASTEXITCODE."
}
$signingStatus = Sign-BootstrapperIfConfigured $outputPath $workDir

$blockmapPath = "$outputPath.blockmap"
Remove-Item -Force -ErrorAction SilentlyContinue $blockmapPath
& $appBuilder "blockmap" "--input" $outputPath "--output" $blockmapPath
if ($LASTEXITCODE -ne 0) {
  throw "WinForms bootstrapper blockmap generation failed with exit code $LASTEXITCODE."
}

$bytes = [System.IO.File]::ReadAllBytes($outputPath)
$sha512 = [Convert]::ToBase64String([System.Security.Cryptography.SHA512]::Create().ComputeHash($bytes))
$size = $bytes.Length
$releaseDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ss.fffZ")
$latestYmlPath = Join-Path $distDir "latest.yml"
$latest = @"
version: $version
files:
  - url: analytix-standard-$version-x64.exe
    sha512: $sha512
    size: $size
    isAdminRightsRequired: true
path: analytix-standard-$version-x64.exe
sha512: $sha512
releaseDate: '$releaseDate'
"@
[System.IO.File]::WriteAllText($latestYmlPath, $latest, (New-Object System.Text.UTF8Encoding($false)))

$sha256 = (Get-FileHash -Algorithm SHA256 $outputPath).Hash
$payloadArchiveSize = (Get-Item -LiteralPath $payloadArchive).Length
Write-Host "[bootstrapper] output=$outputPath"
Write-Host "[bootstrapper] signing=$signingStatus"
Write-Host "[bootstrapper] sha256=$sha256"
Write-Host "[bootstrapper] size=$size"
Write-Host "[bootstrapper] payloadArchive=$payloadArchive"
Write-Host "[bootstrapper] payloadArchiveSize=$payloadArchiveSize"
Write-Host "[bootstrapper] latest=$latestYmlPath"

if ((Test-Path -LiteralPath $nsisBackupPath) -and $env:ANALYTIX_KEEP_NSIS_PAYLOAD -ne "1") {
  Remove-Item -Force -LiteralPath $nsisBackupPath
  Write-Host "[bootstrapper] removedNsisPayload=$nsisBackupPath"
}
