; wspace.iss - Inno Setup installer script for the wspace CLI
; (design.md §11's "Packaging targets" table: "an Inno Setup installer that
; also handles the PATH entry"). Windows ships the CLI only.
;
; Compiled with Inno Setup's `iscc` (https://jrsoftware.org/isinfo.php),
; Windows-only, so it must be compiled on a Windows machine or a Windows CI
; runner.
;
; Prerequisite: dist\windows\wspace.exe must already exist (a pure-Go
; GOOS=windows CGO_ENABLED=0 cross-build, e.g. `make dist`).
;
; Code-signing: this script does not sign the installer or the binary.
; Windows SmartScreen will warn on an unsigned installer's first run.
; Signing would require:
;   1. An Authenticode code-signing certificate (OV or EV) from a public CA.
;   2. signtool.exe (from the Windows SDK) run against wspace.exe
;      and the compiled installer itself, e.g.:
;        signtool sign /fd sha256 /a /tr http://timestamp.digicert.com /td sha256 dist\windows\wspace.exe
;   3. Wiring that signing step into a Windows release job, reading the
;      certificate from a repository secret — never committed, never
;      hardcoded.
; No certificate is available to this project yet.
; This script's job is to leave a clean place to add signing later, not to
; fake it now.

#define MyAppName "wspace"
#ifndef MyAppVersion
  #define MyAppVersion "0.0.0-dev"
#endif

[Setup]
AppId={{2E9F0F2A-9E7B-4B6C-9F1E-6B3F6E6E2C11}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=wspace project contributors
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
OutputDir=..\..\dist\windows
OutputBaseFilename=wspace-setup-{#MyAppVersion}
Compression=lzma2
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64
; PrivilegesRequired=lowest keeps this a per-user install (HKCU-only PATH
; edit below), matching the packaging-distribution spec's own "install
; command never elevates" requirement for the CLI itself — the installer
; mirrors that same no-elevation posture rather than contradicting it.
PrivilegesRequired=lowest
WizardStyle=modern

[Files]
Source: "..\..\dist\windows\wspace.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"

[Code]
const
  EnvironmentKey = 'Environment';

// --- PATH environment registration ---
// Appends {app} to the current user's PATH exactly once (skips silently
// if already present, so a repeat install/repair never duplicates the
// entry) — the packaging-distribution spec's own "Installer completes
// without manual PATH edit" scenario. HKCU only, matching
// PrivilegesRequired=lowest above: no machine-wide registry write, no
// elevation prompt. A new shell picks up HKCU\Environment automatically;
// an already-open shell does not, which is expected and matches the
// spec's own "from a new shell" wording.
procedure EnvAddPath(Value: string);
var
  Paths: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, EnvironmentKey, 'Path', Paths) then
    Paths := '';

  if Pos(';' + Uppercase(Value) + ';', ';' + Uppercase(Paths) + ';') > 0 then
    exit; // already present

  if (Length(Paths) > 0) and (Paths[Length(Paths)] <> ';') then
    Paths := Paths + ';';
  Paths := Paths + Value;

  if not RegWriteStringValue(HKEY_CURRENT_USER, EnvironmentKey, 'Path', Paths) then
    Log('EnvAddPath: failed to write Path');
end;

// EnvRemovePath undoes EnvAddPath on uninstall, leaving no dangling PATH
// entry behind for a directory that no longer exists.
procedure EnvRemovePath(Value: string);
var
  Paths: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, EnvironmentKey, 'Path', Paths) then
    exit;

  P := Pos(';' + Uppercase(Value) + ';', ';' + Uppercase(Paths) + ';');
  if P = 0 then
    exit;

  Delete(Paths, P, Length(Value) + 1);
  RegWriteStringValue(HKEY_CURRENT_USER, EnvironmentKey, 'Path', Paths);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    EnvAddPath(ExpandConstant('{app}'));
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    EnvRemovePath(ExpandConstant('{app}'));
end;
