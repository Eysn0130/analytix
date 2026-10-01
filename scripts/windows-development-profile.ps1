param(
  [Parameter(Mandatory = $true)][string]$Directory,
  [switch]$Create
)

$ErrorActionPreference = 'Stop'
if (-not [System.IO.Path]::IsPathRooted($Directory) -or
    [System.IO.Path]::GetFullPath($Directory) -cne $Directory -or
    $Directory.StartsWith('\\')) {
  throw 'Development profile path is not a canonical local Windows path.'
}

$parent = [System.IO.Path]::GetDirectoryName($Directory)
if (-not [System.IO.Directory]::Exists($parent)) {
  throw 'Development profile parent is missing.'
}
if (([System.IO.File]::GetAttributes($parent) -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
  throw 'Development profile parent is a reparse point.'
}

$owner = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
if ($null -eq $owner) { throw 'Development profile owner is unavailable.' }
$ownerSid = $owner.Value
if ($Create -and -not [System.IO.Directory]::Exists($Directory)) {
  $security = New-Object System.Security.AccessControl.DirectorySecurity
  $sddl = 'O:' + $ownerSid + 'D:P(A;OICI;FA;;;' + $ownerSid + ')(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)'
  $security.SetSecurityDescriptorSddlForm($sddl)
  [System.IO.Directory]::CreateDirectory($Directory, $security) | Out-Null
}
if (-not [System.IO.Directory]::Exists($Directory)) {
  throw 'Development profile directory is missing.'
}
$item = Get-Item -LiteralPath $Directory -Force
if (-not $item.PSIsContainer -or ($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
  throw 'Development profile directory is a reparse point or non-directory.'
}
$actual = [System.IO.Directory]::GetAccessControl($Directory)
if (-not $actual.GetOwner([System.Security.Principal.SecurityIdentifier]).Equals($owner) -or
    -not $actual.AreAccessRulesProtected) {
  throw 'Development profile owner or inherited ACL is unsafe.'
}
$allowed = @($ownerSid, 'S-1-5-18', 'S-1-5-32-544')
$rules = $actual.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier])
$ownerAllowed = $false
foreach ($rule in $rules) {
  if ($rule.AccessControlType -eq [System.Security.AccessControl.AccessControlType]::Deny) { continue }
  $sid = $rule.IdentityReference.Value
  if ($allowed -cnotcontains $sid) { throw 'Development profile ACL grants access outside owner and machine authorities.' }
  if ($sid -ceq $ownerSid) { $ownerAllowed = $true }
}
if (-not $ownerAllowed) { throw 'Development profile ACL does not admit the owner.' }

$exact = @([System.IO.Directory]::EnumerateFileSystemEntries($parent) |
  Where-Object { [System.IO.Path]::GetFileName($_) -ieq [System.IO.Path]::GetFileName($Directory) })
if ($exact.Count -ne 1 -or $exact[0] -cne $Directory) {
  throw 'Development profile path has a case alias.'
}
