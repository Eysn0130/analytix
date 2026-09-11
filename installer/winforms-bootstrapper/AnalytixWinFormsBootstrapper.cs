using Microsoft.Win32;
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.Globalization;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
#if ANALYTIX_WEBVIEW2_INSTALLER
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.WinForms;
using System.Web.Script.Serialization;
#endif

[assembly: AssemblyTitle("Analytix Standard Installer")]
[assembly: AssemblyProduct("Analytix")]
[assembly: AssemblyCompany("Analytix Team")]
[assembly: AssemblyFileVersion("1.0.6.0")]
[assembly: AssemblyInformationalVersion("1.0.6")]
[assembly: AssemblyVersion("1.0.6.0")]

namespace AnalytixBootstrapper
{
    internal static class Program
    {
        private const string ProductName = "analytix";
        private const string PayloadResourceName = "AnalytixPayloadArchive";
        private const string InstallerUiResourceName = "AnalytixInstallerUiZip";

        [STAThread]
        private static int Main(string[] args)
        {
            try
            {
                DpiAwareness.EnableForInstaller();
                var options = BootstrapperOptions.Parse(args);
                if (options.InstallSilent)
                {
                    InstallerOperations.Install(options.InstallPath, PayloadResourceName, options.LaunchAfterInstall);
                    return 0;
                }
                if (options.UninstallSilent)
                {
                    InstallerOperations.Uninstall(options.InstallPath);
                    return 0;
                }

                Application.EnableVisualStyles();
                Application.SetCompatibleTextRenderingDefault(false);
#if ANALYTIX_WEBVIEW2_INSTALLER
                BootstrapperRuntime.Initialize();
                Application.Run(new ReactInstallerForm(PayloadResourceName, InstallerUiResourceName));
#else
#error Analytix Windows installer must be compiled with ANALYTIX_WEBVIEW2_INSTALLER. The legacy WinForms fallback is not a release path.
#endif
                return 0;
            }
            catch (Exception error)
            {
                try
                {
                    File.AppendAllText(
                        Path.Combine(Path.GetTempPath(), "analytix-bootstrapper-error.log"),
                        DateTime.Now.ToString("o") + " " + error + Environment.NewLine
                    );
                }
                catch
                {
                    // Best-effort diagnostic only.
                }
                var options = BootstrapperOptions.Parse(args);
                if (!options.IsSilent)
                {
                    MessageBox.Show(error.Message, "analytix 安装", MessageBoxButtons.OK, MessageBoxIcon.Error);
                }
                return 1;
            }
        }

        internal static string DefaultInstallPath()
        {
            var programFiles = Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles);
            if (string.IsNullOrWhiteSpace(programFiles))
            {
                programFiles = @"C:\Program Files";
            }
            return Path.Combine(programFiles, "Analytix", "analytix");
        }
    }

    internal static class DpiAwareness
    {
        private static readonly IntPtr DpiAwarenessContextPerMonitorAwareV2 = new IntPtr(-4);

        [DllImport("user32.dll", EntryPoint = "SetProcessDpiAwarenessContext")]
        private static extern bool SetProcessDpiAwarenessContext(IntPtr dpiContext);

        [DllImport("user32.dll", EntryPoint = "SetProcessDPIAware")]
        private static extern bool SetProcessDPIAware();

        [DllImport("user32.dll", EntryPoint = "GetDpiForWindow")]
        private static extern uint GetDpiForWindow(IntPtr windowHandle);

        public static void EnableForInstaller()
        {
            try
            {
                if (SetProcessDpiAwarenessContext(DpiAwarenessContextPerMonitorAwareV2))
                {
                    return;
                }
            }
            catch
            {
                // Older Windows builds do not expose per-monitor v2 awareness.
            }

            try
            {
                SetProcessDPIAware();
            }
            catch
            {
                // Best effort: the window clamp still protects the installer on default DPI.
            }
        }

        public static double ResolveWebViewZoomFactor(IntPtr windowHandle)
        {
            try
            {
                var dpi = GetDpiForWindow(windowHandle);
                if (dpi > 96)
                {
                    return Math.Max(0.62, Math.Min(1.0, 96.0 / dpi));
                }
            }
            catch
            {
                // DPI probing is best-effort; the default WebView2 zoom is correct on 100% displays.
            }
            return 1.0;
        }
    }

    internal sealed class BootstrapperOptions
    {
        public bool InstallSilent { get; private set; }
        public bool UninstallSilent { get; private set; }
        public bool LaunchAfterInstall { get; private set; }
        public bool ElectronUpdaterSilent { get; private set; }
        public bool IsSilent { get { return InstallSilent || UninstallSilent || ElectronUpdaterSilent; } }
        public string InstallPath { get; private set; }

        public static BootstrapperOptions Parse(string[] args)
        {
            var result = new BootstrapperOptions
            {
                InstallPath = Program.DefaultInstallPath(),
                LaunchAfterInstall = true
            };
            var installPathExplicit = false;
            for (var index = 0; index < args.Length; index++)
            {
                var arg = args[index] ?? string.Empty;
                if (EqualsArg(arg, "--install-silent") || EqualsArg(arg, "/S"))
                {
                    result.InstallSilent = true;
                    result.LaunchAfterInstall = false;
                    continue;
                }
                if (IsElectronUpdaterInstallArgument(arg))
                {
                    result.ElectronUpdaterSilent = true;
                    continue;
                }
                if (EqualsArg(arg, "--uninstall-silent") || EqualsArg(arg, "/uninstall"))
                {
                    result.UninstallSilent = true;
                    continue;
                }
                if (EqualsArg(arg, "--no-launch"))
                {
                    result.LaunchAfterInstall = false;
                    continue;
                }
                if (arg.StartsWith("/D=", StringComparison.OrdinalIgnoreCase))
                {
                    result.InstallPath = arg.Substring(3).Trim('"');
                    installPathExplicit = true;
                    continue;
                }
                if ((EqualsArg(arg, "--path") || EqualsArg(arg, "/D")) && index + 1 < args.Length)
                {
                    result.InstallPath = ReadPathArgument(args, ref index);
                    installPathExplicit = true;
                }
            }
            if (result.ElectronUpdaterSilent)
            {
                result.InstallSilent = true;
                result.LaunchAfterInstall = true;
                if (!installPathExplicit)
                {
                    result.InstallPath = InstallerOperations.ResolveInstalledPath();
                }
            }
            if (string.IsNullOrWhiteSpace(result.InstallPath))
            {
                result.InstallPath = Program.DefaultInstallPath();
            }
            return result;
        }

        private static bool EqualsArg(string left, string right)
        {
            return string.Equals(left, right, StringComparison.OrdinalIgnoreCase);
        }

        private static bool IsElectronUpdaterInstallArgument(string arg)
        {
            return EqualsArg(arg, "--updated") ||
                EqualsArg(arg, "--force-run") ||
                EqualsArg(arg, "--squirrel-firstrun") ||
                EqualsArg(arg, "--squirrel-updated") ||
                EqualsArg(arg, "--squirrel-install");
        }

        private static string ReadPathArgument(string[] args, ref int index)
        {
            index++;
            var parts = new List<string>();
            while (index < args.Length)
            {
                var value = args[index] ?? string.Empty;
                if (parts.Count > 0 && IsOptionToken(value))
                {
                    index--;
                    break;
                }
                parts.Add(value.Trim('"'));
                index++;
            }
            return string.Join(" ", parts.ToArray()).Trim();
        }

        private static bool IsOptionToken(string value)
        {
            if (string.IsNullOrWhiteSpace(value))
            {
                return false;
            }
            if (value.Length >= 3 && char.IsLetter(value[0]) && value[1] == ':' && (value[2] == '\\' || value[2] == '/'))
            {
                return false;
            }
            return value.StartsWith("--", StringComparison.Ordinal) || value.StartsWith("/", StringComparison.Ordinal);
        }
    }

    internal static class InstallerOperations
    {
        private sealed class PayloadBlobRecord
        {
            public long size { get; set; }
            public string sha256 { get; set; }
        }

        private sealed class PayloadFileRecord
        {
            public string path { get; set; }
            public long size { get; set; }
            public string sha256 { get; set; }
        }

        private sealed class PayloadManifest
        {
            public int schemaVersion { get; set; }
            public string treeAlgorithm { get; set; }
            public int fileCount { get; set; }
            public long totalBytes { get; set; }
            public string treeSha256 { get; set; }
            public PayloadFileRecord[] files { get; set; }
            public PayloadBlobRecord archive { get; set; }
            public PayloadBlobRecord extractor { get; set; }
        }

        private const string ProductName = "analytix";
        private const string ProductDisplayName = "Analytix灵鉴";
        private const string LegacyProductDisplayName = "Analytix";
        private const string AppExeName = "analytix.exe";
        private const string UninstallerName = "Uninstall analytix.exe";
        private const string UninstallerDirectoryName = "AnalytixUninstaller";
        private const string UninstallRegistryPath = @"Software\Microsoft\Windows\CurrentVersion\Uninstall\Analytix";
        private const string ExpectedWindowsSignerSha1 = "ANALYTIX_EXPECTED_WINDOWS_SIGNER_SHA1";
        private const bool AllowUnsignedWindowsQa = false;
        public const string CurrentVersionText = "1.0.6";
        private static readonly Version CurrentPackageVersion = new Version(1, 0, 6, 0);

        public static bool IsInstalled(string installPath)
        {
            return File.Exists(Path.Combine(NormalizeInstallPath(installPath), AppExeName));
        }

        public static string ResolveInstalledPath()
        {
            var registryPath = ReadUninstallRegistryValue("InstallLocation");
            if (!string.IsNullOrWhiteSpace(registryPath))
            {
                return NormalizeInstallPath(registryPath);
            }
            return Program.DefaultInstallPath();
        }

        public static string GetInstalledVersionText(string installPath)
        {
            var registryVersion = ReadUninstallRegistryValue("DisplayVersion");
            if (!string.IsNullOrWhiteSpace(registryVersion))
            {
                return registryVersion;
            }
            var appPath = Path.Combine(NormalizeInstallPath(installPath), AppExeName);
            if (!File.Exists(appPath))
            {
                return "";
            }
            try
            {
                var info = FileVersionInfo.GetVersionInfo(appPath);
                var productVersion = CleanVersionText(info.ProductVersion);
                return string.IsNullOrWhiteSpace(productVersion) ? CleanVersionText(info.FileVersion) : productVersion;
            }
            catch
            {
                return "";
            }
        }

        public static Version GetInstalledVersion(string installPath)
        {
            Version version;
            var text = CleanVersionText(GetInstalledVersionText(installPath));
            if (Version.TryParse(text, out version))
            {
                return version;
            }
            return null;
        }

        public static bool IsCurrentPackageNewerThanInstalled(string installPath)
        {
            var installedVersion = GetInstalledVersion(installPath);
            return installedVersion != null && CompareVersionParts(CurrentPackageVersion, installedVersion) > 0;
        }

        public static void Install(string installPath, string payloadResourceName, bool launchAfterInstall)
        {
            var targetDir = ValidateInstallTarget(installPath, false);
            var parent = Directory.GetParent(targetDir);
            if (parent == null)
            {
                throw new InvalidOperationException("安装目录不能是卷根目录。");
            }
            StopInstalledApp(targetDir);
            Directory.CreateDirectory(parent.FullName);
            var stagingDir = Path.Combine(parent.FullName, ".analytix-staging-" + Guid.NewGuid().ToString("N"));
            var backupDir = Path.Combine(parent.FullName, ".analytix-backup-" + Guid.NewGuid().ToString("N"));
            var failedDir = Path.Combine(parent.FullName, ".analytix-failed-" + Guid.NewGuid().ToString("N"));
            var previousMoved = false;
            var stagedMoved = false;
            Directory.CreateDirectory(stagingDir);
            try
            {
                ExtractPayload(stagingDir, payloadResourceName);
                VerifyInstalledPayload(stagingDir);
                if (Directory.Exists(targetDir))
                {
                    Directory.Move(targetDir, backupDir);
                    previousMoved = true;
                }
                Directory.Move(stagingDir, targetDir);
                stagedMoved = true;
                VerifyInstalledPayload(targetDir);
                InstallUninstaller(targetDir);
                CreateShortcuts(targetDir);
                DeleteLegacyUninstallRegistry(targetDir);
                WriteUninstallRegistry(targetDir);
            }
            catch (Exception installError)
            {
                Exception rollbackError = null;
                try
                {
                    if (stagedMoved && Directory.Exists(targetDir))
                    {
                        Directory.Move(targetDir, failedDir);
                    }
                }
                catch (Exception error)
                {
                    rollbackError = error;
                }
                try
                {
                    if (previousMoved && Directory.Exists(backupDir) && !Directory.Exists(targetDir))
                    {
                        Directory.Move(backupDir, targetDir);
                    }
                }
                catch (Exception error)
                {
                    rollbackError = rollbackError == null ? error : new AggregateException(rollbackError, error);
                }
                if (Directory.Exists(failedDir))
                {
                    try { DeleteDirectoryWithRetries(failedDir); } catch { }
                }
                if (rollbackError != null)
                {
                    throw new AggregateException("安装失败且旧版本回滚未完整完成。", installError, rollbackError);
                }
                throw;
            }
            finally
            {
                if (Directory.Exists(stagingDir))
                {
                    try { DeleteDirectoryWithRetries(stagingDir); } catch { }
                }
            }
            if (Directory.Exists(backupDir))
            {
                try { DeleteDirectoryWithRetries(backupDir); } catch { }
            }
            if (launchAfterInstall)
            {
                var appPath = Path.Combine(targetDir, AppExeName);
                if (File.Exists(appPath))
                {
                    Process.Start(new ProcessStartInfo(appPath) { UseShellExecute = true });
                }
            }
        }

        public static void Uninstall(string installPath)
        {
            var targetDir = ValidateInstallTarget(installPath, true);
            var currentExe = Assembly.GetExecutingAssembly().Location;
            StopInstalledApp(targetDir);
            // User data intentionally stays on disk so reinstall/update does not lose cases,
            // settings, or imported analysis data.
            if (!Directory.Exists(targetDir))
            {
                DeleteShortcuts();
                DeleteUninstallRegistry();
                DeleteLegacyUninstallRegistry(targetDir);
                DeleteExternalUninstaller(currentExe);
                return;
            }

            if (IsInsideDirectory(currentExe, targetDir))
            {
                var parent = Directory.GetParent(targetDir);
                var parentDir = parent == null ? "" : parent.FullName;
                DeleteExternalUninstaller(currentExe);
                ScheduleDirectoryDelete(targetDir, parentDir);
                return;
            }
            DeleteDirectoryWithRetries(targetDir);
            DeleteShortcuts();
            DeleteUninstallRegistry();
            DeleteLegacyUninstallRegistry(targetDir);
            TryDeleteEmptyParentDirectory(targetDir);
            DeleteExternalUninstaller(currentExe);
        }

        private static string ToPowerShellSingleQuotedString(string value)
        {
            return "'" + (value ?? "").Replace("'", "''") + "'";
        }

        private static void ScheduleDirectoryDelete(string targetDir, string parentDir)
        {
            var cleanupScript =
                "$target = " + ToPowerShellSingleQuotedString(targetDir) + "\n" +
                "$parent = " + ToPowerShellSingleQuotedString(parentDir) + "\n" +
                "Start-Sleep -Seconds 2\n" +
                "for ($i = 0; $i -lt 180; $i++) {\n" +
                "  try {\n" +
                "    if (Test-Path -LiteralPath $target) {\n" +
                "      Get-ChildItem -LiteralPath $target -Recurse -Force -ErrorAction SilentlyContinue | ForEach-Object { try { $_.Attributes = 'Normal' } catch {} }\n" +
                "      Remove-Item -LiteralPath $target -Recurse -Force -ErrorAction Stop\n" +
                "    }\n" +
                "    if (-not (Test-Path -LiteralPath $target)) { break }\n" +
                "  } catch { Start-Sleep -Milliseconds 500 }\n" +
                "}\n" +
                "try {\n" +
                "  if ($parent -and (Test-Path -LiteralPath $parent) -and -not (Get-ChildItem -LiteralPath $parent -Force -ErrorAction SilentlyContinue)) {\n" +
                "    Remove-Item -LiteralPath $parent -Force -ErrorAction SilentlyContinue\n" +
                "  }\n" +
                "} catch {}\n";
            RunHiddenPowerShell(cleanupScript);
            ScheduleDirectoryDeleteWithCmd(targetDir, parentDir);
        }

        private static void ScheduleFileDelete(string filePath, string parentDir)
        {
            var args = "/c ping 127.0.0.1 -n 3 > nul & del /f /q \"" + filePath + "\"";
            if (!string.IsNullOrWhiteSpace(parentDir))
            {
                args += " & rmdir /s /q \"" + parentDir + "\" 2> nul";
            }
            Process.Start(new ProcessStartInfo(ResolveCanonicalSystemExecutable("cmd.exe"), args)
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                WorkingDirectory = Path.GetTempPath(),
                WindowStyle = ProcessWindowStyle.Hidden
            });
        }

        private static void ScheduleDirectoryDeleteWithCmd(string targetDir, string parentDir)
        {
            var args = "/c ping 127.0.0.1 -n 4 > nul & attrib -r -s -h \"" + Path.Combine(targetDir, "*") + "\" /s /d 2> nul & rmdir /s /q \"" + targetDir + "\" 2> nul";
            if (!string.IsNullOrWhiteSpace(parentDir))
            {
                args += " & rmdir \"" + parentDir + "\" 2> nul";
            }
            Process.Start(new ProcessStartInfo(ResolveCanonicalSystemExecutable("cmd.exe"), args)
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                WorkingDirectory = Path.GetTempPath(),
                WindowStyle = ProcessWindowStyle.Hidden
            });
        }

        private static void RunHiddenPowerShell(string script)
        {
            var encoded = Convert.ToBase64String(Encoding.Unicode.GetBytes(script));
            Process.Start(new ProcessStartInfo(ResolveCanonicalSystemExecutable(Path.Combine("WindowsPowerShell", "v1.0", "powershell.exe")), "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -EncodedCommand " + encoded)
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                WorkingDirectory = Path.GetTempPath(),
                WindowStyle = ProcessWindowStyle.Hidden
            });
        }

        public static void RunElevatedInstall(string installPath, Action<int, string> onProgress)
        {
            RunElevated("--install-silent --no-launch --path \"" + NormalizeInstallPath(installPath) + "\"", true, onProgress);
        }

        public static void RunElevatedUninstall(string installPath, Action<int, string> onProgress)
        {
            RunElevated("--uninstall-silent --path \"" + NormalizeInstallPath(installPath) + "\"", false, onProgress);
        }

        public static void LaunchInstalledApp(string installPath)
        {
            var appPath = Path.Combine(NormalizeInstallPath(installPath), AppExeName);
            if (!File.Exists(appPath))
            {
                throw new FileNotFoundException("安装完成后未找到 analytix.exe。", appPath);
            }
            Process.Start(new ProcessStartInfo(appPath) { UseShellExecute = true });
        }

        private static void RunElevated(string arguments, bool install, Action<int, string> onProgress)
        {
            var currentExe = Assembly.GetExecutingAssembly().Location;
            var process = Process.Start(new ProcessStartInfo(currentExe, arguments)
            {
                UseShellExecute = true,
                Verb = IsAdministrator() ? string.Empty : "runas",
                WindowStyle = ProcessWindowStyle.Hidden
            });
            if (process == null)
            {
                throw new InvalidOperationException("无法启动安装进程。");
            }
            var startedAt = DateTime.UtcNow;
            var progressFloor = install ? 18 : 24;
            var lastProgress = progressFloor;
            var progressCap = install ? 94 : 92;
            if (onProgress != null)
            {
                onProgress(progressFloor, ProgressDetailForNativeOperation(progressFloor, install));
            }
            while (!process.WaitForExit(180))
            {
                var elapsedMs = Math.Max(0, (DateTime.UtcNow - startedAt).TotalMilliseconds);
                var durationMs = install ? 10500.0 : 5200.0;
                var ratio = Math.Min(1.0, elapsedMs / durationMs);
                var eased = 1.0 - Math.Pow(1.0 - ratio, 3.0);
                var nextProgress = Math.Min(progressCap, progressFloor + (int)Math.Round((progressCap - progressFloor) * eased));
                if (nextProgress > lastProgress)
                {
                    lastProgress = nextProgress;
                    if (onProgress != null)
                    {
                        onProgress(lastProgress, ProgressDetailForNativeOperation(lastProgress, install));
                    }
                }
            }
            if (process.ExitCode != 0)
            {
                throw new InvalidOperationException("安装进程失败，退出码 " + process.ExitCode + "。");
            }
        }

        private static string ProgressDetailForNativeOperation(int percent, bool install)
        {
            if (!install)
            {
                if (percent >= 78)
                {
                    return "CLEARING SHORTCUTS";
                }
                if (percent >= 52)
                {
                    return "REMOVING APPLICATION FILES";
                }
                if (percent >= 24)
                {
                    return "STOPPING LOCAL SERVICES";
                }
                return "PREPARING UNINSTALLER";
            }
            if (percent >= 88)
            {
                return "FINALIZING INSTALLATION";
            }
            if (percent >= 68)
            {
                return "CREATING LOCAL WORKSPACE";
            }
            if (percent >= 42)
            {
                return "INSTALLING CORE COMPONENTS";
            }
            if (percent >= 18)
            {
                return "VERIFYING INSTALL LOCATION";
            }
            return "PREPARING INSTALL PACKAGE";
        }

        private static bool IsAdministrator()
        {
            return IsCurrentProcessAdministrator();
        }

        public static bool IsCurrentProcessAdministrator()
        {
            using (var identity = WindowsIdentity.GetCurrent())
            {
                var principal = new WindowsPrincipal(identity);
                return principal.IsInRole(WindowsBuiltInRole.Administrator);
            }
        }

        private static string NormalizeInstallPath(string installPath)
        {
            var cleaned = string.IsNullOrWhiteSpace(installPath) ? Program.DefaultInstallPath() : installPath.Trim();
            return Path.GetFullPath(cleaned);
        }

        private static string ValidateInstallTarget(string installPath, bool requireExistingBinding)
        {
            var targetDir = NormalizeInstallPath(installPath).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
            var volumeRoot = Path.GetPathRoot(targetDir).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
            if (string.IsNullOrWhiteSpace(targetDir) || string.Equals(targetDir, volumeRoot, StringComparison.OrdinalIgnoreCase))
            {
                throw new InvalidOperationException("安装目录不能是卷根目录。");
            }
            var allowed = false;
            foreach (var root in new[] {
                Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),
                Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86)
            })
            {
                if (
                    !string.IsNullOrWhiteSpace(root) &&
                    !string.Equals(Path.GetFullPath(root).TrimEnd(Path.DirectorySeparatorChar), targetDir, StringComparison.OrdinalIgnoreCase) &&
                    IsInsideDirectory(targetDir, root)
                )
                {
                    allowed = true;
                    break;
                }
            }
            if (!allowed)
            {
                throw new InvalidOperationException("安装目录必须位于 Program Files 的子目录中。");
            }
            RequireNoReparseAncestors(targetDir);
            if (File.Exists(targetDir))
            {
                throw new InvalidOperationException("安装目录不能是文件。");
            }
            if (!Directory.Exists(targetDir))
            {
                if (requireExistingBinding)
                {
                    throw new DirectoryNotFoundException("未找到已安装的 analytix 目录。");
                }
                return targetDir;
            }
            CollectPayloadFilesNoFollow(targetDir);
            var entries = Directory.GetFileSystemEntries(targetDir);
            if (!requireExistingBinding && entries.Length == 0)
            {
                return targetDir;
            }
            var registryPath = ReadUninstallRegistryValue("InstallLocation");
            var appPath = Path.Combine(targetDir, AppExeName);
            if (
                string.IsNullOrWhiteSpace(registryPath) ||
                !string.Equals(Path.GetFullPath(registryPath).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar), targetDir, StringComparison.OrdinalIgnoreCase) ||
                !File.Exists(appPath) ||
                (File.GetAttributes(appPath) & FileAttributes.ReparsePoint) != 0
            )
            {
                throw new InvalidOperationException("现有目录不是 registry 绑定的 analytix 安装，拒绝覆盖或删除。");
            }
            return targetDir;
        }

        private static void RequireNoReparseAncestors(string targetDir)
        {
            var current = Path.GetFullPath(targetDir);
            while (!string.IsNullOrWhiteSpace(current))
            {
                if ((Directory.Exists(current) || File.Exists(current)) && (File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0)
                {
                    throw new InvalidOperationException("安装目录祖先包含重解析点: " + current);
                }
                var parent = Directory.GetParent(current);
                if (parent == null)
                {
                    break;
                }
                current = parent.FullName;
            }
        }

        private static string ReadUninstallRegistryValue(string valueName)
        {
            try
            {
                using (var key = Registry.LocalMachine.OpenSubKey(UninstallRegistryPath))
                {
                    if (key != null)
                    {
                        return Convert.ToString(key.GetValue(valueName)) ?? "";
                    }
                }
            }
            catch
            {
                // Fall through to legacy per-user lookup.
            }
            try
            {
                using (var key = Registry.CurrentUser.OpenSubKey(UninstallRegistryPath))
                {
                    if (key == null)
                    {
                        return "";
                    }
                    return Convert.ToString(key.GetValue(valueName)) ?? "";
                }
            }
            catch
            {
                return "";
            }
        }

        private static string CleanVersionText(string value)
        {
            var text = (value ?? "").Trim();
            if (text.Length == 0)
            {
                return "";
            }
            var plusIndex = text.IndexOf('+');
            if (plusIndex >= 0)
            {
                text = text.Substring(0, plusIndex);
            }
            var spaceIndex = text.IndexOf(' ');
            if (spaceIndex >= 0)
            {
                text = text.Substring(0, spaceIndex);
            }
            return text.Trim();
        }

        private static int CompareVersionParts(Version left, Version right)
        {
            if (left == null && right == null)
            {
                return 0;
            }
            if (left == null)
            {
                return -1;
            }
            if (right == null)
            {
                return 1;
            }
            var leftParts = new[] { left.Major, left.Minor, left.Build < 0 ? 0 : left.Build, left.Revision < 0 ? 0 : left.Revision };
            var rightParts = new[] { right.Major, right.Minor, right.Build < 0 ? 0 : right.Build, right.Revision < 0 ? 0 : right.Revision };
            for (var index = 0; index < leftParts.Length; index++)
            {
                if (leftParts[index] != rightParts[index])
                {
                    return leftParts[index].CompareTo(rightParts[index]);
                }
            }
            return 0;
        }

        private static void ExtractPayload(string targetDir, string payloadResourceName)
        {
            var tempDir = Path.Combine(
                Path.GetTempPath(),
                "AnalytixPayload-" + Process.GetCurrentProcess().Id + "-" + Guid.NewGuid().ToString("N")
            );
            Directory.CreateDirectory(tempDir);
            try
            {
                var archivePath = Path.Combine(tempDir, "AnalytixPayload.7z");
                var extractorPath = Path.Combine(tempDir, "7za.exe");
                var manifestPath = Path.Combine(tempDir, "AnalytixPayloadManifest.json");
                ExtractManifestResourceToFile(payloadResourceName, archivePath, "安装负载缺失。");
                ExtractManifestResourceToFile("PayloadExtractorExe", extractorPath, "安装负载解压器缺失。");
                ExtractManifestResourceToFile("AnalytixPayloadManifest", manifestPath, "安装负载清单缺失。");
                VerifyTrustedBootstrapperExecutables(extractorPath);
                var manifest = LoadPayloadManifest(manifestPath);
                VerifyPayloadBlob(archivePath, manifest.archive, "安装负载归档");
                VerifyPayloadBlob(extractorPath, manifest.extractor, "安装负载解压器");
                ValidateSevenZipPayloadEntries(extractorPath, archivePath, targetDir, manifest);
                RunProcessAndCapture(
                    extractorPath,
                    "x -y -bd -bb0 " + QuoteProcessArgument(archivePath) + " -o" + QuoteProcessArgument(targetDir),
                    15 * 60 * 1000,
                    "安装负载解压失败。"
                );
                VerifyExtractedPayload(targetDir, manifest);
            }
            finally
            {
                try
                {
                    Directory.Delete(tempDir, true);
                }
                catch
                {
                    // Best-effort cleanup; temp files are process-scoped and harmless if locked.
                }
            }
        }

        private static void VerifyTrustedBootstrapperExecutables(string extractorPath)
        {
            if (AllowUnsignedWindowsQa)
            {
                return;
            }
            if (ExpectedWindowsSignerSha1.Length != 40)
            {
                throw new InvalidDataException("安装器缺少批准的 Windows signer 身份。");
            }
            var powershell = ResolveCanonicalSystemExecutable(Path.Combine("WindowsPowerShell", "v1.0", "powershell.exe"));
            var hostPath = Assembly.GetExecutingAssembly().Location;
            var script =
                "$ErrorActionPreference='Stop';" +
                "$tab=[char]9;" +
                "$paths=@(" + ToPowerShellSingleQuotedString(hostPath) + "," + ToPowerShellSingleQuotedString(extractorPath) + ");" +
                "foreach($path in $paths){" +
                "$signature=Get-AuthenticodeSignature -LiteralPath $path;" +
                "$thumb=if($signature.SignerCertificate){[string]$signature.SignerCertificate.Thumbprint}else{''};" +
                "$timestamp=if($signature.TimeStamperCertificate){[string]$signature.TimeStamperCertificate.Thumbprint}else{''};" +
                "Write-Output (([string]$signature.Status)+$tab+$thumb+$tab+$timestamp);}";
            var encoded = Convert.ToBase64String(Encoding.Unicode.GetBytes(script));
            var output = RunProcessAndCapture(
                powershell,
                "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand " + encoded,
                30000,
                "安装器签名验证失败。"
            );
            var lines = output.Replace("\r\n", "\n").Split(new[] { '\n' }, StringSplitOptions.RemoveEmptyEntries);
            if (lines.Length != 2)
            {
                throw new InvalidDataException("安装器签名验证结果不完整。");
            }
            foreach (var line in lines)
            {
                var fields = line.TrimEnd('\r').Split('\t');
                if (
                    fields.Length != 3 || fields[0] != "Valid" ||
                    !string.Equals(fields[1], ExpectedWindowsSignerSha1, StringComparison.OrdinalIgnoreCase) ||
                    string.IsNullOrWhiteSpace(fields[2])
                )
                {
                    throw new InvalidDataException("安装器或解压器未绑定批准且带时间戳的 Windows signer。");
                }
            }
        }

        private static string ResolveCanonicalSystemExecutable(string relativePath)
        {
            var windowsRoot = Environment.GetFolderPath(Environment.SpecialFolder.Windows);
            var driveRoot = Path.GetPathRoot(windowsRoot);
            if (string.IsNullOrWhiteSpace(driveRoot))
            {
                throw new InvalidDataException("Windows 系统信任根无效。");
            }
            var canonicalWindowsRoot = Path.Combine(driveRoot, "Windows");
            if (!string.Equals(Path.GetFullPath(windowsRoot), Path.GetFullPath(canonicalWindowsRoot), StringComparison.OrdinalIgnoreCase))
            {
                throw new InvalidDataException("Windows 系统信任根无效。");
            }
            var executable = Path.Combine(canonicalWindowsRoot, "System32", relativePath);
            if (!File.Exists(executable) || (File.GetAttributes(executable) & FileAttributes.ReparsePoint) != 0)
            {
                throw new FileNotFoundException("Windows 系统可执行文件不可用。", executable);
            }
            return executable;
        }

        private static PayloadManifest LoadPayloadManifest(string manifestPath)
        {
            var serializer = new JavaScriptSerializer
            {
                MaxJsonLength = int.MaxValue,
                RecursionLimit = 100
            };
            var manifestText = File.ReadAllText(manifestPath, Encoding.UTF8);
            var raw = serializer.DeserializeObject(manifestText) as Dictionary<string, object>;
            object[] rawFiles;
            if (
                !HasExactKeys(raw, "schemaVersion", "treeAlgorithm", "fileCount", "totalBytes", "treeSha256", "files", "archive", "extractor") ||
                (rawFiles = raw["files"] as object[]) == null ||
                !HasExactKeys(raw["archive"] as Dictionary<string, object>, "size", "sha256") ||
                !HasExactKeys(raw["extractor"] as Dictionary<string, object>, "size", "sha256")
            )
            {
                throw new InvalidDataException("安装负载清单 schema 无效。");
            }
            foreach (var rawFile in rawFiles)
            {
                if (!HasExactKeys(rawFile as Dictionary<string, object>, "path", "size", "sha256"))
                {
                    throw new InvalidDataException("安装负载清单文件 schema 无效。");
                }
            }
            var manifest = serializer.Deserialize<PayloadManifest>(manifestText);
            if (
                manifest == null ||
                manifest.schemaVersion != 1 ||
                !string.Equals(manifest.treeAlgorithm, "analytix-windows-payload-tree-v1", StringComparison.Ordinal) ||
                manifest.files == null ||
                manifest.fileCount <= 0 ||
                manifest.files.Length != manifest.fileCount ||
                manifest.totalBytes <= 0 ||
                !IsSha256(manifest.treeSha256) ||
                manifest.archive == null ||
                manifest.archive.size <= 0 ||
                !IsSha256(manifest.archive.sha256) ||
                manifest.extractor == null ||
                manifest.extractor.size <= 0 ||
                !IsSha256(manifest.extractor.sha256)
            )
            {
                throw new InvalidDataException("安装负载清单无效。");
            }
            return manifest;
        }

        private static bool HasExactKeys(Dictionary<string, object> value, params string[] expected)
        {
            if (value == null || value.Count != expected.Length)
            {
                return false;
            }
            foreach (var key in expected)
            {
                if (!value.ContainsKey(key))
                {
                    return false;
                }
            }
            return true;
        }

        private static bool IsSha256(string value)
        {
            if (string.IsNullOrWhiteSpace(value) || value.Length != 64)
            {
                return false;
            }
            foreach (var character in value)
            {
                if (!((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')))
                {
                    return false;
                }
            }
            return true;
        }

        private static string ComputeFileSha256(string filePath)
        {
            using (var stream = File.OpenRead(filePath))
            using (var sha = SHA256.Create())
            {
                return BitConverter.ToString(sha.ComputeHash(stream)).Replace("-", "").ToLowerInvariant();
            }
        }

        private static void VerifyPayloadBlob(string filePath, PayloadBlobRecord expected, string label)
        {
            var info = new FileInfo(filePath);
            if (
                expected == null ||
                expected.size <= 0 ||
                info.Length != expected.size ||
                !string.Equals(ComputeFileSha256(filePath), expected.sha256, StringComparison.Ordinal)
            )
            {
                throw new InvalidDataException(label + "与签名清单不匹配。");
            }
        }

        private static void VerifyExtractedPayload(string targetDir, PayloadManifest manifest)
        {
            var root = Path.GetFullPath(targetDir).TrimEnd(Path.DirectorySeparatorChar) + Path.DirectorySeparatorChar;
            var actualFiles = CollectPayloadFilesNoFollow(targetDir);
            var actualByPath = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
            foreach (var filePath in actualFiles)
            {
                var relativePath = filePath.Substring(root.Length).Replace(Path.DirectorySeparatorChar, '/');
                ValidatePortablePayloadPath(relativePath);
                if (actualByPath.ContainsKey(relativePath))
                {
                    throw new InvalidDataException("安装负载包含大小写冲突路径。");
                }
                actualByPath.Add(relativePath, filePath);
            }
            if (actualByPath.Count != manifest.fileCount)
            {
                throw new InvalidDataException("安装负载文件数量与签名清单不匹配。");
            }

            long totalBytes = 0;
            string previousPath = null;
            using (var treeHash = SHA256.Create())
            {
                foreach (var record in manifest.files)
                {
                    if (
                        record == null ||
                        record.size < 0 ||
                        !IsSha256(record.sha256) ||
                        (previousPath != null && string.CompareOrdinal(previousPath, record.path) >= 0)
                    )
                    {
                        throw new InvalidDataException("安装负载清单文件记录无效。");
                    }
                    ValidatePortablePayloadPath(record.path);
                    string filePath;
                    if (!actualByPath.TryGetValue(record.path, out filePath))
                    {
                        throw new FileNotFoundException("安装负载缺少签名清单文件。", record.path);
                    }
                    var info = new FileInfo(filePath);
                    if (
                        info.Length != record.size ||
                        !string.Equals(ComputeFileSha256(filePath), record.sha256, StringComparison.Ordinal)
                    )
                    {
                        throw new InvalidDataException("安装负载文件与签名清单不匹配: " + record.path);
                    }
                    checked { totalBytes += record.size; }
                    previousPath = record.path;
                    foreach (var value in new[] { record.path, "\0", record.size.ToString(CultureInfo.InvariantCulture), "\0", record.sha256, "\0" })
                    {
                        var bytes = Encoding.UTF8.GetBytes(value);
                        treeHash.TransformBlock(bytes, 0, bytes.Length, bytes, 0);
                    }
                }
                treeHash.TransformFinalBlock(new byte[0], 0, 0);
                var digest = BitConverter.ToString(treeHash.Hash).Replace("-", "").ToLowerInvariant();
                if (totalBytes != manifest.totalBytes || !string.Equals(digest, manifest.treeSha256, StringComparison.Ordinal))
                {
                    throw new InvalidDataException("安装负载树摘要与签名清单不匹配。");
                }
            }
        }

        private static string[] CollectPayloadFilesNoFollow(string targetDir)
        {
            var files = new List<string>();
            var pending = new Stack<string>();
            pending.Push(Path.GetFullPath(targetDir));
            while (pending.Count > 0)
            {
                var directory = pending.Pop();
                var directoryAttributes = File.GetAttributes(directory);
                if ((directoryAttributes & FileAttributes.ReparsePoint) != 0)
                {
                    throw new InvalidDataException("安装负载包含重解析目录。");
                }
                foreach (var entry in new DirectoryInfo(directory).GetFileSystemInfos())
                {
                    var attributes = entry.Attributes;
                    if ((attributes & FileAttributes.ReparsePoint) != 0)
                    {
                        throw new InvalidDataException("安装负载包含重解析项。");
                    }
                    if ((attributes & FileAttributes.Directory) != 0)
                    {
                        pending.Push(entry.FullName);
                    }
                    else if (entry is FileInfo)
                    {
                        files.Add(entry.FullName);
                    }
                    else
                    {
                        throw new InvalidDataException("安装负载包含特殊文件。");
                    }
                }
            }
            return files.ToArray();
        }

        private static void ExtractManifestResourceToFile(string resourceName, string targetPath, string missingMessage)
        {
            var assembly = Assembly.GetExecutingAssembly();
            using (var resource = assembly.GetManifestResourceStream(resourceName))
            {
                if (resource == null)
                {
                    throw new FileNotFoundException(missingMessage, resourceName);
                }
                using (var target = File.Create(targetPath))
                {
                    resource.CopyTo(target);
                }
            }
        }

        private static void ValidateSevenZipPayloadEntries(string extractorPath, string archivePath, string targetDir, PayloadManifest manifest)
        {
            var listing = RunProcessAndCapture(
                extractorPath,
                "l -slt " + QuoteProcessArgument(archivePath),
                5 * 60 * 1000,
                "安装负载校验失败。"
            );
            var expectedFiles = new Dictionary<string, PayloadFileRecord>(StringComparer.OrdinalIgnoreCase);
            foreach (var file in manifest.files)
            {
                ValidatePortablePayloadPath(file.path);
                if (expectedFiles.ContainsKey(file.path))
                {
                    throw new InvalidDataException("安装负载清单包含重复路径。");
                }
                expectedFiles.Add(file.path, file);
            }
            var inEntries = false;
            var entryCount = 0;
            var fields = new Dictionary<string, string>(StringComparer.Ordinal);
            var archivePaths = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            foreach (var rawLine in listing.Replace("\r\n", "\n").Split('\n'))
            {
                var line = rawLine.TrimEnd('\r');
                if (line.StartsWith("----------", StringComparison.Ordinal))
                {
                    inEntries = true;
                    continue;
                }
                if (!inEntries)
                {
                    continue;
                }
                if (line.Length == 0)
                {
                    ValidateSevenZipPayloadRecord(fields, targetDir, expectedFiles, archivePaths, ref entryCount);
                    fields.Clear();
                    continue;
                }
                var separator = line.IndexOf(" = ", StringComparison.Ordinal);
                if (separator <= 0)
                {
                    throw new InvalidDataException("安装负载归档条目格式无效。");
                }
                var key = line.Substring(0, separator);
                if (fields.ContainsKey(key))
                {
                    throw new InvalidDataException("安装负载归档条目包含重复字段。");
                }
                fields.Add(key, line.Substring(separator + 3));
            }
            ValidateSevenZipPayloadRecord(fields, targetDir, expectedFiles, archivePaths, ref entryCount);
            if (entryCount != manifest.fileCount || expectedFiles.Count != 0)
            {
                throw new InvalidOperationException("安装负载归档文件集合与清单不匹配。");
            }
        }

        private static void ValidateSevenZipPayloadRecord(
            Dictionary<string, string> fields,
            string targetDir,
            Dictionary<string, PayloadFileRecord> expectedFiles,
            HashSet<string> archivePaths,
            ref int entryCount
        )
        {
            if (fields.Count == 0)
            {
                return;
            }
            string entryPath;
            if (!fields.TryGetValue("Path", out entryPath) || string.IsNullOrEmpty(entryPath))
            {
                throw new InvalidDataException("安装负载归档条目缺少路径。");
            }
            entryPath = entryPath.Replace('\\', '/');
            ValidatePortablePayloadPath(entryPath);
            ValidateArchiveEntryPath(entryPath, targetDir);
            if (!archivePaths.Add(entryPath))
            {
                throw new InvalidDataException("安装负载归档包含重复或大小写冲突路径。");
            }
            string linkTarget;
            foreach (var key in new[] { "Symbolic Link", "Hard Link", "Alternate Stream" })
            {
                if (fields.TryGetValue(key, out linkTarget) && !string.IsNullOrEmpty(linkTarget) && linkTarget != "-")
                {
                    throw new InvalidDataException("安装负载归档包含链接或数据流条目。");
                }
            }
            string folder;
            string attributes;
            fields.TryGetValue("Folder", out folder);
            fields.TryGetValue("Attributes", out attributes);
            var isDirectory = folder == "+" || (!string.IsNullOrEmpty(attributes) && attributes.ToUpperInvariant().Contains("D"));
            if (!string.IsNullOrEmpty(attributes) && attributes.ToUpperInvariant().Contains("L"))
            {
                throw new InvalidDataException("安装负载归档包含链接属性。");
            }
            if (isDirectory)
            {
                var prefix = entryPath.TrimEnd('/') + "/";
                foreach (var expectedPath in expectedFiles.Keys)
                {
                    if (expectedPath.StartsWith(prefix, StringComparison.OrdinalIgnoreCase))
                    {
                        return;
                    }
                }
                throw new InvalidDataException("安装负载归档包含未声明目录。");
            }
            PayloadFileRecord expected;
            if (!expectedFiles.TryGetValue(entryPath, out expected) || !string.Equals(expected.path, entryPath, StringComparison.Ordinal))
            {
                throw new InvalidDataException("安装负载归档包含未声明文件。");
            }
            string sizeText;
            long size;
            if (!fields.TryGetValue("Size", out sizeText) || !long.TryParse(sizeText, NumberStyles.None, CultureInfo.InvariantCulture, out size) || size != expected.size)
            {
                throw new InvalidDataException("安装负载归档文件大小与清单不匹配。");
            }
            expectedFiles.Remove(entryPath);
            entryCount++;
        }

        private static void ValidatePortablePayloadPath(string value)
        {
            if (string.IsNullOrEmpty(value) || value.IndexOf('\\') >= 0)
            {
                throw new InvalidDataException("安装负载路径不是 portable ASCII。");
            }
            foreach (var character in value)
            {
                if (character < 0x20 || character > 0x7e)
                {
                    throw new InvalidDataException("安装负载路径不是 portable ASCII。");
                }
            }
            foreach (var segment in value.Split('/'))
            {
                var dot = segment.IndexOf('.');
                var stem = (dot < 0 ? segment : segment.Substring(0, dot)).ToUpperInvariant();
                if (
                    string.IsNullOrEmpty(segment) || segment == "." || segment == ".." ||
                    segment.IndexOfAny(new[] { '<', '>', ':', '"', '|', '?', '*' }) >= 0 ||
                    segment.EndsWith(".", StringComparison.Ordinal) || segment.EndsWith(" ", StringComparison.Ordinal) ||
                    stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" ||
                    (stem.Length == 4 && (stem.StartsWith("COM", StringComparison.Ordinal) || stem.StartsWith("LPT", StringComparison.Ordinal)) && stem[3] >= '1' && stem[3] <= '9')
                )
                {
                    throw new InvalidDataException("安装负载路径在 Windows 上不安全: " + value);
                }
            }
        }

        private static void ValidateArchiveEntryPath(string entryPath, string targetDir)
        {
            ValidatePortablePayloadPath(entryPath);
            var normalized = entryPath.Replace('/', Path.DirectorySeparatorChar);
            if (Path.IsPathRooted(normalized) || normalized.IndexOf(':') >= 0)
            {
                throw new InvalidOperationException("安装负载包含非法路径: " + entryPath);
            }
            var destinationPath = Path.GetFullPath(Path.Combine(targetDir, normalized));
            if (!IsInsideDirectory(destinationPath, targetDir) && !string.Equals(destinationPath, targetDir, StringComparison.OrdinalIgnoreCase))
            {
                throw new InvalidOperationException("安装负载包含非法路径: " + entryPath);
            }
        }

        private static string RunProcessAndCapture(string fileName, string arguments, int timeoutMs, string failureMessage)
        {
            var startInfo = new ProcessStartInfo(fileName, arguments)
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                WorkingDirectory = Path.GetTempPath(),
                WindowStyle = ProcessWindowStyle.Hidden
            };
            using (var process = Process.Start(startInfo))
            {
                if (process == null)
                {
                    throw new InvalidOperationException(failureMessage + " 无法启动进程。");
                }
                var outputTask = process.StandardOutput.ReadToEndAsync();
                var errorTask = process.StandardError.ReadToEndAsync();
                if (!process.WaitForExit(timeoutMs))
                {
                    try { process.Kill(); } catch {}
                    throw new TimeoutException(failureMessage + " 操作超时。");
                }
                Task.WaitAll(outputTask, errorTask);
                var output = outputTask.Result ?? "";
                var error = errorTask.Result ?? "";
                if (process.ExitCode != 0)
                {
                    var detail = (error.Length > 0 ? error : output).Trim();
                    throw new InvalidOperationException(failureMessage + (detail.Length > 0 ? " " + detail : ""));
                }
                return output;
            }
        }

        private static string QuoteProcessArgument(string value)
        {
            if (string.IsNullOrEmpty(value))
            {
                return "\"\"";
            }
            var quoted = new StringBuilder();
            quoted.Append('"');
            var backslashes = 0;
            foreach (var ch in value)
            {
                if (ch == '\\')
                {
                    backslashes++;
                    continue;
                }
                if (ch == '"')
                {
                    quoted.Append('\\', backslashes * 2 + 1);
                    quoted.Append('"');
                    backslashes = 0;
                    continue;
                }
                if (backslashes > 0)
                {
                    quoted.Append('\\', backslashes);
                    backslashes = 0;
                }
                quoted.Append(ch);
            }
            if (backslashes > 0)
            {
                quoted.Append('\\', backslashes * 2);
            }
            quoted.Append('"');
            return quoted.ToString();
        }

        private static void VerifyInstalledPayload(string targetDir)
        {
            var requiredFiles = new[]
            {
                AppExeName,
                "icudtl.dat",
                "ffmpeg.dll",
                @"resources\app.asar",
                @"resources\runtime-go\bin\runtime-server.exe",
                @"resources\runtime\analytix-native-components-receipt.json",
                @"resources\.python-runtime\current\analytix-python-runtime-manifest.json",
                @"resources\python-site-packages\analytix-site-packages-manifest.json",
                @"resources\app.asar.unpacked\packages\runtime\node_modules\analytix-computer-use\dist\windows\amd64\analytix-computer-use.exe"
            };
            var requiredDirectories = new[]
            {
                @"resources\backend"
            };
            var missing = new List<string>();
            foreach (var relativePath in requiredFiles)
            {
                var filePath = Path.Combine(targetDir, relativePath);
                if (!File.Exists(filePath))
                {
                    missing.Add(relativePath);
                }
            }
            foreach (var relativePath in requiredDirectories)
            {
                var directoryPath = Path.Combine(targetDir, relativePath);
                if (!Directory.Exists(directoryPath))
                {
                    missing.Add(relativePath);
                }
            }
            if (missing.Count > 0)
            {
                throw new FileNotFoundException("安装负载解压不完整，缺少: " + string.Join(", ", missing.ToArray()));
            }
        }

        private static bool IsInsideDirectory(string path, string directory)
        {
            var fullPath = Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
            var fullDirectory = Path.GetFullPath(directory).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
            return fullPath.StartsWith(fullDirectory, StringComparison.OrdinalIgnoreCase);
        }

        private static void StopInstalledApp(string targetDir)
        {
            var currentPid = Process.GetCurrentProcess().Id;
            foreach (var process in Process.GetProcesses())
            {
                try
                {
                    if (process.Id == currentPid)
                    {
                        continue;
                    }
                    var modulePath = process.MainModule.FileName;
                    if (string.IsNullOrWhiteSpace(modulePath) || !IsInsideDirectory(modulePath, targetDir))
                    {
                        continue;
                    }
                    process.Kill();
                    process.WaitForExit(5000);
                }
                catch
                {
                    // Process may exit while we inspect it. Continue cleanup.
                }
            }
        }

        private static void DeleteDirectoryWithRetries(string targetDir)
        {
            if (!Directory.Exists(targetDir))
            {
                return;
            }

            Exception lastError = null;
            for (var attempt = 0; attempt < 12; attempt++)
            {
                try
                {
                    StopInstalledApp(targetDir);
                    DeleteDirectoryTreeNoFollow(targetDir);
                    if (!Directory.Exists(targetDir))
                    {
                        return;
                    }
                }
                catch (UnauthorizedAccessException error)
                {
                    lastError = error;
                }
                catch (IOException error)
                {
                    lastError = error;
                }
                Thread.Sleep(180 + attempt * 90);
            }

            throw new IOException("无法清理旧安装目录，请退出正在运行的 analytix 或旧安装器后重试: " + targetDir, lastError);
        }

        private static void DeleteDirectoryTreeNoFollow(string targetDir)
        {
            var root = Path.GetFullPath(targetDir);
            var rootAttributes = File.GetAttributes(root);
            if ((rootAttributes & FileAttributes.ReparsePoint) != 0)
            {
                Directory.Delete(root, false);
                return;
            }
            var pending = new Stack<string>();
            var directories = new List<string>();
            pending.Push(root);
            while (pending.Count > 0)
            {
                var directory = pending.Pop();
                directories.Add(directory);
                foreach (var entry in new DirectoryInfo(directory).GetFileSystemInfos())
                {
                    var attributes = entry.Attributes;
                    if ((attributes & FileAttributes.Directory) != 0 && (attributes & FileAttributes.ReparsePoint) == 0)
                    {
                        pending.Push(entry.FullName);
                        continue;
                    }
                    if ((attributes & FileAttributes.Directory) != 0)
                    {
                        Directory.Delete(entry.FullName, false);
                        continue;
                    }
                    try { File.SetAttributes(entry.FullName, FileAttributes.Normal); } catch { }
                    File.Delete(entry.FullName);
                }
            }
            directories.Sort((left, right) => right.Length.CompareTo(left.Length));
            foreach (var directory in directories)
            {
                try { File.SetAttributes(directory, FileAttributes.Normal); } catch { }
                Directory.Delete(directory, false);
            }
        }

        private static void TryDeleteEmptyParentDirectory(string targetDir)
        {
            try
            {
                var parent = Directory.GetParent(NormalizeInstallPath(targetDir));
                if (parent == null || !Directory.Exists(parent.FullName))
                {
                    return;
                }
                if (Directory.GetFileSystemEntries(parent.FullName).Length == 0)
                {
                    Directory.Delete(parent.FullName, false);
                }
            }
            catch
            {
                // Parent cleanup is best-effort and only runs when empty.
            }
        }

        private static void NormalizeDirectoryAttributes(string targetDir)
        {
            if (!Directory.Exists(targetDir))
            {
                return;
            }
            foreach (var directory in Directory.GetDirectories(targetDir, "*", SearchOption.AllDirectories))
            {
                try
                {
                    File.SetAttributes(directory, FileAttributes.Normal);
                }
                catch
                {
                    // Best-effort cleanup before delete.
                }
            }
            foreach (var file in Directory.GetFiles(targetDir, "*", SearchOption.AllDirectories))
            {
                try
                {
                    File.SetAttributes(file, FileAttributes.Normal);
                }
                catch
                {
                    // Best-effort cleanup before delete.
                }
            }
            try
            {
                File.SetAttributes(targetDir, FileAttributes.Normal);
            }
            catch
            {
                // Best-effort cleanup before delete.
            }
        }

        private static void InstallUninstaller(string targetDir)
        {
            var source = Assembly.GetExecutingAssembly().Location;
            var target = ResolveExternalUninstallerPath();
            Directory.CreateDirectory(Path.GetDirectoryName(target));
            File.Copy(source, target, true);
            DeleteLegacyExternalUninstallerFile();
        }

        private static string ResolveExternalUninstallerPath()
        {
            var commonAppData = Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData);
            if (string.IsNullOrWhiteSpace(commonAppData))
            {
                commonAppData = Path.GetTempPath();
            }
            return Path.Combine(commonAppData, "Analytix", UninstallerDirectoryName, UninstallerName);
        }

        private static string ResolveLegacyExternalUninstallerPath()
        {
            var localAppData = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
            if (string.IsNullOrWhiteSpace(localAppData))
            {
                localAppData = Path.GetTempPath();
            }
            return Path.Combine(localAppData, LegacyProductDisplayName, "Uninstall Analytix.exe");
        }

        private static void DeleteExternalUninstaller(string currentExe)
        {
            foreach (var external in ResolveExternalUninstallerCandidates())
            {
                DeleteExternalUninstallerPath(external, currentExe);
            }
        }

        private static IEnumerable<string> ResolveExternalUninstallerCandidates()
        {
            yield return ResolveExternalUninstallerPath();
            yield return ResolveLegacyExternalUninstallerPath();
        }

        private static void DeleteExternalUninstallerPath(string external, string currentExe)
        {
            var externalDir = Path.GetDirectoryName(external);
            if (string.Equals(Path.GetFullPath(currentExe), Path.GetFullPath(external), StringComparison.OrdinalIgnoreCase))
            {
                ScheduleFileDelete(external, externalDir);
                return;
            }
            try
            {
                if (File.Exists(external))
                {
                    File.Delete(external);
                }
                if (!string.IsNullOrWhiteSpace(externalDir) && Directory.Exists(externalDir) && Directory.GetFileSystemEntries(externalDir).Length == 0)
                {
                    Directory.Delete(externalDir, false);
                }
            }
            catch
            {
                // External uninstaller cleanup is best-effort after the app and registry have been removed.
            }
        }

        private static void DeleteLegacyExternalUninstallerFile()
        {
            var legacy = ResolveLegacyExternalUninstallerPath();
            var currentExe = Assembly.GetExecutingAssembly().Location;
            if (string.Equals(Path.GetFullPath(currentExe), Path.GetFullPath(legacy), StringComparison.OrdinalIgnoreCase))
            {
                return;
            }
            try
            {
                if (File.Exists(legacy))
                {
                    File.Delete(legacy);
                }
            }
            catch
            {
                // Legacy external uninstaller cleanup is best-effort during upgrades.
            }
        }

        private static void DeleteAnalytixUserData(string currentExe)
        {
            foreach (var directory in ResolveAnalytixUserDataDirectories())
            {
                DeleteAnalytixUserDataDirectory(directory, currentExe);
            }
        }

        private static IEnumerable<string> ResolveAnalytixUserDataDirectories()
        {
            var directories = new List<string>();
            var localAppData = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
            var appData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);

            if (!string.IsNullOrWhiteSpace(localAppData))
            {
                directories.Add(Path.Combine(localAppData, "Analytix资金分析工具"));
                directories.Add(Path.Combine(localAppData, ProductDisplayName));
                directories.Add(Path.Combine(localAppData, LegacyProductDisplayName));
                directories.Add(Path.Combine(localAppData, ProductName));
                directories.Add(Path.Combine(localAppData, UninstallerDirectoryName));
                directories.Add(Path.Combine(localAppData, "analytix-desktop-updater"));
                directories.Add(Path.Combine(localAppData, "analytix-desktop"));
            }
            if (!string.IsNullOrWhiteSpace(appData))
            {
                directories.Add(Path.Combine(appData, ProductDisplayName));
                directories.Add(Path.Combine(appData, LegacyProductDisplayName));
                directories.Add(Path.Combine(appData, ProductName));
                directories.Add(Path.Combine(appData, "analytix-desktop-updater"));
                directories.Add(Path.Combine(appData, "analytix-desktop"));
            }

            var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
            foreach (var directory in directories)
            {
                string fullPath;
                try
                {
                    fullPath = Path.GetFullPath(directory);
                }
                catch
                {
                    continue;
                }
                if (seen.Add(fullPath))
                {
                    yield return fullPath;
                }
            }
        }

        private static void DeleteAnalytixUserDataDirectory(string targetDir, string currentExe)
        {
            try
            {
                if (string.IsNullOrWhiteSpace(targetDir) || !Directory.Exists(targetDir))
                {
                    return;
                }
                var fullPath = Path.GetFullPath(targetDir);
                if (!string.IsNullOrWhiteSpace(currentExe) && IsInsideDirectory(currentExe, fullPath))
                {
                    DeleteDirectoryContentsExcept(fullPath, currentExe);
                    var parent = Directory.GetParent(fullPath);
                    ScheduleDirectoryDelete(fullPath, parent == null ? "" : parent.FullName);
                    return;
                }
                NormalizeDirectoryAttributes(fullPath);
                Directory.Delete(fullPath, true);
            }
            catch
            {
                // Lifecycle smoke treats any residue as a release blocker.
            }
        }

        private static void DeleteDirectoryContentsExcept(string targetDir, string protectedPath)
        {
            try
            {
                NormalizeDirectoryAttributes(targetDir);
                var protectedFullPath = Path.GetFullPath(protectedPath);
                foreach (var file in Directory.GetFiles(targetDir, "*", SearchOption.AllDirectories))
                {
                    try
                    {
                        if (string.Equals(Path.GetFullPath(file), protectedFullPath, StringComparison.OrdinalIgnoreCase))
                        {
                            continue;
                        }
                        File.Delete(file);
                    }
                    catch
                    {
                        // Best-effort; scheduled cleanup gets another chance after this process exits.
                    }
                }
                var directories = new List<string>(Directory.GetDirectories(targetDir, "*", SearchOption.AllDirectories));
                directories.Sort((left, right) => right.Length.CompareTo(left.Length));
                foreach (var directory in directories)
                {
                    try
                    {
                        if (Directory.Exists(directory) && Directory.GetFileSystemEntries(directory).Length == 0)
                        {
                            Directory.Delete(directory, false);
                        }
                    }
                    catch
                    {
                        // Best-effort; scheduled cleanup gets another chance after this process exits.
                    }
                }
            }
            catch
            {
                // The caller still schedules a delayed recursive delete.
            }
        }

        private static void CreateShortcuts(string targetDir)
        {
            var appPath = Path.Combine(targetDir, AppExeName);
            CreateShortcut(
                Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonDesktopDirectory), ProductDisplayName + ".lnk"),
                appPath,
                targetDir
            );
            var startMenuDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonPrograms), ProductDisplayName);
            Directory.CreateDirectory(startMenuDir);
            CreateShortcut(Path.Combine(startMenuDir, ProductDisplayName + ".lnk"), appPath, targetDir);
            DeleteLegacyShortcuts();
        }

        private static void DeleteShortcuts()
        {
            var programsDir = Environment.GetFolderPath(Environment.SpecialFolder.CommonPrograms);
            DeleteFileQuietly(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonDesktopDirectory), ProductDisplayName + ".lnk"));
            DeleteFileQuietly(Path.Combine(programsDir, ProductDisplayName, ProductDisplayName + ".lnk"));
            TryDeleteEmptyDirectory(Path.Combine(programsDir, ProductDisplayName));
            DeleteLegacyShortcuts();
        }

        private static void DeleteLegacyShortcuts()
        {
            var programsDir = Environment.GetFolderPath(Environment.SpecialFolder.CommonPrograms);
            DeleteFileQuietly(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonDesktopDirectory), "Analytix.lnk"));
            DeleteFileQuietly(Path.Combine(programsDir, "Analytix", "Analytix.lnk"));
            TryDeleteEmptyDirectory(Path.Combine(programsDir, "Analytix"));
        }

        private static void TryDeleteEmptyDirectory(string path)
        {
            try
            {
                if (Directory.Exists(path) && Directory.GetFiles(path).Length == 0 && Directory.GetDirectories(path).Length == 0)
                {
                    Directory.Delete(path);
                }
            }
            catch
            {
                // Shortcut cleanup is best-effort.
            }
        }

        private static void DeleteFileQuietly(string path)
        {
            try
            {
                if (File.Exists(path))
                {
                    File.Delete(path);
                }
            }
            catch
            {
                // Shortcut cleanup is best-effort.
            }
        }

        private static void CreateShortcut(string shortcutPath, string targetPath, string workingDirectory)
        {
            var shellType = Type.GetTypeFromProgID("WScript.Shell");
            if (shellType == null)
            {
                return;
            }
            var shell = Activator.CreateInstance(shellType);
            var shortcut = shellType.InvokeMember("CreateShortcut", System.Reflection.BindingFlags.InvokeMethod, null, shell, new object[] { shortcutPath });
            var shortcutType = shortcut.GetType();
            shortcutType.InvokeMember("TargetPath", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { targetPath });
            shortcutType.InvokeMember("WorkingDirectory", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { workingDirectory });
            shortcutType.InvokeMember("IconLocation", System.Reflection.BindingFlags.SetProperty, null, shortcut, new object[] { targetPath + ",0" });
            shortcutType.InvokeMember("Save", System.Reflection.BindingFlags.InvokeMethod, null, shortcut, null);
        }

        private static void WriteUninstallRegistry(string targetDir)
        {
            using (var key = Registry.LocalMachine.CreateSubKey(UninstallRegistryPath))
            {
                if (key == null)
                {
                    return;
                }
                key.SetValue("DisplayName", ProductDisplayName);
                key.SetValue("DisplayVersion", CurrentVersionText);
                key.SetValue("Publisher", "Analytix Team");
                key.SetValue("InstallLocation", targetDir);
                key.SetValue("DisplayIcon", Path.Combine(targetDir, AppExeName));
                var uninstallerPath = ResolveExternalUninstallerPath();
                key.SetValue("UninstallString", "\"" + uninstallerPath + "\" --uninstall-silent --path \"" + targetDir + "\"");
                key.SetValue("QuietUninstallString", "\"" + uninstallerPath + "\" --uninstall-silent --path \"" + targetDir + "\"");
                key.SetValue("NoModify", 1, RegistryValueKind.DWord);
                key.SetValue("NoRepair", 1, RegistryValueKind.DWord);
            }
        }

        private static void DeleteUninstallRegistry()
        {
            try
            {
                Registry.LocalMachine.DeleteSubKeyTree(UninstallRegistryPath, false);
            }
            catch
            {
                // HKLM registry cleanup is best-effort.
            }
            try
            {
                Registry.CurrentUser.DeleteSubKeyTree(UninstallRegistryPath, false);
            }
            catch
            {
                // HKCU legacy registry cleanup is best-effort.
            }
        }

        private static void DeleteLegacyUninstallRegistry(string targetDir)
        {
            var normalizedTarget = NormalizeInstallPath(targetDir);
            DeleteLegacyUninstallRegistryRoot(Registry.LocalMachine, @"Software\Microsoft\Windows\CurrentVersion\Uninstall", normalizedTarget);
            DeleteLegacyUninstallRegistryRoot(Registry.LocalMachine, @"Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall", normalizedTarget);
            DeleteLegacyUninstallRegistryRoot(Registry.CurrentUser, @"Software\Microsoft\Windows\CurrentVersion\Uninstall", normalizedTarget);
        }

        private static void DeleteLegacyUninstallRegistryRoot(RegistryKey hive, string subKeyPath, string targetDir)
        {
            try
            {
                using (var root = hive.OpenSubKey(subKeyPath, true))
                {
                    if (root == null)
                    {
                        return;
                    }
                    foreach (var subKeyName in root.GetSubKeyNames())
                    {
                        if (string.Equals(subKeyName, "Analytix", StringComparison.OrdinalIgnoreCase))
                        {
                            continue;
                        }
                        var shouldDelete = false;
                        using (var key = root.OpenSubKey(subKeyName, false))
                        {
                            if (key == null)
                            {
                                continue;
                            }
                            var displayName = Convert.ToString(key.GetValue("DisplayName")) ?? "";
                            var displayIcon = Convert.ToString(key.GetValue("DisplayIcon")) ?? "";
                            var uninstallString = Convert.ToString(key.GetValue("UninstallString")) ?? "";
                            shouldDelete = IsLegacyAnalytixUninstallEntry(displayName, displayIcon, uninstallString, targetDir);
                        }
                        if (shouldDelete)
                        {
                            root.DeleteSubKeyTree(subKeyName, false);
                        }
                    }
                }
            }
            catch
            {
                // Legacy registry cleanup is best-effort; the installer still writes the current entry.
            }
        }

        private static bool IsLegacyAnalytixUninstallEntry(string displayName, string displayIcon, string uninstallString, string targetDir)
        {
            var name = (displayName ?? "").Trim();
            if (!name.Equals(ProductName, StringComparison.OrdinalIgnoreCase) &&
                !name.Equals(ProductDisplayName, StringComparison.OrdinalIgnoreCase) &&
                !name.Equals(LegacyProductDisplayName, StringComparison.OrdinalIgnoreCase) &&
                !name.StartsWith(ProductName + " ", StringComparison.OrdinalIgnoreCase) &&
                !name.StartsWith(ProductDisplayName + " ", StringComparison.OrdinalIgnoreCase) &&
                !name.StartsWith(LegacyProductDisplayName + " ", StringComparison.OrdinalIgnoreCase))
            {
                return false;
            }
            return ContainsPath(displayIcon, targetDir) ||
                ContainsPath(uninstallString, targetDir) ||
                name.StartsWith(ProductName + " ", StringComparison.OrdinalIgnoreCase) ||
                name.StartsWith(ProductDisplayName + " ", StringComparison.OrdinalIgnoreCase) ||
                name.StartsWith(LegacyProductDisplayName + " ", StringComparison.OrdinalIgnoreCase);
        }

        private static bool ContainsPath(string value, string path)
        {
            if (string.IsNullOrWhiteSpace(value) || string.IsNullOrWhiteSpace(path))
            {
                return false;
            }
            return value.IndexOf(path.TrimEnd('\\'), StringComparison.OrdinalIgnoreCase) >= 0;
        }
    }

#if ANALYTIX_WEBVIEW2_INSTALLER
    internal static class BootstrapperRuntime
    {
        private const string WebView2CoreResourceName = "WebView2CoreDll";
        private const string WebView2WinFormsResourceName = "WebView2WinFormsDll";
        private const string WebView2LoaderResourceName = "WebView2LoaderDll";
        private const string WebView2RuntimeInstallerResourceName = "WebView2RuntimeInstaller";
        private static string runtimeDir;

        [DllImport("kernel32.dll", SetLastError = true)]
        private static extern bool SetDllDirectory(string lpPathName);

        public static void Initialize()
        {
            DeleteOldRuntimeDirs();
            runtimeDir = Path.Combine(Path.GetTempPath(), "AnalytixBootstrapperRuntime-" + Process.GetCurrentProcess().Id);
            Directory.CreateDirectory(runtimeDir);
            ExtractResource(WebView2CoreResourceName, Path.Combine(runtimeDir, "Microsoft.Web.WebView2.Core.dll"));
            ExtractResource(WebView2WinFormsResourceName, Path.Combine(runtimeDir, "Microsoft.Web.WebView2.WinForms.dll"));
            ExtractResource(WebView2LoaderResourceName, Path.Combine(runtimeDir, "WebView2Loader.dll"));
            SetDllDirectory(runtimeDir);
            Environment.SetEnvironmentVariable("PATH", runtimeDir + ";" + Environment.GetEnvironmentVariable("PATH"));
            AppDomain.CurrentDomain.AssemblyResolve += ResolveWebView2Assembly;
            EnsureEvergreenRuntimeAvailable();
        }

        private static void EnsureEvergreenRuntimeAvailable()
        {
            if (IsEvergreenRuntimeAvailable())
            {
                return;
            }
            var installerPath = ExtractOptionalResource(
                WebView2RuntimeInstallerResourceName,
                Path.Combine(runtimeDir, "MicrosoftEdgeWebView2RuntimeInstallerX64.exe")
            );
            if (string.IsNullOrWhiteSpace(installerPath))
            {
                throw new InvalidOperationException(
                    "此 Windows 系统缺少 Microsoft Edge WebView2 Evergreen Runtime，且安装包没有内置离线运行时安装器。请安装 WebView2 Evergreen Runtime 后重试，或使用带 WebView2 离线运行时的 analytix 安装包。"
                );
            }
            RunWebView2RuntimeInstaller(installerPath);
            if (!IsEvergreenRuntimeAvailable())
            {
                throw new InvalidOperationException("Microsoft Edge WebView2 Evergreen Runtime 安装后仍不可用，请重启 Windows 后重试。");
            }
        }

        private static bool IsEvergreenRuntimeAvailable()
        {
            try
            {
                var version = CoreWebView2Environment.GetAvailableBrowserVersionString();
                return !string.IsNullOrWhiteSpace(version);
            }
            catch
            {
                return false;
            }
        }

        private static void RunWebView2RuntimeInstaller(string installerPath)
        {
            var startInfo = new ProcessStartInfo(installerPath, "/silent /install")
            {
                UseShellExecute = true,
                WindowStyle = ProcessWindowStyle.Hidden
            };
            if (!InstallerOperations.IsCurrentProcessAdministrator())
            {
                startInfo.Verb = "runas";
            }
            var process = Process.Start(startInfo);
            if (process == null)
            {
                throw new InvalidOperationException("无法启动 Microsoft Edge WebView2 Evergreen Runtime 安装器。");
            }
            if (!process.WaitForExit(180000))
            {
                try { process.Kill(); } catch {}
                throw new TimeoutException("Microsoft Edge WebView2 Evergreen Runtime 安装超时。");
            }
            if (process.ExitCode != 0 && process.ExitCode != 3010)
            {
                throw new InvalidOperationException("Microsoft Edge WebView2 Evergreen Runtime 安装失败，退出码 " + process.ExitCode + "。");
            }
        }

        private static void DeleteOldRuntimeDirs()
        {
            try
            {
                var tempDir = Path.GetTempPath();
                foreach (var directory in Directory.GetDirectories(tempDir, "AnalytixBootstrapperRuntime-*"))
                {
                    try
                    {
                        Directory.Delete(directory, true);
                    }
                    catch
                    {
                        // Another installer process may still be using this runtime.
                    }
                }
            }
            catch
            {
                // Runtime cleanup is best-effort and must never block the installer.
            }
        }

        private static Assembly ResolveWebView2Assembly(object sender, ResolveEventArgs args)
        {
            var name = new AssemblyName(args.Name).Name;
            if (string.Equals(name, "Microsoft.Web.WebView2.Core", StringComparison.OrdinalIgnoreCase))
            {
                return Assembly.LoadFrom(Path.Combine(runtimeDir, "Microsoft.Web.WebView2.Core.dll"));
            }
            if (string.Equals(name, "Microsoft.Web.WebView2.WinForms", StringComparison.OrdinalIgnoreCase))
            {
                return Assembly.LoadFrom(Path.Combine(runtimeDir, "Microsoft.Web.WebView2.WinForms.dll"));
            }
            return null;
        }

        private static void ExtractResource(string resourceName, string targetPath)
        {
            var assembly = Assembly.GetExecutingAssembly();
            using (var resource = assembly.GetManifestResourceStream(resourceName))
            {
                if (resource == null)
                {
                    throw new FileNotFoundException("安装器运行时资源缺失。", resourceName);
                }
                using (var file = File.Create(targetPath))
                {
                    resource.CopyTo(file);
                }
            }
        }

        private static string ExtractOptionalResource(string resourceName, string targetPath)
        {
            var assembly = Assembly.GetExecutingAssembly();
            using (var resource = assembly.GetManifestResourceStream(resourceName))
            {
                if (resource == null)
                {
                    return "";
                }
                using (var file = File.Create(targetPath))
                {
                    resource.CopyTo(file);
                }
            }
            return targetPath;
        }
    }

    internal sealed class InstallerTrafficControlHitTargets
    {
        private readonly Action closeAction;
        private readonly Action minimizeAction;
        private readonly Action zoomAction;
        private readonly Func<float> scaleProvider;
        private int pressedIndex = -1;

        public InstallerTrafficControlHitTargets(Action onClose, Action onMinimize, Action onZoom, Func<float> getScale)
        {
            closeAction = onClose;
            minimizeAction = onMinimize;
            zoomAction = onZoom;
            scaleProvider = getScale;
        }

        public Rectangle Bounds
        {
            get
            {
                var scale = CurrentScale();
                return new Rectangle(0, 0, Scale(80, scale), Scale(36, scale));
            }
        }

        public bool HandleExternalMouseMove(Point ownerClientPoint)
        {
            if (!Bounds.Contains(ownerClientPoint))
            {
                ClearExternalState();
                return false;
            }
            return DotIndexAt(ToLocalPoint(ownerClientPoint)) >= 0;
        }

        public bool HandleExternalMouseDown(Point ownerClientPoint)
        {
            if (!Bounds.Contains(ownerClientPoint))
            {
                ClearExternalState();
                return false;
            }
            pressedIndex = DotIndexAt(ToLocalPoint(ownerClientPoint));
            return pressedIndex >= 0;
        }

        public bool HandleExternalMouseUp(Point ownerClientPoint)
        {
            if (!Bounds.Contains(ownerClientPoint))
            {
                ClearExternalState();
                return false;
            }
            var releasedIndex = DotIndexAt(ToLocalPoint(ownerClientPoint));
            var actionIndex = pressedIndex == releasedIndex ? releasedIndex : -1;
            pressedIndex = -1;
            RunAction(actionIndex);
            return releasedIndex >= 0;
        }

        public void ClearExternalState()
        {
            pressedIndex = -1;
        }

        private int DotIndexAt(Point point)
        {
            var scale = CurrentScale();
            var hitRadius = Scale(10, scale);
            if (DistanceSquared(point, Scale(20, scale), Scale(18, scale)) <= hitRadius * hitRadius)
            {
                return 0;
            }
            if (DistanceSquared(point, Scale(40, scale), Scale(18, scale)) <= hitRadius * hitRadius)
            {
                return 1;
            }
            if (DistanceSquared(point, Scale(60, scale), Scale(18, scale)) <= hitRadius * hitRadius)
            {
                return 2;
            }
            return -1;
        }

        private Point ToLocalPoint(Point ownerClientPoint)
        {
            return new Point(ownerClientPoint.X - Bounds.Left, ownerClientPoint.Y - Bounds.Top);
        }

        private void RunAction(int actionIndex)
        {
            if (actionIndex == 0)
            {
                closeAction();
            }
            else if (actionIndex == 1)
            {
                minimizeAction();
            }
            else if (actionIndex == 2)
            {
                zoomAction();
            }
        }

        private static int DistanceSquared(Point point, int x, int y)
        {
            var dx = point.X - x;
            var dy = point.Y - y;
            return dx * dx + dy * dy;
        }

        private float CurrentScale()
        {
            try
            {
                var scale = scaleProvider == null ? 1f : scaleProvider();
                if (float.IsNaN(scale) || float.IsInfinity(scale) || scale < 1f)
                {
                    return 1f;
                }
                return scale;
            }
            catch
            {
                return 1f;
            }
        }

        private static int Scale(int value, float scale)
        {
            return Math.Max(1, (int)Math.Round(value * scale));
        }
    }

    internal sealed class InstallerSyntheticClickFilter : IMessageFilter
    {
        private const int WmMouseMove = 0x0200;
        private const int WmLButtonDown = 0x0201;
        private const int WmLButtonUp = 0x0202;
        private readonly ReactInstallerForm owner;

        public InstallerSyntheticClickFilter(ReactInstallerForm target)
        {
            owner = target;
        }

        public bool PreFilterMessage(ref Message message)
        {
            if (owner == null || owner.IsDisposed)
            {
                return false;
            }
            if (message.Msg == WmMouseMove || message.Msg == WmLButtonDown || message.Msg == WmLButtonUp)
            {
                var handledChrome = owner.DispatchInstallerChromeMouseMessage(message.Msg, Cursor.Position);
                if (handledChrome && message.Msg != WmMouseMove)
                {
                    return true;
                }
            }
            if (message.Msg != WmLButtonUp)
            {
                return false;
            }
            return owner.DispatchSyntheticInstallerClickFromScreen(Cursor.Position);
        }
    }

    internal sealed class ReactInstallerForm : Form
    {
        private const string InstallerHostName = "analytix-installer.local";
        private readonly string payloadResourceName;
        private readonly string installerUiResourceName;
        private readonly JavaScriptSerializer serializer = new JavaScriptSerializer();
        private const double InstallerPrepaintOpacity = 0.01D;
        private readonly WebView2 webView;
        private readonly InstallerTrafficControlHitTargets nativeWindowControls;
        private readonly InstallerSyntheticClickFilter syntheticClickFilter;
        private string uiDir;
        private string webViewUserDataDir;
        private bool installed;
        private string installPath;
        private string lifecycleState = "idle";
        private int progressPercent;
        private string detail = "";
        private string lastError = "";
        private double webViewZoomFactor = 1.0;
        private bool installerSurfaceRevealed;

        [DllImport("user32.dll", EntryPoint = "GetSystemMetrics")]
        private static extern int GetSystemMetrics(int metricIndex);

        [DllImport("user32.dll", EntryPoint = "SetWindowPos")]
        private static extern bool SetWindowPos(IntPtr windowHandle, IntPtr insertAfter, int x, int y, int width, int height, uint flags);

        private static readonly IntPtr NoWindowZOrderChange = new IntPtr(0);
        private const int SystemMetricScreenWidth = 0;
        private const int SystemMetricScreenHeight = 1;
        private const uint SetWindowPosNoZOrder = 0x0004;
        private const uint SetWindowPosShowWindow = 0x0040;

        public ReactInstallerForm(string payloadResource, string installerUiResource)
        {
            payloadResourceName = payloadResource;
            installerUiResourceName = installerUiResource;
            installPath = InstallerOperations.ResolveInstalledPath();
            installed = InstallerOperations.IsInstalled(installPath);

            Text = "analytix 安装";
            FormBorderStyle = FormBorderStyle.None;
            StartPosition = FormStartPosition.Manual;
            MinimumSize = new Size(1100, 660);
            Size = new Size(1360, 760);
            BackColor = Color.White;
            Opacity = InstallerPrepaintOpacity;
            ShowInTaskbar = false;
            ApplyInitialWindowBounds(false);
            KeyPreview = true;

            webView = new WebView2
            {
                Dock = DockStyle.Fill,
                DefaultBackgroundColor = Color.White
            };
            nativeWindowControls = new InstallerTrafficControlHitTargets(
                () => Close(),
                () => WindowState = FormWindowState.Minimized,
                ToggleInstallerWindowZoom,
                ResolveCurrentDpiScale
            );
            syntheticClickFilter = new InstallerSyntheticClickFilter(this);

            Controls.Add(webView);
            Shown += ReactInstallerForm_Shown;
            FormClosed += ReactInstallerForm_FormClosed;
            Application.AddMessageFilter(syntheticClickFilter);
        }

        private void ApplyInitialWindowBounds(bool showWindow)
        {
            var screenWidth = Math.Max(1, GetSystemMetrics(SystemMetricScreenWidth));
            var screenHeight = Math.Max(1, GetSystemMetrics(SystemMetricScreenHeight));
            var width = Math.Min(1360, Math.Max(1100, screenWidth - 120));
            var height = Math.Min(760, Math.Max(660, screenHeight - 96));
            if (width > screenWidth)
            {
                width = Math.Max(640, screenWidth - 48);
            }
            if (height > screenHeight)
            {
                height = Math.Max(420, screenHeight - 48);
            }
            var left = Math.Max(0, (screenWidth - width) / 2);
            var top = Math.Max(0, (screenHeight - height) / 2);
            var flags = SetWindowPosNoZOrder;
            if (showWindow)
            {
                flags |= SetWindowPosShowWindow;
            }
            Size = new Size(width, height);
            SetWindowPos(
                Handle,
                NoWindowZOrderChange,
                left,
                top,
                width,
                height,
                flags
            );
        }

        protected override void OnKeyDown(KeyEventArgs e)
        {
            base.OnKeyDown(e);
            if (e.KeyCode == Keys.Escape)
            {
                Close();
            }
        }

        private void ToggleInstallerWindowZoom()
        {
            WindowState = WindowState == FormWindowState.Maximized ? FormWindowState.Normal : FormWindowState.Maximized;
        }

        private float ResolveCurrentDpiScale()
        {
            try
            {
                using (var graphics = CreateGraphics())
                {
                    return Math.Max(1f, graphics.DpiX / 96f);
                }
            }
            catch
            {
                return 1f;
            }
        }

        private async void ReactInstallerForm_Shown(object sender, EventArgs e)
        {
            try
            {
                ApplyInitialWindowBounds(false);
                LogWindowPlacement("shown");
                uiDir = ExtractInstallerUi();
                webViewUserDataDir = Path.Combine(Path.GetTempPath(), "AnalytixBootstrapperRuntime-" + Process.GetCurrentProcess().Id);
                Directory.CreateDirectory(webViewUserDataDir);
                var environment = await CoreWebView2Environment.CreateAsync(null, webViewUserDataDir);
                await webView.EnsureCoreWebView2Async(environment);
                webView.CoreWebView2.Settings.AreDefaultContextMenusEnabled = false;
                webView.CoreWebView2.Settings.AreDevToolsEnabled = false;
                webViewZoomFactor = 1.0;
                webView.ZoomFactor = webViewZoomFactor;
                webView.CoreWebView2.WebMessageReceived += WebView_WebMessageReceived;
                webView.CoreWebView2.NavigationCompleted += WebView_NavigationCompleted;
                webView.CoreWebView2.ProcessFailed += WebView_ProcessFailed;
                await webView.CoreWebView2.AddScriptToExecuteOnDocumentCreatedAsync(CreateInstallerPrepaintScript());
                await webView.CoreWebView2.AddScriptToExecuteOnDocumentCreatedAsync(CreateBridgeScript());
                var indexPath = Path.Combine(uiDir, "index.html");
                webView.CoreWebView2.SetVirtualHostNameToFolderMapping(
                    InstallerHostName,
                    uiDir,
                    CoreWebView2HostResourceAccessKind.Allow
                );
                var targetUrl = "https://" + InstallerHostName + "/index.html#/startup-animation?install=package&platform=win32&standalone=1";
                LogDiagnostic("uiDir=" + uiDir + " indexExists=" + File.Exists(indexPath) + " targetUrl=" + targetUrl);
                webView.Source = new Uri(targetUrl);
                ScheduleInstallerSurfaceFallback();
                LogWindowPlacement("webview-source");
            }
            catch (Exception error)
            {
                RevealInstallerSurface("startup-error");
                File.AppendAllText(
                    Path.Combine(Path.GetTempPath(), "analytix-bootstrapper-error.log"),
                    DateTime.Now.ToString("o") + " " + error + Environment.NewLine
                );
                MessageBox.Show(this, error.Message, "analytix 安装", MessageBoxButtons.OK, MessageBoxIcon.Error);
                Close();
            }
        }

        private async void WebView_NavigationCompleted(object sender, CoreWebView2NavigationCompletedEventArgs e)
        {
            LogDiagnostic("navigation completed success=" + e.IsSuccess + " status=" + e.WebErrorStatus + " source=" + webView.Source);
            if (!e.IsSuccess || webView.CoreWebView2 == null)
            {
                return;
            }
            try
            {
                var location = await webView.CoreWebView2.ExecuteScriptAsync("location.href");
                var title = await webView.CoreWebView2.ExecuteScriptAsync("document.title");
                var bodyLength = await webView.CoreWebView2.ExecuteScriptAsync("String(document.body && document.body.innerText ? document.body.innerText.length : 0)");
                LogDiagnostic("document location=" + location + " title=" + title + " bodyTextLength=" + bodyLength);
            }
            catch (Exception error)
            {
                LogDiagnostic("navigation diagnostic failed " + error);
            }
            await ReleaseInstallerSurfaceAsync("navigation-completed");
        }

        private void ScheduleInstallerSurfaceFallback()
        {
            ThreadPool.QueueUserWorkItem(_ =>
            {
                Thread.Sleep(3500);
                try
                {
                    BeginInvoke(new Action(() => ReleaseInstallerSurfaceFromReady("fallback-timeout")));
                }
                catch
                {
                    // The installer may have closed before the fallback fires.
                }
            });
        }

        private async Task ReleaseInstallerSurfaceAsync(string reason)
        {
            if (installerSurfaceRevealed || IsDisposed)
            {
                return;
            }
            try
            {
                if (webView.CoreWebView2 != null)
                {
                    await webView.CoreWebView2.ExecuteScriptAsync(@"
new Promise((resolve) => {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      try {
        if (window.__analytixReleaseInstallerPrepaint) {
          window.__analytixReleaseInstallerPrepaint();
        }
      } catch (_) {}
      resolve(true);
    });
  });
});
");
                }
            }
            catch (Exception error)
            {
                LogDiagnostic("installer prepaint release failed " + error);
            }
            RevealInstallerSurface(reason);
        }

        private async void ReleaseInstallerSurfaceFromReady(string reason)
        {
            await ReleaseInstallerSurfaceAsync(reason);
        }

        private void RevealInstallerSurface(string reason)
        {
            if (IsDisposed)
            {
                return;
            }
            if (InvokeRequired)
            {
                BeginInvoke(new Action(() => RevealInstallerSurface(reason)));
                return;
            }
            if (installerSurfaceRevealed)
            {
                return;
            }
            installerSurfaceRevealed = true;
            ShowInTaskbar = true;
            Opacity = 1;
            ApplyInitialWindowBounds(true);
            Activate();
            LogWindowPlacement("surface-revealed-" + reason);
        }

        private void WebView_ProcessFailed(object sender, CoreWebView2ProcessFailedEventArgs e)
        {
            LogDiagnostic("webview process failed kind=" + e.ProcessFailedKind + " reason=" + e.Reason + " exitCode=" + e.ExitCode);
        }

        private void ReactInstallerForm_FormClosed(object sender, FormClosedEventArgs e)
        {
            Application.RemoveMessageFilter(syntheticClickFilter);
            try
            {
                if (!string.IsNullOrWhiteSpace(uiDir) && Directory.Exists(uiDir))
                {
                    Directory.Delete(uiDir, true);
                }
            }
            catch
            {
                // Temporary UI cleanup is best-effort.
            }
            try
            {
                if (!string.IsNullOrWhiteSpace(webViewUserDataDir) && Directory.Exists(webViewUserDataDir))
                {
                    Directory.Delete(webViewUserDataDir, true);
                }
            }
            catch
            {
                // WebView2 runtime cleanup is best-effort.
            }
        }

        private string ExtractInstallerUi()
        {
            var targetDir = Path.Combine(Path.GetTempPath(), "AnalytixInstallerUi-" + Process.GetCurrentProcess().Id);
            if (Directory.Exists(targetDir))
            {
                Directory.Delete(targetDir, true);
            }
            Directory.CreateDirectory(targetDir);
            var assembly = Assembly.GetExecutingAssembly();
            using (var resource = assembly.GetManifestResourceStream(installerUiResourceName))
            {
                if (resource == null)
                {
                    throw new FileNotFoundException("React 安装器页面资源缺失。", installerUiResourceName);
                }
                using (var archive = new ZipArchive(resource, ZipArchiveMode.Read))
                {
                    foreach (var entry in archive.Entries)
                    {
                        var destinationPath = Path.GetFullPath(Path.Combine(targetDir, entry.FullName));
                        if (!IsInsideDirectory(destinationPath, targetDir) && !string.Equals(destinationPath, targetDir, StringComparison.OrdinalIgnoreCase))
                        {
                            throw new InvalidOperationException("React 安装器页面包含非法路径: " + entry.FullName);
                        }
                        if (string.IsNullOrEmpty(entry.Name))
                        {
                            Directory.CreateDirectory(destinationPath);
                            continue;
                        }
                        Directory.CreateDirectory(Path.GetDirectoryName(destinationPath));
                        using (var source = entry.Open())
                        using (var file = File.Create(destinationPath))
                        {
                            source.CopyTo(file);
                        }
                    }
                }
            }
            return targetDir;
        }

        private static bool IsInsideDirectory(string path, string directory)
        {
            var fullPath = Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
            var fullDirectory = Path.GetFullPath(directory).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar) + Path.DirectorySeparatorChar;
            return fullPath.StartsWith(fullDirectory, StringComparison.OrdinalIgnoreCase);
        }

        private static void LogDiagnostic(string message)
        {
            try
            {
                File.AppendAllText(
                    Path.Combine(Path.GetTempPath(), "analytix-bootstrapper-webview.log"),
                    DateTime.Now.ToString("o") + " " + message + Environment.NewLine
                );
            }
            catch
            {
                // Diagnostic logging must never block the installer UI.
            }
        }

        private void LogWindowPlacement(string phase)
        {
            try
            {
                LogDiagnostic(
                    "window " + phase +
                    " screen=" + GetSystemMetrics(SystemMetricScreenWidth) + "x" + GetSystemMetrics(SystemMetricScreenHeight) +
                    " bounds=" + Bounds.Left + "," + Bounds.Top + "," + Bounds.Width + "," + Bounds.Height
                );
            }
            catch
            {
                // Placement diagnostics are best-effort.
            }
        }

        private void WebView_WebMessageReceived(object sender, CoreWebView2WebMessageReceivedEventArgs e)
        {
            Dictionary<string, object> message;
            try
            {
                message = serializer.Deserialize<Dictionary<string, object>>(e.WebMessageAsJson);
            }
            catch
            {
                return;
            }
            var id = TextValue(message, "id");
            var channel = TextValue(message, "channel");
            var method = TextValue(message, "method");
            var payload = message.ContainsKey("payload") ? message["payload"] as Dictionary<string, object> : null;
            LogDiagnostic("webMessage method=" + method + " id=" + id);

            if (string.Equals(channel, "analytix-installer-surface", StringComparison.OrdinalIgnoreCase) &&
                string.Equals(method, "surfaceReady", StringComparison.OrdinalIgnoreCase))
            {
                ReleaseInstallerSurfaceFromReady("surface-ready");
                return;
            }
            if (string.Equals(method, "getPackageInstallerState", StringComparison.OrdinalIgnoreCase))
            {
                Resolve(id, CreateState());
                return;
            }
            if (string.Equals(method, "pickDirectory", StringComparison.OrdinalIgnoreCase))
            {
                Resolve(id, PickInstallDirectory());
                return;
            }
            if (string.Equals(method, "closeApp", StringComparison.OrdinalIgnoreCase))
            {
                Resolve(id, true);
                Close();
                return;
            }
            if (string.Equals(method, "minimizeWindow", StringComparison.OrdinalIgnoreCase))
            {
                WindowState = FormWindowState.Minimized;
                Resolve(id, true);
                return;
            }
            if (string.Equals(method, "toggleFullscreenWindow", StringComparison.OrdinalIgnoreCase))
            {
                ToggleInstallerWindowZoom();
                Resolve(id, true);
                return;
            }
            if (string.Equals(method, "installPackage", StringComparison.OrdinalIgnoreCase))
            {
                var selectedPath = TextValue(payload, "installPath");
                RunPackageOperation(id, true, string.IsNullOrWhiteSpace(selectedPath) ? installPath : selectedPath);
                return;
            }
            if (string.Equals(method, "uninstallPackage", StringComparison.OrdinalIgnoreCase))
            {
                RunPackageOperation(id, false, installPath);
                return;
            }
            Resolve(id, null);
        }

        private string PickInstallDirectory()
        {
            using (var dialog = new FolderBrowserDialog())
            {
                dialog.Description = "选择 analytix 安装路径";
                dialog.SelectedPath = Directory.Exists(installPath) ? installPath : Path.GetDirectoryName(installPath);
                if (dialog.ShowDialog(this) != DialogResult.OK)
                {
                    return "";
                }
                installPath = Path.Combine(dialog.SelectedPath, "Analytix");
                EmitState();
                return installPath;
            }
        }

        private void RunPackageOperation(string id, bool install, string selectedPath)
        {
            ThreadPool.QueueUserWorkItem(_ =>
            {
                try
                {
                    installPath = string.IsNullOrWhiteSpace(selectedPath) ? Program.DefaultInstallPath() : selectedPath;
                    lifecycleState = install ? "installing" : "uninstalling";
                    progressPercent = install ? 12 : 18;
                    detail = install ? "VERIFYING INSTALL LOCATION" : "PREPARING UNINSTALLER";
                    lastError = "";
                    BeginInvoke(new Action(EmitState));

                    if (install)
                    {
                        InstallerOperations.RunElevatedInstall(installPath, ReportNativePackageProgress);
                        installed = true;
                        progressPercent = Math.Max(progressPercent, 96);
                        detail = "STARTING ANALYTIX";
                        BeginInvoke(new Action(EmitState));
                        InstallerOperations.LaunchInstalledApp(installPath);
                        progressPercent = 100;
                        detail = "INSTALLATION COMPLETE";
                    }
                    else
                    {
                        InstallerOperations.RunElevatedUninstall(installPath, ReportNativePackageProgress);
                        installed = false;
                        progressPercent = 100;
                        detail = "UNINSTALL COMPLETE";
                    }
                    lifecycleState = "completed";
                    BeginInvoke(new Action(() =>
                    {
                        EmitState();
                        Resolve(id, CreateState());
                    }));
                }
                catch (Exception error)
                {
                    lifecycleState = "failed";
                    lastError = error.Message;
                    BeginInvoke(new Action(() =>
                    {
                        EmitState();
                        Resolve(id, CreateState());
                    }));
                }
            });
        }

        internal bool DispatchSyntheticInstallerClickFromScreen(Point screenPoint)
        {
            if (webView.CoreWebView2 == null || WindowState == FormWindowState.Minimized)
            {
                return false;
            }
            var clientPoint = PointToClient(screenPoint);
            if (!ClientRectangle.Contains(clientPoint) || nativeWindowControls.Bounds.Contains(clientPoint))
            {
                return false;
            }
            if (!IsPromptActionCandidate(clientPoint))
            {
                return false;
            }
            DispatchSyntheticClickAt(clientPoint);
            return true;
        }

        internal bool DispatchInstallerChromeMouseMessage(int message, Point screenPoint)
        {
            var clientPoint = PointToClient(screenPoint);
            if (!ClientRectangle.Contains(clientPoint))
            {
                nativeWindowControls.ClearExternalState();
                return false;
            }
            if (message == 0x0200)
            {
                nativeWindowControls.HandleExternalMouseMove(clientPoint);
                return false;
            }
            if (message == 0x0201)
            {
                return nativeWindowControls.HandleExternalMouseDown(clientPoint);
            }
            if (message == 0x0202)
            {
                return nativeWindowControls.HandleExternalMouseUp(clientPoint);
            }
            return false;
        }

        private bool IsPromptActionCandidate(Point clientPoint)
        {
            if (!string.Equals(lifecycleState, "idle", StringComparison.OrdinalIgnoreCase) &&
                !string.Equals(lifecycleState, "completed", StringComparison.OrdinalIgnoreCase) &&
                !string.Equals(lifecycleState, "failed", StringComparison.OrdinalIgnoreCase))
            {
                return false;
            }
            var minX = ClientSize.Width / 2 - 520;
            var maxX = ClientSize.Width / 2 + 520;
            var minY = (int)Math.Round(ClientSize.Height * 0.42);
            var maxY = (int)Math.Round(ClientSize.Height * 0.86);
            return clientPoint.X >= minX && clientPoint.X <= maxX && clientPoint.Y >= minY && clientPoint.Y <= maxY;
        }

        private void DispatchSyntheticClickAt(Point clientPoint)
        {
            try
            {
                var zoom = webViewZoomFactor <= 0 ? 1.0 : webViewZoomFactor;
                var x = Math.Max(0, (int)Math.Round(clientPoint.X / zoom));
                var y = Math.Max(0, (int)Math.Round(clientPoint.Y / zoom));
                var script = @"
(() => {
  const element = document.elementFromPoint(" + x + @", " + y + @");
  const target = element && element.closest('button:not(:disabled), [role=""button""]:not([aria-disabled=""true""])');
  if (!target) return false;
  target.focus && target.focus({ preventScroll: true });
  target.click();
  return true;
})();
";
                webView.CoreWebView2.ExecuteScriptAsync(script);
            }
            catch (Exception error)
            {
                LogDiagnostic("synthetic click failed " + error);
            }
        }

        private void ReportNativePackageProgress(int percent, string nextDetail)
        {
            if (IsDisposed)
            {
                return;
            }
            try
            {
                BeginInvoke(new Action(() =>
                {
                    progressPercent = Math.Max(progressPercent, percent);
                    if (!string.IsNullOrWhiteSpace(nextDetail))
                    {
                        detail = nextDetail;
                    }
                    EmitState();
                }));
            }
            catch
            {
                // Progress updates are visual only; the install child process remains authoritative.
            }
        }

        private Dictionary<string, object> CreateState()
        {
            return new Dictionary<string, object>
            {
                { "source", "native" },
                { "platform", "win32" },
                { "packageEdition", "standard" },
                { "version", InstallerOperations.CurrentVersionText },
                { "installedVersion", InstallerOperations.GetInstalledVersionText(installPath) },
                { "canUpdate", InstallerOperations.IsCurrentPackageNewerThanInstalled(installPath) },
                { "installed", installed },
                { "installPath", installPath },
                { "canChooseInstallPath", !installed && lifecycleState == "idle" },
                { "canInstall", lifecycleState == "idle" || lifecycleState == "completed" || lifecycleState == "failed" },
                { "canUninstall", installed },
                { "lifecycleState", lifecycleState },
                { "progressPercent", progressPercent },
                { "detail", detail },
                { "lastError", lastError },
                { "updatedAt", DateTime.UtcNow.ToString("o") }
            };
        }

        private void EmitState()
        {
            if (webView.CoreWebView2 == null)
            {
                return;
            }
            var script = "window.__analytixInstallerEmitPackageState && window.__analytixInstallerEmitPackageState(" +
                serializer.Serialize(CreateState()) +
                ");";
            webView.CoreWebView2.ExecuteScriptAsync(script);
        }

        private void Resolve(string id, object result)
        {
            if (webView.CoreWebView2 == null || string.IsNullOrWhiteSpace(id))
            {
                return;
            }
            var script = "window.__analytixInstallerResolve && window.__analytixInstallerResolve(" +
                serializer.Serialize(id) +
                "," +
                serializer.Serialize(result) +
                ",null);";
            webView.CoreWebView2.ExecuteScriptAsync(script);
        }

        private static string TextValue(Dictionary<string, object> record, string key)
        {
            if (record == null || !record.ContainsKey(key) || record[key] == null)
            {
                return "";
            }
            return Convert.ToString(record[key]);
        }

        private static string CreateInstallerPrepaintScript()
        {
            return @"
(() => {
  const attributeName = 'data-analytix-installer-prepaint';
  const styleId = 'analytix-installer-prepaint-style';
  let readyPosted = false;

  function appendStyle() {
    if (!document.head || document.getElementById(styleId)) return;
    const style = document.createElement('style');
    style.id = styleId;
    style.textContent = `
html[${attributeName}=""1""],
html[${attributeName}=""1""] body,
html[${attributeName}=""1""] #root {
  background: #ffffff !important;
}
html[${attributeName}=""1""] *,
html[${attributeName}=""1""] *::before,
html[${attributeName}=""1""] *::after {
  animation-play-state: paused !important;
  transition-duration: 0s !important;
}`;
    document.head.appendChild(style);
  }

  function postReady() {
    if (readyPosted) return;
    readyPosted = true;
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        try {
          if (window.chrome && window.chrome.webview) {
            window.chrome.webview.postMessage({
              channel: 'analytix-installer-surface',
              method: 'surfaceReady'
            });
          }
        } catch (_) {}
      });
    });
  }

  function waitForStartupStage() {
    let checks = 0;
    const check = () => {
      if (
        document.querySelector('.startup-animation-stage') ||
        (document.body && document.body.classList.contains('analytix-react-ready')) ||
        checks >= 160
      ) {
        postReady();
        return;
      }
      checks += 1;
      setTimeout(check, 25);
    };
    check();
  }

  document.documentElement.setAttribute(attributeName, '1');
  appendStyle();
  document.addEventListener('DOMContentLoaded', appendStyle, { once: true });
  setTimeout(waitForStartupStage, 0);
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', waitForStartupStage, { once: true });
  } else {
    waitForStartupStage();
  }

  window.__analytixReleaseInstallerPrepaint = () => {
    document.documentElement.removeAttribute(attributeName);
    const style = document.getElementById(styleId);
    if (style) style.remove();
    return true;
  };
})();
";
        }

        private static string CreateBridgeScript()
        {
            return @"
(() => {
  const pending = new Map();
  const packageListeners = new Set();
  let nextId = 1;
  function invoke(method, payload) {
    return new Promise((resolve, reject) => {
      const id = String(nextId++);
      pending.set(id, { resolve, reject });
      window.chrome.webview.postMessage({ channel: 'analytix-installer', id, method, payload: payload || {} });
    });
  }
  window.__analytixInstallerResolve = (id, result, error) => {
    const entry = pending.get(String(id));
    if (!entry) return;
    pending.delete(String(id));
    if (error) {
      entry.reject(new Error(String(error)));
      return;
    }
    entry.resolve(result);
  };
  window.__analytixInstallerEmitPackageState = (state) => {
    for (const listener of Array.from(packageListeners)) {
      try { listener(state); } catch (_) {}
    }
  };
  const installerBridge = {
    version: 'winforms-webview2-bootstrapper-1.0.6',
    platform: 'win32',
    closeApp: () => invoke('closeApp'),
    closeWindow: () => invoke('closeApp'),
    minimizeWindow: () => invoke('minimizeWindow'),
    toggleFullscreenWindow: () => invoke('toggleFullscreenWindow'),
    setWindowChrome: () => Promise.resolve(true),
    getRuntimeInfo: () => Promise.resolve(null),
    getPackageInstallerState: () => invoke('getPackageInstallerState'),
    installPackage: (params) => invoke('installPackage', params || {}),
    uninstallPackage: () => invoke('uninstallPackage'),
    onPackageInstallerState: (callback) => {
      packageListeners.add(callback);
      return () => packageListeners.delete(callback);
    },
    pickDirectory: () => invoke('pickDirectory'),
    pickFiles: () => Promise.resolve([]),
    openPath: () => Promise.resolve(false)
  };
  window.analytixInstaller = installerBridge;
  const previousAnalytix = window.analytix || {};
  window.analytix = {
    ...previousAnalytix,
    app: {
      ...(previousAnalytix.app || {}),
      platform: 'win32',
      startupSurfaceReady: () => Promise.resolve(undefined),
      runDesktopCommand: (command) => {
        if (command === 'close' || command === 'quit') return installerBridge.closeWindow().then(() => undefined);
        if (command === 'minimize') return installerBridge.minimizeWindow().then(() => undefined);
        if (command === 'toggleFullscreen' || command === 'toggleMaximize') return installerBridge.toggleFullscreenWindow().then(() => undefined);
        return Promise.resolve(undefined);
      }
    }
  };
})();
";
        }
    }

#endif


}
