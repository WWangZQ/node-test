; SPDX-License-Identifier: AGPL-3.0-only
#ifndef AppVersion
  #error AppVersion is required
#endif
#ifndef SourceDir
  #error SourceDir is required
#endif
#ifndef OutputDir
  #error OutputDir is required
#endif

[Setup]
AppId={{CA46B11B-DB6D-4FC2-B29B-22D53F964015}
AppName=节点测评
AppVersion={#AppVersion}
AppPublisher=WWangZQ
AppPublisherURL=https://github.com/WWangZQ/node-test
AppSupportURL=https://github.com/WWangZQ/node-test/issues
AppUpdatesURL=https://github.com/WWangZQ/node-test/releases/latest
DefaultDirName={localappdata}\Programs\NodeTest
DefaultGroupName=节点测评
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#OutputDir}
OutputBaseFilename=NodeTest-{#AppVersion}-windows-x64-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\NodeTest.exe
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Tasks]
Name: desktopicon; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:";

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\节点测评"; Filename: "{app}\NodeTest.exe"
Name: "{autodesktop}\节点测评"; Filename: "{app}\NodeTest.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\NodeTest.exe"; Description: "Launch 节点测评"; Flags: nowait postinstall skipifsilent
