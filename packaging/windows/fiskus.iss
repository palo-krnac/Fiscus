; Inštalátor pre Windows (Inno Setup 6). Zostavuje release.yml:
;   iscc /DAppVersion=1.2.3 /DSourceDir=..\..\dist packaging\windows\fiskus.iss
; Inštaluje sa pre aktuálneho používateľa (bez práv správcu); dáta sú
; v %APPDATA%\Fiskus a odinštalovanie ich nemaže.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir "..\..\dist"
#endif

[Setup]
AppId={{6B0E8F52-5C1A-4C7B-9A3E-2F1D7C0B4F11}
AppName=Fiškus
AppVersion={#AppVersion}
AppVerName=Fiškus {#AppVersion}
AppPublisher=Pavol Krnáč
AppPublisherURL=https://fiskus.sk
AppSupportURL=https://fiskus.sk
DefaultDirName={localappdata}\Programs\Fiskus
DefaultGroupName=Fiškus
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir={#SourceDir}
OutputBaseFilename=fiskus-{#AppVersion}-windows-setup
SetupIconFile=..\icons\fiskus.ico
UninstallDisplayIcon={app}\fiskus.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=force

[Languages]
; slovenčina je v Inno Setup medzi prekladmi; ak chýba, inštalátor je po anglicky
#if FileExists(CompilerPath + "Languages\Slovak.isl")
Name: "sk"; MessagesFile: "compiler:Languages\Slovak.isl"
#else
Name: "en"; MessagesFile: "compiler:Default.isl"
#endif

[Tasks]
Name: "desktopicon"; Description: "Ikona na pracovnej ploche"; GroupDescription: "Ďalšie ikony:"

[Files]
Source: "{#SourceDir}\fiskus-windows-amd64-app.exe"; DestDir: "{app}"; DestName: "fiskus.exe"; Flags: ignoreversion
Source: "{#SourceDir}\fiskus-windows-amd64.exe"; DestDir: "{app}"; DestName: "fiskus-cli.exe"; Flags: ignoreversion
Source: "{#SourceDir}\fiskus-mcp-windows-amd64.exe"; DestDir: "{app}"; DestName: "fiskus-mcp.exe"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\Fiškus"; Filename: "{app}\fiskus.exe"; Parameters: "-app"
Name: "{autodesktop}\Fiškus"; Filename: "{app}\fiskus.exe"; Parameters: "-app"; Tasks: desktopicon

[Run]
Filename: "{app}\fiskus.exe"; Parameters: "-app"; Description: "Spustiť Fiškus"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{sys}\taskkill.exe"; Parameters: "/IM fiskus.exe /F"; Flags: runhidden; RunOnceId: "StopFiskus"
