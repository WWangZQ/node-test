$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$version = (Get-Content "$projectRoot/VERSION").Trim()
$installer = Join-Path $projectRoot "dist/NodeTest-$version-windows-x64-Setup.exe"
$destination = Join-Path ([IO.Path]::GetTempPath()) 'node-test-installer-fixture'
$evidence = Join-Path $projectRoot 'dist/installer-smoke'
New-Item -ItemType Directory -Force $evidence | Out-Null
$env:NODE_TEST_SMOKE_OUTPUT = $evidence
$shortcut = Join-Path ([Environment]::GetFolderPath('Desktop')) '节点测评.lnk'
$startMenu = Join-Path ([Environment]::GetFolderPath('Programs')) '节点测评/节点测评.lnk'
if (Test-Path $destination) { throw 'Installer test destination already exists' }
if ((Test-Path $shortcut) -or (Test-Path $startMenu)) { throw 'Installer test requires a fresh runner profile' }
try {
    $proc = Start-Process -FilePath $installer -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/TASKS=desktopicon',"/DIR=`"$destination`"", "/LOG=`"$evidence/install.log`"") -Wait -PassThru
    if ($proc.ExitCode -ne 0) { throw "Installation failed: $($proc.ExitCode)" }
    foreach ($required in @("$destination/NodeTest.exe", "$destination/resources/node-worker.exe", "$destination/unins000.exe", $shortcut, $startMenu)) {
        if (!(Test-Path $required)) { throw "Missing installed component: $required" }
    }
    $proc = Start-Process -FilePath "$destination/NodeTest.exe" -ArgumentList '--smoke-test' -Wait -PassThru
    if ($proc.ExitCode -ne 0) { throw "Installed app smoke test failed: $($proc.ExitCode)" }
    if (!(Test-Path "$evidence/smoke-result.json")) { throw 'Installed app smoke evidence missing' }
    $preserve = Join-Path $env:APPDATA 'NodeTestPublic/upgrade-preserve.txt'
    New-Item -ItemType Directory -Force (Split-Path $preserve) | Out-Null
    Set-Content $preserve 'retained user data'
    Remove-Item "$evidence/smoke-result.json"
    $proc = Start-Process -FilePath $installer -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/UPDATE=1','/UPDATETEST=1',"/DIR=`"$destination`"", "/LOG=`"$evidence/update.log`"") -Wait -PassThru
    if ($proc.ExitCode -ne 0) { throw "Update install failed: $($proc.ExitCode)" }
    $deadline = (Get-Date).AddSeconds(90)
    while (!(Test-Path "$evidence/smoke-result.json") -and (Get-Date) -lt $deadline) { Start-Sleep -Seconds 1 }
    if (!(Test-Path "$evidence/smoke-result.json")) { throw 'Updated application did not relaunch' }
    $updated = Get-Content "$evidence/smoke-result.json" | ConvertFrom-Json
    if ($updated.version -ne $version) { throw 'Relaunched app version mismatch' }
    if ((Get-Content $preserve).Trim() -ne 'retained user data') { throw 'Update changed user data' }

} finally {
    if (Test-Path "$destination/unins000.exe") {
        $proc = Start-Process -FilePath "$destination/unins000.exe" -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',"/LOG=`"$evidence/uninstall.log`"") -Wait -PassThru
        if ($proc.ExitCode -ne 0) { throw "Uninstall failed: $($proc.ExitCode)" }
    }
}
foreach ($removed in @("$destination/NodeTest.exe", "$destination/resources/node-worker.exe", $shortcut, $startMenu)) {
    if (Test-Path $removed) { throw "Uninstall left behind component: $removed" }
}
@{version=$version; installation='passed'; shortcuts='passed'; installedApp='passed'; updateRelaunch='passed'; preservedData='passed'; uninstall='passed'} | ConvertTo-Json | Set-Content "$evidence/installer-result.json"
Write-Output 'Installer, shortcuts, installed application and uninstall verified'
