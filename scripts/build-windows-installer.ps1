$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$version = (Get-Content "$projectRoot/VERSION").Trim()
$tempRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }
$compilerRoot = Join-Path $tempRoot 'node-test-innosetup-7.1.0'
$compiler = Join-Path $compilerRoot 'ISCC.exe'
if (!(Test-Path $compiler)) {
    $download = Join-Path ([IO.Path]::GetTempPath()) 'node-test-innosetup-7.1.0.exe'
    Invoke-WebRequest -Uri 'https://github.com/jrsoftware/issrc/releases/download/is-7_1_0/innosetup-7.1.0-x64.exe' -OutFile $download
    $expected = '0362a383ed217d4c4239b5933866dd96d3eb2102737da92f80f6057a4b40df2f'
    if ((Get-FileHash $download -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw 'Inno Setup checksum mismatch' }
    $proc = Start-Process -FilePath $download -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',"/DIR=`"$compilerRoot`"") -Wait -PassThru
    if ($proc.ExitCode -ne 0) { throw "Compiler setup failed: $($proc.ExitCode)" }
}
& $compiler "/DAppVersion=$version" "/DSourceDir=$projectRoot/dist/NodeTest-win32-x64" "/DOutputDir=$projectRoot/dist" "$projectRoot/packaging/windows.iss"
if ($LASTEXITCODE -ne 0) { throw 'Installer compilation failed' }
