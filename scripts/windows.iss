; Complete, per-user, offline installer. Uninstall never removes clinic state.
[Setup]
AppId=network.ohc.care-anywhere
AppName=CARE Anywhere
AppVersion={#AppVersion}
DefaultDirName={localappdata}\Programs\CARE Anywhere
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutputDir}
OutputBaseFilename=CARE-Anywhere-{#AppVersion}-windows-amd64
Compression=lzma2/fast
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\care-anywhere.exe
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Files]
Source: "{#AppSource}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{userprograms}\CARE Anywhere"; Filename: "{app}\care-anywhere.exe"
Name: "{userdesktop}\CARE Anywhere"; Filename: "{app}\care-anywhere.exe"

[Run]
Filename: "{app}\care-anywhere.exe"; Description: "Open CARE Anywhere"; Flags: nowait postinstall skipifsilent
