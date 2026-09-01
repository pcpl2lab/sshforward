; Inno Setup script for the sshforward Windows installer.
;
; One installer serves all three Windows architectures: the right binary is
; chosen at install time, so nobody has to know whether their machine is x64,
; arm64 or 32-bit.
;
; BinDir must contain one subdirectory per architecture:
;   <BinDir>\386\sshforward.exe
;   <BinDir>\amd64\sshforward.exe
;   <BinDir>\arm64\sshforward.exe
;
; Built in CI by the release workflow:
;   ISCC /DAppVersion=1.2.3 /DBinDir=..\winbin installer\sshforward.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef BinDir
  #define BinDir "..\winbin"
#endif
#ifndef OutputName
  #define OutputName "sshforward_setup"
#endif

#define AppName "sshforward"
#define AppPublisher "Patryk Ławicki"
#define AppURL "https://github.com/pcpl2lab/sshforward"

[Setup]
; Never change AppId: Windows identifies an upgrade of an existing install by
; it. A new value would install a second copy alongside the first.
AppId={{923ADEC1-21DD-42F7-A149-0AAAB97049F9}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
VersionInfoVersion={#AppVersion}

; A per-user install needs no administrator, which matches the binary's own
; manifest (asInvoker) and keeps the tool out of UAC prompts entirely.
PrivilegesRequired=lowest
DefaultDirName={autopf}\{#AppName}
DisableProgramGroupPage=yes
DisableDirPage=auto
UninstallDisplayName={#AppName} {#AppVersion}
UninstallDisplayIcon={app}\sshforward.exe

LicenseFile=..\LICENSE
OutputDir=..\dist
OutputBaseFilename={#OutputName}
SetupIconFile=..\winres\icon.ico
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x86compatible x64compatible arm64

; The installer edits PATH, so tell Explorer to refresh its environment.
ChangesEnvironment=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#BinDir}\amd64\sshforward.exe"; DestDir: "{app}"; Check: IsTargetX64; Flags: ignoreversion
Source: "{#BinDir}\arm64\sshforward.exe"; DestDir: "{app}"; Check: IsTargetArm64; Flags: ignoreversion
Source: "{#BinDir}\386\sshforward.exe"; DestDir: "{app}"; Check: IsTargetX86; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "addtopath"; Description: "Add sshforward to my PATH"; GroupDescription: "Command line:"

[Code]
const
  EnvironmentKey = 'Environment';

// One of these three is true for any machine the installer runs on. They are
// mutually exclusive on purpose: an arm64 machine also reports x64
// compatibility through emulation, and would otherwise match twice.
function IsTargetArm64: Boolean;
begin
  Result := ProcessorArchitecture = paArm64;
end;

function IsTargetX64: Boolean;
begin
  Result := ProcessorArchitecture = paX64;
end;

function IsTargetX86: Boolean;
begin
  Result := ProcessorArchitecture = paX86;
end;

function PathListContains(const Haystack, Needle: string): Boolean;
begin
  Result := Pos(';' + Uppercase(Needle) + ';', ';' + Uppercase(Haystack) + ';') > 0;
end;

// AddToPath appends the install directory to the user's PATH, if it is not
// already there. Per-user PATH avoids needing administrator rights.
procedure AddToPath();
var
  Existing: string;
begin
  if not RegQueryStringValue(HKCU, EnvironmentKey, 'Path', Existing) then
    Existing := '';
  if PathListContains(Existing, ExpandConstant('{app}')) then
    Exit;
  if (Existing <> '') and (Copy(Existing, Length(Existing), 1) <> ';') then
    Existing := Existing + ';';
  RegWriteExpandStringValue(HKCU, EnvironmentKey, 'Path', Existing + ExpandConstant('{app}'));
end;

// RemoveFromPath drops the install directory again on uninstall, leaving any
// other entries the user has untouched.
procedure RemoveFromPath();
var
  Existing, Target: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKCU, EnvironmentKey, 'Path', Existing) then
    Exit;
  Target := ExpandConstant('{app}');
  P := Pos(';' + Uppercase(Target) + ';', ';' + Uppercase(Existing) + ';');
  if P = 0 then
    Exit;
  Delete(Existing, P, Length(Target) + 1);
  if (Existing <> '') and (Copy(Existing, Length(Existing), 1) = ';') then
    Delete(Existing, Length(Existing), 1);
  RegWriteExpandStringValue(HKCU, EnvironmentKey, 'Path', Existing);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and WizardIsTaskSelected('addtopath') then
    AddToPath();
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath();
end;
