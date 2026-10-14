# Installs hat and the hatrack tray app for the current user, and starts the tray with Windows.
# irm https://raw.githubusercontent.com/phongptn93/hatrack/main/install.ps1 | iex
$ErrorActionPreference = 'Stop'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$dir = Join-Path $env:LOCALAPPDATA 'hatrack'
$base = 'https://github.com/phongptn93/hatrack/releases/latest/download'
New-Item -ItemType Directory -Force $dir | Out-Null

# a running tray locks its exe
Get-Process hatrack-tray -ErrorAction SilentlyContinue | ForEach-Object { $_.Kill(); $_.WaitForExit() }
foreach ($f in 'hat', 'hatrack-tray') {
    Invoke-WebRequest "$base/$f-windows-$arch.exe" -OutFile (Join-Path $dir "$f.exe") -UseBasicParsing
}

$path = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($path -split ';') -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', "$path;$dir", 'User')
}

$tray = Join-Path $dir 'hatrack-tray.exe'
$lnk = (New-Object -ComObject WScript.Shell).CreateShortcut((Join-Path ([Environment]::GetFolderPath('Startup')) 'hatrack.lnk'))
$lnk.TargetPath = $tray
$lnk.Save()
Start-Process $tray
Write-Host "Installed to $dir. hatrack is in the tray (check the ^ overflow); open a new terminal to use 'hat'."
