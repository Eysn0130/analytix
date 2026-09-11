!ifndef BUILD_UNINSTALLER
Var /GLOBAL analytixInstallHandoffAfterSilentInstall
Var /GLOBAL analytixUpdateHandoffAfterSilentInstall
!endif

!macro customInit
  StrCpy $analytixInstallHandoffAfterSilentInstall "0"
  StrCpy $analytixUpdateHandoffAfterSilentInstall "0"
  ${ifNot} ${Silent}
    ${if} ${isUpdated}
      StrCpy $analytixUpdateHandoffAfterSilentInstall "1"
    ${else}
      StrCpy $analytixInstallHandoffAfterSilentInstall "1"
    ${endif}
    SetSilent silent
  ${endif}

  ${if} ${Silent}
  ${orIf} ${isUpdated}
    !ifdef INSTALL_MODE_PER_ALL_USERS_REQUIRED
      !insertmacro setInstallModePerAllUsers
    !endif
  ${endif}
!macroend

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

!macro customInstall
  !ifdef ONE_CLICK
    ${if} $analytixInstallHandoffAfterSilentInstall == "1"
    ${andIfNot} ${isUpdated}
      HideWindow
      ${StdUtils.ExecShellAsUser} $0 "$launchLink" "open" "--analytix-install-handoff"
    ${else}
      ${if} $analytixUpdateHandoffAfterSilentInstall == "1"
        HideWindow
        ${StdUtils.ExecShellAsUser} $0 "$launchLink" "open" "--updated"
      ${endif}
    ${endif}
  !endif
!macroend

!macro customFinishPage
  !ifndef HIDE_RUN_AFTER_FINISH
    Function StartApp
      ${if} ${isUpdated}
        StrCpy $1 "--updated"
      ${else}
        StrCpy $1 "--analytix-install-handoff"
      ${endif}
      ${StdUtils.ExecShellAsUser} $0 "$launchLink" "open" "$1"
    FunctionEnd

    !define MUI_FINISHPAGE_RUN
    !define MUI_FINISHPAGE_RUN_FUNCTION "StartApp"
  !endif
  !insertmacro MUI_PAGE_FINISH
!macroend
