!macro customCheckAppRunning
  Var /GLOBAL AnalytixInstallerCurrentPid
  Var /GLOBAL AnalytixInstallerStopAttempt
  Var /GLOBAL AnalytixInstallerStopResult

  ${if} $INSTDIR == ""
    Return
  ${endif}

  System::Call 'kernel32::GetCurrentProcessId() i .r0'
  StrCpy $AnalytixInstallerCurrentPid $0
  System::Call 'kernel32::SetEnvironmentVariable(t, t)i ("ANALYTIX_INSTALLER_APP_ROOT", "$INSTDIR").r0'
  System::Call 'kernel32::SetEnvironmentVariable(t, t)i ("ANALYTIX_INSTALLER_SELF_PID", "$AnalytixInstallerCurrentPid").r0'

  StrCpy $AnalytixInstallerStopAttempt 0

  AnalytixStopProcessesFromInstallDir:
    IntOp $AnalytixInstallerStopAttempt $AnalytixInstallerStopAttempt + 1
    DetailPrint "Checking for running ${PRODUCT_NAME} processes under $INSTDIR."
    nsExec::Exec `"$PowerShellPath" -NoProfile -ExecutionPolicy Bypass -Command "$$ErrorActionPreference='SilentlyContinue'; $$root=[System.IO.Path]::GetFullPath($$env:ANALYTIX_INSTALLER_APP_ROOT).TrimEnd('\'); $$prefix=$$root + '\'; $$self=[int]$$env:ANALYTIX_INSTALLER_SELF_PID; $$procs=@(Get-CimInstance -ClassName Win32_Process | Where-Object { $$path=$$_.ExecutablePath; if (-not $$path) { $$path=$$_.Path }; $$_.ProcessId -ne $$self -and $$path -and [System.IO.Path]::GetFullPath($$path).StartsWith($$prefix, [System.StringComparison]::OrdinalIgnoreCase) }); if ($$procs.Count -gt 0) { $$procs | ForEach-Object { Stop-Process -Id $$_.ProcessId -Force -ErrorAction SilentlyContinue }; exit 0 } else { exit 1 }"`
    Pop $AnalytixInstallerStopResult

    ${if} $AnalytixInstallerStopResult != 0
      Goto AnalytixInstallDirProcessesStopped
    ${endif}

    Sleep 1200
    ${if} $AnalytixInstallerStopAttempt <= 5
      Goto AnalytixStopProcessesFromInstallDir
    ${endif}

    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(appCannotBeClosed)" /SD IDCANCEL IDRETRY AnalytixStopProcessesFromInstallDir
    Quit

  AnalytixInstallDirProcessesStopped:
!macroend
