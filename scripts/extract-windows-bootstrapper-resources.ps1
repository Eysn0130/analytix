param(
  [Parameter(Mandatory = $true)]
  [string] $Installer,
  [Parameter(Mandatory = $true)]
  [string] $OutputDirectory
)

$ErrorActionPreference = 'Stop'
$installerPath = [System.IO.Path]::GetFullPath($Installer)
$outputPath = [System.IO.Path]::GetFullPath($OutputDirectory)
if (-not [System.IO.File]::Exists($installerPath)) {
  throw "Bootstrapper installer not found: $installerPath"
}
[System.IO.Directory]::CreateDirectory($outputPath) | Out-Null
$assembly = [System.Reflection.Assembly]::LoadFile($installerPath)
$resources = @{
  'AnalytixPayloadArchive' = 'AnalytixPayload.7z'
  'PayloadExtractorExe' = '7za.exe'
  'AnalytixPayloadManifest' = 'AnalytixPayloadManifest.json'
}
foreach ($entry in $resources.GetEnumerator()) {
  $stream = $assembly.GetManifestResourceStream($entry.Key)
  if ($null -eq $stream) {
    throw "Bootstrapper resource is missing: $($entry.Key)"
  }
  try {
    $target = [System.IO.Path]::Combine($outputPath, $entry.Value)
    $file = [System.IO.File]::Create($target)
    try {
      $stream.CopyTo($file)
      $file.Flush($true)
    } finally {
      $file.Dispose()
    }
  } finally {
    $stream.Dispose()
  }
}
